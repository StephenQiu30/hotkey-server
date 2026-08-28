package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const rejectedMessage = "application event rejected by logging policy"

func New(environment string) (*zap.Logger, error) {
	configuration := zap.NewDevelopmentConfig()
	if environment == "production" {
		configuration = zap.NewProductionConfig()
	}
	configuration.DisableCaller = true
	configuration.DisableStacktrace = true
	logger, err := configuration.Build()
	if err != nil {
		return nil, err
	}
	return logger.WithOptions(zap.WrapCore(newSafeCore)), nil
}

type safeCore struct {
	inner zapcore.Core
}

func newSafeCore(inner zapcore.Core) zapcore.Core {
	return &safeCore{inner: inner}
}

func (core *safeCore) Enabled(level zapcore.Level) bool {
	return core.inner.Enabled(level)
}

func (core *safeCore) With(fields []zapcore.Field) zapcore.Core {
	return &safeCore{inner: core.inner.With(sanitizeFields(fields))}
}

func (core *safeCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}
	return checked
}

func (core *safeCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	entry.Message = safeMessage(entry.Message)
	entry.LoggerName = ""
	entry.Caller = zapcore.EntryCaller{}
	entry.Stack = ""
	return core.inner.Write(entry, sanitizeFields(fields))
}

func (core *safeCore) Sync() error {
	return core.inner.Sync()
}

var allowedMessages = map[string]bool{
	"AI provider configuration rejected":                    true,
	"Agent shadow comparison":                               true,
	"HTTP panic recovered":                                  true,
	"HTTP request completed":                                true,
	"HTTP server started":                                   true,
	"HTTP server stopped unexpectedly":                      true,
	"HTTP server stopping":                                  true,
	"X metric refresh scheduler started":                    true,
	"X metric refresh scheduler stopped":                    true,
	"collection scheduler started":                          true,
	"collection scheduler stopped":                          true,
	"collection source fetch failed":                        true,
	"notification email dispatch failed":                    true,
	"notification email dispatcher started":                 true,
	"notification email dispatcher stopped":                 true,
	"raw evidence archive skipped by current rights policy": true,
	"worker loop stopped":                                   true,
	"worker runtime started":                                true,
	"worker runtime stopped":                                true,
	// Fx emits only these fixed lifecycle messages. Its function names, stack
	// traces and errors are deliberately removed by the field policy below.
	"OnStart hook executing":            true,
	"OnStart hook executed":             true,
	"OnStart hook failed":               true,
	"OnStop hook executing":             true,
	"OnStop hook executed":              true,
	"OnStop hook failed":                true,
	"supplied":                          true,
	"provided":                          true,
	"replaced":                          true,
	"decorated":                         true,
	"before run":                        true,
	"run":                               true,
	"invoking":                          true,
	"received signal":                   true,
	"started":                           true,
	"initialized custom fxevent.Logger": true,
	"error encountered while applying options": true,
	"error encountered while replacing":        true,
	"error returned":                           true,
	"invoke failed":                            true,
	"stop failed":                              true,
	"start failed, rolling back":               true,
	"rollback failed":                          true,
	"start failed":                             true,
	"custom logger initialization failed":      true,
}

var allowedModules = map[string]bool{
	"platform": true, "agentaccess": true, "alert": true, "identity": true,
	"ingestion": true, "intelligence": true, "knowledge": true, "monitor": true,
	"notification": true, "operations": true, "report": true, "search": true, "source": true,
}

var allowedProviders = map[string]bool{"deepseek": true, "ollama": true}
var allowedTaskTypes = map[string]bool{
	"embedding": true, "term_expansion": true, "relevance_review": true,
	"event_cluster": true, "event_summary": true, "entity_claim_extraction": true,
}
var allowedSchemaNames = map[string]bool{
	"term-expansion-output-v1": true, "relevance-review-output-v1": true,
	"event-cluster-output-v1": true, "event-summary-output-v1": true,
	"entity-claim-output-v1": true, "atomic-claim-evidence-output-v2": true,
}
var allowedShadowResults = map[string]bool{
	"dropped": true, "agent_error": true, "invalid": true, "matched": true, "different": true,
}
var allowedErrorKinds = map[string]bool{
	"authentication": true, "rate_limited": true, "temporary": true, "parse": true, "permanent": true,
}

func safeMessage(message string) string {
	if allowedMessages[message] {
		return message
	}
	return rejectedMessage
}

func sanitizeFields(fields []zapcore.Field) []zapcore.Field {
	safe := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		if sanitized, ok := sanitizeField(field); ok {
			safe = append(safe, sanitized)
		}
	}
	return safe
}

func sanitizeField(field zapcore.Field) (zapcore.Field, bool) {
	switch field.Key {
	case "request_id":
		if field.Type == zapcore.StringType && validRequestID(field.String) {
			return zap.String(field.Key, field.String), true
		}
	case "trace_id":
		if field.Type == zapcore.StringType && validTraceID(field.String) {
			return zap.String(field.Key, field.String), true
		}
	case "module":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedModules)), true
		}
	case "method":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, observability.SafeHTTPMethod(field.String)), true
		}
	case "route":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, observability.SafeHTTPRoute(field.String)), true
		}
	case "status":
		if value, ok := boundedInteger(field, 100, 599); ok {
			return zap.Int64(field.Key, value), true
		}
	case "duration", "poll_interval", "interval":
		if field.Type == zapcore.DurationType && field.Integer >= 0 && time.Duration(field.Integer) <= 24*time.Hour {
			return zap.Duration(field.Key, time.Duration(field.Integer)), true
		}
	case "concurrency":
		if value, ok := boundedInteger(field, 1, 128); ok {
			return zap.Int64(field.Key, value), true
		}
	case "source_connection_id", "collection_run_id", "job_id", "run_id", "event_id", "report_id", "knowledge_document_id":
		if value, ok := boundedInteger(field, 1, 1<<62); ok {
			return zap.Int64(field.Key, value), true
		}
	case "provider":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedProviders)), true
		}
	case "task_type":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedTaskTypes)), true
		}
	case "schema_name":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedSchemaNames)), true
		}
	case "schema_version":
		if field.Type == zapcore.StringType && (field.String == "v1" || field.String == "v2") {
			return zap.String(field.Key, field.String), true
		}
	case "result":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedShadowResults)), true
		}
	case "error_kind":
		if field.Type == zapcore.StringType {
			return zap.String(field.Key, boundedString(field.String, allowedErrorKinds)), true
		}
	case "error_code":
		if value, ok := boundedInteger(field, 0, 999999); ok {
			return zap.Int64(field.Key, value), true
		}
	case "duration_ms":
		if value, ok := boundedInteger(field, 0, 30000); ok {
			return zap.Int64(field.Key, value), true
		}
	case "dropped":
		if field.Type == zapcore.BoolType {
			return zap.Bool(field.Key, field.Integer == 1), true
		}
	case "error":
		return zap.String("failure_code", "internal_error"), true
	case "stack":
		if value, ok := fieldBytes(field); ok {
			digest := sha256.Sum256(value)
			return zap.String("stack_sha256", hex.EncodeToString(digest[:])), true
		}
	}
	return zap.Skip(), false
}

func boundedString(value string, allowed map[string]bool) string {
	if allowed[value] {
		return value
	}
	return "unknown"
}

func boundedInteger(field zapcore.Field, minimum, maximum int64) (int64, bool) {
	switch field.Type {
	case zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type:
		return field.Integer, field.Integer >= minimum && field.Integer <= maximum
	default:
		return 0, false
	}
}

func fieldBytes(field zapcore.Field) ([]byte, bool) {
	switch field.Type {
	case zapcore.ByteStringType:
		value, ok := field.Interface.([]byte)
		return value, ok
	case zapcore.StringType:
		return []byte(field.String), true
	default:
		return nil, false
	}
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

func validTraceID(value string) bool {
	if len(value) != 32 || value != strings.ToLower(value) || value == strings.Repeat("0", 32) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
