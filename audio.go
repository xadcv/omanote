package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	sinkName                = "OmanoteMix"
	sourceName              = "Omanote"
	noDeviceName            = "none"
	pipeWireDummySinkName   = "auto_null"
	systemLoopbackLatencyMS = "40"
)

func cacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "omanote")
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	return dir
}

func modulesFile() string { return filepath.Join(cacheDir(), "modules") }

type AudioDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type pactlSinkInput struct {
	Index       int             `json:"index"`
	OwnerModule json.RawMessage `json:"owner_module"`
	Sink        int             `json:"sink"`
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

func runPactl(args ...string) error {
	out, err := exec.Command("pactl", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

var errSinkMissing = errors.New("sink not found")

func setDefaultSink(sink string) error {
	if sink == "" {
		return fmt.Errorf("missing sink")
	}
	return runPactl("set-default-sink", sink)
}

func sinkExistsChecked(name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	_, err := sinkIndexByName(name)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errSinkMissing):
		return false, nil
	default:
		return false, err
	}
}

func listSinkInputs() ([]pactlSinkInput, error) {
	out, err := exec.Command("pactl", "-f", "json", "list", "sink-inputs").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list sink inputs: %w", err)
	}
	var inputs []pactlSinkInput
	if err := json.Unmarshal(out, &inputs); err != nil {
		return nil, fmt.Errorf("cannot parse sink inputs: %w", err)
	}
	return inputs, nil
}

func sinkIndexByName(name string) (int, error) {
	out, err := exec.Command("pactl", "-f", "json", "list", "sinks").Output()
	if err != nil {
		return 0, fmt.Errorf("cannot list sinks: %w", err)
	}
	var sinks []pactlDevice
	if err := json.Unmarshal(out, &sinks); err != nil {
		return 0, fmt.Errorf("cannot parse sinks: %w", err)
	}
	for _, sink := range sinks {
		if sink.Name == name {
			return sink.Index, nil
		}
	}
	return 0, fmt.Errorf("%w: %s", errSinkMissing, name)
}

func sinkInputOwnerModule(input pactlSinkInput) string {
	if len(input.OwnerModule) == 0 || string(input.OwnerModule) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(input.OwnerModule, &text); err == nil {
		return text
	}
	var numeric int
	if err := json.Unmarshal(input.OwnerModule, &numeric); err == nil {
		return strconv.Itoa(numeric)
	}
	return strings.Trim(string(input.OwnerModule), `"`)
}

func moveSinkInputs(fromSink, targetSink string, skipModules ...string) error {
	targetIndex, err := sinkIndexByName(targetSink)
	if err != nil {
		return err
	}

	fromIndex := -1
	if fromSink != "" {
		fromIndex, err = sinkIndexByName(fromSink)
		if err != nil {
			return err
		}
	}

	skip := make(map[string]bool, len(skipModules))
	for _, id := range skipModules {
		id = strings.TrimSpace(id)
		if id != "" {
			skip[id] = true
		}
	}

	inputs, err := listSinkInputs()
	if err != nil {
		return err
	}

	var firstErr error
	for _, input := range inputs {
		if skip[sinkInputOwnerModule(input)] {
			continue
		}
		if fromIndex >= 0 && input.Sink != fromIndex {
			continue
		}
		if input.Sink == targetIndex {
			continue
		}
		if err := runPactl("move-sink-input", strconv.Itoa(input.Index), targetSink); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("move sink input %d to %s: %w", input.Index, targetSink, err)
		}
	}
	return firstErr
}

func restoreSystemAudioRoute(defaultSink string, skipModules ...string) error {
	if defaultSink == "" {
		return nil
	}
	defaultSinkExists, err := sinkExistsChecked(defaultSink)
	if err != nil {
		return fmt.Errorf("check default sink %s: %w", defaultSink, err)
	}
	if !defaultSinkExists {
		return fmt.Errorf("%w: %s", errSinkMissing, defaultSink)
	}
	var firstErr error
	if err := setDefaultSink(defaultSink); err != nil {
		firstErr = fmt.Errorf("restore default sink %s: %w", defaultSink, err)
	}
	mixSinkExists, err := sinkExistsChecked(sinkName)
	if err != nil && firstErr == nil {
		firstErr = fmt.Errorf("check mix sink: %w", err)
	}
	if mixSinkExists {
		if err := moveSinkInputs(sinkName, defaultSink, skipModules...); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func classifyRestoreErr(err error, defaultSink string, sinkPresent bool) (retry, ignore bool) {
	if err == nil || !errors.Is(err, errSinkMissing) {
		return false, false
	}
	if sinkPresent {
		return true, false
	}
	if defaultSink == pipeWireDummySinkName {
		return false, true
	}
	return false, false
}

// finishSinkRestore retries a missing-sink restore after OmanoteMix is gone.
// PipeWire's dummy auto_null sink disappears while another sink is default and
// comes back after unload.
func finishSinkRestore(err error, defaultSink string) error {
	if err == nil || defaultSink == "" || !errors.Is(err, errSinkMissing) {
		return err
	}
	sinkPresent, checkErr := sinkExistsChecked(defaultSink)
	if checkErr != nil {
		return errors.Join(err, fmt.Errorf("check restored sink %s: %w", defaultSink, checkErr))
	}
	retry, ignore := classifyRestoreErr(err, defaultSink, sinkPresent)
	switch {
	case ignore:
		return nil
	case retry:
		return restoreSystemAudioRoute(defaultSink)
	default:
		return err
	}
}

// RunState tracks PA modules plus any default sink Omanote temporarily replaced.
type RunState struct {
	Running          bool   `json:"running"`
	SinkMod          string `json:"sink_mod,omitempty"`
	RemapMod         string `json:"remap_mod,omitempty"`
	MicMod           string `json:"mic_mod,omitempty"`
	SysMod           string `json:"sys_mod,omitempty"`
	SavedDefaultSink string `json:"saved_default_sink,omitempty"`
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

func omanoteModuleIDs() ([]string, error) {
	out, err := exec.Command("pactl", "list", "modules", "short").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list modules: %w", err)
	}
	return omanoteModuleIDsFromShort(string(out)), nil
}

func omanoteModuleIDsFromShort(modulesShort string) []string {
	var ids []string
	for _, line := range strings.Split(modulesShort, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		id, args := strings.TrimSpace(parts[0]), parts[2]
		if id == "" {
			continue
		}
		if strings.Contains(args, sinkName) || strings.Contains(args, "source_name="+sourceName) {
			ids = append(ids, id)
		}
	}
	return ids
}

func unloadOmanoteModules() error {
	ids, err := omanoteModuleIDs()
	if err != nil {
		return err
	}
	for i := len(ids) - 1; i >= 0; i-- {
		exec.Command("pactl", "unload-module", ids[i]).Run()
	}
	return nil
}

type StartResult struct {
	SinkMod          string
	RemapMod         string
	MicMod           string
	SysMod           string
	SavedDefaultSink string
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
			case "default_sink":
				state.SavedDefaultSink = value
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
	if state.SavedDefaultSink != "" {
		lines = append(lines, "default_sink="+state.SavedDefaultSink)
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
	if err := unloadOmanoteModules(); err != nil {
		return StartResult{}, err
	}
	os.Remove(modulesFile())
	if micDevice == "" {
		micDevice = noDeviceName
	}
	if outputDevice == "" {
		outputDevice = noDeviceName
	}

	savedDefaultSink := ""
	if !isNoDevice(outputDevice) {
		savedDefaultSink = getDefaultSink()
		if savedDefaultSink == "" {
			return StartResult{}, fmt.Errorf("cannot determine default sink")
		}
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

	// 4. Route app playback through OmanoteMix, then play it to the selected output.
	sysMod := ""
	if !isNoDevice(outputDevice) {
		sysMod, err = loadModule("module-loopback",
			"source="+sinkName+".monitor",
			"sink="+outputDevice,
			"latency_msec="+systemLoopbackLatencyMS,
		)
		if err != nil {
			cleanup()
			return StartResult{}, fmt.Errorf("system output loopback failed: %w", err)
		}
		loaded = append(loaded, sysMod)

		if err := setDefaultSink(sinkName); err != nil {
			cleanup()
			return StartResult{}, fmt.Errorf("set default sink to %s: %w", sinkName, err)
		}
		if err := moveSinkInputs("", sinkName, micMod, sysMod); err != nil {
			restoreSystemAudioRoute(savedDefaultSink, micMod, sysMod)
			cleanup()
			return StartResult{}, fmt.Errorf("move playback streams to %s: %w", sinkName, err)
		}
	}

	// Persist all module IDs
	state := RunState{
		Running:          true,
		SinkMod:          sinkMod,
		RemapMod:         remapMod,
		MicMod:           micMod,
		SysMod:           sysMod,
		SavedDefaultSink: savedDefaultSink,
	}
	if err := writeRunState(state); err != nil {
		restoreSystemAudioRoute(savedDefaultSink, micMod, sysMod)
		cleanup()
		return StartResult{}, fmt.Errorf("persist modules: %w", err)
	}

	return StartResult{
		SinkMod:          sinkMod,
		RemapMod:         remapMod,
		MicMod:           micMod,
		SysMod:           sysMod,
		SavedDefaultSink: savedDefaultSink,
	}, nil
}

func stopVirtualMic() error {
	data, err := os.ReadFile(modulesFile())
	if err != nil {
		return unloadOmanoteModules()
	}
	state, ok := parseRunState(data)
	if !ok {
		os.Remove(modulesFile())
		return unloadOmanoteModules()
	}
	restoreErr := restoreSystemAudioRoute(state.SavedDefaultSink, state.MicMod, state.SysMod)
	ids := state.moduleIDs()
	// Unload in reverse order (sys-loopback, mic-loopback, remap, sink)
	for i := len(ids) - 1; i >= 0; i-- {
		exec.Command("pactl", "unload-module", ids[i]).Run()
	}
	cleanupErr := unloadOmanoteModules()
	os.Remove(modulesFile())
	restoreErr = finishSinkRestore(restoreErr, state.SavedDefaultSink)
	if restoreErr != nil {
		return restoreErr
	}
	return cleanupErr
}
