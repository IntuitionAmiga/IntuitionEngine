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

// iceState holds the decompression state
type iceState struct {
	unpackedStop int    // destination start position
	unpacked     int    // current write position (works backwards)
	packed       int    // current read position (works backwards)
	bits         int    // bit buffer
	data         []byte // packed data
	output       []byte // output buffer
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
func iceGetDepackLength(state *iceState) int {
	bitsToGet := []int{0, 0, 1, 2, 10}
	numberToAdd := []int{2, 3, 4, 6, 10}

	i := 0
	for i < 4 {
		if iceGetBit(state) == 0 {
			break
		}
		i++
	}

	length := 0
	if bitsToGet[i] > 0 {
		length = iceGetBits(state, bitsToGet[i])
	}
	length += numberToAdd[i]

	return length
}

// iceGetDepackOffset decodes the match offset
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
		bitsToGet := []int{8, 5, 12}
		numberToAdd := []int{31, -1, 287}

		i := 0
		for i < 2 {
			if iceGetBit(state) == 0 {
				break
			}
			i++
		}

		bits = bitsToGet[i]
		add = numberToAdd[i]
		offset = iceGetBits(state, bits) + add
		if offset < 0 {
			offset -= length - 2
		}
	}

	return offset
}

// iceGetDirectLength decodes the literal copy length
func iceGetDirectLength(state *iceState) int {
	bitsToGet := []int{1, 2, 2, 3, 8, 15}
	allOnes := []int{1, 3, 3, 7, 0xff, 0x7fff}
	numberToAdd := []int{1, 2, 5, 8, 15, 270, 270}

	i := 0
	n := 0
	for i < 6 {
		n = iceGetBits(state, bitsToGet[i])
		if n != allOnes[i] {
			break
		}
		i++
	}
	n += numberToAdd[i]

	return n
}

// iceMemcpyBwd copies n bytes backwards (for overlapping regions)
func iceMemcpyBwd(output []byte, to, from, n int) {
	to += n
	from += n
	for n > 0 {
		to--
		from--
		if to >= 0 && to < len(output) && from >= 0 && from < len(output) {
			output[to] = output[from]
		}
		n--
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
