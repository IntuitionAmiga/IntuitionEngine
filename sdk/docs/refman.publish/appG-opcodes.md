
# Appendix G - Per-CPU Opcode Quick Reference

One table per CPU. Each table is a single-line summary of every
mnemonic; the full encoding, addressing modes, flag effects, and
cycle counts are in the per-CPU chapter (Ch 24-29). This appendix
is for the reader who has the chapter open already and only needs
to look up "is this mnemonic spelled this way" or "what does this
opcode do at a glance".

## G.1 IE64 (Chapter 24)

Fixed `8`-byte instruction, 32 GPRs `R0`-`R31`, `R0 = 0`,
`R31 = SP`. Compare-and-branch ISA; no separate flag register.

| Group | Mnemonics |
|-------|-----------|
| ALU   | `ADD`, `SUB`, `MUL`, `DIV`, `MOD`, `AND`, `OR`, `XOR`, `NOT`, `NEG`, `SHL`, `SHR`, `SAR`, `ROL`, `ROR`. Each has `.Q` (full register) and `.L`/`.W`/`.B` width variants for memory ops. |
| Load / store | `LOAD.B/.W/.L/.Q`, `STORE.B/.W/.L/.Q`. Address modes: register, register + immediate, register + register, PC-relative. |
| Immediate | `MOVE.Q rd, #imm32`, `LI rd, #imm`. |
| Branch   | `BRA`, `JSR`, `RTS`, `BEQ`, `BNE`, `BLT`, `BLE`, `BGT`, `BGE`, `BEQZ`, `BNEZ`. All compare-and-branch forms take two GPRs and a target. |
| FPU      | `FADD`, `FSUB`, `FMUL`, `FDIV`, `FNEG`, `FABS`, `FSQRT`, `FCMP`, `FTOI`, `ITOF`, single and double precision. |
| System   | `SYSCALL`, `BREAK`, `HALT`, `CRMOV` (move to/from CR0-CR15), `TLBI` (invalidate one TLB entry), `MFENCE`. |

## G.2 IE32 (Chapter 25)

`8`-byte instruction, 16 named registers `A,X,Y,Z,B,C,D,E,F,G,H,I,J,K,L,W`, no MMU, no FPU.

| Group | Mnemonics |
|-------|-----------|
| ALU   | `ADD`, `SUB`, `MUL`, `DIV`, `AND`, `OR`, `XOR`, `NOT`, `NEG`, `SHL`, `SHR`. |
| Memory | `LOAD`, `STORE`, with `.B`/`.W`/`.L` width suffixes. |
| Immediate | `LDI A, #imm32`. |
| Branch | `JMP`, `JSR`, `RTS`, `JZ rA`, `JNZ rA` (test single register against zero). |
| Stack  | `PUSH rA`, `POP rA`. |
| Timer  | direct MMIO (`IE32_TIMER_COUNT`, `IE32_TIMER_PERIOD`). |

## G.3 6502 (Chapter 26)

`A`, `X`, `Y`, `S`, `P`, `PC`. Addressing modes: implied,
accumulator, immediate, zero-page (+ X, + Y), absolute (+ X, + Y),
indirect, (indirect,X), (indirect),Y, relative.

| Group | Mnemonics |
|-------|-----------|
| Load / store | `LDA`, `LDX`, `LDY`, `STA`, `STX`, `STY`. |
| Transfer    | `TAX`, `TAY`, `TXA`, `TYA`, `TSX`, `TXS`. |
| Arith / log | `ADC`, `SBC`, `AND`, `ORA`, `EOR`, `BIT`, `CMP`, `CPX`, `CPY`. |
| Inc / dec   | `INC`, `DEC`, `INX`, `INY`, `DEX`, `DEY`. |
| Shift       | `ASL`, `LSR`, `ROL`, `ROR`. |
| Branch      | `BCC`, `BCS`, `BEQ`, `BNE`, `BMI`, `BPL`, `BVC`, `BVS`. |
| Jump / sub  | `JMP`, `JSR`, `RTS`, `RTI`. |
| Flag        | `CLC`, `SEC`, `CLD`, `SED`, `CLI`, `SEI`, `CLV`. |
| Stack       | `PHA`, `PLA`, `PHP`, `PLP`. |
| System      | `BRK`, `NOP`. |
| Undoc       | `LAX`, `SAX`, `DCP`, `ISC`, `RLA`, `RRA`, `SLO`, `SRE`, `ANC`, `ARR`, `ASR`, `AXS`, `XAA`. |

Flag register `P`: bit `0` C, `1` Z, `2` I, `3` D, `4` B, `5` -,
`6` V, `7` N (silicon convention; see AUTHOR_PROVENANCE for the
MOS 1976 manual divergence).

## G.4 Z80 (Chapter 27)

Main: `A,F,B,C,D,E,H,L`; alt: `A',F',B',C',D',E',H',L'`; index:
`IX`, `IY`; refresh / interrupt: `I`, `R`; stack: `SP`. Prefixes:
`CB` (bit / rotate), `ED` (extended), `DD` / `FD` (IX / IY).

| Group | Mnemonics |
|-------|-----------|
| 8-bit load | `LD r, r'`, `LD r, n`, `LD r, (HL)`, `LD r, (IX+d)`, `LD A, (BC)/(DE)/(nn)`. |
| 16-bit load | `LD rp, nn`, `LD rp, (nn)`, `LD (nn), rp`, `PUSH rp`, `POP rp`. |
| Block       | `LDI`, `LDIR`, `LDD`, `LDDR`, `CPI`, `CPIR`, `CPD`, `CPDR`. |
| Arith / log | `ADD`, `ADC`, `SUB`, `SBC`, `AND`, `OR`, `XOR`, `CP`, `INC`, `DEC`. |
| 16-bit ALU  | `ADD HL,rp`, `ADC HL,rp`, `SBC HL,rp`, `ADD IX,rp`, `ADD IY,rp`. |
| Rotate / shift | `RLCA`, `RLA`, `RRCA`, `RRA`, `RLC`, `RL`, `RRC`, `RR`, `SLA`, `SRA`, `SRL`, `SLL` (undoc). |
| Bit         | `BIT b,r`, `SET b,r`, `RES b,r`. |
| Jump / call | `JP`, `JR`, `CALL`, `RET`, with condition codes `NZ`, `Z`, `NC`, `C`, `PO`, `PE`, `P`, `M`. `DJNZ`. |
| I/O         | `IN A,(n)`, `IN r,(C)`, `OUT (n),A`, `OUT (C),r`, `INI`, `OUTI`, `INIR`, `OTIR`, `IND`, `OUTD`, `INDR`, `OTDR`. |
| Interrupt   | `EI`, `DI`, `IM 0`, `IM 1`, `IM 2`, `RETI`, `RETN`. |
| Misc        | `NOP`, `HALT`, `EX`, `EXX`, `DAA`, `CPL`, `NEG`, `CCF`, `SCF`. |

Flag register: `S`, `Z`, `Y` (bit 5 undoc), `H`, `X` (bit 3
undoc), `P/V`, `N`, `C`.

## G.5 M68K 68020 (Chapter 28)

`D0`-`D7`, `A0`-`A7`, `PC`, `SR`. CCR low byte: X N Z V C.
Big-endian, byte-addressable, 32-bit address bus.

| Group | Mnemonics |
|-------|-----------|
| Move        | `MOVE`, `MOVEA`, `MOVEM`, `MOVEP`, `MOVEQ`, `EXG`, `SWAP`, `LEA`, `PEA`. |
| Arith       | `ADD`, `ADDA`, `ADDI`, `ADDQ`, `ADDX`, `SUB`, `SUBA`, `SUBI`, `SUBQ`, `SUBX`, `NEG`, `NEGX`, `CMP`, `CMPA`, `CMPI`, `CMPM`, `CLR`, `EXT`, `EXTB`. |
| Mul / div   | `MULU`, `MULS`, `DIVU`, `DIVS` (68020 also `MULU.L`, `MULS.L`, `DIVUL`, `DIVSL`). |
| Logical     | `AND`, `ANDI`, `OR`, `ORI`, `EOR`, `EORI`, `NOT`. |
| Shift / rot | `ASL`, `ASR`, `LSL`, `LSR`, `ROL`, `ROR`, `ROXL`, `ROXR`. |
| Bit         | `BTST`, `BSET`, `BCLR`, `BCHG`. |
| Bit field   | `BFTST`, `BFSET`, `BFCLR`, `BFCHG`, `BFEXTS`, `BFEXTU`, `BFINS`, `BFFFO`. |
| Branch      | `BRA`, `BSR`, `Bcc` (`HI`, `LS`, `CC`, `CS`, `NE`, `EQ`, `VC`, `VS`, `PL`, `MI`, `GE`, `LT`, `GT`, `LE`), `DBcc`, `Scc`, `JMP`, `JSR`, `RTS`, `RTR`, `RTD`. |
| BCD         | `ABCD`, `SBCD`, `NBCD`. |
| Multiprocessor | `TAS`, `CAS`, `CAS2`. |
| System      | `TRAP #n`, `TRAPV`, `CHK`, `CHK2`, `STOP`, `RESET`, `ILLEGAL`, `NOP`, `LINK`, `UNLK`, `MOVE USP`, `MOVEC`, `MOVES`, `BKPT`, `RTE`. |
| Line A/F    | unassigned opcode trap. |

## G.6 x86 (Chapter 29, 8086 base + 386 extensions)

`EAX`, `EBX`, `ECX`, `EDX`, `ESI`, `EDI`, `EBP`, `ESP`, plus
16-bit and 8-bit subregisters. Segments `CS`, `DS`, `ES`, `FS`,
`GS`, `SS`. Real-mode only.

| Group | Mnemonics |
|-------|-----------|
| Move        | `MOV`, `MOVZX`, `MOVSX`, `LEA`, `XCHG`, `XLAT`, `PUSH`, `POP`, `PUSHA`, `POPA`, `PUSHAD`, `POPAD`, `PUSHF`, `POPF`, `PUSHFD`, `POPFD`. |
| Arith       | `ADD`, `ADC`, `SUB`, `SBB`, `INC`, `DEC`, `CMP`, `NEG`, `MUL`, `IMUL`, `DIV`, `IDIV`, `DAA`, `DAS`, `AAA`, `AAS`, `AAM`, `AAD`, `CBW`, `CWD`, `CWDE`, `CDQ`. |
| Logical     | `AND`, `OR`, `XOR`, `NOT`, `TEST`. |
| Shift / rot | `SHL`, `SHR`, `SAR`, `SAL`, `ROL`, `ROR`, `RCL`, `RCR`, `SHLD`, `SHRD`. |
| Bit         | `BT`, `BTS`, `BTR`, `BTC`, `BSF`, `BSR`, `SETcc`. |
| Branch      | `JMP`, `Jcc` (`JE`, `JNE`, `JL`, `JG`, `JLE`, `JGE`, `JB`, `JA`, `JBE`, `JAE`, `JO`, `JNO`, `JS`, `JNS`, `JP`, `JNP`, `JCXZ`, `JECXZ`), `CALL`, `RET`, `RETF`, `LOOP`, `LOOPE`, `LOOPNE`, `INT n`, `INTO`, `IRET`. |
| String      | `MOVS`, `CMPS`, `SCAS`, `LODS`, `STOS`, `INS`, `OUTS`, prefixes `REP`, `REPE`, `REPNE`. |
| I/O         | `IN`, `OUT`. |
| Flag        | `STC`, `CLC`, `CMC`, `STD`, `CLD`, `STI`, `CLI`, `LAHF`, `SAHF`. |
| Segment     | `LDS`, `LES`, `LFS`, `LGS`, `LSS`. |
| System      | `HLT`, `WAIT`, `NOP`, `ESC`, `LOCK`. |
| 386 extras  | `BSWAP`, `MOVSXD`, dword forms of all 16-bit ops via `66h` / `67h` prefixes. |

Omitted (Chapter 29): all protected-mode opcodes (`LGDT`,
`LIDT`, `LLDT`, `LTR`, `LMSW`, `SMSW`, `ARPL`, `LAR`, `LSL`,
`VERR`, `VERW`, `STR`, `SLDT`), `CR` and `DR` register moves,
`INVLPG`, `WBINVD`, `INVD`.
