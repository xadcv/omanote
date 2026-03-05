package main

import (
	"fmt"
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
	state    appState
	runState RunState
	devices  AudioDevices
	spinner  spinner.Model
	err      error
	width    int
	height   int
	frame    int // animation tick counter
}

// Messages
type devicesDetectedMsg struct{ devices AudioDevices }
type statusCheckedMsg struct{ state RunState }
type startedMsg struct {
	result StartResult
	err    error
}
type stoppedMsg struct{ err error }
type tickRefreshMsg struct{}
type animTickMsg struct{}

// Commands
func cmdDetectDevices() tea.Msg {
	return devicesDetectedMsg{devices: detectDevices()}
}

func cmdCheckStatus() tea.Msg {
	return statusCheckedMsg{state: checkRunState()}
}

func cmdStart(devices AudioDevices) tea.Cmd {
	return func() tea.Msg {
		result, err := startVirtualMic(devices)
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

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6AD5"))

	keyDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8795E8"))

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
		cmdDetectDevices,
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
			if m.devices.Err != nil {
				return m, nil
			}
			m.state = stateStarting
			return m, tea.Batch(m.spinner.Tick, cmdStart(m.devices))
		case "r":
			return m, tea.Batch(cmdDetectDevices, cmdCheckStatus)
		}

	case devicesDetectedMsg:
		m.devices = msg.devices
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
				MicPID:   msg.result.MicPID,
				SysPID:   msg.result.SysPID,
				ModuleID: msg.result.ModuleID,
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
		statusContent.WriteString(m.spinner.View() + " Conjuring virtual mic...")
	case stateStopping:
		statusContent.WriteString(m.spinner.View() + " Banishing virtual mic...")
	default:
		if m.runState.Running {
			statusContent.WriteString(runningStyle.Render("  ** Virtual Mic is LIVE **"))
			statusContent.WriteString("\n\n")
			statusContent.WriteString(labelStyle.Render("  mic loopback ") + valueStyle.Render(fmt.Sprintf("pid %d", m.runState.MicPID)))
			statusContent.WriteString("\n")
			statusContent.WriteString(labelStyle.Render("  sys loopback ") + valueStyle.Render(fmt.Sprintf("pid %d", m.runState.SysPID)))
			statusContent.WriteString("\n")
			statusContent.WriteString(labelStyle.Render("  null sink    ") + valueStyle.Render(fmt.Sprintf("module %s", m.runState.ModuleID)))
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

	// Audio devices section
	b.WriteString(divider)
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("  Audio Devices"))
	b.WriteString("\n\n")
	if m.devices.Err != nil {
		b.WriteString(errStyle.Render("  " + m.devices.Err.Error()))
		b.WriteString("\n")
	} else if m.devices.DefaultSink != "" {
		b.WriteString(labelStyle.Render("  speaker ") + valueStyle.Render(m.devices.DefaultSink))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("  mic     ") + valueStyle.Render(m.devices.DefaultSource))
		b.WriteString("\n")
	}

	// Error
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
	b.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" refresh"))
	b.WriteString("  " + keyStyle.Render("q") + keyDescStyle.Render(" quit"))
	b.WriteString("\n")

	return tea.NewView(b.String())
}
