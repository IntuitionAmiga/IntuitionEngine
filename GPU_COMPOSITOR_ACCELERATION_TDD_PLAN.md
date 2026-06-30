# Full GPU Compositor Acceleration With TDD

## Summary

Move normal Ebiten presentation compositing off the CPU by uploading native source layers and drawing scaled quads on the GPU. Keep the current CPU compositor as the deterministic reference and fallback for headless, snapshots, diagnostics, and hardware failure.

"Fully accelerated" means the normal Ebiten display path does not call `blendFrameScaled` or build a presentation-sized CPU frame. CPU composition remains allowed for headless, `GetCurrentFrame`, `rec.start`, tests, and fallback.

## Key Changes

- Add an optional output interface while leaving `VideoOutput` unchanged:
  - `HardwareCompositingOutput.UpdateHardwareCompositorFrame(CompositorFrameUpdate) error`
  - `HardwareCompositingOutput.HardwareCompositorSnapshot(frameID uint64) ([]byte, bool)`
  - `CompositorFrameUpdate` carries frame ID, presentation size, `HasContent`, and ordered native layers with source ID, source size, destination rect, and RGBA buffer.
- Refactor the compositor so CPU and GPU paths share one collected layer list, including full-frame and scanline-aware sources.
- In `composite()`, prefer hardware output when available and enabled; after the first runtime hardware update error, disable hardware compositing and use the existing CPU path until `SetDisplayConfig` or process restart.
- Implement Ebiten hardware composition with staged native layer buffers, reusable textures, and `DrawTrianglesShader`/`DrawTrianglesShader32` scaled quads.
- Use a Kage pixel shader that exactly preserves CPU semantics:
  - all-zero source pixels call `discard()`;
  - zero-alpha nonzero-RGB pixels return alpha `1.0`;
  - partial-alpha pixels are copied unchanged;
  - visible pixels are drawn with `BlendCopy`;
  - the shader computes `srcX=floor(localX*srcW/rectW)` and `srcY=floor(localY*srcH/rectH)`, clamps to source bounds, and samples that exact texel.
- Preserve compatibility:
  - `GetCurrentFrame`/screenshots use a lazy per-frame CPU reference snapshot cache after hardware frames.
  - `rec.start` keeps compositor-frame semantics; `rec.start_screen` keeps screen readback semantics.
  - `IE_DISABLE_GPU_COMPOSITOR=1` forces the legacy CPU compositor path.

## TDD Plan

- First add failing compositor tests with a fake `HardwareCompositingOutput`:
  - default `960x540 -> 1920x1080` sends one native layer and does not call `UpdateFrame`;
  - 4:3 aspect-fit and stretch-fill rects match existing mapping;
  - hardware-reference output matches CPU output for scale boundaries, z-order, transparent zero, zero-alpha RGB promotion, partial alpha, scanline sources, and no-content clearing;
  - hardware update failure falls back for the current frame and disables hardware for later frames;
  - repeated `GetCurrentFrame` calls after one hardware frame reuse the cached snapshot.
- Add a test hook/counter so accelerated happy-path tests fail if `blendFrameScaled` or CPU presentation rendering runs.
- Add Ebiten-specific tests:
  - hardware update validates dimensions, buffer lengths, and rect bounds;
  - `UpdateFrame` preserves legacy behaviour;
  - `SetDisplayConfig` invalidates GPU resources and re-enables hardware after prior runtime failure;
  - offscreen shader composition matches CPU reference pixels for transparency, alpha promotion, z-order, and scaling.
- Add performance coverage:
  - CI-safe benchmarks report `BenchmarkCompositorSoftwareScaled960x540To1080p` versus `BenchmarkCompositorHardwareLayerBuild960x540To1080p`.
  - A normal test hook, not a benchmark threshold, is the CI gate proving the accelerated path avoids presentation-sized CPU iteration.
  - An opt-in real backend benchmark, enabled only with `IE_PERF_GPU_COMPOSITOR=1`, measures one opaque `960x540` RGBA source presented at `1920x1080`, stretch-fill, no overlays, no recorder, warmed up, median over a fixed frame count.
  - The opt-in benchmark reports both paths: native upload + GPU draw versus software scale + full-frame upload. In a controlled local perf run on the same machine, the target is at least 25% lower median frame time for the accelerated path; this target is not a default CI gate.

## Verification

- Run targeted tests first:
  - `go test -tags headless -run 'TestCompositor_.*Hardware|TestCompositor_.*Scale|TestCompositor_.*Alpha|TestCompositor_.*Scanline' .`
  - `go test -run 'TestEbitenOutput_.*Hardware|TestEbitenOutput_UpdateFrame|TestEbitenF11' .`
- Run broad regression:
  - `go test -tags headless ./...`
  - `go build -tags novulkan .`
- Confirm performance:
  - CI gate: test hook proves no CPU presentation-sized scaling on the accelerated happy path;
  - CI-safe benchmarks document relative software-scaling versus hardware-layer-build costs without enforcing a timing threshold;
  - opt-in real Ebiten benchmark documents median frame times and meets the 25% target in a controlled same-machine run;
  - CPU profile for the same workload shows `blendFrameScaled` absent or near-zero on the normal Ebiten path.

## Assumptions

- Hardware compositing is enabled by default only for non-headless Ebiten.
- Headless remains CPU-rendered and is the authoritative pixel reference.
- Existing overlays, cursor drawing, F11 scale toggle, coordinate mapping, and resolution locking remain unchanged.
- The implementation must not rely on normal GPU alpha blending because compositor transparency semantics are not standard alpha blending.
- "Measurably faster" means both: CI-safe tests prove the hot CPU scaling work is eliminated, and the opt-in same-machine backend benchmark reports end-to-end accelerated presentation at least 25% faster under the defined workload.
