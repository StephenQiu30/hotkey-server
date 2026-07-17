package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	stdsmtp "net/smtp"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
)

type Message struct {
	To, Subject, HTML, Text string
}

type Failure struct {
	Code      int
	Temporary bool
	Err       error
}

func (failure *Failure) Error() string {
	if failure == nil || failure.Err == nil {
		return "smtp failure"
	}
	return failure.Err.Error()
}

func (failure *Failure) Unwrap() error          { return failure.Err }
func (failure *Failure) TemporaryFailure() bool { return failure != nil && failure.Temporary }

type Mailer struct {
	config config.SMTPConfig
	send   func(context.Context, Message) error
}

func NewMailer(cfg config.SMTPConfig, send ...func(context.Context, Message) error) *Mailer {
	mailer := &Mailer{config: cfg}
	if len(send) > 0 && send[0] != nil {
		mailer.send = send[0]
	} else {
		mailer.send = mailer.sendSMTP
	}
	return mailer
}

func (mailer *Mailer) Send(ctx context.Context, message Message) error {
	if mailer == nil || !mailer.config.Enabled || !valid(mailer.config) || strings.TrimSpace(message.To) == "" || strings.TrimSpace(message.Subject) == "" {
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp is unavailable")}
	}
	return mailer.send(ctx, message)
}

func (mailer *Mailer) sendSMTP(ctx context.Context, message Message) error {
	address := net.JoinHostPort(mailer.config.Host, fmt.Sprintf("%d", mailer.config.Port))
	dialer := &net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp connection failed")}
	}
	defer connection.Close()
	if mailer.config.TLSMode == "tls" {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: mailer.config.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return &Failure{Temporary: true, Err: fmt.Errorf("smtp TLS handshake failed")}
		}
		connection = tlsConnection
	}
	client, err := stdsmtp.NewClient(connection, mailer.config.Host)
	if err != nil {
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp client failed")}
	}
	defer client.Quit()
	if mailer.config.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return &Failure{Temporary: false, Err: fmt.Errorf("smtp STARTTLS unavailable")}
		}
		if err := client.StartTLS(&tls.Config{ServerName: mailer.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return &Failure{Temporary: true, Err: fmt.Errorf("smtp STARTTLS failed")}
		}
	}
	if strings.TrimSpace(mailer.config.Username) != "" {
		if err := client.Auth(stdsmtp.PlainAuth("", mailer.config.Username, mailer.config.Password, mailer.config.Host)); err != nil {
			return &Failure{Temporary: false, Err: fmt.Errorf("smtp authentication failed")}
		}
	}
	if err := client.Mail(mailer.config.FromEmail); err != nil {
		return &Failure{Temporary: false, Err: fmt.Errorf("smtp sender rejected")}
	}
	if err := client.Rcpt(message.To); err != nil {
		return &Failure{Temporary: false, Err: fmt.Errorf("smtp recipient rejected")}
	}
	body := "From: " + mailer.config.FromEmail + "\r\nTo: " + message.To + "\r\nSubject: " + message.Subject + "\r\nContent-Type: multipart/alternative; boundary=hotkey\r\n\r\n--hotkey\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + message.Text + "\r\n--hotkey\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + message.HTML + "\r\n--hotkey--\r\n"
	writer, err := client.Data()
	if err != nil {
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp DATA failed")}
	}
	if _, err := io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp body failed")}
	}
	if err := writer.Close(); err != nil {
		return &Failure{Temporary: true, Err: fmt.Errorf("smtp delivery failed")}
	}
	return nil
}

func valid(cfg config.SMTPConfig) bool {
	return strings.TrimSpace(cfg.Host) != "" && cfg.Port > 0 && strings.TrimSpace(cfg.FromEmail) != "" && (cfg.TLSMode == "tls" || cfg.TLSMode == "starttls")
}
