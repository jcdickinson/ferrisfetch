package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <rsdoc://crate/version/path>",
	Short: "Read a documentation item by URI",
	Example: `  rsdoc get rsdoc://serde/latest/serde::Serialize
  rsdoc get rsdoc://tokio/1.0.0/tokio::spawn
  rsdoc get serde/latest/serde::Serialize`,
	Args: cobra.ExactArgs(1),
	Run:  runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) {
	uri := strings.TrimPrefix(args[0], "rsdoc://")
	parts := strings.SplitN(uri, "/", 3)
	if len(parts) < 3 {
		slog.Error("invalid URI: need crate/version/path")
		os.Exit(1)
	}

	path := parts[2]
	var fragment string
	if idx := strings.LastIndex(path, "#"); idx >= 0 {
		fragment = path[idx+1:]
		path = path[:idx]
	}

	client, err := connectDaemon()
	if err != nil {
		slog.Error("failed to connect to daemon", "error", err)
		os.Exit(1)
	}

	resp, err := client.GetDoc(context.Background(), rpc.GetDocRequest{
		Crate:    parts[0],
		Version:  parts[1],
		Path:     path,
		Fragment: fragment,
	})
	if err != nil {
		slog.Error("get doc failed", "error", err)
		os.Exit(1)
	}

	fmt.Print(resp.Markdown)
}
