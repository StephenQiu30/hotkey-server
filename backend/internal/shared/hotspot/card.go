// Package hotspot contains the small, framework-free product projection used
// by both transient search and the persisted radar.
package hotspot

import (
	"errors"
	"math"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

const (
	QualityUnavailable = "unavailable"

	ImportanceLow    = "low"
	ImportanceMedium = "medium"

	SourceSuccess     SourceState = "success"
	SourceEmpty       SourceState = "empty"
	SourcePartial     SourceState = "partial"
	SourceFailed      SourceState = "failed"
	SourceUnavailable SourceState = "unavailable"
)

type Metrics struct {
	ViewCount    *int64
	LikeCount    *int64
	CommentCount *int64
	ShareCount   *int64
}

type Card struct {
	ID               *int64
	SourceType       string
	SourceName       string
	ExternalID       string
	ContentType      string
	Title            string
	Summary          string
	CanonicalURL     string
	Author           string
	Language         string
	PublishedAt      *time.Time
	DiscoveredAt     time.Time
	Metrics          Metrics
	HeatScore        float64
	QualityState     string
	Relevance        int
	RelevanceReason  string
	KeywordMentioned bool
	Importance       string
}

type SourceState string

type SourceStatus struct {
	SourceType  string
	SourceName  string
	State       SourceState
	ResultCount int
	ErrorCode   string
}

func HeatScore(metrics Metrics) float64 {
	metric := func(value *int64) float64 {
		if value == nil {
			return 0
		}
		return math.Log1p(float64(*value))
	}
	score := metric(metrics.ViewCount)*2 + metric(metrics.LikeCount)*10 + metric(metrics.CommentCount)*4 + metric(metrics.ShareCount)*6
	return math.Round(math.Min(score, 100)*10) / 10
}

func Importance(heat float64) string {
	if heat >= 25 {
		return ImportanceMedium
	}
	return ImportanceLow
}

// NormalizeURL canonicalizes a credential-free HTTP(S) URL and removes only
// explicit tracking parameters. Product identifiers and search parameters are
// retained.
func NormalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(norm.NFC.String(strings.TrimSpace(raw)))
	if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", errors.New("invalid canonical URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" {
		return "", errors.New("invalid canonical URL")
	}
	if net.ParseIP(hostname) == nil {
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil || hostname == "" {
			return "", errors.New("invalid canonical URL")
		}
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port == "" {
		if strings.Contains(hostname, ":") {
			parsed.Host = "[" + hostname + "]"
		} else {
			parsed.Host = hostname
		}
	} else {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	parsed.Scheme = scheme
	parsed.User = nil
	parsed.Fragment = ""
	parsed.ForceQuery = false
	if parsed.Path == "" {
		parsed.Path = "/"
	} else if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		if parsed.Path == "" {
			parsed.Path = "/"
		}
	}
	parsed.RawPath = ""
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", errors.New("invalid canonical URL")
	}
	for key := range query {
		if trackingQueryKey(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func Dedupe(cards []Card) []Card {
	seenIdentity := map[string]struct{}{}
	seenURL := map[string]struct{}{}
	result := make([]Card, 0, len(cards))
	for _, card := range cards {
		identity := strings.TrimSpace(card.SourceType) + "\x00" + strings.TrimSpace(card.ExternalID)
		if card.ExternalID != "" {
			if _, exists := seenIdentity[identity]; exists {
				continue
			}
		}
		if card.CanonicalURL != "" {
			if _, exists := seenURL[card.CanonicalURL]; exists {
				continue
			}
			seenURL[card.CanonicalURL] = struct{}{}
		}
		if card.ExternalID != "" {
			seenIdentity[identity] = struct{}{}
		}
		result = append(result, card)
	}
	return result
}

func SortByHeat(cards []Card) {
	sort.SliceStable(cards, func(left, right int) bool {
		if cards[left].HeatScore != cards[right].HeatScore {
			return cards[left].HeatScore > cards[right].HeatScore
		}
		return cards[left].DiscoveredAt.After(cards[right].DiscoveredAt)
	})
}

func trackingQueryKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if strings.HasPrefix(key, "utm_") {
		return true
	}
	switch key {
	case "fbclid", "gclid", "dclid", "msclkid", "mc_cid", "mc_eid", "igshid", "yclid", "vero_conv", "vero_id":
		return true
	default:
		return false
	}
}
