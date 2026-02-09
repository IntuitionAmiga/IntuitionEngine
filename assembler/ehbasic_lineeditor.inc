; ehbasic_lineeditor.inc - EhBASIC IE64 Line Editor
;
; Manages BASIC program lines in memory as a sorted singly-linked list.
;
; Line format in memory:
;   +0  (4 bytes): Next line pointer (0 = end of program)
;   +4  (4 bytes): Line number
;   +8  (n bytes): Tokenized content (null-terminated)
;   After null: aligned to 4-byte boundary
;
; Register conventions:
;   R16 = interpreter state base (BASIC_STATE)
;   R26 = cached TERM_OUT address
;   R27 = cached TERM_STATUS address
;
; Calling conventions:
;   line_init:   Initialise program memory. R16 must be set.
;   line_new:    Clear all program text. R16 must be set.
;   line_store:  R8 = line number, R9 = tokenized content ptr,
;                R10 = content length (0 = delete line).
;   line_search: R8 = target line number.
;                Returns R8 = pointer to line (0 if not found),
;                        R9 = pointer to previous line's next-ptr (for insertion).
;   line_list:   R8 = start line number, R9 = end line number.
;                Outputs to terminal.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; line_init - Initialise program memory
; ============================================================================
; Sets up an empty program with a null terminator at BASIC_PROG_START.
; Input:  R16 = state block base
; Clobbers: R1, R2

line_init:
    ; Write BASIC_PROG_START to ST_PROG_START in state block
    la      r1, BASIC_PROG_START
    store.l r1, (r16)               ; state[ST_PROG_START] = BASIC_PROG_START

    ; Write null terminator (next-ptr = 0) at BASIC_PROG_START
    store.l r0, (r1)

    ; Set prog_end = BASIC_PROG_START + 4
    add.q   r2, r1, #4
    add.q   r1, r16, #4             ; &state[ST_PROG_END]
    store.l r2, (r1)

    rts

; ============================================================================
; line_new - Clear all program text (NEW command)
; ============================================================================
; Input:  R16 = state block base
; Clobbers: R1, R2

line_new:
    ; Same as line_init — reset to empty program
    bra     line_init

; ============================================================================
; line_search - Find a line by number
; ============================================================================
; Input:  R8 = target line number
; Output: R8 = pointer to the line with this number (0 if not found)
;         R9 = pointer to the next-pointer field that points AT or PAST the
;              target (for insertion: where to link a new line before).
;              If target < first line, R9 = address of state[ST_PROG_START].
; Clobbers: R1-R5

line_search:
    ; R3 = pointer to "previous next-pointer" (starts at state block's prog_start field)
    move.q  r3, r16                 ; &state[ST_PROG_START]

    ; R4 = current line pointer (read from the next-pointer)
    load.l  r4, (r3)               ; first line address

.ls_loop:
    ; If current = 0, end of list
    beqz    r4, .ls_not_found

    ; Read line number at current+4
    add.q   r5, r4, #4
    load.l  r5, (r5)               ; line number of current line

    ; Compare with target
    beq     r5, r8, .ls_found      ; exact match
    bgt     r5, r8, .ls_not_found  ; passed it — target not in list

    ; Advance: prev_next_ptr = &current->next, current = current->next
    move.q  r3, r4                 ; prev_next_ptr = current (which is &next field)
    load.l  r4, (r4)              ; current = current->next
    bra     .ls_loop

.ls_found:
    move.q  r8, r4                 ; R8 = pointer to found line
    move.q  r9, r3                 ; R9 = prev_next_ptr
    rts

.ls_not_found:
    move.q  r8, r0                 ; R8 = 0 (not found)
    move.q  r9, r3                 ; R9 = insertion point
    rts

; ============================================================================
; line_store - Insert, replace, or delete a line
; ============================================================================
; Input:  R8 = line number
;         R9 = pointer to tokenized content (null-terminated)
;         R10 = content length (bytes, NOT including null terminator)
;               If R10 = 0, the line is deleted.
; Output: none
; Clobbers: R1-R15
;
; Algorithm:
;   1. Search for existing line with this number
;   2. If found, delete it (close the gap)
;   3. If R10 > 0, insert new line at the correct position
;   4. Update prog_end in state block

line_store:
    push    r14
    push    r15
    push    r22
    push    r23
    push    r24
    push    r25

    move.q  r22, r8                 ; R22 = line number
    move.q  r23, r9                 ; R23 = content pointer
    move.q  r24, r10                ; R24 = content length

    ; --- Step 1: Search for existing line ---
    jsr     line_search             ; R8 = found line (or 0), R9 = prev_next_ptr

    move.q  r25, r9                 ; R25 = prev_next_ptr (saved for insertion)

    ; --- Step 2: If found, delete old line ---
    beqz    r8, .ls_no_delete

    ; Found existing line at R8. Calculate its size.
    ; Line size = header (8) + content bytes + null + align to 4
    move.q  r14, r8                 ; R14 = old line start
    load.l  r15, (r14)              ; R15 = old line's next-pointer (start of next line or gap end)

    ; Calculate bytes to move: everything from R15 to prog_end needs to shift up to R14
    add.q   r1, r16, #4             ; &state[ST_PROG_END]
    load.l  r2, (r1)               ; R2 = prog_end

    ; Number of bytes to copy = prog_end - old_next
    sub.q   r3, r2, r15            ; R3 = bytes to copy

    ; Copy from R15 to R14 (forward copy — src > dst so safe)
    move.q  r4, r14                 ; dst
    move.q  r5, r15                 ; src
    beqz    r3, .ls_copy_done

.ls_del_copy:
    load.b  r6, (r5)
    store.b r6, (r4)
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    sub.q   r3, r3, #1
    bnez    r3, .ls_del_copy

.ls_copy_done:
    ; Update prog_end: prog_end -= (old_next - old_line_start)
    sub.q   r1, r15, r14           ; size of deleted line
    add.q   r2, r16, #4             ; &state[ST_PROG_END]
    load.l  r3, (r2)
    sub.q   r3, r3, r1
    store.l r3, (r2)

    ; Re-search for the insertion point (addresses may have shifted)
    move.q  r8, r22                 ; line number
    jsr     line_search
    move.q  r25, r9                 ; update prev_next_ptr

.ls_no_delete:
    ; --- Step 3: If content length = 0, re-chain and done (delete only) ---
    bnez    r24, .ls_insert
    jsr     line_rechain
    bra     .ls_done

.ls_insert:

    ; --- Step 4: Calculate new line size ---
    ; size = 8 (header) + content_length + 1 (null) + padding to align 4
    add.q   r14, r24, #9            ; 8 header + content + 1 null
    ; Align to 4: (size + 3) & ~3
    add.q   r14, r14, #3
    and.q   r14, r14, #0xFFFFFFFC   ; R14 = aligned line size

    ; --- Step 5: Make room by shifting data down ---
    ; Everything from insertion point to prog_end needs to move down by R14 bytes
    ; The insertion point is what R25 (prev_next_ptr) points to.
    ; The new line will be placed at the address currently stored in *R25.

    load.l  r1, (r25)              ; R1 = current address at insertion point
    ; This is either the address of the next line or 0 (end).
    ; But we need to insert AT the position. The new line goes at the
    ; address where the next item starts.

    ; Find where to insert: if prev_next_ptr is the state block field,
    ; insert at prog_start. Otherwise, insert at the address stored in *R25.
    move.q  r15, r1                 ; R15 = insert address (where next line starts, or end)

    ; If R15 = 0, we're appending at the end.
    ; The insert address is actually where the null terminator is.
    ; We need to find the actual memory address.
    bnez    r15, .ls_has_next

    ; Appending at end: insert address = prog_end - 4 (the null terminator location)
    add.q   r1, r16, #4             ; &state[ST_PROG_END]
    load.l  r15, (r1)
    sub.q   r15, r15, #4            ; point at the null terminator
    bra     .ls_shift

.ls_has_next:
    ; R15 = address of the next line (we insert before it)

.ls_shift:
    ; Shift everything from R15 to prog_end down by R14 bytes
    add.q   r1, r16, #4             ; &state[ST_PROG_END]
    load.l  r2, (r1)               ; R2 = prog_end
    sub.q   r3, r2, r15            ; R3 = bytes to move

    ; Copy backwards (dst > src)
    add.q   r4, r2, r14            ; dst end = prog_end + new_line_size
    move.q  r5, r2                 ; src end = prog_end
    beqz    r3, .ls_shift_done

.ls_shift_copy:
    sub.q   r4, r4, #1
    sub.q   r5, r5, #1
    load.b  r6, (r5)
    store.b r6, (r4)
    sub.q   r3, r3, #1
    bnez    r3, .ls_shift_copy

.ls_shift_done:
    ; Update prog_end
    add.q   r1, r16, #4             ; &state[ST_PROG_END]
    load.l  r2, (r1)
    add.q   r2, r2, r14
    store.l r2, (r1)

    ; --- Step 6: Write the new line at R15 ---
    ; Header: next-pointer (will be fixed by re-chain), line number
    ; For now, write 0 as next-pointer (will be set by re-chain)
    store.l r0, (r15)               ; next-pointer = 0 (temporary)
    add.q   r1, r15, #4
    store.l r22, (r1)               ; line number
    add.q   r1, r1, #4

    ; Copy content from R23 (length R24)
    move.q  r2, r23                 ; src = content
    move.q  r3, r24                 ; count
    beqz    r3, .ls_content_done

.ls_content_copy:
    load.b  r6, (r2)
    store.b r6, (r1)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    sub.q   r3, r3, #1
    bnez    r3, .ls_content_copy

.ls_content_done:
    ; Write null terminator after content
    store.b r0, (r1)

    ; --- Step 7: Re-chain all lines ---
    jsr     line_rechain

.ls_done:
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts

; ============================================================================
; line_rechain - Fix up all next-line pointers
; ============================================================================
; Walks through program memory, setting each line's next-pointer to point
; at the next line (determined by scanning past the null-terminated content
; and aligning to 4 bytes).
; Input:  R16 = state block base
; Clobbers: R1-R5

line_rechain:
    ; Start at prog_start
    load.l  r1, (r16)              ; R1 = current line address

.rc_loop:
    ; Check if this is the end terminator (next-ptr = 0 and no valid line after)
    ; Read what's at R1: if it's the terminator sentinel, we're done
    ; A line has: next-ptr(4), linenum(4), content..., null
    ; The end is just a single 0x00000000 word.

    ; Scan past content to find where next line would be
    add.q   r2, r1, #8             ; R2 = start of content

.rc_scan:
    load.b  r3, (r2)
    add.q   r2, r2, #1
    bnez    r3, .rc_scan

    ; R2 now points past the null terminator.
    ; Align to 4: r2 = (r2 + 3) & ~3
    add.q   r2, r2, #3
    and.q   r2, r2, #0xFFFFFFFC

    ; Check if R2 >= prog_end — if so, this is the last line
    add.q   r3, r16, #4             ; &state[ST_PROG_END]
    load.l  r3, (r3)               ; prog_end
    bge     r2, r3, .rc_last

    ; R2 points at the next line. Check if it's a valid line or the end terminator.
    load.l  r4, (r2)               ; peek at next line's next-pointer field
    ; Check if there's room for another line header (8 bytes minimum).
    ; If R2 + 8 > prog_end, this must be the terminator.
    add.q   r5, r2, #8
    bgt     r5, r3, .rc_last

    ; Valid next line exists at R2
    store.l r2, (r1)               ; current->next = R2
    move.q  r1, r2                 ; advance to next line
    bra     .rc_loop

.rc_last:
    ; This is the last line. Its next-pointer = address of end terminator.
    store.l r2, (r1)               ; current->next = R2 (terminator location)
    store.l r0, (r2)               ; terminator = 0
    rts

; ============================================================================
; line_list - Output program listing
; ============================================================================
; Input:  R8 = start line number (0 for beginning)
;         R9 = end line number (0xFFFFFF or large for all)
; Output: Prints lines to terminal
; Clobbers: R1-R7, R8-R10, R14, R15

line_list:
    push    r14
    push    r15
    push    r22
    push    r23

    move.q  r22, r8                 ; R22 = start line#
    move.q  r23, r9                 ; R23 = end line#

    ; Start at first line
    load.l  r14, (r16)              ; R14 = current line pointer

.ll_loop:
    ; Check for end of list
    beqz    r14, .ll_done

    ; Read next-pointer and line number
    load.l  r15, (r14)              ; R15 = next-pointer
    add.q   r1, r14, #4
    load.l  r1, (r1)               ; R1 = line number

    ; Check range
    blt     r1, r22, .ll_skip      ; before start range
    bgt     r1, r23, .ll_done      ; past end range

    ; Print line number
    move.q  r8, r1
    jsr     print_uint32

    ; Print space
    move.q  r8, #0x20
    jsr     putchar

    ; Print content (starting at offset +8)
    add.q   r8, r14, #8
    jsr     print_string

    ; Print CR+LF
    jsr     print_crlf

.ll_skip:
    ; Advance to next line
    move.q  r14, r15
    bra     .ll_loop

.ll_done:
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts

; ============================================================================
; print_uint32 - Print an unsigned 32-bit integer
; ============================================================================
; Input:  R8 = value to print (32-bit unsigned)
; Output: Printed to terminal via putchar
; Clobbers: R1-R7

print_uint32:
    push    r14

    ; Handle zero specially
    beqz    r8, .pu_zero

    ; Convert to decimal digits on stack (reverse order)
    move.q  r14, r0                 ; R14 = digit count
    move.q  r1, r8                  ; R1 = value
    and.l   r1, r1, #0xFFFFFFFF    ; mask to 32 bits

.pu_div_loop:
    beqz    r1, .pu_print
    ; Divide by 10: r2 = r1 / 10, r3 = r1 % 10
    move.q  r2, r1
    move.q  r3, #10
    divu.l  r2, r2, r3             ; quotient
    mulu.l  r4, r2, r3             ; quotient * 10
    sub.l   r3, r1, r4             ; remainder = value - quotient*10
    add.q   r3, r3, #0x30          ; convert to ASCII
    push    r3
    add.q   r14, r14, #1
    move.q  r1, r2
    bra     .pu_div_loop

.pu_print:
    beqz    r14, .pu_end
    pop     r8
    jsr     putchar
    sub.q   r14, r14, #1
    bra     .pu_print

.pu_zero:
    move.q  r8, #0x30               ; '0'
    jsr     putchar

.pu_end:
    pop     r14
    rts

; ============================================================================
; EOF
; ============================================================================
