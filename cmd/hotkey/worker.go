package main

import (
	"log"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/spf13/cobra"
)

func workerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Start the background worker",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := app.InitializeWorker()
			if err != nil {
				log.Fatal(err)
			}
			app.NewWorkerApp(cfg).Run()
		},
	}
}
