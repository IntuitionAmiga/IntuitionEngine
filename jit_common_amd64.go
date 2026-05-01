// jit_common_amd64.go — AMD64-specific scanner gates for jit_common.
//
// The AMD64 IE64 emitter understands the fused-leaf markers
// (ie64FusedJSRLeafCall, ie64FusedRTSLeafReturn) emitted by
// scanInstructions when a JSR resolves to a register-only leaf.
// Enable the scan-time fusion here.

//go:build amd64

package main

const ie64ScanJSRLeafFusionEnabled = true
