package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
)

// RegisterMonitorRoutes registers the monitor CRUD endpoints.
func RegisterMonitorRoutes(r *gin.Engine, svc *monitor.Service) {
	r.GET("/api/v1/monitors", listMonitorsHandler(svc))
	r.POST("/api/v1/monitors", createMonitorHandler(svc))
	r.GET("/api/v1/monitors/:id", getMonitorHandler(svc))
	r.PATCH("/api/v1/monitors/:id", updateMonitorHandler(svc))
}

type MonitorData struct {
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

func monitorToResponse(m monitor.Monitor) MonitorData {
	return MonitorData{
		ID: m.ID, UserID: m.UserID, Name: m.Name,
		QueryText: m.QueryText, Language: m.Language, Region: m.Region,
		Status: m.Status, PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled: m.AlertEnabled,
	}
}

// listMonitorsHandler godoc
// @Summary List monitors
// @ID list-monitors
// @Tags monitors
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MonitorListResponse
// @Failure 401 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors [get]
func listMonitorsHandler(svc *monitor.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		resp := make([]MonitorData, len(monitors))
		for i, m := range monitors {
			resp[i] = monitorToResponse(m)
		}
		RespondOK(c, resp)
	}
}

// createMonitorHandler godoc
// @Summary Create monitor
// @ID create-monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateMonitorRequest true "Monitor payload"
// @Success 201 {object} MonitorResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors [post]
func createMonitorHandler(svc *monitor.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		var body CreateMonitorRequest
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

		RespondCreated(c, monitorToResponse(m))
	}
}

// getMonitorHandler godoc
// @Summary Get monitor
// @ID get-monitor
// @Tags monitors
// @Produce json
// @Security BearerAuth
// @Param id path int true "Monitor ID"
// @Success 200 {object} MonitorResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 403 {object} ErrorBody
// @Failure 404 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors/{id} [get]
func getMonitorHandler(svc *monitor.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		RespondOK(c, monitorToResponse(m))
	}
}

// updateMonitorHandler godoc
// @Summary Update monitor
// @ID update-monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Monitor ID"
// @Param body body UpdateMonitorRequest true "Monitor update payload"
// @Success 200 {object} MonitorResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 403 {object} ErrorBody
// @Failure 404 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors/{id} [patch]
func updateMonitorHandler(svc *monitor.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		var body UpdateMonitorRequest
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

		RespondOK(c, monitorToResponse(updated))
	}
}
