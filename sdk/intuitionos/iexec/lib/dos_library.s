prog_doslib:
    .libmanifest name="dos.library", version=14, revision=0, type=1, flags=MODF_COMPAT_PORT|MODF_ASLR_CAPABLE, msg_abi=0
    ; Header
    dc.l    0, 0
    dc.l    prog_doslib_code_end - prog_doslib_code
    dc.l    prog_doslib_data_end - prog_doslib_data
    dc.l    0
    ds.b    12
prog_doslib_code:

    ; =====================================================================
    ; Preamble: compute data page base (preemption-safe double-check).
    ;
    ; M12.8 prerequisite: the previous version added a hard-coded
    ; `#0x3000` offset to USER_CODE_BASE + task_id*stride. That offset
    ; assumed dos.library has exactly 2 code pages — wrong as soon as
    ; the code section grows past 8 KiB. The robust derivation: the
    ; The kernel seeds the top of the initial stack page with data_base.
    ; After the entry-point `sub sp, sp, #16`, that seeded qword sits at
    ; 8(sp), so the preamble can recover data_base without assuming the
    ; stack page is adjacent to the data page.
    ; =====================================================================
    sub     sp, sp, #16
.dos_preamble:
    load.q  r30, (sp)                  ; r30 = startup_base
    load.q  r29, 8(sp)                 ; r29 = data_base
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)               ; data[128] = task_id
    store.q r30, 192(r29)              ; data[192] = startup_base (boot-only scratch)

    ; =====================================================================
    ; OpenLibrary("console.handler", 0) with retry until found
    ; =====================================================================
.dos_openlib_retry:
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = &data[0] = "console.handler"
    move.q  r2, r0                      ; R2 = version 0
    syscall #SYS_FIND_PORT             ; R1 = handle (port_id), R2 = err
    load.q  r29, (sp)
    bnez    r2, .dos_openlib_wait
    store.q r1, 136(r29)               ; data[136] = console_port
    bra     .dos_banner_done
.dos_openlib_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_openlib_retry

    ; =====================================================================
    ; Legacy boot banner disabled.
    ; =====================================================================
.dos_send_banner:
    add     r20, r29, #32              ; r20 = &data[32] = banner string
.dos_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .dos_ban_id
    store.q r20, 8(sp)
.dos_ban_retry:
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
    beq     r2, r28, .dos_ban_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .dos_ban_loop
.dos_ban_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_ban_retry
.dos_ban_id:
    ; Send task_id digit + "]\r\n"
.dos_bid_retry:
    load.q  r29, (sp)
    load.q  r3, 128(r29)
    add     r3, r3, #0x30              ; ASCII '0'+id
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_bid_full
    bra     .dos_bid_bracket
.dos_bid_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_bid_retry
.dos_bid_bracket:
.dos_brk_retry:
    move.l  r3, #0x5D                   ; ']'
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_brk_full
    bra     .dos_cr
.dos_brk_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_brk_retry
.dos_cr:
.dos_cr_retry:
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
    beq     r2, r28, .dos_cr_full
    bra     .dos_lf
.dos_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_cr_retry
.dos_lf:
.dos_lf_retry:
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
    beq     r2, r28, .dos_lf_full
    bra     .dos_banner_done
.dos_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_lf_retry
.dos_banner_done:

    ; =====================================================================
    ; M12.6 Phase A: initialize the metadata + handle chain heads.
    ; Each chain starts with one AllocMem'd 4 KiB page; further pages are
    ; allocated on demand by .dos_meta_alloc_page / .dos_hnd_alloc_page.
    ; CreatePort is still deferred until after seeding — boot race prevention.
    ; =====================================================================
    ; Allocate first metadata chain page
    move.l  r1, #4096
    move.l  r2, #(MEMF_PUBLIC | MEMF_CLEAR)
    syscall #SYS_ALLOC_MEM             ; R1 = VA, R2 = err
    load.q  r29, (sp)
    store.q r1, 152(r29)               ; data[152] = meta_chain_head_va
    ; Allocate first handle chain page
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1 = VA, R2 = err
    load.q  r29, (sp)
    store.q r1, 160(r29)               ; data[160] = hnd_chain_head_va

    ; =====================================================================
    ; Pre-create "readme" file at metadata chain page slot 0.
    ; The first chain page is empty so the "find free slot" walker would
    ; trivially return slot 0; we just inline the same operations here
    ; for the boot path to keep boot init simple and dependency-free.
    ; M12.8 Phase 2: file body is now an extent chain, not a fixed
    ; AllocMem(DOS_FILE_SIZE) page.
    ; =====================================================================
    ; Allocate the readme body via the extent allocator (14 bytes).
    move.l  r1, #14
    jsr     .dos_extent_alloc           ; r1 = first extent VA, r2 = err
    bnez    r2, .dos_init_done          ; alloc failed → leave entry unset
    move.q  r24, r1                     ; r24 = readme_body_first_va

    ; Get the metadata chain head and address slot 0
    load.q  r25, 152(r29)              ; r25 = meta page VA
    add     r25, r25, #DOS_META_HDR_SZ ; r25 = &entries[0]

    ; Copy filename from data[896] = "readme" to entry.name (max 31 + NUL)
    add     r20, r29, #(prog_doslib_seed_readme_name - prog_doslib_data)
    move.q  r21, r25                   ; dst = &entry.name
    move.l  r14, #0
.dos_cpname:
    load.b  r15, (r20)
    store.b r15, (r21)
    beqz    r15, .dos_cpname_done
    add     r20, r20, #1
    add     r21, r21, #1
    add     r14, r14, #1
    move.l  r28, #31
    blt     r14, r28, .dos_cpname
.dos_cpname_done:
    ; entry.file_va = r24 (readme_body_first_va, head of extent chain)
    store.q r24, DOS_META_OFF_VA(r25)
    ; entry.size = 14 (length of welcome message)
    move.l  r14, #14
    store.l r14, DOS_META_OFF_SIZE(r25)

    ; Copy welcome message from data[912] into the extent chain payload.
    move.q  r1, r24                    ; r1 = first extent VA
    add     r2, r29, #(prog_doslib_seed_readme_body - prog_doslib_data)
    move.l  r3, #14                    ; r3 = byte_count
    jsr     .dos_extent_write
    load.q  r29, (sp)
.dos_init_done:

    ; =====================================================================
    ; Create the DOS port (after initialization = readiness signal)
    ; =====================================================================
    load.q  r29, (sp)
    add     r1, r29, #16               ; R1 = &data[16] = "dos.library"
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT           ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 144(r29)               ; data[144] = dos_port
    add     r1, r29, #16               ; R1 = &data[16] = "dos.library"
    move.l  r2, #14                    ; lib_version
    move.l  r3, #0                     ; lib_revision
    load.q  r4, 144(r29)               ; compat public port
    syscall #SYS_ADD_LIBRARY
    load.q  r29, (sp)
    bnez    r2, .dos_boot_fail

    ; M16.2: dos.library owns the eager non-library module boot policy.
    add     r16, r29, #(prog_doslib_empty_args - prog_doslib_data)
    move.q  r18, r0
    move.l  r1, #BOOT_MANIFEST_ID_HWRES
    move.q  r2, r16
    move.q  r3, r18
    jsr     .dos_manifest_launch_by_id
    load.q  r29, (sp)
    bnez    r2, .dos_boot_fail
    add     r16, r29, #(prog_doslib_empty_args - prog_doslib_data)
    move.q  r18, r0
    move.l  r1, #BOOT_MANIFEST_ID_INPUT
    move.q  r2, r16
    move.q  r3, r18
    jsr     .dos_manifest_launch_by_id
    load.q  r29, (sp)
    bnez    r2, .dos_boot_fail

    ; =====================================================================
    ; Launch the boot shell from the startup-block relpath when present.
    ; Older images can still fall back to the baked private literal.
    ; =====================================================================
    load.q  r30, 192(r29)              ; startup_base
    load.l  r11, TASKSB_SIZE(r30)
    move.l  r12, #TASK_STARTUP_SIZE
    blt     r11, r12, .dos_boot_shell_use_private
    add     r23, r30, #TASKSB_BOOT_DOS_SHELL_RELPATH
    load.b  r11, (r23)
    bnez    r11, .dos_boot_shell_path_ready
.dos_boot_shell_use_private:
    add     r23, r29, #(prog_doslib_boot_shell_relpath - prog_doslib_data)
.dos_boot_shell_path_ready:
    add     r16, r29, #(prog_doslib_empty_args - prog_doslib_data)
    move.q  r18, r0
    move.q  r17, r0
    move.q  r30, r29
    jsr     .dos_launch_hostfs_relpath_name
    move.q  r29, r30
    bnez    r1, .dos_main_loop
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #DOS_ERR_NOTFOUND
    beq     r28, r15, .dos_main_loop
    bnez    r28, .dos_boot_fail

    ; =====================================================================
    ; Main loop: WaitPort(dos_port) → dispatch on message type
    ; =====================================================================
.dos_main_loop:
    load.q  r29, (sp)
    load.q  r1, 144(r29)               ; R1 = dos_port
    syscall #SYS_WAIT_PORT              ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    load.q  r29, (sp)

    ; Save message fields to data page scratch
    store.q r5, 944(r29)               ; saved reply_port
    store.q r1, 952(r29)               ; saved type (opcode)
    store.q r2, 960(r29)               ; saved data0
    store.q r4, 968(r29)               ; saved data1
    store.q r6, 976(r29)               ; saved share_handle

    ; --- Map caller's shared buffer when a share is supplied ---
    load.l  r15, 976(r29)              ; incoming share_handle
    beqz    r15, .dos_have_buf         ; messages like DOS_CLOSE don't need a buffer
    store.l r15, 984(r29)              ; update cached handle
    move.q  r1, r15                    ; R1 = new share_handle
    move.l  r2, #(MAPF_READ | MAPF_WRITE)
    syscall #SYS_MAP_SHARED            ; R1=VA R2=err R3=share_pages
    move.q  r24, r3                    ; preserve R3 across r29 reload
    load.q  r29, (sp)
    beqz    r1, .dos_reply_err         ; MapShared failed
    ; Reject shares smaller than 1 page. Defense-in-depth: AllocMem rounds
    ; up to ≥1 page (4096B), so this never triggers in normal use. The
    ; DOS_READ/WRITE share-size clamps below also bound the copy by
    ; share_pages*4096 for additional safety.
    move.l  r11, #1                    ; min pages = 1
    blt     r24, r11, .dos_reply_badarg
    store.q r1, 168(r29)               ; update cached VA
    store.q r24, 184(r29)              ; cache share_pages
.dos_have_buf:

.dos_dispatch:
    load.q  r14, 952(r29)              ; r14 = opcode
    move.l  r28, #MSG_GET_IOSM
    beq     r14, r28, .dos_do_get_iosm
    move.l  r28, #DOS_OP_PARSE_MANIFEST
    beq     r14, r28, .dos_do_parse_manifest
    move.l  r28, #DOS_DIR
    beq     r14, r28, .dos_do_dir
    move.l  r28, #DOS_OPEN
    beq     r14, r28, .dos_do_open
    move.l  r28, #DOS_READ
    beq     r14, r28, .dos_do_read
    move.l  r28, #DOS_WRITE
    beq     r14, r28, .dos_do_write
    move.l  r28, #DOS_CLOSE
    beq     r14, r28, .dos_do_close
    move.l  r28, #DOS_ASSIGN
    beq     r14, r28, .dos_do_assign
    move.l  r28, #DOS_RUN
    beq     r14, r28, .dos_do_run
    move.l  r28, #DOS_LOADSEG
    beq     r14, r28, .dos_do_loadseg
    move.l  r28, #DOS_UNLOADSEG
    beq     r14, r28, .dos_do_unloadseg
    move.l  r28, #DOS_RUNSEG
    beq     r14, r28, .dos_do_runseg
    move.l  r28, #DOS_LOADLIB
    beq     r14, r28, .dos_do_loadlib
    ; Unknown opcode → reply with error and loop
    bra     .dos_reply_err

.dos_do_get_iosm:
    load.l  r21, 976(r29)              ; incoming share_handle required
    beqz    r21, .dos_get_iosm_reply_badarg
    load.q  r24, 184(r29)              ; cached share_pages
    move.l  r11, #1
    bne     r24, r11, .dos_get_iosm_badarg_free
    load.q  r23, 168(r29)              ; mapped caller buffer
    add     r14, r29, #(prog_doslib_iosm - prog_doslib_data)
    move.l  r15, #(IOSM_SIZE / 8)
.dos_get_iosm_copy:
    load.q  r16, (r14)
    store.q r16, (r23)
    add     r14, r14, #8
    add     r23, r23, #8
    sub     r15, r15, #1
    bnez    r15, .dos_get_iosm_copy
    load.q  r1, 168(r29)
    move.q  r2, r24
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    load.q  r1, 944(r29)
    move.q  r2, r0
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_get_iosm_badarg_free:
    load.q  r1, 168(r29)
    move.q  r2, r24
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    bra     .dos_get_iosm_reply_badarg
.dos_get_iosm_reply_badarg:
    load.q  r1, 944(r29)
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_do_parse_manifest:
    load.l  r21, 976(r29)
    beqz    r21, .dos_pmp_reply_badarg
    load.q  r24, 184(r29)
    move.l  r11, #1
    bne     r24, r11, .dos_pmp_badarg_free
    load.q  r23, 168(r29)              ; mapped 4 KiB parse page

    ; Path must be NUL-terminated within DOS_PMP_PATH_MAX bytes.
    move.l  r14, #0
.dos_pmp_scan_nul:
    move.l  r11, #DOS_PMP_PATH_MAX
    bge     r14, r11, .dos_pmp_badarg_write
    add     r15, r23, r14
    load.b  r16, (r15)
    beqz    r16, .dos_pmp_have_path
    add     r14, r14, #1
    bra     .dos_pmp_scan_nul

.dos_pmp_have_path:
    move.q  r1, r23
    add     r2, r23, #DOS_PMP_IOSM_OFF
    jsr     .dos_pmp_parse_file_iosm    ; R20=rc
    load.q  r29, (sp)
    load.q  r23, 168(r29)
    bra     .dos_pmp_write_rc
.dos_pmp_badarg_write:
    move.l  r20, #ERR_BADARG
.dos_pmp_write_rc:
    add     r15, r23, #DOS_PMP_RC_OFF
    store.l r20, (r15)
    store.q r20, 656(r29)
    load.q  r1, 168(r29)
    move.l  r2, #4096
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    load.q  r20, 656(r29)
    load.q  r1, 944(r29)
    move.q  r2, r20
    move.q  r3, r20
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_pmp_badarg_free:
    load.q  r1, 168(r29)
    move.q  r2, r24
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.dos_pmp_reply_badarg:
    load.q  r1, 944(r29)
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_DIR (type=5): format directory listing into caller's buffer
    ; M12.6 Phase A: walks the metadata chain instead of a fixed file table.
    ; =================================================================
.dos_do_dir:
    load.q  r23, 168(r29)              ; input path in caller's shared buffer
    load.b  r14, (r23)
    bnez    r14, .dos_dir_explicit
.dos_dir_root:
    load.q  r20, 168(r29)              ; r20 = dest (caller's shared buffer)
    move.q  r21, r0                     ; r21 = total bytes written
    ; Compute share_bytes - 56 reserve into r24 (max ~56 bytes per entry:
    ; 32 name pad + up to 10 size digits + 2 CRLF + 12 slack/NUL). Defense-in-depth
    ; against a small share that can't fit the full directory listing.
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    sub     r24, r24, #56              ; r24 = safe write ceiling
    ; Walk the metadata chain. r25 = current chain page VA.
    load.q  r25, 152(r29)              ; meta chain head
.dos_dir_page_loop:
    beqz    r25, .dos_dir_meta_done
    move.q  r22, r25
    add     r22, r22, #DOS_META_HDR_SZ ; r22 = &entries[0]
    move.l  r23, #0                    ; entry index in this page
.dos_dir_entry:
    ; Stop if buffer is nearly full
    bge     r21, r24, .dos_dir_done
    move.l  r28, #DOS_META_PER_PAGE
    bge     r23, r28, .dos_dir_next_page
    move.q  r14, r22                    ; r14 = entry VA
    load.b  r15, (r14)                  ; first byte of name
    beqz    r15, .dos_dir_next          ; empty entry → skip
    ; Copy name chars until null (max 16)
    move.q  r16, r14                    ; r16 = name pointer
    move.l  r17, #0                     ; name char count
.dos_dir_cpname:
    load.b  r15, (r16)
    beqz    r15, .dos_dir_pad
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r16, r16, #1
    add     r17, r17, #1
    move.l  r28, #32
    blt     r17, r28, .dos_dir_cpname
.dos_dir_pad:
    ; Pad with spaces to column 32
    move.l  r28, #32
    bge     r17, r28, .dos_dir_size
    move.l  r15, #0x20                  ; ' '
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .dos_dir_pad
.dos_dir_size:
    ; Read file size from entry+DOS_META_OFF_SIZE and write it in decimal.
    ; Keep the historical minimum width of 4 digits for small files, but
    ; expand cleanly past 9999 now that M12.8 removed the old per-file cap.
    load.l  r15, DOS_META_OFF_SIZE(r14) ; r15 = remaining size
    move.l  r18, #0                     ; r18 = started flag

    ; 1,000,000,000
    move.l  r28, #1000000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_1g_chk
    bra     .dds_emit_1g
.dds_skip_1g_chk:
    beqz    r16, .dds_skip_1g
.dds_emit_1g:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_1g:

    ; 100,000,000
    move.l  r28, #100000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_100m_chk
    bra     .dds_emit_100m
.dds_skip_100m_chk:
    beqz    r16, .dds_skip_100m
.dds_emit_100m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_100m:

    ; 10,000,000
    move.l  r28, #10000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_10m_chk
    bra     .dds_emit_10m
.dds_skip_10m_chk:
    beqz    r16, .dds_skip_10m
.dds_emit_10m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_10m:

    ; 1,000,000
    move.l  r28, #1000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_1m_chk
    bra     .dds_emit_1m
.dds_skip_1m_chk:
    beqz    r16, .dds_skip_1m
.dds_emit_1m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_1m:

    ; 100,000
    move.l  r28, #100000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_100k_chk
    bra     .dds_emit_100k
.dds_skip_100k_chk:
    beqz    r16, .dds_skip_100k
.dds_emit_100k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_100k:

    ; 10,000
    move.l  r28, #10000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_10k_chk
    bra     .dds_emit_10k
.dds_skip_10k_chk:
    beqz    r16, .dds_skip_10k
.dds_emit_10k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_10k:

    ; 1,000 (always emit from here down for 4-digit minimum width)
    move.l  r28, #1000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1

    ; Hundreds
    move.l  r28, #100
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1

    ; Tens
    move.l  r28, #10
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1

    ; Ones
    add     r15, r15, #0x30
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    ; Write "\r\n"
    move.l  r15, #0x0D
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r15, #0x0A
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
.dos_dir_next:
    add     r22, r22, #DOS_META_ENTRY_SZ
    add     r23, r23, #1
    bra     .dos_dir_entry
.dos_dir_next_page:
    load.q  r25, (r25)
    bra     .dos_dir_page_loop
.dos_dir_explicit:
    jsr     .dos_resolve_file
    load.q  r29, (sp)
    beqz    r22, .dos_dir_explicit_empty
    sub     sp, sp, #96
    load.q  r20, 168(r29)
    move.q  r21, r0
    load.q  r24, 184(r29)
    lsl     r24, r24, #12
    sub     r24, r24, #56
    store.q r20, 0(sp)                 ; dest ptr
    store.q r21, 8(sp)                 ; bytes written
    store.q r24, 16(sp)                ; safe ceiling
    move.q  r1, r23
    add     r2, sp, #32                ; original resolved path scratch
    jsr     .dos_copy_zstr
.dos_dir_explicit_try_hostfs:
    add     r1, sp, #32
    jsr     .dos_hostfs_relpath_for_resolved_name
    beqz    r3, .dos_dir_explicit_prepare_meta
    store.q r1, 24(sp)                 ; preserve relpath across helper JSRs
    jsr     .dos_bootfs_stat
    bnez    r3, .dos_dir_explicit_prepare_meta
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_DIR
    bne     r28, r15, .dos_dir_explicit_prepare_meta
    load.q  r26, 24(sp)
    move.q  r27, r26
    move.l  r28, #0
.dos_dir_explicit_hostfs_len:
    add     r14, r27, r28
    load.b  r15, (r14)
    beqz    r15, .dos_dir_explicit_hostfs_len_done
    add     r28, r28, #1
    move.l  r16, #47
    blt     r28, r16, .dos_dir_explicit_hostfs_len
.dos_dir_explicit_hostfs_len_done:
    beqz    r28, .dos_dir_explicit_hostfs_ready
    add     r14, r27, r28
    sub     r14, r14, #1
    load.b  r15, (r14)
    move.l  r16, #0x2F
    beq     r15, r16, .dos_dir_explicit_hostfs_ready
    add     r14, r27, r28
    store.b r16, (r14)
    add     r14, r14, #1
    store.b r0, (r14)
.dos_dir_explicit_hostfs_ready:
    load.q  r20, 0(sp)
    load.q  r21, 8(sp)
    load.q  r24, 16(sp)
    load.q  r1, 24(sp)
    jsr     .dos_dir_append_hostfs_explicit_names
    bnez    r21, .dos_dir_explicit_done
    bra     .dos_dir_explicit_prepare_meta
.dos_dir_explicit_prepare_meta:
    add     r1, sp, #32
    add     r2, sp, #64                ; normalized prefix scratch
    jsr     .dos_copy_zstr
    move.q  r26, r1
    add     r27, sp, #64
    load.b  r14, (r27)
    move.l  r15, #0x53                 ; 'S'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 1(r27)
    move.l  r15, #0x59                 ; 'Y'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 2(r27)
    move.l  r15, #0x53                 ; 'S'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 3(r27)
    move.l  r15, #0x2F                 ; '/'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 4(r27)
    move.l  r15, #0x49                 ; 'I'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 5(r27)
    move.l  r15, #0x4F                 ; 'O'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 6(r27)
    move.l  r15, #0x53                 ; 'S'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 7(r27)
    move.l  r15, #0x53                 ; 'S'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 8(r27)
    move.l  r15, #0x59                 ; 'Y'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 9(r27)
    move.l  r15, #0x53                 ; 'S'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    load.b  r14, 10(r27)
    move.l  r15, #0x2F                 ; '/'
    bne     r14, r15, .dos_dir_explicit_prefix_base_done
    add     r27, r27, #11
.dos_dir_explicit_prefix_base_done:
    sub     r30, r26, r27
    beqz    r30, .dos_dir_explicit_meta
    sub     r14, r26, #1
    load.b  r15, (r14)
    move.l  r16, #0x2F
    beq     r15, r16, .dos_dir_explicit_prefix_done
    store.b r16, (r26)
    add     r26, r26, #1
    store.b r0, (r26)
    add     r30, r30, #1
.dos_dir_explicit_prefix_done:
.dos_dir_explicit_meta:
    load.q  r20, 0(sp)
    load.q  r21, 8(sp)
    load.q  r24, 16(sp)
    move.q  r1, r27
    move.q  r2, r30
    jsr     .dos_dir_append_meta_explicit_prefix
.dos_dir_explicit_meta_done:
    bnez    r21, .dos_dir_explicit_done
.dos_dir_explicit_done:
    store.b r0, (r20)
    add     sp, sp, #96
    bra     .dos_dir_reply_ok
.dos_dir_explicit_empty:
    load.q  r20, 168(r29)
    move.q  r21, r0
    store.b r0, (r20)
    bra     .dos_dir_reply_ok
.dos_dir_meta_done:
    ; M15.3: merge writable SYS: overlay entries first (first-effective),
    ; then the read-only IOSSYS entries. Writable scans use the public
    ; path string as BOTH the display prefix and the hostfs-relative dir
    ; (e.g. "C/" → hostRoot/C). Duplicate names across the two passes
    ; are collapsed at the emission site by .dos_dir_name_already_emitted
    ; so a file that exists in both hostRoot/C/ and hostRoot/IOSSYS/C/
    ; appears exactly once (first-effective wins, writable layer first).
    add     r1, r29, #(prog_doslib_hostfs_public_c - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_c - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_s - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_s - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_l - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_l - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_libs - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_libs - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_devs - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_devs - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_resources - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_public_resources - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_c - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_c - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_s - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_s - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_l - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_l - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_libs - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_libs - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_devs - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_devs - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
    add     r1, r29, #(prog_doslib_hostfs_public_resources - prog_doslib_data)
    add     r2, r29, #(prog_doslib_hostfs_rel_resources - prog_doslib_data)
    jsr     .dos_dir_append_hostfs_dir
.dos_dir_done:
    ; Null-terminate
    store.b r0, (r20)
.dos_dir_reply_ok:
    ; Reply with data0 = bytes written
    load.q  r1, 944(r29)               ; reply_port
    move.l  r2, #DOS_OK                 ; type = success
    move.q  r3, r21                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; .dos_dir_append_hostfs_dir
    ; In/out: r20 = dest ptr, r21 = bytes written, r24 = safe ceiling
    ; In: r1 = public DOS prefix ptr, r2 = hostfs-relative dir path ptr
.dos_dir_append_hostfs_dir:
    push    r1
    push    r2
    move.l  r16, #0
.ddahd_entry:
    add     r17, r21, #38
    bgt     r17, r24, .ddahd_done
    push    r20
    push    r21
    push    r24
    load.q  r1, 24(sp)
    move.q  r2, r16
    jsr     .dos_bootfs_readdir
    pop     r24
    pop     r21
    pop     r20
    bnez    r3, .ddahd_done
    push    r1
    push    r20
    push    r21
    push    r24
    move.q  r2, r1
    load.q  r1, 40(sp)
    jsr     .dos_dir_hostfs_name_is_seeded
    pop     r24
    pop     r21
    pop     r20
    pop     r1
    bnez    r3, .ddahd_skip
    ; M15.3 Gap 1: dedup guard. Scan already-emitted output for an entry
    ; whose prefix+name matches the current candidate so writable-overlay
    ; and IOSSYS passes don't list the same file twice.
    push    r1                            ; preserve dirent ptr
    push    r20
    push    r21
    push    r24
    load.q  r1, 32(sp)                    ; prefix ptr (sp+32 = original push r1)
    load.q  r14, 24(sp)                   ; dirent ptr just saved
    add     r2, r14, #BOOT_HOSTFS_DIRENT_NAME_OFF
    jsr     .dos_dir_name_already_emitted
    pop     r24
    pop     r21
    pop     r20
    pop     r1
    bnez    r3, .ddahd_skip
    move.q  r17, r20
    move.q  r18, r21
    move.l  r19, #0
    load.q  r14, 8(sp)
.ddahd_prefix:
    load.b  r22, (r14)
    beqz    r22, .ddahd_name
    move.l  r23, #32
    bge     r19, r23, .ddahd_name
    store.b r22, (r17)
    add     r14, r14, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahd_prefix
.ddahd_name:
    add     r22, r1, #BOOT_HOSTFS_DIRENT_NAME_OFF
.ddahd_name_loop:
    load.b  r23, (r22)
    beqz    r23, .ddahd_pad
    move.l  r25, #32
    bge     r19, r25, .ddahd_pad
    store.b r23, (r17)
    add     r22, r22, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahd_name_loop
.ddahd_pad:
    move.l  r25, #32
    bge     r19, r25, .ddahd_size
    move.l  r23, #0x20
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahd_pad
.ddahd_size:
    move.l  r23, #0x30
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0D
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0A
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.q  r20, r17
    move.q  r21, r18
.ddahd_skip:
    add     r16, r16, #1
    bra     .ddahd_entry
.ddahd_done:
    pop     r2
    pop     r1
    rts

    ; M15.3 .dos_dir_name_already_emitted
    ; In:  r1 = prefix ptr (NUL-terminated, e.g. "C/")
    ;      r2 = name ptr (NUL-terminated, from dirent NAME_OFF)
    ;      Buffer base at 168(r29), current bytes written in r21.
    ; Out: r3 = 1 if the candidate prefix+name matches an entry already
    ;      in the output buffer (case-insensitive with the trailing
    ;      space-padding delimiter), 0 otherwise.
    ; Clobbers: r4-r19, r22-r26.
.dos_dir_name_already_emitted:
    load.q  r4, 168(r29)                  ; buffer base
    move.q  r5, r4                        ; stride cursor
    move.q  r6, r4
    add     r6, r6, r21                   ; buffer end (current cursor)
.ddname_stride_loop:
    move.l  r7, #38
    add     r8, r5, r7
    bgt     r8, r6, .ddname_no_match
    ; Compare prefix chars (ci) against stride[0..]
    move.q  r9, r1
    move.q  r10, r5
.ddname_cmp_prefix:
    load.b  r12, (r9)
    beqz    r12, .ddname_cmp_name_init
    load.b  r13, (r10)
    move.l  r14, #0x61
    blt     r12, r14, .ddname_p_ok
    move.l  r14, #0x7B
    bge     r12, r14, .ddname_p_ok
    sub     r12, r12, #0x20
.ddname_p_ok:
    move.l  r14, #0x61
    blt     r13, r14, .ddname_s_ok
    move.l  r14, #0x7B
    bge     r13, r14, .ddname_s_ok
    sub     r13, r13, #0x20
.ddname_s_ok:
    bne     r12, r13, .ddname_next_stride
    add     r9, r9, #1
    add     r10, r10, #1
    bra     .ddname_cmp_prefix
.ddname_cmp_name_init:
    move.q  r9, r2
.ddname_cmp_name:
    load.b  r12, (r9)
    beqz    r12, .ddname_cmp_terminator
    load.b  r13, (r10)
    move.l  r14, #0x61
    blt     r12, r14, .ddname_pn_ok
    move.l  r14, #0x7B
    bge     r12, r14, .ddname_pn_ok
    sub     r12, r12, #0x20
.ddname_pn_ok:
    move.l  r14, #0x61
    blt     r13, r14, .ddname_sn_ok
    move.l  r14, #0x7B
    bge     r13, r14, .ddname_sn_ok
    sub     r13, r13, #0x20
.ddname_sn_ok:
    bne     r12, r13, .ddname_next_stride
    add     r9, r9, #1
    add     r10, r10, #1
    bra     .ddname_cmp_name
.ddname_cmp_terminator:
    ; Candidate ended; stride must have a space (or any non-name char)
    ; here to avoid "C/Foo" matching "C/FooBar".
    load.b  r13, (r10)
    move.l  r14, #0x20
    bne     r13, r14, .ddname_next_stride
    move.l  r3, #1
    rts
.ddname_next_stride:
    add     r5, r5, #38
    bra     .ddname_stride_loop
.ddname_no_match:
    move.q  r3, r0
    rts

    ; .dos_dir_append_hostfs_explicit_names
    ; In/out: r20 = dest ptr, r21 = bytes written, r24 = safe ceiling
    ; In: r1 = hostfs-relative dir path ptr
.dos_dir_append_hostfs_explicit_names:
    push    r1
    move.l  r16, #0
.ddahe_entry:
    add     r17, r21, #38
    bgt     r17, r24, .ddahe_done
    push    r20
    push    r21
    push    r24
    load.q  r1, 24(sp)
    move.q  r2, r16
    jsr     .dos_bootfs_readdir
    pop     r24
    pop     r21
    pop     r20
    bnez    r3, .ddahe_done
    move.q  r17, r20
    move.q  r18, r21
    move.l  r19, #0
    add     r22, r1, #BOOT_HOSTFS_DIRENT_NAME_OFF
.ddahe_name_loop:
    load.b  r23, (r22)
    beqz    r23, .ddahe_pad
    move.l  r25, #32
    bge     r19, r25, .ddahe_pad
    store.b r23, (r17)
    add     r22, r22, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahe_name_loop
.ddahe_pad:
    move.l  r25, #32
    bge     r19, r25, .ddahe_size
    move.l  r23, #0x20
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahe_pad
.ddahe_size:
    move.l  r23, #0x30
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0D
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0A
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.q  r20, r17
    move.q  r21, r18
    add     r16, r16, #1
    bra     .ddahe_entry
.ddahe_done:
    pop     r1
    rts

    ; .dos_dir_append_meta_explicit_prefix
    ; In/out: r20 = dest ptr, r21 = bytes written, r24 = safe ceiling
    ; In: r1 = metadata prefix ptr ending with '/', r2 = prefix len
.dos_dir_append_meta_explicit_prefix:
    push    r1
    push    r2
    load.q  r25, 152(r29)
.ddame_page_loop:
    beqz    r25, .ddame_done
    move.q  r22, r25
    add     r22, r22, #DOS_META_HDR_SZ
    move.l  r19, #0
.ddame_entry:
    bge     r21, r24, .ddame_done
    move.l  r28, #DOS_META_PER_PAGE
    bge     r19, r28, .ddame_next_page
    move.q  r14, r22
    load.b  r15, (r14)
    beqz    r15, .ddame_next
    move.q  r26, r19
    push    r20
    push    r21
    push    r22
    push    r24
    push    r25
    move.q  r1, r14
    load.q  r2, 48(sp)
    jsr     .dos_prefix_eq_ci
    pop     r25
    pop     r24
    pop     r22
    pop     r21
    pop     r20
    move.q  r28, r23
    move.q  r19, r26
    bnez    r28, .ddame_next
    load.q  r28, (sp)
    add     r16, r14, r28
    load.b  r15, (r16)
    beqz    r15, .ddame_next
    move.l  r17, #0
.ddame_cpname:
    load.b  r15, (r16)
    beqz    r15, .ddame_pad
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r16, r16, #1
    add     r17, r17, #1
    move.l  r28, #32
    blt     r17, r28, .ddame_cpname
.ddame_pad:
    move.l  r28, #32
    bge     r17, r28, .ddame_size
    move.l  r15, #0x20
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .ddame_pad
.ddame_size:
    load.l  r15, DOS_META_OFF_SIZE(r14)
    move.l  r18, #0
    move.l  r28, #1000000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_1g_chk
    bra     .ddames_emit_1g
.ddames_skip_1g_chk:
    beqz    r16, .ddames_skip_1g
.ddames_emit_1g:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_1g:
    move.l  r28, #100000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_100m_chk
    bra     .ddames_emit_100m
.ddames_skip_100m_chk:
    beqz    r16, .ddames_skip_100m
.ddames_emit_100m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_100m:
    move.l  r28, #10000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_10m_chk
    bra     .ddames_emit_10m
.ddames_skip_10m_chk:
    beqz    r16, .ddames_skip_10m
.ddames_emit_10m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_10m:
    move.l  r28, #1000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_1m_chk
    bra     .ddames_emit_1m
.ddames_skip_1m_chk:
    beqz    r16, .ddames_skip_1m
.ddames_emit_1m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_1m:
    move.l  r28, #100000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_100k_chk
    bra     .ddames_emit_100k
.ddames_skip_100k_chk:
    beqz    r16, .ddames_skip_100k
.ddames_emit_100k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_100k:
    move.l  r28, #10000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .ddames_skip_10k_chk
    bra     .ddames_emit_10k
.ddames_skip_10k_chk:
    beqz    r16, .ddames_skip_10k
.ddames_emit_10k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.ddames_skip_10k:
    move.l  r28, #1000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r28, #100
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r28, #10
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r15, r15, #0x30
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r15, #0x0D
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r15, #0x0A
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
.ddame_next:
    add     r22, r22, #DOS_META_ENTRY_SZ
    add     r19, r19, #1
    bra     .ddame_entry
.ddame_next_page:
    load.q  r25, (r25)
    bra     .ddame_page_loop
.ddame_done:
    pop     r2
    pop     r1
    rts

    ; .dos_dir_append_hostfs_explicit_dir
    ; In/out: r20 = dest ptr, r21 = bytes written, r24 = safe ceiling
    ; In: r1 = resolved metadata prefix ptr ending with '/', r2 = hostfs-relative dir path ptr
.dos_dir_append_hostfs_explicit_dir:
    push    r1
    push    r2
    move.l  r16, #0
.ddahed_entry:
    add     r17, r21, #38
    bgt     r17, r24, .ddahed_done
    push    r20
    push    r21
    push    r24
    load.q  r1, 24(sp)
    move.q  r2, r16
    jsr     .dos_bootfs_readdir
    pop     r24
    pop     r21
    pop     r20
    bnez    r3, .ddahed_done
    push    r1
    add     r22, r29, #1000
    load.q  r14, 8(sp)
.ddahed_full_prefix:
    load.b  r23, (r14)
    store.b r23, (r22)
    beqz    r23, .ddahed_fix_name
    add     r14, r14, #1
    add     r22, r22, #1
    bra     .ddahed_full_prefix
.ddahed_fix_name:
    add     r24, r1, #BOOT_HOSTFS_DIRENT_NAME_OFF
.ddahed_full_name:
    load.b  r23, (r24)
    store.b r23, (r22)
    beqz    r23, .ddahed_lookup
    add     r24, r24, #1
    add     r22, r22, #1
    bra     .ddahed_full_name
.ddahed_lookup:
    add     r1, r29, #1000
    jsr     .dos_meta_find_by_name
    pop     r24
    bnez    r1, .ddahed_skip
    move.q  r17, r20
    move.q  r18, r21
    move.l  r19, #0
    add     r22, r24, #BOOT_HOSTFS_DIRENT_NAME_OFF
.ddahed_name_loop:
    load.b  r23, (r22)
    beqz    r23, .ddahed_pad
    move.l  r25, #32
    bge     r19, r25, .ddahed_pad
    store.b r23, (r17)
    add     r22, r22, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahed_name_loop
.ddahed_pad:
    move.l  r25, #32
    bge     r19, r25, .ddahed_size
    move.l  r23, #0x20
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .ddahed_pad
.ddahed_size:
    move.l  r23, #0x30
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0D
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r23, #0x0A
    store.b r23, (r17)
    add     r17, r17, #1
    add     r18, r18, #1
    move.q  r20, r17
    move.q  r21, r18
.ddahed_skip:
    add     r16, r16, #1
    bra     .ddahed_entry
.ddahed_done:
    pop     r2
    pop     r1
    rts

    ; .dos_dir_hostfs_name_is_seeded
    ; In:  r1 = public DOS prefix ptr (e.g. "C/")
    ;      r2 = hostfs dirent scratch ptr
    ; Out: r3 = 1 if that name already exists in seeded metadata, else 0
.dos_dir_hostfs_name_is_seeded:
    move.q  r20, r1
    move.q  r21, r2
    add     r22, r29, #1000
.ddhnis_prefix:
    load.b  r23, (r20)
    store.b r23, (r22)
    beqz    r23, .ddhnis_fix
    add     r20, r20, #1
    add     r22, r22, #1
    bra     .ddhnis_prefix
.ddhnis_fix:
    add     r21, r21, #BOOT_HOSTFS_DIRENT_NAME_OFF
.ddhnis_name:
    load.b  r23, (r21)
    store.b r23, (r22)
    beqz    r23, .ddhnis_lookup
    add     r21, r21, #1
    add     r22, r22, #1
    bra     .ddhnis_name
.ddhnis_lookup:
    add     r1, r29, #1000
    jsr     .dos_meta_find_by_name
    move.l  r3, #1
    bnez    r1, .ddhnis_done
    move.l  r3, #0
.ddhnis_done:
    rts

    ; =================================================================
    ; DOS_OPEN (type=1): open file by name from shared buffer
    ; M12.6 Phase A: walks the metadata chain via .dos_meta_find_by_name;
    ; allocates a new entry via .dos_meta_alloc_entry on write-mode miss;
    ; allocates a handle slot via .dos_hnd_alloc.
    ; =================================================================
    ; data0 = mode (READ=0, WRITE=1), filename in caller's shared buffer
.dos_do_open:
    ; M15.3 multi-entry fallthrough: 648(r29) = attempt index (reserved in
    ; the unused 648..744 dead-space slab to avoid collisions with scratch
    ; reused by the ELF loader, metadata walker, and hostfs helpers). Each
    ; miss in READ mode increments and re-enters resolution so DOS_OPEN
    ; walks the full effective list (overlay[0..N-1] then base/table) in
    ; order. Resolution returns not-found once attempt > overlay_count,
    ; which terminates the loop.
    store.q r0, 648(r29)
.dos_do_open_retry:
    load.q  r20, 960(r29)              ; r20 = mode (0=READ, 1=WRITE)
    load.q  r23, 168(r29)              ; r23 = mapped VA (filename pointer)
    ; Propagate attempt index into lookup's scratch input.
    store.q r0, 856(r29)               ; skip_overlay = 0 (iterate instead)
    load.q  r28, 648(r29)
    store.q r28, 880(r29)

    ; Resolve filename through the DOS assign table.
    jsr     .dos_resolve_file
    load.q  r29, (sp)
    beqz    r22, .dos_reply_err
    load.q  r20, 960(r29)              ; resolver clobbers caller-save regs
    store.q r23, 336(r29)              ; preserve resolved name across helper JSRs

    ; M15.3 Gap 2: on WRITE mode, skip read-only candidates so the
    ; effective-list scan walks forward to the first writable target.
    ; The only read-only shape that can appear here is a resolved name
    ; under "IOSSYS/" or "SYS/IOSSYS/" (validate_target rejects ':' in
    ; overlay values, so overlays can't introduce any other read-only
    ; target). Bumping the attempt index and re-entering resolve moves
    ; to the next effective target; when the list is exhausted the
    ; beqz above replies NOTFOUND cleanly — no RAM synthesis.
    beqz    r20, .dos_do_open_post_write_filter
    load.q  r1, 336(r29)
    jsr     .dos_resolved_is_iossys
    beqz    r3, .dos_do_open_post_write_filter
    load.q  r28, 648(r29)
    add     r28, r28, #1
    store.q r28, 648(r29)
    bra     .dos_do_open_retry

.dos_do_open_post_write_filter:
    load.q  r23, 336(r29)

    ; M15.3: pick the layered hostfs path. For READ mode this prefers the
    ; writable SYS: overlay (e.g. hostRoot/C/Foo) when present and falls
    ; back to the read-only IOSSYS path (hostRoot/IOSSYS/C/Foo). For
    ; WRITE mode the helper returns the writable path; we then try a
    ; hostfs CREATE_WRITE on it to land the file in the writable SYS
    ; overlay. Gap 2: a CREATE_WRITE failure is now a hard abort (no
    ; silent RAM synthesis) so `writes target the first writable
    ; effective target` means what the plan says.
    move.q  r1, r23
    load.q  r2, 960(r29)
    jsr     .dos_hostfs_layered_relpath_for_resolved_name
    beqz    r3, .dos_open_meta_lookup
    load.q  r20, 960(r29)              ; helper clobbers caller-save regs
    move.q  r24, r1
    beqz    r20, .dos_open_hostfs_try
    bra     .dos_open_hostfs_write_try
.dos_open_hostfs_try:
    store.q r24, 608(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_stat
    beqz    r3, .dos_open_hostfs_stat_ok
    move.l  r28, #ERR_NOTFOUND
    beq     r3, r28, .dos_open_meta_lookup
    bra     .dos_reply_err
.dos_open_hostfs_stat_ok:
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_FILE
    bne     r28, r15, .dos_reply_err
    move.q  r25, r1
    store.q r25, 632(r29)
    load.q  r24, 608(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_open
    bnez    r2, .dos_reply_err
    move.q  r24, r1
    store.q r24, 616(r29)
    load.q  r25, 632(r29)
    beqz    r25, .dos_open_hostfs_buf_ready
    push    r29
    move.q  r1, r25
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_open_hostfs_nomem_close
    store.q r1, 624(r29)
    load.q  r24, 616(r29)
    move.q  r1, r24
    load.q  r2, 624(r29)
    move.q  r3, r25
    jsr     .dos_bootfs_read_all
    move.q  r26, r2
    load.q  r24, 616(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_close
    bnez    r26, .dos_open_hostfs_free_tmp_err
    load.q  r25, 632(r29)
    bra     .dos_open_hostfs_buf_have
.dos_open_hostfs_buf_ready:
    move.q  r1, r24
    jsr     .dos_bootfs_close
    store.q r0, 624(r29)
.dos_open_hostfs_buf_have:
    load.q  r25, 632(r29)
    move.q  r1, r25
    load.q  r2, 624(r29)
    jsr     .dos_hostrec_alloc
    beqz    r2, .dos_open_hostfs_hnd
    load.q  r24, 624(r29)
    beqz    r24, .dos_reply_full
    push    r29
    move.q  r1, r24
    move.q  r2, r25
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_reply_full
.dos_open_hostfs_nomem_close:
    load.q  r24, 616(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_close
    bra     .dos_reply_full
.dos_open_hostfs_free_tmp:
    load.q  r24, 624(r29)
    beqz    r24, .dos_open_meta_lookup
    push    r29
    move.q  r1, r24
    load.q  r2, 632(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_open_meta_lookup
.dos_open_hostfs_free_tmp_err:
    load.q  r24, 624(r29)
    beqz    r24, .dos_reply_err
    push    r29
    move.q  r1, r24
    load.q  r2, 632(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_reply_err
.dos_open_hostfs_hnd:
    move.q  r25, r1
    jsr     .dos_hnd_alloc
    beqz    r2, .dos_open_hostfs_reply
    move.q  r1, r25
    jsr     .dos_hostrec_free
    bra     .dos_reply_full
.dos_open_hostfs_reply:
    move.q  r18, r1
    load.q  r29, (sp)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r18
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; M15.3: write-mode hostfs entry. r24 = writable SYS-overlay relpath
    ; from .dos_hostfs_layered_relpath_for_resolved_name. We attempt
    ; BOOT_HOSTFS_CREATE_WRITE on it; on success a hostrec-write handle is
    ; returned. Gap 2: a failure here is a HARD ABORT — the plan's
    ; "writes target the first writable effective target" and "fails
    ; cleanly when no writable target exists" require the write to
    ; stop at the chosen writable candidate. Silently diverting the
    ; bytes into a RAM-metadata create with a hostfs-looking name was
    ; the old M15.2 behaviour and is what the reviewer flagged as the
    ; last semantic gap; we now reply the first hard error instead.
    ; RAM-backed targets (T:/RAM:/user-custom) still reach this path
    ; via .dos_open_meta_lookup since layered_relpath returns r3=0 for
    ; them, so the RAM create path below is only reached when the
    ; current attempt's candidate is genuinely RAM-backed.
.dos_open_hostfs_write_try:
    store.q r24, 608(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_create_write
    bnez    r2, .dos_reply_err
    move.q  r24, r1                    ; r24 = host write handle
    store.q r24, 616(r29)
    move.q  r1, r24
    jsr     .dos_hostrec_write_alloc
    bnez    r2, .dos_open_hostfs_write_no_rec
    move.q  r25, r1                    ; r25 = tagged hostrec ptr
    jsr     .dos_hnd_alloc
    bnez    r2, .dos_open_hostfs_write_no_hnd
    move.q  r18, r1
    load.q  r29, (sp)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r18
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_open_hostfs_write_no_hnd:
    move.q  r1, r25
    jsr     .dos_hostrec_write_free
    bra     .dos_reply_full
.dos_open_hostfs_write_no_rec:
    load.q  r24, 616(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_close
    bra     .dos_reply_full

 .dos_open_meta_lookup:
    ; --- Search metadata chain for matching name ---
    load.q  r23, 336(r29)
    load.q  r20, 960(r29)
    move.q  r1, r23                     ; r1 = request name ptr
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    load.q  r29, (sp)
    load.q  r23, 336(r29)
    load.q  r20, 960(r29)
    bnez    r1, .dos_open_have_entry
    ; Not found
    bnez    r20, .dos_open_create       ; WRITE → create new
    ; M15.3: READ-mode miss — bump the attempt index and re-enter
    ; resolution so the next overlay entry (or the base/table target)
    ; gets tried. Resolution will return not-found (r22=0) once the
    ; effective list is exhausted, which falls out to .dos_reply_err at
    ; the top of .dos_do_open. Stack shape matches entry (inner jsrs are
    ; balanced), so we re-enter without pushing.
    load.q  r28, 648(r29)
    add     r28, r28, #1
    store.q r28, 648(r29)
    bra     .dos_do_open_retry

.dos_open_create:
    ; M12.8 Phase 2: write-mode create no longer pre-allocates a body.
    ; entry.file_va starts as 0 (empty file). The first DOS_WRITE will
    ; allocate an extent chain via .dos_extent_alloc and link it in.
    ; Allocate a fresh metadata entry.
    jsr     .dos_meta_alloc_entry       ; r1 = entry VA, r2 = err
    bnez    r2, .dos_reply_full
    load.q  r23, 336(r29)
    move.q  r25, r1                     ; r25 = entry VA
    ; entry.file_va = 0 (empty body)
    store.q r0, DOS_META_OFF_VA(r25)
    ; Copy filename from request buffer to entry.name (max 31 + NUL)
    move.q  r16, r23                    ; src = request name
    move.q  r17, r25                    ; dst = entry.name
    move.l  r18, #0
.dos_cpy_fname:
    load.b  r15, (r16)
    store.b r15, (r17)
    beqz    r15, .dos_cpy_fname_done
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #31
    blt     r18, r28, .dos_cpy_fname
    store.b r0, (r17)
.dos_cpy_fname_done:
    ; entry.size = 0
    store.l r0, DOS_META_OFF_SIZE(r25)
    move.q  r1, r25                     ; r1 = entry VA for handle alloc
    bra     .dos_open_have_entry

.dos_open_have_entry:
    ; r1 = entry VA. Allocate a handle slot referencing this entry.
    jsr     .dos_hnd_alloc              ; r1 = handle_id, r2 = err
    bnez    r2, .dos_reply_full
    move.q  r18, r1                     ; r18 = handle_id

    ; Reply: type=DOS_OK, data0=handle_id
    load.q  r29, (sp)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r18                     ; data0 = handle_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_READ (type=2): read file data into caller's shared buffer.
    ; M12.8 Phase 2: file body is now an extent chain. The body VA at
    ; DOS_META_OFF_VA is the first_extent_va of the chain (or 0 for an
    ; empty file). The clamped max_bytes is read via .dos_extent_walk.
    ; =================================================================
    ; data0 = handle, data1 = max_bytes
.dos_do_read:
    load.q  r1, 960(r29)               ; r1 = handle_id
    load.q  r19, 968(r29)              ; r19 = max_bytes
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0)
    beqz    r1, .dos_read_badh
    move.q  r18, r2                     ; r18 = slot VA
    and     r24, r1, #1
    bnez    r24, .dos_read_hostrec
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r29, (sp)
    load.q  r20, DOS_META_OFF_VA(r14)  ; r20 = first extent VA (or 0)
    load.l  r16, DOS_META_OFF_SIZE(r14)
    ; Clamp max_bytes to file size
    blt     r19, r16, .dos_read_clamp
    move.q  r19, r16
.dos_read_clamp:
    ; Clamp max_bytes to share size in bytes (share_pages << 12).
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    blt     r19, r24, .dos_read_share_ok
    move.q  r19, r24
.dos_read_share_ok:
    ; Empty body shortcut: file_va == 0 → read 0 bytes (skip the walk).
    move.q  r17, r0                     ; bytes copied = 0
    beqz    r20, .dos_read_reply
    beqz    r19, .dos_read_reply
    ; Walk the extent chain into the caller's shared buffer.
    move.q  r1, r20                     ; r1 = first extent VA
    load.q  r2, 168(r29)                ; r2 = dst = caller's mapped buffer
    move.q  r3, r19                     ; r3 = byte_count
    jsr     .dos_extent_walk            ; r1 = bytes copied
    move.q  r17, r1
    load.q  r29, (sp)
.dos_read_reply:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r17                     ; data0 = bytes read
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_read_hostrec:
    sub     r1, r1, #1
    move.q  r14, r1
    load.q  r29, (sp)
    load.q  r16, 8(r14)                ; host file size
    blt     r19, r16, .dos_read_host_clamp
    move.q  r19, r16
.dos_read_host_clamp:
    load.q  r24, 184(r29)
    lsl     r24, r24, #12
    blt     r19, r24, .dos_read_host_share_ok
    move.q  r19, r24
.dos_read_host_share_ok:
    beqz    r19, .dos_read_host_reply
    load.q  r20, 16(r14)
    beqz    r20, .dos_reply_err
    load.q  r21, 168(r29)
    move.l  r22, #0
.dos_read_host_copy:
    bge     r22, r19, .dos_read_host_copy_done
    add     r23, r20, r22
    load.b  r24, (r23)
    add     r23, r21, r22
    store.b r24, (r23)
    add     r22, r22, #1
    bra     .dos_read_host_copy
.dos_read_host_copy_done:
    move.q  r17, r19
    bra     .dos_read_host_reply_set
.dos_read_host_reply:
    move.q  r17, r0
.dos_read_host_reply_set:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r17
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_read_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_WRITE (type=3): write data from caller's buffer to file.
    ; M12.8 Phase 2: file body is now an extent chain. The per-file
    ; DOS_FILE_SIZE cap has been removed; the only cap is share size.
    ;
    ; Atomic-swap-on-rewrite rule: allocate a NEW extent chain for the
    ; new content; on alloc failure, leave the OLD chain intact and
    ; reply DOS_ERR_FULL. Only after the new chain is fully allocated
    ; AND the new bytes are copied in do we (a) point entry.file_va at
    ; the new chain and (b) free the old chain. A failed write therefore
    ; never corrupts the previous file content.
    ;
    ; Handler scratch slots in dos.library data page:
    ;   256: entry_va        (8 bytes — survives helper JSRs)
    ;   264: old_first_va    (8 bytes — for atomic swap)
    ;   272: clamped_count   (8 bytes — clamped byte_count)
    ; =================================================================
    ; data0 = handle, data1 = byte_count
.dos_do_write:
    load.q  r1, 960(r29)               ; r1 = handle_id
    load.q  r19, 968(r29)              ; r19 = byte_count (raw)
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0)
    beqz    r1, .dos_write_badh
    and     r24, r1, #1
    beqz    r24, .dos_write_meta_path
    ; M15.3: tagged hostrec. If it's a hostrec-write (HSTW), stream bytes
    ; through BOOT_HOSTFS_WRITE. Hostrec-read handles still reject writes.
    push    r19
    push    r1
    jsr     .dos_hostrec_is_write
    pop     r1
    pop     r19
    beqz    r3, .dos_write_badh
    ; r1 is still the tagged hostrec ptr.
    sub     r24, r1, #1                 ; r24 = hostrec base
    load.q  r25, 8(r24)                 ; host handle
    load.q  r29, (sp)
    ; Clamp byte_count to share-bytes (mirror RAM-extent path bound).
    load.q  r26, 184(r29)
    lsl     r26, r26, #12
    blt     r19, r26, .dwh_share_ok
    move.q  r19, r26
.dwh_share_ok:
    store.q r25, 256(r29)              ; preserve host handle
    store.q r19, 272(r29)              ; clamped byte_count
    store.q r24, 264(r29)              ; preserve hostrec base
    beqz    r19, .dwh_done
    load.q  r2, 168(r29)               ; src = caller's mapped buffer
    move.q  r3, r19
    move.q  r1, r25
    jsr     .dos_bootfs_write
    bnez    r2, .dos_write_full
    ; M15.3 correctness: BOOT_HOSTFS_WRITE returns the actual byte count
    ; in r1. Trust that over the clamped request in r19 — if the host
    ; fs layer reports a short write (e.g. partial write accepted by a
    ; non-regular backend) we must NOT advance hostrec.bytes_written or
    ; reply.data0 past what actually reached disk, or the guest will
    ; silently corrupt its logical file contents.
    load.q  r29, (sp)
    store.q r1, 272(r29)               ; replace clamped count with actual
    load.q  r24, 264(r29)
    load.q  r19, 272(r29)
    load.q  r25, 16(r24)
    add     r25, r25, r19
    store.q r25, 16(r24)               ; bytes_written += actual
.dwh_done:
    load.q  r19, 272(r29)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r19                    ; reply.data0 = actual byte count
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_write_meta_path:
    load.q  r29, (sp)
    store.q r1, 256(r29)                ; saved entry VA
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r4, DOS_META_OFF_VA(r14)
    store.q r4, 264(r29)                ; saved old_first_va

    ; Clamp byte_count to share size in bytes (share_pages << 12).
    ; The previous DOS_FILE_SIZE cap is removed; the only remaining
    ; bound is the size of the caller's mapped share.
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    blt     r19, r24, .dos_write_share_ok
    move.q  r19, r24
.dos_write_share_ok:
    store.q r19, 272(r29)               ; saved clamped byte_count

    ; ---- Allocate the new extent chain (size = clamped byte_count) ----
    ; .dos_extent_alloc returns r1=0 if byte_count==0 (legitimate empty
    ; write) and r2=ERR_OK in that case — handled below at .dwr_no_alloc.
    move.q  r1, r19
    jsr     .dos_extent_alloc           ; r1 = new_first_va, r2 = err
    bnez    r2, .dos_write_full
    load.q  r29, (sp)
    move.q  r25, r1                     ; r25 = new_first_va (may be 0)

    ; ---- Copy bytes from caller's share into the new chain ----
    beqz    r25, .dwr_no_alloc          ; empty write → skip extent_write
    move.q  r1, r25                     ; r1 = new_first_va
    load.q  r2, 168(r29)                ; r2 = src = caller's mapped buffer
    load.q  r3, 272(r29)                ; r3 = clamped byte_count
    jsr     .dos_extent_write
    load.q  r29, (sp)
.dwr_no_alloc:
    ; ---- Atomic swap: link new chain into entry, then free old ----
    load.q  r14, 256(r29)               ; reload entry VA
    store.q r25, DOS_META_OFF_VA(r14)   ; entry.file_va = new_first_va
    load.q  r19, 272(r29)               ; clamped byte_count
    store.l r19, DOS_META_OFF_SIZE(r14) ; entry.size = byte_count

    ; Free the old chain (no-op if old_first_va == 0)
    load.q  r1, 264(r29)
    beqz    r1, .dwr_done
    jsr     .dos_extent_free
    load.q  r29, (sp)
.dwr_done:
    ; Reply DOS_OK with bytes_written = clamped byte_count
    load.q  r19, 272(r29)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r19                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_write_full:
    ; Allocation failed; .dos_extent_alloc has already freed any
    ; partially-allocated chain via its internal cleanup. The entry
    ; is untouched, so the previous file content is intact.
    load.q  r29, (sp)
    bra     .dos_reply_full
.dos_write_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_CLOSE (type=4): close a file handle.
    ; M12.6 Phase A: walks the handle chain via .dos_hnd_lookup; clears
    ; the slot in place. The file body and metadata entry persist.
    ; =================================================================
    ; data0 = handle_id
.dos_do_close:
    load.q  r1, 960(r29)               ; r1 = handle_id
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0), r2 = slot VA
    beqz    r1, .dos_close_badh
    and     r24, r1, #1
    beqz    r24, .dos_close_clear
    ; M15.3: dispatch read vs write hostrec on close.
    push    r2
    push    r1
    jsr     .dos_hostrec_is_write
    pop     r1
    pop     r2
    beqz    r3, .dos_close_hostrec_read
    push    r2
    jsr     .dos_hostrec_write_free
    pop     r2
    bra     .dos_close_clear
.dos_close_hostrec_read:
    push    r2
    jsr     .dos_hostrec_free
    pop     r2
.dos_close_clear:
    ; Clear the slot
    store.q r0, (r2)
    load.q  r29, (sp)
    ; Reply success
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_close_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_ASSIGN (type=10): list/query/set DOS assigns
    ; share buffer uses rows of [name[16], target[16]]
    ; data0 selects sub-op: LIST / QUERY / SET
    ; =================================================================
.dos_do_assign:
    load.l  r21, 976(r29)              ; incoming share_handle required
    beqz    r21, .dos_reply_badarg
    load.q  r20, 960(r29)              ; sub-op
    move.l  r28, #DOS_ASSIGN_LIST
    beq     r20, r28, .dos_assign_list
    move.l  r28, #DOS_ASSIGN_QUERY
    beq     r20, r28, .dos_assign_query
    move.l  r28, #DOS_ASSIGN_SET
    beq     r20, r28, .dos_assign_set
    move.l  r28, #DOS_ASSIGN_LAYERED_QUERY
    beq     r20, r28, .dos_assign_layered_query
    move.l  r28, #DOS_ASSIGN_ADD
    beq     r20, r28, .dos_assign_add
    move.l  r28, #DOS_ASSIGN_REMOVE
    beq     r20, r28, .dos_assign_remove
    bra     .dos_reply_badarg

.dos_assign_list:
    load.q  r20, 168(r29)              ; dest ptr
    load.q  r24, 184(r29)              ; share_pages
    lsl     r24, r24, #12              ; share_bytes
    add     r21, r29, #(prog_doslib_assign_table - prog_doslib_data)
    move.l  r22, #0                    ; row index
    move.l  r23, #0                    ; rows written
.dos_assign_list_loop:
    move.l  r28, #DOS_ASSIGN_TABLE_COUNT
    bge     r22, r28, .dos_assign_list_done
    load.b  r25, (r21)
    beqz    r25, .dos_assign_list_next
    add     r25, r23, #1
    lsl     r25, r25, #5
    bgt     r25, r24, .dos_reply_full
    move.l  r26, #0
.dos_assign_list_copy:
    move.l  r28, #DOS_ASSIGN_ENTRY_SZ
    bge     r26, r28, .dos_assign_list_copied
    add     r27, r21, r26
    load.b  r28, (r27)
    add     r27, r20, r26
    store.b r28, (r27)
    add     r26, r26, #1
    bra     .dos_assign_list_copy
.dos_assign_list_copied:
    ; M15.3: if this slot has a non-empty overlay, overwrite the target
    ; portion of the just-emitted row with the first overlay target so the
    ; old LIST projection reflects the layered model (first effective only).
    store.q r20, 304(r29)
    store.q r21, 312(r29)
    store.q r22, 320(r29)
    store.q r23, 328(r29)
    move.q  r1, r22
    jsr     .dos_assign_overlay_first_target_for_slot
    load.q  r20, 304(r29)
    load.q  r21, 312(r29)
    load.q  r22, 320(r29)
    load.q  r23, 328(r29)
    load.q  r24, 184(r29)
    lsl     r24, r24, #12
    beqz    r3, .dos_assign_list_no_overlay
    move.q  r26, r1                    ; overlay target ptr
    add     r25, r20, #DOS_ASSIGN_TARGET_OFF
    move.l  r27, #0
.dosal_zero_tgt:
    move.l  r28, #DOS_ASSIGN_TARGET_MAX
    bge     r27, r28, .dosal_copy_ovl
    add     r28, r25, r27
    store.b r0, (r28)
    add     r27, r27, #1
    bra     .dosal_zero_tgt
.dosal_copy_ovl:
    move.l  r27, #0
.dosal_copy_loop:
    move.l  r28, #DOS_ASSIGN_OVERLAY_TGT_SZ
    bge     r27, r28, .dos_assign_list_no_overlay
    add     r28, r26, r27
    load.b  r3, (r28)
    beqz    r3, .dos_assign_list_no_overlay
    add     r28, r25, r27
    store.b r3, (r28)
    add     r27, r27, #1
    bra     .dosal_copy_loop
.dos_assign_list_no_overlay:
    add     r20, r20, #DOS_ASSIGN_ENTRY_SZ
    add     r23, r23, #1
.dos_assign_list_next:
    add     r21, r21, #DOS_ASSIGN_ENTRY_SZ
    add     r22, r22, #1
    bra     .dos_assign_list_loop
.dos_assign_list_done:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r23
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_assign_query:
    load.q  r1, 168(r29)
    add     r3, r29, #192
    jsr     .dos_assign_read_name
    beqz    r3, .dos_reply_badarg
    store.q r2, 240(r29)
    jsr     .dos_assign_builtin_root_query_row
    bnez    r3, .dos_assign_query_copy
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_reply_err
    store.q r1, 304(r29)               ; preserve table entry ptr (M15.3 scratch)
    store.q r2, 312(r29)               ; preserve slot index
    move.q  r1, r2
    jsr     .dos_assign_overlay_first_target_for_slot
    beqz    r3, .dos_assign_query_use_table
    ; M15.3: overlay non-empty — emit synthetic row with table name + overlay tgt.
    move.q  r24, r1                    ; r24 = overlay target ptr
    load.q  r25, 168(r29)              ; share buffer
    load.q  r26, 304(r29)              ; table entry ptr
    ; Copy 16-byte name from table.
    load.q  r27, (r26)
    store.q r27, (r25)
    load.q  r27, 8(r26)
    store.q r27, 8(r25)
    ; Copy 16-byte overlay target into share[16..31] (zero-pad first).
    move.q  r20, r0
    add     r21, r25, #DOS_ASSIGN_TARGET_OFF
.daq_zero_ovl:
    move.l  r22, #DOS_ASSIGN_TARGET_MAX
    bge     r20, r22, .daq_copy_ovl
    add     r23, r21, r20
    store.b r0, (r23)
    add     r20, r20, #1
    bra     .daq_zero_ovl
.daq_copy_ovl:
    move.q  r20, r0
.daq_copy_ovl_loop:
    move.l  r22, #DOS_ASSIGN_OVERLAY_TGT_SZ
    bge     r20, r22, .dos_assign_query_done
    add     r23, r24, r20
    load.b  r3, (r23)
    beqz    r3, .dos_assign_query_done
    add     r23, r21, r20
    store.b r3, (r23)
    add     r20, r20, #1
    bra     .daq_copy_ovl_loop
.dos_assign_query_use_table:
    load.q  r1, 304(r29)
.dos_assign_query_copy:
    load.q  r21, 168(r29)
    load.q  r22, (r1)
    store.q r22, (r21)
    load.q  r22, 8(r1)
    store.q r22, 8(r21)
    load.q  r22, 16(r1)
    store.q r22, 16(r21)
    load.q  r22, 24(r1)
    store.q r22, 24(r21)
.dos_assign_query_done:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.l  r3, #1
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_assign_set:
    load.q  r1, 168(r29)
    add     r3, r29, #192
    jsr     .dos_assign_read_name
    beqz    r3, .dos_reply_badarg
    store.q r2, 240(r29)               ; normalized name len
    jsr     .dos_assign_builtin_root_row
    bnez    r3, .dos_reply_badarg
    add     r1, r29, #192
    load.q  r2, 240(r29)
    add     r3, r29, #(prog_doslib_assign_table - prog_doslib_data)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dos_reply_badarg
    load.q  r1, 168(r29)
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_validate_target
    beqz    r3, .dos_reply_badarg
    store.q r1, 224(r29)               ; canonical target ptr
    store.q r2, 232(r29)               ; canonical target len

    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_assign_set_new
    beqz    r2, .dos_reply_badarg
    move.q  r25, r1                    ; r25 = table entry ptr
    bra     .dos_assign_set_slot_ready
.dos_assign_set_new:
    jsr     .dos_assign_find_free_entry
    beqz    r3, .dos_reply_full
    move.q  r25, r1

.dos_assign_set_slot_ready:
    store.q r0, (r25)
    store.q r0, 8(r25)
    store.q r0, 16(r25)
    store.q r0, 24(r25)
    add     r26, r29, #192
    load.q  r27, 240(r29)
    move.l  r20, #0
.dos_assign_set_name_copy:
    bge     r20, r27, .dos_assign_set_target_copy_setup
    add     r21, r26, r20
    load.b  r22, (r21)
    add     r21, r25, r20
    store.b r22, (r21)
    add     r20, r20, #1
    bra     .dos_assign_set_name_copy
.dos_assign_set_target_copy_setup:
    load.q  r21, 224(r29)
    load.q  r22, 232(r29)
    move.l  r20, #0
.dos_assign_set_target_copy:
    bge     r20, r22, .dos_assign_set_target_copied
    add     r24, r21, r20
    load.b  r28, (r24)
    add     r24, r25, #DOS_ASSIGN_TARGET_OFF
    add     r24, r24, r20
    store.b r28, (r24)
    add     r20, r20, #1
    bra     .dos_assign_set_target_copy

    ; M15.3: after writing the table entry, also replace the overlay list of
    ; canonical layered assigns with [TARGET]. The overlay drives LAYERED_QUERY
    ; and the first-effective projection used by the old QUERY/LIST ops, while
    ; the table entry keeps path resolution (dos_assign_lookup → first hit)
    ; pointed at the user's new target. Non-layered slots are left alone.
.dos_assign_set_target_copied:
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_assign_set_done
    store.q r2, 316(r29)               ; preserve slot index (M15.3 scratch)
    move.q  r1, r2
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dos_assign_set_done
    load.q  r1, 316(r29)
    jsr     .dos_assign_overlay_for_slot
    move.q  r27, r1                    ; overlay block
    ; Zero block: 1 count byte + 7 pad + 4 × 16 targets.
    move.q  r20, r0
.das_ovl_clear:
    move.l  r21, #DOS_ASSIGN_OVERLAY_ENTRY_SZ
    bge     r20, r21, .das_ovl_fill
    add     r22, r27, r20
    store.b r0, (r22)
    add     r20, r20, #1
    bra     .das_ovl_clear
.das_ovl_fill:
    ; Copy canonical target into overlay slot 0 (block+8).
    load.q  r24, 224(r29)              ; canonical target ptr
    load.q  r25, 232(r29)              ; canonical target len
    add     r26, r27, #8
    move.q  r20, r0
.das_ovl_copy:
    bge     r20, r25, .das_ovl_copy_done
    add     r21, r24, r20
    load.b  r22, (r21)
    add     r21, r26, r20
    store.b r22, (r21)
    add     r20, r20, #1
    bra     .das_ovl_copy
.das_ovl_copy_done:
    move.l  r28, #1
    store.b r28, (r27)

.dos_assign_set_done:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.l  r3, #1
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; M15.3 DOS_ASSIGN_LAYERED_QUERY (sub-op 3)
    ; Returns the FULL effective ordered target list for one assign:
    ;   overlay entries (up to OVERLAY_MAX) followed by the canonical
    ;   built-in base list (writable SYS path, read-only IOSSYS path).
    ; Non-layered assigns and built-in roots project into a single-entry
    ; list (the table or synthetic-row target).
    ; share buffer in:  name (NUL-terminated)
    ; share buffer out: count × DOS_ASSIGN_LAYERED_TGT_SZ-byte target slots
    ; reply.data0 = count of effective targets
    ; =================================================================
.dos_assign_layered_query:
    load.q  r1, 168(r29)
    add     r3, r29, #192
    jsr     .dos_assign_read_name
    beqz    r3, .dos_reply_badarg
    store.q r2, 240(r29)               ; normalized name len

    ; Built-in roots (SYS, IOSSYS) project as single-entry effective lists
    ; using the QUERY synthetic row's target.
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_builtin_root_query_row
    beqz    r3, .dalq_user_lookup
    ; r1 = synthetic row pointer; emit one 32-byte target slot containing
    ; the synthetic row's target string.
    add     r24, r1, #DOS_ASSIGN_TARGET_OFF
    load.q  r25, 168(r29)              ; dest = share buffer base
    move.q  r1, r24
    move.q  r2, r25
    jsr     .dos_assign_layered_emit_target_slot
    move.l  r3, #1                     ; count = 1
    bra     .dalq_reply_count

.dalq_user_lookup:
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_reply_err
    move.q  r24, r2                    ; r24 = slot index
    store.q r24, 248(r29)              ; preserve slot

    load.q  r25, 168(r29)              ; dest cursor (share buffer base)
    store.q r25, 256(r29)
    move.q  r26, r0                    ; emitted count
    store.q r26, 264(r29)

    ; Emit overlay entries first. Dedup: if a slot-equal target is
    ; already in the buffer (e.g. user did `ASSIGN ADD C: C:` so overlay
    ; and base both hold "C/"), skip the duplicate. This keeps reply.data0
    ; == the number of distinct targets the caller sees in the buffer.
    move.q  r1, r24
    jsr     .dos_assign_overlay_for_slot
    move.q  r27, r1                    ; overlay block
    load.b  r28, (r27)                 ; overlay count
    move.q  r20, r0                    ; loop index
.dalq_overlay_loop:
    bge     r20, r28, .dalq_overlay_done
    move.l  r21, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r22, r20, r21
    add     r22, r22, #8
    add     r22, r27, r22              ; overlay entry ptr
    move.q  r1, r22
    load.q  r2, 168(r29)               ; share buffer base
    load.q  r3, 256(r29)               ; cursor
    jsr     .dos_assign_layered_target_already_emitted
    bnez    r3, .dalq_overlay_skip
    load.q  r25, 256(r29)
    move.q  r1, r22
    move.q  r2, r25
    jsr     .dos_assign_layered_emit_target_slot
    load.q  r25, 256(r29)
    add     r25, r25, #DOS_ASSIGN_LAYERED_TGT_SZ
    store.q r25, 256(r29)
    load.q  r26, 264(r29)
    add     r26, r26, #1
    store.q r26, 264(r29)
.dalq_overlay_skip:
    add     r20, r20, #1
    bra     .dalq_overlay_loop
.dalq_overlay_done:

    ; Emit base list entries if this slot is canonical layered. Otherwise
    ; emit the table entry's target as a single-entry list (when no overlay
    ; was emitted).
    load.q  r24, 248(r29)
    move.q  r1, r24
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dalq_emit_table_entry

    ; Layered: emit two base targets from prog_doslib_assign_base_table,
    ; with the same dedup guard as the overlay loop.
    load.q  r24, 248(r29)
    move.q  r20, r24
    lsl     r20, r20, #4               ; slot * 16
    add     r20, r20, #(prog_doslib_assign_base_table - prog_doslib_data)
    add     r20, r29, r20              ; base entry pair pointer
    ; first base: writable SYS overlay path
    load.q  r21, (r20)
    add     r21, r29, r21              ; absolute path string ptr
    move.q  r1, r21
    load.q  r2, 168(r29)
    load.q  r3, 256(r29)
    jsr     .dos_assign_layered_target_already_emitted
    bnez    r3, .dalq_skip_base_writable
    load.q  r24, 248(r29)
    move.q  r20, r24
    lsl     r20, r20, #4
    add     r20, r20, #(prog_doslib_assign_base_table - prog_doslib_data)
    add     r20, r29, r20
    load.q  r21, (r20)
    add     r21, r29, r21
    load.q  r25, 256(r29)
    move.q  r1, r21
    move.q  r2, r25
    jsr     .dos_assign_layered_emit_target_slot
    load.q  r25, 256(r29)
    add     r25, r25, #DOS_ASSIGN_LAYERED_TGT_SZ
    store.q r25, 256(r29)
    load.q  r26, 264(r29)
    add     r26, r26, #1
    store.q r26, 264(r29)
.dalq_skip_base_writable:
    ; second base: read-only IOSSYS path
    load.q  r24, 248(r29)
    move.q  r20, r24
    lsl     r20, r20, #4
    add     r20, r20, #(prog_doslib_assign_base_table - prog_doslib_data)
    add     r20, r29, r20
    add     r20, r20, #8
    load.q  r21, (r20)
    add     r21, r29, r21
    move.q  r1, r21
    load.q  r2, 168(r29)
    load.q  r3, 256(r29)
    jsr     .dos_assign_layered_target_already_emitted
    bnez    r3, .dalq_skip_base_readonly
    load.q  r24, 248(r29)
    move.q  r20, r24
    lsl     r20, r20, #4
    add     r20, r20, #(prog_doslib_assign_base_table - prog_doslib_data)
    add     r20, r29, r20
    add     r20, r20, #8
    load.q  r21, (r20)
    add     r21, r29, r21
    load.q  r25, 256(r29)
    move.q  r1, r21
    move.q  r2, r25
    jsr     .dos_assign_layered_emit_target_slot
    load.q  r25, 256(r29)
    add     r25, r25, #DOS_ASSIGN_LAYERED_TGT_SZ
    store.q r25, 256(r29)
    load.q  r26, 264(r29)
    add     r26, r26, #1
    store.q r26, 264(r29)
.dalq_skip_base_readonly:
    bra     .dalq_done

.dalq_emit_table_entry:
    ; Non-layered: if no overlay was emitted, emit the table entry's target.
    load.q  r26, 264(r29)
    bnez    r26, .dalq_done
    load.q  r24, 248(r29)
    move.l  r20, #DOS_ASSIGN_ENTRY_SZ
    mulu.q  r21, r24, r20              ; slot * 32
    add     r21, r21, #(prog_doslib_assign_table - prog_doslib_data)
    add     r21, r29, r21              ; table entry ptr
    add     r21, r21, #DOS_ASSIGN_TARGET_OFF
    load.q  r25, 256(r29)
    move.q  r1, r21
    move.q  r2, r25
    jsr     .dos_assign_layered_emit_target_slot
    load.q  r26, 264(r29)
    add     r26, r26, #1
    store.q r26, 264(r29)

.dalq_done:
    load.q  r3, 264(r29)
.dalq_reply_count:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; M15.3 DOS_ASSIGN_ADD (sub-op 4)
    ; Append TARGET to the mutable overlay list of a canonical layered
    ; assign. Duplicate-target add is a no-op (returns DOS_OK). Rejects:
    ;   - SYS / IOSSYS / RAM (built-in immutable)
    ;   - T (single-target writable)
    ;   - non-layered user assigns (slots not in DOS_ASSIGN_LAYERED_MASK)
    ;   - invalid TARGET (per .dos_assign_validate_target)
    ;   - overlay full (DOS_ERR_FULL)
    ; share buffer: row[name[16], target[16]]
    ; =================================================================
.dos_assign_add:
    load.q  r1, 168(r29)
    add     r3, r29, #192
    jsr     .dos_assign_read_name
    beqz    r3, .dos_reply_badarg
    store.q r2, 240(r29)
    ; Reject SYS / IOSSYS root names.
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_builtin_root_row
    bnez    r3, .dos_reply_badarg
    ; Find slot in table.
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_reply_badarg
    move.q  r24, r2                    ; slot index
    store.q r24, 248(r29)
    ; Reject if not canonical layered.
    move.q  r1, r24
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dos_reply_badarg
    ; Validate target (canonicalize into scratch at offset 208).
    load.q  r1, 168(r29)
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_validate_target
    beqz    r3, .dos_reply_badarg
    store.q r1, 224(r29)               ; canonical target ptr
    store.q r2, 232(r29)               ; canonical target len
    ; Get overlay block.
    load.q  r24, 248(r29)
    move.q  r1, r24
    jsr     .dos_assign_overlay_for_slot
    move.q  r27, r1                    ; overlay block ptr
    load.b  r28, (r27)                 ; current overlay count
    ; Duplicate check.
    move.q  r20, r0
.daa_dup_loop:
    bge     r20, r28, .daa_dup_done
    move.l  r21, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r22, r20, r21
    add     r22, r22, #8
    add     r22, r27, r22              ; existing overlay entry
    move.q  r1, r22
    load.q  r2, 232(r29)
    load.q  r3, 224(r29)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .daa_dup_noop
    add     r20, r20, #1
    bra     .daa_dup_loop
.daa_dup_done:
    ; Capacity check.
    move.l  r21, #DOS_ASSIGN_OVERLAY_MAX
    bge     r28, r21, .dos_reply_full
    ; Append.
    move.l  r21, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r22, r28, r21
    add     r22, r22, #8
    add     r22, r27, r22              ; new entry slot
    ; Zero the slot first.
    move.q  r20, r0
.daa_zero:
    move.l  r21, #DOS_ASSIGN_OVERLAY_TGT_SZ
    bge     r20, r21, .daa_copy
    add     r24, r22, r20
    store.b r0, (r24)
    add     r20, r20, #1
    bra     .daa_zero
.daa_copy:
    load.q  r25, 224(r29)              ; canonical target src
    load.q  r24, 232(r29)              ; canonical target len
    move.q  r20, r0
.daa_copy_loop:
    bge     r20, r24, .daa_copy_done
    add     r21, r25, r20
    load.b  r26, (r21)
    add     r21, r22, r20
    store.b r26, (r21)
    add     r20, r20, #1
    bra     .daa_copy_loop
.daa_copy_done:
    add     r28, r28, #1
    store.b r28, (r27)                 ; bump overlay count
.daa_dup_noop:
    ; Note: `ASSIGN ADD` intentionally does NOT mirror overlay[0] into the
    ; single-target table entry because doing so would shadow the canonical
    ; default (e.g. C/) and break command-search fallback when the added
    ; overlay target doesn't itself contain the command. The full
    ; multi-entry overlay fallthrough during path resolution is the proper
    ; fix (see plan's "first hit wins / missing earlier targets fall
    ; through"); it's deferred to a follow-up milestone.
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.l  r3, #1
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; M15.3 DOS_ASSIGN_REMOVE (sub-op 5)
    ; Remove TARGET from the mutable overlay list of a canonical layered
    ; assign. Built-in base entries cannot be removed and are not searched.
    ; If TARGET is not in the overlay, returns DOS_ERR_BADARG.
    ; share buffer: row[name[16], target[16]]
    ; =================================================================
.dos_assign_remove:
    load.q  r1, 168(r29)
    add     r3, r29, #192
    jsr     .dos_assign_read_name
    beqz    r3, .dos_reply_badarg
    store.q r2, 240(r29)
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_builtin_root_row
    bnez    r3, .dos_reply_badarg
    add     r1, r29, #192
    load.q  r2, 240(r29)
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_reply_badarg
    move.q  r24, r2
    store.q r24, 248(r29)
    move.q  r1, r24
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dos_reply_badarg
    load.q  r1, 168(r29)
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_validate_target
    beqz    r3, .dos_reply_badarg
    store.q r1, 224(r29)
    store.q r2, 232(r29)
    load.q  r24, 248(r29)
    move.q  r1, r24
    jsr     .dos_assign_overlay_for_slot
    move.q  r27, r1
    load.b  r28, (r27)
    ; Find target in overlay.
    move.q  r20, r0
.dar_find:
    bge     r20, r28, .dos_reply_badarg
    move.l  r21, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r22, r20, r21
    add     r22, r22, #8
    add     r22, r27, r22
    move.q  r1, r22
    load.q  r2, 232(r29)
    load.q  r3, 224(r29)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dar_found
    add     r20, r20, #1
    bra     .dar_find
.dar_found:
    ; Shift remaining entries down.
.dar_shift_loop:
    add     r21, r20, #1
    bge     r21, r28, .dar_shift_done
    move.l  r22, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r23, r20, r22
    add     r23, r23, #8
    add     r23, r27, r23              ; dst slot
    mulu.q  r24, r21, r22
    add     r24, r24, #8
    add     r24, r27, r24              ; src slot
    move.q  r25, r0
.dar_shift_byte:
    move.l  r26, #DOS_ASSIGN_OVERLAY_TGT_SZ
    bge     r25, r26, .dar_shift_next
    add     r26, r24, r25
    load.b  r3, (r26)
    add     r26, r23, r25
    store.b r3, (r26)
    add     r25, r25, #1
    bra     .dar_shift_byte
.dar_shift_next:
    add     r20, r20, #1
    bra     .dar_shift_loop
.dar_shift_done:
    ; Zero the now-vacated last slot.
    sub     r28, r28, #1
    move.l  r22, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r23, r28, r22
    add     r23, r23, #8
    add     r23, r27, r23
    move.q  r25, r0
.dar_clear:
    move.l  r26, #DOS_ASSIGN_OVERLAY_TGT_SZ
    bge     r25, r26, .dar_done
    add     r26, r23, r25
    store.b r0, (r26)
    add     r25, r25, #1
    bra     .dar_clear
.dar_done:
    store.b r28, (r27)                 ; new overlay count
    ; Note: see `.daa_dup_noop` — `ASSIGN REMOVE` likewise does not touch
    ; the table entry. The table entry tracks SET (which is deliberately
    ; destructive replace); ADD/REMOVE affect only the overlay list.
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.l  r3, #1
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_LOADSEG (type=7): load an ELF file into a DOS-owned seglist.
    ; Shared buffer contains the program name. Reply data0 = seglist VA.
    ; =================================================================
.dos_do_loadseg:
    ; M15.3 multi-entry fallthrough: 664(r29) = attempt index (reserved
    ; in the unused 648..744 dead-space slab). 360(r29) is off-limits
    ; because .dos_elf_build_seglist uses it for file_va. The opcode at
    ; 952(r29) disambiguates which retry label to re-enter inside
    ; .dos_run_reply_notfound.
    store.q r0, 664(r29)
.dos_do_loadseg_retry:
    load.q  r23, 168(r29)              ; name ptr in shared buffer
    store.q r0, 856(r29)               ; skip_overlay = 0
    load.q  r28, 664(r29)
    store.q r28, 880(r29)              ; attempt index
    jsr     .dos_resolve_cmd
    load.q  r29, (sp)
    beqz    r22, .dos_run_reply_notfound_final
    store.q r23, 320(r29)

    ; M15.3: prefer the writable SYS overlay on LOADSEG just like
    ; DOS_OPEN and DOS_RUN. Mode=0 routes the resolved name through
    ; layered_relpath so a command or library shadowed in SYS/<X>/
    ; loads before falling back to IOSSYS/<X>/.
    move.q  r1, r23
    move.l  r2, #0
    jsr     .dos_hostfs_layered_relpath_for_resolved_name
    beqz    r3, .dos_loadseg_meta_lookup
    move.q  r24, r1
    store.q r24, 608(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_stat
    beqz    r3, .dos_loadseg_host_stat_ok
    move.l  r28, #ERR_NOTFOUND
    beq     r3, r28, .dos_loadseg_meta_lookup
    bra     .dos_reply_err
.dos_loadseg_host_stat_ok:
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_FILE
    bne     r28, r15, .dos_reply_err
    move.q  r23, r1
    beqz    r23, .dos_reply_badarg
    store.q r23, 328(r29)

    push    r29
    move.q  r1, r23
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_reply_nomem
    store.q r1, 336(r29)

    load.q  r24, 608(r29)
    move.q  r1, r24
    jsr     .dos_bootfs_open
    bnez    r2, .dos_loadseg_host_free_tmp_err
    move.q  r20, r1

    move.q  r1, r20
    load.q  r2, 336(r29)
    load.q  r3, 328(r29)
    jsr     .dos_bootfs_read_all
    move.q  r24, r1
    move.q  r25, r2
    move.q  r1, r20
    jsr     .dos_bootfs_close
    bnez    r25, .dos_loadseg_host_free_tmp_err

    load.q  r1, 336(r29)
    move.q  r2, r24
    push    r24
    jsr     .dos_validate_loaded_elf_aslr_contract
    pop     r24
    bnez    r20, .dos_loadseg_host_badarg_free_tmp
    load.q  r1, 336(r29)
    move.q  r2, r24
    move.q  r30, r29
    jsr     .dos_elf_build_seglist
    move.q  r29, r30
    store.q r1, 344(r29)
    store.q r2, 352(r29)

    push    r29
    load.q  r1, 336(r29)
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
    pop     r29

    load.q  r1, 944(r29)
    load.q  r2, 352(r29)
    load.q  r3, 344(r29)
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_loadseg_host_free_tmp:
    push    r29
    load.q  r1, 336(r29)
    beqz    r1, .dos_loadseg_host_free_done
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
.dos_loadseg_host_free_done:
    pop     r29
    bra     .dos_loadseg_meta_lookup
.dos_loadseg_host_free_tmp_err:
    push    r29
    load.q  r1, 336(r29)
    beqz    r1, .dos_loadseg_host_free_done_err
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
.dos_loadseg_host_free_done_err:
    pop     r29
    bra     .dos_reply_err
.dos_loadseg_host_badarg_free_tmp:
    push    r29
    load.q  r1, 336(r29)
    beqz    r1, .dos_loadseg_host_badarg_free_done
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
.dos_loadseg_host_badarg_free_done:
    pop     r29
    bra     .dos_reply_badarg

.dos_loadseg_meta_lookup:
    load.q  r23, 320(r29)
    move.q  r1, r23
    jsr     .dos_meta_find_by_name
    beqz    r1, .dos_run_notfound
    move.q  r14, r1
    load.q  r29, (sp)

    load.q  r21, DOS_META_OFF_VA(r14)  ; first extent VA
    load.l  r23, DOS_META_OFF_SIZE(r14) ; file size
    beqz    r21, .dos_reply_badarg
    beqz    r23, .dos_reply_badarg

    ; Scratch:
    ; 320: first_extent_va
    ; 328: image_size
    ; 336: temp_buf_va
    ; 344: seglist_va
    ; 352: seglist_err
    store.q r21, 320(r29)
    store.q r23, 328(r29)

    push    r29
    move.q  r1, r23
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_reply_nomem
    store.q r1, 336(r29)

    load.q  r1, 320(r29)
    load.q  r2, 336(r29)
    load.q  r3, 328(r29)
    jsr     .dos_extent_walk
    load.q  r29, (sp)

    load.q  r1, 336(r29)               ; temp file VA
    load.q  r2, 328(r29)               ; file size
    jsr     .dos_validate_loaded_elf_aslr_contract
    bnez    r20, .dos_loadseg_meta_badarg_free
    load.q  r1, 336(r29)               ; temp file VA
    load.q  r2, 328(r29)               ; file size
    move.q  r30, r29
    jsr     .dos_elf_build_seglist
    move.q  r29, r30
    store.q r1, 344(r29)
    store.q r2, 352(r29)

    push    r29
    load.q  r1, 336(r29)
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
    pop     r29

    load.q  r1, 944(r29)
    load.q  r2, 352(r29)
    load.q  r3, 344(r29)
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_loadseg_meta_badarg_free:
    push    r29
    load.q  r1, 336(r29)
    load.q  r2, 328(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_reply_badarg

    ; =================================================================
    ; DOS_UNLOADSEG (type=8): free a DOS-owned seglist.
    ; data0 = seglist VA
    ; =================================================================
.dos_do_unloadseg:
    load.q  r20, 960(r29)              ; seglist VA
    load.q  r21, 176(r29)              ; head
    move.q  r22, r0                    ; prev = 0
.dos_ul_walk:
    beqz    r21, .dos_reply_badh
    beq     r21, r20, .dos_ul_found
    move.q  r22, r21
    load.q  r21, DOS_SEGLIST_NEXT(r21)
    bra     .dos_ul_walk
.dos_ul_found:
    load.q  r24, DOS_SEGLIST_NEXT(r21)
    beqz    r22, .dos_ul_head
    store.q r24, DOS_SEGLIST_NEXT(r22)
    bra     .dos_ul_free
.dos_ul_head:
    store.q r24, 176(r29)
.dos_ul_free:
    move.q  r1, r21
    jsr     .dos_seglist_free_unlinked
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_RUNSEG (type=9): launch a previously loaded DOS seglist.
    ; data0 = seglist VA, shared buffer = args string (or empty)
    ; The seglist remains DOS-owned; successful launch copies the image
    ; into a private child task via the M14 descriptor bridge.
    ; =================================================================
.dos_do_runseg:
    load.q  r20, 960(r29)              ; requested seglist VA
    load.q  r21, 176(r29)              ; head
    move.q  r22, r0
.dos_runseg_walk:
    beqz    r21, .dos_reply_badh
    beq     r21, r20, .dos_runseg_found
    move.q  r22, r21
    load.q  r21, DOS_SEGLIST_NEXT(r21)
    bra     .dos_runseg_walk

.dos_runseg_found:
.dos_runseg_args:
    load.q  r16, 168(r29)              ; args_ptr
    move.q  r17, r16
    move.l  r18, #0
.dos_runseg_arglen:
    load.b  r15, (r17)
    beqz    r15, .dos_runseg_launch
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r18, r28, .dos_runseg_arglen
    bra     .dos_reply_badarg

.dos_runseg_launch:
    move.q  r1, r21                     ; seglist VA
    move.q  r2, r16                     ; args_ptr
    move.q  r3, r18                     ; args_len
    move.q  r30, r29                    ; preserve DOS data-page base across helper return
    move.q  r19, r29                    ; explicit DOS data-page anchor
    jsr     .dos_launch_seglist
    move.q  r29, r30
    move.q  r5, r1                      ; task_id
    store.q r29, (sp)
    load.q  r1, 944(r29)                ; reply_port
    move.q  r3, r5                      ; data0 = task_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_boot_fail:
    move.q  r3, r2
    add     r1, r1, #0x30
    syscall #SYS_DEBUG_PUTCHAR
    add     r1, r3, #0x30
    syscall #SYS_DEBUG_PUTCHAR
    syscall #SYS_EXIT_TASK

    ; ------------------------------------------------------------------
    ; .dos_launch_seglist: launch a DOS-owned seglist through the M14
    ; descriptor bridge without consuming it.
    ; In:  r1 = seglist VA, r2 = args_ptr, r3 = args_len
    ; Out: r1 = task_id (or 0), r2 = DOS_OK / DOS_ERR_BADARG / DOS_ERR_NOMEM
    ; ------------------------------------------------------------------
.dos_launch_seglist:
    sub     sp, sp, #240
    store.q r30, 224(sp)
    store.q r19, 232(sp)
    move.q  r21, r1
    move.q  r16, r2
    move.q  r18, r3
    load.l  r23, DOS_SEGLIST_OFF_COUNT(r21)
    beqz    r23, .dos_launchseg_badarg
    store.q r21, 64(sp)
    store.q r16, 72(sp)
    store.q r18, 80(sp)
    store.q r23, 88(sp)

    ; Stack-local scratch:
    ;   0   code_target_base
    ;   8   code_target_end
    ;   16  data_target_base
    ;   24  data_target_end
    ;   32  code_tmp_va
    ;   40  data_tmp_va
    ;   48  code_pages
    ;   56  data_pages
    ;   64  seglist VA
    ;   72  args_ptr
    ;   80  args_len
    ;   88  seg_count
    ;   96  launch desc ptr
    ;   104 reserved
    ;   224 saved r30
    ;   232 saved r19
    store.q r0, 0(sp)
    store.q r0, 8(sp)
    store.q r0, 16(sp)
    store.q r0, 24(sp)
    store.q r0, 32(sp)
    store.q r0, 40(sp)
    store.q r0, 48(sp)
    store.q r0, 56(sp)
    store.q r0, 96(sp)
    store.q r0, 104(sp)

    move.q  r24, r21
    add     r24, r24, #DOS_SEGLIST_HDR_SZ
    move.l  r26, #0
    move.l  r27, #0                    ; code_found
    move.l  r28, #0                    ; data_found
    move.l  r25, #0                    ; exec_entry_ok
.dos_launchseg_scan:
    bge     r26, r23, .dos_launchseg_scanned
    load.q  r7, DOS_SEG_OFF_TARGET(r24)
    load.l  r8, DOS_SEG_OFF_PAGES(r24)
    beqz    r8, .dos_launchseg_badarg
    move.q  r9, r8
    lsl     r9, r9, #12
    add     r10, r7, r9
    blt     r10, r7, .dos_launchseg_badarg
    load.l  r11, DOS_SEG_OFF_FLAGS(r24)
    move.q  r12, r11
    and     r12, r12, #0xFFFFFFF8
    bnez    r12, .dos_launchseg_badarg
    beqz    r11, .dos_launchseg_badarg
    move.q  r12, r11
    move.q  r13, r0
    add     r13, r13, #2
    beq     r12, r13, .dos_launchseg_badarg
    move.q  r12, r11
    and     r12, r12, #3
    move.q  r13, r0
    add     r13, r13, #3
    beq     r12, r13, .dos_launchseg_badarg
    and     r12, r11, #1
    beqz    r12, .dos_launchseg_scan_data
    bnez    r27, .dos_launchseg_scan_code_seen
    move.l  r27, #1
    store.q r7, 0(sp)
    store.q r10, 8(sp)
    store.q r11, 104(sp)
    bra     .dos_launchseg_scan_exec
.dos_launchseg_scan_code_seen:
    load.q  r12, 0(sp)
    bge     r7, r12, .dos_launchseg_code_min_ok
    store.q r7, 0(sp)
.dos_launchseg_code_min_ok:
    load.q  r12, 8(sp)
    bge     r12, r10, .dos_launchseg_scan_exec
    store.q r10, 8(sp)
.dos_launchseg_scan_exec:
    and     r12, r11, #1
    beqz    r12, .dos_launchseg_scan_next
    load.q  r12, DOS_SEGLIST_OFF_ENTRY(r21)
    blt     r12, r7, .dos_launchseg_scan_next
    bge     r12, r10, .dos_launchseg_scan_next
    move.l  r25, #1
    bra     .dos_launchseg_scan_next
.dos_launchseg_scan_data:
    bnez    r28, .dos_launchseg_scan_data_seen
    move.l  r28, #1
    store.q r7, 16(sp)
    store.q r10, 24(sp)
    store.q r11, 112(sp)
    bra     .dos_launchseg_scan_next
.dos_launchseg_scan_data_seen:
    load.q  r12, 16(sp)
    bge     r7, r12, .dos_launchseg_data_min_ok
    store.q r7, 16(sp)
.dos_launchseg_data_min_ok:
    load.q  r12, 24(sp)
    bge     r12, r10, .dos_launchseg_scan_next
    store.q r10, 24(sp)
.dos_launchseg_scan_next:
    add     r24, r24, #DOS_SEG_ENTRY_SZ
    add     r26, r26, #1
    bra     .dos_launchseg_scan

.dos_launchseg_scanned:
    beqz    r27, .dos_launchseg_badarg
    beqz    r25, .dos_launchseg_badarg

    load.q  r3, 8(sp)
    load.q  r4, 0(sp)
    sub     r3, r3, r4
    lsr     r3, r3, #12
    beqz    r3, .dos_launchseg_badarg
    store.q r3, 48(sp)
    beqz    r28, .dos_launchseg_no_data
    load.q  r5, 24(sp)
    load.q  r6, 16(sp)
    sub     r5, r5, r6
    lsr     r5, r5, #12
    beqz    r5, .dos_launchseg_badarg
    store.q r5, 56(sp)
    bra     .dos_launchseg_alloc_code
.dos_launchseg_no_data:
    store.q r0, 56(sp)

.dos_launchseg_alloc_code:
    move.q  r1, r3
    lsl     r1, r1, #12
    move.l  r2, #MEMF_CLEAR
    push    r29
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_launchseg_nomem
    store.q r1, 32(sp)

    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    push    r29
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_launchseg_fail_free_code
    store.q r1, 96(sp)

    beqz    r28, .dos_launchseg_copy_loop
    move.q  r1, r5
    lsl     r1, r1, #12
    move.l  r2, #MEMF_CLEAR
    push    r29
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_launchseg_fail_free_desc
    store.q r1, 40(sp)

    load.q  r21, 64(sp)
    load.q  r23, 88(sp)
    move.q  r24, r21
    add     r24, r24, #DOS_SEGLIST_HDR_SZ
    move.l  r26, #0
.dos_launchseg_copy_loop:
    bge     r26, r23, .dos_launchseg_exec
    load.q  r7, DOS_SEG_OFF_MEMVA(r24)
    load.q  r8, DOS_SEG_OFF_FILESZ(r24)
    load.q  r9, DOS_SEG_OFF_TARGET(r24)
    load.l  r11, DOS_SEG_OFF_FLAGS(r24)
    and     r12, r11, #1
    beqz    r12, .dos_launchseg_copy_data
    load.q  r13, 32(sp)
    load.q  r14, 0(sp)
    sub     r14, r9, r14
    add     r13, r13, r14
    bra     .dos_launchseg_copy_bytes
.dos_launchseg_copy_data:
    load.q  r13, 40(sp)
    load.q  r14, 16(sp)
    sub     r14, r9, r14
    add     r13, r13, r14
.dos_launchseg_copy_bytes:
    move.l  r10, #0
.dos_launchseg_copy_byte_loop:
    bge     r10, r8, .dos_launchseg_copy_next
    add     r5, r7, r10
    load.b  r6, (r5)
    add     r5, r13, r10
    store.b r6, (r5)
    add     r10, r10, #1
    bra     .dos_launchseg_copy_byte_loop
.dos_launchseg_copy_next:
    add     r24, r24, #DOS_SEG_ENTRY_SZ
    add     r26, r26, #1
    bra     .dos_launchseg_copy_loop

.dos_launchseg_exec:
    load.q  r24, 96(sp)
    move.l  r3, #M14_LDESC_MAGIC
    store.l r3, M14_LDESC_OFF_MAGIC(r24)
    move.l  r3, #M14_LDESC_VERSION
    store.l r3, M14_LDESC_OFF_VERSION(r24)
    move.l  r3, #M14_LDESC_SIZE
    store.l r3, M14_LDESC_OFF_SIZE(r24)
    move.l  r3, #1
    bnez    r28, .dos_launchseg_two_segments
    bra     .dos_launchseg_store_segcnt
.dos_launchseg_two_segments:
    move.l  r3, #2
.dos_launchseg_store_segcnt:
    store.l r3, M14_LDESC_OFF_SEGCNT(r24)
    load.q  r3, DOS_SEGLIST_OFF_ENTRY(r21)
    store.q r3, M14_LDESC_OFF_ENTRY(r24)
    move.l  r3, #1
    store.l r3, M14_LDESC_OFF_STACKPG(r24)
    move.q  r3, r24
    add     r3, r3, #48
    store.q r3, M14_LDESC_OFF_SEGTBL(r24)

    move.q  r6, r24
    add     r6, r6, #48
    load.q  r7, 32(sp)
    store.q r7, M14_LDSEG_OFF_SRCPTR(r6)
    load.q  r7, 48(sp)
    lsl     r7, r7, #12
    store.q r7, M14_LDSEG_OFF_SRCSZ(r6)
    load.q  r7, 0(sp)
    store.q r7, M14_LDSEG_OFF_TARGET(r6)
    load.q  r7, 48(sp)
    store.l r7, M14_LDSEG_OFF_PAGES(r6)
    load.q  r7, 104(sp)
    store.l r7, M14_LDSEG_OFF_FLAGS(r6)

    beqz    r28, .dos_launchseg_exec_call
    move.q  r6, r24
    add     r6, r6, #80
    load.q  r7, 40(sp)
    store.q r7, M14_LDSEG_OFF_SRCPTR(r6)
    load.q  r7, 56(sp)
    lsl     r7, r7, #12
    store.q r7, M14_LDSEG_OFF_SRCSZ(r6)
    load.q  r7, 16(sp)
    store.q r7, M14_LDSEG_OFF_TARGET(r6)
    load.q  r7, 56(sp)
    store.l r7, M14_LDSEG_OFF_PAGES(r6)
    load.q  r7, 112(sp)
    store.l r7, M14_LDSEG_OFF_FLAGS(r6)

.dos_launchseg_exec_call:
    move.q  r1, r24
    move.l  r2, #M14_LDESC_SIZE
    load.q  r3, 72(sp)
    load.q  r4, 80(sp)
    push    r29
    syscall #SYS_EXEC_PROGRAM
    pop     r29
    move.q  r26, r1                    ; preserve task id
    move.q  r27, r2                    ; preserve exec error

    load.q  r1, 96(sp)
    beqz    r1, .dos_launchseg_free_data
    move.l  r2, #4096
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
.dos_launchseg_free_data:
    load.q  r1, 40(sp)
    beqz    r1, .dos_launchseg_free_code
    load.q  r2, 56(sp)
    lsl     r2, r2, #12
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
.dos_launchseg_free_code:
    load.q  r1, 32(sp)
    beqz    r1, .dos_launchseg_result
    load.q  r2, 48(sp)
    lsl     r2, r2, #12
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
.dos_launchseg_result:

    beqz    r27, .dos_launchseg_ok
    move.l  r1, #0x58
    syscall #SYS_DEBUG_PUTCHAR
    move.q  r1, r27
    add     r1, r1, #0x30
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r5, #ERR_NOMEM
    beq     r27, r5, .dos_launchseg_nomem
.dos_launchseg_badarg:
    move.l  r1, #0x50
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r19, 232(sp)
    load.q  r30, 224(sp)
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    add     sp, sp, #240
    rts
.dos_launchseg_fail_free_code:
    load.q  r1, 96(sp)
    beqz    r1, .dos_launchseg_fail_free_code_only
    move.l  r2, #4096
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
.dos_launchseg_fail_free_code_only:
    load.q  r1, 32(sp)
    beqz    r1, .dos_launchseg_nomem
    load.q  r2, 48(sp)
    lsl     r2, r2, #12
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_launchseg_nomem
.dos_launchseg_fail_free_desc:
    load.q  r1, 40(sp)
    beqz    r1, .dos_launchseg_fail_free_code
    load.q  r2, 56(sp)
    lsl     r2, r2, #12
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_launchseg_fail_free_code
.dos_launchseg_nomem:
    load.q  r19, 232(sp)
    load.q  r30, 224(sp)
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    add     sp, sp, #240
    rts
.dos_launchseg_ok:
    load.q  r19, 232(sp)
    load.q  r30, 224(sp)
    move.q  r1, r26
    move.l  r2, #DOS_OK
    add     sp, sp, #240
    rts

    ; ------------------------------------------------------------------
    ; M14.1 phase 3: embedded-manifest helpers
    ; ------------------------------------------------------------------

    ; .dos_manifest_launch_by_id
    ; In:  r1 = manifest entry ID, r2 = args_ptr, r3 = args_len
    ; Out: r1 = task_id (or 0), r2 = DOS_OK / DOS_ERR_*
.dos_manifest_launch_by_id:
    syscall #SYS_BOOT_MANIFEST
    beqz    r2, .dmli_ok
    move.l  r4, #ERR_NOMEM
    beq     r2, r4, .dmli_nomem
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dmli_nomem:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    rts
.dmli_ok:
    move.l  r2, #DOS_OK
    rts

DOS_ASSIGN_NAME_MAX   equ 16
DOS_ASSIGN_TARGET_OFF equ 16
DOS_ASSIGN_TARGET_MAX equ 16
DOS_ASSIGN_ENTRY_SZ   equ 32
DOS_ASSIGN_DEFAULT_COUNT equ 8
DOS_ASSIGN_TABLE_COUNT equ 16

; M15.3 layered assign overlay parameters. DOS_ASSIGN_OVERLAY_MAX and
; DOS_ASSIGN_LAYERED_TGT_SZ are inherited from iexec.inc; per-slot storage
; sizes below are private to dos.library.
DOS_ASSIGN_OVERLAY_TGT_SZ  equ 16   ; bytes per overlay target (matches table target slot)
DOS_ASSIGN_OVERLAY_ENTRY_SZ equ 72  ; per-slot block: 1 count byte + 7 pad + 4 × 16 targets

; M15.3 layered-slot bitmask: 1 bit per assign-table slot, set if the slot is
; a canonical layered assign (C, L, LIBS, DEVS, S, RESOURCES). Slot 0 = RAM
; (special, non-mutable) and slot 5 = T (single-target, writable, not layered)
; remain unmasked.
;   bit 0: RAM       — 0
;   bit 1: C         — 1
;   bit 2: L         — 1
;   bit 3: LIBS      — 1
;   bit 4: DEVS      — 1
;   bit 5: T         — 0
;   bit 6: S         — 1
;   bit 7: RESOURCES — 1
; mask = 0b11011110 = 0xDE
DOS_ASSIGN_LAYERED_MASK    equ 0xDE

    ; .dos_name_eq_ci32
    ; In:  r1 = ptr_a, r2 = ptr_b
    ; Out: r23 = 0 if equal, 1 if mismatch
.dos_name_eq_ci32:
    move.l  r20, #0
.dne_loop:
    move.l  r21, #32
    bge     r20, r21, .dne_match
    add     r22, r1, r20
    load.b  r22, (r22)
    add     r24, r2, r20
    load.b  r24, (r24)
    beqz    r22, .dne_a_zero
    beqz    r24, .dne_mismatch
    bra     .dne_fold_case
.dne_a_zero:
    beqz    r24, .dne_match
    bra     .dne_mismatch
.dne_fold_case:
    move.l  r25, #0x61
    blt     r22, r25, .dne_a_done
    move.l  r25, #0x7B
    bge     r22, r25, .dne_a_done
    sub     r22, r22, #0x20
.dne_a_done:
    move.l  r25, #0x61
    blt     r24, r25, .dne_b_done
    move.l  r25, #0x7B
    bge     r24, r25, .dne_b_done
    sub     r24, r24, #0x20
.dne_b_done:
    bne     r22, r24, .dne_mismatch
    add     r20, r20, #1
    bra     .dne_loop
.dne_match:
    move.q  r23, r0
    rts
.dne_mismatch:
    move.l  r23, #1
    rts

    ; .dos_manifest_find_row_by_name
    ; In:  r1 = resolved DOS name ptr
    ; Out: r1 = manifest entry ID (or 0), r2 = 1 if found, 0 if not found
.dos_manifest_find_row_by_name:
    move.q  r20, r1
    add     r2, r29, #(prog_doslib_seed_name_hwres - prog_doslib_data)
    jsr     .dos_name_eq_ci32
    beqz    r23, .dmfrn_hwres
    move.q  r1, r20
    add     r2, r29, #(prog_doslib_seed_name_input - prog_doslib_data)
    jsr     .dos_name_eq_ci32
    beqz    r23, .dmfrn_input
    move.q  r1, r20
    add     r2, r29, #(prog_doslib_seed_name_graphics - prog_doslib_data)
    jsr     .dos_name_eq_ci32
    beqz    r23, .dmfrn_graphics
    move.q  r1, r20
    add     r2, r29, #(prog_doslib_seed_name_intuition - prog_doslib_data)
    jsr     .dos_name_eq_ci32
    beqz    r23, .dmfrn_intuition
.dmfrn_notfound:
    move.q  r1, r0
    move.q  r2, r0
    rts
.dmfrn_hwres:
    move.l  r20, #BOOT_MANIFEST_ID_HWRES
    bra     .dmfrn_found
.dmfrn_input:
    move.l  r20, #BOOT_MANIFEST_ID_INPUT
    bra     .dmfrn_found
.dmfrn_graphics:
    move.l  r20, #BOOT_MANIFEST_ID_GRAPHICS
    bra     .dmfrn_found
.dmfrn_intuition:
    move.l  r20, #BOOT_MANIFEST_ID_INTUITION
    bra     .dmfrn_found
.dmfrn_found:
    move.q  r1, r20
    move.l  r2, #1
    rts

    ; .dos_manifest_launch_raw
    ; In:  r1 = raw executable ptr, r2 = raw size, r3 = args_ptr, r4 = args_len
    ; Used only by the internal embedded-manifest launch path; flat images are
    ; rejected by SYS_EXEC_PROGRAM in M14.2.
    ; Out: r1 = task_id (or 0), r2 = DOS_OK / DOS_ERR_*
.dos_manifest_launch_raw:
    syscall #SYS_EXEC_PROGRAM
    beqz    r2, .dmrl_ok
    move.l  r5, #ERR_NOMEM
    beq     r2, r5, .dmrl_nomem
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dmrl_nomem:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    rts
.dmrl_ok:
    move.l  r2, #DOS_OK
    rts

    ; .dos_launch_hostfs_resolved_name
    ; In:  r23 = resolved DOS name ptr
    ;      r16 = args_ptr
    ;      r18 = args_len
    ; Out: r1 = task_id (or 0), r2 = DOS_ERR_*
.dos_launch_hostfs_resolved_name:
    store.q r23, 336(r29)
    move.q  r1, r23
    jsr     .dos_hostfs_relpath_for_resolved_name
    beqz    r3, .dlhrn_notfound
    move.q  r24, r1
    move.q  r27, r24
    bra     .dlhrn_have_relpath
.dos_launch_hostfs_relpath_name:
    move.q  r24, r23
    move.q  r27, r24
    store.q r23, 888(r29)
    store.q r16, 672(r29)              ; saved args_ptr
    store.q r18, 680(r29)              ; saved args_len
    store.q r17, 688(r29)              ; saved boot-elf flags
.dlhrn_have_relpath:
    move.q  r1, r24
    jsr     .dos_bootfs_stat
    bnez    r3, .dlhrn_hosterr
    move.q  r25, r1
    beqz    r25, .dlhrn_badarg
    store.q r25, 624(r29)              ; preserve hostfs alloc size across syscalls

    move.q  r1, r27
    jsr     .dos_bootfs_open
    bnez    r2, .dlhrn_hosterr_r2
    move.q  r20, r1
    store.q r20, 616(r29)

    load.q  r25, 624(r29)
    push    r29
    move.q  r1, r25
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dlhrn_nomem_close
    store.q r1, 296(r29)

    load.q  r20, 616(r29)
    move.q  r1, r20
    load.q  r2, 296(r29)
    move.q  r3, r25
    jsr     .dos_bootfs_read_all
    store.q r1, 608(r29)
    move.q  r26, r2
    load.q  r20, 616(r29)
    move.q  r1, r20
    jsr     .dos_bootfs_close
    bnez    r26, .dlhrn_free_tmp_readerr

    load.q  r21, 296(r29)
    load.q  r23, 608(r29)
    load.l  r15, (r21)
    move.l  r28, #0x464C457F
    bne     r15, r28, .dlhrn_free_tmp_nonelf
    move.q  r1, r21
    move.q  r2, r23
    load.q  r14, 672(r29)
    load.q  r15, 680(r29)
    load.q  r16, 688(r29)
    push    r16
    push    r15
    push    r14
    push    r23
    push    r21
    jsr     .dos_validate_loaded_elf_aslr_contract
    pop     r21
    pop     r23
    pop     r14
    pop     r15
    pop     r16
    store.q r14, 672(r29)
    store.q r15, 680(r29)
    store.q r16, 688(r29)
    bnez    r20, .dlhrn_free_tmp_badarg

    push    r29
    move.q  r1, r21
    move.q  r2, r23
    load.q  r3, 672(r29)
    load.q  r4, 680(r29)
    load.q  r5, 688(r29)
    syscall #SYS_BOOT_ELF_EXEC
    pop     r29
    store.q r1, 304(r29)
    store.q r2, 312(r29)

    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29

    load.q  r1, 304(r29)
    load.q  r2, 312(r29)
    rts

.dlhrn_nomem_close:
    load.q  r20, 616(r29)
    move.q  r1, r20
    jsr     .dos_bootfs_close
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    rts
.dlhrn_free_tmp_badarg:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dlhrn_free_tmp_readerr:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    move.q  r1, r0
    move.q  r2, r26
    rts
.dlhrn_free_tmp_nonelf:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dlhrn_badarg:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dlhrn_nonfile:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts
.dlhrn_hosterr:
    move.q  r1, r0
    move.q  r20, r3
    bra     .dlhrn_map_err
.dlhrn_hosterr_r2:
    move.q  r1, r0
    move.q  r20, r2
.dlhrn_map_err:
    move.l  r21, #ERR_NOTFOUND
    bne     r20, r21, .dlhrn_map_nomem
    move.l  r2, #DOS_ERR_NOTFOUND
    rts
.dlhrn_map_nomem:
    move.l  r21, #ERR_NOMEM
    bne     r20, r21, .dlhrn_map_badarg
    move.l  r2, #DOS_ERR_NOMEM
    rts
.dlhrn_map_badarg:
    move.l  r2, #DOS_ERR_BADARG
    rts
.dlhrn_notfound:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOTFOUND
    rts

    ; =================================================================
    ; Name resolution subroutines
    ; =================================================================

    ; .dos_assign_name_eq_ci
    ; In:  r1 = input name ptr, r2 = input name len, r3 = entry name ptr
    ; Out: r23 = 0 if equal, 1 if mismatch
.dos_assign_name_eq_ci:
    move.l  r20, #0
.dane_loop:
    bge     r20, r2, .dane_input_done
    add     r21, r1, r20
    load.b  r21, (r21)
    add     r24, r3, r20
    load.b  r24, (r24)
    beqz    r24, .dane_mismatch
    move.l  r25, #0x61
    blt     r21, r25, .dane_a_done
    move.l  r25, #0x7B
    bge     r21, r25, .dane_a_done
    sub     r21, r21, #0x20
.dane_a_done:
    move.l  r25, #0x61
    blt     r24, r25, .dane_b_done
    move.l  r25, #0x7B
    bge     r24, r25, .dane_b_done
    sub     r24, r24, #0x20
.dane_b_done:
    bne     r21, r24, .dane_mismatch
    add     r20, r20, #1
    bra     .dane_loop
.dane_input_done:
    add     r24, r3, r2
    load.b  r24, (r24)
    beqz    r24, .dane_match
.dane_mismatch:
    move.l  r23, #1
    rts
.dane_match:
    move.q  r23, r0
    rts

    ; .dos_assign_target_len
    ; In:  r1 = target ptr
    ; Out: r2 = target len
.dos_assign_target_len:
    move.l  r2, #0
.datl_loop:
    add     r24, r1, r2
    load.b  r25, (r24)
    beqz    r25, .datl_done
    add     r2, r2, #1
    move.l  r24, #DOS_ASSIGN_TARGET_MAX
    blt     r2, r24, .datl_loop
.datl_done:
    rts

    ; .dos_assign_find_entry
    ; In:  r1 = input name ptr, r2 = input name len, r29 = data base
    ; Out: r1 = entry ptr, r2 = index, r3 = 1 if found, 0 otherwise
    ; M15.3 fix: previously the loop counter r20 was shared with
    ; .dos_assign_name_eq_ci (which clobbers r20), silently producing the
    ; wrong slot index for any assign past slot 1 (only "C" worked by
    ; coincidence). The counter is now preserved across the JSR via the
    ; stack so external callers that rely on r27/r28 being preserved
    ; (e.g. .dos_resolve_no_slash, .dos_resolve_has_colon) keep working.
.dos_assign_find_entry:
    add     r21, r29, #(prog_doslib_assign_table - prog_doslib_data)
    move.l  r20, #0
.dafe_loop:
    move.l  r24, #DOS_ASSIGN_TABLE_COUNT
    bge     r20, r24, .dafe_notfound
    load.b  r24, (r21)
    beqz    r24, .dafe_next_empty
    move.q  r26, r21
    move.q  r3, r26
    push    r20
    push    r21
    jsr     .dos_assign_name_eq_ci
    pop     r21
    pop     r20
    beqz    r23, .dafe_found
.dafe_next:
    add     r21, r26, #DOS_ASSIGN_ENTRY_SZ
    add     r20, r20, #1
    bra     .dafe_loop
.dafe_next_empty:
    add     r21, r21, #DOS_ASSIGN_ENTRY_SZ
    add     r20, r20, #1
    bra     .dafe_loop
.dafe_found:
    move.q  r1, r26
    move.q  r2, r20
    move.l  r3, #1
    rts
.dafe_notfound:
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r0
    rts

    ; .dos_assign_find_free_entry
    ; Out: r1 = entry ptr, r2 = index, r3 = 1 if found, 0 otherwise
.dos_assign_find_free_entry:
    add     r21, r29, #(prog_doslib_assign_table - prog_doslib_data)
    move.l  r20, #DOS_ASSIGN_DEFAULT_COUNT
    move.l  r24, #DOS_ASSIGN_DEFAULT_COUNT
    lsl     r24, r24, #5
    add     r21, r21, r24
.daffe_loop:
    move.l  r24, #DOS_ASSIGN_TABLE_COUNT
    bge     r20, r24, .daffe_notfound
    load.b  r24, (r21)
    beqz    r24, .daffe_found
    add     r21, r21, #DOS_ASSIGN_ENTRY_SZ
    add     r20, r20, #1
    bra     .daffe_loop
.daffe_found:
    move.q  r1, r21
    move.q  r2, r20
    move.l  r3, #1
    rts
.daffe_notfound:
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r0
    rts

    ; .dos_assign_builtin_root_row
    ; In:  r1 = input name ptr, r2 = input name len, r29 = data base
    ; Out: r1 = synthetic row ptr, r3 = 1 if SYS/IOSSYS, 0 otherwise
.dos_assign_builtin_root_row:
    move.q  r26, r1
    move.q  r27, r2
    move.l  r24, #3
    bne     r27, r24, .dabr_try_iossys
    move.q  r1, r26
    move.q  r2, r27
    add     r3, r29, #(prog_doslib_assign_builtin_sys_row - prog_doslib_data)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dabr_sys
.dabr_try_iossys:
    move.l  r24, #6
    bne     r27, r24, .dabr_notfound
    move.q  r1, r26
    move.q  r2, r27
    add     r3, r29, #(prog_doslib_assign_builtin_iossys_row - prog_doslib_data)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dabr_iossys
.dabr_notfound:
    move.q  r1, r0
    move.q  r3, r0
    rts
.dabr_sys:
    add     r1, r29, #(prog_doslib_assign_builtin_sys_row - prog_doslib_data)
    move.l  r3, #1
    rts
.dabr_iossys:
    add     r1, r29, #(prog_doslib_assign_builtin_iossys_row - prog_doslib_data)
    move.l  r3, #1
    rts

    ; .dos_assign_builtin_root_query_row
    ; In:  r1 = input name ptr, r2 = input name len, r29 = data base
    ; Out: r1 = synthetic public-query row ptr, r3 = 1 if SYS/IOSSYS, 0 otherwise
.dos_assign_builtin_root_query_row:
    move.q  r26, r1
    move.q  r27, r2
    move.l  r24, #3
    bne     r27, r24, .dabrq_try_iossys
    move.q  r1, r26
    move.q  r2, r27
    add     r3, r29, #(prog_doslib_assign_builtin_sys_row - prog_doslib_data)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dabrq_sys
.dabrq_try_iossys:
    move.l  r24, #6
    bne     r27, r24, .dabrq_notfound
    move.q  r1, r26
    move.q  r2, r27
    add     r3, r29, #(prog_doslib_assign_builtin_iossys_query_row - prog_doslib_data)
    jsr     .dos_assign_name_eq_ci
    beqz    r23, .dabrq_iossys
.dabrq_notfound:
    move.q  r1, r0
    move.q  r3, r0
    rts
.dabrq_sys:
    add     r1, r29, #(prog_doslib_assign_builtin_sys_row - prog_doslib_data)
    move.l  r3, #1
    rts
.dabrq_iossys:
    add     r1, r29, #(prog_doslib_assign_builtin_iossys_query_row - prog_doslib_data)
    move.l  r3, #1
    rts

    ; .dos_assign_lookup
    ; In:  r1 = input volume ptr, r2 = input volume len, r29 = data base
    ; Out: r1 = target ptr, r2 = target len, r3 = 1 if found, 0 otherwise
    ;
    ; M15.3: project the layered model into the single-target shape used by
    ; the rest of dos.library. If the resolved slot is canonical layered
    ; AND has a non-empty mutable overlay, return the first overlay target
    ; instead of the table entry's target. Otherwise behaviour matches the
    ; M15.2 contract (table entry target, or built-in root synthetic row).
    ;
    ; r20 is preserved for callers (e.g. .dos_assign_validate_target uses
    ; r20 across the lookup call as the "full path length" for its
    ; nested_ok branch — clobbering it here causes garbage canonical
    ; lengths). Uses dos.library scratch at offsets 800 (entry ptr) and
    ; 808 (caller's r20).
    ; Scratch inputs (both cleared before each return so stale values never
    ; leak to the next caller):
    ;   856(r29) non-zero = force base/table target (used by
    ;       .dos_assign_validate_target, which must canonicalize against
    ;       the static base list regardless of overlay state).
    ;   880(r29) = attempt index for layered multi-entry iteration:
    ;       0..overlay_count-1 → overlay[attempt] target
    ;       overlay_count       → base/table target (layered_relpath then
    ;                             handles the SYS:/IOSSYS: base fallback
    ;                             internally at the host layer)
    ;       > overlay_count     → not-found (iteration exhausted)
    ;   For non-layered slots and built-in root rows, attempt must be 0;
    ;   any higher attempt returns not-found so DOS_OPEN/RUN/LOADSEG loops
    ;   stop after a single pass.
.dos_assign_lookup:
    store.q r20, 808(r29)              ; preserve caller's r20 (M15.3)
    store.q r27, 776(r29)              ; preserve caller's r27 (M15.3 — see below)
    ; Save input name ptr+len so builtin_root_row can see them after a
    ; find_entry miss (find_entry zeroes r1/r2 on miss). Pre-existing bug
    ; that broke every SYS:/IOSSYS: lookup through .dos_resolve_has_colon.
    store.q r1, 448(r29)
    store.q r2, 456(r29)
    jsr     .dos_assign_find_entry
    bnez    r3, .dal_found_table
    load.q  r1, 448(r29)
    load.q  r2, 456(r29)
    jsr     .dos_assign_builtin_root_row
    beqz    r3, .dal_notfound
    ; Built-in root row: single-target, iteration stops past attempt 0.
    load.q  r26, 880(r29)
    bnez    r26, .dal_notfound
    bra     .dal_found
.dal_found_table:
    ; r1 = table entry ptr, r2 = slot index.
    store.q r1, 864(r29)               ; preserve entry ptr
    store.q r2, 872(r29)               ; preserve slot index
    load.q  r25, 856(r29)
    bnez    r25, .dal_reload_entry     ; skip_overlay → base/table target
    move.q  r1, r2
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dal_not_layered
    ; Canonical layered: interpret attempt against overlay_count.
    load.q  r1, 872(r29)
    jsr     .dos_assign_overlay_for_slot
    move.q  r20, r1                    ; overlay block ptr
    load.b  r21, (r20)                 ; overlay_count
    load.q  r26, 880(r29)              ; attempt
    bge     r26, r21, .dal_layered_past_overlay
    ; overlay hit: r1 = block + 8 + attempt * OVERLAY_TGT_SZ
    move.l  r27, #DOS_ASSIGN_OVERLAY_TGT_SZ
    mulu.q  r28, r26, r27
    add     r28, r28, #8
    add     r1, r20, r28
    jsr     .dos_assign_target_len
    bra     .dal_return_ok
.dal_layered_past_overlay:
    beq     r26, r21, .dal_reload_entry ; attempt == count: base/table
    bra     .dal_notfound               ; attempt > count: exhausted
.dal_not_layered:
    ; Non-layered user slot: attempt > 0 → not-found.
    load.q  r26, 880(r29)
    bnez    r26, .dal_notfound
.dal_reload_entry:
    load.q  r1, 864(r29)
.dal_found:
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_target_len
.dal_return_ok:
    store.q r0, 856(r29)               ; clear skip-overlay scratch
    store.q r0, 880(r29)               ; clear attempt-index scratch
    load.q  r20, 808(r29)
    load.q  r27, 776(r29)
    move.l  r3, #1
    rts
.dal_notfound:
    store.q r0, 856(r29)               ; clear skip-overlay scratch
    store.q r0, 880(r29)               ; clear attempt-index scratch
    load.q  r20, 808(r29)
    load.q  r27, 776(r29)
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r0
    rts

    ; .dos_assign_read_name
    ; In:  r1 = source ptr, r3 = dest ptr
    ; Out: r1 = dest ptr, r2 = name len, r3 = 1 if valid, 0 otherwise
.dos_assign_read_name:
    move.q  r22, r3
    move.l  r20, #0
.darn_loop:
    move.l  r24, #DOS_ASSIGN_NAME_MAX
    sub     r24, r24, #1
    bge     r20, r24, .darn_limit
    add     r21, r1, r20
    load.b  r24, (r21)
    beqz    r24, .darn_done
    move.l  r25, #0x3A
    beq     r24, r25, .darn_bad
    move.l  r25, #0x61
    blt     r24, r25, .darn_upper_chk
    move.l  r25, #0x7B
    bge     r24, r25, .darn_upper_chk
    sub     r24, r24, #0x20
.darn_upper_chk:
    move.l  r25, #0x41
    blt     r24, r25, .darn_bad
    move.l  r25, #0x5B
    bge     r24, r25, .darn_bad
    add     r21, r22, r20
    store.b r24, (r21)
    add     r20, r20, #1
    bra     .darn_loop
.darn_limit:
    add     r21, r1, r20
    load.b  r24, (r21)
    beqz    r24, .darn_done
.darn_done:
    beqz    r20, .darn_bad
    add     r21, r22, r20
    store.b r0, (r21)
    move.q  r1, r22
    move.q  r2, r20
    move.l  r3, #1
    rts
.darn_bad:
.davt_bad:
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r0
    rts

    ; .dos_assign_validate_target
    ; In:  r1 = source ptr
    ; Out: r1 = canonical target ptr, r2 = target len, r3 = 1 if valid, 0 otherwise
.dos_assign_validate_target:
    move.q  r27, r1
    move.l  r20, #0
.davt_len:
    move.l  r24, #DOS_ASSIGN_TARGET_MAX
    sub     r24, r24, #1
    bge     r20, r24, .davt_limit
    add     r21, r27, r20
    load.b  r24, (r21)
    beqz    r24, .davt_len_done
    add     r20, r20, #1
    bra     .davt_len
.davt_limit:
    add     r21, r27, r20
    load.b  r24, (r21)
    bnez    r24, .davt_bad
.davt_len_done:
    move.l  r24, #2
    blt     r20, r24, .davt_bad
    add     r21, r27, r20
    sub     r21, r21, #1
    load.b  r24, (r21)
    move.l  r25, #0x2F
    bne     r24, r25, .davt_bad

    ; Canonicalize the path into scratch, uppercasing letters and validating
    ; that every component only contains [A-Z]. The first component must
    ; resolve through the existing assign table / built-in rows. If there are
    ; nested components, append them to that canonical base target.
    add     r22, r29, #208
    move.l  r26, #0
    move.q  r28, r0                     ; first component len
    move.q  r30, r0                     ; saw first slash
.davt_copy_path:
    bge     r26, r20, .davt_path_done
    add     r21, r27, r26
    load.b  r24, (r21)
    move.l  r25, #0x2F
    beq     r24, r25, .davt_store_slash
    move.l  r25, #0x61
    blt     r24, r25, .davt_path_upper_chk
    move.l  r25, #0x7B
    bge     r24, r25, .davt_path_upper_chk
    sub     r24, r24, #0x20
.davt_path_upper_chk:
    move.l  r25, #0x41
    blt     r24, r25, .davt_bad
    move.l  r25, #0x5B
    bge     r24, r25, .davt_bad
    bnez    r30, .davt_store_char
    add     r28, r28, #1
.davt_store_char:
    add     r21, r22, r26
    store.b r24, (r21)
    add     r26, r26, #1
    bra     .davt_copy_path
.davt_store_slash:
    beqz    r26, .davt_bad
    beqz    r28, .davt_bad
    move.l  r30, #1
    add     r21, r22, r26
    store.b r24, (r21)
    add     r26, r26, #1
    bra     .davt_copy_path
.davt_path_done:
    add     r21, r22, r26
    store.b r0, (r21)
    move.l  r24, #3
    bne     r28, r24, .davt_check_ram
    load.b  r24, (r22)
    move.l  r25, #0x53                 ; 'S'
    bne     r24, r25, .davt_check_ram
    load.b  r24, 1(r22)
    move.l  r25, #0x59                 ; 'Y'
    bne     r24, r25, .davt_check_ram
    load.b  r24, 2(r22)
    move.l  r25, #0x53                 ; 'S'
    bne     r24, r25, .davt_check_ram
    load.b  r24, 3(r22)
    move.l  r25, #0x2F                 ; '/'
    bne     r24, r25, .davt_check_ram
    move.q  r1, r22
    move.q  r2, r20
    move.l  r3, #1
    rts
.davt_check_ram:
    move.l  r24, #3
    bne     r28, r24, .davt_lookup_prefix
    load.b  r24, (r22)
    move.l  r25, #0x52                 ; 'R'
    bne     r24, r25, .davt_lookup_prefix
    load.b  r24, 1(r22)
    move.l  r25, #0x41                 ; 'A'
    bne     r24, r25, .davt_lookup_prefix
    load.b  r24, 2(r22)
    move.l  r25, #0x4D                 ; 'M'
    beq     r24, r25, .davt_bad
.davt_lookup_prefix:
    ; M15.3: validate_target must resolve against the static base table,
    ; not the overlay, so `ASSIGN ADD C: SYS:C/` stores the canonical
    ; "SYS:C/" target even if C has an overlay redirecting elsewhere.
    move.l  r24, #1
    store.q r24, 856(r29)
    move.q  r1, r22
    move.q  r2, r28
    jsr     .dos_assign_lookup
    beqz    r3, .davt_bad
    move.q  r24, r1                     ; canonical base target ptr
    move.q  r25, r2                     ; canonical base target len
    blt     r28, r20, .davt_nested_ok   ; nested path already canonicalized
.davt_lookup_done:
    move.q  r1, r24
    move.q  r2, r25
    move.l  r3, #1
    rts
.davt_nested_ok:
    move.q  r1, r22
    move.q  r2, r20
    move.l  r3, #1
    rts

.darn_bad:
    store.b r0, (r22)
    move.q  r1, r22
    move.q  r2, r0
    move.q  r3, r0
    rts

    ; M15.3 .dos_resolved_is_iossys
    ; In:  r1 = resolved DOS name ptr
    ; Out: r3 = 1 if the resolved name points under the read-only IOSSYS
    ;      namespace (either bare "IOSSYS/" or "SYS/IOSSYS/"), else 0.
    ; Clobbers: r14, r15, r17.
    ;
    ; Used by the DOS_OPEN write loop to pre-filter read-only candidates
    ; so the write-forward scan skips IOSSYS-pointing effective targets
    ; instead of falling into BOOT_HOSTFS_CREATE_WRITE (which is gated)
    ; or synthesizing a misleading IOSSYS-named RAM entry.
.dos_resolved_is_iossys:
    move.q  r14, r1
    ; Strip optional "SYS/" prefix so "SYS/IOSSYS/..." reads as IOSSYS.
    load.b  r15, (r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x53                 ; 'S'
    bne     r15, r17, .dri_check
    load.b  r15, 1(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x59                 ; 'Y'
    bne     r15, r17, .dri_check
    load.b  r15, 2(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x53
    bne     r15, r17, .dri_check
    load.b  r15, 3(r14)
    move.l  r17, #0x2F                 ; '/'
    bne     r15, r17, .dri_check
    add     r14, r14, #4
.dri_check:
    load.b  r15, (r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x49                 ; 'I'
    bne     r15, r17, .dri_no
    load.b  r15, 1(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x4F                 ; 'O'
    bne     r15, r17, .dri_no
    load.b  r15, 2(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x53
    bne     r15, r17, .dri_no
    load.b  r15, 3(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x53
    bne     r15, r17, .dri_no
    load.b  r15, 4(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x59                 ; 'Y'
    bne     r15, r17, .dri_no
    load.b  r15, 5(r14)
    and     r15, r15, #0xDF
    move.l  r17, #0x53
    bne     r15, r17, .dri_no
    load.b  r15, 6(r14)
    move.l  r17, #0x2F                 ; '/'
    bne     r15, r17, .dri_no
    move.l  r3, #1
    rts
.dri_no:
    move.q  r3, r0
    rts

    ; .dos_resolve_apply_target
    ; In:  r1 = target ptr, r2 = target len, r14 = source ptr, r29 = data base
    ; Out: r23 = resolved scratch ptr, r22 = 1
.dos_resolve_apply_target:
    add     r17, r29, #1000
    move.q  r20, r1
    move.q  r21, r2
.drat_copy_target:
    beqz    r21, .drat_copy_rest_setup
    load.b  r16, (r20)
    store.b r16, (r17)
    add     r20, r20, #1
    add     r17, r17, #1
    sub     r21, r21, #1
    bra     .drat_copy_target
.drat_copy_rest_setup:
    move.l  r19, #31
    sub     r19, r19, r2
.drat_copy_rest:
    load.b  r16, (r14)
    beqz    r16, .drat_term
    beqz    r19, .drat_term
    store.b r16, (r17)
    add     r14, r14, #1
    add     r17, r17, #1
    sub     r19, r19, #1
    bra     .drat_copy_rest
.drat_term:
    store.b r0, (r17)
    add     r23, r29, #1000
    move.l  r22, #1
    rts

    ; .dos_resolve_cmd: resolve command name (default prefix C:)
    ; Input: r23 = name pointer in shared buffer, r29 = data base
    ; Output: r23 = resolved name pointer (may be unchanged), r22 = 1 if valid
    ; Clobbers: r14-r25
.dos_resolve_cmd:
    move.l  r18, #1                     ; default = C: volume
    bra     .dos_resolve_scan
    ; .dos_resolve_file: resolve filename (bare default)
    ; Input: r23 = name pointer in shared buffer, r29 = data base
    ; Output: r23 = resolved name pointer (may be unchanged), r22 = 1 if valid
.dos_resolve_file:
    move.l  r18, #0                     ; default = bare (no prefix)
.dos_resolve_scan:
    move.l  r22, #1
    ; Scan for ':' in name
    move.q  r14, r23
    move.l  r15, #0                     ; char index
.dos_resolve_colon:
    load.b  r16, (r14)
    beqz    r16, .dos_resolve_no_colon  ; end of string, no colon found
    move.l  r17, #0x3A                  ; ':'
    beq     r16, r17, .dos_resolve_has_colon
    add     r14, r14, #1
    add     r15, r15, #1
    move.l  r17, #32
    blt     r15, r17, .dos_resolve_colon
.dos_resolve_no_colon:
    ; Already-qualified slash path (e.g. "C/Version", "LIBS/foo") —
    ; leave it alone. DOS seed names are stored with slash assigns, and
    ; callers such as the M14 LoadSeg tests may legitimately supply that
    ; exact on-disk form. Only bare names should get the default C/
    ; prefix.
    move.q  r14, r23
    move.l  r15, #0
.dos_resolve_slash_scan:
    load.b  r16, (r14)
    beqz    r16, .dos_resolve_no_slash
    move.l  r17, #0x2F                 ; '/'
    beq     r16, r17, .dos_resolve_bare_ret
    add     r14, r14, #1
    add     r15, r15, #1
    move.l  r17, #32
    blt     r15, r17, .dos_resolve_slash_scan
.dos_resolve_no_slash:
    ; No colon found — check default mode
    beqz    r18, .dos_resolve_bare_ret  ; mode=0 → bare name, return unchanged
    move.q  r27, r23
    add     r1, r29, #(prog_doslib_assign_default_c - prog_doslib_data)
    move.l  r2, #1
    jsr     .dos_assign_lookup
    beqz    r3, .dos_resolve_notfound
    move.q  r14, r27
    jsr     .dos_resolve_apply_target
.dos_resolve_bare_ret:
    move.l  r22, #1
    rts
.dos_resolve_has_colon:
    ; M15.3: parked in scratch rather than r27 — builtin_root_row (via
    ; name_supported / lookup) clobbered r27 for SYS:/IOSSYS: roots,
    ; which silently broke apply_target's src-past-colon computation.
    store.q r23, 760(r29)
    store.q r15, 752(r29)
    move.q  r1, r23
    move.q  r2, r15
    jsr     .dos_assign_name_supported
    beqz    r3, .dos_resolve_notfound
    load.q  r1, 760(r29)
    load.q  r2, 752(r29)
    jsr     .dos_assign_lookup
    beqz    r3, .dos_resolve_notfound
    load.q  r27, 760(r29)
    load.q  r15, 752(r29)
    add     r14, r27, r15
    add     r14, r14, #1                ; src = past ':'
    beqz    r2, .dos_resolve_ram
    jsr     .dos_resolve_apply_target
    rts
.dos_resolve_ram:
    move.q  r23, r14
    move.l  r22, #1
    rts
.dos_resolve_notfound:
    move.q  r22, r0
    move.q  r23, r0
    rts

    ; .dos_assign_name_supported
    ; In:  r1 = input volume ptr, r2 = input volume len
    ; Out: r3 = 1 if the name exists in the active assign table, 0 otherwise
    ;
    ; M15.3: preserve r27 for callers (.dos_resolve_has_colon parks the
    ; original resolved-name ptr in r27 around this call). The internal
    ; .dos_assign_builtin_root_row helper uses r27 as scratch for SYS:/
    ; IOSSYS: roots, so we save/restore via dos.library scratch at 768.
.dos_assign_name_supported:
    store.q r27, 768(r29)
    ; Save input for builtin_root_row fallback after find_entry miss
    ; (find_entry zeroes r1/r2 on miss).
    store.q r1, 464(r29)
    store.q r2, 472(r29)
    jsr     .dos_assign_find_entry
    bnez    r3, .dans_done
    load.q  r1, 464(r29)
    load.q  r2, 472(r29)
    jsr     .dos_assign_builtin_root_row
.dans_done:
    load.q  r27, 768(r29)
    rts

    ; =================================================================
    ; M15.3 layered-assign helpers
    ; =================================================================

    ; .dos_assign_slot_layered_p
    ; In:  r1 = slot index (0..7), r29 = data base
    ; Out: r3 = 1 if canonical layered slot, 0 otherwise
    ; Uses prog_doslib_assign_base_table (zero entry → not layered).
.dos_assign_slot_layered_p:
    move.q  r20, r1
    lsl     r20, r20, #4               ; slot * 16
    add     r20, r20, #(prog_doslib_assign_base_table - prog_doslib_data)
    add     r20, r29, r20
    load.q  r20, (r20)
    beqz    r20, .daslp_no
    move.l  r3, #1
    rts
.daslp_no:
    move.q  r3, r0
    rts

    ; .dos_assign_overlay_for_slot
    ; In:  r1 = slot index, r29 = data base
    ; Out: r1 = pointer to overlay block (count + 4 × 16 targets)
.dos_assign_overlay_for_slot:
    move.q  r20, r1
    move.l  r21, #DOS_ASSIGN_OVERLAY_ENTRY_SZ
    mulu.q  r22, r20, r21
    add     r22, r22, #(prog_doslib_assign_overlay - prog_doslib_data)
    add     r1, r29, r22
    rts

    ; M15.3 .dos_assign_layered_target_already_emitted
    ; In:  r1 = candidate src ptr (NUL-terminated),
    ;      r2 = share buffer base ptr,
    ;      r3 = current cursor ptr (= base + N*DOS_ASSIGN_LAYERED_TGT_SZ)
    ; Out: r3 = 1 if the candidate already appears in [base..cursor),
    ;      0 otherwise
    ; Clobbers: r20-r28
    ; Used by .dos_assign_layered_query to honour the plan's "duplicate
    ; targets are collapsed so the same target is not visited twice" rule
    ; — e.g. when the user runs `ASSIGN ADD C: C:`, the overlay copy of
    ; "C/" must not appear alongside the base list's own "C/".
.dos_assign_layered_target_already_emitted:
    sub     r4, r3, r2                 ; total bytes already written
    move.q  r20, r0                    ; slot offset = 0
.daltae_slot_loop:
    bge     r20, r4, .daltae_not_dup
    add     r21, r2, r20               ; existing slot ptr
    move.q  r22, r0
.daltae_byte_loop:
    move.l  r23, #DOS_ASSIGN_LAYERED_TGT_SZ
    bge     r22, r23, .daltae_match
    add     r24, r1, r22
    load.b  r25, (r24)
    add     r24, r21, r22
    load.b  r26, (r24)
    bne     r25, r26, .daltae_next_slot
    beqz    r25, .daltae_match
    add     r22, r22, #1
    bra     .daltae_byte_loop
.daltae_match:
    move.l  r3, #1
    rts
.daltae_next_slot:
    add     r20, r20, #DOS_ASSIGN_LAYERED_TGT_SZ
    bra     .daltae_slot_loop
.daltae_not_dup:
    move.q  r3, r0
    rts

    ; .dos_assign_layered_emit_target_slot
    ; Copy a NUL-terminated string into a 32-byte dest slot, padded with
    ; zeroes. Used when emitting effective-list entries for LAYERED_QUERY.
    ; In:  r1 = src ptr, r2 = dst ptr (32-byte slot)
.dos_assign_layered_emit_target_slot:
    move.q  r20, r1
    move.q  r21, r2
    move.q  r22, r0
.daleds_zero:
    move.l  r23, #DOS_ASSIGN_LAYERED_TGT_SZ
    bge     r22, r23, .daleds_copy_init
    add     r24, r21, r22
    store.b r0, (r24)
    add     r22, r22, #1
    bra     .daleds_zero
.daleds_copy_init:
    move.q  r22, r0
.daleds_copy:
    move.l  r23, #DOS_ASSIGN_LAYERED_TGT_SZ
    sub     r23, r23, #1
    bge     r22, r23, .daleds_done
    add     r24, r20, r22
    load.b  r25, (r24)
    beqz    r25, .daleds_done
    add     r24, r21, r22
    store.b r25, (r24)
    add     r22, r22, #1
    bra     .daleds_copy
.daleds_done:
    rts

    ; .dos_assign_overlay_first_target_for_slot
    ; In:  r1 = slot index, r29 = data base
    ; Out: r1 = ptr to first overlay target (16 bytes), r3 = 1 if exists, 0 otherwise
.dos_assign_overlay_first_target_for_slot:
    jsr     .dos_assign_overlay_for_slot
    move.q  r20, r1
    load.b  r21, (r20)
    beqz    r21, .daofts_none
    add     r1, r20, #8
    move.l  r3, #1
    rts
.daofts_none:
    move.q  r1, r0
    move.q  r3, r0
    rts

    ; M15.3 .dos_assign_sync_table_entry_to_overlay
    ; In: r1 = slot index (must be canonical layered), r29 = data base
    ; Overwrites table[slot].target with:
    ;   - overlay[0] target if overlay_count > 0
    ;   - canonical default "<name>/" if overlay is empty
    ; This keeps the M15.2 first-effective projection (`dos_assign_lookup`)
    ; consistent after ADD/REMOVE so that DOS_OPEN path resolution sees
    ; the intended target without a separate layered-iteration pass.
.dos_assign_sync_table_entry_to_overlay:
    move.q  r24, r1
    jsr     .dos_assign_slot_layered_p
    beqz    r3, .dast_done
    ; Compute table[slot].target ptr → r25. Save table[slot] base at
    ; 784(r29) so the .dast_use_default branch can reload it after
    ; .dos_assign_overlay_first_target_for_slot (which clobbers r22).
    move.q  r20, r24
    move.l  r21, #DOS_ASSIGN_ENTRY_SZ
    mulu.q  r22, r20, r21
    add     r22, r22, #(prog_doslib_assign_table - prog_doslib_data)
    add     r22, r29, r22                   ; r22 = table[slot] base
    store.q r22, 784(r29)
    add     r25, r22, #DOS_ASSIGN_TARGET_OFF
    store.q r25, 792(r29)
    ; Zero the target field so short writes don't leak stale trailing bytes.
    move.q  r20, r0
.dast_zero:
    move.l  r21, #DOS_ASSIGN_TARGET_MAX
    bge     r20, r21, .dast_source
    add     r26, r25, r20
    store.b r0, (r26)
    add     r20, r20, #1
    bra     .dast_zero
.dast_source:
    move.q  r1, r24
    jsr     .dos_assign_overlay_first_target_for_slot
    load.q  r22, 784(r29)
    load.q  r25, 792(r29)
    beqz    r3, .dast_use_default
    ; r1 = overlay[0] ptr (16 bytes, NUL-padded). Copy up to first NUL.
    move.q  r26, r1
    move.q  r20, r0
.dast_copy_ovl:
    move.l  r21, #DOS_ASSIGN_TARGET_MAX
    bge     r20, r21, .dast_done
    add     r27, r26, r20
    load.b  r28, (r27)
    beqz    r28, .dast_done
    add     r27, r25, r20
    store.b r28, (r27)
    add     r20, r20, #1
    bra     .dast_copy_ovl
.dast_use_default:
    ; Default target = "{name}/" where name is table[slot].name.
    move.q  r20, r0
.dast_copy_name:
    move.l  r21, #DOS_ASSIGN_NAME_MAX
    sub     r21, r21, #1
    bge     r20, r21, .dast_append_slash
    add     r27, r22, r20                    ; table[slot].name[r20]
    load.b  r28, (r27)
    beqz    r28, .dast_append_slash
    add     r27, r25, r20
    store.b r28, (r27)
    add     r20, r20, #1
    bra     .dast_copy_name
.dast_append_slash:
    move.l  r21, #DOS_ASSIGN_TARGET_MAX
    sub     r21, r21, #1
    bge     r20, r21, .dast_done
    add     r27, r25, r20
    move.l  r28, #0x2F
    store.b r28, (r27)
.dast_done:
    rts

    ; .dos_prefix_eq_ci
    ; In:  r1 = candidate ptr, r2 = prefix ptr
    ; Out: r23 = 0 if candidate starts with prefix (case-insensitive), 1 otherwise
.dos_prefix_eq_ci:
    move.l  r20, #0
.dpec_loop:
    add     r21, r2, r20
    load.b  r22, (r21)
    beqz    r22, .dpec_match
    add     r21, r1, r20
    load.b  r24, (r21)
    beqz    r24, .dpec_mismatch
    move.l  r25, #0x61
    blt     r22, r25, .dpec_pref_done
    move.l  r25, #0x7B
    bge     r22, r25, .dpec_pref_done
    sub     r22, r22, #0x20
.dpec_pref_done:
    move.l  r25, #0x61
    blt     r24, r25, .dpec_cand_done
    move.l  r25, #0x7B
    bge     r24, r25, .dpec_cand_done
    sub     r24, r24, #0x20
.dpec_cand_done:
    bne     r22, r24, .dpec_mismatch
    add     r20, r20, #1
    bra     .dpec_loop
.dpec_match:
    move.q  r23, r0
    rts
.dpec_mismatch:
    move.l  r23, #1
    rts

    ; .dos_pmp_parse_file_iosm
    ; In:  r1 = caller path ptr, r2 = destination IOSM descriptor ptr
    ; Out: r20 = ERR_OK, ERR_NOTFOUND, ERR_NOMEM, or ERR_BADARG.
    ; Resolves the real DOS path, reads the ELF, and copies the on-disk
    ; .ios.manifest/IOS-MOD descriptor. No shipped filename table is consulted.
.dos_pmp_parse_file_iosm:
    push    r29
    move.l  r10, #(prog_doslib_pmp_desc_dst - prog_doslib_data)
    add     r10, r29, r10
    store.q r2, (r10)                   ; descriptor destination
    move.q  r23, r1
    jsr     .dos_resolve_file
    load.q  r29, (sp)
    beqz    r22, .dpmf_notfound

    move.q  r1, r23
    move.q  r2, r0                      ; read mode
    jsr     .dos_hostfs_layered_relpath_for_resolved_name
    load.q  r29, (sp)
    beqz    r3, .dpmf_notfound
    store.q r1, 608(r29)                ; hostfs relative path

    jsr     .dos_bootfs_stat
    load.q  r29, (sp)
    bnez    r3, .dpmf_notfound
    beqz    r1, .dpmf_badarg
    store.q r1, 624(r29)                ; file size
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_FILE
    bne     r28, r15, .dpmf_notfound

    load.q  r1, 608(r29)
    jsr     .dos_bootfs_open
    load.q  r29, (sp)
    bnez    r2, .dpmf_notfound
    store.q r1, 616(r29)                ; host handle

    load.q  r1, 624(r29)
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    bnez    r2, .dpmf_nomem_close
    store.q r1, 296(r29)                ; temp ELF buffer

    load.q  r1, 616(r29)
    load.q  r2, 296(r29)
    load.q  r3, 624(r29)
    jsr     .dos_bootfs_read_all
    load.q  r29, (sp)
    store.q r1, 640(r29)                ; bytes read
    store.q r2, 648(r29)                ; read err

    load.q  r1, 616(r29)
    jsr     .dos_bootfs_close
    load.q  r29, (sp)

    load.q  r2, 648(r29)
    bnez    r2, .dpmf_free_notfound
    load.q  r1, 640(r29)
    load.q  r2, 624(r29)
    bne     r1, r2, .dpmf_free_badarg

    load.q  r1, 296(r29)                ; ELF ptr
    load.q  r2, 624(r29)                ; ELF size
    move.l  r10, #(prog_doslib_pmp_desc_dst - prog_doslib_data)
    add     r10, r29, r10
    load.q  r3, (r10)                   ; descriptor destination
    jsr     .dos_pmp_copy_iosm_from_elf
    load.q  r29, (sp)
    store.q r20, 656(r29)

    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    load.q  r20, 656(r29)
    pop     r29
    rts
.dpmf_nomem_close:
    load.q  r1, 616(r29)
    jsr     .dos_bootfs_close
    load.q  r29, (sp)
    move.l  r20, #ERR_NOMEM
    pop     r29
    rts
.dpmf_free_notfound:
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.dpmf_notfound:
    move.l  r20, #ERR_NOTFOUND
    pop     r29
    rts
.dpmf_free_badarg:
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.dpmf_badarg:
    move.l  r20, #ERR_BADARG
    pop     r29
    rts
.dpmf_known_done:
    pop     r29
    rts

    ; .dos_pmp_copy_iosm_from_elf
    ; In:  r1 = ELF ptr, r2 = ELF size, r3 = destination IOSM descriptor ptr
    ; Out: r20 = ERR_OK / ERR_NOTFOUND / ERR_BADARG.
.dos_pmp_copy_iosm_from_elf:
    move.q  r20, #ERR_BADARG
    move.q  r21, r1                      ; ELF base
    move.q  r22, r2                      ; ELF size
    store.q r3, 664(r29)                 ; descriptor destination
    move.l  r11, #64
    blt     r22, r11, .dpcife_done
    load.l  r11, (r21)
    move.l  r12, #0x464C457F
    bne     r11, r12, .dpcife_done
    load.b  r11, 4(r21)
    move.l  r12, #2                      ; ELFCLASS64
    bne     r11, r12, .dpcife_done
    load.b  r11, 5(r21)
    move.l  r12, #1                      ; little-endian
    bne     r11, r12, .dpcife_done
    load.b  r11, 6(r21)
    move.l  r12, #1
    bne     r11, r12, .dpcife_done

    load.q  r18, 40(r21)                 ; e_shoff
    beqz    r18, .dpcife_notfound
    store.q r18, 672(r29)
    load.w  r24, 58(r21)                 ; e_shentsize
    move.l  r11, #64
    bne     r24, r11, .dpcife_done
    store.q r24, 680(r29)
    load.w  r25, 60(r21)                 ; e_shnum
    beqz    r25, .dpcife_notfound
    store.q r25, 688(r29)
    load.w  r26, 62(r21)                 ; e_shstrndx
    bge     r26, r25, .dpcife_done
    move.q  r27, r25
    mulu    r27, r24, r27
    add     r27, r18, r27
    bgt     r27, r22, .dpcife_done

    move.q  r27, r26
    mulu    r27, r24, r27
    add     r27, r18, r27
    add     r27, r21, r27                ; shstrtab section header
    load.q  r28, 24(r27)                 ; shstr offset
    load.q  r30, 32(r27)                 ; shstr size
    beqz    r30, .dpcife_done
    store.q r28, 696(r29)
    store.q r30, 704(r29)
    add     r11, r28, r30
    bgt     r11, r22, .dpcife_done

    move.l  r19, #0
.dpcife_sh_loop:
    load.q  r25, 688(r29)                ; e_shnum
    load.q  r24, 680(r29)                ; e_shentsize
    load.q  r18, 672(r29)                ; e_shoff
    load.q  r28, 696(r29)                ; shstr offset
    load.q  r30, 704(r29)                ; shstr size
    bge     r19, r25, .dpcife_notfound
    move.q  r27, r19
    mulu    r27, r24, r27
    add     r27, r18, r27
    add     r27, r21, r27
    load.l  r11, 4(r27)                  ; sh_type
    move.l  r12, #7                      ; SHT_NOTE
    bne     r11, r12, .dpcife_next_sh
    load.l  r11, (r27)                   ; sh_name
    bge     r11, r30, .dpcife_next_sh
    move.q  r12, r11
    add     r12, r12, #14                 ; len(".ios.manifest\0")
    bgt     r12, r30, .dpcife_next_sh
    add     r14, r21, r28
    add     r14, r14, r11
    jsr     .dos_pmp_is_iosm_section_name
    bnez    r23, .dpcife_next_sh
    load.q  r11, 24(r27)                 ; note section offset
    load.q  r12, 32(r27)                 ; note section size
    beqz    r12, .dpcife_done
    add     r13, r11, r12
    bgt     r13, r22, .dpcife_done
    add     r27, r21, r11                ; note ptr
    add     r30, r27, r12                ; note section end
    store.q r30, 712(r29)
.dpcife_note_loop:
    load.q  r30, 712(r29)                ; note section end
    move.q  r13, r27
    add     r13, r13, #12
    bgt     r13, r30, .dpcife_done
    load.l  r11, (r27)                   ; namesz
    move.q  r25, r11
    load.l  r12, 4(r27)                  ; descsz
    move.q  r24, r12
    move.q  r13, r25
    add     r13, r13, #3
    and     r13, r13, #0xFFFFFFFC
    move.q  r26, r24
    add     r26, r26, #3
    and     r26, r26, #0xFFFFFFFC
    move.q  r17, r27
    add     r17, r17, #12
    add     r17, r17, r13
    add     r17, r17, r26                ; next note ptr
    store.q r17, 720(r29)
    bgt     r17, r30, .dpcife_done
    move.l  r12, #8
    bne     r25, r12, .dpcife_note_next
    load.l  r12, 8(r27)
    move.l  r11, #IOSM_NOTE_TYPE
    bne     r12, r11, .dpcife_note_next
    add     r14, r27, #12
    jsr     .dos_pmp_is_iosm_note_name
    bnez    r23, .dpcife_note_next
    move.l  r11, #IOSM_SIZE
    bne     r24, r11, .dpcife_note_next
    add     r27, r27, #12
    add     r27, r27, r13                ; descriptor ptr
    load.l  r11, IOSM_OFF_MAGIC(r27)
    move.l  r12, #IOSM_MAGIC
    bne     r11, r12, .dpcife_note_next
    load.l  r11, IOSM_OFF_SCHEMA_VERSION(r27)
    move.l  r12, #IOSM_SCHEMA_VERSION
    bne     r11, r12, .dpcife_note_next
    load.b  r11, IOSM_OFF_RESERVED0(r27)
    bnez    r11, .dpcife_note_next
    move.l  r11, #0
.dpcife_reserved2_loop:
    move.l  r12, #8
    bge     r11, r12, .dpcife_copy_desc
    add     r13, r27, #IOSM_OFF_RESERVED2
    add     r13, r13, r11
    load.b  r13, (r13)
    bnez    r13, .dpcife_note_next
    add     r11, r11, #1
    bra     .dpcife_reserved2_loop
.dpcife_copy_desc:
    jsr     .dos_validate_iosm_aslr_contract
    bnez    r20, .dpcife_note_next
    load.q  r14, 664(r29)                ; destination
    move.l  r15, #(IOSM_SIZE / 8)
.dpcife_copy_loop:
    load.q  r16, (r27)
    store.q r16, (r14)
    add     r27, r27, #8
    add     r14, r14, #8
    sub     r15, r15, #1
    bnez    r15, .dpcife_copy_loop
    move.q  r20, #ERR_OK
    rts
.dpcife_note_next:
    load.q  r27, 720(r29)
    load.q  r30, 712(r29)
    blt     r27, r30, .dpcife_note_loop
    bra     .dpcife_notfound
.dpcife_next_sh:
    add     r19, r19, #1
    bra     .dpcife_sh_loop
.dpcife_notfound:
    move.q  r20, #ERR_NOTFOUND
.dpcife_done:
    rts

    ; .dos_validate_loaded_elf_aslr_contract
    ; In:  r1 = ELF ptr, r2 = ELF size
    ; Out: r20 = ERR_OK / ERR_NOTFOUND / ERR_BADARG.
.dos_validate_loaded_elf_aslr_contract:
    move.l  r3, #(prog_doslib_pmp_desc_dst - prog_doslib_data)
    add     r3, r29, r3
    jsr     .dos_pmp_copy_iosm_from_elf
    rts

    ; .dos_validate_iosm_aslr_contract
    ; In:  r27 = IOSM descriptor ptr
    ; Out: r20 = ERR_OK or ERR_BADARG.
.dos_validate_iosm_aslr_contract:
    move.q  r20, #ERR_BADARG
    load.b  r11, IOSM_OFF_KIND(r27)
    move.l  r12, #IOSM_KIND_COMMAND
    beq     r11, r12, .dviac_command
    move.l  r12, #IOSM_KIND_LIBRARY
    beq     r11, r12, .dviac_module
    move.l  r12, #IOSM_KIND_DEVICE
    beq     r11, r12, .dviac_module
    move.l  r12, #IOSM_KIND_HANDLER
    beq     r11, r12, .dviac_module
    move.l  r12, #IOSM_KIND_RESOURCE
    beq     r11, r12, .dviac_module
    rts
.dviac_command:
    load.l  r11, IOSM_OFF_FLAGS(r27)
    move.l  r12, #MODF_ASLR_CAPABLE
    bne     r11, r12, .dviac_done
    move.q  r20, #ERR_OK
    rts
.dviac_module:
    load.l  r11, IOSM_OFF_FLAGS(r27)
    and     r12, r11, #MODF_ASLR_CAPABLE
    beqz    r12, .dviac_done
    move.l  r12, #(MODF_COMPAT_PORT | MODF_ASLR_CAPABLE)
    bne     r11, r12, .dviac_done
    move.q  r20, #ERR_OK
.dviac_done:
    rts

    ; R14 = NUL-terminated candidate. R23 = 0 on exact ".ios.manifest".
.dos_pmp_is_iosm_section_name:
    load.b  r11, 0(r14)
    move.l  r12, #0x2E
    bne     r11, r12, .dpiis_no
    load.b  r11, 1(r14)
    move.l  r12, #0x69
    bne     r11, r12, .dpiis_no
    load.b  r11, 2(r14)
    move.l  r12, #0x6F
    bne     r11, r12, .dpiis_no
    load.b  r11, 3(r14)
    move.l  r12, #0x73
    bne     r11, r12, .dpiis_no
    load.b  r11, 4(r14)
    move.l  r12, #0x2E
    bne     r11, r12, .dpiis_no
    load.b  r11, 5(r14)
    move.l  r12, #0x6D
    bne     r11, r12, .dpiis_no
    load.b  r11, 6(r14)
    move.l  r12, #0x61
    bne     r11, r12, .dpiis_no
    load.b  r11, 7(r14)
    move.l  r12, #0x6E
    bne     r11, r12, .dpiis_no
    load.b  r11, 8(r14)
    move.l  r12, #0x69
    bne     r11, r12, .dpiis_no
    load.b  r11, 9(r14)
    move.l  r12, #0x66
    bne     r11, r12, .dpiis_no
    load.b  r11, 10(r14)
    move.l  r12, #0x65
    bne     r11, r12, .dpiis_no
    load.b  r11, 11(r14)
    move.l  r12, #0x73
    bne     r11, r12, .dpiis_no
    load.b  r11, 12(r14)
    move.l  r12, #0x74
    bne     r11, r12, .dpiis_no
    load.b  r11, 13(r14)
    bnez    r11, .dpiis_no
    move.q  r23, r0
    rts
.dpiis_no:
    move.l  r23, #1
    rts

    ; R14 = note name. R23 = 0 on exact "IOS-MOD\0".
.dos_pmp_is_iosm_note_name:
    load.b  r11, 0(r14)
    move.l  r12, #0x49
    bne     r11, r12, .dpiin_no
    load.b  r11, 1(r14)
    move.l  r12, #0x4F
    bne     r11, r12, .dpiin_no
    load.b  r11, 2(r14)
    move.l  r12, #0x53
    bne     r11, r12, .dpiin_no
    load.b  r11, 3(r14)
    move.l  r12, #0x2D
    bne     r11, r12, .dpiin_no
    load.b  r11, 4(r14)
    move.l  r12, #0x4D
    bne     r11, r12, .dpiin_no
    load.b  r11, 5(r14)
    move.l  r12, #0x4F
    bne     r11, r12, .dpiin_no
    load.b  r11, 6(r14)
    move.l  r12, #0x44
    bne     r11, r12, .dpiin_no
    load.b  r11, 7(r14)
    bnez    r11, .dpiin_no
    move.q  r23, r0
    rts
.dpiin_no:
    move.l  r23, #1
    rts

    ; .dos_copy_zstr
    ; In: r1 = src ptr, r2 = dst ptr
    ; Out: r1 = dst end ptr (points at terminating NUL)
.dos_copy_zstr:
    move.q  r20, r1
    move.q  r21, r2
.dcz_loop:
    load.b  r22, (r20)
    store.b r22, (r21)
    beqz    r22, .dcz_done
    add     r20, r20, #1
    add     r21, r21, #1
    bra     .dcz_loop
.dcz_done:
    move.q  r1, r21
    rts

    ; .dos_hostfs_make_relpath
    ; In: r1 = rel-prefix ptr, r2 = suffix ptr, r3 = dest ptr
    ; Out: r1 = dest ptr, r3 = 1 on success, 0 on overflow
.dos_hostfs_make_relpath:
    move.q  r24, r3
    move.q  r25, r2
    move.q  r2, r3
    jsr     .dos_copy_zstr
    move.q  r26, r1
    move.l  r20, #0
.dhmr_suffix:
    move.l  r21, #47
    bge     r20, r21, .dhmr_overflow
    add     r22, r25, r20
    load.b  r23, (r22)
    beqz    r23, .dhmr_done
    add     r22, r26, r20
    store.b r23, (r22)
    add     r20, r20, #1
    bra     .dhmr_suffix
.dhmr_done:
    add     r22, r26, r20
    store.b r0, (r22)
    move.q  r1, r24
    move.l  r3, #1
    rts
.dhmr_overflow:
    add     r22, r26, #47
    store.b r0, (r22)
    move.q  r1, r24
    move.q  r3, r0
    rts

; .dos_hostfs_relpath_for_resolved_name
; In:  r1 = resolved DOS name ptr
; Out: r1 = hostfs-relative path ptr, r3 = 1 if hostfs-backed path, 0 otherwise
.dos_hostfs_relpath_for_resolved_name:
    move.q  r20, r1
    add     r27, r29, #544

    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_check_c
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x59
    bne     r21, r22, .dhrfrn_check_c
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_check_c
    load.b  r21, 3(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_sys

.dhrfrn_check_c:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x43
    bne     r21, r22, .dhrfrn_check_s
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_c

.dhrfrn_check_s:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_check_l
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_s

.dhrfrn_check_l:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4C
    bne     r21, r22, .dhrfrn_check_libs
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_l

.dhrfrn_check_libs:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4C
    bne     r21, r22, .dhrfrn_check_devs
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x49
    bne     r21, r22, .dhrfrn_check_devs
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x42
    bne     r21, r22, .dhrfrn_check_devs
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_check_devs
    load.b  r21, 4(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_libs

.dhrfrn_check_devs:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x44
    bne     r21, r22, .dhrfrn_check_resources
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45
    bne     r21, r22, .dhrfrn_check_resources
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x56
    bne     r21, r22, .dhrfrn_check_resources
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_check_resources
    load.b  r21, 4(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhrfrn_devs

.dhrfrn_check_resources:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x52
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4F
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 4(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x55
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 5(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x52
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 6(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x43
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 7(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 8(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhrfrn_none
    load.b  r21, 9(r20)
    move.l  r22, #0x2F
    bne     r21, r22, .dhrfrn_none

.dhrfrn_resources:
    add     r1, r29, #(prog_doslib_hostfs_rel_resources - prog_doslib_data)
    add     r2, r20, #10
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts
.dhrfrn_none:
    move.q  r1, r0
    move.q  r3, r0
    rts
.dhrfrn_sys:
    add     r1, r20, #4
    ; M15.3: when the resolved name is exactly "SYS/" (e.g. the bare
    ; volume DIR after `SYS:`), the past-prefix tail is empty. Empty
    ; relpaths are rejected by bootfs.resolveExistingPath (errCode 3),
    ; so emit "." here — that maps to hostRoot via resolveRelativePath
    ; and lets DIR enumerate the writable SYS overlay root.
    load.b  r28, (r1)
    bnez    r28, .dhrfrn_sys_copy
    move.l  r28, #0x2E                 ; '.'
    store.b r28, (r27)
    store.b r0, 1(r27)
    move.q  r1, r27
    move.l  r3, #1
    rts
.dhrfrn_sys_copy:
    move.q  r2, r27
    jsr     .dos_copy_zstr
    move.q  r1, r27
    move.l  r3, #1
    rts
.dhrfrn_c:
    add     r1, r29, #(prog_doslib_hostfs_rel_c - prog_doslib_data)
    add     r2, r20, #2
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts
.dhrfrn_s:
    add     r1, r29, #(prog_doslib_hostfs_rel_s - prog_doslib_data)
    add     r2, r20, #2
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts
.dhrfrn_l:
    add     r1, r29, #(prog_doslib_hostfs_rel_l - prog_doslib_data)
    add     r2, r20, #2
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts
.dhrfrn_libs:
    add     r1, r29, #(prog_doslib_hostfs_rel_libs - prog_doslib_data)
    add     r2, r20, #5
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts
.dhrfrn_devs:
    add     r1, r29, #(prog_doslib_hostfs_rel_devs - prog_doslib_data)
    add     r2, r20, #5
    move.q  r3, r27
    jsr     .dos_hostfs_make_relpath
    rts

    ; M15.3 .dos_hostfs_writable_relpath_for_resolved_name
    ; In:  r1 = resolved DOS name ptr
    ; Out: r1 = hostfs-relative writable path ptr, r3 = 1 if hostfs-backed, 0 otherwise
    ;
    ; Returns the writable SYS: overlay path for canonical layered assigns
    ; (e.g. "C/Version" → "C/Version", no IOSSYS/ prefix), and strips the
    ; "SYS/" prefix for explicit SYS: paths so that "SYS/Foo" → "Foo" at
    ; the hostfs root. Returns (0, 0) for paths that cannot be written via
    ; hostfs (RAM: / T: targets).
.dos_hostfs_writable_relpath_for_resolved_name:
    move.q  r20, r1
    add     r27, r29, #544             ; scratch ptr (shared with read-only mapper)

    ; --- SYS/ prefix → strip and copy the remainder to scratch ---
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhwr_check_c
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x59
    bne     r21, r22, .dhwr_check_c
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53
    bne     r21, r22, .dhwr_check_c
    load.b  r21, 3(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_sys

.dhwr_check_c:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x43                 ; 'C'
    bne     r21, r22, .dhwr_check_s
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_check_s:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53                 ; 'S'
    bne     r21, r22, .dhwr_check_l
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_check_l:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4C                 ; 'L'
    bne     r21, r22, .dhwr_check_libs
    load.b  r21, 1(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_check_libs:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4C                 ; 'L'
    bne     r21, r22, .dhwr_check_devs
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x49                 ; 'I'
    bne     r21, r22, .dhwr_check_devs
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x42                 ; 'B'
    bne     r21, r22, .dhwr_check_devs
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53                 ; 'S'
    bne     r21, r22, .dhwr_check_devs
    load.b  r21, 4(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_check_devs:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x44                 ; 'D'
    bne     r21, r22, .dhwr_check_resources
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45                 ; 'E'
    bne     r21, r22, .dhwr_check_resources
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x56                 ; 'V'
    bne     r21, r22, .dhwr_check_resources
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53                 ; 'S'
    bne     r21, r22, .dhwr_check_resources
    load.b  r21, 4(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_check_resources:
    load.b  r21, (r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x52                 ; 'R'
    bne     r21, r22, .dhwr_none
    load.b  r21, 1(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45                 ; 'E'
    bne     r21, r22, .dhwr_none
    load.b  r21, 2(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53                 ; 'S'
    bne     r21, r22, .dhwr_none
    load.b  r21, 3(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x4F                 ; 'O'
    bne     r21, r22, .dhwr_none
    load.b  r21, 4(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x55                 ; 'U'
    bne     r21, r22, .dhwr_none
    load.b  r21, 5(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x52                 ; 'R'
    bne     r21, r22, .dhwr_none
    load.b  r21, 6(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x43                 ; 'C'
    bne     r21, r22, .dhwr_none
    load.b  r21, 7(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x45                 ; 'E'
    bne     r21, r22, .dhwr_none
    load.b  r21, 8(r20)
    and     r21, r21, #0xDF
    move.l  r22, #0x53                 ; 'S'
    bne     r21, r22, .dhwr_none
    load.b  r21, 9(r20)
    move.l  r22, #0x2F
    beq     r21, r22, .dhwr_copy_full

.dhwr_none:
    move.q  r1, r0
    move.q  r3, r0
    rts
.dhwr_sys:
    ; Strip "SYS/" (4 bytes) and copy the remainder.
    add     r1, r20, #4
    move.q  r2, r27
    jsr     .dos_copy_zstr
    move.q  r1, r27
    move.l  r3, #1
    rts
.dhwr_copy_full:
    ; Copy the full resolved name verbatim as the hostfs-relative path.
    move.q  r1, r20
    move.q  r2, r27
    jsr     .dos_copy_zstr
    move.q  r1, r27
    move.l  r3, #1
    rts

    ; M15.3 .dos_hostfs_layered_relpath_for_resolved_name
    ; In:  r1 = resolved DOS name ptr, r2 = mode (0=read, 1=write)
    ; Out: r1 = chosen hostfs-relative path ptr, r3 = 1 if hostfs-backed, 0 otherwise
    ;
    ; For canonical layered assigns, the writable SYS: overlay path takes
    ; precedence when the file already exists there (READ mode) or when
    ; the caller is writing (WRITE mode). Falls back to the read-only
    ; IOSSYS path when no writable file is present (READ mode only).
    ;
    ; r29 must remain valid across the helper. Uses dos.library scratch
    ; at offsets 824 (resolved name), 832 (mode), 840 (writable path).
.dos_hostfs_layered_relpath_for_resolved_name:
    store.q r1, 824(r29)
    store.q r2, 832(r29)
    jsr     .dos_hostfs_writable_relpath_for_resolved_name
    beqz    r3, .dhlr_no_writable
    load.q  r4, 832(r29)
    bnez    r4, .dhlr_use_writable_now
    ; READ mode: stat writable. If exists as a file, use it.
    store.q r1, 840(r29)
    jsr     .dos_bootfs_stat
    bnez    r3, .dhlr_writable_miss
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_FILE
    bne     r28, r15, .dhlr_writable_miss
    load.q  r1, 840(r29)
    move.l  r3, #1
    rts
.dhlr_writable_miss:
.dhlr_no_writable:
    load.q  r1, 824(r29)
    jsr     .dos_hostfs_relpath_for_resolved_name
    rts
.dhlr_use_writable_now:
    move.l  r3, #1
    rts

    ; .dos_bootfs_stat
    ; In:  r1 = hostfs-relative path ptr
    ; Out: r1 = size, r2 = kind, r3 = err
.dos_bootfs_stat:
    move.q  r20, r1
    push    r29
    move.q  r2, r20
    move.l  r1, #BOOT_HOSTFS_STAT
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    move.q  r24, r2
    bnez    r24, .dbfs_err
    move.q  r25, r3
    move.q  r2, r25
    move.q  r3, r24
    rts
.dbfs_err:
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r24
    rts

    ; .dos_bootfs_open
    ; In:  r1 = hostfs-relative path ptr
    ; Out: r1 = host handle, r2 = err
.dos_bootfs_open:
    move.q  r20, r1
    push    r29
    move.q  r2, r20
    move.l  r1, #BOOT_HOSTFS_OPEN
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    rts

    ; M15.3 .dos_bootfs_create_write
    ; In:  r1 = hostfs-relative path ptr
    ; Out: r1 = host handle, r2 = err
    ; Rejects any path whose first component is IOSSYS (host-side check).
.dos_bootfs_create_write:
    move.q  r20, r1
    push    r29
    move.q  r2, r20
    move.l  r1, #BOOT_HOSTFS_CREATE_WRITE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    rts

    ; M15.3 .dos_bootfs_write
    ; In:  r1 = host handle, r2 = src ptr, r3 = byte_count
    ; Out: r1 = bytes_written, r2 = err
.dos_bootfs_write:
    move.q  r20, r1
    move.q  r21, r2
    move.q  r22, r3
    push    r29
    move.q  r2, r20
    move.q  r3, r21
    move.q  r4, r22
    move.l  r1, #BOOT_HOSTFS_WRITE
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    rts

    ; .dos_bootfs_read
    ; In:  r1 = host handle, r2 = dst ptr, r3 = byte_count
    ; Out: r1 = bytes read, r2 = err
.dos_bootfs_read:
    move.q  r20, r1
    move.q  r21, r2
    move.q  r22, r3
    push    r29
    move.q  r2, r20
    move.q  r3, r21
    move.q  r4, r22
    move.l  r1, #BOOT_HOSTFS_READ
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    rts

    ; .dos_bootfs_read_all
    ; In:  r1 = host handle, r2 = dst ptr, r3 = byte_count
    ; Out: r1 = total bytes read, r2 = err
.dos_bootfs_read_all:
    sub     sp, sp, #32
    store.q r1, 0(sp)                  ; handle
    store.q r2, 8(sp)                  ; dst
    store.q r3, 16(sp)                 ; remaining
    store.q r0, 24(sp)                 ; total
.dbfra_loop:
    load.q  r22, 16(sp)
    beqz    r22, .dbfra_done
    load.q  r1, 0(sp)
    load.q  r2, 8(sp)
    move.q  r3, r22
    jsr     .dos_bootfs_read
    bnez    r2, .dbfra_err
    beqz    r1, .dbfra_short
    load.q  r22, 16(sp)
    load.q  r23, 24(sp)
    add     r23, r23, r1
    load.q  r21, 8(sp)
    add     r21, r21, r1
    sub     r22, r22, r1
    store.q r23, 24(sp)
    store.q r21, 8(sp)
    store.q r22, 16(sp)
    bra     .dbfra_loop
.dbfra_short:
    load.q  r1, 24(sp)
    move.q  r2, r0
    add     sp, sp, #32
    rts
.dbfra_done:
    load.q  r1, 24(sp)
    move.q  r2, r0
    add     sp, sp, #32
    rts
.dbfra_err:
    move.q  r24, r2
    load.q  r1, 24(sp)
    move.q  r2, r24
    add     sp, sp, #32
    rts

    ; .dos_bootfs_close
    ; In:  r1 = host handle
    ; Out: r2 = err
.dos_bootfs_close:
    move.q  r20, r1
    push    r29
    move.q  r2, r20
    move.l  r1, #BOOT_HOSTFS_CLOSE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    rts

    ; .dos_bootfs_readdir
    ; In:  r1 = hostfs-relative dir path ptr, r2 = index
    ; Out: r1 = dirent scratch ptr, r2 = kind, r3 = err
.dos_bootfs_readdir:
    move.q  r20, r1
    move.q  r21, r2
    push    r29
    move.q  r2, r20
    move.q  r3, r21
    add     r4, r29, #704
    move.l  r1, #BOOT_HOSTFS_READDIR
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    move.q  r24, r2
    bnez    r24, .dbfrd_err
    add     r1, r29, #704
    move.q  r2, r3
    move.q  r3, r24
    rts
.dbfrd_err:
    move.q  r1, r0
    move.q  r2, r0
    move.q  r3, r24
    rts

    ; .dos_hostrec_alloc
    ; In:  r1 = file size, r2 = host handle
    ; Out: r1 = tagged record ptr, r2 = err
.dos_hostrec_alloc:
    move.q  r20, r1
    move.q  r21, r2
    push    r29
    push    r20
    push    r21
    move.l  r1, #32
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r21
    pop     r20
    pop     r29
    bnez    r2, .dhra_fail
    move.q  r24, r1
    move.l  r25, #0x54534F48          ; "HOST"
    store.l r25, (r24)
    store.q r20, 8(r24)
    store.q r21, 16(r24)
    add     r1, r24, #1
    move.q  r2, r0
    rts
.dhra_fail:
    move.q  r1, r0
    rts

    ; .dos_hostrec_free
    ; In: r1 = tagged record ptr
.dos_hostrec_free:
    beqz    r1, .dhrf_done
    sub     r1, r1, #1
    load.q  r20, 16(r1)
    beqz    r20, .dhrf_skip_buf
    push    r1
    push    r29
    load.q  r21, 8(r1)
    move.q  r2, r21
    move.q  r1, r20
    syscall #SYS_FREE_MEM
    pop     r29
    pop     r1
.dhrf_skip_buf:
    store.l r0, (r1)
    store.q r0, 8(r1)
    store.q r0, 16(r1)
    push    r29
    move.q  r2, #32
    syscall #SYS_FREE_MEM
    pop     r29
.dhrf_done:
    rts

    ; M15.3 .dos_hostrec_write_alloc
    ; In:  r1 = host write handle (from BOOT_HOSTFS_CREATE_WRITE)
    ; Out: r1 = tagged hostrec-write ptr (low bit set), r2 = err
    ; Layout (32 bytes):
    ;   [0..3]   magic = "HSTW"
    ;   [4..7]   pad (zero)
    ;   [8..15]  host handle id
    ;   [16..23] bytes_written running count (initially 0)
    ;   [24..31] reserved
.dos_hostrec_write_alloc:
    move.q  r20, r1
    push    r29
    push    r20
    move.l  r1, #32
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r20
    pop     r29
    bnez    r2, .dhrwa_fail
    move.q  r24, r1
    move.l  r25, #0x57545348           ; "HSTW" little-endian
    store.l r25, (r24)
    store.q r20, 8(r24)                ; host handle
    store.q r0, 16(r24)                ; bytes written = 0
    add     r1, r24, #1
    move.q  r2, r0
    rts
.dhrwa_fail:
    move.q  r1, r0
    rts

    ; M15.3 .dos_hostrec_write_free
    ; In: r1 = tagged hostrec-write ptr
    ; Closes the host handle, then frees the hostrec block.
.dos_hostrec_write_free:
    beqz    r1, .dhrwf_done
    sub     r1, r1, #1
    load.q  r20, 8(r1)                 ; host handle
    beqz    r20, .dhrwf_skip_close
    push    r1
    push    r29
    move.q  r2, r20
    move.l  r1, #BOOT_HOSTFS_CLOSE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_BOOT_HOSTFS
    pop     r29
    pop     r1
.dhrwf_skip_close:
    store.l r0, (r1)
    store.q r0, 8(r1)
    store.q r0, 16(r1)
    push    r29
    move.q  r2, #32
    syscall #SYS_FREE_MEM
    pop     r29
.dhrwf_done:
    rts

    ; M15.3 .dos_hostrec_is_write
    ; In:  r1 = tagged hostrec ptr (low bit MUST already be set)
    ; Out: r3 = 1 if hostrec-write (HSTW magic), 0 otherwise (HOST or other)
.dos_hostrec_is_write:
    move.q  r20, r1
    sub     r20, r20, #1
    load.l  r21, (r20)
    move.l  r22, #0x57545348
    bne     r21, r22, .dhriw_no
    move.l  r3, #1
    rts
.dhriw_no:
    move.q  r3, r0
    rts

    ; =================================================================
    ; DOS_RUN (type=6): launch program by name (M10)
    ; =================================================================
    ; Shared buffer format: "command_name\0args_string\0"
    ; Resolves command name through C: assign, finds image in file table,
    ; launches via SYS_EXEC_PROGRAM (new ABI: image_ptr, image_size).
.dos_do_run:
    ; M15.3 multi-entry fallthrough: 656(r29) = attempt index (reserved in
    ; the unused 648..744 dead-space slab). Each miss (meta + manifest
    ; both empty) increments and re-enters resolution so DOS_RUN walks
    ; the full effective list — overlay[0..N-1] then the canonical
    ; base — before replying NOTFOUND. 352(r29) is off-limits because
    ; .dos_do_loadseg uses it for the build_seglist err stash.
    store.q r0, 656(r29)
.dos_do_run_retry:
    ; 1. Read command name from mapped shared buffer
    store.q r0, 632(r29)              ; host-backed direct-exec flag
    store.q r0, 648(r29)              ; trusted-internal boot-elf flags
    load.q  r23, 168(r29)              ; r23 = caller's mapped buffer (name ptr)
    ; Propagate attempt index into lookup's scratch input.
    store.q r0, 856(r29)              ; skip_overlay = 0 (iterate instead)
    load.q  r28, 656(r29)
    store.q r28, 880(r29)

    ; 2. Resolve through C: assign (r23 in/out)
    jsr     .dos_resolve_cmd            ; r23 = resolved name (e.g. "C/Version")
    load.q  r29, (sp)
    ; r22=0 here means the effective list is exhausted (or the name was
    ; never resolvable) — terminate iteration without another retry.
    beqz    r22, .dos_run_reply_notfound_final
    store.q r23, 336(r29)

    ; Prefer a host-backed file when the resolved DOS name maps into the
    ; mounted system tree. M15.3: route through the layered helper so the
    ; writable SYS overlay takes priority when a command copy lives there
    ; (matches the DOS_OPEN behaviour). Mode=0 (READ) so the helper stats
    ; the writable path first and falls back to IOSSYS/ when absent.
    move.q  r1, r23
    move.l  r2, #0
    jsr     .dos_hostfs_layered_relpath_for_resolved_name
    beqz    r3, .dos_run_file_lookup
    move.q  r24, r1
    move.q  r27, r24
    store.q r24, 608(r29)
    move.q  r1, r24
    push    r29
    jsr     .dos_bootfs_stat
    pop     r29
    beqz    r3, .dos_run_hostfs_stat_ok
    move.l  r28, #ERR_NOTFOUND
    beq     r3, r28, .dos_run_file_lookup
    bra     .dos_run_reply_notfound
.dos_run_hostfs_stat_ok:
    move.q  r25, r1                    ; image size
    beqz    r25, .dos_run_reply_notfound
    store.q r25, 624(r29)              ; preserve hostfs alloc size across syscalls
    load.q  r24, 608(r29)
    beqz    r24, .dos_run_reply_notfound
    move.q  r1, r24
    push    r29
    jsr     .dos_bootfs_open
    pop     r29
    bnez    r2, .dos_run_reply_notfound
    move.q  r20, r1                    ; host handle
    store.q r20, 320(r29)

    load.q  r25, 624(r29)
    push    r29
    move.q  r1, r25
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_run_hostfs_nomem_close
    store.q r1, 296(r29)               ; temp contiguous image

    load.q  r20, 320(r29)
    move.q  r1, r20
    load.q  r2, 296(r29)
    move.q  r3, r25
    push    r29
    jsr     .dos_bootfs_read_all
    pop     r29
    store.q r1, 640(r29)               ; preserve actual bytes read separately from alloc size
    move.q  r26, r2
    load.q  r20, 320(r29)
    move.q  r1, r20
    push    r29
    jsr     .dos_bootfs_close
    pop     r29
    bnez    r26, .dos_run_hostfs_free_tmp_err
    load.q  r21, 296(r29)
    load.q  r23, 640(r29)
    move.l  r15, #1
    store.q r15, 632(r29)
    bra     .dos_run_parse_args

.dos_run_hostfs_nomem_close:
    load.q  r20, 320(r29)
    move.q  r1, r20
    jsr     .dos_bootfs_close
    bra     .dos_run_reply_notfound

.dos_run_hostfs_free_tmp_err:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_run_reply_notfound

.dos_run_file_lookup:
    ; 3. M12.6 Phase A: walk metadata chain by name
    ; Preserve the resolved command-name pointer before helper calls:
    ; .dos_meta_find_by_name clobbers high registers, and the M14.1
    ; manifest-name fallback still needs the original resolved name on a
    ; miss. Without this, unknown commands can pass a null/stale pointer
    ; into .dos_manifest_find_row_by_name and fault dos.library.
    load.q  r23, 336(r29)
    move.q  r1, r23
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    beqz    r1, .dos_run_notfound
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r29, (sp)
    bra     .dos_run_found

.dos_run_notfound:
    load.q  r29, (sp)
.dos_run_reply_notfound:
    load.q  r29, (sp)
    ; M15.3: bump the op-specific attempt index and retry. Resolution
    ; returns not-found once the effective list is exhausted; in that
    ; case resolve_cmd leaves r22=0 and the per-op retry entry branches
    ; to .dos_run_reply_notfound_final at the top of the op.
    load.q  r27, 952(r29)              ; opcode
    move.l  r26, #DOS_LOADSEG
    beq     r27, r26, .drn_retry_loadseg
    ; DOS_RUN / DOS_RUNSEG path — attempt index at 656.
    load.q  r28, 656(r29)
    add     r28, r28, #1
    store.q r28, 656(r29)
    move.l  r27, #8
    bge     r28, r27, .dos_run_reply_notfound_final
    bra     .dos_do_run_retry
.drn_retry_loadseg:
    load.q  r28, 664(r29)
    add     r28, r28, #1
    store.q r28, 664(r29)
    move.l  r27, #8
    bge     r28, r27, .dos_run_reply_notfound_final
    bra     .dos_do_loadseg_retry
.dos_run_reply_notfound_final:
    ; Reply with DOS_ERR_NOTFOUND
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_NOTFOUND
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_found:
    ; r14 = entry VA. M12.8 Phase 2: file body is an extent chain.
    ; SYS_EXEC_PROGRAM needs a contiguous image_ptr, so we:
    ;   1. AllocMem a temp contiguous buffer (image_size bytes)
    ;   2. Walk the extent chain into the temp buffer
    ;   3. Pass the temp buffer to SYS_EXEC_PROGRAM
    ;   4. FreeMem the temp after SYS_EXEC_PROGRAM returns (the kernel
    ;      has already copied the image into the new task's slot at
    ;      that point — see load_program in iexec.s ~line 700).
    ;
    ; Handler scratch slots in dos.library data page (DOS_RUN-specific,
    ; non-overlapping with DOS_WRITE's 256..279):
    ;   280: first_extent_va (8 bytes — for the walker call)
    ;   288: image_size      (8 bytes — for walker count + later FreeMem)
    ;   296: temp_buf_va     (8 bytes — for FreeMem after exec)
    load.q  r21, DOS_META_OFF_VA(r14)   ; r21 = first extent VA (or 0)
    load.l  r23, DOS_META_OFF_SIZE(r14) ; r23 = image size

    ; A zero-size or empty-body file cannot be a valid program.
    beqz    r21, .dos_run_notfound
    beqz    r23, .dos_run_notfound

    ; Save state to scratch slots BEFORE any syscall (syscalls clobber regs)
    store.q r21, 280(r29)               ; first_extent_va
    store.q r23, 288(r29)               ; image_size

    ; ---- AllocMem a temp contiguous buffer of size = image_size ----
    push    r29
    move.q  r1, r23                     ; r1 = image size
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM              ; r1 = temp buf VA, r2 = err
    pop     r29
    bnez    r2, .dos_run_notfound       ; AllocMem failure → treat as notfound
    store.q r1, 296(r29)                ; saved temp buf VA

    ; ---- Walk the extent chain into the temp buffer ----
    load.q  r1, 280(r29)                ; r1 = first_extent_va
    load.q  r2, 296(r29)                ; r2 = dst = temp buf VA
    load.q  r3, 288(r29)                ; r3 = byte_count = image size
    jsr     .dos_extent_walk
    load.q  r29, (sp)
    load.q  r21, 296(r29)               ; r21 = temp buf VA (image_ptr for exec)
    load.q  r23, 288(r29)               ; r23 = image size

.dos_run_parse_args:
    ; 5. Find args: scan past command name null in shared buffer.
    ; Bounded scan — without a length cap, a malicious caller could
    ; send a shared buffer with no terminator and walk dos.library
    ; off the mapped page (faulting the service). DATA_ARGS_MAX (256)
    ; is the same upper bound used by the args-length scan below; a
    ; command name longer than that is treated as malformed input
    ; and routed to the DOS_ERR_NOTFOUND reply.
    load.q  r20, 168(r29)              ; original shared buffer
    move.q  r16, r20
    move.l  r24, #0                    ; r24 = scan counter
.dos_run_skip_cmd:
    load.b  r15, (r16)
    beqz    r15, .dos_run_args_start
    add     r16, r16, #1
    add     r24, r24, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r24, r28, .dos_run_skip_cmd
    ; Hit cap without finding a NUL — malformed request, bail out.
    bra     .dos_run_notfound
.dos_run_args_start:
    add     r16, r16, #1               ; skip the null → args start
    ; Compute args_len (scan for second null)
    move.q  r17, r16                   ; args_ptr
    move.l  r18, #0                    ; args_len
.dos_run_arglen:
    load.b  r15, (r17)
    beqz    r15, .dos_run_launch
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r18, r28, .dos_run_arglen

.dos_run_launch:
    load.q  r15, 632(r29)
    bnez    r15, .dos_run_launch_hostfs_elf
    ; Prefer the M14 native path for ELF files. M14.2 phase 1 rejects legacy
    ; flat-image IE64PROG executables instead of falling back to SYS_EXEC_PROGRAM.
    load.l  r15, (r21)
    move.l  r28, #0x464C457F
    bne     r15, r28, .dos_run_reject_flat
    store.q r16, 320(r29)               ; saved args_ptr for ELF path
    store.q r18, 328(r29)               ; saved args_len for ELF path

    move.q  r1, r21                    ; temp contiguous ELF image
    move.q  r2, r23                    ; image_size
    jsr     .dos_validate_loaded_elf_aslr_contract
    bnez    r20, .dos_run_meta_reject_badarg

    move.q  r1, r21                    ; temp contiguous ELF image
    move.q  r2, r23                    ; image_size
    move.q  r30, r29
    jsr     .dos_elf_build_seglist
    move.q  r29, r30
    store.q r1, 304(r29)               ; seglist VA (temp buf still lives in 296)
    store.q r2, 312(r29)               ; DOS build result

    push    r29
    load.q  r1, 296(r29)               ; free temp contiguous image
    load.q  r2, 288(r29)
    syscall #SYS_FREE_MEM
    pop     r29

    load.q  r15, 312(r29)
    beqz    r15, .dos_run_launch_seglist
    load.q  r1, 944(r29)
    move.q  r2, r15
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_meta_reject_badarg:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 288(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_launch_hostfs_elf:
    load.l  r15, (r21)
    move.l  r28, #0x464C457F
    bne     r15, r28, .dos_run_reject_flat
    push    r18
    push    r16
    push    r23
    push    r21
    move.q  r1, r21
    move.q  r2, r23
    jsr     .dos_validate_loaded_elf_aslr_contract
    pop     r21
    pop     r23
    pop     r16
    pop     r18
    bnez    r20, .dos_run_hostfs_reject_badarg
    push    r29
    move.q  r1, r21
    move.q  r2, r23
    move.q  r3, r16
    move.q  r4, r18
    load.q  r5, 648(r29)
    syscall #SYS_BOOT_ELF_EXEC
    pop     r29
    store.q r1, 304(r29)
    store.q r2, 312(r29)
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    load.q  r1, 944(r29)
    load.q  r2, 312(r29)
    load.q  r3, 304(r29)
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_hostfs_reject_badarg:
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    load.q  r29, (sp)
    store.q r0, 304(r29)
    move.l  r15, #DOS_ERR_BADARG
    store.q r15, 312(r29)
    store.q r29, (sp)
    load.q  r1, 944(r29)
    load.q  r2, 312(r29)
    load.q  r3, 304(r29)
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_launch_seglist:
    load.q  r1, 304(r29)               ; seglist VA
    store.q r1, 296(r29)               ; repurpose temp slot after free
    load.q  r2, 320(r29)               ; args_ptr
    load.q  r3, 328(r29)               ; args_len
    move.q  r30, r29                   ; preserve DOS data-page base across helper return
    move.q  r19, r29                   ; explicit DOS data-page anchor
    jsr     .dos_launch_seglist
    move.q  r29, r30
    store.q r1, 304(r29)               ; saved task_id
    store.q r2, 312(r29)               ; saved DOS err

    load.q  r1, 296(r29)
    beqz    r1, .dos_run_free_seglist
    load.q  r2, 176(r29)
    load.q  r3, DOS_SEGLIST_NEXT(r1)
    beq     r2, r1, .dos_run_unlink_head
    bra     .dos_run_free_seglist
.dos_run_unlink_head:
    store.q r3, 176(r29)
.dos_run_free_seglist:
    load.q  r1, 296(r29)
    jsr     .dos_seglist_free_unlinked

    load.q  r1, 944(r29)
    store.q r29, (sp)
    load.q  r2, 312(r29)               ; DOS reply type
    load.q  r3, 304(r29)               ; data0 = task_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_reject_flat:
    ; Flat executable content is no longer a valid DOS_RUN target.
    push    r29
    load.q  r1, 296(r29)               ; r1 = temp buf VA
    load.q  r2, 288(r29)               ; r2 = image size (matches AllocMem)
    syscall #SYS_FREE_MEM
    pop     r29
    load.q  r29, (sp)
    store.q r0, 304(r29)               ; task_id = 0
    move.l  r15, #DOS_ERR_BADARG
    store.q r15, 312(r29)              ; err = DOS_ERR_BADARG

    ; Reply: type=err, data0=task_id
    store.q r29, (sp)
    load.q  r1, 944(r29)
    load.q  r2, 312(r29)               ; type = err
    load.q  r3, 304(r29)               ; data0 = task_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_LOADLIB (type=11): internal M16 runtime library autoload.
    ; data0 = module row index. No reply message is emitted; dos.library
    ; reports success/failure back to exec via the trusted-internal M16
    ; load-result syscall.
    ; =================================================================
.dos_do_loadlib:
    load.q  r20, 960(r29)              ; module row index
    add     r21, r29, #1000            ; 32-byte name scratch
    add     r22, r29, #704             ; "LIBS:<name>" buffer in dead-space scratch
    store.q r0, 616(r29)               ; clear stale bootfs handle before any open
    move.q  r1, r20
    move.q  r2, r21
    syscall #SYS_M16_COPY_MODULE_NAME
    load.q  r29, (sp)
    load.q  r20, 960(r29)
    add     r21, r29, #1000
    add     r22, r29, #704
    bnez    r2, .dos_loadlib_fail

    move.l  r11, #0x4C                 ; 'L'
    store.b r11, 0(r22)
    move.l  r11, #0x49                 ; 'I'
    store.b r11, 1(r22)
    move.l  r11, #0x42                 ; 'B'
    store.b r11, 2(r22)
    move.l  r11, #0x53                 ; 'S'
    store.b r11, 3(r22)
    move.l  r11, #0x3A                 ; ':'
    store.b r11, 4(r22)
    move.l  r23, #0
.dos_loadlib_copy_name:
    move.l  r11, #32
    bge     r23, r11, .dos_loadlib_name_done
    add     r24, r21, r23
    load.b  r25, (r24)
    add     r26, r22, r23
    add     r26, r26, #5
    store.b r25, (r26)
    beqz    r25, .dos_loadlib_name_done
    add     r23, r23, #1
    bra     .dos_loadlib_copy_name
.dos_loadlib_name_done:
    add     r26, r22, r23
    add     r26, r26, #5
    store.b r0, (r26)

    move.q  r23, r22
    push    r20
    push    r29
    jsr     .dos_resolve_file
    pop     r29
    pop     r20
    beqz    r22, .dos_loadlib_fail
    move.q  r1, r23
    move.q  r2, r0
    push    r20
    push    r29
    jsr     .dos_hostfs_layered_relpath_for_resolved_name
    pop     r29
    pop     r20
    beqz    r3, .dos_loadlib_fail
    move.q  r23, r1
    store.q r23, 608(r29)
    move.q  r1, r23
    push    r29
    jsr     .dos_bootfs_stat
    pop     r29
    bnez    r3, .dos_loadlib_fail
    move.q  r25, r1
    beqz    r25, .dos_loadlib_fail
    store.q r25, 624(r29)
    move.q  r28, r2
    and     r28, r28, #0xFFFFFFFF
    move.q  r15, r0
    add     r15, r15, #BOOT_HOSTFS_KIND_FILE
    bne     r28, r15, .dos_loadlib_fail

    load.q  r24, 608(r29)
    beqz    r24, .dos_loadlib_fail
    move.q  r1, r24
    push    r29
    jsr     .dos_bootfs_open
    pop     r29
    bnez    r2, .dos_loadlib_report_fail
    move.q  r24, r1
    store.q r24, 616(r29)

    load.q  r25, 624(r29)
    push    r29
    move.q  r1, r25
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_loadlib_nomem_close
    store.q r1, 296(r29)

    load.q  r24, 616(r29)
    move.q  r1, r24
    load.q  r2, 296(r29)
    move.q  r3, r25
    push    r29
    jsr     .dos_bootfs_read_all
    pop     r29
    store.q r1, 640(r29)
    move.q  r26, r2
    load.q  r24, 616(r29)
    move.q  r1, r24
    push    r29
    jsr     .dos_bootfs_close
    pop     r29
    bnez    r26, .dos_loadlib_free_tmp_readerr

    load.q  r21, 296(r29)
    load.q  r23, 640(r29)
    load.l  r15, (r21)
    move.l  r28, #0x464C457F
    bne     r15, r28, .dos_loadlib_free_tmp_nonelf

    push    r23
    push    r21
    move.q  r1, r21
    move.q  r2, r23
    jsr     .dos_validate_loaded_elf_aslr_contract
    pop     r21
    pop     r23
    bnez    r20, .dos_loadlib_free_tmp_badarg

    add     r16, r29, #(prog_doslib_empty_args - prog_doslib_data)
    move.q  r18, r0
    move.l  r17, #BOOT_ELF_EXEC_FLAG_TRUSTED_INTERNAL
    push    r29
    move.q  r1, r21
    move.q  r2, r23
    move.q  r3, r16
    move.q  r4, r18
    move.q  r5, r17
    syscall #SYS_BOOT_ELF_EXEC
    pop     r29
    store.q r1, 304(r29)
    store.q r2, 312(r29)

    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29

    load.q  r24, 304(r29)
    load.q  r2, 312(r29)
    bnez    r2, .dos_loadlib_report_fail
    beqz    r24, .dos_loadlib_report_fail
    load.q  r20, 960(r29)
    move.q  r1, r20
    move.q  r2, r24
    move.q  r3, #ERR_OK
    syscall #SYS_M16_LIBRARY_LOAD_RESULT
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_loadlib_fail:
    move.l  r2, #DOS_ERR_NOTFOUND
.dos_loadlib_report_only:
    bra     .dos_loadlib_report_fail
.dos_loadlib_nomem_close:
    load.q  r24, 616(r29)
    move.q  r1, r24
    push    r29
    jsr     .dos_bootfs_close
    pop     r29
    bra     .dos_loadlib_report_fail
.dos_loadlib_free_tmp_readerr:
    push    r26
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    pop     r2
    bra     .dos_loadlib_report_fail
.dos_loadlib_free_tmp_nonelf:
.dos_loadlib_free_tmp_badarg:
    move.l  r2, #DOS_ERR_BADARG
    push    r29
    load.q  r1, 296(r29)
    load.q  r2, 624(r29)
    syscall #SYS_FREE_MEM
    pop     r29
    move.l  r2, #DOS_ERR_BADARG
.dos_loadlib_report_fail:
    load.q  r20, 960(r29)
    move.q  r1, r20
    move.q  r3, r2
    move.q  r2, r0
    syscall #SYS_M16_LIBRARY_LOAD_RESULT
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; Shared reply blocks (saves code space by consolidating duplicates)
    ; =================================================================
.dos_reply_badh:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_BADHANDLE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_full:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_FULL
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_badarg:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_nomem:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_NOMEM
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_err:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_NOTFOUND
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; ==================================================================
    ; M12.6 Phase A: dos.library chain-allocator helpers
    ; ==================================================================
    ; The file metadata table and the open-handle table are no longer
    ; fixed-size inline arrays. Each is a singly-linked list of 4 KiB
    ; AllocMem'd "chain pages":
    ;
    ;   chain page (4 KiB):
    ;     [0..7]    next_va (8 bytes, 0 = end of chain)
    ;     [8..15]   reserved
    ;     [16..]    entries (per-table stride)
    ;
    ; Metadata entries are 48 bytes (DOS_META_PER_PAGE = 85 per page).
    ; Handle entries are 8 bytes (DOS_HND_PER_PAGE = 510 per page).
    ;
    ; Both helpers preserve r29 (data base) implicitly via the stack
    ; convention used by every other dos.library subroutine.

    ; ------------------------------------------------------------------
    ; .dos_meta_alloc_entry: walk the metadata chain looking for an
    ; entry whose name[0] == 0 (free). If none, allocate a new chain
    ; page and use entry 0 of the new page.
    ; In:  r29 = dos.library data base
    ; Out: r1  = entry VA (always non-zero on success)
    ;      r2  = ERR_OK or err code
    ; Clobbers: r3..r19, r25, r26
    ; ------------------------------------------------------------------
.dos_meta_alloc_entry:
    load.q  r25, 152(r29)              ; r25 = current chain page VA
.dmae_walk_pages:
    beqz    r25, .dmae_alloc_new       ; chain head not allocated yet
    move.q  r26, r25
    add     r26, r26, #DOS_META_HDR_SZ ; r26 = &entries[0]
    move.l  r3, #0                     ; entry index
.dmae_scan_rows:
    move.l  r4, #DOS_META_PER_PAGE
    bge     r3, r4, .dmae_next_page
    load.b  r5, DOS_META_OFF_NAME(r26) ; first byte of name
    beqz    r5, .dmae_found            ; free entry
    add     r26, r26, #DOS_META_ENTRY_SZ
    add     r3, r3, #1
    bra     .dmae_scan_rows
.dmae_next_page:
    load.q  r25, (r25)                 ; next page VA
    bra     .dmae_walk_pages
.dmae_found:
    move.q  r1, r26
    move.q  r2, r0                     ; ERR_OK = 0
    rts
.dmae_alloc_new:
    ; Save r29 across the syscall (AllocMem clobbers user regs).
    push    r29
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; r1 = VA, r2 = err
    pop     r29
    bnez    r2, .dmae_fail
    move.q  r25, r1                    ; r25 = new page VA
    ; Walk to the tail of the existing chain and link the new page.
    load.q  r3, 152(r29)               ; head
    beqz    r3, .dmae_set_head
.dmae_link_walk:
    load.q  r4, (r3)
    beqz    r4, .dmae_at_tail
    move.q  r3, r4
    bra     .dmae_link_walk
.dmae_at_tail:
    store.q r25, (r3)                  ; tail.next_va = new page
    bra     .dmae_link_done
.dmae_set_head:
    store.q r25, 152(r29)              ; head = new page
.dmae_link_done:
    ; Use entry 0 of the new page (already zero from MEMF_CLEAR).
    add     r1, r25, #DOS_META_HDR_SZ
    move.q  r2, r0                     ; ERR_OK
    rts
.dmae_fail:
    move.q  r1, r0
    ; r2 already has the err code from AllocMem
    rts

    ; ------------------------------------------------------------------
    ; .dos_meta_find_by_name: walk the metadata chain comparing each
    ; entry's name to the request name (case-insensitive, max 32 chars).
    ; Returns the entry VA on match, or 0 if not found.
    ; In:  r1  = request name VA (NUL-terminated, in dos.library AS)
    ;      r29 = data base
    ; Out: r1  = entry VA (or 0 if not found)
    ; Clobbers: r3..r12, r25, r26, r27
    ; ------------------------------------------------------------------
.dos_meta_find_by_name:
    move.q  r27, r1                    ; r27 = request name (preserved)
    load.q  r25, 152(r29)              ; chain head
.dmfn_walk_pages:
    beqz    r25, .dmfn_not_found
    move.q  r26, r25
    add     r26, r26, #DOS_META_HDR_SZ
    move.l  r3, #0                     ; entry index in page
.dmfn_scan_rows:
    move.l  r4, #DOS_META_PER_PAGE
    bge     r3, r4, .dmfn_next_page
    load.b  r5, DOS_META_OFF_NAME(r26)
    beqz    r5, .dmfn_skip             ; empty entry → skip
    ; Case-insensitive name compare (max 32 bytes).
    move.q  r6, r26                    ; r6 = entry name ptr
    move.q  r7, r27                    ; r7 = request name ptr
    move.l  r8, #0                     ; char index
.dmfn_cmp:
    load.b  r9, (r6)
    load.b  r10, (r7)
    move.l  r11, #0x41                 ; 'A'
    blt     r9, r11, .dmfn_skip1
    move.l  r11, #0x5A                 ; 'Z'
    bgt     r9, r11, .dmfn_skip1
    or      r9, r9, #0x20
.dmfn_skip1:
    move.l  r11, #0x41
    blt     r10, r11, .dmfn_skip2
    move.l  r11, #0x5A
    bgt     r10, r11, .dmfn_skip2
    or      r10, r10, #0x20
.dmfn_skip2:
    bne     r9, r10, .dmfn_skip
    beqz    r9, .dmfn_match            ; both null → match
    add     r6, r6, #1
    add     r7, r7, #1
    add     r8, r8, #1
    move.l  r11, #32
    blt     r8, r11, .dmfn_cmp
    bra     .dmfn_match                ; reached 32 chars → treat as match
.dmfn_skip:
    add     r26, r26, #DOS_META_ENTRY_SZ
    add     r3, r3, #1
    bra     .dmfn_scan_rows
.dmfn_next_page:
    load.q  r25, (r25)
    bra     .dmfn_walk_pages
.dmfn_match:
    move.q  r1, r26
    rts
.dmfn_not_found:
    move.q  r1, r0
    rts

    ; ------------------------------------------------------------------
    ; .dos_hnd_alloc: walk the handle chain looking for an unused slot
    ; (entry == 0). If none, allocate a new chain page and use entry 0.
    ; Stores the supplied metadata entry VA in the slot and returns the
    ; integer handle_id (page_index * DOS_HND_PER_PAGE + slot_in_page).
    ; In:  r1  = metadata entry VA to store in the slot (must be non-zero)
    ;      r29 = data base
    ; Out: r1  = handle_id (>= 0 on success)
    ;      r2  = ERR_OK or err code
    ; Clobbers: r3..r19, r25, r26
    ; ------------------------------------------------------------------
.dos_hnd_alloc:
    move.q  r19, r1                    ; r19 = entry VA (preserved)
    load.q  r25, 160(r29)              ; r25 = current handle page VA
    move.l  r12, #0                    ; r12 = page index
.dha_walk_pages:
    beqz    r25, .dha_alloc_new
    move.q  r26, r25
    add     r26, r26, #DOS_HND_HDR_SZ  ; r26 = &slots[0]
    move.l  r3, #0                     ; slot index
.dha_scan_rows:
    move.l  r4, #DOS_HND_PER_PAGE
    bge     r3, r4, .dha_next_page
    load.q  r5, (r26)
    beqz    r5, .dha_found
    add     r26, r26, #DOS_HND_ENTRY_SZ
    add     r3, r3, #1
    bra     .dha_scan_rows
.dha_next_page:
    load.q  r25, (r25)
    add     r12, r12, #1
    bra     .dha_walk_pages
.dha_found:
    store.q r19, (r26)                 ; slot = entry VA
    ; handle_id = page_index * DOS_HND_PER_PAGE + slot_index
    move.l  r4, #DOS_HND_PER_PAGE
    mulu    r4, r12, r4
    add     r1, r4, r3
    move.q  r2, r0                     ; ERR_OK
    rts
.dha_alloc_new:
    push    r29
    push    r19
    push    r12
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r12
    pop     r19
    pop     r29
    bnez    r2, .dha_fail
    move.q  r25, r1                    ; new page VA
    ; Link onto chain.
    load.q  r3, 160(r29)               ; head
    beqz    r3, .dha_set_head
.dha_link_walk:
    load.q  r4, (r3)
    beqz    r4, .dha_at_tail
    move.q  r3, r4
    bra     .dha_link_walk
.dha_at_tail:
    store.q r25, (r3)
    bra     .dha_link_done
.dha_set_head:
    store.q r25, 160(r29)              ; head = new page
.dha_link_done:
    ; Use slot 0 of the new page.
    add     r26, r25, #DOS_HND_HDR_SZ
    store.q r19, (r26)
    move.l  r4, #DOS_HND_PER_PAGE
    mulu    r4, r12, r4
    move.q  r1, r4                     ; slot index = 0
    move.q  r2, r0                     ; ERR_OK
    rts
.dha_fail:
    move.q  r1, r0
    ; r2 has err code from AllocMem
    rts

    ; ------------------------------------------------------------------
    ; .dos_hnd_lookup: walk handle chain to slot at handle_id, return
    ; the metadata entry VA stored there. Returns 0 if handle_id is
    ; out of range or the slot is empty.
    ; In:  r1  = handle_id
    ;      r29 = data base
    ; Out: r1  = metadata entry VA (or 0)
    ;      r2  = slot VA inside the chain page (for callers that need
    ;            to clear the slot, e.g. DOS_CLOSE; 0 if not found)
    ; Clobbers: r3..r10, r25
    ; ------------------------------------------------------------------
.dos_hnd_lookup:
    bltz    r1, .dhl_not_found
    move.l  r3, #DOS_HND_PER_PAGE
    divu    r4, r1, r3                 ; r4 = page index
    mulu    r5, r4, r3
    sub     r5, r1, r5                 ; r5 = slot in page
    load.q  r25, 160(r29)              ; chain head
.dhl_walk:
    beqz    r25, .dhl_not_found
    beqz    r4, .dhl_at_page
    load.q  r25, (r25)
    sub     r4, r4, #1
    bra     .dhl_walk
.dhl_at_page:
    move.l  r6, #DOS_HND_ENTRY_SZ
    mulu    r6, r5, r6
    add     r6, r6, #DOS_HND_HDR_SZ
    add     r6, r25, r6                ; r6 = slot VA
    load.q  r1, (r6)
    beqz    r1, .dhl_empty
    move.q  r2, r6
    rts
.dhl_empty:
    move.q  r1, r0
    move.q  r2, r0
    rts
.dhl_not_found:
    move.q  r1, r0
    move.q  r2, r0
    rts

    ; ==================================================================
    ; M12.8 Phase 1 — file body extent allocator (DEAD CODE in Phase 1).
    ;
    ; These helpers allocate, free, and walk a chain of 4 KiB extents
    ; that will replace the fixed-size DOS_FILE_SIZE per-file body in
    ; Phase 2. They are wired into NO existing code path in Phase 1;
    ; they only need to assemble cleanly and not break any existing
    ; test. Phase 2 will switch DOS_OPEN/READ/WRITE over and remove
    ; the DOS_FILE_SIZE cap.
    ;
    ; Extent layout (one AllocMem'd 4 KiB page):
    ;   [0..7]    next_va (0 = end of chain)
    ;   [8..15]   reserved
    ;   [16..4095] payload (DOS_EXT_PAYLOAD = 4080 bytes)
    ; ==================================================================

    ; ------------------------------------------------------------------
    ; .dos_extent_alloc: allocate a chain of 4 KiB extents large enough
    ; to hold byte_count payload bytes.
    ; In:  r1  = byte_count (>=0; 0 means "no body, return first_va=0")
    ;      r29 = data base
    ; Out: r1  = first_extent_va (or 0 if byte_count==0 or alloc failed)
    ;      r2  = ERR_OK on success, AllocMem err code on failure
    ; Clobbers: r3, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_alloc:
    beqz    r1, .dea_zero_size
    ; n_extents = ceil(byte_count / DOS_EXT_PAYLOAD)
    add     r17, r1, #(DOS_EXT_PAYLOAD - 1)
    move.l  r3, #DOS_EXT_PAYLOAD
    divu    r17, r17, r3                ; r17 = remaining extents to allocate
    ; Allocate the first extent.
    push    r29
    push    r17
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r17
    pop     r29
    bnez    r2, .dea_fail_first
    move.q  r19, r1                     ; r19 = first_va (preserved)
    move.q  r18, r1                     ; r18 = tail_va (advances each loop)
    sub     r17, r17, #1
.dea_loop:
    beqz    r17, .dea_done
    push    r29
    push    r19
    push    r18
    push    r17
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r17
    pop     r18
    pop     r19
    pop     r29
    bnez    r2, .dea_fail_partial
    store.q r1, DOS_EXT_OFF_NEXT(r18)   ; tail.next_va = new extent
    move.q  r18, r1                     ; advance tail
    sub     r17, r17, #1
    bra     .dea_loop
.dea_done:
    move.q  r1, r19
    move.q  r2, r0                      ; ERR_OK
    rts
.dea_zero_size:
    move.q  r1, r0
    move.q  r2, r0
    rts
.dea_fail_first:
    move.q  r1, r0
    ; r2 already holds the AllocMem err code
    rts
.dea_fail_partial:
    ; r2 holds the AllocMem err code; preserve it across the free call.
    push    r2
    move.q  r1, r19                     ; head of partially-allocated chain
    jsr     .dos_extent_free            ; preserves r29 internally
    pop     r2
    move.q  r1, r0
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_free: walk an extent chain and FreeMem each page.
    ; Safe to call with r1 == 0 (no-op).
    ; In:  r1  = first_extent_va (or 0)
    ;      r29 = data base
    ; Out: r2  = ERR_OK
    ; Clobbers: r1, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_free:
    move.q  r19, r1                     ; r19 = current extent
.def_loop:
    beqz    r19, .def_done
    load.q  r18, DOS_EXT_OFF_NEXT(r19)  ; r18 = next extent
    push    r29
    push    r18
    move.q  r1, r19
    move.l  r2, #4096
    syscall #SYS_FREE_MEM
    pop     r18
    pop     r29
    move.q  r19, r18
    bra     .def_loop
.def_done:
    move.q  r2, r0                      ; ERR_OK
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_walk: copy up to byte_count bytes from the start of an
    ; extent chain into a destination buffer. Returns the number of
    ; bytes actually copied — equals byte_count if the chain is long
    ; enough, otherwise the chain length in bytes.
    ; In:  r1  = first_extent_va (or 0)
    ;      r2  = dst VA
    ;      r3  = byte_count to copy
    ;      r29 = data base
    ; Out: r1  = bytes copied
    ; Clobbers: r4, r5, r6, r7, r8, r16, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_walk:
    move.q  r19, r1                     ; r19 = current extent VA
    move.q  r18, r2                     ; r18 = dst
    move.q  r17, r3                     ; r17 = remaining bytes to copy
    move.q  r16, r0                     ; r16 = total copied
.dew_extent_loop:
    beqz    r19, .dew_done
    beqz    r17, .dew_done
    ; n = min(r17, DOS_EXT_PAYLOAD)
    move.q  r4, r17
    move.l  r5, #DOS_EXT_PAYLOAD
    blt     r4, r5, .dew_have_n
    move.q  r4, r5
.dew_have_n:
    ; r4 = bytes to copy this extent
    add     r5, r19, #DOS_EXT_HDR_SZ    ; src = extent + header
    move.q  r6, r18                     ; dst
    move.q  r7, r0                      ; counter
.dew_byte_copy:
    bge     r7, r4, .dew_extent_done
    load.b  r8, (r5)
    store.b r8, (r6)
    add     r5, r5, #1
    add     r6, r6, #1
    add     r7, r7, #1
    bra     .dew_byte_copy
.dew_extent_done:
    add     r16, r16, r4                ; total += n
    add     r18, r18, r4                ; dst += n
    sub     r17, r17, r4                ; remaining -= n
    load.q  r19, DOS_EXT_OFF_NEXT(r19)  ; advance to next extent
    bra     .dew_extent_loop
.dew_done:
    move.q  r1, r16
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_write: copy up to byte_count bytes from a source
    ; buffer into the start of an extent chain. Symmetric counterpart
    ; to .dos_extent_walk. The extent chain MUST already have been
    ; allocated (via .dos_extent_alloc) with enough capacity; this
    ; helper does NOT grow the chain. Returns bytes actually written
    ; (= byte_count when the chain has enough capacity, otherwise the
    ; chain capacity in bytes).
    ; In:  r1  = first_extent_va (or 0)
    ;      r2  = src VA
    ;      r3  = byte_count to copy
    ;      r29 = data base
    ; Out: r1  = bytes written
    ; Clobbers: r4, r5, r6, r7, r8, r16, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_write:
    move.q  r19, r1                     ; r19 = current extent VA
    move.q  r18, r2                     ; r18 = src
    move.q  r17, r3                     ; r17 = remaining bytes to copy
    move.q  r16, r0                     ; r16 = total written
.dexw_extent_loop:
    beqz    r19, .dexw_done
    beqz    r17, .dexw_done
    ; n = min(r17, DOS_EXT_PAYLOAD)
    move.q  r4, r17
    move.l  r5, #DOS_EXT_PAYLOAD
    blt     r4, r5, .dexw_have_n
    move.q  r4, r5
.dexw_have_n:
    ; r4 = bytes to copy this extent
    add     r5, r19, #DOS_EXT_HDR_SZ    ; dst = extent + header
    move.q  r6, r18                     ; src = caller buffer
    move.q  r7, r0                      ; counter
.dexw_byte_copy:
    bge     r7, r4, .dexw_extent_done
    load.b  r8, (r6)
    store.b r8, (r5)
    add     r5, r5, #1
    add     r6, r6, #1
    add     r7, r7, #1
    bra     .dexw_byte_copy
.dexw_extent_done:
    add     r16, r16, r4                ; total += n
    add     r18, r18, r4                ; src += n
    sub     r17, r17, r4                ; remaining -= n
    load.q  r19, DOS_EXT_OFF_NEXT(r19)  ; advance to next extent
    bra     .dexw_extent_loop
.dexw_done:
    move.q  r1, r16
    rts

    ; ------------------------------------------------------------------
    ; .dos_seglist_free_unlinked: free a DOS-owned seglist and all segment
    ; allocations it references. Does not touch the global seglist list.
    ; In:  r1 = seglist VA (or 0)
    ; Out: r2 = ERR_OK
    ; ------------------------------------------------------------------
.dos_seglist_free_unlinked:
    push    r29
    move.q  r19, r1
.dslf_entry:
    beqz    r19, .dslf_done
    load.l  r18, DOS_SEGLIST_OFF_COUNT(r19)
    lsl     r18, r18, #32
    lsr     r18, r18, #32
    move.l  r17, #0
.dslf_loop:
    bge     r17, r18, .dslf_free_hdr
    move.l  r4, #DOS_SEG_ENTRY_SZ
    mulu    r4, r17, r4
    add     r4, r4, #DOS_SEGLIST_HDR_SZ
    add     r4, r4, r19
    load.q  r1, DOS_SEG_OFF_MEMVA(r4)
    beqz    r1, .dslf_next
    load.l  r2, DOS_SEG_OFF_PAGES(r4)
    lsl     r2, r2, #32
    lsr     r2, r2, #32
    lsl     r2, r2, #12
    push    r29
    push    r19
    push    r18
    push    r17
    push    r4
    syscall #SYS_FREE_MEM
    pop     r4
    pop     r17
    pop     r18
    pop     r19
    pop     r29
.dslf_next:
    add     r17, r17, #1
    bra     .dslf_loop
.dslf_free_hdr:
    push    r29
    push    r19
    move.q  r1, r19
    move.l  r2, #4096
    syscall #SYS_FREE_MEM
    pop     r19
    pop     r29
.dslf_done:
    pop     r29
    move.q  r2, r0
    rts

    ; ------------------------------------------------------------------
    ; .dos_elf_build_seglist: validate a Phase-1 M14 ELF image already
    ; copied into a contiguous DOS buffer and build a DOS-owned seglist.
    ; In:  r1 = file VA, r2 = file size
    ; Out: r1 = seglist VA (or 0), r2 = DOS_OK / DOS_ERR_BADARG / DOS_ERR_NOMEM
    ; Scratch in data page:
    ;   360 file_va      368 file_size    376 seglist_va
    ;   384 target_va    392 filesz       400 memsz
    ;   408 file_off     416 flags        424 pages
    ;   440 entry_va     464 entry_seen
    ; ------------------------------------------------------------------
.dos_elf_build_seglist:
    store.q r1, 360(r29)
    store.q r2, 368(r29)
    store.q r0, 376(r29)
    store.q r0, 464(r29)

    move.l  r3, #64
    blt     r2, r3, .debs_badarg
    load.l  r3, (r1)
    move.l  r4, #0x464C457F            ; "\x7FELF" little-endian
    bne     r3, r4, .debs_badarg
    load.b  r3, 4(r1)
    move.l  r4, #2
    bne     r3, r4, .debs_badarg
    load.b  r3, 5(r1)
    move.l  r4, #1
    bne     r3, r4, .debs_badarg
    load.b  r3, 6(r1)
    move.l  r4, #1
    bne     r3, r4, .debs_badarg
    load.b  r3, 7(r1)
    bnez    r3, .debs_badarg
    load.l  r3, 16(r1)
    and     r3, r3, #0xFFFF
    move.l  r4, #2
    bne     r3, r4, .debs_badarg
    load.l  r3, 18(r1)
    and     r3, r3, #0xFFFF
    move.l  r4, #0x4945
    bne     r3, r4, .debs_badarg
    load.l  r3, 20(r1)
    move.l  r4, #1
    bne     r3, r4, .debs_badarg
    load.l  r3, 28(r1)
    bnez    r3, .debs_badarg
    load.l  r3, 24(r1)
    beqz    r3, .debs_badarg
    store.q r3, 440(r29)
    load.l  r3, 36(r1)
    bnez    r3, .debs_badarg
    load.l  r20, 32(r1)                ; phoff
    store.q r20, 504(r29)
    load.l  r3, 52(r1)
    and     r3, r3, #0xFFFF
    move.l  r4, #64
    bne     r3, r4, .debs_badarg
    load.l  r3, 54(r1)
    and     r3, r3, #0xFFFF
    move.l  r4, #56
    bne     r3, r4, .debs_badarg
    load.l  r21, 56(r1)
    and     r21, r21, #0xFFFF          ; phnum
    beqz    r21, .debs_badarg
    store.q r21, 528(r29)
    move.l  r3, #56
    mulu    r22, r21, r3
    add     r23, r20, r22
    blt     r23, r20, .debs_badarg
    load.q  r4, 368(r29)
    blt     r4, r23, .debs_badarg

    move.l  r3, #2
    store.q r3, 472(r29)
    push    r29
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .debs_nomem_noobj
    store.q r1, 376(r29)
    move.q  r26, r1
    move.l  r3, #DOS_SEGLIST_MAGIC
    store.l r3, DOS_SEGLIST_OFF_MAGIC(r26)
    store.l r0, DOS_SEGLIST_OFF_COUNT(r26)
    load.q  r3, 440(r29)
    store.q r3, DOS_SEGLIST_OFF_ENTRY(r26)
    load.q  r20, 504(r29)
    load.q  r21, 528(r29)
    load.q  r5, 360(r29)
    add     r24, r5, r20
    store.q r24, 520(r29)

    move.l  r18, #0
.debs_ph_loop:
    bge     r18, r21, .debs_done_parse
    load.q  r24, 520(r29)              ; ph ptr

    load.l  r3, (r24)
    lsl     r3, r3, #32
    lsr     r3, r3, #32
    move.q  r4, r0
    add     r4, r4, #1
    bne     r3, r4, .debs_badarg_free
    load.l  r8, 4(r24)                 ; flags
    lsl     r8, r8, #32
    lsr     r8, r8, #32
    move.q  r9, r8
    and     r9, r9, #0xFFFFFFF8
    bnez    r9, .debs_badarg_free
    beqz    r8, .debs_badarg_free
    move.q  r9, r8
    move.q  r10, r0
    add     r10, r10, #2
    beq     r9, r10, .debs_badarg_free
    move.q  r9, r8
    and     r9, r9, #3
    move.q  r10, r0
    add     r10, r10, #3
    beq     r9, r10, .debs_badarg_free

    load.l  r3, 12(r24)                ; off hi
    bnez    r3, .debs_badarg_free
    load.l  r3, 20(r24)                ; vaddr hi
    bnez    r3, .debs_badarg_free
    load.l  r3, 36(r24)                ; filesz hi
    bnez    r3, .debs_badarg_free
    load.l  r3, 44(r24)                ; memsz hi
    bnez    r3, .debs_badarg_free
    load.l  r3, 52(r24)                ; align hi
    bnez    r3, .debs_badarg_free

    load.l  r11, 8(r24)                ; file offset
    lsl     r11, r11, #32
    lsr     r11, r11, #32
    load.l  r6, 16(r24)                ; target vaddr
    lsl     r6, r6, #32
    lsr     r6, r6, #32
    load.l  r5, 32(r24)                ; filesz
    lsl     r5, r5, #32
    lsr     r5, r5, #32
    load.l  r7, 40(r24)                ; memsz
    lsl     r7, r7, #32
    lsr     r7, r7, #32
    load.l  r3, 48(r24)                ; align low
    lsl     r3, r3, #32
    lsr     r3, r3, #32
    move.l  r4, #4096
    bne     r3, r4, .debs_badarg_free
    beqz    r7, .debs_badarg_free
    and     r3, r11, #0xFFF
    and     r4, r6, #0xFFF
    bne     r3, r4, .debs_badarg_free
    and     r3, r6, #0xFFF
    bnez    r3, .debs_badarg_free
    blt     r7, r5, .debs_badarg_free

    add     r9, r11, r5
    blt     r9, r11, .debs_badarg_free
    load.q  r10, 368(r29)
    blt     r10, r9, .debs_badarg_free

    add     r9, r6, r7
    blt     r9, r6, .debs_badarg_free
    move.l  r10, #USER_CODE_BASE
    blt     r6, r10, .debs_badarg_free
    move.l  r10, #0x02000000
    blt     r10, r9, .debs_badarg_free

    ; overlap check against earlier segments already stored in seglist
    load.l  r12, DOS_SEGLIST_OFF_COUNT(r26)
    move.l  r13, #0
.debs_overlap_loop:
    bge     r13, r12, .debs_overlap_done
    move.l  r3, #DOS_SEG_ENTRY_SZ
    mulu    r4, r13, r3
    add     r4, r4, #DOS_SEGLIST_HDR_SZ
    add     r4, r4, r26
    load.l  r14, DOS_SEG_OFF_TARGET(r4)
    load.l  r15, DOS_SEG_OFF_MEMSZ(r4)
    add     r16, r14, r15
    blt     r6, r16, .debs_chk_other
    bra     .debs_ov_next
.debs_chk_other:
    blt     r14, r9, .debs_badarg_free
.debs_ov_next:
    add     r13, r13, #1
    bra     .debs_overlap_loop
.debs_overlap_done:
    load.q  r3, 440(r29)
    blt     r3, r6, .debs_entry_skip
    blt     r3, r9, .debs_entry_maybe
    bra     .debs_entry_skip
.debs_entry_maybe:
    and     r4, r8, #1
    beqz    r4, .debs_badarg_free
    move.l  r4, #1
    store.q r4, 464(r29)
.debs_entry_skip:

    add     r3, r7, #4095
    move.l  r4, #4096
    divu    r17, r3, r4                ; pages
    store.q r6, 384(r29)
    store.q r5, 392(r29)
    store.q r7, 400(r29)
    store.q r11, 408(r29)
    store.q r8, 416(r29)
    store.q r17, 424(r29)

    move.q  r1, r17
    lsl     r1, r1, #12
    push    r29
    push    r18
    push    r20
    push    r21
    push    r24
    push    r26
    move.q  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r26
    pop     r24
    pop     r21
    pop     r20
    pop     r18
    pop     r29
    bnez    r2, .debs_nomem_free
    move.q  r12, r1                    ; seg mem VA

    load.q  r13, 360(r29)
    load.q  r14, 408(r29)
    add     r13, r13, r14              ; src = file + offset
    move.q  r15, r12                   ; dst = seg mem
    load.q  r16, 392(r29)              ; filesz
.debs_cp_loop:
    beqz    r16, .debs_store_entry
    load.b  r3, (r13)
    store.b r3, (r15)
    add     r13, r13, #1
    add     r15, r15, #1
    sub     r16, r16, #1
    bra     .debs_cp_loop

.debs_store_entry:
    move.q  r3, r18
    add     r3, r3, #20
    store.q r3, 472(r29)
    load.l  r3, DOS_SEGLIST_OFF_COUNT(r26)
    move.l  r4, #DOS_SEGLIST_MAX_ENTRIES
    bge     r3, r4, .debs_badarg_free
    move.l  r4, #DOS_SEG_ENTRY_SZ
    mulu    r4, r3, r4
    add     r4, r4, #DOS_SEGLIST_HDR_SZ
    add     r4, r4, r26
    store.q r12, DOS_SEG_OFF_MEMVA(r4)
    load.q  r5, 384(r29)
    store.q r5, DOS_SEG_OFF_TARGET(r4)
    load.q  r5, 392(r29)
    store.q r5, DOS_SEG_OFF_FILESZ(r4)
    load.q  r5, 400(r29)
    store.q r5, DOS_SEG_OFF_MEMSZ(r4)
    load.q  r5, 424(r29)
    store.l r5, DOS_SEG_OFF_PAGES(r4)
    load.q  r5, 416(r29)
    store.l r5, DOS_SEG_OFF_FLAGS(r4)
    add     r3, r3, #1
    store.l r3, DOS_SEGLIST_OFF_COUNT(r26)
    add     r18, r18, #1
    add     r24, r24, #56
    store.q r24, 520(r29)
    bra     .debs_ph_loop

.debs_done_parse:
    load.q  r3, 464(r29)
    beqz    r3, .debs_badarg_free
    load.q  r1, 376(r29)
    load.q  r3, 176(r29)
    store.q r3, DOS_SEGLIST_NEXT(r1)
    store.q r1, 176(r29)
    move.q  r2, r0
    rts

.debs_nomem_noobj:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    rts
.debs_nomem_free:
    load.q  r1, 376(r29)
    jsr     .dos_seglist_free_unlinked
    move.q  r1, r0
    move.l  r2, #DOS_ERR_NOMEM
    rts
.debs_badarg_free:
    load.q  r1, 376(r29)
    jsr     .dos_seglist_free_unlinked
.debs_badarg:
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    rts

prog_doslib_code_end:

prog_doslib_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 16 bytes ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: banner "dos.library M14 [Task \0" ---
    dc.b    "dos.library M14 [Task ", 0
    ds.b    7                           ; pad to offset 64
    ; --- Offset 64: padding to 128 ---
    ds.b    64
    ; --- Offset 128: task_id (8) ---
    ds.b    8
    ; --- Offset 136: console_port (8) ---
    ds.b    8
    ; --- Offset 144: dos_port (8) ---
    ds.b    8
    ; --- Offset 152: meta_chain_head_va (8) — M12.6 Phase A ---
    ds.b    8
    ; --- Offset 160: hnd_chain_head_va  (8) — M12.6 Phase A ---
    ds.b    8
    ; --- Offset 168: caller_mapped_va (8) ---
    ds.b    8
    ; --- Offset 176: reserved (was open_handles[8] before M12.6 Phase A) ---
    ds.b    8
    ; --- Offset 184: cached share_pages (8) ---
    ds.b    8
    ; --- Offset 192..895: dead-space scratch (was: file_table 16×44 before M12.6 Phase A) ---
    ; Boot-time seeding helpers use 192..223 as save slots.
    ds.b    704
prog_doslib_seed_readme_name:
    ; --- Offset 896: pre-create filename "readme\0" + pad to 16 ---
    dc.b    "readme", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
prog_doslib_seed_readme_body:
    ; --- Offset 912: pre-create content ---
    dc.b    "Intuition OS", 0x0D, 0x0A, 0
    ; --- Offset 941: pad 1 byte ---
    ds.b    1
    ; --- Offset 942: seed file names ---
prog_doslib_seed_name_version:
    dc.b    "C/Version", 0              ; 942 (10 bytes)
prog_doslib_seed_name_avail:
    dc.b    "C/Avail", 0                ; 952 (8 bytes)
prog_doslib_seed_name_dir:
    dc.b    "C/Dir", 0                  ; 960 (6 bytes)
prog_doslib_seed_name_type:
    dc.b    "C/Type", 0                 ; 966 (7 bytes)
prog_doslib_seed_name_echo:
    dc.b    "C/Echo", 0                 ; 973 (7 bytes)
prog_doslib_seed_name_startup:
    dc.b    "S/Startup-Sequence", 0     ; 980 (19 bytes) → 999
    ds.b    1                           ; pad to 1000
    ; --- Offset 1000: name resolution scratch buffer (32 bytes) ---
    ds.b    32
    ; --- Offset 1032+: seed names ---
prog_doslib_seed_name_assign:
    dc.b    "C/Assign", 0
prog_doslib_seed_name_list:
    dc.b    "C/List", 0
prog_doslib_seed_name_which:
    dc.b    "C/Which", 0
prog_doslib_seed_name_help_cmd:
    dc.b    "C/Help", 0
prog_doslib_seed_name_resident:
    dc.b    "C/Resident", 0
prog_doslib_seed_name_help_text:
    dc.b    "S/Help", 0
prog_doslib_seed_name_loader_info:
    dc.b    "L/Loader-Info", 0
prog_doslib_seed_name_input:
    dc.b    "DEVS/input.device", 0
prog_doslib_seed_name_graphics:
    dc.b    "LIBS/graphics.library", 0
prog_doslib_seed_name_gfxdemo:
    dc.b    "C/GfxDemo", 0
    ; --- M12 seed names ---
prog_doslib_seed_name_intuition:
    dc.b    "LIBS/intuition.library", 0
prog_doslib_seed_name_about:
    dc.b    "C/About", 0
    ; --- M12.5 seed names ---
prog_doslib_seed_name_hwres:
    dc.b    "RESOURCES/hardware.resource", 0
    ; --- M14 Phase 2 seed names ---
prog_doslib_seed_name_elfseg:
    dc.b    "C/ElfSeg", 0
prog_doslib_boot_shell_path:
    dc.b    "IOSSYS:Tools/Shell", 0
prog_doslib_boot_shell_resolved:
    dc.b    "SYS/IOSSYS/Tools/Shell", 0
prog_doslib_boot_shell_relpath:
    dc.b    "IOSSYS/Tools/Shell", 0
prog_doslib_empty_args:
    dc.b    0
    align   8
    ; Static assign table for M15.2 phase-1 resolver. Entry layout:
    ;   [0..15]  assign name (NUL-terminated, uppercase canonical)
    ;   [16..31] target prefix (NUL-terminated, empty for RAM:)
prog_doslib_assign_default_c:
    dc.b    "C", 0
prog_doslib_assign_builtin_sys_row:
    dc.b    "SYS", 0
    ds.b    12
    dc.b    "SYS/", 0
    ds.b    11
prog_doslib_assign_builtin_iossys_query_row:
    dc.b    "IOSSYS", 0
    ds.b    9
    dc.b    "SYS:IOSSYS/", 0
    ds.b    4
prog_doslib_assign_builtin_iossys_row:
    dc.b    "IOSSYS", 0
    ds.b    9
    dc.b    "SYS/IOSSYS/", 0
    ds.b    4
prog_doslib_hostfs_public_sys:
    dc.b    "SYS/", 0
prog_doslib_hostfs_public_c:
    dc.b    "C/", 0
prog_doslib_hostfs_public_s:
    dc.b    "S/", 0
prog_doslib_hostfs_public_l:
    dc.b    "L/", 0
prog_doslib_hostfs_public_libs:
    dc.b    "LIBS/", 0
prog_doslib_hostfs_public_devs:
    dc.b    "DEVS/", 0
prog_doslib_hostfs_public_resources:
    dc.b    "RESOURCES/", 0
prog_doslib_hostfs_rel_c:
    dc.b    "IOSSYS/C/", 0
prog_doslib_hostfs_rel_s:
    dc.b    "IOSSYS/S/", 0
prog_doslib_hostfs_rel_l:
    dc.b    "IOSSYS/L/", 0
prog_doslib_hostfs_rel_libs:
    dc.b    "IOSSYS/LIBS/", 0
prog_doslib_hostfs_rel_devs:
    dc.b    "IOSSYS/DEVS/", 0
prog_doslib_hostfs_rel_resources:
    dc.b    "IOSSYS/RESOURCES/", 0
prog_doslib_assign_table:
    dc.b    "RAM", 0
    ds.b    12
    dc.b    0
    ds.b    15
    dc.b    "C", 0
    ds.b    14
    dc.b    "C/", 0
    ds.b    13
    dc.b    "L", 0
    ds.b    14
    dc.b    "L/", 0
    ds.b    13
    dc.b    "LIBS", 0
    ds.b    11
    dc.b    "LIBS/", 0
    ds.b    10
    dc.b    "DEVS", 0
    ds.b    11
    dc.b    "DEVS/", 0
    ds.b    10
    dc.b    "T", 0
    ds.b    14
    dc.b    "T/", 0
    ds.b    13
    dc.b    "S", 0
    ds.b    14
    dc.b    "S/", 0
    ds.b    13
    dc.b    "RESOURCES", 0
    ds.b    6
    dc.b    "RESOURCES/", 0
    ds.b    5
    ds.b    (DOS_ASSIGN_ENTRY_SZ * (DOS_ASSIGN_TABLE_COUNT - DOS_ASSIGN_DEFAULT_COUNT))

    ; M15.3 mutable overlay storage. One DOS_ASSIGN_OVERLAY_ENTRY_SZ block per
    ; assign-table slot (only canonical layered slots are ever touched, but we
    ; allocate symmetric storage so the slot index can be used as a direct
    ; multiplier). Per-slot layout:
    ;   [0]   overlay_count (byte, 0..DOS_ASSIGN_OVERLAY_MAX)
    ;   [1..7] padding
    ;   [8..71] DOS_ASSIGN_OVERLAY_MAX × DOS_ASSIGN_OVERLAY_TGT_SZ targets
prog_doslib_assign_overlay:
    ds.b    (DOS_ASSIGN_OVERLAY_ENTRY_SZ * DOS_ASSIGN_TABLE_COUNT)

    ; M15.3 base list target pointer table. Per canonical layered slot, two
    ; pointers (writable SYS overlay path, then read-only IOSSYS path), each a
    ; relative offset into prog_doslib_data. Non-layered slots have zeros and
    ; are never read. Indexed by slot * 16 (two 8-byte offsets per slot).
prog_doslib_assign_base_table:
    ; slot 0: RAM (not layered)
    dc.q    0
    dc.q    0
    ; slot 1: C
    dc.q    (prog_doslib_hostfs_public_c - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_c - prog_doslib_data)
    ; slot 2: L
    dc.q    (prog_doslib_hostfs_public_l - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_l - prog_doslib_data)
    ; slot 3: LIBS
    dc.q    (prog_doslib_hostfs_public_libs - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_libs - prog_doslib_data)
    ; slot 4: DEVS
    dc.q    (prog_doslib_hostfs_public_devs - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_devs - prog_doslib_data)
    ; slot 5: T (not layered)
    dc.q    0
    dc.q    0
    ; slot 6: S
    dc.q    (prog_doslib_hostfs_public_s - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_s - prog_doslib_data)
    ; slot 7: RESOURCES
    dc.q    (prog_doslib_hostfs_public_resources - prog_doslib_data)
    dc.q    (prog_doslib_hostfs_rel_resources - prog_doslib_data)

    align   8

; ---------------------------------------------------------------------------
; Per-command source files under `../cmd/`.
; ---------------------------------------------------------------------------

include "../cmd/version.s"
include "../cmd/avail.s"
include "../cmd/dir.s"
include "../cmd/type.s"
include "../cmd/echo.s"
include "../cmd/help.s"
include "../cmd/resident.s"
include "../cmd/list.s"
include "../cmd/which.s"
include "../cmd/assign.s"

    align   8

; ---------------------------------------------------------------------------
; input.device — keyboard/mouse event service (M11)
; ---------------------------------------------------------------------------
; Polls SCAN_*/MOUSE_* registers (mapped via SYS_MAP_IO with the M11
; range-aware extension), pushes INPUT_EVENT messages to a single registered
; subscriber port. Single subscriber for M11; multi-subscriber fan-out is M12
; work in intuition.library.
;
; Protocol: see iexec.inc INPUT_OPEN / INPUT_CLOSE / INPUT_EVENT.
; Data layout:
;   0:   "console.handler\0"  (16 bytes — unused, kept for standard layout)
;   16:  "input.device\0"     (16 bytes, padded — port name)
;   32:  banner string        (32 bytes — "input.device ONLINE [Task ")
;   64:  padding              (64 bytes)
;   128: task_id              (8 bytes)
;   136: (unused)             (8 bytes)
;   144: input_port           (8 bytes — own port_id)
;   152: chip_mmio_va         (8 bytes — from SYS_MAP_IO)
;   160: subscriber_port      (8 bytes, 0 = none)
;   168: last_mouse_x         (4 bytes)
;   172: last_mouse_y         (4 bytes)
;   176: last_mouse_buttons   (4 bytes)
;   180: event_seq            (4 bytes — monotonic event counter)
;   184: padding              (8 bytes)
include "../dev/input_device.s"

; ---------------------------------------------------------------------------
; hardware.resource — user-space MMIO arbiter (M12.5)
; ---------------------------------------------------------------------------
; The first user-space service to claim broker identity via SYS_HWRES_OP /
; HWRES_BECOME. Owns the policy mapping from 4-byte region tags ('CHIP',
; 'VRAM') to physical PPN ranges. Clients send HWRES_MSG_REQUEST naming a
; tag; the broker resolves the tag, calls
; SYS_HWRES_OP / HWRES_CREATE to write a grant row covering the right PPN
; range for the requesting task, and replies with HWRES_MSG_GRANTED whose
; data0 carries (ppn_base<<32) | page_count so the client can call
; SYS_MAP_IO with values it learned from the broker (no PPN literals
; baked into clients).
;
; Data layout (offsets relative to data page):
;   0..31:  port name "hardware.resource\0..." (32 bytes; PORT_NAME_LEN=32)
;   32..95: banner "hardware.resource ONLINE [Task " (variable, padded)
;   96..127: pad
;   128:    task_id (8 bytes)
;   136:    hwres_port (8 bytes)
;   144:    pad

include "../resource/hardware_resource.s"

; ---------------------------------------------------------------------------
; graphics.library — fullscreen RGBA32 display service (M11, M12: 800x600)
; ---------------------------------------------------------------------------
; Maps the chip register page (0xF0) and the 800x600x4 VRAM range
; (PPNs 0x100..0x2D5 = 470 pages = 1925120 bytes), creates the
; "graphics.library" port, then services requests synchronously.
;
; M12: bumped from 640x480 to 800x600 to match the chip's DEFAULT_VIDEO_MODE
; and give clients more screen real estate. The chip is left in mode 1 the
; whole time (the chip's DEFAULT_VIDEO_MODE = MODE_800x600), so a kernel-side
; VideoTerminal that started in 800x600 keeps the same framebuffer dimensions.
; The protocol still allows clients to request other modes — graphics.library
; just defaults to 800x600 in M12 because no other mode is enumerated yet.
;
; Single surface only (USER_DYN_PAGES=768 doesn't fit two persistent surface
; mappings + persistent VRAM). Client double-buffering remains deferred.
;
; Protocol: see iexec.inc GFX_* constants.
include "graphics_library.s"

; ---------------------------------------------------------------------------
; C/GfxDemo — minimal graphics.library client (M11)
; ---------------------------------------------------------------------------
; Opens graphics.library + input.device, allocates a 640x480 RGBA32 surface,
; registers it, fills with a solid color, presents once, then waits for
; Escape (scancode 0x01) and exits cleanly.
include "../cmd/gfxdemo.s"

; ---------------------------------------------------------------------------
; intuition.library — single-window compositor + IDCMP delivery (M12)
; ---------------------------------------------------------------------------
; intuition.library is the sole graphics.library client. On the FIRST
; INTUITION_OPEN_WINDOW it lazily opens the display, allocates a fullscreen
; screen surface, registers it with graphics.library, and subscribes to
; input.device. From then on it composites the (single) window's backing
; surface into the screen surface and routes input as IDCMP-* messages to
; the window's idcmp_port. On CLOSE_WINDOW it tears down all of the above
; and returns to text mode.
;
; Protocol: see iexec.inc INTUITION_* / IDCMP_* constants.
; M12 ships single-window only — no z-order, no compositor overlap.
;
; Data layout:
;   0:    "console.handler\0"  (16) — convention slot, unused
;   16:   "intuition.library"  (16, exactly 16, NO null) — port name
;   32:   "graphics.library"   (16, exactly 16, NO null) — for FindPort
;   48:   "input.device", 0x00 (16) — for FindPort
;   64:   "intuition.library ONLINE [Task " + null (32) → ends at 96
;   96:   pad to 128 (32)
;   128:  task_id              (8)
;   136:  intuition_port       (8)  — public port
;   144:  graphics_port        (8)  — cached after first FindPort
;   152:  input_port           (8)  — cached after first FindPort
;   160:  reply_port           (8)  — anonymous, sync replies
;   168:  my_input_port        (8)  — anonymous, receives input events
;   176:  display_open         (1)  — 0 = text mode, 1 = graphics mode
;   177:  input_subscribed     (1)  — 1 if our INPUT_OPEN succeeded; close
;                                     skips INPUT_CLOSE if 0 so we don't
;                                     clobber another client's subscription
;   178..183: pad                  (6)
;   184:  display_handle       (8)  — graphics.library display handle
;   192:  surface_handle       (8)  — graphics.library surface handle
;   200:  screen_va            (8)  — own MEMF_PUBLIC screen buffer VA
;   208:  screen_share         (8)  — (4) own surface share handle + pad
;   216:  win_in_use           (8)  — (1) + pad (1=window open)
;   224:  win_x                (4)  — window origin x (signed)
;   228:  win_y                (4)  — window origin y (signed)
;   232:  win_w                (4)  — window width
;   236:  win_h                (4)  — window height
;   240:  win_share            (8)  — (4) app buffer share handle + pad
;   248:  win_mapped_va        (8)  — MapShared'd app buffer VA
;   256:  idcmp_port           (8)  — owner's IDCMP delivery port
;   264:  event_seq            (8)  — (4) monotonic seq + pad
;   272:  msg_type             (8)  — saved message fields (scratch)
;   280:  msg_data0            (8)
;   288:  msg_data1            (8)
;   296:  msg_reply            (8)
;   304:  msg_share            (8)
;   312:  pad                  (8)
include "intuition_library.s"

; ---------------------------------------------------------------------------
; About — intuition.library client with text rendering (M12)
; ---------------------------------------------------------------------------
; Allocates a 320x200 RGBA32 backing surface, opens an intuition.library
; window centered on the 800x600 screen, fills the content area with a
; teal backdrop, draws several lines of "About IntuitionOS" text using
; the embedded Topaz 8x16 bitmap font, sends DAMAGE, then waits on its
; IDCMP port. On IDCMP_CLOSEWINDOW (Esc key OR click on close gadget)
; it sends INTUITION_CLOSE_WINDOW and exits.
;
; Data layout:
;   0..127:  pad
;   128:  task_id              (8)
;   136:  intuition_port       (8)
;   144:  reply_port           (8)
;   152:  idcmp_port           (8)
;   160:  surface_va           (8)
;   168:  surface_share        (8) — (4) + pad
;   176:  window_handle        (8)
;   184:  pad
;   192:  "intuition.library"  (32, port name for FindPort)
;   224:  "About M12 ready"    (test marker)
;   256:  about text strings (each null-terminated)
;   ...
;   1024: topaz font (4096 bytes, full 256 chars × 16 bytes)
include "../cmd/about.s"

; ---------------------------------------------------------------------------
; Embedded ELF fixture for M14 Phase 2 LoadSeg tests
; ---------------------------------------------------------------------------
; Strict phase-1 subset:
;   ELF64, little-endian, ET_EXEC, EM_IE64, 2 PT_LOAD segments
;   seg0: RX @ 0x00601000, file bytes 0x11 0x22 0x33 0x44
;   seg1: RW @ 0x00602000, file bytes 0x55 0x66 0x77 0x88

include "../assets/elfseg_fixture.s"
    align   8

prog_doslib_pmp_desc_dst:
    ds.b    IOSM_SIZE

; ---------------------------------------------------------------------------
; SHELL — interactive command shell (M10)
; ---------------------------------------------------------------------------
; Opens console.handler and dos.library via OpenLibrary.
; Reads lines from console, parses command, launches external programs via
; DOS_RUN, or prints "Unknown command\r\n".
;
; Data page layout:
;   0:   "console.handler\0"   (16 bytes)
;   16:  "dos.library\0"       (16 bytes, padded)
;   32:  "Shell ONLINE [Task\0"   (16 bytes, banner prefix)
;   48:  "IntuitionOS M9\r\n\0" (17 bytes + pad = 32 bytes)
;   80:  "1> \0"               (4 bytes + pad = 8 bytes)
;   88:  "Unknown command\r\n\0" (18 bytes + pad)
;   128: task_id               (8 bytes)
;   136: console_port          (8 bytes)
;   144: dos_port              (8 bytes)
;   152: reply_port            (8 bytes)
;   160: shared_buf_va         (8 bytes)
;   168: shared_buf_handle     (4 bytes + pad)
;   192: command name table    (5 x 8 bytes = 40 bytes)
;   232: command index table   (5 bytes)
;   240: line buffer           (128 bytes)

    align   8
prog_doslib_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_LIBRARY
    dc.b    0
    dc.w    14
    dc.w    0
    dc.w    0
    dc.b    "dos.library", 0
    ds.b    IOSM_NAME_SIZE - 12
    dc.l    MODF_COMPAT_PORT | MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8

include "../handler/shell.s"

prog_doslib_data_end:
    align   8
prog_doslib_end:
