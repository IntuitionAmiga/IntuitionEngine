// gen_interp_m68k - generator scaffold for the M68K threaded-dispatch
// interpreter (Phase 7h of the six-CPU JIT unification plan).
//
// Plan note: M68K has variable-length encoding and dense effective-address
// modes that make full coverage expensive. Phase 7h's "scoped variant"
// rule applies: this generator initially covers only the dense
// fixed-shape opcode subset (MOVE/ADD/SUB/CMP/Bcc/JMP common forms);
// rare or extension-word-heavy opcodes still go through the existing Go
// path. Canonical ABI in jit_m68k_abi.go.
//
// Scaffold today.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gen_interp_m68k: scaffold — not yet implemented.")
	fmt.Fprintln(os.Stderr, "Initial scope per plan: dense fixed-shape opcodes only.")
	fmt.Fprintln(os.Stderr, "Canonical ABI in jit_m68k_abi.go.")
	os.Exit(2)
}
