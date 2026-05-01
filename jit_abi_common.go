// jit_abi_common.go - per-backend canonical JIT↔interp register ABI
// (Phase 7g of the six-CPU JIT unification plan).
//
// Today, 6502 JIT pins (A→RBX, X→RBP, Y→R12, SP→R13, PC→R14, SR→R15) but
// the 6502 goasm interpreter pins a different layout
// (PC→R8, A→R10, X→R11, Y→R12, SP→DX, SR→R13, mem→SI, cycles→R9).
// Every JIT→interp bail therefore spills JIT-mapped registers to the CPU
// struct, calls into Go, then re-spills on return. This dominates
// I/O-bail-heavy code — exactly the workload tryFastMMIOPollLoop was added
// to rescue.
//
// Phase 7g picks one canonical register layout per backend, used by both
// the JIT emitter and the asm interpreter handlers. This file is the
// registry of those choices so a future audit ("does the 6502 JIT and
// goasm-interp agree?") is a single-file read.
//
// The actual ABI for each backend is enforced by:
//
//   - The JIT emitter's register-allocation table (jit_<cpu>_emit_amd64.go).
//   - The asm-interpreter prologue/epilogue (cpu_<cpu>_interp_amd64.s).
//   - This file's BackendCanonicalABI table (cross-check at test time).
//
// If a future edit changes one without the other, the layout-test in this
// file fails.

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
