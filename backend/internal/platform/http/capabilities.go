package http

import "github.com/gin-gonic/gin"

type Capabilities struct {
	APIVersion string `json:"api_version"`
}

// CapabilitiesHandler reports the version of the public HTTP contract.
// @Summary Get API capabilities
// @Tags platform
// @Produce json
// @Success 200 {object} Result[Capabilities]
// @Router /api/v1/capabilities [get]
func CapabilitiesHandler(c *gin.Context) error {
	OK(c, Capabilities{APIVersion: "v1"})
	return nil
}
