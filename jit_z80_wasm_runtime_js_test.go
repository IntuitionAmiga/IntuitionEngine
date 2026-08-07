//go:build js && wasm

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

func TestZ80WasmFrontendRejectsHALTForDispatcher(t *testing.T) {
	payload := z80CanonicalPayloadFromBytes(0, [4]byte{0x76})
	if z80WasmFrontendAdmits(payload) {
		t.Fatal("HALT admitted as a wasm instruction instead of a dispatcher outcome")
	}
}

func TestZ80WasmFrontendStopsBeforeHALTAfterEI(t *testing.T) {
	mem := []byte{0xED, 0x56, 0xFB, 0x76, 0xCD, 0x00, 0x40}
	got := z80FrontendScanBlock(func(pc uint16) byte { return mem[pc] }, func(uint16) bool { return true }, z80WasmFrontendAdmits, 0)
	if len(got) != 2 || got[1].Opcode != 0xFB || got[1].ResumePC != 3 {
		t.Fatalf("frontend block = %+v, want IM 1 then EI ending at HALT PC 3", got)
	}
}

type z80WasmFrozenBus struct {
	mem  [0x10000]byte
	port uint16
	data byte
}

func (b *z80WasmFrozenBus) Read(addr uint16) byte         { return b.mem[addr] }
func (b *z80WasmFrozenBus) Write(addr uint16, value byte) { b.mem[addr] = value }
func (b *z80WasmFrozenBus) In(uint16) byte                { return 0 }
func (b *z80WasmFrozenBus) Out(port uint16, value byte)   { b.port, b.data = port, value }
func (b *z80WasmFrozenBus) Tick(int)                      {}

func TestWasmJIT_Z80CacheRejectsPhysicalAndMappingChanges(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	bus.Write8(0, 0x00) // NOP
	rt := newZ80WasmRuntime(cpu, adapter)
	if rt == nil {
		t.Fatal("no wasm runtime")
	}
	defer rt.unreg()
	block := rt.compile(0)
	if block == nil {
		t.Fatal("NOP did not compile")
	}
	rt.cache[0] = block
	cpu.jitCodeGeneration[0].Add(1)
	if rt.sourceMatches(0, block) {
		t.Fatal("physical generation snapshot retained cached module")
	}
	// Restore the cache from a fresh module before exercising the publisher.
	block = rt.compile(0)
	rt.cache[0] = block
	bus.Write8(0, 0x3E) // physical publisher must invalidate the owned cache
	rt.drainPhysicalWrites()
	if len(rt.cache) != 0 {
		t.Fatal("physical write retained cached module")
	}
	block = rt.compile(0)
	if block == nil {
		t.Fatal("changed source did not compile")
	}
	adapter.mappingGeneration.Add(1)
	if rt.sourceMatches(0, block) {
		t.Fatal("mapping generation mismatch retained cached module")
	}
}

func TestWasmJIT_Z80CompileMarksCodePagesForSelfModification(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	initZ80WasmDirectPages(cpu, adapter)
	// LD ($0000),A writes directly over its own compiled source page.
	bus.Write8(0, 0x32)
	bus.Write8(1, 0x00)
	bus.Write8(2, 0x00)
	rt := newZ80WasmRuntime(cpu, adapter)
	if rt == nil {
		t.Fatal("no wasm runtime")
	}
	defer rt.unreg()
	block := rt.compile(0)
	if block == nil {
		t.Fatal("self-store did not compile")
	}
	if cpu.codePageBitmap[0] == 0 {
		t.Fatal("compiled source page was not marked as code")
	}
	if !rt.invoke(block) {
		t.Fatal("self-store module invocation failed")
	}
	if got := binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffNeedInval:]); got != 1 {
		t.Fatalf("NeedInval = %d, want 1 for direct write to compiled source", got)
	}
}

func TestWasmJIT_Z80MappedMMIOCodePageIsNotDirect(t *testing.T) {
	bus := NewMachineBus()
	// $C000 is normally direct Z80 RAM. Give it a fetch-side-effecting MMIO
	// handler and seal before deriving the wasm direct bitmap.
	bus.MapIO(0xC000, 0xC0FF, func(uint32) uint32 { return 0 }, nil)
	bus.SealMappings()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	initZ80WasmDirectPages(cpu, adapter)
	if got := cpu.directPageBitmap[0xC0]; got == 0 {
		t.Fatal("mapped MMIO code page C000 remained direct for wasm JIT")
	}
}

func TestWasmJIT_Z80HALTWakesForDelayedInterrupt(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	for _, test := range []struct {
		name   string
		vector uint16
		setup  func(*CPU_Z80)
	}{
		{
			name:   "IRQ",
			vector: 0x0038,
			setup: func(cpu *CPU_Z80) {
				cpu.IFF1 = true
				cpu.IFF2 = true
				cpu.SetIRQLine(true)
			},
		},
		{
			name:   "NMI",
			vector: 0x0066,
			setup: func(cpu *CPU_Z80) {
				cpu.SetNMILine(true)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			bus.Write8(0, 0x76) // HALT
			bus.Write8(uint32(test.vector), 0x18)
			bus.Write8(uint32(test.vector+1), 0xFE) // JR to the interrupt vector
			cpu.SP = 0xFFFE
			cpu.SetRunning(true)
			done := make(chan struct{})
			go func() {
				cpu.ExecuteJITZ80()
				close(done)
			}()
			t.Cleanup(func() {
				cpu.SetRunning(false)
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("wasm Z80 execution loop did not stop")
				}
			})

			deadline := time.Now().Add(time.Second)
			for !cpu.Halted && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if !cpu.Halted {
				t.Fatal("wasm Z80 did not enter HALT")
			}

			test.setup(cpu)
			deadline = time.Now().Add(time.Second)
			for (cpu.Halted || cpu.PC != test.vector) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if cpu.Halted || cpu.PC != test.vector {
				t.Fatalf("delayed %s did not wake HALT: PC=%04X halted=%v", test.name, cpu.PC, cpu.Halted)
			}
		})
	}
}

func TestWasmJIT_Z80CanonicalHelperUsesFrozenBytes(t *testing.T) {
	bus := &z80WasmFrozenBus{}
	bus.mem[0], bus.mem[1] = 0xD3, 0x12 // OUT ($12),A
	cpu := NewCPU_Z80(bus)
	cpu.A = 0x7A
	cpu.SetRunning(true)
	payload := z80CanonicalHelperPayload{StartPC: 0, Opcode: 0xD3, Length: 4, Bytes: [4]byte{0xD3, 0x12}}
	// The instruction stream changes after capture. A helper re-decoding it
	// would address a different port.
	bus.mem[1] = 0x34
	cpu.executeZ80CanonicalHelper(payload)
	if bus.port != 0x7A12 || bus.data != 0x7A {
		t.Fatalf("OUT observed port=%04X data=%02X, want 7A12/7A", bus.port, bus.data)
	}
}

func z80WasmManifestCPU(source []byte) (*z80WasmFrozenBus, *CPU_Z80) {
	bus := &z80WasmFrozenBus{}
	copy(bus.mem[:], source)
	bus.mem[0x0200], bus.mem[0x0201] = 0x5A, 0xA5
	bus.mem[0x0300], bus.mem[0x0301] = 0xC3, 0x3C
	cpu := NewCPU_Z80(bus)
	cpu.SP = 0x1FFE
	cpu.SetBC(0x0400)
	cpu.SetDE(0x0300)
	cpu.SetHL(0x0200)
	cpu.IX, cpu.IY = 0x0200, 0x0300
	cpu.A, cpu.F = 0x5A, z80FlagC
	cpu.SetRunning(true)
	return bus, cpu
}

func z80WasmManifestEqual(a, b *CPU_Z80) bool {
	return a.A == b.A && a.F == b.F && a.B == b.B && a.C == b.C && a.D == b.D && a.E == b.E && a.H == b.H && a.L == b.L &&
		a.A2 == b.A2 && a.F2 == b.F2 && a.B2 == b.B2 && a.C2 == b.C2 && a.D2 == b.D2 && a.E2 == b.E2 && a.H2 == b.H2 && a.L2 == b.L2 &&
		a.IX == b.IX && a.IY == b.IY && a.SP == b.SP && a.PC == b.PC && a.I == b.I && a.R == b.R && a.IM == b.IM && a.WZ == b.WZ &&
		a.IFF1 == b.IFF1 && a.IFF2 == b.IFF2 && a.Halted == b.Halted && a.Running() == b.Running() && a.Cycles == b.Cycles && a.iffDelay == b.iffDelay
}

// TestWasmJIT_Z80ManifestFrozenHelper executes the same immutable helper as
// the browser dispatcher for every decoded family. It closes the gap between
// the platform-neutral manifest oracle and the js/wasm runtime.
func TestWasmJIT_Z80ManifestFrozenHelper(t *testing.T) {
	families := []struct {
		name   string
		source func(byte) []byte
	}{
		{"base", func(op byte) []byte { return []byte{op, 0x5A, 0xA5, 0x76} }},
		{"cb", func(op byte) []byte { return []byte{0xCB, op, 0x5A, 0xA5} }},
		{"ed", func(op byte) []byte { return []byte{0xED, op, 0x5A, 0xA5} }},
		{"dd", func(op byte) []byte { return []byte{0xDD, op, 0x5A, 0xA5} }},
		{"fd", func(op byte) []byte { return []byte{0xFD, op, 0x5A, 0xA5} }},
		{"ddcb", func(op byte) []byte { return []byte{0xDD, 0xCB, 0x5A, op} }},
		{"fdcb", func(op byte) []byte { return []byte{0xFD, 0xCB, 0x5A, op} }},
	}
	for _, family := range families {
		for opcode := 0; opcode < 256; opcode++ {
			source := family.source(byte(opcode))
			interpBus, interp := z80WasmManifestCPU(source)
			helperBus, helper := z80WasmManifestCPU(source)
			interp.Step()
			payload := z80CanonicalHelperPayload{StartPC: 0, Opcode: source[0], Length: 4, Bytes: [4]byte{source[0], source[1], source[2], source[3]}}
			helper.executeZ80CanonicalHelper(payload)
			if !z80WasmManifestEqual(interp, helper) || !bytes.Equal(interpBus.mem[:], helperBus.mem[:]) || interpBus.port != helperBus.port || interpBus.data != helperBus.data {
				t.Fatalf("%s:%02X helper mismatch", family.name, opcode)
			}
		}
	}
}

func TestWasmJIT_Z80ManifestNativeDifferential(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	variants := []struct {
		name                   string
		a, f, i, r             byte
		bc, de, hl, ix, iy, sp uint16
		iff1, iff2             bool
	}{
		{"baseline", 0x5A, z80FlagC, 0x00, 0x00, 0x1200, 0x1300, 0x1000, 0x1000, 0x1100, 0x1FFE, false, false},
		{"conditions-set", 0x80, z80FlagS | z80FlagZ | z80FlagPV, 0x7F, 0x7E, 0x1201, 0x1301, 0x1001, 0x10FF, 0x11FF, 0x1FFC, true, true},
		{"carry-half-subtract", 0xFF, z80FlagC | z80FlagH | z80FlagN, 0x80, 0xFE, 0x1200, 0x1300, 0x1000, 0x1001, 0x1101, 0x1FFA, false, true},
	}
	var compileFailures, stateFailures []string
	for _, manifestRow := range z80WasmCanonicalDirectRows() {
		for _, variant := range variants {
			row, variant := manifestRow, variant
			t.Run(row.Name+"/"+variant.name, func(t *testing.T) {
				source := append(append([]byte(nil), row.Payload.Bytes[:row.Payload.Length]...), 0xD3, 0x00)
				interpBus, interp := z80WasmManifestCPUAt(source, variant.a, variant.f, variant.i, variant.r, variant.bc, variant.de, variant.hl, variant.ix, variant.iy, variant.sp, variant.iff1, variant.iff2)
				interp.Step()

				bus := NewMachineBus()
				adapter := NewZ80BusAdapter(bus)
				cpu := NewCPU_Z80(adapter)
				mem := bus.GetMemory()
				copy(mem[0x0100:], source)
				mem[0x1000], mem[0x1001] = 0x5A, 0xA5
				mem[0x1100], mem[0x1101] = 0xC3, 0x3C
				applyZ80WasmManifestVariant(cpu, variant.a, variant.f, variant.i, variant.r, variant.bc, variant.de, variant.hl, variant.ix, variant.iy, variant.sp, variant.iff1, variant.iff2)
				bus.SealMappings()
				initZ80WasmDirectPages(cpu, adapter)
				rt := newZ80WasmRuntime(cpu, adapter)
				if rt == nil {
					t.Fatal("no wasm runtime")
				}
				defer rt.unreg()
				block := rt.compile(0x0100)
				if block == nil {
					compileFailures = append(compileFailures, row.Name+"/"+variant.name)
					return
				}
				if !rt.invoke(block) {
					t.Fatal("wasm module invocation failed")
				}
				if binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffNeedBail:]) != 0 {
					compileFailures = append(compileFailures, row.Name+"/"+variant.name+":bail")
					return
				}
				cpu.PC = uint16(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRetPC:]))
				cpu.Cycles += binary.LittleEndian.Uint64(rt.ctx[z80WasmCtxOffRetCycles:])
				cpu.R = (cpu.R & 0x80) | ((cpu.R + byte(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRIncrements:]))) & 0x7F)
				if !z80WasmManifestEqual(interp, cpu) || !bytes.Equal(interpBus.mem[:], mem[:0x10000]) {
					stateFailures = append(stateFailures, fmt.Sprintf("%s/%s(%s <> %s mem=%s)", row.Name, variant.name, z80WasmState(interp), z80WasmState(cpu), z80WasmFirstMemoryDifference(interpBus.mem[:], mem[:0x10000])))
				}
			})
		}
	}
	if len(compileFailures) != 0 || len(stateFailures) != 0 {
		t.Fatalf("wasm manifest differential failures: compile=%d %v; state=%d %v", len(compileFailures), boundedZ80FailureList(compileFailures, 80), len(stateFailures), boundedZ80FailureList(stateFailures, 30))
	}
}

func z80WasmState(cpu *CPU_Z80) string {
	return fmt.Sprintf("pc=%04X af=%04X bc=%04X de=%04X hl=%04X ix=%04X iy=%04X sp=%04X wz=%04X i=%02X r=%02X im=%d iff=%t/%t d=%d cy=%d h=%t", cpu.PC, cpu.AF(), cpu.BC(), cpu.DE(), cpu.HL(), cpu.IX, cpu.IY, cpu.SP, cpu.WZ, cpu.I, cpu.R, cpu.IM, cpu.IFF1, cpu.IFF2, cpu.iffDelay, cpu.Cycles, cpu.Halted)
}

func z80WasmFirstMemoryDifference(a, b []byte) string {
	for i := range a {
		if a[i] != b[i] {
			return fmt.Sprintf("%04X:%02X/%02X", i, a[i], b[i])
		}
	}
	return "equal"
}

type z80WasmCanonicalRow struct {
	Name    string
	Payload z80CanonicalHelperPayload
}

func z80WasmCanonicalDirectRows() []z80WasmCanonicalRow {
	manifest := z80JITOpcodeManifest()
	rows := make([]z80WasmCanonicalRow, 0, len(manifest))
	for _, row := range manifest {
		if row.WasmOutcome != z80JITOutcomeDirect {
			continue
		}
		source := z80ManifestSourceBytes(row.Instr)
		var image [4]byte
		copy(image[:], source)
		payload := z80CanonicalPayloadFromBytes(0x0100, image)
		if payload.Length == 0 {
			panic("canonical manifest contains an undecodable wasm row: " + row.Name)
		}
		rows = append(rows, z80WasmCanonicalRow{Name: row.Name, Payload: payload})
	}
	return rows
}

func z80WasmCanonicalObservation(payload z80CanonicalHelperPayload) bool {
	if payload.Prefix == 0 {
		return payload.Opcode == 0xD3 || payload.Opcode == 0xDB
	}
	if payload.Prefix == z80JITPrefixDD || payload.Prefix == z80JITPrefixFD {
		return payload.Bytes[1] == 0xD3 || payload.Bytes[1] == 0xDB
	}
	if payload.Prefix != z80JITPrefixED {
		return false
	}
	switch payload.Opcode {
	case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x70, 0x78,
		0x41, 0x49, 0x51, 0x59, 0x61, 0x69, 0x71, 0x79,
		0xA2, 0xAA, 0xB2, 0xBA, 0xA3, 0xAB, 0xB3, 0xBB:
		return true
	default:
		return false
	}
}

func boundedZ80FailureList(failures []string, limit int) []string {
	if len(failures) <= limit {
		return failures
	}
	return append(append([]string(nil), failures[:limit]...), fmt.Sprintf("... %d more", len(failures)-limit))
}

func z80WasmManifestCPUAt(source []byte, a, f, i, r byte, bc, de, hl, ix, iy, sp uint16, iff1, iff2 bool) (*z80WasmFrozenBus, *CPU_Z80) {
	bus := &z80WasmFrozenBus{}
	copy(bus.mem[0x0100:], source)
	bus.mem[0x1000], bus.mem[0x1001] = 0x5A, 0xA5
	bus.mem[0x1100], bus.mem[0x1101] = 0xC3, 0x3C
	cpu := NewCPU_Z80(bus)
	applyZ80WasmManifestVariant(cpu, a, f, i, r, bc, de, hl, ix, iy, sp, iff1, iff2)
	return bus, cpu
}

func applyZ80WasmManifestVariant(cpu *CPU_Z80, a, f, i, r byte, bc, de, hl, ix, iy, sp uint16, iff1, iff2 bool) {
	cpu.PC, cpu.SP = 0x0100, sp
	cpu.SetBC(bc)
	cpu.SetDE(de)
	cpu.SetHL(hl)
	cpu.IX, cpu.IY = ix, iy
	cpu.A, cpu.F, cpu.I, cpu.R = a, f, i, r
	cpu.IFF1, cpu.IFF2 = iff1, iff2
	cpu.SetRunning(true)
}

func z80WasmRunProgram(t *testing.T, program []byte, jit bool) *CPU_Z80 {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	for address, value := range program {
		bus.Write8(uint32(address), value)
	}
	cpu.jitEnabled = jit
	cpu.SetRunning(true)
	if jit {
		// Production execution polls HALT until an interrupt arrives. These
		// finite parity programs instead stop at the first halted boundary so
		// their cycle counts remain deterministic.
		cpu.debugBreakIn = func(uint64) bool { return cpu.Halted }
		cpu.ExecuteJITZ80()
		cpu.SetRunning(true)
	} else {
		for steps := 0; steps < 100 && !cpu.Halted; steps++ {
			cpu.Step()
		}
	}
	if !cpu.Halted {
		t.Fatalf("program did not halt (jit=%v PC=%04X)", jit, cpu.PC)
	}
	return cpu
}

func TestWasmJIT_Z80DirectRegisterParity(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	program := []byte{0x3E, 0x42, 0x47, 0x00, 0x76} // LD A,42; LD B,A; NOP; HALT
	interp := z80WasmRunProgram(t, program, false)
	jit := z80WasmRunProgram(t, program, true)
	if jit.A != interp.A || jit.B != interp.B || jit.PC != interp.PC || jit.R != interp.R || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT state A=%02X B=%02X PC=%04X R=%02X cycles=%d, interpreter A=%02X B=%02X PC=%04X R=%02X cycles=%d",
			jit.A, jit.B, jit.PC, jit.R, jit.Cycles, interp.A, interp.B, interp.PC, interp.R, interp.Cycles)
	}
	if got := jit.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("browser path did not execute an emitted Z80 wasm block")
	}
}

func TestWasmJIT_Z80RetiredAccountingIncludesCanonicalHelpers(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	for address, value := range []byte{0x3E, 0x5A, 0xD3, 0x10, 0x76} { // LD A,$5A; OUT ($10),A; HALT
		bus.Write8(uint32(address), value)
	}
	cpu.PerfEnabled = true
	cpu.debugBreakIn = func(uint64) bool { return cpu.Halted }
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got, want := cpu.InstructionCount, uint64(3); got != want {
		t.Fatalf("retired instructions = %d, want %d", got, want)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 || cpu.jitStats.helperExits.Load() == 0 {
		t.Fatalf("expected direct and helper execution: native=%d helper=%d", cpu.jitStats.nativeEntries.Load(), cpu.jitStats.helperExits.Load())
	}
}

func TestWasmJIT_Z80StaticRegionPromotionParity(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	// JP $0003; JP $0006; LD A,$5A; HALT. The two static edges must be
	// precompiled through the shared frontend before the first module runs.
	program := []byte{0xC3, 0x03, 0x00, 0xC3, 0x06, 0x00, 0x3E, 0x5A, 0x76}
	interp := z80WasmRunProgram(t, program, false)
	jit := z80WasmRunProgram(t, program, true)
	if !z80WasmManifestEqual(interp, jit) {
		t.Fatalf("static region state mismatch: interp=%+v jit=%+v", interp, jit)
	}
	if jit.jitStats.regionPromotions.Load() == 0 {
		t.Fatal("wasm static JP chain was not promoted")
	}
}

func TestWasmJIT_Z80DiagnosticBackend(t *testing.T) {
	if got := z80JITBackend(); got != "wasm" {
		t.Fatalf("backend = %q, want wasm", got)
	}
}

func TestWasmJIT_Z80IEScriptStatsBackend(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	compositor := NewVideoCompositor(nil)
	engine := NewScriptEngine(bus, compositor, term)
	runner := NewCPUZ80Runner(bus, CPUZ80Config{})
	runner.cpu.SetRunning(false)
	runtimeStatus.setCPUs(runtimeCPUZ80, nil, nil, nil, runner, nil, nil)
	t.Cleanup(func() { runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil) })
	if err := engine.RunString(`local stats = cpu.jit_stats(); if stats.backend ~= "wasm" then error(stats.backend) end`, "z80_wasm_stats"); err != nil {
		t.Fatal(err)
	}
	waitScriptStopped(t, engine)
	if err := engine.LastError(); err != nil {
		t.Fatal(err)
	}
}

func TestWasmJIT_Z80ConstructionAndIEScriptControl(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	bus := NewMachineBus()
	if !NewCPU_Z80(NewZ80BusAdapter(bus)).jitEnabled {
		t.Fatal("direct Z80 constructor did not enable the wasm backend")
	}
	if !NewCPUZ80Runner(bus, CPUZ80Config{}).JITEnabled {
		t.Fatal("Z80 runner did not enable the wasm backend")
	}
	if NewCPUZ80Runner(bus, CPUZ80Config{DisableJIT: true}).JITEnabled {
		t.Fatal("DisableJIT did not select the interpreter")
	}
	term := NewTerminalMMIO()
	compositor := NewVideoCompositor(nil)
	engine := NewScriptEngine(bus, compositor, term)
	runner := NewCPUZ80Runner(bus, CPUZ80Config{})
	runner.cpu.SetRunning(false)
	runtimeStatus.setCPUs(runtimeCPUZ80, nil, nil, nil, runner, nil, nil)
	t.Cleanup(func() { runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil) })
	if err := engine.RunString(`
		if not cpu.jit_enabled() then error("default") end
		cpu.set_jit_enabled(false)
		if cpu.jit_enabled() or cpu.execution_mode() ~= "interpreter" then error("disable") end
		cpu.set_jit_enabled(true)
		if not cpu.jit_enabled() or cpu.execution_mode() ~= "jit" then error("enable") end
	`, "z80_wasm_jit_control"); err != nil {
		t.Fatal(err)
	}
	waitScriptStopped(t, engine)
	if err := engine.LastError(); err != nil {
		t.Fatal(err)
	}
	runner.cpu.SetRunning(true)
	if err := engine.RunString(`cpu.set_jit_enabled(false)`, "z80_wasm_jit_running"); err != nil {
		t.Fatal(err)
	}
	waitScriptStopped(t, engine)
	if err := engine.LastError(); err == nil {
		t.Fatal("running Z80 accepted a JIT state change")
	}
	runner.cpu.SetRunning(false)
}

func TestWasmJIT_Z80CoprocessorWorkerExecutesModule(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	worker, err := createZ80Worker(NewMachineBus(), []byte{
		0x3E, 0x5A, // LD A,$5A
		0x18, 0xFC, // JR $0000
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cpu := worker.debugCPU.(*DebugZ80).cpu
	t.Cleanup(func() {
		worker.stopCPU()
		select {
		case <-worker.done:
		case <-time.After(time.Second):
			t.Error("Z80 wasm worker did not stop")
		}
	})
	deadline := time.Now().Add(time.Second)
	for cpu.jitStats.nativeEntries.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("coprocessor worker did not execute a Z80 wasm module")
	}
}

func TestWasmJIT_Z80CanonicalHelperParity(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	// OUT is deliberately not emitted. It must execute after LD A,n through a
	// frozen helper rather than re-decoding the mutable code stream.
	program := []byte{0x3E, 0x7A, 0xD3, 0x12, 0x76}
	interp := z80WasmRunProgram(t, program, false)
	jit := z80WasmRunProgram(t, program, true)
	if jit.A != interp.A || jit.PC != interp.PC || jit.R != interp.R || jit.Cycles != interp.Cycles {
		t.Fatalf("helper state differs: JIT PC=%04X R=%02X cycles=%d, interpreter PC=%04X R=%02X cycles=%d",
			jit.PC, jit.R, jit.Cycles, interp.PC, interp.R, interp.Cycles)
	}
	if got := jit.jitStats.helperExits.Load(); got == 0 {
		t.Fatal("browser path did not execute a canonical helper")
	}
}

func TestWasmJIT_Z80ManifestCanonicalHelperDispatchDifferential(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if row.WasmOutcome != z80JITOutcomeCanonicalHelper {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			program := append(z80ManifestSourceBytes(row.Instr), 0x76)
			run := func(jit bool) (*CPU_Z80, []byte) {
				bus := NewMachineBus()
				adapter := NewZ80BusAdapter(bus)
				cpu := NewCPU_Z80(adapter)
				copy(bus.GetMemory(), program)
				cpu.SP, cpu.A = 0x1FFE, 0x5A
				cpu.SetBC(0x0100)
				cpu.SetDE(0x0300)
				cpu.SetHL(0x0200)
				cpu.jitEnabled = jit
				cpu.SetRunning(true)
				if jit {
					cpu.debugBreakIn = func(uint64) bool { return cpu.Halted }
					cpu.ExecuteJITZ80()
					cpu.SetRunning(true)
				} else {
					for steps := 0; steps < 300 && !cpu.Halted; steps++ {
						cpu.Step()
					}
				}
				if !cpu.Halted {
					t.Fatalf("helper program did not halt (jit=%v PC=%04X)", jit, cpu.PC)
				}
				return cpu, append([]byte(nil), bus.GetMemory()...)
			}
			interp, interpMem := run(false)
			wasm, wasmMem := run(true)
			if !z80WasmManifestEqual(interp, wasm) || !bytes.Equal(interpMem, wasmMem) {
				t.Fatalf("wasm helper mismatch: interp=%s wasm=%s", z80WasmState(interp), z80WasmState(wasm))
			}
			if got := wasm.jitStats.helperExits.Load(); got == 0 {
				t.Fatal("canonical helper outcome was not observed")
			}
		})
	}
}

func TestWasmJIT_Z80ManifestHaltContract(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT capability is unavailable")
	}
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if row.WasmOutcome != z80JITOutcomeHalt {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			run := func(jit bool) (*CPU_Z80, []byte) {
				bus := NewMachineBus()
				adapter := NewZ80BusAdapter(bus)
				cpu := NewCPU_Z80(adapter)
				copy(bus.GetMemory(), z80ManifestSourceBytes(row.Instr))
				bus.GetMemory()[0x0038] = 0x76
				cpu.SP, cpu.IFF1, cpu.IFF2, cpu.IM = 0x1FFE, true, true, 1
				cpu.jitEnabled = jit
				cpu.SetRunning(true)
				if jit {
					cpu.debugBreakIn = func(uint64) bool { return cpu.Halted }
					cpu.ExecuteJITZ80()
					cpu.SetRunning(true)
				} else {
					cpu.Step()
				}
				if !cpu.Halted {
					t.Fatal("HALT did not enter stopped state")
				}
				cpu.SetIRQLine(true)
				cpu.SetRunning(true)
				if jit {
					cpu.debugBreakIn = func(uint64) bool { return cpu.Halted && cpu.PC == 0x0039 }
					cpu.ExecuteJITZ80()
					cpu.SetRunning(true)
				} else {
					cpu.Step()
					cpu.Step()
				}
				if !cpu.Halted || cpu.PC != 0x0039 || cpu.IFF1 || cpu.IFF2 {
					t.Fatalf("interrupt wake contract: halted=%v PC=%04X IFF=%v/%v", cpu.Halted, cpu.PC, cpu.IFF1, cpu.IFF2)
				}
				return cpu, append([]byte(nil), bus.GetMemory()...)
			}
			interp, interpMem := run(false)
			wasm, wasmMem := run(true)
			if !z80WasmManifestEqual(interp, wasm) || !bytes.Equal(interpMem, wasmMem) {
				t.Fatalf("wasm HALT mismatch: interp=%s wasm=%s", z80WasmState(interp), z80WasmState(wasm))
			}
		})
	}
}
