// jit_wasm_ctx_layout_test.go - JITContext ABI guard for the wasm backend.
//
// The wasm translator addresses JITContext fields through the jitCtxOff*
// constants, exactly like the native emitters. GOARCH=wasm is a 64-bit Go
// target (PtrSize 8), so the offsets hold there too; this test pins that
// fact on every platform the suite runs on, including GOOS=js under node.
// Any future field reorder or pointer-size divergence fails here loudly
// instead of corrupting CPU state at runtime.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"testing"
	"unsafe"
)

func TestWasmJIT_CtxLayout(t *testing.T) {
	var ctx JITContext
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"RegsPtr", unsafe.Offsetof(ctx.RegsPtr), jitCtxOffRegsPtr},
		{"MemPtr", unsafe.Offsetof(ctx.MemPtr), jitCtxOffMemPtr},
		{"MemSize", unsafe.Offsetof(ctx.MemSize), jitCtxOffMemSize},
		{"IOStart", unsafe.Offsetof(ctx.IOStart), jitCtxOffIOStart},
		{"PCPtr", unsafe.Offsetof(ctx.PCPtr), jitCtxOffPCPtr},
		{"LoadMemFn", unsafe.Offsetof(ctx.LoadMemFn), jitCtxOffLoadMemFn},
		{"StoreMemFn", unsafe.Offsetof(ctx.StoreMemFn), jitCtxOffStoreMemFn},
		{"CpuPtr", unsafe.Offsetof(ctx.CpuPtr), jitCtxOffCpuPtr},
		{"NeedInval", unsafe.Offsetof(ctx.NeedInval), jitCtxOffNeedInval},
		{"NeedIOFallback", unsafe.Offsetof(ctx.NeedIOFallback), jitCtxOffNeedIOFallback},
		{"IOBitmapPtr", unsafe.Offsetof(ctx.IOBitmapPtr), jitCtxOffIOBitmapPtr},
		{"FPUPtr", unsafe.Offsetof(ctx.FPUPtr), jitCtxOffFPUPtr},
		{"ChainBudget", unsafe.Offsetof(ctx.ChainBudget), jitCtxOffChainBudget},
		{"ChainCount", unsafe.Offsetof(ctx.ChainCount), jitCtxOffChainCount},
		{"RTSCache0PC", unsafe.Offsetof(ctx.RTSCache0PC), jitCtxOffRTSCache0PC},
		{"RTSCache0Addr", unsafe.Offsetof(ctx.RTSCache0Addr), jitCtxOffRTSCache0Addr},
		{"RTSCache1PC", unsafe.Offsetof(ctx.RTSCache1PC), jitCtxOffRTSCache1PC},
		{"RTSCache1Addr", unsafe.Offsetof(ctx.RTSCache1Addr), jitCtxOffRTSCache1Addr},
		{"RTSCache2PC", unsafe.Offsetof(ctx.RTSCache2PC), jitCtxOffRTSCache2PC},
		{"RTSCache2Addr", unsafe.Offsetof(ctx.RTSCache2Addr), jitCtxOffRTSCache2Addr},
		{"RTSCache3PC", unsafe.Offsetof(ctx.RTSCache3PC), jitCtxOffRTSCache3PC},
		{"RTSCache3Addr", unsafe.Offsetof(ctx.RTSCache3Addr), jitCtxOffRTSCache3Addr},
		{"RetPC", unsafe.Offsetof(ctx.RetPC), jitCtxOffRetPC},
		{"RetCount", unsafe.Offsetof(ctx.RetCount), jitCtxOffRetCount},
		{"MMUEnabled", unsafe.Offsetof(ctx.MMUEnabled), jitCtxOffMMUEnabled},
		{"NeedHelper", unsafe.Offsetof(ctx.NeedHelper), jitCtxOffNeedHelper},
		{"HelperSize", unsafe.Offsetof(ctx.HelperSize), jitCtxOffHelperSize},
		{"HelperRd", unsafe.Offsetof(ctx.HelperRd), jitCtxOffHelperRd},
		{"HelperAddr", unsafe.Offsetof(ctx.HelperAddr), jitCtxOffHelperAddr},
		{"HelperVal", unsafe.Offsetof(ctx.HelperVal), jitCtxOffHelperVal},
		{"HelperPC", unsafe.Offsetof(ctx.HelperPC), jitCtxOffHelperPC},
		{"LiveSP", unsafe.Offsetof(ctx.LiveSP), jitCtxOffLiveSP},
		{"ResumeAddr", unsafe.Offsetof(ctx.ResumeAddr), jitCtxOffResumeAddr},
		{"ResumePC", unsafe.Offsetof(ctx.ResumePC), jitCtxOffResumePC},
		{"ResumePTBR", unsafe.Offsetof(ctx.ResumePTBR), jitCtxOffResumePTBR},
		{"ResumeCountBase", unsafe.Offsetof(ctx.ResumeCountBase), jitCtxOffResumeCountBase},
		{"ResumeMMUEnabled", unsafe.Offsetof(ctx.ResumeMMUEnabled), jitCtxOffResumeMMUEnabled},
		{"ResumeValid", unsafe.Offsetof(ctx.ResumeValid), jitCtxOffResumeValid},
		{"MicroTLBReadPrefix", unsafe.Offsetof(ctx.MicroTLBReadPrefix), jitCtxOffMicroTLBReadPrefix},
		{"MicroTLBWritePrefix", unsafe.Offsetof(ctx.MicroTLBWritePrefix), jitCtxOffMicroTLBWritePrefix},
		{"MicroTLBKeys", unsafe.Offsetof(ctx.MicroTLBKeys), jitCtxOffMicroTLBKeys},
		{"MicroTLBPhys", unsafe.Offsetof(ctx.MicroTLBPhys), jitCtxOffMicroTLBPhys},
		{"CodePageBitmapPtr", unsafe.Offsetof(ctx.CodePageBitmapPtr), jitCtxOffCodePageBitmapPtr},
		{"InvalAddr", unsafe.Offsetof(ctx.InvalAddr), jitCtxOffInvalAddr},
		{"InvalSize", unsafe.Offsetof(ctx.InvalSize), jitCtxOffInvalSize},
		{"CodePageBitmapLen", unsafe.Offsetof(ctx.CodePageBitmapLen), jitCtxOffCodePageBitmapLen},
		{"CodeHighStartPage", unsafe.Offsetof(ctx.CodeHighStartPage), jitCtxOffCodeHighStartPage},
		{"CodeHighEndPage", unsafe.Offsetof(ctx.CodeHighEndPage), jitCtxOffCodeHighEndPage},
		{"PhysCodeBitmapPtr", unsafe.Offsetof(ctx.PhysCodeBitmapPtr), jitCtxOffPhysCodeBitmapPtr},
		{"PhysCodeBitmapLen", unsafe.Offsetof(ctx.PhysCodeBitmapLen), jitCtxOffPhysCodeBitmapLen},
		{"CodePageSpansPtr", unsafe.Offsetof(ctx.CodePageSpansPtr), jitCtxOffCodePageSpansPtr},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("JITContext.%s at offset %d, jitCtxOff constant says %d", c.name, c.got, c.want)
		}
	}
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Errorf("uintptr is %d bytes; the wasm backend assumes the 64-bit JITContext layout", unsafe.Sizeof(uintptr(0)))
	}
}
