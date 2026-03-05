package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

type appState int

const (
	stateIdle     appState = iota
	stateStarting
	stateStopping
)

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
}

// Messages
type devicesListedMsg struct {
	sources       []AudioDevice
	sinks         []AudioDevice
	defaultSource string
	defaultSink   string
	err           error
}
type statusCheckedMsg struct{ state RunState }
type startedMsg struct {
	result StartResult
	err    error
}
type stoppedMsg struct{ err error }
type tickRefreshMsg struct{}
type animTickMsg struct{}

// Commands
func cmdListDevices() tea.Msg {
	sources, srcErr := listSources()
	sinks, sinkErr := listSinks()
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
	return statusCheckedMsg{state: checkRunState()}
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

// Gradient palette for the logo
var gradientColors = []string{
	"#FF6AD5", // pink
	"#C774E8", // purple
	"#AD8CFF", // lavender
	"#8795E8", // periwinkle
	"#94D0FF", // sky blue
}

func rainbowText(text string, offset int) string {
	var b strings.Builder
	ci := offset
	for _, ch := range text {
		if ch == ' ' {
			b.WriteRune(ch)
			continue
		}
		color := gradientColors[ci%len(gradientColors)]
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
	return model{
		state:   stateIdle,
		spinner: s,
		vis:     NewVisualizer(48000),
		mon:     NewAudioMonitor(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		cmdListDevices,
		cmdCheckStatus,
		cmdScheduleRefresh(),
		cmdAnimTick(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.mon.Stop()
			return m, tea.Quit
		case "v":
			m.vis.CycleMode()
			return m, nil
		case "enter", " ":
			if m.state != stateIdle {
				return m, nil
			}
			if m.runState.Running {
				m.state = stateStopping
				return m, tea.Batch(m.spinner.Tick, cmdStop)
			}
			if len(m.sources) == 0 || len(m.sinks) == 0 || m.devicesErr != nil {
				return m, nil
			}
			m.state = stateStarting
			mic := m.sources[m.selectedSource].Name
			out := m.sinks[m.selectedSink].Name
			return m, tea.Batch(m.spinner.Tick, cmdStart(mic, out))
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
			return m, tea.Batch(cmdListDevices, cmdCheckStatus)
		}

	case sampleMsg:
		m.bands = m.vis.Analyze(msg.samples)
		return m, cmdReadSamples(m.mon)

	case devicesListedMsg:
		m.devicesErr = msg.err
		m.sources = msg.sources
		m.sinks = msg.sinks
		// Auto-select defaults on first load
		for i, s := range m.sources {
			if s.Name == msg.defaultSource {
				m.selectedSource = i
				break
			}
		}
		for i, s := range m.sinks {
			if s.Name == msg.defaultSink {
				m.selectedSink = i
				break
			}
		}
		// Clamp selection
		if m.selectedSource >= len(m.sources) {
			m.selectedSource = max(0, len(m.sources)-1)
		}
		if m.selectedSink >= len(m.sinks) {
			m.selectedSink = max(0, len(m.sinks)-1)
		}
		return m, nil

	case statusCheckedMsg:
		m.runState = msg.state
		return m, nil

	case startedMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.runState = RunState{
				Running:  true,
				SinkMod:  msg.result.SinkMod,
				RemapMod: msg.result.RemapMod,
				MicMod:   msg.result.MicMod,
				SysMod:   msg.result.SysMod,
			}
			m.mon.Start(sinkName + ".monitor")
			return m, tea.Batch(cmdReadSamples(m.mon))
		}
		return m, nil

	case stoppedMsg:
		m.state = stateIdle
		m.err = msg.err
		m.runState = RunState{}
		m.mon.Stop()
		return m, nil

	case tickRefreshMsg:
		if m.state == stateIdle {
			return m, tea.Batch(cmdCheckStatus, cmdScheduleRefresh())
		}
		return m, cmdScheduleRefresh()

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
	for i, line := range strings.Split(logo, "\n") {
		if line == "" {
			continue
		}
		b.WriteString(rainbowText(line, m.frame+i))
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
			// Show device descriptions for mic and system audio
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
	b.WriteString("\n\n")

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

	// Right panel: System Audio
	var rightB strings.Builder
	if m.focusPanel == 1 && canSelect {
		rightB.WriteString(subtitleStyle.Render("System Audio"))
	} else {
		rightB.WriteString(dimSubtitleStyle.Render("System Audio"))
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
	if m.runState.Running {
		help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" stop"))
	} else {
		help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" start"))
	}
	help.WriteString("  " + keyStyle.Render("v") + keyDescStyle.Render(" "+m.vis.ModeName()))
	help.WriteString("  " + keyStyle.Render("tab") + keyDescStyle.Render(" switch"))
	help.WriteString("  " + keyStyle.Render("\u2191\u2193") + keyDescStyle.Render(" select"))
	help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" refresh"))
	help.WriteString("  " + keyStyle.Render("q") + keyDescStyle.Render(" quit"))
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

