package main

import "testing"

func TestControlTimeoutAllowsLongAudioActions(t *testing.T) {
	for _, command := range []string{"start", "stop", "record-start", "record-save", "quit"} {
		if got := controlTimeout(command); got != controlActionTimeout {
			t.Errorf("controlTimeout(%q) = %s, want %s", command, got, controlActionTimeout)
		}
	}
	for _, command := range []string{"status", "record-stop", "devices", "unknown"} {
		if got := controlTimeout(command); got != controlDefaultTimeout {
			t.Errorf("controlTimeout(%q) = %s, want %s", command, got, controlDefaultTimeout)
		}
	}
}
