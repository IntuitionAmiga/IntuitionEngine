//go:build headless || wasm

// No desktop clipboard on headless builds or in the browser (js/wasm), so
// terminal clipboard integration is a no-op. This constraint is the exact
// complement of video_terminal_clipboard.go (!headless && !wasm).

package main

func initTerminalClipboard(_ *VideoTerminal) {}

// readPrimarySelection has no desktop X primary selection to read in the
// browser or headless; the debug overlay's middle-click paste is empty.
func readPrimarySelection() []byte { return nil }
