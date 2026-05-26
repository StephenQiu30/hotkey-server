package keyword

import (
	"errors"
	"testing"
)

func TestServiceManagesPlatformKeywords(t *testing.T) {
	service := NewService()

	created, err := service.CreatePlatformKeyword(CreatePlatformKeywordInput{
		Term:     " OpenAI ",
		Category: "lab",
	})
	if err != nil {
		t.Fatalf("create keyword: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("keyword ID is empty")
	}
	if created.Term != "OpenAI" {
		t.Fatalf("term = %q, want OpenAI", created.Term)
	}
	if !created.Enabled {
		t.Fatalf("keyword should be enabled by default")
	}

	updated, err := service.SetPlatformKeywordEnabled(created.ID, false)
	if err != nil {
		t.Fatalf("disable keyword: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("keyword should be disabled")
	}

	keywords := service.ListPlatformKeywords()
	if len(keywords) != 1 {
		t.Fatalf("keywords len = %d, want 1", len(keywords))
	}
	if keywords[0].ID != created.ID {
		t.Fatalf("keyword ID = %q, want %q", keywords[0].ID, created.ID)
	}
}

func TestServiceRejectsBlankPlatformKeyword(t *testing.T) {
	service := NewService()

	_, err := service.CreatePlatformKeyword(CreatePlatformKeywordInput{Term: "  "})

	if !errors.Is(err, ErrInvalidKeyword) {
		t.Fatalf("err = %v, want ErrInvalidKeyword", err)
	}
}

func TestServiceStoresUserKeywordPreferences(t *testing.T) {
	service := NewService()

	if err := service.FollowKeyword("user-1", "OpenAI"); err != nil {
		t.Fatalf("follow keyword: %v", err)
	}
	if err := service.BlockKeyword("user-1", "AI slop"); err != nil {
		t.Fatalf("block keyword: %v", err)
	}
	if err := service.AddUserKeyword("user-1", "Claude Code"); err != nil {
		t.Fatalf("add keyword: %v", err)
	}

	preferences := service.GetUserPreferences("user-1")
	if got := preferences.FollowedKeywords; len(got) != 1 || got[0] != "OpenAI" {
		t.Fatalf("followed keywords = %#v", got)
	}
	if got := preferences.BlockedKeywords; len(got) != 1 || got[0] != "AI slop" {
		t.Fatalf("blocked keywords = %#v", got)
	}
	if got := preferences.AdditionalKeywords; len(got) != 1 || got[0] != "Claude Code" {
		t.Fatalf("additional keywords = %#v", got)
	}
}

func TestFollowingKeywordRemovesItFromBlockedList(t *testing.T) {
	service := NewService()

	if err := service.BlockKeyword("user-1", "OpenAI"); err != nil {
		t.Fatalf("block keyword: %v", err)
	}
	if err := service.FollowKeyword("user-1", "OpenAI"); err != nil {
		t.Fatalf("follow keyword: %v", err)
	}

	preferences := service.GetUserPreferences("user-1")
	if len(preferences.BlockedKeywords) != 0 {
		t.Fatalf("blocked keywords = %#v, want empty", preferences.BlockedKeywords)
	}
	if got := preferences.FollowedKeywords; len(got) != 1 || got[0] != "OpenAI" {
		t.Fatalf("followed keywords = %#v", got)
	}
}
