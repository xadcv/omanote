package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatWaybarStatusStates(t *testing.T) {
	tests := []struct {
		name   string
		status DaemonStatus
		want   string
	}{
		{
			name:   "inactive",
			status: DaemonStatus{DaemonRunning: false, RecState: "off"},
			want:   `"alt":"inactive"`,
		},
		{
			name: "live",
			status: DaemonStatus{
				DaemonRunning: true,
				RunState:      RunState{Running: true},
				RecState:      "off",
			},
			want: `"alt":"live"`,
		},
		{
			name: "recording",
			status: DaemonStatus{
				DaemonRunning:  true,
				RunState:       RunState{Running: true},
				RecState:       "recording",
				RecElapsedSecs: 65,
			},
			want: `"alt":"recording"`,
		},
		{
			name: "pending",
			status: DaemonStatus{
				DaemonRunning: true,
				RunState:      RunState{Running: true},
				RecState:      "pending",
			},
			want: `"alt":"pending"`,
		},
		{
			name: "error",
			status: DaemonStatus{
				DaemonRunning: true,
				Error:         "boom",
			},
			want: `"alt":"error"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatWaybarStatus(tt.status)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("formatWaybarStatus() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMenuOptionsReflectState(t *testing.T) {
	idle := menuOptions(DaemonStatus{DaemonRunning: true, RecState: "off"})
	if !containsOption(idle, "Start virtual mic") {
		t.Fatalf("idle options = %v, want Start virtual mic", idle)
	}

	recording := menuOptions(DaemonStatus{
		DaemonRunning: true,
		RunState:      RunState{Running: true},
		RecState:      "recording",
	})
	if !containsOption(recording, "Stop recording") {
		t.Fatalf("recording options = %v, want Stop recording", recording)
	}

	pending := menuOptions(DaemonStatus{
		DaemonRunning: true,
		RunState:      RunState{Running: true},
		RecState:      "pending",
	})
	if !containsOption(pending, "Save recording") || !containsOption(pending, "Discard recording") {
		t.Fatalf("pending options = %v, want save and discard", pending)
	}
}

func TestStatusAltStates(t *testing.T) {
	tests := []struct {
		name   string
		status DaemonStatus
		want   string
	}{
		{name: "inactive", status: DaemonStatus{RecState: "off"}, want: "inactive"},
		{name: "idle", status: DaemonStatus{DaemonRunning: true, RecState: "off"}, want: "idle"},
		{name: "live", status: DaemonStatus{DaemonRunning: true, RunState: RunState{Running: true}, RecState: "off"}, want: "live"},
		{name: "recording", status: DaemonStatus{DaemonRunning: true, RunState: RunState{Running: true}, RecState: "recording"}, want: "recording"},
		{name: "pending", status: DaemonStatus{DaemonRunning: true, RunState: RunState{Running: true}, RecState: "pending"}, want: "pending"},
		{name: "error", status: DaemonStatus{DaemonRunning: true, Error: "boom"}, want: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusAlt(tt.status); got != tt.want {
				t.Fatalf("statusAlt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatJSONStatusPayload(t *testing.T) {
	got := formatJSONStatus(DaemonStatus{
		DaemonRunning:   true,
		RunState:        RunState{Running: true, SinkMod: "12"},
		RecState:        "recording",
		RecElapsedSecs:  65,
		OutputDir:       "/tmp/recordings",
		PreferredSource: "alsa_input.usb",
		PreferredSink:   "none",
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("formatJSONStatus() produced invalid JSON %s: %v", got, err)
	}
	if payload["state"] != "recording" {
		t.Fatalf("state = %v, want recording", payload["state"])
	}
	if payload["preferred_source"] != "alsa_input.usb" {
		t.Fatalf("preferred_source = %v", payload["preferred_source"])
	}
	if payload["preferred_sink"] != "none" {
		t.Fatalf("preferred_sink = %v", payload["preferred_sink"])
	}
	if _, ok := payload["autostart"]; !ok {
		t.Fatalf("autostart missing from %s", got)
	}
	runState, ok := payload["run_state"].(map[string]any)
	if !ok {
		t.Fatalf("run_state = %T, want object", payload["run_state"])
	}
	if runState["running"] != true {
		t.Fatalf("run_state.running = %v, want true", runState["running"])
	}
}

func TestFormatDevicesJSONShape(t *testing.T) {
	got := formatDevicesJSON(
		[]AudioDevice{{Name: "none", Description: "None"}, {Name: "mic", Description: "USB Mic"}},
		[]AudioDevice{{Name: "none", Description: "None"}},
	)

	var payload deviceList
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("formatDevicesJSON() produced invalid JSON %s: %v", got, err)
	}
	if len(payload.Sources) != 2 || payload.Sources[0].Name != "none" || payload.Sources[1].Description != "USB Mic" {
		t.Fatalf("sources = %#v", payload.Sources)
	}
	if len(payload.Sinks) != 1 || payload.Sinks[0].Name != "none" {
		t.Fatalf("sinks = %#v", payload.Sinks)
	}
}

func TestFormatDevicesJSONEmptySlices(t *testing.T) {
	got := formatDevicesJSON(nil, nil)
	if !strings.Contains(got, `"sources":[]`) || !strings.Contains(got, `"sinks":[]`) {
		t.Fatalf("formatDevicesJSON(nil, nil) = %s", got)
	}
}

func containsOption(options []string, needle string) bool {
	for _, option := range options {
		if strings.Contains(option, needle) {
			return true
		}
	}
	return false
}
