// Package http adapts the ingestion active Content query application use cases
// to the public HTTP contract. Its DTOs are explicit allowlists: raw evidence
// text, object keys, object-store configuration and credentials are absent.
package http

import (
	"math"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/hotspot"
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
	ID                int64                  `json:"id" example:"7"`
	SourceType        string                 `json:"source_type" example:"rss"`
	SourceName        string                 `json:"source_name" example:"Product feed"`
	ExternalID        string                 `json:"external_id" example:"item-123"`
	ContentType       string                 `json:"content_type" example:"article"`
	Title             string                 `json:"title" example:"Release notes"`
	CanonicalURL      string                 `json:"canonical_url" example:"https://example.test/items/123"`
	Language          string                 `json:"language" example:"en"`
	PublishedAt       *time.Time             `json:"published_at" extensions:"x-nullable"`
	FetchedAt         time.Time              `json:"fetched_at"`
	Metrics           ContentMetricsResponse `json:"metrics"`
	DedupeStatus      string                 `json:"dedupe_status" enums:"active,duplicate"`
	DedupeReason      *string                `json:"dedupe_reason" extensions:"x-nullable"`
	DedupeVersion     *string                `json:"dedupe_version" extensions:"x-nullable"`
	RelevanceScore    *float64               `json:"relevance_score,omitempty" extensions:"x-nullable"`
	MatchDecision     *string                `json:"match_decision,omitempty" extensions:"x-nullable" enums:"accepted,review,rejected"`
	DocumentVersionID *int64                 `json:"document_version_id,omitempty" extensions:"x-nullable"`
}

type ContentPageResponse struct {
	Items      []ContentResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

// HotspotPageResponse exposes the same card shape as instant search while
// retaining the existing Content cursor and persistence model underneath.
type HotspotPageResponse struct {
	Items      []hotspot.HotspotCardResponse `json:"items"`
	NextCursor string                        `json:"next_cursor"`
	Summary    HotspotSummaryResponse        `json:"summary"`
}

type HotspotSummaryResponse struct {
	Total  int64 `json:"total"`
	Today  int64 `json:"today"`
	Urgent int64 `json:"urgent"`
}

type ContentDocumentResponse struct {
	ContentID         int64      `json:"content_id" example:"7"`
	Title             string     `json:"title" example:"Release notes"`
	SourceName        string     `json:"source_name" example:"Product feed"`
	CanonicalURL      string     `json:"canonical_url" example:"https://example.test/items/123"`
	Language          string     `json:"language" example:"en"`
	PublishedAt       *time.Time `json:"published_at" extensions:"x-nullable"`
	Availability      string     `json:"availability" enums:"ready,not_captured,unavailable"`
	UnavailableReason *string    `json:"unavailable_reason,omitempty" enums:"pending,missing,deleting,read_failed,integrity_failed" extensions:"x-nullable"`
	Markdown          string     `json:"markdown" example:"# Release notes"`
	SHA256            string     `json:"sha256" example:"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	CapturedAt        *time.Time `json:"captured_at" extensions:"x-nullable"`
}

func contentResponse(content ingestiondomain.Content) ContentResponse {
	response := ContentResponse{
		ID: content.ID, SourceType: string(content.SourceType), SourceName: content.SourceName,
		ExternalID: content.ExternalID, ContentType: content.ContentType, Title: content.Title,
		CanonicalURL: content.CanonicalURL, Language: content.Language,
		PublishedAt: optionalContentTime(content.PublishedAt), FetchedAt: content.FetchedAt,
		Metrics: ContentMetricsResponse{
			ViewCount: content.Metrics.ViewCount, LikeCount: content.Metrics.LikeCount,
			CommentCount: content.Metrics.CommentCount, ShareCount: content.Metrics.ShareCount,
		},
		DedupeStatus: string(content.Status), DedupeReason: nullableContentField(content.DedupeReason), DedupeVersion: nullableContentField(content.DedupeVersion),
		RelevanceScore:    content.RelevanceScore,
		DocumentVersionID: content.DocumentVersionID,
	}
	if content.MatchDecision != nil {
		value := string(*content.MatchDecision)
		response.MatchDecision = &value
	}
	return response
}

func contentPageResponse(page ingestiondomain.ContentPage) ContentPageResponse {
	items := make([]ContentResponse, 0, len(page.Items))
	for _, content := range page.Items {
		items = append(items, contentResponse(content))
	}
	return ContentPageResponse{Items: items, NextCursor: page.NextCursor}
}

func hotspotPageResponse(page ingestiondomain.ContentPage) HotspotPageResponse {
	items := make([]hotspot.HotspotCardResponse, 0, len(page.Items))
	for _, content := range page.Items {
		items = append(items, hotspot.Response(hotspotCard(content)))
	}
	response := HotspotPageResponse{Items: items, NextCursor: page.NextCursor}
	if page.Summary != nil {
		response.Summary = HotspotSummaryResponse{Total: page.Summary.Total, Today: page.Summary.Today, Urgent: page.Summary.Urgent}
	}
	return response
}

func hotspotCard(content ingestiondomain.Content) hotspot.Card {
	id := content.ID
	metrics := hotspot.Metrics{
		ViewCount: content.Metrics.ViewCount, LikeCount: content.Metrics.LikeCount,
		CommentCount: content.Metrics.CommentCount, ShareCount: content.Metrics.ShareCount,
	}
	heat := hotspot.HeatScore(metrics)
	relevance := 0
	if content.RelevanceScore != nil {
		relevance = int(math.Round(*content.RelevanceScore))
		if relevance < 0 {
			relevance = 0
		} else if relevance > 100 {
			relevance = 100
		}
	}
	reason := "AI 尚未分析"
	if content.RelevanceScore != nil {
		reason = "当前监控的最近一次相关性判断"
	}
	return hotspot.Card{
		ID: &id, SourceType: string(content.SourceType), SourceName: content.SourceName,
		ExternalID: content.ExternalID, ContentType: content.ContentType, Title: content.Title,
		Summary: content.Excerpt, CanonicalURL: content.CanonicalURL,
		Author: content.Author.DisplayName, Language: content.Language,
		PublishedAt: optionalContentTime(content.PublishedAt), DiscoveredAt: content.FetchedAt, Metrics: metrics,
		HeatScore: heat, QualityState: hotspot.QualityUnavailable,
		Relevance: relevance, RelevanceReason: reason,
		Importance: hotspot.Importance(heat),
	}
}

func contentDocumentResponse(document ingestiondomain.ContentDocument) ContentDocumentResponse {
	response := ContentDocumentResponse{
		ContentID: document.ContentID, Title: document.Title, SourceName: document.SourceName,
		CanonicalURL: document.CanonicalURL, Language: document.Language, PublishedAt: optionalContentTime(document.PublishedAt),
		Availability: string(document.Availability), Markdown: document.Markdown, SHA256: document.SHA256,
	}
	if document.UnavailableReason != "" {
		reason := string(document.UnavailableReason)
		response.UnavailableReason = &reason
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

func optionalContentTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
