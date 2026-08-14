# omanote

Omanote creates a virtual microphone on Linux that mixes your physical microphone with system audio. It is useful for meeting tools, screen recordings, streaming, OBS, and any app that needs one input containing both voice and desktop sound.

It includes a terminal UI, a background daemon, WAV recording, and an Omarchy Quattro / Quickshell bar plugin.

```text
  ___  _ __ ___   __ _ _ __   ___ | |_ ___
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|
```

## Features

- Virtual microphone named `Omanote`
- Mixes selected microphone input with app playback routed through the selected system output, with `None` available for either side
- Background daemon for CLI, TUI, and Quattro bar control
- Floating Bubble Tea TUI with real-time FFT visualizer
- WAV recording with save/discard flow
- Persistent config for output directory, devices, visualizer mode, and color scheme
- Optional start-on-login through a user systemd service

## Requirements

- Linux with PipeWire and the PulseAudio compatibility layer
- `pactl` for PulseAudio module control
- `parec` for visualization and recording capture
- Go 1.25.5 or newer to build from source
- Optional: Omarchy Quattro (`omarchy-shell`) for the native bar widget and panel

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
- Detects Omarchy Quattro and **asks before** running:
  `omarchy plugin add https://github.com/xadcv/omanote.git --enable`
- Detects Hyprland + Quattro and **asks before** adding:
  `bindd = SUPER SHIFT, R, Omanote, exec, omanote menu`

Quattro install (binary + shell plugin):

```sh
go install github.com/xadcv/omanote@latest
omarchy plugin add https://github.com/xadcv/omanote.git --enable
```

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
omanote status [--follow] [--json|--waybar]
omanote start [mic] [sink]
omanote stop
omanote devices [--json]
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

`omanote status --json` is the payload the Quattro plugin polls. `omanote menu` summons the Quattro panel when `omarchy-shell` is available, and falls back to Walker otherwise.

## Quattro Plugin

Omanote is a third-party Omarchy shell plugin (`xadcv.omanote`). After the binary is on `PATH`:

```sh
omarchy plugin add https://github.com/xadcv/omanote.git --enable
omarchy bar move xadcv.omanote --section right
```

The bar icon shows idle, live, recording, pending-save, and error states. Left click opens the details panel. Right click starts or stops the virtual mic. The panel can start/stop the mix, record, pick microphone and system output (including `None`), toggle start on login, open the TUI, and quit the daemon.

Summon the panel from a keybind or script:

```sh
omarchy-shell shell summon xadcv.omanote '{}'
# or
omanote menu
```

```conf
bindd = SUPER SHIFT, R, Omanote, exec, omanote menu
```

Remove it with `omarchy plugin remove xadcv.omanote`.

For a floating TUI window on Hyprland:

```conf
windowrule = float on, match:class org.omarchy.omanote
windowrule = size 800 500, match:class org.omarchy.omanote
windowrule = center on, match:class org.omarchy.omanote
```

## How It Works

Omanote creates a software mix and routes app playback through it:

1. `module-null-sink` named `OmanoteMix`
2. `module-remap-source` named `Omanote`, backed by `OmanoteMix.monitor`
3. Optional `module-loopback` from the selected microphone into `OmanoteMix`
4. For system audio, Omanote saves the current default sink, sets `OmanoteMix` as the default sink, and moves existing playback streams into it
5. Optional `module-loopback` from `OmanoteMix.monitor` to the selected system output so audio remains audible

Select `None` for Microphone to skip the mic loopback. Select `None` for System Output to skip playback routing. Selecting `None` for both creates a silent virtual `Omanote` microphone.

When Omanote stops, it restores the saved default sink, moves playback streams back, and unloads its modules.

Applications can then select `Omanote` as their microphone input.

The daemon owns the audio module lifecycle and recording state. The TUI, CLI, and Quattro plugin all talk to the daemon through a Unix socket at `$XDG_RUNTIME_DIR/omanote/daemon.sock`.

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
manifest.json  Quattro plugin contract
BarWidget.qml  Omarchy bar entry point
Panel.qml      details popup for mix, record, and devices
Service.qml    Process wrapper around the omanote CLI
Model.js       status parsing and bar icon mapping
main.go        entrypoint and CLI dispatch
cli.go         CLI commands, JSON status, Quattro/Walker menu
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
