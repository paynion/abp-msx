//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
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
