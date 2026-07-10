// wasm_hostfs_assets_native.go - no demo asset seeding on native builds.
//
// Native targets read from a real host filesystem, so there is nothing to seed
// into an in-memory store. This is the compile-time complement of
// wasm_hostfs_assets.go.

//go:build !wasm

package main

func seedRuntimeHostFSAssets(_ *BootstrapHostFSDevice) {}
