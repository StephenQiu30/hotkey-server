package mailrepo

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

	servicemail "github.com/StephenQiu30/hotkey-server/internal/service/mail"
)

func TestRepositoryCreateDeliveryInsertsAuditRecord(t *testing.T) {
	driver := &recordingDriver{}
	db := openRecordingDB(t, driver)
	repo := New(db)
	repo.now = func() time.Time { return time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC) }

	delivery, err := repo.CreateDelivery(context.Background(), servicemail.Delivery{
		RecipientUserID: "user-1",
		RecipientEmail:  "reader@example.com",
		ReportID:        "report-1",
		Status:          servicemail.DeliveryStatusPending,
		Attempt:         1,
	})
	if err != nil {
		t.Fatalf("create delivery failed: %v", err)
	}
	if delivery.ID == "" {
		t.Fatal("expected repository to assign delivery id")
	}
	if !strings.Contains(driver.lastQuery(), "insert into email_deliveries") {
		t.Fatalf("expected insert into email_deliveries, got %q", driver.lastQuery())
	}
	for _, want := range []string{"recipient_user_id", "recipient_email", "report_id", "status", "attempt", "last_error", "sent_at"} {
		if !strings.Contains(driver.lastQuery(), want) {
			t.Fatalf("expected query to include %q, got %q", want, driver.lastQuery())
		}
	}
}

func TestRepositoryScansRecipientEmailPreferences(t *testing.T) {
	driver := &recordingDriver{rows: [][]driver.Value{{"user-1", "reader@example.com", true, "09:15"}}}
	db := openRecordingDB(t, driver)
	repo := New(db)

	recipient, err := repo.RecipientByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("recipient lookup failed: %v", err)
	}
	if recipient.Email != "reader@example.com" || !recipient.EmailEnabled || recipient.DailySendAt != "09:15" {
		t.Fatalf("unexpected recipient: %#v", recipient)
	}
	if !strings.Contains(driver.lastQuery(), "email_enabled") {
		t.Fatalf("expected query to read email_enabled, got %q", driver.lastQuery())
	}
}

func openRecordingDB(t *testing.T, d *recordingDriver) *sql.DB {
	t.Helper()
	name := "mailrepo_test_" + strings.ReplaceAll(t.Name(), "/", "_")
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

func (c *recordingConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.driver.record(query, namedValues(args))
	return driver.RowsAffected(1), nil
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
	return []string{"id", "email", "email_enabled", "daily_send_at"}
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
