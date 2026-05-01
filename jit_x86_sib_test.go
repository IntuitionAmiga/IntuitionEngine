// jit_x86_sib_test.go - Slice-2 SIB-encoded memory parity tests
// (force-native JIT vs interpreter for MOV with [base + index*scale + disp]
// addressing). Each subtest pins index/base regs to known values, runs the
// program through both backends, and asserts identical regs/EIP/memory.
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

// x86SIBHarness sets up two CPUs (interp + force-native JIT) with the
// given prelude bytes, runs both to HLT, and returns them for diffing.
func x86SIBHarness(t *testing.T, code []byte) (interp, jit *CPU_X86) {
	t.Helper()
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	startPC := uint32(0x10000)
	memBase := uint32(0x20000)

	build := func(forceNative bool) *CPU_X86 {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
		cpu.EIP = startPC
		// Pin scratch-region pointers; tests adjust ESI/EDI/ECX as needed.
		cpu.ESI = memBase
		cpu.EDI = memBase
		cpu.ECX = 0
		cpu.EBX = 0
		cpu.EAX = 0
		cpu.EDX = 0
		cpu.EBP = 0
		cpu.ESP = memBase + 0xC00
		// Seed scratch RAM with a recognizable pattern so reads can be
		// asserted against a deterministic source.
		for i := uint32(0); i < 0x800; i++ {
			cpu.memory[memBase+i] = byte(i & 0xFF)
		}
		for i, b := range code {
			cpu.memory[startPC+uint32(i)] = b
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

func x86SIBDiff(interp, jit *CPU_X86) string {
	if interp.EIP != jit.EIP {
		return fmt.Sprintf("EIP: interp=%08X jit=%08X", interp.EIP, jit.EIP)
	}
	pairs := [...]struct {
		name   string
		ip, jt uint32
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
	if !bytes.Equal(interp.memory[0x20000:0x20800], jit.memory[0x20000:0x20800]) {
		for i := uint32(0); i < 0x800; i++ {
			if interp.memory[0x20000+i] != jit.memory[0x20000+i] {
				return fmt.Sprintf("memory[%X]: interp=%02X jit=%02X",
					0x20000+i, interp.memory[0x20000+i], jit.memory[0x20000+i])
			}
		}
	}
	return ""
}

// TestX86JIT_SIB_Load_BaseIndexScale exercises MOV r32, [base + index*scale + disp]
// across scales 1, 2, 4, 8 with a fixed-magnitude index so the resulting
// address always lands inside scratch RAM. The seed pattern above makes
// memory[memBase+k] = k&0xFF, so each load pulls a deterministic 4-byte
// little-endian value.
func TestX86JIT_SIB_Load_BaseIndexScale(t *testing.T) {
	cases := []struct {
		name  string
		scale byte // SIB scale field 0=1, 1=2, 2=4, 3=8
	}{
		{"scale1", 0},
		{"scale2", 1},
		{"scale4", 2},
		{"scale8", 3},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// Program:
			//   MOV ECX, 0x10               ; index = 16
			//   MOV EAX, [ESI + ECX*scale + 0x20]  ; SIB load
			//   HLT
			// SIB byte: scale<<6 | index(ECX=1)<<3 | base(ESI=6)
			modrm := byte(0x44) // mod=01 reg=000 (EAX) rm=100 (SIB)
			sib := byte((c.scale << 6) | (1 << 3) | 6)
			code := []byte{
				0xB9, 0x10, 0x00, 0x00, 0x00, // MOV ECX, 0x10
				0x8B, modrm, sib, 0x20, // MOV EAX, [ESI + ECX*scale + 0x20]
				0xF4, // HLT
			}
			interp, jit := x86SIBHarness(t, code)
			if msg := x86SIBDiff(interp, jit); msg != "" {
				t.Errorf("%s: %s", c.name, msg)
			}
		})
	}
}

// TestX86JIT_SIB_Store_BaseIndexScale exercises MOV [base + index*scale + disp], r32.
func TestX86JIT_SIB_Store_BaseIndexScale(t *testing.T) {
	cases := []struct {
		name  string
		scale byte
	}{
		{"scale1", 0},
		{"scale2", 1},
		{"scale4", 2},
		{"scale8", 3},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// Program:
			//   MOV EAX, 0xDEADBEEF
			//   MOV ECX, 0x10
			//   MOV [EDI + ECX*scale + 0x40], EAX
			//   HLT
			modrm := byte(0x44) // mod=01 reg=000 (EAX) rm=100 (SIB)
			sib := byte((c.scale << 6) | (1 << 3) | 7)
			code := []byte{
				0xB8, 0xEF, 0xBE, 0xAD, 0xDE, // MOV EAX, 0xDEADBEEF
				0xB9, 0x10, 0x00, 0x00, 0x00, // MOV ECX, 0x10
				0x89, modrm, sib, 0x40, // MOV [EDI + ECX*scale + 0x40], EAX
				0xF4, // HLT
			}
			interp, jit := x86SIBHarness(t, code)
			if msg := x86SIBDiff(interp, jit); msg != "" {
				t.Errorf("%s: %s", c.name, msg)
			}
		})
	}
}

// TestX86JIT_SIB_NoIndex covers SIB with index=4 (encoded "no index"):
// addr = base + disp. Verifies the SIB-decoder special case.
func TestX86JIT_SIB_NoIndex(t *testing.T) {
	// MOV EAX, [ESI + 0x10]  via SIB encoding with index=4 (none).
	// modrm = mod=01 reg=000 rm=100 (SIB)
	// sib   = scale=00 index=100 base=110 (ESI)
	code := []byte{
		0x8B, 0x44, 0x26, 0x10, // MOV EAX, [ESI + 0x10] (SIB no-index)
		0xF4, // HLT
	}
	interp, jit := x86SIBHarness(t, code)
	if msg := x86SIBDiff(interp, jit); msg != "" {
		t.Errorf("%s", msg)
	}
}

// TestX86JIT_SIB_Disp32 covers SIB with mod=10 (disp32) and index reg.
func TestX86JIT_SIB_Disp32(t *testing.T) {
	// MOV EAX, [ESI + ECX*4 + 0x80]
	code := []byte{
		0xB9, 0x08, 0x00, 0x00, 0x00, // MOV ECX, 8
		0x8B, 0x84, 0x8E, // MOV EAX, [ESI + ECX*4 + disp32], modrm=10_000_100, sib=10_001_110
		0x80, 0x00, 0x00, 0x00, // disp32 = 0x80
		0xF4, // HLT
	}
	interp, jit := x86SIBHarness(t, code)
	if msg := x86SIBDiff(interp, jit); msg != "" {
		t.Errorf("%s", msg)
	}
}
