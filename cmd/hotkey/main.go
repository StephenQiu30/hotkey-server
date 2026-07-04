package main

import (
	_ "github.com/StephenQiu30/hotkey-server/docs"
	"github.com/StephenQiu30/hotkey-server/internal/app"
)

// @title HotKey Server API
// @version 1.0
// @description HotKey server API documentation generated from Gin handlers.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	app.Run()
}
