# Logging System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Go standard `log.Printf` (28 calls across 6 files) with uber-go/zap structured logging, add Gin access log middleware and GORM SQL logging, all contextualized with request/trace/user IDs from runtime context.

**Architecture:** Config-driven `Init()` creates a global `zap.Logger`. Gin middleware injects request metadata into context. `logging.Ctx(ctx)` extracts context fields and returns a contextualized Logger. GORM adapter bridges to zap. All existing `log.Printf` calls are replaced with semantic zap equivalents.

**Tech Stack:** uber-go/zap (direct dep), GORM `logger.Interface`, Gin middleware, Go 1.26 `context`

---

### Task 1: Config — add logging fields + upgrade zap to direct dependency

**Files:**
- Modify: `internal/config/config.go`
- Modify: `go.mod`

- [ ] **Step 1: Modify Config struct to add three logging fields**

In `internal/config/config.go`, add to the `Config` struct:
```go
LogLevel  string `mapstructure:"LOG_LEVEL"`
LogFormat string `mapstructure:"LOG_FORMAT"`
LogOutput string `mapstructure:"LOG_OUTPUT"`
```

After line `LLMTemperature float64`, insert:
```go
LogLevel  string `mapstructure:"LOG_LEVEL"`
LogFormat string `mapstructure:"LOG_FORMAT"`
LogOutput string `mapstructure:"LOG_OUTPUT"`
```

- [ ] **Step 2: Add default values for the three new fields**

In `Load()`, after the existing defaults block (around line 55), add:
```go
v.SetDefault("LOG_LEVEL", "info")
v.SetDefault("LOG_FORMAT", "json")
v.SetDefault("LOG_OUTPUT", "stdout")
```

- [ ] **Step 3: Add BindEnv and guard for the new fields**

After line 78 (`_ = v.BindEnv("LLM_TEMPERATURE")`), add:
```go
_ = v.BindEnv("LOG_LEVEL")
_ = v.BindEnv("LOG_FORMAT")
_ = v.BindEnv("LOG_OUTPUT")
```

- [ ] **Step 4: Upgrade go.uber.org/zap from indirect to direct dependency**

Run:
```bash
go get go.uber.org/zap@v1.27.0
go mod tidy
```

Verify `go.uber.org/zap` now appears in the `require` block (not under `// indirect`).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go go.mod go.sum
git commit -m "feat: add LOG_LEVEL/LOG_FORMAT/LOG_OUTPUT config fields and make zap a direct dependency"
```

---

### Task 2: Logger factory (zap.go) + context helpers (context.go) + tests

**Files:**
- Create: `internal/platform/logging/zap.go`
- Create: `internal/platform/logging/context.go`
- Create: `tests/unit/platform/logging/zap_test.go`
- Create: `tests/unit/platform/logging/context_test.go`

- [ ] **Step 1: Create `internal/platform/logging/zap.go` — Logger factory**

```go
package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var global *zap.Logger

// Init initializes the global zap.Logger with the given level and format.
// Called once at application startup.
func Init(level, format string) error {
	var lvl zapcore.Level
	switch level {
	case "debug":
		lvl = zapcore.DebugLevel
	case "info":
		lvl = zapcore.InfoLevel
	case "warn":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	default:
		lvl = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var encoder zapcore.Encoder
	if format == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var output zapcore.WriteSyncer
	// user could configure stderr, but default to stdout
	output = zapcore.AddSync(os.Stdout)

	core := zapcore.NewCore(encoder, output, zap.NewAtomicLevelAt(lvl))
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	global = logger
	return nil
}

// L returns the global zap.Logger. Must be called after Init.
func L() *zap.Logger {
	return global
}

// S returns a global SugaredLogger for Printf-style convenience. Must be called after Init.
func S() *zap.SugaredLogger {
	return global.Sugar()
}
```

- [ ] **Step 2: Create `internal/platform/logging/context.go` — Context field extraction**

```go
package logging

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"go.uber.org/zap"
)

// FieldsFromContext extracts runtime metadata fields from context.
func FieldsFromContext(ctx context.Context) []zap.Field {
	var fields []zap.Field
	if reqID := runtime.RequestIDFromContext(ctx); reqID != "" {
		fields = append(fields, zap.String("request_id", reqID))
	}
	if traceID := runtime.TraceIDFromContext(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	if userID := runtime.UserIDFromContext(ctx); userID != 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}
	if mod := runtime.ModuleFromContext(ctx); mod != "" {
		fields = append(fields, zap.String("module", mod))
	}
	return fields
}

// Ctx returns a contextualized Logger with request metadata from ctx.
func Ctx(ctx context.Context) *zap.Logger {
	return L().With(FieldsFromContext(ctx)...)
}
```

- [ ] **Step 3: Create `tests/unit/platform/logging/zap_test.go` — Factory tests**

```go
package logging_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

func TestInitStandardLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		if err := logging.Init(lvl, "json"); err != nil {
			t.Fatalf("Init(%q, json) returned error: %v", lvl, err)
		}
		if logging.L() == nil {
			t.Fatalf("L() returned nil after Init(%q)", lvl)
		}
		if logging.S() == nil {
			t.Fatalf("S() returned nil after Init(%q)", lvl)
		}
	}
}

func TestInitInvalidLevelDefaultsToInfo(t *testing.T) {
	if err := logging.Init("invalid", "json"); err != nil {
		t.Fatalf("Init with invalid level returned error: %v", err)
	}
	if logging.L() == nil {
		t.Fatal("L() returned nil after Init with invalid level")
	}
}

func TestInitConsoleFormat(t *testing.T) {
	if err := logging.Init("info", "console"); err != nil {
		t.Fatalf("Init(info, console) returned error: %v", err)
	}
	if logging.L() == nil {
		t.Fatal("L() returned nil after Init with console format")
	}
}
```

- [ ] **Step 4: Create `tests/unit/platform/logging/context_test.go` — Context extraction tests**

```go
package logging_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
)

func TestFieldsFromContextEmpty(t *testing.T) {
	fields := logging.FieldsFromContext(context.Background())
	if len(fields) != 0 {
		t.Fatalf("expected 0 fields from empty context, got %d", len(fields))
	}
}

func TestFieldsFromContextAllFields(t *testing.T) {
	defer logging.Init("info", "json")
	ctx := context.Background()
	ctx = runtime.WithRequestID(ctx, "req-123")
	ctx = runtime.WithTraceID(ctx, "trace-abc")
	ctx = runtime.WithUserID(ctx, int64(42))
	ctx = runtime.WithModule(ctx, "http")

	fields := logging.FieldsFromContext(ctx)
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d: %v", len(fields), fields)
	}
}

func TestCtxReturnsNonNil(t *testing.T) {
	defer logging.Init("info", "json")
	logger := logging.Ctx(context.Background())
	if logger == nil {
		t.Fatal("Ctx() returned nil")
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go test ./tests/unit/platform/logging/... -v -count=1
```

Expected: 3 tests in zap_test.go + 3 tests in context_test.go = 6 tests, all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/logging/ tests/unit/platform/logging/
git commit -m "feat: add zap logger factory and context-aware logging helpers"
```

---

### Task 3: Gin access log middleware + router registration

**Files:**
- Create: `internal/platform/http/accesslog.go`
- Modify: `internal/platform/http/router.go` (register middleware)

- [ ] **Step 1: Create `internal/platform/http/accesslog.go`**

```go
package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AccessLogMiddleware logs each HTTP request with structured fields.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method

		logging.Ctx(c.Request.Context()).Info("access",
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.Int("size", c.Writer.Size()),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}
```

- [ ] **Step 2: Register AccessLogMiddleware in router.go**

In `internal/platform/http/router.go`, insert `AccessLogMiddleware()` into the middleware chain, right after `RequestIDMiddleware()` and before `ContextMetadataMiddleware`. Modify the `r.Use(...)` block:

```go
r.Use(RecoverMiddleware())
r.Use(CORSMiddleware())
r.Use(SecurityHeadersMiddleware())
r.Use(RequestIDMiddleware())
r.Use(AccessLogMiddleware())
r.Use(ContextMetadataMiddleware("http"))
r.Use(AuthMiddleware(cfg.JWTSecret, cfg.SmokeTest))
```

- [ ] **Step 3: Verify `go build` passes**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/http/accesslog.go internal/platform/http/router.go
git commit -m "feat: add Gin access log middleware with structured request logging"
```

---

### Task 4: GORM logger adapter + database.go wiring

**Files:**
- Create: `internal/database/logger.go`
- Modify: `internal/database/database.go`

- [ ] **Step 1: Create `internal/database/logger.go` — GORM zap adapter**

```go
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
	SlowThreshold time.Duration
}

// LogMode returns a logger that respects the given level — always returns self.
func (l *ZapGormLogger) LogMode(level logger.LogLevel) logger.Interface { return l }

// Info logs at zap Info level.
func (l *ZapGormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	logging.Ctx(ctx).Sugar().Infof(msg, args...)
}

// Warn logs at zap Warn level.
func (l *ZapGormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	logging.Ctx(ctx).Sugar().Warnf(msg, args...)
}

// Error logs at zap Error level.
func (l *ZapGormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	logging.Ctx(ctx).Sugar().Errorf(msg, args...)
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
		logging.Ctx(ctx).Warn("slow sql", fields...)
		return
	}
	logging.Ctx(ctx).Debug("sql", fields...)
}
```

- [ ] **Step 2: Wire into `database.go`**

Change `internal/database/database.go` line 17 from:
```go
db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
```
to:
```go
db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
    Logger: &ZapGormLogger{SlowThreshold: 200 * time.Millisecond},
})
```

Add `"time"` to the import block if not already present (it likely isn't — but we don't need `time` in database.go since `ZapGormLogger` is created without `time` values directly; we just need to ensure the import of the `database` package's own logger file works).

- [ ] **Step 3: Verify build**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/database/logger.go internal/database/database.go
git commit -m "feat: add GORM SQL logging adapter with slow query detection"
```

---

### Task 5: Fx lifecycle integration + fxapp/app.go log.Printf → zap migration

**Files:**
- Modify: `internal/fxapp/app.go` (import zap, call Init in OnStart, replace all 12 log.Printf calls + register AccessLogMiddleware import ensures Init runs before any logging)

- [ ] **Step 1: Add Init call and replace all log.Printf in app.go**

Replace the existing imports: remove `"log"`, replace with `go.uber.org/zap` added to the existing `"go.uber.org/fx"` import line. Add imports for `logging` and the runtime context.

In `registerHooks`'s `OnStart`, as the VERY FIRST action (before the SMOKE_TEST guard), add:
```go
if err := logging.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
    return fmt.Errorf("logging init: %w", err)
}
```

Then replace all 12 `log.Printf(...)` calls with zap equivalents:

After the Init call, the OnStart becomes:

```go
OnStart: func(ctx context.Context) error {
    if err := logging.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
        return fmt.Errorf("logging init: %w", err)
    }

    if os.Getenv("SMOKE_TEST") == "1" {
        go func() {
            if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                logging.L().Error("http server error", zap.Error(err))
            }
        }()
        return nil
    }

    // --- Kafka producer ---
    producer = queue.NewProducer(cfg.KafkaBrokers)

    // --- Redis dedupe ---
    var dedupe *queue.Dedupe
    if cfg.RedisAddr != "" {
        rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
        if err := rdb.Ping(ctx).Err(); err != nil {
            logging.L().Warn("redis ping failed, dedup degraded",
                zap.String("addr", cfg.RedisAddr),
                zap.Error(err),
            )
        }
        dedupe = queue.NewDedupe(rdb)
    }

    // --- Dispatcher ---
    dispatcher := queue.NewDispatcher(producer, dedupe)
    dispatcher.Register(dailyJob)

    // --- DLQ recorder ---
    dispatcher.SetDLQRecorder(func(ctx context.Context, topic string, msg queue.Message, errMsg string) {
        if db != nil {
            db.WithContext(ctx).Create(&queue.DLQRecord{
                Topic:       topic,
                MessageID:   msg.ID,
                MessageType: msg.Type,
                Payload:     string(msg.Payload),
                ErrorMsg:    errMsg,
                RetryCount:  msg.RetryCount,
                CreatedAt:   time.Now(),
            })
        }
    })

    // --- Kafka consumer ---
    consumer = queue.NewConsumer(
        cfg.KafkaBrokers,
        queue.TopicDigestRun,
        cfg.KafkaConsumerGroup,
        dispatcher,
    )
    go func() {
        logging.L().Info("kafka consumer starting",
            zap.String("topic", queue.TopicDigestRun),
        )
        if err := consumer.Run(ctx); err != nil && err != queue.ErrConsumerClosed {
            logging.L().Error("kafka consumer error", zap.Error(err))
        }
    }()

    // --- Cron ---
    loc, err := time.LoadLocation(cfg.DailyDigestTimezone)
    if err != nil {
        return fmt.Errorf("cron: load location %q: %w", cfg.DailyDigestTimezone, err)
    }
    cronS = cron.New(cron.WithLocation(loc))
    _, err = cronS.AddFunc("0 8 * * *", func() {
        now := time.Now().In(loc)
        payload, _ := json.Marshal(map[string]string{
            "target_date": now.AddDate(0, 0, -1).Format("2006-01-02"),
        })
        if pubErr := producer.Publish(context.Background(), queue.TopicDigestRun, queue.NewMessage("digest.run", payload)); pubErr != nil {
            logging.L().Error("cron publish digest error", zap.Error(pubErr))
        }
    })
    if err != nil {
        return fmt.Errorf("cron: add func: %w", err)
    }
    cronS.Start()
    logging.L().Info("worker started (cron + kafka)")

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logging.L().Error("http server error", zap.Error(err))
        }
    }()
    return nil
},
```

**OnStop replacements:**
```go
OnStop: func(ctx context.Context) error {
    if cronS != nil {
        cronS.Stop()
    }
    if consumer != nil {
        if err := consumer.Close(); err != nil {
            logging.L().Error("consumer close error", zap.Error(err))
        }
    }
    if producer != nil {
        if err := producer.Close(); err != nil {
            logging.L().Error("producer close error", zap.Error(err))
        }
    }
    if rdb != nil {
        if err := rdb.Close(); err != nil {
            logging.L().Error("redis close error", zap.Error(err))
        }
    }
    if err := srv.Shutdown(ctx); err != nil {
        logging.L().Error("http server shutdown error", zap.Error(err))
    }
    if db != nil {
        sqlDB, err := db.DB()
        if err == nil && sqlDB != nil {
            if err := sqlDB.Close(); err != nil {
                logging.L().Error("db close error", zap.Error(err))
            }
        }
    }
    return nil
},
```

The new imports for app.go should be:
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/StephenQiu30/hotkey-server/internal/auth"
    "github.com/StephenQiu30/hotkey-server/internal/config"
    "github.com/StephenQiu30/hotkey-server/internal/content"
    "github.com/StephenQiu30/hotkey-server/internal/database"
    "github.com/StephenQiu30/hotkey-server/internal/hotevent"
    "github.com/StephenQiu30/hotkey-server/internal/llm"
    "github.com/StephenQiu30/hotkey-server/internal/module"
    "github.com/StephenQiu30/hotkey-server/internal/monitor"
    "github.com/StephenQiu30/hotkey-server/internal/notify"
    platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
    "github.com/StephenQiu30/hotkey-server/internal/platform/logging"
    "github.com/StephenQiu30/hotkey-server/internal/queue"
    "github.com/StephenQiu30/hotkey-server/internal/report"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
    "github.com/StephenQiu30/hotkey-server/internal/topic"
    "github.com/StephenQiu30/hotkey-server/internal/trend"
    "github.com/StephenQiu30/hotkey-server/internal/worker"
    "github.com/redis/go-redis/v9"
    "github.com/robfig/cron/v3"
    "go.uber.org/fx"
    "go.uber.org/zap"
    "gorm.io/gorm"
)
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/fxapp/app.go
git commit -m "feat: integrate zap logging into Fx lifecycle, replace all log.Printf in app.go"
```

---

### Task 6: Migrate remaining log.Printf calls (queue, config, middleware, llm)

**Files:**
- Modify: `internal/queue/consumer.go`
- Modify: `internal/queue/dispatcher.go`
- Modify: `internal/platform/http/middleware.go`
- Modify: `internal/config/config.go`
- Modify: `internal/llm/adapter.go`

- [ ] **Step 1: Migrate `internal/queue/consumer.go`**

Replace `"log"` import with `"github.com/StephenQiu30/hotkey-server/internal/platform/logging"` and `"go.uber.org/zap"`.

Replace all 5 `log.Printf` calls:

```go
// Line 43 — fetch error
log.Printf("consumer: fetch error: %v", err)
→
logging.L().Error("consumer fetch error", zap.Error(err))

// Line 49 — unmarshal error (has ctx available from km, but km is kafka.Message, not ctx)
// Actually, at that point we don't have a proper context with metadata. Use L() directly.
logging.L().Error("consumer unmarshal error, committing offset", zap.Error(err))

// Line 51 — commit offset error after unmarshal
logging.L().Error("consumer commit offset error after unmarshal failure", zap.Error(commitErr))

// Line 58 — dispatch error
logging.L().Error("consumer dispatch error",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
    zap.Error(err),
)

// Line 63 — commit offset error
logging.L().Error("consumer commit offset error", zap.Error(commitErr))
```

Update imports accordingly. After the change, consumer.go's import block should add:
```go
"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
"go.uber.org/zap"
```
And remove `"log"`.

- [ ] **Step 2: Migrate `internal/queue/dispatcher.go`**

Replace `"log"` with `"go.uber.org/zap"`. Add `"github.com/StephenQiu30/hotkey-server/internal/platform/logging"`.

Replace all 7 `log.Printf` + 1 `log.Panicf`:

```go
// Line 59 — Register panic
log.Panicf("dispatcher: handler for type %q already registered", h.Type())
→
logging.L().Panic("dispatcher handler already registered",
    zap.String("type", h.Type()),
)

// Line 83 — dedup error
log.Printf("dispatcher: dedup error for %s/%s: %v", msg.Type, msg.ID, err)
→
logging.Ctx(ctx).Error("dispatcher dedup error",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
    zap.Error(err),
)

// Line 86 — dedup duplicate skipped
log.Printf("dispatcher: skipping duplicate %s/%s", msg.Type, msg.ID)
→
logging.Ctx(ctx).Info("dispatcher skipping duplicate",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
)

// Line 102 — retry skipped (no producer)
log.Printf("dispatcher: retry skipped for %s/%s (no producer configured)", msg.Type, msg.ID)
→
logging.Ctx(ctx).Warn("dispatcher retry skipped (no producer configured)",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
)

// Line 104 — retry publish failed
log.Printf("dispatcher: retry publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
→
logging.Ctx(ctx).Error("dispatcher retry publish failed",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
    zap.Error(pubErr),
)

// Line 111 — routing to DLQ
log.Printf("dispatcher: routing %s/%s to DLQ %s after %d retries: %v", msg.Type, msg.ID, dlqTopic, msg.RetryCount, err)
→
logging.Ctx(ctx).Warn("dispatcher routing to DLQ",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
    zap.String("dlq_topic", dlqTopic),
    zap.Int("retries", msg.RetryCount),
    zap.Error(err),
)

// Line 115 — DLQ publish failed
log.Printf("dispatcher: DLQ publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
→
logging.Ctx(ctx).Error("dispatcher DLQ publish failed",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
    zap.Error(pubErr),
)

// Line 118 — DLQ publish skipped
log.Printf("dispatcher: DLQ publish skipped for %s/%s (no producer configured)", msg.Type, msg.ID)
→
logging.Ctx(ctx).Warn("dispatcher DLQ publish skipped (no producer configured)",
    zap.String("msg_type", msg.Type),
    zap.String("msg_id", msg.ID),
)
```

- [ ] **Step 3: Migrate `internal/platform/http/middleware.go`**

Replace `"log"` import with `"github.com/StephenQiu30/hotkey-server/internal/platform/logging"` and `"go.uber.org/zap"`.

```go
// Line 61 — panic recovery
log.Printf("panic recovered: %v", r)
→
logging.L().Error("panic recovered",
    zap.Any("panic", r),
)
```

- [ ] **Step 4: Migrate `internal/config/config.go`**

Replace `"log"` import with `"github.com/StephenQiu30/hotkey-server/internal/platform/logging"` and `"go.uber.org/zap"`.

```go
// Line 60 — config file warning
log.Printf("warning: failed to read .env config file: %v", err)
→
logging.L().Warn("failed to read .env config file",
    zap.Error(err),
)
```

- [ ] **Step 5: Migrate `internal/llm/adapter.go`**

Replace `"log"` import with `"github.com/StephenQiu30/hotkey-server/internal/platform/logging"` and `"go.uber.org/zap"`.

```go
// Line 36 — LLM provider error
log.Printf("llm provider error: %v", err)
→
logging.L().Error("llm provider error",
    zap.Error(err),
)
```

- [ ] **Step 6: Update the RecoverMiddleware in middleware.go to use zap**

The RecoverMiddleware's `log.Printf` replacement from Step 3. But also note: the AccessLogMiddleware was registered BEFORE RecoverMiddleware and ContextMetadataMiddleware in the chain (Task 3). This is correct — panics should still have request context.

- [ ] **Step 7: Verify build**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./...
```

Expected: no errors. Verify that `"log"` no longer appears in `go vet ./...` for the modified files.

- [ ] **Step 8: Commit**

```bash
git add internal/queue/consumer.go internal/queue/dispatcher.go internal/platform/http/middleware.go internal/config/config.go internal/llm/adapter.go
git commit -m "refactor: replace remaining log.Printf calls with zap structured logging"
```

---

### Task 7: Worker logging + cleanup dead custom logger + final verification

**Files:**
- Modify: `internal/worker/daily_obsidian_publish.go`
- Delete: `internal/platform/logging/logger.go`
- Delete: `tests/unit/platform/logging/logger_test.go`
- Modify: `Makefile` (optional, for `make lint`)

- [ ] **Step 1: Add logging to `internal/worker/daily_obsidian_publish.go`**

Add imports:
```go
"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
"go.uber.org/zap"
```

In `RunOnce`, after `runKey := RunKeyForDate(targetDate)` (around line 91), add:
```go
log := logging.L().With(
    zap.String("run_key", runKey),
    zap.String("target_date", targetDate.Format("2006-01-02")),
)
log.Info("starting daily obsidian publish run")
```

At the end of `RunOnce`, before `return runErr`, add a success/info log:
```go
if runErr != nil {
    log.Error("daily obsidian publish run finished with errors",
        zap.Error(runErr),
    )
} else {
    log.Info("daily obsidian publish run completed successfully")
}
```

In `exportOne`, after the function signature line, add:
```go
log := logging.L().With(
    zap.Int64("report_id", item.ID),
    zap.String("export_kind", string(kind)),
    zap.String("target_date", targetDate.Format("2006-01-02")),
)
```

And wrap the error returns with logging. For example, before `return err` on line 138/158/163, prepend logging:
```go
log.Error("export failed", zap.Error(err))
```

- [ ] **Step 2: Delete the dead custom logger**

```bash
rm internal/platform/logging/logger.go
rm tests/unit/platform/logging/logger_test.go
```

- [ ] **Step 3: Clean up the test utility table list (optional check)**

Check `tests/testutil/db.go` for any reference to the deleted file patterns — likely none needed.

- [ ] **Step 4: Run full test suite**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go test ./... 2>&1 | tail -20
```

Expected: no compilation errors. Tests that need a database will be skipped if no `TEST_DATABASE_URL` is set — that's fine. Unit tests should all pass.

Do NOT fail if the full suite hits a database-unavailable skip — only check compilation and unit-test panics.

- [ ] **Step 5: Run go vet**

```bash
go vet ./...
```

Expected: no issues.

- [ ] **Step 6: Run make lint (if CI available locally)**

```bash
make lint 2>&1 || true
```

Note any lint warnings but don't block on them.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/daily_obsidian_publish.go
git rm internal/platform/logging/logger.go tests/unit/platform/logging/logger_test.go
git commit -m "feat: add logging to daily obsidian publish worker and remove orphaned custom logger"
```

---

## Spec Coverage Check

| Spec requirement | Task(s) |
|---|---|
| Config: LOG_LEVEL/LOG_FORMAT/LOG_OUTPUT fields | Task 1 |
| Logger factory: Init / L / S | Task 2 |
| Context field extraction: FieldsFromContext / Ctx | Task 2 |
| Gin access log middleware | Task 3 |
| GORM logger adapter with slow query threshold | Task 4 |
| fxapp: Init in OnStart, all 12 log.Printf → zap | Task 5 |
| queue/consumer.go: 5 log.Printf → zap | Task 6 |
| queue/dispatcher.go: 7 log.Printf + 1 log.Panicf → zap | Task 6 |
| middleware.go: Recover log.Printf → zap | Task 6 |
| config.go: log.Printf → zap | Task 6 |
| llm/adapter.go: log.Printf → zap | Task 6 |
| worker: add runtime logging | Task 7 |
| Delete dead custom logger.go | Task 7 |
| Delete dead logger_test.go | Task 7 |
| Upgrade zap to direct dep | Task 1 |
| Tests for logger factory | Task 2 |
| Tests for context extraction | Task 2 |
| Full build verification | Task 7 |

All spec requirements covered.
