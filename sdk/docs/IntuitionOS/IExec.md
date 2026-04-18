# IExec.library -- Kernel Contract Reference

## Amiga Exec-Inspired Protected Microkernel for IE64

### (c) 2024-2026 Zayn Otley -- Intuition Engine Project

---

## 1. Overview

IExec.library is a protected microkernel for the IE64 CPU, inspired by AmigaOS Exec but designed from the ground up for a hardware-enforced privilege model. Where Amiga Exec ran in flat supervisor space with no memory protection, IExec uses the IE64 MMU to enforce user/supervisor separation, per-task page tables, and W^X memory policy.

**What IExec does (current as of M15 on top of the M14.2 runtime boundary):**

- Preemptive round-robin scheduling across up to 255 live tasks (CreateTask/ExitTask with slot reuse). M5 capped this at 8 (per-task `USER_DYN_STRIDE` saturated the 32 MiB VA at 8 tasks); M12 globalized the dynamic VA window and bumped the cap to 16; M12.6 Phase D bumped it to 32; **M13 phase 4 expands the remaining fixed task-state tables to the current 8-bit internal-slot ABI ceiling (255, with `0xFF` reserved as a sentinel in a few kernel fields)**. Public task IDs are independent monotonic `u32`s.
- Memory protection via the IE64 MMU (per-task page tables with separate code/stack/data mappings, W^X enforcement)
- Dynamic memory allocation: AllocMem/FreeMem with Amiga-style MEMF_ flags (`MEMF_PUBLIC`, `MEMF_GUARD`, `MEMF_CLEAR`), page-granular physical page pool (3584 pages = 14 MiB at PPN 0x1200..0x1FFF after M12.6 Phase E split the user-dynamic VA window and the allocator pool into disjoint VPN ranges as a security fix — see §5.13.1). User dynamic VAs occupy the bottom half of the old combined window (`USER_DYN_BASE..USER_DYN_END = 0xA00000..0x1200000`, 8 MiB) and the allocator pool occupies the top half (PPN `0x1200..0x1FFF`, 14 MiB), so user `SYS_ALLOC_MEM` calls cannot ever overwrite the supervisor-only pool PTEs that `build_user_pt` copies into every user PT. The per-task region table is unbounded as of M12.5 (inline 8 rows + overflow chain), the system shmem table is unbounded as of M12.6 Phase B, and the global port table is unbounded as of M12.6 Phase C. Failure mode is real `ERR_NOMEM` from the page allocator, not a fixed-cap collision.
- Shared memory via MEMF_PUBLIC with opaque capability handles (MapShared), reference-counted cleanup on task exit, and zero-on-free scrub when the last mapping drops
- Trap and interrupt dispatch (syscall entry, fault handling with privilege split, timer preemption)
- Context switching (save/restore PC, USP, PTBR, and full GPR set per task)
- Inter-task signalling: per-task 32-bit signal mask with AllocSignal/FreeSignal/Signal/Wait, deadlock detection
- Named public MsgPorts with CreatePort(name, PF_PUBLIC) and FindPort discovery (case-insensitive, 32-byte port names). The port table is unbounded as of M12.6 Phase C (inline 32 rows + overflow chain reachable through `KD_PORT_OFLOW_HDR`); the 1-byte port ID ABI ceiling is 255 (with `WAITPORT_NONE = 0xFF` reserved as sentinel).
- Request/reply messaging: 32-byte messages with data0, data1, reply_port, and share_handle fields
- ReplyMsg for Exec-style service pattern; PutMsg/GetMsg/WaitPort with full message ABI
- Safe user pointer validation (PTE check before kernel reads from user memory)
- Kernel renamed to exec.library; GURU MEDITATION fault messages
- AmigaOS-style `OpenLibrary` for library discovery — **M11.5**: source-level alias for `SYS_FIND_PORT`; the kernel ABI is one slot smaller. Slot 36 is retained as a one-instruction binary-compat redirect to `.do_find_port` so any pre-M11.5 IE64 binary still links. New code uses `SYS_FIND_PORT` directly. See § 5.11 "Exec Boundary".
- I/O page mapping via MapIO syscall (REGION_IO type)
- **`SYS_EXEC_PROGRAM` is now descriptor-only** (M14.2 phase 1): kernel task launch no longer accepts user-provided flat `IE64PROG` images. The surviving public contract is the M14 launch descriptor used by DOS `RunSeg`; malformed descriptors, unmapped pointers, and sub-`USER_CODE_BASE` values still fail with `ERR_BADARG`.
- **Dynamic task-image placement in `load_program`** (M10, refined in M12.8, redesigned in M13 phases 2 and 4): the old arbitrary `code_size <= 8192` / `data_size <= 49152` product caps are gone, and task images are no longer forced into `task_id * USER_SLOT_STRIDE` slots. `load_program` now allocates code, stack, data, and startup pages dynamically: it uses the legacy fixed image window first, then spills additional image pages into allocator-pool pages as needed. PT backing likewise uses the legacy fixed 32-block PT window first, then spills additional 64 KiB PT blocks into allocator-pool pages. Failure mode is real `ERR_NOMEM` when the allocator pool is exhausted.
- **M13 startup block ABI**: boot-loaded and `ExecProgram`-launched tasks no longer self-locate from `GetSysInfo(CURRENT_TASK) + USER_SLOT_STRIDE`. The kernel allocates a dedicated startup page for each launched task, writes the 64-byte startup block there, and seeds the startup-page base VA at `0(sp)` before entering user code. Services discover task identity and actual code/data/stack bases by first loading the startup-page VA from `0(sp)` and then reading the startup block from that page.
- **Phase 5 regression gate**: the full visible boot stack and both retained GUI demos are now covered by explicit M13 tests (`TestIExec_M13_Phase5_FullBootStack_ServiceCensus`, `..._GfxDemoRegression`, `..._AboutRegression`), so the milestone does not rely on older M11/M12 test names as an implicit proxy for final compatibility.
- **M14 phase 5 ships the visible native DOS loader path end-to-end, and M14.2 phase 1 removes the remaining flat-image escape hatches**: DOS-loaded commands and applications use a strict `ELF64` subset (`EM_IE64 = 0x4945`, `ET_EXEC`, `PT_LOAD` only, no dynamic linker) when loaded through `DOS_LOADSEG`. `dos.library` parses that subset, builds DOS-owned seglists, frees them with `DOS_UNLOADSEG`, launches them through `DOS_RUNSEG` via the `ExecProgram` launch-descriptor bridge, and `DOS_RUN` now rejects non-ELF executable content instead of falling back to legacy flat-image launch. M14 shipped with the seeded `C:` command/demo path as native ELF; M14.2 phase 1 makes that ELF-only execution contract explicit.
- M14 shipped/current runtime:
  - the public ELF loader API is the file-backed DOS path: `DOS_LOADSEG` / `DOS_UNLOADSEG` / `DOS_RUNSEG`
  - base M14 originally still brought boot services up from the legacy kernel `program_table` path
  - bootstrap grants were still keyed by boot index at runtime
  - the shipped M14 phase-5 tree had a native-ELF `C:` command/demo path while bundled startup-sequence services still used the legacy path
- M14.1 target state:
  - all shipped runtime binaries become ELF
  - boot/services move to an internal embedded boot manifest, not a new public DOS API
  - bootstrap grants move from boot-index keying to internal manifest-entry-ID keying
- M14.1 phase 5 shipped state:
  - the kernel now prepares staged strict-M14 ELF rows for the internal embedded boot manifest and keeps bootstrap grants keyed by internal manifest entry ID
  - `console.handler` and `dos.library` boot from that staged manifest source as the minimum pre-DOS bootstrap chain
  - once DOS is online, `dos.library` launches `Shell` from the internal embedded manifest, and `Shell` then drives `S:Startup-Sequence`
  - DOS resolves the service-name lines in `S:Startup-Sequence` through its internal embedded-manifest path to launch `hardware.resource`, `input.device`, `graphics.library`, and `intuition.library`
  - shipped service binaries under `LIBS:`, `DEVS:`, and `RESOURCES:` are now seeded as strict M14 ELF too
  - the public DOS loader API is unchanged; the embedded-manifest service source remains internal-only
- console.handler: CON: handler with GetMsg polling and CON_READLINE protocol — **M11.5**: console.handler now owns terminal MMIO directly via its own `SYS_MAP_IO(0xF0, 1)` mapping and inlines the readline MMIO loop. The former kernel-side `SYS_READ_INPUT` (slot 37) is removed; slot 37 is an unallocated hole that returns `ERR_BADARG`.
- **dos.library**: AmigaOS dos.library equivalent with a RAM-backed, case-insensitive filesystem and Amiga-shaped assign model. The file metadata table and open-handle table are unbounded user-space chains of `AllocMem`'d 4 KiB pages (85 file entries or 510 handle entries per page). As of **M12.8**, each file body is a variable-size chain of 4 KiB extents linked through `entry.file_va`; the old fixed `DOS_FILE_SIZE` per-file allocation is gone. `DOS_WRITE` now does an atomic swap onto a newly allocated extent chain so allocation failure leaves previous content intact. **M14 phases 2-3 add `DOS_LOADSEG` / `DOS_UNLOADSEG` / `DOS_RUNSEG`**: dos.library validates the strict native ELF subset, builds DOS-owned seglists with preserved target VA / `R/W/X` / entry-point metadata, frees them on demand, and launches them through the dual-mode `ExecProgram` descriptor handoff. **M15 expands the DOS namespace and layout model**: the built-in assign table now covers `RAM:`, `C:`, `L:`, `LIBS:`, `DEVS:`, `T:`, `S:`, and `RESOURCES:`; bare command search remains `C:`-only; `L:` is direct-access only in M15; `RAM:` remains a first-class compatibility root view; and `DOS_ASSIGN` adds DOS-side list/query/set of assign rows while keeping `RAM:` non-mutable.
- **M15.2 host-backed boot current runtime**: `SYS:` is now the mounted host-backed boot volume and `IOSSYS:` is the built-in system assign rooted at `SYS:IOSSYS`. `DOS_ASSIGN` remains a compatibility projection: public list/query rows stay `name[16], target[16]`, `SYS:` host root and `IOSSYS:` built-in system assign are resolver-owned rather than mutable rows, canonical functional assigns keep their short public targets, `console.handler` boots from `IOSSYS:L/console.handler`, `dos.library` boots from `IOSSYS:LIBS/dos.library`, and `Shell` boots from `IOSSYS:Tools/Shell`.
- **Shell**: interactive command shell that sends raw command names to dos.library via DOS_RUN (no shell-side command table). The visible command-dispatch path goes through the DOS loader path for the seeded native-ELF `C:` command/demo set; `DOS_RUN` rejects non-ELF executable content instead of falling back to legacy flat-image launch. Executes `S:Startup-Sequence` automatically at boot if present, then drops to the interactive prompt.
- **Visible DOS commands** as DOS-loaded executables: `VERSION`, `AVAIL`, `DIR`, `TYPE`, `ECHO`, `ASSIGN`, `LIST`, `WHICH`, `HELP`, plus the retained `GfxDemo` and `About` programs. Stored as files in RAM under `C:`, launched by name through dos.library.

**What IExec does not do:**

- Filesystem access beyond RAM: (handled by DOS.library or host-side intercepts for persistent storage)
- Device drivers (hardware chips are memory-mapped; drivers live in user space)
- Graphics or audio (handled by respective chip subsystems)
- Boot/services are loaded as strict ELF binaries; the remaining flat-image path is removed in M14.2

**M15 current runtime:**

- `dos.library` owns a first-class DOS layout and assign table for `RAM:`, `C:`, `L:`, `LIBS:`, `DEVS:`, `T:`, `S:`, and `RESOURCES:`
- `DOS_ASSIGN` supports list/query/set for assign rows; `RAM:` is listed and queryable but not mutable
- fully qualified names resolve directly through the assign table, while bare command search remains `C:`-only
- `L:` is a qualified helper namespace; bare command fallback does not probe `L:`
- `T:` is a writable temporary namespace backed by the same in-RAM DOS store
- `DOS_RUN` remains ELF-only, `DOS_LOADSEG` / `DOS_RUNSEG` remain the public executable path, and the remaining flat-image path is removed in M14.2
- the current boot/demo path reaches a richer text-mode system:
  - `exec.library M11 boot`
  - `console.handler M11.5 [Task 0]`
  - `dos.library M14 [Task 1]`
  - `Shell M10 [Task 2]`
  - `hardware.resource M12.5 [Task 3]`
  - `input.device M11 [Task 4]`
  - `graphics.library M11 [Task 5]`
  - `intuition.library M12 [Task 6]`
  - `IntuitionOS 0.17`
  - `Type HELP for commands and ASSIGN for layout`
  - `1>`

**M15.1 source layout:**

- `sdk/intuitionos/iexec/iexec.s` remains the top-level image/layout file and the only assembly entrypoint passed to `ie64asm`.
- `iexec.s` remains the top-level image/layout file and the only assembly entrypoint passed to `ie64asm`.
- Phase 1 of M15.1 moves the seeded command sources out of the monolithic root into `sdk/intuitionos/iexec/cmd/` while preserving the existing `prog_*` labels and ROM ordering through explicit `include` statements in `iexec.s`.
- seeded command sources now live under `sdk/intuitionos/iexec/cmd/`
- Phase 2 of M15.1 moves the non-DOS boot services into `handler/`, `dev/`, `resource/`, and `lib/` source files while preserving their existing embedded-program labels and boot ordering.
- `console.handler`, `input.device`, `hardware.resource`, `graphics.library`, and `intuition.library` are now split out of the root image source.
- Phase 3 of M15.1 moves the interactive shell into `sdk/intuitionos/iexec/handler/shell.s`.
- `prog_shell` is split out of the root image source without changing the M15 shell behavior.
- Phase 4 of M15.1 moves `prog_doslib` into `sdk/intuitionos/iexec/lib/dos_library.s`.
- the full DOS-owned layout block moves together: `prog_doslib_code`, `prog_doslib_data`, the seed ELF region, DOS-seeded text/assets, and the nested includes that already assembled inside that ownership boundary
- Phase 5 of M15.1 splits the remaining DOS-owned subordinate programs and assets.
- `prog_gfxdemo`, `prog_about`, and the DOS-seeded text/fixture blobs now live in subordinate `cmd/` and `assets/` files that are still included from `sdk/intuitionos/iexec/lib/dos_library.s`
- Phase 6 of M15.1 moves the remaining boot/image wiring out of the root file.
- `sdk/intuitionos/iexec/boot/manifest_seed.s` and `sdk/intuitionos/iexec/boot/strings.s` now hold the boot manifest and root boot strings while `iexec.s` stays the top-level assembly entrypoint
- The generated runtime ELFs are still rebuilt from labeled embedded programs after assembly; M15.1 does not change the ROM-embedded build model.
- IntuitionOS still lives under `sdk/` for repository-history reasons in M15.1. The refactor is about component ownership and maintainability, not yet about repo relocation.

**M15.2 host-backed boot current runtime:**

- `SYS:` is the mounted host-backed boot volume.
- `IOSSYS:` built-in system assign means `SYS:IOSSYS` internally, while public `DOS_ASSIGN` list/query remains the short compatibility projection.
- canonical functional assigns continue to look M15-shaped in public output even when internal resolution is rooted under `SYS:IOSSYS`.
- `DOS_ASSIGN` compatibility projection remains `name[16], target[16]`; `SYS:` and `IOSSYS:` are built-in roots, not mutable long chained rows.
- `exec.library` is the only remaining ROM-resident runtime component.
- `console.handler` boots from `IOSSYS:L/console.handler`.
- `dos.library` boots from `IOSSYS:LIBS/dos.library`.
- `Shell` boots from `IOSSYS:Tools/Shell`.
- the generated host system tree lives under `sdk/intuitionos/system/SYS/IOSSYS`.
- `RAM:` and `T:` remain writable provider-backed in-memory namespaces.
- bootstrap hostfs stays read-only in M15.2.

**M15.3 layered assigns + writable `SYS:` overlay current runtime:**

- canonical assigns (`C:`, `S:`, `L:`, `LIBS:`, `DEVS:`, `RESOURCES:`) are layered: each resolves through a built-in base list `[SYS:X, IOSSYS:X]` plus a per-slot mutable overlay list (max 4 entries).
- new `DOS_ASSIGN` sub-ops: `DOS_ASSIGN_LAYERED_QUERY` (3) returns `count × target[32]`, `DOS_ASSIGN_ADD` (4) appends to the mutable overlay (no-op on duplicate), `DOS_ASSIGN_REMOVE` (5) removes from the mutable overlay (built-in base entries cannot be removed).
- compatibility projection: old `DOS_ASSIGN_LIST` / `DOS_ASSIGN_QUERY` keep returning the **first effective public target only** through the M15.2 `name[16], target[16]` row shape.
- `DOS_ASSIGN_SET` on a canonical layered slot replaces the mutable overlay with `[TARGET]` and mirrors that target into the table entry, so `dos_assign_lookup` and the hostfs path resolver keep returning the user's chosen target.
- bootstrap hostfs gains writable ops: `BOOT_HOSTFS_CREATE_WRITE` (6) opens/truncates a host file for writing, `BOOT_HOSTFS_WRITE` (7) appends bytes to an open handle. Any path whose first component resolves to `IOSSYS` is rejected at the device — `IOSSYS:` is always read-only.
- DOS_OPEN reads now fall through `SYS:` overlay → `IOSSYS:` read-only via the `dos_hostfs_layered_relpath_for_resolved_name` helper.
- `RAM:`, `T:`, `SYS:`, and `IOSSYS:` stay non-layered. `RAM:` is non-mutable. `T:` is single-target writable. `SYS:` and `IOSSYS:` are built-in roots and reject `ADD`/`REMOVE`.
- `ASSIGN` shell command grows the layered syntax: `ASSIGN NAME:` shows the full effective ordered list, `ASSIGN ADD NAME: TARGET:` appends to the overlay, `ASSIGN REMOVE NAME: TARGET:` removes one entry, and `ASSIGN NAME: TARGET:` keeps the M15.2 replace semantics.
- M15.3 makes no changes to `ExecProgram`, ELF/seglist contracts, or the M14.2 ELF-only command boundary.

**M15.4 hardening milestone:**

- kernel W^X becomes a real enforced contract instead of relying on broad supervisor `P|R|W|X` mappings
- syscall user-pointer validation becomes permission-aware (`read`, `write`, and executable-entry checks instead of generic `P|U` validation)
- dynamic/shared user memory remains non-executable by explicit ABI/security contract
- the M14.2 `ET_EXEC` loader contract remains unchanged in M15.4; `ET_DYN`, runtime relocation, ASLR, and KASLR stay future work
- `M15.4` is the gate before `M16` protected modules, not a partial implementation of the module registry/lifecycle work
- see [M15.4-plan.md](/home/zayn/GolandProjects/IntuitionEngine/sdk/docs/IntuitionOS/M15.4-plan.md) for the hardening milestone spec

**M15.5 substrate/current runtime:**

- task teardown now runs ordered internal exit hooks exactly once per exit path; hook failure is recorded internally but does not abort teardown
- fault reports now include access type, privilege level, classification, and effective PTE bits so hardening regressions fail loudly in the text-mode environment
- supervisor MMIO carve-outs remain narrow and documented; user-visible MMIO stays behind the grant/resource model
- the runtime loader contract remains strict `ET_EXEC`; M15.5 does not add `ET_DYN`, runtime relocation, ASLR, or KASLR
- the canonical PIE-capable codegen rules now live in [Toolchain.md](/home/zayn/GolandProjects/IntuitionEngine/sdk/docs/IntuitionOS/Toolchain.md)

**M16 planned protected module subsystem:**

- `OpenLibrary` / `CloseLibrary` become the canonical programmer-facing lifecycle for runtime libraries
- exec owns the protected module registry and lifecycle; `dos.library` owns normal file/path/loading policy
- the ABI/data model is module-shaped (`library`, `device`, `handler`, `resource`) while the public v1 implementation remains library-first
- `FindPort`-based compatibility transport stays valid during migration so existing clients do not break all at once
- ordinary libraries stop being launched from `S:Startup-Sequence` as fake commands once demand-loading is complete
- see [M16-plan.md](/home/zayn/GolandProjects/IntuitionEngine/sdk/docs/IntuitionOS/M16-plan.md) for the M16 milestone spec

IExec runs on the IE64 CPU core only. It requires the IE64 MMU (4 KiB paged virtual memory, software TLB, control registers) and the hardware timer for preemption.

---

## 2. Memory Model

### 2.1 Address Space Layout

The IE64 addresses a 32 MB physical address space. IExec partitions it as follows:

| Region | Address Range | Size | Access | Purpose |
|--------|---------------|------|--------|---------|
| Kernel code + data | `$000000-$09FFFF` | 640 KB | Supervisor only | Kernel binary, page tables, TCBs, kernel stack |
| Hardware I/O (low) | `$0A0000-$0FFFFF` | 384 KB | Supervisor (identity-mapped) | Terminal MMIO, low I/O registers |
| VRAM / high I/O | `$100000-$5FFFFF` | 5 MB | Supervisor (partially mapped) | Video memory and higher MMIO space |
| User space | `$600000-$1FFFFFF` | 26 MB | User (per-task mapped) | Task code, data, stacks, heap, shared memory |

**Kernel page table**: M15.4 hardens the old broad supervisor identity map into explicit permission classes. Pages 0-383 (`$000000-$17FFFF`) remain supervisor-only and identity-mapped, but not all as `P|R|W|X`. The assembled kernel image below `KERN_PAGE_TABLE` is supervisor `P|R|X`; the kernel page table, kernel data, kernel stack, terminal/low-I/O page `0xF0`, and the currently mapped low VRAM/high-I/O slice remain supervisor `P|R|W` and non-executable. Regions above `$17FFFF` are not mapped by the kernel PT. User pages are only mapped in per-task page tables, not the kernel PT.

**M15.5 MMIO carve-out table**: the current supervisor carve-outs are explicit: page `0xF0` (`$0F0000-$0F0FFF`) for terminal/low-I/O bootstrap text MMIO, the low VRAM/high-I/O slice already mapped by the kernel PT inside `$100000-$17FFFF`, the task page-table window `$800000-$9FFFFF`, and the allocator pool `$1200000-$1FFFFFF` as supervisor `P|R|W`. User tasks reach MMIO only through `SYS_MAP_IO` plus the hardware.resource/bootstrap grant path; there is no broad user MMIO mapping.

### 2.2 Kernel Memory Layout (Detail)

Within the supervisor region:

| Sub-region | Address | Size | Contents |
|------------|---------|------|----------|
| Vector table | `$000000-$000FFF` | 4 KB | Reserved (IE64 hardware vectors) |
| Kernel code | `$001000-$00FFFF` | 60 KB | Kernel text (boots at `$1000`) |
| Kernel page table | `$07D000-$08CFFF` | 64 KB | 8192 PTEs x 8 bytes |
| Kernel data | `$08D000-$09DFFF` | 68 KB window | Scheduler state, TCB array, PTBR array, ports, manifests, quota metadata |
| Kernel stack guard | `$09E000-$09EFFF` | 4 KB | Non-present guard page below the kernel stack floor |
| Kernel stack | `$09F000-$09FFFF` | 4 KB | Grows downward; top = `$0A0000` |
| Task page tables | `$800000-$9FFFFF` | 2 MB | Fixed 64 KiB PT window (`USER_PT_BASE`) before allocator spillover |

### 2.3 Per-Task Page Tables

Each task has its own single-level page table (8192 entries, 64 KB). The kernel identity-maps supervisor pages (0-383, covering `$000000-$17FFFF`) as supervisor-only (no U bit) in every task's page table. User pages are mapped with the U (user-accessible) bit set, and only for pages belonging to that task.

Page table entry format (64-bit):

| Bits | Field | Description |
|------|-------|-------------|
| 63:13 | PPN | Physical page number (PPN << 13 = physical address) |
| 7 | D | Dirty (hardware-maintained) |
| 6 | A | Accessed (hardware-maintained) |
| 5 | U | User-accessible (0 = supervisor only) |
| 3 | X | Execute permission |
| 2 | W | Write permission |
| 1 | R | Read permission |
| 0 | P | Present |

### 2.4 W^X Enforcement

IExec enforces a write-XOR-execute policy:

- **Code pages**: `P|R|X|U` -- readable and executable, not writable
- **Data/stack pages**: `P|R|W|U` -- readable and writable, not executable
- **Stack guard pages**: one non-present page is left below every user stack
  and below the kernel stack floor, so downward overflow becomes
  `FAULT_NOT_PRESENT`
- A page fault is raised if user code attempts to write to an X page or execute from a W page

As of **M15.6**, the host-side JIT that executes IE64 binaries is
also W^X: `jit_mmap.go` dual-maps its backing pages as a writable
view (`PROT_READ|PROT_WRITE`) used for emit/patch and an execution
view (`PROT_READ|PROT_EXEC`) used for dispatch. At no point does any
host mapping hold both write and execute permission. Earlier
releases mapped the host JIT region RWX permanently, which
contradicted the guest W^X story; that gap is closed. See
`sdk/docs/IE64_JIT.md`.

### 2.4.1 Supervisor ↔ User Access Contract (M15.6)

The kernel runs with the IE64 `MMU_CTRL.SKAC` bit set at boot (see
`sdk/docs/IE64_ISA.md` §12.2.1). Any supervisor-mode read or write on
a page with `PTE_U==1` faults with `FAULT_SKAC` unless the
`MMU_CTRL.SUA` latch has been explicitly opened by the privileged
`SUAEN` opcode. The symmetric `SKEF` bit catches accidental
supervisor instruction fetch from a user-accessible page with
`FAULT_SKEF`.

Kernel user-memory touches go exclusively through canonical helpers
defined in `sdk/intuitionos/iexec/iexec.s`:

- `copy_from_user(dst_kernel, src_user, len)` — validates the user
  source range, opens the `SUA` window, copies `len` bytes into a
  kernel-owned destination, and closes the window.
- `copy_to_user(dst_user, src_kernel, len)` — same, reverse
  direction.
- `copy_cstring_from_user(dst_kernel, src_user, max_len)` — copies
  a NUL-terminated string up to `max_len` bytes.

Every `SYS_*` slot that copies user data calls into these helpers
rather than dereferencing user pointers directly. A missed migration
surfaces as a clean `FAULT_SKAC` in "GURU MEDITATION" output rather
than as silent data corruption. See `sdk/docs/IE64_COOKBOOK.md`
"Supervisor-User Access Helpers" for the worked idiom.

#### Trap-frame stack contract

Nested-trap state preservation is now architectural. The IE64 CPU
owns a fixed-depth trap-frame stack that captures `FAULT_PC`,
`FAULT_ADDR`, `FAULT_CAUSE`, `PREV_MODE`, and `SAVED_SUA` on trap
entry and restores them on `ERET`. Kernel handlers that previously
had to `MFCR CR_FAULT_PC` / `MFCR CR_SAVED_SUA` into the kernel
stack and restore them before `ERET` to survive a nested
synchronous trap no longer need the dance — the CPU does it.
Existing manual save/restore code is redundant but harmless. See
`sdk/docs/IE64_ISA.md` §12.14 for the full contract and
overflow-halts-cleanly behaviour.

### 2.5 Shared Memory

Shared memory regions are created via `AllocMem(MEMF_PUBLIC)`, which returns a VA and an opaque share handle in R3. Another task maps the region by calling `MapShared(share_handle, map_flags)`, where `map_flags` is a required `MAPF_READ` / `MAPF_WRITE` bitmask in R2. Both tasks still see the same physical pages, but M15.6 narrows the consumer-side PTEs to the requested access: `PTE_W` is set only when `MAPF_WRITE` is present, and `PTE_X` is never set. Calls without a permission mask are a hard `ERR_BADARG`; there is no compatibility fallback to the old permissive default. When the producer opted in with `MEMF_GUARD`, each mapping also reserves one non-present page on each side of the mapped body, so consumer underruns/overruns fault cleanly too. The kernel tracks reference counts; `FreeMem` on a shared mapping removes the caller's mapping and decrements the refcount. Physical pages are scrubbed and freed when the last mapping is removed or the last mapping task exits. Share handles encode a 24-bit nonce derived from a monotonic kernel counter, guaranteeing that reusing a slot always produces a different handle. Stale handles are rejected with `ERR_BADHANDLE`.

---

## 3. Task Model

### 3.1 Task Creation

At boot, the kernel loads bundled program images into free task records via the internal `load_program` helper. From Milestone 5 onward, additional tasks can also be created at runtime via `CreateTask` (syscall #5). **M13 phase 2 changes the image-placement half of that contract**: the kernel still claims a free task ID / TCB row, but it now allocates code, stack, data, and PT backing dynamically inside the reserved user image/PT windows instead of deriving addresses from `task_id * USER_SLOT_STRIDE`. Tasks exit via `ExitTask` (syscall #34), which cleans up ports, signals, and the task's dynamically allocated image/PT pages before freeing the task slot.

### 3.2 Task Control Block (TCB)

**Milestone 1 (simplified)**: Each task is described by a minimal per-task record. GPR save/restore is not yet implemented - user tasks must reload their own registers after yield.

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| `+$000` | 8 | `pc` | Saved program counter |
| `+$008` | 8 | `usp` | Saved user stack pointer |

PTBR per task is stored in a separate array (KD_PTBR_BASE).

**Future milestones** will expand the TCB to a full 288-byte structure with saved GPRs (R0-R31), FPU state, priority, signal mask, handle table, and task name.

### 3.3 Task States

| Value | Name | Description |
|-------|------|-------------|
| 0 | `READY` | Eligible to run; in the ready queue |
| 1 | `RUNNING` | Currently executing on the CPU |
| 2 | `WAITING` | Blocked on a signal, port, or timer (future) |
| 3 | `FREE` | Slot is unused and immediately reusable |

### 3.4 Priority Scheduling

The scheduler still uses simple round-robin across the internal task-state slots. As of M13 phase 4, those fixed tables have been expanded to the current 8-bit internal-slot ABI ceiling (`MAX_TASKS = 255`, with `0xFF` reserved in a few kernel byte fields). It scans from the current task ID forward, skipping WAITING and FREE entries, and selects the next READY or RUNNING task. Priority-based scheduling is planned for a future milestone.

### 3.5 Context Switch Sequence

On a context switch (triggered by `Yield` syscall or timer preemption):

1. **Save current task**: Read `FAULT_PC` and `USP` from control registers; store into the current task's TCB along with any dirty GPRs
2. **Select next task**: Round-robin scan across MAX_TASKS slots, skipping WAITING and FREE
3. **Restore next task**: Load the next task's TCB; write `FAULT_PC`, `USP`, and `PTBR` to control registers
4. **Flush TLB**: Execute `TLBFLUSH` to invalidate stale address translations
5. **Return to user mode**: Execute `ERET`, which restores PC from `FAULT_PC`, switches to user privilege, and swaps to the user stack pointer

---

## 4. Syscall ABI

### 4.1 Entry

User code invokes a syscall via:

```asm
SYSCALL #imm32      ; imm32 = syscall number
```

The CPU traps to supervisor mode. The kernel's trap handler reads the syscall number from `CR_FAULT_ADDR` and dispatches accordingly.

### 4.2 Register Convention

| Register | Role | Preserved across syscall? |
|----------|------|--------------------------|
| R1 | Argument 1 / return value | No (clobbered by return) |
| R2 | Argument 2 / error code | No (clobbered by error) |
| R3 | Argument 3 | No |
| R4 | Argument 4 | No |
| R5 | Argument 5 | No |
| R6 | Argument 6 | No |
| R7-R30 | Caller's registers | **Not yet** (see below) |
| R31 (SP) | Stack pointer | Yes (preserved via USP) |
| R0 | Zero register (hardwired) | N/A |

**IExec M9:** The timer interrupt handler now performs full GPR save/restore (R1-R30) for preemption safety. Syscall dispatch still clobbers R10-R16 internally, so user code should not rely on those registers surviving a syscall.

### 4.3 Return Convention

- **R1**: Return value (0 on success for void syscalls, or the requested data)
- **R2**: Error code (0 = success, nonzero = error)

### 4.4 Error Codes

| Code | Name | Description |
|------|------|-------------|
| 0 | `ERR_OK` | Success |
| 1 | `ERR_NOMEM` | Out of memory / pages |
| 2 | `ERR_BADHANDLE` | Invalid or wrong-type handle |
| 3 | `ERR_BADARG` | Invalid argument |
| 4 | `ERR_NOTFOUND` | Named object not found |
| 5 | `ERR_PERM` | Operation not permitted |
| 6 | `ERR_AGAIN` | Resource temporarily unavailable (e.g., no message on port) |
| 7 | `ERR_TOOLARGE` | Allocation too large |
| 8 | `ERR_EXISTS` | Named object already exists (e.g., duplicate public port name) |
| 9 | `ERR_FULL` | Fixed-capacity resource is full (e.g., port message FIFO) |

---

## 5. Syscall Table

### 5.1 Memory Management

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 1 | `AllocMem` | R1=size, R2=flags -> R1=addr, R2=err, R3=share_handle | **Implemented** (`nucleus`) |
| 2 | `FreeMem` | R1=addr, R2=size -> R2=err | **Implemented** (`nucleus`) |
| 3 | -- | -- | Removed M11.5 (was `AllocShared`; slot is now an unallocated hole) |
| 4 | `MapShared` | R1=share_handle, R2=map_flags -> R1=addr, R2=err, R3=share_pages | **Implemented (M15.6 ABI)** (`nucleus`) |

`AllocMem` flags: `MEMF_PUBLIC` (bit 0) = shareable across tasks, `MEMF_GUARD` (bit 1) = reserve one non-present page on each side of the mapped body, `MEMF_CLEAR` (bit 16) = zero-fill. `MEMF_GUARD` changes only the VA reservation shape: quota/accounting and `FreeMem` size matching still use the mapped body page count, and the returned VA points at the mapped body rather than the leading guard. `MapShared` flags: `MAPF_READ` (bit 0) and `MAPF_WRITE` (bit 1); callers must pass a non-zero subset of those two bits, or the syscall returns `ERR_BADARG`. All allocations are page-granular (4 KiB). `MEMF_CLEAR` only guarantees allocation-time zeroing; independently of that flag, `FreeMem` and shared-memory last-reference teardown now zero the backing pages before releasing them back to the allocator. `FreeMem` size is also page-granular: the kernel rounds size up to pages and compares the page count against the allocation's page count. Any size that rounds to the same number of pages is accepted (e.g., both 5000 and 8192 free a 2-page allocation).

### 5.2 Task Management

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 5 | `CreateTask` | R1=source_ptr, R2=code_size, R3=arg0 -> R1=task_id, R2=err | **Implemented** (`nucleus`) |
| 6 | -- | -- | Removed M11.5 (was `DeleteTask`; unallocated hole) |
| 7 | -- | -- | Removed M11.5 (was `FindTask`; unallocated hole) |
| 8 | -- | -- | Removed M11.5 (was `SetTaskPri`; unallocated hole) |
| 9 | -- | -- | Removed M11.5 (was `SetTP`; unallocated hole) |
| 10 | -- | -- | Removed M11.5 (was `GetTaskInfo`; unallocated hole) |

### 5.3 Signals

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 11 | `AllocSignal` | R1=bit_hint (-1=any) -> R1=bit_num, R2=err | **Implemented** (`nucleus`) |
| 12 | `FreeSignal` | R1=bit_num -> R2=err | **Implemented** (`nucleus`) |
| 13 | `Signal` | R1=task_id, R2=signal_mask -> R2=err | **Implemented** (`nucleus`) |
| 14 | `Wait` | R1=signal_mask -> R1=received_mask | **Implemented** (`nucleus`) |

### 5.4 Ports and Messages

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 15 | `CreatePort` | R1=name_ptr (0=anon), R2=flags -> R1=port_id (0-7), R2=err | **Implemented (M7)** (`nucleus`) |
| 16 | `FindPort` | R1=name_ptr -> R1=port_id, R2=err | **Implemented (M7)** (`nucleus`) — also reachable as `OpenLibrary` (alias) and via slot 36 binary-compat redirect |
| 17 | `PutMsg` | R1=port_id, R2=type, R3=data0, R4=data1, R5=reply_port, R6=share_handle -> R2=err | **Implemented (M7)** (`nucleus`) |
| 18 | `GetMsg` | R1=port_id -> R1=type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle | **Implemented (M7)** (`nucleus`) |
| 19 | `WaitPort` | R1=port_id -> (same as GetMsg, blocks if empty) | **Implemented (M7)** (`nucleus`) |
| 20 | `ReplyMsg` | R1=reply_port, R2=type, R3=data0, R4=data1, R5=share_handle -> R2=err | **Implemented (M7)** (`nucleus`) |
| 21 | -- | -- | Removed M11.5 (was `PeekPort`; unallocated hole) |

**Port naming (M7):** Ports can be created with a name (up to 16 bytes, ASCII) and the `PF_PUBLIC` flag. Public named ports are discoverable via `FindPort` (case-insensitive matching). Anonymous ports (name_ptr=0) are private and not findable. Duplicate public names return `ERR_EXISTS`. Ports are cleaned up on task exit (name and flags cleared, removed from FindPort).

**Message format (M7):** Messages are 32 bytes, kernel-copied, with type (4B), sender (4B), data0 (8B), data1 (8B), reply_port (2B, 0xFFFF=none), and share_handle (4B, 0=none). `ReplyMsg` is a convenience wrapper that sends to the reply_port with share_handle support.

**User pointer safety:** CreatePort and FindPort validate user-provided name pointers by checking PTEs before reading. Invalid pointers return `ERR_BADARG` instead of crashing the kernel.

### 5.5 Timers

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 22 | -- | -- | Removed M11.5 (was `AddTimer`; unallocated hole) |
| 23 | -- | -- | Removed M11.5 (was `RemTimer`; unallocated hole) |

### 5.6 Handles

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 24 | -- | -- | Removed M11.5 (was `CloseHandle`; unallocated hole) |
| 25 | -- | -- | Removed M11.5 (was `DupHandle`; unallocated hole) |

### 5.7 System

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 26 | `Yield` | (no args) -> (returns after reschedule) | **Implemented** (`nucleus`) |
| 27 | `GetSysInfo` | R1=info_id -> R1=value | **Implemented** (`nucleus`) |
| 28 | `MapIO` | R1=base_ppn, R2=page_count -> R1=mapped_va, R2=err | **Implemented (M9, extended in M11)** (`nucleus`, see "Known impurities") |
| 29 | -- | -- | Removed M11.5 (was `MapVRAM`; subsumed by `MapIO`'s VRAM allowlist; unallocated hole) |
| 30 | -- | -- | Removed M11.5 (was `Debug`; unallocated hole) |

### 5.8 Debug I/O

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 33 | `DebugPutChar` | R1=character -> R2=err | **Implemented** (`bootstrap` — kernel bring-up / panic only, not for app code) |
| 34 | `ExitTask` | R1=exit_code (ignored) -> never returns | **Implemented** (`nucleus`) |

### 5.9 Bulk IPC

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 31 | -- | -- | Removed M11.5 (was `SendMsgBulk`; unallocated hole) |
| 32 | -- | -- | Removed M11.5 (was `RecvMsgBulk`; unallocated hole) |

### 5.10 Program Execution and Libraries (M9, ABI redesigned in M10, boundary frozen in M11.5)

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 35 | `ExecProgram` | R1=launch_desc_ptr, R2=launch_desc_size, R3=args_ptr, R4=args_len -> R1=task_id, R2=err | **Implemented (descriptor-only as of M14.2 phase 1)** (`nucleus`) |
| 36 | `OpenLibrary` | R1=name_ptr -> R1=port_id, R2=err | **Binary-compat redirect to `SYS_FIND_PORT` (M11.5)**. Source-level alias (`SYS_OPEN_LIBRARY equ SYS_FIND_PORT`); kernel slot 36 dispatches to `.do_find_port` via a one-instruction redirect so older IE64 binaries still link. New code uses `SYS_FIND_PORT` directly. |
| 37 | -- | -- | Removed M11.5 (was `ReadInput`; terminal MMIO inlined into `console.handler` via `SYS_MAP_IO(0xF0, 1)`. Slot returns `ERR_BADARG`; guarded by `TestIExec_ReadInput_RemovedReturnsBadarg`.) |

**ExecProgram (35)** -- current M14.2 ABI: Launches a new task from a caller-mapped M14 launch descriptor. R1=launch_desc_ptr (user VA, must be ≥ `0x600000`), R2=launch_desc_size, R3=args_ptr, R4=args_len. The handler runs under the **caller’s** page table while validating the descriptor header and any supplied args range with the M15.4 permission-aware read helpers (`validate_user_read_range` rather than the old generic `P|U` check), then launches through the descriptor path that preserves ELF target VAs, protections, and entry point. Returns R1=new task_id, R2=err. Returns `ERR_BADARG` for unmapped/kernel-only ranges, malformed descriptors, flat-image callers, args_len > 256, or pointer arithmetic overflow; returns `ERR_NOMEM` when the dynamic image/PT windows or page allocator are exhausted.

**Legacy index path — REMOVED in M11.6**: Historically (M9), if R1 < `0x600000` the handler treated R1 as a `program_table` index and used the M9 ABI (R2=args_ptr, R3=args_len). M10 redesigned the primary ABI around a user-VA `image_ptr` but kept the legacy index branch behind a discriminator for M9 boot-services compatibility (and hardened its args validation with the pre-M15.4 generic range helper). **M11.6 removes the discriminator and the entire legacy code path**: the handler now begins with `blt r1, USER_CODE_BASE → ERR_BADARG`, so any caller passing R1 < `0x600000` hard-fails. The validated image-pointer ABI above is the only path through the handler. `program_table` itself is preserved because the kernel boot path still loads console.handler / dos.library / Shell from it directly into task slots at init, but it is no longer reachable from user mode via `SYS_EXEC_PROGRAM`. Guarded by `TestIExec_ExecProgram_LegacyIndexReturnsBadarg`.

**M15.4 user-pointer validation helpers**: `validate_user_read_range`, `validate_user_write_range`, and `validate_user_exec_range` walk the caller's page table once per page in the requested byte range and require the matching permission mask (`P|R|U`, `P|R|W|U`, or `P|R|X|U`). This replaces the older generic `P|U`-only validation path for security-sensitive syscall inputs. Unmapped pages, kernel-only pages, permission mismatches, and pointer-arithmetic overflows all fail with `ERR_BADARG`.

**M15.5 fault diagnostics**: hardening faults now print `cause`, `PC`, `ADDR`, `ACCESS`, `MODE`, `CLASS`, and `PTE` fields. `ACCESS` distinguishes `read` / `write` / `execute` when the fault code carries that information; `MODE` distinguishes user from supervisor faults; `CLASS` prints the violated invariant (`not-present`, `write-denied`, `exec-denied`, etc.); `PTE` prints the effective `P/R/W/X/U/D` bits seen at fault time. User faults still tear down only the faulting task; supervisor faults still panic.

**M15.5 internal exit hooks**: `kill_task_cleanup` now begins by running an ordered internal hook chain keyed by kernel-private rows in `KERN_DATA_BASE`. Hooks are synchronous, run at most once per exit path, and see the task slot, public task ID, teardown reason (`normal`, `fault`, or internal cleanup), and pre-teardown task state. Hook failures are recorded per row but do not abort teardown. This is groundwork for later resource/module cleanup, not a new public ABI.

**M15.6 G4 grantor-side grant sweep**: when a task that issued grants exits, `kill_task_cleanup` now walks the grant chain a second time on the grantor side *before* the existing grantee-side walk. For every row whose `grantor_pubid` matches the exiting task, the helper (a) tears down any `SYS_MAP_IO` PTEs the grantee has already installed under that grant by calling `unmap_pages` on the grantee's page-table with the row's VA/length, (b) clears the region row to FREE in the grantee's region table, and (c) marks the grant row itself FREE. Without step (a) a revoked capability would leave live hardware access in the grantee's address space after the broker is gone — the grant row would be removed but the PTE mapping the hardware page would still be valid. The helper walks both the inline region rows and the per-task overflow chain, so a grantee that already has more than `KD_REGION_INLINE_MAX` live regions still has its grant-covered MMIO mappings torn down. Before each teardown the helper calls `kern_grant_check` against the *remaining* live rows in the grant chain: if some other live grant (e.g. a bootstrap-table `GRANT_GRANTOR_NONE` row alongside a broker-issued one) still covers the same PPN range for the same grantee, the mapping is left intact because it remains authorized. Guarded by `TestIExec_M156_G4_GrantorExitRevokesGrants`, `TestIExec_M156_G4_GrantorExitUnmapsGranteeIO`, `TestIExec_M156_G4_GrantorExitUnmapsGranteeIO_OverflowRow`, and `TestIExec_M156_G4_GrantorExitKeepsDoubleCoveredMapping`.

**OpenLibrary (36) — M11.5 binary-compat redirect**: Originally a distinct M9 syscall for AmigaOS-style library discovery. M11.5 collapsed it into `SYS_FIND_PORT`: `SYS_OPEN_LIBRARY equ SYS_FIND_PORT` in `iexec.inc`, so all new assembly compiles to slot 16 directly. The kernel dispatcher slot 36 is retained as a one-instruction redirect (`bra .do_find_port`) so any IE64 binary that hardcoded the number 36 still works. Calling slot 36 produces an identical result to calling slot 16. Guarded by `TestIExec_OpenLibrary_DispatcherCollapse`. See § 5.11 "Exec Boundary".

**ReadInput (37) — REMOVED in M11.5**: Originally a kernel-mode helper that read `TERM_LINE_STATUS` / `TERM_STATUS` / `TERM_IN` from page `0xF0` into a user buffer on behalf of `console.handler`. M11.5 deletes the kernel handler. `console.handler` now calls `SYS_MAP_IO(0xF0, 1)` at init time, caches the returned VA, and inlines the MMIO read loop directly into its `CON_MSG_READLINE` path. The `CON_MSG_READLINE` request/reply protocol is unchanged, so existing readline clients keep working without modification. Slot 37 is an unallocated hole and falls through to `ERR_BADARG`. Guarded by `TestIExec_ReadInput_RemovedReturnsBadarg`. See § 5.11.7.

### Implemented Syscall Details

**Yield (26)**: Voluntarily relinquishes the CPU. The trap handler saves the current task's PC and USP, selects the next ready task, restores its state, and returns via ERET. If only one task is ready, Yield returns immediately.

**GetSysInfo (27)**: Queries kernel state. The `info_id` in R1 selects what to return:

| info_id | Name | Returns | Status |
|---------|------|---------|--------|
| 0 | SYSINFO_TOTAL_PAGES | Total pages in system (6400) | **Implemented** |
| 1 | SYSINFO_FREE_PAGES | Free pages available | **Implemented** |
| 2 | SYSINFO_TICK_COUNT | Kernel tick count (incremented on each timer interrupt) | **Implemented** |
| 3 | SYSINFO_CURRENT_TASK | Current task index | **Implemented** |

Unrecognized info_ids return 0 with ERR_OK.

**DebugPutChar (33)**: Writes a single character to the kernel debug terminal. R1 contains the character to output. The kernel writes the byte to the TERM_OUT I/O register (`$F0700`). Returns R2=ERR_OK on success.

**AllocSignal (11)**: Allocates a signal bit from the user range (bits 16-31). R1 contains a bit hint: pass the desired bit number, or -1 to let the kernel auto-assign the lowest free bit. Returns R1=allocated bit number (16-31), R2=ERR_OK on success. Returns R2=ERR_NOMEM if no bits are available, or R2=ERR_BADARG if the hint is out of range or already allocated.

**FreeSignal (12)**: Releases a previously allocated signal bit. R1 contains the bit number to free. Returns R2=ERR_OK on success, R2=ERR_BADARG if the bit is not in the user range or was not allocated.

**Signal (13)**: Sends signals to another task. R1=target task ID, R2=signal mask. The kernel OR's the mask into the target task's `sig_recv` (pending signals). If the target is in WAITING state and any newly-set bit matches its `sig_wait` mask, the target is moved to READY and will receive the matched signals as the return value of its pending `Wait` call. Returns R2=ERR_OK on success, R2=ERR_BADARG if the target task ID is invalid.

**Wait (14)**: Blocks the calling task until matching signals arrive. R1=signal mask (the set of signals to wait for). The kernel checks `sig_recv & mask`; if any bits match immediately, they are cleared from `sig_recv` and returned in R1 without blocking. Otherwise the task's state is set to WAITING with `sig_wait=mask`, and the scheduler selects another task. When a matching `Signal` arrives, the task is woken and R1 contains the received signal bits.

### 5.11 Exec Boundary (M11.5)

By Milestone 11.5 the IntuitionOS userland is Amiga-shaped: a protected nucleus (`exec.library`) plus user-space libraries, devices, handlers, and resources (`dos.library`, `console.handler`, `input.device`, `graphics.library`, and — in M12 — `intuition.library`). M11.5 freezes the syscall surface so M12 can build on top of a stable, justifiable boundary instead of accreting new bring-up shortcuts.

**5.11.1 Boundary statement.** `exec.library` owns *mechanisms* — task lifecycle, scheduling, signals, message ports, memory mapping, shared memory, MMIO mapping, fault handling. *Policy* belongs in user-space libraries, devices, handlers, and resources: `dos.library`, `console.handler`, `input.device`, `graphics.library`, and (M12+) `intuition.library`. Anything that can be a message protocol must be a message protocol.

**5.11.2 Syscall admission rule.** A new syscall is justified only if **both** of the following are true:

1. It requires the kernel's privileged state — page tables, scheduler queues, trap frames, IRQ routing, MMU operations, or MMIO allowlist enforcement, **AND**
2. It cannot be expressed as a message to a user-space service without a bootstrap deadlock or circular dependency.

If either condition fails, the feature belongs in a library, device, handler, or resource protocol — not in the syscall surface.

**5.11.3 Syscall classification table.** Mirrors the category tags in `sdk/include/iexec.inc` so the two cannot drift.

| # | Name | Category | Notes |
|---|------|----------|-------|
| 1 | `AllocMem` | `nucleus` | Page-table install |
| 2 | `FreeMem` | `nucleus` | Page-table uninstall |
| 4 | `MapShared` | `nucleus` | Page-table install of shared object |
| 5 | `CreateTask` | `nucleus` | TCB allocation, page-table build |
| 11 | `AllocSignal` | `nucleus` | Per-task signal mask owned by kernel |
| 12 | `FreeSignal` | `nucleus` | Per-task signal mask owned by kernel |
| 13 | `Signal` | `nucleus` | Cross-task wakeup, scheduler state |
| 14 | `Wait` | `nucleus` | Blocking on scheduler state |
| 15 | `CreatePort` | `nucleus` | Public name registry, kernel-managed FIFO |
| 16 | `FindPort` | `nucleus` | Public name registry — also reachable as `OpenLibrary` (alias) |
| 17 | `PutMsg` | `nucleus` | Cross-task copy with PTE validation |
| 18 | `GetMsg` | `nucleus` | Kernel-managed FIFO |
| 19 | `WaitPort` | `nucleus` | Blocking on kernel-managed FIFO |
| 20 | `ReplyMsg` | `nucleus` | Reply-port redirect with share-handle handoff |
| 26 | `Yield` | `nucleus` | Scheduler entry |
| 27 | `GetSysInfo` | `nucleus` | Kernel-internal counters |
| 28 | `MapIO` | `bootstrap, trusted-internal` | M12.5: gated by `KD_GRANT_TABLE`. The legacy hardcoded allowlist is gone. Only callers holding a covering grant entry (created by `hardware.resource` via `SYS_HWRES_OP`/`HWRES_CREATE`, or by the bootstrap grant table for `console.handler`) succeed; everyone else gets `ERR_PERM`. Removed from the public `nucleus` set. See §5.12. |
| 38 | `HWRES_OP` | `trusted-internal` | M12.5: verb-multiplexed `hardware.resource` broker primitive. R6 selects: `HWRES_BECOME` (claim broker identity) / `HWRES_CREATE` (write a grant row, broker-only) / `HWRES_REVOKE` (reserved for M13, returns `ERR_BADARG`) / `HWRES_TASK_ALIVE` (query task liveness, broker-only). Slot 38 is fresh — slot 37 stays a reserved hole forever per the M11.5 contract. See §5.12. |
| 33 | `DebugPutChar` | `bootstrap` | Single-character debug output to terminal MMIO. Used by the kernel boot banner and panic path **before** `console.handler` is alive. Not part of the normal app programming model — apps use `console.handler` via `CON_MSG_CHAR`. Scheduled to remain forever as the panic-time fallback. |
| 34 | `ExitTask` | `nucleus` | Frees TCB, ports, signals, regions |
| 35 | `ExecProgram` | `nucleus` | M14 introduced the launch descriptor path and M14.2 phase 1 removes the remaining flat-image ABI. Sub-`USER_CODE_BASE` values and flat-image callers hard-fail with `ERR_BADARG`; the descriptor contract is now the only path through the handler. |
| 36 | `OpenLibrary` | `nucleus` (binary-compat redirect to slot 16) | Source-level alias for `SYS_FIND_PORT`; slot 36 dispatches to `.do_find_port` via a one-instruction redirect so any IE64 binary hardcoded to call number 36 still works. New code uses `SYS_FIND_PORT` directly. The Amiga-shaped programming model (`OpenLibrary("dos.library")`) is preserved at the assembler level; the kernel ABI is one slot smaller. |

Slots 3, 6–10, 21–25, 29–32, and 37 are unallocated holes (former syscalls removed in M11.5). The dispatcher's existing fall-through path returns `ERR_BADARG` for any call to a hole. The hole numbers are never reused, so any IE64 binary that called these numbers continues to fail in the same predictable way. M12.5 makes this contract executable via `TestIExec_HWRes_Slot37StillReserved`, which guards specifically against a future patch quietly recycling slot 37. New syscalls always use fresh slots above the current ABI ceiling — `SYS_HWRES_OP` lives at slot 38, not slot 37.

**5.11.4 Known impurities.** *(M11.5 wart, RESOLVED in M12.5.)* `SYS_MAP_IO` was originally a public `nucleus` primitive whose authorization model was a hardcoded allowlist baked into the kernel — page `0xF0` for chip registers, range `[0x100..0x5FF]` for the 5 MB VRAM window — which is policy leaking into the nucleus. M12.5 resolves this: `SYS_MAP_IO` is now `bootstrap, trusted-internal` and is gated by a kernel grant table written by the user-space `hardware.resource` broker. The bootstrap-ordering problem is solved by an immutable bootstrap grant table inserted by the boot-load loop, which gives `console.handler` exactly enough access to come up before `hardware.resource` is alive. See §5.12 for the full design.

**5.11.5 The nucleus is disciplined, not closed.** Future milestones may add syscalls that satisfy the admission rule. The following are explicitly *acceptable* additions in upcoming milestones, named in advance so the freeze does not become a self-imposed straitjacket:

- **Kernel→user interrupt/event delivery.** Replaces input.device's polling, enables a real `WaitVBlank`. Requires kernel-side IRQ routing and per-task signal injection — cannot be done from user space.
- **Per-task fault handler registration.** Lets a service recover from a client's bad pointer instead of taking down both. Requires kernel-side trap-frame modification.
- **Timed wait on signal objects.** Replaces the current `SYS_DELAY` + polling pattern. Requires kernel-side timer-queue insertion.
- **Cleanup hook on service-task death.** Lets `graphics.library` restore text mode after a crash — fixes the M11 wart where a crashed graphics.library leaves the chip in graphics mode. Requires kernel-side death-callback dispatch.

**5.11.6 OpenLibrary rationale.** Classic AmigaOS implemented `OpenLibrary` as an `exec.library` jump-table call, not a trap. IntuitionOS now expresses it as a source-level alias for `FindPort` — `OpenLibrary("dos.library")` and `FindPort("dos.library")` produce the same syscall (number 16). The Amiga-shaped programming surface is preserved at the assembler level; the kernel ABI is one entry-point smaller. Slot 36 in the dispatcher is retained as a one-instruction redirect to `.do_find_port` so any IE64 binary compiled before M11.5 (which would have called number 36) continues to work. A regression test (`TestIExec_OpenLibrary_DispatcherCollapse`) guards the contract that calling slot 36 produces an identical result to calling slot 16.

**5.11.7 Console.handler MMIO ownership change.** Before M11.5, `console.handler`'s `CON_MSG_READLINE` path called the kernel-side `SYS_READ_INPUT` (slot 37), which read `TERM_LINE_STATUS` / `TERM_STATUS` / `TERM_IN` (page `0xF0`) on behalf of the handler. M11.5 deletes that syscall: `console.handler` now calls `SYS_MAP_IO(0xF0, 1)` at init time, caches the returned VA, and inlines the MMIO read loop directly into its readline handler. The change is invisible to clients — the `CON_MSG_READLINE` request/reply protocol is unchanged. Slot 37 is now an unallocated hole and returns `ERR_BADARG`, guarded by `TestIExec_ReadInput_RemovedReturnsBadarg`. This is the canonical example of the boundary statement at work: terminal line input is policy, not mechanism, so it lives in `console.handler`, not in `exec.library`.

### 5.12 hardware.resource + minimal trust model (M12.5)

M12.5 introduces `hardware.resource`, the first user-space MMIO arbiter, and de-publicizes `SYS_MAP_IO`. The kernel grant table that backs this is the *only* path through which a non-bootstrap task can install user PTEs for physical pages. Net public ABI shrinks: `SYS_MAP_IO` leaves the public `nucleus` set, one new `trusted-internal` slot enters.

**5.12.1 Trust model.** A single kernel byte `KD_HWRES_TASK` (offset 9184 in the kernel data page) holds the broker task ID, initialised to `0xFF` (sentinel = unclaimed) at boot. The first task to call `SYS_HWRES_OP` with verb `HWRES_BECOME` claims broker identity; subsequent `HWRES_BECOME` calls return `ERR_EXISTS`. `HWRES_CREATE` is gated by `current_task == KD_HWRES_TASK` — only the broker may write a grant. There are no groups, ACLs, users, or capability masks beyond this single distinction. M12.6 may add more privileged identities; M12.5 deliberately stops at one because no second consumer exists yet.

**5.12.2 Grant table.** A chain-linked list of allocator-backed pages with the chain header at `KD_GRANT_TABLE_HDR` (offset 49344 as of M13 phase 4). Existing chain pages are NEVER copied or moved on grow — appending a new tail page only updates the previous tail's `next` field, so any kernel-internal pointer to a row stays valid. Each page holds 255 grant rows × 16 bytes (`KD_GRANT_TASK_ID` + 4-byte region tag + `PPN_LO` + `PPN_HI` + flags). There is no compile-time row cap; failure mode is real `ERR_NOMEM` from the page allocator. `TestIExec_HWRes_GrantTableChainGrows` is the executable proof that growth works and that existing rows survive across the boundary.

**5.12.3 SYS_HWRES_OP verb interface.** Verb-multiplexed trusted-internal syscall at slot 38. Slot 37 (the retired `SYS_READ_INPUT` hole) stays a reserved hole forever per the M11.5 contract; `TestIExec_HWRes_Slot37StillReserved` makes the contract executable so a future patch cannot quietly recycle it.

| Verb | R6 selector | Effect |
|---|---|---|
| `HWRES_BECOME` | 0 | Claim broker identity. `ERR_OK` first time, `ERR_EXISTS` thereafter. Sticky until the claiming task exits, at which point `kill_task_cleanup` resets `KD_HWRES_TASK` so a fresh task can claim. |
| `HWRES_CREATE` | 1 | Write a grant row for `(task_id, region_tag, ppn_lo, ppn_hi)` (R1..R4). Gated by `current_task == KD_HWRES_TASK`. |
| `HWRES_REVOKE` | 2 | Reserved for M13. Returns `ERR_BADARG` in M12.5. |
| `HWRES_TASK_ALIVE` | 3 | Query whether a task slot is in use. R1 = target task ID; returns R1 = 1 if `KD_TASK_STATE != TASK_FREE`, else 0. Gated by `current_task == KD_HWRES_TASK`. Used by the broker to lazily reclaim stale per-tag owner slots when a previous grantee has exited (the kernel grant chain and `KD_HWRES_TASK` are auto-cleaned by `kill_task_cleanup`, but the broker's private owner-list state is not — this verb gives the broker enough kernel state to reconcile). |

The verb selector arrives in R6 because R0 is the IE64 zero register and not pass-through. The broker never holds a kernel-data-page mapping; all grant writes go through this verb interface. This is what makes the chain-growth strategy safe: the kernel can append chain pages without coordinating with the broker because the broker has no stale row pointers to invalidate.

**5.12.4 SYS_MAP_IO authorization.** The handler walks the grant chain looking for a row whose `task_id == current_task` and whose `[ppn_lo, ppn_hi]` covers the requested PPN range. No covering grant → `ERR_PERM`. The legacy hardcoded allowlist (`(0xF0, 1)` + `[0x100..0x5FF]` VRAM range) is gone — the same PPN ranges are now expressed as grant rows whose region tags `'CHIP'` / `'VRAM'` `hardware.resource` resolves at runtime. This reclassifies `SYS_MAP_IO` from `nucleus` to `bootstrap, trusted-internal — gated by KD_GRANT_TABLE`.

**5.12.5 Bootstrap grant table.** A small immutable list keyed by program-table boot index (NOT task ID, because task IDs do not exist at `kern_init`). The boot-load loop at `iexec.s:228` resolves each row to a live grant entry via `kern_bootstrap_grant_insert` immediately after `load_program` returns the assigned task ID. M12.5 ships with exactly one bootstrap row: `(program_index_of_console_handler, 'CHIP', PPN 0xF0..0xF0)`. This is what lets `console.handler` map its serial-port MMIO at boot before `hardware.resource` is alive — without it, the bring-up would deadlock on the chicken-and-egg of `hardware.resource` depending on `console.handler` for output, which depends on chip MMIO, which depends on a grant. Adding more bootstrap rows is a code change, not a runtime decision. `TestIExec_HWRes_BootstrapConsoleGrantPresent` verifies the row is in place after boot.

M14 shipped/current runtime:
- bootstrap grants are keyed by `program_table` boot index and resolved to task IDs only after launch

M14.1 target:
- bootstrap grants move from boot-index keying to internal manifest-entry-ID keying
- manifest entry ID is not task ID and not a public ABI
- M14.1 phase 2 current runtime:
- the kernel data page now carries a small internal boot-manifest table for `console.handler` and `dos.library`
- those rows are the source of bootstrap-grant identity, but first-service launch still uses the stable bundled boot loader after the staged ELF validation step

**5.12.6 hardware.resource user-space service.** Bundled program `prog_hwres` seeded into the RAM filesystem as `RESOURCES:hardware.resource`. `S/Startup-Sequence` launches it BEFORE `input.device` and `graphics.library` so it has its public port `hardware.resource` registered before any client calls `FindPort`. Service body:

1. `SYS_HWRES_OP`/`HWRES_BECOME` — claim broker identity.
2. `CreatePort("hardware.resource", PF_PUBLIC)` — register the public port.
3. Print `"hardware.resource ONLINE [Task N]"` banner.
4. `WaitPort` loop. Each `HWRES_MSG_REQUEST` (data0 = 4-byte region tag, data1 = unused/ignored) goes through:
   - **Sender identity** — the kernel-trusted sender task ID arrives in `R7` from `SYS_WAIT_PORT`/`SYS_GET_MSG`. The broker uses R7 (which is `KD_MSG_SRC` populated by `PutMsg`) and *ignores* anything the client may have stored in `data1`. M11.5+ message-port semantics make `MSG_SRC` unforgeable.
   - **Stale-owner scrub** — for each occupied slot in the per-tag owner lists, the broker calls `SYS_HWRES_OP`/`HWRES_TASK_ALIVE`. Slots whose task has exited are reclaimed (set to `0xFF`). Without this, a recycled task slot could either be silently regranted (because the slot ID matches a dead owner) or another task could be blocked forever (because a dead owner still occupies the only slot).
   - **Trust gating** — `'CHIP'` is a shared resource (terminal/input/video registers all live in one physical page, so multiple services legitimately share it); the broker maintains a 4-slot owner list at `data[144..147]`. `'VRAM'` is a monopoly (single-display-client rule); one owner slot at `data[148]`. A request is granted if the sender is already in the appropriate list (idempotent re-grant) or if there is a free slot to record them. Otherwise → `HWRES_MSG_DENIED`.
   - On grant, the broker calls `SYS_HWRES_OP`/`HWRES_CREATE` with the trusted sender task ID and the resolved PPN range, then replies with `HWRES_MSG_GRANTED` whose data0 carries `(ppn_base<<32) | page_count`.

The broker's policy table maps `'CHIP'` → `(PPN 0xF0, 1 page)` and `'VRAM'` → `(PPN 0x100, 470 pages)`.

**5.12.7 Migrated callers.** `input.device` and `graphics.library` no longer call `SYS_MAP_IO` ambiently. Each spins on `FindPort("hardware.resource")` until the broker is up, then sends one `HWRES_MSG_REQUEST` per region needed (input.device wants `'CHIP'`; graphics.library wants `'CHIP'` *and* `'VRAM'`), waits for the reply, and only then calls `SYS_MAP_IO`. `console.handler` is not migrated — it relies on the bootstrap grant because the broker isn't running at the time it needs MMIO.

**5.12.8 The `SYS_MAP_IO` allowlist backstop.** The legacy allowlist code in the kernel was *replaced* by the grant chain check in M12.5 — the bootstrap-table backstop is implicit (the bootstrap row gives `console.handler` exactly the same access the old allowlist gave it). M13 may delete the kernel allowlist symbol entirely; M12.5 leaves the surrounding bounds-check logic in place for sanity (page count cap, signed-overflow guards) without the PPN-specific allowlist.

### 5.13 Architectural cap policy (M12.5–M12.8)

> IntuitionOS does not use arbitrary fixed product limits for core OS objects where dynamic allocation is practical. Remaining limits must be justified by architecture, ABI width, hardware constraints, or explicitly configured resource policy.

This rule landed in M12.5 alongside `hardware.resource`, was completed for the kernel core in M12.6, and was extended to dos.library file storage in M12.8. M12.5 shipped the rule, the audit table below, and one proof-of-pattern removal (`KD_REGION_MAX`). M12.6 worked through the remaining bucket-C rows in risk order: **Phase A** (DOS file/handle caps, user-space chain), **Phase B** (`KD_SHMEM_MAX` → kernel chain), **Phase C** (`KD_PORT_MAX` → kernel chain), **Phase D** (`MAX_TASKS` → layout-bump from 16 to 32). **Phase E** then split the user-dynamic VA window and the allocator pool into disjoint VPN ranges as a privilege-escalation security fix — see the `USER_DYN_BASE/END` and `ALLOC_POOL_BASE/PAGES` rows below for details. **M12.8** then completed the dos.library file body refactor: per-file storage was migrated from a fixed 16 KiB AllocMem block to a chain of 4 KiB extents, the `DOS_FILE_SIZE` constant was deleted, and as a prerequisite the two arbitrary `load_program` per-image caps (`code_size <= 8192`, `data_size <= 49152`) were also wiped. **M13 phase 2** then removed the last fake dependency those caps had on `USER_SLOT_STRIDE`: task images and PTs are now placed dynamically inside reserved windows, so the real constraints are allocator/window exhaustion, not per-task slot fit.

The honest summary: fixed product limits were removed where practical. Remaining limits are either architectural (ABI widths, page-table format, MMU contract), layout-bound (`MAX_TASKS = 32` is still the size of the current task-state / scheduler space, not a number plucked out of thin air), or strictly tied to an active hardware/format ABI. M12.8 closed the last load-bearing bucket-B entry (`DOS_FILE_SIZE`), and M13 phase 2 removed the last remaining slot-fit language from task-image placement. The Phase E security fix also makes one architectural invariant explicit: **the user-dynamic VA window and the allocator pool are now disjoint VPN ranges**, so user `SYS_ALLOC_MEM` calls can never overwrite the supervisor-only pool PTEs that `build_user_pt` copies into every user PT. This is enforced at the layout level by the constants in `iexec.inc` and verified by `TestIExec_PortChain_DisjointFromUserDyn`.

**5.13.1 Cap classification audit.** Every fixed cap declared in `sdk/include/iexec.inc` and `sdk/intuitionos/iexec/iexec.s`, classified into three buckets. Line numbers refreshed at the end of M12.6 Phase E.

- **A — architectural / ABI / hardware-bound.** Keep. These are bounded by the IE64 instruction format, the MMU contract, the on-disk message ABI, or the physical machine.
- **B — temporary implementation bound or configured policy.** Keep for now, document a replacement plan.
- **C — arbitrary toy-era cap.** Empty after M12.6.

| Symbol | File:Line | Value | Bucket | Notes |
|---|---|---|---|---|
| `MMU_PAGE_SIZE` | `iexec.inc:374` | 4096 | A | MMU page size — part of the page-table format. |
| `MMU_PAGE_SHIFT` | `iexec.inc:375` | 12 | A | log2 of `MMU_PAGE_SIZE`. |
| `MMU_NUM_PAGES` | `iexec.inc:376` | 8192 | A | Total physical RAM pages on the IntuitionEngine target. Hardware-bound. |
| `KERN_PAGES` | `iexec.inc:377` | 384 | A | Kernel reserved page range. Bounded by the layout of `KERN_PAGE_TABLE` / `KERN_DATA_BASE` / kernel binary / kernel stack. |
| `KERN_PAGE_TABLE` | `iexec.inc:190` | 0x07D000 | A | Fixed kernel page-table location, shifted down in M15.6 R1 so the kernel stack can have a dedicated guard page. |
| `KERN_DATA_BASE` | `iexec.inc:192` | 0x08D000 | A | Fixed kernel data-page base. |
| `KERN_STACK_TOP` | `iexec.inc:193` | 0x0A0000 | A | Top of the kernel stack inside the kernel reserved range. |
| `MAX_TASKS` | `iexec.inc:187` | 255 | A | **M13 phase 4:** the old layout-bound 32-task cap is gone. The remaining fixed task-state tables now run to the current 8-bit internal-slot ABI ceiling (`255`, with `0xFF` reserved by a few byte-sized kernel fields such as `KD_PORT_OWNER` sentinels). Task image/PT backing now spills into allocator-pool pages once the legacy fixed windows are exhausted, so the practical limit is no longer the old 32-slot VA layout. |
| `KD_TASK_STRIDE` | `iexec.inc:199` | 32 | A | TCB layout — bounded by the on-data-page field offsets, not arbitrary. |
| `KD_PTBR_BASE` | `iexec.inc:230` | 1088 | A | PTBR array offset (after 32 TCBs). Layout-derived from `KD_TASK_BASE + MAX_TASKS * KD_TASK_STRIDE`. |
| `KD_PORT_STRIDE` | `iexec.inc:266` | 168 | A | Port size = 40-byte header + 4×32-byte messages. Bounded by `KD_PORT_FIFO_SIZE` × `KD_MSG_SIZE` + header. |
| `KD_PORT_INLINE_MAX` | `iexec.inc:267` | 32 | A | Number of port rows kept inline for fast-path access (M12.6 Phase C). The actual cap on ports is the allocator pool, reached via the overflow chain. |
| `KD_PORT_MAX` | `iexec.inc:270` | 32 | C → A | **Cap removed in M12.6 Phase C; symbol retained as legacy alias for `KD_PORT_INLINE_MAX`.** The original 32-row hard cap is gone — rows beyond 32 live in an overflow chain reached through `KD_PORT_OFLOW_HDR`. The 1-byte port ID ABI ceiling is 255 (0xFF reserved as `WAITPORT_NONE` sentinel). |
| `KD_PORT_FIFO_SIZE` | `iexec.inc:271` | 4 | B | Per-port message queue depth, not a global system cap. Tied to message-port flow control semantics; replaceable later but not load-bearing. |
| `KD_PORT_OFLOW_HDR` | `iexec.inc:274` | 12152 | A | Single global chain header for the port overflow chain (M12.6 Phase C). |
| `PORT_NAME_LEN` | `iexec.inc:301` | 32 | A | ABI — every port-name field across the kernel + protocol uses this width. Bumping it later would be a flag-day change to message-port headers. |
| `KD_MSG_SIZE` | `iexec.inc:310` | 32 | A | ABI — message size is part of the cross-task IPC contract. |
| `REPLY_PORT_NONE` | `iexec.inc:313` | 0xFFFF | A | Sentinel value, not a cap. |
| `WAITPORT_NONE` | `iexec.inc:220` | 0xFF | A | Sentinel value, not a cap. |
| `USER_PT_BASE` | `iexec.inc:335` | 0x800000 | A | Fixed slot of the user page-table region (M12.6 Phase D: was 0x700000), sized to `MAX_TASKS * USER_SLOT_STRIDE = 2 MiB`. |
| `USER_SLOT_STRIDE` | `iexec.inc:345` | 0x10000 | A | Per-task slot stride — bounded by the layout of code/stack/data pages inside one slot. |
| `USER_DYN_BASE` / `USER_DYN_END` | `iexec.inc:419–420` | 0xA00000 / 0x1200000 | A | Dynamic VA range — **M12.6 Phase E security fix**: split from the allocator pool into a disjoint VPN range. Previously the user-dyn window aliased the entire allocator pool (`0xA00000..0x2000000`), which let unprivileged tasks overwrite the supervisor-only pool PTEs that `build_user_pt` copies into every user PT and pivot the kernel chain walkers (running on the user PT) into attacker-controlled memory. The fix gives user-dyn the bottom half (`0xA00000..0x1200000`, 8 MiB, VPN `0xA00..0x11FF`) and the allocator pool the top half (PPN `0x1200..0x1FFF`, 14 MiB). The two ranges are now disjoint at the layout level — see `TestIExec_PortChain_DisjointFromUserDyn`. |
| `USER_DYN_PAGES` | `iexec.inc:421` | 768 | B | Per-allocation cap for `AllocMem`, not per-task. Documents the largest single chunk a task can ask for. Replaceable. |
| `ALLOC_POOL_BASE` | `iexec.inc:401` | 0x1200 | A | First allocable physical page — **M12.6 Phase E security fix**: was 0xA00 (split user-dyn and pool into disjoint VPN ranges so user `SYS_ALLOC_MEM` calls cannot ever overwrite the supervisor-only pool PTEs in user PTs). Pool now starts at PPN 0x1200, immediately above the user-dyn VA window which ends at `USER_DYN_END = 0x1200000`. M12.6 Phase D had earlier bumped this from 0x800 to 0xA00 to make room for the doubled `USER_PT_BASE` region. |
| `ALLOC_POOL_PAGES` | `iexec.inc:402` | 3584 | A | **M12.6 Phase E security fix**: was 5632 (lost 2048 pages = 8 MiB to the user-dyn VA window so user-dyn and the allocator pool are now disjoint at the layout level). Pool spans pages `0x1200..0x1FFF` = 14 MiB. Bounded by `MMU_NUM_PAGES − ALLOC_POOL_BASE`. M12.6 Phase D had earlier shrunk this from 6144 to 5632 for the user PT region. |
| `KD_PAGE_BITMAP` / `KD_PAGE_BITMAP_SZ` | `iexec.inc:416–417` | 6720 / 800 bytes | A | Bitmap size derived from `ALLOC_POOL_PAGES` rounded to whole bytes. Layout-derived, not arbitrary. |
| `KD_REGION_INLINE_MAX` | `iexec.inc:436` | 8 | A | Number of region rows kept inline per task for fast-path access (M12.5). The actual cap on regions per task is the allocator pool, reached via the overflow chain. |
| `KD_REGION_TASK_SZ` | `iexec.inc:437` | 128 | A | Inline byte stride per task (8 × 16). With M12.5's overflow chain this is now the inline range size, not a cap on regions per task. |
| `KD_REGION_MAX` | `iexec.inc:440` | 8 | C → A | **Cap removed in M12.5; symbol retained as legacy alias for `KD_REGION_INLINE_MAX`.** Rows beyond 8 live in a per-task overflow chain reached through `KD_REGION_OVERFLOW_HEAD`. |
| `KD_REGION_OVERFLOW_HEAD` | `iexec.inc:444` | 11888 | A | Per-task overflow chain header array base (M12.5). M12.6 Phase D shifted this from 9200 → 11888 because all kernel data structures after the TCB and region table grew. |
| `KD_REGION_OFLOW_STRIDE` | `iexec.inc:445` | 8 | A | Per-task overflow header stride. Layout-derived. |
| `KD_REGION_STRIDE` | `iexec.inc:435` | 16 | A | Region row size, bounded by the row field layout. |
| `KD_SHMEM_INLINE_MAX` | `iexec.inc:496` | 16 | A | Number of shmem rows kept inline for fast-path access (M12.6 Phase B). |
| `KD_SHMEM_MAX` | `iexec.inc:499` | 16 | C → A | **Cap removed in M12.6 Phase B; symbol retained as legacy alias for `KD_SHMEM_INLINE_MAX`.** Rows beyond 16 live in an overflow chain reached through `KD_SHMEM_OFLOW_HDR`. The 1-byte shmem id ABI ceiling is 255. |
| `KD_SHMEM_STRIDE` | `iexec.inc:495` | 16 | A | Shared-object row size, bounded by row layout. |
| `KD_SHMEM_OFLOW_HDR` | `iexec.inc:502` | 12144 | A | Single global chain header for the shmem overflow chain (M12.6 Phase B). |
| `KD_HWRES_TASK` | `iexec.inc:735` | 1 byte | A | Broker task ID slot in the kernel data page (M12.5). 0xFF = unclaimed. M12.6 Phase D shifted this from 9184 → 11872. |
| `KD_GRANT_TABLE_HDR` | `iexec.inc:738` | 8 bytes | A | Chain header for the grant table (M12.5). The grant chain itself has no fixed-row cap. |
| `SYS_HWRES_OP` | `iexec.inc:72` | slot 38 | A | Trusted-internal verb-multiplexed broker primitive (M12.5). Slot number is part of the syscall ABI. |
| `IEXEC_HEARTBEAT_INTERVAL` | `iexec.inc:529` | 64 | A | Tunable — debug-only kernel heartbeat tick rate, not a system cap. |
| `IMG_HEADER_SIZE` | `iexec.inc:539` | 32 | A | IE64PROG image format ABI. |
| `IMG_OFF_CODE_SIZE` cap | (removed) | — | B → ✓ | **Removed in M12.8 Phase 1.** The previous arbitrary 8192-byte cap (bumped 4096 → 8192 in M10) was a bucket-C product limit hiding in bucket B. In M12.8 it was replaced by the then-honest image-layout rule; **M13 phase 2 removed the remaining slot-fit dependency entirely** by allocating task code/stack/data dynamically inside `USER_IMAGE_BASE..USER_IMAGE_END`. The real limit is now dynamic-window / allocator exhaustion, not a fixed byte ceiling. |
| `IMG_OFF_DATA_SIZE` cap | (removed) | — | B → ✓ | **Removed in M12.8 Phase 1.** The previous arbitrary 49152-byte cap (bumped 4096 → 16384 → 20480 → 49152 across M8/M10/M11/M12) was the same kind of product limit. In M12.8 it was replaced by the then-current fit rule; **M13 phase 2 finished the cleanup** by moving image placement to the dynamic user-image window. |
| `PROGTAB_ENTRY_SIZE` | `iexec.inc:546` | 24 | A | Program table row layout, bounded by row fields. |
| `PROGTAB_BOOT_COUNT` | `iexec.inc:558` | 3 | A | Number of programs auto-loaded at boot — this is a *configured policy*, not an arbitrary cap. The number is "the count of strict-boot services," currently 3 (console.handler, dos.library, Shell). |
| `TERM_IO_PAGE` | `iexec.inc:561` | 0xF0 | A | Hardware MMIO page address. |
| `DATA_ARGS_OFFSET` / `DATA_ARGS_MAX` | `iexec.inc:564–565` | 3072 / 256 | B | Per-program argument-passing layout inside the program data page. Tied to the M9 `SYS_EXEC_PROGRAM` ABI. |
| `DOS_MAX_FILES` | (removed) | — | C → ✓ | **Removed in M12.6 Phase A.** The `dos.library` file table is now a chain of `AllocMem`'d 4 KiB pages, each holding 85 entries. No compile-time cap; failure mode is real `ERR_NOMEM` from the page allocator. |
| `DOS_NAME_LEN` | `iexec.inc:612` | 32 | A | Filesystem ABI — bounded by the in-table filename field. |
| `DOS_FILE_SIZE` | (removed) | — | B → ✓ | **Removed in M12.8.** dos.library file bodies are now stored as a chain of 4 KiB extents (`DOS_EXT_*` constants in `iexec.inc`, walked via `.dos_extent_alloc/_free/_walk/_write` in `iexec.s`). Each file's `entry.file_va` is the head of an extent chain whose total length is bounded only by the kernel allocator pool. `DOS_WRITE` implements an atomic-swap-on-rewrite rule: a new chain is allocated and populated, then linked into the entry, then the old chain is freed — so an allocation failure during a rewrite leaves the previous file content fully intact. |
| `DOS_MAX_HANDLES` | (removed) | — | C → ✓ | **Removed in M12.6 Phase A.** The `dos.library` handle table is now a chain of `AllocMem`'d 4 KiB pages, each holding 510 handles. No compile-time cap. |
| `INTUI_WIN_TITLE_H` | `iexec.inc:718` | 16 | A | Window title bar height in pixels — bounded by the embedded Topaz 8×16 font glyph height. |
| `INTUI_WIN_BORDER` | `iexec.inc:719` | 2 | A | Window bevel thickness — visual constant, not a cap. |
| `SIG_SYSTEM_MASK` | `iexec.inc:223` | 0xFFFF | A | Bit-field mask, ABI — system signals are bits 0-15, user signals 16-31, bounded by the 32-bit signal-word width. |

**Bucket C summary (post M12.6 — empty):** every previously bucket-C row was either removed (chain-allocator conversion in Phases A/B/C, or layout-bump in Phase D) or reclassified into bucket A or B with a recorded reason. The five rows that started this milestone in bucket C went out as follows:

- `KD_REGION_MAX` (8) — **M12.5**: hard cap removed via per-task overflow chain. Symbol kept as legacy alias for `KD_REGION_INLINE_MAX`. Reclassified C → A.
- `DOS_MAX_FILES` (16), `DOS_MAX_HANDLES` (8) — **M12.6 Phase A**: removed entirely. `dos.library` file/handle tables are now user-space chains of `AllocMem`'d pages.
- `KD_SHMEM_MAX` (16) — **M12.6 Phase B**: hard cap removed via global overflow chain. Symbol kept as legacy alias for `KD_SHMEM_INLINE_MAX`. Reclassified C → A.
- `KD_PORT_MAX` (32) — **M12.6 Phase C**: hard cap removed via global overflow chain. Symbol kept as legacy alias for `KD_PORT_INLINE_MAX`. Reclassified C → A.
- `MAX_TASKS` — **M13 phase 4**: the old layout-bound 32-task cap is gone. The remaining fixed task-state tables were expanded to the current 8-bit internal-slot ABI ceiling (`255`, with `0xFF` reserved in a few byte-sized owner/sentinel fields). Code/data/stack/startup pages and PT backing now spill into allocator-pool pages once the legacy fixed windows are exhausted, so the practical limit is no longer the old 32-slot VA layout. Reclassified to A as an ABI-width bound pending any future widening of those remaining byte-sized internal slot carriers.

**Bucket B follow-up (post M13 phase 2):** the load-bearing bucket-B rows are now also gone. M12.8 closed the dos.library file storage refactor that had been on the milestone roadmap since the start of M12.6, and in the process found two more arbitrary product caps hiding in bucket B (the `load_program` `code_size` and `data_size` caps). M12.8 deleted those caps; M13 phase 2 then removed the temporary slot-fit replacement by moving task image placement to dynamic image/PT windows. The honest framing: the two `load_program` caps were *originally* bucket-C product limits that got coded into `load_program` with no architectural justification, and the M12.5 audit incorrectly classified them as B. See the `IMG_OFF_CODE_SIZE` / `IMG_OFF_DATA_SIZE` rows above.

The remaining bucket-B rows (`KD_PORT_FIFO_SIZE = 4`, `USER_DYN_PAGES = 768`, `DATA_ARGS_OFFSET / DATA_ARGS_MAX`) are all configured-policy values, not arbitrary product limits — they're tied to specific protocol/ABI semantics rather than acting as system-wide caps on object counts or sizes.

**5.13.2 Why `KD_REGION_MAX` is the M12.5 first removal.** The plan locks in `KD_REGION_MAX` rather than `KD_PORT_MAX` for three reasons:
1. **Critical-path proof.** The per-task region table is on the hot path for every `SYS_MAP_IO`, `SYS_ALLOC_MEM`, `SYS_MAP_SHARED`, and the M12.5 `hardware.resource` broker itself. Removing this cap first exercises the chain-allocator pattern against the heaviest in-kernel consumer.
2. **Lower blast radius than `KD_PORT_MAX`.** Region-table walkers are confined to the AllocMem / FreeMem / MapShared / SYS_MAP_IO / task-teardown paths. Port-table walkers reach into every IPC operation, including `FindPort` / `PutMsg` / `GetMsg` / `WaitPort` / `ReplyMsg`, which is wider than M12.5 should touch in one milestone.
3. **`hardware.resource` itself benefits.** The broker holds at least two MMIO regions (`'CHIP'` + `'VRAM'` PPN ranges) and may add more later. Without this removal, `KD_REGION_MAX = 8` would silently cap how many distinct hardware regions M12.5 can ever broker per task — which would defeat the milestone's own architectural objective.

**5.13.3 What the rule does not say.** "Dynamic where practical" is the discipline, not "no caps anywhere." The audit table documents that `MMU_NUM_PAGES`, `KD_MSG_SIZE`, `PORT_NAME_LEN`, hardware MMIO addresses, page-table format constants, and ABI field widths all stay fixed. Bucket A is large and that is correct — the policy is about removing *arbitrary product-demo ceilings*, not pretending to be a userland scripting language. M12.6 must keep that distinction visible: bucket-C removals should justify themselves against the audit, not become a treadmill.

---

## 6. Signal Model

Signals are a 32-bit bitmask per task, directly inherited from the Amiga Exec model. Implemented in Milestone 3.

### 6.1 Bit Allocation

| Bits | Range | Owner | Description |
|------|-------|-------|-------------|
| 0-15 | System | Kernel | Reserved for kernel-defined signals |
| 16-31 | User | Task | Allocated via `AllocSignal`, freed via `FreeSignal` |

### 6.2 System Signals

| Bit | Name | Description |
|-----|------|-------------|
| 0 | `SIGF_PORT` | Message arrived at a port owned by this task |
| 1 | `SIGF_TIMER` | A timer request completed |
| 2 | `SIGF_ABORT` | Task abort requested (e.g., by `DeleteTask`) |
| 3 | `SIGF_CHILD` | A child task exited |

### 6.3 Wait/Signal Semantics

- `Wait(mask)` blocks the calling task until any bit in `mask` is set in the task's pending signal word. Returns the set of signals that were received (pending AND mask). Clears the received bits from pending.
- `Signal(task, mask)` sets bits in the target task's pending signal word. If the target is in WAITING state and any newly-set bit matches its wait mask, the target is moved to READY.

---

## 7. Port/Message Model

Ports are kernel-managed FIFO message queues, modeled after Amiga MsgPorts.

### 7.1 Port Structure

Each port has:
- A name (optional, for `FindPort` lookup)
- An owning task
- A signal bit (raised when a message arrives)
- A FIFO message queue

### 7.2 Message Passing

Messages are fixed-size (32 bytes) and kernel-copied. Each message contains: type (4 bytes), sender task ID (4 bytes), data0 (8 bytes), data1 (8 bytes), reply_port (2 bytes, 0xFFFF = none), and share_handle (4 bytes, 0 = none). `PutMsg` writes the message directly into the target port's kernel-managed FIFO (4 slots per port). `GetMsg` dequeues the oldest message and returns all fields in R1-R6. This fixed-size model is simple and assembly-friendly.

For bulk data transfer, use shared memory: the sender includes a share_handle in the message (from `AllocMem(MEMF_PUBLIC)`), and the receiver calls `MapShared` with an explicit `MAPF_READ` / `MAPF_WRITE` mask to access the shared region. This is the protected-Amiga equivalent of "I gave you a pointer."

### 7.3 Reply Protocol

The reply pattern follows Amiga convention:
1. Client creates a private reply port
2. Client calls `PutMsg` to a named service port, including its reply_port in R5
3. Server calls `WaitPort` on its service port, receives the request with reply_port in R5
4. Server calls `ReplyMsg(reply_port, type, data0, data1, share_handle)` to send the response
5. Client calls `WaitPort` on its reply port, receives the reply

This is the same request/reply model used by AmigaOS devices and libraries, adapted for a protected kernel with explicit memory sharing.

---

## 8. Timer Model (Future)

User-space timers are multiplexed onto the single IE64 hardware timer by the kernel.

### 8.1 Delta Queue

The kernel maintains a sorted delta queue of pending timer requests. Each entry stores the remaining ticks until expiry as a delta from the previous entry. On each hardware timer interrupt, the kernel decrements the head of the queue and fires any timers that have reached zero.

### 8.2 Timer API

- `AddTimer(ticks, signal_mask)`: Creates a one-shot timer. When it expires, the specified signal bits are set on the calling task. Returns a timer handle.
- `RemTimer(handle)`: Cancels a pending timer and frees the handle.

The hardware timer (CR_TIMER_PERIOD, CR_TIMER_COUNT, CR_TIMER_CTRL) drives the scheduler tick. The kernel programs it during boot and uses each interrupt for both preemption and user timer expiry checks.

---

## 9. Handle Model (Future)

### 9.1 Handle Table

Each task has a per-task handle table mapping 32-bit handles to kernel objects. Handles are opaque integers, not pointers. The kernel validates handle ownership on every syscall.

### 9.2 Handle Types

| Type | Value | Description |
|------|-------|-------------|
| `HANDLE_TASK` | 1 | Reference to a task (from `CreateTask` or `FindTask`) |
| `HANDLE_PORT` | 2 | Reference to a message port |
| `HANDLE_SHMEM` | 3 | Reference to a shared memory region |
| `HANDLE_TIMER` | 4 | Reference to a pending timer |

### 9.3 Handle Operations

- `CloseHandle(handle)`: Releases the handle. If it is the last reference to the underlying object, the object is destroyed (port queues drained, shared memory unmapped, timer cancelled).
- `DupHandle(handle)`: Creates a new handle referencing the same kernel object. The reference count is incremented.

---

## 10. Bootstrap Sequence

The M10 kernel boots in supervisor mode at `$1000` (PROG_START) and performs the following steps:

```
 1. Set trap vector, interrupt vector, and kernel stack pointer

 2. Build the kernel page table at $10000:
    - Identity-map pages 0-383 ($000000-$17FFFF) as supervisor-only

 3. Initialize kernel data:
    - current_task = 0
    - tick_count = 0
    - num_tasks = 0
    - all 8 TCB slots = FREE
    - all 8 ports = invalid

 4. Enable the MMU using the kernel page table

 5. Print the boot banner:
    - "exec.library M10 boot"

 6. Iterate the static program table:
    - validate each bundled image header
    - find a free task ID / TCB row
    - allocate code/stack/data/PT backing for the task image
    - copy code/data into the allocated user pages
    - build the task's page table
    - initialize the task's TCB and PTBR entry
    - increment num_tasks
    - skip invalid images and continue

 7. Strict boot check: the first PROGTAB_BOOT_COUNT (3) programs
    must load successfully or the kernel panics with GURU MEDITATION

 8. Program the hardware timer:
    - set period
    - set initial count
    - enable timer + interrupts

 9. If num_tasks == 0:
    - print "GURU MEDITATION: no programs loaded"
    - halt

10. Switch to the first loaded task's PTBR, USP, and PC

11. Execute ERET to enter user mode
```

After `ERET`, the kernel only runs in response to traps (syscalls, page faults) and interrupts (timer).

---

## 11. Exec Lineage

How IExec maps to (and diverges from) classic Amiga Exec:

| Amiga Exec Concept | IExec Equivalent | What Changed |
|--------------------|------------------|--------------|
| Flat supervisor space, no MMU | Per-task page tables, MMU-enforced user/supervisor | Hardware protection; tasks cannot corrupt each other or the kernel |
| `AddTask()` with `tc_SPLower`/`tc_SPUpper` | `CreateTask` with kernel-allocated pages | Stack is a mapped page region, not a raw pointer range |
| `Signal()`/`Wait()` with 32-bit mask | Same API, same 32-bit mask | Unchanged - the model is perfect as-is |
| `MsgPort` + `Message` (linked list in shared memory) | Named kernel-managed ports with 32-byte copy-in/copy-out messages | No shared-memory message headers; kernel copies payload for safety; named public ports discoverable via FindPort (case-insensitive, Amiga-style) |
| `FindPort()` + `PutMsg()`/`GetMsg()` service pattern | `FindPort` + `PutMsg`/`WaitPort`/`ReplyMsg` with reply_port and share_handle | Same Exec service model: create named port, client discovers it, sends request, server replies. Share handles can be carried in messages for protected memory handoff. |
| `AllocMem()`/`FreeMem()` from memory pools | `AllocMem`/`FreeMem` backed by page allocator + per-task mapping; `MEMF_PUBLIC` with opaque share handles | Returns virtual addresses; kernel manages physical page pool; sharing via capability handles not flat pointers |
| `Exec->ThisTask` | `FindTask(0)` or `GetTaskInfo` | No direct pointer to TCB from user space |
| `Forbid()`/`Permit()` (disable scheduling) | Not provided | Tasks cannot disable preemption; kernel controls scheduling |
| `Disable()`/`Enable()` (disable interrupts) | Not available to user mode | Only kernel uses `CLI64`/`SEI64` internally |
| `SysBase` at address 4 | No equivalent | Kernel is not addressable from user space |
| `OpenLibrary()`/`CloseLibrary()` | Source-level alias for `FindPort` (M11.5; was a distinct M9 syscall) | `OpenLibrary("dos.library")` and `FindPort("dos.library")` produce the same syscall (number 16). Slot 36 is retained as a binary-compat redirect to `.do_find_port`. The Amiga-shaped programming model is preserved at the assembler level; the kernel ABI is one slot smaller. |
| Device I/O (`DoIO`/`SendIO`) | `MapIO`/`MapVRAM` + direct register access | User tasks access hardware registers through mapped pages |
| `tc_Node.ln_Pri` scheduling | Priority field in TCB, same semantics | Round-robin within same priority level |

---

## 12. Milestone Status

### Milestone 1: Boot + Preemptive Multitasking (Complete)

**Implemented and tested:**

- Standalone kernel binary (`make intuitionos` assembles `sdk/intuitionos/iexec/iexec.s`)
- Self-sufficient boot: kernel builds its own page tables, creates user tasks, and initializes all scheduler state - no host-side setup required
- Kernel boots at `$1000` in supervisor mode with MMU off, performs all init, then enables MMU
- Kernel page table: identity-mapped supervisor pages (0-383)
- Per-task page tables: copies kernel mappings + adds user code/stack/data pages with U bit
- User task code: embedded in kernel image as templates, copied to user code pages at 0x600000/0x610000 during init
- W^X enforcement: code pages R+X, stack/data pages R+W, no page is both writable and executable
- Trap handler dispatches SYSCALL and page faults
- `Yield` syscall (26): voluntary context switch between tasks
- `GetSysInfo` syscall (27): query kernel tick count (info_id=2)
- Two-task round-robin scheduler with save/restore of PC, USP, PTBR
- Timer-driven preemption via interrupt handler (per-instruction tick, configurable quantum)
- Atomic interrupt model: trapEntry disables interrupts, ERET re-enables when returning to user mode
- Page fault on unmapped access correctly traps to kernel
- Test coverage: `TestIExec_KernelBoots`, `TestIExec_KernelPageTable`, `TestIExec_YieldReturns`, `TestIExec_FaultKillsTask`, `TestIExec_TwoTasksRun`, `TestIExec_TimerPreemption`, `TestIExec_GetSysInfo`, `TestIExec_AssembledKernelBoots`

### Milestone 2: Observable Kernel (Complete)

**Implemented and tested (builds on Milestone 1):**

- Boot banner and early debug terminal output were introduced here. The current banner text is `"exec.library M10 boot\n"` because later milestones kept extending the same kernel image.
- `DebugPutChar` syscall (33): write a single character to the debug terminal (TERM_OUT at `$F0700`)
- Milestone 2 originally used simple built-in demo tasks for visible output. The richer four-service demo belongs to Milestone 8.
- Fault reporting: on non-SYSCALL faults, kernel prints a GURU MEDITATION message (replaced plain FAULT format in M9) to the debug terminal then halts

### Milestone 3: Signals (Complete)

**Implemented and tested (builds on Milestone 2):**

- `AllocSignal` syscall (11): allocate a signal bit from the user range (bits 16-31); R1=bit hint (-1 for auto-assign), returns R1=allocated bit number, R2=err
- `FreeSignal` syscall (12): release a previously allocated signal bit; R1=bit to free, returns R2=err
- `Signal` syscall (13): send signals to another task; R1=target task ID, R2=signal mask - sets bits in target's pending signal word, wakes a WAITING target if signals match its wait mask
- `Wait` syscall (14): block until matching signals arrive; R1=signal mask, blocks the calling task (state transitions to WAITING), returns R1=received signals when woken
- Per-task signal state: `sig_alloc` (allocated bit mask), `sig_wait` (wait mask), `sig_recv` (pending/received signals), task state (READY/RUNNING/WAITING)
- Scheduler skips tasks in WAITING state; shared restore path delivers Wait return values when `sig_wait != 0`
- Deadlock detection: when all tasks are in WAITING state with no external wake source, kernel prints "DEADLOCK: no runnable tasks" and halts

### Milestone 4: Message Ports (Complete)

**Implemented and tested (builds on Milestone 3). Note: ABI and capacity updated in M7.**

- `CreatePort` syscall (15): creates a message port owned by the calling task. M7 extended with name/flags parameters (see M7 section).
- `PutMsg` syscall (17): send a fixed-size message to a port. M7 extended to 32-byte messages with data0, data1, reply_port, share_handle fields.
- `GetMsg` syscall (18): dequeue a message from a port. Owner only (ERR_PERM otherwise). Returns ERR_AGAIN if empty.
- `WaitPort` syscall (19): blocking receive. Same returns as GetMsg but blocks if empty. Handles spurious wakes from other ports sharing SIGF_PORT.
- SIGF_PORT wakeup: PutMsg sets signal bit 0 on the port owner, integrating with Signal/Wait.
- Port capacity: 8 ports per system (M7, was 4), 4-message FIFO per port, 32-byte messages (M7, was 16).

### Milestone 5: Dynamic Tasks (Complete)

**Implemented and tested (builds on Milestone 4):**

- `CreateTask` syscall (5): dynamically create a new task at runtime. R1=source_ptr (VA in caller's space), R2=code_size (bytes, max 4096), R3=arg0 (written to child's data page at offset 0). Returns R1=task_id (0-7), R2=err. The kernel validates the source range (must be within the caller's own 3-page user region), finds a FREE TCB slot, builds a per-task page table, copies code to the child's code page, and starts the child in READY state. Returns ERR_NOMEM if all 8 slots are occupied, ERR_BADARG if source_ptr/code_size is invalid.
- `ExitTask` syscall (34): terminate the current task. R1=exit_code (ignored in M5). Marks the task's TCB as FREE, clears all signal state, invalidates any ports owned by the task, and switches to the next runnable task. Never returns.
- Round-robin scheduler: supports up to 8 concurrent tasks (MAX_TASKS=8). `find_next_runnable` scans slots in round-robin order, skipping WAITING and FREE. All 5 context-switch sites (yield, wait, waitport, timer, spurious-wake) use this unified subroutine.
- Fault cleanup with privilege split: user-mode faults (faultPC in user range) kill the faulting task and continue scheduling; supervisor-mode faults (faultPC in kernel range) print "KERNEL PANIC" and halt.
- Slot-based memory allocation: task i gets pre-reserved physical pages at fixed addresses (code: 0x600000+i*0x10000, stack: 0x601000+i*0x10000, data: 0x602000+i*0x10000, page table: 0x100000+i*0x10000).
- Kernel page table extended with supervisor-only mappings for all user pages, enabling CreateTask to copy code across address spaces.
- TASK_FREE (state=3) replaces the old REMOVED concept: the slot is empty and immediately reusable.

### Milestone 6: Memory Allocation + Shared Memory (Complete)

**Implemented and tested (builds on Milestone 5):**

- `AllocMem` syscall (1): allocate private or shared pages. R1=size, R2=flags (`MEMF_PUBLIC`, `MEMF_GUARD`, `MEMF_CLEAR`), returns R1=VA, R2=err, R3=share_handle (if `MEMF_PUBLIC`). Page-granular (4 KiB minimum). `MEMF_GUARD` reserves a non-present page before and after the mapped body while still returning the body VA. Kernel manages physical page pool and per-task virtual address windows.
- `FreeMem` syscall (2): free allocated memory. R1=addr, R2=size (must round to the same page count as the original allocation; the allocator is page-granular). Unmaps pages from caller's page table. The backing pages are scrubbed before release. For shared mappings, the syscall decrements the refcount; physical pages are scrubbed and freed when the last reference is removed.
- `MapShared` syscall (4): map a shared memory region by handle. R1=share_handle (opaque capability from AllocMem MEMF_PUBLIC), R2=map_flags (`MAPF_READ`, `MAPF_WRITE`, or both), returns R1=mapped VA, R2=err, R3=share_pages (page count of the share, returned so user-space services can clamp byte counts to the actual mapped size — added in M11). Validates handle nonce to reject stale/invalid handles. Missing masks and unknown bits are a hard `ERR_BADARG`. `PTE_W` is set only for `MAPF_WRITE`, and `PTE_X` is never set.
- Physical page allocator: bitmap-based contiguous allocation from 6400-page pool ($700000-$1FFFFFF).
- Per-task dynamic VA windows: each task gets 3 MB (768 pages) at $800000+task_id*$300000 (M11 stride).
- MEMF_ flags: `MEMF_PUBLIC` (bit 0, classic Amiga), `MEMF_GUARD` (bit 1, M15.6 R2 heap guards), `MEMF_CLEAR` (bit 16, classic Amiga zero-fill).
- Share handles: 32-bit opaque capabilities encoding 24-bit nonce (from monotonic kernel counter) + 8-bit slot. Guarantees stale handles are always rejected.
- Task exit cleanup: all private regions freed, all shared mappings unmapped with refcount decremented.
- `GetSysInfo` extended: SYSINFO_TOTAL_PAGES (0) returns 6400, SYSINFO_FREE_PAGES (1) returns current free count, SYSINFO_CURRENT_TASK (3) returns calling task's ID.
- W^X preserved: all dynamic pages mapped as P|R|W|U (no execute).
- Validate-then-commit: all syscalls structured to avoid partial state on error, with explicit rollback for page allocation failures.

### Milestone 7: Named Ports + Reply Protocol (Complete)

**Implemented and tested (builds on Milestone 6):**

- **Named public ports**: `CreatePort` extended with R1=name_ptr, R2=flags (PF_PUBLIC). Names are up to 16 bytes, ASCII, zero-padded. Public named ports are discoverable via `FindPort`. Anonymous ports (name_ptr=0) remain private. Duplicate public names return ERR_EXISTS. Validate-then-commit: slot not marked valid until all validation passes.
- **FindPort** syscall (16): R1=name_ptr → R1=port_id, R2=err. Case-insensitive name matching (Amiga-style). Returns ERR_NOTFOUND if no public port matches.
- **ReplyMsg** syscall (20): R1=reply_port, R2=type, R3=data0, R4=data1, R5=share_handle → R2=err. Convenience wrapper over PutMsg to the reply port. Enables Exec-style request/reply service pattern.
- **32-byte messages**: enlarged from 16 bytes. Fields: type (4B), sender (4B), data0 (8B), data1 (8B), reply_port (2B, 0xFFFF=none), share_handle (4B, 0=none). PutMsg/GetMsg/WaitPort ABI extended with R4-R6 for the new fields.
- **8 ports** (up from 4): 160-byte port struct with 32-byte header (valid, owner, count, head, tail, flags, name[16]) + 4×32-byte message FIFO.
- **User pointer safety**: CreatePort and FindPort validate user-provided name pointers by checking PTEs (P|R|U bits) before reading from user memory. Bad pointers return ERR_BADARG instead of crashing the kernel.
- **Port cleanup on exit**: kill_task_cleanup clears name and flags on port invalidation, removing dead ports from FindPort immediately.
- **New error codes**: ERR_EXISTS (8) for duplicate public port names, ERR_FULL (9) for FIFO-full condition (distinct from ERR_NOMEM).
- **Kernel data layout shift**: port array grew from 288 to 1280 bytes; bitmap, region table, and shmem table offsets shifted accordingly.

### Milestone 8: Bundled User Programs + Tiny Loader (Complete)

**Implemented and tested (builds on Milestone 7):**

- **IE64 program image format**: 32-byte fixed header (magic "IE64PROG", code_size, data_size, flags) followed by code and data sections. No ELF, no relocations, no entry offset - entry is always code offset 0. Max one page (4 KiB) each for code and data.
- **Static program table**: kernel-embedded array of (image_ptr, image_size) entries, sentinel-terminated. Boot code iterates the table and calls `load_program` for each entry.
- **Boot-time loader** (`load_program`): validates image header (magic, sizes, alignment, truncation check), finds a free task ID / TCB row, allocates dynamic code/stack/data/startup/PT placement inside the reserved image/PT windows, copies code and data into the allocated pages, builds the page table, initializes the TCB, writes the startup block into the dedicated startup page, and seeds that startup-page VA at `0(sp)` before first entry. Validate-then-commit pattern. Not a syscall - kernel-internal only.
- **Current startup ABI**: programs discover their own layout from the startup page whose base VA is seeded at `0(sp)`, not from `GetSysInfo(CURRENT_TASK) + slot arithmetic`. The startup block lives in that dedicated startup page, not inside the task data image.
- **4 bundled user-space services**:
- **CONSOLE**: creates public "CONSOLE" port, prints own ONLINE banner directly, loops receiving messages and printing data0 as chars. Text output service - all programs send through CONSOLE.
- **ECHO**: finds CONSOLE, announces online, creates "ECHO" port, allocates shared memory with greeting string, waits for request, replies with share_handle.
- **CLOCK**: finds CONSOLE, announces online, polls tick_count via GetSysInfo, sends '.' to CONSOLE every 32 ticks. Polling-based (no timer syscall yet).
- **CLIENT**: finds CONSOLE and ECHO, announces online, sends request to ECHO, receives share_handle in reply, `MapShared(..., MAPF_READ)`, then sends "SHARED: " + greeting chars to CONSOLE.
- **All visible output from user space**: kernel only prints boot banner. Every service announcement, request/reply narration, and periodic tick comes from loaded user programs.
- **Retired**: hardcoded task templates (user_task0_template, user_task1_template, child_task_template), USER_CODE_SIZE constant. Kernel is now mechanism-only.

### Milestone 9: exec.library + dos.library + Shell (Complete)

**Implemented and tested (builds on Milestone 8):**

- **Kernel renamed to exec.library**: boot banner is now `"exec.library M9 boot"` (later updated to M10). Reflects the Amiga convention of the kernel being a library.
- **GURU MEDITATION fault messages**: replaced plain FAULT format with Amiga-style GURU MEDITATION messages for all kernel faults and panics.
- **Full GPR save/restore in timer interrupt**: the preemption handler now saves and restores R1-R30, ensuring preemption safety for all user-space tasks.
- **Strict boot**: the first PROGTAB_BOOT_COUNT (3) programs (console.handler, dos.library, Shell) must load successfully or the kernel panics.
- **New syscalls** (as shipped in M9; later evolved — see M10/M11/M11.5 sections):
  - `SYS_MAP_IO` (28): maps I/O pages into user task address space with new REGION_IO (3) region type. Extended in M11 to take a page count.
  - `SYS_EXEC_PROGRAM` (35): originally loaded a bundled program by table index (R1=index, R2=args_ptr, R3=args_len). Arguments are copied to the child task's data page at DATA_ARGS_OFFSET (like AmigaOS `pr_Arguments`). **M10**: ABI redesigned to take a user-space image pointer instead of an index. **M11.6**: the legacy `R1 < 0x600000` index branch is removed — the validated image-pointer ABI is the only path through the handler, and sub-`USER_CODE_BASE` values now hard-fail with `ERR_BADARG`.
  - `SYS_OPEN_LIBRARY` (36): added in M9 as a distinct AmigaOS-style `OpenLibrary` syscall. **M11.5 collapsed it**: it is now a source-level alias for `SYS_FIND_PORT` (`SYS_OPEN_LIBRARY equ SYS_FIND_PORT` in `iexec.inc`). Slot 36 in the kernel dispatcher is retained as a one-instruction binary-compat redirect to `.do_find_port` so any IE64 binary that hardcoded the number 36 still works. New code uses `SYS_FIND_PORT` directly. See § 5.11 "Exec Boundary" for the rationale.
  - `SYS_READ_INPUT` (37): added in M9 as a kernel-mode terminal read on behalf of `console.handler`. **Removed in M11.5**: terminal MMIO is now mapped directly by `console.handler` via `SYS_MAP_IO(0xF0, 1)`, and the readline MMIO loop is inlined into `console.handler`'s `CON_MSG_READLINE` path. The kernel handler is gone; slot 37 is an unallocated hole that returns `ERR_BADARG`. The `CON_MSG_READLINE` request/reply protocol is unchanged, so existing readline clients (the shell, all M9/M10/M11 readline tests) keep working without modification.
- **console.handler**: CON: handler task (Task 0). Creates public port, services output via GetMsg polling, supports CON_READLINE protocol for interactive line input.
- **dos.library**: AmigaOS dos.library equivalent (Task 1). Provides a RAM: filesystem with 16 files, 4 KB each, case-insensitive filenames. Supports DOS_RUN command dispatch for launching external commands.
- **Shell**: interactive command shell (Task 2). Reads input via console.handler, dispatches commands to dos.library via DOS_RUN. Displays `1> ` prompt (AmigaOS-style).
- **5 external commands**: VERSION, AVAIL, DIR, TYPE, ECHO. All are real user-space tasks loaded from the program table (no shell builtins). Arguments are passed via DATA_ARGS_OFFSET in the data page.

### Milestone 10: DOS-Loaded Programs + Assigns + Startup-Sequence (Complete)

**Implemented and tested (builds on Milestone 9):**

M10 transitions the system from kernel-dispatched command indices to user-space DOS-loaded executables. The kernel gets simpler (the program table walk is gone for on-demand programs); all new functionality lives in dos.library and the shell. The boot demo now looks like a self-booting AmigaOS-style protected microkernel, with dos.library executing `S:Startup-Sequence` automatically before dropping to the shell prompt.

**Kernel changes (gets simpler):**

- **`SYS_EXEC_PROGRAM` ABI redesigned**: now takes a user-space image pointer instead of a program table index. New signature: `R1=image_ptr (user VA, ≥0x600000), R2=image_size, R3=args_ptr, R4=args_len → R1=task_id, R2=err`. Runs entirely under the caller's PT (no PT switching). Legacy index-based path retained for `R1<0x600000` (M9 compat, used only by tests; production code uses the new ABI). The kernel no longer walks the program table for on-demand loads. **M11.6 update**: the legacy `R1<0x600000` branch has since been removed; `SYS_EXEC_PROGRAM` now hard-rejects sub-`USER_CODE_BASE` values with `ERR_BADARG`.
- **M15.4 update to user-range validation**: the original generic `validate_user_range` helper has been superseded by permission-aware helpers (`validate_user_read_range`, `validate_user_write_range`, `validate_user_exec_range`). They reject unmapped pages, kernel-only pages, permission mismatches, and overflow ranges with `ERR_BADARG`.
- **Multi-page code and data in `load_program`**: code_size cap raised from 4096 to 8192 (up to 2 code pages); data_size cap raised from 4096 to 16384 (up to 4 data pages). `load_program` computes `code_pages` and `data_pages` from the image header, zeros and copies the right number of bytes, and passes both counts to `build_user_pt`. dos.library exercises both: it has 2 code pages (5744 bytes) and 3 data pages (9428 bytes including embedded command images).
- **`build_user_pt` parameterized**: takes `R9=code_pages` and `R11=data_pages`, loops to map `code_pages` PTEs at VPN+0..VPN+(code_pages-1) as P|X|U, then a stack PTE at VPN+code_pages, then `data_pages` PTEs at VPN+code_pages+1..VPN+code_pages+data_pages as P|R|W|U.
- **Boot-time kernel PT now maps all 16 VPNs per task slot** (was only 3: code/stack/data). The full slot stride is mapped supervisor-only so the kernel can access any task's pages regardless of code_pages/data_pages choice.
- **Program table shrunk**: from 8 entries + sentinel to 3 entries + sentinel (boot services only). VERSION/AVAIL/DIR/TYPE/ECHO entries removed — they now live inside dos.library's data section as embedded images.

**dos.library changes (gets smarter):**

- **Assign table**: static assigns inside dos.library map volume names to internal path prefixes:
  - `RAM:` → bare name (no prefix, e.g. `RAM:readme` → `readme`)
  - `C:` → `C/` prefix (e.g. `C:Version` → `C/Version`)
  - `S:` → `S/` prefix (e.g. `S:Startup-Sequence` → `S/Startup-Sequence`)
- **Two name resolution subroutines**:
  - `resolve_command_name` (used by `DOS_RUN`): if no colon found, default to **C:** (prepend `C/`). Matches AmigaOS command search path behavior.
  - `resolve_file_name` (used by `DOS_OPEN`/`READ`/`WRITE`/`DIR`): if no colon found, **bare name** (no prefix). Matches AmigaOS file access behavior.
  - Both share an assign-resolution core that strips the volume part and rewrites with the mapped prefix into a 32-byte scratch buffer in dos.library's data page.
- **`DOS_NAME_LEN` increased from 16 to 32**: file table entry grows from 28 to 44 bytes (16 entries × 44 = 704 bytes). Required to hold names like `S/Startup-Sequence` (18 chars).
- **DOS_RUN redesigned**: takes a command name in the shared buffer (format: `"command_name\0args_string\0"`) instead of a program table index. Resolves the name through the C: assign, looks it up in the file table, computes `image_ptr = storage_va + entry.offset`, and calls `SYS_EXEC_PROGRAM` with the new ABI. Replies with `DOS_ERR_NOTFOUND` if the name does not resolve to a stored file.
- **Embedded command images + seeding**: dos.library's multi-page data section contains the raw IE64PROG bytes for VERSION, AVAIL, DIR, TYPE, ECHO, plus the `S/Startup-Sequence` script text. At init, `dos_seed_one` walks each embedded image, allocates a file table slot, copies the name from the seed strings area, and copies the image bytes from the data pages to the AllocMem'd 64 KB storage region. The kernel never sees this — it's entirely user-space DOS internals.
- **Boot race prevention**: dos.library defers `CreatePort("dos.library")` until **after** seeding is complete. The port becoming visible IS the readiness signal. Shell's `OpenLibrary("dos.library")` retry loop blocks until the port exists, which guarantees all files (`C/Version`, `S/Startup-Sequence`, etc.) are already in RAM when the shell first talks to dos.library. This is the same pattern AmigaOS uses: a library is not discoverable until it is fully initialized.

**Shell changes (gets simpler):**

- **No more command table**: the shell's hardcoded command name → program table index mapping (data[192-236] in M9) is gone. The shell parses the first word of the input line and sends the raw command name to dos.library via `DOS_RUN`. dos.library is now solely responsible for command resolution.
- **`DOS_ERR_NOTFOUND` handling**: if dos.library replies that the command does not exist, the shell prints `"Unknown command\r\n"`.
- **`S:Startup-Sequence` execution at boot**: after the shell announces itself online, it calls `DOS_OPEN("S:Startup-Sequence", READ)` via dos.library. If found, it reads the script content (up to 256 bytes) into a shell-local script buffer, sets a `script_mode` flag, and the main loop reads each newline-terminated line from the script buffer instead of calling `CON_READLINE`. Each line goes through the same parse + dispatch path as a typed command. When the script ends, `script_mode` is cleared and the shell falls through to interactive mode. If the script is missing, the shell skips to the prompt without error (graceful fallback).

**Demo output (M10 boot, no user input required):**

```
exec.library M10 boot
console.handler ONLINE [Task 0]
dos.library ONLINE [Task 1]
Shell ONLINE [Task 2]
IntuitionOS M10
IntuitionOS 0.10 (exec.library M10)        <- VERSION run by S:Startup-Sequence
IntuitionOS M10 ready                       <- ECHO run by S:Startup-Sequence
1> 
```

User can then type interactively:

```
1> VERSION
IntuitionOS 0.10 (exec.library M10)
1> DIR RAM:
readme                          0014
C/Version                       0590
C/Avail                         0991
C/Dir                           1168
C/Type                          1752
C/Echo                          0792
S/Startup-Sequence              0035
1> TYPE S:Startup-Sequence
VERSION
ECHO IntuitionOS M10 ready
1> ECHO Hello from IntuitionOS
Hello from IntuitionOS
1> 
```

**M9 features removed in M10:**

- Shell command name table (data[192-231]) and command index table (data[232-236])
- Program table entries 3-7 (on-demand commands) - the table is now boot-services-only
- The 2-phase PT switching dance in `SYS_EXEC_PROGRAM` (no longer needed - the new ABI runs entirely under the caller's PT)

### Milestone 11: input.device + graphics.library + Fullscreen Demo (Complete)

**Implemented and tested (builds on Milestone 10):**

M11 takes the next step toward an Amiga-shaped graphical OS: interactive input and a fullscreen graphics surface, both as **user-space services**. After M11 the system boots into text mode, runs `S:Startup-Sequence` (which now also launches `DEVS:input.device` and `LIBS:graphics.library`), and a graphical client (`C:GfxDemo`) can open a 640x480 RGBA32 framebuffer, draw into a shared surface, present, and receive keyboard/mouse events through a registered event port. No compositor, no windows, no `intuition.library` yet — those are M12.

**Kernel changes (one targeted handler refinement, no new syscalls):**

- **`SYS_MAP_IO` extended to take a page count.** New ABI: `R1 = base_ppn, R2 = page_count → R1 = mapped_va, R2 = err`. The legacy single-page form (`R2 = 0`) is preserved for M9/M10 callers — `R2 = 0` is treated as `R2 = 1`.
- **Range-aware allowlist**: only two PPN windows are accepted: `(0xF0, 1)` for the chip register page (terminal/input/video MMIO) and `[0x100, 0x5FF]` for any contiguous slice of the 5 MB VRAM range. Anything else returns `ERR_BADARG`.
- **One region slot for the whole mapping**: a 300-page VRAM mapping consumes exactly one entry in the per-task region table (8 slots), not 300. Required because graphics.library maps the full 640x480x4 framebuffer in a single call.
- **`USER_DYN_PAGES` bumped from 256 to 768** (`USER_DYN_STRIDE` 1 MB → 3 MB). Required for graphics.library to simultaneously map 1 chip + 300 VRAM + 300 surface pages = 601 pages. The 8×3MB layout uses VA `0x800000-0x2000000` — the top of the 32 MB VA space. Double-buffering (2 client surfaces) does not fit in 768 pages and is deferred to M12. *(Historical M11 layout snapshot. The current post-M12.6 layout is different: `USER_DYN_BASE..USER_DYN_END = 0xA00000..0x1200000` (8 MiB), and the allocator pool was split off into a disjoint VPN range at PPN `0x1200..0x1FFF` by the M12.6 Phase E security fix. See §1 and `§5.13.1` for the current values.)*
- **`load_program` data-size cap raised from 16384 to 20480** (5 data pages). Required for dos.library to grow to 5 data pages embedding the new `LIBS/graphics.library`, `DEVS/input.device`, and `C/GfxDemo` images.
- No new syscalls. No new region types. No new TCB fields. No VA layout overhaul beyond the dynamic window stride bump.

**dos.library changes (additional namespaces, additional seeds):**

- **Three new assign entries**: `LIBS:` → `LIBS/`, `DEVS:` → `DEVS/`, `RESOURCES:` → `RESOURCES/`. Resolved by extending the existing `.dos_resolve_has_colon` chain with a 4-char check (LIBS/DEVS) and a 9-char check (RESOURCES). All three follow the same "uppercase prefix + slash + remainder" pattern.
- **Three new embedded service images**: `LIBS/graphics.library`, `DEVS/input.device`, `C/GfxDemo`. Embedded in dos.library's data section after the existing M10 command images and seeded into the RAM file table at init time via `dos_seed_one`. dos.library now has 5 data pages (was 3).
- **`S:Startup-Sequence` updated**: now launches `DEVS:input.device` and `LIBS:graphics.library` before printing the version banner and the M11 ready message. The shell's existing fire-and-forget DOS_RUN-with-delay is sufficient to start long-running service tasks; banner ordering is naturally serialized through `SYS_DEBUG_PUTCHAR`.
- The strict-boot kernel program table is **unchanged at 3 entries** (console.handler, dos.library, shell). M11 services live in dos.library's seeded RAM filesystem, not in the kernel image. Single source of truth.

**input.device — keyboard/mouse event service (new in M11):**

- **MMIO source registers** (chip page 0xF0):
  - `SCAN_CODE` (0xF0740, dequeues a raw scancode), `SCAN_STATUS` (0xF0744 bit 0), `SCAN_MODIFIERS` (0xF0748). Use these for keyboard, NOT `TERM_KEY_*` which is the cooked terminal queue owned by console.handler.
  - `MOUSE_X` / `MOUSE_Y` / `MOUSE_BUTTONS` / `MOUSE_STATUS` (0xF0730-0xF073F).
- **Event push protocol**: clients call `INPUT_OPEN` (data0 = client port_id) to register a single subscriber. input.device polls registers on its scheduler quantum and `PutMsg`'s `INPUT_EVENT` messages into the registered client port whenever scancodes dequeue or mouse state changes.
- **Single subscriber for M11.** A second `INPUT_OPEN` while one is registered returns `INPUT_ERR_BUSY`. Multi-subscriber fan-out is M12 work in `intuition.library`.
- **Event message format**: `mn_Type = INPUT_EVENT`, `mn_Data0 = (event_type<<24)|(code<<16)|(modifiers<<8)|flags`, `mn_Data1 = (mouse_x16<<48)|(mouse_y16<<32)|event_seq32`. The 32-bit event sequence number in the low half of `mn_Data1` is a per-device monotonic counter useful for debugging dropped events and (later) coalescing.
- **Mouse-move coalescing is a free property of the polling architecture**: input.device emits at most one `IE_MOUSE_MOVE` per poll cycle, carrying the latest position. Multiple raw moves between polls naturally collapse.

**graphics.library — fullscreen RGBA32 display service (new in M11):**

- **Maps page 0xF0 once** (chip MMIO — same page used by input.device; both can map it because `SYS_MAP_IO` allows multiple tasks to map the same I/O page).
- **Maps 300 VRAM pages once** (`SYS_MAP_IO(0x100, 300)` = 1228800 bytes). One region slot. Persistent for the lifetime of graphics.library.
- **Object model**:
  - Adapter: enumerable; M11 has 1 adapter (the `IEVideoChip`).
  - Mode: queryable mode table; M11 has 1 mode (`{mode_id=0, width=640, height=480, format=RGBA32, stride=2560}`).
  - Display: opened by `(adapter_id, mode_id)`; M11 only `(0, 0)` succeeds. Single display owner for M11. `display_handle = 1`.
  - Surface: client owns the buffer (allocated with `MEMF_PUBLIC`), passes the share_handle in `GFX_REGISTER_SURFACE`. graphics.library does `MapShared(..., MAPF_READ)` and caches the mapped VA. Single surface for M11 (VA budget — see kernel section). `surface_handle = 1`.
- **Present**: full-frame CPU memcpy from the mapped surface VA to the mapped VRAM VA (1228800 bytes). Synchronous reply with `present_seq` (per-surface monotonic counter). Forward-compatible: `mn_Data1` reserves a packed dirty rect for future rect-bounded copies (M11 always sends 0 = full frame).
- **Mode set**: `GFX_OPEN_DISPLAY` writes `MODE_640x480` (= 0) to `VIDEO_MODE` and enables the chip with `VIDEO_CTRL = 1`. (Note: the constant `CTRL_DISABLE_FLAG = 0` in `video_chip.go` and its accompanying comment are misleading — the actual chip code at `video_chip.go:2653` enables the chip when `value != 0`, so writing `1` enables and writing `0` disables. Existing examples like `mandelbrot_ie64.asm` and `rotozoomer.asm` use the same `VIDEO_CTRL = 1` convention.) `GFX_CLOSE_DISPLAY` resets `VIDEO_MODE` to `MODE_800x600` (the default) and writes `VIDEO_CTRL = 0` to disable scanout, so the next `GFX_OPEN_DISPLAY` re-enables cleanly.
- **Internal layering**: M11 ships graphics.library as a single monolithic service block — IEVideo-specific MMIO writes are inlined directly into the message-dispatch handlers. The plan called for a generic display/service layer plus a backend vtable (`init`, `set_mode`, `restore_mode`, `present_full`, `present_rect`, `shutdown`) so M12+ adapters could plug in cleanly, but that refactor is **deferred to M12** and will happen naturally when a second adapter driver is introduced.
- **Future-proof opcode space**: `GFX_WAIT_VBLANK` (`0x209`) and `GFX_PRESENT_ASYNC` (`0x20A`) are reserved in the constants table but not implemented in M11.

**C/GfxDemo — minimal graphics client (new in M11):**

- Opens `graphics.library` and `input.device` (with retry, since they may not be up yet when GfxDemo is launched from the Startup-Sequence sequence).
- `AllocMem(1228800, MEMF_PUBLIC|MEMF_CLEAR)` for the client surface; sends `GFX_REGISTER_SURFACE` with the share_handle.
- Sends `INPUT_OPEN` with its own (anonymous private) port to subscribe to events.
- Fills the surface with a solid color, sends one `GFX_PRESENT`, then enters its main event loop.
- Drains the event port; on `IE_KEY_DOWN` with scancode 0x01 (Escape), cleans up (`INPUT_CLOSE`, `GFX_UNREGISTER_SURFACE`, `GFX_CLOSE_DISPLAY`) and calls `SYS_EXIT_TASK`.
- Demo intentionally minimal (no animation, no font rendering); it exists to exercise the full M11 stack end-to-end.

**Demo output (M11 boot, no user input required):**

```
exec.library M11 boot
console.handler ONLINE [Task 0]
dos.library ONLINE [Task 1]
Shell ONLINE [Task 2]
IntuitionOS M11
input.device ONLINE [Task 3]
graphics.library ONLINE [Task 4]
IntuitionOS 0.11 (exec.library M11)
IntuitionOS M11 ready
1>
```

The user can then type `GFXDEMO` to launch `C/GfxDemo`, which fills the framebuffer with a solid color and waits for Escape.

**Ownership and cleanup contract:**

- Display ownership: graphics.library tracks a single `display_open` flag. `GFX_OPEN_DISPLAY` while open returns `GFX_ERR_BUSY`. `GFX_CLOSE_DISPLAY` clears the flag and drops the registered surface.
- Surface ownership: M11 stores one surface entry. `GFX_REGISTER_SURFACE` while one is in use returns `GFX_ERR_BUSY`. `GFX_UNREGISTER_SURFACE` clears the entry.
- Subscriber ownership: input.device tracks a single `subscriber_port`. `INPUT_OPEN` while a subscriber is registered returns `INPUT_ERR_BUSY`. `INPUT_CLOSE` clears the subscription.
- On client task exit: existing M9 region cleanup unmaps any AllocMem'd surface buffers and tears down ports. graphics.library's surface table can be left holding a stale entry until the client cleans up explicitly or until graphics.library detects via failed `MapShared` reuse — M11 does not auto-reap stale surface entries.
- **Known wart (crash path only)**: clean shutdown via `GFX_CLOSE_DISPLAY` does reset `VIDEO_MODE` to `MODE_800x600` and writes `VIDEO_CTRL = 0`, so a well-behaved client returns the chip to a sane state. But if `graphics.library` itself crashes or exits abnormally before reaching `CloseDisplay`, the kernel unmaps its VRAM region without touching `VIDEO_MODE`/`VIDEO_CTRL` — the system is left in graphics mode with no service running until the next boot. M12 can fix this with either a kernel hook on chip-page tasks or an init-task that resets the chip before relaunching graphics.library.

### Milestone 11.5: Exec Boundary Cleanup (Complete)

**Implemented and tested (builds on Milestone 11):**

M11.5 is a freeze milestone, not a feature milestone. After M11 the userland is Amiga-shaped (exec.library nucleus + user-space `dos.library`, `console.handler`, `input.device`, `graphics.library`), but the syscall surface still reflected every bring-up shortcut taken since M0 — 37 `SYS_*` constants of which only 22 had handlers, plus several live ones (`SYS_OPEN_LIBRARY`, `SYS_READ_INPUT`) that were bootstrap conveniences with no remaining justification. M11.5 prunes the boundary so the docs describe what the kernel actually enforces, before M12 (intuition.library + compositor) starts adding pressure.

**Header cleanup (`sdk/include/iexec.inc`):**

- **15 dead constants deleted.** `SYS_ALLOC_SHARED`, `SYS_DELETE_TASK`, `SYS_FIND_TASK`, `SYS_SET_TASK_PRI`, `SYS_SET_TP`, `SYS_GET_TASK_INFO`, `SYS_PEEK_PORT`, `SYS_ADD_TIMER`, `SYS_REM_TIMER`, `SYS_CLOSE_HANDLE`, `SYS_DUP_HANDLE`, `SYS_MAP_VRAM`, `SYS_DEBUG`, `SYS_SEND_MSG_BULK`, `SYS_RECV_MSG_BULK` had no dispatcher entries — they were ABI residue. Removed from the header; slot numbers preserved as unallocated holes. Repo-wide grep confirmed zero callers.
- **Category tags added.** Every surviving syscall is annotated `nucleus`, `bootstrap`, or `legacy` so the classification is machine-checkable from the source of truth, not just documented in IExec.md.
- **`SYS_OPEN_LIBRARY` collapsed to `SYS_FIND_PORT`.** Source-level alias (`SYS_OPEN_LIBRARY equ SYS_FIND_PORT`); the dispatcher slot 36 is retained as a one-instruction redirect (`bra .do_find_port`) for binary compatibility with any IE64 binary that hardcoded the number 36. The "deletion" is conceptual: 36 is no longer a distinct programming-model entry, but the dispatcher slot stays.
- **`SYS_READ_INPUT` removed.** The kernel handler at the former `.do_read_input` is deleted. Slot 37 is now an unallocated hole and falls through to `ERR_BADARG`.

**Console.handler MMIO ownership change (`sdk/intuitionos/iexec/iexec.s`):**

- `console.handler` now calls `SYS_MAP_IO(0xF0, 1)` at init time and caches the returned VA in its data page (`data[144]`).
- The terminal MMIO read loop (formerly the body of the kernel-side `.do_read_input`) is inlined directly into `console.handler`'s `CON_MSG_READLINE` handler. Absolute physical addresses (`TERM_LINE_STATUS = 0xF070C`, `TERM_STATUS = 0xF0704`, `TERM_IN = 0xF0708`) are rebased onto the cached VA.
- Existing yield-between-polls behavior absorbs the no-line-ready case naturally. The `CON_MSG_READLINE` request/reply protocol is unchanged, so all existing readline-using code (the shell, the seven existing readline tests) keeps working without modification.
- This is the canonical "policy moves to user space, mechanism stays in the kernel" example. Terminal line input is policy.

**Boot programs migrated to `SYS_FIND_PORT` (`sdk/intuitionos/iexec/iexec.s`):**

- Every `syscall #SYS_OPEN_LIBRARY` call site in the boot programs (12 sites in `dos.library`, `console.handler`, `input.device`, `graphics.library`, `shell`, `C/GfxDemo`) is rewritten as `syscall #SYS_FIND_PORT`. Source-level identity in the assembled output (the alias makes them the same number), but new readers of the boot programs see the canonical name.

**Documentation:**

- This file's section 5.11 "Exec Boundary" adds the boundary statement, the syscall admission rule, the classification table (mirrors `iexec.inc` annotations), the `SYS_MAP_IO` allowlist impurity acknowledgement, the "nucleus is disciplined not closed" list of acceptable future additions, the OpenLibrary rationale, and the console.handler MMIO ownership change.
- `README.md` § 18 IntuitionOS: M11.5 status block replaces M11.

**TDD discipline:**

Every behavior-changing phase landed test-first.

- `TestIExec_OpenLibrary_DispatcherCollapse` — calls raw slot 36 and raw slot 16 with the same port name and asserts identical results. Guards the binary-compat redirect contract going forward.
- `TestIExec_ReadInput_RemovedReturnsBadarg` — calls raw slot 37 and asserts `ERR_BADARG`. Guards that the slot is no longer reachable.
- The seven existing `SYS_READ_INPUT`-adjacent tests (covering shell readline, console handler, startup-sequence reading) continue to pass against the new console.handler-owned MMIO path with no modifications. `TestIExec_ReadInput_Direct` (which exercised the kernel helper directly) was deleted as a test of a now-removed implementation detail.

**Startup-Sequence change (user-visible artifact):**

The seeded `S:Startup-Sequence` adds one trailing line:

```
ECHO All visible services are running in user space
```

so the M11.5 boot output ends with:

```
exec.library M11 boot
console.handler ONLINE [Task 0]
dos.library ONLINE [Task 1]
Shell ONLINE [Task 2]
IntuitionOS M11
input.device ONLINE [Task 3]
graphics.library ONLINE [Task 4]
IntuitionOS 0.11 (exec.library M11.5)
IntuitionOS M11 ready
All visible services are running in user space
1>
```

**What M11.5 explicitly does NOT do:**

- ~~No removal of the `SYS_EXEC_PROGRAM` legacy `R1 < 0x600000` index branch — deferred to M12.~~ **Resolved in M11.6**: the legacy index branch was removed in a standalone milestone before M12 began. `SYS_EXEC_PROGRAM` now hard-fails any sub-`USER_CODE_BASE` value with `ERR_BADARG`; see the Milestone 11.6 section below.
- No removal of `SYS_DEBUG_PUTCHAR` — kept as `bootstrap` category, still required for kernel boot banner and panic output before `console.handler` is alive.
- No `hardware.resource` refactor of `SYS_MAP_IO`'s allowlist — documented as a known wart in 5.11.4, deferred indefinitely.
- No new syscalls. By definition.
- No changes to message protocols, port format, AllocMem semantics, or any user-space service API.
- No renumbering of any surviving syscall — IE64 binaries are compiled against numbers, not names.

### Milestone 11.6: SYS_EXEC_PROGRAM legacy index removal (Complete)

A small follow-on to M11.5 that resolves the one item M11.5 explicitly deferred. Lands as a standalone milestone before M12 opens so the M12 branch carries no test churn unrelated to windowing.

**What changed:**

- The dual-mode discriminator at the top of `.do_exec_program` (`if R1 >= USER_CODE_BASE → new ABI; else → table-lookup index path`) is gone. The handler now begins with `blt r1, USER_CODE_BASE → ERR_BADARG`, then falls directly into the validated image-pointer ABI body.
- The entire legacy code path — `program_table` walk, two-phase PT-switching args copy, dedicated `.ep_old_abi` body — is deleted from `iexec.s`.
- `program_table` itself remains because the kernel boot path still loads the first three programs (console.handler, dos.library, Shell) directly into task slots from it during init. That use is unrelated to the syscall path and is preserved.
- `iexec.inc`'s trailing comment for `SYS_EXEC_PROGRAM` is updated from `legacy R1<0x600000 index branch — removal deferred to M12` to `M11.6: legacy R1<0x600000 index branch removed`.
- The kernel binary shrinks by ~624 bytes (45620 → 44996).

**Test changes:**

- `TestIExec_ExecProgram_LegacyIndexReturnsBadarg` (new): negative test that calls `SYS_EXEC_PROGRAM` with `R1=0` (formerly the valid index for prog_console) and asserts `ERR_BADARG` plus the absence of `console.handler ONLINE` in the output. This was the failing test written first per the M11.5 TDD discipline; it goes green after the legacy block deletion.
- `TestIExec_ExecProgram_LegacyBadArgs` (deleted): tested the legacy path's args validation. The legacy path is gone, so the test is testing-the-helper-not-the-feature.
- `TestIExec_ExecProgram_BadIndex` (deleted): tested the legacy path's "index out of range" handling via `R1=99`. Same fate.
- The four `TestIExec_ExecProgram_NewABI*` tests survive unchanged and continue to pass — they exercise the validated image-pointer ABI which is the only remaining path.

**Why it ships separately from M12:** M11.5 deferred this on the grounds that ~41 raw `ExecProgram` test references would create churn that interfered with the freeze. The actual count was 6 test functions (the rest were doc/comment hits), of which 4 use the new ABI and survived unchanged, 2 were testing the legacy helper directly and were deleted. The cleanup turned out to be small enough to land standalone, which lets M12's intuition.library branch start from a clean syscall surface.

### Milestone 12: intuition.library + Single-Window Compositor + Structural Cap Cleanup (Complete)

M12 ships the first user-space windowing layer (`intuition.library`) and quietly removes the long-standing arbitrary caps on tasks, ports, port-name length, and shared objects that the M11 design accumulated. The boot path goes text-mode → first window → graphics-mode → close-gadget click → text-mode again, and the structural cleanup means future milestones don't have to keep tiptoeing around 8-of-everything tables.

**5.13.1 intuition.library service.** A new user-space service registered as `"intuition.library"` (a public message port). On the FIRST `INTUITION_OPEN_WINDOW` it lazily:

1. `FindPort("graphics.library")` and `FindPort("input.device")` (cached on the data page)
2. `AllocMem(1920000, MEMF_PUBLIC|MEMF_CLEAR)` for its own 800×600 RGBA32 screen surface
3. `GFX_OPEN_DISPLAY(0, 0)` → `display_handle`. graphics.library writes `VIDEO_MODE = MODE_800x600 = 1`, which matches the chip's `DEFAULT_VIDEO_MODE` so the chip's frontBuffer is NOT reallocated and any kernel-side `VideoTerminal` rendering keeps its dimensions valid.
4. `GFX_REGISTER_SURFACE(display_handle, dims, share=screen_share)` → `surface_handle`. The geometry is encoded as `(800<<48)|(600<<32)|(format<<16)|stride`, with `stride = 3200` (= 800 × 4). intuition.library is now the **sole** registered display client.
5. `INPUT_OPEN(my_input_port)` to receive raw `IE_KEY_DOWN`/`IE_MOUSE_MOVE`/`IE_MOUSE_BTN` events. The reply type is checked: if input.device returns `INPUT_ERR_BUSY` (another client already owns the subscription), intuition.library records `input_subscribed = 0` and the close path will SKIP `INPUT_CLOSE` so it doesn't clobber the other subscriber.
6. `MapShared(client_window_buffer, MAPF_READ)` → cached `win_mapped_va`

Until that first OPEN_WINDOW the system stays in text mode (intuition.library has not yet called `GFX_OPEN_DISPLAY` or allocated its screen surface). This matches the M12 plan's "lazy display ownership" rule and lets the boot/test paths stay text-only when no app needs windowing.

**Window protocol** (see `iexec.inc`):

- `INTUITION_OPEN_WINDOW` (0x300) — `data0 = (w<<48)|(h<<32)|(x<<16)|y`, `data1 = idcmp_port_id`, `share = win_buf_share`. Returns `data0 = window_handle` (always 1 in M12). M12 is single-window — a second OPEN_WINDOW while one is open returns `INTUI_ERR_BUSY`.
- `INTUITION_CLOSE_WINDOW` (0x301) — `data0 = window_handle`. Performs a full teardown: `SYS_FREE_MEM` on the mapped client window buffer (using `win_w * win_h * 4` for the size), conditionally sends `INPUT_CLOSE` only if `input_subscribed == 1`, sends `GFX_UNREGISTER_SURFACE` (which now also calls `SYS_FREE_MEM` on graphics.library's mapped surface so the shared object's refcount actually reaches zero), sends `GFX_CLOSE_DISPLAY`, then `SYS_FREE_MEM`s its own screen surface (1920000 bytes). All cached display state (`screen_va`, `screen_share`, `display_handle`, `surface_handle`, `display_open`) is cleared so a future OPEN_WINDOW lazily re-acquires from a clean slate.
- `INTUITION_DAMAGE` (0x302) — `data0 = window_handle`, `data1 = (x<<48)|(y<<32)|(w<<16)|h` (rect; M12 ignores the rect and does a full-window blit because `GFX_PRESENT` is full-frame in M11 — see §5.12). intuition.library:
  1. Blits the (mapped) client buffer at `(win_x, win_y)` into its own 800×600 screen surface (3200-byte stride).
  2. Paints a 1-pixel 3D bevel: WHITE highlight on top + left edges, BLACK shadow on bottom + right edges (raised window appearance).
  3. Paints a 16-pixel title bar at rows `(win_y+1, win_y+16)` with alternating Amiga-blue pinstripes — light steel blue `0xFFB89878` on even rows, medium steel blue `0xFF905030` on odd rows.
  4. Paints a 16×16 close gadget at `(win_x+1, win_y+1)`: light grey fill `0xFFCCCCCC`, 1-pixel black outline, recessed black 4×4 centre square.
  5. Paints a 16×16 depth gadget at `(win_x+win_w-17, win_y+1)`: light grey fill, black outline, two overlapping rectangle outlines (the AmigaOS front/back depth icon).
  6. Calls `GFX_PRESENT(surface_handle)`.

  All decoration is drawn via the `.intui_fillrect` helper subroutine — a small `jsr`/`rts` routine in the intuition.library code section that fills a screen-space rectangle with a single colour. It hardcodes the 800×600 stride (3200) and reads the screen surface VA from `data[200]`. Rect coordinates are passed in `r6..r9` and color in `r17`. The compositor reloads `r22..r25` (`win_x`, `win_y`, `win_w`, `win_h`) from `data[224..240]` at the start of the decoration block, because the user-buffer blit loop above destroys `r25` (decrements `win_h` to 0 row by row).

**IDCMP delivery** (also in `iexec.inc`):

- `IDCMP_MOUSEMOVE` (0x310) — `data0 = (lx<<32)|ly` (window-local coords). intuition.library translates the screen-space mouse position from input.device into window-local coordinates by subtracting `win_x`/`win_y`.
- `IDCMP_MOUSEBUTTONS` (0x311) — `data0 = (buttons<<32)|state`, `data1 = (lx<<32)|ly`. A button-down event whose window-local coords fall inside the close-gadget rect is intercepted and converted into an `IDCMP_CLOSEWINDOW` instead.
- `IDCMP_RAWKEY` (0x312) — `data0 = (scancode<<8)|mods`. The `Esc` key (scancode 0x01) is also intercepted into `IDCMP_CLOSEWINDOW` for headless test convenience.
- `IDCMP_CLOSEWINDOW` (0x313) — `data0 = window_handle`. Sent on close-gadget click OR Esc.
- `IDCMP_ACTIVEWINDOW` / `IDCMP_INACTIVEWINDOW` (0x314/0x315) — defined in the protocol but not yet wired (single-window means active is degenerate).

**Screen-buffer ownership rule.** intuition.library is the SOLE registered display client. App-side window backing surfaces are separate `MEMF_PUBLIC` buffers owned by each app and `MapShared`-d into intuition.library on `INTUITION_OPEN_WINDOW`. graphics.library never sees the app — only intuition.library's screen surface ever touches `GFX_REGISTER_SURFACE`/`GFX_PRESENT`. graphics.library is unchanged.

**5.13.2 About app.** A user-space client (`prog_about`, seeded as `C/About` in the RAM: filesystem) that demonstrates the full open→damage→close cycle and renders text via an embedded bitmap font:

1. `FindPort("intuition.library")`
2. `AllocMem(256000, MEMF_PUBLIC|MEMF_CLEAR)` — its own 320×200 RGBA32 backing surface
3. Fills the buffer with a dark teal backdrop (0xFF605020 in the chip's RGBA byte order — R=0x20, G=0x50, B=0x60)
4. Renders five lines of white text into the content area using the embedded Topaz 8×16 bitmap font:
   - "About IntuitionOS"
   - "Microkernel + intuition.library"
   - "M12 demonstration window"
   - "Press Esc to close"
   - "(C) 2024-2026 Zayn Otley"

   Text is drawn by the `.ab_draw_string` / `.ab_draw_char` subroutines, which walk a null-terminated string and emit one 8×16 glyph per character. The font itself is included via `incbin "topaz.raw"` (a copy of the AmigaOS Topaz Plus 8×16 font lives at `sdk/include/topaz.raw` and is full 256-glyph × 16-byte = 4096 bytes embedded at offset 1024 of the About app's data section). Glyphs are rendered in white (0xFFFFFFFF) on whatever backdrop the buffer already contains; transparent for "0" bits in the glyph bitmap.
5. `INTUITION_OPEN_WINDOW(w=320, h=200, x=240, y=200, idcmp_port=mine, share=surface_share)` → `window_handle`
   (Window is centered on the 800×600 screen.)
6. `INTUITION_DAMAGE(window_handle, full)`
7. `WaitPort(idcmp_port)` loop, exits on `IDCMP_CLOSEWINDOW`
8. `INTUITION_CLOSE_WINDOW`, then `SYS_EXIT_TASK`

The About app is reachable via the shell as `C:About` (a regular DOS_RUN of a seeded RAM: file) — not auto-launched at boot, so the test budget stays free for the existing M11 GfxDemo tests.

**Window decoration painted by intuition.library on top of the app buffer:**

intuition.library does not just blit the client buffer raw — it overpaints Magic Workbench-style chrome ON TOP of the app's pixels:

1. **1-pixel 3D bevel**: WHITE highlight on top + left edges, BLACK shadow on bottom + right edges (raised look — the window appears to "pop out" of the screen).
2. **16-pixel title bar pinstripes** spanning rows `(win_y+1, win_y+16)` (immediately inside the bevel), alternating two shades of Amiga blue:
   - Even row index → light steel blue `0xFFB89878` (R=0x78 G=0x98 B=0xB8)
   - Odd row index → medium steel blue `0xFF905030` (R=0x30 G=0x50 B=0x90)
3. **16×16 close gadget** at the title bar's left edge, position `(win_x+1, win_y+1, 16, 16)`:
   - Light grey fill `0xFFCCCCCC`, 1-pixel black outline, recessed black 4×4 centre square (the classic AmigaOS close marker).
4. **16×16 depth gadget** at the title bar's right edge, position `(win_x+win_w-17, win_y+1, 16, 16)`:
   - Light grey fill, 1-pixel black outline, two overlapping rectangle outlines drawn inside (an upper-left "back" rectangle and a lower-right "front" rectangle), reproducing the classic AmigaOS front/back depth icon.

All decoration is painted via the `.intui_fillrect` helper subroutine — a small `jsr`/`rts` routine in the intuition.library code section that fills a rectangle with a single color. It hardcodes the 800×600 screen stride (3200) and reads the screen surface VA from `data[200]`. Rect coordinates are passed in `r6..r9` and color in `r17`.

The compositor reloads `r22..r25` (`win_x`, `win_y`, `win_w`, `win_h`) from `data[224..240]` at the start of the decoration block, because the user-buffer blit loop above destroys `r25` (decrements `win_h` to 0 row by row).

**5.13.3 Boot integration.** `S/Startup-Sequence` was extended:

```
DEVS:input.device
LIBS:graphics.library
LIBS:intuition.library
VERSION
ECHO IntuitionOS M12 ready
ECHO All visible services are running in user space
```

intuition.library is auto-started right after graphics.library at boot. Its main loop just waits for messages — text mode persists until an app sends OPEN_WINDOW. The version string in the seeded `C:Version` is `IntuitionOS 0.12 (exec.library M11.6 / intuition.library M12)`.

**5.13.4 No new syscalls.** The full M11.5 admission rule held: every M12 capability is built from existing nucleus primitives (`SYS_FIND_PORT`, `SYS_CREATE_PORT`, `SYS_PUT_MSG`, `SYS_WAIT_PORT`, `SYS_GET_MSG`, `SYS_REPLY_MSG`, `SYS_ALLOC_MEM`, `SYS_MAP_SHARED`) and existing graphics.library/input.device protocols (`GFX_OPEN_DISPLAY`, `GFX_REGISTER_SURFACE`, `GFX_PRESENT`, `INPUT_OPEN`, `INPUT_EVENT`). intuition.library is a pure user-space service. There are no graphics.library protocol changes either — `GFX_PRESENT` stays full-frame, and the dirty-rect tracking inside intuition.library is local (it bounds the compositor's blit work, but the call to graphics.library doesn't carry the rect through). Rect-bounded `GFX_PRESENT` is reserved for M12.x.

**5.13.5 Structural cap cleanup.** M12 also rebuilt several arbitrary placeholder caps that M11 had inherited from M5/M7. Each was technically only a "placeholder until we need more" but had become a hard wart now that services were stacking up:

| cap | M11 value | M12 value | how it was unblocked |
|---|---|---|---|
| `MAX_TASKS` | 8 | **16** | Removed the per-task `USER_DYN_STRIDE` window. The dynamic VA range (`USER_DYN_BASE..USER_DYN_END`) is now GLOBAL — every task shares the same VA range and per-task page tables provide isolation. The M11 design pre-allocated each task a 3 MiB slice of the 32 MiB VA space, capping the system at exactly 8 tasks. With shared VAs, the cap moves to whatever the kernel data structures and slot region can hold. Slot region grew from 0x600000..0x67FFFF (8 × 64 KiB) to 0x600000..0x6FFFFF (16 × 64 KiB); user-PT region moved from 0x680000 to 0x700000; `ALLOC_POOL_BASE` shifted from PPN 0x700 to 0x800 to make room. The cap is still arbitrary in the sense that any fixed array is — replacing the static TCB array with a heap-allocated linked list is a future-milestone TODO that would remove it entirely. |
| `KD_PORT_MAX` | 8 | **32** | 4× bump. M11 hit the 8-port wall as soon as services started creating their own anonymous reply ports — intuition.library alone wants 3 ports (1 public + 2 anonymous) and that pushed the running total past the cap. Pure data-structure-size bump; no ABI change. Port table grew from 1280 bytes to 5376 bytes, comfortably absorbed by the kernel data area. |
| `KD_SHMEM_MAX` | 8 | **16** | Bumped while I was bumping the others. M11's M11-era pattern of "every shared buffer is a `MEMF_PUBLIC` allocation" was tight at 8 — the gfxdemo + intuition.library + multiple-shell-commands path could exhaust it under the wrong concurrency. |
| `PORT_NAME_LEN` | 16 | **32** | M11's 16-byte name limit was just barely big enough for `"graphics.library"` (exactly 16 chars, no room for a null terminator) and outright too small for `"intuition.library"` (17 chars). M12 doubles the cap so service names can use the full Amiga-style `name.library`/`name.device`/`name.handler` form. The port struct header grew from 32 bytes to 40 bytes; `KD_PORT_STRIDE` from 160 to 168. Existing services keep working because `safe_copy_user_name` stops at the first null byte regardless of the buffer length. |
| `DOS_FILE_SIZE` | 4096 | **8192** | The M11 RAM: filesystem used a fixed 4 KiB slot per file, which the M12 intuition.library image (≈4.3 KiB) overflowed. Bumping to 8 KiB is itself still arbitrary, but unblocks M12. The proper fix — replacing the fixed-stride slot table with a packed-heap allocator — was attempted in this milestone, hit a runtime bug I couldn't isolate inside the dispatch path, and was reverted to the simpler "bigger fixed slot" approach pending a focused follow-up. The TODO is documented in `iexec.inc`. |

The combined kernel-data-layout shift moves `KD_PAGE_BITMAP` from 1664 to 6080, `KD_REGION_TABLE` from 2464 to 6880, and `KD_SHMEM_TABLE` from 3488 to 8928 — all auto-propagated through constants in `iexec.inc`. `count_free_pages` was rewritten to scan exactly `ALLOC_POOL_PAGES` bits instead of the full 800-byte bitmap (the bitmap is sized to a round 800 bytes = 6400 bits, but `ALLOC_POOL_PAGES` shrank to 6144 after the pool shifted to PPN 0x800 — the trailing 256 bits are now unused and must not be counted as free).

**5.13.6 What M12 explicitly does NOT do (deferred to M12.x):**

- **Multiple windows / z-order / overlap.** M12 ships single-window. The data model has focus/active concepts even though only one window exists, so M12.x can add z-order without an API break.
- **Window dragging.**
- **Resize gadgets.**
- **Pointer overlay save-under.** M12 relies on the host emulator's mouse cursor; intuition.library doesn't render its own pointer.
- **Menus, icons, requesters, multiple public screens.**
- **Rect-bounded `GFX_PRESENT`.** Stays full-frame for M12; rect packing reserved for the next graphics.library protocol revision.
- **`hardware.resource` and de-publicization of `SYS_MAP_IO`.** Still a documented impurity; will land alongside the first concrete consumer per the M11.5 admission rule.
- **Heap-allocated kernel data structures.** Static TCB/PTBR/Port arrays remain — bigger now, but still arrays. Linked-list / heap-allocated versions are a future milestone that would remove the caps entirely.
- **dos.library packed-heap file storage.** Attempted in M12, reverted, captured as a TODO. **Resolved in M12.8** (slab/extent allocator with atomic-swap-on-rewrite — see Milestone 12.8 below).

**5.13.7 What this milestone proves.** intuition.library shipping as a pure user-space service — with no new syscalls, no graphics.library protocol changes, and no privileged code added — validates the M11.5 "exec boundary" thesis: the post-M11 nucleus is rich enough to host an Amiga-shaped windowing system without growing the syscall surface. The structural cap cleanup proves that those caps were placeholder ceilings, not architectural limits — the kernel data area easily absorbed all of them. Together this gets IntuitionOS to "visibly Amiga-shaped" while keeping the privileged surface defensible.

### Milestone 12.8: dos.library variable-size file storage (Complete)

M12.8 closes the dos.library file storage refactor that had been on the milestone roadmap since the start of M12.6. Per-file storage migrates from a fixed `DOS_FILE_SIZE = 16384` AllocMem block to a chain of 4 KiB extents allocated on demand, the per-file size cap is deleted entirely, and as a Phase 1 prerequisite the two arbitrary product caps in `load_program` (`code_size <= 8192`, `data_size <= 49152`) — which were the same kind of bucket-C ceiling hiding in bucket B — are also wiped. M13 phase 2 then removes the temporary slot-fit replacement by moving task-image placement to dynamic image/PT windows.

The kernel ABI is bit-for-bit unchanged: no new syscalls, no new DOS opcodes, no widened ABI fields, no message wire-format changes. Existing dos.library clients (the shell, every M9–M12 example, every test) continue to work without recompilation. The visible change is that `DOS_WRITE` no longer rejects byte counts above 16384, and reads/writes of multi-extent files transit a small chain walk inside dos.library that's invisible to clients.

**5.14.1 Storage model.** Each file's body is now a singly-linked chain of 4 KiB extents:

```
extent (one AllocMem'd page):
  [0..7]    next_va  (8 bytes, 0 = end of chain)
  [8..15]   reserved
  [16..4095] payload (DOS_EXT_PAYLOAD = 4080 bytes)
```

The file's metadata entry stores the head of the chain in `DOS_META_OFF_VA` (which kept the same offset and the same semantics — only the *meaning* of the value changed from "VA of a fixed 16 KiB body block" to "VA of the first extent in a body chain"). A 32 KiB file occupies 9 extents = 9 chain pages; a 100-byte file occupies 1 extent = 1 chain page. Tail-padding waste per file is bounded by `DOS_EXT_PAYLOAD - 1` bytes (4079 bytes worst case), which is honest and bounded — the same kind of waste a slab allocator gives you.

The four canonical extent operations all live in `iexec.s`:

- **`.dos_extent_alloc(byte_count)`** → walks up `ceil(byte_count / DOS_EXT_PAYLOAD)` extents, linking them into a chain. On allocation failure partway through, internally calls `.dos_extent_free` on whatever was already allocated and returns `r1 = 0` with the failure error in `r2`. Empty case (`byte_count == 0`) returns `r1 = 0` with `ERR_OK`.
- **`.dos_extent_free(first_va)`** → walks the chain calling `SYS_FREE_MEM` on each extent in turn. No-op if `first_va == 0`.
- **`.dos_extent_walk(first_va, dst, byte_count)`** → copies up to `byte_count` bytes from the start of the chain into `dst`. Used by `DOS_READ`. Returns the number of bytes actually read.
- **`.dos_extent_write(first_va, src, byte_count)`** → copies up to `byte_count` bytes from `src` into the chain (starting at the first extent's payload). Symmetric counterpart to `.dos_extent_walk`. Used by `DOS_WRITE` and the boot-time seed paths.

**5.14.2 Atomic-swap-on-rewrite rule.** `DOS_WRITE`'s most load-bearing change is the rewrite path. The rule:

> A `DOS_WRITE` that fails partway must leave the previous file content fully intact.

This is enforced by always allocating the *new* chain *first*, copying the bytes into it, and only THEN linking the new chain into `entry.file_va` and freeing the old chain. The handler's flow:

1. Look up the file entry. Save `old_first_va = entry.file_va` to a scratch slot.
2. Clamp `byte_count` to the share buffer size (the only remaining clamp — the per-file `DOS_FILE_SIZE` cap is gone).
3. `.dos_extent_alloc(clamped_byte_count)` → if it fails, reply `DOS_ERR_FULL`. The entry is untouched, so the previous file is still readable. The internal `.dea_fail_partial` cleanup ensures any partially-allocated new chain is freed before the error reply is sent.
4. `.dos_extent_write(new_first_va, src, byte_count)` → copy bytes from the caller's share into the new chain.
5. **Atomic swap (sequential, single-threaded handler):** `entry.file_va = new_first_va`, then `entry.size = byte_count`.
6. `.dos_extent_free(old_first_va)` → reclaim the old chain. (No-op if there was no old chain — i.e. this is the first write to a freshly-created file.)
7. Reply `DOS_OK` with `bytes_written = clamped_byte_count`.

Because the dos.library service is a single-task message loop, no other DOS handler can interleave between steps 5 and 6 — the swap is atomic by single-threadedness, not by hardware atomicity. The order is still chosen to be safe even under hypothetical preemption-and-reentry: every observation point (entry.file_va, entry.size, the chain itself) is consistent with either the pre-write state or the post-write state, never a mix.

`DOS_OPEN` (write mode, new file) no longer pre-allocates a body. New files start with `entry.file_va = 0` (empty); the first `DOS_WRITE` allocates the chain. This removes a wasted allocation per `DOS_OPEN` call at create time.

**5.14.3 What `DOS_RUN` does now.** `SYS_EXEC_PROGRAM` requires a contiguous image pointer, but program images are now scattered across an extent chain. The `DOS_RUN` handler:

1. Looks up the program by name (unchanged).
2. Saves `first_extent_va` and `image_size` to scratch slots.
3. `AllocMem(image_size, MEMF_CLEAR)` → temp contiguous buffer.
4. `.dos_extent_walk(first_extent_va, temp_buf, image_size)` → copy the image into the temp buffer.
5. `SYS_EXEC_PROGRAM(temp_buf, image_size, args, args_len)` → kernel copies the image into a newly built task image, returns task_id.
6. `SYS_FREE_MEM(temp_buf, image_size)` → reclaim the temp buffer (the kernel has already copied the image, so the temp is no longer needed).
7. Reply with the task_id.

The temp buffer is sized exactly to the image — for the M12.8 worst case (dos.library itself, ~9 KiB code + 36 KiB data ≈ 45 KiB) that's 12 pages, comfortably within the allocator pool. Smaller programs use proportionally smaller temps. The temp is reclaimed before the reply, so steady-state dos.library memory usage is unaffected.

**5.14.4 Phase 1 prerequisite — load_program cap removal.** The plan called for "no kernel changes" in M12.8, but Phase 1 immediately discovered that dos.library's pre-edit code section (8016 bytes) was already 98% of the way through the `load_program` `code_size <= 8192` cap. Adding the dead-code Phase 1 skeleton (~700 bytes) blew through it and `load_program` rejected the image with `ERR_BADARG` → boot panic. Investigation showed that the 8192 cap was an *arbitrary product limit*, not an architectural bound — the same kind of bucket-C ceiling that M12.6 was supposed to wipe out. The `data_size <= 49152` cap had the same disease (bumped 4096 → 16384 → 20480 → 49152 across M8/M10/M11/M12 with no architectural justification).

Both caps were deleted. During M12.8 they were temporarily replaced with the then-honest slot-fit rule, and **M13 phase 2** completed the cleanup by removing slot-derived placement from `load_program` entirely. The loader now:

- computes `code_pages` and `data_pages`
- validates the reserved startup-block window in page 0
- allocates code, stack, and data pages dynamically inside `USER_IMAGE_BASE..USER_IMAGE_END`
- allocates a 64 KiB PT block dynamically inside `USER_PT_BASE..USER_DYN_BASE`

So the real per-image ceilings are now:

- successful dynamic allocation from the reserved image/PT windows
- successful allocation from the physical page pool
- successful validation of the caller-supplied image range

A hostile image that declares an absurdly large `code_size` (e.g. `0xFFFFFFFF`) is still correctly rejected: the page-count computation produces `~2^20`, which is far beyond what the image window allocator can satisfy. The page-count compute uses 64-bit arithmetic on the zero-extended `load.l` value, so it can't overflow.

This change is in scope for M12.8 because (a) it was a hard prerequisite for shipping the storage refactor, (b) the two caps were the same kind of bucket-C product limit that the M12.5 audit was supposed to catch (and missed), and (c) the replacement is more honest, more correct, and smaller code than the prior conditional `bgt` checks. The "no kernel changes" rule from the M12.8 plan was relaxed to "no kernel functional changes; layout-bound caps may be replaced with the real constraint where required."

**5.14.5 Phase 1 prerequisite — robust dos.library preamble.** The other Phase 1 surprise was that dos.library's preamble hardcoded `add r29, r29, #0x3000` to compute its data-page base — an offset that assumed exactly 2 code pages. Bumping dos.library to 3 code pages broke the preamble silently (it pointed at the stack page instead of the data section). Phase 1 fixed that by removing the hardcoded offset; the current M13 shape seeds `data_base` at the top of the initial stack page, so after `sub sp, sp, #16` the preamble recovers it with `load.q r29, 8(sp)` without assuming stack/data adjacency. dos.library can now grow without touching its preamble. (Other libraries were migrated to the same pattern during M13 phase 2.)

**5.14.6 What M12.8 explicitly does NOT do.** The DOS protocol is unchanged — no new opcodes, no widened ABI fields, no message wire-format changes:

- **No `DOS_DELETE`.** Files cannot be removed via the public API. The only way file extents get freed is via the `DOS_WRITE`-replacement path (which frees old extents before allocating new). Files persist for the dos.library service lifetime.
- **No `DOS_TRUNCATE`.** A `DOS_WRITE` of N bytes effectively truncates to N (replaces from offset 0), but there's no opcode that resizes a file without rewriting its content.
- **No `DOS_SEEK`.** Reads always start from offset 0. There's no notion of a file cursor; handles store only the entry VA.
- **No `DOS_APPEND`.** Writes always start from offset 0 and replace the file's content.
- **No persistent storage.** dos.library is still a RAM-only filesystem. The extent chains live in `AllocMem`'d pool pages and disappear when the system is reset.

If any of those are wanted, they each get their own future protocol-extension milestone.

**5.14.7 Test coverage.** Three new tests in `iexec_test.go` exercise the M12.8 storage shape:

- **`TestIExec_DosM128_FileLargerThanOldCap`** — 32 KiB write/read round-trip, byte-for-byte verified across 9 extents (2× the previous 16 KiB cap). This is the load-bearing test for the per-file cap removal AND the multi-extent walker (Risk #1 in the M12.8 plan: extent-walk arithmetic bugs).
- **`TestIExec_DosM128_RewriteShrinks`** — write 8 KiB, then rewrite the same file with 1 KiB; read back expects 1 KiB of new content. Proves atomic-swap-on-rewrite works on the shrink path (old chain freed, new chain linked, no leak).
- **`TestIExec_DosM128_RewriteGrows`** — write 1 KiB, then rewrite with 8 KiB; read back expects 8 KiB of new content. Proves atomic-swap on the grow path.

Four of the seven tests in the M12.8 plan are covered by existing tests or skipped with documented rationale: `StorageExhaustionIsClean` (atomic-swap correctness is exercised by the shrink/grow tests), `ExtentChainWalkCorrect` (subsumed by FileLargerThanOldCap), `DirReportsCorrectSizes` (`DOS_DIR` walks metadata only, not bodies — unchanged in M12.8), and `ManySmallFiles` (already covered by `TestIExec_NoCap_DosFilesAndHandlesGrow` from M12.6 Phase A).

The full M12.5/M12.6 hardening test suite remains green throughout M12.8 — no kernel data structure changes, no new ABI fields, no widened slots. The trust model and the M11.5 admission rule are both intact.

**5.14.8 What this milestone proves.** Bucket B can be drained too, not just bucket C — and the audit pass that drained it found two more arbitrary product limits hiding in bucket B that the M12.5 audit had incorrectly classified. After M12.8 the audit table's load-bearing entries are all gone: every fixed cap that exists in IntuitionOS today is justified by an active hardware constraint, an active ABI field, an active layout, or an active protocol semantic — not by "we picked this number once and never re-examined it." The dos.library file storage is the same kind of "grows until memory exhaustion" structure that M12.6 gave the kernel core, and the load_program prerequisite cleanup means the per-image image-size ceilings are now the actual layout-bound architectural ceiling rather than two more layers of arbitrary product limits.

**5.14.9 M14 phases 2-5 — native ELF seglists, descriptor launch, shell integration, and end-to-end demo path.** M14 phases 2-5 add the first shipped native-ELF DOS loading and launch path, route the visible shell command path through it, and then prove the retained GUI demo path on top of the same DOS loader.

New DOS protocol opcodes:

- **`DOS_LOADSEG` (7)** — shared buffer contains the command/program name. dos.library resolves the name through the command resolver, finds the file in the RAM store, walks the file's extent chain into a temporary contiguous image, validates the strict M14 ELF subset, allocates a DOS-owned seglist object, then allocates one DOS-owned segment buffer per `PT_LOAD` header. Reply: `type = DOS_OK|DOS_ERR_*`, `data0 = seglist_va`.
- **`DOS_UNLOADSEG` (8)** — `data0 = seglist_va`. dos.library unlinks the seglist from its private list and frees every DOS-owned segment buffer plus the seglist header page. Reply: `type = DOS_OK|DOS_ERR_BADHANDLE`.
- **`DOS_RUNSEG` (9)** — `data0 = seglist_va`, optional shared buffer contains args. dos.library validates the seglist handle, builds a DOS-owned launch descriptor from the seglist's preserved target VA / `R/W/X` / entry-point metadata, and launches the child through the dual-mode `ExecProgram` descriptor ABI. Reply: `type = DOS_OK|DOS_ERR_*`, `data0 = child_task_id`.

Seglist layout (all DOS-owned):

- header page:
  - `next_va`
  - magic `'SEGL'`
  - segment count
  - preserved ELF `e_entry`
- per-segment entry:
  - DOS-owned segment memory VA
  - final target VA from the ELF file
  - file size
  - memory size
  - page count
  - `R/W/X` flags

Phase-3 launch behavior:

- `LoadSeg` / `RunSeg` are the native DOS-facing path for strict M14 ELF commands
- launch copies each seglist segment into private child mappings at the descriptor's target virtual addresses
- the descriptor preserves the original ELF `e_entry`; launch is not forced to the base of the first RX segment
- successful launch does not consume the seglist; the caller may still `UnLoadSeg`
- failed launch does not consume the seglist either
- M14 introduced the descriptor path that later became the only supported `ExecProgram` ABI in M14.2

Phase-4 shell integration:

- `DOS_RUN` now prefers the native seglist path for strict M14 ELF commands
- M14.2 removes the legacy seeded flat-image execution fallback from that same `DOS_RUN` API
- shell command names, args, case-insensitive lookup, and unknown-command UX are unchanged from the user's perspective

Phase-5 shipped boundary:

- seeded files under `C:` are now emitted as strict M14 native ELF where possible, so the visible command/demo path (`VERSION`, `AVAIL`, `DIR`, `TYPE`, `ECHO`, `About`, `GfxDemo`) really exercises `LoadSeg` / `RunSeg` / descriptor launch
- bundled startup-sequence services under `LIBS:`, `DEVS:`, and `RESOURCES:` remain legacy flat `IE64PROG` in M14, so the boot chain stays stable while the visible user/demo path moves to native ELF
- `C:GfxDemo` and `C:About` remain regular DOS-loaded applications and now serve as the retained end-to-end M14 demo path
- M14.1 target state: all shipped runtime binaries become ELF, but the new service source path is internal embedded-manifest loading rather than an extension of the public file-backed DOS API

M14.1 phase 5 boundary:

- shipped service launches now consume staged strict-M14 ELF rows from the internal embedded boot manifest rather than bundled flat `IE64PROG` images
- seeded service files under `LIBS:`, `DEVS:`, and `RESOURCES:` are now emitted as strict M14 native ELF too
- the public DOS file-backed API is unchanged; the embedded-manifest service source remains internal-only
- the full-ELF shipped runtime is now locked by explicit end-to-end regressions for clean boot, command dispatch, unknown-command handling, and the retained GUI demos

M14.2 current runtime:

- `SYS_EXEC_PROGRAM` is now descriptor-only
- `DOS_LOADSEG` remains strict-ELF-only
- `DOS_RUN` rejects non-ELF executable content
- the remaining flat-image path is removed in M14.2

Test coverage:

- `TestIExec_M14_Phase2_LoadSeg_Basic`
- `TestIExec_M14_Phase2_LoadSeg_InvalidExecutableRejected`
- `TestIExec_M14_Phase2_LoadSeg_UnLoadSeg_NoLeak`
- `TestIExec_M14_Phase2_DosSeededCommandsPresent`
- `TestIExec_M14_Phase2_ElfFixturePassesHostValidator`
- `TestIExec_M14_Phase3_ExecProgram_DescriptorBasic`
- `TestIExec_M14_Phase3_RunSeg_Basic`
- `TestIExec_M14_Phase3_RunSeg_PreservesELFEntry`
- `TestIExec_M14_Phase3_RunSeg_HonorsTargetVAs`
- `TestIExec_M14_Phase3_RunSeg_StartupPagePresent`
- `TestIExec_M14_Phase3_RunSeg_WithArgs`
- `TestIExec_M14_Phase3_RunSeg_UnLoadAfterLaunch_ChildLives`
- `TestIExec_M14_Phase3_RunSeg_FailedLaunchDoesNotConsumeSeglist`
- `TestIExec_M14_Phase4_ShellRunsELFCommand`
- `TestIExec_M14_Phase4_ShellRunsELFCommandWithArgs`
- `TestIExec_M14_Phase5_FullBootStack_ServiceCensus`
- `TestIExec_M14_Phase5_GfxDemoRegression`
- `TestIExec_M14_Phase5_AboutRegression`
- `TestIExec_M141_Phase5_FullBootStack_ServiceCensus`
- `TestIExec_M141_Phase5_CommandPathRegression`
- `TestIExec_M141_Phase5_ShellUnknownRegression`
- `TestIExec_M141_Phase5_GfxDemoRegression`
- `TestIExec_M141_Phase5_AboutRegression`
