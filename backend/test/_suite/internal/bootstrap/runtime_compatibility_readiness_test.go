package bootstrap

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"go.uber.org/zap"
)

func TestRuntimeCompatibilityReadinessRejectsEveryIncompatibleContractBeforeTraffic(t *testing.T) {
	t.Parallel()

	for _, contract := range []string{"schema", "openapi", "configuration"} {
		contract := contract
		t.Run(contract, func(t *testing.T) {
			t.Parallel()

			checks := compatibleRuntimeChecks()
			checks[contract] = func(context.Context) error {
				return errors.New(contract + " password=must-not-leak is incompatible")
			}
			readiness := newRuntimeCompatibilityReadiness(
				httptransport.ReadinessFunc(func(context.Context) error { return nil }),
				checks,
			)
			metrics, err := observability.NewMetrics()
			if err != nil {
				t.Fatal(err)
			}
			telemetry, err := observability.NewTelemetry(config.Default())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = telemetry.Shutdown(context.Background()) }()
			router := httptransport.NewRouter(
				readiness,
				metrics,
				telemetry,
				zap.NewNop(),
				config.Default(),
			)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil))
			if response.Code != stdhttp.StatusServiceUnavailable {
				t.Fatalf("%s incompatible readiness status = %d, want 503", contract, response.Code)
			}
			for _, forbidden := range []string{contract, "password", "must-not-leak", "incompatible"} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("%s incompatible readiness leaked %q: %s", contract, forbidden, response.Body.String())
				}
			}
		})
	}
}

func TestRuntimeCompatibilityReadinessRecoversOnlyAfterAllContractsMatch(t *testing.T) {
	t.Parallel()

	blocked := true
	checks := compatibleRuntimeChecks()
	checks["openapi"] = func(context.Context) error {
		if blocked {
			return errors.New("openapi mismatch")
		}
		return nil
	}
	readiness := newRuntimeCompatibilityReadiness(
		httptransport.ReadinessFunc(func(context.Context) error { return nil }),
		checks,
	)

	if err := readiness.Check(context.Background()); err == nil {
		t.Fatal("Check() error = nil while OpenAPI is incompatible")
	}
	blocked = false
	if err := readiness.Check(context.Background()); err != nil {
		t.Fatalf("Check() after compatible rollback error = %v", err)
	}
}

func TestEmbeddedOpenAPICompatibilityRejectsMalformedOrWrongVersionDocuments(t *testing.T) {
	t.Parallel()

	for _, document := range []string{
		`not-json`,
		`{"swagger":"3.0","info":{"version":"1.0"},"paths":{"/readyz":{}}}`,
		`{"swagger":"2.0","info":{"version":"2.0"},"paths":{"/api/v1/capabilities":{}},"securityDefinitions":{"BearerAuth":{}}}`,
		`{"swagger":"2.0","info":{"version":""},"paths":{"/readyz":{}}}`,
		`{"swagger":"2.0","info":{"version":"1.0"},"paths":{}}`,
	} {
		if err := verifyOpenAPICompatibility(document); err == nil {
			t.Fatalf("verifyOpenAPICompatibility(%q) error = nil", document)
		}
	}
	if err := verifyEmbeddedOpenAPICompatibility(); err != nil {
		t.Fatalf("verifyEmbeddedOpenAPICompatibility() error = %v", err)
	}
}

func compatibleRuntimeChecks() map[string]runtimeCompatibilityCheck {
	return map[string]runtimeCompatibilityCheck{
		"schema":        func(context.Context) error { return nil },
		"openapi":       func(context.Context) error { return nil },
		"configuration": func(context.Context) error { return nil },
	}
}
