# omanote

A terminal UI for creating virtual audio loopback devices on Linux. Combines your microphone and system audio into a single virtual mic input — useful for streaming, screen recording, or any application that needs to capture both sources at once.

```
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|
          ~ * ~~ * ~~ *
```

## How it works

omanote creates a PulseAudio null sink (`VirtualMic`) and routes two `pw-loopback` streams into it:

1. **Mic loopback** — your default microphone
2. **System loopback** — your default speaker's monitor (desktop audio)

Any application can then select `VirtualMic` as its input to receive both streams mixed together.

## Requirements

- Linux with **PipeWire** (and PulseAudio compatibility layer)
- `pactl` — PulseAudio control CLI
- `pw-loopback` — PipeWire loopback tool (usually part of `pipewire-pulse` or `pipewire-alsa`)
- Go 1.25+ (to build from source)

On Arch Linux:

```sh
sudo pacman -S pipewire pipewire-pulse
```

On Ubuntu/Debian:

```sh
sudo apt install pipewire pipewire-pulse
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
./omanote
```

### Controls

| Key | Action |
|---|---|
| `Enter` / `Space` | Start or stop the virtual mic |
| `r` | Refresh audio device detection |
| `q` / `Ctrl+C` | Quit |

The TUI shows:

- **Status** — whether the virtual mic is live, with PIDs of the loopback processes and the null sink module ID
- **Audio Devices** — your detected default speaker and microphone
- **Errors** — any issues with device detection or process management

## Architecture

```
main.go    — entry point, launches the Bubble Tea program
model.go   — TUI state machine, rendering, and animations (Bubble Tea + Lip Gloss)
audio.go   — PulseAudio/PipeWire device detection and virtual mic lifecycle
```

The app follows the [Bubble Tea](https://github.com/charmbracelet/bubbletea) architecture: commands produce messages, the `Update` function transitions state, and `View` renders the current state. Audio operations run asynchronously so the UI never blocks.

State is cached in `~/.cache/omanote/` (respects `XDG_CACHE_HOME`):

- `pids` — process IDs of active loopback processes
- `module_id` — PulseAudio module ID for cleanup

## License

MIT
