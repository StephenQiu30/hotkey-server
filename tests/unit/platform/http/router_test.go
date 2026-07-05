package platformhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

type stubAuthRepo struct{ users []auth.User }

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}
func (r *stubAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	u := auth.User{ID: int64(len(r.users) + 1), Email: email, PasswordHash: passwordHash, DisplayName: displayName, Status: "active", PlanType: "free"}
	r.users = append(r.users, u)
	return u, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (r *stubAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

type stubMonitorRepo struct{}

func (r *stubMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{ID: 1, UserID: 1, Name: "test", Status: "active"}, nil
}
func (r *stubMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) {
	return nil, monitor.ErrNotFound
}
func (r *stubMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *stubMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, monitor.ErrNotFound
}

type stubNotifyRepo struct{}

func (r *stubNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *stubNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *stubNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

func newTestHandler() http.Handler {
	return platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     true,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})
}

func TestHealthEndpoint(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "req-health")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Data.Status != "ok" {
		t.Fatalf("expected wrapped health status ok, got %q", body.Data.Status)
	}
	if body.RequestID != "req-health" {
		t.Fatalf("expected request id req-health, got %q", body.RequestID)
	}
}

func TestHealthEndpointDoesNotRequireAuth(t *testing.T) {
	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthEndpointsDoNotRequireAuth(t *testing.T) {
	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	registerBody := `{"email":"public-auth@example.com","password":"Passw0rd!","display_name":"Public Auth"}`
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRR := httptest.NewRecorder()
	router.ServeHTTP(registerRR, registerReq)
	if registerRR.Code != http.StatusCreated {
		t.Fatalf("expected register 201 without auth, got %d: %s", registerRR.Code, registerRR.Body.String())
	}

	loginBody := `{"email":"public-auth@example.com","password":"Passw0rd!"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("expected login 200 without auth, got %d: %s", loginRR.Code, loginRR.Body.String())
	}
}

func TestRegisterReturns201(t *testing.T) {
	handler := newTestHandler()
	body := `{"email":"test@example.com","password":"Passw0rd!","display_name":"Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req-register")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if resp.Data.Email != "test@example.com" {
		t.Fatalf("expected wrapped register email, got %q", resp.Data.Email)
	}
	if resp.RequestID != "req-register" {
		t.Fatalf("expected request id req-register, got %q", resp.RequestID)
	}
}

func TestLoginReturns200(t *testing.T) {
	handler := newTestHandler()
	regBody := `{"email":"login@example.com","password":"Passw0rd!","display_name":"Login"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRR := httptest.NewRecorder()
	handler.ServeHTTP(regRR, regReq)

	if regRR.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", regRR.Code)
	}

	loginBody := `{"email":"login@example.com","password":"Passw0rd!"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	handler.ServeHTTP(loginRR, loginReq)

	if loginRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", loginRR.Code, loginRR.Body.String())
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginRR.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatalf("expected token in wrapped response, got: %s", loginRR.Body.String())
	}
}

func TestMonitorsRequireAuth(t *testing.T) {
	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/monitors"},
		{http.MethodGet, "/api/v1/monitors/1/posts"},
		{http.MethodGet, "/api/v1/monitors/1/topics"},
		{http.MethodGet, "/api/v1/monitors/1/trends"},
		{http.MethodGet, "/api/v1/topics/1/trends"},
		{http.MethodGet, "/api/v1/notifications"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestUnauthorizedBusinessRouteIncludesStableErrorCode(t *testing.T) {
	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	req.Header.Set("X-Request-Id", "req-unauthorized")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	var body platformhttp.ErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error body, got %v: %s", err, rr.Body.String())
	}
	if body.Code != string(platformhttp.ErrorCodeUnauthorized) {
		t.Fatalf("expected unauthorized code, got %q", body.Code)
	}
	if body.RequestID != "req-unauthorized" {
		t.Fatalf("expected request id req-unauthorized, got %q", body.RequestID)
	}
}

func TestPublicPathDoesNotInjectUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.AuthMiddleware("test-secret", false))
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": platformruntime.UserIDFromContext(c.Request.Context()),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.UserID != 0 {
		t.Fatalf("expected anonymous public path, got user id %d", body.UserID)
	}
}

func TestJWTAuthPropagatesUserID(t *testing.T) {
	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": float64(42),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid JWT, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSmokeBypassAuth(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list monitors", http.MethodGet, "/api/v1/monitors"},
		{"list posts", http.MethodGet, "/api/v1/monitors/1/posts"},
		{"list topics", http.MethodGet, "/api/v1/monitors/1/topics"},
		{"monitor trends", http.MethodGet, "/api/v1/monitors/1/trends"},
		{"topic trends", http.MethodGet, "/api/v1/topics/1/trends"},
		{"list notifications", http.MethodGet, "/api/v1/notifications"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			var body struct {
				Data      json.RawMessage `json:"data"`
				RequestID string          `json:"request_id"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected JSON envelope, got %v: %s", err, rr.Body.String())
			}
			if len(body.Data) == 0 {
				t.Fatalf("expected wrapped data for %s, got %s", tt.path, rr.Body.String())
			}
		})
	}
}

func TestMarkNotificationReadReturnsUnifiedEnvelope(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/1/read", nil)
	req.Header.Set("X-Request-Id", "req-notify-read")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data struct {
			Read bool `json:"read"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON envelope, got %v: %s", err, rr.Body.String())
	}
	if !body.Data.Read {
		t.Fatalf("expected read=true, got %s", rr.Body.String())
	}
	if body.RequestID != "req-notify-read" {
		t.Fatalf("expected request id req-notify-read, got %q", body.RequestID)
	}
}

func TestRecoverMiddlewareReturnsUnifiedErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.RecoverMiddleware())
	r.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("X-Request-Id", "req-test-123")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}

	var body platformhttp.ErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error body, got %v: %s", err, rr.Body.String())
	}

	if body.Error != "internal server error" {
		t.Fatalf("expected unified error message, got %q", body.Error)
	}

	if body.Code != "internal_error" {
		t.Fatalf("expected unified error code, got %q", body.Code)
	}

	if body.RequestID != "req-test-123" {
		t.Fatalf("expected request id in error body, got %q", body.RequestID)
	}
}

func TestRespondAppErrorUsesCodeStatusMessageAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/app-error", func(c *gin.Context) {
		platformhttp.RespondAppError(c, platformhttp.NewAppError(
			platformhttp.ErrorCode("MONITOR_NOT_FOUND"),
			http.StatusNotFound,
			"monitor not found",
			nil,
		))
	})

	req := httptest.NewRequest(http.MethodGet, "/app-error", nil)
	req.Header.Set("X-Request-Id", "req-app-error")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var body platformhttp.ErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error body, got %v: %s", err, rr.Body.String())
	}

	if body.Code != "MONITOR_NOT_FOUND" {
		t.Fatalf("expected error code MONITOR_NOT_FOUND, got %q", body.Code)
	}
	if body.Error != "monitor not found" {
		t.Fatalf("expected message monitor not found, got %q", body.Error)
	}
	if body.RequestID != "req-app-error" {
		t.Fatalf("expected request id req-app-error, got %q", body.RequestID)
	}
}

func TestRespondErrorCodeUsesRegisteredHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/registered-error", func(c *gin.Context) {
		platformhttp.RespondErrorCode(c, platformhttp.ErrorCodeMonitorNotFound, "monitor not found", nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/registered-error", nil)
	req.Header.Set("X-Request-Id", "req-registered")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected registered 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var body platformhttp.ErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error body, got %v: %s", err, rr.Body.String())
	}
	if body.Code != string(platformhttp.ErrorCodeMonitorNotFound) {
		t.Fatalf("expected monitor not found code, got %q", body.Code)
	}
	if body.RequestID != "req-registered" {
		t.Fatalf("expected request id req-registered, got %q", body.RequestID)
	}
}

func TestRequestIDMiddlewareInjectsRequestAndTraceContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/context", func(c *gin.Context) {
		requestID := platformruntime.RequestIDFromContext(c.Request.Context())
		traceID := platformruntime.TraceIDFromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{
			"request_id": requestID,
			"trace_id":   traceID,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/context", nil)
	req.Header.Set("X-Request-Id", "req-context")
	req.Header.Set("X-Trace-Id", "trace-context")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Request-Id"); got != "req-context" {
		t.Fatalf("expected request header propagation, got %q", got)
	}
	if got := rr.Header().Get("X-Trace-Id"); got != "trace-context" {
		t.Fatalf("expected trace header propagation, got %q", got)
	}

	var body struct {
		RequestID string `json:"request_id"`
		TraceID   string `json:"trace_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.RequestID != "req-context" {
		t.Fatalf("expected request id in context, got %q", body.RequestID)
	}
	if body.TraceID != "trace-context" {
		t.Fatalf("expected trace id in context, got %q", body.TraceID)
	}
}

func TestContextMetadataMiddlewareInjectsModuleAndOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.ContextMetadataMiddleware("monitor"))
	r.GET("/metadata", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"module":   platformruntime.ModuleFromContext(c.Request.Context()),
			"operator": platformruntime.OperatorFromContext(c.Request.Context()),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/metadata", nil)
	req.Header.Set("X-Operator", "alice@example.com")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Module   string `json:"module"`
		Operator string `json:"operator"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Module != "monitor" {
		t.Fatalf("expected module monitor, got %q", body.Module)
	}
	if body.Operator != "alice@example.com" {
		t.Fatalf("expected operator alice@example.com, got %q", body.Operator)
	}
}

func TestRuntimeContextStoresUserModuleAndOperator(t *testing.T) {
	ctx := context.Background()
	ctx = platformruntime.WithUserID(ctx, 42)
	ctx = platformruntime.WithModule(ctx, "monitor")
	ctx = platformruntime.WithOperator(ctx, "alice@example.com")

	if got := platformruntime.UserIDFromContext(ctx); got != 42 {
		t.Fatalf("expected user id 42, got %d", got)
	}
	if got := platformruntime.ModuleFromContext(ctx); got != "monitor" {
		t.Fatalf("expected module monitor, got %q", got)
	}
	if got := platformruntime.OperatorFromContext(ctx); got != "alice@example.com" {
		t.Fatalf("expected operator alice@example.com, got %q", got)
	}
}

func TestRespondOKWrapsDataAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/ok", func(c *gin.Context) {
		platformhttp.RespondOK(c, gin.H{"name": "hotkey"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("X-Request-Id", "req-ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Data.Name != "hotkey" {
		t.Fatalf("expected wrapped data, got %q", body.Data.Name)
	}
	if body.RequestID != "req-ok" {
		t.Fatalf("expected request id req-ok, got %q", body.RequestID)
	}
}

func TestRespondPageWrapsPaginationAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/page", func(c *gin.Context) {
		platformhttp.RespondPage(c, []string{"a", "b"}, 2, 10, 42)
	})

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.Header.Set("X-Request-Id", "req-page")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Data      []string `json:"data"`
		Page      int      `json:"page"`
		PageSize  int      `json:"page_size"`
		Total     int      `json:"total"`
		RequestID string   `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if len(body.Data) != 2 || body.Data[0] != "a" || body.Data[1] != "b" {
		t.Fatalf("expected wrapped page data, got %#v", body.Data)
	}
	if body.Page != 2 || body.PageSize != 10 || body.Total != 42 {
		t.Fatalf("expected pagination metadata, got page=%d page_size=%d total=%d", body.Page, body.PageSize, body.Total)
	}
	if body.RequestID != "req-page" {
		t.Fatalf("expected request id req-page, got %q", body.RequestID)
	}
}
