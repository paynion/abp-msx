package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	bannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Padding(0, 2).
			Align(lipgloss.Center)

	sectionStyle = lipgloss.NewStyle().
			Foreground(dim).
			Bold(true).
			MarginLeft(2).
			MarginTop(1)

	selectedSectionStyle = lipgloss.NewStyle().
				Bold(true).
				MarginLeft(1).
				Foreground(accent).
				Background(surface)

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Bold(true).
				Background(surface).
				Foreground(white)

	upBadge = lipgloss.NewStyle().
		Foreground(green).
		Bold(true)

	downBadge = lipgloss.NewStyle().
			Foreground(red).
			Bold(true)

	extBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	dotUp   = lipgloss.NewStyle().Foreground(green).Render("●")
	dotDown = lipgloss.NewStyle().Foreground(red).Render("○")
	dotExt  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render("◆")

	helpBar = lipgloss.NewStyle().
		Foreground(dim).
		MarginTop(1).
		MarginLeft(2)

	helpKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Bold(true)

	statusBar = lipgloss.NewStyle().
			Foreground(yellow).
			MarginLeft(2)

	logHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Padding(0, 1)

	logBorderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151"))
)

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
type actionDoneMsg struct{ message string }

const (
	viewList = iota
	viewLogs
)

// ── Model ─────────────────────────────────────────────────────

type model struct {
	services       []Service
	statuses       []ServiceStatus
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
	logVP          viewport.Model
	logService     int
	logUserScroll  bool
}

func newModel(services []Service, rootDir, logDir, pidsFile, composeProject, projectName string) model {
	statuses := make([]ServiceStatus, len(services))
	for i, svc := range services {
		statuses[i] = svc.GetStatus(pidsFile)
	}
	return model{
		services:       services,
		statuses:       statuses,
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
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func tickLogs() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

// ── Helpers ───────────────────────────────────────────────────

func (m model) cursorRow() row {
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
	chrome := 10
	h := m.height - chrome
	if h < 5 {
		h = 5
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
			m.logVP.Height = m.height - 4
		}

	case statusTickMsg:
		for i, svc := range m.services {
			m.statuses[i] = svc.GetStatus(m.pidsFile)
		}
		return m, tickStatus()

	case logTickMsg:
		if m.view == viewLogs {
			content := m.services[m.logService].LogContent(m.rootDir, m.logDir, 500)
			wasAtBottom := m.logVP.AtBottom()
			m.logVP.SetContent(content)
			if wasAtBottom || !m.logUserScroll {
				m.logVP.GotoBottom()
			}
			return m, tickLogs()
		}

	case actionDoneMsg:
		m.statusMsg = msg.message
		for i, svc := range m.services {
			m.statuses[i] = svc.GetStatus(m.pidsFile)
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
			return m, m.toggleSection(r.sectionName)
		}
		if m.statuses[r.serviceIdx] != StatusDown {
			return m, m.stopService(r.serviceIdx)
		}
		return m, m.startService(r.serviceIdx)

	case "l":
		r := m.cursorRow()
		if r.kind != rowItem {
			m.statusMsg = "✗ Select a service to view logs"
			return m, nil
		}
		m.view = viewLogs
		m.logService = r.serviceIdx
		m.logUserScroll = false
		m.logVP = viewport.New(m.width, m.height-4)
		content := m.services[r.serviceIdx].LogContent(m.rootDir, m.logDir, 500)
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
			return m, m.killPortCmd(r.serviceIdx)
		}
		m.statusMsg = "✗ No port to kill for this service"

	case "s":
		return m, m.startAll()
	case "x":
		return m, m.stopAll()
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
	case "G", "end":
		m.logUserScroll = false
	}
	var cmd tea.Cmd
	m.logVP, cmd = m.logVP.Update(msg)
	return m, cmd
}

// ── Commands ──────────────────────────────────────────────────

func (m model) startService(idx int) tea.Cmd {
	svc := m.services[idx]
	return func() tea.Msg {
		err := svc.Start(m.rootDir, m.logDir, m.pidsFile, m.composeProject)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("▲ Started %s", svc.Name)}
	}
}

func (m model) stopService(idx int) tea.Cmd {
	svc := m.services[idx]
	return func() tea.Msg {
		err := svc.Stop(m.rootDir, m.pidsFile, m.composeProject)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("▼ Stopped %s", svc.Name)}
	}
}

func (m model) toggleSection(sectionName string) tea.Cmd {
	indices := m.sectionServiceIndices(sectionName)
	allUp := true
	for _, idx := range indices {
		if m.statuses[idx] == StatusDown {
			allUp = false
			break
		}
	}

	return func() tea.Msg {
		count := 0
		if allUp {
			for _, idx := range indices {
				if m.statuses[idx] != StatusDown {
					m.services[idx].Stop(m.rootDir, m.pidsFile, m.composeProject)
					count++
				}
			}
			return actionDoneMsg{message: fmt.Sprintf("▼ Stopped %d services in %s", count, sectionName)}
		}
		for _, idx := range indices {
			if m.statuses[idx] == StatusDown {
				m.services[idx].Start(m.rootDir, m.logDir, m.pidsFile, m.composeProject)
				count++
			}
		}
		return actionDoneMsg{message: fmt.Sprintf("▲ Started %d services in %s", count, sectionName)}
	}
}

func (m model) killPortCmd(idx int) tea.Cmd {
	svc := m.services[idx]
	return func() tea.Msg {
		msg, err := killPort(svc.URL)
		if err != nil {
			return actionDoneMsg{message: fmt.Sprintf("✗ %s: %v", svc.Name, err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("☠ %s: %s", svc.Name, msg)}
	}
}

func (m model) startAll() tea.Cmd {
	return func() tea.Msg {
		count := 0
		for i, svc := range m.services {
			if m.statuses[i] == StatusDown {
				svc.Start(m.rootDir, m.logDir, m.pidsFile, m.composeProject)
				count++
			}
		}
		return actionDoneMsg{message: fmt.Sprintf("▲ Started %d services", count)}
	}
}

func (m model) stopAll() tea.Cmd {
	return func() tea.Msg {
		count := 0
		for i, svc := range m.services {
			if m.statuses[i] != StatusDown {
				svc.Stop(m.rootDir, m.pidsFile, m.composeProject)
				count++
			}
		}
		return actionDoneMsg{message: fmt.Sprintf("▼ Stopped %d services", count)}
	}
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

	banner := bannerStyle.Width(w - 4).Render("abp-msx — " + m.projectName)
	b.WriteString("\n" + banner + "\n")

	upCount := 0
	extCount := 0
	for _, s := range m.statuses {
		if s == StatusUp {
			upCount++
		} else if s == StatusExternal {
			extCount++
		}
	}
	summaryText := fmt.Sprintf("  %d/%d running", upCount+extCount, len(m.services))
	if extCount > 0 {
		summaryText += fmt.Sprintf("  (%d external)", extCount)
	}
	summary := lipgloss.NewStyle().Foreground(dim).MarginLeft(2).Render(summaryText)
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
					text: selectedSectionStyle.Render(" ▸ " + strings.ToUpper(r.sectionName) + badge),
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

		var dot, badgeText string
		switch st {
		case StatusUp:
			dot = dotUp
			badgeText = upBadge.Render("  UP")
		case StatusExternal:
			dot = dotExt
			badgeText = extBadge.Render(" EXT")
		default:
			dot = dotDown
			badgeText = downBadge.Render("DOWN")
		}

		name := fmt.Sprintf("%-28s", svc.Name)
		line := fmt.Sprintf("%s  %s  %s", dot, name, badgeText)

		if selected {
			allLines = append(allLines, renderedLine{text: selectedItemStyle.Render(" ▸ " + line)})
		} else {
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
	if end > total {
		end = total
	}

	if offset > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).MarginLeft(4).Render(fmt.Sprintf("  ▲ %d more above", offset)) + "\n")
	}

	for i := offset; i < end; i++ {
		b.WriteString(allLines[i].text + "\n")
	}

	if end < total {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).MarginLeft(4).Render(fmt.Sprintf("  ▼ %d more below", total-end)) + "\n")
	}

	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(statusBar.Render(m.statusMsg) + "\n")
	}

	help := fmt.Sprintf(
		"  %s navigate  %s start/stop (group on section)  %s logs  %s open  %s kill port\n  %s start all  %s stop all  %s quit",
		helpKey.Render("↑↓"),
		helpKey.Render("Enter"),
		helpKey.Render("L"),
		helpKey.Render("O"),
		helpKey.Render("Shift+K"),
		helpKey.Render("S"),
		helpKey.Render("X"),
		helpKey.Render("Q"),
	)
	b.WriteString(helpBar.Render(help) + "\n")

	return b.String()
}

func (m model) viewLogScreen() string {
	var b strings.Builder

	svc := m.services[m.logService]

	scrollHint := ""
	if m.logUserScroll {
		scrollHint = lipgloss.NewStyle().Foreground(yellow).Render("  (paused - press G to resume)")
	}

	header := fmt.Sprintf("  %s  %s%s", svc.Name, lipgloss.NewStyle().Foreground(dim).Render("Q back · ↑↓ scroll"), scrollHint)
	b.WriteString(logHeaderStyle.Render(header) + "\n")
	b.WriteString(logBorderStyle.Render(strings.Repeat("─", m.width)) + "\n")
	b.WriteString(m.logVP.View())
	b.WriteString("\n")

	return b.String()
}
