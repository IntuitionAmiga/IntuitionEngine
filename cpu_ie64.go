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
	CR_PTBR        = 0 // Page Table Base Register (physical address)
	CR_FAULT_ADDR  = 1 // Virtual address that caused last fault
	CR_FAULT_CAUSE = 2 // Fault cause code
	CR_FAULT_PC    = 3 // PC saved at trap entry
	CR_TRAP_VEC    = 4 // Trap handler vector address
	CR_MMU_CTRL    = 5 // Bit 0 = MMU enable (RW), Bit 1 = supervisor mode (RO)
	CR_TP          = 6 // Thread Pointer (user-readable, supervisor-writable)
	CR_COUNT       = 7 // Number of control registers
)

// Fault cause codes
const (
	FAULT_NOT_PRESENT  = 0 // PTE P bit = 0
	FAULT_READ_DENIED  = 1 // PTE R bit = 0 on read access
	FAULT_WRITE_DENIED = 2 // PTE W bit = 0 on write access
	FAULT_EXEC_DENIED  = 3 // PTE X bit = 0 on instruction fetch
	FAULT_USER_SUPER   = 4 // PTE U bit = 0 in user mode
	FAULT_PRIV         = 5 // Privileged instruction in user mode
	FAULT_SYSCALL      = 6 // SYSCALL instruction
	FAULT_MISALIGNED   = 7 // Misaligned atomic access
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

	// Logic
	OP_AND64 = 0x30 // Bitwise AND
	OP_OR64  = 0x31 // Bitwise OR
	OP_EOR   = 0x32 // Bitwise XOR
	OP_NOT64 = 0x33 // Bitwise NOT
	OP_LSL   = 0x34 // Logical shift left
	OP_LSR   = 0x35 // Logical shift right
	OP_ASR   = 0x36 // Arithmetic shift right
	OP_CLZ   = 0x37 // Count leading zeros

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
	ptbr           uint32       // Page Table Base Register (physical address)
	trapVector     uint64       // Trap handler entry point
	faultPC        uint64       // PC saved at trap entry
	faultAddr      uint32       // Virtual address that caused fault
	faultCause     uint32       // Fault cause code
	trapped        bool         // Set by memory helpers on MMU fault; checked by Execute/StepOne
	jitNeedInval   bool         // Set by MMU ops; consumed by JIT dispatcher
	tlb            [64]TLBEntry // 64-entry direct-mapped software TLB
	threadPointer  uint64       // Thread Pointer (CR_TP)
}

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

// trapFault handles involuntary traps (page fault, privilege violation).
// Saves PC of the faulting instruction so ERET re-executes it.
func (cpu *CPU64) trapFault(cause uint32, addr uint32) {
	cpu.faultPC = cpu.PC // re-execute on ERET
	cpu.faultAddr = addr
	cpu.faultCause = cause
	cpu.supervisorMode = true
	cpu.PC = cpu.trapVector
}

// trapSyscall handles SYSCALL. Saves PC+8 so ERET skips the SYSCALL.
// The syscall number (imm32) is stored in faultAddr for the handler to read
// via MFCR CR1. User convention: arguments in R1-R6 before SYSCALL.
func (cpu *CPU64) trapSyscall(syscallNum uint32) {
	cpu.faultPC = cpu.PC + IE64_INSTR_SIZE // skip SYSCALL on ERET
	cpu.faultAddr = syscallNum             // handler reads via MFCR CR_FAULT_ADDR
	cpu.faultCause = FAULT_SYSCALL
	cpu.supervisorMode = true
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
	cpu.ptbr = 0
	cpu.trapVector = 0
	cpu.faultPC = 0
	cpu.faultAddr = 0
	cpu.faultCause = 0
	cpu.trapped = false
	cpu.jitNeedInval = false
	cpu.threadPointer = 0

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

		// Timer handling with lock-free atomics
		if cpu.timerEnabled.Load() {
			cpu.cycleCounter++
			if cpu.cycleCounter >= SAMPLE_RATE {
				cpu.cycleCounter = 0

				count := cpu.timerCount.Load()
				if count > 0 {
					newCount := count - 1
					cpu.timerCount.Store(newCount)
					if newCount == 0 {
						cpu.timerState.Store(TIMER_EXPIRED)
						// Inline handleInterrupt - uses memBase/memSize locals
						if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
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
						}
						// Reload timer if still enabled (handler may have disabled it)
						if cpu.timerEnabled.Load() {
							cpu.timerCount.Store(cpu.timerPeriod.Load())
						}
					}
				} else {
					period := cpu.timerPeriod.Load()
					if period > 0 {
						cpu.timerCount.Store(period)
						cpu.timerState.Store(TIMER_RUNNING)
					}
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
				// Bit 0 = mmuEnabled (writable), Bit 1 = supervisor (read-only, ignored)
				newMMU := val&1 != 0
				if newMMU != cpu.mmuEnabled {
					cpu.mmuEnabled = newMMU
					cpu.tlbFlush()
					cpu.jitNeedInval = true
				}
			case CR_TP:
				cpu.threadPointer = val
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
					val |= 1
				}
				if cpu.supervisorMode {
					val |= 2
				}
			case CR_TP:
				val = cpu.threadPointer
			}
			if rd != 0 {
				cpu.regs[rd] = val
			}

		case OP_ERET:
			if !cpu.requireSupervisor() {
				continue
			}
			cpu.PC = cpu.faultPC
			cpu.supervisorMode = false
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
				newMMU := val&1 != 0
				if newMMU != cpu.mmuEnabled {
					cpu.mmuEnabled = newMMU
					cpu.tlbFlush()
					cpu.jitNeedInval = true
				}
			case CR_TP:
				cpu.threadPointer = val
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
					val |= 1
				}
				if cpu.supervisorMode {
					val |= 2
				}
			case CR_TP:
				val = cpu.threadPointer
			}
			if rd != 0 {
				cpu.regs[rd] = val
			}
		}
	case OP_ERET:
		if !cpu.requireSupervisor() {
			pcAdvanced = true
		} else {
			cpu.PC = cpu.faultPC
			cpu.supervisorMode = false
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
