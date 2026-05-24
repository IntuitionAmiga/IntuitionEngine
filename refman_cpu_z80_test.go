package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func refmanCh28PSGChordProgram() []byte {
	return []byte{
		0x3E, 0x01, 0x32, 0x00, 0xF8,
		0x21, 0x17, 0x10, 0x06, 0x0A,
		0x7E, 0xD3, 0xF0, 0x23, 0x7E, 0xD3,
		0xF1, 0x23, 0x10, 0xF6, 0xC3, 0x14, 0x10,
		0x07, 0x38, 0x00, 0x71, 0x01, 0x00, 0x02,
		0x5F, 0x03, 0x00, 0x04, 0x47, 0x05, 0x00,
		0x08, 0x0F, 0x09, 0x0C, 0x0A, 0x09,
	}
}

func refmanCh28ANTICGraphicsProgram() []byte {
	return []byte{
		0x21, 0x46, 0x11,
		0x11, 0x00, 0x20,
		0x01, 0x43, 0x00,
		0xED, 0xB0,
		0x3E, 0x00, 0xD3, 0xD6, 0x3E, 0x02, 0xD3, 0xD7,
		0x3E, 0x01, 0xD3, 0xD6, 0x3E, 0xCE, 0xD3, 0xD7,
		0x3E, 0x04, 0xD3, 0xD6, 0x3E, 0x24, 0xD3, 0xD7,
		0x3E, 0x02, 0xD3, 0xD4, 0x3E, 0x00, 0xD3, 0xD5,
		0x3E, 0x03, 0xD3, 0xD4, 0x3E, 0x20, 0xD3, 0xD5,
		0x3E, 0x00, 0xD3, 0xD4, 0x3E, 0x22, 0xD3, 0xD5,
		0x3E, 0x0E, 0xD3, 0xD4, 0x3E, 0x01, 0xD3, 0xD5,
		0xC3, 0x43, 0x11,
		0x4F, 0x1B, 0x20, 0x4F, 0x1B, 0x20, 0x4F, 0x1B, 0x20,
		0x4F, 0x1B, 0x20, 0x4F, 0x1B, 0x20, 0x4F, 0x1B, 0x20,
		0x4F, 0x1B, 0x20, 0x4F, 0x1B, 0x20, 0x41, 0x18, 0x20,
		0x81, 0x42, 0x24, 0x18, 0x18, 0x24, 0x42, 0x81,
		0xFF, 0x00, 0xFF, 0x00, 0x3C, 0x42, 0x81, 0x42,
		0x3C, 0x00, 0x18, 0x3C, 0x7E, 0xFF, 0x7E, 0x3C,
		0x18, 0x00, 0x55, 0xAA, 0x55, 0xAA, 0xF0, 0x0F,
		0xF0, 0x0F, 0x99, 0x66, 0x99, 0x66, 0xC3, 0x3C,
	}
}

func extractRefmanCh28MonitorBytes(t *testing.T, heading string, startAddr uint64) []byte {
	t.Helper()

	path := filepath.Join("sdk", "docs", "refman", "28-z80.md")
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
		if strings.HasPrefix(line, "(z80)> d ") {
			break
		}
		if !strings.HasPrefix(line, "(z80)> w ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "(z80)> w "))
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
		t.Fatalf("no Z80 monitor writes found under %q in %s", heading, path)
	}
	return program
}

func TestRefmanCh28Z80PSGChordExample(t *testing.T) {
	wantProgram := refmanCh28PSGChordProgram()
	docProgram := extractRefmanCh28MonitorBytes(t, "## 28.7 A small example", uint64(PROG_START))
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
	lines := disassembleZ80(readMem, uint64(PROG_START), 12)
	wantMnemonics := []string{
		"LD A, $01",
		"LD ($F800), A",
		"LD HL, $1017",
		"LD B, $0A",
		"LD A, (HL)",
		"OUT ($F0), A",
		"INC HL",
		"LD A, (HL)",
		"OUT ($F1), A",
		"INC HL",
		"DJNZ $100A",
		"JP $1014",
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
	psg := NewPSGEngine(sound, SAMPLE_RATE)
	psg.AttachBusMemory(bus.GetMemory())
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterRead, sound.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterWrite8)
	bus.MapIO(PSG_BASE, PSG_END, psg.HandleRead, psg.HandleWrite)
	bus.MapIOByte(PSG_BASE, PSG_END, psg.HandleWrite8)

	z80Bus := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(z80Bus)
	for i, value := range docProgram {
		bus.Write8(uint32(PROG_START)+uint32(i), value)
	}
	cpu.PC = uint16(PROG_START)

	adapter := NewDebugZ80(cpu, nil)
	for range 4 + 10*7 {
		adapter.Step()
	}

	if got := sound.HandleRegisterRead(AUDIO_CTRL); got&1 == 0 {
		t.Fatalf("AUDIO_CTRL = 0x%X, want enabled", got)
	}
	wantRegs := []uint8{0x71, 0x00, 0x5F, 0x00, 0x47, 0x00, 0x00, 0x38, 0x0F, 0x0C, 0x09}
	for i, want := range wantRegs {
		if got := bus.Read8(PSG_BASE + uint32(i)); got != want {
			t.Fatalf("PSG shadow register %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	psg.mutex.Lock()
	gotRegs := append([]uint8(nil), psg.regs[:len(wantRegs)]...)
	psg.mutex.Unlock()
	if !bytes.Equal(gotRegs, wantRegs) {
		t.Fatalf("PSG engine registers = % X, want % X", gotRegs, wantRegs)
	}
	if cpu.PC != 0x1014 {
		t.Fatalf("PC after setup = 0x%04X, want 0x1014", cpu.PC)
	}
	if cpu.B != 0 || cpu.H != 0x10 || cpu.L != 0x2B {
		t.Fatalf("loop registers B/HL = 0x%02X/0x%02X%02X, want 0x00/0x102B", cpu.B, cpu.H, cpu.L)
	}
}

func TestRefmanCh28Z80ANTICGraphicsExample(t *testing.T) {
	const start = 0x1100

	wantProgram := refmanCh28ANTICGraphicsProgram()
	docProgram := extractRefmanCh28MonitorBytes(t, "## 28.8 ANTIC graphics example", start)
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
	lines := disassembleZ80(readMem, start, 33)
	wantMnemonics := []string{
		"LD HL, $1146",
		"LD DE, $2000",
		"LD BC, $0043",
		"LDIR",
		"LD A, $00",
		"OUT ($D6), A",
		"LD A, $02",
		"OUT ($D7), A",
		"LD A, $01",
		"OUT ($D6), A",
		"LD A, $CE",
		"OUT ($D7), A",
		"LD A, $04",
		"OUT ($D6), A",
		"LD A, $24",
		"OUT ($D7), A",
		"LD A, $02",
		"OUT ($D4), A",
		"LD A, $00",
		"OUT ($D5), A",
		"LD A, $03",
		"OUT ($D4), A",
		"LD A, $20",
		"OUT ($D5), A",
		"LD A, $00",
		"OUT ($D4), A",
		"LD A, $22",
		"OUT ($D5), A",
		"LD A, $0E",
		"OUT ($D4), A",
		"LD A, $01",
		"OUT ($D5), A",
		"JP $1143",
	}
	if len(lines) != len(wantMnemonics) {
		t.Fatalf("disassembled %d lines, want %d", len(lines), len(wantMnemonics))
	}
	for i, want := range wantMnemonics {
		if lines[i].Mnemonic != want {
			t.Fatalf("disassembly line %d = %q, want %q", i, lines[i].Mnemonic, want)
		}
	}

	bus, antic := newMappedANTICTestBus()
	z80Bus := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(z80Bus)
	for i, value := range docProgram {
		bus.Write8(uint32(start)+uint32(i), value)
	}
	cpu.PC = start

	adapter := NewDebugZ80(cpu, nil)
	for i := 0; i < 200 && cpu.PC != 0x1143; i++ {
		adapter.Step()
	}
	if cpu.PC != 0x1143 {
		t.Fatalf("PC after setup = 0x%04X, want 0x1143", cpu.PC)
	}

	wantCopied := wantProgram[0x46:]
	for i, want := range wantCopied {
		if got := bus.Read8(0x2000 + uint32(i)); got != want {
			t.Fatalf("copied ANTIC data byte %d = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	if got := antic.HandleRead(GTIA_COLPF0); got != 0x02 {
		t.Fatalf("GTIA_COLPF0 = 0x%02X, want 0x02", got)
	}
	if got := antic.HandleRead(GTIA_COLPF1); got != 0xCE {
		t.Fatalf("GTIA_COLPF1 = 0x%02X, want 0xCE", got)
	}
	if got := antic.HandleRead(GTIA_COLBK); got != 0x24 {
		t.Fatalf("GTIA_COLBK = 0x%02X, want 0x24", got)
	}
	if got := antic.HandleRead(ANTIC_DLISTL); got != 0x00 {
		t.Fatalf("ANTIC_DLISTL = 0x%02X, want 0x00", got)
	}
	if got := antic.HandleRead(ANTIC_DLISTH); got != 0x20 {
		t.Fatalf("ANTIC_DLISTH = 0x%02X, want 0x20", got)
	}
	if got := antic.HandleRead(ANTIC_DMACTL); got != ANTIC_DMA_DL|ANTIC_DMA_NORMAL {
		t.Fatalf("ANTIC_DMACTL = 0x%02X, want 0x22", got)
	}
	if got := antic.HandleRead(ANTIC_ENABLE); got != ANTIC_ENABLE_VIDEO {
		t.Fatalf("ANTIC_ENABLE = 0x%02X, want 0x01", got)
	}

	frame := antic.RenderFrame(nil)
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP), anticRGBA(0xCE); got != want {
		t.Fatalf("first motif pixel = %v, want %v", got, want)
	}
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT+1, ANTIC_BORDER_TOP), anticRGBA(0x02); got != want {
		t.Fatalf("second motif pixel = %v, want %v", got, want)
	}
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP+7), anticRGBA(0xCE); got != want {
		t.Fatalf("eighth motif row pixel = %v, want %v", got, want)
	}
}
