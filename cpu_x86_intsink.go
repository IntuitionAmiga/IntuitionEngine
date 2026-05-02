package main

type X86InterruptSink struct {
	cpu *CPU_X86
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
