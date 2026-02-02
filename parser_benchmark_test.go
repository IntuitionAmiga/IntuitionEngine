// parser_benchmark_test.go - Benchmarks for music file parsers

package main

import (
	"testing"
)

// BenchmarkParseAY benchmarks raw AY frame parsing
func BenchmarkParseAY(b *testing.B) {
	// Create 100 frames of AY data (100 × 16 bytes)
	data := make([]byte, 100*PSG_REG_COUNT)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseAYData(data)
		if err != nil {
			b.Fatalf("ParseAYData failed: %v", err)
		}
	}
}

// BenchmarkParseAY_Large benchmarks AY parsing with large frame count
func BenchmarkParseAY_Large(b *testing.B) {
	// Create 10000 frames of AY data
	data := make([]byte, 10000*PSG_REG_COUNT)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseAYData(data)
		if err != nil {
			b.Fatalf("ParseAYData failed: %v", err)
		}
	}
}

// BenchmarkParseYM benchmarks YM file parsing
func BenchmarkParseYM(b *testing.B) {
	data := createTestYMData(1000) // 1000 frames

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseYMData(data)
		if err != nil {
			b.Fatalf("parseYMData failed: %v", err)
		}
	}
}

// BenchmarkParseYM_Interleaved benchmarks YM interleaved format parsing
func BenchmarkParseYM_Interleaved(b *testing.B) {
	data := createTestYMDataInterleaved(1000) // 1000 frames

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseYMData(data)
		if err != nil {
			b.Fatalf("parseYMData failed: %v", err)
		}
	}
}

// BenchmarkParseVGM benchmarks VGM file parsing
func BenchmarkParseVGM(b *testing.B) {
	data := createTestVGMData(1000) // 1000 events

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseVGMData(data)
		if err != nil {
			b.Fatalf("ParseVGMData failed: %v", err)
		}
	}
}

// BenchmarkParseSAP benchmarks SAP file parsing
func BenchmarkParseSAP(b *testing.B) {
	data := createTestSAPData()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSAPData(data)
		if err != nil {
			b.Fatalf("ParseSAPData failed: %v", err)
		}
	}
}

// BenchmarkParseSAP_LargeHeader benchmarks SAP with many header lines
func BenchmarkParseSAP_LargeHeader(b *testing.B) {
	data := createTestSAPDataLargeHeader()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSAPData(data)
		if err != nil {
			b.Fatalf("ParseSAPData failed: %v", err)
		}
	}
}

// BenchmarkParseSNDH benchmarks SNDH file parsing
func BenchmarkParseSNDH(b *testing.B) {
	data := createTestSNDHData()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSNDHData(data)
		if err != nil {
			b.Fatalf("ParseSNDHData failed: %v", err)
		}
	}
}

// BenchmarkParseTED benchmarks TED file parsing
func BenchmarkParseTED(b *testing.B) {
	data := createTestTEDData()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseTEDFile(data)
		if err != nil {
			b.Fatalf("parseTEDFile failed: %v", err)
		}
	}
}

// BenchmarkParseSID benchmarks SID file parsing
func BenchmarkParseSID(b *testing.B) {
	data := createTestSIDData()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSIDData(data)
		if err != nil {
			b.Fatalf("ParseSIDData failed: %v", err)
		}
	}
}

// BenchmarkParseAYZ80 benchmarks ZXAYEMUL format parsing
func BenchmarkParseAYZ80(b *testing.B) {
	data := createTestAYZ80Data()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseAYZ80Data(data)
		if err != nil {
			b.Fatalf("ParseAYZ80Data failed: %v", err)
		}
	}
}

// Helper functions to create test data

func createTestYMData(frames int) []byte {
	// YM5! header + frame data (non-interleaved)
	headerSize := 34 // 4+8+4+4+2+4+2+4+2 + strings
	dataSize := frames * 16
	data := make([]byte, headerSize+dataSize+20) // +20 for strings

	copy(data[0:4], "YM5!")
	copy(data[4:12], "LeOnArD!")

	// Number of frames (big-endian)
	data[12] = byte(frames >> 24)
	data[13] = byte(frames >> 16)
	data[14] = byte(frames >> 8)
	data[15] = byte(frames)

	// Song attributes (non-interleaved = 0)
	data[16] = 0
	data[17] = 0
	data[18] = 0
	data[19] = 0

	// Num drums = 0
	data[20] = 0
	data[21] = 0

	// Clock (2MHz)
	data[22] = 0
	data[23] = 0x1E
	data[24] = 0x84
	data[25] = 0x80

	// Frame rate (50Hz)
	data[26] = 0
	data[27] = 50

	// Loop frame
	data[28] = 0
	data[29] = 0
	data[30] = 0
	data[31] = 0

	// Additional data size = 0
	data[32] = 0
	data[33] = 0

	// Strings (title, author, comments - null terminated)
	pos := 34
	data[pos] = 0 // title
	pos++
	data[pos] = 0 // author
	pos++
	data[pos] = 0 // comments
	pos++

	// Frame data
	for i := 0; i < frames*16; i++ {
		data[pos+i] = byte(i & 0xFF)
	}

	return data[:pos+frames*16]
}

func createTestYMDataInterleaved(frames int) []byte {
	data := createTestYMData(frames)
	// Set interleaved bit
	data[16] = 0
	data[17] = 0
	data[18] = 0
	data[19] = 1 // Interleaved flag
	return data
}

func createTestVGMData(events int) []byte {
	// VGM header (0x40 bytes minimum) + event data
	headerSize := 0x80
	// Each AY event is 3 bytes (0xA0, reg, val)
	// Plus some wait commands
	dataSize := events * 4 // Rough estimate
	data := make([]byte, headerSize+dataSize+10)

	copy(data[0:4], "Vgm ")

	// Version 1.50
	data[0x08] = 0x50
	data[0x09] = 0x01
	data[0x0A] = 0x00
	data[0x0B] = 0x00

	// Total samples
	samples := uint32(events * 735)
	data[0x18] = byte(samples)
	data[0x19] = byte(samples >> 8)
	data[0x1A] = byte(samples >> 16)
	data[0x1B] = byte(samples >> 24)

	// Data offset (relative to 0x34)
	offset := uint32(0x80 - 0x34)
	data[0x34] = byte(offset)
	data[0x35] = byte(offset >> 8)
	data[0x36] = byte(offset >> 16)
	data[0x37] = byte(offset >> 24)

	// AY clock at 0x74
	clock := uint32(1789773)
	data[0x74] = byte(clock)
	data[0x75] = byte(clock >> 8)
	data[0x76] = byte(clock >> 16)
	data[0x77] = byte(clock >> 24)

	// Event data
	pos := 0x80
	for i := 0; i < events; i++ {
		// AY write
		data[pos] = 0xA0
		data[pos+1] = byte(i % 14) // Register
		data[pos+2] = byte(i)      // Value
		pos += 3

		// Wait 735 samples (NTSC frame)
		if i%10 == 9 {
			data[pos] = 0x62 // Wait 735
			pos++
		}
	}

	// End marker
	data[pos] = 0x66
	pos++

	return data[:pos]
}

func createTestSAPData() []byte {
	header := `SAP
AUTHOR "Test Author"
NAME "Test Song"
DATE "2024"
SONGS 1
TYPE B
INIT 2000
PLAYER 2100
`
	// Binary section
	binary := []byte{0xFF, 0xFF}
	// Data block: start=$2000, end=$21FF
	binary = append(binary, 0x00, 0x20, 0xFF, 0x21)
	// Fill with dummy code
	for i := 0; i < 0x200; i++ {
		binary = append(binary, byte(i&0xFF))
	}

	return append([]byte(header), binary...)
}

func createTestSAPDataLargeHeader() []byte {
	header := "SAP\r\n"
	header += `AUTHOR "Test Author With A Very Long Name Here"` + "\r\n"
	header += `NAME "Test Song With An Extremely Long Title That Goes On"` + "\r\n"
	header += `DATE "2024/01/15"` + "\r\n"
	header += "SONGS 10\r\n"
	header += "DEFSONG 0\r\n"
	header += "TYPE B\r\n"
	header += "STEREO\r\n"
	header += "INIT 2000\r\n"
	header += "PLAYER 2100\r\n"
	header += "FASTPLAY 312\r\n"
	// Add multiple TIME entries
	for i := 0; i < 10; i++ {
		header += "TIME 03:30.500\r\n"
	}

	binary := []byte{0xFF, 0xFF}
	binary = append(binary, 0x00, 0x20, 0xFF, 0x21)
	for i := 0; i < 0x200; i++ {
		binary = append(binary, byte(i&0xFF))
	}

	return append([]byte(header), binary...)
}

func createTestSNDHData() []byte {
	// SNDH format: BRA instructions + SNDH magic + tags + HDNS + code
	data := make([]byte, 512)

	// BRA.W to INIT at offset 0
	data[0] = 0x60
	data[1] = 0x00
	data[2] = 0x00
	data[3] = 0x80 // Branch to offset 0x82

	// BRA.W to EXIT at offset 4
	data[4] = 0x60
	data[5] = 0x00
	data[6] = 0x00
	data[7] = 0x90 // Branch to offset 0x94

	// BRA.W to PLAY at offset 8
	data[8] = 0x60
	data[9] = 0x00
	data[10] = 0x00
	data[11] = 0xA0 // Branch to offset 0xAC

	// SNDH magic at offset 12
	copy(data[12:16], "SNDH")

	// Tags starting at offset 16
	pos := 16
	copy(data[pos:pos+4], "TITL")
	pos += 4
	copy(data[pos:], "Test Song")
	pos += 10 // includes null

	copy(data[pos:pos+4], "COMM")
	pos += 4
	copy(data[pos:], "Test Composer")
	pos += 14

	copy(data[pos:pos+4], "YEAR")
	pos += 4
	copy(data[pos:], "2024")
	pos += 5

	copy(data[pos:pos+4], "##01") // 1 subsong
	pos += 4

	copy(data[pos:pos+4], "TC50") // Timer C 50Hz
	pos += 4

	// Padding to align
	for pos%2 != 0 {
		pos++
	}

	copy(data[pos:pos+4], "HDNS")
	pos += 4

	return data[:pos+100] // Include some "code" space
}

func createTestTEDData() []byte {
	// TED/TMF format with TEDMUSIC header
	data := make([]byte, 256)

	// Load address (little-endian): $1001 (Plus/4 BASIC)
	data[0] = 0x01
	data[1] = 0x10

	// BASIC stub (fake line number)
	data[2] = 0x00
	data[3] = 0x10 // Line < 4096

	// Skip to signature offset
	// TEDMUSIC at offset 17
	copy(data[17:25], "TEDMUSIC")
	data[25] = 0 // Null terminator

	// Init offset
	data[26] = 0x80
	data[27] = 0x10 // Init at $1080

	// Play address
	data[28] = 0x90
	data[29] = 0x10 // Play at $1090

	// End address
	data[30] = 0xFF
	data[31] = 0x1F

	// Reserved
	data[32] = 0
	data[33] = 0

	// Subtunes
	data[34] = 1
	data[35] = 0

	// FileFlags
	data[36] = 0

	// Strings at offset 48 (relative to signature)
	stringsStart := 17 + 48
	// Title (32 bytes)
	copy(data[stringsStart:], "Test TED Tune")
	// Author (32 bytes)
	copy(data[stringsStart+32:], "Test Author")
	// Date (32 bytes)
	copy(data[stringsStart+64:], "2024")
	// Tool (32 bytes)
	copy(data[stringsStart+96:], "TMF Tool")

	return data
}

func createTestSIDData() []byte {
	// PSID v2 header (0x7C bytes)
	data := make([]byte, 0x7C+256) // Header + some program data

	copy(data[0:4], "PSID")

	// Version = 2
	data[0x04] = 0x00
	data[0x05] = 0x02

	// Data offset = 0x7C
	data[0x06] = 0x00
	data[0x07] = 0x7C

	// Load address = 0 (use embedded)
	data[0x08] = 0x00
	data[0x09] = 0x00

	// Init address
	data[0x0A] = 0x10
	data[0x0B] = 0x00

	// Play address
	data[0x0C] = 0x10
	data[0x0D] = 0x03

	// Songs = 1
	data[0x0E] = 0x00
	data[0x0F] = 0x01

	// Start song = 1
	data[0x10] = 0x00
	data[0x11] = 0x01

	// Speed
	data[0x12] = 0x00
	data[0x13] = 0x00
	data[0x14] = 0x00
	data[0x15] = 0x00

	// Name (32 bytes at 0x16)
	copy(data[0x16:0x36], "Test SID Tune")

	// Author (32 bytes at 0x36)
	copy(data[0x36:0x56], "Test Author")

	// Released (32 bytes at 0x56)
	copy(data[0x56:0x76], "2024 Test Release")

	// Flags at 0x76
	data[0x76] = 0x00
	data[0x77] = 0x00

	// Embedded load address at data start
	data[0x7C] = 0x00
	data[0x7D] = 0x10 // Load at $1000

	// Fill with dummy code
	for i := 0x7E; i < len(data); i++ {
		data[i] = byte(i & 0xFF)
	}

	return data
}

func createTestAYZ80Data() []byte {
	// ZXAYEMUL format
	data := make([]byte, 350)

	copy(data[0:8], "ZXAYEMUL")

	// File version
	data[8] = 0x00
	data[9] = 0x00

	// Player version
	data[10] = 0x00

	// Special player
	data[11] = 0x00

	// Author pointer (relative from offset 12)
	// Points to offset 50
	relAuthor := int16(50 - 12)
	data[12] = byte(relAuthor >> 8)
	data[13] = byte(relAuthor)

	// Misc pointer (relative from offset 14)
	// Points to offset 70
	relMisc := int16(70 - 14)
	data[14] = byte(relMisc >> 8)
	data[15] = byte(relMisc)

	// Song count - 1
	data[16] = 0 // 1 song

	// First song
	data[17] = 0

	// Songs pointer (relative from offset 18)
	// Points to offset 100
	relSongs := int16(100 - 18)
	data[18] = byte(relSongs >> 8)
	data[19] = byte(relSongs)

	// Author string at offset 50
	copy(data[50:], "Test Author")
	data[61] = 0

	// Misc string at offset 70
	copy(data[70:], "Test Misc")
	data[79] = 0

	// Song table at offset 100
	// Name pointer (relative from 100)
	relName := int16(120 - 100)
	data[100] = byte(relName >> 8)
	data[101] = byte(relName)

	// Data pointer (relative from 102)
	relData := int16(140 - 102)
	data[102] = byte(relData >> 8)
	data[103] = byte(relData)

	// Song name at offset 120
	copy(data[120:], "Test Song")
	data[129] = 0

	// Song data at offset 140
	// Channel map (4 bytes)
	data[140] = 0
	data[141] = 1
	data[142] = 2
	data[143] = 0

	// Length frames
	data[144] = 0x00
	data[145] = 0x64 // 100 frames

	// Fade frames
	data[146] = 0x00
	data[147] = 0x10

	// Hi/Lo reg
	data[148] = 0xFF
	data[149] = 0xFE

	// Points pointer (relative from 150)
	relPoints := int16(160 - 150)
	data[150] = byte(relPoints >> 8)
	data[151] = byte(relPoints)

	// Blocks pointer (relative from 152)
	relBlocks := int16(170 - 152)
	data[152] = byte(relBlocks >> 8)
	data[153] = byte(relBlocks)

	// Points at offset 160
	// Stack
	data[160] = 0xFF
	data[161] = 0xFF
	// Init
	data[162] = 0x80
	data[163] = 0x00
	// Interrupt
	data[164] = 0x80
	data[165] = 0x38

	// Blocks at offset 170
	// Block 1: addr=0x8000, length=100
	data[170] = 0x80
	data[171] = 0x00
	data[172] = 0x00
	data[173] = 0x64

	// Block data pointer (relative from 174)
	relBlockData := int16(190 - 174)
	data[174] = byte(relBlockData >> 8)
	data[175] = byte(relBlockData)

	// End of blocks
	data[176] = 0x00
	data[177] = 0x00

	// Block data at offset 190
	for i := 0; i < 100; i++ {
		data[190+i] = byte(i)
	}

	return data
}
