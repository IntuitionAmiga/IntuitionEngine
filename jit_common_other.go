// jit_common_other.go — non-AMD64 scanner gates for jit_common.
//
// On non-AMD64 builds the IE64 emitter (jit_emit_arm64.go) does not
// honor the fused-leaf markers; emitting them would result in a real
// JSR + inlined body + real RTS, corrupting stack semantics. Disable
// the scan-time fusion so the scanner emits a plain JSR and the
// emitter handles it normally.
//
// Codex review (2026-04-30) flagged the original untagged scanner as
// breaking the supported ARM64 IE64 JIT. This gate closes that gap.
// When the ARM64 emitter learns to honor the fused markers, flip this
// constant to true (or move to a per-arch file under that arch's tag).

//go:build !amd64

package main

const ie64ScanJSRLeafFusionEnabled = false
