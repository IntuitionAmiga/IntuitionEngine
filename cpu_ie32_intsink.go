package main

type IE32InterruptSink struct {
	cpu   *CPU
	level interruptLevelState
}

func NewIE32InterruptSink(cpu *CPU) *IE32InterruptSink {
	return &IE32InterruptSink{cpu: cpu}
}

func (s *IE32InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.handleInterrupt()
}

func (s *IE32InterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.assert(mask) {
		s.cpu.handleInterrupt()
	}
}

func (s *IE32InterruptSink) Deassert(mask InterruptMask) {
	if s == nil {
		return
	}
	s.level.deassert(mask)
}

func (s *IE32InterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.ack(mask) {
		s.cpu.handleInterrupt()
	}
}

func (s *IE32InterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.setMask(mask, masked) {
		s.cpu.handleInterrupt()
	}
}
