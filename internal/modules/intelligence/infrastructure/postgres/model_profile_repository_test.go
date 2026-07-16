package postgres_test

import (
	"context"
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
)

func TestModelProfileRepositoryUsesOptimisticOperationalUpdatesAndSoftLifecycle(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	profile.TimeoutSeconds = 45
	updated, err := repository.UpdateProfile(context.Background(), profile, profile.Version)
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if updated.Version != 2 || updated.TimeoutSeconds != 45 {
		t.Fatalf("UpdateProfile() = %#v, want version 2 and timeout 45", updated)
	}
	if _, err := repository.UpdateProfile(context.Background(), profile, profile.Version); err == nil {
		t.Fatal("UpdateProfile(stale) error = nil, want optimistic conflict")
	}

	semanticChange := updated
	semanticChange.ModelName = "different-model"
	if _, err := repository.UpdateProfile(context.Background(), semanticChange, updated.Version); err == nil {
		t.Fatal("UpdateProfile(semantic change) error = nil, want rejection")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIModelProfileInvalid {
		t.Fatalf("UpdateProfile(semantic change) code = %d/%t, want 70000", code, ok)
	}

	deleted, err := repository.SoftDeleteProfile(context.Background(), profile.ID, updated.Version)
	if err != nil {
		t.Fatalf("SoftDeleteProfile() error = %v", err)
	}
	if deleted.Version != 3 {
		t.Fatalf("SoftDeleteProfile() version = %d, want 3", deleted.Version)
	}
	if _, err := repository.Claim(context.Background(), testClaim(deleted)); err == nil {
		t.Fatal("Claim(deleted profile) error = nil, want unavailable")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIModelUnavailable {
		t.Fatalf("Claim(deleted profile) code = %d/%t, want 70001", code, ok)
	}
	restored, err := repository.RestoreProfile(context.Background(), profile.ID, deleted.Version)
	if err != nil {
		t.Fatalf("RestoreProfile() error = %v", err)
	}
	if restored.Version != 4 {
		t.Fatalf("RestoreProfile() version = %d, want 4", restored.Version)
	}
}
