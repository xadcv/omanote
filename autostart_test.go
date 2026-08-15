package main

import (
	"strings"
	"testing"
)

func TestRenderAutostartServiceQuotesExecutable(t *testing.T) {
	got := renderAutostartService("/tmp/Omanote 100% Build/omanote")
	if !strings.Contains(got, `ExecStart="/tmp/Omanote 100%% Build/omanote" daemon`) {
		t.Fatalf("service file did not quote executable path:\n%s", got)
	}
	if !strings.Contains(got, "WantedBy=default.target") {
		t.Fatalf("service file missing install target:\n%s", got)
	}
}
