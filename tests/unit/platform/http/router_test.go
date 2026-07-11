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

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/controller"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

type stubAuthRepo struct{ users []dto.User }

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}
func (r *stubAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (dto.User, error) {
	u := dto.User{ID: int64(len(r.users) + 1), Email: email, PasswordHash: passwordHash, DisplayName: displayName, Status: "active", PlanType: "free"}
	r.users = append(r.users, u)
	return u, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, email string) (*dto.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (r *stubAuthRepo) GetByID(_ context.Context, _ int64) (*dto.User, error) { return nil, nil }
func (r *stubAuthRepo) UpdatePassword(_ context.Context, _ int64, _ string, _ time.Time) error { return nil }
func (r *stubAuthRepo) UpdateLastLogin(_ context.Context, _ int64, _ time.Time) error { return nil }
func (r *stubAuthRepo) SetEmailVerified(_ context.Context, _ int64, _ time.Time) error { return nil }
func (r *stubAuthRepo) Transaction(_ context.Context, fn func(service.UserRepository) error) error { return fn(r) }

type stubMonitorRepo struct{}

func (r *stubMonitorRepo) Create(_ context.Context, _ int64, _ dto.CreateMonitorInput) (dto.Monitor, error) {
	return dto.Monitor{ID: 1, UserID: 1, Name: "test", Status: "active"}, nil
}
func (r *stubMonitorRepo) GetByID(_ context.Context, id int64) (*dto.Monitor, error) {
	if id == 999 {
		return nil, service.MonitorErrNotFound
	}
	return &dto.Monitor{ID: id, UserID: 1}, nil
}
func (r *stubMonitorRepo) ListByUser(_ context.Context, _ int64) ([]dto.Monitor, error) {
	return nil, nil
}
func (r *stubMonitorRepo) Update(_ context.Context, _ int64, _ int64, _ dto.UpdateMonitorInput) (dto.Monitor, error) {
	return dto.Monitor{}, service.MonitorErrNotFound
}
func (r *stubMonitorRepo) ListActive(_ context.Context) ([]dto.Monitor, error) {
	return []dto.Monitor{{ID: 1, UserID: 1, Name: "test", Status: "active"}}, nil
}
func (r *stubMonitorRepo) SetQueryEmbedding(_ context.Context, _ int64, _ pkg.Vector384) error {
	return nil
}

type stubNotifyRepo struct{}

func (r *stubNotifyRepo) ListUnread(_ context.Context, _ int64) ([]dto.Notification, error) {
	return nil, nil
}
func (r *stubNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *stubNotifyRepo) Create(_ context.Context, n dto.Notification) (dto.Notification, error) {
	return n, nil
}

type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]service.TopicSummary, error) {
	return nil, nil
}
func (s *stubTopicQueryService) GetMonitorID(_ context.Context, topicID int64) (int64, error) {
	return topicID, nil
}

type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]service.TrendPoint, error) {
	return nil, nil
}
func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]service.TrendPoint, error) {
	return nil, nil
}

func newTestHandler() http.Handler {
	return controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     true,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
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
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Data.Status != "ok" {
		t.Fatalf("expected wrapped health status ok, got %q", body.Data.Status)
	}
	if body.Code != "SUCCESS" {
		t.Fatalf("expected code SUCCESS, got %q", body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected message success, got %q", body.Message)
	}
	if body.RequestID != "req-health" {
		t.Fatalf("expected request id req-health, got %q", body.RequestID)
	}
}

func TestHealthEndpointDoesNotRequireAuth(t *testing.T) {
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
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
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
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
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if resp.Data.Email != "test@example.com" {
		t.Fatalf("expected wrapped register email, got %q", resp.Data.Email)
	}
	if resp.Code != "SUCCESS" {
		t.Fatalf("expected code SUCCESS, got %q", resp.Code)
	}
	if resp.Message != "success" {
		t.Fatalf("expected message success, got %q", resp.Message)
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
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(loginRR.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatalf("expected token in wrapped response, got: %s", loginRR.Body.String())
	}
	if resp.Code != "SUCCESS" {
		t.Fatalf("expected code SUCCESS, got %q", resp.Code)
	}
	if resp.Message != "success" {
		t.Fatalf("expected message success, got %q", resp.Message)
	}
}

func TestMonitorsRequireAuth(t *testing.T) {
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
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
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
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
	var body struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Data      json.RawMessage `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Code != string(enum.ErrorCodeUnauthorized) {
		t.Fatalf("expected unauthorized code, got %q", body.Code)
	}
	if body.Message == "" {
		t.Fatal("expected non-empty error message")
	}
	if string(body.Data) != "null" {
		t.Fatalf("expected null data on error, got %s", string(body.Data))
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
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	tokenStr, err := security.SignAccessToken(security.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
	}, "test-secret")
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
				Code      string          `json:"code"`
				Message   string          `json:"message"`
				Data      json.RawMessage `json:"data"`
				RequestID string          `json:"request_id"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected JSON envelope, got %v: %s", err, rr.Body.String())
			}
			if body.Code != "SUCCESS" {
				t.Errorf("expected SUCCESS code, got %q", body.Code)
			}
			if body.Message != "success" {
				t.Errorf("expected success message, got %q", body.Message)
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
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON envelope, got %v: %s", err, rr.Body.String())
	}
	if !body.Data.Read {
		t.Fatalf("expected read=true, got %s", rr.Body.String())
	}
	if body.Code != "SUCCESS" {
		t.Fatalf("expected SUCCESS code, got %q", body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected message success, got %q", body.Message)
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

	var body struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}

	if body.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected code INTERNAL_ERROR, got %q", body.Code)
	}
	if body.Message == "" {
		t.Fatal("expected non-empty error message")
	}
	if string(body.Data) != "null" {
		t.Fatalf("expected null data on error, got %s", string(body.Data))
	}
	if body.RequestID != "req-test-123" {
		t.Fatalf("expected request id in error body, got %q", body.RequestID)
	}
}

func TestRespondAppErrorUsesCodeStatusMessageAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.ErrorHandlerMiddleware())
	r.GET("/app-error", func(c *gin.Context) {
		c.Error(platformhttp.NewAppError(
			enum.ErrorCodeNotFound,
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

	var body struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}

	if body.Code != string(enum.ErrorCodeNotFound) {
		t.Fatalf("expected error code NOT_FOUND, got %q", body.Code)
	}
	if body.Message == "" {
		t.Fatal("expected non-empty error message")
	}
	if string(body.Data) != "null" {
		t.Fatalf("expected null data on error, got %s", string(body.Data))
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
		platformhttp.RespondErrorCode(c, enum.ErrorCodeNotFound, "monitor not found", nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/registered-error", nil)
	req.Header.Set("X-Request-Id", "req-registered")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected registered 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Code != string(enum.ErrorCodeNotFound) {
		t.Fatalf("expected NOT_FOUND, got %q", body.Code)
	}
	if body.Message == "" {
		t.Fatal("expected non-empty error message")
	}
	if string(body.Data) != "null" {
		t.Fatalf("expected null data on error, got %s", string(body.Data))
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
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
	}
	if body.Data.Name != "hotkey" {
		t.Fatalf("expected wrapped data, got %q", body.Data.Name)
	}
	if body.Code != "SUCCESS" {
		t.Fatalf("expected SUCCESS code, got %q", body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected message success, got %q", body.Message)
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
		Code      string   `json:"code"`
		Message   string   `json:"message"`
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
	if body.Code != "SUCCESS" {
		t.Fatalf("expected SUCCESS code, got %q", body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected message success, got %q", body.Message)
	}
	if body.RequestID != "req-page" {
		t.Fatalf("expected request id req-page, got %q", body.RequestID)
	}
}

func TestMonitorScopedEndpointsRejectOtherUsers(t *testing.T) {
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	tokenStr, err := security.SignAccessToken(security.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "2"},
	}, "test-secret")
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"posts", "/api/v1/monitors/1/posts"},
		{"topics", "/api/v1/monitors/1/topics"},
		{"monitor trends", "/api/v1/monitors/1/trends"},
		{"topic trends", "/api/v1/topics/1/trends"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
			}
			var body struct {
				Code      string          `json:"code"`
				Message   string          `json:"message"`
				Data      json.RawMessage `json:"data"`
				RequestID string          `json:"request_id"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
			}
			if body.Code != string(enum.ErrorCodeForbidden) {
				t.Errorf("expected FORBIDDEN code, got %q", body.Code)
			}
			if body.Message == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}

func TestMonitorScopedEndpointsReturn404ForNonexistentMonitor(t *testing.T) {
	router := controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	tokenStr, err := security.SignAccessToken(security.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "1"},
	}, "test-secret")
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"posts", "/api/v1/monitors/999/posts"},
		{"topics", "/api/v1/monitors/999/topics"},
		{"monitor trends", "/api/v1/monitors/999/trends"},
		{"topic trends", "/api/v1/topics/999/trends"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
			}
			var body struct {
				Code      string          `json:"code"`
				Message   string          `json:"message"`
				Data      json.RawMessage `json:"data"`
				RequestID string          `json:"request_id"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected JSON body, got %v: %s", err, rr.Body.String())
			}
			if body.Code != string(enum.ErrorCodeNotFound) {
				t.Errorf("expected NOT_FOUND code, got %q", body.Code)
			}
			if body.Message == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}
