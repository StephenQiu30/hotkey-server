package main

import (
	"github.com/spf13/cobra"

	"github.com/StephenQiu30/hotkey-server/internal/app"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Long:  "Start the HotKey API server with HTTP endpoints.",
	Run: func(cmd *cobra.Command, args []string) {
		app.RunAPI()
	},
}

func init() {
	rootCmd.AddCommand(apiCmd)
}
