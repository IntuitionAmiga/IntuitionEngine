package main

type CPU6502InterruptSink struct {
	cpu *CPU_6502
}

func NewCPU6502InterruptSink(cpu *CPU_6502) *CPU6502InterruptSink {
	return &CPU6502InterruptSink{cpu: cpu}
}

func (s *CPU6502InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetIRQLine(true)
}
