package hackernews

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestHackerNewsResourceLimitProfileFreezesEightFiniteDimensions(t *testing.T) {
	profile := DefaultResourceLimitProfile()
	if err := profile.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if profile.Version != ResourceLimitProfileVersion || profile.ConnectTimeout <= 0 || profile.ReadTimeout <= 0 ||
		profile.WallClockTimeout <= 0 || profile.MaxPages != 1 || profile.MaxItems <= 0 ||
		profile.MaxCumulativeResponseBytes <= 0 || profile.MaxRetries <= 0 || profile.DailyRequestQuota <= 0 {
		t.Fatalf("resource limit profile is not finite: %#v", profile)
	}
	if profile.ConnectTimeout >= profile.WallClockTimeout || profile.ReadTimeout >= profile.WallClockTimeout {
		t.Fatalf("timeout hierarchy is invalid: %#v", profile)
	}
}

func TestHackerNewsResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect(t *testing.T) {
	t.Run("cumulative bytes limit-1 limit limit+1", func(t *testing.T) {
		const limit = 16
		for _, size := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(size), func(t *testing.T) {
				var requests atomic.Int32
				payload := []byte(strings.Repeat(" ", size-3) + "100")
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					_, _ = writer.Write(payload)
				}))
				defer server.Close()
				profile := testHNResourceLimitProfile()
				profile.MaxCumulativeResponseBytes = limit
				connector := newTestConnectorWithLimits(t, server, profile, allowingRequestBudget{}, noHNRetryWait)
				result, err := connector.Fetch(context.Background(), testFetchRequest(1, "100"))
				if size <= limit && (err != nil || result.NextCursor != "100") {
					t.Fatalf("size %d result/error = %#v/%v", size, result, err)
				}
				if size > limit && (err == nil || len(result.Snapshots) != 0 || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent) {
					t.Fatalf("oversize result/error = %#v/%v", result, err)
				}
				if requests.Load() != 1 {
					t.Fatalf("requests = %d, want one", requests.Load())
				}
			})
		}
	})

	t.Run("items limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, requested := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(requested), func(t *testing.T) {
				var itemRequests atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/v0/maxitem.json" {
						_, _ = writer.Write([]byte("103"))
						return
					}
					itemRequests.Add(1)
					id := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v0/item/"), ".json")
					_, _ = writer.Write([]byte(`{"id":` + id + `,"type":"story","title":"bounded"}`))
				}))
				defer server.Close()
				profile := testHNResourceLimitProfile()
				profile.MaxItems = limit
				connector := newTestConnectorWithLimits(t, server, profile, allowingRequestBudget{}, noHNRetryWait)
				result, err := connector.Fetch(context.Background(), testFetchRequest(requested, "100"))
				want := min(requested, limit)
				if err != nil || len(result.Items) != want || int(itemRequests.Load()) != want {
					t.Fatalf("requested %d items/requests/error = %d/%d/%v", requested, len(result.Items), itemRequests.Load(), err)
				}
			})
		}
	})

	t.Run("retries limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, failures := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(failures), func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					if int(requests.Add(1)) <= failures {
						writer.WriteHeader(http.StatusBadGateway)
						return
					}
					_, _ = writer.Write([]byte("100"))
				}))
				defer server.Close()
				profile := testHNResourceLimitProfile()
				profile.MaxRetries = limit
				connector := newTestConnectorWithLimits(t, server, profile, allowingRequestBudget{}, noHNRetryWait)
				_, err := connector.Fetch(context.Background(), testFetchRequest(1, "100"))
				if failures <= limit && err != nil {
					t.Fatalf("failures %d error = %v", failures, err)
				}
				if failures > limit && (err == nil || requests.Load() != limit+1) {
					t.Fatalf("overflow retries requests/error = %d/%v", requests.Load(), err)
				}
			})
		}
	})

	t.Run("daily quota stops before dial", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = writer.Write([]byte("100"))
		}))
		defer server.Close()
		profile := testHNResourceLimitProfile()
		profile.DailyRequestQuota = 2
		budget := &boundedHNRequestBudget{limit: 2}
		connector := newTestConnectorWithLimits(t, server, profile, budget, noHNRetryWait)
		for attempt := 0; attempt < 3; attempt++ {
			result, err := connector.Fetch(context.Background(), testFetchRequest(1, "100"))
			if attempt < 2 && err != nil {
				t.Fatalf("allowed attempt %d: %v", attempt+1, err)
			}
			if attempt == 2 && (err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil) {
				t.Fatalf("quota result/error = %#v/%v", result, err)
			}
		}
		if requests.Load() != 2 || budget.callCount() != 3 {
			t.Fatalf("network/budget calls = %d/%d, want 2/3", requests.Load(), budget.callCount())
		}
	})

	t.Run("redirect reserves quota before next dial", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests.Add(1)
			http.Redirect(writer, request, "/v0/redirected.json", http.StatusFound)
		}))
		defer server.Close()
		profile := testHNResourceLimitProfile()
		profile.DailyRequestQuota = 1
		budget := &boundedHNRequestBudget{limit: 1}
		connector := newTestConnectorWithLimits(t, server, profile, budget, noHNRetryWait)
		result, err := connector.Fetch(context.Background(), testFetchRequest(1, "100"))
		if domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil ||
			requests.Load() != 1 || budget.callCount() != 2 {
			t.Fatalf("redirect quota result/error/requests/budget = %#v/%v/%d/%d", result, err, requests.Load(), budget.callCount())
		}
	})
}

func TestHackerNewsResourceLimitsBoundConnectReadAndWallClock(t *testing.T) {
	profile := testHNResourceLimitProfile()
	profile.ConnectTimeout = 10 * time.Millisecond
	profile.ReadTimeout = 20 * time.Millisecond
	profile.WallClockTimeout = 50 * time.Millisecond
	profile.MaxRetries = 0

	t.Run("connect", func(t *testing.T) {
		connector, err := newConnector(testHNConnection(), connectorOptions{
			clientOptions: clientOptions{
				resolver: func(context.Context, string) ([]net.IPAddr, error) {
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
				},
				dialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() },
			},
			resourceLimits: profile, requestBudget: allowingRequestBudget{}, retryWait: noHNRetryWait,
		})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest(1, "100")); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("connect timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("read", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
		defer server.Close()
		connector := newTestConnectorWithLimits(t, server, profile, allowingRequestBudget{}, noHNRetryWait)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest(1, "100")); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("read timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("wall clock", func(t *testing.T) {
		wall := profile
		wall.MaxRetries = 1
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusBadGateway) }))
		defer server.Close()
		waitForDeadline := func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() }
		connector := newTestConnectorWithLimits(t, server, wall, allowingRequestBudget{}, waitForDeadline)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest(1, "100")); err == nil || time.Since(started) > 120*time.Millisecond {
			t.Fatalf("wall timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})
}

func TestHackerNewsResourceLimitProfileRejectsMissingOrUnboundedDimensions(t *testing.T) {
	valid := DefaultResourceLimitProfile()
	for _, test := range []struct {
		name   string
		mutate func(*ResourceLimitProfile)
	}{
		{"version", func(profile *ResourceLimitProfile) { profile.Version = "" }},
		{"connect", func(profile *ResourceLimitProfile) { profile.ConnectTimeout = 0 }},
		{"read", func(profile *ResourceLimitProfile) { profile.ReadTimeout = 0 }},
		{"wall", func(profile *ResourceLimitProfile) { profile.WallClockTimeout = time.Second }},
		{"pages", func(profile *ResourceLimitProfile) { profile.MaxPages = 2 }},
		{"items", func(profile *ResourceLimitProfile) { profile.MaxItems = 0 }},
		{"bytes", func(profile *ResourceLimitProfile) { profile.MaxCumulativeResponseBytes = 0 }},
		{"retries", func(profile *ResourceLimitProfile) { profile.MaxRetries = -1 }},
		{"quota", func(profile *ResourceLimitProfile) { profile.DailyRequestQuota = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := valid
			test.mutate(&profile)
			if err := profile.Validate(); err == nil {
				t.Fatalf("Validate(%s) unexpectedly passed", test.name)
			}
		})
	}
}

type boundedHNRequestBudget struct {
	mu    sync.Mutex
	limit int64
	calls int64
}

func (budget *boundedHNRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.calls++
	used := min(budget.calls, budget.limit)
	allowed := budget.calls <= budget.limit
	if !allowed {
		used = reservation.DailyLimit
	}
	if used < 1 {
		used = 1
	}
	rateUsed := min(used, reservation.PerMinuteLimit)
	return domain.ExternalRequestBudgetDecision{Allowed: allowed, Used: used, RateUsed: rateUsed, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}

func (budget *boundedHNRequestBudget) callCount() int64 {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.calls
}

func testHNResourceLimitProfile() ResourceLimitProfile {
	profile := DefaultResourceLimitProfile()
	profile.ConnectTimeout = time.Second
	profile.ReadTimeout = 2 * time.Second
	profile.WallClockTimeout = 3 * time.Second
	profile.MaxRetries = 0
	return profile
}

func testHNConnection() domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.HackerNewsMode = domain.HackerNewsModeNew
	config.MaxPagesPerRun = 1
	return domain.SourceConnection{ID: 9, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: config, Enabled: true}
}

func newTestConnectorWithLimits(t *testing.T, server *httptest.Server, profile ResourceLimitProfile, budget domain.ExternalRequestBudget, wait func(context.Context, int) error) *Connector {
	t.Helper()
	connector, err := newConnector(testHNConnection(), connectorOptions{
		clientOptions: clientOptions{
			resolver: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			},
			dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			},
			tlsConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local httptest only
			now:       func() time.Time { return time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC) },
		},
		resourceLimits: profile, requestBudget: budget, retryWait: wait,
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func noHNRetryWait(context.Context, int) error { return nil }
