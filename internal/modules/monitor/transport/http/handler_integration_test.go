package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/gin-gonic/gin"
)

func TestMonitorDraftHTTPDefaultsPersistAndPublishCanonicalHashesAndSignatures(t *testing.T) {
	runtime := monitorHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorHTTPAdmin(t, runtime)
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage,
		PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime),
	})
	if err != nil {
		t.Fatalf("NewSourceService(): %v", err)
	}
	connection, err := sources.Create(context.Background(), sourceapplication.CreateInput{Subject: admin, Connection: sourcedomain.SourceConnection{
		SourceType: sourcedomain.SourceTypeRSS, Name: "HTTP priority source", Endpoint: "https://feeds.example.test/http-priority",
		AuthType: sourcedomain.AuthTypeNone, Config: sourcedomain.DefaultSourceConfig(), Enabled: true,
	}})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	monitors, err := monitorapplication.NewService(monitorapplication.Dependencies{
		Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: operationspostgres.NewAuditWriter(runtime),
	})
	if err != nil {
		t.Fatalf("NewMonitorService(): %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, monitors, testAuthenticator{subject: httptransport.Subject{UserID: admin.UserID, SessionID: admin.SessionID, Role: httptransport.RoleAdmin}})
	create := fmt.Sprintf(`{"name":"HTTP priority monitor","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI","weight":100}],"sources":[{"source_connection_id":%d}]}`, connection.ID)
	monitorHTTPJSON(t, router, http.MethodPost, "/api/v1/monitors", create, http.StatusCreated, nil)

	repository := monitorpostgres.NewRepository(runtime)
	var monitorID int64
	if err := runtime.SQL.QueryRow(`SELECT id FROM monitors WHERE name = 'HTTP priority monitor'`).Scan(&monitorID); err != nil {
		t.Fatalf("find created monitor: %v", err)
	}
	created, err := repository.FindByID(context.Background(), monitorID)
	if err != nil || created.DraftConfigVersionID == nil {
		t.Fatalf("created monitor = %#v, %v", created, err)
	}
	draft, rules, associations, err := repository.FindConfig(context.Background(), *created.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load omitted-priority draft: %v", err)
	}
	assertMonitorHTTPPriority(t, draft, rules, associations, 100)
	omittedSignature := monitorHTTPPreviewSignature(t, router, monitorID)
	publishedMonitor, publishedConfig := monitorHTTPPublish(t, router, repository, created, draft, omittedSignature)
	assertMonitorHTTPCanonicalHash(t, publishedMonitor.ID, publishedConfig, repository)

	explicitDefault := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":null,"name":"HTTP priority monitor","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI","weight":100,"priority":100}],"sources":[{"source_connection_id":%d,"priority":100}]}`, publishedMonitor.Version, connection.ID)
	monitorHTTPJSON(t, router, http.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/draft", monitorID), explicitDefault, http.StatusOK, nil)
	explicitMonitor, _ := repository.FindByID(context.Background(), monitorID)
	explicitDraft, explicitRules, explicitSources, err := repository.FindConfig(context.Background(), *explicitMonitor.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load explicit-default draft: %v", err)
	}
	assertMonitorHTTPPriority(t, explicitDraft, explicitRules, explicitSources, 100)
	explicitDefaultSignature := monitorHTTPPreviewSignature(t, router, monitorID)
	if explicitDefaultSignature != omittedSignature {
		t.Fatalf("omitted priority signature = %s, explicit 100 signature = %s", omittedSignature, explicitDefaultSignature)
	}
	publishedMonitor, publishedConfig = monitorHTTPPublish(t, router, repository, explicitMonitor, explicitDraft, explicitDefaultSignature)
	assertMonitorHTTPCanonicalHash(t, publishedMonitor.ID, publishedConfig, repository)

	explicitZero := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":null,"name":"HTTP priority monitor","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI","weight":100,"priority":0}],"sources":[{"source_connection_id":%d,"priority":0}]}`, publishedMonitor.Version, connection.ID)
	monitorHTTPJSON(t, router, http.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/draft", monitorID), explicitZero, http.StatusOK, nil)
	zeroMonitor, _ := repository.FindByID(context.Background(), monitorID)
	zeroDraft, zeroRules, zeroSources, err := repository.FindConfig(context.Background(), *zeroMonitor.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load explicit-zero draft: %v", err)
	}
	assertMonitorHTTPPriority(t, zeroDraft, zeroRules, zeroSources, 0)
	zeroSignature := monitorHTTPPreviewSignature(t, router, monitorID)
	if zeroSignature == omittedSignature {
		t.Fatal("explicit zero rule priority did not change query signature")
	}
	publishedMonitor, publishedConfig = monitorHTTPPublish(t, router, repository, zeroMonitor, zeroDraft, zeroSignature)
	assertMonitorHTTPCanonicalHash(t, publishedMonitor.ID, publishedConfig, repository)

	candidateDraftRequest := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":null,"name":"HTTP priority monitor","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI","weight":100,"priority":100}],"sources":[{"source_connection_id":%d,"priority":100}]}`, publishedMonitor.Version, connection.ID)
	monitorHTTPJSON(t, router, http.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/draft", monitorID), candidateDraftRequest, http.StatusOK, nil)
	candidateMonitor, _ := repository.FindByID(context.Background(), monitorID)
	candidateDraft, _, _, err := repository.FindConfig(context.Background(), *candidateMonitor.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load AI candidate draft: %v", err)
	}
	pendingSignature := monitorHTTPPreviewSignature(t, router, monitorID)
	var candidateResult struct {
		Code int                 `json:"code"`
		Data MonitorRuleResponse `json:"data"`
	}
	candidateBody := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":%d,"rule_type":"keyword","operator":"contains","value":"candidate","weight":10}`, candidateMonitor.Version, candidateDraft.Version)
	monitorHTTPJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/draft/ai-candidates", monitorID), candidateBody, http.StatusOK, &candidateResult)
	if candidateResult.Code != 0 || candidateResult.Data.ID <= 0 || candidateResult.Data.Priority != 100 {
		t.Fatalf("omitted-priority AI candidate response = %#v, want persisted default 100", candidateResult)
	}
	pendingMonitor, _ := repository.FindByID(context.Background(), monitorID)
	pendingConfig, pendingRules, _, err := repository.FindConfig(context.Background(), *pendingMonitor.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load pending AI candidate: %v", err)
	}
	if len(pendingRules) != 2 || pendingRules[1].Priority != 100 || pendingRules[1].ApprovalStatus != domain.RuleApprovalPending {
		t.Fatalf("persisted AI candidate rules = %#v, want pending priority 100", pendingRules)
	}
	if signature := monitorHTTPPreviewSignature(t, router, monitorID); signature != pendingSignature {
		t.Fatalf("pending AI candidate changed signature: before=%s after=%s", pendingSignature, signature)
	}
	approvalBody := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":%d,"approval":"approved"}`, pendingMonitor.Version, pendingConfig.Version)
	monitorHTTPJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/draft/rules/%d/approval", monitorID, candidateResult.Data.ID), approvalBody, http.StatusOK, nil)
	approvedMonitor, _ := repository.FindByID(context.Background(), monitorID)
	approvedConfig, approvedRules, _, err := repository.FindConfig(context.Background(), *approvedMonitor.DraftConfigVersionID)
	if err != nil {
		t.Fatalf("load approved AI candidate: %v", err)
	}
	if len(approvedRules) != 2 || approvedRules[1].Priority != 100 || approvedRules[1].ApprovalStatus != domain.RuleApprovalApproved {
		t.Fatalf("approved AI candidate rules = %#v, want approved priority 100", approvedRules)
	}
	approvedSignature := monitorHTTPPreviewSignature(t, router, monitorID)
	if approvedSignature == pendingSignature {
		t.Fatal("approved omitted-priority AI candidate did not change query signature")
	}
	publishedMonitor, publishedConfig = monitorHTTPPublish(t, router, repository, approvedMonitor, approvedConfig, approvedSignature)
	assertMonitorHTTPCanonicalHash(t, publishedMonitor.ID, publishedConfig, repository)
}

func monitorHTTPRuntime(t *testing.T) *database.Runtime {
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

func monitorHTTPAdmin(t *testing.T, runtime *database.Runtime) identitydomain.Subject {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email, password_hash, display_name, role, status) VALUES ($1, 'hash', 'HTTP Admin', 'admin', 'active') RETURNING id`, fmt.Sprintf("monitor-http-%d@example.test", time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return identitydomain.Subject{UserID: id, SessionID: 1, Role: identitydomain.RoleAdmin}
}

func monitorHTTPJSON(t *testing.T, router *gin.Engine, method, path, body string, wantStatus int, target any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %s", method, path, response.Code, wantStatus, response.Body.String())
	}
	if target != nil {
		if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
			t.Fatalf("decode %s %s response: %v", method, path, err)
		}
	}
}

func monitorHTTPPreviewSignature(t *testing.T, router *gin.Engine, monitorID int64) string {
	t.Helper()
	var result struct {
		Code int             `json:"code"`
		Data PreviewResponse `json:"data"`
	}
	monitorHTTPJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/preview", monitorID), "", http.StatusOK, &result)
	if result.Code != 0 || len(result.Data.Sources) != 1 || result.Data.Sources[0].QuerySignature == "" {
		t.Fatalf("preview result = %#v", result)
	}
	return result.Data.Sources[0].QuerySignature
}

func monitorHTTPPublish(t *testing.T, router *gin.Engine, repository *monitorpostgres.Repository, monitor *domain.Monitor, draft *domain.MonitorConfigVersion, wantSignature string) (*domain.Monitor, *domain.MonitorConfigVersion) {
	t.Helper()
	body := fmt.Sprintf(`{"expected_monitor_version":%d,"expected_draft_version":%d}`, monitor.Version, draft.Version)
	monitorHTTPJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/publish", monitor.ID), body, http.StatusOK, nil)
	publishedMonitor, err := repository.FindByID(context.Background(), monitor.ID)
	if err != nil || publishedMonitor.PublishedConfigVersionID == nil {
		t.Fatalf("load published monitor = %#v, %v", publishedMonitor, err)
	}
	publishedConfig, _, sources, err := repository.FindConfig(context.Background(), *publishedMonitor.PublishedConfigVersionID)
	if err != nil {
		t.Fatalf("load published config: %v", err)
	}
	if len(sources) != 1 || sources[0].QuerySignature != wantSignature {
		t.Fatalf("published signatures = %#v, want %s", sources, wantSignature)
	}
	return publishedMonitor, publishedConfig
}

func assertMonitorHTTPPriority(t *testing.T, config *domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource, want int16) {
	t.Helper()
	if config.Config.EventThreshold != 0 || len(rules) != 1 || len(sources) != 1 || rules[0].Priority != want || sources[0].Priority != want {
		t.Fatalf("persisted draft = config %#v rules %#v sources %#v, want event 0 and priorities %d", config.Config, rules, sources, want)
	}
}

func assertMonitorHTTPCanonicalHash(t *testing.T, monitorID int64, config *domain.MonitorConfigVersion, repository *monitorpostgres.Repository) {
	t.Helper()
	persisted, rules, sources, err := repository.FindConfig(context.Background(), config.ID)
	if err != nil {
		t.Fatalf("load config for hash: %v", err)
	}
	want, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: monitorID, Revision: persisted.Revision, Config: persisted.Config, Rules: rules, Sources: sources})
	if err != nil {
		t.Fatalf("CanonicalConfigHash(): %v", err)
	}
	if persisted.ConfigHash == "" || persisted.ConfigHash != want {
		t.Fatalf("persisted config hash = %s, recomputed = %s", persisted.ConfigHash, want)
	}
}
