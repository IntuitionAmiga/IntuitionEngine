# SDK ISA Source Audit

| CPU | Kind | Value | Source symbol | Executable evidence |
|-----|------|-------|---------------|---------------------|
| IE64 | opcode | 0x01 | `OP_MOVE` | `cpu_ie64.go` const `OP_MOVE`; execute switch case `OP_MOVE`; step switch case `OP_MOVE` |
| IE64 | opcode | 0x02 | `OP_MOVT` | `cpu_ie64.go` const `OP_MOVT`; execute switch case `OP_MOVT`; step switch case `OP_MOVT` |
| IE64 | opcode | 0x03 | `OP_MOVEQ` | `cpu_ie64.go` const `OP_MOVEQ`; execute switch case `OP_MOVEQ`; step switch case `OP_MOVEQ` |
| IE64 | opcode | 0x04 | `OP_LEA` | `cpu_ie64.go` const `OP_LEA`; execute switch case `OP_LEA`; step switch case `OP_LEA` |
| IE64 | opcode | 0x10 | `OP_LOAD` | `cpu_ie64.go` const `OP_LOAD`; execute switch case `OP_LOAD`; step switch case `OP_LOAD` |
| IE64 | opcode | 0x11 | `OP_STORE` | `cpu_ie64.go` const `OP_STORE`; execute switch case `OP_STORE`; step switch case `OP_STORE` |
| IE64 | opcode | 0x20 | `OP_ADD` | `cpu_ie64.go` const `OP_ADD`; execute switch case `OP_ADD`; step switch case `OP_ADD` |
| IE64 | opcode | 0x21 | `OP_SUB` | `cpu_ie64.go` const `OP_SUB`; execute switch case `OP_SUB`; step switch case `OP_SUB` |
| IE64 | opcode | 0x22 | `OP_MULU` | `cpu_ie64.go` const `OP_MULU`; execute switch case `OP_MULU`; step switch case `OP_MULU` |
| IE64 | opcode | 0x23 | `OP_MULS` | `cpu_ie64.go` const `OP_MULS`; execute switch case `OP_MULS`; step switch case `OP_MULS` |
| IE64 | opcode | 0x24 | `OP_DIVU` | `cpu_ie64.go` const `OP_DIVU`; execute switch case `OP_DIVU`; step switch case `OP_DIVU` |
| IE64 | opcode | 0x25 | `OP_DIVS` | `cpu_ie64.go` const `OP_DIVS`; execute switch case `OP_DIVS`; step switch case `OP_DIVS` |
| IE64 | opcode | 0x26 | `OP_MOD64` | `cpu_ie64.go` const `OP_MOD64`; execute switch case `OP_MOD64`; step switch case `OP_MOD64` |
| IE64 | opcode | 0x27 | `OP_NEG` | `cpu_ie64.go` const `OP_NEG`; execute switch case `OP_NEG`; step switch case `OP_NEG` |
| IE64 | opcode | 0x28 | `OP_MODS` | `cpu_ie64.go` const `OP_MODS`; execute switch case `OP_MODS`; step switch case `OP_MODS` |
| IE64 | opcode | 0x29 | `OP_MULHU` | `cpu_ie64.go` const `OP_MULHU`; execute switch case `OP_MULHU`; step switch case `OP_MULHU` |
| IE64 | opcode | 0x2A | `OP_MULHS` | `cpu_ie64.go` const `OP_MULHS`; execute switch case `OP_MULHS`; step switch case `OP_MULHS` |
| IE64 | opcode | 0x30 | `OP_AND64` | `cpu_ie64.go` const `OP_AND64`; execute switch case `OP_AND64`; step switch case `OP_AND64` |
| IE64 | opcode | 0x31 | `OP_OR64` | `cpu_ie64.go` const `OP_OR64`; execute switch case `OP_OR64`; step switch case `OP_OR64` |
| IE64 | opcode | 0x32 | `OP_EOR` | `cpu_ie64.go` const `OP_EOR`; execute switch case `OP_EOR`; step switch case `OP_EOR` |
| IE64 | opcode | 0x33 | `OP_NOT64` | `cpu_ie64.go` const `OP_NOT64`; execute switch case `OP_NOT64`; step switch case `OP_NOT64` |
| IE64 | opcode | 0x34 | `OP_LSL` | `cpu_ie64.go` const `OP_LSL`; execute switch case `OP_LSL`; step switch case `OP_LSL` |
| IE64 | opcode | 0x35 | `OP_LSR` | `cpu_ie64.go` const `OP_LSR`; execute switch case `OP_LSR`; step switch case `OP_LSR` |
| IE64 | opcode | 0x36 | `OP_ASR` | `cpu_ie64.go` const `OP_ASR`; execute switch case `OP_ASR`; step switch case `OP_ASR` |
| IE64 | opcode | 0x37 | `OP_CLZ` | `cpu_ie64.go` const `OP_CLZ`; execute switch case `OP_CLZ`; step switch case `OP_CLZ` |
| IE64 | opcode | 0x38 | `OP_SEXT` | `cpu_ie64.go` const `OP_SEXT`; execute switch case `OP_SEXT`; step switch case `OP_SEXT` |
| IE64 | opcode | 0x39 | `OP_ROL` | `cpu_ie64.go` const `OP_ROL`; execute switch case `OP_ROL`; step switch case `OP_ROL` |
| IE64 | opcode | 0x3A | `OP_ROR` | `cpu_ie64.go` const `OP_ROR`; execute switch case `OP_ROR`; step switch case `OP_ROR` |
| IE64 | opcode | 0x3B | `OP_CTZ` | `cpu_ie64.go` const `OP_CTZ`; execute switch case `OP_CTZ`; step switch case `OP_CTZ` |
| IE64 | opcode | 0x3C | `OP_POPCNT` | `cpu_ie64.go` const `OP_POPCNT`; execute switch case `OP_POPCNT`; step switch case `OP_POPCNT` |
| IE64 | opcode | 0x3D | `OP_BSWAP` | `cpu_ie64.go` const `OP_BSWAP`; execute switch case `OP_BSWAP`; step switch case `OP_BSWAP` |
| IE64 | opcode | 0x40 | `OP_BRA` | `cpu_ie64.go` const `OP_BRA`; execute switch case `OP_BRA`; step switch case `OP_BRA` |
| IE64 | opcode | 0x41 | `OP_BEQ` | `cpu_ie64.go` const `OP_BEQ`; execute switch case `OP_BEQ`; step switch case `OP_BEQ` |
| IE64 | opcode | 0x42 | `OP_BNE` | `cpu_ie64.go` const `OP_BNE`; execute switch case `OP_BNE`; step switch case `OP_BNE` |
| IE64 | opcode | 0x43 | `OP_BLT` | `cpu_ie64.go` const `OP_BLT`; execute switch case `OP_BLT`; step switch case `OP_BLT` |
| IE64 | opcode | 0x44 | `OP_BGE` | `cpu_ie64.go` const `OP_BGE`; execute switch case `OP_BGE`; step switch case `OP_BGE` |
| IE64 | opcode | 0x45 | `OP_BGT` | `cpu_ie64.go` const `OP_BGT`; execute switch case `OP_BGT`; step switch case `OP_BGT` |
| IE64 | opcode | 0x46 | `OP_BLE` | `cpu_ie64.go` const `OP_BLE`; execute switch case `OP_BLE`; step switch case `OP_BLE` |
| IE64 | opcode | 0x47 | `OP_BHI` | `cpu_ie64.go` const `OP_BHI`; execute switch case `OP_BHI`; step switch case `OP_BHI` |
| IE64 | opcode | 0x48 | `OP_BLS` | `cpu_ie64.go` const `OP_BLS`; execute switch case `OP_BLS`; step switch case `OP_BLS` |
| IE64 | opcode | 0x49 | `OP_JMP` | `cpu_ie64.go` const `OP_JMP`; execute switch case `OP_JMP`; step switch case `OP_JMP` |
| IE64 | opcode | 0x50 | `OP_JSR64` | `cpu_ie64.go` const `OP_JSR64`; execute switch case `OP_JSR64`; step switch case `OP_JSR64` |
| IE64 | opcode | 0x51 | `OP_RTS64` | `cpu_ie64.go` const `OP_RTS64`; execute switch case `OP_RTS64`; step switch case `OP_RTS64` |
| IE64 | opcode | 0x52 | `OP_PUSH64` | `cpu_ie64.go` const `OP_PUSH64`; execute switch case `OP_PUSH64`; step switch case `OP_PUSH64` |
| IE64 | opcode | 0x53 | `OP_POP64` | `cpu_ie64.go` const `OP_POP64`; execute switch case `OP_POP64`; step switch case `OP_POP64` |
| IE64 | opcode | 0x54 | `OP_JSR_IND` | `cpu_ie64.go` const `OP_JSR_IND`; execute switch case `OP_JSR_IND`; step switch case `OP_JSR_IND` |
| IE64 | opcode | 0x60 | `OP_FMOV` | `cpu_ie64.go` const `OP_FMOV`; execute switch case `OP_FMOV`; step switch case `OP_FMOV` |
| IE64 | opcode | 0x61 | `OP_FLOAD` | `cpu_ie64.go` const `OP_FLOAD`; execute switch case `OP_FLOAD`; step switch case `OP_FLOAD` |
| IE64 | opcode | 0x62 | `OP_FSTORE` | `cpu_ie64.go` const `OP_FSTORE`; execute switch case `OP_FSTORE`; step switch case `OP_FSTORE` |
| IE64 | opcode | 0x63 | `OP_FADD` | `cpu_ie64.go` const `OP_FADD`; execute switch case `OP_FADD`; step switch case `OP_FADD` |
| IE64 | opcode | 0x64 | `OP_FSUB` | `cpu_ie64.go` const `OP_FSUB`; execute switch case `OP_FSUB`; step switch case `OP_FSUB` |
| IE64 | opcode | 0x65 | `OP_FMUL` | `cpu_ie64.go` const `OP_FMUL`; execute switch case `OP_FMUL`; step switch case `OP_FMUL` |
| IE64 | opcode | 0x66 | `OP_FDIV` | `cpu_ie64.go` const `OP_FDIV`; execute switch case `OP_FDIV`; step switch case `OP_FDIV` |
| IE64 | opcode | 0x67 | `OP_FMOD` | `cpu_ie64.go` const `OP_FMOD`; execute switch case `OP_FMOD`; step switch case `OP_FMOD` |
| IE64 | opcode | 0x68 | `OP_FABS` | `cpu_ie64.go` const `OP_FABS`; execute switch case `OP_FABS`; step switch case `OP_FABS` |
| IE64 | opcode | 0x69 | `OP_FNEG` | `cpu_ie64.go` const `OP_FNEG`; execute switch case `OP_FNEG`; step switch case `OP_FNEG` |
| IE64 | opcode | 0x6A | `OP_FSQRT` | `cpu_ie64.go` const `OP_FSQRT`; execute switch case `OP_FSQRT`; step switch case `OP_FSQRT` |
| IE64 | opcode | 0x6B | `OP_FINT` | `cpu_ie64.go` const `OP_FINT`; execute switch case `OP_FINT`; step switch case `OP_FINT` |
| IE64 | opcode | 0x6C | `OP_FCMP` | `cpu_ie64.go` const `OP_FCMP`; execute switch case `OP_FCMP`; step switch case `OP_FCMP` |
| IE64 | opcode | 0x6D | `OP_FCVTIF` | `cpu_ie64.go` const `OP_FCVTIF`; execute switch case `OP_FCVTIF`; step switch case `OP_FCVTIF` |
| IE64 | opcode | 0x6E | `OP_FCVTFI` | `cpu_ie64.go` const `OP_FCVTFI`; execute switch case `OP_FCVTFI`; step switch case `OP_FCVTFI` |
| IE64 | opcode | 0x6F | `OP_FMOVI` | `cpu_ie64.go` const `OP_FMOVI`; execute switch case `OP_FMOVI`; step switch case `OP_FMOVI` |
| IE64 | opcode | 0x70 | `OP_FMOVO` | `cpu_ie64.go` const `OP_FMOVO`; execute switch case `OP_FMOVO`; step switch case `OP_FMOVO` |
| IE64 | opcode | 0x71 | `OP_FSIN` | `cpu_ie64.go` const `OP_FSIN`; execute switch case `OP_FSIN`; step switch case `OP_FSIN` |
| IE64 | opcode | 0x72 | `OP_FCOS` | `cpu_ie64.go` const `OP_FCOS`; execute switch case `OP_FCOS`; step switch case `OP_FCOS` |
| IE64 | opcode | 0x73 | `OP_FTAN` | `cpu_ie64.go` const `OP_FTAN`; execute switch case `OP_FTAN`; step switch case `OP_FTAN` |
| IE64 | opcode | 0x74 | `OP_FATAN` | `cpu_ie64.go` const `OP_FATAN`; execute switch case `OP_FATAN`; step switch case `OP_FATAN` |
| IE64 | opcode | 0x75 | `OP_FLOG` | `cpu_ie64.go` const `OP_FLOG`; execute switch case `OP_FLOG`; step switch case `OP_FLOG` |
| IE64 | opcode | 0x76 | `OP_FEXP` | `cpu_ie64.go` const `OP_FEXP`; execute switch case `OP_FEXP`; step switch case `OP_FEXP` |
| IE64 | opcode | 0x77 | `OP_FPOW` | `cpu_ie64.go` const `OP_FPOW`; execute switch case `OP_FPOW`; step switch case `OP_FPOW` |
| IE64 | opcode | 0x78 | `OP_FMOVECR` | `cpu_ie64.go` const `OP_FMOVECR`; execute switch case `OP_FMOVECR`; step switch case `OP_FMOVECR` |
| IE64 | opcode | 0x79 | `OP_FMOVSR` | `cpu_ie64.go` const `OP_FMOVSR`; execute switch case `OP_FMOVSR`; step switch case `OP_FMOVSR` |
| IE64 | opcode | 0x7A | `OP_FMOVCR` | `cpu_ie64.go` const `OP_FMOVCR`; execute switch case `OP_FMOVCR`; step switch case `OP_FMOVCR` |
| IE64 | opcode | 0x7B | `OP_FMOVSC` | `cpu_ie64.go` const `OP_FMOVSC`; execute switch case `OP_FMOVSC`; step switch case `OP_FMOVSC` |
| IE64 | opcode | 0x7C | `OP_FMOVCC` | `cpu_ie64.go` const `OP_FMOVCC`; execute switch case `OP_FMOVCC`; step switch case `OP_FMOVCC` |
| IE64 | opcode | 0x80 | `OP_DMOV` | `cpu_ie64.go` const `OP_DMOV`; execute switch case `OP_DMOV`; step switch case `OP_DMOV` |
| IE64 | opcode | 0x81 | `OP_DLOAD` | `cpu_ie64.go` const `OP_DLOAD`; execute switch case `OP_DLOAD`; step switch case `OP_DLOAD` |
| IE64 | opcode | 0x82 | `OP_DSTORE` | `cpu_ie64.go` const `OP_DSTORE`; execute switch case `OP_DSTORE`; step switch case `OP_DSTORE` |
| IE64 | opcode | 0x83 | `OP_DADD` | `cpu_ie64.go` const `OP_DADD`; execute switch case `OP_DADD`; step switch case `OP_DADD` |
| IE64 | opcode | 0x84 | `OP_DSUB` | `cpu_ie64.go` const `OP_DSUB`; execute switch case `OP_DSUB`; step switch case `OP_DSUB` |
| IE64 | opcode | 0x85 | `OP_DMUL` | `cpu_ie64.go` const `OP_DMUL`; execute switch case `OP_DMUL`; step switch case `OP_DMUL` |
| IE64 | opcode | 0x86 | `OP_DDIV` | `cpu_ie64.go` const `OP_DDIV`; execute switch case `OP_DDIV`; step switch case `OP_DDIV` |
| IE64 | opcode | 0x87 | `OP_DMOD` | `cpu_ie64.go` const `OP_DMOD`; execute switch case `OP_DMOD`; step switch case `OP_DMOD` |
| IE64 | opcode | 0x88 | `OP_DABS` | `cpu_ie64.go` const `OP_DABS`; execute switch case `OP_DABS`; step switch case `OP_DABS` |
| IE64 | opcode | 0x89 | `OP_DNEG` | `cpu_ie64.go` const `OP_DNEG`; execute switch case `OP_DNEG`; step switch case `OP_DNEG` |
| IE64 | opcode | 0x8A | `OP_DSQRT` | `cpu_ie64.go` const `OP_DSQRT`; execute switch case `OP_DSQRT`; step switch case `OP_DSQRT` |
| IE64 | opcode | 0x8B | `OP_DINT` | `cpu_ie64.go` const `OP_DINT`; execute switch case `OP_DINT`; step switch case `OP_DINT` |
| IE64 | opcode | 0x8C | `OP_DCMP` | `cpu_ie64.go` const `OP_DCMP`; execute switch case `OP_DCMP`; step switch case `OP_DCMP` |
| IE64 | opcode | 0x8D | `OP_DCVTIF` | `cpu_ie64.go` const `OP_DCVTIF`; execute switch case `OP_DCVTIF`; step switch case `OP_DCVTIF` |
| IE64 | opcode | 0x8E | `OP_DCVTFI` | `cpu_ie64.go` const `OP_DCVTFI`; execute switch case `OP_DCVTFI`; step switch case `OP_DCVTFI` |
| IE64 | opcode | 0x8F | `OP_FCVTSD` | `cpu_ie64.go` const `OP_FCVTSD`; execute switch case `OP_FCVTSD`; step switch case `OP_FCVTSD` |
| IE64 | opcode | 0x90 | `OP_FCVTDS` | `cpu_ie64.go` const `OP_FCVTDS`; execute switch case `OP_FCVTDS`; step switch case `OP_FCVTDS` |
| IE64 | opcode | 0x91 | `OP_DSIN` | `cpu_ie64.go` const `OP_DSIN`; execute switch case `OP_DSIN`; step switch case `OP_DSIN` |
| IE64 | opcode | 0x92 | `OP_DCOS` | `cpu_ie64.go` const `OP_DCOS`; execute switch case `OP_DCOS`; step switch case `OP_DCOS` |
| IE64 | opcode | 0x93 | `OP_DTAN` | `cpu_ie64.go` const `OP_DTAN`; execute switch case `OP_DTAN`; step switch case `OP_DTAN` |
| IE64 | opcode | 0x94 | `OP_DATAN` | `cpu_ie64.go` const `OP_DATAN`; execute switch case `OP_DATAN`; step switch case `OP_DATAN` |
| IE64 | opcode | 0x95 | `OP_DLOG` | `cpu_ie64.go` const `OP_DLOG`; execute switch case `OP_DLOG`; step switch case `OP_DLOG` |
| IE64 | opcode | 0x96 | `OP_DEXP` | `cpu_ie64.go` const `OP_DEXP`; execute switch case `OP_DEXP`; step switch case `OP_DEXP` |
| IE64 | opcode | 0x97 | `OP_DPOW` | `cpu_ie64.go` const `OP_DPOW`; execute switch case `OP_DPOW`; step switch case `OP_DPOW` |
| IE64 | opcode | 0xE0 | `OP_NOP64` | `cpu_ie64.go` const `OP_NOP64`; execute switch case `OP_NOP64`; step switch case `OP_NOP64` |
| IE64 | opcode | 0xE1 | `OP_HALT64` | `cpu_ie64.go` const `OP_HALT64`; execute switch case `OP_HALT64`; step switch case `OP_HALT64` |
| IE64 | opcode | 0xE2 | `OP_SEI64` | `cpu_ie64.go` const `OP_SEI64`; execute switch case `OP_SEI64`; step switch case `OP_SEI64` |
| IE64 | opcode | 0xE3 | `OP_CLI64` | `cpu_ie64.go` const `OP_CLI64`; execute switch case `OP_CLI64`; step switch case `OP_CLI64` |
| IE64 | opcode | 0xE4 | `OP_RTI64` | `cpu_ie64.go` const `OP_RTI64`; execute switch case `OP_RTI64`; step switch case `OP_RTI64` |
| IE64 | opcode | 0xE5 | `OP_WAIT64` | `cpu_ie64.go` const `OP_WAIT64`; execute switch case `OP_WAIT64`; step switch case `OP_WAIT64` |
| IE64 | opcode | 0xE6 | `OP_MTCR` | `cpu_ie64.go` const `OP_MTCR`; execute switch case `OP_MTCR`; step switch case `OP_MTCR` |
| IE64 | opcode | 0xE7 | `OP_MFCR` | `cpu_ie64.go` const `OP_MFCR`; execute switch case `OP_MFCR`; step switch case `OP_MFCR` |
| IE64 | opcode | 0xE8 | `OP_ERET` | `cpu_ie64.go` const `OP_ERET`; execute switch case `OP_ERET`; step switch case `OP_ERET` |
| IE64 | opcode | 0xE9 | `OP_TLBFLUSH` | `cpu_ie64.go` const `OP_TLBFLUSH`; execute switch case `OP_TLBFLUSH`; step switch case `OP_TLBFLUSH` |
| IE64 | opcode | 0xEA | `OP_TLBINVAL` | `cpu_ie64.go` const `OP_TLBINVAL`; execute switch case `OP_TLBINVAL`; step switch case `OP_TLBINVAL` |
| IE64 | opcode | 0xEB | `OP_SYSCALL` | `cpu_ie64.go` const `OP_SYSCALL`; execute switch case `OP_SYSCALL`; step switch case `OP_SYSCALL` |
| IE64 | opcode | 0xEC | `OP_SMODE` | `cpu_ie64.go` const `OP_SMODE`; execute switch case `OP_SMODE`; step switch case `OP_SMODE` |
| IE64 | opcode | 0xED | `OP_CAS` | `cpu_ie64.go` const `OP_CAS`; execute switch case `OP_CAS`; step switch case `OP_CAS` |
| IE64 | opcode | 0xEE | `OP_XCHG` | `cpu_ie64.go` const `OP_XCHG`; execute switch case `OP_XCHG`; step switch case `OP_XCHG` |
| IE64 | opcode | 0xEF | `OP_FAA` | `cpu_ie64.go` const `OP_FAA`; execute switch case `OP_FAA`; step switch case `OP_FAA` |
| IE64 | opcode | 0xF0 | `OP_FAND` | `cpu_ie64.go` const `OP_FAND`; execute switch case `OP_FAND`; step switch case `OP_FAND` |
| IE64 | opcode | 0xF1 | `OP_FOR` | `cpu_ie64.go` const `OP_FOR`; execute switch case `OP_FOR`; step switch case `OP_FOR` |
| IE64 | opcode | 0xF2 | `OP_FXOR` | `cpu_ie64.go` const `OP_FXOR`; execute switch case `OP_FXOR`; step switch case `OP_FXOR` |
| IE64 | opcode | 0xF3 | `OP_SUAEN` | `cpu_ie64.go` const `OP_SUAEN`; execute switch case `OP_SUAEN`; step switch case `OP_SUAEN` |
| IE64 | opcode | 0xF4 | `OP_SUADIS` | `cpu_ie64.go` const `OP_SUADIS`; execute switch case `OP_SUADIS`; step switch case `OP_SUADIS` |
| IE64 | fpu side effect | 0x63 | `OP_FADD` | `cpu_ie64.go` execute and step cases for `OP_FADD`; `fpu_ie64.go` `FADD` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x64 | `OP_FSUB` | `cpu_ie64.go` execute and step cases for `OP_FSUB`; `fpu_ie64.go` `FSUB` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x65 | `OP_FMUL` | `cpu_ie64.go` execute and step cases for `OP_FMUL`; `fpu_ie64.go` `FMUL` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x66 | `OP_FDIV` | `cpu_ie64.go` execute and step cases for `OP_FDIV`; `fpu_ie64.go` `FDIV` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x67 | `OP_FMOD` | `cpu_ie64.go` execute and step cases for `OP_FMOD`; `fpu_ie64.go` `FMOD` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x68 | `OP_FABS` | `cpu_ie64.go` execute and step cases for `OP_FABS`; `fpu_ie64.go` `FABS` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x69 | `OP_FNEG` | `cpu_ie64.go` execute and step cases for `OP_FNEG`; `fpu_ie64.go` `FNEG` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x6A | `OP_FSQRT` | `cpu_ie64.go` execute and step cases for `OP_FSQRT`; `fpu_ie64.go` `FSQRT` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x6B | `OP_FINT` | `cpu_ie64.go` execute and step cases for `OP_FINT`; `fpu_ie64.go` `FINT` side effects: reads FPCR rounding bits, writes FPSR condition-code bits from the rounded result, and does not set FPSR sticky exception flags |
| IE64 | fpu side effect | 0x71 | `OP_FSIN` | `cpu_ie64.go` execute and step cases for `OP_FSIN`; `fpu_ie64.go` `FSIN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x72 | `OP_FCOS` | `cpu_ie64.go` execute and step cases for `OP_FCOS`; `fpu_ie64.go` `FCOS` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x73 | `OP_FTAN` | `cpu_ie64.go` execute and step cases for `OP_FTAN`; `fpu_ie64.go` `FTAN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x74 | `OP_FATAN` | `cpu_ie64.go` execute and step cases for `OP_FATAN`; `fpu_ie64.go` `FATAN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x75 | `OP_FLOG` | `cpu_ie64.go` execute and step cases for `OP_FLOG`; `fpu_ie64.go` `FLOG` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x76 | `OP_FEXP` | `cpu_ie64.go` execute and step cases for `OP_FEXP`; `fpu_ie64.go` `FEXP` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x77 | `OP_FPOW` | `cpu_ie64.go` execute and step cases for `OP_FPOW`; `fpu_ie64.go` `FPOW` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x83 | `OP_DADD` | `cpu_ie64.go` execute and step cases for `OP_DADD`; `fpu_ie64.go` `DADD` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x84 | `OP_DSUB` | `cpu_ie64.go` execute and step cases for `OP_DSUB`; `fpu_ie64.go` `DSUB` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x85 | `OP_DMUL` | `cpu_ie64.go` execute and step cases for `OP_DMUL`; `fpu_ie64.go` `DMUL` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x86 | `OP_DDIV` | `cpu_ie64.go` execute and step cases for `OP_DDIV`; `fpu_ie64.go` `DDIV` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x87 | `OP_DMOD` | `cpu_ie64.go` execute and step cases for `OP_DMOD`; `fpu_ie64.go` `DMOD` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x88 | `OP_DABS` | `cpu_ie64.go` execute and step cases for `OP_DABS`; `fpu_ie64.go` `DABS` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x89 | `OP_DNEG` | `cpu_ie64.go` execute and step cases for `OP_DNEG`; `fpu_ie64.go` `DNEG` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x8A | `OP_DSQRT` | `cpu_ie64.go` execute and step cases for `OP_DSQRT`; `fpu_ie64.go` `DSQRT` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x8B | `OP_DINT` | `cpu_ie64.go` execute and step cases for `OP_DINT`; `fpu_ie64.go` `DINT` side effects: reads FPCR rounding bits, writes FPSR condition-code bits from the rounded result, and does not set FPSR sticky exception flags |
| IE64 | fpu side effect | 0x91 | `OP_DSIN` | `cpu_ie64.go` execute and step cases for `OP_DSIN`; `fpu_ie64.go` `DSIN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x92 | `OP_DCOS` | `cpu_ie64.go` execute and step cases for `OP_DCOS`; `fpu_ie64.go` `DCOS` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x93 | `OP_DTAN` | `cpu_ie64.go` execute and step cases for `OP_DTAN`; `fpu_ie64.go` `DTAN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x94 | `OP_DATAN` | `cpu_ie64.go` execute and step cases for `OP_DATAN`; `fpu_ie64.go` `DATAN` side effects: writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x95 | `OP_DLOG` | `cpu_ie64.go` execute and step cases for `OP_DLOG`; `fpu_ie64.go` `DLOG` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x96 | `OP_DEXP` | `cpu_ie64.go` execute and step cases for `OP_DEXP`; `fpu_ie64.go` `DEXP` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | fpu side effect | 0x97 | `OP_DPOW` | `cpu_ie64.go` execute and step cases for `OP_DPOW`; `fpu_ie64.go` `DPOW` side effects: writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read |
| IE64 | control register | 0x00 | `CR_PTBR` | `cpu_ie64.go` const `CR_PTBR`; MFCR switch case `CR_PTBR`; MTCR switch case `CR_PTBR` |
| IE64 | control register | 0x01 | `CR_FAULT_ADDR` | `cpu_ie64.go` const `CR_FAULT_ADDR`; MFCR switch case `CR_FAULT_ADDR`; MTCR switch case `CR_FAULT_ADDR` |
| IE64 | control register | 0x02 | `CR_FAULT_CAUSE` | `cpu_ie64.go` const `CR_FAULT_CAUSE`; MFCR switch case `CR_FAULT_CAUSE`; MTCR switch case `CR_FAULT_CAUSE` |
| IE64 | control register | 0x03 | `CR_FAULT_PC` | `cpu_ie64.go` const `CR_FAULT_PC`; MFCR switch case `CR_FAULT_PC`; MTCR switch case `CR_FAULT_PC` |
| IE64 | control register | 0x04 | `CR_TRAP_VEC` | `cpu_ie64.go` const `CR_TRAP_VEC`; MFCR switch case `CR_TRAP_VEC`; MTCR switch case `CR_TRAP_VEC` |
| IE64 | control register | 0x05 | `CR_MMU_CTRL` | `cpu_ie64.go` const `CR_MMU_CTRL`; MFCR switch case `CR_MMU_CTRL`; MTCR switch case `CR_MMU_CTRL` |
| IE64 | control register | 0x06 | `CR_TP` | `cpu_ie64.go` const `CR_TP`; MFCR switch case `CR_TP`; MTCR switch case `CR_TP` |
| IE64 | control register | 0x07 | `CR_INTR_VEC` | `cpu_ie64.go` const `CR_INTR_VEC`; MFCR switch case `CR_INTR_VEC`; MTCR switch case `CR_INTR_VEC` |
| IE64 | control register | 0x08 | `CR_KSP` | `cpu_ie64.go` const `CR_KSP`; MFCR switch case `CR_KSP`; MTCR switch case `CR_KSP` |
| IE64 | control register | 0x09 | `CR_TIMER_PERIOD` | `cpu_ie64.go` const `CR_TIMER_PERIOD`; MFCR switch case `CR_TIMER_PERIOD`; MTCR switch case `CR_TIMER_PERIOD` |
| IE64 | control register | 0x0A | `CR_TIMER_COUNT` | `cpu_ie64.go` const `CR_TIMER_COUNT`; MFCR switch case `CR_TIMER_COUNT`; MTCR switch case `CR_TIMER_COUNT` |
| IE64 | control register | 0x0B | `CR_TIMER_CTRL` | `cpu_ie64.go` const `CR_TIMER_CTRL`; MFCR switch case `CR_TIMER_CTRL`; MTCR switch case `CR_TIMER_CTRL` |
| IE64 | control register | 0x0C | `CR_USP` | `cpu_ie64.go` const `CR_USP`; MFCR switch case `CR_USP`; MTCR switch case `CR_USP` |
| IE64 | control register | 0x0D | `CR_PREV_MODE` | `cpu_ie64.go` const `CR_PREV_MODE`; MFCR switch case `CR_PREV_MODE`; no MTCR switch case for `CR_PREV_MODE` |
| IE64 | control register | 0x0E | `CR_SAVED_SUA` | `cpu_ie64.go` const `CR_SAVED_SUA`; MFCR switch case `CR_SAVED_SUA`; MTCR switch case `CR_SAVED_SUA` |
| IE64 | control register | 0x0F | `CR_RAM_SIZE_BYTES` | `cpu_ie64.go` const `CR_RAM_SIZE_BYTES`; MFCR switch case `CR_RAM_SIZE_BYTES`; MTCR illegal-instruction check `CR_RAM_SIZE_BYTES` |
| IE32 | opcode | 0x01 | `LOAD` | `cpu_ie32.go` const `LOAD`; execute switch case `LOAD`; step switch case `LOAD` |
| IE32 | opcode | 0x02 | `STORE` | `cpu_ie32.go` const `STORE`; execute switch case `STORE`; step switch case `STORE` |
| IE32 | opcode | 0x03 | `ADD` | `cpu_ie32.go` const `ADD`; execute switch case `ADD`; step switch case `ADD` |
| IE32 | opcode | 0x04 | `SUB` | `cpu_ie32.go` const `SUB`; execute switch case `SUB`; step switch case `SUB` |
| IE32 | opcode | 0x05 | `AND` | `cpu_ie32.go` const `AND`; execute switch case `AND`; step switch case `AND` |
| IE32 | opcode | 0x06 | `JMP` | `cpu_ie32.go` const `JMP`; execute switch case `JMP`; step switch case `JMP` |
| IE32 | opcode | 0x07 | `JNZ` | `cpu_ie32.go` const `JNZ`; execute switch case `JNZ`; step switch case `JNZ` |
| IE32 | opcode | 0x08 | `JZ` | `cpu_ie32.go` const `JZ`; execute switch case `JZ`; step switch case `JZ` |
| IE32 | opcode | 0x09 | `OR` | `cpu_ie32.go` const `OR`; execute switch case `OR`; step switch case `OR` |
| IE32 | opcode | 0x0A | `XOR` | `cpu_ie32.go` const `XOR`; execute switch case `XOR`; step switch case `XOR` |
| IE32 | opcode | 0x0B | `SHL` | `cpu_ie32.go` const `SHL`; execute switch case `SHL`; step switch case `SHL` |
| IE32 | opcode | 0x0C | `SHR` | `cpu_ie32.go` const `SHR`; execute switch case `SHR`; step switch case `SHR` |
| IE32 | opcode | 0x0D | `NOT` | `cpu_ie32.go` const `NOT`; execute switch case `NOT`; step switch case `NOT` |
| IE32 | opcode | 0x0E | `JGT` | `cpu_ie32.go` const `JGT`; execute switch case `JGT`; step switch case `JGT` |
| IE32 | opcode | 0x0F | `JGE` | `cpu_ie32.go` const `JGE`; execute switch case `JGE`; step switch case `JGE` |
| IE32 | opcode | 0x10 | `JLT` | `cpu_ie32.go` const `JLT`; execute switch case `JLT`; step switch case `JLT` |
| IE32 | opcode | 0x11 | `JLE` | `cpu_ie32.go` const `JLE`; execute switch case `JLE`; step switch case `JLE` |
| IE32 | opcode | 0x12 | `PUSH` | `cpu_ie32.go` const `PUSH`; execute switch case `PUSH`; step switch case `PUSH` |
| IE32 | opcode | 0x13 | `POP` | `cpu_ie32.go` const `POP`; execute switch case `POP`; step switch case `POP` |
| IE32 | opcode | 0x14 | `MUL` | `cpu_ie32.go` const `MUL`; execute switch case `MUL`; step switch case `MUL` |
| IE32 | opcode | 0x15 | `DIV` | `cpu_ie32.go` const `DIV`; execute switch case `DIV`; step switch case `DIV` |
| IE32 | opcode | 0x16 | `MOD` | `cpu_ie32.go` const `MOD`; execute switch case `MOD`; step switch case `MOD` |
| IE32 | opcode | 0x17 | `WAIT` | `cpu_ie32.go` const `WAIT`; execute switch case `WAIT`; step switch case `WAIT` |
| IE32 | opcode | 0x18 | `JSR` | `cpu_ie32.go` const `JSR`; execute switch case `JSR`; step switch case `JSR` |
| IE32 | opcode | 0x19 | `RTS` | `cpu_ie32.go` const `RTS`; execute switch case `RTS`; step switch case `RTS` |
| IE32 | opcode | 0x1A | `SEI` | `cpu_ie32.go` const `SEI`; execute switch case `SEI`; step switch case `SEI` |
| IE32 | opcode | 0x1B | `CLI` | `cpu_ie32.go` const `CLI`; execute switch case `CLI`; step switch case `CLI` |
| IE32 | opcode | 0x1C | `RTI` | `cpu_ie32.go` const `RTI`; execute switch case `RTI`; step switch case `RTI` |
| IE32 | opcode | 0x20 | `LDA` | `cpu_ie32.go` const `LDA`; execute switch case `LDA`; step switch case `LDA` |
| IE32 | opcode | 0x21 | `LDX` | `cpu_ie32.go` const `LDX`; execute switch case `LDX`; step switch case `LDX` |
| IE32 | opcode | 0x22 | `LDY` | `cpu_ie32.go` const `LDY`; execute switch case `LDY`; step switch case `LDY` |
| IE32 | opcode | 0x23 | `LDZ` | `cpu_ie32.go` const `LDZ`; execute switch case `LDZ`; step switch case `LDZ` |
| IE32 | opcode | 0x24 | `STA` | `cpu_ie32.go` const `STA`; execute switch case `STA`; step switch case `STA` |
| IE32 | opcode | 0x25 | `STX` | `cpu_ie32.go` const `STX`; execute switch case `STX`; step switch case `STX` |
| IE32 | opcode | 0x26 | `STY` | `cpu_ie32.go` const `STY`; execute switch case `STY`; step switch case `STY` |
| IE32 | opcode | 0x27 | `STZ` | `cpu_ie32.go` const `STZ`; execute switch case `STZ`; step switch case `STZ` |
| IE32 | opcode | 0x28 | `INC` | `cpu_ie32.go` const `INC`; execute switch case `INC`; step switch case `INC` |
| IE32 | opcode | 0x29 | `DEC` | `cpu_ie32.go` const `DEC`; execute switch case `DEC`; step switch case `DEC` |
| IE32 | opcode | 0x3A | `LDB` | `cpu_ie32.go` const `LDB`; execute switch case `LDB`; step switch case `LDB` |
| IE32 | opcode | 0x3B | `LDC` | `cpu_ie32.go` const `LDC`; execute switch case `LDC`; step switch case `LDC` |
| IE32 | opcode | 0x3C | `LDD` | `cpu_ie32.go` const `LDD`; execute switch case `LDD`; step switch case `LDD` |
| IE32 | opcode | 0x3D | `LDE` | `cpu_ie32.go` const `LDE`; execute switch case `LDE`; step switch case `LDE` |
| IE32 | opcode | 0x3E | `LDF` | `cpu_ie32.go` const `LDF`; execute switch case `LDF`; step switch case `LDF` |
| IE32 | opcode | 0x3F | `LDG` | `cpu_ie32.go` const `LDG`; execute switch case `LDG`; step switch case `LDG` |
| IE32 | opcode | 0x40 | `LDU` | `cpu_ie32.go` const `LDU`; execute switch case `LDU`; step switch case `LDU` |
| IE32 | opcode | 0x41 | `LDV` | `cpu_ie32.go` const `LDV`; execute switch case `LDV`; step switch case `LDV` |
| IE32 | opcode | 0x42 | `LDW` | `cpu_ie32.go` const `LDW`; execute switch case `LDW`; step switch case `LDW` |
| IE32 | opcode | 0x43 | `STB` | `cpu_ie32.go` const `STB`; execute switch case `STB`; step switch case `STB` |
| IE32 | opcode | 0x44 | `STC` | `cpu_ie32.go` const `STC`; execute switch case `STC`; step switch case `STC` |
| IE32 | opcode | 0x45 | `STD` | `cpu_ie32.go` const `STD`; execute switch case `STD`; step switch case `STD` |
| IE32 | opcode | 0x46 | `STE` | `cpu_ie32.go` const `STE`; execute switch case `STE`; step switch case `STE` |
| IE32 | opcode | 0x47 | `STF` | `cpu_ie32.go` const `STF`; execute switch case `STF`; step switch case `STF` |
| IE32 | opcode | 0x48 | `STG` | `cpu_ie32.go` const `STG`; execute switch case `STG`; step switch case `STG` |
| IE32 | opcode | 0x49 | `STU` | `cpu_ie32.go` const `STU`; execute switch case `STU`; step switch case `STU` |
| IE32 | opcode | 0x4A | `STV` | `cpu_ie32.go` const `STV`; execute switch case `STV`; step switch case `STV` |
| IE32 | opcode | 0x4B | `STW` | `cpu_ie32.go` const `STW`; execute switch case `STW`; step switch case `STW` |
| IE32 | opcode | 0x4C | `LDH` | `cpu_ie32.go` const `LDH`; execute switch case `LDH`; step switch case `LDH` |
| IE32 | opcode | 0x4D | `LDS` | `cpu_ie32.go` const `LDS`; execute switch case `LDS`; step switch case `LDS` |
| IE32 | opcode | 0x4E | `LDT` | `cpu_ie32.go` const `LDT`; execute switch case `LDT`; step switch case `LDT` |
| IE32 | opcode | 0x4F | `STH` | `cpu_ie32.go` const `STH`; execute switch case `STH`; step switch case `STH` |
| IE32 | opcode | 0x50 | `STS` | `cpu_ie32.go` const `STS`; execute switch case `STS`; step switch case `STS` |
| IE32 | opcode | 0x51 | `STT` | `cpu_ie32.go` const `STT`; execute switch case `STT`; step switch case `STT` |
| IE32 | opcode | 0xEE | `NOP` | `cpu_ie32.go` const `NOP`; execute switch case `NOP`; step switch case `NOP` |
| IE32 | opcode | 0xFF | `HALT` | `cpu_ie32.go` const `HALT`; execute switch case `HALT`; step switch case `HALT` |
