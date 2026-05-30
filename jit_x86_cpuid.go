// jit_x86_cpuid.go - Runtime CPUID host feature detection for x86-64 JIT
//
// Detects host CPU features (BMI1, BMI2, AVX2, LZCNT, ERMS, FSRM) at init
// time via CPUID. Emitters check these flags to select optimal instruction
// encodings (e.g., SHLX instead of SHL when flags preservation is needed).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import "errors"

// cpuidRaw executes the CPUID instruction with the given EAX and ECX inputs.
// Implemented in jit_x86_cpuid_amd64.s.
func cpuidRaw(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

// x86HostFeatures records which host CPU extensions are available.
// Populated once at init time via CPUID; read by emitters during compilation.
type x86HostFeatures struct {
	HasSSE41    bool // SSE4.1 (ROUNDSS/ROUNDSD, PMULLD, BLENDV, ...) — IE64 JIT FINT uses ROUNDSS
	HasLAHFSAHF bool // LAHF/SAHF usable in 64-bit mode — x86 guest JIT uses them in REP/Jcc flag plumbing
	HasBMI1     bool // Bit Manipulation Instruction Set 1 (ANDN, BEXTR, BLSI, BLSMSK, BLSR, TZCNT)
	HasBMI2     bool // BMI2 (BZHI, MULX, PDEP, PEXT, RORX, SARX, SHLX, SHRX)
	HasAVX2     bool // Advanced Vector Extensions 2 (256-bit integer SIMD)
	HasLZCNT    bool // Leading Zero Count (LZCNT instruction, also implies TZCNT via BMI1)
	HasERMS     bool // Enhanced REP MOVSB/STOSB (fast short REP string ops)
	HasFSRM     bool // Fast Short REP MOVSB (even faster for short copies, Ice Lake+)
}

// x86Host is the detected host feature set. Initialized as a package-level
// variable (not in an init func) so that JIT-availability init funcs in other
// files — which read x86Host to gate their backends — observe the populated
// value regardless of init ordering.
var x86Host = detectX86HostFeatures()

// checkJITHostFeatures reports whether the host has every CPU feature the JIT
// emits unconditionally. Called from initJIT (when the JIT is actually turned
// on), so a missing feature degrades to the interpreter via initJIT's caller
// rather than aborting the process; interpreter-only / -nojit runs are
// unaffected.
//
// The IE64 amd64 JIT emits ROUNDSS (SSE4.1) for FINT with no per-emit gate.
// SSE4.1 ships on every x86-64 CPU since ~2008 (Intel Penryn / AMD Bulldozer),
// so this only fails on genuinely ancient hosts. Returning an error (instead of
// faulting with SIGILL inside generated code) lets all callers fall back to the
// interpreter.
func checkJITHostFeatures() error {
	if !x86Host.HasSSE41 {
		return errors.New("host CPU lacks SSE4.1 (ROUNDSS, used by IE64 FINT); " +
			"SSE4.1 has shipped on all x86-64 CPUs since ~2008 (Intel Penryn / AMD Bulldozer)")
	}
	return nil
}

// detectX86HostFeatures queries CPUID to determine available host extensions.
func detectX86HostFeatures() x86HostFeatures {
	var f x86HostFeatures

	// Check max CPUID leaf
	maxLeaf, _, _, _ := cpuidRaw(0, 0)

	// CPUID leaf 1: legacy feature flags (always available on x86-64).
	if maxLeaf >= 1 {
		_, _, ecx1, _ := cpuidRaw(1, 0)
		f.HasSSE41 = ecx1&(1<<19) != 0 // ECX bit 19
	}

	// CPUID leaf 7, subleaf 0: structured extended feature flags. Guarded
	// individually (not via an early return) so the extended-leaf read below
	// still runs on CPUs whose basic max leaf is < 7 but which still advertise
	// LAHF/SAHF and LZCNT in leaf 0x80000001.
	if maxLeaf >= 7 {
		_, ebx7, _, edx7 := cpuidRaw(7, 0)
		f.HasBMI1 = ebx7&(1<<3) != 0 // EBX bit 3
		f.HasAVX2 = ebx7&(1<<5) != 0 // EBX bit 5
		f.HasBMI2 = ebx7&(1<<8) != 0 // EBX bit 8
		f.HasERMS = ebx7&(1<<9) != 0 // EBX bit 9
		f.HasFSRM = edx7&(1<<4) != 0 // EDX bit 4
	}

	// CPUID leaf 0x80000001: extended feature flags. Independent of the basic
	// max leaf, so this must not sit behind a maxLeaf<7 bailout — x86JitAvailable
	// depends on HasLAHFSAHF here.
	maxExtLeaf, _, _, _ := cpuidRaw(0x80000000, 0)
	if maxExtLeaf >= 0x80000001 {
		_, _, ecx80, _ := cpuidRaw(0x80000001, 0)
		f.HasLAHFSAHF = ecx80&(1<<0) != 0 // ECX bit 0 (LAHF/SAHF in 64-bit mode)
		f.HasLZCNT = ecx80&(1<<5) != 0    // ECX bit 5 (ABM/LZCNT)
	}

	return f
}
