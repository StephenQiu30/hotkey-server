package application

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestQueryPlannerPlanPrefersPublishedOverrideWithoutRehashing(t *testing.T) {
	t.Parallel()

	planner := QueryPlanner{}
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	target := plannerTarget(20, 200, 2, strings.Repeat("a", 64))
	target.QueryOverride = "climate OR energy"
	target.Languages = []string{"en", "zh-CN"}
	target.Regions = []string{"US"}
	target.Terms = []domain.CollectionTerm{{Value: "ignored when override exists"}}

	request, err := planner.Plan(target, windowStart, windowStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if request.Query != "climate OR energy" || request.QuerySignature != target.QuerySignature {
		t.Fatalf("planned query/signature = %#v, want published override with unchanged signature", request)
	}
	if !reflect.DeepEqual(request.Languages, target.Languages) || !reflect.DeepEqual(request.Regions, target.Regions) {
		t.Fatalf("planned locales = %#v, want published target locales", request)
	}
	if len(request.Targets) != 1 || request.Targets[0].MonitorSourceID != target.MonitorSourceID {
		t.Fatalf("planned targets = %#v", request.Targets)
	}
}

func TestQueryPlannerPlanBuildsStableTermsAndRejectsNonPublishedTarget(t *testing.T) {
	t.Parallel()

	planner := QueryPlanner{}
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	target := plannerTarget(20, 200, 2, strings.Repeat("a", 64))
	target.Terms = []domain.CollectionTerm{{Value: "climate"}, {Value: "spam", Excluded: true}}
	request, err := planner.Plan(target, windowStart, windowStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if request.Query != "climate -spam" {
		t.Fatalf("planned terms = %q, want stable include/exclude query", request.Query)
	}
	target.QueryOverride, target.Terms = "", nil
	if _, err := planner.Plan(target, windowStart, windowStart.Add(time.Hour)); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("Plan(empty terms) error class = %q, want permanent", domain.ClassifyCollectionError(err))
	}
	target = plannerTarget(20, 0, 2, strings.Repeat("a", 64))
	if _, err := planner.Plan(target, windowStart, windowStart.Add(time.Hour)); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("Plan(target without immutable published config) error class = %q, want permanent", domain.ClassifyCollectionError(err))
	}
}

func TestQueryPlannerPlanCanonicalizesPublishedTermOrder(t *testing.T) {
	t.Parallel()

	planner := QueryPlanner{}
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	target := plannerTarget(20, 200, 2, strings.Repeat("a", 64))
	target.Terms = []domain.CollectionTerm{{Value: "spam", Excluded: true}, {Value: "climate"}}
	request, err := planner.Plan(target, windowStart, windowStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if request.Query != "climate -spam" {
		t.Fatalf("planned canonical terms = %q, want source-record-order-independent query", request.Query)
	}
}

func TestQueryPlannerGroupRequestsUsesPublishedIdentityAndStableTargetOrder(t *testing.T) {
	t.Parallel()

	planner := QueryPlanner{}
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	signature := strings.Repeat("b", 64)
	requestFor := func(target domain.PublishedCollectionTarget, start, end time.Time) domain.CollectionRequest {
		request, err := planner.Plan(target, start, end)
		if err != nil {
			t.Fatalf("Plan(): %v", err)
		}
		return request
	}

	first := requestFor(plannerTarget(20, 200, 2, signature), windowStart, windowStart.Add(time.Hour))
	second := requestFor(plannerTarget(10, 100, 2, signature), windowStart, windowStart.Add(time.Hour))
	differentWindow := requestFor(plannerTarget(30, 300, 2, signature), windowStart.Add(time.Hour), windowStart.Add(2*time.Hour))
	differentSource := requestFor(plannerTarget(40, 400, 3, signature), windowStart, windowStart.Add(time.Hour))

	groups, err := planner.GroupRequests([]domain.CollectionRequest{differentSource, first, differentWindow, second})
	if err != nil {
		t.Fatalf("GroupRequests(): %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("group count = %d, want distinct source/window identities", len(groups))
	}
	firstGroup := groups[0]
	if firstGroup.SourceConnectionID != 2 || !firstGroup.WindowStart.Equal(windowStart) || len(firstGroup.Targets) != 2 || firstGroup.Targets[0].MonitorSourceID != 10 || firstGroup.Targets[1].MonitorSourceID != 20 {
		t.Fatalf("first group = %#v, want stable shared request and target order", firstGroup)
	}
	if groups[1].SourceConnectionID != 2 || !groups[1].WindowStart.Equal(windowStart.Add(time.Hour)) || groups[2].SourceConnectionID != 3 {
		t.Fatalf("group order/identity = %#v", groups)
	}
	for _, group := range groups {
		if group.QuerySignature != signature {
			t.Fatalf("group signature = %q, want preserved published signature %q", group.QuerySignature, signature)
		}
	}
}

func TestQueryPlannerGroupRequestsRejectsDriftFromPublishedTarget(t *testing.T) {
	t.Parallel()

	planner := QueryPlanner{}
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	request, err := planner.Plan(plannerTarget(20, 200, 2, strings.Repeat("a", 64)), windowStart, windowStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	for _, mutate := range []struct {
		name  string
		apply func(*domain.CollectionRequest)
	}{
		{"query", func(request *domain.CollectionRequest) { request.Query = "different query" }},
		{"languages", func(request *domain.CollectionRequest) { request.Languages = []string{"fr"} }},
		{"regions", func(request *domain.CollectionRequest) { request.Regions = []string{"US"} }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			candidate := request
			candidate.Languages = append([]string(nil), request.Languages...)
			candidate.Regions = append([]string(nil), request.Regions...)
			candidate.Targets = append([]domain.PublishedCollectionTarget(nil), request.Targets...)
			mutate.apply(&candidate)
			if _, err := planner.GroupRequests([]domain.CollectionRequest{candidate}); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
				t.Fatalf("GroupRequests(%s drift) error = %v, class = %q; want permanent error", mutate.name, err, domain.ClassifyCollectionError(err))
			}
		})
	}
}

func plannerTarget(monitorSourceID, configVersionID, sourceConnectionID int64, signature string) domain.PublishedCollectionTarget {
	return domain.PublishedCollectionTarget{
		MonitorSourceID: monitorSourceID, MonitorConfigVersionID: configVersionID, SourceConnectionID: sourceConnectionID,
		QuerySignature: signature, Terms: []domain.CollectionTerm{{Value: "climate"}}, Languages: []string{"en"},
		CollectionInterval: 5 * time.Minute,
		Checkpoint:         domain.CollectionCheckpoint{MonitorSourceID: monitorSourceID, QueryHash: signature, NextPollAt: time.Date(2026, time.July, 16, 7, 55, 0, 0, time.UTC)},
	}
}
