# IntuitionOS Native ELF Contract (M16.4.1)

## Memory Model (PLAN_MAX_RAM.md)

IE64 ELF images run against the autodetected guest RAM. Total guest RAM is selected at boot from host `/proc/meminfo` minus a per-platform reserve; IE64 sees the full active visible RAM and reports it through `CR_RAM_SIZE_BYTES` and the `SYSINFO_ACTIVE_RAM_LO/HI` MMIO pair (total guest RAM is `SYSINFO_TOTAL_RAM_LO/HI`). Loaders must size the runtime image and userland ASLR placement against active visible RAM, not against the retired fixed 32 MB IExec model. PLAN_MAX_RAM.md slice 4 retired the flat `MMU_NUM_PAGES = 8192` page-count constant; M16.4 ELF segment bounds checks consult the per-task page table installed via `kern_pt_install_leaf` and the multi-level walker rather than a fixed pages-per-task ceiling.

## Status

This document records the historical transition from M14 to the current
ELF-only execution contract. As of M16.4.1, every accepted DOS-loadable or
protected runtime ELF is a self-contained `ET_DYN` IE64 image with
zero-relative `PT_LOAD` addresses, IOS runtime metadata in one `PT_NOTE`,
optional local bounded `IOS-REL` relocation metadata, no section header table,
and userland ASLR placement. These stripped, section-header-free runtime ELFs are the accepted form:
`e_shoff == 0`, `e_shnum == 0`, and `e_shstrndx == 0`.

M16.4 changes the live runtime executable contract from fixed `ET_EXEC` to
self-contained `ET_DYN`. This applies to shipped commands, libraries, devices,
handlers, resources, host-provided DOS ELFs, and third-party DOS-loadable
runtime ELFs after the cutover. There is no implicit `ET_EXEC` compatibility
exception.

The IntuitionOS model still has no dynamic linking. There is no `PT_INTERP`,
`PT_DYNAMIC`, `DT_NEEDED`, PLT/GOT binding, imported-symbol lookup, lazy
binding, or shared-object namespace. Relocation is local and self-contained:
the M16.4.1 ABI accepts bounded `IOS-REL` note records using
`R_IE64_RELATIVE64`, where symbol index must be zero and the loader writes
`chosen_image_base + r_addend` to a writable non-executable target.
`section-header-only` metadata is rejected after the M16.4.1 cutover.

`.library` and class-specific module acquisition remain the service boundary:
`OpenLibrary`, `CloseLibrary`, handler/device/resource acquisition IPC, ports,
and message ABI remain the programmer-facing model. Trusted-internal module
launch authority comes from trusted read-only system source provenance
(`IOSSYS:L`, `IOSSYS:LIBS`, `IOSSYS:DEVS`, `IOSSYS:RESOURCES`) plus validated
IOSM name/class metadata, not from an arbitrary filename in a writable overlay.
Userland ASLR is enabled; M16.4.1 only readies the code for KASLR and does not
implement kernel randomization. The M16.5 blocker inventory is fixed kernel
page-table/data/stack placement, supervisor identity mapping, trap/fault paths,
scheduler state access, panic/debug paths, and task page-table copying of
kernel mappings. W^X means no page may be writable and executable at the same
time. SKEF/SKAC/SUA discipline, explicit shared-memory `MAPF_*` masks, bounded
inputs, and the rule against raw cross-task pointers remain mandatory.

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

- the shipped `C:` command/demo path now uses native ELF by default
- the retained GUI demos (`C:GfxDemo`, `C:About`) are covered as end-to-end regressions on that native path

M14.2 phase 1 current contract:

- `ELF` is now the only supported executable format
- `ExecProgram` is descriptor-only
- `DOS_RUN` rejects flat-image executable content
- `DOS_LOADSEG` remains strict-ELF-only
- boot/service migration remains historical context here; Phase 1 does not reintroduce any flat-image runtime contract

So the current runtime is:

- boot/services: internal embedded-manifest ELF path for shipped runtime binaries
- shell / `DOS_RUN`: native seglist launch for the `C:` ELF command/demo set, with flat-image executable content rejected
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
- once DOS is online, `dos.library` launches `Shell`; the current M16 phase-5 startup script launches `hardware.resource` and `input.device`, while `graphics.library` and `intuition.library` are demand-loaded later through `OpenLibrary`
- shipped service files under `LIBS:`, `DEVS:`, and `RESOURCES:` are now emitted as strict M14 ELF too
- that service boot path is still internal-only; the public DOS surface remains the file-backed `DOS_LOADSEG` / `DOS_UNLOADSEG` / `DOS_RUNSEG` API
- the visible runtime is locked by explicit M14.1 phase-5 regressions for boot census, shell command dispatch, unknown-command handling, and the retained GUI demos

## M16.1 Module Manifest (.ios.manifest, IOSM)

M16.1 supersedes the M16 library-only `LIBM` note with a universal
128-byte `IOSM` descriptor. Every rebuilt runtime ELF carries one
`.ios.manifest` SHT_NOTE named `IOS-MOD` with note type `0x494F5331`.

Descriptor layout:

| Offset | Size | Field |
|---:|---:|---|
| 0 | u32 | magic `0x4D534F49` (`IOSM`) |
| 4 | u32 | schema version, currently `1` |
| 8 | u8 | kind: library=1, device=2, handler=3, resource=4, command=5 |
| 9 | u8 | reserved, must be zero |
| 10 | u16 | version |
| 12 | u16 | revision |
| 14 | u16 | patch |
| 16 | 32 | NUL-padded public name |
| 48 | u32 | `MODF_*` flags |
| 52 | u32 | message ABI version |
| 56 | 16 | build date, `YYYY-MM-DD` |
| 72 | 48 | UTF-8 copyright string |
| 120 | 8 | reserved, must be zero |

Rendering uses `name version.revision.patch` when patch is non-zero,
otherwise `name version.revision`. The manifest is file-format universal;
registry semantics remain library-specific unless a later milestone widens
the protected module lifecycle.

## M16 Protected Module Notes

M16 does not change the public M14.2 `ET_EXEC` contract.

- runtime commands/apps still use the strict M14.2 `ET_EXEC` subset
- `LoadSeg` / `UnLoadSeg` / `RunSeg` semantics are unchanged
- M16 adds protected-module lifecycle around trusted library tasks; it does not widen the public executable ABI to `ET_DYN`, runtime relocation, or shared-library mapping

What M16 adds on top of the existing loader boundary:

- a module manifest note section: `.ios.manifest` / `IOSM`
- exec/dos use that manifest to validate module identity, class, version, flags, and message ABI before a library becomes discoverable through `OpenLibrary`
- the internal module registry and `module_load_handle` lifecycle are separate from the public DOS seglist contract
- M16.3 makes `MODF_ASLR_CAPABLE` mandatory for all DOS-loaded ELFs. Commands must carry exactly `MODF_ASLR_CAPABLE`; libraries, devices, handlers, and resources must carry exactly `MODF_COMPAT_PORT | MODF_ASLR_CAPABLE`.
- M16.3 keeps the strict `ET_EXEC` placement contract. M16.4, not M16.3, owns relocation and ASLR, including `ET_DYN`, randomized placement, ASLR, and KASLR.
- W^X, SKEF/SKAC/SUA discipline, bounded IOSM/path inputs, and shared-memory `MAPF_READ` / `MAPF_WRITE` rules remain mandatory.

M16.2 extends the internal protected-module lifecycle to handlers, devices,
and resources using the existing `IOSM.kind` values and class-specific path
policy (`L:`, `DEVS:`, and `RESOURCES:`). `console.handler`,
`hardware.resource`, and `input.device` now register as protected module rows;
`dos.library` runs the eager post-DOS policy for the resource and device before
Shell, and `S:Startup-Sequence` no longer launches module files as commands.
M16.2.1 freezes public non-library acquisition without changing the executable
format contract: `AttachHandler`, `OpenDevice`, and `OpenResource` are SDK
wrappers over the kernel-serviced public `exec.library` port, using
`EXEC_MSG_*` request/reply IPC and opaque generation-aware tokens. It is
ONLINE-only for non-library rows and does not demand-load absent handlers,
devices, or resources. It also does not introduce variable-base placement,
relocation processing, `ET_DYN`, PIE enforcement, ASLR, or third-party install
policy. The intended split is M16.2 for internal non-library module semantics,
M16.2.1 for public acquisition APIs, M16.3 for consistently PIE-capable shipped
ELFs, and M16.4 for real relocation plus randomized placement.

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
- every segment must request at least one permission bit
- `W`-only segments are currently rejected; the runtime still widens data pages to readable+writable, so write-only ELF data is not a stable contract yet
- `W|X` together is rejected
- `PT_LOAD` memory ranges must not overlap

### Entry-point rules

- `e_entry` must be nonzero
- `e_entry` must lie inside an executable `PT_LOAD` segment

## M15.6 R4 Execute-Only Code Pages

M15.6 R4 makes execute-only user text a real runtime contract.

- executable `PT_LOAD` segments no longer need `R`; `X` alone is valid
- the loader preserves the ELF segment flags when building child PTEs
- `validate_user_exec_range` now requires `P|X|U`, not `P|R|X|U`
- the end-to-end regression is `TestIExec_M156_R4_ShellRunsExecuteOnlyELFAndReadFaults`: the child executes from an `X`-only page and a later user-mode load from that page faults `read-denied`

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

M14 Phase 1 does **not** change that. Later M14 phases must still install:

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

- `C:` commands and demo applications that now ship as strict M14 ELF
- explicit `LoadSeg` / `RunSeg` use on strict M14 ELF files in the DOS file store

What is removed in M14.2 phase 1:

- the flat-image `ExecProgram(image_ptr, image_size, args_ptr, args_len)` ABI is no longer supported
- `DOS_RUN` no longer keeps a flat-image fallback for non-ELF inputs

M14.1 changes that boundary by moving shipped services to an embedded boot manifest and an internal embedded-manifest source path that feeds the same validator/seglist/descriptor machinery. That M14.1 source path is internal-only; it does not change the public DOS file-backed API described in this document.

The surviving `ExecProgram` contract is the M14 launch-descriptor path used by DOS `RunSeg`. The descriptor preserves each segment's target virtual address, `R/W/X` flags, and the original ELF `e_entry`, so launched children execute as linked rather than as a reflatted `IE64PROG`.

## M16.4.3 Universal Userland Residency

Universal Userland Residency does not create shared executable mappings.
`DOS_RESIDENT_ADD` validates an `IOSM_KIND_COMMAND` ELF, requires
`MODF_ASLR_CAPABLE`, and stores private bytes in the dos.library resident
command cache. `DOS_RESIDENT_REMOVE` drops that cache row, and
`DOS_RESIDENT_LIST` exports resident command cache rows in the RSIV-compatible
record shape.

## Why No HUNK

IntuitionOS is not targeting classic Amiga binary compatibility in this milestone.

That means:

- there is no reason to make `HUNK` the native format
- `ELF` gives a better long-term toolchain story
- the Amiga feel comes from DOS/`LoadSeg`/libraries/devices/resources, not from inheriting old binary containers
