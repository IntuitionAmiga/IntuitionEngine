# VGA Mode 03h Z80 SAP Demo Implementation Plan

## Objective

Replace the restored `sdk/examples/asm/vga_text_sap_demo.asm` presentation with a polished Z80 text-mode production while preserving its identity and runtime contract.

The finished demo must:

- remain in VGA Mode 03h for its entire run;
- render an animated 80 by 25 plasma using character and attribute bytes;
- exercise the Z80 JIT with a substantial CPU-rendered inner loop;
- use the existing Atari POKEY SAP music in POKEY+ mode;
- embed `../assets/music/Hobbytronic_92_2.sap` with `incbin`;
- require no runtime music file access;
- present an integrated logo, credits and scroller;
- loop cleanly without visible stale cells or abrupt palette changes;
- behave identically under the interpreter and JIT;
- include deterministic tests and native JIT execution evidence.

All source comments and documentation changed by this work must use British English. They must not contain em dashes, inflated promotional language, repetitive commentary or prose which merely narrates individual instructions.

## Fixed constraints

### Video

- Select `VGA_MODE_TEXT` and keep it selected.
- Use the VGA 80 by 25 text buffer at physical address `0xB8000`.
- Access it through the existing Z80 bank window.
- Do not use Mode 12h, Mode 13h, Mode X, VideoChip pixel modes, the blitter or the Copper.
- Keep blink disabled so all four background attribute bits remain available.
- Preserve the existing two-phase vertical blank wait.

### Music

The existing SAP asset and embedded data layout are mandatory:

```asm
    .org 0xE000

sap_data:
    .incbin "../assets/music/Hobbytronic_92_2.sap"
sap_data_end:
```

The implementation must preserve:

- `Hobbytronic_92_2.sap` as the music source;
- assembly-time embedding with `incbin`;
- `SAP_DATA_LEN` derived from `sap_data_end-sap_data`;
- the four-byte SAP pointer and length writes;
- POKEY+ enablement;
- start and loop control value `0x05`.

No replacement, conversion, generated copy or runtime loading path is in scope.

### Memory

- Keep code, runtime state, lookup tables, logo data and scroller data below `0xE000`.
- Keep the SAP payload at its existing fixed origin.
- Retain a safe stack region below the Z80 MMIO window.
- Add an assembly-time or test-time size guard against overlap with the SAP region.
- Avoid self-modifying code because it would create unnecessary JIT invalidations.

## Intended presentation

The demo will be a timed sequence which repeats without changing video mode:

1. Hold the plasma alone while the scroller remains visible.
2. Fade the `INTUITION ENGINE` logo into the moving plasma with an independent ordered dither.
3. Hold both effects together long enough to read the logo and credit text.
4. Fade the plasma out while retaining the logo and scroller.
5. Hold the logo alone while retaining the scroller.
6. Cross-fade the logo out while the plasma returns, then restart with the plasma fully visible.

The standard VGA palette will be replaced with a coherent 16-colour circular ramp. The initial proposal is black, deep blue, indigo, violet, magenta, amber, white, cyan and blue, with intermediate shades chosen to avoid harsh steps. Final values will be selected from captured frames rather than accepted from the first draft.

## Rendering design

### Plasma inputs

Each cell will combine four table-driven components:

```text
horizontal = sine[x_phase + time_1]
vertical   = sine[y_phase + time_2]
diagonal   = sine[x_phase + y_phase + time_3]
cross      = sine[x_phase - y_phase + x_time - y_time]
```

Two results will be derived:

```text
hue        = horizontal + vertical
brightness = horizontal + vertical + diagonal + cross
```

All phase arithmetic will use natural eight-bit wrapping. Multiplication and division must not occur in the per-cell loop.

The horizontal and vertical phases use cosine and sine velocities from a 256-sample table. Each sample lasts two frames, producing a 512-frame orbit through every cardinal and diagonal direction without abrupt velocity changes. Signed velocities are scaled by five quarters before entering 16-bit fixed-point accumulators, and their high bytes drive the visible phase offsets. The two diagonal phases receive the sum and difference of those offsets. All four waves therefore translate coherently from the same position.

### Precomputed data

The source will contain or generate during initialisation:

- one page-aligned 256-byte sine table;
- an 80-byte horizontal phase table;
- a 25-byte vertical phase table;
- a 256-byte signed sine velocity table;
- packed character and attribute mapping tables;
- a compact logo interior, outline and shadow mask;
- scroller character and vertical-phase data;
- a 4 by 4 Bayer threshold table for layer transitions.

Where practical, tables used together will be aligned so their high address byte remains constant during indexed lookup.

### Cell mapping

The mapping stage will select from:

- space;
- light shade;
- medium shade;
- dark shade;
- full block;
- upper and lower half blocks where they improve an overlay edge.

Each plasma result maps to a foreground colour, background colour and density character. Foreground and background colours should normally be neighbours in the active palette ramp. Density characters then provide an ordered transition between those colours.

The density calculation retains the horizontal and vertical hue sum as well as both diagonal waves. This makes changes to either spatial accumulator visible in the glyph field rather than only in its colour attributes.

The inner loop will stream through all 2,000 cells and write the character byte followed by the attribute byte. Row-invariant terms will be calculated once per row.

### Logo

The existing nine-row CP437 logo will be replaced with a compact mask designed for the 80 by 25 composition.

The logo must remain part of the plasma rather than cover it with static text:

- interior cells use a brighter or phase-shifted mapping;
- outline cells use a controlled highlight ramp;
- shadow cells use a darker neighbouring ramp;
- unmasked cells use the normal plasma mapping.

The overlay may be a second pass if that produces a smaller and faster plasma loop. The choice will be based on measured native execution and frame stability.

The logo uses the Bayer threshold independently from the plasma. Its cells are progressively overlaid during entry and progressively returned to the underlying composition during exit. This preserves the complete plasma palette.

### Scroller and credits

- Reserve at most two rows for the main scroller.
- Use shade or half-block characters to suggest vertical sine movement.
- Keep credit text short and readable.
- Draw the scroller on every frame, including plasma-only, logo-only and transition intervals.
- Do not clear entire rows more often than required.
- Derive scroll position from frame state so the sequence repeats deterministically.

### Palette animation

- Programme the first 16 VGA DAC entries during initialisation.
- Keep the complete palette active throughout the sequence so the persistent scroller remains readable.
- Apply plasma and logo transitions through independent ordered cell dithers rather than shared palette changes.
- Rotate only the plasma's foreground and background attribute indices through a private 16-colour phase every four frames.
- Avoid writing all 48 DAC components when the intended frame changes only a subset.
- Verify that full-frame and scanline VGA rendering agree on bright background attributes before depending on them.

## Z80 and JIT structure

The main loop will follow this order:

```text
advance the scene timeline
advance four plasma phases
render the plasma into the hidden text page at its current dither level
apply the logo at its independent dither level
draw the scroller and credits
wait for the next vertical blank edge
publish the hidden text page through the CRTC start address
repeat
```

The hot path will:

- keep the text-buffer destination in a register pair;
- use sequential stores;
- keep row constants outside the column loop;
- favour table lookup and byte addition over general arithmetic;
- avoid calls inside the per-cell loop where inlining is reasonably small;
- use `DJNZ` or another compact counted-loop form where it remains clear;
- keep instruction fetches in ordinary programme memory;
- avoid writes to emitted code pages;
- preserve bounded exits so vertical blank and device observation remain correct.

Optimisation decisions must be supported by native-entry and frame-progress evidence. A visually correct interpreter run is not evidence that the JIT compiled and entered the renderer.

## Test-driven implementation

### Phase 1: protect the hardware contract

Add focused VGA tests before changing the demo:

- prove that attribute bit 7 selects a bright background when blink is disabled;
- prove that foreground and background indices select the intended DAC entries;
- compare representative cells through the full-frame and scanline renderers;
- expose and fix any disagreement before the demo relies on bright backgrounds.

These tests should fail only for a real missing contract. They must not encode incidental implementation details.

### Phase 2: define the demo source contract

Extend the SDK example inventory tests to require:

- inclusion of `ie80.inc`;
- selection of `VGA_MODE_TEXT`;
- a two-phase vertical blank wait;
- four independent plasma phase sources;
- VGA DAC programming;
- traversal of 80 by 25 cells;
- the exact `Hobbytronic_92_2.sap` `incbin` path;
- derived SAP length;
- POKEY+ and start-plus-loop writes;
- absence of VideoChip, blitter, Copper and graphics-mode setup.

Add a size check which fails if assembled content below the SAP origin overlaps `0xE000`.

### Phase 3: establish the runtime fixture

Add `vga_text_sap_demo.ie80` to a deterministic Z80 runtime fixture. Capture checkpoints after named scene states rather than after arbitrary wall-clock delays.

Each checkpoint will record:

- Z80 architectural state;
- animation and scene variables;
- the complete 4,000-byte VGA text buffer;
- the first 16 VGA DAC entries;
- VGA mode and control state;
- SAP pointer, length and control state;
- machine fault and stop state;
- JIT counters when enabled.

Semantic assertions will require:

- all cells to be initialised;
- several density glyphs to be present;
- useful foreground and background colour diversity;
- consecutive plasma frames to differ;
- the logo region to differ structurally from its surroundings;
- the scroller position to advance;
- plasma and logo dithers to reach their independent endpoints;
- the scroller to remain visible at every named scene checkpoint;
- the sequence to restart without stale cells.

Opaque framebuffer hashes may supplement these checks, but they will not replace them.

### Phase 4: prove interpreter and JIT parity

Run the same checkpoints once with the interpreter and once with the JIT. Compare architectural state, scene state, VGA text memory, palette state, SAP control state and faults.

The JIT run must report:

- backend `native` on a supported native host;
- positive `native_entries`;
- continued frame progress;
- no unexpected invalidation growth;
- no fallback loop which produces the image entirely through the interpreter.

Cross-target compile or execution gates will follow the existing Z80 JIT policy. Native amd64 is the primary local execution proof. ARM64 and WebAssembly coverage will be included where the repository's established test targets make it practical.

## IEScript development harness

Create `sdk/scripts/vga_text_sap_demo_acceptance.ies` early in the work. It will provide a short edit, build, run and inspect cycle without replacing Go tests.

The script will:

1. Load the rebuilt Z80 image.
2. Wait for a bounded number of compositor frames.
3. Read `cpu.jit_stats()` and require a Z80 backend with positive native entries when JIT proof is requested.
4. Use `cpu.freeze()` before raw RAM reads and always balance it with `cpu.resume()`.
5. Read the 4,000-byte text buffer at stable checkpoints.
6. Read the first 16 palette entries with `video.vga_get_palette()`.
7. Capture screenshots of the fade-in, plasma, logo, scroller and fade-out scenes.
8. Fail on a missing scene transition, unchanged text buffer or absent JIT progress.
9. Exit through a bounded watchdog rather than wait indefinitely.

Use `sys.wait_until()` or a guest-published scene marker for synchronisation. Frame counts alone may be used during the first prototype, but the final harness must follow guest progress so slow machines do not capture the wrong scene.

Use `rec.screenshot()` for reproducible compositor output. Use `rec.screenshot_composed()` or `rec.screenshot_screen()` only when separating composition from final presentation is necessary.

The harness will write diagnostic output beneath a script-owned output directory. Generated screenshots and logs will not be added to the commit unless they are explicitly required as maintained fixtures.

## IEMon development workflow

Use IEMon through the IEScript `dbg.*` interface and interactively when a frame or JIT result is wrong.

The planned workflow is:

- load symbols or use stable labels from an assembler map if available;
- set breakpoints at scene transitions and at the plasma renderer entry;
- inspect `PC`, `SP`, phase variables and buffer pointers with `dbg.get_regs()` and `dbg.read_mem()`;
- use a write watchpoint on the first text-buffer cell to confirm frame publication;
- enable the trace ring only around a suspected faulty loop;
- disassemble the current block with `dbg.disasm()`;
- inspect recent control flow with `dbg.tracering_show()`;
- use `dbg.accesslog()` and `dbg.who()` when a cell or phase variable is unexpectedly overwritten;
- inspect VGA and SAP registers through `dbg.io()` after discovering canonical device names with `dbg.io_devices()`;
- use `dbg.mmio_stats()` during profiling runs started with `IE_MMIO_STATS=1` to identify excessive VGA or SAP register traffic;
- take device snapshots before and after a scene transition when register state diverges;
- use whole-machine reverse history only for faults which cannot be reduced with checkpoints and watchpoints.

Raw monitor memory and register inspection will accelerate diagnosis, but no monitor observation will be treated as a substitute for a deterministic regression test.

## Development sequence

1. Add the VGA bright-background parity test.
2. Add the source contract and embedded SAP assertions.
3. Add the demo to the dedicated Z80 build path and produce the first prebuilt fixture.
4. Create the IEScript acceptance harness with scene screenshots and JIT telemetry.
5. Implement palette setup, fade state and guest-published scene markers.
6. Implement the four-wave plasma with a simple mapping table.
7. Establish interpreter and JIT runtime parity for the plasma-only frame.
8. Tune the palette and glyph mapping from captured output.
9. Add the integrated logo mask and its semantic checkpoint.
10. Add the scroller, credits and complete timeline.
11. Use IEMon to reduce any state divergence or missed frame progress.
12. Run native JIT proof and inspect invalidation and bailout counters.
13. Regenerate the checked-in prebuilt binary from the final source.
14. Update documentation, website copy and published assets.
15. Run the complete scoped verification and prose audit.

Each functional stage begins with a failing test or acceptance assertion and ends only when interpreter and JIT results agree.

## Build and publication integration

Update the following as required:

- `sdk/examples/asm/vga_text_sap_demo.asm`;
- `sdk/examples/prebuilt/vga_text_sap_demo.ie80`;
- `sdk/scripts/build-z80.sh`;
- `sdk/scripts/vga_text_sap_demo_acceptance.ies`;
- the `showreel-z80` target in `Makefile`;
- the relevant Go source-contract and runtime tests;
- `sdk/docs/demo-matrix.md`;
- `sdk/README.md`;
- `intuitionengine.com/index.html`;
- the website SDK source copy and Z80 binary;
- `intuitionengine.com/assets/MANIFEST` where publication requires it.

The build rules must make the embedded `Hobbytronic_92_2.sap` file an explicit input dependency where the build system supports file dependencies.

Website copy must describe only behaviour visible in the final accepted build. Published binaries and source copies must be byte-checked against their canonical SDK counterparts.

## Verification

Run the narrowest proving checks during development, followed by the complete scoped gate:

```bash
vasmz80_std -Fbin -I sdk/include \
  -o sdk/examples/prebuilt/vga_text_sap_demo.ie80 \
  sdk/examples/asm/vga_text_sap_demo.asm

go test -tags headless -run 'TestVGAText|TestZ80.*VGATextSAP' ./...
go test -tags headless -run 'TestZ80JIT.*VGATextSAP' ./...
make check-docs
git diff --check
```

Also run the IEScript acceptance harness with JIT enabled and disabled. Inspect the final composed screenshots rather than relying only on intermediate text-buffer contents.

Before completion:

- compare the source-built binary with the checked-in prebuilt binary;
- compare canonical SDK source and binary files with published website copies;
- confirm the SAP bytes are embedded unchanged;
- confirm the programme never changes from VGA Mode 03h;
- confirm native JIT entries occur during the plasma renderer;
- confirm no unrelated worktree files are included.

## Prose review

Review every changed comment and documentation paragraph for:

- British spelling;
- no em dashes;
- no American spellings where a normal British form exists;
- no generic claims such as "powerful", "stunning" or "revolutionary";
- no long historical digressions unrelated to implementation;
- no comments which repeat the instruction immediately below them;
- precise statements about hardware, memory and timing.

Use targeted searches across the changed files, then inspect the diff manually. Automated spelling searches are a warning mechanism, not the final authority.

## Completion criteria

The work is complete only when:

- the demo assembles through dedicated and aggregate SDK paths;
- `Hobbytronic_92_2.sap` remains embedded with the required `incbin` path;
- the checked-in binary is regenerated from the final source;
- the presentation stays in VGA Mode 03h;
- the plasma, logo, scroller, fades and loop are visible and coherent;
- IEScript captures every intended scene through guest-progress markers;
- IEMon-assisted diagnosis has been converted into permanent tests where applicable;
- interpreter and JIT checkpoints match;
- native Z80 JIT execution is proven;
- SAP playback remains enabled and looping;
- documentation and published assets match the finished demo;
- all changed prose meets the stated style rules;
- scoped tests, documentation checks and `git diff --check` pass.

No commit is part of this plan unless explicitly requested after review.
