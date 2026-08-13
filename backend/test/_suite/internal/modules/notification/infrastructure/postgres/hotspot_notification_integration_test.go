//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestContentBindingProjectsOneMonitorScopedHotspotNotificationAndHighEmail(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	var userID, sourceID, monitorID, configID, monitorSourceID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','热点用户','editor') RETURNING id`, fmt.Sprintf("hotspot-notification-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections(source_type,name,endpoint,enabled)
VALUES ('rss','热点 Feed','https://example.test/feed',true) RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors(name,status,created_by,updated_by)
VALUES ('热点监控','draft',$1,$1) RETURNING id`, userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions(
monitor_id,revision,state,languages,alert_email_enabled,alert_email_min_severity,created_by,updated_by)
VALUES ($1,1,'draft',ARRAY['zh'],true,'warning',$2,$2) RETURNING id`, monitorID, userID).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_sources(config_version_id,source_connection_id,query_signature)
VALUES ($1,$2,repeat('a',64)) RETURNING id`, configID, sourceID).Scan(&monitorSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions
SET state='published',config_hash=repeat('b',64),published_at=$1 WHERE id=$2`, now, configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='active',published_config_version_id=$1 WHERE id=$2`, configID, monitorID); err != nil {
		t.Fatal(err)
	}

	var runID, targetID, itemID, contentID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO collection_runs(
source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at,status)
VALUES ($1,repeat('a',64),$2::timestamptz,$2::timestamptz+interval '1 minute','manual',$2::timestamptz,'succeeded') RETURNING id`, sourceID, now).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO collection_run_targets(
collection_run_id,monitor_source_id,monitor_config_version_id,target_status)
VALUES ($1,$2,$3,'succeeded') RETURNING id`, runID, monitorSourceID, configID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO collection_run_items(
run_id,source_connection_id,source_code,external_id,content_type,captured_item_version,captured_item,
payload_hash,raw_payload_disposition,outcome,observed_at)
VALUES ($1,$2,'rss','hotspot-1','article','v2','{"title":"AI 新品发布"}',repeat('c',64),'discarded','captured',$3)
RETURNING id`, runID, sourceID, now).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO collection_run_target_items(
collection_run_id,collection_run_target_id,collection_run_item_id,outcome)
VALUES ($1,$2,$3,'captured')`, runID, targetID, itemID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO contents(
source_connection_id,external_id,content_type,title,excerpt,canonical_url,language,published_at,fetched_at,dedupe_key,like_count)
VALUES ($1,'hotspot-1','article','AI 新品发布','官方刚刚发布了新的 AI 产品。','https://example.test/hotspot-1','zh',$2,$2,repeat('d',64),200)
RETURNING id`, sourceID, now).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE collection_run_items
SET content_id=$1,ingestion_status='succeeded' WHERE id=$2`, contentID, itemID); err != nil {
		t.Fatal(err)
	}
	// A replayed update must not create a second fact.
	if _, err := runtime.SQL.Exec(`UPDATE collection_run_items SET content_id=content_id WHERE id=$1`, itemID); err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(runtime)
	page, err := repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: userID, Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("notification page = %#v / %v", page, err)
	}
	notification := page.Items[0]
	if notification.MonitorID != monitorID || notification.EventType != "hotspot.discovered" ||
		notification.ResourceType != "hotspot" || notification.ResourceID != contentID ||
		notification.ResourceStatus != "high" || notification.DeepLink != fmt.Sprintf("/dashboard/contents/%d", contentID) {
		t.Fatalf("hotspot notification = %#v", notification)
	}
	claimed, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("e", 64), ClaimedAt: now.Add(time.Second), LeaseUntil: now.Add(time.Minute),
	})
	if err != nil || !claimed.Claimed || claimed.Notification.ID != notification.ID {
		t.Fatalf("high hotspot email claim = %#v / %v", claimed, err)
	}
}
