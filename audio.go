package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const sinkName = "VirtualMic"

func cacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "omanote")
	os.MkdirAll(dir, 0o755)
	return dir
}

func pidFile() string    { return filepath.Join(cacheDir(), "pids") }
func moduleFile() string { return filepath.Join(cacheDir(), "module_id") }

type AudioDevices struct {
	DefaultSink   string
	DefaultSource string
	Err           error
}

func detectDevices() AudioDevices {
	var d AudioDevices

	sinkOut, err := exec.Command("pactl", "get-default-sink").Output()
	if err != nil {
		d.Err = fmt.Errorf("no default sink: %w", err)
		return d
	}
	d.DefaultSink = strings.TrimSpace(string(sinkOut))

	srcOut, err := exec.Command("pactl", "list", "sources", "short").Output()
	if err != nil {
		d.Err = fmt.Errorf("cannot list sources: %w", err)
		return d
	}
	for _, line := range strings.Split(string(srcOut), "\n") {
		if line == "" || strings.Contains(line, ".monitor") || strings.Contains(line, sinkName) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			d.DefaultSource = fields[1]
			break
		}
	}
	if d.DefaultSource == "" {
		d.Err = fmt.Errorf("no microphone found")
	}
	return d
}

type RunState struct {
	Running  bool
	MicPID   int
	SysPID   int
	ModuleID string
}

func checkRunState() RunState {
	var s RunState

	data, err := os.ReadFile(pidFile())
	if err != nil {
		return s
	}
	pids := strings.Fields(strings.TrimSpace(string(data)))

	allAlive := len(pids) > 0
	for i, p := range pids {
		pid, _ := strconv.Atoi(p)
		if err := syscall.Kill(pid, 0); err != nil {
			allAlive = false
		}
		if i == 0 {
			s.MicPID = pid
		}
		if i == 1 {
			s.SysPID = pid
		}
	}

	if modData, err := os.ReadFile(moduleFile()); err == nil {
		s.ModuleID = strings.TrimSpace(string(modData))
	}

	s.Running = allAlive
	if !allAlive {
		os.Remove(pidFile())
		os.Remove(moduleFile())
		s = RunState{}
	}
	return s
}

type StartResult struct {
	MicPID   int
	SysPID   int
	ModuleID string
}

func startVirtualMic(devices AudioDevices) (StartResult, error) {
	os.Remove(pidFile())
	os.Remove(moduleFile())

	modOut, err := exec.Command("pactl", "load-module", "module-null-sink",
		"sink_name="+sinkName,
		"sink_properties=device.description=VirtualMic",
		"channel_map=stereo",
	).Output()
	if err != nil {
		return StartResult{}, fmt.Errorf("failed to create null sink: %w", err)
	}
	moduleID := strings.TrimSpace(string(modOut))
	os.WriteFile(moduleFile(), []byte(moduleID), 0o644)

	micCmd := exec.Command("pw-loopback",
		"-C", devices.DefaultSource,
		"-P", sinkName,
		"-n", "omanote-mic",
	)
	micCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := micCmd.Start(); err != nil {
		return StartResult{}, fmt.Errorf("mic loopback failed: %w", err)
	}

	sysCmd := exec.Command("pw-loopback",
		"-C", devices.DefaultSink+".monitor",
		"-P", sinkName,
		"-n", "omanote-sys",
	)
	sysCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sysCmd.Start(); err != nil {
		syscall.Kill(micCmd.Process.Pid, syscall.SIGTERM)
		return StartResult{}, fmt.Errorf("system loopback failed: %w", err)
	}

	pidData := fmt.Sprintf("%d\n%d\n", micCmd.Process.Pid, sysCmd.Process.Pid)
	os.WriteFile(pidFile(), []byte(pidData), 0o644)

	return StartResult{
		MicPID:   micCmd.Process.Pid,
		SysPID:   sysCmd.Process.Pid,
		ModuleID: moduleID,
	}, nil
}

func stopVirtualMic() error {
	if data, err := os.ReadFile(pidFile()); err == nil {
		for _, p := range strings.Fields(string(data)) {
			pid, _ := strconv.Atoi(p)
			if pid > 0 {
				syscall.Kill(pid, syscall.SIGTERM)
			}
		}
		os.Remove(pidFile())
	}
	if data, err := os.ReadFile(moduleFile()); err == nil {
		modID := strings.TrimSpace(string(data))
		exec.Command("pactl", "unload-module", modID).Run()
		os.Remove(moduleFile())
	}
	return nil
}
