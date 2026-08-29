package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestModelProfileListCursorIsSignedExpiringAndSnapshotStable(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	codec, err := pagination.NewCodec(strings.Repeat("model-profile-list-secret-", 2), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := intelligencepostgres.NewRepositoryWithCursorCodec(runtime, codec)
	create := func(name string) intelligencedomain.ModelProfile {
		t.Helper()
		profile := testEmbeddingProfile()
		profile.Name = name
		if err := repository.CreateProfile(context.Background(), &profile); err != nil {
			t.Fatalf("CreateProfile(%s): %v", name, err)
		}
		return profile
	}
	first := create("profile-cursor-first")
	second := create("profile-cursor-second")
	third := create("profile-cursor-third")

	firstPage, err := repository.ListProfilePage(context.Background(), intelligencedomain.ModelProfileListQuery{Limit: 2})
	if err != nil || len(firstPage.Items) != 2 || firstPage.Items[0].ID != first.ID || firstPage.Items[1].ID != second.ID || firstPage.NextCursor == "" || strings.Count(firstPage.NextCursor, ".") != 1 {
		t.Fatalf("ListProfilePage(first) = %#v, %v", firstPage, err)
	}
	concurrent := create("profile-cursor-concurrent")
	secondPage, err := repository.ListProfilePage(context.Background(), intelligencedomain.ModelProfileListQuery{Limit: 2, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != third.ID || secondPage.NextCursor != "" {
		t.Fatalf("ListProfilePage(second) = %#v, %v; concurrent=%d", secondPage, err, concurrent.ID)
	}
	fresh, err := repository.ListProfilePage(context.Background(), intelligencedomain.ModelProfileListQuery{Limit: 10})
	if err != nil || len(fresh.Items) != 4 || fresh.Items[3].ID != concurrent.ID {
		t.Fatalf("ListProfilePage(fresh) = %#v, %v", fresh, err)
	}

	tampered := firstPage.NextCursor[:len(firstPage.NextCursor)-1] + "A"
	if strings.HasSuffix(firstPage.NextCursor, "A") {
		tampered = firstPage.NextCursor[:len(firstPage.NextCursor)-1] + "B"
	}
	for name, query := range map[string]intelligencedomain.ModelProfileListQuery{
		"tampered":  {Limit: 2, Cursor: tampered},
		"oversized": {Limit: 201},
	} {
		if _, err := repository.ListProfilePage(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Errorf("%s query error = %v", name, err)
		}
	}

	shortCodec, err := pagination.NewCodec(strings.Repeat("short-model-profile-secret-", 2), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	expiringRepository := intelligencepostgres.NewRepositoryWithCursorCodec(runtime, shortCodec)
	expiring, err := expiringRepository.ListProfilePage(context.Background(), intelligencedomain.ModelProfileListQuery{Limit: 1})
	if err != nil || expiring.NextCursor == "" {
		t.Fatalf("expiring profile page = %#v, %v", expiring, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := expiringRepository.ListProfilePage(context.Background(), intelligencedomain.ModelProfileListQuery{Limit: 1, Cursor: expiring.NextCursor}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired profile cursor error = %v", err)
	}
}

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

func TestModelProfileRepositoryPersistsEventIntelligenceProfiles(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)

	for _, taskType := range []intelligencedomain.TaskType{
		intelligencedomain.TaskTypeEventCluster,
		intelligencedomain.TaskTypeEventSummary,
		intelligencedomain.TaskTypeEntityClaimExtraction,
	} {
		profile := testEmbeddingProfile()
		profile.Name = string(taskType) + "-profile"
		profile.TaskType = taskType
		profile.ModelName = "gpt-5.6sol"
		profile.EmbeddingDimensions = nil
		if err := repository.CreateProfile(context.Background(), &profile); err != nil {
			t.Fatalf("CreateProfile(%s) error = %v", taskType, err)
		}
		profiles, err := repository.EligibleProfiles(context.Background(), taskType)
		if err != nil {
			t.Fatalf("EligibleProfiles(%s) error = %v", taskType, err)
		}
		if len(profiles) != 1 || profiles[0].ID != profile.ID || profiles[0].TaskType != taskType {
			t.Fatalf("EligibleProfiles(%s) = %#v, want persisted profile %d", taskType, profiles, profile.ID)
		}
	}
}
