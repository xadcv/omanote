# omanote

Omanote creates a virtual microphone on Linux that mixes your physical microphone with system audio. It is useful for meeting tools, screen recordings, streaming, OBS, and any app that needs one input containing both voice and desktop sound.

It includes a terminal UI, a background daemon, WAV recording, and a Waybar-ready menu interface.

```text
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|
```

## Features

- Virtual microphone named `Omanote`
- Mixes selected microphone input with selected system output monitor, with `None` available for either side
- Background daemon for Waybar/menu control
- Floating Bubble Tea TUI with real-time FFT visualizer
- WAV recording with save/discard flow
- Persistent config for output directory, devices, visualizer mode, and color scheme
- Optional start-on-login through a user systemd service

## Requirements

- Linux with PipeWire and the PulseAudio compatibility layer
- `pactl` for PulseAudio module control
- `parec` for visualization and recording capture
- Go 1.25.5 or newer to build from source
- Optional: Waybar, Walker, Hyprland/Omarchy for desktop menu integration

Arch Linux:

```sh
sudo pacman -S pipewire pipewire-pulse
```

Ubuntu/Debian:

```sh
sudo apt install pipewire pipewire-pulse pulseaudio-utils
```

## Install

Installer script (recommended):

```sh
curl -fsSL https://raw.githubusercontent.com/xadcv/omanote/main/install.sh | bash
```

The installer:

- Runs `go install github.com/xadcv/omanote@latest`
- Detects `mise` and adds `$HOME/go/bin` to your shell PATH rc file
- Detects Hyprland + Omarchy and **asks before** adding:
  `bindd = SUPER SHIFT, R, Omanote Menu, exec, omanote menu`

You can also run the installer from a clone:

```sh
./install.sh
```

Direct Go install:

```sh
go install github.com/xadcv/omanote@latest
```

From source:

```sh
git clone https://github.com/xadcv/omanote.git
cd omanote
go build -o omanote
```

## Quick Start

Open the TUI:

```sh
omanote
```

Start the virtual mic without opening the TUI:

```sh
omanote start
```

Record and save a WAV file:

```sh
omanote record start
omanote record stop
omanote record save
```

Stop the virtual mic:

```sh
omanote stop
```

Quit the daemon and clean up Omanote audio modules:

```sh
omanote quit
```

## TUI Controls

| Key | Action |
| --- | --- |
| `Enter` / `Space` | Start or stop the virtual mic |
| `r` | Start/stop recording while live; refresh devices while stopped |
| `s` | Save a stopped recording |
| `d` | Discard a stopped recording |
| `o` | Edit recording output directory |
| `a` | Toggle daemon start on login |
| `v` | Cycle visualizer mode |
| `c` | Cycle color scheme |
| `Tab` | Switch device panel while stopped |
| `Up` / `Down` / `j` / `k` | Select device while stopped |
| `q` / `Ctrl+C` | Close the TUI |

The `a` toggle controls future logins. It does not stop the daemon that is already running in the current session.

## CLI

```sh
omanote [tui]
omanote daemon
omanote status [--follow] [--waybar]
omanote start
omanote stop
omanote record start
omanote record stop
omanote record save
omanote record discard
omanote menu
omanote autostart enable
omanote autostart disable
omanote autostart status
omanote quit
```

`omanote status --follow --waybar` prints newline-delimited Waybar JSON. `omanote menu` opens a Walker menu with state-aware actions for opening the TUI, starting/stopping the virtual mic, starting/stopping/saving/discarding recordings, and quitting.

## Waybar Integration

Example Waybar module:

```jsonc
"custom/omanote": {
  "exec": "omanote status --follow --waybar",
  "return-type": "json",
  "format": "{icon}",
  "format-icons": {
    "inactive": "󰍭",
    "idle": "󰍭",
    "live": "󰍬",
    "recording": "󰑋",
    "pending": "󰆓",
    "error": "󰅚"
  },
  "tooltip": true,
  "on-click": "omarchy-launch-or-focus-tui omanote",
  "on-click-right": "omanote menu"
}
```

Example Hyprland keybinding to open the menu directly:

```conf
bindd = SUPER SHIFT, R, Omanote Menu, exec, omanote menu
```

For a floating TUI window on Hyprland:

```conf
windowrule = float on, match:class org.omarchy.omanote
windowrule = size 800 500, match:class org.omarchy.omanote
windowrule = center on, match:class org.omarchy.omanote
```

## How It Works

Omanote creates four PulseAudio modules:

1. `module-null-sink` named `OmanoteMix`
2. `module-remap-source` named `Omanote`, backed by `OmanoteMix.monitor`
3. Optional `module-loopback` from the selected microphone into `OmanoteMix`
4. Optional `module-loopback` from the selected system output monitor into `OmanoteMix`

Select `None` for Microphone or System Audio in the TUI to skip that loopback. Selecting `None` for both creates a silent virtual `Omanote` microphone.

Applications can then select `Omanote` as their microphone input.

The daemon owns the audio module lifecycle and recording state. The TUI, CLI, and Waybar menu all talk to the daemon through a Unix socket at `$XDG_RUNTIME_DIR/omanote/daemon.sock`.

## Configuration And State

Config file:

```text
$XDG_CONFIG_HOME/omanote/config.toml
```

Defaults to:

```text
~/.config/omanote/config.toml
```

State files:

```text
$XDG_CACHE_HOME/omanote/modules
$XDG_CACHE_HOME/omanote/daemon.log
```

Defaults to:

```text
~/.cache/omanote/
```

Recordings default to:

```text
~/Recordings/
```

## Architecture

```text
main.go        entrypoint and CLI dispatch
cli.go         CLI commands, Waybar JSON, Walker menu
daemon.go      background controller and recording owner
ipc.go         Unix socket client/server messages
autostart.go   user systemd service management
model.go       Bubble Tea TUI state and key handling
audio.go       PulseAudio device detection and module lifecycle
monitor.go     parec audio capture
recorder.go    WAV writer
visualizer.go  FFT analysis and render modes
config.go      TOML config persistence
```

## License

MIT
