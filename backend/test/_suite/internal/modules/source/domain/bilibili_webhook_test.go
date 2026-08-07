package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

func TestVerifyBilibiliWebhookAuthenticatesAndParsesOfficialEvents(t *testing.T) {
	body := []byte(`{"event":"deauthorize","content":{"openid":"creator_open_id"}}`)
	digest := sha1.Sum(append([]byte("fixture-secret"), body...))
	webhook, err := VerifyBilibiliWebhook(body, hex.EncodeToString(digest[:]), "fixture-secret")
	if err != nil || webhook.Event != "deauthorize" || webhook.OpenID != "creator_open_id" || len(webhook.Digest) != 40 {
		t.Fatalf("VerifyBilibiliWebhook() = %#v, %v", webhook, err)
	}
	if _, err := VerifyBilibiliWebhook(body, "0000000000000000000000000000000000000000", "fixture-secret"); err == nil {
		t.Fatal("wrong signature must be rejected")
	}
}

func TestVerifyBilibiliWebhookReturnsChallengePayload(t *testing.T) {
	body := []byte(`{"event":"verify_webhooks","content":{"data":"official-challenge"}}`)
	digest := sha1.Sum(append([]byte("fixture-secret"), body...))
	webhook, err := VerifyBilibiliWebhook(body, hex.EncodeToString(digest[:]), "fixture-secret")
	if err != nil || string(webhook.Echo) != `"official-challenge"` {
		t.Fatalf("VerifyBilibiliWebhook() = %#v, %v", webhook, err)
	}
}

func TestNormalizeBilibiliSourceRequiresOfficialAuthorizationContract(t *testing.T) {
	config := DefaultSourceConfig()
	config.RequiresAttribution, config.RequiresDeletionSync = true, true
	config.BilibiliOpenID = "creator_open_id"
	connection, err := NormalizeSourceConnection(SourceConnection{
		SourceType: SourceTypeBilibili, Name: "Bilibili 官方账号", Endpoint: BilibiliOpenEndpoint,
		AuthType: AuthTypeOAuth2, CredentialRef: "env:BILIBILI_OAUTH", Config: config,
		TermsPolicyURL: "https://openhome.bilibili.com/agreement/privacy-policy",
	})
	if err != nil || connection.Config.BilibiliOpenID != "creator_open_id" {
		t.Fatalf("NormalizeSourceConnection() = %#v, %v", connection, err)
	}
	connection.Config.BilibiliOpenID = ""
	if _, err := NormalizeSourceConnection(connection); err == nil {
		t.Fatal("missing authorized OpenID must be rejected")
	}
}
