package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	stdsmtp "net/smtp"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

type Config struct {
	Host      string
	Port      int
	TLSMode   string
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type Message struct {
	From    string
	To      string
	Subject string
	Body    string
}

type SendFunc func(context.Context, Message) error

type Mailer struct {
	config Config
	send   SendFunc
}

var _ domain.Mailer = (*Mailer)(nil)

func NewMailer(config Config, send ...SendFunc) *Mailer {
	mailer := &Mailer{config: config}
	if len(send) > 0 && send[0] != nil {
		mailer.send = send[0]
	} else {
		mailer.send = mailer.sendSMTP
	}
	return mailer
}

func (mailer *Mailer) SendVerificationCode(ctx context.Context, purpose domain.VerificationPurpose, email, code string) error {
	if mailer == nil || !purpose.Valid() || strings.TrimSpace(email) == "" || strings.TrimSpace(code) == "" || !mailer.config.valid() {
		return unavailable()
	}
	message := Message{
		From:    formatAddress(mailer.config.FromName, mailer.config.FromEmail),
		To:      strings.TrimSpace(email),
		Subject: "HotKey verification code",
		Body:    fmt.Sprintf("Your HotKey %s verification code is: %s\r\n", purpose, code),
	}
	if err := mailer.send(ctx, message); err != nil {
		return unavailable()
	}
	return nil
}

func (mailer *Mailer) sendSMTP(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	address := net.JoinHostPort(mailer.config.Host, fmt.Sprintf("%d", mailer.config.Port))
	dialer := &net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()
	if mailer.config.TLSMode == "tls" {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: mailer.config.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return err
		}
		connection = tlsConnection
	}

	client, err := stdsmtp.NewClient(connection, mailer.config.Host)
	if err != nil {
		return err
	}
	defer client.Quit()
	if mailer.config.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: mailer.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(mailer.config.Username) != "" {
		if err := client.Auth(stdsmtp.PlainAuth("", mailer.config.Username, mailer.config.Password, mailer.config.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(mailer.config.FromEmail); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, "From: "+message.From+"\r\nTo: "+message.To+"\r\nSubject: "+message.Subject+"\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n"+message.Body); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func (config Config) valid() bool {
	if strings.TrimSpace(config.Host) == "" || config.Port <= 0 || strings.TrimSpace(config.FromEmail) == "" {
		return false
	}
	return config.TLSMode == "tls" || config.TLSMode == "starttls"
}

func formatAddress(name, email string) string {
	if strings.TrimSpace(name) == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", strings.TrimSpace(name), email)
}

func unavailable() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
}
