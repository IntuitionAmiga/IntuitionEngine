# AB3D64

IE64 source-port of Alien Breed 3D II **Redux High Overdrive**, built
directly from the upstream M68K sources via `m68kto64` and assembled
with `ie64asm`.

Single variant: `ab3d2_ie64_redux_high_overdrive.ie64`. Default
(non-overdrive), `redux-low`, and SID variants are intentionally
out of scope for v1 — see [§5 Known gaps](#5-known-gaps).

## Build

From the repo root:

```bash
make m68kto64 ie64asm     # build the transpile/assemble tools
make ab3d64               # build the .ie64 (delegates to this dir)
```

Or directly:

```bash
make -C sdk/ab3d64 all
```

Prerequisites: Go toolchain (for `m68kto64` / `ie64asm`), Python 3
(for `tools/run_media_prep.sh`).

## Run

```bash
./bin/IntuitionEngine -ie64 sdk/ab3d64/bin/ab3d2_ie64_redux_high_overdrive.ie64
```

`-ie64` is a boolean flag that selects IE64 CPU mode; the `.ie64`
path is parsed as a positional argument and loaded via
`LoadProgram(filename)`. The program loads at `PROG_START = 0x1000`,
matching upstream `vlink -Ttext 0x1000`.

**ARM64 perf caveat.** Per the IE64 JIT maturity status, the ARM64
JIT is functional but immature relative to amd64. Apple Silicon and
Linux ARM64 hosts run but expect a perf delta against amd64.

## Layout

```
sdk/ab3d64/
├── Makefile                   # this dir's build entry point
├── README.md                  # operator-facing (this file)
├── src/                       # frozen snapshot of ab3d2_source/, post Amiga-strip
├── tools/
│   ├── amiga_strip.py         # Phase 0.5 dead-branch strip
│   ├── run_media_prep.sh      # emits media_profile.i, optional asset staging
│   ├── check_upstream_drift.sh# informational diff vs. fresh upstream
│   ├── prepare_media_profile.py  # vendored from ab3d2_source/ie/tools/
│   ├── unpack_sb_assets.py    # vendored
│   └── convert_menu_assets.py # vendored
├── bin/                       # build output (.ie64 binaries)
└── build/                     # intermediate per-profile staging
```

The companion engineer-facing internals doc is
[`sdk/docs/AB3D64.md`](../docs/AB3D64.md).

## 5. Known gaps

- **Amiga surface** is stripped at Phase 0.5 (`tools/amiga_strip.py`).
  Any future upstream re-sync MUST re-run the strip; see
  [Updating the snapshot](#updating-the-snapshot).
- **Self-modifying code** is unsupported by `m68kto64` (per
  `sdk/docs/m68Kto64.md` §12). The AB3D2 snapshot does not appear to
  contain genuine SMC (only data-table byte patches), but a re-sync
  could introduce some.
- **`diag_symbols.lua` extraction** (upstream `awk` symbol pipeline)
  is not yet ported; the `.ie64` builds without it.
- **Non-overdrive / redux-low / SID variants** are not built.
  The recipe to restore them lives in
  [`sdk/docs/AB3D64.md`](../docs/AB3D64.md) §7. No source changes
  required — the upstream gates are intact.
- **Residual transpiler gaps.** First-pass build surfaces a handful
  of `; ERROR:` lines for vasm-only directives (`output`, `opt`),
  the `rs.b/.w/.l` / `rsreset` offset counter, single-register
  `movem`, indented dot-local labels, and `LABEL:mnemonic` inline
  syntax. Each is tracked in [`sdk/docs/AB3D64.md`](../docs/AB3D64.md)
  §6 with category (transpiler fix vs. source rewrite) and status.
- **FP5/FP6/FP7 corruption** is auto-handled by `m68kto64`'s scratch-
  slot spill. If RTE-driven IRQ handlers surface FP-clobber issues,
  turn on the transpiler's `-fp-irq-wrap` via
  `M68KTO64_EXTRA_FLAGS`.

## Updating the snapshot

```bash
rsync -a --exclude=_build --exclude=obsoleted --exclude='ie/bin' \
  --exclude='bin' --exclude='*.o' \
  /path/to/alienbreed3d2/ab3d2_source/ sdk/ab3d64/src/
python3 sdk/ab3d64/tools/amiga_strip.py sdk/ab3d64/src
```

Commit the two steps separately (rsync, then strip) so reviewers can
see what upstream changed versus what the strip neutralised. Use
`make -C sdk/ab3d64 drift` to preview a re-sync diff without
overwriting `src/`.

## Internals / why it looks this way

See [`sdk/docs/AB3D64.md`](../docs/AB3D64.md) for: port lineage,
build-pipeline diagram, final strip-list, M68K→IE64 mapping notes
specific to AB3D2, BSS strategy, the enumerated category-3 source
rewrites, and the recipe for porting another AB3D2-style M68K
codebase.
