//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"
	"os"
	"testing"
	"time"
)

func floatBitsForTest(f float32) uint32 { return math.Float32bits(f) }

// x86ServiceTestCPU builds a CPU with the T&L service loaded and the mailbox
// ring seeded with one OP_ADD and one OP_TNL_BATCH request, ready to execute.
func x86ServiceTestCPU(t *testing.T, data []byte) *CPU_X86 {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = x86JitAvailable
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	mem := bus.GetMemory()
	copy(mem[WORKER_X86_BASE:], data)
	cpu.EIP = WORKER_X86_BASE
	cpu.ESP = WORKER_X86_END - 0xFF

	const ringBase = 0x790C00
	const entries = ringBase + 0x08
	const reqBuf = 0x030000
	const respBuf = 0x030100
	const tnlReq = 0x031000
	const vtxBuf = 0x032000
	const outBuf = 0x033000
	put32 := func(addr, v uint32) {
		mem[addr] = byte(v)
		mem[addr+1] = byte(v >> 8)
		mem[addr+2] = byte(v >> 16)
		mem[addr+3] = byte(v >> 24)
	}
	put32be := func(addr, v uint32) {
		mem[addr] = byte(v >> 24)
		mem[addr+1] = byte(v >> 16)
		mem[addr+2] = byte(v >> 8)
		mem[addr+3] = byte(v)
	}
	putf32be := func(addr uint32, f float32) { put32be(addr, floatBitsForTest(f)) }
	put16be := func(addr uint32, v uint16) { mem[addr] = byte(v >> 8); mem[addr+1] = byte(v) }

	put32(reqBuf, 40)
	put32(reqBuf+4, 2)
	put32(entries+0, 1)
	put32(entries+8, 1) // OP_ADD
	put32(entries+16, reqBuf)
	put32(entries+24, respBuf)

	m := []float32{2, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0.5, 0, 8, 16, 32, 1}
	for i, f := range m {
		putf32be(tnlReq+uint32(i)*4, f)
	}
	put32be(tnlReq+64, 0x8000)
	put32be(tnlReq+68, 0x8000)
	put32be(tnlReq+72, 2)
	put32be(tnlReq+76, vtxBuf)
	put32be(tnlReq+80, outBuf)
	put32be(tnlReq+84, 0)
	put32be(tnlReq+88, 0)
	put16be(vtxBuf+0, 100)
	put16be(vtxBuf+2, 0xFFCE)
	put16be(vtxBuf+4, 8)
	put16be(vtxBuf+8, 512)
	put16be(vtxBuf+10, 0xFC00)
	mem[vtxBuf+12], mem[vtxBuf+13], mem[vtxBuf+14], mem[vtxBuf+15] = 10, 20, 30, 40
	put32(entries+32+0, 2)
	put32(entries+32+8, 3) // OP_TNL_BATCH
	put32(entries+32+16, tnlReq)
	put32(entries+32+24, tnlReq)
	mem[ringBase+2] = 16
	mem[ringBase] = 2
	mem[ringBase+1] = 0
	return cpu
}

// TestX86JIT_ServiceParityWindows walks the seeded service on interpreter and
// JIT in small instruction windows, comparing canonical state hashes. The
// window after the last matching checkpoint brackets any divergence or fault.
func TestX86JIT_ServiceParityWindows(t *testing.T) {
	data, err := os.ReadFile("testdata/tnl_service_x86.ie86")
	if err != nil {
		t.Skipf("service binary not available: %v", err)
	}
	interp := x86ServiceTestCPU(t, data)
	interp.x86JitEnabled = false
	jit := x86ServiceTestCPU(t, data)

	const window = 50
	for cp := 1; cp <= 2000; cp++ {
		x86ShadowStepBudget(t, interp, false, window, 20*time.Second)
		x86ShadowStepBudget(t, jit, true, window, 20*time.Second)

		interpBytes, interpSum := x86CanonicalStateHash(interp)
		jitBytes, jitSum := x86CanonicalStateHash(jit)
		if interpSum != jitSum {
			for i := range interpBytes {
				if interpBytes[i] != jitBytes[i] {
					t.Errorf("first diff at canonical byte %d: interp=%02X jit=%02X", i, interpBytes[i], jitBytes[i])
					break
				}
			}
			t.Fatalf("checkpoint %d diverged: interp EIP=%08X jit EIP=%08X interpFSW=%04X jitFSW=%04X interpFTW=%04X jitFTW=%04X",
				cp, interp.EIP, jit.EIP, interp.FPU.FSW, jit.FPU.FSW, interp.FPU.FTW, jit.FPU.FTW)
		}
	}
}

// TestX86JIT_ServiceUnbounded runs the seeded service worker-style: an
// unbounded x86JitExecute on its own goroutine, full block chaining
// (bounded runs set ChainBudget=1, so chaining is otherwise untested).
// The main thread watches the ring tail like the guest's drain loop.
func TestX86JIT_ServiceUnbounded(t *testing.T) {
	data, err := os.ReadFile("testdata/tnl_service_x86.ie86")
	if err != nil {
		t.Skipf("service binary not available: %v", err)
	}
	cpu := x86ServiceTestCPU(t, data)
	mem := cpu.memory
	NewDebugX86(cpu, nil) // workers always carry a debug adapter; match that

	// Live shape: the worker spins on an EMPTY ring first (the A0 head-poll
	// bail path gets hot), and requests arrive while it is mid-spin.
	seededHead := mem[0x790C00]
	mem[0x790C00] = 0 // hide the seeded requests; ring appears empty

	cpu.SetRunning(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		cpu.x86JitExecute()
	}()
	defer func() {
		cpu.SetRunning(false)
		<-done
	}()

	time.Sleep(300 * time.Millisecond) // let the poll loop compile and spin
	mem[0x790C00] = seededHead         // "enqueue": publish head mid-spin

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if mem[0x790C01] == 2 {
			gotX := uint32(mem[0x033000+16])<<24 | uint32(mem[0x033000+17])<<16 |
				uint32(mem[0x033000+18])<<8 | uint32(mem[0x033000+19])
			if gotX != floatBitsForTest(208.0) {
				t.Fatalf("TNL clip X bits = %#x, want %#x (208.0)", gotX, floatBitsForTest(208.0))
			}
			return // tail advanced past both requests, output correct
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ring tail = %d after 10s, want 2 (worker never completed the requests)", mem[0x790C01])
}

// TestX86JIT_CompiledServiceBinary runs a gcc-compiled freestanding service
// binary (the mk64-ie T&L coprocessor service) under the JIT with a bounded
// instruction budget. Hand-written test snippets and the asm example services
// exercise emitters in isolation; a compiled C binary composes them densely
// and has caught register-scheduling faults the unit tests cannot.
//
// The binary is the checked-in testdata fixture: a gcc i486 freestanding
// mailbox T&L service. It guards the full worker-style JIT execution path.
func TestX86JIT_CompiledServiceBinary(t *testing.T) {
	data, err := os.ReadFile("testdata/tnl_service_x86.ie86")
	if err != nil {
		t.Skipf("service binary not available: %v", err)
	}

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = x86JitAvailable
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	mem := bus.GetMemory()
	copy(mem[WORKER_X86_BASE:], data)
	cpu.EIP = WORKER_X86_BASE
	cpu.ESP = WORKER_X86_END - 0xFF

	// One OP_ADD request in the mailbox ring so the service leaves its
	// poll loop and runs real code: descriptor at ring slot 0, head=1.
	const ringBase = 0x790C00
	const entries = ringBase + 0x08
	const reqBuf = 0x030000
	const respBuf = 0x030100
	put32 := func(addr, v uint32) {
		mem[addr] = byte(v)
		mem[addr+1] = byte(v >> 8)
		mem[addr+2] = byte(v >> 16)
		mem[addr+3] = byte(v >> 24)
	}
	put32(reqBuf, 40)
	put32(reqBuf+4, 2)
	put32(entries+0, 1) // ticket
	put32(entries+8, 1) // OP_ADD
	put32(entries+16, reqBuf)
	put32(entries+24, respBuf)
	mem[ringBase+2] = 16 // capacity
	mem[ringBase] = 1    // head=1: one pending request
	mem[ringBase+1] = 0  // tail

	// Second request: OP_TNL_BATCH over two vertices - the x87-dense
	// kernel path. All request fields are big-endian (M68K caller).
	const tnlReq = 0x031000
	const vtxBuf = 0x032000
	const outBuf = 0x033000
	put32be := func(addr, v uint32) {
		mem[addr] = byte(v >> 24)
		mem[addr+1] = byte(v >> 16)
		mem[addr+2] = byte(v >> 8)
		mem[addr+3] = byte(v)
	}
	putf32be := func(addr uint32, f float32) { put32be(addr, floatBitsForTest(f)) }
	// Identity-ish matrix with scale/translate (row*4+col), floats BE.
	m := []float32{2, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0.5, 0, 8, 16, 32, 1}
	for i, f := range m {
		putf32be(tnlReq+uint32(i)*4, f)
	}
	put32be(tnlReq+64, 0x8000) // texscale s
	put32be(tnlReq+68, 0x8000) // texscale t
	put32be(tnlReq+72, 2)      // nverts
	put32be(tnlReq+76, vtxBuf) // verts ptr
	put32be(tnlReq+80, outBuf) // out ptr
	put32be(tnlReq+84, 0)      // geomode (unlit)
	put32be(tnlReq+88, 0)      // num lights
	// Vtx[0]: ob=(100,-50,8) tc=(512,-1024) cn=(10,20,30,40)
	put16be := func(addr uint32, v uint16) { mem[addr] = byte(v >> 8); mem[addr+1] = byte(v) }
	put16be(vtxBuf+0, 100)
	put16be(vtxBuf+2, 0xFFCE) // -50
	put16be(vtxBuf+4, 8)
	put16be(vtxBuf+8, 512)
	put16be(vtxBuf+10, 0xFC00) // -1024
	mem[vtxBuf+12], mem[vtxBuf+13], mem[vtxBuf+14], mem[vtxBuf+15] = 10, 20, 30, 40
	// Vtx[1]: zeros are fine.
	put32(entries+32+0, 2) // ticket
	put32(entries+32+8, 3) // OP_TNL_BATCH
	put32(entries+32+16, tnlReq)
	put32(entries+32+24, tnlReq)
	mem[ringBase] = 2 // head=2: both requests pending

	// Bounded run: enough budget to process both requests many times over.
	cpu.x86BudgetActive = true
	cpu.x86InstrBudget = 5_000_000
	cpu.SetRunning(true)
	cpu.x86JitExecute() // must not fault; returns on budget exhaustion

	if got := uint32(mem[respBuf]) | uint32(mem[respBuf+1])<<8 |
		uint32(mem[respBuf+2])<<16 | uint32(mem[respBuf+3])<<24; got != 42 {
		t.Fatalf("OP_ADD response = %d, want 42", got)
	}
	if mem[ringBase+1] != 2 {
		t.Fatalf("ring tail = %d, want 2 (both requests consumed)", mem[ringBase+1])
	}
	// Clip-space X of vertex 0: 100*2 + 8 = 208.0 (BE float at out+16).
	gotX := uint32(mem[outBuf+16])<<24 | uint32(mem[outBuf+17])<<16 |
		uint32(mem[outBuf+18])<<8 | uint32(mem[outBuf+19])
	if gotX != floatBitsForTest(208.0) {
		t.Fatalf("TNL clip X bits = %#x, want %#x (208.0)", gotX, floatBitsForTest(208.0))
	}
}
