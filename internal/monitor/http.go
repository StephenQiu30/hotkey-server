package monitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Handler provides HTTP endpoints for monitor operations.
type Handler struct {
	svc *Service
}

// NewHandler creates a new monitor HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ServeHTTP routes requests to the appropriate handler method.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Extract user ID from context (set by auth middleware).
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// /api/v1/monitors/{id}
	if strings.HasPrefix(path, "/api/v1/monitors/") {
		idStr := strings.TrimPrefix(path, "/api/v1/monitors/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid monitor id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.get(w, r, id, userID)
		case http.MethodPatch:
			h.update(w, r, id, userID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /api/v1/monitors
	switch r.Method {
	case http.MethodGet:
		h.list(w, r, userID)
	case http.MethodPost:
		h.create(w, r, userID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type createRequest struct {
	Name                string `json:"name"`
	QueryText           string `json:"query_text"`
	Language            string `json:"language"`
	Region              string `json:"region"`
	PollIntervalMinutes int    `json:"poll_interval_minutes"`
	AlertEnabled        bool   `json:"alert_enabled"`
}

type updateRequest struct {
	Name                *string `json:"name"`
	QueryText           *string `json:"query_text"`
	Language            *string `json:"language"`
	Region              *string `json:"region"`
	PollIntervalMinutes *int    `json:"poll_interval_minutes"`
	AlertEnabled        *bool   `json:"alert_enabled"`
	Status              *string `json:"status"`
}

type monitorResponse struct {
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

func monitorToResponse(m Monitor) monitorResponse {
	return monitorResponse{
		ID:                  m.ID,
		UserID:              m.UserID,
		Name:                m.Name,
		QueryText:           m.QueryText,
		Language:            m.Language,
		Region:              m.Region,
		Status:              m.Status,
		PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled:        m.AlertEnabled,
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, userID int64) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	m, err := h.svc.Create(r.Context(), userID, CreateMonitorInput{
		Name:                req.Name,
		QueryText:           req.QueryText,
		Language:            req.Language,
		Region:              req.Region,
		PollIntervalMinutes: req.PollIntervalMinutes,
		AlertEnabled:        req.AlertEnabled,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInterval), errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, monitorToResponse(m))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, userID int64) {
	monitors, err := h.svc.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]monitorResponse, len(monitors))
	for i, m := range monitors {
		resp[i] = monitorToResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, id, userID int64) {
	m, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "monitor not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if m.UserID != userID {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}
	writeJSON(w, http.StatusOK, monitorToResponse(m))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, id, userID int64) {
	// Verify ownership
	m, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "monitor not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if m.UserID != userID {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.svc.Update(r.Context(), id, UpdateMonitorInput{
		Name:                req.Name,
		QueryText:           req.QueryText,
		Language:            req.Language,
		Region:              req.Region,
		PollIntervalMinutes: req.PollIntervalMinutes,
		AlertEnabled:        req.AlertEnabled,
		Status:              req.Status,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInterval):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, monitorToResponse(updated))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
