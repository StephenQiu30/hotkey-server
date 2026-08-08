package domain

import (
	"fmt"
	"strings"
	"time"
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
	if policy.ID <= 0 || policy.Version <= 0 || strings.TrimSpace(policy.DataClass) == "" || policy.RetentionDays <= 0 || (policy.Action != "archive" && policy.Action != "delete") {
		return fmt.Errorf("invalid retention policy")
	}
	return nil
}

type CleanupResult struct {
	DataClass string    `json:"data_class"`
	Cutoff    time.Time `json:"cutoff"`
	Affected  int64     `json:"affected"`
	BatchSize int       `json:"batch_size"`
	HasMore   bool      `json:"has_more"`
	DryRun    bool      `json:"dry_run"`
}
