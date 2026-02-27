package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/spf13/cobra"
)

var searchCratesCmd = &cobra.Command{
	Use:   "search-crates <query>",
	Short: "Search crates.io for Rust crates",
	Example: `  rsdoc search-crates serde
  rsdoc search-crates "async http client"
  rsdoc search-crates --limit 5 tokio`,
	Args: cobra.ExactArgs(1),
	Run:  runSearchCrates,
}

var searchCratesLimit int

func init() {
	searchCratesCmd.Flags().IntVar(&searchCratesLimit, "limit", 20, "max results")
}

func runSearchCrates(cmd *cobra.Command, args []string) {
	client, err := connectDaemon()
	if err != nil {
		slog.Error("failed to connect to daemon", "error", err)
		os.Exit(1)
	}

	resp, err := client.SearchCrates(context.Background(), rpc.SearchCratesRequest{
		Query: args[0],
		Limit: searchCratesLimit,
	})
	if err != nil {
		slog.Error("search failed", "error", err)
		os.Exit(1)
	}

	if len(resp.Results) == 0 {
		fmt.Println("no results")
		return
	}

	for _, r := range resp.Results {
		indexed := ""
		if r.IndexedVersion != "" {
			indexed = fmt.Sprintf(" [indexed: %s]", r.IndexedVersion)
		}
		fmt.Printf("  %-30s %s  (%d downloads)%s\n", r.Name, r.MaxVersion, r.Downloads, indexed)
		if r.Description != "" {
			fmt.Printf("    %s\n", r.Description)
		}
	}
}
