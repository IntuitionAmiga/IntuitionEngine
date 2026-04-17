// cpu_ie64.go - Intuition Engine 64-bit RISC CPU

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
cpu_ie64.go - 64-bit RISC CPU for the Intuition Engine

This module implements a 64-bit RISC load-store CPU with a clean, regular
instruction encoding. The architecture uses compare-and-branch instead of
flags, 32 general-purpose 64-bit registers (R0 hardwired to zero), and
fixed 8-byte instructions.

Core Features:
- 32 general-purpose 64-bit registers (R0 hardwired zero)
- R31 aliases as stack pointer (SP)
- Fixed 8-byte instruction encoding
- Compare-and-branch (no flags register)
- Load-store architecture
- Size-annotated operations (.B, .W, .L, .Q)
- Hardware timer with interrupt support
- PC masked to 32MB address space

Instruction Encoding (8 bytes, little-endian):
  Byte 0: Opcode      (8 bits)
  Byte 1: Rd[4:0]     (5 bits) | Size[1:0] (2 bits) | X (1 bit)
  Byte 2: Rs[4:0]     (5 bits) | unused    (3 bits)
  Byte 3: Rt[4:0]     (5 bits) | unused    (3 bits)
  Bytes 4-7: imm32    (32-bit LE)

Thread Safety:
- running, debug use atomic.Bool for lock-free cross-thread signalling
- Timer fields use atomic types for lock-free access
- Interrupt fields use atomic types
- Execute() loop uses local running flag with periodic atomic check
*/

package main

import (
	"fmt"
	"math"
	"math/bits"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ------------------------------------------------------------------------------
// IE64 Instruction Size
// ------------------------------------------------------------------------------
const (
	IE64_INSTR_SIZE = 8 // Fixed 8-byte instructions
)

// ------------------------------------------------------------------------------
// IE64 Size Codes
// ------------------------------------------------------------------------------
const (
	IE64_SIZE_B = 0 // Byte (8-bit)
	IE64_SIZE_W = 1 // Word (16-bit)
	IE64_SIZE_L = 2 // Long (32-bit)
	IE64_SIZE_Q = 3 // Quad (64-bit)
)

// ------------------------------------------------------------------------------
// IE64 Address Space
// ------------------------------------------------------------------------------
const (
	IE64_ADDR_MASK = 0x1FFFFFF // 32MB address space mask
)

// ------------------------------------------------------------------------------
// IE64 MMU Constants
// ------------------------------------------------------------------------------
const (
	MMU_PAGE_SIZE  = 0x1000                               // 4 KiB virtual pages
	MMU_PAGE_SHIFT = 12                                   // log2(MMU_PAGE_SIZE)
	MMU_PAGE_MASK  = MMU_PAGE_SIZE - 1                    // offset mask within page
	MMU_NUM_PAGES  = (IE64_ADDR_MASK + 1) / MMU_PAGE_SIZE // 8192 pages in 32MB
)

// PTE permission bits (64-bit page table entry)
const (
	PTE_P = 1 << 0 // Present
	PTE_R = 1 << 1 // Read permission
	PTE_W = 1 << 2 // Write permission
	PTE_X = 1 << 3 // Execute permission (0 = non-executable / NX)
	PTE_U = 1 << 4 // User-accessible (0 = supervisor only)
	PTE_A = 1 << 5 // Accessed (set by hardware on any access)
	PTE_D = 1 << 6 // Dirty (set by hardware on write)
)

// PTE physical page number field
const (
	PTE_PPN_SHIFT = 13     // PPN starts at bit 13
	PTE_PPN_MASK  = 0x1FFF // 13-bit PPN (8192 pages)
)

// Control register indices for MTCR/MFCR
const (
	CR_PTBR         = 0  // Page Table Base Register (physical address)
	CR_FAULT_ADDR   = 1  // Virtual address that caused last fault
	CR_FAULT_CAUSE  = 2  // Fault cause code
	CR_FAULT_PC     = 3  // PC saved at trap entry
	CR_TRAP_VEC     = 4  // Trap handler vector address
	CR_MMU_CTRL     = 5  // See MMU_CTRL_* bit constants below
	CR_TP           = 6  // Thread Pointer (user-readable, supervisor-writable)
	CR_INTR_VEC     = 7  // Interrupt vector (MMU-mode timer interrupts)
	CR_KSP          = 8  // Kernel Stack Pointer (auto-swap on user→supervisor)
	CR_TIMER_PERIOD = 9  // Timer reload period (instruction cycles)
	CR_TIMER_COUNT  = 10 // Current timer countdown
	CR_TIMER_CTRL   = 11 // Bit 0 = timer enable, Bit 1 = interrupt enable
	CR_USP          = 12 // Saved User Stack Pointer (for context switch)
	CR_PREV_MODE    = 13 // Previous privilege mode (0=user, 1=supervisor) saved by trapEntry
	CR_SAVED_SUA    = 14 // Saved SUA latch (stashed on trap entry; mirrors FAULT_PC for save/restore discipline)
	CR_COUNT        = 15 // Number of control registers
)

// Fault cause codes
const (
	FAULT_NOT_PRESENT  = 0  // PTE P bit = 0
	FAULT_READ_DENIED  = 1  // PTE R bit = 0 on read access
	FAULT_WRITE_DENIED = 2  // PTE W bit = 0 on write access
	FAULT_EXEC_DENIED  = 3  // PTE X bit = 0 on instruction fetch
	FAULT_USER_SUPER   = 4  // PTE U bit = 0 in user mode
	FAULT_PRIV         = 5  // Privileged instruction in user mode
	FAULT_SYSCALL      = 6  // SYSCALL instruction
	FAULT_MISALIGNED   = 7  // Misaligned atomic access
	FAULT_TIMER        = 8  // Timer interrupt (via INTR_VEC)
	FAULT_SKEF         = 9  // Supervisor instruction fetch from user page (SKEF set, PTE_U=1)
	FAULT_SKAC         = 10 // Supervisor data access to user page outside SUA region (SKAC set, PTE_U=1, SUA=0)
)

// CR_MMU_CTRL bit constants.
//
// Bit 0 (MMU_CTRL_ENABLE) and bit 1 (MMU_CTRL_SUPER) are the legacy bits
// established by M13. Bits 2–4 (SKEF / SKAC / SUA) are the M15.6 G2
// SMEP/SMAP-equivalent controls.
//
//   - SKEF / SKAC are enable bits: the kernel flips them via MTCR.
//   - SUA is a latch: it is writable only by the privileged SUAEN /
//     SUADIS instructions. MTCR to CR_MMU_CTRL ignores attempts to set
//     or clear SUA. The latch is saved on trap entry into CR_SAVED_SUA
//     and restored from it on ERET (supervisor return); user-mode ERET
//     clears SUA unconditionally.
//
// Nested-trap discipline (matches FAULT_PC / PREV_MODE): trap entry
// overwrites CR_SAVED_SUA with the current latch. Kernel handlers that
// can take a nested synchronous trap must MFCR CR_SAVED_SUA into a GPR
// on entry, save it to the kernel stack, and MTCR it back before ERET —
// the same discipline already used for CR_FAULT_PC. A single-slot save
// is sufficient when this discipline is followed; skipping it loses the
// outer value on the next nested trap.
const (
	MMU_CTRL_ENABLE = 1 << 0 // Bit 0: MMU translation active (RW, supervisor)
	MMU_CTRL_SUPER  = 1 << 1 // Bit 1: supervisor mode (RO)
	MMU_CTRL_SKEF   = 1 << 2 // Bit 2: Supervisor-Kernel-Execute-Fault enable (RW, supervisor)
	MMU_CTRL_SKAC   = 1 << 3 // Bit 3: Supervisor-Kernel-Access-Check enable (RW, supervisor)
	MMU_CTRL_SUA    = 1 << 4 // Bit 4: Supervisor-User-Access latch (RO via MTCR; set by SUAEN, cleared by SUADIS)
)

// Memory access types for translateAddr
const (
	ACCESS_READ  = 0
	ACCESS_WRITE = 1
	ACCESS_EXEC  = 2
)

// ------------------------------------------------------------------------------
// IE64 Opcodes
// ------------------------------------------------------------------------------
const (
	// Data Movement
	OP_MOVE  = 0x01 // Move register/immediate to register
	OP_MOVT  = 0x02 // Move to upper 32 bits
	OP_MOVEQ = 0x03 // Move with sign-extend 32 to 64
	OP_LEA   = 0x04 // Load effective address

	// Memory Access
	OP_LOAD  = 0x10 // Load from memory
	OP_STORE = 0x11 // Store to memory

	// Arithmetic
	OP_ADD   = 0x20 // Add
	OP_SUB   = 0x21 // Subtract
	OP_MULU  = 0x22 // Multiply unsigned
	OP_MULS  = 0x23 // Multiply signed
	OP_DIVU  = 0x24 // Divide unsigned
	OP_DIVS  = 0x25 // Divide signed
	OP_MOD64 = 0x26 // Modulo
	OP_NEG   = 0x27 // Negate
	OP_MODS  = 0x28 // Signed modulo
	OP_MULHU = 0x29 // Unsigned high multiply
	OP_MULHS = 0x2A // Signed high multiply

	// Logic
	OP_AND64  = 0x30 // Bitwise AND
	OP_OR64   = 0x31 // Bitwise OR
	OP_EOR    = 0x32 // Bitwise XOR
	OP_NOT64  = 0x33 // Bitwise NOT
	OP_LSL    = 0x34 // Logical shift left
	OP_LSR    = 0x35 // Logical shift right
	OP_ASR    = 0x36 // Arithmetic shift right
	OP_CLZ    = 0x37 // Count leading zeros
	OP_SEXT   = 0x38 // Sign extend
	OP_ROL    = 0x39 // Rotate left
	OP_ROR    = 0x3A // Rotate right
	OP_CTZ    = 0x3B // Count trailing zeros
	OP_POPCNT = 0x3C // Population count
	OP_BSWAP  = 0x3D // Byte swap (32-bit)

	// Branches (compare-and-branch, PC-relative)
	OP_BRA = 0x40 // Branch always
	OP_BEQ = 0x41 // Branch if equal (Rs == Rt)
	OP_BNE = 0x42 // Branch if not equal (Rs != Rt)
	OP_BLT = 0x43 // Branch if less than (signed)
	OP_BGE = 0x44 // Branch if greater or equal (signed)
	OP_BGT = 0x45 // Branch if greater than (signed)
	OP_BLE = 0x46 // Branch if less or equal (signed)
	OP_BHI = 0x47 // Branch if higher (unsigned)
	OP_BLS = 0x48 // Branch if lower or same (unsigned)
	OP_JMP = 0x49 // Jump register-indirect

	// Subroutine / Stack
	OP_JSR64   = 0x50 // Jump to subroutine (PC-relative)
	OP_RTS64   = 0x51 // Return from subroutine
	OP_PUSH64  = 0x52 // Push register to stack
	OP_POP64   = 0x53 // Pop from stack to register
	OP_JSR_IND = 0x54 // JSR register-indirect

	// Floating Point (FPU)
	OP_FMOV    = 0x60 // FP reg copy
	OP_FLOAD   = 0x61 // Memory -> FP reg
	OP_FSTORE  = 0x62 // FP reg -> memory
	OP_FADD    = 0x63 // fd = fs + ft
	OP_FSUB    = 0x64 // fd = fs - ft
	OP_FMUL    = 0x65 // fd = fs * ft
	OP_FDIV    = 0x66 // fd = fs / ft
	OP_FMOD    = 0x67 // fd = fmod(fs, ft)
	OP_FABS    = 0x68 // fd = |fs|
	OP_FNEG    = 0x69 // fd = -fs
	OP_FSQRT   = 0x6A // fd = sqrt(fs)
	OP_FINT    = 0x6B // Round to integer
	OP_FCMP    = 0x6C // Compare fs, ft
	OP_FCVTIF  = 0x6D // int -> float
	OP_FCVTFI  = 0x6E // float -> int
	OP_FMOVI   = 0x6F // Bitwise int -> FP
	OP_FMOVO   = 0x70 // Bitwise FP -> int
	OP_FSIN    = 0x71 // Sin
	OP_FCOS    = 0x72 // Cos
	OP_FTAN    = 0x73 // Tan
	OP_FATAN   = 0x74 // Atan
	OP_FLOG    = 0x75 // Ln
	OP_FEXP    = 0x76 // e^x
	OP_FPOW    = 0x77 // fs^ft
	OP_FMOVECR = 0x78 // Load ROM constant
	OP_FMOVSR  = 0x79 // Read FPSR
	OP_FMOVCR  = 0x7A // Read FPCR
	OP_FMOVSC  = 0x7B // Write FPSR
	OP_FMOVCC  = 0x7C // Write FPCR
	OP_DMOV    = 0x80 // FP64 reg-pair copy
	OP_DLOAD   = 0x81 // Memory -> FP64 reg pair
	OP_DSTORE  = 0x82 // FP64 reg pair -> memory
	OP_DADD    = 0x83 // FP64 add
	OP_DSUB    = 0x84 // FP64 subtract
	OP_DMUL    = 0x85 // FP64 multiply
	OP_DDIV    = 0x86 // FP64 divide
	OP_DMOD    = 0x87 // FP64 modulo
	OP_DABS    = 0x88 // FP64 abs
	OP_DNEG    = 0x89 // FP64 negate
	OP_DSQRT   = 0x8A // FP64 sqrt
	OP_DINT    = 0x8B // FP64 round to integer
	OP_DCMP    = 0x8C // FP64 compare
	OP_DCVTIF  = 0x8D // int64 -> float64
	OP_DCVTFI  = 0x8E // float64 -> int64
	OP_FCVTSD  = 0x8F // float32 -> float64
	OP_FCVTDS  = 0x90 // float64 -> float32

	// System
	OP_NOP64 = 0xE0 // No operation

	OP_HALT64 = 0xE1 // Halt processor
	OP_SEI64  = 0xE2 // Set interrupt enable
	OP_CLI64  = 0xE3 // Clear interrupt enable
	OP_RTI64  = 0xE4 // Return from interrupt
	OP_WAIT64 = 0xE5 // Wait (microseconds in imm32)

	// MMU / Privilege (privileged except SYSCALL and SMODE)
	OP_MTCR     = 0xE6 // Move To Control Register (rd=CR#, rs=src_reg)
	OP_MFCR     = 0xE7 // Move From Control Register (rd=dest_reg, rs=CR#)
	OP_ERET     = 0xE8 // Exception Return (PC = faultPC, switch to user mode)
	OP_TLBFLUSH = 0xE9 // Flush entire software TLB + invalidate JIT cache
	OP_TLBINVAL = 0xEA // Invalidate single TLB entry (rs=vpn_reg)
	OP_SYSCALL  = 0xEB // Trap into supervisor (imm32 = syscall number)
	OP_SMODE    = 0xEC // Read current mode into Rd (0=user, 1=supervisor)

	// Atomic Memory RMW (always 64-bit, naturally aligned)
	OP_CAS  = 0xED // Compare-and-swap: old=[addr]; if old==rd then [addr]=rt; rd=old
	OP_XCHG = 0xEE // Exchange: old=[addr]; [addr]=rt; rd=old
	OP_FAA  = 0xEF // Fetch-and-add: old=[addr]; [addr]=old+rt; rd=old
	OP_FAND = 0xF0 // Fetch-and-and: old=[addr]; [addr]=old&rt; rd=old
	OP_FOR  = 0xF1 // Fetch-and-or:  old=[addr]; [addr]=old|rt; rd=old
	OP_FXOR = 0xF2 // Fetch-and-xor: old=[addr]; [addr]=old^rt; rd=old

	// M15.6 G2: supervisor-user-access latch controls.
	// Privileged; SUAEN sets the SUA latch, SUADIS clears it. Single
	// cycle with no operands.
	OP_SUAEN  = 0xF3 // Enable supervisor access to user pages (set SUA latch)
	OP_SUADIS = 0xF4 // Disable supervisor access to user pages (clear SUA latch)
)

// ------------------------------------------------------------------------------
// CPU64 - 64-bit RISC CPU
// ------------------------------------------------------------------------------

type CPU64 struct {
	// Cache Lines 0-3 (256 bytes): Register file
	regs [32]uint64

	// Cache Line 4: Execution state
	PC           uint64
	running      atomic.Bool
	debug        atomic.Bool
	cycleCounter uint64
	_pad4        [40]byte // pad to 64

	// Cache Line 5: Timer
	timerCount   atomic.Uint64
	timerPeriod  atomic.Uint64
	timerState   atomic.Uint32
	timerEnabled atomic.Bool
	_pad5        [35]byte

	// Cache Line 6: Interrupt
	interruptVector  uint64
	interruptEnabled atomic.Bool
	inInterrupt      atomic.Bool
	_pad6            [46]byte

	// Beyond cache lines:
	memory  []byte
	bus     *MachineBus
	memBase unsafe.Pointer

	// VRAM direct access
	vramDirect []byte
	vramStart  uint32
	vramEnd    uint32

	// Floating Point Unit
	FPU *IE64FPU

	// Performance measurement
	PerfEnabled      bool
	InstructionCount uint64
	perfStartTime    time.Time
	lastPerfReport   time.Time

	// Execution lifecycle
	execMu     sync.Mutex
	execDone   chan struct{}
	execActive bool

	// JIT compiler state (populated only when JIT is enabled)
	jitEnabled bool
	jitPersist bool // when true, freeJIT() is a no-op (used by benchmarks)
	jitCache   *CodeCache
	jitExecMem any // *ExecMem — uses any to avoid build tag dependency
	jitCtx     *JITContext

	// Coprocessor mode: allows PC outside PROG_START..STACK_START
	CoprocMode bool

	// MMU state
	mmuEnabled     bool         // MMU translation active
	supervisorMode bool         // true = supervisor, false = user
	previousMode   bool         // privilege level before last trap/interrupt entry
	ptbr           uint32       // Page Table Base Register (physical address)
	trapVector     uint64       // Trap handler entry point
	intrVector     uint64       // Interrupt vector (CR_INTR_VEC, for MMU-mode interrupts)
	faultPC        uint64       // PC saved at trap entry
	faultAddr      uint32       // Virtual address that caused fault
	faultCause     uint32       // Fault cause code
	trapped        bool         // Set by memory helpers on MMU fault; checked by Execute/StepOne
	jitNeedInval   bool         // Set by MMU ops; consumed by JIT dispatcher
	tlb            [64]TLBEntry // 64-entry direct-mapped software TLB
	threadPointer  uint64       // Thread Pointer (CR_TP)
	kernelSP       uint64       // Kernel Stack Pointer (CR_KSP)
	userSP         uint64       // Saved User Stack Pointer (CR_USP)

	// M15.6 G2: SMEP/SMAP-equivalent controls. See MMU_CTRL_* constants.
	skef     bool // SKEF: supervisor-kernel-execute-fault enabled
	skac     bool // SKAC: supervisor-kernel-access-check enabled
	suaLatch bool // SUA latch: explicit supervisor access to user pages (SUAEN/SUADIS)
	savedSUA bool // SUA saved on trap entry; restored on ERET to supervisor

	// M15.6 G2 Phase 2c-trap: per-CPU trap-frame stack.
	//
	// Nested trap state (faultPC, previousMode, savedSUA, faultAddr,
	// faultCause) used to live in single-slot fields. A nested trap
	// would therefore overwrite the outer trap's context, forcing every
	// kernel handler that could take a nested synchronous trap to save
	// and restore CR_FAULT_PC / CR_SAVED_SUA manually. That discipline
	// was institutionalized debt: any handler path missing the restore
	// silently corrupted outer state.
	//
	// The trap-frame stack makes nested preservation architectural. On
	// trapEntry the current active frame is pushed; on ERET the popped
	// frame is restored. The active fields above (faultPC et al.) remain
	// the canonical "top of stack" accessed by MFCR/MTCR, so CR semantics
	// are unchanged and existing kernel save/restore code continues to
	// work (now redundantly).
	//
	// Depth is fixed. Exceeding it is always a kernel bug (runaway
	// nested faults); the CPU halts with a diagnostic rather than
	// silently losing a frame.
	trapStack [TrapStackDepth]trapFrame
	trapDepth int

	// trapHalted is set by pushTrapFrame on overflow. It is read by the
	// interpreter's main loop and the JIT dispatcher so they can bail
	// out on the very same iteration the overflow occurred. Using a
	// plain bool (single-owner, read/written only from the CPU goroutine)
	// avoids an atomic Load in the hot instruction fetch path;
	// cross-goroutine "please stop" signalling still goes through the
	// existing cpu.running atomic.
	trapHalted bool
}

// trapFrame captures everything that a trap entry overwrites. On
// trapEntry the current active state is snapshotted into a frame and
// pushed; on ERET the frame on top of the stack is popped back into the
// active fields. The kernel never sees the stack directly — it observes
// the active frame through the existing CR_FAULT_PC / CR_FAULT_ADDR /
// CR_FAULT_CAUSE / CR_PREV_MODE / CR_SAVED_SUA interface.
type trapFrame struct {
	faultPC    uint64
	faultAddr  uint32
	faultCause uint32
	prevMode   bool
	savedSUA   bool
}

// TrapStackDepth is the fixed nesting limit. Kernel handlers typically
// reach depth 1 (user→supervisor) or occasionally 2 (nested synchronous
// trap during a copy helper). Deeper nesting almost always indicates a
// runaway fault storm; 8 leaves generous headroom without making a
// broken kernel silently progress.
const TrapStackDepth = 8

// ------------------------------------------------------------------------------
// Constructor
// ------------------------------------------------------------------------------

func NewCPU64(bus *MachineBus) *CPU64 {
	cpu := &CPU64{
		PC:             PROG_START,
		bus:            bus,
		memory:         bus.GetMemory(),
		supervisorMode: true, // Boot in supervisor mode
		FPU:            NewIE64FPU(),
	}
	cpu.memBase = unsafe.Pointer(&cpu.memory[0])
	cpu.regs[31] = STACK_START // R31 is the stack pointer
	cpu.running.Store(true)
	return cpu
}

// ------------------------------------------------------------------------------
// Trap Helpers (MMU)
// ------------------------------------------------------------------------------

// trapEntry performs the common trap/interrupt entry sequence:
// pushes the current active frame onto the trap stack, saves previous
// mode, switches stack if coming from user mode, sets supervisor, and
// disables interrupts. ERET re-enables interrupts when returning to
// user mode.
//
// The push happens first so the outer trap's context
// (faultPC, previousMode, savedSUA, faultAddr, faultCause) is preserved
// even if a nested handler overwrites the active fields. ERET's pop
// reverses this; nested preservation is therefore architectural, and
// kernel handlers do not need to save/restore CR_FAULT_PC or
// CR_SAVED_SUA on their own.
//
// Returns false if the trap-frame stack overflowed — in that case the
// CPU is halted, none of the trap-entry side effects were applied,
// and the caller must not redirect PC to the trap handler or otherwise
// proceed as if the trap had taken effect.
func (cpu *CPU64) trapEntry() bool {
	if !cpu.pushTrapFrame() {
		return false // overflow halted the CPU; abort trap entry cleanly
	}
	cpu.previousMode = cpu.supervisorMode
	if !cpu.supervisorMode {
		// User → supervisor: swap stacks
		cpu.userSP = cpu.regs[31]
		cpu.regs[31] = cpu.kernelSP
	}
	cpu.supervisorMode = true
	cpu.interruptEnabled.Store(false) // atomically disable interrupts on entry

	// M15.6 G2: save and clear the SUA latch so a nested kernel handler
	// cannot inherit an open supervisor-user-access window from the
	// interrupted code path. ERET restores savedSUA when returning to
	// supervisor mode so the interrupted copy helper (if any) resumes
	// safely; user-mode ERET clears SUA unconditionally.
	cpu.savedSUA = cpu.suaLatch
	cpu.suaLatch = false
	return true
}

// pushTrapFrame snapshots the current active frame onto the trap stack.
// Returns false if the stack is full; in that case the CPU halts with a
// diagnostic and sets trapHalted so the interpreter/JIT main loops can
// bail out on the current iteration rather than continuing to execute
// guest instructions until the next periodic cpu.running poll. Overflow
// only happens on runaway nested faults, which is always a kernel bug.
func (cpu *CPU64) pushTrapFrame() bool {
	if cpu.trapDepth >= TrapStackDepth {
		fmt.Printf("IE64: trap stack overflow (depth=%d) — halting; runaway nested faults indicate a kernel bug\n",
			cpu.trapDepth)
		cpu.trapHalted = true
		cpu.running.Store(false)
		return false
	}
	cpu.trapStack[cpu.trapDepth] = trapFrame{
		faultPC:    cpu.faultPC,
		faultAddr:  cpu.faultAddr,
		faultCause: cpu.faultCause,
		prevMode:   cpu.previousMode,
		savedSUA:   cpu.savedSUA,
	}
	cpu.trapDepth++
	return true
}

// popTrapFrame restores the frame on top of the trap stack into the
// active fields. Called by ERET after it has consumed the current
// active frame (setting PC and suaLatch). If the stack is empty the
// active fields are cleared to zero; this matches the fresh-boot
// state and prevents stale values from leaking across ERET boundaries.
func (cpu *CPU64) popTrapFrame() {
	if cpu.trapDepth == 0 {
		cpu.faultPC = 0
		cpu.faultAddr = 0
		cpu.faultCause = 0
		cpu.previousMode = false
		cpu.savedSUA = false
		return
	}
	cpu.trapDepth--
	f := cpu.trapStack[cpu.trapDepth]
	cpu.faultPC = f.faultPC
	cpu.faultAddr = f.faultAddr
	cpu.faultCause = f.faultCause
	cpu.previousMode = f.prevMode
	cpu.savedSUA = f.savedSUA
}

// trapFault handles involuntary traps (page fault, privilege violation).
// Saves PC of the faulting instruction so ERET re-executes it. On
// trap-frame stack overflow returns early with the CPU halted; the
// active fields and PC are left untouched so the interpreter/JIT
// main loops do not jump into a trap handler on top of a halted CPU.
func (cpu *CPU64) trapFault(cause uint32, addr uint32) {
	if !cpu.trapEntry() {
		return
	}
	cpu.faultPC = cpu.PC // re-execute on ERET
	cpu.faultAddr = addr
	cpu.faultCause = cause
	cpu.PC = cpu.trapVector
}

// trapSyscall handles SYSCALL. Saves PC+8 so ERET skips the SYSCALL.
// The syscall number (imm32) is stored in faultAddr for the handler to read
// via MFCR CR1. User convention: arguments in R1-R6 before SYSCALL.
// Overflow handling matches trapFault: the CPU halts and PC is left alone.
func (cpu *CPU64) trapSyscall(syscallNum uint32) {
	if !cpu.trapEntry() {
		return
	}
	cpu.faultPC = cpu.PC + IE64_INSTR_SIZE // skip SYSCALL on ERET
	cpu.faultAddr = syscallNum             // handler reads via MFCR CR_FAULT_ADDR
	cpu.faultCause = FAULT_SYSCALL
	cpu.PC = cpu.trapVector
}

// requireSupervisor checks privilege; returns true if in supervisor mode.
// If in user mode, fires a privilege violation trap and returns false.
func (cpu *CPU64) requireSupervisor() bool {
	if cpu.supervisorMode {
		return true
	}
	cpu.trapFault(FAULT_PRIV, 0)
	return false
}

// ------------------------------------------------------------------------------
// Atomic Memory RMW Helper
// ------------------------------------------------------------------------------

// execAtomic performs an atomic read-modify-write on a 64-bit aligned address.
// op selects the operation: OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR.
// Sets cpu.trapped on fault (misalignment, MMU, I/O region).
func (cpu *CPU64) execAtomic(rd, rs, rt byte, imm32 uint32, op byte) {
	addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))

	// Alignment check (must be 8-byte aligned)
	if addr&7 != 0 {
		cpu.trapFault(FAULT_MISALIGNED, addr)
		cpu.trapped = true
		return
	}

	// Reject I/O region (atomics are only meaningful on RAM)
	if addr >= IO_REGION_START {
		cpu.trapFault(FAULT_MISALIGNED, addr)
		cpu.trapped = true
		return
	}

	// MMU translation
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(addr, ACCESS_WRITE)
		if fault {
			cpu.trapFault(cause, addr)
			cpu.trapped = true
			return
		}
		addr = phys
	}

	// Bounds check
	if uint64(addr)+8 > uint64(len(cpu.memory)) {
		cpu.trapFault(FAULT_MISALIGNED, addr)
		cpu.trapped = true
		return
	}

	base := unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))
	old := *(*uint64)(base)

	switch op {
	case OP_CAS:
		if old == cpu.regs[rd] {
			*(*uint64)(base) = cpu.regs[rt]
		}
	case OP_XCHG:
		*(*uint64)(base) = cpu.regs[rt]
	case OP_FAA:
		*(*uint64)(base) = old + cpu.regs[rt]
	case OP_FAND:
		*(*uint64)(base) = old & cpu.regs[rt]
	case OP_FOR:
		*(*uint64)(base) = old | cpu.regs[rt]
	case OP_FXOR:
		*(*uint64)(base) = old ^ cpu.regs[rt]
	}

	cpu.setReg(rd, old)
}

// ------------------------------------------------------------------------------
// Register Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) setReg(idx byte, val uint64) {
	if idx == 0 {
		return // R0 is hardwired to zero
	}
	cpu.regs[idx] = val
}

func (cpu *CPU64) getReg(idx byte) uint64 {
	return cpu.regs[idx] // R0 always reads 0 since it is never written
}

// ------------------------------------------------------------------------------
// Size Masking
// ------------------------------------------------------------------------------

var ie64SizeMask = [4]uint64{0xFF, 0xFFFF, 0xFFFFFFFF, 0xFFFFFFFFFFFFFFFF}

func maskToSize(val uint64, size byte) uint64 {
	return val & ie64SizeMask[size]
}

func signExtendToInt64(val uint64, size byte) int64 {
	switch size {
	case IE64_SIZE_B:
		return int64(int8(val))
	case IE64_SIZE_W:
		return int64(int16(val))
	case IE64_SIZE_L:
		return int64(int32(val))
	default:
		return int64(val)
	}
}

func rotateLeftToSize(val uint64, shift uint64, size byte) uint64 {
	switch size {
	case IE64_SIZE_B:
		return uint64(bits.RotateLeft8(uint8(val), int(shift&7)))
	case IE64_SIZE_W:
		return uint64(bits.RotateLeft16(uint16(val), int(shift&15)))
	case IE64_SIZE_L:
		return uint64(bits.RotateLeft32(uint32(val), int(shift&31)))
	default:
		return bits.RotateLeft64(val, int(shift&63))
	}
}

func rotateRightToSize(val uint64, shift uint64, size byte) uint64 {
	switch size {
	case IE64_SIZE_B:
		return uint64(bits.RotateLeft8(uint8(val), -int(shift&7)))
	case IE64_SIZE_W:
		return uint64(bits.RotateLeft16(uint16(val), -int(shift&15)))
	case IE64_SIZE_L:
		return uint64(bits.RotateLeft32(uint32(val), -int(shift&31)))
	default:
		return bits.RotateLeft64(val, -int(shift&63))
	}
}

func mulHighSigned(a, b int64) uint64 {
	neg := (a < 0) != (b < 0)
	ua := uint64(a)
	if a < 0 {
		ua = uint64(-a)
	}
	ub := uint64(b)
	if b < 0 {
		ub = uint64(-b)
	}
	hi, lo := bits.Mul64(ua, ub)
	if neg {
		hi = ^hi
		lo = ^lo + 1
		if lo == 0 {
			hi++
		}
	}
	return hi
}

func isValidDPairReg(idx byte) bool {
	return idx <= 15 && (idx&1) == 0
}

// ------------------------------------------------------------------------------
// Memory Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) loadMem(addr uint32, size byte) uint64 {
	// MMU translation
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(addr, ACCESS_READ)
		if fault {
			cpu.trapFault(cause, addr)
			cpu.trapped = true
			return 0
		}
		addr = phys
	}

	// VRAM direct read fast path (VRAM addresses are above IO_REGION_START)
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		base := unsafe.Pointer(&cpu.vramDirect[offset])
		switch size {
		case IE64_SIZE_Q:
			return *(*uint64)(base)
		case IE64_SIZE_L:
			return uint64(*(*uint32)(base))
		case IE64_SIZE_W:
			return uint64(*(*uint16)(base))
		case IE64_SIZE_B:
			return uint64(*(*byte)(base))
		}
	}

	// Non-I/O fast path - direct memory read, no bus overhead
	if addr < IO_REGION_START {
		base := unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))
		switch size {
		case IE64_SIZE_Q:
			return *(*uint64)(base)
		case IE64_SIZE_L:
			return uint64(*(*uint32)(base))
		case IE64_SIZE_W:
			return uint64(*(*uint16)(base))
		case IE64_SIZE_B:
			return uint64(*(*byte)(base))
		}
	}

	// I/O slow path - bus callbacks protect their own state
	switch size {
	case IE64_SIZE_B:
		return uint64(cpu.bus.Read8(addr))
	case IE64_SIZE_W:
		return uint64(cpu.bus.Read16(addr))
	case IE64_SIZE_L:
		return uint64(cpu.bus.Read32(addr))
	case IE64_SIZE_Q:
		return cpu.bus.Read64(addr)
	}
	return 0
}

func (cpu *CPU64) storeMem(addr uint32, val uint64, size byte) {
	// MMU translation
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(addr, ACCESS_WRITE)
		if fault {
			cpu.trapFault(cause, addr)
			cpu.trapped = true
			return
		}
		addr = phys
	}

	// VRAM direct write fast path (VRAM addresses are above IO_REGION_START)
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		base := unsafe.Pointer(&cpu.vramDirect[offset])
		switch size {
		case IE64_SIZE_B:
			*(*byte)(base) = byte(val)
		case IE64_SIZE_W:
			*(*uint16)(base) = uint16(val)
		case IE64_SIZE_L:
			*(*uint32)(base) = uint32(val)
		case IE64_SIZE_Q:
			*(*uint64)(base) = val
		}
		return
	}

	// Non-I/O fast path - direct memory write, no bus overhead
	if addr < IO_REGION_START {
		base := unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))
		switch size {
		case IE64_SIZE_B:
			*(*byte)(base) = byte(val)
		case IE64_SIZE_W:
			*(*uint16)(base) = uint16(val)
		case IE64_SIZE_L:
			*(*uint32)(base) = uint32(val)
		case IE64_SIZE_Q:
			*(*uint64)(base) = val
		}
		return
	}

	// I/O slow path - bus callbacks protect their own state
	switch size {
	case IE64_SIZE_B:
		cpu.bus.Write8(addr, uint8(val))
	case IE64_SIZE_W:
		cpu.bus.Write16(addr, uint16(val))
	case IE64_SIZE_L:
		cpu.bus.Write32(addr, uint32(val))
	case IE64_SIZE_Q:
		cpu.bus.Write64(addr, val)
	}
}

// ------------------------------------------------------------------------------
// Program Loading
// ------------------------------------------------------------------------------

func (cpu *CPU64) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	cpu.LoadProgramBytes(program)
	return nil
}

// LoadProgramBytes loads raw machine code bytes at PROG_START and resets PC.
func (cpu *CPU64) LoadProgramBytes(program []byte) {
	// Clear program area
	for i := PROG_START; i < len(cpu.memory) && i < STACK_START; i++ {
		cpu.memory[i] = 0
	}
	copy(cpu.memory[PROG_START:], program)
	cpu.PC = PROG_START
}

// ------------------------------------------------------------------------------
// Reset
// ------------------------------------------------------------------------------

func (cpu *CPU64) Reset() {
	cpu.running.Store(false)
	time.Sleep(RESET_DELAY)

	// Clear registers
	for i := range cpu.regs {
		cpu.regs[i] = 0
	}
	cpu.PC = PROG_START
	cpu.regs[31] = STACK_START
	cpu.cycleCounter = 0
	cpu.interruptVector = 0
	cpu.interruptEnabled.Store(false)
	cpu.inInterrupt.Store(false)
	cpu.timerCount.Store(0)
	cpu.timerPeriod.Store(0)
	cpu.timerState.Store(TIMER_STOPPED)
	cpu.timerEnabled.Store(false)
	cpu.InstructionCount = 0

	// Clear JIT code cache
	if cpu.jitCache != nil {
		cpu.jitCache.Invalidate()
	}

	// Reset MMU state
	cpu.mmuEnabled = false
	cpu.supervisorMode = true
	cpu.previousMode = false
	cpu.ptbr = 0
	cpu.trapVector = 0
	cpu.intrVector = 0
	cpu.faultPC = 0
	cpu.faultAddr = 0
	cpu.faultCause = 0
	cpu.trapped = false
	cpu.jitNeedInval = false
	cpu.threadPointer = 0
	cpu.kernelSP = 0
	cpu.userSP = 0

	// M15.6 G2: clear the SMEP/SMAP-equivalent latches so a reused CPU
	// instance doesn't start the next program with a stale SKEF/SKAC
	// enable or an open SUA window. Must match the fresh-boot contract
	// where all four bits are zero.
	cpu.skef = false
	cpu.skac = false
	cpu.suaLatch = false
	cpu.savedSUA = false

	// M15.6 G2 Phase 2c-trap: wipe the trap-frame stack so a reused CPU
	// does not inherit a half-built frame from an interrupted prior run.
	// Also clear the overflow-halt flag — without this, a reused CPU
	// whose previous life ended in trap-stack overflow would refuse to
	// execute the first instruction of the next run.
	cpu.trapDepth = 0
	for i := range cpu.trapStack {
		cpu.trapStack[i] = trapFrame{}
	}
	cpu.trapHalted = false

	// Reset FPU
	if cpu.FPU != nil {
		cpu.FPU.FPSR = 0
		cpu.FPU.FPCR = 0
		for i := range cpu.FPU.FPRegs {
			cpu.FPU.FPRegs[i] = 0
		}
	}

	// Clear memory in chunks for better cache utilization
	for i := PROG_START; i < len(cpu.memory); i += CACHE_LINE_SIZE {
		end := min(i+CACHE_LINE_SIZE, len(cpu.memory))
		for j := i; j < end; j++ {
			cpu.memory[j] = 0
		}
	}

	cpu.running.Store(true)
}

// ------------------------------------------------------------------------------
// VRAM Direct Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) AttachDirectVRAM(buffer []byte, start, end uint32) {
	cpu.vramDirect = buffer
	cpu.vramStart = start
	cpu.vramEnd = end
}

func (cpu *CPU64) DetachDirectVRAM() {
	cpu.vramDirect = nil
}

// ------------------------------------------------------------------------------
// Execute - Main Instruction Loop
// ------------------------------------------------------------------------------

func (cpu *CPU64) Execute() {
	if !cpu.CoprocMode && (cpu.PC < PROG_START || cpu.PC >= STACK_START) {
		fmt.Printf("IE64: Invalid initial PC value: 0x%08x\n", cpu.PC)
		cpu.running.Store(false)
		return
	}

	// Initialize performance measurement
	cpu.perfStartTime = time.Now()
	cpu.lastPerfReport = cpu.perfStartTime
	cpu.InstructionCount = 0

	// Cache locals that never change during execution
	perfEnabled := cpu.PerfEnabled
	memBase := unsafe.Pointer(&cpu.memory[0])
	memSize := uint64(len(cpu.memory))

	// Use local running flag to avoid atomic load every iteration
	// Check external stop signal every 4096 instructions
	running := true
	checkCounter := uint32(0)

	for running {
		// M15.6 G2 Phase 2c-trap: trap-frame stack overflow sets
		// trapHalted in pushTrapFrame. Poll it every iteration (cheap
		// non-atomic read) so a runaway nested-fault kernel bug stops
		// the interpreter on the same instruction it failed, not up
		// to 4095 instructions later at the next cpu.running poll.
		if cpu.trapHalted {
			break
		}
		// Periodic check of external stop signal (every 4096 instructions)
		checkCounter++
		if checkCounter&0xFFF == 0 && !cpu.running.Load() {
			break
		}

		// Performance measurement: count instructions and report periodically
		if perfEnabled {
			cpu.InstructionCount++
			if cpu.InstructionCount&0xFFFFFF == 0 { // Every ~16M instructions
				now := time.Now()
				if now.Sub(cpu.lastPerfReport) >= time.Second {
					elapsed := now.Sub(cpu.perfStartTime).Seconds()
					mips := float64(cpu.InstructionCount) / elapsed / 1_000_000
					fmt.Printf("\rIE64: %.2f MIPS (%.0f instructions in %.1fs)", mips, float64(cpu.InstructionCount), elapsed)
					cpu.lastPerfReport = now
				}
			}
		}

		// Mask PC to 32MB address space
		pc32 := uint32(cpu.PC & IE64_ADDR_MASK)

		// MMU translation for instruction fetch
		if cpu.mmuEnabled {
			phys, fault, cause := cpu.translateAddr(pc32, ACCESS_EXEC)
			if fault {
				cpu.trapFault(cause, pc32)
				continue
			}
			pc32 = phys
		}

		if uint64(pc32)+8 > memSize {
			fmt.Printf("IE64: PC out of bounds during fetch: PC=0x%08X mem=%d\n", pc32, memSize)
			cpu.running.Store(false)
			break
		}

		// Fetch entire 8-byte instruction in one read (LE platform enforced by le_check.go)
		instr := *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(pc32)))
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		// Decode fields
		rd := byte1 >> 3            // 5 bits
		size := (byte1 >> 1) & 0x03 // 2 bits
		xbit := byte1 & 1           // 1 bit
		rs := byte2 >> 3            // 5 bits
		rt := byte3 >> 3            // 5 bits

		// Resolve third operand: immediate or register Rt
		var operand3 uint64
		if xbit == 1 {
			operand3 = uint64(imm32) // zero-extended
		} else {
			operand3 = cpu.regs[rt]
		}

		// Timer handling: decrement TIMER_COUNT every instruction cycle.
		// When it reaches 0, fire interrupt and reload from TIMER_PERIOD.
		if cpu.timerEnabled.Load() {
			count := cpu.timerCount.Load()
			if count > 0 {
				newCount := count - 1
				cpu.timerCount.Store(newCount)
				if newCount == 0 {
					cpu.timerState.Store(TIMER_EXPIRED)
					// Reload timer before dispatching interrupt (handler may disable it)
					if cpu.timerEnabled.Load() {
						cpu.timerCount.Store(cpu.timerPeriod.Load())
					}
					// Handle timer interrupt
					if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
						if cpu.mmuEnabled && cpu.intrVector != 0 {
							// ERET-model interrupt entry (unified with trap path).
							// If trapEntry overflows, the CPU is halted and PC
							// must not be redirected to the interrupt vector:
							// the next loop iteration's trapHalted check will
							// break out.
							if !cpu.trapEntry() {
								running = false
								continue
							}
							cpu.faultPC = cpu.PC
							cpu.faultAddr = 0
							cpu.faultCause = FAULT_TIMER
							cpu.PC = cpu.intrVector
							continue // re-enter loop at interrupt handler
						} else {
							// Legacy push-PC/RTI model (MMU off or INTR_VEC not set)
							cpu.inInterrupt.Store(true)
							cpu.regs[31] -= 8
							sp := uint32(cpu.regs[31])
							if !cpu.mmuStackWrite(sp, cpu.PC, memBase, memSize) {
								if cpu.trapped {
									cpu.trapped = false
									cpu.regs[31] += 8
									cpu.inInterrupt.Store(false)
									continue
								}
								cpu.running.Store(false)
								running = false
								continue
							}
							cpu.PC = cpu.interruptVector
							continue // re-enter loop at interrupt handler
						}
					}
				}
			} else {
				// Auto-start: if count is 0 but period is set, load initial count
				period := cpu.timerPeriod.Load()
				if period > 0 {
					cpu.timerCount.Store(period)
					cpu.timerState.Store(TIMER_RUNNING)
				}
			}
		}

		switch opcode {
		case OP_MOVE:
			if xbit == 1 {
				if rd != 0 {
					cpu.regs[rd] = maskToSize(uint64(imm32), size)
				}
			} else {
				if rd != 0 {
					cpu.regs[rd] = maskToSize(cpu.regs[rs], size)
				}
			}

		case OP_MOVT:
			if rd != 0 {
				cpu.regs[rd] = (cpu.regs[rd] & 0x00000000FFFFFFFF) | (uint64(imm32) << 32)
			}

		case OP_MOVEQ:
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(int32(imm32)))
			}

		case OP_LEA:
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			}

		case OP_LOAD:
			if rd != 0 {
				addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
				cpu.regs[rd] = cpu.loadMem(addr, size)
				if cpu.trapped {
					cpu.trapped = false
					continue
				}
			}

		case OP_STORE:
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, maskToSize(cpu.regs[rd], size), size)
			if cpu.trapped {
				cpu.trapped = false
				continue
			}

		case OP_ADD:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]+operand3, size)
			}

		case OP_SUB:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]-operand3, size)
			}

		case OP_MULU:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]*operand3, size)
			}

		case OP_MULS:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])*int64(operand3)), size)
			}

		case OP_DIVU:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(cpu.regs[rs]/operand3, size)
				}
			}

		case OP_DIVS:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])/int64(operand3)), size)
				}
			}

		case OP_MOD64:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(cpu.regs[rs]%operand3, size)
				}
			}

		case OP_NEG:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(uint64(-int64(cpu.regs[rs])), size)
			}

		case OP_MODS:
			if rd != 0 {
				a := signExtendToInt64(cpu.regs[rs], size)
				b := signExtendToInt64(operand3, size)
				if b == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(uint64(a%b), size)
				}
			}

		case OP_MULHU:
			if rd != 0 {
				hi, _ := bits.Mul64(cpu.regs[rs], operand3)
				cpu.regs[rd] = hi
			}

		case OP_MULHS:
			if rd != 0 {
				cpu.regs[rd] = mulHighSigned(int64(cpu.regs[rs]), int64(operand3))
			}

		case OP_AND64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]&operand3, size)
			}

		case OP_OR64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]|operand3, size)
			}

		case OP_EOR:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]^operand3, size)
			}

		case OP_NOT64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(^cpu.regs[rs], size)
			}

		case OP_LSL:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]<<(operand3&63), size)
			}

		case OP_LSR:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]>>(operand3&63), size)
			}

		case OP_ASR:
			if rd != 0 {
				shift := operand3 & 63
				var sval int64
				switch size {
				case IE64_SIZE_B:
					sval = int64(int8(cpu.regs[rs]))
				case IE64_SIZE_W:
					sval = int64(int16(cpu.regs[rs]))
				case IE64_SIZE_L:
					sval = int64(int32(cpu.regs[rs]))
				case IE64_SIZE_Q:
					sval = int64(cpu.regs[rs])
				}
				cpu.regs[rd] = maskToSize(uint64(sval>>shift), size)
			}

		case OP_CLZ:
			if rd != 0 {
				cpu.regs[rd] = uint64(bits.LeadingZeros32(uint32(cpu.regs[rs])))
			}

		case OP_SEXT:
			if rd != 0 {
				switch size {
				case IE64_SIZE_B, IE64_SIZE_W, IE64_SIZE_L:
					cpu.regs[rd] = uint64(signExtendToInt64(cpu.regs[rs], size))
				case IE64_SIZE_Q:
					cpu.regs[rd] = cpu.regs[rs]
				}
			}

		case OP_ROL:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(rotateLeftToSize(maskToSize(cpu.regs[rs], size), operand3, size), size)
			}

		case OP_ROR:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(rotateRightToSize(maskToSize(cpu.regs[rs], size), operand3, size), size)
			}

		case OP_CTZ:
			if rd != 0 {
				cpu.regs[rd] = uint64(bits.TrailingZeros32(uint32(cpu.regs[rs])))
			}

		case OP_POPCNT:
			if rd != 0 {
				cpu.regs[rd] = uint64(bits.OnesCount32(uint32(cpu.regs[rs])))
			}

		case OP_BSWAP:
			if rd != 0 {
				cpu.regs[rd] = uint64(bits.ReverseBytes32(uint32(cpu.regs[rs])))
			}

		case OP_BRA:
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			continue

		case OP_BEQ:
			if cpu.regs[rs] == cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BNE:
			if cpu.regs[rs] != cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLT:
			if int64(cpu.regs[rs]) < int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BGE:
			if int64(cpu.regs[rs]) >= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BGT:
			if int64(cpu.regs[rs]) > int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLE:
			if int64(cpu.regs[rs]) <= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BHI:
			if cpu.regs[rs] > cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLS:
			if cpu.regs[rs] <= cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_JMP:
			target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.PC = target & IE64_ADDR_MASK
			continue

		case OP_JSR64:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if !cpu.mmuStackWrite(sp, cpu.PC+IE64_INSTR_SIZE, memBase, memSize) {
				if cpu.trapped {
					cpu.trapped = false
					cpu.regs[31] += 8 // undo SP decrement on fault
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			continue

		case OP_RTS64:
			sp := uint32(cpu.regs[31])
			val, ok := cpu.mmuStackRead(sp, memBase, memSize)
			if !ok {
				if cpu.trapped {
					cpu.trapped = false
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}
			cpu.PC = val
			cpu.regs[31] += 8
			continue

		case OP_PUSH64:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if !cpu.mmuStackWrite(sp, cpu.regs[rs], memBase, memSize) {
				if cpu.trapped {
					cpu.trapped = false
					cpu.regs[31] += 8
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}

		case OP_POP64:
			sp := uint32(cpu.regs[31])
			val, ok := cpu.mmuStackRead(sp, memBase, memSize)
			if !ok {
				if cpu.trapped {
					cpu.trapped = false
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}
			if rd != 0 {
				cpu.regs[rd] = val
			}
			cpu.regs[31] += 8

		case OP_JSR_IND:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if !cpu.mmuStackWrite(sp, cpu.PC+IE64_INSTR_SIZE, memBase, memSize) {
				if cpu.trapped {
					cpu.trapped = false
					cpu.regs[31] += 8
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}
			target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.PC = target & IE64_ADDR_MASK
			continue

		// ----------------------------------------------------------------------
		// Floating Point (FPU)
		// ----------------------------------------------------------------------

		case OP_FMOV:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FPRegs[rd&0x0F] = cpu.FPU.FPRegs[rs&0x0F]

		case OP_FLOAD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 {
				goto invalid_freg
			}
			{
				addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
				val := uint32(cpu.loadMem(addr, IE64_SIZE_L))
				if cpu.trapped {
					cpu.trapped = false
					continue
				}
				cpu.FPU.FPRegs[rd] = val
				cpu.FPU.setConditionCodesBits(val)
			}

		case OP_FSTORE:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 {
				goto invalid_freg
			}
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, uint64(cpu.FPU.FPRegs[rd]), IE64_SIZE_L)
			if cpu.trapped {
				cpu.trapped = false
				continue
			}

		case OP_FADD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FADD(rd, rs, rt)

		case OP_FSUB:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FSUB(rd, rs, rt)

		case OP_FMUL:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FMUL(rd, rs, rt)

		case OP_FDIV:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FDIV(rd, rs, rt)

		case OP_FMOD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FMOD(rd, rs, rt)

		case OP_FABS:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			bits := cpu.FPU.FPRegs[rs&0x0F] & 0x7FFFFFFF
			cpu.FPU.FPRegs[rd&0x0F] = bits
			cpu.FPU.setConditionCodesBits(bits)

		case OP_FNEG:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			bits := cpu.FPU.FPRegs[rs&0x0F] ^ 0x80000000
			cpu.FPU.FPRegs[rd&0x0F] = bits
			cpu.FPU.setConditionCodesBits(bits)

		case OP_FSQRT:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FSQRT(rd, rs)

		case OP_FINT:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FINT(rd, rs)

		case OP_FCMP:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rs > 15 || rt > 15 {
				goto invalid_freg
			}
			res := cpu.FPU.FCMP(rs, rt)
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(res))
			}

		case OP_FCVTIF:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 {
				goto invalid_freg
			}
			cpu.FPU.FCVTIF(rd, cpu.regs[rs])

		case OP_FCVTFI:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rs > 15 {
				goto invalid_freg
			}
			res := cpu.FPU.FCVTFI(rs)
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(res))
			}

		case OP_FMOVI:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 {
				goto invalid_freg
			}
			cpu.FPU.FMOVI(rd, cpu.regs[rs])

		case OP_FMOVO:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rs > 15 {
				goto invalid_freg
			}
			res := cpu.FPU.FMOVO(rs)
			if rd != 0 {
				cpu.regs[rd] = res
			}

		case OP_FSIN:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FSIN(rd, rs)

		case OP_FCOS:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FCOS(rd, rs)

		case OP_FTAN:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FTAN(rd, rs)

		case OP_FATAN:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FATAN(rd, rs)

		case OP_FLOG:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FLOG(rd, rs)

		case OP_FEXP:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FEXP(rd, rs)

		case OP_FPOW:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || rs > 15 || rt > 15 {
				goto invalid_freg
			}
			cpu.FPU.FPOW(rd, rs, rt)

		case OP_FMOVECR:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 {
				goto invalid_freg
			}
			cpu.FPU.FMOVECR(rd, byte(imm32))

		case OP_FMOVSR:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd != 0 {
				cpu.regs[rd] = uint64(cpu.FPU.FMOVSR())
			}

		case OP_FMOVCR:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd != 0 {
				cpu.regs[rd] = uint64(cpu.FPU.FMOVCR())
			}

		case OP_FMOVSC:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			cpu.FPU.FMOVSC(uint32(cpu.regs[rs]))

		case OP_FMOVCC:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			cpu.FPU.FMOVCC(uint32(cpu.regs[rs]))

		case OP_DMOV:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.setDPair(rd, cpu.FPU.getDPair(rs))

		case OP_DLOAD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) {
				goto invalid_freg
			}
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			val := cpu.loadMem(addr, IE64_SIZE_Q)
			if cpu.trapped {
				cpu.trapped = false
				continue
			}
			cpu.FPU.setDPair(rd, math.Float64frombits(val))
			cpu.FPU.setConditionCodesBits64(val)

		case OP_DSTORE:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) {
				goto invalid_freg
			}
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, math.Float64bits(cpu.FPU.getDPair(rd)), IE64_SIZE_Q)
			if cpu.trapped {
				cpu.trapped = false
				continue
			}

		case OP_DADD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			cpu.FPU.DADD(rd, rs, rt)
		case OP_DSUB:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			cpu.FPU.DSUB(rd, rs, rt)
		case OP_DMUL:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			cpu.FPU.DMUL(rd, rs, rt)
		case OP_DDIV:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			cpu.FPU.DDIV(rd, rs, rt)
		case OP_DMOD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			cpu.FPU.DMOD(rd, rs, rt)
		case OP_DABS:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.DABS(rd, rs)
		case OP_DNEG:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.DNEG(rd, rs)
		case OP_DSQRT:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.DSQRT(rd, rs)
		case OP_DINT:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.DINT(rd, rs)
		case OP_DCMP:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rs) || !isValidDPairReg(rt) {
				goto invalid_freg
			}
			if rd != 0 {
				cpu.regs[rd] = uint64(cpu.FPU.DCMP(rs, rt))
			}
		case OP_DCVTIF:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) {
				goto invalid_freg
			}
			cpu.FPU.DCVTIF(rd, cpu.regs[rs])
		case OP_DCVTFI:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rs) {
				goto invalid_freg
			}
			if rd != 0 {
				cpu.regs[rd] = uint64(cpu.FPU.DCVTFI(rs))
			}
		case OP_FCVTSD:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if !isValidDPairReg(rd) || rs > 15 {
				goto invalid_freg
			}
			cpu.FPU.FCVTSD(rd, rs)
		case OP_FCVTDS:
			if cpu.FPU == nil {
				goto fpu_missing
			}
			if rd > 15 || !isValidDPairReg(rs) {
				goto invalid_freg
			}
			cpu.FPU.FCVTDS(rd, rs)

		case OP_NOP64:
			// default PC advance

		case OP_HALT64:
			cpu.running.Store(false)
			running = false
			continue

		case OP_SEI64:
			cpu.interruptEnabled.Store(true)

		case OP_CLI64:
			cpu.interruptEnabled.Store(false)

		case OP_RTI64:
			sp := uint32(cpu.regs[31])
			val, ok := cpu.mmuStackRead(sp, memBase, memSize)
			if !ok {
				if cpu.trapped {
					cpu.trapped = false
					continue
				}
				cpu.running.Store(false)
				running = false
				continue
			}
			cpu.PC = val
			cpu.regs[31] += 8
			cpu.inInterrupt.Store(false)
			continue

		case OP_WAIT64:
			if imm32 > 0 {
				time.Sleep(time.Duration(imm32) * time.Microsecond)
			}

		// ------------------------------------------------------------------
		// MMU / Privilege
		// ------------------------------------------------------------------

		case OP_MTCR:
			if !cpu.requireSupervisor() {
				continue // trap was fired, PC is now at trap handler
			}
			crIdx := rd
			val := cpu.regs[rs]
			switch crIdx {
			case CR_PTBR:
				cpu.ptbr = uint32(val)
				cpu.tlbFlush()
				cpu.jitNeedInval = true
			case CR_FAULT_ADDR:
				cpu.faultAddr = uint32(val)
			case CR_FAULT_CAUSE:
				cpu.faultCause = uint32(val)
			case CR_FAULT_PC:
				cpu.faultPC = val
			case CR_TRAP_VEC:
				cpu.trapVector = val
			case CR_MMU_CTRL:
				// Bit 0 = mmuEnabled (writable); Bit 1 = supervisor (read-only, ignored);
				// Bits 2–3 = SKEF / SKAC enable (writable); Bit 4 = SUA latch
				// (read-only via MTCR — only SUAEN/SUADIS mutate it).
				newMMU := val&MMU_CTRL_ENABLE != 0
				if newMMU != cpu.mmuEnabled {
					cpu.mmuEnabled = newMMU
					cpu.tlbFlush()
					cpu.jitNeedInval = true
				}
				cpu.skef = val&MMU_CTRL_SKEF != 0
				cpu.skac = val&MMU_CTRL_SKAC != 0
			case CR_TP:
				cpu.threadPointer = val
			case CR_INTR_VEC:
				cpu.intrVector = val
			case CR_KSP:
				cpu.kernelSP = val
			case CR_TIMER_PERIOD:
				cpu.timerPeriod.Store(val)
			case CR_TIMER_COUNT:
				cpu.timerCount.Store(val)
			case CR_TIMER_CTRL:
				cpu.timerEnabled.Store(val&1 != 0)
				cpu.interruptEnabled.Store(val&2 != 0)
			case CR_USP:
				cpu.userSP = val
			case CR_SAVED_SUA:
				// Writable so a kernel handler can restore the outer
				// trap's SUA value before ERET. See CR_MMU_CTRL header
				// for the nested-trap discipline.
				cpu.savedSUA = val&1 != 0
			}

		case OP_MFCR:
			// CR_TP is readable from user mode; all others require supervisor
			crIdx := rs
			if crIdx != CR_TP && !cpu.requireSupervisor() {
				continue
			}
			var val uint64
			switch crIdx {
			case CR_PTBR:
				val = uint64(cpu.ptbr)
			case CR_FAULT_ADDR:
				val = uint64(cpu.faultAddr)
			case CR_FAULT_CAUSE:
				val = uint64(cpu.faultCause)
			case CR_FAULT_PC:
				val = cpu.faultPC
			case CR_TRAP_VEC:
				val = cpu.trapVector
			case CR_MMU_CTRL:
				val = 0
				if cpu.mmuEnabled {
					val |= MMU_CTRL_ENABLE
				}
				if cpu.supervisorMode {
					val |= MMU_CTRL_SUPER
				}
				if cpu.skef {
					val |= MMU_CTRL_SKEF
				}
				if cpu.skac {
					val |= MMU_CTRL_SKAC
				}
				if cpu.suaLatch {
					val |= MMU_CTRL_SUA
				}
			case CR_TP:
				val = cpu.threadPointer
			case CR_INTR_VEC:
				val = cpu.intrVector
			case CR_KSP:
				val = cpu.kernelSP
			case CR_TIMER_PERIOD:
				val = cpu.timerPeriod.Load()
			case CR_TIMER_COUNT:
				val = cpu.timerCount.Load()
			case CR_TIMER_CTRL:
				val = 0
				if cpu.timerEnabled.Load() {
					val |= 1
				}
				if cpu.interruptEnabled.Load() {
					val |= 2
				}
			case CR_USP:
				val = cpu.userSP
			case CR_PREV_MODE:
				if cpu.previousMode {
					val = 1
				}
			case CR_SAVED_SUA:
				if cpu.savedSUA {
					val = 1
				}
			}
			if rd != 0 {
				cpu.regs[rd] = val
			}

		case OP_ERET:
			if !cpu.requireSupervisor() {
				continue
			}
			// Consume the active frame: its faultPC becomes PC, and its
			// savedSUA (or 0 for user return) becomes the new suaLatch.
			cpu.PC = cpu.faultPC
			if !cpu.previousMode {
				// Returning to user mode: swap stack and atomically re-enable interrupts.
				// This eliminates the SEI/ERET race: no interrupt can fire between
				// enabling interrupts and leaving supervisor mode.
				cpu.kernelSP = cpu.regs[31]
				cpu.regs[31] = cpu.userSP
				cpu.supervisorMode = false
				cpu.interruptEnabled.Store(true)
				// M15.6 G2: user mode must never have SUA set.
				cpu.suaLatch = false
			} else {
				// Returning to supervisor (nested trap). Restore the SUA
				// latch the interrupted code path held so an in-flight
				// copy helper resumes inside its own SUAEN/SUADIS region.
				cpu.suaLatch = cpu.savedSUA
			}
			// Pop the outer frame: active fields now reflect the caller
			// trap (or fresh zeros if we were at the bottom of the stack).
			cpu.popTrapFrame()
			// If previousMode was true, stay in supervisor mode (interrupts stay disabled)
			continue // PC was set explicitly

		case OP_TLBFLUSH:
			if !cpu.requireSupervisor() {
				continue
			}
			cpu.tlbFlush()
			cpu.jitNeedInval = true

		case OP_TLBINVAL:
			if !cpu.requireSupervisor() {
				continue
			}
			vpn := uint16(cpu.regs[rs] >> MMU_PAGE_SHIFT)
			cpu.tlbInvalidate(vpn)
			cpu.jitNeedInval = true

		case OP_SYSCALL:
			cpu.trapSyscall(imm32)
			continue // PC was set to trapVector

		case OP_SMODE:
			if rd != 0 {
				if cpu.supervisorMode {
					cpu.regs[rd] = 1
				} else {
					cpu.regs[rd] = 0
				}
			}

		case OP_SUAEN:
			if !cpu.requireSupervisor() {
				continue
			}
			cpu.suaLatch = true

		case OP_SUADIS:
			if !cpu.requireSupervisor() {
				continue
			}
			cpu.suaLatch = false

		// ------------------------------------------------------------------
		// Atomic Memory RMW
		// ------------------------------------------------------------------

		case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			cpu.execAtomic(rd, rs, rt, imm32, opcode)
			if cpu.trapped {
				cpu.trapped = false
				continue
			}

		default:
			fmt.Printf("IE64: Invalid opcode 0x%02X at PC=0x%X\n", opcode, cpu.PC)
			cpu.running.Store(false)
			running = false
			continue
		}

		// Default PC advance - opcodes that set PC themselves use `continue` above
		cpu.PC += IE64_INSTR_SIZE
		continue

	fpu_missing:
		fmt.Printf("IE64: FPU instruction executed but FPU is missing at PC=0x%X\n", cpu.PC)
		cpu.running.Store(false)
		break

	invalid_freg:
		fmt.Printf("IE64: Invalid FP register index at PC=0x%X\n", cpu.PC)
		cpu.running.Store(false)
		break
	}
}

func (cpu *CPU64) IsRunning() bool {
	return cpu.running.Load()
}

func (cpu *CPU64) StartExecution() {
	cpu.execMu.Lock()
	defer cpu.execMu.Unlock()
	if cpu.execActive {
		return
	}
	cpu.execActive = true
	cpu.running.Store(true)
	cpu.execDone = make(chan struct{})
	go func() {
		defer func() {
			cpu.execMu.Lock()
			cpu.execActive = false
			close(cpu.execDone)
			cpu.execMu.Unlock()
		}()
		cpu.jitExecute()
	}()
}

func (cpu *CPU64) Stop() {
	cpu.execMu.Lock()
	if !cpu.execActive {
		cpu.running.Store(false)
		cpu.execMu.Unlock()
		return
	}
	cpu.running.Store(false)
	done := cpu.execDone
	cpu.execMu.Unlock()
	<-done
}

// StepOne executes a single instruction at the current PC and returns 1 (cycle count).
// Must only be called when the CPU is frozen (not running in its Execute loop).
func (cpu *CPU64) StepOne() int {
	memSize := uint64(len(cpu.memory))
	memBase := unsafe.Pointer(&cpu.memory[0])

	pc32 := uint32(cpu.PC & IE64_ADDR_MASK)

	// MMU translation for instruction fetch
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(pc32, ACCESS_EXEC)
		if fault {
			cpu.trapFault(cause, pc32)
			return 1 // trap consumed a cycle
		}
		pc32 = phys
	}

	if uint64(pc32)+8 > memSize {
		return 0
	}

	instr := *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(pc32)))
	opcode := byte(instr)
	byte1 := byte(instr >> 8)
	byte2 := byte(instr >> 16)
	byte3 := byte(instr >> 24)
	imm32 := uint32(instr >> 32)

	rd := byte1 >> 3
	size := (byte1 >> 1) & 0x03
	rs := byte2 >> 3
	rt := byte3 >> 3
	xbit := byte1 & 1

	var operand3 uint64
	if xbit == 1 {
		operand3 = uint64(imm32)
	} else {
		operand3 = cpu.regs[rt]
	}

	// pcAdvanced tracks whether the opcode set PC itself
	pcAdvanced := false

	switch opcode {
	case OP_MOVE:
		if xbit == 1 {
			if rd != 0 {
				cpu.regs[rd] = maskToSize(uint64(imm32), size)
			}
		} else {
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs], size)
			}
		}
	case OP_MOVT:
		if rd != 0 {
			cpu.regs[rd] = (cpu.regs[rd] & 0x00000000FFFFFFFF) | (uint64(imm32) << 32)
		}
	case OP_MOVEQ:
		if rd != 0 {
			cpu.regs[rd] = uint64(int64(int32(imm32)))
		}
	case OP_LEA:
		if rd != 0 {
			cpu.regs[rd] = uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
		}
	case OP_LOAD:
		if rd != 0 {
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.regs[rd] = cpu.loadMem(addr, size)
			if cpu.trapped {
				cpu.trapped = false
				pcAdvanced = true // trap set PC
			}
		}
	case OP_STORE:
		addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
		cpu.storeMem(addr, maskToSize(cpu.regs[rd], size), size)
		if cpu.trapped {
			cpu.trapped = false
			pcAdvanced = true
		}
	case OP_ADD:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]+operand3, size)
		}
	case OP_SUB:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]-operand3, size)
		}
	case OP_MULU:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]*operand3, size)
		}
	case OP_MULS:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])*int64(operand3)), size)
		}
	case OP_DIVU:
		if rd != 0 {
			if operand3 == 0 {
				cpu.regs[rd] = 0
			} else {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]/operand3, size)
			}
		}
	case OP_DIVS:
		if rd != 0 {
			if operand3 == 0 {
				cpu.regs[rd] = 0
			} else {
				cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])/int64(operand3)), size)
			}
		}
	case OP_MOD64:
		if rd != 0 {
			if operand3 == 0 {
				cpu.regs[rd] = 0
			} else {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]%operand3, size)
			}
		}
	case OP_NEG:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(uint64(-int64(cpu.regs[rs])), size)
		}
	case OP_MODS:
		if rd != 0 {
			a := signExtendToInt64(cpu.regs[rs], size)
			b := signExtendToInt64(operand3, size)
			if b == 0 {
				cpu.regs[rd] = 0
			} else {
				cpu.regs[rd] = maskToSize(uint64(a%b), size)
			}
		}
	case OP_MULHU:
		if rd != 0 {
			hi, _ := bits.Mul64(cpu.regs[rs], operand3)
			cpu.regs[rd] = hi
		}
	case OP_MULHS:
		if rd != 0 {
			cpu.regs[rd] = mulHighSigned(int64(cpu.regs[rs]), int64(operand3))
		}
	case OP_AND64:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]&operand3, size)
		}
	case OP_OR64:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]|operand3, size)
		}
	case OP_EOR:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]^operand3, size)
		}
	case OP_NOT64:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(^cpu.regs[rs], size)
		}
	case OP_LSL:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]<<(operand3&63), size)
		}
	case OP_LSR:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(cpu.regs[rs]>>(operand3&63), size)
		}
	case OP_ASR:
		if rd != 0 {
			shift := operand3 & 63
			var sval int64
			switch size {
			case IE64_SIZE_B:
				sval = int64(int8(cpu.regs[rs]))
			case IE64_SIZE_W:
				sval = int64(int16(cpu.regs[rs]))
			case IE64_SIZE_L:
				sval = int64(int32(cpu.regs[rs]))
			case IE64_SIZE_Q:
				sval = int64(cpu.regs[rs])
			}
			cpu.regs[rd] = maskToSize(uint64(sval>>shift), size)
		}
	case OP_CLZ:
		if rd != 0 {
			cpu.regs[rd] = uint64(bits.LeadingZeros32(uint32(cpu.regs[rs])))
		}
	case OP_SEXT:
		if rd != 0 {
			switch size {
			case IE64_SIZE_B, IE64_SIZE_W, IE64_SIZE_L:
				cpu.regs[rd] = uint64(signExtendToInt64(cpu.regs[rs], size))
			case IE64_SIZE_Q:
				cpu.regs[rd] = cpu.regs[rs]
			}
		}
	case OP_ROL:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(rotateLeftToSize(maskToSize(cpu.regs[rs], size), operand3, size), size)
		}
	case OP_ROR:
		if rd != 0 {
			cpu.regs[rd] = maskToSize(rotateRightToSize(maskToSize(cpu.regs[rs], size), operand3, size), size)
		}
	case OP_CTZ:
		if rd != 0 {
			cpu.regs[rd] = uint64(bits.TrailingZeros32(uint32(cpu.regs[rs])))
		}
	case OP_POPCNT:
		if rd != 0 {
			cpu.regs[rd] = uint64(bits.OnesCount32(uint32(cpu.regs[rs])))
		}
	case OP_BSWAP:
		if rd != 0 {
			cpu.regs[rd] = uint64(bits.ReverseBytes32(uint32(cpu.regs[rs])))
		}
	case OP_BRA:
		cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		pcAdvanced = true
	case OP_BEQ:
		if cpu.regs[rs] == cpu.regs[rt] {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BNE:
		if cpu.regs[rs] != cpu.regs[rt] {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BLT:
		if int64(cpu.regs[rs]) < int64(cpu.regs[rt]) {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BGE:
		if int64(cpu.regs[rs]) >= int64(cpu.regs[rt]) {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BGT:
		if int64(cpu.regs[rs]) > int64(cpu.regs[rt]) {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BLE:
		if int64(cpu.regs[rs]) <= int64(cpu.regs[rt]) {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BHI:
		if cpu.regs[rs] > cpu.regs[rt] {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_BLS:
		if cpu.regs[rs] <= cpu.regs[rt] {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
		} else {
			cpu.PC += IE64_INSTR_SIZE
		}
		pcAdvanced = true
	case OP_JMP:
		target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
		cpu.PC = target & IE64_ADDR_MASK
		pcAdvanced = true
	case OP_JSR64:
		cpu.regs[31] -= 8
		sp := uint32(cpu.regs[31])
		if !cpu.mmuStackWrite(sp, cpu.PC+IE64_INSTR_SIZE, memBase, memSize) {
			if cpu.trapped {
				cpu.trapped = false
				cpu.regs[31] += 8
			}
			pcAdvanced = true
		} else {
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			pcAdvanced = true
		}
	case OP_RTS64:
		sp := uint32(cpu.regs[31])
		val, ok := cpu.mmuStackRead(sp, memBase, memSize)
		if ok {
			cpu.PC = val
			cpu.regs[31] += 8
		} else if cpu.trapped {
			cpu.trapped = false
		}
		pcAdvanced = true
	case OP_PUSH64:
		cpu.regs[31] -= 8
		sp := uint32(cpu.regs[31])
		if !cpu.mmuStackWrite(sp, cpu.regs[rs], memBase, memSize) {
			if cpu.trapped {
				cpu.trapped = false
				cpu.regs[31] += 8
				pcAdvanced = true // trap set PC
			}
		}
	case OP_POP64:
		sp := uint32(cpu.regs[31])
		val, ok := cpu.mmuStackRead(sp, memBase, memSize)
		if ok {
			if rd != 0 {
				cpu.regs[rd] = val
			}
			cpu.regs[31] += 8
		} else if cpu.trapped {
			cpu.trapped = false
			pcAdvanced = true // trap set PC
		}
	case OP_JSR_IND:
		cpu.regs[31] -= 8
		sp := uint32(cpu.regs[31])
		if !cpu.mmuStackWrite(sp, cpu.PC+IE64_INSTR_SIZE, memBase, memSize) {
			if cpu.trapped {
				cpu.trapped = false
				cpu.regs[31] += 8
			}
			pcAdvanced = true
		} else {
			target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.PC = target & IE64_ADDR_MASK
			pcAdvanced = true
		}
	case OP_FMOV:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FPRegs[rd&0x0F] = cpu.FPU.FPRegs[rs&0x0F]
		}
	case OP_FLOAD:
		if cpu.FPU != nil && rd <= 15 {
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			val := uint32(cpu.loadMem(addr, IE64_SIZE_L))
			if cpu.trapped {
				cpu.trapped = false
				pcAdvanced = true
			} else {
				cpu.FPU.FPRegs[rd] = val
				cpu.FPU.setConditionCodesBits(val)
			}
		}
	case OP_FSTORE:
		if cpu.FPU != nil && rd <= 15 {
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, uint64(cpu.FPU.FPRegs[rd]), IE64_SIZE_L)
			if cpu.trapped {
				cpu.trapped = false
				pcAdvanced = true
			}
		}
	case OP_FADD:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FADD(rd, rs, rt)
		}
	case OP_FSUB:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FSUB(rd, rs, rt)
		}
	case OP_FMUL:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FMUL(rd, rs, rt)
		}
	case OP_FDIV:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FDIV(rd, rs, rt)
		}
	case OP_FMOD:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FMOD(rd, rs, rt)
		}
	case OP_FABS:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			b := cpu.FPU.FPRegs[rs&0x0F] & 0x7FFFFFFF
			cpu.FPU.FPRegs[rd&0x0F] = b
			cpu.FPU.setConditionCodesBits(b)
		}
	case OP_FNEG:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			b := cpu.FPU.FPRegs[rs&0x0F] ^ 0x80000000
			cpu.FPU.FPRegs[rd&0x0F] = b
			cpu.FPU.setConditionCodesBits(b)
		}
	case OP_FSQRT:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FSQRT(rd, rs)
		}
	case OP_FINT:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FINT(rd, rs)
		}
	case OP_FCMP:
		if cpu.FPU != nil && rs <= 15 && rt <= 15 {
			res := cpu.FPU.FCMP(rs, rt)
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(res))
			}
		}
	case OP_FCVTIF:
		if cpu.FPU != nil && rd <= 15 {
			cpu.FPU.FCVTIF(rd, cpu.regs[rs])
		}
	case OP_FCVTFI:
		if cpu.FPU != nil && rs <= 15 {
			res := cpu.FPU.FCVTFI(rs)
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(res))
			}
		}
	case OP_FMOVI:
		if cpu.FPU != nil && rd <= 15 {
			cpu.FPU.FMOVI(rd, cpu.regs[rs])
		}
	case OP_FMOVO:
		if cpu.FPU != nil && rs <= 15 {
			res := cpu.FPU.FMOVO(rs)
			if rd != 0 {
				cpu.regs[rd] = res
			}
		}
	case OP_FSIN:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FSIN(rd, rs)
		}
	case OP_FCOS:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FCOS(rd, rs)
		}
	case OP_FTAN:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FTAN(rd, rs)
		}
	case OP_FATAN:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FATAN(rd, rs)
		}
	case OP_FLOG:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FLOG(rd, rs)
		}
	case OP_FEXP:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 {
			cpu.FPU.FEXP(rd, rs)
		}
	case OP_FPOW:
		if cpu.FPU != nil && rd <= 15 && rs <= 15 && rt <= 15 {
			cpu.FPU.FPOW(rd, rs, rt)
		}
	case OP_FMOVECR:
		if cpu.FPU != nil && rd <= 15 {
			cpu.FPU.FMOVECR(rd, byte(imm32))
		}
	case OP_FMOVSR:
		if cpu.FPU != nil && rd != 0 {
			cpu.regs[rd] = uint64(cpu.FPU.FMOVSR())
		}
	case OP_FMOVCR:
		if cpu.FPU != nil && rd != 0 {
			cpu.regs[rd] = uint64(cpu.FPU.FMOVCR())
		}
	case OP_FMOVSC:
		if cpu.FPU != nil {
			cpu.FPU.FMOVSC(uint32(cpu.regs[rs]))
		}
	case OP_FMOVCC:
		if cpu.FPU != nil {
			cpu.FPU.FMOVCC(uint32(cpu.regs[rs]))
		}
	case OP_DMOV:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) {
			cpu.FPU.setDPair(rd, cpu.FPU.getDPair(rs))
		}
	case OP_DLOAD:
		if cpu.FPU != nil && isValidDPairReg(rd) {
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			val := cpu.loadMem(addr, IE64_SIZE_Q)
			if cpu.trapped {
				cpu.trapped = false
				pcAdvanced = true
			} else {
				cpu.FPU.setDPair(rd, math.Float64frombits(val))
				cpu.FPU.setConditionCodesBits64(val)
			}
		}
	case OP_DSTORE:
		if cpu.FPU != nil && isValidDPairReg(rd) {
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, math.Float64bits(cpu.FPU.getDPair(rd)), IE64_SIZE_Q)
			if cpu.trapped {
				cpu.trapped = false
				pcAdvanced = true
			}
		}
	case OP_DADD:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) && isValidDPairReg(rt) {
			cpu.FPU.DADD(rd, rs, rt)
		}
	case OP_DSUB:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) && isValidDPairReg(rt) {
			cpu.FPU.DSUB(rd, rs, rt)
		}
	case OP_DMUL:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) && isValidDPairReg(rt) {
			cpu.FPU.DMUL(rd, rs, rt)
		}
	case OP_DDIV:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) && isValidDPairReg(rt) {
			cpu.FPU.DDIV(rd, rs, rt)
		}
	case OP_DMOD:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) && isValidDPairReg(rt) {
			cpu.FPU.DMOD(rd, rs, rt)
		}
	case OP_DABS:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) {
			cpu.FPU.DABS(rd, rs)
		}
	case OP_DNEG:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) {
			cpu.FPU.DNEG(rd, rs)
		}
	case OP_DSQRT:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) {
			cpu.FPU.DSQRT(rd, rs)
		}
	case OP_DINT:
		if cpu.FPU != nil && isValidDPairReg(rd) && isValidDPairReg(rs) {
			cpu.FPU.DINT(rd, rs)
		}
	case OP_DCMP:
		if cpu.FPU != nil && isValidDPairReg(rs) && isValidDPairReg(rt) && rd != 0 {
			cpu.regs[rd] = uint64(cpu.FPU.DCMP(rs, rt))
		}
	case OP_DCVTIF:
		if cpu.FPU != nil && isValidDPairReg(rd) {
			cpu.FPU.DCVTIF(rd, cpu.regs[rs])
		}
	case OP_DCVTFI:
		if cpu.FPU != nil && isValidDPairReg(rs) && rd != 0 {
			cpu.regs[rd] = uint64(cpu.FPU.DCVTFI(rs))
		}
	case OP_FCVTSD:
		if cpu.FPU != nil && isValidDPairReg(rd) && rs <= 15 {
			cpu.FPU.FCVTSD(rd, rs)
		}
	case OP_FCVTDS:
		if cpu.FPU != nil && rd <= 15 && isValidDPairReg(rs) {
			cpu.FPU.FCVTDS(rd, rs)
		}
	case OP_NOP64:
		// advance PC
	case OP_HALT64:
		// don't advance, CPU halted
		return 1
	case OP_SEI64:
		cpu.interruptEnabled.Store(true)
	case OP_CLI64:
		cpu.interruptEnabled.Store(false)
	case OP_RTI64:
		sp := uint32(cpu.regs[31])
		val, ok := cpu.mmuStackRead(sp, memBase, memSize)
		if ok {
			cpu.PC = val
			cpu.regs[31] += 8
			cpu.inInterrupt.Store(false)
		} else if cpu.trapped {
			cpu.trapped = false
		}
		pcAdvanced = true
	case OP_WAIT64:
		// In step mode, just skip the wait

	// MMU / Privilege
	case OP_MTCR:
		if !cpu.requireSupervisor() {
			pcAdvanced = true // trap set PC
		} else {
			crIdx := rd
			val := cpu.regs[rs]
			switch crIdx {
			case CR_PTBR:
				cpu.ptbr = uint32(val)
				cpu.tlbFlush()
				cpu.jitNeedInval = true
			case CR_FAULT_ADDR:
				cpu.faultAddr = uint32(val)
			case CR_FAULT_CAUSE:
				cpu.faultCause = uint32(val)
			case CR_FAULT_PC:
				cpu.faultPC = val
			case CR_TRAP_VEC:
				cpu.trapVector = val
			case CR_MMU_CTRL:
				// See MMU_CTRL_* bit constants. Bit 1 (supervisor) is
				// read-only; bit 4 (SUA latch) is writable only by
				// SUAEN/SUADIS.
				newMMU := val&MMU_CTRL_ENABLE != 0
				if newMMU != cpu.mmuEnabled {
					cpu.mmuEnabled = newMMU
					cpu.tlbFlush()
					cpu.jitNeedInval = true
				}
				cpu.skef = val&MMU_CTRL_SKEF != 0
				cpu.skac = val&MMU_CTRL_SKAC != 0
			case CR_TP:
				cpu.threadPointer = val
			case CR_INTR_VEC:
				cpu.intrVector = val
			case CR_KSP:
				cpu.kernelSP = val
			case CR_TIMER_PERIOD:
				cpu.timerPeriod.Store(val)
			case CR_TIMER_COUNT:
				cpu.timerCount.Store(val)
			case CR_TIMER_CTRL:
				cpu.timerEnabled.Store(val&1 != 0)
				cpu.interruptEnabled.Store(val&2 != 0)
			case CR_USP:
				cpu.userSP = val
			case CR_SAVED_SUA:
				// Kernel restores outer-trap SUA before ERET.
				cpu.savedSUA = val&1 != 0
			}
		}
	case OP_MFCR:
		// CR_TP is readable from user mode; all others require supervisor
		crIdx := rs
		if crIdx != CR_TP && !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			var val uint64
			switch crIdx {
			case CR_PTBR:
				val = uint64(cpu.ptbr)
			case CR_FAULT_ADDR:
				val = uint64(cpu.faultAddr)
			case CR_FAULT_CAUSE:
				val = uint64(cpu.faultCause)
			case CR_FAULT_PC:
				val = cpu.faultPC
			case CR_TRAP_VEC:
				val = cpu.trapVector
			case CR_MMU_CTRL:
				val = 0
				if cpu.mmuEnabled {
					val |= MMU_CTRL_ENABLE
				}
				if cpu.supervisorMode {
					val |= MMU_CTRL_SUPER
				}
				if cpu.skef {
					val |= MMU_CTRL_SKEF
				}
				if cpu.skac {
					val |= MMU_CTRL_SKAC
				}
				if cpu.suaLatch {
					val |= MMU_CTRL_SUA
				}
			case CR_TP:
				val = cpu.threadPointer
			case CR_INTR_VEC:
				val = cpu.intrVector
			case CR_KSP:
				val = cpu.kernelSP
			case CR_TIMER_PERIOD:
				val = cpu.timerPeriod.Load()
			case CR_TIMER_COUNT:
				val = cpu.timerCount.Load()
			case CR_TIMER_CTRL:
				val = 0
				if cpu.timerEnabled.Load() {
					val |= 1
				}
				if cpu.interruptEnabled.Load() {
					val |= 2
				}
			case CR_USP:
				val = cpu.userSP
			case CR_PREV_MODE:
				if cpu.previousMode {
					val = 1
				}
			case CR_SAVED_SUA:
				if cpu.savedSUA {
					val = 1
				}
			}
			if rd != 0 {
				cpu.regs[rd] = val
			}
		}
	case OP_ERET:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			// Consume active frame.
			cpu.PC = cpu.faultPC
			if !cpu.previousMode {
				cpu.kernelSP = cpu.regs[31]
				cpu.regs[31] = cpu.userSP
				cpu.supervisorMode = false
				cpu.interruptEnabled.Store(true)
				// M15.6 G2: user mode must never have SUA set.
				cpu.suaLatch = false
			} else {
				// Nested supervisor return: restore the interrupted
				// code path's SUA latch so an in-flight copy helper
				// resumes inside its SUAEN/SUADIS region.
				cpu.suaLatch = cpu.savedSUA
			}
			// Pop the outer frame; active state becomes the caller trap
			// (or fresh zeros if we were the outermost trap).
			cpu.popTrapFrame()
			pcAdvanced = true
		}
	case OP_TLBFLUSH:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			cpu.tlbFlush()
			cpu.jitNeedInval = true
		}
	case OP_TLBINVAL:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			vpn := uint16(cpu.regs[rs] >> MMU_PAGE_SHIFT)
			cpu.tlbInvalidate(vpn)
			cpu.jitNeedInval = true
		}
	case OP_SYSCALL:
		cpu.trapSyscall(imm32)
		pcAdvanced = true
	case OP_SMODE:
		if rd != 0 {
			if cpu.supervisorMode {
				cpu.regs[rd] = 1
			} else {
				cpu.regs[rd] = 0
			}
		}
	case OP_SUAEN:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			cpu.suaLatch = true
		}
	case OP_SUADIS:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			cpu.suaLatch = false
		}

	// Atomic Memory RMW
	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		cpu.execAtomic(rd, rs, rt, imm32, opcode)
		if cpu.trapped {
			cpu.trapped = false
			pcAdvanced = true
		}

	default:
		return 0
	}

	if !pcAdvanced {
		cpu.PC += IE64_INSTR_SIZE
	}
	return 1
}
