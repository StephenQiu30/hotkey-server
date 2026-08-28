package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestP0RequestBoundaryMatrixIsVersionedFiniteAndComplete(t *testing.T) {
	profiles := DefaultRequestBoundaryProfiles()
	wantClasses := []RequestBoundaryClass{
		RequestBoundaryLogin,
		RequestBoundaryVerification,
		RequestBoundaryPasswordReset,
		RequestBoundarySourceProbe,
		RequestBoundaryCollection,
		RequestBoundaryCodex,
		RequestBoundaryReport,
		RequestBoundaryOperations,
	}
	gotClasses := make([]RequestBoundaryClass, 0, len(profiles))
	seenRoutes := map[RequestBoundaryRoute]RequestBoundaryClass{}
	for _, profile := range profiles {
		if err := profile.Validate(); err != nil {
			t.Fatalf("profile %q is not finite: %v", profile.Class, err)
		}
		if profile.Version != RequestBoundaryProfileVersion {
			t.Fatalf("profile %q version = %q", profile.Class, profile.Version)
		}
		gotClasses = append(gotClasses, profile.Class)
		for _, route := range profile.Routes {
			if owner, exists := seenRoutes[route]; exists {
				t.Fatalf("route %#v belongs to both %q and %q", route, owner, profile.Class)
			}
			seenRoutes[route] = profile.Class
		}
	}
	if !reflect.DeepEqual(gotClasses, wantClasses) {
		t.Fatalf("boundary classes = %#v, want %#v", gotClasses, wantClasses)
	}

	wantRoutes := map[RequestBoundaryRoute]RequestBoundaryClass{
		{Method: stdhttp.MethodPost, Path: "/api/v1/auth/login"}:                                RequestBoundaryLogin,
		{Method: stdhttp.MethodPost, Path: "/api/v1/auth/email-verifications"}:                  RequestBoundaryVerification,
		{Method: stdhttp.MethodPost, Path: "/api/v1/auth/email-verifications/confirm"}:          RequestBoundaryVerification,
		{Method: stdhttp.MethodPost, Path: "/api/v1/auth/password-resets/confirm"}:              RequestBoundaryPasswordReset,
		{Method: stdhttp.MethodPost, Path: "/api/v1/source-connections/:id/health"}:             RequestBoundarySourceProbe,
		{Method: stdhttp.MethodPost, Path: "/api/v1/monitors/:id/collect"}:                      RequestBoundaryCollection,
		{Method: stdhttp.MethodPost, Path: "/api/v1/collection-runs/:id/retry"}:                 RequestBoundaryCollection,
		{Method: stdhttp.MethodPost, Path: "/api/v1/ai/runs/:id/recompute"}:                     RequestBoundaryCodex,
		{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/build"}:                         RequestBoundaryReport,
		{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/submit"}:                        RequestBoundaryReport,
		{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/approve"}:                       RequestBoundaryReport,
		{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/reject"}:                        RequestBoundaryReport,
		{Method: stdhttp.MethodPost, Path: "/api/v1/operations/jobs/:id/cancel"}:                RequestBoundaryOperations,
		{Method: stdhttp.MethodPost, Path: "/api/v1/operations/jobs/:id/retry"}:                 RequestBoundaryOperations,
		{Method: stdhttp.MethodPost, Path: "/api/v1/operations/retention-policies/:id/preview"}: RequestBoundaryOperations,
		{Method: stdhttp.MethodPost, Path: "/api/v1/operations/retention-policies/:id/run"}:     RequestBoundaryOperations,
	}
	for route, wantClass := range wantRoutes {
		if got := seenRoutes[route]; got != wantClass {
			t.Errorf("route %#v class = %q, want %q", route, got, wantClass)
		}
	}
}

func TestFixedRequestBoundaryFixtureMatchesProductionProfilesAndStableCodes(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "security", "request_boundaries.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ProfileVersion   string         `json:"profile_version"`
		StableErrorCodes map[string]int `json:"stable_error_codes"`
		Entries          []struct {
			Class         RequestBoundaryClass         `json:"class"`
			Routes        []RequestBoundaryRoute       `json:"routes"`
			Rate          requestBoundaryFixtureWindow `json:"rate"`
			MaxConcurrent int                          `json:"max_concurrent"`
			Attempts      requestBoundaryFixtureWindow `json:"attempts"`
			WallClock     string                       `json:"wall_clock"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	wantCodes := map[string]int{
		"rate_limited":         sharederrors.CodeRateLimited,
		"attempts_exceeded":    sharederrors.CodeRateLimited,
		"concurrency_exceeded": sharederrors.CodeRateLimited,
		"wall_clock_exceeded":  sharederrors.CodeDeadlineExceeded,
	}
	if fixture.ProfileVersion != RequestBoundaryProfileVersion || !reflect.DeepEqual(fixture.StableErrorCodes, wantCodes) {
		t.Fatalf("fixture version/codes = %q/%#v", fixture.ProfileVersion, fixture.StableErrorCodes)
	}
	got := make([]RequestBoundaryProfile, 0, len(fixture.Entries))
	for _, entry := range fixture.Entries {
		rateWindow, err := time.ParseDuration(entry.Rate.Window)
		if err != nil {
			t.Fatalf("%s rate window: %v", entry.Class, err)
		}
		attemptWindow, err := time.ParseDuration(entry.Attempts.Window)
		if err != nil {
			t.Fatalf("%s attempt window: %v", entry.Class, err)
		}
		wallClock, err := time.ParseDuration(entry.WallClock)
		if err != nil {
			t.Fatalf("%s wall clock: %v", entry.Class, err)
		}
		got = append(got, RequestBoundaryProfile{
			Version: fixture.ProfileVersion, Class: entry.Class, Routes: entry.Routes,
			Rate: RequestBoundaryWindow{Limit: entry.Rate.Limit, Window: rateWindow}, MaxConcurrent: entry.MaxConcurrent,
			Attempts: RequestBoundaryWindow{Limit: entry.Attempts.Limit, Window: attemptWindow}, WallClock: wallClock,
		})
	}
	if want := DefaultRequestBoundaryProfiles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture profiles = %#v, want %#v", got, want)
	}
}

type requestBoundaryFixtureWindow struct {
	Limit  int    `json:"limit"`
	Window string `json:"window"`
}

func TestRequestBoundaryRateAndAttemptDimensionsStopAtLimitPlusOne(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		profile    RequestBoundaryProfile
		wantReason RequestBoundaryReason
	}{
		{
			name:       "rate",
			profile:    requestBoundaryTestProfile(RequestBoundaryWindow{Limit: 2, Window: time.Minute}, RequestBoundaryWindow{Limit: 100, Window: time.Minute}),
			wantReason: RequestBoundaryRateLimited,
		},
		{
			name:       "attempts",
			profile:    requestBoundaryTestProfile(RequestBoundaryWindow{Limit: 100, Window: time.Minute}, RequestBoundaryWindow{Limit: 2, Window: 10 * time.Second}),
			wantReason: RequestBoundaryAttemptsExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := now
			audit := &requestBoundaryAuditRecorder{}
			controller, err := NewRequestBoundaryController(RequestBoundaryControllerOptions{
				Profiles: []RequestBoundaryProfile{test.profile}, Audit: audit, Now: func() time.Time { return clock },
			})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			router := requestBoundaryTestRouter(controller, func(c *gin.Context) {
				calls.Add(1)
				Empty(c)
			})

			for attempt, wantStatus := range []int{stdhttp.StatusOK, stdhttp.StatusOK, stdhttp.StatusTooManyRequests} {
				response := performRequestBoundaryRequest(router)
				if response.Code != wantStatus {
					t.Fatalf("attempt %d status = %d body=%s", attempt+1, response.Code, response.Body.String())
				}
				if wantStatus != stdhttp.StatusOK {
					assertRequestBoundaryError(t, response, sharederrors.CodeRateLimited)
				}
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("business handler calls = %d, want limit", got)
			}
			if entries := audit.snapshot(); len(entries) != 1 || entries[0].Reason != test.wantReason {
				t.Fatalf("audit entries = %#v", entries)
			}
			if test.wantReason == RequestBoundaryRateLimited {
				clock = clock.Add(test.profile.Rate.Window)
			} else {
				clock = clock.Add(test.profile.Attempts.Window)
			}
			if response := performRequestBoundaryRequest(router); response.Code != stdhttp.StatusOK {
				t.Fatalf("request after reset status = %d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRequestBoundaryConcurrencyRejectsBeforeBusinessHandler(t *testing.T) {
	profile := requestBoundaryTestProfile(RequestBoundaryWindow{Limit: 100, Window: time.Minute}, RequestBoundaryWindow{Limit: 100, Window: time.Minute})
	profile.MaxConcurrent = 1
	audit := &requestBoundaryAuditRecorder{}
	controller, err := NewRequestBoundaryController(RequestBoundaryControllerOptions{Profiles: []RequestBoundaryProfile{profile}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	router := requestBoundaryTestRouter(controller, func(c *gin.Context) {
		calls.Add(1)
		close(entered)
		<-release
		Empty(c)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	firstDone := make(chan *stdhttp.Response, 1)
	go func() {
		response, _ := server.Client().Post(server.URL+"/bounded", "application/json", nil)
		firstDone <- response
	}()
	<-entered
	second, err := server.Client().Post(server.URL+"/bounded", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != stdhttp.StatusTooManyRequests {
		t.Fatalf("concurrent status = %d", second.StatusCode)
	}
	close(release)
	first := <-firstDone
	if first == nil {
		t.Fatal("first response is nil")
	}
	defer first.Body.Close()
	if first.StatusCode != stdhttp.StatusOK || calls.Load() != 1 {
		t.Fatalf("first status/calls = %d/%d", first.StatusCode, calls.Load())
	}
	entries := audit.snapshot()
	if len(entries) != 1 || entries[0].Reason != RequestBoundaryConcurrencyExceeded {
		t.Fatalf("concurrency audit = %#v", entries)
	}
}

func TestRequestBoundaryWallClockCancelsAndAuditsWithoutBusinessSideEffect(t *testing.T) {
	profile := requestBoundaryTestProfile(RequestBoundaryWindow{Limit: 100, Window: time.Minute}, RequestBoundaryWindow{Limit: 100, Window: time.Minute})
	profile.WallClock = 10 * time.Millisecond
	audit := &requestBoundaryAuditRecorder{}
	controller, err := NewRequestBoundaryController(RequestBoundaryControllerOptions{Profiles: []RequestBoundaryProfile{profile}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	var sideEffects atomic.Int64
	router := requestBoundaryTestRouter(controller, func(c *gin.Context) {
		<-c.Request.Context().Done()
		if c.Request.Context().Err() == nil {
			sideEffects.Add(1)
		}
	})

	response := performRequestBoundaryRequest(router)
	if response.Code != stdhttp.StatusGatewayTimeout {
		t.Fatalf("wall-clock status = %d body=%s", response.Code, response.Body.String())
	}
	assertRequestBoundaryError(t, response, sharederrors.CodeDeadlineExceeded)
	if sideEffects.Load() != 0 {
		t.Fatalf("wall-clock request produced %d side effects", sideEffects.Load())
	}
	entries := audit.snapshot()
	if len(entries) != 1 || entries[0].Reason != RequestBoundaryWallClockExceeded {
		t.Fatalf("wall-clock audit = %#v", entries)
	}
}

func TestRequestBoundaryAuditIsDeduplicatedAndContainsOnlySafeIdentifiers(t *testing.T) {
	profile := requestBoundaryTestProfile(RequestBoundaryWindow{Limit: 1, Window: time.Minute}, RequestBoundaryWindow{Limit: 100, Window: time.Minute})
	audit := &requestBoundaryAuditRecorder{}
	clientHashKey := []byte("test-only-boundary-client-hmac-key")
	controller, err := NewRequestBoundaryController(RequestBoundaryControllerOptions{Profiles: []RequestBoundaryProfile{profile}, Audit: audit, ClientHashKey: clientHashKey})
	if err != nil {
		t.Fatal(err)
	}
	router := requestBoundaryTestRouter(controller, func(c *gin.Context) { Empty(c) })
	_ = performRequestBoundaryRequest(router)
	_ = performRequestBoundaryRequest(router)
	_ = performRequestBoundaryRequest(router)

	entries := audit.snapshot()
	if len(entries) != 1 {
		t.Fatalf("deduplicated audit count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ProfileVersion != RequestBoundaryProfileVersion || entry.Class != RequestBoundaryLogin || entry.Reason != RequestBoundaryRateLimited {
		t.Fatalf("audit identity = %#v", entry)
	}
	expectedMAC := hmac.New(sha256.New, clientHashKey)
	_, _ = expectedMAC.Write([]byte("203.0.113.10"))
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(entry.ClientHash) || entry.ClientHash != fmt.Sprintf("%x", expectedMAC.Sum(nil)) {
		t.Fatalf("audit client hash = %q", entry.ClientHash)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"203.0.113.10", "/bounded", "Authorization", "payload"} {
		if regexp.MustCompile(regexp.QuoteMeta(forbidden)).Match(encoded) {
			t.Fatalf("audit leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestEveryP0RequestBoundaryClassExercisesAllFourThresholdDimensions(t *testing.T) {
	now := time.Date(2026, time.August, 28, 15, 0, 0, 0, time.UTC)
	clientHash := strings.Repeat("a", 64)
	for _, productionProfile := range DefaultRequestBoundaryProfiles() {
		productionProfile := productionProfile
		t.Run(string(productionProfile.Class), func(t *testing.T) {
			t.Run("rate_limit_minus_one_limit_limit_plus_one", func(t *testing.T) {
				profile := productionProfile
				profile.Attempts = RequestBoundaryWindow{Limit: profile.Rate.Limit + 10, Window: profile.Rate.Window}
				controller := mustRequestBoundaryController(t, profile, nil)
				for attempt := 1; attempt <= profile.Rate.Limit+1; attempt++ {
					release, reason, _ := controller.admit(profile, clientHash, now)
					if attempt <= profile.Rate.Limit {
						if reason != "" {
							t.Fatalf("rate attempt %d rejected: %s", attempt, reason)
						}
						release()
					} else if reason != RequestBoundaryRateLimited {
						t.Fatalf("rate limit+1 reason = %q", reason)
					}
				}
			})

			t.Run("attempt_limit_minus_one_limit_limit_plus_one", func(t *testing.T) {
				profile := productionProfile
				profile.Rate = RequestBoundaryWindow{Limit: profile.Attempts.Limit + 10, Window: profile.Attempts.Window}
				controller := mustRequestBoundaryController(t, profile, nil)
				for attempt := 1; attempt <= profile.Attempts.Limit+1; attempt++ {
					release, reason, _ := controller.admit(profile, clientHash, now)
					if attempt <= profile.Attempts.Limit {
						if reason != "" {
							t.Fatalf("short-window attempt %d rejected: %s", attempt, reason)
						}
						release()
					} else if reason != RequestBoundaryAttemptsExceeded {
						t.Fatalf("attempt limit+1 reason = %q", reason)
					}
				}
			})

			t.Run("concurrency_limit_minus_one_limit_limit_plus_one", func(t *testing.T) {
				profile := productionProfile
				profile.Rate = RequestBoundaryWindow{Limit: 100, Window: time.Minute}
				profile.Attempts = RequestBoundaryWindow{Limit: 100, Window: time.Minute}
				profile.MaxConcurrent = 2
				controller := mustRequestBoundaryController(t, profile, nil)
				firstRelease, firstReason, _ := controller.admit(profile, clientHash, now)
				secondRelease, secondReason, _ := controller.admit(profile, clientHash, now)
				_, thirdReason, _ := controller.admit(profile, clientHash, now)
				if firstReason != "" || secondReason != "" || thirdReason != RequestBoundaryConcurrencyExceeded {
					t.Fatalf("concurrency reasons = %q/%q/%q", firstReason, secondReason, thirdReason)
				}
				firstRelease()
				secondRelease()
			})

			t.Run("wall_clock_limit_plus_one", func(t *testing.T) {
				profile := productionProfile
				profile.Rate = RequestBoundaryWindow{Limit: 100, Window: time.Minute}
				profile.Attempts = RequestBoundaryWindow{Limit: 100, Window: time.Minute}
				profile.WallClock = 5 * time.Millisecond
				audit := &requestBoundaryAuditRecorder{}
				controller := mustRequestBoundaryController(t, profile, audit)
				router, requestPath := requestBoundaryRouterForProfile(profile, controller, func(c *gin.Context) {
					<-c.Request.Context().Done()
				})
				request := httptest.NewRequest(profile.Routes[0].Method, requestPath, nil)
				request.RemoteAddr = "203.0.113.10:443"
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != stdhttp.StatusGatewayTimeout {
					t.Fatalf("wall-clock status = %d body=%s", response.Code, response.Body.String())
				}
				entries := audit.snapshot()
				if len(entries) != 1 || entries[0].Class != profile.Class || entries[0].Reason != RequestBoundaryWallClockExceeded {
					t.Fatalf("wall-clock audit = %#v", entries)
				}
			})
		})
	}
}

func requestBoundaryTestProfile(rate, attempts RequestBoundaryWindow) RequestBoundaryProfile {
	return RequestBoundaryProfile{
		Version: RequestBoundaryProfileVersion,
		Class:   RequestBoundaryLogin,
		Routes:  []RequestBoundaryRoute{{Method: stdhttp.MethodPost, Path: "/bounded"}},
		Rate:    rate, MaxConcurrent: 2, Attempts: attempts, WallClock: time.Second,
	}
}

func requestBoundaryTestRouter(controller *RequestBoundaryController, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestID(), controller.Middleware())
	router.POST("/bounded", handler)
	return router
}

func requestBoundaryRouterForProfile(profile RequestBoundaryProfile, controller *RequestBoundaryController, handler gin.HandlerFunc) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestID(), controller.Middleware())
	route := profile.Routes[0]
	router.Handle(route.Method, route.Path, handler)
	return router, regexp.MustCompile(`:[A-Za-z0-9_]+`).ReplaceAllString(route.Path, "1")
}

func mustRequestBoundaryController(t *testing.T, profile RequestBoundaryProfile, audit RequestBoundaryAudit) *RequestBoundaryController {
	t.Helper()
	controller, err := NewRequestBoundaryController(RequestBoundaryControllerOptions{
		Profiles: []RequestBoundaryProfile{profile}, Audit: audit, ClientHashKey: []byte("test-only-boundary-client-hmac-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func performRequestBoundaryRequest(router *gin.Engine) *httptest.ResponseRecorder {
	request := httptest.NewRequest(stdhttp.MethodPost, "/bounded", nil)
	request.RemoteAddr = "203.0.113.10:443"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertRequestBoundaryError(t *testing.T, response *httptest.ResponseRecorder, wantCode int) {
	t.Helper()
	var envelope Result[any]
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != wantCode || envelope.Data != nil {
		t.Fatalf("boundary error = %#v", envelope)
	}
}

type requestBoundaryAuditRecorder struct {
	mu      sync.Mutex
	entries []RequestBoundaryRejection
}

func (recorder *requestBoundaryAuditRecorder) WriteRequestBoundaryRejection(_ context.Context, entry RequestBoundaryRejection) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.entries = append(recorder.entries, entry)
	return nil
}

func (recorder *requestBoundaryAuditRecorder) snapshot() []RequestBoundaryRejection {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]RequestBoundaryRejection(nil), recorder.entries...)
}
