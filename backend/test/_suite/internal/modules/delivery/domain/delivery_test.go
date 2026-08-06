package domain

import "testing"

func TestTokenHashAndRetryClassification(t *testing.T) {
	if TokenHash("token") == TokenHash("other") {
		t.Fatal("token hash collision")
	}
	if !RetryableSMTP(421) || RetryableSMTP(550) {
		t.Fatal("SMTP retry classification is wrong")
	}
}

func TestRSSSubscriptionRejectsPlaintextToken(t *testing.T) {
	subscription := Subscription{ID: 1, Version: 1, UserID: 1, ReportType: "daily", Channel: ChannelRSS, TokenHash: "plaintext-token", Timezone: "UTC", Schedule: "0 8 * * *", Enabled: true}
	if err := subscription.Validate(); err == nil {
		t.Fatal("RSS subscription accepted plaintext token")
	}
}
