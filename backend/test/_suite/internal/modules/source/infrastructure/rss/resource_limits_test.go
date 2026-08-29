package rss

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestRSSResourceLimitProfileFreezesEightFiniteDimensions(t *testing.T) {
	profile := DefaultResourceLimitProfile()
	if err := profile.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if profile.Version != ResourceLimitProfileVersion ||
		profile.ConnectTimeout <= 0 || profile.ReadTimeout <= 0 || profile.WallClockTimeout <= 0 ||
		profile.MaxPages <= 0 || profile.MaxItems <= 0 || profile.MaxCumulativeResponseBytes <= 0 ||
		profile.MaxRetries <= 0 || profile.DailyRequestQuota <= 0 {
		t.Fatalf("resource limit profile is not finite: %#v", profile)
	}
	if profile.ConnectTimeout >= profile.WallClockTimeout || profile.ReadTimeout >= profile.WallClockTimeout {
		t.Fatalf("timeout hierarchy is invalid: %#v", profile)
	}
}

func TestRSSResourceLimitsStopBeforeTheNextExternalOrEvidenceSideEffect(t *testing.T) {
	t.Run("cumulative bytes limit-1 limit limit+1", func(t *testing.T) {
		const limit = 256
		for _, size := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(size), func(t *testing.T) {
				payload := exactEmptyRSSPayload(t, size)
				requests := 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					requests++
					_, _ = writer.Write(payload)
				}))
				defer server.Close()
				profile := testResourceLimitProfile()
				profile.MaxCumulativeResponseBytes = limit
				connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), profile, allowingRequestBudget{}, noRetryWait)
				result, err := connector.Fetch(context.Background(), testFetchRequest())
				if size <= limit && (err != nil || len(result.Snapshots) != 1) {
					t.Fatalf("size %d result/error = %#v/%v", size, result, err)
				}
				if size > limit && (err == nil || len(result.Snapshots) != 0 || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent) {
					t.Fatalf("oversize result/error = %#v/%v", result, err)
				}
				if requests != 1 {
					t.Fatalf("requests = %d, want one", requests)
				}
			})
		}
	})

	t.Run("items limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, items := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(items), func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					_, _ = writer.Write([]byte(rssItems(items)))
				}))
				defer server.Close()
				profile := testResourceLimitProfile()
				profile.MaxItems = limit
				connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), profile, allowingRequestBudget{}, noRetryWait)
				result, err := connector.Fetch(context.Background(), testFetchRequest())
				if items <= limit && (err != nil || len(result.Items) != items || len(result.Snapshots) != 1) {
					t.Fatalf("items %d result/error = %#v/%v", items, result, err)
				}
				if items > limit && (err == nil || len(result.Snapshots) != 0) {
					t.Fatalf("overflow result/error = %#v/%v", result, err)
				}
			})
		}
	})

	t.Run("pages limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, available := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(available), func(t *testing.T) {
				requests := 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					requests++
					page := requests
					if page < available {
						writer.Header().Set("Link", fmt.Sprintf(`<https://feeds.example.test/page-%d>; rel="next"`, page+1))
					}
					_, _ = writer.Write([]byte(rssItems(1)))
				}))
				defer server.Close()
				profile := testResourceLimitProfile()
				profile.MaxPages = limit
				connector := newTestConnectorWithLimits(t, server, 20, publicResolver(), profile, allowingRequestBudget{}, noRetryWait)
				result, err := connector.Fetch(context.Background(), testFetchRequest())
				if err != nil || requests != min(available, limit) {
					t.Fatalf("available %d requests/result/error = %d/%#v/%v", available, requests, result, err)
				}
				if (available > limit) != result.HasMore {
					t.Fatalf("available %d HasMore = %t", available, result.HasMore)
				}
			})
		}
	})

	t.Run("retries limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, failures := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(failures), func(t *testing.T) {
				requests := 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					requests++
					if requests <= failures {
						writer.WriteHeader(http.StatusBadGateway)
						return
					}
					_, _ = writer.Write([]byte(rssItems(1)))
				}))
				defer server.Close()
				profile := testResourceLimitProfile()
				profile.MaxRetries = limit
				connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), profile, allowingRequestBudget{}, noRetryWait)
				_, err := connector.Fetch(context.Background(), testFetchRequest())
				if failures <= limit && err != nil {
					t.Fatalf("failures %d error = %v", failures, err)
				}
				if failures > limit && (err == nil || requests != limit+1) {
					t.Fatalf("overflow retries requests/error = %d/%v", requests, err)
				}
			})
		}
	})

	t.Run("daily quota stops before dial", func(t *testing.T) {
		requests := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = writer.Write([]byte(rssItems(1)))
		}))
		defer server.Close()
		profile := testResourceLimitProfile()
		profile.DailyRequestQuota = 2
		budget := &boundedRequestBudget{limit: 2}
		connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), profile, budget, noRetryWait)
		for attempt := 0; attempt < 3; attempt++ {
			result, err := connector.Fetch(context.Background(), testFetchRequest())
			if attempt < 2 && err != nil {
				t.Fatalf("allowed attempt %d: %v", attempt+1, err)
			}
			if attempt == 2 && (err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil) {
				t.Fatalf("quota result/error = %#v/%v", result, err)
			}
		}
		if requests != 2 || budget.calls != 3 {
			t.Fatalf("network/budget calls = %d/%d, want 2/3", requests, budget.calls)
		}
	})
}

func TestRSSResourceLimitsBoundConnectReadAndWallClock(t *testing.T) {
	profile := testResourceLimitProfile()
	profile.ConnectTimeout = 10 * time.Millisecond
	profile.ReadTimeout = 20 * time.Millisecond
	profile.WallClockTimeout = 50 * time.Millisecond
	profile.MaxRetries = 0

	t.Run("connect", func(t *testing.T) {
		connector, err := newConnector(testRSSConnection(1), connectorOptions{
			resolver: publicResolver(), requestBudget: allowingRequestBudget{}, resourceLimits: profile,
			dialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("connect timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("read", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), profile, allowingRequestBudget{}, noRetryWait)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("read timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("wall clock", func(t *testing.T) {
		wallProfile := profile
		wallProfile.MaxRetries = 1
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()
		waitForDeadline := func(ctx context.Context, _ int) error {
			<-ctx.Done()
			return ctx.Err()
		}
		connector := newTestConnectorWithLimits(t, server, 1, publicResolver(), wallProfile, allowingRequestBudget{}, waitForDeadline)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || time.Since(started) > 120*time.Millisecond {
			t.Fatalf("wall timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})
}

type boundedRequestBudget struct {
	limit int64
	calls int64
}

func (budget *boundedRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	budget.calls++
	used := budget.calls
	allowed := used <= budget.limit
	if !allowed {
		used = reservation.DailyLimit
	}
	if used < 1 {
		used = 1
	}
	rateUsed := min(used, reservation.PerMinuteLimit)
	return domain.ExternalRequestBudgetDecision{Allowed: allowed, Used: used, RateUsed: rateUsed, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}

func testResourceLimitProfile() ResourceLimitProfile {
	profile := DefaultResourceLimitProfile()
	profile.ConnectTimeout = time.Second
	profile.ReadTimeout = 2 * time.Second
	profile.WallClockTimeout = 3 * time.Second
	profile.MaxRetries = 0
	return profile
}

func testRSSConnection(maxPages int) domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.MaxPagesPerRun = maxPages
	return domain.SourceConnection{
		ID: 7, SourceType: domain.SourceTypeRSS, Name: "Fixture RSS", Endpoint: "https://feeds.example.test/rss",
		AuthType: domain.AuthTypeNone, Config: config, Enabled: true,
	}
}

func noRetryWait(context.Context, int) error { return nil }

func exactEmptyRSSPayload(t *testing.T, size int) []byte {
	t.Helper()
	prefix, suffix := `<rss><channel><!--`, `--></channel></rss>`
	if size < len(prefix)+len(suffix) {
		t.Fatalf("fixture size %d is too small", size)
	}
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}

func rssItems(count int) string {
	var builder strings.Builder
	builder.WriteString(`<rss><channel>`)
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, `<item><guid>item-%d</guid><title>Item %d</title></item>`, index, index)
	}
	builder.WriteString(`</channel></rss>`)
	return builder.String()
}

func TestRSSResourceLimitProfileRejectsMissingOrUnboundedDimensions(t *testing.T) {
	valid := DefaultResourceLimitProfile()
	tests := []struct {
		name   string
		mutate func(*ResourceLimitProfile)
	}{
		{"version", func(profile *ResourceLimitProfile) { profile.Version = "" }},
		{"connect", func(profile *ResourceLimitProfile) { profile.ConnectTimeout = 0 }},
		{"read", func(profile *ResourceLimitProfile) { profile.ReadTimeout = 0 }},
		{"wall", func(profile *ResourceLimitProfile) { profile.WallClockTimeout = 0 }},
		{"pages", func(profile *ResourceLimitProfile) { profile.MaxPages = 0 }},
		{"items", func(profile *ResourceLimitProfile) { profile.MaxItems = 0 }},
		{"bytes", func(profile *ResourceLimitProfile) { profile.MaxCumulativeResponseBytes = 0 }},
		{"retries", func(profile *ResourceLimitProfile) { profile.MaxRetries = -1 }},
		{"quota", func(profile *ResourceLimitProfile) { profile.DailyRequestQuota = 0 }},
		{"timeout hierarchy", func(profile *ResourceLimitProfile) { profile.WallClockTimeout = time.Second }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid profile accepted: %#v", candidate)
			}
		})
	}
}
