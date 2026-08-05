//go:build headless

package main

import (
	"bytes"
	"os"
	"testing"
)

// TestP65JITFixtureShadowParity compares a fixed execution prefix of reviewed
// 6502 showreel binaries. Keeping JIT blocks to one instruction makes the
// retirement checkpoint exact while still exercising native emission,
// dispatcher admission, and MMIO bailouts against the same machine state as
// the interpreter.
func TestP65JITFixtureShadowParity(t *testing.T) {
	if !jit6502Available {
		t.Skip("native 6502 JIT is unavailable on this target")
	}

	tests := []struct {
		name       string
		path       string
		checkpoint uint64
	}{
		{name: "rotozoomer", path: "sdk/examples/prebuilt/rotozoomer_65.ie65", checkpoint: 5_000},
		{name: "robocop", path: "sdk/examples/prebuilt/robocop_intro_65.ie65", checkpoint: 5_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			interp := newP65FixtureRunner(t, program)
			t.Cleanup(interp.close)
			jit := newP65FixtureRunner(t, program)
			t.Cleanup(jit.close)
			// These demos poll VIDEO_STATUS. Drive one explicit VSync edge into
			// each isolated rig rather than letting wall-clock VBlank fallback
			// decide a different branch in the sequential shadow executions.
			interp.rig.video.SignalVSync()
			jit.rig.video.SignalVSync()
			jit.runner.cpu.jitTestBlockLimit = 1
			// Execute one native checkpoint at a time. A fixture can return to a
			// cached block after it has changed its own code or page mapping; a
			// single long dispatcher run would then cross the requested boundary
			// before test control observes it.
			for retired := uint64(0); retired < tt.checkpoint; retired++ {
				interpPC := interp.runner.cpu.PC
				interpOpcode := interp.rig.bus.Read8(uint32(interpPC))
				jitPC := jit.runner.cpu.PC
				jitOpcode := jit.rig.bus.Read8(uint32(jitPC))
				interp.runner.cpu.Step()
				if !interp.runner.cpu.Running() {
					t.Fatalf("interpreter stopped after %d instructions at PC=$%04X", retired, interp.runner.cpu.PC)
				}
				jit.runner.cpu.jitTestStopAfter = retired + 1
				jit.runner.cpu.SetRunning(true)
				jit.runner.cpu.ExecuteJIT6502()
				if got := jit.runner.cpu.jitTestRetired; got != retired+1 {
					t.Fatalf("JIT retired %d instructions at checkpoint %d", got, retired+1)
				}
				if interp.runner.cpu.PC != jit.runner.cpu.PC || interp.runner.cpu.SP != jit.runner.cpu.SP ||
					interp.runner.cpu.A != jit.runner.cpu.A || interp.runner.cpu.X != jit.runner.cpu.X || interp.runner.cpu.Y != jit.runner.cpu.Y ||
					interp.runner.cpu.SR != jit.runner.cpu.SR || interp.runner.cpu.Cycles != jit.runner.cpu.Cycles {
					t.Fatalf("fixture diverged after instruction %d (interpreter $%04X:%02X, JIT $%04X:%02X): interpreter %s, JIT %s", retired+1, interpPC, interpOpcode, jitPC, jitOpcode, p65FixtureCPUState(interp.runner.cpu), p65FixtureCPUState(jit.runner.cpu))
				}
			}
			if got := jit.runner.cpu.jitTestRetired; got != tt.checkpoint {
				t.Fatalf("JIT retired %d instructions, want %d", got, tt.checkpoint)
			}

			assertP65FixtureStateEqual(t, interp, jit)
		})
	}
}

type p65FixtureRunner struct {
	rig    *eightBitRotoRig
	runner *CPU6502Runner
}

func newP65FixtureRunner(t *testing.T, program []byte) *p65FixtureRunner {
	t.Helper()
	rig := newEightBitRotoRig(t)
	// NewVideoChip owns a real-time refresh goroutine. Stop it for this shadow
	// rig: fixture execution supplies its own explicit VSync edge and must not
	// race an independent host frame counter between interpreter and JIT steps.
	if err := rig.video.Stop(); err != nil {
		t.Fatalf("stop fixture video clock: %v", err)
	}
	runner := NewCPU6502Runner(rig.bus, CPU6502Config{LoadAddr: 0x0800, Entry: 0x0800})
	runner.LoadProgramBytes(program)
	return &p65FixtureRunner{rig: rig, runner: runner}
}

func (r *p65FixtureRunner) close() {
	r.rig.sound.Stop()
}

func assertP65FixtureStateEqual(t *testing.T, want, got *p65FixtureRunner) {
	t.Helper()
	if want.runner.cpu.PC != got.runner.cpu.PC || want.runner.cpu.SP != got.runner.cpu.SP ||
		want.runner.cpu.A != got.runner.cpu.A || want.runner.cpu.X != got.runner.cpu.X || want.runner.cpu.Y != got.runner.cpu.Y ||
		want.runner.cpu.SR != got.runner.cpu.SR || want.runner.cpu.Cycles != got.runner.cpu.Cycles {
		t.Fatalf("fixture CPU state mismatch: want %s, got %s", p65FixtureCPUState(want.runner.cpu), p65FixtureCPUState(got.runner.cpu))
	}

	const p65AddressSpace = 1 << 16
	if !bytes.Equal(want.rig.bus.memory[:p65AddressSpace], got.rig.bus.memory[:p65AddressSpace]) {
		for address := 0; address < p65AddressSpace; address++ {
			if want.rig.bus.memory[address] != got.rig.bus.memory[address] {
				t.Fatalf("fixture RAM mismatch at $%04X: want $%02X, got $%02X", address, want.rig.bus.memory[address], got.rig.bus.memory[address])
			}
		}
		t.Fatal("fixture RAM mismatch")
	}

	// The reviewed fixtures configure the video device and write framebuffer
	// backing. Compare its serialised state as well as the visible frame so a
	// CPU-only match cannot hide a divergent device transaction.
	_, wantVideo, err := want.rig.video.DebugSnapshot()
	if err != nil {
		t.Fatalf("snapshot interpreter video: %v", err)
	}
	_, gotVideo, err := got.rig.video.DebugSnapshot()
	if err != nil {
		t.Fatalf("snapshot JIT video: %v", err)
	}
	if !bytes.Equal(wantVideo, gotVideo) {
		first := 0
		for first < len(wantVideo) && first < len(gotVideo) && wantVideo[first] == gotVideo[first] {
			first++
		}
		lo, hi := first-24, first+48
		if lo < 0 {
			lo = 0
		}
		if hi > len(wantVideo) {
			hi = len(wantVideo)
		}
		if hi > len(gotVideo) {
			hi = len(gotVideo)
		}
		t.Fatalf("fixture video-device state mismatch at snapshot byte %d (interpreter length=%d, JIT length=%d): interpreter %q, JIT %q", first, len(wantVideo), len(gotVideo), wantVideo[lo:hi], gotVideo[lo:hi])
	}
	if !bytes.Equal(want.rig.video.GetFrame(), got.rig.video.GetFrame()) {
		t.Fatal("fixture framebuffer mismatch")
	}
}
