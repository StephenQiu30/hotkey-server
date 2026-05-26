package tenant

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var (
	ErrInvalidTenant     = errors.New("invalid tenant")
	ErrTenantNotFound    = errors.New("tenant not found")
	ErrInvalidMembership = errors.New("invalid membership")
)

type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Membership struct {
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
	Role     string `json:"role"`
}

type CreateTenantInput struct {
	Name string
	Slug string
}

type MembershipInput struct {
	TenantID string
	UserID   string
	Role     string
}

type Service struct {
	mu         sync.Mutex
	nextTenant int
	tenants    map[string]Tenant
	members    map[string]Membership
}

func NewService() *Service {
	return &Service{
		nextTenant: 1,
		tenants:    make(map[string]Tenant),
		members:    make(map[string]Membership),
	}
}

func (s *Service) CreateTenant(input CreateTenantInput) (Tenant, error) {
	name := strings.Join(strings.Fields(input.Name), " ")
	slug := strings.TrimSpace(input.Slug)
	if name == "" || slug == "" {
		return Tenant{}, ErrInvalidTenant
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tenant := Tenant{
		ID:   fmt.Sprintf("tenant_%d", s.nextTenant),
		Name: name,
		Slug: slug,
	}
	s.nextTenant++
	s.tenants[tenant.ID] = tenant
	return tenant, nil
}

func (s *Service) AddMembership(input MembershipInput) error {
	tenantID := strings.TrimSpace(input.TenantID)
	userID := strings.TrimSpace(input.UserID)
	role := normalizeRole(input.Role)
	if tenantID == "" || userID == "" {
		return ErrInvalidMembership
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[tenantID]; !ok {
		return ErrTenantNotFound
	}
	s.members[membershipKey(tenantID, userID)] = Membership{
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}
	return nil
}

func (s *Service) ListUserTenants(userID string) []Tenant {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Tenant, 0)
	for _, membership := range s.members {
		if membership.UserID != userID {
			continue
		}
		if tenant, ok := s.tenants[membership.TenantID]; ok {
			result = append(result, tenant)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Slug < result[j].Slug
	})
	return result
}

func (s *Service) ListTenants() []Tenant {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Tenant, 0, len(s.tenants))
	for _, tenant := range s.tenants {
		result = append(result, tenant)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Slug < result[j].Slug
	})
	return result
}

func normalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case RoleOwner:
		return RoleOwner
	case RoleAdmin:
		return RoleAdmin
	default:
		return RoleMember
	}
}

func membershipKey(tenantID string, userID string) string {
	return tenantID + ":" + userID
}
