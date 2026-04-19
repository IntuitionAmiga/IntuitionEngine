// jit_x86_cpuid_stub.go - CPUID stub for non-amd64 platforms
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build !amd64 || (!linux && !windows && !darwin)

package main

// x86HostFeatures records which host CPU extensions are available.
type x86HostFeatures struct {
	HasBMI1  bool
	HasBMI2  bool
	HasAVX2  bool
	HasLZCNT bool
	HasERMS  bool
	HasFSRM  bool
}

// x86Host is always empty on non-amd64 platforms.
var x86Host x86HostFeatures
