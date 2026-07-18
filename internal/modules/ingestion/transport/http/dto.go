// Package http adapts the ingestion active Content query application use cases
// to the public HTTP contract. Its DTOs are explicit allowlists: raw evidence
// text, object keys, object-store configuration and credentials are absent.
package http

import (
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
)

// ContentResult mirrors the shared Result envelope for Swagger only. Runtime
// responses always use internal/platform/http helpers.
type ContentResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

// ContentMetricsResponse preserves the difference between an unknown metric
// (null) and an explicit source-reported zero (0).
type ContentMetricsResponse struct {
	ViewCount    *int64 `json:"view_count" extensions:"x-nullable"`
	LikeCount    *int64 `json:"like_count" extensions:"x-nullable"`
	CommentCount *int64 `json:"comment_count" extensions:"x-nullable"`
	ShareCount   *int64 `json:"share_count" extensions:"x-nullable"`
}

// ContentResponse is deliberately the complete public allowlist. In
// particular, excerpt/body, author profile values, asset object keys and
// object-store details are not public Content query fields.
type ContentResponse struct {
	ID            int64                  `json:"id" example:"7"`
	SourceType    string                 `json:"source_type" example:"rss"`
	SourceName    string                 `json:"source_name" example:"Product feed"`
	ExternalID    string                 `json:"external_id" example:"item-123"`
	ContentType   string                 `json:"content_type" example:"article"`
	Title         string                 `json:"title" example:"Release notes"`
	CanonicalURL  string                 `json:"canonical_url" example:"https://example.test/items/123"`
	Language      string                 `json:"language" example:"en"`
	PublishedAt   time.Time              `json:"published_at"`
	FetchedAt     time.Time              `json:"fetched_at"`
	Metrics       ContentMetricsResponse `json:"metrics"`
	DedupeStatus  string                 `json:"dedupe_status" enums:"active,duplicate"`
	DedupeReason  *string                `json:"dedupe_reason" extensions:"x-nullable"`
	DedupeVersion *string                `json:"dedupe_version" extensions:"x-nullable"`
}

type ContentPageResponse struct {
	Items      []ContentResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type ContentDocumentResponse struct {
	ContentID    int64      `json:"content_id" example:"7"`
	Title        string     `json:"title" example:"Release notes"`
	SourceName   string     `json:"source_name" example:"Product feed"`
	CanonicalURL string     `json:"canonical_url" example:"https://example.test/items/123"`
	Language     string     `json:"language" example:"en"`
	PublishedAt  time.Time  `json:"published_at"`
	Availability string     `json:"availability" enums:"ready,not_captured"`
	Markdown     string     `json:"markdown" example:"# Release notes"`
	SHA256       string     `json:"sha256" example:"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	CapturedAt   *time.Time `json:"captured_at" extensions:"x-nullable"`
}

func contentResponse(content ingestiondomain.Content) ContentResponse {
	return ContentResponse{
		ID: content.ID, SourceType: string(content.SourceType), SourceName: content.SourceName,
		ExternalID: content.ExternalID, ContentType: content.ContentType, Title: content.Title,
		CanonicalURL: content.CanonicalURL, Language: content.Language,
		PublishedAt: content.PublishedAt, FetchedAt: content.FetchedAt,
		Metrics: ContentMetricsResponse{
			ViewCount: content.Metrics.ViewCount, LikeCount: content.Metrics.LikeCount,
			CommentCount: content.Metrics.CommentCount, ShareCount: content.Metrics.ShareCount,
		},
		DedupeStatus: string(content.Status), DedupeReason: nullableContentField(content.DedupeReason), DedupeVersion: nullableContentField(content.DedupeVersion),
	}
}

func contentPageResponse(page ingestiondomain.ContentPage) ContentPageResponse {
	items := make([]ContentResponse, 0, len(page.Items))
	for _, content := range page.Items {
		items = append(items, contentResponse(content))
	}
	return ContentPageResponse{Items: items, NextCursor: page.NextCursor}
}

func contentDocumentResponse(document ingestiondomain.ContentDocument) ContentDocumentResponse {
	response := ContentDocumentResponse{
		ContentID: document.ContentID, Title: document.Title, SourceName: document.SourceName,
		CanonicalURL: document.CanonicalURL, Language: document.Language, PublishedAt: document.PublishedAt,
		Availability: string(document.Availability), Markdown: document.Markdown, SHA256: document.SHA256,
	}
	if !document.CapturedAt.IsZero() {
		capturedAt := document.CapturedAt
		response.CapturedAt = &capturedAt
	}
	return response
}

func nullableContentField(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
