//go:build headless

package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"testing"
)

// TestIE32JITFixtureShadowParity runs the shipped IE32 showreel programs in
// isolated machines to a fixed guest-instruction checkpoint. The JIT control
// boundary is retired-instruction based, not wall-clock based, so the shadow
// comparison remains useful on x64, QEMU ARM64, and wasm.
func TestIE32JITFixtureShadowParity(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "rotozoomer", path: "sdk/examples/prebuilt/rotozoomer.iex"},
		{name: "robocop", path: "sdk/examples/prebuilt/robocop_intro.iex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			interpreter := newIE32FixtureCPU(t, program)
			jit := newIE32FixtureCPU(t, program)
			const checkpoint = uint64(50_000)
			for retired := uint64(0); retired < checkpoint; retired++ {
				if interpreter.cpu.StepOne() == 0 || !interpreter.cpu.running.Load() {
					t.Fatalf("interpreter stopped at instruction %d PC=%#x", retired, interpreter.cpu.PC)
				}
				// StepOne intentionally leaves retirement accounting to its caller.
				// Publish it here so VIDEO_STATUS polling observes the same
				// guest-progress-driven VSync phase as the JIT dispatcher.
				interpreter.cpu.InstructionCount++
			}
			// Match the deterministic checkpoint controller used by the JIT.
			// StepOne intentionally does not maintain the Execute retirement
			// counter, so publish the same completed checkpoint state before
			// comparing lifecycle and retired-instruction state.
			interpreter.cpu.InstructionCount = checkpoint
			interpreter.cpu.running.Store(false)
			jit.cpu.jit.testStopAfter = checkpoint
			jit.cpu.jit.testExactRetirement = true
			jit.cpu.Execute()
			if got := jit.cpu.jit.testRetired; got != checkpoint {
				t.Fatalf("JIT retired %d instructions, want %d", got, checkpoint)
			}
			assertIE32FixtureStateEqual(t, interpreter, jit)
		})
	}
}

type ie32FixtureCPU struct {
	cpu   *CPU
	video *VideoChip
}

func newIE32FixtureCPU(t *testing.T, program []byte) ie32FixtureCPU {
	t.Helper()
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("create fixture video: %v", err)
	}
	if err := video.Stop(); err != nil {
		t.Fatalf("stop fixture video clock: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	video.SetBigEndianMode(false)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	cpu := NewCPU(bus)
	bus.SetVideoStatusReader(func(uint32) uint32 {
		const frameInstructions = uint64(10_000)
		vblank := cpu.InstructionCount%frameInstructions >= frameInstructions*9/10
		video.setVBlank(vblank)
		if vblank {
			return videoStatusVBlank
		}
		return 0
	})
	cpu.LoadProgramBytes(program)
	cpu.running.Store(true)
	return ie32FixtureCPU{cpu: cpu, video: video}
}

func assertIE32FixtureStateEqual(t *testing.T, want, got ie32FixtureCPU) {
	t.Helper()
	wantCPU, gotCPU := want.cpu, got.cpu
	if wantCPU.PC != gotCPU.PC || wantCPU.SP != gotCPU.SP || wantCPU.A != gotCPU.A || wantCPU.X != gotCPU.X || wantCPU.Y != gotCPU.Y || wantCPU.Z != gotCPU.Z ||
		wantCPU.B != gotCPU.B || wantCPU.C != gotCPU.C || wantCPU.D != gotCPU.D || wantCPU.E != gotCPU.E || wantCPU.F != gotCPU.F || wantCPU.G != gotCPU.G ||
		wantCPU.H != gotCPU.H || wantCPU.S != gotCPU.S || wantCPU.T != gotCPU.T || wantCPU.U != gotCPU.U || wantCPU.V != gotCPU.V || wantCPU.W != gotCPU.W ||
		wantCPU.interruptEnabled.Load() != gotCPU.interruptEnabled.Load() || wantCPU.inInterrupt.Load() != gotCPU.inInterrupt.Load() ||
		wantCPU.timerEnabled.Load() != gotCPU.timerEnabled.Load() || wantCPU.timerCount.Load() != gotCPU.timerCount.Load() ||
		wantCPU.timerPeriod.Load() != gotCPU.timerPeriod.Load() || wantCPU.timerState.Load() != gotCPU.timerState.Load() || wantCPU.cycleCounter != gotCPU.cycleCounter ||
		wantCPU.InstructionCount != gotCPU.InstructionCount || wantCPU.running.Load() != gotCPU.running.Load() {
		t.Fatalf("fixture CPU state mismatch: interpreter PC=%#x SP=%#x A=%#x, JIT PC=%#x SP=%#x A=%#x", wantCPU.PC, wantCPU.SP, wantCPU.A, gotCPU.PC, gotCPU.SP, gotCPU.A)
	}
	if !bytes.Equal(wantCPU.memory[:STACK_START], gotCPU.memory[:STACK_START]) {
		for i := 0; i < STACK_START; i++ {
			if wantCPU.memory[i] != gotCPU.memory[i] {
				t.Fatalf("fixture RAM mismatch at %#x: interpreter=%#x JIT=%#x", i, wantCPU.memory[i], gotCPU.memory[i])
			}
		}
		t.Fatal("fixture RAM mismatch")
	}
	_, wantVideo, err := want.video.DebugSnapshot()
	if err != nil {
		t.Fatalf("snapshot interpreter video: %v", err)
	}
	_, gotVideo, err := got.video.DebugSnapshot()
	if err != nil {
		t.Fatalf("snapshot JIT video: %v", err)
	}
	if !bytes.Equal(wantVideo, gotVideo) {
		t.Fatalf("fixture video-device state mismatch: interpreter=%x JIT=%x", sha256.Sum256(wantVideo), sha256.Sum256(gotVideo))
	}
	if wantFrame, gotFrame := want.video.GetFrame(), got.video.GetFrame(); !bytes.Equal(wantFrame, gotFrame) {
		t.Fatalf("fixture framebuffer mismatch: interpreter=%x JIT=%x", sha256.Sum256(wantFrame), sha256.Sum256(gotFrame))
	}
}
