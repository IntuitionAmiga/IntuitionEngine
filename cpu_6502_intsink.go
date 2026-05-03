package main

type CPU6502InterruptSink struct {
	cpu   *CPU_6502
	level interruptLevelState
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

func (s *CPU6502InterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.assert(mask) {
		s.cpu.SetIRQLine(true)
	}
}

func (s *CPU6502InterruptSink) Deassert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetIRQLine(s.level.deassert(mask))
}

func (s *CPU6502InterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetIRQLine(s.level.ack(mask))
}

func (s *CPU6502InterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetIRQLine(s.level.setMask(mask, masked))
}
