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
	return x86CallRetHarnessWithSetup(t, code, nil)
}

func x86CallRetHarnessWithSetup(t *testing.T, code []byte, setup func(*CPU_X86)) (interp, jit *CPU_X86) {
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
		if setup != nil {
			setup(cpu)
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

func TestX86JIT_CallRet_FrameByteCopyLoop(t *testing.T) {
	code := []byte{
		0xBF, 0x00, 0x20, 0x02, 0x00, // MOV EDI, 0x22000
		0xBE, 0x00, 0x10, 0x02, 0x00, // MOV ESI, 0x21000
		0xB9, 0x0F, 0x00, 0x00, 0x00, // MOV ECX, 15
		0x51,                         // PUSH ECX
		0x56,                         // PUSH ESI
		0x57,                         // PUSH EDI
		0xE8, 0x04, 0x00, 0x00, 0x00, // CALL memcpy-like function
		0x83, 0xC4, 0x0C, // ADD ESP, 12
		0xF4, // HLT

		0x55,       // PUSH EBP
		0x89, 0xE5, // MOV EBP, ESP
		0x83, 0xEC, 0x08, // SUB ESP, 8
		0x8B, 0x55, 0x0C, // MOV EDX, [EBP+12]
		0x89, 0x55, 0xF8, // MOV [EBP-8], EDX
		0x8B, 0x45, 0x08, // MOV EAX, [EBP+8]
		0x89, 0x45, 0xFC, // MOV [EBP-4], EAX
		0x8B, 0x55, 0xF8, // MOV EDX, [EBP-8]
		0x8D, 0x42, 0x01, // LEA EAX, [EDX+1]
		0x89, 0x45, 0xF8, // MOV [EBP-8], EAX
		0x8B, 0x45, 0xFC, // MOV EAX, [EBP-4]
		0x8D, 0x48, 0x01, // LEA ECX, [EAX+1]
		0x89, 0x4D, 0xFC, // MOV [EBP-4], ECX
		0x8A, 0x12, // MOV DL, [EDX]
		0x88, 0x10, // MOV [EAX], DL
		0x8B, 0x45, 0x10, // MOV EAX, [EBP+16]
		0x8D, 0x50, 0xFF, // LEA EDX, [EAX-1]
		0x89, 0x55, 0x10, // MOV [EBP+16], EDX
		0x85, 0xC0, // TEST EAX, EAX
		0x75, 0xDD, // JNZ loop
		0x8B, 0x45, 0x08, // MOV EAX, [EBP+8]
		0xC9, // LEAVE
		0xC3, // RET
	}
	setup := func(cpu *CPU_X86) {
		for i := 0; i < 16; i++ {
			cpu.memory[0x21000+uint32(i)] = byte(0xA0 + i)
			cpu.memory[0x22000+uint32(i)] = 0
		}
		cpu.memory[0x22010] = 0x5A
	}
	interp, jit := x86CallRetHarnessWithSetup(t, code, setup)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("frame byte-copy CALL/RET: %s", msg)
	}
	for i := 0; i < 17; i++ {
		addr := 0x22000 + uint32(i)
		if interp.memory[addr] != jit.memory[addr] {
			t.Fatalf("dest[%d]: interp=%02X jit=%02X", i, interp.memory[addr], jit.memory[addr])
		}
	}
}

func TestX86JIT_CallRet_OptionScanStrcmpLoop(t *testing.T) {
	code := make([]byte, 0xC0)
	mainCode := []byte{
		0x68, 0x00, 0x20, 0x03, 0x00, // PUSH 0x32000 (search string)
		0x68, 0x00, 0x00, 0x03, 0x00, // PUSH 0x30000 (options struct)
		0xE8, 0x11, 0x00, 0x00, 0x00, // CALL 0x10020
		0x83, 0xC4, 0x08, // ADD ESP, 8
		0xF4, // HLT
	}
	copy(code, mainCode)

	findOption := []byte{
		0x55,       // PUSH EBP
		0x89, 0xE5, // MOV EBP, ESP
		0x83, 0xEC, 0x18, // SUB ESP, 0x18
		0xC7, 0x45, 0xF4, 0, 0, 0, 0, // MOV [EBP-0xc], 0
		0xEB, 0x40, // JMP check
		0x8B, 0x45, 0x08, // loop: MOV EAX, [EBP+8]
		0x8B, 0x08, // MOV ECX, [EAX]
		0x8B, 0x55, 0xF4, // MOV EDX, [EBP-0xc]
		0x89, 0xD0, // MOV EAX, EDX
		0x01, 0xC0, // ADD EAX, EAX
		0x01, 0xD0, // ADD EAX, EDX
		0xC1, 0xE0, 0x03, // SHL EAX, 3
		0x01, 0xC8, // ADD EAX, ECX
		0x8B, 0x00, // MOV EAX, [EAX]
		0x83, 0xEC, 0x08, // SUB ESP, 8
		0x50,             // PUSH EAX
		0xFF, 0x75, 0x0C, // PUSH [EBP+0xc]
		0xE8, 0x40, 0x00, 0x00, 0x00, // CALL strcmp at 0x10090
		0x83, 0xC4, 0x10, // ADD ESP, 0x10
		0x85, 0xC0, // TEST EAX, EAX
		0x75, 0x15, // JNZ next
		0x8B, 0x45, 0x08, // MOV EAX, [EBP+8]
		0x8B, 0x08, // MOV ECX, [EAX]
		0x8B, 0x55, 0xF4, // MOV EDX, [EBP-0xc]
		0x89, 0xD0, // MOV EAX, EDX
		0x01, 0xC0, // ADD EAX, EAX
		0x01, 0xD0, // ADD EAX, EDX
		0xC1, 0xE0, 0x03, // SHL EAX, 3
		0x01, 0xC8, // ADD EAX, ECX
		0xEB, 0x13, // JMP done
		0xFF, 0x45, 0xF4, // next: INC [EBP-0xc]
		0x8B, 0x45, 0x08, // check: MOV EAX, [EBP+8]
		0x8B, 0x40, 0x04, // MOV EAX, [EAX+4]
		0x39, 0x45, 0xF4, // CMP [EBP-0xc], EAX
		0x7C, 0xB5, // JL loop
		0xB8, 0, 0, 0, 0, // MOV EAX, 0
		0xC9, // LEAVE
		0xC3, // RET
	}
	copy(code[0x20:], findOption)

	strcmp := []byte{
		0x55,       // PUSH EBP
		0x89, 0xE5, // MOV EBP, ESP
		0xEB, 0x06, // JMP check
		0xFF, 0x45, 0x08, // INC [EBP+8]
		0xFF, 0x45, 0x0C, // INC [EBP+12]
		0x8B, 0x45, 0x08, // check: MOV EAX, [EBP+8]
		0x8A, 0x00, // MOV AL, [EAX]
		0x84, 0xC0, // TEST AL, AL
		0x74, 0x0E, // JZ finish
		0x8B, 0x45, 0x08, // MOV EAX, [EBP+8]
		0x8A, 0x10, // MOV DL, [EAX]
		0x8B, 0x45, 0x0C, // MOV EAX, [EBP+12]
		0x8A, 0x00, // MOV AL, [EAX]
		0x38, 0xC2, // CMP DL, AL
		0x74, 0xE3, // JZ advance
		0x8B, 0x45, 0x08, // finish: MOV EAX, [EBP+8]
		0x8A, 0x00, // MOV AL, [EAX]
		0x0F, 0xB6, 0xD0, // MOVZX EDX, AL
		0x8B, 0x45, 0x0C, // MOV EAX, [EBP+12]
		0x8A, 0x00, // MOV AL, [EAX]
		0x0F, 0xB6, 0xC0, // MOVZX EAX, AL
		0x29, 0xC2, // SUB EDX, EAX
		0x89, 0xD0, // MOV EAX, EDX
		0x5D, // POP EBP
		0xC3, // RET
	}
	copy(code[0x90:], strcmp)

	setup := func(cpu *CPU_X86) {
		put32 := func(addr, v uint32) {
			cpu.memory[addr] = byte(v)
			cpu.memory[addr+1] = byte(v >> 8)
			cpu.memory[addr+2] = byte(v >> 16)
			cpu.memory[addr+3] = byte(v >> 24)
		}
		putCString := func(addr uint32, s string) {
			for i := range s {
				cpu.memory[addr+uint32(i)] = s[i]
			}
			cpu.memory[addr+uint32(len(s))] = 0
		}
		put32(0x30000, 0x30100)
		put32(0x30004, 3)
		put32(0x30100, 0x31000)
		put32(0x30118, 0x31010)
		put32(0x30130, 0x31020)
		putCString(0x31000, "alpha")
		putCString(0x31010, "beta")
		putCString(0x31020, "target")
		putCString(0x32000, "target")
	}
	interp, jit := x86CallRetHarnessWithSetup(t, code, setup)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("option-scan strcmp loop: %s", msg)
	}
}

func TestX86JIT_OperandSizeMOVFallsBackToWordSemantics(t *testing.T) {
	code := []byte{
		0xB8, 0x00, 0x10, 0x02, 0x00, // MOV EAX, 0x21000
		0xBA, 0xDD, 0xCC, 0xBB, 0xAA, // MOV EDX, 0xAABBCCDD
		0x66, 0x89, 0x10, // MOV [EAX], DX
		0xBB, 0x00, 0x10, 0x02, 0x00, // MOV EBX, 0x21000
		0xB8, 0xFF, 0xFF, 0xFF, 0xFF, // MOV EAX, 0xFFFFFFFF
		0x66, 0x8B, 0x03, // MOV AX, [EBX]
		0xF4, // HLT
	}
	setup := func(cpu *CPU_X86) {
		cpu.memory[0x21000] = 0x11
		cpu.memory[0x21001] = 0x22
		cpu.memory[0x21002] = 0x33
		cpu.memory[0x21003] = 0x44
	}
	interp, jit := x86CallRetHarnessWithSetup(t, code, setup)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("operand-size MOV fallback: %s", msg)
	}
	if got := jit.EAX; got != 0xFFFFCCDD {
		t.Fatalf("MOV AX,[EBX] produced EAX=%08X, want FFFFCCDD", got)
	}
	if got := []byte{jit.memory[0x21000], jit.memory[0x21001], jit.memory[0x21002], jit.memory[0x21003]}; got[0] != 0xDD || got[1] != 0xCC || got[2] != 0x33 || got[3] != 0x44 {
		t.Fatalf("MOV [EAX],DX wrote bytes % X, want DD CC 33 44", got)
	}
}

func TestX86JIT_Stack_PushESPUsesOriginalValue(t *testing.T) {
	code := []byte{
		0x54, // PUSH ESP
		0x58, // POP EAX
		0xF4, // HLT
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("PUSH ESP: %s", msg)
	}
}

func TestX86JIT_Stack_PopESPUsesPoppedValue(t *testing.T) {
	code := []byte{
		0xBC, 0x00, 0x0C, 0x02, 0x00, // MOV ESP, 0x20C00
		0x5C, // POP ESP
		0xF4, // HLT
	}
	setup := func(cpu *CPU_X86) {
		cpu.memory[0x20C00] = 0x78
		cpu.memory[0x20C01] = 0x56
		cpu.memory[0x20C02] = 0x34
		cpu.memory[0x20C03] = 0x12
	}
	interp, jit := x86CallRetHarnessWithSetup(t, code, setup)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("POP ESP: %s", msg)
	}
}

func TestX86JIT_CallRet_FallbackPushMemoryBeforeCall(t *testing.T) {
	code := []byte{
		0xB8, 0x44, 0x33, 0x22, 0x11, // MOV EAX, 0x11223344
		0x50,             // PUSH EAX
		0xFF, 0x34, 0x24, // PUSH DWORD PTR [ESP] (interpreter fallback)
		0xE8, 0x04, 0x00, 0x00, 0x00, // CALL function
		0x83, 0xC4, 0x08, // ADD ESP, 8
		0xF4, // HLT

		0x55,       // PUSH EBP
		0x89, 0xE5, // MOV EBP, ESP
		0x8B, 0x45, 0x08, // MOV EAX, [EBP+8]
		0x8B, 0x55, 0x0C, // MOV EDX, [EBP+12]
		0x01, 0xD0, // ADD EAX, EDX
		0xC9, // LEAVE
		0xC3, // RET
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("fallback PUSH r/m before CALL: %s", msg)
	}
}

func TestX86JIT_LEAPreservesFlagsBeforeJcc(t *testing.T) {
	code := []byte{
		0xB8, 0x05, 0x00, 0x00, 0x00, // MOV EAX, 5
		0xBB, 0x05, 0x00, 0x00, 0x00, // MOV EBX, 5
		0xB9, 0x01, 0x00, 0x00, 0x00, // MOV ECX, 1
		0x39, 0xD8, // CMP EAX, EBX
		0x8D, 0x49, 0x01, // LEA ECX, [ECX+1]
		0x74, 0x06, // JE good
		0xBA, 0x11, 0x01, 0x00, 0x00, // MOV EDX, 0x111
		0xF4,                         // HLT
		0xBA, 0x22, 0x02, 0x00, 0x00, // MOV EDX, 0x222
		0xF4, // HLT
	}
	interp, jit := x86CallRetHarness(t, code)
	if msg := x86CallRetDiff(interp, jit); msg != "" {
		t.Fatalf("LEA flags before Jcc: %s", msg)
	}
}
