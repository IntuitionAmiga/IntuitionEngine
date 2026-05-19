; ehbasic_hw_host.inc - EhBASIC IE64 HOST command bridge
;
; Statement handler for:
;   HOST NET
;   HOST UPDATE
;   HOST REBOOT
;   HOST POWEROFF
;
; The guest side only writes a command enum and trigger bit to the host helper
; MMIO aperture, then polls its status byte until the host-side helper reports
; a terminal state.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; exec_do_host - HOST subverb dispatcher
; ============================================================================

exec_do_host:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    jsr     toupper

    move.q  r2, #0x4E             ; 'N'
    beq     r1, r2, .host_net
    move.q  r2, #0x55             ; 'U'
    beq     r1, r2, .host_update
    move.q  r2, #0x52             ; 'R'
    beq     r1, r2, .host_reboot
    move.q  r2, #0x50             ; 'P'
    beq     r1, r2, .host_poweroff
    bra     exec_do_unknown

.host_net:
    load.b  r1, (r17)
    jsr     toupper
    move.q  r2, #0x4E             ; 'N'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 1(r17)
    jsr     toupper
    move.q  r2, #0x45             ; 'E'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 2(r17)
    jsr     toupper
    move.q  r2, #0x54             ; 'T'
    bne     r1, r2, exec_do_unknown
    add.q   r17, r17, #3
    jsr     .host_require_end
    beqz    r6, exec_do_unknown
    move.q  r1, #HOST_CMD_NET
    bra     .host_do

.host_update:
    load.b  r1, (r17)
    jsr     toupper
    move.q  r2, #0x55             ; 'U'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 1(r17)
    jsr     toupper
    move.q  r2, #0x50             ; 'P'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 2(r17)
    jsr     toupper
    move.q  r2, #0x44             ; 'D'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 3(r17)
    jsr     toupper
    move.q  r2, #0x41             ; 'A'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 4(r17)
    jsr     toupper
    move.q  r2, #0x54             ; 'T'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 5(r17)
    jsr     toupper
    move.q  r2, #0x45             ; 'E'
    bne     r1, r2, exec_do_unknown
    add.q   r17, r17, #6
    jsr     .host_require_end
    beqz    r6, exec_do_unknown
    move.q  r1, #HOST_CMD_UPDATE
    bra     .host_do

.host_reboot:
    load.b  r1, (r17)
    jsr     toupper
    move.q  r2, #0x52             ; 'R'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 1(r17)
    jsr     toupper
    move.q  r2, #0x45             ; 'E'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 2(r17)
    jsr     toupper
    move.q  r2, #0x42             ; 'B'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 3(r17)
    jsr     toupper
    move.q  r2, #0x4F             ; 'O'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 4(r17)
    jsr     toupper
    move.q  r2, #0x4F             ; 'O'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 5(r17)
    jsr     toupper
    move.q  r2, #0x54             ; 'T'
    bne     r1, r2, exec_do_unknown
    add.q   r17, r17, #6
    jsr     .host_require_end
    beqz    r6, exec_do_unknown
    move.q  r1, #HOST_CMD_REBOOT
    bra     .host_do

.host_poweroff:
    load.b  r1, (r17)
    jsr     toupper
    move.q  r2, #0x50             ; 'P'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 1(r17)
    jsr     toupper
    move.q  r2, #0x4F             ; 'O'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 2(r17)
    jsr     toupper
    move.q  r2, #0x57             ; 'W'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 3(r17)
    jsr     toupper
    move.q  r2, #0x45             ; 'E'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 4(r17)
    jsr     toupper
    move.q  r2, #0x52             ; 'R'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 5(r17)
    jsr     toupper
    move.q  r2, #0x4F             ; 'O'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 6(r17)
    jsr     toupper
    move.q  r2, #0x46             ; 'F'
    bne     r1, r2, exec_do_unknown
    load.b  r1, 7(r17)
    jsr     toupper
    move.q  r2, #0x46             ; 'F'
    bne     r1, r2, exec_do_unknown
    add.q   r17, r17, #8
    jsr     .host_require_end
    beqz    r6, exec_do_unknown
    move.q  r1, #HOST_CMD_POWEROFF

.host_do:
    la      r2, HOST_MMIO_BASE
    store.b r1, (r2)
    move.q  r3, #1
    store.b r3, 4(r2)

.host_poll:
    load.b  r4, 8(r2)
    move.q  r5, #HOST_STATUS_RUNNING
    beq     r4, r5, .host_poll

    move.q  r5, #HOST_STATUS_OK
    beq     r4, r5, .host_done
    move.q  r5, #HOST_STATUS_CANCEL
    beq     r4, r5, .host_done

.host_err:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    rts

.host_done:
    rts

.host_require_end:
    jsr     exec_skip_spaces
    load.b  r6, (r17)
    move.q  r7, #0
    beq     r6, r7, .host_require_end_ok
    move.q  r7, #0x3A             ; ':'
    beq     r6, r7, .host_require_end_ok
    move.q  r6, #0
    rts
.host_require_end_ok:
    move.q  r6, #1
    rts
