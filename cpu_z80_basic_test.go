package main

import "testing"

func TestZ80ResetDefaults(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.A = 0x11
	cpu.F = 0x22
	cpu.B = 0x33
	cpu.C = 0x44
	cpu.D = 0x55
	cpu.E = 0x66
	cpu.H = 0x77
	cpu.L = 0x88
	cpu.A2 = 0x99
	cpu.F2 = 0xAA
	cpu.B2 = 0xBB
	cpu.C2 = 0xCC
	cpu.D2 = 0xDD
	cpu.E2 = 0xEE
	cpu.H2 = 0xFF
	cpu.L2 = 0x01
	cpu.IX = 0x1234
	cpu.IY = 0x4567
	cpu.SP = 0xABCD
	cpu.PC = 0xFEED
	cpu.I = 0x12
	cpu.R = 0x34
	cpu.IM = 2
	cpu.WZ = 0x2222
	cpu.IFF1 = true
	cpu.IFF2 = true
	cpu.irqLine.Store(true)
	cpu.nmiLine.Store(true)
	cpu.nmiPending.Store(true)
	cpu.iffDelay = 1
	cpu.irqVector.Store(0x00)
	cpu.Halted = true
	cpu.Cycles = 999

	cpu.Reset()

	requireZ80EqualU16(t, "PC", cpu.PC, 0x0000)
	requireZ80EqualU16(t, "SP", cpu.SP, 0xFFFF)
	requireZ80EqualU8(t, "A", cpu.A, 0x00)
	requireZ80EqualU8(t, "F", cpu.F, 0x00)
	requireZ80EqualU8(t, "B", cpu.B, 0x00)
	requireZ80EqualU8(t, "C", cpu.C, 0x00)
	requireZ80EqualU8(t, "D", cpu.D, 0x00)
	requireZ80EqualU8(t, "E", cpu.E, 0x00)
	requireZ80EqualU8(t, "H", cpu.H, 0x00)
	requireZ80EqualU8(t, "L", cpu.L, 0x00)
	requireZ80EqualU8(t, "A'", cpu.A2, 0x00)
	requireZ80EqualU8(t, "F'", cpu.F2, 0x00)
	requireZ80EqualU8(t, "B'", cpu.B2, 0x00)
	requireZ80EqualU8(t, "C'", cpu.C2, 0x00)
	requireZ80EqualU8(t, "D'", cpu.D2, 0x00)
	requireZ80EqualU8(t, "E'", cpu.E2, 0x00)
	requireZ80EqualU8(t, "H'", cpu.H2, 0x00)
	requireZ80EqualU8(t, "L'", cpu.L2, 0x00)
	requireZ80EqualU16(t, "IX", cpu.IX, 0x0000)
	requireZ80EqualU16(t, "IY", cpu.IY, 0x0000)
	requireZ80EqualU8(t, "I", cpu.I, 0x00)
	requireZ80EqualU8(t, "R", cpu.R, 0x00)
	requireZ80EqualU16(t, "WZ", cpu.WZ, 0x0000)
	if cpu.IFF1 || cpu.IFF2 {
		t.Fatalf("IFF1/IFF2 should be cleared on reset")
	}
	if cpu.irqLine.Load() || cpu.nmiLine.Load() || cpu.nmiPending.Load() {
		t.Fatalf("interrupt lines should be cleared on reset")
	}
	if cpu.iffDelay != 0 {
		t.Fatalf("iffDelay should be cleared on reset")
	}
	if cpu.irqVector.Load() != 0xFF {
		t.Fatalf("irqVector = 0x%02X, want 0xFF", cpu.irqVector.Load())
	}
	if cpu.IM != 0 {
		t.Fatalf("IM = %d, want 0", cpu.IM)
	}
	if cpu.Halted {
		t.Fatalf("Halted should be false on reset")
	}
	if cpu.Cycles != 0 {
		t.Fatalf("Cycles = %d, want 0", cpu.Cycles)
	}
	if !cpu.Running() {
		t.Fatalf("Running should be true after reset")
	}
}

func TestZ80RegisterPairs(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.SetAF(0x1234)
	cpu.SetBC(0x2345)
	cpu.SetDE(0x3456)
	cpu.SetHL(0x4567)
	cpu.SetAF2(0x6789)
	cpu.SetBC2(0x789A)
	cpu.SetDE2(0x89AB)
	cpu.SetHL2(0x9ABC)

	requireZ80EqualU16(t, "AF", cpu.AF(), 0x1234)
	requireZ80EqualU16(t, "BC", cpu.BC(), 0x2345)
	requireZ80EqualU16(t, "DE", cpu.DE(), 0x3456)
	requireZ80EqualU16(t, "HL", cpu.HL(), 0x4567)
	requireZ80EqualU16(t, "AF'", cpu.AF2(), 0x6789)
	requireZ80EqualU16(t, "BC'", cpu.BC2(), 0x789A)
	requireZ80EqualU16(t, "DE'", cpu.DE2(), 0x89AB)
	requireZ80EqualU16(t, "HL'", cpu.HL2(), 0x9ABC)
}

func TestZ80StepNOP(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x00})

	cpu := rig.cpu
	cpu.Step()

	requireZ80EqualU16(t, "PC", cpu.PC, 0x0001)
	if cpu.Cycles != 4 {
		t.Fatalf("Cycles = %d, want 4", cpu.Cycles)
	}
	if rig.bus.ticks != 4 {
		t.Fatalf("bus ticks = %d, want 4", rig.bus.ticks)
	}
}

// TestZ80_Voodoo_IOPort_Integration tests the Z80 Voodoo I/O port mechanism
func TestZ80_Voodoo_IOPort_Integration(t *testing.T) {
	// Create a system bus and Voodoo engine
	bus := NewMachineBus()
	voodoo, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer voodoo.Destroy()

	// Map Voodoo to the system bus
	bus.MapIO(VOODOO_BASE, VOODOO_END, voodoo.HandleRead, voodoo.HandleWrite)

	// Create Z80 system bus with Voodoo
	z80Bus := NewZ80BusAdapterWithVoodoo(bus, nil, voodoo)

	// Simulate: write 0x00001400 to VOODOO_VERTEX_AX (offset 0x008)
	// Step 1: Set register offset to 0x008
	z80Bus.Out(Z80_VOODOO_PORT_ADDR_LO, 0x08) // Low byte of offset
	z80Bus.Out(Z80_VOODOO_PORT_ADDR_HI, 0x00) // High byte of offset

	// Step 2: Write data bytes (little-endian: 0x00, 0x14, 0x00, 0x00)
	z80Bus.Out(Z80_VOODOO_PORT_DATA0, 0x00) // Bits 0-7
	z80Bus.Out(Z80_VOODOO_PORT_DATA1, 0x14) // Bits 8-15 (0x14 << 8 = 0x1400)
	z80Bus.Out(Z80_VOODOO_PORT_DATA2, 0x00) // Bits 16-23
	z80Bus.Out(Z80_VOODOO_PORT_DATA3, 0x00) // Bits 24-31 (triggers write)

	// Verify the vertex X coordinate was set correctly
	// 0x1400 in 12.4 format = 5120 / 16 = 320.0
	expectedX := float32(320.0)
	actualX := voodoo.vertices[0].X
	if actualX != expectedX {
		t.Errorf("VERTEX_AX: expected %f, got %f", expectedX, actualX)
	}

	// Test setting Y coordinate
	z80Bus.Out(Z80_VOODOO_PORT_ADDR_LO, 0x0C) // Offset 0x00C for VERTEX_AY
	z80Bus.Out(Z80_VOODOO_PORT_ADDR_HI, 0x00)
	z80Bus.Out(Z80_VOODOO_PORT_DATA0, 0x40) // 0x0640 = 100 * 16 = 1600
	z80Bus.Out(Z80_VOODOO_PORT_DATA1, 0x06)
	z80Bus.Out(Z80_VOODOO_PORT_DATA2, 0x00)
	z80Bus.Out(Z80_VOODOO_PORT_DATA3, 0x00)

	expectedY := float32(100.0)
	actualY := voodoo.vertices[0].Y
	if actualY != expectedY {
		t.Errorf("VERTEX_AY: expected %f, got %f", expectedY, actualY)
	}

	t.Logf("Z80 Voodoo I/O port integration test passed! X=%f, Y=%f", actualX, actualY)
}

// TestZ80_Voodoo_FullTriangle_IOPort tests complete triangle via I/O ports with pixel verification
func TestZ80_Voodoo_FullTriangle_IOPort(t *testing.T) {
	bus := NewMachineBus()
	voodoo, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer voodoo.Destroy()

	bus.MapIO(VOODOO_BASE, VOODOO_END, voodoo.HandleRead, voodoo.HandleWrite)
	z80Bus := NewZ80BusAdapterWithVoodoo(bus, nil, voodoo)

	// Helper to write 32-bit value via I/O ports (mimics Z80 voodoo_write32)
	write32 := func(offset uint16, value uint32) {
		z80Bus.Out(Z80_VOODOO_PORT_ADDR_LO, byte(offset&0xFF))
		z80Bus.Out(Z80_VOODOO_PORT_ADDR_HI, byte(offset>>8))
		z80Bus.Out(Z80_VOODOO_PORT_DATA0, byte(value))
		z80Bus.Out(Z80_VOODOO_PORT_DATA1, byte(value>>8))
		z80Bus.Out(Z80_VOODOO_PORT_DATA2, byte(value>>16))
		z80Bus.Out(Z80_VOODOO_PORT_DATA3, byte(value>>24))
	}

	// 1. Enable Voodoo and set video dimensions (640x480)
	write32(0x004, 1) // VOODOO_ENABLE
	write32(0x214, (640<<16)|480)
	t.Logf("After VIDEO_DIM: width=%d, height=%d", voodoo.width.Load(), voodoo.height.Load())

	// 2. Disable texture
	write32(0x300, 0) // TEXTURE_MODE = 0

	// 3. Set color path to vertex only
	write32(0x104, 0) // FBZCOLOR_PATH = 0 (ITERATED)

	// 4. Clear with blue
	write32(0x1D8, 0xFF0000FF) // COLOR0 = blue
	write32(0x124, 0)          // FAST_FILL_CMD

	// 5. Set vertex A (320, 100)
	write32(0x008, 320<<4) // VERTEX_AX
	write32(0x00C, 100<<4) // VERTEX_AY
	t.Logf("Vertex A: X=%f, Y=%f", voodoo.vertices[0].X, voodoo.vertices[0].Y)

	// 6. Set vertex B (500, 380)
	write32(0x010, 500<<4) // VERTEX_BX
	write32(0x014, 380<<4) // VERTEX_BY
	t.Logf("Vertex B: X=%f, Y=%f", voodoo.vertices[1].X, voodoo.vertices[1].Y)

	// 7. Set vertex C (140, 380)
	write32(0x018, 140<<4) // VERTEX_CX
	write32(0x01C, 380<<4) // VERTEX_CY
	t.Logf("Vertex C: X=%f, Y=%f", voodoo.vertices[2].X, voodoo.vertices[2].Y)

	// 8. Set white color
	write32(0x020, 0x1000) // START_R
	write32(0x024, 0x1000) // START_G
	write32(0x028, 0x1000) // START_B
	write32(0x030, 0x1000) // START_A
	t.Logf("Color: R=%f, G=%f, B=%f, A=%f",
		voodoo.currentVertex.R, voodoo.currentVertex.G,
		voodoo.currentVertex.B, voodoo.currentVertex.A)

	// 9. Submit triangle
	write32(0x080, 0) // TRIANGLE_CMD
	t.Logf("Triangle batch size: %d", len(voodoo.triangleBatch))

	if len(voodoo.triangleBatch) != 1 {
		t.Fatalf("Triangle not added! batch=%d", len(voodoo.triangleBatch))
	}

	// Log triangle colors
	tri := voodoo.triangleBatch[0]
	t.Logf("Triangle[0] colors: R=%f G=%f B=%f A=%f",
		tri.Vertices[0].R, tri.Vertices[0].G, tri.Vertices[0].B, tri.Vertices[0].A)

	// 10. Swap buffers
	write32(0x128, 1) // SWAP_BUFFER_CMD

	// 11. Check pixels
	frame := voodoo.GetFrame()
	t.Logf("Frame size: %d", len(frame))

	centerIdx := (250*640 + 320) * 4
	cornerIdx := (10*640 + 10) * 4

	t.Logf("Center (320,250): R=%d G=%d B=%d A=%d",
		frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2], frame[centerIdx+3])
	t.Logf("Corner (10,10): R=%d G=%d B=%d A=%d",
		frame[cornerIdx], frame[cornerIdx+1], frame[cornerIdx+2], frame[cornerIdx+3])

	// Center should be white (255,255,255)
	if frame[centerIdx] < 200 || frame[centerIdx+1] < 200 || frame[centerIdx+2] < 200 {
		t.Errorf("Center not white! R=%d G=%d B=%d",
			frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2])
	}

	// Corner should be blue (0,0,255)
	if frame[cornerIdx+2] < 200 {
		t.Errorf("Corner not blue! R=%d G=%d B=%d",
			frame[cornerIdx], frame[cornerIdx+1], frame[cornerIdx+2])
	}
}
