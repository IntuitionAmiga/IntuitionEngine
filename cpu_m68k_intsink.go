package main

type M68KInterruptSink struct {
	cpu *M68KCPU
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
