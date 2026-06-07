package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// NewRouterWithAuthAndAz creates a router with auth and authorization services wired.
// FOR TESTING ONLY.
func NewRouterWithAuthAndAz(authService *serviceauth.Service) *gin.Engine {
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		panic(err)
	}
	azService, err := serviceauth.NewAuthorizationService(authService.Repository(), nil, enc, nil)
	if err != nil {
		panic(err)
	}
	return NewRouterWithDependencies(Dependencies{
		AuthService:          authService,
		AuthorizationService: azService,
	})
}
