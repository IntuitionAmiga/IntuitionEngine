package main

type IE32InterruptSink struct {
	cpu *CPU
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
