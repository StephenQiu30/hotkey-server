package billing

import (
	"strings"
	"sync"
)

const (
	MetricCollection = "collection"
	MetricRefresh    = "refresh"
	MetricAICall     = "ai_call"

	ReasonQuotaExceeded = "quota_exceeded"
)

type Plan struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Quotas map[string]int `json:"quotas"`
}

type UsageInput struct {
	TenantID string
	Metric   string
	Amount   int
}

type UsageResult struct {
	TenantID string `json:"tenantId"`
	Metric   string `json:"metric"`
	Amount   int    `json:"amount"`
	Used     int    `json:"used"`
	Quota    int    `json:"quota"`
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason,omitempty"`
}

type UsageSummary struct {
	TenantID string         `json:"tenantId"`
	Plan     Plan           `json:"plan"`
	Usage    map[string]int `json:"usage"`
	Quotas   map[string]int `json:"quotas"`
}

type Service struct {
	mu     sync.Mutex
	plans  map[string]Plan
	usages map[string]map[string]int
}

func NewService() *Service {
	return &Service{
		plans:  make(map[string]Plan),
		usages: make(map[string]map[string]int),
	}
}

func (s *Service) AssignPlan(tenantID string, plan Plan) Plan {
	tenantID = strings.TrimSpace(tenantID)
	plan.ID = strings.TrimSpace(plan.ID)
	plan.Name = strings.Join(strings.Fields(plan.Name), " ")
	plan.Quotas = cloneIntMap(plan.Quotas)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.plans[tenantID] = plan
	if _, ok := s.usages[tenantID]; !ok {
		s.usages[tenantID] = make(map[string]int)
	}
	return clonePlan(plan)
}

func (s *Service) RecordUsage(input UsageInput) UsageResult {
	tenantID := strings.TrimSpace(input.TenantID)
	metric := normalizeMetric(input.Metric)
	amount := input.Amount
	if amount <= 0 {
		amount = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	plan := s.plans[tenantID]
	quota := plan.Quotas[metric]
	used := s.usages[tenantID][metric]
	if quota > 0 && used+amount > quota {
		return UsageResult{
			TenantID: tenantID,
			Metric:   metric,
			Amount:   amount,
			Used:     used,
			Quota:    quota,
			Allowed:  false,
			Reason:   ReasonQuotaExceeded,
		}
	}
	if _, ok := s.usages[tenantID]; !ok {
		s.usages[tenantID] = make(map[string]int)
	}
	s.usages[tenantID][metric] = used + amount
	return UsageResult{
		TenantID: tenantID,
		Metric:   metric,
		Amount:   amount,
		Used:     used + amount,
		Quota:    quota,
		Allowed:  true,
	}
}

func (s *Service) GetUsageSummary(tenantID string) UsageSummary {
	tenantID = strings.TrimSpace(tenantID)
	s.mu.Lock()
	defer s.mu.Unlock()

	plan := clonePlan(s.plans[tenantID])
	return UsageSummary{
		TenantID: tenantID,
		Plan:     plan,
		Usage:    cloneIntMap(s.usages[tenantID]),
		Quotas:   cloneIntMap(plan.Quotas),
	}
}

func normalizeMetric(metric string) string {
	switch strings.TrimSpace(metric) {
	case MetricRefresh:
		return MetricRefresh
	case MetricAICall:
		return MetricAICall
	default:
		return MetricCollection
	}
}

func clonePlan(plan Plan) Plan {
	plan.Quotas = cloneIntMap(plan.Quotas)
	return plan
}

func cloneIntMap(values map[string]int) map[string]int {
	result := make(map[string]int, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
