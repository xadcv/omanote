package main

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	OutputDir       string `toml:"output_dir"`
	VisMode         string `toml:"vis_mode"`
	ColorScheme     string `toml:"color_scheme"`
	PreferredSource string `toml:"preferred_source"`
	PreferredSink   string `toml:"preferred_sink"`
}

func defaultConfig() Config {
	return Config{
		OutputDir:   defaultOutputDir(),
		VisMode:     "Bars",
		ColorScheme: "Synthwave",
	}
}

func configDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "omanote")
}

func configFile() string {
	return filepath.Join(configDir(), "config.toml")
}

func loadConfig() Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFile())
	if err != nil {
		return cfg
	}
	toml.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg Config) {
	os.MkdirAll(configDir(), 0o755)
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Encode(cfg)
	os.WriteFile(configFile(), buf.Bytes(), 0o644)
}
