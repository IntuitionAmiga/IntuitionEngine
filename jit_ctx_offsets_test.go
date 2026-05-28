// jit_ctx_offsets_test.go - Pin JITContext field offsets used by emitted
// native code.
//
// Both AMD64 and ARM64 emitters hard-code byte offsets into the JITContext
// struct (jitCtxOff* constants in jit_common.go). If a future change adds,
// removes, or reorders a field without updating the constants, the JIT
// would silently read/write the wrong memory. This test catches that at
// `go test` time by checking every constant against unsafe.Offsetof.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"testing"
	"unsafe"
)

func TestJITContext_FieldOffsets(t *testing.T) {
	var ctx JITContext
	cases := []struct {
		name string
		want uintptr
		got  uintptr
	}{
		{"RegsPtr", jitCtxOffRegsPtr, unsafe.Offsetof(ctx.RegsPtr)},
		{"MemPtr", jitCtxOffMemPtr, unsafe.Offsetof(ctx.MemPtr)},
		{"MemSize", jitCtxOffMemSize, unsafe.Offsetof(ctx.MemSize)},
		{"IOStart", jitCtxOffIOStart, unsafe.Offsetof(ctx.IOStart)},
		{"PCPtr", jitCtxOffPCPtr, unsafe.Offsetof(ctx.PCPtr)},
		{"LoadMemFn", jitCtxOffLoadMemFn, unsafe.Offsetof(ctx.LoadMemFn)},
		{"StoreMemFn", jitCtxOffStoreMemFn, unsafe.Offsetof(ctx.StoreMemFn)},
		{"CpuPtr", jitCtxOffCpuPtr, unsafe.Offsetof(ctx.CpuPtr)},
		{"NeedInval", jitCtxOffNeedInval, unsafe.Offsetof(ctx.NeedInval)},
		{"NeedIOFallback", jitCtxOffNeedIOFallback, unsafe.Offsetof(ctx.NeedIOFallback)},
		{"IOBitmapPtr", jitCtxOffIOBitmapPtr, unsafe.Offsetof(ctx.IOBitmapPtr)},
		{"FPUPtr", jitCtxOffFPUPtr, unsafe.Offsetof(ctx.FPUPtr)},
		{"ChainBudget", jitCtxOffChainBudget, unsafe.Offsetof(ctx.ChainBudget)},
		{"ChainCount", jitCtxOffChainCount, unsafe.Offsetof(ctx.ChainCount)},
		{"RTSCache0PC", jitCtxOffRTSCache0PC, unsafe.Offsetof(ctx.RTSCache0PC)},
		{"RTSCache0Addr", jitCtxOffRTSCache0Addr, unsafe.Offsetof(ctx.RTSCache0Addr)},
		{"RTSCache1PC", jitCtxOffRTSCache1PC, unsafe.Offsetof(ctx.RTSCache1PC)},
		{"RTSCache1Addr", jitCtxOffRTSCache1Addr, unsafe.Offsetof(ctx.RTSCache1Addr)},
		{"RTSCache2PC", jitCtxOffRTSCache2PC, unsafe.Offsetof(ctx.RTSCache2PC)},
		{"RTSCache2Addr", jitCtxOffRTSCache2Addr, unsafe.Offsetof(ctx.RTSCache2Addr)},
		{"RTSCache3PC", jitCtxOffRTSCache3PC, unsafe.Offsetof(ctx.RTSCache3PC)},
		{"RTSCache3Addr", jitCtxOffRTSCache3Addr, unsafe.Offsetof(ctx.RTSCache3Addr)},
		{"RetPC", jitCtxOffRetPC, unsafe.Offsetof(ctx.RetPC)},
		{"RetCount", jitCtxOffRetCount, unsafe.Offsetof(ctx.RetCount)},
		{"MMUEnabled", jitCtxOffMMUEnabled, unsafe.Offsetof(ctx.MMUEnabled)},
		{"NeedHelper", jitCtxOffNeedHelper, unsafe.Offsetof(ctx.NeedHelper)},
		{"HelperSize", jitCtxOffHelperSize, unsafe.Offsetof(ctx.HelperSize)},
		{"HelperRd", jitCtxOffHelperRd, unsafe.Offsetof(ctx.HelperRd)},
		{"HelperAddr", jitCtxOffHelperAddr, unsafe.Offsetof(ctx.HelperAddr)},
		{"HelperVal", jitCtxOffHelperVal, unsafe.Offsetof(ctx.HelperVal)},
		{"HelperPC", jitCtxOffHelperPC, unsafe.Offsetof(ctx.HelperPC)},
		{"LiveSP", jitCtxOffLiveSP, unsafe.Offsetof(ctx.LiveSP)},
	}
	for _, c := range cases {
		if c.want != c.got {
			t.Errorf("jitCtxOff%s = %d, unsafe.Offsetof = %d", c.name, c.want, c.got)
		}
	}
}
