// gen_stubs writes the per-CPU park-loop binaries the iemon child
// wrappers load. Each binary, once loaded at the per-CPU default load
// address, puts the CPU in a tight branch-to-self loop. The wrapper's
// `dbg.open()` then freezes execution at whichever loop instruction the
// CPU happened to be running.
//
// Why park loops (and not just HALT/STOP instructions):
//   - Z80 `HALT` latches CPU_Z80.Halted; the monitor's `r pc <addr>` does
//     not clear the latch, so a subsequent `g` would no-op.
//   - M68K `STOP` latches `stopped` until an interrupt — same problem.
//   - 6502/x86/RISC: there is no portable halt; a self-jump is the only
//     correct neutral state.
//
// Output: sdk/scripts/prm-runner/stubs/{6502,z80,m68k,x86,ie32,ie64}.bin
// plus the deterministic breakpoint smoke at 6502_bp_test.bin.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("o", "sdk/scripts/prm-runner/stubs", "output dir")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	stubs := map[string][]byte{
		// 6502: default load addr is 0x0600 (main.go:351).
		//   Byte 0 at runtime = $0600 = `4C 00 06` = JMP $0600.
		"6502.bin": {0x4C, 0x00, 0x06},

		// Z80: default load addr is 0x0000 (per -load-addr default for
		// Z80 mode in main.go). Byte 0 = $0000 = `18 FE` = JR -2.
		"z80.bin": {0x18, 0xFE},

		// M68K: load addr at $0 puts the entry at offset 0 in the binary.
		// `60 FE` = BRA.S -2 (branch to self).
		"m68k.bin": {0x60, 0xFE},

		// x86: real-mode segment 0:0, byte 0 = `EB FE` = JMP $-2.
		"x86.bin": {0xEB, 0xFE},

		// IE32/IE64: branch-to-self differs by ISA. Until we have the
		// canonical encoding documented, write a placeholder of all-zero
		// bytes (most ISAs trap, which the monitor surfaces as a fault we
		// can diagnose) — replace with the proper encoding once a single
		// `dbg.open()`-friendly self-branch is confirmed for each.
		"ie32.bin": {0, 0, 0, 0},
		"ie64.bin": {0, 0, 0, 0, 0, 0, 0, 0},

		// Deterministic linear stub for the (6502)> b $1234 / g / r
		// breakpoint smoke. Loaded at $0600 by default; we place the
		// entry slide so PC reaches $1234 by NOP-walking from $1000.
		// Layout: header bytes 0..$09FF = NOPs (so $0600..$09FF NOP),
		// then $0A00..$0BFF = NOPs reaching $1000? — no, byte i lives
		// at $0600+i, so for $1234 to be NOP-reachable we'd need
		// 0x0C34 bytes. Easier path: ship a stub that, when loaded at
		// $0600, reads `4C 00 10` at $0600 = JMP $1000, then a sea of
		// NOPs from $1000..$1233, then BRK ($00) at $1234 = the test's
		// breakpoint target.
		"6502_bp_test.bin": buildSixBPTestStub(),
	}
	for name, data := range stubs {
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(fmt.Errorf("write %s: %w", path, err))
		}
		fmt.Printf("  wrote %s (%d bytes)\n", path, len(data))
	}
}

// buildSixBPTestStub assembles the 6502 breakpoint smoke binary.
//
// Load address: $0600. The binary contains the JMP and the NOP slide
// reachable from $1000, with the breakpoint target instruction at $1234.
// Offsets in the binary:
//
//	offset 0x0000 ($0600)  : 4C 00 10        JMP $1000
//	offsets 0x0A00..0x0C33 : 0xEA (NOP)       fill from $1000..$1233
//	offset  0x0C34 ($1234) : 0xEA (NOP)       — the breakpoint target
//	offsets 0x0C35..0x0C3F : 0xEA (NOP)       small tail so PC keeps moving
func buildSixBPTestStub() []byte {
	const loadBase = 0x0600
	const target = 0x1234
	const tailEnd = target + 16
	size := tailEnd - loadBase + 1
	buf := make([]byte, size)
	// JMP $1000 at $0600.
	buf[0x0000] = 0x4C
	buf[0x0001] = 0x00
	buf[0x0002] = 0x10
	// NOP slide from $1000 ($0A00 in the binary) up through tailEnd.
	for off := 0x1000 - loadBase; off < size; off++ {
		buf[off] = 0xEA
	}
	return buf
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen_stubs:", err)
	os.Exit(1)
}
