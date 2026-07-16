package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts every accepted identity route at the fixed v1
// contract. Public verification remains separate from bearer-protected and
// administrator-only route groups.
func RegisterRoutes(router *gin.Engine, service identityService, authenticator httptransport.Authenticator, cfg config.Config) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service, cfg)
	api := router.Group("/api/v1")

	publicAuth := httptransport.PublicAuthGroup(api)
	publicAuth.POST("/email-verifications", httptransport.Wrap(handler.RequestVerification))
	publicAuth.POST("/email-verifications/confirm", httptransport.Wrap(handler.ConfirmVerification))
	publicAuth.POST("/registrations", httptransport.Wrap(handler.Register))
	publicAuth.POST("/login", httptransport.Wrap(handler.Login))
	publicAuth.POST("/password-resets/confirm", httptransport.Wrap(handler.ConfirmPasswordReset))

	// Refresh and logout carry the HttpOnly cookie, so both require the exact
	// configured Origin even if logout also presents a valid Bearer token.
	publicAuth.POST("/refresh", httptransport.RequireCookieOrigin(cfg.Authentication.AllowedOrigins), httptransport.Wrap(handler.Refresh))
	publicAuth.POST("/logout", httptransport.RequireCookieOrigin(cfg.Authentication.AllowedOrigins), optionalAuthentication(authenticator), httptransport.Wrap(handler.Logout))

	protectedAuth := api.Group("/auth", httptransport.RequireAuthentication(authenticator))
	protectedAuth.GET("/me", httptransport.Wrap(handler.Me))
	protectedAuth.POST("/password", httptransport.Wrap(handler.ChangePassword))

	adminUsers := api.Group("/users", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	adminUsers.GET("", httptransport.Wrap(handler.ListUsers))
	adminUsers.PATCH("/:id", httptransport.Wrap(handler.UpdateUser))
	adminUsers.DELETE("/:id", httptransport.Wrap(handler.DeleteUser))
	adminUsers.POST("/:id/restore", httptransport.Wrap(handler.RestoreUser))
}
