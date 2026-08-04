# IE64 bare-metal C ABI V3

This document defines the `ie64-unknown-none` C ABI used by the V3 static
toolchain. The canonical instruction and machine contracts are
[`IE64_ISA.md`](IE64_ISA.md) and [`architecture.md`](architecture.md). Those
documents take precedence if this ABI document conflicts with the machine.

V3 retains the documented LP64, little-endian data model and calling convention
for the freestanding compiler suite. It does not reserve an image-base register and
does not add an IntuitionOS runtime contract.

## Objects and images

Compilation produces ELF64 little-endian `ET_REL` objects with machine value
`EM_IE64` (`0x4945`) and `e_flags` equal to `EF_IE64_ABI_V3` (`3`). Relocation
records use RELA addends. V3 defines `R_IE64_NONE`, `R_IE64_ABS64`,
`R_IE64_ABS32`, `R_IE64_PC32`, `R_IE64_LO32` and `R_IE64_HI32`. Relocation
number 1 remains reserved for the existing IntuitionOS relative relocation.

The static linker emits a flat image whose entry and first text byte are at
`0x1000`. The `baremetal-low` profile reserves `[0x8f000,0x9f000)` for the
stack, places the heap between BSS and `0x8f000`, and requires visible RAM to
reach at least `0x9f000`. File-backed data and BSS must remain below the stack
reservation.

The V3 linker has no synthetic fixed interrupt section. Interrupt handlers
are ordinary linked functions. A program that needs a programmable timer
handler must establish the documented MMU, CR7 interrupt vector and ERET path
before enabling the timer.

## Startup and shutdown

`_start` validates visible RAM, clears BSS, sets the stack to `0x9f000`, runs
Picolibc preinitialisers and initialisers, calls `main`, and passes its result
to `exit`. Finalisers and registered exit handlers run exactly once. The
low-level termination hook receives the original status in `R1` and halts.

## Library and platform contract

The sysroot uses Picolibc with process-global `errno`; it is not thread-safe.
The reclaiming allocator uses the linker-defined heap interval. C atomic
objects other than `atomic_flag` use the ABI lock table in `libatomic.a`: 64
aligned 64-bit locks indexed by `(object_address >> 3) & 63`. All supported
memory orders are conservatively strengthened to sequential consistency.

Applications may override the weak functions declared by
`<ie64/platform.h>`. File handles and stream cookies are unsigned 64-bit
values. Hooks return zero on success or a positive `errno` value on failure
and leave output arguments unchanged on failure. The default file hooks and,
because the architecture has no stable monotonic clock interface, the default
monotonic hook return `ENOSYS`.

The default console hooks use canonical terminal MMIO. The default termination
hook preserves its status and halts. V3 intentionally provides no `fdopen` or
`fileno` interface because the platform ABI has handles rather than a file
descriptor namespace.
