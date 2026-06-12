package main

import (
	"log"
	"net/http"

	"github.com/stephenqiu/hotkey-server/internal/config"
	"github.com/stephenqiu/hotkey-server/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	router := server.NewRouter()
	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}
