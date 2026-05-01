// Closure-plan F.3 disposition (DEFERRED): the original Phase 7h plan
// was to ship a full threaded-dispatch interpreter per backend so JIT
// bails would land on a fast asm handler instead of cpu.Step(). Cost is
// high (four generators, four sets of asm handlers, build-tag policy
// plus default-build promotion); benefit shrinks once Slice B regions
// land and JIT-bail frequency drops further. Per plan §7h escape
// hatch, this generator stays a scaffold (exit 2) until the Phase 9
// gate identifies a backend whose JIT-bail cost dominates real-workload
// time. cmd/gen_interp6502 is the working template.
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
