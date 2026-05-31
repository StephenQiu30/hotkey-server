package channel

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ChannelStatus string

const (
	ChannelStatusActive   ChannelStatus = "active"
	ChannelStatusDisabled ChannelStatus = "disabled"

	defaultDailySendAtKey = "default_daily_send_at"
	defaultDailySendAt    = "08:30"
)

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrNotFound         = errors.New("not found")
	ErrChannelDisabled  = errors.New("channel disabled")
	ErrAlreadyExists    = errors.New("already exists")
	errInvalidTimeRegex = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

type Channel struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Status      ChannelStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Subscription struct {
	UserID    string
	Channel   Channel
	CreatedAt time.Time
}

type Keyword struct {
	ID        string
	UserID    string
	Keyword   string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repository interface {
	ListChannels(ctx context.Context, activeOnly bool) ([]Channel, error)
	ChannelByID(ctx context.Context, channelID string) (Channel, error)
	CreateChannel(ctx context.Context, channel Channel) (Channel, error)
	UpdateChannel(ctx context.Context, channel Channel) (Channel, error)
	DeleteChannel(ctx context.Context, channelID string) error
	UpsertSubscription(ctx context.Context, userID string, channelID string, createdAt time.Time) (Subscription, error)
	DeleteSubscription(ctx context.Context, userID string, channelID string) error
	ListSubscriptions(ctx context.Context, userID string) ([]Subscription, error)
	CreateKeyword(ctx context.Context, keyword Keyword) (Keyword, error)
	UpdateKeyword(ctx context.Context, keyword Keyword) (Keyword, error)
	KeywordByID(ctx context.Context, userID string, keywordID string) (Keyword, error)
	DeleteKeyword(ctx context.Context, userID string, keywordID string) error
	ListKeywords(ctx context.Context, userID string) ([]Keyword, error)
	Setting(ctx context.Context, key string) (string, error)
	UpsertSetting(ctx context.Context, key string, value string, updatedAt time.Time) error
	UserDailySendAt(ctx context.Context, userID string) (string, error)
	SetUserDailySendAt(ctx context.Context, userID string, dailySendAt string, updatedAt time.Time) error
}

type Service struct {
	repo Repository
	now  func() time.Time
}

type ListChannelsInput struct {
	ActiveOnly bool
}

type CreateChannelInput struct {
	Name        string
	Slug        string
	Description string
}

type UpdateChannelInput struct {
	ChannelID   string
	Name        string
	Slug        string
	Description string
}

type UpdateChannelStatusInput struct {
	ChannelID string
	Status    ChannelStatus
}

type UserChannelInput struct {
	UserID    string
	ChannelID string
}

type KeywordInput struct {
	UserID  string
	Keyword string
}

type UpdateKeywordInput struct {
	UserID    string
	KeywordID string
	Keyword   string
	Enabled   *bool
}

type UserDailySendAtInput struct {
	UserID      string
	DailySendAt string
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) ListChannels(ctx context.Context, input ListChannelsInput) ([]Channel, error) {
	return s.repo.ListChannels(ctx, input.ActiveOnly)
}

func (s *Service) CreateChannel(ctx context.Context, input CreateChannelInput) (Channel, error) {
	name := strings.TrimSpace(input.Name)
	slug := strings.TrimSpace(input.Slug)
	if name == "" || slug == "" {
		return Channel{}, ErrInvalidInput
	}
	now := s.now().UTC()
	return s.repo.CreateChannel(ctx, Channel{
		ID:          newID("chn"),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(input.Description),
		Status:      ChannelStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (s *Service) UpdateChannel(ctx context.Context, input UpdateChannelInput) (Channel, error) {
	found, err := s.repo.ChannelByID(ctx, input.ChannelID)
	if err != nil {
		return Channel{}, normalizeNotFound(err)
	}
	name := strings.TrimSpace(input.Name)
	slug := strings.TrimSpace(input.Slug)
	if name == "" || slug == "" {
		return Channel{}, ErrInvalidInput
	}
	found.Name = name
	found.Slug = slug
	found.Description = strings.TrimSpace(input.Description)
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateChannel(ctx, found)
}

func (s *Service) UpdateChannelStatus(ctx context.Context, input UpdateChannelStatusInput) (Channel, error) {
	if input.Status != ChannelStatusActive && input.Status != ChannelStatusDisabled {
		return Channel{}, ErrInvalidInput
	}
	found, err := s.repo.ChannelByID(ctx, input.ChannelID)
	if err != nil {
		return Channel{}, normalizeNotFound(err)
	}
	found.Status = input.Status
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateChannel(ctx, found)
}

func (s *Service) DeleteChannel(ctx context.Context, channelID string) error {
	if strings.TrimSpace(channelID) == "" {
		return ErrInvalidInput
	}
	return normalizeNotFound(s.repo.DeleteChannel(ctx, channelID))
}

func (s *Service) Subscribe(ctx context.Context, input UserChannelInput) (Subscription, error) {
	if strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ChannelID) == "" {
		return Subscription{}, ErrInvalidInput
	}
	found, err := s.repo.ChannelByID(ctx, input.ChannelID)
	if err != nil {
		return Subscription{}, normalizeNotFound(err)
	}
	if found.Status != ChannelStatusActive {
		return Subscription{}, ErrChannelDisabled
	}
	return s.repo.UpsertSubscription(ctx, input.UserID, input.ChannelID, s.now().UTC())
}

func (s *Service) Unsubscribe(ctx context.Context, input UserChannelInput) error {
	if strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ChannelID) == "" {
		return ErrInvalidInput
	}
	return normalizeNotFound(s.repo.DeleteSubscription(ctx, input.UserID, input.ChannelID))
}

func (s *Service) ListSubscriptions(ctx context.Context, userID string) ([]Subscription, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListSubscriptions(ctx, userID)
}

func (s *Service) CreateKeyword(ctx context.Context, input KeywordInput) (Keyword, error) {
	keyword := strings.TrimSpace(input.Keyword)
	if strings.TrimSpace(input.UserID) == "" || keyword == "" {
		return Keyword{}, ErrInvalidInput
	}
	now := s.now().UTC()
	return s.repo.CreateKeyword(ctx, Keyword{
		ID:        newID("kwd"),
		UserID:    input.UserID,
		Keyword:   keyword,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Service) UpdateKeyword(ctx context.Context, input UpdateKeywordInput) (Keyword, error) {
	found, err := s.repo.KeywordByID(ctx, input.UserID, input.KeywordID)
	if err != nil {
		return Keyword{}, normalizeNotFound(err)
	}
	if strings.TrimSpace(input.Keyword) != "" {
		found.Keyword = strings.TrimSpace(input.Keyword)
	}
	if input.Enabled != nil {
		found.Enabled = *input.Enabled
	}
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateKeyword(ctx, found)
}

func (s *Service) DeleteKeyword(ctx context.Context, userID string, keywordID string) error {
	return normalizeNotFound(s.repo.DeleteKeyword(ctx, userID, keywordID))
}

func (s *Service) ListKeywords(ctx context.Context, userID string) ([]Keyword, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListKeywords(ctx, userID)
}

func (s *Service) SetDefaultDailySendAt(ctx context.Context, dailySendAt string) error {
	if !validDailySendAt(dailySendAt) {
		return ErrInvalidInput
	}
	return s.repo.UpsertSetting(ctx, defaultDailySendAtKey, dailySendAt, s.now().UTC())
}

func (s *Service) DefaultDailySendAt(ctx context.Context) (string, error) {
	value, err := s.repo.Setting(ctx, defaultDailySendAtKey)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
		return defaultDailySendAt, nil
	}
	return value, err
}

func (s *Service) SetUserDailySendAt(ctx context.Context, input UserDailySendAtInput) error {
	if strings.TrimSpace(input.UserID) == "" || !validDailySendAt(input.DailySendAt) {
		return ErrInvalidInput
	}
	return s.repo.SetUserDailySendAt(ctx, input.UserID, input.DailySendAt, s.now().UTC())
}

func (s *Service) UserDailySendAt(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", ErrInvalidInput
	}
	return s.repo.UserDailySendAt(ctx, userID)
}

func validDailySendAt(value string) bool {
	return errInvalidTimeRegex.MatchString(value)
}

func normalizeNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func newID(prefix string) string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
