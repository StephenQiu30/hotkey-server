package http

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

type HandlerFunc func(*gin.Context) error

// Wrap is the only HTTP error adapter for application handlers. Successful
// handlers write through Result helpers; failures are converted exactly once.
func Wrap(handler HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil && !c.Writer.Written() {
				WriteError(c, fmt.Errorf("handler panic: %v", recovered))
			}
		}()

		if err := handler(c); err != nil && !c.Writer.Written() {
			WriteError(c, err)
		}
	}
}
