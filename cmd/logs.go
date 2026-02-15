package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon log file",
	Run:   runLogs,
}

var (
	logsFollow bool
	logsLines  int
)

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "number of lines to show")
}

func runLogs(cmd *cobra.Command, args []string) {
	logPath := config.LogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Println("no log file found (daemon may not have run yet)")
		return
	}

	tailArgs := []string{"-n", strconv.Itoa(logsLines)}
	if logsFollow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, logPath)

	tailCmd := exec.Command("tail", tailArgs...)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr

	if err := tailCmd.Run(); err != nil {
		log.Fatalf("tail failed: %v", err)
	}
}
