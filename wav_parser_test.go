//go:build headless

package main

import (
	"encoding/binary"
	"math"
	"testing"
)

// buildTestWAV constructs a minimal valid WAV file with the given parameters.
func buildTestWAV(sampleRate uint32, numChannels, bitsPerSample uint16, pcmData []byte) []byte {
	dataSize := uint32(len(pcmData))
	fmtSize := uint32(16)
	bytesPerSec := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8

	// Total file size: 4 (WAVE) + 8+16 (fmt) + 8+dataSize (data)
	fileSize := 4 + 8 + fmtSize + 8 + dataSize

	buf := make([]byte, 12+8+fmtSize+8+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], fileSize)
	copy(buf[8:12], "WAVE")

	// fmt chunk
	off := 12
	copy(buf[off:off+4], "fmt ")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], fmtSize)
	binary.LittleEndian.PutUint16(buf[off+8:off+10], 1) // PCM
	binary.LittleEndian.PutUint16(buf[off+10:off+12], numChannels)
	binary.LittleEndian.PutUint32(buf[off+12:off+16], sampleRate)
	binary.LittleEndian.PutUint32(buf[off+16:off+20], bytesPerSec)
	binary.LittleEndian.PutUint16(buf[off+20:off+22], blockAlign)
	binary.LittleEndian.PutUint16(buf[off+22:off+24], bitsPerSample)

	// data chunk
	off += 8 + int(fmtSize)
	copy(buf[off:off+4], "data")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], dataSize)
	copy(buf[off+8:], pcmData)

	return buf
}

func TestWAVParseHeader(t *testing.T) {
	pcm := make([]byte, 100) // 50 samples of 16-bit mono
	wav := buildTestWAV(44100, 1, 16, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.SampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", w.SampleRate)
	}
	if w.NumChannels != 1 {
		t.Errorf("expected 1 channel, got %d", w.NumChannels)
	}
	if w.BitsPerSample != 16 {
		t.Errorf("expected 16 bits, got %d", w.BitsPerSample)
	}
	if len(w.Samples) != 50 {
		t.Errorf("expected 50 samples, got %d", len(w.Samples))
	}
}

func TestWAVParse8Bit(t *testing.T) {
	// 8-bit unsigned: 128=center(0), 0=-1.0, 255≈+1.0
	pcm := []byte{128, 0, 255}
	wav := buildTestWAV(22050, 1, 8, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.Samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(w.Samples))
	}

	// Center (128) → 0.0
	if math.Abs(float64(w.Samples[0])) > 0.01 {
		t.Errorf("sample[0]: expected ~0.0, got %f", w.Samples[0])
	}
	// Min (0) → -1.0
	if math.Abs(float64(w.Samples[1])+1.0) > 0.01 {
		t.Errorf("sample[1]: expected ~-1.0, got %f", w.Samples[1])
	}
	// Max (255) → ~+0.992
	if w.Samples[2] < 0.98 || w.Samples[2] > 1.01 {
		t.Errorf("sample[2]: expected ~+0.99, got %f", w.Samples[2])
	}
}

func TestWAVParse16Bit(t *testing.T) {
	// 16-bit signed LE: 0=center, -32768=-1.0, 32767≈+1.0
	pcm := make([]byte, 6)
	binary.LittleEndian.PutUint16(pcm[0:2], 0)             // 0 → 0.0
	binary.LittleEndian.PutUint16(pcm[2:4], uint16(32768)) // -32768 → -1.0
	binary.LittleEndian.PutUint16(pcm[4:6], 32767)         // 32767 → ~+1.0

	wav := buildTestWAV(44100, 1, 16, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.Samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(w.Samples))
	}

	if math.Abs(float64(w.Samples[0])) > 0.01 {
		t.Errorf("sample[0]: expected ~0.0, got %f", w.Samples[0])
	}
	if math.Abs(float64(w.Samples[1])+1.0) > 0.01 {
		t.Errorf("sample[1]: expected ~-1.0, got %f", w.Samples[1])
	}
	if w.Samples[2] < 0.99 || w.Samples[2] > 1.01 {
		t.Errorf("sample[2]: expected ~+1.0, got %f", w.Samples[2])
	}
}

func TestWAVParseStereoDownmix(t *testing.T) {
	// Stereo 16-bit: L=16384(+0.5), R=-16384(-0.5) → mono average = 0.0
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], uint16(int16(16384)))
	neg := int16(-16384)
	binary.LittleEndian.PutUint16(pcm[2:4], uint16(neg))

	wav := buildTestWAV(44100, 2, 16, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.Samples) != 1 {
		t.Fatalf("expected 1 mono sample, got %d", len(w.Samples))
	}
	if math.Abs(float64(w.Samples[0])) > 0.01 {
		t.Errorf("expected ~0.0 after downmix, got %f", w.Samples[0])
	}
}

func TestWAVParseInvalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"short data", []byte{0, 1, 2}},
		{"missing RIFF", func() []byte {
			d := buildTestWAV(44100, 1, 16, make([]byte, 4))
			copy(d[0:4], "XXXX")
			return d
		}()},
		{"missing WAVE", func() []byte {
			d := buildTestWAV(44100, 1, 16, make([]byte, 4))
			copy(d[8:12], "XXXX")
			return d
		}()},
		{"non-PCM format", func() []byte {
			d := buildTestWAV(44100, 1, 16, make([]byte, 4))
			// Change audio format from 1 (PCM) to 3 (float)
			binary.LittleEndian.PutUint16(d[20:22], 3)
			return d
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseWAV(tt.data)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestWAVParseExtraChunks(t *testing.T) {
	// Build a WAV with an extra "LIST" chunk before the data chunk
	sampleRate := uint32(44100)
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	pcm := make([]byte, 20) // 10 samples
	binary.LittleEndian.PutUint16(pcm[0:2], uint16(int16(1000)))

	// Build manually with extra chunk
	fmtSize := uint32(16)
	extraChunkData := []byte("extra padding data!!")
	extraChunkSize := uint32(len(extraChunkData))
	dataSize := uint32(len(pcm))
	fileSize := 4 + (8 + fmtSize) + (8 + extraChunkSize) + (8 + dataSize)

	buf := make([]byte, 12+(8+fmtSize)+(8+extraChunkSize)+(8+dataSize))
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], fileSize)
	copy(buf[8:12], "WAVE")

	off := 12
	// fmt chunk
	copy(buf[off:off+4], "fmt ")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], fmtSize)
	binary.LittleEndian.PutUint16(buf[off+8:off+10], 1)
	binary.LittleEndian.PutUint16(buf[off+10:off+12], numChannels)
	binary.LittleEndian.PutUint32(buf[off+12:off+16], sampleRate)
	binary.LittleEndian.PutUint32(buf[off+16:off+20], sampleRate*uint32(numChannels)*uint32(bitsPerSample)/8)
	binary.LittleEndian.PutUint16(buf[off+20:off+22], numChannels*bitsPerSample/8)
	binary.LittleEndian.PutUint16(buf[off+22:off+24], bitsPerSample)
	off += 8 + int(fmtSize)

	// Extra chunk (LIST)
	copy(buf[off:off+4], "LIST")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], extraChunkSize)
	copy(buf[off+8:off+8+int(extraChunkSize)], extraChunkData)
	off += 8 + int(extraChunkSize)

	// data chunk
	copy(buf[off:off+4], "data")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], dataSize)
	copy(buf[off+8:], pcm)

	w, err := ParseWAV(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.Samples) != 10 {
		t.Errorf("expected 10 samples, got %d", len(w.Samples))
	}
	// First sample should be 1000/32768 ≈ 0.0305
	expected := float32(1000) / 32768.0
	if math.Abs(float64(w.Samples[0]-expected)) > 0.001 {
		t.Errorf("sample[0]: expected %f, got %f", expected, w.Samples[0])
	}
}
