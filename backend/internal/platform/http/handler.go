package http

import "github.com/gin-gonic/gin"

type HandlerFunc func(*gin.Context) error

// Wrap is the only HTTP error adapter for application handlers. Successful
// handlers write through Result helpers; failures are converted exactly once.
func Wrap(handler HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := handler(c); err != nil && !c.Writer.Written() {
			WriteError(c, err)
		}
	}
}
