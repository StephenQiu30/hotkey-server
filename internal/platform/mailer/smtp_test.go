package mailer

import (
	"strings"
	"testing"

	servicemail "github.com/StephenQiu30/hotkey-server/internal/service/mail"
)

func TestBuildMIMERejectsHeaderNewlines(t *testing.T) {
	_, err := buildMIME(servicemail.Message{
		From:     "daily@example.com\r\nBcc: attacker@example.com",
		To:       "reader@example.com",
		Subject:  "日报",
		TextBody: "正文",
		HTMLBody: "<p>正文</p>",
	})
	if err == nil {
		t.Fatal("expected header newline to be rejected")
	}
	if !strings.Contains(err.Error(), "from") {
		t.Fatalf("expected error to identify header, got %v", err)
	}
}
