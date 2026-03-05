# Omanote TUI Visualizer Design

## Overview

Transform the Omanote TUI into a full-screen, animated audio visualizer with real FFT-based spectrum analysis. Inspired by [cliamp](https://github.com/bjarneo/cliamp).

## Audio Data Pipeline

- When running: spawn `parec --device=OmanoteMix.monitor --format=float32le --channels=1 --rate=48000` subprocess
- Read raw PCM float32 samples from stdout in a goroutine
- Feed chunks to FFT analyzer (2048 sample window, Hann windowed)
- When stopped: visualizer decays to silence (smooth `prev * 0.8` falloff)
- New dependency: `github.com/madelynnblue/go-dsp` for FFT

## Visualizer Modes

Cycle with `v` key:

1. **Bars** — smooth fractional Unicode blocks, 10 frequency bands
2. **Wave** — braille oscilloscope waveform from raw samples
3. **Flame** — braille rising fire tendrils per band
4. **Retro** — synthwave perspective grid with audio-reactive wave + striped sun
5. **Pulse** — braille pulsating filled circle with shockwave ring

Color gradient: pink (low) → purple (mid) → cyan (high) — matching existing synthwave palette.

## Full-Screen Layout

Alternate screen mode. Centered 80-char frame.

```
  ___  _ __ ___   __ _ _ __   ___ | |_ ___        <- rainbow gradient logo
 / _ \| '_ ' _ \ / _' | '_ \ / _ \| __/ _ \
| (_) | | | | | | (_| | | | | (_) | ||  __/
 \___/|_| |_| |_|\__,_|_| |_|\___/ \__\___|

  ▇▇▇  ▅▅▅  ███  ▃▃▃  ▆▆▆  ██  ▄▄▄  ▇▇  ▅▅  ▂▂  <- FFT visualizer (5 rows)
  ███  ███  ███  ▇▇▇  ███  ██  ███  ██  ▇▇  ▅▅
  ███  ███  ███  ███  ███  ██  ███  ██  ██  ██

  ╭─────────────────────────────────────────────╮
  │  ** Omanote is LIVE **                      │   <- status box
  │  mic: RDE NT USB Mini  sys: WH-1000XM6     │
  ╰─────────────────────────────────────────────╯

  Microphone                   System Audio         <- device selection
  > RDE NT USB Mini            > WH-1000XM6
    Ryzen HD Audio               Ryzen HD Audio

  enter stop  v Bars  tab switch  ↑↓ select  q quit <- help bar
```

## File Structure

- `audio.go` — unchanged
- `visualizer.go` — NEW: FFT analysis + 5 render modes (ported from cliamp)
- `monitor.go` — NEW: parec subprocess, sample reading goroutine
- `model.go` — rewritten: full-screen layout, visualizer integration, v key

## Key Bindings

- `v` — cycle visualizer mode
- `enter`/`space` — start/stop
- `tab` — switch device panel focus
- `↑↓`/`jk` — select device
- `r` — refresh devices
- `q`/`ctrl+c` — quit
