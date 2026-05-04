package main

import (
	"sync/atomic"
	"testing"
)

func TestM68K_INTENAAddressIsPlainMemory(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.Write16(0xDFF09A, 0x4000)
	if got := cpu.Read16(0xDFF09A); got != 0x4000 {
		t.Fatalf("Read16(0xDFF09A)=0x%04X, want plain memory value 0x4000", got)
	}
}

func TestM68K_ProcessInterruptGatesOnSRIPLOnly(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x1000)
	cpu.AddrRegs[7] = 0x20000
	cpu.SR = 0
	if !cpu.ProcessInterrupt(4) {
		t.Fatalf("level 4 interrupt blocked with IPL=0")
	}

	cpu = NewM68KCPU(bus)
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x1000)
	cpu.AddrRegs[7] = 0x20000
	cpu.SR = 7 << M68K_SR_SHIFT
	if cpu.ProcessInterrupt(4) {
		t.Fatalf("level 4 interrupt delivered with IPL=7")
	}
}

func TestM68K_OptionalINTENAGatesInterrupts(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	intena := &atomic.Bool{}
	intena.Store(true)
	cpu.AmigaINTENA = intena
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x1000)
	cpu.AddrRegs[7] = 0x20000
	cpu.SR = 0

	cpu.Write16(0xDFF09A, 0x4000)
	if intena.Load() {
		t.Fatalf("INTENA still enabled after clear write")
	}
	if cpu.ProcessInterrupt(4) {
		t.Fatalf("level 4 interrupt delivered while INTENA disabled")
	}

	cpu.Write16(0xDFF09A, 0xC000)
	if !intena.Load() {
		t.Fatalf("INTENA not enabled after set write")
	}
	if !cpu.ProcessInterrupt(4) {
		t.Fatalf("level 4 interrupt blocked after INTENA enabled")
	}
}
