# M68881 FPU Notes

The M68K core in IE implements a subset of the Motorola 68881/68882 floating-point coprocessor instruction set. Reference: *MC68881/MC68882 Floating-Point Coprocessor User's Manual* (Motorola), tables 6-1 and 6-2.

## Reg-to-Reg Op Table

`fpuOpTable` in `cpu_m68k.go:11387` maps the low 7 bits of the FPU command word to the implementation function pointer. Populated entries are the architecturally valid ops per the 68881 PRM. Unpopulated indices (e.g. 0x07) are intentionally absent — they are not defined ops and `execFPURegToReg` raises `LINE_F` for them.

The test pair in `fpu_integration_test.go` guards both directions of drift:

- `TestFPU_IllegalOpcodeRaisesLineF` — hand-maintained "valid" set encodes the 68881 PRM. Drift here means the impl handles a non-spec op or vice versa. **Do not** auto-derive this set from `fpuOpTable` — that defeats spec-vs-impl conformance checking.
- `TestFPU_OpTableCoverage_NoLineF` — asserts every populated `fpuOpTable[op] != nil` slot actually executes without raising `LINE_F`. Catches table entries that lack spec backing.

## FMOVECR (ROM constants)

FMOVECR loads a constant from the on-chip ROM (`Pi`, `e`, `Ln(2)`, etc.) into an FP register. Encoding: cmdWord bits 15:10 = `0b010111` (`& 0xFC00 == 0x5C00`), bits 9:7 = destination FPn, bits 6:0 = ROM address.

Decoding lives in `execFPUGeneral` (`cpu_m68k.go:11489`), **not** in `execFPURegToReg`. The reg-to-reg helper only handles the plain ops in `fpuOpTable`. Tests exercising FMOVECR must call `execFPUGeneral(opcode, cmdWord)` (or `cpu.FPU.FMOVECR(romAddr, dstReg)` directly), not `execFPURegToReg`.

## FMOVEM Direction Bit

`execFMOVEM` is reached from `execFPUGeneral`'s `switch (cmdWord >> 13) & 3` for cases 2 (load) and 3 (store). The direction is encoded in bit 13 of `cmdWord` and *must stay consistent with the decoder switch*:

- `dr = 0` (cmdWord bits 15:13 = `110`) — effective address → FP regs (load)
- `dr = 1` (cmdWord bits 15:13 = `111`) — FP regs → effective address (store)

Tests calling `execFMOVEM` directly with truncated cmdWords (no opclass bit 15) must use `0x00FF`-style words for *load* and `0x20FF`-style words for *store*, matching the bit-13 convention above. Inverting the helper would silently break every assembled FMOVEM in shipped guest code.

## Extended-Precision (96-bit) Layout in Memory

`writeExtendedReal96` and `readExtendedReal96` use the standard 12-byte layout:

| Offset | Field |
|---|---|
| `ea+0..1` | sign (bit 15) + biased exponent (bits 14:0) |
| `ea+2..3` | padding (zeroed on write, ignored on read) |
| `ea+4..7` | mantissa high 32 bits |
| `ea+8..11` | mantissa low 32 bits |

Both helpers must agree on the padding word; round-trip tests gate this.
