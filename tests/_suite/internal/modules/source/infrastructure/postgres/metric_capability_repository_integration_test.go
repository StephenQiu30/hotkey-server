//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestMetricCapabilityRepositoryPublishesNewVersionWithoutRewritingHistory(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewMetricCapabilityRepository(runtime)
	ctx := context.Background()

	v1 := metricCapabilityProfile("v1")
	if err := repository.CreateDraft(ctx, &v1); err != nil {
		t.Fatalf("CreateDraft(v1) error = %v", err)
	}
	if err := repository.Publish(ctx, &v1); err != nil {
		t.Fatalf("Publish(v1) error = %v", err)
	}
	if v1.Status != domain.MetricCapabilityPublished || v1.PublishedAt == nil {
		t.Fatalf("published v1 = %#v", v1)
	}
	if _, err := runtime.SQL.Exec(`UPDATE metric_capability_profiles SET credibility_weight = 0.1 WHERE id = $1`, v1.ID); err == nil {
		t.Fatal("published profile accepted configuration rewrite")
	}

	v2 := metricCapabilityProfile("v2")
	if err := repository.CreateDraft(ctx, &v2); err != nil {
		t.Fatalf("CreateDraft(v2) error = %v", err)
	}
	if err := repository.Archive(ctx, &v1); err != nil {
		t.Fatalf("Archive(v1) error = %v", err)
	}
	if err := repository.Publish(ctx, &v2); err != nil {
		t.Fatalf("Publish(v2) error = %v", err)
	}
	published, err := repository.FindPublished(ctx, domain.SourceTypeRSS)
	if err != nil {
		t.Fatalf("FindPublished() error = %v", err)
	}
	if published.ID != v2.ID || published.ProfileVersion != "v2" {
		t.Fatalf("published profile = %#v, want v2", published)
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM metric_capability_profiles WHERE id = $1`, v1.ID); err == nil {
		t.Fatal("archived profile accepted deletion")
	}
}

func TestMetricCapabilityRepositoryReportsMissingPublishedProfile(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	_, err := sourcepostgres.NewMetricCapabilityRepository(runtime).FindPublished(context.Background(), domain.SourceTypeRSS)
	if !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("FindPublished() error = %v, want not found", err)
	}
}

func TestMetricCapabilityRepositoryLocksProfilesInsideCallerTransaction(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewMetricCapabilityRepository(runtime)
	rollback := errors.New("rollback metric capability transaction")
	profile := metricCapabilityProfile("transactional-v1")

	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		if err := repository.CreateDraft(ctx, &profile); err != nil {
			t.Fatalf("CreateDraft() error = %v", err)
		}
		locked, err := repository.LockByID(ctx, profile.ID)
		if err != nil {
			t.Fatalf("LockByID() error = %v", err)
		}
		if locked.ID != profile.ID {
			t.Fatalf("locked profile = %#v, want id %d", locked, profile.ID)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v, want rollback sentinel", err)
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM metric_capability_profiles WHERE profile_version = 'transactional-v1'`).Scan(&count); err != nil {
		t.Fatalf("count rolled back profiles: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled back profile count = %d, want 0", count)
	}
}

func metricCapabilityProfile(version string) domain.MetricCapabilityProfile {
	return domain.MetricCapabilityProfile{
		SourceType:                domain.SourceTypeRSS,
		ProfileVersion:            version,
		SupportsViews:             true,
		SupportsComments:          true,
		IndependenceStrategy:      domain.IndependenceBySourceConnection,
		NormalizationWindowHours:  24,
		CredibilityWeight:         0.8,
		MaxSingleItemContribution: 50,
	}
}
