package main

import (
	"encoding/binary"
	"fmt"
)

// WAVFile holds parsed WAV audio data with samples normalized to float32.
type WAVFile struct {
	SampleRate    uint32
	NumChannels   uint16
	BitsPerSample uint16
	Samples       []float32 // normalized to [-1.0, +1.0], mono (downmixed if stereo)
}

// ParseWAV parses a RIFF/WAVE file and returns normalized float32 samples.
// Supports 8-bit unsigned and 16-bit signed PCM. Stereo is downmixed to mono.
func ParseWAV(data []byte) (*WAVFile, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("wav: data too short (%d bytes)", len(data))
	}

	// Validate RIFF header
	if string(data[0:4]) != "RIFF" {
		return nil, fmt.Errorf("wav: missing RIFF magic")
	}
	if string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: missing WAVE magic")
	}

	// Find fmt and data chunks (can appear in any order)
	var fmtChunk []byte
	var dataChunk []byte
	pos := 12

	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		chunkData := pos + 8

		if chunkData+chunkSize > len(data) {
			// Truncated chunk — use whatever is available for data chunk
			if chunkID == "data" && dataChunk == nil {
				dataChunk = data[chunkData:]
			}
			break
		}

		switch chunkID {
		case "fmt ":
			fmtChunk = data[chunkData : chunkData+chunkSize]
		case "data":
			dataChunk = data[chunkData : chunkData+chunkSize]
		}

		// Advance to next chunk (chunks are word-aligned)
		pos = chunkData + chunkSize
		if chunkSize%2 != 0 {
			pos++
		}
	}

	if fmtChunk == nil {
		return nil, fmt.Errorf("wav: missing fmt chunk")
	}
	if len(fmtChunk) < 16 {
		return nil, fmt.Errorf("wav: fmt chunk too short")
	}
	if dataChunk == nil {
		return nil, fmt.Errorf("wav: missing data chunk")
	}

	audioFormat := binary.LittleEndian.Uint16(fmtChunk[0:2])
	if audioFormat != 1 {
		return nil, fmt.Errorf("wav: unsupported format %d (only PCM=1 supported)", audioFormat)
	}

	numChannels := binary.LittleEndian.Uint16(fmtChunk[2:4])
	sampleRate := binary.LittleEndian.Uint32(fmtChunk[4:8])
	bitsPerSample := binary.LittleEndian.Uint16(fmtChunk[14:16])

	if numChannels == 0 {
		return nil, fmt.Errorf("wav: zero channels")
	}
	if sampleRate == 0 {
		return nil, fmt.Errorf("wav: zero sample rate")
	}
	if bitsPerSample != 8 && bitsPerSample != 16 {
		return nil, fmt.Errorf("wav: unsupported bits per sample %d (only 8 or 16 supported)", bitsPerSample)
	}

	// Decode samples and normalize to float32
	bytesPerSample := int(bitsPerSample) / 8
	frameSize := bytesPerSample * int(numChannels)
	if frameSize == 0 {
		return nil, fmt.Errorf("wav: zero frame size")
	}
	numFrames := len(dataChunk) / frameSize

	samples := make([]float32, numFrames)

	for i := range numFrames {
		frameOff := i * frameSize
		var monoSum float32

		for ch := range int(numChannels) {
			sampleOff := frameOff + ch*bytesPerSample
			var val float32

			switch bitsPerSample {
			case 8:
				// 8-bit unsigned: 0-255, center at 128
				val = float32(int(dataChunk[sampleOff])-128) / 128.0
			case 16:
				// 16-bit signed little-endian
				raw := int16(binary.LittleEndian.Uint16(dataChunk[sampleOff : sampleOff+2]))
				val = float32(raw) / 32768.0
			}

			monoSum += val
		}

		// Downmix to mono
		samples[i] = monoSum / float32(numChannels)
	}

	return &WAVFile{
		SampleRate:    sampleRate,
		NumChannels:   numChannels,
		BitsPerSample: bitsPerSample,
		Samples:       samples,
	}, nil
}
