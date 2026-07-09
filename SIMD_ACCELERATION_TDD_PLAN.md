# SIMD Acceleration TDD Plan and Tracking

Status of the SIMD acceleration work (`simd/archsimd`, Go 1.26 `goexperiment.simd`,
amd64 only). Every kernel keeps an always-built scalar leaf as its canonical
reference; the SIMD variant is additive, hidden behind the `goexperiment.simd`
build tag and an amd64 guard, and gated at runtime by `IE_SIMD` (default on,
`IE_SIMD=0` reverts to scalar). Non-amd64 targets and hosts without the AVX2
baseline compile and run scalar only via the stubs.

## Toolchain

- Go 1.26.4, `toolchain go1.26.4` pinned in `go.mod`.
- `make` builds export `GOEXPERIMENT=simd` by default (Makefile, next to `GOAMD64`).
- `go run .` and IDE builds need a one-time `go env -w GOEXPERIMENT=simd` (the
  experiment cannot live in `go.mod`); without it they still build and run
  correctly, scalar only.
- `make test-simd` runs the gating tests three ways: SIMD build with `-race`, the
  `IE_SIMD=0` kill switch, and a `GOEXPERIMENT=none` build check.

## Dispatch

Package-level `...Impl` function variables default to the scalar leaf and are
reassigned to the SIMD variant in `assignSIMDKernels` (simd\_dispatch\_amd64.go),
called once from the `init` in simd\_gate\_amd64.go when SIMD is requested and the
host provides AVX2. Differential tests call the scalar and SIMD leaves directly,
never the dispatch variable.

## Phase table

| Phase | Item | Outcome | Evidence |
|-------|------|---------|----------|
| 0 | Toolchain probe, gate files, Makefile, docs | Landed | `archsimd` API pinned; masked store and float/int conversion present; three build worlds green |
| 1 | SparseBacking span chunking (not SIMD) | Landed | Byte-identical; read 4K ~785x, write 4K ~1200x over the per-byte loop |
| 2 | Compositor blend, opaque copy, lease normalise | Landed | Bit-exact differential; blend ~12-13x, opaque copy ~3-6x, normalise ~10-11x |
| 2 | Scaled blend / opaque copy (`blendFrameScaled`, `copyOpaqueFrameScaled`) | Landed | srcX table hoist + srcY row-copy (opaque) + gather-then-SIMD-blend; bit-exact vs old formula; blend ~8.5x SIMD (~1.5x scalar), copy ~3.7x |
| 3 | Blitter colour expand (scalar fast path) | Landed | Characterisation matrix and fast-vs-generic byte-identical; EmuTOS desktop golden hash unchanged |
| 3 | Blitter colour expand (SIMD row) | Dropped | Stop rule: no gather or bit-to-lane expand in archsimd 1.26; the template unpack bounds the kernel |
| 3 | Blitter fill (`fillUint32LESpanSIMD`) | Landed | Bit-exact; ~6-8x |
| 4 | Voodoo untextured spans (core) | Landed | 2000-triangle bit-exact differential (framebuffer and depth), race-clean, golden-clean; 400px ~2.4x (~4.8x with chunk early-out) |
| 4 | Voodoo expansions (alpha test, stipple, dither, chroma, textured) | Landed | Each widened `voodooSetupSIMDEligible` behind its own bit-exact differential; quantising stages (dither, chroma) and texture run as scalar hybrids in the lane loop; textured ~4.6x on 400px. Only alpha blend stays scalar-routed |
| 5 | CLUT8 expand | Scalar (unrolled) | SIMD regressed ~0.68x (no gather); the 4x-unrolled scalar leaf wins ~13% and is the default |
| 6 | Audio post-effects (scale, clamp spans) | Kernels landed, wiring deferred | Bit-exact incl NaN/Inf/denormals, ~5-6x. Master gain is fused into the per-sample post-FX serial chain (filter/reverb recurrences) with no separable block span; wiring deferred on architectural grounds, not a regression |
| 7 | Docs and close-out | Landed | This document, AGENTS.md, DEVELOPERS.md, README.md, CHANGELOG.md |

## No-go and stop-rule outcomes recorded

- Colour-expand SIMD row dropped (archsimd 1.26 has no gather; scalar fast path is
  the win).
- CLUT8 SIMD dropped (staging round-trip loses to direct scalar stores); unrolled
  scalar kept.
- Textured Voodoo SIMD first regressed 4x as a naive hybrid; a chunk-level
  early-out (skip chunks with no surviving lane) plus per-lane texel-sample skip
  turned it into a ~4.6x win, so it landed rather than being dropped.
- Audio master-gain wiring deferred on architectural grounds (fused into the
  per-sample serial post-FX chain, no separable block span); kernels landed and
  tested.

## Float exactness rules honoured

- Clamps use compare and blend (`Merge`), never VMINPS/VMAXPS, to preserve NaN
  pass-through.
- `uint32(f*255)` packs squash NaN lanes to zero to match Go's scalar conversion.
- Differential inputs include NaN, Inf, and denormals.

## FMA fusion caveat (Voodoo)

The plan assumed gc never fuses scalar `a + b*c` into FMA. That is false under
`GOAMD64=v3` (the make default): gc fuses the outer mul-add of the Voodoo
interpolation `v0 + dx*ddx + dy*ddy` into a single FMA, shifting raw depth by up
to 1 ULP versus separate mul-add. The golden checksums (8-bit colour) tolerate
this; the strict raw-float differential does not. Consequences, all handled:

- The Voodoo SIMD interpolation uses `MulAdd` shaped exactly as the fused scalar
  (`dy.MulAdd(ddy, v0 + dx*ddx)` — outer fused, inner plain add), so it is
  bit-identical to the release scalar reference that the golden pins. This is the
  one place FMA is intentionally used; every other kernel is FMA-free.
- The strict differential targets the release build: it skips under `-race` (the
  race detector inhibits gc FMA fusion, `race_flag_on.go`) and under non-FMA
  builds (`GOAMD64 < v3`, detected at runtime). `make test-simd` runs it once
  without `-race` at `GOAMD64=v3` so it asserts in the shipping config, and keeps
  the `-race` sweep for the concurrent golden-band path.
- The compositor, blitter, CLUT8 and audio kernels do no multiply-accumulate over
  a running value, so they carry no FMA-fusion hazard and stay FMA-free.
