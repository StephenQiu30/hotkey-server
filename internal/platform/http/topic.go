package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// RegisterTopicRoutes registers the topic endpoints.
func RegisterTopicRoutes(api huma.API, svc topic.TopicQueryService) {
	huma.Register(api, huma.Operation{
		OperationID: "list-topics",
		Method:      http.MethodGet,
		Path:        "/api/v1/monitors/{id}/topics",
		Summary:     "List topics for a monitor",
		Description: "Returns topics extracted from posts collected by the specified monitor.",
		Tags:        []string{"topics"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *ListTopicsInput) (*ListTopicsOutput, error) {
		if _, ok := userIDFromCtx(ctx); !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		topics, err := svc.ListByMonitor(input.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		if topics == nil {
			topics = []topic.TopicSummary{}
		}

		return &ListTopicsOutput{Body: topics}, nil
	})
}

type ListTopicsInput struct {
	ID int64 `path:"id" validate:"required" doc:"Monitor ID"`
}

type ListTopicsOutput struct {
	Body []topic.TopicSummary
}
