// m68k_jit_bcc_fusion_test.go - TDD gates for CMP/Bcc host-flags fusion.
//
// Fusion contract: when CCR is lazily live in host EFLAGS at a Bcc, the
// emitter must branch with a single direct Jcc off the live flags instead
// of materializing the 5-bit CCR into R14 first. Materialization moves to
// the taken (block-exit) path only; the fall-through path keeps flags
// live. Architectural results (registers, CCR, X propagation, loop
// counts) must remain bit-identical to the interpreter.

//go:build amd64 && linux

package main

import (
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// Emit-shape gates (unit level)
// ---------------------------------------------------------------------------

// bccFusionEmit emits a lone Bcc through m68kEmitBcc with a controlled
// lazy flag state and returns the emitted bytes plus the compile state
// after emission.
func bccFusionEmit(t *testing.T, state m68kFlagState, cond uint16) ([]byte, m68kFlagState) {
	t.Helper()
	mem := make([]byte, 0x10000)
	const startPC = 0x1000
	const bccOff = 8 // pretend setup+producer occupy 8 bytes
	instrPC := uint32(startPC + bccOff)
	opcode := uint16(0x6006 | cond<<8) // Bcc.S +6
	mem[instrPC] = byte(opcode >> 8)
	mem[instrPC+1] = byte(opcode)

	cb := NewCodeBuffer(4096)
	cs := &m68kCompileState{flagState: state}
	prev := m68kCurrentCS
	m68kCurrentCS = cs
	defer func() { m68kCurrentCS = prev }()

	var br m68kBlockRegs
	ji := &M68KJITInstr{opcode: opcode, group: 6, pcOffset: bccOff, length: 2}
	m68kEmitBcc(cb, ji, mem, startPC, &br, 4, nil, 8, nil, nil)
	return cb.Bytes(), cs.flagState
}

// firstJccOffset returns the byte offset of the first two-byte Jcc rel32
// (0F 80..8F), or -1.
func firstJccOffset(code []byte) int {
	for i := 0; i+1 < len(code); i++ {
		if code[i] == 0x0F && code[i+1] >= 0x80 && code[i+1] <= 0x8F {
			return i
		}
	}
	return -1
}

// firstSETccOffset returns the byte offset of the first SETcc (0F 90..9F),
// or -1.
func firstSETccOffset(code []byte) int {
	for i := 0; i+1 < len(code); i++ {
		if code[i] == 0x0F && code[i+1] >= 0x90 && code[i+1] <= 0x9F {
			return i
		}
	}
	return -1
}

// TestM68KJIT_BccFusion_DirectJccBeforeMaterialize: with arithmetic flags
// live, the very first emitted control transfer must be the direct Jcc —
// any SETcc materialization may only appear after it (taken path).
func TestM68KJIT_BccFusion_DirectJccBeforeMaterialize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state m68kFlagState
		cond  uint16
	}{
		{"Arith_BEQ", flagsLiveArith, 7},
		{"Arith_BLT", flagsLiveArith, 13},
		{"ArithNoX_BNE", flagsLiveArithNoX, 6},
		{"ArithNoX_BGE", flagsLiveArithNoX, 12},
		{"ArithNoX_BGT", flagsLiveArithNoX, 14},
		{"ArithNoX_BLE", flagsLiveArithNoX, 15},
		{"ArithNoX_BHI", flagsLiveArithNoX, 2},
		{"Logi_BEQ", flagsLiveLogi, 7},
		{"Logi_BMI", flagsLiveLogi, 11},
		{"LogiPreserveVC_BEQ", flagsLiveLogiPreserveVC, 7},
		{"LogiPreserveVC_BPL", flagsLiveLogiPreserveVC, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, after := bccFusionEmit(t, tc.state, tc.cond)
			jcc := firstJccOffset(code)
			setcc := firstSETccOffset(code)
			if jcc < 0 {
				t.Fatal("no Jcc emitted")
			}
			if setcc >= 0 && setcc < jcc {
				t.Fatalf("CCR materialized (SETcc at %d) before direct Jcc (at %d) — fusion not applied", setcc, jcc)
			}
			if after != tc.state {
				t.Fatalf("fall-through flag state must stay live: got %v want %v", after, tc.state)
			}
		})
	}
}

// TestM68KJIT_BccFusion_PreserveVCGatesVCConditions: after AND/OR/EOR the
// architectural V/C are preserved values living in R14, not in EFLAGS
// (host flags have OF=CF=0). Conditions that read V or C must NOT fuse —
// they must materialize and take the R14 bit-test path.
func TestM68KJIT_BccFusion_PreserveVCGatesVCConditions(t *testing.T) {
	for _, tc := range []struct {
		name string
		cond uint16
	}{
		{"BHI", 2}, {"BLS", 3}, {"BCC", 4}, {"BCS", 5},
		{"BVC", 8}, {"BVS", 9},
		{"BGE", 12}, {"BLT", 13}, {"BGT", 14}, {"BLE", 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, after := bccFusionEmit(t, flagsLiveLogiPreserveVC, tc.cond)
			jcc := firstJccOffset(code)
			setcc := firstSETccOffset(code)
			if jcc < 0 {
				t.Fatal("no Jcc emitted")
			}
			if setcc < 0 || setcc > jcc {
				t.Fatalf("V/C-reading condition after PreserveVC producer must materialize before branching (SETcc=%d Jcc=%d)", setcc, jcc)
			}
			if after != flagsMaterialized {
				t.Fatalf("gated path must leave flags materialized, got %v", after)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End-to-end parity gates (JIT vs interpreter)
// ---------------------------------------------------------------------------

func bccFusionRunJIT(cpu *M68KCPU, startPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()
	for !cpu.stopped.Load() {
		runtime.Gosched()
	}
	cpu.running.Store(false)
	<-done
}

func bccFusionRunInterp(cpu *M68KCPU, startPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	for i := 0; i < 1_000_000; i++ {
		if cpu.stopped.Load() {
			break
		}
		cpu.StepOne()
	}
	cpu.running.Store(false)
	cpu.stopped.Store(false)
}

// bccFusionCompare runs the same program via JIT and interpreter and
// compares data registers, CCR, and PC.
func bccFusionCompare(t *testing.T, name string, program []uint16) {
	t.Helper()
	const startPC = 0x1000

	jitCPU := setupM68KJITBenchCPU()
	intCPU := setupM68KJITBenchCPU()
	writeM68KProgram(jitCPU, startPC, program...)
	writeM68KProgram(intCPU, startPC, program...)

	jitCPU.m68kJitEnabled = true
	jitCPU.m68kJitForceNative = true
	jitCPU.m68kJitPersist = true

	bccFusionRunJIT(jitCPU, startPC)
	bccFusionRunInterp(intCPU, startPC)

	for i := 0; i < 8; i++ {
		if jitCPU.DataRegs[i] != intCPU.DataRegs[i] {
			t.Errorf("%s: D%d mismatch jit=%08X interp=%08X", name, i, jitCPU.DataRegs[i], intCPU.DataRegs[i])
		}
	}
	if jitCPU.SR&0x1F != intCPU.SR&0x1F {
		t.Errorf("%s: CCR mismatch jit=%02X interp=%02X", name, jitCPU.SR&0x1F, intCPU.SR&0x1F)
	}
	if jitCPU.PC != intCPU.PC {
		t.Errorf("%s: PC mismatch jit=%08X interp=%08X", name, jitCPU.PC, intCPU.PC)
	}
}

// buildBccGridProgram: D0=a, D1=b, producer, Bcc.S +6 → D2 tells which path.
func buildBccGridProgram(a, b uint32, producer uint16, cond uint16) []uint16 {
	return []uint16{
		0x203C, uint16(a >> 16), uint16(a), // MOVE.L #a,D0
		0x223C, uint16(b >> 16), uint16(b), // MOVE.L #b,D1
		producer,
		0x6006 | cond<<8, // Bcc.S +6
		0x7401,           // MOVEQ #1,D2 (not taken)
		0x4E72, 0x2700,   // STOP
		0x7402,         // MOVEQ #2,D2 (taken)
		0x4E72, 0x2700, // STOP
	}
}

func TestM68KJIT_BccFusion_ParityGrid(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	producers := []struct {
		name string
		op   uint16
	}{
		{"CMP.L", 0xB081},
		{"ADD.L", 0xD081},
		{"SUB.L", 0x9081},
		{"MOVE.L", 0x2001},
		{"AND.L", 0xC081},
		{"OR.L", 0x8081},
	}
	values := [][2]uint32{
		{0, 0}, {5, 5}, {5, 3}, {3, 5},
		{0x80000000, 1}, {0x7FFFFFFF, 0xFFFFFFFF}, {0xFFFFFFFF, 1},
		{0x80000000, 0x7FFFFFFF}, {0x12345678, 0x12345678}, {0, 0xFFFFFFFF},
		{0x80000000, 0x80000000}, {1, 0x80000000},
	}
	for _, p := range producers {
		for cond := uint16(2); cond <= 15; cond++ {
			for _, v := range values {
				name := p.name + "_cc" + string(rune('0'+cond/10)) + string(rune('0'+cond%10))
				bccFusionCompare(t, name, buildBccGridProgram(v[0], v[1], p.op, cond))
			}
		}
	}
}

// TestM68KJIT_BccFusion_XPreservedAcrossFusedBranch: ADD sets X, CMP
// leaves EFLAGS live without touching X, fused BNE branches, ADDX on the
// taken path must see ADD's X — exercises the X stack-slot handoff
// through the fused (non-materializing) branch.
func TestM68KJIT_BccFusion_XPreservedAcrossFusedBranch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, v := range [][2]uint32{
		{0xFFFFFFFF, 1}, // ADD carries → X=1
		{1, 1},          // no carry → X=0
	} {
		program := []uint16{
			0x203C, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #a,D0
			0x223C, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #b,D1
			0x7400,         // MOVEQ #0,D2
			0xD081,         // ADD.L D1,D0 (sets X)
			0xB081,         // CMP.L D1,D0 (flags live, X untouched)
			0x6606,         // BNE.S +6
			0x7401,         // MOVEQ #1,D2
			0x4E72, 0x2700, // STOP
			0xD581,         // ADDX.L D1,D2 (reads X)
			0x4E72, 0x2700, // STOP
		}
		bccFusionCompare(t, "XPreserved", program)
	}
}

// TestM68KJIT_BccFusion_BackwardLoop: fused backward BNE with in-block
// budget arithmetic. Materialization must happen on the taken side before
// the loop-counter math clobbers EFLAGS; loop count and exit CCR must
// match the interpreter.
func TestM68KJIT_BccFusion_BackwardLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	program := []uint16{
		0x203C, 0x0000, 0x0025, // MOVE.L #37,D0
		0x7400, // MOVEQ #0,D2
		// loop:
		0x5282,         // ADDQ.L #1,D2
		0x5380,         // SUBQ.L #1,D0
		0x66FA,         // BNE.S loop (-6)
		0x4E72, 0x2700, // STOP
	}
	bccFusionCompare(t, "BackwardLoop", program)
}

// TestM68KJIT_BccFusion_TwoConsumersOneProducer: one CMP feeds two
// consecutive Bcc's — the second must still see valid flags (fusion keeps
// EFLAGS live across the first, or materializes correctly).
func TestM68KJIT_BccFusion_TwoConsumersOneProducer(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, v := range [][2]uint32{{5, 3}, {3, 5}, {5, 5}, {0x80000000, 1}} {
		program := []uint16{
			0x203C, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #a,D0
			0x223C, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #b,D1
			0x7400,         // MOVEQ #0,D2
			0xB081,         // CMP.L D1,D0
			0x6708,         // BEQ.S +8  → taken1
			0x6D0C,         // BLT.S +12 → taken2 (consumes same CMP flags)
			0x7401,         // MOVEQ #1,D2
			0x4E72, 0x2700, // STOP
			0x7402,         // MOVEQ #2,D2 (taken1)
			0x4E72, 0x2700, // STOP
			0x7403,         // MOVEQ #3,D2 (taken2)
			0x4E72, 0x2700, // STOP
		}
		bccFusionCompare(t, "TwoConsumers", program)
	}
}

// ---------------------------------------------------------------------------
// DBcc fusion gates
// ---------------------------------------------------------------------------

// dbccFusionEmit emits a lone DBcc through m68kEmitDBcc with a controlled
// lazy flag state; returns emitted bytes and the post-emit compile state.
func dbccFusionEmit(t *testing.T, state m68kFlagState, cond uint16) ([]byte, m68kFlagState) {
	t.Helper()
	mem := make([]byte, 0x10000)
	const startPC = 0x1000
	const dbccOff = 8
	instrPC := uint32(startPC + dbccOff)
	opcode := uint16(0x50C8 | cond<<8) // DBcc D0,disp
	mem[instrPC] = byte(opcode >> 8)
	mem[instrPC+1] = byte(opcode)
	// forward displacement +0x10 (no in-block backward loop)
	mem[instrPC+2] = 0x00
	mem[instrPC+3] = 0x10

	cb := NewCodeBuffer(4096)
	cs := &m68kCompileState{flagState: state}
	prev := m68kCurrentCS
	m68kCurrentCS = cs
	defer func() { m68kCurrentCS = prev }()

	var br m68kBlockRegs
	ji := &M68KJITInstr{opcode: opcode, group: 5, pcOffset: dbccOff, length: 4}
	m68kEmitDBcc(cb, ji, mem, startPC, &br, 4, nil, nil, nil)
	return cb.Bytes(), cs.flagState
}

// TestM68KJIT_DBccFusion_DirectJccBeforeMaterialize: with fusable live
// flags, the condition test must be the first control transfer; SETcc
// materialization only after it (continue/exit sides).
func TestM68KJIT_DBccFusion_DirectJccBeforeMaterialize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state m68kFlagState
		cond  uint16
	}{
		{"ArithNoX_DBNE", flagsLiveArithNoX, 6},
		{"ArithNoX_DBEQ", flagsLiveArithNoX, 7},
		{"ArithNoX_DBLT", flagsLiveArithNoX, 13},
		{"Arith_DBGE", flagsLiveArith, 12},
		{"Logi_DBMI", flagsLiveLogi, 11},
		{"LogiPreserveVC_DBEQ", flagsLiveLogiPreserveVC, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, after := dbccFusionEmit(t, tc.state, tc.cond)
			jcc := firstJccOffset(code)
			setcc := firstSETccOffset(code)
			if jcc < 0 {
				t.Fatal("no Jcc emitted")
			}
			if setcc >= 0 && setcc < jcc {
				t.Fatalf("CCR materialized (SETcc at %d) before direct Jcc (at %d) — DBcc fusion not applied", setcc, jcc)
			}
			if after != flagsMaterialized {
				t.Fatalf("DBcc convergence must leave flags materialized, got %v", after)
			}
		})
	}
}

// TestM68KJIT_DBccFusion_DBTKeepsFlagsLive: DBT emits nothing and clobbers
// nothing — live flags must survive it untouched.
func TestM68KJIT_DBccFusion_DBTKeepsFlagsLive(t *testing.T) {
	code, after := dbccFusionEmit(t, flagsLiveArithNoX, 0)
	if len(code) != 0 {
		t.Fatalf("DBT must emit no code, got %d bytes", len(code))
	}
	if after != flagsLiveArithNoX {
		t.Fatalf("DBT must keep flags live, got %v", after)
	}
}

// TestM68KJIT_DBccFusion_DBRAStillMaterializes: DBRA consumes no flags but
// its decrement clobbers EFLAGS — live flags must materialize up front
// (until loop-carried liveness lands).
func TestM68KJIT_DBccFusion_DBRAStillMaterializes(t *testing.T) {
	code, after := dbccFusionEmit(t, flagsLiveArith, 1)
	setcc := firstSETccOffset(code)
	if setcc < 0 {
		t.Fatal("DBRA with live flags must materialize CCR")
	}
	jcc := firstJccOffset(code)
	if jcc >= 0 && jcc < setcc {
		t.Fatalf("DBRA must materialize before its EFLAGS-clobbering body (SETcc=%d Jcc=%d)", setcc, jcc)
	}
	if after != flagsMaterialized {
		t.Fatalf("DBRA must leave flags materialized, got %v", after)
	}
}

// TestM68KJIT_DBccFusion_PreserveVCGatesVCConditions: same V/C gating rule
// as Bcc — after AND/OR/EOR, V/C-reading DBcc conditions must materialize.
func TestM68KJIT_DBccFusion_PreserveVCGatesVCConditions(t *testing.T) {
	for _, tc := range []struct {
		name string
		cond uint16
	}{
		{"DBCS", 5}, {"DBVS", 9}, {"DBGE", 12}, {"DBLE", 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := dbccFusionEmit(t, flagsLiveLogiPreserveVC, tc.cond)
			jcc := firstJccOffset(code)
			setcc := firstSETccOffset(code)
			if jcc < 0 || setcc < 0 {
				t.Fatal("expected both SETcc and Jcc")
			}
			if setcc > jcc {
				t.Fatalf("V/C condition after PreserveVC producer must materialize first (SETcc=%d Jcc=%d)", setcc, jcc)
			}
		})
	}
}

// TestM68KJIT_DBccFusion_ParityGrid: DBcc with a real condition, exercised
// through both exit modes (condition-true exit and counter exhaustion),
// JIT vs interpreter.
func TestM68KJIT_DBccFusion_ParityGrid(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	// D2 counts iterations; loop: ADDQ.L #1,D2; CMP.L D3,D2; DBcc D7,loop.
	// With D3=k the condition fires after k iterations; with D3 unreachable
	// the counter (D7=n) exhausts first.
	build := func(cond uint16, n uint16, k uint32) []uint16 {
		return []uint16{
			0x7400,                             // MOVEQ #0,D2
			0x263C, uint16(k >> 16), uint16(k), // MOVE.L #k,D3
			0x3E3C, n, // MOVE.W #n,D7
			// loop:
			0x5282,                   // ADDQ.L #1,D2
			0xB683,                   // CMP.L D3,D2
			0x50CF | cond<<8, 0xFFF8, // DBcc D7,loop (-8)
			0x4E72, 0x2700, // STOP
		}
	}
	for _, cond := range []uint16{6, 7, 12, 13, 14, 15, 2, 3} {
		for _, tc := range [][2]uint32{
			{5, 3},        // condition path decides
			{5, 100},      // exhaustion path (k unreachable)
			{0, 1},        // immediate
			{5, 6},        // boundary: cond fires on last count
			{5, 0x80DEAD}, // negative-compare territory
		} {
			program := build(cond, uint16(tc[0]), tc[1])
			bccFusionCompare(t, "DBcc", program)
		}
	}
}
