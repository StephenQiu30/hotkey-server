package http

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"testing"
	"time"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/gin-gonic/gin"
)

func TestModelProfileHTTPPostgresLifecyclePreservesWriteOnlyCredentialBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := openModelProfileHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()
	service, err := intelligenceapplication.NewModelProfileService(intelligencepostgres.NewRepository(runtime))
	if err != nil {
		t.Fatalf("NewModelProfileService(): %v", err)
	}
	router := newModelProfileRouter(service, httptransport.RoleAdmin)

	create := `{"name":"integration-embedding","task_type":"embedding","provider":"openai","model_name":"text-embedding-3-large","model_version":"2026-07","credential_ref":"env:OPENAI_API_KEY","embedding_dimensions":1024,"timeout_seconds":30,"max_attempts":2,"max_cost":"0.1000","daily_budget":"10.0000","fallback_priority":100,"enabled":true}`
	response := modelProfileRequest(router, stdhttp.MethodPost, "/api/v1/ai/model-profiles", create, "admin")
	if response.Code != stdhttp.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", response.Code, response.Body.String())
	}
	assertNoModelProfileSecret(t, response.Body.String())
	var created struct {
		Data ModelProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created profile: %v", err)
	}
	if created.Data.ID <= 0 || created.Data.Version != 1 || created.Data.CreatedAt.IsZero() || created.Data.UpdatedAt.IsZero() {
		t.Fatalf("created profile = %#v", created.Data)
	}

	response = modelProfileRequest(router, stdhttp.MethodPatch, fmt.Sprintf("/api/v1/ai/model-profiles/%d", created.Data.ID), `{"version":1,"timeout_seconds":45,"daily_budget":null}`, "admin")
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var updated struct {
		Data ModelProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated profile: %v", err)
	}
	if updated.Data.Version != 2 || updated.Data.TimeoutSeconds != 45 || updated.Data.DailyBudget != nil {
		t.Fatalf("updated profile = %#v", updated.Data)
	}
	assertNoModelProfileSecret(t, response.Body.String())

	response = modelProfileRequest(router, stdhttp.MethodDelete, fmt.Sprintf("/api/v1/ai/model-profiles/%d", created.Data.ID), `{"version":2}`, "admin")
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var deleted struct {
		Data ModelProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("decode deleted profile: %v", err)
	}
	if deleted.Data.Version != 3 || !deleted.Data.Deleted {
		t.Fatalf("deleted profile = %#v", deleted.Data)
	}

	response = modelProfileRequest(router, stdhttp.MethodPost, fmt.Sprintf("/api/v1/ai/model-profiles/%d/restore", created.Data.ID), `{"version":3}`, "admin")
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("restore status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var restored struct {
		Data ModelProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatalf("decode restored profile: %v", err)
	}
	if restored.Data.Version != 4 || restored.Data.Deleted {
		t.Fatalf("restored profile = %#v", restored.Data)
	}

	response = modelProfileRequest(router, stdhttp.MethodGet, "/api/v1/ai/model-profiles", "", "admin")
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var listed struct {
		Data ModelProfileListResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listed profiles: %v", err)
	}
	if len(listed.Data.Items) != 1 || listed.Data.Items[0].ID != created.Data.ID || listed.Data.Items[0].Deleted {
		t.Fatalf("listed profiles = %#v", listed.Data.Items)
	}
	assertNoModelProfileSecret(t, response.Body.String())
}

func TestPlan009ModelProfileHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := openModelProfileHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()
	service, err := intelligenceapplication.NewModelProfileService(intelligencepostgres.NewRepository(runtime))
	if err != nil {
		t.Fatalf("NewModelProfileService(): %v", err)
	}
	router := newModelProfileRouter(service, httptransport.RoleAdmin)

	create := `{"name":"integration-relevance-review","task_type":"relevance_review","provider":"openai","model_name":"gpt-5.6sol","model_version":"2026-07","credential_ref":"env:OPENAI_API_KEY","timeout_seconds":30,"max_attempts":2,"max_cost":"0.1000","daily_budget":"10.0000","fallback_priority":100,"enabled":true}`
	response := modelProfileRequest(router, stdhttp.MethodPost, "/api/v1/ai/model-profiles", create, "admin")
	if response.Code != stdhttp.StatusCreated {
		t.Fatalf("create relevance-review profile status = %d, want 201: %s", response.Code, response.Body.String())
	}
	var created struct {
		Data ModelProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode relevance-review profile: %v", err)
	}
	if created.Data.TaskType != "relevance_review" || created.Data.EmbeddingDimensions != nil || created.Data.ModelName != "gpt-5.6sol" {
		t.Fatalf("created relevance-review profile = %#v", created.Data)
	}
	assertNoModelProfileSecret(t, response.Body.String())
}

func openModelProfileHTTPRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}
