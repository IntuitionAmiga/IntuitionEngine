// jit_x86_adc_sbb_classify_test.go - regression cover for ADC/SBB
// flag-producer classification.
//
// Reviewer P2 fix on the JIT-summit branch: x86InstrFlagOpKind
// previously returned x86FlagOpNone for ADC AL,imm8 (0x14), ADC
// EAX,imm32 (0x15), SBB AL,imm8 (0x1C), SBB EAX,imm32 (0x1D), and the
// r/m forms 0x10-0x13 / 0x18-0x1B. Those instructions DO modify every
// visible flag (CF,PF,AF,ZF,SF,OF), so the missing classification
// caused the post-instruction capture to skip them — the epilogue then
// merged stale guest EFLAGS and any downstream flag consumer saw
// pre-ADC/SBB flag state.
//
// This test pins the classification so a future "trim flag-capture
// emit" patch cannot silently drop ADC/SBB back into x86FlagOpNone.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestX86InstrFlagOpKind_ADC_SBB_AreArith(t *testing.T) {
	cases := []struct {
		op   uint16
		name string
	}{
		// ADC r/m forms.
		{0x10, "ADC Eb,Gb"},
		{0x11, "ADC Ev,Gv"},
		{0x12, "ADC Gb,Eb"},
		{0x13, "ADC Gv,Ev"},
		// ADC AL/EAX,imm.
		{0x14, "ADC AL,Ib"},
		{0x15, "ADC EAX,Iv"},
		// SBB r/m forms.
		{0x18, "SBB Eb,Gb"},
		{0x19, "SBB Ev,Gv"},
		{0x1A, "SBB Gb,Eb"},
		{0x1B, "SBB Gv,Ev"},
		// SBB AL/EAX,imm.
		{0x1C, "SBB AL,Ib"},
		{0x1D, "SBB EAX,Iv"},
	}
	for _, c := range cases {
		got := x86InstrFlagOpKind(c.op, 0)
		if got != x86FlagOpArith {
			t.Errorf("x86InstrFlagOpKind(%#x %q) = %v, want x86FlagOpArith. "+
				"ADC/SBB define every visible flag (CF/PF/AF/ZF/SF/OF); without "+
				"x86FlagOpArith the post-instr capture skips them and the epilogue "+
				"merges stale guest EFLAGS into *cpu.Flags.",
				c.op, c.name, got)
		}
	}
}
