// jit_x86_modrm_memo.go - per-block ModR/M memoization scaffold
// (Phase 7e of the six-CPU JIT unification plan).
//
// cpu_x86.go:72-74 already has modrmLoaded / sibLoaded shadow fields on
// the interpreter side: when a block decodes the same ModR/M byte and SIB
// twice, it skips the second decode. The JIT emitter does not have this
// memo today; every emit of an addressing-mode lowering re-walks the same
// ModR/M and SIB bytes from scratch.
//
// Phase 7e adds a per-block memo: while compiling block B, the emitter
// keeps a small map of "ModR/M byte at offset O has decoded form F".
// Subsequent decodes inside the same block hit the memo. The memo is
// per-block — block invalidation invalidates the memo automatically — so
// no extra bookkeeping at cache invalidation.
//
// This file is the scaffold: it declares the memo type and a builder.
// Wiring into jit_x86_emit_amd64.go is the follow-up patch.

//go:build amd64 && (linux || windows || darwin)

package main

// X86ModRMDecoded is the cached form of one decoded ModR/M (+ optional
// SIB + optional displacement). Matches the shape of the existing
// interpreter-side decode helpers in jit_x86_emit_amd64.go.
type X86ModRMDecoded struct {
	Mod    byte
	Reg    byte
	RM     byte
	HasSIB bool
	Scale  byte
	Index  byte
	Base   byte
	Disp   int32
	Bytes  uint8 // total bytes consumed (1=ModR/M only, up to 6 with SIB+disp32)
}

// X86ModRMMemo is per-block: keyed by guest PC offset of the ModR/M byte
// being decoded. Cleared on every block-emit start.
type X86ModRMMemo struct {
	entries map[uint32]X86ModRMDecoded
}

// NewX86ModRMMemo builds an empty memo with a small starting capacity.
// Most blocks decode 0-4 distinct ModR/M sites; the map grows on demand.
func NewX86ModRMMemo() *X86ModRMMemo {
	return &X86ModRMMemo{entries: make(map[uint32]X86ModRMDecoded, 4)}
}

// Lookup returns the memoized decode for a given guest PC, or
// (zero, false) if none.
func (m *X86ModRMMemo) Lookup(pc uint32) (X86ModRMDecoded, bool) {
	if m == nil || m.entries == nil {
		return X86ModRMDecoded{}, false
	}
	d, ok := m.entries[pc]
	return d, ok
}

// Store records a decode under the given guest PC. Idempotent under a
// given block — a re-store for the same PC is allowed and overwrites.
func (m *X86ModRMMemo) Store(pc uint32, d X86ModRMDecoded) {
	if m == nil {
		return
	}
	if m.entries == nil {
		m.entries = make(map[uint32]X86ModRMDecoded, 4)
	}
	m.entries[pc] = d
}

// Reset clears the memo. Called by the emitter at block-start.
func (m *X86ModRMMemo) Reset() {
	if m == nil {
		return
	}
	for k := range m.entries {
		delete(m.entries, k)
	}
}
