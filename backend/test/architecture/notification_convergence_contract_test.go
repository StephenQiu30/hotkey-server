package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationConvergenceGateMatchesAC004001(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/Makefile": {
			"notification-convergence-acceptance:",
			"TestProductEventRefreshRiverReplaysMembersMetricsSnapshotsUpdatesAndNotifications",
			"TestEmailDeliveryRepositoryRechecksCurrentChannelPreferenceBeforeClaim",
			"TestUserNotificationRepositoryReplaysByUserRechecksMonitorAccessAndSeparatesDeliveryAttempts",
			"TestWebSocketAuthenticatesInTheFirstFrameReplaysAndRecordsDelivery",
			"notificationStore.test.ts",
			"RealtimeNotifications.test.tsx",
			"dashboard-notifications-page.test.tsx",
		},
		"backend/test/_suite/internal/modules/event/infrastructure/postgres/product_event_refresh_integration_test.go": {
			"cooldown_suppressed",
			"DuplicateCount != 1",
			"assertUserNotificationCount(t, runtime, 2)",
		},
		"backend/test/_suite/internal/modules/notification/infrastructure/postgres/email_delivery_repository_integration_test.go": {
			"TestEmailDeliveryRepositoryRechecksCurrentChannelPreferenceBeforeClaim",
			"disabled current email preference claim",
			"disabled preference attempts",
		},
		"backend/test/_suite/internal/modules/notification/infrastructure/postgres/user_notification_repository_integration_test.go": {
			"replayed outbox projection",
			"replayed read receipt",
			"regressed read receipt",
			"cross-user read receipt",
		},
		"frontend/test/unit/stores/notificationStore.test.ts": {
			"treats local read state as cache and converges to the exact server cursor",
			"allows the authoritative server cursor to replace a forged local advance",
		},
		"frontend/test/unit/components/notifications/RealtimeNotifications.test.tsx": {
			"recovers after cursor N and suppresses a duplicate frame after reconnect",
			"after_id: 12",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing notification convergence evidence %q", relative, fragment)
			}
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/004-通知报告知识投影与检索计划.md")
	for _, fragment := range []string{
		"- [x] `CHK-004-G4-001`",
		"`make notification-convergence-acceptance`",
		"TestEmailDeliveryRepositoryRechecksCurrentChannelPreferenceBeforeClaim",
		"TestUserNotificationRepositoryReplaysByUserRechecksMonitorAccessAndSeparatesDeliveryAttempts",
	} {
		if !strings.Contains(plan, fragment) {
			t.Errorf("plan 004 is missing completed AC-004-001 evidence %q", fragment)
		}
	}
}
