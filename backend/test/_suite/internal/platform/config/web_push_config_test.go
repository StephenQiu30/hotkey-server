package config

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestNotificationConfigAcceptsCompleteWebPushAndRejectsPartialOrUnsafeSettings(t *testing.T) {
	valid := NotificationConfig{
		VAPIDPublicKey:                base64.RawURLEncoding.EncodeToString(append([]byte{4}, bytes.Repeat([]byte{1}, 64)...)),
		VAPIDPrivateKey:               base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
		VAPIDSubject:                  "mailto:push@example.test",
		PushSubscriptionEncryptionKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)),
	}
	if err := valid.ValidateWebPushRuntime(); err != nil {
		t.Fatalf("complete Web Push configuration error = %v", err)
	}
	for _, mutate := range []func(*NotificationConfig){
		func(value *NotificationConfig) { value.VAPIDPrivateKey = "" },
		func(value *NotificationConfig) { value.VAPIDPublicKey = "not-base64" },
		func(value *NotificationConfig) { value.VAPIDSubject = "http://push.example.test" },
		func(value *NotificationConfig) {
			value.PushSubscriptionEncryptionKey = base64.StdEncoding.EncodeToString([]byte("short"))
		},
	} {
		fixture := valid
		mutate(&fixture)
		if err := fixture.ValidateWebPushRuntime(); err == nil {
			t.Fatalf("invalid Web Push configuration was accepted: %#v", fixture)
		}
	}
	if err := (NotificationConfig{}).ValidateWebPushRuntime(); err != nil {
		t.Fatalf("disabled Web Push configuration error = %v", err)
	}
}
