package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ── Styles ────────────────────────────────────────────────────

var (
	accent  = lipgloss.Color("#06B6D4")
	green   = lipgloss.Color("#22C55E")
	red     = lipgloss.Color("#EF4444")
	yellow  = lipgloss.Color("#EAB308")
	dim     = lipgloss.Color("#6B7280")
	white   = lipgloss.Color("#F9FAFB")
	surface = lipgloss.Color("#1F2937")
	border  = lipgloss.Color("#374151")

	bannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBottom(true).
			BorderTop(true).
			BorderLeft(true).
			BorderRight(true).
			Padding(0, 2).
			Align(lipgloss.Center)

	sectionStyle = lipgloss.NewStyle().
			Foreground(dim).
			Bold(true).
			MarginLeft(2).
			MarginTop(1).
			Padding(0, 0, 0, 1)

	selectedSectionStyle = lipgloss.NewStyle().
				Bold(true).
				MarginLeft(2).
				MarginTop(1).
				Foreground(accent).
				Background(surface).
				Padding(0, 0, 0, 1)

	// Item rows: same left width so UP/DOWN don't shift when selecting (7 chars: 4 pad + " ▸ " for selected, 7 pad for unselected)
	itemStyle = lipgloss.NewStyle().
			PaddingLeft(7)

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(4).
				Bold(true).
				Background(surface).
				Foreground(white)

	// Badges with padding for alignment (status column width)
	upBadge = lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Padding(0, 1)

	downBadge = lipgloss.NewStyle().
			Foreground(red).
			Bold(true).
			Padding(0, 1)

	extBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true).
			Padding(0, 1)

	startingBadge = lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true).
			Padding(0, 1)

	retryBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F97316")).
			Bold(true).
			Padding(0, 1)

	dotUp       = lipgloss.NewStyle().Foreground(green).Render("●")
	dotDown     = lipgloss.NewStyle().Foreground(red).Render("○")
	dotExt      = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render("◆")
	dotStarting = lipgloss.NewStyle().Foreground(yellow).Render("◐")

	// Selected row: same colors but with background so text doesn't lose selection bg
	dotUpSel       = lipgloss.NewStyle().Foreground(green).Background(surface).Render("●")
	dotDownSel     = lipgloss.NewStyle().Foreground(red).Background(surface).Render("○")
	dotExtSel      = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Background(surface).Render("◆")
	dotStartingSel = lipgloss.NewStyle().Foreground(yellow).Background(surface).Render("◐")
	upBadgeSel     = lipgloss.NewStyle().Foreground(green).Bold(true).Background(surface).Padding(0, 1)
	downBadgeSel   = lipgloss.NewStyle().Foreground(red).Bold(true).Background(surface).Padding(0, 1)
	extBadgeSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Background(surface).Padding(0, 1)
	startingBadgeSel = lipgloss.NewStyle().Foreground(yellow).Bold(true).Background(surface).Padding(0, 1)
	retryBadgeSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316")).Bold(true).Background(surface).Padding(0, 1)

	helpBar = lipgloss.NewStyle().
		Foreground(dim).
		MarginTop(1).
		MarginLeft(2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#374151")).
		BorderTop(true).
		PaddingTop(1)

	helpKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Bold(true)

	statusBar = lipgloss.NewStyle().
			Foreground(yellow).
			Background(surface).
			MarginLeft(0).
			Padding(0, 2)

	// Light mode: arka plan yok, metin okunaklı
	statusBarLight = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B45309")).
			MarginLeft(0).
			Padding(0, 2)

	summaryRunning = lipgloss.NewStyle().Foreground(green).Bold(true)
	summaryDown    = lipgloss.NewStyle().Foreground(dim)

	// Padding to fill selection background to full row width
	surfacePadStyle = lipgloss.NewStyle().Background(surface)
	// Selected row prefix " ▸ " with surface bg (no padding on row style so we control full width)
	selectedPrefixStyle = lipgloss.NewStyle().Background(surface).Foreground(white).Bold(true)

	logHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBottom(true).
			PaddingBottom(0)

	// Light mode log: daha açık border (sadece header border)
	logHeaderStyleLight = logHeaderStyle.Copy().
				BorderForeground(lipgloss.Color("#9CA3AF"))
)

var (
	darkBackgroundOnce sync.Once
	cachedDarkBg       bool
)

// InitTheme detects dark/light background once. Call from main before tea.Run() so termenv doesn't run during TUI (can block).
func InitTheme() {
	_ = hasDarkBackground()
}

func hasDarkBackground() bool {
	darkBackgroundOnce.Do(func() {
		cachedDarkBg = termenv.NewOutput(os.Stdout).HasDarkBackground()
	})
	return cachedDarkBg
}

// ── Row abstraction ──────────────────────────────────────────

type rowKind int

const (
	rowSection rowKind = iota
	rowItem
)

type row struct {
	kind        rowKind
	sectionName string // for rowSection
	serviceIdx  int    // for rowItem — index into services[]
}

func buildRows(services []Service) []row {
	var rows []row
	prevSection := ""
	for i, svc := range services {
		if svc.Section != prevSection {
			prevSection = svc.Section
			rows = append(rows, row{kind: rowSection, sectionName: svc.Section})
		}
		rows = append(rows, row{kind: rowItem, serviceIdx: i, sectionName: svc.Section})
	}
	return rows
}

// ── Messages ──────────────────────────────────────────────────

type statusTickMsg time.Time
type logTickMsg time.Time
type actionDoneMsg struct {
	message        string
	startedIndices []int // indices we just started (for retry: if still Down, schedule retry)
	stoppedIndices []int // indices we just stopped (set userStopped so we don't auto-retry)
}
type retryServiceMsg int // service index to retry

// delayedRetryCheckMsg runs after a start; re-check startedIndices and schedule retry for any still Down/External (handles race where service was Starting at actionDoneMsg time)
type delayedRetryCheckMsg struct{ indices []int }

// asyncRefreshMsg is sent periodically while an async Cmd runs so the UI can re-render (don't freeze)
type asyncRefreshMsg struct{}

const (
	viewList = iota
	viewLogs
)

// ── Model ─────────────────────────────────────────────────────

type model struct {
	services       []Service
	statuses       []ServiceStatus
	prevStatuses   []ServiceStatus // for detecting transition to down
	retryCount     []int           // per-service retry attempt number (RETRY N)
	retryScheduled []bool          // retry already scheduled for this down period
	retryCancelled []bool          // user pressed Enter to stop retrying (force DOWN)
	userStopped    []bool          // user explicitly stopped this service (don't auto-retry)
	rows           []row
	cursor         int // indexes into rows[]
	scrollOffset   int
	view           int
	rootDir        string
	logDir         string
	pidsFile       string
	composeProject string
	projectName    string
	width          int
	height         int
	statusMsg      string
	logVP            viewport.Model
	logService       int
	logUserScroll    bool
	pendingAsyncCmd  tea.Cmd // işlem bitene kadar her asyncRefreshMsg'da tekrar çalıştırılır (UI kilitlenmesin)
}

func newModel(services []Service, rootDir, logDir, pidsFile, composeProject, projectName string) model {
	statuses := make([]ServiceStatus, len(services))
	for i, svc := range services {
		statuses[i] = svc.GetStatus(pidsFile)
	}
	prevStatuses := make([]ServiceStatus, len(statuses))
	copy(prevStatuses, statuses)
	return model{
		services:       services,
		statuses:       statuses,
		prevStatuses:   prevStatuses,
		retryCount:     make([]int, len(services)),
		retryScheduled: make([]bool, len(services)),
		retryCancelled: make([]bool, len(services)),
		userStopped:    make([]bool, len(services)),
		rows:           buildRows(services),
		rootDir:        rootDir,
		logDir:         logDir,
		pidsFile:       pidsFile,
		composeProject: composeProject,
		projectName:    projectName,
	}
}

func (m model) Init() tea.Cmd {
	return tickStatus()
}

func tickStatus() tea.Cmd {
	return tea.Tick(statusTickInterval, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func tickLogs() tea.Cmd {
	return tea.Tick(logTickInterval, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

const (
	statusTickInterval   = 2 * time.Second
	logTickInterval      = 2 * time.Second
	retryDelay           = 5 * time.Second
	logTailLines         = 500
	logViewChrome        = 4  // height reserved for log header/footer
	listChrome           = 10 // height reserved for summary, scroll hints, status, help
	minListVisible       = 5
	asyncRefreshInterval = 150 * time.Millisecond // UI yenileme aralığı (işlem sürerken donmasın)
)

func scheduleRetry(serviceIdx int) tea.Cmd {
	return tea.Tick(retryDelay, func(time.Time) tea.Msg {
		return retryServiceMsg(serviceIdx)
	})
}

// runAsync runs fn in a goroutine; returned Cmd yields asyncRefreshMsg periodically so UI stays responsive, then the result.
func runAsync(fn func() tea.Msg) tea.Cmd {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- fn() }()
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-time.After(asyncRefreshInterval):
			return asyncRefreshMsg{}
		}
	}
}

func scheduleDelayedRetryCheck(indices []int) tea.Cmd {
	if len(indices) == 0 {
		return nil
	}
	indicesCopy := make([]int, len(indices))
	copy(indicesCopy, indices)
	return tea.Tick(retryDelay, func(time.Time) tea.Msg {
		return delayedRetryCheckMsg{indices: indicesCopy}
	})
}

// ── Helpers ───────────────────────────────────────────────────

func (m model) cursorRow() row {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{kind: rowSection, sectionName: ""}
	}
	return m.rows[m.cursor]
}

func (m model) sectionServiceIndices(sectionName string) []int {
	var indices []int
	for i, svc := range m.services {
		if svc.Section == sectionName {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m model) sectionSummary(sectionName string) (up, ext, down int) {
	for _, idx := range m.sectionServiceIndices(sectionName) {
		switch m.statuses[idx] {
		case StatusUp:
			up++
		case StatusExternal:
			ext++
		case StatusStarting:
			down++ // starting counts as not yet running
		default:
			down++
		}
	}
	return
}

// ── Scroll ────────────────────────────────────────────────────

func (m *model) adjustScroll() {
	visible := m.listVisibleHeight()
	if visible <= 0 {
		return
	}

	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+visible {
		m.scrollOffset = m.cursor - visible + 1
	}

	total := len(m.rows)
	if m.scrollOffset > total-visible {
		m.scrollOffset = total - visible
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m model) listVisibleHeight() int {
	h := m.height - listChrome
	if m.statusMsg != "" {
		h-- // status bar bir satır kullanıyor, taşma olmasın
	}
	if h < minListVisible {
		h = minListVisible
	}
	return h
}

// ── Update ────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.view == viewLogs {
			m.logVP.Width = m.width
			m.logVP.Height = m.height - logViewChrome
		}

	case statusTickMsg:
		var retryCmds []tea.Cmd
		for i, svc := range m.services {
			newSt := svc.GetStatus(m.pidsFile)
			prev := m.prevStatuses[i]
			m.statuses[i] = newSt
			m.prevStatuses[i] = newSt
			// Schedule retry when Down or External (port open but not our process - e.g. same port as another app)
			needRetry := newSt == StatusDown || newSt == StatusExternal
			shouldRetry := needRetry && !m.retryScheduled[i] && !m.userStopped[i] &&
				((prev == StatusUp || prev == StatusStarting) || m.retryCount[i] > 0)
			if shouldRetry {
				m.retryCount[i]++
				m.retryScheduled[i] = true
				retryCmds = append(retryCmds, scheduleRetry(i))
			}
			// userStopped sadece kullanıcı servisi tekrar başlattığında temizlenir (X ile durdurulanlar retry'a düşmesin)
			if newSt == StatusUp {
				m.retryCount[i] = 0
			}
		}
		cmds := append([]tea.Cmd{tickStatus()}, retryCmds...)
		return m, tea.Batch(cmds...)

	case retryServiceMsg:
		idx := int(msg)
		if idx < 0 || idx >= len(m.services) {
			return m, nil
		}
		m.retryScheduled[idx] = false
		if m.retryCancelled[idx] {
			m.retryCancelled[idx] = false
			return m, nil
		}
		cmd := m.startService(idx)
		m.pendingAsyncCmd = cmd
		return m, cmd

	case logTickMsg:
		if m.view == viewLogs && m.logService >= 0 && m.logService < len(m.services) {
			content := m.services[m.logService].LogContent(m.rootDir, m.logDir, logTailLines)
			wasAtBottom := m.logVP.AtBottom()
			m.logVP.SetContent(content)
			if wasAtBottom || !m.logUserScroll {
				m.logVP.GotoBottom()
			}
			return m, tickLogs()
		}

	case asyncRefreshMsg:
		// UI yenilendi; işlem bitene kadar aynı Cmd'i tekrar çalıştır (donma olmasın)
		if m.pendingAsyncCmd != nil {
			return m, m.pendingAsyncCmd
		}
		return m, nil

	case actionDoneMsg:
		m.pendingAsyncCmd = nil
		m.statusMsg = msg.message
		for _, i := range msg.stoppedIndices {
			if i >= 0 && i < len(m.services) {
				m.userStopped[i] = true
			}
		}
		for _, i := range msg.startedIndices {
			if i >= 0 && i < len(m.services) {
				m.userStopped[i] = false // kullanıcı bunları başlattı, down olursa retry yapılabilsin
			}
		}
		for i, svc := range m.services {
			m.statuses[i] = svc.GetStatus(m.pidsFile)
			if m.statuses[i] == StatusUp {
				m.retryCount[i] = 0
			}
		}
		var retryCmds []tea.Cmd
		for _, i := range msg.startedIndices {
			if i < 0 || i >= len(m.services) {
				continue
			}
			st := m.statuses[i]
			if st != StatusDown && st != StatusExternal {
				continue
			}
			m.retryScheduled[i] = false
			m.retryCount[i]++
			m.retryScheduled[i] = true
			retryCmds = append(retryCmds, scheduleRetry(i))
		}
		// Delayed check: in 5s re-check startedIndices; catches services that were Starting at actionDoneMsg and then crashed
		if len(msg.startedIndices) > 0 {
			retryCmds = append(retryCmds, scheduleDelayedRetryCheck(msg.startedIndices))
		}
		if len(retryCmds) > 0 {
			return m, tea.Batch(retryCmds...)
		}

	case delayedRetryCheckMsg:
		for i, svc := range m.services {
			m.statuses[i] = svc.GetStatus(m.pidsFile)
			if m.statuses[i] == StatusUp {
				m.retryCount[i] = 0
			}
		}
		var retryCmds []tea.Cmd
		for _, i := range msg.indices {
			if i < 0 || i >= len(m.services) {
				continue
			}
			if m.userStopped[i] || m.retryScheduled[i] {
				continue
			}
			// Only schedule if we didn't already in actionDoneMsg (retryCount was 0 then → was Starting)
			if m.retryCount[i] > 0 {
				continue
			}
			st := m.statuses[i]
			if st != StatusDown && st != StatusExternal {
				continue
			}
			m.retryCount[i]++
			m.retryScheduled[i] = true
			retryCmds = append(retryCmds, scheduleRetry(i))
		}
		if len(retryCmds) > 0 {
			return m, tea.Batch(retryCmds...)
		}

	case tea.KeyMsg:
		if m.view == viewLogs {
			return m.updateLogView(msg)
		}
		return m.updateListView(msg)
	}

	return m, nil
}

func (m model) updateListView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
			m.adjustScroll()
		}

	case "home":
		m.cursor = 0
		m.adjustScroll()
	case "end":
		m.cursor = len(m.rows) - 1
		m.adjustScroll()

	case "pgup":
		visible := m.listVisibleHeight()
		m.cursor -= visible
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustScroll()
	case "pgdown":
		visible := m.listVisibleHeight()
		m.cursor += visible
		if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
		}
		m.adjustScroll()

	case "enter", " ":
		r := m.cursorRow()
		if r.kind == rowSection {
			cmd := m.toggleSection(r.sectionName)
			m.pendingAsyncCmd = cmd
			return m, cmd
		}
		if m.statuses[r.serviceIdx] != StatusDown {
			m.retryCount[r.serviceIdx] = 0
			m.userStopped[r.serviceIdx] = true
			cmd := m.stopService(r.serviceIdx)
			m.pendingAsyncCmd = cmd
			return m, cmd
		}
		// Down: if in retry cycle, Enter = cancel retries (force DOWN); else start
		if m.retryCount[r.serviceIdx] > 0 {
			m.retryCount[r.serviceIdx] = 0
			m.retryScheduled[r.serviceIdx] = false
			m.retryCancelled[r.serviceIdx] = true
			m.statusMsg = fmt.Sprintf("▼ %s — retry cancelled", m.services[r.serviceIdx].Name)
			return m, nil
		}
		m.retryCancelled[r.serviceIdx] = false
		m.userStopped[r.serviceIdx] = false // kullanıcı tekrar başlattı, down olursa retry yapılabilsin
		cmd := m.startService(r.serviceIdx)
		m.pendingAsyncCmd = cmd
		return m, cmd

	case "l":
		r := m.cursorRow()
		if r.kind != rowItem {
			m.statusMsg = "✗ Select a service to view logs"
			return m, nil
		}
		m.view = viewLogs
		m.logService = r.serviceIdx
		m.logUserScroll = false
		m.logVP = viewport.New(m.width, m.height-logViewChrome)
		content := m.services[r.serviceIdx].LogContent(m.rootDir, m.logDir, logTailLines)
		m.logVP.SetContent(content)
		m.logVP.GotoBottom()
		return m, tickLogs()

	case "o":
		r := m.cursorRow()
		if r.kind != rowItem {
			m.statusMsg = "✗ Select a service to open"
			return m, nil
		}
		svc := m.services[r.serviceIdx]
		if svc.URL != "" {
			openBrowser(svc.URL)
			m.statusMsg = fmt.Sprintf("⇗ Opened %s", svc.URL)
		} else {
			m.statusMsg = "✗ No URL for this service"
		}

	case "K":
		r := m.cursorRow()
		if r.kind != rowItem {
			m.statusMsg = "✗ Select a service to kill port"
			return m, nil
		}
		svc := m.services[r.serviceIdx]
		if svc.URL != "" {
			cmd := m.killPortCmd(r.serviceIdx)
			m.pendingAsyncCmd = cmd
			return m, cmd
		}
		m.statusMsg = "✗ No port to kill for this service"

	case "f":
		r := m.cursorRow()
		if r.kind != rowItem {
			m.statusMsg = "✗ Select a service to open folder"
			return m, nil
		}
		svc := m.services[r.serviceIdx]
		dir := svc.WorkingDirectory(m.rootDir)
		if err := openFileExplorer(dir); err != nil {
			m.statusMsg = fmt.Sprintf("✗ Open folder: %v", err)
		} else {
			m.statusMsg = fmt.Sprintf("📂 %s → %s", svc.Name, dir)
		}
		return m, nil

	case "s":
		cmd := m.startAll()
		m.pendingAsyncCmd = cmd
		return m, cmd
	case "x":
		// Tüm retry'ları iptal et, tüm servisleri durdur, bir daha retry etme
		for i := range m.services {
			m.retryScheduled[i] = false
			m.retryCount[i] = 0
			m.retryCancelled[i] = true
			if m.statuses[i] != StatusDown {
				m.userStopped[i] = true
			}
		}
		cmd := m.stopAll()
		m.pendingAsyncCmd = cmd
		return m, cmd
	}

	return m, nil
}

func (m model) updateLogView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.view = viewList
		return m, nil
	case "up", "k", "pgup":
		m.logUserScroll = true
	case "G", "g", "end":
		m.logUserScroll = false
	}
	var cmd tea.Cmd
	m.logVP, cmd = m.logVP.Update(msg)
	return m, cmd
}

// ── Commands ──────────────────────────────────────────────────

func (m model) startService(idx int) tea.Cmd {
	svc := m.services[idx]
	rootDir, logDir, pidsFile, composeProject := m.rootDir, m.logDir, m.pidsFile, m.composeProject
	return runAsync(func() tea.Msg {
		err := svc.Start(rootDir, logDir, pidsFile, composeProject)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("▲ Started %s", svc.Name), startedIndices: []int{idx}}
	})
}

func (m model) stopService(idx int) tea.Cmd {
	svc := m.services[idx]
	rootDir, pidsFile, composeProject := m.rootDir, m.pidsFile, m.composeProject
	return runAsync(func() tea.Msg {
		err := svc.Stop(rootDir, pidsFile, composeProject)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("▼ Stopped %s", svc.Name), stoppedIndices: []int{idx}}
	})
}

func (m model) toggleSection(sectionName string) tea.Cmd {
	indices := m.sectionServiceIndices(sectionName)
	var toStart, toStop []int
	for _, idx := range indices {
		if m.statuses[idx] == StatusDown {
			toStart = append(toStart, idx)
		} else {
			toStop = append(toStop, idx)
		}
	}
	rootDir := m.rootDir
	logDir := m.logDir
	pidsFile := m.pidsFile
	composeProject := m.composeProject
	services := m.services

	return runAsync(func() tea.Msg {
		if len(toStart) > 0 {
			for _, idx := range toStart {
				services[idx].Start(rootDir, logDir, pidsFile, composeProject)
			}
			return actionDoneMsg{
				message:        fmt.Sprintf("▲ Started %d services in %s", len(toStart), sectionName),
				startedIndices: toStart,
			}
		}
		for _, idx := range toStop {
			services[idx].Stop(rootDir, pidsFile, composeProject)
		}
		return actionDoneMsg{
			message:        fmt.Sprintf("▼ Stopped %d services in %s", len(toStop), sectionName),
			stoppedIndices: toStop,
		}
	})
}

func (m model) killPortCmd(idx int) tea.Cmd {
	svc := m.services[idx]
	return runAsync(func() tea.Msg {
		msg, err := killPort(svc.URL)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("☠ %s: %s", svc.Name, msg)}
	})
}

func (m model) startAll() tea.Cmd {
	var toStart []int
	for i := range m.services {
		if m.statuses[i] == StatusDown {
			toStart = append(toStart, i)
		}
	}
	rootDir := m.rootDir
	logDir := m.logDir
	pidsFile := m.pidsFile
	composeProject := m.composeProject
	services := m.services

	return runAsync(func() tea.Msg {
		for _, idx := range toStart {
			services[idx].Start(rootDir, logDir, pidsFile, composeProject)
		}
		return actionDoneMsg{
			message:        fmt.Sprintf("▲ Started %d services", len(toStart)),
			startedIndices: toStart,
		}
	})
}

func (m model) stopAll() tea.Cmd {
	rootDir := m.rootDir
	pidsFile := m.pidsFile
	composeProject := m.composeProject
	services := m.services
	return runAsync(func() tea.Msg {
		var stopped []int
		for i, svc := range services {
			if m.statuses[i] != StatusDown {
				svc.Stop(rootDir, pidsFile, composeProject)
				stopped = append(stopped, i)
			}
		}
		return actionDoneMsg{message: fmt.Sprintf("▼ Stopped %d services", len(stopped)), stoppedIndices: stopped}
	})
}

// ── View ──────────────────────────────────────────────────────

func (m model) View() string {
	if m.view == viewLogs {
		return m.viewLogScreen()
	}
	return m.viewListScreen()
}

func (m model) viewListScreen() string {
	var b strings.Builder

	w := m.width
	if w < 50 {
		w = 60
	}

	upCount := 0
	extCount := 0
	downCount := 0
	for _, s := range m.statuses {
		switch s {
		case StatusUp:
			upCount++
		case StatusExternal:
			extCount++
		default:
			downCount++
		}
	}
	running := upCount + extCount
	summaryParts := []string{
		summaryRunning.Render(fmt.Sprintf("● %d running", running)),
		summaryDown.Render(fmt.Sprintf("○ %d down", downCount)),
	}
	if extCount > 0 {
		summaryParts = append(summaryParts, summaryDown.Render(fmt.Sprintf("◆ %d external", extCount)))
	}
	summary := lipgloss.NewStyle().Foreground(dim).MarginLeft(2).Render(
		strings.Join(summaryParts, "   "),
	)
	b.WriteString(summary + "\n")

	type renderedLine struct {
		text string
	}

	var allLines []renderedLine
	for i, r := range m.rows {
		selected := i == m.cursor

		if r.kind == rowSection {
			up, ext, dn := m.sectionSummary(r.sectionName)
			total := up + ext + dn
			running := up + ext
			badge := fmt.Sprintf(" (%d/%d)", running, total)

			if selected {
				allLines = append(allLines, renderedLine{
					text: selectedSectionStyle.Width(w - 2).Render(" ▸ " + strings.ToUpper(r.sectionName) + badge),
				})
			} else {
				allLines = append(allLines, renderedLine{
					text: sectionStyle.Render(strings.ToUpper(r.sectionName) + badge),
				})
			}
			continue
		}

		svc := m.services[r.serviceIdx]
		st := m.statuses[r.serviceIdx]

		var dot, badgeText, nameStr, badgeStr string
		nameStr = fmt.Sprintf("%-30s", svc.Name)
		if selected {
			nameStr = lipgloss.NewStyle().Background(surface).Foreground(white).Render(nameStr)
			switch st {
			case StatusUp:
				dot, badgeStr = dotUpSel, "  UP"
				badgeText = upBadgeSel.Render(badgeStr)
			case StatusExternal:
				dot = dotExtSel
				if n := m.retryCount[r.serviceIdx]; n > 0 {
					badgeStr = fmt.Sprintf("RETRY (%d)", n)
					badgeText = retryBadgeSel.Render(badgeStr)
				} else {
					badgeStr = " EXT"
					badgeText = extBadgeSel.Render(badgeStr)
				}
			case StatusStarting:
				dot, badgeStr = dotStartingSel, "STARTING"
				badgeText = startingBadgeSel.Render(badgeStr)
			default:
				dot = dotDownSel
				if n := m.retryCount[r.serviceIdx]; n > 0 {
					badgeStr = fmt.Sprintf("RETRY (%d)", n)
					badgeText = retryBadgeSel.Render(badgeStr)
				} else {
					badgeStr = "DOWN"
					badgeText = downBadgeSel.Render(badgeStr)
				}
			}
		} else {
			switch st {
			case StatusUp:
				dot = dotUp
				badgeText = upBadge.Render("  UP")
			case StatusExternal:
				dot = dotExt
				if n := m.retryCount[r.serviceIdx]; n > 0 {
					badgeText = retryBadge.Render(fmt.Sprintf("RETRY (%d)", n))
				} else {
					badgeText = extBadge.Render(" EXT")
				}
			case StatusStarting:
				dot = dotStarting
				badgeText = startingBadge.Render("STARTING")
			default:
				dot = dotDown
				if n := m.retryCount[r.serviceIdx]; n > 0 {
					badgeText = retryBadge.Render(fmt.Sprintf("RETRY (%d)", n))
				} else {
					badgeText = downBadge.Render("DOWN")
				}
			}
		}

		if selected {
			// Circle–isim–status arasındaki boşluklar da surface arka planı alsın (aralar background color işlesin).
			line := dot + surfacePadStyle.Render("  ") + nameStr + surfacePadStyle.Render("  ") + badgeText
			// Satırı baştan sona surface ile doldur (sol/sağ padding dahil).
			contentWidth := 4 + 3 + 1 + 2 + 30 + 2 + len(badgeStr) + 2
			pad := w - contentWidth
			if pad < 0 {
				pad = 0
			}
			fullLine := surfacePadStyle.Render("    ") +
				selectedPrefixStyle.Render(" ▸ ") +
				line +
				surfacePadStyle.Render(strings.Repeat(" ", pad))
			allLines = append(allLines, renderedLine{text: fullLine})
		} else {
			line := fmt.Sprintf("%s  %s  %s", dot, nameStr, badgeText)
			allLines = append(allLines, renderedLine{text: itemStyle.Render(line)})
		}
	}

	visible := m.listVisibleHeight()
	total := len(allLines)
	offset := m.scrollOffset
	if offset > total-visible {
		offset = total - visible
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + visible
	// İlk görünen satır item ise grup başlığını sticky göster; alttaki satırı kesmemek için sadece "ortada" iken
	needStickySection := offset > 0 && offset < len(m.rows) && m.rows[offset].kind == rowItem && (offset+visible < total)
	if needStickySection {
		end = offset + visible - 1
	}
	if end > total {
		end = total
	}

	if offset > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).MarginLeft(4).Render(fmt.Sprintf("↑ %d above", offset)) + "\n")
	}
	if needStickySection {
		sectionName := ""
		for i := offset - 1; i >= 0; i-- {
			if m.rows[i].kind == rowSection {
				sectionName = m.rows[i].sectionName
				break
			}
		}
		if sectionName != "" {
			up, ext, dn := m.sectionSummary(sectionName)
			running := up + ext
			totalSec := up + ext + dn
			badge := fmt.Sprintf(" (%d/%d)", running, totalSec)
			b.WriteString(sectionStyle.Render(strings.ToUpper(sectionName)+badge) + "\n")
		}
	}
	for i := offset; i < end; i++ {
		b.WriteString(allLines[i].text + "\n")
	}

	if end < total {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).MarginLeft(4).Render(fmt.Sprintf("↓ %d below", total-end)) + "\n")
	}

	b.WriteString("\n")
	if m.statusMsg != "" {
		sb := statusBar
		if !hasDarkBackground() {
			sb = statusBarLight
		}
		b.WriteString(sb.Render(" "+m.statusMsg) + "\n")
	}

	appTitle := lipgloss.NewStyle().Bold(true).Foreground(accent).Render("abp-msx — " + m.projectName)
	help1 := fmt.Sprintf("%s nav  %s start/stop  %s logs  %s open  %s kill  %s folder",
		helpKey.Render("↑↓"),
		helpKey.Render("Enter"),
		helpKey.Render("L"),
		helpKey.Render("O"),
		helpKey.Render("K"),
		helpKey.Render("F"),
	)
	help2 := fmt.Sprintf("%s start all  %s stop all  %s quit",
		helpKey.Render("S"),
		helpKey.Render("X"),
		helpKey.Render("q"),
	)
	b.WriteString(helpBar.Render(" "+appTitle+"  ·  "+help1+"  ·  "+help2) + "\n")

	return b.String()
}

func (m model) viewLogScreen() string {
	var b strings.Builder
	dark := hasDarkBackground()
	headerStyle := logHeaderStyle
	primaryFg := white
	if !dark {
		headerStyle = logHeaderStyleLight
		primaryFg = lipgloss.Color("#111827")
	}
	if m.logService < 0 || m.logService >= len(m.services) {
		b.WriteString(headerStyle.Render("  Invalid service · q back") + "\n")
		return b.String()
	}
	svc := m.services[m.logService]
	st := m.statuses[m.logService]
	var statusDot string
	switch st {
	case StatusUp:
		statusDot = dotUp
	case StatusExternal:
		statusDot = dotExt
	case StatusStarting:
		statusDot = dotStarting
	default:
		statusDot = dotDown
	}

	scrollHint := ""
	if m.logUserScroll {
		scrollHint = "  " + lipgloss.NewStyle().Foreground(yellow).Render("⏸ paused · G resume")
	}
	header := fmt.Sprintf("  %s %s  %s%s",
		statusDot,
		lipgloss.NewStyle().Bold(true).Foreground(primaryFg).Render(svc.Name),
		lipgloss.NewStyle().Foreground(dim).Render("q back · ↑↓ scroll"),
		scrollHint,
	)
	b.WriteString(headerStyle.Render(header) + "\n")
	b.WriteString(m.logVP.View())
	b.WriteString("\n")

	return b.String()
}
