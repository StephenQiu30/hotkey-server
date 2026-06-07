package scorerepo

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

	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
)

func TestRepositorySaveScore(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"score-1", "cluster-1", 0.85, 0.3, 0.2, 0.15, 0.1, 0.1, `{"sourceCount": 5}`, "v1", now, now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)

	score := servicehotspot.HotspotScore{
		ID:         "score-1",
		ClusterID:  "cluster-1",
		TotalScore: 0.85,
		Explanation: `{"sourceCount": 5}`,
		ScoreVersion: "v1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	saved, err := repo.SaveScore(context.Background(), score)
	if err != nil {
		t.Fatalf("save score failed: %v", err)
	}
	if saved.ID != "score-1" || saved.TotalScore != 0.85 {
		t.Fatalf("unexpected score: %+v", saved)
	}
	if !strings.Contains(driver.lastQuery(), "insert into hotspot_scores") {
		t.Fatalf("expected insert query, got %q", driver.lastQuery())
	}
}

func TestRepositoryListScores(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	driver := &recordingDriver{
		rows: [][]driver.Value{{
			"score-1", "cluster-1", 0.85, 0.3, 0.2, 0.15, 0.1, 0.1, `{"sourceCount": 5}`, "v1", now, now,
		}},
	}
	db := openRecordingDB(t, driver)
	repo := New(db)

	scores, err := repo.ListScores(context.Background())
	if err != nil {
		t.Fatalf("list scores failed: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected one score, got %d", len(scores))
	}
	if scores[0].ID != "score-1" {
		t.Fatalf("unexpected score ID: %s", scores[0].ID)
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "scorerepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
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
	return []string{"col1", "col2", "col3", "col4", "col5", "col6", "col7", "col8", "col9", "col10", "col11", "col12"}
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
