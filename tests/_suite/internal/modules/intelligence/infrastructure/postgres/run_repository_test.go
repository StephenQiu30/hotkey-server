package postgres_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
)

func TestRunRepositoryAllowsOnlyOneConcurrentInFlightReuseKey(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	const callers = 6
	results := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repository.Claim(context.Background(), testClaim(profile))
			results <- err
		}()
	}
	group.Wait()
	close(results)

	created, inProgress := 0, 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		if code, ok := intelligencedomain.CodeOf(err); ok && code == intelligencedomain.CodeAIRunInProgress {
			inProgress++
			continue
		}
		t.Fatalf("concurrent Claim() error = %v", err)
	}
	if created != 1 || inProgress != callers-1 {
		t.Fatalf("concurrent claims created/in-progress = %d/%d, want 1/%d", created, inProgress, callers-1)
	}
}

func TestRunRepositoryRefreshesRetryLeaseAndReclaimsOnlyExpiredRuns(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	created, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	running, err := repository.Transition(context.Background(), created.Run.ID, intelligencedomain.RunStatusRunning, claim.Now.Add(10*time.Second))
	if err != nil || running.LeaseExpiresAt == nil {
		t.Fatalf("Transition(running) = %#v / %v", running, err)
	}
	validating, err := repository.Transition(context.Background(), created.Run.ID, intelligencedomain.RunStatusValidating, claim.Now.Add(15*time.Second))
	if err != nil || validating.LeaseExpiresAt == nil || !validating.LeaseExpiresAt.After(*running.LeaseExpiresAt) {
		t.Fatalf("Transition(validating) = %#v / %v, want a refreshed lease", validating, err)
	}
	retrying, err := repository.Transition(context.Background(), created.Run.ID, intelligencedomain.RunStatusRetryWait, claim.Now.Add(20*time.Second))
	if err != nil || retrying.LeaseExpiresAt == nil {
		t.Fatalf("Transition(retry_wait) = %#v / %v", retrying, err)
	}
	if reclaimed, err := repository.ReclaimExpired(context.Background(), retrying.LeaseExpiresAt.Add(-time.Microsecond)); err != nil || reclaimed != 0 {
		t.Fatalf("ReclaimExpired(before lease) = %d / %v, want 0/nil", reclaimed, err)
	}
	resumed, err := repository.Transition(context.Background(), created.Run.ID, intelligencedomain.RunStatusRunning, claim.Now.Add(22*time.Second))
	if err != nil || resumed.LeaseExpiresAt == nil || !resumed.LeaseExpiresAt.After(*retrying.LeaseExpiresAt) {
		t.Fatalf("Transition(retry_wait -> running) = %#v / %v, want a refreshed lease", resumed, err)
	}
	if reclaimed, err := repository.ReclaimExpired(context.Background(), resumed.LeaseExpiresAt.Add(-time.Microsecond)); err != nil || reclaimed != 0 {
		t.Fatalf("ReclaimExpired(before refreshed lease) = %d / %v, want 0/nil", reclaimed, err)
	}
	if reclaimed, err := repository.ReclaimExpired(context.Background(), resumed.LeaseExpiresAt.Add(time.Microsecond)); err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimExpired(after lease) = %d / %v, want 1/nil", reclaimed, err)
	}
	var status string
	var code *int
	var reserved string
	if err := runtime.SQL.QueryRow(`SELECT status, error_code, reserved_cost::text FROM ai_runs WHERE id=$1`, created.Run.ID).Scan(&status, &code, &reserved); err != nil {
		t.Fatalf("read reclaimed run: %v", err)
	}
	if status != string(intelligencedomain.RunStatusFailed) || code == nil || *code != intelligencedomain.CodeAIRunLeaseExpired || reserved != "0.0000" {
		t.Fatalf("reclaimed run status/code/reserved = %s/%v/%s, want failed/70009/0", status, code, reserved)
	}
}

func TestRunRepositoryBlocksOverageForUnlimitedBudgetButAllowsNextUTCDay(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "unlimited-overage-profile"
	profile.DailyBudget = nil
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	created, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := repository.Settle(context.Background(), created.Run.ID, "1.0100", claim.Now.Add(time.Minute)); err != nil {
		t.Fatalf("Settle(overage) error = %v", err)
	}
	if _, err := repository.Claim(context.Background(), claim); err == nil {
		t.Fatal("Claim(same overage day) error = nil, want 70002")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIBudgetExhausted {
		t.Fatalf("Claim(same overage day) code = %d/%t, want 70002", code, ok)
	}
	nextDay := claim
	nextDay.Now = claim.Now.AddDate(0, 0, 1)
	nextDay.InputHash = strings.Repeat("c", 64)
	if _, err := repository.Claim(context.Background(), nextDay); err != nil {
		t.Fatalf("Claim(next UTC day) error = %v", err)
	}
}

func TestRunRepositoryCancellationReleasesBudgetReservation(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	created, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	cancelled, err := repository.Cancel(context.Background(), created.Run.ID, claim.Now.Add(time.Second))
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != intelligencedomain.RunStatusCancelled || cancelled.ReservedCost != "0.0000" || cancelled.LeaseExpiresAt != nil {
		t.Fatalf("Cancel() = %#v, want released cancelled run", cancelled)
	}
	var reserved string
	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text FROM ai_budget_ledgers WHERE model_profile_id=$1 AND budget_day=DATE '2026-07-17'`, profile.ID).Scan(&reserved); err != nil {
		t.Fatalf("read cancelled ledger: %v", err)
	}
	if reserved != "0.0000" {
		t.Fatalf("cancelled ledger reservation = %s, want 0.0000", reserved)
	}
}

func TestRunRepositoryClaimReclaimsExpiredProfileDayReservationsBeforeReserving(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "claim-reclaims-profile-day"
	dailyBudget := "1.0000"
	profile.DailyBudget = &dailyBudget
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	first, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim(first) error = %v", err)
	}
	secondClaim := claim
	secondClaim.Now = first.Run.LeaseExpiresAt.Add(time.Second)
	secondClaim.InputHash = strings.Repeat("d", 64)
	second, err := repository.Claim(context.Background(), secondClaim)
	if err != nil {
		t.Fatalf("Claim(after expired reservation) error = %v", err)
	}
	if second.Run.ID == first.Run.ID || second.Reused {
		t.Fatalf("Claim(after expiration) = %#v, want a new run", second)
	}
	var firstStatus string
	var firstCode *int
	if err := runtime.SQL.QueryRow(`SELECT status,error_code FROM ai_runs WHERE id=$1`, first.Run.ID).Scan(&firstStatus, &firstCode); err != nil {
		t.Fatalf("read reclaimed first run: %v", err)
	}
	if firstStatus != string(intelligencedomain.RunStatusFailed) || firstCode == nil || *firstCode != intelligencedomain.CodeAIRunLeaseExpired {
		t.Fatalf("first run status/code = %s/%v, want failed/70009", firstStatus, firstCode)
	}
}

func TestRunRepositoryReclaimsQueuedRunningAndRetryWaitAfterProcessDeath(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "reclaim-all-inflight-states"
	dailyBudget := "3.0000"
	profile.DailyBudget = &dailyBudget
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	queued, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim(queued) error = %v", err)
	}
	runningClaim := claim
	runningClaim.InputHash = strings.Repeat("e", 64)
	running, err := repository.Claim(context.Background(), runningClaim)
	if err != nil {
		t.Fatalf("Claim(running) error = %v", err)
	}
	if _, err := repository.Transition(context.Background(), running.Run.ID, intelligencedomain.RunStatusRunning, claim.Now.Add(time.Second)); err != nil {
		t.Fatalf("Transition(running) error = %v", err)
	}
	retryClaim := claim
	retryClaim.InputHash = strings.Repeat("f", 64)
	retrying, err := repository.Claim(context.Background(), retryClaim)
	if err != nil {
		t.Fatalf("Claim(retry) error = %v", err)
	}
	if _, err := repository.Transition(context.Background(), retrying.Run.ID, intelligencedomain.RunStatusRunning, claim.Now.Add(time.Second)); err != nil {
		t.Fatalf("Transition(retry running) error = %v", err)
	}
	if _, err := repository.Transition(context.Background(), retrying.Run.ID, intelligencedomain.RunStatusRetryWait, claim.Now.Add(2*time.Second)); err != nil {
		t.Fatalf("Transition(retry wait) error = %v", err)
	}
	if reclaimed, err := repository.ReclaimExpired(context.Background(), claim.Now.Add(time.Hour)); err != nil || reclaimed != 3 {
		t.Fatalf("ReclaimExpired(all states) = %d / %v, want 3/nil", reclaimed, err)
	}
	var failed int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM ai_runs WHERE id = ANY($1) AND status='failed' AND error_code=$2 AND reserved_cost=0`, []int64{queued.Run.ID, running.Run.ID, retrying.Run.ID}, intelligencedomain.CodeAIRunLeaseExpired).Scan(&failed); err != nil {
		t.Fatalf("read reclaimer state: %v", err)
	}
	if failed != 3 {
		t.Fatalf("reclaimed terminal rows = %d, want 3", failed)
	}
}

func TestRunRepositoryRejectsTerminalTransitionAndFailsWithReleasedBudget(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "terminal-transition-and-failure"
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	created, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := repository.Transition(context.Background(), created.Run.ID, intelligencedomain.RunStatusSucceeded, claim.Now.Add(time.Second)); err == nil {
		t.Fatal("Transition(succeeded) error = nil, want terminal transition rejection")
	}
	failed, err := repository.Fail(context.Background(), created.Run.ID, intelligencedomain.CodeAIOutputInvalid, claim.Now.Add(time.Second))
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if failed.Status != intelligencedomain.RunStatusFailed || failed.ErrorCode == nil || *failed.ErrorCode != intelligencedomain.CodeAIOutputInvalid || failed.ReservedCost != "0.0000" || failed.LeaseExpiresAt != nil {
		t.Fatalf("Fail() = %#v, want released failed 70006 run", failed)
	}
	var reserved string
	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text FROM ai_budget_ledgers WHERE model_profile_id=$1 AND budget_day=DATE '2026-07-17'`, profile.ID).Scan(&reserved); err != nil {
		t.Fatalf("read failed ledger: %v", err)
	}
	if reserved != "0.0000" {
		t.Fatalf("failed run leaves reserved budget %s, want 0.0000", reserved)
	}
}

func TestRunRepositoryConcurrentReclaimIsIdempotent(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "idempotent-reclaim"
	dailyBudget := "3.0000"
	profile.DailyBudget = &dailyBudget
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	for _, hash := range []string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)} {
		candidate := claim
		candidate.InputHash = hash
		if _, err := repository.Claim(context.Background(), candidate); err != nil {
			t.Fatalf("Claim(expired candidate) error = %v", err)
		}
	}
	results := make(chan struct {
		reclaimed int
		err       error
	}, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			reclaimed, err := repository.ReclaimExpired(context.Background(), claim.Now.Add(time.Hour))
			results <- struct {
				reclaimed int
				err       error
			}{reclaimed, err}
		}()
	}
	group.Wait()
	close(results)
	total := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent ReclaimExpired() error = %v", result.err)
		}
		total += result.reclaimed
	}
	if total != 3 {
		t.Fatalf("concurrent reclaimed total = %d, want 3", total)
	}
}
