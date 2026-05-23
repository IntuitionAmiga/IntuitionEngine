package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func refmanCh26POKEYChordProgram() []byte {
	return []byte{
		0xA9, 0x01, 0x8D, 0x00, 0xF8,
		0xA9, 0x00, 0x8D, 0x08, 0xD2,
		0xA9, 0x79, 0x8D, 0x00, 0xD2,
		0xA9, 0xAF, 0x8D, 0x01, 0xD2,
		0xA9, 0x5F, 0x8D, 0x02, 0xD2,
		0xA9, 0xAC, 0x8D, 0x03, 0xD2,
		0xA9, 0x3F, 0x8D, 0x04, 0xD2,
		0xA9, 0xA8, 0x8D, 0x05, 0xD2,
		0x4C, 0x28, 0x10,
	}
}

func refmanCh26ULAGraphicsProgram() []byte {
	return []byte{
		0xA9, 0x05, 0x8D, 0x00, 0xD8,
		0xA9, 0x05, 0x8D, 0x04, 0xD8,
		0xA9, 0x00, 0x8D, 0x0C, 0xD8,
		0xA9, 0x00, 0x8D, 0x10, 0xD8,
		0xA9, 0xFF, 0x8D, 0x14, 0xD8,
		0xA9, 0x81, 0x8D, 0x14, 0xD8,
		0xA9, 0xBD, 0x8D, 0x14, 0xD8,
		0xA9, 0xA5, 0x8D, 0x14, 0xD8,
		0xA9, 0xA5, 0x8D, 0x14, 0xD8,
		0xA9, 0xBD, 0x8D, 0x14, 0xD8,
		0xA9, 0x81, 0x8D, 0x14, 0xD8,
		0xA9, 0xFF, 0x8D, 0x14, 0xD8,
		0xA9, 0x00, 0x8D, 0x0C, 0xD8,
		0xA9, 0x18, 0x8D, 0x10, 0xD8,
		0xA9, 0x46, 0x8D, 0x14, 0xD8,
		0x4C, 0x4B, 0x11,
	}
}

func extractRefman6502MonitorBytesFromFile(t *testing.T, path string, heading string, startAddr uint64) []byte {
	t.Helper()

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
		if strings.HasPrefix(line, "(6502)> d ") {
			break
		}
		if !strings.HasPrefix(line, "(6502)> w ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "(6502)> w "))
		if len(fields) < 2 {
			t.Fatalf("malformed monitor write line: %q", line)
		}
		addr, err := strconv.ParseUint(fields[0], 16, 16)
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
		t.Fatalf("no 6502 monitor writes found under %q in %s", heading, path)
	}
	return program
}

func extractRefmanCh26MonitorBytes(t *testing.T, heading string, startAddr uint64) []byte {
	t.Helper()
	path := filepath.Join("sdk", "docs", "refman", "26-6502.md")
	return extractRefman6502MonitorBytesFromFile(t, path, heading, startAddr)
}

func extractRefmanCh32MonitorBytes(t *testing.T, heading string, startAddr uint64) []byte {
	t.Helper()
	path := filepath.Join("sdk", "docs", "refman", "32-iemon.md")
	return extractRefman6502MonitorBytesFromFile(t, path, heading, startAddr)
}

func TestRefmanCh26POKEYChordExample(t *testing.T) {
	wantProgram := refmanCh26POKEYChordProgram()
	docProgram := extractRefmanCh26MonitorBytes(t, "## 26.8 A small example", uint64(PROG_START))
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	readMem := func(addr uint64, size int) []byte {
		if addr < uint64(PROG_START) {
			return nil
		}
		start := int(addr - uint64(PROG_START))
		if start >= len(docProgram) {
			return nil
		}
		end := start + size
		if end > len(docProgram) {
			end = len(docProgram)
		}
		out := make([]byte, end-start)
		copy(out, docProgram[start:end])
		return out
	}
	lines := disassemble6502(readMem, uint64(PROG_START), 17)
	wantMnemonics := []string{
		"LDA #$01",
		"STA $F800",
		"LDA #$00",
		"STA $D208",
		"LDA #$79",
		"STA $D200",
		"LDA #$AF",
		"STA $D201",
		"LDA #$5F",
		"STA $D202",
		"LDA #$AC",
		"STA $D203",
		"LDA #$3F",
		"STA $D204",
		"LDA #$A8",
		"STA $D205",
		"JMP $1028",
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
	pokey := NewPOKEYEngine(sound, SAMPLE_RATE)
	pokey.AttachBusMemory(bus.GetMemory())
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterRead, sound.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterWrite8)
	bus.MapIO(POKEY_BASE, POKEY_END, pokey.HandleRead, pokey.HandleWrite)
	bus.MapIOByte(POKEY_BASE, POKEY_END, pokey.HandleWrite8)
	bus.MapIOWideWriteFanout(POKEY_BASE, POKEY_END)

	cpu := NewCPU_6502(bus)
	for i, value := range docProgram {
		bus.Write8(uint32(PROG_START)+uint32(i), value)
	}
	cpu.PC = uint16(PROG_START)
	cpu.SetRunning(true)

	adapter := NewDebug6502(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("6502 stopped before reaching the self-loop")
		}
	}

	if got := sound.HandleRegisterRead(AUDIO_CTRL); got&1 == 0 {
		t.Fatalf("AUDIO_CTRL = 0x%X, want enabled", got)
	}
	wantRegs := []uint8{0x79, 0xAF, 0x5F, 0xAC, 0x3F, 0xA8}
	for i, want := range wantRegs {
		if got := bus.Read8(POKEY_BASE + uint32(i)); got != want {
			t.Fatalf("POKEY shadow register %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	pokey.mutex.Lock()
	gotRegs := append([]uint8(nil), pokey.regs[:6]...)
	gotAUDCTL := pokey.regs[8]
	pokey.mutex.Unlock()
	if !bytes.Equal(gotRegs, wantRegs) {
		t.Fatalf("POKEY engine registers = % X, want % X", gotRegs, wantRegs)
	}
	if gotAUDCTL != 0 {
		t.Fatalf("POKEY AUDCTL = 0x%02X, want 0x00", gotAUDCTL)
	}
	if cpu.PC != 0x1028 {
		t.Fatalf("PC after setup = 0x%04X, want 0x1028", cpu.PC)
	}
}

func TestRefmanCh26ULAGraphicsExample(t *testing.T) {
	const start = 0x1100

	wantProgram := refmanCh26ULAGraphicsProgram()
	docProgram := extractRefmanCh26MonitorBytes(t, "## 26.9 ULA graphics example", start)
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("manual monitor bytes = % X, want % X", docProgram, wantProgram)
	}

	readMem := func(addr uint64, size int) []byte {
		if addr < start {
			return nil
		}
		startOffset := int(addr - start)
		if startOffset >= len(docProgram) {
			return nil
		}
		end := startOffset + size
		if end > len(docProgram) {
			end = len(docProgram)
		}
		out := make([]byte, end-startOffset)
		copy(out, docProgram[startOffset:end])
		return out
	}
	lines := disassemble6502(readMem, start, 31)
	wantMnemonics := []string{
		"LDA #$05",
		"STA $D800",
		"LDA #$05",
		"STA $D804",
		"LDA #$00",
		"STA $D80C",
		"LDA #$00",
		"STA $D810",
		"LDA #$FF",
		"STA $D814",
		"LDA #$81",
		"STA $D814",
		"LDA #$BD",
		"STA $D814",
		"LDA #$A5",
		"STA $D814",
		"LDA #$A5",
		"STA $D814",
		"LDA #$BD",
		"STA $D814",
		"LDA #$81",
		"STA $D814",
		"LDA #$FF",
		"STA $D814",
		"LDA #$00",
		"STA $D80C",
		"LDA #$18",
		"STA $D810",
		"LDA #$46",
		"STA $D814",
		"JMP $114B",
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
	ula := installRealULA(bus)
	cpu := NewCPU_6502(bus)
	for i, value := range docProgram {
		bus.Write8(uint32(start)+uint32(i), value)
	}
	cpu.PC = start
	cpu.SetRunning(true)

	adapter := NewDebug6502(cpu, nil)
	for range len(wantMnemonics) - 1 {
		if adapter.Step() == 0 {
			t.Fatalf("6502 stopped before reaching the self-loop")
		}
	}

	if got := ula.HandleRead(ULA_BORDER); got != 0x05 {
		t.Fatalf("ULA_BORDER = 0x%02X, want 0x05", got)
	}
	if got := ula.HandleRead(ULA_CTRL); got != ULA_CTRL_ENABLE|ULA_CTRL_AUTO_INC {
		t.Fatalf("ULA_CTRL = 0x%02X, want enable plus auto-increment", got)
	}
	wantBitmap := []uint8{0xFF, 0x81, 0xBD, 0xA5, 0xA5, 0xBD, 0x81, 0xFF}
	for i, want := range wantBitmap {
		if got := ula.HandleVRAMRead(uint16(i)); got != want {
			t.Fatalf("ULA bitmap byte %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	if got := ula.HandleVRAMRead(0x1800); got != 0x46 {
		t.Fatalf("ULA attribute 0x1800 = 0x%02X, want 0x46", got)
	}
	if cpu.PC != 0x114B {
		t.Fatalf("PC after setup = 0x%04X, want 0x114B", cpu.PC)
	}
}

func TestRefmanCh32ULAGraphicsWorkflowBytes(t *testing.T) {
	const start = 0x1100

	wantProgram := refmanCh26ULAGraphicsProgram()
	docProgram := extractRefmanCh32MonitorBytes(t, "### 32.4.2 Byte-entry graphics workflow", start)
	if !bytes.Equal(docProgram, wantProgram) {
		t.Fatalf("Chapter 32 ULA workflow bytes = % X, want % X", docProgram, wantProgram)
	}
}
