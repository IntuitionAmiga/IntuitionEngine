
bootstrap_grant_table:
    ; Row 0: console.handler (manifest ID 10) — CHIP grant for PPN 0xF0
    dc.b    BOOT_MANIFEST_ID_CONSOLE, 0, 0, 0
    dc.l    HWRES_TAG_CHIP              ; 'CHIP' (little-endian uint32)
    dc.w    0xF0                        ; ppn_lo
    dc.w    0xF0                        ; ppn_hi
    dc.l    0                           ; reserved
    ; Sentinel
    dc.b    0xFF
    ds.b    15

; ============================================================================
; M14.1: bootstrap manifest seed table
; ============================================================================
; Seed rows use the same 40-byte shape as the runtime manifest rows in kernel
; data. PTR/SIZE reference canonical embedded strict-M14 ELF service blobs.
; NAME points at the internal path/name used by dos.library when matching a
; shipped service against the embedded manifest.
boot_manifest_name_console:
    dc.b    "console.handler", 0
    align   8
boot_manifest_name_doslib:
    dc.b    "dos.library", 0
    align   8
boot_manifest_name_shell:
    dc.b    "Shell", 0
    align   8
boot_manifest_name_hwres:
    dc.b    "RESOURCES/hardware.resource", 0
    align   8
boot_manifest_name_input:
    dc.b    "DEVS/input.device", 0
    align   8
boot_manifest_name_graphics:
    dc.b    "LIBS/graphics.library", 0
    align   8
boot_manifest_name_intuition:
    dc.b    "LIBS/intuition.library", 0
    align   8

    align   4096
boot_elf_console:
    incbin  "boot_console_handler.elf"
boot_elf_console_end:

    align   4096
boot_elf_shell:
    incbin  "boot_shell.elf"
boot_elf_shell_end:

    align   4096
boot_elf_hwres:
    incbin  "boot_hardware_resource.elf"
boot_elf_hwres_end:

    align   4096
boot_elf_input:
    incbin  "boot_input_device.elf"
boot_elf_input_end:

    align   4096
boot_elf_graphics:
    incbin  "boot_graphics_library.elf"
boot_elf_graphics_end:

    align   4096
boot_elf_intuition:
    incbin  "boot_intuition_library.elf"
boot_elf_intuition_end:

boot_manifest_seed_table:
    dc.l    BOOT_MANIFEST_ID_CONSOLE
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_console
    dc.q    boot_elf_console_end - boot_elf_console
    dc.q    boot_manifest_name_console
    dc.l    HWRES_TAG_CHIP
    dc.w    0xF0
    dc.w    0xF0

    dc.l    BOOT_MANIFEST_ID_DOSLIB
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_doslib
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_SHELL
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_shell
    dc.q    boot_elf_shell_end - boot_elf_shell
    dc.q    boot_manifest_name_shell
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_HWRES
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_hwres
    dc.q    boot_elf_hwres_end - boot_elf_hwres
    dc.q    boot_manifest_name_hwres
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_INPUT
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_input
    dc.q    boot_elf_input_end - boot_elf_input
    dc.q    boot_manifest_name_input
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_GRAPHICS
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_graphics
    dc.q    boot_elf_graphics_end - boot_elf_graphics
    dc.q    boot_manifest_name_graphics
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_INTUITION
    dc.l    1                           ; strict/fatal boot
    dc.q    boot_elf_intuition
    dc.q    boot_elf_intuition_end - boot_elf_intuition
    dc.q    boot_manifest_name_intuition
    dc.l    0
    dc.w    0
    dc.w    0
