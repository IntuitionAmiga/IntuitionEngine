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
		name       string
		path       string
		checkpoint uint64
		voodoo     bool
	}{
		{name: "rotozoomer", path: "sdk/examples/prebuilt/rotozoomer.iex", checkpoint: 50_000},
		{name: "robocop", path: "sdk/examples/prebuilt/robocop_intro.iex", checkpoint: 50_000},
		{name: "voodoo-mega-demo", path: "sdk/examples/prebuilt/voodoo_mega_demo.iex", checkpoint: 10_000, voodoo: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			interpreter := newIE32FixtureCPUWithVoodoo(t, program, tc.voodoo)
			jit := newIE32FixtureCPUWithVoodoo(t, program, tc.voodoo)
			for retired := uint64(0); retired < tc.checkpoint; retired++ {
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
			interpreter.cpu.InstructionCount = tc.checkpoint
			interpreter.cpu.running.Store(false)
			jit.cpu.jit.testStopAfter = tc.checkpoint
			jit.cpu.jit.testExactRetirement = true
			jit.cpu.Execute()
			if got := jit.cpu.jit.testRetired; got != tc.checkpoint {
				t.Fatalf("JIT retired %d instructions, want %d", got, tc.checkpoint)
			}
			assertIE32FixtureStateEqual(t, interpreter, jit)
		})
	}
}

// TestIE32JITVoodooCheckpointParity narrows the Voodoo initialisation shadow
// check to bounded retirement windows. It catches a generated block that
// retires the right total but leaves a different architectural state.
func TestIE32JITVoodooCheckpointParity(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	interpreter := newIE32FixtureCPUWithVoodoo(t, program, true)
	jit := newIE32FixtureCPUWithVoodoo(t, program, true)
	const checkpoint = uint64(10_000)
	const window = uint64(32)
	jit.cpu.jit.testExactRetirement = true
	for retired := uint64(0); retired < checkpoint; {
		limit := retired + window
		if limit > checkpoint {
			limit = checkpoint
		}
		for retired < limit {
			if interpreter.cpu.StepOne() == 0 || !interpreter.cpu.running.Load() {
				t.Fatalf("interpreter stopped at instruction %d PC=%#x", retired, interpreter.cpu.PC)
			}
			interpreter.cpu.InstructionCount++
			retired++
		}
		jit.cpu.jit.testStopAfter = limit
		jit.cpu.running.Store(true)
		jit.cpu.Execute()
		if got := jit.cpu.jit.testRetired; got != limit {
			t.Fatalf("JIT retired %d instructions, want %d", got, limit)
		}
		if interpreter.cpu.PC != jit.cpu.PC || interpreter.cpu.SP != jit.cpu.SP || interpreter.cpu.A != jit.cpu.A || interpreter.cpu.X != jit.cpu.X || interpreter.cpu.Y != jit.cpu.Y || interpreter.cpu.B != jit.cpu.B {
			t.Fatalf("window ending at %d diverged: interpreter PC=%#x SP=%#x A=%#x X=%#x Y=%#x B=%#x; JIT PC=%#x SP=%#x A=%#x X=%#x Y=%#x B=%#x", limit, interpreter.cpu.PC, interpreter.cpu.SP, interpreter.cpu.A, interpreter.cpu.X, interpreter.cpu.Y, interpreter.cpu.B, jit.cpu.PC, jit.cpu.SP, jit.cpu.A, jit.cpu.X, jit.cpu.Y, jit.cpu.B)
		}
	}
}

// TestIE32JITVoodooFrameLoopParity reaches the animated part of the shipped
// Voodoo demo with ordinary retained-cache execution. Initialisation-only
// checkpoints cannot detect stale per-frame state such as star positions or
// the sine-driven scroll offset.
func TestIE32JITVoodooFrameLoopParity(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	interpreter := newIE32FixtureCPUWithVoodoo(t, program, true)
	jit := newIE32FixtureCPUWithVoodoo(t, program, true)
	const requestedCheckpoint = uint64(250_000)
	jit.cpu.jit.testStopAfter = requestedCheckpoint
	jit.cpu.Execute()
	checkpoint := jit.cpu.jit.testRetired
	if checkpoint < requestedCheckpoint {
		t.Fatalf("JIT retired %d instructions, want at least %d", checkpoint, requestedCheckpoint)
	}
	for retired := uint64(0); retired < checkpoint; retired++ {
		if interpreter.cpu.StepOne() == 0 || !interpreter.cpu.running.Load() {
			t.Fatalf("interpreter stopped at instruction %d PC=%#x", retired, interpreter.cpu.PC)
		}
		interpreter.cpu.InstructionCount++
	}
	interpreter.cpu.InstructionCount = checkpoint
	interpreter.cpu.running.Store(false)
	assertIE32FixtureStateEqual(t, interpreter, jit)
}

func TestIE32JITVoodooFrameLoopExactCheckpointParity(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	interpreter := newIE32FixtureCPUWithVoodoo(t, program, true)
	jit := newIE32FixtureCPUWithVoodoo(t, program, true)
	const checkpoint = uint64(250_000)
	const window = uint64(64)
	jit.cpu.jit.testExactRetirement = true
	for retired := uint64(0); retired < checkpoint; {
		limit := retired + window
		if limit > checkpoint {
			limit = checkpoint
		}
		for retired < limit {
			if interpreter.cpu.StepOne() == 0 || !interpreter.cpu.running.Load() {
				t.Fatalf("interpreter stopped at instruction %d PC=%#x", retired, interpreter.cpu.PC)
			}
			interpreter.cpu.InstructionCount++
			retired++
		}
		jit.cpu.jit.testStopAfter = limit
		jit.cpu.running.Store(true)
		jit.cpu.Execute()
		if got := jit.cpu.jit.testRetired; got != limit {
			t.Fatalf("JIT retired %d instructions, want %d", got, limit)
		}
		if interpreter.cpu.PC != jit.cpu.PC || interpreter.cpu.SP != jit.cpu.SP || interpreter.cpu.A != jit.cpu.A || interpreter.cpu.X != jit.cpu.X || interpreter.cpu.Y != jit.cpu.Y || interpreter.cpu.B != jit.cpu.B {
			t.Fatalf("window ending at %d diverged: interpreter PC=%#x SP=%#x A=%#x X=%#x Y=%#x B=%#x; JIT PC=%#x SP=%#x A=%#x X=%#x Y=%#x B=%#x", limit, interpreter.cpu.PC, interpreter.cpu.SP, interpreter.cpu.A, interpreter.cpu.X, interpreter.cpu.Y, interpreter.cpu.B, jit.cpu.PC, jit.cpu.SP, jit.cpu.A, jit.cpu.X, jit.cpu.Y, jit.cpu.B)
		}
	}
}

type ie32FixtureCPU struct {
	cpu    *CPU
	video  *VideoChip
	voodoo *VoodooEngine
}

func newIE32FixtureCPU(t *testing.T, program []byte) ie32FixtureCPU {
	return newIE32FixtureCPUWithVoodoo(t, program, false)
}

func newIE32FixtureCPUWithVoodoo(t *testing.T, program []byte, mapVoodoo bool) ie32FixtureCPU {
	t.Helper()
	bus := NewMachineBus()
	var voodoo *VoodooEngine
	if mapVoodoo {
		var err error
		voodoo, err = NewVoodooEngine(bus)
		if err != nil {
			t.Fatalf("create fixture Voodoo: %v", err)
		}
		t.Cleanup(voodoo.Destroy)
		bus.MapIO(VOODOO_BASE, VOODOO_END, voodoo.HandleRead, voodoo.HandleWrite)
		bus.MapIOByteRead(VOODOO_BASE, VOODOO_END, voodoo.HandleRead8)
		bus.MapIOByte(VOODOO_BASE, VOODOO_END, voodoo.HandleWrite8)
		bus.MapIO64(VOODOO_BASE, VOODOO_END, voodoo.HandleRead64, voodoo.HandleWrite64)
	}
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
	return ie32FixtureCPU{cpu: cpu, video: video, voodoo: voodoo}
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
	if (want.voodoo == nil) != (got.voodoo == nil) {
		t.Fatal("fixture Voodoo mapping differs")
	}
	if want.voodoo != nil {
		wantFrame, gotFrame := want.voodoo.GetFrame(), got.voodoo.GetFrame()
		if !bytes.Equal(wantFrame, gotFrame) {
			t.Fatalf("fixture Voodoo framebuffer mismatch: interpreter=%x JIT=%x", sha256.Sum256(wantFrame), sha256.Sum256(gotFrame))
		}
	}
}
