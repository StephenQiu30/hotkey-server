package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"

	servicemail "github.com/StephenQiu30/hotkey-server/internal/service/mail"
)

type SMTPMailer struct {
	cfg servicemail.Config
}

func NewSMTPMailer(cfg servicemail.Config) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) Send(ctx context.Context, message servicemail.Message) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !m.cfg.Configured || strings.TrimSpace(m.cfg.Host) == "" || strings.TrimSpace(m.cfg.From) == "" {
		return errors.New("smtp missing_config")
	}
	port := m.cfg.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", port))
	auth := smtp.Auth(nil)
	if m.cfg.Username != "" || m.cfg.Password != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	body, err := buildMIME(message)
	if err != nil {
		return err
	}
	return m.send(ctx, addr, auth, message, body)
}

func (m *SMTPMailer) send(ctx context.Context, addr string, auth smtp.Auth, message servicemail.Message, body []byte) error {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	tlsConfig := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	if m.cfg.TLS {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return err
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !m.cfg.TLS && m.cfg.StartTLS {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(message.From); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildMIME(message servicemail.Message) ([]byte, error) {
	for name, value := range map[string]string{
		"from":    message.From,
		"to":      message.To,
		"subject": message.Subject,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("smtp message %s header contains newline", name)
		}
	}
	var body bytes.Buffer
	body.WriteString("From: " + message.From + "\r\n")
	body.WriteString("To: " + message.To + "\r\n")
	body.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject) + "\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: multipart/alternative; boundary=hotkey-daily-report\r\n\r\n")
	body.WriteString("--hotkey-daily-report\r\n")
	body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	body.WriteString(message.TextBody + "\r\n")
	body.WriteString("--hotkey-daily-report\r\n")
	body.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	body.WriteString(message.HTMLBody + "\r\n")
	body.WriteString("--hotkey-daily-report--\r\n")
	return body.Bytes(), nil
}
