package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hotkey-server",
	Short: "HotKey - X热点监控平台",
	Long:  "HotKey is a real-time keyword monitoring platform for X (Twitter).",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
