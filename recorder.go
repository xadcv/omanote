package main

import (
	"encoding/binary"
	"errors"
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
	tempPath  string
	dataSize  uint32
	startTime time.Time
	recording bool
	writeErr  error
}

// Start creates a temp file and writes a 44-byte WAV header with placeholder
// sizes. The recorder begins accepting samples via WriteSamples.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording || r.tempPath != "" {
		return fmt.Errorf("recorder already has an active or pending recording")
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
	r.tempPath = f.Name()
	r.dataSize = 0
	r.startTime = time.Now()
	r.recording = true
	r.writeErr = nil
	return nil
}

// WriteSamples converts float64 samples to float32 little-endian and appends
// them to the WAV data chunk. Silently returns if not recording.
func (r *Recorder) WriteSamples(samples []float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.recording || r.file == nil || r.writeErr != nil {
		return
	}

	buf := make([]byte, len(samples)*4)
	if uint64(r.dataSize)+uint64(len(buf)) > uint64(^uint32(0)) {
		r.writeErr = fmt.Errorf("recording exceeds WAV size limit")
		return
	}
	for i, s := range samples {
		bits := math.Float32bits(float32(s))
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], bits)
	}

	n, err := r.file.Write(buf)
	if err != nil {
		r.writeErr = fmt.Errorf("write recording: %w", err)
		return
	}
	if n != len(buf) {
		r.writeErr = io.ErrShortWrite
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

	if r.recording {
		return "", fmt.Errorf("stop the recording before saving")
	}
	if r.tempPath == "" {
		return "", fmt.Errorf("no recording to save")
	}
	if r.writeErr != nil {
		return "", r.writeErr
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	if r.file != nil {
		// Seek back to the beginning and rewrite the header with correct sizes.
		if _, err := r.file.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek to header: %w", err)
		}
		if err := writeWAVHeader(r.file, r.dataSize); err != nil {
			return "", fmt.Errorf("finalize WAV header: %w", err)
		}
		if err := r.file.Sync(); err != nil {
			return "", fmt.Errorf("sync temp file: %w", err)
		}
		if err := r.file.Close(); err != nil {
			r.file = nil
			return "", fmt.Errorf("close temp file: %w", err)
		}
		r.file = nil
	}

	var destPath string
	for sequence := 0; ; sequence++ {
		destPath = recordingPath(destDir, r.startTime, sequence)
		err := moveFileNoReplace(r.tempPath, destPath)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("move recording: %w", err)
		}
		break
	}

	r.tempPath = ""
	r.dataSize = 0
	r.writeErr = nil
	return destPath, nil
}

// Discard closes and deletes the temp file.
func (r *Recorder) Discard() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	if r.tempPath != "" {
		os.Remove(r.tempPath)
		r.tempPath = ""
	}
	r.dataSize = 0
	r.writeErr = nil
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

func recordingPath(destDir string, started time.Time, sequence int) string {
	ts := started.Format("2006-01-02-15-04-05")
	name := fmt.Sprintf("omanote-%s.wav", ts)
	if sequence > 0 {
		name = fmt.Sprintf("omanote-%s-%d.wav", ts, sequence+1)
	}
	return filepath.Join(destDir, name)
}

// moveFileNoReplace moves src to dst without overwriting an existing recording.
// A hard link avoids copying on the common same-filesystem path; copying is the
// fallback for cross-filesystem and filesystems without hard-link support.
func moveFileNoReplace(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		if err := os.Remove(src); err != nil {
			rollbackErr := os.Remove(dst)
			return errors.Join(err, rollbackErr)
		}
		return nil
	} else if os.IsExist(err) {
		return err
	}

	if err := copyFileExclusive(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		rollbackErr := os.Remove(dst)
		return errors.Join(err, rollbackErr)
	}
	return nil
}

func copyFileExclusive(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
