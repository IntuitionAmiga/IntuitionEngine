//go:build headless && m68k_test && amd64

package main

import "testing"

// TestM68KJIT_NativeLoopInstructionCountMatchesInterpreter verifies that a
// native block containing an internal backward-branch loop retires exactly the
// same number of guest instructions as the interpreter. A mismatch means the
// JIT's RetCount/ChainCount/LoopCount accounting over- or under-reports retired
// instructions, which drifts the dispatcher's InstructionCount and therefore the
// instruction-count-keyed deterministic interrupt schedule used by AROS.
func TestM68KJIT_NativeLoopInstructionCountMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)

	// MOVEQ #0,D0 ; loop: ADDQ.L #1,D0 ; CMPI.L #N,D0 ; BNE.S loop ; STOP
	writeProgram := func(cpu *M68KCPU, n uint32) {
		writeM68KWords(cpu, startPC,
			0x7000,                   // 1000 MOVEQ #0,D0
			0x5280,                   // 1002 ADDQ.L #1,D0      (loop body)
			0x0C80,                   // 1004 CMPI.L #N,D0
			uint16(n>>16), uint16(n), // imm32
			0x66F6,         // 100A BNE.S 1002 (back 10 bytes: 1002 - (100A+2))
			0x4E72, 0x2700, // 100C STOP #$2700
		)
	}

	// Chain-loop: two blocks (A at 0x1000, B at 0x1006) that chain to each
	// other to form the loop. Mirrors the AROS resident-scan structure where
	// the loop is realized by backward Bcc/BRA chain exits, not an internal
	// block loop. A: ADDQ.L #1,D0 ; BRA.S B    B: CMPI.L #N,D0 ; BNE.S A ; STOP
	writeChainLoop := func(cpu *M68KCPU, n uint32) {
		writeM68KWords(cpu, startPC,
			0x5280,         // 1000 ADDQ.L #1,D0       (block A)
			0x6000, 0x0002, // 1002 BRA.W 1006         (block A terminator -> chain to B)
			0x0C80, // 1006 CMPI.L #N,D0       (block B start)
			uint16(n>>16), uint16(n),
			0x6600, 0xFFF2, // 100C BNE.W 1000         (chain back to A)
			0x4E72, 0x2700, // 1010 STOP
		)
	}

	runCase := func(t *testing.T, label string, n uint32, write func(*M68KCPU, uint32)) {
		t.Run(label, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			write(interp, n)
			var wantCount uint64
			for i := 0; i < 1<<20 && !interp.stopped.Load(); i++ {
				if interp.StepOne() == 0 {
					break
				}
				wantCount++
			}
			if !interp.stopped.Load() {
				t.Fatalf("%s N=%d: interpreter did not STOP", label, n)
			}

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			jit.m68kJitForceNative = true
			write(jit, n)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s N=%d: no native block executed", label, n)
			}
			if jit.DataRegs[0] != interp.DataRegs[0] {
				t.Fatalf("%s N=%d: D0 mismatch jit=%d interp=%d", label, n, jit.DataRegs[0], interp.DataRegs[0])
			}
			if jit.InstructionCount != wantCount {
				t.Fatalf("%s N=%d: JIT InstructionCount=%d want=%d (delta=%+d) native_blocks=%d ret_sum=%d chain_sum=%d",
					label, n, jit.InstructionCount, wantCount,
					int64(jit.InstructionCount)-int64(wantCount),
					jit.m68kJitNativeBlocksExecuted.Load(),
					jit.m68kJitNativeRetCountSum.Load(),
					jit.m68kJitNativeChainCountSum.Load())
			}
		})
	}
	for _, n := range []uint32{1, 2, 16, 1000} {
		runCase(t, "chainloop", n, writeChainLoop)
	}

	// Nested within-block loop: inner loop (3 iters) inside outer loop (N iters).
	// MOVEQ #0,D1; outer: ADDQ #1,D1; MOVEQ #0,D0; inner: ADDQ #1,D0;
	// CMPI.L #3,D0; BNE inner; CMPI.L #N,D1; BNE outer; STOP
	writeNested := func(cpu *M68KCPU, n uint32) {
		writeM68KWords(cpu, startPC,
			0x7200,                 // 1000 MOVEQ #0,D1
			0x5281,                 // 1002 ADDQ.L #1,D1   (outer)
			0x7000,                 // 1004 MOVEQ #0,D0
			0x5280,                 // 1006 ADDQ.L #1,D0   (inner)
			0x0C80, 0x0000, 0x0003, // 1008 CMPI.L #3,D0
			0x66F6,                           // 100E BNE.S inner (1006)
			0x0C81, uint16(n>>16), uint16(n), // 1010 CMPI.L #N,D1
			0x66EA,         // 1016 BNE.S outer (1002)
			0x4E72, 0x2700, // 1018 STOP
		)
	}
	for _, n := range []uint32{1, 2, 8, 200} {
		runCase(t, "nested", n, writeNested)
	}

	// Forward Bcc (skip) followed by fall-through terminator — no loop.
	// MOVEQ #5,D0; CMPI.L #5,D0; BNE skip; MOVEQ #1,D1; skip: STOP
	writeForward := func(cpu *M68KCPU, _ uint32) {
		writeM68KWords(cpu, startPC,
			0x700A,                 // 1000 MOVEQ #10,D0
			0x0C80, 0x0000, 0x0005, // 1002 CMPI.L #5,D0
			0x6602,         // 1008 BNE.S skip (100C) -> taken (10!=5)
			0x7201,         // 100A MOVEQ #1,D1 (skipped)
			0x4E72, 0x2700, // 100C STOP
		)
	}
	runCase(t, "forward", 0, writeForward)

	for _, n := range []uint32{1, 2, 16, 1000} {
		t.Run("", func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			writeProgram(interp, n)
			var wantCount uint64
			for i := 0; i < 1<<20 && !interp.stopped.Load(); i++ {
				if interp.StepOne() == 0 {
					break
				}
				wantCount++
			}
			if !interp.stopped.Load() {
				t.Fatalf("N=%d: interpreter did not STOP", n)
			}

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			jit.m68kJitForceNative = true
			writeProgram(jit, n)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("N=%d: no native block executed", n)
			}
			if jit.DataRegs[0] != interp.DataRegs[0] {
				t.Fatalf("N=%d: D0 mismatch jit=%d interp=%d", n, jit.DataRegs[0], interp.DataRegs[0])
			}
			if jit.InstructionCount != wantCount {
				t.Fatalf("N=%d: JIT InstructionCount=%d want=%d (delta=%+d) native_blocks=%d ret_sum=%d chain_sum=%d",
					n, jit.InstructionCount, wantCount,
					int64(jit.InstructionCount)-int64(wantCount),
					jit.m68kJitNativeBlocksExecuted.Load(),
					jit.m68kJitNativeRetCountSum.Load(),
					jit.m68kJitNativeChainCountSum.Load())
			}
		})
	}
}
