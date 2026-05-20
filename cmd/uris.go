package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/spf13/cobra"
)

var urisCmd = &cobra.Command{
	Use:   "uris <crate[@version]>",
	Short: "List all rsdoc:// URIs for a crate",
	Long: `List every rsdoc:// URI in a crate, one per line, including fragment URIs.

Useful when you have an item name in mind but aren't sure how to spell its URI,
or when a `+"`rsdoc get`"+` lookup failed and you want to see the canonical paths.

If the crate isn't indexed yet, it will be fetched first.`,
	Example: `  rsdoc uris serde
  rsdoc uris tokio@1.44.2
  rsdoc uris openrouter-rs | grep StreamEvent`,
	Args: cobra.ExactArgs(1),
	Run:  runURIs,
}

var urisFragments bool

func init() {
	urisCmd.Flags().BoolVar(&urisFragments, "fragments", true, "include fragment URIs (#fields, #variants, etc.)")
	rootCmd.AddCommand(urisCmd)
}

func runURIs(cmd *cobra.Command, args []string) {
	name, version := parseCrateArg(args[0])

	client, err := connectDaemon()
	if err != nil {
		slog.Error("failed to connect to daemon", "error", err)
		os.Exit(1)
	}

	resp, err := client.ListURIs(context.Background(), rpc.ListURIsRequest{
		Crate:   name,
		Version: version,
	})
	if err != nil {
		slog.Error("list uris failed", "error", err)
		os.Exit(1)
	}

	sep := "#"
	if isAgent() {
		sep = "%"
	}

	for _, it := range resp.Items {
		fmt.Printf("%s\t%s\n", it.Kind, it.URI)
		if urisFragments {
			for _, f := range it.Fragments {
				fmt.Printf("%s\t%s%s%s\n", it.Kind, it.URI, sep, f)
			}
		}
	}
}
