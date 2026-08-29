package domain

import "testing"

func TestRetentionPolicyValidationAllowsOnlyFixedSevenDataClasses(t *testing.T) {
	dataClasses := []string{
		"captured_items",
		"content_metric_snapshots",
		"event_metric_snapshots",
		"sessions",
		"delivery_attempts",
		"job_attempts",
		"audit_logs",
	}
	for index, dataClass := range dataClasses {
		policy := RetentionPolicy{ID: int64(index + 1), Version: 1, DataClass: dataClass, RetentionDays: 30, Action: "delete"}
		if err := policy.Validate(); err != nil {
			t.Errorf("Validate(%s): %v", dataClass, err)
		}
	}
	if err := (RetentionPolicy{ID: 8, Version: 1, DataClass: "arbitrary_extra_class", RetentionDays: 30, Action: "delete"}).Validate(); err == nil {
		t.Fatal("Validate(arbitrary_extra_class) error = nil, want fixed catalog rejection")
	}
	for _, dataClass := range []string{"delivery_attempts", "audit_logs"} {
		if !ProtectedRetentionDataClass(dataClass) {
			t.Errorf("ProtectedRetentionDataClass(%q) = false, want protected durable fact", dataClass)
		}
	}
	if ProtectedRetentionDataClass("content_metric_snapshots") {
		t.Fatal("content_metric_snapshots unexpectedly protected")
	}
}
