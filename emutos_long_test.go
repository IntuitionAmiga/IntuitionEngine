//go:build headless && m68k_test

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestEmuTOS_LongBoot(t *testing.T) {
	romData, err := os.ReadFile("etos256us.img")
	if err != nil {
		t.Skipf("EmuTOS ROM not found: %v", err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)

	// Match GUI behaviour: unmap VRAM so EmuTOS heap writes go to bus memory directly
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Let it boot for 15 seconds — well past where the GUI crashes
	time.Sleep(15 * time.Second)
	cpu.running.Store(false)
	time.Sleep(50 * time.Millisecond)

	runPtr := cpu.Read32(0x63FC)
	t.Logf("After 15s: PC=%08X, _run=%08X, SR=%04X", cpu.PC, runPtr, cpu.SR)

	if runPtr == 0 || runPtr > 0x02000000 {
		t.Errorf("_run pointer is invalid: %08X", runPtr)
	}
	if !cpu.running.Load() {
		t.Logf("CPU has stopped (PC=%08X)", cpu.PC)
	}
}

// TestEmuTOS_WatchPalette uses a write watchpoint on ie_vdi_palette[0] (0x74DC)
// to trace all writes and find what sets it to black after init_colors sets it to white.
func TestEmuTOS_WatchPalette(t *testing.T) {
	romData, err := os.ReadFile("etos256us.img")
	if err != nil {
		t.Skipf("EmuTOS ROM not found: %v", err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleWrite8)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	// Watch ie_vdi_palette[0..7] at 0x74DC..0x74FB
	const paletteBase = 0x74DC
	const watchEnd = paletteBase + 8*4 // 8 entries * 4 bytes
	writeCount := 0
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		// Check if this write overlaps with palette[0..7]
		writeEnd := addr + uint32(size)
		if addr < watchEnd && writeEnd > paletteBase {
			idx := int(addr-paletteBase) / 4
			if writeCount < 200 {
				fmt.Printf("PALETTE WRITE #%d: addr=%08X val=%08X size=%d pc=%08X (entry ~%d) D0=%08X D1=%08X D2=%08X D3=%08X A0=%08X\n",
					writeCount, addr, value, size, pc, idx,
					cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3], cpu.AddrRegs[0])
			}
			writeCount++
		}
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	time.Sleep(5 * time.Second)
	cpu.running.Store(false)
	time.Sleep(50 * time.Millisecond)

	t.Logf("Total writes to palette[0..7]: %d", writeCount)
	// Dump final palette values
	for i := range 16 {
		val := cpu.Read32(paletteBase + uint32(i*4))
		t.Logf("  ie_vdi_palette[%2d] = %08X", i, val)
	}
}

// TestEmuTOS_TraceCorruption enables instruction tracing and dumps the trace
// when a bus error occurs, showing the instruction sequence leading to corruption.
func TestEmuTOS_TraceCorruption(t *testing.T) {
	romData, err := os.ReadFile("etos256us.img")
	if err != nil {
		t.Skipf("EmuTOS ROM not found: %v", err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)

	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	// Enable instruction tracing
	cpu.DebugTraceEnabled = true

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Poll for bus faults (not ILLEGAL exceptions which happen during normal MOVEC probes)
	ticker := time.NewTicker(5 * time.Millisecond)
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			faultCount := cpu.faultLogCount.Load()
			if faultCount >= 1 {
				// Bus fault happened — wait briefly for CPU to settle
				time.Sleep(5 * time.Millisecond)
				cpu.running.Store(false)
				time.Sleep(10 * time.Millisecond)

				fmt.Printf("\n*** Bus fault #%d detected, dumping trace ***\n", faultCount)
				fmt.Printf("PC=%08X SR=%04X SP=%08X\n", cpu.PC, cpu.SR, cpu.AddrRegs[7])
				fmt.Printf("D: %08X %08X %08X %08X %08X %08X %08X %08X\n",
					cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
					cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7])
				fmt.Printf("A: %08X %08X %08X %08X %08X %08X %08X %08X\n",
					cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
					cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7])
				cpu.DumpDebugTrace(128)

				t.Logf("Trace dumped after bus fault")
				ticker.Stop()
				return
			}
		case <-timeout:
			cpu.running.Store(false)
			time.Sleep(50 * time.Millisecond)
			t.Logf("No bus fault in 30 seconds — boot succeeded!")
			t.Logf("PC=%08X SR=%04X SP=%08X", cpu.PC, cpu.SR, cpu.AddrRegs[7])
			t.Logf("osmem(0x4578)=%08X _run(0x63FC)=%08X",
				cpu.Read32(0x4578), cpu.Read32(0x63FC))
			ticker.Stop()
			return
		}
	}
}

// TestEmuTOS_BlitterEnabled verifies that the blitter works correctly in
// directVRAM mode (as used by EmuTOS), writing to busMemory instead of
// frontBuffer so that GetFrame() returns visible output.
func TestEmuTOS_BlitterEnabled(t *testing.T) {
	romData, err := os.ReadFile("etos256us.img")
	if err != nil {
		t.Skipf("EmuTOS ROM not found: %v", err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)

	// Match production setup: unmap VRAM I/O and use directVRAM
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+VRAM_SIZE])

	// Map video chip registers for blitter access
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Let EmuTOS boot for 5 seconds
	time.Sleep(5 * time.Second)

	// Now test blitter directly: fill a small VRAM region
	fillColor := uint32(0xFF00FF00)          // green with full alpha
	fillDst := uint32(VRAM_START + 2560*100) // row 100

	bus.Write32(BLT_OP, 1) // bltOpFill
	bus.Write32(BLT_DST, fillDst)
	bus.Write32(BLT_WIDTH, 8)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_DST_STRIDE, 2560)
	bus.Write32(BLT_COLOR, fillColor)
	bus.Write32(BLT_CTRL, 1) // bltCtrlStart

	// Wait for blitter to complete
	for i := range 1000 {
		_ = i
		status := video.HandleRead(BLT_STATUS)
		if status&2 == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Verify blitter wrote to busMemory (directVRAM path)
	for y := range 4 {
		for x := range 8 {
			addr := fillDst + uint32(y*2560+x*4)
			got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
			if got != fillColor {
				t.Fatalf("EmuTOS blitter fill at (%d,%d): got 0x%08X, want 0x%08X", x, y, got, fillColor)
			}
		}
	}

	cpu.running.Store(false)
	time.Sleep(50 * time.Millisecond)
	t.Logf("Blitter directVRAM path verified during EmuTOS boot")
}
