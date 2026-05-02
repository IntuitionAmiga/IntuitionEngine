//go:build headless

package main

import "testing"

func newMappedTestVoodoo(t *testing.T) (*MachineBus, *VoodooEngine) {
	t.Helper()
	bus := NewMachineBus()
	v, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	t.Cleanup(v.Destroy)

	bus.MapIO(VOODOO_BASE, VOODOO_END, v.HandleRead, v.HandleWrite)
	bus.MapIOByteRead(VOODOO_BASE, VOODOO_END, v.HandleRead8)
	bus.MapIOByte(VOODOO_BASE, VOODOO_END, v.HandleWrite8)
	bus.MapIO64(VOODOO_BASE, VOODOO_END, v.HandleRead64, v.HandleWrite64)
	bus.MapIO(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead, v.HandleTexMemWrite)
	bus.MapIOByteRead(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead8)
	bus.MapIOByte(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemWrite8)
	return bus, v
}

func TestVoodoo_MapIO_Coverage(t *testing.T) {
	bus, _ := newMappedTestVoodoo(t)
	for _, addr := range []uint32{
		VOODOO_BASE,
		VOODOO_END,
		VOODOO_PALETTE_BASE,
		VOODOO_TEXMEM_BASE,
		VOODOO_TEXMEM_BASE + VOODOO_TEXMEM_SIZE - 1,
	} {
		if bus.findIORegion(addr) == nil {
			t.Fatalf("address $%05X is not covered by a 32-bit IO region", addr)
		}
	}
	if bus.findIORegion64(VOODOO_BASE) == nil || bus.findIORegion64(VOODOO_END) == nil {
		t.Fatal("Voodoo register aperture is not covered by 64-bit IO")
	}
}

func TestVoodoo_ByteWrite_NoCorruption(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.HandleWrite8(VOODOO_ENABLE+0, 0xAA)
	v.HandleWrite8(VOODOO_ENABLE+1, 0xBB)
	if got := v.HandleRead(VOODOO_ENABLE); got != 0x0000BBAA {
		t.Fatalf("partial shadow = %#08x, want 0x0000BBAA", got)
	}
}

func TestVoodoo_ByteRead_UsesAddressedRegisterByte(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	v.HandleWrite(VOODOO_COLOR0, 0x11223344)

	if got := bus.Read8(VOODOO_COLOR0 + 1); got != 0x33 {
		t.Fatalf("byte read COLOR0+1 = %#02x, want 0x33", got)
	}
}

func TestVoodoo_ByteReadWithFault_UsesAddressedRegisterByte(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	v.HandleWrite(VOODOO_COLOR0, 0x11223344)

	got, ok := bus.Read8WithFault(VOODOO_COLOR0 + 1)
	if !ok {
		t.Fatal("Read8WithFault returned ok=false for mapped Voodoo register byte")
	}
	if got != 0x33 {
		t.Fatalf("faulting byte read COLOR0+1 = %#02x, want 0x33", got)
	}
}

func TestVoodoo_BusWrite32_UsesDwordRegisterSemantics(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)

	bus.Write32(VOODOO_ENABLE, 1)
	if !v.IsEnabled() {
		t.Fatal("Write32 VOODOO_ENABLE did not enable Voodoo")
	}
	if got := bus.Read32(VOODOO_ENABLE); got != 1 {
		t.Fatalf("VOODOO_ENABLE readback = %#08x, want 0x00000001", got)
	}

	const dim = uint32(0x028001E0)
	bus.Write32(VOODOO_VIDEO_DIM, dim)
	if got := bus.Read32(VOODOO_VIDEO_DIM); got != dim {
		t.Fatalf("VOODOO_VIDEO_DIM readback = %#08x, want %#08x", got, dim)
	}
	if w, h := v.GetDimensions(); w != 640 || h != 480 {
		t.Fatalf("Voodoo dimensions = %dx%d, want 640x480", w, h)
	}

	const fbz = uint32(0x00000670)
	bus.Write32(VOODOO_FBZ_MODE, fbz)
	if got := bus.Read32(VOODOO_FBZ_MODE); got != fbz {
		t.Fatalf("VOODOO_FBZ_MODE readback = %#08x, want %#08x", got, fbz)
	}
	if v.fbzMode != fbz {
		t.Fatalf("fbzMode side effect = %#08x, want %#08x", v.fbzMode, fbz)
	}
}

func TestVoodoo_OverridesOverlappingTEDVRAMWindow(t *testing.T) {
	bus := NewMachineBus()
	ted := NewTEDVideoEngine(bus)
	bus.MapIO(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1,
		ted.HandleBusVRAMRead,
		ted.HandleBusVRAMWrite)

	v, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	t.Cleanup(v.Destroy)
	bus.MapIO(VOODOO_BASE, VOODOO_END, v.HandleRead, v.HandleWrite)
	bus.MapIOByteRead(VOODOO_BASE, VOODOO_END, v.HandleRead8)
	bus.MapIOByte(VOODOO_BASE, VOODOO_END, v.HandleWrite8)
	bus.MapIO64(VOODOO_BASE, VOODOO_END, v.HandleRead64, v.HandleWrite64)
	bus.MapIO(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead, v.HandleTexMemWrite)
	bus.MapIOByteRead(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead8)
	bus.MapIOByte(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemWrite8)

	bus.Write32(VOODOO_ENABLE, 1)
	if !v.IsEnabled() {
		t.Fatal("overlapping TED VRAM mapping intercepted VOODOO_ENABLE")
	}

	const dim = uint32(0x028001E0)
	bus.Write32(VOODOO_VIDEO_DIM, dim)
	if got := bus.Read32(VOODOO_VIDEO_DIM); got != dim {
		t.Fatalf("VOODOO_VIDEO_DIM readback = %#08x, want %#08x", got, dim)
	}

	bus.Write8(VOODOO_TEXMEM_BASE, 0xA5)
	if got := v.textureMemory[0]; got != 0xA5 {
		t.Fatalf("overlapping TED VRAM mapping intercepted Voodoo texture byte: got %#02x", got)
	}
}

func TestVoodoo_OldAddressesAreNotAliases(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)

	const oldVoodooEnable = 0xF4004
	const oldVoodooTexmem = 0xF5000

	bus.Write32(oldVoodooEnable, 1)
	if v.IsEnabled() {
		t.Fatal("old VOODOO_ENABLE address still aliases Voodoo")
	}

	bus.Write8(oldVoodooTexmem, 0xA5)
	if got := v.textureMemory[0]; got == 0xA5 {
		t.Fatal("old VOODOO_TEXMEM_BASE address still aliases Voodoo texture memory")
	}
}

func TestVoodoo_M68KTexMemLongWriteUsesDeviceByteOrder(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	cpu := NewM68KCPU(bus)

	cpu.Write32(VOODOO_TEXMEM_BASE, 0x11223344)
	if got := v.HandleTexMemRead(VOODOO_TEXMEM_BASE); got != 0x11223344 {
		t.Fatalf("M68K Voodoo texture dword = %#08x, want 0x11223344", got)
	}
}

func TestVoodoo_ScriptHelpers_UseMappedDwordSemantics(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, NewTerminalMMIO())
	runtimeStatus.setChips(nil, nil, nil, nil, nil, v, nil, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	})

	if err := se.RunString(`
		video.voodoo_enable(true)
		video.voodoo_resolution(640, 480)
		video.voodoo_zbuffer(0x0670)
	`, "voodoo_script_helpers"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}

	if !v.IsEnabled() {
		t.Fatal("script voodoo_enable did not enable Voodoo")
	}
	if got := bus.Read32(VOODOO_VIDEO_DIM); got != 0x028001E0 {
		t.Fatalf("script VOODOO_VIDEO_DIM readback = %#08x, want 0x028001E0", got)
	}
	if got := bus.Read32(VOODOO_FBZ_MODE); got != 0x00000670 {
		t.Fatalf("script VOODOO_FBZ_MODE readback = %#08x, want 0x00000670", got)
	}
}

func TestVoodoo_WordWrite_Coalesce(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	bus.Write16(VOODOO_COLOR0, 0x3344)
	bus.Write16(VOODOO_COLOR0+2, 0x1122)
	if got := v.HandleRead(VOODOO_COLOR0); got != 0x11223344 {
		t.Fatalf("word-coalesced shadow = %#08x, want 0x11223344", got)
	}
	if v.color0 != 0x11223344 {
		t.Fatalf("side effect color0 = %#08x, want 0x11223344", v.color0)
	}
}

func TestVoodoo_64BitWrite_Atomic(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	bus.Write64(VOODOO_COLOR0, 0x5566778811223344)
	if got := v.HandleRead(VOODOO_COLOR0); got != 0x11223344 {
		t.Fatalf("low register = %#08x, want 0x11223344", got)
	}
	if got := v.HandleRead(VOODOO_COLOR1); got != 0x55667788 {
		t.Fatalf("high register = %#08x, want 0x55667788", got)
	}
}

func TestVoodoo_TexMem_DirectUpload(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	for i, val := range []byte{0x10, 0x21, 0x32, 0x43, 0x54} {
		bus.Write8(VOODOO_TEXMEM_BASE+uint32(i), val)
	}
	bus.Write32(VOODOO_TEXMEM_BASE+8, 0xDDCCBBAA)
	want := []byte{0x10, 0x21, 0x32, 0x43, 0x54, 0, 0, 0, 0xAA, 0xBB, 0xCC, 0xDD}
	for i, wantByte := range want {
		if got := v.textureMemory[i]; got != wantByte {
			t.Fatalf("textureMemory[%d] = %#02x, want %#02x", i, got, wantByte)
		}
	}
}

func TestVoodoo_TexMem_Z80PortUpload(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	z80 := NewZ80BusAdapterWithVoodoo(bus, nil, v)
	for i, val := range []byte{0xCA, 0xFE, 0xBA, 0xBE} {
		bus.Write8(0x2000+uint32(i), val)
	}
	z80.Out(Z80_VOODOO_PORT_TEXSRC_LO, 0x00)
	z80.Out(Z80_VOODOO_PORT_TEXSRC_HI, 0x20)
	v.HandleWrite(VOODOO_TEX_WIDTH, 1)
	v.HandleWrite(VOODOO_TEX_HEIGHT, 1)
	z80.Out(Z80_VOODOO_PORT_ADDR_LO, byte((VOODOO_TEX_UPLOAD-VOODOO_BASE)&0xFF))
	z80.Out(Z80_VOODOO_PORT_ADDR_HI, byte(((VOODOO_TEX_UPLOAD-VOODOO_BASE)>>8)&0xFF))
	z80.Out(Z80_VOODOO_PORT_DATA0, 1)
	z80.Out(Z80_VOODOO_PORT_DATA1, 0)
	z80.Out(Z80_VOODOO_PORT_DATA2, 0)
	z80.Out(Z80_VOODOO_PORT_DATA3, 0)
	if got := v.textureMemory[:4]; got[0] != 0xCA || got[1] != 0xFE || got[2] != 0xBA || got[3] != 0xBE {
		t.Fatalf("Z80 tex upload = % x", got)
	}
}

func TestVoodoo_TexMem_6502_BankedWindow(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	adapter := NewBus6502AdapterWithVoodoo(bus, nil, v)

	adapter.Write(VOODOO_6502_BANK_HI, 0xD0)
	adapter.Write(VOODOO_6502_WINDOW_BASE+0, 0x10)
	adapter.Write(VOODOO_6502_WINDOW_BASE+1, 0x20)
	if v.textureMemory[0] != 0x10 || v.textureMemory[1] != 0x20 {
		t.Fatalf("6502 texture window bytes = %#02x %#02x", v.textureMemory[0], v.textureMemory[1])
	}

	adapter.Write(VOODOO_6502_BANK_HI, 0xF8)
	adapter.Write(VOODOO_6502_WINDOW_BASE+4, 0x01)
	adapter.Write(VOODOO_6502_WINDOW_BASE+5, 0x00)
	adapter.Write(VOODOO_6502_WINDOW_BASE+6, 0x00)
	if v.enabled.Load() {
		t.Fatal("6502 partial register write enabled Voodoo early")
	}
	adapter.Write(VOODOO_6502_WINDOW_BASE+7, 0x00)
	if !v.enabled.Load() {
		t.Fatal("6502 completed register dword did not enable Voodoo")
	}
}

func TestVoodoo_TexMem_6502_BankedWindow_UpperTexturePage(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	adapter := NewBus6502AdapterWithVoodoo(bus, nil, v)

	adapter.Write(VOODOO_6502_BANK_HI, 0xDF)
	adapter.Write(VOODOO_6502_BANK_PAGE_HI, 0x00)
	adapter.Write(VOODOO_6502_WINDOW_BASE+0xFFF, 0xA7)

	if got := v.textureMemory[VOODOO_TEXMEM_SIZE-1]; got != 0xA7 {
		t.Fatalf("last texture byte via 6502 aperture = %#02x, want 0xA7", got)
	}
}

func TestVoodoo_6502Runner_UsesVoodooAwareAdapter(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	runner := NewCPU6502Runner(bus, CPU6502Config{VoodooEngine: v})

	runner.cpu.writeByte(VOODOO_6502_BANK_HI, 0xD0)
	runner.cpu.writeByte(VOODOO_6502_WINDOW_BASE, 0x5C)

	if got := v.textureMemory[0]; got != 0x5C {
		t.Fatalf("runner 6502 Voodoo window wrote texture byte %#02x, want 0x5C", got)
	}
}

func TestVoodoo_6502Reset_ClearsVoodooBankPage(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	runner := NewCPU6502Runner(bus, CPU6502Config{VoodooEngine: v})

	runner.cpu.writeByte(VOODOO_6502_BANK_HI, 0xD0)
	runner.cpu.Reset()
	runner.cpu.writeByte(VOODOO_6502_WINDOW_BASE, 0x7D)

	if got := v.textureMemory[0]; got != 0 {
		t.Fatalf("reset preserved Voodoo bank page and wrote texture byte %#02x", got)
	}
}

func TestVoodoo_PartialWrite_NoSideEffect(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.HandleWrite8(VOODOO_ENABLE, 0x01)
	if v.enabled.Load() {
		t.Fatal("single byte write enabled Voodoo before dword completion")
	}
	v.HandleWrite8(VOODOO_ENABLE+1, 0x00)
	v.HandleWrite8(VOODOO_ENABLE+2, 0x00)
	v.HandleWrite8(VOODOO_ENABLE+3, 0x00)
	if !v.enabled.Load() {
		t.Fatal("completed dword write did not enable Voodoo")
	}
}

func TestVoodoo_PartialWrite_TriangleCmd(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.HandleWrite8(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite8(VOODOO_TRIANGLE_CMD+1, 0)
	v.HandleWrite8(VOODOO_TRIANGLE_CMD+2, 0)
	if got := len(v.triangleBatch); got != 0 {
		t.Fatalf("partial triangle command submitted %d triangles", got)
	}
	v.HandleWrite8(VOODOO_TRIANGLE_CMD+3, 0)
	if got := len(v.triangleBatch); got != 1 {
		t.Fatalf("completed triangle command submitted %d triangles, want 1", got)
	}
}

func TestVoodoo_FTriangleCmd_SubmitsTriangle(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_FTRIANGLECMD, 0)

	if got := len(v.triangleBatch); got != 1 {
		t.Fatalf("FTRIANGLECMD submitted %d triangles, want 1", got)
	}
}

func TestVoodoo_RegArray_Bounds(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.HandleWrite(VOODOO_PALETTE_BASE, 0x12345678)
	v.HandleWrite(VOODOO_FOG_TABLE_BASE+VOODOO_FOG_TABLE_SIZE*VOODOO_FOG_TABLE_STRIDE-4, 0x89ABCDEF)
	if got := v.HandleRead(VOODOO_PALETTE_BASE); got != 0x12345678 {
		t.Fatalf("palette readback = %#08x", got)
	}
	if got := v.HandleRead(VOODOO_FOG_TABLE_BASE + VOODOO_FOG_TABLE_SIZE*VOODOO_FOG_TABLE_STRIDE - 4); got != 0x89ABCDEF {
		t.Fatalf("fog table readback = %#08x", got)
	}
}

func TestVoodoo_RegArray_Sizing(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	highest := VOODOO_PALETTE_BASE + 255*4
	want := int((highest-VOODOO_BASE)/4) + 1
	if len(v.regs) < want {
		t.Fatalf("len(regs) = %d, want at least %d", len(v.regs), want)
	}
}
