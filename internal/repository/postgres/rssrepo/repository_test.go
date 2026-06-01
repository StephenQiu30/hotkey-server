package rssrepo

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

	servicerss "github.com/StephenQiu30/hotkey-server/internal/service/rss"
)

func TestRepositorySaveStoresTokenHashNotPlaintextToken(t *testing.T) {
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	driver := &recordingDriver{rows: [][]driver.Value{{"usr-1", "hash-only", true, nil, now, now}}}
	repo := New(openRecordingDB(t, driver))

	feed, err := repo.Save(context.Background(), servicerss.Feed{
		UserID:    "usr-1",
		TokenHash: "hash-only",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save feed: %v", err)
	}
	if feed.TokenHash != "hash-only" {
		t.Fatalf("unexpected feed: %#v", feed)
	}
	query := driver.lastQuery()
	if !strings.Contains(query, "insert into rss_feeds") || !strings.Contains(query, "token_hash") {
		t.Fatalf("expected rss_feeds token_hash upsert, got %q", query)
	}
	if strings.Contains(query, " token ") {
		t.Fatalf("query must not reference plaintext token column: %q", query)
	}
}

func TestRepositoryFindByTokenHashQueriesRSSFeeds(t *testing.T) {
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	driver := &recordingDriver{rows: [][]driver.Value{{"usr-1", "hash-only", true, now, now, now}}}
	repo := New(openRecordingDB(t, driver))

	feed, err := repo.FindByTokenHash(context.Background(), "hash-only")
	if err != nil {
		t.Fatalf("find by token hash: %v", err)
	}
	if feed.UserID != "usr-1" || feed.LastAccessedAt == nil {
		t.Fatalf("unexpected feed: %#v", feed)
	}
	if !strings.Contains(driver.lastQuery(), "where token_hash =") {
		t.Fatalf("expected token hash lookup, got %q", driver.lastQuery())
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "rssrepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
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
	rows  [][]driver.Value
}

func (d *recordingDriver) Open(string) (driver.Conn, error) {
	return &recordingConn{driver: d}, nil
}

func (d *recordingDriver) record(query string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.query = strings.ToLower(query)
}

func (d *recordingDriver) lastQuery() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.query
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

func (c *recordingConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.driver.record(query)
	return driver.RowsAffected(1), nil
}

func (c *recordingConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.driver.record(query)
	return &recordingRows{rows: c.driver.rows}, nil
}

type recordingRows struct {
	index int
	rows  [][]driver.Value
}

func (r *recordingRows) Columns() []string {
	return []string{"user_id", "token_hash", "enabled", "last_accessed_at", "created_at", "updated_at"}
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
