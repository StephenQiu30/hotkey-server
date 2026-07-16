package application

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

// QueryPlanner turns immutable published target facts into a repeatable shared
// collection request. It never recalculates query_signature: that identity was
// fixed when the Monitor configuration was published.
type QueryPlanner struct{}

func (QueryPlanner) Plan(target domain.PublishedCollectionTarget, windowStart, windowEnd time.Time) (domain.CollectionRequest, error) {
	if err := target.Validate(); err != nil {
		return domain.CollectionRequest{}, invalidCollectionPlan(err)
	}
	if windowStart.IsZero() || windowEnd.IsZero() || !windowEnd.After(windowStart) {
		return domain.CollectionRequest{}, invalidCollectionPlan(fmt.Errorf("collection window is invalid"))
	}
	query := strings.TrimSpace(target.QueryOverride)
	if query == "" {
		query = queryFromTerms(target.Terms)
	}
	if query == "" {
		return domain.CollectionRequest{}, invalidCollectionPlan(fmt.Errorf("published collection target has no effective query terms"))
	}
	request := domain.CollectionRequest{
		SourceConnectionID: target.SourceConnectionID, QuerySignature: target.QuerySignature, Query: query,
		Languages: append([]string(nil), target.Languages...), Regions: append([]string(nil), target.Regions...),
		WindowStart: windowStart.UTC(), WindowEnd: windowEnd.UTC(), Targets: []domain.PublishedCollectionTarget{cloneCollectionTarget(target)},
	}
	if err := request.Validate(); err != nil {
		return domain.CollectionRequest{}, invalidCollectionPlan(err)
	}
	return request, nil
}

// GroupRequests keeps the request identity fixed at source/signature/window.
// Locale and query text are already represented by the published signature;
// if corrupted inputs disagree they are rejected rather than silently merged.
func (QueryPlanner) GroupRequests(requests []domain.CollectionRequest) ([]domain.CollectionRequest, error) {
	if len(requests) == 0 {
		return []domain.CollectionRequest{}, nil
	}
	ordered := make([]domain.CollectionRequest, 0, len(requests))
	for _, request := range requests {
		if err := request.Validate(); err != nil {
			return nil, invalidCollectionPlan(err)
		}
		request.Query = strings.TrimSpace(request.Query)
		if request.Query == "" {
			return nil, invalidCollectionPlan(fmt.Errorf("collection request query is required"))
		}
		request.Languages = append([]string(nil), request.Languages...)
		request.Regions = append([]string(nil), request.Regions...)
		request.Targets = cloneCollectionTargets(request.Targets)
		ordered = append(ordered, request)
	}
	sort.Slice(ordered, func(i, j int) bool { return collectionRequestLess(ordered[i], ordered[j]) })

	groups := make([]domain.CollectionRequest, 0, len(ordered))
	for _, request := range ordered {
		last := len(groups) - 1
		if last < 0 || !sameCollectionIdentity(groups[last], request) {
			groups = append(groups, request)
			continue
		}
		if groups[last].Query != request.Query || !reflect.DeepEqual(groups[last].Languages, request.Languages) || !reflect.DeepEqual(groups[last].Regions, request.Regions) {
			return nil, invalidCollectionPlan(fmt.Errorf("published signature has inconsistent query inputs"))
		}
		groups[last].Targets = append(groups[last].Targets, request.Targets...)
	}
	for index := range groups {
		sort.Slice(groups[index].Targets, func(left, right int) bool {
			if groups[index].Targets[left].MonitorSourceID != groups[index].Targets[right].MonitorSourceID {
				return groups[index].Targets[left].MonitorSourceID < groups[index].Targets[right].MonitorSourceID
			}
			return groups[index].Targets[left].MonitorConfigVersionID < groups[index].Targets[right].MonitorConfigVersionID
		})
		for targetIndex := 1; targetIndex < len(groups[index].Targets); targetIndex++ {
			if groups[index].Targets[targetIndex-1].MonitorSourceID == groups[index].Targets[targetIndex].MonitorSourceID {
				return nil, invalidCollectionPlan(fmt.Errorf("duplicate published collection target"))
			}
		}
	}
	return groups, nil
}

func queryFromTerms(terms []domain.CollectionTerm) string {
	normalized := make([]domain.CollectionTerm, 0, len(terms))
	for _, term := range terms {
		term.Value = strings.TrimSpace(term.Value)
		value := term.Value
		if value == "" {
			continue
		}
		normalized = append(normalized, term)
	}
	sort.Slice(normalized, func(left, right int) bool {
		if normalized[left].Excluded != normalized[right].Excluded {
			return !normalized[left].Excluded
		}
		return normalized[left].Value < normalized[right].Value
	})
	tokens := make([]string, 0, len(normalized))
	for _, term := range normalized {
		value := term.Value
		if term.Excluded {
			value = "-" + value
		}
		tokens = append(tokens, value)
	}
	return strings.Join(tokens, " ")
}

func invalidCollectionPlan(cause error) error {
	return domain.NewCollectionError(domain.CollectionErrorPermanent, cause)
}

func sameCollectionIdentity(left, right domain.CollectionRequest) bool {
	return left.SourceConnectionID == right.SourceConnectionID && left.QuerySignature == right.QuerySignature && left.WindowStart.Equal(right.WindowStart) && left.WindowEnd.Equal(right.WindowEnd)
}

func collectionRequestLess(left, right domain.CollectionRequest) bool {
	if left.SourceConnectionID != right.SourceConnectionID {
		return left.SourceConnectionID < right.SourceConnectionID
	}
	if left.QuerySignature != right.QuerySignature {
		return left.QuerySignature < right.QuerySignature
	}
	if !left.WindowStart.Equal(right.WindowStart) {
		return left.WindowStart.Before(right.WindowStart)
	}
	return left.WindowEnd.Before(right.WindowEnd)
}

func cloneCollectionTargets(targets []domain.PublishedCollectionTarget) []domain.PublishedCollectionTarget {
	cloned := make([]domain.PublishedCollectionTarget, 0, len(targets))
	for _, target := range targets {
		cloned = append(cloned, cloneCollectionTarget(target))
	}
	return cloned
}

func cloneCollectionTarget(target domain.PublishedCollectionTarget) domain.PublishedCollectionTarget {
	target.Terms = append([]domain.CollectionTerm(nil), target.Terms...)
	target.Languages = append([]string(nil), target.Languages...)
	target.Regions = append([]string(nil), target.Regions...)
	return target
}
