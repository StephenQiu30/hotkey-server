package rss

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"strings"
	"time"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrFeedNotFound  = errors.New("feed not found")
	ErrFeedDisabled  = errors.New("feed disabled")
	ErrTokenGenerate = errors.New("token generate")
)

type Config struct {
	BaseURL string
	Now     func() time.Time
}

type Feed struct {
	UserID         string
	TokenHash      string
	Enabled        bool
	LastAccessedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type FeedRepository interface {
	FindByUserID(context.Context, string) (Feed, error)
	FindByTokenHash(context.Context, string) (Feed, error)
	Save(context.Context, Feed) (Feed, error)
	Disable(context.Context, string, time.Time) error
	Touch(context.Context, string, time.Time) error
}

type ReportRepository interface {
	ListReportsByChannel(context.Context, string) ([]servicereport.DailyReport, error)
	ListReportsByUser(context.Context, string) ([]servicereport.DailyReport, error)
}

type Service struct {
	feeds   FeedRepository
	reports ReportRepository
	baseURL string
	now     func() time.Time
}

func NewService(feeds FeedRepository, reports ReportRepository, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		feeds:   feeds,
		reports: reports,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		now:     now,
	}
}

func (s *Service) UserFeed(ctx context.Context, userID string) (Feed, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Feed{}, ErrInvalidInput
	}
	return s.feeds.FindByUserID(ctx, userID)
}

func (s *Service) ResetUserFeed(ctx context.Context, userID string) (Feed, string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Feed{}, "", ErrInvalidInput
	}
	token, err := newToken()
	if err != nil {
		return Feed{}, "", err
	}
	now := s.now().UTC()
	feed, err := s.feeds.FindByUserID(ctx, userID)
	if err != nil && !errors.Is(err, ErrFeedNotFound) {
		return Feed{}, "", err
	}
	if errors.Is(err, ErrFeedNotFound) {
		feed = Feed{UserID: userID, CreatedAt: now}
	}
	feed.TokenHash = hashToken(token)
	feed.Enabled = true
	feed.UpdatedAt = now
	feed.LastAccessedAt = nil
	feed, err = s.feeds.Save(ctx, feed)
	if err != nil {
		return Feed{}, "", err
	}
	return feed, token, nil
}

func (s *Service) DisableUserFeed(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrInvalidInput
	}
	return s.feeds.Disable(ctx, userID, s.now().UTC())
}

func (s *Service) PublicChannelFeed(ctx context.Context, channelID string) (Document, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return Document{}, ErrInvalidInput
	}
	reports, err := s.reports.ListReportsByChannel(ctx, channelID)
	if err != nil {
		return Document{}, err
	}
	return s.document("HotKey "+channelID+" 日报", s.baseURL+"/rss/channels/"+channelID+".xml", reports), nil
}

func (s *Service) PrivateUserFeed(ctx context.Context, token string) (Document, error) {
	token = strings.TrimSpace(strings.TrimSuffix(token, ".xml"))
	if token == "" {
		return Document{}, ErrFeedNotFound
	}
	feed, err := s.feeds.FindByTokenHash(ctx, hashToken(token))
	if err != nil {
		return Document{}, err
	}
	if !feed.Enabled {
		return Document{}, ErrFeedDisabled
	}
	reports, err := s.reports.ListReportsByUser(ctx, feed.UserID)
	if err != nil {
		return Document{}, err
	}
	now := s.now().UTC()
	_ = s.feeds.Touch(ctx, feed.UserID, now)
	return s.document("HotKey 私有日报", s.baseURL+"/rss/users/"+token+".xml", reports), nil
}

func (s *Service) document(title string, link string, reports []servicereport.DailyReport) Document {
	items := make([]Item, 0, len(reports))
	for _, report := range reports {
		published := report.CreatedAt
		if published.IsZero() {
			published = report.UpdatedAt
		}
		items = append(items, Item{
			Title:       reportTitle(report),
			Link:        reportLink(s.baseURL, report),
			PubDate:     published.UTC().Format(time.RFC1123Z),
			GUID:        GUID{IsPermaLink: false, Value: "daily-report:" + report.ID},
			Description: report.Body,
		})
	}
	return Document{Version: "2.0", Channel: Channel{
		Title:       title,
		Link:        link,
		Description: title,
		Items:       items,
	}}
}

type Document struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Items       []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	GUID        GUID   `xml:"guid"`
	Description string `xml:"description"`
}

type GUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func (d Document) XML() ([]byte, error) {
	body, err := xml.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func reportTitle(report servicereport.DailyReport) string {
	if report.ChannelID != "" {
		return report.Date + " " + report.ChannelID + " 日报"
	}
	return report.Date + " 私有日报"
}

func reportLink(baseURL string, report servicereport.DailyReport) string {
	if baseURL == "" {
		return "/api/v1/reports/" + report.ID
	}
	return baseURL + "/api/v1/reports/" + report.ID
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", ErrTokenGenerate
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
