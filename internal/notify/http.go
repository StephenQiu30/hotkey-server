package notify

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Handler provides HTTP endpoints for notifications.
type Handler struct {
	svc *Service
}

// NewHandler creates a new notification Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers notification routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notifications", h.listUnread)
	mux.HandleFunc("POST /api/v1/notifications/{id}/read", h.markRead)
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

func (h *Handler) listUnread(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	items, err := h.svc.ListUnread(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make([]notificationJSON, len(items))
	for i, n := range items {
		result[i] = toNotificationJSON(n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	notifID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid notification id", http.StatusBadRequest)
		return
	}

	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	if err := h.svc.MarkRead(r.Context(), userID, notifID); err != nil {
		if err == ErrNotFound || err == ErrNotOwned {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
