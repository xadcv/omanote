package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func installFakeParec(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parec")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake parec: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAudioMonitorMarksNaturalProcessExit(t *testing.T) {
	installFakeParec(t, "exit 0")
	mon := NewAudioMonitor()
	if err := mon.Start("test.monitor"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(mon.Stop)

	deadline := time.Now().Add(time.Second)
	for mon.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if mon.IsRunning() {
		t.Fatal("monitor remained running after parec exited")
	}
}

func TestAudioMonitorCanRestartAfterStop(t *testing.T) {
	installFakeParec(t, "exec sleep 30")
	mon := NewAudioMonitor()

	for cycle := 0; cycle < 2; cycle++ {
		if err := mon.Start("test.monitor"); err != nil {
			t.Fatalf("Start cycle %d: %v", cycle, err)
		}
		if !mon.IsRunning() {
			t.Fatalf("monitor not running in cycle %d", cycle)
		}
		mon.Stop()
		if mon.IsRunning() {
			t.Fatalf("monitor still running after Stop in cycle %d", cycle)
		}
	}
}

func TestAudioMonitorRejectsConcurrentStart(t *testing.T) {
	installFakeParec(t, "exec sleep 30")
	mon := NewAudioMonitor()
	defer mon.Stop()

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- mon.Start("test.monitor")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent Start results: successes=%d failures=%d", successes, failures)
	}
}
