package http

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// RegisterTrendRoutes registers the trend endpoints.
func RegisterTrendRoutes(api huma.API, svc trend.TrendQueryService) {
	huma.Register(api, huma.Operation{
		OperationID: "get-monitor-trends",
		Method:      http.MethodGet,
		Path:        "/api/v1/monitors/{id}/trends",
		Summary:     "Get trends for a monitor",
		Description: "Returns trend data points for the specified monitor.",
		Tags:        []string{"trends"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *MonitorTrendsInput) (*TrendsOutput, error) {
		if _, ok := userIDFromCtx(ctx); !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		since := parseSince(input.Since)
		points, err := svc.GetMonitorTrends(input.ID, since)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		if points == nil {
			points = []trend.TrendPoint{}
		}

		return &TrendsOutput{Body: points}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-topic-trends",
		Method:      http.MethodGet,
		Path:        "/api/v1/topics/{id}/trends",
		Summary:     "Get trends for a topic",
		Description: "Returns trend data points for the specified topic.",
		Tags:        []string{"trends"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *TopicTrendsInput) (*TrendsOutput, error) {
		if _, ok := userIDFromCtx(ctx); !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		since := parseSince(input.Since)
		points, err := svc.GetTopicTrends(input.ID, since)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		if points == nil {
			points = []trend.TrendPoint{}
		}

		return &TrendsOutput{Body: points}, nil
	})
}

type MonitorTrendsInput struct {
	ID    int64  `path:"id" validate:"required" doc:"Monitor ID"`
	Since string `query:"since" doc:"ISO 8601 timestamp (defaults to 24h ago)"`
}

type TopicTrendsInput struct {
	ID    int64  `path:"id" validate:"required" doc:"Topic ID"`
	Since string `query:"since" doc:"ISO 8601 timestamp (defaults to 24h ago)"`
}

type TrendsOutput struct {
	Body []trend.TrendPoint
}

func parseSince(s string) time.Time {
	if s == "" {
		return time.Now().Add(-24 * time.Hour)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().Add(-24 * time.Hour)
	}
	return t
}
