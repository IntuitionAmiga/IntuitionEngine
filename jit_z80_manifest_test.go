//go:build (amd64 || arm64) && linux

package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestZ80JIT_ManifestDifferential(t *testing.T) {
	for _, row := range z80JITOpcodeManifest() {
		row := row
		t.Run(row.Name, func(t *testing.T) {
			for backend, outcome := range map[string]z80JITOpcodeOutcome{"amd64": row.AMD64Outcome, "arm64": row.ARM64Outcome, "wasm": row.WasmOutcome} {
				if outcome == z80JITOutcomeUnclassified {
					t.Fatalf("%s outcome is unclassified", backend)
				}
				if outcome == z80JITOutcomeHalt && !row.CPUHalts {
					t.Fatalf("%s declares a non-halting form as halt", backend)
				}
			}
			source := z80BlockSourceBytes([]JITZ80Instr{row.Instr})
			interpBus, interp := newZ80ManifestCPU(source)
			helperBus, helper := newZ80ManifestCPU(source)

			interp.Step()
			payload, ok := z80CanonicalHelperPayloadAt(helperBus.mem[:], 0x0100)
			if !ok {
				t.Fatal("canonical helper could not decode manifest source")
			}
			helper.executeZ80CanonicalHelper(payload)

			if !z80ManifestCPUEqual(interp, helper) || !bytes.Equal(interpBus.mem[:], helperBus.mem[:]) || !bytes.Equal(interpBus.io[:], helperBus.io[:]) {
				t.Fatalf("interpreter/helper mismatch\ninterp: %s\nhelper: %s", z80ManifestCPUState(interp), z80ManifestCPUState(helper))
			}
		})
	}
}

// Every helper declaration must be observed through the production dispatcher
// on both Linux native backends. Calling executeZ80CanonicalHelper directly is
// only an oracle check and cannot prove that JIT dispatch selected the helper.
func TestZ80JIT_ManifestCanonicalHelperDispatchDifferential(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 native JIT unavailable")
	}
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if z80ManifestCurrentOutcome(row) != z80JITOutcomeCanonicalHelper {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			source := append(z80ManifestSourceBytes(row.Instr), 0x76)
			run := func(jit bool) (*CPU_Z80, []byte) {
				bus := NewMachineBus()
				adapter := NewZ80BusAdapter(bus)
				cpu := NewCPU_Z80(adapter)
				copy(bus.GetMemory()[0x0100:], source)
				cpu.PC, cpu.SP, cpu.A = 0x0100, 0x1FFE, 0x5A
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
			native, nativeMem := run(true)
			if !z80ManifestCPUEqual(interp, native) || !bytes.Equal(interpMem, nativeMem) {
				t.Fatalf("dispatcher/helper mismatch\ninterp: %s\nnative: %s", z80ManifestCPUState(interp), z80ManifestCPUState(native))
			}
			if got := native.jitStats.helperExits.Load(); got == 0 {
				t.Fatal("canonical helper outcome was not observed")
			}
		})
	}
}

func TestZ80JIT_ManifestHaltContract(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 native JIT unavailable")
	}
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if z80ManifestCurrentOutcome(row) != z80JITOutcomeHalt {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			run := func(jit bool) (*CPU_Z80, []byte) {
				bus := NewMachineBus()
				adapter := NewZ80BusAdapter(bus)
				cpu := NewCPU_Z80(adapter)
				copy(bus.GetMemory()[0x0100:], z80ManifestSourceBytes(row.Instr))
				bus.GetMemory()[0x0038] = 0x76
				cpu.PC, cpu.SP, cpu.IFF1, cpu.IFF2, cpu.IM = 0x0100, 0x1FFE, true, true, 1
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
					cpu.Step() // service IRQ and leave HALT
					cpu.Step() // execute vector HALT
				}
				if !cpu.Halted || cpu.PC != 0x0039 || cpu.IFF1 || cpu.IFF2 {
					t.Fatalf("interrupt wake contract: halted=%v PC=%04X IFF=%v/%v", cpu.Halted, cpu.PC, cpu.IFF1, cpu.IFF2)
				}
				return cpu, append([]byte(nil), bus.GetMemory()...)
			}
			interp, interpMem := run(false)
			native, nativeMem := run(true)
			if !z80ManifestCPUEqual(interp, native) || !bytes.Equal(interpMem, nativeMem) {
				t.Fatalf("HALT contract mismatch\ninterp: %s\nnative: %s", z80ManifestCPUState(interp), z80ManifestCPUState(native))
			}
		})
	}
}

// Every form which CPU_Z80 can execute without crossing a host-observation
// boundary must be admitted by the amd64 emitter. A canonical helper is a
// correctness mechanism for ports, mapped memory and debugger-visible work,
// not a substitute for an omitted native lowering.
func TestAMD64Z80JIT_ManifestAdmitsEveryNonObservationForm(t *testing.T) {
	var missing []string
	for _, row := range z80JITOpcodeManifest() {
		if row.AMD64Outcome == z80JITOutcomeCanonicalHelper && !z80JITNeedsFallback(&row.Instr) {
			missing = append(missing, row.Name)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("amd64 JIT has %d non-observation forms without a native lowering: %v", len(missing), missing)
	}
}

func TestARM64Z80JIT_ManifestAdmitsEveryNonObservationForm(t *testing.T) {
	var missing []string
	for _, row := range z80JITOpcodeManifest() {
		if row.ARM64Outcome == z80JITOutcomeCanonicalHelper && !z80JITNeedsFallback(&row.Instr) {
			missing = append(missing, row.Name)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("arm64 JIT has %d non-observation forms without a native lowering: %v", len(missing), missing)
	}
}

func TestZ80JIT_ManifestPreservesIgnoredIndexPrefixOperands(t *testing.T) {
	for _, instr := range []JITZ80Instr{
		{prefix: z80JITPrefixDD, opcode: 0xC3}, // DD JP nn
		{prefix: z80JITPrefixFD, opcode: 0x3E}, // FD LD A,n
		{prefix: z80JITPrefixDD, opcode: 0x18}, // DD JR e
	} {
		decoded := z80ManifestDecodedInstruction(instr)
		if got, want := len(z80BlockSourceBytes([]JITZ80Instr{decoded})), int(decoded.length); got != want {
			t.Fatalf("%02X:%02X manifest source length = %d, want %d", instr.prefix, instr.opcode, got, want)
		}
	}
}

func TestZ80JIT_ManifestDeclaresStaticJumpsDirectOnAllBackends(t *testing.T) {
	for _, row := range z80JITOpcodeManifest() {
		if row.Instr.prefix != z80JITPrefixNone || (row.Instr.opcode != 0xC3 && row.Instr.opcode != 0x18) {
			continue
		}
		if row.AMD64Outcome != z80JITOutcomeDirect || row.ARM64Outcome != z80JITOutcomeDirect || row.WasmOutcome != z80JITOutcomeDirect {
			t.Fatalf("%s outcomes: amd64=%d arm64=%d wasm=%d, want direct", row.Name, row.AMD64Outcome, row.ARM64Outcome, row.WasmOutcome)
		}
	}
}

func TestZ80JIT_ManifestDeclaresGuardedDirectMemoryForms(t *testing.T) {
	forms := map[byte]string{
		0x02: "LD (BC),A", 0x0A: "LD A,(BC)", 0x12: "LD (DE),A", 0x1A: "LD A,(DE)",
		0x2A: "LD HL,(nn)", 0x32: "LD (nn),A", 0x3A: "LD A,(nn)", 0x36: "LD (HL),n", 0x46: "LD B,(HL)",
		0x70: "LD (HL),B", 0x86: "ADD A,(HL)",
	}
	for _, row := range z80JITOpcodeManifest() {
		name, ok := forms[row.Instr.opcode]
		if !ok || row.Instr.prefix != z80JITPrefixNone {
			continue
		}
		if row.ARM64Outcome != z80JITOutcomeDirect || row.WasmOutcome != z80JITOutcomeDirect {
			t.Fatalf("%s outcomes: arm64=%d wasm=%d, want guarded direct", name, row.ARM64Outcome, row.WasmOutcome)
		}
		delete(forms, row.Instr.opcode)
	}
	for opcode, name := range forms {
		t.Fatalf("manifest missing %s (%02X)", name, opcode)
	}
}

func TestZ80JIT_ManifestDeclaresDirectEDRegisterForms(t *testing.T) {
	forms := map[byte]string{0x44: "NEG", 0x46: "IM 0", 0x47: "LD I,A", 0x56: "IM 1", 0x57: "LD A,I", 0x5E: "IM 2"}
	for _, row := range z80JITOpcodeManifest() {
		name, ok := forms[row.Instr.opcode]
		if !ok || row.Instr.prefix != z80JITPrefixED {
			continue
		}
		if row.ARM64Outcome != z80JITOutcomeDirect || row.WasmOutcome != z80JITOutcomeDirect {
			t.Fatalf("ED %s outcomes: arm64=%d wasm=%d, want direct", name, row.ARM64Outcome, row.WasmOutcome)
		}
		delete(forms, row.Instr.opcode)
	}
	for opcode, name := range forms {
		t.Fatalf("manifest missing ED %s (%02X)", name, opcode)
	}
}

func TestZ80JIT_ManifestDeclaresDirectIgnoredIndexPrefixNOP(t *testing.T) {
	seen := 0
	for _, row := range z80JITOpcodeManifest() {
		if (row.Instr.prefix != z80JITPrefixDD && row.Instr.prefix != z80JITPrefixFD) || row.Instr.opcode != 0x00 {
			continue
		}
		if row.ARM64Outcome != z80JITOutcomeDirect || row.WasmOutcome != z80JITOutcomeDirect {
			t.Fatalf("%02X NOP outcomes: arm64=%d wasm=%d, want direct", row.Instr.prefix, row.ARM64Outcome, row.WasmOutcome)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("ignored-prefix NOP rows = %d, want 2", seen)
	}
}

func TestZ80JIT_ManifestDeclaresStackExchangeDirectOnAllBackends(t *testing.T) {
	want := map[string]bool{"base:E3": false, "dd:E3": false, "fd:E3": false}
	for _, row := range z80JITOpcodeManifest() {
		if _, ok := want[row.Name]; !ok {
			continue
		}
		if row.AMD64Outcome != z80JITOutcomeDirect || row.ARM64Outcome != z80JITOutcomeDirect || row.WasmOutcome != z80JITOutcomeDirect {
			t.Fatalf("%s outcomes: amd64=%d arm64=%d wasm=%d, want direct", row.Name, row.AMD64Outcome, row.ARM64Outcome, row.WasmOutcome)
		}
		want[row.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("manifest missing %s", name)
		}
	}
}

func newZ80ManifestCPU(source []byte) (*z80TestBus, *CPU_Z80) {
	bus := &z80TestBus{}
	copy(bus.mem[0x0100:], source)
	// Keep generic operands and stack accesses away from the code image while
	// making every direct-memory form deterministic.
	bus.mem[0x0200], bus.mem[0x0201] = 0x5A, 0xA5
	bus.mem[0x0300], bus.mem[0x0301] = 0xC3, 0x3C
	cpu := NewCPU_Z80(bus)
	cpu.PC = 0x0100
	cpu.SP = 0x1FFE
	cpu.SetBC(0x0400)
	cpu.SetDE(0x0300)
	cpu.SetHL(0x0200)
	cpu.IX = 0x0200
	cpu.IY = 0x0300
	cpu.A = 0x5A
	cpu.F = z80FlagC
	cpu.SetRunning(true)
	return bus, cpu
}

func z80ManifestCPUEqual(a, b *CPU_Z80) bool {
	return a.A == b.A && a.F == b.F && a.B == b.B && a.C == b.C &&
		a.D == b.D && a.E == b.E && a.H == b.H && a.L == b.L &&
		a.A2 == b.A2 && a.F2 == b.F2 && a.B2 == b.B2 && a.C2 == b.C2 &&
		a.D2 == b.D2 && a.E2 == b.E2 && a.H2 == b.H2 && a.L2 == b.L2 &&
		a.IX == b.IX && a.IY == b.IY && a.SP == b.SP && a.PC == b.PC &&
		a.I == b.I && a.R == b.R && a.IM == b.IM && a.WZ == b.WZ &&
		a.IFF1 == b.IFF1 && a.IFF2 == b.IFF2 && a.Halted == b.Halted &&
		a.Running() == b.Running() && a.Cycles == b.Cycles && a.iffDelay == b.iffDelay
}

func z80ManifestCPUState(cpu *CPU_Z80) string {
	return fmt.Sprintf("pc=%04x af=%04x bc=%04x de=%04x hl=%04x ix=%04x iy=%04x sp=%04x wz=%04x i=%02x r=%02x im=%d iff=%t/%t delay=%d cycles=%d halted=%v",
		cpu.PC, cpu.AF(), cpu.BC(), cpu.DE(), cpu.HL(), cpu.IX, cpu.IY, cpu.SP, cpu.WZ, cpu.I, cpu.R, cpu.IM, cpu.IFF1, cpu.IFF2, cpu.iffDelay, cpu.Cycles, cpu.Halted)
}
