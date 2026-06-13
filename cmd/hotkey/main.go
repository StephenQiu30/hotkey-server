// Package main is the entry point for the hotkey-server binary.
// It uses Cobra for subcommand routing (api / worker).
package main

import (
	"log"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "hotkey-server",
		Short: "HotKey monitoring platform server",
	}

	root.AddCommand(apiCmd())
	root.AddCommand(workerCmd())

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
