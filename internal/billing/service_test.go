package billing

import "testing"

func TestTenantUsageCanBeTrackedByMetric(t *testing.T) {
	service := NewService()
	service.AssignPlan("tenant-alpha", Plan{
		ID:   "starter",
		Name: "Starter",
		Quotas: map[string]int{
			MetricCollection: 10,
			MetricRefresh:    5,
			MetricAICall:     3,
		},
	})

	if result := service.RecordUsage(UsageInput{TenantID: "tenant-alpha", Metric: MetricCollection, Amount: 2}); !result.Allowed {
		t.Fatalf("usage should be allowed: %#v", result)
	}
	if result := service.RecordUsage(UsageInput{TenantID: "tenant-alpha", Metric: MetricRefresh, Amount: 1}); !result.Allowed {
		t.Fatalf("refresh should be allowed: %#v", result)
	}

	summary := service.GetUsageSummary("tenant-alpha")
	if summary.TenantID != "tenant-alpha" {
		t.Fatalf("tenant id = %q", summary.TenantID)
	}
	if summary.Usage[MetricCollection] != 2 || summary.Usage[MetricRefresh] != 1 {
		t.Fatalf("usage summary = %#v", summary.Usage)
	}
	if summary.Quotas[MetricAICall] != 3 {
		t.Fatalf("quota summary = %#v", summary.Quotas)
	}
}

func TestPlanQuotaLimitsCollectionRefreshAndAICalls(t *testing.T) {
	service := NewService()
	service.AssignPlan("tenant-alpha", Plan{
		ID:   "starter",
		Name: "Starter",
		Quotas: map[string]int{
			MetricCollection: 1,
			MetricRefresh:    1,
			MetricAICall:     1,
		},
	})

	for _, metric := range []string{MetricCollection, MetricRefresh, MetricAICall} {
		if result := service.RecordUsage(UsageInput{TenantID: "tenant-alpha", Metric: metric, Amount: 1}); !result.Allowed {
			t.Fatalf("%s first usage should be allowed: %#v", metric, result)
		}
		blocked := service.RecordUsage(UsageInput{TenantID: "tenant-alpha", Metric: metric, Amount: 1})
		if blocked.Allowed {
			t.Fatalf("%s should be blocked after quota: %#v", metric, blocked)
		}
		if blocked.Reason != ReasonQuotaExceeded {
			t.Fatalf("%s blocked reason = %q, want %q", metric, blocked.Reason, ReasonQuotaExceeded)
		}
	}
}
