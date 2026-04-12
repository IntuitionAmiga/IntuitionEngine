prog_shell:
    ; Header
    dc.l    0, 0
    dc.l    prog_shell_code_end - prog_shell_code
    dc.l    prog_shell_data_end - prog_shell_data
    dc.l    0
    ds.b    12
prog_shell_code:

    ; =====================================================================
    ; Preamble: compute data page base (preemption-safe double-check)
    ; =====================================================================
    sub     sp, sp, #16
.sh_preamble:
    load.q  r30, (sp)
    load.q  r29, 8(sp)
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; =====================================================================
    ; OpenLibrary("console.handler", 0) with retry
    ; =====================================================================
.sh_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = &data[0] = "console.handler"
    move.q  r2, r0                      ; version 0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .sh_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_open_con_retry
.sh_open_con_ok:
    store.q r1, 136(r29)               ; data[136] = console_port

    ; =====================================================================
    ; OpenLibrary("dos.library", 0) with retry
    ; =====================================================================
.sh_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16                ; R1 = &data[16] = "dos.library"
    move.q  r2, r0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .sh_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_open_dos_retry
.sh_open_dos_ok:
    store.q r1, 144(r29)               ; data[144] = dos_port

    ; =====================================================================
    ; CreatePort(anonymous, flags=0)
    ; =====================================================================
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 152(r29)               ; data[152] = reply_port

    ; =====================================================================
    ; AllocMem(0x1000, MEMF_PUBLIC | MEMF_CLEAR)
    ; =====================================================================
    move.l  r1, #0x1000
    move.l  r2, #0x10001               ; MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1=VA, R2=err, R3=share_handle
    load.q  r29, (sp)
    store.q r1, 160(r29)               ; data[160] = shared_buf_va
    store.l r3, 168(r29)               ; data[168] = shared_buf_handle

    ; =====================================================================
    ; Send "Shell ONLINE [Taskn]\r\n" banner
    ; =====================================================================
    add     r20, r29, #32              ; &data[32] = "Shell ONLINE [Task"
    jsr     .sh_send_string
    load.q  r29, (sp)
    ; task_id digit
.sh_ban_id_retry:
    load.q  r29, (sp)
    load.q  r3, 128(r29)
    add     r3, r3, #0x30               ; ASCII digit
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_id_full
    bra     .sh_ban_bracket
.sh_ban_id_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_id_retry
.sh_ban_bracket:
    ; ']'
.sh_ban_brk_retry:
    move.l  r3, #0x5D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_brk_full
    bra     .sh_ban_cr
.sh_ban_brk_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_brk_retry
.sh_ban_cr:
    ; '\r'
.sh_ban_cr_retry:
    move.l  r3, #0x0D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_cr_full
    bra     .sh_ban_lf
.sh_ban_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_cr_retry
.sh_ban_lf:
    ; '\n'
.sh_ban_lf_retry:
    move.l  r3, #0x0A
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_lf_full
    bra     .sh_ban_done
.sh_ban_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_lf_retry
.sh_ban_done:

    ; (Stale "IntuitionOS M11" banner removed: incorrect milestone label
    ;  and redundant — exec.library already prints the canonical version
    ;  banner. The data slot at offset 56 is preserved as padding to keep
    ;  the offsets of subsequent strings stable.)

    ; =====================================================================
    ; M10: Try to open and read S:Startup-Sequence
    ; =====================================================================
    load.q  r29, (sp)
    ; Initialize script_mode = 0, script_pos = 0, script_len = 0
    store.b r0, 176(r29)
    store.l r0, 184(r29)
    store.l r0, 188(r29)

    ; Write "S:Startup-Sequence" to shared buffer
    load.q  r14, 160(r29)             ; r14 = shared_buf_va
    add     r15, r29, #320             ; r15 = "S:Startup-Sequence" string source
    ; Inline string copy of "S:Startup-Sequence\0"
    move.l  r16, #0x53                 ; 'S'
    store.b r16, (r14)
    move.l  r16, #0x3A                 ; ':'
    store.b r16, 1(r14)
    move.l  r16, #0x53                 ; 'S'
    store.b r16, 2(r14)
    move.l  r16, #0x74                 ; 't'
    store.b r16, 3(r14)
    move.l  r16, #0x61                 ; 'a'
    store.b r16, 4(r14)
    move.l  r16, #0x72                 ; 'r'
    store.b r16, 5(r14)
    move.l  r16, #0x74                 ; 't'
    store.b r16, 6(r14)
    move.l  r16, #0x75                 ; 'u'
    store.b r16, 7(r14)
    move.l  r16, #0x70                 ; 'p'
    store.b r16, 8(r14)
    move.l  r16, #0x2D                 ; '-'
    store.b r16, 9(r14)
    move.l  r16, #0x53                 ; 'S'
    store.b r16, 10(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 11(r14)
    move.l  r16, #0x71                 ; 'q'
    store.b r16, 12(r14)
    move.l  r16, #0x75                 ; 'u'
    store.b r16, 13(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 14(r14)
    move.l  r16, #0x6E                 ; 'n'
    store.b r16, 15(r14)
    move.l  r16, #0x63                 ; 'c'
    store.b r16, 16(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 17(r14)
    store.b r0, 18(r14)                ; null

    ; Send DOS_OPEN(mode=READ=0)
    load.q  r29, (sp)
    move.l  r2, #DOS_OPEN              ; type
    move.q  r3, r0                      ; data0 = mode (READ=0)
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 144(r29)               ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; WaitPort for DOS_OPEN reply
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R1 = type (0=OK, nonzero=error), R2 = data0 (file handle if OK)
    bnez    r1, .sh_no_script           ; not found → skip startup
    move.q  r17, r2                     ; r17 = file handle (save)
    store.q r17, 8(sp)                 ; save handle to scratch

    ; Send DOS_READ(handle, max=512). The script buffer at data[368] is
    ; 512 bytes (M12.8: bumped from 256 to absorb the new boot ECHO line
    ; without truncating earlier startup commands). The shell's share is
    ; 4 KiB so the share clamp doesn't kick in.
    load.q  r29, (sp)
    move.l  r2, #DOS_READ              ; type
    load.q  r3, 8(sp)                  ; data0 = handle
    move.l  r4, #512                   ; data1 = max bytes
    load.q  r5, 152(r29)
    load.l  r6, 168(r29)
    load.q  r1, 144(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; WaitPort for DOS_READ reply
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R1=type, R2=data0=bytes_read
    bnez    r1, .sh_close_script       ; read error → close + skip
    move.q  r18, r2                    ; r18 = bytes_read
    store.l r18, 188(r29)              ; save script_len

    ; Copy script content from shared_buf_va to data[368] (script buffer)
    load.q  r14, 160(r29)             ; src
    add     r15, r29, #368             ; dst
    move.l  r16, #0
.sh_copy_script:
    bge     r16, r18, .sh_copy_script_done
    add     r19, r14, r16
    load.b  r20, (r19)
    add     r19, r15, r16
    store.b r20, (r19)
    add     r16, r16, #1
    bra     .sh_copy_script
.sh_copy_script_done:
    ; Set script_mode = 1
    move.l  r14, #1
    store.b r14, 176(r29)

.sh_close_script:
    ; Send DOS_CLOSE(handle)
    load.q  r29, (sp)
    move.l  r2, #DOS_CLOSE
    load.q  r3, 8(sp)                  ; handle
    move.q  r4, r0
    load.q  r5, 152(r29)
    load.l  r6, 168(r29)
    load.q  r1, 144(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

.sh_no_script:

    ; =====================================================================
    ; Script line reader: read next line from script buffer to line buffer
    ; (Defined before main loop so backward branch resolves)
    ; =====================================================================
.sh_script_line:
    load.q  r29, (sp)
    ; r14 = script_pos, r15 = script_len
    load.l  r14, 184(r29)
    load.l  r15, 188(r29)
    ; If script_pos >= script_len, end of script
    bge     r14, r15, .sh_script_done
    ; Copy bytes from script_buffer[script_pos] until '\n' or end
    add     r16, r29, #368             ; r16 = script buffer base
    add     r16, r16, r14              ; r16 = current read ptr
    add     r17, r29, #240             ; r17 = line buffer (dest)
    move.l  r18, #0                    ; chars copied
.sh_sl_copy:
    bge     r14, r15, .sh_sl_eol       ; reached script end
    move.l  r19, #126                  ; max line length
    bge     r18, r19, .sh_sl_eol
    load.b  r20, (r16)
    move.l  r19, #0x0A                 ; '\n'
    beq     r20, r19, .sh_sl_eol_inc
    store.b r20, (r17)
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r14, r14, #1
    bra     .sh_sl_copy
.sh_sl_eol_inc:
    add     r14, r14, #1               ; consume the '\n'
.sh_sl_eol:
    store.b r0, (r17)                  ; null-terminate line
    store.l r14, 184(r29)              ; save updated script_pos
    bra     .sh_line_ready

.sh_script_done:
    ; End of script: clear script_mode, fall through to interactive
    store.b r0, 176(r29)
    bra     .sh_main_loop

    ; =====================================================================
    ; Main loop
    ; =====================================================================
.sh_main_loop:
    load.q  r29, (sp)
    ; Check script mode
    load.b  r14, 176(r29)
    bnez    r14, .sh_script_line

    ; --- Interactive mode: print prompt + read line via console ---
    add     r20, r29, #80
    jsr     .sh_send_string

    ; Send CON_READLINE request to console.handler (with ERR_FULL retry)
.sh_readline_retry:
    load.q  r29, (sp)
    move.l  r2, #CON_MSG_READLINE       ; type
    move.q  r3, r0                      ; data0 = 0
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_readline_full
    bra     .sh_readline_sent
.sh_readline_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_readline_retry
.sh_readline_sent:

    ; WaitPort(reply_port) → R2=data0=byte_count
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R2 = byte_count
    store.q r2, 8(sp)                  ; save byte_count to scratch

    ; Copy line from shared_buf_va to data[240..366], null-terminate
    load.q  r14, 160(r29)             ; r14 = shared_buf_va (source)
    add     r15, r29, #240             ; r15 = &data[240] (dest)
    load.q  r16, 8(sp)                ; r16 = byte_count
    move.l  r17, #0                    ; counter
.sh_copy_line:
    bge     r17, r16, .sh_copy_done
    move.l  r18, #126
    bge     r17, r18, .sh_copy_done
    add     r19, r14, r17
    load.b  r20, (r19)
    add     r19, r15, r17
    store.b r20, (r19)
    add     r17, r17, #1
    bra     .sh_copy_line
.sh_copy_done:
    ; Null-terminate
    add     r19, r15, r17
    store.b r0, (r19)

    ; Strip trailing CR/LF from line buffer
    add     r15, r29, #240             ; r15 = &data[240]
.sh_strip_trail:
    beqz    r17, .sh_strip_done
    sub     r18, r17, #1
    add     r19, r15, r18
    load.b  r20, (r19)
    move.l  r21, #0x0D
    beq     r20, r21, .sh_strip_char
    move.l  r21, #0x0A
    beq     r20, r21, .sh_strip_char
    bra     .sh_strip_done
.sh_strip_char:
    store.b r0, (r19)
    move.q  r17, r18
    bra     .sh_strip_trail
.sh_strip_done:

    ; Strip prompt "1> " from line if present (GUI terminal includes it)
    add     r15, r29, #240
    load.b  r20, (r15)
    move.l  r21, #0x31                  ; '1'
    bne     r20, r21, .sh_no_prompt_strip
    load.b  r20, 1(r15)
    move.l  r21, #0x3E                  ; '>'
    bne     r20, r21, .sh_no_prompt_strip
    load.b  r20, 2(r15)
    move.l  r21, #0x20                  ; ' '
    bne     r20, r21, .sh_no_prompt_strip
    ; Line starts with "1> " — skip 3 chars
    add     r15, r29, #243              ; data[240+3]
    ; Copy remainder back to data[240] (shift left by 3)
    add     r14, r29, #240
.sh_prompt_copy:
    load.b  r20, (r15)
    store.b r20, (r14)
    beqz    r20, .sh_no_prompt_strip
    add     r15, r15, #1
    add     r14, r14, #1
    bra     .sh_prompt_copy
.sh_no_prompt_strip:
.sh_line_ready:

    ; If line is empty, re-prompt
    add     r15, r29, #240
    load.b  r20, (r15)
    beqz    r20, .sh_main_loop

    ; =====================================================================
    ; Parse first word: scan for space or null in line buffer
    ; =====================================================================
    add     r15, r29, #240             ; r15 = line buffer start
    move.q  r16, r15                   ; r16 = scan pointer
.sh_scan_word:
    load.b  r17, (r16)
    beqz    r17, .sh_word_end
    move.l  r18, #0x20                 ; space
    beq     r17, r18, .sh_word_end
    add     r16, r16, #1
    bra     .sh_scan_word
.sh_word_end:
    ; r15 = start of word, r16 = end of word (points to space or null)
    sub     r17, r16, r15              ; r17 = word length
    beqz    r17, .sh_main_loop

.sh_dispatch_command:

    ; =====================================================================
    ; M10: Name-based command dispatch via DOS_RUN
    ; =====================================================================
    ; Write "command_name\0args\0" to shared buffer, send DOS_RUN.
    ; r15 = line start, r16 = end of first word, r17 = word length.
    ; If word length == 0, skip (empty line handled above).

    ; 1. Copy command name to shared buffer at offset 0
    load.q  r14, 160(r29)             ; r14 = shared_buf_va
    move.q  r18, r15                   ; src = line start
    move.q  r19, r14                   ; dst = shared buf
    move.l  r20, #0
.sh_cp_cmd:
    bge     r20, r17, .sh_cp_cmd_done
    load.b  r21, (r18)
    store.b r21, (r19)
    add     r18, r18, #1
    add     r19, r19, #1
    add     r20, r20, #1
    bra     .sh_cp_cmd
.sh_cp_cmd_done:
    store.b r0, (r19)                  ; null-terminate command name
    add     r19, r19, #1               ; advance past null

    ; 2. Copy args (everything after first word + space) after null.
    ; If the shell has a selected current volume and DIR/LIST are invoked
    ; without explicit args, inject that volume token as the command arg.
    ; r16 = end of word (space or null)
    load.b  r21, (r16)
    beqz    r21, .sh_no_args
    add     r16, r16, #1               ; skip space
.sh_copy_explicit_args:
    move.l  r20, #0
.sh_cp_args:
    load.b  r21, (r16)
    store.b r21, (r19)
    beqz    r21, .sh_args_done
    add     r16, r16, #1
    add     r19, r19, #1
    add     r20, r20, #1
    move.l  r22, #DATA_ARGS_MAX
    blt     r20, r22, .sh_cp_args
    store.b r0, (r19)
.sh_args_done:
    bra     .sh_send_run

.sh_no_args:
    ; Bare "X:" token with no args updates the shell's current listing
    ; context and returns to the prompt.
    move.l  r21, #2
    blt     r17, r21, .sh_no_args_maybe_command
    sub     r18, r19, #2
    load.b  r20, (r18)
    move.l  r21, #0x3A                  ; ':'
    bne     r20, r21, .sh_no_args_maybe_command
    move.q  r18, r14
    add     r21, r29, #192
.sh_set_current_volume:
    load.b  r20, (r18)
    store.b r20, (r21)
    beqz    r20, .sh_main_loop
    add     r18, r18, #1
    add     r21, r21, #1
    bra     .sh_set_current_volume

.sh_no_args_maybe_command:
    move.q  r18, r14
    load.b  r20, (r18)
    move.l  r21, #0x44                  ; 'D'
    beq     r20, r21, .sh_maybe_inject_dir
    move.l  r21, #0x64                  ; 'd'
    beq     r20, r21, .sh_maybe_inject_dir
    move.l  r21, #0x4C                  ; 'L'
    beq     r20, r21, .sh_maybe_inject_list
    move.l  r21, #0x6C                  ; 'l'
    beq     r20, r21, .sh_maybe_inject_list
    store.b r0, (r19)
    bra     .sh_args_done

.sh_maybe_inject_dir:
    move.l  r21, #3
    bne     r17, r21, .sh_no_args_plain
    add     r18, r18, #1
    load.b  r20, (r18)
    move.l  r21, #0x49                  ; 'I'
    beq     r20, r21, .sh_inject_current_volume
    move.l  r21, #0x69                  ; 'i'
    beq     r20, r21, .sh_inject_current_volume
    bra     .sh_no_args_plain

.sh_maybe_inject_list:
    move.l  r21, #4
    bne     r17, r21, .sh_no_args_plain
    add     r18, r18, #1
    load.b  r20, (r18)
    move.l  r21, #0x49                  ; 'I'
    bne     r20, r21, .sh_maybe_inject_list_lower
    add     r18, r18, #1
    load.b  r20, (r18)
    move.l  r21, #0x53                  ; 'S'
    beq     r20, r21, .sh_inject_current_volume
.sh_maybe_inject_list_lower:
    sub     r18, r18, #1
    add     r18, r18, #1
    load.b  r20, (r18)
    move.l  r21, #0x69                  ; 'i'
    bne     r20, r21, .sh_no_args_plain
    add     r18, r18, #1
    load.b  r20, (r18)
    move.l  r21, #0x73                  ; 's'
    beq     r20, r21, .sh_inject_current_volume

.sh_no_args_plain:
    store.b r0, (r19)
    bra     .sh_args_done

.sh_inject_current_volume:
    add     r18, r29, #192
    load.b  r20, (r18)
    beqz    r20, .sh_no_args_plain
.sh_cp_current_volume:
    load.b  r20, (r18)
    store.b r20, (r19)
    beqz    r20, .sh_args_done
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .sh_cp_current_volume

.sh_send_run:

    ; 3. Send DOS_RUN to dos.library
    load.q  r29, (sp)
    move.l  r2, #DOS_RUN               ; type = DOS_RUN
    move.q  r3, r0                      ; data0 = 0 (unused in M10)
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 144(r29)               ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; 4. WaitPort(reply_port) for dos.library response
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; 5. Check response: R1=type (error code), if NOTFOUND → "Unknown command"
    move.l  r14, #DOS_ERR_NOTFOUND
    bne     r1, r14, .sh_cmd_ok

.sh_unknown_cmd:
    ; Unknown command
    load.q  r29, (sp)
    add     r20, r29, #88             ; "Unknown command\r\n"
    jsr     .sh_send_string
    bra     .sh_main_loop

.sh_cmd_ok:
    ; Command launched — yield 200 times to let it finish
    move.l  r20, #200
    store.q r20, 8(sp)
.sh_delay:
    load.q  r20, 8(sp)
    beqz    r20, .sh_delay_done
    sub     r20, r20, #1
    store.q r20, 8(sp)
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_delay
.sh_delay_done:
    bra     .sh_main_loop

    ; =====================================================================
    ; send_string subroutine: R20 = string addr, console port from data[136]
    ; Clobbers R1-R6, R20. After return, R29 is valid (reloaded from sp).
    ; =====================================================================
; .sh_send_string: send null-terminated string at R20 to console.handler.
; Called via jsr — has its own 16-byte stack frame.
; Stack layout: [sp]=local R29, [sp+8]=R20 save, [sp+16]=return addr, [sp+24]=caller R29
; Clobbers R1-R6, R20, R28. R29 reloaded from caller frame.
.sh_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.sh_ss_loop:
    ; R20 must be valid here (set by caller or by post-PutMsg reload)
    ; Save R20 immediately, then re-read from saved copy for safety
    store.q r20, 8(sp)                  ; save R20 to scratch
    load.q  r20, 8(sp)                  ; reload (survives context switch at store.q boundary)
    load.b  r1, (r20)
    beqz    r1, .sh_ss_done
.sh_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)                   ; data0 = char
    move.l  r2, #0                      ; type = CON_MSG_CHAR
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .sh_ss_loop
.sh_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ss_retry
.sh_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_shell_code_end:

prog_shell_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 32 ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: "Shell [Task \0" ---
    dc.b    "Shell M10 [Task ", 0
    ds.b    7                           ; pad to offset 56
    ; --- Offset 56: dead 24-byte slot (formerly "IntuitionOS M11\r\n\0",
    ;     printed by a stale shell banner removed in M12.8 Phase 1).
    ;     Kept as padding to preserve the offsets of the strings below. ---
    ds.b    24                          ; pad to offset 80
    ; --- Offset 80: "1> \0" (4 bytes) + pad to 88 ---
    dc.b    "1> ", 0
    ds.b    4                           ; pad to offset 88
    ; --- Offset 88: "Unknown command\r\n\0" (18 bytes, ends at 106) + pad to 128 ---
    dc.b    "Unknown command", 0x0D, 0x0A, 0
    ds.b    22                          ; pad to offset 128
    ; --- Offset 128: task_id (8 bytes) ---
    ds.b    8
    ; --- Offset 136: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 144: dos_port (8 bytes) ---
    ds.b    8
    ; --- Offset 152: reply_port (8 bytes) ---
    ds.b    8
    ; --- Offset 160: shared_buf_va (8 bytes) ---
    ds.b    8
    ; --- Offset 168: shared_buf_handle (4 bytes) + pad ---
    ds.b    8
    ; --- Offset 176: script_mode (1 byte) + padding ---
    ds.b    8
    ; --- Offset 184: script_pos (4 bytes) + script_len (4 bytes) ---
    ds.b    8
    ; --- Offset 192: (M10: command table removed, space available) ---
    ds.b    48                          ; pad to offset 240
    ; --- Offset 240: line buffer (128 bytes) ---
    ds.b    128
    ; --- Offset 368: script buffer (512 bytes for Startup-Sequence —
    ;     M12.8: bumped from 256 to absorb new boot ECHO line) ---
    ds.b    512
prog_shell_data_end:
    align   8
prog_shell_end:
