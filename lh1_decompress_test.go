package main

import (
	"bytes"
	"testing"
)

// lh1Encoder implements an LH1 compressor for round-trip testing.
// Uses the same adaptive Huffman tree as the decoder.
type lh1Encoder struct {
	buf    []byte
	putBuf uint16
	putLen uint8

	freq [lh1T + 1]uint32
	prnt [lh1T + lh1NChar]int32
	son  [lh1T]int32

	pCode [64]uint8
	pLen  [64]uint8
}

func newLH1Encoder() *lh1Encoder {
	e := &lh1Encoder{}
	e.startHuff()
	e.initPositionTables()
	return e
}

func (e *lh1Encoder) startHuff() {
	for i := range lh1NChar {
		e.freq[i] = 1
		e.son[i] = int32(i + lh1T)
		e.prnt[i+lh1T] = int32(i)
	}
	i := 0
	j := lh1NChar
	for j <= lh1R {
		e.freq[j] = e.freq[i] + e.freq[i+1]
		e.son[j] = int32(i)
		e.prnt[i] = int32(j)
		e.prnt[i+1] = int32(j)
		i += 2
		j++
	}
	e.freq[lh1T] = 0xFFFF
	e.prnt[lh1R] = 0
}

func (e *lh1Encoder) initPositionTables() {
	for i := range 256 {
		code := lh1DCode[i]
		bits := lh1DLen[i]
		if e.pLen[code] == 0 {
			e.pCode[code] = uint8(i >> (8 - bits))
			e.pLen[code] = bits
		}
	}
}

func (e *lh1Encoder) putBit(bit int) {
	e.putBuf |= uint16(bit&1) << (15 - e.putLen)
	e.putLen++
	if e.putLen >= 8 {
		e.buf = append(e.buf, byte(e.putBuf>>8))
		e.putBuf <<= 8
		e.putLen -= 8
	}
}

func (e *lh1Encoder) flush() {
	if e.putLen > 0 {
		e.buf = append(e.buf, byte(e.putBuf>>8))
		e.putBuf = 0
		e.putLen = 0
	}
}

func (e *lh1Encoder) encodeChar(c int) {
	// Walk from leaf to root collecting bits (same as original LZHUF EncodeChar)
	var code uint32
	nbits := 0
	k := int(e.prnt[c+lh1T])
	for {
		code >>= 1
		if k&1 != 0 {
			code |= 0x80000000
		}
		nbits++
		k = int(e.prnt[k])
		if k == lh1R {
			break
		}
	}
	// Output nbits from MSB of code
	for i := range nbits {
		e.putBit(int((code >> uint(31-i)) & 1))
	}
	e.update(c)
}

func (e *lh1Encoder) encodePosition(pos int) {
	upper := pos >> 6
	// Output upper 6 bits via static Huffman table
	bits := int(e.pLen[upper])
	code := e.pCode[upper]
	for i := bits - 1; i >= 0; i-- {
		e.putBit(int((code >> uint(i)) & 1))
	}
	// Output lower 6 bits verbatim
	for i := 5; i >= 0; i-- {
		e.putBit((pos >> uint(i)) & 1)
	}
}

func (e *lh1Encoder) reconst() {
	j := 0
	for i := range lh1T {
		if e.son[i] >= lh1T {
			e.freq[j] = (e.freq[i] + 1) / 2
			e.son[j] = e.son[i]
			j++
		}
	}
	i := 0
	for jj := lh1NChar; jj < lh1T; jj++ {
		f := e.freq[i] + e.freq[i+1]
		e.freq[jj] = f
		k := jj - 1
		for f < e.freq[k] {
			k--
		}
		k++
		copy(e.freq[k+1:jj+1], e.freq[k:jj])
		e.freq[k] = f
		copy(e.son[k+1:jj+1], e.son[k:jj])
		e.son[k] = int32(i)
		i += 2
	}
	for i := range lh1T {
		k := e.son[i]
		if k >= lh1T {
			e.prnt[k] = int32(i)
		} else {
			e.prnt[k] = int32(i)
			e.prnt[k+1] = int32(i)
		}
	}
}

func (e *lh1Encoder) update(c int) {
	if e.freq[lh1R] == lh1MaxFreq {
		e.reconst()
	}
	c = int(e.prnt[c+lh1T])
	for c != 0 {
		e.freq[c]++
		k := e.freq[c]
		l := c + 1
		if k > e.freq[l] {
			for k > e.freq[l+1] {
				l++
			}
			e.freq[c] = e.freq[l]
			e.freq[l] = k
			i := e.son[c]
			e.prnt[i] = int32(l)
			if i < lh1T {
				e.prnt[i+1] = int32(l)
			}
			jj := e.son[l]
			e.son[l] = i
			e.prnt[jj] = int32(c)
			if jj < lh1T {
				e.prnt[jj+1] = int32(c)
			}
			e.son[c] = jj
			c = l
		}
		c = int(e.prnt[c])
	}
}

// testCompressLH1 compresses data using the LH1 algorithm with greedy matching.
func testCompressLH1(data []byte) []byte {
	e := newLH1Encoder()

	var textBuf [lh1N]byte
	for i := range lh1N - lh1F {
		textBuf[i] = 0x20
	}
	r := lh1N - lh1F
	pos := 0

	for pos < len(data) {
		bestLen := 0
		bestDist := 0

		maxMatch := lh1F
		if pos+maxMatch > len(data) {
			maxMatch = len(data) - pos
		}

		// Search backward in window for longest match
		maxDist := lh1N - lh1F
		if pos < maxDist {
			maxDist = pos
		}

		for dist := 1; dist <= maxDist; dist++ {
			matchLen := 0
			for matchLen < maxMatch {
				// Look up from the circular buffer to handle window correctly
				bufPos := (r - dist + lh1N) & (lh1N - 1)
				curPos := (bufPos + matchLen) & (lh1N - 1)
				if textBuf[curPos] != data[pos+matchLen] {
					break
				}
				matchLen++
			}
			if matchLen > bestLen {
				bestLen = matchLen
				bestDist = dist
			}
			if bestLen >= lh1F {
				break
			}
		}

		if bestLen >= lh1Threshold+1 {
			// Encode match: symbol = 255 - THRESHOLD + matchLen
			sym := 255 - lh1Threshold + bestLen
			e.encodeChar(sym)
			// Position in LHA format is the distance - 1
			e.encodePosition(bestDist - 1)
			for i := range bestLen {
				textBuf[r] = data[pos+i]
				r = (r + 1) & (lh1N - 1)
			}
			pos += bestLen
		} else {
			e.encodeChar(int(data[pos]))
			textBuf[r] = data[pos]
			r = (r + 1) & (lh1N - 1)
			pos++
		}
	}

	e.flush()
	return e.buf
}

func TestDecompressLH1_TruncatedStreamRejected(t *testing.T) {
	compressed := testCompressLH1([]byte("truncated lh1 data"))
	if len(compressed) < 2 {
		t.Fatal("test compressor produced too little data")
	}
	if _, err := decompressLH1(compressed[:len(compressed)-1], len("truncated lh1 data")); err == nil {
		t.Fatal("expected error for truncated stream")
	}
}

func TestLH1UpdateGuardsFrequencyWalk(t *testing.T) {
	d := &lh1Decoder{}
	d.startHuff()
	d.freq[lh1T] = 0
	d.freq[lh1R] = 1
	d.prnt[lh1T] = int32(lh1R)
	d.update(0)
}

// testCompressLH1LiteralsOnly compresses using only literals (no backreferences).
func testCompressLH1LiteralsOnly(data []byte) []byte {
	e := newLH1Encoder()
	for _, b := range data {
		e.encodeChar(int(b))
	}
	e.flush()
	return e.buf
}

func TestDecompressLH1_SmallInput(t *testing.T) {
	original := []byte("Hello, World!")
	compressed := testCompressLH1LiteralsOnly(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("mismatch:\n  got:  %q\n  want: %q", result, original)
	}
}

func TestDecompressLH1_AllZeros(t *testing.T) {
	original := make([]byte, 256)
	compressed := testCompressLH1(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("decompressed data mismatch (all zeros)")
	}
}

func TestDecompressLH1_EmptyInput(t *testing.T) {
	_, err := decompressLH1(nil, 100)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestDecompressLH1_ZeroOrigSize(t *testing.T) {
	_, err := decompressLH1([]byte{0x00}, 0)
	if err == nil {
		t.Error("expected error for zero original size")
	}
}

func TestDecompressLH1_Backreference(t *testing.T) {
	original := bytes.Repeat([]byte("ABCDEFGH"), 32)
	compressed := testCompressLH1(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("backreference data mismatch")
	}
}

func TestDecompressLH1_WindowWraparound(t *testing.T) {
	original := make([]byte, 5000)
	for i := range original {
		original[i] = byte(i % 37)
	}
	compressed := testCompressLH1(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("window wraparound data mismatch")
	}
}

func TestDecompressLH1_MixedSymbols(t *testing.T) {
	var original []byte
	for i := range 50 {
		original = append(original, byte(i*5+1), byte(i*5+2), byte(i*5+3))
		original = append(original, bytes.Repeat([]byte{byte(i)}, 10)...)
	}
	compressed := testCompressLH1(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("mixed symbols data mismatch")
	}
}

func TestDecompressLH1_LargeData(t *testing.T) {
	original := make([]byte, 32768)
	for i := range original {
		original[i] = byte((i*7 + i/256) & 0xFF)
	}
	compressed := testCompressLH1(original)

	result, err := decompressLH1(compressed, len(original))
	if err != nil {
		t.Fatalf("decompressLH1 error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("large data mismatch")
	}
}
