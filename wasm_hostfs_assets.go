// wasm_hostfs_assets.go - bootstrap hostfs asset seeding for the wasm build.
//
// BASIC's disk volume is the FileIO device, which the js/wasm build preloads
// from the demo's assets folder over HTTP (file_io_select_wasm.go). The
// BootstrapHostFS device is the IntuitionOS ROM bridge and needs no demo assets
// seeded, so this is a no-op. Nothing is embedded into the wasm binary here.

//go:build wasm

package main

func seedRuntimeHostFSAssets(_ *BootstrapHostFSDevice) {}
