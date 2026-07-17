package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// EventFingerprintVersion identifies the deterministic facts and daily bucket
// used for bounded fingerprint recall. Changing these inputs requires a new
// version rather than comparing incompatible hashes.
const EventFingerprintVersion = "event_facts_v1"

// EventFingerprintFacts contains only normalized, bounded facts already owned
// by PLAN-010: accepted Monitor rule terms, configured regions and published
// time. It deliberately does not depend on entities, claims or an LLM.
type EventFingerprintFacts struct {
	EntityTerms []string
	ActionTerms []string
	Regions     []string
	PublishedAt time.Time
}

type EventFingerprint struct {
	Value      string
	Version    string
	TimeBucket time.Time
}

// BuildEventFingerprint returns no fingerprint when the current deterministic
// facts do not contain both an entity and an action term. In that case recall
// must rely on the other bounded channels instead of making a broad hash.
func BuildEventFingerprint(facts EventFingerprintFacts) (EventFingerprint, bool) {
	entities := normalizedFingerprintTerms(facts.EntityTerms)
	actions := normalizedFingerprintTerms(facts.ActionTerms)
	if len(entities) == 0 || len(actions) == 0 || facts.PublishedAt.IsZero() {
		return EventFingerprint{}, false
	}
	bucket := facts.PublishedAt.UTC().Truncate(24 * time.Hour)
	payload := strings.Join([]string{
		EventFingerprintVersion,
		"entities=" + strings.Join(entities, "\x1f"),
		"actions=" + strings.Join(actions, "\x1f"),
		"regions=" + strings.Join(normalizedFingerprintTerms(facts.Regions), "\x1f"),
		"time_bucket=" + bucket.Format(time.DateOnly),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return EventFingerprint{Value: hex.EncodeToString(sum[:]), Version: EventFingerprintVersion, TimeBucket: bucket}, true
}

func normalizedFingerprintTerms(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
		if normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
