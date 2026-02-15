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

func TestSIDParse_MultiSIDAccepted(t *testing.T) {
	// Multi-SID (v3/v4 with sid2addr != 0) should now be accepted.
	// Playback uses single-SID (chip 0) with graceful degradation.
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
	binary.BigEndian.PutUint16(data[0x7C:], 0xD500) // sid2addr
	binary.BigEndian.PutUint16(data[0x7E:], 0xD600) // sid3addr
	for i := 0x80; i < len(data); i++ {
		data[i] = 0x60 // RTS
	}

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(multi-SID) should succeed, got error: %v", err)
	}
	if sid.Header.Sid2Addr != 0xD500 {
		t.Errorf("expected Sid2Addr=0xD500, got 0x%04X", sid.Header.Sid2Addr)
	}
	if sid.Header.Sid3Addr != 0xD600 {
		t.Errorf("expected Sid3Addr=0xD600, got 0x%04X", sid.Header.Sid3Addr)
	}
}

func TestSIDParse_MultiSID_DualSIDOnly(t *testing.T) {
	// Dual-SID (sid2addr set, sid3addr zero) should parse correctly.
	data := make([]byte, 0x80+16)
	copy(data[0x00:], "PSID")
	binary.BigEndian.PutUint16(data[0x04:], 3)
	binary.BigEndian.PutUint16(data[0x06:], 0x80)
	binary.BigEndian.PutUint16(data[0x08:], 0x1000)
	binary.BigEndian.PutUint16(data[0x0A:], 0x1000)
	binary.BigEndian.PutUint16(data[0x0C:], 0x1003)
	binary.BigEndian.PutUint16(data[0x0E:], 1)
	binary.BigEndian.PutUint16(data[0x10:], 1)
	copy(data[0x16:0x36], "DualSID\x00")
	copy(data[0x36:0x56], "Author\x00")
	copy(data[0x56:0x76], "2024\x00")
	binary.BigEndian.PutUint16(data[0x7C:], 0xD420) // sid2addr at $D420 (common)
	// sid3addr left as 0
	for i := 0x80; i < len(data); i++ {
		data[i] = 0x60
	}

	sid, err := ParseSIDData(data)
	if err != nil {
		t.Fatalf("ParseSIDData(dual-SID) should succeed, got error: %v", err)
	}
	if sid.Header.Sid2Addr != 0xD420 {
		t.Errorf("expected Sid2Addr=0xD420, got 0x%04X", sid.Header.Sid2Addr)
	}
	if sid.Header.Sid3Addr != 0 {
		t.Errorf("expected Sid3Addr=0, got 0x%04X", sid.Header.Sid3Addr)
	}
}

func TestSIDPlaybackBus_MultiSIDCapture(t *testing.T) {
	// Verify that the 6502 playback bus captures writes to SID2/SID3 addresses
	// with correct chip tagging.
	bus := newSIDPlaybackBus6502Multi(false, 0xD500, 0xD600)
	bus.StartFrame()

	// Write to SID1 (primary, $D400)
	bus.Write(0xD400, 0xAA) // SID1 voice 1 freq lo
	bus.Write(0xD404, 0x41) // SID1 voice 1 ctrl (gate+triangle)

	// Write to SID2 ($D500)
	bus.Write(0xD500, 0xBB) // SID2 voice 1 freq lo
	bus.Write(0xD504, 0x21) // SID2 voice 1 ctrl (gate+sawtooth)

	// Write to SID3 ($D600)
	bus.Write(0xD600, 0xCC) // SID3 voice 1 freq lo

	events := bus.GetEvents()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify chip tagging
	if events[0].Chip != 0 || events[0].Reg != 0x00 || events[0].Value != 0xAA {
		t.Errorf("event 0: chip=%d reg=0x%02X val=0x%02X, want chip=0 reg=0x00 val=0xAA",
			events[0].Chip, events[0].Reg, events[0].Value)
	}
	if events[1].Chip != 0 || events[1].Reg != 0x04 || events[1].Value != 0x41 {
		t.Errorf("event 1: chip=%d reg=0x%02X val=0x%02X, want chip=0 reg=0x04 val=0x41",
			events[1].Chip, events[1].Reg, events[1].Value)
	}
	if events[2].Chip != 1 || events[2].Reg != 0x00 || events[2].Value != 0xBB {
		t.Errorf("event 2: chip=%d reg=0x%02X val=0x%02X, want chip=1 reg=0x00 val=0xBB",
			events[2].Chip, events[2].Reg, events[2].Value)
	}
	if events[3].Chip != 1 || events[3].Reg != 0x04 || events[3].Value != 0x21 {
		t.Errorf("event 3: chip=%d reg=0x%02X val=0x%02X, want chip=1 reg=0x04 val=0x21",
			events[3].Chip, events[3].Reg, events[3].Value)
	}
	if events[4].Chip != 2 || events[4].Reg != 0x00 || events[4].Value != 0xCC {
		t.Errorf("event 4: chip=%d reg=0x%02X val=0x%02X, want chip=2 reg=0x00 val=0xCC",
			events[4].Chip, events[4].Reg, events[4].Value)
	}
}

func TestSIDPlaybackBus_MultiSIDReadback(t *testing.T) {
	// Verify that register reads from SID2/SID3 return written values.
	bus := newSIDPlaybackBus6502Multi(false, 0xD500, 0)
	bus.Write(0xD500, 0x42) // SID2 voice 1 freq lo
	bus.Write(0xD507, 0x99) // SID2 voice 2 freq lo

	if got := bus.Read(0xD500); got != 0x42 {
		t.Errorf("SID2 read reg 0x00: got 0x%02X, want 0x42", got)
	}
	if got := bus.Read(0xD507); got != 0x99 {
		t.Errorf("SID2 read reg 0x07: got 0x%02X, want 0x99", got)
	}
}

func TestSIDEngine_MultiSIDChipFilter(t *testing.T) {
	// Verify that TickSample dispatches Chip 1/2 events to secondary engines.
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	engine := NewSIDEngine(sound, 44100)
	engine.sid2 = NewSIDEngineMulti(sound, 44100, 4, SID2_BASE, SID2_END)
	engine.sid3 = NewSIDEngineMulti(sound, 44100, 7, SID3_BASE, SID3_END)

	events := []SIDEvent{
		{Sample: 0, Reg: 0x18, Value: 0x0F, Chip: 0}, // SID1 master vol=15
		{Sample: 0, Reg: 0x18, Value: 0x0F, Chip: 1}, // SID2 master vol=15
		{Sample: 0, Reg: 0x00, Value: 0x50, Chip: 0}, // SID1 voice 1 freq lo
		{Sample: 0, Reg: 0x00, Value: 0xA0, Chip: 2}, // SID3 voice 1 freq lo
	}
	engine.SetEvents(events, 100, false, 0)
	engine.SetPlaying(true)

	// Tick one sample to apply events
	engine.TickSample()

	// Verify SID1 register was applied
	engine.mutex.Lock()
	if engine.regs[0x18] != 0x0F {
		t.Errorf("SID1 reg 0x18: got 0x%02X, want 0x0F", engine.regs[0x18])
	}
	if engine.regs[0x00] != 0x50 {
		t.Errorf("SID1 reg 0x00: got 0x%02X, want 0x50", engine.regs[0x00])
	}
	engine.mutex.Unlock()

	// Verify SID2 register was applied (Chip 1)
	engine.sid2.mutex.Lock()
	if engine.sid2.regs[0x18] != 0x0F {
		t.Errorf("SID2 reg 0x18: got 0x%02X, want 0x0F", engine.sid2.regs[0x18])
	}
	engine.sid2.mutex.Unlock()

	// Verify SID3 register was applied (Chip 2)
	engine.sid3.mutex.Lock()
	if engine.sid3.regs[0x00] != 0xA0 {
		t.Errorf("SID3 reg 0x00: got 0x%02X, want 0xA0", engine.sid3.regs[0x00])
	}
	engine.sid3.mutex.Unlock()
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
