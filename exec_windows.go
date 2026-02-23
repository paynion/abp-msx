//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	// Windows: process group kill is done via taskkill /T
}

func killProcessGroup(pid int) error {
	// /T = kill child processes too
	out, err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", string(out))
	}
	return nil
}

func isProcessAlive(pid int) bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, _, _ := openProcess.Call(0x1000, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	syscall.CloseHandle(syscall.Handle(h))
	return true
}

func killPort(rawURL string) (string, error) {
	port := portFromURL(rawURL)
	if port == "" {
		return "", fmt.Errorf("could not extract port from URL")
	}
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return "", fmt.Errorf("netstat: %w", err)
	}
	var pids []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":"+port) || !strings.Contains(line, "LISTENING") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid := fields[len(fields)-1]
		if pid != "0" && !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return "", fmt.Errorf("no process found on port %s", port)
	}
	for _, p := range pids {
		exec.Command("taskkill", "/PID", p, "/F").Run()
	}
	return fmt.Sprintf("killed %d process(es) on port %s (PIDs: %s)", len(pids), port, strings.Join(pids, ", ")), nil
}
