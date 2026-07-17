package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestRepositoryPersistsDefaultedSourcesAndSafeProjections(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRepository(runtime)
	connection := sourceConnection("repository-defaults")
	connection.CredentialRef = "env:RSS_TOKEN"
	connection.AuthType = domain.AuthTypeBearer
	if err := repository.Create(context.Background(), &connection); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if connection.ID == 0 || connection.Version != 1 {
		t.Fatalf("created source = %#v, want assigned ID and version 1", connection)
	}
	loaded, err := repository.FindByID(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if loaded.Config.ContentRetentionDays != 30 || loaded.Config.MaxPagesPerRun != 1 {
		t.Fatalf("loaded defaults = %#v, want stable defaults", loaded.Config)
	}
	public, err := repository.FindPublicByID(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("FindPublicByID() error = %v", err)
	}
	if !public.CredentialConfigured || public.Name != connection.Name {
		t.Fatalf("public source = %#v, want configured credential bool only", public)
	}
	management, err := repository.FindManagementByID(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("FindManagementByID() error = %v", err)
	}
	if management.Endpoint == "" || management.Config.RequestTimeoutSeconds != 30 {
		t.Fatalf("management source = %#v, want safe endpoint and config", management)
	}
	monitor, err := repository.FindForMonitor(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("FindForMonitor() error = %v", err)
	}
	if monitor.Endpoint == "" || monitor.Config.RateLimitPerMinute != 60 {
		t.Fatalf("monitor source = %#v, want safe execution settings", monitor)
	}
}

func TestRepositorySourceSemanticUpdateIsRejectedAfterPublishedReference(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRepository(runtime)
	connection := sourceConnection("repository-published")
	if err := repository.Create(context.Background(), &connection); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "source-repository-monitor-"+suffix).Scan(&monitorID); err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_sources (config_version_id, source_connection_id) VALUES ($1, $2)`, configID, connection.ID); err != nil {
		t.Fatalf("create monitor source: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("b", 64), configID); err != nil {
		t.Fatalf("publish config: %v", err)
	}
	connection.Endpoint = "https://feeds.example.test/changed"
	if err := repository.Update(ctx, &connection); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("published semantic Update() error = %v, want source trigger constraint", err)
	}
}

func openRuntime(t *testing.T) *database.Runtime {
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

func sourceConnection(name string) domain.SourceConnection {
	return domain.SourceConnection{SourceType: domain.SourceTypeRSS, Name: name, Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown}
}
