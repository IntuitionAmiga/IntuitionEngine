// gen_interp_ie64 - generator scaffold for the IE64 threaded-dispatch
// interpreter (Phase 7h of the six-CPU JIT unification plan).
//
// Mirrors cmd/gen_interp6502 + the canonical IE64 ABI in jit_ie64_abi.go.
// IE64 has a fixed 32-bit instruction encoding so the dispatch table
// keys on the primary opcode field.
//
// Scaffold today.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gen_interp_ie64: scaffold — not yet implemented.")
	fmt.Fprintln(os.Stderr, "Canonical ABI in jit_ie64_abi.go.")
	os.Exit(2)
}
