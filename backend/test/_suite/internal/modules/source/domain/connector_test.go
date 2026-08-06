package domain

import (
	"context"
	"testing"
)

func TestCollectionConnectorExposesOnlySourceItems(t *testing.T) {
	t.Parallel()

	var _ Connector = (*collectionConnectorFake)(nil)
	result, err := (&collectionConnectorFake{}).Fetch(context.Background(), FetchRequest{Limit: 1})
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ExternalID != "fixture-1" || result.NextCursor != "cursor-2" {
		t.Fatalf("FetchResult = %#v, want source-item-only fixture result", result)
	}
}

type collectionConnectorFake struct{}

func (*collectionConnectorFake) Validate(context.Context, SourceConnection) error { return nil }
func (*collectionConnectorFake) Fetch(context.Context, FetchRequest) (FetchResult, error) {
	return FetchResult{Items: []SourceItem{{SourceCode: "rss", ExternalID: "fixture-1", ContentType: "article"}}, NextCursor: "cursor-2"}, nil
}
func (*collectionConnectorFake) Health(context.Context, SourceConnection) HealthResult {
	return HealthResult{Healthy: true}
}
