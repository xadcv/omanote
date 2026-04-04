# TUI Visualizer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform Omanote TUI into a full-screen animated audio visualizer with real FFT-based spectrum analysis, inspired by cliamp.

**Architecture:** `parec` subprocess captures audio from `OmanoteMix.monitor`, a goroutine reads float32 PCM samples and delivers them to the TUI via bubbletea messages, a Visualizer struct runs FFT and renders 5 modes (bars, wave, flame, retro, pulse) using braille/block characters. Full-screen alternate mode with centered 80-char frame.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, `github.com/madelynnblue/go-dsp` (FFT), `parec` (PipeWire audio capture)

---

### Task 1: Add go-dsp dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the FFT dependency**

Run: `go get github.com/madelynnblue/go-dsp@latest`

**Step 2: Verify it resolves**

Run: `go mod tidy && go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add go-dsp dependency for FFT audio analysis"
```

---

### Task 2: Create visualizer.go — FFT analysis + all render modes

**Files:**
- Create: `visualizer.go`

Port cliamp's visualizer system, adapted to Omanote's synthwave palette. This is the largest task — all rendering logic lives here.

**Step 1: Write visualizer.go**

The file must contain:

1. **Constants and types:**
   - `numBands = 10`, `fftSize = 2048`, `defaultVisRows = 8`, `panelWidth = 74`
   - `VisMode` enum: `VisBars`, `VisBricks`, `VisColumns`, `VisWave`, `VisScatter`, `VisFlame`, `VisRetro`, `VisPulse`, `VisNone`
   - `barBlocks` slice: `" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"`
   - `brailleBit` lookup table for 4x2 dot grid

2. **Spectrum color styles** using Omanote's synthwave palette:
   - `specLow` = `#04B575` (green)
   - `specMid` = `#C774E8` (purple)
   - `specHigh` = `#FF6AD5` (hot pink)

3. **Visualizer struct** (same as cliamp):
   - `prev [numBands]float64` for temporal smoothing
   - `sr float64` (sample rate)
   - `buf []float64` (reusable FFT buffer)
   - `Mode VisMode`
   - `Rows int`
   - `waveBuf []float64` (raw samples for wave mode)
   - `frame uint64`

4. **Methods:**
   - `NewVisualizer(sampleRate float64) *Visualizer`
   - `CycleMode()`
   - `ModeName() string`
   - `Analyze(samples []float64) [numBands]float64` — Hann window, FFT, band binning, dB normalization, temporal smoothing (fast attack / slow decay)
   - `Render(bands [numBands]float64) string` — dispatches to active mode

5. **Render modes** (port directly from cliamp, adjusting `panelWidth` constant):
   - `renderBars(bands)` — smooth fractional Unicode blocks
   - `renderBricks(bands)` — solid bricks with gaps
   - `renderColumns(bands)` — thin interpolated columns
   - `renderWave()` — braille oscilloscope from raw waveBuf
   - `renderScatter(bands)` — twinkling braille particles
   - `renderFlame(bands)` — rising fire tendrils
   - `renderRetro(bands)` — synthwave grid + sun + wave
   - `renderPulse(bands)` — pulsating filled circle

6. **Helpers:**
   - `visBandWidth(b int) int`
   - `specStyle(rowBottom float64) lipgloss.Style`
   - `scatterHash(band, row, col int, frame uint64) float64`

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: clean build (visualizer not yet wired to model)

**Step 3: Commit**

```bash
git add visualizer.go
git commit -m "Add FFT visualizer with 8 render modes"
```

---

### Task 3: Create monitor.go — parec subprocess + sample reader

**Files:**
- Create: `monitor.go`

**Step 1: Write monitor.go**

The file must contain:

1. **`AudioMonitor` struct:**
   ```go
   type AudioMonitor struct {
       cmd     *exec.Cmd
       stdout  io.ReadCloser
       mu      sync.Mutex
       samples []float64  // latest chunk
       running bool
   }
   ```

2. **`NewAudioMonitor()` constructor**

3. **`Start(deviceName string) error`:**
   - Spawn: `parec --device=<deviceName>.monitor --format=float32le --channels=1 --rate=48000 --latency-msec=50`
   - Pipe stdout
   - Launch goroutine that reads 2048 float32 samples (8192 bytes) per chunk into `samples` under mutex

4. **`Stop()`:**
   - Kill subprocess, close pipe, set running=false

5. **`Samples() []float64`:**
   - Lock mutex, return copy of latest samples

6. **Bubbletea integration — `sampleMsg` type and `cmdReadSamples` command:**
   ```go
   type sampleMsg struct{ samples []float64 }

   func cmdReadSamples(mon *AudioMonitor) tea.Cmd {
       return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
           return sampleMsg{samples: mon.Samples()}
       })
   }
   ```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add monitor.go
git commit -m "Add parec audio monitor for real-time sample capture"
```

---

### Task 4: Rewrite model.go — full-screen layout + visualizer integration

**Files:**
- Modify: `model.go`

**Step 1: Update model struct**

Add fields:
```go
vis     *Visualizer
mon     *AudioMonitor
bands   [10]float64
visMode VisMode  // tracked via vis.Mode
```

**Step 2: Update Init()**

- Create visualizer: `vis: NewVisualizer(48000)`
- Create monitor: `mon: NewAudioMonitor()`

**Step 3: Update Update()**

- Add `"v"` key handler: `m.vis.CycleMode()`
- Add `sampleMsg` handler: run `m.bands = m.vis.Analyze(msg.samples)`, return `cmdReadSamples(m.mon)`
- On `startedMsg` success: call `m.mon.Start(sinkName)` and return `cmdReadSamples(m.mon)`
- On `stoppedMsg`: call `m.mon.Stop()`
- Change animation tick from 500ms to 50ms for smooth 20 FPS visualizer

**Step 4: Rewrite View()**

Full-screen centered layout (80-char panel like cliamp):

```
[rainbow logo]
[blank line]
[visualizer — vis.Render(bands)]
[blank line]
[status box — running/stopped with device names]
[blank line]
[device panels side by side — Microphone | System Audio]
[blank line]
[help bar with v mode indicator]
```

Key changes:
- Use `lipgloss.Place()` or manual centering to center the frame
- Device panels rendered side-by-side using `lipgloss.JoinHorizontal`
- Status box shows selected device descriptions instead of module IDs
- Help bar includes `v <ModeName>` indicator
- Frame padding with `lipgloss.NewStyle().Padding(1, 3).Width(80)`

**Step 5: Verify it compiles and runs**

Run: `go build -o omanote . && echo "build OK"`
Expected: clean build

**Step 6: Commit**

```bash
git add model.go
git commit -m "Rewrite TUI: full-screen visualizer with real audio FFT"
```

---

### Task 5: End-to-end test + install

**Step 1: Clean build**

Run: `go build -o omanote .`

**Step 2: Manual smoke test**

Run `./omanote` in a real terminal:
- Verify full-screen alternate mode
- Verify device lists populate
- Press Enter to start — verify visualizer animates with real audio
- Press `v` to cycle through all modes
- Press Enter to stop — verify visualizer decays
- Press `q` to quit cleanly

**Step 3: Install and push**

```bash
go install .
git push origin master
```

---

### Task 6: Update bash script and README

**Files:**
- Modify: `omanote.sh` (minor — already done, no changes needed)
- Modify: `README.md` (update to document visualizer modes and new keybindings)

**Step 1: Update README**

Add visualizer section documenting:
- The 8 visualization modes
- `v` key to cycle modes
- Audio monitoring via parec
- Full keybinding table

**Step 2: Commit and push**

```bash
git add README.md
git commit -m "Update README with visualizer documentation"
git push origin master
```
