package http

import (
	"context"
	"strings"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type InstantSearchRequest struct {
	Query       string   `json:"query" binding:"required,max=200" example:"Claude"`
	SourceTypes []string `json:"source_types,omitempty" binding:"max=9,dive,oneof=x bing_grounding google_agent_search duckduckgo hacker_news sogou bilibili weibo rss"`
	Limit       int      `json:"limit,omitempty" binding:"omitempty,gte=1,lte=50" default:"20"`
}

type InstantSearchMetricsResponse struct {
	ViewCount    *int64 `json:"view_count" extensions:"x-nullable"`
	LikeCount    *int64 `json:"like_count" extensions:"x-nullable"`
	CommentCount *int64 `json:"comment_count" extensions:"x-nullable"`
	ShareCount   *int64 `json:"share_count" extensions:"x-nullable"`
}

// HotspotCardResponse is the flat card contract shared conceptually by live
// search and the persisted hotspot radar. Analysis fields are never nested.
type HotspotCardResponse struct {
	SourceType       string                       `json:"source_type"`
	SourceName       string                       `json:"source_name"`
	ExternalID       string                       `json:"external_id"`
	ContentType      string                       `json:"content_type"`
	Title            string                       `json:"title"`
	Summary          string                       `json:"summary"`
	CanonicalURL     string                       `json:"canonical_url"`
	Author           string                       `json:"author"`
	PublishedAt      *time.Time                   `json:"published_at" extensions:"x-nullable"`
	DiscoveredAt     time.Time                    `json:"discovered_at"`
	Metrics          InstantSearchMetricsResponse `json:"metrics"`
	HeatScore        float64                      `json:"heat_score"`
	QualityState     string                       `json:"quality_state" enums:"credible,suspicious,unavailable"`
	Relevance        int                          `json:"relevance" minimum:"0" maximum:"100"`
	RelevanceReason  string                       `json:"relevance_reason"`
	KeywordMentioned bool                         `json:"keyword_mentioned"`
	Importance       string                       `json:"importance" enums:"low,medium,high,urgent"`
}

type InstantSearchSourceStatusResponse struct {
	SourceType  string `json:"source_type"`
	SourceName  string `json:"source_name"`
	State       string `json:"state" enums:"success,empty,partial,failed,unavailable"`
	ResultCount int    `json:"result_count"`
	ErrorCode   string `json:"error_code,omitempty"`
}

type InstantSearchResponse struct {
	Query          string                              `json:"query"`
	SearchedAt     time.Time                           `json:"searched_at"`
	Results        []HotspotCardResponse               `json:"results"`
	SourceStatuses []InstantSearchSourceStatusResponse `json:"source_statuses"`
}

type instantSearchService interface {
	Search(context.Context, sourceapplication.InstantSearchInput) (sourceapplication.InstantSearchResult, error)
}

type InstantSearchHandler struct{ service instantSearchService }

func NewInstantSearchHandler(service instantSearchService) *InstantSearchHandler {
	return &InstantSearchHandler{service: service}
}

// Search performs a bounded, non-persistent query against configured source
// connectors and always reports each requested source's actual status.
// @Summary Search configured hotspot sources
// @Tags search
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body InstantSearchRequest true "instant search"
// @Success 200 {object} SourceResult[InstantSearchResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/search [post]
func (handler *InstantSearchHandler) Search(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	var request InstantSearchRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Query) == "" {
		if err == nil {
			err = &instantSearchValidationError{}
		}
		return invalidRequest(err)
	}
	result, err := handler.service.Search(c.Request.Context(), sourceapplication.InstantSearchInput{
		Subject: subject, Query: request.Query,
		SourceTypes: append([]string(nil), request.SourceTypes...), Limit: request.Limit,
	})
	if err != nil {
		return err
	}
	httptransport.OK(c, instantSearchResponse(result))
	return nil
}

type instantSearchValidationError struct{}

func (*instantSearchValidationError) Error() string { return "invalid instant search request" }

func instantSearchResponse(result sourceapplication.InstantSearchResult) InstantSearchResponse {
	response := InstantSearchResponse{
		Query: result.Query, SearchedAt: result.SearchedAt,
		Results:        make([]HotspotCardResponse, 0, len(result.Items)),
		SourceStatuses: make([]InstantSearchSourceStatusResponse, 0, len(result.SourceStatuses)),
	}
	for _, item := range result.Items {
		response.Results = append(response.Results, HotspotCardResponse{
			SourceType: item.SourceType, SourceName: item.SourceName, ExternalID: item.ExternalID,
			ContentType: item.ContentType, Title: item.Title, Summary: item.Summary,
			CanonicalURL: item.CanonicalURL, Author: item.Author, PublishedAt: item.PublishedAt,
			DiscoveredAt: item.DiscoveredAt, Metrics: InstantSearchMetricsResponse{
				ViewCount: item.Metrics.ViewCount, LikeCount: item.Metrics.LikeCount,
				CommentCount: item.Metrics.CommentCount, ShareCount: item.Metrics.ShareCount,
			}, HeatScore: item.HeatScore, QualityState: item.QualityState,
			Relevance: item.Relevance, RelevanceReason: item.RelevanceReason,
			KeywordMentioned: item.KeywordMentioned, Importance: item.Importance,
		})
	}
	for _, status := range result.SourceStatuses {
		response.SourceStatuses = append(response.SourceStatuses, InstantSearchSourceStatusResponse{
			SourceType: status.SourceType, SourceName: status.SourceName, State: string(status.State),
			ResultCount: status.ResultCount, ErrorCode: status.ErrorCode,
		})
	}
	return response
}

func RegisterInstantSearchRoutes(router *gin.Engine, service instantSearchService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewInstantSearchHandler(service)
	api := router.Group("/api/v1", httptransport.RequireAuthentication(authenticator))
	api.POST("/search", httptransport.Wrap(handler.Search))
}
