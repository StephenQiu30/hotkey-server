package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"time"
)

const (
	RetentionRunPendingApproval = "pending_approval"
	RetentionRunApproved        = "approved"
	RetentionRunCompleted       = "completed"
	RetentionRunBlocked         = "blocked"

	RetentionFailureCandidateDrift = "candidate_drift"
	RetentionFailurePolicyDrift    = "policy_drift"
)

type RetentionPolicy struct {
	ID, Version   int64
	DataClass     string
	RetentionDays int
	Action        string
	Enabled       bool
	Description   string
	Protected     bool
}

func (policy RetentionPolicy) Validate() error {
	if policy.ID <= 0 || policy.Version <= 0 || !supportedRetentionDataClass(policy.DataClass) || policy.RetentionDays <= 0 || (policy.Action != "archive" && policy.Action != "delete") {
		return fmt.Errorf("invalid retention policy")
	}
	return nil
}

func supportedRetentionDataClass(value string) bool {
	switch value {
	case "captured_items", "content_metric_snapshots", "event_metric_snapshots", "sessions", "delivery_attempts", "job_attempts", "audit_logs":
		return true
	default:
		return false
	}
}

func ProtectedRetentionDataClass(value string) bool {
	return value == "delivery_attempts" || value == "audit_logs"
}

type RetentionRun struct {
	ID             int64
	PolicyID       int64
	PolicyVersion  int64
	DataClass      string
	Cutoff         time.Time
	BatchSize      int
	CandidateCount int64
	HasMore        bool
	CandidateHash  string
	Status         string
	RequestedBy    int64
	ApprovedBy     int64
	ExecutedBy     int64
	Affected       int64
	FailureCode    string
	CreatedAt      time.Time
	ApprovedAt     time.Time
	ExecutedAt     time.Time
}

func RetentionCandidateHash(policy RetentionPolicy, cutoff time.Time, batchSize int, candidateIDs []int64) (string, error) {
	if err := policy.Validate(); err != nil || cutoff.IsZero() || batchSize < 1 || batchSize > 1000 {
		return "", fmt.Errorf("invalid retention candidate boundary")
	}
	digest := sha256.New()
	writeRetentionHashPart(digest, policy.ID)
	writeRetentionHashPart(digest, policy.Version)
	writeRetentionHashPart(digest, policy.DataClass)
	writeRetentionHashPart(digest, cutoff.UTC().Format(time.RFC3339Nano))
	writeRetentionHashPart(digest, batchSize)
	for _, candidateID := range candidateIDs {
		if candidateID <= 0 {
			return "", fmt.Errorf("invalid retention candidate id")
		}
		writeRetentionHashPart(digest, candidateID)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeRetentionHashPart(digest hash.Hash, value any) {
	_, _ = fmt.Fprintf(digest, "%v\x00", value)
}

type CleanupResult struct {
	RunID             int64     `json:"run_id"`
	PolicyVersion     int64     `json:"policy_version"`
	DataClass         string    `json:"data_class"`
	Cutoff            time.Time `json:"cutoff"`
	Affected          int64     `json:"affected"`
	BatchSize         int       `json:"batch_size"`
	HasMore           bool      `json:"has_more"`
	CandidateHash     string    `json:"candidate_hash"`
	Status            string    `json:"status"`
	RequestedByUserID int64     `json:"requested_by_user_id"`
	ApprovedByUserID  int64     `json:"approved_by_user_id,omitempty"`
	FailureCode       string    `json:"failure_code,omitempty"`
	DryRun            bool      `json:"dry_run"`
}
