// hostfs_select_native.go - runtime hostfs selection for native builds.
//
// Native targets have a real host filesystem, so the bootstrap hostfs bridge
// resolves a host root. The js/wasm build has none and uses the in-memory
// store instead (hostfs_select_wasm.go).

//go:build !wasm

package main

func newRuntimeHostFSDevice(bus *MachineBus, hostRoot string) *BootstrapHostFSDevice {
	return NewBootstrapHostFSDevice(bus, hostRoot)
}
