package domain_test

import (
	"testing"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
)

func TestRuntimeAlertPolicyRegistersOnlyTheApprovedBoundedRules(t *testing.T) {
	tests := []struct {
		alertID          string
		severity         string
		reasonCode       string
		thresholdCount   int64
		thresholdSeconds int64
	}{
		{"ALERT-DB-UNAVAILABLE", "p0", "database_readiness_unavailable", 3, 0},
		{"ALERT-RIVER-JOB-FAILED", "p1", "river_job_discarded", 1, 0},
		{"ALERT-RIVER-NO-WORKER", "p1", "river_queue_lag_exceeded", 1, 300},
		{"ALERT-SOURCE-AUTH", "p1", "source_authentication_failure_streak", 3, 0},
		{"ALERT-MINIO-WRITE", "p0", "evidence_object_integrity_failed", 1, 0},
		{"ALERT-CODEX-FAILURE", "p1", "agent_failure_streak", 3, 0},
		{"ALERT-VAULT-CONFLICT", "p2", "vault_projection_conflict", 1, 0},
		{"ALERT-BACKUP-FAILED", "p0", "backup_failed_or_recovery_point_stale", 1, 900},
		{"ALERT-SEARCH-BACKLOG", "p2", "search_projection_lag_exceeded", 1, 300},
		{"ALERT-DELIVERY-UNKNOWN", "p1", "notification_delivery_unknown", 1, 0},
	}

	for _, test := range tests {
		t.Run(test.alertID, func(t *testing.T) {
			alert, found := operationsdomain.ApplyRuntimeAlertPolicy(operationsdomain.RuntimeAlert{
				AlertID: test.alertID, AffectedCount: 1,
			})
			if !found || alert.PolicyVersion != operationsdomain.RuntimeAlertPolicyVersion ||
				alert.Severity != test.severity || alert.ReasonCode != test.reasonCode ||
				alert.Owner != operationsdomain.RuntimeAlertOwner || alert.SilenceKey != test.alertID ||
				alert.ThresholdCount != test.thresholdCount || alert.ThresholdSeconds != test.thresholdSeconds ||
				alert.RunbookURL == "" || alert.AffectedCount != 1 {
				t.Fatalf("policy alert = %#v, found = %t", alert, found)
			}
		})
	}

	if alert, found := operationsdomain.ApplyRuntimeAlertPolicy(operationsdomain.RuntimeAlert{AlertID: "ALERT-UNREGISTERED"}); found || alert != (operationsdomain.RuntimeAlert{}) {
		t.Fatalf("unregistered alert = %#v, found = %t", alert, found)
	}
}
