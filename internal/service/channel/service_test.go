package channel_test

import (
	"context"
	"errors"
	"testing"

	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
)

func TestServiceSubscriptionsKeywordsAndPreferences(t *testing.T) {
	ctx := context.Background()
	svc := servicechannel.NewService(servicechannel.NewMemoryRepository())

	defaults, err := svc.ListChannels(ctx, servicechannel.ListChannelsInput{ActiveOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults) != 4 {
		t.Fatalf("expected 4 seeded AI channels, got %d", len(defaults))
	}

	sub, err := svc.Subscribe(ctx, servicechannel.UserChannelInput{
		UserID:    "usr_1",
		ChannelID: defaults[0].ID,
	})
	if err != nil {
		t.Fatalf("subscribe active channel: %v", err)
	}
	if sub.UserID != "usr_1" || sub.Channel.ID != defaults[0].ID || sub.Channel.Status != servicechannel.ChannelStatusActive {
		t.Fatalf("unexpected subscription: %#v", sub)
	}

	if _, err := svc.UpdateChannelStatus(ctx, servicechannel.UpdateChannelStatusInput{
		ChannelID: defaults[0].ID,
		Status:    servicechannel.ChannelStatusDisabled,
	}); err != nil {
		t.Fatalf("disable channel: %v", err)
	}
	if _, err := svc.Subscribe(ctx, servicechannel.UserChannelInput{UserID: "usr_2", ChannelID: defaults[0].ID}); !errors.Is(err, servicechannel.ErrChannelDisabled) {
		t.Fatalf("expected disabled channel subscription error, got %v", err)
	}

	keyword, err := svc.CreateKeyword(ctx, servicechannel.KeywordInput{UserID: "usr_1", Keyword: "Claude Code"})
	if err != nil {
		t.Fatalf("create keyword: %v", err)
	}
	updated, err := svc.UpdateKeyword(ctx, servicechannel.UpdateKeywordInput{
		UserID:    "usr_1",
		KeywordID: keyword.ID,
		Keyword:   "OpenAI Agents",
		Enabled:   boolPtr(false),
	})
	if err != nil {
		t.Fatalf("update own keyword: %v", err)
	}
	if updated.Keyword != "OpenAI Agents" || updated.Enabled {
		t.Fatalf("unexpected updated keyword: %#v", updated)
	}
	if _, err := svc.UpdateKeyword(ctx, servicechannel.UpdateKeywordInput{UserID: "usr_2", KeywordID: keyword.ID, Keyword: "stolen"}); !errors.Is(err, servicechannel.ErrNotFound) {
		t.Fatalf("expected cross-user keyword update to be hidden as not found, got %v", err)
	}

	if err := svc.SetUserDailySendAt(ctx, servicechannel.UserDailySendAtInput{UserID: "usr_1", DailySendAt: "07:45"}); err != nil {
		t.Fatalf("set user send time: %v", err)
	}
	sendAt, err := svc.UserDailySendAt(ctx, "usr_1")
	if err != nil {
		t.Fatalf("read user send time: %v", err)
	}
	if sendAt != "07:45" {
		t.Fatalf("expected user override 07:45, got %q", sendAt)
	}

	if err := svc.SetDefaultDailySendAt(ctx, "09:15"); err != nil {
		t.Fatalf("set default send time: %v", err)
	}
	if got, err := svc.DefaultDailySendAt(ctx); err != nil || got != "09:15" {
		t.Fatalf("expected default send time 09:15, got %q err %v", got, err)
	}

	custom, err := svc.CreateChannel(ctx, servicechannel.CreateChannelInput{
		Name:        "AI Agents",
		Slug:        "ai-agents",
		Description: "agent tooling",
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := svc.DeleteChannel(ctx, custom.ID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if _, err := svc.UpdateChannelStatus(ctx, servicechannel.UpdateChannelStatusInput{ChannelID: custom.ID, Status: servicechannel.ChannelStatusActive}); !errors.Is(err, servicechannel.ErrNotFound) {
		t.Fatalf("expected deleted channel to be not found, got %v", err)
	}
	channels, err := svc.ListChannels(ctx, servicechannel.ListChannelsInput{})
	if err != nil {
		t.Fatalf("list channels after delete: %v", err)
	}
	for _, channel := range channels {
		if channel.ID == "" || channel.ID == custom.ID {
			t.Fatalf("expected deleted channel to be absent without zero-value rows, got %#v", channels)
		}
	}
	if _, err := svc.CreateChannel(ctx, servicechannel.CreateChannelInput{
		Name: "Duplicate slug",
		Slug: defaults[1].Slug,
	}); !errors.Is(err, servicechannel.ErrAlreadyExists) {
		t.Fatalf("expected duplicate slug to return already exists, got %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
