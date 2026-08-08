//go:build js && wasm

package main

import "syscall/js"

func ie32JITRuntimeAvailable() bool { return js.Global().Get("__goMem").Truthy() }
