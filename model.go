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
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
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
	logoBoxStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

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

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4B3880"))
)

var logo = `
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|`

var soundWaveFrames = []string{
	" ~~ * ~~ * ~~ ",
	" * ~~ * ~~ * ~ ",
	" ~ * ~~ * ~~ * ",
	" ~~ * ~ * ~~ * ",
}

func initialModel() model {
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6AD5"))),
	)
	return model{
		state:   stateIdle,
		spinner: s,
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
			return m, tea.Quit
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
		}
		return m, nil

	case stoppedMsg:
		m.state = stateIdle
		m.err = msg.err
		m.runState = RunState{}
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

	// Animated gradient logo
	for i, line := range strings.Split(logo, "\n") {
		if line == "" {
			continue
		}
		b.WriteString(rainbowText(line, m.frame+i))
		b.WriteString("\n")
	}

	// Sound wave animation under logo
	wave := soundWaveFrames[m.frame%len(soundWaveFrames)]
	waveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8795E8"))
	b.WriteString("          " + waveStyle.Render(wave))
	b.WriteString("\n\n")

	// Divider
	divider := dividerStyle.Render(strings.Repeat("~", 44))

	// Status section
	var statusContent strings.Builder
	switch m.state {
	case stateStarting:
		statusContent.WriteString(m.spinner.View() + " Starting Omanote...")
	case stateStopping:
		statusContent.WriteString(m.spinner.View() + " Stopping Omanote...")
	default:
		if m.runState.Running {
			statusContent.WriteString(runningStyle.Render("  ** Omanote is LIVE **"))
			statusContent.WriteString("\n\n")
			statusContent.WriteString(labelStyle.Render("  mic loopback ") + valueStyle.Render("module "+m.runState.MicMod))
			statusContent.WriteString("\n")
			statusContent.WriteString(labelStyle.Render("  sys loopback ") + valueStyle.Render("module "+m.runState.SysMod))
			statusContent.WriteString("\n")
			statusContent.WriteString(labelStyle.Render("  null sink    ") + valueStyle.Render("module "+m.runState.SinkMod))
		} else {
			statusContent.WriteString(stoppedStyle.Render("  ~ sleeping ~"))
		}
	}

	statusBox := statusBoxStopped
	if m.runState.Running {
		statusBox = statusBoxRunning
	}
	b.WriteString(statusBox.Render(statusContent.String()))
	b.WriteString("\n\n")

	// Device selection
	b.WriteString(divider)
	b.WriteString("\n")

	canSelect := m.state == stateIdle && !m.runState.Running

	// Microphone panel
	if m.focusPanel == 0 && canSelect {
		b.WriteString(subtitleStyle.Render("  Microphone"))
	} else {
		b.WriteString(dimSubtitleStyle.Render("  Microphone"))
	}
	b.WriteString("\n")
	for i, s := range m.sources {
		label := deviceLabel(s)
		if i == m.selectedSource {
			if m.focusPanel == 0 && canSelect {
				b.WriteString(cursorStyle.Render("  > ") + valueStyle.Render(label))
			} else {
				b.WriteString("  > " + dimValueStyle.Render(label))
			}
		} else {
			b.WriteString(keyDescStyle.Render("    " + label))
		}
		b.WriteString("\n")
	}
	if len(m.sources) == 0 {
		b.WriteString(errStyle.Render("    no microphones found"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// System Audio panel
	if m.focusPanel == 1 && canSelect {
		b.WriteString(subtitleStyle.Render("  System Audio"))
	} else {
		b.WriteString(dimSubtitleStyle.Render("  System Audio"))
	}
	b.WriteString("\n")
	for i, s := range m.sinks {
		label := deviceLabel(s)
		if i == m.selectedSink {
			if m.focusPanel == 1 && canSelect {
				b.WriteString(cursorStyle.Render("  > ") + valueStyle.Render(label))
			} else {
				b.WriteString("  > " + dimValueStyle.Render(label))
			}
		} else {
			b.WriteString(keyDescStyle.Render("    " + label))
		}
		b.WriteString("\n")
	}
	if len(m.sinks) == 0 {
		b.WriteString(errStyle.Render("    no output devices found"))
		b.WriteString("\n")
	}

	// Errors
	if m.devicesErr != nil {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("  !! " + m.devicesErr.Error()))
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("  !! " + m.err.Error()))
		b.WriteString("\n")
	}

	// Controls
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n")
	if m.runState.Running {
		b.WriteString("  " + keyStyle.Render("enter") + keyDescStyle.Render(" stop"))
	} else {
		b.WriteString("  " + keyStyle.Render("enter") + keyDescStyle.Render(" start"))
	}
	b.WriteString("  " + keyStyle.Render("tab") + keyDescStyle.Render(" switch"))
	b.WriteString("  " + keyStyle.Render("\u2191\u2193") + keyDescStyle.Render(" select"))
	b.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" refresh"))
	b.WriteString("  " + keyStyle.Render("q") + keyDescStyle.Render(" quit"))
	b.WriteString("\n")

	return tea.NewView(b.String())
}
