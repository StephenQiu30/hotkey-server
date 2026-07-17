package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func TestMetricCapabilityRoutesRequireAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterMetricCapabilityRoutes(router, &metricCapabilityServiceFake{}, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/metric-capability-profiles", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer viewer")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMetricCapabilityCreateUsesFixedRequestShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &metricCapabilityServiceFake{}
	router := gin.New()
	RegisterMetricCapabilityRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleAdmin}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/metric-capability-profiles", strings.NewReader(`{"source_type":"rss","profile_version":"v1","supports_views":true,"independence_strategy":"source_connection","normalization_window_hours":24,"credibility_weight":0.8,"max_single_item_contribution":50}`))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if service.created.Profile.ProfileVersion != "v1" || service.created.Profile.MaxSingleItemContribution != 50 || !service.created.Profile.SupportsViews {
		t.Fatalf("CreateDraft input = %#v", service.created)
	}
}

type metricCapabilityServiceFake struct {
	created sourceapplication.CreateMetricCapabilityInput
}

func (service *metricCapabilityServiceFake) CreateDraft(_ context.Context, input sourceapplication.CreateMetricCapabilityInput) (*domain.MetricCapabilityProfile, error) {
	service.created = input
	profile := input.Profile
	profile.ID, profile.Version, profile.Status = 1, 1, domain.MetricCapabilityDraft
	return &profile, nil
}

func (service *metricCapabilityServiceFake) Publish(context.Context, sourceapplication.MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error) {
	return &domain.MetricCapabilityProfile{}, nil
}

func (service *metricCapabilityServiceFake) Archive(context.Context, sourceapplication.MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error) {
	return &domain.MetricCapabilityProfile{}, nil
}
