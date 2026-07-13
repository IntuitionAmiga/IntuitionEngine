//go:build js && !headless

// wasm_input_bridge.go - browser text-input bridge so the demo is typable on
// touch devices. Ebiten reads the keyboard from the canvas, but focusing a
// canvas never opens a mobile soft keyboard. The demo page overlays a real text
// input (which does open the IME) and forwards its characters and edit keys to
// ieTypeText/ieKey here, straight into the same guest keyboard path Ebiten
// feeds, so an injected key produces exactly the bytes a hardware key would.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"
)

var (
	wasmInputEmitter *EbitenOutput
	wasmTypeTextFunc js.Func
	wasmKeyFunc      js.Func
	wasmInputReady   bool
)

// wasmSpecialKeys maps the browser KeyboardEvent.key names the page forwards to
// the ebiten keys translateSpecialKey already understands, so an injected edit
// key yields the same byte sequence as the hardware key.
var wasmSpecialKeys = map[string]ebiten.Key{
	"Enter":      ebiten.KeyEnter,
	"Backspace":  ebiten.KeyBackspace,
	"Tab":        ebiten.KeyTab,
	"Escape":     ebiten.KeyEscape,
	"ArrowUp":    ebiten.KeyArrowUp,
	"ArrowDown":  ebiten.KeyArrowDown,
	"ArrowLeft":  ebiten.KeyArrowLeft,
	"ArrowRight": ebiten.KeyArrowRight,
	"Home":       ebiten.KeyHome,
	"End":        ebiten.KeyEnd,
	"Delete":     ebiten.KeyDelete,
}

// registerWasmInput exposes ieTypeText and ieKey on the global object for the
// given output. Idempotent once the callbacks are set.
func registerWasmInput(eo *EbitenOutput) {
	wasmInputEmitter = eo
	if wasmInputReady {
		return
	}
	global := js.Global()

	// ieTypeText(text string): inject printable characters, one guest input byte
	// per representable rune.
	wasmTypeTextFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if wasmInputEmitter == nil || len(args) < 1 {
			return nil
		}
		for _, r := range args[0].String() {
			if b, ok := runeToInputByte(r); ok {
				wasmInputEmitter.emitByte(b)
			}
		}
		return nil
	})
	global.Set("ieTypeText", wasmTypeTextFunc)

	// ieKey(name string): inject one edit or navigation key by its
	// KeyboardEvent.key name. Unknown names are ignored.
	wasmKeyFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if wasmInputEmitter == nil || len(args) < 1 {
			return nil
		}
		key, ok := wasmSpecialKeys[args[0].String()]
		if !ok {
			return nil
		}
		if seq, ok := translateSpecialKey(key); ok {
			wasmInputEmitter.emitSeq(seq)
		}
		return nil
	})
	global.Set("ieKey", wasmKeyFunc)

	wasmInputReady = true
}
