package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const videoStatusVBlankBit = 1 << 1

func newM68KVideoMMIORig(t *testing.T) (*M68KCPU, *VideoChip, *MachineBus) {
	t.Helper()

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)

	cpu := NewM68KCPU(bus)
	cpu.SR = M68K_SR_S
	cpu.PC = M68K_ENTRY_POINT
	cpu.AddrRegs[7] = M68K_STACK_START

	return cpu, video, bus
}

func runM68KInstructions(cpu *M68KCPU, count int) {
	for range count {
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

func TestM68K_MoveImmediateAbsoluteLong_WritesVideoMMIO(t *testing.T) {
	cpu, video, _ := newM68KVideoMMIORig(t)

	// move.l #1,$000F0000
	// move.l #0,$000F0004
	opcodes := []uint16{
		0x23FC, 0x0000, 0x0001, 0x000F, 0x0000,
		0x23FC, 0x0000, 0x0000, 0x000F, 0x0004,
	}
	for i, op := range opcodes {
		addr := uint32(M68K_ENTRY_POINT + i*2)
		cpu.memory[addr] = byte(op >> 8)
		cpu.memory[addr+1] = byte(op)
	}

	runM68KInstructions(cpu, 2)

	if got := video.HandleRead(VIDEO_CTRL); got != 1 {
		t.Fatalf("VIDEO_CTRL = %d, want 1", got)
	}
	if got := video.HandleRead(VIDEO_MODE); got != 0 {
		t.Fatalf("VIDEO_MODE = %d, want 0", got)
	}
}

func TestM68K_VideoStatusVBlankPollBecomesVisible(t *testing.T) {
	cpu, video, _ := newM68KVideoMMIORig(t)

	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = video.Stop()
	})

	cpu.Write32(VIDEO_CTRL, 1)
	cpu.Write32(VIDEO_MODE, 0)

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.Read32(VIDEO_STATUS)&videoStatusVBlankBit != 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("VIDEO_STATUS never exposed STATUS_VBLANK to M68K polling")
}

func TestM68K_RotozoomerBinary_ReachesWaitVSyncWithVideoEnabled(t *testing.T) {
	cpu, video, _ := newM68KVideoMMIORig(t)

	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = video.Stop()
	})

	programPath := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_68k.ie68")
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", programPath, err)
	}
	cpu.LoadProgramBytes(program)

	// wait_vsync's phase-1 poll top in the current prebuilt binary:
	//   0x1078: MOVE.L $000F0008,D0 (VIDEO_STATUS); ANDI.L #2,D0; BNE.S .wait_end
	// Regenerating rotozoomer_68k.ie68 can shift this label — re-derive it
	// from the disassembly (look for the first VIDEO_STATUS poll loop after
	// the render_mode7 blitter wait) when this assert starts failing.
	const waitVSyncPC = 0x1078
	const maxInstructions = 200000

	reachedWait := false
	for i := 0; i < maxInstructions; i++ {
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		if cpu.PC == waitVSyncPC {
			reachedWait = true
			break
		}
	}

	if !reachedWait {
		t.Fatalf("rotozoomer did not reach wait_vsync; PC=0x%X", cpu.PC)
	}
	if got := video.HandleRead(VIDEO_CTRL); got != 1 {
		t.Fatalf("VIDEO_CTRL at wait_vsync = %d, want 1", got)
	}
	if got := video.HandleRead(BLT_DST); got == 0 {
		t.Fatalf("BLT_DST at wait_vsync = 0, want configured blit destination")
	}
	if got := video.HandleRead(BLT_WIDTH); got == 0 {
		t.Fatalf("BLT_WIDTH at wait_vsync = 0, want configured blit width")
	}
	if got := video.HandleRead(BLT_HEIGHT); got == 0 {
		t.Fatalf("BLT_HEIGHT at wait_vsync = 0, want configured blit height")
	}
}

func testM68KRunnerRotozoomerBinaryReachesWaitVSyncWithConfiguredBlitter(t *testing.T, useJIT bool) {
	cpu, video, _ := newM68KVideoMMIORig(t)

	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = video.Stop()
	})

	programPath := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_68k.ie68")
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", programPath, err)
	}
	cpu.LoadProgramBytes(program)

	runner := NewM68KRunner(cpu)
	runner.cpu.m68kJitEnabled = useJIT
	debugger := NewDebugM68K(cpu, runner)
	runner.StartExecution()
	defer runner.Stop()

	// wait_vsync spans 0x1078..0x1095 (two VIDEO_STATUS poll loops + RTS) —
	// see TestM68K_RotozoomerBinary_ReachesWaitVSyncWithVideoEnabled. This
	// variant samples PC asynchronously while the CPU spins, so match the
	// whole routine's range: an exact-PC probe almost always lands on a
	// post-fetch PC inside the MMIO read and can miss a single value for
	// the entire deadline under load (word fetches advance PC in steps).
	const waitVSyncLo, waitVSyncHi = 0x1078, 0x1095
	inWaitVSync := func() bool {
		debugger.Freeze()
		pc := cpu.PC
		debugger.Resume()
		return pc >= waitVSyncLo && pc <= waitVSyncHi
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if inWaitVSync() {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if !inWaitVSync() {
		debugger.Freeze()
		pc := cpu.PC
		t.Fatalf("runner did not reach wait_vsync; PC=0x%X", pc)
	}
	if got := video.HandleRead(VIDEO_CTRL); got != 1 {
		t.Fatalf("VIDEO_CTRL at wait_vsync = %d, want 1", got)
	}
	if got := video.HandleRead(BLT_DST); got == 0 {
		t.Fatalf("BLT_DST at wait_vsync = 0, want configured blit destination")
	}
	if got := video.HandleRead(BLT_WIDTH); got == 0 {
		t.Fatalf("BLT_WIDTH at wait_vsync = 0, want configured blit width")
	}
	if got := video.HandleRead(BLT_HEIGHT); got == 0 {
		t.Fatalf("BLT_HEIGHT at wait_vsync = 0, want configured blit height")
	}
}

func TestM68KRunner_RotozoomerBinary_Interpreter_ReachesWaitVSyncWithConfiguredBlitter(t *testing.T) {
	testM68KRunnerRotozoomerBinaryReachesWaitVSyncWithConfiguredBlitter(t, false)
}
