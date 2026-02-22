package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// buildLHALevel0 constructs a valid Level 0 LHA archive header.
func buildLHALevel0(method string, compressed, original []byte) []byte {
	// Level 0 header layout:
	// [0]    header size (excluding first 2 bytes)
	// [1]    checksum
	// [2:7]  method (5 bytes)
	// [7:11] compressed size (LE uint32)
	// [11:15] original size (LE uint32)
	// [15:19] timestamp
	// [19]   file attribute
	// [20]   level = 0
	// [21]   filename length
	// [22:]  filename
	// [...:+2] CRC16
	// [headerSize+2:] compressed data

	filename := []byte("test.bin")
	headerBody := make([]byte, 0, 64)

	// [1] placeholder checksum
	headerBody = append(headerBody, 0x00)
	// [2:7] method
	headerBody = append(headerBody, []byte(method)...)
	// [7:11] compressed size
	headerBody = binary.LittleEndian.AppendUint32(headerBody, uint32(len(compressed)))
	// [11:15] original size
	headerBody = binary.LittleEndian.AppendUint32(headerBody, uint32(len(original)))
	// [15:19] timestamp
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 0)
	// [19] file attribute
	headerBody = append(headerBody, 0x20)
	// [20] level
	headerBody = append(headerBody, 0x00)
	// [21] filename length
	headerBody = append(headerBody, byte(len(filename)))
	// [22:] filename
	headerBody = append(headerBody, filename...)
	// CRC16 (dummy)
	headerBody = append(headerBody, 0x00, 0x00)

	// data[0] counts bytes from offset 2 to end of header (excludes byte 0 and checksum byte 1)
	headerLen := byte(len(headerBody) - 1)

	// Compute checksum over bytes from offset 2 onward (headerBody[1:])
	var sum byte
	for _, b := range headerBody[1:] {
		sum += b
	}
	headerBody[0] = sum

	out := make([]byte, 0, 1+len(headerBody)+len(compressed))
	out = append(out, headerLen)
	out = append(out, headerBody...)
	out = append(out, compressed...)
	return out
}

// buildLHALevel1 constructs a valid Level 1 LHA archive header with one dummy extended header.
func buildLHALevel1(method string, compressed, original []byte) []byte {
	filename := []byte("test.bin")

	// Base header (before extended headers)
	headerBody := make([]byte, 0, 64)

	// [1] checksum placeholder
	headerBody = append(headerBody, 0x00)
	// [2:7] method
	headerBody = append(headerBody, []byte(method)...)
	// [7:11] compressed size — includes extended headers in Level 1
	// We'll add one 7-byte extended header + terminator
	extHeaderSize := 7
	totalCompressed := len(compressed) + extHeaderSize + 2 // +2 for terminator
	headerBody = binary.LittleEndian.AppendUint32(headerBody, uint32(totalCompressed))
	// [11:15] original size
	headerBody = binary.LittleEndian.AppendUint32(headerBody, uint32(len(original)))
	// [15:19] timestamp
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 0)
	// [19] reserved
	headerBody = append(headerBody, 0x20)
	// [20] level = 1
	headerBody = append(headerBody, 0x01)
	// [21] filename length
	headerBody = append(headerBody, byte(len(filename)))
	// [22:] filename
	headerBody = append(headerBody, filename...)
	// CRC16 (dummy)
	headerBody = append(headerBody, 0x00, 0x00)
	// OS type
	headerBody = append(headerBody, 'U')

	headerLen := byte(len(headerBody) - 1)

	// Compute checksum over bytes from offset 2 onward
	var sum byte
	for _, b := range headerBody[1:] {
		sum += b
	}
	headerBody[0] = sum

	out := make([]byte, 0, 1+len(headerBody)+extHeaderSize+2+len(compressed))
	out = append(out, headerLen)
	out = append(out, headerBody...)

	// Extended header: 7 bytes (type 0x01 = filename, "data" padding)
	extHdr := make([]byte, 7)
	binary.LittleEndian.PutUint16(extHdr[0:2], 0) // next-size=0 after this one... wait, this IS the ext header
	// Actually: extended headers start right after base header
	// Format: [2-byte next-size] [type] [data...]
	// First ext header:
	extData := []byte{0x01, 't', 'e', 's', 't'} // type 0x01 + 4 bytes data
	extSize := len(extData) + 2                 // 7 total
	binary.LittleEndian.PutUint16(extHdr[0:2], uint16(extSize))
	copy(extHdr[2:], extData)
	out = append(out, extHdr...)

	// Terminator: next-size = 0
	out = append(out, 0x00, 0x00)

	// Compressed data
	out = append(out, compressed...)
	return out
}

// buildLHALevel2 constructs a valid Level 2 LHA archive header.
func buildLHALevel2(method string, compressed, original []byte) []byte {
	// Level 2 header layout:
	// [0:2]  total header size (LE uint16)
	// [2:7]  method
	// [7:11] compressed size
	// [11:15] original size
	// [15:19] timestamp
	// [19]   reserved
	// [20]   level = 2
	// [21:23] CRC16
	// [23]   OS type
	// [24:26] first ext header size (0 = none)
	// Total header ends at byte [totalHeaderSize]

	totalHeaderSize := 26
	hdr := make([]byte, totalHeaderSize)
	binary.LittleEndian.PutUint16(hdr[0:2], uint16(totalHeaderSize))
	copy(hdr[2:7], method)
	binary.LittleEndian.PutUint32(hdr[7:11], uint32(len(compressed)))
	binary.LittleEndian.PutUint32(hdr[11:15], uint32(len(original)))
	binary.LittleEndian.PutUint32(hdr[15:19], 0) // timestamp
	hdr[19] = 0x20                               // reserved
	hdr[20] = 0x02                               // level
	hdr[21] = 0x00                               // CRC lo
	hdr[22] = 0x00                               // CRC hi
	hdr[23] = 'U'                                // OS type
	binary.LittleEndian.PutUint16(hdr[24:26], 0) // no extended headers

	out := make([]byte, 0, len(hdr)+len(compressed))
	out = append(out, hdr...)
	out = append(out, compressed...)
	return out
}

func TestLHAExtract_Level0_LH5(t *testing.T) {
	original := []byte("Level 0 LH5 test data for archive parsing")
	compressed := testCompressLH5(original)
	archive := buildLHALevel0("-lh5-", compressed, original)

	result, err := DecompressLHAData(archive)
	if err != nil {
		t.Fatalf("DecompressLHAData error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("data mismatch")
	}
}

func TestLHAExtract_Level0_LH0(t *testing.T) {
	original := []byte("Uncompressed LH0 stored data")
	archive := buildLHALevel0("-lh0-", original, original)

	result, err := DecompressLHAData(archive)
	if err != nil {
		t.Fatalf("DecompressLHAData error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("data mismatch")
	}
}

func TestLHAExtract_Level1_LH5(t *testing.T) {
	original := []byte("Level 1 LH5 test with extended headers")
	compressed := testCompressLH5(original)
	archive := buildLHALevel1("-lh5-", compressed, original)

	result, err := DecompressLHAData(archive)
	if err != nil {
		t.Fatalf("DecompressLHAData error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("data mismatch")
	}
}

func TestLHAExtract_Level2_LH5(t *testing.T) {
	original := []byte("Level 2 LH5 archive test data")
	compressed := testCompressLH5(original)
	archive := buildLHALevel2("-lh5-", compressed, original)

	result, err := DecompressLHAData(archive)
	if err != nil {
		t.Fatalf("DecompressLHAData error: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Errorf("data mismatch")
	}
}

func TestLHAExtract_TruncatedHeader(t *testing.T) {
	_, err := DecompressLHAData([]byte{0x10, 0x00})
	if err == nil {
		t.Error("expected error for truncated header")
	}
}

func TestLHAExtract_TruncatedData(t *testing.T) {
	original := []byte("test data that will be truncated")
	compressed := testCompressLH5(original)
	archive := buildLHALevel0("-lh5-", compressed, original)
	// Truncate compressed data
	archive = archive[:len(archive)-10]

	_, err := DecompressLHAData(archive)
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestLHAExtract_UnsupportedMethod(t *testing.T) {
	archive := buildLHALevel0("-lh2-", []byte{0x00}, []byte{0x00})
	_, err := DecompressLHAData(archive)
	if err == nil {
		t.Error("expected error for unsupported method")
	}
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte("-lh2-")) {
		t.Errorf("error should mention method: %v", err)
	}
}

func TestLHAExtract_Level1_MalformedExtHeaders(t *testing.T) {
	// Build a level 1 archive where ext headers total exceeds compressed size
	filename := []byte("x")
	headerBody := make([]byte, 0, 32)
	headerBody = append(headerBody, 0x00) // checksum
	headerBody = append(headerBody, []byte("-lh5-")...)
	// Compressed size = 10 (but ext headers will claim 20 + 2 = 22 bytes total)
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 10)
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 4) // original size
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 0) // timestamp
	headerBody = append(headerBody, 0x20)                        // attr
	headerBody = append(headerBody, 0x01)                        // level 1
	headerBody = append(headerBody, byte(len(filename)))
	headerBody = append(headerBody, filename...)
	headerBody = append(headerBody, 0x00, 0x00) // CRC
	headerBody = append(headerBody, 'U')        // OS

	headerLen := byte(len(headerBody) - 1)

	archive := make([]byte, 0, 64+22)
	archive = append(archive, headerLen)
	archive = append(archive, headerBody...)

	// Extended header: size=20 (consumes more than compressedSize=10)
	extHdr := make([]byte, 20)
	binary.LittleEndian.PutUint16(extHdr[0:2], 20)
	extHdr[2] = 0x01 // type
	archive = append(archive, extHdr...)

	// Terminator
	archive = append(archive, 0x00, 0x00)

	_, err := DecompressLHAData(archive)
	if err == nil {
		t.Error("expected error for malformed ext headers")
	}
}

func TestLHAExtract_Level1_OverlongChain(t *testing.T) {
	// Extended header chain that extends past data bounds
	filename := []byte("x")

	headerBody := make([]byte, 0, 32)
	headerBody = append(headerBody, 0x00)
	headerBody = append(headerBody, []byte("-lh5-")...)
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 100)
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 4)
	headerBody = binary.LittleEndian.AppendUint32(headerBody, 0)
	headerBody = append(headerBody, 0x20)
	headerBody = append(headerBody, 0x01)
	headerBody = append(headerBody, byte(len(filename)))
	headerBody = append(headerBody, filename...)
	headerBody = append(headerBody, 0x00, 0x00)
	headerBody = append(headerBody, 'U')

	archive := make([]byte, 0, 64)
	archive = append(archive, byte(len(headerBody)-1))
	archive = append(archive, headerBody...)
	// Extended header that claims 50 bytes but only 5 bytes remain
	sizeBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(sizeBytes, 50)
	archive = append(archive, sizeBytes...)
	archive = append(archive, 0x01, 0x02, 0x03)

	_, err := DecompressLHAData(archive)
	if err == nil {
		t.Error("expected error for overlong ext header chain")
	}
}

func TestLHAExtract_LH0_SizeMismatch(t *testing.T) {
	// Build -lh0- archive where compressed size != original size
	compressed := []byte("short")
	original := []byte("much longer original data")
	archive := buildLHALevel0("-lh0-", compressed, original)

	_, err := DecompressLHAData(archive)
	if err == nil {
		t.Error("expected error for lh0 size mismatch")
	}
}

func TestLHAExtract_PathologicalOrigSize(t *testing.T) {
	// Build header with original size > 64MB
	hdr := make([]byte, 30)
	hdr[0] = 26 // header body length
	hdr[1] = 0  // checksum
	copy(hdr[2:7], "-lh5-")
	binary.LittleEndian.PutUint32(hdr[7:11], 10)          // compressed
	binary.LittleEndian.PutUint32(hdr[11:15], 0x10000000) // 256MB original
	hdr[20] = 0x00                                        // level 0
	hdr[21] = 1                                           // filename len
	hdr[22] = 'x'                                         // filename
	hdr[23] = 0                                           // CRC lo
	hdr[24] = 0                                           // CRC hi

	_, err := DecompressLHAData(hdr)
	if err == nil {
		t.Error("expected error for pathological original size")
	}
}

func TestLHAExtract_RealYM(t *testing.T) {
	ymPath := "sdk/examples/assets/music/Pushover10.ym"
	data, err := os.ReadFile(ymPath)
	if err != nil {
		t.Skipf("YM file not found: %v", err)
	}

	result, err := DecompressLHAData(data)
	if err != nil {
		t.Fatalf("DecompressLHAData error: %v", err)
	}

	// YM files start with "YM" magic
	if len(result) < 4 {
		t.Fatalf("decompressed data too short: %d bytes", len(result))
	}
	magic := string(result[:4])
	if magic != "YM3!" && magic != "YM3b" && magic != "YM4!" && magic != "YM5!" && magic != "YM6!" {
		t.Errorf("unexpected magic: %q", magic)
	}

	// Verify it parses as valid YM data
	_, err = parseYMData(result)
	if err != nil {
		t.Errorf("parseYMData failed on decompressed data: %v", err)
	}
}

func TestDecompressLHAFile_RealYM(t *testing.T) {
	ymPath := "sdk/examples/assets/music/Pushover10.ym"
	if _, err := os.Stat(ymPath); err != nil {
		t.Skipf("YM file not found: %v", err)
	}

	result, err := DecompressLHAFile(ymPath)
	if err != nil {
		t.Fatalf("DecompressLHAFile error: %v", err)
	}
	if len(result) < 4 {
		t.Fatalf("decompressed data too short: %d bytes", len(result))
	}

	_, err = parseYMData(result)
	if err != nil {
		t.Errorf("parseYMData failed: %v", err)
	}
}
