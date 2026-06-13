package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// RegisterContentRoutes registers the content (posts) endpoints.
func RegisterContentRoutes(api huma.API, svc content.PostQueryService) {
	huma.Register(api, huma.Operation{
		OperationID: "list-posts",
		Method:      http.MethodGet,
		Path:        "/api/v1/monitors/{id}/posts",
		Summary:     "List posts for a monitor",
		Description: "Returns posts collected by the specified monitor.",
		Tags:        []string{"content"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *ListPostsInput) (*ListPostsOutput, error) {
		if _, ok := userIDFromCtx(ctx); !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		posts, err := svc.ListPostsByMonitor(input.ID, input.Limit, input.Offset)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		if posts == nil {
			posts = []content.PostSummary{}
		}

		return &ListPostsOutput{Body: posts}, nil
	})
}

type ListPostsInput struct {
	ID     int64 `path:"id" validate:"required" doc:"Monitor ID"`
	Limit  int   `query:"limit" default:"20" doc:"Maximum number of posts to return"`
	Offset int   `query:"offset" default:"0" doc:"Number of posts to skip"`
}

type ListPostsOutput struct {
	Body []content.PostSummary
}
