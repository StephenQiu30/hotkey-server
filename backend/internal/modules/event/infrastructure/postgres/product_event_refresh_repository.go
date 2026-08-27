package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type ProductEventRefreshPostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.ProductEventRefreshRepository = (*ProductEventRefreshPostgresRepository)(nil)
var _ eventapplication.ProductEventRefreshScheduleTargetReader = (*ProductEventRefreshPostgresRepository)(nil)
var _ eventapplication.ProductEventAlertEvaluator = (*ProductEventRefreshPostgresRepository)(nil)

func NewProductEventRefreshPostgresRepository(runtime *database.Runtime) (*ProductEventRefreshPostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("product event refresh database runtime is required")
	}
	return &ProductEventRefreshPostgresRepository{runtime: runtime}, nil
}

func (repository *ProductEventRefreshPostgresRepository) ReadProductEventRefreshScheduleTarget(ctx context.Context,
	query eventapplication.ProductEventRefreshScheduleTargetQuery) (eventapplication.ProductEventRefreshTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.ExpectedEventVersion < 0 {
		return eventapplication.ProductEventRefreshTargetDTO{}, eventapplication.ErrInvalidProductEventRefreshContract
	}
	return repository.readTarget(ctx, query.MicroEventID, query.ExpectedEventVersion, "", "")
}

func (repository *ProductEventRefreshPostgresRepository) ReadProductEventRefreshTarget(ctx context.Context,
	query eventapplication.ProductEventRefreshTargetQuery) (eventapplication.ProductEventRefreshTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.ExpectedEventVersion <= 0 ||
		strings.TrimSpace(query.HeatProfileVersion) == "" || strings.TrimSpace(query.EvidenceStateAlgorithmVersion) == "" {
		return eventapplication.ProductEventRefreshTargetDTO{}, eventapplication.ErrInvalidProductEventRefreshContract
	}
	return repository.readTarget(ctx, query.MicroEventID, query.ExpectedEventVersion,
		query.HeatProfileVersion, query.EvidenceStateAlgorithmVersion)
}

func (repository *ProductEventRefreshPostgresRepository) readTarget(ctx context.Context, eventID, eventVersion int64,
	heatVersion, evidenceVersion string) (eventapplication.ProductEventRefreshTargetDTO, error) {
	var target eventapplication.ProductEventRefreshTargetDTO
	err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT event.id,event.version,heat.id,heat.profile_version,
evidence.id,evidence.algorithm_version
FROM micro_events AS event
CROSS JOIN event_heat_profiles AS heat
CROSS JOIN evidence_state_profiles AS evidence
WHERE event.id=$1 AND ($2=0 OR event.version=$2) AND event.status IN ('active','review_pending')
  AND heat.status='active' AND ($3='' OR heat.profile_version=$3)
  AND evidence.status='active' AND ($4='' OR evidence.algorithm_version=$4)`, eventID, eventVersion,
		heatVersion, evidenceVersion).Scan(&target.MicroEventID, &target.MicroEventVersion, &target.HeatProfileID,
		&target.HeatProfileVersion, &target.EvidenceStateProfileID, &target.EvidenceStateAlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.ProductEventRefreshTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.ProductEventRefreshTargetDTO{}, databaserepository.MapError(err)
	}
	return target, nil
}

type productEventUpdateRecord struct {
	id, version, eventID, eventVersion             int64
	windowEndedAt, createdAt                       time.Time
	windowProfile                                  string
	heatProfileID, evidenceProfileID               int64
	heatProfileVersion, evidenceAlgorithm          string
	heat1ID, heat6ID, heat24ID, evidenceSnapshotID int64
	heatScore                                      float64
	evidenceState                                  string
	independentOrigins                             int
	reasonCodesJSON                                []byte
	refreshKey                                     string
}

func (record productEventUpdateRecord) dto(created bool) (eventapplication.ProductEventUpdateDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonCodesJSON, &reasons); err != nil {
		return eventapplication.ProductEventUpdateDTO{}, fmt.Errorf("decode product event update reasons: %w", err)
	}
	return eventapplication.ProductEventUpdateDTO{ID: record.id, Version: record.version,
		MicroEventID: record.eventID, MicroEventVersion: record.eventVersion, WindowEndedAt: record.windowEndedAt.UTC(),
		WindowProfile: record.windowProfile, HeatProfileID: record.heatProfileID,
		HeatProfileVersion: record.heatProfileVersion, EvidenceStateProfileID: record.evidenceProfileID,
		EvidenceStateAlgorithmVersion: record.evidenceAlgorithm, HeatSnapshot1HourID: record.heat1ID,
		HeatSnapshot6HourID: record.heat6ID, HeatSnapshot24HourID: record.heat24ID,
		EvidenceStateSnapshotID: record.evidenceSnapshotID, HeatScore: record.heatScore,
		EvidenceState: record.evidenceState, IndependentOriginCount: record.independentOrigins,
		ReasonCodes: reasons, RefreshKey: record.refreshKey, CreatedAt: record.createdAt.UTC(), Created: created}, nil
}

func (repository *ProductEventRefreshPostgresRepository) CommitProductEventUpdate(ctx context.Context,
	command eventapplication.CommitProductEventUpdateCommand) (eventapplication.ProductEventUpdateDTO, error) {
	if repository == nil || repository.runtime == nil || command.MicroEventID <= 0 || command.MicroEventVersion <= 0 ||
		command.WindowEndedAt.IsZero() || command.WindowProfile != eventapplication.ProductEventRefreshWindowProfile ||
		command.HeatProfileID <= 0 || command.EvidenceStateProfileID <= 0 || command.HeatSnapshot1HourID <= 0 ||
		command.HeatSnapshot6HourID <= 0 || command.HeatSnapshot24HourID <= 0 || command.EvidenceStateSnapshotID <= 0 ||
		command.HeatScore < 0 || command.HeatScore > 100 || command.IndependentOriginCount < 0 || len(command.ReasonCodes) == 0 ||
		len(command.RefreshKey) != 64 {
		return eventapplication.ProductEventUpdateDTO{}, eventapplication.ErrInvalidProductEventRefreshContract
	}
	reasons, err := json.Marshal(command.ReasonCodes)
	if err != nil {
		return eventapplication.ProductEventUpdateDTO{}, eventapplication.ErrInvalidProductEventRefreshContract
	}
	var result eventapplication.ProductEventUpdateDTO
	err = repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		record, found, readErr := readProductEventUpdateRecord(transactionCtx, transaction.SQL, command.RefreshKey)
		if readErr != nil {
			return readErr
		}
		if found {
			result, readErr = record.dto(false)
			return readErr
		}
		record = productEventUpdateRecord{}
		err := scanProductEventUpdate(transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO micro_event_updates (
micro_event_id,micro_event_version,window_ended_at,window_profile,heat_profile_id,heat_profile_version,
evidence_state_profile_id,evidence_state_algorithm_version,heat_snapshot_1h_id,heat_snapshot_6h_id,
heat_snapshot_24h_id,evidence_state_snapshot_id,heat_score,evidence_state,independent_origin_count,reason_codes,refresh_key,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,$17,$3)
RETURNING id,version,micro_event_id,micro_event_version,window_ended_at,window_profile,heat_profile_id,
heat_profile_version,evidence_state_profile_id,evidence_state_algorithm_version,heat_snapshot_1h_id,
heat_snapshot_6h_id,heat_snapshot_24h_id,evidence_state_snapshot_id,heat_score,evidence_state,
independent_origin_count,reason_codes,refresh_key,created_at`, command.MicroEventID, command.MicroEventVersion,
			command.WindowEndedAt.UTC(), command.WindowProfile, command.HeatProfileID, command.HeatProfileVersion,
			command.EvidenceStateProfileID, command.EvidenceStateAlgorithmVersion, command.HeatSnapshot1HourID,
			command.HeatSnapshot6HourID, command.HeatSnapshot24HourID, command.EvidenceStateSnapshotID,
			command.HeatScore, command.EvidenceState, command.IndependentOriginCount, string(reasons), command.RefreshKey), &record)
		if err != nil {
			return databaserepository.MapError(err)
		}
		result, readErr = record.dto(true)
		return readErr
	})
	if err != nil {
		return eventapplication.ProductEventUpdateDTO{}, databaserepository.MapError(err)
	}
	return result, nil
}

func readProductEventUpdateRecord(ctx context.Context, tx *sql.Tx, refreshKey string) (productEventUpdateRecord, bool, error) {
	var record productEventUpdateRecord
	err := scanProductEventUpdate(tx.QueryRowContext(ctx, `SELECT id,version,micro_event_id,micro_event_version,
window_ended_at,window_profile,heat_profile_id,heat_profile_version,evidence_state_profile_id,
evidence_state_algorithm_version,heat_snapshot_1h_id,heat_snapshot_6h_id,heat_snapshot_24h_id,
evidence_state_snapshot_id,heat_score,evidence_state,independent_origin_count,reason_codes,refresh_key,created_at
FROM micro_event_updates WHERE refresh_key=$1 FOR KEY SHARE`, refreshKey), &record)
	if errors.Is(err, sql.ErrNoRows) {
		return productEventUpdateRecord{}, false, nil
	}
	return record, err == nil, databaserepository.MapError(err)
}

type productEventUpdateScanner interface{ Scan(...any) error }

func scanProductEventUpdate(scanner productEventUpdateScanner, record *productEventUpdateRecord) error {
	return scanner.Scan(&record.id, &record.version, &record.eventID, &record.eventVersion, &record.windowEndedAt,
		&record.windowProfile, &record.heatProfileID, &record.heatProfileVersion, &record.evidenceProfileID,
		&record.evidenceAlgorithm, &record.heat1ID, &record.heat6ID, &record.heat24ID, &record.evidenceSnapshotID,
		&record.heatScore, &record.evidenceState, &record.independentOrigins, &record.reasonCodesJSON,
		&record.refreshKey, &record.createdAt)
}

func (repository *ProductEventRefreshPostgresRepository) EvaluateProductEventUpdate(ctx context.Context,
	update eventapplication.ProductEventUpdateDTO) (eventapplication.ProductEventAlertEvaluationResult, error) {
	if repository == nil || repository.runtime == nil || update.ID <= 0 || update.Version != 1 || update.MicroEventID <= 0 ||
		update.MicroEventVersion <= 0 || len(update.RefreshKey) != 64 || update.WindowEndedAt.IsZero() {
		return eventapplication.ProductEventAlertEvaluationResult{}, eventapplication.ErrInvalidProductEventRefreshContract
	}
	var result eventapplication.ProductEventAlertEvaluationResult
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var eventID, eventVersion int64
		var heatScore float64
		var evidenceState, refreshKey string
		if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT micro_event_id,micro_event_version,heat_score,
evidence_state,refresh_key FROM micro_event_updates WHERE id=$1 AND version=$2 FOR KEY SHARE`, update.ID, update.Version).
			Scan(&eventID, &eventVersion, &heatScore, &evidenceState, &refreshKey); err != nil {
			return databaserepository.MapError(err)
		}
		if eventID != update.MicroEventID || eventVersion != update.MicroEventVersion || heatScore != update.HeatScore ||
			evidenceState != update.EvidenceState || refreshKey != update.RefreshKey {
			return sharedrepository.ErrConflict
		}
		rows, err := transaction.SQL.QueryContext(transactionCtx, `SELECT DISTINCT monitor.id,config.id,
GREATEST(config.event_threshold,config.alert_min_heat)::float8
FROM micro_event_members AS member
JOIN micro_event_membership_decisions AS decision ON decision.id=member.membership_decision_id
JOIN monitors AS monitor ON monitor.id=decision.monitor_id AND monitor.status='active' AND monitor.deleted_at IS NULL
JOIN monitor_config_versions AS config ON config.id=monitor.published_config_version_id AND config.state='published'
JOIN users AS owner ON owner.id=monitor.created_by AND owner.status='active' AND owner.deleted_at IS NULL
WHERE member.micro_event_id=$1 AND member.active
ORDER BY monitor.id,config.id`, update.MicroEventID)
		if err != nil {
			return databaserepository.MapError(err)
		}
		type candidate struct {
			monitorID, configID int64
			threshold           float64
		}
		candidates := []candidate{}
		for rows.Next() {
			var value candidate
			if err := rows.Scan(&value.monitorID, &value.configID, &value.threshold); err != nil {
				rows.Close()
				return databaserepository.MapError(err)
			}
			candidates = append(candidates, value)
		}
		if err := rows.Close(); err != nil {
			return databaserepository.MapError(err)
		}
		if err := rows.Err(); err != nil {
			return databaserepository.MapError(err)
		}
		result.CandidateCount = len(candidates)
		for _, candidate := range candidates {
			eligible := update.HeatScore >= candidate.threshold
			if eligible {
				result.EligibleCount++
			}
			fingerprint := productEventAlertEvaluationKey(update.ID, candidate.configID)
			var existingResult string
			err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT result FROM micro_event_alert_evaluations
WHERE idempotency_key=$1 FOR KEY SHARE`, fingerprint).Scan(&existingResult)
			if err == nil {
				if eligible && existingResult == "outbox_recorded" {
					result.DuplicateCount++
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return databaserepository.MapError(err)
			}
			var notificationID any
			evaluationResult := "below_threshold"
			if eligible {
				evaluationResult = "outbox_recorded"
				dedupeKey := fmt.Sprintf("micro-event-update:%d:monitor:%d", update.ID, candidate.monitorID)
				var outboxID int64
				insertErr := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO notification_outbox_events (
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,$2,$3,$4,'事件更新',$5,$6,$7,$8)
ON CONFLICT (dedupe_key) DO NOTHING RETURNING id`, update.MicroEventID, update.MicroEventVersion,
					candidate.monitorID, update.WindowEndedAt.UTC(), productEventNotificationSummary(update), update.EvidenceState,
					fmt.Sprintf("/dashboard/events?event=%d", update.MicroEventID), dedupeKey).Scan(&outboxID)
				created := insertErr == nil
				if errors.Is(insertErr, sql.ErrNoRows) {
					if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT id FROM notification_outbox_events WHERE dedupe_key=$1`, dedupeKey).Scan(&outboxID); err != nil {
						return databaserepository.MapError(err)
					}
				} else if insertErr != nil {
					return databaserepository.MapError(insertErr)
				}
				notificationID = outboxID
				if created {
					result.NotificationCount++
				} else {
					result.DuplicateCount++
				}
			}
			if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO micro_event_alert_evaluations (
micro_event_update_id,monitor_id,monitor_config_version_id,heat_score,heat_threshold,result,
notification_outbox_event_id,idempotency_key,evaluated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, update.ID, candidate.monitorID, candidate.configID,
				update.HeatScore, candidate.threshold, evaluationResult, notificationID, fingerprint, update.WindowEndedAt.UTC()); err != nil {
				return databaserepository.MapError(err)
			}
		}
		return nil
	})
	if err != nil {
		return eventapplication.ProductEventAlertEvaluationResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func productEventAlertEvaluationKey(updateID, configID int64) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("micro-event-alert:%d:%d", updateID, configID)))
	return hex.EncodeToString(digest[:])
}

func productEventNotificationSummary(update eventapplication.ProductEventUpdateDTO) string {
	return fmt.Sprintf("Heat %.2f，证据状态 %s", update.HeatScore, update.EvidenceState)
}
