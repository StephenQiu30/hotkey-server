package knowledge

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockAuditRecorder implements AuditRecorder for testing.
type mockAuditRecorder struct {
	mu      sync.Mutex
	records []RecordAttemptInput
}

func (m *mockAuditRecorder) RecordAttempt(_ context.Context, in RecordAttemptInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, in)
	return nil
}

func (m *mockAuditRecorder) lastStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return ""
	}
	return m.records[len(m.records)-1].Status
}

// mockSidecarWriter implements SidecarWriter for testing.
type mockSidecarWriter struct {
	mu          sync.Mutex
	manualTags  []string
	conclusion  string
	material    string
	themeRef    string
	failOnApply bool
}

func (m *mockSidecarWriter) SetManualTags(_ context.Context, _ int64, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnApply {
		return errors.New("mock sidecar error")
	}
	m.manualTags = tags
	return nil
}

func (m *mockSidecarWriter) SetAnalystConclusion(_ context.Context, _ int64, conclusion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnApply {
		return errors.New("mock sidecar error")
	}
	m.conclusion = conclusion
	return nil
}

func (m *mockSidecarWriter) SetMaterialStatus(_ context.Context, _ int64, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnApply {
		return errors.New("mock sidecar error")
	}
	m.material = status
	return nil
}

func (m *mockSidecarWriter) SetThemeRef(_ context.Context, _ string, _ int64, themeRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnApply {
		return errors.New("mock sidecar error")
	}
	m.themeRef = themeRef
	return nil
}

func TestService_ApplyChange_Success(t *testing.T) {
	audit := &mockAuditRecorder{}
	sidecar := &mockSidecarWriter{}
	svc := NewService(audit, sidecar)

	err := svc.ApplyChange(context.Background(), WritebackChange{
		ObjectType: "topic",
		ObjectID:   101,
		FieldName:  "manual_tags",
		Value:      []string{"ai监管"},
	}, ConflictInput{CurrentRevision: "rev-1", IncomingRevision: "rev-1"})
	if err != nil {
		t.Fatalf("apply change: %v", err)
	}

	if audit.lastStatus() != "applied" {
		t.Fatalf("expected audit status 'applied', got %q", audit.lastStatus())
	}
}

func TestService_ApplyChange_RejectsMachineField(t *testing.T) {
	audit := &mockAuditRecorder{}
	sidecar := &mockSidecarWriter{}
	svc := NewService(audit, sidecar)

	err := svc.ApplyChange(context.Background(), WritebackChange{
		ObjectType: "topic",
		ObjectID:   101,
		FieldName:  "heat",
		Value:      99.9,
	}, ConflictInput{})
	if !errors.Is(err, ErrFieldNotAllowed) {
		t.Fatalf("expected ErrFieldNotAllowed, got %v", err)
	}
	if audit.lastStatus() != "rejected" {
		t.Fatalf("expected audit status 'rejected', got %q", audit.lastStatus())
	}
}

func TestService_ApplyChange_Conflicted(t *testing.T) {
	audit := &mockAuditRecorder{}
	sidecar := &mockSidecarWriter{}
	svc := NewService(audit, sidecar)

	err := svc.ApplyChange(context.Background(), WritebackChange{
		ObjectType: "topic",
		ObjectID:   101,
		FieldName:  "manual_tags",
		Value:      []string{"ai监管"},
	}, ConflictInput{CurrentRevision: "rev-2", IncomingRevision: "rev-1"})
	if !errors.Is(err, ErrWritebackConflict) {
		t.Fatalf("expected ErrWritebackConflict, got %v", err)
	}
	if audit.lastStatus() != "conflicted" {
		t.Fatalf("expected audit status 'conflicted', got %q", audit.lastStatus())
	}
}

func TestService_ApplyChange_WritesToSidecar(t *testing.T) {
	audit := &mockAuditRecorder{}
	sidecar := &mockSidecarWriter{}
	svc := NewService(audit, sidecar)

	err := svc.ApplyChange(context.Background(), WritebackChange{
		ObjectType: "topic",
		ObjectID:   101,
		FieldName:  "analyst_conclusion",
		Value:      "持续关注监管升级",
	}, ConflictInput{CurrentRevision: "rev-1", IncomingRevision: "rev-1"})
	if err != nil {
		t.Fatalf("apply change: %v", err)
	}

	if sidecar.conclusion != "持续关注监管升级" {
		t.Fatalf("expected '持续关注监管升级', got %q", sidecar.conclusion)
	}
}
