// ice_unpack_test.go - Tests and benchmarks for ICE unpacker

package main

import (
	"testing"
)

func TestICE_IsICE_Valid(t *testing.T) {
	// Create a minimal valid ICE header
	data := []byte{
		0x49, 0x43, 0x45, 0x21, // "ICE!" magic
		0x00, 0x00, 0x00, 0x10, // crunched length = 16
		0x00, 0x00, 0x00, 0x20, // decrunched length = 32
		0x00, 0x00, 0x00, 0x00, // padding
	}
	if !isICE(data) {
		t.Error("isICE should return true for valid ICE header")
	}
}

func TestICE_IsICE_Invalid(t *testing.T) {
	// Not ICE data
	data := []byte{0x00, 0x00, 0x00, 0x00}
	if isICE(data) {
		t.Error("isICE should return false for invalid data")
	}

	// Too short
	data = []byte{0x49, 0x43, 0x45}
	if isICE(data) {
		t.Error("isICE should return false for truncated data")
	}
}

func TestICE_Lengths(t *testing.T) {
	data := []byte{
		0x49, 0x43, 0x45, 0x21, // "ICE!" magic
		0x00, 0x00, 0x01, 0x00, // crunched length = 256
		0x00, 0x00, 0x02, 0x00, // decrunched length = 512
	}

	if crunchedLen := iceCrunchedLength(data); crunchedLen != 256 {
		t.Errorf("expected crunched length 256, got %d", crunchedLen)
	}

	if decrunchedLen := iceDecrunchedLength(data); decrunchedLen != 512 {
		t.Errorf("expected decrunched length 512, got %d", decrunchedLen)
	}
}

// BenchmarkICE_GetBit benchmarks the bit reading function
func BenchmarkICE_GetBit(b *testing.B) {
	// Create a state with some data to read
	data := make([]byte, 1024)
	for i := range data {
		data[i] = 0xAA // Alternating bits
	}
	data[0] = 0x49
	data[1] = 0x43
	data[2] = 0x45
	data[3] = 0x21

	state := &iceState{
		packed:    len(data),
		data:      data,
		bits:      0x80,
		dataLen:   len(data),
		outputLen: 0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset position periodically to avoid underflow
		if state.packed < iceHeaderSize+10 {
			state.packed = len(data)
			state.bits = 0x80
		}
		_ = iceGetBit(state)
	}
}

// BenchmarkICE_GetBits benchmarks multi-bit reading
func BenchmarkICE_GetBits(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = 0xAA
	}
	data[0] = 0x49
	data[1] = 0x43
	data[2] = 0x45
	data[3] = 0x21

	state := &iceState{
		packed:    len(data),
		data:      data,
		bits:      0x80,
		dataLen:   len(data),
		outputLen: 0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if state.packed < iceHeaderSize+20 {
			state.packed = len(data)
			state.bits = 0x80
		}
		_ = iceGetBits(state, 8)
	}
}

// BenchmarkICE_MemcpyBwd benchmarks backward memory copy
func BenchmarkICE_MemcpyBwd(b *testing.B) {
	output := make([]byte, 4096)
	// Initialize with some pattern
	for i := range output {
		output[i] = byte(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Copy 32 bytes backwards (typical match length)
		iceMemcpyBwd(output, 1000, 1100, 32)
	}
}

// BenchmarkICE_MemcpyBwd_Large benchmarks larger backward copy
func BenchmarkICE_MemcpyBwd_Large(b *testing.B) {
	output := make([]byte, 16384)
	for i := range output {
		output[i] = byte(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		iceMemcpyBwd(output, 1000, 2000, 256)
	}
}
