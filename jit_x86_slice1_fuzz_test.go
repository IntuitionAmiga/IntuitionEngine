// jit_x86_slice1_fuzz_test.go - Slice-1 differential fuzz: native x86 JIT
// vs interpreter parity for a narrow ISA cut (ALU r,r / r,imm32, MOV r,imm32,
// MOV r/m32 register copy, MOV [disp32]/[base+disp8] memory ops, no branches,
// no segments, no stack, no string/REP). Drives the slice-1 force-native path.
//
// Usage:
//   go test -run TestX86JIT_Slice1Fuzz                # default 200 iters
//   X86_FUZZ_ITERS=5000 go test -run TestX86JIT_Slice1Fuzz
//   X86_FUZZ_SEED=42 go test -run TestX86JIT_Slice1Fuzz
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	x86Slice1FuzzProgPC    uint32 = 0x10000
	x86Slice1FuzzMemPC     uint32 = 0x20000
	x86Slice1FuzzMemSize   uint32 = 0x1000
	x86Slice1FuzzMaxBranch        = 16 // bytes — generator caps the per-instr footprint

	// Stack region (slice 2): the high half of the scratch RAM is the
	// guest stack. ESP starts at FuzzStackTop and the generator tracks
	// PUSH/POP balance so it never underflows into the data half or
	// overflows past the top of scratch. CALL/RET in slice 2 share the
	// same stack region.
	x86Slice2FuzzStackTop      uint32 = x86Slice1FuzzMemPC + 0xC00 // grows down
	x86Slice2FuzzStackBottom   uint32 = x86Slice1FuzzMemPC + 0x800
	x86Slice2FuzzMaxStackDepth        = 64 // bytes of pushed data; 16 PUSH r32 fits
)

// x86FuzzState carries generator state across instructions: stack depth
// (in bytes pushed since program start, must never exceed
// x86Slice2FuzzMaxStackDepth or underflow below 0). Tracking depth lets
// the generator gate PUSH on room-available and POP on stack-non-empty
// without producing programs that fault out of the fuzz scratch range.
type x86FuzzState struct {
	stackDepth int
	// noStackOps: when true, AppendInstr will not emit PUSH/POP. Used
	// for the post-HLT function body so it never leaves uneven push
	// counts when RET pops the caller's pushed retPC. Function bodies
	// must end with funcState.stackDepth == 0.
	noStackOps bool
}

// x86Slice1FuzzAppendInstr picks a random slice-1-eligible x86 encoding and
// appends its bytes to code. Returns the new slice. Generator is intentionally
// narrow:
//   - MOV r32, imm32                              (B8+r imm32)
//   - MOV r/m32, r32     mod=11                   (89 /r)
//   - ALU r/m32, r32     mod=11                   (01/09/21/29/31/39 /r)
//   - ALU r32, imm32                              (Grp1 81 /n imm32)
//   - MOV [disp32], r32                           (89 /r mod=00 rm=101 disp32)
//   - MOV r32, [disp32]                           (8B /r mod=00 rm=101 disp32)
//   - MOV [base+disp8], r32  (base ∈ {ESI, EDI})  (89 /r mod=01 rm=base disp8)
//   - MOV r32, [base+disp8]                       (8B /r mod=01 rm=base disp8)
//
// Slice 1 deliberately omits ESP/EBP from the register pool (their r/m
// encodings overlap SIB / disp32-only forms that need separate validation),
// and omits PUSH/POP/CALL/RET/Jcc/JMP/REP/segment/SIB.
func x86Slice1FuzzAppendInstr(rng *rand.Rand, code []byte, state *x86FuzzState) []byte {
	// Slice 1/2: writable pool excludes ESP (4) / EBP (5) / ESI (6) / EDI
	// (7). ESP is mutated by PUSH/POP and CALL/RET; ESI/EDI are pinned
	// as memory bases; EBP is reserved for slice-3 SIB encodings.
	regPool := [...]byte{0 /*EAX*/, 1 /*ECX*/, 2 /*EDX*/, 3 /*EBX*/}
	pickReg := func() byte { return regPool[rng.IntN(len(regPool))] }
	pickBase := func() byte {
		// ESI/EDI: no SIB required, no special r/m=101 disp32 collision at mod=01.
		if rng.IntN(2) == 0 {
			return 6
		}
		return 7
	}

	// Slice 2 stack ops gated on depth budget and the noStackOps flag
	// (function bodies must keep stack-balanced so RET pops the right
	// value).
	if !state.noStackOps && rng.IntN(20) == 0 && state.stackDepth+4 <= x86Slice2FuzzMaxStackDepth {
		// PUSH r32 (0x50+r). Skip ESP (r=4) — IA32 PUSH ESP semantics
		// (pushes original) are valid but slice 2 keeps the model
		// simple by not testing it.
		r := pickReg()
		state.stackDepth += 4
		return append(code, 0x50+r)
	}
	if !state.noStackOps && rng.IntN(20) == 0 && state.stackDepth >= 4 {
		// POP r32 (0x58+r).
		r := pickReg()
		state.stackDepth -= 4
		return append(code, 0x58+r)
	}

	switch rng.IntN(8) {
	case 0: // MOV r32, imm32
		r := pickReg()
		imm := rng.Uint32()
		code = append(code, 0xB8+r)
		code = append(code, byte(imm), byte(imm>>8), byte(imm>>16), byte(imm>>24))

	case 1: // MOV r/m32, r32 (mod=11): r/m = guest dst, reg = guest src
		src := pickReg()
		dst := pickReg()
		code = append(code, 0x89, 0xC0|(src<<3)|dst)

	case 2: // ALU r/m32, r32 (mod=11)
		ops := [...]byte{0x01 /*ADD*/, 0x09 /*OR*/, 0x21 /*AND*/, 0x29 /*SUB*/, 0x31 /*XOR*/, 0x39 /*CMP*/}
		op := ops[rng.IntN(len(ops))]
		src := pickReg()
		dst := pickReg()
		code = append(code, op, 0xC0|(src<<3)|dst)

	case 3: // ALU r32, imm32 — Grp1 81 /n
		ns := [...]byte{0 /*ADD*/, 1 /*OR*/, 4 /*AND*/, 5 /*SUB*/, 6 /*XOR*/, 7 /*CMP*/}
		n := ns[rng.IntN(len(ns))]
		r := pickReg()
		imm := rng.Uint32()
		code = append(code, 0x81, 0xC0|(n<<3)|r)
		code = append(code, byte(imm), byte(imm>>8), byte(imm>>16), byte(imm>>24))

	case 4: // MOV [disp32], r32
		// Restrict to data half [memPC, stackBottom) globally — a store
		// inside the stack region could clobber a CALL's pushed retAddr,
		// causing both interp and JIT to RET into garbage and loop on
		// whatever the bus returns. Stack writes happen via PUSH only.
		r := pickReg()
		disp := x86Slice1FuzzMemPC + uint32(rng.IntN(int(x86Slice2FuzzStackBottom-x86Slice1FuzzMemPC)-4))
		code = append(code, 0x89, 0x05|(r<<3))
		code = append(code, byte(disp), byte(disp>>8), byte(disp>>16), byte(disp>>24))

	case 5: // MOV r32, [disp32]
		r := pickReg()
		disp := x86Slice1FuzzMemPC + uint32(rng.IntN(int(x86Slice2FuzzStackBottom-x86Slice1FuzzMemPC)-4))
		code = append(code, 0x8B, 0x05|(r<<3))
		code = append(code, byte(disp), byte(disp>>8), byte(disp>>16), byte(disp>>24))

	case 6: // MOV [base+disp8], r32
		base := pickBase()
		src := pickReg()
		disp := byte(rng.IntN(120))
		code = append(code, 0x89, 0x40|(src<<3)|base, disp)

	case 7: // MOV r32, [base+disp8]
		base := pickBase()
		dst := pickReg()
		disp := byte(rng.IntN(120))
		code = append(code, 0x8B, 0x40|(dst<<3)|base, disp)
	}
	return code
}

// x86Slice1FuzzInstrSpec describes one generator-chosen instruction. For
// non-branch instrs `bytes` is the final encoding. For branches, `bytes`
// contains the opcode + ModR/M + a placeholder rel32 (zeros); the layout
// pass back-patches the rel32 once instr offsets are known. `skipInstrs`
// is the forward instruction count to skip past — the branch target is
// the instruction starting at index i+1+skipInstrs in the program.
//
// Slice 2: when callTarget is true the rel32 is patched to point at the
// post-HLT function block (CALL rel32). The function executes, ends in
// RET, and control returns to the byte after the CALL — which is the
// next slot in the main program.
type x86Slice1FuzzInstrSpec struct {
	bytes      []byte
	branch     bool
	rel32Off   int // byte offset within bytes where the rel32 starts
	skipInstrs int
	callTarget bool // CALL rel32 to appended function block
}

// x86Slice1FuzzAppendBranch chooses a forward conditional or unconditional
// branch and returns its placeholder bytes + rel32 offset + skip distance.
// Branches in slice 1 are forward-only with small skip distances, which
// rules out infinite loops without needing a separate timeout watchdog.
func x86Slice1FuzzAppendBranch(rng *rand.Rand) x86Slice1FuzzInstrSpec {
	skip := 1 + rng.IntN(3) // skip 1..3 instrs forward
	if rng.IntN(4) == 0 {
		// Unconditional JMP rel32 (E9 disp32). 5 bytes total.
		return x86Slice1FuzzInstrSpec{
			bytes:      []byte{0xE9, 0, 0, 0, 0},
			branch:     true,
			rel32Off:   1,
			skipInstrs: skip,
		}
	}
	// Jcc rel32 (0F 80+cond disp32). 6 bytes total.
	cond := byte(rng.IntN(16))
	return x86Slice1FuzzInstrSpec{
		bytes:      []byte{0x0F, 0x80 | cond, 0, 0, 0, 0},
		branch:     true,
		rel32Off:   2,
		skipInstrs: skip,
	}
}

// x86Slice1FuzzGenProgram emits n random slice-1/2 instructions followed
// by a HLT terminator. Branches are placed as separate generator
// decisions and back-patched after the linear layout is known. Slice 2
// optionally emits up to one CALL targeting an appended post-HLT
// function block (also random ALU/MOV ending in RET).
func x86Slice1FuzzGenProgram(rng *rand.Rand, n int) []byte {
	state := &x86FuzzState{}
	specs := make([]x86Slice1FuzzInstrSpec, 0, n)

	// Decide whether this program contains CALL+function pairs. Slice 3
	// emits 0..3 CALL sites, all targeting the same appended function.
	// Multiple CALLs exercise the 2-entry RTS cache: each CALL leaves
	// distinct return-PC slots in different main-flow blocks, so the
	// callee's RET probes the cache and chains directly back to the
	// caller's continuation rather than bouncing through Go dispatcher
	// per return.
	emitCall := rng.IntN(2) == 0 && n >= 4
	callSlots := map[int]bool{}
	if emitCall {
		nCalls := 1 + rng.IntN(3) // 1..3 CALL sites
		for tries := 0; tries < nCalls*4 && len(callSlots) < nCalls; tries++ {
			cand := 1 + rng.IntN(n-2)
			callSlots[cand] = true
		}
	}

	for i := 0; i < n; i++ {
		if callSlots[i] {
			// CALL rel32 placeholder; rel32 fixed up after layout.
			specs = append(specs, x86Slice1FuzzInstrSpec{
				bytes:      []byte{0xE8, 0, 0, 0, 0},
				branch:     true,
				rel32Off:   1,
				callTarget: true,
			})
			continue
		}
		// 15% chance of branch, but only when at least 2 more instrs remain
		// (so the branch can skip past at least one and still land before HLT).
		if rng.IntN(100) < 15 && (n-i) >= 3 {
			specs = append(specs, x86Slice1FuzzAppendBranch(rng))
			continue
		}
		// Capture the byte sequence for one non-branch instr.
		var buf []byte
		buf = x86Slice1FuzzAppendInstr(rng, buf, state)
		specs = append(specs, x86Slice1FuzzInstrSpec{bytes: buf})
	}

	// Layout: compute byte offset of each instr.
	offsets := make([]int, len(specs)+1)
	for i, s := range specs {
		offsets[i+1] = offsets[i] + len(s.bytes)
	}
	hltOffset := offsets[len(specs)]

	// Layout function block (if any) immediately after HLT.
	functionStart := hltOffset + 1
	var functionBytes []byte
	if emitCall {
		funcState := &x86FuzzState{noStackOps: true}
		funcInstrs := 1 + rng.IntN(3) // 1..3 ALU/MOV instrs (no stack ops)
		for j := 0; j < funcInstrs; j++ {
			functionBytes = x86Slice1FuzzAppendInstr(rng, functionBytes, funcState)
		}
		functionBytes = append(functionBytes, 0xC3) // RET
	}

	// Fix up rel32s.
	for i, s := range specs {
		if !s.branch {
			continue
		}
		var targetByteOff int
		if s.callTarget {
			targetByteOff = functionStart
		} else {
			targetIdx := i + 1 + s.skipInstrs
			if targetIdx >= len(specs) {
				targetByteOff = hltOffset
			} else {
				targetByteOff = offsets[targetIdx]
			}
		}
		instrEndOff := offsets[i] + len(s.bytes)
		rel := int32(targetByteOff - instrEndOff)
		s.bytes[s.rel32Off+0] = byte(rel)
		s.bytes[s.rel32Off+1] = byte(rel >> 8)
		s.bytes[s.rel32Off+2] = byte(rel >> 16)
		s.bytes[s.rel32Off+3] = byte(rel >> 24)
	}

	code := make([]byte, 0, hltOffset+1+len(functionBytes))
	for _, s := range specs {
		code = append(code, s.bytes...)
	}
	code = append(code, 0xF4) // HLT (terminates main flow)
	code = append(code, functionBytes...)
	return code
}

// x86Slice1FuzzSetupCPU builds a CPU with the given code at progPC and the
// given initial register/flag state. Memory at memPC..memPC+memSize is
// pre-populated from initMem so interp and JIT see the same initial bytes.
func x86Slice1FuzzSetupCPU(t *testing.T, code []byte, initRegs [8]uint32, initFlags uint32, initMem []byte) *CPU_X86 {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	cpu.EIP = x86Slice1FuzzProgPC
	cpu.EAX = initRegs[0]
	cpu.ECX = initRegs[1]
	cpu.EDX = initRegs[2]
	cpu.EBX = initRegs[3]
	cpu.ESP = initRegs[4]
	cpu.EBP = initRegs[5]
	cpu.ESI = initRegs[6]
	cpu.EDI = initRegs[7]
	cpu.Flags = initFlags

	for i, b := range code {
		cpu.memory[x86Slice1FuzzProgPC+uint32(i)] = b
	}
	for i, b := range initMem {
		cpu.memory[x86Slice1FuzzMemPC+uint32(i)] = b
	}
	return cpu
}

// x86Slice1FuzzRunInterp runs the CPU through the interpreter until HLT or
// timeout. Returns final state for diffing. The seed/iter context is
// included in the timeout message so failing programs are easy to
// reproduce by re-running with the same env-driven seed/iter pair.
func x86Slice1FuzzRunInterp(t *testing.T, cpu *CPU_X86, ctx string) {
	t.Helper()
	cpu.running.Store(true)
	cpu.Halted = false

	done := make(chan struct{})
	go func() {
		for cpu.Running() && !cpu.Halted {
			cpu.Step()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatalf("interpreter execution timed out (%s)", ctx)
	}
}

// x86Slice1FuzzRunJIT runs the CPU through the native JIT with the slice-1
// force-native gate engaged.
func x86Slice1FuzzRunJIT(t *testing.T, cpu *CPU_X86) {
	t.Helper()
	cpu.x86JitEnabled = true
	cpu.running.Store(true)
	cpu.Halted = false

	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("JIT execution timed out")
	}
}

// x86Slice1FuzzVisibleFlagsMask is the subset of EFLAGS slice 1/2 compares.
// AF (bit 4) is included: slice 2 lands the per-op capture split that
// preserves guest's prior AF for AND/OR/XOR/TEST/SHL/SHR/SAR/ROL/ROR
// (Intel-undefined ops; IE interpreter leaves AF untouched), and adopts
// host's well-defined AF for ADD/SUB/CMP/INC/DEC/NEG.
const x86Slice1FuzzVisibleFlagsMask uint32 = 0x0000_08D5 // OF|SF|ZF|AF|PF|CF

// x86Slice1FuzzDiff returns "" if both CPUs have equivalent visible state at
// halt, otherwise a description of the first divergence.
func x86Slice1FuzzDiff(interp, jit *CPU_X86) string {
	if interp.EIP != jit.EIP {
		return fmt.Sprintf("EIP mismatch: interp=%08X jit=%08X", interp.EIP, jit.EIP)
	}
	regNames := [...]string{"EAX", "ECX", "EDX", "EBX", "ESP", "EBP", "ESI", "EDI"}
	interpRegs := [...]uint32{interp.EAX, interp.ECX, interp.EDX, interp.EBX,
		interp.ESP, interp.EBP, interp.ESI, interp.EDI}
	jitRegs := [...]uint32{jit.EAX, jit.ECX, jit.EDX, jit.EBX,
		jit.ESP, jit.EBP, jit.ESI, jit.EDI}
	for i, name := range regNames {
		if interpRegs[i] != jitRegs[i] {
			return fmt.Sprintf("%s mismatch: interp=%08X jit=%08X", name, interpRegs[i], jitRegs[i])
		}
	}
	interpF := interp.Flags & x86Slice1FuzzVisibleFlagsMask
	jitF := jit.Flags & x86Slice1FuzzVisibleFlagsMask
	if interpF != jitF {
		return fmt.Sprintf("Flags mismatch (visible mask=%X): interp=%08X jit=%08X",
			x86Slice1FuzzVisibleFlagsMask, interpF, jitF)
	}
	interpMem := interp.memory[x86Slice1FuzzMemPC : x86Slice1FuzzMemPC+x86Slice1FuzzMemSize]
	jitMem := jit.memory[x86Slice1FuzzMemPC : x86Slice1FuzzMemPC+x86Slice1FuzzMemSize]
	if !bytes.Equal(interpMem, jitMem) {
		for i := range interpMem {
			if interpMem[i] != jitMem[i] {
				return fmt.Sprintf("memory mismatch at offset %d: interp=%02X jit=%02X",
					i, interpMem[i], jitMem[i])
			}
		}
	}
	return ""
}

// TestX86JIT_Slice1Fuzz drives the slice-1 differential fuzz harness.
// Each iteration generates a random slice-1 program, runs it through both
// the interpreter and the force-native JIT, and asserts equivalent final
// state.
func TestX86JIT_Slice1Fuzz(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available on this platform")
	}

	seed := uint64(1)
	if s := os.Getenv("X86_FUZZ_SEED"); s != "" {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			seed = v
		}
	}
	iters := 200
	if s := os.Getenv("X86_FUZZ_ITERS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			iters = v
		}
	}

	for iter := 0; iter < iters; iter++ {
		rng := rand.New(rand.NewPCG(seed, uint64(iter)))
		nInstr := 4 + rng.IntN(28)
		code := x86Slice1FuzzGenProgram(rng, nInstr)

		var initRegs [8]uint32
		for j := range initRegs {
			initRegs[j] = rng.Uint32()
		}
		// Constrain ESI/EDI to scratch range so [base+disp8] memory ops
		// land inside the diff window.
		initRegs[6] = x86Slice1FuzzMemPC
		initRegs[7] = x86Slice1FuzzMemPC
		// Slice 2: ESP starts at the top of the dedicated stack region;
		// the generator tracks depth so PUSH/POP and CALL/RET stay
		// inside [stackBottom, stackTop).
		initRegs[4] = x86Slice2FuzzStackTop
		// EBP unused by slice-1/2 forms; pin deterministic.
		initRegs[5] = 0

		// Initial flag bits constrained to the visible mask so divergence on
		// reserved/system bits doesn't poison the diff. Set to 0 in slice 1
		// to keep the diff focused on flag-output correctness, not initial
		// flag-state propagation through register-only sequences.
		initFlags := uint32(0)
		_ = rng.Uint32() // keep PRNG sequence stable

		initMem := make([]byte, x86Slice1FuzzMemSize)
		for j := range initMem {
			initMem[j] = byte(rng.IntN(256))
		}

		interpCPU := x86Slice1FuzzSetupCPU(t, code, initRegs, initFlags, initMem)
		x86Slice1FuzzRunInterp(t, interpCPU, fmt.Sprintf("seed=%d iter=%d nInstr=%d code=%X", seed, iter, nInstr, code))

		jitCPU := x86Slice1FuzzSetupCPU(t, code, initRegs, initFlags, initMem)
		x86Slice1FuzzRunJIT(t, jitCPU)

		if msg := x86Slice1FuzzDiff(interpCPU, jitCPU); msg != "" {
			t.Errorf("seed=%d iter=%d nInstr=%d: %s\n  code=%X",
				seed, iter, nInstr, msg, code)
			if testing.Short() || iter >= 5 {
				// Cap diagnostic noise — first failure dump captures it.
				t.FailNow()
			}
		}
	}
}
