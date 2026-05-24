package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func refmanCh30TEDAudioProgram() []byte {
	return []byte{
		0xB0, 0x01, 0xA2, 0x00, 0x08, 0x0F, 0x00,
		0xB0, 0x1C, 0xA2, 0x00, 0x0F, 0x0F, 0x00,
		0xB0, 0x02, 0xA2, 0x04, 0x0F, 0x0F, 0x00,
		0xB0, 0x58, 0xA2, 0x01, 0x0F, 0x0F, 0x00,
		0xB0, 0x02, 0xA2, 0x02, 0x0F, 0x0F, 0x00,
		0xB0, 0x38, 0xA2, 0x03, 0x0F, 0x0F, 0x00,
		0xEB, 0xFE,
	}
}

func refmanCh30TEDGraphicsProgram() []byte {
	return []byte{
		0xB0, 0x01, 0xA2, 0x58, 0x0F, 0x0F, 0x00,
		0xB0, 0x18, 0xA2, 0x20, 0x0F, 0x0F, 0x00,
		0xB0, 0x08, 0xA2, 0x24, 0x0F, 0x0F, 0x00,
		0xB0, 0x06, 0xA2, 0x30, 0x0F, 0x0F, 0x00,
		0xB0, 0x2E, 0xA2, 0x40, 0x0F, 0x0F, 0x00,
		0xB0, 0x01, 0xA2, 0x00, 0x30, 0x0F, 0x00,
		0xB0, 0x4E, 0xA2, 0x00, 0x34, 0x0F, 0x00,
		0xB0, 0xFF, 0xA2, 0x08, 0x38, 0x0F, 0x00,
		0xB0, 0x81, 0xA2, 0x09, 0x38, 0x0F, 0x00,
		0xB0, 0xBD, 0xA2, 0x0A, 0x38, 0x0F, 0x00,
		0xB0, 0xA5, 0xA2, 0x0B, 0x38, 0x0F, 0x00,
		0xB0, 0xA5, 0xA2, 0x0C, 0x38, 0x0F, 0x00,
		0xB0, 0xBD, 0xA2, 0x0D, 0x38, 0x0F, 0x00,
		0xB0, 0x81, 0xA2, 0x0E, 0x38, 0x0F, 0x00,
		0xB0, 0xFF, 0xA2, 0x0F, 0x38, 0x0F, 0x00,
		0xEB, 0xFE,
	}
}

func extractRefmanCh30MonitorBytes(t *testing.T, heading string, startAddr uint64) []byte {
	t.Helper()

	path := filepath.Join("sdk", "docs", "refman", "30-x86.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var program []byte
	nextAddr := startAddr
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		if line == heading {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			break
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(line, "(x86)> d ") {
			break
		}
		if !strings.HasPrefix(line, "(x86)> w ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "(x86)> w "))
		if len(fields) < 2 {
			t.Fatalf("malformed monitor write line: %q", line)
		}
		addr, err := strconv.ParseUint(fields[0], 16, 32)
		if err != nil {
			t.Fatalf("parse monitor write address %q: %v", fields[0], err)
		}
		if addr != nextAddr {
			t.Fatalf("monitor write address 0x%04X, want 0x%04X", addr, nextAddr)
		}
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 16, 8)
			if err != nil {
				t.Fatalf("parse monitor byte %q: %v", field, err)
			}
			program = append(program, byte(value))
			nextAddr++
		}
	}
	if len(program) == 0 {
		t.Fatalf("no x86 monitor writes found under %q in %s", heading, path)
	}
	return program
}

func readX86DocProgram(program []byte, start uint64) func(uint64, int) []byte {
	return func(addr uint64, size int) []byte {
		if addr < start {
			return nil
		}
		startOffset := int(addr - start)
		if startOffset >= len(program) {
			return nil
		}
		end := startOffset + size
		if end > len(program) {
			end = len(program)
		}
		out := make([]byte, end-startOffset)
		copy(out, program[startOffset:end])
		return out
	}
}

func TestRefmanCh30X86TEDAudioExample(t *testing.T) {
	wantProgram := refmanCh30TEDAudioProgram()
	docProgram := extractRefmanCh30MonitorBytes(t, "## 30.7 A small example", uint64(PROG_START))
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	lines := disassembleX86(readX86DocProgram(docProgram, uint64(PROG_START)), uint64(PROG_START), 13)
	wantMnemonics := []string{
		"MOV AL, $01",
		"MOV [$000F0800], AL",
		"MOV AL, $1C",
		"MOV [$000F0F00], AL",
		"MOV AL, $02",
		"MOV [$000F0F04], AL",
		"MOV AL, $58",
		"MOV [$000F0F01], AL",
		"MOV AL, $02",
		"MOV [$000F0F02], AL",
		"MOV AL, $38",
		"MOV [$000F0F03], AL",
		"JMP SHORT $0000102A",
	}
	if len(lines) != len(wantMnemonics) {
		t.Fatalf("disassembled %d lines, want %d", len(lines), len(wantMnemonics))
	}
	for i, want := range wantMnemonics {
		if lines[i].Mnemonic != want {
			t.Fatalf("disassembly line %d = %q, want %q", i, lines[i].Mnemonic, want)
		}
	}

	bus := NewMachineBus()
	sound := newTestSoundChip()
	sound.AttachBus(bus)
	ted := NewTEDEngine(sound, SAMPLE_RATE)
	ted.AttachBusMemory(bus.GetMemory())
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterRead, sound.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterWrite8)
	bus.MapIO(TED_BASE, TED_END, ted.HandleRead, ted.HandleWrite)

	x86Bus := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(x86Bus)
	for i, value := range docProgram {
		bus.Write8(uint32(PROG_START)+uint32(i), value)
	}
	cpu.EIP = uint32(PROG_START)
	adapter := NewDebugX86(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("x86 stopped before reaching the self-loop")
		}
	}

	if got := sound.HandleRegisterRead(AUDIO_CTRL); got&1 == 0 {
		t.Fatalf("AUDIO_CTRL = 0x%X, want enabled", got)
	}
	wantRegs := []uint8{0x1C, 0x58, 0x02, 0x38, 0x02}
	for i, want := range wantRegs {
		if got := bus.Read8(TED_BASE + uint32(i)); got != want {
			t.Fatalf("TED audio register %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	if cpu.EIP != 0x102A {
		t.Fatalf("EIP after setup = 0x%04X, want 0x102A", cpu.EIP)
	}
}

func TestRefmanCh30X86TEDGraphicsExample(t *testing.T) {
	const start = 0x1100

	wantProgram := refmanCh30TEDGraphicsProgram()
	docProgram := extractRefmanCh30MonitorBytes(t, "## 30.8 TED video example", start)
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	lines := disassembleX86(readX86DocProgram(docProgram, start), start, 31)
	wantMnemonics := []string{
		"MOV AL, $01", "MOV [$000F0F58], AL",
		"MOV AL, $18", "MOV [$000F0F20], AL",
		"MOV AL, $08", "MOV [$000F0F24], AL",
		"MOV AL, $06", "MOV [$000F0F30], AL",
		"MOV AL, $2E", "MOV [$000F0F40], AL",
		"MOV AL, $01", "MOV [$000F3000], AL",
		"MOV AL, $4E", "MOV [$000F3400], AL",
		"MOV AL, $FF", "MOV [$000F3808], AL",
		"MOV AL, $81", "MOV [$000F3809], AL",
		"MOV AL, $BD", "MOV [$000F380A], AL",
		"MOV AL, $A5", "MOV [$000F380B], AL",
		"MOV AL, $A5", "MOV [$000F380C], AL",
		"MOV AL, $BD", "MOV [$000F380D], AL",
		"MOV AL, $81", "MOV [$000F380E], AL",
		"MOV AL, $FF", "MOV [$000F380F], AL",
		"JMP SHORT $00001169",
	}
	if len(lines) != len(wantMnemonics) {
		t.Fatalf("disassembled %d lines, want %d", len(lines), len(wantMnemonics))
	}
	for i, want := range wantMnemonics {
		if lines[i].Mnemonic != want {
			t.Fatalf("disassembly line %d = %q, want %q", i, lines[i].Mnemonic, want)
		}
	}

	bus := NewMachineBus()
	ted := NewTEDVideoEngine(bus)
	bus.MapIO(TED_VIDEO_BASE, TED_VIDEO_END, ted.HandleRead, ted.HandleWrite)
	bus.MapIO(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1, ted.HandleBusVRAMRead, ted.HandleBusVRAMWrite)

	x86Bus := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(x86Bus)
	for i, value := range docProgram {
		bus.Write8(uint32(start)+uint32(i), value)
	}
	cpu.EIP = start
	adapter := NewDebugX86(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("x86 stopped before reaching the self-loop")
		}
	}

	if got := ted.HandleRead(TED_V_ENABLE); got != TED_V_ENABLE_VIDEO {
		t.Fatalf("TED_V_ENABLE = 0x%02X, want 0x01", got)
	}
	if got := ted.HandleRead(TED_V_CTRL1); got != TED_V_CTRL1_DEN|TED_V_CTRL1_RSEL {
		t.Fatalf("TED_V_CTRL1 = 0x%02X, want 0x18", got)
	}
	if got := ted.HandleRead(TED_V_CTRL2); got != TED_V_CTRL2_CSEL {
		t.Fatalf("TED_V_CTRL2 = 0x%02X, want 0x08", got)
	}
	if got := ted.HandleRead(TED_V_BG_COLOR0); got != 0x06 {
		t.Fatalf("TED_V_BG_COLOR0 = 0x%02X, want 0x06", got)
	}
	if got := ted.HandleRead(TED_V_BORDER); got != 0x2E {
		t.Fatalf("TED_V_BORDER = 0x%02X, want 0x2E", got)
	}
	if got := ted.HandleVRAMRead(0); got != 0x01 {
		t.Fatalf("TED matrix byte 0 = 0x%02X, want 0x01", got)
	}
	if got := ted.HandleVRAMRead(TED_V_MATRIX_SIZE); got != 0x4E {
		t.Fatalf("TED colour byte 0 = 0x%02X, want 0x4E", got)
	}
	wantGlyph := []uint8{0xFF, 0x81, 0xBD, 0xA5, 0xA5, 0xBD, 0x81, 0xFF}
	for i, want := range wantGlyph {
		if got := ted.HandleVRAMRead(uint16(TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE + 8 + i)); got != want {
			t.Fatalf("TED glyph byte %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	frame := ted.RenderFrame()
	setOffset := (TED_V_BORDER_TOP*TED_V_FRAME_WIDTH + TED_V_BORDER_LEFT) * 4
	clearOffset := ((TED_V_BORDER_TOP+1)*TED_V_FRAME_WIDTH + TED_V_BORDER_LEFT + 1) * 4
	wantSet := TEDPalette[0x4E&0x7F]
	wantClear := TEDPalette[0x06]
	if got := [3]byte{frame[setOffset], frame[setOffset+1], frame[setOffset+2]}; got != wantSet {
		t.Fatalf("top-left set pixel RGB = %v, want %v", got, wantSet)
	}
	if got := [3]byte{frame[clearOffset], frame[clearOffset+1], frame[clearOffset+2]}; got != wantClear {
		t.Fatalf("top-left clear pixel RGB = %v, want %v", got, wantClear)
	}
	if cpu.EIP != 0x1169 {
		t.Fatalf("EIP after setup = 0x%04X, want 0x1169", cpu.EIP)
	}
}
