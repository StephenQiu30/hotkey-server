package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

type HTTPHandler struct {
	svc *Service
}

func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

func (h *HTTPHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.svc.Register(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrEmailExists) {
			writeError(w, http.StatusConflict, "email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input LoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	output, err := h.svc.Login(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, output)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
