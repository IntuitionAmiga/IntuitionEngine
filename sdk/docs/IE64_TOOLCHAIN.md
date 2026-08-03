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
the compiler test suite owns executable layout examples. For example,
`struct { unsigned int a:3, b:5, c:8; }` has size and alignment 4. Its fields
occupy the low 16 bits of one 32-bit allocation unit in declaration order.

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

`ie64-cproc -o program.ie64 source.c source2.c routine.s` compiles and links
one flat image. It accepts `-I`, `-D`, `-E`, `-S`, `-c`, `-o`, and
`-nostdlib`. `-S` and `-c` both produce linkable IE64 assembly, not object
files. With several C inputs they write one `.s` beside each source; a single
`-o` is rejected. `-E`, `-S`, and `-c` accept only C inputs. Final links accept
C and linkable assembly inputs.

The development driver resolves its executable and searches that directory
and each parent for a sibling `IntuitionEngine/sdk`. It never derives the SDK
from the current directory. The first matching sibling supplies these files:

| Mode | Required SDK paths |
| --- | --- |
| `-E`, `-S`, `-c` | `include/` |
| final `-nostdlib` link | `include/`, `bin/ie64asm` |
| ordinary final link | `include/`, `bin/ie64asm`, `lib/ie64-cproc/crt0.s`, `lib/ie64-cproc/libie64c.s` |

An ordinary link validates all required paths before starting a compiler
subprocess. `-nostdlib` never requires either runtime file. Installed layout
discovery is not part of v1.

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
last three headers:

- `string.h`: `memchr`, `memcmp`, `memcpy`, `memmove`, `memset`, `strcat`,
  `strchr`, `strcmp`, `strcpy`, `strlen`, `strncat`, `strncmp`, and `strncpy`.
- `ctype.h`: `isalnum`, `isalpha`, `isblank`, `iscntrl`, `isdigit`, `isgraph`,
  `islower`, `isprint`, `ispunct`, `isspace`, `isupper`, `isxdigit`, `tolower`,
  and `toupper`.
- `stdlib.h`: `abs`, `labs`, `llabs`, `strtol`, `strtoul`, `strtoll`,
  `strtoull`, `qsort`, `bsearch`, `malloc`, `calloc`, `realloc`, and `free`.

The remaining standard headers provide their corresponding freestanding types
and macros only. There is no formatted I/O, environment, locale, file, thread,
process or hosted termination interface. `ctype` arguments must be
representable as `unsigned char`. `strto*` supports C11 bases and `endptr`;
range errors return the applicable limit and there is no `errno`.

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
| FPU special operations | `fmovecr`, `fmod`, `dmod`, `fint`, `dint`, `fcvt*`, `fsin`, `fcos`, `ftan`, `fatan`, `flog`, `fexp`, `fpow`, double equivalents, `fsqrt`, `dsqrt`, `fmov*` | matching FPU opcodes |
| Interrupt and control flow | `sei`, `cli`, `rti`, `nop`, `halt`, `wait`, `syscall`, `smode` | SEI, CLI, RTI, NOP, HALT, WAIT, SYSCALL, SMODE |

`wait`, `syscall`, FPU constant loads, control-register identifiers and all
other immediate-only operands must be compile-time constants.

### FPU lowering table

Every scalar FPU opcode has one defined compiler route. The following
operations arise from ordinary C or QBE lowering:

| IE64 opcodes | C or QBE operation |
| --- | --- |
| `FMOV`, `DMOV` | scalar copy |
| `FLOAD`, `DLOAD`, `FSTORE`, `DSTORE` | scalar object load or store |
| `FADD`, `DADD`, `FSUB`, `DSUB`, `FMUL`, `DMUL`, `FDIV`, `DDIV` | C arithmetic |
| `FNEG`, `DNEG` | unary minus |
| `FCMP`, `DCMP` | C floating comparison |
| `FCVTIF`, `DCVTIF`, `FCVTFI`, `DCVTFI` | integer and floating conversion |
| `FMOVI`, `FMOVO` | QBE bit-preserving scalar cast |

Operations without an ordinary C expression use the private interface below.
The argument codes are `u32`, `u64`, `f32`, `f64`, and `ptr64`. An `imm`
argument must be a compile-time integer constant.

| C builtin | Arguments | Private QBE operation | IE64 instruction |
| --- | --- | --- | --- |
| `__builtin_ie64_fmovecr` | `imm 0..15` | `ie64fmovecr` | `FMOVECR` |
| `__builtin_ie64_dmovecr` | `imm 0..15` | `ie64dmovecr` | `FMOVECR`, `FCVTSD` |
| `__builtin_ie64_fmod` | `f32, f32` | `ie64fmod` | `FMOD` |
| `__builtin_ie64_dmod` | `f64, f64` | `ie64dmod` | `DMOD` |
| `__builtin_ie64_fabs` | `f32` | `ie64fabs` | `FABS` |
| `__builtin_ie64_dabs` | `f64` | `ie64dabs` | `DABS` |
| `__builtin_ie64_fint` | `f32` | `ie64fint` | `FINT` |
| `__builtin_ie64_dint` | `f64` | `ie64dint` | `DINT` |
| `__builtin_ie64_fcvtsd` | `f32` | `ie64fcvtsd` | `FCVTSD` |
| `__builtin_ie64_fcvtds` | `f64` | `ie64fcvtds` | `FCVTDS` |
| `__builtin_ie64_fsin`, `__builtin_ie64_dsin` | `f32` or `f64` | `ie64fsin`, `ie64dsin` | `FSIN`, `DSIN` |
| `__builtin_ie64_fcos`, `__builtin_ie64_dcos` | `f32` or `f64` | `ie64fcos`, `ie64dcos` | `FCOS`, `DCOS` |
| `__builtin_ie64_ftan`, `__builtin_ie64_dtan` | `f32` or `f64` | `ie64ftan`, `ie64dtan` | `FTAN`, `DTAN` |
| `__builtin_ie64_fatan`, `__builtin_ie64_datan` | `f32` or `f64` | `ie64fatan`, `ie64datan` | `FATAN`, `DATAN` |
| `__builtin_ie64_flog`, `__builtin_ie64_dlog` | `f32` or `f64` | `ie64flog`, `ie64dlog` | `FLOG`, `DLOG` |
| `__builtin_ie64_fexp`, `__builtin_ie64_dexp` | `f32` or `f64` | `ie64fexp`, `ie64dexp` | `FEXP`, `DEXP` |
| `__builtin_ie64_fpow`, `__builtin_ie64_dpow` | two `f32` or two `f64` | `ie64fpow`, `ie64dpow` | `FPOW`, `DPOW` |
| `__builtin_ie64_fsqrt`, `__builtin_ie64_dsqrt` | `f32` or `f64` | `ie64fsqrt`, `ie64dsqrt` | `FSQRT`, `DSQRT` |
| `__builtin_ie64_fmovsr` | none | `ie64fmovsr` | `FMOVSR` |
| `__builtin_ie64_fmovcr` | none | `ie64fmovcr` | `FMOVCR` |
| `__builtin_ie64_fmovsc` | `u32` | `ie64fmovsc` | `FMOVSC` |
| `__builtin_ie64_fmovcc` | `u32` | `ie64fmovcc` | `FMOVCC` |

### Non-FPU builtin lowering table

| C builtin | Arguments | Private QBE operation | IE64 instruction |
| --- | --- | --- | --- |
| `__builtin_ie64_mfcr` | `imm 0..15` | `ie64mfcr` | `MFCR` |
| `__builtin_ie64_mtcr` | `imm 0..15, u64` | `ie64mtcr` | `MTCR` |
| `__builtin_ie64_tlbinval` | `u64` | `ie64tlbinval` | `TLBINVAL` |
| `__builtin_ie64_tlbflush` | none | `ie64tlbflush` | `TLBFLUSH` |
| `__builtin_ie64_suaen`, `__builtin_ie64_suadis` | none | `ie64suaen`, `ie64suadis` | `SUAEN`, `SUADIS` |
| `__builtin_ie64_eret`, `__builtin_ie64_rti` | none | `ie64eret`, `ie64rti` | `ERET`, `RTI` |
| `__builtin_ie64_nop`, `__builtin_ie64_halt` | none | `ie64nop`, `ie64halt` | `NOP`, `HALT` |
| `__builtin_ie64_sei`, `__builtin_ie64_cli` | none | `ie64sei`, `ie64cli` | `SEI`, `CLI` |
| `__builtin_ie64_wait` | `imm u32` | `ie64wait` | `WAIT` |
| `__builtin_ie64_syscall` | `imm u32` | `ie64syscall` | `SYSCALL` |
| `__builtin_ie64_smode` | none | `ie64smode` | `SMODE` |
| `__builtin_ie64_xchg` | `ptr64, u64` | `ie64xchg` | `XCHG` |
| `__builtin_ie64_faa` | `ptr64, u64` | `ie64faa` | `FAA` |
| `__builtin_ie64_fand` | `ptr64, u64` | `ie64fand` | `FAND` |
| `__builtin_ie64_for` | `ptr64, u64` | `ie64for` | `FOR` |
| `__builtin_ie64_fxor` | `ptr64, u64` | `ie64fxor` | `FXOR` |
| `__builtin_ie64_cas` | `ptr64, u64, u64` | `ie64cas` | `CAS` |
