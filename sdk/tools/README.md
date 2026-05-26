# Intuition Engine SDK Tools

Choose the folder that matches the computer where you will run the tools:

| Folder | Host platform |
| --- | --- |
| `linux-x64` | Linux on x86-64 |
| `linux-arm64` | Linux on ARM64 |
| `macos-x64` | macOS on Intel x86-64 |
| `macos-arm64` | macOS on Apple silicon |
| `windows-x64` | Windows on x86-64 |
| `windows-arm64` | Windows on ARM64 |

Windows tools use the `.exe` suffix. Linux and macOS tools do not.

From the `SDK/Tools` directory, you can verify copied tool binaries with:

```sh
sha256sum -c SHA256SUMS.txt
```

The checksum file covers the shipped binaries only.

## Shipped Tools

| Tool | Purpose |
| --- | --- |
| `ie32asm` | Assembles IE32 source. |
| `ie64asm` | Assembles IE64 source. |
| `ie64dis` | Disassembles IE64 binaries. |
| `ie32to64` | Transpiles IE32 assembly to IE64 assembly. |
| `m68kto64` | Transpiles supported M68K assembly to IE64 assembly. |

There is currently no shipped IE32 disassembler.

## Include Files

| SDK include file | Required tool |
| --- | --- |
| `ie32.inc` | Use shipped `ie32asm`. |
| `ie64.inc` and `ie64_fp.inc` | Use shipped `ie64asm`. |
| `ie68.inc` | Use `vasmm68k_mot` from vasm, <https://sun.hasenbraten.de/vasm/>. |
| `ie65.inc` plus `ie65.cfg`, `ie65_bindata.cfg`, or `ie65_service.cfg` | Use `ca65` and `ld65` from cc65, <https://cc65.github.io/>. |
| `ie80.inc` | Use `vasmz80_std` from vasm, <https://sun.hasenbraten.de/vasm/>. |
| `ie86.inc` | Use NASM, <https://nasm.us/>. |

External assemblers are named for convenience but are not bundled on `IESHARE`.
