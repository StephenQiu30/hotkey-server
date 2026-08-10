package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

func TestPushSubscriptionCipherRoundTripAndTamperDetection(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewPushSubscriptionCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	sealed, err := cipher.Seal(context.Background(), application.SealPushSubscriptionSecretsCommand{
		EndpointSHA256: digest, Endpoint: "https://push.example/subscription/one", P256DH: "p256dh-secret", Auth: "auth-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed.EndpointCiphertext), "push.example") || strings.Contains(string(sealed.P256DHCiphertext), "p256dh-secret") {
		t.Fatal("ciphertext contains plaintext")
	}
	opened, err := cipher.Open(context.Background(), application.OpenPushSubscriptionSecretsCommand{
		EndpointSHA256: digest, EndpointCiphertext: sealed.EndpointCiphertext, P256DHCiphertext: sealed.P256DHCiphertext,
		AuthCiphertext: sealed.AuthCiphertext, KeyVersion: sealed.KeyVersion,
	})
	if err != nil || opened.Endpoint != "https://push.example/subscription/one" || opened.P256DH != "p256dh-secret" || opened.Auth != "auth-secret" {
		t.Fatalf("Open() = %#v / %v", opened, err)
	}
	tampered := append([]byte(nil), sealed.EndpointCiphertext...)
	tampered[len(tampered)-1] ^= 1
	if _, err := cipher.Open(context.Background(), application.OpenPushSubscriptionSecretsCommand{
		EndpointSHA256: digest, EndpointCiphertext: tampered, P256DHCiphertext: sealed.P256DHCiphertext,
		AuthCiphertext: sealed.AuthCiphertext, KeyVersion: sealed.KeyVersion,
	}); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}
