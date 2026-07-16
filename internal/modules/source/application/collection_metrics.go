package application

// CollectionMetrics is deliberately low-cardinality: operation and outcome
// are controlled vocabulary, never source IDs, query signatures, endpoints or
// upstream diagnostic text.
type CollectionMetrics interface {
	RecordCollectionOperation(operation, outcome string)
}

type noopCollectionMetrics struct{}

func (noopCollectionMetrics) RecordCollectionOperation(string, string) {}
