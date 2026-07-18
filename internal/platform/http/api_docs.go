package http

import (
	stdhttp "net/http"

	"github.com/StephenQiu30/hotkey-server/docs/openapi"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func registerAPIDocumentation(router *gin.Engine) {
	router.GET("/openapi.json", func(c *gin.Context) {
		document := openapi.SwaggerInfo.ReadDoc()
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(stdhttp.StatusOK, "application/json; charset=utf-8", []byte(document))
	})
	router.GET("/docs", func(c *gin.Context) {
		c.Redirect(stdhttp.StatusTemporaryRedirect, "/docs/index.html")
	})
	router.GET(
		"/docs/*any",
		ginSwagger.WrapHandler(
			swaggerfiles.Handler,
			ginSwagger.URL("/openapi.json"),
			ginSwagger.DocExpansion("none"),
			ginSwagger.DeepLinking(true),
			ginSwagger.PersistAuthorization(false),
		),
	)
}
