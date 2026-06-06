; aot_runtime_blob.asm - standalone COMPILE runtime blob.
;
; This assembles the IE64 BASIC expression/variable/string/maths/exec runtime as a
; single position-fixed blob, linked at AOT_RT_BASE. COMPILE bundles the trimmed
; blob into standalone .ie64 images and the bootstrap copies it to AOT_RT_BASE
; before running user code, so the blob's internal absolute references (la/lea to
; its own tables, RNG state, error strings, stmt_jump_table) resolve to their link
; addresses, and its references to the fixed BASIC memory map (variables, string
; heap, MMIO) are valid anywhere.
;
; The first bytes of the blob are a jump table (fixed-order dc.q entries) that the
; transpiler calls or derefs through by fixed address (RT_* constants in ie64.inc),
; so the generated code is decoupled from the blob's internal layout.
;
; ehbasic_exec.inc is bundled for the exec_do_* statement helpers (LET/PRINT/FOR/
; NEXT/READ/...). Its hardware, file, coprocessor, host and LIST/NEW handlers are
; not bundled (they pull in the whole hardware/video/audio tree); they are provided
; as inert stubs in aot_runtime_stubs.inc. The standalone transpiler lowers those
; statements natively or rejects them, so the stubs are never reached at runtime.
;
; Build/trim/staleness guard: TestAOTRuntimeBlob in
; ehbasic_aot_runtime_blob_test.go assembles this unit, trims the [PROG_START,
; AOT_RT_BASE) origin padding so byte 0 maps to AOT_RT_BASE, and checks the result
; matches the committed sdk/include/aot_runtime_blob.bin and fits the placement
; budget.

include "ie64.inc"
include "ehbasic_tokens.inc"

    org AOT_RT_BASE

; --- Jump table (stable ABI; order must match the RT_* equs in ie64.inc) -------
aot_rt_jump_table:
    dc.q    expr_eval               ; RT_EXPR_EVAL
    dc.q    var_lookup              ; RT_VAR_LOOKUP
    dc.q    var_lookup_tag          ; RT_VAR_LOOKUP_TAG
    dc.q    var_read                ; RT_VAR_READ
    dc.q    var_parse_name          ; RT_VAR_PARSE_NAME
    dc.q    arr_find                ; RT_ARR_FIND
    dc.q    arr_create              ; RT_ARR_CREATE
    dc.q    arr_dim                 ; RT_ARR_DIM
    dc.q    svar_lookup             ; RT_SVAR_LOOKUP
    dc.q    svar_read               ; RT_SVAR_READ
    dc.q    str_eval                ; RT_STR_EVAL
    dc.q    str_alloc               ; RT_STR_ALLOC
    dc.q    var_init                ; RT_VAR_INIT
    dc.q    raise_error             ; RT_RAISE_ERROR
    dc.q    exec_do_for             ; RT_EXEC_DO_FOR
    dc.q    exec_do_next            ; RT_EXEC_DO_NEXT
    dc.q    stmt_jump_table         ; RT_STMT_JUMP_TAB  (pointer to the table)
    dc.q    if_else_boundary        ; RT_IF_ELSE_BOUND  (pointer to the global)
    dc.q    exec_do_let             ; RT_EXEC_DO_LET
    dc.q    exec_do_print           ; RT_EXEC_DO_PRINT
    dc.q    exec_do_dim             ; RT_EXEC_DO_DIM
    dc.q    exec_do_read            ; RT_EXEC_DO_READ
    dc.q    exec_do_restore         ; RT_EXEC_DO_RESTORE
    dc.q    exec_do_input           ; RT_EXEC_DO_INPUT
    dc.q    exec_do_list            ; RT_EXEC_DO_LIST
    dc.q    exec_do_save            ; RT_EXEC_DO_SAVE
    dc.q    exec_do_bload           ; RT_EXEC_DO_BLOAD
    dc.q    exec_reset_control_stack ; RT_EXEC_RESET_CONTROL_STACK
    dc.q    expr_truthy             ; RT_EXPR_TRUTHY
    dc.q    expr_to_i64             ; RT_EXPR_TO_I64

; --- Runtime closure -----------------------------------------------------------
include "ie64_fp.inc"
include "ehbasic_expr.inc"
include "ehbasic_vars.inc"
include "ehbasic_strings.inc"
include "ehbasic_io.inc"
include "ehbasic_exec.inc"
; Bundled for standalone LIST/SAVE/BLOAD: line_list + print_uint32 (lineeditor) and
; detokenize + exec_do_save + exec_do_bload + uint32_to_buf (file_io). LIST/SAVE walk/
; serialise the tokenised programme the bootstrap copies to AOT_RT_PROG; BLOAD reads raw
; bytes through the File I/O MMIO (FILE_NAME_PTR/FILE_DATA_PTR/FILE_CTRL), the same path
; the interpreter uses. exec_do_load still needs the resident tokeniser and stays rejected
; at transpile; its only un-bundled dependency, tokenize, is an inert stub in
; aot_runtime_stubs.inc (exec_do_bload does not call it).
include "ehbasic_lineeditor.inc"
include "ehbasic_file_io.inc"
include "aot_runtime_stubs.inc"

expr_to_i64:
    jsr     fp_int
    jsr     fp_fix
    move.q  r9, #VAL_I64
    rts
