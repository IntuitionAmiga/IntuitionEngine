// m68k_jit_bcc_fusion_nonlinux_test.go - non-Linux parity-test stubs.

//go:build amd64 && !linux && (windows || darwin)

package main

import "testing"

func bccFusionCompare(t *testing.T, name string, program []uint16) {
	t.Helper()
	t.Skip("M68K Bcc fusion parity harness uses linux-only benchmark fixtures")
}
