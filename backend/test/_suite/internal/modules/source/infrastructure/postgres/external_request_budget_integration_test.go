package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
)

func TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	connection := sourceConnection("rss-budget")
	connection.SourceType = sourcedomain.SourceTypeRSS
	connection.Endpoint = "https://feeds.example.test/rss"
	connection.AuthType = sourcedomain.AuthTypeNone
	connection.CredentialRef = ""
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatal(err)
	}
	budget := sourcepostgres.NewExternalRequestBudget(runtime)
	now := time.Date(2026, time.August, 27, 23, 59, 0, 0, time.UTC)
	reservation := sourcedomain.ExternalRequestBudgetReservation{
		SourceConnectionID: connection.ID, ResourceProfileVersion: "rss-resource-limits-v1",
		DailyLimit: 2, PerMinuteLimit: 2, At: now,
	}
	for index, wantAllowed := range []bool{true, true, false} {
		decision, err := budget.ReserveExternalRequest(context.Background(), reservation)
		if err != nil || decision.Allowed != wantAllowed || decision.Used != int64(min(index+1, 2)) {
			t.Fatalf("reservation %d = %#v/%v, want allowed=%t", index+1, decision, err, wantAllowed)
		}
		if !decision.ResetAt.Equal(time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("reset = %s", decision.ResetAt)
		}
	}
	nextDay := reservation
	nextDay.At = now.Add(2 * time.Minute)
	decision, err := budget.ReserveExternalRequest(context.Background(), nextDay)
	if err != nil || !decision.Allowed || decision.Used != 1 {
		t.Fatalf("next UTC day reservation = %#v/%v", decision, err)
	}

	concurrent := reservation
	concurrent.ResourceProfileVersion = "rss-resource-limits-v2"
	concurrent.DailyLimit = 5
	concurrent.PerMinuteLimit = 20
	var allowed atomic.Int64
	errors := make(chan error, 20)
	var workers sync.WaitGroup
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			decision, err := budget.ReserveExternalRequest(context.Background(), concurrent)
			if err != nil {
				errors <- err
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent reservation: %v", err)
	}
	if allowed.Load() != concurrent.DailyLimit {
		t.Fatalf("concurrent allowed = %d, want %d", allowed.Load(), concurrent.DailyLimit)
	}
	var used int64
	if err := runtime.SQL.QueryRow(`SELECT used FROM source_request_usage_ledgers WHERE source_connection_id=$1 AND resource_profile_version=$2 AND budget_day=$3`, connection.ID, concurrent.ResourceProfileVersion, now.Format(time.DateOnly)).Scan(&used); err != nil || used != concurrent.DailyLimit {
		t.Fatalf("concurrent persisted usage = %d/%v", used, err)
	}
}

func TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	connection := sourceConnection("rss-rate-limit")
	connection.SourceType = sourcedomain.SourceTypeRSS
	connection.Endpoint = "https://feeds.example.test/rate-limit"
	connection.AuthType = sourcedomain.AuthTypeNone
	connection.CredentialRef = ""
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatal(err)
	}

	budget := sourcepostgres.NewExternalRequestBudget(runtime)
	window := time.Date(2026, time.August, 29, 12, 34, 0, 0, time.UTC)
	reservation := sourcedomain.ExternalRequestBudgetReservation{
		SourceConnectionID: connection.ID, ResourceProfileVersion: "rss-resource-limits-v1",
		DailyLimit: 100, PerMinuteLimit: 5, At: window,
	}
	preflight, err := budget.CheckExternalRequest(context.Background(), reservation)
	if err != nil || !preflight.Allowed || preflight.Used != 0 || preflight.RateUsed != 0 {
		t.Fatalf("empty preflight = %#v/%v", preflight, err)
	}
	var allowed atomic.Int64
	errors := make(chan error, 20)
	var workers sync.WaitGroup
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			decision, err := budget.ReserveExternalRequest(context.Background(), reservation)
			if err != nil {
				errors <- err
				return
			}
			if decision.Allowed {
				allowed.Add(1)
				return
			}
			if !decision.ResetAt.Equal(window.Add(time.Minute)) {
				errors <- fmt.Errorf("rate reset = %s", decision.ResetAt)
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent minute reservation: %v", err)
	}
	if allowed.Load() != reservation.PerMinuteLimit {
		t.Fatalf("concurrent minute allowed = %d, want %d", allowed.Load(), reservation.PerMinuteLimit)
	}
	exhausted, err := budget.CheckExternalRequest(context.Background(), reservation)
	if err != nil || exhausted.Allowed || exhausted.Used != reservation.PerMinuteLimit || exhausted.RateUsed != reservation.PerMinuteLimit || !exhausted.ResetAt.Equal(window.Add(time.Minute)) {
		t.Fatalf("exhausted preflight = %#v/%v", exhausted, err)
	}

	var used, rateUsed int64
	var rateWindow time.Time
	if err := runtime.SQL.QueryRow(`SELECT used, rate_window_start, rate_used FROM source_request_usage_ledgers WHERE source_connection_id=$1 AND resource_profile_version=$2 AND budget_day=$3`, connection.ID, reservation.ResourceProfileVersion, window.Format(time.DateOnly)).Scan(&used, &rateWindow, &rateUsed); err != nil {
		t.Fatal(err)
	}
	if used != reservation.PerMinuteLimit || rateUsed != reservation.PerMinuteLimit || !rateWindow.Equal(window) {
		t.Fatalf("persisted daily/rate/window = %d/%d/%s", used, rateUsed, rateWindow)
	}

	nextMinute := reservation
	nextMinute.At = window.Add(time.Minute)
	nextPreflight, err := budget.CheckExternalRequest(context.Background(), nextMinute)
	if err != nil || !nextPreflight.Allowed || nextPreflight.Used != reservation.PerMinuteLimit || nextPreflight.RateUsed != 0 {
		t.Fatalf("next minute preflight = %#v/%v", nextPreflight, err)
	}
	decision, err := budget.ReserveExternalRequest(context.Background(), nextMinute)
	if err != nil || !decision.Allowed || decision.Used != reservation.PerMinuteLimit+1 || decision.RateUsed != 1 {
		t.Fatalf("next minute reservation = %#v/%v", decision, err)
	}
}
