// jit_wasm_supported_other.go - wasm JIT capability marker, non-browser side.

//go:build !(js && wasm)

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// wasmJITSupported is true only on js/wasm, where the IE64 wasm bytecode
// backend exists. It lets shared wiring (main, the programme executor) treat
// "some IE64 JIT exists" uniformly: on the browser build jitAvailable is
// false (no native code execution) but the wasm backend still counts.
const wasmJITSupported = false
