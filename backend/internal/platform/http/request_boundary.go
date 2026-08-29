package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"strings"
	"sync"
	"time"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
	"github.com/gin-gonic/gin"
)

const (
	RequestBoundaryProfileVersion = "p0-request-boundaries-v2"
	defaultBoundaryStateEntries   = 4096
)

type RequestBoundaryClass string

const (
	RequestBoundaryLogin         RequestBoundaryClass = "login"
	RequestBoundaryVerification  RequestBoundaryClass = "verification"
	RequestBoundaryPasswordReset RequestBoundaryClass = "password_reset"
	RequestBoundarySourceProbe   RequestBoundaryClass = "source_probe"
	RequestBoundaryCollection    RequestBoundaryClass = "collection"
	RequestBoundaryCodex         RequestBoundaryClass = "codex"
	RequestBoundaryReport        RequestBoundaryClass = "report"
	RequestBoundaryOperations    RequestBoundaryClass = "operations"
)

type RequestBoundaryReason string

const (
	RequestBoundaryRateLimited         RequestBoundaryReason = "rate_limited"
	RequestBoundaryAttemptsExceeded    RequestBoundaryReason = "attempts_exceeded"
	RequestBoundaryConcurrencyExceeded RequestBoundaryReason = "concurrency_exceeded"
	RequestBoundaryWallClockExceeded   RequestBoundaryReason = "wall_clock_exceeded"
	RequestBoundaryCapacityExceeded    RequestBoundaryReason = "capacity_exceeded"
)

type RequestBoundaryRoute struct {
	Method string
	Path   string
}

type RequestBoundaryWindow struct {
	Limit  int
	Window time.Duration
}

type RequestBoundaryProfile struct {
	Version       string
	Class         RequestBoundaryClass
	Routes        []RequestBoundaryRoute
	Rate          RequestBoundaryWindow
	MaxConcurrent int
	Attempts      RequestBoundaryWindow
	WallClock     time.Duration
}

func (profile RequestBoundaryProfile) Validate() error {
	if profile.Version != RequestBoundaryProfileVersion || !profile.Class.Valid() || len(profile.Routes) == 0 ||
		profile.Rate.Validate() != nil || profile.Attempts.Validate() != nil ||
		profile.MaxConcurrent < 1 || profile.MaxConcurrent > 256 ||
		profile.WallClock <= 0 || profile.WallClock > 2*time.Minute {
		return fmt.Errorf("invalid request boundary profile")
	}
	seen := make(map[RequestBoundaryRoute]struct{}, len(profile.Routes))
	for _, route := range profile.Routes {
		if route.Method == "" || route.Method != strings.ToUpper(route.Method) || !strings.HasPrefix(route.Path, "/") || strings.TrimSpace(route.Path) != route.Path {
			return fmt.Errorf("invalid request boundary route")
		}
		if _, exists := seen[route]; exists {
			return fmt.Errorf("duplicate request boundary route")
		}
		seen[route] = struct{}{}
	}
	return nil
}

func (window RequestBoundaryWindow) Validate() error {
	if window.Limit < 1 || window.Limit > 100_000 || window.Window <= 0 || window.Window > 24*time.Hour {
		return fmt.Errorf("invalid request boundary window")
	}
	return nil
}

func (class RequestBoundaryClass) Valid() bool {
	switch class {
	case RequestBoundaryLogin, RequestBoundaryVerification, RequestBoundaryPasswordReset, RequestBoundarySourceProbe,
		RequestBoundaryCollection, RequestBoundaryCodex, RequestBoundaryReport, RequestBoundaryOperations:
		return true
	default:
		return false
	}
}

func DefaultRequestBoundaryProfiles() []RequestBoundaryProfile {
	return []RequestBoundaryProfile{
		requestBoundaryProfile(RequestBoundaryLogin, 20, time.Minute, 8, 5, 15*time.Second, 10*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/auth/login"}),
		requestBoundaryProfile(RequestBoundaryVerification, 12, time.Minute, 4, 4, time.Minute, 10*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/auth/email-verifications"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/auth/email-verifications/confirm"}),
		requestBoundaryProfile(RequestBoundaryPasswordReset, 10, time.Minute, 4, 4, time.Minute, 10*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/auth/password-resets/confirm"}),
		requestBoundaryProfile(RequestBoundarySourceProbe, 30, time.Minute, 4, 5, 10*time.Second, 15*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/source-connections/:id/health"}),
		requestBoundaryProfile(RequestBoundaryCollection, 30, time.Minute, 4, 6, 10*time.Second, 15*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/monitors/:id/collect"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/collection-runs/:id/retry"}),
		requestBoundaryProfile(RequestBoundaryCodex, 12, time.Minute, 2, 3, 10*time.Second, 15*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/ai/runs/:id/recompute"}),
		requestBoundaryProfile(RequestBoundaryReport, 30, time.Minute, 4, 6, 10*time.Second, 15*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/preview"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/build"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/submit"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/approve"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/reports/:id/reject"}),
		requestBoundaryProfile(RequestBoundaryOperations, 60, time.Minute, 4, 10, 10*time.Second, 15*time.Second,
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/operations/jobs/:id/cancel"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/operations/jobs/:id/retry"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/operations/retention-policies/:id/preview"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/operations/retention-runs/:id/approve"},
			RequestBoundaryRoute{Method: stdhttp.MethodPost, Path: "/api/v1/operations/retention-runs/:id/execute"}),
	}
}

func requestBoundaryProfile(class RequestBoundaryClass, rate int, rateWindow time.Duration, concurrent, attempts int, attemptsWindow, wallClock time.Duration, routes ...RequestBoundaryRoute) RequestBoundaryProfile {
	return RequestBoundaryProfile{
		Version: RequestBoundaryProfileVersion, Class: class, Routes: routes,
		Rate: RequestBoundaryWindow{Limit: rate, Window: rateWindow}, MaxConcurrent: concurrent,
		Attempts: RequestBoundaryWindow{Limit: attempts, Window: attemptsWindow}, WallClock: wallClock,
	}
}

type RequestBoundaryRejection struct {
	ProfileVersion string                `json:"profile_version"`
	Class          RequestBoundaryClass  `json:"class"`
	Reason         RequestBoundaryReason `json:"reason"`
	RequestID      string                `json:"request_id,omitempty"`
	TraceID        string                `json:"trace_id,omitempty"`
	ClientHash     string                `json:"client_hash"`
}

type RequestBoundaryAudit interface {
	WriteRequestBoundaryRejection(context.Context, RequestBoundaryRejection) error
}

type RequestBoundaryControllerOptions struct {
	Profiles        []RequestBoundaryProfile
	Audit           RequestBoundaryAudit
	Now             func() time.Time
	MaxStateEntries int
	ClientHashKey   []byte
}

type RequestBoundaryController struct {
	mu                 sync.Mutex
	profiles           map[RequestBoundaryRoute]RequestBoundaryProfile
	states             map[requestBoundaryStateKey]*requestBoundaryState
	audit              RequestBoundaryAudit
	now                func() time.Time
	maxStateEntries    int
	capacityAuditUntil map[RequestBoundaryClass]time.Time
	clientHashKey      []byte
}

type requestBoundaryStateKey struct {
	Class      RequestBoundaryClass
	ClientHash string
}

type requestBoundaryState struct {
	rateStarted    time.Time
	rateCount      int
	attemptStarted time.Time
	attemptCount   int
	concurrent     int
	lastSeen       time.Time
	auditUntil     map[RequestBoundaryReason]time.Time
}

func NewRequestBoundaryController(options RequestBoundaryControllerOptions) (*RequestBoundaryController, error) {
	if len(options.Profiles) == 0 {
		return nil, fmt.Errorf("request boundary profiles are required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaxStateEntries == 0 {
		options.MaxStateEntries = defaultBoundaryStateEntries
	}
	if options.MaxStateEntries < 1 || options.MaxStateEntries > 1_000_000 {
		return nil, fmt.Errorf("invalid request boundary state capacity")
	}
	clientHashKey := append([]byte(nil), options.ClientHashKey...)
	if len(clientHashKey) == 0 {
		clientHashKey = make([]byte, 32)
		if _, err := rand.Read(clientHashKey); err != nil {
			return nil, fmt.Errorf("generate request boundary client hash key: %w", err)
		}
	}
	if len(clientHashKey) < 32 || len(clientHashKey) > 1024 {
		return nil, fmt.Errorf("invalid request boundary client hash key")
	}
	controller := &RequestBoundaryController{
		profiles: make(map[RequestBoundaryRoute]RequestBoundaryProfile), states: make(map[requestBoundaryStateKey]*requestBoundaryState),
		audit: options.Audit, now: options.Now, maxStateEntries: options.MaxStateEntries,
		capacityAuditUntil: make(map[RequestBoundaryClass]time.Time), clientHashKey: clientHashKey,
	}
	seenClasses := make(map[RequestBoundaryClass]struct{}, len(options.Profiles))
	for _, profile := range options.Profiles {
		if err := profile.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seenClasses[profile.Class]; exists {
			return nil, fmt.Errorf("duplicate request boundary class")
		}
		seenClasses[profile.Class] = struct{}{}
		for _, route := range profile.Routes {
			if _, exists := controller.profiles[route]; exists {
				return nil, fmt.Errorf("duplicate request boundary route")
			}
			controller.profiles[route] = profile
		}
	}
	return controller, nil
}

func (controller *RequestBoundaryController) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil {
			return
		}
		if controller == nil || c.Request == nil {
			c.Next()
			return
		}
		profile, protected := controller.profiles[RequestBoundaryRoute{Method: c.Request.Method, Path: c.FullPath()}]
		if !protected {
			c.Next()
			return
		}
		now := controller.now().UTC()
		clientHash := requestBoundaryClientHash(c.Request.RemoteAddr, controller.clientHashKey)
		release, rejection, shouldAudit := controller.admit(profile, clientHash, now)
		if rejection != "" {
			if shouldAudit {
				controller.writeAudit(c.Request.Context(), profile, rejection, clientHash)
			}
			c.Header("Retry-After", "1")
			WriteError(c, sharederrors.New(sharederrors.CodeRateLimited, stdhttp.StatusTooManyRequests, ""))
			return
		}
		defer release()

		boundaryContext, cancel := context.WithTimeout(c.Request.Context(), profile.WallClock)
		defer cancel()
		c.Request = c.Request.WithContext(boundaryContext)
		c.Next()
		if errors.Is(boundaryContext.Err(), context.DeadlineExceeded) {
			controller.auditOnce(c.Request.Context(), profile, RequestBoundaryWallClockExceeded, clientHash, now)
			if !c.Writer.Written() {
				WriteError(c, context.DeadlineExceeded)
			}
		}
	}
}

func (controller *RequestBoundaryController) admit(profile RequestBoundaryProfile, clientHash string, now time.Time) (func(), RequestBoundaryReason, bool) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	key := requestBoundaryStateKey{Class: profile.Class, ClientHash: clientHash}
	state := controller.states[key]
	if state == nil {
		controller.pruneLocked(now)
		if len(controller.states) >= controller.maxStateEntries {
			until := controller.capacityAuditUntil[profile.Class]
			shouldAudit := !now.Before(until)
			if shouldAudit {
				controller.capacityAuditUntil[profile.Class] = now.Add(time.Minute)
			}
			return func() {}, RequestBoundaryCapacityExceeded, shouldAudit
		}
		state = &requestBoundaryState{rateStarted: now, attemptStarted: now, lastSeen: now, auditUntil: make(map[RequestBoundaryReason]time.Time)}
		controller.states[key] = state
	}
	state.lastSeen = now
	resetRequestBoundaryWindow(now, profile.Rate.Window, &state.rateStarted, &state.rateCount)
	resetRequestBoundaryWindow(now, profile.Attempts.Window, &state.attemptStarted, &state.attemptCount)
	if state.rateCount >= profile.Rate.Limit {
		return func() {}, RequestBoundaryRateLimited, markRequestBoundaryAuditLocked(state, RequestBoundaryRateLimited, state.rateStarted.Add(profile.Rate.Window), now)
	}
	if state.attemptCount >= profile.Attempts.Limit {
		return func() {}, RequestBoundaryAttemptsExceeded, markRequestBoundaryAuditLocked(state, RequestBoundaryAttemptsExceeded, state.attemptStarted.Add(profile.Attempts.Window), now)
	}
	if state.concurrent >= profile.MaxConcurrent {
		return func() {}, RequestBoundaryConcurrencyExceeded, markRequestBoundaryAuditLocked(state, RequestBoundaryConcurrencyExceeded, now.Add(time.Minute), now)
	}
	state.rateCount++
	state.attemptCount++
	state.concurrent++
	var once sync.Once
	return func() {
		once.Do(func() {
			controller.mu.Lock()
			defer controller.mu.Unlock()
			if current := controller.states[key]; current != nil && current.concurrent > 0 {
				current.concurrent--
			}
		})
	}, "", false
}

func (controller *RequestBoundaryController) auditOnce(ctx context.Context, profile RequestBoundaryProfile, reason RequestBoundaryReason, clientHash string, now time.Time) {
	controller.mu.Lock()
	state := controller.states[requestBoundaryStateKey{Class: profile.Class, ClientHash: clientHash}]
	shouldAudit := state != nil && markRequestBoundaryAuditLocked(state, reason, now.Add(time.Minute), now)
	controller.mu.Unlock()
	if shouldAudit {
		controller.writeAudit(ctx, profile, reason, clientHash)
	}
}

func (controller *RequestBoundaryController) writeAudit(ctx context.Context, profile RequestBoundaryProfile, reason RequestBoundaryReason, clientHash string) {
	if controller.audit == nil {
		return
	}
	_ = controller.audit.WriteRequestBoundaryRejection(ctx, RequestBoundaryRejection{
		ProfileVersion: profile.Version, Class: profile.Class, Reason: reason,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), ClientHash: clientHash,
	})
}

func (controller *RequestBoundaryController) pruneLocked(now time.Time) {
	for key, state := range controller.states {
		profile := controller.profileForClassLocked(key.Class)
		retention := profile.Rate.Window
		if profile.Attempts.Window > retention {
			retention = profile.Attempts.Window
		}
		if state.concurrent == 0 && !state.lastSeen.IsZero() && !now.Before(state.lastSeen.Add(retention)) {
			delete(controller.states, key)
		}
	}
}

func (controller *RequestBoundaryController) profileForClassLocked(class RequestBoundaryClass) RequestBoundaryProfile {
	for _, profile := range controller.profiles {
		if profile.Class == class {
			return profile
		}
	}
	return RequestBoundaryProfile{}
}

func resetRequestBoundaryWindow(now time.Time, window time.Duration, started *time.Time, count *int) {
	if started.IsZero() || !now.Before(started.Add(window)) {
		*started = now
		*count = 0
	}
}

func markRequestBoundaryAuditLocked(state *requestBoundaryState, reason RequestBoundaryReason, until, now time.Time) bool {
	if state == nil || now.Before(state.auditUntil[reason]) {
		return false
	}
	state.auditUntil[reason] = until
	return true
}

func requestBoundaryClientHash(remoteAddress string, key []byte) string {
	host := strings.TrimSpace(remoteAddress)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if parsed := net.ParseIP(strings.Trim(host, "[]")); parsed != nil {
		host = parsed.String()
	}
	if host == "" {
		host = "unknown"
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(host))
	return hex.EncodeToString(digest.Sum(nil))
}

func deriveRequestBoundaryClientHashKey(secret string) []byte {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write([]byte("hotkey-request-boundary-client-hash-v1"))
	return digest.Sum(nil)
}
