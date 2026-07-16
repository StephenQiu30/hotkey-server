package postgres_test

import (
	"context"
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
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
	} else if appCode, ok := err.(*sharederrors.AppError); !ok || appCode.Code != sharederrors.CodeConflict {
		t.Fatalf("UpdateProfile(stale) error = %#v, want stable conflict", err)
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

func TestModelProfileRepositoryOrdersOnlyEligibleProfiles(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	first := testEmbeddingProfile()
	first.Name, first.FallbackPriority = "eligible-second", 20
	if err := repository.CreateProfile(context.Background(), &first); err != nil {
		t.Fatalf("CreateProfile(first): %v", err)
	}
	second := testEmbeddingProfile()
	second.Name, second.FallbackPriority = "eligible-first", 10
	if err := repository.CreateProfile(context.Background(), &second); err != nil {
		t.Fatalf("CreateProfile(second): %v", err)
	}
	disabled := testEmbeddingProfile()
	disabled.Name, disabled.Enabled, disabled.FallbackPriority = "disabled-profile", false, 0
	if err := repository.CreateProfile(context.Background(), &disabled); err != nil {
		t.Fatalf("CreateProfile(disabled): %v", err)
	}
	profiles, err := repository.EligibleProfiles(context.Background(), intelligencedomain.TaskTypeEmbedding)
	if err != nil {
		t.Fatalf("EligibleProfiles(): %v", err)
	}
	if len(profiles) != 2 || profiles[0].ID != second.ID || profiles[1].ID != first.ID {
		t.Fatalf("EligibleProfiles() = %#v, want ordered enabled profiles %d,%d", profiles, second.ID, first.ID)
	}
}
