package application

import (
	"context"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestMonitorReadsRespectPublishedAndDraftVisibility(t *testing.T) {
	publishedID, draftID := int64(10), int64(11)
	active := domain.Monitor{ID: 1, Version: 3, Name: "active", Status: domain.MonitorStatusActive, PublishedConfigVersionID: &publishedID, CreatedByUserID: 9}
	paused := domain.Monitor{ID: 2, Version: 3, Name: "paused", Status: domain.MonitorStatusPaused, PublishedConfigVersionID: &publishedID, CreatedByUserID: 8}
	draft := domain.Monitor{ID: 3, Version: 2, Name: "own draft", Status: domain.MonitorStatusDraft, DraftConfigVersionID: &draftID, CreatedByUserID: 7}
	otherDraft := domain.Monitor{ID: 4, Version: 2, Name: "other draft", Status: domain.MonitorStatusDraft, DraftConfigVersionID: &draftID, CreatedByUserID: 8}
	repository := &readRepository{monitors: map[int64]domain.Monitor{1: active, 2: paused, 3: draft, 4: otherDraft}, all: []domain.Monitor{active, paused, draft, otherDraft}, configs: map[int64]readConfiguration{
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
	analyst := identitydomain.Subject{UserID: 7, Role: identitydomain.RoleAnalyst}
	analystPage, err := service.List(context.Background(), ListInput{Subject: analyst})
	if err != nil {
		t.Fatalf("analyst List(): %v", err)
	}
	if repository.lastQuery.VisibleOwnerUserID != analyst.UserID || repository.lastQuery.PublishedOnly || len(analystPage.Items) != 3 || analystPage.Items[2].Monitor.ID != draft.ID || analystPage.Items[2].Draft == nil {
		t.Fatalf("analyst page = %#v query=%#v", analystPage, repository.lastQuery)
	}
	if _, err := service.Get(context.Background(), analyst, draft.ID); err != nil {
		t.Fatalf("analyst Get(own draft): %v", err)
	}
	if _, err := service.Get(context.Background(), analyst, otherDraft.ID); err == nil {
		t.Fatal("analyst read another owner's draft")
	}
}

func TestMonitorHistoryReturnsAllVersionsToCollaboratorsAndPublishedOnlyToViewers(t *testing.T) {
	publishedID, draftID := int64(10), int64(11)
	monitor := domain.Monitor{ID: 1, Version: 4, Name: "active", Status: domain.MonitorStatusActive, PublishedConfigVersionID: &publishedID, DraftConfigVersionID: &draftID, CreatedByUserID: 7}
	superseded := testReadConfig(9, domain.ConfigVersionSuperseded)
	superseded.Revision = 1
	published := testReadConfig(publishedID, domain.ConfigVersionPublished)
	published.Revision = 2
	draft := testReadConfig(draftID, domain.ConfigVersionDraft)
	draft.Revision = 3
	repository := &readRepository{
		monitors: map[int64]domain.Monitor{1: monitor},
		history:  map[int64][]domain.MonitorConfigVersion{1: {draft, published, superseded}},
		configs: map[int64]readConfiguration{
			9:           {config: superseded},
			publishedID: {config: published},
			draftID:     {config: draft},
		},
	}
	service, err := NewService(Dependencies{Runtime: &database.Runtime{}, Monitors: repository, Sources: readSourceReader{}, Audit: &previewAudit{}})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}

	editorHistory, err := service.History(context.Background(), identitydomain.Subject{UserID: 2, Role: identitydomain.RoleEditor}, 1)
	if err != nil {
		t.Fatalf("editor History(): %v", err)
	}
	if len(editorHistory) != 3 || editorHistory[0].Config.Revision != 3 || editorHistory[2].Config.State != domain.ConfigVersionSuperseded {
		t.Fatalf("editor history = %#v", editorHistory)
	}
	viewerHistory, err := service.History(context.Background(), identitydomain.Subject{UserID: 1, Role: identitydomain.RoleViewer}, 1)
	if err != nil {
		t.Fatalf("viewer History(): %v", err)
	}
	if len(viewerHistory) != 2 || viewerHistory[0].Config.State != domain.ConfigVersionPublished || viewerHistory[1].Config.State != domain.ConfigVersionSuperseded {
		t.Fatalf("viewer history = %#v", viewerHistory)
	}
	analystHistory, err := service.History(context.Background(), identitydomain.Subject{UserID: 7, Role: identitydomain.RoleAnalyst}, 1)
	if err != nil || len(analystHistory) != 3 {
		t.Fatalf("analyst own history = %#v/%v", analystHistory, err)
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
	history   map[int64][]domain.MonitorConfigVersion
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
		published := (monitor.Status == domain.MonitorStatusActive || monitor.Status == domain.MonitorStatusPaused) && monitor.PublishedConfigVersionID != nil
		if query.PublishedOnly && !published {
			continue
		}
		if query.VisibleOwnerUserID > 0 && !published && monitor.CreatedByUserID != query.VisibleOwnerUserID {
			continue
		}
		result = append(result, monitor)
	}
	return result, "", nil
}
func (repository *readRepository) ListConfigs(_ context.Context, monitorID int64) ([]domain.MonitorConfigVersion, error) {
	return append([]domain.MonitorConfigVersion(nil), repository.history[monitorID]...), nil
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
