package main

import (
	"os"
	"testing"
)

func TestSaveConfigRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := Config{
		OutputDir:       "/tmp/recordings",
		VisMode:         "Wave",
		ColorScheme:     "Synthwave",
		PreferredSource: "test-source",
		PreferredSink:   "test-sink",
	}

	if err := saveConfig(want); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	if got := loadConfig(); got != want {
		t.Fatalf("loadConfig() = %#v, want %#v", got, want)
	}
}

func TestLoadConfigRejectsMalformedFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	data := "output_dir = \"/preserved\"\nvis_mode = [\"invalid type\"]\n"
	if err := os.WriteFile(configFile(), []byte(data), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	want := defaultConfig()
	want.OutputDir = "/preserved"
	if got := loadConfig(); got != want {
		t.Fatalf("loadConfig() = %#v, want valid fields preserved as %#v", got, want)
	}
}
