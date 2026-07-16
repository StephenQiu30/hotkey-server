package application

import (
	"context"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestMonitorReadsRespectPublishedAndDraftVisibility(t *testing.T) {
	publishedID, draftID := int64(10), int64(11)
	active := domain.Monitor{ID: 1, Version: 3, Name: "active", Status: domain.MonitorStatusActive, PublishedConfigVersionID: &publishedID}
	paused := domain.Monitor{ID: 2, Version: 3, Name: "paused", Status: domain.MonitorStatusPaused, PublishedConfigVersionID: &publishedID}
	draft := domain.Monitor{ID: 3, Version: 2, Name: "draft", Status: domain.MonitorStatusDraft, DraftConfigVersionID: &draftID}
	repository := &readRepository{monitors: map[int64]domain.Monitor{1: active, 2: paused, 3: draft}, all: []domain.Monitor{active, paused, draft}, configs: map[int64]readConfiguration{
		publishedID: {config: testReadConfig(publishedID, domain.ConfigVersionPublished), rules: []domain.MonitorRule{testReadRule()}, sources: []domain.MonitorSource{{ID: 5, SourceConnectionID: 9, Enabled: true}}},
		draftID:     {config: testReadConfig(draftID, domain.ConfigVersionDraft), rules: []domain.MonitorRule{testReadRule()}, sources: []domain.MonitorSource{{ID: 6, SourceConnectionID: 9, Enabled: true}}},
	}}
	service, err := NewService(Dependencies{Runtime: &database.Runtime{}, Monitors: repository, Sources: readSourceReader{}, Audit: &previewAudit{}})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}

	viewerPage, err := service.List(context.Background(), ListInput{Subject: identitydomain.Subject{UserID: 1, Role: identitydomain.RoleViewer}})
	if err != nil {
		t.Fatalf("viewer List(): %v", err)
	}
	if !repository.lastQuery.PublishedOnly || len(viewerPage.Items) != 2 || viewerPage.Items[0].Monitor.Status != domain.MonitorStatusActive || viewerPage.Items[1].Monitor.Status != domain.MonitorStatusPaused {
		t.Fatalf("viewer page = %#v query=%#v", viewerPage, repository.lastQuery)
	}
	if viewerPage.Items[0].Draft != nil || viewerPage.Items[0].Published == nil {
		t.Fatalf("viewer visibility = %#v", viewerPage.Items[0])
	}
	if viewerPage.Items[0].Published.Sources[0].SourceName != "RSS" || viewerPage.Items[0].Published.Sources[0].SourceType != "rss" {
		t.Fatalf("safe source projection = %#v", viewerPage.Items[0].Published.Sources[0])
	}

	editorView, err := service.Get(context.Background(), identitydomain.Subject{UserID: 2, Role: identitydomain.RoleEditor}, 3)
	if err != nil {
		t.Fatalf("editor Get(draft): %v", err)
	}
	if editorView.Draft == nil || editorView.Published != nil {
		t.Fatalf("editor draft visibility = %#v", editorView)
	}
	if _, err := service.Get(context.Background(), identitydomain.Subject{UserID: 1, Role: identitydomain.RoleViewer}, 3); err == nil {
		t.Fatal("viewer read a draft-only monitor")
	}
}

type readConfiguration struct {
	config  domain.MonitorConfigVersion
	rules   []domain.MonitorRule
	sources []domain.MonitorSource
}
type readRepository struct {
	*previewRepository
	monitors  map[int64]domain.Monitor
	all       []domain.Monitor
	configs   map[int64]readConfiguration
	lastQuery domain.MonitorListQuery
}

func (repository *readRepository) FindByID(_ context.Context, id int64) (*domain.Monitor, error) {
	monitor := repository.monitors[id]
	return &monitor, nil
}
func (repository *readRepository) FindConfig(_ context.Context, id int64) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	item := repository.configs[id]
	config := item.config
	return &config, append([]domain.MonitorRule(nil), item.rules...), append([]domain.MonitorSource(nil), item.sources...), nil
}
func (repository *readRepository) List(_ context.Context, query domain.MonitorListQuery) ([]domain.Monitor, string, error) {
	repository.lastQuery = query
	result := make([]domain.Monitor, 0, len(repository.all))
	for _, monitor := range repository.all {
		if query.PublishedOnly && (monitor.Status != domain.MonitorStatusActive && monitor.Status != domain.MonitorStatusPaused) {
			continue
		}
		result = append(result, monitor)
	}
	return result, "", nil
}

type readSourceReader struct{}

func (readSourceReader) FindForMonitor(context.Context, int64) (sourcedomain.MonitorSourceConnection, error) {
	return sourcedomain.MonitorSourceConnection{ID: 9, Name: "RSS", SourceType: sourcedomain.SourceTypeRSS}, nil
}
func (readSourceReader) LockForMonitor(context.Context, int64) (sourcedomain.MonitorSourceConnection, error) {
	return sourcedomain.MonitorSourceConnection{}, nil
}
func testReadConfig(id int64, state domain.ConfigVersionState) domain.MonitorConfigVersion {
	return domain.MonitorConfigVersion{ID: id, Version: 1, MonitorID: 1, Revision: 1, State: state, Config: domain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}}
}
func testReadRule() domain.MonitorRule {
	return domain.MonitorRule{ID: 4, RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "monitor", Weight: 100, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}
}
