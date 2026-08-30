package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/openapi"
)

type RuntimeCompatibilityCheck func(context.Context) error

type runtimeCompatibilityCheck = RuntimeCompatibilityCheck

type runtimeCompatibilityReadiness struct {
	readiness httptransport.Readiness
	checks    map[string]runtimeCompatibilityCheck
}

const supportedOpenAPIContractVersion = "1.0"

var runtimeCompatibilityOrder = []string{"configuration", "schema", "openapi"}

func newRuntimeCompatibilityReadiness(readiness httptransport.Readiness, checks map[string]runtimeCompatibilityCheck) httptransport.Readiness {
	return NewRuntimeCompatibilityReadiness(readiness, checks)
}

func NewRuntimeCompatibilityReadiness(readiness httptransport.Readiness, checks map[string]RuntimeCompatibilityCheck) httptransport.Readiness {
	return &runtimeCompatibilityReadiness{readiness: readiness, checks: checks}
}

func (readiness *runtimeCompatibilityReadiness) Check(ctx context.Context) error {
	if readiness == nil || readiness.readiness == nil {
		return errors.New("runtime compatibility readiness is not configured")
	}
	if err := readiness.readiness.Check(ctx); err != nil {
		return err
	}
	for _, name := range runtimeCompatibilityOrder {
		check := readiness.checks[name]
		if check == nil {
			return fmt.Errorf("runtime compatibility check is missing: %s", name)
		}
		if err := check(ctx); err != nil {
			return fmt.Errorf("runtime compatibility check failed: %s", name)
		}
	}
	return nil
}

func runtimeConfigurationCompatibilityCheck(cfg config.Config) runtimeCompatibilityCheck {
	return RuntimeConfigurationCompatibilityCheck(cfg)
}

func RuntimeConfigurationCompatibilityCheck(cfg config.Config) RuntimeCompatibilityCheck {
	return func(context.Context) error {
		if err := cfg.ValidateRuntime(); err != nil {
			return err
		}
		role, err := ParseRole(cfg.Role)
		if err != nil {
			return err
		}
		if role.StartsAPI() {
			return cfg.ValidateAuthenticationRuntime()
		}
		return nil
	}
}

func verifyEmbeddedOpenAPICompatibility() error {
	return VerifyEmbeddedOpenAPICompatibility()
}

func VerifyEmbeddedOpenAPICompatibility() error {
	return verifyOpenAPICompatibility(openapi.SwaggerInfo.ReadDoc())
}

func verifyOpenAPICompatibility(document string) error {
	var contract struct {
		Swagger string `json:"swagger"`
		Info    struct {
			Version string `json:"version"`
		} `json:"info"`
		Paths               map[string]json.RawMessage `json:"paths"`
		SecurityDefinitions map[string]json.RawMessage `json:"securityDefinitions"`
	}
	if err := json.Unmarshal([]byte(document), &contract); err != nil {
		return errors.New("embedded OpenAPI is not valid JSON")
	}
	if contract.Swagger != "2.0" || contract.Info.Version != supportedOpenAPIContractVersion || len(contract.Paths) == 0 {
		return errors.New("embedded OpenAPI contract metadata is incompatible")
	}
	if _, found := contract.Paths["/api/v1/capabilities"]; !found {
		return errors.New("embedded OpenAPI capabilities contract is missing")
	}
	if _, found := contract.SecurityDefinitions["BearerAuth"]; !found {
		return errors.New("embedded OpenAPI authentication contract is missing")
	}
	return nil
}
