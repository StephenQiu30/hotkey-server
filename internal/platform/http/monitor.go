package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
)

// RegisterMonitorRoutes registers the monitor CRUD endpoints.
func RegisterMonitorRoutes(api huma.API, svc *monitor.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-monitors",
		Method:      http.MethodGet,
		Path:        "/api/v1/monitors",
		Summary:     "List monitors",
		Description: "Returns all monitors for the authenticated user.",
		Tags:        []string{"monitors"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *struct{}) (*ListMonitorsOutput, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		monitors, err := svc.ListByUser(ctx, userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}

		resp := make([]MonitorResponse, len(monitors))
		for i, m := range monitors {
			resp[i] = monitorToResponse(m)
		}
		return &ListMonitorsOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-monitor",
		Method:      http.MethodPost,
		Path:        "/api/v1/monitors",
		Summary:     "Create a monitor",
		Description: "Creates a new monitoring configuration for the authenticated user.",
		Tags:        []string{"monitors"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{400, 401, 500},
	}, func(ctx context.Context, input *CreateMonitorInput) (*CreateMonitorOutput, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		m, err := svc.Create(ctx, userID, monitor.CreateMonitorInput{
			Name:                input.Body.Name,
			QueryText:           input.Body.QueryText,
			Language:            input.Body.Language,
			Region:              input.Body.Region,
			PollIntervalMinutes: input.Body.PollIntervalMinutes,
			AlertEnabled:        input.Body.AlertEnabled,
		})
		if err != nil {
			switch {
			case err == monitor.ErrInvalidInterval || err == monitor.ErrInvalidInput:
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}

		return &CreateMonitorOutput{Body: monitorToResponse(m)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-monitor",
		Method:      http.MethodGet,
		Path:        "/api/v1/monitors/{id}",
		Summary:     "Get a monitor",
		Description: "Returns a single monitor by ID.",
		Tags:        []string{"monitors"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 403, 404, 500},
	}, func(ctx context.Context, input *GetMonitorInput) (*GetMonitorOutput, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		m, err := svc.GetByID(ctx, input.ID)
		if err != nil {
			switch {
			case err == monitor.ErrNotFound:
				return nil, huma.Error404NotFound("monitor not found")
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}
		if m.UserID != userID {
			return nil, huma.Error403Forbidden("not authorized")
		}

		return &GetMonitorOutput{Body: monitorToResponse(m)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-monitor",
		Method:      http.MethodPatch,
		Path:        "/api/v1/monitors/{id}",
		Summary:     "Update a monitor",
		Description: "Updates an existing monitor configuration.",
		Tags:        []string{"monitors"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{400, 401, 403, 404, 500},
	}, func(ctx context.Context, input *UpdateMonitorInput) (*UpdateMonitorOutput, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		m, err := svc.GetByID(ctx, input.ID)
		if err != nil {
			switch {
			case err == monitor.ErrNotFound:
				return nil, huma.Error404NotFound("monitor not found")
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}
		if m.UserID != userID {
			return nil, huma.Error403Forbidden("not authorized")
		}

		updated, err := svc.Update(ctx, input.ID, monitor.UpdateMonitorInput{
			Name:                input.Body.Name,
			QueryText:           input.Body.QueryText,
			Language:            input.Body.Language,
			Region:              input.Body.Region,
			PollIntervalMinutes: input.Body.PollIntervalMinutes,
			AlertEnabled:        input.Body.AlertEnabled,
			Status:              input.Body.Status,
		})
		if err != nil {
			switch {
			case err == monitor.ErrInvalidInterval:
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}

		return &UpdateMonitorOutput{Body: monitorToResponse(updated)}, nil
	})
}

// --- Input / Output types ---

type CreateMonitorInput struct {
	Body struct {
		Name                string `json:"name" validate:"required" doc:"Monitor name"`
		QueryText           string `json:"query_text" validate:"required" doc:"Search query text"`
		Language            string `json:"language" doc:"Language filter"`
		Region              string `json:"region" doc:"Region filter"`
		PollIntervalMinutes int    `json:"poll_interval_minutes" validate:"min=1" doc:"Polling interval in minutes"`
		AlertEnabled        bool   `json:"alert_enabled" doc:"Whether alerts are enabled"`
	}
}

type CreateMonitorOutput struct{ Body MonitorResponse }
type ListMonitorsOutput struct{ Body []MonitorResponse }

type GetMonitorInput struct {
	ID int64 `path:"id" validate:"required" doc:"Monitor ID"`
}

type GetMonitorOutput struct{ Body MonitorResponse }

type UpdateMonitorInput struct {
	ID   int64 `path:"id" validate:"required" doc:"Monitor ID"`
	Body struct {
		Name                *string `json:"name,omitempty" doc:"Monitor name"`
		QueryText           *string `json:"query_text,omitempty" doc:"Search query text"`
		Language            *string `json:"language,omitempty" doc:"Language filter"`
		Region              *string `json:"region,omitempty" doc:"Region filter"`
		PollIntervalMinutes *int    `json:"poll_interval_minutes,omitempty" doc:"Polling interval in minutes"`
		AlertEnabled        *bool   `json:"alert_enabled,omitempty" doc:"Whether alerts are enabled"`
		Status              *string `json:"status,omitempty" doc:"Monitor status"`
	}
}

type UpdateMonitorOutput struct{ Body MonitorResponse }

type MonitorResponse struct {
	ID                  int64  `json:"id" doc:"Monitor ID"`
	UserID              int64  `json:"user_id" doc:"Owner user ID"`
	Name                string `json:"name" doc:"Monitor name"`
	QueryText           string `json:"query_text" doc:"Search query text"`
	Language            string `json:"language" doc:"Language filter"`
	Region              string `json:"region" doc:"Region filter"`
	Status              string `json:"status" doc:"Monitor status"`
	PollIntervalMinutes int    `json:"poll_interval_minutes" doc:"Polling interval in minutes"`
	AlertEnabled        bool   `json:"alert_enabled" doc:"Whether alerts are enabled"`
}

func monitorToResponse(m monitor.Monitor) MonitorResponse {
	return MonitorResponse{
		ID: m.ID, UserID: m.UserID, Name: m.Name,
		QueryText: m.QueryText, Language: m.Language, Region: m.Region,
		Status: m.Status, PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled: m.AlertEnabled,
	}
}
