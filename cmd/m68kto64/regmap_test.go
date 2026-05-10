package main

import "testing"

func TestRegmap_Data(t *testing.T) {
	cases := map[string]string{
		"d0": "r1", "D0": "r1",
		"d1": "r2", "d2": "r3", "d3": "r4",
		"d4": "r5", "d5": "r6", "d6": "r7", "d7": "r8",
	}
	for in, want := range cases {
		r, ok := LookupRegister(in)
		if !ok {
			t.Fatalf("%q: not recognised", in)
		}
		if r.Class != RegData {
			t.Errorf("%q: class = %v, want RegData", in, r.Class)
		}
		if r.IE64 != want {
			t.Errorf("%q: IE64 = %q, want %q", in, r.IE64, want)
		}
	}
}

func TestRegmap_Addr(t *testing.T) {
	cases := map[string]string{
		"a0": "r9", "A0": "r9",
		"a1": "r10", "a2": "r11", "a3": "r12",
		"a4": "r13", "a5": "r14", "a6": "r15",
		"fp": "r15", "FP": "r15", // alias for a6
	}
	for in, want := range cases {
		r, ok := LookupRegister(in)
		if !ok {
			t.Fatalf("%q: not recognised", in)
		}
		if r.Class != RegAddr {
			t.Errorf("%q: class = %v, want RegAddr", in, r.Class)
		}
		if r.IE64 != want {
			t.Errorf("%q: IE64 = %q, want %q", in, r.IE64, want)
		}
	}
}

func TestRegmap_Stack(t *testing.T) {
	for _, name := range []string{"a7", "A7", "sp", "SP", "Sp"} {
		r, ok := LookupRegister(name)
		if !ok {
			t.Fatalf("%q: not recognised", name)
		}
		if r.Class != RegSP {
			t.Errorf("%q: class = %v, want RegSP", name, r.Class)
		}
		if !r.IsStack {
			t.Errorf("%q: IsStack false", name)
		}
		if r.IE64 != "r30" {
			t.Errorf("%q: IE64 = %q, want r30 (emulated guest stack, NOT r31)", name, r.IE64)
		}
	}
}

func TestRegmap_Special(t *testing.T) {
	for in, wantClass := range map[string]RegClass{
		"pc": RegPC, "PC": RegPC,
		"ccr": RegCCR, "sr": RegSR, "usp": RegUSP,
	} {
		r, ok := LookupRegister(in)
		if !ok {
			t.Fatalf("%q: not recognised", in)
		}
		if r.Class != wantClass {
			t.Errorf("%q: class = %v, want %v", in, r.Class, wantClass)
		}
		if r.IE64 != "" {
			t.Errorf("%q: IE64 = %q, want empty (no direct IE64 reg)", in, r.IE64)
		}
	}
}

func TestRegmap_Unknown(t *testing.T) {
	for _, in := range []string{"d8", "a8", "r0", "x", "", "foo", "d", "a"} {
		if _, ok := LookupRegister(in); ok {
			t.Errorf("%q: unexpectedly recognised", in)
		}
		if IsRegisterName(in) {
			t.Errorf("%q: IsRegisterName true, want false", in)
		}
	}
}

func TestRegmap_NoCollision(t *testing.T) {
	// d0..d7 + a0..a7 + sp/fp/pc/ccr/sr/usp must not collide on IE64 reg.
	seen := map[string]string{}
	for _, name := range []string{
		"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7",
		"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7",
	} {
		r, _ := LookupRegister(name)
		if prev, ok := seen[r.IE64]; ok {
			t.Errorf("collision: %s and %s both map to %s", prev, name, r.IE64)
		}
		seen[r.IE64] = name
	}
}
