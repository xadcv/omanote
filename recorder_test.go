package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecorder_StartCreatesValidWAVHeader(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rec.Discard()

	// Read back the 44-byte header from the temp file.
	path := rec.file.Name()
	hdr := make([]byte, wavHeaderSize)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	defer f.Close()

	if _, err := f.Read(hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}

	// RIFF marker
	if string(hdr[0:4]) != "RIFF" {
		t.Errorf("expected RIFF, got %q", string(hdr[0:4]))
	}

	// WAVE marker
	if string(hdr[8:12]) != "WAVE" {
		t.Errorf("expected WAVE, got %q", string(hdr[8:12]))
	}

	// fmt chunk
	if string(hdr[12:16]) != "fmt " {
		t.Errorf("expected 'fmt ', got %q", string(hdr[12:16]))
	}

	// Audio format: 3 = IEEE float
	audioFmt := binary.LittleEndian.Uint16(hdr[20:22])
	if audioFmt != 3 {
		t.Errorf("audio format: want 3, got %d", audioFmt)
	}

	// Channels: 1
	channels := binary.LittleEndian.Uint16(hdr[22:24])
	if channels != 1 {
		t.Errorf("channels: want 1, got %d", channels)
	}

	// Sample rate: 48000
	rate := binary.LittleEndian.Uint32(hdr[24:28])
	if rate != 48000 {
		t.Errorf("sample rate: want 48000, got %d", rate)
	}

	// Bits per sample: 32
	bps := binary.LittleEndian.Uint16(hdr[34:36])
	if bps != 32 {
		t.Errorf("bits per sample: want 32, got %d", bps)
	}

	// data chunk marker
	if string(hdr[36:40]) != "data" {
		t.Errorf("expected 'data', got %q", string(hdr[36:40]))
	}
}

func TestRecorder_WriteSamples(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rec.Discard()

	if !rec.IsRecording() {
		t.Fatal("expected IsRecording()=true after Start")
	}

	// Generate 100 samples of a 440Hz sine wave.
	const nSamples = 100
	samples := make([]float64, nSamples)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / float64(wavSampleRate))
	}

	rec.WriteSamples(samples)

	// File should be header (44) + 100 samples * 4 bytes = 444 bytes.
	info, err := os.Stat(rec.file.Name())
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}

	want := int64(wavHeaderSize + nSamples*4)
	if info.Size() != want {
		t.Errorf("file size: want %d, got %d", want, info.Size())
	}
}

func TestRecorder_Save(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Write 48000 samples (1 second of audio).
	const nSamples = 48000
	samples := make([]float64, nSamples)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / float64(wavSampleRate))
	}
	rec.WriteSamples(samples)

	rec.Stop()

	destDir := t.TempDir()
	path, err := rec.Save(destDir)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists and matches omanote-*.wav pattern.
	base := filepath.Base(path)
	matched, err := filepath.Match("omanote-*.wav", base)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if !matched {
		t.Errorf("filename %q does not match omanote-*.wav", base)
	}

	// Read back the finalized header.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open saved file: %v", err)
	}
	defer f.Close()

	hdr := make([]byte, wavHeaderSize)
	if _, err := f.Read(hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}

	expectedDataSize := uint32(nSamples * 4)

	// data chunk size
	dataSize := binary.LittleEndian.Uint32(hdr[40:44])
	if dataSize != expectedDataSize {
		t.Errorf("data size: want %d, got %d", expectedDataSize, dataSize)
	}

	// RIFF size = 36 + dataSize
	riffSize := binary.LittleEndian.Uint32(hdr[4:8])
	if riffSize != 36+expectedDataSize {
		t.Errorf("RIFF size: want %d, got %d", 36+expectedDataSize, riffSize)
	}
}

func TestRecorder_Discard(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	tmpPath := rec.file.Name()

	rec.Discard()

	// Temp file should no longer exist.
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file still exists after Discard: %s", tmpPath)
	}

	// Should not be recording.
	if rec.IsRecording() {
		t.Error("expected IsRecording()=false after Discard")
	}
}

func TestRecorder_SaveDoesNotOverwriteSameSecondRecording(t *testing.T) {
	destDir := t.TempDir()
	started := time.Date(2026, 8, 15, 13, 0, 0, 0, time.Local)

	save := func(sample float64) string {
		t.Helper()
		var rec Recorder
		if err := rec.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		rec.startTime = started
		rec.WriteSamples([]float64{sample})
		rec.Stop()
		path, err := rec.Save(destDir)
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		return path
	}

	first := save(0.25)
	second := save(0.75)
	if first == second {
		t.Fatalf("recordings used the same path: %s", first)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("first recording was overwritten or removed: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("second recording missing: %v", err)
	}
}

func TestRecorder_SaveCanRetryAfterDestinationError(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rec.Discard()
	rec.WriteSamples([]float64{0.25})
	rec.Stop()

	parent := t.TempDir()
	notDir := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}
	if _, err := rec.Save(filepath.Join(notDir, "recordings")); err == nil {
		t.Fatal("Save succeeded with an invalid destination")
	}
	if rec.tempPath == "" {
		t.Fatal("failed Save discarded the pending recording path")
	}
	if _, err := os.Stat(rec.tempPath); err != nil {
		t.Fatalf("pending recording missing after failed Save: %v", err)
	}

	path, err := rec.Save(filepath.Join(parent, "recordings"))
	if err != nil {
		t.Fatalf("retry Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("retried recording missing: %v", err)
	}
}

func TestRecorder_StartRejectsPendingRecording(t *testing.T) {
	var rec Recorder
	if err := rec.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rec.Discard()
	rec.Stop()

	if err := rec.Start(); err == nil {
		t.Fatal("Start accepted a second recording while one was pending")
	}
}
