// hostfs_select_wasm.go - runtime hostfs selection for the js/wasm build.
//
// The browser build has no host filesystem, so the hostfs device is backed by
// the in-memory specials store. The wasm entry seeds it with the embedded demo
// assets (Phase 4); the hostRoot argument is ignored.

//go:build wasm

package main

func newRuntimeHostFSDevice(bus *MachineBus, hostRoot string) *BootstrapHostFSDevice {
	_ = hostRoot
	return NewMemoryHostFSDevice(bus)
}
