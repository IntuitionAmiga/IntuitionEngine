//go:build headless

package main

import (
	"encoding/binary"
	"reflect"
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
	if len(w.LeftSamples) != 50 || len(w.RightSamples) != 50 {
		t.Errorf("expected 50 frames, got L=%d R=%d", len(w.LeftSamples), len(w.RightSamples))
	}
}

func TestWAVParse8Bit(t *testing.T) {
	// 8-bit unsigned: 128=center(0), 0=-32768, 255=32512
	pcm := []byte{128, 0, 255}
	wav := buildTestWAV(22050, 1, 8, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.LeftSamples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(w.LeftSamples))
	}
	want := []int16{0, -32768, 32512}
	if !reflect.DeepEqual(w.LeftSamples, want) {
		t.Fatalf("LeftSamples = %v, want %v", w.LeftSamples, want)
	}
	if !reflect.DeepEqual(w.RightSamples, want) {
		t.Fatalf("RightSamples = %v, want %v", w.RightSamples, want)
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
	want := []int16{0, -32768, 32767}
	if !reflect.DeepEqual(w.LeftSamples, want) {
		t.Fatalf("LeftSamples = %v, want %v", w.LeftSamples, want)
	}
	if !reflect.DeepEqual(w.RightSamples, want) {
		t.Fatalf("RightSamples = %v, want %v", w.RightSamples, want)
	}
}

func TestWAVParseStereoPreserved(t *testing.T) {
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], uint16(int16(16384)))
	neg := int16(-16384)
	binary.LittleEndian.PutUint16(pcm[2:4], uint16(neg))

	wav := buildTestWAV(44100, 2, 16, pcm)

	w, err := ParseWAV(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.LeftSamples) != 1 || len(w.RightSamples) != 1 {
		t.Fatalf("expected 1 stereo frame, got L=%d R=%d", len(w.LeftSamples), len(w.RightSamples))
	}
	if w.LeftSamples[0] != 16384 || w.RightSamples[0] != -16384 {
		t.Fatalf("stereo not preserved: L=%d R=%d", w.LeftSamples[0], w.RightSamples[0])
	}
}

func TestWAVParseRejectZeroFrames(t *testing.T) {
	if _, err := ParseWAV(buildTestWAV(44100, 1, 16, nil)); err == nil {
		t.Fatal("expected zero-frame data chunk to be rejected")
	}
}

func TestWAVParseRejectTruncatedDataChunk(t *testing.T) {
	wav := buildTestWAV(44100, 1, 16, []byte{0, 0, 1, 0})
	binary.LittleEndian.PutUint32(wav[len(wav)-8:len(wav)-4], 100)
	if _, err := ParseWAV(wav); err == nil {
		t.Fatal("expected truncated data chunk to be rejected")
	}
}

func TestWAVParseValidatesBlockAlign(t *testing.T) {
	wav := buildTestWAV(44100, 2, 16, make([]byte, 4))
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	if _, err := ParseWAV(wav); err == nil {
		t.Fatal("expected invalid blockAlign to be rejected")
	}
}

func TestWAVParseValidatesByteRate(t *testing.T) {
	wav := buildTestWAV(44100, 1, 16, make([]byte, 2))
	binary.LittleEndian.PutUint32(wav[28:32], 1234)
	if _, err := ParseWAV(wav); err == nil {
		t.Fatal("expected invalid byteRate to be rejected")
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
	if len(w.LeftSamples) != 10 {
		t.Errorf("expected 10 samples, got %d", len(w.LeftSamples))
	}
	if w.LeftSamples[0] != 1000 {
		t.Errorf("sample[0]: expected 1000, got %d", w.LeftSamples[0])
	}
}

func TestWAVParseExtensible16BitPCM(t *testing.T) {
	wav := buildExtensibleTestWAV(44100, 2, 16, 16, wavPCMSubformatGUID, make([]byte, 4))
	if _, err := ParseWAV(wav); err != nil {
		t.Fatalf("expected extensible PCM to parse: %v", err)
	}
}

func TestWAVParseExtensibleRejectsFloatSubformat(t *testing.T) {
	wav := buildExtensibleTestWAV(44100, 2, 16, 16, wavFloatSubformatGUID, make([]byte, 4))
	if _, err := ParseWAV(wav); err == nil {
		t.Fatal("expected extensible float subformat to be rejected")
	}
}

func TestWAVParseExtensibleRejects24BitContainer(t *testing.T) {
	wav := buildExtensibleTestWAV(44100, 2, 24, 24, wavPCMSubformatGUID, make([]byte, 6))
	if _, err := ParseWAV(wav); err == nil {
		t.Fatal("expected 24-bit extensible container to be rejected")
	}
}

func TestWAVParseNoFloat32Buffer(t *testing.T) {
	if _, ok := reflect.TypeOf(WAVFile{}).FieldByName("Samples"); ok {
		t.Fatal("WAVFile must not expose the old float32 Samples buffer")
	}
}

func buildExtensibleTestWAV(sampleRate uint32, numChannels, bitsPerSample, validBits uint16, guid [16]byte, pcmData []byte) []byte {
	dataSize := uint32(len(pcmData))
	fmtSize := uint32(40)
	blockAlign := numChannels * bitsPerSample / 8
	bytesPerSec := sampleRate * uint32(blockAlign)
	fileSize := 4 + 8 + fmtSize + 8 + dataSize
	buf := make([]byte, 12+8+fmtSize+8+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], fileSize)
	copy(buf[8:12], "WAVE")
	off := 12
	copy(buf[off:off+4], "fmt ")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], fmtSize)
	binary.LittleEndian.PutUint16(buf[off+8:off+10], wavFormatExtensible)
	binary.LittleEndian.PutUint16(buf[off+10:off+12], numChannels)
	binary.LittleEndian.PutUint32(buf[off+12:off+16], sampleRate)
	binary.LittleEndian.PutUint32(buf[off+16:off+20], bytesPerSec)
	binary.LittleEndian.PutUint16(buf[off+20:off+22], blockAlign)
	binary.LittleEndian.PutUint16(buf[off+22:off+24], bitsPerSample)
	binary.LittleEndian.PutUint16(buf[off+24:off+26], 22)
	binary.LittleEndian.PutUint16(buf[off+26:off+28], validBits)
	copy(buf[off+32:off+48], guid[:])
	off += 8 + int(fmtSize)
	copy(buf[off:off+4], "data")
	binary.LittleEndian.PutUint32(buf[off+4:off+8], dataSize)
	copy(buf[off+8:], pcmData)
	return buf
}
