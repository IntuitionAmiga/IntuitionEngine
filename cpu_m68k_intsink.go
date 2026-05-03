package main

type M68KInterruptSink struct {
	cpu   *M68KCPU
	level interruptLevelState
}

func NewM68KInterruptSink(cpu *M68KCPU) *M68KInterruptSink {
	return &M68KInterruptSink{cpu: cpu}
}

func (s *M68KInterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.AssertInterrupt(7)
}

func (s *M68KInterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.assert(mask) {
		s.cpu.AssertInterrupt(7)
	}
}

func (s *M68KInterruptSink) Deassert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.deassert(mask) {
		s.cpu.AssertInterrupt(7)
		return
	}
	s.cpu.pendingInterrupt.Store(s.cpu.pendingInterrupt.Load() &^ (1 << 7))
}

func (s *M68KInterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.ack(mask) {
		s.cpu.AssertInterrupt(7)
	}
}

func (s *M68KInterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.setMask(mask, masked) {
		s.cpu.AssertInterrupt(7)
	} else {
		s.cpu.pendingInterrupt.Store(s.cpu.pendingInterrupt.Load() &^ (1 << 7))
	}
}
