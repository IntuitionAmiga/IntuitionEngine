// input_gamepad.go - Host-independent USB gamepad MMIO discovery block.
//
// A single read-only memory-mapped register block that the host fills once per
// frame and every guest reads. The canonical layout is vendor neutral: the
// host backend (Ebiten) resolves controller differences via its built-in
// mapping database and publishes a fixed button/axis layout here.
//
// This file holds no windowing-backend import. The Ebiten polling lives in
// input_gamepad_ebiten.go; headless/js builds use input_gamepad_stub.go.
//
// See how-do-you-propose-tender-pearl.md (USB Gamepad Input plan), step 1.

package main

import (
	"math"
	"sync"
)

// Canonical button bit positions (fixed, vendor neutral, from Ebiten
// StandardGamepadButton). These indices are the ABI: guests and the joydefs
// library read the same bit for the same physical button.
const (
	JOY_BIT_UP = iota
	JOY_BIT_DOWN
	JOY_BIT_LEFT
	JOY_BIT_RIGHT
	JOY_BIT_A
	JOY_BIT_B
	JOY_BIT_X
	JOY_BIT_Y
	JOY_BIT_LB
	JOY_BIT_RB
	JOY_BIT_LT
	JOY_BIT_RT
	JOY_BIT_SELECT
	JOY_BIT_START
	JOY_BIT_L3
	JOY_BIT_R3
	JOY_BIT_HOME

	GAMEPAD_BUTTON_COUNT
)

// GamepadPad is one pad's per-frame input, indexed by canonical button bit.
// Axes are Ebiten-native floats (down and right positive), clamped and scaled
// to signed 16-bit on publication.
type GamepadPad struct {
	Connected bool
	Buttons   [GAMEPAD_BUTTON_COUNT]bool
	LX, LY    float64
	RX, RY    float64
}

// GamepadSnapshot is a whole-frame view of all pads produced by the host poll.
type GamepadSnapshot struct {
	Pads [GAMEPAD_MAX_PADS]GamepadPad
}

// padState holds the packed register words for one pad.
type padState struct {
	buttons uint32
	axisLXY uint32
	axisRXY uint32
}

// GamepadMMIO is the read-only device backing the gamepad register block.
type GamepadMMIO struct {
	mu        sync.RWMutex
	pads      [GAMEPAD_MAX_PADS]padState
	connected uint32 // bit p set when pad p connected
	count     uint32 // number of connected pads
}

// scaleAxis clamps an Ebiten axis float to -1..1 and scales it to signed 16-bit.
// NaN maps to 0.
func scaleAxis(v float64) int16 {
	if math.IsNaN(v) {
		return 0
	}
	if v > 1 {
		v = 1
	}
	if v < -1 {
		v = -1
	}
	return int16(v * 32767)
}

// packAxisPair packs two axis floats into one register word: x in the low 16
// bits, y in the high 16 bits.
func packAxisPair(x, y float64) uint32 {
	return uint32(uint16(scaleAxis(x))) | uint32(uint16(scaleAxis(y)))<<16
}

// applySnapshot packs a whole-frame snapshot into the register words. A
// disconnected pad's previous state is cleared so stale input never lingers.
// Pure with respect to inputs: it derives all state from snap.
func (m *GamepadMMIO) applySnapshot(snap GamepadSnapshot) {
	var connected, count uint32
	var pads [GAMEPAD_MAX_PADS]padState
	for i := 0; i < GAMEPAD_MAX_PADS; i++ {
		p := snap.Pads[i]
		if !p.Connected {
			continue // pads[i] stays zero: cleared
		}
		connected |= 1 << uint(i)
		count++
		var b uint32
		for bit := 0; bit < GAMEPAD_BUTTON_COUNT; bit++ {
			if p.Buttons[bit] {
				b |= 1 << uint(bit)
			}
		}
		pads[i].buttons = b
		pads[i].axisLXY = packAxisPair(p.LX, p.LY)
		pads[i].axisRXY = packAxisPair(p.RX, p.RY)
	}
	m.mu.Lock()
	m.pads = pads
	m.connected = connected
	m.count = count
	m.mu.Unlock()
}

// readWord returns the full 32-bit register value at a 4-byte-aligned block
// address, or 0 for gaps. Every gamepad register is word-aligned.
func (m *GamepadMMIO) readWord(wordAddr uint32) uint32 {
	if wordAddr == GAMEPAD_STATUS {
		return m.connected | m.count<<8
	}
	if wordAddr >= GAMEPAD_PAD0_BASE && wordAddr <= GAMEPAD_REGION_END {
		rel := wordAddr - GAMEPAD_PAD0_BASE
		p := rel / GAMEPAD_PAD_STRIDE
		if p >= GAMEPAD_MAX_PADS {
			return 0
		}
		switch rel % GAMEPAD_PAD_STRIDE {
		case GAMEPAD_BUTTONS_OFF:
			return m.pads[p].buttons
		case GAMEPAD_AXIS_LXY_OFF:
			return m.pads[p].axisLXY
		case GAMEPAD_AXIS_RXY_OFF:
			return m.pads[p].axisRXY
		}
	}
	return 0
}

// read serves reads from the gamepad register block. The bus passes the exact
// byte address for sub-word (8/16-bit) accesses and truncates the low bytes of
// the returned value, so we align to the containing 32-bit word and shift the
// requested lane down. This preserves the documented 6502/Z80 byte-lane ABI
// (e.g. JOY_HOME at bit 16, or the signed high axis half).
func (m *GamepadMMIO) read(addr uint32) uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	word := m.readWord(addr &^ 3)
	return word >> ((addr & 3) * 8)
}

// write is a no-op: the block is read-only. Writes are accepted (so guest
// stores do not fault under strict MMIO policy) but ignored, matching
// RegisterSysInfoMMIO.
func (m *GamepadMMIO) write(addr uint32, value uint32) {}

// RegisterGamepadMMIO maps the read-only gamepad block onto the bus and returns
// the device so the host poll can publish snapshots into it.
func RegisterGamepadMMIO(bus *MachineBus) *GamepadMMIO {
	m := &GamepadMMIO{}
	bus.MapIO(GAMEPAD_REGION_BASE, GAMEPAD_REGION_END, m.read, m.write)
	return m
}
