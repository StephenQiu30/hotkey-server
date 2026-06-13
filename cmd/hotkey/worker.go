package main

import (
	"github.com/spf13/cobra"

	"github.com/StephenQiu30/hotkey-server/internal/app"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Start the background worker",
	Long:  "Start the HotKey background worker for processing jobs.",
	Run: func(cmd *cobra.Command, args []string) {
		app.RunWorker()
	},
}

func init() {
	rootCmd.AddCommand(workerCmd)
}
