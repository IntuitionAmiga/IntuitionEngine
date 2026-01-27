// music_common.go - Shared utilities for music players and parsers

package main

import "strings"

// writeUint32Byte writes a single byte to a uint32 at the given byte index (0-3)
// Used by register-mapped players for byte-at-a-time register writes
func writeUint32Byte(current uint32, value uint32, byteIndex uint32) uint32 {
	shift := byteIndex * 8
	mask := uint32(0xFF) << shift
	return (current & ^mask) | ((value & 0xFF) << shift)
}

// writeUint32Word writes a 16-bit word to a uint32 starting at the given byte index
// Handles both low and high bytes when value > 0xFF
func writeUint32Word(current uint32, value uint32, byteIndex uint32) uint32 {
	current = writeUint32Byte(current, value, byteIndex)
	if value > 0xFF {
		current = writeUint32Byte(current, value>>8, byteIndex+1)
	}
	return current
}

// readUint32Byte reads a single byte from a uint32 at the given byte index (0-3)
// Used by register-mapped players for byte-at-a-time register reads
func readUint32Byte(value uint32, byteIndex uint32) uint32 {
	shift := byteIndex * 8
	return (value >> shift) & 0xFF
}

// parseNullTerminatedString extracts a string up to the first null byte
// Returns the string and the new offset (after the null terminator)
// Used by SNDH and other formats that use null-terminated strings
func parseNullTerminatedString(data []byte, offset int) (string, int) {
	start := offset
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	end := offset
	if offset < len(data) {
		offset++ // Skip null terminator
	}
	if end <= start {
		return "", offset
	}
	return string(data[start:end]), offset
}

// parsePaddedString extracts a string from a fixed-size field,
// trimming trailing null bytes and spaces
// Used by SID, TED, and other formats with fixed-width string fields
func parsePaddedString(data []byte) string {
	// Find first null byte
	end := len(data)
	for i, b := range data {
		if b == 0 {
			end = i
			break
		}
	}
	return strings.TrimRight(string(data[:end]), " ")
}

// MusicMetadata contains common metadata fields across all music formats
type MusicMetadata struct {
	Title    string
	Author   string
	System   string // "C64", "Atari ST", "ZX Spectrum", etc.
	Date     string
	Subsongs int
	Duration float64
}

// PlayerControlState contains common state for memory-mapped playback control
// This state is used by players that support register-mapped control from
// running programs (SID, PSG, TED players)
type PlayerControlState struct {
	PlayPtrStaged uint32
	PlayLenStaged uint32
	PlayPtr       uint32
	PlayLen       uint32
	PlayBusy      bool
	PlayErr       bool
	ForceLoop     bool
	Bus           MemoryBus
}

// HandlePtrWrite handles writes to PLAY_PTR register bytes
// offset is the byte offset within the 32-bit register (0-3)
func (s *PlayerControlState) HandlePtrWrite(offset uint32, value uint32) {
	if offset == 0 {
		s.PlayPtrStaged = value
	} else {
		s.PlayPtrStaged = writeUint32Byte(s.PlayPtrStaged, value, offset)
	}
}

// HandlePtrWordWrite handles 16-bit word writes to PLAY_PTR register
func (s *PlayerControlState) HandlePtrWordWrite(offset uint32, value uint32) {
	s.PlayPtrStaged = writeUint32Word(s.PlayPtrStaged, value, offset)
}

// HandleLenWrite handles writes to PLAY_LEN register bytes
// offset is the byte offset within the 32-bit register (0-3)
func (s *PlayerControlState) HandleLenWrite(offset uint32, value uint32) {
	if offset == 0 {
		s.PlayLenStaged = value
	} else {
		s.PlayLenStaged = writeUint32Byte(s.PlayLenStaged, value, offset)
	}
}

// HandleLenWordWrite handles 16-bit word writes to PLAY_LEN register
func (s *PlayerControlState) HandleLenWordWrite(offset uint32, value uint32) {
	s.PlayLenStaged = writeUint32Word(s.PlayLenStaged, value, offset)
}

// ReadPtrByte reads a byte from the PlayPtrStaged register
func (s *PlayerControlState) ReadPtrByte(offset uint32) uint32 {
	if offset == 0 {
		return s.PlayPtrStaged
	}
	return readUint32Byte(s.PlayPtrStaged, offset)
}

// ReadLenByte reads a byte from the PlayLenStaged register
func (s *PlayerControlState) ReadLenByte(offset uint32) uint32 {
	if offset == 0 {
		return s.PlayLenStaged
	}
	return readUint32Byte(s.PlayLenStaged, offset)
}

// PreparePlay validates and stages the playback parameters
// Returns error string if validation fails, empty string on success
func (s *PlayerControlState) PreparePlay(forceLoop bool) string {
	s.PlayPtr = s.PlayPtrStaged
	s.PlayLen = s.PlayLenStaged
	s.ForceLoop = forceLoop
	s.PlayErr = false

	if s.Bus == nil {
		s.PlayErr = true
		return "no bus attached"
	}
	if s.PlayLen == 0 {
		s.PlayErr = true
		return "zero length"
	}
	return ""
}

// ReadDataFromBus reads playback data from attached memory bus
// Returns the data slice or error
func (s *PlayerControlState) ReadDataFromBus() ([]byte, error) {
	if s.Bus == nil {
		s.PlayErr = true
		return nil, nil
	}

	mem := s.Bus.GetMemory()
	if int(s.PlayPtr)+int(s.PlayLen) > len(mem) {
		s.PlayErr = true
		s.PlayBusy = false
		return nil, nil
	}

	data := make([]byte, s.PlayLen)
	copy(data, mem[s.PlayPtr:s.PlayPtr+s.PlayLen])
	return data, nil
}

// PlayStatus returns the current busy/error status word
// Bit 0 = busy, Bit 1 = error
func (s *PlayerControlState) PlayStatus(enginePlaying bool) uint32 {
	ctrl := uint32(0)
	busy := s.PlayBusy
	if enginePlaying {
		busy = true
	} else if !busy {
		s.PlayBusy = false
	}
	if busy {
		ctrl |= 1
	}
	if s.PlayErr {
		ctrl |= 2
	}
	return ctrl
}

// SetError marks the playback as failed and clears busy flag
func (s *PlayerControlState) SetError() {
	s.PlayErr = true
	s.PlayBusy = false
}

// ClearError clears the error flag
func (s *PlayerControlState) ClearError() {
	s.PlayErr = false
}

// SetBusy marks the playback as busy
func (s *PlayerControlState) SetBusy() {
	s.PlayBusy = true
}
