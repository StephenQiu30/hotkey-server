package content

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Normalize converts a raw post from a platform into a normalized post
// with a stable content hash for deduplication.
func Normalize(raw RawPost, platform string) NormalizedPost {
	text := strings.TrimSpace(raw.Text)
	authorHandle := strings.TrimSpace(raw.AuthorHandle)

	hash := computeContentHash(platform, raw.AuthorID, text)

	postURL := ""
	if platform == "x" && authorHandle != "" && raw.ID != "" {
		postURL = fmt.Sprintf("https://x.com/%s/status/%s", authorHandle, raw.ID)
	}

	return NormalizedPost{
		Platform:         platform,
		PlatformPostID:   raw.ID,
		AuthorPlatformID: raw.AuthorID,
		AuthorName:       raw.AuthorName,
		AuthorHandle:     authorHandle,
		ContentText:      text,
		ContentLang:      raw.Language,
		PostURL:          postURL,
		PublishedAt:      raw.PublishedAt,
		LikeCount:        raw.LikeCount,
		ReplyCount:       raw.ReplyCount,
		RepostCount:      raw.RepostCount,
		QuoteCount:       raw.QuoteCount,
		ViewCount:        raw.ViewCount,
		NormalizedHash:   hash,
	}
}

// computeContentHash generates a stable SHA-256 hash from platform, author, and content.
// Uses null-byte separators to prevent collisions when fields contain colons.
func computeContentHash(platform, authorID, text string) string {
	h := sha256.Sum256([]byte(platform + "\x00" + authorID + "\x00" + text))
	return fmt.Sprintf("%x", h)
}
