# Intuition Engine host SDK

This archive is the supported Linux host SDK for the architecture named in the
download. The x86-64 and ARM64 archives have the same layout and contents
apart from their host executable architecture. Its top level contains
only `bin`, `include`, `lib` and `share`.

`bin` contains the supported first-party tools: `ie32asm`, `ie32to64`,
`ie64asm`, `ie64dis`, `ie64-cproc`, `ie64ld`, `ie64-ar`, `ie64-ranlib`, `qbe`
and `cproc-qbe`.

Include `intuitionengine.h` for C hardware access and select one documented
`IE_TARGET_*` definition. The IE64 driver selects `IE_TARGET_IE64` itself and
rejects user target selection. IE32 is assembly-only and uses `ie32.inc`.

The bundled IE64 runtime and freestanding standard headers are supplied for
`ie64-cproc`; its driver adds the private target-header directory automatically.
External M68K, Z80, 6502 and x86 compilers and their libraries
are not bundled. See `share/docs/toolchain-profiles.md` for the supported matrix.
