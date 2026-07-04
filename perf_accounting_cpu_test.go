//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"time"
)

func TestPerfAcct_IE64JITSplit(t *testing.T) {
	withPerfAcct(t, true, func() {
		cpu := runJITProgram(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))
		snap := cpu.perfAcct.Snapshot()
		if snap.Instructions != 2 {
			t.Fatalf("IE64 instructions = %d, want 2", snap.Instructions)
		}
		if snap.JitNs <= 0 {
			t.Fatalf("IE64 JitNs = %d, want > 0", snap.JitNs)
		}
		if snap.JitNs+snap.InterpNs <= 0 {
			t.Fatalf("IE64 timing was not recorded: %+v", snap)
		}
	})
}

func TestPerfAcct_M68KJITSplit(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	withPerfAcct(t, true, func() {
		const startPC = uint32(0x1000)
		cpu := NewM68KCPU(NewMachineBus())
		cpu.PC = startPC
		cpu.SR = M68K_SR_S | 0x0700
		cpu.AddrRegs[7] = M68K_STACK_START
		cpu.SSP = M68K_STACK_START
		cpu.USP = M68K_STACK_START
		cpu.stackLowerBound = 0x00002000
		cpu.stackUpperBound = M68K_MEMORY_SIZE
		cpu.m68kJitEnabled = true
		cpu.m68kJitForceNative = true
		perfAcctWriteM68KWords(cpu, startPC,
			0x7001,         // MOVEQ #1,D0
			0x5280,         // ADDQ.L #1,D0
			0x4E72, 0x2700, // STOP #$2700
		)
		cpu.running.Store(true)

		done := make(chan struct{})
		go func() {
			cpu.M68KExecuteJIT()
			close(done)
		}()
		deadline := time.After(2 * time.Second)
		for !cpu.stopped.Load() {
			select {
			case <-done:
				t.Fatalf("M68K JIT exited before STOP: pc=%08X", cpu.PC)
			case <-deadline:
				cpu.running.Store(false)
				<-done
				t.Fatalf("M68K JIT timed out: pc=%08X", cpu.PC)
			default:
				time.Sleep(time.Millisecond)
			}
		}
		cpu.running.Store(false)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("M68K JIT did not stop after running=false")
		}

		snap := cpu.perfAcct.Snapshot()
		if snap.Instructions != 3 {
			t.Fatalf("M68K instructions = %d, want 3", snap.Instructions)
		}
		if snap.JitNs <= 0 || snap.InterpNs <= 0 {
			t.Fatalf("M68K split not recorded: %+v", snap)
		}
	})
}

func TestPerfAcct_Z80JITSplit(t *testing.T) {
	withPerfAcct(t, true, func() {
		bus := NewMachineBus()
		adapter := NewZ80BusAdapter(bus)
		cpu := NewCPU_Z80(adapter)
		cpu.jitEnabled = true
		bus.Write8(0x0000, 0x00) // NOP
		bus.Write8(0x0001, 0x00) // NOP
		bus.Write8(0x0002, 0x76) // HALT
		cpu.PC = 0
		cpu.SP = 0x1FFE
		cpu.SetRunning(true)

		cpu.ExecuteJITZ80()

		snap := cpu.perfAcct.Snapshot()
		if snap.Instructions != 3 {
			t.Fatalf("Z80 instructions = %d, want 3", snap.Instructions)
		}
		if snap.JitNs <= 0 || snap.InterpNs <= 0 {
			t.Fatalf("Z80 split not recorded: %+v", snap)
		}
	})
}

func TestPerfAcct_6502JITSplit(t *testing.T) {
	withPerfAcct(t, true, func() {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		cpu.SetRDYLine(true)
		bus.Write8(0x0600, 0xEA) // NOP
		bus.Write8(0x0601, 0xEA) // NOP
		bus.Write8(0x0602, 0x02) // JAM
		cpu.PC = 0x0600
		cpu.SetRunning(true)
		cpu.jitEnabled = true

		cpu.ExecuteJIT6502()

		snap := cpu.perfAcct.Snapshot()
		if snap.Instructions != 3 {
			t.Fatalf("6502 instructions = %d, want 3", snap.Instructions)
		}
		if snap.JitNs <= 0 || snap.InterpNs <= 0 {
			t.Fatalf("6502 split not recorded: %+v", snap)
		}
	})
}

func perfAcctWriteM68KWords(cpu *M68KCPU, pc uint32, words ...uint16) {
	for _, word := range words {
		cpu.Write16(pc, word)
		pc += 2
	}
}
