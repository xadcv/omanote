package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const daemonSocketFile = "daemon.sock"

type controlRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type controlResponse struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
	Status  DaemonStatus `json:"status"`
}

type DaemonStatus struct {
	DaemonRunning    bool     `json:"daemon_running"`
	RunState         RunState `json:"run_state"`
	RecState         string   `json:"rec_state"`
	RecElapsedSecs   int      `json:"rec_elapsed_secs"`
	OutputDir        string   `json:"output_dir"`
	LastSaved        string   `json:"last_saved,omitempty"`
	Error            string   `json:"error,omitempty"`
	MonitorAvailable bool     `json:"monitor_available"`
	PreferredSource  string   `json:"preferred_source,omitempty"`
	PreferredSink    string   `json:"preferred_sink,omitempty"`
	Autostart        bool     `json:"autostart"`
}

func runtimeDir() string {
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" {
		dir := filepath.Join(base, "omanote")
		os.MkdirAll(dir, 0o700)
		return dir
	}
	return cacheDir()
}

func daemonSocketPath() string {
	return filepath.Join(runtimeDir(), daemonSocketFile)
}

func sendControl(command string, args ...string) (controlResponse, error) {
	conn, err := net.DialTimeout("unix", daemonSocketPath(), 750*time.Millisecond)
	if err != nil {
		return controlResponse{}, err
	}
	defer conn.Close()

	req := controlRequest{Command: command, Args: args}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return controlResponse{}, err
	}

	var resp controlResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return controlResponse{}, err
	}
	if !resp.OK && resp.Error == "" {
		resp.Error = "command failed"
	}
	return resp, nil
}

func sendControlAutoStart(command string, args ...string) (controlResponse, error) {
	resp, err := sendControl(command, args...)
	if err == nil {
		return resp, nil
	}
	if err := startDaemonProcess(); err != nil {
		return controlResponse{}, err
	}
	if err := waitForDaemon(3 * time.Second); err != nil {
		return controlResponse{}, err
	}
	return sendControl(command, args...)
}

func daemonStatus(autoStart bool) (DaemonStatus, error) {
	var resp controlResponse
	var err error
	if autoStart {
		resp, err = sendControlAutoStart("status")
	} else {
		resp, err = sendControl("status")
	}
	if err != nil {
		if autoStart {
			return DaemonStatus{}, err
		}
		cfg := loadConfig()
		return DaemonStatus{
			DaemonRunning:   false,
			RecState:        recStateName(recOff),
			OutputDir:       cfg.OutputDir,
			PreferredSource: cfg.PreferredSource,
			PreferredSink:   cfg.PreferredSink,
		}, nil
	}
	if !resp.OK {
		return resp.Status, errors.New(resp.Error)
	}
	return resp.Status, nil
}

func startDaemonProcess() error {
	if _, err := sendControl("status"); err == nil {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	logPath := filepath.Join(cacheDir(), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}

	cmd := exec.Command(exe, "daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logFile.Close()
		return fmt.Errorf("release daemon process: %w", err)
	}
	logFile.Close()
	return nil
}

func waitForDaemon(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := sendControl("status"); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(75 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("daemon did not become ready: %w", lastErr)
	}
	return fmt.Errorf("daemon did not become ready")
}
