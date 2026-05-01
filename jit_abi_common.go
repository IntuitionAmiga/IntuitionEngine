// jit_abi_common.go - per-backend canonical JIT register ABI registry.
//
// Closure-plan F.2 disposition (RETIRED): the original Phase 7g plan
// was to converge the JIT and goasm-interp register layouts so a
// JIT→interp bail no longer spilled+reloaded the mapped GPR set.
// After Phase 7f generalized the MMIO poll matcher, bail rate on real
// workloads dropped sharply and the spill cost the convergence would
// save is small. The asm-interpreter ABI is therefore deliberately kept
// separate; bail-through-cpu.Step() is the supported path.
//
// This file remains as documentation of the JIT's canonical layout per
// backend. The constants in jit_<cpu>_abi.go pin those exact host
// registers on the JIT side, and jit_abi_consistency_test.go asserts
// that the per-backend constants match the entries below — the only
// invariant still enforced. No promise is made about the asm-interpreter
// matching this layout.

//go:build amd64 && (linux || windows || darwin)

package main

// CanonicalABISlot names a guest-register role; the value records the host
// (amd64) register pinned to that role under the canonical ABI for the
// backend. Roles not pinned by the backend (e.g. 6502 has no FP register
// role) are absent from the map.
type CanonicalABISlot string

const (
	// PC, A, X, Y are 6502 roles. Z80 / M68K / x86 roles use their own
	// names (HL, A, BC, DE for Z80; D0, A0 for M68K, etc.). The slot names
	// are documented per-backend in BackendCanonicalABI; this file does
	// not constrain the vocabulary.
	ABISlotPC          CanonicalABISlot = "PC"
	ABISlotAccumulator CanonicalABISlot = "A"
	ABISlotIndexX      CanonicalABISlot = "X"
	ABISlotIndexY      CanonicalABISlot = "Y"
	ABISlotStack       CanonicalABISlot = "SP"
	ABISlotStatus      CanonicalABISlot = "SR"
	ABISlotMemoryBase  CanonicalABISlot = "MEM"
	ABISlotCycleAcc    CanonicalABISlot = "CYC"
)

// BackendCanonicalABI is a per-backend table of slot → host register name
// (e.g. "RBX", "R12"). The asm-interpreter handlers and the JIT emitter
// must agree on every entry; mismatches are caught by the cross-check test
// in jit_abi_consistency_test.go (added by sub-phase 7g per backend).
//
// As of Phase 7g initial wiring, only the 6502 entry is recorded here as
// the reference; the other backends inherit empty maps until their
// per-backend audit lands. Adding a new backend's ABI is a one-line change
// here plus the corresponding *_interp_amd64.s and *_emit_amd64.go updates.
var BackendCanonicalABI = map[string]map[CanonicalABISlot]string{
	"6502": {
		// Canonical: align JIT and goasm-interp on the JIT's existing
		// layout (jit_6502_emit_amd64.go:31-36). The asm interpreter
		// (cpu_6502_interp_amd64.s) currently uses a different layout —
		// sub-phase 7g brings it onto this canonical assignment.
		ABISlotAccumulator: "RBX",
		ABISlotIndexX:      "RBP",
		ABISlotIndexY:      "R12",
		ABISlotStack:       "R13",
		ABISlotPC:          "R14",
		ABISlotStatus:      "R15",
	},
	// M68K — jit_m68k_emit_amd64.go:31-39. Slot vocabulary uses M68K
	// register names (D0/D1/A0/A7/CCR) plus shared roles for SP/SR/MEM/CTX.
	"m68k": {
		"D0":              "RBX",
		"D1":              "RBP",
		"A0":              "R12",
		ABISlotStack:      "R13", // A7/SP
		ABISlotStatus:     "R14", // CCR (5-bit XNZVC)
		ABISlotMemoryBase: "RSI",
		"DataBase":        "RDI",
		"AddrBase":        "R9",
	},
	// Z80 — jit_z80_emit_amd64.go:31-39. Pairs are packed 16-bit on the
	// low word of the host reg.
	"z80": {
		ABISlotAccumulator: "RBX", // A
		"F":                "RBP", // flags byte
		"BC":               "R12",
		"DE":               "R13",
		"HL":               "R14",
		ABISlotMemoryBase:  "RSI",
		"DPB":              "R8",
		"CPB":              "R9",
	},
	// x86 — jit_x86_emit_amd64.go:36-45. Five guest GPRs are pinned;
	// EBP/ESI/EDI live in the JITContext spill slots.
	"x86": {
		"EAX":             "RBX",
		"ECX":             "RBP",
		"EDX":             "R12",
		"EBX":             "R13",
		"ESP":             "R14",
		ABISlotMemoryBase: "RSI",
		"IOBM":            "R9",
	},
	// IE64 — jit_emit_amd64.go:22-27. Five mapped GPRs (R1, R2, R3, R4,
	// R31). R31 is SP. Unmapped IE64 regs (R5..R30) are spilled to
	// [RDI + ie64Reg*8] via emitLoadSpilledRegAMD64.
	"ie64": {
		"R1":         "RBX",
		"R2":         "RBP",
		"R3":         "R12",
		"R4":         "R13",
		ABISlotStack: "R14", // R31/SP
		ABISlotPC:    "R15", // PC / return channel
	},
}
