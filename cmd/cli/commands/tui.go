package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/packalares/packalares/pkg/installer/phases"
)

// -- Styles --

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")). // bright white
			PaddingLeft(2).PaddingTop(1)

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")) // yellow

	completedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")) // green

	failedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")) // red

	skippedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dim gray

	pendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dim gray

	durationStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dim

	logBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			PaddingLeft(1).PaddingRight(1)

	logTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")) // normal gray

	progressBarStyle = lipgloss.NewStyle().
				PaddingLeft(2).PaddingTop(1).PaddingBottom(1)

	errorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("9")).
			Foreground(lipgloss.Color("9")).
			PaddingLeft(1).PaddingRight(1).
			MarginLeft(2)
)

// -- Phase status --

type phaseStatus int

const (
	statusPending phaseStatus = iota
	statusRunning
	statusDone
	statusFailed
	statusSkipped
)

type phaseInfo struct {
	name     string
	status   phaseStatus
	duration time.Duration
}

// -- Tea messages --

type phaseEventMsg phases.PhaseEvent

// waitForEvent returns a tea.Cmd that reads the next event from the channel.
func waitForEvent(ch <-chan phases.PhaseEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil // channel closed
		}
		return phaseEventMsg(ev)
	}
}

// -- Model --

type installModel struct {
	phases       []phaseInfo
	currentPhase int
	logs         []string
	maxLogLines  int

	spinner  spinner.Model
	progress progress.Model
	viewport viewport.Model

	width  int
	height int

	eventCh <-chan phases.PhaseEvent
	done    bool
	err     error
	reboot  bool

	// Final error message for display
	errMsg   string
	password string // captured generated password
}

func newInstallModel(eventCh <-chan phases.PhaseEvent) installModel {
	names := phases.PhaseNames()
	pi := make([]phaseInfo, len(names))
	for i, n := range names {
		pi[i] = phaseInfo{name: n, status: statusPending}
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = runningStyle

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	vp := viewport.New(80, 10)

	return installModel{
		phases:       pi,
		currentPhase: -1,
		logs:         nil,
		maxLogLines:  200,
		spinner:      s,
		progress:     p,
		viewport:     vp,
		eventCh:      eventCh,
		width:        80,
		height:       24,
	}
}

func (m installModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.eventCh),
	)
}

func (m installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			// Allow quitting during install (will leave state file for resume)
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recalculate viewport size
		vpWidth, vpHeight := m.logViewportSize()
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		m.progress.Width = m.width - 6
		m.updateViewportContent()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		model, cmd := m.progress.Update(msg)
		m.progress = model.(progress.Model)
		cmds = append(cmds, cmd)

	case phaseEventMsg:
		ev := phases.PhaseEvent(msg)
		cmds = append(cmds, m.handlePhaseEvent(ev)...)
		// Keep listening for more events
		if !m.done {
			cmds = append(cmds, waitForEvent(m.eventCh))
		}

	case nil:
		// Channel closed, we're done
		m.done = true
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

func (m *installModel) handlePhaseEvent(ev phases.PhaseEvent) []tea.Cmd {
	var cmds []tea.Cmd

	switch ev.Type {
	case phases.EventPhaseStart:
		if ev.PhaseIdx < len(m.phases) {
			m.phases[ev.PhaseIdx].status = statusRunning
			m.currentPhase = ev.PhaseIdx
		}
		// Clear logs for new phase
		m.logs = nil
		m.updateViewportContent()

	case phases.EventPhaseLog:
		// Capture generated password
		if strings.HasPrefix(ev.Message, "Generated admin password: ") {
			m.password = strings.TrimPrefix(ev.Message, "Generated admin password: ")
		}
		m.logs = append(m.logs, ev.Message)
		if len(m.logs) > m.maxLogLines {
			m.logs = m.logs[len(m.logs)-m.maxLogLines:]
		}
		m.updateViewportContent()
		// Auto-scroll to bottom
		m.viewport.GotoBottom()

	case phases.EventPhaseComplete:
		if ev.PhaseIdx < len(m.phases) {
			m.phases[ev.PhaseIdx].status = statusDone
			m.phases[ev.PhaseIdx].duration = ev.Duration
		}
		// Update progress bar
		pct := float64(ev.PhaseIdx+1) / float64(ev.Total)
		cmds = append(cmds, m.progress.SetPercent(pct))

	case phases.EventPhaseFailed:
		if ev.PhaseIdx < len(m.phases) {
			m.phases[ev.PhaseIdx].status = statusFailed
			m.phases[ev.PhaseIdx].duration = ev.Duration
		}
		m.err = ev.Err
		m.errMsg = fmt.Sprintf("Phase %q failed: %v", ev.Phase, ev.Err)
		m.done = true
		return []tea.Cmd{tea.Quit}

	case phases.EventPhaseSkipped:
		if ev.PhaseIdx < len(m.phases) {
			m.phases[ev.PhaseIdx].status = statusSkipped
		}

	case phases.EventRebootRequired:
		m.reboot = true
		m.done = true
		return []tea.Cmd{tea.Quit}

	case phases.EventInstallComplete:
		m.done = true
		cmds = append(cmds, m.progress.SetPercent(1.0))
		// Quit after brief delay so progress bar animation completes
		cmds = append(cmds, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
			return tea.Quit()
		}))
	}

	return cmds
}

func (m *installModel) updateViewportContent() {
	var sb strings.Builder
	for _, line := range m.logs {
		sb.WriteString(logTextStyle.Render(line))
		sb.WriteString("\n")
	}
	m.viewport.SetContent(sb.String())
}

func (m *installModel) logViewportSize() (width, height int) {
	// Leave room for: title (2), phases list, separator, progress bar (3), padding
	phaseLines := len(m.phases) + 1 // +1 for spacing
	headerLines := 2                // title
	footerLines := 3                // progress bar area
	available := m.height - headerLines - phaseLines - footerLines - 4

	if available < 3 {
		available = 3
	}
	if available > 15 {
		available = 15
	}

	w := m.width - 6 // left/right padding + border
	if w < 20 {
		w = 20
	}

	return w, available
}

func (m installModel) View() string {
	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("Packalares Installer"))
	sb.WriteString("\n\n")

	// Phase list
	completedCount := 0
	for _, p := range m.phases {
		var icon, name, dur string

		switch p.status {
		case statusDone:
			icon = completedStyle.Render(" \u2713") // checkmark
			name = completedStyle.Render(p.name)
			dur = durationStyle.Render(formatDuration(p.duration))
			completedCount++
		case statusRunning:
			icon = " " + m.spinner.View()
			name = runningStyle.Render(p.name)
		case statusFailed:
			icon = failedStyle.Render(" \u2717") // x mark
			name = failedStyle.Render(p.name)
			dur = durationStyle.Render(formatDuration(p.duration))
			completedCount++
		case statusSkipped:
			icon = skippedStyle.Render(" \u2013") // en dash
			name = skippedStyle.Render(p.name)
			dur = skippedStyle.Render("skipped")
			completedCount++
		default:
			icon = pendingStyle.Render(" \u25cb") // open circle
			name = pendingStyle.Render(p.name)
		}

		line := fmt.Sprintf("  %s %s", icon, name)
		if dur != "" {
			// Pad to align durations
			pad := m.width - lipgloss.Width(line) - lipgloss.Width(dur) - 4
			if pad < 2 {
				pad = 2
			}
			line += strings.Repeat(" ", pad) + dur
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Log viewport
	sb.WriteString("\n")
	vpWidth, vpHeight := m.logViewportSize()
	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight

	logHeader := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("Log")

	logBox := logBorderStyle.
		Width(m.width - 6).
		Render(m.viewport.View())

	sb.WriteString("  " + logHeader + "\n")
	sb.WriteString("  " + logBox + "\n")

	// Progress bar
	total := len(m.phases)
	pct := float64(completedCount) / float64(total)
	m.progress.Width = m.width - 6

	progressLine := fmt.Sprintf("  %s  %d/%d (%d%%)",
		m.progress.ViewAs(pct),
		completedCount, total, int(pct*100))
	sb.WriteString(progressBarStyle.Render(progressLine))
	sb.WriteString("\n")

	// Error message if any
	if m.errMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(errorBoxStyle.Render(m.errMsg))
		sb.WriteString("\n")
	}

	// Reboot message
	if m.reboot {
		sb.WriteString("\n")
		rebootMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			PaddingLeft(2).
			Render("Reboot required to continue install.\nAfter reboot, run: packalares install")
		sb.WriteString(rebootMsg)
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// runInstallTUI launches the bubbletea TUI and blocks until install completes.
func runInstallTUI(opts *phases.InstallOptions) error {
	eventCh := make(chan phases.PhaseEvent, 64)

	var installErr error
	go func() {
		installErr = phases.RunInstallWithEvents(opts, eventCh)
	}()

	model := newInstallModel(eventCh)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	m := finalModel.(installModel)

	// Print password after TUI exits (alt-screen restored)
	if m.password != "" {
		fmt.Printf("\n  Generated admin password: %s\n", m.password)
	}

	if m.reboot {
		return phases.ErrRebootRequired
	}
	if installErr != nil {
		return installErr
	}
	if m.err != nil {
		return m.err
	}

	return nil
}
