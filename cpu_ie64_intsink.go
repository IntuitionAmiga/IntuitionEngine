package main

type IE64InterruptSink struct {
	cpu *CPU64
}

func NewIE64InterruptSink(cpu *CPU64) *IE64InterruptSink {
	return &IE64InterruptSink{cpu: cpu}
}

func (s *IE64InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.handleExternalInterrupt()
}

func (cpu *CPU64) handleExternalInterrupt() {
	if cpu == nil || !cpu.interruptEnabled.Load() || cpu.inInterrupt.Load() {
		return
	}
	if cpu.mmuEnabled && cpu.intrVector != 0 {
		if !cpu.trapEntry() {
			return
		}
		cpu.faultPC = cpu.PC
		cpu.faultAddr = uint64(IntMaskBlitter)
		cpu.faultCause = FAULT_TIMER
		cpu.PC = cpu.intrVector
		return
	}
	cpu.inInterrupt.Store(true)
	cpu.regs[31] -= 8
	if !cpu.mmuStackWriteU64(cpu.regs[31], cpu.PC) {
		cpu.regs[31] += 8
		cpu.inInterrupt.Store(false)
		return
	}
	cpu.PC = cpu.interruptVector
}
