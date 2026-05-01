// gen_interp_z80 - generator scaffold for the Z80 threaded-dispatch
// interpreter (Phase 7h of the six-CPU JIT unification plan).
//
// Mirrors cmd/gen_interp6502 — emits a 256-entry dispatch table plus
// per-opcode NOSPLIT asm handlers using the canonical Z80 ABI declared
// in jit_z80_abi.go. Phase-7h initial wiring is gated by build tag
// `interpz80full`; once parity is green and the bench delta meets the
// ≥5% target the dispatch is moved to the default amd64 build.
//
// Scaffold body: prints a status banner explaining the generator is not
// yet wired. Phase-7h sub-phase 7h-z80 fills in the real generator using
// gen_interp6502 as the template.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gen_interp_z80: scaffold — not yet implemented.")
	fmt.Fprintln(os.Stderr, "Phase 7h sub-phase wires this generator using")
	fmt.Fprintln(os.Stderr, "cmd/gen_interp6502 as the template + the canonical")
	fmt.Fprintln(os.Stderr, "Z80 ABI from jit_z80_abi.go.")
	os.Exit(2)
}
