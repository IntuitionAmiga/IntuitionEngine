# IntuitionOS IE64 ABI v0

## 1. Scope and Status

This document defines the application binary interface for IE64 code running
under IntuitionOS. It covers register roles, function calling convention,
syscall convention, task entry/exit, trap preservation guarantees, TLS, and
binary interface assumptions.

This ABI is:
- stable for handwritten assembly and kernel/userland interfaces
- subject to one compatibility review before C compiler support

This ABI does not yet standardize:
- shared library calling convention
- position-independent code
- GOT/PLT conventions
- ELF or object file format details
- unwind table encoding
- aggregate (struct/union) passing rules
- variadic function rules

## 2. Basic Model

- 64-bit little-endian architecture
- Stack grows toward lower addresses
- Code and data are separated by normal MMU policy (W^X)
- User code runs in user mode; kernel is entered only by trap, syscall, or
  interrupt
- Fixed 8-byte instruction width

## 3. Register Roles

### 3.1 General-Purpose Registers

| Register   | Role                              | Preserved across calls? |
|------------|-----------------------------------|-------------------------|
| R0         | Hardwired zero                    | N/A                     |
| R1-R6      | Argument and scratch registers    | No (caller-saved)       |
| R1         | Primary return value              | No                      |
| R2         | Secondary return value / error    | No                      |
| R7-R12     | Caller-saved temporaries          | No (caller-saved)       |
| R13-R15    | Callee-saved registers            | Yes (callee-saved)      |
| R16-R30    | Unclassified in v0                | —                       |
| R31 / SP   | Stack pointer                     | Yes                     |

R0 reads as zero; writes are discarded.

R16-R30 are not assigned roles in ABI v0. Code must not assume they are
preserved or clobbered across external calls until a future ABI version
classifies them. Within a single compilation unit or handwritten module, authors
may use them freely as long as both caller and callee agree.

### 3.2 Control Registers

Control registers are not part of the function call ABI. User code must not
assume direct access to control registers except:

- **TP (CR6)**: readable from user mode via `MFCR Rd, CR6`. Provides the
  thread/task-local storage base pointer.

All other control registers require supervisor mode.

### 3.3 Floating-Point Registers

Scalar FP32 values use `f0`-`f15`.

Double-precision values use register pairs:
- `d0` = `f0:f1`
- `d1` = `f2:f3`
- `d2` = `f4:f5`
- `d3` = `f6:f7`
- `d4` = `f8:f9`
- `d5` = `f10:f11`
- `d6` = `f12:f13`
- `d7` = `f14:f15`

ABI convention:
- `d0`-`d3` are caller-saved.
- `d4`-`d7` are callee-saved.

Writing a scalar `f*` register destroys the corresponding half of any live
double in the enclosing pair. Writing a `d*` register destroys both halves.

## 4. Function Calling Convention

### 4.1 Argument Passing

- Arguments are passed left to right in R1 through R6.
- Additional arguments beyond the sixth are passed on the stack, in 8-byte
  slots, in left-to-right order at increasing addresses from the stack pointer.

### 4.2 Return Values

- Primary return value in R1.
- Secondary return value (or error code) in R2.

### 4.3 Caller and Callee Responsibilities

- The caller must save any caller-saved registers (R1-R12) it needs across a
  call.
- The callee must preserve all callee-saved registers (R13-R15) and SP.
- The callee must restore SP to its entry value before returning via `rts`.

### 4.4 Stack Alignment

- SP must be 16-byte aligned immediately before every `jsr`.
- SP must be 16-byte aligned at function entry (after the return address is
  pushed by `jsr`, SP will be misaligned by 8; the callee adjusts if needed).

### 4.5 Return Address

The return address is pushed to the stack by `jsr` and popped by `rts`. There
is no dedicated link register.

### 4.6 Leaf Functions

Leaf functions (functions that make no calls) may omit callee-saved register
saves if they do not modify those registers. There is no mandatory frame pointer.

## 5. Syscall Convention

The syscall ABI is separate from the function calling convention.

| Item               | Rule                                      |
|--------------------|-------------------------------------------|
| Instruction        | `SYSCALL #imm32`                          |
| Syscall number     | Encoded in the imm32 field                |
| Arguments          | R1-R6                                     |
| Primary return     | R1                                        |
| Error return       | R2 (0 = success)                          |
| Caller-saved regs  | May be clobbered by the kernel            |
| Callee-saved regs  | See note below                            |

**Current implementation (IExec M9 onwards):** The kernel's syscall dispatch
logic uses R10-R16 internally. Callee-saved registers (R13-R15) are **not
preserved** across syscalls — the kernel does not save or restore GPRs in
the TCB on the syscall entry path. (Timer interrupts DO save/restore the
full GPR set as of M9; this section is about explicit syscalls only.)

**ABI v0 target:** Once the kernel implements full GPR save/restore on the
syscall entry path, callee-saved registers (R13-R15) and SP will be
preserved across syscalls. Until then, user code must treat all registers
except R1, R2, and SP as potentially clobbered after a syscall.

After syscall return, R1 and R2 contain the return values. SP is preserved
(via automatic USP swap). All other registers must be assumed clobbered
until the kernel implements full GPR preservation in the syscall path.

C code must reach syscalls through wrapper functions that save any live
caller-saved state before issuing SYSCALL.

### 5.1 Error Convention

- R2 = 0 means success.
- R2 != 0 is an error code.
- R1 holds the result value when R2 = 0.

## 6. Task Entry ABI

When a new task begins execution:

| Register | Initial value                                     |
|----------|---------------------------------------------------|
| PC       | Task entry point                                  |
| SP       | Top of task user stack                             |
| TP (CR6) | Task-local block pointer, or 0 if unused           |
| R1       | Optional startup argument pointer                  |
| R2       | Optional startup argument value or count           |
| All other GPRs | Undefined unless explicitly documented       |

## 7. Task Exit ABI

A task must not return past its entry point. Falling off the end of a task
entry function is undefined behavior.

A task must exit by one of:
- The `ExitTask` syscall (#34) — implemented since M5
- `HALT` instruction (early bootstrap and demo code only)

## 8. Trap and Interrupt Preservation Contract

### 8.1 Syscall Return

After a syscall returns to user mode via `ERET`:
- SP is preserved (via automatic USP swap).
- R1 and R2 contain the syscall return values.
- All other registers: see the current-implementation note in section 5.
  Once the kernel implements full GPR preservation, callee-saved registers
  (R13-R15) will be reliably preserved and caller-saved registers (R1-R12)
  may be clobbered.

### 8.2 Interrupt Return

**Current implementation (IExec M9 onwards):** The kernel saves and restores
the full GPR set (R1-R31) on the user stack across timer interrupts. PC, USP,
PTBR, and all GPRs are preserved across preemption. Code that is preempted by
the timer resumes with its register state intact.

**ABI v0 rule:** All user-visible GPRs (R1-R31) are preserved across timer
interrupt preemption. User code does not need to defensively reload registers
after a yield point — the timer interrupt is transparent.

**Note:** The explicit syscall path (Section 5) is separate and still
clobbers R10-R16 internally. Only timer interrupts have full GPR preservation.
A future milestone may extend GPR preservation to the syscall path as well.

### 8.3 Fault Delivery

If a fault (page fault, privilege violation) is recoverable and the kernel
resumes user execution:
- The kernel re-executes the faulting instruction (PC is not advanced).
- Register state at the point of the fault is not modified by the fault
  delivery itself (the CPU does not clobber GPRs on trap entry).
- However, if fault handling triggers a context switch, the same GPR
  preservation limitations as section 8.2 apply.

If a fault is fatal, the task is terminated.

### 8.4 Supervisor Kernel-User Access Contract (M15.6)

The kernel runs with the `MMU_CTRL.SKAC` bit set at boot (see
`IE64_ISA.md` §12.2.1). Any supervisor-mode read or write on a page
with `PTE_U==1` faults with `FAULT_SKAC` unless the `MMU_CTRL.SUA`
latch is also set. `SUA` is mutated only by the privileged `SUAEN`
and `SUADIS` opcodes.

Supervisor code that must touch a user pointer therefore bracket
every user-memory access with `SUAEN` / `SUADIS`:

```
    suaen                       ; open the access window
    load.b  r3, (r1_user_ptr)
    store.b r3, (r2_kernel_ptr)
    suadis                      ; close the access window
```

Equivalently, kernel code calls the canonical usercopy helpers
`copy_from_user` / `copy_to_user` / `copy_cstring_from_user` in
`sdk/intuitionos/iexec/iexec.s`, which handle the bracketing and
all permission/MMU-validation internally. User-mode code is never
responsible for `SUAEN` / `SUADIS` — the opcodes are privileged.

Nested-trap discipline: on trap entry the `SUA` latch is saved into
the active trap frame's `cr14` (`CR_SAVED_SUA`) slot and then
forcibly cleared. A nested kernel handler therefore starts with
`SUA == 0` and must re-open its own window if it needs user access.
On `ERET`, the live latch is restored from the frame's saved value
(supervisor return) or cleared unconditionally (user return). The
trap-frame stack (see `IE64_ISA.md` §12.14) preserves
`CR_FAULT_PC` / `CR_FAULT_ADDR` / `CR_FAULT_CAUSE` / `CR_PREV_MODE`
/ `CR_SAVED_SUA` across nested traps automatically, so handlers
that were previously required to save and restore these CRs around
a possibly-faulting region no longer need to. Existing manual
save/restore code is redundant but harmless.

The `SUA` latch is supervisor-mode state and does not leak to user
code: user-mode `ERET` forces `SUA = 0` regardless of the
interrupted supervisor latch value.

## 9. TLS Convention

- TP (CR6) is reserved for thread/task-local storage.
- TP is readable from user mode via `MFCR Rd, CR6`.
- **Current implementation:** TP is not yet initialized by the kernel. CR6
  contains zero at task startup. The `SetTP` syscall (syscall #9) is listed as
  future in the IExec syscall table and is not yet implemented.
- **ABI v0 intent:** Once implemented, TP will be set by the kernel at task
  creation and will be stable for the lifetime of the task. TP will be
  per-task; if threading within a task is added in the future, TP becomes
  per-thread.
- User code must not rely on TP containing a valid pointer until the kernel
  initializes it. Code that needs TP today must check for zero.

## 10. Stack Frame Guidance

For handwritten assembly authors:

1. Save callee-saved registers (R13-R15) if you modify them.
2. Allocate locals by subtracting from SP.
3. Maintain 16-byte alignment before calling another function.
4. Restore SP exactly before `rts`.

A typical frame:

```
High addresses
  +---------------------------+
  | stack-passed arguments    |  (from caller, if any)
  +---------------------------+
  | return address            |  <- SP on entry
  +---------------------------+
  | saved callee-saved regs   |
  +---------------------------+
  | local storage             |
  +---------------------------+
  | outgoing stack arguments  |
  +---------------------------+  <- SP during function body
Low addresses
```

This layout is descriptive. The only mandatory invariants are:
- Return address is at entry SP.
- Callee-saved registers must be restored.
- SP alignment rules must hold.

## 11. Binary and Runtime Assumptions

- Endianness: little-endian throughout (instructions, data, immediates).
- Instruction width: 8 bytes, naturally aligned.
- Code entry: for flat binaries, the entry point is the first instruction at
  the load address (currently PROG_START = 0x1000).
- Executable image: flat binary loaded at a fixed address for v0. Relocation
  model is not yet defined.
- Object file format: not yet standardized. Current toolchain (ie64asm)
  produces flat binaries with no symbol or relocation information.

## 12. Versioning

This document defines **IntuitionOS IE64 ABI v0**.

Any incompatible change to:
- register roles
- stack alignment
- argument placement
- return value placement
- syscall convention
- task entry convention
- TLS convention

must define a new ABI version rather than silently updating v0.

Classification of R16-R30 in a future version is an additive change, not a
breaking change, provided it does not contradict v0 guarantees.
