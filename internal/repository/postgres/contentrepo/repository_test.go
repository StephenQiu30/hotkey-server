package contentrepo

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

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

func TestRepositoryCreateInsertsSourceItemDedupeFields(t *testing.T) {
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	publishedAt := now.Add(-time.Hour)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"item-1",
			"src-1",
			"AI 新闻",
			"正文片段",
			"https://example.com/a?utm_source=rss",
			"https://example.com/a",
			publishedAt,
			"hash-1",
			"zh",
			string(content.ItemStatusDuplicate),
			"item-primary",
			string(content.ItemFilterStatusPassed),
			"",
			0.85,
			true,
			now,
			now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)
	item := content.SourceItem{
		ID:                "item-1",
		SourceID:          "src-1",
		Title:             "AI 新闻",
		Snippet:           "正文片段",
		RawURL:            "https://example.com/a?utm_source=rss",
		CanonicalURL:      "https://example.com/a",
		PublishedAt:       &publishedAt,
		ContentHash:       "hash-1",
		Language:          "zh",
		Status:            content.ItemStatusDuplicate,
		DuplicateOfItemID: "item-primary",
		FilterStatus:      content.ItemFilterStatusPassed,
		QualityScore:      0.85,
		Summarizable:      true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if _, err := repo.Create(context.Background(), item); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if !strings.Contains(driver.lastQuery(), "insert into source_items") {
		t.Fatalf("expected insert into source_items, got %q", driver.lastQuery())
	}
	for _, want := range []string{"canonical_url", "content_hash", "language", "status", "duplicate_of_item_id", "filter_status", "filter_reason", "quality_score", "summarizable"} {
		if !strings.Contains(driver.lastQuery(), want) {
			t.Fatalf("expected query to include %q, got %q", want, driver.lastQuery())
		}
	}
	args := driver.lastArgs()
	if len(args) != 17 {
		t.Fatalf("expected 17 insert args, got %d", len(args))
	}
	if args[5] != item.CanonicalURL || args[7] != item.ContentHash || args[10] != item.DuplicateOfItemID {
		t.Fatalf("unexpected insert args: %#v", args)
	}
}

func TestRepositoryCreateInsertsFilteredItemWithReason(t *testing.T) {
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"item-2",
			"src-1",
			"广告内容",
			"推广片段",
			"https://example.com/ad",
			"https://example.com/ad",
			nil,
			"hash-2",
			"zh",
			string(content.ItemStatusPrimary),
			"",
			string(content.ItemFilterStatusFiltered),
			"exclusion_word_match",
			0.0,
			false,
			now,
			now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)
	item := content.SourceItem{
		ID:           "item-2",
		SourceID:     "src-1",
		Title:        "广告内容",
		Snippet:      "推广片段",
		RawURL:       "https://example.com/ad",
		CanonicalURL: "https://example.com/ad",
		ContentHash:  "hash-2",
		Language:     "zh",
		Status:       content.ItemStatusPrimary,
		FilterStatus: content.ItemFilterStatusFiltered,
		FilterReason: "exclusion_word_match",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if _, err := repo.Create(context.Background(), item); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	args := driver.lastArgs()
	// filter_reason should be at index 12 (0-based: id,source_id,title,snippet,raw_url,canonical_url,published_at,content_hash,language,status,duplicate_of_item_id,filter_status,filter_reason,...)
	if args[12] != "exclusion_word_match" {
		t.Fatalf("expected filter_reason 'exclusion_word_match', got %q", args[12])
	}
}

func TestRepositoryFindByContentHashQueriesPrimaryItem(t *testing.T) {
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"item-1",
			"src-1",
			"AI 新闻",
			"正文片段",
			"https://example.com/a",
			"https://example.com/a",
			nil,
			"hash-1",
			"zh",
			string(content.ItemStatusPrimary),
			nil,
			string(content.ItemFilterStatusPassed),
			"",
			0.9,
			true,
			now,
			now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)

	item, err := repo.FindByContentHash(context.Background(), "hash-1")
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if item.ID != "item-1" || item.ContentHash != "hash-1" || item.Status != content.ItemStatusPrimary {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.FilterStatus != content.ItemFilterStatusPassed || item.QualityScore != 0.9 || !item.Summarizable {
		t.Fatalf("expected quality fields populated, got %+v", item)
	}
	if !strings.Contains(driver.lastQuery(), "where content_hash =") || !strings.Contains(driver.lastQuery(), "status = 'primary'") {
		t.Fatalf("expected primary content hash lookup, got %q", driver.lastQuery())
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "contentrepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
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
	return []string{"id", "source_id", "title", "snippet", "raw_url", "canonical_url", "published_at", "content_hash", "language", "status", "duplicate_of_item_id", "filter_status", "filter_reason", "quality_score", "summarizable", "created_at", "updated_at"}
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
