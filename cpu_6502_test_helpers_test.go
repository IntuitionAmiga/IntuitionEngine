package main

import (
	"os"
	"testing"
	"time"
)

type cpu6502TestRig struct {
	bus *SystemBus
	cpu *CPU_6502
}

func newCPU6502TestRig() *cpu6502TestRig {
	bus := NewSystemBus()
	cpu := NewCPU_6502(bus)
	return &cpu6502TestRig{
		bus: bus,
		cpu: cpu,
	}
}

func (r *cpu6502TestRig) resetAndLoad(start uint16, program []byte) {
	r.bus.Reset()
	for i, value := range program {
		r.bus.Write8(uint32(start)+uint32(i), value)
	}
	r.cpu.Reset()
	r.cpu.PC = start
	r.cpu.SetRDYLine(true)
}

func (r *cpu6502TestRig) setVectors(entry uint16) {
	r.bus.Write8(RESET_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(RESET_VECTOR+1, uint8(entry>>8))
	r.bus.Write8(NMI_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(NMI_VECTOR+1, uint8(entry>>8))
	r.bus.Write8(IRQ_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(IRQ_VECTOR+1, uint8(entry>>8))
}

func runSingleInstruction(t *testing.T, cpu *CPU_6502, start uint16) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		pc := read6502PC(cpu)
		if pc != start {
			stop6502CPU(cpu)
			<-done
			return
		}
		if time.Now().After(deadline) {
			stop6502CPU(cpu)
			<-done
			t.Fatalf("timeout waiting for instruction at PC=0x%04X", start)
		}
	}
}

func runUntilPC(t *testing.T, cpu *CPU_6502, target uint16, timeout time.Duration) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()

	deadline := time.Now().Add(timeout)
	for {
		pc := read6502PC(cpu)
		if pc == target {
			stop6502CPU(cpu)
			<-done
			return
		}
		if !read6502Running(cpu) {
			stop6502CPU(cpu)
			<-done
			t.Fatalf("CPU stopped before reaching PC=0x%04X (current PC=0x%04X)", target, pc)
		}
		if time.Now().After(deadline) {
			stop6502CPU(cpu)
			<-done
			t.Fatalf("timeout waiting for PC=0x%04X (current PC=0x%04X, cycles=%d)", target, pc, read6502Cycles(cpu))
		}
	}
}

func runUntilCondition(t *testing.T, cpu *CPU_6502, timeout time.Duration, condition func() bool) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()

	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			stop6502CPU(cpu)
			<-done
			return
		}
		if !read6502Running(cpu) {
			stop6502CPU(cpu)
			<-done
			t.Fatalf("CPU stopped before condition was met (PC=0x%04X)", read6502PC(cpu))
		}
		if time.Now().After(deadline) {
			stop6502CPU(cpu)
			<-done
			t.Fatalf("timeout waiting for condition (PC=0x%04X, cycles=%d)", read6502PC(cpu), read6502Cycles(cpu))
		}
	}
}

func stop6502CPU(cpu *CPU_6502) {
	cpu.mutex.Lock()
	cpu.Running = false
	cpu.mutex.Unlock()
}

func read6502PC(cpu *CPU_6502) uint16 {
	cpu.mutex.RLock()
	defer cpu.mutex.RUnlock()
	return cpu.PC
}

func read6502Cycles(cpu *CPU_6502) uint64 {
	cpu.mutex.RLock()
	defer cpu.mutex.RUnlock()
	return cpu.Cycles
}

func read6502Running(cpu *CPU_6502) bool {
	cpu.mutex.RLock()
	defer cpu.mutex.RUnlock()
	return cpu.Running
}

func requireTestFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("missing test artifact %s (run xa to build the .bin)", path)
	}
	return data
}
