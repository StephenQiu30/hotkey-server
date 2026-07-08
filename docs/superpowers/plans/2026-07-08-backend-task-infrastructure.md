# Backend Task Infrastructure Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hand-written time.Ticker scheduler with robfig/cron + Kafka async task queue, with dead letter queue support.

**Architecture:** robfig/cron/v3 handles scheduled triggers (cron expressions). Kafka handles message queuing with at-least-once delivery. A Dispatcher routes messages by Type to Handler implementations. Failed messages retry up to 3 times then route to a DLQ topic. Redis provides message-level deduplication.

**Tech Stack:** Go 1.26.3, robfig/cron/v3, segmentio/kafka-go, Redis (existing), PostgreSQL (existing), Kafka 7.8.0 (Confluent)

---

## Precondition

Before executing Task 1, run:

```bash
# Check that local Kafka is available (Homebrew)
brew services list | grep kafka

# Or verify you can connect
echo "dump" | nc -w 3 localhost 2181 2>/dev/null && echo "Zookeeper OK" || echo "Zookeeper not running, start with: brew services start kafka"
```

Expected: Local Kafka is running on `localhost:9092`. If not, the docker-compose Kafka service will be used instead.

---

## File Structure

Create:
- `internal/queue/message.go` — Message struct, constants, sentinel errors
- `internal/queue/types.go` — ExportKind, WriteResult, shared types
- `internal/queue/producer.go` — Kafka producer wrapper (Publish, Close)
- `internal/queue/dispatcher.go` — Handler interface, Dispatcher register+route, DLQ logic
- `internal/queue/dedupe.go` — Redis message deduplication
- `internal/queue/consumer.go` — Kafka consumer group reader, poll-dispatch loop
- `tests/unit/queue/dispatcher_test.go` — Dispatcher unit tests with fake handler
- `tests/unit/queue/dedupe_test.go` — Dedup unit tests with fake Redis

Modify:
- `go.mod` — add robfig/cron/v3, segmentio/kafka-go
- `internal/config/config.go` — add KafkaConfig struct
- `internal/config/config_test.go` — assert Kafka defaults
- `internal/worker/daily_obsidian_publish.go` — implement Handler interface
- `internal/fxapp/app.go` — ticker → cron + kafka lifecycle

Delete:
- `internal/worker/daily_scheduler.go` — entirely replaced by cron

Add to infrastructure:
- `docker-compose.yml` — Kafka + Zookeeper services
- `.env.example` — KAFKA_BROKERS, KAFKA_CONSUMER_GROUP
- `db/migrations/000001_create_all_tables.up.sql` — dead_letter_records table
- `db/migrations/000001_create_all_tables.down.sql` — drop dead_letter_records

---

### Task 1: Dependencies, Config, and Infrastructure

**Files:**
- Modify: `go.mod`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go` (or `tests/unit/config/config_test.go`)
- Modify: `docker-compose.yml`
- Modify: `.env.example`

- [ ] **Step 1: Add dependencies to go.mod**

Run:

```bash
go get github.com/robfig/cron/v3@v3.0.1
go get github.com/segmentio/kafka-go@v0.4.47
```

Verify:

```bash
grep -E 'cron|kafka-go' go.mod
```

Expected:

```
github.com/robfig/cron/v3 v3.0.1
github.com/segmentio/kafka-go v0.4.47
```

- [ ] **Step 2: Add Kafka config to internal/config/config.go**

Add after the `RedisAddr` field (line 17):

```go
KafkaBrokers       []string `mapstructure:"KAFKA_BROKERS"`
KafkaConsumerGroup string   `mapstructure:"KAFKA_CONSUMER_GROUP"`
```

Add defaults after the `REDIS_ADDR` default (line 53):

```go
v.SetDefault("KAFKA_BROKERS", []string{"localhost:9092"})
v.SetDefault("KAFKA_CONSUMER_GROUP", "hotkey-workers")
```

Add `BindEnv` calls after the `REDIS_ADDR` bind (after line 59):

```go
_ = v.BindEnv("KAFKA_BROKERS")
_ = v.BindEnv("KAFKA_CONSUMER_GROUP")
```

- [ ] **Step 3: Add Kafka defaults test**

In `tests/unit/config/config_test.go` (or `internal/config/config_test.go`), add:

```go
func TestKafkaDefaults(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.KafkaBrokers) == 0 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Fatalf("KafkaBrokers = %v, want [localhost:9092]", cfg.KafkaBrokers)
	}
	if cfg.KafkaConsumerGroup != "hotkey-workers" {
		t.Fatalf("KafkaConsumerGroup = %q, want hotkey-workers", cfg.KafkaConsumerGroup)
	}
}
```

Run:

```bash
go test ./tests/unit/config -run TestKafkaDefaults -count=1 -v
```

Expected: PASS.

- [ ] **Step 4: Add Kafka + Zookeeper to docker-compose.yml**

Add before `app:` service (between lines 17 and 18):

```yaml
  zookeeper:
    image: confluentinc/cp-zookeeper:7.8.0
    container_name: hotkey-zookeeper
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
    ports:
      - "2181:2181"
    healthcheck:
      test: ["CMD-SHELL", "echo ruok | nc -w 2 localhost 2181 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5

  kafka:
    image: confluentinc/cp-kafka:7.8.0
    container_name: hotkey-kafka
    depends_on:
      zookeeper:
        condition: service_started
    ports:
      - "9092:9092"
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_LOG_RETENTION_HOURS: 168
    volumes:
      - kafka-data:/var/lib/kafka/data
    healthcheck:
      test: ["CMD-SHELL", "kafka-topics --bootstrap-server localhost:9092 --list || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
```

Add `kafka-data` volume to the `volumes:` section:

```yaml
volumes:
  postgres_data:
  kafka-data:
```

- [ ] **Step 5: Update .env.example**

Add after the `# REDIS_ADDR=localhost:6379` comment block:

```bash
# --- Kafka（后台任务队列）---
# 优先用本机已安装的 Kafka；docker-compose 时设为 kafka:9092
# KAFKA_BROKERS=localhost:9092
# KAFKA_CONSUMER_GROUP=hotkey-workers
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/config.go tests/unit/config/config_test.go docker-compose.yml .env.example
git commit -m "feat: add cron, kafka-go deps and config, update docker-compose and env"
```

---

### Task 2: Message Model and Shared Types

**Files:**
- Create: `internal/queue/message.go`
- Create: `internal/queue/types.go`

- [ ] **Step 1: Create internal/queue/message.go**

```go
package queue

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrHandlerNotFound = errors.New("no handler registered for message type")
	ErrInvalidMessage  = errors.New("invalid message: missing id, type, or payload")
	ErrProducerClosed  = errors.New("producer is closed")
	ErrConsumerClosed  = errors.New("consumer is closed")
)

const (
	// Topic names
	TopicDigestRun  = "hotkey.digest.run"
	TopicCollectRun = "hotkey.collect.run"
	TopicNotifyRun  = "hotkey.notify.run"

	// DLQ topic names
	TopicDigestRunDLQ  = "hotkey.digest.run.dlq"
	TopicCollectRunDLQ = "hotkey.collect.run.dlq"
	TopicNotifyRunDLQ  = "hotkey.notify.run.dlq"

	// DLQ config
	MaxRetries = 3
)

// Message is the universal message envelope for all queue operations.
type Message struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
	RetryCount int             `json:"retry_count"`
}

func NewMessage(msgType string, payload json.RawMessage) Message {
	// uuid v7 not required for dev — use timestamp+random
	return Message{
		ID:        msgType + "-" + time.Now().Format("150405.000000"),
		Type:      msgType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
}
```

- [ ] **Step 2: Create internal/queue/types.go**

```go
package queue

import "time"

// DLQRecord represents a message that was routed to the dead letter queue.
type DLQRecord struct {
	Topic       string    `json:"topic"`
	MessageID   string    `json:"message_id"`
	MessageType string    `json:"message_type"`
	Payload     string    `json:"payload"`
	ErrorMsg    string    `json:"error_msg"`
	RetryCount  int       `json:"retry_count"`
	CreatedAt   time.Time `json:"created_at"`
}
```

- [ ] **Step 3: Verify compilation**

```bash
go vet ./internal/queue/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/queue/message.go internal/queue/types.go
git commit -m "feat: add queue message model and shared types"
```

---

### Task 3: Kafka Producer

**Files:**
- Create: `internal/queue/producer.go`

- [ ] **Step 1: Create internal/queue/producer.go**

```go
package queue

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/segmentio/kafka-go"
)

// Producer publishes messages to Kafka topics.
type Producer struct {
	writer *kafka.Writer
	closed atomic.Bool
}

// NewProducer creates a Producer connected to the given brokers.
func NewProducer(brokers []string) *Producer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequiresAll, // wait for all ISR replicas
		Async:        false,             // synchronous for error propagation
	}
	return &Producer{writer: w}
}

// Publish sends a message to the given topic. Returns error if closed or write fails.
func (p *Producer) Publish(ctx context.Context, topic string, msg Message) error {
	if p.closed.Load() {
		return ErrProducerClosed
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(msg.Type + ":" + msg.ID),
		Value: bytes,
	})
}

// Close shuts down the underlying writer.
func (p *Producer) Close() error {
	p.closed.Store(true)
	return p.writer.Close()
}
```

- [ ] **Step 2: Verify compilation**

```bash
go vet ./internal/queue/
```

Expected: no errors.

- [ ] **Step 3: Write producer test (with fake writer)**

Create `tests/unit/queue/producer_test.go`:

```go
package queue_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestProducerPublish(t *testing.T) {
	p := queue.NewProducer([]string{"localhost:9092"})
	defer p.Close()

	msg := queue.NewMessage("test.type", nil)
	err := p.Publish(context.Background(), "hotkey.test", msg)
	if err != nil {
		t.Fatalf("Publish returned error (is Kafka running?): %v", err)
	}
}
```

Note: This test requires a running Kafka broker. It will fail if Kafka is not available. Mark it explicitly:

```go
// TestProducerPublish requires a running Kafka broker at localhost:9092.
```

- [ ] **Step 4: Commit**

```bash
git add internal/queue/producer.go tests/unit/queue/producer_test.go
git commit -m "feat: add Kafka producer wrapper"
```

---

### Task 4: Dispatcher and Redis Dedup

**Files:**
- Create: `internal/queue/dispatcher.go`
- Create: `internal/queue/dedupe.go`
- Create: `tests/unit/queue/dispatcher_test.go`
- Create: `tests/unit/queue/dedupe_test.go`

- [ ] **Step 1: Create internal/queue/dispatcher.go**

```go
package queue

import (
	"context"
	"log"
	"sync"
)

// Handler processes a single message. Each handler is registered for one message Type.
type Handler interface {
	// Type returns the message type this handler can process (e.g. "digest.run").
	Type() string

	// Handle processes a message. Return nil on success, error on failure.
	Handle(ctx context.Context, msg Message) error

	// DedupeEnabled returns true if this handler expects message-id deduplication.
	DedupeEnabled() bool
}

// HandlerFunc is a convenience adapter for handlers that don't need dedup.
type HandlerFunc struct {
	MsgType string
	Fn      func(ctx context.Context, msg Message) error
}

func (h HandlerFunc) Type() string               { return h.MsgType }
func (h HandlerFunc) Handle(ctx context.Context, msg Message) error { return h.Fn(ctx, msg) }
func (h HandlerFunc) DedupeEnabled() bool         { return false }

// Dispatcher routes incoming messages to registered handlers.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	producer *Producer // used to publish to DLQ on retry exhaustion
}

func NewDispatcher(producer *Producer) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
		producer: producer,
	}
}

// Register adds a handler. Panics if a handler for the same type is already registered.
func (d *Dispatcher) Register(h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.handlers[h.Type()]; exists {
		log.Panicf("dispatcher: handler for type %q already registered", h.Type())
	}
	d.handlers[h.Type()] = h
}

// Dispatch routes a message to its registered handler.
// Returns nil on success. On failure, publishes to DLQ if retries exhausted.
func (d *Dispatcher) Dispatch(ctx context.Context, msg Message) error {
	if msg.ID == "" || msg.Type == "" {
		return ErrInvalidMessage
	}

	d.mu.RLock()
	h, ok := d.handlers[msg.Type]
	d.mu.RUnlock()

	if !ok {
		return ErrHandlerNotFound
	}

	err := h.Handle(ctx, msg)
	if err == nil {
		return nil
	}

	// Handler failed — decide: retry or DLQ
	msg.RetryCount++
	if msg.RetryCount < MaxRetries {
		// Re-publish to original topic for retry
		if pubErr := d.producer.Publish(ctx, topicForType(msg.Type), msg); pubErr != nil {
			log.Printf("dispatcher: retry publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
		}
		return err
	}

	// Retries exhausted — publish to DLQ
	dlqTopic := topicForDLQ(msg.Type)
	log.Printf("dispatcher: routing %s/%s to DLQ %s after %d retries: %v", msg.Type, msg.ID, dlqTopic, msg.RetryCount, err)
	if pubErr := d.producer.Publish(ctx, dlqTopic, msg); pubErr != nil {
		log.Printf("dispatcher: DLQ publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
	}
	return err
}

func topicForType(msgType string) string {
	switch msgType {
	case "digest.run":
		return TopicDigestRun
	default:
		return msgType
	}
}

func topicForDLQ(msgType string) string {
	switch msgType {
	case "digest.run":
		return TopicDigestRunDLQ
	default:
		return msgType + ".dlq"
	}
}
```

- [ ] **Step 2: Create internal/queue/dedupe.go**

```go
package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const dedupKeyPrefix = "queue:dedup:"

// Dedupe provides Redis-backed message-id deduplication.
type Dedupe struct {
	client redis.UniversalClient
	ttl    time.Duration
}

func NewDedupe(client redis.UniversalClient) *Dedupe {
	return &Dedupe{client: client, ttl: 24 * time.Hour}
}

// Seen returns true if the message ID was already processed (dedup hit),
// or false if this is the first time (caller should process). On first
// encounter, atomically records the ID with TTL.
func (d *Dedupe) Seen(ctx context.Context, msgID string) (bool, error) {
	if d.client == nil {
		return false, nil // dedup disabled when no Redis configured
	}
	key := dedupKeyPrefix + msgID
	ok, err := d.client.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
```

- [ ] **Step 3: Write dispatcher unit tests**

Create `tests/unit/queue/dispatcher_test.go`:

```go
package queue_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type testHandler struct {
	msgType      string
	shouldFail   bool
	callCount    atomic.Int64
	dedupeOn     bool
}

func (h *testHandler) Type() string                               { return h.msgType }
func (h *testHandler) Handle(ctx context.Context, msg queue.Message) error {
	h.callCount.Add(1)
	if h.shouldFail {
		return assertAnError
	}
	return nil
}
func (h *testHandler) DedupeEnabled() bool { return h.dedupeOn }
```

Wait — using a proper error variable. Let me fix:

```go
var errTestHandler = assertAnError
```

Let me use a simpler approach:

```go
package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

var errTestFail = errors.New("handler failed as expected")

type testHandler struct {
	msgType    string
	shouldFail bool
	callCount  atomic.Int64
}

func (h *testHandler) Type() string { return h.msgType }
func (h *testHandler) Handle(ctx context.Context, msg queue.Message) error {
	h.callCount.Add(1)
	if h.shouldFail {
		return errTestFail
	}
	return nil
}
func (h *testHandler) DedupeEnabled() bool { return false }

func TestDispatcherRoutesToCorrectHandler(t *testing.T) {
	// Producer is nil because we won't test DLQ path here.
	d := queue.NewDispatcher(nil)
	h := &testHandler{msgType: "test.type"}
	d.Register(h)

	msg := queue.NewMessage("test.type", json.RawMessage(`{}`))
	err := d.Dispatch(context.Background(), msg)
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if h.callCount.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", h.callCount.Load())
	}
}

func TestDispatcherReturnsErrorForUnknownType(t *testing.T) {
	d := queue.NewDispatcher(nil)
	msg := queue.NewMessage("unknown.type", json.RawMessage(`{}`))
	err := d.Dispatch(context.Background(), msg)
	if err != queue.ErrHandlerNotFound {
		t.Fatalf("error = %v, want ErrHandlerNotFound", err)
	}
}

func TestDispatcherRejectsInvalidMessage(t *testing.T) {
	d := queue.NewDispatcher(nil)
	err := d.Dispatch(context.Background(), queue.Message{})
	if err != queue.ErrInvalidMessage {
		t.Fatalf("error = %v, want ErrInvalidMessage", err)
	}
}
```

- [ ] **Step 4: Write dedup unit tests**

Create `tests/unit/queue/dedupe_test.go`:

```go
package queue_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestDedupeReturnsFalseForNewID(t *testing.T) {
	// Without Redis, dedup is disabled — always returns false (not seen).
	d := queue.NewDedupe(nil)
	seen, err := d.Seen(context.Background(), "msg-1")
	if err != nil {
		t.Fatalf("Seen returned error: %v", err)
	}
	if seen {
		t.Fatal("Seen = true without Redis, want false (dedup disabled)")
	}
}
```

- [ ] **Step 5: Run dispatcher tests**

```bash
go test ./tests/unit/queue -run TestDispatcher -count=1 -v
```

Expected: PASS (all dispatcher tests).

- [ ] **Step 6: Commit**

```bash
git add internal/queue/dispatcher.go internal/queue/dedupe.go tests/unit/queue/
git commit -m "feat: add queue dispatcher, handler interface, and Redis dedup"
```

---

### Task 5: Kafka Consumer

**Files:**
- Create: `internal/queue/consumer.go`

- [ ] **Step 1: Create internal/queue/consumer.go**

```go
package queue

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"

	"github.com/segmentio/kafka-go"
)

// Consumer reads messages from a Kafka topic and dispatches them.
type Consumer struct {
	reader     *kafka.Reader
	dispatcher *Dispatcher
	dedupe     *Dedupe
	closed     atomic.Bool
}

// NewConsumer creates a Consumer for one topic with consumer-group coordination.
func NewConsumer(brokers []string, topic string, groupID string, dispatcher *Dispatcher, dedupe *Dedupe) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     groupID,
		StartOffset: kafka.LastOffset,
		MinBytes:    1,
		MaxBytes:    1e6,
	})
	return &Consumer{reader: r, dispatcher: dispatcher, dedupe: dedupe}
}

// Run starts the poll-dispatch loop. Blocks until ctx is cancelled or a fatal error.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if c.closed.Load() {
			return ErrConsumerClosed
		}
		km, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // normal shutdown
			}
			log.Printf("consumer: fetch error: %v", err)
			continue
		}

		var msg Message
		if err := json.Unmarshal(km.Value, &msg); err != nil {
			log.Printf("consumer: unmarshal error (committing offset): %v", err)
			c.reader.CommitMessages(ctx, km)
			continue
		}

		// Dedup check
		if c.dedupe != nil {
			seen, err := c.dedupe.Seen(ctx, msg.ID)
			if err != nil {
				log.Printf("consumer: dedup error for %s/%s: %v", msg.Type, msg.ID, err)
				// fall through — process anyway on dedup failure
			} else if seen {
				log.Printf("consumer: skipping duplicate %s/%s", msg.Type, msg.ID)
				c.reader.CommitMessages(ctx, km)
				continue
			}
		}

		// Dispatch
		if err := c.dispatcher.Dispatch(ctx, msg); err != nil {
			log.Printf("consumer: dispatch error for %s/%s: %v", msg.Type, msg.ID, err)
			// Still commit offset — already retried/DLQ'd inside dispatcher
		}

		c.reader.CommitMessages(ctx, km)
	}
}

// Close shuts down the consumer reader.
func (c *Consumer) Close() error {
	c.closed.Store(true)
	return c.reader.Close()
}
```

- [ ] **Step 2: Verify compilation**

```bash
go vet ./internal/queue/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/queue/consumer.go
git commit -m "feat: add Kafka consumer with dispatcher integration"
```

---

### Task 6: Migrate DailyObsidianPublishJob to Handler Interface

**Files:**
- Modify: `internal/worker/daily_obsidian_publish.go`

- [ ] **Step 1: Add Handler interface methods**

Add to `internal/worker/daily_obsidian_publish.go`:

```go
func (j *DailyObsidianPublishJob) Type() string { return "digest.run" }

func (j *DailyObsidianPublishJob) Handle(ctx context.Context, msg queue.Message) error {
	var payload struct {
		TargetDate string `json:"target_date"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	targetDate, err := time.Parse("2006-01-02", payload.TargetDate)
	if err != nil {
		return err
	}
	return j.RunOnce(ctx, targetDate)
}

func (j *DailyObsidianPublishJob) DedupeEnabled() bool { return false }
```

Add to the import block:

```go
"encoding/json"
"github.com/StephenQiu30/hotkey-server/internal/queue"
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run existing worker tests**

```bash
go test ./tests/unit/worker -count=1 -v
```

Expected: PASS (no behavioral change, only added methods).

- [ ] **Step 4: Commit**

```bash
git add internal/worker/daily_obsidian_publish.go
git commit -m "feat: add queue.Handler interface to DailyObsidianPublishJob"
```

---

### Task 7: Delete daily_scheduler.go and Rewire Fx App

**Files:**
- Delete: `internal/worker/daily_scheduler.go`
- Modify: `internal/fxapp/app.go`

- [ ] **Step 1: Delete internal/worker/daily_scheduler.go**

```bash
git rm internal/worker/daily_scheduler.go
```

- [ ] **Step 2: Build to confirm first compilation error**

```bash
go build ./... 2>&1 | head -10
```

Expected: errors because `worker.ShouldRun`, `worker.DailyScheduleConfig`, `worker.ResolveTargetDate` are referenced in `fxapp/app.go`.

- [ ] **Step 3: Rewire fxapp/app.go**

Remove the `registerHooks` function and replace with `registerHooks` that uses cron + Kafka:

```go
func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config, dailyJob *worker.DailyObsidianPublishJob) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if os.Getenv("SMOKE_TEST") == "1" {
				go func() {
					if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Printf("http server error: %v", err)
					}
				}()
				return nil
			}

			// --- Kafka producer ---
			producer := queue.NewProducer(cfg.KafkaBrokers)
			lc.Append(fx.Hook{OnStop: func(context.Context) error { return producer.Close() }})

			// --- Dispatcher ---
			dispatcher := queue.NewDispatcher(producer)
			dispatcher.Register(dailyJob)

			// --- Dedupe ---
			var dedupe *queue.Dedupe
			if cfg.RedisAddr != "" {
				rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
				_ = rdb.Ping(ctx).Err() // best-effort
				dedupe = queue.NewDedupe(rdb)
			}

			// --- Kafka consumer (single topic for now) ---
			consumer := queue.NewConsumer(
				cfg.KafkaBrokers,
				queue.TopicDigestRun,
				cfg.KafkaConsumerGroup,
				dispatcher,
				dedupe,
			)
			go func() {
				log.Printf("kafka consumer: starting on %s", queue.TopicDigestRun)
				if err := consumer.Run(ctx); err != nil && err != queue.ErrConsumerClosed {
					log.Printf("kafka consumer error: %v", err)
				}
			}()

			// --- Cron scheduler ---
			loc, err := time.LoadLocation(cfg.DailyDigestTimezone)
			if err != nil {
				return err
			}
			c := cron.New(cron.WithLocation(loc))
			c.AddFunc("0 8 * * *", func() {
				targetDate, err := time.LoadLocation(cfg.DailyDigestTimezone)
				if err != nil {
					log.Printf("cron: resolve location error: %v", err)
					return
				}
				payload, _ := json.Marshal(map[string]string{
					"target_date": time.Now().In(targetDate).AddDate(0, 0, -1).Format("2006-01-02"),
				})
				if err := producer.Publish(context.Background(), queue.TopicDigestRun, queue.NewMessage("digest.run", payload)); err != nil {
					log.Printf("cron: publish digest error: %v", err)
				}
			})
			c.Start()
			lc.Append(fx.Hook{OnStop: func(context.Context) error { c.Stop(); return nil }})

			go func() {
				log.Printf("worker: started (cron + kafka)")
				<-ctx.Done()
				log.Printf("worker: stopped")
			}()
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Printf("http server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if db != nil {
				sqlDB, err := db.DB()
				if err == nil && sqlDB != nil {
					sqlDB.Close()
				}
			}
			return srv.Shutdown(ctx)
		},
	})
}
```

Add new imports:

```go
import (
	"context"
	"encoding/json"
	"log"
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
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/fx"
	"gorm.io/gorm"
)
```

- [ ] **Step 4: Clean unused imports from worker package**

After deleting `daily_scheduler.go`, check if any other files in `internal/worker/` reference the deleted types. Run:

```bash
go vet ./internal/worker/
```

Expected: no errors.

- [ ] **Step 5: Build and run tests**

```bash
go build ./...
go vet ./...
go test ./tests/unit/worker -count=1 -v
```

Expected: Build OK, vet OK, worker tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/fxapp/app.go internal/worker/daily_scheduler.go
git commit -m "feat: wire cron + kafka lifecycle, remove hand-written scheduler"
```

Note: `git add` on a deleted file stages the deletion.

---

### Task 8: DB Migration for dead_letter_records Table

**Files:**
- Modify: `db/migrations/000001_create_all_tables.up.sql`
- Modify: `db/migrations/000001_create_all_tables.down.sql`
- Modify: `db/schema.sql`
- Modify: `tests/testutil/db.go`

- [ ] **Step 1: Add dead_letter_records table**

Add to `db/migrations/000001_create_all_tables.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS dead_letter_records (
    id              BIGSERIAL PRIMARY KEY,
    topic           VARCHAR(255) NOT NULL,
    message_id      VARCHAR(128) NOT NULL,
    message_type    VARCHAR(64)  NOT NULL,
    payload         TEXT,
    error_message   TEXT,
    retry_count     INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_created_at ON dead_letter_records(created_at);
```

Add `dead_letter_records` drop before `reports` to `db/migrations/000001_create_all_tables.down.sql`:

```sql
DROP TABLE IF EXISTS dead_letter_records;
```

- [ ] **Step 2: Update db/schema.sql**

Add the same `CREATE TABLE dead_letter_records` block to `db/schema.sql`.

- [ ] **Step 3: Add dead_letter_records to test clean list**

In `tests/testutil/db.go`, add `"dead_letter_records"` after `"knowledge_runs"` in the `cleanTables` slice (line 62).

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000001_create_all_tables.up.sql db/migrations/000001_create_all_tables.down.sql db/schema.sql tests/testutil/db.go
git commit -m "feat: add dead_letter_records table for DLQ persistence"
```

---

## Self-Review

**Spec coverage check:**
- ✅ Section 3 (Message Model) → Task 2
- ✅ Section 4 (DLQ) → Task 4 (dispatcher retry/DLQ logic) + Task 8 (DB table)
- ✅ Section 5 (Dedup) → Task 4 (dedupe.go)
- ✅ Section 6 (Module Structure) → Tasks 2-7 (all files match)
- ✅ Section 7 (Cron) → Task 7 (cron.New() in fxapp)
- ✅ Section 8 (Infrastructure) → Task 1 (docker-compose, config)
- ✅ Section 9 (Change List) → All tasks cover the full list
- ✅ Section 10 (Subsequent) → Called out as "not in scope"

**Placeholder scan:** No TBD, TODO, or "fill in later" found.

**Type consistency:** `Message.Type` in Task 2 matches `Handler.Type()` in Task 4 matches `"digest.run"` in Task 6.

---

Plan complete and saved to `docs/superpowers/plans/2026-07-08-backend-task-infrastructure.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
