package main

import (
	"math"
	"math/cmplx"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/madelynnblue/go-dsp/fft"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	numBands       = 10
	fftSize        = 2048
	defaultVisRows = 8
	panelWidth     = 74
)

// bandEdges defines the frequency boundaries for the 10 spectrum bands.
// Each band spans bandEdges[i] .. bandEdges[i+1].
var bandEdges = [11]float64{
	20, 100, 200, 400, 800, 1600, 3200, 6400, 12800, 16000, 20000,
}

// ---------------------------------------------------------------------------
// Visualization mode enum
// ---------------------------------------------------------------------------

// VisMode selects the active visualizer rendering style.
type VisMode int

const (
	VisBars    VisMode = iota // smooth fractional Unicode blocks
	VisBricks                 // solid half-height blocks with gaps
	VisColumns                // thin interpolated columns
	VisWave                   // braille oscilloscope
	VisScatter                // braille particle field
	VisFlame                  // braille fire tendrils
	VisRetro                  // synthwave sun + wave + grid
	VisPulse                  // braille pulsating ellipse
	VisNone                   // blank / disabled
	visCount                  // sentinel for cycling
)

// ---------------------------------------------------------------------------
// Visual primitives
// ---------------------------------------------------------------------------

// barBlocks provides sub-row fractional block characters (9 levels: 0/8 .. 8/8).
var barBlocks = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// brailleBit maps a (row 0-3, col 0-1) position to the corresponding
// bit in the Unicode Braille base character U+2800.
var brailleBit = [4][2]rune{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// ---------------------------------------------------------------------------
// Spectrum color palette (Omanote synthwave)
// ---------------------------------------------------------------------------

var (
	specLow  = lipgloss.Color("#04B575") // green  — low frequencies / bottom
	specMid  = lipgloss.Color("#C774E8") // purple — mid frequencies / center
	specHigh = lipgloss.Color("#FF6AD5") // hot pink — high frequencies / top

	specLowStyle  = lipgloss.NewStyle().Foreground(specLow)
	specMidStyle  = lipgloss.NewStyle().Foreground(specMid)
	specHighStyle = lipgloss.NewStyle().Foreground(specHigh)
)

// ---------------------------------------------------------------------------
// Visualizer struct
// ---------------------------------------------------------------------------

// Visualizer holds FFT analysis state, temporal smoothing, and render config.
type Visualizer struct {
	prev    [numBands]float64 // smoothed band magnitudes from previous frame
	sr      float64           // audio sample rate (Hz)
	buf     []float64         // reusable FFT input buffer
	Mode    VisMode           // current render mode
	Rows    int               // number of terminal rows for the visualizer
	waveBuf []float64         // raw audio samples for oscilloscope mode
	frame   uint64            // monotonic frame counter (for animation)
}

// NewVisualizer creates a Visualizer configured for the given sample rate.
func NewVisualizer(sampleRate float64) *Visualizer {
	return &Visualizer{
		sr:   sampleRate,
		buf:  make([]float64, fftSize),
		Rows: defaultVisRows,
	}
}

// CycleMode advances to the next visualization mode, wrapping around.
func (v *Visualizer) CycleMode() {
	v.Mode = (v.Mode + 1) % visCount
}

// ModeName returns a human-readable label for the current mode.
func (v *Visualizer) ModeName() string {
	switch v.Mode {
	case VisBars:
		return "Bars"
	case VisBricks:
		return "Bricks"
	case VisColumns:
		return "Columns"
	case VisWave:
		return "Wave"
	case VisScatter:
		return "Scatter"
	case VisFlame:
		return "Flame"
	case VisRetro:
		return "Retro"
	case VisPulse:
		return "Pulse"
	case VisNone:
		return "None"
	default:
		return "?"
	}
}

// ---------------------------------------------------------------------------
// FFT analysis
// ---------------------------------------------------------------------------

// Analyze performs windowed FFT on the incoming audio samples and returns
// 10 frequency-band magnitudes normalized to [0, 1].
//
// When samples is empty the previous magnitudes decay toward zero.
func (v *Visualizer) Analyze(samples []float64) [numBands]float64 {
	// Store raw samples for oscilloscope (wave) mode.
	if len(samples) > 0 {
		v.waveBuf = append(v.waveBuf[:0], samples...)
	}

	v.frame++

	// Silence / no input: decay previous bands.
	if len(samples) == 0 {
		for i := range v.prev {
			v.prev[i] *= 0.8
		}
		return v.prev
	}

	// Fill the FFT buffer (zero-pad or truncate as needed).
	buf := v.buf
	for i := range buf {
		buf[i] = 0
	}
	n := len(samples)
	if n > fftSize {
		n = fftSize
	}

	// Apply Hann window while copying.
	for i := 0; i < n; i++ {
		w := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(n-1)))
		buf[i] = samples[i] * w
	}

	// Forward FFT (real-valued input).
	spectrum := fft.FFTReal(buf)

	// Bin magnitudes into frequency bands.
	var bands [numBands]float64
	var counts [numBands]int

	nyquist := v.sr / 2.0
	usable := len(spectrum) / 2 // only first half is unique for real input

	for k := 1; k < usable; k++ {
		freq := float64(k) * v.sr / float64(len(spectrum))
		if freq > nyquist {
			break
		}
		mag := cmplx.Abs(spectrum[k])

		// Find which band this bin belongs to.
		for b := 0; b < numBands; b++ {
			if freq >= bandEdges[b] && freq < bandEdges[b+1] {
				bands[b] += mag
				counts[b]++
				break
			}
		}
	}

	// Average, convert to dB, normalize to [0, 1].
	for b := range bands {
		if counts[b] > 0 {
			bands[b] /= float64(counts[b])
		}
		// Convert to dB (reference amplitude = 1.0).
		if bands[b] > 1e-12 {
			bands[b] = 20.0 * math.Log10(bands[b])
		} else {
			bands[b] = -120
		}
		// Map dB range [-80, 0] to [0, 1].
		bands[b] = (bands[b] + 80.0) / 80.0
		if bands[b] < 0 {
			bands[b] = 0
		}
		if bands[b] > 1 {
			bands[b] = 1
		}
	}

	// Temporal smoothing: fast attack, slow decay.
	for i := range bands {
		if bands[i] > v.prev[i] {
			// Attack: 60% new, 40% old.
			bands[i] = 0.6*bands[i] + 0.4*v.prev[i]
		} else {
			// Decay: 25% new, 75% old.
			bands[i] = 0.25*bands[i] + 0.75*v.prev[i]
		}
		v.prev[i] = bands[i]
	}

	return bands
}

// ---------------------------------------------------------------------------
// Render dispatcher
// ---------------------------------------------------------------------------

// Render returns the string representation of the current visualizer mode
// for the given frequency bands.
func (v *Visualizer) Render(bands [numBands]float64) string {
	switch v.Mode {
	case VisBars:
		return v.renderBars(bands)
	case VisBricks:
		return v.renderBricks(bands)
	case VisColumns:
		return v.renderColumns(bands)
	case VisWave:
		return v.renderWave()
	case VisScatter:
		return v.renderScatter(bands)
	case VisFlame:
		return v.renderFlame(bands)
	case VisRetro:
		return v.renderRetro(bands)
	case VisPulse:
		return v.renderPulse(bands)
	case VisNone:
		return ""
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Helper: band width distribution
// ---------------------------------------------------------------------------

// visBandWidth returns the character width for band b, distributing
// panelWidth across numBands bands with 1-char gaps between them.
func visBandWidth(b int) int {
	totalGaps := numBands - 1
	usable := panelWidth - totalGaps
	base := usable / numBands
	extra := usable % numBands
	if b < extra {
		return base + 1
	}
	return base
}

// ---------------------------------------------------------------------------
// Helper: spectrum color selection
// ---------------------------------------------------------------------------

// specStyle returns a lipgloss style colored according to the vertical
// position within the visualizer (0.0 = bottom/low, 1.0 = top/high).
func specStyle(rowBottom float64) lipgloss.Style {
	if rowBottom > 0.66 {
		return specHighStyle
	}
	if rowBottom > 0.33 {
		return specMidStyle
	}
	return specLowStyle
}

// ---------------------------------------------------------------------------
// Helper: pseudo-random hash for particle effects
// ---------------------------------------------------------------------------

// scatterHash returns a deterministic pseudo-random value in [0, 1) for
// the given band, row, column, and frame. Used by scatter and flame modes.
func scatterHash(band, row, col int, frame uint64) float64 {
	h := uint64(band)*374761393 +
		uint64(row)*668265263 +
		uint64(col)*2654435761 +
		frame*2246822519
	h ^= h >> 13
	h *= 3266489917
	h ^= h >> 16
	return float64(h&0x7FFFFFFF) / float64(0x7FFFFFFF)
}

// ---------------------------------------------------------------------------
// Mode 1: Bars — smooth fractional Unicode blocks
// ---------------------------------------------------------------------------

// renderBars draws a spectrum analyser using sub-row-resolution Unicode
// block characters (▁ through █) for smooth amplitude display.
func (v *Visualizer) renderBars(bands [numBands]float64) string {
	height := v.Rows
	lines := make([]string, height)

	for row := range height {
		var sb strings.Builder
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)

		for i, level := range bands {
			bw := visBandWidth(i)
			style := specStyle(rowBottom)

			if level >= rowTop {
				// Full block for this row.
				sb.WriteString(style.Render(strings.Repeat("█", bw)))
			} else if level > rowBottom {
				// Fractional block: determine which of the 8 sub-levels.
				frac := (level - rowBottom) / (rowTop - rowBottom)
				idx := int(frac * 8)
				if idx > 8 {
					idx = 8
				}
				sb.WriteString(style.Render(strings.Repeat(barBlocks[idx], bw)))
			} else {
				sb.WriteString(strings.Repeat(" ", bw))
			}

			if i < numBands-1 {
				sb.WriteString(" ")
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 2: Bricks — solid half-height blocks with visible gaps
// ---------------------------------------------------------------------------

// renderBricks draws solid block columns with visible gaps between rows and
// bands. Uses half-height blocks (▄) so each brick is half a terminal row.
func (v *Visualizer) renderBricks(bands [numBands]float64) string {
	height := v.Rows
	lines := make([]string, height)

	for row := range height {
		var sb strings.Builder
		rowThreshold := float64(height-1-row) / float64(height)

		for i, level := range bands {
			bw := visBandWidth(i)
			style := specStyle(rowThreshold)
			if level > rowThreshold {
				sb.WriteString(style.Render(strings.Repeat("▄", bw)))
			} else {
				sb.WriteString(strings.Repeat(" ", bw))
			}
			if i < numBands-1 {
				sb.WriteString(" ")
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 3: Columns — thin single-char columns with inter-band interpolation
// ---------------------------------------------------------------------------

// renderColumns draws thin columns that interpolate between neighboring
// frequency bands to create a dense, organic appearance.
func (v *Visualizer) renderColumns(bands [numBands]float64) string {
	height := v.Rows
	lines := make([]string, height)

	for row := range height {
		var sb strings.Builder
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)

		for i := range numBands {
			bw := visBandWidth(i)

			for c := range bw {
				// Interpolate between this band and the next.
				t := float64(c) / float64(bw)
				var level float64
				if i < numBands-1 {
					level = bands[i]*(1.0-t) + bands[i+1]*t
				} else {
					level = bands[i]
				}

				style := specStyle(rowBottom)
				if level >= rowTop {
					sb.WriteString(style.Render("█"))
				} else if level > rowBottom {
					frac := (level - rowBottom) / (rowTop - rowBottom)
					idx := int(frac * 8)
					if idx > 8 {
						idx = 8
					}
					sb.WriteString(style.Render(barBlocks[idx]))
				} else {
					sb.WriteString(" ")
				}
			}
			if i < numBands-1 {
				sb.WriteString(" ")
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 4: Wave — braille oscilloscope from raw samples
// ---------------------------------------------------------------------------

// renderWave draws a Braille-character oscilloscope waveform from raw audio
// samples. Each Braille character covers a 2x4 dot grid, giving smooth
// sub-cell resolution.
func (v *Visualizer) renderWave() string {
	height := v.Rows
	const charCols = panelWidth
	dotRows := height * 4
	dotCols := charCols * 2

	samples := v.waveBuf
	n := len(samples)

	// Downsample audio to one y-position per horizontal dot column.
	ypos := make([]int, dotCols)
	for x := range dotCols {
		var sample float64
		if n > 0 {
			idx := x * n / dotCols
			if idx >= n {
				idx = n - 1
			}
			sample = samples[idx]
		}
		// Map sample [-1, 1] to dot row [0, dotRows-1]; center is dotRows/2.
		y := int((1.0 - sample) * float64(dotRows-1) / 2.0)
		if y < 0 {
			y = 0
		}
		if y >= dotRows {
			y = dotRows - 1
		}
		ypos[x] = y
	}

	lines := make([]string, height)
	for row := range height {
		var sb strings.Builder
		dotRowStart := row * 4

		for ch := range charCols {
			var braille rune = '\u2800'
			dotColStart := ch * 2

			for dc := range 2 {
				x := dotColStart + dc
				y := ypos[x]

				// Connect to previous point so the waveform is continuous.
				prevY := y
				if x > 0 {
					prevY = ypos[x-1]
				}
				yMin := min(y, prevY)
				yMax := max(y, prevY)

				for dr := range 4 {
					dotY := dotRowStart + dr
					if dotY >= yMin && dotY <= yMax {
						braille |= brailleBit[dr][dc]
					}
				}
			}

			style := specStyle(float64(height-1-row) / float64(height))
			sb.WriteString(style.Render(string(braille)))
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 5: Scatter — braille particle field
// ---------------------------------------------------------------------------

// renderScatter draws a braille particle field where particle density is
// proportional to the square of band energy, with gravity bias pulling
// particles toward the bottom.
func (v *Visualizer) renderScatter(bands [numBands]float64) string {
	height := v.Rows
	dotRows := height * 4
	lines := make([]string, height)

	for row := range height {
		var sb strings.Builder
		dotRowStart := row * 4

		for i := range numBands {
			bw := visBandWidth(i)
			energy := bands[i] * bands[i] // density proportional to energy^2

			for c := 0; c < bw; c += 2 {
				var braille rune = '\u2800'
				colsHere := 2
				if c+1 >= bw {
					colsHere = 1
				}

				for dr := range 4 {
					dotY := dotRowStart + dr
					// Gravity bias: higher probability for dots nearer the bottom.
					gravityBias := float64(dotY) / float64(dotRows)
					for dc := 0; dc < colsHere; dc++ {
						h := scatterHash(i, dotY, c+dc, v.frame)
						threshold := energy * (0.3 + 0.7*gravityBias)
						if h < threshold {
							braille |= brailleBit[dr][dc]
						}
					}
				}

				style := specStyle(float64(height-1-row) / float64(height))
				sb.WriteString(style.Render(string(braille)))
			}

			if i < numBands-1 {
				sb.WriteString(" ")
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 6: Flame — braille fire tendrils
// ---------------------------------------------------------------------------

// renderFlame draws rising flame tendrils per band using Braille characters.
// Includes lateral wobble driven by a sine-based displacement and tip
// narrowing for a realistic fire look.
func (v *Visualizer) renderFlame(bands [numBands]float64) string {
	height := v.Rows
	dotRows := height * 4
	lines := make([]string, height)

	for row := range height {
		var sb strings.Builder
		dotRowStart := row * 4

		for i := range numBands {
			bw := visBandWidth(i)
			energy := bands[i]
			flameHeight := int(energy * float64(dotRows))

			for c := 0; c < bw; c += 2 {
				var braille rune = '\u2800'
				colsHere := 2
				if c+1 >= bw {
					colsHere = 1
				}

				for dr := range 4 {
					dotY := dotRowStart + dr
					distFromBottom := dotRows - 1 - dotY
					if distFromBottom >= flameHeight {
						continue
					}

					// Tip narrowing: flames narrow toward the top.
					progress := float64(distFromBottom) / float64(max(flameHeight, 1))
					narrowing := 1.0 - progress*progress

					// Lateral wobble driven by sine.
					wobble := math.Sin(float64(v.frame)*0.3+float64(i)*1.7+float64(dotY)*0.5) * 0.3

					centerCol := float64(bw) / 2.0
					colDist := math.Abs(float64(c)+0.5-centerCol) / centerCol

					for dc := 0; dc < colsHere; dc++ {
						colDistDC := math.Abs(float64(c+dc)+0.5-centerCol+wobble) / centerCol
						threshold := narrowing * (1.0 - colDist*0.5)
						h := scatterHash(i, dotY, c+dc, v.frame)
						if h < threshold && colDistDC < narrowing {
							braille |= brailleBit[dr][dc]
						}
					}
				}

				// Flames are hot pink at the top, purple in the middle, green at the base.
				style := specStyle(float64(height-1-row) / float64(height))
				sb.WriteString(style.Render(string(braille)))
			}

			if i < numBands-1 {
				sb.WriteString(" ")
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 7: Retro — synthwave sun, audio-reactive wave, perspective grid
// ---------------------------------------------------------------------------

// renderRetro draws a retro 80s synthwave scene: a striped setting sun above
// the horizon, a smooth audio-reactive wave, and a perspective grid floor
// with scrolling horizontal lines.
func (v *Visualizer) renderRetro(bands [numBands]float64) string {
	height := v.Rows
	if height < 3 {
		height = 3
	}

	lines := make([]string, height)
	horizonRow := height / 3 // sun occupies top third

	// Compute average energy for wave amplitude.
	var avgEnergy float64
	for _, b := range bands {
		avgEnergy += b
	}
	avgEnergy /= float64(numBands)

	for row := range height {
		var sb strings.Builder

		if row <= horizonRow {
			// --- Sun: striped disc ---
			centerY := float64(horizonRow) / 2.0
			radius := centerY + 0.5
			dy := math.Abs(float64(row) - centerY)

			for col := range panelWidth {
				centerX := float64(panelWidth) / 2.0
				dx := (float64(col) - centerX) / 2.0 // aspect ratio correction
				dist := math.Sqrt(dx*dx + dy*dy)

				if dist <= radius {
					// Alternating stripes across the sun.
					isStripe := (row+int(v.frame))%2 == 0
					if isStripe {
						sb.WriteString(specHighStyle.Render("█"))
					} else {
						sb.WriteString(specMidStyle.Render("▒"))
					}
				} else {
					sb.WriteString(" ")
				}
			}
		} else if row == horizonRow+1 {
			// --- Audio-reactive wave at the horizon ---
			for col := range panelWidth {
				t := float64(col) / float64(panelWidth)
				// Interpolate band energy across the width.
				bandF := t * float64(numBands-1)
				bandIdx := int(bandF)
				if bandIdx >= numBands-1 {
					bandIdx = numBands - 2
				}
				frac := bandF - float64(bandIdx)
				energy := bands[bandIdx]*(1.0-frac) + bands[bandIdx+1]*frac

				// Wave displacement.
				waveY := math.Sin(float64(col)*0.2+float64(v.frame)*0.15) * energy * 2.0
				if math.Abs(waveY) > 0.5 {
					sb.WriteString(specHighStyle.Render("~"))
				} else {
					sb.WriteString(specMidStyle.Render("─"))
				}
			}
		} else {
			// --- Perspective grid floor ---
			depth := float64(row-horizonRow-1) / float64(height-horizonRow-1)
			scrollOffset := float64(v.frame) * 0.5

			// Horizontal grid lines: more frequent as depth increases.
			hLineSpacing := max(1.0, 4.0*(1.0-depth))
			isHLine := math.Mod(float64(row)+scrollOffset, hLineSpacing) < 0.5

			for col := range panelWidth {
				centerX := float64(panelWidth) / 2.0
				dx := float64(col) - centerX

				// Vertical grid lines: converge toward center with perspective.
				perspectiveScale := 0.2 + 0.8*depth
				gridX := dx * perspectiveScale
				isVLine := math.Abs(math.Mod(gridX+0.5, 8.0)-4.0) < 0.5

				if isHLine && isVLine {
					sb.WriteString(specMidStyle.Render("+"))
				} else if isHLine {
					sb.WriteString(specLowStyle.Render("─"))
				} else if isVLine {
					sb.WriteString(specLowStyle.Render("│"))
				} else {
					sb.WriteString(" ")
				}
			}
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mode 8: Pulse — braille filled ellipse with shockwave ring
// ---------------------------------------------------------------------------

// renderPulse draws a pulsating ellipse using Braille dots. The radius is
// modulated by frequency bands, with a shockwave ring that expands on
// transients and a breathing animation.
func (v *Visualizer) renderPulse(bands [numBands]float64) string {
	height := v.Rows
	charCols := panelWidth
	dotRows := height * 4
	dotCols := charCols * 2

	centerX := float64(dotCols) / 2.0
	centerY := float64(dotRows) / 2.0

	// Compute average energy and max for shockwave detection.
	var avgEnergy, maxEnergy float64
	for _, b := range bands {
		avgEnergy += b
		if b > maxEnergy {
			maxEnergy = b
		}
	}
	avgEnergy /= float64(numBands)

	// Base radius with breathing animation.
	breath := 0.5 + 0.5*math.Sin(float64(v.frame)*0.1)
	baseRadius := (0.2 + 0.6*avgEnergy) * float64(min(dotRows, dotCols)) / 2.0
	baseRadius *= 0.8 + 0.2*breath

	// Shockwave ring: expands when energy exceeds threshold.
	shockRadius := 0.0
	shockAlpha := 0.0
	if maxEnergy > 0.7 {
		// Ring at a larger radius, fading out.
		shockPhase := math.Mod(float64(v.frame)*0.3, 1.0)
		shockRadius = baseRadius * (1.2 + 0.8*shockPhase)
		shockAlpha = 1.0 - shockPhase
	}

	lines := make([]string, height)
	for row := range height {
		var sb strings.Builder
		dotRowStart := row * 4

		for ch := range charCols {
			var braille rune = '\u2800'
			dotColStart := ch * 2

			for dc := range 2 {
				for dr := range 4 {
					dotY := float64(dotRowStart + dr)
					dotX := float64(dotColStart + dc)

					dy := dotY - centerY
					dx := (dotX - centerX) * 0.5 // aspect ratio correction

					dist := math.Sqrt(dx*dx + dy*dy)

					// Per-band radial deformation.
					angle := math.Atan2(dy, dx)
					bandF := (angle + math.Pi) / (2.0 * math.Pi) * float64(numBands)
					bandIdx := int(bandF) % numBands
					nextIdx := (bandIdx + 1) % numBands
					frac := bandF - math.Floor(bandF)
					bandEnergy := bands[bandIdx]*(1.0-frac) + bands[nextIdx]*frac
					modRadius := baseRadius * (0.6 + 0.4*bandEnergy)

					// Filled ellipse.
					if dist <= modRadius {
						braille |= brailleBit[dr][dc]
					}

					// Shockwave ring.
					if shockAlpha > 0.1 {
						ringDist := math.Abs(dist - shockRadius)
						if ringDist < 1.5 {
							h := scatterHash(int(dotX), int(dotY), 0, v.frame)
							if h < shockAlpha*0.8 {
								braille |= brailleBit[dr][dc]
							}
						}
					}
				}
			}

			// Radial color gradient.
			dy := float64(dotRowStart+2) - centerY
			dx := (float64(dotColStart+1) - centerX) * 0.5
			dist := math.Sqrt(dx*dx + dy*dy)
			normDist := dist / baseRadius
			if normDist > 1 {
				normDist = 1
			}

			style := specStyle(normDist)
			sb.WriteString(style.Render(string(braille)))
		}
		lines[row] = sb.String()
	}

	return strings.Join(lines, "\n")
}
