package application

import (
	"context"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

// ConfigurationView is a safe configuration projection for the HTTP boundary.
// Source details come only from Source's existing application read port and
// therefore carry no endpoint, configuration, credential or diagnostics.
type ConfigurationView struct {
	Config  domain.MonitorConfigVersion
	Rules   []domain.MonitorRule
	Sources []MonitorSourceView
}

type MonitorSourceView struct {
	MonitorSource domain.MonitorSource
	SourceName    string
	SourceType    string
}

type MonitorView struct {
	Monitor   domain.Monitor
	Published *ConfigurationView
	Draft     *ConfigurationView
}

type ListInput struct {
	Subject identitydomain.Subject
	Cursor  string
	Limit   int
}

type MonitorPage struct {
	Items      []MonitorView
	NextCursor string
}

type HistoryInput struct {
	Subject   identitydomain.Subject
	MonitorID int64
	Cursor    string
	Limit     int
}

type ConfigurationPage struct {
	Items      []ConfigurationView
	NextCursor string
}

// AuthorizeContribution is the narrow cross-module authorization port used by
// workflows such as manual collection. Analyst access is owner-scoped while
// Editor and Admin retain access to every Monitor.
func (service *Service) AuthorizeContribution(ctx context.Context, subject identitydomain.Subject, id int64) error {
	if err := requireContributor(subject); err != nil {
		return err
	}
	if id <= 0 {
		return domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, id)
	if err != nil {
		return monitorReadError(err)
	}
	return authorizeMonitorContributor(subject, *monitor)
}

// AuthorizeRead is the narrow cross-module authorization port used by
// Monitor-owned read projections such as collection scan history.
func (service *Service) AuthorizeRead(ctx context.Context, subject identitydomain.Subject, id int64) error {
	if err := requireAuthenticated(subject); err != nil {
		return err
	}
	if id <= 0 {
		return domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, id)
	if err != nil {
		return monitorReadError(err)
	}
	if !canReadMonitorDraft(subject, *monitor) &&
		(monitor.Status != domain.MonitorStatusActive && monitor.Status != domain.MonitorStatusPaused || monitor.PublishedConfigVersionID == nil) {
		return domain.MonitorDraftUnavailable()
	}
	return nil
}

// Get returns a published-safe view to viewers for both active and paused
// Monitors. Analysts additionally receive their own current draft; Editors and
// administrators receive every current draft when it exists.
func (service *Service) Get(ctx context.Context, subject identitydomain.Subject, id int64) (MonitorView, error) {
	if err := requireAuthenticated(subject); err != nil {
		return MonitorView{}, err
	}
	if id <= 0 {
		return MonitorView{}, domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, id)
	if err != nil {
		return MonitorView{}, monitorReadError(err)
	}
	return service.monitorView(ctx, subject, *monitor)
}

// History returns newest-first immutable configuration history. Viewers may
// inspect only published/superseded versions of an operational Monitor; draft
// facts remain restricted to editors and administrators.
func (service *Service) History(ctx context.Context, input HistoryInput) (ConfigurationPage, error) {
	if err := requireAuthenticated(input.Subject); err != nil {
		return ConfigurationPage{}, err
	}
	if input.MonitorID <= 0 {
		return ConfigurationPage{}, domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, input.MonitorID)
	if err != nil {
		return ConfigurationPage{}, monitorReadError(err)
	}
	readOnly := !canReadMonitorDraft(input.Subject, *monitor)
	if readOnly && (monitor.Status != domain.MonitorStatusActive && monitor.Status != domain.MonitorStatusPaused || monitor.PublishedConfigVersionID == nil) {
		return ConfigurationPage{}, domain.MonitorDraftUnavailable()
	}
	page, err := service.monitors.ListConfigPage(ctx, domain.MonitorConfigListQuery{
		MonitorID: input.MonitorID, Cursor: input.Cursor, Limit: input.Limit, IncludeDrafts: !readOnly,
	})
	if err != nil {
		return ConfigurationPage{}, monitorReadError(err)
	}
	result := make([]ConfigurationView, 0, len(page.Items))
	for _, config := range page.Items {
		view, err := service.configurationView(ctx, config.ID)
		if err != nil {
			return ConfigurationPage{}, err
		}
		result = append(result, *view)
	}
	return ConfigurationPage{Items: result, NextCursor: page.NextCursor}, nil
}

// List preserves a fixed repository-owned cursor/id ascending order. Viewer
// reads are constrained at the repository to active/paused published facts;
// collaborators receive all shared Monitor metadata with a safe draft view.
func (service *Service) List(ctx context.Context, input ListInput) (MonitorPage, error) {
	if err := requireAuthenticated(input.Subject); err != nil {
		return MonitorPage{}, err
	}
	query := domain.MonitorListQuery{Cursor: input.Cursor, Limit: input.Limit}
	if input.Subject.Role == identitydomain.RoleViewer {
		query.PublishedOnly = true
	} else if input.Subject.Role == identitydomain.RoleAnalyst {
		query.VisibleOwnerUserID = input.Subject.UserID
	}
	monitors, nextCursor, err := service.monitors.List(ctx, query)
	if err != nil {
		return MonitorPage{}, monitorReadError(err)
	}
	items := make([]MonitorView, 0, len(monitors))
	for _, monitor := range monitors {
		view, err := service.monitorView(ctx, input.Subject, monitor)
		if err != nil {
			return MonitorPage{}, err
		}
		items = append(items, view)
	}
	return MonitorPage{Items: items, NextCursor: nextCursor}, nil
}

func (service *Service) monitorView(ctx context.Context, subject identitydomain.Subject, monitor domain.Monitor) (MonitorView, error) {
	readOnly := !canReadMonitorDraft(subject, monitor)
	if readOnly && (monitor.Status != domain.MonitorStatusActive && monitor.Status != domain.MonitorStatusPaused || monitor.PublishedConfigVersionID == nil) {
		return MonitorView{}, domain.MonitorDraftUnavailable()
	}
	view := MonitorView{Monitor: monitor}
	if monitor.PublishedConfigVersionID != nil {
		published, err := service.configurationView(ctx, *monitor.PublishedConfigVersionID)
		if err != nil {
			return MonitorView{}, err
		}
		view.Published = published
	}
	if !readOnly && monitor.DraftConfigVersionID != nil {
		draft, err := service.configurationView(ctx, *monitor.DraftConfigVersionID)
		if err != nil {
			return MonitorView{}, err
		}
		view.Draft = draft
	}
	return view, nil
}

func canReadMonitorDraft(subject identitydomain.Subject, monitor domain.Monitor) bool {
	return subject.Role == identitydomain.RoleEditor || subject.Role == identitydomain.RoleAdmin ||
		subject.Role == identitydomain.RoleAnalyst && monitor.CreatedByUserID == subject.UserID
}

func (service *Service) configurationView(ctx context.Context, id int64) (*ConfigurationView, error) {
	config, rules, sources, err := service.monitors.FindConfig(ctx, id)
	if err != nil {
		return nil, monitorReadError(err)
	}
	view := &ConfigurationView{Config: *config, Rules: rules, Sources: make([]MonitorSourceView, 0, len(sources))}
	for _, source := range sources {
		connection, err := service.sources.FindForMonitor(ctx, source.SourceConnectionID)
		if err != nil {
			return nil, monitorSourceError(err)
		}
		view.Sources = append(view.Sources, MonitorSourceView{MonitorSource: source, SourceName: connection.Name, SourceType: string(connection.SourceType)})
	}
	return view, nil
}
