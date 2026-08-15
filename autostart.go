package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const autostartServiceName = "omanote.service"

func systemdUserDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user")
}

func autostartServicePath() string {
	return filepath.Join(systemdUserDir(), autostartServiceName)
}

func renderAutostartService(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=Omanote background controller

[Service]
Type=simple
ExecStart=%s daemon
Restart=on-failure

[Install]
WantedBy=default.target
`, systemdQuote(exe))
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	if strings.ContainsAny(value, " \t\"'\\") {
		return strconv.Quote(value)
	}
	return value
}

func setAutostartEnabled(enabled bool) error {
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("find executable: %w", err)
		}
		if err := os.MkdirAll(systemdUserDir(), 0o755); err != nil {
			return fmt.Errorf("create systemd user dir: %w", err)
		}
		if err := os.WriteFile(autostartServicePath(), []byte(renderAutostartService(exe)), 0o644); err != nil {
			return fmt.Errorf("write service file: %w", err)
		}
		if err := runSystemctlUser("daemon-reload"); err != nil {
			return err
		}
		return runSystemctlUser("enable", autostartServiceName)
	}

	disableErr := runSystemctlUser("disable", autostartServiceName)
	os.Remove(autostartServicePath())
	reloadErr := runSystemctlUser("daemon-reload")
	if disableErr != nil {
		return disableErr
	}
	return reloadErr
}

func autostartEnabled() bool {
	err := exec.Command("systemctl", "--user", "is-enabled", "--quiet", autostartServiceName).Run()
	return err == nil
}

func runSystemctlUser(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
