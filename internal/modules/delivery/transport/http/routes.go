package http

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, reader FeedReader) {
	if router == nil {
		return
	}
	handler := NewHandler(reader)
	router.GET("/feeds/:token", handler.Feed)
}
