package domain

import (
	"strings"
	"testing"
	"time"
)

func TestExternalRequestBudgetContractsRejectUnboundedOrAmbiguousFacts(t *testing.T) {
	now := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)
	valid := ExternalRequestBudgetReservation{SourceConnectionID: 7, ResourceProfileVersion: "rss-resource-limits-v1", DailyLimit: 1440, PerMinuteLimit: 60, At: now}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid reservation: %v", err)
	}
	for _, candidate := range []ExternalRequestBudgetReservation{
		{},
		{SourceConnectionID: 7, ResourceProfileVersion: "latest", DailyLimit: 1, PerMinuteLimit: 1, At: now},
		{SourceConnectionID: 7, ResourceProfileVersion: strings.Repeat("a", 63) + "-v1", DailyLimit: 1, PerMinuteLimit: 1, At: now},
		{SourceConnectionID: 7, ResourceProfileVersion: "rss-resource-limits-v1", DailyLimit: 0, PerMinuteLimit: 1, At: now},
		{SourceConnectionID: 7, ResourceProfileVersion: "rss-resource-limits-v1", DailyLimit: 1, PerMinuteLimit: 0, At: now},
	} {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid reservation accepted: %#v", candidate)
		}
	}
	for _, decision := range []ExternalRequestBudgetDecision{
		{Allowed: true, Used: 1, RateUsed: 1, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: false, Used: 2, RateUsed: 2, ResetAt: now.Add(time.Minute)},
	} {
		if err := decision.Validate(2, 2); err != nil {
			t.Fatalf("valid decision %#v: %v", decision, err)
		}
	}
	for _, decision := range []ExternalRequestBudgetDecision{
		{},
		{Allowed: true, Used: 0, RateUsed: 1, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: true, Used: 1, RateUsed: 0, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: true, Used: 3, RateUsed: 1, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: true, Used: 1, RateUsed: 3, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: false, Used: 1, RateUsed: 1, ResetAt: now.Add(time.Minute)},
	} {
		if err := decision.Validate(2, 2); err == nil {
			t.Fatalf("invalid decision accepted: %#v", decision)
		}
	}
	for _, availability := range []ExternalRequestBudgetAvailability{
		{Allowed: true, Used: 0, RateUsed: 0, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: true, Used: 1, RateUsed: 1, ResetAt: now.Add(time.Minute)},
		{Allowed: false, Used: 1440, RateUsed: 1, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: false, Used: 1, RateUsed: 60, ResetAt: now.Add(time.Minute)},
	} {
		if err := availability.Validate(valid); err != nil {
			t.Fatalf("valid availability %#v: %v", availability, err)
		}
	}
	for _, availability := range []ExternalRequestBudgetAvailability{
		{},
		{Allowed: true, Used: 1440, RateUsed: 1, ResetAt: now.Add(24 * time.Hour)},
		{Allowed: true, Used: 1, RateUsed: 60, ResetAt: now.Add(time.Minute)},
		{Allowed: false, Used: 0, RateUsed: 0, ResetAt: now.Add(time.Minute)},
		{Allowed: false, Used: 2, RateUsed: 2, ResetAt: now.Add(time.Minute)},
	} {
		if err := availability.Validate(valid); err == nil {
			t.Fatalf("invalid availability accepted: %#v", availability)
		}
	}
}
