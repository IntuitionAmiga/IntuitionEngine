# IE64 bare-metal C toolchain ABI

## Scope

This is the frozen v1 ABI for `ie64-unknown-none`. It produces one flat
`.ie64` image for direct execution at `PROG_START` (`0x1000`). It is neither
the IntuitionOS ABI in `IE64_ABI.md` nor a hosted C environment. There is no
ELF, relocation format, dynamic linking, system call dependency, formatted
I/O, thread support, locale or SIMD ABI.

The architectural sources are `IE64_ISA.md` and `architecture.md`. The SDK
assembly convenience includes are not ABI authority.

## Data layout

The ABI is LP64 and little-endian. `char` is signed. `_Bool`, `char`, signed
and unsigned `char` have size/alignment 1; `short` 2/2; `int` 4/4; `long` and
`long long` 8/8; pointers 8/8; `float` 4/4; `double` 8/8; and `wchar_t` is
unsigned 32-bit. Enums use `int`. `long double` and C `_Atomic` are rejected.

An aggregate uses its largest member alignment, capped at 8, and rounds its
size up to that alignment. Bit-fields use cproc's recorded allocation rules;
the compiler test suite owns executable layout examples.

## Calls

`R1` through `R6` carry integer and pointer arguments. `F0` through `F7`
carry scalar floating arguments on an independent cursor. A `double` rounds
that cursor to an even register and consumes the pair. An unavailable float or
double is put in an 8-byte stack slot without advancing the floating cursor,
so a later float may still use `F7`.

Signed scalar integer values are sign-extended and unsigned values are
zero-extended to 64 bits in registers and complete stack slots. `_Bool` is
zero or one. A spilled float occupies the low four bytes of its slot; its high
four bytes are unspecified. A spilled double occupies all eight bytes.

Scalar results use `R1`, `F0`, or `F0:F1`. Aggregates always pass by address:
the caller makes an aligned temporary copy. Aggregate results use a hidden
result pointer in `R1`, shifting ordinary integer arguments to start at `R2`.

`R1` to `R17` and `R26` to `R30` are caller-saved. `R18` to `R25` and `F8` to
`F15` are callee-saved. `R0` remains zero and `R31` is the stack pointer.

The caller reserves and reclaims outbound slots. Immediately before `JSR`, SP
is 16-byte aligned. At entry the return address is `[R31]` and the first stack
argument is `[R31 + 8]`; slots increase in source order.

Named variadic parameters follow the normal cursors. Unnamed arguments are
stack-only, after default promotions. `va_list` starts at the first unnamed
slot. Scalar `va_arg` advances by its rounded ABI value size; aggregate
`va_arg` reads one pointer slot, copies the pointed-to aggregate to its C
result, and advances by eight bytes.

## Image and runtime

The ordinary driver link order is `crt0.s`, generated C units, supplied
assembly units, and `libie64c.s`. It emits `__ie64_heap_start` after all input.
The image must fit `[0x1000, 0x8F000)`. `[0x8F000, 0x9F000)` is a full
descending 64 KiB stack and `R31` begins at `0x9F000`. Bare-metal execution
therefore needs `CR_RAM_SIZE_BYTES >= 0x9F000`.

`crt0.s` establishes this stack, checks the RAM-size precondition, calls
`main`, and halts on return. On failed precondition it sets `R1` to one and
halts before `main`. `-nostdlib` supplies no CRT or library: its input must
provide executable code at `PROG_START` and any required checks itself.

Linkable units have no `org` that resets the cursor to `PROG_START`; a forward
`org` is allowed only below `0x8F000`. The driver invokes `ie64asm -Werror`.

## Headers and library

The SDK supplies `assert.h`, `stdbool.h`, `stddef.h`, `stdint.h`, `stdarg.h`,
`stdalign.h`, `stdnoreturn.h`, `limits.h`, `ctype.h`, `string.h`, `stdlib.h`,
and `ie64.h`. The partial C library exports exactly the declarations in the
last three headers. `ctype` arguments must be representable as `unsigned char`.
`strto*` supports C11 bases and `endptr`; range errors return the applicable
limit and there is no `errno`.

The allocator is an 8-byte-aligned bump allocator over
`[__ie64_heap_start, 0x8F000)`. Zero-size allocation and allocation failure
return null. `calloc` detects multiplication overflow. `realloc` preserves the
lesser old/new byte count; `free` never releases storage.

`assert` evaluates once and halts on false; `NDEBUG` suppresses evaluation.

## Machine facilities

`ie64.h` exposes private `__builtin_ie64_*` spellings. cproc must validate
their operand types and immediate-only operands, and QBE's IE64 target alone
must lower them. They are not normal function calls or inline assembly.
Privileged operations remain explicit. The raw atomic wrappers take an aligned
`volatile uint64_t *`, return the ISA old value, retain ISA full-barrier
behaviour and faults, and intentionally define no C memory-order API.

| Facility | Builtin family | Instruction family |
| --- | --- | --- |
| Control, TLB, privilege | `mfcr`, `mtcr`, `tlbinval`, `tlbflush`, `sua*`, `eret`, `rti` | MFCR, MTCR, TLB*, SUA*, ERET, RTI |
| Atomics | `cas`, `xchg`, `faa`, `fand`, `for`, `fxor` | CAS, XCHG, FAA, FAND, FOR, FXOR |
| FPU special operations | `fmovecr`, `fmod`, `dmod`, `fint`, `dint`, `fcvt*`, `fmov*` | matching FPU opcodes |
| Interrupt and control flow | `sei`, `cli`, `rti`, `nop`, `halt`, `wait`, `syscall`, `smode` | SEI, CLI, RTI, NOP, HALT, WAIT, SYSCALL, SMODE |

`wait`, `syscall`, FPU constant loads, control-register identifiers and all
other immediate-only operands must be compile-time constants.
