//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func killPort(rawURL string) (string, error) {
	port := portFromURL(rawURL)
	if port == "" {
		return "", fmt.Errorf("could not extract port from URL")
	}
	out, err := exec.Command("lsof", "-ti", "tcp:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		return "", fmt.Errorf("no process found on port %s", port)
	}
	pids := strings.Fields(strings.TrimSpace(string(out)))
	if len(pids) == 0 {
		return "", fmt.Errorf("no process found on port %s", port)
	}
	for _, p := range pids {
		exec.Command("kill", "-9", p).Run()
	}
	return fmt.Sprintf("killed %d process(es) on port %s (PIDs: %s)", len(pids), port, strings.Join(pids, ", ")), nil
}
