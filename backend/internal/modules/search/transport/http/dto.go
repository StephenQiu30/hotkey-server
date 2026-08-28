package http

import (
	"time"

	searchapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/application"
)

type SearchResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type SearchEmptyResponseDTO struct{}

type SearchItemResponseDTO struct {
	Type             string    `json:"type"`
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	Snippet          string    `json:"snippet"`
	TitleHighlight   string    `json:"title_highlight"`
	SnippetHighlight string    `json:"snippet_highlight"`
	Status           string    `json:"status"`
	OccurredAt       time.Time `json:"occurred_at"`
	Score            float64   `json:"score"`
}

type SearchPageResponseDTO struct {
	Items      []SearchItemResponseDTO `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func searchPageResponse(result searchapplication.Result) SearchPageResponseDTO {
	response := SearchPageResponseDTO{Items: make([]SearchItemResponseDTO, 0, len(result.Items)), NextCursor: result.NextCursor}
	for _, item := range result.Items {
		response.Items = append(response.Items, SearchItemResponseDTO{
			Type: string(item.Type), ID: item.ID, Title: item.Title, Snippet: item.Snippet,
			TitleHighlight: item.TitleHighlight, SnippetHighlight: item.SnippetHighlight,
			Status: item.Status, OccurredAt: item.OccurredAt, Score: item.Score,
		})
	}
	return response
}
