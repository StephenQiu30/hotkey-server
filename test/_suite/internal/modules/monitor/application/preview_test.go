package application

import (
	"context"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestPreviewUsesOnlyReadPortsAndNeverWritesAudit(t *testing.T) {
	draftID := int64(2)
	repository := &previewRepository{monitor: domain.Monitor{ID: 1, Version: 1, Name: "preview", Status: domain.MonitorStatusDraft, DraftConfigVersionID: &draftID}, config: domain.MonitorConfigVersion{ID: draftID, Version: 1, MonitorID: 1, Revision: 1, State: domain.ConfigVersionDraft, Config: domain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}}, rules: []domain.MonitorRule{{ID: 3, RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "preview", Weight: 100, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}}, sources: []domain.MonitorSource{{ID: 4, SourceConnectionID: 5, Priority: 1, Enabled: true}}}
	sources := &previewSourceReader{}
	audit := &previewAudit{}
	service, err := NewService(Dependencies{Runtime: &database.Runtime{}, Monitors: repository, Sources: sources, Audit: audit})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	result, err := service.Preview(context.Background(), identitydomain.Subject{UserID: 1, Role: identitydomain.RoleEditor}, 1)
	if err != nil {
		t.Fatalf("Preview(): %v", err)
	}
	if !result.Eligible || len(result.Sources) != 1 || result.Sources[0].EstimatedRequests != 1 || audit.writes != 0 || repository.writes != 0 || sources.locks != 0 {
		t.Fatalf("preview result or side effects = %#v writes=%d repositoryWrites=%d locks=%d", result, audit.writes, repository.writes, sources.locks)
	}
	if _, err := service.Preview(context.Background(), identitydomain.Subject{UserID: 1, Role: identitydomain.RoleViewer}, 1); err == nil {
		t.Fatal("viewer preview succeeded")
	}
}

type previewRepository struct {
	monitor domain.Monitor
	config  domain.MonitorConfigVersion
	rules   []domain.MonitorRule
	sources []domain.MonitorSource
	writes  int
}

func (repository *previewRepository) Create(context.Context, *domain.Monitor, *domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) FindByID(context.Context, int64) (*domain.Monitor, error) {
	copy := repository.monitor
	return &copy, nil
}
func (repository *previewRepository) LockByID(context.Context, int64) (*domain.Monitor, error) {
	repository.writes++
	return nil, nil
}
func (repository *previewRepository) FindConfig(context.Context, int64) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	config := repository.config
	return &config, append([]domain.MonitorRule(nil), repository.rules...), append([]domain.MonitorSource(nil), repository.sources...), nil
}
func (repository *previewRepository) LockConfig(context.Context, int64) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	repository.writes++
	return nil, nil, nil, nil
}
func (repository *previewRepository) CreateDraft(context.Context, *domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) SaveDraft(context.Context, *domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) SaveMonitor(context.Context, *domain.Monitor) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) SoftDelete(context.Context, *domain.Monitor) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) Publish(context.Context, *domain.Monitor, *domain.MonitorConfigVersion, *domain.MonitorConfigVersion, []domain.MonitorSource) error {
	repository.writes++
	return nil
}
func (repository *previewRepository) List(context.Context, domain.MonitorListQuery) ([]domain.Monitor, string, error) {
	return nil, "", nil
}
func (repository *previewRepository) ListActivePublished(context.Context) ([]domain.PublishedMonitor, error) {
	return nil, nil
}

type previewSourceReader struct{ locks int }

func (reader *previewSourceReader) FindForMonitor(context.Context, int64) (sourcedomain.MonitorSourceConnection, error) {
	return sourcedomain.MonitorSourceConnection{ID: 5, SourceType: sourcedomain.SourceTypeRSS, Endpoint: "https://feeds.example.test/rss", Enabled: true, Config: sourcedomain.DefaultSourceConfig()}, nil
}
func (reader *previewSourceReader) LockForMonitor(context.Context, int64) (sourcedomain.MonitorSourceConnection, error) {
	reader.locks++
	return sourcedomain.MonitorSourceConnection{}, nil
}

type previewAudit struct{ writes int }

var _ operationsapplication.AuditWriter = (*previewAudit)(nil)

func (audit *previewAudit) Write(context.Context, operationsdomain.AuditEntry) error {
	audit.writes++
	return nil
}
