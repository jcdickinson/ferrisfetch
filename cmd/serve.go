package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/jcdickinson/ferrisfetch/internal/db"
	"github.com/spf13/cobra"
)

//go:embed agent_help.md
var agentHelp string

var debug bool

var rootCmd = &cobra.Command{
	Use:   "rsdoc",
	Short: "Rust documentation semantic search",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "run daemon in-process (visible log output)")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(clearCacheCmd)
	rootCmd.AddCommand(searchCratesCmd)

	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if isAgent() {
			fmt.Print(agentHelp)
			return
		}
		defaultHelp(cmd, args)
	})
}

func isAgent() bool {
	return os.Getenv("CLAUDECODE") == "1" || os.Getenv("AGENT") == "1"
}

// connectDaemon returns a daemon client. In debug mode, starts the daemon
// in-process so all log output is visible in the terminal.
func connectDaemon() (*daemon.Client, error) {
	socketPath := config.SocketPath()

	if !debug {
		return daemon.ConnectOrSpawn(socketPath)
	}

	// In debug mode: stop any existing daemon, then start in-process
	client := daemon.NewClient(socketPath)
	if client.IsAvailable() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client.Shutdown(shutdownCtx)
		cancel()
		time.Sleep(200 * time.Millisecond)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	database, err := db.New(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	srv := daemon.NewServer(cfg, database, socketPath)
	go func() {
		if err := srv.Start(context.Background()); err != nil {
			slog.Error("in-process daemon error", "error", err)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if client.IsAvailable() {
			return client, nil
		}
	}

	return nil, fmt.Errorf("in-process daemon did not start within 5 seconds")
}
