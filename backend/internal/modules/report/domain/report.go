package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

type ReportType string

const (
	ReportDaily  ReportType = "daily"
	ReportWeekly ReportType = "weekly"
)

type ReportStatus string

const (
	ReportDraft           ReportStatus = "draft"
	ReportPendingApproval ReportStatus = "pending_approval"
	ReportPublished       ReportStatus = "published"
	ReportRejected        ReportStatus = "rejected"
	ReportFailed          ReportStatus = "failed"
	ReportArchived        ReportStatus = "archived"
)

type Period struct {
	Start, End time.Time
	Location   *time.Location
}

func (period Period) Validate() error {
	if period.Start.IsZero() || period.End.IsZero() || !period.End.After(period.Start) || period.Location == nil {
		return fmt.Errorf("invalid report period")
	}
	return nil
}

func PeriodFor(at time.Time, reportType ReportType, location *time.Location) (Period, error) {
	if location == nil {
		return Period{}, fmt.Errorf("report timezone is required")
	}
	local := at.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	switch reportType {
	case ReportDaily:
		return Period{Start: start, End: start.AddDate(0, 0, 1), Location: location}, nil
	case ReportWeekly:
		delta := (int(start.Weekday()) + 6) % 7
		week := start.AddDate(0, 0, -delta)
		return Period{Start: week, End: week.AddDate(0, 0, 7), Location: location}, nil
	default:
		return Period{}, fmt.Errorf("invalid report type")
	}
}

type Item struct {
	// EventID/EventUpdateID retain read compatibility for reports frozen before
	// Product Event v2. New drafts freeze the exact MicroEvent update and cited
	// summary instead; legacy aggregates are never valid publication inputs.
	EventID, EventUpdateID                              int64
	MicroEventID, MicroEventVersion, MicroEventUpdateID int64
	MicroEventSummaryID                                 int64
	Rank                                                int
	Title, Summary, InclusionReason                     string
	EvidenceSetHash                                     string
	ReasonCodes                                         []string
	HeatScore                                           float64
	Sentences                                           []Sentence
}

type Sentence struct {
	SourceSummarySentenceID int64
	Ordinal                 int
	Text                    string
	EditorialNote           bool
	DecisionOrigin          string
	ModelRunID, ActorUserID *int64
	ClaimEvidenceVersionIDs []int64
}

var (
	ErrEvidenceInvalid = errors.New("report evidence is invalid")
	ErrUnsafeContent   = errors.New("report content is unsafe")
)

var (
	reportHTMLTagPattern        = regexp.MustCompile(`(?i)<\s*/?\s*[a-z][^>]*>`)
	reportEventAttributePattern = regexp.MustCompile(`(?i)\bon[a-z0-9_-]+\s*=`)
)

type Report struct {
	ID, Version, VersionNo  int64
	Type                    ReportType
	MonitorID               *int64
	Period                  Period
	Title, Summary, Body    string
	InputSnapshotHash       string
	Status                  ReportStatus
	Items                   []Item
	Frozen                  bool
	GeneratedAt             *time.Time
	PublishedAt             *time.Time
	SubmittedAt, ReviewedAt *time.Time
	CreatedBy, UpdatedBy    *int64
	SubmittedBy, ReviewedBy *int64
	ReviewReason            string
}

type ListQuery struct {
	Cursor int64
	Limit  int
	Type   *ReportType
	Status *ReportStatus
}

type Page struct {
	Items      []Report
	NextCursor int64
}

type RevisionTransition struct {
	ReportID, ExpectedVersion, ActorID int64
	From, To                           ReportStatus
	ReasonCode                         string
}

func (query ListQuery) Validate() error {
	if query.Cursor < 0 || query.Limit < 1 || query.Limit > 100 {
		return fmt.Errorf("invalid report list query")
	}
	if query.Type != nil && *query.Type != ReportDaily && *query.Type != ReportWeekly {
		return fmt.Errorf("invalid report type")
	}
	if query.Status != nil && !validStatus(*query.Status) {
		return fmt.Errorf("invalid report status")
	}
	return nil
}

func (report Report) Validate() error {
	if report.ID <= 0 || report.Version <= 0 || report.VersionNo <= 0 || (report.Type != ReportDaily && report.Type != ReportWeekly) || !validStatus(report.Status) {
		return fmt.Errorf("invalid report")
	}
	if err := report.Period.Validate(); err != nil {
		return err
	}
	if unsafeReportContent(report.Title) || unsafeReportContent(report.Summary) || unsafeReportContent(report.Body) {
		return ErrUnsafeContent
	}
	if report.Status == ReportPublished && !report.Frozen {
		return fmt.Errorf("published report must be frozen")
	}
	if report.Status != ReportPublished && report.Frozen {
		return fmt.Errorf("only published report can be frozen")
	}
	for _, item := range report.Items {
		if err := item.validate(); err != nil {
			return err
		}
	}
	if len(report.InputSnapshotHash) != 64 || report.InputSnapshotHash != ComputeInputSnapshotHash(report) {
		return fmt.Errorf("invalid report input snapshot hash")
	}
	return nil
}

func ComputeInputSnapshotHash(report Report) string {
	timezone := ""
	if report.Period.Location != nil {
		timezone = report.Period.Location.String()
	}
	payload, _ := json.Marshal(struct {
		Type        ReportType `json:"type"`
		MonitorID   *int64     `json:"monitor_id"`
		PeriodStart string     `json:"period_start"`
		PeriodEnd   string     `json:"period_end"`
		Timezone    string     `json:"timezone"`
		Items       []Item     `json:"items"`
	}{
		Type: report.Type, MonitorID: report.MonitorID,
		PeriodStart: report.Period.Start.UTC().Format(time.RFC3339Nano),
		PeriodEnd:   report.Period.End.UTC().Format(time.RFC3339Nano),
		Timezone:    timezone, Items: report.Items,
	})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func (report Report) ValidatePublicationShape() error {
	if err := report.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrEvidenceInvalid, err)
	}
	for _, item := range report.Items {
		if !item.isMicroEventSnapshot() || len(item.Sentences) == 0 {
			return fmt.Errorf("%w: publication requires cited Product Event snapshots", ErrEvidenceInvalid)
		}
		for _, sentence := range item.Sentences {
			if err := sentence.validate(); err != nil {
				return fmt.Errorf("%w: %w", ErrEvidenceInvalid, err)
			}
		}
	}
	return nil
}

func (item Item) validate() error {
	if item.Rank <= 0 || strings.TrimSpace(item.Title) == "" || item.HeatScore < 0 || len(item.EvidenceSetHash) != 64 || len(item.ReasonCodes) == 0 {
		return fmt.Errorf("invalid report item")
	}
	if unsafeReportContent(item.Title) || unsafeReportContent(item.Summary) || unsafeReportContent(item.InclusionReason) {
		return ErrUnsafeContent
	}
	legacy := item.EventID > 0 && item.EventUpdateID > 0 && item.MicroEventID == 0 && item.MicroEventVersion == 0 && item.MicroEventUpdateID == 0 && item.MicroEventSummaryID == 0
	v2 := item.isMicroEventSnapshot() && item.EventID == 0 && item.EventUpdateID == 0
	if !legacy && !v2 || legacy && len(item.Sentences) != 0 {
		return fmt.Errorf("invalid report item identity")
	}
	for index, sentence := range item.Sentences {
		if sentence.Ordinal != index {
			return fmt.Errorf("invalid report sentence ordinal")
		}
		if err := sentence.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (item Item) isMicroEventSnapshot() bool {
	return item.MicroEventID > 0 && item.MicroEventVersion > 0 && item.MicroEventUpdateID > 0 && item.MicroEventSummaryID > 0
}

func (sentence Sentence) validate() error {
	if sentence.SourceSummarySentenceID <= 0 || sentence.Ordinal < 0 || strings.TrimSpace(sentence.Text) == "" || len(sentence.Text) > 8000 {
		return fmt.Errorf("invalid report sentence")
	}
	if unsafeReportContent(sentence.Text) {
		return ErrUnsafeContent
	}
	switch sentence.DecisionOrigin {
	case "automatic":
		if sentence.EditorialNote || sentence.ModelRunID == nil || *sentence.ModelRunID <= 0 || sentence.ActorUserID != nil {
			return fmt.Errorf("invalid automatic report sentence provenance")
		}
	case "manual":
		if sentence.ModelRunID != nil || sentence.ActorUserID == nil || *sentence.ActorUserID <= 0 {
			return fmt.Errorf("invalid manual report sentence provenance")
		}
	default:
		return fmt.Errorf("invalid report sentence provenance")
	}
	if sentence.EditorialNote && len(sentence.ClaimEvidenceVersionIDs) != 0 || !sentence.EditorialNote && len(sentence.ClaimEvidenceVersionIDs) == 0 {
		return fmt.Errorf("factual report sentence requires ClaimEvidence")
	}
	seen := make(map[int64]struct{}, len(sentence.ClaimEvidenceVersionIDs))
	for _, evidenceID := range sentence.ClaimEvidenceVersionIDs {
		if evidenceID <= 0 {
			return fmt.Errorf("invalid report sentence citation")
		}
		if _, found := seen[evidenceID]; found {
			return fmt.Errorf("duplicated report sentence citation")
		}
		seen[evidenceID] = struct{}{}
	}
	return nil
}

func unsafeReportContent(value string) bool {
	if len(value) > 256*1024 {
		return true
	}
	candidates := []string{strings.ToLower(value)}
	seen := map[string]struct{}{candidates[0]: {}}
	for index := 0; index < len(candidates) && index < 16; index++ {
		for _, decoded := range []string{stdhtml.UnescapeString(candidates[index]), queryUnescapeReportContent(candidates[index])} {
			if decoded == candidates[index] {
				continue
			}
			if _, found := seen[decoded]; !found {
				seen[decoded] = struct{}{}
				candidates = append(candidates, decoded)
			}
		}
	}
	for _, candidate := range candidates {
		if strings.IndexByte(candidate, 0) >= 0 || reportHTMLTagPattern.MatchString(candidate) ||
			reportEventAttributePattern.MatchString(candidate) {
			return true
		}
		compact := strings.Map(func(character rune) rune {
			if unicode.IsSpace(character) || unicode.IsControl(character) {
				return -1
			}
			return character
		}, candidate)
		for _, marker := range []string{"javascript:", "vbscript:", "data:text/html", "data:image/svg+xml", "srcdoc=", "expression("} {
			if strings.Contains(compact, marker) {
				return true
			}
		}
	}
	return false
}

func queryUnescapeReportContent(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func validStatus(status ReportStatus) bool {
	switch status {
	case ReportDraft, ReportPendingApproval, ReportPublished, ReportRejected, ReportFailed, ReportArchived:
		return true
	default:
		return false
	}
}

func SortItems(items []Item) []Item {
	result := append([]Item(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].HeatScore != result[j].HeatScore {
			return result[i].HeatScore > result[j].HeatScore
		}
		leftID, rightID := result[i].EventID, result[j].EventID
		if leftID == 0 {
			leftID = result[i].MicroEventID
		}
		if rightID == 0 {
			rightID = result[j].MicroEventID
		}
		return leftID < rightID
	})
	for index := range result {
		result[index].Rank = index + 1
	}
	return result
}
