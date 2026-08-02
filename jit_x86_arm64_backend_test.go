//go:build arm64 && linux

package main

import (
	"bytes"
	"math"
	"testing"
	"unsafe"
)

func newX86ARM64DispatchCPU(code []byte) *CPU_X86 {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0x100
	copy(cpu.memory[cpu.EIP:], code)
	cpu.running.Store(true)
	return cpu
}

func TestX86ARM64_ProductionDispatcherParity(t *testing.T) {
	if !x86JitAvailable {
		t.Fatal("Linux ARM64 x86 JIT dispatcher is not available")
	}
	// The ADD remains interpreter-owned. This checks a direct prefix, a
	// fallback transition, and the final HLT boundary through the public loop.
	code := []byte{
		0xB8, 0x44, 0x33, 0x22, 0x11, // MOV EAX,11223344
		0xBB, 0x80, 0, 0, 0, // MOV EBX,80
		0x0F, 0xB6, 0xD3, // MOVZX EDX,BL
		0x05, 1, 0, 0, 0, // ADD EAX,1
		0xF4,
	}
	jit := newX86ARM64DispatchCPU(code)
	jit.X86ExecuteJIT()
	interp := newX86ARM64DispatchCPU(code)
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if !jit.Halted {
		t.Fatal("JIT program did not halt")
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.EDX, interp.EDX; got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
	if got, want := jit.EIP, interp.EIP; got != want {
		t.Fatalf("EIP = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("Cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_ProductionDispatcherHonoursInstructionBudget(t *testing.T) {
	// Both MOVs are directly emitted. The bounded loop must stop after the
	// first retirement instead of executing the compiled prefix to HLT.
	cpu := newX86ARM64DispatchCPU([]byte{
		0xB8, 1, 0, 0, 0,
		0xBB, 2, 0, 0, 0,
		0xF4,
	})
	cpu.x86BudgetActive = true
	cpu.x86InstrBudget = 1
	cpu.X86ExecuteJIT()
	if got := cpu.x86InstrBudget; got != 0 {
		t.Fatalf("budget = %d, want 0", got)
	}
	if got := cpu.EAX; got != 1 {
		t.Fatalf("EAX = %08X, want 00000001", got)
	}
	if got := cpu.EBX; got != 0 {
		t.Fatalf("EBX = %08X, want 00000000 after one retirement", got)
	}
	if cpu.Halted {
		t.Fatal("bounded execution ran through HLT")
	}
}

func TestX86ARM64_ProductionDispatcherServicesPendingNMI(t *testing.T) {
	cpu := newX86ARM64DispatchCPU([]byte{0xB4, 0x12, 0xF4}) // MOV AH,12; HLT
	cpu.memory[0x08], cpu.memory[0x09] = 0x00, 0x20
	cpu.memory[0x0A], cpu.memory[0x0B] = 0x00, 0x00
	cpu.memory[0x2000] = 0xF4
	cpu.SetNMI(true)
	cpu.X86ExecuteJIT()
	if got := cpu.AH(); got != 0 {
		t.Fatalf("AH = %02X, want 00 because NMI must preempt native dispatch", got)
	}
	if cpu.nmiPending.Load() {
		t.Fatal("NMI remains pending after dispatcher observation")
	}
	if !cpu.Halted || cpu.EIP != 0x2001 {
		t.Fatalf("handler did not halt at NMI target: halted=%v EIP=%08X", cpu.Halted, cpu.EIP)
	}
}

func TestX86ARM64_ProductionDispatcherHonoursBreakpoint(t *testing.T) {
	cpu := newX86ARM64DispatchCPU([]byte{0xB8, 1, 0, 0, 0, 0xF4})
	seen := uint64(0)
	cpu.debugBreakIn = func(pc uint64) bool {
		seen = pc
		return true
	}
	cpu.X86ExecuteJIT()
	if seen != 0x100 {
		t.Fatalf("breakpoint PC = %08X, want 00000100", seen)
	}
	if got := cpu.EAX; got != 0 {
		t.Fatalf("EAX = %08X, want unchanged state", got)
	}
	if cpu.Running() {
		t.Fatal("breakpoint did not stop ARM64 JIT execution")
	}
}

func TestX86ARM64_ProductionDispatcherPreservesConfiguredMMIOBitmap(t *testing.T) {
	const deviceAddr = uint32(0x400)
	const deviceValue = uint32(0xA1B2C3D4)
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(deviceAddr, deviceAddr+3, func(addr uint32) uint32 {
		reads++
		return uint32(byte(deviceValue >> ((addr - deviceAddr) * 8)))
	}, nil)
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP = 0x100
	copy(cpu.memory[cpu.EIP:], []byte{0xA1, 0x00, 0x04, 0x00, 0x00, 0xF4}) // MOV EAX,[0400]; HLT
	// A distinct backing value proves that the native load did not bypass the
	// configured mapping before the interpreter replayed the instruction.
	cpu.memory[deviceAddr] = 0x5A
	cpu.running.Store(true)
	cpu.X86ExecuteJIT()
	if got, want := cpu.EAX, deviceValue; got != want {
		t.Fatalf("EAX = %08X, want MMIO value %08X", got, want)
	}
	if reads == 0 {
		t.Fatal("configured MMIO read was not routed through the bus")
	}
}

func TestX86ARM64_ProductionDispatcherGuardsVisibleRAMCeiling(t *testing.T) {
	const visible = uint32(0x1000)
	const hidden = uint32(0x2000)
	bus := NewMachineBus()
	bus.ApplyProfileVisibleCeiling(uint64(visible))
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP, cpu.EAX = 0x100, 0x11223344
	copy(cpu.memory[cpu.EIP:], []byte{0xA3, 0x00, 0x20, 0x00, 0x00, 0xF4}) // MOV [2000],EAX; HLT
	cpu.x86JitPersist = true                                               // retain the context for the post-dispatch assertion
	t.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})
	cpu.running.Store(true)
	cpu.X86ExecuteJIT()
	if got, want := cpu.x86JitCtx.MemSize, visible; got != want {
		t.Fatalf("native direct-access ceiling = %08X, want %08X", got, want)
	}
	// Step owns the above-ceiling access after the guard returns. Its exact
	// policy is bus-defined, but the native emitter must never use the backing
	// directly: the retained production context is the bound it consumes.
	if got, want := cpu.memory[hidden], byte(0x44); got != want {
		t.Fatalf("interpreter replay did not complete the store: %02X, want %02X", got, want)
	}
}

func TestX86ARM64_DirectLESLDSParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		addr uint32
		data []byte
		init func(*CPU_X86)
	}{
		{"les32-memory", []byte{0xC4, 0x03, 0xF4}, 0x500, []byte{0x78, 0x56, 0x34, 0x12, 0xCD, 0xAB}, func(cpu *CPU_X86) { cpu.EBX = 0x500 }},
		{"lds16-memory", []byte{0x66, 0xC5, 0x13, 0xF4}, 0x520, []byte{0x34, 0x12, 0xDC, 0xBA}, func(cpu *CPU_X86) { cpu.EDX, cpu.EAX = 0x520, 0xDEAD0000 }},
		{"les-register-address", []byte{0xC4, 0xCB, 0xF4}, 0x540, []byte{0x11, 0x22, 0x33, 0x44, 0x66, 0x55}, func(cpu *CPU_X86) { cpu.EBX = 0x540 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				tc.init(cpu)
				copy(cpu.memory[tc.addr:], tc.data)
				return cpu
			}
			jit := newCPU()
			jit.x86BudgetActive = true
			jit.x86InstrBudget = 1
			jit.X86ExecuteJIT()
			interp := newCPU()
			interp.Step()
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("EAX = %08X, want %08X", got, want)
			}
			if got, want := jit.EDX, interp.EDX; got != want {
				t.Fatalf("EDX = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.ES, interp.ES; got != want {
				t.Fatalf("ES = %04X, want %04X", got, want)
			}
			if got, want := jit.DS, interp.DS; got != want {
				t.Fatalf("DS = %04X, want %04X", got, want)
			}
			if got, want := jit.EIP, interp.EIP; got != want {
				t.Fatalf("EIP = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectMemoryDoubleShiftImmediateParity(t *testing.T) {
	for _, code := range [][]byte{
		{0x0F, 0xA4, 0x03, 4, 0xF4}, // SHLD dword [EBX],EAX,4
		{0x0F, 0xAC, 0x03, 4, 0xF4}, // SHRD dword [EBX],EAX,4
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX = 0x01234567, 0x500
			cpu.Flags = x86FlagOF | x86FlagAF
			cpu.memory[0x500] = 0xEF
			cpu.memory[0x501] = 0xCD
			cpu.memory[0x502] = 0xAB
			cpu.memory[0x503] = 0x89
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.memory[0x500:0x504], interp.memory[0x500:0x504]; !bytes.Equal(got, want) {
			t.Fatalf("memory = % X, want % X for % X", got, want, code)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("EFLAGS = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, code)
		}
	}
}

func TestX86ARM64_DirectDoubleShiftCLParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		count uint32
	}{
		{[]byte{0x0F, 0xA5, 0xC8, 0xF4}, 0}, // SHLD EAX,ECX,CL
		{[]byte{0x0F, 0xA5, 0xC8, 0xF4}, 1},
		{[]byte{0x0F, 0xA5, 0xC8, 0xF4}, 4},
		{[]byte{0x0F, 0xAD, 0xC8, 0xF4}, 1}, // SHRD EAX,ECX,CL
		{[]byte{0x0F, 0xAD, 0xC8, 0xF4}, 4},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.EAX, cpu.ECX = 0x89ABCDEF, 0x01234500|tc.count
			cpu.Flags = x86FlagOF | x86FlagAF
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X CL=%d", got, want, tc.code, tc.count)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("EFLAGS = %08X, want %08X for % X CL=%d", got, want, tc.code, tc.count)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X CL=%d", got, want, tc.code, tc.count)
		}
	}
}

func TestX86ARM64_DirectDoubleShift16ImmediateParity(t *testing.T) {
	for _, tc := range []struct {
		code []byte
		mem  bool
	}{
		{[]byte{0x66, 0x0F, 0xA4, 0xC8, 1, 0xF4}, false},  // SHLD AX,CX,1
		{[]byte{0x66, 0x0F, 0xAC, 0xC8, 4, 0xF4}, false},  // SHRD AX,CX,4
		{[]byte{0x66, 0x0F, 0xA4, 0xC8, 16, 0xF4}, false}, // SHLD AX,CX,16
		{[]byte{0x66, 0x0F, 0xAC, 0x03, 4, 0xF4}, true},   // SHRD word [EBX],AX,4
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.EAX, cpu.ECX = 0xDEAD89AB, 0xBEEF4567
			cpu.EBX = 0x500
			cpu.Flags = x86FlagOF | x86FlagAF
			if tc.mem {
				cpu.memory[0x500], cpu.memory[0x501] = 0xAB, 0x89
			}
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("EFLAGS = %08X, want %08X for % X", got, want, tc.code)
		}
		if tc.mem && !bytes.Equal(jit.memory[0x500:0x502], interp.memory[0x500:0x502]) {
			t.Fatalf("memory = % X, want % X for % X", jit.memory[0x500:0x502], interp.memory[0x500:0x502], tc.code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectGroup2DwordShiftCLParity(t *testing.T) {
	for _, op := range []byte{0xE0, 0xE8, 0xF8} { // SHL, SHR, SAR EAX,CL
		for _, count := range []uint32{0, 1, 4, 31, 33} {
			code := []byte{0xD3, op, 0xF4}
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.ECX = 0x89ABCDEF, 0x12340000|count
				cpu.Flags = x86FlagOF | x86FlagAF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("EAX = %08X, want %08X for D3 %02X CL=%d", got, want, op, count)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("EFLAGS = %08X, want %08X for D3 %02X CL=%d", got, want, op, count)
			}
		}
	}
}

func TestX86ARM64_DirectGroup2ByteShiftCLParity(t *testing.T) {
	for _, op := range []byte{0xE0, 0xE8, 0xF8, 0xFC} { // SHL, SHR, SAR AH,CL
		for _, count := range []uint32{0, 1, 4, 7, 8, 15, 31, 33} {
			code := []byte{0xD2, op, 0xF4}
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.ECX = 0x1234A55A, 0xCAFE0000|count
				cpu.Flags = x86FlagOF | x86FlagAF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("EAX = %08X, want %08X for D2 %02X CL=%d", got, want, op, count)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("EFLAGS = %08X, want %08X for D2 %02X CL=%d", got, want, op, count)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d for D2 %02X CL=%d", got, want, op, count)
			}
		}
	}
}

func TestX86ARM64_DirectIMUL16Parity(t *testing.T) {
	for _, code := range [][]byte{
		{0x66, 0x6B, 0xC8, 0x02, 0xF4},       // IMUL CX,AX,2
		{0x66, 0x69, 0xC8, 0x00, 0x80, 0xF4}, // IMUL CX,AX,-32768
		{0x66, 0x0F, 0xAF, 0xC8, 0xF4},       // IMUL CX,AX
		{0x66, 0x6B, 0x0B, 0x02, 0xF4},       // IMUL CX,word [EBX],2
		{0x66, 0x0F, 0xAF, 0x0B, 0xF4},       // IMUL CX,word [EBX]
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.ECX = 0xDEAD8001, 0x500, 0x12340003
			cpu.memory[0x500], cpu.memory[0x501] = 0x01, 0x80
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.ECX, interp.ECX; got != want {
			t.Fatalf("ECX = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("EFLAGS = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, code)
		}
	}
}

func TestX86ARM64_DirectFCHSFABSParity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  []byte
		value float64
	}{
		{"fchs", []byte{0xD9, 0xE0, 0xF4}, 3.5},
		{"fabs", []byte{0xD9, 0xE1, 0xF4}, -3.5},
		{"fabs-negative-zero", []byte{0xD9, 0xE1, 0xF4}, math.Copysign(0, -1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.FPU.setST(0, tc.value)
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
				t.Fatalf("ST(0) = %016X, want %016X", got, want)
			}
			if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
				t.Fatalf("FSW = %04X, want %04X", got, want)
			}
			if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
				t.Fatalf("FTW = %04X, want %04X", got, want)
			}
			if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
				t.Fatalf("FIP = %08X, want %08X", got, want)
			}
			if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
				t.Fatalf("FOP = %04X, want %04X", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectFCHSFABSEmptyStackHelperParity(t *testing.T) {
	for _, code := range [][]byte{{0xD9, 0xE0, 0xF4}, {0xD9, 0xE1, 0xF4}} {
		jit := newX86ARM64DispatchCPU(code)
		jit.X86ExecuteJIT()
		interp := newX86ARM64DispatchCPU(code)
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
			t.Fatalf("FSW = %04X, want %04X for % X", got, want, code)
		}
		if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
			t.Fatalf("FTW = %04X, want %04X for % X", got, want, code)
		}
		if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
			t.Fatalf("FIP = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
			t.Fatalf("FOP = %04X, want %04X for % X", got, want, code)
		}
	}
}

func TestX86ARM64_DirectFADDRegisterParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xD8, 0xC1, 0xF4}) // FADD ST(0),ST(1)
		cpu.FPU.setST(0, 1.25)
		cpu.FPU.setST(1, 2.5)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFSUBRegisterParity(t *testing.T) {
	for _, tc := range []struct {
		code []byte
		want float64
	}{
		{[]byte{0xD8, 0xE1, 0xF4}, -1.25}, // FSUB ST(0),ST(1)
		{[]byte{0xD8, 0xE9, 0xF4}, 1.25},  // FSUBR ST(0),ST(1)
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.FPU.setST(0, 1.25)
			cpu.FPU.setST(1, 2.5)
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(tc.want); got != want {
			t.Fatalf("ST(0) = %016X, want %016X for % X", got, want, tc.code)
		}
		if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
			t.Fatalf("ST(0) = %016X, interpreter %016X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
			t.Fatalf("FSW = %04X, want %04X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
			t.Fatalf("FTW = %04X, want %04X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
			t.Fatalf("FIP = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
			t.Fatalf("FOP = %04X, want %04X for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectFMULRegisterParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xD8, 0xC9, 0xF4}) // FMUL ST(0),ST(1)
		cpu.FPU.setST(0, 1.25)
		cpu.FPU.setST(1, 2.5)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFDIVRegisterParity(t *testing.T) {
	for _, tc := range []struct {
		code []byte
		want float64
	}{
		{[]byte{0xD8, 0xF1, 0xF4}, 0.5}, // FDIV ST(0),ST(1)
		{[]byte{0xD8, 0xF9, 0xF4}, 2.0}, // FDIVR ST(0),ST(1)
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.FPU.setST(0, 1.25)
			cpu.FPU.setST(1, 2.5)
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(tc.want); got != want {
			t.Fatalf("ST(0) = %016X, want %016X for % X", got, want, tc.code)
		}
		if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
			t.Fatalf("ST(0) = %016X, interpreter %016X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
			t.Fatalf("FSW = %04X, want %04X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
			t.Fatalf("FTW = %04X, want %04X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
			t.Fatalf("FIP = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
			t.Fatalf("FOP = %04X, want %04X for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectREPMOVSParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		step uint32
		back bool
	}{
		{"movsb-forward", []byte{0xF3, 0xA4, 0xF4}, 1, false},
		{"movsw-forward", []byte{0x66, 0xF3, 0xA5, 0xF4}, 2, false},
		{"movsd-forward", []byte{0xF3, 0xA5, 0xF4}, 4, false},
		{"movsb-reverse", []byte{0xF3, 0xA4, 0xF4}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.ECX = 4
				cpu.ESI, cpu.EDI = 0x500, 0x600
				if tc.back {
					cpu.ESI += 3 * tc.step
					cpu.EDI += 3 * tc.step
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < 4*tc.step; i++ {
					cpu.memory[0x500+i] = byte(0x80 + i)
				}
				return cpu
			}
			// Direct admission is part of the contract: a parity-only dispatcher
			// check would also pass if the form merely fell back to Step.
			admit := newCPU()
			em, err := AllocExecMem(4096)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(em.Free)
			instrs := x86ScanBlock(admit.memory, admit.EIP)
			if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
				t.Fatalf("REP MOVS was not admitted natively: %v", err)
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.memory[0x600:0x610], interp.memory[0x600:0x610]; !bytes.Equal(got, want) {
				t.Fatalf("destination = % X, want % X", got, want)
			}
			if got, want := jit.ESI, interp.ESI; got != want {
				t.Fatalf("ESI = %08X, want %08X", got, want)
			}
			if got, want := jit.EDI, interp.EDI; got != want {
				t.Fatalf("EDI = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectMOVSBParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		width uint32
	}{
		{[]byte{0xA4, 0xF4}, 1},
		{[]byte{0x66, 0xA5, 0xF4}, 2},
		{[]byte{0xA5, 0xF4}, 4},
	} {
		for _, df := range []bool{false, true} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.ESI, cpu.EDI = 0x500, 0x600
				if df {
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < tc.width; i++ {
					cpu.memory[0x500+i] = byte(0xA5 + i)
				}
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.ESI != interp.ESI || jit.EDI != interp.EDI || !bytes.Equal(jit.memory[0x600:0x600+tc.width], interp.memory[0x600:0x600+tc.width]) || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X DF=%v ESI/EDI=%08X/%08X %08X/%08X flags=%08X/%08X cycles=%d/%d", tc.code, df, jit.ESI, interp.ESI, jit.EDI, interp.EDI, jit.Flags, interp.Flags, jit.Cycles, interp.Cycles)
			}
		}
	}
}

func TestX86ARM64_DirectREPSTOSParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		step uint32
		back bool
	}{
		{"stosb-forward", []byte{0xF3, 0xAA, 0xF4}, 1, false},
		{"stosw-forward", []byte{0x66, 0xF3, 0xAB, 0xF4}, 2, false},
		{"stosd-forward", []byte{0xF3, 0xAB, 0xF4}, 4, false},
		{"stosb-reverse", []byte{0xF3, 0xAA, 0xF4}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.ECX, cpu.EDI = 0xA1B2C3D4, 4, 0x600
				if tc.back {
					cpu.EDI += 3 * tc.step
					cpu.Flags |= x86FlagDF
				}
				return cpu
			}
			admit := newCPU()
			em, err := AllocExecMem(4096)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(em.Free)
			instrs := x86ScanBlock(admit.memory, admit.EIP)
			if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
				t.Fatalf("REP STOS was not admitted natively: %v", err)
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.memory[0x600:0x610], interp.memory[0x600:0x610]; !bytes.Equal(got, want) {
				t.Fatalf("destination = % X, want % X", got, want)
			}
			if got, want := jit.EDI, interp.EDI; got != want {
				t.Fatalf("EDI = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectSTOSParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		width uint32
	}{{[]byte{0xAA, 0xF4}, 1}, {[]byte{0x66, 0xAB, 0xF4}, 2}, {[]byte{0xAB, 0xF4}, 4}} {
		for _, df := range []bool{false, true} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.EDI = 0xA1B2C3D4, 0x600
				if df {
					cpu.Flags |= x86FlagDF
				}
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EDI != interp.EDI || !bytes.Equal(jit.memory[0x600:0x600+tc.width], interp.memory[0x600:0x600+tc.width]) || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X DF=%v EDI=%08X/%08X", tc.code, df, jit.EDI, interp.EDI)
			}
		}
	}
}

func TestX86ARM64_DirectREPLODSParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		step uint32
		back bool
	}{
		{"lodsb-forward", []byte{0xF3, 0xAC, 0xF4}, 1, false},
		{"lodsw-forward", []byte{0x66, 0xF3, 0xAD, 0xF4}, 2, false},
		{"lodsd-forward", []byte{0xF3, 0xAD, 0xF4}, 4, false},
		{"lodsb-reverse", []byte{0xF3, 0xAC, 0xF4}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.ECX, cpu.ESI = 0xDEAD0000, 4, 0x500
				if tc.back {
					cpu.ESI += 3 * tc.step
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < 4*tc.step; i++ {
					cpu.memory[0x500+i] = byte(0x80 + i)
				}
				return cpu
			}
			admit := newCPU()
			em, err := AllocExecMem(4096)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(em.Free)
			instrs := x86ScanBlock(admit.memory, admit.EIP)
			if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
				t.Fatalf("REP LODS was not admitted natively: %v", err)
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("EAX = %08X, want %08X", got, want)
			}
			if got, want := jit.ESI, interp.ESI; got != want {
				t.Fatalf("ESI = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_SeparateREPBlockCycleParity(t *testing.T) {
	// The first REP advances ESI/EDI while the second advances EDI only. They
	// must dispatch separately so dynamic cycle accounting observes each form's
	// own pointer delta.
	code := []byte{0xF3, 0xA4, 0xF3, 0xAA, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX, cpu.ECX, cpu.ESI, cpu.EDI = 0x0000007E, 3, 0x500, 0x600
		cpu.memory[0x500], cpu.memory[0x501], cpu.memory[0x502] = 1, 2, 3
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.memory[0x600:0x606], interp.memory[0x600:0x606]; !bytes.Equal(got, want) {
		t.Fatalf("destination = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectLODSParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		width uint32
	}{{[]byte{0xAC, 0xF4}, 1}, {[]byte{0x66, 0xAD, 0xF4}, 2}, {[]byte{0xAD, 0xF4}, 4}} {
		for _, df := range []bool{false, true} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.ESI = 0xDEADBEEF, 0x500
				if df {
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < tc.width; i++ {
					cpu.memory[0x500+i] = byte(0x40 + i)
				}
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EAX != interp.EAX || jit.ESI != interp.ESI || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X DF=%v EAX=%08X/%08X ESI=%08X/%08X", tc.code, df, jit.EAX, interp.EAX, jit.ESI, interp.ESI)
			}
		}
	}
}

func TestX86ARM64_DirectREPCMPSParity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     []byte
		src, dst []byte
	}{
		{"repe-stops-on-mismatch", []byte{0xF3, 0xA6, 0xF4}, []byte{1, 2, 3, 4}, []byte{1, 9, 3, 4}},
		{"repne-stops-on-match", []byte{0xF2, 0xA6, 0xF4}, []byte{1, 2, 3, 4}, []byte{9, 2, 8, 7}},
		{"repe-completes", []byte{0xF3, 0xA7, 0xF4}, []byte{1, 0, 2, 0, 3, 0, 4, 0}, []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.ECX, cpu.ESI, cpu.EDI = 4, 0x500, 0x600
				copy(cpu.memory[0x500:], tc.src)
				copy(cpu.memory[0x600:], tc.dst)
				return cpu
			}
			admit := newCPU()
			em, err := AllocExecMem(4096)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(em.Free)
			instrs := x86ScanBlock(admit.memory, admit.EIP)
			if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
				t.Fatalf("REP CMPS was not admitted natively: %v", err)
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.ESI, interp.ESI; got != want {
				t.Fatalf("ESI = %08X, want %08X", got, want)
			}
			if got, want := jit.EDI, interp.EDI; got != want {
				t.Fatalf("EDI = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("EFLAGS = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectCMPSParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		width uint32
	}{{[]byte{0xA6, 0xF4}, 1}, {[]byte{0x66, 0xA7, 0xF4}, 2}, {[]byte{0xA7, 0xF4}, 4}} {
		for _, df := range []bool{false, true} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.ESI, cpu.EDI = 0x500, 0x600
				if df {
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < tc.width; i++ {
					cpu.memory[0x500+i], cpu.memory[0x600+i] = byte(0x40+i), byte(0x20+i)
				}
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.ESI != interp.ESI || jit.EDI != interp.EDI || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X DF=%v ESI/EDI=%08X/%08X %08X/%08X flags=%08X/%08X", tc.code, df, jit.ESI, interp.ESI, jit.EDI, interp.EDI, jit.Flags, interp.Flags)
			}
		}
	}
}

func TestX86ARM64_DirectREPSCASParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		data []byte
	}{
		{"repe-stops-on-mismatch", []byte{0xF3, 0xAE, 0xF4}, []byte{0x44, 0x99, 0x44, 0x44}},
		{"repne-stops-on-match", []byte{0xF2, 0xAE, 0xF4}, []byte{0x11, 0x44, 0x22, 0x33}},
		{"repe-completes", []byte{0xF3, 0xAF, 0xF4}, []byte{0x44, 0, 0, 0, 0x44, 0, 0, 0, 0x44, 0, 0, 0, 0x44, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.ECX, cpu.EDI = 0x44, 4, 0x600
				copy(cpu.memory[0x600:], tc.data)
				return cpu
			}
			admit := newCPU()
			em, err := AllocExecMem(4096)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(em.Free)
			instrs := x86ScanBlock(admit.memory, admit.EIP)
			if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
				t.Fatalf("REP SCAS was not admitted natively: %v", err)
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EDI, interp.EDI; got != want {
				t.Fatalf("EDI = %08X, want %08X", got, want)
			}
			if got, want := jit.ECX, interp.ECX; got != want {
				t.Fatalf("ECX = %08X, want %08X", got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("EFLAGS = %08X, want %08X", got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("cycles = %d, want %d", got, want)
			}
		})
	}
}

func TestX86ARM64_DirectSCASParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		width uint32
	}{{[]byte{0xAE, 0xF4}, 1}, {[]byte{0x66, 0xAF, 0xF4}, 2}, {[]byte{0xAF, 0xF4}, 4}} {
		for _, df := range []bool{false, true} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc.code)
				cpu.EAX, cpu.EDI = 0x10203040, 0x600
				if df {
					cpu.Flags |= x86FlagDF
				}
				for i := uint32(0); i < tc.width; i++ {
					cpu.memory[0x600+i] = byte(0x20 + i)
				}
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EDI != interp.EDI || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X DF=%v EDI=%08X/%08X flags=%08X/%08X", tc.code, df, jit.EDI, interp.EDI, jit.Flags, interp.Flags)
			}
		}
	}
}

func TestX86ARM64_DirectFADDOverflowUsesHelperParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xD8, 0xC1, 0xF4})
		cpu.FPU.setST(0, math.MaxFloat64)
		cpu.FPU.setST(1, math.MaxFloat64)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFADDZeroResultUsesHelperParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xD8, 0xC1, 0xF4})
		cpu.FPU.setST(0, 1.0)
		cpu.FPU.setST(1, -1.0)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectRelativeJMPParity(t *testing.T) {
	// Both short and operand-size near forms must return their branch target,
	// rather than the direct-prefix scanner's fall-through PC.
	for _, code := range [][]byte{
		{0xEB, 0x05, 0xB8, 1, 0, 0, 0, 0xB8, 2, 0, 0, 0, 0xF4},
		{0x66, 0xE9, 0x05, 0, 0xB8, 1, 0, 0, 0, 0xB8, 2, 0, 0, 0, 0xF4},
	} {
		jit := newX86ARM64DispatchCPU(code)
		jit.X86ExecuteJIT()
		interp := newX86ARM64DispatchCPU(code)
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatal("JIT JMP program did not halt")
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, code)
		}
	}
}

func TestX86ARM64_DirectCALLRETParity(t *testing.T) {
	// The CALL targets the callee after HLT. RET must restore the saved
	// fall-through PC so the dispatcher reaches that HLT, with the same stack
	// and cycle state as the interpreter.
	code := []byte{0xE8, 1, 0, 0, 0, 0xF4, 0xB8, 2, 0, 0, 0, 0xC3}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP = 0x800
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if !jit.Halted {
		t.Fatal("JIT CALL/RET program did not halt")
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.EIP, interp.EIP; got != want {
		t.Fatalf("EIP = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectRETImm16Parity(t *testing.T) {
	// RET imm16 consumes the native 32-bit return address then performs its
	// caller-cleanup adjustment. The continuation is HLT at 0x105.
	code := []byte{0xE8, 1, 0, 0, 0, 0xF4, 0xC2, 4, 0}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP = 0x800
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if !jit.Halted {
		t.Fatal("JIT RET imm16 program did not halt")
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.EIP, interp.EIP; got != want {
		t.Fatalf("EIP = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectJccParity(t *testing.T) {
	// JZ selects EAX=2 when ZF is set and falls through to EAX=1 otherwise.
	// The near JNZ case also exercises the two-byte opcode map and signed
	// branch displacement.
	for _, tc := range []struct {
		code  []byte
		flags uint32
	}{
		{[]byte{0x74, 7, 0xB8, 1, 0, 0, 0, 0xEB, 5, 0xB8, 2, 0, 0, 0, 0xF4}, x86FlagZF},
		{[]byte{0x74, 7, 0xB8, 1, 0, 0, 0, 0xEB, 5, 0xB8, 2, 0, 0, 0, 0xF4}, 0},
		{[]byte{0x0F, 0x85, 7, 0, 0, 0, 0xB8, 1, 0, 0, 0, 0xEB, 5, 0xB8, 2, 0, 0, 0, 0xF4}, 0},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.Flags = tc.flags
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT Jcc program did not halt: % X", tc.code)
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectJccAllConditionsParity(t *testing.T) {
	// Exercise every short Jcc predicate against all combinations of the flag
	// bits it can observe. This guards the ARM64 predicate materialisation from
	// accidentally inheriting host NZCV or conflating signed conditions.
	flagBits := []uint32{x86FlagCF, x86FlagPF, x86FlagZF, x86FlagSF, x86FlagOF}
	for condition := byte(0); condition < 16; condition++ {
		for combination := 0; combination < 1<<len(flagBits); combination++ {
			flags := uint32(0)
			for bit, mask := range flagBits {
				if combination&(1<<bit) != 0 {
					flags |= mask
				}
			}
			code := []byte{0x70 + condition, 7, 0xB8, 1, 0, 0, 0, 0xEB, 5, 0xB8, 2, 0, 0, 0, 0xF4}
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.Flags = flags
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if !jit.Halted {
				t.Fatalf("Jcc %02X did not halt with EFLAGS=%08X", code[0], flags)
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("Jcc %02X EFLAGS=%08X: EAX=%08X, want %08X", code[0], flags, got, want)
			}
			if got, want := jit.EIP, interp.EIP; got != want {
				t.Fatalf("Jcc %02X EFLAGS=%08X: EIP=%08X, want %08X", code[0], flags, got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("Jcc %02X EFLAGS=%08X: cycles=%d, want %d", code[0], flags, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectLoopParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		ecx   uint32
		flags uint32
	}{
		{[]byte{0xE2, 0xFE, 0xF4}, 2, 0},                        // LOOP
		{[]byte{0xE1, 0xFE, 0xF4}, 2, x86FlagZF},                // LOOPE
		{[]byte{0xE0, 0xFE, 0xF4}, 2, 0},                        // LOOPNE
		{[]byte{0xE1, 0xFE, 0xF4}, 2, 0},                        // LOOPE stops
		{[]byte{0xE3, 0x00, 0xF4}, 0, 0},                        // JCXZ taken
		{[]byte{0x67, 0xE2, 0xFD, 0xF4}, 0x12340002, x86FlagZF}, // LOOP CX
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.ECX, cpu.Flags = tc.ecx, tc.flags
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT LOOP program did not halt: % X", tc.code)
		}
		if got, want := jit.ECX, interp.ECX; got != want {
			t.Fatalf("ECX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.EIP, interp.EIP; got != want {
			t.Fatalf("EIP = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectSETccParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		flags uint32
	}{
		{[]byte{0x0F, 0x94, 0xC4, 0xF4}, x86FlagZF},    // SETZ AH
		{[]byte{0x0F, 0x94, 0xC4, 0xF4}, 0},            // SETZ AH false
		{[]byte{0x0F, 0x9C, 0x43, 1, 0xF4}, x86FlagSF}, // SETL [EBX+1]
		{[]byte{0x0F, 0x9C, 0x43, 1, 0xF4}, x86FlagSF | x86FlagOF},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.EAX = 0xDEADBEEF
			cpu.EBX = 0x800
			cpu.Flags = tc.flags
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT SETcc program did not halt: % X", tc.code)
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.memory[0x801], interp.memory[0x801]; got != want {
			t.Fatalf("[801] = %02X, want %02X for % X", got, want, tc.code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectCMOVccParity(t *testing.T) {
	for _, tc := range []struct {
		code  []byte
		flags uint32
	}{
		{[]byte{0x0F, 0x44, 0xD8, 0xF4}, x86FlagZF},          // CMOVZ EBX,EAX
		{[]byte{0x0F, 0x44, 0xD8, 0xF4}, 0},                  // false condition
		{[]byte{0x0F, 0x44, 0x43, 4, 0xF4}, x86FlagZF},       // CMOVZ EAX,[EBX+4]
		{[]byte{0x66, 0x0F, 0x44, 0x43, 4, 0xF4}, x86FlagZF}, // CMOVZ AX,[EBX+4]
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc.code)
			cpu.EAX, cpu.EBX, cpu.EDX = 0x11223344, 0x800, 0xAABBCCDD
			cpu.Flags = tc.flags
			cpu.memory[0x804], cpu.memory[0x805], cpu.memory[0x806], cpu.memory[0x807] = 0x78, 0x56, 0x34, 0x12
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT CMOVcc program did not halt: % X", tc.code)
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.EBX, interp.EBX; got != want {
			t.Fatalf("EBX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.EDX, interp.EDX; got != want {
			t.Fatalf("EDX = %08X, want %08X for % X", got, want, tc.code)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, tc.code)
		}
	}
}

func TestX86ARM64_DirectBSFBSRParity(t *testing.T) {
	for _, tc := range [][]byte{
		{0x0F, 0xBC, 0xC3, 0xF4},       // BSF EAX,EBX
		{0x0F, 0xBD, 0x43, 4, 0xF4},    // BSR EAX,[EBX+4]
		{0x66, 0x0F, 0xBC, 0xC3, 0xF4}, // BSF AX,BX
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc)
			cpu.EAX, cpu.EBX, cpu.Flags = 0xAABBCCDD, 0x800, x86FlagCF
			cpu.memory[0x804] = 0x00
			cpu.memory[0x805] = 0x10
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT BSF/BSR program did not halt: % X", tc)
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("EAX = %08X, want %08X for % X", got, want, tc)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("EFLAGS = %08X, want %08X for % X", got, want, tc)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("cycles = %d, want %d for % X", got, want, tc)
		}
	}
	// A zero source sets ZF and leaves its destination untouched.
	jit := newX86ARM64DispatchCPU([]byte{0x0F, 0xBC, 0xC3, 0xF4})
	jit.EAX, jit.EBX, jit.Flags = 0x11223344, 0, x86FlagCF
	jit.X86ExecuteJIT()
	if jit.EAX != 0x11223344 || jit.Flags&x86FlagZF == 0 || jit.Flags&x86FlagCF == 0 {
		t.Fatalf("zero BSF state EAX=%08X EFLAGS=%08X", jit.EAX, jit.Flags)
	}
}

func TestX86ARM64_DirectBTRegisterParity(t *testing.T) {
	for _, tc := range [][]byte{
		{0x0F, 0xA3, 0xC8, 0xF4},       // BT EAX,ECX
		{0x66, 0x0F, 0xA3, 0xC8, 0xF4}, // BT AX,CX
		{0x0F, 0xBA, 0xE0, 0x03, 0xF4}, // BT EAX,3
	} {
		for _, bit := range []uint32{3, 4, 19} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(tc)
				cpu.EAX, cpu.ECX, cpu.Flags = 0x00080008, bit, x86FlagZF|x86FlagOF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X bit %d: EFLAGS=%08X, want %08X", tc, bit, got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("% X bit %d: cycles=%d, want %d", tc, bit, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectBitMutationRegisterParity(t *testing.T) {
	for _, tc := range [][]byte{
		{0x0F, 0xAB, 0xC8, 0xF4},       // BTS EAX,ECX
		{0x0F, 0xB3, 0xC8, 0xF4},       // BTR EAX,ECX
		{0x0F, 0xBB, 0xC8, 0xF4},       // BTC EAX,ECX
		{0x66, 0x0F, 0xBB, 0xC8, 0xF4}, // BTC AX,CX
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(tc)
			cpu.EAX, cpu.ECX, cpu.Flags = 0xAABB0008, 3, x86FlagZF|x86FlagOF
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("% X: EAX=%08X, want %08X", tc, got, want)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X: EFLAGS=%08X, want %08X", tc, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X: cycles=%d, want %d", tc, got, want)
		}
	}
}

func TestX86ARM64_DirectTESTRegisterParity(t *testing.T) {
	for _, code := range [][]byte{
		{0x85, 0xD8, 0xF4}, {0x66, 0x85, 0xD8, 0xF4}, // TEST EAX,EBX / AX,BX
		{0x84, 0xE0, 0xF4}, // TEST AL,AH
		{0xA8, 0x0F, 0xF4}, {0xA9, 0x0F, 0, 0, 0, 0xF4}, {0x66, 0xA9, 0x0F, 0, 0xF4},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = 0xAABB0010, 0x1122000F, x86FlagAF|x86FlagCF|x86FlagOF
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("% X EAX=%08X want %08X", code, got, want)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X flags=%08X want %08X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles=%d want %d", code, got, want)
		}
	}
}

func TestX86ARM64_DirectLogicalRegisterParity(t *testing.T) {
	for _, code := range [][]byte{
		{0x09, 0xD8, 0xF4}, {0x21, 0xD8, 0xF4}, {0x31, 0xD8, 0xF4}, // dword r/m,r
		{0x08, 0xE0, 0xF4}, {0x20, 0xE0, 0xF4}, {0x30, 0xE0, 0xF4}, // AH,AL
		{0x66, 0x09, 0xD8, 0xF4}, {0x66, 0x21, 0xD8, 0xF4}, {0x66, 0x31, 0xD8, 0xF4},
		{0x0D, 0x0F, 0, 0, 0, 0xF4}, {0x25, 0x0F, 0, 0, 0, 0xF4}, {0x35, 0x0F, 0, 0, 0, 0xF4},
		{0x0C, 0x0F, 0xF4}, {0x24, 0x0F, 0xF4}, {0x34, 0x0F, 0xF4},
		{0x80, 0xCC, 0x0F, 0xF4}, {0x80, 0xE4, 0x0F, 0xF4}, {0x80, 0xF4, 0x0F, 0xF4}, // AH,imm8
		{0x83, 0xC8, 0xF0, 0xF4}, {0x83, 0xE0, 0xF0, 0xF4}, {0x83, 0xF0, 0xF0, 0xF4},
		{0x66, 0x81, 0xC8, 0x0F, 0, 0xF4}, {0x66, 0x81, 0xE0, 0x0F, 0, 0xF4}, {0x66, 0x81, 0xF0, 0x0F, 0, 0xF4},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = 0x8000F0F0, 0x0F, x86FlagAF|x86FlagCF|x86FlagOF
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("% X EAX=%08X want %08X", code, got, want)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X flags=%08X want %08X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles=%d want %d", code, got, want)
		}
	}
}

func TestX86ARM64_DirectArithmeticRegisterParity(t *testing.T) {
	cases := [][]byte{
		{0x00, 0xE0, 0xF4}, {0x02, 0xE0, 0xF4}, // ADD AL,AH / AH,AL
		{0x01, 0xD8, 0xF4}, {0x03, 0xD8, 0xF4}, // ADD EAX,EBX / EBX,EAX
		{0x66, 0x01, 0xD8, 0xF4},
		{0x28, 0xE0, 0xF4}, {0x2A, 0xE0, 0xF4}, // SUB AL,AH / AH,AL
		{0x29, 0xD8, 0xF4}, {0x2B, 0xD8, 0xF4},
		{0x38, 0xE0, 0xF4}, {0x3A, 0xE0, 0xF4}, // CMP AL,AH / AH,AL
		{0x39, 0xD8, 0xF4}, {0x3B, 0xD8, 0xF4}, {0x66, 0x39, 0xD8, 0xF4},
		{0x04, 0xFF, 0xF4}, {0x05, 1, 0, 0, 0, 0xF4}, {0x66, 0x05, 1, 0, 0xF4},
		{0x2C, 0xFF, 0xF4}, {0x2D, 1, 0, 0, 0, 0xF4}, {0x3C, 1, 0xF4}, {0x3D, 1, 0, 0, 0, 0xF4},
		{0x80, 0xC0, 0xFF, 0xF4}, {0x80, 0xE8, 1, 0xF4}, {0x80, 0xF8, 1, 0xF4},
		{0x83, 0xC0, 0xFF, 0xF4}, {0x83, 0xE8, 1, 0xF4}, {0x83, 0xF8, 1, 0xF4},
		{0x66, 0x81, 0xC0, 1, 0, 0xF4}, {0x66, 0x81, 0xE8, 1, 0, 0xF4}, {0x66, 0x81, 0xF8, 1, 0, 0xF4},
	}
	for _, code := range cases {
		for _, regs := range [][2]uint32{{0x7F80FF01, 0x000000FF}, {0x8000000F, 0x80000001}, {0xFFFFFFFF, 1}, {0, 1}} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.EBX, cpu.Flags = regs[0], regs[1], x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("% X EAX=%08X want %08X", code, got, want)
			}
			if got, want := jit.EBX, interp.EBX; got != want {
				t.Fatalf("% X EBX=%08X want %08X", code, got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X regs=%08X/%08X flags=%08X want %08X", code, regs[0], regs[1], got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("% X cycles=%d want %d", code, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectINCDECParity(t *testing.T) {
	for _, code := range [][]byte{{0x40, 0xF4}, {0x48, 0xF4}, {0x66, 0x40, 0xF4}, {0x66, 0x48, 0xF4}} {
		for _, eax := range []uint32{0, 1, 0x7FFF, 0x7FFFFFFF, 0xFFFFFFFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.Flags = eax, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("% X EAX=%08X want %08X", code, got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X EAX=%08X flags=%08X want %08X", code, eax, got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("% X cycles=%d want %d", code, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectGroup4INCDECParity(t *testing.T) {
	for _, code := range [][]byte{{0xFE, 0xC0, 0xF4}, {0xFE, 0xC8, 0xF4}, {0xFE, 0xC4, 0xF4}, {0xFE, 0xCC, 0xF4}} {
		for _, eax := range []uint32{0, 1, 0x7F, 0x80, 0xFF, 0xA5A5007F} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.Flags = eax, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("% X EAX=%08X want %08X", code, got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X EAX=%08X flags=%08X want %08X", code, eax, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectShiftOneParity(t *testing.T) {
	for _, code := range [][]byte{
		{0xC1, 0xC0, 33, 0xF4}, {0xC1, 0xC8, 33, 0xF4}, {0xC1, 0xE0, 33, 0xF4}, {0xC1, 0xE8, 33, 0xF4}, {0xC1, 0xF8, 33, 0xF4},
		{0xC0, 0xC0, 1, 0xF4}, {0xC0, 0xC8, 1, 0xF4}, {0xC0, 0xE0, 1, 0xF4}, {0xC0, 0xE8, 1, 0xF4}, {0xC0, 0xF8, 1, 0xF4},
		{0x66, 0xC1, 0xC0, 1, 0xF4}, {0x66, 0xC1, 0xC8, 1, 0xF4}, {0x66, 0xC1, 0xE0, 1, 0xF4}, {0x66, 0xC1, 0xE8, 1, 0xF4}, {0x66, 0xC1, 0xF8, 1, 0xF4},
		{0xC1, 0xC0, 1, 0xF4}, {0xC1, 0xC8, 1, 0xF4}, {0xC1, 0xE0, 1, 0xF4}, {0xC1, 0xE8, 1, 0xF4}, {0xC1, 0xF8, 1, 0xF4},
		{0xD0, 0xC0, 0xF4}, {0xD0, 0xC8, 0xF4}, {0xD0, 0xE0, 0xF4}, {0xD0, 0xE8, 0xF4}, {0xD0, 0xF8, 0xF4},
		{0x66, 0xD1, 0xC0, 0xF4}, {0x66, 0xD1, 0xC8, 0xF4}, {0x66, 0xD1, 0xE0, 0xF4}, {0x66, 0xD1, 0xE8, 0xF4}, {0x66, 0xD1, 0xF8, 0xF4},
		{0xD1, 0xC0, 0xF4}, {0xD1, 0xC8, 0xF4}, {0xD1, 0xE0, 0xF4}, {0xD1, 0xE8, 0xF4}, {0xD1, 0xF0, 0xF4}, {0xD1, 0xF8, 0xF4},
	} {
		for _, eax := range []uint32{0, 1, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.Flags = eax, x86FlagAF|x86FlagPF|x86FlagZF
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X EAX=%08X/%08X flags=%08X/%08X cycles=%d/%d", code, jit.EAX, interp.EAX, jit.Flags, interp.Flags, jit.Cycles, interp.Cycles)
			}
		}
	}
}

func TestX86ARM64_DirectShiftImmediate32Parity(t *testing.T) {
	for _, code := range [][]byte{{0xC1, 0xC0, 34, 0xF4}, {0xC1, 0xC8, 7, 0xF4}, {0xC1, 0xE0, 34, 0xF4}, {0xC1, 0xE8, 7, 0xF4}, {0xC1, 0xF0, 2, 0xF4}, {0xC1, 0xF8, 255, 0xF4}, {0xC1, 0xE0, 32, 0xF4}} {
		for _, eax := range []uint32{0, 1, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.Flags = eax, x86FlagAF|x86FlagPF|x86FlagZF|x86FlagOF
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X EAX=%08X/%08X flags=%08X/%08X cycles=%d/%d", code, jit.EAX, interp.EAX, jit.Flags, interp.Flags, jit.Cycles, interp.Cycles)
			}
		}
	}
}

func TestX86ARM64_DirectDoubleShiftImmediateParity(t *testing.T) {
	for _, code := range [][]byte{{0x0F, 0xA4, 0xD8, 1, 0xF4}, {0x0F, 0xA4, 0xD8, 7, 0xF4}, {0x0F, 0xAC, 0xD8, 1, 0xF4}, {0x0F, 0xAC, 0xD8, 31, 0xF4}} {
		for _, regs := range [][2]uint32{{0, 0}, {1, 0x80000000}, {0x7FFFFFFF, 0xFFFFFFFF}, {0x80000000, 1}} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.EBX, cpu.Flags = regs[0], regs[1], x86FlagAF|x86FlagPF|x86FlagZF|x86FlagOF
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X EAX=%08X/%08X flags=%08X/%08X cycles=%d/%d", code, jit.EAX, interp.EAX, jit.Flags, interp.Flags, jit.Cycles, interp.Cycles)
			}
		}
	}
}

func TestX86ARM64_DirectIMUL32ImmediateParity(t *testing.T) {
	for _, code := range [][]byte{{0x6B, 0xC3, 0xFE, 0xF4}, {0x69, 0xC3, 0x00, 0x00, 0x01, 0x80, 0xF4}} {
		for _, ebx := range []uint32{0, 1, 2, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.EBX, cpu.Flags = 0xB4B60000, ebx, x86FlagPF|x86FlagAF|x86FlagZF
				return cpu
			}
			jit, interp := newCPU(), newCPU()
			jit.X86ExecuteJIT()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X EBX=%08X EAX=%08X/%08X flags=%08X/%08X", code, ebx, jit.EAX, interp.EAX, jit.Flags, interp.Flags)
			}
		}
	}
}

func TestX86ARM64_DirectIMUL32RegisterParity(t *testing.T) {
	code := []byte{0x0F, 0xAF, 0xC3, 0xF4}
	for _, regs := range [][2]uint32{{0, 1}, {1, 2}, {0x7FFFFFFF, 2}, {0x80000000, 2}, {0xFFFFFFFF, 0xFFFFFFFF}} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = regs[0], regs[1], x86FlagPF|x86FlagAF|x86FlagZF
			return cpu
		}
		jit, interp := newCPU(), newCPU()
		jit.X86ExecuteJIT()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
			t.Fatalf("EAX=%08X/%08X flags=%08X/%08X", jit.EAX, interp.EAX, jit.Flags, interp.Flags)
		}
	}
}

func TestX86ARM64_DirectIMUL32MemoryParity(t *testing.T) {
	code := []byte{0x6B, 0x03, 0xFE, 0xF4}
	for _, value := range []uint32{0, 1, 2, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EBX, cpu.Flags = 0x400, x86FlagPF|x86FlagAF|x86FlagZF
			cpu.memory[0x400] = byte(value)
			cpu.memory[0x401] = byte(value >> 8)
			cpu.memory[0x402] = byte(value >> 16)
			cpu.memory[0x403] = byte(value >> 24)
			return cpu
		}
		jit, interp := newCPU(), newCPU()
		jit.X86ExecuteJIT()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
			t.Fatalf("value=%08X EAX=%08X/%08X flags=%08X/%08X", value, jit.EAX, interp.EAX, jit.Flags, interp.Flags)
		}
	}
}

func TestX86ARM64_DirectIMUL32RegisterMemoryParity(t *testing.T) {
	code := []byte{0x0F, 0xAF, 0x03, 0xF4}
	for _, value := range []uint32{0, 1, 2, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = 3, 0x400, x86FlagPF|x86FlagAF|x86FlagZF
			cpu.memory[0x400] = byte(value)
			cpu.memory[0x401] = byte(value >> 8)
			cpu.memory[0x402] = byte(value >> 16)
			cpu.memory[0x403] = byte(value >> 24)
			return cpu
		}
		jit, interp := newCPU(), newCPU()
		jit.X86ExecuteJIT()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if jit.EAX != interp.EAX || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
			t.Fatalf("value=%08X EAX=%08X/%08X flags=%08X/%08X", value, jit.EAX, interp.EAX, jit.Flags, interp.Flags)
		}
	}
}

func TestX86ARM64_DirectMemoryShiftOneParity(t *testing.T) {
	for _, code := range [][]byte{
		{0xD0, 0x23, 0xF4}, {0xD0, 0x2B, 0xF4}, {0xD0, 0x3B, 0xF4},
		{0x66, 0xD1, 0x23, 0xF4}, {0x66, 0xD1, 0x2B, 0xF4}, {0x66, 0xD1, 0x3B, 0xF4},
		{0xD1, 0x23, 0xF4}, {0xD1, 0x2B, 0xF4}, {0xD1, 0x3B, 0xF4},
	} {
		for _, value := range []uint32{0, 1, 0x7F, 0x80, 0x7FFF, 0x8000, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EBX, cpu.Flags = 0x400, x86FlagAF|x86FlagPF|x86FlagZF
				cpu.memory[0x400] = byte(value)
				cpu.memory[0x401] = byte(value >> 8)
				cpu.memory[0x402] = byte(value >> 16)
				cpu.memory[0x403] = byte(value >> 24)
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.memory[0x400:0x404], interp.memory[0x400:0x404]; !bytes.Equal(got, want) || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
				t.Fatalf("% X value=%08X memory=% X/% X flags=%08X/%08X cycles=%d/%d", code, value, got, want, jit.Flags, interp.Flags, jit.Cycles, interp.Cycles)
			}
		}
	}
}

func TestX86ARM64_DirectMemoryGroup4INCDECParity(t *testing.T) {
	for _, code := range [][]byte{{0xFE, 0x03, 0xF4}, {0xFE, 0x0B, 0xF4}} {
		for _, v := range []byte{0, 1, 0x7F, 0x80, 0xFF} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EBX, cpu.Flags = 0x400, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
				cpu.memory[0x400] = v
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.memory[0x400], interp.memory[0x400]; got != want {
				t.Fatalf("% X value=%02X memory=%02X want %02X", code, v, got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X value=%02X flags=%08X want %08X", code, v, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectMemoryArithmeticParity(t *testing.T) {
	cases := [][]byte{
		{0x01, 0x03, 0xF4}, {0x03, 0x03, 0xF4}, // ADD [EBX],EAX / EAX,[EBX]
		{0x29, 0x03, 0xF4}, {0x2B, 0x03, 0xF4}, // SUB [EBX],EAX / EAX,[EBX]
		{0x39, 0x03, 0xF4}, {0x3B, 0x03, 0xF4}, // CMP [EBX],EAX / EAX,[EBX]
		{0x00, 0x03, 0xF4}, {0x02, 0x03, 0xF4}, // ADD [EBX],AL / AL,[EBX]
		{0x66, 0x29, 0x03, 0xF4}, {0x66, 0x3B, 0x03, 0xF4},
		{0x09, 0x03, 0xF4}, {0x0B, 0x03, 0xF4}, {0x21, 0x03, 0xF4}, {0x23, 0x03, 0xF4}, {0x31, 0x03, 0xF4}, {0x33, 0x03, 0xF4},
		{0x08, 0x03, 0xF4}, {0x0A, 0x03, 0xF4}, {0x66, 0x21, 0x03, 0xF4},
		{0x84, 0x03, 0xF4}, {0x85, 0x03, 0xF4}, {0x66, 0x85, 0x03, 0xF4},
	}
	for _, code := range cases {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = 0x8000000F, 0x400, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
			cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = 0xF0, 0xFF, 0xFF, 0x7F
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("% X EAX=%08X want %08X", code, got, want)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X flags=%08X want %08X", code, got, want)
		}
		if got, want := jit.memory[0x400:0x404], interp.memory[0x400:0x404]; string(got) != string(want) {
			t.Fatalf("% X memory=% X want % X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles=%d want %d", code, got, want)
		}
	}
}

func TestX86ARM64_DirectMemoryGroup1Parity(t *testing.T) {
	for _, code := range [][]byte{
		{0x83, 0x03, 1, 0xF4}, {0x83, 0x0B, 1, 0xF4}, {0x83, 0x23, 1, 0xF4},
		{0x83, 0x2B, 1, 0xF4}, {0x83, 0x33, 1, 0xF4}, {0x83, 0x3B, 1, 0xF4},
		{0x80, 0x03, 1, 0xF4}, {0x80, 0x0B, 1, 0xF4}, {0x80, 0x23, 1, 0xF4},
		{0x66, 0x81, 0x2B, 1, 0, 0xF4}, {0x66, 0x81, 0x3B, 1, 0, 0xF4},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EBX, cpu.Flags = 0x400, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
			cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = 0xFF, 0, 0, 0x80
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X flags=%08X want %08X", code, got, want)
		}
		if got, want := jit.memory[0x400:0x404], interp.memory[0x400:0x404]; string(got) != string(want) {
			t.Fatalf("% X memory=% X want % X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles=%d want %d", code, got, want)
		}
	}
}

func TestX86ARM64_DirectGroup3NotNegParity(t *testing.T) {
	cases := [][]byte{
		{0xF6, 0xD0, 0xF4}, {0xF6, 0xD8, 0xF4}, // NOT AL; NEG AL
		{0xF7, 0xD0, 0xF4}, {0xF7, 0xD8, 0xF4}, // NOT EAX; NEG EAX
		{0x66, 0xF7, 0xD8, 0xF4},
		{0xF6, 0x13, 0xF4}, {0xF6, 0x1B, 0xF4}, // NOT/NEG byte [EBX]
		{0xF7, 0x13, 0xF4}, {0xF7, 0x1B, 0xF4}, // NOT/NEG dword [EBX]
		{0x66, 0xF7, 0x1B, 0xF4},
	}
	for _, code := range cases {
		for _, eax := range []uint32{0, 1, 0x80, 0x8000, 0x80000000} {
			newCPU := func() *CPU_X86 {
				cpu := newX86ARM64DispatchCPU(code)
				cpu.EAX, cpu.EBX, cpu.Flags = eax, 0x400, x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF
				cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = byte(eax), byte(eax>>8), byte(eax>>16), byte(eax>>24)
				return cpu
			}
			jit := newCPU()
			jit.X86ExecuteJIT()
			interp := newCPU()
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.EAX, interp.EAX; got != want {
				t.Fatalf("% X EAX=%08X want %08X", code, got, want)
			}
			if got, want := jit.Flags, interp.Flags; got != want {
				t.Fatalf("% X EAX=%08X flags=%08X want %08X", code, eax, got, want)
			}
			if got, want := jit.memory[0x400:0x404], interp.memory[0x400:0x404]; string(got) != string(want) {
				t.Fatalf("% X EAX=%08X memory=% X want % X", code, eax, got, want)
			}
			if got, want := jit.Cycles, interp.Cycles; got != want {
				t.Fatalf("% X cycles=%d want %d", code, got, want)
			}
		}
	}
}

func TestX86ARM64_DirectGroup3TestEmission(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x180)
	copy(mem[pc:], []byte{0xF7, 0xC0, 0x0F, 0, 0, 0, 0xF4}) // TEST EAX,0F
	instrs := x86ScanBlock(mem, pc)
	if got, want := instrs[0].length, uint16(6); got != want {
		t.Fatalf("TEST EAX,imm32 length=%d want %d", got, want)
	}
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem, Flags: x86FlagCF | x86FlagOF}
	cpu.jitRegs[0] = 0x8000000F
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs[:1], pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.RetPC, pc+6; got != want {
		t.Fatalf("RetPC=%08X want %08X", got, want)
	}
	if got, want := ctx.RetCount, uint32(1); got != want {
		t.Fatalf("RetCount=%d want %d", got, want)
	}
	if got, want := cpu.Flags, uint32(x86FlagPF); got != want {
		t.Fatalf("EFLAGS=%08X want %08X", got, want)
	}
}

func TestX86ARM64_DirectGroup3TestParity(t *testing.T) {
	for _, code := range [][]byte{
		{0xF6, 0xC0, 0x0F, 0xF4}, {0xF6, 0xC4, 0x0F, 0xF4},
		{0xF7, 0xC0, 0x0F, 0, 0, 0, 0xF4},
		{0x66, 0xF7, 0xC0, 0x0F, 0, 0xF4},
		{0xF6, 0x03, 0x0F, 0xF4},
		{0xF7, 0x03, 0x0F, 0, 0, 0, 0xF4},
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.EBX, cpu.Flags = 0x8000000F, 0x400, x86FlagCF|x86FlagAF|x86FlagOF
			cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = 0x0F, 0, 0, 0x80
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("% X EAX=%08X want %08X", code, got, want)
		}
		if got, want := jit.Flags, interp.Flags; got != want {
			t.Fatalf("% X flags=%08X want %08X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles=%d want %d", code, got, want)
		}
	}
}

func TestX86ARM64_ProductionFPUHelperParity(t *testing.T) {
	// F2XM1 intentionally uses the canonical helper path. The following HLT
	// proves that helper replay advances EIP and returns to the dispatcher.
	code := []byte{0xD9, 0xF0, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.FPU.Reset()
		cpu.FPU.regs[0] = 1
		cpu.FPU.setTag(0, x87TagValid)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if !jit.Halted {
		t.Fatal("JIT x87 helper program did not halt")
	}
	if got, want := jit.FPU.regs, interp.FPU.regs; got != want {
		t.Fatalf("FPU registers = %v, want %v", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
		t.Fatalf("FCS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("Cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_FPUHelperPrefixParity(t *testing.T) {
	for _, code := range [][]byte{
		{0x64, 0xD9, 0xF0, 0xF4}, // FS:F2XM1
		{0x67, 0xD9, 0xF0, 0xF4}, // address-size F2XM1
	} {
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.FPU.Reset()
			cpu.FPU.regs[0] = 1
			cpu.FPU.setTag(0, x87TagValid)
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if !jit.Halted {
			t.Fatalf("JIT prefixed FPU helper program did not halt: % X", code)
		}
		if got, want := jit.FPU.regs, interp.FPU.regs; got != want {
			t.Fatalf("% X FPU registers = %v, want %v", code, got, want)
		}
		if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
			t.Fatalf("% X FIP = %08X, want %08X", code, got, want)
		}
		if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
			t.Fatalf("% X FOP = %04X, want %04X", code, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("% X cycles = %d, want %d", code, got, want)
		}
	}
}

func TestX86ARM64_DirectFFREEProvenanceParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xDD, 0xC1, 0xF4}) // FFREE ST(1)
		cpu.FPU.push(2)
		cpu.FPU.push(1)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
		t.Fatalf("FCS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFXCHParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xD9, 0xC9, 0xF4}) // FXCH ST(1)
		cpu.FPU.push(2)
		cpu.FPU.push(1)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := math.Float64bits(jit.FPU.ST(1)), math.Float64bits(interp.FPU.ST(1)); got != want {
		t.Fatalf("ST(1) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFSTPSTiParity(t *testing.T) {
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU([]byte{0xDD, 0xD9, 0xF4}) // FSTP ST(1)
		cpu.FPU.push(2)
		cpu.FPU.push(1)
		return cpu
	}
	admit := newCPU()
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	instrs := x86ScanBlock(admit.memory, admit.EIP)
	if _, err := x86CompileBlockForCPU(admit, instrs[:1], admit.EIP, em); err != nil {
		t.Fatalf("FSTP ST(i) was not admitted natively: %v", err)
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := math.Float64bits(jit.FPU.ST(0)), math.Float64bits(interp.FPU.ST(0)); got != want {
		t.Fatalf("ST(0) = %016X, want %016X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFNOPProvenanceParity(t *testing.T) {
	code := []byte{0xD9, 0xD0, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.CS = 0x2468
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
		t.Fatalf("FCS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("Cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectFNCLEXParity(t *testing.T) {
	code := []byte{0xDB, 0xE2, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.CS = 0x2468
		cpu.FPU.FSW = x87FSW_B | x87FSW_ES | x87FSW_IE | x87FSW_PE | x87FSW_C3
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectFNINITParity(t *testing.T) {
	code := []byte{0xDB, 0xE3, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.FPU.FCW = 0
		cpu.FPU.FSW = x87FSW_B | x87FSW_ES | x87FSW_IE | x87FSW_C3
		cpu.FPU.FTW = 0
		cpu.FPU.FIP, cpu.FPU.FCS = 0x12345678, 0x2468
		cpu.FPU.FDP, cpu.FPU.FDS, cpu.FPU.FOP = 0x87654321, 0x1357, 0xBEEF
		for i := range cpu.FPU.regs {
			cpu.FPU.regs[i] = float64(i) + 0.5
		}
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.FPU.regs, interp.FPU.regs; got != want {
		t.Fatalf("FPU registers = %v, want %v", got, want)
	}
	if got, want := jit.FPU.FCW, interp.FPU.FCW; got != want {
		t.Fatalf("FCW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
		t.Fatalf("FIP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
		t.Fatalf("FCS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FDP, interp.FPU.FDP; got != want {
		t.Fatalf("FDP = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FDS, interp.FPU.FDS; got != want {
		t.Fatalf("FDS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectFNSTSWAXParity(t *testing.T) {
	code := []byte{0xDF, 0xE0, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX = 0xBEEF0000
		cpu.FPU.FSW = x87FSW_IE | x87FSW_C3
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
		t.Fatalf("FOP = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_FPURegisterArithmeticHelperParity(t *testing.T) {
	newCPU := func(code []byte, top int) *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.FPU.Reset()
		cpu.FPU.setTop(top)
		st0, st1 := cpu.FPU.physReg(0), cpu.FPU.physReg(1)
		cpu.FPU.regs[st0], cpu.FPU.regs[st1] = 1.25, 2.5
		cpu.FPU.setTag(st0, x87TagValid)
		cpu.FPU.setTag(st1, x87TagValid)
		return cpu
	}
	for _, op := range []byte{0xC1, 0xC9, 0xE1, 0xE9, 0xF1, 0xF9} {
		code := []byte{0xD8, op, 0xF4}
		for _, top := range []int{0, 7} {
			jit := newCPU(code, top)
			jit.X86ExecuteJIT()
			interp := newCPU(code, top)
			for interp.Running() && !interp.Halted {
				interp.Step()
			}
			if got, want := jit.FPU.regs, interp.FPU.regs; got != want {
				t.Fatalf("op %02X TOP %d FPU registers = %v, want %v", op, top, got, want)
			}
			if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
				t.Fatalf("op %02X TOP %d FIP = %08X, want %08X", op, top, got, want)
			}
			if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
				t.Fatalf("op %02X TOP %d FCS = %04X, want %04X", op, top, got, want)
			}
			if got, want := jit.FPU.FOP, interp.FPU.FOP; got != want {
				t.Fatalf("op %02X TOP %d FOP = %04X, want %04X", op, top, got, want)
			}
		}
	}
	// Empty-stack arithmetic is the admission boundary for the future direct
	// NEON path. It must replay through the helper and retain the interpreter's
	// masked stack-fault status instead of writing a floating-point result.
	code := []byte{0xD8, 0xC1, 0xF4}
	jit := newCPU(code, 0)
	jit.FPU.setTag(0, x87TagEmpty)
	jit.X86ExecuteJIT()
	interp := newCPU(code, 0)
	interp.FPU.setTag(0, x87TagEmpty)
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("empty-stack FSW = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.regs, interp.FPU.regs; got != want {
		t.Fatalf("empty-stack registers = %v, want %v", got, want)
	}
}

// TestX86ARM64_DirectRegisterPrefix executes emitted ARM64, rather than only
// inspecting words. It pins the x86 context offsets, AAPCS64 X0 argument ABI,
// executable-memory publication and the register-file store convention.
func TestX86ARM64_DirectRegisterPrefix(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x100)
	copy(mem[pc:], []byte{
		0xB8, 0x44, 0x33, 0x22, 0x11, // MOV EAX,11223344
		0xBB, 0x88, 0x77, 0x66, 0x55, // MOV EBX,55667788
		0x89, 0xC3, // MOV EBX,EAX
	})
	instrs := x86ScanBlock(mem, pc)
	// The shared scanner intentionally continues through zero-filled test RAM.
	// This fixture exercises only its three explicit instructions.
	instrs = instrs[:3]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0x11223344); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0x11223344); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
	if got, want := ctx.RetPC, pc+12; got != want {
		t.Fatalf("RetPC = %08X, want %08X", got, want)
	}
	if got, want := ctx.RetCount, uint32(3); got != want {
		t.Fatalf("RetCount = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectByteRegisterMoves(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x140)
	copy(mem[pc:], []byte{
		0xB4, 0xAB, // MOV AH,AB
		0x88, 0xE3, // MOV BL,AH
		0x8A, 0xE7, // MOV AH,BH
		0xF4,
	})
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[3] = 0x5500
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0x5500); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0x55AB); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectOperandSizeMoves(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x160)
	copy(mem[pc:], []byte{
		0x66, 0xB8, 0x34, 0x12, // MOV AX,1234
		0x66, 0x89, 0xC3, // MOV BX,AX
		0xF4,
	})
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0xDEAD0000, 0xBEEF0000
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xDEAD1234); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0xBEEF1234); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectMOVRMImmediateRegisters(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1A0)
	copy(mem[pc:], []byte{
		0xC6, 0xC7, 0xAB, // MOV BH,AB
		0xC7, 0xC1, 0x44, 0x33, 0x22, 0x11, // MOV ECX,11223344
		0x66, 0xC7, 0xC2, 0x34, 0x12, // MOV DX,1234
	})
	instrs := x86ScanBlock(mem, pc)[:3]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[2] = 0xDEAD0000
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[3], uint32(0xAB00); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[1], uint32(0x11223344); got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[2], uint32(0xDEAD1234); got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectMemoryMOVReadsAndGuardBails(t *testing.T) {
	code := []byte{0x8B, 0x43, 0x04, 0x8A, 0x6B, 0x08, 0xF4} // MOV EAX,[EBX+4]; MOV CH,[EBX+8]
	jit := newX86ARM64DispatchCPU(code)
	jit.EBX = 0x400
	jit.memory[0x404], jit.memory[0x405], jit.memory[0x406], jit.memory[0x407] = 0x44, 0x33, 0x22, 0x11
	jit.memory[0x408] = 0xAB
	jit.X86ExecuteJIT()
	interp := newX86ARM64DispatchCPU(code)
	interp.EBX = 0x400
	interp.memory[0x404], interp.memory[0x405], interp.memory[0x406], interp.memory[0x407] = 0x44, 0x33, 0x22, 0x11
	interp.memory[0x408] = 0xAB
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.ECX, interp.ECX; got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}

	// The guard must return before modifying the destination. A page marked
	// MMIO is replayed through Step, not read from the backing slice.
	mem := make([]byte, 0x1000)
	const pc = uint32(0x100)
	copy(mem[pc:], []byte{0x8B, 0x03}) // MOV EAX,[EBX]
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0xDEADBEEF, 0x200
	ioBitmap := make([]byte, len(mem)/256)
	ioBitmap[2] = 1
	ctx := newX86JITContext(cpu, make([]byte, len(ioBitmap)), ioBitmap)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	block, err := x86CompileBlockForCPU(cpu, x86ScanBlock(mem, pc)[:1], pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.RetPC, pc; got != want {
		t.Fatalf("guard RetPC = %08X, want %08X", got, want)
	}
	if got, want := ctx.RetCount, uint32(0); got != want {
		t.Fatalf("guard RetCount = %d, want %d", got, want)
	}
	if ctx.NeedIOFallback == 0 {
		t.Fatal("MMIO guard did not request interpreter replay")
	}
	if got, want := cpu.jitRegs[0], uint32(0xDEADBEEF); got != want {
		t.Fatalf("guard changed EAX to %08X, want %08X", got, want)
	}

	// A multi-byte access crossing the visible backing boundary takes the
	// identical pre-mutation exit. This also covers the page-span check.
	cpu.jitRegs[3] = 0xFFF
	ctx.RetPC, ctx.RetCount, ctx.NeedIOFallback = 0, 0, 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.RetPC, pc; got != want {
		t.Fatalf("boundary guard RetPC = %08X, want %08X", got, want)
	}
	if ctx.NeedIOFallback == 0 {
		t.Fatal("boundary guard did not request interpreter replay")
	}
	if got, want := cpu.jitRegs[0], uint32(0xDEADBEEF); got != want {
		t.Fatalf("boundary guard changed EAX to %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectMemoryMOVStorePublishesSMCInvalidation(t *testing.T) {
	mem := make([]byte, 0x1000)
	const pc = uint32(0x100)
	copy(mem[pc:], []byte{0x89, 0x03}) // MOV [EBX],EAX
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0x11223344, 0x200
	ioBitmap, codeBitmap := make([]byte, len(mem)/256), make([]byte, len(mem)/256)
	codeBitmap[2] = 1
	ctx := newX86JITContext(cpu, codeBitmap, ioBitmap)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	block, err := x86CompileBlockForCPU(cpu, x86ScanBlock(mem, pc)[:1], pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := mem[0x200:0x204], []byte{0x44, 0x33, 0x22, 0x11}; string(got) != string(want) {
		t.Fatalf("stored bytes = % X, want % X", got, want)
	}
	if ctx.NeedInval == 0 {
		t.Fatal("store to compiled page did not request invalidation")
	}
	if got, want := ctx.InvalAddr, uint32(0x200); got != want {
		t.Fatalf("InvalAddr = %08X, want %08X", got, want)
	}
	if got, want := ctx.InvalSize, uint32(4); got != want {
		t.Fatalf("InvalSize = %d, want %d", got, want)
	}
	if got, want := ctx.RetPC, pc+2; got != want {
		t.Fatalf("RetPC = %08X, want %08X", got, want)
	}
	if got, want := ctx.RetCount, uint32(1); got != want {
		t.Fatalf("RetCount = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectMemoryMOVImmediateStores(t *testing.T) {
	code := []byte{
		0xC6, 0x43, 0x01, 0xAB, // MOV byte [EBX+1],AB
		0xC7, 0x43, 0x04, 0x44, 0x33, 0x22, 0x11, // MOV dword [EBX+4],11223344
		0xF4,
	}
	jit := newX86ARM64DispatchCPU(code)
	jit.EBX = 0x400
	jit.X86ExecuteJIT()
	interp := newX86ARM64DispatchCPU(code)
	interp.EBX = 0x400
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.memory[0x401], interp.memory[0x401]; got != want {
		t.Fatalf("byte store = %02X, want %02X", got, want)
	}
	if got, want := jit.memory[0x404:0x408], interp.memory[0x404:0x408]; string(got) != string(want) {
		t.Fatalf("dword store = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectMemoryMOVExtensions(t *testing.T) {
	code := []byte{
		0x0F, 0xB6, 0x43, 0x01, // MOVZX EAX,byte [EBX+1]
		0x0F, 0xB7, 0x4B, 0x02, // MOVZX ECX,word [EBX+2]
		0x0F, 0xBE, 0x53, 0x04, // MOVSX EDX,byte [EBX+4]
		0x0F, 0xBF, 0x7B, 0x06, // MOVSX EDI,word [EBX+6]
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EBX = 0x400
		cpu.memory[0x401] = 0xAB
		cpu.memory[0x402], cpu.memory[0x403] = 0x34, 0x12
		cpu.memory[0x404] = 0x80
		cpu.memory[0x406], cpu.memory[0x407] = 0x01, 0x80
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	for _, reg := range []struct {
		name string
		got  uint32
		want uint32
	}{
		{"EAX", jit.EAX, interp.EAX},
		{"ECX", jit.ECX, interp.ECX},
		{"EDX", jit.EDX, interp.EDX},
		{"EDI", jit.EDI, interp.EDI},
	} {
		if reg.got != reg.want {
			t.Fatalf("%s = %08X, want %08X", reg.name, reg.got, reg.want)
		}
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectMemorySegmentMOV(t *testing.T) {
	code := []byte{
		0x8C, 0x5B, 0x00, // MOV word [EBX],DS
		0x8E, 0x63, 0x02, // MOV FS,word [EBX+2]
		0x8C, 0x4B, 0x04, // MOV word [EBX+4],CS
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EBX, cpu.DS, cpu.CS = 0x400, 0x1357, 0x2468
		cpu.memory[0x402], cpu.memory[0x403] = 0xEF, 0xBE
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.memory[0x400:0x406], interp.memory[0x400:0x406]; string(got) != string(want) {
		t.Fatalf("segment memory = % X, want % X", got, want)
	}
	if got, want := jit.FS, interp.FS; got != want {
		t.Fatalf("FS = %04X, want %04X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectRegisterPushPop(t *testing.T) {
	code := []byte{
		0x50,       // PUSH EAX
		0x66, 0x53, // PUSH BX
		0x59,       // POP ECX
		0x66, 0x5A, // POP DX
		0x54, // PUSH ESP (pre-decrement ESP value)
		0x5C, // POP ESP
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX, cpu.EBX, cpu.ECX, cpu.EDX, cpu.ESP = 0x11223344, 0xBEEFCAFE, 0xDEADBEEF, 0xA5A5A5A5, 0x500
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	for _, reg := range []struct {
		name string
		got  uint32
		want uint32
	}{
		{"ECX", jit.ECX, interp.ECX},
		{"EDX", jit.EDX, interp.EDX},
		{"ESP", jit.ESP, interp.ESP},
	} {
		if reg.got != reg.want {
			t.Fatalf("%s = %08X, want %08X", reg.name, reg.got, reg.want)
		}
	}
	if got, want := jit.memory[0x4F8:0x500], interp.memory[0x4F8:0x500]; string(got) != string(want) {
		t.Fatalf("stack bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectImmediatePush(t *testing.T) {
	code := []byte{
		0x68, 0x44, 0x33, 0x22, 0x11, // PUSH 11223344
		0x66, 0x68, 0x34, 0x12, // PUSH 1234
		0x6A, 0x80, // PUSH -128
		0x66, 0x6A, 0x80, // PUSH word -128
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP = 0x500
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x4F4:0x500], interp.memory[0x4F4:0x500]; string(got) != string(want) {
		t.Fatalf("stack bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectSegmentPushPop(t *testing.T) {
	code := []byte{0x06, 0x16, 0x1F, 0x07, 0xF4} // PUSH ES; PUSH SS; POP DS; POP ES
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ES, cpu.SS, cpu.DS, cpu.ESP = 0x1111, 0x2222, 0x3333, 0x500
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	for _, seg := range []struct {
		name string
		got  uint16
		want uint16
	}{
		{"ES", jit.ES, interp.ES},
		{"DS", jit.DS, interp.DS},
	} {
		if seg.got != seg.want {
			t.Fatalf("%s = %04X, want %04X", seg.name, seg.got, seg.want)
		}
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x4FC:0x500], interp.memory[0x4FC:0x500]; string(got) != string(want) {
		t.Fatalf("stack bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectMemoryXCHG(t *testing.T) {
	code := []byte{
		0x87, 0x03, // XCHG dword [EBX],EAX
		0x66, 0x87, 0x4B, 0x04, // XCHG word [EBX+4],CX
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX, cpu.ECX, cpu.EBX = 0x11223344, 0xBEEFCAFE, 0x400
		cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = 0xDD, 0xCC, 0xBB, 0xAA
		cpu.memory[0x404], cpu.memory[0x405] = 0x34, 0x12
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.ECX, interp.ECX; got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x400:0x406], interp.memory[0x400:0x406]; string(got) != string(want) {
		t.Fatalf("XCHG memory = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectPushfPopf(t *testing.T) {
	code := []byte{0x9C, 0x58, 0x9D, 0xF4} // PUSHFD; POP EAX; POPFD
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP, cpu.Flags = 0x500, 0xA5A50123
		cpu.memory[0x500], cpu.memory[0x501], cpu.memory[0x502], cpu.memory[0x503] = 0xEF, 0xBE, 0xAD, 0xDE
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.Flags, interp.Flags; got != want {
		t.Fatalf("Flags = %08X, want %08X", got, want)
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x4FC:0x504], interp.memory[0x4FC:0x504]; string(got) != string(want) {
		t.Fatalf("stack bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectPushaPopa(t *testing.T) {
	code := []byte{0x60, 0x61, 0x66, 0x60, 0x66, 0x61, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX, cpu.ECX, cpu.EDX, cpu.EBX = 0x11223344, 0x55667788, 0x99AABBCC, 0xDDEEFF00
		cpu.ESP, cpu.EBP, cpu.ESI, cpu.EDI = 0x500, 0x13572468, 0x24681357, 0xCAFEBEEF
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := [8]uint32{jit.EAX, jit.ECX, jit.EDX, jit.EBX, jit.ESP, jit.EBP, jit.ESI, jit.EDI}, [8]uint32{interp.EAX, interp.ECX, interp.EDX, interp.EBX, interp.ESP, interp.EBP, interp.ESI, interp.EDI}; got != want {
		t.Fatalf("register file = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x4E0:0x500], interp.memory[0x4E0:0x500]; string(got) != string(want) {
		t.Fatalf("stack bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectPopMemory(t *testing.T) {
	code := []byte{0x8F, 0x03, 0x66, 0x8F, 0x43, 0x04, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EBX, cpu.ESP = 0x400, 0x500
		cpu.memory[0x500], cpu.memory[0x501], cpu.memory[0x502], cpu.memory[0x503] = 0x44, 0x33, 0x22, 0x11
		cpu.memory[0x504], cpu.memory[0x505] = 0x34, 0x12
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.memory[0x400:0x406], interp.memory[0x400:0x406]; string(got) != string(want) {
		t.Fatalf("POP memory = % X, want % X", got, want)
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectMoffsMOV(t *testing.T) {
	code := []byte{
		0xA0, 0x00, 0x04, 0x00, 0x00, // MOV AL,[0400]
		0xA2, 0x10, 0x04, 0x00, 0x00, // MOV [0410],AL
		0xA1, 0x00, 0x04, 0x00, 0x00, // MOV EAX,[0400]
		0xA3, 0x08, 0x04, 0x00, 0x00, // MOV [0408],EAX
		0x66, 0xA1, 0x04, 0x04, 0x00, 0x00, // MOV AX,[0404]
		0x66, 0xA3, 0x0C, 0x04, 0x00, 0x00, // MOV [040C],AX
		0xF4,
	}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.memory[0x400], cpu.memory[0x401], cpu.memory[0x402], cpu.memory[0x403] = 0x44, 0x33, 0x22, 0x11
		cpu.memory[0x404], cpu.memory[0x405] = 0xCD, 0xAB
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x408:0x411], interp.memory[0x408:0x411]; string(got) != string(want) {
		t.Fatalf("moffs memory = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectXLAT(t *testing.T) {
	code := []byte{0xD7, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.EAX, cpu.EBX = 0xDEAD00A5, 0x400
		cpu.memory[0x4A5] = 0x7E
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.EAX, interp.EAX; got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectSALC(t *testing.T) {
	for _, flags := range []uint32{0, x86FlagCF} {
		code := []byte{0xD6, 0xF4}
		newCPU := func() *CPU_X86 {
			cpu := newX86ARM64DispatchCPU(code)
			cpu.EAX, cpu.Flags = 0xDEADBE12, flags
			return cpu
		}
		jit := newCPU()
		jit.X86ExecuteJIT()
		interp := newCPU()
		for interp.Running() && !interp.Halted {
			interp.Step()
		}
		if got, want := jit.EAX, interp.EAX; got != want {
			t.Fatalf("flags %08X: EAX = %08X, want %08X", flags, got, want)
		}
		if got, want := jit.Cycles, interp.Cycles; got != want {
			t.Fatalf("flags %08X: cycles = %d, want %d", flags, got, want)
		}
	}
}

func TestX86ARM64_DirectWAIT(t *testing.T) {
	code := []byte{0x9B, 0xF4}
	jit := newX86ARM64DispatchCPU(code)
	jit.X86ExecuteJIT()
	interp := newX86ARM64DispatchCPU(code)
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectLEAVE(t *testing.T) {
	code := []byte{0xC9, 0x66, 0xC9, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP, cpu.EBP = 0xDEAD0400, 0x500
		cpu.memory[0x500], cpu.memory[0x501], cpu.memory[0x502], cpu.memory[0x503] = 0x00, 0x06, 0x00, 0x00
		cpu.memory[0x600], cpu.memory[0x601] = 0x34, 0x12
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.EBP, interp.EBP; got != want {
		t.Fatalf("EBP = %08X, want %08X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_DirectENTERLevelZero(t *testing.T) {
	code := []byte{0xC8, 0x08, 0x00, 0x00, 0xC9, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ESP, cpu.EBP = 0x500, 0x12345678
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.ESP, interp.ESP; got != want {
		t.Fatalf("ESP = %08X, want %08X", got, want)
	}
	if got, want := jit.EBP, interp.EBP; got != want {
		t.Fatalf("EBP = %08X, want %08X", got, want)
	}
	if got, want := jit.memory[0x4F4:0x500], interp.memory[0x4F4:0x500]; string(got) != string(want) {
		t.Fatalf("frame bytes = % X, want % X", got, want)
	}
	if got, want := jit.Cycles, interp.Cycles; got != want {
		t.Fatalf("cycles = %d, want %d", got, want)
	}
}

func TestX86ARM64_ProductionMOVStoreInvalidatesCachedTarget(t *testing.T) {
	// Pre-publish a target block, then modify its first byte through a native
	// MOV store. The dispatcher must remove that target before it can be used.
	cpu := newX86ARM64DispatchCPU([]byte{0x89, 0x03, 0xF4}) // MOV [EBX],EAX; HLT
	const target = uint32(0x200)
	copy(cpu.memory[target:], []byte{0x90, 0xF4})
	cpu.EAX, cpu.EBX = 0x00000090, target
	if err := cpu.initX86JIT(); err != nil {
		t.Fatal(err)
	}
	cpu.x86JitPersist = true
	t.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})
	targetBlock, err := x86CompileBlockForCPU(cpu, x86ScanBlock(cpu.memory, target), target, cpu.x86GetJITExecMem())
	if err != nil {
		t.Fatal(err)
	}
	cpu.x86JitCache.Put(targetBlock)
	x86MarkCodePagesForBlock(cpu.x86JitCodeBM, targetBlock)
	if cpu.x86JitCache.Get(uint64(target)) == nil {
		t.Fatal("failed to publish target cache block")
	}
	cpu.X86ExecuteJIT()
	if !cpu.Halted {
		t.Fatal("program did not halt")
	}
	if cpu.x86JitCache.Get(uint64(target)) != nil {
		t.Fatal("native store left a stale cached target block")
	}
}

func TestX86ARM64_DirectSegmentMOVRegisters(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1B0)
	copy(mem[pc:], []byte{
		0x8C, 0xD8, // MOV AX,DS
		0x8E, 0xE1, // MOV FS,CX
		0x8C, 0xE2, // MOV DX,FS
	})
	instrs := x86ScanBlock(mem, pc)[:3]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem, DS: 0x1357}
	cpu.jitRegs[0], cpu.jitRegs[1], cpu.jitRegs[2] = 0xBEEF0000, 0xCAFE2468, 0xDEAD0000
	cpu.syncJITSegRegsFromNamed()
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xBEEF1357); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitSegRegs[x86SegFS], uint16(0x2468); got != want {
		t.Fatalf("FS = %04X, want %04X", got, want)
	}
	if got, want := cpu.jitRegs[2], uint32(0xDEAD2468); got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectSignExtendInstructions(t *testing.T) {
	run := func(code []byte, instrCount int, eax, edx uint32) (*CPU_X86, error) {
		mem := make([]byte, 0x2000)
		const pc = uint32(0x1B8)
		copy(mem[pc:], code)
		em, err := AllocExecMem(4096)
		if err != nil {
			return nil, err
		}
		t.Cleanup(em.Free)
		cpu := &CPU_X86{memory: mem}
		cpu.jitRegs[0], cpu.jitRegs[2] = eax, edx
		ctx := newX86JITContext(cpu, nil, nil)
		block, err := x86CompileBlockForCPU(cpu, x86ScanBlock(mem, pc)[:instrCount], pc, em)
		if err != nil {
			return nil, err
		}
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		return cpu, nil
	}
	// CBW then CWDE preserve EAX's upper half for the first instruction and
	// sign-extend the resulting AX for the second.
	cpu, err := run([]byte{0x66, 0x98, 0x98}, 2, 0xBEEF8081, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cpu.jitRegs[0], uint32(0xFFFFFF81); got != want {
		t.Fatalf("CWDE EAX = %08X, want %08X", got, want)
	}
	cpu, err = run([]byte{0x66, 0x99}, 1, 0xDEAD8001, 0xBEEF1234)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cpu.jitRegs[2], uint32(0xBEEFFFFF); got != want {
		t.Fatalf("CWD EDX = %08X, want %08X", got, want)
	}
	cpu, err = run([]byte{0x99}, 1, 0x80000001, 0x12345678)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cpu.jitRegs[2], uint32(0xFFFFFFFF); got != want {
		t.Fatalf("CDQ EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_SegmentMOVPublishesBeforeFPUHelper(t *testing.T) {
	// MOV CS,CX must become visible before the following x87 helper captures
	// FCS. The flat model permits the selector assignment as state only.
	code := []byte{0x8E, 0xC9, 0xD9, 0xF0, 0xF4}
	newCPU := func() *CPU_X86 {
		cpu := newX86ARM64DispatchCPU(code)
		cpu.ECX = 0x2468
		cpu.FPU.Reset()
		cpu.FPU.regs[0] = 1
		cpu.FPU.setTag(0, x87TagValid)
		return cpu
	}
	jit := newCPU()
	jit.X86ExecuteJIT()
	interp := newCPU()
	for interp.Running() && !interp.Halted {
		interp.Step()
	}
	if got, want := jit.CS, interp.CS; got != want {
		t.Fatalf("CS = %04X, want %04X", got, want)
	}
	if got, want := jit.FPU.FCS, interp.FPU.FCS; got != want {
		t.Fatalf("FCS = %04X, want %04X", got, want)
	}
}

func TestX86ARM64_DirectLEABaseDisplacement(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1C0)
	copy(mem[pc:], []byte{0x8D, 0x43, 0xF0, 0xF4}) // LEA EAX,[EBX-16]
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[3] = 0x100
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xF0); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectLEASIBAndAbsolute(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1D0)
	copy(mem[pc:], []byte{
		0x8D, 0x44, 0x8D, 0xF0, // LEA EAX,[EBP+ECX*4-16]
		0x8D, 0x1D, 0x78, 0x56, 0x34, 0x12, // LEA EBX,[12345678]
	})
	instrs := x86ScanBlock(mem, pc)[:2]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[5], cpu.jitRegs[1] = 0x1000, 3
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xFFC); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0x12345678); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectXCHGAndBSWAP(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1E0)
	copy(mem[pc:], []byte{0x93, 0x0F, 0xCB, 0xF4}) // XCHG EAX,EBX; BSWAP EBX
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0x11223344, 0xA1B2C3D4
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xA1B2C3D4); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0x44332211); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectOperandSizeXCHG(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x210)
	copy(mem[pc:], []byte{
		0x66, 0x93, // XCHG AX,BX
		0x66, 0x87, 0xCA, // XCHG DX,CX
	})
	instrs := x86ScanBlock(mem, pc)[:2]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0xAAAA1111, 0xBBBB2222
	cpu.jitRegs[1], cpu.jitRegs[2] = 0xCCCC3333, 0xDDDD4444
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xAAAA2222); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[3], uint32(0xBBBB1111); got != want {
		t.Fatalf("EBX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[1], uint32(0xCCCC4444); got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[2], uint32(0xDDDD3333); got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectMOVZXRegisters(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x220)
	copy(mem[pc:], []byte{0x0F, 0xB6, 0xC4, 0x0F, 0xB7, 0xCB, 0x0F, 0xB6, 0xD3}) // MOVZX EAX,AH; MOVZX ECX,BX; MOVZX EDX,BL
	instrs := x86ScanBlock(mem, pc)
	instrs = instrs[:3]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0x12ABCD00, 0xFFFF1234
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xCD); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[1], uint32(0x1234); got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[2], uint32(0x34); got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectMOVSXRegisters(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x240)
	copy(mem[pc:], []byte{0x0F, 0xBE, 0xC4, 0x0F, 0xBF, 0xCB, 0x0F, 0xBE, 0xD3}) // MOVSX EAX,AH; MOVSX ECX,BX; MOVSX EDX,BL
	instrs := x86ScanBlock(mem, pc)
	instrs = instrs[:3]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[0], cpu.jitRegs[3] = 0x00008000, 0x00008080
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0xFFFFFF80); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[1], uint32(0xFFFF8080); got != want {
		t.Fatalf("ECX = %08X, want %08X", got, want)
	}
	if got, want := cpu.jitRegs[2], uint32(0xFFFFFF80); got != want {
		t.Fatalf("EDX = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_DirectFlagBitInstructions(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x260)
	copy(mem[pc:], []byte{0xFC, 0xFD, 0xF8, 0xF9, 0xF5, 0xF4}) // CLD; STD; CLC; STC; CMC
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem, Flags: x86FlagCF | 0x40}
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if cpu.Flags&x86FlagCF != 0 {
		t.Fatalf("CF remained set: %08X", cpu.Flags)
	}
	if cpu.Flags&x86FlagDF == 0 {
		t.Fatalf("DF not set: %08X", cpu.Flags)
	}
	if cpu.Flags&0x40 == 0 {
		t.Fatalf("unrelated flag lost: %08X", cpu.Flags)
	}
}

func TestX86ARM64_DirectCLIAndSTI(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x280)
	copy(mem[pc:], []byte{0xFA, 0xFB})
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	ctx := newX86JITContext(cpu, nil, nil)
	// CLI and STI are block terminators: the dispatcher must observe IF before
	// admitting the following instruction. Execute their two native blocks.
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	block, err = x86CompileBlockForCPU(cpu, x86ScanBlock(mem, pc+1), pc+1, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if cpu.Flags&x86FlagIF == 0 {
		t.Fatalf("IF not set after STI: %08X", cpu.Flags)
	}
}

func TestX86ARM64_DirectLAHFSAHF(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x2A0)
	copy(mem[pc:], []byte{0x9F, 0x9E, 0xF4}) // LAHF; SAHF
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem, Flags: 0xA5000043}
	cpu.jitRegs[0] = 0x11220044
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.jitRegs[0], uint32(0x11224344); got != want {
		t.Fatalf("EAX = %08X, want %08X", got, want)
	}
	if got, want := cpu.Flags, uint32(0xA5000043); got != want {
		t.Fatalf("Flags = %08X, want %08X", got, want)
	}
}

func TestX86ARM64_RegisterFPUHelperPublishesDecodedPayload(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x180)
	copy(mem[pc:], []byte{0xD9, 0xF0}) // F2XM1, canonical helper form
	instrs := x86ScanBlock(mem, pc)
	instrs = instrs[:1]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem, CS: 0x1234}
	cpu.syncJITSegRegsFromNamed()
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	p, ok := x86FPUHelperPayloadFromContext(ctx)
	if !ok {
		t.Fatalf("helper context is not decodable: reason=%d escape=%02X length=%d", ctx.ExitReason, ctx.FPUHelperEscape, ctx.FPUHelperLength)
	}
	if got, want := p.InstrPC, pc; got != want {
		t.Fatalf("payload PC = %08X, want %08X", got, want)
	}
	if got, want := p.CS, uint16(0x1234); got != want {
		t.Fatalf("payload CS = %04X, want %04X", got, want)
	}
	if got, want := p.Bytes[:2], []byte{0xD9, 0xF0}; string(got) != string(want) {
		t.Fatalf("payload bytes = % X, want % X", got, want)
	}
	if ctx.RetCount != 0 || ctx.RetPC != pc {
		t.Fatalf("helper return = pc=%08X count=%d, want pc=%08X count=0", ctx.RetPC, ctx.RetCount, pc)
	}
}

func TestX86ARM64_Disp32FPUHelperPublishesMemoryProvenance(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x200)
	copy(mem[pc:], []byte{0xD8, 0x05, 0x00, 0x0F, 0x00, 0x00}) // FADD dword [0xF00]
	instrs := x86ScanBlock(mem, pc)
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.FPUHelperEA, uint32(0xF00); got != want {
		t.Fatalf("helper EA = %08X, want %08X", got, want)
	}
	if got, want := ctx.FPUHelperWidth, uint32(4); got != want {
		t.Fatalf("helper width = %d, want %d", got, want)
	}
}

func TestX86ARM64_FPUHelperPublishesDynamicMemoryProvenance(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x240)
	// FADD dword [EBP+ECX*4-16]. The default segment is SS because EBP is
	// the SIB base, and the live EA must be captured before helper replay.
	copy(mem[pc:], []byte{0xD8, 0x44, 0x8D, 0xF0})
	instrs := x86ScanBlock(mem, pc)[:1]
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	cpu := &CPU_X86{memory: mem}
	cpu.jitRegs[5], cpu.jitRegs[1] = 0x1000, 3
	ctx := newX86JITContext(cpu, nil, nil)
	block, err := x86CompileBlockForCPU(cpu, instrs, pc, em)
	if err != nil {
		t.Fatal(err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.FPUHelperEA, uint32(0xFFC); got != want {
		t.Fatalf("helper EA = %08X, want %08X", got, want)
	}
	if got, want := ctx.FPUHelperSegment, byte(x86SegSS); got != want {
		t.Fatalf("helper segment = %d, want SS (%d)", got, want)
	}
}
