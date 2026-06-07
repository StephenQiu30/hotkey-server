package monitortopic

import (
	"errors"
	"time"
)

// TopicStatus represents the lifecycle state of a monitor topic.
type TopicStatus string

const (
	TopicStatusDraft    TopicStatus = "draft"
	TopicStatusActive   TopicStatus = "active"
	TopicStatusPaused   TopicStatus = "paused"
	TopicStatusArchived TopicStatus = "archived"
)

// Language represents the monitoring language scope.
type Language string

const (
	LanguageZH     Language = "zh"
	LanguageEN     Language = "en"
	LanguageJA     Language = "ja"
	LanguageMulti  Language = "multi"
)

// Platform represents a monitoring source platform.
type Platform string

const (
	PlatformWeibo      Platform = "weibo"
	PlatformTwitter    Platform = "twitter"
	PlatformWechat     Platform = "wechat"
	PlatformReddit     Platform = "reddit"
	PlatformHackerNews Platform = "hackernews"
	PlatformRSS        Platform = "rss"
)

// KeywordType distinguishes inclusion from exclusion keywords.
type KeywordType string

const (
	KeywordTypeInclude KeywordType = "include"
	KeywordTypeExclude KeywordType = "exclude"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInvalidTransition = errors.New("invalid status transition")
)

// MonitorTopic is the aggregate root for topic-level monitoring configuration.
type MonitorTopic struct {
	ID                  string
	UserID              string
	Name                string
	Description         string
	Status              TopicStatus
	Language            Language
	Platforms           []Platform
	SimilarityThreshold float64
	CollectIntervalMin  int
	DailyReportEnabled  bool
	ObsidianOutputDir   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// TopicKeyword is a keyword or exclusion word bound to a monitor topic.
type TopicKeyword struct {
	ID        string
	TopicID   string
	Word      string
	Type      KeywordType
	CreatedAt time.Time
}

// validLanguages is the set of allowed language values.
var validLanguages = map[Language]bool{
	LanguageZH:    true,
	LanguageEN:    true,
	LanguageJA:    true,
	LanguageMulti: true,
}

// validPlatforms is the set of allowed platform values.
var validPlatforms = map[Platform]bool{
	PlatformWeibo:      true,
	PlatformTwitter:    true,
	PlatformWechat:     true,
	PlatformReddit:     true,
	PlatformHackerNews: true,
	PlatformRSS:        true,
}

// validTransitions defines the allowed status transitions.
var validTransitions = map[TopicStatus]map[TopicStatus]bool{
	TopicStatusDraft:    {TopicStatusActive: true, TopicStatusArchived: true},
	TopicStatusActive:   {TopicStatusPaused: true, TopicStatusArchived: true},
	TopicStatusPaused:   {TopicStatusActive: true, TopicStatusArchived: true},
	TopicStatusArchived: {},
}

// CanTransitionTo returns true if the transition from current to target is allowed.
func (s TopicStatus) CanTransitionTo(target TopicStatus) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	return allowed[target]
}
