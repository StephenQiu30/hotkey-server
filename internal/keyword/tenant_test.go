package keyword

import "testing"

func TestPlatformKeywordsCanBeIsolatedByTenant(t *testing.T) {
	service := NewService()

	alpha, err := service.CreatePlatformKeyword(CreatePlatformKeywordInput{
		TenantID: "tenant-alpha",
		Term:     "OpenAI",
		Category: "lab",
	})
	if err != nil {
		t.Fatalf("create alpha keyword: %v", err)
	}
	beta, err := service.CreatePlatformKeyword(CreatePlatformKeywordInput{
		TenantID: "tenant-beta",
		Term:     "Claude",
		Category: "lab",
	})
	if err != nil {
		t.Fatalf("create beta keyword: %v", err)
	}

	alphaKeywords := service.ListPlatformKeywordsByTenant("tenant-alpha")
	if len(alphaKeywords) != 1 || alphaKeywords[0].ID != alpha.ID {
		t.Fatalf("alpha keywords = %#v, want only %#v", alphaKeywords, alpha)
	}
	betaKeywords := service.ListPlatformKeywordsByTenant("tenant-beta")
	if len(betaKeywords) != 1 || betaKeywords[0].ID != beta.ID {
		t.Fatalf("beta keywords = %#v, want only %#v", betaKeywords, beta)
	}
}
