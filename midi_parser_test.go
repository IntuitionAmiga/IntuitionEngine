package main

import (
	"encoding/binary"
	"testing"
)

func TestParseMIDI_SMFType0RunningStatusTempoAndMetadata(t *testing.T) {
	data := testSMF(0, 96, [][]byte{
		testMTrk(
			0x00, 0xFF, 0x03, 0x04, 'T', 'e', 's', 't',
			0x00, 0xFF, 0x51, 0x03, 0x07, 0xA1, 0x20, // 120 BPM
			0x00, 0xC0, 0x05,
			0x00, 0x90, 0x3C, 0x64,
			0x60, 0x40, 0x50, // running-status note on
			0x60, 0x80, 0x3C, 0x00,
			0x00, 0x80, 0x40, 0x00,
			0x00, 0xFF, 0x2F, 0x00,
		),
	})

	file, err := ParseMIDIData(data)
	if err != nil {
		t.Fatalf("ParseMIDIData: %v", err)
	}
	if file.Format != 0 || file.TrackCount != 1 || file.Division != 96 {
		t.Fatalf("header = format %d tracks %d division %d", file.Format, file.TrackCount, file.Division)
	}
	if file.Metadata.Title != "Test" {
		t.Fatalf("title = %q, want Test", file.Metadata.Title)
	}
	if file.PatchTableName != "RawlandMini" {
		t.Fatalf("patch table = %q, want RawlandMini", file.PatchTableName)
	}
	if len(file.Events) != 5 {
		t.Fatalf("events=%d, want 5: %#v", len(file.Events), file.Events)
	}
	if file.Events[0].Kind != MIDIEventProgramChange || file.Events[0].Program != 5 {
		t.Fatalf("first event = %#v, want program change 5", file.Events[0])
	}
	if file.Events[2].Kind != MIDIEventNoteOn || file.Events[2].Note != 64 || file.Events[2].Velocity != 80 || file.Events[2].Tick != 96 {
		t.Fatalf("running-status note event = %#v", file.Events[2])
	}
	if file.Events[2].SampleTime != 22050 {
		t.Fatalf("running-status note sample time = %d, want 22050", file.Events[2].SampleTime)
	}
	if file.DurationSamples != 44100 {
		t.Fatalf("duration samples = %d, want 44100", file.DurationSamples)
	}
}

func TestParseMIDI_SMFType1MergesTempoTrack(t *testing.T) {
	data := testSMF(1, 96, [][]byte{
		testMTrk(
			0x00, 0xFF, 0x51, 0x03, 0x07, 0xA1, 0x20,
			0x60, 0xFF, 0x51, 0x03, 0x0F, 0x42, 0x40, // 60 BPM after tick 96
			0x00, 0xFF, 0x2F, 0x00,
		),
		testMTrk(
			0x00, 0x90, 0x3C, 0x64,
			0x81, 0x40, 0x80, 0x3C, 0x00,
			0x00, 0xFF, 0x2F, 0x00,
		),
	})

	file, err := ParseMIDIData(data)
	if err != nil {
		t.Fatalf("ParseMIDIData: %v", err)
	}
	if file.Format != 1 || len(file.Events) != 2 {
		t.Fatalf("format/events = %d/%d", file.Format, len(file.Events))
	}
	if file.Events[1].SampleTime != 66150 {
		t.Fatalf("merged tempo sample time = %d, want 66150", file.Events[1].SampleTime)
	}
	if bpm := file.TempoBPMAtSample(file.Events[1].SampleTime); bpm != 60 {
		t.Fatalf("tempo at note-off = %d BPM, want 60", bpm)
	}
}

func TestParseMIDI_RejectsMalformedType2AndSMPTE(t *testing.T) {
	if _, err := ParseMIDIData([]byte("MThd\x00\x00\x00\x06\x00\x02\x00\x01\x00\x60")); err == nil {
		t.Fatal("expected type 2 rejection")
	}
	if _, err := ParseMIDIData([]byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\xE7\x28")); err == nil {
		t.Fatal("expected SMPTE division rejection")
	}
	if _, err := ParseMIDIData([]byte("MThd\x00\x00")); err == nil {
		t.Fatal("expected truncated header rejection")
	}
}

func TestParseMIDI_MinimalMUS(t *testing.T) {
	data := testMUS(
		0x80|0x10, 60, // note on channel 0, note 60, last event in group
		0x01,     // delta 1 at 140 Hz
		0x00, 60, // note off channel 0, note 60
		0x60|0x00, // finish
	)
	file, err := ParseMIDIData(data)
	if err != nil {
		t.Fatalf("ParseMIDIData(MUS): %v", err)
	}
	if !file.IsMUS || file.FormatName != "MUS" {
		t.Fatalf("format = IsMUS %v name %q", file.IsMUS, file.FormatName)
	}
	if len(file.Events) != 2 {
		t.Fatalf("events=%d, want 2", len(file.Events))
	}
	if file.Events[0].Kind != MIDIEventNoteOn || file.Events[0].Channel != 0 || file.Events[0].Note != 60 {
		t.Fatalf("note on = %#v", file.Events[0])
	}
	if file.Events[1].SampleTime != 315 {
		t.Fatalf("MUS delta sample time=%d, want 315", file.Events[1].SampleTime)
	}
}

func TestParseMIDI_MUSChannelSwap9And15(t *testing.T) {
	data := testMUS(
		0x19, 60, // MUS channel 9 normal instrument maps away from MIDI percussion
		0x1F, 36, // MUS channel 15 percussion maps to MIDI channel 9
		0x60, // finish
	)
	file, err := ParseMIDIData(data)
	if err != nil {
		t.Fatalf("ParseMIDIData(MUS): %v", err)
	}
	if len(file.Events) != 2 {
		t.Fatalf("events=%d, want 2", len(file.Events))
	}
	if file.Events[0].Channel != 15 {
		t.Fatalf("MUS channel 9 mapped to MIDI channel %d, want 15", file.Events[0].Channel)
	}
	if file.Events[1].Channel != 9 {
		t.Fatalf("MUS channel 15 mapped to MIDI channel %d, want 9", file.Events[1].Channel)
	}
}

func testSMF(format uint16, division uint16, tracks [][]byte) []byte {
	out := []byte{'M', 'T', 'h', 'd', 0, 0, 0, 6}
	var hdr [6]byte
	binary.BigEndian.PutUint16(hdr[0:2], format)
	binary.BigEndian.PutUint16(hdr[2:4], uint16(len(tracks)))
	binary.BigEndian.PutUint16(hdr[4:6], division)
	out = append(out, hdr[:]...)
	for _, trk := range tracks {
		out = append(out, 'M', 'T', 'r', 'k')
		var lenbuf [4]byte
		binary.BigEndian.PutUint32(lenbuf[:], uint32(len(trk)))
		out = append(out, lenbuf[:]...)
		out = append(out, trk...)
	}
	return out
}

func testMTrk(data ...byte) []byte { return data }

func testMUS(events ...byte) []byte {
	out := make([]byte, 16)
	copy(out, "MUS\x1A")
	binary.LittleEndian.PutUint16(out[4:6], uint16(len(events)))
	binary.LittleEndian.PutUint16(out[6:8], 16)
	return append(out, events...)
}
