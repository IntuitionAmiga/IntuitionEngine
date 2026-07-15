//go:build arm64 && (linux || windows || darwin)

package main

import "testing"

// The parity tests in jit_ie64_fpsr_sink_test.go prove a sunk CC update reaches
// FPSR with the right value. They cannot prove it was ever sunk: emitting the
// update inline is also correct, just slower, so they stay green if the emitter
// quietly ignores fpsrCCSink and forfeits the optimisation.
//
// These tests pin the wiring itself.

// TestARM64FPCCUpdateHonoursSink covers the three outcomes of the CC gate.
func TestARM64FPCCUpdateHonoursSink(t *testing.T) {
	tests := []struct {
		name        string
		ji          JITInstr
		wantEmit    bool
		wantPending bool
	}{
		{
			name:     "dead: dropped entirely",
			ji:       JITInstr{opcode: OP_FADD, rd: 3, fpsrCCDead: true},
			wantEmit: false,
		},
		{
			name:        "sink: deferred to the exit funnel",
			ji:          JITInstr{opcode: OP_FADD, rd: 3, fpsrCCSink: true},
			wantEmit:    false,
			wantPending: true,
		},
		{
			name:     "neither: emitted in place",
			ji:       JITInstr{opcode: OP_FADD, rd: 3},
			wantEmit: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cb := NewCodeBuffer(1024)
			emitFPCondCodesARM64(cb, &tc.ji)

			if got := cb.Len() > 0; got != tc.wantEmit {
				t.Errorf("emitted code = %v, want %v", got, tc.wantEmit)
			}
			if cb.pendingFPCC.valid != tc.wantPending {
				t.Fatalf("pending = %v, want %v", cb.pendingFPCC.valid, tc.wantPending)
			}
			if tc.wantPending && cb.pendingFPCC.reg != tc.ji.rd {
				t.Errorf("pending reg = F%d, want F%d", cb.pendingFPCC.reg, tc.ji.rd)
			}
		})
	}
}

// TestARM64FPCCInPlaceClearsPending pins that an in-place update retires any
// outstanding sunk one. Leaving it pending would let the funnel overwrite this
// newer CC with the older writer's value.
func TestARM64FPCCInPlaceClearsPending(t *testing.T) {
	cb := NewCodeBuffer(1024)
	cb.pendingFPCC = ie64FPCCPending{valid: true, reg: 7}

	emitFPCondCodesARM64(cb, &JITInstr{opcode: OP_FADD, rd: 3})

	if cb.pendingFPCC.valid {
		t.Errorf("in-place CC update left F%d pending; the funnel would overwrite it", cb.pendingFPCC.reg)
	}
}

// TestARM64EpilogueMaterializesSunkCC pins that emitEpilogue is wired to the
// pending slot. emitEpilogue is arm64's only exit funnel, so a sunk update that
// it does not materialise is simply lost.
func TestARM64EpilogueMaterializesSunkCC(t *testing.T) {
	bare := NewCodeBuffer(1024)
	emitEpilogue(bare, 0, 0)

	sunk := NewCodeBuffer(1024)
	sunk.pendingFPCC = ie64FPCCPending{valid: true, reg: 3}
	emitEpilogue(sunk, 0, 0)

	if sunk.Len() <= bare.Len() {
		t.Errorf("epilogue with a pending CC emitted %d bytes, without one %d: "+
			"the sunk update is not being materialised", sunk.Len(), bare.Len())
	}
}
