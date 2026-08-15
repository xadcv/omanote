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
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg
	}
	return cfg
}

func saveConfig(cfg Config) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(configDir(), ".config.toml.tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, configFile())
}
