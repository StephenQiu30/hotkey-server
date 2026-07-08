package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// HotEventManager defines the read operations needed for the hot event API.
type HotEventManager interface {
	ListEvents(ctx context.Context, filter service.HotEventListFilter) ([]*dto.HotEvent, int64, error)
	GetEventByID(ctx context.Context, id int64) (*dto.HotEvent, error)
	ListEventPosts(ctx context.Context, id int64) ([]service.PostBrief, error)
	GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error)
}

// RegisterTrendingRoutes registers the trending and hot event API endpoints.
func RegisterTrendingRoutes(r *gin.Engine, mgr HotEventManager) {
	r.GET("/api/v1/trending", listTrendingHandler(mgr))
	r.GET("/api/v1/hot-events", listHotEventsHandler(mgr))
	r.GET("/api/v1/hot-events/:id", getHotEventHandler(mgr))
	r.GET("/api/v1/hot-events/:id/posts", getHotEventPostsHandler(mgr))
}

// --- Handlers ---

func listTrendingHandler(mgr HotEventManager) gin.HandlerFunc {
	type trendingItem struct {
		Platform string  `json:"platform"`
		Title    string  `json:"title"`
		Rank     int     `json:"rank"`
		Heat     float64 `json:"heat"`
		URL      string  `json:"url"`
	}

	return func(c *gin.Context) {
		platform := c.Query("platform")
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		filter := service.HotEventListFilter{
			Status:   service.StatusActive,
			Platform: platform,
			Sort:     "heat_score",
			Limit:    limit,
		}

		events, _, err := mgr.ListEvents(c.Request.Context(), filter)
		if err != nil {
			_ = c.Error(fmt.Errorf("list trending: %w", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trending data"})
			return
		}

		items := make([]trendingItem, 0, len(events))
		for _, ev := range events {
			items = append(items, trendingItem{
				Platform: ev.Platform,
				Title:    ev.Name,
				Rank:     0,
				Heat:     ev.HeatScore,
				URL:      "",
			})
		}

		c.JSON(http.StatusOK, gin.H{"data": items})
	}
}

func listHotEventsHandler(mgr HotEventManager) gin.HandlerFunc {
	type hotEventItem struct {
		ID        int64   `json:"id"`
		Name      string  `json:"name"`
		HeatScore float64 `json:"heat_score"`
		Platform  string  `json:"platform"`
		Trend     string  `json:"trend"`
		Summary   string  `json:"summary"`
		Category  string  `json:"category"`
		Status    string  `json:"status"`
	}

	return func(c *gin.Context) {
		filter := service.HotEventListFilter{
			Status:   c.DefaultQuery("status", service.StatusActive),
			Platform: c.Query("platform"),
			Sort:     c.DefaultQuery("sort", "heat_score"),
			Limit:    20,
		}
		if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 {
			filter.Limit = l
		}

		events, total, err := mgr.ListEvents(c.Request.Context(), filter)
		if err != nil {
			_ = c.Error(fmt.Errorf("list hot events: %w", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch hot events"})
			return
		}

		items := make([]hotEventItem, 0, len(events))
		for _, ev := range events {
			items = append(items, hotEventItem{
				ID:        ev.ID,
				Name:      ev.Name,
				HeatScore: ev.HeatScore,
				Platform:  ev.Platform,
				Trend:     ev.Trend,
				Summary:   ev.Summary,
				Category:  ev.Category,
				Status:    ev.Status,
			})
		}

		c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"total": total}})
	}
}

func getHotEventHandler(mgr HotEventManager) gin.HandlerFunc {
	type eventDetail struct {
		ID          int64                     `json:"id"`
		Name        string                    `json:"name"`
		HeatScore   float64                   `json:"heat_score"`
		Platform    string                    `json:"platform"`
		Trend       string                    `json:"trend"`
		FirstSeenAt time.Time                 `json:"first_seen_at"`
		LastSeenAt  time.Time                 `json:"last_seen_at"`
		Summary     string                    `json:"summary"`
		Category    string                    `json:"category"`
		Status      string                    `json:"status"`
		Platforms   []*dto.EventPlatform `json:"platforms,omitempty"`
	}

	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
			return
		}

		ev, err := mgr.GetEventByID(c.Request.Context(), id)
		if err != nil {
			if err == service.HotEventErrNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "hot event not found"})
				return
			}
			_ = c.Error(fmt.Errorf("get hot event %d: %w", id, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch hot event"})
			return
		}

		detail := eventDetail{
			ID:          ev.ID,
			Name:        ev.Name,
			HeatScore:   ev.HeatScore,
			Platform:    ev.Platform,
			Trend:       ev.Trend,
			FirstSeenAt: ev.FirstSeenAt,
			LastSeenAt:  ev.LastSeenAt,
			Summary:     ev.Summary,
			Category:    ev.Category,
			Status:      ev.Status,
		}

		// Fetch platform details
		platforms, err := mgr.GetPlatforms(c.Request.Context(), ev.ID)
		if err == nil {
			detail.Platforms = platforms
		}

		c.JSON(http.StatusOK, gin.H{"data": detail})
	}
}

func getHotEventPostsHandler(mgr HotEventManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
			return
		}

		// Verify event exists
		if _, err := mgr.GetEventByID(c.Request.Context(), id); err != nil {
			if err == service.HotEventErrNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "hot event not found"})
				return
			}
			_ = c.Error(fmt.Errorf("get hot event posts %d: %w", id, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch posts"})
			return
		}

		posts, err := mgr.ListEventPosts(c.Request.Context(), id)
		if err != nil {
			_ = c.Error(fmt.Errorf("list event posts %d: %w", id, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch posts"})
			return
		}

		if posts == nil {
			posts = []service.PostBrief{}
		}

		c.JSON(http.StatusOK, gin.H{"data": posts})
	}
}
