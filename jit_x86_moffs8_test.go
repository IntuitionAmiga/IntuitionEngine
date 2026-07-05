//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// The T&L coprocessor service advances its mailbox ring tail with
// MOV moffs8, AL (0xA2) and polls the head with MOV AL, moffs8 (0xA0).
// A silent failure in either desynchronizes the ring: the tail never
// visibly advances and the guest spins to its drain cap on every batch.

func TestX86JIT_MOV_moffs8_AL_Store(t *testing.T) {
	r := newX86JITTestRig(t)
	addr := uint32(0x790C01) // the ring-tail byte the service writes
	r.cpu.EAX = 0x00000042

	// A2 id: MOV moffs8, AL
	r.compileAndRun(t, 0x1000,
		0xA2, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24),
	)

	if got := r.bus.Read8(addr); got != 0x42 {
		t.Errorf("mem[0x%X] = 0x%02X, want 0x42", addr, got)
	}
}

func TestX86JIT_MOV_AL_moffs8_Load(t *testing.T) {
	r := newX86JITTestRig(t)
	addr := uint32(0x790C00)
	r.bus.Write8(addr, 0x37)
	r.cpu.EAX = 0xAABBCC00

	// A0 id: MOV AL, moffs8 (high bytes of EAX preserved)
	r.compileAndRun(t, 0x1000,
		0xA0, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24),
	)

	if r.cpu.EAX != 0xAABBCC37 {
		t.Errorf("EAX = 0x%08X, want 0xAABBCC37", r.cpu.EAX)
	}
}
