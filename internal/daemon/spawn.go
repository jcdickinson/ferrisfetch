package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Spawn starts a daemon as a detached subprocess.
// It runs the same binary with the "daemon" subcommand.
func Spawn() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	cmd := exec.Command(exe, "daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Detach — don't wait for the child
	cmd.Process.Release()
	return nil
}
