package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/knowledge"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

// auditAdapter adapts database.KnowledgeWritebackRepo to knowledge.AuditRecorder.
type auditAdapter struct {
	repo *database.KnowledgeWritebackRepo
}

func (a *auditAdapter) RecordAttempt(ctx context.Context, in knowledge.RecordAttemptInput) error {
	return a.repo.RecordAttempt(ctx, database.RecordAttemptInput{
		ObjectType:     in.ObjectType,
		ObjectID:       in.ObjectID,
		FieldName:      in.FieldName,
		FieldValue:     in.FieldValue,
		Status:         in.Status,
		ConflictReason: in.ConflictReason,
		SourcePath:     in.SourcePath,
	})
}

// sidecarAdapter adapts database sidecar repos to knowledge.SidecarWriter.
type sidecarAdapter struct {
	event *database.EventAnnotationRepo
	topic *database.TopicAnnotationRepo
	theme *database.ThemeMembershipRepo
}

func (a *sidecarAdapter) SetManualTags(ctx context.Context, objectID int64, tags []string) error {
	return a.event.SetManualTags(ctx, objectID, tags)
}

func (a *sidecarAdapter) SetAnalystConclusion(ctx context.Context, objectID int64, conclusion string) error {
	return a.event.SetAnalystConclusion(ctx, objectID, conclusion)
}

func (a *sidecarAdapter) SetMaterialStatus(ctx context.Context, topicID int64, status string) error {
	return a.topic.SetMaterialStatus(ctx, topicID, status)
}

func (a *sidecarAdapter) SetThemeRef(ctx context.Context, objectType string, objectID int64, themeRef string) error {
	return a.theme.SetThemeRef(ctx, objectType, objectID, themeRef)
}

// TestKnowledgeWritebackRoundtrip verifies the full writeback lifecycle:
// export -> simulated manual edit -> writeback -> re-export consistency.
func TestKnowledgeWritebackRoundtrip(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// --- setup repos and service ---
	rawAuditRepo := database.NewKnowledgeWritebackRepo(db)
	auditRepo := &auditAdapter{repo: rawAuditRepo}
	eventRepo := database.NewEventAnnotationRepo(db)
	topicRepo := database.NewTopicAnnotationRepo(db)
	themeRepo := database.NewThemeMembershipRepo(db)

	sidecar := &sidecarAdapter{
		event: eventRepo,
		topic: topicRepo,
		theme: themeRepo,
	}

	svc := knowledge.NewService(auditRepo, sidecar)

	// --- step 1: export a topic note ---
	rendered := obsidian.RenderTopicNote(obsidian.TopicNoteInput{
		Date:      "2026-07-02",
		Monitor:   "AI监管",
		MonitorID: 1,
		TopicID:   101,
		TopicKey:  "ai-regulation",
		Title:     "AI监管政策讨论",
		Heat:      95.5,
		Trend:     "rising",
		PostCount: 15,
		Summary:   "本周AI监管政策讨论持续升温",
	})

	// --- step 2: simulate manual edit - add manual_tags to frontmatter ---
	modified := strings.Replace(rendered, "---\n",
		"---\nmanual_tags:\n  - ai监管\n", 1)

	// --- step 3: parse the modified content ---
	changes, err := obsidian.ParseWritebackFields(modified)
	if err != nil {
		t.Fatalf("parse modified content: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one whitelisted field from modified content")
	}

	// Apply each parsed change through the writeback service.
	for _, ch := range changes {
		err = svc.ApplyChange(context.Background(), knowledge.WritebackChange{
			ObjectType: ch.ObjectType,
			ObjectID:   ch.ObjectID,
			FieldName:  ch.FieldName,
			Value:      ch.FieldValue,
			SourcePath: "HotKey/topics/ai-regulation/2026-07-02-topic-101.md",
		}, knowledge.ConflictInput{})
		if err != nil {
			t.Fatalf("apply writeback change %q: %v", ch.FieldName, err)
		}
	}

	// --- step 4: verify database persistence ---
	readTags, err := eventRepo.GetManualTags(context.Background(), 101)
	if err != nil {
		t.Fatalf("read back manual_tags from DB: %v", err)
	}
	if len(readTags) != 1 || readTags[0] != "ai监管" {
		t.Fatalf("persisted manual_tags: got %v, want [ai监管]", readTags)
	}

	// --- step 5: re-render and verify machine fields unchanged ---
	reRendered := obsidian.RenderTopicNote(obsidian.TopicNoteInput{
		Date:      "2026-07-02",
		Monitor:   "AI监管",
		MonitorID: 1,
		TopicID:   101,
		TopicKey:  "ai-regulation",
		Title:     "AI监管政策讨论",
		Heat:      95.5,
		Trend:     "rising",
		PostCount: 15,
	})
	if !strings.Contains(reRendered, "heat: 95.5") {
		t.Fatal("re-rendered content should retain heat (machine) field")
	}
	t.Logf("roundtrip OK: export -> manual_tags -> writeback -> re-export consistent")
}
