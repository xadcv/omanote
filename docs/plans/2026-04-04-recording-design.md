# Recording / Save Feature Design

**Date:** 2026-04-04
**Status:** Approved

## Overview

Add optional audio recording to omanote. When the virtual mic is live, the user can toggle recording on/off with `r`. On stop, they choose to save or discard. Recordings are WAV files saved to a configurable folder (default `~/Recordings/`).

Default behavior (no recording) is unchanged.

## Approach: Stream to Temp File

While recording, stream raw PCM samples to a temp WAV file in real-time. On save, finalize the WAV header (seek back and write correct data length) and move the file to the destination folder. On discard, delete the temp file.

**Why this approach:** Constant memory usage, supports arbitrarily long recordings. The trade-off is slightly more complexity (WAV header fixup on finalize).

## WAV Format

- Sample rate: 48000 Hz
- Channels: 1 (mono)
- Bit depth: 32-bit float (IEEE 754) — matches `parec` output format
- Container: RIFF WAV with `fmt ` and `data` chunks

The WAV header is 44 bytes. We write it with placeholder lengths on start, then seek back to fix `RIFF` chunk size and `data` chunk size on finalize.

## New File: `recorder.go`

```go
type Recorder struct {
    mu        sync.Mutex
    file      *os.File    // temp file
    dataSize  uint32      // bytes written to data chunk
    startTime time.Time
    recording bool
}
```

Key methods:
- `Start() error` — create temp file in os.TempDir(), write WAV header placeholder
- `WriteSamples(samples []float64)` — convert float64 → float32le, append to file
- `Stop()` — stop accepting samples (but don't close file yet)
- `Duration() time.Duration` — elapsed recording time
- `Save(destDir string) (string, error)` — finalize WAV header, move file to `destDir/omanote-YYYY-MM-DD-HH-MM-SS.wav`
- `Discard()` — delete temp file
- `IsRecording() bool`

## Model Changes (`model.go`)

### New State

```go
type recState int
const (
    recOff     recState = iota  // not recording
    recOn                        // actively recording
    recPrompt                    // stopped, showing save/discard prompt
)
```

New fields on `model`:
- `rec *Recorder`
- `recState recState`
- `recStart time.Time` (for elapsed display)
- `outputDir string` (default `~/Recordings/`, user-configurable)
- `editingDir bool` (true when user is editing the output dir path)
- `dirInput string` (text buffer for editing)

### Key Bindings

| Key | Condition | Action |
|-----|-----------|--------|
| `r` | Virtual mic running, recOff | Start recording |
| `r` | Virtual mic running, recOn | Stop recording, enter recPrompt |
| `s` | recPrompt | Save recording to outputDir |
| `d` | recPrompt | Discard recording |
| `o` | recOff or recOn | Toggle output dir editing |
| `R` | Any (replaces old `r`) | Refresh device list |

When `editingDir` is true, keypresses go to the text input instead of normal bindings. `enter` confirms, `esc` cancels.

### Recording Data Flow

The `sampleMsg` handler in `Update()` already receives audio samples every 50ms. When `recState == recOn`, also pass samples to `rec.WriteSamples()`. No changes to `monitor.go` needed.

```
sampleMsg received
  → vis.Analyze(samples)     // existing
  → if recOn: rec.WriteSamples(samples)  // new
```

### Status Display

When recording, add a red indicator to the status box:

```
  ** Omanote is LIVE **          ● REC 01:23
  mic RODE NT-USB Mini
  sys Built-in Audio
```

The `● REC` text uses a red style. Duration updates every animation tick (50ms, but display in seconds).

When in `recPrompt` state, replace the help bar:

```
  Recording stopped (01:23)  s save  d discard
```

### Output Dir Display

Show the current output directory in the status area (subtle, only when relevant):

```
  ** Omanote is LIVE **          ● REC 01:23
  mic RODE NT-USB Mini
  sys Built-in Audio
  rec ~/Recordings/
```

When `editingDir` is true, show an editable text field with cursor.

### Help Bar Updates

When virtual mic is running and not recording:
```
enter stop  r rec  v bars  o output  q quit
```

When recording:
```
r stop rec  v bars  o output  q quit
```

When in save/discard prompt:
```
s save  d discard
```

## Edge Cases

- **User quits while recording:** Stop recording, discard temp file (no surprise saves)
- **Virtual mic stops while recording:** Auto-stop recording, enter save/discard prompt
- **Output dir doesn't exist:** Create it with `os.MkdirAll` on save
- **Temp file write error:** Set `m.err`, stop recording, discard temp file
- **User starts new recording while in prompt:** Not allowed, must save/discard first

## File Changes Summary

| File | Changes |
|------|---------|
| `recorder.go` | **New** — WAV writer, temp file management, save/discard |
| `model.go` | Add rec state, key handlers, status display, dir editing |
| `monitor.go` | No changes |
| `audio.go` | No changes |
| `visualizer.go` | No changes |

## Dependencies

No new dependencies. WAV writing uses only `encoding/binary`, `os`, `io`, `math`, `time`, `sync` from stdlib.
