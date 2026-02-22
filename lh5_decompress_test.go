package main

import (
	"bytes"
	"testing"
)

// testCompressLH5 creates valid LH5 compressed data for testing purposes.
// Uses all-literal encoding: every byte is encoded as its own Huffman code
// with fixed 8-bit code length. No backreferences, no actual compression.
//
// Block format:
//
//	16 bits: block size (number of symbols)
//	NT tree: n=0 (5 bits), c=10 (5 bits) → constant "code length 8"
//	C tree:  n=256 (9 bits) → 256 entries, each decoded as NT value 10 → actual length 8
//	P tree:  n=0 (4 bits), c=0 (4 bits) → not used
//	Data:    N × 8-bit literal codes (identity: code = byte value)
func testCompressLH5(data []byte) []byte {
	w := &testBitWriter{}

	// Process in blocks of up to 65535 symbols
	offset := 0
	for offset < len(data) {
		blockSize := min(len(data)-offset, 65535)

		// Block header: 16-bit block size
		w.putBits(16, uint32(blockSize))

		// NT tree: n=0 (constant mode), value=10
		// NT value 10 means "code length = 10 - 2 = 8"
		w.putBits(5, 0)  // n = 0 entries (constant mode)
		w.putBits(5, 10) // constant value = 10

		// C tree: n=256 entries
		// Each entry's code length is decoded using NT tree → all return 10 → length 8
		// When NT tree is constant, each lookup costs 0 bits from the stream
		w.putBits(9, 256) // n = 256 entries (covers all literal bytes)

		// P tree: n=0 (constant mode), value=0
		w.putBits(4, 0) // n = 0
		w.putBits(4, 0) // constant = 0

		// Emit each byte as its 8-bit identity code
		for i := range blockSize {
			w.putBits(8, uint32(data[offset+i]))
		}

		offset += blockSize
	}

	w.flush()
	return w.buf
}

type testBitWriter struct {
	buf    []byte
	bitBuf uint32
	bitCnt int // number of valid bits from MSB
}

func (w *testBitWriter) putBits(n int, value uint32) {
	// Add n bits (MSB-first) from value to the buffer
	w.bitBuf |= (value & ((1 << uint(n)) - 1)) << uint(32-w.bitCnt-n)
	w.bitCnt += n
	for w.bitCnt >= 8 {
		w.buf = append(w.buf, byte(w.bitBuf>>24))
		w.bitBuf <<= 8
		w.bitCnt -= 8
	}
}

func (w *testBitWriter) flush() {
	if w.bitCnt > 0 {
		w.buf = append(w.buf, byte(w.bitBuf>>24))
		w.bitBuf = 0
		w.bitCnt = 0
	}
}

func TestDecompressLH5_Identity(t *testing.T) {
	// Compress and decompress known data
	original := []byte("Hello, World! This is a test of LH5 decompression.")
	compressed := testCompressLH5(original)

	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch:\n  got:  %q\n  want: %q", result, original)
	}
}

func TestDecompressLH5_AllZeros(t *testing.T) {
	original := make([]byte, 256)
	compressed := testCompressLH5(original)

	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch (all zeros)")
	}
}

func TestDecompressLH5_AllBytes(t *testing.T) {
	// Test with all 256 byte values
	original := make([]byte, 256)
	for i := range 256 {
		original[i] = byte(i)
	}
	compressed := testCompressLH5(original)

	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch (all bytes)")
	}
}

func TestDecompressLH5_SingleByte(t *testing.T) {
	original := []byte{0x42}
	compressed := testCompressLH5(original)

	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch: got %v, want %v", result, original)
	}
}

func TestDecompressLH5_EmptyInput(t *testing.T) {
	_, err := decompressLH5(nil, 100)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestDecompressLH5_ZeroOrigSize(t *testing.T) {
	_, err := decompressLH5([]byte{0x00}, 0)
	if err == nil {
		t.Error("expected error for zero original size")
	}
}

func TestDecompressLH5_YMRegisterData(t *testing.T) {
	// Simulate 3 frames of 14 AY registers (interleaved like YM3)
	// In YM3 interleaved format: all values for reg 0, then reg 1, etc.
	frameCount := 3
	regCount := 14
	original := make([]byte, frameCount*regCount)
	for reg := range regCount {
		for frame := range frameCount {
			original[reg*frameCount+frame] = byte(reg*16 + frame)
		}
	}

	compressed := testCompressLH5(original)
	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed YM register data mismatch")
	}
}

func TestDecompressLH_LH5_Identity(t *testing.T) {
	original := []byte("Hello, World! This is a test of parameterized LH decompression.")
	compressed := testCompressLH5(original)

	result, err := decompressLH(compressed, len(original), 13)
	if err != nil {
		t.Fatalf("decompressLH(dicBit=13) error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestDecompressLH_LH4(t *testing.T) {
	original := []byte("LH4 test data with 4KB window (dicBit=12)")
	compressed := testCompressLH5(original) // all-literal encoding works regardless of window size

	result, err := decompressLH(compressed, len(original), 12)
	if err != nil {
		t.Fatalf("decompressLH(dicBit=12) error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestDecompressLH_LH6(t *testing.T) {
	original := []byte("LH6 test data with 32KB window (dicBit=15)")
	compressed := testCompressLH5(original)

	result, err := decompressLH(compressed, len(original), 15)
	if err != nil {
		t.Fatalf("decompressLH(dicBit=15) error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestDecompressLH_LH7(t *testing.T) {
	original := []byte("LH7 test data with 64KB window (dicBit=16)")
	compressed := testCompressLH5(original)

	result, err := decompressLH(compressed, len(original), 16)
	if err != nil {
		t.Fatalf("decompressLH(dicBit=16) error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestDecompressLH_InvalidDicBit(t *testing.T) {
	data := testCompressLH5([]byte("test"))
	if _, err := decompressLH(data, 4, 0); err == nil {
		t.Error("expected error for dicBit=0")
	}
	if _, err := decompressLH(data, 4, 17); err == nil {
		t.Error("expected error for dicBit=17")
	}
}

func TestDecompressLH5_StillWorks(t *testing.T) {
	original := make([]byte, 1024)
	for i := range original {
		original[i] = byte(i % 251) // prime modulus for variety
	}
	compressed := testCompressLH5(original)

	result, err := decompressLH5(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH5 wrapper error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressLH5 wrapper data mismatch")
	}
}
