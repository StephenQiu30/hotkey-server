package x

import (
	"context"
	"crypto/tls"
	"errors"
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

func TestXResourceLimitProfileFreezesEightFiniteDimensions(t *testing.T) {
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

func TestXResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect(t *testing.T) {
	t.Run("cumulative bytes limit-1 limit limit+1", func(t *testing.T) {
		const limit = 32
		for _, size := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(size), func(t *testing.T) {
				var requests atomic.Int32
				payload := []byte(`{"meta":{"result_count":0}}` + strings.Repeat(" ", size-len(`{"meta":{"result_count":0}}`)))
				server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					_, _ = writer.Write(payload)
				}))
				defer server.Close()
				profile := testXResourceLimitProfile()
				profile.MaxCumulativeResponseBytes = limit
				connector := newXConnectorWithLimits(t, server, profile, allowingRequestBudget{}, tokenLookup, noXRetryWait)
				result, err := connector.Fetch(context.Background(), xFetchRequest())
				if size <= limit && (err != nil || len(result.Snapshots) != 1) {
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
		const limit = 10
		for _, count := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(count), func(t *testing.T) {
				server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Query().Get("max_results") != "10" {
						t.Errorf("max_results = %q, want profile cap 10", request.URL.Query().Get("max_results"))
					}
					_, _ = writer.Write([]byte(xPostsPayload(count)))
				}))
				defer server.Close()
				profile := testXResourceLimitProfile()
				profile.MaxItems = limit
				connector := newXConnectorWithLimits(t, server, profile, allowingRequestBudget{}, tokenLookup, noXRetryWait)
				result, err := connector.Fetch(context.Background(), xFetchRequest())
				if count <= limit && (err != nil || len(result.Items) != count || len(result.Snapshots) != 1) {
					t.Fatalf("count %d result/error = %#v/%v", count, result, err)
				}
				if count > limit && (err == nil || len(result.Items) != 0 || len(result.Snapshots) != 0) {
					t.Fatalf("overflow count %d result/error = %#v/%v", count, result, err)
				}
			})
		}
	})

	t.Run("one page exposes cursor without next network request", func(t *testing.T) {
		var requests atomic.Int32
		server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{"meta":{"newest_id":"10","next_token":"next","result_count":0}}`))
		}))
		defer server.Close()
		connector := newXConnectorWithLimits(t, server, testXResourceLimitProfile(), allowingRequestBudget{}, tokenLookup, noXRetryWait)
		result, err := connector.Fetch(context.Background(), xFetchRequest())
		if err != nil || !result.HasMore || result.NextCursor == "" || requests.Load() != 1 {
			t.Fatalf("page result/error/requests = %#v/%v/%d", result, err, requests.Load())
		}
	})

	t.Run("retries limit-1 limit limit+1", func(t *testing.T) {
		const limit = 2
		for _, failures := range []int{limit - 1, limit, limit + 1} {
			t.Run(fmt.Sprint(failures), func(t *testing.T) {
				var requests atomic.Int32
				server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					if int(requests.Add(1)) <= failures {
						writer.WriteHeader(http.StatusBadGateway)
						return
					}
					_, _ = writer.Write([]byte(`{"meta":{"result_count":0}}`))
				}))
				defer server.Close()
				profile := testXResourceLimitProfile()
				profile.MaxRetries = limit
				connector := newXConnectorWithLimits(t, server, profile, allowingRequestBudget{}, tokenLookup, noXRetryWait)
				_, err := connector.Fetch(context.Background(), xFetchRequest())
				if failures <= limit && err != nil {
					t.Fatalf("failures %d error = %v", failures, err)
				}
				if failures > limit && (err == nil || requests.Load() != limit+1) {
					t.Fatalf("overflow retries requests/error = %d/%v", requests.Load(), err)
				}
			})
		}
	})

	t.Run("daily quota stops before dial and covers lookup", func(t *testing.T) {
		var requests atomic.Int32
		server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{"meta":{"result_count":0}}`))
		}))
		defer server.Close()
		profile := testXResourceLimitProfile()
		profile.DailyRequestQuota = 1
		budget := &boundedXRequestBudget{limit: 1}
		connector := newXConnectorWithLimits(t, server, profile, budget, tokenLookup, noXRetryWait)
		if _, err := connector.Fetch(context.Background(), xFetchRequest()); err != nil {
			t.Fatal(err)
		}
		result, err := connector.LookupPostMetrics(context.Background(), domain.XPostMetricLookupRequest{SourceConnectionID: 10, PostIDs: []string{"10"}})
		if domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil ||
			requests.Load() != 1 || budget.callCount() != 2 {
			t.Fatalf("lookup quota result/error/requests/budget = %#v/%v/%d/%d", result, err, requests.Load(), budget.callCount())
		}
	})

	t.Run("redirect reserves quota before next dial", func(t *testing.T) {
		var requests atomic.Int32
		server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests.Add(1)
			http.Redirect(writer, request, "/2/tweets/search/recent?redirected=1", http.StatusFound)
		}))
		defer server.Close()
		profile := testXResourceLimitProfile()
		profile.DailyRequestQuota = 1
		budget := &boundedXRequestBudget{limit: 1}
		connector := newXConnectorWithLimits(t, server, profile, budget, tokenLookup, noXRetryWait)
		result, err := connector.Fetch(context.Background(), xFetchRequest())
		if domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil ||
			requests.Load() != 1 || budget.callCount() != 2 {
			t.Fatalf("redirect quota result/error/requests/budget = %#v/%v/%d/%d", result, err, requests.Load(), budget.callCount())
		}
	})
}

func TestXResourceLimitOrderingValidatesInputAndCredentialBeforeBudgetOrNetwork(t *testing.T) {
	var requests, credentialReads atomic.Int32
	server := newXTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	budget := &boundedXRequestBudget{limit: 0}
	lookup := func(string) (string, bool) { credentialReads.Add(1); return "fixture-secret", true }
	connector := newXConnectorWithLimits(t, server, testXResourceLimitProfile(), budget, lookup, noXRetryWait)

	invalid := xFetchRequest()
	invalid.Query = ""
	if _, err := connector.Fetch(context.Background(), invalid); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || credentialReads.Load() != 0 || budget.callCount() != 0 || requests.Load() != 0 {
		t.Fatalf("invalid input ordering error/credential/budget/network = %v/%d/%d/%d", err, credentialReads.Load(), budget.callCount(), requests.Load())
	}
	missingBudget := &boundedXRequestBudget{limit: 1}
	missing := newXConnectorWithLimits(t, server, testXResourceLimitProfile(), missingBudget, func(string) (string, bool) { return "", false }, noXRetryWait)
	if _, err := missing.Fetch(context.Background(), xFetchRequest()); domain.ClassifyCollectionError(err) != domain.CollectionErrorAuthentication || missingBudget.callCount() != 0 || requests.Load() != 0 {
		t.Fatalf("missing credential ordering error/budget/network = %v/%d/%d", err, missingBudget.callCount(), requests.Load())
	}
	result, err := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil || credentialReads.Load() != 1 || budget.callCount() != 1 || requests.Load() != 0 {
		t.Fatalf("quota ordering result/error/credential/budget/network = %#v/%v/%d/%d/%d", result, err, credentialReads.Load(), budget.callCount(), requests.Load())
	}
}

func TestXEndpointPolicyRejectsUnsafeResolutionBeforeCredentialBudgetOrDial(t *testing.T) {
	var credentialReads, dialCalls atomic.Int32
	budget := &boundedXRequestBudget{limit: 1}
	connector, err := newConnector(xConnection(), connectorOptions{
		lookupEnv: func(string) (string, bool) {
			credentialReads.Add(1)
			return "fixture-secret", true
		},
		requestBudget:  budget,
		resourceLimits: testXResourceLimitProfile(),
		retryWait:      noXRetryWait,
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalls.Add(1)
			return nil, errors.New("unexpected dial")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.Fetch(context.Background(), xFetchRequest()); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("unsafe endpoint error = %v", err)
	}
	if credentialReads.Load() != 0 || budget.callCount() != 0 || dialCalls.Load() != 0 {
		t.Fatalf("unsafe endpoint credential/budget/dial = %d/%d/%d, want 0/0/0", credentialReads.Load(), budget.callCount(), dialCalls.Load())
	}
}

func TestXResourceLimitsBoundConnectReadAndWallClock(t *testing.T) {
	profile := testXResourceLimitProfile()
	profile.ConnectTimeout = 10 * time.Millisecond
	profile.ReadTimeout = 20 * time.Millisecond
	profile.WallClockTimeout = 50 * time.Millisecond
	profile.MaxRetries = 0

	t.Run("connect", func(t *testing.T) {
		connector, err := newConnector(xConnection(), connectorOptions{
			lookupEnv: tokenLookup, requestBudget: allowingRequestBudget{}, resourceLimits: profile, retryWait: noXRetryWait,
			resolver: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			},
			dialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() },
		})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), xFetchRequest()); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("connect timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("read", func(t *testing.T) {
		server := newXTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
		defer server.Close()
		connector := newXConnectorWithLimits(t, server, profile, allowingRequestBudget{}, tokenLookup, noXRetryWait)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), xFetchRequest()); err == nil || time.Since(started) > 100*time.Millisecond {
			t.Fatalf("read timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})

	t.Run("wall clock", func(t *testing.T) {
		wall := profile
		wall.MaxRetries = 1
		server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusBadGateway) }))
		defer server.Close()
		waitForDeadline := func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() }
		connector := newXConnectorWithLimits(t, server, wall, allowingRequestBudget{}, tokenLookup, waitForDeadline)
		started := time.Now()
		if _, err := connector.Fetch(context.Background(), xFetchRequest()); err == nil || time.Since(started) > 120*time.Millisecond {
			t.Fatalf("wall timeout error/elapsed = %v/%s", err, time.Since(started))
		}
	})
}

func TestXResourceLimitProfileRejectsMissingOrUnboundedDimensions(t *testing.T) {
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
		{"items", func(profile *ResourceLimitProfile) { profile.MaxItems = 9 }},
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

type boundedXRequestBudget struct {
	mu    sync.Mutex
	limit int64
	calls int64
}

func (budget *boundedXRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.calls++
	used := min(budget.calls, budget.limit)
	return domain.ExternalRequestBudgetDecision{Allowed: budget.calls <= budget.limit, Used: used, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}

func (budget *boundedXRequestBudget) callCount() int64 {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.calls
}

func testXResourceLimitProfile() ResourceLimitProfile {
	profile := DefaultResourceLimitProfile()
	profile.ConnectTimeout = time.Second
	profile.ReadTimeout = 2 * time.Second
	profile.WallClockTimeout = 3 * time.Second
	profile.MaxRetries = 0
	return profile
}

func newXConnectorWithLimits(t *testing.T, server *httptest.Server, profile ResourceLimitProfile, budget domain.ExternalRequestBudget, lookup func(string) (string, bool), wait func(context.Context, int) error) *Connector {
	t.Helper()
	connector, err := newConnector(xConnection(), connectorOptions{
		lookupEnv: lookup, requestBudget: budget, resourceLimits: profile, retryWait: wait,
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		tlsConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local httptest only
		now:       func() time.Time { return time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func xPostsPayload(count int) string {
	posts := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		posts = append(posts, fmt.Sprintf(`{"id":"%d","text":"post %d"}`, index, index))
	}
	return `{"data":[` + strings.Join(posts, ",") + `],"meta":{"newest_id":"` + fmt.Sprint(count) + `","result_count":` + fmt.Sprint(count) + `}}`
}

func noXRetryWait(context.Context, int) error { return nil }
