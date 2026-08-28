package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
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

func TestRepositoryListsConfigurationHistoryNewestRevisionFirst(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := monitorpostgres.NewRepository(runtime)
	monitor := domain.Monitor{Name: "history monitor", Status: domain.MonitorStatusDraft}
	first := domain.MonitorConfigVersion{Revision: 1, State: domain.ConfigVersionDraft, Config: repositoryConfig()}
	rules := []domain.MonitorRule{{RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "history", Weight: 100, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}}
	if err := repository.Create(context.Background(), &monitor, &first, rules, nil); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	publishedAt := time.Now().UTC().Truncate(time.Microsecond)
	monitor.Status, monitor.Version, monitor.DraftConfigVersionID, monitor.PublishedConfigVersionID = domain.MonitorStatusActive, monitor.Version+1, nil, &first.ID
	first.State, first.ConfigHash, first.PublishedAt, first.Version = domain.ConfigVersionPublished, strings.Repeat("a", 64), &publishedAt, first.Version+1
	if err := repository.Publish(context.Background(), &monitor, &first, nil, nil); err != nil {
		t.Fatalf("Publish first config: %v", err)
	}
	second := domain.MonitorConfigVersion{MonitorID: monitor.ID, Revision: 2, State: domain.ConfigVersionDraft, Config: repositoryConfig()}
	if err := repository.CreateDraft(context.Background(), &second, nil, nil); err != nil {
		t.Fatalf("CreateDraft(): %v", err)
	}

	history, err := repository.ListConfigs(context.Background(), monitor.ID)
	if err != nil {
		t.Fatalf("ListConfigs(): %v", err)
	}
	if len(history) != 2 || history[0].ID != second.ID || history[0].Revision != 2 || history[1].ID != first.ID || history[1].Revision != 1 {
		t.Fatalf("history = %#v", history)
	}
}

func TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	codec, err := pagination.NewCodec(strings.Repeat("monitor-list-secret-", 2), time.Minute)
	if err != nil {
		t.Fatalf("pagination.NewCodec(): %v", err)
	}
	repository := monitorpostgres.NewRepositoryWithCursorCodec(runtime, codec)
	createUser := func(label string) int64 {
		t.Helper()
		var userID int64
		if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role,status) VALUES ($1,'not-a-credential',$2,'analyst','active') RETURNING id`,
			fmt.Sprintf("monitor-cursor-%s-%d@example.test", label, time.Now().UnixNano()), "Cursor "+label).Scan(&userID); err != nil {
			t.Fatalf("create user %s: %v", label, err)
		}
		return userID
	}
	ownerID := createUser("owner")
	otherOwnerID := createUser("other")
	create := func(name string, ownerID int64) domain.Monitor {
		t.Helper()
		monitor := domain.Monitor{Name: name, Description: "cursor fixture", Status: domain.MonitorStatusDraft, CreatedByUserID: ownerID}
		config := domain.MonitorConfigVersion{Revision: 1, State: domain.ConfigVersionDraft, Config: repositoryConfig()}
		if err := repository.Create(context.Background(), &monitor, &config, nil, nil); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		return monitor
	}
	firstFixture := create("cursor-first", ownerID)
	secondFixture := create("cursor-second", ownerID)
	thirdFixture := create("cursor-third", ownerID)

	query := domain.MonitorListQuery{Limit: 2, VisibleOwnerUserID: ownerID}
	first, cursor, err := repository.List(context.Background(), query)
	if err != nil || len(first) != 2 || cursor == "" {
		t.Fatalf("List(first) monitors/cursor/error = %#v/%q/%v", first, cursor, err)
	}
	if first[0].ID != firstFixture.ID || first[1].ID != secondFixture.ID || strings.Count(cursor, ".") != 1 {
		t.Fatalf("first page/cursor = %#v/%q", first, cursor)
	}
	concurrent := create("cursor-concurrent", ownerID)

	query.Cursor = cursor
	second, nextCursor, err := repository.List(context.Background(), query)
	if err != nil || len(second) != 1 || second[0].ID != thirdFixture.ID || nextCursor != "" {
		t.Fatalf("List(second) monitors/cursor/error = %#v/%q/%v; concurrent=%d", second, nextCursor, err, concurrent.ID)
	}

	tampered := cursor[:len(cursor)-1] + "A"
	if strings.HasSuffix(cursor, "A") {
		tampered = cursor[:len(cursor)-1] + "B"
	}
	query.Cursor = tampered
	if _, _, err := repository.List(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("tampered cursor error = %v, want invalid input", err)
	}
	query.Cursor = cursor
	query.VisibleOwnerUserID = otherOwnerID
	if _, _, err := repository.List(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-owner cursor error = %v, want invalid input", err)
	}
	query.VisibleOwnerUserID = 0
	query.PublishedOnly = true
	if _, _, err := repository.List(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-visibility cursor error = %v, want invalid input", err)
	}
	if _, _, err := repository.List(context.Background(), domain.MonitorListQuery{Limit: 201}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("oversized page error = %v, want invalid input", err)
	}

	shortCodec, err := pagination.NewCodec(strings.Repeat("short-monitor-secret-", 2), time.Millisecond)
	if err != nil {
		t.Fatalf("pagination.NewCodec(short): %v", err)
	}
	shortRepository := monitorpostgres.NewRepositoryWithCursorCodec(runtime, shortCodec)
	_, expiring, err := shortRepository.List(context.Background(), domain.MonitorListQuery{Limit: 2, VisibleOwnerUserID: ownerID})
	if err != nil || expiring == "" {
		t.Fatalf("List(expiring) cursor/error = %q/%v", expiring, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, _, err := shortRepository.List(context.Background(), domain.MonitorListQuery{
		Limit: 2, VisibleOwnerUserID: ownerID, Cursor: expiring,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v, want invalid input", err)
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
