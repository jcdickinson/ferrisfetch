package cmd

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/jcdickinson/ferrisfetch/internal/db"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the background daemon (usually spawned automatically)",
	Run:   runDaemon,
}

func runDaemon(cmd *cobra.Command, args []string) {
	logPath := config.LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		slog.Error("failed to create log directory", "error", err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("failed to open log file", "error", err)
		os.Exit(1)
	}
	defer logFile.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	database, err := db.New(config.DBPath())
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	srv := daemon.NewServer(cfg, database, config.SocketPath())
	if err := srv.Start(context.Background()); err != nil {
		slog.Error("daemon failed", "error", err)
		os.Exit(1)
	}
}
