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
}

func (policy RetentionPolicy) Validate() error {
	if policy.ID <= 0 || policy.Version <= 0 || strings.TrimSpace(policy.DataClass) == "" || policy.RetentionDays <= 0 || (policy.Action != "archive" && policy.Action != "delete") {
		return fmt.Errorf("invalid retention policy")
	}
	return nil
}

type CleanupResult struct {
	DataClass string
	Cutoff    time.Time
	Affected  int64
}
