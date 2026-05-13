package main

import "testing"

func faultTestService(kind string) (*DebugFaultService, <-chan BreakpointEvent) {
	svc := NewDebugFaultService()
	ch := make(chan BreakpointEvent, 2)
	svc.RegisterCPU(0, ch)
	svc.EnableKind(kind)
	return svc, ch
}

func expectFaultEvent(t *testing.T, ch <-chan BreakpointEvent, kind string) BreakpointEvent {
	t.Helper()
	select {
	case ev := <-ch:
		if !ev.IsFault || ev.FaultKind != kind {
			t.Fatalf("event = %+v, want fault kind %s", ev, kind)
		}
		return ev
	default:
		t.Fatalf("no fault event for %s", kind)
		return BreakpointEvent{}
	}
}

func TestFaultCommand_DefaultOffAndSelectiveEnable(t *testing.T) {
	mon, _ := newTestMonitor()
	_, out := mon.ExecuteCommandResult("fault list")
	if len(out) == 0 || out[len(out)-1].Text != "Fault interception: off" {
		t.Fatalf("fault list output = %#v, want off", out)
	}
	mon.ExecuteCommand("fault break ie64.priv")
	if !mon.faults.Enabled("ie64.priv") {
		t.Fatal("ie64.priv fault kind was not enabled")
	}
	if mon.faults.Enabled("m68k.illegal") {
		t.Fatal("unrequested fault kind should remain disabled")
	}
	mon.ExecuteCommand("fault off")
	if mon.faults.Enabled("ie64.priv") {
		t.Fatal("fault off did not clear selective kind")
	}
}

func TestFault_DefaultOff_GuestHandlerRuns_IE64(t *testing.T) {
	cpu := NewCPU64(NewMachineBus())
	cpu.PC = PROG_START
	cpu.trapVector = 0x8000
	cpu.trapFault(FAULT_PRIV, 0)
	if cpu.PC != 0x8000 {
		t.Fatalf("PC = %#x, want trap vector when fault interception is off", cpu.PC)
	}
}

func TestFault_InterceptsCentralFaultPaths_AllCPUs(t *testing.T) {
	t.Run("IE64", func(t *testing.T) {
		svc, ch := faultTestService("ie64.priv")
		cpu := NewCPU64(NewMachineBus())
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.running.Store(true)
		cpu.PC = PROG_START
		cpu.trapVector = 0x8000
		cpu.trapFault(FAULT_PRIV, 0)
		ev := expectFaultEvent(t, ch, "ie64.priv")
		if ev.Address != PROG_START || cpu.PC != PROG_START {
			t.Fatalf("event/cpu after intercept = %+v PC=%#x, want pre-handler PC", ev, cpu.PC)
		}
	})

	t.Run("IE32", func(t *testing.T) {
		svc, ch := faultTestService("ie32.invalid-opcode")
		cpu := NewCPU(NewMachineBus())
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.memory[0] = 0xFF
		cpu.StepOne()
		expectFaultEvent(t, ch, "ie32.invalid-opcode")
	})

	t.Run("M68K", func(t *testing.T) {
		svc, ch := faultTestService("m68k.illegal")
		cpu := NewM68KCPU(NewMachineBus())
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.PC = M68K_ENTRY_POINT
		cpu.lastExecPC = M68K_ENTRY_POINT
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		ev := expectFaultEvent(t, ch, "m68k.illegal")
		if ev.Address != M68K_ENTRY_POINT {
			t.Fatalf("event = %+v, want faulting PC %#x", ev, uint64(M68K_ENTRY_POINT))
		}
	})

	t.Run("Z80", func(t *testing.T) {
		svc, ch := faultTestService("z80.rst38")
		cpu := NewCPU_Z80(NewZ80BusAdapter(NewMachineBus()))
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.PC = 0x1234
		cpu.opRST(0x38)
		ev := expectFaultEvent(t, ch, "z80.rst38")
		if ev.Address != 0x1234 || cpu.PC != 0x1234 {
			t.Fatalf("event/cpu after intercept = %+v PC=%#x, want pre-RST PC", ev, cpu.PC)
		}
	})

	t.Run("6502", func(t *testing.T) {
		svc, ch := faultTestService("6502.brk")
		cpu := NewCPU_6502(NewMachineBus())
		cpu.Debug = true
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.PC = 0x0600
		cpu.memory.Write(0x0600, 0x00)
		cpu.running.Store(true)
		cpu.Step()
		ev := expectFaultEvent(t, ch, "6502.brk")
		if ev.Address != 0x0600 || cpu.PC != 0x0600 {
			t.Fatalf("event/cpu after intercept = %+v PC=%#x, want BRK opcode PC preserved", ev, cpu.PC)
		}
	})

	t.Run("6502 fast", func(t *testing.T) {
		svc, ch := faultTestService("6502.brk")
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		cpu.SetRDYLine(true)
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.PC = 0x0600
		bus.Write8(0x0600, 0x00)
		cpu.running.Store(true)
		cpu.Execute()
		ev := expectFaultEvent(t, ch, "6502.brk")
		if ev.Address != 0x0600 || cpu.PC != 0x0600 {
			t.Fatalf("fast event/cpu after intercept = %+v PC=%#x, want BRK opcode PC preserved", ev, cpu.PC)
		}
	})

	t.Run("X86", func(t *testing.T) {
		svc, ch := faultTestService("x86.ud")
		cpu := NewCPU_X86(NewX86BusAdapter(NewMachineBus()))
		cpu.debugFaults = svc
		cpu.debugCPUID = 0
		cpu.EIP = 0x1234
		cpu.debugFault("x86.ud", 0x1234, "opcode=$0F")
		ev := expectFaultEvent(t, ch, "x86.ud")
		if ev.Address != 0x1234 {
			t.Fatalf("event = %+v, want EIP", ev)
		}
	})
}
