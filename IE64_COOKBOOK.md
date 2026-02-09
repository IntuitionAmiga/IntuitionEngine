# IE64 Assembly Cookbook

Canonical no-flags programming patterns for the IE64 64-bit RISC CPU.

The IE64 is a load-store RISC processor with 32 general-purpose 64-bit registers,
fixed 8-byte instructions, and compare-and-branch semantics (no flags register).
Assembly uses 68K-flavored lowercase syntax with `.b`/`.w`/`.l`/`.q` size suffixes
(default `.q` for 64-bit).

---

## Register Conventions

The following calling/register rules are **project ABI conventions** used by IE64
examples. They are not enforced by CPU hardware instructions.

| Register | Purpose |
|----------|---------|
| `r0` | Hardwired zero (reads always return 0, writes are discarded) |
| `r1`-`r7` | Temporaries / scratch (caller-saved) |
| `r8`-`r15` | Argument / return value registers (caller-saved) |
| `r16`-`r27` | Callee-saved (preserved across calls) |
| `r28`-`r30` | Temporaries (caller-saved) |
| `r31` / `sp` | Stack pointer (grows downward, 8-byte aligned) |

The link address is pushed to the stack by `jsr` and popped by `rts`. There is no
dedicated link register.

---

## 1. Countdown Loops

Decrement a counter and loop until it hits zero. This is the simplest loop form on
the IE64 because `bnez` (pseudo-op for `bne rs, r0, label`) naturally tests against
the hardwired-zero register.

```asm
; Fill 256 bytes at address in r2 with the value in r3.
; r1 = loop counter (destroyed)

                move.q  r1, #256        ; iteration count
.fill_loop:
                store.b r3, (r2)        ; write one byte
                add.q   r2, r2, #1      ; advance pointer
                sub.q   r1, r1, #1      ; decrement counter
                bnez    r1, .fill_loop  ; loop while r1 != 0

; After the loop, r1 = 0 and r2 points one past the last byte written.
```

Notes:
- `bnez r1, label` expands to `bne r1, r0, label`. Since `r0` is always zero, this
  is an efficient "branch if nonzero" idiom.
- For quad-word aligned data, use `store.q` with a stride of 8 and divide the
  iteration count accordingly.

---

## 2. Count-Up Loops

Count upward from zero to a limit. Use `blt` to compare the index register against
a limit held in another register (signed comparison).

```asm
; Sum integers 0..99 into r3.
; r1 = index, r2 = limit, r3 = accumulator

                move.q  r3, #0          ; accumulator = 0
                move.q  r1, #0          ; i = 0
                move.q  r2, #100        ; limit = 100
.sum_loop:
                add.q   r3, r3, r1      ; accumulator += i
                add.q   r1, r1, #1      ; i++
                blt     r1, r2, .sum_loop ; loop while i < 100

; After the loop, r3 = 4950 (sum of 0..99).
```

Notes:
- `blt` performs a signed comparison: `int64(r1) < int64(r2)`.
- If you need `i <= limit`, use `ble` instead or set the limit to N+1 with `blt`.
- For descending iteration over a signed range, swap `blt` to `bgt`.

---

## 3. Signed vs Unsigned Compare Branches

The IE64 provides two sets of comparison branches:

| Mnemonic | Condition | Interpretation |
|----------|-----------|----------------|
| `blt` | `int64(rs) < int64(rt)` | Signed less-than |
| `bge` | `int64(rs) >= int64(rt)` | Signed greater-or-equal |
| `bgt` | `int64(rs) > int64(rt)` | Signed greater-than |
| `ble` | `int64(rs) <= int64(rt)` | Signed less-or-equal |
| `bhi` | `uint64(rs) > uint64(rt)` | Unsigned higher |
| `bls` | `uint64(rs) <= uint64(rt)` | Unsigned lower-or-same |

### Signed example: temperature bounds check

```asm
; Branch to .too_cold if temperature (r1) < -40 (signed).
                moveq   r2, #-40        ; r2 = -40 (sign-extended to 64 bits)
                blt     r1, r2, .too_cold
```

### Unsigned example: array bounds check

```asm
; Branch to .out_of_bounds if index (r1) > max_index (r2), treating both as
; unsigned quantities. This catches negative indices that wrap to huge values.
                move.q  r2, #255        ; maximum valid index
                bhi     r1, r2, .out_of_bounds
```

Notes:
- Use `moveq` to load negative 32-bit constants; it sign-extends to 64 bits.
- Use `bhi`/`bls` whenever operands represent addresses, sizes, or array indices
  (inherently unsigned quantities).
- `beq` and `bne` are signedness-neutral since they test bitwise equality.

---

## 4. Range Checks

Testing `low <= x < high` without flags uses a subtract-and-unsigned-compare trick.
Subtracting `low` from `x` maps the valid range to `[0, high-low)`. A single
unsigned compare then catches both below-low (wraps to a huge unsigned value) and
at-or-above-high.

```asm
; Check if r1 is in range [32, 127) (printable ASCII).
; Branch to .not_printable if outside.

                sub.q   r3, r1, #32     ; r3 = x - low
                move.q  r4, #94         ; r4 = high - low - 1 (127 - 32 - 1)
                bhi     r3, r4, .not_printable ; unsigned: if r3 > 94, out of range

; Falls through here if 32 <= r1 < 127.
```

### General pattern

```asm
; Test low <= x < high (unsigned range check in 3 instructions):
;   sub.q  tmp, x, #low
;   move.q limit, #(high - low - 1)
;   bhi    tmp, limit, .out_of_range
```

Notes:
- This works because if `x < low`, the subtraction wraps to a very large unsigned
  value that exceeds `high - low - 1`.
- For an inclusive upper bound (`low <= x <= high`), use `high - low` as the limit.
- This pattern assumes `high > low`.
- This technique costs 3 instructions and 2 scratch registers, which is optimal for
  a no-flags architecture.

---

## 5. Function Prologue/Epilogue

The IE64 uses `jsr label` (jump to subroutine) which pushes the return address onto
the stack, and `rts` (return from subroutine) which pops it. Callee-saved registers
must be preserved by the called function.

> **Conventions vs ISA guarantees**
> - **ISA-guaranteed behavior**: `jsr`/`rts`/`push`/`pop` stack mechanics, register
>   widths, and branch semantics (as defined in `IE64_ISA.md`).
> - **Project ABI convention (this cookbook)**: which registers are caller-saved vs
>   callee-saved, argument register assignments, and return-value registers.
> - You may define a different ABI for a project, but all code in that project must
>   follow it consistently.

### Leaf function (no nested calls, no callee-saved registers used)

```asm
; int add_two(int a, int b) — args in r8, r9; result in r8
add_two:
                add.q   r8, r8, r9      ; r8 = a + b
                rts
```

### Non-leaf function with callee-saved registers

```asm
; void process(int *data, int count) — args in r8, r9
process:
                ; -- prologue --
                push    r16             ; save callee-saved registers
                push    r17
                push    r18

                move.q  r16, r8         ; r16 = data pointer (preserved)
                move.q  r17, r9         ; r17 = count (preserved)
                move.q  r18, #0         ; r18 = index

.proc_loop:
                ; call helper(data[index]) — sets up arg in r8
                lsl.q   r1, r18, #3     ; r1 = index * 8 (quad offset)
                add.q   r1, r16, r1     ; r1 = &data[index]
                load.q  r8, (r1)        ; r8 = data[index]
                jsr     helper          ; call helper (may clobber r1-r15, r28-r30)

                add.q   r18, r18, #1    ; index++
                blt     r18, r17, .proc_loop

                ; -- epilogue --
                pop     r18             ; restore callee-saved registers (reverse order)
                pop     r17
                pop     r16
                rts
```

Notes:
- `push` and `pop` always operate on 8-byte (quad) values.
- Push/pop order must be symmetric: push in forward order, pop in reverse order
  (LIFO).
- `r1`-`r15` and `r28`-`r30` are caller-saved; a callee may freely destroy them.
- `r16`-`r27` are callee-saved; if used, they must be pushed in the prologue and
  popped in the epilogue.
- Arguments are passed in `r8`-`r15`; return values in `r8` (or `r8`:`r9` for
  128-bit results).

---

## 6. 64-bit Constants

### Small constants (fits in 32-bit unsigned)

```asm
                move.q  r1, #$DEADBEEF  ; r1 = 0x00000000_DEADBEEF (zero-extended)
```

`move` with an immediate loads a 32-bit value zero-extended to 64 bits.

### Negative constants (sign-extended from 32-bit)

```asm
                moveq   r1, #-1         ; r1 = 0xFFFFFFFF_FFFFFFFF (-1 sign-extended)
                moveq   r2, #-1000      ; r2 = 0xFFFFFFFF_FFFFFC18
```

`moveq` sign-extends a 32-bit immediate to the full 64 bits. Use this for any
negative value or for setting all-ones masks.

### Full 64-bit constants

```asm
; Load r1 = $CAFEBABE_DEADBEEF using the li pseudo-op.
; The assembler automatically expands this to two instructions:
;   move.l r1, #$DEADBEEF      ; load low 32 bits
;   movt   r1, #$CAFEBABE      ; set upper 32 bits

                li      r1, #$CAFEBABE_DEADBEEF
```

### Manual two-instruction sequence (equivalent to `li`)

```asm
                move.l  r1, #$DEADBEEF  ; r1 = 0x00000000_DEADBEEF
                movt    r1, #$CAFEBABE  ; r1 = 0xCAFEBABE_DEADBEEF
```

Notes:
- `li` is a pseudo-op. If the value fits in 32 bits, the assembler emits a single
  `move.l`. Otherwise it emits `move.l` + `movt` (two instructions, 16 bytes).
- `movt` replaces the upper 32 bits while preserving the lower 32 bits.
- `move.l` (with `.l` size suffix) masks the result to 32 bits, which is necessary
  when `movt` will fill the upper half. Using `.q` with an immediate also works but
  only zero-extends the 32-bit immediate field.

---

## 7. Conditional Execution Without Flags

The IE64 has no condition flags and no predicated instructions. All conditional
logic is expressed through compare-and-branch.

### Pattern: conditional assignment (select)

```asm
; r3 = (r1 == r2) ? r4 : r5
; Uses a branch to skip the alternative.

                move.q  r3, r5          ; assume false path: r3 = r5
                beq     r1, r2, .was_eq ; if r1 == r2, skip to true path
                bra     .done_sel
.was_eq:
                move.q  r3, r4          ; true path: r3 = r4
.done_sel:
```

### Pattern: branchless absolute value

```asm
; r2 = abs(r1)
; Uses arithmetic shift to extract sign, then conditional negate.

                asr.q   r3, r1, #63     ; r3 = 0 if r1 >= 0, -1 if r1 < 0
                eor.q   r2, r1, r3      ; if negative: flip all bits
                sub.q   r2, r2, r3      ; if negative: add 1 (two's complement abs)

; This works because: if r1 >= 0, r3=0 and eor/sub are no-ops.
; If r1 < 0, r3=-1 (all ones), eor flips bits, sub adds 1 = negate.
```

### Pattern: conditional increment using sub+beqz

```asm
; Increment r3 only if r1 == r2.

                sub.q   r4, r1, r2      ; r4 = r1 - r2
                bnez    r4, .skip_inc   ; if r1 != r2, skip
                add.q   r3, r3, #1      ; r3++ (only when r1 == r2)
.skip_inc:
```

Notes:
- The `sub` + `beqz`/`bnez` idiom is the fundamental "test and branch" pattern.
  Subtracting two values and branching on the result against zero replaces the
  traditional "compare; branch-on-flags" sequence.
- For simple min/max, branch over a `move`:
  `blt r1, r2, .skip` / `move.q r1, r2` / `.skip:` gives `r1 = min(r1, r2)`.

---

## 8. Memory-Mapped I/O Access

Device registers are accessed through `load`/`store` at fixed addresses. Use `la`
(load address) to put the device base into a register, then access registers via
displacement.

### VGA mode selection

```asm
; Set VGA to Mode 13h (320x200, 256 colors).
; VGA registers are at $F1000-$F13FF.

VGA_BASE        equ     $F1000
VGA_MODE        equ     $F1000          ; mode register
VGA_CTRL        equ     $F1008          ; control register (bit 0 = enable)

                la      r1, VGA_BASE    ; r1 = base address of VGA registers
                move.l  r2, #$13        ; mode 13h
                store.l r2, (r1)        ; write to VGA_MODE (offset 0)
                move.l  r2, #1          ; enable bit
                store.l r2, 8(r1)       ; write to VGA_CTRL (offset 8 from base)
```

### Polling VGA vsync

```asm
; Wait for vertical sync (bit 0 of VGA_STATUS).
; VGA_STATUS is at VGA_BASE + 4.

VGA_STATUS      equ     $F1004

                la      r1, VGA_STATUS
.wait_vsync:
                load.l  r2, (r1)        ; read status register
                and.l   r3, r2, #1      ; isolate vsync bit
                beqz    r3, .wait_vsync ; loop until vsync = 1
```

### Writing a pixel to VRAM (Mode 13h)

```asm
; Plot pixel at (x=100, y=50) with color index 15.
; VRAM window is at $A0000, stride = 320 bytes in mode 13h.

VRAM_BASE       equ     $A0000

                la      r1, VRAM_BASE
                move.q  r2, #50         ; y
                mulu.q  r3, r2, #320    ; y * stride
                add.q   r3, r3, #100    ; + x
                add.q   r3, r1, r3      ; absolute VRAM address
                move.b  r4, #15         ; color index
                store.b r4, (r3)        ; write pixel
```

Notes:
- `la rd, addr` is a pseudo-op that expands to `lea rd, addr(r0)`. Since `r0` is
  always zero, this loads the absolute address into the register.
- `la` is lowered textually to `lea rd, expr(r0)`. Address expressions containing
  inner parentheses can be parsed as addressing syntax and fail to assemble
  (for example `la r1, BASE+(1*4)`). Prefer flattened expressions like `BASE+4`, or
  compute complex address math in separate instructions.
- I/O registers are typically 32-bit (`.l`), while VRAM pixels are byte (`.b`) in
  mode 13h.
- There is no "volatile" keyword; the load/store instructions access the bus
  directly. The CPU does not cache memory reads, so repeated loads from I/O
  addresses will always read fresh values.
- Important for `.q` (64-bit) accesses: bus operations may be split into two 32-bit
  transactions when touching I/O regions. Treat `.q` MMIO accesses as potentially
  non-atomic unless a device explicitly documents 64-bit atomic behavior.

---

## 9. Memcpy/Memset Loops

### Byte-at-a-time memcpy

```asm
; Copy r3 bytes from address in r1 (src) to address in r2 (dst).
; r1, r2, r3 are all destroyed.

memcpy_bytes:
                beqz    r3, .copy_done  ; nothing to copy
.copy_loop:
                load.b  r4, (r1)        ; read byte from src
                store.b r4, (r2)        ; write byte to dst
                add.q   r1, r1, #1      ; src++
                add.q   r2, r2, #1      ; dst++
                sub.q   r3, r3, #1      ; count--
                bnez    r3, .copy_loop
.copy_done:
                rts
```

### Quad-word aligned memcpy (8 bytes per iteration)

```asm
; Copy r3 quad-words (8 bytes each) from r1 (src) to r2 (dst).
; Source and destination must be 8-byte aligned.
; Total bytes copied = r3 * 8.

memcpy_quads:
                beqz    r3, .qcopy_done
.qcopy_loop:
                load.q  r4, (r1)        ; read 8 bytes from src
                store.q r4, (r2)        ; write 8 bytes to dst
                add.q   r1, r1, #8      ; src += 8
                add.q   r2, r2, #8      ; dst += 8
                sub.q   r3, r3, #1      ; count--
                bnez    r3, .qcopy_loop
.qcopy_done:
                rts
```

### Quad-word memset

```asm
; Fill r3 quad-words at address r1 with value in r2.
; Address must be 8-byte aligned.

memset_quads:
                beqz    r3, .mset_done
.mset_loop:
                store.q r2, (r1)        ; write 8 bytes
                add.q   r1, r1, #8      ; ptr += 8
                sub.q   r3, r3, #1      ; count--
                bnez    r3, .mset_loop
.mset_done:
                rts
```

Notes:
- For large copies, prefer the quad-word variant; each iteration moves 8 bytes vs 1.
- To handle an arbitrary byte count with the quad-word loop, divide by 8, do the
  quad loop for the bulk, then do a byte loop for the 0-7 byte remainder:
  `lsr.q r5, r3, #3` gives the quad count; `and.q r6, r3, #7` gives the remainder.
- Overlapping copies (memmove semantics) require checking whether `dst < src` and
  copying backward if not.
- Use the quad-word form for RAM/VRAM bulk data paths, not control MMIO registers.
  `.q` device accesses can split into two 32-bit bus transactions and are not a safe
  replacement for explicit `.l` register programming sequences.

---

## 10. Switch/Dispatch Patterns

IE64 branch instructions target labels (PC-relative offsets resolved by the
assembler). The `jmp (Rs)` and `jsr (Rs)` instructions provide register-indirect
control flow for jump tables, function pointers, and vtable dispatch.

### Dispatch on value 0..3

```asm
; Dispatch on r1 (0..3). Jump to case_0, case_1, case_2, or case_3.
; r1 is bounds-checked first.

                move.q  r2, #3
                bhi     r1, r2, .default_case   ; unsigned: if r1 > 3, default
dispatch:
                beqz    r1, case_0              ; r1 == 0?
                move.q  r2, #1
                beq     r1, r2, case_1          ; r1 == 1?
                move.q  r2, #2
                beq     r1, r2, case_2          ; r1 == 2?
                move.q  r2, #3
                beq     r1, r2, case_3          ; r1 == 3?
                bra     .default_case           ; fall through to default

case_0:
                ; handle case 0
                bra     .switch_end
case_1:
                ; handle case 1
                bra     .switch_end
case_2:
                ; handle case 2
                bra     .switch_end
case_3:
                ; handle case 3
                bra     .switch_end
.default_case:
                ; handle default
.switch_end:
```

### Data-table dispatch (for shared handler logic)

For larger switch statements, one practical pattern is a table of constants and a
single handler path. This keeps dispatch explicit while still using table data.

```asm
; Example: map opcode (0..3) to a mode value via table, then branch on mode.
; r1 = opcode index

                move.q  r2, #3
                bhi     r1, r2, .default_case
                lsl.q   r3, r1, #3              ; index * 8 (dc.q entries)
                la      r4, .mode_table
                add.q   r4, r4, r3
                load.q  r5, (r4)                ; mode value for opcode

                ; Branch based on loaded mode value.
                beqz    r5, case_0
                move.q  r6, #1
                beq     r5, r6, case_1
                move.q  r6, #2
                beq     r5, r6, case_2
                bra     case_3

                align   8
.mode_table:
                dc.q    0, 1, 2, 3
```

Notes:
- For small dispatch tables (up to ~8 cases), the cascading `beq` pattern is clean
  and fast. Each test is 2 instructions (16 bytes).
- For larger dispatch sets, use a jump table with `jmp (Rs)` (see below).

### Function Pointers

```asm
; Call a function whose address is in r5.
                la      r5, handler
                jsr     (r5)                    ; push return addr, jump to handler
                ; ... execution continues here after rts
                bra     .done

handler:
                ; function body
                rts
.done:
```

### Jump Tables

```asm
; Dispatch on r1 (0..N) via address table.
; r1 = case index (bounds-checked)

                move.q  r2, #3
                bhi     r1, r2, .default_case   ; unsigned bounds check
                lsl.q   r3, r1, #3              ; index * 8 (64-bit entries)
                la      r4, .jump_table
                add.q   r4, r4, r3
                load.q  r5, (r4)                ; load target address
                jmp     (r5)                    ; jump to case handler

                align   8
.jump_table:
                dc.q    case_0, case_1, case_2, case_3
```

### VTable Dispatch

```asm
; r8 = pointer to object (first field is vtable pointer)
; Call method at vtable offset 16.

                load.q  r1, (r8)                ; r1 = vtable pointer
                jsr     16(r1)                  ; call vtable[2] (offset 16)
                ; returns here after rts
```

---

## 11. Multiply by Constants

Use shift-and-add sequences to multiply by small constants without using `mulu`.
This can be faster for trivial multipliers and avoids tying up the multiplier unit.

```asm
; Multiply r1 by 2:
                lsl.q   r2, r1, #1              ; r2 = r1 * 2

; Multiply r1 by 3:
                lsl.q   r2, r1, #1              ; r2 = r1 * 2
                add.q   r2, r2, r1              ; r2 = r1 * 2 + r1 = r1 * 3

; Multiply r1 by 5:
                lsl.q   r2, r1, #2              ; r2 = r1 * 4
                add.q   r2, r2, r1              ; r2 = r1 * 4 + r1 = r1 * 5

; Multiply r1 by 7:
                lsl.q   r2, r1, #3              ; r2 = r1 * 8
                sub.q   r2, r2, r1              ; r2 = r1 * 8 - r1 = r1 * 7

; Multiply r1 by 10:
                lsl.q   r2, r1, #3              ; r2 = r1 * 8
                lsl.q   r3, r1, #1              ; r3 = r1 * 2
                add.q   r2, r2, r3              ; r2 = r1 * 8 + r1 * 2 = r1 * 10

; Multiply r1 by 320 (VGA mode 13h stride):
                lsl.q   r2, r1, #8              ; r2 = r1 * 256
                lsl.q   r3, r1, #6              ; r3 = r1 * 64
                add.q   r2, r2, r3              ; r2 = r1 * 256 + r1 * 64 = r1 * 320
```

### General decomposition

Any constant can be decomposed as a sum/difference of powers of two. For example:
- `N * 15 = N * 16 - N` (one shift, one sub)
- `N * 12 = N * 8 + N * 4` (two shifts, one add)
- `N * 100 = N * 64 + N * 32 + N * 4` (three shifts, two adds)

Notes:
- For constants with more than 3 terms, `mulu rd, rs, #imm` is likely more compact
  and often simpler; benchmark in hot paths if performance is critical.
- `lsl.q` by 0 is a no-op (effectively a move), so `N * 1` costs nothing.
- These patterns work for both signed and unsigned values since bit-level shifts and
  adds produce the same result in two's complement.

---

## 12. Bit Manipulation

### Test bit N

```asm
; Test if bit 7 of r1 is set. Branch to .bit_set if so.

                and.q   r2, r1, #$80    ; isolate bit 7
                bnez    r2, .bit_set    ; branch if bit was set
```

### Test arbitrary bit N (N in register r3)

```asm
; Test if bit r3 of r1 is set.

                move.q  r2, #1
                lsl.q   r2, r2, r3      ; r2 = 1 << N
                and.q   r4, r1, r2      ; r4 = r1 & (1 << N)
                bnez    r4, .bit_set
```

### Set bit N

```asm
; Set bit 5 of r1.

                or.q    r1, r1, #$20    ; r1 |= (1 << 5)
```

### Set arbitrary bit N (N in register r3)

```asm
                move.q  r2, #1
                lsl.q   r2, r2, r3      ; r2 = 1 << N
                or.q    r1, r1, r2      ; r1 |= (1 << N)
```

### Clear bit N

```asm
; Clear bit 5 of r1.
; Since there is no "and-not" instruction, invert the mask first.

                move.q  r2, #$20        ; mask = 1 << 5
                not.q   r2, r2          ; r2 = ~(1 << 5) = $FFFF_FFFF_FFFF_FFDF
                and.q   r1, r1, r2      ; r1 &= ~(1 << 5)
```

### Clear arbitrary bit N (N in register r3)

```asm
                move.q  r2, #1
                lsl.q   r2, r2, r3      ; r2 = 1 << N
                not.q   r2, r2          ; r2 = ~(1 << N)
                and.q   r1, r1, r2      ; r1 &= ~(1 << N)
```

### Toggle bit N

```asm
; Toggle bit 0 of r1.

                eor.q   r1, r1, #1      ; r1 ^= (1 << 0)
```

### Count set bits (popcount, naive loop)

```asm
; Count set bits in r1, result in r3.
; r1 is destroyed. r2 is scratch.

                move.q  r3, #0          ; count = 0
.popcount_loop:
                beqz    r1, .popcount_done
                and.q   r2, r1, #1      ; isolate lowest bit
                add.q   r3, r3, r2      ; count += lowest bit
                lsr.q   r1, r1, #1      ; shift right by 1
                bra     .popcount_loop
.popcount_done:
```

### Extract bit field

```asm
; Extract bits [11:8] from r1 into r2 (4-bit field).

                lsr.q   r2, r1, #8      ; shift field to bit 0
                and.q   r2, r2, #$F     ; mask to 4 bits
```

Notes:
- Immediate values in `and`/`or`/`eor` are zero-extended 32-bit. For masks wider
  than 32 bits, load the mask into a register first using `li`.
- `not.q` inverts all 64 bits. For size-restricted NOT, use `.b`/`.w`/`.l` suffix.

---

## 13. Stack Frame Example

A complete example of a multi-argument function with local variables allocated on
the stack.

```asm
; int dot_product(int *a, int *b, int count)
; Arguments: r8 = pointer to array a
;            r9 = pointer to array b
;            r10 = element count
; Returns:   r8 = dot product (sum of a[i]*b[i])
;
; Local variables on stack:
;   sp+0  : saved r20 (loop index)
;   sp+8  : saved r19 (accumulator)
;   sp+16 : saved r18 (count)
;   sp+24 : saved r17 (array b pointer)
;   sp+32 : saved r16 (array a pointer)

dot_product:
                ; -- prologue: save callee-saved registers --
                push    r16
                push    r17
                push    r18
                push    r19
                push    r20

                ; -- copy arguments to callee-saved registers --
                move.q  r16, r8         ; r16 = a
                move.q  r17, r9         ; r17 = b
                move.q  r18, r10        ; r18 = count

                ; -- initialize locals --
                move.q  r19, #0         ; accumulator = 0
                move.q  r20, #0         ; index = 0

                ; -- loop body --
.dp_loop:
                bge     r20, r18, .dp_done  ; if index >= count, exit loop

                ; compute byte offset: index * 8 (quad-word elements)
                lsl.q   r1, r20, #3     ; r1 = index * 8

                ; load a[index]
                add.q   r2, r16, r1     ; r2 = &a[index]
                load.q  r3, (r2)        ; r3 = a[index]

                ; load b[index]
                add.q   r2, r17, r1     ; r2 = &b[index]
                load.q  r4, (r2)        ; r4 = b[index]

                ; accumulate: accumulator += a[index] * b[index]
                muls.q  r5, r3, r4      ; r5 = a[index] * b[index] (signed)
                add.q   r19, r19, r5    ; accumulator += product

                ; increment index
                add.q   r20, r20, #1
                bra     .dp_loop

.dp_done:
                ; -- set return value --
                move.q  r8, r19         ; return accumulator in r8

                ; -- epilogue: restore callee-saved registers (reverse order) --
                pop     r20
                pop     r19
                pop     r18
                pop     r17
                pop     r16
                rts

; ---------------------------------------------------------------------------
; Caller example: compute dot product of two 4-element arrays
; ---------------------------------------------------------------------------

main:
                ; set up array a at a known address
                la      r1, array_a
                move.q  r8, r1          ; arg 0: pointer to a

                la      r1, array_b
                move.q  r9, r1          ; arg 1: pointer to b

                move.q  r10, #4         ; arg 2: count = 4

                jsr     dot_product     ; call; result in r8

                ; r8 now holds the dot product:
                ; (1*5 + 2*6 + 3*7 + 4*8) = 5 + 12 + 21 + 32 = 70

                halt

; ---------------------------------------------------------------------------
; Data
; ---------------------------------------------------------------------------

                align   8
array_a:
                dc.q    1, 2, 3, 4
array_b:
                dc.q    5, 6, 7, 8
```

### Stack layout during dot_product execution

```
High addresses (toward STACK_START = $9F000)
  +40  [return address]        <- pushed by jsr
  +32  [saved r16]             <- pushed first
  +24  [saved r17]
  +16  [saved r18]
  +8   [saved r19]
  +0   [saved r20]             <- sp points here
Low addresses
```

Notes:
- The stack grows downward. `push` decrements `sp` by 8, then stores. `pop` loads,
  then increments `sp` by 8.
- `jsr` pushes the return address (PC + 8) onto the stack before branching. `rts`
  pops it and jumps back.
- If you need true stack-allocated local variables (beyond just saving registers),
  subtract from `sp` in the prologue and add back in the epilogue:
  ```asm
  sub.q sp, sp, #32    ; allocate 32 bytes of locals
  ; ... use 0(sp), 8(sp), 16(sp), 24(sp) for locals ...
  add.q sp, sp, #32    ; deallocate locals
  ```
- Keep `sp` 8-byte aligned at all times for quad-word load/store correctness.
