// jit_ie64_flags_liveness.go - per-instruction flag liveness for IE64 JIT
// (Phase 2c of the six-CPU JIT unification plan).
//
// IE64 has CR-flag bits set by CMP/arithmetic and consumed by Bcc.
// Sibling backends already have liveness scaffolds (6502, M68K, Z80,
// x86); this file completes the per-backend coverage so Phase 4 region
// superblocks have the uniform contract on every backend.
//
// IE64's JIT emit/codegen is amd64-specific today (no per-instruction
// JITInstr struct exported yet) so this scaffold uses a thin opaque
// slot type that the future emitter populates. The function shape
// matches the other backends' analyzers.

//go:build amd64 && (linux || windows || darwin)

package main

// IE64JITSlot is a placeholder for the per-instruction JIT IR slot the
// IE64 emitter will publish in Phase 4. It carries enough metadata for
// liveness analysis without leaking the rest of the codegen private
// state. Phase 2c scaffold: empty body — populated when IE64 JIT IR
// lands.
type IE64JITSlot struct {
	_ [0]byte
}

// ie64FlagsLiveness returns, for each slot i in instrs, whether the
// CR-flag output of instrs[i] is consumed by a downstream instruction
// in the same block. Phase 2c initial wiring is conservative — every
// slot is true. Tightens once the IE64 JIT IR lands.
func ie64FlagsLiveness(instrs []IE64JITSlot) JITFlagLiveness {
	if len(instrs) == 0 {
		return nil
	}
	live := make(JITFlagLiveness, len(instrs))
	for i := range live {
		live[i] = true
	}
	return live
}

// ie64FlagsConsumers reports whether the IE64 instruction at instrs[i]
// is a CR-flag consumer (Bcc, conditional moves). Phase 2c: returns
// true conservatively.
func ie64FlagsConsumers(instrs []IE64JITSlot, i int) bool {
	_ = instrs
	_ = i
	return true
}
