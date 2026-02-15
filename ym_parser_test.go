package main

import (
	"encoding/binary"
	"testing"
)

// buildYM2Data creates a minimal YM2 file: "YM2!" + interleaved 14-reg frame data.
func buildYM2Data(frameCount int) []byte {
	data := []byte("YM2!")
	// Interleaved: all values for reg 0 (frameCount bytes), then reg 1, etc.
	for reg := 0; reg < 14; reg++ {
		for f := 0; f < frameCount; f++ {
			data = append(data, byte(reg*16+f))
		}
	}
	return data
}

// buildYM3Data creates a minimal YM3 file: "YM3!" + interleaved 14-reg frame data.
func buildYM3Data(frameCount int) []byte {
	data := []byte("YM3!")
	for reg := 0; reg < 14; reg++ {
		for f := 0; f < frameCount; f++ {
			data = append(data, byte(reg*16+f))
		}
	}
	return data
}

// buildYM3bData creates a minimal YM3b file: "YM3b" + interleaved data + 4-byte loop frame.
func buildYM3bData(frameCount int, loopFrame uint32) []byte {
	data := []byte("YM3b")
	for reg := 0; reg < 14; reg++ {
		for f := 0; f < frameCount; f++ {
			data = append(data, byte(reg*16+f))
		}
	}
	loop := make([]byte, 4)
	binary.BigEndian.PutUint32(loop, loopFrame)
	data = append(data, loop...)
	return data
}

// buildYM4Data creates a minimal YM4 file: "YM4!" + "LeOnArD!" + header + metadata + frame data.
func buildYM4Data(frameCount int, clock uint32, frameRate uint16, loopFrame uint32, interleaved bool) []byte {
	data := []byte("YM4!LeOnArD!")
	header := make([]byte, 22)
	binary.BigEndian.PutUint32(header[0:4], uint32(frameCount))
	attrs := uint32(0)
	if interleaved {
		attrs |= 0x01
	}
	binary.BigEndian.PutUint32(header[4:8], attrs)   // songAttrs
	binary.BigEndian.PutUint16(header[8:10], 0)      // numDrums
	binary.BigEndian.PutUint32(header[10:14], clock) // clock
	binary.BigEndian.PutUint16(header[14:16], frameRate)
	binary.BigEndian.PutUint32(header[16:20], loopFrame)
	binary.BigEndian.PutUint16(header[20:22], 0) // addData
	data = append(data, header...)

	// Null-terminated strings: title, author, comments
	data = append(data, []byte("Test Song\x00Author\x00Comment\x00")...)

	// Frame data: 16 regs per frame
	regsPerFrame := 16
	if interleaved {
		for reg := 0; reg < regsPerFrame; reg++ {
			for f := 0; f < frameCount; f++ {
				data = append(data, byte(reg*16+f))
			}
		}
	} else {
		for f := 0; f < frameCount; f++ {
			for reg := 0; reg < regsPerFrame; reg++ {
				data = append(data, byte(reg*16+f))
			}
		}
	}
	return data
}

func TestYMParse_YM2(t *testing.T) {
	data := buildYM2Data(100)
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM2) failed: %v", err)
	}
	if len(ym.Frames) != 100 {
		t.Fatalf("expected 100 frames, got %d", len(ym.Frames))
	}
	if ym.FrameRate != 50 {
		t.Errorf("expected default frame rate 50, got %d", ym.FrameRate)
	}
	if ym.LoopFrame != 0 {
		t.Errorf("expected loop frame 0 (no loop), got %d", ym.LoopFrame)
	}
	// Verify deinterleaving: frame 0 reg 0 should be 0*16+0=0
	if ym.Frames[0][0] != 0x00 {
		t.Errorf("frame 0 reg 0: expected 0x00, got 0x%02X", ym.Frames[0][0])
	}
	// frame 0 reg 1 should be 1*16+0=16
	if ym.Frames[0][1] != 0x10 {
		t.Errorf("frame 0 reg 1: expected 0x10, got 0x%02X", ym.Frames[0][1])
	}
	// frame 1 reg 0 should be 0*16+1=1
	if ym.Frames[1][0] != 0x01 {
		t.Errorf("frame 1 reg 0: expected 0x01, got 0x%02X", ym.Frames[1][0])
	}
}

func TestYMParse_YM3(t *testing.T) {
	data := buildYM3Data(50)
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM3) failed: %v", err)
	}
	if len(ym.Frames) != 50 {
		t.Fatalf("expected 50 frames, got %d", len(ym.Frames))
	}
	if ym.FrameRate != 50 {
		t.Errorf("expected default frame rate 50, got %d", ym.FrameRate)
	}
}

func TestYMParse_YM3b_Loop(t *testing.T) {
	data := buildYM3bData(200, 100)
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM3b) failed: %v", err)
	}
	if len(ym.Frames) != 200 {
		t.Fatalf("expected 200 frames, got %d", len(ym.Frames))
	}
	if ym.LoopFrame != 100 {
		t.Errorf("expected loop frame 100, got %d", ym.LoopFrame)
	}
}

func TestYMParse_YM4_Interleaved(t *testing.T) {
	data := buildYM4Data(100, 2000000, 50, 0, true)
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM4 interleaved) failed: %v", err)
	}
	if len(ym.Frames) != 100 {
		t.Fatalf("expected 100 frames, got %d", len(ym.Frames))
	}
	if ym.ClockHz != 2000000 {
		t.Errorf("expected clock 2000000, got %d", ym.ClockHz)
	}
	if ym.Title != "Test Song" {
		t.Errorf("expected title 'Test Song', got %q", ym.Title)
	}
	if ym.Author != "Author" {
		t.Errorf("expected author 'Author', got %q", ym.Author)
	}
}

func TestYMParse_YM4_NonInterleaved(t *testing.T) {
	data := buildYM4Data(80, 1773400, 50, 20, false)
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM4 non-interleaved) failed: %v", err)
	}
	if len(ym.Frames) != 80 {
		t.Fatalf("expected 80 frames, got %d", len(ym.Frames))
	}
	if ym.LoopFrame != 20 {
		t.Errorf("expected loop frame 20, got %d", ym.LoopFrame)
	}
	if ym.Interleaved {
		t.Error("expected non-interleaved")
	}
	// Verify frame data: frame 0 reg 0 = 0*16+0 = 0
	if ym.Frames[0][0] != 0x00 {
		t.Errorf("frame 0 reg 0: expected 0x00, got 0x%02X", ym.Frames[0][0])
	}
	// frame 0 reg 1 = 1*16+0 = 16
	if ym.Frames[0][1] != 0x10 {
		t.Errorf("frame 0 reg 1: expected 0x10, got 0x%02X", ym.Frames[0][1])
	}
}

func TestYMParse_YM5_StillWorks(t *testing.T) {
	// Ensure YM5 parsing is not broken by the new code.
	data := buildYM4Data(50, 2000000, 50, 0, true)
	// Change magic to YM5!
	copy(data[0:4], "YM5!")
	ym, err := parseYMData(data)
	if err != nil {
		t.Fatalf("parseYMData(YM5) failed: %v", err)
	}
	if len(ym.Frames) != 50 {
		t.Fatalf("expected 50 frames, got %d", len(ym.Frames))
	}
}
