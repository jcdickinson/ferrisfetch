package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/spf13/cobra"
)

var clearCacheCmd = &cobra.Command{
	Use:   "clear-cache",
	Short: "Clear the daemon's version resolution cache",
	Run:   runClearCache,
}

func runClearCache(cmd *cobra.Command, args []string) {
	client := daemon.NewClient(config.SocketPath())
	if !client.IsAvailable() {
		fmt.Println("daemon is not running")
		return
	}

	if err := client.ClearCache(context.Background()); err != nil {
		slog.Error("failed to clear cache", "error", err)
		os.Exit(1)
	}
	fmt.Println("version cache cleared")
}
