package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	"github.com/gin-gonic/gin"
)

// NewRouter creates a router with memory repositories and hardcoded test secrets.
// FOR TESTING ONLY.
func NewRouter() *gin.Engine {
	repo := serviceauth.NewMemoryRepository()
	authService, err := serviceauth.NewService(repo, serviceauth.Config{
		AccessTokenSecret: "test-router-secret",
	})
	if err != nil {
		panic(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		panic(err)
	}
	azService := serviceauth.NewAuthorizationService(repo, nil, enc, nil)
	return NewRouterWithServices(authService, servicechannel.NewService(servicechannel.NewMemoryRepository()), azService)
}

// NewRouterWithAuth creates a router with the provided auth service and a shared repository for authorizations.
// FOR TESTING ONLY.
func NewRouterWithAuth(authService *serviceauth.Service) *gin.Engine {
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		panic(err)
	}
	// Share the same user repository between auth and authorization services
	azService := serviceauth.NewAuthorizationService(authService.Repository(), nil, enc, nil)
	return NewRouterWithServices(authService, servicechannel.NewService(servicechannel.NewMemoryRepository()), azService)
}

// NewRouterWithServices creates a router with provided services.
// FOR TESTING ONLY.
func NewRouterWithServices(authService *serviceauth.Service, channelService *servicechannel.Service, azServices ...*serviceauth.AuthorizationService) *gin.Engine {
	deps := Dependencies{AuthService: authService, ChannelService: channelService}
	if len(azServices) > 0 && azServices[0] != nil {
		deps.AuthorizationService = azServices[0]
	}

	if deps.AuthorizationService == nil {
		key := []byte("0123456789abcdef0123456789abcdef")
		enc, err := crypto.NewAESGCMEncryptor(key)
		if err != nil {
			panic(err)
		}
		// Use the same repository as authService if possible
		var userRepo serviceauth.Repository
		if authService != nil {
			userRepo = authService.Repository()
		} else {
			userRepo = serviceauth.NewMemoryRepository()
		}
		deps.AuthorizationService = serviceauth.NewAuthorizationService(userRepo, serviceauth.NewMemoryAuthorizationRepository(), enc, nil)
	}

	return NewRouterWithDependencies(deps)
}
