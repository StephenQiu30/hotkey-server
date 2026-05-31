package hotspotrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

func TestRepositorySaveEmbeddingUpsertsVectorAndAuditStatus(t *testing.T) {
	now := time.Date(2026, 5, 31, 3, 0, 0, 0, time.UTC)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"item-1",
			"text-embedding-v2",
			"[0.1,0.2,0.3]",
			"hash-1",
			string(hotspot.EmbeddingStatusSucceeded),
			"",
			now,
			now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)

	created, err := repo.SaveEmbedding(context.Background(), hotspot.Embedding{
		ItemID:    "item-1",
		Model:     "text-embedding-v2",
		Vector:    []float64{0.1, 0.2, 0.3},
		TextHash:  "hash-1",
		Status:    hotspot.EmbeddingStatusSucceeded,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save embedding failed: %v", err)
	}
	if created.ItemID != "item-1" || len(created.Vector) != 3 {
		t.Fatalf("unexpected embedding: %+v", created)
	}
	if !strings.Contains(driver.lastQuery(), "insert into item_embeddings") || !strings.Contains(driver.lastQuery(), "::vector") {
		t.Fatalf("expected vector upsert query, got %q", driver.lastQuery())
	}
	args := driver.lastArgs()
	if len(args) != 8 || args[2] != "[0.1,0.2,0.3]" || args[4] != string(hotspot.EmbeddingStatusSucceeded) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestParseVectorLiteralRejectsMalformedToken(t *testing.T) {
	if _, err := parseVectorLiteral("[0.1,nope,0.3]"); err == nil {
		t.Fatal("expected malformed vector token to return an error")
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "hotspotrepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
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

func (c *recordingConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.driver.record(query, namedValues(args))
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
	return []string{"item_id", "model", "embedding", "text_hash", "status", "last_error", "created_at", "updated_at"}
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
