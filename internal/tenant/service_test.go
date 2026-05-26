package tenant

import "testing"

func TestUserCanBelongToMultipleTenants(t *testing.T) {
	service := NewService()

	alpha, err := service.CreateTenant(CreateTenantInput{Name: "Alpha Lab", Slug: "alpha"})
	if err != nil {
		t.Fatalf("create alpha tenant: %v", err)
	}
	beta, err := service.CreateTenant(CreateTenantInput{Name: "Beta Studio", Slug: "beta"})
	if err != nil {
		t.Fatalf("create beta tenant: %v", err)
	}

	if err := service.AddMembership(MembershipInput{TenantID: alpha.ID, UserID: "user-1", Role: RoleOwner}); err != nil {
		t.Fatalf("add alpha membership: %v", err)
	}
	if err := service.AddMembership(MembershipInput{TenantID: beta.ID, UserID: "user-1", Role: RoleMember}); err != nil {
		t.Fatalf("add beta membership: %v", err)
	}

	tenants := service.ListUserTenants("user-1")
	if len(tenants) != 2 {
		t.Fatalf("tenants len = %d, want 2: %#v", len(tenants), tenants)
	}
	if tenants[0].Slug != "alpha" || tenants[1].Slug != "beta" {
		t.Fatalf("tenants not sorted or isolated: %#v", tenants)
	}
}

func TestTenantRequiresNameSlugAndMembershipUser(t *testing.T) {
	service := NewService()

	if _, err := service.CreateTenant(CreateTenantInput{Name: " ", Slug: "alpha"}); err != ErrInvalidTenant {
		t.Fatalf("create invalid tenant err = %v, want ErrInvalidTenant", err)
	}
	tenant, err := service.CreateTenant(CreateTenantInput{Name: "Alpha Lab", Slug: "alpha"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := service.AddMembership(MembershipInput{TenantID: tenant.ID, UserID: " ", Role: RoleMember}); err != ErrInvalidMembership {
		t.Fatalf("invalid membership err = %v, want ErrInvalidMembership", err)
	}
}
