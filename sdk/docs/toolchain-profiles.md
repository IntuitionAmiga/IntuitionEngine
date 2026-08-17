# Host SDK toolchain profiles

The Linux x86-64 and ARM64 Host SDKs supply `ie64-cproc` and its IE64 runtime.
They also ship assembly includes and examples for the other CPUs. External
compilers and their standard libraries are never bundled.

| CPU | Supported compiler | Required target definition | Output |
| --- | --- | --- | --- |
| IE64 | `ie64-cproc` | injected by the driver | `.ie64` flat image |
| M68K | GCC or VBCC | `-DIE_TARGET_M68K=1` | source-owned flat layout |
| Z80 | VBCC | `-DIE_TARGET_Z80=1` | source-owned flat layout |
| 6502 | SDCC or VBCC | `-DIE_TARGET_6502=1` | source-owned flat layout |
| x86 | GCC | `-DIE_TARGET_X86=1` | source-owned flat layout |

Every external compiler must supply `<stdint.h>` with correctly sized
`uint8_t`, `uint16_t` and `uint32_t` types before including
`intuitionengine.h`. IE64 additionally requires `uint64_t`. The 6502 linker
configuration files are `share/cc65/ie65.cfg`, `share/cc65/ie65_bindata.cfg`
and `share/cc65/ie65_service.cfg`, for example `ld65 -C share/cc65/ie65.cfg`.

Each example owns its startup source, entry symbol, entry address and linker
layout. Compile and link it with the selected toolchain, load it in the
matching Intuition Engine profile, and check its documented terminal or MMIO
result. There is no common CPU load address.

IE32 is assembly-only: use `ie32asm` with `include/ie32.inc`. It has no C
compiler, C header branch, linker or runtime. `m68kto64`, cc65, SDCC, VBCC,
GCC, VASM, vlink, NASM, EmuTOS and AROS toolchains are not SDK payloads.
