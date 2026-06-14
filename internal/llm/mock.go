package llm

import "context"

// MockClient is a test-friendly Client that returns a fixed summary.
// Set Err to simulate failures.
type MockClient struct {
	Summary   string
	Err       error
	LastInput TopicSummaryInput
}

// SummarizeTopic records the input and returns the canned Summary/Err.
func (m *MockClient) SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error) {
	m.LastInput = in
	return m.Summary, m.Err
}
