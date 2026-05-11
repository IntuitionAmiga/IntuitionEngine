# M68K-to-IE64 Assembly Source Converter (`m68kto64`)

> **Status: shipped (Phases 0–7 complete).** Integer (68000 + 68020) and 68881/68882 FPU
> coverage are content-complete. Round-trip verified against `sdk/bin/ie64asm` on the
> four checked-in goldens (`arith_basic`, `control_flow`, `shadow_ccr`, `fpu_basic`)
> and on multi-file 68k inputs concatenated through `sdk/scripts/ab3d2/kmake.sh`. See
> `.claude/plans/M68KtoIE64plan.md` for the engineering plan and TDD gates and §15 for
> the live status / open gaps.

> **Changelog (vasm/devpac preprocessor pass).** Phases A–G add a full
> vasm/devpac preprocessor in front of the lowerer: include resolution with
> `-I` paths and cycle guarding (Phase D); `-D NAME[=VAL]` symbol seeding with
> equ/set/= capture and a recursive-descent expression evaluator (Phase B);
> generic `if` / `ifd` / `ifnd` / `ifeq` / `ifne` plus the full `elseif*`
> family with first-true latch, default Model A (wrappers preserved) and
> opt-in Model B via `-strip-cond` (Phase C); macro / endm / mexit / rept /
> endr with `\1..\9` and globally-monotonic `\@` (Phase E); `section`
> directive dropped (Phase F); env-gated real-world corpus smoke test
> (Phase G).

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

`m68kto64` exists to put an existing m68k assembly codebase onto the IE64 core without
re-architecting it. The IE64 core ships two JIT backends: a heavily-optimized amd64
backend and an immature arm64 backend. The M68K core has an amd64 JIT only and no
arm64 JIT path at all. Source-level transpile to IE64 therefore extends portable
reach (the arm64 IE64 backend exists where the M68K backend does not — a portability
win, not a peak-performance win against amd64) and unlocks the IE64 JIT register file
and 64-bit ALU width for pre-existing 68k programs without intermediate dynamic
translation. On amd64 hosts, the mature IE64 backend is also the faster execution
target than the M68K JIT for non-trivial workloads.

The converter is single-pass-per-line with auxiliary peephole and liveness passes for
the flag model and the FPU shadow. It does not interpret, link, or relocate; it emits
IE64 assembly source consumed by `assembler/ie64asm.go`. Multi-file inputs are handled
by concatenating units through the `kmake.sh` wrapper before transpile.

This converter is the m68k sibling of [`ie32to64`](ie32to64.md). The integer-side
philosophy mirrors `ie32to64` (line-by-line emit, opaque MMIO equates); the additions
beyond `ie32to64` are full m68k addressing-mode lowering, the CMP/Bcc fuse + shadow CCR
fallback (§7), an emulated 32-bit guest stack on r30 (§5), and the 68881/68882 FPU
contract documented in the §*.FP sub-sections.

## 2. Quickstart

```bash
# Build
make m68kto64

# Convert one file
sdk/bin/m68kto64 input.s -o input.ie64.s
sdk/bin/ie64asm input.ie64.s -o input.bin

# Multi-file project (concatenate via kmake.sh wrapper)
sdk/bin/m68kto64-kmake -I include/ -o project.bin src/*.s
```

## 3. CLI reference

```text
Usage: m68kto64 [options] input.s
```

| Flag | Purpose |
|------|---------|
| `-o <file>` | Output path (default: `<input>_ie64.s`) |
| `-size .l\|.q` | Default size suffix when an input mnemonic carries none (default `.l`) |
| `-no-header` | Omit the `; Converted from m68k by m68kto64` header line |
| `-no-flags-fuse` | Disable CMP/TST + Bcc adjacent-fuse (debug aid; forces shadow CCR path) |
| `-strict` | Error on unfused flag spans, `.X`/`.P` size degradation, FSAVE/FRESTORE, and other approximated/unsupported ops |
| `-I <dir>` | (scaffolded; activated Phase D) Add directory to `include` search path; repeatable |
| `-D NAME[=VALUE]` | (scaffolded; activated Phase B) Define symbol for the preprocessor; repeatable. Value parses as `$hex`, `0x...`, `%bin`, decimal. Whitespace around `=` rejected. Bare `-D NAME` → 1 |
| `-strip-cond` | (scaffolded; activated Phase C) Strip `if`/`else`/`endif` wrappers from output (Model B). Default off — wrappers preserved (Model A) |
| `-max-macro-recurs N` | (scaffolded; activated Phase E) Max macro expansion depth (default 1000, vasm-compatible) |
| `-Werror-unknown-mnemonic` | (scaffolded; activated Phase E) Treat unknown mnemonics as errors (default on; `=false` restores legacy passthrough) |
| `-no-default-seeds` | (scaffolded; activated Phase B) Suppress IE-convenience symbol seeds (currently `IS_IE=1`) for vasm-pure behavior |

Generic invocation (no kmake wrapper):

```text
m68kto64 -I include/ -I include/shared -D DEBUG=1 src/main.s
ie64asm -I include/ src/main_ie64.s -o main.ie64
```

See §9 for the per-phase activation status of each preproc flag.

## 4. Register-file ABI

| m68k | IE64 | Notes |
|------|------|-------|
| d0–d7 | r1–r8 | data registers |
| a0–a6 | r9–r15 | address registers |
| a7 (m68k SP) | r30 | **emulated 32-bit guest stack** (see §5) |
| (host SP) | r31 | reserved for IE64-side internal use; do not touch from inline |
| ccr (NZCV shadow) | r24/r25/r26/r27 | only materialized when fuse fails — see §7 |
| EA / mem scratch | r16 (ScrEA), r17 (ScrV1), r18 (ScrV2) | EA computation, mem temps, immediates |
| aux scratch | r19 (ScrAux), r20 (ScrDC) | partial-update + DBcc/condition materialization; doubles as mul/div pair lo/hi for MULU.L / MULS.L / DIVU.L / DIVS.L |
| pre-op snapshot | r21 (ShadowSnap) | source operand capture for shadow C / V |
| shadow helpers | r22 (ShadowTmp1), r23 (ShadowTmp2) | shadow-update internal scratch |
| ShadowX | r28 | m68k X (extend) flag — separate from C; ABCD/SBCD/NBCD read it as chain-in carry; logical/MOVE ops clear C but leave X unchanged |
| ShadowFPCC | r29 | mirrors m68k FPSR cc field (bits 27:24) — see §7.FP |

> **Inline IE64 asm rule (integer):** do **not** touch r16–r29 or r30/r31 from inline
> IE64 fragments embedded in transpiled m68k. r30 is the emulated guest SP; r31 is the
> IE64 hardwired SP; r16–r23 are EA / aux / shadow scratch; r24–r27 are CCR shadows;
> r28 is ShadowX; r29 is ShadowFPCC.

### §4.FP — Floating-point register ABI

m68k 68881/68882 FP0–FP7 map onto **even-numbered** IE64 FP registers; the odd half is
implicit double-precision storage and must not be referenced separately (per IE64 ISA
§4.6.6).

| m68k FP | IE64 even reg | High-half storage | Notes |
|---------|---------------|-------------------|-------|
| FP0 | f0  | f1  | |
| FP1 | f2  | f3  | |
| FP2 | f4  | f5  | |
| FP3 | f6  | f7  | |
| FP4 | f8  | f9  | |
| FP5 | f10 | f11 | **also `ScrFP1` — clobbered by FTST/FSCALE/FGETEXP/FGETMAN** |
| FP6 | f12 | f13 | **also `ScrFP2` — clobbered by FTST/FSCALE/FGETEXP** |
| FP7 | f14 | f15 | |
| FPCR  | IE64 hardware FPCR (`fmovcr`/`fmovcc`) | — | direct |
| FPSR (sticky) | IE64 hardware FPSR (`fmovsr`/`fmovsc`) | — | exception bits 3:0 |
| FPSR (cc field) | r29 (ShadowFPCC) | — | bits 27:24 — see §7.FP |
| FPIAR | not exposed; reads return 0 + diagnostic comment | — | writes silently dropped |

**FP5/FP6 scratch overlay.** The transpiler reuses f10 (FP5) and f12 (FP6) as
scratch for the synthesised ops listed above. Programs that hold live values in
FP5 or FP6 across an FTST/FSCALE/FGETEXP/FGETMAN sequence will observe corruption.
Either spill FP5/FP6 around those ops, or restrict floating-point register use to
FP0–FP4 + FP7 in transpiled code paths. Lifting this restriction is the single
open FPU implementation gap (see §15.FP).

**Inline-asm reservation rule:** transpiled m68k FPU code reserves the entire f0–f15
file: even regs (f0/f2/.../f14) hold guest FP0–FP7, odd regs (f1/f3/.../f15) hold the
implicit double-precision high halves, and f10/f12 additionally serve as `ScrFP1`/
`ScrFP2` scratch (overlaid on FP5/FP6 storage). Inline IE64 fragments embedded in
transpiled m68k must not clobber any `f` register. Integer scratch reservations
(r16–r29) and r30/r31 unchanged.

**Per-output-file memory-slot reservations** (BSS-style globals, single-thread
guest):

| Symbol | Width | Used by |
|--------|-------|---------|
| `__m68kto64_fpcr_save`     | `dc.q` | FINTRZ FPCR save/restore |
| `__m68kto64_fp_scratch_q`  | `dc.q` | FSCALE bit-pattern memory round-trip |
| `__m68kto64_fp_const_pool` | `dc.s/d` block | FP immediates not in IE64 fmovecr ROM |

Slots are non-reentrant under guest interrupts — see §15.FP roadmap entry.

## 5. Stack model

m68k a7 (the m68k SP) maps to IE64 r30 and operates as a **32-bit-emulated guest stack**.
Native IE64 push/pop opcodes use 8-byte slots on r31 and are reserved for transpiler
internals; they are never emitted for guest stack ops.

Every guest stack op lowers to an explicit width-correct sequence:

| m68k | IE64 lowering |
|------|---------------|
| push (`-(a7)` write) | `sub.l r30, r30, #4; store.l <src>, (r30)` (or `#2` / `store.w` for `.w`) |
| pop  (`(a7)+` read)  | `load.l <dst>, (r30); add.l r30, r30, #4` |
| `JSR target` | push 32-bit return label; `bra target` (return label is auto-generated `__m68kto64_bsr_ret_N`) |
| `BSR target` | identical to `JSR` |
| `RTS` | `load.l ScrV1, (r30); add.l r30, r30, #4; jmp (ScrV1)` |
| `LINK an,#disp` | push An; `move.l An, r30`; `add.l r30, r30, #disp` (disp is m68k word-signed) |
| `UNLK an` | `move.l r30, An`; pop An |
| `MOVEM.W <list>,-(a7)` / `MOVEM.L <list>,-(a7)` | predec spill, descending mask order, 2 / 4-byte slots respectively |
| `MOVEM.W (a7)+,<list>` / `MOVEM.L (a7)+,<list>` | postinc reload, ascending mask order, 2 / 4-byte slots; `.W` reloads sign-extend into Dn |

Stack slot width is 4 bytes (m68k 32-bit ABI). FPU `MOVEM.X` against the guest stack
expands per-element to `dstore` / `dload` 8-byte slots — `.X` operands degrade to `.D`
per §6.FP, so each element consumes 8 bytes, not 12.

## 6. Addressing-mode lowering

Every m68k addressing mode lowers to an IE64 sequence using the EA scratch trio
(`r16`/`r17`/`r18`). The EA computation always lands in `ScrEA` (r16) when an indirect
form is used; immediates and direct register forms are inlined.

| m68k mode | IE64 lowering (read into Rs, write from Rd) |
|-----------|---------------------------------------------|
| `Dn` | direct map (r1–r8) |
| `An` | direct map (r9–r15) |
| `(An)` | `load.<sz> Rd, (An)` / `store.<sz> Rs, (An)` |
| `(An)+` | EA = An; `load/store`; `add.l An, An, #<sz>` |
| `-(An)` | `sub.l An, An, #<sz>`; `load/store` at `(An)` |
| `(d16,An)` | `add.l ScrEA, An, #d16`; access via `(ScrEA)` |
| `(d8,An,Xn)` | `<sext.w/.l Xn>` if needed; `add.l ScrEA, An, Xn`; `add.l ScrEA, ScrEA, #d8` |
| `(xxx).w` | sign-extend word to 32; `move.l ScrEA, #...`; `(ScrEA)` |
| `(xxx).l` | `move.l ScrEA, #addr`; `(ScrEA)` |
| `(d16,PC)` | resolved at transpile time — `la ScrEA, label` |
| `(d8,PC,Xn)` | `la ScrEA, label`; `add.l ScrEA, ScrEA, Xn`; `add.l ScrEA, ScrEA, #d8` |
| `#imm` | `move.l ScrV1, #imm` and feed ScrV1 into the op |

`<sz>` resolves from the m68k size suffix: `.b` → 1, `.w` → 2, `.l` → 4. Predec/postinc
on `(a7)` always uses 4 (m68k 32-bit ABI) regardless of the suffix on the operation,
matching real-hardware 68k behaviour.

### §6.FP — FPU addressing modes

Most FP addressing modes are integer modes applied to the source/dest of FMOVE and
its arithmetic siblings. New cases:

| Form | Lowering |
|------|----------|
| `FPn` | direct `f(2n)`; double ops accept the even reg, implicitly use the odd half |
| `#imm` (FP) | `fmovecr fd, #idx` if it matches an IE64 ROM entry; else allocate `dc.s`/`dc.d` in `__m68kto64_fp_const_pool` then `la r17, label; fload/dload f(2n), (r17)` |
| `(An)+` / `-(An)` / `(An)` / `(d,An)` / `(d,An,Xn)` / abs / PC-rel | EA via existing integer helpers, then **single: `fload`/`fstore` (4 B); double: `dload`/`dstore` (8 B, opcode 0x81/0x82)**. Predec/postinc adjusts An by 4 (single) or 8 (double) |
| `FPCR` | `fmovcr` (read) / `fmovcc` (write) |
| `FPSR` (sticky bits 3:0) | `fmovsr` (read) / `fmovsc` (write) |
| `FPSR` (cc bits 27:24) | r29 (ShadowFPCC) — see split-on-write/compose-on-read fold in §7.FP |
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

**FMOVECR ROM-offset translation.** A static table maps the 7-bit m68k FMOVECR ROM
offset to the 4-bit IE64 `fmovecr` index where the constant exists in IE64 ROM; the
remaining offsets fall back to a `dc.d` constant-pool entry.

## 7. Flag model

The integer flag model uses a two-pass design:

1. **Liveness / fuse pass.** Backward walk classifies every line as flag-producer,
   flag-consumer, or neither. Adjacent producer/consumer pairs (CMP/TST + Bcc with no
   intervening label, no intervening flag-clobber) are marked for **fuse**: lower as a
   single IE64 register-pair branch and skip shadow emission. Spans that fail fuse mark
   the producer line as "shadows live" so pass 2 emits the full shadow update.

2. **Emit pass.** Producers emit the IE64 instruction; if shadows are live they also
   emit the four `r24`/`r25`/`r26`/`r27` updates per m68k semantics. Consumers either
   take the fused integer Bcc form or read the shadow regs through the standalone-test
   path.

Width normalisation is mandatory before signed fused branches: m68k computes flags at
.b/.w/.l width, IE64 compares full 64-bit. The fused emit chains a `sext.b`/`sext.w`
through ScrV1 before the integer Bcc when the producer was sub-32-bit.

`-no-flags-fuse` forces every span through the shadow path (debug aid). `-strict` errors
on any consumer that finds itself reading a shadow that was never written upstream.

**Shadow CCR layout:**

| IE64 reg | m68k bit | Encoding |
|----------|----------|----------|
| r24 | N | sign-extended last result (test against 0 with `bltz`/`bgez`) |
| r25 | Z | width-masked last result (test against 0 with `beqz`/`bnez`) |
| r26 | C | 0 / 1 (carry-out of unsigned add/sub at the m68k width) |
| r27 | V | 0 / 1 (signed-overflow indicator, computed from pre-op snapshot in r21) |
| r28 | X | 0 / 1 (extend bit). Tracks C for arithmetic ops; left unchanged by logical/MOVE ops; chain-in carry source for ABCD/SBCD/NBCD. ADDX/SUBX/NEGX/ROXL/ROXR consumers are not implemented (see §8). |

Producers (CMP/TST/ADD/SUB/AND/OR/EOR/NOT/NEG/CLR/MOVE/MOVEQ/MULU/MULS/DIVU/DIVS/EXT/
EXTB/SWAP/BTST and the bit-field family) emit shadows per m68k semantics; ADD/SUB/CMP/
NEG capture the pre-op destination into r21 (ShadowSnap) for full C/V. Standalone Bcc
(all 14 cc), DBcc (all variants), and Scc (all variants) consume shadows.

### §7.FP — FPU shadow model

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

**FINTRZ memory-slot save/restore** uses `__m68kto64_fpcr_save`. Sequence (using
canonical scratch names — the assembler sees real `rN` tokens; `ScrEA`/`ScrV1`/`ScrV2`
are documentation aliases for r16/r17/r18 respectively):

```asm
fmovcr  r17                            ; capture current FPCR (alias: ScrV1)
la      r16, __m68kto64_fpcr_save      ; alias: ScrEA
store.l r17, (r16)
move.l  r18, #2                        ; round-toward-zero (alias: ScrV2)
fmovcc  r18                            ; force RZ
dint    fd, fs                         ; rounded integer with RZ
la      r16, __m68kto64_fpcr_save
load.l  r17, (r16)
fmovcc  r17                            ; restore caller's FPCR
```

**FPSR ↔ Dn fold (split-on-write, compose-on-read).**

- `FMOVE.L FPSR,Dn` (read): `fmovsr ScrV1` (sticky bits 3:0); `lsl.l ScrV2, ShadowFPCC, #24`; `or.l Dn, ScrV1, ScrV2`.
- `FMOVE.L Dn,FPSR` (write): `lsr.l ScrV1, Dn, #24; and.l ShadowFPCC, ScrV1, #$F` (cc into shadow); `fmovsc Dn` (hardware masks input to bits 3:0; cc bits ignored by hw FPSR).
- `FMOVE.L FPCR,Dn` / `FMOVE.L Dn,FPCR`: direct `fmovcr` / `fmovcc`. No fold.
- `FMOVE.L FPIAR,Dn`: read returns 0; write dropped; diagnostic comment.

**Fuse-skip rule for FPSR writes.** Any line that writes ShadowFPCC by means other
than FCMP/FTST — most importantly `FMOVE.L Dn,FPSR` — breaks adjacency. The FPSR
write clobbers r29 unconditionally per m68k semantics, so a downstream FBcc must
consume the freshly-written shadow, not a stale fcmp result. Fuse fires only when
producer and consumer are both adjacent **and** the producer is `fcmp`/`ftst`. Any
other producer (incl. `fmove.l Dn,fpsr`, `fmovsc`, or any intervening label)
suppresses fuse and routes the FBcc through `emitShadowFBcc`.

**Liveness pass (mirrors integer pass).** Backward walk: at each consumer, mark
"FPCC live"; at each producer, record live-set into `fpccLiveAt[i]` and reset; force
"all live" at any label or function entry. Pass-2 emits the 4-instruction shadow
update only when `fpccLiveAt[i]` is set. Forward branches into a sequence force the
producer to emit shadows since the target may be a consumer reached via a
non-elided path.

## 8. Instruction reference

Convention: ✅ fully covered, ⚠️ semantic caveat, ❌ unsupported.

**Data movement (Phase 2):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| `MOVE.<sz>` | EA load → EA store via ScrV1; shadows updated | ✅ |
| `MOVEA.<sz>` | sign-extend word source as needed; flags untouched | ✅ |
| `MOVEQ #imm,Dn` | `move.l Dn, #sext8(imm)`; shadows updated | ✅ |
| `LEA <ea>,An` | EA computation only; no flag effect | ✅ |
| `MOVEM.W <list>,<ea>` / `<ea>,<list>` | per-element load/store; 2-byte slots, sign-extended on `.W` reload | ✅ |
| `MOVEM.L <list>,<ea>` / `<ea>,<list>` | per-element load/store; 4-byte slots | ✅ |
| `EXT.W/.L Dn` | `sext.b`/`sext.w` into Dn; shadows updated | ✅ |
| `EXTB.L Dn` (68020) | `sext.b` Dn → 32-bit | ✅ |
| `SWAP Dn` | half-word swap via mask + or; shadows updated | ✅ |

**Arithmetic / logical (Phase 2):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| `ADD.<sz>` / `ADDA` / `ADDI` / `ADDQ` | `add.l` / width-masked variant; full NZCV | ✅ |
| `SUB` / `SUBA` / `SUBI` / `SUBQ` / `CMP` / `CMPA` / `CMPI` | `sub.l`; CMP discards result, keeps shadows | ✅ |
| `NEG` / `NOT` / `CLR` | direct ops; full NZCV | ✅ |
| `AND` / `OR` / `EOR` (+ immediate / to-CCR-or-SR) | direct ops | ✅ |
| `MULU` / `MULS` (.W) | widen → multiply; shadows updated | ✅ |
| `MULU.L` / `MULS.L` (68020) | 64-bit pair lowering into r19/r20 | ✅ |
| `DIVU` / `DIVS` (.W) | divide; zero-divisor → syscall #16 | ✅ |
| `DIVU.L` / `DIVS.L` (68020) | 64-bit pair; quotient/remainder pair form | ✅ |
| `LSL` / `LSR` / `ASL` / `ASR` / `ROL` / `ROR` | direct ops; shadows updated | ✅ |
| `BTST` / `BSET` / `BCLR` / `BCHG` | bit ops; Z reflects pre-op bit | ✅ |
| `TST.<sz>` | shadow update only | ✅ |
| `St` / `Sf` (Scc family — all 14 cc) | shadow read → 0/-1 byte | ✅ |
| `ABCD` / `SBCD` / `NBCD` | BCD lowering, both Dn and -(Ay),-(Ax) forms | ✅ |
| `PACK` / `UNPK` (68020) | byte-level pack/unpack | ✅ |
| `BFEXTU` / `BFEXTS` / `BFINS` / `BFCLR` / `BFSET` / `BFCHG` / `BFTST` / `BFFFO` (68020) | bit-field ops on Dn destinations | ✅ |
| `CAS` (68020) | non-atomic load-cmp-store fallback | ⚠️ non-atomic |
| `CAS2` | unsupported | ❌ |
| `ADDX` / `SUBX` / `NEGX` / `ROXL` / `ROXR` | unsupported (X-flag chain-in arithmetic) | ❌ |
| `RTM` / `MOVES` / `CALLM` / `RETM` | unsupported | ❌ |

**Control flow (Phase 3):**

| Mnemonic | Lowering | Status |
|----------|----------|--------|
| `Bcc` (all 14 cc) | adjacent-fuse with CMP/TST → integer Bcc; standalone via shadow CCR | ✅ |
| `BRA` / `BSR` / `JMP` / `JSR` / `RTS` / `RTR` | direct/emulated 32-bit return on r30; RTR additionally restores CCR shadows | ✅ |
| `DBcc Dn,label` (all variants) | decrement+branch on Dn low word; cc tested against shadow CCR | ✅ |
| `TRAP #n` | `syscall #n` (n in 0..15) | ✅ |
| `CHK` / `CHK2` | bounds test → syscall #17 | ✅ |
| `TRAPV` | guarded `syscall #18` against shadow V | ✅ |
| `TRAPcc` (68020, integer-cc form) | conditional `syscall #18` against integer CCR | ⚠️ shares #18 with TRAPV |
| `STOP` / `RESET` / `MOVEC` | stripped + diagnostic | ⚠️ |

**Floating-point** — see §8.FP.

### §8.FP — FPU instruction reference

Convention: ✅ fully covered (bit-exact within IE64 double-precision limits),
⚠️ semantic caveat (typically due to extended → double degradation or transcendental
approximation), ❌ unsupported.

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

### Symbol table (Phase B)

The preprocessor maintains a symbol table populated from three sources, scanned in
forward order:

1. **IE-convenience seeds** — `IS_IE=1` is auto-seeded for compatibility with
   the legacy `ifd IS_IE` rewrite (`-no-default-seeds` suppresses for vasm-pure
   behavior).
2. **`-D NAME[=VALUE]`** — CLI seeds, **mutable** class. Value literals share the
   source-expression grammar: `$hex`, `0x...`, `%bin`, decimal, optional sign.
   Whitespace around `=` is rejected at the CLI surface only (source-level
   `FOO = 5` allows whitespace per vasm).
3. **Source-level `equ` / `set` / `=`** — captured at preprocess time, evaluated
   against the symtab as it stands at that line.

Mutability classes:

| Form | Class | Redefinition |
|------|-------|--------------|
| `NAME equ EXPR` | immutable | error |
| `NAME set EXPR` | mutable   | overwrite |
| `NAME = EXPR`   | mutable   | overwrite |
| `-D NAME[=VAL]` | mutable   | overridable by source `set` / `=`; `equ` errors |

Cross-class: `equ` after any prior binding errors; `set` / `=` after an existing
`equ` errors (vasm semantics — `equ` locks the symbol).

The symtab is captured in Phase B and drives conditional gates from Phase C
onward. Symtab-driven `if`/`elseif*` predicates use the same recursive-descent
expression grammar listed below.

### Conditional assembly (Phase C)

Supported spellings: `if EXPR`, `ifd SYM`, `ifnd SYM`, `ifeq EXPR`, `ifne EXPR`,
`ifb \N`, `ifnb \N`, plus `elseif EXPR`, `elseifd SYM`, `elseifnd SYM`,
`elseifeq EXPR`, `elseifne EXPR`, `else`, `endc` (alias of `endif`), `endif`.

The preprocessor evaluates each predicate transpile-time against the symtab and
rewrites the directive line to a literal `if N` / `elseif N` / `else` / `endif`
form so ie64asm sees stable wrapper shape (Model A, default). First-true latch
applies across an `if`/`elseif*`/`else` chain. The `cpp`-style `elif` is **not** a
vasm token and is rejected.

`-strip-cond` switches to Model B: the preprocessor drops both wrapper
directives and inactive bodies, producing cleaner output but breaking byte-diff
against the legacy wrapper-preserving shape.

Expression grammar (lowest→highest precedence): `||`, `&&`, `|`, `^`, `&`,
equality (`==`, `!=`, `=`, `<>`), relation (`<`, `>`, `<=`, `>=`), shift
(`<<`, `>>`), additive (`+`, `-`), multiplicative (`*`, `/`, `%`), unary
(`-`, `~`, `!`), primary (literal / symbol / parens). Literals: decimal,
`$hex`, `0x...`, `%bin`.

Failed symbol lookup in an active branch records an error; in an inactive
branch it is silent (allows nested feature gates to reference undefined
symbols).

| m68k directive | IE64 emit | Notes |
|----------------|-----------|-------|
| `dc.b/w/l/q` | passthrough | width-equivalent in `ie64asm` |
| `ds.b/w/l/q` | passthrough | reservation, width-equivalent |
| `equ` / `set` | passthrough | symbol assignment |
| `org` | passthrough | layout directive |
| `section` | dropped + diagnostic | ie64asm assembles into a single flat output (Phase F) |
| `align` / `even` | `align 2` / passthrough | `even` lowered to `align 2` |
| `include` | inlined transpile-time | resolves against `-I` paths and the includer's dir (Phase D). Cycle-guarded |
| `incbin` | passthrough verbatim | ie64asm resolves at assemble-time via its own `-I` |
| `if` / `ifd` / `ifnd` / `ifeq` / `ifne` | `if N` (preproc-evaluated) | predicate against symtab |
| `else` / `elseif` / `elseifd` / `elseifnd` / `elseifeq` / `elseifne` | `else` / `elseif N` | first-true latch at preproc |
| `endc` / `endif` | `endif` | conditional terminator |
| `xdef` / `xref` / `public` / `global` / `extern` | dropped + diagnostic | flat single-file namespace |
| `macro` / `endm` / `mexit` / `\1`–`\9` / `\@` | expanded transpile-time (Phase E) | preprocessor owns expansion; converter no longer passes raw macro bodies via `ConvertFile` path |
| `rept` / `endr` | expanded transpile-time | per-iteration global-monotonic `\@` |
| `ifb \N` / `ifnb \N` | preproc conditional | macro-arg-blank tests; valid inside macro bodies |

Macro and rept bodies are emitted verbatim; the converter does not expand them. This
keeps `\1`–`\9` argument substitutions intact for `ie64asm` to handle.

## 10. MMIO & equates

m68k programs that target the IE platform already use IE chip equates (e.g.
`SOMECTRL equ $F2324`). The transpiler treats those as opaque symbols — no remap table,
no rewriting. This mirrors `ie32to64` behaviour.

Programs targeting non-IE m68k platforms will assemble cleanly but will not function:
their hardware-register addresses are not mapped on the IE platform.
Hardware-register translation is out of scope; either run such programs on a host that
emulates the original platform, or rewrite the I/O layer to IE chip equates before
transpile.

## 11. TRAP / syscall vectors

`TRAP #n` → `syscall #n` (`assembler/ie64asm.go:3898`). Locked syscall # range
(disjoint reservation across all m68k exception sources; see §11.FP for the FP
extension):

| syscall # | m68k source | Notes |
|-----------|-------------|-------|
| `#0`–`#15` | `TRAP #n` (instruction-encoded immediate) | direct 1:1 mapping |
| `#16` | integer divide-by-zero (m68k vector 5, relocated) | DIVU/DIVS/DIVU.L/DIVS.L zero-divisor guard |
| `#17` | CHK / CHK2 fail (m68k vector 6, relocated) | bounds-test bltz/bgt fall-through |
| `#18` | TRAPV / integer TRAPcc (m68k vector 7, relocated) | guarded on ShadowV / integer CCR |
| `#32`–`#63` | FTRAPcc (one syscall # per FP cc kind) | see §11.FP |

CHK/TRAPV/divide-by-zero originally landed at `#5/#6/#7` in early Phase 5; relocated
above the `TRAP #n` instruction-encoded range as part of Phase 7.0 to keep
TRAP-instruction-# disjoint from m68k exception-vector-#.

### §11.FP — Locked syscall # vector table

`FTRAPcc` (Phase 7.4) consumes syscall numbers `#32`–`#63`, one per FP cc kind. Order
is fixed in `cmd/m68kto64/fpu_shadow.go::fpFTrapccOrder` and is permanent — downstream
consumers may pin handlers to specific numbers.

| # | cc | # | cc | # | cc | # | cc |
|---|----|----|----|----|----|----|----|
| 32 | F  | 40 | UN  | 48 | SF  | 56 | NGLE |
| 33 | EQ | 41 | UEQ | 49 | SEQ | 57 | NGL  |
| 34 | OGT| 42 | UGT | 50 | GT  | 58 | NLE  |
| 35 | OGE| 43 | UGE | 51 | GE  | 59 | NLT  |
| 36 | OLT| 44 | ULT | 52 | LT  | 60 | NGE  |
| 37 | OLE| 45 | ULE | 53 | LE  | 61 | NGT  |
| 38 | OGL| 46 | NE  | 54 | GL  | 62 | SNE  |
| 39 | OR | 47 | T   | 55 | GLE | 63 | ST   |

## 12. Limitations & caveats

- **Self-modifying code.** Not supported. Source-level transpile freezes the m68k
  instruction stream at convert time; runtime patches against m68k addresses have no
  effect on the emitted IE64 stream.
- **Multi-unit linkage.** `xdef` / `xref` are dropped. Multi-file builds must be
  concatenated through `sdk/scripts/ab3d2/kmake.sh` (or an equivalent per-port
  wrapper) so all symbols resolve in a single namespace.
- **Unsupported preprocessor extensions.** `\?` (devpac alt-arg) and `\<name>`
  (vasm named macro args) emit explicit errors — add when a real port needs
  them. Float-expression evaluation in conditional predicates is out of scope
  (no consumer demand). `equ.s` / `equ.l` size-typed equates are not honored
  (orthogonal to the preprocessor surface).
- **CAS / CAS2.** `CAS` lowers to a non-atomic load-cmp-store fallback; multi-context
  guests racing on the same address will observe lost updates. `CAS2` is unsupported.
- **BCD edge cases.** ABCD/SBCD/NBCD cover the common `Dn,Dn` and `-(Ay),-(Ax)` forms
  with X-flag carry propagation; underflow into the X flag matches m68k semantics but
  has not been bit-validated against every 6809-era undefined-flag corner case.
- **Performance.** Shadow-flag emission inflates straight-line code 4–8× when fuse
  fails. The fuse pass mitigates the bulk of CMP/Bcc traffic, but unfused spans
  (especially around macros that hide flag dependencies) carry real runtime cost.
- **Memory-slot reentrancy.** `__m68kto64_fpcr_save` and `__m68kto64_fp_scratch_q` are
  per-output-file globals. Single-thread guests are unaffected; multi-context guests
  must wrap interrupt prolog/epilog with manual save/restore (see §15.FP).

## 13. Worked examples

All snippets in this section are real `m68kto64` output. They round-trip through
`sdk/bin/ie64asm` cleanly and back the integer goldens
(`cmd/m68kto64/golden/{arith_basic,control_flow,shadow_ccr}.s`).

#### 1. Straight-line arithmetic — fused ADD / SUB

m68k input:

```asm
        move.l  #5,d1
        add.l   d1,d0
        sub.l   #1,d0
        rts
```

IE64 lowering (key fragments shown; full shadow CCR updates elided for brevity):

```asm
        move.l  r17, #5
        move.l  r2, r17           ; d1 ← #5
        ; … shadow update r24..r27 …
        move.l  r21, r1           ; pre-op snapshot of d0 for V
        add.l   r1, r1, r2        ; d0 ← d0 + d1
        ; … N/Z/C/V shadows from r21, r2, r1 …
        move.l  r17, #1
        move.l  r21, r1
        sub.l   r1, r1, r17       ; d0 ← d0 - 1
        ; … shadows …
        load.l  r17, (r30)        ; rts (32-bit emulated stack on r30)
        add.l   r30, r30, #4
        jmp     (r17)
```

Notes: r1 = d0, r2 = d1, r17 = ScrV1 immediate carrier, r21 = pre-op snapshot for
shadow C/V. The RTS sequence shows the 4-byte guest-stack pop on r30 — never a native
IE64 8-byte slot.

#### 2. Adjacent-fuse loop — DBcc-shaped countdown

m68k input:

```asm
start:
        move.l  #10,d0
loop:
        sub.l   #1,d0
        cmp.l   #0,d0
        bne     loop
        bsr     helper
        bra     done
helper:
        move.l  #1,d1
        rts
done:
        rts
```

IE64 lowering of the fused-loop body and the BSR sequence:

```asm
loop:
        move.l  r17, #1
        move.l  r21, r1
        sub.l   r1, r1, r17        ; d0 -= 1 (shadows updated, fused below)
        ; … (shadow updates emitted because Bcc is non-adjacent to sub) …
        move.l  r17, #0
        bne     r1, r17, loop      ; ← fused CMP+BNE → integer-pair branch on r1,r17

        sub.l   r30, r30, #4       ; bsr helper — push 32-bit return label
        la      r17, __m68kto64_bsr_ret_1
        store.l r17, (r30)
        bra     helper
__m68kto64_bsr_ret_1:
        bra     done

helper:
        move.l  r17, #1
        move.l  r2, r17            ; d1 ← #1
        ; … shadows …
        load.l  r17, (r30)         ; rts
        add.l   r30, r30, #4
        jmp     (r17)
```

Notes: the auto-generated label `__m68kto64_bsr_ret_N` makes the m68k JSR/BSR push
explicit; the 4-byte slot on r30 carries the return address. The `bne r1, r17, loop`
form is the fused emit — no shadow CCR read is required because the producer (`sub.l`)
and the consumer (`bne` after the immediate compare against #0) are adjacent.

#### 3. Standalone shadow-CCR consumer (fuse fails)

m68k input — an intervening `move.l` between the flag producer and the consumer
defeats the fuse:

```asm
start:
        move.l  #10,d0
        tst.l   d0
        move.l  #$1234,d1   ; intervening op — forces shadow-CCR path
        beq     target
        cmp.l   #5,d0
        move.l  #$5678,d2
        blt     other
target: rts
other:  rts
```

IE64 lowering:

```asm
start:
        move.l  r17, #10
        move.l  r1, r17            ; d0 ← #10  (shadows updated below)
        sext.l  r24, r17           ; N
        move.l  r25, r17           ; Z
        move.l  r26, #0            ; C
        move.l  r27, #0            ; V

        ; tst.l d0  — refresh shadows from r1
        sext.l  r24, r1
        move.l  r25, r1
        move.l  r26, #0
        move.l  r27, #0

        move.l  r17, #$1234        ; intervening op (does its own shadow update)
        move.l  r2, r17
        ; … shadows …

        beqz    r25, target        ; standalone BEQ — reads shadow Z (r25)

        ; cmp.l #5,d0 — produces full N/Z/C/V from pre-op snapshot
        move.l  r17, #5
        sub.l   r21, r1, r17       ; result discarded for cmp; shadows captured
        ; … N/Z/C/V update sequence …

        move.l  r17, #$5678
        move.l  r3, r17            ; d2 ← #$5678 (clobbers shadows again)
        ; … shadows …

        ; blt other — needs N != V, must reconstruct from shadow regs
        lsr.q   r17, r24, #63      ; sign bit of shadow N
        and.q   r17, r17, #1
        eor.q   r17, r17, r27      ; N XOR V
        bnez    r17, other         ; LT condition

target: load.l r17, (r30); add.l r30, r30, #4; jmp (r17)
other:  load.l r17, (r30); add.l r30, r30, #4; jmp (r17)
```

Notes: `beqz r25, target` is the simple shadow-Z form. The `blt` lower at the bottom
shows the signed-LT compose: `(N XOR V) != 0`. Every intervening op refreshes shadows,
so the consumer always reads the most recently composed values — m68k semantics.

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
        sext.w  r17, r1                  ; ScrV1 = sext.w(d0 low word)
        bne     r17, #-1, .loop          ; DBRA: branch while low word ≠ -1
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
        sext.w  r17, r1                  ; ScrV1 = sext.w(d0 low word)
        bne     r17, #-1, .gen           ; DBRA: branch while low word ≠ -1
        rts
```

All three goldens validate that the FMOVEM mask order, FCMP+FBcc fuse, and
ShadowFPCC liveness elision interact cleanly across typical FPU hot loops.

## 14. Troubleshooting

**Shadow-CCR consumer reads stale flags.** Symptom: a Bcc takes the wrong path after
an apparently-unrelated `move.l`. Cause: m68k MOVE updates NZCV; the shadow-CCR
producer pass faithfully refreshes r24/r25/r26/r27 on every flag-touching op, so the
consumer sees the most recent producer's flags, not the CMP/TST you intended. Either
move the CMP/TST adjacent to the consumer (so the fuse fires), or restructure to
avoid the intervening flag-clobber.

**`ie64asm` rejects an emit with `unknown register`.** Almost always an EA scratch
collision: hand-written inline IE64 inside transpiled m68k clobbered r16–r29 or
r30/r31. Re-read §4 reservation rule and confine inline fragments to r0 or r1–r15
copies that you have first spilled.

**`-strict` errors on a span with no obvious flag consumer.** The fuse pass walks
forward across labels and conservatively forces "all live" at any label entry. Code
that falls through to a label but does not consume flags there will still report
"unfused" under `-strict`. Either restructure the label out (if the label is dead) or
accept the standalone shadow path under non-strict mode.

**MOVEM mask order looks wrong.** m68k specifies that `MOVEM <list>,-(An)` writes in
**descending** register order (a7 first) and `MOVEM (An)+,<list>` reads in
**ascending** order. The transpiler matches both. If you need the opposite ordering
(writing low-to-high through `-(An)`), re-order the source-side mask — the converter
does not silently re-sort.

**Multi-file build: undefined symbol from a cross-unit macro.** Symbols defined via
`DC`-style macros expanded across multiple input files require all units to be
concatenated into a single `m68kto64` invocation. Use `sdk/bin/m68kto64-kmake` with
all relevant include and source paths so that macro expansions resolve in one
namespace.

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
f14 per §4.FP) or has inadvertently written through `ScrFP1` (f10 — also FP5)
or `ScrFP2` (f12 — also FP6). Re-map against the table in §4.FP and ensure no
inline IE64 fragment touches `f0`–`f15`.

**FP5 / FP6 corruption across FTST/FSCALE/FGETEXP/FGETMAN.** The transpiler
overlays `ScrFP1` on f10 (FP5) and `ScrFP2` on f12 (FP6). Programs that hold
live FP5 / FP6 values across one of those synthesised ops will read garbage
back. Either spill FP5 / FP6 around the op (manual `dstore`/`dload` to a
guest-managed slot) or restrict floating-point register use to FP0–FP4 + FP7
in the affected routine. This is the single open implementation gap (see §15.FP).

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
round-toward-zero FPCR value. Single-thread guests are unaffected. The same
caveat applies to `__m68kto64_fp_scratch_q` used by FSCALE and FGETMAN.

## 14.1. Real-world ports (Phase G)

`cmd/m68kto64/integration_realworld_test.go` (build tag
`m68kto64_integration`) runs each entry in the corpus registry
(`corpus_registry_test.go`) through `Preprocess` + `ConvertLines`
+ `sdk/bin/ie64asm`. Each sub-test `t.Skip`s when:

- the corpus env var is unset (e.g. `IE_M68KTO64_CORPUS_AB3D2`);
- the configured root file is not stat-able;
- `sdk/bin/ie64asm` is missing (run `make ie64asm`).

Invoke (AB3D2 example):

```text
IE_M68KTO64_CORPUS_AB3D2=/path/to/alienbreed3d2 \
  go test -tags m68kto64_integration -run TestRealworldCorpus ./cmd/m68kto64/
```

Registered corpora:

| Name | Env var | Root file |
|------|---------|-----------|
| ab3d2 | `IE_M68KTO64_CORPUS_AB3D2` | `ab3d2_source/ie/hires.s` |

**Coverage caveat.** A clean `ie64asm` exit proves the output assembles, not
that the lowering is semantically correct. Functional/runtime validation is
owned by the per-port companion plan.

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
- **Phase 6 — done.** Multi-file wrapper (`cmd/m68kto64/kmake.sh`, installed as `sdk/bin/m68kto64-kmake`) concatenates multi-file builds and invokes `ie64asm` with `-I` paths.
- **Phase 7 — done.** 68881/68882 FPU coprocessor support: FP0–FP7 → f0/f2/.../f14 even-pair ABI, ShadowFPCC at r29, FCMP+FBcc adjacent fuse, ShadowFPCC liveness pass, FPSR↔Dn split-on-write/compose-on-read fold, FINTRZ memory-slot save/restore, FSCALE+FGETMAN exponent-bit-pattern round-trip, all 32 FP cc kinds (FBcc/FDBcc/FScc/FTRAPcc), full transcendental set via single-precision IE64 ops + identities, FNOP/FSAVE/FRESTORE/.P diagnostics, locked syscall range #16/#17/#18 + #32–#63 (FTRAPcc).

### Round-trip verified

- Four checked-in goldens (`cmd/m68kto64/golden/{arith_basic,control_flow,shadow_ccr,fpu_basic}.s`) transpile and assemble cleanly through `sdk/bin/ie64asm`, producing non-zero binaries. Harness: `TestGolden_RoundTrip` in `cmd/m68kto64/golden_test.go`.
- Multi-file inputs concatenate cleanly through `sdk/bin/m68kto64-kmake` and assemble to non-zero binaries.
- Statement coverage: **100%** of `cmd/m68kto64/` package (every function and branch covered by the test suite, including direct-invoke tests for defensive error paths).
- Numeric differential validated: `TestM68KFPU_NumericDifferential_RuntimeVsHostMath` runs a transpiled+assembled FPU program on a real `MachineBus` + `CPU64`, reads back 5 transcendental results from guest memory, and confirms agreement with host `math.Sin/Cos/Exp/Log/Sqrt` within ε=1e-6 (Δ profile 0 to 8.3e-08, reflecting IE64 single-precision native ops widened to double).

### Genuinely out of scope for transpiler-only ship

1. **End-to-end boot of an arbitrary transpiled binary.** Diagnostic-driven smoke
   on the IE64 core requires runtime debugging of the running binary, not transpiler
   code work. The transpile path is verified; the runtime behaviour of any specific
   downstream binary is the binary's responsibility.
2. **Harte 68020 conformance harness.** Harte test data (`testdata/680x0/`) is raw
   machine code, so wiring it requires a m68k disassembler step before transpile.
   The transpiler operates on assembly source, not bytes; conformance via Harte
   therefore needs an external chain (vasm round-trip or musashi-driven differential).
   Deferred until that chain is built.
3. **Differential vs M68K core.** Same dependency on a m68k assembler chain
   (vasm/devpac) to produce the reference binary the IE64 transpilation is compared
   to. Plan §TDD acknowledges this is built on top of the transpiler.
4. **Performance pass on transpiled hot loops.** Shadow-flag overhead + fuse-coverage
   measurements; needs binaries running on the IE64 core. Blocked on (1).

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

Two open implementation gaps remain. The first is the FP5 / FP6 scratch
overlay: `ScrFP1` (f10) and `ScrFP2` (f12) double as FP5 / FP6 storage,
so any synthesised op that uses scratch (FTST, FSCALE, FGETEXP, FGETMAN)
clobbers the live FP5 / FP6 value. Programs are responsible for spilling
FP5 / FP6 around those ops, or for restricting register use to FP0–FP4 +
FP7. Lifting this restriction requires either widening the IE64 FP file
or burning two more f-registers as dedicated scratch.

The second open gap is memory-slot reentrancy under guest interrupts.
`__m68kto64_fpcr_save` and `__m68kto64_fp_scratch_q` are single-instance
globals per output file; the transpiler emits no save/restore of these
slots across guest interrupt entry and exit. A program where an interrupt
handler executes FINTRZ, FSCALE, or FGETMAN while the main line is
mid-sequence will corrupt the slot and observe the transient FPCR or
exponent value. Single-thread targets are unaffected. Multi-context
guests must currently wrap handler prolog/epilog with manual slot
save/restore. Lifting this restriction requires an interrupt-aware
codegen pass that emits per-handler save/restore stubs.

Phase 7 is otherwise content-complete: every m68k FPU mnemonic lowers to
IE64; every FP cc kind either fuses with an adjacent FCMP/FTST or
consumes ShadowFPCC through the standalone-test path; all eight
documentation sub-sections (§4.FP through §15.FP) are filled; the
`cmd/m68kto64/fpu*.go` files maintain 100% Go statement coverage in the
test suite; and the `golden/fpu_basic.s` round-trip exercise confirms
end-to-end transpile → ie64asm conformance.

See [`ie32to64.md`](ie32to64.md) for the sibling IE32→IE64 transpiler.
