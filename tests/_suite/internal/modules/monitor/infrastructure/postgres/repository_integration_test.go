package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRepositoryCreatesAndReadsVersionedDraft(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := monitorpostgres.NewRepository(runtime)
	monitor := domain.Monitor{Name: "repository monitor", Description: "draft record", Status: domain.MonitorStatusDraft}
	config := domain.MonitorConfigVersion{Revision: 1, State: domain.ConfigVersionDraft, Config: repositoryConfig()}
	rules := []domain.MonitorRule{{RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "repository", Weight: 100, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}}
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		return repository.Create(ctx, &monitor, &config, rules, nil)
	}); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if monitor.ID == 0 || monitor.DraftConfigVersionID == nil || config.ID == 0 || config.Version != 1 || rules[0].ID == 0 {
		t.Fatalf("created facts = monitor %#v config %#v rule %#v", monitor, config, rules[0])
	}
	loaded, err := repository.FindByID(context.Background(), monitor.ID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}
	if loaded.DraftConfigVersionID == nil || *loaded.DraftConfigVersionID != config.ID || loaded.Version != 1 {
		t.Fatalf("loaded monitor=%#v", loaded)
	}
	loadedConfig, loadedRules, loadedSources, err := repository.FindConfig(context.Background(), config.ID)
	if err != nil {
		t.Fatalf("FindConfig(): %v", err)
	}
	if loadedConfig.MonitorID != monitor.ID || len(loadedRules) != 1 || len(loadedSources) != 0 || loadedRules[0].Value != "repository" {
		t.Fatalf("loaded config/rules/sources = %#v %#v %#v", loadedConfig, loadedRules, loadedSources)
	}
}

func monitorRepositoryRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("InitializeEmpty(): %v", err)
	}
	return runtime
}
func repositoryConfig() domain.MonitorConfig {
	return domain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}
}
