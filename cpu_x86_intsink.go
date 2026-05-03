package main

type X86InterruptSink struct {
	cpu   *CPU_X86
	level interruptLevelState
}

func NewX86InterruptSink(cpu *CPU_X86) *X86InterruptSink {
	return &X86InterruptSink{cpu: cpu}
}

func (s *X86InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetNMI(true)
}

func (s *X86InterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.assert(mask) {
		s.cpu.SetNMI(true)
	}
}

func (s *X86InterruptSink) Deassert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetNMI(s.level.deassert(mask))
}

func (s *X86InterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.ack(mask) {
		s.cpu.SetNMI(true)
	}
}

func (s *X86InterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetNMI(s.level.setMask(mask, masked))
}
