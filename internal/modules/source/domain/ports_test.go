package domain

import (
	"context"
	"testing"
	"time"
)

func TestSourcePortsExposeOnlyDomainReadAndWriteContracts(t *testing.T) {
	var _ SourceConnectionRepository = (*sourceConnectionRepositoryFake)(nil)
	var _ MonitorUsageReader = (*monitorUsageReaderFake)(nil)
	var _ MonitorPublishedReferenceReader = (*monitorPublishedReferenceReaderFake)(nil)
	var _ CollectionRepository = (*collectionRepositoryFake)(nil)
	var _ PublishedCollectionTargetReader = (*publishedCollectionTargetReaderFake)(nil)
}

type sourceConnectionRepositoryFake struct{}

func (*sourceConnectionRepositoryFake) Create(context.Context, *SourceConnection) error { return nil }
func (*sourceConnectionRepositoryFake) FindByID(context.Context, int64) (*SourceConnection, error) {
	return nil, nil
}
func (*sourceConnectionRepositoryFake) LockByID(context.Context, int64) (*SourceConnection, error) {
	return nil, nil
}
func (*sourceConnectionRepositoryFake) List(context.Context, SourceConnectionListQuery) ([]SourceConnection, string, error) {
	return nil, "", nil
}
func (*sourceConnectionRepositoryFake) Update(context.Context, *SourceConnection) error { return nil }

type monitorUsageReaderFake struct{}

func (*monitorUsageReaderFake) UsageForSource(context.Context, int64) (SourceUsage, error) {
	return SourceUsage{}, nil
}

type monitorPublishedReferenceReaderFake struct{}

func (*monitorPublishedReferenceReaderFake) HasPublishedReference(context.Context, int64) (bool, error) {
	return false, nil
}

type collectionRepositoryFake struct{}

func (*collectionRepositoryFake) CreateOrReuseRun(context.Context, CollectionRequest) (CollectionRun, bool, error) {
	return CollectionRun{}, false, nil
}

type publishedCollectionTargetReaderFake struct{}

func (*publishedCollectionTargetReaderFake) ListDue(context.Context, time.Time) ([]PublishedCollectionTarget, error) {
	return nil, nil
}
