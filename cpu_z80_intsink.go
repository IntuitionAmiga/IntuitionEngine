package main

type Z80InterruptSink struct {
	cpu *CPU_Z80
}

func NewZ80InterruptSink(cpu *CPU_Z80) *Z80InterruptSink {
	return &Z80InterruptSink{cpu: cpu}
}

func (s *Z80InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.SetNMILine(false)
	s.cpu.SetNMILine(true)
	s.cpu.SetNMILine(false)
}
