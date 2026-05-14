# Release and SDK Layout Audit

This is a working plan for reducing repository and release-layout overload without losing useful artifacts.

## Summary

The current layout mixes four different things:

- source files that developers edit
- generated build outputs
- release/runtime payloads
- local diagnostic artifacts

The main problem is not just that files are in odd places. The larger problem is that release archives currently copy `sdk/` almost wholesale, so anything that lands in `sdk/` can accidentally become part of the shipped product.

The target should be:

- `sdk/` is source-facing and clean enough for developers to inspect.
- generated files are staged into build output directories.
- runtime releases are assembled from an explicit manifest.
- the Live USB FAT32 partition is assembled from an explicit manifest.

## Current Findings

### High-risk SDK Mixing

`sdk/` currently contains all of these categories:

- SDK source and docs: `sdk/include/`, `sdk/docs/`, `sdk/examples/asm/`, `sdk/examples/c/`, `sdk/examples/basic/`
- installed tool binaries: `sdk/bin/`
- assembled examples: `sdk/examples/prebuilt/`
- assembled files beside source: examples under `sdk/examples/asm/*.ie64`, `*.iex`, `*.ie80`
- OS build products: `sdk/intuitionos/iexec/*.elf`, `*.ie64`, `*.lst`
- ROM images: `sdk/roms/aros-ie-m68k.rom`, `sdk/examples/prebuilt/etos256us.img`, `sdk/examples/prebuilt/aros-ie.rom`
- AB3D64 source port plus build outputs: `sdk/ab3d64/src/`, `sdk/ab3d64/bin/`, `sdk/ab3d64/build/`, `sdk/ab3d64/_build/`
- Python cache files tracked under `sdk/ab3d64/src/ie/tools/__pycache__/`
- diagnostic scripts and local logs mixed under `sdk/scripts/` and `sdk/ab3d64/tools/`

### Release Packaging Risk

The runtime release targets copy the whole SDK tree:

```make
cp -r sdk $$STAGING/sdk
rm -rf $$STAGING/sdk/bin
```

This means release contents are defined by the current state of `sdk/`, not by a deliberate release manifest.

### Live USB FAT32 Status

`build_x64_ie_img.sh` currently creates and formats an empty FAT32 `IESHARE` partition. It does not stage SDK files, examples, assets, docs, ROMs, or tools there yet.

That is good news: the FAT32 payload can be designed cleanly instead of reverse-engineering accidental behavior.

### Root Workspace Clutter

There are many untracked local artifacts at repo root, including old plans, images, binaries, test binaries, and image-build outputs. These should be treated as local cleanup, separate from tracked layout cleanup.

Do not mix this with the release-layout refactor.

## Proposed Repository Layout

Keep the source tree and shipped layouts separate.

```text
/
  assembler/              Go sources for IE32/IE64 assembler/disassembler package/tools
  cmd/                    Go command entrypoints
  internal/               internal Go packages
  assets/                 host integration/runtime assets used by the VM
  examples/               editable example source, not installed/generated output
    asm/
    basic/
    c/
    assets/
  sdk/                    public SDK source surface
    include/
    docs/
    scripts/
    players/
    cputest/
  os/
    intuitionos/          IntuitionOS source, boot/runtime source, system template
    emutos/               EmuTOS integration source/patches
    aros/                 AROS integration notes/patches/staging hooks
  ports/
    ab3d64/               AB3D64 source port and its tools
  dist/
    manifests/            explicit file lists for shipped payloads
    staging/              generated, ignored staging roots
  build/                  generated build outputs, ignored
  release/                release archives, ignored
```

This does not need to happen in one commit. The important first step is to stop treating `sdk/` as both source and output.

## Proposed Shipped Layouts

### Runtime Release Archive

Suggested top-level release archive:

```text
IntuitionEngine-<version>-<os>-<arch>/
  IntuitionEngine[.exe]
  README.md
  CHANGELOG.md
  DEVELOPERS.md
  sdk/
    bin/
    include/
    docs/
    scripts/
    examples/
      asm/
      basic/
      c/
      assets/
      prebuilt/
  os/
    intuitionos/
      system/
    aros/
    emutos/
```

Rules:

- `sdk/bin/` contains tools built for that host OS/arch only.
- `sdk/examples/prebuilt/` contains curated prebuilt demos only.
- generated listings, temporary build dirs, screenshots, pycache, and diagnostics do not ship.
- AROS/IntuitionOS runtime system payloads ship under a runtime/OS area, not as editable SDK source.

### Standalone SDK Archive

Suggested standalone SDK:

```text
IntuitionEngine-SDK-<version>/
  README.md
  include/
  docs/
  scripts/
  examples/
    asm/
    basic/
    c/
    assets/
    prebuilt/
  players/
  cputest/
  bin/
```

Rules:

- include examples source by default
- include only curated prebuilt examples
- include tools for the package target where applicable
- exclude OS desktop payloads unless explicitly documented as SDK fixtures

### Live USB FAT32 `IESHARE`

Recommended FAT32 payload:

```text
IESHARE/
  README.TXT
  DEMOS/
    IE32/
    IE64/
    M68K/
    Z80/
    6502/
    X86/
  DOCS/
    GETTING_STARTED.TXT
    SDK/
  SDK/
    INCLUDE/
    EXAMPLES/
    SCRIPTS/
  MUSIC/
  ROMS/
```

Keep FAT32 user-facing and small. Do not put toolchain binaries there unless the live environment is intended to compile on-device.

Recommended first FAT32 content:

- short `README.TXT` with how to run demos and where persistent save data lives
- curated runnable demos grouped by CPU
- music/demo assets needed by those demos
- SDK include/examples as reference material
- EmuTOS/AROS ROMs only if their redistribution status is settled

Avoid on FAT32:

- Go build outputs
- `sdk/bin/` host tools unless there is a real live-build workflow
- `sdk/ab3d64/build/`, `sdk/ab3d64/_build/`, listings, screenshots
- pycache and local diagnostic files
- full AROS workbench tree unless the user needs host-visible inspection

## Artifact Classification

### Keep as Editable Source

- `assembler/`
- `cmd/`
- root Go runtime sources
- `sdk/include/`
- `sdk/docs/`
- `sdk/examples/asm/*.asm`
- `sdk/examples/basic/*.bas`
- `sdk/examples/c/*.c`
- `sdk/examples/assets/` source assets used by examples
- `sdk/players/*.asm`, `sdk/players/*.a80`
- `sdk/cputest/*.asm`, `sdk/cputest/include/`, `sdk/cputest/generated/` if the generated cases are intentionally versioned fixtures
- `sdk/emutos/*.c`, `sdk/emutos/*.h`
- `sdk/ab3d64/src/` if AB3D64 remains in-tree

### Generated and Should Move Out of Source Paths

- `sdk/bin/`
- `sdk/examples/prebuilt/`
- `sdk/examples/asm/*.ie64`
- `sdk/examples/asm/*.iex`
- `sdk/examples/asm/*.ie80`
- `sdk/intuitionos/iexec/*.elf`
- `sdk/intuitionos/iexec/*.ie64`
- `sdk/intuitionos/iexec/*.lst`
- `sdk/ab3d64/bin/`
- `sdk/ab3d64/build/`
- `sdk/ab3d64/_build/`
- `sdk/ab3d64/tools/generated/`
- `sdk/ab3d64/tools/shots/`
- `sdk/players/*.bin`
- `sdk/cputest/cputest_suite.bin`

Some of these may still be shipped, but they should be staged from `build/` or `dist/staging/`, not committed beside source.

### Should Be Removed From Git

- `sdk/ab3d64/src/ie/tools/__pycache__/*.pyc`

### Needs License/Redistribution Decision

- `TopazPlus_a1200_v1.0.raw`
- `sdk/include/topaz.raw`
- `etos256us.img`
- `sdk/examples/prebuilt/etos256us.img`
- `sdk/examples/prebuilt/aros-ie.rom`
- `sdk/roms/aros-ie-m68k.rom`
- third-party music/demo assets in `sdk/examples/assets/music/`
- AB3D64/AB3D2-derived assets and binaries

These may be fine, but they should have an explicit `LICENSES/` or `THIRD_PARTY.md` entry before being put in release archives or FAT32 images.

## Migration Plan

### Phase 1: Stop New Mess

- Add ignore rules for Python caches and local generated SDK outputs.
- Remove tracked `__pycache__/*.pyc`.
- Change example build rules so outputs go to `build/sdk/examples/prebuilt/` or `dist/staging/sdk/examples/prebuilt/`.
- Add a release manifest file for runtime archives.
- Add a separate FAT32 manifest file for Live USB.

### Phase 2: Make Releases Explicit

- Replace `cp -r sdk $$STAGING/sdk` with a staging script.
- Stage only approved SDK subtrees.
- Stage host-specific `sdk/bin/` tools from freshly built binaries.
- Stage curated prebuilt examples from generated output.
- Update `scripts/test-dist-layout.sh` to reject pycache, build dirs, listings, and accidental binaries.

### Phase 3: Move Big Domains

- Move editable examples out of `sdk/examples/` to `examples/`, or keep them in `sdk/examples/` but enforce that the directory is source-only.
- Move IntuitionOS source/build area from `sdk/intuitionos/` to `os/intuitionos/`.
- Move AB3D64 from `sdk/ab3d64/` to `ports/ab3d64/`.
- Keep compatibility wrappers or update docs/scripts in the same commit.

### Phase 4: FAT32 Payload

- Add a `make x64-live-share-payload` target that stages `dist/staging/x64-live/IESHARE/`.
- Teach `build_x64_ie_img.sh` to populate the FAT32 image from that staging directory.
- Add a small FAT32 layout test that verifies max size, required files, and forbidden files.

## Immediate Low-risk Changes

These are safe first commits:

1. Remove tracked `sdk/ab3d64/src/ie/tools/__pycache__/*.pyc`.
2. Add `.gitignore` entries for `__pycache__/`, `*.pyc`, SDK build dirs, and AB3D64 generated outputs.
3. Add `dist/manifests/runtime.txt`, `dist/manifests/sdk.txt`, and `dist/manifests/x64-live-fat32.txt`.
4. Add a staging script that copies only manifest-approved files.
5. Update release targets to call the staging script instead of copying all of `sdk/`.

## Recommended Decision

Do not start by moving dozens of files.

Start by making release staging explicit. Once releases and FAT32 payloads are manifest-driven, the repo can be reorganized in smaller, safer commits without worrying that moving a file silently changes what users receive.
