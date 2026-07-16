// Package postgres contains Source-owned persistence adapters. It never reads
// Monitor tables; cross-module lifecycle facts arrive through a domain port.
package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

type sourceConnectionRecord struct {
	ID             int64
	Version        int64
	SourceType     string
	Name           string
	Endpoint       string
	AuthType       string
	CredentialRef  sql.NullString
	Config         []byte
	Enabled        bool
	HealthStatus   string
	TermsPolicyURL sql.NullString
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      sql.NullTime
}

func (record sourceConnectionRecord) sourceConnection() (domain.SourceConnection, error) {
	var raw map[string]any
	if err := json.Unmarshal(record.Config, &raw); err != nil {
		return domain.SourceConnection{}, fmt.Errorf("decode source config: %w", err)
	}
	config, err := domain.NormalizeSourceConfig(raw)
	if err != nil {
		return domain.SourceConnection{}, fmt.Errorf("normalize source config: %w", err)
	}
	return domain.SourceConnection{
		ID:             record.ID,
		Version:        record.Version,
		SourceType:     domain.SourceType(record.SourceType),
		Name:           record.Name,
		Endpoint:       record.Endpoint,
		AuthType:       domain.AuthType(record.AuthType),
		CredentialRef:  record.CredentialRef.String,
		Config:         config,
		Enabled:        record.Enabled,
		HealthStatus:   domain.HealthStatus(record.HealthStatus),
		TermsPolicyURL: record.TermsPolicyURL.String,
		Deleted:        record.DeletedAt.Valid,
	}, nil
}

func sourcePublic(connection domain.SourceConnection) domain.PublicSourceConnection {
	return domain.PublicSourceConnection{
		ID:                   connection.ID,
		Version:              connection.Version,
		Name:                 connection.Name,
		SourceType:           connection.SourceType,
		Enabled:              connection.Enabled,
		HealthStatus:         connection.HealthStatus,
		TermsPolicyURL:       connection.TermsPolicyURL,
		CredentialConfigured: connection.CredentialRef != "",
		Deleted:              connection.Deleted,
	}
}

func sourceManagement(connection domain.SourceConnection) domain.ManagementSourceConnection {
	return domain.ManagementSourceConnection{
		PublicSourceConnection: sourcePublic(connection),
		Endpoint:               connection.Endpoint,
		Config:                 connection.Config,
	}
}

func sourceForMonitor(connection domain.SourceConnection) domain.MonitorSourceConnection {
	return domain.MonitorSourceConnection{
		ID:         connection.ID,
		Version:    connection.Version,
		Name:       connection.Name,
		SourceType: connection.SourceType,
		Endpoint:   connection.Endpoint,
		Config:     connection.Config,
		Enabled:    connection.Enabled,
		Deleted:    connection.Deleted,
	}
}
