package main

import (
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

func containsOption(options []string, needle string) bool {
	for _, option := range options {
		if strings.Contains(option, needle) {
			return true
		}
	}
	return false
}
