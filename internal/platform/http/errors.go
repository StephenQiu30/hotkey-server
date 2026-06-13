package http

// ErrorBody is the unified error response format for all API endpoints.
// Matches ADR-007 OpenAPI conventions.
type ErrorBody struct {
	Error string `json:"error" doc:"Human-readable error message"`
	Code  string `json:"code,omitempty" doc:"Optional machine-readable error code"`
}
