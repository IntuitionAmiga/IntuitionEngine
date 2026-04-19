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
; Bootstrap launch table
; ============================================================================
; Rows use the same 40-byte shape as the runtime manifest rows in kernel data.
; PTR/SIZE are populated from hostfs at boot. NAME points at the canonical
; IntuitionOS runtime path used for diagnostics and internal matching.
boot_manifest_name_console:
    dc.b    "L/console.handler", 0
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

boot_manifest_table:
    dc.l    BOOT_MANIFEST_ID_CONSOLE
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
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
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_shell
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_HWRES
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_hwres
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_INPUT
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_input
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_GRAPHICS
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_graphics
    dc.l    0
    dc.w    0
    dc.w    0

    dc.l    BOOT_MANIFEST_ID_INTUITION
    dc.l    1                           ; strict/fatal boot
    dc.q    0
    dc.q    0
    dc.q    boot_manifest_name_intuition
    dc.l    0
    dc.w    0
    dc.w    0
