//go:build integration

package application_test

import (
	"context"
	"strings"
	"testing"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	deliverypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/infrastructure/postgres"
	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestSubscriptionServiceRotatesOnlyHashedTokenAndAudits(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, display_name, role) VALUES ('subscription-' || md5(random()::text) || '@example.test', 'hash', 'subscriber', 'viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	tokens := []string{"rss-token-one", "rss-token-two"}
	service, err := deliveryapplication.NewSubscriptionService(deliveryapplication.SubscriptionDependencies{
		Runtime: runtime, Store: deliverypostgres.NewRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime),
		Token: func() (string, error) { value := tokens[0]; tokens = tokens[1:]; return value, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	subject := identitydomain.Subject{UserID: userID, SessionID: 11, Role: identitydomain.RoleViewer}
	created, err := service.Create(ctx, deliveryapplication.CreateSubscriptionInput{Subject: subject, ReportType: "daily", Channel: domain.ChannelRSS, Timezone: "Asia/Shanghai", Schedule: "0 8 * * *", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.RSSToken != "rss-token-one" || created.Subscription.TokenHash != domain.TokenHash(created.RSSToken) {
		t.Fatalf("created secret = %#v", created)
	}
	rotated, err := service.RotateRSSToken(ctx, deliveryapplication.RotateRSSTokenInput{Subject: subject, SubscriptionID: created.Subscription.ID, ExpectedVersion: created.Subscription.Version})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RSSToken != "rss-token-two" || rotated.Subscription.TokenHash != domain.TokenHash(rotated.RSSToken) || rotated.Subscription.TokenHash == created.Subscription.TokenHash {
		t.Fatalf("rotated secret = %#v", rotated)
	}
	stored, err := service.Get(ctx, subject, created.Subscription.ID)
	if err != nil || stored.TokenHash != domain.TokenHash("rss-token-two") || strings.Contains(stored.TokenHash, "rss-token-two") {
		t.Fatalf("stored subscription = %#v/%v", stored, err)
	}
	var auditText string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT coalesce(string_agg(coalesce(before_data::text, '') || coalesce(after_data::text, ''), ''), '') FROM audit_logs WHERE resource_type = 'report_subscription'`).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditText, "rss-token") {
		t.Fatalf("audit leaked RSS token: %s", auditText)
	}
}
