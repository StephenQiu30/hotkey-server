// Package postgres contains Monitor-owned persistence adapters. It may read
// Monitor configuration tables, but never SourceConnection tables.
package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
)

type monitorRecord struct {
	ID, Version                                    int64
	Name, Description, Status                      string
	DraftConfigVersionID, PublishedConfigVersionID sql.NullInt64
	CreatedAt, UpdatedAt                           time.Time
	DeletedAt                                      sql.NullTime
}

func (record monitorRecord) monitor() domain.Monitor {
	monitor := domain.Monitor{ID: record.ID, Version: record.Version, Name: record.Name, Description: record.Description,
		Status: domain.MonitorStatus(record.Status), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
	if record.DraftConfigVersionID.Valid {
		value := record.DraftConfigVersionID.Int64
		monitor.DraftConfigVersionID = &value
	}
	if record.PublishedConfigVersionID.Valid {
		value := record.PublishedConfigVersionID.Int64
		monitor.PublishedConfigVersionID = &value
	}
	if record.DeletedAt.Valid {
		value := record.DeletedAt.Time
		monitor.DeletedAt = &value
	}
	return monitor
}

type configRecord struct {
	ID, Version, MonitorID, Revision int64
	State, Timezone, ConfigHash      string
	Languages, Regions               []byte
	Interval                         int
	Relevance, Event                 float64
	Retention                        int
	PublishedAt                      sql.NullTime
	CreatedAt, UpdatedAt             time.Time
}

func (record configRecord) config() (domain.MonitorConfigVersion, error) {
	var languages, regions []string
	if err := json.Unmarshal(record.Languages, &languages); err != nil {
		return domain.MonitorConfigVersion{}, fmt.Errorf("decode monitor languages: %w", err)
	}
	if err := json.Unmarshal(record.Regions, &regions); err != nil {
		return domain.MonitorConfigVersion{}, fmt.Errorf("decode monitor regions: %w", err)
	}
	config := domain.MonitorConfigVersion{ID: record.ID, Version: record.Version, MonitorID: record.MonitorID, Revision: record.Revision,
		State: domain.ConfigVersionState(record.State), Config: domain.MonitorConfig{Timezone: record.Timezone, Languages: languages, Regions: regions,
			CollectionIntervalSeconds: record.Interval, RelevanceThreshold: record.Relevance, EventThreshold: record.Event, RetentionDays: record.Retention},
		ConfigHash: record.ConfigHash, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
	if record.PublishedAt.Valid {
		value := record.PublishedAt.Time
		config.PublishedAt = &value
	}
	return config, nil
}

type ruleRecord struct {
	ID, Version, ConfigVersionID int64
	RuleType, Operator, Value    string
	Weight                       float64
	Priority                     int16
	Origin, Approval             string
	Enabled                      bool
}

func (record ruleRecord) rule() domain.MonitorRule {
	return domain.MonitorRule{ID: record.ID, Version: record.Version, ConfigVersionID: record.ConfigVersionID, RuleType: domain.RuleType(record.RuleType),
		Operator: domain.RuleOperator(record.Operator), Value: record.Value, Weight: record.Weight, Priority: record.Priority,
		Origin: domain.RuleOrigin(record.Origin), ApprovalStatus: domain.RuleApprovalStatus(record.Approval), Enabled: record.Enabled}
}

type sourceRecord struct {
	ID, Version, ConfigVersionID, SourceConnectionID int64
	QueryOverride                                    sql.NullString
	QuerySignature                                   sql.NullString
	Priority                                         int16
	Enabled                                          bool
}

func (record sourceRecord) source() domain.MonitorSource {
	return domain.MonitorSource{ID: record.ID, Version: record.Version, ConfigVersionID: record.ConfigVersionID,
		SourceConnectionID: record.SourceConnectionID, QueryOverride: record.QueryOverride.String, QuerySignature: record.QuerySignature.String,
		Priority: record.Priority, Enabled: record.Enabled}
}
