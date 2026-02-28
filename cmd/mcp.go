package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

//go:embed mcp_prelude.md
var mcpPrelude string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as MCP server (publishes CLI instructions only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := binaryName()
		instructions := fmt.Sprintf(mcpPrelude, name) + agentHelp

		s := server.NewMCPServer("rsdoc", "1.0.0",
			server.WithInstructions(instructions),
		)
		return server.ServeStdio(s)
	},
}

// binaryName returns "rsdoc" if it's in PATH and points to the current binary,
// otherwise returns the full path to the binary.
func binaryName() string {
	exe, err := os.Executable()
	if err != nil {
		return "rsdoc"
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "rsdoc"
	}

	rsdocPath, err := exec.LookPath("rsdoc")
	if err == nil {
		resolved, err := filepath.EvalSymlinks(rsdocPath)
		if err == nil && resolved == exe {
			return "rsdoc"
		}
	}

	return exe
}
