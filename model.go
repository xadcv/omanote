package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type appState int

const (
	stateIdle appState = iota
	stateStarting
	stateStopping
)

type recState int

const (
	recOff recState = iota
	recOn
	recPrompt
)

func defaultOutputDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Recordings")
}

type model struct {
	state          appState
	runState       RunState
	sources        []AudioDevice
	sinks          []AudioDevice
	selectedSource int
	selectedSink   int
	focusPanel     int // 0=source, 1=sink
	devicesErr     error
	spinner        spinner.Model
	err            error
	width          int
	height         int
	frame          int
	vis            *Visualizer
	mon            *AudioMonitor
	bands          [numBands]float64
	recState       recState
	recElapsed     time.Duration
	outputDir      string
	editingDir     bool
	dirInput       string
	schemeIdx      int
	cfg            Config
	autostart      bool
}

// Messages
type devicesListedMsg struct {
	sources       []AudioDevice
	sinks         []AudioDevice
	defaultSource string
	defaultSink   string
	err           error
}
type statusCheckedMsg struct {
	status DaemonStatus
	err    error
}
type startedMsg struct {
	result StartResult
	err    error
}
type stoppedMsg struct{ err error }
type tickRefreshMsg struct{}
type animTickMsg struct{}
type daemonCommandMsg struct {
	status  DaemonStatus
	message string
	err     error
}
type autostartMsg struct {
	enabled bool
	err     error
}

// Commands
func cmdListDevices() tea.Msg {
	sources, srcErr := listSelectableSources()
	sinks, sinkErr := listSelectableSinks()
	var err error
	if srcErr != nil {
		err = srcErr
	} else if sinkErr != nil {
		err = sinkErr
	}
	return devicesListedMsg{
		sources:       sources,
		sinks:         sinks,
		defaultSource: getDefaultSource(),
		defaultSink:   getDefaultSink(),
		err:           err,
	}
}

func cmdCheckStatus() tea.Msg {
	status, err := daemonStatus(true)
	return statusCheckedMsg{status: status, err: err}
}

func cmdStart(micDevice, outputDevice string) tea.Cmd {
	return func() tea.Msg {
		result, err := startVirtualMic(micDevice, outputDevice)
		return startedMsg{result: result, err: err}
	}
}

func cmdStop() tea.Msg {
	return stoppedMsg{err: stopVirtualMic()}
}

func cmdScheduleRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickRefreshMsg{}
	})
}

func cmdAnimTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func cmdDaemon(command string, args ...string) tea.Cmd {
	return func() tea.Msg {
		resp, err := sendControlAutoStart(command, args...)
		if err != nil {
			return daemonCommandMsg{err: err}
		}
		if !resp.OK {
			return daemonCommandMsg{status: resp.Status, err: fmt.Errorf("%s", resp.Error)}
		}
		return daemonCommandMsg{status: resp.Status, message: resp.Message}
	}
}

func cmdAutostartStatus() tea.Msg {
	return autostartMsg{enabled: autostartEnabled()}
}

func cmdSetAutostart(enabled bool) tea.Cmd {
	return func() tea.Msg {
		err := setAutostartEnabled(enabled)
		return autostartMsg{enabled: enabled, err: err}
	}
}

func rainbowText(text string, offset int, gradient []string) string {
	var b strings.Builder
	ci := offset
	for _, ch := range text {
		if ch == ' ' {
			b.WriteRune(ch)
			continue
		}
		color := gradient[ci%len(gradient)]
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color))
		b.WriteString(style.Render(string(ch)))
		ci++
	}
	return b.String()
}

// Styles
var (
	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#C774E8"))

	dimSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8B6AAE"))

	runningStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6AD5"))

	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF4444"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AD8CFF"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94D0FF"))

	dimValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6A8FAA"))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6AD5"))

	keyDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8795E8"))

	cursorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6AD5"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#C774E8")).
			Padding(0, 1)

	statusBoxRunning = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#04B575")).
				Padding(0, 1)

	statusBoxStopped = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF6AD5")).
				Padding(0, 1)

	recStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF4444"))
)

var logo = `
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|`

func initialModel() model {
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6AD5"))),
	)
	cfg := loadConfig()
	schemeIdx := colorSchemeByName(cfg.ColorScheme)
	vis := NewVisualizer(48000)
	vis.Scheme = &colorSchemes[schemeIdx]
	vis.Mode = visModeByName(cfg.VisMode)
	return model{
		state:     stateIdle,
		spinner:   s,
		vis:       vis,
		mon:       NewAudioMonitor(),
		outputDir: cfg.OutputDir,
		schemeIdx: schemeIdx,
		cfg:       cfg,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		cmdListDevices,
		cmdCheckStatus,
		cmdScheduleRefresh(),
		cmdAnimTick(),
		cmdAutostartStatus,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if m.editingDir {
			key := msg.String()
			switch key {
			case "enter":
				nextOutputDir := strings.TrimSpace(m.dirInput)
				if nextOutputDir == "" {
					m.err = fmt.Errorf("output directory cannot be empty")
					return m, nil
				}
				m.outputDir = nextOutputDir
				m.editingDir = false
				return m, cmdDaemon("set-output-dir", m.outputDir)
			case "escape":
				m.editingDir = false
			case "backspace":
				if len(m.dirInput) > 0 {
					m.dirInput = m.dirInput[:len(m.dirInput)-1]
				}
			default:
				if len(key) == 1 {
					m.dirInput += key
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.mon.Stop()
			return m, tea.Quit
		case "v":
			previousMode := m.vis.Mode
			m.vis.CycleMode()
			updatedCfg := m.cfg
			updatedCfg.VisMode = m.vis.ModeName()
			if err := saveConfig(updatedCfg); err != nil {
				m.vis.Mode = previousMode
				m.err = fmt.Errorf("save config: %w", err)
				return m, nil
			}
			m.cfg = updatedCfg
			m.err = nil
			return m, nil
		case "c":
			nextSchemeIdx := (m.schemeIdx + 1) % len(colorSchemes)
			updatedCfg := m.cfg
			updatedCfg.ColorScheme = colorSchemes[nextSchemeIdx].Name
			if err := saveConfig(updatedCfg); err != nil {
				m.err = fmt.Errorf("save config: %w", err)
				return m, nil
			}
			m.schemeIdx = nextSchemeIdx
			m.vis.Scheme = &colorSchemes[m.schemeIdx]
			m.cfg = updatedCfg
			m.err = nil
			return m, nil
		case "enter", " ":
			if m.state != stateIdle {
				return m, nil
			}
			if m.runState.Running {
				m.state = stateStopping
				return m, tea.Batch(m.spinner.Tick, cmdDaemon("stop"))
			}
			if len(m.sources) == 0 || len(m.sinks) == 0 || m.devicesErr != nil {
				return m, nil
			}
			m.state = stateStarting
			mic := m.sources[m.selectedSource].Name
			out := m.sinks[m.selectedSink].Name
			return m, tea.Batch(m.spinner.Tick, cmdDaemon("start", mic, out))
		case "tab":
			if m.state == stateIdle && !m.runState.Running {
				m.focusPanel = (m.focusPanel + 1) % 2
			}
		case "up", "k":
			if m.state == stateIdle && !m.runState.Running {
				if m.focusPanel == 0 && m.selectedSource > 0 {
					m.selectedSource--
				} else if m.focusPanel == 1 && m.selectedSink > 0 {
					m.selectedSink--
				}
			}
		case "down", "j":
			if m.state == stateIdle && !m.runState.Running {
				if m.focusPanel == 0 && m.selectedSource < len(m.sources)-1 {
					m.selectedSource++
				} else if m.focusPanel == 1 && m.selectedSink < len(m.sinks)-1 {
					m.selectedSink++
				}
			}
		case "r":
			if m.recState == recPrompt {
				return m, nil
			}
			if m.recState == recOn {
				return m, cmdDaemon("record-stop")
			}
			if m.runState.Running {
				return m, cmdDaemon("record-start")
			}
			return m, tea.Batch(cmdListDevices, cmdCheckStatus)
		case "R":
			return m, tea.Batch(cmdListDevices, cmdCheckStatus)
		case "s":
			if m.recState == recPrompt {
				return m, cmdDaemon("record-save")
			}
		case "d":
			if m.recState == recPrompt {
				return m, cmdDaemon("record-discard")
			}
		case "o":
			if m.runState.Running && m.recState != recPrompt && !m.editingDir {
				m.editingDir = true
				m.dirInput = m.outputDir
				return m, nil
			}
		case "a":
			return m, cmdSetAutostart(!m.autostart)
		case "escape":
			if m.editingDir {
				m.editingDir = false
				return m, nil
			}
		}

	case sampleMsg:
		if len(msg.samples) > 0 {
			m.bands = m.vis.Analyze(msg.samples)
		}
		if m.mon.IsRunning() {
			return m, cmdReadSamples(m.mon)
		}
		return m, nil

	case devicesListedMsg:
		m.devicesErr = msg.err

		// Remember current selection by name before updating lists.
		prevSource := ""
		if m.selectedSource < len(m.sources) {
			prevSource = m.sources[m.selectedSource].Name
		}
		prevSink := ""
		if m.selectedSink < len(m.sinks) {
			prevSink = m.sinks[m.selectedSink].Name
		}

		m.sources = msg.sources
		m.sinks = msg.sinks

		// Restore selection by name. Priority: previous selection > config preferred > system default.
		m.selectedSource = findDevice(m.sources, prevSource, m.cfg.PreferredSource, msg.defaultSource)
		m.selectedSink = findDevice(m.sinks, prevSink, m.cfg.PreferredSink, msg.defaultSink)
		return m, nil

	case statusCheckedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
		}
		cmd := m.applyDaemonStatus(msg.status)
		return m, cmd

	case daemonCommandMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
		}
		cmd := m.applyDaemonStatus(msg.status)
		return m, cmd

	case autostartMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.autostart = msg.enabled
		}
		return m, nil

	case tickRefreshMsg:
		if m.state == stateIdle {
			return m, tea.Batch(cmdListDevices, cmdCheckStatus, cmdScheduleRefresh())
		}
		return m, tea.Batch(cmdCheckStatus, cmdScheduleRefresh())

	case animTickMsg:
		m.frame++
		return m, cmdAnimTick()

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) applyDaemonStatus(status DaemonStatus) tea.Cmd {
	m.runState = status.RunState
	m.recState = recStateFromName(status.RecState)
	m.recElapsed = time.Duration(status.RecElapsedSecs) * time.Second
	if status.OutputDir != "" && !m.editingDir {
		m.outputDir = status.OutputDir
		m.cfg.OutputDir = status.OutputDir
	}
	if status.Error != "" {
		m.err = fmt.Errorf("%s", status.Error)
	}

	if m.runState.Running {
		if !m.mon.IsRunning() {
			if err := m.mon.Start(sinkName + ".monitor"); err != nil {
				m.err = err
				return nil
			}
			return cmdReadSamples(m.mon)
		}
		return nil
	}

	if m.mon.IsRunning() {
		m.mon.Stop()
	}
	return nil
}

// findDevice returns the index of the first matching device name from candidates.
// Falls back to 0 if none match.
func findDevice(devices []AudioDevice, names ...string) int {
	for _, name := range names {
		if name == "" {
			continue
		}
		for i, d := range devices {
			if d.Name == name {
				return i
			}
		}
	}
	return 0
}

func deviceLabel(d AudioDevice) string {
	if d.Description != "" {
		return d.Description
	}
	return d.Name
}

func (m model) View() tea.View {
	var b strings.Builder
	contentWidth := 80

	// --- Rainbow logo ---
	gradient := colorSchemes[m.schemeIdx].Gradient
	for i, line := range strings.Split(logo, "\n") {
		if line == "" {
			continue
		}
		b.WriteString(rainbowText(line, m.frame+i, gradient))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// --- Visualizer ---
	visOutput := m.vis.Render(m.bands)
	if visOutput != "" {
		b.WriteString(visOutput)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// --- Status box ---
	var statusContent strings.Builder
	switch m.state {
	case stateStarting:
		statusContent.WriteString(m.spinner.View() + " Starting Omanote...")
	case stateStopping:
		statusContent.WriteString(m.spinner.View() + " Stopping Omanote...")
	default:
		if m.runState.Running {
			statusContent.WriteString(runningStyle.Render("  ** Omanote is LIVE **"))
			if m.recState == recOn {
				elapsed := m.recElapsed
				mins := int(elapsed.Minutes())
				secs := int(elapsed.Seconds()) % 60
				statusContent.WriteString("  " + recStyle.Render(fmt.Sprintf("● REC %02d:%02d", mins, secs)))
			}
			// Show device descriptions for mic and system output
			micDesc := ""
			if m.selectedSource < len(m.sources) {
				micDesc = deviceLabel(m.sources[m.selectedSource])
			}
			sysDesc := ""
			if m.selectedSink < len(m.sinks) {
				sysDesc = deviceLabel(m.sinks[m.selectedSink])
			}
			if micDesc != "" {
				statusContent.WriteString("\n")
				statusContent.WriteString(labelStyle.Render("  mic ") + valueStyle.Render(micDesc))
			}
			if sysDesc != "" {
				statusContent.WriteString("\n")
				statusContent.WriteString(labelStyle.Render("  sys ") + valueStyle.Render(sysDesc))
			}
		} else {
			statusContent.WriteString(stoppedStyle.Render("  ~ sleeping ~"))
		}
	}

	statusBox := statusBoxStopped
	if m.runState.Running {
		statusBox = statusBoxRunning
	}
	b.WriteString(statusBox.Width(contentWidth - 2).Render(statusContent.String()))
	b.WriteString("\n")

	if m.autostart {
		b.WriteString(labelStyle.Render("  login ") + valueStyle.Render("enabled"))
	} else {
		b.WriteString(labelStyle.Render("  login ") + dimValueStyle.Render("disabled"))
	}
	b.WriteString("\n")

	// --- Recording info (below status box) ---
	if m.recState != recOff {
		if m.editingDir {
			b.WriteString(labelStyle.Render("  dir ") + valueStyle.Render(m.dirInput+"_"))
		} else {
			b.WriteString(labelStyle.Render("  dir ") + dimValueStyle.Render(m.outputDir))
		}
		b.WriteString("\n")
	}
	if m.recState == recPrompt {
		elapsed := m.recElapsed
		mins := int(elapsed.Minutes())
		secs := int(elapsed.Seconds()) % 60
		b.WriteString(recStyle.Render(fmt.Sprintf("  Recording stopped (%02d:%02d)", mins, secs)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// --- Errors ---
	if m.devicesErr != nil {
		b.WriteString(errStyle.Render("  !! " + m.devicesErr.Error()))
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString(errStyle.Render("  !! " + m.err.Error()))
		b.WriteString("\n")
	}

	// --- Device panels side by side ---
	canSelect := m.state == stateIdle && !m.runState.Running
	panelW := 37

	// Left panel: Microphone
	var leftB strings.Builder
	if m.focusPanel == 0 && canSelect {
		leftB.WriteString(subtitleStyle.Render("Microphone"))
	} else {
		leftB.WriteString(dimSubtitleStyle.Render("Microphone"))
	}
	leftB.WriteString("\n")
	for i, s := range m.sources {
		label := deviceLabel(s)
		// Truncate label if needed to fit panel width
		maxLabelW := panelW - 4
		if len(label) > maxLabelW {
			label = label[:maxLabelW-1] + "\u2026"
		}
		if i == m.selectedSource {
			if m.focusPanel == 0 && canSelect {
				leftB.WriteString(cursorStyle.Render("> ") + valueStyle.Render(label))
			} else {
				leftB.WriteString("> " + dimValueStyle.Render(label))
			}
		} else {
			leftB.WriteString(keyDescStyle.Render("  " + label))
		}
		leftB.WriteString("\n")
	}
	if len(m.sources) == 0 {
		leftB.WriteString(errStyle.Render("  no mics found"))
		leftB.WriteString("\n")
	}

	// Right panel: System Output
	var rightB strings.Builder
	if m.focusPanel == 1 && canSelect {
		rightB.WriteString(subtitleStyle.Render("System Output"))
	} else {
		rightB.WriteString(dimSubtitleStyle.Render("System Output"))
	}
	rightB.WriteString("\n")
	for i, s := range m.sinks {
		label := deviceLabel(s)
		maxLabelW := panelW - 4
		if len(label) > maxLabelW {
			label = label[:maxLabelW-1] + "\u2026"
		}
		if i == m.selectedSink {
			if m.focusPanel == 1 && canSelect {
				rightB.WriteString(cursorStyle.Render("> ") + valueStyle.Render(label))
			} else {
				rightB.WriteString("> " + dimValueStyle.Render(label))
			}
		} else {
			rightB.WriteString(keyDescStyle.Render("  " + label))
		}
		rightB.WriteString("\n")
	}
	if len(m.sinks) == 0 {
		rightB.WriteString(errStyle.Render("  no outputs found"))
		rightB.WriteString("\n")
	}

	leftPanel := boxStyle.Width(panelW).Render(leftB.String())
	rightPanel := boxStyle.Width(panelW).Render(rightB.String())
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel))
	b.WriteString("\n\n")

	// --- Help bar ---
	var help strings.Builder
	if m.recState == recPrompt {
		help.WriteString(keyStyle.Render("s") + keyDescStyle.Render(" save"))
		help.WriteString("  " + keyStyle.Render("d") + keyDescStyle.Render(" discard"))
	} else if m.editingDir {
		help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" confirm"))
		help.WriteString("  " + keyStyle.Render("esc") + keyDescStyle.Render(" cancel"))
	} else {
		if m.runState.Running {
			help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" stop"))
			if m.recState == recOn {
				help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" stop rec"))
			} else {
				help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" rec"))
			}
			help.WriteString("  " + keyStyle.Render("o") + keyDescStyle.Render(" output"))
		} else {
			help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" start"))
			help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" refresh"))
		}
		help.WriteString("  " + keyStyle.Render("v") + keyDescStyle.Render(" "+m.vis.ModeName()))
		help.WriteString("  " + keyStyle.Render("c") + keyDescStyle.Render(" "+colorSchemes[m.schemeIdx].Name))
		if m.autostart {
			help.WriteString("  " + keyStyle.Render("a") + keyDescStyle.Render(" login off"))
		} else {
			help.WriteString("  " + keyStyle.Render("a") + keyDescStyle.Render(" login on"))
		}
		if !m.runState.Running {
			help.WriteString("  " + keyStyle.Render("tab") + keyDescStyle.Render(" switch"))
			help.WriteString("  " + keyStyle.Render("\u2191\u2193") + keyDescStyle.Render(" select"))
		}
		help.WriteString("  " + keyStyle.Render("q") + keyDescStyle.Render(" close"))
	}
	b.WriteString(help.String())
	b.WriteString("\n")

	// Center content in the terminal
	content := b.String()

	// Use a fixed-width wrapper to constrain the content
	wrappedContent := lipgloss.NewStyle().Width(contentWidth).Render(content)

	// If we have terminal dimensions, center the frame
	var view tea.View
	if m.width > 0 && m.height > 0 {
		placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, wrappedContent)
		view = tea.NewView(placed)
	} else {
		view = tea.NewView(wrappedContent)
	}
	view.AltScreen = true
	return view
}
