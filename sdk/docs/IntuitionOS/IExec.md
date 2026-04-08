# IExec.library -- Kernel Contract Reference

## Amiga Exec-Inspired Protected Microkernel for IE64

### (c) 2024-2026 Zayn Otley -- Intuition Engine Project

---

## 1. Overview

IExec.library is a protected microkernel for the IE64 CPU, inspired by AmigaOS Exec but designed from the ground up for a hardware-enforced privilege model. Where Amiga Exec ran in flat supervisor space with no memory protection, IExec uses the IE64 MMU to enforce user/supervisor separation, per-task page tables, and W^X memory policy.

**What IExec does (current as of Milestone 12.5):**

- Preemptive round-robin scheduling across up to 16 dynamic tasks (CreateTask/ExitTask with slot reuse). The 8-task cap from M5 was removed in M12 by globalizing the dynamic VA window so per-task VA stride no longer constrains the task count.
- Memory protection via the IE64 MMU (per-task page tables with separate code/stack/data mappings, W^X enforcement)
- Dynamic memory allocation: AllocMem/FreeMem with Amiga-style MEMF_ flags (MEMF_PUBLIC, MEMF_CLEAR), page-granular physical page pool (6144 pages = 24 MiB at PPN 0x800..0x1FFF), shared global VA range. The per-task region table is unbounded as of M12.5: the first 8 rows live inline; rows 9+ live in an allocator-backed overflow chain. Failure mode is real `ERR_NOMEM` from the page allocator, not a fixed-cap collision.
- Shared memory via MEMF_PUBLIC with opaque capability handles (MapShared), reference-counted cleanup on task exit
- Trap and interrupt dispatch (syscall entry, fault handling with privilege split, timer preemption)
- Context switching (save/restore PC, USP, PTBR, and full GPR set per task)
- Inter-task signalling: per-task 32-bit signal mask with AllocSignal/FreeSignal/Signal/Wait, deadlock detection
- Named public MsgPorts with CreatePort(name, PF_PUBLIC) and FindPort discovery (case-insensitive, up to 32 ports, 32-byte port names). The M11 8-port cap was bumped to 32 in M12; further bumps may follow as the M12.6 cap-removal sweep hits the port table.
- Request/reply messaging: 32-byte messages with data0, data1, reply_port, and share_handle fields
- ReplyMsg for Exec-style service pattern; PutMsg/GetMsg/WaitPort with full message ABI
- Safe user pointer validation (PTE check before kernel reads from user memory)
- Kernel renamed to exec.library; GURU MEDITATION fault messages
- AmigaOS-style `OpenLibrary` for library discovery — **M11.5**: source-level alias for `SYS_FIND_PORT`; the kernel ABI is one slot smaller. Slot 36 is retained as a one-instruction binary-compat redirect to `.do_find_port` so any pre-M11.5 IE64 binary still links. New code uses `SYS_FIND_PORT` directly. See § 5.11 "Exec Boundary".
- I/O page mapping via MapIO syscall (REGION_IO type)
- **`SYS_EXEC_PROGRAM` takes a user-space image pointer** (M10): kernel creates tasks from user-provided IE64PROG images. Runs entirely under the caller's PT (no PT switching). `validate_user_range` checks both P and U bits on every page in the requested range. **M11.6**: the legacy `R1 < USER_CODE_BASE` built-in-program-table index branch is removed — sub-`USER_CODE_BASE` values now hard-fail with `ERR_BADARG` and the validated image-pointer ABI is the only path through the handler.
- **Multi-page code and data in `load_program`** (M10): up to 2 code pages (8 KB) and 4 data pages (16 KB) per task. dos.library is itself a 2-code-page program, with 3 data pages containing embedded command images.
- console.handler: CON: handler with GetMsg polling and CON_READLINE protocol — **M11.5**: console.handler now owns terminal MMIO directly via its own `SYS_MAP_IO(0xF0, 1)` mapping and inlines the readline MMIO loop. The former kernel-side `SYS_READ_INPUT` (slot 37) is removed; slot 37 is an unallocated hole that returns `ERR_BADARG`.
- **dos.library**: AmigaOS dos.library equivalent with RAM: filesystem (16 files, 4 KB each, case-insensitive, 32-byte filenames). M10 adds: assign table (RAM:, C:, S:), name-based command resolution (`resolve_command_name` defaults to C:, `resolve_file_name` defaults to bare), embedded command images, init-time seeding into the RAM file store, and boot-race-free port creation (CreatePort deferred until after seeding).
- **Shell**: interactive command shell that sends raw command names to dos.library via DOS_RUN (no shell-side command table). Executes `S:Startup-Sequence` automatically at boot if present, then drops to the interactive prompt.
- **5 external commands** as DOS-loaded executables: VERSION, AVAIL, DIR, TYPE, ECHO. Stored as files in RAM under `C:`, launched by name through dos.library — not by program table index.

**What IExec does not do:**

- Filesystem access beyond RAM: (handled by DOS.library or host-side intercepts for persistent storage)
- Device drivers (hardware chips are memory-mapped; drivers live in user space)
- Graphics or audio (handled by respective chip subsystems)
- Dynamic loading (executables are loaded before boot; future: user-space loader)

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

**Kernel page table**: Identity-maps pages 0-383 (`$000000-$17FFFF`) as supervisor-only (P|R|W|X, no U). This covers the kernel region, low hardware I/O (including terminal MMIO at `$F0700` = page `$F0`), and the lower portion of VRAM/high I/O. Regions above `$17FFFF` are not mapped by the kernel PT - user-space drivers will gain access via `MapIO`/`MapVRAM` syscalls in a future milestone. User pages are only mapped in per-task page tables, not the kernel PT.

### 2.2 Kernel Memory Layout (Detail)

Within the supervisor region:

| Sub-region | Address | Size | Contents |
|------------|---------|------|----------|
| Vector table | `$000000-$000FFF` | 4 KB | Reserved (IE64 hardware vectors) |
| Kernel code | `$001000-$00FFFF` | 60 KB | Kernel text (boots at `$1000`) |
| Kernel page table | `$010000-$01FFFF` | 64 KB | 8192 PTEs x 8 bytes |
| Kernel data | `$020000-$02FFFF` | 64 KB | Scheduler state, TCB array (8 tasks), PTBR array, ports |
| (reserved) | `$030000-$09EFFF` | 448 KB | Available for future kernel use |
| Kernel stack | `$09F000` (top) | 4 KB | Grows downward |
| Task page tables | `$100000-$17FFFF` | 512 KB | 8 per-task PTs at `$100000 + i*$10000` |

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
- A page fault is raised if user code attempts to write to an X page or execute from a W page

### 2.5 Shared Memory

Shared memory regions are created via `AllocMem(MEMF_PUBLIC)`, which returns a VA and an opaque share handle in R3. Another task maps the region by calling `MapShared(share_handle)`. Both tasks see the same physical pages. The kernel tracks reference counts; `FreeMem` on a shared mapping removes the caller's mapping and decrements the refcount. Physical pages are freed when the last mapping is removed or the last mapping task exits. Share handles encode a 24-bit nonce derived from a monotonic kernel counter, guaranteeing that reusing a slot always produces a different handle. Stale handles are rejected with `ERR_BADHANDLE`.

---

## 3. Task Model

### 3.1 Task Creation

At boot, the kernel loads bundled program images into free task slots via the internal `load_program` helper. From Milestone 5 onward, additional tasks can also be created at runtime via `CreateTask` (syscall #5). The kernel allocates a TCB slot, builds a per-task page table, copies code from the caller's address space, and starts the child in user mode. Tasks exit via `ExitTask` (syscall #34), which cleans up ports and signals and frees the slot.

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

The scheduler uses round-robin across all task slots (MAX_TASKS = 8). It scans from the current task's slot forward, skipping WAITING and FREE slots, and selects the next READY or RUNNING task. Priority-based scheduling is planned for a future milestone.

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
| 4 | `MapShared` | R1=share_handle -> R1=addr, R2=err, R3=share_pages | **Implemented (M11 extended)** (`nucleus`) |

`AllocMem` flags: MEMF_PUBLIC (bit 0) = shareable across tasks, MEMF_CLEAR (bit 16) = zero-fill. Matches classic Amiga MEMF_ conventions. All allocations are page-granular (4 KiB). `FreeMem` size is also page-granular: the kernel rounds size up to pages and compares the page count against the allocation's page count. Any size that rounds to the same number of pages is accepted (e.g., both 5000 and 8192 free a 2-page allocation).

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
| 35 | `ExecProgram` | R1=image_ptr, R2=image_size, R3=args_ptr, R4=args_len -> R1=task_id, R2=err | **Implemented (M9, redesigned M10, legacy index path removed M11.6)** (`nucleus`) |
| 36 | `OpenLibrary` | R1=name_ptr -> R1=port_id, R2=err | **Binary-compat redirect to `SYS_FIND_PORT` (M11.5)**. Source-level alias (`SYS_OPEN_LIBRARY equ SYS_FIND_PORT`); kernel slot 36 dispatches to `.do_find_port` via a one-instruction redirect so older IE64 binaries still link. New code uses `SYS_FIND_PORT` directly. |
| 37 | -- | -- | Removed M11.5 (was `ReadInput`; terminal MMIO inlined into `console.handler` via `SYS_MAP_IO(0xF0, 1)`. Slot returns `ERR_BADARG`; guarded by `TestIExec_ReadInput_RemovedReturnsBadarg`.) |

**ExecProgram (35)** -- M10 ABI: Creates a new task from a user-provided IE64PROG image. R1=image_ptr (user VA pointing to a complete IE64PROG image, e.g. an entry in dos.library's RAM file store; must be ≥ `0x600000`), R2=image_size (total bytes including 32-byte header, code, and data; valid range 32..24608, matching `load_program`'s max of header + 8 KiB code + 16 KiB data), R3=args_ptr (user VA pointing to null-terminated argument string in the caller's address space, or 0 for no args), R4=args_len (byte count of arguments, max 256, or 0 for no args). The handler runs entirely under the **caller's** page table (no PT switching): every page in `[image_ptr, image_ptr+image_size)` and `[args_ptr, args_ptr+args_len)` is validated via `validate_user_range` (checks both **P** and **U** PTE bits), then `load_program` is called directly to copy the image into a free task slot. Arguments are copied to the new task's data page at `DATA_ARGS_OFFSET` (like AmigaOS `pr_Arguments`). Returns R1=new task_id, R2=err. Returns `ERR_BADARG` for unmapped/kernel-only ranges, oversize images, args_len > 256, or pointer arithmetic overflow.

**Legacy index path — REMOVED in M11.6**: Historically (M9), if R1 < `0x600000` the handler treated R1 as a `program_table` index and used the M9 ABI (R2=args_ptr, R3=args_len). M10 redesigned the primary ABI around a user-VA `image_ptr` but kept the legacy index branch behind a discriminator for M9 boot-services compatibility (and hardened its args validation with `validate_user_range`). **M11.6 removes the discriminator and the entire legacy code path**: the handler now begins with `blt r1, USER_CODE_BASE → ERR_BADARG`, so any caller passing R1 < `0x600000` hard-fails. The validated image-pointer ABI above is the only path through the handler. `program_table` itself is preserved because the kernel boot path still loads console.handler / dos.library / Shell from it directly into task slots at init, but it is no longer reachable from user mode via `SYS_EXEC_PROGRAM`. Guarded by `TestIExec_ExecProgram_LegacyIndexReturnsBadarg`.

**`validate_user_range` subroutine**: Walks the caller's page table once per page in the requested byte range. For each VPN, loads the PTE and checks `(pte & 0x11) == 0x11` (P bit + U bit set). Rejects unmapped pages, kernel-only pages, and pointer-arithmetic overflows. Returns R1=0 on success or 1 (ERR_BADARG) on any failure.

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
| 35 | `ExecProgram` | `nucleus` | M10 takes a user-VA `image_ptr`. The legacy `R1 < 0x600000` index path was removed in M11.6 — sub-`USER_CODE_BASE` values now hard-fail with `ERR_BADARG`, leaving the validated image-pointer ABI as the only path through the handler. |
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

**5.12.2 Grant table.** A chain-linked list of allocator-backed pages with the chain header at `KD_GRANT_TABLE_HDR` (offset 9192). Existing chain pages are NEVER copied or moved on grow — appending a new tail page only updates the previous tail's `next` field, so any kernel-internal pointer to a row stays valid. Each page holds 255 grant rows × 16 bytes (`KD_GRANT_TASK_ID` + 4-byte region tag + `PPN_LO` + `PPN_HI` + flags). There is no compile-time row cap; failure mode is real `ERR_NOMEM` from the page allocator. `TestIExec_HWRes_GrantTableChainGrows` is the executable proof that growth works and that existing rows survive across the boundary.

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

### 5.13 Architectural cap policy (M12.5)

> IntuitionOS does not use arbitrary fixed product limits for core OS objects where dynamic allocation is practical. Remaining limits must be justified by architecture, ABI width, hardware constraints, or explicitly configured resource policy.

This rule lands in M12.5 alongside `hardware.resource`, with a working demonstration: every new table introduced by `hardware.resource` itself is allocator-backed and growable from day one (no compile-time row cap), and exactly one pre-existing arbitrary cap — `KD_REGION_MAX` — is removed in the same milestone using the same chain-allocator pattern. The remaining bucket-C caps from the audit below are removed in a follow-up sweep milestone (M12.6) using the pattern proven in M12.5. M12.5 is "rule + first proof"; M12.6 is "removal pass."

**5.13.1 Cap classification audit.** Every fixed cap declared in `sdk/include/iexec.inc` and `sdk/intuitionos/iexec/iexec.s`, classified into three buckets:

- **A — architectural / ABI / hardware-bound.** Keep. These are bounded by the IE64 instruction format, the MMU contract, the on-disk message ABI, or the physical machine.
- **B — temporary implementation bound.** Keep for now, document a replacement plan.
- **C — arbitrary toy-era cap.** Remove. M12.5 removes one (`KD_REGION_MAX`); M12.6 sweeps the rest.

| Symbol | File:Line | Value | Bucket | Notes |
|---|---|---|---|---|
| `MMU_PAGE_SIZE` | `iexec.inc:308` | 4096 | A | MMU page size — part of the page-table format. |
| `MMU_PAGE_SHIFT` | `iexec.inc:309` | 12 | A | log2 of `MMU_PAGE_SIZE`. |
| `MMU_NUM_PAGES` | `iexec.inc:310` | 8192 | A | Total physical RAM pages on the IntuitionEngine target. Hardware-bound. |
| `KERN_PAGES` | `iexec.inc:311` | 384 | A | Kernel reserved page range. Bounded by the layout of `KERN_PAGE_TABLE` / `KERN_DATA_BASE` / kernel binary / kernel stack. |
| `KERN_PAGE_TABLE` | `iexec.inc:117` | 0x040000 | A | Fixed kernel page-table location, inside the kernel reserved range. |
| `KERN_DATA_BASE` | `iexec.inc:118` | 0x050000 | A | Fixed kernel data-page location. |
| `KERN_STACK_TOP` | `iexec.inc:119` | 0x09F000 | A | Top of the kernel stack inside the kernel reserved range. |
| `MAX_TASKS` | `iexec.inc:167` | 16 | C | Arbitrary, already bumped 8→16 in M12. Bounded only by `MAX_TASKS * USER_SLOT_STRIDE = 1 MiB` of slot region and the kernel data-page TCB array. **M12.6 sweep candidate.** Replacing the static TCB/PTBR arrays with a heap-allocated linked list removes it. |
| `KD_TASK_STRIDE` | `iexec.inc:170` | 32 | A | TCB layout — bounded by the on-data-page field offsets, not arbitrary. |
| `KD_PORT_STRIDE` | `iexec.inc:225` | 168 | A | Port size = 40-byte header + 4×32-byte messages. Bounded by `KD_PORT_FIFO_SIZE` × `KD_MSG_SIZE` + header. |
| `KD_PORT_MAX` | `iexec.inc:226` | 32 | C | Arbitrary, already bumped 8→32 in M12 because intuition.library's anonymous reply ports pushed past the M11 cap. **M12.6 sweep candidate.** Wider blast radius than `KD_REGION_MAX` because every service touches port allocation. Left for the focused M12.6 sweep so M12.5 can land cleanly. |
| `KD_PORT_FIFO_SIZE` | `iexec.inc:227` | 4 | B | Per-port message queue depth, not a global system cap. Tied to message-port flow control semantics; replaceable later but not load-bearing. |
| `PORT_NAME_LEN` | `iexec.inc:241` | 32 | A | ABI — every port-name field across the kernel + protocol uses this width. Bumping it later would be a flag-day change to message-port headers. |
| `KD_MSG_SIZE` | `iexec.inc:250` | 32 | A | ABI — message size is part of the cross-task IPC contract. |
| `REPLY_PORT_NONE` | `iexec.inc:253` | 0xFFFF | A | Sentinel value, not a cap. |
| `WAITPORT_NONE` | `iexec.inc:191` | 0xFF | A | Sentinel value, not a cap. |
| `USER_PT_BASE` | `iexec.inc:275` | 0x700000 | A | Fixed slot of the user page-table region, sized to `MAX_TASKS * USER_SLOT_STRIDE`. |
| `USER_SLOT_STRIDE` | `iexec.inc:279` | 0x10000 | A | Per-task slot stride — bounded by the layout of code/stack/data pages inside one slot. |
| `USER_DYN_BASE` / `USER_DYN_END` | `iexec.inc:341–342` | 0x800000 / 0x2000000 | A | Dynamic VA range — bounded by the IE64 user address space layout. |
| `USER_DYN_PAGES` | `iexec.inc:343` | 768 | B | Per-allocation cap for `AllocMem`, not per-task. Documents the largest single chunk a task can ask for. Replaceable; leaving for M12.6 audit. |
| `ALLOC_POOL_BASE` | `iexec.inc:323` | 0x800 | A | First allocable physical page — bounded by where the kernel binary, kernel data, kernel stack, and user-PT region end. |
| `ALLOC_POOL_PAGES` | `iexec.inc:324` | 6144 | A | Bounded by `MMU_NUM_PAGES − ALLOC_POOL_BASE`. |
| `KD_PAGE_BITMAP` / `KD_PAGE_BITMAP_SZ` | `iexec.inc:350–351` | 6080 / 800 bytes | A | Bitmap size derived from `ALLOC_POOL_PAGES` rounded to whole bytes. Layout-derived, not arbitrary. |
| `KD_REGION_MAX` | `iexec.inc:360` | 8 | C | **REMOVED IN M12.5.** Arbitrary per-task region count. On the critical path for MMIO/shared-mapping-heavy services (`graphics.library`, `hardware.resource`, intuition.library), so removing it first proves the chain-allocator pattern *where it matters*. Replaced with a per-task region chain using `kern_chain_alloc_page` / `kern_chain_find_free_row`. |
| `KD_REGION_TASK_SZ` | `iexec.inc:361` | 128 | C | **REMOVED IN M12.5** alongside `KD_REGION_MAX` — the per-task fixed-stride block becomes a chain header. |
| `KD_REGION_STRIDE` | `iexec.inc:359` | 16 | A | Region row size, bounded by the row field layout. |
| `KD_SHMEM_MAX` | `iexec.inc:384` | 16 | C | Arbitrary system-wide shared-object cap, already bumped 8→16 in M12. **M12.6 sweep candidate.** |
| `KD_SHMEM_STRIDE` | `iexec.inc:383` | 16 | A | Shared-object row size, bounded by row layout. |
| `IEXEC_HEARTBEAT_INTERVAL` | `iexec.inc:398` | 64 | A | Tunable — debug-only kernel heartbeat tick rate, not a system cap. |
| `IMG_HEADER_SIZE` | `iexec.inc:408` | 32 | A | IE64PROG image format ABI. |
| `IMG_OFF_CODE_SIZE` cap | `iexec.inc:410` | 4096 | B | Per-image code section cap from M8. The image format itself doesn't require this — it's a load-time policy cap. M12.6 or later. |
| `IMG_OFF_DATA_SIZE` cap | `iexec.inc:411` | 4096 | B | Same — per-image data section cap. M12.6 or later. |
| `PROGTAB_ENTRY_SIZE` | `iexec.inc:415` | 24 | A | Program table row layout, bounded by row fields. |
| `PROGTAB_BOOT_COUNT` | `iexec.inc:427` | 3 | A | Number of programs auto-loaded at boot — this is a *configured policy*, not an arbitrary cap. The number is "the count of strict-boot services," currently 3 (console.handler, dos.library, Shell). |
| `TERM_IO_PAGE` | `iexec.inc:430` | 0xF0 | A | Hardware MMIO page address. |
| `DATA_ARGS_OFFSET` / `DATA_ARGS_MAX` | `iexec.inc:433–434` | 3072 / 256 | B | Per-program argument-passing layout inside the program data page. Tied to the M9 `SYS_EXEC_PROGRAM` ABI. M12.6 or later. |
| `DOS_MAX_FILES` | `iexec.inc:473` | 16 | C | Arbitrary RAM filesystem file count. **M12.6 sweep candidate.** |
| `DOS_NAME_LEN` | `iexec.inc:474` | 32 | A | Filesystem ABI — bounded by the in-table filename field. |
| `DOS_FILE_SIZE` | `iexec.inc:475` | 16384 | C | Arbitrary per-file slot size, already bumped 4096→8192→16384 in M12. **M12.6 sweep candidate.** The proper fix is a packed-heap allocator (attempted in M12, reverted; documented TODO). |
| `DOS_MAX_HANDLES` | `iexec.inc:476` | 8 | C | Arbitrary maximum simultaneous open file handles. **M12.6 sweep candidate.** |
| `INTUI_WIN_TITLE_H` | `iexec.inc:563` | 16 | A | Window title bar height in pixels — bounded by the embedded Topaz 8×16 font glyph height. |
| `INTUI_WIN_BORDER` | `iexec.inc:564` | 2 | A | Window bevel thickness — visual constant, not a cap. |
| `SIG_SYSTEM_MASK` | `iexec.inc:194` | 0xFFFF | A | Bit-field mask, ABI — system signals are bits 0-15, user signals 16-31, bounded by the 32-bit signal-word width. |

**Bucket C summary:** `MAX_TASKS`, `KD_PORT_MAX`, `KD_REGION_MAX` (+ `KD_REGION_TASK_SZ`), `KD_SHMEM_MAX`, `DOS_MAX_FILES`, `DOS_FILE_SIZE`, `DOS_MAX_HANDLES`. M12.5 removes exactly one (`KD_REGION_MAX` + `KD_REGION_TASK_SZ`); M12.6 removes the rest using the same chain-allocator pattern.

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

For bulk data transfer, use shared memory: the sender includes a share_handle in the message (from `AllocMem(MEMF_PUBLIC)`), and the receiver calls `MapShared` to access the shared region. This is the protected-Amiga equivalent of "I gave you a pointer."

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
    - find a free task slot
    - copy code/data into that slot's user pages
    - build the slot's page table
    - initialize the slot's TCB and PTBR entry
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

- `AllocMem` syscall (1): allocate private or shared pages. R1=size, R2=flags (MEMF_PUBLIC, MEMF_CLEAR), returns R1=VA, R2=err, R3=share_handle (if MEMF_PUBLIC). Page-granular (4 KiB minimum). Kernel manages physical page pool and per-task virtual address windows.
- `FreeMem` syscall (2): free allocated memory. R1=addr, R2=size (must round to the same page count as the original allocation; the allocator is page-granular). Unmaps pages from caller's page table. For shared mappings, decrements refcount; physical pages freed when last reference removed.
- `MapShared` syscall (4): map a shared memory region by handle. R1=share_handle (opaque capability from AllocMem MEMF_PUBLIC), returns R1=mapped VA, R2=err, R3=share_pages (page count of the share, returned so user-space services can clamp byte counts to the actual mapped size — added in M11). Validates handle nonce to reject stale/invalid handles.
- Physical page allocator: bitmap-based contiguous allocation from 6400-page pool ($700000-$1FFFFFF).
- Per-task dynamic VA windows: each task gets 3 MB (768 pages) at $800000+task_id*$300000 (M11 stride).
- MEMF_ flags: MEMF_PUBLIC (bit 0, classic Amiga), MEMF_CLEAR (bit 16, classic Amiga zero-fill).
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
- **Boot-time loader** (`load_program`): validates image header (magic, sizes, alignment, truncation check), finds free task slot, copies code and data to task pages, builds page table, initializes TCB. Validate-then-commit pattern. Not a syscall - kernel-internal only.
- **Standard startup preamble**: programs compute own base addresses via `GetSysInfo(CURRENT_TASK)` + slot arithmetic. No startup arguments in registers (GPRs don't survive initial dispatch). Data page starts with image data section at offset 0.
- **4 bundled user-space services**:
- **CONSOLE**: creates public "CONSOLE" port, prints own ONLINE banner directly, loops receiving messages and printing data0 as chars. Text output service - all programs send through CONSOLE.
- **ECHO**: finds CONSOLE, announces online, creates "ECHO" port, allocates shared memory with greeting string, waits for request, replies with share_handle.
- **CLOCK**: finds CONSOLE, announces online, polls tick_count via GetSysInfo, sends '.' to CONSOLE every 32 ticks. Polling-based (no timer syscall yet).
- **CLIENT**: finds CONSOLE and ECHO, announces online, sends request to ECHO, receives share_handle in reply, `MapShared`, then sends "SHARED: " + greeting chars to CONSOLE.
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
- **`validate_user_range` subroutine**: walks the caller's page table and checks both **P (present) AND U (user-accessible)** bits for every page in `[ptr, ptr+size)`. Rejects unmapped pages, kernel-only pages, and overflow ranges with `ERR_BADARG`. Used by `SYS_EXEC_PROGRAM` to validate the image and args byte ranges before passing them to `load_program`.
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
readme                          0028
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
- **`USER_DYN_PAGES` bumped from 256 to 768** (`USER_DYN_STRIDE` 1 MB → 3 MB). Required for graphics.library to simultaneously map 1 chip + 300 VRAM + 300 surface pages = 601 pages. The 8x3MB layout uses VA `0x800000-0x2000000` — the top of the 32 MB VA space. Double-buffering (2 client surfaces) does not fit in 768 pages and is deferred to M12.
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
  - Surface: client owns the buffer (allocated with `MEMF_PUBLIC`), passes the share_handle in `GFX_REGISTER_SURFACE`. graphics.library does `MapShared` and caches the mapped VA. Single surface for M11 (VA budget — see kernel section). `surface_handle = 1`.
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
6. `MapShared(client_window_buffer)` → cached `win_mapped_va`

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
- **dos.library packed-heap file storage.** Attempted in M12, reverted, captured as a TODO.

**5.13.7 What this milestone proves.** intuition.library shipping as a pure user-space service — with no new syscalls, no graphics.library protocol changes, and no privileged code added — validates the M11.5 "exec boundary" thesis: the post-M11 nucleus is rich enough to host an Amiga-shaped windowing system without growing the syscall surface. The structural cap cleanup proves that those caps were placeholder ceilings, not architectural limits — the kernel data area easily absorbed all of them. Together this gets IntuitionOS to "visibly Amiga-shaped" while keeping the privileged surface defensible.
