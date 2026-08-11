# ULA Boing IE64

This demo uses the Host SDK C23 compiler and IE64 FPU to map and animate a
red-and-white chequered sphere in real time. The only video device is the ZX
Spectrum ULA. Two SoundChip voices create each impact, followed by restrained
global reverb. `shadowofthebeast.mid` loops underneath at `64/255` MIDI volume,
leaving the synthesised Boing impacts in the foreground.

Frames are composed completely in ordinary RAM and committed to the ULA's IE64
VRAM aperture with aligned 64-bit stores. The paged `ULA_DATA` interface was
measured at roughly 17 frames per second for a full 6,912-byte upload; the wide
aperture preserves authentic ULA scanout while sustaining the intended 50 Hz
cadence.

The release recipe uses the compiler's highest supported QBE optimisation level,
`-O3`. The Host SDK does not currently accept `-ffast-math`; safe equivalent
transformations are explicit in the renderer, including hoisted reciprocal
multiplication for the shadow ellipse's invariant divisors.

The soundtrack is embedded directly in the `.ie64` image by
`ula_boing_music_data.s`. The guest starts it through the MIDI MMIO block, so
the demo has no runtime asset or file-root dependency. The assembly helper is
local to the demo because QBE's IE64 backend does not currently select the
equivalent C pointer-to-`uint32_t` conversion.

Build and run it from the repository root:

```sh
make ula-boing-ie64
./bin/IntuitionEngine sdk/examples/prebuilt/ula_boing_ie64.ie64
```

Inspect the compiler-generated IE64 assembly with:

```sh
sdk/bin/ie64-cproc -O3 -S -o /tmp/ula_boing_ie64.s sdk/examples/c/ula_boing_ie64.c
```

The development harness has `static`, `motion`, `performance`, `capture`, and
`audio` modes. For example:

```sh
IE_ULA_BOING_MODE=static ./bin/IntuitionEngine \
  -script sdk/scripts/diag_ula_boing_ie64.ies
```

If the image is already supplied on the command line, set
`IE_ULA_BOING_PRELOADED=true`. This is useful with a headless runner whose
current CPU lifecycle cancels a script while replacing the default BASIC
guest:

```sh
IE_ULA_BOING_MODE=performance IE_ULA_BOING_PRELOADED=true \
  ./bin/IntuitionEngine -script sdk/scripts/diag_ula_boing_ie64.ies \
  sdk/examples/prebuilt/ula_boing_ie64.ie64
```

Set `IE_ULA_BOING_AUDIO_PRESET` to `soft_rubber`, `classic_boing`,
`heavy_ball`, or `reject_reverb` with `IE_ULA_BOING_MODE=audio` to audition
repeated impacts. The rejection preset is intentionally excessive.

The linker map is the debugger ABI. In IEMon, load symbols from
`sdk/examples/prebuilt/ula_boing_ie64.map`, then place breakpoints at
`advance_physics`, `resolve_collision`, `render_ball`, `classify_ula_cell`,
`commit_ula_frame`, and `trigger_boing`. Watch `boing_diagnostics`, the ULA
register block at `$F2000`, flexible audio channels at `$F0A80` and `$F0AC0`,
and reverb registers `$F0A50` and `$F0A54`.
