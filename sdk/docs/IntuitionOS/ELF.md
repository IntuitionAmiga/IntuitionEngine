# IntuitionOS Native ELF Contract (M14.2 Phase 1)

## Status

This document freezes the executable-format contract through M14.2 phase 1 and records the historical transition from M14 to the current ELF-only execution contract.

What M14 phase 1 did:

- chooses `ELF` as the native executable format for **DOS-loaded commands and applications**
- defines the exact accepted subset
- adds test fixtures and validator coverage for the subset

What M14 phase 2 adds:

- `DOS_LOADSEG`
- `DOS_UNLOADSEG`
- DOS-owned seglist objects built from the frozen ELF subset

What M14 phase 3 adds:

- `DOS_RUNSEG`
- dual-mode `ExecProgram` launch descriptors
- descriptor-launched children mapped at the ELF target virtual addresses with the preserved ELF entry point

What M14 phase 4 adds:

- shell / `DOS_RUN` now prefers the native seglist path for strict M14 ELF commands
- legacy flat-image command files still work through `DOS_RUN` via fallback

What M14 phase 5 adds:

- the shipped seeded `C:` command/demo path now uses native ELF by default
- the retained GUI demos (`C:GfxDemo`, `C:About`) are covered as end-to-end regressions on that native path

M14.2 phase 1 current contract:

- `ELF` is now the only supported executable format
- `ExecProgram` is descriptor-only
- `DOS_RUN` rejects flat-image executable content
- `DOS_LOADSEG` remains strict-ELF-only
- boot/service migration remains historical context here; Phase 1 does not reintroduce any flat-image runtime contract

So the current runtime is:

- boot/services: internal embedded-manifest ELF path for shipped runtime binaries
- shell / `DOS_RUN`: native seglist launch for the seeded `C:` ELF command/demo set, with flat-image executable content rejected
- explicit DOS loading: `LoadSeg` / `UnLoadSeg` for strict native `ELF`
- explicit DOS launch: `RunSeg` for strict native `ELF`

Historical M14 shipped state:

- boot services originally remained on the legacy kernel `IE64PROG`/`program_table` path
- `DOS_LOADSEG` remained the public file-backed DOS loader API
- the visible `C:` command/demo path was native ELF

M14.1 target state:

- all shipped runtime binaries become ELF
- boot/services move to an embedded boot manifest plus an internal embedded-manifest service loader
- the future M14.1 embedded-manifest service source path is internal-only and not a public DOS API

M14.1 phase 5 shipped state:

- the kernel prepares staged strict-M14 ELF rows for the internal embedded boot manifest
- `console.handler` and `dos.library` boot from that staged manifest source
- once DOS is online, `dos.library` launches `Shell`, then resolves the `S:Startup-Sequence` service-name lines through the same internal embedded-manifest path to launch `hardware.resource`, `input.device`, `graphics.library`, and `intuition.library`
- shipped service files under `LIBS:`, `DEVS:`, and `RESOURCES:` are now emitted as strict M14 ELF too
- that service boot path is still internal-only; the public DOS surface remains the file-backed `DOS_LOADSEG` / `DOS_UNLOADSEG` / `DOS_RUNSEG` API
- the visible runtime is locked by explicit M14.1 phase-5 regressions for boot census, shell command dispatch, unknown-command handling, and the retained GUI demos

## Design Goal

Use a modern binary file format without adopting a POSIX-shaped process model.

IntuitionOS keeps the Amiga-facing model:

- `LoadSeg`
- `UnLoadSeg`
- DOS command lookup in `C:`
- libraries/devices/resources as named services

`ELF` is the file format, not the user-facing programming model.

## Scope

This contract applies to:

- DOS-loaded commands
- DOS-loaded applications

This contract historically did **not** apply to shipped boot services in base M14. In the current M14.2 runtime it does apply to the shipped binaries too, but boot/services reach it through the internal embedded-manifest source path rather than a new public DOS API.

This contract does **not** apply to:

- classic Amiga `HUNK`
- any compatibility format

## Frozen M14 ELF Subset

Accepted files must satisfy all of the following:

### File identity

- magic = `0x7F 'E' 'L' 'F'`
- class = `ELF64`
- data encoding = little-endian
- ELF ident version = `1`
- OSABI = `0` (`System V`)
- ELF file type = `ET_EXEC`
- machine = `EM_IE64 = 0x4945`
- ELF header version = `1`

`EM_IE64` is the IntuitionOS native IE64 machine tag frozen by this milestone.

### Program-header subset

- at least one program header
- program header entry size = 56 bytes
- only `PT_LOAD` is accepted
- `PT_DYNAMIC`, `PT_INTERP`, `PT_TLS`, `PT_NOTE`, and any other non-`PT_LOAD` header are rejected in M14

### Segment rules

For every `PT_LOAD` segment:

- `p_align = 0x1000`
- `p_vaddr` is page-aligned
- `p_filesz <= p_memsz`
- `p_offset + p_filesz` stays inside the file
- segment must lie inside IntuitionOS user image space:
  - `0x00600000 <= p_vaddr`
  - `p_vaddr + p_memsz <= 0x02000000`
- segment flags may use only `R`, `W`, `X`
- every segment must be readable
- `W|X` together is rejected
- `PT_LOAD` memory ranges must not overlap

### Entry-point rules

- `e_entry` must be nonzero
- `e_entry` must lie inside an executable `PT_LOAD` segment

## Explicitly Unsupported In M14 Phase 5

- `ET_DYN`
- shared objects
- dynamic linking
- interpreter-based launch
- symbol resolution at runtime
- relocations that require a runtime linker
- writable+executable load segments
- classic Amiga `HUNK`

## M15.4 Hardening Note

`ET_DYN` remains unsupported in M15.4.

M15.4 keeps deterministic `ET_EXEC` placement. The runtime loader contract is
still the strict M14.2 `ET_EXEC` subset; runtime relocation, randomized
placement, ASLR, and KASLR remain future work.

## M15.5 PIE-capable codegen contract

M15.5 turns the forward-compatible codegen guidance into an explicit contract.

- strict `ET_EXEC` runtime contract remains in force
- `ET_DYN`, runtime relocation, ASLR, and KASLR remain future work
- IOS-native user code should avoid baking task-local absolute addresses into shipped user code when a startup-block, descriptor, imported pointer, or PC-relative pattern can carry the dependency instead
- writable+executable segments remain invalid; the future PIE story is about codegen discipline first, not a widened loader today

The canonical source-level rules for hand-written IE64 assembly and future
compiler backends now live in `sdk/docs/IntuitionOS/Toolchain.md`.

## Relation To M13 Startup ABI

The M13 startup-page ABI remains the task-entry contract.

M14 Phase 1 does **not** change that. Later M14 phases must still seed:

- startup page base VA at `0(sp)`
- the 64-byte startup block inside that page

for any child launched from an ELF image.

## Relation To LoadSeg / UnLoadSeg / RunSeg / DOS_RUN

M14 phases 2-5 ship, and M14.2 phase 1 keeps only the ELF side of that public DOS surface:

- `DOS_LOADSEG`: resolve a DOS path/name, validate the strict ELF subset, and build a DOS-owned seglist
- `DOS_UNLOADSEG`: free that DOS-owned seglist
- `DOS_RUNSEG`: build a nucleus-facing launch descriptor from that seglist and launch the child through the dual-mode `ExecProgram` bridge
- `DOS_RUN`: use the same native seglist path for strict ELF commands and reject flat-image executable content

Seglist lifetime rules:

- `LoadSeg` returns a DOS-owned seglist object
- each seglist entry records:
  - DOS-owned segment memory VA
  - final target VA from the ELF file
  - file size
  - memory size
  - page count
  - `R/W/X` flags
- the seglist header also records the preserved ELF `e_entry`
- `UnLoadSeg` frees the DOS-owned segment memory and the seglist header
- `RunSeg` copies the seglist into private child mappings; it does not consume the seglist
- successful `UnLoadSeg` after launch does not affect the running child
- failed launch leaves the seglist intact

## Relation To ExecProgram

M14 phases 3-5 introduced the launch-descriptor contract for `ExecProgram`. M14.2 phase 1 completes that compatibility break for the public runtime surface: `ExecProgram` now accepts only the launch descriptor contract.

## Shipped Phase-5 Boundary

What is native today:

- seeded `C:` commands and demo applications that now ship as strict M14 ELF
- explicit `LoadSeg` / `RunSeg` use on strict M14 ELF files in the DOS file store

What is removed in M14.2 phase 1:

- the flat-image `ExecProgram(image_ptr, image_size, args_ptr, args_len)` ABI is no longer supported
- `DOS_RUN` no longer keeps a flat-image fallback for non-ELF inputs

M14.1 changes that boundary by moving shipped services to an embedded boot manifest and an internal embedded-manifest source path that feeds the same validator/seglist/descriptor machinery. That M14.1 source path is internal-only; it does not change the public DOS file-backed API described in this document.

The surviving `ExecProgram` contract is the M14 launch-descriptor path used by DOS `RunSeg`. The descriptor preserves each segment's target virtual address, `R/W/X` flags, and the original ELF `e_entry`, so launched children execute as linked rather than as a reflatted `IE64PROG`.

## Why No HUNK

IntuitionOS is not targeting classic Amiga binary compatibility in this milestone.

That means:

- there is no reason to make `HUNK` the native format
- `ELF` gives a better long-term toolchain story
- the Amiga feel comes from DOS/`LoadSeg`/libraries/devices/resources, not from inheriting old binary containers
