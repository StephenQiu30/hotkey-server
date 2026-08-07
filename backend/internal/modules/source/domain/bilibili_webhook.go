package domain

import (
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const BilibiliWebhookMaxBodyBytes = 256 << 10

type BilibiliWebhook struct {
	Event  string
	OpenID string
	Echo   json.RawMessage
	Digest string
}

type bilibiliWebhookEnvelope struct {
	Event   string `json:"event"`
	Content struct {
		OpenID string          `json:"openid"`
		Data   json.RawMessage `json:"data"`
	} `json:"content"`
}

// VerifyBilibiliWebhook authenticates the exact raw bytes before decoding.
func VerifyBilibiliWebhook(body []byte, signature, secret string) (BilibiliWebhook, error) {
	if len(body) == 0 || len(body) > BilibiliWebhookMaxBodyBytes || strings.TrimSpace(secret) == "" {
		return BilibiliWebhook{}, errors.New("invalid Bilibili webhook")
	}
	expectedHash := sha1.Sum(append([]byte(secret), body...)) // #nosec G505 -- mandated by Bilibili's webhook protocol.
	expected := hex.EncodeToString(expectedHash[:])
	provided := strings.ToLower(strings.TrimSpace(signature))
	if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return BilibiliWebhook{}, errors.New("invalid Bilibili webhook signature")
	}
	var envelope bilibiliWebhookEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return BilibiliWebhook{}, errors.New("invalid Bilibili webhook body")
	}
	payloadHash := sha1.Sum(body) // #nosec G505 -- non-security idempotency digest.
	result := BilibiliWebhook{Event: strings.TrimSpace(envelope.Event), OpenID: strings.TrimSpace(envelope.Content.OpenID), Echo: envelope.Content.Data, Digest: hex.EncodeToString(payloadHash[:])}
	switch result.Event {
	case "verify_webhooks":
		if len(result.Echo) == 0 || string(result.Echo) == "null" {
			return BilibiliWebhook{}, errors.New("Bilibili webhook challenge is missing")
		}
	case "deauthorize":
		if result.OpenID == "" || len(result.OpenID) > 128 {
			return BilibiliWebhook{}, errors.New("Bilibili webhook OpenID is invalid")
		}
	default:
		return BilibiliWebhook{}, errors.New("unsupported Bilibili webhook event")
	}
	return result, nil
}

type BilibiliWebhookRepository interface {
	LockBilibiliByOpenID(context.Context, string) (*SourceConnection, error)
	CreateBilibiliWebhookReceipt(context.Context, BilibiliWebhook) (bool, error)
}
