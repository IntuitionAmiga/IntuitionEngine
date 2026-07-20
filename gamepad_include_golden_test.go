//go:build headless

// gamepad_include_golden_test.go - Drift guard for the per-arch gamepad
// assembly includes.
//
// USB Gamepad Input plan, step 3. Parses the GAMEPAD_* and JOY_* equates from
// all six hardware includes, resolves each to an effective bus address/value
// (modelling the 6502/Z80 bank window), and asserts they match the canonical
// Go constants. Compares resolved addresses and bit values, not literal
// equates.

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var incEquateRes = []*regexp.Regexp{
	regexp.MustCompile(`^\s*\.equ\s+(\w+)\s+(\S+)`),      // ie32
	regexp.MustCompile(`^\s*\.set\s+(\w+)\s*,\s*(\S+)`),  // ie80
	regexp.MustCompile(`^\s*(\w+)\s+equ\s+(\S+)`),        // ie64/ie68/ie86
	regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*=\s*(\S+)`), // ie65
}

func parseIncValue(tok string) (uint64, bool) {
	tok = strings.TrimSpace(tok)
	// Strip a trailing comment if it fused onto the token.
	if i := strings.IndexByte(tok, ';'); i >= 0 {
		tok = tok[:i]
	}
	tok = strings.TrimSpace(tok)
	switch {
	case strings.HasPrefix(tok, "0x"), strings.HasPrefix(tok, "0X"):
		v, err := strconv.ParseUint(tok[2:], 16, 64)
		return v, err == nil
	case strings.HasPrefix(tok, "$"):
		v, err := strconv.ParseUint(tok[1:], 16, 64)
		return v, err == nil
	default:
		v, err := strconv.ParseUint(tok, 10, 64)
		return v, err == nil
	}
}

// parseIncEquates returns GAMEPAD_*/JOY_* equates from an include file.
func parseIncEquates(t *testing.T, path string) map[string]uint64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		for _, re := range incEquateRes {
			m := re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name := m[1]
			if !strings.HasPrefix(name, "GAMEPAD_") && !strings.HasPrefix(name, "JOY_") {
				break
			}
			if v, ok := parseIncValue(m[2]); ok {
				out[name] = v
			}
			break
		}
	}
	return out
}

func TestGamepadIncludeGolden(t *testing.T) {
	joyWant := map[string]uint64{
		"JOY_UP": 1 << JOY_BIT_UP, "JOY_DOWN": 1 << JOY_BIT_DOWN,
		"JOY_LEFT": 1 << JOY_BIT_LEFT, "JOY_RIGHT": 1 << JOY_BIT_RIGHT,
		"JOY_A": 1 << JOY_BIT_A, "JOY_B": 1 << JOY_BIT_B,
		"JOY_X": 1 << JOY_BIT_X, "JOY_Y": 1 << JOY_BIT_Y,
		"JOY_LB": 1 << JOY_BIT_LB, "JOY_RB": 1 << JOY_BIT_RB,
		"JOY_LT": 1 << JOY_BIT_LT, "JOY_RT": 1 << JOY_BIT_RT,
		"JOY_SELECT": 1 << JOY_BIT_SELECT, "JOY_START": 1 << JOY_BIT_START,
		"JOY_L3": 1 << JOY_BIT_L3, "JOY_R3": 1 << JOY_BIT_R3,
		"JOY_HOME": 1 << JOY_BIT_HOME,
	}

	files := []struct {
		path   string
		banked bool // 6502/Z80 reach the block via a BANK1 window
	}{
		{"sdk/include/ie64.inc", false},
		{"sdk/include/ie32.inc", false},
		{"sdk/include/ie68.inc", false},
		{"sdk/include/ie86.inc", false},
		{"sdk/include/ie65.inc", true},
		{"sdk/include/ie80.inc", true},
	}

	for _, f := range files {
		eq := parseIncEquates(t, f.path)

		// Resolve status/pad0 to an effective bus address.
		statusBus := eq["GAMEPAD_STATUS"]
		pad0Bus := eq["GAMEPAD_PAD0_BASE"]
		if f.banked {
			bank := eq["GAMEPAD_IO_BANK"]
			if bank == 0 {
				t.Errorf("%s: missing GAMEPAD_IO_BANK for banked arch", f.path)
			}
			base := bank * 0x2000
			statusBus = base + (statusBus & 0x1FFF)
			pad0Bus = base + (pad0Bus & 0x1FFF)
			// The documented canonical bus constant must also be present and correct.
			if got := eq["GAMEPAD_REGION_BASE"]; got != uint64(GAMEPAD_REGION_BASE) {
				t.Errorf("%s: GAMEPAD_REGION_BASE = %#x, want %#x", f.path, got, GAMEPAD_REGION_BASE)
			}
		}

		if statusBus != uint64(GAMEPAD_STATUS) {
			t.Errorf("%s: resolved GAMEPAD_STATUS = %#x, want %#x", f.path, statusBus, GAMEPAD_STATUS)
		}
		if pad0Bus != uint64(GAMEPAD_PAD0_BASE) {
			t.Errorf("%s: resolved GAMEPAD_PAD0_BASE = %#x, want %#x", f.path, pad0Bus, GAMEPAD_PAD0_BASE)
		}

		// Offsets and stride are window-relative and identical across arches.
		checks := map[string]uint64{
			"GAMEPAD_PAD_STRIDE":   GAMEPAD_PAD_STRIDE,
			"GAMEPAD_BUTTONS_OFF":  GAMEPAD_BUTTONS_OFF,
			"GAMEPAD_AXIS_LXY_OFF": GAMEPAD_AXIS_LXY_OFF,
			"GAMEPAD_AXIS_RXY_OFF": GAMEPAD_AXIS_RXY_OFF,
		}
		for name, want := range checks {
			got, ok := eq[name]
			if !ok {
				t.Errorf("%s: missing %s", f.path, name)
				continue
			}
			if got != want {
				t.Errorf("%s: %s = %#x, want %#x", f.path, name, got, want)
			}
		}

		for name, want := range joyWant {
			got, ok := eq[name]
			if !ok {
				t.Errorf("%s: missing %s", f.path, name)
				continue
			}
			if got != want {
				t.Errorf("%s: %s = %#x, want %#x", f.path, name, got, want)
			}
		}
	}
}
