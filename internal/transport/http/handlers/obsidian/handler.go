package obsidian

import (
	"errors"
	"net/http"

	domain "github.com/StephenQiu30/hotkey-server/internal/domain/obsidian"
	svc "github.com/StephenQiu30/hotkey-server/internal/service/obsidian"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *svc.Service
}

func New(service *svc.Service) *Handler {
	return &Handler{service: service}
}

// Connect handles POST /me/obsidian/connect
func (h *Handler) Connect(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req connectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	result, err := h.service.Connect(c.Request.Context(), svc.ConnectInput{
		UserID:             account.ID,
		RepoURL:            req.RepoURL,
		Branch:             req.Branch,
		BaseDir:            req.BaseDir,
		AccessToken:        req.AccessToken,
		EventNoteTemplate:  req.EventNoteTemplate,
		DailyReportIndex:   req.DailyReportIndex,
		WeeklyReportIndex:  req.WeeklyReportIndex,
		ConflictResolution: domain.ConflictResolution(req.ConflictResolution),
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, connectResponse{
		ConfigID:      result.ConfigID,
		Status:        string(result.Status),
		DefaultBranch: result.DefaultBranch,
		Branches:      result.Branches,
	})
}

// Disconnect handles DELETE /me/obsidian
func (h *Handler) Disconnect(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.service.Disconnect(c.Request.Context(), account.ID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetStatus handles GET /me/obsidian
func (h *Handler) GetStatus(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	result, err := h.service.GetStatus(c.Request.Context(), account.ID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	var lastSync *string
	if result.LastSync != nil {
		s := result.LastSync.Format("2006-01-02T15:04:05Z")
		lastSync = &s
	}

	c.JSON(http.StatusOK, statusResponse{
		ConfigID:  result.ConfigID,
		RepoURL:   result.RepoURL,
		Branch:    result.Branch,
		BaseDir:   result.BaseDir,
		Status:    string(result.Status),
		LastError: result.LastError,
		LastSync:  lastSync,
	})
}

// Sync handles POST /me/obsidian/sync
func (h *Handler) Sync(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req syncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	result, err := h.service.Sync(c.Request.Context(), svc.SyncInput{
		UserID:      account.ID,
		ContentType: req.ContentType,
		ContentID:   req.ContentID,
		Title:       req.Title,
		Body:        req.Body,
		Tags:        req.Tags,
		Frontmatter: req.Frontmatter,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, syncResponse{
		RecordID:      result.RecordID,
		FilePath:      result.FilePath,
		CommitSHA:     result.CommitSHA,
		CommitURL:     result.CommitURL,
		State:         string(result.State),
		WasIdempotent: result.WasIdempotent,
	})
}

// --- request/response types ---

type connectRequest struct {
	RepoURL            string `json:"repoUrl" binding:"required"`
	Branch             string `json:"branch"`
	BaseDir            string `json:"baseDir"`
	AccessToken        string `json:"accessToken" binding:"required"`
	EventNoteTemplate  string `json:"eventNoteTemplate"`
	DailyReportIndex   string `json:"dailyReportIndex"`
	WeeklyReportIndex  string `json:"weeklyReportIndex"`
	ConflictResolution string `json:"conflictResolution"`
}

type connectResponse struct {
	ConfigID      string   `json:"configId"`
	Status        string   `json:"status"`
	DefaultBranch string   `json:"defaultBranch"`
	Branches      []string `json:"branches"`
}

type statusResponse struct {
	ConfigID  string     `json:"configId"`
	RepoURL   string     `json:"repoUrl"`
	Branch    string     `json:"branch"`
	BaseDir   string     `json:"baseDir"`
	Status    string     `json:"status"`
	LastError string     `json:"lastError,omitempty"`
	LastSync  *string    `json:"lastSync,omitempty"` // simplified for JSON
}

type syncRequest struct {
	ContentType string            `json:"contentType" binding:"required"`
	ContentID   string            `json:"contentId" binding:"required"`
	Title       string            `json:"title" binding:"required"`
	Body        string            `json:"body"`
	Tags        []string          `json:"tags"`
	Frontmatter map[string]string `json:"frontmatter"`
}

type syncResponse struct {
	RecordID      string `json:"recordId"`
	FilePath      string `json:"filePath"`
	CommitSHA     string `json:"commitSha"`
	CommitURL     string `json:"commitUrl"`
	State         string `json:"state"`
	WasIdempotent bool   `json:"wasIdempotent"`
}

// --- helpers ---

func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, svc.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, svc.ErrNotConnected):
		writeError(c, http.StatusNotFound, "not_connected", "obsidian sync not connected")
	case errors.Is(err, svc.ErrAuthFailed):
		writeError(c, http.StatusForbidden, "auth_failed", "git authentication failed")
	case errors.Is(err, svc.ErrBranchNotFound):
		writeError(c, http.StatusBadRequest, "branch_not_found", "branch not found in repository")
	case errors.Is(err, svc.ErrDirNotFound):
		writeError(c, http.StatusBadRequest, "dir_not_found", "directory not found in repository")
	case errors.Is(err, svc.ErrConflict):
		writeError(c, http.StatusConflict, "conflict", "file conflict detected")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
