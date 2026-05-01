// jit_x86_rep_test.go - Slice-4 REP/string parity tests (force-native vs
// interpreter for REP MOVSB/MOVSD/STOSB/STOSD/CMPSB/SCASB).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// x86REPHarness sets up two CPUs (interp + force-native JIT), pre-fills
// scratch RAM with deterministic patterns, runs both to HLT, returns
// them for diffing.
func x86REPHarness(t *testing.T, code []byte, esi, edi, ecx uint32, eax uint32, srcSeed []byte) (interp, jit *CPU_X86) {
	t.Helper()
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	build := func(forceNative bool) *CPU_X86 {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
		cpu.EIP = 0x10000
		cpu.EAX = eax
		cpu.ECX = ecx
		cpu.ESI = esi
		cpu.EDI = edi
		cpu.ESP = 0x40000
		cpu.Flags = 0 // DF=0 (forward direction)
		for i, b := range code {
			cpu.memory[0x10000+uint32(i)] = b
		}
		// Seed source range so MOVSB/CMPSB/SCASB have something to read.
		for i, b := range srcSeed {
			cpu.memory[esi+uint32(i)] = b
		}
		if forceNative {
			cpu.x86JitEnabled = true
		}
		cpu.running.Store(true)
		cpu.Halted = false
		done := make(chan struct{})
		go func() {
			if forceNative {
				cpu.X86ExecuteJIT()
			} else {
				for cpu.Running() && !cpu.Halted {
					cpu.Step()
				}
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cpu.running.Store(false)
			<-done
			t.Fatal("execution timed out")
		}
		return cpu
	}
	return build(false), build(true)
}

func x86REPDiff(interp, jit *CPU_X86, memLo, memHi uint32) string {
	if interp.EIP != jit.EIP {
		return fmt.Sprintf("EIP: interp=%08X jit=%08X", interp.EIP, jit.EIP)
	}
	pairs := [...]struct {
		name string
		ip   uint32
		jt   uint32
	}{
		{"EAX", interp.EAX, jit.EAX}, {"ECX", interp.ECX, jit.ECX},
		{"EDX", interp.EDX, jit.EDX}, {"EBX", interp.EBX, jit.EBX},
		{"ESP", interp.ESP, jit.ESP}, {"EBP", interp.EBP, jit.EBP},
		{"ESI", interp.ESI, jit.ESI}, {"EDI", interp.EDI, jit.EDI},
	}
	for _, p := range pairs {
		if p.ip != p.jt {
			return fmt.Sprintf("%s: interp=%08X jit=%08X", p.name, p.ip, p.jt)
		}
	}
	if !bytes.Equal(interp.memory[memLo:memHi], jit.memory[memLo:memHi]) {
		for i := memLo; i < memHi; i++ {
			if interp.memory[i] != jit.memory[i] {
				return fmt.Sprintf("memory[%X]: interp=%02X jit=%02X",
					i, interp.memory[i], jit.memory[i])
			}
		}
	}
	return ""
}

// TestX86JIT_REP_MOVSB_Slice4: copy 32 bytes from 0x20000 to 0x21000.
func TestX86JIT_REP_MOVSB_Slice4(t *testing.T) {
	code := []byte{
		0xF3, 0xA4, // REP MOVSB
		0xF4, // HLT
	}
	srcSeed := make([]byte, 64)
	for i := range srcSeed {
		srcSeed[i] = byte(0xA0 + i)
	}
	interp, jit := x86REPHarness(t, code, 0x20000, 0x21000, 32, 0, srcSeed)
	if msg := x86REPDiff(interp, jit, 0x21000, 0x21040); msg != "" {
		t.Errorf("MOVSB: %s", msg)
	}
	if interp.ECX != 0 {
		t.Errorf("MOVSB interp ECX = %d, want 0", interp.ECX)
	}
}

// TestX86JIT_REP_MOVSD_Slice4: copy 8 dwords from 0x20000 to 0x21000.
func TestX86JIT_REP_MOVSD_Slice4(t *testing.T) {
	code := []byte{
		0xF3, 0xA5, // REP MOVSD
		0xF4, // HLT
	}
	srcSeed := make([]byte, 64)
	for i := range srcSeed {
		srcSeed[i] = byte(0xB0 + i)
	}
	interp, jit := x86REPHarness(t, code, 0x20000, 0x21000, 8, 0, srcSeed)
	if msg := x86REPDiff(interp, jit, 0x21000, 0x21040); msg != "" {
		t.Errorf("MOVSD: %s", msg)
	}
}

// TestX86JIT_REP_STOSB_Slice4: fill 16 bytes at 0x21000 with AL=0x55.
func TestX86JIT_REP_STOSB_Slice4(t *testing.T) {
	code := []byte{
		0xF3, 0xAA, // REP STOSB
		0xF4, // HLT
	}
	interp, jit := x86REPHarness(t, code, 0, 0x21000, 16, 0x55, nil)
	if msg := x86REPDiff(interp, jit, 0x21000, 0x21020); msg != "" {
		t.Errorf("STOSB: %s", msg)
	}
}

// TestX86JIT_REP_STOSD_Slice4: fill 4 dwords at 0x21000 with EAX=0xDEADBEEF.
func TestX86JIT_REP_STOSD_Slice4(t *testing.T) {
	code := []byte{
		0xF3, 0xAB, // REP STOSD
		0xF4, // HLT
	}
	interp, jit := x86REPHarness(t, code, 0, 0x21000, 4, 0xDEADBEEF, nil)
	if msg := x86REPDiff(interp, jit, 0x21000, 0x21020); msg != "" {
		t.Errorf("STOSD: %s", msg)
	}
}

// TestX86JIT_REPE_CMPSB_Slice4: compare 16 equal bytes; ECX should reach 0,
// ZF=1.
func TestX86JIT_REPE_CMPSB_Slice4(t *testing.T) {
	// Pre-fill both src and dst with same pattern so REPE CMPSB completes
	// without early exit.
	code := []byte{
		0xF3, 0xA6, // REPE CMPSB
		0xF4, // HLT
	}
	srcSeed := make([]byte, 64)
	for i := range srcSeed {
		srcSeed[i] = byte(0xC0 + (i & 0x0F))
	}
	interp, jit := x86REPHarness(t, code, 0x20000, 0x21000, 16, 0, srcSeed)
	// Pre-fill dst to match src so equal-comparison runs to ECX=0.
	for i := uint32(0); i < 64; i++ {
		interp.memory[0x21000+i] = srcSeed[i]
		jit.memory[0x21000+i] = srcSeed[i]
	}
	// Re-run with matching pre-fill applied (harness already ran, results
	// reflect mismatch on first byte). For this test the pre-fill needed
	// to happen BEFORE the run; rebuild via direct setup.
	build := func(forceNative bool) *CPU_X86 {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
		cpu.EIP = 0x10000
		cpu.ECX = 16
		cpu.ESI = 0x20000
		cpu.EDI = 0x21000
		cpu.ESP = 0x40000
		for i := uint32(0); i < 64; i++ {
			cpu.memory[0x20000+i] = srcSeed[i]
			cpu.memory[0x21000+i] = srcSeed[i]
		}
		for i, b := range code {
			cpu.memory[0x10000+uint32(i)] = b
		}
		if forceNative {
			cpu.x86JitEnabled = true
		}
		cpu.running.Store(true)
		cpu.Halted = false
		done := make(chan struct{})
		go func() {
			if forceNative {
				cpu.X86ExecuteJIT()
			} else {
				for cpu.Running() && !cpu.Halted {
					cpu.Step()
				}
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cpu.running.Store(false)
			<-done
			t.Fatal("timed out")
		}
		return cpu
	}
	interp = build(false)
	jit = build(true)
	if interp.ECX != jit.ECX {
		t.Errorf("CMPSB ECX: interp=%d jit=%d", interp.ECX, jit.ECX)
	}
	// ZF/CF compare.
	if (interp.Flags & 0x40) != (jit.Flags & 0x40) {
		t.Errorf("CMPSB ZF: interp=%X jit=%X", interp.Flags&0x40, jit.Flags&0x40)
	}
}
