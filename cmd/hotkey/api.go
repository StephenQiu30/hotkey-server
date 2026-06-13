package main

import (
	"log"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/spf13/cobra"
)

func apiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "api",
		Short: "Start the HTTP API server",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := app.InitializeAPI()
			if err != nil {
				log.Fatal(err)
			}
			app.NewAPIApp(cfg).Run()
		},
	}
}
