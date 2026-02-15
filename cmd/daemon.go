package cmd

import (
	"context"
	"log"
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
		log.Fatalf("failed to create log directory: %v", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	database, err := db.New(config.DBPath())
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	srv := daemon.NewServer(cfg, database, config.SocketPath())
	if err := srv.Start(context.Background()); err != nil {
		log.Fatalf("daemon failed: %v", err)
	}
}
