//go:build headless

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const vgaTextSAPSource = "sdk/examples/asm/vga_text_sap_demo.asm"

const (
	vgaTextSAPSceneAddress  = 0x0F00
	vgaTextSAPFrameLow      = 0x0F01
	vgaTextSAPFrameHigh     = 0x0F02
	vgaTextSAPFrameDone     = 0x0F11
	vgaTextSAPMotionAngle   = 0x0F1B
	vgaTextSAPMotionVX      = 0x0F1C
	vgaTextSAPMotionVY      = 0x0F1D
	vgaTextSAPMotionXAccum  = 0x0F1E
	vgaTextSAPMotionYAccum  = 0x0F20
	vgaTextSAPColourPhase   = 0x0F22
	vgaTextSAPLogoPhaseX    = 0x0F26
	vgaTextSAPLogoPhaseY    = 0x0F27
	vgaTextSAPLogoPositionX = 0x0F28
	vgaTextSAPLogoPositionY = 0x0F29
	vgaTextSAPLogoFractionX = 0x0F2A
	vgaTextSAPLogoFractionY = 0x0F2B
	vgaTextSAPLogoYTick     = 0x0F2C
)

type vgaTextSAPTestBus struct {
	memory        [65536]byte
	vga           *VGAEngine
	textBank      byte
	statusRead    uint64
	displayStart  int
	visibleWrites int
}

func (b *vgaTextSAPTestBus) Read(addr uint16) byte {
	if b.textBank == 0x2E && addr >= 0x8000 && addr < 0xC000 {
		return byte(b.vga.HandleTextRead(VGA_TEXT_WINDOW + uint32(addr-0x8000)))
	}
	return b.memory[addr]
}

func (b *vgaTextSAPTestBus) Write(addr uint16, value byte) {
	if addr == Z80_VRAM_BANK_REG {
		b.textBank = value
		return
	}
	if b.textBank == 0x2E && addr >= 0x8000 && addr < 0xC000 {
		offset := int(addr - 0x8000)
		if offset >= b.displayStart && offset < b.displayStart+VGA_TEXT_COLS*VGA_TEXT_ROWS*2 {
			b.visibleWrites++
		}
		b.vga.HandleTextWrite(VGA_TEXT_WINDOW+uint32(addr-0x8000), uint32(value))
		return
	}
	b.memory[addr] = value
}

func (b *vgaTextSAPTestBus) In(port uint16) byte {
	if byte(port) == Z80_VGA_PORT_STATUS {
		b.statusRead++
		if b.statusRead&1 == 0 {
			return VGA_STATUS_VSYNC
		}
		return 0
	}
	return 0
}

func (b *vgaTextSAPTestBus) Out(port uint16, value byte) {
	switch byte(port) {
	case Z80_VGA_PORT_MODE:
		b.vga.HandleWrite(VGA_MODE, uint32(value))
	case Z80_VGA_PORT_CTRL:
		b.vga.HandleWrite(VGA_CTRL, uint32(value))
	case Z80_VGA_PORT_CRTC_IDX:
		b.vga.HandleWrite(VGA_CRTC_INDEX, uint32(value))
	case Z80_VGA_PORT_CRTC_DATA:
		b.vga.HandleWrite(VGA_CRTC_DATA, uint32(value))
		b.displayStart = int(b.vga.GetStartAddress()*2) % len(b.vga.textBuffer)
	case Z80_VGA_PORT_DAC_WIDX:
		b.vga.HandleWrite(VGA_DAC_WINDEX, uint32(value))
	case Z80_VGA_PORT_DAC_DATA:
		b.vga.HandleWrite(VGA_DAC_DATA, uint32(value))
	case Z80_VGA_PORT_DAC_MASK:
		b.vga.HandleWrite(VGA_DAC_MASK, uint32(value))
	}
}

func (b *vgaTextSAPTestBus) Tick(int) {}

type vgaTextSAPCheckpoint struct {
	cpu     [32]byte
	memory  [32]byte
	text    [32]byte
	palette [32]byte
	scene   byte
	done    byte
}

func vgaTextSAPCPUHash(cpu *CPU_Z80) [32]byte {
	var state [48]byte
	copy(state[0:16], []byte{cpu.A, cpu.F, cpu.B, cpu.C, cpu.D, cpu.E, cpu.H, cpu.L, cpu.A2, cpu.F2, cpu.B2, cpu.C2, cpu.D2, cpu.E2, cpu.H2, cpu.L2})
	binary.LittleEndian.PutUint16(state[16:], cpu.IX)
	binary.LittleEndian.PutUint16(state[18:], cpu.IY)
	binary.LittleEndian.PutUint16(state[20:], cpu.SP)
	binary.LittleEndian.PutUint16(state[22:], cpu.PC)
	copy(state[24:27], []byte{cpu.I, cpu.R, cpu.IM})
	binary.LittleEndian.PutUint16(state[28:], cpu.WZ)
	if cpu.IFF1 {
		state[30] = 1
	}
	if cpu.IFF2 {
		state[31] = 1
	}
	if cpu.Halted {
		state[32] = 1
	}
	binary.LittleEndian.PutUint64(state[40:], cpu.Cycles)
	return sha256.Sum256(state[:])
}

func advanceVGATextSAPFrame(t *testing.T, cpu *CPU_Z80, bus *vgaTextSAPTestBus, jit bool) vgaTextSAPCheckpoint {
	t.Helper()
	wantDone := bus.memory[vgaTextSAPFrameDone] + 1
	cpu.SetRunning(true)
	if jit {
		cpu.PerfEnabled = true
		cpu.jitSingleStep = true
		cpu.executionBoundary = func() {
			if bus.memory[vgaTextSAPFrameDone] == wantDone {
				cpu.SetRunning(false)
			}
		}
		cpu.z80JitExecute()
		cpu.executionBoundary = nil
	} else {
		for steps := 0; bus.memory[vgaTextSAPFrameDone] != wantDone && steps < 2_000_000; steps++ {
			cpu.Step()
			cpu.InstructionCount++
		}
		cpu.SetRunning(false)
	}
	if bus.memory[vgaTextSAPFrameDone] != wantDone {
		t.Fatalf("frame watchdog: pc=%04x done=%d want=%d", cpu.PC, bus.memory[vgaTextSAPFrameDone], wantDone)
	}
	palette := make([]byte, 0, 48)
	for index := range 16 {
		r, g, b := bus.vga.GetPaletteEntry(uint8(index))
		palette = append(palette, r, g, b)
	}
	return vgaTextSAPCheckpoint{
		cpu: vgaTextSAPCPUHash(cpu), memory: sha256.Sum256(bus.memory[:]),
		text: sha256.Sum256(bus.vga.textBuffer[:]), palette: sha256.Sum256(palette),
		scene: bus.memory[vgaTextSAPSceneAddress], done: wantDone,
	}
}

func runVGATextSAPFrames(t *testing.T, jit bool, frameCount int) (*CPU_Z80, *vgaTextSAPTestBus, []vgaTextSAPCheckpoint) {
	t.Helper()
	program, err := os.ReadFile("sdk/examples/prebuilt/vga_text_sap_demo.ie80")
	if err != nil {
		t.Fatal(err)
	}
	bus := &vgaTextSAPTestBus{vga: NewVGAEngine(nil)}
	copy(bus.memory[:], program)
	cpu := NewCPU_Z80(bus)
	cpu.jitEnabled = jit
	cpu.Reset()
	cpu.SP = 0xDFF0
	checkpoints := make([]vgaTextSAPCheckpoint, 0, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		checkpoints = append(checkpoints, advanceVGATextSAPFrame(t, cpu, bus, jit))
	}
	return cpu, bus, checkpoints
}

func TestZ80VGATextSAPSourceContract(t *testing.T) {
	source, err := os.ReadFile(vgaTextSAPSource)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	required := []string{
		`.include "ie80.inc"`,
		`ld a,VGA_MODE_TEXT`,
		`out (VGA_PORT_MODE),a`,
		`in a,(VGA_PORT_STATUS)`,
		`out (VGA_PORT_DAC_WIDX),a`,
		`out (VGA_PORT_DAC_DATA),a`,
		`.set CELL_COUNT,80*25`,
		`ld a,25`,
		`ld a,80`,
		`phase_horizontal:`,
		`phase_vertical:`,
		`phase_diagonal:`,
		`phase_cross:`,
		`scene_marker:`,
		`plasma_level:`,
		`logo_level:`,
		`dither_table:`,
		`motion_sine_table:`,
		`motion_angle:`,
		`motion_velocity_x:`,
		`motion_velocity_y:`,
		`motion_x_accumulator:`,
		`motion_y_accumulator:`,
		`motion_x_double_velocity:`,
		`motion_y_double_velocity:`,
		`colour_phase:`,
		`diagonal_wave:`,
		`hue_wave:`,
		`cross_wave:`,
		`update_logo_motion:`,
		`logo_phase_x:`,
		`logo_phase_y:`,
		`logo_position_x:`,
		`logo_position_y:`,
		`logo_fraction_x:`,
		`logo_fraction_y:`,
		`logo_y_tick:`,
		`.set TEXT_PAGE_BYTES,80*25*2`,
		`render_page_base:`,
		`publish_text_page:`,
		`out (VGA_PORT_CRTC_IDX),a`,
		`out (VGA_PORT_CRTC_DATA),a`,
		`.set SAP_DATA_LEN,sap_data_end-sap_data`,
		`.org 0xE000`,
		`sap_data:`,
		`.incbin "../assets/music/Hobbytronic_92_2.sap"`,
		`ld (Z80_POKEY_PLUS),a`,
		`Hobbytronic '92 - 2 by Stephan Duesterhoeft (Benjy)`,
		`sap_data_end:`,
		`ld a,0x05`,
		`ld (Z80_SAP_CTRL),a`,
	}
	for _, fragment := range required {
		if !strings.Contains(text, fragment) {
			t.Errorf("source is missing %q", fragment)
		}
	}
	if strings.Count(text, "in a,(VGA_PORT_STATUS)") < 2 {
		t.Error("vertical blank wait must poll both blanking phases")
	}
	mainLoop := text[strings.Index(text, "main_loop:"):strings.Index(text, "init_sap:")]
	if renderAt, waitAt, publishAt := strings.Index(mainLoop, "call render_plasma"), strings.Index(mainLoop, "call wait_vsync"), strings.Index(mainLoop, "call publish_text_page"); renderAt < 0 || waitAt < renderAt || publishAt < waitAt {
		t.Error("main loop must compose the hidden page before vertical blank and publish it afterwards")
	}
	if strings.Contains(mainLoop, "call update_palette") {
		t.Error("layer transitions must not fade the shared VGA palette")
	}
	publishTextPage := text[strings.Index(text, "publish_text_page:"):strings.Index(text, "init_sap:")]
	if !strings.Contains(publishTextPage, "ld b,0x07") || !strings.Contains(publishTextPage, "ld c,0xD0") {
		t.Error("text page 1 must use the documented 2,000-character CRTC start address")
	}
	if strings.Contains(publishTextPage, "ld b,0x0F") || strings.Contains(publishTextPage, "ld c,0xA0") {
		t.Error("text page 1 uses a byte offset instead of character-cell units")
	}
	renderPlasma := text[strings.Index(text, "render_plasma:"):strings.Index(text, "draw_logo:")]
	if !strings.Contains(renderPlasma, "ld a,(colour_phase)") {
		t.Error("plasma renderer does not apply its private colour phase")
	}
	for _, forbidden := range []string{"VIDEO_MODE", "BLITTER_", "COPPER_", "VGA_MODE_13H", "VGA_MODE_12H", "VGA_MODE_X"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("source uses prohibited graphics hardware token %q", forbidden)
		}
	}
	if matched, err := regexp.Match(`(?s)\.org\s+0xE000\s+sap_data:\s+\.incbin\s+"\.\./assets/music/Hobbytronic_92_2\.sap"\s+sap_data_end:`, source); err != nil || !matched {
		t.Error("SAP data layout does not match the required fixed block")
	}
}

func TestZ80VGATextSAPMotionChangesPlasmaGeometry(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpuA, busA, _ := runVGATextSAPFrames(t, jit, 1)
		cpuB, busB, _ := runVGATextSAPFrames(t, jit, 1)
		for _, bus := range []*vgaTextSAPTestBus{busA, busB} {
			bus.memory[vgaTextSAPFrameLow] = 0
			bus.memory[vgaTextSAPFrameHigh] = 2
			bus.memory[vgaTextSAPMotionAngle] = 64
			bus.memory[0x0F06] = 42
			bus.memory[vgaTextSAPColourPhase] = 3
			binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionXAccum:], 0x2000)
			binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionYAccum:], 0x6000)
		}
		binary.LittleEndian.PutUint16(busB.memory[vgaTextSAPMotionXAccum:], 0x6000)
		binary.LittleEndian.PutUint16(busB.memory[vgaTextSAPMotionYAccum:], 0x2000)
		advanceVGATextSAPFrame(t, cpuA, busA, jit)
		advanceVGATextSAPFrame(t, cpuB, busB, jit)
		glyphsA := make([]byte, VGA_TEXT_COLS*VGA_TEXT_ROWS)
		glyphsB := make([]byte, VGA_TEXT_COLS*VGA_TEXT_ROWS)
		for cell := range glyphsA {
			glyphsA[cell] = busA.vga.textBuffer[busA.displayStart+cell*2]
			glyphsB[cell] = busB.vga.textBuffer[busB.displayStart+cell*2]
		}
		if bytes.Equal(glyphsA, glyphsB) {
			t.Fatalf("JIT=%t motion accumulator changed attributes but not plasma glyph geometry", jit)
		}
	}
}

func TestZ80VGATextSAPVisibleGlyphMotionCadence(t *testing.T) {
	cpu, bus, _ := runVGATextSAPFrames(t, false, 1)
	bus.memory[vgaTextSAPFrameLow] = 0
	bus.memory[vgaTextSAPFrameHigh] = 0
	before := make([]byte, VGA_TEXT_COLS*VGA_TEXT_ROWS)
	for cell := range before {
		before[cell] = bus.vga.textBuffer[bus.displayStart+cell*2]
	}
	for range 8 {
		advanceVGATextSAPFrame(t, cpu, bus, false)
	}
	changed := 0
	for cell := range before {
		if before[cell] != bus.vga.textBuffer[bus.displayStart+cell*2] {
			changed++
		}
	}
	if changed < 200 {
		t.Fatalf("glyph changes across eight displayed frames = %d of %d, want at least 200", changed, len(before))
	}
}

func TestZ80VGATextSAPMotionProducesCoherentCellTranslation(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpuA, busA, _ := runVGATextSAPFrames(t, jit, 1)
		cpuB, busB, _ := runVGATextSAPFrames(t, jit, 1)
		for _, bus := range []*vgaTextSAPTestBus{busA, busB} {
			bus.memory[vgaTextSAPFrameLow] = 0
			bus.memory[vgaTextSAPFrameHigh] = 0
			bus.memory[vgaTextSAPMotionAngle] = 64
			bus.memory[vgaTextSAPColourPhase] = 3
			binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionXAccum:], 0x2000)
			binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionYAccum:], 0x4000)
		}
		binary.LittleEndian.PutUint16(busB.memory[vgaTextSAPMotionXAccum:], 0x2500)
		advanceVGATextSAPFrame(t, cpuA, busA, jit)
		advanceVGATextSAPFrame(t, cpuB, busB, jit)
		matches := 0
		total := 0
		for row := 0; row < 3; row++ {
			for column := 0; column < VGA_TEXT_COLS-1; column++ {
				glyphA, attributeA := vgaTextSAPDisplayedCell(busA, row, column+1)
				glyphB, attributeB := vgaTextSAPDisplayedCell(busB, row, column)
				if glyphA == glyphB && attributeA == attributeB {
					matches++
				}
				total++
			}
		}
		if matches*100 < total*90 {
			t.Fatalf("JIT=%t one-cell translation matched %d of %d cells, want at least 90 per cent", jit, matches, total)
		}
	}
}

func TestZ80VGATextSAPPlasmaColourCycleKeepsDACStable(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpu, bus, checkpoints := runVGATextSAPFrames(t, jit, 1)
		initialPalette := checkpoints[0].palette
		initialColourPhase := bus.memory[vgaTextSAPColourPhase]
		for range 4 {
			checkpoint := advanceVGATextSAPFrame(t, cpu, bus, jit)
			if checkpoint.palette != initialPalette {
				t.Fatalf("JIT=%t plasma colour cycle changed the shared VGA DAC", jit)
			}
		}
		if bus.memory[vgaTextSAPColourPhase] == initialColourPhase {
			t.Fatalf("JIT=%t plasma colour phase did not advance", jit)
		}
	}
}

func TestZ80VGATextSAPSineMotion(t *testing.T) {
	cardinals := []struct {
		angle  byte
		wantVX int8
		wantVY int8
	}{{0, 127, 0}, {64, 0, 127}, {128, -127, 0}, {192, 0, -127}}
	for _, jit := range []bool{false, true} {
		name := "interpreter"
		if jit {
			name = "JIT"
		}
		t.Run(name, func(t *testing.T) {
			cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
			for _, cardinal := range cardinals {
				bus.memory[vgaTextSAPFrameLow] = 0
				bus.memory[vgaTextSAPFrameHigh] = 2
				bus.memory[vgaTextSAPMotionAngle] = cardinal.angle
				binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionXAccum:], 0x4000)
				binary.LittleEndian.PutUint16(bus.memory[vgaTextSAPMotionYAccum:], 0x4000)
				advanceVGATextSAPFrame(t, cpu, bus, jit)
				vx, vy := int8(bus.memory[vgaTextSAPMotionVX]), int8(bus.memory[vgaTextSAPMotionVY])
				if vx != cardinal.wantVX || vy != cardinal.wantVY {
					t.Fatalf("angle %d velocity = (%d,%d), want (%d,%d)", cardinal.angle, vx, vy, cardinal.wantVX, cardinal.wantVY)
				}
				wantXDelta := int16(vx) * 2
				wantYDelta := int16(vy) * 2
				if got := int16(binary.LittleEndian.Uint16(bus.memory[vgaTextSAPMotionXAccum:]) - 0x4000); got != wantXDelta {
					t.Errorf("angle %d horizontal accumulator delta = %d, want %d", cardinal.angle, got, wantXDelta)
				}
				if got := int16(binary.LittleEndian.Uint16(bus.memory[vgaTextSAPMotionYAccum:]) - 0x4000); got != wantYDelta {
					t.Errorf("angle %d vertical accumulator delta = %d, want %d", cardinal.angle, got, wantYDelta)
				}
			}
		})
	}

	t.Run("smooth velocity orbit", func(t *testing.T) {
		cpu, bus, _ := runVGATextSAPFrames(t, false, 1)
		velocities := make([][2]int8, 256)
		for angle := range 256 {
			bus.memory[vgaTextSAPFrameLow] = 0
			bus.memory[vgaTextSAPFrameHigh] = 2
			bus.memory[vgaTextSAPMotionAngle] = byte(angle)
			advanceVGATextSAPFrame(t, cpu, bus, false)
			velocities[angle] = [2]int8{int8(bus.memory[vgaTextSAPMotionVX]), int8(bus.memory[vgaTextSAPMotionVY])}
		}
		for angle := range 256 {
			next := (angle + 1) & 255
			dx := int(velocities[next][0]) - int(velocities[angle][0])
			dy := int(velocities[next][1]) - int(velocities[angle][1])
			if dx < -4 || dx > 4 || dy < -4 || dy > 4 {
				t.Fatalf("velocity discontinuity at angle %d: (%d,%d) to (%d,%d)", angle, velocities[angle][0], velocities[angle][1], velocities[next][0], velocities[next][1])
			}
		}
	})
}

func TestZ80VGATextSAPLogoSineMotion(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
		bus.memory[vgaTextSAPFrameLow] = 0
		bus.memory[vgaTextSAPFrameHigh] = 2
		positions := map[[2]byte]bool{}
		var first, last [2]byte
		for frame := 0; frame < 64; frame++ {
			advanceVGATextSAPFrame(t, cpu, bus, jit)
			position := [2]byte{bus.memory[vgaTextSAPLogoPositionX], bus.memory[vgaTextSAPLogoPositionY]}
			if position[0] > 40 || position[1] > 12 {
				t.Fatalf("JIT=%t frame %d logo position = (%d,%d)", jit, frame, position[0], position[1])
			}
			logoGlyphs := 0
			for row := int(position[1]); row <= int(position[1])+13; row++ {
				for column := int(position[0]); column <= int(position[0])+40; column++ {
					glyph, _ := vgaTextSAPDisplayedCell(bus, row, column)
					if glyph == 0xB0 || glyph == 0xB1 || glyph == 0xDB {
						logoGlyphs++
					}
				}
			}
			if logoGlyphs < 100 {
				t.Fatalf("JIT=%t frame %d logo footprint at (%d,%d) contains only %d logo glyphs", jit, frame, position[0], position[1], logoGlyphs)
			}
			if frame == 0 {
				first = position
			}
			last = position
			positions[position] = true
		}
		if len(positions) < 4 || first == last {
			t.Fatalf("JIT=%t logo positions=%d first=%v last=%v, want smooth displacement", jit, len(positions), first, last)
		}
		if bus.memory[vgaTextSAPLogoPhaseX] == 0 || bus.memory[vgaTextSAPLogoPhaseY] == 0 {
			t.Fatalf("JIT=%t logo sine phases did not advance", jit)
		}
	}
}

func TestZ80VGATextSAPLogoUsesFullTravelArea(t *testing.T) {
	positions := []struct {
		phaseX byte
		phaseY byte
		wantX  byte
		wantY  byte
	}{{64, 64, 40, 12}, {192, 192, 0, 0}}
	for _, jit := range []bool{false, true} {
		cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
		for _, position := range positions {
			bus.memory[vgaTextSAPFrameLow] = 0
			bus.memory[vgaTextSAPFrameHigh] = 2
			bus.memory[vgaTextSAPLogoPhaseX] = position.phaseX
			bus.memory[vgaTextSAPLogoPhaseY] = position.phaseY
			bus.memory[vgaTextSAPLogoYTick] = 0
			advanceVGATextSAPFrame(t, cpu, bus, jit)
			gotX := bus.memory[vgaTextSAPLogoPositionX]
			gotY := bus.memory[vgaTextSAPLogoPositionY]
			if gotX != position.wantX || gotY != position.wantY {
				t.Fatalf("JIT=%t phases (%d,%d) position=(%d,%d), want (%d,%d)", jit, position.phaseX, position.phaseY, gotX, gotY, position.wantX, position.wantY)
			}
			if (gotX == 40 && bus.memory[vgaTextSAPLogoFractionX] != 0) || (gotY == 12 && bus.memory[vgaTextSAPLogoFractionY] != 0) {
				t.Fatalf("JIT=%t edge position retained an unsafe fraction", jit)
			}
		}
	}
}

func TestZ80VGATextSAPLogoEightDirectionCadence(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
		bus.memory[vgaTextSAPFrameLow] = 0
		bus.memory[vgaTextSAPFrameHigh] = 2
		steps := map[[2]bool]bool{}
		for range 12 {
			beforeX := bus.memory[vgaTextSAPLogoPhaseX]
			beforeY := bus.memory[vgaTextSAPLogoPhaseY]
			advanceVGATextSAPFrame(t, cpu, bus, jit)
			step := [2]bool{bus.memory[vgaTextSAPLogoPhaseX] != beforeX, bus.memory[vgaTextSAPLogoPhaseY] != beforeY}
			if step[0] || step[1] {
				steps[step] = true
			}
		}
		for _, want := range [][2]bool{{true, false}, {false, true}, {true, true}} {
			if !steps[want] {
				t.Fatalf("JIT=%t logo phase cadence lacks axis step %v, got %v", jit, want, steps)
			}
		}
	}
}

func TestZ80VGATextSAPLogoStableBetweenCellBoundaries(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
		bus.memory[vgaTextSAPFrameLow] = 0
		bus.memory[vgaTextSAPFrameHigh] = 5
		var previousPosition, previousFraction [2]byte
		var previousLogo [32]byte
		foundStableFractionalStep := false
		for frame := 0; frame < 96; frame++ {
			advanceVGATextSAPFrame(t, cpu, bus, jit)
			position := [2]byte{bus.memory[vgaTextSAPLogoPositionX], bus.memory[vgaTextSAPLogoPositionY]}
			fraction := [2]byte{bus.memory[vgaTextSAPLogoFractionX], bus.memory[vgaTextSAPLogoFractionY]}
			glyphs := make([]byte, 0, 22*VGA_TEXT_COLS)
			for row := 0; row < 22; row++ {
				for column := 0; column < VGA_TEXT_COLS; column++ {
					glyph, _ := vgaTextSAPDisplayedCell(bus, row, column)
					glyphs = append(glyphs, glyph)
				}
			}
			logo := sha256.Sum256(glyphs)
			if frame > 0 && position == previousPosition && fraction != previousFraction {
				if logo != previousLogo {
					t.Fatalf("JIT=%t fractional motion changed logo glyph placement before a cell boundary", jit)
				}
				foundStableFractionalStep = true
				break
			}
			previousPosition, previousFraction, previousLogo = position, fraction, logo
		}
		if !foundStableFractionalStep {
			t.Fatalf("JIT=%t did not observe a fractional step within one cell", jit)
		}
	}
}

func TestZ80VGATextSAPLogoInterpolationKeepsGlyphsCoherent(t *testing.T) {
	cpu, bus, _ := runVGATextSAPFrames(t, false, 1)
	bus.memory[vgaTextSAPFrameLow] = 0
	bus.memory[vgaTextSAPFrameHigh] = 2
	advanceVGATextSAPFrame(t, cpu, bus, false)
	baseX := int(bus.memory[vgaTextSAPLogoPositionX])
	baseY := int(bus.memory[vgaTextSAPLogoPositionY])
	expected := map[int]byte{1: 0xB0, 2: 0xDB, 3: 0xDB, 4: 0xDB, 5: 0xB0, 6: 0xDB, 7: 0xB0, 8: 0xDB, 9: 0xB0}
	coherent := false
	for yOffset := 0; yOffset <= 1; yOffset++ {
		for xOffset := 0; xOffset <= 1; xOffset++ {
			matches := true
			for column, want := range expected {
				glyph, _ := vgaTextSAPDisplayedCell(bus, baseY+yOffset, baseX+xOffset+column)
				if glyph != want {
					matches = false
					break
				}
			}
			coherent = coherent || matches
		}
	}
	if !coherent {
		t.Fatal("sub-cell interpolation broke the logo glyph rows")
	}
}

func TestZ80VGATextSAPShortInitialPlasmaScene(t *testing.T) {
	for _, jit := range []bool{false, true} {
		cpu, bus, _ := runVGATextSAPFrames(t, jit, 1)
		bus.memory[vgaTextSAPFrameLow] = 94
		bus.memory[vgaTextSAPFrameHigh] = 0
		advanceVGATextSAPFrame(t, cpu, bus, jit)
		if bus.memory[vgaTextSAPFrameHigh] != 0 || bus.memory[vgaTextSAPLogoPositionX] == 0 {
			t.Fatalf("JIT=%t initial scene ended before frame 96", jit)
		}
		advanceVGATextSAPFrame(t, cpu, bus, jit)
		if bus.memory[vgaTextSAPFrameHigh] != 1 || bus.memory[vgaTextSAPFrameLow] != 0 || bus.memory[0x0F19] == 0 {
			t.Fatalf("JIT=%t frame 96 state: scene=%d frame=%d logo=%d", jit, bus.memory[vgaTextSAPFrameHigh], bus.memory[vgaTextSAPFrameLow], bus.memory[0x0F19])
		}
	}
}

func vgaTextSAPDisplayedCell(bus *vgaTextSAPTestBus, row, column int) (byte, byte) {
	offset := bus.displayStart + (row*VGA_TEXT_COLS+column)*2
	return bus.vga.textBuffer[offset], bus.vga.textBuffer[offset+1]
}

func vgaTextSAPPalette(vga *VGAEngine) [48]byte {
	var palette [48]byte
	for index := range 16 {
		r, g, b := vga.GetPaletteEntry(uint8(index))
		palette[index*3], palette[index*3+1], palette[index*3+2] = r, g, b
	}
	return palette
}

func vgaTextSAPLayerCounts(bus *vgaTextSAPTestBus) (plasma, logo int, scrollVisible bool) {
	for row := 0; row < 3; row++ {
		for column := 0; column < VGA_TEXT_COLS; column++ {
			character, attribute := vgaTextSAPDisplayedCell(bus, row, column)
			if character != ' ' || attribute != 0 {
				plasma++
			}
		}
	}
	for row := 3; row < 16; row++ {
		for column := 20; column < 60; column++ {
			character, _ := vgaTextSAPDisplayedCell(bus, row, column)
			if character == 0xB0 || character == 0xB1 || character == 0xDB {
				logo++
			}
		}
	}
	for column := 0; column < VGA_TEXT_COLS; column++ {
		character, _ := vgaTextSAPDisplayedCell(bus, 22, column)
		if character != ' ' && character != 0 {
			scrollVisible = true
			break
		}
	}
	return plasma, logo, scrollVisible
}

func TestZ80VGATextSAPIndependentLayerTransitions(t *testing.T) {
	cpu, bus, _ := runVGATextSAPFrames(t, false, 1)
	type expected struct {
		scene       byte
		frame       byte
		plasmaLevel byte
		logoLevel   byte
	}
	stages := []expected{
		{scene: 0, frame: 0x00, plasmaLevel: 16, logoLevel: 0},
		{scene: 1, frame: 0x70, plasmaLevel: 16, logoLevel: 8},
		{scene: 2, frame: 0x00, plasmaLevel: 16, logoLevel: 16},
		{scene: 4, frame: 0xF0, plasmaLevel: 0, logoLevel: 16},
		{scene: 5, frame: 0x70, plasmaLevel: 0, logoLevel: 16},
		{scene: 6, frame: 0x70, plasmaLevel: 8, logoLevel: 8},
	}
	for _, stage := range stages {
		bus.memory[vgaTextSAPFrameLow] = stage.frame
		bus.memory[vgaTextSAPFrameHigh] = stage.scene
		advanceVGATextSAPFrame(t, cpu, bus, false)
		plasma, logo, scroll := vgaTextSAPLayerCounts(bus)
		if bus.memory[0x0F18] != stage.plasmaLevel || bus.memory[0x0F19] != stage.logoLevel {
			t.Errorf("scene %d frame %#02x: plasma level=%d logo level=%d", stage.scene, stage.frame, bus.memory[0x0F18], bus.memory[0x0F19])
		}
		if (plasma != 0) != (stage.plasmaLevel != 0) || !scroll {
			t.Errorf("scene %d frame %#02x: plasma=%d logo=%d scroll=%t", stage.scene, stage.frame, plasma, logo, scroll)
		}
		if stage.plasmaLevel == 0 && (logo != 0) != (stage.logoLevel != 0) {
			t.Errorf("scene %d frame %#02x: logo cells=%d at level %d", stage.scene, stage.frame, logo, stage.logoLevel)
		}
		if stage.plasmaLevel == 0 && stage.logoLevel == 0 {
			t.Errorf("scene %d frame %#02x has no principal visual layer", stage.scene, stage.frame)
		}
	}
	for scene := byte(0); scene < 7; scene++ {
		for step := byte(0); step < 16; step++ {
			bus.memory[vgaTextSAPFrameLow] = step << 4
			bus.memory[vgaTextSAPFrameHigh] = scene
			advanceVGATextSAPFrame(t, cpu, bus, false)
			if bus.memory[0x0F18] == 0 && bus.memory[0x0F19] == 0 {
				t.Fatalf("scene %d transition step %d has no principal visual layer", scene, step)
			}
			_, _, scroll := vgaTextSAPLayerCounts(bus)
			if !scroll {
				t.Fatalf("scene %d transition step %d hides the scrolltext", scene, step)
			}
		}
	}
}

func TestZ80VGATextSAPAssemblesBelowMusic(t *testing.T) {
	assembler, err := exec.LookPath("vasmz80_std")
	if err != nil {
		t.Skip("vasmz80_std is not installed")
	}
	out := filepath.Join(t.TempDir(), "vga_text_sap_demo.ie80")
	cmd := exec.Command(assembler, "-Fbin", "-I", "../../../sdk/include", "-o", out, filepath.Base(vgaTextSAPSource))
	cmd.Dir = filepath.Dir(vgaTextSAPSource)
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assemble demo: %v\n%s", err, result)
	}
	binary, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sap, err := os.ReadFile("sdk/examples/assets/music/Hobbytronic_92_2.sap")
	if err != nil {
		t.Fatal(err)
	}
	if len(binary) != 0xE000+len(sap) {
		t.Fatalf("binary size = %#x, want %#x", len(binary), 0xE000+len(sap))
	}
	if !bytes.Equal(binary[0xE000:], sap) {
		t.Fatal("embedded SAP bytes differ from Hobbytronic_92_2.sap")
	}
}

func TestZ80VGATextSAPInterpreterAndJITParity(t *testing.T) {
	interpreterCPU, interpreterBus, interpreter := runVGATextSAPFrames(t, false, 4)
	nativeCPU, nativeBus, native := runVGATextSAPFrames(t, true, 4)
	for _, scene := range []byte{2, 3, 4, 5, 6} {
		interpreterBus.memory[vgaTextSAPFrameLow] = 0
		interpreterBus.memory[vgaTextSAPFrameHigh] = scene
		nativeBus.memory[vgaTextSAPFrameLow] = 0
		nativeBus.memory[vgaTextSAPFrameHigh] = scene
		interpreter = append(interpreter, advanceVGATextSAPFrame(t, interpreterCPU, interpreterBus, false))
		native = append(native, advanceVGATextSAPFrame(t, nativeCPU, nativeBus, true))
	}
	if len(interpreter) != len(native) {
		t.Fatalf("checkpoint count: interpreter=%d JIT=%d", len(interpreter), len(native))
	}
	for index := range interpreter {
		if interpreter[index] != native[index] {
			t.Fatalf("checkpoint %d mismatch\ninterpreter=%+v\nJIT=%+v", index, interpreter[index], native[index])
		}
	}
	if interpreterBus.visibleWrites != 0 || nativeBus.visibleWrites != 0 {
		t.Fatalf("writes to displayed text page: interpreter=%d JIT=%d", interpreterBus.visibleWrites, nativeBus.visibleWrites)
	}
}

func TestZ80VGATextSAPFrameSemantics(t *testing.T) {
	_, bus, checkpoints := runVGATextSAPFrames(t, false, 4)
	if bus.vga.mode != VGA_MODE_TEXT {
		t.Fatalf("VGA mode = %#x, want Mode 03h", bus.vga.mode)
	}
	if checkpoints[0].text == checkpoints[1].text {
		t.Fatal("consecutive plasma frames are unchanged")
	}
	glyphs := map[byte]bool{}
	foregrounds := map[byte]bool{}
	backgrounds := map[byte]bool{}
	for offset := 0; offset < VGA_TEXT_SIZE; offset += 2 {
		glyphs[bus.vga.textBuffer[offset]] = true
		attribute := bus.vga.textBuffer[offset+1]
		foregrounds[attribute&15] = true
		backgrounds[attribute>>4] = true
	}
	if len(glyphs) < 4 || len(foregrounds) < 6 || len(backgrounds) < 6 {
		t.Fatalf("plasma diversity: glyphs=%d foregrounds=%d backgrounds=%d", len(glyphs), len(foregrounds), len(backgrounds))
	}
	if bus.memory[0xFD09] != 1 || bus.memory[0xFD18] != 0x05 {
		t.Fatalf("SAP state: POKEY+=%#x control=%#x", bus.memory[0xFD09], bus.memory[0xFD18])
	}
}

func TestZ80VGATextSAPNamedScenesAndLoop(t *testing.T) {
	cpu, bus, _ := runVGATextSAPFrames(t, false, 1)

	bus.memory[vgaTextSAPFrameLow] = 0
	bus.memory[vgaTextSAPFrameHigh] = 2
	logo := advanceVGATextSAPFrame(t, cpu, bus, false)
	if logo.scene != 2 {
		t.Fatalf("logo scene marker = %d", logo.scene)
	}
	logoGlyphs := map[byte]int{}
	for row := 3; row < 16; row++ {
		for column := 20; column < 60; column++ {
			logoGlyphs[bus.vga.textBuffer[(row*80+column)*2]]++
		}
	}
	if logoGlyphs[0xDB] < 40 || logoGlyphs[0xB0] < 20 || logoGlyphs[0xB1] < 20 {
		t.Fatalf("logo mask glyph counts = %#v", logoGlyphs)
	}

	bus.memory[vgaTextSAPFrameLow] = 0
	bus.memory[vgaTextSAPFrameHigh] = 3
	beforePointer := uint16(bus.memory[0x0F07]) | uint16(bus.memory[0x0F08])<<8
	for range 4 {
		advanceVGATextSAPFrame(t, cpu, bus, false)
	}
	afterPointer := uint16(bus.memory[0x0F07]) | uint16(bus.memory[0x0F08])<<8
	if afterPointer == beforePointer {
		t.Fatal("scroller pointer did not advance")
	}
	for cell := 22 * 80; cell < 24*80; cell++ {
		if bus.vga.textBuffer[cell*2] == 0 {
			t.Fatalf("scroller left cell %d uninitialised", cell)
		}
	}
	var scroller strings.Builder
	for cell := 22 * 80; cell < 23*80; cell++ {
		scroller.WriteByte(bus.vga.textBuffer[cell*2])
	}
	if !strings.Contains(scroller.String(), "WELCOME TO THE INTUITION ENGINE") {
		t.Fatalf("scroller row = %q", scroller.String())
	}
	duplicateGlyphs := 0
	for column := 0; column < VGA_TEXT_COLS; column++ {
		upper := bus.vga.textBuffer[(22*VGA_TEXT_COLS+column)*2]
		lower := bus.vga.textBuffer[(23*VGA_TEXT_COLS+column)*2]
		if upper == lower {
			duplicateGlyphs++
		}
	}
	if duplicateGlyphs == VGA_TEXT_COLS {
		t.Fatal("scroller is duplicated on the row below")
	}

	wantPalette := vgaTextSAPPalette(bus.vga)
	bus.memory[vgaTextSAPFrameLow] = 0xEF
	bus.memory[vgaTextSAPFrameHigh] = 6
	advanceVGATextSAPFrame(t, cpu, bus, false)
	if gotPalette := vgaTextSAPPalette(bus.vga); gotPalette != wantPalette {
		t.Fatal("layer transition changed the shared VGA palette")
	}
	if bus.memory[0x0F18] == 0 && bus.memory[0x0F19] == 0 {
		t.Fatal("loop transition has no principal visual layer")
	}

	bus.memory[vgaTextSAPFrameLow] = 0xFF
	bus.memory[vgaTextSAPFrameHigh] = 6
	restarted := advanceVGATextSAPFrame(t, cpu, bus, false)
	if restarted.scene != 0 || bus.memory[vgaTextSAPFrameLow] != 0 || bus.memory[vgaTextSAPFrameHigh] != 0 {
		t.Fatalf("loop restart state: scene=%d frame=%02x%02x", restarted.scene, bus.memory[vgaTextSAPFrameHigh], bus.memory[vgaTextSAPFrameLow])
	}
}

func TestZ80VGATextSAPRealAdapterRenders(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/prebuilt/vga_text_sap_demo.ie80")
	if err != nil {
		t.Fatal(err)
	}
	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1, vga.HandleTextRead, vga.HandleTextWrite)
	adapter := NewZ80BusAdapterWithVGA(bus, vga)
	for address, value := range program {
		bus.Write8(uint32(address), value)
	}
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.Reset()
	cpu.SetRunning(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for cpu.Running() {
			vga.SetVSync(false)
			time.Sleep(100 * time.Microsecond)
			vga.SetVSync(true)
			time.Sleep(100 * time.Microsecond)
		}
	}()
	cpu.PerfEnabled = true
	cpu.jitSingleStep = true
	cpu.executionBoundary = func() {
		if bus.Read8(vgaTextSAPFrameDone) != 0 {
			cpu.SetRunning(false)
		}
	}
	executionDone := make(chan struct{})
	go func() {
		cpu.z80JitExecute()
		close(executionDone)
	}()
	select {
	case <-executionDone:
	case <-time.After(5 * time.Second):
		cpu.SetRunning(false)
		<-executionDone
		t.Fatal("real-adapter native renderer timed out")
	}
	cpu.executionBoundary = nil
	<-done
	if bus.Read8(vgaTextSAPFrameDone) == 0 {
		t.Fatalf("real-adapter renderer watchdog: pc=%04x", cpu.PC)
	}
	populated := 0
	for offset := 0; offset < VGA_TEXT_SIZE; offset += 2 {
		if vga.textBuffer[offset] != 0 || vga.textBuffer[offset+1] != 0 {
			populated++
		}
	}
	wantCells := VGA_TEXT_COLS * VGA_TEXT_ROWS
	if populated != wantCells {
		t.Fatalf("real VGA text cells populated = %d, want %d", populated, wantCells)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("real-adapter renderer did not enter native Z80 code")
	}
}
