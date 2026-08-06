package queue

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPayloadRequiresVersionedBoundedEnvelope(t *testing.T) {
	valid := Payload{EntityID: 1, EntityVersion: 1, InputHash: "hash"}
	tests := []struct {
		name    string
		payload Payload
		wantErr bool
	}{
		{name: "valid", payload: valid},
		{name: "missing entity", payload: Payload{EntityVersion: 1}, wantErr: true},
		{name: "missing version", payload: Payload{EntityID: 1}, wantErr: true},
		{name: "oversized hash", payload: Payload{EntityID: 1, EntityVersion: 1, InputHash: strings.Repeat("x", 129)}, wantErr: true},
		{name: "invalid window", payload: Payload{EntityID: 1, EntityVersion: 1, WindowStart: time.Unix(20, 0), WindowEnd: time.Unix(10, 0)}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.payload.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestJobRejectsUnknownKindAndUnboundedKey(t *testing.T) {
	base := Job{
		Kind:        KindCollectSource,
		UniqueKey:   "stable-key",
		Payload:     Payload{EntityID: 1, EntityVersion: 1},
		ScheduledAt: time.Now().UTC(),
		MaxAttempts: 3,
		Priority:    1,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid job rejected: %v", err)
	}
	unknown := base
	unknown.Kind = "unknown"
	if err := unknown.Validate(); err == nil {
		t.Fatal("unknown kind was accepted")
	}
	oversized := base
	oversized.UniqueKey = strings.Repeat("x", MaxUniqueKeyBytes+1)
	if err := oversized.Validate(); err == nil {
		t.Fatal("oversized unique key was accepted")
	}
}

func TestErrorClassificationPreservesPermanentAndCancellation(t *testing.T) {
	if !IsRetryable(NewRetryableError(errors.New("temporary"))) {
		t.Fatal("retryable error was not classified as retryable")
	}
	if !IsPermanent(NewPermanentError(errors.New("invalid"))) {
		t.Fatal("permanent error was not classified as permanent")
	}
	if !IsCancelled(NewCancelledError(errors.New("cancelled"))) {
		t.Fatal("cancelled error was not classified as cancelled")
	}
}
