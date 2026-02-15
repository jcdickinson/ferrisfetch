package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [crate[@version] ...]",
	Short: "Index crate documentation from docs.rs",
	Long:  `Fetch, parse, embed, and index Rust crate documentation. Version defaults to "latest".`,
	Example: `  ferrisfetch add serde
  ferrisfetch add serde@1.0 tokio@1.0
  ferrisfetch add serde serde_json tokio`,
	Args: cobra.MinimumNArgs(1),
	Run:  runAdd,
}

func runAdd(cmd *cobra.Command, args []string) {
	var specs []rpc.CrateSpec
	for _, arg := range args {
		name, version, _ := strings.Cut(arg, "@")
		specs = append(specs, rpc.CrateSpec{Name: name, Version: version})
	}

	client, err := connectDaemon()
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}

	resp, err := client.AddCrates(context.Background(), specs, func(msg string) {
		fmt.Printf("  %s\n", msg)
	})
	if err != nil {
		log.Fatalf("failed to add crates: %v", err)
	}

	for _, r := range resp.Results {
		if r.Error != "" {
			fmt.Printf("  %s@%s: error: %s\n", r.Name, r.Version, r.Error)
		} else {
			fmt.Printf("  %s@%s: %d items indexed\n", r.Name, r.Version, r.Items)
		}
	}
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search indexed crate documentation",
	Example: `  ferrisfetch search "serialize a struct to JSON"
  ferrisfetch search --crate serde "derive macro"
  ferrisfetch search --limit 5 "async runtime"`,
	Args: cobra.ExactArgs(1),
	Run:  runSearch,
}

var (
	searchCrates []string
	searchLimit  int
)

func init() {
	searchCmd.Flags().StringSliceVar(&searchCrates, "crate", nil, "filter to specific crates (repeatable)")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "max results")
}

func runSearch(cmd *cobra.Command, args []string) {
	client, err := connectDaemon()
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}

	resp, err := client.Search(context.Background(), rpc.SearchRequest{
		Query:  args[0],
		Crates: searchCrates,
		Limit:  searchLimit,
	})
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}

	if len(resp.Results) == 0 {
		fmt.Println("no results")
		return
	}

	for i, r := range resp.Results {
		fmt.Printf("%d. [%.2f] %s (%s) — %s@%s\n", i+1, r.Score, r.Path, r.Kind, r.CrateName, r.CrateVersion)
		if r.Snippet != "" {
			fmt.Printf("   %s\n", r.Snippet)
		}
	}
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show indexed crates and daemon state",
	Run:   runStatus,
}

var statusJSON bool

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output as JSON")
}

func runStatus(cmd *cobra.Command, args []string) {
	client, err := connectDaemon()
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}

	resp, err := client.Status(context.Background())
	if err != nil {
		log.Fatalf("status failed: %v", err)
	}

	if statusJSON {
		out, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(out))
		return
	}

	if len(resp.Crates) == 0 {
		fmt.Println("no crates indexed")
		return
	}

	for _, c := range resp.Crates {
		state := "processing"
		if c.Processed {
			state = "ready"
		}
		fmt.Printf("  %s@%s [%s]\n", c.Name, c.Version, state)
	}
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon",
	Run:   runStop,
}

func runStop(cmd *cobra.Command, args []string) {
	client := daemon.NewClient(config.SocketPath())
	if !client.IsAvailable() {
		fmt.Println("daemon is not running")
		return
	}

	if err := client.Shutdown(context.Background()); err != nil {
		// Connection reset is expected — daemon exits after responding
		fmt.Println("daemon stopped")
		return
	}
	fmt.Println("daemon stopped")
}
