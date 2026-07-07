package main

import (
	"github.com/StephenQiu30/hotkey-server/internal/fxapp"
)

// @title           HotKey API
// @version         1.0
// @description     HotKey 热点监控平台后端 API
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @license.name    MIT

func main() {
	fxapp.NewApp().Run()
}
