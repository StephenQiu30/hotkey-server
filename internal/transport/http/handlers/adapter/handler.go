package adapter

import (
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for adapter management.
type Handler struct {
	registry *adapter.Registry
}

// New creates a new adapter Handler.
func New(registry *adapter.Registry) *Handler {
	return &Handler{registry: registry}
}

// ListAdapters returns all registered adapters.
func (h *Handler) ListAdapters(c *gin.Context) {
	adapters := h.registry.List()
	result := make([]map[string]any, 0, len(adapters))
	for _, a := range adapters {
		health := a.Health()
		caps := a.Capabilities()
		result = append(result, map[string]any{
			"name":                  a.Name(),
			"provider":              string(a.Provider()),
			"health_status":         string(health.Status),
			"health_last_error":     health.LastError,
			"health_last_checked_at": health.LastCheckedAt,
			"supports_incremental":  caps.SupportsIncremental,
			"max_items_per_fetch":   caps.MaxItemsPerFetch,
			"rate_limit_per_hour":   caps.RateLimitPerHour,
		})
	}
	c.JSON(http.StatusOK, result)
}

// GetAdapterHealth returns the health status of a specific adapter.
func (h *Handler) GetAdapterHealth(c *gin.Context) {
	provider := adapter.Provider(c.Param("provider"))
	a, ok := h.registry.Get(provider)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "adapter_not_found", "message": "no adapter registered for provider: " + string(provider)}})
		return
	}
	health := a.Health()
	c.JSON(http.StatusOK, gin.H{
		"status":         string(health.Status),
		"last_error":     health.LastError,
		"last_checked_at": health.LastCheckedAt,
	})
}

// GetAdapterCapabilities returns the capabilities of a specific adapter.
func (h *Handler) GetAdapterCapabilities(c *gin.Context) {
	provider := adapter.Provider(c.Param("provider"))
	a, ok := h.registry.Get(provider)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "adapter_not_found", "message": "no adapter registered for provider: " + string(provider)}})
		return
	}
	caps := a.Capabilities()
	c.JSON(http.StatusOK, gin.H{
		"supports_incremental": caps.SupportsIncremental,
		"max_items_per_fetch":  caps.MaxItemsPerFetch,
		"rate_limit_per_hour":  caps.RateLimitPerHour,
	})
}
