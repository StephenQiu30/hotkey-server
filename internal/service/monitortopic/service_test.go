package monitortopic_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/service/monitortopic"
)

func TestCreateTopic_RequiredFields(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	// Empty name should fail
	_, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	// Empty language should fail
	_, err = svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err == nil {
		t.Fatal("expected error for empty language")
	}

	// Empty platforms should fail
	_, err = svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:   "usr_1",
		Name:     "AI Trends",
		Language: monitortopic.LanguageZH,
	})
	if err == nil {
		t.Fatal("expected error for empty platforms")
	}

	// Empty userID should fail
	_, err = svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}

func TestCreateTopic_InvalidEnums(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	// Invalid language
	_, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.Language("invalid"),
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err == nil {
		t.Fatal("expected error for invalid language")
	}

	// Invalid platform
	_, err = svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.Platform("tiktok")},
	})
	if err == nil {
		t.Fatal("expected error for invalid platform")
	}
}

func TestCreateTopic_SimilarityThresholdBounds(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()
	valid := monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	}

	// Negative threshold
	input := valid
	input.SimilarityThreshold = float64Ptr(-0.1)
	_, err := svc.CreateTopic(ctx, input)
	if err == nil {
		t.Fatal("expected error for negative similarity threshold")
	}

	// Threshold > 1.0
	input.SimilarityThreshold = float64Ptr(1.1)
	_, err = svc.CreateTopic(ctx, input)
	if err == nil {
		t.Fatal("expected error for similarity threshold > 1.0")
	}

	// Valid threshold
	input.SimilarityThreshold = float64Ptr(0.85)
	topic, err := svc.CreateTopic(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic.SimilarityThreshold != 0.85 {
		t.Fatalf("expected 0.85, got %f", topic.SimilarityThreshold)
	}
}

func TestCreateTopic_CollectIntervalMin(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()
	valid := monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	}

	// Below minimum (5 min)
	input := valid
	input.CollectIntervalMin = intPtr(4)
	_, err := svc.CreateTopic(ctx, input)
	if err == nil {
		t.Fatal("expected error for collect interval < 5")
	}

	// Valid
	input.CollectIntervalMin = intPtr(10)
	topic, err := svc.CreateTopic(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic.CollectIntervalMin != 10 {
		t.Fatalf("expected 10, got %d", topic.CollectIntervalMin)
	}
}

func TestCreateTopic_DefaultValues(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	topic, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if topic.Status != monitortopic.TopicStatusDraft {
		t.Fatalf("expected status draft, got %s", topic.Status)
	}
	if topic.SimilarityThreshold != 0.80 {
		t.Fatalf("expected default threshold 0.80, got %f", topic.SimilarityThreshold)
	}
	if topic.CollectIntervalMin != 30 {
		t.Fatalf("expected default interval 30, got %d", topic.CollectIntervalMin)
	}
	if !topic.DailyReportEnabled {
		t.Fatal("expected daily report enabled by default")
	}
	if topic.ID == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestGetTopic_FoundAndNotFound(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	found, err := svc.GetTopic(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.Name != "AI Trends" {
		t.Fatalf("expected AI Trends, got %s", found.Name)
	}

	_, err = svc.GetTopic(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent topic")
	}
}

func TestListTopics_FilterByUser(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	if _, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID: "usr_1", Name: "Topic A", Language: monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	}); err != nil {
		t.Fatalf("create topic A: %v", err)
	}
	if _, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID: "usr_1", Name: "Topic B", Language: monitortopic.LanguageEN,
		Platforms: []monitortopic.Platform{monitortopic.PlatformTwitter},
	}); err != nil {
		t.Fatalf("create topic B: %v", err)
	}
	if _, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID: "usr_2", Name: "Topic C", Language: monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	}); err != nil {
		t.Fatalf("create topic C: %v", err)
	}

	topics, err := svc.ListTopics(ctx, "usr_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}
}

func TestUpdateTopic_SuccessAndValidation(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Update name
	updated, err := svc.UpdateTopic(ctx, monitortopic.UpdateTopicInput{
		TopicID: created.ID,
		Name:    strPtr("AI & ML Trends"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "AI & ML Trends" {
		t.Fatalf("expected AI & ML Trends, got %s", updated.Name)
	}

	// Empty name should fail
	_, err = svc.UpdateTopic(ctx, monitortopic.UpdateTopicInput{
		TopicID: created.ID,
		Name:    strPtr(""),
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	// Invalid threshold
	_, err = svc.UpdateTopic(ctx, monitortopic.UpdateTopicInput{
		TopicID:             created.ID,
		SimilarityThreshold: float64Ptr(2.0),
	})
	if err == nil {
		t.Fatal("expected error for invalid threshold")
	}

	// Nonexistent topic
	_, err = svc.UpdateTopic(ctx, monitortopic.UpdateTopicInput{
		TopicID: "nonexistent",
		Name:    strPtr("Updated"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent topic")
	}
}

func TestStatusTransitions_ValidAndInvalid(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// draft → active
	activated, err := svc.SetTopicStatus(ctx, created.ID, monitortopic.TopicStatusActive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activated.Status != monitortopic.TopicStatusActive {
		t.Fatalf("expected active, got %s", activated.Status)
	}

	// active → paused
	paused, err := svc.SetTopicStatus(ctx, created.ID, monitortopic.TopicStatusPaused)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paused.Status != monitortopic.TopicStatusPaused {
		t.Fatalf("expected paused, got %s", paused.Status)
	}

	// paused → active (reactivate)
	reactivated, err := svc.SetTopicStatus(ctx, created.ID, monitortopic.TopicStatusActive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reactivated.Status != monitortopic.TopicStatusActive {
		t.Fatalf("expected active, got %s", reactivated.Status)
	}

	// active → archived
	archived, err := svc.SetTopicStatus(ctx, created.ID, monitortopic.TopicStatusArchived)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archived.Status != monitortopic.TopicStatusArchived {
		t.Fatalf("expected archived, got %s", archived.Status)
	}

	// archived → active (invalid)
	_, err = svc.SetTopicStatus(ctx, created.ID, monitortopic.TopicStatusActive)
	if err == nil {
		t.Fatal("expected error for archived → active")
	}

	// draft → paused (invalid)
	created2, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID: "usr_1", Name: "Another", Language: monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic 2: %v", err)
	}
	_, err = svc.SetTopicStatus(ctx, created2.ID, monitortopic.TopicStatusPaused)
	if err == nil {
		t.Fatal("expected error for draft → paused")
	}
}

func TestDeleteTopic_CascadingCleanup(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Add keywords
	if _, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "GPT", Type: monitortopic.KeywordTypeInclude,
	}); err != nil {
		t.Fatalf("add keyword GPT: %v", err)
	}
	if _, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "spam", Type: monitortopic.KeywordTypeExclude,
	}); err != nil {
		t.Fatalf("add keyword spam: %v", err)
	}

	// Delete topic
	err = svc.DeleteTopic(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Topic should be gone
	_, err = svc.GetTopic(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error for deleted topic")
	}

	// Keywords should be gone
	keywords, err := svc.ListKeywords(ctx, created.ID)
	if err != nil {
		t.Fatalf("list keywords: %v", err)
	}
	if len(keywords) != 0 {
		t.Fatalf("expected 0 keywords after delete, got %d", len(keywords))
	}
}

func TestKeywordManagement_AddListDelete(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Add include keyword
	kw1, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "GPT", Type: monitortopic.KeywordTypeInclude,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kw1.Word != "GPT" || kw1.Type != monitortopic.KeywordTypeInclude {
		t.Fatalf("unexpected keyword: %+v", kw1)
	}

	// Add exclude keyword
	kw2, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "spam", Type: monitortopic.KeywordTypeExclude,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kw2.Type != monitortopic.KeywordTypeExclude {
		t.Fatalf("expected exclude, got %s", kw2.Type)
	}

	// List keywords
	keywords, err := svc.ListKeywords(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d", len(keywords))
	}

	// Delete keyword
	err = svc.DeleteKeyword(ctx, kw1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	keywords, err = svc.ListKeywords(ctx, created.ID)
	if err != nil {
		t.Fatalf("list keywords after delete: %v", err)
	}
	if len(keywords) != 1 {
		t.Fatalf("expected 1 keyword after delete, got %d", len(keywords))
	}
}

func TestKeywordLimits_MaxIncludeAndExclude(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID:    "usr_1",
		Name:      "AI Trends",
		Language:  monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Add 50 include keywords (max)
	for i := 0; i < 50; i++ {
		_, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
			TopicID: created.ID, Word: "kw" + intToStr(i), Type: monitortopic.KeywordTypeInclude,
		})
		if err != nil {
			t.Fatalf("unexpected error at %d: %v", i, err)
		}
	}

	// 51st include keyword should fail
	_, err = svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "overflow", Type: monitortopic.KeywordTypeInclude,
	})
	if err == nil {
		t.Fatal("expected error for 51st include keyword")
	}

	// Add 100 exclude keywords (max)
	for i := 0; i < 100; i++ {
		_, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
			TopicID: created.ID, Word: "ex" + intToStr(i), Type: monitortopic.KeywordTypeExclude,
		})
		if err != nil {
			t.Fatalf("unexpected error at %d: %v", i, err)
		}
	}

	// 101st exclude keyword should fail
	_, err = svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "exoverflow", Type: monitortopic.KeywordTypeExclude,
	})
	if err == nil {
		t.Fatal("expected error for 101st exclude keyword")
	}
}

func TestAddKeyword_EmptyWordAndInvalidTopic(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	ctx := context.Background()

	// Empty word
	_, err := svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: "some-topic", Word: "", Type: monitortopic.KeywordTypeInclude,
	})
	if err == nil {
		t.Fatal("expected error for empty word")
	}

	// Nonexistent topic
	_, err = svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: "nonexistent", Word: "test", Type: monitortopic.KeywordTypeInclude,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent topic")
	}

	// Invalid keyword type
	created, err := svc.CreateTopic(ctx, monitortopic.CreateTopicInput{
		UserID: "usr_1", Name: "Test", Language: monitortopic.LanguageZH,
		Platforms: []monitortopic.Platform{monitortopic.PlatformWeibo},
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	_, err = svc.AddKeyword(ctx, monitortopic.AddKeywordInput{
		TopicID: created.ID, Word: "test", Type: monitortopic.KeywordType("invalid"),
	})
	if err == nil {
		t.Fatal("expected error for invalid keyword type")
	}
}

func TestStatusTransitionConstants(t *testing.T) {
	// Verify the state machine transitions are correct
	cases := []struct {
		from    monitortopic.TopicStatus
		to      monitortopic.TopicStatus
		allowed bool
	}{
		{monitortopic.TopicStatusDraft, monitortopic.TopicStatusActive, true},
		{monitortopic.TopicStatusDraft, monitortopic.TopicStatusPaused, false},
		{monitortopic.TopicStatusDraft, monitortopic.TopicStatusArchived, true},
		{monitortopic.TopicStatusActive, monitortopic.TopicStatusPaused, true},
		{monitortopic.TopicStatusActive, monitortopic.TopicStatusArchived, true},
		{monitortopic.TopicStatusActive, monitortopic.TopicStatusDraft, false},
		{monitortopic.TopicStatusPaused, monitortopic.TopicStatusActive, true},
		{monitortopic.TopicStatusPaused, monitortopic.TopicStatusArchived, true},
		{monitortopic.TopicStatusPaused, monitortopic.TopicStatusDraft, false},
		{monitortopic.TopicStatusArchived, monitortopic.TopicStatusActive, false},
		{monitortopic.TopicStatusArchived, monitortopic.TopicStatusDraft, false},
	}
	for _, tc := range cases {
		got := tc.from.CanTransitionTo(tc.to)
		if got != tc.allowed {
			t.Errorf("%s → %s: expected %v, got %v", tc.from, tc.to, tc.allowed, got)
		}
	}
}

func strPtr(s string) *string       { return &s }
func float64Ptr(f float64) *float64 { return &f }
func intPtr(i int) *int             { return &i }
func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for i > 0 {
		pos--
		digits[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(digits[pos:])
}
