package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type AuditEntry struct {
	ActorType    string
	ActorID      int64
	Action       string
	ResourceType string
	ResourceID   int64
	RequestID    string
	TraceID      string
	BeforeData   map[string]any
	AfterData    map[string]any
	Result       string
	IPHash       string
}

type AuditRepository struct {
	runtime *database.Runtime
}

func NewAuditRepository(runtime *database.Runtime) *AuditRepository {
	return &AuditRepository{runtime: runtime}
}

func (repository *AuditRepository) Create(ctx context.Context, entry AuditEntry) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if strings.TrimSpace(entry.ActorType) == "" || strings.TrimSpace(entry.Action) == "" || strings.TrimSpace(entry.ResourceType) == "" || !validAuditResult(entry.Result) {
		return fmt.Errorf("%w: invalid audit entry", sharedrepository.ErrInvalidInput)
	}
	before, err := marshalAuditData(entry.BeforeData)
	if err != nil {
		return err
	}
	after, err := marshalAuditData(entry.AfterData)
	if err != nil {
		return err
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		return createAuditEntry(ctx, transaction.SQL, entry, before, after)
	})
}

func createAuditEntry(ctx context.Context, executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, entry AuditEntry, before, after []byte) error {
	_, err := executor.ExecContext(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, resource_type, resource_id, request_id, trace_id, before_data, after_data, result, ip_hash)
VALUES ($1, NULLIF($2, 0), $3, $4, NULLIF($5, 0), NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, NULLIF($11, ''))`,
		entry.ActorType, entry.ActorID, entry.Action, entry.ResourceType, entry.ResourceID, entry.RequestID, entry.TraceID, before, after, entry.Result, entry.IPHash,
	)
	return mapRepositoryError(err)
}

func validAuditResult(value string) bool {
	return value == "success" || value == "failure" || value == "denied"
}

func marshalAuditData(data map[string]any) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(redactAuditValue(data))
	if err != nil {
		return nil, fmt.Errorf("%w: encode audit data: %v", sharedrepository.ErrInvalidInput, err)
	}
	return encoded, nil
}

func redactAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clean := make(map[string]any, len(typed))
		for key, child := range typed {
			if !allowedAuditKey(key) {
				continue
			}
			clean[key] = redactAuditValue(child)
		}
		return clean
	case []any:
		clean := make([]any, len(typed))
		for index, child := range typed {
			clean[index] = redactAuditValue(child)
		}
		return clean
	default:
		return value
	}
}

func allowedAuditKey(key string) bool {
	if sensitiveAuditKey(key) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "role", "status", "deleted_at", "is_deleted":
		return true
	default:
		return false
	}
}

func sensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{"password", "token", "code", "secret", "credential", "authorization", "cookie", "hash"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
