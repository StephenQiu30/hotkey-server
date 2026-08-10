package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	MinimumPushTTLSeconds = 60
	MaximumPushTTLSeconds = 86400
	MaximumPushMonitors   = 100
)

type PushSubscriptionStatus string

const (
	PushSubscriptionActive   PushSubscriptionStatus = "active"
	PushSubscriptionDisabled PushSubscriptionStatus = "disabled"
	PushSubscriptionExpired  PushSubscriptionStatus = "expired"
)

func (status PushSubscriptionStatus) Valid() bool {
	return status == PushSubscriptionActive || status == PushSubscriptionDisabled || status == PushSubscriptionExpired
}

type PushSubscription struct {
	ID                   int64
	Version              int64
	UserID               int64
	Endpoint             string
	P256DH               string
	Auth                 string
	DeviceLabel          string
	Timezone             string
	QuietStart           *string
	QuietEnd             *string
	TTLSeconds           int
	Status               PushSubscriptionStatus
	ExpirationReason     string
	MonitorIDs           []int64
	IdempotencyKey       string
	CommandFingerprint   string
	EncryptionKeyVersion int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type PushSubscriptionPreferences struct {
	DeviceLabel string
	Timezone    string
	QuietStart  *string
	QuietEnd    *string
	TTLSeconds  int
	MonitorIDs  []int64
}

var pushIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,95}$`)

func NormalizePushSubscription(subscription PushSubscription) (PushSubscription, error) {
	subscription.Endpoint = strings.TrimSpace(subscription.Endpoint)
	subscription.P256DH = strings.TrimSpace(subscription.P256DH)
	subscription.Auth = strings.TrimSpace(subscription.Auth)
	subscription.DeviceLabel = strings.TrimSpace(subscription.DeviceLabel)
	subscription.Timezone = strings.TrimSpace(subscription.Timezone)
	subscription.IdempotencyKey = strings.TrimSpace(subscription.IdempotencyKey)
	if subscription.Status == "" {
		subscription.Status = PushSubscriptionActive
	}
	if subscription.TTLSeconds == 0 {
		subscription.TTLSeconds = 3600
	}
	if subscription.Timezone == "" {
		subscription.Timezone = "UTC"
	}
	preferences, err := NormalizePushSubscriptionPreferences(PushSubscriptionPreferences{
		DeviceLabel: subscription.DeviceLabel, Timezone: subscription.Timezone, QuietStart: subscription.QuietStart,
		QuietEnd: subscription.QuietEnd, TTLSeconds: subscription.TTLSeconds, MonitorIDs: subscription.MonitorIDs,
	})
	if err != nil {
		return PushSubscription{}, err
	}
	subscription.DeviceLabel, subscription.Timezone = preferences.DeviceLabel, preferences.Timezone
	subscription.QuietStart, subscription.QuietEnd = preferences.QuietStart, preferences.QuietEnd
	subscription.TTLSeconds, subscription.MonitorIDs = preferences.TTLSeconds, preferences.MonitorIDs
	if err := subscription.Validate(); err != nil {
		return PushSubscription{}, err
	}
	return subscription, nil
}

func NormalizePushSubscriptionPreferences(preferences PushSubscriptionPreferences) (PushSubscriptionPreferences, error) {
	preferences.DeviceLabel = strings.TrimSpace(preferences.DeviceLabel)
	preferences.Timezone = strings.TrimSpace(preferences.Timezone)
	if preferences.Timezone == "" {
		preferences.Timezone = "UTC"
	}
	if preferences.TTLSeconds == 0 {
		preferences.TTLSeconds = 3600
	}
	monitorIDs := append([]int64(nil), preferences.MonitorIDs...)
	sort.Slice(monitorIDs, func(left, right int) bool { return monitorIDs[left] < monitorIDs[right] })
	for index := 1; index < len(monitorIDs); index++ {
		if monitorIDs[index] == monitorIDs[index-1] {
			return PushSubscriptionPreferences{}, fmt.Errorf("push monitor ids must be unique")
		}
	}
	preferences.MonitorIDs = monitorIDs
	if preferences.DeviceLabel == "" || len([]byte(preferences.DeviceLabel)) > 80 || hasUnsafeControl(preferences.DeviceLabel) {
		return PushSubscriptionPreferences{}, fmt.Errorf("push device label is invalid")
	}
	if _, err := time.LoadLocation(preferences.Timezone); err != nil || len(preferences.Timezone) > 64 {
		return PushSubscriptionPreferences{}, fmt.Errorf("push timezone is invalid")
	}
	if (preferences.QuietStart == nil) != (preferences.QuietEnd == nil) {
		return PushSubscriptionPreferences{}, fmt.Errorf("push quiet hours must be supplied together")
	}
	if preferences.QuietStart != nil {
		if !validClockTime(*preferences.QuietStart) || !validClockTime(*preferences.QuietEnd) || *preferences.QuietStart == *preferences.QuietEnd {
			return PushSubscriptionPreferences{}, fmt.Errorf("push quiet hours are invalid")
		}
	}
	if preferences.TTLSeconds < MinimumPushTTLSeconds || preferences.TTLSeconds > MaximumPushTTLSeconds {
		return PushSubscriptionPreferences{}, fmt.Errorf("push TTL is invalid")
	}
	if len(preferences.MonitorIDs) == 0 || len(preferences.MonitorIDs) > MaximumPushMonitors {
		return PushSubscriptionPreferences{}, fmt.Errorf("push monitor selection is invalid")
	}
	for _, monitorID := range preferences.MonitorIDs {
		if monitorID <= 0 {
			return PushSubscriptionPreferences{}, fmt.Errorf("push monitor selection is invalid")
		}
	}
	return preferences, nil
}

func (subscription PushSubscription) Validate() error {
	if subscription.UserID <= 0 || !subscription.Status.Valid() || subscription.EncryptionKeyVersion < 0 {
		return fmt.Errorf("push subscription identity is invalid")
	}
	if subscription.ID < 0 || subscription.Version < 0 {
		return fmt.Errorf("push subscription persisted identity is invalid")
	}
	if err := validatePushEndpoint(subscription.Endpoint); err != nil {
		return err
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(subscription.P256DH); err != nil || len(decoded) != 65 || decoded[0] != 4 {
		return fmt.Errorf("push p256dh key is invalid")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(subscription.Auth); err != nil || len(decoded) != 16 {
		return fmt.Errorf("push auth secret is invalid")
	}
	if _, err := NormalizePushSubscriptionPreferences(PushSubscriptionPreferences{
		DeviceLabel: subscription.DeviceLabel, Timezone: subscription.Timezone, QuietStart: subscription.QuietStart,
		QuietEnd: subscription.QuietEnd, TTLSeconds: subscription.TTLSeconds, MonitorIDs: subscription.MonitorIDs,
	}); err != nil {
		return err
	}
	if !pushIdempotencyKey.MatchString(subscription.IdempotencyKey) {
		return fmt.Errorf("push idempotency key is invalid")
	}
	if subscription.CommandFingerprint != "" && !lowerHex64(subscription.CommandFingerprint) {
		return fmt.Errorf("push command fingerprint is invalid")
	}
	if subscription.Status == PushSubscriptionExpired && strings.TrimSpace(subscription.ExpirationReason) == "" ||
		subscription.Status != PushSubscriptionExpired && subscription.ExpirationReason != "" {
		return fmt.Errorf("push expiration state is invalid")
	}
	return nil
}

func PushEndpointSHA256(endpoint string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(endpoint)))
	return hex.EncodeToString(digest[:])
}

func PushSubscriptionFingerprint(subscription PushSubscription) string {
	quietStart, quietEnd := "", ""
	if subscription.QuietStart != nil {
		quietStart, quietEnd = *subscription.QuietStart, *subscription.QuietEnd
	}
	monitorParts := make([]string, 0, len(subscription.MonitorIDs))
	for _, monitorID := range subscription.MonitorIDs {
		monitorParts = append(monitorParts, fmt.Sprintf("%d", monitorID))
	}
	value := strings.Join([]string{
		subscription.Endpoint, subscription.P256DH, subscription.Auth, subscription.DeviceLabel,
		subscription.Timezone, quietStart, quietEnd, fmt.Sprintf("%d", subscription.TTLSeconds),
		strings.Join(monitorParts, ","),
	}, "\n")
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validatePushEndpoint(value string) error {
	if value == "" || len([]byte(value)) > 4096 || hasUnsafeControl(value) {
		return fmt.Errorf("push endpoint is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("push endpoint is invalid")
	}
	return nil
}

func validClockTime(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	_, err := time.Parse("15:04", value)
	return err == nil
}

func hasUnsafeControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func lowerHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
