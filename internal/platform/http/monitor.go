package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
)

// RegisterMonitorRoutes registers the monitor CRUD endpoints.
func RegisterMonitorRoutes(r *gin.Engine, svc *monitor.Service) {
	r.GET("/api/v1/monitors", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		monitors, err := svc.ListByUser(c.Request.Context(), userID)
		if err != nil {
			respondInternalError(c)
			return
		}

		resp := make([]MonitorResponse, len(monitors))
		for i, m := range monitors {
			resp[i] = monitorToResponse(m)
		}
		c.JSON(http.StatusOK, resp)
	})

	r.POST("/api/v1/monitors", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		var body struct {
			Name                string `json:"name" binding:"required"`
			QueryText           string `json:"query_text" binding:"required"`
			Language            string `json:"language"`
			Region              string `json:"region"`
			PollIntervalMinutes int    `json:"poll_interval_minutes" binding:"min=1"`
			AlertEnabled        bool   `json:"alert_enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}

		m, err := svc.Create(c.Request.Context(), userID, monitor.CreateMonitorInput{
			Name:                body.Name,
			QueryText:           body.QueryText,
			Language:            body.Language,
			Region:              body.Region,
			PollIntervalMinutes: body.PollIntervalMinutes,
			AlertEnabled:        body.AlertEnabled,
		})
		if err != nil {
			switch {
			case err == monitor.ErrInvalidInterval || err == monitor.ErrInvalidInput:
				respondError(c, http.StatusBadRequest, err.Error())
			default:
				respondInternalError(c)
			}
			return
		}

		c.JSON(http.StatusCreated, monitorToResponse(m))
	})

	r.GET("/api/v1/monitors/:id", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
			return
		}

		m, err := svc.GetByID(c.Request.Context(), id)
		if err != nil {
			switch {
			case err == monitor.ErrNotFound:
				respondError(c, http.StatusNotFound, "monitor not found")
			default:
				respondInternalError(c)
			}
			return
		}
		if m.UserID != userID {
			respondError(c, http.StatusForbidden, "not authorized")
			return
		}

		c.JSON(http.StatusOK, monitorToResponse(m))
	})

	r.PATCH("/api/v1/monitors/:id", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
			return
		}

		m, err := svc.GetByID(c.Request.Context(), id)
		if err != nil {
			switch {
			case err == monitor.ErrNotFound:
				respondError(c, http.StatusNotFound, "monitor not found")
			default:
				respondInternalError(c)
			}
			return
		}
		if m.UserID != userID {
			respondError(c, http.StatusForbidden, "not authorized")
			return
		}

		var body struct {
			Name                *string `json:"name"`
			QueryText           *string `json:"query_text"`
			Language            *string `json:"language"`
			Region              *string `json:"region"`
			PollIntervalMinutes *int    `json:"poll_interval_minutes"`
			AlertEnabled        *bool   `json:"alert_enabled"`
			Status              *string `json:"status"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}

		updated, err := svc.Update(c.Request.Context(), id, monitor.UpdateMonitorInput{
			Name:                body.Name,
			QueryText:           body.QueryText,
			Language:            body.Language,
			Region:              body.Region,
			PollIntervalMinutes: body.PollIntervalMinutes,
			AlertEnabled:        body.AlertEnabled,
			Status:              body.Status,
		})
		if err != nil {
			switch {
			case err == monitor.ErrInvalidInterval:
				respondError(c, http.StatusBadRequest, err.Error())
			default:
				respondInternalError(c)
			}
			return
		}

		c.JSON(http.StatusOK, monitorToResponse(updated))
	})
}

type MonitorResponse struct {
	ID                  int64  `json:"id"`
	UserID              int64  `json:"user_id"`
	Name                string `json:"name"`
	QueryText           string `json:"query_text"`
	Language            string `json:"language"`
	Region              string `json:"region"`
	Status              string `json:"status"`
	PollIntervalMinutes int    `json:"poll_interval_minutes"`
	AlertEnabled        bool   `json:"alert_enabled"`
}

func monitorToResponse(m monitor.Monitor) MonitorResponse {
	return MonitorResponse{
		ID: m.ID, UserID: m.UserID, Name: m.Name,
		QueryText: m.QueryText, Language: m.Language, Region: m.Region,
		Status: m.Status, PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled: m.AlertEnabled,
	}
}
