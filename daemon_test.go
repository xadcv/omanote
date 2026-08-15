package main

import (
	"strings"
	"testing"
)

func TestStartRecordingRejectsPendingBeforeStartingVirtualMic(t *testing.T) {
	d := &daemonServer{
		mon:      NewAudioMonitor(),
		rec:      &Recorder{},
		recState: recPrompt,
		done:     make(chan struct{}),
	}

	err := d.startRecording()
	if err == nil || !strings.Contains(err.Error(), "save or discard") {
		t.Fatalf("startRecording() error = %v, want pending-recording error", err)
	}
	if d.runState.Running {
		t.Fatal("startRecording() started the virtual mic despite a pending recording")
	}
}
