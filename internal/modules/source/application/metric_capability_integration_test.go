//go:build integration

package application_test

import (
	"context"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestMetricCapabilityServicePublishesVersionedProfilesAndAudits(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := seedAdmin(t, runtime)
	service, err := sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{
		Runtime: runtime, Profiles: sourcepostgres.NewMetricCapabilityRepository(runtime), SourceContexts: sourcepostgres.NewRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime),
	})
	if err != nil {
		t.Fatalf("NewMetricCapabilityService() error = %v", err)
	}
	ctx := context.Background()
	if _, err := service.CreateDraft(ctx, sourceapplication.CreateMetricCapabilityInput{Subject: identitydomain.Subject{UserID: 2, Role: identitydomain.RoleEditor}, Profile: applicationMetricCapability("v0")}); appCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("editor CreateDraft() code = %d, want forbidden", appCode(err))
	}
	v1, err := service.CreateDraft(ctx, sourceapplication.CreateMetricCapabilityInput{Subject: admin, Profile: applicationMetricCapability("v1")})
	if err != nil {
		t.Fatalf("CreateDraft(v1) error = %v", err)
	}
	publishedV1, err := service.Publish(ctx, sourceapplication.MetricCapabilityLifecycleInput{Subject: admin, ID: v1.ID, ExpectedVersion: v1.Version, ReasonCode: "initial_publish"})
	if err != nil {
		t.Fatalf("Publish(v1) error = %v", err)
	}
	if publishedV1.Status != domain.MetricCapabilityPublished || publishedV1.Version != v1.Version+1 {
		t.Fatalf("published v1 = %#v", publishedV1)
	}
	connection := sourceConnection("metric-capability-context")
	connection.HealthStatus = domain.HealthStatusUnknown
	if err := sourcepostgres.NewRepository(runtime).Create(ctx, &connection); err != nil {
		t.Fatalf("Create(metric source connection) error = %v", err)
	}
	capabilities, err := service.ResolveMetricSourceCapabilities(ctx, []int64{connection.ID})
	if err != nil || len(capabilities) != 1 || capabilities[0].SourceConnectionID != connection.ID || capabilities[0].Profile.ID != publishedV1.ID {
		t.Fatalf("ResolveMetricSourceCapabilities() = %#v/%v, want source context and published v1", capabilities, err)
	}
	v2, err := service.CreateDraft(ctx, sourceapplication.CreateMetricCapabilityInput{Subject: admin, Profile: applicationMetricCapability("v2")})
	if err != nil {
		t.Fatalf("CreateDraft(v2) error = %v", err)
	}
	publishedV2, err := service.Publish(ctx, sourceapplication.MetricCapabilityLifecycleInput{Subject: admin, ID: v2.ID, ExpectedVersion: v2.Version, ReasonCode: "profile_upgrade"})
	if err != nil {
		t.Fatalf("Publish(v2) error = %v", err)
	}
	if publishedV2.Status != domain.MetricCapabilityPublished {
		t.Fatalf("published v2 = %#v", publishedV2)
	}
	profile, err := service.FindPublishedMetricCapability(ctx, domain.SourceTypeRSS)
	if err != nil || profile.ID != publishedV2.ID || profile.ProfileVersion != "v2" {
		t.Fatalf("FindPublishedMetricCapability() = %#v/%v, want v2", profile, err)
	}
	var archived, audited int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM metric_capability_profiles WHERE status = 'archived'`).Scan(&archived); err != nil {
		t.Fatalf("count archived profiles: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE resource_type = 'metric_capability_profile'`).Scan(&audited); err != nil {
		t.Fatalf("count capability audit rows: %v", err)
	}
	if archived != 1 || audited != 5 {
		t.Fatalf("archived/audited = %d/%d, want 1/5", archived, audited)
	}
}

func applicationMetricCapability(version string) domain.MetricCapabilityProfile {
	return domain.MetricCapabilityProfile{
		SourceType: domain.SourceTypeRSS, ProfileVersion: version, SupportsViews: true,
		SupportsLikes: true, IndependenceStrategy: domain.IndependenceBySourceConnection,
		NormalizationWindowHours: 24, CredibilityWeight: 0.8, MaxSingleItemContribution: 50,
	}
}
