package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	PrimaryEmailDeliveryTarget = "primary_email"
	MaximumEmailAttempts       = 5
	EmailDeliveryLeaseDuration = 2 * time.Minute
)

type ClaimNextEmailDeliveryCommand struct {
	ClaimToken string
	ClaimedAt  time.Time
	LeaseUntil time.Time
}

type ClaimedEmailDeliveryDTO struct {
	Claimed           bool
	ClaimToken        string
	AttemptCount      int
	RecipientEmail    string
	Notification      UserNotificationDTO
	PublishedConfigID int64
	PublishedRevision int64
	AlertEmailEnabled bool
}

type CompleteEmailDeliveryCommand struct {
	UserNotificationID int64
	UserID             int64
	ClaimToken         string
	Status             string
	ProviderMessageID  string
	ResponseCode       *int
	ErrorCode          string
	AttemptedAt        time.Time
}

type EmailDeliveryRepository interface {
	ClaimNextEmailDelivery(context.Context, ClaimNextEmailDeliveryCommand) (ClaimedEmailDeliveryDTO, error)
	CompleteEmailDelivery(context.Context, CompleteEmailDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error)
}

type NotificationEmailMessageDTO struct {
	Recipient string
	Subject   string
	Text      string
	HTML      string
}

type NotificationEmailSender interface {
	SendNotificationEmail(context.Context, NotificationEmailMessageDTO) (string, error)
}

type EmailDeliveryServiceDependencies struct {
	Repository EmailDeliveryRepository
	Sender     NotificationEmailSender
	Clock      func() time.Time
	NewToken   func() (string, error)
	WebOrigin  string
}

type EmailDeliveryService struct {
	repository EmailDeliveryRepository
	sender     NotificationEmailSender
	clock      func() time.Time
	newToken   func() (string, error)
	webOrigin  string
}

type DispatchEmailDeliveryResult struct {
	Claimed            bool
	UserNotificationID int64
	Status             string
	AttemptNo          int
}

func NewEmailDeliveryService(dependencies EmailDeliveryServiceDependencies) (*EmailDeliveryService, error) {
	if dependencies.Repository == nil || dependencies.Sender == nil {
		return nil, fmt.Errorf("notification email delivery dependencies are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = func() time.Time { return time.Now().UTC() }
	}
	if dependencies.NewToken == nil {
		dependencies.NewToken = newEmailClaimToken
	}
	origin, err := normalizeWebOrigin(dependencies.WebOrigin)
	if err != nil {
		return nil, err
	}
	return &EmailDeliveryService{
		repository: dependencies.Repository, sender: dependencies.Sender, clock: dependencies.Clock,
		newToken: dependencies.NewToken, webOrigin: origin,
	}, nil
}

// DispatchNext claims at most one durable user notification. SMTP outcomes are
// converted into immutable DeliveryAttempt facts; a channel failure never
// mutates or retracts the UserNotification.
func (service *EmailDeliveryService) DispatchNext(ctx context.Context) (DispatchEmailDeliveryResult, error) {
	if service == nil {
		return DispatchEmailDeliveryResult{}, sharedrepository.ErrUnavailable
	}
	now := service.clock().UTC()
	token, err := service.newToken()
	if err != nil {
		return DispatchEmailDeliveryResult{}, fmt.Errorf("create notification email claim: %w", err)
	}
	claimed, err := service.repository.ClaimNextEmailDelivery(ctx, ClaimNextEmailDeliveryCommand{
		ClaimToken: token, ClaimedAt: now, LeaseUntil: now.Add(EmailDeliveryLeaseDuration),
	})
	if err != nil {
		return DispatchEmailDeliveryResult{}, err
	}
	if !claimed.Claimed {
		return DispatchEmailDeliveryResult{}, nil
	}
	result := DispatchEmailDeliveryResult{Claimed: true, UserNotificationID: claimed.Notification.ID}
	status, errorCode, providerMessageID := "succeeded", "", ""
	if err := validateClaimedEmailDelivery(claimed, token); err != nil {
		status, errorCode = "permanent_failure", "invalid_notification_projection"
	} else {
		message := service.message(claimed)
		providerMessageID, err = service.sender.SendNotificationEmail(ctx, message)
		if err != nil {
			status, errorCode = "failed", "smtp_temporary"
			var temporary interface{ TemporaryFailure() bool }
			if errors.As(err, &temporary) && !temporary.TemporaryFailure() {
				status, errorCode = "permanent_failure", "smtp_permanent"
			}
			if claimed.AttemptCount+1 >= MaximumEmailAttempts {
				status, errorCode = "permanent_failure", "smtp_attempts_exhausted"
			}
		}
	}
	completed, err := service.repository.CompleteEmailDelivery(ctx, CompleteEmailDeliveryCommand{
		UserNotificationID: claimed.Notification.ID, UserID: claimed.Notification.UserID, ClaimToken: token,
		Status: status, ProviderMessageID: providerMessageID, ErrorCode: errorCode, AttemptedAt: service.clock().UTC(),
	})
	if err != nil {
		return DispatchEmailDeliveryResult{}, err
	}
	result.Status, result.AttemptNo = status, completed.AttemptNo
	return result, nil
}

func (service *EmailDeliveryService) message(claimed ClaimedEmailDeliveryDTO) NotificationEmailMessageDTO {
	notification := claimed.Notification
	absoluteLink := service.webOrigin + notification.DeepLink
	title := cleanNotificationText(notification.Title)
	summary := cleanNotificationText(notification.Summary)
	resourceStatus := cleanNotificationText(notification.ResourceStatus)
	text := title + "\n\n" + summary + "\n\n状态：" + resourceStatus + "\n在 HotKey 中打开：" + absoluteLink
	htmlBody := "<h1>" + html.EscapeString(title) + "</h1><p>" + html.EscapeString(summary) +
		"</p><p>状态：" + html.EscapeString(resourceStatus) + "</p><p><a href=\"" + html.EscapeString(absoluteLink) +
		"\">在 HotKey 中打开</a></p>"
	return NotificationEmailMessageDTO{
		Recipient: claimed.RecipientEmail, Subject: "[HotKey] 热点事件更新", Text: text, HTML: htmlBody,
	}
}

func validateClaimedEmailDelivery(claimed ClaimedEmailDeliveryDTO, expectedToken string) error {
	if !claimed.Claimed || claimed.ClaimToken != expectedToken || claimed.AttemptCount < 0 ||
		claimed.AttemptCount >= MaximumEmailAttempts || claimed.PublishedConfigID <= 0 || claimed.PublishedRevision <= 0 ||
		!claimed.AlertEmailEnabled {
		return fmt.Errorf("claimed notification email identity is invalid")
	}
	if err := ValidateUserNotificationDTO(claimed.Notification); err != nil {
		return err
	}
	address, err := mail.ParseAddress(claimed.RecipientEmail)
	if err != nil || address.Address != claimed.RecipientEmail || strings.ContainsAny(claimed.RecipientEmail, "\r\n") {
		return fmt.Errorf("notification recipient is invalid")
	}
	return nil
}

func normalizeWebOrigin(value string) (string, error) {
	value = strings.TrimSuffix(strings.TrimSpace(value), "/")
	if value == "" {
		value = "http://127.0.0.1:8123"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("notification web origin must be an absolute HTTP(S) origin")
	}
	return value, nil
}

func cleanNotificationText(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return -1
		}
		return character
	}, strings.TrimSpace(value))
}

func newEmailClaimToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
