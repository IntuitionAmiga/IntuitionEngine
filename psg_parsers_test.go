// psg_parsers_test.go - Parser tests for PSG formats.

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseVGMFileBasic(t *testing.T) {
	data := make([]byte, 0x40)
	copy(data[0:4], []byte("Vgm "))
	dataOffset := uint32(0)
	binary.LittleEndian.PutUint32(data[0x34:0x38], dataOffset)

	commands := []byte{
		0xA0, 0x00, 0x01,
		0x61, 0x10, 0x00,
		0x66,
	}
	data = append(data, commands...)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.vgm")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write vgm: %v", err)
	}

	vgm, err := ParseVGMFile(path)
	if err != nil {
		t.Fatalf("parse vgm: %v", err)
	}
	if len(vgm.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(vgm.Events))
	}
	if vgm.Events[0].Reg != 0 || vgm.Events[0].Value != 1 {
		t.Fatalf("unexpected event: %+v", vgm.Events[0])
	}
}

func TestParseVGMWaitsAndLoop(t *testing.T) {
	data := make([]byte, 0x40)
	copy(data[0:4], []byte("Vgm "))
	binary.LittleEndian.PutUint32(data[0x34:0x38], 0)

	commands := []byte{
		0xA0, 0x00, 0x01,
		0x61, 0x02, 0x00,
		0xA0, 0x00, 0x02,
		0x70, // wait 1 sample
		0xA0, 0x00, 0x03,
		0x66,
	}
	data = append(data, commands...)

	loopOffset := uint32(0x40 + 6) // loop starts at second write
	binary.LittleEndian.PutUint32(data[0x1C:0x20], loopOffset-0x1C)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_wait.vgm")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write vgm: %v", err)
	}

	vgm, err := ParseVGMFile(path)
	if err != nil {
		t.Fatalf("parse vgm: %v", err)
	}
	if len(vgm.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(vgm.Events))
	}
	if vgm.Events[0].Sample != 0 || vgm.Events[1].Sample != 2 || vgm.Events[2].Sample != 3 {
		t.Fatalf("unexpected event samples: %+v", vgm.Events)
	}
	if vgm.LoopSample != 2 {
		t.Fatalf("expected loop sample 2, got %d", vgm.LoopSample)
	}
}

func TestParseVGMPSGSkip(t *testing.T) {
	data := make([]byte, 0x40)
	copy(data[0:4], []byte("Vgm "))
	binary.LittleEndian.PutUint32(data[0x34:0x38], 0)

	commands := []byte{
		0x50, 0xFF, // SN76489 PSG write (unsupported, should skip)
		0xA0, 0x00, 0x01,
		0x66,
	}
	data = append(data, commands...)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_skip.vgm")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write vgm: %v", err)
	}

	vgm, err := ParseVGMFile(path)
	if err != nil {
		t.Fatalf("parse vgm: %v", err)
	}
	if len(vgm.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(vgm.Events))
	}
}

func TestParseAYRawFrames(t *testing.T) {
	frame := make([]byte, PSG_REG_COUNT)
	frame[0] = 0xAA
	frame[13] = 0x0F
	data := append(frame, frame...)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ay")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ay: %v", err)
	}

	ay, err := ParseAYFile(path)
	if err != nil {
		t.Fatalf("parse ay: %v", err)
	}
	if len(ay.Frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(ay.Frames))
	}
	if ay.Frames[0][0] != 0xAA || ay.Frames[0][13] != 0x0F {
		t.Fatalf("unexpected ay frame data")
	}
}

func TestParseYMFileBasic(t *testing.T) {
	frames := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E}
	data := make([]byte, 0)
	data = append(data, []byte("YM5!")...)
	data = append(data, []byte("LeOnArD!")...)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, 1) // frames
	data = append(data, buf...)
	binary.BigEndian.PutUint32(buf, 0) // attrs
	data = append(data, buf...)
	buf2 := make([]byte, 2)
	binary.BigEndian.PutUint16(buf2, 0) // drums
	data = append(data, buf2...)
	binary.BigEndian.PutUint32(buf, PSG_CLOCK_ATARI_ST)
	data = append(data, buf...)
	binary.BigEndian.PutUint16(buf2, 50)
	data = append(data, buf2...)
	binary.BigEndian.PutUint32(buf, 0)
	data = append(data, buf...)
	binary.BigEndian.PutUint16(buf2, 0)
	data = append(data, buf2...)
	data = append(data, []byte("Title\x00Author\x00Comment\x00")...)
	data = append(data, frames...)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ym")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ym: %v", err)
	}

	ym, err := ParseYMFile(path)
	if err != nil {
		t.Fatalf("parse ym: %v", err)
	}
	if ym.FrameRate != 50 {
		t.Fatalf("expected frame rate 50, got %d", ym.FrameRate)
	}
	if len(ym.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(ym.Frames))
	}
	if ym.Frames[0][0] != 0x01 || ym.Frames[0][13] != 0x0E {
		t.Fatalf("unexpected ym frame data")
	}
}
