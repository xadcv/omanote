package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WAV format constants for the recorder output.
const (
	wavSampleRate    = 48000
	wavChannels      = 1
	wavBitsPerSample = 32
	wavHeaderSize    = 44
)

// Recorder streams PCM samples to a temporary WAV file during recording.
// On save it finalizes the WAV header and moves the file to the destination.
// On discard it deletes the temp file.
type Recorder struct {
	mu        sync.Mutex
	file      *os.File
	dataSize  uint32
	startTime time.Time
	recording bool
}

// Start creates a temp file and writes a 44-byte WAV header with placeholder
// sizes. The recorder begins accepting samples via WriteSamples.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder already running")
	}

	f, err := os.CreateTemp(os.TempDir(), "omanote-rec-*.wav")
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

// WriteSamples converts float64 samples to float32 little-endian and appends
// them to the WAV data chunk. Silently returns if not recording.
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

// Stop marks recording as finished but keeps the file open for Save or Discard.
func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recording = false
}

// Save finalizes the WAV header with the correct data size, closes the file,
// and moves it to destDir with an omanote-YYYY-MM-DD-HH-MM-SS.wav name.
// If rename fails (e.g. cross-filesystem), it falls back to copy+delete.
func (r *Recorder) Save(destDir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file == nil {
		return "", fmt.Errorf("no recording to save")
	}

	// Seek back to the beginning and rewrite the header with correct sizes.
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek to header: %w", err)
	}
	if err := writeWAVHeader(r.file, r.dataSize); err != nil {
		return "", fmt.Errorf("finalize WAV header: %w", err)
	}

	tmpPath := r.file.Name()
	if err := r.file.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	r.file = nil

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	ts := r.startTime.Format("2006-01-02-15-04-05")
	destPath := filepath.Join(destDir, fmt.Sprintf("omanote-%s.wav", ts))

	if err := os.Rename(tmpPath, destPath); err != nil {
		// Cross-filesystem fallback: copy then delete.
		if err := copyFile(tmpPath, destPath); err != nil {
			return "", fmt.Errorf("copy file: %w", err)
		}
		os.Remove(tmpPath)
	}

	return destPath, nil
}

// Discard closes and deletes the temp file.
func (r *Recorder) Discard() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		name := r.file.Name()
		r.file.Close()
		os.Remove(name)
		r.file = nil
	}
	r.recording = false
}

// IsRecording reports whether the recorder is actively capturing samples.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Duration returns time elapsed since recording started, or 0 if not recording.
func (r *Recorder) Duration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording {
		return time.Since(r.startTime)
	}
	return 0
}

// writeWAVHeader writes a 44-byte RIFF WAV header for 32-bit float mono PCM.
// dataSize is the number of bytes in the data chunk (0 for a placeholder).
func writeWAVHeader(f *os.File, dataSize uint32) error {
	h := make([]byte, wavHeaderSize)

	// RIFF chunk descriptor
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], 36+dataSize) // file size - 8
	copy(h[8:12], "WAVE")

	// fmt sub-chunk
	copy(h[12:16], "fmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16) // sub-chunk size
	binary.LittleEndian.PutUint16(h[20:22], 3)  // audio format: IEEE float
	binary.LittleEndian.PutUint16(h[22:24], wavChannels)
	binary.LittleEndian.PutUint32(h[24:28], wavSampleRate)
	byteRate := uint32(wavSampleRate * wavChannels * wavBitsPerSample / 8)
	binary.LittleEndian.PutUint32(h[28:32], byteRate)
	blockAlign := uint16(wavChannels * wavBitsPerSample / 8)
	binary.LittleEndian.PutUint16(h[32:34], blockAlign)
	binary.LittleEndian.PutUint16(h[34:36], wavBitsPerSample)

	// data sub-chunk
	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], dataSize)

	_, err := f.Write(h)
	return err
}

// copyFile copies src to dst by reading and writing the full contents.
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

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
