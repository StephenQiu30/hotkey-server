package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	"github.com/gin-gonic/gin"
)

type FeedReader interface {
	ReadFeed(ctx context.Context, tokenHash string) (application.Feed, error)
}

type Handler struct{ reader FeedReader }

func NewHandler(reader FeedReader) *Handler { return &Handler{reader: reader} }

// Feed serves a private RSS 2.0 or Atom feed. The URL token is hashed before
// it crosses the application boundary; it is never returned or logged.
func (handler *Handler) Feed(c *gin.Context) {
	if handler == nil || handler.reader == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	feed, err := handler.reader.ReadFeed(c.Request.Context(), domain.TokenHash(token))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	format := strings.ToLower(c.DefaultQuery("format", "rss"))
	var body []byte
	var etag string
	contentType := "application/rss+xml; charset=utf-8"
	if format == "atom" {
		body, etag, err = application.RenderAtom(feed)
		contentType = "application/atom+xml; charset=utf-8"
	} else if format == "rss" {
		body, etag, err = application.RenderRSS(feed)
	} else {
		c.Status(http.StatusBadRequest)
		return
	}
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	lastModified := feed.UpdatedAt.UTC().Truncate(time.Second)
	c.Header("ETag", `"`+etag+`"`)
	c.Header("Last-Modified", lastModified.Format(http.TimeFormat))
	if strings.Trim(c.GetHeader("If-None-Match"), `"`) == etag || ifModifiedSince(c.GetHeader("If-Modified-Since"), lastModified) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, contentType, body)
}

func ifModifiedSince(raw string, updatedAt time.Time) bool {
	if raw == "" {
		return false
	}
	parsed, err := http.ParseTime(raw)
	return err == nil && !updatedAt.After(parsed)
}
