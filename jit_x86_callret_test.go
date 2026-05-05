// jit_x86_callret_test.go - Slice-3 cross-block CALL/RET correctness tests
// (force-native vs interpreter parity for hand-crafted CALL/RET sequences).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"testing"
	"time"
)

// x86CallRetHarness builds two CPUs (interp + force-native JIT), loads the
// given program at 0x10000, runs both to HLT, and returns them for diffing.
// ESP is pre-set to a known stack-top inside scratch RAM.
func x86CallRetHarness(t *testing.T, code []byte) (interp, jit *CPU_X86) {
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
		cpu.EAX = 0x11111111
		cpu.ECX = 0x22222222
		cpu.EDX = 0x33333333
		cpu.EBX = 0x44444444
		cpu.ESI = 0x20000
		cpu.EDI = 0x20000
		cpu.EBP = 0
		cpu.ESP = 0x20C00
		cpu.Flags = 0
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
			waitDoneWithGuard(t, done)
			t.Fatal("execution timed out")
		}
		return cpu
	}
	return build(false), build(true)
}

func x86CallRetDiff(interp, jit *CPU_X86) string {
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
	return ""
}

// TestX86JIT_CallRet_Simple: single CALL to a function that adds to EBX
// and returns. Validates retAddr push, function exec, RET pop, fall-through.
func TestX86JIT_CallRet_Simple(t *testing.T) {
	// 0x10000: CALL +5             (E8 05 00 00 00 — target = 0x1000A)
	// 0x10005: ADD EAX, 1          (83 C0 01)
	// 0x10008: HLT                 (F4)
	// 0x10009: <unused>             (00)
	// 0x1000A: ADD EBX, 0x10        (83 C3 10)
	// 0x1000D: RET                 (C3)
	code := []byte{
		0xE8, 0x05, 0x00, 0x00, 0x00, // CALL +5
		0x83, 0xC0, 0x01, // ADD EAX, 1
		0xF4,             // HLT
		0x00,             // pad
		0x83, 0xC3, 0x10, // ADD EBX, 0x10
		0xC3, // RET
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Errorf("simple CALL/RET: %s", msg)
	}
}

// TestX86JIT_CallRet_PushPopAround: PUSH/POP around a CALL.
func TestX86JIT_CallRet_PushPopAround(t *testing.T) {
	// 0x10000: PUSH ECX             (51)
	// 0x10001: CALL +5              (E8 05 00 00 00, target=0x1000B)
	// 0x10006: POP EAX              (58)
	// 0x10007: ADD EBX, 1           (83 C3 01)
	// 0x1000A: HLT                  (F4)
	// 0x1000B: ADD EBX, 0x20        (83 C3 20)
	// 0x1000E: RET                  (C3)
	code := []byte{
		0x51,                         // PUSH ECX
		0xE8, 0x05, 0x00, 0x00, 0x00, // CALL +5
		0x58,             // POP EAX
		0x83, 0xC3, 0x01, // ADD EBX, 1
		0xF4,             // HLT
		0x83, 0xC3, 0x20, // ADD EBX, 0x20
		0xC3, // RET
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Errorf("PUSH/POP/CALL: %s", msg)
	}
}

// TestX86JIT_CallRet_TwoCalls: two CALLs to the same function from
// different sites. Each CALL has a distinct return PC; the second RET's
// cache probe should miss the first return PC and bail to the dispatcher.
func TestX86JIT_CallRet_TwoCalls(t *testing.T) {
	// 0x10000: CALL +0x10           (E8 10 00 00 00, target=0x10015)
	// 0x10005: ADD EAX, 0x10        (83 C0 10)
	// 0x10008: CALL +8              (E8 08 00 00 00, target=0x10015)
	// 0x1000D: ADD ECX, 0x20        (83 C1 20)
	// 0x10010: HLT                  (F4)
	// 0x10011..14: pad
	// 0x10015: ADD EBX, 0x30        (83 C3 30)
	// 0x10018: RET                  (C3)
	code := []byte{
		0xE8, 0x10, 0x00, 0x00, 0x00, // CALL +0x10
		0x83, 0xC0, 0x10, // ADD EAX, 0x10
		0xE8, 0x08, 0x00, 0x00, 0x00, // CALL +8
		0x83, 0xC1, 0x20, // ADD ECX, 0x20
		0xF4,                   // HLT
		0x00, 0x00, 0x00, 0x00, // pad
		0x83, 0xC3, 0x30, // ADD EBX, 0x30
		0xC3, // RET
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Errorf("two CALLs: %s", msg)
	}
}
