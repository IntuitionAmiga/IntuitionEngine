module github.com/intuitionamiga/IntuitionEngine

go 1.26.0

// Pinned: the SIMD kernels use the experimental simd/archsimd package on the
// default make build path. That API is not covered by the Go 1 compatibility
// promise, so toolchain drift can break the build; pin the toolchain that ships
// the probed API surface.
toolchain go1.26.4

require (
	github.com/ebitengine/oto/v3 v3.5.0-alpha.9
	github.com/ebitengine/purego v0.11.0-alpha.8
	github.com/godbus/dbus/v5 v5.2.2
	github.com/goki/vulkan v1.0.8
	github.com/hajimehoshi/ebiten/v2 v2.10.0-alpha.13
	github.com/tetratelabs/wazero v1.12.0
	golang.org/x/image v0.44.0
	golang.org/x/term v0.45.0
)

require github.com/jfreymuth/pulse v0.1.2 // indirect

require (
	github.com/ebitengine/gomobile v0.0.0-20260211053922-3d992dae95d1 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/yuin/gopher-lua v1.1.2
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0
)
