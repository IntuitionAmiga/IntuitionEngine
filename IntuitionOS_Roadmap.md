# IntuitionOS Roadmap

## Goal

Build IntuitionOS as a protected, Amiga-inspired operating system on IE64:

- small `IExec.library` microkernel
- user-space services discovered through ports
- explicit shared-memory handoff
- no POSIX-shaped core design
- eventual default graphical boot similar to AmigaOS when no `S:Startup-Sequence` is present

Native IntuitionOS binaries are **not** targeting classic Amiga binary compatibility.
The native executable format should be **ELF**, but the OS-facing loading model should
remain Amiga-shaped:

- `LoadSeg`
- DOS assigns and command lookup
- libraries, devices, handlers, and resources as named services

`HUNK` is out of scope for the native OS roadmap. A future compatibility subsystem for
68K binaries may exist one day, but it should not distort the native design.

## Current State: M16.4 Runtime ELF / Userland ASLR

Implemented:

- protected `exec.library` nucleus with MMU-enforced user/supervisor split and W^X
- preemptive scheduling, signals, message ports, shared memory, and task creation/exit
- dynamic task model with startup-page ABI and dynamic image/PT placement
- public `u32` task IDs with protected user-space services
- `console.handler`, `dos.library`, and `Shell`
- `hardware.resource` with trusted MMIO grant brokering
- `input.device`, `graphics.library`, and `intuition.library`
- DOS-loaded commands in `C:` (`Version`, `Avail`, `Dir`, `Type`, `Echo`, `Assign`, `List`, `Which`, `Help`, `Resident`, `About`, `GfxDemo`, `ElfSeg`)
- `S:Startup-Sequence` boot, Intuition windows, retained GUI demos, and generated host-backed system roots
- strict runtime ELF loading: self-contained IE64 `ET_DYN`, zero-relative `PT_LOAD`, IOSM metadata, local `R_IE64_RELATIVE64`, W^X, and userland ASLR placement

What is still missing for a real OS feel:

- more commands and utilities so the system feels self-hosting rather than a demo
- default graphical shell / desktop boot
- later scheduler refinement, serious toolchain bring-up, and automatic cleanup of task-owned module handles/resources

## M14: ELF Executable Format + LoadSeg

Goal:
Replace the current stopgap image model with the real native executable/load contract.

Add:

- native `ELF` executable format for IntuitionOS binaries
- `LoadSeg`-style loader semantics and segment-list contract
- DOS-facing load/run path built on the new loader model
- explicit separation between executable format and OS launch semantics

Do not add:

- `HUNK` support
- classic Amiga binary compatibility
- POSIX process/exec culture

Why now:

- the platform needs a stable executable/load model before serious DOS growth
- later toolchain work should target the real loader, not the current stopgap
- the loading model should feel Amiga-like even if the on-disk format is ELF

Suggested demo:

- store native ELF commands in `C:`
- load them through `LoadSeg`
- launch them through `dos.library`
- prove the same DOS-facing model works for commands and user apps

Visible outcome:

- IntuitionOS has a stable native binary format
- loading is segment-based and DOS-shaped
- later tooling can target something intended to last

## M15: DOS Expansion + System Layout

Goal:
Make `dos.library` and the visible system layout feel like a real Amiga-inspired OS
rather than a narrowly sufficient demo substrate.

Add:

- fuller `dos.library`
- first-class system layout and assigns:
  - `RAM:` compatibility preserved as the root/no-prefix view
  - `C:`
  - `L:`
  - `LIBS:`
  - `DEVS:`
  - `T:`
  - `S:`
  - `RESOURCES:`
- more built-in commands and utilities
- more complete launch/search behavior through DOS
- enough shipped system content that the OS feels coherent before desktop work

Keep it Amiga-shaped:

- commands launched through DOS naming and `LoadSeg`
- libraries/devices/resources discovered as services
- `L:` as a qualified helper namespace, not part of bare command fallback
- `T:` as a writable temporary namespace
- no change to the M14.2 ELF-only execution boundary
- no Unix pathname/process culture

Why now:

- the desktop should sit on top of a believable DOS/userland substrate
- toolchain work is less useful before the OS has a real command/runtime environment

Suggested demo:

- boot into a richer text-mode system
- show a fuller `C:` and system assigns
- run multiple real commands and helper tools entirely through DOS

Visible outcome:

- the text-mode OS already feels substantially more complete
- desktop work no longer has to compensate for a thin command/DOS layer
- shipped M15 scope now includes the canonical assign layout, `DOS_ASSIGN`, the richer built-in command/demo set, and the `Type HELP for commands and ASSIGN for layout` boot/demo path while keeping no change to the M14.2 ELF-only execution boundary

## M16: Protected Module Subsystem

Goal:
Replace startup-script-launched libraries with a real protected module subsystem
that keeps the Amiga-shaped `OpenLibrary` model while preserving IOS memory
protection.

Add:

- exec-owned protected module registry and lifecycle
- `OpenLibrary` / `CloseLibrary` as the canonical programmer-facing runtime
  library model
- `dos.library`-owned normal module file/path/loading policy
- library-first public implementation on top of a module-shaped ABI/data model
- compatibility transport remains published behind the runtime while shipped
  library clients acquire libraries through `OpenLibrary` first
- demand-loaded runtime libraries instead of `LIBS/*.library` lines in
  `S:Startup-Sequence`

Why here:

- M15.4 hardens the kernel and loader contract first so the module lifecycle
  work is built on a security model that is actually true
- a believable Amiga-shaped desktop should sit on top of real library/module
  semantics rather than boot-script hacks

Suggested demo:

- `graphics.library` and `intuition.library` are no longer launched as commands
- first `OpenLibrary("graphics.library", 0)` demand-loads the protected library
- later opens reuse the same loaded instance with normal version/open-count
  behavior

Visible shipped M16 outcome:

- IntuitionOS libraries now feel AmigaOS-shaped to callers without giving up
  protected server-task isolation
- `graphics.library` and `intuition.library` demand-load through `OpenLibrary`
  instead of `LIBS/*.library` startup-sequence lines
- `RESIDENT` can pin a library row without turning libraries back into startup-sequence commands
- `SIGF_MODDEAD` notifies openers when an online library task dies, after
  which the next `OpenLibrary` starts a clean reload

Post-M16 note:

- `M16.1` adds universal `IOSM` metadata and resident version discovery
- `M16.2` extends the protected-module model from libraries to handlers,
  devices, and resources as an internal lifecycle and boot-policy milestone:
  `console.handler`, `hardware.resource`, and `input.device` are class-correct
  registry rows, `dos.library` owns the eager post-DOS resource/device policy,
  and `S:Startup-Sequence` no longer launches module files as commands. Public
  `AttachHandler`, `OpenDevice`, and `OpenResource` acquisition APIs are
  deferred to `M16.2.1`
- `M16.2.1` freezes public non-library acquisition as messages to the
  kernel-serviced public `exec.library` port, not as new public syscalls.
- `M16.3` makes `MODF_ASLR_CAPABLE` mandatory for all DOS-loaded ELFs and
  audits/marks shipped commands, libraries, devices, handlers, and resources.
- `M16.4` cuts runtime ELFs over to self-contained IE64 `ET_DYN`: shipped
  commands, libraries, devices, handlers, resources, host-provided DOS ELFs,
  and third-party DOS-loadable runtime ELFs use zero-relative `PT_LOAD`
  addresses, mandatory section headers, bounded local relocation metadata, and
  userland ASLR placement. Dynamic linking remains absent, `ET_EXEC` has no
  post-cutover runtime compatibility exception, and KASLR is deferred.
- `M16.2.1` SDK wrappers send `EXEC_MSG_ATTACH_HANDLER`, `EXEC_MSG_OPEN_DEVICE`, or
  `EXEC_MSG_OPEN_RESOURCE` with a one-page shared request object, and release
  opaque generation-aware tokens through the matching close/detach opcodes.
  That slice is ONLINE-only for non-library rows and keeps compat-port-only use
  as legacy transport.
- `M16.3` made the whole shipped ELF surface consistently PIE-capable and
  enforced the codegen/tooling contract where appropriate
- `M16.4` implements the runtime `ET_DYN` relocation and ASLR/randomized
  placement contract for userland images
- task/process lifecycle hardening remains important follow-on work, especially
  automatic cleanup of task-owned module handles and resources on exit or crash

## M18: Default Graphical Shell

The desktop milestone is listed as `M18` in the current draft ordering.
The intermediate milestone numbering remains intentionally flexible while
the process/toolchain follow-on work after `M16` is still being shaped.

Goal:
Boot to a Workbench-like graphical shell by default when no startup script is present.

Desired boot policy:

1. Boot `exec.library`
2. Start core services
3. Start `dos.library`
4. Check for `S:Startup-Sequence`
5. If present: run it
6. If absent: launch graphical shell by default

Add:

- graphical shell / desktop task
- icon or launcher model
- app launching through DOS + `LoadSeg`
- fallback default desktop behavior

Why here:

- by this point the loader model and DOS substrate are stable enough to support a real shell
- the desktop can feel like an extension of the OS, not a special demo path

Suggested demo:

- no `S:Startup-Sequence`
- system boots straight into the graphical shell
- launch apps from the desktop using the same DOS loading model used in text mode

Visible outcome:

- IntuitionOS starts feeling recognizably like a protected AmigaOS desktop

## M15.1: Source Split Before Disk Loading

Goal:
Split the monolithic `sdk/intuitionos/iexec/iexec.s` into per-component sources before DOS grows disk-backed loading.
split the monolithic `sdk/intuitionos/iexec/iexec.s` into per-component sources

Add:

- per-component assembly sources under `sdk/intuitionos/iexec/`
- a thinner `iexec.s` that stays responsible for top-level ROM/image layout
- stricter source-ownership boundaries without changing runtime behavior
- Phase 2 splits the booted non-DOS services out of the monolithic root:
  `console.handler`, `input.device`, `hardware.resource`, `graphics.library`, and `intuition.library`
- Phase 3 extracts `prog_shell` into `sdk/intuitionos/iexec/handler/shell.s`
- Phase 4 extracts `prog_doslib` into `sdk/intuitionos/iexec/lib/dos_library.s`
  and preserves the full DOS-owned layout block
- Phase 5 splits the remaining DOS-owned subordinate programs and assets:
  `prog_gfxdemo`, `prog_about`, and the ELF fixture
- Phase 6 moves the remaining boot/image wiring into `sdk/intuitionos/iexec/boot/`:
  bootstrap tables and root boot strings

Keep:

- hostfs runtime artifacts now build separately from `runtime_builder.s`
- keep `exec.library` as the only ROM-resident runtime component
- `make intuitionos` as the assembly/build entrypoint
- existing `prog_*` and `boot_*.elf` export labels
- IntuitionOS under `sdk/` until a later milestone intentionally moves it

Why here:

- M15 made the monolithic source file too large and too cross-coupled
- splitting source ownership now is safer than doing it during the later disk-loading work

Visible outcome:

- IntuitionOS is easier to maintain without changing its shipped runtime behavior
- later DOS file-loading work can build on cleaner source boundaries instead of a growing monolith

## M15.2: Host-Backed Boot via `SYS:` + `IOSSYS:`

Goal:
Move IntuitionOS off the embedded-ROM runtime model and onto a host-backed boot tree while keeping the public DOS surface Amiga-shaped and ABI-compatible with M15.

Add:

- `SYS:` as the mounted host-backed boot volume
- `IOSSYS:` as the built-in system assign rooted at `SYS:IOSSYS`
- resolver-owned default mappings for `C:`, `S:`, `L:`, `LIBS:`, `DEVS:`, and `RESOURCES:` under the `IOSSYS:` tree
- a hostfs-backed bootstrap path that loads `dos.library`, shell, commands, libraries, devices, and resources from disk-backed paths

Keep:

- `DOS_ASSIGN` list/query/set in the old `name[16], target[16]` compatibility shape
- `RAM:` compatibility as the bare root view
- `T:` as a writable temporary namespace
- bare command search as `C:`-only
- no change to the M14.2 ELF-only execution boundary
- no `ASSIGN ADD` semantics yet

Why here:

- the ROM-embedded runtime model has served its bootstrap purpose, but later OS work needs a real system tree and a real boot volume
- the storage/bootstrap shift needs to happen before writable overlays, layered assigns, or a desktop can be made coherent

Visible shipped M15.2 outcome:

- the public DOS layout remains M15-shaped while the internal resolver exposes `SYS:` / `IOSSYS:` roots
- `exec.library` stays ROM-resident as the minimal bootstrap nucleus
- `console.handler` boots from `IOSSYS:L/console.handler`
- `dos.library` boots from `IOSSYS:LIBS/dos.library`
- `Shell` boots from `IOSSYS:Tools/Shell`
- generated host system tree under `sdk/intuitionos/system/SYS/IOSSYS`
- `RAM:` / `T:` remain the writable in-memory namespaces

### M15.3 shipped outcome (layered assigns)

- `DOS_ASSIGN` grows three new sub-ops: `DOS_ASSIGN_LAYERED_QUERY` (3),
  `DOS_ASSIGN_ADD` (4), and `DOS_ASSIGN_REMOVE` (5)
- canonical functional assigns (`C:`, `L:`, `LIBS:`, `DEVS:`, `S:`,
  `RESOURCES:`) now back onto a built-in base list
  `[SYS:X, IOSSYS:X]` plus a per-slot mutable overlay list
- `DOS_ASSIGN_LAYERED_QUERY` returns the full effective ordered list
  (overlay entries first, then base list) as 32-byte target slots
- old `DOS_ASSIGN_LIST` / `DOS_ASSIGN_QUERY` remain as compatibility
  projections returning the first effective target only
- old `DOS_ASSIGN_SET` on canonical layered slots replaces the mutable
  overlay list with `[TARGET]` and mirrors the target into the table
  entry so `dos_assign_lookup` (and therefore the hostfs path resolver)
  keeps returning the user's target
- `RAM:`, `T:`, `SYS:`, and `IOSSYS:` remain non-layered and reject
  `ADD` / `REMOVE`
- incidental fix: `.dos_assign_find_entry` no longer lets
  `.dos_assign_name_eq_ci` clobber its loop counter (r20), which had
  been silently returning slot 1 for any table lookup past `C:`

## M15.5: Loader, Toolchain, and Process Groundwork Before M16

Visible shipped M15.5 outcome:

- internal task-death hooks and better hardening diagnostics are now part of exec's private substrate
- MMIO carve-out table and grant-path assumptions are now explicit instead of tribal knowledge
- PIE-capable codegen rules have a canonical IOS-native document and keep the runtime on the strict `ET_EXEC` path
- loader/page-lifecycle hygiene stays within the M15.4 hardening model rather than widening into `ET_DYN`
- `M16` still owns the protected module subsystem

## M15.6: Pre-M16 Kernel Hardening

M15.6 is the hard-gate hardening milestone between M15.5 substrate work
and the M16 protected module subsystem. M16 opens a much wider
privileged surface (module identity, lifecycle state, cross-task ports,
shared memory, host-backed ELF loading); M15.6 widens the hardening
base so M16 does not inherit the SMEP/SMAP, W^X, quota, and cleanup
classes of mistake.

Visible shipped M15.6 outcome:

- host-side JIT memory is never simultaneously writable and executable
  (dual-mapped backing pages; RW view for emit/patch, RX view for
  dispatch)
- IE64 has real SMEP-equivalent (`SKEF`) and SMAP-equivalent (`SKAC`)
  guards plus an explicit supervisor-user-access latch (`SUA`) gated
  through named `copy_from_user` / `copy_to_user` /
  `copy_cstring_from_user` helpers
- user and kernel stacks now carry a dedicated non-present guard page
  below the mapped stack floor, so downward overflow faults cleanly as
  `FAULT_NOT_PRESENT` instead of corrupting adjacent pages
- `AllocMem(MEMF_GUARD)` now reserves flanking non-present guard pages
  around the mapped heap body, and guarded shared allocations carry the
  same contract through `MapShared`
- nested trap state (`faultPC`, `previousMode`, `savedSUA`,
  `faultAddr`, `faultCause`) is now architectural: CPU64 pushes the
  outer frame on trap entry and pops on `ERET`, so kernel handlers no
  longer need to manually save and restore `CR_FAULT_PC` /
  `CR_SAVED_SUA` to survive nested synchronous traps
- user-mode `MTCR CR_FAULT_PC` still faults with `FAULT_PRIV`, while kernel trap handlers may still write `CR_FAULT_PC` before `ERET` to skip or redirect the faulting instruction
- per-task quotas cap blast radius for pages, ports, waiters, shared
  mappings, and grants
- task-exit hook sweeps ports, waiters, reply-port state, M16 registry
  waiters, and grantor-side grant rows, closing the "stale ownership
  after task recycling" gap before M16 opens the registry
- private and shared pages are scrubbed on free so no prior-task bytes
  reach the next owner
- `MapShared` takes an explicit permission bitmask; consumers map RO
  by default, producers map RW; `PTE_X` is never set; missing-mask is
  a hard error (deliberate ABI break, not a compatibility shim)
- execute-only user pages are a real contract backed by loader tests
- HostFS bootstrap confinement is complete against per-component
  symlink escape and case-variant drift
- diagnostic landmarks (MMIO PTE enforcement, `CR_FAULT_PC` contract,
  minimal kernel stack canary behind `KERNEL_STACK_CANARY_ENABLED`,
  scoped atomic RMW plumbing (`m16_atomic_row_try_transition`,
  `m16_atomic_list_push_head`, `m16_atomic_list_detach_all`)) are
  test-pinned rather than tribal knowledge
- `M16` still owns the protected module subsystem; M15.6 lands before
  M16 Phase 1 begins

Out-of-tree callers of `MapShared` predating M15.6 need a one-line
migration: the `MAPF_READ` / `MAPF_WRITE` bitmask is now a required
positional argument, and calls without a mask fault cleanly instead
of taking the old permissive default. See `sdk/docs/IntuitionOS/IExec.md`
for the new calling convention.

See `sdk/docs/IntuitionOS/M15.6-plan.md` for the full TDD phase plan,
hard-gate landmarks, and non-goals.

## Later Milestones

These matter, but they should come **after** M14-M18:

- smarter Executive-style scheduler heuristics
- serious `vbcc` / toolchain bring-up
- persistent storage-backed user identity and later multiuser work
- any future compatibility subsystem for 68K binaries

Why later:

- scheduler complexity is still not the main bottleneck
- toolchain work should target the stable ELF + `LoadSeg` + DOS surface, not churn under it
- multiuser only becomes meaningful once persistent storage and identity exist

## Milestone Summary

1. `M14` ELF executable format + `LoadSeg`
2. `M15` DOS expansion + system layout
3. `M15.4` kernel hardening
4. `M15.5` substrate groundwork before protected modules
5. `M15.6` pre-M16 hardening: JIT W^X, SMEP/SMAP, trap-stack, quotas, exit sweep, zero-on-free, permission-preserving `MapShared`
6. `M16` protected module subsystem
7. `M16.1` universal `IOSM` metadata and VERSION
8. `M16.2` protected handlers/devices/resources internal lifecycle
9. `M16.2.1` public non-library acquisition APIs
10. `M16.3` consistently PIE-capable shipped ELF surface
11. `M16.4` relocation and ASLR/randomized placement
12. `M18` default graphical shell / Workbench-like boot
13. later: scheduler refinement
14. later: serious toolchain bring-up

## Recommended Next Demo Path

If the goal is an impressive near-term demo, do this sequence:

1. `M14`: prove ELF + `LoadSeg` by launching real DOS-loaded binaries
2. `M15`: show a richer DOS system with more commands, assigns, and system content
3. `M15.4`: harden the kernel and loader contract before higher-level runtime changes
4. `M15.5` + `M15.6`: substrate groundwork and pre-M16 hardening
5. `M16`: replace startup-script-launched libraries with the protected module subsystem
6. `M16.2`: bring handlers, devices, and resources onto the same internal protected-module lifecycle
7. `M16.2.1`: freeze public `AttachHandler`, `OpenDevice`, and `OpenResource` acquisition APIs
8. `M16.3`: make shipped ELFs consistently PIE-capable
9. `M16.4`: add relocation and ASLR/randomized placement
10. `M18`: boot to the graphical shell by default when no `S:Startup-Sequence` is present

That path keeps IntuitionOS feeling like a secure AmigaOS:

- microkernel in architecture
- Amiga-shaped in its loading model and userland
- not a POSIX/Linux clone
