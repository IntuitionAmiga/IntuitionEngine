// jit_x86_shadow_parity_test.go - deterministic shadow-parity harness.
//
// Drives the canonical rotozoomer x86 binary (and at least one
// non-rotozoomer workload to avoid workload-specific accidental parity)
// through the interpreter and the force-native JIT for an identical
// number of guest instructions, snapshotting register state at fixed
// instruction-count checkpoints. Asserts byte-identical canonical CPU and
// x87 state plus guest-memory hashes at every checkpoint.
//
// Coverage limitation (documented):
//   - The current bus has no video chip wired, so workloads that wait
//     on a VBlank/status MMIO never see the flag flip. Both paths spin
//     on the entry MMIO poll under the bounded-budget contract — the
//     tryFastMMIOPollLoop accounting keeps both sides in lock-step
//     state. This validates parity through the poll path but not deep
//     into the workload's compute kernels. Wiring the global videoChip
//     into the test bus (with a deterministic VBlank schedule) is the
//     follow-up that would extend coverage; tracked separately.
//
// Determinism contract:
//   - Both paths driven by guest-instruction count via cpu.x86InstrBudget,
//     not wall-clock.
//   - Identical loaded ROM bytes; identical bus/MMIO state.
//   - Test runs under the headless build tag (no display backend).
//   - The headless bus has no video chip, so display-device output is not a
//     meaningful independent signal. The backing-memory hash still covers
//     every ordinary guest framebuffer write; MMIO/video device hashing needs
//     a deterministic video device fixture and remains separately scoped.
//     Audio output is wall-clock-driven and remains outside this bounded CPU
//     comparison.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// x86CanonicalStateHash returns a SHA-256 over the canonical guest
// register byte image: GP regs in encoding order, segment regs in encoding
// order, EIP, Flags, cycles and complete x87 state. Order is fixed so any
// future field must be appended (never inserted) to keep snapshots stable.
func x86CanonicalStateHash(cpu *CPU_X86) (regBytes [160]byte, sum [32]byte) {
	// GP/segment/EIP/Flags/cycles consume 60 bytes. The physical x87 register
	// file follows, then its complete environment and provenance state.
	binary.LittleEndian.PutUint32(regBytes[0:], cpu.EAX)
	binary.LittleEndian.PutUint32(regBytes[4:], cpu.ECX)
	binary.LittleEndian.PutUint32(regBytes[8:], cpu.EDX)
	binary.LittleEndian.PutUint32(regBytes[12:], cpu.EBX)
	binary.LittleEndian.PutUint32(regBytes[16:], cpu.ESP)
	binary.LittleEndian.PutUint32(regBytes[20:], cpu.EBP)
	binary.LittleEndian.PutUint32(regBytes[24:], cpu.ESI)
	binary.LittleEndian.PutUint32(regBytes[28:], cpu.EDI)
	binary.LittleEndian.PutUint16(regBytes[32:], cpu.ES)
	binary.LittleEndian.PutUint16(regBytes[34:], cpu.CS)
	binary.LittleEndian.PutUint16(regBytes[36:], cpu.SS)
	binary.LittleEndian.PutUint16(regBytes[38:], cpu.DS)
	binary.LittleEndian.PutUint16(regBytes[40:], cpu.FS)
	binary.LittleEndian.PutUint16(regBytes[42:], cpu.GS)
	binary.LittleEndian.PutUint32(regBytes[44:], cpu.EIP)
	binary.LittleEndian.PutUint32(regBytes[48:], cpu.Flags)
	binary.LittleEndian.PutUint64(regBytes[52:], cpu.Cycles)
	if cpu.FPU != nil {
		const fpuOff = 60
		for i, value := range cpu.FPU.regs {
			binary.LittleEndian.PutUint64(regBytes[fpuOff+i*8:], math.Float64bits(value))
		}
		binary.LittleEndian.PutUint16(regBytes[124:], cpu.FPU.FCW)
		binary.LittleEndian.PutUint16(regBytes[126:], cpu.FPU.FSW)
		binary.LittleEndian.PutUint16(regBytes[128:], cpu.FPU.FTW)
		binary.LittleEndian.PutUint32(regBytes[130:], cpu.FPU.FIP)
		binary.LittleEndian.PutUint16(regBytes[134:], cpu.FPU.FCS)
		binary.LittleEndian.PutUint32(regBytes[136:], cpu.FPU.FDP)
		binary.LittleEndian.PutUint16(regBytes[140:], cpu.FPU.FDS)
		binary.LittleEndian.PutUint16(regBytes[142:], cpu.FPU.FOP)
	}
	sum = sha256.Sum256(regBytes[:])
	return
}

func x86GuestMemoryHash(cpu *CPU_X86) [32]byte {
	return sha256.Sum256(cpu.memory)
}

// x86ShadowSetup loads the named ROM into a fresh CPU configured for
// the requested execution mode. forceNative=true selects the JIT path,
// false selects the interpreter.
func x86ShadowSetup(t *testing.T, romPath string, forceNative bool) *CPU_X86 {
	t.Helper()
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	data, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("rom not present (%s): %v", romPath, err)
	}
	var bus *MachineBus
	if filepath.Base(romPath) == "iedoom.ie86" {
		bus, err = NewMachineBusSized(256 * 1024 * 1024)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		bus = NewMachineBus()
	}
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP = 0
	cpu.ESP = 0xFFF0
	cpu.x86JitEnabled = forceNative

	for i, b := range data {
		if uint32(i) >= uint32(len(cpu.memory)) {
			break
		}
		cpu.memory[i] = b
	}
	return cpu
}

// x86ShadowStepBudget runs the given CPU forward by exactly `budget`
// guest instructions (or until the CPU naturally halts). Returns when
// the loop exits. Caller is responsible for snapshot capture afterward.
func x86ShadowStepBudget(t *testing.T, cpu *CPU_X86, forceNative bool, budget int64, deadline time.Duration) {
	t.Helper()
	cpu.x86InstrBudget = budget
	cpu.x86BudgetActive = true
	cpu.running.Store(true)
	cpu.Halted = false
	done := make(chan struct{})
	go func() {
		if forceNative {
			cpu.X86ExecuteJIT()
		} else {
			cpu.x86RunInterpreter()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(deadline):
		cpu.running.Store(false)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit after deadline + stop")
		}
		t.Fatalf("budget run did not complete within deadline %v (forceNative=%v)", deadline, forceNative)
	}
}

// x86ShadowParityCheckpoints runs the workload through both paths in
// fixed-budget windows, hashing register state at each checkpoint, and
// asserts the hashes are byte-identical. On mismatch, prints the first
// divergent checkpoint plus a per-register diff.
func x86ShadowParityCheckpoints(t *testing.T, romPath string, windowInstrs int64, checkpoints int) {
	t.Helper()
	interp := x86ShadowSetup(t, romPath, false)
	jit := x86ShadowSetup(t, romPath, true)

	// Per-checkpoint deadline. JIT side typically runs faster; interp
	// side is the long pole. 30 seconds per window is generous on slow
	// hardware while still keeping total under the 5-minute outer cap.
	deadline := 30 * time.Second

	for cp := 1; cp <= checkpoints; cp++ {
		x86ShadowStepBudget(t, interp, false, windowInstrs, deadline)
		x86ShadowStepBudget(t, jit, true, windowInstrs, deadline)

		interpBytes, interpSum := x86CanonicalStateHash(interp)
		jitBytes, jitSum := x86CanonicalStateHash(jit)
		interpMemSum := x86GuestMemoryHash(interp)
		jitMemSum := x86GuestMemoryHash(jit)

		if interpSum != jitSum {
			t.Errorf("checkpoint %d: register-state SHA mismatch", cp)
			t.Errorf("  interp: %s", hex.EncodeToString(interpSum[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitSum[:]))
			t.Errorf("  interp regs: EIP=%08X EAX=%08X ECX=%08X EDX=%08X EBX=%08X ESP=%08X EBP=%08X ESI=%08X EDI=%08X Flags=%08X Cycles=%d",
				interp.EIP, interp.EAX, interp.ECX, interp.EDX, interp.EBX, interp.ESP, interp.EBP, interp.ESI, interp.EDI, interp.Flags, interp.Cycles)
			t.Errorf("  jit    regs: EIP=%08X EAX=%08X ECX=%08X EDX=%08X EBX=%08X ESP=%08X EBP=%08X ESI=%08X EDI=%08X Flags=%08X Cycles=%d",
				jit.EIP, jit.EAX, jit.ECX, jit.EDX, jit.EBX, jit.ESP, jit.EBP, jit.ESI, jit.EDI, jit.Flags, jit.Cycles)
			// First differing byte index for forensics.
			for i := range interpBytes {
				if interpBytes[i] != jitBytes[i] {
					t.Errorf("  first diff at canonical-image byte %d: interp=%02X jit=%02X", i, interpBytes[i], jitBytes[i])
					break
				}
			}
			return // first failing checkpoint wins
		}
		if interpMemSum != jitMemSum {
			t.Errorf("checkpoint %d: guest-memory SHA mismatch", cp)
			t.Errorf("  interp: %s", hex.EncodeToString(interpMemSum[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitMemSum[:]))
			return // first failing checkpoint wins
		}
		t.Logf("checkpoint %d ok: SHA=%s EIP=%08X Cycles=%d", cp, hex.EncodeToString(interpSum[:8]), jit.EIP, jit.Cycles)
	}
}

// TestX86JIT_ShadowParity_Rotozoomer drives both paths through the
// rotozoomer binary in 50k-instruction windows for 4 checkpoints
// (200k guest instructions total). Workload is GPU-style register
// churn dominant, exercises the regalloc + chain-slot paths heavily.
func TestX86JIT_ShadowParity_Rotozoomer(t *testing.T) {
	rom := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86")
	x86ShadowParityCheckpoints(t, rom, 50_000, 4)
}

// TestX86JIT_ShadowParity_AnticPlasma adds a second workload to guard
// against rotozoomer-specific accidental parity. Plasma exercises a
// different opcode mix (more ALU-heavy MMIO writes).
func TestX86JIT_ShadowParity_AnticPlasma(t *testing.T) {
	rom := filepath.Join("sdk", "examples", "prebuilt", "antic_plasma_x86.ie86")
	x86ShadowParityCheckpoints(t, rom, 50_000, 4)
}

func TestX86JIT_ShadowParity_IEDoomLinkedImage(t *testing.T) {
	rom := filepath.Join("..", "chocolate-doom", "build", "iedoom.ie86")
	if _, err := os.Stat(rom); err != nil {
		t.Skipf("IEDoom linked image not present: %v", err)
	}
	// Keep IEDoom on the same four 50k-instruction checkpoint contract as the
	// continuous demos. The linked image's early startup is intentionally
	// included: it exercises the real loader, C runtime and Doom entry path
	// before the external timedemo acceptance advances gametic.
	x86ShadowParityCheckpoints(t, rom, 50_000, 4)
}
