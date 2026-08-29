package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceResourceLimitGateMatchesAC002009(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	requiredProfiles := map[string][]string{
		"backend/internal/modules/source/infrastructure/rss/resource_limits.go": {
			`ResourceLimitProfileVersion = "rss-resource-limits-v1"`,
			"ConnectTimeout", "ReadTimeout", "WallClockTimeout", "MaxPages", "MaxItems",
			"MaxCumulativeResponseBytes", "MaxRetries", "DailyRequestQuota",
		},
		"backend/internal/modules/source/infrastructure/hackernews/resource_limits.go": {
			`ResourceLimitProfileVersion = "hacker-news-resource-limits-v1"`,
			"ConnectTimeout", "ReadTimeout", "WallClockTimeout", "MaxPages", "MaxItems",
			"MaxCumulativeResponseBytes", "MaxRetries", "DailyRequestQuota",
		},
		"backend/internal/modules/source/infrastructure/x/resource_limits.go": {
			`ResourceLimitProfileVersion = "x-api-resource-limits-v1"`,
			"ConnectTimeout", "ReadTimeout", "WallClockTimeout", "MaxPages", "MaxItems",
			"MaxCumulativeResponseBytes", "MaxRetries", "DailyRequestQuota",
		},
	}
	for path, fragments := range requiredProfiles {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-002-009 profile %s is missing %q", path, fragment)
			}
		}
	}

	requiredEvidence := map[string][]string{
		"backend/internal/modules/source/domain/external_request_budget.go": {
			"PerMinuteLimit",
			"RateUsed",
		},
		"backend/db/schema.sql": {
			"rate_window_start",
			"rate_used",
		},
		"backend/internal/modules/source/infrastructure/rss/connector.go": {
			"PerMinuteLimit: connector.perMinuteLimit",
		},
		"backend/internal/modules/source/infrastructure/hackernews/connector.go": {
			"perMinuteLimit: int64(normalized.Config.RateLimitPerMinute)",
		},
		"backend/internal/modules/source/infrastructure/x/connector.go": {
			"perMinuteLimit: int64(normalized.Config.RateLimitPerMinute)",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/rss/resource_limits_test.go": {
			"TestRSSResourceLimitProfileFreezesEightFiniteDimensions",
			"TestRSSResourceLimitsStopBeforeTheNextExternalOrEvidenceSideEffect",
			"TestRSSResourceLimitsBoundConnectReadAndWallClock",
			"limit-1 limit limit+1",
			"daily quota stops before dial",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/hackernews/resource_limits_test.go": {
			"TestHackerNewsResourceLimitProfileFreezesEightFiniteDimensions",
			"TestHackerNewsResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect",
			"TestHackerNewsResourceLimitsBoundConnectReadAndWallClock",
			"limit-1 limit limit+1",
			"daily quota stops before dial",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/x/resource_limits_test.go": {
			"TestXResourceLimitProfileFreezesEightFiniteDimensions",
			"TestXResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect",
			"TestXResourceLimitsBoundConnectReadAndWallClock",
			"limit-1 limit limit+1",
			"daily quota stops before dial and covers lookup",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/postgres/external_request_budget_integration_test.go": {
			"TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically",
			"TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget",
			"next UTC day reservation",
			"next minute reservation",
			"concurrent persisted usage",
		},
		"backend/test/_suite/internal/modules/source/application/collection_service_integration_test.go": {
			"TestCollectionServiceFailureRetainsCursorAndPersistsRetryState",
			"TestCollectionServicePersistsAuthenticationAndPermanentFailures",
			"want retained cursor",
		},
	}
	for path, fragments := range requiredEvidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-002-009 evidence %s is missing %q", path, fragment)
			}
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"source-resource-limit-acceptance:",
		"TestRSSResourceLimitsStopBeforeTheNextExternalOrEvidenceSideEffect",
		"TestHackerNewsResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect",
		"TestXResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect",
		"TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically",
		"TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget",
		"TestCollectionServiceFailureRetainsCursorAndPersistsRetryState",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("source resource acceptance target is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	for _, fragment := range []string{
		"## AC-002-009 资源边界冻结与验收证据",
		"rss-resource-limits-v1",
		"hacker-news-resource-limits-v1",
		"x-api-resource-limits-v1",
		"连接超时",
		"累计响应字节",
		"跨 UTC 日",
		"make source-resource-limit-acceptance",
	} {
		if !strings.Contains(plan, fragment) {
			t.Errorf("AC-002-009 plan evidence is missing %q", fragment)
		}
	}
	for _, checklistID := range []string{"CHK-002-G3-003", "CHK-002-G4-006"} {
		row := markdownChecklistRow(t, plan, checklistID)
		if !strings.HasPrefix(row, "- [x]") {
			t.Errorf("AC-002-009 evidence exists but %s is not complete: %s", checklistID, row)
		}
	}
}
