package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
	"golang.org/x/crypto/bcrypt"
)

func TestMailGetEmailPreferenceRequiresAuth(t *testing.T) {
	handler, _ := setupMailTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/email", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMailGetEmailPreferenceReturnsDefaults(t *testing.T) {
	handler, authService := setupMailTest(t)
	token := loginAsUser(t, authService)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/email", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		EmailEnabled  bool `json:"emailEnabled"`
		DailyEnabled  bool `json:"dailyEnabled"`
		WeeklyEnabled bool `json:"weeklyEnabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.EmailEnabled || !body.DailyEnabled {
		t.Fatalf("expected defaults to be true, got %+v", body)
	}
}

func TestMailSetEmailPreferenceTogglesValues(t *testing.T) {
	handler, authService := setupMailTest(t)
	token := loginAsUser(t, authService)
	falseVal := false
	payload, _ := json.Marshal(map[string]*bool{"weeklyEnabled": &falseVal})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/me/email", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		EmailEnabled  bool `json:"emailEnabled"`
		DailyEnabled  bool `json:"dailyEnabled"`
		WeeklyEnabled bool `json:"weeklyEnabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.EmailEnabled || !body.DailyEnabled || body.WeeklyEnabled {
		t.Fatalf("expected only weekly disabled, got %+v", body)
	}
}

func setupMailTest(t *testing.T) (http.Handler, *serviceauth.Service) {
	t.Helper()
	authRepo := serviceauth.NewMemoryRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = authRepo.CreateUser(context.Background(), user.User{
		ID:           "usr_mail",
		Email:        "mail-test@example.com",
		PasswordHash: string(hash),
		Role:         user.RoleUser,
		Status:       user.StatusActive,
		Timezone:     "Asia/Shanghai",
		DailySendAt:  "08:30",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := serviceauth.NewService(authRepo, serviceauth.Config{AccessTokenSecret: "test-mail-secret"})
	if err != nil {
		t.Fatal(err)
	}
	emailPrefRepo := newMemoryEmailPreferenceRepo()
	router := transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
		AuthService:      authService,
		EmailPrefService: emailPrefRepo,
	})
	return router, authService
}

func loginAsUser(t *testing.T, authService *serviceauth.Service) string {
	t.Helper()
	result, err := authService.Login(context.Background(), serviceauth.LoginInput{
		Email:    "mail-test@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return result.AccessToken
}

type memoryEmailPreferenceRepo struct {
	prefs map[string]transporthttp.EmailPreference
}

func newMemoryEmailPreferenceRepo() *memoryEmailPreferenceRepo {
	return &memoryEmailPreferenceRepo{prefs: make(map[string]transporthttp.EmailPreference)}
}

func (r *memoryEmailPreferenceRepo) GetEmailPreference(userID string) (transporthttp.EmailPreference, error) {
	if pref, ok := r.prefs[userID]; ok {
		return pref, nil
	}
	return transporthttp.EmailPreference{EmailEnabled: true, DailyEnabled: true, WeeklyEnabled: true}, nil
}

func (r *memoryEmailPreferenceRepo) SetEmailPreference(userID string, pref transporthttp.EmailPreference) error {
	r.prefs[userID] = pref
	return nil
}
