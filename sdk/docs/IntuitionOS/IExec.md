# IExec.library -- Kernel Contract Reference

## Amiga Exec-Inspired Protected Microkernel for IE64

### (c) 2024-2026 Zayn Otley -- Intuition Engine Project

---

## 1. Overview

IExec.library is a protected microkernel for the IE64 CPU, inspired by AmigaOS Exec but designed from the ground up for a hardware-enforced privilege model. Where Amiga Exec ran in flat supervisor space with no memory protection, IExec uses the IE64 MMU to enforce user/supervisor separation, per-task page tables, and W^X memory policy.

**What IExec does (Milestone 4):**

- Task scheduling (preemptive round-robin between two static tasks; priority-based scheduling is future)
- Memory protection via the IE64 MMU (per-task page tables with separate code/stack/data mappings)
- Trap and interrupt dispatch (syscall entry, fault handling, timer preemption)
- Context switching (save/restore PC, USP, and PTBR per task; full GPR save/restore is future)
- Inter-task signalling: per-task 32-bit signal mask with AllocSignal/FreeSignal/Signal/Wait, deadlock detection
- Inter-task communication via message ports: CreatePort/PutMsg/GetMsg/WaitPort with fixed-size 16-byte messages and SIGF_PORT-based wakeup

**What IExec does not do:**

- Filesystem access (handled by DOS.library or host-side intercepts)
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
| Hardware I/O | `$0A0000-$0FFFFF` | 384 KB | Unmapped in M1 | Memory-mapped video, audio, timer registers |
| VRAM | `$100000-$5FFFFF` | 5 MB | Unmapped in M1 | Video framebuffer, tile maps, sprite data |
| User space | `$600000-$1FFFFFF` | 26 MB | User (per-task mapped) | Task code, data, stacks, heap, shared memory |

**Milestone 1 kernel page table**: Identity-maps pages 0-383 (`$000000-$17FFFF`) as supervisor-only (P|R|W|X, no U). I/O and VRAM regions at `$A0000+` are not mapped by the kernel — user-space drivers will gain access via `MapIO`/`MapVRAM` syscalls in a future milestone. User pages are only mapped in per-task page tables, not the kernel PT.

### 2.2 Kernel Memory Layout (Detail)

Within the supervisor region:

| Sub-region | Address | Size | Contents |
|------------|---------|------|----------|
| Vector table | `$000000-$000FFF` | 4 KB | Reserved (IE64 hardware vectors) |
| Kernel code | `$001000-$00FFFF` | 60 KB | Kernel text (boots at `$1000`) |
| Kernel page table | `$010000-$01FFFF` | 64 KB | 8192 PTEs x 8 bytes |
| Kernel data | `$020000-$02FFFF` | 64 KB | Scheduler state, TCB array |
| Task 0 page table | `$030000-$03FFFF` | 64 KB | Per-task page table |
| Task 1 page table | `$040000-$04FFFF` | 64 KB | Per-task page table |
| (additional PTs) | `$050000-$09EFFF` | 320 KB | Room for ~5 more task page tables |
| Kernel stack | `$09F000` (top) | 4 KB | Grows downward |

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

### 2.5 Shared Memory (Future)

Shared memory regions will be created via `AllocShared` and mapped into a second task's address space via `MapShared`. Both tasks see the same physical pages. The kernel tracks reference counts and unmaps on `FreeMem` or task exit.

---

## 3. Task Model

### 3.1 Task Creation

In Milestone 1, tasks are created statically at boot time. The kernel pre-builds TCBs and page tables for a fixed number of tasks before entering user mode. Dynamic `CreateTask` is planned for a future milestone.

### 3.2 Task Control Block (TCB)

**Milestone 1 (simplified)**: Each task is described by a minimal per-task record. GPR save/restore is not yet implemented — user tasks must reload their own registers after yield.

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
| 3 | `REMOVED` | Terminated; TCB pending cleanup |

### 3.4 Priority Scheduling

The scheduler maintains a ready queue ordered by priority. Within the same priority level, tasks are scheduled round-robin. The current implementation uses a simple two-task alternation (toggle between task 0 and task 1). Priority-based scheduling with arbitrary task counts is planned.

### 3.5 Context Switch Sequence

On a context switch (triggered by `Yield` syscall or timer preemption):

1. **Save current task**: Read `FAULT_PC` and `USP` from control registers; store into the current task's TCB along with any dirty GPRs
2. **Select next task**: Advance the scheduler (currently: toggle `current_task` between 0 and 1)
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

**Current limitation (IExec M2):** The kernel does not yet save or restore R1-R30 in the TCB. The syscall dispatch logic clobbers R10-R16 internally. User code must assume all registers except R1, R2, and SP are unreliable after a syscall.

**Target:** A future IExec milestone will save and restore the full GPR set (R1-R30) in the TCB on every context switch, at which point callee-saved registers (R13-R15 per the [IE64 ABI v0](../IE64_ABI.md)) will be reliably preserved across syscalls.

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
| 7 | `ERR_TOOLARGE` | Message or allocation too large |

---

## 5. Syscall Table

### 5.1 Memory Management

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 1 | `AllocMem` | R1=size, R2=flags -> R1=addr | Future |
| 2 | `FreeMem` | R1=addr, R2=size -> R2=err | Future |
| 3 | `AllocShared` | R1=size, R2=flags -> R1=handle | Future |
| 4 | `MapShared` | R1=handle, R2=addr_hint -> R1=addr | Future |

`AllocMem` flags: bit 0 = zero-fill, bit 1 = align to page boundary. The allocator manages user-space pages, updating the calling task's page table.

### 5.2 Task Management

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 5 | `CreateTask` | R1=entry_pc, R2=stack_size, R3=priority -> R1=task_handle | Future |
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
| 15 | `CreatePort` | (no args) -> R1=port_id (0-3), R2=err | **Implemented** |
| 16 | `FindPort` | R1=name_ptr -> R1=port_handle | Future |
| 17 | `PutMsg` | R1=port_id, R2=msg_type, R3=msg_data -> R2=err | **Implemented** |
| 18 | `GetMsg` | R1=port_id -> R1=msg_type, R2=msg_data, R3=err | **Implemented** |
| 19 | `WaitPort` | R1=port_id -> R1=msg_type, R2=msg_data, R3=err | **Implemented** |
| 20 | `ReplyMsg` | R1=port_handle, R2=msg_ptr, R3=msg_len -> R2=err | Future |
| 21 | `PeekPort` | R1=port_handle -> R1=msg_count | Future |

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
| 28 | `MapIO` | R1=io_base, R2=size -> R1=mapped_addr | Future |
| 29 | `MapVRAM` | R1=vram_base, R2=size -> R1=mapped_addr | Future |
| 30 | `Debug` | R1=debug_op, R2=arg -> R1=result | Future |

### 5.8 Debug I/O

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 33 | `DebugPutChar` | R1=character -> R2=err | **Implemented** |

### 5.9 Bulk IPC

| # | Name | Signature | Status |
|---|------|-----------|--------|
| 31 | `SendMsgBulk` | R1=port_handle, R2=shmem_handle, R3=offset, R4=len -> R2=err | Future |
| 32 | `RecvMsgBulk` | R1=port_handle, R2=shmem_handle, R3=offset, R4=buf_len -> R1=actual_len | Future |

### Implemented Syscall Details

**Yield (26)**: Voluntarily relinquishes the CPU. The trap handler saves the current task's PC and USP, selects the next ready task, restores its state, and returns via ERET. If only one task is ready, Yield returns immediately.

**GetSysInfo (27)**: Queries kernel state. The `info_id` in R1 selects what to return:

| info_id | Name | Returns | Status |
|---------|------|---------|--------|
| 0 | SYSINFO_TOTAL_PAGES | Total pages in system | Future |
| 1 | SYSINFO_FREE_PAGES | Free pages available | Future |
| 2 | SYSINFO_TICK_COUNT | Kernel tick count (incremented on each timer interrupt) | **Implemented** |
| 3 | SYSINFO_CURRENT_TASK | Current task index | Future |

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

Messages are copied by the kernel. `PutMsg` copies up to 4 KB from the sender's address space into a kernel buffer and enqueues it. `GetMsg` copies the next message from the kernel buffer into the receiver's address space. This copy-based model avoids shared-memory complexity for small messages.

For bulk data transfer, `SendMsgBulk` and `RecvMsgBulk` use shared memory handles instead of copying, allowing zero-copy transfer of large buffers between tasks that have mapped the same shared region.

### 7.3 Reply Protocol

The reply pattern follows Amiga convention:
1. Sender calls `PutMsg` to a target port
2. Receiver calls `GetMsg` or `WaitPort` + `GetMsg`
3. Receiver processes the message, then calls `ReplyMsg` to the sender's reply port
4. Sender waits on its reply port's signal bit

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

The kernel boots in supervisor mode at `$1000` (PROG_START) and performs the following steps:

```
 1. MOVE R1, #trap_handler_addr
    MTCR CR_TRAP_VEC, R1           ; Set trap vector (syscall + fault entry)

 2. MOVE R1, #intr_handler_addr
    MTCR CR_INTR_VEC, R1           ; Set interrupt vector (timer preemption)

 3. MOVE R1, #$9F000
    MTCR CR_KSP, R1                ; Set kernel stack pointer

 4. Build kernel page table at $10000:
    - Identity-map pages 0-383 ($000000-$17FFFF) as P|R|W|X (supervisor only)
    - Identity-map pages 384-1535 ($180000-$5FFFFF) as P|R|W (VRAM, supervisor only)

 5. Build per-task page tables at $30000, $40000, ...:
    - Copy kernel supervisor mappings (pages 0-1535)
    - Add user-accessible pages for each task's code (P|R|X|U),
      stack (P|R|W|U), and data (P|R|W|U)

 6. MOVE R1, #$10000
    MTCR CR_PTBR, R1               ; Set page table base register
    MOVE R1, #1
    MTCR CR_MMU_CTRL, R1           ; Enable MMU

 7. Program hardware timer:
    MTCR CR_TIMER_PERIOD, #period  ; Timer period (ticks per quantum)
    MTCR CR_TIMER_COUNT, #period   ; Initial count
    MTCR CR_TIMER_CTRL, #3         ; Enable timer + enable interrupts

 8. Initialize TCBs for each task (PC, USP, PTBR, state=READY)

 9. Switch to task 0's page table:
    MTCR CR_PTBR, task0_ptbr
    TLBFLUSH

10. Set user entry point and stack:
    MTCR CR_USP, task0_stack_top
    MTCR CR_FAULT_PC, task0_entry

11. ERET                            ; Enter user mode at task 0's entry point
```

After ERET, the kernel only runs in response to traps (SYSCALL, page faults) and interrupts (timer).

---

## 11. Exec Lineage

How IExec maps to (and diverges from) classic Amiga Exec:

| Amiga Exec Concept | IExec Equivalent | What Changed |
|--------------------|------------------|--------------|
| Flat supervisor space, no MMU | Per-task page tables, MMU-enforced user/supervisor | Hardware protection; tasks cannot corrupt each other or the kernel |
| `AddTask()` with `tc_SPLower`/`tc_SPUpper` | `CreateTask` with kernel-allocated pages | Stack is a mapped page region, not a raw pointer range |
| `Signal()`/`Wait()` with 32-bit mask | Same API, same 32-bit mask | Unchanged -- the model is perfect as-is |
| `MsgPort` + `Message` (linked list in shared memory) | Kernel-managed ports with copy-in/copy-out | No shared-memory message headers; kernel copies payload for safety |
| `AllocMem()`/`FreeMem()` from memory pools | `AllocMem` backed by page allocator + per-task mapping | Returns virtual addresses; kernel manages physical page pool |
| `Exec->ThisTask` | `FindTask(0)` or `GetTaskInfo` | No direct pointer to TCB from user space |
| `Forbid()`/`Permit()` (disable scheduling) | Not provided | Tasks cannot disable preemption; kernel controls scheduling |
| `Disable()`/`Enable()` (disable interrupts) | Not available to user mode | Only kernel uses `CLI64`/`SEI64` internally |
| `SysBase` at address 4 | No equivalent | Kernel is not addressable from user space |
| `OpenLibrary()`/`CloseLibrary()` | Not provided (future: user-space shared libraries) | No jump table / library base mechanism yet |
| Device I/O (`DoIO`/`SendIO`) | `MapIO`/`MapVRAM` + direct register access | User tasks access hardware registers through mapped pages |
| `tc_Node.ln_Pri` scheduling | Priority field in TCB, same semantics | Round-robin within same priority level |

---

## 12. Milestone Status

### Milestone 1: Boot + Preemptive Multitasking (Complete)

**Implemented and tested:**

- Standalone kernel binary (`make intuitionos` assembles `sdk/intuitionos/iexec/iexec.s`)
- Self-sufficient boot: kernel builds its own page tables, creates user tasks, and initializes all scheduler state — no host-side setup required
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

- Boot banner: kernel prints "IExec M2 boot\n" to TERM_OUT before entering user mode
- `DebugPutChar` syscall (33): write a single character to the debug terminal (TERM_OUT at `$F0700`)
- Visible demo tasks: task 0 prints 'A', task 1 prints 'B', both yield and delay in a loop — output shows interleaved letters confirming preemptive multitasking
- Fault reporting: on non-SYSCALL faults, kernel prints "FAULT cause=NNNN PC=$XXXX ADDR=$XXXX\n" to the debug terminal then halts
- Scheduler heartbeat: prints '.' to the debug terminal every 64 timer ticks

### Milestone 3: Signals (Complete)

**Implemented and tested (builds on Milestone 2):**

- `AllocSignal` syscall (11): allocate a signal bit from the user range (bits 16-31); R1=bit hint (-1 for auto-assign), returns R1=allocated bit number, R2=err
- `FreeSignal` syscall (12): release a previously allocated signal bit; R1=bit to free, returns R2=err
- `Signal` syscall (13): send signals to another task; R1=target task ID, R2=signal mask — sets bits in target's pending signal word, wakes a WAITING target if signals match its wait mask
- `Wait` syscall (14): block until matching signals arrive; R1=signal mask, blocks the calling task (state transitions to WAITING), returns R1=received signals when woken
- Per-task signal state: `sig_alloc` (allocated bit mask), `sig_wait` (wait mask), `sig_recv` (pending/received signals), task state (READY/RUNNING/WAITING)
- Scheduler skips tasks in WAITING state; shared restore path delivers Wait return values when `sig_wait != 0`
- Deadlock detection: when all tasks are in WAITING state with no external wake source, kernel prints "DEADLOCK: no runnable tasks" and halts

### Milestone 4: Message Ports (Current)

**Implemented and tested (builds on Milestone 3):**

- `CreatePort` syscall (15): creates an anonymous message port owned by the calling task. Returns R1=port ID (0-3), R2=err. Returns ERR_NOMEM if all 4 port slots are occupied.
- `PutMsg` syscall (17): send a fixed-size message to a port. R1=port ID, R2=msg_type, R3=msg_data. Any task may send to any valid port. Enqueues a 16-byte message (4 bytes type, 4 bytes src_task, 8 bytes data) into the port's FIFO. Sets SIGF_PORT (signal bit 0) on the owning task and wakes the owner if it is in WAITING state. Returns R2=ERR_BADARG if the port ID is invalid, R2=ERR_AGAIN if the FIFO is full (4 messages max).
- `GetMsg` syscall (18): dequeue a message from a port. R1=port ID. Returns R1=msg_type, R2=msg_data, R3=err. Only the port owner may call GetMsg (returns R3=ERR_PERM otherwise). Returns R3=ERR_AGAIN if the FIFO is empty.
- `WaitPort` syscall (19): blocking receive from a port. R1=port ID. Returns R1=msg_type, R2=msg_data, R3=err. Owner only (R3=ERR_PERM otherwise). Semantics: if messages are available, dequeues and returns immediately. Otherwise calls Wait(SIGF_PORT) and rechecks on wake -- this loop handles spurious wakes from other ports sharing the same signal bit.
- Fixed-size message model: all messages are 16 bytes (4-byte type, 4-byte source task ID, 8-byte data payload). Register-based ABI with no memory-to-memory copy.
- Port capacity: 4 ports per system, 4-message FIFO per port.
- SIGF_PORT wakeup: PutMsg sets signal bit 0 on the port owner, integrating with the existing Signal/Wait infrastructure from Milestone 3.

### Milestone 5: Dynamic Tasks (Planned)

- `CreateTask` / `DeleteTask` syscalls
- Dynamic page allocation (`AllocMem` / `FreeMem`)
- Priority-based scheduling with arbitrary task count
- Full GPR save/restore in TCB on context switch

### Milestone 6: Named Ports + Reply Protocol (Planned)

- `FindPort` / `ReplyMsg` / `PeekPort`
- Named port registry
- Reply port convention for request/response patterns

### Milestone 7: Shared Memory + Bulk IPC (Planned)

- `AllocShared` / `MapShared` for zero-copy bulk transfer
- `SendMsgBulk` / `RecvMsgBulk`
- Reference-counted shared memory regions

### Milestone 8: Timers + Handles (Planned)

- `AddTimer` / `RemTimer` with delta queue
- `MapIO` / `MapVRAM` for user-space hardware access
- Unified handle table (`CloseHandle` / `DupHandle`)
- `Debug` syscall for kernel diagnostics
