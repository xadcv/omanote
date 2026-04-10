# Config File, Device Hot-Plug, Color Scheme Cycling

Status: approved

## Config File

TOML config at `$XDG_CONFIG_HOME/omanote/config.toml` (default `~/.config/omanote/config.toml`).

```toml
output_dir = "~/Recordings"
vis_mode = "Bars"
color_scheme = "Synthwave"
preferred_source = "alsa_input.usb-RODE..."
preferred_sink = "alsa_output.pci-..."
```

New file `config.go`:
- `Config` struct with 5 fields matching TOML keys
- `configDir()` uses `$XDG_CONFIG_HOME` or `~/.config/omanote`
- `loadConfig()` reads + unmarshals, returns defaults if file missing
- `saveConfig()` marshals + writes atomically
- Dependency: `github.com/BurntSushi/toml`

Save triggers: `c` (scheme cycle), `v` (vis mode cycle), `o` + enter (output dir), virtual mic start (device prefs).

Load: once at startup in `initialModel()`.

## Device Hot-Plug

Poll-based via existing 2-second `tickRefreshMsg` tick. Add `cmdListDevices` to the batch in the handler:

```go
case tickRefreshMsg:
    if m.state == stateIdle {
        return m, tea.Batch(cmdListDevices, cmdCheckStatus, cmdScheduleRefresh())
    }
    return m, tea.Batch(cmdCheckStatus, cmdScheduleRefresh())
```

Preserve device selection by name (not index) across refreshes — store selected device name, find its new index after refresh.

## Color Schemes

New file `colorscheme.go`. Theme viz colors (specLow/Mid/High) + logo gradient. UI chrome stays fixed.

```go
type ColorScheme struct {
    Name     string
    Low      lipgloss.Color
    Mid      lipgloss.Color
    High     lipgloss.Color
    Gradient []string // 5 logo colors
}
```

5 schemes:

| Scheme | Low | Mid | High |
|--------|-----|-----|------|
| Synthwave | #04B575 (green) | #C774E8 (purple) | #FF6AD5 (hot pink) |
| Monochrome | #808080 | #C0C0C0 | #FFFFFF |
| Matrix | #005500 | #00AA00 | #00FF00 |
| Ocean | #006688 | #4488CC | #44DDFF |
| Sunset | #DDAA00 | #FF6600 | #FF2200 |

Visualizer changes:
- Add `Scheme *ColorScheme` field
- `specStyle()` becomes method on `Visualizer`, reads from `v.Scheme`
- All render methods already call `specStyle()` — those just work
- `renderRetro` directly references `specHighStyle`/`specMidStyle`/`specLowStyle` — change to method calls

Model changes:
- `c` key cycles `colorSchemes` slice, updates `vis.Scheme`
- Help bar shows scheme name alongside viz mode
- `rainbowText()` takes gradient colors from current scheme

## Files

| File | Change |
|------|--------|
| config.go (new) | Config struct, load/save |
| colorscheme.go (new) | ColorScheme, 5 presets |
| visualizer.go | Dynamic colors via scheme field |
| model.go | Config integration, `c` key, hot-plug tick, device selection by name |
| go.mod/go.sum | BurntSushi/toml dep |
