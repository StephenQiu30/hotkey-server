package domain

import (
	"context"
	"testing"
)

func TestMonitorPortsRemainStorageAgnostic(t *testing.T) {
	var _ MonitorRepository = (*monitorRepositoryFake)(nil)
	var _ SourceConnectionReader = (*sourceConnectionReaderFake)(nil)
}

type monitorRepositoryFake struct{}

func (*monitorRepositoryFake) Create(context.Context, *Monitor, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error {
	return nil
}
func (*monitorRepositoryFake) FindByID(context.Context, int64) (*Monitor, error) { return nil, nil }
func (*monitorRepositoryFake) LockByID(context.Context, int64) (*Monitor, error) { return nil, nil }
func (*monitorRepositoryFake) FindConfig(context.Context, int64) (*MonitorConfigVersion, []MonitorRule, []MonitorSource, error) {
	return nil, nil, nil, nil
}
func (*monitorRepositoryFake) LockConfig(context.Context, int64) (*MonitorConfigVersion, []MonitorRule, []MonitorSource, error) {
	return nil, nil, nil, nil
}
func (*monitorRepositoryFake) CreateDraft(context.Context, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error {
	return nil
}
func (*monitorRepositoryFake) SaveDraft(context.Context, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error {
	return nil
}
func (*monitorRepositoryFake) SaveMonitor(context.Context, *Monitor) error { return nil }
func (*monitorRepositoryFake) Publish(context.Context, *Monitor, *MonitorConfigVersion, *MonitorConfigVersion, []MonitorSource) error {
	return nil
}
func (*monitorRepositoryFake) ListActivePublished(context.Context) ([]PublishedMonitor, error) {
	return nil, nil
}

type sourceConnectionReaderFake struct{}

func (*sourceConnectionReaderFake) FindForMonitor(context.Context, int64) (SourceConnectionSummary, error) {
	return SourceConnectionSummary{}, nil
}
