// cpu_m68k.go - Motorola 68EC020 CPU emulation for the Intuition Engine

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
cpu_m68k.go - Motorola 68EC020 CPU Emulation for the Intuition Engine

This module implements a Motorola 68EC020 CPU emulator with 90%+ instruction
coverage, supervisor/user mode separation, hardware interrupts, and memory-mapped I/O.
The 68EC020 is the embedded variant of the 68020 without MMU. Amiga 1200 ftw!

Architectural Features:
- 8 data registers (D0-D7) and 8 address registers (A0-A7, with A7 as stack pointer)
- 32-bit internal and external data paths
- 16MB linear address space (24-bit addressing, no MMU)
- 32-bit programme counter with word alignment
- 16-bit status register (system byte + condition codes)
- Supervisor/user mode privilege separation with exception-based transitions
- Hardware interrupt prioritisation (levels 1-7)
- Exception vector table with 256 vectors (relocatable via VBR)

68020-Specific Enhancements Implemented:
- Bit field operations (BFTST, BFEXTU, BFEXTS, BFCHG, BFCLR, BFSET, BFFFO, BFINS)
- 32-bit multiply and divide (MULU.L, MULS.L, DIVU.L, DIVS.L with 64-bit intermediates)
- Atomic compare-and-swap (CAS, CAS2 for lock-free synchronisation)
- Bounds checking (CHK.L, CHK2, CMP2 for array protection)
- BCD packed arithmetic (PACK, UNPK for decimal data)
- Control register access (MOVEC for VBR, CACR, CAAR, SFC, DFC)
- Address space control (MOVES with function code specification)
- Enhanced addressing modes (memory indirect, scaled indexing, full extension words)
- Module call/return (CALLM, RTM for protected subsystems)
- Sign-extend byte to long (EXTB.L)

Addressing Modes Supported (12 basic + 68020 extensions):
1. Data register direct               Dn
2. Address register direct            An
3. Address register indirect          (An)
4. Address register indirect postincrement  (An)+
5. Address register indirect predecrement   -(An)
6. Address register indirect with displacement  (d16,An)
7. Address register indirect with index     (d8,An,Xn)
8. Absolute short                     (xxx).W
9. Absolute long                      (xxx).L
10. Programme counter with displacement     (d16,PC)
11. Programme counter with index            (d8,PC,Xn)
12. Immediate                                #<data>
13. [68020] Memory indirect preindexed      ([bd,An,Xn],od)
14. [68020] Memory indirect postindexed     ([bd,An],Xn,od)
15. [68020] PC-relative memory indirect     ([bd,PC,Xn],od)
16. [68020] Scaled indexing (×1, ×2, ×4, ×8)

Not Implemented:
- MMU and address translation
- Coprocessor interface (cpGEN, cpScc, etc as they require FPU/MMU chips)
- Instruction cache emulation
- Trace mode (T0/T1 bits defined but not enforced)
- Dynamic bus sizing (assumes 32-bit bus)

Execution Flow:
1. Fetch opcode from memory (big-endian to little-endian conversion)
2. Decode instruction and addressing modes
3. Calculate effective addresses and fetch operands
4. Execute operation with condition code updates
5. Write results to destination
6. Advance programme counter
7. Check for pending interrupts
8. Update cycle counter for timing accuracy

Memory Layout Optimisation (64-bit host system):
The struct layout is cache-line aligned to minimise false sharing in multi-threaded
environments. Hot path registers (PC, SR, data registers) occupy the first cache line
for maximum performance during instruction decode.

Cache Line 0 (64 bytes) - Hot Path:
  PC, SR, DataRegs[8] - Accessed every instruction cycle

Cache Line 1 (64 bytes) - Address Registers:
  AddrRegs[8], USP, execution state flags

Cache Line 2 (64 bytes) - Control Registers:
  VBR, SFC, DFC, CACR, CAAR, cycle counter, prefetch queue

Cache Lines 3+ - Synchronisation:
  Mutexes for thread-safe memory access and interrupt handling

Thread Safety:
All memory operations and register updates are protected by read/write locks,
enabling safe interaction from external threads (interrupt injection, DMA, debugging).
The implementation uses:
- Read locks for instruction fetch and operand reads (concurrent access allowed)
- Write locks for register updates and memory writes (exclusive access required)
- Separate interrupt mutex to prevent race conditions on pending interrupt state

Integration with Intuition Engine:
The emulator integrates with a memory bus abstraction layer that handles:
- Big-endian (68000) to little-endian (x86-64/ARM64) conversion
- Memory-mapped I/O device registration
- DMA and bus arbitration
- Address decoding and chip select logic
*/

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"os"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

// ------------------------------------------------------------------------------
// Core System Constants
// ------------------------------------------------------------------------------
const (
	M68K_BYTE_SIZE      = 1
	M68K_WORD_SIZE      = 2
	M68K_LONG_SIZE      = 4
	M68K_BYTE_SIZE_BITS = 8
	M68K_WORD_SIZE_BITS = 16
	M68K_LONG_SIZE_BITS = 32
	M68K_CACHE_LINE     = 64                    // Optimises struct layout for modern CPU cache architecture
	M68K_MEMORY_SIZE    = DEFAULT_MEMORY_SIZE   // Use unified memory size from machine_bus.go
	M68K_RESET_DELAY    = 50 * time.Millisecond // Hardware requires stabilisation time before resuming
	M68K_PREFETCH_SIZE  = 4                     // Balances fetch overhead with branch misprediction cost
)

// ------------------------------------------------------------------------------
// Memory Layout Constants
// ------------------------------------------------------------------------------
const (
	M68K_STACK_START  = 0x00FF0000 // Provides room for program whilst keeping stack in upper memory
	M68K_VECTOR_SIZE  = 1024       // 256 exception vectors of 4 bytes each
	M68K_RESET_VECTOR = 0x00000004 // Hardware fetches this after initial SP at address 0
	M68K_ENTRY_POINT  = 0x00001000 // Avoids collision with exception vector table
	M68K_IO_BASE      = 0x00F00000 // Memory-mapped I/O begins here
	M68K_IO_LIMIT     = 0x00FFFFFF // Last accessible I/O address
	M68K_ADDRESS_MASK = 0x00FFFFFF // 68000/68EC020 has 24-bit address bus
)

// ------------------------------------------------------------------------------
// Status Register Bit Masks
// ------------------------------------------------------------------------------
const (
	M68K_SR_C     = 0x0001
	M68K_SR_V     = 0x0002
	M68K_SR_Z     = 0x0004
	M68K_SR_N     = 0x0008
	M68K_SR_X     = 0x0010 // Chains multi-precision arithmetic without software propagation
	M68K_SR_IPL   = 0x0700 // Masks interrupts below this priority level
	M68K_SR_S     = 0x2000 // Prevents user code from manipulating system state
	M68K_SR_T0    = 0x4000 // Debugger support for instruction stepping
	M68K_SR_T1    = 0x8000 // Debugger support for procedure-level tracing
	M68K_SR_CCR   = 0x001F // Isolates condition codes from system byte
	M68K_SR_SHIFT = 8
)

// ------------------------------------------------------------------------------
// Instruction Format Constants
// ------------------------------------------------------------------------------

// Size Codes for Instructions
const (
	M68K_SIZE_BYTE = iota
	M68K_SIZE_WORD
	M68K_SIZE_LONG
)

// Addressing Mode Codes
const (
	M68K_AM_DR = iota
	M68K_AM_AR
	M68K_AM_AR_IND
	M68K_AM_AR_POST
	M68K_AM_AR_PRE
	M68K_AM_AR_DISP
	M68K_AM_AR_INDEX
	M68K_AM_ABS_SHORT
	M68K_AM_ABS_LONG
	M68K_AM_PC_DISP
	M68K_AM_PC_INDEX
	M68K_AM_IMM
	M68K_AM_AR_INDEX_DISP8  = 0x20 // 68020 extension for scaled indexing
	M68K_AM_AR_INDEX_BD     = 0x30 // 68020 full extension word format
	M68K_AM_MEM_INDIRECT    = 0x40 // 68020 memory indirection support
	M68K_AM_PC_INDEX_BD     = 0x50 // PC-relative with base displacement
	M68K_AM_PC_MEM_INDIRECT = 0x60 // PC-relative memory indirection
)

// Operand Direction Constants
const (
	M68K_DIRECTION_REG_TO_EA = 0
	M68K_DIRECTION_EA_TO_REG = 1
)

// Extension Word Bit Positions
const (
	M68K_EXT_FULL_FORMAT      = 0x0100 // Distinguishes 68020 full format from 68000 brief format
	M68K_EXT_BRIEF_FORMAT     = 0x0000
	M68K_EXT_BS_BIT           = 7 // Allows omitting base register from calculation
	M68K_EXT_IS_BIT           = 6 // Allows omitting index register from calculation
	M68K_EXT_BD_START_BIT     = 4
	M68K_EXT_BD_SIZE          = 2
	M68K_EXT_SCALE_START_BIT  = 9 // Enables array indexing without separate multiply instruction
	M68K_EXT_SCALE_SIZE       = 2
	M68K_EXT_INDIRECTION_MASK = 0x03
	M68K_EXT_REG_MASK         = 0x0F
	M68K_EXT_REG_TYPE_BIT     = 15 // Distinguishes address vs data register
	M68K_EXT_SIZE_BIT         = 11 // Determines word vs long index
	M68K_EXT_ADDR_REG_TYPE    = 1
	M68K_EXT_DATA_REG_TYPE    = 0
)

// Bit Field Constants
const (
	M68K_BF_OFFSET_REG  = 0x0800 // Dynamic offset for variable bit field positioning
	M68K_BF_WIDTH_REG   = 0x0020 // Dynamic width for variable bit field sizes
	M68K_BF_OFFSET_MASK = 0x07C0
	M68K_BF_WIDTH_MASK  = 0x001F
)

// Special condition flags
const (
	M68K_OVERFLOW_NONE   = 0
	M68K_OVERFLOW_POS    = 1
	M68K_OVERFLOW_NEG    = 2
	M68K_OVERFLOW_MULDIV = 3
)

// ------------------------------------------------------------------------------
// Instruction Condition Codes
// ------------------------------------------------------------------------------
const (
	M68K_CC_T  = 0
	M68K_CC_F  = 1
	M68K_CC_HI = 2 // Unsigned: tests C=0 AND Z=0
	M68K_CC_LS = 3 // Unsigned: tests C=1 OR Z=1
	M68K_CC_CC = 4
	M68K_CC_CS = 5
	M68K_CC_NE = 6
	M68K_CC_EQ = 7
	M68K_CC_VC = 8
	M68K_CC_VS = 9
	M68K_CC_PL = 10
	M68K_CC_MI = 11
	M68K_CC_GE = 12 // Signed: tests N=V
	M68K_CC_LT = 13 // Signed: tests N≠V
	M68K_CC_GT = 14 // Signed: tests Z=0 AND N=V
	M68K_CC_LE = 15 // Signed: tests Z=1 OR N≠V
)

// ------------------------------------------------------------------------------
// Exception and Interrupt Constants
// ------------------------------------------------------------------------------
const (
	M68K_VEC_RESET         = 1
	M68K_VEC_BUS_ERROR     = 2
	M68K_VEC_ADDRESS_ERROR = 3 // Triggered by misaligned word/long access on real hardware
	M68K_VEC_ILLEGAL_INSTR = 4
	M68K_VEC_ZERO_DIVIDE   = 5
	M68K_VEC_CHK           = 6
	M68K_VEC_TRAPV         = 7
	M68K_VEC_PRIVILEGE     = 8
	M68K_VEC_TRACE         = 9
	M68K_VEC_LINE_A        = 10 // Operating system trap mechanism
	M68K_VEC_LINE_F        = 11 // FPU/coprocessor trap mechanism
	M68K_VEC_FORMAT_ERROR  = 14 // Exception stack frame format error
	M68K_VEC_SPURIOUS      = 24 // Hardware acknowledged interrupt but no device responded
	M68K_VEC_LEVEL1        = 25
	M68K_VEC_LEVEL2        = 26
	M68K_VEC_LEVEL3        = 27
	M68K_VEC_LEVEL4        = 28
	M68K_VEC_LEVEL5        = 29
	M68K_VEC_LEVEL6        = 30
	M68K_VEC_LEVEL7        = 31 // NMI cannot be masked
	M68K_VEC_TRAP_BASE     = 32
	M68K_VEC_USER          = 64
	M68K_VEC_BKPT          = 46
)

// ------------------------------------------------------------------------------
// Cycle Timing Constants
// ------------------------------------------------------------------------------
const (
	// Base cycle timings - reflects 68020 instruction pipeline behaviour
	M68K_CYCLE_FETCH     = 4
	M68K_CYCLE_DECODE    = 2
	M68K_CYCLE_EXECUTE   = 2
	M68K_CYCLE_MEM_READ  = 4
	M68K_CYCLE_MEM_WRITE = 4
	M68K_CYCLE_REG       = 2
	M68K_CYCLE_BRANCH    = 10 // Includes pipeline flush penalty
	M68K_CYCLE_EXCEPTION = 20 // Includes stack frame creation overhead

	// MOVE instruction timing - varies by operand location and size
	M68K_CYCLE_MOVE_RR_B = 4
	M68K_CYCLE_MOVE_RR_W = 4
	M68K_CYCLE_MOVE_RR_L = 4
	M68K_CYCLE_MOVE_RM_B = 8
	M68K_CYCLE_MOVE_RM_W = 8
	M68K_CYCLE_MOVE_RM_L = 12 // Long requires two bus cycles for memory write
	M68K_CYCLE_MOVE_MR_B = 4  // Add EA calculation time
	M68K_CYCLE_MOVE_MR_W = 4  // Add EA calculation time
	M68K_CYCLE_MOVE_MR_L = 6  // Add EA calculation time
	M68K_CYCLE_MOVE_MM_B = 8  // Add both source and destination EA times
	M68K_CYCLE_MOVE_MM_W = 8  // Add both source and destination EA times
	M68K_CYCLE_MOVE_MM_L = 12 // Add both source and destination EA times

	// Effective address calculation timing - complexity reflects addressing mode
	M68K_CYCLE_EA_RD   = 0 // No calculation needed
	M68K_CYCLE_EA_AI   = 4
	M68K_CYCLE_EA_PI   = 4
	M68K_CYCLE_EA_PD   = 6  // Extra cycle for decrement operation
	M68K_CYCLE_EA_DI   = 8  // Requires fetching displacement word
	M68K_CYCLE_EA_IX   = 10 // Requires fetching extension word and scaling
	M68K_CYCLE_EA_AW   = 8
	M68K_CYCLE_EA_AL   = 12 // Requires fetching two extension words
	M68K_CYCLE_EA_PCDI = 8
	M68K_CYCLE_EA_PCIX = 10
	M68K_CYCLE_EA_IM   = 4

	// Arithmetic operation timing
	M68K_CYCLE_ADD_R_B  = 4
	M68K_CYCLE_ADD_R_W  = 4
	M68K_CYCLE_ADD_R_L  = 6 // Long operations need extra ALU cycle
	M68K_CYCLE_ADD_M_B  = 4 // Add EA calculation time
	M68K_CYCLE_ADD_M_W  = 4 // Add EA calculation time
	M68K_CYCLE_ADD_M_L  = 6 // Add EA calculation time
	M68K_CYCLE_ADDX_R_B = 4
	M68K_CYCLE_ADDX_R_W = 4
	M68K_CYCLE_ADDX_R_L = 8
	M68K_CYCLE_ADDX_M_B = 18 // Memory-to-memory with read-modify-write
	M68K_CYCLE_ADDX_M_W = 18 // Memory-to-memory with read-modify-write
	M68K_CYCLE_ADDX_M_L = 30 // Memory-to-memory with read-modify-write

	// Multiplication/division timing - actual cycles vary with operand values
	M68K_CYCLE_MULU_W = 38 // Best case; worst case can be 70+ cycles
	M68K_CYCLE_MULU_L = 40 // Best case; worst case can be 70+ cycles
	M68K_CYCLE_MULS_W = 38 // Best case; worst case can be 70+ cycles
	M68K_CYCLE_MULS_L = 40 // Best case; worst case can be 70+ cycles
	M68K_CYCLE_DIVU_W = 38 // Best case; worst case can be 140+ cycles
	M68K_CYCLE_DIVU_L = 40 // Best case; worst case can be 140+ cycles
	M68K_CYCLE_DIVS_W = 38 // Best case; worst case can be 158+ cycles
	M68K_CYCLE_DIVS_L = 40 // Best case; worst case can be 158+ cycles

	// Special instruction timing
	M68K_CYCLE_BSR   = 18
	M68K_CYCLE_RTS   = 16
	M68K_CYCLE_RTR   = 20 // Restores CCR from stack
	M68K_CYCLE_RTE   = 24 // Restores SR and PC from stack frame
	M68K_CYCLE_LINK  = 16
	M68K_CYCLE_UNLK  = 12
	M68K_CYCLE_TRAP  = 38
	M68K_CYCLE_TAS_R = 4
	M68K_CYCLE_TAS_M = 18 // Atomic read-modify-write with bus locking
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Data Movement
// ------------------------------------------------------------------------------
const (
	M68K_MOVE_BYTE       = 0x1000
	M68K_MOVE_LONG       = 0x2000
	M68K_MOVE_WORD       = 0x3000
	M68K_MOVEQ           = 0x7000 // Hardware sign-extends 8-bit to 32-bit during decode
	M68K_MOVE_IMM_TO_A_L = 0x207C
	M68K_MOVE_IMM_TO_A_W = 0x307C
	M68K_MOVE_FROM_SR    = 0x40C0 // Privileged to prevent security information leaks
	M68K_MOVE_TO_SR      = 0x46C0 // Privileged to prevent privilege escalation
	M68K_MOVE_USP        = 0x4E60 // Privileged for context switching
	M68K_MOVEM           = 0x4880
	M68K_LEA             = 0x41C0 // Faster than MOVE when only address is needed
	M68K_PEA             = 0x4840
	M68K_EXT             = 0x4880
	M68K_SWAP            = 0x4840
	M68K_CLR             = 0x4200
	M68K_TST             = 0x4A00
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Integer Arithmetic
// ------------------------------------------------------------------------------
const (
	M68K_ADD  = 0xD000
	M68K_ADDA = 0xD0C0 // Preserves flags because address arithmetic is unsigned
	M68K_ADDI = 0x0600
	M68K_ADDQ = 0x5000 // Value 0 encodes as 8 for efficiency
	M68K_ADDX = 0xD100 // Uses X flag for multi-precision arithmetic
	M68K_SUB  = 0x9000
	M68K_SUBA = 0x90C0 // Preserves flags because address arithmetic is unsigned
	M68K_SUBI = 0x0400
	M68K_SUBQ = 0x5100 // Value 0 encodes as 8 for efficiency
	M68K_SUBX = 0x9100 // Uses X flag for multi-precision arithmetic
	M68K_NEG  = 0x4400
	M68K_NEGX = 0x4000 // Uses X flag for multi-precision arithmetic
	M68K_MULU = 0xC0C0
	M68K_MULS = 0xC1C0
	M68K_DIVU = 0x80C0
	M68K_DIVS = 0x81C0
	M68K_CMP  = 0xB000
	M68K_CMPA = 0xB0C0
	M68K_CMPI = 0x0C00
	M68K_CMPM = 0xB108 // Efficient for comparing memory blocks
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Logical Operations
// ------------------------------------------------------------------------------
const (
	M68K_AND  = 0xC000
	M68K_ANDI = 0x0200
	M68K_OR   = 0x8000
	M68K_ORI  = 0x0000
	M68K_EOR  = 0xB100
	M68K_EORI = 0x0A00
	M68K_NOT  = 0x4600
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Shift and Rotate
// ------------------------------------------------------------------------------
const (
	M68K_ASL       = 0xE100 // Arithmetic preserves sign for signed multiplication/division
	M68K_ASR       = 0xE000 // Arithmetic preserves sign for signed multiplication/division
	M68K_LSL       = 0xE108 // Logical for unsigned operations
	M68K_LSR       = 0xE008 // Logical for unsigned operations
	M68K_ROL       = 0xE118
	M68K_ROR       = 0xE018
	M68K_ROXL      = 0xE110 // Nine-bit rotate including X flag
	M68K_ROXR      = 0xE010 // Nine-bit rotate including X flag
	M68K_SHIFT_MEM = 0xE0C0 // Memory form always shifts by 1
	M68K_SHIFT_ROT = 0xE000
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Bit Manipulation
// ------------------------------------------------------------------------------
const (
	M68K_BCHG = 0x0140
	M68K_BCLR = 0x0180
	M68K_BSET = 0x01C0
	M68K_BTST = 0x0100 // Read-only test for non-destructive checking
	M68K_TAS  = 0x4AC0 // Hardware bus lock prevents race conditions
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - 68020 Bit Field Operations
// ------------------------------------------------------------------------------
const (
	M68K_BFTST  = 0xE8C0 // Efficient for packed data structures
	M68K_BFEXTU = 0xE9C0
	M68K_BFCHG  = 0xEAC0
	M68K_BFEXTS = 0xEBC0
	M68K_BFCLR  = 0xECC0
	M68K_BFFFO  = 0xEDC0 // Hardware accelerates bit scanning
	M68K_BFSET  = 0xEEC0
	M68K_BFINS  = 0xEFC0
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - BCD Operations
// ------------------------------------------------------------------------------
const (
	M68K_ABCD = 0xC100 // Hardware BCD avoids software conversion overhead
	M68K_SBCD = 0x8100 // Hardware BCD avoids software conversion overhead
	M68K_NBCD = 0x4800 // Hardware BCD avoids software conversion overhead
	M68K_PACK = 0x8140
	M68K_UNPK = 0x8180
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Program Control
// ------------------------------------------------------------------------------
const (
	M68K_BRA       = 0x6000
	M68K_BSR       = 0x6100
	M68K_Bcc       = 0x6200
	M68K_DBcc      = 0x50C8 // Hardware decrement-and-branch for efficient loops
	M68K_JMP       = 0x4EC0
	M68K_JSR       = 0x4E80
	M68K_RTR       = 0x4E77 // Preserves supervisor state when returning from user code
	M68K_RTS       = 0x4E75
	M68K_RTE       = 0x4E73 // Privileged to prevent privilege escalation
	M68K_NOP       = 0x4E71
	M68K_TRAP      = 0x4E40
	M68K_TRAPV     = 0x4E76 // Signed overflow detection for safe arithmetic
	M68K_CHK       = 0x4180 // Array bounds enforcement prevents buffer overflows
	M68K_RESET     = 0x4E70 // Privileged because it affects all system hardware
	M68K_STOP      = 0x4E72 // Privileged to prevent denial of service
	M68K_LINK      = 0x4E50 // Hardware stack frame for debuggers and profilers
	M68K_UNLK      = 0x4E58 // Hardware stack frame for debuggers and profilers
	M68K_CHK2_CMP2 = 0x00C0 // Tests both lower and upper bounds in one operation
	M68K_Scc       = 0x50C0 // Materialises boolean values for data-driven control flow
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - System Control
// ------------------------------------------------------------------------------
const (
	M68K_ANDI_TO_SR  = 0x027C // Privileged to prevent interrupt mask manipulation
	M68K_EORI_TO_SR  = 0x0A7C // Privileged to prevent interrupt mask manipulation
	M68K_ORI_TO_SR   = 0x007C // Privileged to prevent interrupt mask manipulation
	M68K_ANDI_TO_CCR = 0x023C
	M68K_EORI_TO_CCR = 0x0A3C
	M68K_ORI_TO_CCR  = 0x003C
	M68K_BKPT        = 0x4848 // Triggers debugger without modifying programme
	M68K_CAS         = 0x0AC0 // Lock-free synchronisation primitive
	M68K_CAS2        = 0x0CFC // Lock-free double-word synchronisation
)

// Exception frame format codes (68020)
const (
	M68K_FRAME_FMT_0 = 0x0
	M68K_FRAME_FMT_1 = 0x1
	M68K_FRAME_FMT_2 = 0x2
	M68K_FRAME_FMT_9 = 0x9
	M68K_FRAME_FMT_A = 0xA
	M68K_FRAME_FMT_B = 0xB
)

// ------------------------------------------------------------------------------
// Instruction Opcodes - Privileged and Miscellaneous
// ------------------------------------------------------------------------------
const (
	M68K_CALLM = 0x06C0 // Hardware-enforced capability security (removed in 68030)
	M68K_RTM   = 0x06C0 // Hardware-enforced capability security (removed in 68030)
	M68K_MOVEP = 0x0108 // Byte-interleaved I/O for 8-bit peripherals on 16-bit bus
	M68K_MOVEC = 0x4E7A
	M68K_MOVES = 0x0E00 // Allows supervisor to access user address space safely
)

// Control register codes for MOVEC instruction
const (
	M68K_CR_SFC  = 0x000 // Source function code for MOVES instruction
	M68K_CR_DFC  = 0x001 // Destination function code for MOVES instruction
	M68K_CR_USP  = 0x800
	M68K_CR_VBR  = 0x801 // Allows multiple exception vector tables for virtual machines
	M68K_CR_CACR = 0x002
	M68K_CR_CAAR = 0x802 // Selective cache invalidation for DMA coherency
	M68K_CR_MSP  = 0x803
	M68K_CR_ISP  = 0x804
)

// M68K CPU structure
type M68KCPU struct {
	/*
	   Cache Line 0 (64 bytes) - Hot Path Registers:
	   Optimised layout for frequent access patterns:
	   - PC          : Programme counter, accessed every instruction
	   - SR          : Status register - flags and mode control
	   - DataRegs    : D0-D7 data registers
	   - _padding0   : Ensures cache line alignment

	   Cache Line 1 (64 bytes) - Secondary Registers:
	   - AddrRegs    : A0-A7 address registers (A7 is SP)
	   - Running     : CPU execution state
	   - Debug       : Debug mode flag
	   - _padding1   : Cache line alignment

	   Cache Line 2 (64 bytes) - Execution Context:
	   - cycleCounter      : Cycle accounting
	   - prefetchQueue     : Instruction prefetch buffer
	   - currentIR         : Current instruction register
	   - pendingInterrupt  : Interrupt level pending
	   - _padding2         : Cache line alignment

	   Cache Lines 3+ - Synchronisation:
	   - mutex             : Memory access control
	   - interruptMutex    : Interrupt synchronisation
	   - bus               : Memory interface
	*/

	// Cache Line 0 (64 bytes) - Hot Path Registers
	PC        uint32
	SR        uint16
	DataRegs  [8]uint32
	_padding0 [26]byte // Cache line alignment reduces false sharing

	// Cache Line 1 (64 bytes) - Address Registers and Execution State
	AddrRegs  [8]uint32   // A7 switches between SSP and USP based on SR.S bit
	USP       uint32      // Hardware preserves this across supervisor mode transitions
	SSP       uint32      // Supervisor stack pointer (A7 when SR.S is set)
	running   atomic.Bool // Execution state (atomic for lock-free access)
	stopped   atomic.Bool // STOP state; resumes on interrupt
	debug     atomic.Bool // Debug mode flag (atomic for lock-free access)
	_padding1 [18]byte    // Cache line alignment reduces false sharing

	// Control registers
	VBR  uint32 // Relocates exception vector table for multiple contexts
	SFC  uint8  // Source function code for MOVES instruction
	DFC  uint8  // Destination function code for MOVES instruction
	CACR uint32
	CAAR uint32 // Selective cache invalidation for DMA coherency

	// Cache Line 2 (64 bytes) - Execution Context
	cycleCounter     uint32    // Enables accurate timing for cycle-exact emulation
	prefetchQueue    [4]uint16 // Real hardware hides memory latency with prefetch
	prefetchSize     uint8
	currentIR        uint16
	pendingInterrupt atomic.Uint32 // Set by external hardware; checked each instruction (atomic for lock-free access)
	pendingException atomic.Uint32 // Deferred exceptions (atomic for lock-free access)
	inException      atomic.Bool   // Prevents double-fault situations (atomic for lock-free access)
	stackLowerBound  uint32        // Detects stack corruption early
	stackUpperBound  uint32        // Detects stack corruption early
	_padding2        [33]byte      // Cache line alignment reduces false sharing

	// Cache Lines 3+ - Bus Interface (lock-free design like IE32)
	bus Bus32

	// FPU Coprocessor (68881/68882)
	FPU *M68881FPU // Optional FPU - nil if not present

	// Direct VRAM access (bypasses bus for video writes)
	vramDirect     []byte // Direct pointer to video framebuffer
	vramStart      uint32 // Cached VRAM start address
	vramEnd        uint32 // Cached VRAM end address
	VRAMWriteCount uint64 // Counter for direct VRAM writes (for benchmarking)

	// Debug counters
	debugIOReadCount int // Counter for I/O read debug output

	// Direct memory access for lock-free instruction fetches
	memory  []byte         // Direct pointer to main memory (bypasses mutex for non-I/O reads)
	memBase unsafe.Pointer // Unsafe base pointer for bounds-check-free access

	// Fault tracking for bus/address errors (format $A frames)
	accessIsInstruction    bool
	lastFaultAddr          uint32
	lastFaultSize          uint8
	lastFaultWrite         bool
	lastFaultData          uint32
	lastFaultIsInstruction bool

	// Coprocessor mode: skip byte-swap for shared data regions (mailbox, user data)
	// When true, Read16/Read32/Write16/Write32 return/accept LE values directly
	// for addresses in the mailbox (0x820000-0x820FFF) and user data (0x400000-0x7FFFFF)
	CoprocMode bool

	// Performance monitoring (matching IE32 pattern)
	PerfEnabled      bool      // Enable MIPS reporting
	InstructionCount uint64    // Total instructions executed
	perfStartTime    time.Time // When execution started
	lastPerfReport   time.Time // Last time we printed stats
}

// Running returns the execution state (thread-safe)
func (cpu *M68KCPU) Running() bool {
	return cpu.running.Load()
}

// SetRunning sets the execution state (thread-safe)
func (cpu *M68KCPU) SetRunning(state bool) {
	cpu.running.Store(state)
}

type faultingBus interface {
	Read8WithFault(addr uint32) (uint8, bool)
	Read16WithFault(addr uint32) (uint16, bool)
	Read32WithFault(addr uint32) (uint32, bool)
	Write8WithFault(addr uint32, value uint8) bool
	Write16WithFault(addr uint32, value uint16) bool
	Write32WithFault(addr uint32, value uint32) bool
}

func NewM68KCPU(bus Bus32) *M68KCPU {
	mem := bus.GetMemory()
	cpu := &M68KCPU{
		SR:              M68K_SR_S, // Hardware powers up in supervisor mode
		bus:             bus,
		memory:          mem,                     // Direct memory access for lock-free reads
		memBase:         unsafe.Pointer(&mem[0]), // Unsafe base pointer for bounds-check-free access
		stackLowerBound: 0x00002000,              // Reserves space for exception vectors
		stackUpperBound: M68K_MEMORY_SIZE,
		FPU:             NewM68881FPU(), // Initialize 68881 FPU coprocessor
	}
	// Atomic fields default to false - set running to true
	cpu.running.Store(true)

	// Hardware reads vectors 0 and 1 during reset
	sp := cpu.Read32(0)
	if sp == 0 {
		cpu.AddrRegs[7] = M68K_STACK_START
	} else {
		cpu.AddrRegs[7] = sp
		// Dynamically adjust bounds to match programme's stack placement
		if sp > cpu.stackLowerBound && sp <= cpu.stackUpperBound {
			if sp+0x10000 < cpu.stackUpperBound {
				cpu.stackUpperBound = sp + 0x10000
			}
			if sp > 0x10000 {
				cpu.stackLowerBound = sp - 0x10000
			}
		}
	}
	cpu.SSP = cpu.AddrRegs[7]

	pc := cpu.Read32(M68K_RESET_VECTOR)
	if pc == 0 {
		cpu.PC = M68K_ENTRY_POINT
	} else {
		cpu.PC = pc
	}

	return cpu
}
func (cpu *M68KCPU) Reset() {
	// Lock-free reset like IE32
	cpu.running.Store(false)

	time.Sleep(M68K_RESET_DELAY)

	// Flush prefetch queue to prevent executing stale instructions
	for i := range cpu.prefetchQueue {
		cpu.prefetchQueue[i] = 0
	}
	cpu.prefetchSize = 0

	// Hardware enters supervisor mode with interrupts masked
	cpu.SR = M68K_SR_S

	sp := cpu.Read32(0)
	if sp == 0 {
		cpu.AddrRegs[7] = M68K_STACK_START
	} else {
		cpu.AddrRegs[7] = sp
	}
	cpu.SSP = cpu.AddrRegs[7]

	pc := cpu.Read32(M68K_RESET_VECTOR)
	if pc == 0 {
		cpu.PC = M68K_ENTRY_POINT
	} else {
		cpu.PC = pc
	}

	cpu.running.Store(true)
}
func (cpu *M68KCPU) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	fmt.Printf("LoadProgram: Read %d bytes\n", len(program))

	cpu.LoadProgramBytes(program)

	fmt.Printf("LoadProgram: Program loaded at memory address 0x%08X\n", M68K_ENTRY_POINT)
	return nil
}

// LoadProgramBytes loads a program from a byte slice into memory at M68K_ENTRY_POINT
func (cpu *M68KCPU) LoadProgramBytes(program []byte) {
	entryPoint := uint32(M68K_ENTRY_POINT)
	for i := 0; i < len(program) && i+(M68K_WORD_SIZE-1) < len(program); i += M68K_WORD_SIZE {
		addr := entryPoint + uint32(i)

		// Programme files are big-endian per 68K conventions
		beValue := binary.BigEndian.Uint16(program[i : i+M68K_WORD_SIZE])

		// Write16 handles endian conversion to host format
		cpu.Write16(addr, beValue)
	}

	cpu.PC = M68K_ENTRY_POINT
	cpu.AddrRegs[7] = M68K_STACK_START
	cpu.SR = M68K_SR_S
	cpu.running.Store(true)
}

// Main execution loop and control
// Dispatch table for top 4 bits of opcode (bits 15-12)
// This replaces 82+ if-else checks with a single array lookup
var m68kDispatchTable = [16]func(cpu *M68KCPU, opcode uint16){
	(*M68KCPU).decodeGroup0, // 0x0xxx: Bit manipulation and immediate
	(*M68KCPU).decodeGroup1, // 0x1xxx: MOVE.B
	(*M68KCPU).decodeGroup2, // 0x2xxx: MOVE.L
	(*M68KCPU).decodeGroup3, // 0x3xxx: MOVE.W
	(*M68KCPU).decodeGroup4, // 0x4xxx: Miscellaneous
	(*M68KCPU).decodeGroup5, // 0x5xxx: ADDQ, SUBQ, Scc, DBcc
	(*M68KCPU).decodeGroup6, // 0x6xxx: Bcc (branches)
	(*M68KCPU).decodeGroup7, // 0x7xxx: MOVEQ
	(*M68KCPU).decodeGroup8, // 0x8xxx: OR, DIV, SBCD
	(*M68KCPU).decodeGroup9, // 0x9xxx: SUB, SUBA, SUBX
	(*M68KCPU).decodeGroupA, // 0xAxxx: Line A trap
	(*M68KCPU).decodeGroupB, // 0xBxxx: CMP, CMPA, EOR
	(*M68KCPU).decodeGroupC, // 0xCxxx: AND, MUL, ABCD, EXG
	(*M68KCPU).decodeGroupD, // 0xDxxx: ADD, ADDA, ADDX
	(*M68KCPU).decodeGroupE, // 0xExxx: Shift, Rotate, Bit Field
	(*M68KCPU).decodeGroupF, // 0xFxxx: Line F (coprocessor)
}

func (cpu *M68KCPU) FetchAndDecodeInstruction() {
	opcode := cpu.currentIR
	// Single array lookup replaces 80+ conditional checks
	m68kDispatchTable[opcode>>12](cpu, opcode)
}

// decodeGroup0: 0x0xxx - Bit manipulation and immediate instructions
func (cpu *M68KCPU) decodeGroup0(opcode uint16) {
	// Exact matches for SR/CCR operations
	switch opcode {
	case M68K_ANDI_TO_CCR:
		cpu.ExecAndiCcr()
		return
	case M68K_EORI_TO_CCR:
		cpu.ExecEoriCcr()
		return
	case M68K_ORI_TO_CCR:
		cpu.ExecOriCcr()
		return
	case M68K_ANDI_TO_SR:
		cpu.ExecAndiSr()
		return
	case M68K_EORI_TO_SR:
		cpu.ExecEoriSr()
		return
	case M68K_ORI_TO_SR:
		cpu.ExecOriSr()
		return
	}

	// MOVEP - Byte-interleaved I/O for 8-bit peripherals on 16-bit data bus
	if (opcode & 0xF138) == M68K_MOVEP {
		cpu.ExecMovep()
		return
	}

	// CALLM - Protected module invocation
	if (opcode&0xFFC0) == M68K_CALLM && (opcode&0x00C0) == 0x00C0 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecCallm(mode, reg)
		return
	}

	// RTM - Returns from protected module
	if (opcode&0xFFF0) == M68K_RTM && (opcode&0x00C0) == 0x0080 {
		reg := opcode & 0xF
		cpu.ExecRtm(reg)
		return
	}

	// Bit manipulation with dynamic bit number
	if (opcode & 0xF1C0) == M68K_BTST {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		cpu.ExecBtst(reg, mode, xreg)
		return
	}
	if (opcode & 0xF1C0) == M68K_BCHG {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		cpu.ExecBchg(reg, mode, xreg)
		return
	}
	if (opcode & 0xF1C0) == M68K_BCLR {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		cpu.ExecBclr(reg, mode, xreg)
		return
	}
	if (opcode & 0xF1C0) == M68K_BSET {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		cpu.ExecBset(reg, mode, xreg)
		return
	}

	// Immediate instructions
	if (opcode & 0xFF00) == M68K_ADDI {
		cpu.ExecAddi()
		return
	}
	if (opcode & 0xFF00) == M68K_SUBI {
		cpu.ExecSubi()
		return
	}
	if (opcode & 0xFF00) == M68K_CMPI {
		cpu.ExecCmpi()
		return
	}
	if (opcode & 0xFF00) == M68K_ANDI {
		cpu.ExecAndi()
		return
	}
	if (opcode & 0xFF00) == M68K_ORI {
		cpu.ExecOri()
		return
	}
	if (opcode & 0xFF00) == M68K_EORI {
		cpu.ExecEori()
		return
	}

	// Immediate bit manipulation
	if (opcode & 0xFFC0) == 0x0800 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBitManipImm(BTST, mode, reg)
		return
	}
	if (opcode & 0xFFC0) == 0x0840 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBitManipImm(BCHG, mode, reg)
		return
	}
	if (opcode & 0xFFC0) == 0x0880 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBitManipImm(BCLR, mode, reg)
		return
	}
	if (opcode & 0xFFC0) == 0x08C0 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBitManipImm(BSET, mode, reg)
		return
	}

	// CAS - Lock-free synchronisation
	if (opcode & 0xFFC0) == M68K_CAS {
		opmode := (opcode >> 9) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecCas(opmode, mode, reg)
		return
	}

	// CAS2
	if opcode == M68K_CAS2 {
		opmode := (opcode >> 9) & 0x3
		cpu.ExecCas2(opmode)
		return
	}

	// CHK2/CMP2
	if (opcode & 0xFFC0) == M68K_CHK2_CMP2 {
		size := (opcode >> 9) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecChk2Cmp2(size, mode, reg)
		return
	}

	// MOVES
	if (opcode & 0xFF00) == M68K_MOVES {
		cpu.ExecMoves()
		return
	}

	// Illegal instruction
	fmt.Printf("M68K: Unimplemented opcode %04x at PC=%08x\n", opcode, cpu.PC-M68K_WORD_SIZE)
	cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
}

// decodeGroup1: 0x1xxx - MOVE.B
func (cpu *M68KCPU) decodeGroup1(opcode uint16) {
	destReg := (opcode >> 9) & 0x7
	destMode := (opcode >> 6) & 0x7
	srcMode := (opcode >> 3) & 0x7
	srcReg := opcode & 0x7
	cpu.ExecMove(srcMode, srcReg, destMode, destReg, 1) // size=1 for byte
}

// decodeGroup2: 0x2xxx - MOVE.L and MOVEA.L
func (cpu *M68KCPU) decodeGroup2(opcode uint16) {
	destReg := (opcode >> 9) & 0x7
	destMode := (opcode >> 6) & 0x7
	srcMode := (opcode >> 3) & 0x7
	srcReg := opcode & 0x7

	// Fast path: MOVE.L Dn,Dm (register-to-register, most common case)
	if srcMode == 0 && destMode == 0 {
		value := cpu.DataRegs[srcReg]
		cpu.DataRegs[destReg] = value
		// Set N and Z flags, clear V and C
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (value & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	// Fast path: MOVE.L An,Dm
	if srcMode == M68K_AM_AR && destMode == 0 {
		value := cpu.AddrRegs[srcReg]
		cpu.DataRegs[destReg] = value
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (value & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	// Fast path: MOVE.L #imm,Dn
	if srcMode == 7 && srcReg == 4 && destMode == 0 {
		value := cpu.Fetch32()
		cpu.DataRegs[destReg] = value
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (value & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_EA_IM + M68K_CYCLE_REG
		return
	}

	// Fast path: MOVE.L (An),Dm
	if srcMode == M68K_AM_AR_IND && destMode == 0 {
		value := cpu.Read32(cpu.AddrRegs[srcReg])
		cpu.DataRegs[destReg] = value
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (value & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_REG
		return
	}

	// Fast path: MOVE.L Dn,(Am)
	if srcMode == 0 && destMode == M68K_AM_AR_IND {
		value := cpu.DataRegs[srcReg]
		cpu.Write32(cpu.AddrRegs[destReg], value)
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (value & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG + M68K_CYCLE_MEM_WRITE
		return
	}

	// Fast path: MOVE.L with brief-format indexed source
	if srcMode == M68K_AM_AR_INDEX {
		extWord := cpu.Fetch16()

		if (extWord & M68K_EXT_FULL_FORMAT) == 0 { // Brief format
			// Decode index register
			idxRegNum := (extWord >> 12) & 0x7
			var idxValue uint32
			if (extWord & 0x8000) != 0 { // Address register
				idxValue = cpu.AddrRegs[idxRegNum]
			} else { // Data register
				idxValue = cpu.DataRegs[idxRegNum]
			}
			// Word index: sign-extend to long
			if (extWord & 0x0800) == 0 {
				idxValue = uint32(int32(int16(idxValue & 0xFFFF)))
			}
			// Apply scale
			scale := (extWord >> M68K_EXT_SCALE_START_BIT) & 0x3
			idxValue <<= scale

			disp8 := int8(extWord & 0xFF)
			addr := cpu.AddrRegs[srcReg] + idxValue + uint32(int32(disp8))
			value := cpu.Read32(addr)

			// Inline destination handling for common modes
			switch destMode {
			case 0: // MOVE.L (An,Xn),Dm
				cpu.DataRegs[destReg] = value
				cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
				if value == 0 {
					cpu.SR |= M68K_SR_Z
				} else if (value & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX + M68K_CYCLE_REG
				return

			case M68K_AM_AR: // MOVEA.L (An,Xn),Am - no flags
				cpu.AddrRegs[destReg] = value
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
				return

			case M68K_AM_AR_IND: // MOVE.L (An,Xn),(Am)
				cpu.Write32(cpu.AddrRegs[destReg], value)
				cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
				if value == 0 {
					cpu.SR |= M68K_SR_Z
				} else if (value & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX + M68K_CYCLE_MEM_WRITE
				return

			case M68K_AM_AR_POST: // MOVE.L (An,Xn),(Am)+
				cpu.Write32(cpu.AddrRegs[destReg], value)
				cpu.AddrRegs[destReg] += M68K_LONG_SIZE
				cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
				if value == 0 {
					cpu.SR |= M68K_SR_Z
				} else if (value & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX + M68K_CYCLE_MEM_WRITE
				return
			}
		}
		// Full format or unhandled dest: rewind extension word fetch, use slow path
		cpu.PC -= M68K_WORD_SIZE
	}

	cpu.ExecMove(srcMode, srcReg, destMode, destReg, 2) // size=2 for long
}

// decodeGroup3: 0x3xxx - MOVE.W and MOVEA.W
func (cpu *M68KCPU) decodeGroup3(opcode uint16) {
	destReg := (opcode >> 9) & 0x7
	destMode := (opcode >> 6) & 0x7
	srcMode := (opcode >> 3) & 0x7
	srcReg := opcode & 0x7
	cpu.ExecMove(srcMode, srcReg, destMode, destReg, 3) // size=3 for word
}

// decodeGroup4: 0x4xxx - Miscellaneous instructions
func (cpu *M68KCPU) decodeGroup4(opcode uint16) {
	// Exact matches first
	switch opcode {
	case M68K_NOP:
		cpu.ExecNOP()
		return
	case M68K_RTS:
		cpu.ExecRTS()
		return
	case M68K_RTE:
		cpu.ExecRTE()
		return
	case M68K_STOP:
		cpu.ExecSTOP()
		return
	case M68K_RESET:
		cpu.ExecRESET()
		return
	case M68K_TRAPV:
		cpu.ExecTRAPV()
		return
	case M68K_RTR:
		cpu.ExecRTR()
		return
	}

	// TRAP
	if (opcode & 0xFFF0) == M68K_TRAP {
		cpu.ExecTRAP(opcode & 0xF)
		return
	}

	// JSR
	if (opcode & 0xFFC0) == M68K_JSR {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecJsr(mode, reg)
		return
	}

	// JMP
	if (opcode & 0xFFC0) == M68K_JMP {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecJmp(mode, reg)
		return
	}

	// MOVEM
	if (opcode & 0xFB80) == M68K_MOVEM {
		mode := (opcode >> 3) & 0x7
		if mode != M68K_AM_DR {
			direction := (opcode >> 10) & 0x1
			size := (opcode >> 6) & 0x1
			reg := opcode & 0x7
			cpu.ExecMovem(direction, size, mode, reg)
			return
		}
	}

	// MOVE from SR
	if (opcode & 0xFFC0) == M68K_MOVE_FROM_SR {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecMoveFromSR(mode, reg)
		return
	}

	// MOVE to SR
	if (opcode & 0xFFC0) == M68K_MOVE_TO_SR {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecMoveToSR(mode, reg)
		return
	}

	// MOVE USP
	if (opcode & 0xFFF0) == M68K_MOVE_USP {
		direction := (opcode >> 3) & 0x1
		if direction == 0 {
			cpu.ExecMoveToUSP()
		} else {
			cpu.ExecMoveFromUSP()
		}
		return
	}

	// MOVEC
	if (opcode & 0xFFFE) == M68K_MOVEC {
		cpu.ExecMovec()
		return
	}

	// LEA
	if (opcode & 0xF1C0) == M68K_LEA {
		areg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7

		// Fast path: LEA d16(An),Am
		if mode == M68K_AM_AR_DISP {
			disp := int16(cpu.Fetch16())
			cpu.AddrRegs[areg] = cpu.AddrRegs[xreg] + uint32(disp)
			cpu.cycleCounter += M68K_CYCLE_REG
			return
		}

		cpu.ExecLea(areg, mode, xreg)
		return
	}

	// PEA
	if (opcode & 0xFFC0) == M68K_PEA {
		mode := (opcode >> 3) & 0x7
		if mode != M68K_AM_DR {
			reg := opcode & 0x7
			cpu.ExecPea(mode, reg)
			return
		}
	}

	// CHK
	if (opcode & 0xF1C0) == M68K_CHK {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		var size int
		if (opcode & 0x0080) != 0 {
			size = M68K_SIZE_LONG
		} else {
			size = M68K_SIZE_WORD
		}
		cpu.ExecChk(reg, mode, xreg, size)
		return
	}

	// CLR
	if (opcode & 0xFF00) == M68K_CLR {
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecClr(size, mode, reg)
		return
	}

	// NOT
	if (opcode & 0xFF00) == M68K_NOT {
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecNot(mode, reg, sizeCode)
		return
	}

	// NEG
	if (opcode & 0xFF00) == M68K_NEG {
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecNeg(mode, reg, sizeCode)
		return
	}

	// NEGX
	if (opcode & 0xFF00) == M68K_NEGX {
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecNegx(mode, reg, sizeCode)
		return
	}

	// SWAP
	if (opcode & 0xFFF8) == M68K_SWAP {
		reg := opcode & 0x7
		cpu.ExecSwap(reg)
		return
	}

	// EXT
	masked := opcode & 0xFFC0
	if masked == 0x4880 || masked == 0x48C0 || masked == 0x49C0 {
		reg := opcode & 0x7
		opmode := (opcode >> 6) & 0x7
		cpu.ExecExt(reg, opmode)
		return
	}

	// TST
	if (opcode & 0xFF00) == M68K_TST {
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecTst(size, mode, reg)
		return
	}

	// LINK
	if (opcode & 0xFFF8) == M68K_LINK {
		reg := opcode & 0x7
		cpu.ExecLink(reg)
		return
	}

	// UNLK
	if (opcode & 0xFFF8) == M68K_UNLK {
		reg := opcode & 0x7
		cpu.ExecUnlk(reg)
		return
	}

	// TAS
	if (opcode & 0xFFC0) == M68K_TAS {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecTas(mode, reg)
		return
	}

	// BKPT
	if (opcode & 0xFFF8) == M68K_BKPT {
		bkptNum := opcode & 0x7
		cpu.ExecBkpt(bkptNum)
		return
	}

	// NBCD
	if (opcode & 0xFFC0) == M68K_NBCD {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecNbcd(mode, reg)
		return
	}

	// MULL/DIVL (68020) - 0x4C00-0x4C7F range
	// Bit 6 distinguishes: 0 = MULL (0x4C00), 1 = DIVL (0x4C40)
	if (opcode & 0xFF80) == 0x4C00 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		if (opcode & 0x0040) == 0 {
			cpu.ExecMulL(0, mode, reg)
		} else {
			cpu.ExecDIVL(0, mode, reg)
		}
		return
	}

	// Illegal instruction
	fmt.Printf("M68K: Unimplemented opcode %04x at PC=%08x\n", opcode, cpu.PC-M68K_WORD_SIZE)
	cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
}

// decodeGroup5: 0x5xxx - ADDQ, SUBQ, Scc, DBcc
func (cpu *M68KCPU) decodeGroup5(opcode uint16) {
	// DBcc (inlined)
	if (opcode & 0xF0F8) == M68K_DBcc {
		reg := opcode & 0x7
		condition := (opcode >> 8) & 0xF
		displacement := int16(cpu.Fetch16()) // MUST fetch before condition check

		var condMet bool
		switch condition {
		case 0: // DBT
			condMet = true
		case 1: // DBF (DBRA)
			condMet = false
		case 6: // DBNE
			condMet = (cpu.SR & M68K_SR_Z) == 0
		case 7: // DBEQ
			condMet = (cpu.SR & M68K_SR_Z) != 0
		default:
			condMet = cpu.CheckCondition(uint8(condition))
		}

		if !condMet {
			counter := int16(cpu.DataRegs[reg] & 0xFFFF)
			counter--
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | uint32(uint16(counter))
			if counter != -1 {
				cpu.PC = cpu.PC - M68K_WORD_SIZE + uint32(displacement)
			}
		}
		cpu.cycleCounter += M68K_CYCLE_BRANCH
		return
	}

	// Scc
	if (opcode & 0xF0C0) == M68K_Scc {
		condition := uint8((opcode >> 8) & 0xF)
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecScc(condition, mode, reg)
		return
	}

	// ADDQ/SUBQ (exclude size = 3 which is Scc/DBcc)
	if (opcode & 0x00C0) != 0x00C0 {
		data := (opcode >> 9) & 0x7
		if data == 0 {
			data = 8
		}
		size := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		isSub := (opcode & 0x0100) != 0

		// Fast path: ADDQ/SUBQ An (no flags affected)
		if mode == M68K_AM_AR {
			if isSub {
				cpu.AddrRegs[reg] -= uint32(data)
			} else {
				cpu.AddrRegs[reg] += uint32(data)
			}
			cpu.cycleCounter += M68K_CYCLE_REG
			return
		}

		// Fast path: ADDQ.L/SUBQ.L #n,Dn
		if mode == M68K_AM_DR && size == 2 {
			d := uint32(data)
			dest := cpu.DataRegs[reg]
			if isSub {
				result := dest - d
				cpu.DataRegs[reg] = result
				cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
				if result == 0 {
					cpu.SR |= M68K_SR_Z
				}
				if (result & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
				if dest < d {
					cpu.SR |= M68K_SR_C | M68K_SR_X
				}
				// Overflow: dest negative and result positive (data is always 1-8, positive)
				if ((dest & 0x80000000) != 0) && ((result & 0x80000000) == 0) {
					cpu.SR |= M68K_SR_V
				}
			} else {
				result := dest + d
				cpu.DataRegs[reg] = result
				cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
				if result == 0 {
					cpu.SR |= M68K_SR_Z
				}
				if (result & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
				if result < dest { // unsigned overflow
					cpu.SR |= M68K_SR_C | M68K_SR_X
				}
				// Overflow: positive + positive = negative (data always positive)
				signDest := (dest & 0x80000000) != 0
				signResult := (result & 0x80000000) != 0
				if !signDest && signResult {
					cpu.SR |= M68K_SR_V
				}
			}
			cpu.cycleCounter += M68K_CYCLE_REG
			return
		}

		// Slow path
		if isSub {
			cpu.ExecSubq(uint32(data), size, mode, reg)
		} else {
			cpu.ExecAddq(uint32(data), size, mode, reg)
		}
		return
	}

	// Illegal instruction
	fmt.Printf("M68K: Unimplemented opcode %04x at PC=%08x\n", opcode, cpu.PC-M68K_WORD_SIZE)
	cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
}

// decodeGroup6: 0x6xxx - Bcc (branches including BRA, BSR)
func (cpu *M68KCPU) decodeGroup6(opcode uint16) {
	condition := (opcode >> 8) & 0xF
	displacement := int8(opcode & 0xFF)

	// Fast path: Bcc.B (byte displacement, not BRA/BSR, not word/long)
	if displacement != 0 && displacement != -1 && condition >= 2 {
		var take bool
		switch condition {
		case 6: // BNE
			take = (cpu.SR & M68K_SR_Z) == 0
		case 7: // BEQ
			take = (cpu.SR & M68K_SR_Z) != 0
		case 4: // BCC
			take = (cpu.SR & M68K_SR_C) == 0
		case 5: // BCS
			take = (cpu.SR & M68K_SR_C) != 0
		case 10: // BPL
			take = (cpu.SR & M68K_SR_N) == 0
		case 11: // BMI
			take = (cpu.SR & M68K_SR_N) != 0
		default:
			take = cpu.CheckCondition(uint8(condition))
		}
		if take {
			targetPC := cpu.PC + uint32(int32(displacement))
			if targetPC >= M68K_MEMORY_SIZE-M68K_WORD_SIZE {
				cpu.running.Store(false)
				return
			}
			cpu.PC = targetPC
			cpu.prefetchSize = 0
		}
		return
	}

	// Fast path: Bcc.W (word displacement, not BRA/BSR)
	if displacement == 0 && condition >= 2 {
		effectiveDisp := int32(int16(cpu.Fetch16()))
		var take bool
		switch condition {
		case 6: // BNE
			take = (cpu.SR & M68K_SR_Z) == 0
		case 7: // BEQ
			take = (cpu.SR & M68K_SR_Z) != 0
		case 4: // BCC
			take = (cpu.SR & M68K_SR_C) == 0
		case 5: // BCS
			take = (cpu.SR & M68K_SR_C) != 0
		case 10: // BPL
			take = (cpu.SR & M68K_SR_N) == 0
		case 11: // BMI
			take = (cpu.SR & M68K_SR_N) != 0
		default:
			take = cpu.CheckCondition(uint8(condition))
		}
		if take {
			targetPC := cpu.PC - M68K_WORD_SIZE + uint32(effectiveDisp)
			if targetPC >= M68K_MEMORY_SIZE-M68K_WORD_SIZE {
				cpu.running.Store(false)
				return
			}
			cpu.PC = targetPC
			cpu.prefetchSize = 0
		}
		return
	}

	cpu.ExecBRA(opcode) // Slow path: BRA, BSR, long displacement
}

// decodeGroup7: 0x7xxx - MOVEQ (fully inlined - always LONG, no size switch needed)
func (cpu *M68KCPU) decodeGroup7(opcode uint16) {
	reg := (opcode >> 9) & 0x7
	value := uint32(int32(int8(opcode & 0xFF)))
	cpu.DataRegs[reg] = value
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	if value == 0 {
		cpu.SR |= M68K_SR_Z
	} else if (value & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_N
	}
	cpu.cycleCounter += M68K_CYCLE_REG
}

// decodeGroup8: 0x8xxx - OR, DIV, SBCD
func (cpu *M68KCPU) decodeGroup8(opcode uint16) {
	// DIVU/DIVS.W
	if (opcode & 0xF0C0) == 0x80C0 {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		if (opcode & 0x0100) == 0 {
			cpu.ExecDivu(reg, mode, xreg)
		} else {
			cpu.ExecDivs(reg, mode, xreg)
		}
		return
	}

	// SBCD
	if (opcode & 0xF1F0) == M68K_SBCD {
		regMode := (opcode >> 3) & 0x1
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		cpu.ExecSbcd(regMode, rx, ry)
		return
	}

	// PACK
	if (opcode & 0xF1F0) == M68K_PACK {
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		regMode := (opcode >> 3) & 0x1
		cpu.ExecPack(rx, ry, regMode == 0)
		return
	}

	// UNPK
	if (opcode & 0xF1F0) == M68K_UNPK {
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		regMode := (opcode >> 3) & 0x1
		cpu.ExecUnpk(rx, ry, regMode == 0)
		return
	}

	// OR
	reg := (opcode >> 9) & 0x7
	opmode := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	xreg := opcode & 0x7

	// Fast path: OR.L Dn,Dm (opmode=2: <ea> to Dn, register-to-register long)
	if mode == 0 && opmode == 2 {
		result := cpu.DataRegs[reg] | cpu.DataRegs[xreg]
		cpu.DataRegs[reg] = result
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z)
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	cpu.ExecOr(reg, opmode, mode, xreg)
}

// decodeGroup9: 0x9xxx - SUB, SUBA, SUBX
func (cpu *M68KCPU) decodeGroup9(opcode uint16) {
	// SUBX (check before SUB)
	if (opcode&0xF130) == M68K_SUBX && (opcode&0x00C0) != 0x00C0 {
		regMode := (opcode >> 3) & 0x1
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		size := (opcode >> 6) & 0x3
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecSubx(regMode, rx, ry, sizeCode)
		return
	}

	reg := (opcode >> 9) & 0x7
	opmode := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	xreg := opcode & 0x7

	// SUBA
	if opmode == 3 || opmode == 7 {
		cpu.ExecSuba(reg, opmode, mode, xreg)
		return
	}

	// Fast path: SUB.L Dn,Dm (register-to-register long)
	if mode == 0 && (opmode == 2 || opmode == 6) {
		var source, dest, result uint32
		if opmode == 2 {
			// SUB.L <ea>,Dn  (source from xreg, dest is reg)
			source = cpu.DataRegs[xreg]
			dest = cpu.DataRegs[reg]
			result = dest - source
			cpu.DataRegs[reg] = result
		} else {
			// SUB.L Dn,<ea>  (source from reg, dest is xreg)
			source = cpu.DataRegs[reg]
			dest = cpu.DataRegs[xreg]
			result = dest - source
			cpu.DataRegs[xreg] = result
		}
		// Set all flags for SUB
		cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// Borrow: source > dest means borrow occurred
		if source > dest {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		// Overflow: sign change when it shouldn't
		srcSign := source & 0x80000000
		dstSign := dest & 0x80000000
		resSign := result & 0x80000000
		if (srcSign != dstSign) && (resSign != dstSign) {
			cpu.SR |= M68K_SR_V
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	cpu.ExecSub(reg, opmode, mode, xreg)
}

// decodeGroupA: 0xAxxx - Line A trap
func (cpu *M68KCPU) decodeGroupA(opcode uint16) {
	cpu.ProcessException(M68K_VEC_LINE_A)
}

// decodeGroupB: 0xBxxx - CMP, CMPA, EOR, CMPM
func (cpu *M68KCPU) decodeGroupB(opcode uint16) {
	// CMPM
	if (opcode & 0xF1F8) == M68K_CMPM {
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		size := (opcode >> 6) & 0x3
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecCmpm(rx, ry, sizeCode)
		return
	}

	// EOR (bit 8 set)
	if (opcode & 0xF138) == M68K_EOR {
		reg := (opcode >> 9) & 0x7
		opmode := (opcode >> 6) & 0x3
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		cpu.ExecEor(reg, opmode, mode, xreg)
		return
	}

	reg := (opcode >> 9) & 0x7
	opmode := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	xreg := opcode & 0x7

	// CMPA
	if opmode == 3 || opmode == 7 {
		cpu.ExecCmpa(reg, opmode, mode, xreg)
		return
	}

	// Fast path: CMP.L Dn,Dm (register-to-register long)
	if mode == 0 && (opmode&3) == 2 {
		source := cpu.DataRegs[xreg]
		dest := cpu.DataRegs[reg]
		result := dest - source
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if dest < source {
			cpu.SR |= M68K_SR_C
		}
		if ((dest & 0x80000000) != (source & 0x80000000)) && ((result & 0x80000000) == (source & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG + M68K_CYCLE_EXECUTE
		return
	}

	cpu.ExecCmp(reg, opmode, mode, xreg)
}

// decodeGroupC: 0xCxxx - AND, MUL, ABCD, EXG
func (cpu *M68KCPU) decodeGroupC(opcode uint16) {
	// MULU/MULS.W
	if (opcode & 0xF0C0) == 0xC0C0 {
		reg := (opcode >> 9) & 0x7
		mode := (opcode >> 3) & 0x7
		xreg := opcode & 0x7
		if (opcode & 0x0100) == 0 {
			cpu.ExecMulu(reg, mode, xreg)
		} else {
			cpu.ExecMuls(reg, mode, xreg)
		}
		return
	}

	// ABCD
	if (opcode & 0xF1F0) == M68K_ABCD {
		regMode := (opcode >> 3) & 0x1
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		cpu.ExecAbcd(regMode, rx, ry)
		return
	}

	// EXG - Exchange registers
	// Format: 1100 xxx1 opmode yyy where opmode distinguishes Dx/Dx, Ax/Ax, Dx/Ax
	if (opcode & 0xF130) == 0xC100 {
		rx := (opcode >> 9) & 0x7
		ry := opcode & 0x7
		opmode := (opcode >> 3) & 0x1F

		switch opmode {
		case 0x08: // EXG Dx,Dy
			cpu.DataRegs[rx], cpu.DataRegs[ry] = cpu.DataRegs[ry], cpu.DataRegs[rx]
		case 0x09: // EXG Ax,Ay
			cpu.AddrRegs[rx], cpu.AddrRegs[ry] = cpu.AddrRegs[ry], cpu.AddrRegs[rx]
		case 0x11: // EXG Dx,Ay
			cpu.DataRegs[rx], cpu.AddrRegs[ry] = cpu.AddrRegs[ry], cpu.DataRegs[rx]
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		}
		return
	}

	// AND
	reg := (opcode >> 9) & 0x7
	opmode := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	xreg := opcode & 0x7

	// Fast path: AND.L Dn,Dm (register-to-register long)
	if mode == 0 && (opmode == 2 || opmode == 6) {
		var result uint32
		if opmode == 2 {
			// AND.L <ea>,Dn - source from xreg, dest is reg
			result = cpu.DataRegs[reg] & cpu.DataRegs[xreg]
			cpu.DataRegs[reg] = result
		} else {
			// AND.L Dn,<ea> - source from reg, dest is xreg
			result = cpu.DataRegs[xreg] & cpu.DataRegs[reg]
			cpu.DataRegs[xreg] = result
		}
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z)
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		} else if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	cpu.ExecAnd(reg, opmode, mode, xreg)
}

// decodeGroupD: 0xDxxx - ADD, ADDA, ADDX
func (cpu *M68KCPU) decodeGroupD(opcode uint16) {
	// ADDX (check before ADD)
	if (opcode&0xF130) == M68K_ADDX && (opcode&0x00C0) != 0x00C0 {
		regMode := (opcode >> 3) & 0x1
		rx := opcode & 0x7
		ry := (opcode >> 9) & 0x7
		size := (opcode >> 6) & 0x3
		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
		cpu.ExecAddx(regMode, rx, ry, sizeCode)
		return
	}

	reg := (opcode >> 9) & 0x7
	opmode := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	xreg := opcode & 0x7

	// ADDA
	if opmode == 3 || opmode == 7 {
		cpu.ExecAdda(reg, opmode, mode, xreg)
		return
	}

	// Fast path: ADD.L Dn,Dm (register-to-register long)
	if mode == 0 && (opmode == 2 || opmode == 6) {
		var source, dest, result uint32
		if opmode == 2 {
			// ADD.L <ea>,Dn  (source from xreg, dest is reg)
			source = cpu.DataRegs[xreg]
			dest = cpu.DataRegs[reg]
			result = dest + source
			cpu.DataRegs[reg] = result
		} else {
			// ADD.L Dn,<ea>  (source from reg, dest is xreg)
			source = cpu.DataRegs[reg]
			dest = cpu.DataRegs[xreg]
			result = dest + source
			cpu.DataRegs[xreg] = result
		}
		// Set all flags for ADD
		cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// Carry: result < source means overflow in unsigned addition
		if result < source {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		// Overflow: sign of result differs from both operands' expected sign
		srcSign := source & 0x80000000
		dstSign := dest & 0x80000000
		resSign := result & 0x80000000
		if (srcSign == dstSign) && (resSign != srcSign) {
			cpu.SR |= M68K_SR_V
		}
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	cpu.ExecAdd(reg, opmode, mode, xreg)
}

// decodeGroupE: 0xExxx - Shift, Rotate, Bit Field
func (cpu *M68KCPU) decodeGroupE(opcode uint16) {
	// Bit field operations
	if (opcode & 0xFFC0) == M68K_BFTST {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFTST(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFEXTU {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFEXTU(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFCHG {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFCHG(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFEXTS {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFEXTS(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFCLR {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFCLR(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFFFO {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFFO(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFSET {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFSET(mode, reg)
		return
	}
	if (opcode & 0xFFC0) == M68K_BFINS {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		cpu.ExecBFINS(mode, reg)
		return
	}

	// Memory shift/rotate
	if (opcode & 0xFEC0) == M68K_SHIFT_MEM {
		cpu.ExecShiftRotateMemory()
		return
	}

	// Register shift/rotate (exclude bit field and memory shift)
	if (opcode&0x08C0) != 0x08C0 && (opcode&0x00C0) != 0x00C0 {
		count := (opcode >> 9) & 0x7
		op := (opcode >> 3) & 0x3
		reg := opcode & 0x7
		direction := (opcode >> 8) & 0x1
		regOrImm := (opcode >> 5) & 0x1
		size := (opcode >> 6) & 0x3

		// Fast path: LSL.L/LSR.L #imm,Dn (immediate long shifts are very common)
		if regOrImm == 0 && size == 2 && op == 1 {
			shiftCount := uint32(count)
			if shiftCount == 0 {
				shiftCount = 8
			}
			value := cpu.DataRegs[reg]
			var result uint32
			var cFlag bool

			cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

			if direction == 1 { // LSL
				cFlag = (value & (1 << (32 - shiftCount))) != 0
				result = value << shiftCount
			} else { // LSR
				cFlag = (value & (1 << (shiftCount - 1))) != 0
				result = value >> shiftCount
			}

			cpu.DataRegs[reg] = result
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if cFlag {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			cpu.cycleCounter += M68K_CYCLE_REG
			return
		}

		var sizeCode int
		switch size {
		case 0:
			sizeCode = M68K_SIZE_BYTE
		case 1:
			sizeCode = M68K_SIZE_WORD
		case 2:
			sizeCode = M68K_SIZE_LONG
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		if direction == 0 {
			switch op {
			case 0:
				cpu.ExecASR(reg, count, regOrImm, sizeCode)
			case 1:
				cpu.ExecLSR(reg, count, regOrImm, sizeCode)
			case 2:
				cpu.ExecROXR(reg, count, regOrImm, sizeCode)
			case 3:
				cpu.ExecROR(reg, count, regOrImm, sizeCode)
			}
		} else {
			switch op {
			case 0:
				cpu.ExecASL(reg, count, regOrImm, sizeCode)
			case 1:
				cpu.ExecLSL(reg, count, regOrImm, sizeCode)
			case 2:
				cpu.ExecROXL(reg, count, regOrImm, sizeCode)
			case 3:
				cpu.ExecROL(reg, count, regOrImm, sizeCode)
			}
		}
		return
	}

	// Illegal instruction
	fmt.Printf("M68K: Unimplemented opcode %04x at PC=%08x\n", opcode, cpu.PC-M68K_WORD_SIZE)
	cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
}

// decodeGroupF: 0xFxxx - Line F (coprocessor/FPU)
func (cpu *M68KCPU) decodeGroupF(opcode uint16) {
	cpID := (opcode >> 9) & 0x7
	if cpID == 1 && cpu.FPU != nil {
		cpu.ExecFPUInstruction(opcode)
		return
	}
	cpu.ProcessException(M68K_VEC_LINE_F)
}
func (cpu *M68KCPU) ExecuteInstruction() {
	fmt.Printf("M68K: Starting execution at PC=%08x\n", cpu.PC)

	instructionCount := uint64(0)
	lastPC := uint32(0)
	stuckCounter := 0

	// Initialize perf counters if enabled
	if cpu.PerfEnabled {
		cpu.perfStartTime = time.Now()
		cpu.lastPerfReport = cpu.perfStartTime
		cpu.InstructionCount = 0
	}

	// Cache memory base pointer for fast instruction fetch
	memBase := cpu.memBase

	// Main execution loop - batch check running flag every 4096 instructions
	for cpu.running.Load() {
	innerLoop:
		for range 4096 {
			// STOP state: only interrupts can resume execution
			if cpu.stopped.Load() {
				pendingException := cpu.pendingException.Load()
				if pendingException != 0 && !cpu.inException.Load() {
					cpu.pendingException.Store(0)
					cpu.ProcessException(uint8(pendingException))
				}

				ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
				pending := cpu.pendingInterrupt.Load()
				if pending > ipl && !cpu.inException.Load() {
					cpu.stopped.Store(false)
					cpu.ProcessInterrupt(uint8(pending))
					cpu.pendingInterrupt.Store(0)
				}

				runtime.Gosched()
				break innerLoop
			}

			// Fast inline fetch - PC is always word-aligned after valid execution
			// Read as little-endian uint16, then byte-swap to big-endian
			leValue := *(*uint16)(unsafe.Pointer(uintptr(memBase) + uintptr(cpu.PC)))
			cpu.currentIR = bits.ReverseBytes16(leValue)
			cpu.PC += M68K_WORD_SIZE

			cpu.FetchAndDecodeInstruction()

			instructionCount++

			// Process interrupts + stuck-PC debug check every 256 instructions.
			// Stuck-PC detection is sampled rather than per-instruction - a stuck PC
			// will be caught within ~256 iterations instead of immediately. This is
			// acceptable: stuck-PC is a debug safety net, not a correctness mechanism.
			if instructionCount&0xFF == 0 {
				pendingException := cpu.pendingException.Load()
				if pendingException != 0 && !cpu.inException.Load() {
					cpu.pendingException.Store(0)
					cpu.ProcessException(uint8(pendingException))
				}

				pending := cpu.pendingInterrupt.Load()
				if pending != 0 {
					ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
					if pending > ipl && !cpu.inException.Load() {
						cpu.ProcessInterrupt(uint8(pending))
						cpu.pendingInterrupt.Store(0)
					}
				}

				// Stuck-PC detection (sampled every 256 instructions)
				if cpu.PC == lastPC {
					stuckCounter++
					if stuckCounter > 10 {
						fmt.Printf("Error: PC stuck at 0x%08X, skipping instruction\n", cpu.PC)
						cpu.PC += 2
						stuckCounter = 0
					}
				} else {
					stuckCounter = 0
					lastPC = cpu.PC
				}
			}
		}

		// Performance monitoring
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				ips := float64(instructionCount) / elapsed
				mips := ips / 1_000_000
				fmt.Printf("M68K: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(instructionCount), elapsed)
				cpu.lastPerfReport = now
			}
		}
	}

	fmt.Printf("\n\nM68K: CPU halted at PC=%08x after %d instructions\n",
		cpu.PC, instructionCount)
}

// StepOne executes a single M68K instruction and returns 1.
// Must only be called when the CPU is frozen.
func (cpu *M68KCPU) StepOne() int {
	if cpu.stopped.Load() {
		return 0
	}

	// Fast inline fetch
	if cpu.PC >= uint32(len(cpu.memory))-2 {
		return 0
	}
	leValue := *(*uint16)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(cpu.PC)))
	cpu.currentIR = bits.ReverseBytes16(leValue)
	cpu.PC += M68K_WORD_SIZE

	cpu.FetchAndDecodeInstruction()
	return 1
}

func (cpu *M68KCPU) FillPrefetch() {
	if cpu.prefetchSize < M68K_PREFETCH_SIZE {
		prefetchAddr := cpu.PC + uint32(cpu.prefetchSize*M68K_WORD_SIZE)

		// Check if prefetching would go beyond memory bounds
		if prefetchAddr >= M68K_MEMORY_SIZE-M68K_WORD_SIZE {
			fmt.Printf("Warning: Prefetch beyond memory bounds: addr=0x%08X\n", prefetchAddr)
			cpu.running.Store(false)
			return
		}

		for i := cpu.prefetchSize; i < M68K_PREFETCH_SIZE; i++ {
			if prefetchAddr >= M68K_MEMORY_SIZE-M68K_WORD_SIZE {
				break
			}
			cpu.prefetchQueue[i] = cpu.Read16(prefetchAddr)
			prefetchAddr += M68K_WORD_SIZE
			cpu.prefetchSize++
		}
	}
}
func (cpu *M68KCPU) DumpRegisters() {
	fmt.Println("\nM68K CPU Register Dump:")

	// Data registers
	fmt.Printf("D0: %08X  D1: %08X  D2: %08X  D3: %08X\n",
		cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3])
	fmt.Printf("D4: %08X  D5: %08X  D6: %08X  D7: %08X\n",
		cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7])

	// Address registers
	fmt.Printf("A0: %08X  A1: %08X  A2: %08X  A3: %08X\n",
		cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3])
	fmt.Printf("A4: %08X  A5: %08X  A6: %08X  A7: %08X (SP)\n",
		cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7])

	// Programme counter and status register
	fmt.Printf("PC: %08X  SR: %04X  ", cpu.PC, cpu.SR)

	// Flags
	fmt.Printf("Flags: [")
	if (cpu.SR & M68K_SR_X) != 0 {
		fmt.Printf("X")
	} else {
		fmt.Printf("-")
	}
	if (cpu.SR & M68K_SR_N) != 0 {
		fmt.Printf("N")
	} else {
		fmt.Printf("-")
	}
	if (cpu.SR & M68K_SR_Z) != 0 {
		fmt.Printf("Z")
	} else {
		fmt.Printf("-")
	}
	if (cpu.SR & M68K_SR_V) != 0 {
		fmt.Printf("V")
	} else {
		fmt.Printf("-")
	}
	if (cpu.SR & M68K_SR_C) != 0 {
		fmt.Printf("C")
	} else {
		fmt.Printf("-")
	}
	fmt.Printf("]  ")

	// Supervisor mode
	if (cpu.SR & M68K_SR_S) != 0 {
		fmt.Printf("Supervisor Mode  ")
	} else {
		fmt.Printf("User Mode  ")
	}

	// Interrupt priority level
	fmt.Printf("IPL: %d\n", (cpu.SR&M68K_SR_IPL)>>M68K_SR_SHIFT)

	fmt.Println()
}

// isCoprocSharedAddr returns true if the address is in a coprocessor shared data
// region where byte-swap should be skipped: mailbox (0x820000-0x820FFF) or user
// data buffers (0x400000-0x7FFFFF). Worker code regions are NOT included - instruction
// fetch must still byte-swap for correct BE opcode decoding.
func (cpu *M68KCPU) isCoprocSharedAddr(addr uint32) bool {
	return cpu.CoprocMode &&
		((addr >= 0x400000 && addr < 0x800000) ||
			(addr >= 0x820000 && addr < 0x821000))
}

func (cpu *M68KCPU) Read8(addr uint32) uint8 {
	// Lock-free fast path for non-I/O addresses using unsafe pointer
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		return *(*byte)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr)))
	}

	// Terminal device always returns zero to indicate ready state
	if addr == TERM_OUT || addr == TERM_OUT_SIGNEXT {
		return 0
	}

	// For addresses >= M68K_MEMORY_SIZE, let bus handle (Atari ST hardware registers)
	// Lock-free I/O path - bus handles its own synchronisation
	if fb, ok := cpu.bus.(faultingBus); ok {
		value, ok := fb.Read8WithFault(addr)
		if !ok {
			cpu.recordFault(addr, 1, false, 0)
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
			return 0
		}
		return value
	}
	return cpu.bus.Read8(addr)
}
func (cpu *M68KCPU) Read16(addr uint32) uint16 {
	if (addr & M68K_BYTE_SIZE) != 0 {
		hi := uint16(cpu.Read8(addr))
		lo := uint16(cpu.Read8(addr + 1))
		return (hi << 8) | lo
	}

	// Lock-free fast path for non-I/O addresses using unsafe pointer
	// Read as little-endian uint16, then byte-swap to big-endian
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		leValue := *(*uint16)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr)))
		return bits.ReverseBytes16(leValue)
	}

	// For addresses >= M68K_MEMORY_SIZE, let bus handle (Atari ST hardware registers)

	// Lock-free I/O path - bus handles its own synchronisation
	var leValue uint16
	if fb, ok := cpu.bus.(faultingBus); ok {
		value, ok := fb.Read16WithFault(addr)
		if !ok {
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
			return 0
		}
		leValue = value
	} else {
		leValue = cpu.bus.Read16(addr)
	}

	// Coprocessor shared data: return LE value directly (no byte-swap)
	if cpu.isCoprocSharedAddr(addr) {
		return leValue
	}

	// Endian conversion required because host bus is little-endian
	return bits.ReverseBytes16(leValue)
}
func (cpu *M68KCPU) Read32(addr uint32) uint32 {
	if (addr & M68K_BYTE_SIZE) != 0 {
		b0 := uint32(cpu.Read8(addr))
		b1 := uint32(cpu.Read8(addr + 1))
		b2 := uint32(cpu.Read8(addr + 2))
		b3 := uint32(cpu.Read8(addr + 3))
		return (b0 << 24) | (b1 << 16) | (b2 << 8) | b3
	}

	// Lock-free fast path for non-I/O addresses using unsafe pointer
	// Read as little-endian uint32, then byte-swap to big-endian
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		leValue := *(*uint32)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr)))
		return bits.ReverseBytes32(leValue)
	}

	// I/O register path - return value directly (no byte-swap)
	// Hardware handlers return numeric values, not memory byte order
	// Lock-free - bus handles its own synchronisation
	if addr >= 0xF0000 && addr < 0x100000 {
		if fb, ok := cpu.bus.(faultingBus); ok {
			value, ok := fb.Read32WithFault(addr)
			if !ok {
				cpu.ProcessException(M68K_VEC_BUS_ERROR)
				return 0
			}
			return value
		}
		return cpu.bus.Read32(addr)
	}

	var leValue uint32
	if fb, ok := cpu.bus.(faultingBus); ok {
		value, ok := fb.Read32WithFault(addr)
		if !ok {
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
			return 0
		}
		leValue = value
	} else {
		leValue = cpu.bus.Read32(addr)
	}

	// Coprocessor shared data: return LE value directly (no byte-swap)
	if cpu.isCoprocSharedAddr(addr) {
		return leValue
	}

	// Endian conversion required because host bus is little-endian
	return bits.ReverseBytes32(leValue)
}
func (cpu *M68KCPU) Write8(addr uint32, value uint8) {
	// Terminal device has no buffer to avoid latency
	if addr == TERM_OUT || addr == TERM_OUT_SIGNEXT {
		fmt.Printf("%c", value)
		cpu.bus.Write8(TERM_OUT, value)
		return
	}

	// Fast path: non-I/O memory using unsafe pointer
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		*(*byte)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))) = value
		return
	}

	// For addresses >= M68K_MEMORY_SIZE, let bus handle (Atari ST hardware registers)
	// Lock-free - bus handles its own synchronisation
	if fb, ok := cpu.bus.(faultingBus); ok {
		if !fb.Write8WithFault(addr, value) {
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
		}
		return
	}
	cpu.bus.Write8(addr, value)
}
func (cpu *M68KCPU) Write16(addr uint32, value uint16) {
	if (addr & M68K_BYTE_SIZE) != 0 {
		cpu.Write8(addr, uint8(value>>8))
		cpu.Write8(addr+1, uint8(value))
		return
	}

	// Fast path: non-I/O memory using unsafe pointer
	// Byte-swap from big-endian to little-endian and write as uint16
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		*(*uint16)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))) = bits.ReverseBytes16(value)
		return
	}

	// For addresses >= M68K_MEMORY_SIZE, let bus handle (Atari ST hardware registers)

	// Coprocessor shared data: write value directly (no byte-swap)
	// Normal path: endian conversion required because host bus is little-endian
	var busValue uint16
	if cpu.isCoprocSharedAddr(addr) {
		busValue = value
	} else {
		busValue = bits.ReverseBytes16(value)
	}

	// Lock-free - bus handles its own synchronisation
	if fb, ok := cpu.bus.(faultingBus); ok {
		if !fb.Write16WithFault(addr, busValue) {
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
		}
		return
	}
	cpu.bus.Write16(addr, busValue)
}
func (cpu *M68KCPU) Write32(addr uint32, value uint32) {
	if (addr & M68K_BYTE_SIZE) != 0 {
		cpu.Write8(addr, uint8(value>>24))
		cpu.Write8(addr+1, uint8(value>>16))
		cpu.Write8(addr+2, uint8(value>>8))
		cpu.Write8(addr+3, uint8(value))
		return
	}

	// Fast path: non-I/O memory using unsafe pointer
	// Byte-swap from big-endian to little-endian and write as uint32
	// EXCLUDE VGA windows (0xA0000-0xBFFFF) which need bus routing
	if addr < 0xA0000 {
		*(*uint32)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))) = bits.ReverseBytes32(value)
		return
	}

	// Coprocessor shared data: write value directly (no byte-swap)
	// Normal path: endian conversion required because host bus is little-endian
	var leValue uint32
	if cpu.isCoprocSharedAddr(addr) {
		leValue = value
	} else {
		leValue = bits.ReverseBytes32(value)
	}

	// VRAM fast path - direct write, no mutex, no bus overhead
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		if offset+4 <= uint32(len(cpu.vramDirect)) {
			binary.LittleEndian.PutUint32(cpu.vramDirect[offset:], leValue)
			cpu.VRAMWriteCount++
			return
		}
	}

	// I/O register path - pass original value (not byte-swapped) for hardware registers
	// Hardware handlers expect numeric values, not memory byte order
	if addr >= 0xF0000 && addr < 0x100000 {
		if fb, ok := cpu.bus.(faultingBus); ok {
			if !fb.Write32WithFault(addr, value) {
				cpu.ProcessException(M68K_VEC_BUS_ERROR)
			}
			return
		}
		cpu.bus.Write32(addr, value)
		return
	}

	// Non-I/O fast path - direct memory write with endian conversion
	// CPU is the sole writer, so no synchronisation needed
	// Lock-free - bus handles its own synchronisation
	if fb, ok := cpu.bus.(faultingBus); ok {
		if !fb.Write32WithFault(addr, leValue) {
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
		}
		return
	}
	cpu.bus.Write32(addr, leValue)
}

// AttachDirectVRAM enables direct VRAM access mode for maximum video throughput.
// The buffer should be obtained from VideoChip.EnableDirectMode().
// This bypasses the memory bus and mutex for VRAM writes.
func (cpu *M68KCPU) AttachDirectVRAM(buffer []byte, start, end uint32) {
	cpu.vramDirect = buffer
	cpu.vramStart = start
	cpu.vramEnd = end
}

// DetachDirectVRAM disables direct VRAM access, returning to bus-based writes.
func (cpu *M68KCPU) DetachDirectVRAM() {
	cpu.vramDirect = nil
}

// Stack operations
func (cpu *M68KCPU) Push16(value uint16) {
	cpu.AddrRegs[7] -= M68K_WORD_SIZE
	if cpu.AddrRegs[7] < cpu.stackLowerBound {
		fmt.Printf("%s m68k.Push16\tStack overflow error at PC=%08x (SP=%08x, limit=%08x)\n",
			time.Now().Format("15:04:05.000"), cpu.PC, cpu.AddrRegs[7], cpu.stackLowerBound)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return
	}
	cpu.Write16(cpu.AddrRegs[7], value)
}
func (cpu *M68KCPU) Push32(value uint32) {
	cpu.AddrRegs[7] -= M68K_LONG_SIZE
	if cpu.AddrRegs[7] < cpu.stackLowerBound {
		fmt.Printf("%s m68k.Push32\tStack overflow error at PC=%08x (SP=%08x, limit=%08x)\n",
			time.Now().Format("15:04:05.000"), cpu.PC, cpu.AddrRegs[7], cpu.stackLowerBound)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return
	}
	cpu.Write32(cpu.AddrRegs[7], value)
}
func (cpu *M68KCPU) Pop16() uint16 {
	if cpu.AddrRegs[7] >= cpu.stackUpperBound {
		fmt.Printf("M68K Stack underflow error at PC=%08x (SP=%08x, limit=%08x)\n",
			cpu.PC, cpu.AddrRegs[7], cpu.stackUpperBound)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return 0
	}
	value := cpu.Read16(cpu.AddrRegs[7])
	cpu.AddrRegs[7] += M68K_WORD_SIZE
	return value
}
func (cpu *M68KCPU) Pop32() uint32 {
	if cpu.AddrRegs[7] >= cpu.stackUpperBound {
		fmt.Printf("M68K Stack underflow error at PC=%08x (SP=%08x, limit=%08x)\n",
			cpu.PC, cpu.AddrRegs[7], cpu.stackUpperBound)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return 0
	}
	value := cpu.Read32(cpu.AddrRegs[7])
	cpu.AddrRegs[7] += M68K_LONG_SIZE
	return value
}

// Instruction fetching
func (cpu *M68KCPU) Fetch16() uint16 {
	// Fast path: PC is always word-aligned after valid execution
	// Skip alignment check in hot path - only matters for corrupted PC
	addr := cpu.PC
	// Lock-free fast path for non-I/O addresses using unsafe pointer
	// Read as little-endian uint16, then byte-swap to big-endian
	leValue := *(*uint16)(unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr)))
	value := bits.ReverseBytes16(leValue)
	cpu.PC += M68K_WORD_SIZE
	cpu.cycleCounter += M68K_CYCLE_FETCH
	return value
}
func (cpu *M68KCPU) Fetch32() uint32 {
	if (cpu.PC & M68K_BYTE_SIZE) != 0 {
		cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
		return 0
	}
	high := uint32(cpu.Read16(cpu.PC)) << 16
	cpu.PC += 2 // Advance PC after reading high word
	low := uint32(cpu.Read16(cpu.PC))
	cpu.PC += 2 // Advance PC after reading low word
	return high | low
}
func (cpu *M68KCPU) GetEffectiveAddress(mode, reg uint16) uint32 {
	// Handle extended addressing modes (mode 7)
	if mode == 7 {
		switch reg {
		case 0: // Absolute Short
			return uint32(int16(cpu.Fetch16()))

		case 1: // Absolute Long
			high := uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
			low := uint32(cpu.Fetch16())
			return high | low

		case 2: // PC with Displacement
			disp := int16(cpu.Fetch16())
			return cpu.PC - M68K_WORD_SIZE + uint32(disp)

		case 3: // PC with Index
			extWord := cpu.Fetch16()
			// Handle brief or full extension word format
			if (extWord & M68K_EXT_FULL_FORMAT) != 0 {
				return cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
			}

			// Brief format
			idxReg := (extWord >> 12) & 0x0F
			idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
			idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01
			disp8 := int8(extWord & 0xFF)

			var idxValue uint32
			if idxType == M68K_EXT_DATA_REG_TYPE {
				idxValue = cpu.DataRegs[idxReg&0x07]
			} else {
				idxValue = cpu.AddrRegs[idxReg&0x07]
			}

			if idxSize == 0 {
				// Sign extend word to long
				if (idxValue & 0x8000) != 0 {
					idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
				} else {
					idxValue &= 0x0000FFFF
				}
			}

			// Apply scale factor
			scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)
			idxValue <<= scale

			return (cpu.PC - M68K_WORD_SIZE) + uint32(disp8) + idxValue

		case 4: // Immediate
			// Not valid for addressing - bug in instruction decoder
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 0

		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 0
		}
	}

	// Handle normal addressing modes
	switch mode {
	case M68K_AM_DR: // Data Register Direct - invalid for memory addressing
		return 0

	case M68K_AM_AR: // Address Register Direct
		return cpu.AddrRegs[reg]

	case M68K_AM_AR_IND: // Address Register Indirect
		return cpu.AddrRegs[reg]

	case M68K_AM_AR_POST: // Address Register Indirect with Postincrement
		return cpu.AddrRegs[reg]

	case M68K_AM_AR_PRE: // Address Register Indirect with Predecrement
		return cpu.AddrRegs[reg]

	case M68K_AM_AR_DISP: // Address Register Indirect with Displacement
		disp := int16(cpu.Fetch16())
		return cpu.AddrRegs[reg] + uint32(disp)

	case M68K_AM_AR_INDEX: // Address Register Indirect with Index
		extWord := cpu.Fetch16()

		// Enhanced 68020 form with extension word?
		if (extWord & M68K_EXT_FULL_FORMAT) != 0 {
			return cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[reg], true)
		}

		// Brief format
		idxReg := (extWord >> 12) & 0x0F
		idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
		idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01
		disp8 := int8(extWord & 0xFF)

		var idxValue uint32
		if idxType == M68K_EXT_DATA_REG_TYPE {
			idxValue = cpu.DataRegs[idxReg&0x07]
		} else {
			idxValue = cpu.AddrRegs[idxReg&0x07]
		}

		if idxSize == 0 {
			// Sign extend word to long
			if (idxValue & 0x8000) != 0 {
				idxValue |= 0xFFFF0000
			} else {
				idxValue &= 0x0000FFFF
			}
		}

		// Apply scale factor
		scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)
		idxValue <<= scale

		addr := cpu.AddrRegs[reg] + uint32(disp8) + idxValue
		return addr

	default:
		fmt.Printf("Invalid addressing mode: mode=%d, reg=%d\n", mode, reg)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 0
	}
}
func (cpu *M68KCPU) GetIndexWithExtWords(extWord uint16, baseAddr uint32, isAreg bool) uint32 {
	// Extract extension word fields using our constants
	bs := (extWord >> M68K_EXT_BS_BIT) & 0x01
	is := (extWord >> M68K_EXT_IS_BIT) & 0x01
	bd := (extWord >> M68K_EXT_BD_START_BIT) & ((1 << M68K_EXT_BD_SIZE) - 1)
	scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)

	idxReg := extWord & M68K_EXT_REG_MASK
	idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
	idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01

	var address uint32 = 0

	// Add base register if not suppressed
	if bs == 0 {
		address = baseAddr
	}

	// Add base displacement
	switch bd {
	case 0: // No displacement
		break
	case 1: // Word displacement
		// Sign-extend word displacement
		displacement := int16(cpu.Fetch16())
		address += uint32(displacement)
	case 2: // Long displacement
		address += cpu.Fetch32()
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 0
	}

	// Add index register if not suppressed
	if is == 0 {
		var idxValue uint32
		if idxType == M68K_EXT_DATA_REG_TYPE {
			idxValue = cpu.DataRegs[idxReg&0x07]
		} else {
			idxValue = cpu.AddrRegs[idxReg&0x07]
		}

		if idxSize == 0 {
			// Sign extend word to long
			if (idxValue & 0x8000) != 0 {
				idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
			} else {
				idxValue &= 0x0000FFFF
			}
		}

		// Apply scaling
		idxValue <<= scale

		address += idxValue
	}

	// Check if there's an additional indirect level
	indLevel := extWord & M68K_EXT_INDIRECTION_MASK
	if indLevel != 0 {
		// Implement memory indirect addressing
		switch indLevel {
		case 1: // [bd,An,Xn] - Memory indirect preindexed
			// Read the address at the computed address
			indirectAddr := cpu.Read32(address)

			// Get outer displacement if available
			if (extWord & 0x04) != 0 { // Check od bit
				switch extWord & 0x03 {
				case 0: // No displacement
					break
				case 1: // Word-sized outer displacement
					// Sign-extend word displacement
					displacement := int16(cpu.Fetch16())
					indirectAddr += uint32(displacement)
				case 2: // Long-sized outer displacement
					indirectAddr += cpu.Fetch32()
				default:
					cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
					return 0
				}
			}

			return indirectAddr

		case 2: // [bd,An],Xn - Memory indirect postindexed
			// First get the intermediate indirect address
			indirectAddr := cpu.Read32(address)

			// Apply outer displacement if needed
			if (extWord & 0x04) != 0 { // Check od bit
				switch extWord & 0x03 {
				case 0: // No displacement
					break
				case 1: // Word-sized outer displacement
					// Sign-extend word displacement
					displacement := int16(cpu.Fetch16())
					indirectAddr += uint32(displacement)
				case 2: // Long-sized outer displacement
					indirectAddr += cpu.Fetch32()
				default:
					cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
					return 0
				}
			}

			// Then apply the index register
			if is == 0 {
				var idxValue uint32
				if idxType == M68K_EXT_DATA_REG_TYPE {
					idxValue = cpu.DataRegs[idxReg&0x07]
				} else {
					idxValue = cpu.AddrRegs[idxReg&0x07]
				}

				if idxSize == 0 {
					// Sign extend word to long
					if (idxValue & 0x8000) != 0 {
						idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
					} else {
						idxValue &= 0x0000FFFF
					}
				}

				// Apply scaling
				idxValue <<= scale

				indirectAddr += idxValue
			}

			return indirectAddr

		case 3: // [bd] - Memory indirect with base register suppressed
			// Read the address at the base displacement
			indirectAddr := cpu.Read32(address)

			// Handle outer displacement if present
			if (extWord & 0x04) != 0 { // Check od bit
				switch extWord & 0x03 {
				case 0: // No displacement
					break
				case 1: // Word-sized outer displacement
					// Sign-extend word displacement
					displacement := int16(cpu.Fetch16())
					indirectAddr += uint32(displacement)
				case 2: // Long-sized outer displacement
					indirectAddr += cpu.Fetch32()
				default:
					cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
					return 0
				}
			}

			// Add index register if not suppressed
			if is == 0 {
				var idxValue uint32
				if idxType == M68K_EXT_DATA_REG_TYPE {
					idxValue = cpu.DataRegs[idxReg&0x07]
				} else {
					idxValue = cpu.AddrRegs[idxReg&0x07]
				}

				if idxSize == 0 {
					// Sign extend word to long
					if (idxValue & 0x8000) != 0 {
						idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
					} else {
						idxValue &= 0x0000FFFF
					}
				}

				// Apply scaling
				idxValue <<= scale

				indirectAddr += idxValue
			}

			return indirectAddr
		}
	}

	return address
}
func (cpu *M68KCPU) GetEACycles(mode, reg uint16) uint32 {
	switch mode {
	case M68K_AM_DR: // Data Register Direct
		return M68K_CYCLE_EA_RD
	case M68K_AM_AR: // Address Register Direct
		return M68K_CYCLE_EA_RD
	case M68K_AM_AR_IND: // Address Register Indirect
		return M68K_CYCLE_EA_AI
	case M68K_AM_AR_POST: // Address Register Indirect with Postincrement
		return M68K_CYCLE_EA_PI
	case M68K_AM_AR_PRE: // Address Register Indirect with Predecrement
		return M68K_CYCLE_EA_PD
	case M68K_AM_AR_DISP: // Address Register Indirect with Displacement
		return M68K_CYCLE_EA_DI
	case M68K_AM_AR_INDEX: // Address Register Indirect with Index
		// Basic index mode - more complex modes need additional calculations
		return M68K_CYCLE_EA_IX
	case M68K_AM_ABS_SHORT: // Absolute Short
		return M68K_CYCLE_EA_AW
	case M68K_AM_ABS_LONG: // Absolute Long
		return M68K_CYCLE_EA_AL
	case M68K_AM_PC_DISP: // PC with Displacement
		return M68K_CYCLE_EA_PCDI
	case M68K_AM_PC_INDEX: // PC with Index
		return M68K_CYCLE_EA_PCIX
	case M68K_AM_IMM: // Immediate
		return M68K_CYCLE_EA_IM
	default:
		return 4 // Default value for unknown modes
	}
}

func (cpu *M68KCPU) SetFlags(result uint32, size int) {
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if size == M68K_SIZE_BYTE {
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
}
func (cpu *M68KCPU) SetFlagsNZ(result uint32, size int) {
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z)

	if size == M68K_SIZE_BYTE {
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
}
func (cpu *M68KCPU) CheckCondition(condition uint8) bool {
	n := (cpu.SR & M68K_SR_N) != 0
	z := (cpu.SR & M68K_SR_Z) != 0
	v := (cpu.SR & M68K_SR_V) != 0
	c := (cpu.SR & M68K_SR_C) != 0

	switch condition {
	case M68K_CC_T:
		return true
	case M68K_CC_F:
		return false
	case M68K_CC_HI:
		return !c && !z
	case M68K_CC_LS:
		return c || z
	case M68K_CC_CC:
		return !c
	case M68K_CC_CS:
		return c
	case M68K_CC_NE:
		return !z
	case M68K_CC_EQ:
		return z
	case M68K_CC_VC:
		return !v
	case M68K_CC_VS:
		return v
	case M68K_CC_PL:
		return !n
	case M68K_CC_MI:
		return n
	case M68K_CC_GE:
		return (n && v) || (!n && !v)
	case M68K_CC_LT:
		return (n && !v) || (!n && v)
	case M68K_CC_GT:
		return !z && ((n && v) || (!n && !v))
	case M68K_CC_LE:
		return z || (n && !v) || (!n && v)
	}

	return false
}
func (cpu *M68KCPU) GetCCR() uint8 {
	return uint8(cpu.SR & M68K_SR_CCR)
}
func (cpu *M68KCPU) SetCCR(value uint8) {
	cpu.SR = (cpu.SR & ^uint16(M68K_SR_CCR)) | uint16(value&M68K_SR_CCR)
}

func (cpu *M68KCPU) recordFault(addr uint32, size uint8, write bool, data uint32) {
	cpu.lastFaultAddr = addr
	cpu.lastFaultSize = size
	cpu.lastFaultWrite = write
	cpu.lastFaultData = data
	cpu.lastFaultIsInstruction = cpu.accessIsInstruction
}

func (cpu *M68KCPU) faultFunctionCode() uint16 {
	supervisor := (cpu.SR & M68K_SR_S) != 0
	if cpu.lastFaultIsInstruction {
		if supervisor {
			return 0x6
		}
		return 0x2
	}
	if supervisor {
		return 0x5
	}
	return 0x1
}

func (cpu *M68KCPU) buildSSW() uint16 {
	var ssw uint16
	if cpu.lastFaultIsInstruction {
		ssw |= 1 << 15 // FC
		ssw |= 1 << 13 // RC
	} else {
		ssw |= 1 << 8 // DF
	}

	if cpu.lastFaultWrite {
		ssw |= 1 << 6 // RW
	}

	sizeBits := uint16(0)
	switch cpu.lastFaultSize {
	case 1:
		sizeBits = 0
	case 2:
		sizeBits = 1
	case 4:
		sizeBits = 2
	}
	ssw |= (sizeBits & 0x3) << 4
	ssw |= cpu.faultFunctionCode() & 0x7
	return ssw
}

func (cpu *M68KCPU) exceptionFrameWords(format uint16) int {
	switch format {
	case M68K_FRAME_FMT_0, M68K_FRAME_FMT_1:
		return 4
	case M68K_FRAME_FMT_2:
		return 6
	case M68K_FRAME_FMT_9:
		return 10
	case M68K_FRAME_FMT_A:
		return 16
	case M68K_FRAME_FMT_B:
		return 46
	default:
		return 0
	}
}

func (cpu *M68KCPU) pushExceptionFrame(oldSR uint16, oldPC uint32, vector uint8, format uint16) {
	words := cpu.exceptionFrameWords(format)
	if words == 0 {
		format = M68K_FRAME_FMT_0
		words = cpu.exceptionFrameWords(format)
	}

	extraWords := words - 4
	extra := make([]uint16, extraWords)
	if format == M68K_FRAME_FMT_A && extraWords >= 12 {
		extra[0] = 0
		extra[1] = cpu.buildSSW()
		extra[2] = 0
		extra[3] = 0
		extra[4] = uint16(cpu.lastFaultAddr >> 16)
		extra[5] = uint16(cpu.lastFaultAddr)
		extra[6] = 0
		extra[7] = 0
		extra[8] = uint16(cpu.lastFaultData >> 16)
		extra[9] = uint16(cpu.lastFaultData)
		extra[10] = 0
		extra[11] = 0
	}

	formatWord := (format << 12) | (uint16(vector) << 2)
	for i := extraWords - 1; i >= 0; i-- {
		cpu.Push16(extra[i])
	}

	cpu.Push16(formatWord)
	cpu.Push32(oldPC)
	cpu.Push16(oldSR)
}

func (cpu *M68KCPU) swapStacksForMode(newSupervisor bool) {
	if newSupervisor {
		cpu.USP = cpu.AddrRegs[7]
		cpu.AddrRegs[7] = cpu.SSP
	} else {
		cpu.SSP = cpu.AddrRegs[7]
		cpu.AddrRegs[7] = cpu.USP
	}
}

func (cpu *M68KCPU) ProcessException(vector uint8) {
	// Double fault halts CPU to prevent infinite recursion
	if cpu.inException.Load() && (vector == M68K_VEC_BUS_ERROR || vector == M68K_VEC_ADDRESS_ERROR) {
		fmt.Printf("M68K: Double fault (exception %d during exception handling), halting CPU\n", vector)
		cpu.running.Store(false)
		return
	}

	// Defer non-critical exceptions to prevent stack corruption
	if cpu.inException.Load() && vector != M68K_VEC_RESET {
		fmt.Printf("M68K: Exception %d during exception handling, deferring\n", vector)
		cpu.pendingException.Store(uint32(vector))
		return
	}

	cpu.inException.Store(true)

	oldSR := cpu.SR
	oldPC := cpu.PC

	// RESET doesn't push state because stack may be corrupted
	if vector == M68K_VEC_RESET {
		cpu.SR = M68K_SR_S
		cpu.SR &= ^uint16(M68K_SR_T0 | M68K_SR_T1)

		cpu.AddrRegs[7] = cpu.Read32(0)
		cpu.PC = cpu.Read32(M68K_RESET_VECTOR)

		cpu.inException.Store(false)
		return
	}

	// Hardware automatically enters supervisor mode during exceptions
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.swapStacksForMode(true)
	}
	cpu.SR |= M68K_SR_S
	cpu.SR &= ^uint16(M68K_SR_T0 | M68K_SR_T1)

	frameFormat := uint16(M68K_FRAME_FMT_0)
	if vector == M68K_VEC_BUS_ERROR || vector == M68K_VEC_ADDRESS_ERROR {
		frameFormat = M68K_FRAME_FMT_A
	}
	cpu.pushExceptionFrame(oldSR, oldPC, vector, frameFormat)

	vecAddr := cpu.VBR + uint32(vector)*M68K_LONG_SIZE
	newPC := cpu.Read32(vecAddr)

	// Uninitialised vectors: restore state and continue (ignore exception)
	// This matches IE32 behaviour where missing handlers don't halt execution
	if newPC == 0 {
		// Restore stack and continue from where we were
		cpu.Pop16() // discard saved SR
		cpu.Pop32() // discard saved PC
		cpu.SR = oldSR
		cpu.PC = oldPC
		cpu.inException.Store(false)
		return
	}

	cpu.PC = newPC
	cpu.cycleCounter += M68K_CYCLE_EXCEPTION
	cpu.inException.Store(false)
}
func (cpu *M68KCPU) ProcessInterrupt(level uint8) {
	// Ignore if current interrupt mask is higher
	currentIPL := uint8((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
	if level <= currentIPL && level < M68K_VEC_LEVEL7-M68K_VEC_LEVEL1+1 {
		return
	}

	// Level 7 is non-maskable
	if level == M68K_VEC_LEVEL7-M68K_VEC_LEVEL1+1 || level > currentIPL {
		// Save old interrupt mask
		oldSR := cpu.SR

		// Update interrupt mask
		cpu.SR &= ^uint16(M68K_SR_IPL)
		cpu.SR |= uint16(level) << M68K_SR_SHIFT

		// Process as exception using autovector
		vector := M68K_VEC_LEVEL1 + level - 1

		// Save PC and SR
		if (cpu.SR & M68K_SR_S) == 0 {
			cpu.swapStacksForMode(true)
		}
		cpu.pushExceptionFrame(oldSR, cpu.PC, uint8(vector), M68K_FRAME_FMT_0)

		// Get new PC from vector table
		vecAddr := cpu.VBR + uint32(vector)*M68K_LONG_SIZE
		cpu.PC = cpu.Read32(vecAddr)

		// Set supervisor mode
		cpu.SR |= M68K_SR_S

		// Disable tracing during exception handling
		cpu.SR &= ^uint16(M68K_SR_T0 | M68K_SR_T1)

		cpu.inException.Store(true)
	}
}
func (cpu *M68KCPU) ProcessTerminalOutput(value uint32) {
	// Extract the character to print (least significant byte)
	char := byte(value & 0xFF)

	// Print to terminal
	fmt.Printf("%c", char)
}

// Data movement instruction execution
func (cpu *M68KCPU) ExecMove(srcMode, srcReg, destMode, destReg uint16, size uint16) {
	var value uint32
	var effectiveSize int

	// Determine operation size
	switch size {
	case 1: // Size bits 01 = byte
		effectiveSize = M68K_SIZE_BYTE
	case 3: // Size bits 11 = word
		effectiveSize = M68K_SIZE_WORD
	case 2: // Size bits 10 = long
		effectiveSize = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get source value
	if srcMode == M68K_AM_DR {
		// Data register direct
		value = cpu.DataRegs[srcReg]
		if effectiveSize == M68K_SIZE_BYTE {
			value &= 0xFF
		} else if effectiveSize == M68K_SIZE_WORD {
			value &= 0xFFFF
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if srcMode == M68K_AM_AR {
		// Address register direct
		value = cpu.AddrRegs[srcReg]
		if effectiveSize == M68K_SIZE_BYTE {
			value &= 0xFF
		} else if effectiveSize == M68K_SIZE_WORD {
			value &= 0xFFFF
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if srcMode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[srcReg]
		if effectiveSize == M68K_SIZE_BYTE {
			value = uint32(cpu.Read8(addr))
		} else if effectiveSize == M68K_SIZE_WORD {
			value = uint32(cpu.Read16(addr))
		} else {
			value = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if srcMode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[srcReg]
		if effectiveSize == M68K_SIZE_BYTE {
			value = uint32(cpu.Read8(addr))
			// Increment by 1 for byte access
			cpu.AddrRegs[srcReg] += M68K_BYTE_SIZE
			// Special case for A7 (SP) - always increment by 2 for byte operations
			if srcReg == 7 {
				cpu.AddrRegs[srcReg] += M68K_BYTE_SIZE
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			value = uint32(cpu.Read16(addr))
			cpu.AddrRegs[srcReg] += M68K_WORD_SIZE
		} else {
			value = cpu.Read32(addr)
			cpu.AddrRegs[srcReg] += M68K_LONG_SIZE
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if srcMode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		var decrementSize uint32

		// Calculate decrement size based on operation size
		if effectiveSize == M68K_SIZE_BYTE {
			decrementSize = M68K_BYTE_SIZE
			// Special case for A7 (SP) - always decrement by 2 for byte operations
			if srcReg == 7 {
				decrementSize = M68K_WORD_SIZE
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			decrementSize = M68K_WORD_SIZE
		} else { // Long
			decrementSize = M68K_LONG_SIZE
		}

		// Decrement the address register
		cpu.AddrRegs[srcReg] -= decrementSize

		// Read from the decremented address
		addr := cpu.AddrRegs[srcReg]
		if effectiveSize == M68K_SIZE_BYTE {
			value = uint32(cpu.Read8(addr))
		} else if effectiveSize == M68K_SIZE_WORD {
			value = uint32(cpu.Read16(addr))
		} else {
			value = cpu.Read32(addr)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if srcMode == M68K_AM_AR_DISP {
		// Address register indirect with displacement
		disp := int16(cpu.Fetch16())
		addr := cpu.AddrRegs[srcReg] + uint32(disp)

		if effectiveSize == M68K_SIZE_BYTE {
			value = uint32(cpu.Read8(addr))
		} else if effectiveSize == M68K_SIZE_WORD {
			value = uint32(cpu.Read16(addr))
		} else {
			value = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_DI
	} else if srcMode == M68K_AM_AR_INDEX {
		// Address register indirect with index
		extWord := cpu.Fetch16()

		// Process the brief or full extension word
		if (extWord & M68K_EXT_FULL_FORMAT) != 0 {
			addr := cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[srcReg], true)
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
		} else {
			// Brief format
			idxReg := (extWord >> 12) & 0x0F
			idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
			idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01
			disp8 := int8(extWord & 0xFF)

			var idxValue uint32
			if idxType == M68K_EXT_DATA_REG_TYPE {
				idxValue = cpu.DataRegs[idxReg&0x07]
			} else {
				idxValue = cpu.AddrRegs[idxReg&0x07]
			}

			if idxSize == 0 {
				// Sign extend word to long
				if (idxValue & 0x8000) != 0 {
					idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
				} else {
					idxValue &= 0x0000FFFF
				}
			}

			// Apply scale factor
			scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)
			idxValue <<= scale

			// Calculate effective address
			addr := cpu.AddrRegs[srcReg] + uint32(disp8) + idxValue

			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
	} else if srcMode == 7 { // Extended addressing modes
		switch srcReg {
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
		case 2: // PC with Displacement
			disp := int16(cpu.Fetch16())
			addr := cpu.PC - M68K_WORD_SIZE + uint32(disp)
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCDI
		case 3: // PC with Index
			extWord := cpu.Fetch16()
			addr := cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Read8(addr))
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Read16(addr))
			} else {
				value = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCIX
		case 4: // Immediate
			if effectiveSize == M68K_SIZE_BYTE {
				value = uint32(cpu.Fetch16() & 0xFF)
			} else if effectiveSize == M68K_SIZE_WORD {
				value = uint32(cpu.Fetch16())
			} else {
				value = cpu.Fetch32()
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_EA_IM
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Set destination value
	if destMode == M68K_AM_DR {
		// Data register direct
		if effectiveSize == M68K_SIZE_BYTE {
			// For data register direct mode, do not sign extend
			cpu.DataRegs[destReg] = (cpu.DataRegs[destReg] & 0xFFFFFF00) | (value & 0xFF)
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.DataRegs[destReg] = (cpu.DataRegs[destReg] & 0xFFFF0000) | (value & 0xFFFF)
		} else {
			cpu.DataRegs[destReg] = value
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if destMode == M68K_AM_AR {
		// Address register direct - MOVEA instruction
		if effectiveSize == M68K_SIZE_BYTE {
			// For byte operations, sign-extend to 32 bits for address registers
			if (value & 0x80) != 0 {
				value |= 0xFFFFFF00
			}
			cpu.AddrRegs[destReg] = value
		} else if effectiveSize == M68K_SIZE_WORD {
			// For word operations, sign-extend to 32 bits for address registers
			if (value & 0x8000) != 0 {
				value |= 0xFFFF0000
			}
			cpu.AddrRegs[destReg] = value
		} else {
			cpu.AddrRegs[destReg] = value
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if destMode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[destReg]
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.Write8(addr, uint8(value))
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.Write16(addr, uint16(value))
		} else {
			cpu.Write32(addr, value)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	} else if destMode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[destReg]
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.Write8(addr, uint8(value))
			cpu.AddrRegs[destReg] += M68K_BYTE_SIZE
			// Special case for A7 (SP) - always increment by 2 for byte operations
			if destReg == 7 {
				cpu.AddrRegs[destReg] += M68K_BYTE_SIZE
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.Write16(addr, uint16(value))
			cpu.AddrRegs[destReg] += M68K_WORD_SIZE
		} else {
			cpu.Write32(addr, value)
			cpu.AddrRegs[destReg] += M68K_LONG_SIZE
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	} else if destMode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		var decrementSize uint32

		// Calculate decrement size based on operation size
		if effectiveSize == M68K_SIZE_BYTE {
			decrementSize = M68K_BYTE_SIZE
			// Special case for A7 (SP) - always decrement by 2 for byte operations
			if destReg == 7 {
				decrementSize = M68K_WORD_SIZE
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			decrementSize = M68K_WORD_SIZE
		} else { // Long
			decrementSize = M68K_LONG_SIZE
		}

		// Decrement the address register
		cpu.AddrRegs[destReg] -= decrementSize

		// Write to the decremented address
		addr := cpu.AddrRegs[destReg]
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.Write8(addr, uint8(value))
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.Write16(addr, uint16(value))
		} else {
			cpu.Write32(addr, value)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	} else if destMode == M68K_AM_AR_DISP {
		// Address register indirect with displacement
		disp := int16(cpu.Fetch16())
		addr := cpu.AddrRegs[destReg] + uint32(disp)

		if effectiveSize == M68K_SIZE_BYTE {
			cpu.Write8(addr, uint8(value))
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.Write16(addr, uint16(value))
		} else {
			cpu.Write32(addr, value)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE + M68K_CYCLE_EA_DI
	} else if destMode == M68K_AM_AR_INDEX {
		// Address register indirect with index
		extWord := cpu.Fetch16()

		// Process the brief or full extension word
		if (extWord & M68K_EXT_FULL_FORMAT) != 0 {
			addr := cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[destReg], true)
			if effectiveSize == M68K_SIZE_BYTE {
				cpu.Write8(addr, uint8(value))
			} else if effectiveSize == M68K_SIZE_WORD {
				cpu.Write16(addr, uint16(value))
			} else {
				cpu.Write32(addr, value)
			}
		} else {
			// Brief format
			idxReg := (extWord >> 12) & 0x0F
			idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
			idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01
			disp8 := int8(extWord & 0xFF)

			var idxValue uint32
			if idxType == M68K_EXT_DATA_REG_TYPE {
				idxValue = cpu.DataRegs[idxReg&0x07]
			} else {
				idxValue = cpu.AddrRegs[idxReg&0x07]
			}

			if idxSize == 0 {
				// Sign extend word to long
				if (idxValue & 0x8000) != 0 {
					idxValue = (idxValue & 0xFFFF) | 0xFFFF0000
				} else {
					idxValue &= 0x0000FFFF
				}
			}

			// Apply scale factor
			scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)
			idxValue <<= scale

			// Calculate effective address
			addr := cpu.AddrRegs[destReg] + uint32(disp8) + idxValue

			if effectiveSize == M68K_SIZE_BYTE {
				cpu.Write8(addr, uint8(value))
			} else if effectiveSize == M68K_SIZE_WORD {
				cpu.Write16(addr, uint16(value))
			} else {
				cpu.Write32(addr, value)
			}
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE + M68K_CYCLE_EA_IX
	} else if destMode == 7 { // Absolute addressing
		switch destReg {
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if effectiveSize == M68K_SIZE_BYTE {
				cpu.Write8(addr, uint8(value))
			} else if effectiveSize == M68K_SIZE_WORD {
				cpu.Write16(addr, uint16(value))
			} else {
				cpu.Write32(addr, value)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_WRITE + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if effectiveSize == M68K_SIZE_BYTE {
				cpu.Write8(addr, uint8(value))
			} else if effectiveSize == M68K_SIZE_WORD {
				cpu.Write16(addr, uint16(value))
			} else {
				cpu.Write32(addr, value)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_WRITE + M68K_CYCLE_EA_AL
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Only set condition codes for standard MOVE, not MOVEA
	if destMode != M68K_AM_AR {
		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

		// Set Z flag if result is zero
		if effectiveSize == M68K_SIZE_BYTE {
			if (value & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			// Set N flag if MSB is set
			if (value & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if (value & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			// Set N flag if MSB is set
			if (value & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else { // M68K_SIZE_LONG
			if value == 0 {
				cpu.SR |= M68K_SR_Z
			}
			// Set N flag if MSB is set
			if (value & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}
	}
}
func (cpu *M68KCPU) ExecMoveFromUSP() {
	// Check supervisor mode
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Extract register number
	reg := cpu.currentIR & 0x7

	// Move USP to specified address register
	cpu.AddrRegs[reg] = cpu.USP

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecMoveToUSP() {
	// Check supervisor mode
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Extract register number
	reg := cpu.currentIR & 0x7

	// Move from specified address register to USP
	cpu.USP = cpu.AddrRegs[reg]

	cpu.cycleCounter += M68K_CYCLE_REG
}

// ExecMoveFromSR - Privileged on 68010+; allows supervisor to examine processor state
func (cpu *M68KCPU) ExecMoveFromSR(mode, reg uint16) {
	// On 68010+, this is privileged. 68000 allows user mode access.
	// We'll implement 68010+ behaviour for security
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Write SR to destination
	if mode == 0 {
		// Data register direct (mode=0) - store as word in low 16 bits
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | uint32(cpu.SR)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		cpu.AddrRegs[reg] -= M68K_WORD_SIZE
		cpu.Write16(cpu.AddrRegs[reg], cpu.SR)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[reg]
		cpu.Write16(addr, cpu.SR)
		cpu.AddrRegs[reg] += M68K_WORD_SIZE
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	} else {
		// Other memory destinations - convert raw mode/reg to effective address
		addr := cpu.GetEffectiveAddress(mode, reg)
		cpu.Write16(addr, cpu.SR)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}

// ExecMoveToSR - Mode/interrupt level changes require privilege to prevent security bypass
func (cpu *M68KCPU) ExecMoveToSR(mode, reg uint16) {
	// Always privileged - controls system state
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	var newSR uint16

	// Read source operand
	if mode == 0 {
		// Data register direct (mode=0)
		newSR = uint16(cpu.DataRegs[reg] & 0xFFFF)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && reg == 4 {
		// Immediate (mode=7, reg=4)
		newSR = cpu.Fetch16()
		cpu.cycleCounter += M68K_CYCLE_FETCH
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[reg]
		newSR = cpu.Read16(addr)
		cpu.AddrRegs[reg] += M68K_WORD_SIZE
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		cpu.AddrRegs[reg] -= M68K_WORD_SIZE
		newSR = cpu.Read16(cpu.AddrRegs[reg])
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else {
		// Other memory sources - convert raw mode/reg to effective address
		addr := cpu.GetEffectiveAddress(mode, reg)
		newSR = cpu.Read16(addr)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Update status register
	oldSupervisor := (cpu.SR & M68K_SR_S) != 0
	newSupervisor := (newSR & M68K_SR_S) != 0
	cpu.SR = newSR
	if oldSupervisor != newSupervisor {
		cpu.swapStacksForMode(newSupervisor)
	}
}

// ExecMovec - Move Control Register
// Control registers isolate system functions from application programmer model
func (cpu *M68KCPU) ExecMovec() {
	// MOVEC is privileged - prevents user code from compromising system integrity
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	extWord := cpu.Fetch16()
	direction := (extWord >> 0) & 0x1 // 0=to control, 1=from control
	regNum := (extWord >> 12) & 0xF
	creg := extWord & 0xFFF

	// Determine if register is data or address register
	isDataReg := (regNum & 0x8) == 0
	regIndex := regNum & 0x7

	if direction == 0 {
		// Move general register to control register
		var value uint32
		if isDataReg {
			value = cpu.DataRegs[regIndex]
		} else {
			value = cpu.AddrRegs[regIndex]
		}

		// Control registers isolate system functions from application programmer model
		switch creg {
		case M68K_CR_SFC:
			cpu.SFC = uint8(value & 0x07) // Only 3 bits used
		case M68K_CR_DFC:
			cpu.DFC = uint8(value & 0x07) // Only 3 bits used
		case M68K_CR_USP:
			cpu.USP = value
		case M68K_CR_VBR:
			// Relocates exception vector table for multiple contexts
			cpu.VBR = value
		case M68K_CR_CACR:
			// Cache control - enables/disables instruction cache
			cpu.CACR = value
		case M68K_CR_CAAR:
			// Cache address for selective invalidation
			cpu.CAAR = value
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		// Move control register to general register
		var value uint32

		// Control registers isolate system functions from application programmer model
		switch creg {
		case M68K_CR_SFC:
			value = uint32(cpu.SFC)
		case M68K_CR_DFC:
			value = uint32(cpu.DFC)
		case M68K_CR_USP:
			value = cpu.USP
		case M68K_CR_VBR:
			value = cpu.VBR
		case M68K_CR_CACR:
			value = cpu.CACR
		case M68K_CR_CAAR:
			value = cpu.CAAR
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		if isDataReg {
			cpu.DataRegs[regIndex] = value
		} else {
			cpu.AddrRegs[regIndex] = value
		}
	}

	cpu.cycleCounter += M68K_CYCLE_REG * 2 // Control register access is slower
}

// ExecMoves - Move with Address Space Specification
// Uses SFC/DFC registers to specify address space
// Enables OS memory managers to access user space safely
func (cpu *M68KCPU) ExecMoves() {
	// MOVES is privileged - permits controlled supervisor data access
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	opcode := cpu.currentIR
	extWord := cpu.Fetch16()

	// Extract size from opcode bits 7-6
	sizeCode := (opcode >> 6) & 0x3
	var size int
	switch sizeCode {
	case 0:
		size = M68K_SIZE_BYTE
	case 1:
		size = M68K_SIZE_WORD
	case 2:
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Extract addressing mode and register
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7

	// Extract direction and register from extension word
	direction := (extWord >> 11) & 0x1 // 0=reg to mem, 1=mem to reg
	regNum := (extWord >> 12) & 0xF
	isDataReg := (regNum & 0x8) == 0
	regIndex := regNum & 0x7

	// Address space specification for OS memory managers
	// Uses SFC for read, DFC for write to control address space access
	if direction == 0 {
		// Move register to memory (using DFC)
		var value uint32
		if isDataReg {
			value = cpu.DataRegs[regIndex]
		} else {
			value = cpu.AddrRegs[regIndex]
		}

		// Get effective address
		addr := cpu.GetEffectiveAddress(mode, reg)

		// Write to memory using DFC address space
		// In a full implementation, DFC would select user/supervisor/program/data space
		// For now, we just perform a normal write
		switch size {
		case M68K_SIZE_BYTE:
			cpu.Write8(addr, uint8(value))
		case M68K_SIZE_WORD:
			cpu.Write16(addr, uint16(value))
		case M68K_SIZE_LONG:
			cpu.Write32(addr, value)
		}
	} else {
		// Move memory to register (using SFC)
		// Get effective address
		addr := cpu.GetEffectiveAddress(mode, reg)

		// Read from memory using SFC address space
		var value uint32
		switch size {
		case M68K_SIZE_BYTE:
			value = uint32(cpu.Read8(addr))
		case M68K_SIZE_WORD:
			value = uint32(cpu.Read16(addr))
		case M68K_SIZE_LONG:
			value = cpu.Read32(addr)
		}

		if isDataReg {
			// Sign-extend byte and word values for data registers
			switch size {
			case M68K_SIZE_BYTE:
				if (value & 0x80) != 0 {
					value |= 0xFFFFFF00
				}
			case M68K_SIZE_WORD:
				if (value & 0x8000) != 0 {
					value |= 0xFFFF0000
				}
			}
			cpu.DataRegs[regIndex] = value
		} else {
			cpu.AddrRegs[regIndex] = value
		}
	}

	cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
}

func (cpu *M68KCPU) ExecMovep() {
	opmode := (cpu.currentIR >> 6) & 0x7
	reg := (cpu.currentIR >> 9) & 0x7
	areg := cpu.currentIR & 0x7

	// MOVEP opmode encoding (bits 8-6 of opcode):
	// 100 (4) = word, memory to register (read)
	// 101 (5) = long, memory to register (read)
	// 110 (6) = word, register to memory (write)
	// 111 (7) = long, register to memory (write)
	// Bit 1 (of opmode) determines direction: 0=mem→reg (EA_TO_REG), 1=reg→mem (REG_TO_EA)
	// Bit 0 (of opmode) determines size: 0=word, 1=long
	direction := 1 - ((opmode >> 1) & 0x01) // Bit 1: 0→1(EA_TO_REG), 1→0(REG_TO_EA)
	size := (opmode & 0x01) + 1             // Bit 0: 0=word(1), 1=long(2)

	// Get displacement
	displacement := int16(cpu.Fetch16())
	addr := cpu.AddrRegs[areg] + uint32(displacement)

	if direction == M68K_DIRECTION_REG_TO_EA {
		// Register to memory
		value := cpu.DataRegs[reg]

		if size == 1 { // Word
			// Write high byte
			cpu.Write8(addr, uint8(value>>8))
			// Write low byte
			cpu.Write8(addr+M68K_WORD_SIZE, uint8(value))
		} else { // Long
			// Write bytes in order
			cpu.Write8(addr, uint8(value>>24))
			cpu.Write8(addr+M68K_WORD_SIZE, uint8(value>>16))
			cpu.Write8(addr+M68K_WORD_SIZE*2, uint8(value>>8))
			cpu.Write8(addr+M68K_WORD_SIZE*3, uint8(value))
		}
	} else {
		// Memory to register
		var value uint32

		if size == 1 { // Word
			// Read high byte
			value = uint32(cpu.Read8(addr)) << 8
			// Read low byte
			value |= uint32(cpu.Read8(addr + M68K_WORD_SIZE))
			// Update register
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | value
		} else { // Long
			// Read bytes in order
			value = uint32(cpu.Read8(addr)) << 24
			value |= uint32(cpu.Read8(addr+M68K_WORD_SIZE)) << 16
			value |= uint32(cpu.Read8(addr+M68K_WORD_SIZE*2)) << 8
			value |= uint32(cpu.Read8(addr + M68K_WORD_SIZE*3))
			// Update register
			cpu.DataRegs[reg] = value
		}
	}

	// Calculate cycle count
	if size == 1 { // Word
		cpu.cycleCounter += 16
	} else { // Long
		cpu.cycleCounter += 24
	}
}
func (cpu *M68KCPU) ExecMoveC(direction uint16) {
	// Check supervisor mode
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Get extension word
	ext := cpu.Fetch16()

	// Extract register type and number
	reg := (ext >> 12) & 0x0F
	creg := ext & 0x0FFF

	// Determine if register is data or address register
	var value uint32
	if (reg & 0x8) != 0 {
		value = cpu.AddrRegs[reg&0x07]
	} else {
		value = cpu.DataRegs[reg&0x07]
	}

	if direction == M68K_DIRECTION_REG_TO_EA {
		// Move from control register to general register
		switch creg {
		case 0x000: // SFC - Source Function Code
			// SFC not implemented in this emulator, return 0
			value = 0
		case 0x001: // DFC - Destination Function Code
			// DFC not implemented in this emulator, return 0
			value = 0
		case 0x800: // USP - User Stack Pointer
			value = cpu.USP
		case 0x801: // VBR - Vector Base Register
			// VBR not implemented in this emulator, return 0
			value = 0
		case 0x002: // CACR - Cache Control Register
			// CACR not implemented in this emulator, return 0
			value = 0
		case 0x802: // CAAR - Cache Address Register
			// CAAR not implemented in this emulator, return 0
			value = 0
		case 0x803: // MSP - Master Stack Pointer
			// MSP not implemented in this emulator, use A7 in supervisor mode
			value = cpu.AddrRegs[7]
		case 0x804: // ISP - Interrupt Stack Pointer
			// ISP not implemented in this emulator, return 0
			value = 0
		default:
			// Invalid control register
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		// Update the general register
		if (reg & 0x8) != 0 {
			cpu.AddrRegs[reg&0x07] = value
		} else {
			cpu.DataRegs[reg&0x07] = value
		}
	} else {
		// Move to control register from general register
		switch creg {
		case 0x000: // SFC - Source Function Code
			// SFC not implemented in this emulator, ignore
		case 0x001: // DFC - Destination Function Code
			// DFC not implemented in this emulator, ignore
		case 0x800: // USP - User Stack Pointer
			cpu.USP = value
		case 0x801: // VBR - Vector Base Register
			// VBR not implemented in this emulator, ignore
		case 0x002: // CACR - Cache Control Register
			// CACR not implemented in this emulator, ignore
		case 0x802: // CAAR - Cache Address Register
			// CAAR not implemented in this emulator, ignore
		case 0x803: // MSP - Master Stack Pointer
			// MSP not implemented in this emulator, ignore
		case 0x804: // ISP - Interrupt Stack Pointer
			// ISP not implemented in this emulator, ignore
		default:
			// Invalid control register
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	}

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecMoveImmToAddr(opcode uint16) {
	// Determine which variant of the instruction we're handling.
	if (opcode & 0xF0C0) == 0x207C {
		// This variant supports both word and long sizes.
		reg := (opcode >> 9) & 0x7
		size := (opcode >> 12) & 0x3

		if size == 1 { // Word variant: sign-extend the word to a long.
			value := int32(int16(cpu.Fetch16()))
			cpu.AddrRegs[reg] = uint32(value)
		} else { // Long variant: fetch two words.
			high := uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
			low := uint32(cpu.Fetch16())
			cpu.AddrRegs[reg] = high | low
		}

		cpu.cycleCounter += 12
	} else if (opcode & 0xF0F8) == 0x307C {
		// This variant is for MOVE.W only.
		reg := (opcode >> 9) & 0x7
		value := int32(int16(cpu.Fetch16()))
		cpu.AddrRegs[reg] = uint32(value)
		cpu.cycleCounter += 8
	}
}
func (cpu *M68KCPU) ExecMoveq(reg uint16, data int8) {
	// Set Dn to sign-extended 8-bit data
	cpu.DataRegs[reg] = uint32(int32(data))

	// Set condition codes
	cpu.SetFlags(uint32(int32(data)), M68K_SIZE_LONG)

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecMovem(direction, size, mode, reg uint16) {
	mask := cpu.Fetch16()

	// Determine the size for memory operations
	var operandSize uint32
	if size == 1 { // Long
		operandSize = M68K_LONG_SIZE
	} else { // Word
		operandSize = M68K_WORD_SIZE
	}

	if direction == M68K_DIRECTION_REG_TO_EA {
		// Register to memory
		// For predecrement mode, registers must be stored in reverse order (A7→A0→D7→D0)
		// and the address register must be decremented BEFORE each write, as required by
		// the M68000 architecture for stack-based operations. This ensures that multiple
		// MOVEM operations can be correctly nested (for context switching and subroutine calls).

		if mode == M68K_AM_AR_PRE {
			// Predecrement mode: stores registers in order from A7 down to D0 (highest to lowest).
			// IMPORTANT: For predecrement mode, the register list mask is in REVERSED format:
			//   - Bits 0-7 = A7-A0 (bit 0 = A7, bit 7 = A0)
			//   - Bits 8-15 = D7-D0 (bit 8 = D7, bit 15 = D0)
			// We iterate from bit 0 to bit 15, which processes registers in order A7,A6,...,A0,D7,...,D0.
			// This ensures the stack order matches what postincrement mode expects when restoring.
			for i := range 16 {
				if (mask & (1 << uint(i))) != 0 {
					// Decrement address register BEFORE writing
					cpu.AddrRegs[reg] -= operandSize
					addr := cpu.AddrRegs[reg]

					if i < 8 {
						// Bits 0-7: Address registers A7-A0 (bit 0=A7, bit 7=A0)
						regNum := 7 - i
						if size == 1 { // Long
							cpu.Write32(addr, cpu.AddrRegs[regNum])
						} else { // Word
							cpu.Write16(addr, uint16(cpu.AddrRegs[regNum]))
						}
					} else {
						// Bits 8-15: Data registers D7-D0 (bit 8=D7, bit 15=D0)
						regNum := 15 - i
						if size == 1 { // Long
							cpu.Write32(addr, cpu.DataRegs[regNum])
						} else { // Word
							cpu.Write16(addr, uint16(cpu.DataRegs[regNum]))
						}
					}
				}
			}
		} else {
			// Other modes: iterate in normal order (D0→D7→A0→A7)
			addr := cpu.GetEffectiveAddress(mode, reg)

			for i := range 16 {
				if (mask & (1 << uint(i))) != 0 {
					if i < 8 {
						// Data registers
						if size == 1 { // Long
							cpu.Write32(addr, cpu.DataRegs[i])
						} else { // Word
							cpu.Write16(addr, uint16(cpu.DataRegs[i]))
						}
					} else {
						// Address registers
						if size == 1 { // Long
							cpu.Write32(addr, cpu.AddrRegs[i-8])
						} else { // Word
							cpu.Write16(addr, uint16(cpu.AddrRegs[i-8]))
						}
					}
					addr += operandSize
				}
			}
		}
	} else {
		// Memory to register: always in normal order (D0→D7→A0→A7)
		addr := cpu.GetEffectiveAddress(mode, reg)

		for i := range 16 {
			if (mask & (1 << uint(i))) != 0 {
				if i < 8 {
					// Data registers
					if size == 1 { // Long
						cpu.DataRegs[i] = cpu.Read32(addr)
					} else { // Word
						val := int16(cpu.Read16(addr))
						cpu.DataRegs[i] = uint32(int32(val)) // Sign extend word to long
					}
				} else {
					// Address registers
					if size == 1 { // Long
						cpu.AddrRegs[i-8] = cpu.Read32(addr)
					} else { // Word
						val := int16(cpu.Read16(addr))
						cpu.AddrRegs[i-8] = uint32(int32(val)) // Sign extend word to long
					}
				}
				addr += operandSize
			}
		}

		// For postincrement mode, update the address register with the final address
		if mode == M68K_AM_AR_POST {
			cpu.AddrRegs[reg] = addr
		}
	}

	cpu.cycleCounter += M68K_CYCLE_MEM_READ + (M68K_CYCLE_REG * uint32(bits.OnesCount16(mask)))
}
func (cpu *M68KCPU) ExecLea(reg, mode, xreg uint16) {
	// Get effective address - use GetEffectiveAddress for all modes
	addr := cpu.GetEffectiveAddress(mode, xreg)

	// Store in destination register
	cpu.AddrRegs[reg] = addr

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecPea(mode, reg uint16) {
	addr := cpu.GetEffectiveAddress(mode, reg)
	cpu.Push32(addr)
	cpu.cycleCounter += M68K_CYCLE_REG + M68K_CYCLE_MEM_WRITE
}
func (cpu *M68KCPU) ExecSwap(reg uint16) {
	// SWAP exchanges the upper and lower 16-bit words of a 32-bit data register.
	// This is commonly used for byte-order conversion (endianness swapping) or
	// accessing packed data structures where word order needs to be reversed.
	upper := (cpu.DataRegs[reg] >> M68K_WORD_SIZE_BITS) & 0xFFFF
	lower := cpu.DataRegs[reg] & 0xFFFF
	cpu.DataRegs[reg] = (lower << M68K_WORD_SIZE_BITS) | upper

	// Set condition codes based on the 32-bit result after swapping.
	// N is set if bit 31 (MSB) of the result is set, indicating a negative long word.
	// Z is set if the entire 32-bit result is zero.
	// V and C are always cleared, as SWAP is a data movement operation with no
	// arithmetic overflow or carry semantics.
	result := cpu.DataRegs[reg]
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if (result & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_N
	}
	// V and C remain cleared

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecExt(reg, opmode uint16) {
	// EXT sign-extends a value from a smaller size to a larger size within a data register.
	// This is essential for signed arithmetic operations that need to promote values to larger sizes
	// whilst preserving their sign (negative values remain negative after extension).

	var sizeForFlags int

	if opmode == 2 { // EXT.W - WORD <- BYTE
		// Sign-extend byte (bit 7) to word (bits 8-15), preserving upper word
		if (cpu.DataRegs[reg] & 0x80) != 0 {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | ((cpu.DataRegs[reg] & 0xFF) | 0xFF00)
		} else {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (cpu.DataRegs[reg] & 0xFF)
		}
		sizeForFlags = M68K_SIZE_WORD // Flags reflect the resulting word
	} else if opmode == 3 { // EXT.L - LONG <- WORD
		// Sign-extend word (bit 15) to long (bits 16-31)
		if (cpu.DataRegs[reg] & 0x8000) != 0 {
			cpu.DataRegs[reg] = ((cpu.DataRegs[reg] & 0xFFFF) | 0xFFFF0000)
		} else {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF)
		}
		sizeForFlags = M68K_SIZE_LONG // Flags reflect the resulting long word
	} else if opmode == 7 { // EXTB.L - LONG <- BYTE (68020 enhancement)
		// Direct byte-to-long extension without intermediate word extension.
		// This is more efficient for 68020+ when converting bytes directly to long words.
		if (cpu.DataRegs[reg] & 0x80) != 0 {
			// Sign-extend bit 7 to full 32 bits
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFF) | 0xFFFFFF00
		} else {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFF)
		}
		sizeForFlags = M68K_SIZE_LONG // Flags reflect the resulting long word
	}

	// Set condition codes based on the result.
	// N is set if the MSB of the result size is set (indicating a negative signed value).
	// Z is set if the result is zero.
	// V and C are always cleared, as sign extension is not an arithmetic operation
	// that can overflow or carry.
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	result := cpu.DataRegs[reg]
	if sizeForFlags == M68K_SIZE_WORD {
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else { // M68K_SIZE_LONG
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
	// V and C remain cleared

	cpu.cycleCounter += M68K_CYCLE_REG
}
func (cpu *M68KCPU) ExecClr(size, mode, reg uint16) {
	if mode == M68K_AM_DR {
		// Data register direct
		if size == 0 { // Byte
			cpu.DataRegs[reg] &= 0xFFFFFF00
		} else if size == 1 { // Word
			cpu.DataRegs[reg] &= 0xFFFF0000
		} else { // Long
			cpu.DataRegs[reg] = 0
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Other addressing modes
		addr := cpu.GetEffectiveAddress(mode, reg)
		if size == 0 { // Byte
			cpu.Write8(addr, 0)
		} else if size == 1 { // Word
			cpu.Write16(addr, 0)
		} else { // Long
			cpu.Write32(addr, 0)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_V | M68K_SR_C)
	cpu.SR |= M68K_SR_Z
}
func (cpu *M68KCPU) ExecTst(size, mode, reg uint16) {
	var result uint32
	// Convert to standard size constants
	var sizeCode int
	switch size {
	case 0:
		sizeCode = M68K_SIZE_BYTE
	case 1:
		sizeCode = M68K_SIZE_WORD
	case 2:
		sizeCode = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	if mode == M68K_AM_DR {
		// Data register direct
		result = cpu.DataRegs[reg]
		if sizeCode == M68K_SIZE_BYTE {
			result &= 0xFF
		} else if sizeCode == M68K_SIZE_WORD {
			result &= 0xFFFF
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct
		result = cpu.AddrRegs[reg]
		if sizeCode == M68K_SIZE_BYTE {
			result &= 0xFF
		} else if sizeCode == M68K_SIZE_WORD {
			result &= 0xFFFF
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Other addressing modes
		addr := cpu.GetEffectiveAddress(mode, reg)

		if sizeCode == M68K_SIZE_BYTE {
			result = uint32(cpu.Read8(addr))
		} else if sizeCode == M68K_SIZE_WORD {
			result = uint32(cpu.Read16(addr))
		} else { // Long
			result = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Set condition codes using the new SetFlags function
	cpu.SetFlags(result, sizeCode)
}

// ExecScc - Set byte conditionally based on condition codes
// Boolean materialisation converts condition codes into data for computed branches
func (cpu *M68KCPU) ExecScc(condition uint8, mode, reg uint16) {
	// Evaluate condition using existing CheckCondition function
	conditionMet := cpu.CheckCondition(condition)

	var value uint8
	if conditionMet {
		value = 0xFF // All bits set for true
	} else {
		value = 0x00 // All bits clear for false
	}

	// Store result based on addressing mode
	if mode == M68K_AM_DR {
		// Data register direct - only affects low byte
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | uint32(value)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Other addressing modes
		addr := cpu.GetEffectiveAddress(mode, reg)
		cpu.Write8(addr, value)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}

	// Scc doesn't affect condition codes
}

// Arithmetic operations
func (cpu *M68KCPU) ExecAdd(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int

	// Determine operation size and direction
	direction := (opmode >> 2) & 1
	switch opmode & 3 {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on direction
	if direction == M68K_DIRECTION_REG_TO_EA {
		// Data register is destination
		if mode == M68K_AM_DR {
			// Data register direct
			source = cpu.DataRegs[xreg]
			if size == M68K_SIZE_BYTE {
				source &= 0xFF
			} else if size == M68K_SIZE_WORD {
				source &= 0xFFFF
			}
			cpu.cycleCounter += M68K_CYCLE_REG
		} else if mode == M68K_AM_AR {
			// Address register direct (word/long only - byte not allowed)
			source = cpu.AddrRegs[xreg]
			if size == M68K_SIZE_WORD {
				source &= 0xFFFF
			}
			// Note: byte size is illegal for address register but we don't enforce here
			cpu.cycleCounter += M68K_CYCLE_REG
		} else if mode == 7 {
			// Extended addressing modes
			switch xreg {
			case 0: // Absolute Short
				addr := uint32(int16(cpu.Fetch16()))
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
			case 1: // Absolute Long
				addr := cpu.Fetch32()
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
			case 2: // PC with Displacement
				disp := int16(cpu.Fetch16())
				addr := cpu.PC - M68K_WORD_SIZE + uint32(disp)
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCDI
			case 3: // PC with Index
				extWord := cpu.Fetch16()
				addr := cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCIX
			case 4: // Immediate
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Fetch16() & 0xFF)
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Fetch16())
				} else {
					source = cpu.Fetch32()
				}
				cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_EA_IM
			default:
				fmt.Printf("M68K: Unimplemented extended addressing mode %d for ADD at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
				cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
				return
			}
		} else {
			// Other addressing modes
			addr := cpu.GetEffectiveAddress(mode, xreg)
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + cpu.GetEACycles(mode, xreg)

			// Handle postincrement - increment address register AFTER read
			if mode == M68K_AM_AR_POST {
				if size == M68K_SIZE_BYTE {
					cpu.AddrRegs[xreg] += M68K_BYTE_SIZE
					if xreg == 7 { // SP always increments by 2 for bytes
						cpu.AddrRegs[xreg] += M68K_BYTE_SIZE
					}
				} else if size == M68K_SIZE_WORD {
					cpu.AddrRegs[xreg] += M68K_WORD_SIZE
				} else {
					cpu.AddrRegs[xreg] += M68K_LONG_SIZE
				}
			}
		}

		dest = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if size == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}
	} else {
		// Memory is destination - compute effective address ONCE and reuse for write
		var effectiveAddr uint32
		source = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		if mode == M68K_AM_DR {
			// Data register direct - handle separately (no memory address)
			dest = cpu.DataRegs[xreg]
			if size == M68K_SIZE_BYTE {
				dest &= 0xFF
			} else if size == M68K_SIZE_WORD {
				dest &= 0xFFFF
			}
			cpu.cycleCounter += M68K_CYCLE_REG

			// Perform addition
			result = dest + source

			// Write result back to data register
			if size == M68K_SIZE_BYTE {
				cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFFFF00) | (result & 0xFF)
			} else if size == M68K_SIZE_WORD {
				cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFF0000) | (result & 0xFFFF)
			} else {
				cpu.DataRegs[xreg] = result
			}

			// Set condition codes and return
			cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
			if size == M68K_SIZE_BYTE {
				if result > 0xFF {
					cpu.SR |= M68K_SR_C | M68K_SR_X
				}
				if ((dest & 0x80) == (source & 0x80)) && ((result & 0x80) != (dest & 0x80)) {
					cpu.SR |= M68K_SR_V
				}
				if (result & 0xFF) == 0 {
					cpu.SR |= M68K_SR_Z
				}
				if (result & 0x80) != 0 {
					cpu.SR |= M68K_SR_N
				}
			} else if size == M68K_SIZE_WORD {
				if result > 0xFFFF {
					cpu.SR |= M68K_SR_C | M68K_SR_X
				}
				if ((dest & 0x8000) == (source & 0x8000)) && ((result & 0x8000) != (dest & 0x8000)) {
					cpu.SR |= M68K_SR_V
				}
				if (result & 0xFFFF) == 0 {
					cpu.SR |= M68K_SR_Z
				}
				if (result & 0x8000) != 0 {
					cpu.SR |= M68K_SR_N
				}
			} else {
				if ((dest & 0x80000000) == (source & 0x80000000)) &&
					((result & 0x80000000) != (dest & 0x80000000)) {
					cpu.SR |= M68K_SR_V
				}
				if result < dest || result < source {
					cpu.SR |= M68K_SR_C | M68K_SR_X
				}
				if result == 0 {
					cpu.SR |= M68K_SR_Z
				}
				if (result & 0x80000000) != 0 {
					cpu.SR |= M68K_SR_N
				}
			}
			return
		} else if mode == M68K_AM_AR_IND {
			// Address register indirect
			effectiveAddr = cpu.AddrRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_POST {
			// Address register indirect with postincrement
			effectiveAddr = cpu.AddrRegs[xreg]
			if size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg] += M68K_BYTE_SIZE
				if xreg == 7 {
					cpu.AddrRegs[xreg] += M68K_BYTE_SIZE // Extra for stack pointer
				}
			} else if size == M68K_SIZE_WORD {
				cpu.AddrRegs[xreg] += M68K_WORD_SIZE
			} else {
				cpu.AddrRegs[xreg] += M68K_LONG_SIZE
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_PRE {
			// Address register indirect with predecrement
			if size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg] -= M68K_BYTE_SIZE
				if xreg == 7 {
					cpu.AddrRegs[xreg] -= M68K_BYTE_SIZE // Extra for stack pointer
				}
			} else if size == M68K_SIZE_WORD {
				cpu.AddrRegs[xreg] -= M68K_WORD_SIZE
			} else {
				cpu.AddrRegs[xreg] -= M68K_LONG_SIZE
			}
			effectiveAddr = cpu.AddrRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_DISP {
			// Address register indirect with displacement - fetch ONCE
			disp := int16(cpu.Fetch16())
			effectiveAddr = cpu.AddrRegs[xreg] + uint32(disp)
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_DI
		} else if mode == M68K_AM_AR_INDEX {
			// Address register indirect with index - fetch extension word ONCE
			extWord := cpu.Fetch16()
			effectiveAddr = cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[xreg], true)
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
		} else if mode == 7 {
			// Extended addressing modes
			switch xreg {
			case 0: // Absolute Short - fetch address ONCE
				effectiveAddr = uint32(int16(cpu.Fetch16()))
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
			case 1: // Absolute Long - fetch address ONCE
				effectiveAddr = cpu.Fetch32()
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
			default:
				fmt.Printf("M68K: Unimplemented destination mode %d for ADD at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
				cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
				return
			}
		} else {
			fmt.Printf("M68K: Unimplemented destination mode %d for ADD at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		// Read destination from memory using computed address
		if size == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(effectiveAddr))
		} else if size == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(effectiveAddr))
		} else {
			dest = cpu.Read32(effectiveAddr)
		}

		// Perform addition
		result = dest + source

		// Write result back to memory using SAME address (no re-fetch!)
		if size == M68K_SIZE_BYTE {
			cpu.Write8(effectiveAddr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(effectiveAddr, uint16(result))
		} else {
			cpu.Write32(effectiveAddr, result)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE

		// Set condition codes and return
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
		if size == M68K_SIZE_BYTE {
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x80) == (source & 0x80)) && ((result & 0x80) != (dest & 0x80)) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if size == M68K_SIZE_WORD {
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x8000) == (source & 0x8000)) && ((result & 0x8000) != (dest & 0x8000)) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			if ((dest & 0x80000000) == (source & 0x80000000)) &&
				((result & 0x80000000) != (dest & 0x80000000)) {
				cpu.SR |= M68K_SR_V
			}
			if result < dest || result < source {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}
		return
	}

	// Perform addition (for direction == M68K_DIRECTION_REG_TO_EA case)
	result = dest + source

	// Write result to data register (only reached when direction == M68K_DIRECTION_REG_TO_EA)
	if size == M68K_SIZE_BYTE {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
	} else if size == M68K_SIZE_WORD {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
	} else {
		cpu.DataRegs[reg] = result
	}
	cpu.cycleCounter += M68K_CYCLE_REG

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	// Check for carry
	if size == M68K_SIZE_BYTE {
		if result > 0xFF {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x80) == (source & 0x80)) && ((result & 0x80) != (dest & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if result > 0xFFFF {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x8000) == (source & 0x8000)) && ((result & 0x8000) != (dest & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if ((dest & 0x80000000) == (source & 0x80000000)) &&
			((result & 0x80000000) != (dest & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		// For 32-bit, carry is set if the result is less than either operand
		if result < dest || result < source {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
}

// ExecAdda - Add Address
// ADDA adds an effective address operand to an address register
// For .W (opmode 011): source is sign-extended to 32 bits before addition
// For .L (opmode 111): full 32-bit addition
// ADDA does NOT affect condition codes
func (cpu *M68KCPU) ExecAdda(reg, opmode, mode, xreg uint16) {
	var source uint32
	var isLong bool

	// Determine operation size
	if opmode == 7 {
		isLong = true
	} else {
		isLong = false // Word, source sign-extended to 32-bit
	}

	// Get source operand
	if mode == M68K_AM_DR {
		// Data register direct
		if isLong {
			source = cpu.DataRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.DataRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct
		if isLong {
			source = cpu.AddrRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.AddrRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[xreg]
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[xreg]
		if isLong {
			source = cpu.Read32(addr)
			cpu.AddrRegs[xreg] += M68K_LONG_SIZE
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
			cpu.AddrRegs[xreg] += M68K_WORD_SIZE
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		if isLong {
			cpu.AddrRegs[xreg] -= M68K_LONG_SIZE
			addr := cpu.AddrRegs[xreg]
			source = cpu.Read32(addr)
		} else {
			cpu.AddrRegs[xreg] -= M68K_WORD_SIZE
			addr := cpu.AddrRegs[xreg]
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_DISP {
		// Address register indirect with displacement
		disp := int16(cpu.Fetch16())
		addr := uint32(int32(cpu.AddrRegs[xreg]) + int32(disp))
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_DI
	} else if mode == M68K_AM_AR_INDEX {
		// Address register indirect with index
		extWord := cpu.Fetch16()
		addr := cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[xreg], false)
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
	} else if mode == 7 {
		// Extended addressing modes
		switch xreg {
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
		case 2: // PC with Displacement
			disp := int16(cpu.Fetch16())
			addr := cpu.PC - M68K_WORD_SIZE + uint32(disp)
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCDI
		case 3: // PC with Index
			extWord := cpu.Fetch16()
			addr := cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCIX
		case 4: // Immediate
			if isLong {
				source = cpu.Fetch32()
			} else {
				source = uint32(int32(int16(cpu.Fetch16())))
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH
		default:
			fmt.Printf("M68K: Unimplemented extended addressing mode %d for ADDA at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		fmt.Printf("M68K: Unimplemented source mode %d for ADDA at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Add to address register (full 32-bit, no condition code changes)
	cpu.AddrRegs[reg] += source

	cpu.cycleCounter += M68K_CYCLE_EXECUTE
}

func (cpu *M68KCPU) ExecAddq(data uint32, size uint16, mode, reg uint16) {
	var dest, result uint32
	var effectiveSize int

	// Determine operation size
	switch size {
	case 0: // Byte
		effectiveSize = M68K_SIZE_BYTE
	case 1: // Word
		effectiveSize = M68K_SIZE_WORD
	case 2: // Long
		effectiveSize = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get destination operand
	if mode == M68K_AM_DR {
		// Data register direct
		dest = cpu.DataRegs[reg]
		if effectiveSize == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if effectiveSize == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}

		// Perform addition
		result = dest + data

		// Write result
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[reg] = result
		}

		// Set condition codes (except for address registers)
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if effectiveSize == M68K_SIZE_BYTE {
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x80) != 0
			signData := (data & 0x80) != 0
			signResult := (result & 0x80) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x8000) != 0
			signData := (data & 0x8000) != 0
			signResult := (result & 0x8000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			// For long word, check carry differently
			if result < dest { // Unsigned overflow
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x80000000) != 0
			signData := (data & 0x80000000) != 0
			signResult := (result & 0x80000000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct - special case
		// ADDQ to address registers affects only the destination, not the flags
		if effectiveSize == M68K_SIZE_WORD {
			// Sign extend for word operations on address registers
			if data&0x8000 != 0 {
				data |= 0xFFFF0000
			}
		}
		cpu.AddrRegs[reg] += data
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[reg]
		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest + data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest + data
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest + data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if effectiveSize == M68K_SIZE_BYTE {
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x80) != 0
			signData := (data & 0x80) != 0
			signResult := (result & 0x80) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x8000) != 0
			signData := (data & 0x8000) != 0
			signResult := (result & 0x8000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			// For long word, check carry differently
			if result < dest { // Unsigned overflow
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			// Overflow: positive + positive = negative, or negative + negative = positive
			signDest := (dest & 0x80000000) != 0
			signData := (data & 0x80000000) != 0
			signResult := (result & 0x80000000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if mode >= M68K_AM_AR_POST && mode <= 7 {
		// Other memory addressing modes (3-7): use GetEffectiveAddress
		addr := cpu.GetEffectiveAddress(mode, reg)

		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest + data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest + data
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest + data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if effectiveSize == M68K_SIZE_BYTE {
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			signDest := (dest & 0x80) != 0
			signData := (data & 0x80) != 0
			signResult := (result & 0x80) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			signDest := (dest & 0x8000) != 0
			signData := (data & 0x8000) != 0
			signResult := (result & 0x8000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			if result < dest {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			signDest := (dest & 0x80000000) != 0
			signData := (data & 0x80000000) != 0
			signResult := (result & 0x80000000) != 0
			if (signDest == signData) && (signDest != signResult) {
				cpu.SR |= M68K_SR_V
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else {
		// Invalid mode
		fmt.Printf("M68K: Unimplemented mode %d for ADDQ at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
	}
}
func (cpu *M68KCPU) ExecAddx(regMode, rx, ry uint16, size int) {
	var src, dst, result uint32
	var mask uint32

	// Determine operation size and apply size mask
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
	case M68K_SIZE_WORD:
		mask = 0xFFFF
	case M68K_SIZE_LONG:
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on addressing mode
	if regMode == 0 {
		// Register to register
		src = cpu.DataRegs[rx] & mask
		dst = cpu.DataRegs[ry] & mask

		// Apply cycle timing
		if size == M68K_SIZE_BYTE {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_B
		} else if size == M68K_SIZE_WORD {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_W
		} else {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_L
		}
	} else {
		// Memory to memory (predecrement)
		if size == M68K_SIZE_BYTE {
			cpu.AddrRegs[rx]--
			cpu.AddrRegs[ry]--
			src = uint32(cpu.Read8(cpu.AddrRegs[rx]))
			dst = uint32(cpu.Read8(cpu.AddrRegs[ry]))
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_B
		} else if size == M68K_SIZE_WORD {
			cpu.AddrRegs[rx] -= M68K_WORD_SIZE
			cpu.AddrRegs[ry] -= M68K_WORD_SIZE
			src = uint32(cpu.Read16(cpu.AddrRegs[rx]))
			dst = uint32(cpu.Read16(cpu.AddrRegs[ry]))
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_W
		} else {
			cpu.AddrRegs[rx] -= M68K_LONG_SIZE
			cpu.AddrRegs[ry] -= M68K_LONG_SIZE
			src = cpu.Read32(cpu.AddrRegs[rx])
			dst = cpu.Read32(cpu.AddrRegs[ry])
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_L
		}
	}

	// Save old X flag before operation
	oldX := (cpu.SR & M68K_SR_X) != 0

	// Perform extended addition with X flag
	result = dst + src
	if oldX {
		result++
	}

	// Store result
	if regMode == 0 {
		// Register to register
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFFFF00) | (result & 0xFF)
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[ry] = result
		}
	} else {
		// Memory to memory
		if size == M68K_SIZE_BYTE {
			cpu.Write8(cpu.AddrRegs[ry], uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(cpu.AddrRegs[ry], uint16(result))
		} else {
			cpu.Write32(cpu.AddrRegs[ry], result)
		}
	}

	// Set flags - extended add has special Z flag behaviour
	// Z is only cleared if result is non-zero; unchanged otherwise (for multi-precision)
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	if (result & mask) != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z) // Clear Z if result non-zero
	}
	// If result is zero, Z remains unchanged

	// Check for overflow and carry
	if size == M68K_SIZE_BYTE {
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
		if ((dst & 0x80) == (src & 0x80)) && ((result & 0x80) != (dst & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		// Carry if result overflows byte range
		if result > 0xFF {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	} else if size == M68K_SIZE_WORD {
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		if ((dst & 0x8000) == (src & 0x8000)) && ((result & 0x8000) != (dst & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		// Carry if result overflows word range
		if result > 0xFFFF {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	} else {
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		if ((dst & 0x80000000) == (src & 0x80000000)) && ((result & 0x80000000) != (dst & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		// Carry if result wrapped (unsigned overflow)
		if result < dst || result < src {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	}
}

// ExecSubx - Extended subtraction with borrow for multi-precision arithmetic
func (cpu *M68KCPU) ExecSubx(regMode, rx, ry uint16, size int) {
	var src, dst, result uint32
	var mask uint32

	// Determine operation size and apply size mask
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
	case M68K_SIZE_WORD:
		mask = 0xFFFF
	case M68K_SIZE_LONG:
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on addressing mode
	if regMode == 0 {
		// Register to register
		src = cpu.DataRegs[rx] & mask
		dst = cpu.DataRegs[ry] & mask

		// Apply cycle timing
		if size == M68K_SIZE_BYTE {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_B
		} else if size == M68K_SIZE_WORD {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_W
		} else {
			cpu.cycleCounter += M68K_CYCLE_ADDX_R_L
		}
	} else {
		// Memory to memory (predecrement)
		if size == M68K_SIZE_BYTE {
			cpu.AddrRegs[rx]--
			cpu.AddrRegs[ry]--
			src = uint32(cpu.Read8(cpu.AddrRegs[rx]))
			dst = uint32(cpu.Read8(cpu.AddrRegs[ry]))
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_B
		} else if size == M68K_SIZE_WORD {
			cpu.AddrRegs[rx] -= M68K_WORD_SIZE
			cpu.AddrRegs[ry] -= M68K_WORD_SIZE
			src = uint32(cpu.Read16(cpu.AddrRegs[rx]))
			dst = uint32(cpu.Read16(cpu.AddrRegs[ry]))
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_W
		} else {
			cpu.AddrRegs[rx] -= M68K_LONG_SIZE
			cpu.AddrRegs[ry] -= M68K_LONG_SIZE
			src = cpu.Read32(cpu.AddrRegs[rx])
			dst = cpu.Read32(cpu.AddrRegs[ry])
			cpu.cycleCounter += M68K_CYCLE_ADDX_M_L
		}
	}

	// Save old X flag before operation
	oldX := (cpu.SR & M68K_SR_X) != 0

	// Perform extended subtraction with X flag (borrow)
	result = dst - src
	if oldX {
		result--
	}

	// Store result
	if regMode == 0 {
		// Register to register
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFFFF00) | (result & 0xFF)
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[ry] = result
		}
	} else {
		// Memory to memory
		if size == M68K_SIZE_BYTE {
			cpu.Write8(cpu.AddrRegs[ry], uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(cpu.AddrRegs[ry], uint16(result))
		} else {
			cpu.Write32(cpu.AddrRegs[ry], result)
		}
	}

	// Set flags - extended subtract has special Z flag behaviour
	// Z is only cleared if result is non-zero; unchanged otherwise (for multi-precision)
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	if (result & mask) != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z) // Clear Z if result non-zero
	}
	// If result is zero, Z remains unchanged

	// Check for overflow and borrow (carry)
	if size == M68K_SIZE_BYTE {
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// Overflow: positive - negative = negative, or negative - positive = positive
		if ((dst & 0x80) != (src & 0x80)) && ((result & 0x80) != (dst & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		// Borrow occurs if src > dst, or if src == dst and we had incoming borrow
		if (src&0xFF) > (dst&0xFF) || ((src&0xFF) == (dst&0xFF) && oldX) {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	} else if size == M68K_SIZE_WORD {
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// Overflow: positive - negative = negative, or negative - positive = positive
		if ((dst & 0x8000) != (src & 0x8000)) && ((result & 0x8000) != (dst & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		// Borrow occurs if src > dst, or if src == dst and we had incoming borrow
		if (src&0xFFFF) > (dst&0xFFFF) || ((src&0xFFFF) == (dst&0xFFFF) && oldX) {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	} else {
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// Overflow: positive - negative = negative, or negative - positive = positive
		if ((dst & 0x80000000) != (src & 0x80000000)) && ((result & 0x80000000) != (dst & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		// Borrow occurs if src > dst, or if src == dst and we had incoming borrow
		if src > dst || (src == dst && oldX) {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
	}
}

func (cpu *M68KCPU) ExecAddi() {
	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Get operand and perform addition
	if mode == M68K_AM_DR {
		// Data register direct
		dest := cpu.DataRegs[reg] & mask
		result := dest + imm

		// Update register
		if effSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | (result & mask)
		} else if effSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | (result & mask)
		} else {
			cpu.DataRegs[reg] = result
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if (result & mask) == 0 {
			cpu.SR |= M68K_SR_Z
		}

		if effSize == M68K_SIZE_BYTE {
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x80 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		} else if effSize == M68K_SIZE_WORD {
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x8000 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		} else {
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x80000000 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result < dest || result < imm {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		}

		cpu.cycleCounter += M68K_CYCLE_REG + (M68K_CYCLE_REG * uint32(size))
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)
		var dest, result uint32

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest + imm
			cpu.Write8(addr, uint8(result))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest + imm
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest + imm
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if (result & mask) == 0 {
			cpu.SR |= M68K_SR_Z
		}

		if effSize == M68K_SIZE_BYTE {
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x80 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result > 0xFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		} else if effSize == M68K_SIZE_WORD {
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x8000 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result > 0xFFFF {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		} else {
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest&imm & ^result)|(^dest & ^imm & result))&0x80000000 != 0 {
				cpu.SR |= M68K_SR_V
			}
			if result < dest || result < imm {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + (M68K_CYCLE_REG * uint32(size)) + cpu.GetEACycles(mode, reg)
	}
}
func (cpu *M68KCPU) ExecSub(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int

	// Determine operation size and direction
	direction := (opmode >> 2) & 1
	switch opmode & 3 {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on direction
	var destAddr uint32
	destIsMem := false

	if direction == M68K_DIRECTION_REG_TO_EA {
		// Data register is destination - source from EA
		if mode == M68K_AM_DR {
			// Data register direct
			source = cpu.DataRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_REG
		} else if mode == M68K_AM_AR {
			// Address register direct (for word/long only)
			source = cpu.AddrRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_REG
		} else if mode >= M68K_AM_AR_IND && mode <= M68K_AM_AR_INDEX {
			// Memory addressing modes 2-6
			addr := cpu.GetEffectiveAddress(mode, xreg)
			if mode == M68K_AM_AR_POST {
				cpu.AddrRegs[xreg] += uint32([]int{1, 2, 4}[opmode&3])
				if xreg == 7 && size == M68K_SIZE_BYTE {
					cpu.AddrRegs[xreg]++
				}
			}
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == 7 && xreg == 4 {
			// Immediate mode - fetch value directly from instruction stream
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Fetch16() & 0xFF)
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Fetch16())
			} else {
				source = cpu.Fetch32()
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH
		} else if mode == 7 {
			// Other extended addressing modes (abs short, abs long, pc relative)
			addr := cpu.GetEffectiveAddress(mode, xreg)
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else {
			fmt.Printf("M68K: Unimplemented source mode %d for SUB at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		dest = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if size == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}
	} else {
		// Memory is destination
		source = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		destIsMem = true
		if mode >= M68K_AM_AR_IND && mode <= M68K_AM_AR_INDEX {
			destAddr = cpu.GetEffectiveAddress(mode, xreg)
			// Handle pre-decrement (mode 4) - already handled in GetEffectiveAddress
			if size == M68K_SIZE_BYTE {
				dest = uint32(cpu.Read8(destAddr))
			} else if size == M68K_SIZE_WORD {
				dest = uint32(cpu.Read16(destAddr))
			} else {
				dest = cpu.Read32(destAddr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == 7 {
			destAddr = cpu.GetEffectiveAddress(mode, xreg)
			if size == M68K_SIZE_BYTE {
				dest = uint32(cpu.Read8(destAddr))
			} else if size == M68K_SIZE_WORD {
				dest = uint32(cpu.Read16(destAddr))
			} else {
				dest = cpu.Read32(destAddr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else {
			fmt.Printf("M68K: Unimplemented destination mode %d for SUB at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	}

	// Perform subtraction
	result = dest - source

	// Write result
	if direction == M68K_DIRECTION_REG_TO_EA {
		// Data register is destination
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[reg] = result
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if destIsMem {
		// Memory is destination - use pre-calculated destAddr
		if size == M68K_SIZE_BYTE {
			cpu.Write8(destAddr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(destAddr, uint16(result))
		} else {
			cpu.Write32(destAddr, result)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE

		// Handle post-increment for destination
		if mode == M68K_AM_AR_POST {
			cpu.AddrRegs[xreg] += uint32([]int{1, 2, 4}[opmode&3])
			if xreg == 7 && size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg]++
			}
		}
	}

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	// Check for borrow
	if size == M68K_SIZE_BYTE {
		if dest < source {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x80) != (source & 0x80)) && ((result & 0x80) == (source & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if dest < source {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x8000) != (source & 0x8000)) && ((result & 0x8000) == (source & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if dest < source {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x80000000) != (source & 0x80000000)) && ((result & 0x80000000) == (source & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
}

// ExecSuba - Subtract Address
// SUBA subtracts an effective address operand from an address register
// For .W (opmode 011): source is sign-extended to 32 bits before subtraction
// For .L (opmode 111): full 32-bit subtraction
// SUBA does NOT affect condition codes
func (cpu *M68KCPU) ExecSuba(reg, opmode, mode, xreg uint16) {
	var source uint32
	var isLong bool

	// Determine operation size
	if opmode == 7 {
		isLong = true
	} else {
		isLong = false // Word, source sign-extended to 32-bit
	}

	// Get source operand
	if mode == M68K_AM_DR {
		// Data register direct
		if isLong {
			source = cpu.DataRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.DataRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct
		if isLong {
			source = cpu.AddrRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.AddrRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[xreg]
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[xreg]
		if isLong {
			source = cpu.Read32(addr)
			cpu.AddrRegs[xreg] += M68K_LONG_SIZE
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
			cpu.AddrRegs[xreg] += M68K_WORD_SIZE
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		if isLong {
			cpu.AddrRegs[xreg] -= M68K_LONG_SIZE
			addr := cpu.AddrRegs[xreg]
			source = cpu.Read32(addr)
		} else {
			cpu.AddrRegs[xreg] -= M68K_WORD_SIZE
			addr := cpu.AddrRegs[xreg]
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == M68K_AM_AR_DISP {
		// Address register indirect with displacement
		disp := int16(cpu.Fetch16())
		addr := uint32(int32(cpu.AddrRegs[xreg]) + int32(disp))
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_DI
	} else if mode == M68K_AM_AR_INDEX {
		// Address register indirect with index
		extWord := cpu.Fetch16()
		addr := cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[xreg], false)
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
	} else if mode == 7 {
		// Extended addressing modes
		switch xreg {
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
		case 2: // PC with Displacement
			disp := int16(cpu.Fetch16())
			addr := cpu.PC - M68K_WORD_SIZE + uint32(disp)
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCDI
		case 3: // PC with Index
			extWord := cpu.Fetch16()
			addr := cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCIX
		case 4: // Immediate
			if isLong {
				source = cpu.Fetch32()
			} else {
				source = uint32(int32(int16(cpu.Fetch16())))
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH
		default:
			fmt.Printf("M68K: Unimplemented extended addressing mode %d for SUBA at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		fmt.Printf("M68K: Unimplemented source mode %d for SUBA at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Subtract from address register (full 32-bit, no condition code changes)
	cpu.AddrRegs[reg] -= source

	cpu.cycleCounter += M68K_CYCLE_EXECUTE
}

func (cpu *M68KCPU) ExecSubq(data uint32, size uint16, mode, reg uint16) {
	var dest, result uint32
	var effectiveSize int

	// Determine operation size
	switch size {
	case 0: // Byte
		effectiveSize = M68K_SIZE_BYTE
	case 1: // Word
		effectiveSize = M68K_SIZE_WORD
	case 2: // Long
		effectiveSize = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get destination operand
	if mode == M68K_AM_DR {
		// Data register direct
		dest = cpu.DataRegs[reg]
		if effectiveSize == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if effectiveSize == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}

		// Perform subtraction
		result = dest - data

		// Write result
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[reg] = result
		}

		// Set condition codes (except for address registers)
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if effectiveSize == M68K_SIZE_BYTE {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x80) != 0) && ((result & 0x80) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x8000) != 0) && ((result & 0x8000) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x80000000) != 0) && ((result & 0x80000000) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct - special case
		// SUBQ to address registers affects only the destination, not the flags
		cpu.AddrRegs[reg] -= data
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[reg]
		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest - data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest - data
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest - data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if effectiveSize == M68K_SIZE_BYTE {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x80) != 0) && ((result & 0x80) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x8000) != 0) && ((result & 0x8000) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if (result & 0xFFFF) == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		} else {
			if dest < data {
				cpu.SR |= M68K_SR_C | M68K_SR_X
			}
			if ((dest & 0x80000000) != 0) && ((result & 0x80000000) == 0) {
				cpu.SR |= M68K_SR_V
			}
			if result == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[reg]
		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest - data
			cpu.Write8(addr, uint8(result))
			cpu.AddrRegs[reg] += 1
			if reg == 7 {
				cpu.AddrRegs[reg]++ // Stack pointer always word-aligned
			}
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest - data
			cpu.Write16(addr, uint16(result))
			cpu.AddrRegs[reg] += 2
		} else {
			dest = cpu.Read32(addr)
			result = dest - data
			cpu.Write32(addr, result)
			cpu.AddrRegs[reg] += 4
		}

		// Set condition codes
		cpu.setSubqFlags(dest, data, result, effectiveSize)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		if effectiveSize == M68K_SIZE_BYTE {
			cpu.AddrRegs[reg] -= 1
			if reg == 7 {
				cpu.AddrRegs[reg]-- // Stack pointer always word-aligned
			}
			addr := cpu.AddrRegs[reg]
			dest = uint32(cpu.Read8(addr))
			result = dest - data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			cpu.AddrRegs[reg] -= 2
			addr := cpu.AddrRegs[reg]
			dest = uint32(cpu.Read16(addr))
			result = dest - data
			cpu.Write16(addr, uint16(result))
		} else {
			cpu.AddrRegs[reg] -= 4
			addr := cpu.AddrRegs[reg]
			dest = cpu.Read32(addr)
			result = dest - data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.setSubqFlags(dest, data, result, effectiveSize)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if mode == M68K_AM_AR_DISP {
		// Address register indirect with displacement
		disp := int16(cpu.Fetch16())
		addr := uint32(int32(cpu.AddrRegs[reg]) + int32(disp))

		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest - data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest - data
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest - data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.setSubqFlags(dest, data, result, effectiveSize)
		cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if mode == M68K_AM_AR_INDEX || mode == 7 {
		// Index and absolute modes - use GetEffectiveAddress
		addr := cpu.GetEffectiveAddress(mode, reg)

		if effectiveSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest - data
			cpu.Write8(addr, uint8(result))
		} else if effectiveSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest - data
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest - data
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.setSubqFlags(dest, data, result, effectiveSize)
		cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else {
		// Invalid mode
		fmt.Printf("M68K: Unimplemented mode %d for SUBQ at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
	}
}

// setSubqFlags sets condition codes for SUBQ operations
func (cpu *M68KCPU) setSubqFlags(dest, data, result uint32, size int) {
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	if size == M68K_SIZE_BYTE {
		if dest < data {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x80) != 0) && ((result & 0x80) == 0) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if dest < data {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x8000) != 0) && ((result & 0x8000) == 0) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if dest < data {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}
		if ((dest & 0x80000000) != 0) && ((result & 0x80000000) == 0) {
			cpu.SR |= M68K_SR_V
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}
}
func (cpu *M68KCPU) ExecSubi() {
	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Get operand and perform subtraction
	if mode == M68K_AM_DR {
		// Data register direct
		dest := cpu.DataRegs[reg] & mask
		result := dest - imm

		// Update register
		if effSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | (result & mask)
		} else if effSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | (result & mask)
		} else {
			cpu.DataRegs[reg] = result
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if (result & mask) == 0 {
			cpu.SR |= M68K_SR_Z
		}

		if effSize == M68K_SIZE_BYTE {
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x80) != 0 {
				cpu.SR |= M68K_SR_V
			}
		} else if effSize == M68K_SIZE_WORD {
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x8000) != 0 {
				cpu.SR |= M68K_SR_V
			}
		} else {
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_V
			}
		}

		if (imm & mask) > (dest & mask) {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}

		cpu.cycleCounter += M68K_CYCLE_REG + (M68K_CYCLE_REG * uint32(size))
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)
		var dest, result uint32

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest - imm
			cpu.Write8(addr, uint8(result))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest - imm
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest - imm
			cpu.Write32(addr, result)
		}

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)

		if (result & mask) == 0 {
			cpu.SR |= M68K_SR_Z
		}

		if effSize == M68K_SIZE_BYTE {
			if (result & 0x80) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x80) != 0 {
				cpu.SR |= M68K_SR_V
			}
		} else if effSize == M68K_SIZE_WORD {
			if (result & 0x8000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x8000) != 0 {
				cpu.SR |= M68K_SR_V
			}
		} else {
			if (result & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_N
			}
			if ((dest ^ imm) & (dest ^ result) & 0x80000000) != 0 {
				cpu.SR |= M68K_SR_V
			}
		}

		if (imm & mask) > (dest & mask) {
			cpu.SR |= M68K_SR_C | M68K_SR_X
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + (M68K_CYCLE_REG * uint32(size)) + cpu.GetEACycles(mode, reg)
	}
}
func (cpu *M68KCPU) ExecCmp(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int

	// Determine operation size
	switch opmode & 3 {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands
	dest = cpu.DataRegs[reg]
	if size == M68K_SIZE_BYTE {
		dest &= 0xFF
	} else if size == M68K_SIZE_WORD {
		dest &= 0xFFFF
	}

	if mode == M68K_AM_DR {
		// Data register direct
		source = cpu.DataRegs[xreg]
		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode >= M68K_AM_AR_IND && mode <= M68K_AM_AR_INDEX {
		// Memory addressing modes 2-6: use GetEffectiveAddress
		addr := cpu.GetEffectiveAddress(mode, xreg)

		// Handle post-increment
		if mode == M68K_AM_AR_POST {
			cpu.AddrRegs[xreg] += uint32([]int{1, 2, 4}[opmode&3])
			if xreg == 7 && size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg]++ // Keep A7 word-aligned
			}
		}

		if size == M68K_SIZE_BYTE {
			source = uint32(cpu.Read8(addr))
		} else if size == M68K_SIZE_WORD {
			source = uint32(cpu.Read16(addr))
		} else {
			source = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == 7 {
		// Extended addressing modes
		switch xreg {
		case 4: // Immediate
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Fetch16() & 0xFF)
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Fetch16())
			} else {
				source = cpu.Fetch32()
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
		default:
			fmt.Printf("M68K: Unimplemented extended addressing mode %d for CMP at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		fmt.Printf("M68K: Unimplemented source mode %d for CMP at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Perform comparison (subtraction without saving result)
	result = dest - source

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if size == M68K_SIZE_BYTE {
		if dest < source {
			cpu.SR |= M68K_SR_C
		}
		if ((dest & 0x80) != (source & 0x80)) && ((result & 0x80) == (source & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if dest < source {
			cpu.SR |= M68K_SR_C
		}
		if ((dest & 0x8000) != (source & 0x8000)) && ((result & 0x8000) == (source & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if dest < source {
			cpu.SR |= M68K_SR_C
		}
		if ((dest & 0x80000000) != (source & 0x80000000)) && ((result & 0x80000000) == (source & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}

	cpu.cycleCounter += M68K_CYCLE_EXECUTE
}

// ExecCmpa - Compare Address
// CMPA compares an address register against an effective address operand
// For .W (opmode 011): source is sign-extended to 32 bits before comparison
// For .L (opmode 111): full 32-bit comparison
func (cpu *M68KCPU) ExecCmpa(reg, opmode, mode, xreg uint16) {
	var source uint32
	var isLong bool

	// Determine operation size
	if opmode == 7 {
		isLong = true
	} else {
		isLong = false // Word, source sign-extended to 32-bit
	}

	// Get destination (always full 32-bit address register)
	dest := cpu.AddrRegs[reg]

	// Get source operand
	if mode == M68K_AM_DR {
		// Data register direct
		if isLong {
			source = cpu.DataRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.DataRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR {
		// Address register direct
		if isLong {
			source = cpu.AddrRegs[xreg]
		} else {
			// Sign extend word to long
			source = uint32(int32(int16(cpu.AddrRegs[xreg] & 0xFFFF)))
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_IND {
		// Address register indirect
		addr := cpu.AddrRegs[xreg]
		if isLong {
			source = cpu.Read32(addr)
		} else {
			source = uint32(int32(int16(cpu.Read16(addr))))
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else if mode == 7 {
		// Extended addressing modes
		switch xreg {
		case 4: // Immediate
			if isLong {
				source = cpu.Fetch32()
			} else {
				source = uint32(int32(int16(cpu.Fetch16())))
			}
			cpu.cycleCounter += M68K_CYCLE_FETCH
		case 0: // Absolute Short
			addr := uint32(int16(cpu.Fetch16()))
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
		case 1: // Absolute Long
			addr := cpu.Fetch32()
			if isLong {
				source = cpu.Read32(addr)
			} else {
				source = uint32(int32(int16(cpu.Read16(addr))))
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
		default:
			fmt.Printf("M68K: Unimplemented extended addressing mode %d for CMPA at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}
	} else {
		fmt.Printf("M68K: Unimplemented source mode %d for CMPA at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Perform comparison (subtraction without saving result) - always 32-bit
	result := dest - source

	// Set condition codes (CMPA does not affect X flag)
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	// C: borrow if source > dest (unsigned)
	if source > dest {
		cpu.SR |= M68K_SR_C
	}

	// V: overflow if operands have different signs and result sign differs from dest
	if ((dest ^ source) & (dest ^ result) & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_V
	}

	// Z: set if result is zero
	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}

	// N: set if result is negative
	if (result & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_N
	}

	cpu.cycleCounter += M68K_CYCLE_EXECUTE
}

// ExecCmpm - Memory-to-memory compare with post-increment on both operands
func (cpu *M68KCPU) ExecCmpm(rx, ry uint16, size int) {
	var src, dst, result uint32
	var incrementSize uint32

	// Read from (Ax)+ and (Ay)+
	if size == M68K_SIZE_BYTE {
		src = uint32(cpu.Read8(cpu.AddrRegs[rx]))
		dst = uint32(cpu.Read8(cpu.AddrRegs[ry]))
		// Stack pointer must always be word-aligned
		if rx == 7 {
			incrementSize = M68K_WORD_SIZE
		} else {
			incrementSize = M68K_BYTE_SIZE
		}
		cpu.AddrRegs[rx] += incrementSize
		if ry == 7 {
			incrementSize = M68K_WORD_SIZE
		} else {
			incrementSize = M68K_BYTE_SIZE
		}
		cpu.AddrRegs[ry] += incrementSize
	} else if size == M68K_SIZE_WORD {
		src = uint32(cpu.Read16(cpu.AddrRegs[rx]))
		dst = uint32(cpu.Read16(cpu.AddrRegs[ry]))
		cpu.AddrRegs[rx] += M68K_WORD_SIZE
		cpu.AddrRegs[ry] += M68K_WORD_SIZE
	} else {
		src = cpu.Read32(cpu.AddrRegs[rx])
		dst = cpu.Read32(cpu.AddrRegs[ry])
		cpu.AddrRegs[rx] += M68K_LONG_SIZE
		cpu.AddrRegs[ry] += M68K_LONG_SIZE
	}

	// Perform comparison (dst - src)
	result = dst - src

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if size == M68K_SIZE_BYTE {
		if dst < src {
			cpu.SR |= M68K_SR_C
		}
		// Overflow: positive - negative = negative, or negative - positive = positive
		if ((dst & 0x80) != (src & 0x80)) && ((result & 0x80) == (src & 0x80)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if dst < src {
			cpu.SR |= M68K_SR_C
		}
		if ((dst & 0x8000) != (src & 0x8000)) && ((result & 0x8000) == (src & 0x8000)) {
			cpu.SR |= M68K_SR_V
		}
		if (result & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if dst < src {
			cpu.SR |= M68K_SR_C
		}
		if ((dst & 0x80000000) != (src & 0x80000000)) && ((result & 0x80000000) == (src & 0x80000000)) {
			cpu.SR |= M68K_SR_V
		}
		if result == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}

	cpu.cycleCounter += M68K_CYCLE_MEM_READ*2 + M68K_CYCLE_EXECUTE
}

func (cpu *M68KCPU) ExecCmpi() {
	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Get operand and perform comparison
	var dest uint32

	if mode == M68K_AM_DR {
		// Data register direct
		dest = cpu.DataRegs[reg] & mask
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == M68K_AM_AR_POST {
		// Address register indirect with postincrement
		addr := cpu.AddrRegs[reg]
		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			// Increment by 1 for byte access
			cpu.AddrRegs[reg] += M68K_BYTE_SIZE
			// Special case for A7 (SP) - always increment by 2 for byte operations
			if reg == 7 {
				cpu.AddrRegs[reg] += M68K_BYTE_SIZE
			}
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			cpu.AddrRegs[reg] += M68K_WORD_SIZE
		} else {
			dest = cpu.Read32(addr)
			cpu.AddrRegs[reg] += M68K_LONG_SIZE
		}
		cpu.cycleCounter += M68K_CYCLE_REG + cpu.GetEACycles(mode, reg)
	} else if mode == M68K_AM_AR_PRE {
		// Address register indirect with predecrement
		var decrementSize uint32
		if effSize == M68K_SIZE_BYTE {
			decrementSize = M68K_BYTE_SIZE
			// Special case for A7 (SP) - always decrement by 2 for byte operations
			if reg == 7 {
				decrementSize = M68K_WORD_SIZE
			}
		} else if effSize == M68K_SIZE_WORD {
			decrementSize = M68K_WORD_SIZE
		} else {
			decrementSize = M68K_LONG_SIZE
		}

		// Decrement first, then read
		cpu.AddrRegs[reg] -= decrementSize
		addr := cpu.AddrRegs[reg]

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
		} else {
			dest = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_REG + cpu.GetEACycles(mode, reg)
	} else {
		// Other memory operand modes
		addr := cpu.GetEffectiveAddress(mode, reg)

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
		} else {
			dest = cpu.Read32(addr)
		}

		cpu.cycleCounter += M68K_CYCLE_REG + cpu.GetEACycles(mode, reg)
	}

	// Calculate the result (without storing it)
	var result uint32 = dest - imm

	// Set condition codes (CMP does not affect X flag)
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	// Set Z flag
	if (result & mask) == 0 {
		cpu.SR |= M68K_SR_Z
	}

	// Set N, V, C flags based on size
	if effSize == M68K_SIZE_BYTE {
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
		// V: overflow if operands have different signs and result sign differs from dest
		if ((dest ^ imm) & (dest ^ result) & 0x80) != 0 {
			cpu.SR |= M68K_SR_V
		}
		// C: borrow if imm > dest (unsigned)
		if (imm & 0xFF) > (dest & 0xFF) {
			cpu.SR |= M68K_SR_C
		}
	} else if effSize == M68K_SIZE_WORD {
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		if ((dest ^ imm) & (dest ^ result) & 0x8000) != 0 {
			cpu.SR |= M68K_SR_V
		}
		if (imm & 0xFFFF) > (dest & 0xFFFF) {
			cpu.SR |= M68K_SR_C
		}
	} else {
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
		if ((dest ^ imm) & (dest ^ result) & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_V
		}
		if imm > dest {
			cpu.SR |= M68K_SR_C
		}
	}
}
func (cpu *M68KCPU) ExecNeg(mode, reg uint16, size int) {
	var operand, result uint32
	var mask uint32

	// Apply size mask
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
	case M68K_SIZE_WORD:
		mask = 0xFFFF
	case M68K_SIZE_LONG:
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operand based on addressing mode
	if mode == M68K_AM_DR {
		// Data register direct
		operand = cpu.DataRegs[reg] & mask

		// Perform negation (0 - operand)
		result = (0 - operand) & mask

		// Store result back to register with appropriate masking
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | result
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | result
		} else {
			cpu.DataRegs[reg] = result
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)

		// Read from memory
		if size == M68K_SIZE_BYTE {
			operand = uint32(cpu.Read8(addr))
			result = (0 - operand) & mask
			cpu.Write8(addr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			operand = uint32(cpu.Read16(addr))
			result = (0 - operand) & mask
			cpu.Write16(addr, uint16(result))
		} else {
			operand = cpu.Read32(addr)
			result = (0 - operand) & mask
			cpu.Write32(addr, result)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + cpu.GetEACycles(mode, reg)
	}

	// Set N and Z flags based on result
	cpu.SetFlagsNZ(result, size)

	// Clear V and C flags (SetFlagsNZ doesn't touch these)
	cpu.SR &= ^uint16(M68K_SR_V | M68K_SR_C | M68K_SR_X)

	// V flag - set if operand was 0x80 (byte), 0x8000 (word), or 0x80000000 (long)
	if size == M68K_SIZE_BYTE && operand == 0x80 {
		cpu.SR |= M68K_SR_V
	} else if size == M68K_SIZE_WORD && operand == 0x8000 {
		cpu.SR |= M68K_SR_V
	} else if size == M68K_SIZE_LONG && operand == 0x80000000 {
		cpu.SR |= M68K_SR_V
	}

	// C and X flags - set if operand was not zero
	if operand != 0 {
		cpu.SR |= M68K_SR_C | M68K_SR_X
	}
}
func (cpu *M68KCPU) ExecNegx(mode, reg uint16, size int) {
	var operand, result uint32
	var mask uint32

	// Apply size mask
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
	case M68K_SIZE_WORD:
		mask = 0xFFFF
	case M68K_SIZE_LONG:
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operand based on addressing mode
	if mode == M68K_AM_DR {
		// Data register direct
		operand = cpu.DataRegs[reg] & mask

		// Perform extended negation (0 - operand - X)
		result = 0 - operand
		if (cpu.SR & M68K_SR_X) != 0 {
			result--
		}
		result &= mask

		// Store result back to register with appropriate masking
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | result
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | result
		} else {
			cpu.DataRegs[reg] = result
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)

		// Read from memory
		if size == M68K_SIZE_BYTE {
			operand = uint32(cpu.Read8(addr))
			result = 0 - operand
			if (cpu.SR & M68K_SR_X) != 0 {
				result--
			}
			result &= mask
			cpu.Write8(addr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			operand = uint32(cpu.Read16(addr))
			result = 0 - operand
			if (cpu.SR & M68K_SR_X) != 0 {
				result--
			}
			result &= mask
			cpu.Write16(addr, uint16(result))
		} else {
			operand = cpu.Read32(addr)
			result = 0 - operand
			if (cpu.SR & M68K_SR_X) != 0 {
				result--
			}
			result &= mask
			cpu.Write32(addr, result)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + cpu.GetEACycles(mode, reg)
	}

	// Clear N, V, C, X flags but NOT Z
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_V | M68K_SR_C | M68K_SR_X)

	// Z flag - cleared if result is non-zero, unchanged otherwise
	if (result & mask) != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}

	// N flag - set based on result MSB
	if size == M68K_SIZE_BYTE {
		if (result & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else if size == M68K_SIZE_WORD {
		if (result & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	} else {
		if (result & 0x80000000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}

	// V flag - set if operand was 0x80 (byte), 0x8000 (word), or 0x80000000 (long)
	if size == M68K_SIZE_BYTE && operand == 0x80 && (cpu.SR&M68K_SR_X) != 0 {
		cpu.SR |= M68K_SR_V
	} else if size == M68K_SIZE_WORD && operand == 0x8000 && (cpu.SR&M68K_SR_X) != 0 {
		cpu.SR |= M68K_SR_V
	} else if size == M68K_SIZE_LONG && operand == 0x80000000 && (cpu.SR&M68K_SR_X) != 0 {
		cpu.SR |= M68K_SR_V
	}

	// C and X flags - set if operand was not zero or if X was set
	if operand != 0 || (cpu.SR&M68K_SR_X) != 0 {
		cpu.SR |= M68K_SR_C | M68K_SR_X
	}
}

// Multiplication and division operations
func (cpu *M68KCPU) ExecMulu(reg, mode, xreg uint16) {
	var source uint32

	// Get source operand
	if mode == M68K_AM_DR {
		source = cpu.DataRegs[xreg] & 0xFFFF
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && xreg == 4 {
		// Immediate mode - fetch the value directly
		source = uint32(cpu.Fetch16())
		cpu.cycleCounter += M68K_CYCLE_FETCH
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		source = uint32(cpu.Read16(addr))
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Get destination operand
	dest := cpu.DataRegs[reg] & 0xFFFF

	// Perform multiplication
	result := dest * source

	// Update register
	cpu.DataRegs[reg] = result

	// Set condition codes
	cpu.SetFlagsNZ(result, M68K_SIZE_LONG)

	// Calculate proper cycle count
	var baseCycles uint32 = M68K_CYCLE_MULU_W

	// Add effective address calculation time if needed
	if mode != M68K_AM_DR {
		baseCycles += cpu.GetEACycles(mode, xreg)
	}

	// Use worst case for the 68000-era timing
	cpu.cycleCounter += baseCycles
}
func (cpu *M68KCPU) ExecMuls(reg, mode, xreg uint16) {
	var source uint32

	// Get source operand
	if mode == M68K_AM_DR {
		source = cpu.DataRegs[xreg] & 0xFFFF
		// Sign extend
		if (source & 0x8000) != 0 {
			source |= 0xFFFF0000
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && xreg == 4 {
		// Immediate mode - fetch the value directly and sign extend
		source = uint32(int32(int16(cpu.Fetch16())))
		cpu.cycleCounter += M68K_CYCLE_FETCH
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		source = uint32(int32(int16(cpu.Read16(addr))))
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Get destination operand and sign extend
	dest := cpu.DataRegs[reg] & 0xFFFF
	if (dest & 0x8000) != 0 {
		dest |= 0xFFFF0000
	}

	// Perform signed multiplication
	result := uint32(int32(dest) * int32(source))

	// Update register
	cpu.DataRegs[reg] = result

	// Set condition codes
	cpu.SetFlagsNZ(result, M68K_SIZE_LONG)

	// Calculate proper cycle count
	var baseCycles uint32 = M68K_CYCLE_MULS_W

	// Add effective address calculation time if needed
	if mode != M68K_AM_DR {
		baseCycles += cpu.GetEACycles(mode, xreg)
	}

	cpu.cycleCounter += baseCycles
}
func (cpu *M68KCPU) ExecMulL(reg, mode, xreg uint16) {
	// Get extension word
	extWord := cpu.Fetch16()

	// Determine if signed or unsigned
	signed := (extWord & 0x0800) != 0

	// Check if 64-bit result
	resultHigh := (extWord & 0x0400) != 0

	// Get registers
	dlReg := reg
	dhReg := (extWord >> 12) & 0x7

	// Get source operand
	var source uint32
	if mode == M68K_AM_DR {
		source = cpu.DataRegs[xreg]
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		source = cpu.Read32(addr)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Perform multiplication
	var resultLow, resultHi uint32
	var result64 uint64
	var signed64 int64

	if signed {
		// Signed multiplication
		signed64 = int64(int32(cpu.DataRegs[dlReg])) * int64(int32(source))
		resultLow = uint32(signed64 & 0xFFFFFFFF)
		resultHi = uint32((signed64 >> 32) & 0xFFFFFFFF)
		result64 = uint64(signed64)

		// Use appropriate cycle timing
		cpu.cycleCounter += M68K_CYCLE_MULS_L
	} else {
		// Unsigned multiplication
		result64 = uint64(cpu.DataRegs[dlReg]) * uint64(source)
		resultLow = uint32(result64 & 0xFFFFFFFF)
		resultHi = uint32((result64 >> 32) & 0xFFFFFFFF)

		// Use appropriate cycle timing
		cpu.cycleCounter += M68K_CYCLE_MULU_L
	}

	// Store result(s)
	if resultHigh {
		// Store 64-bit result
		cpu.DataRegs[dhReg] = resultHi
		cpu.DataRegs[dlReg] = resultLow
	} else {
		// Store 32-bit result
		cpu.DataRegs[dlReg] = resultLow
	}

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	// Z flag - all bits of result must be zero
	if result64 == 0 {
		cpu.SR |= M68K_SR_Z
	}

	// N flag - based on MSB of result
	if (resultLow & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_N
	}

	// V flag - set if high part doesn't match sign extension of low part
	if !resultHigh {
		if signed {
			// High 32 bits should all be sign bits of low 32 bits
			if (resultLow & 0x80000000) != 0 {
				// If negative, all high bits should be 1s
				if resultHi != 0xFFFFFFFF {
					cpu.SR |= M68K_SR_V
				}
			} else {
				// If positive, all high bits should be 0s
				if resultHi != 0 {
					cpu.SR |= M68K_SR_V
				}
			}
		} else {
			// For unsigned, overflow if any high bits set
			if resultHi != 0 {
				cpu.SR |= M68K_SR_V
			}
		}
	}

	// Add EA calculation time if needed
	if mode != M68K_AM_DR {
		cpu.cycleCounter += cpu.GetEACycles(mode, xreg)
	}
}
func (cpu *M68KCPU) ExecDivu(reg, mode, xreg uint16) {
	var source uint32

	// Get source operand
	if mode == M68K_AM_DR {
		source = cpu.DataRegs[xreg] & 0xFFFF
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && xreg == 4 {
		// Immediate mode - fetch directly from instruction stream
		source = uint32(cpu.Fetch16())
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		source = uint32(cpu.Read16(addr))
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Check for division by zero
	if source == 0 {
		cpu.ProcessException(M68K_VEC_ZERO_DIVIDE)
		return
	}

	// Get destination operand
	dest := cpu.DataRegs[reg]

	// Check for overflow
	quotient := dest / source
	if quotient > 0xFFFF {
		// Set overflow flag and leave register unchanged
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_C)
		cpu.SR |= M68K_SR_V
	} else {
		remainder := dest % source

		// Update register with quotient in lower word, remainder in upper word
		cpu.DataRegs[reg] = (remainder << M68K_WORD_SIZE_BITS) | (quotient & 0xFFFF)

		// Set condition codes
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

		if (quotient & 0xFFFF) == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (quotient & 0x8000) != 0 {
			cpu.SR |= M68K_SR_N
		}
	}

	// Calculate proper cycle count for DIVU
	var baseCycles uint32 = M68K_CYCLE_DIVU_W

	// Add EA calculation time if needed
	if mode != M68K_AM_DR {
		baseCycles += cpu.GetEACycles(mode, xreg)
	}

	// Use best case timing for 68020
	cpu.cycleCounter += baseCycles
}
func (cpu *M68KCPU) ExecDivs(reg, mode, xreg uint16) {
	var source int32

	// Get source operand and sign extend
	if mode == M68K_AM_DR {
		sourceU := cpu.DataRegs[xreg] & 0xFFFF
		if (sourceU & 0x8000) != 0 {
			source = int32(sourceU | 0xFFFF0000)
		} else {
			source = int32(sourceU)
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && xreg == 4 {
		// Immediate mode - fetch directly from instruction stream
		source = int32(int16(cpu.Fetch16()))
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		source = int32(int16(cpu.Read16(addr)))
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Check for division by zero
	if source == 0 {
		cpu.ProcessException(M68K_VEC_ZERO_DIVIDE)
		return
	}

	// Get destination operand
	dest := int32(cpu.DataRegs[reg])

	// Check for overflow (-2^31 / -1)
	// This special case would overflow in two's complement
	if dest == -0x80000000 && source == -1 {
		// Set overflow flag and leave register unchanged
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_C)
		cpu.SR |= M68K_SR_V
	} else {
		quotient := dest / source

		// Check if quotient fits in 16 bits
		if quotient < -32768 || quotient > 32767 {
			// Set overflow flag and leave register unchanged
			cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_C)
			cpu.SR |= M68K_SR_V
		} else {
			remainder := dest % source

			// Update register with quotient in lower word, remainder in upper word
			// Note: remainder must be sign-extended to 16 bits properly
			cpu.DataRegs[reg] = (uint32(remainder) << M68K_WORD_SIZE_BITS) | uint32(quotient&0xFFFF)

			// Set condition codes
			cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

			if quotient == 0 {
				cpu.SR |= M68K_SR_Z
			}
			if quotient < 0 {
				cpu.SR |= M68K_SR_N
			}
		}
	}

	// Calculate proper cycle count for DIVS
	var baseCycles uint32 = M68K_CYCLE_DIVS_W

	// Add EA calculation time if needed
	if mode != M68K_AM_DR {
		baseCycles += cpu.GetEACycles(mode, xreg)
	}

	cpu.cycleCounter += baseCycles
}
func (cpu *M68KCPU) ExecDIVL(reg, mode, xreg uint16) {
	// Get extension word
	extWord := cpu.Fetch16()

	// Determine if signed or unsigned
	signed := (extWord & 0x0800) != 0

	// Check if 64-bit dividend
	longDiv := (extWord & 0x0400) != 0

	// Get registers
	dqReg := reg
	drReg := (extWord >> 12) & 0x7

	// Get divisor operand
	var divisor uint32
	if mode == M68K_AM_DR {
		divisor = cpu.DataRegs[xreg]
		cpu.cycleCounter += M68K_CYCLE_REG
	} else if mode == 7 && xreg == 4 {
		// Immediate mode - fetch directly from instruction stream
		divisor = cpu.Fetch32()
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		divisor = cpu.Read32(addr)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Check for division by zero
	if divisor == 0 {
		cpu.ProcessException(M68K_VEC_ZERO_DIVIDE)
		return
	}

	var quotient, remainder uint32
	var overflow bool = false

	if longDiv {
		// 64-bit dividend
		dividendHi := cpu.DataRegs[drReg]
		dividendLo := cpu.DataRegs[dqReg]

		if signed {
			// Signed division
			var dividendS int64 = (int64(int32(dividendHi)) << 32) | int64(uint32(dividendLo))
			var divisorS int32 = int32(divisor)

			// Check for overflow
			// Special case: most negative dividend / -1
			if dividendS == -0x8000000000000000 && divisorS == -1 {
				overflow = true
			} else if divisorS != 0 {
				var quotientS int64 = dividendS / int64(divisorS)

				// Check if quotient fits in 32 bits
				if quotientS < -0x80000000 || quotientS > 0x7FFFFFFF {
					overflow = true
				} else {
					quotient = uint32(quotientS)
					remainder = uint32(dividendS % int64(divisorS))
				}
			}

			// Use appropriate cycle timing
			cpu.cycleCounter += M68K_CYCLE_DIVS_L
		} else {
			// Unsigned division
			var dividend uint64 = (uint64(dividendHi) << 32) | uint64(dividendLo)
			var divisorU uint32 = divisor

			// Check for overflow
			if divisorU != 0 {
				var quotientU uint64 = dividend / uint64(divisorU)

				// Check if quotient fits in 32 bits
				if quotientU > 0xFFFFFFFF {
					overflow = true
				} else {
					quotient = uint32(quotientU)
					remainder = uint32(dividend % uint64(divisorU))
				}
			}

			// Use appropriate cycle timing
			cpu.cycleCounter += M68K_CYCLE_DIVU_L
		}
	} else {
		// 32-bit dividend
		dividend := cpu.DataRegs[dqReg]

		if signed {
			// Signed division
			var dividendS int32 = int32(dividend)
			var divisorS int32 = int32(divisor)

			// Check for overflow
			// Special case: most negative number / -1
			if dividendS == -0x80000000 && divisorS == -1 {
				overflow = true
			} else if divisorS != 0 {
				quotient = uint32(dividendS / divisorS)
				remainder = uint32(dividendS % divisorS)
			}

			// Use appropriate cycle timing
			cpu.cycleCounter += M68K_CYCLE_DIVS_L
		} else {
			// Unsigned division
			if divisor != 0 {
				quotient = dividend / divisor
				remainder = dividend % divisor
			}

			// Use appropriate cycle timing
			cpu.cycleCounter += M68K_CYCLE_DIVU_L
		}
	}

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if overflow {
		cpu.SR |= M68K_SR_V
	} else {
		if quotient == 0 {
			cpu.SR |= M68K_SR_Z
		}

		if signed && ((quotient & 0x80000000) != 0) {
			cpu.SR |= M68K_SR_N
		}

		// Store results
		cpu.DataRegs[dqReg] = quotient
		cpu.DataRegs[drReg] = remainder
	}

	// Add EA calculation time if needed
	if mode != M68K_AM_DR {
		cpu.cycleCounter += cpu.GetEACycles(mode, xreg)
	}
}

// Logical operations
func (cpu *M68KCPU) ExecAnd(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int

	// Determine operation size and direction
	direction := (opmode >> 2) & 1
	switch opmode & 3 {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on direction
	if direction == M68K_DIRECTION_REG_TO_EA {
		// Data register is destination
		if mode == M68K_AM_DR {
			// Data register direct
			source = cpu.DataRegs[xreg]
			if size == M68K_SIZE_BYTE {
				source &= 0xFF
			} else if size == M68K_SIZE_WORD {
				source &= 0xFFFF
			}
			cpu.cycleCounter += M68K_CYCLE_REG
		} else if mode == 7 {
			// Extended addressing modes
			switch xreg {
			case 0: // Absolute Short
				addr := uint32(int16(cpu.Fetch16()))
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
			case 1: // Absolute Long
				addr := cpu.Fetch32()
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
			case 2: // PC with Displacement
				disp := int16(cpu.Fetch16())
				addr := cpu.PC - M68K_WORD_SIZE + uint32(disp)
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCDI
			case 3: // PC with Index
				extWord := cpu.Fetch16()
				addr := cpu.GetIndexWithExtWords(extWord, cpu.PC-M68K_WORD_SIZE, false)
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Read8(addr))
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Read16(addr))
				} else {
					source = cpu.Read32(addr)
				}
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_PCIX
			case 4: // Immediate
				if size == M68K_SIZE_BYTE {
					source = uint32(cpu.Fetch16() & 0xFF)
				} else if size == M68K_SIZE_WORD {
					source = uint32(cpu.Fetch16())
				} else {
					source = cpu.Fetch32()
				}
				cpu.cycleCounter += M68K_CYCLE_FETCH + M68K_CYCLE_EA_IM
			default:
				fmt.Printf("M68K: Unimplemented extended addressing mode %d for AND at PC=%08x\n", xreg, cpu.PC-M68K_WORD_SIZE)
				cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
				return
			}
		} else {
			// Other addressing modes
			addr := cpu.GetEffectiveAddress(mode, xreg)
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + cpu.GetEACycles(mode, xreg)
		}

		dest = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if size == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}
	} else {
		// Memory is destination - compute effective address ONCE and reuse for write
		var effectiveAddr uint32
		source = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		if mode == M68K_AM_DR {
			// Data register direct - handle separately (no memory address)
			dest = cpu.DataRegs[xreg]
			if size == M68K_SIZE_BYTE {
				dest &= 0xFF
			} else if size == M68K_SIZE_WORD {
				dest &= 0xFFFF
			}
			cpu.cycleCounter += M68K_CYCLE_REG

			// Perform AND and write back to register
			result = dest & source
			if size == M68K_SIZE_BYTE {
				cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFFFF00) | (result & 0xFF)
			} else if size == M68K_SIZE_WORD {
				cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFF0000) | (result & 0xFFFF)
			} else {
				cpu.DataRegs[xreg] = result
			}
			cpu.cycleCounter += M68K_CYCLE_REG
			cpu.SetFlagsNZ(result, size)
			return
		} else if mode == M68K_AM_AR_IND {
			effectiveAddr = cpu.AddrRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_POST {
			effectiveAddr = cpu.AddrRegs[xreg]
			if size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg] += M68K_BYTE_SIZE
				if xreg == 7 {
					cpu.AddrRegs[xreg] += M68K_BYTE_SIZE
				}
			} else if size == M68K_SIZE_WORD {
				cpu.AddrRegs[xreg] += M68K_WORD_SIZE
			} else {
				cpu.AddrRegs[xreg] += M68K_LONG_SIZE
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_PRE {
			if size == M68K_SIZE_BYTE {
				cpu.AddrRegs[xreg] -= M68K_BYTE_SIZE
				if xreg == 7 {
					cpu.AddrRegs[xreg] -= M68K_BYTE_SIZE
				}
			} else if size == M68K_SIZE_WORD {
				cpu.AddrRegs[xreg] -= M68K_WORD_SIZE
			} else {
				cpu.AddrRegs[xreg] -= M68K_LONG_SIZE
			}
			effectiveAddr = cpu.AddrRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		} else if mode == M68K_AM_AR_DISP {
			disp := int16(cpu.Fetch16())
			effectiveAddr = cpu.AddrRegs[xreg] + uint32(disp)
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_DI
		} else if mode == M68K_AM_AR_INDEX {
			extWord := cpu.Fetch16()
			effectiveAddr = cpu.GetIndexWithExtWords(extWord, cpu.AddrRegs[xreg], true)
			cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_IX
		} else if mode == 7 {
			switch xreg {
			case 0: // Absolute Short
				effectiveAddr = uint32(int16(cpu.Fetch16()))
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AW
			case 1: // Absolute Long
				effectiveAddr = cpu.Fetch32()
				cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_EA_AL
			default:
				fmt.Printf("M68K: Unimplemented destination mode %d,%d for AND at PC=%08x\n", mode, xreg, cpu.PC-M68K_WORD_SIZE)
				cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
				return
			}
		} else {
			fmt.Printf("M68K: Unimplemented destination mode %d for AND at PC=%08x\n", mode, cpu.PC-M68K_WORD_SIZE)
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return
		}

		// Read destination from memory using computed address
		if size == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(effectiveAddr))
		} else if size == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(effectiveAddr))
		} else {
			dest = cpu.Read32(effectiveAddr)
		}

		// Perform AND operation
		result = dest & source

		// Write result back to memory using SAME address (no re-fetch!)
		if size == M68K_SIZE_BYTE {
			cpu.Write8(effectiveAddr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(effectiveAddr, uint16(result))
		} else {
			cpu.Write32(effectiveAddr, result)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE

		cpu.SetFlagsNZ(result, size)
		return
	}

	// Perform AND operation (for direction == M68K_DIRECTION_REG_TO_EA case)
	result = dest & source

	// Write result to data register
	if size == M68K_SIZE_BYTE {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
	} else if size == M68K_SIZE_WORD {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
	} else {
		cpu.DataRegs[reg] = result
	}
	cpu.cycleCounter += M68K_CYCLE_REG

	// Set condition codes
	cpu.SetFlagsNZ(result, size)
}
func (cpu *M68KCPU) ExecAndi() {
	// Check for special CCR/SR variants first by full instruction word
	// ANDI to CCR: 0000 0010 0011 1100 = 0x023C
	// ANDI to SR:  0000 0010 0111 1100 = 0x027C
	if cpu.currentIR == 0x023C {
		cpu.ExecAndiCcr()
		return
	}
	if cpu.currentIR == 0x027C {
		cpu.ExecAndiSr()
		return
	}

	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Track result for flag setting
	var actualResult uint32

	// Get operand and perform AND
	if mode == M68K_AM_DR {
		// Data register direct
		dest := cpu.DataRegs[reg] & mask
		actualResult = dest & imm

		// Update register
		if effSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else if effSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else {
			cpu.DataRegs[reg] = actualResult
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)
		var dest uint32

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			actualResult = dest & imm
			cpu.Write8(addr, uint8(actualResult))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			actualResult = dest & imm
			cpu.Write16(addr, uint16(actualResult))
		} else {
			dest = cpu.Read32(addr)
			actualResult = dest & imm
			cpu.Write32(addr, actualResult)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes based on actual result
	cpu.SetFlagsNZ(actualResult, effSize)
}
func (cpu *M68KCPU) ExecOr(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int
	var destAddr uint32
	destIsMem := false

	// Determine operation size and direction
	direction := (opmode >> 2) & 1
	switch opmode & 3 {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operands based on direction
	if direction == M68K_DIRECTION_REG_TO_EA {
		// EA is source, data register is destination
		if mode == M68K_AM_DR {
			// Data register direct
			source = cpu.DataRegs[xreg]
			cpu.cycleCounter += M68K_CYCLE_REG
		} else {
			// Memory source - use GetEffectiveAddress
			addr := cpu.GetEffectiveAddress(mode, xreg)
			if size == M68K_SIZE_BYTE {
				source = uint32(cpu.Read8(addr))
			} else if size == M68K_SIZE_WORD {
				source = uint32(cpu.Read16(addr))
			} else {
				source = cpu.Read32(addr)
			}
			cpu.cycleCounter += M68K_CYCLE_MEM_READ
		}

		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		dest = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if size == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}
	} else {
		// Data register is source, EA is destination (memory)
		destIsMem = true
		source = cpu.DataRegs[reg]
		if size == M68K_SIZE_BYTE {
			source &= 0xFF
		} else if size == M68K_SIZE_WORD {
			source &= 0xFFFF
		}

		// Memory destination - use GetEffectiveAddress
		destAddr = cpu.GetEffectiveAddress(mode, xreg)
		if size == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(destAddr))
		} else if size == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(destAddr))
		} else {
			dest = cpu.Read32(destAddr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Perform OR operation
	result = dest | source

	// Write result
	if !destIsMem {
		// Data register is destination
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[reg] = result
		}
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory is destination - use stored destAddr
		if size == M68K_SIZE_BYTE {
			cpu.Write8(destAddr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			cpu.Write16(destAddr, uint16(result))
		} else {
			cpu.Write32(destAddr, result)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes
	cpu.SetFlagsNZ(result, size)
}
func (cpu *M68KCPU) ExecOri() {
	// Check for special CCR/SR variants first by full instruction word
	// ORI to CCR: 0000 0000 0011 1100 = 0x003C
	// ORI to SR:  0000 0000 0111 1100 = 0x007C
	if cpu.currentIR == 0x003C {
		cpu.ExecOriCcr()
		return
	}
	if cpu.currentIR == 0x007C {
		cpu.ExecOriSr()
		return
	}

	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Track result for flag setting
	var actualResult uint32

	// Get operand and perform OR
	if mode == M68K_AM_DR {
		// Data register direct
		dest := cpu.DataRegs[reg] & mask
		actualResult = dest | imm

		// Update register
		if effSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else if effSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else {
			cpu.DataRegs[reg] = actualResult
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)
		var dest uint32

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			actualResult = dest | imm
			cpu.Write8(addr, uint8(actualResult))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			actualResult = dest | imm
			cpu.Write16(addr, uint16(actualResult))
		} else {
			dest = cpu.Read32(addr)
			actualResult = dest | imm
			cpu.Write32(addr, actualResult)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes based on actual result
	cpu.SetFlagsNZ(actualResult, effSize)
}
func (cpu *M68KCPU) ExecEor(reg, opmode, mode, xreg uint16) {
	var source, dest, result uint32
	var size int

	// Determine operation size
	switch opmode {
	case 0: // Byte
		size = M68K_SIZE_BYTE
	case 1: // Word
		size = M68K_SIZE_WORD
	case 2: // Long
		size = M68K_SIZE_LONG
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get source operand (always from data register)
	source = cpu.DataRegs[reg]
	if size == M68K_SIZE_BYTE {
		source &= 0xFF
	} else if size == M68K_SIZE_WORD {
		source &= 0xFFFF
	}

	// Get destination operand and perform EOR
	if mode == M68K_AM_DR {
		// Data register direct
		dest = cpu.DataRegs[xreg]
		if size == M68K_SIZE_BYTE {
			dest &= 0xFF
		} else if size == M68K_SIZE_WORD {
			dest &= 0xFFFF
		}

		result = dest ^ source

		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFFFF00) | (result & 0xFF)
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[xreg] = (cpu.DataRegs[xreg] & 0xFFFF0000) | (result & 0xFFFF)
		} else {
			cpu.DataRegs[xreg] = result
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, xreg)

		if size == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			result = dest ^ source
			cpu.Write8(addr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			result = dest ^ source
			cpu.Write16(addr, uint16(result))
		} else {
			dest = cpu.Read32(addr)
			result = dest ^ source
			cpu.Write32(addr, result)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes
	cpu.SetFlagsNZ(result, size)
}
func (cpu *M68KCPU) ExecEori() {
	// Check for special CCR/SR variants first by full instruction word
	// EORI to CCR: 0000 1010 0011 1100 = 0x0A3C
	// EORI to SR:  0000 1010 0111 1100 = 0x0A7C
	if cpu.currentIR == 0x0A3C {
		cpu.ExecEoriCcr()
		return
	}
	if cpu.currentIR == 0x0A7C {
		cpu.ExecEoriSr()
		return
	}

	size := (cpu.currentIR >> 6) & 0x3
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7

	var effSize int
	var mask uint32

	// Determine operation size
	switch size {
	case 0: // Byte
		effSize = M68K_SIZE_BYTE
		mask = 0xFF
	case 1: // Word
		effSize = M68K_SIZE_WORD
		mask = 0xFFFF
	case 2: // Long
		effSize = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Fetch immediate value
	var imm uint32
	if effSize == M68K_SIZE_BYTE {
		imm = uint32(cpu.Fetch16() & 0xFF)
	} else if effSize == M68K_SIZE_WORD {
		imm = uint32(cpu.Fetch16())
	} else {
		imm = uint32(cpu.Fetch16()) << M68K_WORD_SIZE_BITS
		imm |= uint32(cpu.Fetch16())
	}

	// Track result for flag setting
	var actualResult uint32

	// Get operand and perform EOR
	if mode == M68K_AM_DR {
		// Data register direct
		dest := cpu.DataRegs[reg] & mask
		actualResult = dest ^ imm

		// Update register
		if effSize == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else if effSize == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & ^mask) | actualResult
		} else {
			cpu.DataRegs[reg] = actualResult
		}

		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)
		var dest uint32

		if effSize == M68K_SIZE_BYTE {
			dest = uint32(cpu.Read8(addr))
			actualResult = dest ^ imm
			cpu.Write8(addr, uint8(actualResult))
		} else if effSize == M68K_SIZE_WORD {
			dest = uint32(cpu.Read16(addr))
			actualResult = dest ^ imm
			cpu.Write16(addr, uint16(actualResult))
		} else {
			dest = cpu.Read32(addr)
			actualResult = dest ^ imm
			cpu.Write32(addr, actualResult)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	}

	// Set condition codes based on actual result
	cpu.SetFlagsNZ(actualResult, effSize)
}
func (cpu *M68KCPU) ExecNot(mode, reg uint16, size int) {
	var operand, result uint32
	var mask uint32

	// Apply size mask
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
	case M68K_SIZE_WORD:
		mask = 0xFFFF
	case M68K_SIZE_LONG:
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get operand based on addressing mode
	if mode == M68K_AM_DR {
		// Data register direct
		operand = cpu.DataRegs[reg] & mask

		// Perform logical NOT (one's complement)
		result = ^operand & mask

		// Store result back to register with appropriate masking
		if size == M68K_SIZE_BYTE {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | result
		} else if size == M68K_SIZE_WORD {
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | result
		} else {
			cpu.DataRegs[reg] = result
		}

		cpu.cycleCounter += M68K_CYCLE_REG // Base cycle count for register
	} else {
		// Memory operand
		addr := cpu.GetEffectiveAddress(mode, reg)

		// Read from memory
		if size == M68K_SIZE_BYTE {
			operand = uint32(cpu.Read8(addr))
			result = ^operand & mask
			cpu.Write8(addr, uint8(result))
		} else if size == M68K_SIZE_WORD {
			operand = uint32(cpu.Read16(addr))
			result = ^operand & mask
			cpu.Write16(addr, uint16(result))
		} else {
			operand = cpu.Read32(addr)
			result = ^operand & mask
			cpu.Write32(addr, result)
		}

		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + cpu.GetEACycles(mode, reg)
	}

	// Set condition codes
	cpu.SetFlags(cpu.DataRegs[reg], M68K_SIZE_LONG)
}

// CCR/SR operations
type SROperation int

const (
	SR_AND SROperation = iota // Logical AND
	SR_OR                     // Logical OR
	SR_EOR                    // Logical XOR
)

type SRTarget int

const (
	TARGET_CCR SRTarget = iota // Operation targets CCR (lower byte of SR)
	TARGET_SR                  // Operation targets full SR (requires supervisor mode)
)

func (cpu *M68KCPU) ExecSROperation(operation SROperation, target SRTarget) {
	// Check supervisor mode for SR operations
	if target == TARGET_SR && (cpu.SR&M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Fetch immediate data
	data := cpu.Fetch16()

	// For CCR operations, only use the lower byte
	if target == TARGET_CCR {
		data &= 0xFF
	}

	// Apply the operation
	switch operation {
	case SR_AND:
		if target == TARGET_CCR {
			// AND to CCR (lower byte of SR)
			cpu.SR = (cpu.SR & 0xFF00) | ((cpu.SR & data) & 0xFF)
		} else {
			// AND to full SR
			cpu.SR &= data
		}
	case SR_OR:
		if target == TARGET_CCR {
			// OR to CCR (lower byte of SR)
			cpu.SR = (cpu.SR & 0xFF00) | ((cpu.SR | data) & 0xFF)
		} else {
			// OR to full SR
			cpu.SR |= data
		}
	case SR_EOR:
		if target == TARGET_CCR {
			// XOR to CCR (lower byte of SR)
			cpu.SR = (cpu.SR & 0xFF00) | ((cpu.SR ^ data) & 0xFF)
		} else {
			// XOR to full SR
			cpu.SR ^= data
		}
	}

	// Update cycle counter
	cpu.cycleCounter += M68K_CYCLE_EXECUTE + M68K_CYCLE_FETCH
}
func (cpu *M68KCPU) ExecAndiCcr() {
	cpu.ExecSROperation(SR_AND, TARGET_CCR)
}
func (cpu *M68KCPU) ExecOriCcr() {
	cpu.ExecSROperation(SR_OR, TARGET_CCR)
}
func (cpu *M68KCPU) ExecEoriCcr() {
	cpu.ExecSROperation(SR_EOR, TARGET_CCR)
}
func (cpu *M68KCPU) ExecAndiSr() {
	cpu.ExecSROperation(SR_AND, TARGET_SR)
}
func (cpu *M68KCPU) ExecOriSr() {
	cpu.ExecSROperation(SR_OR, TARGET_SR)
}
func (cpu *M68KCPU) ExecEoriSr() {
	cpu.ExecSROperation(SR_EOR, TARGET_SR)
}

// Shift and rotate operations
type ShiftRotateOperation int

const (
	ASL  ShiftRotateOperation = iota // Arithmetic Shift Left
	ASR                              // Arithmetic Shift Right
	LSL                              // Logical Shift Left
	LSR                              // Logical Shift Right
	ROL                              // Rotate Left
	ROR                              // Rotate Right
	ROXL                             // Rotate Left with Extend
	ROXR                             // Rotate Right with Extend
)

func (cpu *M68KCPU) ExecShiftRotateMemory() {
	mode := (cpu.currentIR >> 3) & 0x7
	reg := cpu.currentIR & 0x7
	operation := (cpu.currentIR >> 8) & 0x7 // Determines shift/rotate type

	// Get operand address
	addr := cpu.GetEffectiveAddress(mode, reg)

	// Read word from memory
	value := uint32(cpu.Read16(addr))

	// Determine operation type and perform shift/rotate by 1 bit
	var result uint32
	var setCBit bool

	switch operation {
	case 0: // ASR
		setCBit = (value & 0x0001) != 0
		if (value & 0x8000) != 0 {
			result = (value >> 1) | 0x8000 // Arithmetic shift preserves sign bit
		} else {
			result = value >> 1
		}
	case 1: // ASL
		setCBit = (value & 0x8000) != 0
		result = (value << 1) & 0xFFFF
	case 2: // LSR
		setCBit = (value & 0x0001) != 0
		result = value >> 1
	case 3: // LSL
		setCBit = (value & 0x8000) != 0
		result = (value << 1) & 0xFFFF
	case 4: // ROXR
		setCBit = (value & 0x0001) != 0
		result = value >> 1
		if (cpu.SR & M68K_SR_X) != 0 {
			result |= 0x8000
		}
	case 5: // ROXL
		setCBit = (value & 0x8000) != 0
		result = (value << 1) & 0xFFFF
		if (cpu.SR & M68K_SR_X) != 0 {
			result |= 0x0001
		}
	case 6: // ROR
		setCBit = (value & 0x0001) != 0
		result = value >> 1
		if (value & 0x0001) != 0 {
			result |= 0x8000
		}
	case 7: // ROL
		setCBit = (value & 0x8000) != 0
		result = (value << 1) & 0xFFFF
		if (value & 0x8000) != 0 {
			result |= 0x0001
		}
	}

	// Write result back to memory
	cpu.Write16(addr, uint16(result))

	// Set condition codes
	cpu.SetFlagsNZ(result, M68K_SIZE_WORD)

	// Clear V flag
	cpu.SR &= ^uint16(M68K_SR_V)

	// Set C and X flags if needed
	cpu.SR &= ^uint16(M68K_SR_C | M68K_SR_X)
	if setCBit {
		cpu.SR |= M68K_SR_C | M68K_SR_X
	}

	cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE + cpu.GetEACycles(mode, reg)
}
func (cpu *M68KCPU) GetShiftCount(count uint16, regOrImm uint16, size int, isRotate bool, includesX bool) uint32 {
	var countVal uint32

	// Get shift count
	if regOrImm == 1 {
		// Register shift count
		countVal = cpu.DataRegs[count] & 0x3F
	} else {
		// Immediate shift count
		if count == 0 {
			countVal = 8 // Count of 0 means 8
		} else {
			countVal = uint32(count)
		}
	}

	// Apply size-specific limits
	if isRotate {
		// For rotates, apply modulo
		if size == M68K_SIZE_BYTE {
			if includesX {
				countVal %= 9 // For ROX, modulo is bit size + 1 (due to X flag)
			} else {
				countVal %= 8
			}
		} else if size == M68K_SIZE_WORD {
			if includesX {
				countVal %= 17
			} else {
				countVal %= 16
			}
		} else { // LONG
			if includesX {
				countVal %= 33
			} else {
				countVal %= 32
			}
		}
	} else {
		// For shifts, limit to maximum bits
		if size == M68K_SIZE_BYTE {
			if countVal > 8 {
				countVal = 8
			}
		} else if size == M68K_SIZE_WORD {
			if countVal > 16 {
				countVal = 16
			}
		} else { // LONG
			if countVal > 32 {
				countVal = 32
			}
		}
	}

	return countVal
}
func (cpu *M68KCPU) GetSizeMasks(size int) (mask, msbMask, lsbMask uint32) {
	switch size {
	case M68K_SIZE_BYTE:
		return 0xFF, 0x80, 0x01
	case M68K_SIZE_WORD:
		return 0xFFFF, 0x8000, 0x0001
	case M68K_SIZE_LONG:
		return 0xFFFFFFFF, 0x80000000, 0x00000001
	default:
		return 0, 0, 0
	}
}
func (cpu *M68KCPU) UpdateRegisterWithResult(reg uint16, result uint32, size int) {
	if size == M68K_SIZE_BYTE {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (result & 0xFF)
	} else if size == M68K_SIZE_WORD {
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (result & 0xFFFF)
	} else { // LONG
		cpu.DataRegs[reg] = result
	}
}
func (cpu *M68KCPU) ExecShiftRotate(operation ShiftRotateOperation, reg, count uint16, regOrImm uint16, size int) {
	mask, msbMask, lsbMask := cpu.GetSizeMasks(size)
	if mask == 0 {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	countVal := cpu.GetShiftCount(count, regOrImm, size, isRotateOp(operation), hasExtendBit(operation))
	value := cpu.DataRegs[reg] & mask

	if countVal == 0 {
		cpu.cycleCounter += M68K_CYCLE_REG
		return
	}

	// Initial setup for specific operations
	// IMPORTANT: Read X flag BEFORE clearing flags, as ROXL/ROXR need the original value
	msb := (value & msbMask) != 0
	xFlag := (cpu.SR & M68K_SR_X) != 0

	// Clear flags based on operation
	if operation == ROL || operation == ROR {
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	} else {
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
	}
	var vFlag, cFlag bool

	// Perform the specific shift/rotate operation
	result := value
	switch operation {
	case ASL:
		// Handle ASL operation
		if (size == M68K_SIZE_BYTE && countVal >= 8) ||
			(size == M68K_SIZE_WORD && countVal >= 16) ||
			(size == M68K_SIZE_LONG && countVal >= 32) {
			if value != 0 {
				cFlag = true
				vFlag = true
			}
			result = 0
		} else {
			for range countVal {
				if (result & msbMask) != 0 {
					cFlag = true
					vFlag = true
				}
				result = (result << 1) & mask
			}
		}

	case ASR:
		// Handle ASR operation
		if (size == M68K_SIZE_BYTE && countVal >= 8) ||
			(size == M68K_SIZE_WORD && countVal >= 16) ||
			(size == M68K_SIZE_LONG && countVal >= 32) {
			if value != 0 {
				cFlag = true
			}

			if msb {
				result = mask
			} else {
				result = 0
			}
		} else {
			for range countVal {
				if (result & lsbMask) != 0 {
					cFlag = true
				} else {
					cFlag = false
				}

				result = result >> 1

				if msb {
					result |= msbMask
				}
			}
		}

	case LSL:
		// Handle LSL operation
		if (size == M68K_SIZE_BYTE && countVal >= 8) ||
			(size == M68K_SIZE_WORD && countVal >= 16) ||
			(size == M68K_SIZE_LONG && countVal >= 32) {
			if value != 0 {
				cFlag = true
			}
			result = 0
		} else {
			var lastBit uint32
			if size == M68K_SIZE_BYTE {
				lastBit = (value >> (8 - countVal)) & 0x01
			} else if size == M68K_SIZE_WORD {
				lastBit = (value >> (16 - countVal)) & 0x01
			} else {
				lastBit = (value >> (32 - countVal)) & 0x01
			}

			cFlag = lastBit != 0
			result = (value << countVal) & mask
		}

	case LSR:
		// Handle LSR operation
		if (size == M68K_SIZE_BYTE && countVal >= 8) ||
			(size == M68K_SIZE_WORD && countVal >= 16) ||
			(size == M68K_SIZE_LONG && countVal >= 32) {
			if value != 0 {
				cFlag = true
			}
			result = 0
		} else {
			var lastBit uint32 = (value >> (countVal - 1)) & 0x01
			cFlag = lastBit != 0
			result = (value >> countVal) & mask
		}

	case ROL:
		// Handle ROL operation
		for range countVal {
			var msb uint32 = 0
			if (result & msbMask) != 0 {
				msb = lsbMask
			}

			result = ((result << 1) & mask) | msb
		}

		cFlag = (result & lsbMask) != 0

	case ROR:
		// Handle ROR operation
		for range countVal {
			var lsb uint32 = 0
			if (result & lsbMask) != 0 {
				lsb = msbMask
			}

			result = (result >> 1) | lsb
		}

		cFlag = (result & msbMask) != 0

	case ROXL:
		// Handle ROXL operation
		for range countVal {
			var msb bool = (result & msbMask) != 0

			result = (result << 1) & mask

			if xFlag {
				result |= lsbMask
			}

			xFlag = msb
		}

		cFlag = xFlag

	case ROXR:
		// Handle ROXR operation
		for range countVal {
			var lsb bool = (result & lsbMask) != 0

			result = result >> 1

			if xFlag {
				result |= msbMask
			}

			xFlag = lsb
		}

		cFlag = xFlag
	}

	// Update register with result
	cpu.UpdateRegisterWithResult(reg, result, size)

	// Set N and Z flags based on result
	cpu.SetFlagsNZ(result, size)

	// Set V, C, X flags based on operation results
	if vFlag {
		cpu.SR |= M68K_SR_V
	}

	if cFlag {
		cpu.SR |= M68K_SR_C
		// Also set X flag for operations that affect it
		if operation != ROL && operation != ROR {
			cpu.SR |= M68K_SR_X
		}
	}

	// Update cycle counter
	cpu.cycleCounter += M68K_CYCLE_REG + (M68K_CYCLE_REG * countVal)
}
func isRotateOp(op ShiftRotateOperation) bool {
	return op == ROL || op == ROR || op == ROXL || op == ROXR
}
func hasExtendBit(op ShiftRotateOperation) bool {
	return op == ROXL || op == ROXR
}
func (cpu *M68KCPU) ExecASL(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ASL, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecASR(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ASR, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecLSL(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(LSL, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecLSR(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(LSR, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecROL(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ROL, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecROR(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ROR, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecROXL(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ROXL, reg, count, regOrImm, size)
}
func (cpu *M68KCPU) ExecROXR(reg, count uint16, regOrImm uint16, size int) {
	cpu.ExecShiftRotate(ROXR, reg, count, regOrImm, size)
}

// Bit manipulation operations
type BitManipOperation int

const (
	BTST BitManipOperation = iota // Test bit
	BCHG                          // Test and change bit
	BCLR                          // Test and clear bit
	BSET                          // Test and set bit
)

func (cpu *M68KCPU) ExecBitManip(operation BitManipOperation, reg, mode, xreg uint16) {
	var bitNum uint32
	var value uint32
	var isRegisterMode bool = mode == M68K_AM_DR
	var addr uint32

	// Get operand and bit number
	if isRegisterMode {
		// Data register direct - Long word operation
		value = cpu.DataRegs[xreg]
		bitNum = cpu.DataRegs[reg] & 0x1F
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand - Byte operation
		addr = cpu.GetEffectiveAddress(mode, xreg)
		value = uint32(cpu.Read8(addr))
		bitNum = cpu.DataRegs[reg] & 0x07
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Test the specified bit and set Z flag accordingly.
	// Bit manipulation instructions test a specific bit and set the Z flag to the complement
	// of the tested bit (Z=1 if bit=0, Z=0 if bit=1). This allows conditional branching
	// based on individual bit states, which is essential for status flag testing and bit-field
	// manipulation. Other flags (N, V, C) remain unchanged.
	bitSet := (value & (1 << bitNum)) != 0
	if bitSet {
		cpu.SR &= ^uint16(M68K_SR_Z) // Clear Z if bit is set
	} else {
		cpu.SR |= M68K_SR_Z // Set Z if bit is clear
	}

	// Perform operation-specific actions
	switch operation {
	case BTST:
		// Only test bit, no modification
		return

	case BCHG:
		// Change the bit (invert it)
		value ^= (1 << bitNum)

	case BCLR:
		// Clear the bit
		value &= ^(1 << bitNum)

	case BSET:
		// Set the bit
		value |= (1 << bitNum)
	}

	// Write back the modified value
	if isRegisterMode {
		cpu.DataRegs[xreg] = value
	} else {
		cpu.Write8(addr, uint8(value))
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}

// ExecBitManipImm handles immediate bit manipulation: BTST/BCHG/BCLR/BSET #n,<ea>
func (cpu *M68KCPU) ExecBitManipImm(operation BitManipOperation, mode, xreg uint16) {
	// Fetch bit number from extension word
	bitNum := uint32(cpu.Fetch16() & 0xFF)

	var value uint32
	var isRegisterMode bool = mode == M68K_AM_DR
	var addr uint32

	// Get operand
	if isRegisterMode {
		// Data register direct - Long word operation, modulo 32
		value = cpu.DataRegs[xreg]
		bitNum = bitNum & 0x1F
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand - Byte operation, modulo 8
		addr = cpu.GetEffectiveAddress(mode, xreg)
		value = uint32(cpu.Read8(addr))
		bitNum = bitNum & 0x07
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Test the specified bit and set Z flag accordingly
	bitSet := (value & (1 << bitNum)) != 0
	if bitSet {
		cpu.SR &= ^uint16(M68K_SR_Z) // Clear Z if bit is set
	} else {
		cpu.SR |= M68K_SR_Z // Set Z if bit is clear
	}

	// Perform operation-specific actions
	switch operation {
	case BTST:
		// Only test bit, no modification
		return

	case BCHG:
		// Change the bit (invert it)
		value ^= (1 << bitNum)

	case BCLR:
		// Clear the bit
		value &= ^(1 << bitNum)

	case BSET:
		// Set the bit
		value |= (1 << bitNum)
	}

	// Write back the modified value
	if isRegisterMode {
		cpu.DataRegs[xreg] = value
	} else {
		cpu.Write8(addr, uint8(value))
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}

func (cpu *M68KCPU) ExecBtst(reg, mode, xreg uint16) {
	cpu.ExecBitManip(BTST, reg, mode, xreg)
}
func (cpu *M68KCPU) ExecBchg(reg, mode, xreg uint16) {
	cpu.ExecBitManip(BCHG, reg, mode, xreg)
}
func (cpu *M68KCPU) ExecBclr(reg, mode, xreg uint16) {
	cpu.ExecBitManip(BCLR, reg, mode, xreg)
}
func (cpu *M68KCPU) ExecBset(reg, mode, xreg uint16) {
	cpu.ExecBitManip(BSET, reg, mode, xreg)
}

// BCD operations
func (cpu *M68KCPU) ExecAbcd(rm, rx, ry uint16) {
	var src, dst uint8

	// Get operands
	if rm == 0 {
		// Data register mode
		src = uint8(cpu.DataRegs[rx] & 0xFF)
		dst = uint8(cpu.DataRegs[ry] & 0xFF)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory predecrement mode
		cpu.AddrRegs[rx]--
		src = cpu.Read8(cpu.AddrRegs[rx])

		cpu.AddrRegs[ry]--
		dst = cpu.Read8(cpu.AddrRegs[ry])
		cpu.cycleCounter += M68K_CYCLE_MEM_READ * 2
	}

	// Get extend bit
	xBit := uint16(0)
	if cpu.SR&M68K_SR_X != 0 {
		xBit = 1
	}

	// Perform BCD addition using the correct M68000 algorithm:
	// 1. Add low nibbles + X
	// 2. If > 9, add 6 (low nibble correction)
	// 3. Add high nibbles to the running result
	// 4. If > 0x99, add 0x60 (high nibble correction) and set carry

	res := uint16(src&0x0F) + uint16(dst&0x0F) + xBit
	if res > 9 {
		res += 6
	}
	res += uint16(src&0xF0) + uint16(dst&0xF0)

	// Clear X, C, N, and V flags (N and V are technically undefined but tests expect them cleared/set)
	cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_C | M68K_SR_N | M68K_SR_V)

	// If result > 0x99, adjust and set carry
	if res > 0x99 {
		res += 0x60
		cpu.SR |= M68K_SR_X | M68K_SR_C
	}

	result := uint8(res)

	// Set N based on MSB of result
	if result&0x80 != 0 {
		cpu.SR |= M68K_SR_N
	}

	// Update Z flag - cleared if result is non-zero, unchanged otherwise
	if result != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}

	// Store result
	if rm == 0 {
		// Data register
		cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFFFF00) | uint32(result)
	} else {
		// Memory
		cpu.Write8(cpu.AddrRegs[ry], result)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}
func (cpu *M68KCPU) ExecSbcd(rm, rx, ry uint16) {
	var src, dst uint8

	// Get operands
	if rm == 0 {
		// Data register mode
		src = uint8(cpu.DataRegs[rx] & 0xFF)
		dst = uint8(cpu.DataRegs[ry] & 0xFF)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory predecrement mode
		cpu.AddrRegs[rx]--
		src = cpu.Read8(cpu.AddrRegs[rx])

		cpu.AddrRegs[ry]--
		dst = cpu.Read8(cpu.AddrRegs[ry])
		cpu.cycleCounter += M68K_CYCLE_MEM_READ * 2
	}

	// Get extend bit
	xBit := int16(0)
	if cpu.SR&M68K_SR_X != 0 {
		xBit = 1
	}

	// Perform BCD subtraction using the correct M68000 algorithm:
	// 1. Subtract low nibbles (dst - src - X)
	// 2. If < 0, subtract 6 (low nibble correction)
	// 3. Subtract high nibbles from running result
	// 4. If < 0, add 0xA0 (high nibble correction) and set carry

	res := int16(dst&0x0F) - int16(src&0x0F) - xBit
	if res < 0 {
		res -= 6
	}
	res += int16(dst&0xF0) - int16(src&0xF0)

	// Clear X, C, N, and V flags (N and V are technically undefined but tests expect them cleared/set)
	cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_C | M68K_SR_N | M68K_SR_V)

	// If result < 0, adjust and set carry
	if res < 0 {
		res += 0xA0
		cpu.SR |= M68K_SR_X | M68K_SR_C
	}

	result := uint8(res)

	// Set N based on MSB of result
	if result&0x80 != 0 {
		cpu.SR |= M68K_SR_N
	}

	// Update Z flag - cleared if result is non-zero, unchanged otherwise
	if result != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}

	// Store result
	if rm == 0 {
		// Data register
		cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFFFF00) | uint32(result)
	} else {
		// Memory
		cpu.Write8(cpu.AddrRegs[ry], result)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}
func (cpu *M68KCPU) ExecNbcd(mode, reg uint16) {
	var src uint8
	var addr uint32

	// Get operand
	if mode == M68K_AM_DR {
		// Data register mode
		src = uint8(cpu.DataRegs[reg] & 0xFF)
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		// Memory operand
		addr = cpu.GetEffectiveAddress(mode, reg)
		src = cpu.Read8(addr)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Get extend bit
	xBit := int16(0)
	if cpu.SR&M68K_SR_X != 0 {
		xBit = 1
	}

	// NBCD computes: 0 - source - X (BCD subtraction)
	// Using the same algorithm as SBCD with dst = 0:
	// 1. res = (0 & 0x0F) - (src & 0x0F) - X
	// 2. If < 0, subtract 6
	// 3. res += (0 & 0xF0) - (src & 0xF0)
	// 4. If < 0, add 0xA0 and set carry

	res := -int16(src&0x0F) - xBit
	if res < 0 {
		res -= 6
	}
	res -= int16(src & 0xF0)

	// Clear X, C, N, and V flags (N and V are technically undefined but tests expect them cleared/set)
	cpu.SR &= ^uint16(M68K_SR_X | M68K_SR_C | M68K_SR_N | M68K_SR_V)

	// If result < 0, adjust and set carry
	if res < 0 {
		res += 0xA0
		cpu.SR |= M68K_SR_X | M68K_SR_C
	}

	result := uint8(res)

	// Set N based on MSB of result
	if result&0x80 != 0 {
		cpu.SR |= M68K_SR_N
	}

	// Update Z flag - cleared if result is non-zero, unchanged otherwise
	if result != 0 {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}

	// Store result
	if mode == M68K_AM_DR {
		// Data register
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | uint32(result)
	} else {
		// Memory - use the address we computed earlier
		cpu.Write8(addr, result)
		cpu.cycleCounter += M68K_CYCLE_MEM_WRITE
	}
}
func (cpu *M68KCPU) ExecPack(rx, ry uint16, isRegisterMode bool) {
	// Fetch adjustment value
	adjustment := cpu.Fetch16()

	var source uint16

	if isRegisterMode {
		// Register mode
		source = uint16(cpu.DataRegs[rx] & 0xFFFF)
	} else {
		// Memory mode (predecrement)
		cpu.AddrRegs[rx] -= M68K_WORD_SIZE
		source = cpu.Read16(cpu.AddrRegs[rx])
	}

	// Extract BCD digits
	highNibble := (source >> 8) & 0x0F
	lowNibble := source & 0x0F

	// Pack into a byte and add adjustment
	packed := ((highNibble << 4) | lowNibble) + adjustment

	if isRegisterMode {
		// Register mode - update lower byte of destination register
		cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFFFF00) | uint32(packed&0xFF)
	} else {
		// Memory mode (predecrement)
		cpu.AddrRegs[ry] -= M68K_BYTE_SIZE
		cpu.Write8(cpu.AddrRegs[ry], uint8(packed))
	}

	// Update cycle counter
	if isRegisterMode {
		cpu.cycleCounter += M68K_CYCLE_REG * 8
	} else {
		cpu.cycleCounter += M68K_CYCLE_REG * 10
	}
}
func (cpu *M68KCPU) ExecUnpk(rx, ry uint16, isRegisterMode bool) {
	// Fetch adjustment value
	adjustment := cpu.Fetch16()

	var source uint8

	if isRegisterMode {
		// Register mode
		source = uint8(cpu.DataRegs[rx] & 0xFF)
	} else {
		// Memory mode (predecrement)
		cpu.AddrRegs[rx] -= M68K_BYTE_SIZE
		source = cpu.Read8(cpu.AddrRegs[rx])
	}

	// Extract BCD digits
	highNibble := (source >> 4) & 0x0F
	lowNibble := source & 0x0F

	// Unpack to word and add adjustment
	unpacked := (uint16(highNibble) << 8) | uint16(lowNibble)
	unpacked += adjustment

	if isRegisterMode {
		// Register mode - update lower word of destination register
		cpu.DataRegs[ry] = (cpu.DataRegs[ry] & 0xFFFF0000) | uint32(unpacked)
	} else {
		// Memory mode (predecrement)
		cpu.AddrRegs[ry] -= M68K_WORD_SIZE
		cpu.Write16(cpu.AddrRegs[ry], unpacked)
	}

	// Update cycle counter
	if isRegisterMode {
		cpu.cycleCounter += M68K_CYCLE_REG * 8
	} else {
		cpu.cycleCounter += M68K_CYCLE_REG * 10
	}
}

// Program flow control
func (cpu *M68KCPU) ExecDBcc(condition, reg uint16) {
	// Fetch displacement
	displacement := int16(cpu.Fetch16())

	// Check condition
	conditionMet := cpu.CheckCondition(uint8(condition))

	if !conditionMet {
		// Decrement counter
		counter := int16(cpu.DataRegs[reg] & 0xFFFF)
		counter--

		// Update lower word of register (convert to uint16 first to avoid overflow)
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | uint32(uint16(counter))

		if counter != -1 {
			// Branch
			cpu.PC = cpu.PC - M68K_WORD_SIZE + uint32(displacement)
		}
	}

	cpu.cycleCounter += M68K_CYCLE_BRANCH
}
func (cpu *M68KCPU) ExecChk(reg, mode, xreg uint16, size int) {
	var source uint32

	// Get source operand
	if mode == M68K_AM_DR {
		source = cpu.DataRegs[xreg]
		cpu.cycleCounter += M68K_CYCLE_REG
	} else {
		addr := cpu.GetEffectiveAddress(mode, xreg)
		if size == M68K_SIZE_WORD {
			source = uint32(cpu.Read16(addr))
		} else { // CHK.L for 68020+
			source = cpu.Read32(addr)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ
	}

	// Get register value
	dest := cpu.DataRegs[reg]

	// Apply size mask
	if size == M68K_SIZE_WORD {
		// Sign extend
		if (dest & 0x8000) != 0 {
			dest |= 0xFFFF0000
		} else {
			dest &= 0x0000FFFF
		}

		// Sign extend source too if it's a register
		if mode == M68K_AM_DR && (source&0x8000) != 0 {
			source |= 0xFFFF0000
		} else if mode == M68K_AM_DR {
			source &= 0x0000FFFF
		}
	}

	// Set N flag if register is negative
	cpu.SR &= ^uint16(M68K_SR_N)
	if (int32(dest) < 0 && size == M68K_SIZE_WORD) || (dest&0x80000000) != 0 {
		cpu.SR |= M68K_SR_N
	}

	// Check bounds
	if (int32(dest) < 0) || (dest > source) {
		// Bounds violation
		cpu.ProcessException(M68K_VEC_CHK)
	}

	cpu.cycleCounter += M68K_CYCLE_BRANCH
}
func (cpu *M68KCPU) ExecChk2Cmp2(size, mode, reg uint16) {
	// Get the extension word
	ext := cpu.Fetch16()

	// Extract register number
	rn := ext & M68K_EXT_REG_MASK

	// Determine if it's an address or data register
	isAddressReg := (ext & (1 << M68K_EXT_REG_TYPE_BIT)) != 0

	// Get the bounds address
	boundsAddr := cpu.GetEffectiveAddress(mode, reg)

	// Get the register value to check
	var checkValue uint32
	if isAddressReg {
		checkValue = cpu.AddrRegs[rn&0x07]
	} else {
		checkValue = cpu.DataRegs[rn&0x07]
	}

	// Read the bounds (lower and upper)
	var lowerBound, upperBound uint32
	var mask uint32

	switch size {
	case M68K_SIZE_BYTE:
		lowerBound = uint32(cpu.Read8(boundsAddr))
		upperBound = uint32(cpu.Read8(boundsAddr + M68K_BYTE_SIZE))
		mask = 0xFF
		checkValue &= mask
	case M68K_SIZE_WORD:
		lowerBound = uint32(cpu.Read16(boundsAddr))
		upperBound = uint32(cpu.Read16(boundsAddr + M68K_WORD_SIZE))
		mask = 0xFFFF
		checkValue &= mask
	case M68K_SIZE_LONG:
		lowerBound = cpu.Read32(boundsAddr)
		upperBound = cpu.Read32(boundsAddr + M68K_LONG_SIZE)
		mask = 0xFFFFFFFF
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_C)

	// Compare with lower bound
	if checkValue < lowerBound {
		cpu.SR |= M68K_SR_C // Out of bounds below
		// For signed comparison, if they have different signs or same sign but different result
		if ((checkValue ^ lowerBound) & (mask >> 1)) != 0 {
			if ((checkValue & (mask >> 1)) == 0) && ((lowerBound & (mask >> 1)) != 0) {
				cpu.SR |= M68K_SR_N // Negative for signed comparison
			}
		} else if checkValue < lowerBound {
			cpu.SR |= M68K_SR_N // Negative for signed comparison
		}
	} else if checkValue == lowerBound {
		cpu.SR |= M68K_SR_Z // Equal to lower bound
	}

	// Compare with upper bound
	if checkValue > upperBound {
		cpu.SR |= M68K_SR_C // Out of bounds above
	} else if checkValue == upperBound {
		cpu.SR |= M68K_SR_Z // Equal to upper bound
	}

	// If this is CHK2 and bounds violation detected, trap
	if (ext&0x0800) != 0 && (cpu.SR&M68K_SR_C) != 0 {
		// It's a CHK2 instruction with a bounds violation
		cpu.ProcessException(M68K_VEC_CHK)
	}

	// Update cycle counter
	cpu.cycleCounter += M68K_CYCLE_BRANCH * 2
}
func (cpu *M68KCPU) ExecLink(reg uint16) {
	// LINK An,#<displacement>
	displacement := int16(cpu.Fetch16())

	// Push the current address register value onto the stack
	cpu.Push32(cpu.AddrRegs[reg])

	// Move the stack pointer to the address register
	cpu.AddrRegs[reg] = cpu.AddrRegs[7] // A7 is SP

	// Add displacement to stack pointer
	cpu.AddrRegs[7] += uint32(displacement)

	// Update cycle counter
	cpu.cycleCounter += M68K_CYCLE_LINK
}
func (cpu *M68KCPU) ExecUnlk(reg uint16) {
	// UNLK An

	// Move address register to stack pointer
	cpu.AddrRegs[7] = cpu.AddrRegs[reg] // A7 is SP

	// Pop value from stack to address register
	cpu.AddrRegs[reg] = cpu.Pop32()

	// Update cycle counter
	cpu.cycleCounter += M68K_CYCLE_UNLK
}
func (cpu *M68KCPU) ExecCallm(mode, reg uint16) {
	// Get the extension word (argument count)
	argCount := cpu.Fetch16() & 0xFF

	// Get the module descriptor address
	descAddr := cpu.GetEffectiveAddress(mode, reg)

	// Read module descriptor
	modType := int32(cpu.Read16(descAddr))

	// Check supervisor mode
	if (cpu.SR&M68K_SR_S) == 0 && (modType < 0) {
		// Negative module type requires supervisor mode
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}

	// Create a module stack frame
	// Push current PC
	cpu.Push32(cpu.PC)

	// Create control word
	ctrlWord := uint16(0)
	// Set module type bits
	if modType < 0 {
		ctrlWord |= 1 << 15 // Supervisor module bit (MSB)
	}

	// Push control word
	cpu.Push16(ctrlWord)

	// Push argument count
	cpu.Push16(argCount)

	// Push static frame pointer
	sfp := descAddr + M68K_WORD_SIZE // Skip module type
	cpu.Push32(sfp)

	// Set new PC from descriptor
	cpu.PC = cpu.Read32(descAddr + M68K_LONG_SIZE)

	// If supervisor module, switch to supervisor mode
	if modType < 0 {
		cpu.SR |= M68K_SR_S
	}

	// No specific constant for CALLM cycle timing
	cpu.cycleCounter += M68K_CYCLE_EXCEPTION * 2
}
func (cpu *M68KCPU) ExecRtm(reg uint16) {
	// Determine if register is data or address register and read it
	// The module pointer is specified in the instruction but not used in our implementation
	if (reg & (1 << 3)) != 0 {
		_ = cpu.AddrRegs[reg&0x07] // Read address register
	} else {
		_ = cpu.DataRegs[reg&0x07] // Read data register
	}

	// Pop and discard static frame pointer
	cpu.Pop32()

	// Pop and discard argument count
	cpu.Pop16()

	// Pop control word
	ctrlWord := cpu.Pop16()

	// Pop return address
	returnAddr := cpu.Pop32()

	// Restore PC
	cpu.PC = returnAddr

	// If returning from supervisor module, check if we should return to user mode
	if (ctrlWord & (1 << 15)) != 0 {
		// Check original mode in the saved SR
		savedSR := cpu.SR

		// If returning to user mode
		if (savedSR & M68K_SR_S) == 0 {
			cpu.SR &= ^uint16(M68K_SR_S)
		}
	}

	// No specific constant for RTM cycle timing
	cpu.cycleCounter += M68K_CYCLE_RTE + 6
}
func (cpu *M68KCPU) ExecNOP() {
	cpu.cycleCounter += M68K_CYCLE_EXECUTE
}
func (cpu *M68KCPU) ExecRTS() {
	cpu.PC = cpu.Pop32()
	cpu.cycleCounter += M68K_CYCLE_RTS
}
func (cpu *M68KCPU) ExecRTE() {
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}
	newSR := cpu.Pop16()
	cpu.PC = cpu.Pop32()
	formatWord := cpu.Pop16()
	format := (formatWord >> 12) & 0xF
	words := cpu.exceptionFrameWords(format)
	if words == 0 {
		cpu.ProcessException(M68K_VEC_FORMAT_ERROR)
		return
	}
	remainingWords := words - 4
	for range remainingWords {
		cpu.Pop16()
	}
	oldSupervisor := (cpu.SR & M68K_SR_S) != 0
	newSupervisor := (newSR & M68K_SR_S) != 0
	cpu.SR = newSR
	if oldSupervisor != newSupervisor {
		cpu.swapStacksForMode(newSupervisor)
	}
	cpu.inException.Store(false)
	cpu.cycleCounter += M68K_CYCLE_RTE
}
func (cpu *M68KCPU) ExecSTOP() {
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}
	cpu.SR = cpu.Fetch16()
	cpu.stopped.Store(true)
}
func (cpu *M68KCPU) ExecRESET() {
	if (cpu.SR & M68K_SR_S) == 0 {
		cpu.ProcessException(M68K_VEC_PRIVILEGE)
		return
	}
	cpu.cycleCounter += 132
}
func (cpu *M68KCPU) ExecTRAPV() {
	if (cpu.SR & M68K_SR_V) != 0 {
		cpu.ProcessException(M68K_VEC_TRAPV)
	}
}
func (cpu *M68KCPU) ExecRTR() {
	cpu.SR = (cpu.SR & 0xFF00) | (cpu.Pop16() & 0x00FF)
	cpu.PC = cpu.Pop32()
	cpu.cycleCounter += M68K_CYCLE_RTR
}
func (cpu *M68KCPU) ExecBRA(opcode uint16) {
	// Extract branch condition and displacement from the opcode.
	condition := (opcode >> 8) & 0xF
	displacement := int8(opcode & 0xFF)
	isBSR := condition == 1

	// Determine if the branch should be taken.
	var takeBranch bool
	if condition == 0 { // BRA always branches.
		takeBranch = true
	} else if isBSR { // BSR: Branch to subroutine.
		// Note: Push return address AFTER fetching displacement (below)
		takeBranch = true
	} else {
		takeBranch = cpu.CheckCondition(uint8(condition))
	}

	// Calculate effective displacement.
	// The 68K branch displacement is calculated relative to (instruction_addr + 2).
	// After fetching the instruction word, PC = instruction_addr + 2.
	// For word/long displacements, extra words are fetched which advance PC further.
	var effectiveDisplacement int32
	var pcAdjust uint32 = 0 // Bytes to subtract from current PC to get base for displacement

	if displacement == 0 {
		// Word displacement: PC advanced by 2 for the displacement word
		effectiveDisplacement = int32(int16(cpu.Fetch16()))
		pcAdjust = M68K_WORD_SIZE
	} else if displacement == -1 {
		// Long displacement: PC advanced by 4 for the displacement long
		effectiveDisplacement = int32(cpu.Fetch32())
		pcAdjust = M68K_LONG_SIZE
	} else {
		// Byte displacement: no extra fetch, PC is already at instruction_addr + 2
		effectiveDisplacement = int32(displacement)
		pcAdjust = 0
	}

	// For BSR, push return address AFTER fetching displacement
	// The return address is PC pointing to the instruction after the entire BSR instruction
	if isBSR {
		cpu.Push32(cpu.PC)
	}

	// If branch condition is met, update PC.
	if takeBranch {
		targetPC := cpu.PC - pcAdjust + uint32(effectiveDisplacement)

		// Only halt on truly out-of-bounds addresses (above memory size)
		// Branches to low memory (vector table area) are valid on real hardware
		if targetPC >= M68K_MEMORY_SIZE-M68K_WORD_SIZE {
			cpu.running.Store(false)
			return
		}
		cpu.PC = targetPC
		cpu.prefetchSize = 0
	}
}
func (cpu *M68KCPU) ExecTRAP(opcode uint16) {
	// Calculate the vector: add the lower nibble of the opcode to the TRAP base.
	vector := M68K_VEC_TRAP_BASE + (opcode & 0x000F)
	// Process the exception using the computed vector.
	cpu.ProcessException(uint8(vector))
}
func (cpu *M68KCPU) ExecJsr(mode, reg uint16) {
	addr := cpu.GetEffectiveAddress(mode, reg)
	cpu.Push32(cpu.PC)
	cpu.PC = addr
	cpu.cycleCounter += M68K_CYCLE_BRANCH
}
func (cpu *M68KCPU) ExecJmp(mode, reg uint16) {
	addr := cpu.GetEffectiveAddress(mode, reg)
	cpu.PC = addr
	cpu.cycleCounter += M68K_CYCLE_BRANCH
}
func (cpu *M68KCPU) ExecBkpt(bkptNum uint16) {
	// Process the breakpoint exception
	cpu.ProcessException(M68K_VEC_BKPT)
}

// Atomic operations
func (cpu *M68KCPU) ExecCas(opmode, mode, reg uint16) {
	// Get the extension word
	ext := cpu.Fetch16()

	// Extract register numbers
	dcReg := ext & 0x07        // Data to compare
	duReg := (ext >> 6) & 0x07 // Data to update

	// Determine size
	var size int
	var mask uint32
	var sizeCycles uint32

	switch opmode {
	case M68K_SIZE_BYTE: // Byte
		size = M68K_SIZE_BYTE
		mask = 0xFF
		sizeCycles = 12
	case M68K_SIZE_WORD: // Word
		size = M68K_SIZE_WORD
		mask = 0xFFFF
		sizeCycles = 12
	case M68K_SIZE_LONG: // Long
		size = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
		sizeCycles = 20
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get the destination address
	var destAddr uint32

	if mode == M68K_AM_DR || mode == M68K_AM_AR {
		// CAS doesn't work with registers directly
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	} else {
		destAddr = cpu.GetEffectiveAddress(mode, reg)
	}

	// Read the destination value (with proper size masking)
	var destValue uint32

	switch size {
	case M68K_SIZE_BYTE:
		destValue = uint32(cpu.Read8(destAddr))
	case M68K_SIZE_WORD:
		destValue = uint32(cpu.Read16(destAddr))
	case M68K_SIZE_LONG:
		destValue = cpu.Read32(destAddr)
	}

	// Get comparison value (with proper size masking)
	compareValue := cpu.DataRegs[dcReg] & mask

	// Compare values and set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if compareValue == destValue {
		// Values match, update destination with du
		updateValue := cpu.DataRegs[duReg] & mask

		switch size {
		case M68K_SIZE_BYTE:
			cpu.Write8(destAddr, uint8(updateValue))
		case M68K_SIZE_WORD:
			cpu.Write16(destAddr, uint16(updateValue))
		case M68K_SIZE_LONG:
			cpu.Write32(destAddr, updateValue)
		}

		// Set Z flag (equal)
		cpu.SR |= M68K_SR_Z
	} else {
		// No match, update dc with the destination value
		if size == M68K_SIZE_BYTE || size == M68K_SIZE_WORD {
			// Sign extend for byte/word
			if (destValue & (1 << (mask >> 1))) != 0 {
				if size == M68K_SIZE_BYTE {
					destValue |= 0xFFFFFF00
				} else { // Word
					destValue |= 0xFFFF0000
				}
			}
		}

		cpu.DataRegs[dcReg] = (cpu.DataRegs[dcReg] & ^mask) | (destValue & mask)

		// Set condition codes for the comparison
		result := int32(compareValue) - int32(destValue)

		if result == 0 {
			cpu.SR |= M68K_SR_Z
		} else if result < 0 {
			cpu.SR |= M68K_SR_N
		}

		if compareValue < destValue {
			cpu.SR |= M68K_SR_C
		}

		// Check for overflow in the comparison
		if ((compareValue ^ destValue) & (compareValue ^ uint32(result)) & (1 << (mask >> 1))) != 0 {
			cpu.SR |= M68K_SR_V
		}
	}

	// Calculate accurate cycle count
	// Base cycles + EA calculation time
	cpu.cycleCounter += sizeCycles + cpu.GetEACycles(mode, reg)
}
func (cpu *M68KCPU) ExecCas2(opmode uint16) {
	// Get the two extension words
	ext1 := cpu.Fetch16()
	ext2 := cpu.Fetch16()

	// Extract register numbers from first extension word
	dc1Reg := ext1 & 0x07        // First data to compare
	du1Reg := (ext1 >> 6) & 0x07 // First data to update
	rn1 := (ext1 >> 12) & 0x0F   // First register number

	// Extract register numbers from second extension word
	dc2Reg := ext2 & 0x07        // Second data to compare
	du2Reg := (ext2 >> 6) & 0x07 // Second data to update
	rn2 := (ext2 >> 12) & 0x0F   // Second register number

	// Determine size
	var size int
	var mask uint32
	var sizeCycles uint32

	switch opmode {
	case M68K_SIZE_BYTE: // Byte (not valid for CAS2)
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	case M68K_SIZE_WORD: // Word
		size = M68K_SIZE_WORD
		mask = 0xFFFF
		sizeCycles = 20
	case M68K_SIZE_LONG: // Long
		size = M68K_SIZE_LONG
		mask = 0xFFFFFFFF
		sizeCycles = 30
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return
	}

	// Get the destination addresses from registers
	var addr1, addr2 uint32

	// First address from Rn1
	if (rn1 & 0x08) == 0 {
		addr1 = cpu.DataRegs[rn1&0x07]
	} else {
		addr1 = cpu.AddrRegs[rn1&0x07]
	}

	// Second address from Rn2
	if (rn2 & 0x08) == 0 {
		addr2 = cpu.DataRegs[rn2&0x07]
	} else {
		addr2 = cpu.AddrRegs[rn2&0x07]
	}

	// Read the destination values
	var dest1, dest2 uint32

	switch size {
	case M68K_SIZE_WORD:
		dest1 = uint32(cpu.Read16(addr1))
		dest2 = uint32(cpu.Read16(addr2))
	case M68K_SIZE_LONG:
		dest1 = cpu.Read32(addr1)
		dest2 = cpu.Read32(addr2)
	}

	// Get comparison values
	compare1 := cpu.DataRegs[dc1Reg] & mask
	compare2 := cpu.DataRegs[dc2Reg] & mask

	// Set initial condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	// Compare values
	if compare1 == dest1 && compare2 == dest2 {
		// Both values match, update destinations
		update1 := cpu.DataRegs[du1Reg] & mask
		update2 := cpu.DataRegs[du2Reg] & mask

		switch size {
		case M68K_SIZE_WORD:
			cpu.Write16(addr1, uint16(update1))
			cpu.Write16(addr2, uint16(update2))
		case M68K_SIZE_LONG:
			cpu.Write32(addr1, update1)
			cpu.Write32(addr2, update2)
		}

		// Set Z flag (equal)
		cpu.SR |= M68K_SR_Z
	} else {
		// At least one value doesn't match, update dc registers
		if compare1 != dest1 {
			// First comparison failed
			if size == M68K_SIZE_WORD {
				// Sign extend for word
				if (dest1 & 0x8000) != 0 {
					dest1 |= 0xFFFF0000
				}
			}

			cpu.DataRegs[dc1Reg] = (cpu.DataRegs[dc1Reg] & ^mask) | (dest1 & mask)

			// Set condition codes for the first comparison
			result := int32(compare1) - int32(dest1)

			if result == 0 {
				cpu.SR |= M68K_SR_Z
			} else if result < 0 {
				cpu.SR |= M68K_SR_N
			}

			if compare1 < dest1 {
				cpu.SR |= M68K_SR_C
			}

			// Check for overflow
			if ((compare1 ^ dest1) & (compare1 ^ uint32(result)) & (1 << (mask >> 1))) != 0 {
				cpu.SR |= M68K_SR_V
			}
		} else {
			// First comparison succeeded but second one failed
			if size == M68K_SIZE_WORD {
				// Sign extend for word
				if (dest2 & 0x8000) != 0 {
					dest2 |= 0xFFFF0000
				}
			}

			cpu.DataRegs[dc2Reg] = (cpu.DataRegs[dc2Reg] & ^mask) | (dest2 & mask)

			// Set condition codes for the second comparison
			result := int32(compare2) - int32(dest2)

			if result == 0 {
				cpu.SR |= M68K_SR_Z
			} else if result < 0 {
				cpu.SR |= M68K_SR_N
			}

			if compare2 < dest2 {
				cpu.SR |= M68K_SR_C
			}

			// Check for overflow
			if ((compare2 ^ dest2) & (compare2 ^ uint32(result)) & (1 << (mask >> 1))) != 0 {
				cpu.SR |= M68K_SR_V
			}
		}
	}

	// Calculate accurate cycle count
	// CAS2 has a fixed cycle count but varies by operand size
	cpu.cycleCounter += sizeCycles
}
func (cpu *M68KCPU) ExecTas(mode, reg uint16) {
	var value uint8
	var addr uint32

	// Get operand
	if mode == M68K_AM_DR {
		// Data register direct
		value = uint8(cpu.DataRegs[reg] & 0xFF)

		// Set condition codes based on original value
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (value & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}

		// Set the high bit
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | uint32(value|0x80)
		cpu.cycleCounter += M68K_CYCLE_TAS_R
	} else {
		// Memory operand
		addr = cpu.GetEffectiveAddress(mode, reg)
		value = cpu.Read8(addr)

		// Set condition codes based on original value
		cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
		if value == 0 {
			cpu.SR |= M68K_SR_Z
		}
		if (value & 0x80) != 0 {
			cpu.SR |= M68K_SR_N
		}

		// Set the high bit and write back
		cpu.Write8(addr, value|0x80)
		cpu.cycleCounter += M68K_CYCLE_TAS_M
	}
}

// Bit field operations
type BFParameters struct {
	offset      uint32
	width       uint32
	destReg     uint16
	byteOffset  uint32
	bitOffset   uint32
	bytesToRead uint32
	mask        uint32
}

func (cpu *M68KCPU) GetBitFieldParameters(ext uint16) BFParameters {
	var params BFParameters

	// Extract destination register if present
	params.destReg = (ext >> 12) & 0x07

	// Get offset (from register or immediate)
	if (ext & M68K_BF_OFFSET_REG) != 0 {
		offsetReg := (ext & M68K_BF_OFFSET_MASK) >> 6
		params.offset = cpu.DataRegs[offsetReg] & 0x1F
	} else {
		params.offset = uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	}

	// Get width (from register or immediate)
	if (ext & M68K_BF_WIDTH_REG) != 0 {
		widthReg := ext & 0x0007
		params.width = cpu.DataRegs[widthReg] & 0x1F
		if params.width == 0 {
			params.width = 32
		}
	} else {
		params.width = uint32(ext & M68K_BF_WIDTH_MASK)
		if params.width == 0 {
			params.width = 32
		}
	}

	// Calculate byte offset and bit position
	params.byteOffset = params.offset / 8
	params.bitOffset = params.offset % 8

	// Determine how many bytes to read
	params.bytesToRead = min(((params.bitOffset + params.width + 7) / 8), M68K_LONG_SIZE)

	// Create a mask for the field
	if params.width == 32 {
		params.mask = 0xFFFFFFFF
	} else {
		params.mask = (1 << params.width) - 1
	}

	return params
}
func (cpu *M68KCPU) ReadBitField(addr uint32, params *BFParameters) (fieldData uint32, originalField uint32) {
	// Read bytes
	switch params.bytesToRead {
	case 1:
		fieldData = uint32(cpu.Read8(addr + params.byteOffset))
	case 2:
		fieldData = uint32(cpu.Read16(addr + params.byteOffset))
	case 3:
		fieldData = uint32(cpu.Read16(addr+params.byteOffset)) << 8
		fieldData |= uint32(cpu.Read8(addr + params.byteOffset + 2))
	case 4:
		fieldData = cpu.Read32(addr + params.byteOffset)
	}

	// Extract the field value for condition code setting
	originalField = (fieldData >> params.bitOffset) & params.mask

	return fieldData, originalField
}
func (cpu *M68KCPU) WriteBitField(addr uint32, params *BFParameters, fieldData uint32) {
	// Write the modified data back to memory
	switch params.bytesToRead {
	case 1:
		cpu.Write8(addr+params.byteOffset, uint8(fieldData))
	case 2:
		cpu.Write16(addr+params.byteOffset, uint16(fieldData))
	case 3:
		cpu.Write16(addr+params.byteOffset, uint16(fieldData>>8))
		cpu.Write8(addr+params.byteOffset+2, uint8(fieldData))
	case 4:
		cpu.Write32(addr+params.byteOffset, fieldData)
	}
}
func (cpu *M68KCPU) SetBitFieldFlags(originalField uint32, width uint32) {
	// Set condition codes
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)

	if originalField == 0 {
		cpu.SR |= M68K_SR_Z
	}

	if width > 0 && ((originalField>>(width-1))&1) != 0 {
		cpu.SR |= M68K_SR_N
	}
}

type BitFieldOperation int

const (
	BFTST  BitFieldOperation = iota // Test bit field
	BFEXTU                          // Extract bit field unsigned
	BFEXTS                          // Extract bit field signed
	BFFFO                           // Find first one in bit field
	BFINS                           // Insert bit field
	BFSET                           // Set bit field
	BFCLR                           // Clear bit field
	BFCHG                           // Change bit field
)

func (cpu *M68KCPU) ExecBitField(operation BitFieldOperation, mode, reg uint16) {
	// Get extension word
	ext := cpu.Fetch16()

	// Get bit field parameters
	params := cpu.GetBitFieldParameters(ext)

	var addr uint32
	var fieldData, originalField uint32
	var effectiveBitOffset uint32 // Shift amount for mask operations
	isRegisterOperand := (mode == M68K_AM_DR)

	if isRegisterOperand {
		// For data register operands, work directly with the 32-bit register value
		// Bit field offset is numbered from MSB: bit 31 is offset 0, bit 0 is offset 31
		// Offset is modulo 32 for register operands
		regValue := cpu.DataRegs[reg]
		offset := params.offset % 32
		width := params.width
		if width == 0 {
			width = 32
		}

		// For registers, we extract bits starting from (31 - offset) going right
		// The extracted field is right-aligned (LSB-justified)
		// shiftAmount is how far right we need to shift to get the field to bit 0
		var shiftAmount uint32
		if offset+width <= 32 {
			shiftAmount = 32 - offset - width
		} else {
			// Field wraps around (offset + width > 32) - handle wrap case
			shiftAmount = 0
		}
		effectiveBitOffset = shiftAmount

		if width >= 32 {
			originalField = regValue
		} else {
			originalField = (regValue >> shiftAmount) & params.mask
		}
		fieldData = regValue
	} else {
		// Calculate effective address for memory operands
		addr = cpu.GetEffectiveAddress(mode, reg)

		// Read bit field from memory
		fieldData, originalField = cpu.ReadBitField(addr, &params)
		effectiveBitOffset = uint32(params.bitOffset)
	}

	// Track if memory/register needs to be updated
	needsUpdate := false

	// Operation-specific processing
	switch operation {
	case BFTST:
		// Test only - no modification

	case BFEXTU:
		// Extract unsigned - store to destination register
		cpu.DataRegs[params.destReg] = originalField

	case BFEXTS:
		// Extract signed - store to destination register with sign extension
		extractedField := originalField
		if params.width > 0 && params.width < 32 && ((originalField>>(params.width-1))&1) != 0 {
			// Set all bits above width to 1
			extractedField |= ^((1 << params.width) - 1)
		}
		cpu.DataRegs[params.destReg] = extractedField

	case BFFFO:
		// Find first one - search from MSB to LSB (M68K bit field semantics)
		// In M68K, bit field offset 0 = MSB, so we scan from MSB toward LSB
		var firstOne uint32 = 0
		var foundOne bool = false

		for i := uint32(0); i < params.width; i++ {
			bitPos := params.width - 1 - i // Start from MSB of extracted field
			if ((originalField >> bitPos) & 1) != 0 {
				firstOne = i
				foundOne = true
				break
			}
		}

		// Store result
		if foundOne {
			cpu.DataRegs[params.destReg] = params.offset + firstOne
		} else {
			cpu.DataRegs[params.destReg] = params.offset + params.width
		}

	case BFINS:
		// Insert - replace bit field with data from destination register
		insertData := cpu.DataRegs[params.destReg] & params.mask

		// Clear the bits in the original data
		clearMask := params.mask << effectiveBitOffset
		fieldData &= ^clearMask

		// Insert the new bits
		fieldData |= (insertData << effectiveBitOffset)
		needsUpdate = true

		// Set flags based on the inserted data
		originalField = insertData

	case BFSET:
		// Set all bits in the field
		setMask := params.mask << effectiveBitOffset
		fieldData |= setMask
		needsUpdate = true

	case BFCLR:
		// Clear all bits in the field
		clearMask := params.mask << effectiveBitOffset
		fieldData &= ^clearMask
		needsUpdate = true

	case BFCHG:
		// Invert all bits in the field
		changeMask := params.mask << effectiveBitOffset
		fieldData ^= changeMask
		needsUpdate = true
	}

	// Write back if needed
	if needsUpdate {
		if isRegisterOperand {
			cpu.DataRegs[reg] = fieldData
		} else {
			cpu.WriteBitField(addr, &params, fieldData)
		}
	}

	// Set common flags
	cpu.SetBitFieldFlags(originalField, params.width)

	// Cycle counting
	baseCycles := uint32(M68K_CYCLE_BRANCH)
	if operation == BFINS {
		baseCycles += M68K_CYCLE_REG*7 + M68K_CYCLE_MEM_READ + M68K_CYCLE_MEM_WRITE
	} else if operation == BFFFO {
		baseCycles += M68K_CYCLE_BRANCH
	} else if operation == BFEXTU || operation == BFEXTS {
		baseCycles += M68K_CYCLE_REG * 6
	} else if needsUpdate {
		baseCycles += M68K_CYCLE_REG * 6
	}

	cpu.cycleCounter += baseCycles + cpu.GetEACycles(mode, reg)
}
func (cpu *M68KCPU) ExecBFTST(mode, reg uint16) {
	cpu.ExecBitField(BFTST, mode, reg)
}
func (cpu *M68KCPU) ExecBFEXTU(mode, reg uint16) {
	cpu.ExecBitField(BFEXTU, mode, reg)
}
func (cpu *M68KCPU) ExecBFEXTS(mode, reg uint16) {
	cpu.ExecBitField(BFEXTS, mode, reg)
}
func (cpu *M68KCPU) ExecBFFO(mode, reg uint16) {
	cpu.ExecBitField(BFFFO, mode, reg)
}
func (cpu *M68KCPU) ExecBFINS(mode, reg uint16) {
	cpu.ExecBitField(BFINS, mode, reg)
}
func (cpu *M68KCPU) ExecBFSET(mode, reg uint16) {
	cpu.ExecBitField(BFSET, mode, reg)
}
func (cpu *M68KCPU) ExecBFCLR(mode, reg uint16) {
	cpu.ExecBitField(BFCLR, mode, reg)
}
func (cpu *M68KCPU) ExecBFCHG(mode, reg uint16) {
	cpu.ExecBitField(BFCHG, mode, reg)
}

func btoi(b bool) uint8 {
	// btoi converts bool to uint8 (0 or 1)
	if b {
		return 1
	}
	return 0
}

// =============================================================================
// FPU Instructions (68881/68882 Coprocessor)
// =============================================================================

// FPU command word operation codes
const (
	FPU_OP_FMOVE   = 0x00
	FPU_OP_FINT    = 0x01
	FPU_OP_FSINH   = 0x02
	FPU_OP_FINTRZ  = 0x03
	FPU_OP_FSQRT   = 0x04
	FPU_OP_FLOGNP1 = 0x06
	FPU_OP_FETOXM1 = 0x08
	FPU_OP_FTANH   = 0x09
	FPU_OP_FATAN   = 0x0A
	FPU_OP_FASIN   = 0x0C
	FPU_OP_FATANH  = 0x0D
	FPU_OP_FSIN    = 0x0E
	FPU_OP_FTAN    = 0x0F
	FPU_OP_FETOX   = 0x10
	FPU_OP_FTWOTOX = 0x11
	FPU_OP_FTENTOX = 0x12
	FPU_OP_FLOGN   = 0x14
	FPU_OP_FLOG10  = 0x15
	FPU_OP_FLOG2   = 0x16
	FPU_OP_FABS    = 0x18
	FPU_OP_FCOSH   = 0x19
	FPU_OP_FNEG    = 0x1A
	FPU_OP_FACOS   = 0x1C
	FPU_OP_FCOS    = 0x1D
	FPU_OP_FGETEXP = 0x1E
	FPU_OP_FGETMAN = 0x1F
	FPU_OP_FDIV    = 0x20
	FPU_OP_FMOD    = 0x21
	FPU_OP_FADD    = 0x22
	FPU_OP_FMUL    = 0x23
	FPU_OP_FSGLDIV = 0x24
	FPU_OP_FREM    = 0x25
	FPU_OP_FSCALE  = 0x26
	FPU_OP_FSGLMUL = 0x27
	FPU_OP_FSUB    = 0x28
	FPU_OP_FCMP    = 0x38
	FPU_OP_FTST    = 0x3A
)

var fpuOpTable = func() [128]func(*M68881FPU, int, int) {
	var table [128]func(*M68881FPU, int, int)
	table[FPU_OP_FMOVE] = (*M68881FPU).FMOVE_RegToReg
	table[FPU_OP_FINT] = (*M68881FPU).FINT
	table[FPU_OP_FSINH] = (*M68881FPU).FSINH
	table[FPU_OP_FINTRZ] = (*M68881FPU).FINTRZ
	table[FPU_OP_FSQRT] = (*M68881FPU).FSQRT
	table[FPU_OP_FTANH] = (*M68881FPU).FTANH
	table[FPU_OP_FATAN] = (*M68881FPU).FATAN
	table[FPU_OP_FASIN] = (*M68881FPU).FASIN
	table[FPU_OP_FATANH] = (*M68881FPU).FATANH
	table[FPU_OP_FSIN] = (*M68881FPU).FSIN
	table[FPU_OP_FTAN] = (*M68881FPU).FTAN
	table[FPU_OP_FETOX] = (*M68881FPU).FETOX
	table[FPU_OP_FTWOTOX] = (*M68881FPU).FTWOTOX
	table[FPU_OP_FTENTOX] = (*M68881FPU).FTENTOX
	table[FPU_OP_FLOGN] = (*M68881FPU).FLOGN
	table[FPU_OP_FLOG10] = (*M68881FPU).FLOG10
	table[FPU_OP_FLOG2] = (*M68881FPU).FLOG2
	table[FPU_OP_FABS] = (*M68881FPU).FABS
	table[FPU_OP_FCOSH] = (*M68881FPU).FCOSH
	table[FPU_OP_FNEG] = (*M68881FPU).FNEG
	table[FPU_OP_FACOS] = (*M68881FPU).FACOS
	table[FPU_OP_FCOS] = (*M68881FPU).FCOS
	table[FPU_OP_FGETEXP] = (*M68881FPU).FGETEXP
	table[FPU_OP_FGETMAN] = (*M68881FPU).FGETMAN
	table[FPU_OP_FDIV] = (*M68881FPU).FDIV
	table[FPU_OP_FMOD] = (*M68881FPU).FMOD
	table[FPU_OP_FADD] = (*M68881FPU).FADD
	table[FPU_OP_FMUL] = (*M68881FPU).FMUL
	table[FPU_OP_FSGLDIV] = (*M68881FPU).FSGLDIV
	table[FPU_OP_FREM] = (*M68881FPU).FREM
	table[FPU_OP_FSCALE] = (*M68881FPU).FSCALE
	table[FPU_OP_FSGLMUL] = (*M68881FPU).FSGLMUL
	table[FPU_OP_FSUB] = (*M68881FPU).FSUB
	table[FPU_OP_FCMP] = (*M68881FPU).FCMP
	table[FPU_OP_FTST] = func(fpu *M68881FPU, src, _ int) { fpu.FTST(src) }
	return table
}()

// ExecFPUInstruction decodes and executes an FPU instruction
func (cpu *M68KCPU) ExecFPUInstruction(opcode uint16) {
	// Store instruction address for FPIAR
	cpu.FPU.FPIAR = cpu.PC - M68K_WORD_SIZE

	// Type field determines instruction class
	typeField := (opcode >> 6) & 0x7

	switch typeField {
	case 0:
		// General FPU operations: FPm to FPn or special
		cmdWord := cpu.Fetch16()
		cpu.execFPUGeneral(cmdWord)

	case 1:
		// FMOVE to FP register from memory (various formats)
		cmdWord := cpu.Fetch16()
		cpu.execFPUMemToReg(opcode, cmdWord)

	case 2:
		// FPU operations with memory source
		cmdWord := cpu.Fetch16()
		cpu.execFPUMemOp(opcode, cmdWord)

	case 3:
		// FMOVE from FP register to memory
		cmdWord := cpu.Fetch16()
		cpu.execFPURegToMem(opcode, cmdWord)

	case 4:
		// FMOVEM - Move multiple FP registers
		cmdWord := cpu.Fetch16()
		cpu.execFMOVEM(opcode, cmdWord)

	case 5:
		// FMOVEM control registers
		cmdWord := cpu.Fetch16()
		cpu.execFMOVEMControl(opcode, cmdWord)

	case 6, 7:
		// Conditional branch/set
		cpu.execFPUConditional(opcode)

	default:
		cpu.ProcessException(M68K_VEC_LINE_F)
	}
}

// execFPUGeneral handles register-to-register FPU operations
func (cpu *M68KCPU) execFPUGeneral(cmdWord uint16) {
	// Check for FMOVECR first - it has a special encoding (010111 in bits 15-10)
	if (cmdWord & 0xFC00) == 0x5C00 {
		dstReg := int((cmdWord >> 7) & 0x7)
		romAddr := uint8(cmdWord & 0x7F)
		cpu.FPU.FMOVECR(romAddr, dstReg)
		return
	}

	// Format: R/M=0, src=FP reg (bits 12-10), dst=FP reg (bits 9-7), opcode (bits 6-0)
	srcReg := int((cmdWord >> 10) & 0x7)
	dstReg := int((cmdWord >> 7) & 0x7)
	op := cmdWord & 0x7F

	if fn := fpuOpTable[op]; fn != nil {
		fn(cpu.FPU, srcReg, dstReg)
	} else {
		cpu.ProcessException(M68K_VEC_LINE_F)
	}
}

func (cpu *M68KCPU) readExtendedReal96(ea uint32) ExtendedReal {
	signExp := cpu.Read16(ea)
	mantHi := cpu.Read32(ea + 4)
	mantLo := cpu.Read32(ea + 8)
	return ExtendedReal{
		Sign: uint8(signExp >> 15),
		Exp:  signExp & 0x7FFF,
		Mant: (uint64(mantHi) << 32) | uint64(mantLo),
	}
}

func (cpu *M68KCPU) writeExtendedReal96(ea uint32, ext ExtendedReal) {
	signExp := (uint16(ext.Sign&1) << 15) | (ext.Exp & 0x7FFF)
	cpu.Write16(ea, signExp)
	cpu.Write16(ea+2, 0) // Reserved/padding
	cpu.Write32(ea+4, uint32(ext.Mant>>32))
	cpu.Write32(ea+8, uint32(ext.Mant))
}

// execFPUMemToReg handles loading FP register from memory
func (cpu *M68KCPU) execFPUMemToReg(opcode, cmdWord uint16) {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	dstReg := int((cmdWord >> 7) & 0x7)
	srcFormat := (cmdWord >> 10) & 0x7

	// Calculate effective address
	ea := cpu.GetEffectiveAddress(mode, reg)

	var value float64

	switch srcFormat {
	case 0: // Long integer
		intVal := int32(cpu.Read32(ea))
		value = float64(intVal)
	case 1: // Single precision
		bits := cpu.Read32(ea)
		value = float64(math.Float32frombits(bits))
	case 2: // Extended precision (96-bit storage with 80-bit payload)
		ext := cpu.readExtendedReal96(ea)
		cpu.FPU.SetFromExtendedReal(dstReg, ext)
		cpu.FPU.setCC64(cpu.FPU.GetFP64(dstReg))
		return
	case 3: // Packed decimal (not implemented)
		value = 0.0
	case 4: // Word integer
		intVal := int16(cpu.Read16(ea))
		value = float64(intVal)
	case 5: // Double precision
		hi := cpu.Read32(ea)
		lo := cpu.Read32(ea + 4)
		bits := uint64(hi)<<32 | uint64(lo)
		value = math.Float64frombits(bits)
	case 6: // Byte integer
		intVal := int8(cpu.Read8(ea))
		value = float64(intVal)
	default:
		value = 0.0
	}

	cpu.FPU.SetFP64(dstReg, value)
	cpu.FPU.setCC64(value)
}

// execFPUMemOp handles FPU operations with memory source
func (cpu *M68KCPU) execFPUMemOp(opcode, cmdWord uint16) {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	dstReg := int((cmdWord >> 7) & 0x7)
	srcFormat := (cmdWord >> 10) & 0x7
	op := cmdWord & 0x7F

	// Calculate effective address
	ea := cpu.GetEffectiveAddress(mode, reg)

	var value float64

	switch srcFormat {
	case 0: // Long integer
		intVal := int32(cpu.Read32(ea))
		value = float64(intVal)
	case 1: // Single precision
		bits := cpu.Read32(ea)
		value = float64(math.Float32frombits(bits))
	case 4: // Word integer
		intVal := int16(cpu.Read16(ea))
		value = float64(intVal)
	case 5: // Double precision
		hi := cpu.Read32(ea)
		lo := cpu.Read32(ea + 4)
		bits := uint64(hi)<<32 | uint64(lo)
		value = math.Float64frombits(bits)
	case 6: // Byte integer
		intVal := int8(cpu.Read8(ea))
		value = float64(intVal)
	default:
		value = 0.0
	}

	// Perform operation
	switch op {
	case FPU_OP_FADD:
		cpu.FPU.AddImm(dstReg, value)
	case FPU_OP_FSUB:
		cpu.FPU.SubImm(dstReg, value)
	case FPU_OP_FMUL:
		cpu.FPU.MulImm(dstReg, value)
	case FPU_OP_FDIV:
		cpu.FPU.DivImm(dstReg, value)
	case FPU_OP_FCMP:
		cpu.FPU.CmpImm(dstReg, value)
		return
	case FPU_OP_FMOVE:
		cpu.FPU.MoveImm(dstReg, value)
	default:
		cpu.ProcessException(M68K_VEC_LINE_F)
		return
	}
}

// execFPURegToMem handles storing FP register to memory
func (cpu *M68KCPU) execFPURegToMem(opcode, cmdWord uint16) {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	srcReg := int((cmdWord >> 7) & 0x7)
	dstFormat := (cmdWord >> 10) & 0x7

	// Calculate effective address
	ea := cpu.GetEffectiveAddress(mode, reg)

	value := cpu.FPU.GetFP64(srcReg)

	switch dstFormat {
	case 0: // Long integer
		cpu.Write32(ea, uint32(int32(value)))
	case 1: // Single precision
		bits := math.Float32bits(float32(value))
		cpu.Write32(ea, bits)
	case 2: // Extended precision (96-bit storage with 80-bit payload)
		cpu.writeExtendedReal96(ea, cpu.FPU.GetExtendedReal(srcReg))
	case 4: // Word integer
		cpu.Write16(ea, uint16(int16(value)))
	case 5: // Double precision
		bits := math.Float64bits(value)
		cpu.Write32(ea, uint32(bits>>32))
		cpu.Write32(ea+4, uint32(bits))
	case 6: // Byte integer
		cpu.Write8(ea, uint8(int8(value)))
	default:
		// Unsupported format
	}
}

// execFMOVEM handles moving multiple FP registers
func (cpu *M68KCPU) execFMOVEM(opcode, cmdWord uint16) {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	direction := (cmdWord >> 13) & 0x1 // 0=to memory, 1=from memory
	regList := cmdWord & 0xFF

	// Calculate effective address
	ea := cpu.GetEffectiveAddress(mode, reg)

	if direction == 0 {
		// FP registers to memory
		for i := range 8 {
			if (regList & (1 << (7 - i))) != 0 {
				cpu.writeExtendedReal96(ea, cpu.FPU.GetExtendedReal(i))
				ea += 12 // Extended precision takes 12 bytes
			}
		}
	} else {
		// Memory to FP registers
		for i := range 8 {
			if (regList & (1 << (7 - i))) != 0 {
				cpu.FPU.SetFromExtendedReal(i, cpu.readExtendedReal96(ea))
				ea += 12
			}
		}
	}
}

// execFMOVEMControl handles moving FPU control registers
func (cpu *M68KCPU) execFMOVEMControl(opcode, cmdWord uint16) {
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7
	direction := (cmdWord >> 13) & 0x1 // 0=to memory, 1=from memory
	regList := (cmdWord >> 10) & 0x7   // FPCR, FPSR, FPIAR

	ea := cpu.GetEffectiveAddress(mode, reg)

	if direction == 0 {
		// Control registers to memory/data register
		if (regList & 0x4) != 0 { // FPCR
			cpu.Write32(ea, cpu.FPU.FPCR)
			ea += 4
		}
		if (regList & 0x2) != 0 { // FPSR
			cpu.Write32(ea, cpu.FPU.FPSR)
			ea += 4
		}
		if (regList & 0x1) != 0 { // FPIAR
			cpu.Write32(ea, cpu.FPU.FPIAR)
		}
	} else {
		// Memory/data register to control registers
		if (regList & 0x4) != 0 { // FPCR
			cpu.FPU.FPCR = cpu.Read32(ea)
			ea += 4
		}
		if (regList & 0x2) != 0 { // FPSR
			cpu.FPU.FPSR = cpu.Read32(ea)
			ea += 4
		}
		if (regList & 0x1) != 0 { // FPIAR
			cpu.FPU.FPIAR = cpu.Read32(ea)
		}
	}
}

// execFPUConditional handles FPU conditional instructions (FBcc, FScc, FDBcc)
func (cpu *M68KCPU) execFPUConditional(opcode uint16) {
	condCode := opcode & 0x3F

	// Evaluate FPU condition
	condTrue := cpu.evalFPUCondition(condCode)

	if (opcode & 0x0040) != 0 {
		// FScc - Set byte on condition
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		ea := cpu.GetEffectiveAddress(mode, reg)
		if condTrue {
			cpu.Write8(ea, 0xFF)
		} else {
			cpu.Write8(ea, 0x00)
		}
	} else {
		// FBcc - Branch on condition
		displacement := int16(cpu.Fetch16())
		if condTrue {
			cpu.PC = uint32(int32(cpu.PC) + int32(displacement) - M68K_WORD_SIZE)
		}
	}
}

// evalFPUCondition evaluates an FPU condition code
func (cpu *M68KCPU) evalFPUCondition(condCode uint16) bool {
	n := cpu.FPU.GetConditionN()
	z := cpu.FPU.GetConditionZ()
	nan := cpu.FPU.GetConditionNAN()

	switch condCode & 0x0F {
	case 0x00: // F - False
		return false
	case 0x01: // EQ - Equal
		return z
	case 0x02: // OGT - Ordered Greater Than
		return !nan && !z && !n
	case 0x03: // OGE - Ordered Greater or Equal
		return !nan && (z || !n)
	case 0x04: // OLT - Ordered Less Than
		return !nan && !z && n
	case 0x05: // OLE - Ordered Less or Equal
		return !nan && (z || n)
	case 0x06: // OGL - Ordered Greater or Less
		return !nan && !z
	case 0x07: // OR - Ordered
		return !nan
	case 0x08: // UN - Unordered
		return nan
	case 0x09: // UEQ - Unordered or Equal
		return nan || z
	case 0x0A: // UGT - Unordered or Greater Than
		return nan || (!z && !n)
	case 0x0B: // UGE - Unordered or Greater or Equal
		return nan || z || !n
	case 0x0C: // ULT - Unordered or Less Than
		return nan || (!z && n)
	case 0x0D: // ULE - Unordered or Less or Equal
		return nan || z || n
	case 0x0E: // NE - Not Equal
		return !z
	case 0x0F: // T - True
		return true
	}

	return false
}
