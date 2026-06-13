package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterHealthRoutes registers the /healthz endpoint.
func RegisterHealthRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
		Description: "Returns 200 OK if the server is running.",
		Tags:        []string{"health"},
	}, func(ctx context.Context, input *struct{}) (*HealthOutput, error) {
		return &HealthOutput{Body: HealthBody{Status: "ok"}}, nil
	})
}

// HealthBody is the health check response body.
type HealthBody struct {
	Status string `json:"status" example:"ok" doc:"Server status"`
}

// HealthOutput is the health check response.
type HealthOutput struct {
	Body HealthBody
}
