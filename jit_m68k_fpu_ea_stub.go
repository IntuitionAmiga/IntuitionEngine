//go:build !(amd64 && (linux || windows || darwin))

package main

// m68kJITHasSSE41 is false on non-amd64 M68K JIT builds. Shared scanner code
// uses it to keep SSE4.1-only FPU operations on the helper path.
var m68kJITHasSSE41 = false
