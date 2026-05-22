# omanote

A terminal UI for creating virtual audio loopback devices on Linux. Combines your microphone and system audio into a single virtual mic input — useful for streaming, screen recording, or any application that needs to capture both sources at once.

Features a full-screen animated audio visualizer with real FFT-based spectrum analysis.

```
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|
```

## How it works

omanote creates a PulseAudio null sink (`OmanoteMix`) and a remap source (`Omanote`) that appears as a selectable microphone input. Two `module-loopback` streams route audio into the mix:

1. **Mic loopback** — your selected microphone
2. **System loopback** — your selected output device's monitor (desktop audio)

Any application (Notion AI Meetings, Zoom, OBS, etc.) can then select `Omanote` as its input to receive both streams mixed together.

While running, a `parec` subprocess captures audio from the mix for real-time FFT visualization in the TUI.

## Requirements

- Linux with **PipeWire** (and PulseAudio compatibility layer)
- `pactl` — PulseAudio control CLI
- `parec` — PulseAudio recording tool (for visualizer audio capture)
- Go 1.25+ (to build from source)

On Arch Linux:

```sh
sudo pacman -S pipewire pipewire-pulse
```

On Ubuntu/Debian:

```sh
sudo apt install pipewire pipewire-pulse pulseaudio-utils
```

## Install

```sh
go install github.com/xadcv/omanote@latest
```

Or build from source:

```sh
git clone https://github.com/xadcv/omanote.git
cd omanote
go build -o omanote
```

## Usage

```sh
omanote
```

`omanote` opens the TUI. The same binary also exposes a background controller for menu-bar integrations:

```sh
omanote daemon
omanote start
omanote stop
omanote record start
omanote record stop
omanote record save
omanote record discard
omanote status --follow --waybar
omanote menu
omanote autostart enable
```

On Omarchy, the Waybar item uses `omanote status --follow --waybar`; left-click opens/focuses the floating TUI and right-click opens the Omanote action menu.
The TUI's start-on-login toggle controls whether the daemon starts on future logins; it does not stop the current session.

### Controls

| Key | Action |
|---|---|
| `Enter` / `Space` | Start or stop the virtual mic |
| `a` | Toggle Omanote daemon start on login |
| `v` | Cycle visualizer mode |
| `Tab` | Switch between Microphone and System Audio panels |
| `Up` / `Down` / `j` / `k` | Select device |
| `r` | Start/stop recording while live; refresh devices while stopped |
| `s` / `d` | Save or discard a stopped recording |
| `q` / `Ctrl+C` | Close the TUI |

### Visualizer Modes

Cycle through with `v`:

| Mode | Description |
|---|---|
| **Bars** | Smooth fractional Unicode block spectrum |
| **Bricks** | Solid half-height blocks with gaps |
| **Columns** | Thin interpolated columns between bands |
| **Wave** | Braille oscilloscope waveform |
| **Scatter** | Braille particle field with gravity |
| **Flame** | Rising fire tendrils per frequency band |
| **Retro** | Synthwave sun + audio-reactive wave + perspective grid |
| **Pulse** | Pulsating braille ellipse with shockwave ring |

Audio is analyzed via windowed FFT (2048 samples, Hann window) into 10 frequency bands with temporal smoothing for fluid animation at 20 FPS.

## Architecture

```
main.go        — entry point, launches the Bubble Tea program
model.go       — TUI state machine, full-screen layout, key handling
audio.go       — PulseAudio device detection and virtual mic lifecycle
visualizer.go  — FFT analysis engine and 8 render modes
monitor.go     — parec subprocess for real-time audio capture
```

The app follows the [Bubble Tea](https://github.com/charmbracelet/bubbletea) architecture: commands produce messages, the `Update` function transitions state, and `View` renders the current state. Audio operations run asynchronously so the UI never blocks.

State is cached in `~/.cache/omanote/` (respects `XDG_CACHE_HOME`):

- `modules` — PulseAudio module IDs for the 4 loaded modules (null-sink, remap-source, 2x loopback)

## License

MIT
