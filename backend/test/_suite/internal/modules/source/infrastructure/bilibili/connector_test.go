package bilibili

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestConnectorVerifiesAccountBeforeCollectingOfficialVideoAndArticle(t *testing.T) {
	now := time.Date(2026, time.August, 7, 4, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("access-token") != "fixture-token" || request.Header.Get("Authorization") == "" || request.Header.Get("x-bili-signature-version") != "2.0" {
			t.Errorf("signed headers = %#v", request.Header)
		}
		switch calls.Add(1) {
		case 1:
			if request.URL.Path != "/arcopen/fn/user/account/scopes" {
				t.Errorf("path = %s", request.URL.Path)
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"openid":"creator_open_id","scopes":["USER_INFO","ARC_BASE","ATC_BASE","ATC_DATA"]}}`))
		case 2:
			if request.URL.Path != "/arcopen/fn/archive/viewlist" || request.URL.Query().Get("ps") != "50" {
				t.Errorf("video request = %s?%s", request.URL.Path, request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"list":[{"resource_id":"BV1fixture","title":"官方视频","desc":"视频摘要","cover":"https://i0.hdslb.com/fixture.jpg","ptime":1786074900,"video_info":{"share_url":"https://www.bilibili.com/video/BV1fixture"}}],"page":{"total":1}}}`))
		case 3:
			if request.URL.Path != "/arcopen/fn/article/list" {
				t.Errorf("path = %s", request.URL.Path)
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"list":[{"id":123,"title":"官方专栏","summary":"专栏摘要","banner_url":"https://i0.hdslb.com/article.jpg","publish_time":1786074800,"stats":{"view":8,"like":3,"reply":2,"share":1}}],"page":{"total":1}}}`))
		default:
			t.Fatalf("unexpected request %d", calls.Load())
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	tlsConfig := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsConfig.InsecureSkipVerify = true // fixture server certificate cannot contain the fixed official host
	connection := bilibiliConnection()
	connector, err := newConnector(connection, connectorOptions{
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, parsed.Host)
		},
		tlsConfig: tlsConfig, now: func() time.Time { return now }, nonce: func() string { return "fixture-nonce" },
		lookupEnv: func(name string) (string, bool) {
			value, _ := json.Marshal(credentials{ClientID: "fixture-client", AppSecret: "fixture-secret", AccessToken: "fixture-token"})
			return string(value), name == "BILIBILI_OAUTH"
		},
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	result, err := connector.Fetch(context.Background(), bilibiliFetchRequest())
	if err != nil || len(result.Items) != 2 || result.HasMore || result.NextCursor == "" {
		t.Fatalf("Fetch() = %#v, %v", result, err)
	}
	if result.Items[0].ExternalID != "video:BV1fixture" || result.Items[0].URL != "https://www.bilibili.com/video/BV1fixture" {
		t.Errorf("video = %#v", result.Items[0])
	}
	if result.Items[1].ExternalID != "article:123" || result.Items[1].Metrics.ViewCount == nil || *result.Items[1].Metrics.ViewCount != 8 {
		t.Errorf("article = %#v", result.Items[1])
	}
	if len(result.Snapshots) != 2 || !result.Snapshots[0].VerifyPayload() || !result.Snapshots[1].VerifyPayload() ||
		result.Items[0].EvidenceReferences[0].LocatorValue != "/data/list/0" || result.Items[1].EvidenceReferences[0].LocatorValue != "/data/list/0" ||
		result.Items[0].EvidenceReferences[0].SnapshotKey == result.Items[1].EvidenceReferences[0].SnapshotKey {
		t.Fatalf("Bilibili raw evidence = %#v / %#v", result.Snapshots, result.Items)
	}
	for _, item := range result.Items {
		if len(item.Parties) != 3 || item.Parties[0].Role != domain.SourcePartyRoleContentOrigin || item.Parties[1].Role != domain.SourcePartyRoleDistributor || item.Parties[2].Role != domain.SourcePartyRoleAuthor {
			t.Fatalf("Bilibili explicit parties = %#v", item.Parties)
		}
	}
}

func TestConnectorStopsBeforeContentWhenAuthorizedAccountDoesNotMatch(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"code":0,"data":{"openid":"another_account","scopes":["USER_INFO","ARC_BASE"]}}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	connector, err := newConnector(bilibiliConnection(), connectorOptions{
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, parsed.Host)
		},
		tlsConfig: fixtureTLSConfig(server), nonce: func() string { return "nonce" },
		lookupEnv: func(string) (string, bool) {
			return `{"client_id":"id","app_secret":"secret","access_token":"token"}`, true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Fetch(context.Background(), bilibiliFetchRequest())
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorAuthentication {
		t.Fatalf("Fetch() error = %v", err)
	}
}

func bilibiliConnection() domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.RequiresAttribution = true
	config.RequiresDeletionSync = true
	config.BilibiliOpenID = "creator_open_id"
	return domain.SourceConnection{ID: 13, SourceType: domain.SourceTypeBilibili, Name: "Bilibili", Endpoint: domain.BilibiliOpenEndpoint, AuthType: domain.AuthTypeOAuth2, CredentialRef: "env:BILIBILI_OAUTH", Config: config, Enabled: true, TermsPolicyURL: "https://openhome.bilibili.com/agreement/privacy-policy"}
}

func bilibiliFetchRequest() domain.FetchRequest {
	return domain.FetchRequest{CollectionRunID: 1, SourceConnectionID: 13, QuerySignature: strings.Repeat("a", 64), Query: "ignored", WindowStart: time.Now().Add(-time.Hour), WindowEnd: time.Now(), Limit: 100}
}

func fixtureTLSConfig(server *httptest.Server) *tls.Config {
	config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	config.InsecureSkipVerify = true
	return config
}

var _ = tls.VersionTLS12
