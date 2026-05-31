package jobrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestRepositoryCreateInsertsJobAuditRecord(t *testing.T) {
	driver := &recordingDriver{}
	db := openRecordingDB(t, driver)
	repo := New(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"source_id":"source-1","scheduled_for":"2026-05-31T02:00:00Z"}`)
	job := queue.Job{
		ID:             "6dc3c65d-cc79-49f0-a120-66e671f513a7",
		Type:           queue.JobTypeCollectSource,
		Payload:        payload,
		Status:         queue.JobStatusPending,
		Attempt:        1,
		MaxAttempts:    3,
		IdempotencyKey: "collect_source:source-1:2026-05-31T02",
		LastError:      "temporary failure",
		NextRunAt:      now,
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now,
	}

	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if !strings.Contains(driver.lastQuery(), "insert into jobs") {
		t.Fatalf("expected insert into jobs, got %q", driver.lastQuery())
	}
	for _, want := range []string{"id", "job_type", "payload", "status", "attempt", "max_attempts", "idempotency_key", "last_error", "scheduled_at", "created_at", "updated_at"} {
		if !strings.Contains(driver.lastQuery(), want) {
			t.Fatalf("expected query to include %q, got %q", want, driver.lastQuery())
		}
	}
	args := driver.lastArgs()
	if len(args) != 11 {
		t.Fatalf("expected 11 insert args, got %d", len(args))
	}
	if args[0] != job.ID || args[1] != string(job.Type) || string(args[2].([]byte)) != string(payload) || args[6] != job.IdempotencyKey {
		t.Fatalf("unexpected insert args: %#v", args)
	}
}

func TestRepositoryFindByIdempotencyKeyScansAuditRecord(t *testing.T) {
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"6dc3c65d-cc79-49f0-a120-66e671f513a7",
			string(queue.JobTypeGenerateEmbedding),
			[]byte(`{"item_id":"item-1"}`),
			string(queue.JobStatusScheduled),
			int64(1),
			int64(3),
			"embedding:item-1",
			"retry later",
			time.Date(2026, 5, 31, 2, 5, 0, 0, time.UTC),
			time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 31, 2, 1, 0, 0, time.UTC),
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)

	job, err := repo.FindByIdempotencyKey(context.Background(), "embedding:item-1")
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if job.Type != queue.JobTypeGenerateEmbedding || job.Status != queue.JobStatusScheduled || job.Attempt != 1 || job.LastError != "retry later" {
		t.Fatalf("unexpected job: %+v", job)
	}
	if !strings.Contains(driver.lastQuery(), "where idempotency_key =") {
		t.Fatalf("expected idempotency lookup, got %q", driver.lastQuery())
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "jobrepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
	sql.Register(name, d)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type recordingDriver struct {
	mu    sync.Mutex
	query string
	args  []driver.Value
	rows  [][]driver.Value
}

func (d *recordingDriver) Open(string) (driver.Conn, error) {
	return &recordingConn{driver: d}, nil
}

func (d *recordingDriver) record(query string, args []driver.Value) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.query = strings.ToLower(query)
	d.args = append([]driver.Value(nil), args...)
}

func (d *recordingDriver) lastQuery() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.query
}

func (d *recordingDriver) lastArgs() []driver.Value {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]driver.Value(nil), d.args...)
}

type recordingConn struct {
	driver *recordingDriver
}

func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (c *recordingConn) Close() error {
	return nil
}

func (c *recordingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c *recordingConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	values := namedValues(args)
	c.driver.record(query, values)
	return driver.RowsAffected(1), nil
}

func (c *recordingConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := namedValues(args)
	c.driver.record(query, values)
	return &recordingRows{rows: c.driver.rows}, nil
}

func namedValues(args []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return values
}

type recordingRows struct {
	index int
	rows  [][]driver.Value
}

func (r *recordingRows) Columns() []string {
	return []string{"id", "job_type", "payload", "status", "attempt", "max_attempts", "idempotency_key", "last_error", "scheduled_at", "created_at", "updated_at"}
}

func (r *recordingRows) Close() error {
	return nil
}

func (r *recordingRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}
