package main

import (
	"encoding/binary"
	"fmt"
)

const (
	wavFormatPCM        = 0x0001
	wavFormatExtensible = 0xFFFE
)

var (
	wavPCMSubformatGUID   = [16]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
	wavFloatSubformatGUID = [16]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
)

// WAVFile holds parsed WAV audio data as signed 16-bit source frames.
type WAVFile struct {
	SampleRate    uint32
	NumChannels   uint16
	BitsPerSample uint16
	LeftSamples   []int16
	RightSamples  []int16
}

// ParseWAV parses a RIFF/WAVE file.
// Supports 8-bit unsigned and 16-bit signed PCM, including 16-bit PCM in a
// WAVE_FORMAT_EXTENSIBLE container. Stereo is preserved as L/R int16 frames.
func ParseWAV(data []byte) (*WAVFile, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("wav: data too short (%d bytes)", len(data))
	}
	if string(data[0:4]) != "RIFF" {
		return nil, fmt.Errorf("wav: missing RIFF magic")
	}
	if string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: missing WAVE magic")
	}

	var fmtChunk []byte
	var dataChunk []byte
	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		chunkData := pos + 8
		if chunkData+chunkSize > len(data) {
			return nil, fmt.Errorf("wav: truncated %q chunk", chunkID)
		}
		switch chunkID {
		case "fmt ":
			fmtChunk = data[chunkData : chunkData+chunkSize]
		case "data":
			dataChunk = data[chunkData : chunkData+chunkSize]
		}
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
	numChannels := binary.LittleEndian.Uint16(fmtChunk[2:4])
	sampleRate := binary.LittleEndian.Uint32(fmtChunk[4:8])
	byteRate := binary.LittleEndian.Uint32(fmtChunk[8:12])
	blockAlign := binary.LittleEndian.Uint16(fmtChunk[12:14])
	bitsPerSample := binary.LittleEndian.Uint16(fmtChunk[14:16])

	if numChannels == 0 {
		return nil, fmt.Errorf("wav: zero channels")
	}
	if numChannels > 2 {
		return nil, fmt.Errorf("wav: unsupported channel count %d", numChannels)
	}
	if sampleRate == 0 {
		return nil, fmt.Errorf("wav: zero sample rate")
	}
	if audioFormat == wavFormatExtensible {
		if len(fmtChunk) < 40 {
			return nil, fmt.Errorf("wav: extensible fmt chunk too short")
		}
		validBits := binary.LittleEndian.Uint16(fmtChunk[18:20])
		var guid [16]byte
		copy(guid[:], fmtChunk[24:40])
		if guid == wavFloatSubformatGUID {
			return nil, fmt.Errorf("wav: unsupported extensible float subformat")
		}
		if guid != wavPCMSubformatGUID {
			return nil, fmt.Errorf("wav: unsupported extensible subformat")
		}
		if validBits != 16 || bitsPerSample != 16 {
			return nil, fmt.Errorf("wav: unsupported extensible bit depth valid=%d container=%d", validBits, bitsPerSample)
		}
		audioFormat = wavFormatPCM
	}
	if audioFormat != wavFormatPCM {
		return nil, fmt.Errorf("wav: unsupported format %d", audioFormat)
	}
	if bitsPerSample != 8 && bitsPerSample != 16 {
		return nil, fmt.Errorf("wav: unsupported bits per sample %d (only 8 or 16 supported)", bitsPerSample)
	}

	bytesPerSample := int(bitsPerSample) / 8
	frameSize := bytesPerSample * int(numChannels)
	if int(blockAlign) != frameSize {
		return nil, fmt.Errorf("wav: invalid block align %d, want %d", blockAlign, frameSize)
	}
	if byteRate != sampleRate*uint32(blockAlign) {
		return nil, fmt.Errorf("wav: invalid byte rate %d, want %d", byteRate, sampleRate*uint32(blockAlign))
	}
	if len(dataChunk)%frameSize != 0 {
		return nil, fmt.Errorf("wav: data chunk is not frame aligned")
	}
	numFrames := len(dataChunk) / frameSize
	if numFrames == 0 {
		return nil, fmt.Errorf("wav: zero sample frames")
	}

	left := make([]int16, numFrames)
	right := make([]int16, numFrames)
	for i := range numFrames {
		frameOff := i * frameSize
		for ch := range int(numChannels) {
			sampleOff := frameOff + ch*bytesPerSample
			val := decodeWAVSample(dataChunk[sampleOff:], bitsPerSample)
			if ch == 0 {
				left[i] = val
			} else {
				right[i] = val
			}
		}
		if numChannels == 1 {
			right[i] = left[i]
		}
	}

	return &WAVFile{
		SampleRate:    sampleRate,
		NumChannels:   numChannels,
		BitsPerSample: bitsPerSample,
		LeftSamples:   left,
		RightSamples:  right,
	}, nil
}

func decodeWAVSample(data []byte, bitsPerSample uint16) int16 {
	if bitsPerSample == 8 {
		return int16(int(data[0])-128) << 8
	}
	return int16(binary.LittleEndian.Uint16(data[:2]))
}
