//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

func TestM68KJIT_HarteInterpreterPassingProductionParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	files := []string{
		"BTST.json.gz",
		"BCHG.json.gz",
		"BCLR.json.gz",
		"BSET.json.gz",
		"ANDItoCCR.json.gz",
		"ANDItoSR.json.gz",
		"EORItoCCR.json.gz",
		"EORItoSR.json.gz",
		"ORItoCCR.json.gz",
		"ORItoSR.json.gz",
		"MOVEP.w.json.gz",
		"MOVEP.l.json.gz",
		"Bcc.json.gz",
		"BSR.json.gz",
		"DBcc.json.gz",
		"JMP.json.gz",
		"JSR.json.gz",
		"RTS.json.gz",
		"MOVE.b.json.gz",
		"MOVE.w.json.gz",
		"MOVE.l.json.gz",
		"MOVE.q.json.gz",
		"MOVEA.w.json.gz",
		"MOVEA.l.json.gz",
		"MOVEM.w.json.gz",
		"MOVEM.l.json.gz",
		"MOVEfromSR.json.gz",
		"MOVEtoSR.json.gz",
		"MOVEtoCCR.json.gz",
		"MOVEfromUSP.json.gz",
		"MOVEtoUSP.json.gz",
		"CHK.json.gz",
		"NOP.json.gz",
		"LEA.json.gz",
		"PEA.json.gz",
		"LINK.json.gz",
		"UNLINK.json.gz",
		"CLR.b.json.gz",
		"CLR.w.json.gz",
		"CLR.l.json.gz",
		"NEG.b.json.gz",
		"NEG.w.json.gz",
		"NEG.l.json.gz",
		"NEGX.b.json.gz",
		"NEGX.w.json.gz",
		"NEGX.l.json.gz",
		"NBCD.json.gz",
		"NOT.b.json.gz",
		"NOT.w.json.gz",
		"NOT.l.json.gz",
		"TST.b.json.gz",
		"TST.w.json.gz",
		"TST.l.json.gz",
		"TAS.json.gz",
		"EXT.w.json.gz",
		"EXT.l.json.gz",
		"SWAP.json.gz",
		"ADD.b.json.gz",
		"ADD.w.json.gz",
		"ADD.l.json.gz",
		"ADDX.b.json.gz",
		"ADDX.w.json.gz",
		"ADDX.l.json.gz",
		"ADDA.w.json.gz",
		"ADDA.l.json.gz",
		"SUB.b.json.gz",
		"SUB.w.json.gz",
		"SUB.l.json.gz",
		"SUBX.b.json.gz",
		"SUBX.w.json.gz",
		"SUBX.l.json.gz",
		"SUBA.w.json.gz",
		"SUBA.l.json.gz",
		"SBCD.json.gz",
		"AND.b.json.gz",
		"AND.w.json.gz",
		"AND.l.json.gz",
		"OR.b.json.gz",
		"OR.w.json.gz",
		"OR.l.json.gz",
		"ABCD.json.gz",
		"EXG.json.gz",
		"MULU.json.gz",
		"MULS.json.gz",
		"DIVU.json.gz",
		"DIVS.json.gz",
		"EOR.b.json.gz",
		"EOR.w.json.gz",
		"EOR.l.json.gz",
		"CMP.b.json.gz",
		"CMP.w.json.gz",
		"CMP.l.json.gz",
		"CMPA.w.json.gz",
		"CMPA.l.json.gz",
		"Scc.json.gz",
		"ASL.b.json.gz",
		"ASL.w.json.gz",
		"ASL.l.json.gz",
		"ASR.b.json.gz",
		"ASR.w.json.gz",
		"ASR.l.json.gz",
		"LSL.b.json.gz",
		"LSL.w.json.gz",
		"LSL.l.json.gz",
		"LSR.b.json.gz",
		"LSR.w.json.gz",
		"LSR.l.json.gz",
		"ROXL.b.json.gz",
		"ROXL.w.json.gz",
		"ROXL.l.json.gz",
		"ROXR.b.json.gz",
		"ROXR.w.json.gz",
		"ROXR.l.json.gz",
		"ROL.b.json.gz",
		"ROL.w.json.gz",
		"ROL.l.json.gz",
		"ROR.b.json.gz",
		"ROR.w.json.gz",
		"ROR.l.json.gz",
	}
	fileFilterActive := false
	if raw := os.Getenv("IE_HARTE_JIT_FILES"); raw != "" {
		fileFilterActive = true
		want := make(map[string]bool)
		requested := make([]string, 0)
		for _, field := range strings.Split(raw, ",") {
			name := strings.TrimSpace(field)
			if name == "" {
				continue
			}
			if !strings.HasSuffix(name, ".json.gz") {
				name += ".json.gz"
			}
			if !want[name] {
				requested = append(requested, name)
				want[name] = true
			}
		}
		configured := make(map[string]bool, len(files))
		for _, name := range files {
			configured[name] = true
		}
		filtered := make([]string, 0, len(requested))
		for _, name := range requested {
			if configured[name] {
				filtered = append(filtered, name)
				continue
			}
			if _, err := os.Stat(filepath.Join(harteTestDir, name)); err == nil {
				filtered = append(filtered, name)
			}
		}
		if len(filtered) == 0 {
			t.Fatalf("IE_HARTE_JIT_FILES=%q matched no available Harte parity files", raw)
		}
		files = filtered
	}

	longRun := os.Getenv("IE_HARTE_JIT_LONG") == "1"
	shortAdmissionLimit := 64
	if raw := os.Getenv("IE_HARTE_JIT_CASE_LIMIT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			t.Fatalf("invalid IE_HARTE_JIT_CASE_LIMIT=%q", raw)
		}
		shortAdmissionLimit = limit
	}
	totalAdmitted := 0
	totalInterpreterSkipped := 0
	totalUnsupported := 0
	totalFiles := 0
	totalZeroAdmitted := 0
	totalUnsupportedOpcodes := make(map[uint16]int)
	coverageByKey := make(map[m68kHarteJITCoverageKey]*m68kHarteJITCoverageRow)

	for _, name := range files {
		file := filepath.Join(harteTestDir, name)
		tests, err := LoadHarteTests(file)
		if err != nil {
			t.Skipf("Tom Harte test file %s not available: %v", file, err)
		}

		t.Run(name, func(t *testing.T) {
			admitted, interpreterSkipped, unsupported := 0, 0, 0
			traceUnsupported := os.Getenv("IE_HARTE_JIT_TRACE_UNSUPPORTED") == "1"
			unsupportedByOpcode := make(map[uint16]int)
			unsupportedSamples := make([]string, 0, 16)
			for _, tc := range tests {
				opcode := harteInitialOpcode(tc)
				row := m68kHarteJITCoverageRowFor(coverageByKey, name, opcode)
				interp, interpState := RunHarteInterpreterParityState(tc)
				if !interp.Passed {
					interpreterSkipped++
					row.InterpreterSkipped++
					continue
				}
				row.InterpreterPassing++

				jitResult, ok := RunHarteJITProductionTest(tc, interpState)
				if !ok {
					unsupported++
					row.Unsupported++
					totalUnsupportedOpcodes[opcode]++
					if traceUnsupported {
						unsupportedByOpcode[opcode]++
						if len(unsupportedSamples) < 16 {
							unsupportedSamples = append(unsupportedSamples, fmt.Sprintf("%s opcode=%04X %s", tc.Name, opcode, m68kTraceOpcodeEA(opcode)))
						}
					}
					continue
				}
				admitted++
				row.Admitted++
				totalAdmitted++
				if !jitResult.Passed {
					t.Fatalf("JIT mismatch for interpreter-passing Harte case %s: %v", tc.Name, jitResult.Mismatches)
				}
				if !longRun && admitted >= shortAdmissionLimit {
					break
				}
			}
			totalFiles++
			totalInterpreterSkipped += interpreterSkipped
			totalUnsupported += unsupported
			if admitted == 0 {
				totalZeroAdmitted++
				t.Logf("%s: no interpreter-passing Harte cases admitted by production JIT gate; interpreterSkipped=%d unsupported=%d",
					name, interpreterSkipped, unsupported)
			} else {
				t.Logf("%s: verified %d interpreter-passing production-JIT cases; skipped interpreter=%d unsupported=%d",
					name, admitted, interpreterSkipped, unsupported)
			}
			if traceUnsupported && unsupported > 0 {
				t.Logf("%s: unsupported opcode counts: %s", name, m68kFormatOpcodeCounts(unsupportedByOpcode))
				t.Logf("%s: unsupported samples: %s", name, strings.Join(unsupportedSamples, "; "))
			}
		})
	}

	if totalAdmitted == 0 {
		t.Fatal("no Harte cases were admitted by the production JIT gate")
	}
	t.Logf("Harte JIT parity summary: files=%d verified=%d interpreter_skipped=%d unsupported_interpreter_passing=%d unsupported_unique_opcodes=%d zero_admitted_files=%d case_limit=%d long=%v",
		totalFiles, totalAdmitted, totalInterpreterSkipped, totalUnsupported, len(totalUnsupportedOpcodes), totalZeroAdmitted, shortAdmissionLimit, longRun)
	if totalUnsupported > 0 {
		t.Logf("Harte JIT parity unsupported opcode counts: %s", m68kFormatOpcodeCounts(totalUnsupportedOpcodes))
	}
	if os.Getenv("IE_HARTE_JIT_MATRIX") == "1" {
		path := os.Getenv("IE_HARTE_JIT_MATRIX_PATH")
		scope := "global"
		if fileFilterActive {
			scope = "filtered"
		} else if !longRun {
			scope = "short"
		}
		if path == "" {
			if fileFilterActive {
				t.Log("IE_HARTE_JIT_FILES is set; not overwriting global m68k_jit_harte_coverage_matrix.tsv without IE_HARTE_JIT_MATRIX_PATH")
				return
			}
			if !longRun {
				t.Log("short Harte JIT parity run; not overwriting global m68k_jit_harte_coverage_matrix.tsv without IE_HARTE_JIT_MATRIX_PATH")
				return
			}
			path = "m68k_jit_harte_coverage_matrix.tsv"
		}
		matrix := m68kFormatHarteJITCoverageMatrix(coverageByKey, scope, longRun, shortAdmissionLimit)
		t.Logf("Harte JIT parity coverage matrix:\n%s", matrix)
		if err := os.WriteFile(path, []byte(matrix+"\n"), 0o644); err != nil {
			t.Fatalf("write Harte JIT matrix %q: %v", path, err)
		}
	}
	if os.Getenv("IE_HARTE_JIT_FAIL_UNSUPPORTED") == "1" && totalUnsupported > 0 {
		t.Fatalf("production JIT left %d interpreter-passing Harte cases unsupported", totalUnsupported)
	}
}

type m68kHarteJITCoverageKey struct {
	File   string
	Opcode uint16
}

type m68kHarteJITCoverageRow struct {
	File               string
	Opcode             uint16
	InterpreterPassing int
	Admitted           int
	Unsupported        int
	InterpreterSkipped int
}

func m68kHarteJITCoverageRowFor(rows map[m68kHarteJITCoverageKey]*m68kHarteJITCoverageRow, file string, opcode uint16) *m68kHarteJITCoverageRow {
	key := m68kHarteJITCoverageKey{File: file, Opcode: opcode}
	row := rows[key]
	if row == nil {
		row = &m68kHarteJITCoverageRow{File: file, Opcode: opcode}
		rows[key] = row
	}
	return row
}

func m68kFormatHarteJITCoverageMatrix(rows map[m68kHarteJITCoverageKey]*m68kHarteJITCoverageRow, scope string, longRun bool, caseLimit int) string {
	header := "scope\tlong_run\tcase_limit\tfile\topcode\tea\tinterpreter_passing\tjit_admitted\tjit_unsupported\tinterpreter_skipped"
	if len(rows) == 0 {
		return header
	}
	ordered := make([]*m68kHarteJITCoverageRow, 0, len(rows))
	for _, row := range rows {
		ordered = append(ordered, row)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].File != ordered[j].File {
			return ordered[i].File < ordered[j].File
		}
		return ordered[i].Opcode < ordered[j].Opcode
	})
	var b strings.Builder
	b.WriteString(header)
	for _, row := range ordered {
		fmt.Fprintf(&b, "\n%s\t%t\t%d\t%s\t%04X\t%s\t%d\t%d\t%d\t%d",
			scope, longRun, caseLimit, row.File, row.Opcode, m68kTraceOpcodeEA(row.Opcode),
			row.InterpreterPassing, row.Admitted, row.Unsupported, row.InterpreterSkipped)
	}
	return b.String()
}

func harteInitialOpcode(tc HarteTestCase) uint16 {
	if len(tc.Initial.Prefetch) > 0 {
		return uint16(tc.Initial.Prefetch[0])
	}
	return 0
}

func m68kTraceOpcodeEA(opcode uint16) string {
	group := opcode >> 12
	switch group {
	case 0x1, 0x2, 0x3:
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return fmt.Sprintf("src=%d/%d dst=%d/%d", srcMode, srcReg, dstMode, dstReg)
	default:
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		return fmt.Sprintf("ea=%d/%d", mode, reg)
	}
}

func m68kFormatOpcodeCounts(counts map[uint16]int) string {
	if len(counts) == 0 {
		return ""
	}
	type opcodeCount struct {
		opcode uint16
		count  int
	}
	items := make([]opcodeCount, 0, len(counts))
	for opcode, count := range counts {
		items = append(items, opcodeCount{opcode: opcode, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].opcode < items[j].opcode
	})
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%04X:%d(%s)", item.opcode, item.count, m68kTraceOpcodeEA(item.opcode)))
	}
	return strings.Join(parts, ", ")
}

type harteParityState struct {
	D      [8]uint32
	A      [8]uint32
	USP    uint32
	SR     uint16
	PC     uint32
	RAM    [][2]uint32
	Memory []byte
}

func RunHarteInterpreterParityState(tc HarteTestCase) (HarteTestResult, harteParityState) {
	cpu := getHarteTestCPU()
	resetHarteTestCPU(cpu)
	SetupHarteCPUState(cpu, tc.Initial)

	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	result := VerifyHarteFinalState(cpu, tc.Final, tc.Name)
	state := captureHarteParityState(cpu, tc.Final)
	return result, state
}

func captureHarteParityState(cpu *M68KCPU, expected HarteState) harteParityState {
	var state harteParityState
	copy(state.D[:], cpu.DataRegs[:])
	copy(state.A[:], cpu.AddrRegs[:])
	state.USP = cpu.USP
	state.SR = cpu.SR
	state.PC = cpu.PC & M68K_ADDRESS_MASK
	state.RAM = make([][2]uint32, 0, len(expected.RAM))
	state.Memory = make([]byte, 0, len(expected.RAM))
	for _, entry := range expected.RAM {
		if len(entry) < 2 {
			continue
		}
		addr := entry[0] & M68K_ADDRESS_MASK
		state.RAM = append(state.RAM, [2]uint32{addr, entry[1]})
		if addr < uint32(len(cpu.memory)) {
			state.Memory = append(state.Memory, cpu.memory[addr])
		} else {
			state.Memory = append(state.Memory, 0)
		}
	}
	return state
}

func VerifyHarteJITMatchesInterpreter(cpu *M68KCPU, want harteParityState, testName string) HarteTestResult {
	result := HarteTestResult{TestName: testName, Passed: true}
	mismatch := func(format string, args ...any) {
		result.Passed = false
		result.Mismatches = append(result.Mismatches, fmt.Sprintf(format, args...))
	}
	for i, exp := range want.D {
		if cpu.DataRegs[i] != exp {
			mismatch("D%d: got 0x%08X, interpreter 0x%08X", i, cpu.DataRegs[i], exp)
		}
	}
	for i, exp := range want.A {
		if cpu.AddrRegs[i] != exp {
			mismatch("A%d: got 0x%08X, interpreter 0x%08X", i, cpu.AddrRegs[i], exp)
		}
	}
	if cpu.USP != want.USP {
		mismatch("USP: got 0x%08X, interpreter 0x%08X", cpu.USP, want.USP)
	}
	if cpu.SR != want.SR {
		mismatch("SR: got 0x%04X, interpreter 0x%04X", cpu.SR, want.SR)
	}
	if gotPC := cpu.PC & M68K_ADDRESS_MASK; gotPC != want.PC {
		mismatch("PC: got 0x%08X, interpreter 0x%08X", cpu.PC, want.PC)
	}
	for i, entry := range want.RAM {
		addr := entry[0]
		wantVal := want.Memory[i]
		var gotVal uint8
		if addr < uint32(len(cpu.memory)) {
			gotVal = cpu.memory[addr]
		}
		if gotVal != wantVal {
			mismatch("RAM[0x%06X]: got 0x%02X, interpreter 0x%02X", addr, gotVal, wantVal)
		}
	}
	return result
}

func RunHarteJITProductionTest(tc HarteTestCase, interpreterFinal harteParityState) (HarteTestResult, bool) {
	rig, err := newM68KJITTestRigForHarte()
	if err != nil {
		return HarteTestResult{TestName: tc.Name, Passed: false, Mismatches: []string{err.Error()}}, true
	}

	cpu := rig.cpu
	resetHarteTestCPU(cpu)
	SetupHarteCPUState(cpu, tc.Initial)
	startPC := cpu.PC & M68K_ADDRESS_MASK
	cpu.PC = startPC

	instrs := m68kScanBlock(cpu.memory, startPC)
	if len(instrs) == 0 {
		return HarteTestResult{TestName: tc.Name, Passed: false, Mismatches: []string{"m68kScanBlock returned no instructions"}}, false
	}
	instrs = instrs[:1]
	if !m68kCanUseProductionNativeBlock(cpu.memory, startPC, instrs) {
		return HarteTestResult{TestName: tc.Name, Passed: true}, false
	}

	rig.execMem.Reset()
	clear(rig.codeBitmap)
	clear(rig.pageMin)
	clear(rig.pageMax)
	block, err := m68kCompileBlockWithMem(instrs, startPC, rig.execMem, cpu.memory)
	if err != nil {
		return HarteTestResult{TestName: tc.Name, Passed: false, Mismatches: []string{err.Error()}}, true
	}

	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&cpu.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&cpu.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&cpu.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&cpu.SR))
	rig.ctx.USPPtr = uintptr(unsafe.Pointer(&cpu.USP))
	rig.ctx.SSPPtr = uintptr(unsafe.Pointer(&cpu.SSP))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0
	rig.ctx.NeedHelper = m68kJITHelperNone
	rig.ctx.HelperPC = 0
	rig.ctx.RetCount = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	cpu.PC = rig.ctx.RetPC
	if rig.ctx.NeedHelper != m68kJITHelperNone {
		if _, ok := cpu.m68kHandleJITHelper(rig.ctx); !ok {
			return HarteTestResult{TestName: tc.Name, Passed: false, Mismatches: []string{"JIT helper did not execute"}}, true
		}
	}
	if rig.ctx.NeedIOFallback != 0 {
		rig.ctx.NeedIOFallback = 0
		if cycles := cpu.StepOne(); cycles == 0 {
			return HarteTestResult{TestName: tc.Name, Passed: false, Mismatches: []string{"interpreter fallback did not execute"}}, true
		}
	}

	return VerifyHarteJITMatchesInterpreter(cpu, interpreterFinal, tc.Name), true
}

type m68kHarteJITRig struct {
	cpu        *M68KCPU
	execMem    *ExecMem
	ctx        *M68KJITContext
	codeBitmap []byte
	pageMin    []uint16
	pageMax    []uint16
}

var harteJITRig *m68kHarteJITRig

func newM68KJITTestRigForHarte() (*m68kHarteJITRig, error) {
	if harteJITRig != nil {
		return harteJITRig, nil
	}
	bus := NewMachineBus()
	mem := bus.GetMemory()
	cpu := &M68KCPU{
		SR:              M68K_SR_S,
		bus:             bus,
		memory:          mem,
		memBase:         unsafe.Pointer(&mem[0]),
		stackLowerBound: 0,
		stackUpperBound: 0xFFFFFFFF,
	}
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		return nil, fmt.Errorf("AllocExecMem: %w", err)
	}
	codeBitmap := make([]byte, (uint32(len(mem))+4095)>>12)
	pageMin := make([]uint16, len(codeBitmap))
	pageMax := make([]uint16, len(codeBitmap))
	ctx := newM68KJITContext(cpu, codeBitmap, pageMin, pageMax)
	harteJITRig = &m68kHarteJITRig{
		cpu:        cpu,
		execMem:    execMem,
		ctx:        ctx,
		codeBitmap: codeBitmap,
		pageMin:    pageMin,
		pageMax:    pageMax,
	}
	return harteJITRig, nil
}
