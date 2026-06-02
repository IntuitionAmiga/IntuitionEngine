; aot_runtime_stubs.inc - link stubs for the standalone COMPILE runtime blob.
;
; ehbasic_exec.inc's statement dispatch references the hardware, file, coprocessor,
; host and NEW handlers. The standalone COMPILE runtime bundles exec.inc only
; for its arithmetic/variable/string statement helpers (exec_do_let/print/for/next/
; read/...); the referenced hardware/coprocessor/host handlers would pull in the
; entire graphics/audio/video tree, so they are NOT bundled.
;
; LIST/SAVE are now genuinely bundled (line_list + detokenize + exec_do_save via
; ehbasic_lineeditor.inc + ehbasic_file_io.inc), so their real handlers replace the
; former stubs. exec_do_load/exec_do_bload come with file_io but stay rejected at
; transpile (LOAD reconstructs a tokenised programme, which needs the resident
; tokeniser and a REPL to run); their sole un-bundled dependency, tokenize, is the
; inert stub below, so the blob links and a stray LOAD/BLOAD is a no-op.
;
; The standalone transpiler lowers hardware statements through its own native
; handlers (POKE/CALL/SOUND PLAY/...) and rejects coprocessor/host/NEW/LOAD at
; compile time, so none of the remaining stub symbols is reached at runtime in a
; standalone image. They exist only to let the blob link; each is inert.
;
; If a future change routes a standalone statement to one of these through the
; bundled stmt_jump_table, it would hit an inert stub (a no-op rts) rather than the
; real handler. The standalone delegation allowlist in ehbasic_aot.inc must keep
; these tokens off the delegation path; the per-statement tests guard that.

; Coprocessor expression functions (COCALL/COSTATUS rejected at transpile).
fn_cocall:
    rts
fn_costatus:
    rts

; Coprocessor / host / NEW statement handlers (rejected at transpile).
exec_do_costart:
    rts
exec_do_costop:
    rts
exec_do_cowait:
    rts
exec_do_host:
    rts
line_new:
    rts

; tokenize: the resident line tokeniser, the sole un-bundled dependency of the
; bundled exec_do_load/exec_do_bload. LOAD/BLOAD are rejected at transpile, so this
; is never reached; it exists only to let ehbasic_file_io.inc link in the blob.
tokenize:
    rts

; Hardware statement handlers (lowered natively in standalone, never delegated).
hw_screen:
    rts
hw_cls:
    rts
hw_plot:
    rts
hw_palette:
    rts
hw_vsync:
    rts
hw_locate:
    rts
hw_color:
    rts
hw_line:
    rts
hw_circle:
    rts
hw_box:
    rts
hw_scroll:
    rts
hw_sound:
    rts
hw_envelope:
    rts
hw_gate:
    rts
hw_wait:
    rts
hw_copper:
    rts
hw_blit:
    rts
hw_ula:
    rts
hw_ted_cmd:
    rts
hw_antic:
    rts
hw_gtia:
    rts
hw_voodoo:
    rts
hw_psg:
    rts
hw_sid:
    rts
hw_pokey:
    rts
hw_ahx:
    rts
hw_sap:
    rts
hw_zbuffer:
    rts
hw_vertex:
    rts
hw_triangle:
    rts
hw_texture:
    rts
