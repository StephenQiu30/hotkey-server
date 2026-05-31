package mailrepo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	servicemail "github.com/StephenQiu30/hotkey-server/internal/service/mail"
)

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db, now: time.Now}
}

func (r *Repository) DailyReportByID(ctx context.Context, reportID string) (servicemail.DailyReport, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, report_date, title, summary, body_markdown, body_html, url
		FROM daily_reports
		WHERE id = $1
	`, reportID)
	var report servicemail.DailyReport
	if err := row.Scan(&report.ID, &report.ReportDate, &report.Title, &report.Summary, &report.BodyMarkdown, &report.BodyHTML, &report.URL); err != nil {
		if err == sql.ErrNoRows {
			return servicemail.DailyReport{}, servicemail.ErrNotFound
		}
		return servicemail.DailyReport{}, err
	}
	return report, nil
}

func (r *Repository) RecipientByUserID(ctx context.Context, userID string) (servicemail.Recipient, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, email_enabled, daily_send_at
		FROM users
		WHERE id = $1
	`, userID)
	var recipient servicemail.Recipient
	if err := row.Scan(&recipient.UserID, &recipient.Email, &recipient.EmailEnabled, &recipient.DailySendAt); err != nil {
		if err == sql.ErrNoRows {
			return servicemail.Recipient{}, servicemail.ErrNotFound
		}
		return servicemail.Recipient{}, err
	}
	return recipient, nil
}

func (r *Repository) CreateDelivery(ctx context.Context, delivery servicemail.Delivery) (servicemail.Delivery, error) {
	if delivery.ID == "" {
		delivery.ID = "email_delivery_" + randomHex(16)
	}
	now := r.now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO email_deliveries (
			id,
			recipient_user_id,
			recipient_email,
			report_id,
			status,
			attempt,
			last_error,
			sent_at,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10)
	`, delivery.ID, delivery.RecipientUserID, delivery.RecipientEmail, delivery.ReportID, string(delivery.Status), delivery.Attempt, delivery.LastError, delivery.SentAt, delivery.CreatedAt, delivery.UpdatedAt)
	return delivery, err
}

func (r *Repository) UpdateDelivery(ctx context.Context, delivery servicemail.Delivery) (servicemail.Delivery, error) {
	delivery.UpdatedAt = r.now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE email_deliveries
		SET status = $2,
			attempt = $3,
			last_error = NULLIF($4, ''),
			sent_at = $5,
			updated_at = $6
		WHERE id = $1
	`, delivery.ID, string(delivery.Status), delivery.Attempt, delivery.LastError, delivery.SentAt, delivery.UpdatedAt)
	return delivery, err
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf)
}
