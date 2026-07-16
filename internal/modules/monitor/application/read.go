package application

import (
	"context"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
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

// Get returns a published-safe view to viewers for both active and paused
// Monitors. Editors and administrators additionally receive current draft
// metadata when it exists. No resource ownership policy is inferred here.
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

// List preserves a fixed repository-owned cursor/id ascending order. Viewer
// reads are constrained at the repository to active/paused published facts;
// collaborators receive all shared Monitor metadata with a safe draft view.
func (service *Service) List(ctx context.Context, input ListInput) (MonitorPage, error) {
	if err := requireAuthenticated(input.Subject); err != nil {
		return MonitorPage{}, err
	}
	viewer := input.Subject.Role == identitydomain.RoleViewer
	monitors, nextCursor, err := service.monitors.List(ctx, domain.MonitorListQuery{Cursor: input.Cursor, Limit: input.Limit, PublishedOnly: viewer})
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
	viewer := subject.Role == identitydomain.RoleViewer
	if viewer && (monitor.Status != domain.MonitorStatusActive && monitor.Status != domain.MonitorStatusPaused || monitor.PublishedConfigVersionID == nil) {
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
	if !viewer && monitor.DraftConfigVersionID != nil {
		draft, err := service.configurationView(ctx, *monitor.DraftConfigVersionID)
		if err != nil {
			return MonitorView{}, err
		}
		view.Draft = draft
	}
	return view, nil
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
