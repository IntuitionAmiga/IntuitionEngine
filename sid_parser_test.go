package main

import (
	"encoding/binary"
	"testing"
)

// buildSIDHeader creates a minimal PSID/RSID v2 file.
func buildSIDHeader(magic string, version uint16, loadAddr, initAddr, playAddr, songs, startSong uint16, speed uint32, flags uint16) []byte {
	data := make([]byte, 0x7C+16) // header + 16 bytes of dummy program data
	copy(data[0x00:], magic)
	binary.BigEndian.PutUint16(data[0x04:], version)
	binary.BigEndian.PutUint16(data[0x06:], 0x7C) // data offset (v2)
	binary.BigEndian.PutUint16(data[0x08:], loadAddr)
	binary.BigEndian.PutUint16(data[0x0A:], initAddr)
	binary.BigEndian.PutUint16(data[0x0C:], playAddr)
	binary.BigEndian.PutUint16(data[0x0E:], songs)
	binary.BigEndian.PutUint16(data[0x10:], startSong)
	binary.BigEndian.PutUint32(data[0x12:], speed)
	copy(data[0x16:0x36], "Test Song\x00")
	copy(data[0x36:0x56], "Test Author\x00")
	copy(data[0x56:0x76], "2024\x00")
	binary.BigEndian.PutUint16(data[0x76:], flags)
	// startPage, pageLength at 0x78, 0x79
	// sid2addr, sid3addr at 0x7A-0x7D (zero = single SID)
	// dummy program data at 0x7C
	for i := 0x7C; i < len(data); i++ {
		data[i] = 0x60 // RTS opcodes
	}
	return data
}

func TestSIDParse_PSID_V1(t *testing.T) {
	// PSID v1: header size 0x76 (no flags/startPage/sid2addr)
	data := make([]byte, 0x76+16)
	copy(data[0x00:], "PSID")
	binary.BigEndian.PutUint16(data[0x04:], 1)      // version 1
	binary.BigEndian.PutUint16(data[0x06:], 0x76)   // data offset
	binary.BigEndian.PutUint16(data[0x08:], 0x1000) // loadAddress
	binary.BigEndian.PutUint16(data[0x0A:], 0x1000) // initAddress
	binary.BigEndian.PutUint16(data[0x0C:], 0x1003) // playAddress
	binary.BigEndian.PutUint16(data[0x0E:], 1)      // songs
	binary.BigEndian.PutUint16(data[0x10:], 1)      // startSong
	copy(data[0x16:0x36], "V1 Song\x00")
	copy(data[0x36:0x56], "V1 Author\x00")
	copy(data[0x56:0x76], "1990\x00")
	for i := 0x76; i < len(data); i++ {
		data[i] = 0x60
	}

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(PSID v1) failed: %v", err)
	}
	if sid.Header.Version != 1 {
		t.Errorf("expected version 1, got %d", sid.Header.Version)
	}
	if sid.Header.IsRSID {
		t.Error("expected IsRSID=false for PSID")
	}
	if sid.Header.Name != "V1 Song" {
		t.Errorf("expected name 'V1 Song', got %q", sid.Header.Name)
	}
}

func TestSIDParse_PSID_V2_Flags(t *testing.T) {
	// PSID v2: flags bits 2-3 = NTSC (0b10 << 2 = 0x08), bits 4-5 = 8580 (0b10 << 4 = 0x20)
	flags := uint16(0x08 | 0x20) // NTSC + 8580
	data := buildSIDHeader("PSID", 2, 0x1000, 0x1000, 0x1003, 1, 1, 0, flags)

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(PSID v2) failed: %v", err)
	}
	if sid.Header.Version != 2 {
		t.Errorf("expected version 2, got %d", sid.Header.Version)
	}
	if sid.Header.Flags != flags {
		t.Errorf("expected flags 0x%04X, got 0x%04X", flags, sid.Header.Flags)
	}
	// Verify clock extraction: bits 2-3
	clock := (sid.Header.Flags >> 2) & 0x03
	if clock != 2 { // NTSC
		t.Errorf("expected clock=2 (NTSC), got %d", clock)
	}
	// Verify SID model extraction: bits 4-5
	model := (sid.Header.Flags >> 4) & 0x03
	if model != 2 { // 8580
		t.Errorf("expected sidModel=2 (8580), got %d", model)
	}
}

func TestSIDParse_RSID_Accepted(t *testing.T) {
	// RSID files must now be accepted (not rejected).
	// RSID constraints: playAddress=0, speed=0
	data := buildSIDHeader("RSID", 2, 0, 0x1000, 0, 1, 1, 0, 0x04) // PAL

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(RSID) should succeed, got error: %v", err)
	}
	if !sid.Header.IsRSID {
		t.Error("expected IsRSID=true for RSID")
	}
	if sid.Header.PlayAddress != 0 {
		t.Errorf("RSID playAddress should be 0, got 0x%04X", sid.Header.PlayAddress)
	}
}

func TestSIDParse_RSID_EmbeddedLoadAddress(t *testing.T) {
	// RSID with loadAddress=0 means load address is embedded in first 2 bytes of data.
	data := buildSIDHeader("RSID", 2, 0, 0x1000, 0, 1, 1, 0, 0)
	// Embed load address 0x0801 (C64 BASIC start) at data offset
	data[0x7C] = 0x01
	data[0x7D] = 0x08

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(RSID embedded addr) failed: %v", err)
	}
	if sid.Header.LoadAddress != 0x0801 {
		t.Errorf("expected embedded load address 0x0801, got 0x%04X", sid.Header.LoadAddress)
	}
}

func TestSIDParse_CIATimerSpeed(t *testing.T) {
	// Speed bitmap: bit 0 set = CIA timer for subsong 0
	speed := uint32(0x00000001)
	data := buildSIDHeader("PSID", 2, 0x1000, 0x1000, 0x1003, 3, 1, speed, 0)

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(CIA speed) failed: %v", err)
	}
	if sid.Header.Speed != speed {
		t.Errorf("expected speed 0x%08X, got 0x%08X", speed, sid.Header.Speed)
	}
	// Bit 0 set = subsong 0 uses CIA timer
	if sid.Header.Speed&(1<<0) == 0 {
		t.Error("expected bit 0 set for CIA timer on subsong 0")
	}
	// Bit 1 unset = subsong 1 uses VBL
	if sid.Header.Speed&(1<<1) != 0 {
		t.Error("expected bit 1 unset for VBL on subsong 1")
	}
}

func TestSIDParse_MultiSIDRejected(t *testing.T) {
	// Multi-SID (v3/v4 with sid2addr != 0) should still be rejected.
	// Sid2Addr is read when DataOffset >= 0x80, stored at 0x7C-0x7E
	data := make([]byte, 0x80+16)
	copy(data[0x00:], "PSID")
	binary.BigEndian.PutUint16(data[0x04:], 3)      // version 3
	binary.BigEndian.PutUint16(data[0x06:], 0x80)   // data offset
	binary.BigEndian.PutUint16(data[0x08:], 0x1000) // loadAddress
	binary.BigEndian.PutUint16(data[0x0A:], 0x1000) // initAddress
	binary.BigEndian.PutUint16(data[0x0C:], 0x1003) // playAddress
	binary.BigEndian.PutUint16(data[0x0E:], 1)      // songs
	binary.BigEndian.PutUint16(data[0x10:], 1)      // startSong
	copy(data[0x16:0x36], "MultiSID\x00")
	copy(data[0x36:0x56], "Author\x00")
	copy(data[0x56:0x76], "2024\x00")
	binary.BigEndian.PutUint16(data[0x7C:], 0xD500) // sid2addr (non-zero = multi-SID)

	_, err := ParseSIDData(data)
	if err == nil {
		t.Fatal("expected error for multi-SID, got nil")
	}
}

func TestSIDIsNTSC(t *testing.T) {
	tests := []struct {
		name  string
		flags uint16
		want  bool
	}{
		{"unknown", 0x00, false},
		{"PAL", 0x04, false},      // bits 2-3 = 01
		{"NTSC", 0x08, true},      // bits 2-3 = 10
		{"PAL+NTSC", 0x0C, false}, // bits 2-3 = 11 (treat as PAL)
		{"NTSC+6581", 0x18, true}, // bits 2-3=10, bits 4-5=01
		{"PAL+8580", 0x24, false}, // bits 2-3=01, bits 4-5=10
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := SIDHeader{Flags: tt.flags}
			got := sidIsNTSC(h)
			if got != tt.want {
				t.Errorf("sidIsNTSC(flags=0x%04X) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}
