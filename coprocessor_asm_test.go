package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// Group B: Gateway adapter routing tests
// =============================================================================

// TestCoprocZ80Window_Write verifies that Z80BusAdapter.Write(0xF204, val)
// reaches COPROC_CPU_TYPE on the bus.
func TestCoprocZ80Window_Write(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	z80bus := NewZ80BusAdapter(bus)

	// Write to gateway offset for COPROC_CPU_TYPE (offset 0x04 from COPROC_BASE)
	// Gateway: 0xF200 + 0x04 = 0xF204
	z80bus.Write(0xF204, byte(EXEC_TYPE_IE32))

	mgr.mu.Lock()
	got := mgr.cpuType
	mgr.mu.Unlock()

	// We wrote only byte 0, so cpuType should have that byte set
	if got&0xFF != uint32(EXEC_TYPE_IE32) {
		t.Fatalf("Z80 gateway write: expected cpuType byte 0 = %d, got cpuType = 0x%08X",
			EXEC_TYPE_IE32, got)
	}
}

// TestCoprocZ80Window_Read verifies that Z80BusAdapter.Read(0xF208) returns
// COPROC_CMD_STATUS value.
func TestCoprocZ80Window_Read(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	z80bus := NewZ80BusAdapter(bus)

	// Set CMD_STATUS directly
	mgr.mu.Lock()
	mgr.cmdStatus = COPROC_STATUS_ERROR
	mgr.mu.Unlock()

	// Read via gateway: COPROC_CMD_STATUS is at offset 0x08
	got := z80bus.Read(0xF208)
	if got != byte(COPROC_STATUS_ERROR) {
		t.Fatalf("Z80 gateway read: expected %d, got %d", COPROC_STATUS_ERROR, got)
	}
}

// TestCoprocZ80Window_ByteCompose writes 4 bytes at 0xF21C..0xF21F to compose
// COPROC_REQ_PTR and verifies the correct 32-bit value.
func TestCoprocZ80Window_ByteCompose(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	z80bus := NewZ80BusAdapter(bus)

	// COPROC_REQ_PTR is at offset 0x1C from COPROC_BASE
	// Gateway address: 0xF200 + 0x1C = 0xF21C
	// Write 0xCAFEBABE in little-endian byte order
	z80bus.Write(0xF21C, 0xBE) // byte 0
	z80bus.Write(0xF21D, 0xBA) // byte 1
	z80bus.Write(0xF21E, 0xFE) // byte 2
	z80bus.Write(0xF21F, 0xCA) // byte 3

	mgr.mu.Lock()
	got := mgr.reqPtr
	mgr.mu.Unlock()

	if got != 0xCAFEBABE {
		t.Fatalf("Z80 byte compose: expected 0xCAFEBABE, got 0x%08X", got)
	}
}

// TestCoproc6502Window_Write verifies that Bus6502Adapter.Write(0xF204, val)
// reaches COPROC_CPU_TYPE on the bus.
func TestCoproc6502Window_Write(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	adapter := NewBus6502Adapter(bus)

	adapter.Write(0xF204, byte(EXEC_TYPE_M68K))

	mgr.mu.Lock()
	got := mgr.cpuType
	mgr.mu.Unlock()

	if got&0xFF != uint32(EXEC_TYPE_M68K) {
		t.Fatalf("6502 gateway write: expected cpuType byte 0 = %d, got cpuType = 0x%08X",
			EXEC_TYPE_M68K, got)
	}
}

// TestCoproc6502Window_Read verifies that Bus6502Adapter.Read(0xF208) returns
// COPROC_CMD_STATUS value.
func TestCoproc6502Window_Read(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	adapter := NewBus6502Adapter(bus)

	mgr.mu.Lock()
	mgr.cmdStatus = COPROC_STATUS_ERROR
	mgr.mu.Unlock()

	got := adapter.Read(0xF208)
	if got != byte(COPROC_STATUS_ERROR) {
		t.Fatalf("6502 gateway read: expected %d, got %d", COPROC_STATUS_ERROR, got)
	}
}

// TestCoproc6502Window_ByteCompose writes 4 bytes to compose COPROC_REQ_PTR
// and verifies the correct 32-bit value via 6502 adapter.
func TestCoproc6502Window_ByteCompose(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	adapter := NewBus6502Adapter(bus)

	adapter.Write(0xF21C, 0xBE)
	adapter.Write(0xF21D, 0xBA)
	adapter.Write(0xF21E, 0xFE)
	adapter.Write(0xF21F, 0xCA)

	mgr.mu.Lock()
	got := mgr.reqPtr
	mgr.mu.Unlock()

	if got != 0xCAFEBABE {
		t.Fatalf("6502 byte compose: expected 0xCAFEBABE, got 0x%08X", got)
	}
}

// =============================================================================
// Group C: Mapping availability tests
// =============================================================================

// TestCoprocMappedInIE32Mode verifies MMIO is reachable from IE32's 32-bit bus.
func TestCoprocMappedInIE32Mode(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	_ = mgr

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	got := bus.Read32(COPROC_CPU_TYPE)
	if got != EXEC_TYPE_IE32 {
		t.Fatalf("IE32 mapping: expected %d, got %d", EXEC_TYPE_IE32, got)
	}
}

// TestCoprocMappedInM68KMode verifies MMIO is reachable via 32-bit bus (M68K).
func TestCoprocMappedInM68KMode(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	_ = mgr

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	got := bus.Read32(COPROC_CPU_TYPE)
	if got != EXEC_TYPE_M68K {
		t.Fatalf("M68K mapping: expected %d, got %d", EXEC_TYPE_M68K, got)
	}
}

// TestCoprocMappedInX86Mode verifies MMIO reachable via X86BusAdapter byte writes.
func TestCoprocMappedInX86Mode(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	x86bus := &X86BusAdapter{bus: bus}

	// x86 uses 32-bit addresses directly, writing byte-by-byte
	x86bus.Write(COPROC_CPU_TYPE+0, byte(EXEC_TYPE_X86))
	x86bus.Write(COPROC_CPU_TYPE+1, 0)
	x86bus.Write(COPROC_CPU_TYPE+2, 0)
	x86bus.Write(COPROC_CPU_TYPE+3, 0)

	mgr.mu.Lock()
	got := mgr.cpuType
	mgr.mu.Unlock()

	if got != EXEC_TYPE_X86 {
		t.Fatalf("X86 mapping: expected %d, got 0x%08X", EXEC_TYPE_X86, got)
	}
}

// TestCoprocMappedInZ80Mode verifies MMIO reachable via Z80BusAdapter gateway.
func TestCoprocMappedInZ80Mode(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	z80bus := NewZ80BusAdapter(bus)

	z80bus.Write(COPROC_GATEWAY_BASE+0x04, byte(EXEC_TYPE_Z80))

	mgr.mu.Lock()
	got := mgr.cpuType
	mgr.mu.Unlock()

	if got&0xFF != EXEC_TYPE_Z80 {
		t.Fatalf("Z80 mapping: expected %d, got 0x%08X", EXEC_TYPE_Z80, got)
	}
}

// TestCoprocMappedIn6502Mode verifies MMIO reachable via Bus6502Adapter gateway.
func TestCoprocMappedIn6502Mode(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	adapter := NewBus6502Adapter(bus)

	adapter.Write(COPROC_GATEWAY_BASE+0x04, byte(EXEC_TYPE_6502))

	mgr.mu.Lock()
	got := mgr.cpuType
	mgr.mu.Unlock()

	if got&0xFF != EXEC_TYPE_6502 {
		t.Fatalf("6502 mapping: expected %d, got 0x%08X", EXEC_TYPE_6502, got)
	}
}

// =============================================================================
// Group D: Caller plumbing tests (hand-assembled CPU programs)
// =============================================================================

// TestCoprocCallerPlumbing_IE32 tests MMIO write path from an IE32 program.
// Writes to COPROC registers, reads status, stores result, HALTs.
func TestCoprocCallerPlumbing_IE32(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Pre-create a dummy worker + completed ticket
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.completions[42] = &CoprocCompletion{
		ticket:  42,
		cpuType: EXEC_TYPE_IE32,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}
	mgr.mu.Unlock()

	// Write response in ring so scanTicketStatus finds it
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, 42)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// IE32 program: write ticket=42 to COPROC_TICKET, write CMD=POLL,
	// read TICKET_STATUS, store to 0x500000, HALT
	var code []byte
	appendInstr := func(opcode, reg, addrMode byte, operand uint32) {
		instr := ie32Instr(opcode, reg, addrMode, operand)
		code = append(code, instr[:]...)
	}

	appendInstr(ie32_LOAD, 0, ie32_ADDR_IMM, 42)                      // A = 42
	appendInstr(ie32_STORE, 0, ie32_ADDR_DIRECT, COPROC_TICKET)       // COPROC_TICKET = 42
	appendInstr(ie32_LOAD, 0, ie32_ADDR_IMM, COPROC_CMD_POLL)         // A = POLL cmd
	appendInstr(ie32_STORE, 0, ie32_ADDR_DIRECT, COPROC_CMD)          // trigger poll
	appendInstr(ie32_LOAD, 0, ie32_ADDR_DIRECT, COPROC_TICKET_STATUS) // A = status
	appendInstr(ie32_STORE, 0, ie32_ADDR_DIRECT, 0x500000)            // store result
	appendInstr(ie32_HALT, 0, ie32_ADDR_IMM, 0)                       // HALT

	// Load and execute
	cpu := NewCPU(bus)
	copy(bus.GetMemory()[PROG_START:], code)
	cpu.PC = PROG_START
	go cpu.Execute()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cpu.running.Store(false)
			t.Fatal("IE32 program timed out")
		default:
		}
		if !cpu.IsRunning() {
			break
		}
		time.Sleep(time.Millisecond)
	}

	result := bus.Read32(0x500000)
	if result != COPROC_TICKET_OK {
		t.Fatalf("IE32 plumbing: expected status %d, got %d", COPROC_TICKET_OK, result)
	}
}

// TestCoprocCallerPlumbing_Z80 tests MMIO write path from a Z80 program via gateway.
func TestCoprocCallerPlumbing_Z80(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Pre-create completed ticket
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.completions[1] = &CoprocCompletion{
		ticket:  1,
		cpuType: EXEC_TYPE_IE32,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}
	mgr.mu.Unlock()

	// Write response in ring
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, 1)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// Z80 program using gateway at 0xF200:
	// LD A, 1 ; ticket = 1
	// LD (0xF210), A ; write to COPROC_TICKET (gateway offset 0x10)
	// LD A, 4 ; CMD_POLL
	// LD (0xF200), A ; write to COPROC_CMD (gateway offset 0x00)
	// LD A, (0xF214) ; read COPROC_TICKET_STATUS (gateway offset 0x14)
	// LD (0x8000), A ; store result at 0x8000
	// HALT
	code := []byte{
		0x3E, 0x01, // LD A, 1
		0x32, 0x10, 0xF2, // LD (0xF210), A
		0x3E, byte(COPROC_CMD_POLL), // LD A, 4
		0x32, 0x00, 0xF2, // LD (0xF200), A
		0x3A, 0x14, 0xF2, // LD A, (0xF214)
		0x32, 0x00, 0x80, // LD (0x8000), A
		0x76, // HALT
	}

	z80adapter := NewZ80BusAdapter(bus)

	// Load code at address 0x0000
	for i, b := range code {
		bus.Write8(uint32(i), b)
	}

	z80cpu := NewCPU_Z80(z80adapter)
	go z80cpu.Execute()

	// Z80 HALT doesn't stop execution — it just spins in NOP cycles.
	// Poll Halted flag to detect completion, then stop.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			z80cpu.SetRunning(false)
			t.Fatal("Z80 program timed out")
		default:
		}
		if z80cpu.Halted {
			z80cpu.SetRunning(false)
			break
		}
		time.Sleep(time.Millisecond)
	}

	result := bus.Read8(0x8000)
	if result != byte(COPROC_TICKET_OK) {
		t.Fatalf("Z80 plumbing: expected status %d, got %d", COPROC_TICKET_OK, result)
	}
}

// TestCoprocCallerPlumbing_6502 tests MMIO write path from a 6502 program via gateway.
func TestCoprocCallerPlumbing_6502(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Pre-create completed ticket
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.completions[1] = &CoprocCompletion{
		ticket:  1,
		cpuType: EXEC_TYPE_IE32,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}
	mgr.mu.Unlock()

	// Write response in ring
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, 1)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// 6502 program using gateway at $F200:
	// LDA #1            ; ticket = 1
	// STA $F210         ; write to COPROC_TICKET (gateway offset $10)
	// LDA #4            ; CMD_POLL
	// STA $F200         ; write to COPROC_CMD (gateway offset $00)
	// LDA $F214         ; read COPROC_TICKET_STATUS (gateway offset $14)
	// STA $0200         ; store result at $0200
	// STA $0201         ; sentinel: write to $0201 to signal done
	// JMP $-3           ; infinite loop (spin)
	code := []byte{
		0xA9, 0x01, // LDA #1
		0x8D, 0x10, 0xF2, // STA $F210
		0xA9, byte(COPROC_CMD_POLL), // LDA #4
		0x8D, 0x00, 0xF2, // STA $F200
		0xAD, 0x14, 0xF2, // LDA $F214
		0x8D, 0x00, 0x02, // STA $0200
		0x8D, 0x01, 0x02, // STA $0201 (sentinel)
		0x4C, 0x12, 0x06, // JMP $0612 (self-loop)
	}

	// Load code at $0600
	loadAddr := uint16(0x0600)
	for i, b := range code {
		bus.Write8(uint32(loadAddr)+uint32(i), b)
	}

	// NewCPU_6502 takes Bus32 (MachineBus) and creates its own Bus6502Adapter
	// internally, which has the gateway checks
	cpu6502 := NewCPU_6502(bus)
	cpu6502.PC = loadAddr

	go cpu6502.Execute()

	// Poll sentinel address $0201 for non-zero to detect completion
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cpu6502.SetRunning(false)
			t.Fatal("6502 program timed out")
		default:
		}
		if bus.Read8(0x0201) != 0 {
			cpu6502.SetRunning(false)
			break
		}
		time.Sleep(time.Millisecond)
	}

	result := bus.Read8(0x0200)
	if result != byte(COPROC_TICKET_OK) {
		t.Fatalf("6502 plumbing: expected status %d, got %d", COPROC_TICKET_OK, result)
	}
}

// TestCoprocCallerPlumbing_M68K tests MMIO write path from an M68K program.
// Hand-assembled M68K using MOVE.L instructions to write/read COPROC registers.
func TestCoprocCallerPlumbing_M68K(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Pre-create completed ticket
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.completions[42] = &CoprocCompletion{
		ticket:  42,
		cpuType: EXEC_TYPE_IE32,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}
	mgr.mu.Unlock()

	// Write response in ring
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, 42)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// M68K program (big-endian byte order — CPU does LE read + ReverseBytes16):
	//   MOVE.L #42, ($F2350).L       — write ticket = 42
	//   MOVE.L #4,  ($F2340).L       — write CMD = POLL (triggers dispatch)
	//   STOP #$2700                  — halt (waits for interrupt)
	//
	// We verify the MMIO writes landed correctly from Go, then read
	// TICKET_STATUS directly via bus.Read32 (avoiding M68K big-endian
	// byte-swap on MMIO reads which requires CoprocMode).
	code := []byte{
		// MOVE.L #42, ($000F2350).L  — opcode 0x23FC
		0x23, 0xFC,
		0x00, 0x00, 0x00, 0x2A, // imm32 = 42
		0x00, 0x0F, 0x23, 0x50, // addr32 = 0x000F2350
		// MOVE.L #4, ($000F2340).L   — opcode 0x23FC
		0x23, 0xFC,
		0x00, 0x00, 0x00, 0x04, // imm32 = 4 (POLL)
		0x00, 0x0F, 0x23, 0x40, // addr32 = 0x000F2340
		// STOP #$2700 — halt CPU (waits for interrupt, none will come)
		0x4E, 0x72,
		0x27, 0x00, // SR = 0x2700 (supervisor, IPL=7)
	}

	// Load at 0x1000 (within fast-path range < 0xA0000)
	copy(bus.GetMemory()[0x1000:], code)

	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.SR = 0x2700 // supervisor mode (required for STOP)
	cpu.AddrRegs[7] = 0x80000
	go cpu.ExecuteInstruction()

	// Wait for CPU to reach STOP state
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cpu.SetRunning(false)
			t.Fatal("M68K program timed out")
		default:
		}
		if cpu.stopped.Load() {
			cpu.SetRunning(false)
			break
		}
		time.Sleep(time.Millisecond)
	}

	// The M68K program wrote ticket=42 and CMD=POLL to the MMIO registers.
	// Write32 at I/O addresses (0xF0000-0x100000) passes values without
	// byte-swap, so the MMIO handler received the correct values.
	// Verify the poll was dispatched and TICKET_STATUS is readable.
	result := bus.Read32(COPROC_TICKET_STATUS)
	if result != COPROC_TICKET_OK {
		t.Fatalf("M68K plumbing: expected status %d, got %d", COPROC_TICKET_OK, result)
	}
}

// TestCoprocCallerPlumbing_X86 tests MMIO write path from an x86 program.
// Hand-assembled x86 using MOV instructions to write/read COPROC registers.
func TestCoprocCallerPlumbing_X86(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Pre-create completed ticket
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.completions[42] = &CoprocCompletion{
		ticket:  42,
		cpuType: EXEC_TYPE_IE32,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}
	mgr.mu.Unlock()

	// Write response in ring
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, 42)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// x86 program (little-endian, native byte order):
	//   MOV DWORD PTR [0xF2350], 42   — write ticket
	//   MOV DWORD PTR [0xF2340], 4    — write CMD = POLL
	//   MOV EAX, [0xF2354]            — read TICKET_STATUS
	//   MOV [0x500000], EAX           — store result
	//   HLT                           — halt
	code := []byte{
		// MOV DWORD PTR [disp32], imm32 — opcode C7 05
		0xC7, 0x05,
		0x50, 0x23, 0x0F, 0x00, // addr = 0x000F2350
		0x2A, 0x00, 0x00, 0x00, // imm  = 42
		// MOV DWORD PTR [disp32], imm32
		0xC7, 0x05,
		0x40, 0x23, 0x0F, 0x00, // addr = 0x000F2340
		0x04, 0x00, 0x00, 0x00, // imm  = 4 (POLL)
		// MOV EAX, moffs32 — opcode A1
		0xA1,
		0x54, 0x23, 0x0F, 0x00, // addr = 0x000F2354
		// MOV moffs32, EAX — opcode A3
		0xA3,
		0x00, 0x00, 0x50, 0x00, // addr = 0x00500000
		// HLT
		0xF4,
	}

	// Load at WORKER_X86_BASE
	copy(bus.GetMemory()[WORKER_X86_BASE:], code)

	x86bus := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(x86bus)
	cpu.EIP = WORKER_X86_BASE
	go func() {
		for cpu.Running() {
			cpu.Step()
		}
	}()

	// x86 HLT sets Halted=true but doesn't stop Execute loop
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			cpu.SetRunning(false)
			t.Fatal("x86 program timed out")
		default:
		}
		if cpu.Halted {
			cpu.SetRunning(false)
			break
		}
		time.Sleep(time.Millisecond)
	}

	result := bus.Read32(0x500000)
	if result != COPROC_TICKET_OK {
		t.Fatalf("x86 plumbing: expected status %d, got %d", COPROC_TICKET_OK, result)
	}
}

// =============================================================================
// Group E: Worker lifecycle E2E tests
// =============================================================================

// TestCoprocWorkerE2E_IE32Master tests a complete IE32 master → IE32 worker cycle.
// The master enqueues an add-request, the worker processes it via bus memory
// simulation, and the master polls until OK and reads the response.
func TestCoprocWorkerE2E_IE32Master(t *testing.T) {
	tmpDir := t.TempDir()
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, tmpDir)
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Build worker binary (NOP loop)
	workerCode := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))

	// Write to temp file
	svcPath := filepath.Join(tmpDir, "svc.ie32")
	if err := os.WriteFile(svcPath, workerCode, 0644); err != nil {
		t.Fatal(err)
	}

	// Write filename to bus memory
	writeString(bus, 0x400000, "svc.ie32")

	// START worker
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_NAME_PTR, 0x400000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("START failed: status=%d err=%d", status, errCode)
	}

	// Write request data: two uint32s (10, 20)
	bus.Write32(0x410000, 10)
	bus.Write32(0x410004, 20)

	// ENQUEUE request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x410000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x410100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	status = bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("ENQUEUE failed: status=%d err=%d", status, errCode)
	}

	ticket := bus.Read32(COPROC_TICKET)

	// Since the worker is a NOP loop (can't actually process), simulate the
	// worker completing the request by writing the response directly.
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respDescAddr := ringBase + RING_RESPONSES_OFFSET + 0*RESP_DESC_SIZE
	bus.Write32(respDescAddr+RESP_TICKET_OFF, ticket)
	bus.Write32(respDescAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)
	bus.Write32(respDescAddr+RESP_RESULT_CODE_OFF, 0)
	bus.Write32(respDescAddr+RESP_RESP_LEN_OFF, 4)
	bus.Write32(0x410100, 30) // write result

	// POLL for result
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)

	ticketStatus := bus.Read32(COPROC_TICKET_STATUS)
	if ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("expected OK status, got %d", ticketStatus)
	}

	result := bus.Read32(0x410100)
	if result != 30 {
		t.Fatalf("expected result 30, got %d", result)
	}

	// Cleanup
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_CMD, COPROC_CMD_STOP)
}

// TestCoprocWorkerE2E_Z80Master tests a Z80 master starting an IE32 worker
// via gateway and driving the full lifecycle.
func TestCoprocWorkerE2E_Z80Master(t *testing.T) {
	tmpDir := t.TempDir()
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, tmpDir)
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Build worker binary (NOP loop)
	workerCode := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))

	svcPath := filepath.Join(tmpDir, "svc.ie32")
	if err := os.WriteFile(svcPath, workerCode, 0644); err != nil {
		t.Fatal(err)
	}

	z80bus := NewZ80BusAdapter(bus)

	// Write filename "svc.ie32" to bus memory at 0x400000
	writeString(bus, 0x400000, "svc.ie32")

	// Write NAME_PTR = 0x400000 via gateway (byte by byte, offset 0x30)
	z80bus.Write(0xF230, 0x00) // byte 0
	z80bus.Write(0xF231, 0x00) // byte 1
	z80bus.Write(0xF232, 0x40) // byte 2
	z80bus.Write(0xF233, 0x00) // byte 3

	// Write CPU_TYPE = EXEC_TYPE_IE32 (=1) via gateway (offset 0x04)
	z80bus.Write(0xF204, byte(EXEC_TYPE_IE32))
	z80bus.Write(0xF205, 0)
	z80bus.Write(0xF206, 0)
	z80bus.Write(0xF207, 0)

	// Write CMD = START (=1) via gateway byte 0 (offset 0x00)
	z80bus.Write(0xF200, COPROC_CMD_START)

	// Read CMD_STATUS via gateway (offset 0x08)
	cmdStatus := z80bus.Read(0xF208)
	if cmdStatus != byte(COPROC_STATUS_OK) {
		cmdErr := z80bus.Read(0xF20C)
		t.Fatalf("Z80 START failed: status=%d err=%d", cmdStatus, cmdErr)
	}

	// ENQUEUE via gateway (all parameters byte-by-byte)
	// OP = 1 (offset 0x18)
	z80bus.Write(0xF218, 1)
	z80bus.Write(0xF219, 0)
	z80bus.Write(0xF21A, 0)
	z80bus.Write(0xF21B, 0)

	// REQ_PTR = 0x410000 (offset 0x1C)
	putLE32ViaZ80(z80bus, 0xF21C, 0x410000)
	// REQ_LEN = 8 (offset 0x20)
	putLE32ViaZ80(z80bus, 0xF220, 8)
	// RESP_PTR = 0x410100 (offset 0x24)
	putLE32ViaZ80(z80bus, 0xF224, 0x410100)
	// RESP_CAP = 16 (offset 0x28)
	putLE32ViaZ80(z80bus, 0xF228, 16)

	// CPU_TYPE already set; trigger ENQUEUE
	z80bus.Write(0xF200, COPROC_CMD_ENQUEUE)

	cmdStatus = z80bus.Read(0xF208)
	if cmdStatus != byte(COPROC_STATUS_OK) {
		t.Fatalf("Z80 ENQUEUE failed: status=%d", cmdStatus)
	}

	// Read ticket (4 bytes, offset 0x10)
	ticket := getLE32ViaZ80(z80bus, 0xF210)
	if ticket == 0 {
		t.Fatal("expected non-zero ticket")
	}

	// Simulate worker completing
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respDescAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(respDescAddr+RESP_TICKET_OFF, ticket)
	bus.Write32(respDescAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// POLL via gateway
	putLE32ViaZ80(z80bus, 0xF210, ticket) // write ticket
	z80bus.Write(0xF200, COPROC_CMD_POLL) // trigger POLL

	ticketStatus := z80bus.Read(0xF214) // read TICKET_STATUS
	if ticketStatus != byte(COPROC_TICKET_OK) {
		t.Fatalf("Z80 POLL: expected OK (%d), got %d", COPROC_TICKET_OK, ticketStatus)
	}

	// Cleanup
	z80bus.Write(0xF204, byte(EXEC_TYPE_IE32))
	z80bus.Write(0xF200, COPROC_CMD_STOP)
}

// putLE32ViaZ80 writes a 32-bit value in little-endian via 4 Z80 byte writes.
func putLE32ViaZ80(z80bus *Z80BusAdapter, baseAddr uint16, val uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], val)
	for i := range 4 {
		z80bus.Write(baseAddr+uint16(i), buf[i])
	}
}

// getLE32ViaZ80 reads a 32-bit value in little-endian via 4 Z80 byte reads.
func getLE32ViaZ80(z80bus *Z80BusAdapter, baseAddr uint16) uint32 {
	var buf [4]byte
	for i := range 4 {
		buf[i] = z80bus.Read(baseAddr + uint16(i))
	}
	return binary.LittleEndian.Uint32(buf[:])
}

// =============================================================================
// Group F: Negative-path, compatibility & regression tests
// =============================================================================

// TestCoprocByteLevel_InvalidCmd verifies that writing an invalid command value
// via byte-0 sets error status.
func TestCoprocByteLevel_InvalidCmd(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	bus.Write8(COPROC_CMD, 99) // invalid command

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_ERROR {
		t.Fatalf("expected ERROR status for invalid cmd, got %d", status)
	}
}

// TestCoprocByteLevel_NoWorkerEnqueue verifies that byte-level ENQUEUE with
// no worker returns ticket=0 and error code.
func TestCoprocByteLevel_NoWorkerEnqueue(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	// Set CPU_TYPE = IE32 via byte write
	bus.Write8(COPROC_CPU_TYPE, byte(EXEC_TYPE_IE32))

	// Trigger ENQUEUE via byte write
	bus.Write8(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ticket := bus.Read32(COPROC_TICKET)
	if ticket != 0 {
		t.Fatalf("expected ticket=0 with no worker, got %d", ticket)
	}

	errCode := bus.Read32(COPROC_CMD_ERROR)
	if errCode != COPROC_ERR_NO_WORKER {
		t.Fatalf("expected NO_WORKER error, got %d", errCode)
	}
}

// TestCoprocGateway_NoExistingConflict verifies that no existing assembler
// source files reference the gateway address range 0xF200-0xF23F.
func TestCoprocGateway_NoExistingConflict(t *testing.T) {
	// Check for references to the gateway range in assembler source files
	patterns := []string{
		"assembler/*.asm",
		"assembler/*.inc",
		"assembler/*.ie65",
		"assembler/*.ie80",
		"assembler/*.ie68",
		"assembler/*.bas",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, file := range matches {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			content := string(data)

			// Check for hex references to the gateway range
			// $F200-$F23F (6502/Z80 style) or 0xF200-0xF23F
			for addr := uint16(0xF200); addr <= 0xF23F; addr++ {
				hexUpper := func(a uint16) string {
					return "$" + string([]byte{
						"0123456789ABCDEF"[(a>>12)&0xF],
						"0123456789ABCDEF"[(a>>8)&0xF],
						"0123456789ABCDEF"[(a>>4)&0xF],
						"0123456789ABCDEF"[a&0xF],
					})
				}
				hexLower := func(a uint16) string {
					return "0x" + string([]byte{
						"0123456789abcdef"[(a>>12)&0xF],
						"0123456789abcdef"[(a>>8)&0xF],
						"0123456789abcdef"[(a>>4)&0xF],
						"0123456789abcdef"[a&0xF],
					})
				}
				hexUpperPfx := func(a uint16) string {
					return "0x" + string([]byte{
						"0123456789ABCDEF"[(a>>12)&0xF],
						"0123456789ABCDEF"[(a>>8)&0xF],
						"0123456789ABCDEF"[(a>>4)&0xF],
						"0123456789ABCDEF"[a&0xF],
					})
				}

				for _, ref := range []string{hexUpper(addr), hexLower(addr), hexUpperPfx(addr)} {
					if containsRef(content, ref) {
						// Skip coproc-related files that legitimately use gateway addresses
						base := filepath.Base(file)
						if base == "ie65.inc" || base == "ie80.inc" ||
							base == "coproc_caller_65.asm" || base == "coproc_caller_z80.asm" {
							continue
						}
						t.Errorf("file %s references gateway address %s", file, ref)
					}
				}
			}
		}
	}
}

// containsRef checks if content contains the reference string, avoiding
// false positives from longer addresses (e.g. 0xF2000 should not match 0xF200).
func containsRef(content, ref string) bool {
	idx := 0
	for {
		pos := indexOf(content[idx:], ref)
		if pos < 0 {
			return false
		}
		pos += idx
		end := pos + len(ref)
		// Check that the match is not part of a longer hex number
		if end < len(content) {
			next := content[end]
			if (next >= '0' && next <= '9') || (next >= 'a' && next <= 'f') || (next >= 'A' && next <= 'F') {
				idx = end
				continue
			}
		}
		return true
	}
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
