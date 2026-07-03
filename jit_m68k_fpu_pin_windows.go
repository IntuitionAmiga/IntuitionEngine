//go:build amd64 && windows

package main

// m68kFPPinPlatformOK reports whether the M68K JIT may pin the guest FP register
// file into host xmm8-15 (see m68kFPPinned / m68kBlockRegs.fpPinned).
//
// FALSE on Windows: the Windows x64 ABI makes xmm6-xmm15 CALLEE-saved
// (nonvolatile), and the JIT call trampoline (jit_call_amd64_windows.s) saves
// only GPRs. A pinned FP loop that returns to Go would clobber the runtime's
// nonvolatile xmm8-15. Windows therefore keeps the memory-resident FP model,
// which touches only the volatile xmm0/xmm1 scratch registers. (Fixing this to
// re-enable pinning on Windows would require save/restore of xmm8-15 around the
// native block or in the trampoline.)
const m68kFPPinPlatformOK = false
