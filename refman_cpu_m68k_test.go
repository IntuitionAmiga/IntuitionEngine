//go:build headless

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func refmanCh29SIDProgram() []byte {
	return []byte{
		0x13, 0xFC, 0x00, 0x01, 0x00, 0x0F, 0x08, 0x00,
		0x13, 0xFC, 0x00, 0x0F, 0x00, 0x0F, 0x0E, 0x18,
		0x13, 0xFC, 0x00, 0xC3, 0x00, 0x0F, 0x0E, 0x00,
		0x13, 0xFC, 0x00, 0x10, 0x00, 0x0F, 0x0E, 0x01,
		0x13, 0xFC, 0x00, 0x00, 0x00, 0x0F, 0x0E, 0x05,
		0x13, 0xFC, 0x00, 0xF0, 0x00, 0x0F, 0x0E, 0x06,
		0x13, 0xFC, 0x00, 0x21, 0x00, 0x0F, 0x0E, 0x04,
		0x60, 0x00, 0xFF, 0xFE,
	}
}

func refmanCh29VoodooTriangleProgram() []byte {
	return []byte{
		0x23, 0xFC, 0x00, 0x00, 0x00, 0x01, 0x00, 0x0F, 0x80, 0x04,
		0x23, 0xFC, 0xFF, 0x00, 0x00, 0xFF, 0x00, 0x0F, 0x81, 0xD8,
		0x23, 0xFC, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0F, 0x81, 0x24,
		0x23, 0xFC, 0x00, 0x00, 0x14, 0x00, 0x00, 0x0F, 0x80, 0x08,
		0x23, 0xFC, 0x00, 0x00, 0x06, 0x40, 0x00, 0x0F, 0x80, 0x0C,
		0x23, 0xFC, 0x00, 0x00, 0x1F, 0x40, 0x00, 0x0F, 0x80, 0x10,
		0x23, 0xFC, 0x00, 0x00, 0x17, 0xC0, 0x00, 0x0F, 0x80, 0x14,
		0x23, 0xFC, 0x00, 0x00, 0x08, 0xC0, 0x00, 0x0F, 0x80, 0x18,
		0x23, 0xFC, 0x00, 0x00, 0x17, 0xC0, 0x00, 0x0F, 0x80, 0x1C,
		0x23, 0xFC, 0x00, 0x00, 0x10, 0x00, 0x00, 0x0F, 0x80, 0x20,
		0x23, 0xFC, 0x00, 0x00, 0x08, 0x00, 0x00, 0x0F, 0x80, 0x24,
		0x23, 0xFC, 0x00, 0x00, 0x02, 0x00, 0x00, 0x0F, 0x80, 0x28,
		0x23, 0xFC, 0x00, 0x00, 0x10, 0x00, 0x00, 0x0F, 0x80, 0x30,
		0x23, 0xFC, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0F, 0x80, 0x80,
		0x23, 0xFC, 0x00, 0x00, 0x00, 0x01, 0x00, 0x0F, 0x81, 0x28,
		0x60, 0x00, 0xFF, 0xFE,
	}
}

func extractRefmanCh29MonitorBytes(t *testing.T, heading string, startAddr uint64) []byte {
	t.Helper()

	path := filepath.Join("sdk", "docs", "refman", "29-m68k.md")
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
		if strings.HasPrefix(line, "(m68k)> d ") {
			break
		}
		if !strings.HasPrefix(line, "(m68k)> w ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "(m68k)> w "))
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
		t.Fatalf("no M68K monitor writes found under %q in %s", heading, path)
	}
	return program
}

func readM68KDocProgram(program []byte, start uint64) func(uint64, int) []byte {
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

func TestRefmanCh29M68KSIDExample(t *testing.T) {
	wantProgram := refmanCh29SIDProgram()
	docProgram := extractRefmanCh29MonitorBytes(t, "## 29.9 A small example", uint64(PROG_START))
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	lines := disassembleM68K(readM68KDocProgram(docProgram, uint64(PROG_START)), uint64(PROG_START), 8)
	wantMnemonics := []string{
		"MOVE.B #$01, $000F0800",
		"MOVE.B #$0F, $000F0E18",
		"MOVE.B #$C3, $000F0E00",
		"MOVE.B #$10, $000F0E01",
		"MOVE.B #$00, $000F0E05",
		"MOVE.B #$F0, $000F0E06",
		"MOVE.B #$21, $000F0E04",
		"BRA.W $00001038",
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
	sid := NewSIDEngine(sound, SAMPLE_RATE)
	sid.AttachBusMemory(bus.GetMemory())
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterRead, sound.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterWrite8)
	bus.MapIO(SID_BASE, SID_END, sid.HandleRead, sid.HandleWrite)
	bus.MapIOByte(SID_BASE, SID_END, sid.HandleWrite8)
	bus.MapIOWideWriteFanout(SID_BASE, SID_END)

	cpu := NewM68KCPU(bus)
	for i, value := range docProgram {
		bus.Write8(uint32(PROG_START)+uint32(i), value)
	}
	cpu.PC = uint32(PROG_START)
	adapter := NewDebugM68K(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("M68K stopped before reaching the self-loop")
		}
	}

	if got := sound.HandleRegisterRead(AUDIO_CTRL); got&1 == 0 {
		t.Fatalf("AUDIO_CTRL = 0x%X, want enabled", got)
	}
	wantRegs := []uint8{0xC3, 0x10, 0x00, 0x00, 0x21, 0x00, 0xF0}
	for i, want := range wantRegs {
		if got := bus.Read8(SID_BASE + uint32(i)); got != want {
			t.Fatalf("SID register %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	if got := bus.Read8(SID_BASE + 0x18); got != 0x0F {
		t.Fatalf("SID MODE_VOL = 0x%02X, want 0x0F", got)
	}
	if cpu.PC != 0x1038 {
		t.Fatalf("PC after setup = 0x%04X, want 0x1038", cpu.PC)
	}
}

func TestRefmanCh29M68KVoodooTriangleExample(t *testing.T) {
	const start = 0x1100

	wantProgram := refmanCh29VoodooTriangleProgram()
	docProgram := extractRefmanCh29MonitorBytes(t, "## 29.10 Voodoo triangle example", start)
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	lines := disassembleM68K(readM68KDocProgram(docProgram, start), start, 16)
	wantMnemonics := []string{
		"MOVE.L #$00000001, $000F8004",
		"MOVE.L #$FF0000FF, $000F81D8",
		"MOVE.L #$00000000, $000F8124",
		"MOVE.L #$00001400, $000F8008",
		"MOVE.L #$00000640, $000F800C",
		"MOVE.L #$00001F40, $000F8010",
		"MOVE.L #$000017C0, $000F8014",
		"MOVE.L #$000008C0, $000F8018",
		"MOVE.L #$000017C0, $000F801C",
		"MOVE.L #$00001000, $000F8020",
		"MOVE.L #$00000800, $000F8024",
		"MOVE.L #$00000200, $000F8028",
		"MOVE.L #$00001000, $000F8030",
		"MOVE.L #$00000000, $000F8080",
		"MOVE.L #$00000001, $000F8128",
		"BRA.W $00001196",
	}
	if len(lines) != len(wantMnemonics) {
		t.Fatalf("disassembled %d lines, want %d", len(lines), len(wantMnemonics))
	}
	for i, want := range wantMnemonics {
		if lines[i].Mnemonic != want {
			t.Fatalf("disassembly line %d = %q, want %q", i, lines[i].Mnemonic, want)
		}
	}

	bus, voodoo := newMappedTestVoodoo(t)
	cpu := NewM68KCPU(bus)
	for i, value := range docProgram {
		bus.Write8(uint32(start)+uint32(i), value)
	}
	cpu.PC = start
	adapter := NewDebugM68K(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("M68K stopped before reaching the self-loop")
		}
	}

	if !voodoo.enabled.Load() {
		t.Fatalf("Voodoo engine was not enabled")
	}
	if got := voodoo.HandleRead(VOODOO_COLOR0); got != 0xFF0000FF {
		t.Fatalf("VOODOO_COLOR0 = 0x%08X, want 0xFF0000FF", got)
	}
	if got := voodoo.HandleRead(VOODOO_TRIANGLE_CMD); got != 0 {
		t.Fatalf("VOODOO_TRIANGLE_CMD shadow = 0x%08X, want 0", got)
	}
	if len(voodoo.triangleBatch) != 0 {
		t.Fatalf("triangle batch length after swap = %d, want 0", len(voodoo.triangleBatch))
	}
	if voodoo.vertices[0].X != 320 || voodoo.vertices[0].Y != 100 ||
		voodoo.vertices[1].X != 500 || voodoo.vertices[1].Y != 380 ||
		voodoo.vertices[2].X != 140 || voodoo.vertices[2].Y != 380 {
		t.Fatalf("Voodoo vertices = %+v, want A(320,100) B(500,380) C(140,380)", voodoo.vertices)
	}
	frame := voodoo.GetFrame()
	if len(frame) < 640*480*4 {
		t.Fatalf("Voodoo frame length = %d, want at least %d", len(frame), 640*480*4)
	}
	center := (250*640 + 320) * 4
	if frame[center] == 0x00 && frame[center+1] == 0x00 && frame[center+2] == 0xFF {
		t.Fatalf("triangle centre pixel is still blue, triangle did not render")
	}
	if cpu.PC != 0x1196 {
		t.Fatalf("PC after setup = 0x%04X, want 0x1196", cpu.PC)
	}
}
