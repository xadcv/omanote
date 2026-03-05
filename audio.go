package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	sinkName   = "OmanoteMix"
	sourceName = "Omanote"
)

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

func pidFile() string         { return filepath.Join(cacheDir(), "pids") }
func moduleFile() string      { return filepath.Join(cacheDir(), "module_id") }
func remapModuleFile() string { return filepath.Join(cacheDir(), "remap_module_id") }

type AudioDevice struct {
	Name        string
	Description string
}

type pactlDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// cleanDescription returns a usable description, falling back to
// a human-readable form derived from the pactl device name.
func cleanDescription(desc, name string) string {
	if desc != "" && desc != "(null)" {
		return desc
	}
	// Extract the USB product portion from names like:
	//   alsa_input.usb-R__DE_Microphones_R__DE_NT-USB_Mini_48B8D6F7-00.mono-fallback
	if i := strings.Index(name, "usb-"); i >= 0 {
		rest := name[i+4:]
		// Cut at the profile suffix (.mono-fallback, .analog-stereo, etc.)
		if dot := strings.Index(rest, "."); dot > 0 {
			rest = rest[:dot]
		}
		// Remove serial and interface suffix (e.g. _48B8D6F7-00)
		// Pattern: _HEXSERIAL-NN at the end
		for i := len(rest) - 1; i >= 0; i-- {
			if rest[i] == '_' {
				candidate := rest[i+1:]
				// Check if it looks like SERIAL-NN (hex chars, dash, digits)
				if len(candidate) >= 4 && isHexish(candidate) {
					rest = rest[:i]
					break
				}
			}
		}
		rest = strings.NewReplacer("__", "", "_", " ", "-", " ").Replace(rest)
		return strings.TrimSpace(rest)
	}
	// Fallback: strip common prefixes and clean up.
	for _, prefix := range []string{"alsa_input.", "alsa_output.", "bluez_input.", "bluez_output."} {
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

// isHexish checks if a string looks like a hex serial (possibly with dashes/digits).
func isHexish(s string) bool {
	hexCount := 0
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'F', c >= 'a' && c <= 'f':
			hexCount++
		case c == '-':
			// allowed separator
		default:
			return false
		}
	}
	return hexCount >= 4
}

func listSources() ([]AudioDevice, error) {
	out, err := exec.Command("pactl", "-f", "json", "list", "sources").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list sources: %w", err)
	}
	var raw []pactlDevice
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse sources: %w", err)
	}
	var devices []AudioDevice
	for _, s := range raw {
		if strings.Contains(s.Name, ".monitor") || strings.Contains(s.Name, sinkName) || s.Name == sourceName {
			continue
		}
		devices = append(devices, AudioDevice{Name: s.Name, Description: cleanDescription(s.Description, s.Name)})
	}
	return devices, nil
}

func listSinks() ([]AudioDevice, error) {
	out, err := exec.Command("pactl", "-f", "json", "list", "sinks").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list sinks: %w", err)
	}
	var raw []pactlDevice
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse sinks: %w", err)
	}
	var devices []AudioDevice
	for _, s := range raw {
		if s.Name == sinkName {
			continue
		}
		devices = append(devices, AudioDevice{Name: s.Name, Description: cleanDescription(s.Description, s.Name)})
	}
	return devices, nil
}

func getDefaultSource() string {
	out, _ := exec.Command("pactl", "get-default-source").Output()
	return strings.TrimSpace(string(out))
}

func getDefaultSink() string {
	out, _ := exec.Command("pactl", "get-default-sink").Output()
	return strings.TrimSpace(string(out))
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
		os.Remove(remapModuleFile())
		s = RunState{}
	}
	return s
}

type StartResult struct {
	MicPID   int
	SysPID   int
	ModuleID string
}

func startVirtualMic(micDevice, outputDevice string) (StartResult, error) {
	os.Remove(pidFile())
	os.Remove(moduleFile())
	os.Remove(remapModuleFile())

	// 1. Create null sink (mixing point)
	modOut, err := exec.Command("pactl", "load-module", "module-null-sink",
		"sink_name="+sinkName,
		"sink_properties=device.description="+sinkName,
		"channel_map=stereo",
	).Output()
	if err != nil {
		return StartResult{}, fmt.Errorf("failed to create null sink: %w", err)
	}
	moduleID := strings.TrimSpace(string(modOut))
	os.WriteFile(moduleFile(), []byte(moduleID), 0o644)

	// 2. Create remap-source so "Omanote" appears as a selectable mic input
	remapOut, err := exec.Command("pactl", "load-module", "module-remap-source",
		"source_name="+sourceName,
		"master="+sinkName+".monitor",
		"source_properties=device.description="+sourceName,
	).Output()
	if err != nil {
		exec.Command("pactl", "unload-module", moduleID).Run()
		os.Remove(moduleFile())
		return StartResult{}, fmt.Errorf("failed to create remap source: %w", err)
	}
	remapModuleID := strings.TrimSpace(string(remapOut))
	os.WriteFile(remapModuleFile(), []byte(remapModuleID), 0o644)

	// 3. Loopback: selected mic → virtual sink
	micCmd := exec.Command("pw-loopback",
		"-C", micDevice,
		"-P", sinkName,
		"-n", "omanote-mic",
	)
	micCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := micCmd.Start(); err != nil {
		exec.Command("pactl", "unload-module", remapModuleID).Run()
		exec.Command("pactl", "unload-module", moduleID).Run()
		os.Remove(moduleFile())
		os.Remove(remapModuleFile())
		return StartResult{}, fmt.Errorf("mic loopback failed: %w", err)
	}

	// 4. Loopback: selected output's monitor → virtual sink
	sysCmd := exec.Command("pw-loopback",
		"-C", outputDevice+".monitor",
		"-P", sinkName,
		"-n", "omanote-sys",
	)
	sysCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sysCmd.Start(); err != nil {
		syscall.Kill(micCmd.Process.Pid, syscall.SIGTERM)
		exec.Command("pactl", "unload-module", remapModuleID).Run()
		exec.Command("pactl", "unload-module", moduleID).Run()
		os.Remove(pidFile())
		os.Remove(moduleFile())
		os.Remove(remapModuleFile())
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
	if data, err := os.ReadFile(remapModuleFile()); err == nil {
		modID := strings.TrimSpace(string(data))
		exec.Command("pactl", "unload-module", modID).Run()
		os.Remove(remapModuleFile())
	}
	if data, err := os.ReadFile(moduleFile()); err == nil {
		modID := strings.TrimSpace(string(data))
		exec.Command("pactl", "unload-module", modID).Run()
		os.Remove(moduleFile())
	}
	return nil
}
