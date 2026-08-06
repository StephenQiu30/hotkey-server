package http

import (
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
)

type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func OK[T any](c *gin.Context, data T) {
	c.JSON(stdhttp.StatusOK, Result[T]{Code: 0, Message: "success", Data: data})
}

func Created[T any](c *gin.Context, data T) {
	c.JSON(stdhttp.StatusCreated, Result[T]{Code: 0, Message: "success", Data: data})
}

func Empty(c *gin.Context) {
	c.JSON(stdhttp.StatusOK, Result[any]{Code: 0, Message: "success", Data: nil})
}

func PageOK[T any](c *gin.Context, page Page[T]) {
	OK(c, page)
}

func Fail(c *gin.Context, status int, code int, message string) {
	c.AbortWithStatusJSON(status, Result[any]{Code: code, Message: message, Data: nil})
}
