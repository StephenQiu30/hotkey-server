package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/server"
)

// ContextWithUserID returns a new context with the given user ID.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, server.UserIDKey, userID)
}

// userIDFromContext extracts the user ID from the context.
func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(server.UserIDKey).(int64)
	return id, ok
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// Handler provides HTTP endpoints for notifications.
type Handler struct {
	svc *Service
}

// NewHandler creates a new notification Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ServeHTTP routes requests to the appropriate handler method.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// /api/v1/notifications/{id}/read
	if r.URL.Path != "/api/v1/notifications" {
		h.markRead(w, r, userID)
		return
	}

	// /api/v1/notifications
	switch r.Method {
	case http.MethodGet:
		h.listUnread(w, r, userID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// notificationJSON is the JSON representation of a notification.
type notificationJSON struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	AlertID        int64   `json:"alert_id"`
	Channel        string  `json:"channel"`
	DeliveryStatus string  `json:"delivery_status"`
	ReadAt         *string `json:"read_at"`
	CreatedAt      string  `json:"created_at"`
}

func toNotificationJSON(n Notification) notificationJSON {
	j := notificationJSON{
		ID:             n.ID,
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
		CreatedAt:      n.CreatedAt.Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		j.ReadAt = &s
	}
	return j
}

func (h *Handler) listUnread(w http.ResponseWriter, r *http.Request, userID int64) {
	items, err := h.svc.ListUnread(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]notificationJSON, len(items))
	for i, n := range items {
		result[i] = toNotificationJSON(n)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request, userID int64) {
	// Extract notification ID from path: /api/v1/notifications/{id}/read
	path := r.URL.Path
	prefix := "/api/v1/notifications/"
	suffix := "/read"
	if len(path) <= len(prefix)+len(suffix) || path[:len(prefix)] != prefix || path[len(path)-len(suffix):] != suffix {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	idStr := path[len(prefix) : len(path)-len(suffix)]
	notifID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	if err := h.svc.MarkRead(r.Context(), userID, notifID); err != nil {
		if err == ErrNotFound || err == ErrNotOwned {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
