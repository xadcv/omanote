# Recording Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add optional WAV recording to omanote — toggle with `r`, save/discard prompt, configurable output directory.

**Architecture:** New `recorder.go` handles WAV file writing via stream-to-temp-file approach. `model.go` gains recording state machine (off/on/prompt) and UI changes. Audio samples already flow through `sampleMsg` — we tap into that path to feed the recorder. No changes to audio pipeline or visualizer.

**Tech Stack:** Go stdlib only (`encoding/binary`, `os`, `io`, `math`, `time`, `sync`, `fmt`, `path/filepath`). Bubble Tea v2 for TUI integration.

---

### Task 1: Recorder Core — WAV Writer

**Files:**
- Create: `recorder.go`
- Create: `recorder_test.go`

**Step 1: Write the failing test for WAV header**

```go
// recorder_test.go
package main

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestRecorder_StartCreatesValidWAVHeader(t *testing.T) {
	rec := &Recorder{}
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rec.Discard()

	// Read the temp file and verify WAV header
	f, err := os.Open(rec.file.Name())
	if err != nil {
		t.Fatalf("open temp: %v", err)
	}
	defer f.Close()

	var header [44]byte
	if _, err := f.Read(header[:]); err != nil {
		t.Fatal(err)
	}

	// RIFF header
	if string(header[0:4]) != "RIFF" {
		t.Errorf("expected RIFF, got %q", header[0:4])
	}
	if string(header[8:12]) != "WAVE" {
		t.Errorf("expected WAVE, got %q", header[8:12])
	}
	// fmt chunk
	if string(header[12:16]) != "fmt " {
		t.Errorf("expected 'fmt ', got %q", header[12:16])
	}
	// Audio format: 3 = IEEE float
	audioFmt := binary.LittleEndian.Uint16(header[20:22])
	if audioFmt != 3 {
		t.Errorf("expected audio format 3 (float), got %d", audioFmt)
	}
	// Channels: 1
	channels := binary.LittleEndian.Uint16(header[22:24])
	if channels != 1 {
		t.Errorf("expected 1 channel, got %d", channels)
	}
	// Sample rate: 48000
	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	if sampleRate != 48000 {
		t.Errorf("expected 48000 Hz, got %d", sampleRate)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_StartCreatesValidWAVHeader -v`
Expected: FAIL — `Recorder` type not defined

**Step 3: Write minimal Recorder with Start**

```go
// recorder.go
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Recorder streams audio samples to a temporary WAV file.
type Recorder struct {
	mu        sync.Mutex
	file      *os.File
	dataSize  uint32
	startTime time.Time
	recording bool
}

const (
	wavSampleRate = 48000
	wavChannels   = 1
	wavBitsPerSample = 32
	wavHeaderSize = 44
)

// Start creates a temp file and writes a placeholder WAV header.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("already recording")
	}

	f, err := os.CreateTemp("", "omanote-*.wav")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if err := writeWAVHeader(f, 0); err != nil {
		f.Close()
		os.Remove(f.Name())
		return fmt.Errorf("write WAV header: %w", err)
	}

	r.file = f
	r.dataSize = 0
	r.startTime = time.Now()
	r.recording = true
	return nil
}

func writeWAVHeader(f *os.File, dataSize uint32) error {
	byteRate := uint32(wavSampleRate * wavChannels * wavBitsPerSample / 8)
	blockAlign := uint16(wavChannels * wavBitsPerSample / 8)

	header := make([]byte, wavHeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)   // file size - 8
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)           // fmt chunk size
	binary.LittleEndian.PutUint16(header[20:22], 3)            // IEEE float
	binary.LittleEndian.PutUint16(header[22:24], wavChannels)
	binary.LittleEndian.PutUint32(header[24:28], wavSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], wavBitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	_, err := f.Write(header)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_StartCreatesValidWAVHeader -v`
Expected: PASS

**Step 5: Commit**

```bash
git add recorder.go recorder_test.go
git commit -m "feat: add Recorder with WAV header writing"
```

---

### Task 2: Recorder — WriteSamples and Duration

**Files:**
- Modify: `recorder.go`
- Modify: `recorder_test.go`

**Step 1: Write the failing test for WriteSamples**

```go
// append to recorder_test.go
func TestRecorder_WriteSamples(t *testing.T) {
	rec := &Recorder{}
	if err := rec.Start(); err != nil {
		t.Fatal(err)
	}
	defer rec.Discard()

	// Write 100 samples of a 440Hz sine wave
	samples := make([]float64, 100)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / 48000)
	}
	rec.WriteSamples(samples)

	// Verify file grew by 100 * 4 bytes = 400 bytes
	info, err := rec.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	expectedSize := int64(wavHeaderSize + 100*4)
	if info.Size() != expectedSize {
		t.Errorf("expected file size %d, got %d", expectedSize, info.Size())
	}

	if !rec.IsRecording() {
		t.Error("expected IsRecording() to be true")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_WriteSamples -v`
Expected: FAIL — `WriteSamples` not defined

**Step 3: Implement WriteSamples, IsRecording, Duration**

Add to `recorder.go`:

```go
// WriteSamples converts float64 samples to float32le and appends to the WAV data.
func (r *Recorder) WriteSamples(samples []float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.recording || r.file == nil {
		return
	}

	buf := make([]byte, len(samples)*4)
	for i, s := range samples {
		bits := math.Float32bits(float32(s))
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], bits)
	}

	n, err := r.file.Write(buf)
	if err != nil {
		return
	}
	r.dataSize += uint32(n)
}

// IsRecording reports whether the recorder is actively capturing.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Duration returns how long the current recording has been running.
func (r *Recorder) Duration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return 0
	}
	return time.Since(r.startTime)
}

// Discard stops recording and deletes the temp file.
func (r *Recorder) Discard() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recording = false
	if r.file != nil {
		name := r.file.Name()
		r.file.Close()
		os.Remove(name)
		r.file = nil
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_WriteSamples -v`
Expected: PASS

**Step 5: Commit**

```bash
git add recorder.go recorder_test.go
git commit -m "feat: add WriteSamples, IsRecording, Duration, Discard to Recorder"
```

---

### Task 3: Recorder — Stop, Save, Finalize

**Files:**
- Modify: `recorder.go`
- Modify: `recorder_test.go`

**Step 1: Write the failing test for Save**

```go
// append to recorder_test.go
func TestRecorder_Save(t *testing.T) {
	rec := &Recorder{}
	if err := rec.Start(); err != nil {
		t.Fatal(err)
	}

	// Write some samples
	samples := make([]float64, 48000) // 1 second
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / 48000)
	}
	rec.WriteSamples(samples)
	rec.Stop()

	// Save to temp dir
	destDir := t.TempDir()
	path, err := rec.Save(destDir)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists in destDir
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}

	// Verify filename pattern
	base := filepath.Base(path)
	if len(base) < len("omanote-2026-04-04-12-00-00.wav") {
		t.Errorf("unexpected filename: %s", base)
	}

	// Read back and verify WAV header has correct data size
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var header [44]byte
	f.Read(header[:])

	dataSize := binary.LittleEndian.Uint32(header[40:44])
	expectedDataSize := uint32(48000 * 4)
	if dataSize != expectedDataSize {
		t.Errorf("WAV data size: expected %d, got %d", expectedDataSize, dataSize)
	}

	riffSize := binary.LittleEndian.Uint32(header[4:8])
	if riffSize != 36+expectedDataSize {
		t.Errorf("RIFF size: expected %d, got %d", 36+expectedDataSize, riffSize)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_Save -v`
Expected: FAIL — `Stop` and `Save` not defined

**Step 3: Implement Stop and Save**

Add to `recorder.go`:

```go
// Stop halts recording but keeps the temp file for save/discard.
func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recording = false
}

// Save finalizes the WAV header and moves the temp file to destDir.
// Returns the path of the saved file.
func (r *Recorder) Save(destDir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file == nil {
		return "", fmt.Errorf("no recording to save")
	}

	// Seek to start and rewrite header with correct sizes
	if _, err := r.file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("seek: %w", err)
	}
	if err := writeWAVHeader(r.file, r.dataSize); err != nil {
		return "", fmt.Errorf("finalize header: %w", err)
	}
	r.file.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Generate timestamp filename
	ts := r.startTime.Format("2006-01-02-15-04-05")
	destPath := filepath.Join(destDir, fmt.Sprintf("omanote-%s.wav", ts))

	// Move temp file to destination
	src := r.file.Name()
	if err := os.Rename(src, destPath); err != nil {
		// Rename may fail across filesystems — fall back to copy
		if err := copyFile(src, destPath); err != nil {
			return "", fmt.Errorf("move file: %w", err)
		}
		os.Remove(src)
	}

	r.file = nil
	return destPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = out.ReadFrom(in)
	return err
}
```

**Step 4: Run all recorder tests**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add recorder.go recorder_test.go
git commit -m "feat: add Stop, Save with WAV header finalization"
```

---

### Task 4: Recorder — Discard Test

**Files:**
- Modify: `recorder_test.go`

**Step 1: Write test for Discard**

```go
func TestRecorder_Discard(t *testing.T) {
	rec := &Recorder{}
	if err := rec.Start(); err != nil {
		t.Fatal(err)
	}

	tmpPath := rec.file.Name()

	// Verify temp file exists
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("temp file should exist: %v", err)
	}

	rec.Discard()

	// Verify temp file is deleted
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be deleted after Discard")
	}

	if rec.IsRecording() {
		t.Error("should not be recording after Discard")
	}
}
```

**Step 2: Run test**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test -run TestRecorder_Discard -v`
Expected: PASS (Discard already implemented in Task 2)

**Step 3: Commit**

```bash
git add recorder_test.go
git commit -m "test: add Discard test for Recorder"
```

---

### Task 5: Model — Add Recording State and Fields

**Files:**
- Modify: `model.go:12-37` (types and struct)
- Modify: `model.go:190-201` (initialModel)

**Step 1: Add recording state type and model fields**

Add after the `appState` constants (line 18):

```go
type recState int

const (
	recOff    recState = iota // not recording
	recOn                     // actively recording
	recPrompt                 // showing save/discard prompt
)
```

Add new fields to `model` struct:

```go
type model struct {
	// ... existing fields ...
	rec        *Recorder
	recState   recState
	recStart   time.Time
	outputDir  string
	editingDir bool
	dirInput   string
}
```

Update `initialModel()` to set default output dir:

```go
func defaultOutputDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Recordings")
}

func initialModel() model {
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6AD5"))),
	)
	return model{
		state:     stateIdle,
		spinner:   s,
		vis:       NewVisualizer(48000),
		mon:       NewAudioMonitor(),
		rec:       &Recorder{},
		outputDir: defaultOutputDir(),
	}
}
```

Add required imports: `"os"`, `"path/filepath"`.

**Step 2: Verify it compiles**

Run: `cd /home/adcv/Projects/xadcv/omanote && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add model.go
git commit -m "feat: add recording state and output dir fields to model"
```

---

### Task 6: Model — Recording Key Bindings

**Files:**
- Modify: `model.go:220-265` (Update, KeyPressMsg handler)

**Step 1: Add recording key handlers**

Replace the `"r"` case (line 263-264) and add new cases in the `KeyPressMsg` switch:

```go
case "r":
	if m.recState == recPrompt {
		return m, nil // must save/discard first
	}
	if m.recState == recOn {
		// Stop recording → show prompt
		rec := m.rec
		rec.Stop()
		m.recState = recPrompt
		return m, nil
	}
	if m.runState.Running {
		// Start recording
		if err := m.rec.Start(); err != nil {
			m.err = err
			return m, nil
		}
		m.recState = recOn
		m.recStart = time.Now()
		return m, nil
	}
	// Not running — refresh devices (old behavior)
	return m, tea.Batch(cmdListDevices, cmdCheckStatus)
case "R":
	return m, tea.Batch(cmdListDevices, cmdCheckStatus)
case "s":
	if m.recState == recPrompt {
		path, err := m.rec.Save(m.outputDir)
		if err != nil {
			m.err = err
		} else {
			m.err = nil
			_ = path // saved successfully
		}
		m.recState = recOff
		return m, nil
	}
case "d":
	if m.recState == recPrompt {
		m.rec.Discard()
		m.recState = recOff
		return m, nil
	}
case "o":
	if m.runState.Running && m.recState != recPrompt && !m.editingDir {
		m.editingDir = true
		m.dirInput = m.outputDir
		return m, nil
	}
```

Also update the `"enter", " "` case to handle dir editing and stop-while-recording:

In the `"enter", " "` handler, add at the top:
```go
if m.editingDir {
	m.outputDir = m.dirInput
	m.editingDir = false
	return m, nil
}
```

In the `"q", "ctrl+c"` handler, add cleanup:
```go
case "q", "ctrl+c":
	if m.editingDir {
		m.editingDir = false
		return m, nil
	}
	if m.recState == recOn {
		m.rec.Discard()
	} else if m.recState == recPrompt {
		m.rec.Discard()
	}
	m.mon.Stop()
	return m, tea.Quit
```

In the `"escape"` handler (new):
```go
case "escape":
	if m.editingDir {
		m.editingDir = false
		return m, nil
	}
```

Add text input handling for dir editing — at the top of KeyPressMsg, before the switch:
```go
if m.editingDir {
	key := msg.String()
	switch key {
	case "enter":
		m.outputDir = m.dirInput
		m.editingDir = false
	case "escape":
		m.editingDir = false
	case "backspace":
		if len(m.dirInput) > 0 {
			m.dirInput = m.dirInput[:len(m.dirInput)-1]
		}
	default:
		if len(key) == 1 {
			m.dirInput += key
		}
	}
	return m, nil
}
```

**Step 2: Wire samples to recorder in sampleMsg handler**

Modify the `sampleMsg` case (line 267-269):

```go
case sampleMsg:
	m.bands = m.vis.Analyze(msg.samples)
	if m.recState == recOn {
		m.rec.WriteSamples(msg.samples)
	}
	return m, cmdReadSamples(m.mon)
```

**Step 3: Auto-stop recording when virtual mic stops**

Modify the `stoppedMsg` case (line 319-324):

```go
case stoppedMsg:
	m.state = stateIdle
	m.err = msg.err
	m.runState = RunState{}
	m.mon.Stop()
	if m.recState == recOn {
		m.rec.Stop()
		m.recState = recPrompt
	}
	return m, nil
```

**Step 4: Verify it compiles**

Run: `cd /home/adcv/Projects/xadcv/omanote && go build ./...`
Expected: Success

**Step 5: Commit**

```bash
git add model.go
git commit -m "feat: add recording key bindings and sample routing"
```

---

### Task 7: Model — Recording UI (Status + Help Bar)

**Files:**
- Modify: `model.go:376-413` (status box in View)
- Modify: `model.go:495-507` (help bar in View)

**Step 1: Add recording style**

Add to the styles block (~line 128):

```go
recStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FF4444"))
```

**Step 2: Update status box to show recording indicator**

In the `View()` method, inside `if m.runState.Running` block (after the `** Omanote is LIVE **` line), add recording info:

```go
if m.recState == recOn {
	elapsed := time.Since(m.recStart)
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60
	statusContent.WriteString("  " + recStyle.Render(fmt.Sprintf("● REC %02d:%02d", mins, secs)))
}
```

After the mic/sys device lines, show output dir when recording or in prompt:

```go
if m.recState != recOff {
	if m.editingDir {
		statusContent.WriteString("\n")
		statusContent.WriteString(labelStyle.Render("  dir ") + valueStyle.Render(m.dirInput+"_"))
	} else {
		statusContent.WriteString("\n")
		statusContent.WriteString(labelStyle.Render("  dir ") + dimValueStyle.Render(m.outputDir))
	}
}
```

If in `recPrompt`, show prompt message:

```go
if m.recState == recPrompt {
	elapsed := time.Since(m.recStart)
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60
	statusContent.WriteString("\n")
	statusContent.WriteString(recStyle.Render(fmt.Sprintf("  Recording stopped (%02d:%02d)", mins, secs)))
}
```

**Step 3: Update help bar**

Replace the help bar section:

```go
// --- Help bar ---
var help strings.Builder
if m.recState == recPrompt {
	help.WriteString(keyStyle.Render("s") + keyDescStyle.Render(" save"))
	help.WriteString("  " + keyStyle.Render("d") + keyDescStyle.Render(" discard"))
} else if m.editingDir {
	help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" confirm"))
	help.WriteString("  " + keyStyle.Render("esc") + keyDescStyle.Render(" cancel"))
} else {
	if m.runState.Running {
		help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" stop"))
		if m.recState == recOn {
			help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" stop rec"))
		} else {
			help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" rec"))
		}
		help.WriteString("  " + keyStyle.Render("o") + keyDescStyle.Render(" output"))
	} else {
		help.WriteString(keyStyle.Render("enter") + keyDescStyle.Render(" start"))
		help.WriteString("  " + keyStyle.Render("r") + keyDescStyle.Render(" refresh"))
	}
	help.WriteString("  " + keyStyle.Render("v") + keyDescStyle.Render(" "+m.vis.ModeName()))
	if !m.runState.Running {
		help.WriteString("  " + keyStyle.Render("tab") + keyDescStyle.Render(" switch"))
		help.WriteString("  " + keyStyle.Render("↑↓") + keyDescStyle.Render(" select"))
	}
	help.WriteString("  " + keyStyle.Render("q") + keyDescStyle.Render(" quit"))
}
b.WriteString(help.String())
b.WriteString("\n")
```

**Step 4: Add required imports to model.go**

Add `"fmt"` and `"time"` to the import block (time is already there, fmt may not be).

**Step 5: Verify it compiles and run manually**

Run: `cd /home/adcv/Projects/xadcv/omanote && go build ./...`
Expected: Success

**Step 6: Commit**

```bash
git add model.go
git commit -m "feat: add recording status display and updated help bar"
```

---

### Task 8: Smoke Test and Final Verification

**Files:**
- All modified files

**Step 1: Run all tests**

Run: `cd /home/adcv/Projects/xadcv/omanote && go test ./... -v`
Expected: All PASS

**Step 2: Run vet and build**

Run: `cd /home/adcv/Projects/xadcv/omanote && go vet ./... && go build ./...`
Expected: No errors

**Step 3: Manual test checklist**

1. Start omanote — verify default UI unchanged (no recording indicators)
2. Start virtual mic — verify `r rec` appears in help bar
3. Press `r` — verify `● REC 00:00` appears, help shows `r stop rec`
4. Press `r` again — verify save/discard prompt appears
5. Press `s` — verify file saved to `~/Recordings/omanote-<timestamp>.wav`
6. Play the WAV file — verify audio is audible
7. Repeat recording, press `d` — verify file discarded
8. Press `o` while running — verify dir editing works
9. Stop virtual mic while recording — verify auto-prompts save/discard
10. Press `q` while recording — verify clean exit, temp file removed

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: address smoke test issues"
```
