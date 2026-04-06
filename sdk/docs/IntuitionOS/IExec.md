# IExec.library -- Kernel Contract Reference

## Amiga Exec-Inspired Protected Microkernel for IE64

### (c) 2024-2026 Zayn Otley -- Intuition Engine Project

---

## 1. Overview

IExec.library is a protected microkernel for the IE64 CPU, inspired by AmigaOS Exec but designed from the ground up for a hardware-enforced privilege model. Where Amiga Exec ran in flat supervisor space with no memory protection, IExec uses the IE64 MMU to enforce user/supervisor separation, per-task page tables, and W^X memory policy.

**What IExec does (Milestone 10):**

- Preemptive round-robin scheduling across up to 8 dynamic tasks (CreateTask/ExitTask with slot reuse)
- Memory protection via the IE64 MMU (per-task page tables with separate code/stack/data mappings, W^X enforcement)
- Dynamic memory allocation: AllocMem/FreeMem with Amiga-style MEMF_ flags (MEMF_PUBLIC, MEMF_CLEAR), page-granular physical page pool (6400 pages / 25 MB), per-task VA windows
- Shared memory via MEMF_PUBLIC with opaque capability handles (MapShared), reference-counted cleanup on task exit
- Trap and interrupt dispatch (syscall entry, fault handling with privilege split, timer preemption)
- Context switching (save/restore PC, USP, PTBR, and full GPR set per task)
- Inter-task signalling: per-task 32-bit signal mask with AllocSignal/FreeSignal/Signal/Wait, deadlock detection
- Named public MsgPorts with CreatePort(name, PF_PUBLIC) and FindPort discovery (case-insensitive, up to 8 ports)
- Request/reply messaging: 32-byte messages with data0, data1, reply_port, and share_handle fields
- ReplyMsg for Exec-style service pattern; PutMsg/GetMsg/WaitPort with full message ABI
- Safe user pointer validation (PTE check before kernel reads from user memory)
- Kernel renamed to exec.library; GURU MEDITATION fault messages
- AmigaOS-style OpenLibrary syscall for library discovery
- I/O page mapping via MapIO syscall (REGION_IO type)
- **`SYS_EXEC_PROGRAM` takes a user-space image pointer** (M10): kernel creates tasks from user-provided IE64PROG images. Runs entirely under the caller's PT (no PT switching). `validate_user_range` checks both P and U bits on every page in the requested range. Legacy index path retained for M9 compatibility.
- **Multi-page code and data in `load_program`** (M10): up to 2 code pages (8 KB) and 4 data pages (16 KB) per task. dos.library is itself a 2-code-page program, with 3 data pages containing embedded command images.
- SYS_READ_INPUT for kernel-mode terminal read
- console.handler: CON: handler with GetMsg polling and CON_READLINE protocol
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
| 1 | `AllocMem` | R1=size, R2=flags -> R1=addr, R2=err, R3=share_handle | **Implemented** |
| 2 | `FreeMem` | R1=addr, R2=size -> R2=err | **Implemented** |
| 3 | `AllocShared` | R1=size, R2=flags -> R1=handle | Reserved |
| 4 | `MapShared` | R1=share_handle -> R1=addr, R2=err, R3=share_pages | **Implemented (M11 extended)** |

`AllocMem` flags: MEMF_PUBLIC (bit 0) = shareable across tasks, MEMF_CLEAR (bit 16) = zero-fill. Matches classic Amiga MEMF_ conventions. All allocations are page-granular (4 KiB). `FreeMem` size is also page-granular: the kernel rounds size up to pages and compares the page count against the allocation's page count. Any size that rounds to the same number of pages is accepted (e.g., both 5000 and 8192 free a 2-page allocation).

### 5.2 Task Management

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 5 | `CreateTask` | R1=source_ptr, R2=code_size, R3=arg0 -> R1=task_id, R2=err | **Implemented** |
| 6 | `DeleteTask` | R1=task_handle -> R2=err | Future |
| 7 | `FindTask` | R1=name_ptr (0=self) -> R1=task_handle | Future |
| 8 | `SetTaskPri` | R1=task_handle, R2=priority -> R1=old_pri | Future |
| 9 | `SetTP` | R1=value -> (sets thread pointer register) | Future |
| 10 | `GetTaskInfo` | R1=task_handle, R2=info_id -> R1=value | Future |

### 5.3 Signals

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 11 | `AllocSignal` | R1=bit_hint (-1=any) -> R1=bit_num, R2=err | **Implemented** |
| 12 | `FreeSignal` | R1=bit_num -> R2=err | **Implemented** |
| 13 | `Signal` | R1=task_id, R2=signal_mask -> R2=err | **Implemented** |
| 14 | `Wait` | R1=signal_mask -> R1=received_mask | **Implemented** |

### 5.4 Ports and Messages

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 15 | `CreatePort` | R1=name_ptr (0=anon), R2=flags -> R1=port_id (0-7), R2=err | **Implemented (M7)** |
| 16 | `FindPort` | R1=name_ptr -> R1=port_id, R2=err | **Implemented (M7)** |
| 17 | `PutMsg` | R1=port_id, R2=type, R3=data0, R4=data1, R5=reply_port, R6=share_handle -> R2=err | **Implemented (M7)** |
| 18 | `GetMsg` | R1=port_id -> R1=type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle | **Implemented (M7)** |
| 19 | `WaitPort` | R1=port_id -> (same as GetMsg, blocks if empty) | **Implemented (M7)** |
| 20 | `ReplyMsg` | R1=reply_port, R2=type, R3=data0, R4=data1, R5=share_handle -> R2=err | **Implemented (M7)** |
| 21 | `PeekPort` | R1=port_handle -> R1=msg_count | Future |

**Port naming (M7):** Ports can be created with a name (up to 16 bytes, ASCII) and the `PF_PUBLIC` flag. Public named ports are discoverable via `FindPort` (case-insensitive matching). Anonymous ports (name_ptr=0) are private and not findable. Duplicate public names return `ERR_EXISTS`. Ports are cleaned up on task exit (name and flags cleared, removed from FindPort).

**Message format (M7):** Messages are 32 bytes, kernel-copied, with type (4B), sender (4B), data0 (8B), data1 (8B), reply_port (2B, 0xFFFF=none), and share_handle (4B, 0=none). `ReplyMsg` is a convenience wrapper that sends to the reply_port with share_handle support.

**User pointer safety:** CreatePort and FindPort validate user-provided name pointers by checking PTEs before reading. Invalid pointers return `ERR_BADARG` instead of crashing the kernel.

### 5.5 Timers

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 22 | `AddTimer` | R1=ticks, R2=signal_mask -> R1=timer_handle | Future |
| 23 | `RemTimer` | R1=timer_handle -> R2=err | Future |

### 5.6 Handles

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 24 | `CloseHandle` | R1=handle -> R2=err | Future |
| 25 | `DupHandle` | R1=handle -> R1=new_handle | Future |

### 5.7 System

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 26 | `Yield` | (no args) -> (returns after reschedule) | **Implemented** |
| 27 | `GetSysInfo` | R1=info_id -> R1=value | **Implemented** |
| 28 | `MapIO` | R1=base_ppn, R2=page_count -> R1=mapped_va, R2=err | **Implemented (M9, extended in M11)** |
| 29 | `MapVRAM` | (subsumed by `MapIO` VRAM allowlist) | Subsumed |
| 30 | `Debug` | R1=debug_op, R2=arg -> R1=result | Future |

### 5.8 Debug I/O

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 33 | `DebugPutChar` | R1=character -> R2=err | **Implemented** |
| 34 | `ExitTask` | R1=exit_code (ignored) -> never returns | **Implemented** |

### 5.9 Bulk IPC

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 31 | `SendMsgBulk` | R1=port_handle, R2=shmem_handle, R3=offset, R4=len -> R2=err | Future |
| 32 | `RecvMsgBulk` | R1=port_handle, R2=shmem_handle, R3=offset, R4=buf_len -> R1=actual_len | Future |

### 5.10 Program Execution and Libraries (M9, ABI redesigned in M10)

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 35 | `ExecProgram` | R1=image_ptr, R2=image_size, R3=args_ptr, R4=args_len -> R1=task_id, R2=err | **Implemented (M9, redesigned M10)** |
| 36 | `OpenLibrary` | R1=name_ptr, R2=version -> R1=lib_base, R2=err | **Implemented (M9)** |
| 37 | `ReadInput` | R1=buf_ptr, R2=buf_size -> R1=bytes_read, R2=err | **Implemented (M9)** |

**ExecProgram (35)** -- M10 ABI: Creates a new task from a user-provided IE64PROG image. R1=image_ptr (user VA pointing to a complete IE64PROG image, e.g. an entry in dos.library's RAM file store; must be ≥ `0x600000`), R2=image_size (total bytes including 32-byte header, code, and data; valid range 32..24608, matching `load_program`'s max of header + 8 KiB code + 16 KiB data), R3=args_ptr (user VA pointing to null-terminated argument string in the caller's address space, or 0 for no args), R4=args_len (byte count of arguments, max 256, or 0 for no args). The handler runs entirely under the **caller's** page table (no PT switching): every page in `[image_ptr, image_ptr+image_size)` and `[args_ptr, args_ptr+args_len)` is validated via `validate_user_range` (checks both **P** and **U** PTE bits), then `load_program` is called directly to copy the image into a free task slot. Arguments are copied to the new task's data page at `DATA_ARGS_OFFSET` (like AmigaOS `pr_Arguments`). Returns R1=new task_id, R2=err. Returns `ERR_BADARG` for unmapped/kernel-only ranges, oversize images, args_len > 256, or pointer arithmetic overflow.

**Legacy index path** (M9 compatibility): If R1 < `0x600000`, the handler treats R1 as a program table index and uses the M9 ABI (R2=args_ptr, R3=args_len, R4 ignored). The program table walk + PT-switching path is preserved for this case, but **M10 hardens it** with the same `validate_user_range` check on the args range that the new ABI uses (the M9 path only checked the lower bound and would fault the kernel on unmapped or supervisor-only pages above `0x600000`). With M10 the program table only contains the 3 boot services (console.handler, dos.library, Shell), so legacy index access is effectively limited to those slots; production code uses the new pointer ABI exclusively. The legacy path will be removed in a future milestone.

**`validate_user_range` subroutine**: Walks the caller's page table once per page in the requested byte range. For each VPN, loads the PTE and checks `(pte & 0x11) == 0x11` (P bit + U bit set). Rejects unmapped pages, kernel-only pages, and pointer-arithmetic overflows. Returns R1=0 on success or 1 (ERR_BADARG) on any failure.

**OpenLibrary (36)**: AmigaOS-style library discovery. R1=name_ptr (library name, e.g., "dos.library"), R2=minimum version. Returns R1=library base (port ID or equivalent handle), R2=err.

**ReadInput (37)**: Kernel-mode terminal read. Reads input from the terminal device into a user buffer. R1=buf_ptr, R2=buf_size. Returns R1=bytes read, R2=err.

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
| `OpenLibrary()`/`CloseLibrary()` | `OpenLibrary` syscall (M9) for library discovery | Returns port ID / handle for named library; no jump table mechanism yet |
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
- **New syscalls**:
  - `SYS_MAP_IO` (28): maps I/O pages into user task address space with new REGION_IO (3) region type.
  - `SYS_EXEC_PROGRAM` (35): loads and starts a bundled program by table index (R1=index, R2=args_ptr, R3=args_len). Arguments are copied to the child task's data page at DATA_ARGS_OFFSET (like AmigaOS `pr_Arguments`). **Note**: ABI redesigned in M10 to take a user-space image pointer instead of an index.
  - `SYS_OPEN_LIBRARY` (36): AmigaOS-style `OpenLibrary` for library/service discovery by name.
  - `SYS_READ_INPUT` (37): kernel-mode terminal read for interactive input.
- **console.handler**: CON: handler task (Task 0). Creates public port, services output via GetMsg polling, supports CON_READLINE protocol for interactive line input.
- **dos.library**: AmigaOS dos.library equivalent (Task 1). Provides a RAM: filesystem with 16 files, 4 KB each, case-insensitive filenames. Supports DOS_RUN command dispatch for launching external commands.
- **Shell**: interactive command shell (Task 2). Reads input via console.handler, dispatches commands to dos.library via DOS_RUN. Displays `1> ` prompt (AmigaOS-style).
- **5 external commands**: VERSION, AVAIL, DIR, TYPE, ECHO. All are real user-space tasks loaded from the program table (no shell builtins). Arguments are passed via DATA_ARGS_OFFSET in the data page.

### Milestone 10: DOS-Loaded Programs + Assigns + Startup-Sequence (Complete)

**Implemented and tested (builds on Milestone 9):**

M10 transitions the system from kernel-dispatched command indices to user-space DOS-loaded executables. The kernel gets simpler (the program table walk is gone for on-demand programs); all new functionality lives in dos.library and the shell. The boot demo now looks like a self-booting AmigaOS-style protected microkernel, with dos.library executing `S:Startup-Sequence` automatically before dropping to the shell prompt.

**Kernel changes (gets simpler):**

- **`SYS_EXEC_PROGRAM` ABI redesigned**: now takes a user-space image pointer instead of a program table index. New signature: `R1=image_ptr (user VA, ≥0x600000), R2=image_size, R3=args_ptr, R4=args_len → R1=task_id, R2=err`. Runs entirely under the caller's PT (no PT switching). Legacy index-based path retained for `R1<0x600000` (M9 compat, used only by tests; production code uses the new ABI). The kernel no longer walks the program table for on-demand loads.
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

### Milestone 11: input.device + graphics.library + Fullscreen Demo (Current)

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

### Milestone 12: intuition.library + Compositor + Windowing (Planned)

- `intuition.library` on top of `graphics.library`
- Compositor and damage tracking
- Multiple windows per screen
- Multi-subscriber input fan-out via intuition's IDCMP
- Double buffering (requires either VA layout overhaul or kernel-mediated blit syscall)
- Optional: clean recovery from `graphics.library` crashes (chip-page reset hook)
