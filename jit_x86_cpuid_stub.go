// jit_x86_cpuid_stub.go - CPUID stub for non-amd64 platforms
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build !amd64 || (!linux && !windows && !darwin)

package main

// x86HostFeatures records which host CPU extensions are available.
type x86HostFeatures struct {
	HasSSE41    bool
	HasLAHFSAHF bool
	HasBMI1     bool
	HasBMI2     bool
	HasAVX2     bool
	HasLZCNT    bool
	HasERMS     bool
	HasFSRM     bool
}

// x86Host is always empty on non-amd64 platforms.
var x86Host x86HostFeatures

// checkJITHostFeatures is a no-op on non-amd64 hosts: the x86 SSE4.1
// requirement only applies to the amd64 JIT backend. The arm64 JIT has no
// equivalent unconditional feature dependency.
func checkJITHostFeatures() error { return nil }
