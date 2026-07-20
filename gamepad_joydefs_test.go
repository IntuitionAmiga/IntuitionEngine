//go:build headless

// gamepad_joydefs_test.go - Drift guard for the joydefs.bas button-bit library.
//
// USB Gamepad Input plan, step 4. Compares sdk/basic/joydefs.bas against the
// canonical Go button-bit constants. The include golden test validates the
// assembly includes; this validates the BASIC library.

package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

func TestJoyDefsBasMatchesGoConstants(t *testing.T) {
	data, err := os.ReadFile("sdk/basic/joydefs.bas")
	if err != nil {
		t.Fatalf("read joydefs.bas: %v", err)
	}
	// IE64 BASIC variable names cannot contain '_', so the library uses the
	// JOYxxx form. These map to the same canonical bits as the JOY_* names.
	re := regexp.MustCompile(`(?m)^\s*\d+\s+(JOY[A-Z0-9]+)\s*=\s*(\d+)\s*$`)
	got := map[string]uint64{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		v, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			t.Fatalf("bad value for %s: %v", m[1], err)
		}
		got[m[1]] = v
	}

	want := map[string]uint64{
		"JOYUP": 1 << JOY_BIT_UP, "JOYDOWN": 1 << JOY_BIT_DOWN,
		"JOYLEFT": 1 << JOY_BIT_LEFT, "JOYRIGHT": 1 << JOY_BIT_RIGHT,
		"JOYA": 1 << JOY_BIT_A, "JOYB": 1 << JOY_BIT_B,
		"JOYX": 1 << JOY_BIT_X, "JOYY": 1 << JOY_BIT_Y,
		"JOYLB": 1 << JOY_BIT_LB, "JOYRB": 1 << JOY_BIT_RB,
		"JOYLT": 1 << JOY_BIT_LT, "JOYRT": 1 << JOY_BIT_RT,
		"JOYSELECT": 1 << JOY_BIT_SELECT, "JOYSTART": 1 << JOY_BIT_START,
		"JOYL3": 1 << JOY_BIT_L3, "JOYR3": 1 << JOY_BIT_R3,
		"JOYHOME": 1 << JOY_BIT_HOME,
	}

	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("joydefs.bas missing %s", name)
			continue
		}
		if g != w {
			t.Errorf("joydefs.bas %s = %d, want %d", name, g, w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("joydefs.bas has stale %s not in Go constants", name)
		}
	}
}
