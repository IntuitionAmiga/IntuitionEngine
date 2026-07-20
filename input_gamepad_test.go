// input_gamepad_test.go - Tests for the host-independent gamepad MMIO block.
//
// USB Gamepad Input plan, step 1 (RED phase). Modelled on sysinfo_mmio_test.go.

package main

import (
	"math"
	"testing"
)

// padBusBase returns the bus address of pad p's record base.
func padBusBase(p uint32) uint32 {
	return GAMEPAD_PAD0_BASE + p*GAMEPAD_PAD_STRIDE
}

func TestGamepad_RegionLayoutFits(t *testing.T) {
	// Four 12-byte pad records must fit exactly under GAMEPAD_REGION_END.
	last := padBusBase(GAMEPAD_MAX_PADS-1) + GAMEPAD_AXIS_RXY_OFF + 3
	if last != GAMEPAD_REGION_END {
		t.Fatalf("last pad word ends at %#x, want region end %#x", last, GAMEPAD_REGION_END)
	}
	if GAMEPAD_STATUS != GAMEPAD_REGION_BASE {
		t.Fatalf("status %#x != region base %#x", GAMEPAD_STATUS, GAMEPAD_REGION_BASE)
	}
}

func TestGamepad_ReadPacksSnapshot(t *testing.T) {
	bus := NewMachineBus()
	m := RegisterGamepadMMIO(bus)

	var snap GamepadSnapshot
	snap.Pads[0].Connected = true
	snap.Pads[0].Buttons[JOY_BIT_A] = true
	snap.Pads[0].Buttons[JOY_BIT_START] = true
	snap.Pads[0].LX = 1.0  // full right -> 32767
	snap.Pads[0].LY = -1.0 // full up   -> -32767
	snap.Pads[0].RX = 0.0
	snap.Pads[0].RY = 0.5 // -> 16383
	snap.Pads[2].Connected = true
	m.applySnapshot(snap)

	// Status: pads 0 and 2 connected, count 2.
	status := bus.Read32(GAMEPAD_STATUS)
	wantConnected := uint32(1<<0 | 1<<2)
	if status&0xF != wantConnected {
		t.Errorf("status connected = %#x, want %#x", status&0xF, wantConnected)
	}
	if count := (status >> 8) & 0xF; count != 2 {
		t.Errorf("status count = %d, want 2", count)
	}

	// Pad 0 buttons.
	buttons := bus.Read32(padBusBase(0) + GAMEPAD_BUTTONS_OFF)
	wantBtn := uint32(1<<JOY_BIT_A | 1<<JOY_BIT_START)
	if buttons != wantBtn {
		t.Errorf("pad0 buttons = %#x, want %#x", buttons, wantBtn)
	}

	// Pad 0 left axis packing.
	lxy := bus.Read32(padBusBase(0) + GAMEPAD_AXIS_LXY_OFF)
	lx := int16(uint16(lxy & 0xFFFF))
	ly := int16(uint16(lxy >> 16))
	if lx != 32767 {
		t.Errorf("pad0 LX = %d, want 32767", lx)
	}
	if ly != -32767 {
		t.Errorf("pad0 LY = %d, want -32767", ly)
	}

	rxy := bus.Read32(padBusBase(0) + GAMEPAD_AXIS_RXY_OFF)
	ry := int16(uint16(rxy >> 16))
	if ry != 16383 {
		t.Errorf("pad0 RY = %d, want 16383", ry)
	}
}

// Small CPUs (6502/Z80) read the block one byte or halfword at a time via the
// bus, which passes the exact byte address and truncates. Non-zero lanes must
// carry the correct bits.
func TestGamepad_ByteAndHalfwordLanes(t *testing.T) {
	bus := NewMachineBus()
	m := RegisterGamepadMMIO(bus)
	var snap GamepadSnapshot
	snap.Pads[0].Connected = true
	snap.Pads[0].Buttons[JOY_BIT_HOME] = true // bit 16 -> byte lane 2
	snap.Pads[0].Buttons[JOY_BIT_A] = true    // bit 4  -> byte lane 0
	snap.Pads[0].LX = 1.0                     // 32767 -> low half of AXIS_LXY
	snap.Pads[0].LY = -1.0                    // -32767 -> high half
	m.applySnapshot(snap)

	btnAddr := padBusBase(0) + GAMEPAD_BUTTONS_OFF
	// JOY_HOME lives in byte lane 2 of BUTTONS; a byte read there must be 1.
	if got := bus.Read8(btnAddr + 2); got != 0x01 {
		t.Errorf("BUTTONS+2 byte = %#x, want 0x01 (JOY_HOME)", got)
	}
	if got := bus.Read8(btnAddr + 0); got != (1<<JOY_BIT_A)&0xFF {
		t.Errorf("BUTTONS+0 byte = %#x, want %#x", got, (1<<JOY_BIT_A)&0xFF)
	}

	axAddr := padBusBase(0) + GAMEPAD_AXIS_LXY_OFF
	// High halfword of AXIS_LXY is the signed Y axis.
	if got := int16(bus.Read16(axAddr + 2)); got != -32767 {
		t.Errorf("AXIS_LXY+2 halfword = %d, want -32767 (LY)", got)
	}
	if got := int16(bus.Read16(axAddr + 0)); got != 32767 {
		t.Errorf("AXIS_LXY+0 halfword = %d, want 32767 (LX)", got)
	}
	// STATUS high count nibble via byte lane 1.
	snap.Pads[1].Connected = true
	m.applySnapshot(snap)
	if got := bus.Read8(GAMEPAD_STATUS + 1); got != 0x02 { // count 2 in bits 8..11
		t.Errorf("STATUS+1 byte = %#x, want 0x02 (count)", got)
	}
}

func TestGamepad_WritesIgnored(t *testing.T) {
	bus := NewMachineBus()
	m := RegisterGamepadMMIO(bus)
	var snap GamepadSnapshot
	snap.Pads[0].Connected = true
	snap.Pads[0].Buttons[JOY_BIT_B] = true
	m.applySnapshot(snap)

	bus.Write32(padBusBase(0)+GAMEPAD_BUTTONS_OFF, 0xDEADBEEF)
	bus.Write32(GAMEPAD_STATUS, 0xDEADBEEF)

	if got := bus.Read32(padBusBase(0) + GAMEPAD_BUTTONS_OFF); got != 1<<JOY_BIT_B {
		t.Fatalf("after write buttons = %#x, want %#x", got, 1<<JOY_BIT_B)
	}
}

func TestGamepad_ScaleAxisEndpointsAndNaN(t *testing.T) {
	cases := []struct {
		in   float64
		want int16
	}{
		{1.0, 32767},
		{-1.0, -32767},
		{2.0, 32767},   // clamp high
		{-2.0, -32767}, // clamp low
		{0.0, 0},
		{-0.5, -16383},
		{math.NaN(), 0},
		{math.Inf(1), 32767},
		{math.Inf(-1), -32767},
	}
	for _, c := range cases {
		if got := scaleAxis(c.in); got != c.want {
			t.Errorf("scaleAxis(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGamepad_DisconnectedPadClearsState(t *testing.T) {
	bus := NewMachineBus()
	m := RegisterGamepadMMIO(bus)

	var s1 GamepadSnapshot
	s1.Pads[1].Connected = true
	s1.Pads[1].Buttons[JOY_BIT_X] = true
	s1.Pads[1].LX = 1.0
	m.applySnapshot(s1)
	if got := bus.Read32(padBusBase(1) + GAMEPAD_BUTTONS_OFF); got == 0 {
		t.Fatalf("pad1 buttons should be set before disconnect")
	}

	// New snapshot: pad 1 gone (e.g. controller unplugged / Ebiten ID changed).
	var s2 GamepadSnapshot
	m.applySnapshot(s2)
	if got := bus.Read32(padBusBase(1) + GAMEPAD_BUTTONS_OFF); got != 0 {
		t.Errorf("pad1 buttons after disconnect = %#x, want 0", got)
	}
	if got := bus.Read32(padBusBase(1) + GAMEPAD_AXIS_LXY_OFF); got != 0 {
		t.Errorf("pad1 axis after disconnect = %#x, want 0", got)
	}
	if got := bus.Read32(GAMEPAD_STATUS); got != 0 {
		t.Errorf("status after disconnect = %#x, want 0", got)
	}
}

func TestGamepad_GetIORegionLabelsBlock(t *testing.T) {
	if got := GetIORegion(GAMEPAD_STATUS); got != "Gamepad" {
		t.Fatalf("GetIORegion(GAMEPAD_STATUS) = %q, want \"Gamepad\"", got)
	}
	if got := GetIORegion(GAMEPAD_REGION_END); got != "Gamepad" {
		t.Fatalf("GetIORegion(GAMEPAD_REGION_END) = %q, want \"Gamepad\"", got)
	}
}

// Overlap proof: the gamepad range must not overlap any documented MMIO region
// and must sit inside the IO region. The address is proven here, not asserted.
func TestGamepad_MMIOInventoryNoOverlap(t *testing.T) {
	regions := []struct {
		name       string
		start, end uint32
	}{
		{"VideoChip", VIDEO_REGION_BASE, VIDEO_REGION_END},
		{"Terminal", TERMINAL_REGION_BASE, TERMINAL_REGION_END},
		{"AudioChip", AUDIO_REGION_BASE, AUDIO_REGION_END},
		{"PSG", PSG_REGION_BASE, PSG_REGION_END},
		{"POKEY", POKEY_REGION_BASE, POKEY_REGION_END},
		{"SID", SID_REGION_BASE, SID_REGION_END},
		{"TED", TED_REGION_BASE, TED_REGION_END},
		{"VGA", VGA_REGION_BASE, VGA_REGION_END},
		{"HostHelper", HOST_MMIO_REGION_BASE, HOST_MMIO_REGION_END},
		{"ULA", ULA_REGION_BASE, ULA_REGION_END},
		{"FileIO", FILE_IO_REGION_BASE, FILE_IO_REGION_END},
		{"AROSDOSHandler", AROS_DOS_REGION_BASE, AROS_DOS_REGION_END},
		{"AROSAudioDMA", AROS_AUD_REGION_BASE_REG, AROS_AUD_REGION_END_REG},
		{"MediaLoader", MEDIA_LOADER_REGION_BASE, MEDIA_LOADER_REGION_END},
		{"ProgramExecutor", EXEC_REGION_BASE, EXEC_REGION_END},
		{"ANTIC", ANTIC_REGION_BASE, ANTIC_REGION_END},
		{"GTIA", GTIA_REGION_BASE, GTIA_REGION_END},
		{"Coprocessor", COPROC_REGION_BASE, COPROC_REGION_END},
		{"ClipboardBridge", CLIP_BRIDGE_REGION_BASE, CLIP_BRIDGE_REGION_END},
		{"CoprocessorExt", COPROC_EXT_REGION_BASE, COPROC_EXT_REGION_END},
		{"CoprocessorExt2", COPROC_EXT2_BASE, COPROC_EXT2_END},
		{"IRQDiag", IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END},
		{"BootstrapHostFS", BOOT_HOSTFS_BASE, BOOT_HOSTFS_BASE + 0x1F},
		{"SysInfo", SYSINFO_REGION_BASE, SYSINFO_REGION_END},
		{"AROSHostSocket", AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END},
		{"CPUWait", CPU_WAIT_REGION_BASE, CPU_WAIT_REGION_END},
		{"Voodoo", VOODOO_REGION_BASE, VOODOO_REGION_END},
	}
	overlap := func(a0, a1, b0, b1 uint32) bool { return a0 <= b1 && b0 <= a1 }
	for _, r := range regions {
		if overlap(GAMEPAD_REGION_BASE, GAMEPAD_REGION_END, r.start, r.end) {
			t.Errorf("Gamepad region [%#x,%#x] overlaps %s [%#x,%#x]",
				GAMEPAD_REGION_BASE, GAMEPAD_REGION_END, r.name, r.start, r.end)
		}
	}
	if GAMEPAD_REGION_BASE < IO_REGION_BASE || GAMEPAD_REGION_END > IO_REGION_END {
		t.Errorf("Gamepad region [%#x,%#x] outside IO region [%#x,%#x]",
			GAMEPAD_REGION_BASE, GAMEPAD_REGION_END, IO_REGION_BASE, IO_REGION_END)
	}
}
