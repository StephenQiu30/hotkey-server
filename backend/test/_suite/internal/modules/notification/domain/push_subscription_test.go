package domain

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestPushSubscriptionNormalizesMonitorSelectionAndFreezesFingerprint(t *testing.T) {
	subscription := validPushSubscription()
	subscription.MonitorIDs = []int64{9, 3}
	normalized, err := NormalizePushSubscription(subscription)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.MonitorIDs[0] != 3 || normalized.MonitorIDs[1] != 9 ||
		PushSubscriptionFingerprint(normalized) == "" || PushEndpointSHA256(normalized.Endpoint) == "" {
		t.Fatalf("normalized subscription = %#v", normalized)
	}
	reordered := normalized
	reordered.MonitorIDs = []int64{9, 3}
	reordered, err = NormalizePushSubscription(reordered)
	if err != nil || PushSubscriptionFingerprint(reordered) != PushSubscriptionFingerprint(normalized) {
		t.Fatalf("reordered subscription = %#v / %v", reordered, err)
	}
}

func TestPushSubscriptionRejectsUnsafeEndpointKeysQuietHoursAndDuplicates(t *testing.T) {
	fixtures := []func(*PushSubscription){
		func(value *PushSubscription) { value.Endpoint = "http://push.example/subscription" },
		func(value *PushSubscription) { value.Endpoint = "https://user:secret@push.example/subscription" },
		func(value *PushSubscription) {
			value.P256DH = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 64))
		},
		func(value *PushSubscription) { value.QuietStart, value.QuietEnd = pointer("22:00"), pointer("22:00") },
		func(value *PushSubscription) { value.MonitorIDs = []int64{3, 3} },
	}
	for index, mutate := range fixtures {
		value := validPushSubscription()
		mutate(&value)
		if _, err := NormalizePushSubscription(value); err == nil {
			t.Fatalf("fixture %d accepted", index)
		}
	}
}

func validPushSubscription() PushSubscription {
	p256dh := append([]byte{4}, bytes.Repeat([]byte{7}, 64)...)
	return PushSubscription{
		UserID: 1, Endpoint: "https://push.example/subscription/one",
		P256DH: base64.RawURLEncoding.EncodeToString(p256dh), Auth: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 16)),
		DeviceLabel: "iPhone", Timezone: "Asia/Shanghai", QuietStart: pointer("22:00"), QuietEnd: pointer("07:00"),
		TTLSeconds: 3600, Status: PushSubscriptionActive, MonitorIDs: []int64{3},
		IdempotencyKey: "push-register-fixture-1",
	}
}

func pointer(value string) *string { return &value }
