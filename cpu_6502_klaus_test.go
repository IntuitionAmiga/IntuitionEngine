package main

import (
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	klausFunctionalBin      = "testdata/6502/klaus/6502_functional_test.bin"
	klausDecimalBin         = "testdata/6502/klaus/6502_decimal_test.bin"
	klausInterruptBin       = "testdata/6502/klaus/6502_interrupt_test.bin"
	klausFunctionalSuccess  = 0x3469
	klausFunctionalEntry    = 0x0400
	klausDecimalEntry       = 0x0200
	klausDecimalErrorAddr   = 0x000B
	klausInterruptEntry     = 0x0400
	klausInterruptIOLoc     = 0xBFFC
	klausInterruptIRQBit    = 0
	klausInterruptNMIBit    = 1
	klausInterruptIOFilter  = 0x7F
	klausInterruptLoadBase  = 0x000A
	klausInterruptNMITrap   = 0x0739
	klausInterruptResTrap   = 0x0778
	klausInterruptIRQTrap   = 0x077D
	klausInterruptEnvTarget = "KLAUS_INTERRUPT_SUCCESS_PC"
	klausInterruptTimeout   = 60 * time.Second
	klausFunctionalTimeout  = 60 * time.Second
	klausFunctionalEnv      = "KLAUS_FUNCTIONAL"
	klausDecimalTimeout     = 60 * time.Second
)

func Test6502KlausFunctional(t *testing.T) {
	if os.Getenv(klausFunctionalEnv) == "" {
		t.Skipf("set %s=1 to run the Klaus functional test", klausFunctionalEnv)
	}

	rig := newCPU6502TestRig()
	data := requireTestFile(t, klausFunctionalBin)
	if len(data) != 0x10000 {
		t.Fatalf("functional test size=%d, want 65536", len(data))
	}

	for i, value := range data {
		rig.bus.Write8(uint32(i), value)
	}

	rig.cpu.Reset()
	rig.cpu.PC = klausFunctionalEntry
	rig.cpu.SetRDYLine(true)
	runUntilPC(t, rig.cpu, klausFunctionalSuccess, klausFunctionalTimeout)
}

func Test6502KlausDecimal(t *testing.T) {
	rig := newCPU6502TestRig()
	data := requireTestFile(t, klausDecimalBin)

	for i, value := range data {
		rig.bus.Write8(uint32(klausDecimalEntry)+uint32(i), value)
	}
	rig.setVectors(klausDecimalEntry)

	rig.cpu.Reset()
	rig.cpu.SetRDYLine(true)

	runUntilCondition(t, rig.cpu, klausDecimalTimeout, func() bool {
		return rig.bus.Read8(klausDecimalErrorAddr) == 0
	})
}

func Test6502KlausInterrupt(t *testing.T) {
	successPC := readInterruptSuccessPC(t)
	rig := newCPU6502TestRig()
	data := requireTestFile(t, klausInterruptBin)

	for i, value := range data {
		rig.bus.Write8(uint32(klausInterruptLoadBase)+uint32(i), value)
	}

	port := &klausInterruptPort{cpu: rig.cpu}
	rig.bus.MapIO(klausInterruptIOLoc, klausInterruptIOLoc,
		func(addr uint32) uint32 { return uint32(port.Read()) },
		func(addr uint32, value uint32) { port.Write(uint8(value)) })

	rig.bus.Write8(NMI_VECTOR, uint8(klausInterruptNMITrap&0x00FF))
	rig.bus.Write8(NMI_VECTOR+1, uint8(klausInterruptNMITrap>>8))
	rig.bus.Write8(RESET_VECTOR, uint8(klausInterruptResTrap&0x00FF))
	rig.bus.Write8(RESET_VECTOR+1, uint8(klausInterruptResTrap>>8))
	rig.bus.Write8(IRQ_VECTOR, uint8(klausInterruptIRQTrap&0x00FF))
	rig.bus.Write8(IRQ_VECTOR+1, uint8(klausInterruptIRQTrap>>8))

	rig.cpu.Reset()
	rig.cpu.PC = klausInterruptEntry
	rig.cpu.SetRDYLine(true)

	done := make(chan struct{})
	go func() {
		rig.cpu.Execute()
		close(done)
	}()

	deadline := time.Now().Add(klausInterruptTimeout)
	for {
		pc := read6502PC(rig.cpu)
		if pc == successPC {
			stop6502CPU(rig.cpu)
			<-done
			return
		}
		if !read6502Running(rig.cpu) {
			stop6502CPU(rig.cpu)
			<-done
			t.Fatalf("CPU stopped before reaching PC=0x%04X (current PC=0x%04X)", successPC, pc)
		}
		if time.Now().After(deadline) {
			stop6502CPU(rig.cpu)
			<-done
			t.Fatalf("timeout waiting for PC=0x%04X (current PC=0x%04X, opcode=0x%02X, I_src=0x%02X, SP=0x%02X, SR=0x%02X, cycles=%d)",
				successPC, pc, rig.bus.memory[pc], rig.bus.memory[0x0203], rig.cpu.SP, rig.cpu.SR, read6502Cycles(rig.cpu))
		}
	}
}

type klausInterruptPort struct {
	cpu   *CPU_6502
	value uint8
}

func (p *klausInterruptPort) Read() uint8 {
	return p.value
}

func (p *klausInterruptPort) Write(value uint8) {
	prev := p.value
	p.value = value & klausInterruptIOFilter
	irqLine := (p.value & (1 << klausInterruptIRQBit)) != 0
	nmiLine := (p.value & (1 << klausInterruptNMIBit)) != 0

	p.cpu.irqPending.Store(irqLine)
	if nmiLine && (prev&(1<<klausInterruptNMIBit)) == 0 {
		p.cpu.nmiPending.Store(true)
	}
}

func readInterruptSuccessPC(t *testing.T) uint16 {
	t.Helper()

	value := os.Getenv(klausInterruptEnvTarget)
	if value == "" {
		t.Skipf("set %s to run the interrupt test (hex or decimal)", klausInterruptEnvTarget)
	}
	parsed, err := strconv.ParseUint(value, 0, 16)
	if err != nil || parsed > 0xFFFF {
		t.Fatalf("invalid %s value %q", klausInterruptEnvTarget, value)
	}
	return uint16(parsed)
}
