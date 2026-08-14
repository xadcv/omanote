package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const quattroPluginID = "xadcv.omanote"

func runCLI(args []string) error {
	if len(args) == 0 || args[0] == "tui" {
		return runTUI()
	}

	switch args[0] {
	case "daemon":
		return runDaemon()
	case "status":
		return runStatus(args[1:])
	case "start":
		return runControlCommand("start", args[1:]...)
	case "stop":
		return runControlCommand("stop")
	case "devices":
		return runDevices(args[1:])
	case "record":
		return runRecordCommand(args[1:])
	case "quit":
		resp, err := sendControl("quit")
		if err != nil {
			return nil
		}
		if !resp.OK {
			return errors.New(resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case "menu":
		return runMenu()
	case "autostart":
		return runAutostartCommand(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTUI() error {
	p := teaProgram()
	_, err := p.Run()
	return err
}

func runControlCommand(command string, args ...string) error {
	resp, err := sendControlAutoStart(command, args...)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return nil
}

func runRecordCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: omanote record {start|stop|save|discard}")
	}
	switch args[0] {
	case "start":
		return runControlCommand("record-start")
	case "stop":
		return runControlCommand("record-stop")
	case "save":
		return runControlCommand("record-save")
	case "discard":
		return runControlCommand("record-discard")
	default:
		return fmt.Errorf("unknown record command %q", args[0])
	}
}

func runAutostartCommand(args []string) error {
	if len(args) == 0 {
		if autostartEnabled() {
			fmt.Println("enabled")
		} else {
			fmt.Println("disabled")
		}
		return nil
	}

	switch args[0] {
	case "enable":
		if err := setAutostartEnabled(true); err != nil {
			return err
		}
		fmt.Println("Omanote start on login enabled")
	case "disable":
		if err := setAutostartEnabled(false); err != nil {
			return err
		}
		fmt.Println("Omanote start on login disabled")
	case "status":
		if autostartEnabled() {
			fmt.Println("enabled")
		} else {
			fmt.Println("disabled")
		}
	default:
		return fmt.Errorf("unknown autostart command %q", args[0])
	}
	return nil
}

func runStatus(args []string) error {
	follow := false
	waybar := false
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--follow":
			follow = true
		case "--waybar":
			waybar = true
		case "--json":
			jsonOut = true
		default:
			return fmt.Errorf("unknown status flag %q", arg)
		}
	}
	if waybar && jsonOut {
		return fmt.Errorf("status flags --waybar and --json are mutually exclusive")
	}

	printStatus := func() {
		status, err := daemonStatus(false)
		if err != nil {
			cfg := loadConfig()
			status = DaemonStatus{RecState: recStateName(recOff), OutputDir: cfg.OutputDir, PreferredSource: cfg.PreferredSource, PreferredSink: cfg.PreferredSink, Error: err.Error()}
		}
		status = enrichStatus(status)
		switch {
		case jsonOut:
			fmt.Println(formatJSONStatus(status))
		case waybar:
			fmt.Println(formatWaybarStatus(status))
		default:
			fmt.Println(formatHumanStatus(status))
		}
	}

	printStatus()
	if !follow {
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		printStatus()
	}
	return nil
}

func formatHumanStatus(status DaemonStatus) string {
	if !status.DaemonRunning {
		return "Omanote daemon: stopped"
	}
	state := "sleeping"
	if status.RunState.Running {
		state = "live"
	}
	if status.RecState == "recording" {
		state += ", recording"
	} else if status.RecState == "pending" {
		state += ", recording pending save"
	}
	if status.Error != "" {
		state += "\nerror: " + status.Error
	}
	return "Omanote daemon: running\nstate: " + state
}

func enrichStatus(status DaemonStatus) DaemonStatus {
	cfg := loadConfig()
	if status.OutputDir == "" {
		status.OutputDir = cfg.OutputDir
	}
	if status.PreferredSource == "" {
		status.PreferredSource = cfg.PreferredSource
	}
	if status.PreferredSink == "" {
		status.PreferredSink = cfg.PreferredSink
	}
	if status.RecState == "" {
		status.RecState = recStateName(recOff)
	}
	status.Autostart = autostartEnabled()
	return status
}

func statusAlt(status DaemonStatus) string {
	alt := "inactive"
	if status.DaemonRunning {
		alt = "idle"
		if status.RunState.Running {
			alt = "live"
		}
		if status.RecState == "recording" {
			alt = "recording"
		} else if status.RecState == "pending" {
			alt = "pending"
		}
	}
	if status.Error != "" {
		alt = "error"
	}
	return alt
}

func statusTooltip(status DaemonStatus) string {
	if status.Error != "" {
		return status.Error
	}
	if !status.DaemonRunning {
		return "Omanote daemon stopped"
	}
	if status.RecState == "recording" {
		return fmt.Sprintf("Omanote recording %s", formatDuration(status.RecElapsedSecs))
	}
	if status.RecState == "pending" {
		return "Omanote recording stopped; save or discard"
	}
	if status.RunState.Running {
		return "Omanote virtual mic live"
	}
	return "Omanote ready"
}

func formatWaybarStatus(status DaemonStatus) string {
	alt := statusAlt(status)
	payload := map[string]any{
		"text":    "",
		"alt":     alt,
		"class":   alt,
		"tooltip": statusTooltip(status),
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func formatJSONStatus(status DaemonStatus) string {
	status = enrichStatus(status)
	payload := struct {
		DaemonStatus
		State string `json:"state"`
	}{
		DaemonStatus: status,
		State:        statusAlt(status),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"error":"encode failed"}`
	}
	return string(data)
}

type deviceList struct {
	Sources []AudioDevice `json:"sources"`
	Sinks   []AudioDevice `json:"sinks"`
}

func formatDevicesJSON(sources, sinks []AudioDevice) string {
	if sources == nil {
		sources = []AudioDevice{}
	}
	if sinks == nil {
		sinks = []AudioDevice{}
	}
	data, err := json.Marshal(deviceList{Sources: sources, Sinks: sinks})
	if err != nil {
		return `{"sources":[],"sinks":[]}`
	}
	return string(data)
}

func runDevices(args []string) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOut = true
		default:
			return fmt.Errorf("unknown devices flag %q", arg)
		}
	}

	sources, srcErr := listSelectableSources()
	sinks, sinkErr := listSelectableSinks()
	if srcErr != nil {
		return srcErr
	}
	if sinkErr != nil {
		return sinkErr
	}

	if jsonOut {
		fmt.Println(formatDevicesJSON(sources, sinks))
		return nil
	}

	fmt.Println("Microphone:")
	for _, device := range sources {
		fmt.Printf("  %s\t%s\n", device.Name, device.Description)
	}
	fmt.Println("System output:")
	for _, device := range sinks {
		fmt.Printf("  %s\t%s\n", device.Name, device.Description)
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func summonQuattroPanel() error {
	cmd := exec.Command("omarchy-shell", "shell", "summon", quattroPluginID, "{}")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("summon %s: %w", quattroPluginID, err)
	}
	return nil
}

func runMenu() error {
	if commandExists("omarchy-shell") {
		return summonQuattroPanel()
	}

	status, _ := daemonStatus(false)
	options := menuOptions(status)
	choice, err := selectMenuOption("Omanote", options)
	if err != nil || choice == "" {
		return err
	}

	switch {
	case strings.Contains(choice, "Open"):
		return launchTUI()
	case strings.Contains(choice, "Start virtual mic"):
		return notifyControl("start")
	case strings.Contains(choice, "Stop virtual mic"):
		return notifyControl("stop")
	case strings.Contains(choice, "Start recording"):
		return notifyControl("record-start")
	case strings.Contains(choice, "Stop recording"):
		return notifyControl("record-stop")
	case strings.Contains(choice, "Save recording"):
		return notifyControl("record-save")
	case strings.Contains(choice, "Discard recording"):
		return notifyControl("record-discard")
	case strings.Contains(choice, "Quit"):
		resp, err := sendControl("quit")
		if err != nil {
			return nil
		}
		if !resp.OK {
			notify("Omanote", resp.Error, true)
			return errors.New(resp.Error)
		}
		notify("Omanote", "Daemon stopped", false)
	}
	return nil
}

func menuOptions(status DaemonStatus) []string {
	options := []string{"󰆍  Open Omanote"}
	if status.RecState == "pending" {
		options = append(options, "󰆓  Save recording", "󰆴  Discard recording")
	}
	if status.RunState.Running {
		options = append(options, "󰓛  Stop virtual mic")
		switch status.RecState {
		case "recording":
			options = append(options, "󰓛  Stop recording")
		default:
			if status.RecState != "pending" {
				options = append(options, "󰑋  Start recording")
			}
		}
	} else {
		options = append(options, "󰐊  Start virtual mic")
	}
	if status.DaemonRunning {
		options = append(options, "󰩈  Quit Omanote")
	}
	return options
}

func selectMenuOption(prompt string, options []string) (string, error) {
	cmd := exec.Command("omarchy-launch-walker", "--dmenu", "--width", "295", "--minheight", "1", "--maxheight", "300", "-p", prompt+"...")
	cmd.Stdin = strings.NewReader(strings.Join(options, "\n"))
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func launchTUI() error {
	if commandExists("omarchy-launch-or-focus-tui") {
		return exec.Command("omarchy-launch-or-focus-tui", "omanote").Start()
	}
	return exec.Command("omanote").Start()
}

func notifyControl(command string) error {
	resp, err := sendControlAutoStart(command)
	if err != nil {
		notify("Omanote", err.Error(), true)
		return err
	}
	if !resp.OK {
		notify("Omanote", resp.Error, true)
		return errors.New(resp.Error)
	}
	if resp.Message != "" {
		notify("Omanote", resp.Message, false)
	}
	return nil
}

func notify(summary, body string, critical bool) {
	args := []string{summary, body}
	if critical {
		args = append(args, "-u", "critical")
	} else {
		args = append(args, "-u", "low")
	}
	exec.Command("notify-send", args...).Run()
}

func formatDuration(totalSeconds int) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	mins := totalSeconds / 60
	secs := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d", mins, secs)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: omanote [tui|daemon|status|start|stop|devices|record|menu|autostart|quit]

Commands:
  tui                         Open the floating TUI
  daemon                      Run the background controller
  status [--follow] [--json|--waybar]
  start [mic] [sink]          Start the virtual mic
  stop                        Stop the virtual mic
  devices [--json]            List selectable microphones and outputs
  record start|stop|save|discard
  menu                        Open the Quattro panel, or Walker if unavailable
  autostart enable|disable|status
  quit                        Stop the daemon`)
}
