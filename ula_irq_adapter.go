package main

type ulaIRQSink interface {
	AssertVBlankIRQ()
	DeassertVBlankIRQ()
}

type noopULAIRQAdapter struct{}

func (noopULAIRQAdapter) AssertVBlankIRQ()   {}
func (noopULAIRQAdapter) DeassertVBlankIRQ() {}

type z80ULAIRQAdapter struct {
	cpu *CPU_Z80
}

func newZ80ULAIRQAdapter(cpu *CPU_Z80) ulaIRQSink {
	if cpu == nil {
		return noopULAIRQAdapter{}
	}
	return z80ULAIRQAdapter{cpu: cpu}
}

func (a z80ULAIRQAdapter) AssertVBlankIRQ() {
	a.cpu.SetIRQLine(true)
}

func (a z80ULAIRQAdapter) DeassertVBlankIRQ() {
	a.cpu.SetIRQLine(false)
}

type c6502ULAIRQAdapter struct {
	cpu *CPU_6502
}

func new6502ULAIRQAdapter(cpu *CPU_6502) ulaIRQSink {
	if cpu == nil {
		return noopULAIRQAdapter{}
	}
	return c6502ULAIRQAdapter{cpu: cpu}
}

func (a c6502ULAIRQAdapter) AssertVBlankIRQ() {
	a.cpu.SetIRQLine(true)
}

func (a c6502ULAIRQAdapter) DeassertVBlankIRQ() {
	a.cpu.SetIRQLine(false)
}

type x86ULAIRQAdapter struct {
	cpu *CPU_X86
}

func newX86ULAIRQAdapter(cpu *CPU_X86) ulaIRQSink {
	if cpu == nil {
		return noopULAIRQAdapter{}
	}
	return x86ULAIRQAdapter{cpu: cpu}
}

func (a x86ULAIRQAdapter) AssertVBlankIRQ() {
	a.cpu.SetIRQ(true, IRQ_VECTOR_VBLANK)
}

func (a x86ULAIRQAdapter) DeassertVBlankIRQ() {
	a.cpu.ClearIRQ(IRQ_VECTOR_VBLANK)
}
