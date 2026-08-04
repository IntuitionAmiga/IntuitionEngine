# Host SDK hardware surface catalogue

This catalogue binds the public C surface to the maintained assembly includes.
`sdk/docs/architecture.md` is the canonical definition of access semantics.
IE32 remains assembly-only.

| Family | Assembly names | Public C prefix | C targets |
| --- | --- | --- | --- |
| Program and bank map | `PROGRAM_START`, `BANK*_WINDOW`, `*_REG` | `IE_PROGRAM_*`, `IE_BANK*` | IE64, M68K, Z80, 6502, x86 as applicable |
| Terminal and input | `TERM_*`, `SCAN_*`, `MOUSE_*` | `IE_INPUT_*` | IE64, M68K, Z80, 6502, x86 |
| Time and system | `RTC_*`, system vectors | `IE_SYSTEM_*` | IE64, M68K, Z80, 6502, x86 |
| Video and blitter | `VIDEO_*`, `COPPER_*`, `BLT_*` | `IE_VIDEO_*` | IE64, M68K, Z80, 6502, x86 |
| Audio | audio engine registers | `IE_AUDIO_*` | IE64, M68K, Z80, 6502, x86 |
| Files and host services | file MMIO and open flags | `IE_FILE_*` | IE64, M68K, Z80, 6502, x86 |
| Networking | `HOST_SOCKET_*` | `IE_NET_*` | IE64, M68K, Z80, 6502, x86 |
| Coprocessors | coprocessor mailbox | `IE_COPROC_*` | IE64, M68K, Z80, 6502, x86 |
| Voodoo | Voodoo aperture and commands | `IE_VOODOO_*` | IE64, M68K, Z80, 6502, x86 |
| IE64 control, atomics and FPU | IE64 ISA | `IE64_CR_*`, `ie64_*` | IE64 only |
| x86 ports | x86 I/O instructions | `ie_x86_*` | x86 only |

Whenever an assembly include or the architecture reference changes, update this
catalogue and the corresponding selected-target declaration in
`intuitionengine.h` in the same change.
