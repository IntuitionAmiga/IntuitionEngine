//go:build headless

package main

import "testing"

func newTestXBIOS(t *testing.T) (*XBIOSInterceptor, *M68KCPU, *MachineBus, *VideoChip, *PSGEngine) {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	psg := NewPSGEngine(sound, 44100)
	x := NewXBIOSInterceptor(cpu, bus, video, psg)
	cpu.xbiosHandler = x
	return x, cpu, bus, video, psg
}

func callXBIOS(cpu *M68KCPU, x *XBIOSInterceptor, fn uint16, args ...uint32) uint32 {
	pushTrapFrame(cpu, fn, args...)
	x.HandleTrap14()
	return cpu.DataRegs[0]
}

func TestXBIOS_PhysbaseReturnsVideoChipBase(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	if got := callXBIOS(cpu, x, XBIOS_PHYSBASE); got != VRAM_START {
		t.Fatalf("Physbase=0x%X, want 0x%X", got, VRAM_START)
	}
}

func TestXBIOS_LogbaseMatchesPhysbase(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	if got := callXBIOS(cpu, x, XBIOS_LOGBASE); got != VRAM_START {
		t.Fatalf("Logbase=0x%X, want 0x%X", got, VRAM_START)
	}
}

func TestXBIOS_SetscreenInBoundsAccepted(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	want := uint32(VRAM_START + 0x1000)
	if got := callXBIOS(cpu, x, XBIOS_SETSCREEN, want, 0xFFFFFFFF, 0xFFFFFFFF); got != want {
		t.Fatalf("Setscreen returned 0x%X, want 0x%X", got, want)
	}
	if got := callXBIOS(cpu, x, XBIOS_LOGBASE); got != want {
		t.Fatalf("Logbase after Setscreen=0x%X, want 0x%X", got, want)
	}
}

func TestXBIOS_SetscreenPhysOnlyDoesNotChangeLogbase(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	if got := callXBIOS(cpu, x, XBIOS_SETSCREEN, 0xFFFFFFFF, VRAM_START+0x1000, 0xFFFFFFFF); got != VRAM_START {
		t.Fatalf("phys-only Setscreen returned 0x%X, want unchanged 0x%X", got, VRAM_START)
	}
}

func TestXBIOS_SetscreenOutOfBoundsIgnored(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	if got := callXBIOS(cpu, x, XBIOS_SETSCREEN, EmuTOS_PROFILE_TOP, 0xFFFFFFFF, 0xFFFFFFFF); got != VRAM_START {
		t.Fatalf("Setscreen returned 0x%X, want unchanged 0x%X", got, VRAM_START)
	}
}

func TestXBIOS_SetpaletteWritesIEPalette(t *testing.T) {
	x, cpu, _, video, _ := newTestXBIOS(t)
	pal := uint32(0x3000)
	cpu.Write16(pal, 0x0700)
	cpu.Write16(pal+2, 0x0070)
	if got := callXBIOS(cpu, x, XBIOS_SETPALETTE, pal); got != 0 {
		t.Fatalf("Setpalette returned %d, want 0", got)
	}
	if video.clutPaletteHW[0] != 0x00FF0000 || video.clutPaletteHW[1] != 0x0000FF00 {
		t.Fatalf("palette entries not translated: %#x %#x", video.clutPaletteHW[0], video.clutPaletteHW[1])
	}
}

func TestXBIOS_SetcolorReadModifyWrite(t *testing.T) {
	x, cpu, _, video, _ := newTestXBIOS(t)
	pushTrapFrameRaw(cpu, XBIOS_SETCOLOR, []struct {
		size  int
		value uint32
	}{{2, 2}, {2, 0x0007}})
	x.HandleTrap14()
	if got := video.clutPaletteHW[2]; got != 0x000000FF {
		t.Fatalf("palette[2]=0x%X, want blue", got)
	}
	pushTrapFrameRaw(cpu, XBIOS_SETCOLOR, []struct {
		size  int
		value uint32
	}{{2, 2}, {2, 0x0700}})
	x.HandleTrap14()
	if old := cpu.DataRegs[0]; old != 0x0007 {
		t.Fatalf("Setcolor old=0x%X, want 0x0007", old)
	}
}

func TestXBIOS_DosoundDrivesPSG(t *testing.T) {
	x, cpu, bus, _, psg := newTestXBIOS(t)
	block := uint32(0x4000)
	bus.Write8(block, 0)
	bus.Write8(block+1, 0x34)
	bus.Write8(block+2, 1)
	bus.Write8(block+3, 0x12)
	bus.Write8(block+4, 0xFF)
	callXBIOS(cpu, x, XBIOS_DOSOUND, block)
	if got := psg.HandleRead(PSG_BASE); got != 0x34 {
		t.Fatalf("PSG R0=0x%X, want 0x34", got)
	}
	if got := psg.HandleRead(PSG_BASE + 1); got != 0x12 {
		t.Fatalf("PSG R1=0x%X, want 0x12", got)
	}
}

func TestXBIOS_RandomDeterministic(t *testing.T) {
	x1, cpu1, _, _, _ := newTestXBIOS(t)
	x2, cpu2, _, _, _ := newTestXBIOS(t)
	if a, b := callXBIOS(cpu1, x1, XBIOS_RANDOM), callXBIOS(cpu2, x2, XBIOS_RANDOM); a != b {
		t.Fatalf("first random differs: %d vs %d", a, b)
	}
}

func TestXBIOS_KbrateStoresWithoutFault(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	pushTrapFrameRaw(cpu, XBIOS_KBRATE, []struct {
		size  int
		value uint32
	}{{2, 3}, {2, 4}})
	x.HandleTrap14()
	if old := cpu.DataRegs[0]; old != 0 {
		t.Fatalf("first Kbrate old=%d, want 0", old)
	}
	pushTrapFrameRaw(cpu, XBIOS_KBRATE, []struct {
		size  int
		value uint32
	}{{2, 5}, {2, 6}})
	x.HandleTrap14()
	if old := cpu.DataRegs[0]; old != 0x0304 {
		t.Fatalf("second Kbrate old=0x%X, want 0x0304", old)
	}
}

func TestXBIOS_UnsupportedFallsThroughToEmuTOS(t *testing.T) {
	x, cpu, _, _, _ := newTestXBIOS(t)
	pushTrapFrame(cpu, 0x7FFF)
	cpu.DataRegs[0] = 0x12345678
	if x.HandleTrap14() {
		t.Fatal("unsupported XBIOS call should fall through to EmuTOS")
	}
	if got := cpu.DataRegs[0]; got != 0x12345678 {
		t.Fatalf("D0 changed on unsupported call: got 0x%X", got)
	}
}
