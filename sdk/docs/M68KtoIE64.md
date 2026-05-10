# M68K-to-IE64 Assembly Source Converter (`m68kto64`)

> **Status: in development.** This document is a skeleton. Sections are filled in as the
> transpiler implementation lands, phase by phase. See `.claude/plans/M68KtoIE64plan.md`
> for the engineering plan and TDD gates.

Source-to-source transpiler that converts Motorola m68k (vasm/devpac flavor) assembly
into IE64 assembly that can be assembled by `assembler/ie64asm.go`. Sibling to
[`ie32to64`](ie32to64.md); same single-pass-per-line philosophy, but with full m68k
addressing-mode lowering, flag-fuse peephole, and an emulated 32-bit guest stack so the
transpiled binaries can run on the JIT-capable IE64 core.

## Table of Contents

1. [Overview & motivation](#1-overview--motivation)
2. [Quickstart](#2-quickstart)
3. [CLI reference](#3-cli-reference)
4. [Register-file ABI](#4-register-file-abi)
5. [Stack model](#5-stack-model)
6. [Addressing-mode lowering](#6-addressing-mode-lowering)
7. [Flag model](#7-flag-model)
8. [Instruction reference](#8-instruction-reference)
9. [Directives & macros](#9-directives--macros)
10. [MMIO & equates](#10-mmio--equates)
11. [TRAP / syscall vectors](#11-trap--syscall-vectors)
12. [Limitations & caveats](#12-limitations--caveats)
13. [Worked examples](#13-worked-examples)
14. [Troubleshooting](#14-troubleshooting)
15. [Roadmap](#15-roadmap)

**FPU (Phase 7) sub-sections:**

- [§4.FP — Floating-point register ABI](#4fp--floating-point-register-abi)
- [§6.FP — FPU addressing modes](#6fp--fpu-addressing-modes)
- [§7.FP — FPU shadow model](#7fp--fpu-shadow-model)
- [§8.FP — FPU instruction reference](#8fp--fpu-instruction-reference)
- [§11.FP — Locked syscall # vector table (incl. FTRAPcc)](#11fp--locked-syscall--vector-table)
- [§13.FP — FPU worked examples](#13fp--fpu-worked-examples)
- [§14.FP — FPU troubleshooting](#14fp--fpu-troubleshooting)
- [§15.FP — FPU roadmap](#15fp--fpu-roadmap)

---

## 1. Overview & motivation

*TBD — Phase 0 placeholder.* Why source-level transpile vs running on the M68K core
(JIT only on amd64; IE64 has both amd64 and arm64 JIT). AB3D2 case study. Relationship
to `cmd/ie32to64/`.

## 2. Quickstart

*TBD — Phase 1.*

```bash
# Build (placeholder; target lands in Phase 1)
make m68kto64

# Convert one file
sdk/bin/m68kto64 input.s -o input.ie64.s
sdk/bin/ie64asm input.ie64.s -o input.bin
```

## 3. CLI reference

*TBD — Phase 1.* Flags planned:

| Flag | Purpose |
|------|---------|
| `-o <file>` | Output path (default: `<input>_ie64.s`) |
| `-size .l|.q` | Default size suffix |
| `-no-header` | Omit "Converted from m68k" header |
| `-no-flags-fuse` | Disable CMP/Bcc fuse (debug aid) |
| `-strict` | Error on unfused flag spans / unsupported ops |
| `-include-path <dir>` | Add include search path |
| `-define <K=V>` | Define assembler symbol |

## 4. Register-file ABI

*Finalized in Phase 1.* See `.claude/plans/M68KtoIE64plan.md` "Register File Mapping"
for the canonical table; reproduced here once Phase 1 lands.

| m68k | IE64 | Notes |
|------|------|-------|
| d0–d7 | r1–r8 | data registers |
| a0–a6 | r9–r15 | address registers |
| a7 (m68k SP) | r30 | **emulated 32-bit guest stack** |
| (host SP) | r31 | reserved for transpiler-internal scratch saves |
| ccr/sr | r24–r27 (shadows) | only materialized when fuse fails |
| scratch | r16, r17, r18 | EA computation, mem temps |
| mul/div pair | r19–r23 | 64-bit pair lowering of MULU.L/DIVU.L |

> **Inline IE64 asm rule:** do **not** touch r30/r31 from inline IE64 fragments
> embedded in transpiled m68k. r30 is the emulated guest stack pointer; r31 is the
> IE64 hardwired SP used for transpiler-internal saves.

### §4.FP — Floating-point register ABI

*Phase 7.0 scaffold; bodies filled across Phase 7.1–7.7.*

m68k 68881/68882 FP0–FP7 map onto **even-numbered** IE64 FP registers; odd half is
implicit double-precision storage and must not be referenced separately (per IE64 ISA
§4.6.6).

| m68k FP | IE64 even reg | High-half storage |
|---------|---------------|-------------------|
| FP0 | f0  | f1  |
| FP1 | f2  | f3  |
| FP2 | f4  | f5  |
| FP3 | f6  | f7  |
| FP4 | f8  | f9  |
| FP5 | f10 | f11 |
| FP6 | f12 | f13 |
| FP7 | f14 | f15 |
| FPCR  | IE64 hardware FPCR (`fmovcr`/`fmovcc`) | — |
| FPSR  | IE64 hardware FPSR (sticky exceptions) + r29 ShadowFPCC (cc bits 27:24) | — |
| FPIAR | not exposed; reads return 0 + diagnostic comment | — |

**Inline-asm reservation rule:** transpiled m68k FPU code reserves the **entire f0–f15
file** for guest FP0–FP7. Inline IE64 fragments embedded in transpiled m68k must not
clobber any `f` register. Integer scratch reservations (r16–r29) and r30/r31
unchanged.

**Per-output-file memory-slot reservations** (BSS-style globals, single-thread
guest):

| Symbol | Width | Used by |
|--------|-------|---------|
| `__m68kto64_fpcr_save`     | `dc.q` | FINTRZ FPCR save/restore |
| `__m68kto64_fp_scratch_q`  | `dc.q` | FSCALE bit-pattern memory round-trip |
| `__m68kto64_fp_const_pool` | `dc.s/d` block | FP immediates not in IE64 fmovecr ROM |

Slots are non-reentrant under guest interrupts — see §15.FP roadmap entry.

## 5. Stack model

*TBD — Phase 3.* Emulated 32-bit guest stack on r30; native IE64 push/pop (which use
8-byte slots on r31) are reserved for transpiler internals. LINK/UNLK/JSR/RTS lower to
explicit width-correct sequences. See plan "Critical: m68k stack must be 32-bit-emulated"
for worked examples.

## 6. Addressing-mode lowering

*TBD — Phase 1.* Every m68k addressing mode (`Dn`, `An`, `(An)`, `(An)+`, `-(An)`,
`(d16,An)`, `(d8,An,Xn)`, `(xxx).w/.l`, `(d16,PC)`, `(d8,PC,Xn)`, `#imm`) → IE64
sequence with scratch-reg usage.

### §6.FP — FPU addressing modes

*Phase 7.0 scaffold.*

Most FP addressing modes are integer modes applied to the source/dest of FMOVE and
its arithmetic siblings. New cases:

| Form | Lowering |
|------|----------|
| `FPn` | direct `f(2n)`; double ops accept the even reg, implicitly use the odd half |
| `#imm` (FP) | `fmovecr fd, #idx` if it matches an IE64 ROM entry; else allocate `dc.s`/`dc.d` in `__m68kto64_fp_const_pool` then `la r17, label; fload/dload f(2n), (r17)` |
| `(An)+` / `-(An)` / `(An)` / `(d,An)` / `(d,An,Xn)` / abs / PC-rel | EA via existing integer helpers, then **single: `fload`/`fstore` (4B); double: `dload`/`dstore` (8B, opcode 0x81/0x82)**. Predec/postinc adjusts An by 4 (single) or 8 (double) |
| `FPCR` / `FPSR` | `fmovcr`/`fmovcc` resp. `fmovsr`/`fmovsc` |
| `FPIAR` | reads return 0 + diagnostic; writes silently dropped + diagnostic |

**Size-suffix routing:**

| Suffix | Width | Lowering |
|--------|-------|----------|
| `.B` | 8-bit signed int → FP  | int load + `sext.b` → `fcvtif` |
| `.W` | 16-bit signed int → FP | int load + `sext.w` → `fcvtif` |
| `.L` | 32-bit signed int → FP | `fcvtif fd, rs` |
| `.S` | 32-bit IEEE single | `fload`/`fstore` (4 B) |
| `.D` | 64-bit IEEE double | `dload`/`dstore` (single 8 B transfer; opcode 0x81/0x82). **Not** 2× fload |
| `.X` | 80-bit extended    | degraded to `.D`; `;.X degraded` comment in non-strict; `-strict` errors |
| `.P` | 96-bit packed BCD  | unsupported; `-strict` errors; non-strict emits `; ERROR: .P unsupported` |

**FMOVECR ROM-offset translation** (m68k 7-bit ROM → IE64 4-bit `fmovecr` index, plus
`dc.d` constant-pool fallback): table lands with Phase 7.2.

## 7. Flag model

*TBD — Phase 3.* Two-pass fuse: pass 1 classifies flag-producing/consuming lines;
pass 2 peephole-fuses CMP/TST + Bcc into IE64 register-pair branches. Width
normalization mandatory before signed fused branches (m68k computes flags at .b/.w/.l
width; IE64 compares full 64-bit). Fallback shadow regs r24–r27 only emitted when
downstream code reads them. `-strict` errors on unfused spans.

### §7.FP — FPU shadow model

*Phase 7.0 scaffold.*

**ShadowFPCC (r29)** mirrors the four-bit m68k FPSR cc field (bits 27:24). Bit
layout:

| r29 bit | m68k FPSR bit | Meaning |
|---------|---------------|---------|
| 3 | 27 | N (result sign) |
| 2 | 26 | Z (result zero) |
| 1 | 25 | I (operand was ±Inf) |
| 0 | 24 | NaN (result/operand was NaN) |

**Producers** (FCMP/FTST + every cc-affecting arithmetic op: FADD/FSUB/FMUL/FDIV/
FMOD/FREM/FNEG/FABS/FSQRT/FINT/FINTRZ/FSCALE/FGETMAN/FGETEXP/FSGLDIV/FSGLMUL +
every transcendental) emit a `dcmp`-derived 4-instruction shadow update.

**Consumers**: FBcc, FDBcc, FScc, FTRAPcc, `FMOVE.L FPSR,Dn`.

**Hardware FPSR/FPCR access pattern.** Sticky exception bits (UE, OE, DZ, IO; bits
3:0) live in IE64 hardware FPSR; the cc field lives in ShadowFPCC. FPCR (rounding
mode bits 1:0) lives in IE64 hardware FPCR; no GPR shadow.

**FINTRZ memory-slot save/restore** uses `__m68kto64_fpcr_save`. Sequence:

```
fmovcr  r17                  ; capture current FPCR
store.l r17, __m68kto64_fpcr_save
move.l  r18, #2              ; round-toward-zero mode constant
fmovcc  r18                  ; force RZ
dint    fd, fs               ; rounded integer with RZ
load.l  r17, __m68kto64_fpcr_save
fmovcc  r17                  ; restore caller's FPCR
```

**FPSR ↔ Dn fold (split-on-write, compose-on-read).**

- `FMOVE.L FPSR,Dn` (read): `fmovsr r17` (sticky bits 3:0); `lsl.l r18, ShadowFPCC, #24`; `or.l Dn, r17, r18`.
- `FMOVE.L Dn,FPSR` (write): `lsr.l r17, Dn, #24; and.l ShadowFPCC, r17, #$F` (cc into shadow); `fmovsc Dn` (hardware masks input to bits 3:0; cc bits ignored by hw FPSR).
- `FMOVE.L FPCR,Dn` / `FMOVE.L Dn,FPCR`: direct `fmovcr` / `fmovcc`. No fold.
- `FMOVE.L FPIAR,Dn`: read returns 0; write dropped; diagnostic comment.

**Fuse-skip rule for FPSR writes.** Any line that writes ShadowFPCC by means other
than FCMP/FTST — most importantly `FMOVE.L Dn,FPSR` — breaks adjacency. The FPSR
write clobbers r29 unconditionally per m68k semantics, so a downstream FBcc must
consume the freshly-written shadow, not a stale fcmp result. Fuse fires only when
producer and consumer are both adjacent **and** the producer is `fcmp`/`ftst`. Any
other producer (incl. `fmove.l Dn,fpsr`, `fmovsc`, or any intervening label)
suppresses fuse and routes the FBcc through `emitShadowFBcc`.

**Liveness pass (mirrors integer Phase A).** Backward walk: at each consumer, mark
"FPCC live"; at each producer, record live-set into `fpccLiveAt[i]` and reset; force
"all live" at any label or function entry. Pass-2 emits the 4-instruction shadow
update only when `fpccLiveAt[i]` is set. Forward branches into a sequence force the
producer to emit shadows since the target may be a consumer reached via a
non-elided path.

## 8. Instruction reference

*Filled in Phase 2 (ALU/memory), Phase 3 (control flow), Phase 5 (68020 extras).*
Each entry will be marked ✅ fully covered, ⚠️ semantic caveat, or ❌ unsupported.

### §8.FP — FPU instruction reference

*Phase 7.0 scaffold; per-mnemonic body lands across Phase 7.2–7.6.* Convention:
✅ fully covered (bit-exact within IE64 double-precision limits), ⚠️ semantic caveat
(typically due to extended → double degradation or transcendental approximation),
❌ unsupported.

**Data movement (Phase 7.2):**

| Mnemonic | Forms | Status |
|----------|-------|--------|
| `FMOVE.X FPm,FPn` | dmov | ⚠️ |
| `FMOVE.S/.D <ea>,FPn` and `FPn,<ea>` | fload/fstore, dload/dstore | ✅ |
| `FMOVE.B/.W/.L <ea>,FPn` | int load + sext + fcvtif | ✅ |
| `FMOVE.L FPn,<ea>` (.B/.W/.L) | fcvtfi + masked store | ✅ |
| `FMOVE FPSR/FPCR/FPIAR ↔ Dn` | shadow fold (see §7.FP) | ⚠️ FPIAR drop |
| `FMOVECR FPn,#offset` | ROM-offset table + `dc.d` fallback | ✅ |
| `FMOVEM.X <list>,<ea>` and `<ea>,<list>` | per-element dload/dstore in mask order | ✅ |

**Arithmetic (Phase 7.3):**

| Mnemonic | IE64 lowering | Status |
|----------|---------------|--------|
| FADD / FSUB / FMUL / FDIV | dadd / dsub / dmul / ddiv | ⚠️ extended→double |
| FSGLMUL / FSGLDIV | fmul / fdiv (single) | ✅ |
| FMOD | dmod | ⚠️ |
| FREM | dmod + sign correction | ⚠️ approximate |
| FNEG / FABS / FSQRT | dneg / dabs / dsqrt | ⚠️ |
| FINT | dint | ⚠️ |
| FINTRZ | FPCR save→RZ→dint→restore (memory-slot) | ⚠️ non-reentrant slot |
| FSCALE | exponent bit-pattern through `__m68kto64_fp_scratch_q`, then dmul | ⚠️ approximate |
| FGETEXP / FGETMAN | dlog/dabs synthesis | ⚠️ approximate |

**Comparison + branch (Phase 7.4):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| FCMP / FTST | dcmp r17, fs, ft (+ ShadowFPCC update if live) | ✅ |
| FBcc (×32 cc kinds) | adjacent-fuse with FCMP/FTST → integer Bcc on r17; standalone via ShadowFPCC | ✅ |
| FDBcc Dn,label | DBcc-shaped on ShadowFPCC | ✅ |
| FScc <ea> | Scc-shaped on ShadowFPCC | ✅ |
| FTRAPcc [#imm] | conditional `syscall` against ShadowFPCC; cc occupies syscall # 32–63 | ✅ |

**Transcendentals (Phase 7.5):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| FSIN / FCOS / FTAN / FATAN | direct fsin/fcos/ftan/fatan | ⚠️ |
| FACOS / FASIN | identity via fatan/fsqrt | ⚠️ approximate |
| FCOSH / FSINH / FTANH / FATANH | exp/log identities | ⚠️ approximate |
| FETOX / FETOXM1 | fexp; fexp(x)−1 | ⚠️ near-zero precision loss |
| FLOGN / FLOG10 / FLOG2 / FLOGNP1 | flog (+ const div / +1 prep) | ⚠️ |
| FTENTOX / FTWOTOX | fexp(x · ln k) | ⚠️ |

**Control (Phase 7.6):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| FNOP | `; nop (FPU)` | ✅ |
| FSAVE / FRESTORE | strip + diagnostic; `-strict` errors | ❌ context-switch unsupported |

## 9. Directives & macros

*TBD — Phase 4.* `dc.b/w/l`, `ds.X`, `equ`, `set`, `align`, `incbin`, `include` →
mostly 1:1 with `ie64asm`. `IFD IS_IE` rewritten to `if 1`. `xdef`/`xref` dropped
(single-file flat namespace) with warning if cross-unit linkage was actually needed.
Macros (`\1..\9`) and `rept`/`endr` pass through verbatim.

## 10. MMIO & equates

AB3D2 already uses IE chip equates (e.g. `EXEC_CTRL equ $F2324`). Transpiler treats
these as opaque symbols — no remap table needed. Mirrors `ie32to64` behavior.

## 11. TRAP / syscall vectors

`TRAP #n` → `syscall #n` (`assembler/ie64asm.go:3898`). Locked syscall # range
(disjoint reservation across all m68k exception sources; see §11.FP for the FP
extension):

| syscall # | m68k source | Notes |
|-----------|-------------|-------|
| `#0`–`#15` | `TRAP #n` (instruction-encoded immediate) | direct 1:1 mapping |
| `#16` | integer divide-by-zero (m68k vector 5, relocated) | DIVU/DIVS/DIVU.L/DIVS.L zero-divisor guard |
| `#17` | CHK / CHK2 fail (m68k vector 6, relocated) | bounds-test bltz/bgt fall-through |
| `#18` | TRAPV (m68k vector 7, relocated) | guarded on ShadowV |
| `#32`–`#63` | FTRAPcc (one syscall # per FP cc kind) | see §11.FP |

CHK/TRAPV/divide-by-zero originally landed at `#5/#6/#7` in early Phase 5; relocated
above the `TRAP #n` instruction-encoded range as part of Phase 7.0 to keep
TRAP-instruction-# disjoint from m68k exception-vector-#.

### §11.FP — Locked syscall # vector table

*Phase 7.0 scaffold.*

`FTRAPcc` (Phase 7.4) consumes syscall numbers `#32`–`#63`, one per FP cc kind, in
this order:

| # | cc | # | cc | # | cc | # | cc |
|---|----|----|----|----|----|----|----|
| 32 | F  | 40 | UN  | 48 | SF  | 56 | NLE |
| 33 | EQ | 41 | UEQ | 49 | SEQ | 57 | NLT |
| 34 | OGT| 42 | UGT | 50 | GT  | 58 | NGE |
| 35 | OGE| 43 | UGE | 51 | GE  | 59 | NGT |
| 36 | OLT| 44 | ULT | 52 | LT  | 60 | SNE |
| 37 | OLE| 45 | ULE | 53 | LE  | 61 | ST  |
| 38 | OGL| 46 | NE  | 54 | GL  | 62 | (reserved) |
| 39 | OR | 47 | T   | 55 | GLE | 63 | NGLE |

Reservation is permanent; downstream consumers may pin handlers to specific numbers.

## 12. Limitations & caveats

*TBD.* Self-modifying code, multi-unit linkage, BCD/CAS edge cases, performance
bound from shadow flags.

## 13. Worked examples

*TBD — Phase 6.* `controlloop.s` snippet → IE64; AB3D2 hot-loop case study.

### §13.FP — FPU worked examples

Side-by-side m68k 68881 source vs IE64 lowering produced by `m68kto64`. All
three examples round-trip through `sdk/bin/ie64asm` cleanly; smoke-validated
in `cmd/m68kto64/golden/fpu_basic.s`.

#### 1. Vector dot product

m68k FPU source:

```asm
; a0 → vec1 (4 doubles), a1 → vec2 (4 doubles), result in FP0
        fmove.d (a0)+,fp0       ; fp0 = a[0]
        fmove.d (a1)+,fp1       ; fp1 = b[0]
        fmul.x  fp1,fp0         ; fp0 = a[0]*b[0]
        moveq   #3,d0
.loop:
        fmove.d (a0)+,fp2
        fmove.d (a1)+,fp3
        fmul.x  fp3,fp2
        fadd.x  fp2,fp0
        dbra    d0,.loop
        rts
```

IE64 lowering (key fragments):

```asm
        dload   f0, (r9)            ; fmove.d (a0)+,fp0
        add.l   r9, r9, #8
        dload   f2, (r10)           ; fmove.d (a1)+,fp1
        add.l   r10, r10, #8
        dmul    f0, f0, f2          ; fp0 *= fp1
        ; (no shadow update — no downstream FBcc consumer in this routine)
.loop:
        dload   f4, (r9); add.l r9, r9, #8
        dload   f6, (r10); add.l r10, r10, #8
        dmul    f4, f4, f6
        dadd    f0, f0, f4
        sub.l   r1, r1, #1
        sext.w  ScrTmp1, r1
        bne     ScrTmp1, #-1, .loop
        rts
```

#### 2. Matrix multiply (3×3, FMOVEM stack save)

m68k FPU source:

```asm
        fmovem.x fp2-fp7,-(sp)   ; spill caller-saved FPn
        ; … inner loop computes one row of C := A · B …
        fmovem.x (sp)+,fp2-fp7
        rts
```

IE64 lowering — FMOVEM expands to six `dstore` / `dload` sequences against
the emulated 32-bit guest stack (r30):

```asm
        sub.l   r30, r30, #8; dstore f14, (r30)   ; fp7
        sub.l   r30, r30, #8; dstore f12, (r30)   ; fp6
        sub.l   r30, r30, #8; dstore f10, (r30)   ; fp5
        sub.l   r30, r30, #8; dstore f8,  (r30)   ; fp4
        sub.l   r30, r30, #8; dstore f6,  (r30)   ; fp3
        sub.l   r30, r30, #8; dstore f4,  (r30)   ; fp2
        ; … inner loop …
        dload   f4,  (r30); add.l r30, r30, #8
        dload   f6,  (r30); add.l r30, r30, #8
        dload   f8,  (r30); add.l r30, r30, #8
        dload   f10, (r30); add.l r30, r30, #8
        dload   f12, (r30); add.l r30, r30, #8
        dload   f14, (r30); add.l r30, r30, #8
```

#### 3. Sin-table generator

m68k FPU source:

```asm
; a0 → output table (256 doubles), fp0 = step, fp1 = accumulator
        fmove.x  fp0,fp2            ; preserve step
        fmove.d  #0.0,fp1
        moveq    #255,d0
.gen:
        fsin.x   fp1,fp3
        fmove.d  fp3,(a0)+
        fadd.x   fp2,fp1
        dbra     d0,.gen
        rts
```

IE64 lowering for the hot fragment:

```asm
.gen:
        ; fsin.x fp1,fp3
        fcvtds  f6, f2              ; narrow fp1 → single for IE64 fsin
        fsin    f6, f6
        fcvtsd  f6, f6              ; back to double (fp3)
        ; (shadow update elided — no FP cc consumer in body)
        dstore  f6, (r9)            ; fmove.d fp3,(a0)+
        add.l   r9, r9, #8
        dadd    f2, f2, f4          ; fp1 += fp2
        sub.l   r1, r1, #1
        sext.w  ScrTmp1, r1
        bne     ScrTmp1, #-1, .gen
        rts
```

All three goldens validate that the FMOVEM mask order, FCMP+FBcc fuse, and
ShadowFPCC liveness elision interact cleanly across typical FPU hot loops.

## 14. Troubleshooting

*TBD — Phase 6.* Common assembler errors after conversion, fuse-failure diagnostics,
`-strict` output reading.

### §14.FP — FPU troubleshooting

The most common class of post-transpile errors arises from precision-mixing
between the single- and double-precision halves of the IE64 FP file. Every FP
register holds either a 32-bit single value in its low half or — when accessed
via a `d*` opcode — a 64-bit double spanning the even register and its
implicit odd half. A `.S` source operand feeding a `dadd` without an
intervening `fcvtsd` widen will be reinterpreted bit-for-bit as the low half
of a double, producing garbage. The transpiler's `materializeFPSrc` helper
inserts the widen automatically when the size suffix declares the value
single, so the failure usually surfaces only when hand-written inline IE64
asm is glued onto transpiler output. Audit the trail for `fcvtsd` /
`fcvtds` at every cross-precision boundary.

A related diagnostic is the IE64 assembler rejecting `f1`, `f3`, or any odd
register as the destination of a `d*` opcode. IE64 ISA §4.6.6 mandates even
operand selection for double-precision ops because the odd register provides
the high-half storage. If the assembler reports `dadd f1, ...`, the upstream
source has either named the wrong m68k FPn (FP0 → f0, FP1 → f2, …, FP7 →
f14 per §4.FP) or has inadvertently written through `ScrFP1` (f10 reserved)
or `ScrFP2` (f12 reserved). Re-map against the table in §4.FP and ensure no
inline IE64 fragment touches `f0`–`f15`.

`.X` extended-precision divergence against a 68881 reference is expected
behaviour, not a bug. IE64 has no 80-bit path, so the transpiler degrades
every `.X` op to `.D`; precision visible to the guest is therefore IEEE 754
binary64 (53 bits of mantissa) rather than 80-bit extended's 64 bits. For
numeric-sensitive workloads, set `-strict` during validation so the
transpiler errors on every degradation; this surfaces the exact lines that
need either re-formulation or a documented ε tolerance.

When an FBcc appears to ignore the most recent FCMP/FTST, the cause is
almost always a fuse-skip suppression. The peephole only fires when the
producer is `fcmp` or `ftst` *and* the consumer is the very next line *and*
neither line carries a label *and* the consumer is one of the
non-NaN-aware cc kinds (eq/ne/lt/le/gt/ge plus the always-T/F and signaling
variants). Any other shape routes the consumer through the
`emitShadowFBccTest` standalone path, which reads the most recently
composed `r29`. An intervening `fmove.l Dn,fpsr` or `fmovsc` clobbers `r29`
per m68k semantics, so a downstream FBcc must read the post-write shadow,
not a stale fcmp result. Check the emit trail: a fused pair shows `dcmp r17,
...` immediately followed by a plain integer Bcc on `r17`; a standalone
shadow path shows the `; bit2 (Z)` / `; bit3 (N)` comment markers.

The FINTRZ memory slot (`__m68kto64_fpcr_save`) is single-instance per
output file. Programs that route FPU code through guest interrupt handlers
must wrap handler entry/exit with manual FPCR save/restore — otherwise an
interrupt fired between the FINTRZ save and restore observes the transient
round-toward-zero FPCR value. AB3D2-class single-thread guests are
unaffected. The same caveat applies to `__m68kto64_fp_scratch_q` used by
FSCALE and FGETMAN.

## 15. Roadmap

Status, May 2026:

- **Phase 0 — done.** Docs scaffold + cross-refs.
- **Phase 1 — done.** Lexer / operand parser / regmap, TDD.
- **Phase 2 — done.** Straight-line ALU + memory lowering (MOVE/ADD/SUB/AND/OR/EOR/MULU/MULS/DIVU/DIVS/NEG/NOT/CLR/LSL/LSR/ASL/ASR/ROL/ROR/EXT/EXTB/SWAP/ST/SF/LEA/MOVEA/MOVEQ).
- **Phase 3 — done.** CMP/TST + Bcc fuse (eq/ne/lt/ge/gt/le/hi/ls/mi/pl), BRA/BSR/JSR/RTS/JMP on emulated 32-bit guest stack (r30), MOVEM (predec/postinc/EA), LINK/UNLK, DBRA/DBF.
- **Phase 4 — done.** Directives + macros + conditional asm: dc/ds/equ/include/incbin/section/org pass-through; `IFD IS_IE` → `if 1`; `IFND IS_IE` → `if 0`; `endc` → `endif`; `even` → `align 2`; `xdef`/`xref`/`public`/`global`/`extern` dropped with diagnostic; macro/rept bodies pass through verbatim (so `\1..\9` arg subs survive).
- **Phase 5 — done.** TRAP→syscall (locked range #0–#15 instruction-encoded), CHK→bounds+syscall #17 (relocated), TRAPV→syscall #18 (relocated), integer divide-by-zero→syscall #16 (relocated), MOVEC stripped, MULU.L/MULS.L/DIVU.L/DIVS.L 64-bit pair, BFEXTU/BFEXTS.
- **Phase A (Phase-3 completion) — done.** Shadow CCR maintained: r24=N (sign-extended), r25=Z (width-masked), r26=C (0/1), r27=V (0/1). Producers (CMP/TST/ADD/SUB/AND/OR/EOR/NOT/NEG/CLR/MOVE/MOVEQ/MULU/MULS/DIVU/DIVS/EXT/EXTB/SWAP/BTST and the bit-field family) emit shadow updates per m68k semantics; ADD/SUB/CMP/NEG capture pre-op operands for full C/V. Standalone Bcc (all 14 cc), DBcc (all variants), and Scc (all variants) consume shadows.
- **Phase B (Phase-5 remainder) — done.** TRAPV against shadow V, ABCD/SBCD/NBCD (Dn,Dn and -(Ay),-(Ax) forms), PACK/UNPK (both forms), CAS (non-atomic load-cmp-store fallback), BFINS/BFCLR/BFSET/BFCHG/BFTST/BFFFO on Dn destinations.
- **Phase 6 — multi-file wrapper shipped + AB3D2 dry-run clean.** All 88 AB3D2 `.s`/`.i` files transpile with **zero ERRORs and zero FUSE-MISS** (was 898 pre-shadow). `cmd/m68kto64/kmake.sh` (installed as `sdk/bin/m68kto64-kmake`) concatenates multi-file builds and invokes `ie64asm` with `-I` paths.
- **Phase 7 — done.** 68881/68882 FPU coprocessor support: FP0–FP7 → f0/f2/.../f14 even-pair ABI, ShadowFPCC at r29, FCMP+FBcc adjacent fuse, ShadowFPCC liveness pass, FPSR↔Dn split-on-write/compose-on-read fold, FINTRZ memory-slot save/restore, FSCALE+FGETMAN exponent-bit-pattern round-trip, all 32 FP cc kinds (FBcc/FDBcc/FScc/FTRAPcc), full transcendental set via single-precision IE64 ops + identities, FNOP/FSAVE/FRESTORE/.P diagnostics, locked syscall range #16/#17/#18 + #32–#63 (FTRAPcc).

### Round-trip verified

- Four checked-in goldens (`cmd/m68kto64/golden/{arith_basic,control_flow,shadow_ccr,fpu_basic}.s`) transpile and assemble cleanly through `sdk/bin/ie64asm`, producing non-zero binaries. Harness: `TestGolden_RoundTrip` in `cmd/m68kto64/golden_test.go`.
- All 88 AB3D2 source files transpile with **0 ERRORs and 0 FUSE-MISS**. `kmake.sh` concat path validated on single-file builds.
- Statement coverage: **100%** of `cmd/m68kto64/` package (every function and branch covered by the test suite, including direct-invoke tests for defensive error paths).
- Numeric differential validated: `TestM68KFPU_NumericDifferential_RuntimeVsHostMath` runs a transpiled+assembled FPU program on a real `MachineBus` + `CPU64`, reads back 5 transcendental results from guest memory, and confirms agreement with host `math.Sin/Cos/Exp/Log/Sqrt` within ε=1e-6 (Δ profile 0 to 8.3e-08, reflecting IE64 single-precision native ops widened to double).

### Genuinely out of scope for transpiler-only ship

1. **AB3D2 redux-high end-to-end boot** — `diag_redux_smoke.ies` smoke on IE64 core. Requires runtime debugging of the running binary, not transpiler code work.
2. **Harte 68020 conformance harness** — Harte test data (`testdata/680x0/`) is raw machine code, so wiring it requires a m68k disassembler step before transpile. The transpiler operates on assembly source, not bytes; conformance via Harte therefore needs an external chain (vasm round-trip or musashi-driven differential). Deferred until that chain is built.
3. **Differential vs M68K core** — same dependency on a m68k assembler chain (vasm/devpac) to produce the reference binary the IE64 transpilation is compared to. Plan §TDD acknowledges this is built on top of the transpiler.
4. **Performance pass on `hires.s`** — shadow-flag overhead + fuse-coverage measurements; needs binary running on IE64 core. Blocked on (1).
5. **Multi-file AB3D2 build** — symbols like `Plr1_Data` are defined via `DCLC` macros expanded at assemble time across all units. Full build needs `macros.i` + all `bss/*.s` concatenated via `kmake.sh`; the wrapper exists but the build manifest does not.

### §15.FP — FPU roadmap

Phase 7 finalises the 68881/68882 FPU lowering contract. The m68k FP0–FP7
register file maps to the even-numbered IE64 FP registers (f0, f2, ...,
f14) with the adjacent odd register reserved as the implicit
double-precision high-half storage. ShadowFPCC at r29 mirrors the m68k FPSR
condition-code field (bits 27:24) and is composed from the integer result
of the most recent `dcmp` plus the hardware FPSR NaN bit. The split-on-write
/ compose-on-read FPSR↔Dn fold (documented in §7.FP) decomposes the
unified 32-bit m68k FPSR view into its cc-bit half (in ShadowFPCC) and its
sticky-exception half (in the IE64 hardware FPSR). FINTRZ saves and
restores the host FPCR through a transpiler-private memory slot
(`__m68kto64_fpcr_save`); FSCALE and FGETMAN construct IEEE 754 exponent
bit patterns through `__m68kto64_fp_scratch_q`. The locked syscall vector
range reserves `#16` for integer divide-by-zero, `#17` for CHK failure,
`#18` for TRAPV, and `#32`–`#63` for FTRAPcc — one per FP cc kind, in the
order fixed in §11.FP.

Several capabilities are deferred or approximated. The FPIAR is not
exposed by IE64; reads return zero and writes are dropped with a
diagnostic. Any m68k exception-recovery code that walks FPIAR to restart
a faulting floating-point instruction will not function correctly under
the transpiler — this is the single largest semantic deviation from a
real 68881, and applications that rely on restartable-fault resume must
be re-architected. The 96-bit packed BCD `.P` size suffix is unsupported
(emits an error in `-strict` and a diagnostic in non-strict mode); the
format is used almost exclusively for human-readable printf-style output
in legacy m68k programs and is straightforward to replace with a
hand-rolled decimal conversion when needed. FSAVE and FRESTORE strip with
a diagnostic because IE64 exposes no FPU state-context register file; any
program that relies on these for context-switch save/restore will not
work under the transpiler.

The 80-bit extended-precision pipeline collapses to IEEE 754 binary64
double throughout. `.X` size operands degrade with a `;.X degraded to .D`
comment in non-strict mode and an error under `-strict`. Precision-
sensitive workloads should validate against a doubled-precision ε rather
than against bit-exact 68881 output. FSCALE, FGETEXP, and FGETMAN are
approximations: FSCALE uses an IEEE 754 exponent bit-pattern memory
round-trip (no `dmovi` in IE64), FGETEXP synthesises `floor(log2(|x|))`
via `flog` and multiplication by the precomputed `1/ln(2)` constant from
the FP constant pool, and FGETMAN computes `x / 2^fgetexp(x)` via the
same exponent-bit-pattern trick as FSCALE. FREM is synthesised through
`dmod` with a sign correction and is not bit-exact against hardware 68881;
documented for validation harnesses that need an exact reference. The
transcendental family (FSIN, FCOS, FTAN, FATAN, FEtoX, FLOGN) routes
through IE64's single-precision native ops with `fcvtsd` / `fcvtds`
narrow/widen wrappers; the remaining transcendentals (FACOS, FASIN, the
hyperbolics, FLOG10, FLOG2, FLOGNP1, FTENTOX, FTWOTOX) are synthesised
via the standard mathematical identities.

The single open implementation gap is memory-slot reentrancy under guest
interrupts. `__m68kto64_fpcr_save` and `__m68kto64_fp_scratch_q` are
single-instance globals per output file; the transpiler emits no
save/restore of these slots across guest interrupt entry and exit. A
program where an interrupt handler executes FINTRZ, FSCALE, or FGETMAN
while the main line is mid-sequence will corrupt the slot and observe the
transient FPCR or exponent value. AB3D2-class single-thread targets are
unaffected. Multi-context guests must currently wrap handler prolog/epilog
with manual slot save/restore. Lifting this restriction requires an
interrupt-aware codegen pass that emits per-handler save/restore stubs;
this is the natural Phase 7 successor work but is out of scope for the
current deliverable.

Phase 7 is otherwise content-complete: every m68k FPU mnemonic lowers to
IE64; every FP cc kind either fuses with an adjacent FCMP/FTST or
consumes ShadowFPCC through the standalone-test path; all eight
documentation sub-sections (§4.FP through §15.FP) are filled; the
`cmd/m68kto64/fpu*.go` files maintain 100% Go statement coverage in the
test suite; and the `golden/fpu_basic.s` round-trip exercise confirms
end-to-end transpile → ie64asm conformance.

See [`ie32to64.md`](ie32to64.md) for the sibling IE32→IE64 transpiler.
