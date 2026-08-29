package domain

const (
	RuntimeAlertPolicyVersion = "p0-operational-alerts-v1"
	RuntimeAlertOwner         = "hotkey-oncall"
)

type runtimeAlertRule struct {
	severity         string
	reasonCode       string
	runbookURL       string
	thresholdCount   int64
	thresholdSeconds int64
}

var runtimeAlertRules = map[string]runtimeAlertRule{
	"ALERT-DB-UNAVAILABLE": {
		severity: "p0", reasonCode: "database_readiness_unavailable",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#database-readiness-alert-response",
		thresholdCount: 3,
	},
	"ALERT-RIVER-JOB-FAILED": {
		severity: "p1", reasonCode: "river_job_discarded",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#river-alert-response",
		thresholdCount: 1,
	},
	"ALERT-RIVER-NO-WORKER": {
		severity: "p1", reasonCode: "river_queue_lag_exceeded",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#river-alert-response",
		thresholdCount: 1, thresholdSeconds: 300,
	},
	"ALERT-SOURCE-AUTH": {
		severity: "p1", reasonCode: "source_authentication_failure_streak",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#source-auth-alert-response",
		thresholdCount: 3,
	},
	"ALERT-MINIO-WRITE": {
		severity: "p0", reasonCode: "evidence_object_integrity_failed",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#evidence-object-alert-response",
		thresholdCount: 1,
	},
	"ALERT-CODEX-FAILURE": {
		severity: "p1", reasonCode: "agent_failure_streak",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#agent-codex-alert-response",
		thresholdCount: 3,
	},
	"ALERT-VAULT-CONFLICT": {
		severity: "p2", reasonCode: "vault_projection_conflict",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#vault-conflict-alert-response",
		thresholdCount: 1,
	},
	"ALERT-SEARCH-BACKLOG": {
		severity: "p2", reasonCode: "search_projection_lag_exceeded",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#search-backlog-alert-response",
		thresholdCount: 1, thresholdSeconds: 300,
	},
	"ALERT-DELIVERY-UNKNOWN": {
		severity: "p1", reasonCode: "notification_delivery_unknown",
		runbookURL:     "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#delivery-unknown-alert-response",
		thresholdCount: 1,
	},
}

// ApplyRuntimeAlertPolicy decorates only registered, low-cardinality P0 rules.
// Callers provide safe correlation IDs and counts; policy metadata never comes
// from a provider, queue payload, source value, or user-controlled field.
func ApplyRuntimeAlertPolicy(alert RuntimeAlert) (RuntimeAlert, bool) {
	rule, found := runtimeAlertRules[alert.AlertID]
	if !found {
		return RuntimeAlert{}, false
	}
	alert.PolicyVersion = RuntimeAlertPolicyVersion
	alert.Severity = rule.severity
	alert.ReasonCode = rule.reasonCode
	alert.RunbookURL = rule.runbookURL
	alert.Owner = RuntimeAlertOwner
	alert.SilenceKey = alert.AlertID
	alert.ThresholdCount = rule.thresholdCount
	alert.ThresholdSeconds = rule.thresholdSeconds
	return alert, true
}
