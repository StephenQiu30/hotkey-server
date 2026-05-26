package report

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ScopePlatform = "platform"
	ScopeUser     = "user"
)

var ErrMissingEvidenceLink = errors.New("missing evidence link")

type HotspotSnapshot struct {
	EventID     string   `json:"eventId"`
	Title       string   `json:"title"`
	Keywords    []string `json:"keywords"`
	HeatScore   int      `json:"heatScore"`
	TrustScore  int      `json:"trustScore"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type DailyReportItem struct {
	EventID     string   `json:"eventId"`
	Title       string   `json:"title"`
	Keywords    []string `json:"keywords"`
	HeatScore   int      `json:"heatScore"`
	TrustScore  int      `json:"trustScore"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type DailyReport struct {
	ID          string            `json:"id"`
	Scope       string            `json:"scope"`
	UserID      string            `json:"userId,omitempty"`
	ReportDate  time.Time         `json:"reportDate"`
	GeneratedAt time.Time         `json:"generatedAt"`
	Items       []DailyReportItem `json:"items"`
}

type Service struct {
	mu      sync.Mutex
	reports map[string]DailyReport
}

func NewService() *Service {
	return &Service{
		reports: make(map[string]DailyReport),
	}
}

func (s *Service) GeneratePlatformDailyReport(reportDate time.Time, hotspots []HotspotSnapshot) (DailyReport, error) {
	items, err := buildReportItems(hotspots)
	if err != nil {
		return DailyReport{}, err
	}
	report := DailyReport{
		ID:          reportID(ScopePlatform, "", reportDate),
		Scope:       ScopePlatform,
		ReportDate:  normalizeDate(reportDate),
		GeneratedAt: time.Now().UTC(),
		Items:       items,
	}
	s.save(report)
	return cloneReport(report), nil
}

func (s *Service) GenerateUserDailyReport(reportDate time.Time, userID string, followedKeywords []string, hotspots []HotspotSnapshot) (DailyReport, error) {
	userID = strings.TrimSpace(userID)
	filtered := filterByKeywords(hotspots, followedKeywords)
	items, err := buildReportItems(filtered)
	if err != nil {
		return DailyReport{}, err
	}
	report := DailyReport{
		ID:          reportID(ScopeUser, userID, reportDate),
		Scope:       ScopeUser,
		UserID:      userID,
		ReportDate:  normalizeDate(reportDate),
		GeneratedAt: time.Now().UTC(),
		Items:       items,
	}
	s.save(report)
	return cloneReport(report), nil
}

func (s *Service) save(report DailyReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = cloneReport(report)
}

func buildReportItems(hotspots []HotspotSnapshot) ([]DailyReportItem, error) {
	items := make([]DailyReportItem, 0, len(hotspots))
	for _, hotspot := range hotspots {
		item, err := normalizeItem(hotspot)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].HeatScore == items[j].HeatScore {
			return items[i].EventID < items[j].EventID
		}
		return items[i].HeatScore > items[j].HeatScore
	})
	return items, nil
}

func normalizeItem(hotspot HotspotSnapshot) (DailyReportItem, error) {
	eventID := strings.TrimSpace(hotspot.EventID)
	title := strings.Join(strings.Fields(hotspot.Title), " ")
	evidenceIDs := normalizeStrings(hotspot.EvidenceIDs)
	if eventID == "" || title == "" || len(evidenceIDs) == 0 {
		return DailyReportItem{}, ErrMissingEvidenceLink
	}
	return DailyReportItem{
		EventID:     eventID,
		Title:       title,
		Keywords:    normalizeStrings(hotspot.Keywords),
		HeatScore:   hotspot.HeatScore,
		TrustScore:  hotspot.TrustScore,
		EvidenceIDs: evidenceIDs,
	}, nil
}

func filterByKeywords(hotspots []HotspotSnapshot, keywords []string) []HotspotSnapshot {
	allowed := make(map[string]struct{})
	for _, keyword := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return hotspots
	}
	filtered := make([]HotspotSnapshot, 0, len(hotspots))
	for _, hotspot := range hotspots {
		if matchesKeyword(hotspot, allowed) {
			filtered = append(filtered, hotspot)
		}
	}
	return filtered
}

func matchesKeyword(hotspot HotspotSnapshot, allowed map[string]struct{}) bool {
	title := strings.ToLower(hotspot.Title)
	for keyword := range allowed {
		if strings.Contains(title, keyword) {
			return true
		}
	}
	for _, keyword := range hotspot.Keywords {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(keyword))]; ok {
			return true
		}
	}
	return false
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := strings.Join(strings.Fields(value), " ")
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	sort.Strings(normalized)
	return normalized
}

func reportID(scope string, userID string, reportDate time.Time) string {
	date := normalizeDate(reportDate).Format("2006-01-02")
	if scope == ScopeUser {
		return scope + ":" + userID + ":" + date
	}
	return scope + ":" + date
}

func normalizeDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func cloneReport(report DailyReport) DailyReport {
	report.Items = append([]DailyReportItem(nil), report.Items...)
	for i := range report.Items {
		report.Items[i].Keywords = append([]string(nil), report.Items[i].Keywords...)
		report.Items[i].EvidenceIDs = append([]string(nil), report.Items[i].EvidenceIDs...)
	}
	return report
}
