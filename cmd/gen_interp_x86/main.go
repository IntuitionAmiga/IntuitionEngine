// gen_interp_x86 - generator scaffold for the x86 threaded-dispatch
// interpreter (Phase 7h of the six-CPU JIT unification plan).
//
// Plan note: x86 has prefix-byte combinatorics that make full coverage
// expensive. Phase 7h's "scoped variant" rule applies: this generator
// initially covers only the dense common-case 1-byte opcode subset; rare
// or prefix-heavy opcodes still go through the existing Go path.
//
// Scaffold today.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gen_interp_x86: scaffold — not yet implemented.")
	fmt.Fprintln(os.Stderr, "Initial scope per plan: dense 1-byte opcodes only.")
	fmt.Fprintln(os.Stderr, "Canonical ABI in jit_x86_abi.go.")
	os.Exit(2)
}
