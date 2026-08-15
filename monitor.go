package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// AudioMonitor — captures real audio from PipeWire via parec
// ---------------------------------------------------------------------------

// AudioMonitor spawns a parec subprocess to record float32 PCM samples from
// a PipeWire/PulseAudio device and makes the latest chunk available for
// visualization.
type AudioMonitor struct {
	cmd        *exec.Cmd
	stdout     io.ReadCloser
	done       chan struct{}
	mu         sync.Mutex
	samples    []float64
	sampleC    chan []float64
	running    bool
	generation uint64
}

// NewAudioMonitor creates an idle AudioMonitor.
func NewAudioMonitor() *AudioMonitor {
	return &AudioMonitor{sampleC: make(chan []float64, 8)}
}

// Start spawns parec to capture audio from the given device.
// deviceName is used as-is (e.g. "OmanoteMix.monitor").
func (m *AudioMonitor) Start(deviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("audio monitor already running")
	}
drainSamples:
	for {
		select {
		case <-m.sampleC:
		default:
			break drainSamples
		}
	}

	cmd := exec.Command("parec",
		"--device="+deviceName,
		"--format=float32le",
		"--channels=1",
		"--rate=48000",
		"--latency-msec=50",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("parec stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		return fmt.Errorf("parec start: %w", err)
	}

	m.generation++
	generation := m.generation
	done := make(chan struct{})
	m.cmd = cmd
	m.stdout = stdout
	m.done = done
	m.running = true
	m.samples = nil

	go m.readLoop(cmd, stdout, generation, done)

	return nil
}

// readLoop continuously reads 2048 float32 samples (8192 bytes) from parec
// stdout and stores them as float64 under the mutex.
func (m *AudioMonitor) readLoop(cmd *exec.Cmd, stdout io.ReadCloser, generation uint64, done chan struct{}) {
	const (
		chunkSamples = 2048
		chunkBytes   = chunkSamples * 4 // float32 = 4 bytes
	)

	defer func() {
		cmd.Wait()
		m.mu.Lock()
		if m.generation == generation && m.cmd == cmd {
			m.cmd = nil
			m.stdout = nil
			m.done = nil
			m.running = false
			m.samples = nil
		}
		m.mu.Unlock()
		close(done)
	}()

	buf := make([]byte, chunkBytes)

	for {
		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			// Pipe closed or process exited — stop the loop.
			return
		}

		samples := make([]float64, chunkSamples)
		for i := range chunkSamples {
			bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
			samples[i] = float64(math.Float32frombits(bits))
		}

		m.mu.Lock()
		if m.generation != generation || !m.running {
			m.mu.Unlock()
			return
		}
		m.samples = samples
		sampleC := m.sampleC
		m.mu.Unlock()

		select {
		case sampleC <- samples:
		default:
		}
	}
}

// Stop kills the parec subprocess and cleans up.
func (m *AudioMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	cmd := m.cmd
	stdout := m.stdout
	done := m.done
	m.generation++
	m.cmd = nil
	m.stdout = nil
	m.done = nil
	m.running = false
	m.samples = nil
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
	if stdout != nil {
		stdout.Close()
	}
	if done != nil {
		<-done
	}
}

// Samples returns a copy of the latest audio samples.
// Returns nil if the monitor is not running.
func (m *AudioMonitor) Samples() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || len(m.samples) == 0 {
		return nil
	}

	out := make([]float64, len(m.samples))
	copy(out, m.samples)
	return out
}

// IsRunning reports whether the monitor is actively capturing audio.
func (m *AudioMonitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// SampleChan delivers fresh audio chunks as they are read from parec.
func (m *AudioMonitor) SampleChan() <-chan []float64 {
	return m.sampleC
}

// ---------------------------------------------------------------------------
// Bubbletea integration
// ---------------------------------------------------------------------------

// sampleMsg carries a snapshot of audio samples into the bubbletea Update loop.
type sampleMsg struct{ samples []float64 }

// cmdReadSamples returns a tea.Cmd that polls the AudioMonitor every 50ms
// and delivers the latest samples as a sampleMsg.
func cmdReadSamples(mon *AudioMonitor) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return sampleMsg{samples: mon.Samples()}
	})
}
