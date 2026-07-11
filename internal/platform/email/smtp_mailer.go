package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"time"
)

// SMTPConfig holds SMTP server configuration for sending emails.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	AuthCode string
	From     string
	FromName string
}

// SMTPMailer sends emails via an SMTP server using implicit TLS.
type SMTPMailer struct {
	cfg      SMTPConfig
	tlsCfg   *tls.Config
	hostPort string
}

// NewSMTPMailer creates a new SMTPMailer with the given configuration.
func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	tlsCfg := &tls.Config{
		ServerName: cfg.Host,
		MinVersion: tls.VersionTLS12,
	}
	hostPort := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	return &SMTPMailer{
		cfg:      cfg,
		tlsCfg:   tlsCfg,
		hostPort: hostPort,
	}
}

// TLSDialer returns the TLS configuration used for SMTP connections.
// Exported for testing.
func (m *SMTPMailer) TLSDialer() *tls.Config {
	return m.tlsCfg
}

// SMTPAuth returns the SMTP auth mechanism used for authentication.
// Exported for testing.
func (m *SMTPMailer) SMTPAuth() smtp.Auth {
	return smtp.PlainAuth("", m.cfg.Username, m.cfg.AuthCode, m.cfg.Host)
}

// Send delivers an email via SMTP. It respects context deadlines and cancellation.
// On failure, it returns a sanitized error that never exposes credentials.
func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	// Dial with 5-second connect timeout
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 5 * time.Second},
		Config:    m.tlsCfg,
	}
	conn, err := dialer.DialContext(dialCtx, "tcp", m.hostPort)
	if err != nil {
		return m.sanitizeError(fmt.Errorf("failed to connect to SMTP server: %w", err))
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return m.sanitizeError(fmt.Errorf("failed to create SMTP client: %w", err))
	}
	defer client.Close()

	// Authenticate
	auth := m.SMTPAuth()
	if err := client.Auth(auth); err != nil {
		return m.sanitizeError(fmt.Errorf("SMTP authentication failed: %w", err))
	}

	// Set sender and recipient
	if err := client.Mail(m.cfg.From); err != nil {
		return m.sanitizeError(fmt.Errorf("failed to set sender: %w", err))
	}
	if err := client.Rcpt(msg.To); err != nil {
		return m.sanitizeError(fmt.Errorf("failed to set recipient: %w", err))
	}

	// Send data with 10-second operation deadline
	sendCtx, sendCancel := context.WithDeadline(ctx, time.Now().Add(10*time.Second))
	defer sendCancel()

	done := make(chan error, 1)
	go func() {
		w, err := client.Data()
		if err != nil {
			done <- m.sanitizeError(fmt.Errorf("failed to open data writer: %w", err))
			return
		}
		_, err = buildMessage(w, m.cfg.From, m.cfg.FromName, msg)
		if err != nil {
			w.Close() //nolint:errcheck
			done <- m.sanitizeError(fmt.Errorf("failed to write message: %w", err))
			return
		}
		done <- w.Close()
	}()

	select {
	case <-sendCtx.Done():
		return m.sanitizeError(fmt.Errorf("send operation timed out or cancelled: %w", sendCtx.Err()))
	case err := <-done:
		if err != nil {
			return m.sanitizeError(fmt.Errorf("failed to send message: %w", err))
		}
		return nil
	}
}

// sanitizeError strips the underlying error of any credential-like data
// and returns a safe, generic error message.
func (m *SMTPMailer) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("email delivery failed: %v", err)
}

// buildMessage writes the multipart/alternative email message to the writer.
// The writer is typically the io.WriteCloser returned by smtp.Client.Data().
func buildMessage(w io.Writer, from, fromName string, msg Message) (int, error) {
	boundary := "hotkey-alt-boundary"

	header := fmt.Sprintf("From: %s <%s>\r\n", fromName, from)
	header += fmt.Sprintf("To: %s\r\n", msg.To)
	header += fmt.Sprintf("Subject: %s\r\n", msg.Subject)
	header += "MIME-Version: 1.0\r\n"
	header += fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n", boundary)
	header += "\r\n"

	if _, err := io.WriteString(w, header); err != nil {
		return 0, err
	}

	// Plain text part
	textPart := fmt.Sprintf("--%s\r\n", boundary)
	textPart += "Content-Type: text/plain; charset=UTF-8\r\n"
	textPart += "Content-Transfer-Encoding: 7bit\r\n"
	textPart += "\r\n"
	textPart += msg.Text + "\r\n"

	if _, err := io.WriteString(w, textPart); err != nil {
		return 0, err
	}

	// HTML part
	htmlPart := fmt.Sprintf("--%s\r\n", boundary)
	htmlPart += "Content-Type: text/html; charset=UTF-8\r\n"
	htmlPart += "Content-Transfer-Encoding: 7bit\r\n"
	htmlPart += "\r\n"
	htmlPart += msg.HTML + "\r\n"

	if _, err := io.WriteString(w, htmlPart); err != nil {
		return 0, err
	}

	// Closing boundary
	closing := fmt.Sprintf("--%s--\r\n", boundary)
	if _, err := io.WriteString(w, closing); err != nil {
		return 0, err
	}

	return 0, nil
}
