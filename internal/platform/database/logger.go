package database

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

// ZapGormLogger adapts zap to GORM's logger.Interface.
type ZapGormLogger struct {
	LogLevel      logger.LogLevel
	SlowThreshold time.Duration
}

// LogMode returns a copy of the logger with the requested level.
func (l *ZapGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// Info logs at zap Info level when LogLevel >= Info.
func (l *ZapGormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= logger.Info {
		logging.Ctx(ctx).Sugar().Infof(msg, args...)
	}
}

// Warn logs at zap Warn level when LogLevel >= Warn.
func (l *ZapGormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= logger.Warn {
		logging.Ctx(ctx).Sugar().Warnf(msg, args...)
	}
}

// Error logs at zap Error level when LogLevel >= Error.
func (l *ZapGormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	if l.LogLevel >= logger.Error {
		logging.Ctx(ctx).Sugar().Errorf(msg, args...)
	}
}

// Trace records the SQL execution. On error, logs at Error level.
// When the query exceeds SlowThreshold, logs at Warn level. Otherwise Debug.
func (l *ZapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{
		zap.String("sql", sql),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", elapsed),
	}
	if err != nil {
		logging.Ctx(ctx).Error("sql", append(fields, zap.Error(err))...)
		return
	}
	if l.SlowThreshold > 0 && elapsed > l.SlowThreshold {
		if l.LogLevel >= logger.Warn {
			logging.Ctx(ctx).Warn("slow sql", fields...)
		}
		return
	}
	if l.LogLevel >= logger.Info {
		logging.Ctx(ctx).Debug("sql", fields...)
	}
}
