// ice_unpack.go - ICE packer decompression for Atari ST SNDH files.
//
// Based on Pack-Ice by Axe of Delight/Superior.
// Ported from C implementation at https://github.com/larsbrinkhoff/pack-ice

package main

import (
	"encoding/binary"
	"fmt"
)

const (
	iceMagic      = 0x49434521 // "ICE!"
	iceHeaderSize = 12
)

// Pre-computed lookup tables for faster decoding
var (
	// iceGetDepackLength lookup tables
	iceDepackBitsToGet   = [5]int{0, 0, 1, 2, 10}
	iceDepackNumberToAdd = [5]int{2, 3, 4, 6, 10}

	// iceGetDepackOffset lookup tables (length != 2 case)
	iceOffsetBitsToGet   = [3]int{8, 5, 12}
	iceOffsetNumberToAdd = [3]int{31, -1, 287}

	// iceGetDirectLength lookup tables
	iceDirectBitsToGet   = [6]int{1, 2, 2, 3, 8, 15}
	iceDirectAllOnes     = [6]int{1, 3, 3, 7, 0xff, 0x7fff}
	iceDirectNumberToAdd = [7]int{1, 2, 5, 8, 15, 270, 270}
)

// iceState holds the decompression state
type iceState struct {
	unpackedStop int    // destination start position
	unpacked     int    // current write position (works backwards)
	packed       int    // current read position (works backwards)
	bits         int    // bit buffer (8-bit with marker bit)
	data         []byte // packed data
	output       []byte // output buffer
	dataLen      int    // cached length of data for bounds check
	outputLen    int    // cached length of output for bounds check
}

// isICE checks if data starts with ICE! magic
func isICE(data []byte) bool {
	if len(data) < iceHeaderSize {
		return false
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	return magic == iceMagic
}

// iceCrunchedLength returns the compressed data length (including header)
func iceCrunchedLength(data []byte) int {
	if !isICE(data) {
		return 0
	}
	return int(binary.BigEndian.Uint32(data[4:8]))
}

// iceDecrunchedLength returns the uncompressed data length
func iceDecrunchedLength(data []byte) int {
	if !isICE(data) {
		return 0
	}
	return int(binary.BigEndian.Uint32(data[8:12]))
}

// UnpackICE decompresses ICE-packed data and returns the unpacked data
func UnpackICE(data []byte) ([]byte, error) {
	if !isICE(data) {
		return nil, fmt.Errorf("not ICE packed data")
	}

	crunchedLen := iceCrunchedLength(data)
	decrunchedLen := iceDecrunchedLength(data)

	if crunchedLen <= 0 || decrunchedLen <= 0 {
		return nil, fmt.Errorf("invalid ICE lengths: crunched=%d, decrunched=%d", crunchedLen, decrunchedLen)
	}

	if len(data) < crunchedLen {
		return nil, fmt.Errorf("ICE data truncated: have %d, need %d", len(data), crunchedLen)
	}

	output := make([]byte, decrunchedLen)

	state := &iceState{
		unpackedStop: 0,
		packed:       crunchedLen,
		unpacked:     decrunchedLen,
		data:         data,
		output:       output,
		dataLen:      len(data),
		outputLen:    decrunchedLen,
	}

	// Initialize bit buffer from last byte of packed data
	state.packed--
	state.bits = int(state.data[state.packed])

	// Run decompression
	if err := iceNormalBytes(state); err != nil {
		return nil, err
	}

	return output, nil
}

// iceGetBit reads a single bit from the bit stream
func iceGetBit(state *iceState) int {
	bit := 0
	if (state.bits & 0x80) != 0 {
		bit = 1
	}
	state.bits = (state.bits << 1) & 0xff

	if state.bits == 0 {
		state.packed--
		if state.packed < iceHeaderSize {
			// Underflow protection
			state.bits = 1
			return bit
		}
		state.bits = int(state.data[state.packed])
		if (state.bits & 0x80) != 0 {
			bit = 1
		} else {
			bit = 0
		}
		state.bits = ((state.bits << 1) & 0xff) + 1
	}

	return bit
}

// iceGetBits reads n bits from the bit stream
func iceGetBits(state *iceState, n int) int {
	bits := 0
	for n > 0 {
		bits = (bits << 1) | iceGetBit(state)
		n--
	}
	return bits
}

// iceGetDepackLength decodes the match length
// Uses pre-computed lookup tables for faster decoding.
func iceGetDepackLength(state *iceState) int {
	i := 0
	for i < 4 && iceGetBit(state) != 0 {
		i++
	}

	length := 0
	if bits := iceDepackBitsToGet[i]; bits > 0 {
		length = iceGetBits(state, bits)
	}
	return length + iceDepackNumberToAdd[i]
}

// iceGetDepackOffset decodes the match offset
// Uses pre-computed lookup tables for faster decoding.
func iceGetDepackOffset(state *iceState, length int) int {
	var offset, bits, add int

	if length == 2 {
		if iceGetBit(state) != 0 {
			bits = 9
			add = 0x3f
		} else {
			bits = 6
			add = -1
		}
		offset = iceGetBits(state, bits) + add
	} else {
		i := 0
		for i < 2 && iceGetBit(state) != 0 {
			i++
		}

		bits = iceOffsetBitsToGet[i]
		add = iceOffsetNumberToAdd[i]
		offset = iceGetBits(state, bits) + add
		if offset < 0 {
			offset -= length - 2
		}
	}

	return offset
}

// iceGetDirectLength decodes the literal copy length
// Uses pre-computed lookup tables for faster decoding.
func iceGetDirectLength(state *iceState) int {
	i := 0
	n := 0
	for i < 6 {
		n = iceGetBits(state, iceDirectBitsToGet[i])
		if n != iceDirectAllOnes[i] {
			break
		}
		i++
	}
	return n + iceDirectNumberToAdd[i]
}

// iceMemcpyBwd copies n bytes backwards (for overlapping regions)
// Caller must ensure all indices are within bounds.
func iceMemcpyBwd(output []byte, to, from, n int) {
	// Fast path: validate bounds once, then copy without per-byte checks
	outLen := len(output)
	toEnd := to + n
	fromEnd := from + n

	// Bounds validation (single check)
	if to < 0 || toEnd > outLen || from < 0 || fromEnd > outLen {
		// Fallback to safe byte-by-byte copy for edge cases
		for i := n - 1; i >= 0; i-- {
			toIdx := to + i
			fromIdx := from + i
			if toIdx >= 0 && toIdx < outLen && fromIdx >= 0 && fromIdx < outLen {
				output[toIdx] = output[fromIdx]
			}
		}
		return
	}

	// Optimized path: copy backwards without bounds checks
	// This handles overlapping regions correctly
	for i := n - 1; i >= 0; i-- {
		output[to+i] = output[from+i]
	}
}

// iceNormalBytes is the main decompression loop
func iceNormalBytes(state *iceState) error {
	for {
		// Check for literal copy
		if iceGetBit(state) != 0 {
			length := iceGetDirectLength(state)
			state.packed -= length
			state.unpacked -= length

			if state.unpacked < state.unpackedStop {
				return fmt.Errorf("ice unpack: output underflow during literal copy")
			}
			if state.packed < iceHeaderSize {
				return fmt.Errorf("ice unpack: input underflow during literal copy")
			}

			// Copy literal bytes from packed stream to output
			copy(state.output[state.unpacked:], state.data[state.packed:state.packed+length])
		}

		// Check if we're done
		if state.unpacked <= state.unpackedStop {
			return nil
		}

		// Handle back-reference (LZ77-style match)
		length := iceGetDepackLength(state)
		offset := iceGetDepackOffset(state, length)

		state.unpacked -= length

		if state.unpacked < state.unpackedStop {
			return fmt.Errorf("ice unpack: output underflow during match copy")
		}

		// Copy from already-decoded output (backwards to handle overlap)
		iceMemcpyBwd(state.output, state.unpacked, state.unpacked+length+offset, length)
	}
}
