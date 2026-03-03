package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jcdickinson/ferrisfetch/internal/docs"
	"github.com/spf13/cobra"
)

var whichCmd = &cobra.Command{
	Use:   "which <crate>",
	Short: "Find crate versions with specific dependencies",
	Example: `  rsdoc which tokio --with mio@1.0.0
  rsdoc which tokio --with mio@1.0.0 --older-than 1.40.0
  rsdoc which serde --with serde_derive@1.0 --newer-than 1.0.100`,
	Args: cobra.ExactArgs(1),
	Run:  runWhich,
}

var (
	withDeps  []string
	newerThan string
	olderThan string
)

func init() {
	whichCmd.Flags().StringSliceVar(&withDeps, "with", nil, "dependency@version constraint (repeatable)")
	whichCmd.Flags().StringVar(&newerThan, "newer-than", "", "minimum version (inclusive)")
	whichCmd.Flags().StringVar(&olderThan, "older-than", "", "maximum version (inclusive)")
	rootCmd.AddCommand(whichCmd)
}

type depConstraint struct {
	name    string
	version string
}

func runWhich(cmd *cobra.Command, args []string) {
	crateName := args[0]

	if len(withDeps) == 0 {
		slog.Error("at least one --with flag is required")
		os.Exit(1)
	}

	// Parse --with constraints
	constraints := make([]depConstraint, len(withDeps))
	for i, w := range withDeps {
		name, version := parseCrateArg(w)
		if version == "" {
			slog.Error("--with requires name@version format", "value", w)
			os.Exit(1)
		}
		constraints[i] = depConstraint{name: name, version: version}
	}

	// Parse version bounds
	var lowerBound, upperBound *docs.Version
	if newerThan != "" {
		v, err := docs.ParseVersion(newerThan)
		if err != nil {
			slog.Error("invalid --newer-than version", "error", err)
			os.Exit(1)
		}
		lowerBound = &v
	}
	if olderThan != "" {
		v, err := docs.ParseVersion(olderThan)
		if err != nil {
			slog.Error("invalid --older-than version", "error", err)
			os.Exit(1)
		}
		upperBound = &v
	}

	// Fetch crate versions
	info, err := docs.FetchCrateInfo(crateName)
	if err != nil {
		slog.Error("failed to fetch crate info", "error", err)
		os.Exit(1)
	}

	// Filter candidate versions
	var candidates []docs.VersionInfo
	for _, v := range info.Versions {
		if v.Yanked {
			continue
		}
		ver, err := docs.ParseVersion(v.Num)
		if err != nil {
			continue
		}
		if ver.PreRelease != "" {
			continue
		}
		if lowerBound != nil && ver.Compare(*lowerBound) < 0 {
			continue
		}
		if upperBound != nil && ver.Compare(*upperBound) > 0 {
			continue
		}
		candidates = append(candidates, v)
	}

	// Walk candidates newest-first, find matches
	type match struct {
		version string
		reqs    map[string]string // dep name → requirement string
	}
	var matches []match
	const maxMatches = 5

	for i, v := range candidates {
		if len(matches) >= maxMatches {
			break
		}
		if i > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		deps, err := docs.FetchVersionDeps(crateName, v.Num)
		if err != nil {
			slog.Warn("failed to fetch deps", "version", v.Num, "error", err)
			continue
		}

		// Check all constraints
		reqs := make(map[string]string)
		allMatch := true
		for _, c := range constraints {
			found := false
			for _, d := range deps {
				if d.Name == c.name {
					found = true
					reqs[c.name] = d.Req
					if !docs.SatisfiesReq(c.version, d.Req) {
						allMatch = false
					}
					break
				}
			}
			if !found {
				allMatch = false
			}
			if !allMatch {
				break
			}
		}

		if allMatch {
			matches = append(matches, match{version: v.Num, reqs: reqs})
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No versions of %s found matching constraints.\n", crateName)
		return
	}

	// Build dep name list for header (stable order from constraints)
	depNames := make([]string, len(constraints))
	for i, c := range constraints {
		depNames[i] = c.name
	}

	// Markdown table
	var header strings.Builder
	header.WriteString(fmt.Sprintf("# %s versions with %s\n\n", crateName, strings.Join(withDeps, ", ")))
	header.WriteString("| Version |")
	for _, name := range depNames {
		header.WriteString(fmt.Sprintf(" %s requirement |", name))
	}
	header.WriteString("\n|---------|")
	for range depNames {
		header.WriteString("-----------------|")
	}
	header.WriteString("\n")
	fmt.Print(header.String())

	for _, m := range matches {
		fmt.Printf("| %s |", m.version)
		for _, name := range depNames {
			fmt.Printf(" %s |", m.reqs[name])
		}
		fmt.Println()
	}
}
