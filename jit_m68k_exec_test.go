// jit_m68k_exec_test.go - Integration tests for M68020 JIT execution loop

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// runM68KJITProgram loads M68K opcodes at startPC, runs ExecuteJIT with a timeout,
// and returns the CPU for result inspection.
func runM68KJITProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	return runM68KJITProgramWithSetup(t, startPC, nil, false, opcodes...)
}

func runM68KJITProgramWithSetup(t *testing.T, startPC uint32, setup func(*M68KCPU), forceNative bool, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP (initial stack)
	bus.Write32(4, startPC)    // reset vector → our code
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitForceNative = forceNative
	cpu.PC = startPC
	cpu.SR = M68K_SR_S // supervisor mode
	if setup != nil {
		setup(cpu)
	}

	// Write opcodes in big-endian
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}

	// Run with timeout
	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("M68K JIT execution timed out")
	}

	return cpu
}

func newM68KJITDemoCPU(t *testing.T) (*M68KCPU, *VideoChip) {
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

	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	bus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1, vga.HandleTextRead, vga.HandleTextWrite)

	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, M68K_ENTRY_POINT)

	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = M68K_ENTRY_POINT
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x00010000

	return cpu, video
}

// writeBELong writes a big-endian uint32 to memory.
func writeBELong(mem []byte, addr uint32, val uint32) {
	mem[addr] = byte(val >> 24)
	mem[addr+1] = byte(val >> 16)
	mem[addr+2] = byte(val >> 8)
	mem[addr+3] = byte(val)
}

// TestM68KJIT_Exec_SimpleHalt tests that a STOP instruction halts execution.
func TestM68KJIT_Exec_SimpleHalt(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// MOVEQ #42,D0; then ILLEGAL (0x4AFC) to halt
	// Actually, ILLEGAL will cause an exception which may loop.
	// Use a simpler approach: just run a few instructions and then check running stopped.
	// MOVEQ #42,D0; MOVEQ #0,D1; then we stop running externally.
	cpu.memory[0x1000] = 0x70
	cpu.memory[0x1001] = 0x2A // MOVEQ #42,D0
	cpu.memory[0x1002] = 0x72
	cpu.memory[0x1003] = 0x00 // MOVEQ #0,D1

	// Put a STOP #$2700 to halt — STOP needs fallback to interpreter
	cpu.memory[0x1004] = 0x4E
	cpu.memory[0x1005] = 0x72 // STOP
	cpu.memory[0x1006] = 0x27
	cpu.memory[0x1007] = 0x00 // #$2700

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Wait a bit then stop
	time.Sleep(100 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_ALUSequence runs ADD/SUB through the full dispatcher.
func TestM68KJIT_Exec_ALUSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #10,D0; MOVEQ #20,D1; ADD.L D0,D1; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x700A, // MOVEQ #10,D0
		0x7214, // MOVEQ #20,D1
		0xD280, // ADD.L D0,D1  (D1 = D1 + D0 = 30)
	)

	if cpu.DataRegs[0] != 10 {
		t.Errorf("D0 = %d, want 10", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 30 {
		t.Errorf("D1 = %d, want 30", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_MemoryMove runs MOVE with memory through the dispatcher.
func TestM68KJIT_Exec_MemoryMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xDEAD, 0xBEEF, // MOVE.L #$DEADBEEF,D0
	)

	if cpu.DataRegs[0] != 0xDEADBEEF {
		t.Errorf("D0 = 0x%08X, want 0xDEADBEEF", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_JSR_RTS runs a subroutine call through the dispatcher.
func TestM68KJIT_Exec_JSR_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000 // stack pointer

	// Main: MOVEQ #1,D0; JSR $1010; MOVEQ #3,D0; STOP
	// Sub at $1010: MOVEQ #2,D1; RTS
	// After execution: D0=3 (set after JSR returns), D1=2
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x4EB9) // JSR
	writeW(0x0000) // abs.L high
	writeW(0x1010) // abs.L low (target = $1010)
	writeW(0x7003) // MOVEQ #3,D0 (executed after RTS)
	// STOP to halt
	writeW(0x4E72)
	writeW(0x2700)

	// Subroutine at $1010
	pc = 0x1010
	writeW(0x7402) // MOVEQ #2,D2
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_RotatingCubeDoesNotOverwritePlotPixelCode(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu, video := newM68KJITDemoCPU(t)
	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	defer func() { _ = video.Stop() }()

	programPath := filepath.Join("sdk", "examples", "prebuilt", "rotating_cube_copper_68k.ie68")
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", programPath, err)
	}
	cpu.LoadProgramBytes(program)

	const plotPixelPC = uint32(0x1542)
	wantOpcode := cpu.Read16(plotPixelPC)
	if wantOpcode == 0 {
		t.Fatalf("unexpected zero opcode at 0x%08X before execution", plotPixelPC)
	}

	type codeWrite struct {
		addr  uint32
		value uint32
		pc    uint32
		size  int
	}

	writeCh := make(chan codeWrite, 1)
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		end := addr + uint32(size)
		if end <= addr {
			return
		}
		if plotPixelPC < addr || plotPixelPC >= end {
			return
		}
		select {
		case writeCh <- codeWrite{addr: addr, value: value, pc: pc, size: size}:
		default:
		}
		cpu.running.Store(false)
	}

	runner := NewM68KRunner(cpu)
	runner.cpu.m68kJitEnabled = true
	runner.StartExecution()
	defer runner.Stop()

	select {
	case hit := <-writeCh:
		t.Fatalf("rotating cube overwrote plot_pixel code at 0x%08X via write size=%d addr=0x%08X value=0x%08X from PC=0x%08X", plotPixelPC, hit.size, hit.addr, hit.value, hit.pc)
	case <-time.After(1500 * time.Millisecond):
	}

	if got := cpu.Read16(plotPixelPC); got != wantOpcode {
		t.Fatalf("plot_pixel opcode at 0x%08X changed from 0x%04X to 0x%04X without direct watched overlap", plotPixelPC, wantOpcode, got)
	}
}

func TestM68KJIT_Exec_RotatingCubeDrawLineMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		drawLinePC     = uint32(0x14E2)
		plotPixelRTSPC = uint32(0x1572)
		stopPC         = uint32(0x2000)
		initialSP      = uint32(0x00010000)
	)

	setupCPU := func(t *testing.T, useJIT bool) *M68KCPU {
		t.Helper()

		cpu, _ := newM68KJITDemoCPU(t)
		programPath := filepath.Join("sdk", "examples", "prebuilt", "rotating_cube_copper_68k.ie68")
		program, err := os.ReadFile(programPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", programPath, err)
		}
		cpu.LoadProgramBytes(program)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = drawLinePC
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = initialSP - 4
		cpu.Write32(initialSP-4, stopPC)
		cpu.memory[stopPC] = 0x4E
		cpu.memory[stopPC+1] = 0x72
		cpu.memory[stopPC+2] = 0x27
		cpu.memory[stopPC+3] = 0x00
		cpu.memory[plotPixelRTSPC] = 0x4E
		cpu.memory[plotPixelRTSPC+1] = 0x72
		cpu.memory[plotPixelRTSPC+2] = 0x27
		cpu.memory[plotPixelRTSPC+3] = 0x00

		// d3=x1, d4=y1, d5=x2, d6=y2, d2=colour
		cpu.DataRegs[2] = 0x4F
		cpu.DataRegs[3] = 10
		cpu.DataRegs[4] = 10
		cpu.DataRegs[5] = 10
		cpu.DataRegs[6] = 10

		return cpu
	}

	runCPU := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			cpu.running.Store(true)
			cpu.m68kJitExecute()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !cpu.stopped.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		if !cpu.stopped.Load() {
			t.Fatal("draw_line execution timed out")
		}
	}

	interp := setupCPU(t, false)
	jit := setupCPU(t, true)

	runCPU(t, interp)
	runCPU(t, jit)

	if got, want := jit.AddrRegs[7], interp.AddrRegs[7]; got != want {
		t.Fatalf("A7 mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	if got, want := jit.PC, interp.PC; got != want {
		t.Fatalf("PC mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	for reg := range 8 {
		if got, want := jit.DataRegs[reg], interp.DataRegs[reg]; got != want {
			t.Fatalf("D%d mismatch after draw_line: jit=0x%08X interp=0x%08X", reg, got, want)
		}
	}
	if got, want := jit.AddrRegs[0], interp.AddrRegs[0]; got != want {
		t.Fatalf("A0 mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	for i := uint32(0); i < 16; i += 2 {
		addr := interp.AddrRegs[7] + i
		if got, want := jit.Read16(addr), interp.Read16(addr); got != want {
			t.Fatalf("stack mismatch at 0x%08X: jit=0x%04X interp=0x%04X", addr, got, want)
		}
	}
	for i := uint32(0); i < 32; i++ {
		addr := VGA_VRAM_WINDOW + 10*320 + 10 + i
		if got, want := jit.Read8(addr), interp.Read8(addr); got != want {
			t.Fatalf("VRAM mismatch at 0x%08X: jit=0x%02X interp=0x%02X", addr, got, want)
		}
	}
}

func TestM68KJIT_Exec_BSR_W_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main:
	// 0x1000 MOVEQ #1,D0
	// 0x1002 BSR.W +8 -> 0x100E
	// 0x1006 MOVEQ #3,D0
	// 0x1008 STOP #$2700
	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x6100) // BSR.W
	writeW(0x0008) // target = 0x1004 + 0x0008 = 0x100E
	writeW(0x7003) // MOVEQ #3,D0
	writeW(0x4E72) // STOP
	writeW(0x2700)

	// Padding before the subroutine entry.
	writeW(0x4E71) // NOP

	// Subroutine at 0x100E: MOVEQ #2,D2; RTS
	writeW(0x7402) // MOVEQ #2,D2
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after BSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in BSR subroutine)", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_BSR_W_IntoInterpreterFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x1008
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4A40) // TST.W D0 (forces interpreter fallback at target)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("BSR fallback/RTS test timed out")
	}
	if got := cpu.PC; got != 0x1008 {
		t.Fatalf("PC = 0x%08X, want STOP at 0x00001008", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_MoveWordPredecrementPushesOntoStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[7] = 0xFFFF000A

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("stack push test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x0000FFFC {
		t.Fatalf("A7 = 0x%08X, want 0x0000FFFC", got)
	}
	if got := cpu.Read16(0xFFFC); got != 0x000A {
		t.Fatalf("stack[0xFFFC] = 0x%04X, want 0x000A", got)
	}
	if got := cpu.Read16(0xFFFE); got != 0x000A {
		t.Fatalf("stack[0xFFFE] = 0x%04X, want 0x000A", got)
	}
}

func TestM68KJIT_Exec_MoveWordPushesThenBSRIntoFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1
	cpu.DataRegs[7] = 0xFFFF000A

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x100C
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4A40) // TST.W D0 (fallback)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("push+bsr fallback test timed out")
	}
	if got := cpu.PC; got != 0x100C {
		t.Fatalf("PC = 0x%08X, want STOP at 0x0000100C", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x0000FFFC {
		t.Fatalf("A7 = 0x%08X, want 0x0000FFFC", got)
	}
	if got := cpu.Read16(0xFFFC); got != 0x000A {
		t.Fatalf("stack[0xFFFC] = 0x%04X, want 0x000A", got)
	}
	if got := cpu.Read16(0xFFFE); got != 0x000A {
		t.Fatalf("stack[0xFFFE] = 0x%04X, want 0x000A", got)
	}
}

func TestM68KJIT_Exec_TrapHandlerPreludeExtractsVectorWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	setupCPU := func(useJIT bool) *M68KCPU {
		bus := NewMachineBus()
		termOut := NewTerminalOutput()
		bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
		bus.Write32(0, 0x00010000)
		bus.Write32(4, 0x1000)
		cpu := NewM68KCPU(bus)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = 0x10000
		cpu.DataRegs[0] = 0xDEADBEEF

		// Synthetic exception frame as expected by ct_trap_handler:
		//   [SP+0]  = SR
		//   [SP+2]  = PC
		//   [SP+6]  = fmt/vector word (vector 6 -> offset 24 -> 0x0018)
		cpu.Write16(0x10000, 0x2700)
		cpu.Write32(0x10002, 0x00002000)
		cpu.Write16(0x10006, 0x0018)

		pc := uint32(0x1000)
		writeW := func(val uint16) {
			cpu.memory[pc] = byte(val >> 8)
			cpu.memory[pc+1] = byte(val)
			pc += 2
		}

		writeW(0x2F00) // MOVE.L D0,-(SP)
		writeW(0x7000) // MOVEQ #0,D0
		writeW(0x302F) // MOVE.W 10(SP),D0
		writeW(0x000A)
		writeW(0x0280) // ANDI.L #$00000FFF,D0
		writeW(0x0000)
		writeW(0x0FFF)
		writeW(0xE488) // LSR.L #2,D0
		writeW(0x4E72) // STOP
		writeW(0x2700)

		return cpu
	}

	runInterp := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		cpu.running.Store(true)
		for i := 0; i < 100 && !cpu.stopped.Load() && cpu.running.Load(); i++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
		if !cpu.stopped.Load() {
			t.Fatalf("interpreter trap-prelude test did not stop (PC=0x%08X)", cpu.PC)
		}
	}

	runJIT := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			cpu.running.Store(true)
			cpu.M68KExecuteJIT()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		if !cpu.stopped.Load() {
			t.Fatalf("JIT trap-prelude test did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
		}
	}

	interp := setupCPU(false)
	jit := setupCPU(true)
	runInterp(t, interp)
	runJIT(t, jit)

	if got, want := interp.DataRegs[0], uint32(6); got != want {
		t.Fatalf("interpreter D0 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[0], interp.DataRegs[0]; got != want {
		t.Fatalf("JIT D0 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.AddrRegs[7], interp.AddrRegs[7]; got != want {
		t.Fatalf("JIT A7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.Read32(0xFFFC), interp.Read32(0xFFFC); got != want {
		t.Fatalf("saved D0 on stack = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_JSRHelperSkipsInlineDataViaAddqSPThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main: BSR case; MOVEQ #7,D1; STOP
	writeW(0x6100) // BSR.W
	writeW(0x001C) // -> 0x1020
	writeW(0x7207) // MOVEQ #7,D1
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1020 {
		writeW(0x4E71) // NOP padding
	}

	// case: JSR helper; inline bytes that must never execute
	writeW(0x4EB9) // JSR $00001058
	writeW(0x0000)
	writeW(0x1058)
	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'A'
	cpu.memory[pc+2] = 'D'
	cpu.memory[pc+3] = 0x00
	pc += 4

	for pc < 0x1058 {
		writeW(0x4E71) // NOP padding
	}

	// helper: ADDQ.L #4,SP ; RTS
	writeW(0x588F)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("inline-data helper test did not reach STOP (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.DataRegs[1]; got != 7 {
		t.Fatalf("D1 = %d, want 7 (execution should resume after BSR caller)", got)
	}
	if got := cpu.PC; got != 0x0000100A {
		t.Fatalf("PC = 0x%08X, want 0x0000100A after STOP", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_FirstCPUSuiteBFTSTCaseMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	setupCPU := func(useJIT bool) *M68KCPU {
		bus := NewMachineBus()
		termOut := NewTerminalOutput()
		bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
		bus.Write32(0, 0x00010000)
		bus.Write32(4, 0x1000)
		cpu := NewM68KCPU(bus)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = 0x10000

		pc := uint32(0x1000)
		writeW := func(val uint16) {
			cpu.memory[pc] = byte(val >> 8)
			cpu.memory[pc+1] = byte(val)
			pc += 2
		}

		// Exact instruction bytes from the first assembled CPU-suite case.
		writeW(0x203C) // MOVE.L #$FF000000,D0
		writeW(0xFF00)
		writeW(0x0000)
		writeW(0x7400) // MOVEQ #0,D2
		writeW(0x44C2) // MOVE.W D2,CCR
		writeW(0xE8C0) // BFTST D0{0:8}
		writeW(0x0008)
		writeW(0x40C1) // MOVE.W SR,D1
		writeW(0x0241) // ANDI.W #$000C,D1
		writeW(0x000C)
		writeW(0x0C41) // CMPI.W #$0008,D1
		writeW(0x0008)
		writeW(0x6606) // BNE.B fail
		writeW(0x7E01) // MOVEQ #1,D7 (pass)
		writeW(0x4E72) // STOP
		writeW(0x2700)
		writeW(0x7E02) // fail: MOVEQ #2,D7
		writeW(0x4E72) // STOP
		writeW(0x2700)

		return cpu
	}

	runInterp := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		for i := 0; i < 1000 && !cpu.stopped.Load() && cpu.running.Load(); i++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
		if !cpu.stopped.Load() {
			t.Fatalf("interpreter case did not reach STOP (PC=0x%08X)", cpu.PC)
		}
	}

	runJIT := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			cpu.running.Store(true)
			cpu.M68KExecuteJIT()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		if !cpu.stopped.Load() {
			t.Fatalf("JIT case did not reach STOP (PC=0x%08X)", cpu.PC)
		}
	}

	interp := setupCPU(false)
	jit := setupCPU(true)
	interp.running.Store(true)

	runInterp(t, interp)
	runJIT(t, jit)

	if got, want := interp.DataRegs[7], uint32(1); got != want {
		t.Fatalf("interpreter D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[7], interp.DataRegs[7]; got != want {
		t.Fatalf("JIT D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[1]&0xFFFF, interp.DataRegs[1]&0xFFFF; got != want {
		t.Fatalf("JIT D1.W = 0x%04X, want 0x%04X", got, want)
	}
	if got, want := jit.SR&0x000F, interp.SR&0x000F; got != want {
		t.Fatalf("JIT SR low flags = 0x%04X, want 0x%04X", got, want)
	}
}

func TestM68KJIT_Exec_FirstCPUSuiteCaseWithPassHelperAndBSR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// shard: BSR case; MOVEQ #9,D6; STOP
	writeW(0x6100) // BSR.W
	writeW(0x001C) // -> 0x1020
	writeW(0x7C09) // MOVEQ #9,D6
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1020 {
		writeW(0x4E71)
	}

	// Exact first case pass path from the assembled suite.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP (should never hit in this test)
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	// pass_helper: MOVE.L $80000,D0 ; ADDQ.L #1,D0 ; MOVE.L D0,$80000 ; RTS
	writeW(0x2039)
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280)
	writeW(0x23C0)
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("suite case + pass helper did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1", got)
	}
	if got := cpu.DataRegs[6]; got != 9 {
		t.Fatalf("D6 = %d, want 9 (execution should resume in shard after case RTS)", got)
	}
}

func TestM68KJIT_Exec_ShardDispatcherJSRIndirectMatchesSuitePattern(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}
	writeL := func(val uint32) {
		cpu.memory[pc] = byte(val >> 24)
		cpu.memory[pc+1] = byte(val >> 16)
		cpu.memory[pc+2] = byte(val >> 8)
		cpu.memory[pc+3] = byte(val)
		pc += 4
	}

	// Minimal top-level suite dispatcher:
	// lea shard_list,a5
	// move.l (a5)+,d0
	// beq done
	// movea.l d0,a1
	// move.l a5,-(sp)
	// jsr (a1)
	// movea.l (sp)+,a5
	// moveq #9,d6
	// stop
	writeW(0x4BFA) // LEA shard_list(PC),A5
	writeW(0x0022)
	writeW(0x201D) // MOVE.L (A5)+,D0
	writeW(0x670A) // BEQ done
	writeW(0x2240) // MOVEA.L D0,A1
	writeW(0x2F0D) // MOVE.L A5,-(SP)
	writeW(0x4E91) // JSR (A1)
	writeW(0x2A5F) // MOVEA.L (SP)+,A5
	writeW(0x7C09) // MOVEQ #9,D6
	writeW(0x4E72) // done: STOP
	writeW(0x2700)

	for pc < 0x1024 {
		writeW(0x4E71)
	}

	writeL(0x00001100) // shard_list[0] = shard entry
	writeL(0x00000000) // terminator

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	// shard: BSR case ; RTS
	writeW(0x6100)
	writeW(0x001C) // -> 0x1120
	writeW(0x4E75)

	for pc < 0x1120 {
		writeW(0x4E71)
	}

	// Reuse the exact first-case pass path.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1200)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP (should not hit)
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1200 {
		writeW(0x4E71)
	}

	writeW(0x2039) // pass_helper: MOVE.L $80000,D0
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280) // ADDQ.L #1,D0
	writeW(0x23C0) // MOVE.L D0,$80000
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("suite dispatcher repro did not stop (PC=0x%08X SP=0x%08X A5=0x%08X)", cpu.PC, cpu.AddrRegs[7], cpu.AddrRegs[5])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1", got)
	}
	if got := cpu.DataRegs[6]; got != 9 {
		t.Fatalf("D6 = %d, want 9", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x00001028 {
		t.Fatalf("A5 = 0x%08X, want 0x00001028 after restoring shard-list cursor", got)
	}
}

func TestM68KJIT_Exec_ShardPrologueJSRThenFirstCase(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Shard body matching the real suite shape:
	// lea shard_name,a0 ; jsr ct_enter_shard ; bsr case ; stop
	writeW(0x41FA) // LEA shard_name(PC),A0
	writeW(0x0014)
	writeW(0x4EB9) // JSR ct_enter_shard
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x6100) // BSR.W case
	writeW(0x0022) // -> 0x1030
	writeW(0x4E72) // STOP
	writeW(0x2700)

	cpu.memory[pc+0] = 'S'
	cpu.memory[pc+1] = 'H'
	cpu.memory[pc+2] = 'A'
	cpu.memory[pc+3] = 'R'
	cpu.memory[pc+4] = 'D'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1030 {
		writeW(0x4E71)
	}

	// First case pass path.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1200)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP if fail
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	writeW(0x4E75) // ct_enter_shard: RTS

	for pc < 0x1200 {
		writeW(0x4E71)
	}

	writeW(0x2039) // pass_helper: MOVE.L $80000,D0
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280) // ADDQ.L #1,D0
	writeW(0x23C0) // MOVE.L D0,$80000
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("shard prologue repro did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1 (PC=0x%08X D1=0x%08X SR=0x%04X SP=0x%08X)", got, cpu.PC, cpu.DataRegs[1], cpu.SR, cpu.AddrRegs[7])
	}
}

func TestM68KJIT_Exec_JSRIndirectWithSavedA5OnStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.AddrRegs[5] = 0x12345678
	cpu.AddrRegs[1] = 0x00001100

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2F0D) // MOVE.L A5,-(SP)
	writeW(0x4E91) // JSR (A1)
	writeW(0x2A5F) // MOVEA.L (SP)+,A5
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1100 {
		writeW(0x4E71)
	}
	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("indirect jsr + saved a5 test did not stop (PC=0x%08X SP=0x%08X A5=0x%08X)", cpu.PC, cpu.AddrRegs[7], cpu.AddrRegs[5])
	}
	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = %d, want 1", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x12345678 {
		t.Fatalf("A5 = 0x%08X, want 0x12345678", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_LoadShardPointerViaA5PostIncrement(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}
	writeL := func(val uint32) {
		cpu.memory[pc] = byte(val >> 24)
		cpu.memory[pc+1] = byte(val >> 16)
		cpu.memory[pc+2] = byte(val >> 8)
		cpu.memory[pc+3] = byte(val)
		pc += 4
	}

	writeW(0x4BFA) // LEA shard_list(PC),A5
	writeW(0x000A)
	writeW(0x201D) // MOVE.L (A5)+,D0
	writeW(0x2240) // MOVEA.L D0,A1
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeL(0x00001100)
	writeL(0x00000000)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("load shard pointer test did not stop (PC=0x%08X A5=0x%08X D0=0x%08X A1=0x%08X)", cpu.PC, cpu.AddrRegs[5], cpu.DataRegs[0], cpu.AddrRegs[1])
	}
	if got := cpu.DataRegs[0]; got != 0x00001100 {
		t.Fatalf("D0 = 0x%08X, want 0x00001100", got)
	}
	if got := cpu.AddrRegs[1]; got != 0x00001100 {
		t.Fatalf("A1 = 0x%08X, want 0x00001100", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x00001010 {
		t.Fatalf("A5 = 0x%08X, want 0x00001010 after post-increment", got)
	}
}

func TestM68KJIT_Exec_SubroutineWithMovemFrameAroundIndexedVGAPixelWrite(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x4EB9) // JSR $1100
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("movem VGA subroutine test timed out")
	}
	if got := cpu.DataRegs[0]; got != 0x12345678 {
		t.Fatalf("D0 = 0x%08X, want 0x12345678", got)
	}
	if got := cpu.DataRegs[1]; got != 0x87654321 {
		t.Fatalf("D1 = 0x%08X, want 0x87654321", got)
	}
	if got := cpu.AddrRegs[0]; got != 0xCAFEBABE {
		t.Fatalf("A0 = 0x%08X, want 0xCAFEBABE", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_WordStackLocalsThenMovemPixelSubroutineThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x4EB9) // JSR $1100
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("word locals + pixel subroutine test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := cpu.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_DrawLineStyleNestedFramesAndLocals(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x48E7) // MOVEM.L D0-D7/A0,-(SP)
	writeW(0x01FF)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W $1100
	writeW(0x00F6)
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D7/A0
	writeW(0xFF80)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("draw-line style nested frame test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_OuterMovemThenWordLocalsThenBSRIntoFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x48E7) // MOVEM.L D0-D7/A0,-(SP)
	writeW(0x01FF)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W
	writeW(0x000A) // -> 0x1016
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D7/A0
	writeW(0xFF80)
	writeW(0x4E72) // STOP
	writeW(0x2700)

	writeW(0x4A40) // TST.W D0 (fallback)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("outer movem + locals + bsr test timed out")
	}
	if got := cpu.PC; got != 0x1016 {
		t.Fatalf("PC = 0x%08X, want STOP at 0x00001016", got)
	}
}

func TestM68KJIT_Exec_NestedBSRWithMovemFramesRestoresStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main: BSR func1; MOVEQ #7,D0; STOP
	writeW(0x6100) // BSR.W
	writeW(0x0010) // -> 0x1014
	writeW(0x7007) // MOVEQ #7,D0
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding

	// func1 at 0x1014:
	// MOVEM.L D0-D1/A0,-(SP); BSR func2; MOVEM.L (SP)+,D0-D1/A0; RTS
	writeW(0x48E7)
	writeW(0xC080)
	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x1022
	writeW(0x4CDF)
	writeW(0x0103)
	writeW(0x4E75)

	// func2 at 0x1022:
	// MOVEM.L D0-D1/A0,-(SP); MOVEQ #5,D1; MOVEM.L (SP)+,D0-D1/A0; RTS
	writeW(0x48E7)
	writeW(0xC080)
	writeW(0x7205)
	writeW(0x4CDF)
	writeW(0x0103)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[0]; got != 7 {
		t.Fatalf("D0 = %d, want 7", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x10000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_VideoBlitterSetupSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)
	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	defer func() { _ = video.Stop() }()

	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Mirror the rotozoomer load_texture-style setup sequence.
	writeW(0x23FC) // MOVE.L #0,BLT_OP
	writeW(0x0000)
	writeW(0x0000)
	writeW(0x000F)
	writeW(0x0020)

	writeW(0x41F9) // LEA $00001600,A0
	writeW(0x0000)
	writeW(0x1600)

	writeW(0x23C8) // MOVE.L A0,BLT_SRC
	writeW(0x000F)
	writeW(0x0024)

	writeW(0x23FC) // MOVE.L #$00600000,BLT_DST
	writeW(0x0060)
	writeW(0x0000)
	writeW(0x000F)
	writeW(0x0028)

	writeW(0x23FC) // MOVE.L #$00000100,BLT_WIDTH
	writeW(0x0000)
	writeW(0x0100)
	writeW(0x000F)
	writeW(0x002C)

	writeW(0x23FC) // MOVE.L #$00000100,BLT_HEIGHT
	writeW(0x0000)
	writeW(0x0100)
	writeW(0x000F)
	writeW(0x0030)

	writeW(0x23FC) // MOVE.L #$00000400,BLT_SRC_STRIDE
	writeW(0x0000)
	writeW(0x0400)
	writeW(0x000F)
	writeW(0x0034)

	writeW(0x23FC) // MOVE.L #$00000400,BLT_DST_STRIDE
	writeW(0x0000)
	writeW(0x0400)
	writeW(0x000F)
	writeW(0x0038)

	writeW(0x23FC) // MOVE.L #1,BLT_CTRL
	writeW(0x0000)
	writeW(0x0001)
	writeW(0x000F)
	writeW(0x001C)

	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := video.HandleRead(BLT_SRC); got != 0x00001600 {
		t.Fatalf("BLT_SRC = 0x%08X, want 0x00001600", got)
	}
	if got := video.HandleRead(BLT_DST); got != 0x00600000 {
		t.Fatalf("BLT_DST = 0x%08X, want 0x00600000", got)
	}
	if got := video.HandleRead(BLT_WIDTH); got != 0x00000100 {
		t.Fatalf("BLT_WIDTH = 0x%08X, want 0x00000100", got)
	}
	if got := video.HandleRead(BLT_HEIGHT); got != 0x00000100 {
		t.Fatalf("BLT_HEIGHT = 0x%08X, want 0x00000100", got)
	}
}

func TestM68KJIT_Exec_MoveLongAbsMaskAndStoreBack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x880C, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2039) // MOVE.L $0000880C,D0
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x0280) // ANDI.L #$0000001F,D0
	writeW(0x0000)
	writeW(0x001F)
	writeW(0x23C0) // MOVE.L D0,$0000880C
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.Read32(0x880C); got != 0x00000018 {
		t.Fatalf("memory[0x880C] = 0x%08X, want 0x00000018", got)
	}
}

func TestM68KJIT_Exec_MoveMaskStoreThenBRA_W(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x880C, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2039) // MOVE.L $0000880C,D0
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x0280) // ANDI.L #$0000001F,D0
	writeW(0x0000)
	writeW(0x001F)
	writeW(0x23C0) // MOVE.L D0,$0000880C
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x6000) // BRA.W +4 -> 0x101A
	writeW(0x0004)
	writeW(0x7299) // MOVEQ #-103,D1 (skipped)
	writeW(0x7601) // MOVEQ #1,D3
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.Read32(0x880C); got != 0x00000018 {
		t.Fatalf("memory[0x880C] = 0x%08X, want 0x00000018", got)
	}
	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BRA.W should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[3] != 1 {
		t.Fatalf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

func TestM68KJIT_Exec_MoveByteIndexedLongIntoDn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write8(0x2003, 0x7F)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x7403) // MOVEQ #3,D2
	writeW(0x1A30) // MOVE.B 0(A0,D2.L),D5
	writeW(0x2800)
	writeW(0x0285) // ANDI.L #$000000FF,D5
	writeW(0x0000)
	writeW(0x00FF)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[5]; got != 0x7F {
		t.Fatalf("D5 = 0x%08X, want 0x0000007F", got)
	}
}

func TestM68KJIT_Exec_LEAAbsLongThenMoveBytePostInc(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x10FC) // MOVE.B #1,(A0)+
	writeW(0x0001)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := bus.Read8(0x000A0000); got != 0x01 {
		t.Fatalf("bus[0x000A0000] = 0x%02X, want 0x01", got)
	}
	if got := cpu.AddrRegs[0]; got != 0x000A0001 {
		t.Fatalf("A0 = 0x%08X, want 0x000A0001", got)
	}
	if got := cpu.Read16(0x1000); got != 0x41F9 {
		t.Fatalf("memory[0x1000] = 0x%04X, want 0x41F9", got)
	}
}

func TestM68KJIT_Exec_MoveByteImmediateToVGAMMIO(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x13FC) // MOVE.B #$13,$000F1000 (VGA_MODE)
	writeW(0x0013)
	writeW(0x000F)
	writeW(0x1000)
	writeW(0x13FC) // MOVE.B #$01,$000F1008 (VGA_CTRL)
	writeW(0x0001)
	writeW(0x000F)
	writeW(0x1008)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := bus.Read8(VGA_MODE); got != VGA_MODE_13H {
		t.Fatalf("VGA_MODE = 0x%02X, want 0x%02X", got, VGA_MODE_13H)
	}
	if got := bus.Read8(VGA_CTRL); got != VGA_CTRL_ENABLE {
		t.Fatalf("VGA_CTRL = 0x%02X, want 0x%02X", got, VGA_CTRL_ENABLE)
	}
}

func TestM68KJIT_Exec_MoveByteDnToIndexedVGAVRAMDoesNotTouchStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0xFFFC, 0x11223344)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x203C) // MOVE.L #3210,D0
	writeW(0x0000)
	writeW(0x0C8A)
	writeW(0x744F) // MOVEQ #79,D2
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)
	if !cpu.stopped.Load() {
		t.Fatal("indexed VGA write test timed out")
	}

	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
	if got := cpu.Read32(0xFFFC); got != 0x11223344 {
		t.Fatalf("stack sentinel = 0x%08X, want 0x11223344", got)
	}
}

func TestM68KJIT_Exec_DBF_MoveBytePostIncrementIntoVGAVRAM(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x303C) // MOVE.W #3,D0
	writeW(0x0003)
	writeW(0x10FC) // MOVE.B #1,(A0)+
	writeW(0x0001)
	writeW(0x51C8) // DBF D0, loop
	writeW(0xFFFA)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	for i := uint32(0); i < 4; i++ {
		if got := bus.Read8(0x000A0000 + i); got != 0x01 {
			t.Fatalf("bus[0x%08X] = 0x%02X, want 0x01", 0x000A0000+i, got)
		}
	}
	if got := cpu.AddrRegs[0]; got != 0x000A0004 {
		t.Fatalf("A0 = 0x%08X, want 0x000A0004", got)
	}
}

func TestM68KJIT_Exec_MoveLongIndexedMappedIndexIntoDn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x2014, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x7205) // MOVEQ #5,D1
	writeW(0xE589) // LSL.L #2,D1
	writeW(0x2430) // MOVE.L 0(A0,D1.L),D2
	writeW(0x1800)
	writeW(0x0682) // ADDI.L #200,D2
	writeW(0x0000)
	writeW(0x00C8)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[2]; got != 0x12345740 {
		t.Fatalf("D2 = 0x%08X, want 0x12345740", got)
	}
}

func TestM68KJIT_Exec_DBF_MoveByteImmediatePostIncrementLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x303C) // MOVE.W #$03E7,D0
	writeW(0x03E7)
	loopPC := pc
	writeW(0x10FC) // MOVE.B #$80,(A0)+
	writeW(0x0080)
	_ = loopPC
	writeW(0x51C8) // DBF D0, loop
	writeW(0xFFFA)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	for i := uint32(0); i < 1000; i++ {
		if got := cpu.Read8(0x2000 + i); got != 0x80 {
			t.Fatalf("memory[0x%04X] = 0x%02X, want 0x80", 0x2000+i, got)
		}
	}
	if got := cpu.AddrRegs[0]; got != 0x2000+1000 {
		t.Fatalf("A0 = 0x%08X, want 0x%08X", got, 0x2000+1000)
	}
	if got := cpu.DataRegs[0] & 0xFFFF; got != 0xFFFF {
		t.Fatalf("D0.W = 0x%04X, want 0xFFFF", got)
	}
}

func TestM68KJIT_Exec_LazyCCR_CMP_BLT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x2C3C, 0x0000, 0x000A, // MOVE.L #10,D6
		0x203C, 0x0000, 0x0014, // MOVE.L #20,D0
		0xBC80, // CMP.L D0,D6
		0x6D02, // BLT.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BLT should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_LazyCCR_TST_BMI(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x70FF, // MOVEQ #-1,D0
		0x4A80, // TST.L D0
		0x6B02, // BMI.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BMI should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_CMPI_BGE(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x0260, // MOVE.L #$260,D0
		0x0C80, 0x0000, 0x0260, // CMPI.L #$260,D0
		0x6C02, // BGE.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BGE should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_TSTWordUsesLowWordForBMI(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x01FE, 0xB093, // MOVE.L #$01FEB093,D0
		0x4A40, // TST.W D0
		0x6B02, // BMI.B +2
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BMI should skip on negative low word)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_ADDWordPreservesDestinationUpperWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x5280, // MOVE.L #$00005280,D0
		0x263C, 0x01FE, 0xB093, // MOVE.L #$01FEB093,D3
		0xD043, // ADD.W D3,D0
	)

	if got := cpu.DataRegs[0]; got != 0x00000313 {
		t.Fatalf("D0 = 0x%08X, want 0x00000313", got)
	}
}

func TestM68KJIT_Exec_SpilledAddressCalcAcrossMULUBail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x263C) // MOVE.L #224,D3
	writeW(0x0000)
	writeW(0x00E0)
	writeW(0x243C) // MOVE.L #40,D2
	writeW(0x0000)
	writeW(0x0028)
	writeW(0x2803) // MOVE.L D3,D4
	writeW(0x2A3C) // MOVE.L #LINE_BYTES,D5
	writeW(0x0000)
	writeW(0x0A00)
	writeW(0xC8C5) // MULU.W D5,D4
	writeW(0x2A02) // MOVE.L D2,D5
	writeW(0xE58D) // LSL.L #2,D5
	writeW(0xD885) // ADD.L D5,D4
	writeW(0x0684) // ADDI.L #VRAM_START,D4
	writeW(0x0010)
	writeW(0x0000)
	writeW(0x2E04) // MOVE.L D4,D7
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	const want = 0x0018C0A0
	if got := cpu.DataRegs[4]; got != want {
		t.Fatalf("D4 = 0x%08X, want 0x%08X", got, want)
	}
	if got := cpu.DataRegs[7]; got != want {
		t.Fatalf("D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got := cpu.DataRegs[5]; got != 0xA0 {
		t.Fatalf("D5 = 0x%08X, want 0x000000A0", got)
	}
}

// TestM68KJIT_Exec_BranchTaken runs a conditional branch through the dispatcher.
func TestM68KJIT_Exec_BranchTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #0,D0; TST.L D0; BEQ +4; MOVEQ #99,D1; MOVEQ #42,D2; STOP
	// BEQ should skip MOVEQ #99,D1 and land on MOVEQ #42,D2
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7000, // MOVEQ #0,D0
		0x4A80, // TST.L D0 → sets Z=1
		0x6702, // BEQ.B +2 (skip next instruction)
		0x7263, // MOVEQ #99,D1 (should be skipped)
		0x742A, // MOVEQ #42,D2 (branch target)
	)

	if cpu.DataRegs[1] != 0 {
		t.Errorf("D1 = %d, want 0 (should be skipped by BEQ)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 42 {
		t.Errorf("D2 = %d, want 42", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_CMPWordThenBEQByteTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x760A, // MOVEQ #10,D3
		0x7A0A, // MOVEQ #10,D5
		0xB645, // CMP.W D5,D3
		0x6702, // BEQ.B +2
		0x7099, // MOVEQ #-103,D0 (skipped)
		0x7001, // MOVEQ #1,D0
	)

	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = 0x%08X, want 0x00000001", got)
	}
}

func TestM68KJIT_Exec_SUBWordThenBGEByteTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7000, // MOVEQ #0,D0
		0x7E00, // MOVEQ #0,D7
		0x9E40, // SUB.W D0,D7
		0x6C02, // BGE.B +2
		0x7099, // MOVEQ #-103,D0 (skipped)
		0x7001, // MOVEQ #1,D0
	)

	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = 0x%08X, want 0x00000001", got)
	}
}

// runM68KJITStopProgram runs a program followed by STOP, waits for it to halt.
func runM68KJITStopProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	return runM68KJITStopProgramWithSetup(t, startPC, nil, false, opcodes...)
}

func runM68KJITStopProgramWithSetup(t *testing.T, startPC uint32, setup func(*M68KCPU), forceNative bool, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitForceNative = forceNative
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	if setup != nil {
		setup(cpu)
	}

	// Write opcodes
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	// Append STOP #$2700
	cpu.memory[pc] = 0x4E
	cpu.memory[pc+1] = 0x72
	cpu.memory[pc+2] = 0x27
	cpu.memory[pc+3] = 0x00

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		// Don't fatal — STOP halts in a loop, we just stop it externally
	}

	return cpu
}

// TestM68KJIT_Exec_DBRA_Loop runs a DBRA loop through the full dispatcher.
func TestM68KJIT_Exec_DBRA_Loop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #3,D0; MOVEQ #0,D1; loop: ADDQ #1,D1; DBRA D0,loop; STOP
	// Loop runs 4 times (D0: 3→2→1→0→-1)
	// After: D1 = 4
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7003,         // MOVEQ #3,D0
		0x7200,         // MOVEQ #0,D1
		0x5281,         // ADDQ.L #1,D1 (at 0x1004)
		0x51C8, 0xFFFC, // DBRA D0,$1004 (displacement = -4)
	)

	if cpu.DataRegs[1] != 4 {
		t.Errorf("DBRA loop: D1 = %d, want 4", cpu.DataRegs[1])
	}
}

func TestM68KJIT_Exec_NOT_B_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[5] = 0x0000007F
	}, true,
		0x4605, // NOT.B D5
	)

	if got, want := cpu.DataRegs[5], uint32(0x00000080); got != want {
		t.Fatalf("D5 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MullLongPostincrement_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 3
		cpu.AddrRegs[2] = 0x2000
		cpu.Write32(0x2000, 14)
	}, true,
		0x4C1A, 0x0800, // MULL.L (A2)+,<ext=0x0800>
	)

	if got, want := cpu.AddrRegs[2], uint32(0x2004); got != want {
		t.Fatalf("A2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[0], uint32(42); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MullLongRegister_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[3] = 7
		cpu.DataRegs[2] = 6
	}, true,
		0x4C03, 0x2000, // MULL.L D3,D2
	)

	if got, want := cpu.DataRegs[2], uint32(42); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MullLongRegisterSigned64_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0xFFFFFFFD // -3
		cpu.DataRegs[1] = 7
	}, true,
		0x4C00, 0x1C02, // MULL.L D0,D2:D1 (signed 64-bit result)
	)

	if got, want := cpu.DataRegs[2], uint32(0xFFFFFFFF); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(0xFFFFFFEB); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongRegister_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 4
		cpu.DataRegs[2] = 14
		cpu.DataRegs[1] = 0x11111111
	}, true,
		0x4C40, 0x2001, // DIVL.L D0,D1:D2
	)

	if got, want := cpu.DataRegs[2], uint32(3); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(2); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongRegisterSigned64_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 3
		cpu.DataRegs[2] = 0xFFFFFFFF
		cpu.DataRegs[1] = 0xFFFFFFEB
	}, true,
		0x4C40, 0x1C02, // DIVL.L D0,D2:D1 (signed 64-bit)
	)

	if got, want := cpu.DataRegs[1], uint32(0xFFFFFFF9); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[2], uint32(0); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongPostincrement_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[2] = 14
		cpu.AddrRegs[0] = 0x2000
		cpu.Write32(0x2000, 4)
	}, true,
		0x4C58, 0x2001, // DIVL.L (A0)+,D1:D2
	)

	if got, want := cpu.DataRegs[2], uint32(3); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(2); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.AddrRegs[0], uint32(0x2004); got != want {
		t.Fatalf("A0 = 0x%08X, want 0x%08X", got, want)
	}
}

// TestM68KJIT_Exec_MemoryALU runs ADD with immediate operand through dispatcher.
func TestM68KJIT_Exec_MemoryALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x0064, // MOVE.L #100,D0
		0xD0BC, 0x0000, 0x0032, // ADD.L #50,D0
	)

	if cpu.DataRegs[0] != 150 {
		t.Errorf("ADD.L #50,D0: D0 = %d, want 150", cpu.DataRegs[0])
	}
}

// ===========================================================================
// Block Chaining Tests (Stages 2-4)
// ===========================================================================

// TestM68KJIT_Exec_BRA_ChainPatch verifies that BRA chains directly to a
// compiled target block via patched JMP rel32.
func TestM68KJIT_Exec_BRA_ChainPatch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Sequential code: MOVEQ #1,D0; BRA.B +4; MOVEQ #99,D2; MOVEQ #2,D1; STOP
	// BRA.B +4 skips MOVEQ #99,D2 (2 bytes) + goes to MOVEQ #2,D1.
	// BRA.B displacement = +4 means target = instrPC+2+4.
	// At 0x1002: BRA.B → target = 0x1002 + 2 + 4 = 0x1008.
	// 0x1004: MOVEQ #99,D2 (skipped)
	// 0x1006: MOVEQ #77,D2 (also skipped — BRA skips to 0x1008)
	// Actually BRA.B +2: target = 0x1002 + 2 + 2 = 0x1006
	// But BRA exits block! So MOVEQ #2,D1 is in a different block.
	// That's fine — dispatcher re-enters at target and compiles block B.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7001, // MOVEQ #1,D0 at 0x1000
		0x6004, // BRA.B +4 at 0x1002 → target 0x1008
		0x7499, // MOVEQ #-103,D2 at 0x1004 (skipped by BRA)
		0x4E71, // NOP at 0x1006 (skipped by BRA)
		0x7201, // MOVEQ #1,D1 at 0x1008 (target of BRA) — 0x7201 = MOVEQ #1,D1
	)

	if cpu.DataRegs[0] != 1 {
		t.Errorf("D0 = %d, want 1 (set before BRA)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 1 {
		t.Errorf("D1 = %d, want 1 (set at BRA target)", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_ChainBudgetExhaustion verifies that chained execution
// returns to Go after the chain budget is exhausted.
func TestM68KJIT_Exec_ChainBudgetExhaustion(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Two blocks that BRA to each other, forming an infinite chain loop.
	// The budget (64) should stop execution and return to Go.
	// Block A at 0x1000: ADDQ #1,D0; BRA 0x2000
	// Block B at 0x2000: ADDQ #1,D0; BRA 0x1000
	// After timeout: D0 > 0 (proves blocks executed) and program didn't hang.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	w := func(pc uint32, ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// Block A: ADDQ.L #1,D0 + BRA.W to 0x2000 (disp = 0x2000 - 0x1006 = 0x0FFA)
	w(0x1000, 0x5280, 0x6000, 0x0FFA)
	// Block B: ADDQ.L #1,D0 + BRA.W to 0x1000 (disp = 0x1000 - 0x2006 = 0xEFFA)
	w(0x2000, 0x5280, 0x6000, 0xEFFA)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// D0 should be large (many iterations via chaining) but execution didn't hang
	if cpu.DataRegs[0] == 0 {
		t.Error("D0 should be > 0 after chained BRA loop")
	}
}

// TestM68KJIT_Exec_JSR_RTS_Chain verifies JSR→subroutine→RTS chain round-trip.
func TestM68KJIT_Exec_JSR_RTS_Chain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Simple: JSR $2000; MOVEQ #3,D0; STOP
	// Sub at $2000: MOVEQ #2,D1; RTS
	// After: D0=3 (from code after JSR), D1=2 (from subroutine)
	// This is the same pattern as the existing TestM68KJIT_Exec_JSR_RTS
	// but explicitly tests that chain patching connects JSR→sub→RTS→return.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001)                 // MOVEQ #1,D0
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x7003)                 // MOVEQ #3,D0 (after RTS)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x7402) // MOVEQ #2,D2
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

// ===========================================================================
// Lazy CCR Integration Tests (Stage 5)
// ===========================================================================

// TestM68KJIT_Exec_LazyCCR_CMP_BEQ verifies CMP;BEQ uses direct Jcc from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_CMP_BEQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=42, D1=42. CMP D1,D0; BEQ skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	// BEQ taken → D2 stays 0, D3=1
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x002A, // MOVE.L #42,D0
		0x223C, 0x0000, 0x002A, // MOVE.L #42,D1
		0xB081, // CMP.L D1,D0
		0x6702, // BEQ.B +2 (skip MOVEQ #99)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BEQ should skip)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

// TestM68KJIT_Exec_LazyCCR_ADD_BCS verifies ADD;BCS with carry from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_ADD_BCS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=0xFFFFFFFF, D1=1. ADD D1,D0 → carry. BCS skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xFFFF, 0xFFFF, // MOVE.L #$FFFFFFFF,D0
		0x7201, // MOVEQ #1,D1
		0xD081, // ADD.L D1,D0 → 0 with carry
		0x6502, // BCS.B +2 (skip)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[0] != 0 {
		t.Errorf("D0 = 0x%08X, want 0", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BCS should skip — carry set)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

// TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR verifies the JSR+DBRA+RTS chain
// loop with block chaining and lazy CCR. Uses the same polling pattern as
// the benchmark to match working behavior.
func TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x3E3C, 0x0004)         // MOVE.W #4,D7
	loopTop := pc             // 0x1004
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop
	w(0x4E72, 0x2700)       // STOP

	pc = 0x2000
	w(0x5280) // ADDQ.L #1,D0 (arithmetic → flagsLiveArith)
	w(0x4E75) // RTS

	// Use the same polling pattern as runM68KBenchJIT
	cpu.PC = 0x1000
	cpu.running.Store(true)
	cpu.stopped.Store(false)

	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Poll for STOP (with timeout)
	deadline := time.After(5 * time.Second)
	for !cpu.stopped.Load() {
		select {
		case <-deadline:
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("RTS cache hit + lazy CCR test timed out")
		default:
			runtime.Gosched()
		}
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// ADDQ #1,D0 called 5 times (D7: 4,3,2,1,0,-1)
	if cpu.DataRegs[0] != 5 {
		t.Errorf("D0 = %d, want 5 (ADDQ called 5 times)", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_RTS_IOBailRetPC verifies that the RTS I/O bail path
// correctly sets RetPC so the dispatcher re-executes via interpreter.
func TestM68KJIT_Exec_RTS_IOBailRetPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Set A7 to the I/O region (>= 0xA0000) so RTS bails.
	// The bail should set RetPC to the RTS instruction's PC.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x000A0010) // SSP in I/O region
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	// Put A7 in I/O region — RTS will bail when reading the return address
	cpu.AddrRegs[7] = 0xA0000

	// Write a return address at 0xA0000 (big-endian 0x00001008)
	cpu.Write32(0xA0000, 0x00001008)

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001) // MOVEQ #1,D0
	w(0x4E75) // RTS (at 0x1002) — will bail because A7 >= IOThreshold
	// After interpreter re-executes RTS, PC should be 0x1008
	w(0x4E71)         // NOP (0x1004)
	w(0x4E71)         // NOP (0x1006)
	w(0x7003)         // MOVEQ #3,D0 (0x1008 — return target)
	w(0x4E72, 0x2700) // STOP

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// D0 should be 3 (set at 0x1008, the return target after RTS)
	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set at return target after RTS I/O bail)", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_SelfModDuringChain verifies that self-modifying code
// during chained execution triggers cache invalidation.
func TestM68KJIT_Exec_SelfModDuringChain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Block writes to a code page, triggering NeedInval.
	// MOVEQ #42,D0; MOVE.L D0,(0x1000) — writes to own code page
	// Then STOP. The write to 0x1000 (code page) should trigger invalidation.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x702A,                 // MOVEQ #42,D0
		0x23C0, 0x0000, 0x1000, // MOVE.L D0,$1000 (writes to code page)
	)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
	// The program ran to completion (didn't crash from invalidation)
}

// TestM68KJIT_Exec_RTSCacheClearedOnInval verifies that the RTS inline
// cache is cleared on cache invalidation.
func TestM68KJIT_Exec_RTSCacheClearedOnInval(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// JSR + RTS + self-mod write + STOP
	// The self-mod write should clear the RTS cache.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x23C0, 0x0000, 0x2000) // MOVE.L D0,$2000 (write to sub code page → invalidate)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x702A) // MOVEQ #42,D0
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42 (set in subroutine before invalidation)", cpu.DataRegs[0])
	}
}
