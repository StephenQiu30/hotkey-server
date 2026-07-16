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
func (*monitorRepositoryFake) FindConfigVersion(context.Context, int64) (*MonitorConfigVersion, error) {
	return nil, nil
}
func (*monitorRepositoryFake) SaveDraft(context.Context, *Monitor, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error {
	return nil
}

type sourceConnectionReaderFake struct{}

func (*sourceConnectionReaderFake) FindForMonitor(context.Context, int64) (SourceConnectionSummary, error) {
	return SourceConnectionSummary{}, nil
}
