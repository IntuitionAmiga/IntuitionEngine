// jit_m68k_const_addr_test.go - Milestone 7 constant-address proof slice.
// Shape tests prove guard elision happens exactly when the proof holds;
// the parity grid proves elided blocks match the interpreter bit for bit.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"testing"
	"unsafe"
)

func m68kConstAddrCompileLen(t *testing.T, words []uint16, proof *m68kConstAddrProof) int {
	t.Helper()
	mem := make([]byte, 1<<20)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	if len(instrs) == 0 {
		t.Fatal("scan produced no instructions")
	}
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	block, err := m68kCompileBlockWithMemProof(instrs, pc, em, mem, proof)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return block.execSize
}

// TestM68KJIT_ConstAddrShape proves the RAM-bounds and I/O guards are
// elided for a proven plain-RAM constant address, and retained when the
// address's page is marked I/O or the proof is absent.
func TestM68KJIT_ConstAddrShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	// MOVE.L $2000.L,D1 ; MOVE.L D1,$2004.L ; BRA.S *+4
	words := []uint16{
		0x2239, 0x0000, 0x2000,
		0x23C1, 0x0000, 0x2004,
		0x6002,
	}
	ramProof := &m68kConstAddrProof{ioPageBitmap: make([]bool, 1<<12), memSize: 1 << 20}
	ioBitmap := make([]bool, 1<<12)
	ioBitmap[0x2000>>8] = true
	ioBitmap[0x2004>>8] = true
	ioProof := &m68kConstAddrProof{ioPageBitmap: ioBitmap, memSize: 1 << 20}

	lenNoProof := m68kConstAddrCompileLen(t, words, nil)
	lenRAM := m68kConstAddrCompileLen(t, words, ramProof)
	lenIO := m68kConstAddrCompileLen(t, words, ioProof)

	if lenRAM >= lenNoProof {
		t.Fatalf("const-addr proof did not shrink the block: proof=%d noProof=%d", lenRAM, lenNoProof)
	}
	if lenIO != lenNoProof {
		t.Fatalf("I/O-page constant address changed shape: io=%d noProof=%d", lenIO, lenNoProof)
	}
}

// TestM68KJIT_ConstAddrOutOfRangeKeepsGuard: a constant address beyond
// MemSize (or wrapping) must keep the guards so the access bails to the
// interpreter for the architectural fault path.
func TestM68KJIT_ConstAddrOutOfRangeKeepsGuard(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	// MOVE.L $100000.L,D1 (== memSize, first byte out of range); BRA.S
	words := []uint16{0x2239, 0x0010, 0x0000, 0x6002}
	proof := &m68kConstAddrProof{ioPageBitmap: nil, memSize: 1 << 20}
	lenNoProof := m68kConstAddrCompileLen(t, words, nil)
	lenProof := m68kConstAddrCompileLen(t, words, proof)
	if lenProof != lenNoProof {
		t.Fatalf("out-of-range constant address was elided: proof=%d noProof=%d", lenProof, lenNoProof)
	}
	// Straddle: last byte crosses memSize.
	words = []uint16{0x2239, 0x000F, 0xFFFE, 0x6002}
	lenNoProof = m68kConstAddrCompileLen(t, words, nil)
	lenProof = m68kConstAddrCompileLen(t, words, proof)
	if lenProof != lenNoProof {
		t.Fatalf("straddling constant address was elided: proof=%d noProof=%d", lenProof, lenNoProof)
	}
}

// TestM68KJIT_ConstAddrParity runs a grid of constant-address reads and
// writes with the proof active and compares full CPU state and memory with
// the interpreter.
func TestM68KJIT_ConstAddrParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	type parityCase struct {
		name  string
		words []uint16
		watch []m68kDiffMemWatch
	}
	var cases []parityCase

	// MOVE <abs>,Dn and MOVE Dn,<abs> in all sizes, abs.W and abs.L.
	sizes := []struct {
		name string
		size int
	}{{"B", M68K_SIZE_BYTE}, {"W", M68K_SIZE_WORD}, {"L", M68K_SIZE_LONG}}
	for _, sz := range sizes {
		// abs.L source at 0x2000, abs.L dest at 0x2100
		srcL := m68kDiffMoveOpcode(sz.size, 7, 1, M68K_AM_DR, 1)
		dstL := m68kDiffMoveOpcode(sz.size, M68K_AM_DR, 1, 7, 1)
		cases = append(cases, parityCase{
			name:  "MOVE_" + sz.name + "_absL_roundtrip",
			words: []uint16{srcL, 0x0000, 0x2000, dstL, 0x0000, 0x2100, 0x6002},
			watch: []m68kDiffMemWatch{{addr: 0x2100, size: sz.size}},
		})
		// abs.W source/dest at 0x3000/0x3100
		srcW := m68kDiffMoveOpcode(sz.size, 7, 0, M68K_AM_DR, 2)
		dstW := m68kDiffMoveOpcode(sz.size, M68K_AM_DR, 2, 7, 0)
		cases = append(cases, parityCase{
			name:  "MOVE_" + sz.name + "_absW_roundtrip",
			words: []uint16{srcW, 0x3000, dstW, 0x3100, 0x6002},
			watch: []m68kDiffMemWatch{{addr: 0x3100, size: sz.size}},
		})
		// (d16,PC) source: reads code-adjacent constant pool
		srcPC := m68kDiffMoveOpcode(sz.size, 7, 2, M68K_AM_DR, 3)
		cases = append(cases, parityCase{
			name:  "MOVE_" + sz.name + "_d16PC",
			words: []uint16{srcPC, 0x0100, 0x6002},
		})
	}
	// ALU with abs.L source: ADD.L $2000.L,D1
	cases = append(cases, parityCase{
		name:  "ADD_L_absL",
		words: []uint16{0xD2B9, 0x0000, 0x2000, 0x6002},
	})

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
			m68kDiffSetupCPU(interp)
			m68kConstAddrParitySeed(interp)
			m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
			instrs := m68kScanBlock(interp.memory, m68kDiffStartPC)
			if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
				instrs = instrs[:len(instrs)-1]
			}
			steps := len(instrs)
			for i := 0; i < steps; i++ {
				if cycles := interp.StepOne(); cycles == 0 {
					t.Fatalf("interpreter stopped at instruction %d", i)
				}
			}

			rig := newM68KDiffJITTestRig(t)
			jit := rig.cpu
			jit.PC = m68kDiffStartPC
			m68kDiffSetupCPU(jit)
			m68kConstAddrParitySeed(jit)
			m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

			proof := &m68kConstAddrProof{
				ioPageBitmap: nil, // no I/O pages in the test rig's low RAM
				memSize:      uint32(len(jit.memory)),
			}
			rig.execMem.Reset()
			block, err := m68kCompileBlockWithMemProof(instrs, m68kDiffStartPC, rig.execMem, jit.memory, proof)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
			rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
			rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
			rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
			rig.ctx.RetPC = 0
			rig.ctx.NeedIOFallback = 0
			callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
			jit.PC = rig.ctx.RetPC
			if rig.ctx.NeedIOFallback != 0 {
				t.Fatal("const-addr block requested interpreter fallback")
			}
			if jit.PC != interp.PC {
				t.Fatalf("PC mismatch: got=0x%08X want=0x%08X", jit.PC, interp.PC)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			for _, watch := range tc.watch {
				got := m68kDiffReadMem(jit, watch)
				want := m68kDiffReadMem(interp, watch)
				if got != want {
					t.Fatalf("memory[0x%08X] mismatch: got=0x%X want=0x%X", watch.addr, got, want)
				}
			}
		})
	}
}

func m68kConstAddrParitySeed(cpu *M68KCPU) {
	for _, seed := range []struct {
		addr uint32
		val  uint32
	}{
		{0x2000, 0xA1B2C3D4},
		{0x3000, 0x55AA1234},
		{m68kDiffStartPC + 2 + 0x100, 0xDEADBEEF},
	} {
		cpu.Write32(seed.addr, seed.val)
	}
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x11111111 * uint32(i)
	}
}

var _ = fmt.Sprintf // keep fmt for future diagnostics

// BenchmarkM68KJIT_ConstAddr measures the guard-elision win on a
// constant-address-heavy loop: repeated absolute-address load/store pairs
// executed natively, proof off versus proof on.
func benchmarkM68KConstAddr(b *testing.B, proofOn bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	// 16 MOVE.L $2000.L,D1 / MOVE.L D1,$2100.L pairs then BRA.S exit.
	var words []uint16
	for i := 0; i < 16; i++ {
		words = append(words, 0x2239, 0x0000, 0x2000, 0x23C1, 0x0000, 0x2100)
	}
	words = append(words, 0x6002)

	mem := make([]byte, 1<<20)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		b.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	var proof *m68kConstAddrProof
	if proofOn {
		proof = &m68kConstAddrProof{memSize: uint32(len(mem))}
	}
	block, err := m68kCompileBlockWithMemProof(instrs, pc, em, mem, proof)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	copy(cpu.memory[pc:], mem[pc:pc+uint32(len(words)*2)])
	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)
	ctx.DataRegsPtr = uintptr(unsafe.Pointer(&cpu.DataRegs[0]))
	ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&cpu.AddrRegs[0]))
	ctx.MemPtr = uintptr(unsafe.Pointer(&cpu.memory[0]))
	ctx.SRPtr = uintptr(unsafe.Pointer(&cpu.SR))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.RetPC = 0
		ctx.NeedIOFallback = 0
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	}
}

func BenchmarkM68KJIT_ConstAddrOff(b *testing.B) { benchmarkM68KConstAddr(b, false) }
func BenchmarkM68KJIT_ConstAddrOn(b *testing.B)  { benchmarkM68KConstAddr(b, true) }
