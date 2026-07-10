package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	sinkName     = "OmanoteMix"
	sourceName   = "Omanote"
	noDeviceName = "none"
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

func modulesFile() string { return filepath.Join(cacheDir(), "modules") }

type AudioDevice struct {
	Name        string
	Description string
}

func noneDevice() AudioDevice {
	return AudioDevice{Name: noDeviceName, Description: "None"}
}

func isNoDevice(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), noDeviceName)
}

func selectableDevices(devices []AudioDevice) []AudioDevice {
	withNone := make([]AudioDevice, 0, len(devices)+1)
	withNone = append(withNone, noneDevice())
	withNone = append(withNone, devices...)
	return withNone
}

func listSelectableSources() ([]AudioDevice, error) {
	devices, err := listSources()
	if err != nil {
		return nil, err
	}
	return selectableDevices(devices), nil
}

func listSelectableSinks() ([]AudioDevice, error) {
	devices, err := listSinks()
	if err != nil {
		return nil, err
	}
	return selectableDevices(devices), nil
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
		for i := len(rest) - 1; i >= 0; i-- {
			if rest[i] == '_' {
				candidate := rest[i+1:]
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

// RunState tracks the 4 PA modules: null-sink, remap-source, mic-loopback, sys-loopback.
type RunState struct {
	Running  bool
	SinkMod  string
	RemapMod string
	MicMod   string
	SysMod   string
}

func checkRunState() RunState {
	data, err := os.ReadFile(modulesFile())
	if err != nil {
		return RunState{}
	}

	state, ok := parseRunState(data)
	if !ok {
		os.Remove(modulesFile())
		return RunState{}
	}

	// Verify modules are still loaded
	modList, err := exec.Command("pactl", "list", "modules", "short").Output()
	if err != nil {
		return RunState{}
	}
	modStr := string(modList)
	for _, id := range state.moduleIDs() {
		if !moduleLoaded(modStr, id) {
			os.Remove(modulesFile())
			return RunState{}
		}
	}

	return state
}

type StartResult struct {
	SinkMod  string
	RemapMod string
	MicMod   string
	SysMod   string
}

func (s RunState) moduleIDs() []string {
	ids := make([]string, 0, 4)
	for _, id := range []string{s.SinkMod, s.RemapMod, s.MicMod, s.SysMod} {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func moduleLoaded(modulesShort, id string) bool {
	for _, line := range strings.Split(modulesShort, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == id {
			return true
		}
	}
	return false
}

func parseRunState(data []byte) (RunState, bool) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return RunState{}, false
	}

	lines := strings.Split(text, "\n")
	keyed := false
	for _, line := range lines {
		if strings.Contains(line, "=") {
			keyed = true
			break
		}
	}

	var state RunState
	if keyed {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return RunState{}, false
			}
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			switch strings.TrimSpace(key) {
			case "sink":
				state.SinkMod = value
			case "remap":
				state.RemapMod = value
			case "mic":
				state.MicMod = value
			case "sys":
				state.SysMod = value
			default:
				return RunState{}, false
			}
		}
	} else {
		ids := make([]string, 0, len(lines))
		for _, line := range lines {
			id := strings.TrimSpace(line)
			if id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) != 4 {
			return RunState{}, false
		}
		state = RunState{
			SinkMod:  ids[0],
			RemapMod: ids[1],
			MicMod:   ids[2],
			SysMod:   ids[3],
		}
	}

	if state.SinkMod == "" || state.RemapMod == "" {
		return RunState{}, false
	}
	state.Running = true
	return state, true
}

func writeRunState(state RunState) error {
	lines := []string{
		"sink=" + state.SinkMod,
		"remap=" + state.RemapMod,
	}
	if state.MicMod != "" {
		lines = append(lines, "mic="+state.MicMod)
	}
	if state.SysMod != "" {
		lines = append(lines, "sys="+state.SysMod)
	}
	return os.WriteFile(modulesFile(), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func loadModule(args ...string) (string, error) {
	out, err := exec.Command("pactl", append([]string{"load-module"}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func startVirtualMic(micDevice, outputDevice string) (StartResult, error) {
	os.Remove(modulesFile())
	if micDevice == "" {
		micDevice = noDeviceName
	}
	if outputDevice == "" {
		outputDevice = noDeviceName
	}

	var loaded []string
	cleanup := func() {
		for i := len(loaded) - 1; i >= 0; i-- {
			exec.Command("pactl", "unload-module", loaded[i]).Run()
		}
	}

	// 1. Create null sink (mixing point)
	sinkMod, err := loadModule("module-null-sink",
		"sink_name="+sinkName,
		"sink_properties=device.description="+sinkName,
		"channel_map=stereo",
	)
	if err != nil {
		return StartResult{}, fmt.Errorf("failed to create null sink: %w", err)
	}
	loaded = append(loaded, sinkMod)

	// 2. Create remap-source so "Omanote" appears as a selectable mic input
	remapMod, err := loadModule("module-remap-source",
		"source_name="+sourceName,
		"master="+sinkName+".monitor",
		"source_properties=device.description="+sourceName,
	)
	if err != nil {
		cleanup()
		return StartResult{}, fmt.Errorf("failed to create remap source: %w", err)
	}
	loaded = append(loaded, remapMod)

	// 3. Loopback: selected mic → virtual sink
	micMod := ""
	if !isNoDevice(micDevice) {
		micMod, err = loadModule("module-loopback",
			"source="+micDevice,
			"sink="+sinkName,
			"latency_msec=20",
		)
		if err != nil {
			cleanup()
			return StartResult{}, fmt.Errorf("mic loopback failed: %w", err)
		}
		loaded = append(loaded, micMod)
	}

	// 4. Loopback: selected output's monitor → virtual sink
	sysMod := ""
	if !isNoDevice(outputDevice) {
		sysMod, err = loadModule("module-loopback",
			"source="+outputDevice+".monitor",
			"sink="+sinkName,
			"latency_msec=20",
		)
		if err != nil {
			cleanup()
			return StartResult{}, fmt.Errorf("system loopback failed: %w", err)
		}
		loaded = append(loaded, sysMod)
	}

	// Persist all module IDs
	state := RunState{
		Running:  true,
		SinkMod:  sinkMod,
		RemapMod: remapMod,
		MicMod:   micMod,
		SysMod:   sysMod,
	}
	if err := writeRunState(state); err != nil {
		cleanup()
		return StartResult{}, fmt.Errorf("persist modules: %w", err)
	}

	return StartResult{
		SinkMod:  sinkMod,
		RemapMod: remapMod,
		MicMod:   micMod,
		SysMod:   sysMod,
	}, nil
}

func stopVirtualMic() error {
	data, err := os.ReadFile(modulesFile())
	if err != nil {
		return nil
	}
	state, ok := parseRunState(data)
	if !ok {
		os.Remove(modulesFile())
		return nil
	}
	ids := state.moduleIDs()
	// Unload in reverse order (sys-loopback, mic-loopback, remap, sink)
	for i := len(ids) - 1; i >= 0; i-- {
		exec.Command("pactl", "unload-module", ids[i]).Run()
	}
	os.Remove(modulesFile())
	return nil
}
