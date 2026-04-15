package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

//go:embed mcp_prelude.md
var mcpPrelude string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as MCP server (publishes CLI instructions only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		instructions := usageInstructions()

		s := server.NewMCPServer("rsdoc", "1.0.0",
			server.WithInstructions(instructions),
		)
		s.AddTool(
			mcp.NewTool("usage",
				mcp.WithDescription("Return the server usage instructions. Read this first. rsdoc only applies to Rust crates from crates.io/docs.rs, not npm or other ecosystems."),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText(instructions), nil
			},
		)
		return server.ServeStdio(s)
	},
}

func usageInstructions() string {
	return fmt.Sprintf(mcpPrelude, binaryName()) + agentHelp
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
