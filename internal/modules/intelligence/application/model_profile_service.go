package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
)

// ProviderRegistry is the runtime-only provider boundary. A profile may be
// valid and enabled while its provider is absent because an optional key or
// native ONNX bundle is not installed; callers treat that as safe degradation.
type ProviderRegistry struct {
	providers map[domain.ProviderName]domain.Provider
}

func NewProviderRegistry(providers map[domain.ProviderName]domain.Provider) *ProviderRegistry {
	copyOfProviders := make(map[domain.ProviderName]domain.Provider, len(providers))
	for name, provider := range providers {
		if name.Valid() && provider != nil {
			copyOfProviders[name] = provider
		}
	}
	return &ProviderRegistry{providers: copyOfProviders}
}

func (registry *ProviderRegistry) Resolve(name domain.ProviderName) (domain.Provider, bool) {
	if registry == nil {
		return nil, false
	}
	provider, ok := registry.providers[name]
	return provider, ok
}

// ModelProfileService is the application boundary for the profile lifecycle.
// Credential material remains write-only at the future HTTP transport layer;
// this service neither resolves nor returns a secret value on its own.
type ModelProfileService struct {
	profiles *intelligencepostgres.Repository
}

func NewModelProfileService(profiles *intelligencepostgres.Repository) (*ModelProfileService, error) {
	if profiles == nil {
		return nil, fmt.Errorf("AI profile repository is required")
	}
	return &ModelProfileService{profiles: profiles}, nil
}

func (service *ModelProfileService) Create(ctx context.Context, profile domain.ModelProfile) (domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.ModelProfile{}, fmt.Errorf("AI model profile service is unavailable")
	}
	if err := service.profiles.CreateProfile(ctx, &profile); err != nil {
		return domain.ModelProfile{}, err
	}
	return profile, nil
}

// List returns every profile, including archived ones, for the administrator
// control plane. HTTP maps this value to a DTO that excludes credentials.
func (service *ModelProfileService) List(ctx context.Context) ([]domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return nil, fmt.Errorf("AI model profile service is unavailable")
	}
	return service.profiles.ListProfiles(ctx)
}

// Get includes the deletion state so an administrator can restore a profile.
// The HTTP transport deliberately does not expose CredentialRef.
func (service *ModelProfileService) Get(ctx context.Context, id int64) (domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.ModelProfile{}, fmt.Errorf("AI model profile service is unavailable")
	}
	return service.profiles.GetProfile(ctx, id)
}

func (service *ModelProfileService) Update(ctx context.Context, profile domain.ModelProfile, expectedVersion int64) (domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.ModelProfile{}, fmt.Errorf("AI model profile service is unavailable")
	}
	return service.profiles.UpdateProfile(ctx, profile, expectedVersion)
}

func (service *ModelProfileService) SoftDelete(ctx context.Context, id, expectedVersion int64) (domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.ModelProfile{}, fmt.Errorf("AI model profile service is unavailable")
	}
	return service.profiles.SoftDeleteProfile(ctx, id, expectedVersion)
}

func (service *ModelProfileService) Restore(ctx context.Context, id, expectedVersion int64) (domain.ModelProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.ModelProfile{}, fmt.Errorf("AI model profile service is unavailable")
	}
	return service.profiles.RestoreProfile(ctx, id, expectedVersion)
}
