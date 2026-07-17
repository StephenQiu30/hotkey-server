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
