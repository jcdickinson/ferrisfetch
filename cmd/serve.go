package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/jcdickinson/ferrisfetch/internal/db"
	"github.com/jcdickinson/ferrisfetch/internal/mcp"
	"github.com/spf13/cobra"
)

var debug bool

var rootCmd = &cobra.Command{
	Use:   "ferrisfetch",
	Short: "Rust documentation semantic search MCP server",
	Run:   runServe,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("command failed: %v", err)
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
			log.Printf("in-process daemon error: %v", err)
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

func runServe(cmd *cobra.Command, args []string) {
	socketPath := config.SocketPath()

	server, err := mcp.NewServer(socketPath)
	if err != nil {
		log.Fatalf("failed to create MCP server: %v", err)
	}

	errCh := make(chan error)
	go func() { errCh <- server.Run() }()

	if err := waitForSignal(errCh); err != nil {
		log.Fatalf("server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func waitForSignal(errCh chan error) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigs:
		log.Printf("received signal: %s", sig)
		return nil
	case err := <-errCh:
		return err
	}
}
