package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
	"unsafe"
)

func buildTestROM(size int, sp uint32, pc uint32) []byte {
	rom := make([]byte, size)
	rom[0] = byte(sp >> 24)
	rom[1] = byte(sp >> 16)
	rom[2] = byte(sp >> 8)
	rom[3] = byte(sp)
	rom[4] = byte(pc >> 24)
	rom[5] = byte(pc >> 16)
	rom[6] = byte(pc >> 8)
	rom[7] = byte(pc)
	return rom
}

func TestEmuTOSLoader_LoadROM_192K(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	rom := buildTestROM(emutosROM192K, 0x00120000, emutosBase192+0x100)

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
	if got := bus.Read8(emutosBase192); got != rom[0] {
		t.Fatalf("ROM not loaded at 0x%X, got first byte 0x%02X", emutosBase192, got)
	}
	if got := cpu.Read32(0); got != emutosBootSP {
		t.Fatalf("boot SSP mismatch: got 0x%08X want 0x%08X", got, emutosBootSP)
	}
}

func TestEmuTOSLoader_LoadROM_256K(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	rom := buildTestROM(emutosROM256K, 0x00130000, emutosBaseStd+0x120)

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
	if got := bus.Read8(emutosBaseStd); got != rom[0] {
		t.Fatalf("ROM not loaded at 0x%X, got first byte 0x%02X", emutosBaseStd, got)
	}
}

func TestEmuTOSLoader_VectorSetup(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	wantSP := uint32(0x00123456)
	wantPC := uint32(0x00ABCDEF)
	rom := buildTestROM(emutosROM192K, wantSP, wantPC)

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
	if cpu.AddrRegs[7] != emutosBootSP {
		t.Fatalf("A7 got 0x%08X want 0x%08X", cpu.AddrRegs[7], emutosBootSP)
	}
	if cpu.PC != wantPC {
		t.Fatalf("PC got 0x%08X want 0x%08X", cpu.PC, wantPC)
	}
}

func TestEmuTOSLoader_ROMPageNoIOCollision(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	rom := buildTestROM(emutosROM192K, 0x1000, 0x2000)

	base := emutosROMBase(len(rom))
	startPage := base >> 8
	endPage := (base + uint32(len(rom)) - 1) >> 8
	for p := startPage; p <= endPage; p++ {
		if bus.ioPageBitmap[p] {
			t.Fatalf("unexpected io mapping on ROM page 0x%X", p)
		}
	}

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
}

func TestEmuTOSLoader_TimerFires(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	defer loader.Stop()

	// Install valid interrupt vectors so arming check passes
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	cpu.Write32(uint32(M68K_VEC_LEVEL5)*4, 0x00001000)
	cpu.SetRunning(true)
	cpu.pendingInterrupt.Store(0)
	loader.StartTimer()

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.pendingInterrupt.Load() != 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timer did not assert any interrupt bits")
}

func TestEmuTOSLoader_TimerSurvivesMonitorPause(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)
	defer loader.Stop()

	// Install valid interrupt vectors so arming check passes
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	cpu.Write32(uint32(M68K_VEC_LEVEL5)*4, 0x00001000)
	loader.StartTimer()
	cpu.pendingInterrupt.Store(0)
	cpu.SetRunning(false)
	time.Sleep(30 * time.Millisecond)
	if got := cpu.pendingInterrupt.Load(); got != 0 {
		t.Fatalf("pending interrupt should stay zero while paused, got 0x%X", got)
	}

	cpu.SetRunning(true)
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.pendingInterrupt.Load() != 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timer did not resume after monitor pause")
}

func TestEmuTOSLoader_TimerStopsOnCancel(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, nil)

	loader.StartTimer()
	time.Sleep(10 * time.Millisecond)
	loader.Stop()
	stable := cpu.pendingInterrupt.Load()
	time.Sleep(30 * time.Millisecond)
	if got := cpu.pendingInterrupt.Load(); got != stable {
		t.Fatalf("pending interrupt changed after Stop: before 0x%X after 0x%X", stable, got)
	}
}

func TestEmuTOSBoot(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
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

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Let it boot for 3 seconds
	time.Sleep(3 * time.Second)

	t.Logf("CPU running: %v, PC=%08X, A7=%08X, SR=%04X",
		cpu.Running(), cpu.PC, cpu.AddrRegs[7], cpu.SR)
}

func TestEmuTOSBoot_VDIRendering(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
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

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Let EmuTOS boot fully — GEM desktop takes ~2s
	time.Sleep(5 * time.Second)

	// Stop CPU before reading memory to avoid races
	cpu.running.Store(false)
	time.Sleep(50 * time.Millisecond) // let CPU loop exit

	// --- System variable dump ---
	vbasad := cpu.Read32(0x44E) // v_bas_ad: framebuffer base
	t.Logf("System: v_bas_ad=%08X, PC=%08X, SR=%04X", vbasad, cpu.PC, cpu.SR)

	// Critical: verify CPU detection worked — _longframe must be 1 for 68020 frame handling
	longframe := cpu.Read16(0x59E) // _longframe from tosvars.ld
	mcpu := cpu.Read32(0x2B2C)     // _mcpu from emutos.map
	runPtr := cpu.Read32(0x63FC)   // _run from emutos.map
	t.Logf("CPU detection: _longframe=%d, _mcpu=%d, _run=%08X", longframe, mcpu, runPtr)

	// Dump ie_vdi_palette[0..15] from EmuTOS memory (at 0x74DC)
	paletteAddr := uint32(0x74DC) // _ie_vdi_palette from emutos.map
	for i := range 16 {
		val := cpu.Read32(paletteAddr + uint32(i*4))
		t.Logf("  ie_vdi_palette[%2d] = %08X", i, val)
	}

	// Scan memory for palette init signature (pen 0 = 0xFFFFFFFF white)
	// ie_rgba(255,255,255,255) = 0xFFFFFFFF, ie_rgba(0,0,0,255) = 0x000000FF
	t.Log("Scanning for palette signature (0xFFFFFFFF followed by 0x000000FF):")
	for addr := uint32(0x1000); addr < 0x20000; addr += 4 {
		v0 := cpu.Read32(addr)
		v1 := cpu.Read32(addr + 4)
		if v0 == 0xFFFFFFFF && v1 == 0x000000FF {
			t.Logf("  Palette signature found at 0x%04X", addr)
			// Dump first 16 entries
			for j := range 16 {
				t.Logf("    [%2d] = %08X", j, cpu.Read32(addr+uint32(j*4)))
			}
			break
		}
	}

	// Check v_planes and numcolors at correct EmuTOS addresses
	t.Logf("  v_planes (0x20C6) = %d", cpu.Read16(0x20C6))
	t.Logf("  numcolors: DEV_TAB[13] (0x1E12+26=0x1E2C) = %d", cpu.Read16(0x1E2C))
	t.Logf("  INQ_TAB[4] (0x1DB8+8=0x1DC0) = %d", cpu.Read16(0x1DC0))
	// Check MAP_COL[0..3]
	mapColAddr := uint32(0x7ADC) // from ROM disasm
	for i := range 4 {
		t.Logf("  MAP_COL[%d] (0x%04X) = %d", i, mapColAddr+uint32(i*2), cpu.Read16(mapColAddr+uint32(i*2)))
	}
	// Check REQ_COL[0..2] (first 3 entries of st_palette: white, black, red)
	// REQ_COL at 0x1E74, each entry is 3 WORDs (6 bytes)
	reqColAddr := uint32(0x1E74)
	for i := range 3 {
		base := reqColAddr + uint32(i*6)
		r := cpu.Read16(base)
		g := cpu.Read16(base + 2)
		b := cpu.Read16(base + 4)
		t.Logf("  REQ_COL[%d] = {%d, %d, %d}", i, r, g, b)
	}
	// Check gl_nplanes (AES variable) - search for it in map
	// _gl_nplanes from map
	t.Logf("  gl_nplanes check: scanning at common AES BSS addresses...")
	for _, addr := range []uint32{0x2100, 0x3000, 0x4000, 0x5000, 0x6000} {
		for off := uint32(0); off < 256; off += 2 {
			v := cpu.Read16(addr + off)
			if v == 32 {
				t.Logf("    value 32 at 0x%04X", addr+off)
			}
		}
	}

	// Sample pixels from desktop area (y=100, various x positions)
	for _, x := range []int{10, 100, 200, 320, 500} {
		off := 100*640*4 + x*4
		r, g, b, a := video.frontBuffer[off], video.frontBuffer[off+1], video.frontBuffer[off+2], video.frontBuffer[off+3]
		t.Logf("  pixel(%d,100) = R=%02X G=%02X B=%02X A=%02X", x, r, g, b, a)
	}
	// Sample pixels from menu bar area
	for _, x := range []int{10, 100, 200} {
		off := 5*640*4 + x*4
		r, g, b, a := video.frontBuffer[off], video.frontBuffer[off+1], video.frontBuffer[off+2], video.frontBuffer[off+3]
		t.Logf("  pixel(%d,5)   = R=%02X G=%02X B=%02X A=%02X", x, r, g, b, a)
	}

	// --- Direct frontBuffer analysis ---
	fb := video.frontBuffer
	width := 640
	height := 480
	stride := width * 4 // RGBA

	// Count non-zero pixels across entire framebuffer
	totalPixels := width * height
	nonZeroPixels := 0
	colorHist := make(map[uint32]int) // RGBA → count
	for y := range height {
		for x := range width {
			off := y*stride + x*4
			r, g, b, a := fb[off], fb[off+1], fb[off+2], fb[off+3]
			rgba := uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)
			if rgba != 0 {
				nonZeroPixels++
				colorHist[rgba]++
			}
		}
	}

	t.Logf("Framebuffer: %d/%d non-zero pixels (%.1f%%)",
		nonZeroPixels, totalPixels, float64(nonZeroPixels)*100/float64(totalPixels))

	// Top 10 colors
	type colorCount struct {
		rgba  uint32
		count int
	}
	var colors []colorCount
	for rgba, count := range colorHist {
		colors = append(colors, colorCount{rgba, count})
	}
	for i := range colors {
		for j := i + 1; j < len(colors); j++ {
			if colors[j].count > colors[i].count {
				colors[i], colors[j] = colors[j], colors[i]
			}
		}
	}
	t.Logf("Distinct colors: %d", len(colors))
	for i, c := range colors {
		if i >= 10 {
			break
		}
		r := byte(c.rgba >> 24)
		g := byte(c.rgba >> 16)
		b := byte(c.rgba >> 8)
		a := byte(c.rgba)
		t.Logf("  #%d: R=%02X G=%02X B=%02X A=%02X count=%d (%.1f%%)",
			i+1, r, g, b, a, c.count, float64(c.count)*100/float64(totalPixels))
	}

	// Row-by-row nonzero pixel density (thumbnail: every 20th row)
	t.Logf("Row density (every 20th row, 80 cols = 8px each):")
	for y := 0; y < height; y += 20 {
		var line [80]byte
		for col := range 80 {
			count := 0
			for dx := range 8 {
				x := col*8 + dx
				off := y*stride + x*4
				if fb[off] != 0 || fb[off+1] != 0 || fb[off+2] != 0 || fb[off+3] != 0 {
					count++
				}
			}
			if count == 0 {
				line[col] = '.'
			} else if count < 4 {
				line[col] = ':'
			} else if count < 7 {
				line[col] = '#'
			} else {
				line[col] = '@'
			}
		}
		t.Logf("  y=%3d: %s", y, string(line[:]))
	}

	// Check first few rows for menu bar content (GEM desktop)
	menuBarPixels := 0
	for y := range 20 {
		for x := range width {
			off := y*stride + x*4
			if fb[off+3] != 0 { // any pixel with alpha > 0
				menuBarPixels++
			}
		}
	}
	t.Logf("Menu bar area (rows 0-19): %d/%d pixels with alpha > 0 (%.1f%%)",
		menuBarPixels, 20*width, float64(menuBarPixels)*100/float64(20*width))

	// Check middle of screen for desktop fill
	midPixels := 0
	for y := 200; y < 300; y++ {
		for x := range width {
			off := y*stride + x*4
			if fb[off+3] != 0 {
				midPixels++
			}
		}
	}
	t.Logf("Mid-screen area (rows 200-299): %d/%d pixels with alpha > 0 (%.1f%%)",
		midPixels, 100*width, float64(midPixels)*100/float64(100*width))

	// Show white pixel locations (text) per row — find rows with white pixels
	t.Logf("Rows with WHITE pixels (text/content):")
	for y := range height {
		whiteInRow := 0
		firstX, lastX := -1, -1
		for x := range width {
			off := y*stride + x*4
			if fb[off] == 0xFF && fb[off+1] == 0xFF && fb[off+2] == 0xFF {
				whiteInRow++
				if firstX < 0 {
					firstX = x
				}
				lastX = x
			}
		}
		if whiteInRow > 0 {
			t.Logf("  y=%3d: %d white px, x=[%d..%d]", y, whiteInRow, firstX, lastX)
		}
	}

	// Text reconstruction: scan for 8x8 character cells with white pixels
	// Try to decode text from font bitmap patterns
	t.Logf("White pixel bitmap (rows with content, 1=white 0=other, 8px cols):")
	for y := range height {
		hasWhite := false
		for x := range width {
			off := y*stride + x*4
			if fb[off] == 0xFF && fb[off+1] == 0xFF && fb[off+2] == 0xFF {
				hasWhite = true
				break
			}
		}
		if !hasWhite {
			continue
		}
		var row [80]byte
		for col := range 80 {
			bits := byte(0)
			for dx := range 8 {
				x := col*8 + dx
				off := y*stride + x*4
				if fb[off] == 0xFF {
					bits |= 1 << (7 - dx)
				}
			}
			if bits == 0 {
				row[col] = ' '
			} else {
				row[col] = '#'
			}
		}
		t.Logf("  y=%3d: |%s|", y, string(row[:]))
	}

	// Per-pixel row dump (rows 0-15 at native res, showing 0-200 px in 1px=1char)
	t.Logf("Pixel-level dump (rows 0-15, first 200px: .=black W=white):")
	for y := range 16 {
		var row [200]byte
		for x := range 200 {
			off := y*stride + x*4
			if fb[off] == 0xFF && fb[off+1] == 0xFF && fb[off+2] == 0xFF {
				row[x] = 'W'
			} else if fb[off+3] != 0 {
				row[x] = '.'
			} else {
				row[x] = ' '
			}
		}
		t.Logf("  y=%2d: |%s|", y, string(row[:]))
	}

	// Check VBL clock growth (diagnostic only — framebuffer is the real signal)
	vbclock := cpu.Read32(0x462)
	t.Logf("VBL clock (_vbclock at 0x462): %d", vbclock)

	// Verdict
	if nonZeroPixels == 0 {
		t.Error("FAIL: framebuffer is completely empty — VDI rendered nothing")
	} else if nonZeroPixels < totalPixels/10 {
		t.Errorf("WARN: only %d non-zero pixels — VDI rendering may be incomplete", nonZeroPixels)
	}

	// GEM desktop indicator: more than 2 distinct colors means we're past the
	// black+white splash screen into the full GEM desktop with colored fills
	if len(colors) > 2 {
		t.Logf("GEM desktop detected: %d distinct colors (splash has only black+white)", len(colors))
	} else {
		t.Logf("NOTE: only %d distinct colors — may still be on splash screen", len(colors))
	}
}

func TestEmuTOSBoot_Mouse(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
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

	term := NewTerminalMMIO()
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, term.HandleRead, term.HandleWrite)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Wait for GEM desktop to fully initialize
	time.Sleep(5 * time.Second)

	// EmuTOS map symbols (from emutos.map)
	const addrGCURX = uint32(0x1E6C)
	const addrGCURY = uint32(0x1E6E)
	const addrDEV_TAB = uint32(0x1E12)
	const addrMOUSE_BT = uint32(0x1E72)

	devTab0 := int16(cpu.Read16(addrDEV_TAB))
	devTab1 := int16(cpu.Read16(addrDEV_TAB + 2))
	if devTab0 != 639 || devTab1 != 479 {
		t.Errorf("DEV_TAB mismatch: [0]=%d (want 639), [1]=%d (want 479)", devTab0, devTab1)
	}

	// Test Y-axis: inject position (400, 300)
	term.mouseX.Store(400)
	term.mouseY.Store(300)
	time.Sleep(500 * time.Millisecond)

	gcurx := int16(cpu.Read16(addrGCURX))
	gcury := int16(cpu.Read16(addrGCURY))
	if gcurx != 400 {
		t.Errorf("GCURX=%d, want 400", gcurx)
	}
	if gcury != 300 {
		t.Errorf("GCURY=%d, want 300", gcury)
	}

	// Test full Y range: inject (100, 450)
	term.mouseX.Store(100)
	term.mouseY.Store(450)
	time.Sleep(500 * time.Millisecond)

	gcurx = int16(cpu.Read16(addrGCURX))
	gcury = int16(cpu.Read16(addrGCURY))
	if gcurx != 100 {
		t.Errorf("GCURX=%d, want 100", gcurx)
	}
	if gcury != 450 {
		t.Errorf("GCURY=%d, want 450", gcury)
	}

	// Test left button
	term.mouseButtons.Store(1)
	time.Sleep(300 * time.Millisecond)
	mouseBt := int16(cpu.Read16(addrMOUSE_BT))
	if mouseBt != 1 {
		t.Errorf("MOUSE_BT=%d after left press, want 1", mouseBt)
	}

	// Test button release
	term.mouseButtons.Store(0)
	time.Sleep(300 * time.Millisecond)
	mouseBt = int16(cpu.Read16(addrMOUSE_BT))
	if mouseBt != 0 {
		t.Errorf("MOUSE_BT=%d after release, want 0", mouseBt)
	}

	// Test right button
	term.mouseButtons.Store(2)
	time.Sleep(300 * time.Millisecond)
	mouseBt = int16(cpu.Read16(addrMOUSE_BT))
	if mouseBt != 2 {
		t.Errorf("MOUSE_BT=%d after right press, want 2", mouseBt)
	}

	term.mouseButtons.Store(0)
}

func TestMMIO_Mouse_Read16(t *testing.T) {
	bus := NewMachineBus()
	tm := NewTerminalMMIO()
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, tm.HandleRead, tm.HandleWrite)

	cpu := NewM68KCPU(bus)

	// Store known values in the terminal MMIO atomics
	tm.mouseX.Store(400)
	tm.mouseY.Store(300)
	tm.mouseButtons.Store(1) // left button

	// Test Read16 (what EmuTOS ie_mouse_x/y/buttons actually does)
	gotX := cpu.Read16(MOUSE_X)
	gotY := cpu.Read16(MOUSE_Y)
	gotBtn := cpu.Read16(MOUSE_BUTTONS)

	t.Logf("Read16(MOUSE_X=0x%X) = %d (0x%04X)", MOUSE_X, gotX, gotX)
	t.Logf("Read16(MOUSE_Y=0x%X) = %d (0x%04X)", MOUSE_Y, gotY, gotY)
	t.Logf("Read16(MOUSE_BUTTONS=0x%X) = %d (0x%04X)", MOUSE_BUTTONS, gotBtn, gotBtn)

	if gotX != 400 {
		t.Errorf("Read16(MOUSE_X) = %d, want 400", gotX)
	}
	if gotY != 300 {
		t.Errorf("Read16(MOUSE_Y) = %d, want 300", gotY)
	}
	if gotBtn != 1 {
		t.Errorf("Read16(MOUSE_BUTTONS) = %d, want 1", gotBtn)
	}

	// Also test Read32 path
	gotX32 := cpu.Read32(MOUSE_X)
	gotY32 := cpu.Read32(MOUSE_Y)
	gotBtn32 := cpu.Read32(MOUSE_BUTTONS)

	t.Logf("Read32(MOUSE_X) = %d (0x%08X)", gotX32, gotX32)
	t.Logf("Read32(MOUSE_Y) = %d (0x%08X)", gotY32, gotY32)
	t.Logf("Read32(MOUSE_BUTTONS) = %d (0x%08X)", gotBtn32, gotBtn32)

	// Test Read8 path (check byte ordering)
	b0 := cpu.Read8(MOUSE_Y)
	b1 := cpu.Read8(MOUSE_Y + 1)
	b2 := cpu.Read8(MOUSE_Y + 2)
	b3 := cpu.Read8(MOUSE_Y + 3)
	t.Logf("Read8 bytes at MOUSE_Y: [%02X %02X %02X %02X]", b0, b1, b2, b3)

	// Test with larger Y value near screen bottom
	tm.mouseY.Store(479)
	gotY = cpu.Read16(MOUSE_Y)
	t.Logf("Read16(MOUSE_Y) after Store(479) = %d (0x%04X)", gotY, gotY)
	if gotY != 479 {
		t.Errorf("Read16(MOUSE_Y) = %d, want 479", gotY)
	}

	// Test with right button
	tm.mouseButtons.Store(2)
	gotBtn = cpu.Read16(MOUSE_BUTTONS)
	t.Logf("Read16(MOUSE_BUTTONS) after Store(2) = %d (0x%04X)", gotBtn, gotBtn)
	if gotBtn != 2 {
		t.Errorf("Read16(MOUSE_BUTTONS) = %d, want 2", gotBtn)
	}

	// Test MOUSE_STATUS (changed flag)
	tm.mouseChanged.Store(true)
	gotStatus := cpu.Read16(MOUSE_STATUS)
	t.Logf("Read16(MOUSE_STATUS) with changed=true = %d", gotStatus)
	if gotStatus != 1 {
		t.Errorf("Read16(MOUSE_STATUS) = %d, want 1", gotStatus)
	}
	// Second read should clear the flag
	gotStatus = cpu.Read16(MOUSE_STATUS)
	t.Logf("Read16(MOUSE_STATUS) after second read = %d", gotStatus)
	if gotStatus != 0 {
		t.Errorf("Read16(MOUSE_STATUS) second read = %d, want 0 (cleared)", gotStatus)
	}
}

func TestMMIO_Mouse_FullIOSetup(t *testing.T) {
	// Reproduce the EXACT I/O mapping from main.go emutos mode
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)

	tm := NewTerminalMMIO()

	// Map in the same order as main.go
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleWrite8)
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, tm.HandleRead, tm.HandleWrite)

	// Stub PSG region for test (minimal bus setup)
	bus.MapIO(PSG_BASE, PSG_PLUS_CTRL+1,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) {})

	cpu := NewM68KCPU(bus)

	// Store known values
	tm.mouseX.Store(400)
	tm.mouseY.Store(300)
	tm.mouseButtons.Store(1)

	// Read via M68K CPU Read16 (same path as ie_mouse_y)
	gotX := cpu.Read16(MOUSE_X)
	gotY := cpu.Read16(MOUSE_Y)
	gotBtn := cpu.Read16(MOUSE_BUTTONS)

	t.Logf("Full I/O setup: Read16(MOUSE_X)=%d, Read16(MOUSE_Y)=%d, Read16(MOUSE_BUTTONS)=%d", gotX, gotY, gotBtn)

	if gotX != 400 {
		t.Errorf("Read16(MOUSE_X) = %d, want 400", gotX)
	}
	if gotY != 300 {
		t.Errorf("Read16(MOUSE_Y) = %d, want 300", gotY)
	}
	if gotBtn != 1 {
		t.Errorf("Read16(MOUSE_BUTTONS) = %d, want 1", gotBtn)
	}

	// Also test that HandleRead is called (not just memory)
	// by changing the atomic AFTER the previous read and reading again
	tm.mouseY.Store(479)
	gotY2 := cpu.Read16(MOUSE_Y)
	t.Logf("After mouseY.Store(479): Read16(MOUSE_Y)=%d", gotY2)
	if gotY2 != 479 {
		t.Errorf("Read16(MOUSE_Y) after Store(479) = %d, want 479", gotY2)
	}
}

func TestEmuTOSBoot_Diagnostics(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)

	// Match interactive mode: unmap VRAM I/O, use direct bus memory
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleWrite8)
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	video.SetBusMemory(bus.memory)
	frameSize := 640 * 480 * 4
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+frameSize])

	term := NewTerminalMMIO()
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, term.HandleRead, term.HandleWrite)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Wait for GEM desktop to fully initialize
	time.Sleep(5 * time.Second)

	// EmuTOS map symbols (from emutos.map)
	const (
		addrVBasAd    = uint32(0x44E) // v_bas_ad (4 bytes)
		addrDEV_TAB   = uint32(0x1E12)
		addrGCURX     = uint32(0x1E6C)
		addrGCURY     = uint32(0x1E6E)
		addrMOUSE_BT  = uint32(0x1E72)
		addrV_REZ_HZ  = uint32(0x20BA)
		addrV_REZ_VT  = uint32(0x20C2)
		addrBYTES_LIN = uint32(0x20C4)
		addrVPlanes   = uint32(0x20C6)
		addrVLinWr    = uint32(0x20C8)
		addrSshiftmod = uint32(0x44C) // sshiftmod (1 byte)
	)

	// Read all diagnostic values
	vBasAd := cpu.Read32(addrVBasAd)
	devTab0 := int16(cpu.Read16(addrDEV_TAB))
	devTab1 := int16(cpu.Read16(addrDEV_TAB + 2))
	gcurx := int16(cpu.Read16(addrGCURX))
	gcury := int16(cpu.Read16(addrGCURY))
	vRezHz := int16(cpu.Read16(addrV_REZ_HZ))
	vRezVt := int16(cpu.Read16(addrV_REZ_VT))
	bytesLin := int16(cpu.Read16(addrBYTES_LIN))
	vPlanes := int16(cpu.Read16(addrVPlanes))
	vLinWr := int16(cpu.Read16(addrVLinWr))
	sshiftmod := cpu.Read8(addrSshiftmod)

	t.Logf("=== EmuTOS Boot Diagnostics ===")
	t.Logf("sshiftmod    = %d", sshiftmod)
	t.Logf("v_bas_ad     = 0x%08X", vBasAd)
	t.Logf("V_REZ_HZ     = %d", vRezHz)
	t.Logf("V_REZ_VT     = %d", vRezVt)
	t.Logf("v_planes     = %d", vPlanes)
	t.Logf("BYTES_LIN    = %d", bytesLin)
	t.Logf("v_lin_wr     = %d", vLinWr)
	t.Logf("DEV_TAB[0]   = %d (xres)", devTab0)
	t.Logf("DEV_TAB[1]   = %d (yres)", devTab1)
	t.Logf("GCURX        = %d", gcurx)
	t.Logf("GCURY        = %d", gcury)

	// Read more DEV_TAB entries
	for i := range 10 {
		v := int16(cpu.Read16(addrDEV_TAB + uint32(i*2)))
		t.Logf("DEV_TAB[%d]   = %d", i, v)
	}

	// Verify key values
	if devTab0 != 639 {
		t.Errorf("DEV_TAB[0] = %d, want 639", devTab0)
	}
	if devTab1 != 479 {
		t.Errorf("DEV_TAB[1] = %d, want 479", devTab1)
	}
	if vRezHz != 640 {
		t.Errorf("V_REZ_HZ = %d, want 640", vRezHz)
	}
	if vRezVt != 480 {
		t.Errorf("V_REZ_VT = %d, want 480", vRezVt)
	}

	// Now test mouse movement to full Y range
	term.mouseX.Store(320)
	term.mouseY.Store(400)
	time.Sleep(500 * time.Millisecond)

	gcurx = int16(cpu.Read16(addrGCURX))
	gcury = int16(cpu.Read16(addrGCURY))
	t.Logf("After mouse(320,400): GCURX=%d, GCURY=%d", gcurx, gcury)
	if gcury < 350 {
		t.Errorf("GCURY=%d after setting mouseY=400, should be near 400", gcury)
	}

	// === VRAM scan: check icon text area for rendered pixels ===
	// Icons are at approximately (20,433) and (580,433), 32x32.
	// Icon text labels are below: roughly y=465..475 for each.
	// VRAM layout: 640x480x32bpp, v_lin_wr=2560 bytes/line, base=0x100000.
	// M68K writes big-endian, stored byte-swapped in bus.memory.
	// Black pixel: ie_rgba(0,0,0,0xFF)=0x000000FF → swapped→0xFF000000
	// White pixel: 0xFFFFFFFF → swapped→0xFFFFFFFF
	// Green(3): ie_rgba(0,255,0,0xFF)=0x00FF00FF → swapped→0xFF00FF00
	t.Log("=== VRAM icon text area scan ===")
	be := binary.BigEndian
	// Icons are at y=433, 32px tall → icon ends at y=465.
	// Text labels should be at y>=465.
	for _, iconInfo := range []struct {
		name string
		x0   int
		x1   int
	}{
		{"Trash", 0, 120},
		{"Printer", 540, 640},
	} {
		blackCount := 0
		whiteCount := 0
		greenCount := 0
		otherCount := 0
		var firstBlackX, firstBlackY int
		for y := 465; y < 480; y++ {
			for x := iconInfo.x0; x < iconInfo.x1; x++ {
				addr := VRAM_START + uint32(y)*2560 + uint32(x)*4
				pixel := be.Uint32(bus.memory[addr : addr+4])
				switch pixel {
				case 0x000000FF: // black (with alpha)
					if blackCount == 0 {
						firstBlackX = x
						firstBlackY = y
					}
					blackCount++
				case 0xFFFFFFFF: // white
					whiteCount++
				case 0x00FF00FF: // green (desktop bg)
					greenCount++
				default:
					if otherCount < 5 {
						t.Logf("  [%s] other pixel at (%d,%d): 0x%08X", iconInfo.name, x, y, pixel)
					}
					otherCount++
				}
			}
		}
		t.Logf("[%s] y=465..479 x=%d..%d: black=%d white=%d green=%d other=%d",
			iconInfo.name, iconInfo.x0, iconInfo.x1-1, blackCount, whiteCount, greenCount, otherCount)
		if blackCount > 0 {
			t.Logf("  first black at (%d,%d)", firstBlackX, firstBlackY)
		} else {
			t.Logf("  NO black text pixels found in text area!")
		}
	}

	// Save VRAM as PNG for visual inspection
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := range 480 {
		for x := range 640 {
			addr := VRAM_START + uint32(y)*2560 + uint32(x)*4
			pixel := be.Uint32(bus.memory[addr : addr+4])
			r := uint8(pixel >> 24)
			g := uint8(pixel >> 16)
			b := uint8(pixel >> 8)
			a := uint8(pixel)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	pngPath := "/tmp/emutos_vram.png"
	f, perr := os.Create(pngPath)
	if perr == nil {
		_ = png.Encode(f, img)
		f.Close()
		t.Logf("VRAM saved to %s", pngPath)
	}

	// === Button injection test ===
	// Read initial button state from EmuTOS memory
	const addrCurMsStat = uint32(0x1F6A)
	initMsStat := bus.memory[addrCurMsStat]
	initMouseBT := be.Uint16(bus.memory[addrMOUSE_BT : addrMOUSE_BT+2])
	t.Logf("Before click: cur_ms_stat=0x%02X MOUSE_BT=%d", initMsStat, initMouseBT)

	// Move mouse to a known position, then inject left button press
	term.mouseX.Store(100)
	term.mouseY.Store(100)
	time.Sleep(200 * time.Millisecond) // let position settle

	term.mouseButtons.Store(1)         // left button pressed
	time.Sleep(200 * time.Millisecond) // let EmuTOS Timer C process it

	afterPressMsStat := bus.memory[addrCurMsStat]
	afterPressMouseBT := be.Uint16(bus.memory[addrMOUSE_BT : addrMOUSE_BT+2])
	t.Logf("After press:  cur_ms_stat=0x%02X MOUSE_BT=%d", afterPressMsStat, afterPressMouseBT)

	term.mouseButtons.Store(0) // release
	time.Sleep(200 * time.Millisecond)

	afterRelMsStat := bus.memory[addrCurMsStat]
	afterRelMouseBT := be.Uint16(bus.memory[addrMOUSE_BT : addrMOUSE_BT+2])
	t.Logf("After release: cur_ms_stat=0x%02X MOUSE_BT=%d", afterRelMsStat, afterRelMouseBT)

	if afterPressMouseBT == 0 && afterPressMsStat == initMsStat {
		t.Errorf("Button press had NO effect on EmuTOS state")
	}

	// Dump terminal output
	term.mu.Lock()
	termOut := string(term.outputBuf)
	term.mu.Unlock()
	if len(termOut) > 0 {
		t.Logf("=== EmuTOS terminal output (%d bytes) ===", len(termOut))
		t.Logf("%s", termOut)
	} else {
		t.Logf("No EmuTOS terminal output captured")
	}

	// === AES event chain diagnostics ===
	// Check that user_but/user_mot/user_cur point to the right AES handlers
	const (
		addrUserBut = uint32(0x208C) // user_but function pointer
		addrUserCur = uint32(0x2090) // user_cur function pointer
		addrUserMot = uint32(0x2094) // user_mot function pointer
		addrXrat    = uint32(0xD108) // xrat (AES mouse X)
		addrYrat    = uint32(0xD106) // yrat (AES mouse Y)
		addrPrXrat  = uint32(0xD0FC) // pr_xrat (previous)
		addrPrYrat  = uint32(0xD0FA) // pr_yrat (previous)
	)
	userBut := be.Uint32(bus.memory[addrUserBut : addrUserBut+4])
	userCur := be.Uint32(bus.memory[addrUserCur : addrUserCur+4])
	userMot := be.Uint32(bus.memory[addrUserMot : addrUserMot+4])
	xratVal := int16(be.Uint16(bus.memory[addrXrat : addrXrat+2]))
	yratVal := int16(be.Uint16(bus.memory[addrYrat : addrYrat+2]))
	t.Logf("=== AES Event Chain ===")
	t.Logf("user_but  = 0x%08X (expect far_bcha=0x00E17170)", userBut)
	t.Logf("user_mot  = 0x%08X (expect far_mcha=0x00E17196)", userMot)
	t.Logf("user_cur  = 0x%08X", userCur)
	t.Logf("xrat=%d yrat=%d (AES mouse pos)", xratVal, yratVal)
	if userBut != 0x00E17170 {
		t.Logf("WARNING: user_but is NOT far_bcha — AES button events won't be delivered!")
	}
	if userMot != 0x00E17196 {
		t.Logf("WARNING: user_mot is NOT far_mcha — AES motion events won't be delivered!")
	}

	// === Menu bar click test ===
	// Click on the "Desk" menu and check if VRAM changes (dropdown appears)
	t.Logf("=== Menu Bar Click Test ===")

	// First release any button and move to neutral position
	term.mouseButtons.Store(0)
	term.mouseX.Store(320)
	term.mouseY.Store(240)
	time.Sleep(300 * time.Millisecond) // let position settle and button clear

	// Now move to Desk menu position
	term.mouseX.Store(35)
	term.mouseY.Store(8)
	time.Sleep(300 * time.Millisecond) // let cursor arrive

	// Read GCURX/GCURY and xrat/yrat to verify both VDI and AES positions
	gcurxNow := int16(cpu.Read16(addrGCURX))
	gcuryNow := int16(cpu.Read16(addrGCURY))
	xratNow := int16(be.Uint16(bus.memory[addrXrat : addrXrat+2]))
	yratNow := int16(be.Uint16(bus.memory[addrYrat : addrYrat+2]))
	t.Logf("Before menu click: GCURX=%d GCURY=%d xrat=%d yrat=%d", gcurxNow, gcuryNow, xratNow, yratNow)
	if gcurxNow != xratNow || gcuryNow != yratNow {
		t.Logf("WARNING: GCURX/Y != xrat/yrat — AES motion callback may not be working!")
	}

	// Snapshot the dropdown area (y=19 to y=120, x=0 to x=200)
	// This area should change when the Desk menu opens
	dropdownArea := make([]byte, 200*4*102) // 200 pixels wide * 102 rows
	for y := 19; y <= 120; y++ {
		srcOff := VRAM_START + uint32(y)*2560
		dstOff := (y - 19) * 200 * 4
		copy(dropdownArea[dstOff:dstOff+200*4], bus.memory[srcOff:srcOff+200*4])
	}

	// Check AES button delay state BEFORE clicking
	const (
		addrGlBdely    = uint32(0xD0E8) // gl_bdely
		addrGlBtrue    = uint32(0xD0EA) // gl_btrue
		addrGlBdesired = uint32(0xD0EC) // gl_bdesired
		addrGlBclick   = uint32(0xD0EE) // gl_bclick
		addrGlDclick   = uint32(0x9622) // gl_dclick (double-click delay)
		addrGlCtmown   = uint32(0x9612) // gl_ctmown (ctrl manager owns mouse)
		addrGlMowner   = uint32(0xD0F4) // gl_mowner (mouse owner process ptr)
		addrButton     = uint32(0xD10A) // button (current button state in AES)
		addrRlr        = uint32(0xCE86) // rlr (run list root)
		addrFpcnt      = uint32(0xCE8A) // fpcnt approx (need to find exact)
	)
	_ = addrFpcnt // may be wrong address
	// Check menu bar rectangle (GRECT: x, y, w, h each WORD)
	const addrGlRmenu = uint32(0x974C) // gl_rmenu (menu bar GRECT)
	rmenuX := int16(be.Uint16(bus.memory[addrGlRmenu : addrGlRmenu+2]))
	rmenuY := int16(be.Uint16(bus.memory[addrGlRmenu+2 : addrGlRmenu+4]))
	rmenuW := int16(be.Uint16(bus.memory[addrGlRmenu+4 : addrGlRmenu+6]))
	rmenuH := int16(be.Uint16(bus.memory[addrGlRmenu+6 : addrGlRmenu+8]))
	t.Logf("gl_rmenu (menu bar rect): x=%d y=%d w=%d h=%d", rmenuX, rmenuY, rmenuW, rmenuH)
	t.Logf("Click target (35,8) inside gl_rmenu? x in [%d,%d], y in [%d,%d]",
		rmenuX, rmenuX+rmenuW-1, rmenuY, rmenuY+rmenuH-1)

	// Check ctl_pd (screen manager process descriptor)
	const addrCtlPd = uint32(0xD0F0) // ctl_pd
	ctlPd := be.Uint32(bus.memory[addrCtlPd : addrCtlPd+4])
	glMowner := be.Uint32(bus.memory[addrGlMowner : addrGlMowner+4])
	t.Logf("ctl_pd=0x%08X gl_mowner=0x%08X (same=%v)", ctlPd, glMowner, ctlPd == glMowner)

	glDclick := int16(be.Uint16(bus.memory[addrGlDclick : addrGlDclick+2]))
	glBdelyPre := int16(be.Uint16(bus.memory[addrGlBdely : addrGlBdely+2]))
	glBtruePre := int16(be.Uint16(bus.memory[addrGlBtrue : addrGlBtrue+2]))
	glCtmownPre := int16(be.Uint16(bus.memory[addrGlCtmown : addrGlCtmown+2]))
	aesButton := int16(be.Uint16(bus.memory[addrButton : addrButton+2]))
	t.Logf("Pre-click: gl_dclick=%d gl_bdely=%d gl_btrue=%d gl_ctmown=%d", glDclick, glBdelyPre, glBtruePre, glCtmownPre)
	t.Logf("Pre-click: gl_mowner=0x%08X button=%d", glMowner, aesButton)

	// Read screen manager's CDA and check c_bsleep
	// AESPD.p_cda is at offset 0x14, CDA.c_bsleep is at offset 0x0A
	pCda := be.Uint32(bus.memory[ctlPd+0x14 : ctlPd+0x14+4])
	cBsleep := be.Uint32(bus.memory[pCda+0x0A : pCda+0x0A+4])
	cMsleep := be.Uint32(bus.memory[pCda+0x06 : pCda+0x06+4])
	pName := string(bus.memory[ctlPd+0x0C : ctlPd+0x0C+8])
	pStat := be.Uint16(bus.memory[ctlPd+0x1E : ctlPd+0x1E+2])
	pEvwait := be.Uint16(bus.memory[ctlPd+0x22 : ctlPd+0x22+2])
	pEvflg := be.Uint16(bus.memory[ctlPd+0x24 : ctlPd+0x24+2])
	t.Logf("ctl_pd process: name='%s' stat=0x%04X evwait=0x%04X evflg=0x%04X",
		pName, pStat, pEvwait, pEvflg)
	t.Logf("ctl_pd CDA: p_cda=0x%08X c_bsleep=0x%08X c_msleep=0x%08X", pCda, cBsleep, cMsleep)
	if cBsleep == 0 {
		t.Logf("WARNING: ctl_pd has NO pending button event wait (c_bsleep=NULL)!")
		t.Logf("This means button clicks to the screen manager are silently dropped!")
	}

	// Click: press left button
	preMsStat := bus.memory[addrCurMsStat]
	term.mouseButtons.Store(1)
	time.Sleep(100 * time.Millisecond) // let button reach mouse_int quickly

	// Check gl_bdely IMMEDIATELY after press (before delay expires)
	glBdelyImmediate := int16(be.Uint16(bus.memory[addrGlBdely : addrGlBdely+2]))
	postMsStat := bus.memory[addrCurMsStat]
	t.Logf("Immediate after press: gl_bdely=%d cur_ms_stat=0x%02X", glBdelyImmediate, postMsStat)

	if glBdelyImmediate > 0 {
		t.Logf("Double-click delay active! b_delay must be called %d more times", glBdelyImmediate)
		t.Logf("If tikcod is not connected to the timer chain, the delay will NEVER expire!")
	}

	// Wait for potential delay to expire
	time.Sleep(1000 * time.Millisecond)

	glBdelyPost := int16(be.Uint16(bus.memory[addrGlBdely : addrGlBdely+2]))
	aesButtonPost := int16(be.Uint16(bus.memory[addrButton : addrButton+2]))
	glCtmownPost := int16(be.Uint16(bus.memory[addrGlCtmown : addrGlCtmown+2]))
	glMownerPost := be.Uint32(bus.memory[addrGlMowner : addrGlMowner+4])
	t.Logf("After 1s wait: gl_bdely=%d button=%d gl_ctmown=%d gl_mowner=0x%08X",
		glBdelyPost, aesButtonPost, glCtmownPost, glMownerPost)

	if glBdelyPost > 0 {
		t.Errorf("gl_bdely STUCK at %d — tikcod/b_delay not being called!", glBdelyPost)
	}
	if aesButtonPost == 0 {
		t.Logf("WARNING: AES 'button' var is still 0 — bchange may not have been called!")
	}

	t.Logf("Menu click: cur_ms_stat before=0x%02X after=0x%02X", preMsStat, postMsStat)

	// Release button
	term.mouseButtons.Store(0)
	time.Sleep(200 * time.Millisecond)

	// Compare VRAM in dropdown area
	changedPixels := 0
	for y := 19; y <= 120; y++ {
		srcOff := VRAM_START + uint32(y)*2560
		dstOff := (y - 19) * 200 * 4
		for x := range 200 {
			oldPx := be.Uint32(dropdownArea[dstOff+x*4 : dstOff+x*4+4])
			newPx := be.Uint32(bus.memory[srcOff+uint32(x*4) : srcOff+uint32(x*4)+4])
			if oldPx != newPx {
				changedPixels++
			}
		}
	}
	t.Logf("Menu dropdown area: %d pixels changed out of %d", changedPixels, 200*102)

	if changedPixels == 0 {
		t.Logf("WARNING: No VRAM changes in dropdown area — menu may not have opened")
	} else {
		t.Logf("Menu click produced VRAM changes — GEM event processing works")
	}

	// Save post-click VRAM as PNG
	imgPost := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := range 480 {
		for x := range 640 {
			addr := VRAM_START + uint32(y)*2560 + uint32(x)*4
			pixel := be.Uint32(bus.memory[addr : addr+4])
			r := uint8(pixel >> 24)
			g := uint8(pixel >> 16)
			b := uint8(pixel >> 8)
			a := uint8(pixel)
			imgPost.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	pngPostPath := "/tmp/emutos_vram_after_click.png"
	fPost, perrPost := os.Create(pngPostPath)
	if perrPost == nil {
		_ = png.Encode(fPost, imgPost)
		fPost.Close()
		t.Logf("Post-click VRAM saved to %s", pngPostPath)
	}
}

func TestEmuTOS_MenuClick(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
	}

	bus := NewMachineBus()
	video, verr := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if verr != nil {
		t.Fatalf("NewVideoChip failed: %v", verr)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	video.SetBusMemory(bus.memory)
	frameSize := 640 * 480 * 4
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+frameSize])

	term := NewTerminalMMIO()
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, term.HandleRead, term.HandleWrite)

	cpu := NewM68KCPU(bus)
	loader := NewEmuTOSLoader(bus, cpu, video)
	if err := loader.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	loader.StartTimer()
	defer loader.Stop()

	go cpu.ExecuteInstruction()
	defer cpu.running.Store(false)

	// Wait for GEM desktop to fully initialize
	time.Sleep(5 * time.Second)

	be := binary.BigEndian

	rdW := func(addr uint32) int16 {
		return int16(be.Uint16(bus.memory[addr : addr+2]))
	}
	rdL := func(addr uint32) uint32 {
		return be.Uint32(bus.memory[addr : addr+4])
	}

	const (
		addrGlMntree = uint32(0xD120)
		addrGlCtwait = uint32(0x9614)
		addrCtlPd    = uint32(0xD0F0)
	)

	// Verify menu tree is installed
	glMntree := rdL(addrGlMntree)
	if glMntree == 0 {
		t.Fatalf("gl_mntree is NULL — menu bar not installed")
	}

	// Verify SCRENMGR CDA wait lists are populated (regression for FAKE_EVB bug).
	// GCC 13 -mshort generated wrong displacement in evinsert, causing c_iiowait
	// and c_bsleep to never be written. With the fix, these must be non-zero.
	ctlPd := rdL(addrCtlPd)
	pCda := rdL(ctlPd + 0x14)
	cIiowait := rdL(pCda + 0x02)
	cMsleep := rdL(pCda + 0x06)
	cBsleep := rdL(pCda + 0x0A)
	t.Logf("CDA: c_iiowait=0x%08X c_msleep=0x%08X c_bsleep=0x%08X", cIiowait, cMsleep, cBsleep)

	if cIiowait == 0 {
		t.Errorf("c_iiowait is NULL — evinsert keyboard wait list not populated")
	}
	if cBsleep == 0 {
		t.Errorf("c_bsleep is NULL — evinsert button wait list not populated")
	}

	// Read gl_ctwait to find menu bar area for mouse targeting
	ctwaitX := rdW(addrGlCtwait + 2)
	ctwaitY := rdW(addrGlCtwait + 4)
	ctwaitW := rdW(addrGlCtwait + 6)
	ctwaitH := rdW(addrGlCtwait + 8)

	// Snapshot menu bar VRAM before entering it
	barSnapshot := make([]byte, 640*4*20)
	copy(barSnapshot, bus.memory[VRAM_START:VRAM_START+640*4*20])

	// Move mouse into menu bar title area
	targetX := int32(ctwaitX) + int32(ctwaitW)/2
	targetY := int32(ctwaitY) + int32(ctwaitH)/2
	if targetX <= 0 || targetY <= 0 {
		thebarBase := glMntree + 1*24
		theactiveBase := glMntree + 2*24
		targetX = int32(rdW(thebarBase+16)) + int32(rdW(theactiveBase+16)) + int32(rdW(theactiveBase+20))/2
		targetY = int32(rdW(thebarBase+18)) + int32(rdW(theactiveBase+18)) + int32(rdW(theactiveBase+22))/2
	}
	t.Logf("Moving mouse to menu bar: (%d, %d)", targetX, targetY)

	term.mouseX.Store(targetX)
	term.mouseY.Store(targetY)
	time.Sleep(1 * time.Second)

	// Verify menu bar title highlight changed in VRAM
	barChanged := 0
	for i := range len(barSnapshot) / 4 {
		old := be.Uint32(barSnapshot[i*4 : i*4+4])
		cur := be.Uint32(bus.memory[VRAM_START+uint32(i*4) : VRAM_START+uint32(i*4)+4])
		if old != cur {
			barChanged++
		}
	}
	t.Logf("Title bar VRAM: %d pixels changed", barChanged)
}

func TestEmuTOSLoader_PixelFormat(t *testing.T) {
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)

	cpu := NewM68KCPU(bus)
	cpu.Write32(VRAM_START, 0x11223344)

	got := video.frontBuffer[:4]
	want := []byte{0x11, 0x22, 0x33, 0x44}
	for i := range 4 {
		if got[i] != want[i] {
			t.Fatalf("pixel byte %d got 0x%02X want 0x%02X", i, got[i], want[i])
		}
	}
}

// TestEmuTOSBoot_DirectVRAM boots EmuTOS using the same directVRAM path as
// interactive mode and compares the framebuffer against the HandleWrite path.
func TestEmuTOSBoot_DirectVRAM(t *testing.T) {
	romPath := "etos256us.img"
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("EmuTOS ROM not found at %s: %v", romPath, err)
	}

	// --- Path A: HandleWrite (frontBuffer) ---
	busA := NewMachineBus()
	videoA, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip A failed: %v", err)
	}
	videoA.AttachBus(busA)
	videoA.SetBigEndianMode(true)
	busA.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, videoA.HandleRead, videoA.HandleWrite)
	busA.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, videoA.HandleWrite8)

	cpuA := NewM68KCPU(busA)
	loaderA := NewEmuTOSLoader(busA, cpuA, videoA)
	if err := loaderA.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM A: %v", err)
	}
	loaderA.StartTimer()
	go cpuA.ExecuteInstruction()
	time.Sleep(5 * time.Second)
	cpuA.running.Store(false)
	loaderA.Stop()
	time.Sleep(50 * time.Millisecond)

	// --- Path B: directVRAM (bus memory) ---
	busB := NewMachineBus()
	videoB, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip B failed: %v", err)
	}
	videoB.AttachBus(busB)
	videoB.SetBigEndianMode(true)
	busB.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	videoB.SetBusMemory(busB.memory)
	frameSize := 640 * 480 * 4
	videoB.SetDirectVRAM(busB.memory[VRAM_START : VRAM_START+frameSize])

	cpuB := NewM68KCPU(busB)
	loaderB := NewEmuTOSLoader(busB, cpuB, videoB)
	if err := loaderB.LoadROM(romData); err != nil {
		t.Fatalf("LoadROM B: %v", err)
	}
	loaderB.StartTimer()
	go cpuB.ExecuteInstruction()
	time.Sleep(5 * time.Second)
	cpuB.running.Store(false)
	loaderB.Stop()
	time.Sleep(50 * time.Millisecond)

	// --- Compare framebuffers ---
	fbA := videoA.frontBuffer
	fbB := busB.memory[VRAM_START : VRAM_START+frameSize]

	width := 640
	stride := width * 4

	// Check drvbits
	drvbitsA := binary.BigEndian.Uint32(busA.memory[0x4C2:0x4C6])
	drvbitsB := binary.BigEndian.Uint32(busB.memory[0x4C2:0x4C6])
	t.Logf("_drvbits: pathA=%08X pathB=%08X", drvbitsA, drvbitsB)

	// Compare pixel by pixel
	mismatches := 0
	firstMismatchY := -1
	for y := range 480 {
		for x := range 640 {
			off := y*stride + x*4
			if fbA[off] != fbB[off] || fbA[off+1] != fbB[off+1] ||
				fbA[off+2] != fbB[off+2] || fbA[off+3] != fbB[off+3] {
				mismatches++
				if firstMismatchY < 0 {
					firstMismatchY = y
					t.Logf("First mismatch at (%d,%d): A=[%02X,%02X,%02X,%02X] B=[%02X,%02X,%02X,%02X]",
						x, y, fbA[off], fbA[off+1], fbA[off+2], fbA[off+3],
						fbB[off], fbB[off+1], fbB[off+2], fbB[off+3])
				}
			}
		}
	}
	t.Logf("Total pixel mismatches: %d / %d", mismatches, 640*480)

	// Scan directVRAM for non-dithered-pattern regions (icons/text)
	// The GEM desktop dither alternates white(FFFFFFFF) and green(00FF00FF)
	white := [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
	green := [4]byte{0x00, 0xFF, 0x00, 0xFF}
	black := [4]byte{0x00, 0x00, 0x00, 0xFF}
	zero := [4]byte{0, 0, 0, 0}

	// Count pixel types per row in directVRAM
	t.Logf("DirectVRAM pixel analysis (non-dither rows):")
	for y := range 480 {
		nWhite, nGreen, nBlack, nZero, nOther := 0, 0, 0, 0, 0
		for x := range 640 {
			off := y*stride + x*4
			px := [4]byte{fbB[off], fbB[off+1], fbB[off+2], fbB[off+3]}
			switch px {
			case white:
				nWhite++
			case green:
				nGreen++
			case black:
				nBlack++
			case zero:
				nZero++
			default:
				nOther++
			}
		}
		// Report rows that have black pixels (icons, text) or other unusual pixels
		if nBlack > 0 || nOther > 0 || nZero > 0 {
			t.Logf("  y=%3d: white=%d green=%d black=%d zero=%d other=%d",
				y, nWhite, nGreen, nBlack, nZero, nOther)
		}
	}

	// Save directVRAM (path B) as raw RGBA file for visual inspection
	if err := os.WriteFile("/tmp/emutos_directvram.rgba", fbB, 0644); err != nil {
		t.Logf("Failed to write directVRAM dump: %v", err)
	} else {
		t.Logf("Saved directVRAM to /tmp/emutos_directvram.rgba (640x480 RGBA)")
	}
	// Also save HandleWrite path (path A)
	if err := os.WriteFile("/tmp/emutos_handlewrite.rgba", fbA, 0644); err != nil {
		t.Logf("Failed to write HandleWrite dump: %v", err)
	} else {
		t.Logf("Saved HandleWrite buffer to /tmp/emutos_handlewrite.rgba (640x480 RGBA)")
	}

	// Check compositor pipeline: simulate what the compositor does
	// Clear finalFrame, blend directVRAM, save result
	compositorFrame := make([]byte, frameSize)
	for y := range 480 {
		for x := range 640 {
			srcIdx := y*stride + x*4
			srcPixel := *(*uint32)(unsafe.Pointer(&fbB[srcIdx]))
			if srcPixel&0xFF000000 != 0 {
				*(*uint32)(unsafe.Pointer(&compositorFrame[srcIdx])) = srcPixel
			}
		}
	}
	if err := os.WriteFile("/tmp/emutos_compositor.rgba", compositorFrame, 0644); err != nil {
		t.Logf("Failed to write compositor dump: %v", err)
	} else {
		t.Logf("Saved compositor output to /tmp/emutos_compositor.rgba (640x480 RGBA)")
	}

	// Compare compositor output with directVRAM
	compMismatches := 0
	for i := range frameSize {
		if compositorFrame[i] != fbB[i] {
			compMismatches++
		}
	}
	t.Logf("Compositor vs directVRAM mismatched bytes: %d (pixels with alpha=0 would be zeroed)", compMismatches)
}
