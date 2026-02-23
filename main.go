package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type RunProfile struct {
	Applications map[string]AppConfig `json:"applications"`
	Containers   ContainerConfig      `json:"containers"`
}

type AppConfig struct {
	Type             string     `json:"type"`
	Path             string     `json:"path,omitempty"`
	WorkingDirectory string     `json:"workingDirectory,omitempty"`
	LaunchURL        string     `json:"launchUrl"`
	Folder           string     `json:"folder"`
	HealthEndpoint   string     `json:"healthCheckEndpoint,omitempty"`
	Runnable         *bool      `json:"runnable,omitempty"`
	Execution        *ExecOrder `json:"execution,omitempty"`
}

type ExecOrder struct {
	Order int `json:"order"`
}

type ContainerConfig struct {
	ServiceName string                 `json:"serviceName"`
	Files       map[string]interface{} `json:"files"`
}

func findRootDir(optDir string) (string, error) {
	if optDir != "" {
		abs, err := filepath.Abs(optDir)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "etc", "abp-studio")); err != nil {
			return "", fmt.Errorf("directory %s does not contain etc/abp-studio", abs)
		}
		return abs, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "etc", "abp-studio")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find project root (looking for etc/abp-studio). Use -d to specify directory")
}

func discoverProfiles(rootDir string) ([]string, error) {
	pattern := filepath.Join(rootDir, "etc", "abp-studio", "run-profiles", "*.abprun.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no .abprun.json files found in %s", filepath.Dir(pattern))
	}
	sort.Strings(matches)
	return matches, nil
}

func loadConfig(profilePath string) (*RunProfile, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	var profile RunProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &profile, nil
}

func profileDisplayName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".abprun.json")
}

var containerNameRe = regexp.MustCompile(`(?m)^\s*container_name:\s*(.+)$`)

func containerNameFromCompose(rootDir, composeFile string) string {
	path := filepath.Join(rootDir, "etc", "docker", "containers", composeFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return strings.TrimSuffix(composeFile, ".yml")
	}
	m := containerNameRe.FindSubmatch(data)
	if m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return strings.TrimSuffix(composeFile, ".yml")
}

func buildServices(rootDir, profilePath string, profile *RunProfile) []Service {
	var services []Service

	var containerFiles []string
	for path := range profile.Containers.Files {
		containerFiles = append(containerFiles, filepath.Base(path))
	}
	sort.Strings(containerFiles)

	for _, file := range containerFiles {
		name := strings.TrimSuffix(file, ".yml")
		services = append(services, Service{
			Name:          titleCase(name),
			Kind:          KindContainer,
			Section:       "Containers",
			ComposeFile:   file,
			ContainerName: containerNameFromCompose(rootDir, file),
		})
	}

	type appEntry struct {
		name   string
		config AppConfig
		order  int
	}

	var apps, coreServices, optionalServices, gateways, cliApps []appEntry

	for name, cfg := range profile.Applications {
		order := 0
		if cfg.Execution != nil {
			order = cfg.Execution.Order
		}
		entry := appEntry{name: name, config: cfg, order: order}

		switch {
		case cfg.Type == "cli":
			cliApps = append(cliApps, entry)
		case cfg.Folder == "apps":
			apps = append(apps, entry)
		case cfg.Folder == "gateways":
			gateways = append(gateways, entry)
		case cfg.Folder == "services" && cfg.Runnable != nil && *cfg.Runnable:
			optionalServices = append(optionalServices, entry)
		case cfg.Folder == "services":
			coreServices = append(coreServices, entry)
		}
	}

	sortByOrder := func(entries []appEntry) {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].order < entries[j].order
		})
	}

	sortByOrder(apps)
	sortByOrder(coreServices)
	sortByOrder(optionalServices)
	sortByOrder(gateways)
	sortByOrder(cliApps)

	shortName := func(fullName string) string {
		parts := strings.Split(fullName, ".")
		return parts[len(parts)-1]
	}

	profileDir := filepath.Dir(profilePath)
	resolvePath := func(relative string) string {
		return filepath.Clean(filepath.Join(profileDir, relative))
	}

	for _, e := range apps {
		services = append(services, Service{
			Name:       shortName(e.name),
			Kind:       KindDotNet,
			Section:    "Applications",
			CsprojPath: resolvePath(e.config.Path),
			URL:        e.config.LaunchURL,
		})
	}

	for _, e := range cliApps {
		workDir := resolvePath(e.config.WorkingDirectory)
		services = append(services, Service{
			Name:    shortName(e.name),
			Kind:    KindCLI,
			Section: "Applications",
			WorkDir: workDir,
			URL:     e.config.LaunchURL,
		})
	}

	for _, e := range coreServices {
		services = append(services, Service{
			Name:       shortName(e.name),
			Kind:       KindDotNet,
			Section:    "Core Services",
			CsprojPath: resolvePath(e.config.Path),
			URL:        e.config.LaunchURL,
		})
	}

	for _, e := range optionalServices {
		services = append(services, Service{
			Name:       shortName(e.name),
			Kind:       KindDotNet,
			Section:    "Optional Services",
			CsprojPath: resolvePath(e.config.Path),
			URL:        e.config.LaunchURL,
		})
	}

	for _, e := range gateways {
		services = append(services, Service{
			Name:       shortName(e.name),
			Kind:       KindDotNet,
			Section:    "Gateways",
			CsprojPath: resolvePath(e.config.Path),
			URL:        e.config.LaunchURL,
		})
	}

	return services
}

func titleCase(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func projectName(rootDir string) string {
	return filepath.Base(rootDir)
}

// ── Profile selector TUI ─────────────────────────────────────

type profilePickerModel struct {
	profiles []string
	cursor   int
	chosen   string
	project  string
}

func (m profilePickerModel) Init() tea.Cmd { return nil }

func (m profilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.profiles[m.cursor]
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m profilePickerModel) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#06B6D4")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#374151")).
		Padding(0, 2).
		Render(fmt.Sprintf("%s — Select Run Profile", m.project))

	b.WriteString("\n" + header + "\n\n")

	for i, p := range m.profiles {
		name := profileDisplayName(p)
		if i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB")).Background(lipgloss.Color("#1F2937")).Render(fmt.Sprintf("  ▸ %s", name)))
		} else {
			b.WriteString(fmt.Sprintf("    %s", name))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("  ↑↓ navigate · Enter select · Q quit"))
	b.WriteString("\n")

	return b.String()
}

func selectProfile(profiles []string, project string) (string, error) {
	m := profilePickerModel{profiles: profiles, project: project}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	m, ok := result.(profilePickerModel)
	if !ok {
		return "", fmt.Errorf("no profile selected")
	}
	if m.chosen == "" {
		return "", fmt.Errorf("no profile selected")
	}
	return m.chosen, nil
}

// ── Main ──────────────────────────────────────────────────────

func main() {
	dir := flag.String("d", "", "Project root directory (must contain etc/abp-studio)")
	flag.Parse()

	rootDir, err := findRootDir(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	profiles, err := discoverProfiles(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	project := projectName(rootDir)
	var profilePath string

	if len(profiles) == 1 {
		profilePath = profiles[0]
		fmt.Printf("Using profile: %s\n", profileDisplayName(profilePath))
	} else {
		profilePath, err = selectProfile(profiles, project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	profile, err := loadConfig(profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	services := buildServices(rootDir, profilePath, profile)
	logDir := filepath.Join(rootDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	pidsFile := filepath.Join(logDir, ".pids.json")

	composeProject := strings.ToLower(profile.Containers.ServiceName)
	m := newModel(services, rootDir, logDir, pidsFile, composeProject, project)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
