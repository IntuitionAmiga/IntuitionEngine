// jit_x86_cpuid.go - Runtime CPUID host feature detection for x86-64 JIT
//
// Detects host CPU features (BMI1, BMI2, AVX2, LZCNT, ERMS, FSRM) at init
// time via CPUID. Emitters check these flags to select optimal instruction
// encodings (e.g., SHLX instead of SHL when flags preservation is needed).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && linux

package main

// cpuidRaw executes the CPUID instruction with the given EAX and ECX inputs.
// Implemented in jit_x86_cpuid_amd64.s.
func cpuidRaw(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

// x86HostFeatures records which host CPU extensions are available.
// Populated once at init time via CPUID; read by emitters during compilation.
type x86HostFeatures struct {
	HasBMI1  bool // Bit Manipulation Instruction Set 1 (ANDN, BEXTR, BLSI, BLSMSK, BLSR, TZCNT)
	HasBMI2  bool // BMI2 (BZHI, MULX, PDEP, PEXT, RORX, SARX, SHLX, SHRX)
	HasAVX2  bool // Advanced Vector Extensions 2 (256-bit integer SIMD)
	HasLZCNT bool // Leading Zero Count (LZCNT instruction, also implies TZCNT via BMI1)
	HasERMS  bool // Enhanced REP MOVSB/STOSB (fast short REP string ops)
	HasFSRM  bool // Fast Short REP MOVSB (even faster for short copies, Ice Lake+)
}

// x86Host is the detected host feature set. Initialized in init().
var x86Host x86HostFeatures

func init() {
	x86Host = detectX86HostFeatures()
}

// detectX86HostFeatures queries CPUID to determine available host extensions.
func detectX86HostFeatures() x86HostFeatures {
	var f x86HostFeatures

	// Check max CPUID leaf
	maxLeaf, _, _, _ := cpuidRaw(0, 0)
	if maxLeaf < 7 {
		return f
	}

	// CPUID leaf 7, subleaf 0: structured extended feature flags
	_, ebx7, _, edx7 := cpuidRaw(7, 0)
	f.HasBMI1 = ebx7&(1<<3) != 0 // EBX bit 3
	f.HasAVX2 = ebx7&(1<<5) != 0 // EBX bit 5
	f.HasBMI2 = ebx7&(1<<8) != 0 // EBX bit 8
	f.HasERMS = ebx7&(1<<9) != 0 // EBX bit 9
	f.HasFSRM = edx7&(1<<4) != 0 // EDX bit 4

	// CPUID leaf 0x80000001: extended feature flags
	maxExtLeaf, _, _, _ := cpuidRaw(0x80000000, 0)
	if maxExtLeaf >= 0x80000001 {
		_, _, ecx80, _ := cpuidRaw(0x80000001, 0)
		f.HasLZCNT = ecx80&(1<<5) != 0 // ECX bit 5 (ABM/LZCNT)
	}

	return f
}
