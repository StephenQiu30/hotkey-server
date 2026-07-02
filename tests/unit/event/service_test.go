package event_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/event"
)

func TestService_BuildEventFromPosts(t *testing.T) {
	svc := event.NewService(nil)
	evt, err := svc.BuildEventFromPosts(event.BuildEventInput{
		MonitorID:  1,
		TopicID:    42,
		TopicTitle: "AI 监管",
		EventSeed:  "监管机构发布新规并引发多方解读",
		Posts: []event.PostFact{
			{PostID: 1001, PublishedAt: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC), Text: "监管规则发布"},
			{PostID: 1002, PublishedAt: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), Text: "多位从业者解读监管影响"},
		},
	})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if evt.EventKey == "" {
		t.Fatal("expected event key")
	}
}

func TestService_EventIsNotTopicAlias(t *testing.T) {
	svc := event.NewService(nil)
	evt, err := svc.BuildEventFromPosts(event.BuildEventInput{
		MonitorID:  1,
		TopicID:    42,
		TopicTitle: "AI 监管",
		EventSeed:  "监管新规发布",
		Posts: []event.PostFact{
			{PostID: 1001, PublishedAt: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC), Text: "监管规则发布"},
		},
	})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if evt.Title == "AI 监管" {
		t.Fatal("event title must not collapse to topic title by default")
	}
}

func TestService_BuildEventFromPosts_EmptyPosts(t *testing.T) {
	svc := event.NewService(nil)
	_, err := svc.BuildEventFromPosts(event.BuildEventInput{
		MonitorID:  1,
		TopicID:    42,
		TopicTitle: "AI 监管",
		EventSeed:  "test",
		Posts:      nil,
	})
	if err == nil {
		t.Fatal("expected error for empty posts")
	}
}
