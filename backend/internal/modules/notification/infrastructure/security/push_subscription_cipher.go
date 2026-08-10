package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const pushSubscriptionEncryptionKeyVersion = 1

type PushSubscriptionCipher struct {
	aead cipher.AEAD
}

var _ application.PushSubscriptionCipher = (*PushSubscriptionCipher)(nil)
var _ application.PushSubscriptionSecretOpener = (*PushSubscriptionCipher)(nil)

func NewPushSubscriptionCipher(encodedKey string) (*PushSubscriptionCipher, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return &PushSubscriptionCipher{}, nil
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("push subscription encryption key must be Base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create push subscription cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create push subscription AEAD: %w", err)
	}
	return &PushSubscriptionCipher{aead: aead}, nil
}

func (cipher *PushSubscriptionCipher) Seal(_ context.Context, command application.SealPushSubscriptionSecretsCommand) (application.SealedPushSubscriptionSecretsDTO, error) {
	if cipher == nil || cipher.aead == nil {
		return application.SealedPushSubscriptionSecretsDTO{}, sharedrepository.ErrUnavailable
	}
	if len(command.EndpointSHA256) != 64 || command.Endpoint == "" || command.P256DH == "" || command.Auth == "" {
		return application.SealedPushSubscriptionSecretsDTO{}, sharedrepository.ErrInvalidInput
	}
	endpoint, err := cipher.seal(command.EndpointSHA256, "endpoint", command.Endpoint)
	if err != nil {
		return application.SealedPushSubscriptionSecretsDTO{}, err
	}
	p256dh, err := cipher.seal(command.EndpointSHA256, "p256dh", command.P256DH)
	if err != nil {
		return application.SealedPushSubscriptionSecretsDTO{}, err
	}
	auth, err := cipher.seal(command.EndpointSHA256, "auth", command.Auth)
	if err != nil {
		return application.SealedPushSubscriptionSecretsDTO{}, err
	}
	return application.SealedPushSubscriptionSecretsDTO{
		EndpointCiphertext: endpoint, P256DHCiphertext: p256dh, AuthCiphertext: auth,
		KeyVersion: pushSubscriptionEncryptionKeyVersion,
	}, nil
}

func (cipher *PushSubscriptionCipher) Open(_ context.Context, command application.OpenPushSubscriptionSecretsCommand) (application.OpenPushSubscriptionSecretsResult, error) {
	endpoint, err := cipher.open(command.EndpointSHA256, "endpoint", command.EndpointCiphertext, command.KeyVersion)
	if err != nil {
		return application.OpenPushSubscriptionSecretsResult{}, err
	}
	p256dh, err := cipher.open(command.EndpointSHA256, "p256dh", command.P256DHCiphertext, command.KeyVersion)
	if err != nil {
		return application.OpenPushSubscriptionSecretsResult{}, err
	}
	auth, err := cipher.open(command.EndpointSHA256, "auth", command.AuthCiphertext, command.KeyVersion)
	if err != nil {
		return application.OpenPushSubscriptionSecretsResult{}, err
	}
	return application.OpenPushSubscriptionSecretsResult{Endpoint: endpoint, P256DH: p256dh, Auth: auth}, nil
}

func (cipher *PushSubscriptionCipher) seal(endpointSHA256, field, value string) ([]byte, error) {
	nonce := make([]byte, cipher.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("create push subscription nonce: %w", err)
	}
	sealed := cipher.aead.Seal(nil, nonce, []byte(value), []byte(endpointSHA256+":"+field))
	return append(nonce, sealed...), nil
}

func (cipher *PushSubscriptionCipher) open(endpointSHA256, field string, ciphertext []byte, keyVersion int) (string, error) {
	if cipher == nil || cipher.aead == nil || keyVersion != pushSubscriptionEncryptionKeyVersion || len(ciphertext) <= cipher.aead.NonceSize() {
		return "", sharedrepository.ErrUnavailable
	}
	nonce, sealed := ciphertext[:cipher.aead.NonceSize()], ciphertext[cipher.aead.NonceSize():]
	plaintext, err := cipher.aead.Open(nil, nonce, sealed, []byte(endpointSHA256+":"+field))
	if err != nil {
		return "", sharedrepository.ErrConstraint
	}
	return string(plaintext), nil
}
