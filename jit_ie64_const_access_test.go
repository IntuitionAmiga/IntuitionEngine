package main

import "testing"

func TestIE64ConstLowRAMAccess(t *testing.T) {
	cases := []struct {
		name    string
		rs      byte
		imm     uint32
		size    byte
		wantOK  bool
		wantPtr uint64
	}{
		{"base-R0 low", 0, 0x100, IE64_SIZE_L, true, 0x100},
		{"base-reg not const", 1, 0x100, IE64_SIZE_L, false, 0},
		{"negative disp", 0, 0xFFFFFFFC, IE64_SIZE_B, false, 0}, // int32 = -4
		{"last byte just below IO", 0, IO_REGION_START - 1, IE64_SIZE_B, true, IO_REGION_START - 1},
		{"starts at IO region", 0, IO_REGION_START, IE64_SIZE_B, false, 0},
		{"quad spills into IO", 0, IO_REGION_START - 4, IE64_SIZE_Q, false, 0},
		{"quad fits below IO", 0, IO_REGION_START - 8, IE64_SIZE_Q, true, IO_REGION_START - 8},
	}
	for _, c := range cases {
		got, ok := ie64ConstLowRAMAccess(c.rs, c.imm, c.size)
		if ok != c.wantOK || (ok && got != c.wantPtr) {
			t.Errorf("%s: got (0x%x, %v), want (0x%x, %v)", c.name, got, ok, c.wantPtr, c.wantOK)
		}
	}
}
