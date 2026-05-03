package main

type Z80InterruptSink struct {
	cpu   *CPU_Z80
	level interruptLevelState
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

func (s *Z80InterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.assert(mask) {
		s.reedgeHeldNMI()
	}
}

func (s *Z80InterruptSink) Deassert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.driveLevel(s.level.deassert(mask))
}

func (s *Z80InterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	if s.level.ack(mask) {
		s.reedgeHeldNMI()
	}
}

func (s *Z80InterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	s.driveLevel(s.level.setMask(mask, masked))
}

func (s *Z80InterruptSink) driveLevel(pending bool) {
	if pending {
		s.reedgeHeldNMI()
		return
	}
	s.cpu.SetNMILine(false)
}

func (s *Z80InterruptSink) reedgeHeldNMI() {
	s.cpu.SetNMILine(false)
	s.cpu.SetNMILine(true)
}
