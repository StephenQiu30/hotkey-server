package admin

import (
	"encoding/json"
	"errors"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
	"net/http"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *serviceadmin.Service
}

type rerunDailyReportRequest struct {
	Date      string `json:"date"`
	ChannelID string `json:"channelId"`
	UserID    string `json:"userId"`
}

func New(service *serviceadmin.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if !isWriteMethod(c.Request.Method) {
			return
		}
		account, ok := authhandler.CurrentUser(c)
		if !ok {
			return
		}
		resourceType, resourceID := auditResource(c)
		if resourceType == "" {
			return
		}
		result := "success"
		if c.Writer.Status() >= http.StatusBadRequest {
			result = "failure"
		}
		_, _ = h.service.RecordAuditLog(c.Request.Context(), serviceadmin.AuditLogInput{
			ActorID:      account.ID,
			Action:       auditAction(c.Request.Method),
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Result:       result,
		})
	}
}

func (h *Handler) ListAuditLogs(c *gin.Context) {
	logs, err := h.service.ListAuditLogs(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"auditLogs": auditLogResponses(logs)})
}

func (h *Handler) ConfigStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": configStatusResponse(h.service.ConfigStatus(c.Request.Context()))})
}

func (h *Handler) QueueOverview(c *gin.Context) {
	overview, err := h.service.QueueOverview(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"queue": overview})
}

func (h *Handler) ListFailedJobs(c *gin.Context) {
	jobs, err := h.service.ListFailedJobs(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobResponses(jobs)})
}

func (h *Handler) JobDetail(c *gin.Context) {
	job, err := h.service.JobByID(c.Request.Context(), c.Param("jobID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": jobResponse(job)})
}

func (h *Handler) RetryJob(c *gin.Context) {
	job, err := h.service.RetryJob(c.Request.Context(), c.Param("jobID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"job": jobResponse(job)})
}

func (h *Handler) RerunDailyReport(c *gin.Context) {
	var req rerunDailyReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	job, err := h.service.RerunDailyReport(c.Request.Context(), serviceadmin.RerunDailyReportInput{
		Date:      req.Date,
		ChannelID: req.ChannelID,
		UserID:    req.UserID,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.Set("adminAuditResourceID", job.ID)
	c.JSON(http.StatusAccepted, gin.H{"job": jobResponse(job)})
}

func (h *Handler) RevokeSource(c *gin.Context) {
	sourceID := c.Param("sourceID")
	c.Set("adminAuditResourceID", sourceID)
	result, err := h.service.RevokeSource(c.Request.Context(), sourceID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source": gin.H{
			"id":        result.ID,
			"name":      result.Name,
			"status":    result.Status,
			"updatedAt": result.UpdatedAt,
		},
	})
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	userID := c.Param("userID")
	c.Set("adminAuditResourceID", userID)
	task, err := h.service.DeleteAccount(c.Request.Context(), userID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleanupTask": cleanupTaskResponse(task)})
}

func (h *Handler) CleanupStatus(c *gin.Context) {
	taskID := c.Param("taskID")
	task, err := h.service.CleanupStatus(c.Request.Context(), taskID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleanupTask": cleanupTaskResponse(task)})
}

func (h *Handler) RetryCleanup(c *gin.Context) {
	taskID := c.Param("taskID")
	c.Set("adminAuditResourceID", taskID)
	task, err := h.service.RetryCleanup(c.Request.Context(), taskID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleanupTask": cleanupTaskResponse(task)})
}

func auditLogResponses(logs []serviceadmin.AuditLog) []gin.H {
	responses := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		responses = append(responses, gin.H{
			"id":           log.ID,
			"actorId":      log.ActorID,
			"action":       log.Action,
			"resourceType": log.ResourceType,
			"resourceId":   log.ResourceID,
			"result":       log.Result,
			"createdAt":    log.CreatedAt,
		})
	}
	return responses
}

func configStatusResponse(status serviceadmin.ConfigStatus) gin.H {
	components := gin.H{}
	for name, component := range status.Components {
		components[name] = gin.H{"status": component.Status, "reason": component.Reason}
	}
	return gin.H{"overall": status.Overall, "components": components}
}

func jobResponses(jobs []queue.Job) []gin.H {
	responses := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, jobResponse(job))
	}
	return responses
}

func jobResponse(job queue.Job) gin.H {
	return gin.H{
		"id":             job.ID,
		"type":           job.Type,
		"payload":        json.RawMessage(job.Payload),
		"status":         job.Status,
		"attempt":        job.Attempt,
		"maxAttempts":    job.MaxAttempts,
		"idempotencyKey": job.IdempotencyKey,
		"lastError":      job.LastError,
		"nextRunAt":      job.NextRunAt,
		"createdAt":      job.CreatedAt,
		"updatedAt":      job.UpdatedAt,
	}
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, serviceadmin.ErrInvalidInput):
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, serviceadmin.ErrNotFound):
		httputil.WriteError(c, http.StatusNotFound, "not_found", "not found")
	default:
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut || method == http.MethodDelete
}

func auditAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPatch, http.MethodPut:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func auditResource(c *gin.Context) (string, string) {
	path := c.FullPath()
	contextResourceID := c.GetString("adminAuditResourceID")
	switch {
	case strings.Contains(path, "/sources"):
		return "source", firstNonEmpty(contextResourceID, c.Param("sourceID"))
	case strings.Contains(path, "/channels"):
		return "channel", firstNonEmpty(contextResourceID, c.Param("channelID"))
	case strings.Contains(path, "/settings"):
		return "config", "default_daily_send_at"
	case strings.Contains(path, "/daily-reports"):
		return "daily_report", contextResourceID
	case strings.Contains(path, "/jobs"):
		return "job", c.Param("jobID")
	case strings.Contains(path, "/users"):
		return "user", firstNonEmpty(contextResourceID, c.Param("userID"))
	case strings.Contains(path, "/cleanup-tasks"):
		return "cleanup_task", c.Param("taskID")
	default:
		return "", ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cleanupTaskResponse(task serviceadmin.CleanupTask) gin.H {
	steps := make([]gin.H, 0, len(task.Steps))
	for _, step := range task.Steps {
		s := gin.H{"name": step.Name, "status": step.Status}
		if step.Error != "" {
			s["error"] = step.Error
		}
		steps = append(steps, s)
	}
	return gin.H{
		"id":        task.ID,
		"userId":    task.UserID,
		"status":    task.Status,
		"steps":     steps,
		"createdAt": task.CreatedAt,
		"updatedAt": task.UpdatedAt,
	}
}
