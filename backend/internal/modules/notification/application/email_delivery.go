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
	"regexp"
	"strconv"
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
	ClaimToken           string
	LeaseDuration        time.Duration
	ProviderCapabilities NotificationEmailProviderCapabilities
}

type ClaimedEmailDeliveryDTO struct {
	Claimed              bool
	ClaimToken           string
	AttemptCount         int
	RecipientEmail       string
	Notification         UserNotificationDTO
	PublishedConfigID    int64
	PublishedRevision    int64
	AlertEmailEnabled    bool
	MonitorName          string
	SourceName           string
	SourceType           string
	RelevanceScore       *float64
	OriginalURL          string
	FencingGeneration    int64
	DispatchKey          string
	ReconcileRequired    bool
	RecoveredUnknown     bool
	ProviderCapabilities NotificationEmailProviderCapabilities
}

type StartEmailDeliveryCommand struct {
	UserNotificationID int64
	UserID             int64
	ClaimToken         string
	FencingGeneration  int64
	DispatchKey        string
}

type CompleteEmailDeliveryCommand struct {
	UserNotificationID   int64
	UserID               int64
	ClaimToken           string
	FencingGeneration    int64
	DispatchKey          string
	ProviderCapabilities NotificationEmailProviderCapabilities
	Status               string
	ProviderMessageID    string
	ResponseCode         *int
	ErrorCode            string
}

type EmailDeliveryRepository interface {
	ClaimNextEmailDelivery(context.Context, ClaimNextEmailDeliveryCommand) (ClaimedEmailDeliveryDTO, error)
	StartEmailDelivery(context.Context, StartEmailDeliveryCommand) error
	CompleteEmailDelivery(context.Context, CompleteEmailDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error)
}

type NotificationEmailMessageDTO struct {
	Recipient string
	Subject   string
	Text      string
	HTML      string
}

type NotificationEmailProviderCapabilities struct {
	SupportsIdempotency   bool
	SupportsReceiptLookup bool
}

type NotificationEmailDispatchDTO struct {
	DispatchKey string
	Message     NotificationEmailMessageDTO
}

type NotificationEmailReceiptDTO struct {
	Found             bool
	ProviderMessageID string
}

type NotificationEmailSender interface {
	Capabilities() NotificationEmailProviderCapabilities
	SendNotificationEmail(context.Context, NotificationEmailDispatchDTO) (string, error)
	LookupNotificationEmail(context.Context, string) (NotificationEmailReceiptDTO, error)
}

type EmailDeliveryServiceDependencies struct {
	Repository EmailDeliveryRepository
	Sender     NotificationEmailSender
	NewToken   func() (string, error)
	WebOrigin  string
}

type EmailDeliveryService struct {
	repository EmailDeliveryRepository
	sender     NotificationEmailSender
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
	if dependencies.NewToken == nil {
		dependencies.NewToken = newEmailClaimToken
	}
	origin, err := normalizeWebOrigin(dependencies.WebOrigin)
	if err != nil {
		return nil, err
	}
	return &EmailDeliveryService{
		repository: dependencies.Repository, sender: dependencies.Sender,
		newToken: dependencies.NewToken, webOrigin: origin,
	}, nil
}

// DispatchNext claims at most one durable user notification. Provider outcomes
// become immutable DeliveryAttempt facts; uncertain outcomes either reconcile
// under a stable provider key or stop as unknown without mutating the notification.
func (service *EmailDeliveryService) DispatchNext(ctx context.Context) (DispatchEmailDeliveryResult, error) {
	if service == nil {
		return DispatchEmailDeliveryResult{}, sharedrepository.ErrUnavailable
	}
	token, err := service.newToken()
	if err != nil {
		return DispatchEmailDeliveryResult{}, fmt.Errorf("create notification email claim: %w", err)
	}
	capabilities := service.sender.Capabilities()
	claimed, err := service.repository.ClaimNextEmailDelivery(ctx, ClaimNextEmailDeliveryCommand{
		ClaimToken: token, LeaseDuration: EmailDeliveryLeaseDuration, ProviderCapabilities: capabilities,
	})
	if err != nil {
		return DispatchEmailDeliveryResult{}, err
	}
	if !claimed.Claimed {
		return DispatchEmailDeliveryResult{}, nil
	}
	result := DispatchEmailDeliveryResult{Claimed: true, UserNotificationID: claimed.Notification.ID}
	if claimed.RecoveredUnknown {
		result.Status, result.AttemptNo = "unknown", claimed.AttemptCount
		return result, nil
	}
	if err := validateClaimedEmailDeliveryFence(claimed, token, capabilities); err != nil {
		return result, err
	}
	if err := service.repository.StartEmailDelivery(ctx, StartEmailDeliveryCommand{
		UserNotificationID: claimed.Notification.ID, UserID: claimed.Notification.UserID, ClaimToken: token,
		FencingGeneration: claimed.FencingGeneration, DispatchKey: claimed.DispatchKey,
	}); err != nil {
		return result, err
	}
	status, errorCode, providerMessageID := "succeeded", "", ""
	if err := validateClaimedEmailDelivery(claimed); err != nil {
		status, errorCode = "permanent_failure", "invalid_notification_projection"
	} else {
		if claimed.ReconcileRequired && capabilities.SupportsReceiptLookup {
			receipt, lookupErr := service.sender.LookupNotificationEmail(ctx, claimed.DispatchKey)
			if lookupErr != nil && !capabilities.SupportsIdempotency {
				status, errorCode = "unknown", "provider_receipt_unavailable"
				completed, completeErr := service.completeEmailDelivery(ctx, claimed, token, status, providerMessageID, errorCode)
				if completeErr != nil {
					return result, completeErr
				}
				result.Status, result.AttemptNo = status, completed.AttemptNo
				return result, nil
			}
			if lookupErr == nil && receipt.Found {
				providerMessageID = receipt.ProviderMessageID
				completed, completeErr := service.completeEmailDelivery(ctx, claimed, token, status, providerMessageID, errorCode)
				if completeErr != nil {
					return result, completeErr
				}
				result.Status, result.AttemptNo = status, completed.AttemptNo
				return result, nil
			}
		}
		dispatch := NotificationEmailDispatchDTO{DispatchKey: claimed.DispatchKey, Message: service.message(claimed)}
		providerMessageID, err = service.sender.SendNotificationEmail(ctx, dispatch)
		if err != nil {
			if !emailDeliveryOutcomeKnown(err) {
				if capabilities.SupportsIdempotency || capabilities.SupportsReceiptLookup {
					return result, fmt.Errorf("notification email provider outcome awaits reconciliation: %w", err)
				}
				status, errorCode = "unknown", "provider_outcome_unconfirmed"
			} else {
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
	}
	completed, err := service.completeEmailDelivery(ctx, claimed, token, status, providerMessageID, errorCode)
	if err != nil {
		return result, err
	}
	result.Status, result.AttemptNo = status, completed.AttemptNo
	return result, nil
}

func (service *EmailDeliveryService) completeEmailDelivery(ctx context.Context, claimed ClaimedEmailDeliveryDTO, token string, status string, providerMessageID string, errorCode string) (RecordNotificationDeliveryAttemptResult, error) {
	return service.repository.CompleteEmailDelivery(ctx, CompleteEmailDeliveryCommand{
		UserNotificationID: claimed.Notification.ID, UserID: claimed.Notification.UserID, ClaimToken: token,
		FencingGeneration: claimed.FencingGeneration, DispatchKey: claimed.DispatchKey,
		ProviderCapabilities: claimed.ProviderCapabilities,
		Status:               status, ProviderMessageID: providerMessageID, ErrorCode: errorCode,
	})
}

func (service *EmailDeliveryService) message(claimed ClaimedEmailDeliveryDTO) NotificationEmailMessageDTO {
	notification := claimed.Notification
	absoluteLink := service.webOrigin + notification.DeepLink
	title := cleanNotificationText(notification.Title)
	summary := cleanNotificationText(notification.Summary)
	monitorName := cleanNotificationText(claimed.MonitorName)
	sourceName := cleanNotificationText(claimed.SourceName)
	statusLabel := notificationSeverityLabel(notification.ResourceStatus)
	subjectTitle := notificationSubjectText(title)
	if monitorName == "" {
		monitorName = "监控"
	}
	textLines := []string{title, "", summary, "", "级别：" + statusLabel, "监控：" + monitorName}
	detailRows := []string{
		emailDetailRow("级别", statusLabel),
		emailDetailRow("监控", monitorName),
	}
	actionLinks := make([]string, 0, 2)
	if sourceName != "" {
		textLines = append(textLines, "来源："+sourceName)
		detailRows = append(detailRows, emailDetailRow("来源", sourceName))
	}
	if claimed.RelevanceScore != nil {
		relevance := strconv.FormatFloat(*claimed.RelevanceScore, 'f', 1, 64) + "%"
		textLines = append(textLines, "相关度："+relevance)
		detailRows = append(detailRows, emailDetailRow("相关度", relevance))
	}
	if originalURL := safeNotificationOriginalURL(claimed.OriginalURL); originalURL != "" {
		textLines = append(textLines, "原文："+originalURL)
		actionLinks = append(actionLinks, "<p><a href=\""+html.EscapeString(originalURL)+"\">查看原文</a></p>")
	}
	textLines = append(textLines, "在 HotKey 中打开："+absoluteLink)
	htmlBody := "<h1>" + html.EscapeString(title) + "</h1><p>" + html.EscapeString(summary) +
		"</p><dl>" + strings.Join(detailRows, "") + "</dl>" + strings.Join(actionLinks, "") +
		"<p><a href=\"" + html.EscapeString(absoluteLink) + "\">在 HotKey 中打开</a></p>"
	return NotificationEmailMessageDTO{
		Recipient: claimed.RecipientEmail,
		Subject:   "[HotKey][" + statusLabel + "] " + monitorName + " · " + subjectTitle,
		Text:      strings.Join(textLines, "\n"), HTML: htmlBody,
	}
}

func validateClaimedEmailDeliveryFence(claimed ClaimedEmailDeliveryDTO, expectedToken string, capabilities NotificationEmailProviderCapabilities) error {
	if !claimed.Claimed || claimed.RecoveredUnknown || claimed.ClaimToken != expectedToken || claimed.FencingGeneration <= 0 ||
		!lowerHexEmailDispatchKey.MatchString(claimed.DispatchKey) || claimed.ProviderCapabilities != capabilities ||
		claimed.ReconcileRequired && !capabilities.SupportsIdempotency && !capabilities.SupportsReceiptLookup {
		return fmt.Errorf("claimed notification email fence is invalid")
	}
	return nil
}

func validateClaimedEmailDelivery(claimed ClaimedEmailDeliveryDTO) error {
	if claimed.AttemptCount < 0 || claimed.AttemptCount >= MaximumEmailAttempts || claimed.PublishedConfigID <= 0 ||
		claimed.PublishedRevision <= 0 || !claimed.AlertEmailEnabled {
		return fmt.Errorf("claimed notification email identity is invalid")
	}
	if err := ValidateUserNotificationDTO(claimed.Notification); err != nil {
		return err
	}
	if claimed.RelevanceScore != nil && (*claimed.RelevanceScore < 0 || *claimed.RelevanceScore > 100) {
		return fmt.Errorf("notification relevance score is invalid")
	}
	address, err := mail.ParseAddress(claimed.RecipientEmail)
	if err != nil || address.Address != claimed.RecipientEmail || strings.ContainsAny(claimed.RecipientEmail, "\r\n") {
		return fmt.Errorf("notification recipient is invalid")
	}
	return nil
}

var lowerHexEmailDispatchKey = regexp.MustCompile(`^[0-9a-f]{64}$`)

func emailDeliveryOutcomeKnown(err error) bool {
	var known interface{ DeliveryOutcomeKnown() bool }
	return errors.As(err, &known) && known.DeliveryOutcomeKnown()
}

func normalizeWebOrigin(value string) (string, error) {
	value = strings.TrimSuffix(strings.TrimSpace(value), "/")
	if value == "" {
		value = "http://127.0.0.1:8010"
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

var notificationHTMLTag = regexp.MustCompile(`<[^>]*>`)

func notificationSubjectText(value string) string {
	value = html.UnescapeString(notificationHTMLTag.ReplaceAllString(value, ""))
	value = strings.Join(strings.Fields(cleanNotificationText(value)), " ")
	if value == "" {
		return "热点更新"
	}
	characters := []rune(value)
	if len(characters) > 120 {
		return string(characters[:120]) + "…"
	}
	return value
}

func notificationSeverityLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "urgent":
		return "紧急"
	case "high":
		return "高"
	default:
		return cleanNotificationText(value)
	}
}

func safeNotificationOriginalURL(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.Scheme != "http" && parsed.Scheme != "https" ||
		strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return parsed.String()
}

func emailDetailRow(label string, value string) string {
	return "<dt>" + html.EscapeString(label) + "</dt><dd>" + html.EscapeString(value) + "</dd>"
}

func newEmailClaimToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
