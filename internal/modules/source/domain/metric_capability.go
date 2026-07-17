package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

type MetricCapabilityStatus string
type IndependenceStrategy string

const (
	MetricCapabilityDraft     MetricCapabilityStatus = "draft"
	MetricCapabilityPublished MetricCapabilityStatus = "published"
	MetricCapabilityArchived  MetricCapabilityStatus = "archived"

	IndependenceBySourceConnection IndependenceStrategy = "source_connection"
	IndependenceByAuthor           IndependenceStrategy = "author"
)

var metricCapabilityVersionPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// MetricCapabilityProfile is the immutable scoring capability configuration
// for one source type and profile version. Published configuration fields are
// never changed; replacing a profile creates and publishes a new draft.
type MetricCapabilityProfile struct {
	ID, Version               int64
	SourceType                SourceType
	ProfileVersion            string
	SupportsViews             bool
	SupportsLikes             bool
	SupportsComments          bool
	SupportsShares            bool
	IndependenceStrategy      IndependenceStrategy
	NormalizationWindowHours  int
	CredibilityWeight         float64
	MaxSingleItemContribution float64
	Status                    MetricCapabilityStatus
	PublishedAt, ArchivedAt   *time.Time
}

func (profile MetricCapabilityProfile) ValidateDraft() error {
	if profile.Status != "" && profile.Status != MetricCapabilityDraft {
		return fmt.Errorf("new metric capability profile must be draft")
	}
	return profile.validateFields()
}

func (profile MetricCapabilityProfile) Validate() error {
	if err := profile.validateFields(); err != nil {
		return err
	}
	if profile.Status != MetricCapabilityDraft && profile.Status != MetricCapabilityPublished && profile.Status != MetricCapabilityArchived {
		return fmt.Errorf("invalid metric capability status")
	}
	if profile.Status == MetricCapabilityPublished && profile.PublishedAt == nil {
		return fmt.Errorf("published metric capability profile requires published_at")
	}
	if profile.Status == MetricCapabilityArchived && profile.ArchivedAt == nil {
		return fmt.Errorf("archived metric capability profile requires archived_at")
	}
	return nil
}

func (profile MetricCapabilityProfile) validateFields() error {
	if !profile.SourceType.Valid() || !metricCapabilityVersionPattern.MatchString(strings.TrimSpace(profile.ProfileVersion)) {
		return fmt.Errorf("invalid metric capability profile identity")
	}
	if !profile.SupportsViews && !profile.SupportsLikes && !profile.SupportsComments && !profile.SupportsShares {
		return fmt.Errorf("metric capability profile must support a metric")
	}
	if profile.IndependenceStrategy != IndependenceBySourceConnection && profile.IndependenceStrategy != IndependenceByAuthor {
		return fmt.Errorf("invalid metric capability independence strategy")
	}
	if profile.NormalizationWindowHours < 1 || profile.NormalizationWindowHours > 24*30 {
		return fmt.Errorf("normalization window must be 1-720 hours")
	}
	if invalidCapabilityScore(profile.CredibilityWeight, 0, 1) || invalidCapabilityScore(profile.MaxSingleItemContribution, 0.01, 100) {
		return fmt.Errorf("invalid metric capability weights")
	}
	return nil
}

func (status MetricCapabilityStatus) Valid() bool {
	return status == MetricCapabilityDraft || status == MetricCapabilityPublished || status == MetricCapabilityArchived
}

func invalidCapabilityScore(value, minimum, maximum float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0) || value < minimum || value > maximum
}
