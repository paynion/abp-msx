package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type ServiceKind string

const (
	KindContainer ServiceKind = "container"
	KindDotNet    ServiceKind = "dotnet"
	KindCLI       ServiceKind = "cli"
)

type Service struct {
	Name          string
	Kind          ServiceKind
	Section       string
	ComposeFile   string // container
	ContainerName string // container
	CsprojPath    string // dotnet
	URL           string // dotnet launch url / cli url
	WorkDir       string // cli
}

type ServiceStatus int

const (
	StatusDown     ServiceStatus = iota
	StatusStarting               // process started but not yet reachable (e.g. still building)
	StatusUp                     // started by us and reachable
	StatusExternal               // port is open but not our PID
)

func (s Service) GetStatus(pidsFile string) ServiceStatus {
	switch s.Kind {
	case KindContainer:
		if containerIsRunning(s.ContainerName) {
			return StatusUp
		}
		return StatusDown
	case KindDotNet, KindCLI:
		processRunning := processIsRunning(pidsFile, s.Name)
		if s.URL != "" {
			portOpen := isPortOpen(s.URL)
			if processRunning && portOpen {
				return StatusUp
			}
			if processRunning && !portOpen {
				return StatusStarting
			}
			if !processRunning && portOpen {
				return StatusExternal
			}
			return StatusDown
		}
		if processRunning {
			return StatusUp
		}
		return StatusDown
	}
	return StatusDown
}

func (s Service) IsUp(pidsFile string) bool {
	st := s.GetStatus(pidsFile)
	return st == StatusUp || st == StatusExternal
}

// WorkingDirectory returns the filesystem path to open in a file explorer for this service.
func (s Service) WorkingDirectory(rootDir string) string {
	switch s.Kind {
	case KindDotNet:
		return filepath.Dir(s.CsprojPath)
	case KindCLI:
		return s.WorkDir
	case KindContainer:
		return filepath.Join(rootDir, "etc", "docker", "containers")
	}
	return rootDir
}

func (s Service) Start(rootDir, logDir, pidsFile, composeProject string) error {
	switch s.Kind {
	case KindContainer:
		return startContainer(rootDir, s.ComposeFile, composeProject)
	case KindDotNet:
		return startDotNet(rootDir, logDir, pidsFile, s.Name, s.CsprojPath, s.URL)
	case KindCLI:
		return startCLI(logDir, pidsFile, s.Name, s.WorkDir)
	}
	return nil
}

func (s Service) Stop(rootDir, pidsFile, composeProject string) error {
	switch s.Kind {
	case KindContainer:
		return stopContainer(rootDir, s.ComposeFile, composeProject)
	case KindDotNet, KindCLI:
		return stopProcess(pidsFile, s.Name)
	}
	return nil
}

func (s Service) LogContent(rootDir, logDir string, lines int) string {
	switch s.Kind {
	case KindContainer:
		out, _ := exec.Command("docker", "logs", "--tail", strconv.Itoa(lines), s.ContainerName).CombinedOutput()
		return string(out)
	case KindDotNet, KindCLI:
		logFile := filepath.Join(logDir, s.Name+".log")
		return tailFile(logFile, lines)
	}
	return ""
}

// ── Container management ──────────────────────────────────────

func containerIsRunning(name string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", name).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "running"
}

func runWithOutput(cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%s", lastMeaningfulLine(msg))
		}
		return err
	}
	return nil
}

func lastMeaningfulLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			return l
		}
	}
	return s
}

func startContainer(rootDir, composeFile, project string) error {
	dockerDir := filepath.Join(rootDir, "etc", "docker")

	exec.Command("docker", "network", "create", "tcmb", "--label=tcmb").Run()

	cmd := exec.Command("docker", "compose", "-p", project, "-f", filepath.Join("containers", composeFile), "up", "-d")
	cmd.Dir = dockerDir
	return runWithOutput(cmd)
}

func stopContainer(rootDir, composeFile, project string) error {
	dockerDir := filepath.Join(rootDir, "etc", "docker")
	cmd := exec.Command("docker", "compose", "-p", project, "-f", filepath.Join("containers", composeFile), "down")
	cmd.Dir = dockerDir
	return runWithOutput(cmd)
}

// ── Process management ────────────────────────────────────────

func loadPids(path string) map[string]int {
	pids := make(map[string]int)
	data, err := os.ReadFile(path)
	if err != nil {
		return pids
	}
	if err := json.Unmarshal(data, &pids); err != nil {
		return pids
	}
	return pids
}

func savePids(path string, pids map[string]int) error {
	data, err := json.MarshalIndent(pids, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func processIsRunning(pidsFile, name string) bool {
	pids := loadPids(pidsFile)
	pid, ok := pids[name]
	if !ok {
		return false
	}
	return isProcessAlive(pid)
}

func startDotNet(rootDir, logDir, pidsFile, name, csprojPath, url string) error {
	logFile := filepath.Join(logDir, name+".log")
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	cmd := exec.Command("dotnet", "run", "--project", csprojPath, "--no-launch-profile")
	cmd.Env = append(os.Environ(),
		"ASPNETCORE_ENVIRONMENT=Development",
		"ASPNETCORE_URLS="+url,
	)
	cmd.Stdout = f
	cmd.Stderr = f
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		f.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}

	go func() {
		cmd.Wait()
		f.Close()
	}()

	pids := loadPids(pidsFile)
	pids[name] = cmd.Process.Pid
	return savePids(pidsFile, pids)
}

func startCLI(logDir, pidsFile, name, workDir string) error {
	logFile := filepath.Join(logDir, name+".log")
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	cmd := exec.Command("yarn", "start")
	cmd.Dir = workDir
	cmd.Stdout = f
	cmd.Stderr = f
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		f.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}

	go func() {
		cmd.Wait()
		f.Close()
	}()

	pids := loadPids(pidsFile)
	pids[name] = cmd.Process.Pid
	return savePids(pidsFile, pids)
}

func stopProcess(pidsFile, name string) error {
	pids := loadPids(pidsFile)
	pid, ok := pids[name]
	if !ok {
		return nil
	}

	if err := killProcessGroup(pid); err != nil {
		return err
	}

	delete(pids, name)
	return savePids(pidsFile, pids)
}

// ── Port utilities ────────────────────────────────────────────

func portFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return port
}

const portCheckTimeout = 500 * time.Millisecond

func isPortOpen(rawURL string) bool {
	port := portFromURL(rawURL)
	if port == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", "localhost:"+port, portCheckTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func openFileExplorer(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", absDir)
	case "linux":
		cmd = exec.Command("xdg-open", absDir)
	case "windows":
		cmd = exec.Command("explorer", absDir)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// ── Log reading ───────────────────────────────────────────────

func tailFile(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(no log file: %s)", filepath.Base(path))
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
