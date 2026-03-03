package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jcdickinson/ferrisfetch/internal/docs"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <crate[@version]>",
	Short: "Show crate metadata from crates.io",
	Example: `  rsdoc info serde
  rsdoc info tokio@1.44.2`,
	Args: cobra.ExactArgs(1),
	Run:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) {
	name, version := parseCrateArg(args[0])

	info, err := docs.FetchCrateInfo(name)
	if err != nil {
		slog.Error("failed to fetch crate info", "error", err)
		os.Exit(1)
	}

	// Find the target version (default: latest non-yanked)
	var target *docs.VersionInfo
	if version != "" && version != "latest" {
		for i := range info.Versions {
			if info.Versions[i].Num == version {
				target = &info.Versions[i]
				break
			}
		}
		if target == nil {
			slog.Error("version not found", "crate", name, "version", version)
			os.Exit(1)
		}
	} else {
		for i := range info.Versions {
			if !info.Versions[i].Yanked {
				target = &info.Versions[i]
				break
			}
		}
		if target == nil {
			slog.Error("no non-yanked versions found", "crate", name)
			os.Exit(1)
		}
	}

	// Markdown output
	fmt.Printf("# %s %s\n\n", info.Name, target.Num)
	if info.Description != "" {
		fmt.Printf("%s\n\n", info.Description)
	}

	if info.License != "" {
		fmt.Printf("- **License:** %s\n", info.License)
	}
	if target.MSRV != "" {
		fmt.Printf("- **MSRV:** %s\n", target.MSRV)
	}
	fmt.Printf("- **Downloads:** %s\n", formatNumber(info.Downloads))
	if info.Homepage != "" {
		fmt.Printf("- **Homepage:** %s\n", info.Homepage)
	}
	if info.Repository != "" {
		fmt.Printf("- **Repository:** %s\n", info.Repository)
	}
	if len(info.Keywords) > 0 {
		fmt.Printf("- **Keywords:** %s\n", strings.Join(info.Keywords, ", "))
	}
	fmt.Println()
}

// parseCrateArg splits "crate@version" into (name, version).
func parseCrateArg(arg string) (string, string) {
	if idx := strings.IndexByte(arg, '@'); idx >= 0 {
		return arg[:idx], arg[idx+1:]
	}
	return arg, ""
}

// formatNumber formats an integer with commas (e.g. 1,234,567).
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
