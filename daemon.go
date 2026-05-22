package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type daemonServer struct {
	mu           sync.Mutex
	cfg          Config
	runState     RunState
	mon          *AudioMonitor
	rec          *Recorder
	recState     recState
	recStart     time.Time
	recElapsed   time.Duration
	lastSaved    string
	err          error
	done         chan struct{}
	shutdownOnce sync.Once
	sampleOnce   sync.Once
}

func runDaemon() error {
	if _, err := sendControl("status"); err == nil {
		return fmt.Errorf("omanote daemon is already running")
	}

	socketPath := daemonSocketPath()
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	defer os.Remove(socketPath)

	server := newDaemonServer()
	defer server.shutdown()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigC)
	go func() {
		<-sigC
		server.stop()
	}()

	go func() {
		<-server.done
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-server.done:
				return nil
			default:
				return fmt.Errorf("accept control connection: %w", err)
			}
		}
		go server.handleConn(conn)
	}
}

func newDaemonServer() *daemonServer {
	d := &daemonServer{
		cfg:      loadConfig(),
		runState: checkRunState(),
		mon:      NewAudioMonitor(),
		rec:      &Recorder{},
		recState: recOff,
		done:     make(chan struct{}),
	}
	if d.runState.Running {
		d.startMonitorLocked()
	}
	return d
}

func (d *daemonServer) handleConn(conn net.Conn) {
	defer conn.Close()

	var req controlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(controlResponse{OK: false, Error: err.Error(), Status: d.status()})
		return
	}

	resp := d.handle(req)
	json.NewEncoder(conn).Encode(resp)
}

func (d *daemonServer) handle(req controlRequest) controlResponse {
	var message string
	var err error

	switch req.Command {
	case "status":
	case "start":
		mic, out := "", ""
		if len(req.Args) > 0 {
			mic = req.Args[0]
		}
		if len(req.Args) > 1 {
			out = req.Args[1]
		}
		err = d.startVirtualMic(mic, out)
		message = "Omanote virtual mic started"
	case "stop":
		err = d.stopVirtualMic()
		message = "Omanote virtual mic stopped"
	case "record-start":
		err = d.startRecording()
		message = "Recording started"
	case "record-stop":
		err = d.stopRecording()
		message = "Recording stopped"
	case "record-save":
		message, err = d.saveRecording()
	case "record-discard":
		err = d.discardRecording()
		message = "Recording discarded"
	case "set-output-dir":
		if len(req.Args) == 0 || req.Args[0] == "" {
			err = fmt.Errorf("missing output directory")
		} else {
			err = d.setOutputDir(req.Args[0])
			message = "Output directory updated"
		}
	case "quit":
		err = d.quit()
		message = "Omanote daemon stopped"
	default:
		err = fmt.Errorf("unknown command %q", req.Command)
	}

	if err != nil {
		d.setErr(err)
		return controlResponse{OK: false, Error: err.Error(), Status: d.status()}
	}
	if req.Command != "status" {
		d.setErr(nil)
	}
	return controlResponse{OK: true, Message: message, Status: d.status()}
}

func (d *daemonServer) status() DaemonStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked()
}

func (d *daemonServer) statusLocked() DaemonStatus {
	elapsed := d.recElapsed
	if d.recState == recOn {
		elapsed = time.Since(d.recStart)
	}
	errText := ""
	if d.err != nil {
		errText = d.err.Error()
	}
	return DaemonStatus{
		DaemonRunning:    true,
		RunState:         d.runState,
		RecState:         recStateName(d.recState),
		RecElapsedSecs:   int(elapsed.Seconds()),
		OutputDir:        d.cfg.OutputDir,
		LastSaved:        d.lastSaved,
		Error:            errText,
		MonitorAvailable: d.mon.IsRunning(),
	}
}

func (d *daemonServer) setErr(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

func (d *daemonServer) startVirtualMic(micDevice, outputDevice string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.runState.Running {
		return nil
	}

	if micDevice == "" || outputDevice == "" {
		mic, out, err := d.resolvePreferredDevicesLocked()
		if err != nil {
			return err
		}
		if micDevice == "" {
			micDevice = mic
		}
		if outputDevice == "" {
			outputDevice = out
		}
	}

	result, err := startVirtualMic(micDevice, outputDevice)
	if err != nil {
		return err
	}
	d.runState = RunState{
		Running:  true,
		SinkMod:  result.SinkMod,
		RemapMod: result.RemapMod,
		MicMod:   result.MicMod,
		SysMod:   result.SysMod,
	}
	d.cfg.PreferredSource = micDevice
	d.cfg.PreferredSink = outputDevice
	saveConfig(d.cfg)
	d.startMonitorLocked()
	return nil
}

func (d *daemonServer) resolvePreferredDevicesLocked() (string, string, error) {
	sources, srcErr := listSources()
	sinks, sinkErr := listSinks()
	if srcErr != nil {
		return "", "", srcErr
	}
	if sinkErr != nil {
		return "", "", sinkErr
	}
	if len(sources) == 0 {
		return "", "", fmt.Errorf("no microphone sources found")
	}
	if len(sinks) == 0 {
		return "", "", fmt.Errorf("no output sinks found")
	}
	sourceIdx := findDevice(sources, d.cfg.PreferredSource, getDefaultSource())
	sinkIdx := findDevice(sinks, d.cfg.PreferredSink, getDefaultSink())
	return sources[sourceIdx].Name, sinks[sinkIdx].Name, nil
}

func (d *daemonServer) stopVirtualMic() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.recState == recOn {
		d.rec.Stop()
		d.recElapsed = time.Since(d.recStart)
		d.recState = recPrompt
	}
	d.mon.Stop()
	if err := stopVirtualMic(); err != nil {
		return err
	}
	d.runState = RunState{}
	return nil
}

func (d *daemonServer) startRecording() error {
	if err := d.startVirtualMic("", ""); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recState == recPrompt {
		return fmt.Errorf("save or discard the pending recording first")
	}
	if d.recState == recOn {
		return nil
	}
	if err := d.rec.Start(); err != nil {
		return err
	}
	d.recState = recOn
	d.recStart = time.Now()
	d.recElapsed = 0
	return nil
}

func (d *daemonServer) stopRecording() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recState != recOn {
		return nil
	}
	d.rec.Stop()
	d.recElapsed = time.Since(d.recStart)
	d.recState = recPrompt
	return nil
}

func (d *daemonServer) saveRecording() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recState == recOn {
		d.rec.Stop()
		d.recElapsed = time.Since(d.recStart)
		d.recState = recPrompt
	}
	if d.recState != recPrompt {
		return "", fmt.Errorf("no recording waiting to save")
	}
	path, err := d.rec.Save(d.cfg.OutputDir)
	if err != nil {
		return "", err
	}
	d.recState = recOff
	d.recElapsed = 0
	d.lastSaved = path
	return fmt.Sprintf("Recording saved to %s", path), nil
}

func (d *daemonServer) discardRecording() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recState == recOff {
		return nil
	}
	d.rec.Discard()
	d.recState = recOff
	d.recElapsed = 0
	return nil
}

func (d *daemonServer) setOutputDir(dir string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg.OutputDir = dir
	saveConfig(d.cfg)
	return nil
}

func (d *daemonServer) quit() error {
	if err := d.stopVirtualMic(); err != nil {
		return err
	}
	if err := d.discardRecording(); err != nil {
		return err
	}
	d.stop()
	return nil
}

func (d *daemonServer) startMonitorLocked() {
	if d.mon.IsRunning() {
		return
	}
	if err := d.mon.Start(sinkName + ".monitor"); err != nil {
		d.err = err
		return
	}
	d.sampleOnce.Do(func() {
		go d.recordSamples()
	})
}

func (d *daemonServer) recordSamples() {
	for samples := range d.mon.SampleChan() {
		d.mu.Lock()
		rec := d.rec
		recording := d.recState == recOn
		d.mu.Unlock()
		if recording {
			rec.WriteSamples(samples)
		}
	}
}

func (d *daemonServer) stop() {
	d.shutdownOnce.Do(func() {
		close(d.done)
	})
}

func (d *daemonServer) shutdown() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recState != recOff {
		d.rec.Discard()
		d.recState = recOff
	}
	d.mon.Stop()
	if d.runState.Running || checkRunState().Running {
		if err := stopVirtualMic(); err != nil && !errors.Is(err, os.ErrNotExist) {
			d.err = err
		}
	}
	d.runState = RunState{}
}

func recStateName(state recState) string {
	switch state {
	case recOn:
		return "recording"
	case recPrompt:
		return "pending"
	default:
		return "off"
	}
}

func recStateFromName(name string) recState {
	switch name {
	case "recording":
		return recOn
	case "pending":
		return recPrompt
	default:
		return recOff
	}
}
