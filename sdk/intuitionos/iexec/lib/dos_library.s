prog_doslib:
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
    bra     .dos_send_banner
.dos_openlib_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_openlib_retry

    ; =====================================================================
    ; Send "dos.library ONLINE [Taskn]\r\n" banner to console.handler
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
    move.l  r2, #MEMF_CLEAR
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
    ; Allocate the readme body via the extent allocator (28 bytes).
    move.l  r1, #28
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
    ; entry.size = 28 (length of welcome message)
    move.l  r14, #28
    store.l r14, DOS_META_OFF_SIZE(r25)

    ; Copy welcome message from data[912] into the extent chain payload.
    move.q  r1, r24                    ; r1 = first extent VA
    add     r2, r29, #(prog_doslib_seed_readme_body - prog_doslib_data)
    move.l  r3, #28                    ; r3 = byte_count
    jsr     .dos_extent_write
    load.q  r29, (sp)
.dos_init_done:

    ; =====================================================================
    ; Seed RAM: with canonical ELF files and plain-text assets (M14.2)
    ; =====================================================================
    ; Command/demo files come from bundled ELF blobs. Service files come from
    ; the kernel-exported boot-manifest ELF rows already mapped into
    ; dos.library. Startup-Sequence remains trusted plain text.
    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_version - prog_doslib_data)
    add     r24, r29, #(seed_elf_version - prog_doslib_data)
    move.l  r23, #(seed_elf_version_end - seed_elf_version)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_avail - prog_doslib_data)
    add     r24, r29, #(seed_elf_avail - prog_doslib_data)
    move.l  r23, #(seed_elf_avail_end - seed_elf_avail)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_dir - prog_doslib_data)
    add     r24, r29, #(seed_elf_dir - prog_doslib_data)
    move.l  r23, #(seed_elf_dir_end - seed_elf_dir)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_type - prog_doslib_data)
    add     r24, r29, #(seed_elf_type - prog_doslib_data)
    move.l  r23, #(seed_elf_type_end - seed_elf_type)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_echo - prog_doslib_data)
    add     r24, r29, #(seed_elf_echo - prog_doslib_data)
    move.l  r23, #(seed_elf_echo_end - seed_elf_echo)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_assign - prog_doslib_data)
    add     r24, r29, #(seed_elf_assign - prog_doslib_data)
    move.l  r23, #(seed_elf_assign_end - seed_elf_assign)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_list - prog_doslib_data)
    add     r24, r29, #(seed_elf_list - prog_doslib_data)
    move.l  r23, #(seed_elf_list_end - seed_elf_list)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_which - prog_doslib_data)
    add     r24, r29, #(seed_elf_which - prog_doslib_data)
    move.l  r23, #(seed_elf_which_end - seed_elf_which)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_help_cmd - prog_doslib_data)
    add     r24, r29, #(seed_elf_help - prog_doslib_data)
    move.l  r23, #(seed_elf_help_end - seed_elf_help)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_startup - prog_doslib_data)
    add     r24, r29, #(seed_startup - prog_doslib_data)
    move.l  r23, #(seed_startup_end - seed_startup - 1)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_help_text - prog_doslib_data)
    add     r24, r29, #(seed_help_text - prog_doslib_data)
    move.l  r23, #(seed_help_text_end - seed_help_text - 1)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_loader_info - prog_doslib_data)
    add     r24, r29, #(seed_loader_info - prog_doslib_data)
    move.l  r23, #(seed_loader_info_end - seed_loader_info - 1)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_input - prog_doslib_data)
    move.l  r21, #BOOT_MANIFEST_ID_INPUT
    jsr     .dos_seed_boot_export

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_hwres - prog_doslib_data)
    move.l  r21, #BOOT_MANIFEST_ID_HWRES
    jsr     .dos_seed_boot_export

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_graphics - prog_doslib_data)
    move.l  r21, #BOOT_MANIFEST_ID_GRAPHICS
    jsr     .dos_seed_boot_export

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_gfxdemo - prog_doslib_data)
    add     r24, r29, #(seed_elf_gfxdemo - prog_doslib_data)
    move.l  r23, #(seed_elf_gfxdemo_end - seed_elf_gfxdemo)
    jsr     .dos_seed_known

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_intuition - prog_doslib_data)
    move.l  r21, #BOOT_MANIFEST_ID_INTUITION
    jsr     .dos_seed_boot_export

    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_about - prog_doslib_data)
    add     r24, r29, #(seed_elf_about - prog_doslib_data)
    move.l  r23, #(seed_elf_about_end - seed_elf_about)
    jsr     .dos_seed_known

    ; Seed C/ElfSeg (slot 13) — native ELF fixture
    load.q  r29, (sp)
    add     r20, r29, #(prog_doslib_seed_name_elfseg - prog_doslib_data)
    add     r24, r29, #(prog_elfseg - prog_doslib_data)
    move.l  r23, #0x2004
    jsr     .dos_seed_known
    bra     .dos_seed_done

    ; -----------------------------------------------------------------
    ; .dos_seed_boot_export:
    ; Seed one file from the boot-manifest export table populated by the
    ; kernel for dos.library.
    ; Input:  r20 = name_ptr, r21 = boot-manifest ID, r29 = data base
    ; -----------------------------------------------------------------
.dos_seed_boot_export:
    store.q r20, 192(r29)
    move.q  r1, r21
    jsr     .dos_boot_export_find_row_by_id
    beqz    r2, .dsbe_done
    load.q  r24, DOS_BOOT_EXPORT_PTR(r1)
    load.q  r23, DOS_BOOT_EXPORT_SIZE(r1)
    beqz    r24, .dsbe_done
    beqz    r23, .dsbe_done
    load.q  r20, 192(r29)
    jsr     .dos_seed_known
.dsbe_done:
    rts

    ; -----------------------------------------------------------------
    ; .dos_boot_export_find_row_by_id
    ; Input:  r1 = boot-manifest ID, r29 = dos data base
    ; Output: r1 = export row ptr (or 0), r2 = 1 if found, 0 otherwise
    ; -----------------------------------------------------------------
.dos_boot_export_find_row_by_id:
    move.q  r20, r1
    add     r21, r29, #(prog_doslib_boot_export_rows - prog_doslib_data)
    move.l  r22, #0
.dbefri_loop:
    move.l  r23, #DOS_BOOT_EXPORT_COUNT
    bge     r22, r23, .dbefri_notfound
    load.l  r24, DOS_BOOT_EXPORT_ID(r21)
    beq     r24, r20, .dbefri_found
    add     r21, r21, #DOS_BOOT_EXPORT_ROW_SZ
    add     r22, r22, #1
    bra     .dbefri_loop
.dbefri_found:
    move.q  r1, r21
    move.l  r2, #1
    rts
.dbefri_notfound:
    move.q  r1, r0
    move.q  r2, r0
    rts

    ; -----------------------------------------------------------------
    ; .dos_seed_known: seed one file from embedded bytes when the size is
    ; already known by the caller.
    ; Input:  r20 = name_ptr, r24 = image_ptr, r23 = byte_count, r29 = data
    ; Output: r24 advanced past image (aligned to 8)
    ; -----------------------------------------------------------------
.dos_seed_known:
    store.q r20, 192(r29)
    store.q r24, 200(r29)
    store.q r23, 216(r29)

    jsr     .dos_meta_alloc_entry
    bnez    r2, .dsk_done
    store.q r1, 208(r29)

    load.q  r20, 192(r29)
    load.q  r25, 208(r29)
    move.q  r16, r20
    move.q  r17, r25
    move.l  r18, #0
.dsk_cpname:
    load.b  r15, (r16)
    store.b r15, (r17)
    beqz    r15, .dsk_cpname_done
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #31
    blt     r18, r28, .dsk_cpname
    store.b r0, (r17)
.dsk_cpname_done:

    load.q  r1, 216(r29)
    jsr     .dos_extent_alloc
    bnez    r2, .dsk_done
    store.q r1, 224(r29)

    load.q  r25, 208(r29)
    load.q  r1, 224(r29)
    store.q r1, DOS_META_OFF_VA(r25)
    load.q  r23, 216(r29)
    store.l r23, DOS_META_OFF_SIZE(r25)

    load.q  r1, 224(r29)
    load.q  r2, 200(r29)
    load.q  r3, 216(r29)
    jsr     .dos_extent_write

    load.q  r24, 200(r29)
    load.q  r23, 216(r29)
    add     r24, r24, r23
    add     r24, r24, #7
    and     r24, r24, #0xFFFFFFF8
.dsk_done:
    rts

.dos_seed_done:

    ; =====================================================================
    ; NOW create the DOS port (after seeding = readiness signal)
    ; =====================================================================
    load.q  r29, (sp)
    add     r1, r29, #16               ; R1 = &data[16] = "dos.library"
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT           ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 144(r29)               ; data[144] = dos_port

    ; =====================================================================
    ; M14.1 phase 3: DOS now owns the remaining service boot chain.
    ; Launch Shell from the embedded manifest before entering the public
    ; request loop. Shell then drives Startup-Sequence, whose service-name
    ; lines are resolved through the internal embedded-manifest path.
    ; =====================================================================
    store.b r0, 1000(r29)              ; empty args string for manifest boot
    move.l  r1, #BOOT_MANIFEST_ID_SHELL
    add     r2, r29, #1000
    move.q  r3, r0
    jsr     .dos_manifest_launch_by_id
    load.q  r29, (sp)
    bnez    r2, .dos_boot_fail
    syscall #SYS_YIELD
    load.q  r29, (sp)

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
    ; Unknown opcode → reply with error and loop
    bra     .dos_reply_err

    ; =================================================================
    ; DOS_DIR (type=5): format directory listing into caller's buffer
    ; M12.6 Phase A: walks the metadata chain instead of a fixed file table.
    ; =================================================================
.dos_do_dir:
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
    beqz    r25, .dos_dir_done
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
.dos_dir_done:
    ; Null-terminate
    store.b r0, (r20)
    ; Reply with data0 = bytes written
    load.q  r1, 944(r29)               ; reply_port
    move.l  r2, #DOS_OK                 ; type = success
    move.q  r3, r21                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_OPEN (type=1): open file by name from shared buffer
    ; M12.6 Phase A: walks the metadata chain via .dos_meta_find_by_name;
    ; allocates a new entry via .dos_meta_alloc_entry on write-mode miss;
    ; allocates a handle slot via .dos_hnd_alloc.
    ; =================================================================
    ; data0 = mode (READ=0, WRITE=1), filename in caller's shared buffer
.dos_do_open:
    load.q  r20, 960(r29)              ; r20 = mode (0=READ, 1=WRITE)
    load.q  r23, 168(r29)              ; r23 = mapped VA (filename pointer)

    ; Resolve filename through the DOS assign table.
    jsr     .dos_resolve_file
    load.q  r29, (sp)
    beqz    r22, .dos_reply_err
    load.q  r20, 960(r29)              ; resolver clobbers caller-save regs
    store.q r23, 336(r29)              ; preserve resolved name across helper JSRs

    ; --- Search metadata chain for matching name ---
    move.q  r1, r23                     ; r1 = request name ptr
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    load.q  r29, (sp)
    load.q  r23, 336(r29)
    bnez    r1, .dos_open_have_entry
    ; Not found
    bnez    r20, .dos_open_create       ; WRITE → create new
    bra     .dos_reply_err              ; READ → error

.dos_open_create:
    ; M12.8 Phase 2: write-mode create no longer pre-allocates a body.
    ; entry.file_va starts as 0 (empty file). The first DOS_WRITE will
    ; allocate an extent chain via .dos_extent_alloc and link it in.
    ; Allocate a fresh metadata entry.
    jsr     .dos_meta_alloc_entry       ; r1 = entry VA, r2 = err
    bnez    r2, .dos_reply_full
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
    jsr     .dos_assign_find_entry
    beqz    r3, .dos_reply_err
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
    move.l  r24, #DOS_ASSIGN_DEFAULT_COUNT
    blt     r2, r24, .dos_reply_badarg
    move.q  r25, r1
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
    bge     r20, r22, .dos_assign_set_done
    add     r24, r21, r20
    load.b  r28, (r24)
    add     r24, r25, #DOS_ASSIGN_TARGET_OFF
    add     r24, r24, r20
    store.b r28, (r24)
    add     r20, r20, #1
    bra     .dos_assign_set_target_copy
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
    ; DOS_LOADSEG (type=7): load an ELF file into a DOS-owned seglist.
    ; Shared buffer contains the program name. Reply data0 = seglist VA.
    ; =================================================================
.dos_do_loadseg:
    load.q  r23, 168(r29)              ; name ptr in shared buffer
    jsr     .dos_resolve_cmd
    load.q  r29, (sp)
    beqz    r22, .dos_run_notfound

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
    jsr     .dos_elf_build_seglist
    load.q  r29, (sp)
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
    ;   96  launch descriptor (48 bytes)
    ;   144 launch seg table entry 0
    ;   176 launch seg table entry 1
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
    and     r12, r11, #4
    beqz    r12, .dos_launchseg_badarg
    and     r12, r11, #2
    bnez    r12, .dos_launchseg_scan_data
    bnez    r27, .dos_launchseg_scan_code_seen
    move.l  r27, #1
    store.q r7, 0(sp)
    store.q r10, 8(sp)
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
    beqz    r28, .dos_launchseg_badarg
    beqz    r25, .dos_launchseg_badarg

    load.q  r3, 8(sp)
    load.q  r4, 0(sp)
    sub     r3, r3, r4
    lsr     r3, r3, #12
    beqz    r3, .dos_launchseg_badarg
    store.q r3, 48(sp)
    load.q  r5, 24(sp)
    load.q  r6, 16(sp)
    sub     r5, r5, r6
    lsr     r5, r5, #12
    beqz    r5, .dos_launchseg_badarg
    store.q r5, 56(sp)

    move.q  r1, r3
    lsl     r1, r1, #12
    move.l  r2, #MEMF_CLEAR
    push    r29
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_launchseg_nomem
    store.q r1, 32(sp)

    move.q  r1, r5
    lsl     r1, r1, #12
    move.l  r2, #MEMF_CLEAR
    push    r29
    syscall #SYS_ALLOC_MEM
    pop     r29
    bnez    r2, .dos_launchseg_fail_free_code
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
    and     r12, r11, #2
    bnez    r12, .dos_launchseg_copy_data
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
    add     r24, sp, #96
    move.l  r3, #M14_LDESC_MAGIC
    store.l r3, M14_LDESC_OFF_MAGIC(r24)
    move.l  r3, #M14_LDESC_VERSION
    store.l r3, M14_LDESC_OFF_VERSION(r24)
    move.l  r3, #M14_LDESC_SIZE
    store.l r3, M14_LDESC_OFF_SIZE(r24)
    move.l  r3, #2
    store.l r3, M14_LDESC_OFF_SEGCNT(r24)
    load.q  r3, DOS_SEGLIST_OFF_ENTRY(r21)
    store.q r3, M14_LDESC_OFF_ENTRY(r24)
    move.l  r3, #1
    store.l r3, M14_LDESC_OFF_STACKPG(r24)
    add     r3, sp, #144
    store.q r3, M14_LDESC_OFF_SEGTBL(r24)

    add     r6, sp, #144
    load.q  r7, 32(sp)
    store.q r7, M14_LDSEG_OFF_SRCPTR(r6)
    load.q  r7, 48(sp)
    lsl     r7, r7, #12
    store.q r7, M14_LDSEG_OFF_SRCSZ(r6)
    load.q  r7, 0(sp)
    store.q r7, M14_LDSEG_OFF_TARGET(r6)
    load.q  r7, 48(sp)
    store.l r7, M14_LDSEG_OFF_PAGES(r6)
    move.l  r7, #5
    store.l r7, M14_LDSEG_OFF_FLAGS(r6)

    add     r6, sp, #176
    load.q  r7, 40(sp)
    store.q r7, M14_LDSEG_OFF_SRCPTR(r6)
    load.q  r7, 56(sp)
    lsl     r7, r7, #12
    store.q r7, M14_LDSEG_OFF_SRCSZ(r6)
    load.q  r7, 16(sp)
    store.q r7, M14_LDSEG_OFF_TARGET(r6)
    load.q  r7, 56(sp)
    store.l r7, M14_LDSEG_OFF_PAGES(r6)
    move.l  r7, #6
    store.l r7, M14_LDSEG_OFF_FLAGS(r6)

    add     r1, sp, #96
    move.l  r2, #M14_LDESC_SIZE
    load.q  r3, 72(sp)
    load.q  r4, 80(sp)
    push    r29
    syscall #SYS_EXEC_PROGRAM
    pop     r29
    move.q  r26, r1                    ; preserve task id
    move.q  r27, r2                    ; preserve exec error

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
    move.l  r5, #ERR_NOMEM
    beq     r27, r5, .dos_launchseg_nomem
.dos_launchseg_badarg:
    load.q  r19, 232(sp)
    load.q  r30, 224(sp)
    move.q  r1, r0
    move.l  r2, #DOS_ERR_BADARG
    add     sp, sp, #240
    rts
.dos_launchseg_fail_free_code:
    load.q  r1, 32(sp)
    beqz    r1, .dos_launchseg_nomem
    load.q  r2, 48(sp)
    lsl     r2, r2, #12
    push    r29
    syscall #SYS_FREE_MEM
    pop     r29
    bra     .dos_launchseg_nomem
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
    jsr     .dos_assign_name_eq_ci
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

    ; .dos_assign_lookup
    ; In:  r1 = input volume ptr, r2 = input volume len, r29 = data base
    ; Out: r1 = target ptr, r2 = target len, r3 = 1 if found, 0 otherwise
.dos_assign_lookup:
    jsr     .dos_assign_find_entry
    beqz    r3, .dal_notfound
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_target_len
    move.l  r3, #1
    rts
.dal_notfound:
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

    add     r22, r29, #208
    move.l  r26, #0
.davt_copy_name:
    sub     r24, r20, #1
    bge     r26, r24, .davt_lookup
    add     r21, r27, r26
    load.b  r24, (r21)
    move.l  r25, #0x61
    blt     r24, r25, .davt_upper_chk
    move.l  r25, #0x7B
    bge     r24, r25, .davt_upper_chk
    sub     r24, r24, #0x20
.davt_upper_chk:
    move.l  r25, #0x41
    blt     r24, r25, .davt_bad
    move.l  r25, #0x5B
    bge     r24, r25, .davt_bad
    add     r21, r22, r26
    store.b r24, (r21)
    add     r26, r26, #1
    bra     .davt_copy_name
.davt_lookup:
    add     r21, r22, r26
    store.b r0, (r21)
    move.q  r1, r22
    move.q  r2, r26
    jsr     .dos_assign_find_entry
    beqz    r3, .davt_bad
    move.q  r27, r2
    move.l  r24, #DOS_ASSIGN_DEFAULT_COUNT
    bge     r27, r24, .davt_bad
    add     r1, r1, #DOS_ASSIGN_TARGET_OFF
    jsr     .dos_assign_target_len
    move.l  r3, #1
    rts

.darn_bad:
    store.b r0, (r22)
    move.q  r1, r22
    move.q  r2, r0
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
    move.q  r27, r23
    move.q  r1, r23
    move.q  r2, r15
    jsr     .dos_assign_name_supported
    beqz    r3, .dos_resolve_notfound
    move.q  r1, r27
    move.q  r2, r15
    jsr     .dos_assign_lookup
    beqz    r3, .dos_resolve_notfound
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
.dos_assign_name_supported:
    jsr     .dos_assign_find_entry
    rts

    ; =================================================================
    ; DOS_RUN (type=6): launch program by name (M10)
    ; =================================================================
    ; Shared buffer format: "command_name\0args_string\0"
    ; Resolves command name through C: assign, finds image in file table,
    ; launches via SYS_EXEC_PROGRAM (new ABI: image_ptr, image_size).
.dos_do_run:
    ; 1. Read command name from mapped shared buffer
    load.q  r23, 168(r29)              ; r23 = caller's mapped buffer (name ptr)

    ; 2. Resolve through C: assign (r23 in/out)
    jsr     .dos_resolve_cmd            ; r23 = resolved name (e.g. "C/Version")
    load.q  r29, (sp)
    beqz    r22, .dos_run_reply_notfound

.dos_run_file_lookup:
    ; 3. M12.6 Phase A: walk metadata chain by name
    ; Preserve the resolved command-name pointer before helper calls:
    ; .dos_meta_find_by_name clobbers high registers, and the M14.1
    ; manifest-name fallback still needs the original resolved name on a
    ; miss. Without this, unknown commands can pass a null/stale pointer
    ; into .dos_manifest_find_row_by_name and fault dos.library.
    store.q r23, 336(r29)
    move.q  r1, r23
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    beqz    r1, .dos_run_notfound
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r29, (sp)
    bra     .dos_run_found

.dos_run_notfound:
    load.q  r29, (sp)
    load.q  r1, 336(r29)
    load.b  r11, (r1)
    move.l  r12, #0x44                 ; 'D'
    beq     r11, r12, .dos_run_manifest_maybe
    move.l  r12, #0x4C                 ; 'L'
    beq     r11, r12, .dos_run_manifest_maybe
    move.l  r12, #0x52                 ; 'R'
    bne     r11, r12, .dos_run_reply_notfound
.dos_run_manifest_maybe:
    jsr     .dos_manifest_find_row_by_name ; r1 = manifest entry ID (or 0), r2 = 1 if found
    load.q  r29, (sp)
    beqz    r1, .dos_run_reply_notfound
    beqz    r2, .dos_run_reply_notfound
    move.q  r21, r1                     ; manifest entry ID

    ; Shared buffer still holds "command\0args\0". Reuse the normal
    ; bounded args scan and launch through the manifest-backed path.
    load.q  r20, 168(r29)              ; original shared buffer
    move.q  r16, r20
    move.l  r24, #0
.dos_run_manifest_skip_cmd:
    load.b  r15, (r16)
    beqz    r15, .dos_run_manifest_args_start
    add     r16, r16, #1
    add     r24, r24, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r24, r28, .dos_run_manifest_skip_cmd
    bra     .dos_run_reply_notfound
.dos_run_manifest_args_start:
    add     r16, r16, #1
    move.q  r17, r16
    move.l  r18, #0
.dos_run_manifest_arglen:
    load.b  r15, (r17)
    beqz    r15, .dos_run_launch_manifest_raw
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r18, r28, .dos_run_manifest_arglen
    bra     .dos_run_reply_notfound

.dos_run_launch_manifest_raw:
    move.q  r1, r21                    ; manifest entry ID
    move.q  r2, r16                    ; args_ptr
    move.q  r3, r18                    ; args_len
    jsr     .dos_manifest_launch_by_id
    store.q r1, 304(r29)               ; task_id
    store.q r2, 312(r29)               ; DOS err
    load.q  r1, 944(r29)
    load.q  r2, 312(r29)
    load.q  r3, 304(r29)
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_reply_notfound:
    load.q  r29, (sp)
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
    ; Prefer the M14 native path for ELF files. M14.2 phase 1 rejects legacy
    ; flat-image IE64PROG executables instead of falling back to SYS_EXEC_PROGRAM.
    load.l  r15, (r21)
    move.l  r28, #0x464C457F
    bne     r15, r28, .dos_run_reject_flat
    store.q r16, 320(r29)               ; saved args_ptr for ELF path
    store.q r18, 328(r29)               ; saved args_len for ELF path

    move.q  r1, r21                    ; temp contiguous ELF image
    move.q  r2, r23                    ; image_size
    jsr     .dos_elf_build_seglist
    load.q  r29, (sp)
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
    move.q  r9, r8
    and     r9, r9, #4
    beqz    r9, .debs_badarg_free
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
    dc.b    "Welcome to IntuitionOS M11", 0x0D, 0x0A, 0
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
    align   8
prog_doslib_boot_export_rows:
    ; M14.1 phase 3: dos-private exported staged-service ELF sources.
    ; Filled by kern_export_boot_manifest_to_dos after dos.library boots.
    ds.b    (DOS_BOOT_EXPORT_COUNT * DOS_BOOT_EXPORT_ROW_SZ)
    ; Static assign table for M15 phase 2 resolver. Entry layout:
    ;   [0..15]  assign name (NUL-terminated, uppercase canonical)
    ;   [16..31] target prefix (NUL-terminated, empty for RAM:)
prog_doslib_assign_default_c:
    dc.b    "C", 0
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
    align   4096

; ---------------------------------------------------------------------------
; Embedded command images (VERSION, AVAIL, DIR, TYPE, ECHO)
; ---------------------------------------------------------------------------

prog_doslib_seed_images_start:
    align   8
seed_elf_version:
    incbin  "seed_version.elf"
seed_elf_version_end:

    align   8
seed_elf_avail:
    incbin  "seed_avail.elf"
seed_elf_avail_end:

    align   8
seed_elf_dir:
    incbin  "seed_dir.elf"
seed_elf_dir_end:

    align   8
seed_elf_type:
    incbin  "seed_type.elf"
seed_elf_type_end:

    align   8
seed_elf_echo:
    incbin  "seed_echo.elf"
seed_elf_echo_end:

    align   8
seed_elf_assign:
    incbin  "seed_assign.elf"
seed_elf_assign_end:

    align   8
seed_elf_list:
    incbin  "seed_list.elf"
seed_elf_list_end:

    align   8
seed_elf_which:
    incbin  "seed_which.elf"
seed_elf_which_end:

    align   8
seed_elf_help:
    incbin  "seed_help.elf"
seed_elf_help_end:

    align   8
seed_elf_gfxdemo:
    incbin  "seed_gfxdemo.elf"
seed_elf_gfxdemo_end:

    align   8
seed_elf_about:
    incbin  "seed_about.elf"
seed_elf_about_end:

; ---------------------------------------------------------------------------
; Phase 1 of M15.1: split the seeded command sources out of the monolithic
; iexec root while keeping the program labels and ROM layout order unchanged.
; ---------------------------------------------------------------------------

include "../cmd/version.s"
include "../cmd/avail.s"
include "../cmd/dir.s"
include "../cmd/type.s"
include "../cmd/echo.s"
include "../cmd/help.s"
include "../cmd/list.s"
include "../cmd/which.s"
include "../cmd/assign.s"

include "../assets/dos_seed_text.s"
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

include "../handler/shell.s"

prog_doslib_data_end:
    align   8
prog_doslib_end:
