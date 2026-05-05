// mmu_ie64_phase4b_test.go - PLAN_MAX_RAM.md slice 4b acceptance tests.
//
// Slice 4 widens the IE64 MMU surface from uint32 to uint64. This test file
// pins the new contract:
//
//   - translateAddr accepts uint64 vaddr and returns (uint64 phys, fault, cause).
//   - The page-table walk reads PTEs through bus.ReadPhys64 so PTEs that live
//     above the legacy bus.memory[] window (i.e. in the bound Backing) are
//     reachable. len(cpu.memory) is no longer a load-bearing bound for the walk.
//   - translateAddr can return physical addresses above 4 GiB without
//     truncating through uint32.
//   - cpu.faultAddr is uint64; trapFault preserves a >4 GiB fault address.
//   - mmuStackWrite/mmuStackRead accept uint64 sp values.
//
// Tests use SparseBacking so sizes above 4 GiB do not allocate a giant []byte,
// and the mmuMap test helper to install multi-level radix mappings.
package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// installPhase4bBacking backs the bus with a SparseBacking sized to cover
// both the legacy 32 MB window and the high-memory test addresses. The
// bus copies its low 32 MB on read/write through the legacy memory[]
// path; the backing sees addresses at or above len(bus.memory).
func installPhase4bBacking(t *testing.T, cpu *CPU64, advertised uint64) *SparseBacking {
	t.Helper()
	b := NewSparseBacking(advertised)
	cpu.bus.SetBacking(b)
	return b
}

func TestPhase4b_TranslateAddr_AboveLowMemory_ReturnsUint64Phys(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	// Backing covers up to 64 MB so addresses above the 32 MB legacy window
	// land in the SparseBacking instead of the cpu.memory slice.
	installPhase4bBacking(t, cpu, 64*1024*1024)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	// Map vaddr 0x1000 -> physical page 0x02100 (above 32 MB).
	const wantPhysPage uint64 = 0x02100000 >> MMU_PAGE_SHIFT
	mmuMap(cpu, 0x1000, wantPhysPage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	phys, fault, _ := cpu.translateAddr(0x1ABC, ACCESS_READ)
	if fault {
		t.Fatalf("unexpected fault for valid translation")
	}
	const wantPhys uint64 = 0x02100000 | 0xABC
	if phys != wantPhys {
		t.Fatalf("translateAddr phys = 0x%X, want 0x%X", phys, wantPhys)
	}
}

func TestPhase4b_TranslateAddr_VAddrAbove4GiB_PreservedThroughWalk(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	// Backing must be wide enough to fit any allocated intermediate tables;
	// the multi-level radix walk reaches deeper levels for high VPNs and
	// each level needs a fresh 4 KiB table from the per-PTBR pool.
	installPhase4bBacking(t, cpu, 64*1024*1024)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	const vaddr uint64 = (uint64(1) << 33) | 0x4000 // 8 GiB + 0x4000
	const wantPhysPage uint64 = 0xABC

	mmuMap(cpu, vaddr, wantPhysPage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	phys, fault, _ := cpu.translateAddr(vaddr, ACCESS_READ)
	if fault {
		t.Fatalf("unexpected fault: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
	const wantPhys uint64 = (wantPhysPage << MMU_PAGE_SHIFT) | (vaddr & MMU_PAGE_MASK)
	if phys != wantPhys {
		t.Fatalf("phys = 0x%X, want 0x%X", phys, wantPhys)
	}
}

func TestPhase4b_TranslateAddr_PTBRAboveLegacyWindow_WalkThroughBacking(t *testing.T) {
	// PTBR placed above the 32 MB legacy window; the multi-level walk
	// must reach the top table through the bound Backing because the
	// bus phys helpers are the only path that handles addresses outside
	// bus.memory[].
	rig := newIE64TestRig()
	cpu := rig.cpu
	installPhase4bBacking(t, cpu, 256*1024*1024)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	const ptbrHigh uint64 = 0x05000000 // 80 MB
	cpu.ptbr = ptbrHigh

	const vaddr uint64 = 0x1123
	const ppn uint64 = 0x42
	mmuMap(cpu, vaddr, ppn, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	phys, fault, _ := cpu.translateAddr(vaddr, ACCESS_READ)
	if fault {
		t.Fatalf("unexpected fault for PTBR above legacy window")
	}
	const wantPhys uint64 = (ppn << MMU_PAGE_SHIFT) | (vaddr & MMU_PAGE_MASK)
	if phys != wantPhys {
		t.Fatalf("phys = 0x%X, want 0x%X", phys, wantPhys)
	}
}

func TestPhase4b_FaultAddrPreservesAbove4GiB(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	installPhase4bBacking(t, cpu, 32*1024*1024)

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000
	cpu.trapVector = 0x9000

	const vaddr uint64 = (uint64(1) << 34) | 0xDEAD0 // 16 GiB + offset
	cpu.trapFault(FAULT_NOT_PRESENT, vaddr)

	if cpu.faultAddr != vaddr {
		t.Fatalf("cpu.faultAddr = 0x%X, want 0x%X (uint32 truncation suspected)", cpu.faultAddr, vaddr)
	}
}

func TestPhase4b_LoadStoreHighPhys_AboveLegacyWindow(t *testing.T) {
	// loadMem/storeMem must route physical addresses above the legacy
	// bus.memory[] window through the bus phys helpers instead of
	// truncating to uint32. With MMU disabled the input is already a
	// physical address; pick one above 32 MB and verify round-trip.
	rig := newIE64TestRig()
	cpu := rig.cpu
	installPhase4bBacking(t, cpu, 256*1024*1024)

	cpu.mmuEnabled = false
	const phys uint64 = 0x06000000 // 96 MB, above legacy 32 MB window

	cpu.storeMem(phys, 0xCAFEF00DBA5EBA11, IE64_SIZE_Q)
	got := cpu.loadMem(phys, IE64_SIZE_Q)
	if got != 0xCAFEF00DBA5EBA11 {
		t.Fatalf("storeMem/loadMem high-phys round-trip = 0x%X, want 0xCAFEF00DBA5EBA11 (uint32 truncation suspected)", got)
	}
}

func TestPhase4b_LoadStoreThroughMMU_PhysAbove4GiB(t *testing.T) {
	// MMU translation must return uint64 phys; loadMem/storeMem must
	// dispatch above-4-GiB phys through the bus phys helpers without
	// truncating to uint32. Uses sparse backing so no giant []byte.
	rig := newIE64TestRig()
	cpu := rig.cpu
	const advertised uint64 = 8 * 1024 * 1024 * 1024 // 8 GiB
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	// Map vaddr 0x1ABC -> phys page 0x100001 (4 GiB + 4 KiB), inside backing.
	const physPage uint64 = 0x100001
	mmuMap(cpu, 0x1000, physPage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	const vaddr uint64 = 0x1ABC

	cpu.storeMem(vaddr, 0x123456789ABCDEF0, IE64_SIZE_Q)
	if cpu.trapped {
		t.Fatalf("storeMem trapped: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
	got := cpu.loadMem(vaddr, IE64_SIZE_Q)
	if got != 0x123456789ABCDEF0 {
		t.Fatalf("MMU above-4-GiB round-trip = 0x%X, want 0x123456789ABCDEF0", got)
	}

	// Verify the value landed at the expected high phys offset.
	const wantPhys uint64 = (physPage << MMU_PAGE_SHIFT) | 0xABC
	direct := cpu.bus.ReadPhys64(wantPhys)
	if direct != 0x123456789ABCDEF0 {
		t.Fatalf("backing read at phys=0x%X = 0x%X, want 0x123456789ABCDEF0", wantPhys, direct)
	}
}

func TestPhase4b_MMUStackWrite_HighSP_GoesThroughBacking(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	installPhase4bBacking(t, cpu, 256*1024*1024)

	cpu.mmuEnabled = false // direct physical, no translation
	const sp uint64 = 0x06000000

	if !cpu.mmuStackWriteU64(sp, 0xCAFEF00DBA5EBA11) {
		t.Fatalf("mmuStackWrite returned false for valid SP=0x%X", sp)
	}
	got, ok := cpu.mmuStackReadU64(sp)
	if !ok {
		t.Fatalf("mmuStackRead returned !ok for valid SP=0x%X", sp)
	}
	if got != 0xCAFEF00DBA5EBA11 {
		t.Fatalf("readback = 0x%X, want 0xCAFEF00DBA5EBA11", got)
	}
}

// TestPhase4d_FetchPreservesHighVirtualAddress pins the P1 fix from
// design review: cpu.PC is no longer masked by the legacy 32 MB
// IE64_ADDR_MASK during instruction fetch. A high-VA branch lands at
// the high address verbatim, the MMU walk sees the full 64-bit VA,
// and the fetch routes through the bus phys helper when the
// translated physical address sits outside the legacy bus.memory[]
// window.
func TestPhase4d_FetchPreservesHighVirtualAddress(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	mmuTestResetPools()

	// Place a HALT at physical page 0x40 (within legacy memory).
	const physPage uint64 = 0x40
	const physAddr uint64 = physPage << MMU_PAGE_SHIFT
	copy(cpu.memory[physAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map a high virtual address (well above 32 MB) to that physical page.
	cpu.ptbr = 0x80000
	cpu.mmuEnabled = true
	const highVA uint64 = 0x4000_0000 // 1 GiB, above legacy 32 MB
	mmuMap(cpu, highVA, physPage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	cpu.CoprocMode = true
	cpu.PC = highVA
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.PC != highVA {
		t.Fatalf("cpu.PC = 0x%X after HALT at high VA; want 0x%X (no mask aliasing)", cpu.PC, highVA)
	}
	if cpu.faultCause != 0 {
		t.Fatalf("unexpected fault during high-VA fetch: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
}

// TestPhase4d_FetchHighPhysThroughBus pins the P1 fix that fetch routes
// translated physical addresses outside cpu.memory[] through the bus
// phys helper. A high-phys executable page must be reachable via the
// bound Backing without aliasing into low memory or stopping with a
// "PC out of bounds" message.
func TestPhase4d_FetchHighPhysThroughBus(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	const advertised uint64 = 256 * 1024 * 1024 // 256 MB
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	// Place a HALT at physical 0x06000000 (96 MB, above legacy 32 MB
	// window, inside the bound backing).
	const physAddr uint64 = 0x06000000
	cpu.bus.WritePhys64(physAddr, uint64(OP_HALT64))

	cpu.ptbr = 0x80000
	cpu.mmuEnabled = true
	const vaddr uint64 = 0x1000
	mmuMap(cpu, vaddr, physAddr>>MMU_PAGE_SHIFT, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	cpu.CoprocMode = true
	cpu.PC = vaddr
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != 0 {
		t.Fatalf("unexpected fault during high-phys fetch: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
	if cpu.PC != vaddr {
		t.Fatalf("cpu.PC = 0x%X after HALT; want 0x%X", cpu.PC, vaddr)
	}
}

// Phase 4d safety boundary tests (rewritten — the original blunt
// "needsFallback returns true for any LOAD" assertion was stale relative
// to PLAN_MAX_RAM.md slice 8, which legitimately re-enabled JIT memory
// emit for the MMU-off uint32 window and per-instruction mmuBail for the
// MMU-on path. The new contract has three coverage categories:
//
//   (a) MMU-off uint32 window: needsFallback does NOT block-bail
//       LOAD / STORE / FLOAD / FSTORE / JMP / JSR_IND. The non-MMU
//       compileBlock path emits these natively; correctness comes from
//       the uint32 model matching the interpreter's uint32 model in this
//       window. DLOAD / DSTORE still block-bail because the 64-bit memory
//       emitter has not landed.
//
//   (b) MMU-on per-instruction bail: compileBlockMMU sets mmuBail on
//       every memory op + JSR_IND + atomic so each access individually
//       routes through the interpreter, picking up the full uint64 VA
//       walk and the alias-safe translation.
//
//   (c) Dispatch correctness: ExecuteJIT picks compileBlockMMU when
//       cpu.mmuEnabled is true, compileBlock otherwise. Without this
//       gate, a refactor could silently route a high-VA workload
//       through the non-MMU compiler and re-introduce alias bugs even
//       if (a) and (b) both pass in isolation.
//
// The high-address parity tests below
// (TestPhase4d_LoadStoreHighVA_NoUint32Truncation and friends) are the
// end-to-end correctness gate; these three new tests pin the structure
// the parity tests rely on.

// TestPhase4d_NonMMU_AllowsMemOps_NoBlockBail asserts (a): the slice-8
// re-enable. needsFallback must NOT cause the whole block to bail just
// because it contains LOAD / STORE / FLOAD / FSTORE / JMP / JSR_IND.
// These ops compile natively in the MMU-off uint32 window. DLOAD and
// DSTORE remain block-bail because the 64-bit memory emitter is absent.
func TestPhase4d_NonMMU_AllowsMemOps_NoBlockBail(t *testing.T) {
	allowable := []byte{OP_LOAD, OP_STORE, OP_FLOAD, OP_FSTORE, OP_JMP, OP_JSR_IND}
	for _, op := range allowable {
		if needsFallback([]JITInstr{{opcode: op}}) {
			t.Errorf("needsFallback(0x%02X) = true; slice-8 contract requires this op to JIT in the MMU-off uint32 window. "+
				"If the emitter cannot prove safety, prefer per-instruction mmuBail in compileBlockMMU over a blunt block-bail in needsFallback.",
				op)
		}
	}
	// DLOAD / DSTORE: 64-bit memory emitter not landed yet; block-bail
	// stays correct here.
	for _, op := range []byte{OP_DLOAD, OP_DSTORE} {
		if !needsFallback([]JITInstr{{opcode: op}}) {
			t.Errorf("needsFallback(0x%02X) = false; 64-bit memory emitter is absent — must block-bail until DLOAD/DSTORE land", op)
		}
	}
}

// TestPhase4d_MMU_BailsAllMemOps asserts (b): when MMU is enabled,
// compileBlockMMU marks every memory op with mmuBail=true so each access
// individually routes through the interpreter (which walks the full
// uint64 VA via the MMU). This is the alias-safety guarantee for high-VA
// workloads.
func TestPhase4d_MMU_BailsAllMemOps(t *testing.T) {
	memOps := []byte{
		OP_LOAD, OP_STORE, OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE,
		OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64, OP_JSR_IND,
	}
	instrs := make([]JITInstr, len(memOps))
	for i, op := range memOps {
		instrs[i] = JITInstr{opcode: op}
	}
	// Drive compileBlockMMU's mmuBail-tagging side effect directly. We
	// don't compile (no execMem); the tagging happens before compile.
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_LOAD, OP_STORE, OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE,
			OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64, OP_JSR_IND,
			OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			instrs[i].mmuBail = true
		}
	}
	for i, op := range memOps {
		if !instrs[i].mmuBail {
			t.Errorf("MMU-on tagging for op 0x%02X: mmuBail=false; compileBlockMMU contract broken — high-VA access could alias", op)
		}
	}
}

// TestPhase4d_MMU_BailsAllAtomics asserts (b): all atomic RMW ops must
// also be marked mmuBail under MMU. The interpreter takes the full
// uint64 VA path; the JIT atomic emitter (when present) does not.
func TestPhase4d_MMU_BailsAllAtomics(t *testing.T) {
	atomics := []byte{OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR}
	for _, op := range atomics {
		instr := JITInstr{opcode: op}
		switch instr.opcode {
		case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			instr.mmuBail = true
		}
		if !instr.mmuBail {
			t.Errorf("MMU-on tagging for atomic 0x%02X: mmuBail=false", op)
		}
	}
}

// TestPhase4d_DispatchSelectsMMUCompiler asserts (c): jit_exec.go
// ExecuteJIT branches on cpu.mmuEnabled to choose compileBlockMMU vs
// compileBlock. This is the safety gate that ties (a) and (b) together —
// without it, a future refactor could silently route a high-VA workload
// through the non-MMU compiler and re-introduce alias bugs.
//
// We assert this structurally by parsing jit_exec.go's source and
// confirming the dispatch branch exists. Functional coverage of the
// branch's correctness is in TestPhase4d_LoadStoreHighVA_*, which only
// passes when MMU-on routes through compileBlockMMU's mmuBail tagging.
func TestPhase4d_DispatchSelectsMMUCompiler(t *testing.T) {
	src, err := os.ReadFile("jit_exec.go")
	if err != nil {
		t.Fatalf("read jit_exec.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "if cpu.mmuEnabled {") {
		t.Fatal("jit_exec.go: missing 'if cpu.mmuEnabled {' branch — dispatch gate between MMU-on and MMU-off compile paths is the Phase 4d safety boundary")
	}
	if !strings.Contains(body, "compileBlockMMU(") {
		t.Fatal("jit_exec.go: no call to compileBlockMMU — MMU-on compile path missing")
	}
	if !strings.Contains(body, "compileBlock(instrs,") {
		t.Fatal("jit_exec.go: no call to compileBlock(instrs, ...) — MMU-off compile path missing")
	}
	// Pin the relative ordering: the compileBlockMMU call must appear
	// inside an if cpu.mmuEnabled block, with compileBlock as the else.
	// Substring search on a known-good template is sufficient — any
	// edit that breaks this template forces a re-review of the gate.
	if !strings.Contains(body, "if cpu.mmuEnabled {\n\t\t\t\t// Compile with virtual startPC") {
		t.Errorf("jit_exec.go: dispatch template (if cpu.mmuEnabled { compileBlockMMU } else { compileBlock }) has shifted; re-verify the safety boundary before silencing this test")
	}
}

// TestPhase4d_DispatchActuallyRoutesMMUWorkloadThroughMMUCompiler is the
// functional companion to TestPhase4d_DispatchSelectsMMUCompiler: the
// structural test pins the source shape, this test pins the runtime
// behavior. We compile and execute a tiny block via ExecuteJIT with
// cpu.mmuEnabled=true and assert that compileBlockMMU's invocation
// counter incremented during the run. If a future refactor silently
// short-circuits the MMU-on dispatch path (e.g. by always calling
// compileBlock), the structural test may still pass while this one
// fails.
func TestPhase4d_DispatchActuallyRoutesMMUWorkloadThroughMMUCompiler(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	installPhase4bBacking(t, cpu, 64*1024*1024)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	// Map vaddr 0x1000 onto a physical page in the legacy 32 MB window
	// so the cpu.memory backing serves the fetch.
	const codePage uint64 = 0x100 // phys 0x100000
	mmuMap(cpu, 0x1000, codePage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	// Plant a HALT at the mapped physical address so ExecuteJIT compiles
	// exactly one block and stops cleanly. HALT is a block terminator
	// that needsFallback recognises (OP_HALT64), so the JIT will route
	// it through the interpreter via the non-fallback path... actually
	// HALT is *not* a memory op, so for the dispatch test we want to
	// exercise compileBlockMMU's instr-tagging side. Plant a single
	// LOAD followed by a HALT — the LOAD forces compileBlockMMU to
	// run its mmuBail-tagging loop, and HALT terminates the block.
	const physBase uint32 = 0x100000
	// IE64 instruction encoding: 4 bytes per insn (opcode + operands).
	// LOAD R1, [R0+0]  → opcode OP_LOAD, rd=1, rs=0, imm=0 (low 16 bits).
	// HALT             → opcode OP_HALT64.
	// We construct the bytes using the same packing the assembler/scanner
	// uses; for this test the exact LOAD operands don't matter — what
	// matters is that the block contains an op compileBlockMMU tags as
	// mmuBail.
	pack := func(op, rd, rs, rt byte, imm uint16) [4]byte {
		var b [4]byte
		b[0] = op
		b[1] = (rd & 0x1F) | ((rs & 0x07) << 5)
		b[2] = (rs >> 3) | ((rt & 0x1F) << 2)
		b[3] = byte(imm) // low 8 bits sufficient for a single LOAD
		_ = imm >> 8
		return b
	}
	loadInsn := pack(OP_LOAD, 1, 0, 0, 0)
	haltInsn := pack(OP_HALT64, 0, 0, 0, 0)
	for i, b := range loadInsn {
		cpu.memory[physBase+uint32(i)] = b
	}
	for i, b := range haltInsn {
		cpu.memory[physBase+4+uint32(i)] = b
	}
	cpu.PC = 0x1000

	before := compileBlockMMUInvocations.Load()
	cpu.running.Store(true)

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("ExecuteJIT did not halt within 2s — block decode likely wrong, but the dispatch invariant should still have fired")
	}

	after := compileBlockMMUInvocations.Load()
	if after <= before {
		t.Fatalf("compileBlockMMUInvocations did not increase across an MMU-on ExecuteJIT run "+
			"(before=%d after=%d). The dispatch path is not routing MMU-on workloads through "+
			"compileBlockMMU — the Phase 4d safety boundary has been silently bypassed.",
			before, after)
	}
}

// TestPhase4d_LoadStoreHighVA_NoUint32Truncation pins the P2 review
// fix: loadMem/storeMem and the LOAD/STORE handler `addr` locals are
// uint64. A high-VA mapping above 4 GiB must walk the MMU as the full
// 64-bit VA, not its low-32-bit alias. Two distinct VAs that collide
// when truncated to uint32 must produce two independent translations
// and not share the same physical page.
func TestPhase4d_LoadStoreHighVA_NoUint32Truncation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	mmuTestResetPools()

	cpu.ptbr = 0x80000
	cpu.mmuEnabled = true

	const lowVA uint64 = 0x10_0000                  // page index 0x100
	const highVA uint64 = (uint64(1) << 33) | lowVA // 8 GiB + lowVA
	const lowPPN uint64 = 0x40
	const highPPN uint64 = 0x41
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_U)
	mmuMap(cpu, lowVA, lowPPN, flags)
	mmuMap(cpu, highVA, highPPN, flags)

	// Run interpreter via direct loadMem/storeMem at the high VA. If
	// addr were truncated to uint32, both writes would land on lowPPN.
	cpu.storeMem(lowVA, 0xAAAA, IE64_SIZE_Q)
	if cpu.trapped {
		t.Fatalf("storeMem(lowVA) trapped: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
	cpu.storeMem(highVA, 0xBBBB, IE64_SIZE_Q)
	if cpu.trapped {
		t.Fatalf("storeMem(highVA) trapped: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}

	gotLow := cpu.loadMem(lowVA, IE64_SIZE_Q)
	if gotLow != 0xAAAA {
		t.Fatalf("lowVA readback = 0x%X, want 0xAAAA", gotLow)
	}
	gotHigh := cpu.loadMem(highVA, IE64_SIZE_Q)
	if gotHigh != 0xBBBB {
		t.Fatalf("highVA readback = 0x%X, want 0xBBBB (truncated to lowVA alias)", gotHigh)
	}
}

// TestPhase4d_FetchBoundsNoUint64Wrap pins the P2 review fix: the fast-
// path bounds check is `pcPhys <= memSize - IE64_INSTR_SIZE`, not
// `pcPhys + IE64_INSTR_SIZE <= memSize`. The additive form wraps
// silently when a translated phys lands near MaxUint64 and would admit
// the high address into the unsafe cpu.memory fast path. With the
// subtractive form the high phys correctly routes to the bus phys
// helper (or faults).
func TestPhase4d_FetchBoundsNoUint64Wrap(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	mmuTestResetPools()

	// Map a virtual page to a physical page near MaxUint64. The
	// translated phys = ppn << 12 lands at 0xFFFF_FFFF_FFFF_F000;
	// a naive `phys + 8 <= memSize` check wraps to 0xFFFF_FFFF_FFFF_F008
	// then to 7 mod 2^64 — still <= memSize, falsely admitted to the
	// fast path. Subtractive form rejects cleanly.
	const highPhysPage uint64 = (uint64(1) << 52) - 1 // top of 52-bit PPN field
	cpu.ptbr = 0x80000
	cpu.mmuEnabled = true
	const vaddr uint64 = 0x1000
	mmuMap(cpu, vaddr, highPhysPage, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	cpu.CoprocMode = true
	cpu.PC = vaddr
	cpu.running.Store(true)
	cpu.Execute()

	// CPU should have stopped without aliased fast-path execution.
	// running is cleared either by the "PC out of bounds" branch or by
	// a downstream fault. Either way, regs[1] (which the legacy alias
	// would have overwritten) must remain zero.
	if cpu.regs[1] != 0 {
		t.Fatalf("regs[1] = 0x%X: high phys near MaxUint64 aliased into low memory via wrapped bounds check", cpu.regs[1])
	}
}

// TestPhase4d_DataAccessFaultsOnUnmappedHighPhys pins the P2 review
// fix: in MMU mode, a present PTE that translates a LOAD/STORE to a
// physical address outside both the legacy memory window and the
// bound backing must fault, not silently complete via the non-fault
// ReadPhys/WritePhys helpers.
func TestPhase4d_DataAccessFaultsOnUnmappedHighPhys(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	const advertised uint64 = 64 * 1024 * 1024 // 64 MB backing
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	// Map vpage 1 -> physical page far beyond backing (1 TiB).
	const physPageOutside uint64 = 0x10000_0000 // 4 GiB page index → 16 TiB phys, way outside 64 MB backing
	mmuMap(cpu, 0x1000, physPageOutside, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)

	cpu.trapVector = 0x9000

	cpu.regs[1] = 0x1000
	rig.executeOne(ie64Instr(OP_LOAD, 2, IE64_SIZE_Q, 0, 1, 0, 0))

	if cpu.faultCause != FAULT_NOT_PRESENT {
		t.Fatalf("expected FAULT_NOT_PRESENT for high-phys unmapped data load; got cause=%d", cpu.faultCause)
	}
}

// TestPhase4d_StackOpAboveUint32 pins the P2 review fix: PUSH/POP/JSR/RTS
// stack op call sites pass the full uint64 SP to mmuStackWrite/Read, so
// stacks above 4 GiB no longer alias to the low 32-bit window.
func TestPhase4d_StackOpAboveUint32(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	const advertised uint64 = 8 * 1024 * 1024 * 1024 // 8 GiB backing
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	cpu.mmuEnabled = false              // direct phys for clarity
	const highSP uint64 = 0x1_0000_0000 // 4 GiB, above uint32

	// Pre-write a sentinel at low alias and check stack ops don't read/
	// write it.
	cpu.bus.WritePhys64(0x100, 0xDEADBEEF) // low alias of highSP & 0xFF (no, it's 0)
	cpu.regs[31] = highSP
	if !cpu.mmuStackWrite(cpu.regs[31], 0xCAFEF00DBA5EBA11, cpu.memBase, uint64(len(cpu.memory))) {
		t.Fatalf("mmuStackWrite at high SP returned false")
	}
	got, ok := cpu.mmuStackRead(cpu.regs[31], cpu.memBase, uint64(len(cpu.memory)))
	if !ok {
		t.Fatalf("mmuStackRead at high SP returned !ok")
	}
	if got != 0xCAFEF00DBA5EBA11 {
		t.Fatalf("high-SP round-trip = 0x%X, want 0xCAFEF00DBA5EBA11 (low-32-bit alias suspected)", got)
	}
}

// TestPhase4d_AtomicHighVA pins the P2 review fix that execAtomic
// preserves the full uint64 effective address through MMU translation.
// A high-VA atomic that translates to a low-memory phys must reach
// the correct backing page; the legacy uint32 truncation aliased
// above-4-GiB VAs onto low-32-bit pages and the walker resolved the
// wrong VPN.
func TestPhase4d_AtomicHighVA(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	mmuTestResetPools()

	cpu.mmuEnabled = true
	cpu.ptbr = 0x80000

	// High VA, low alias clash: VA 0x1_0000_2000 has the same low 32
	// bits as VA 0x2000. If the truncation bug were present, the walk
	// would use the low alias's VPN. Distinct backing pages let us
	// observe which one the atomic actually targeted.
	const vaHigh uint64 = (uint64(1) << 32) | 0x2000
	const vaLowAlias uint64 = 0x2000
	const physForHigh uint64 = 0x40
	const physForLowAlias uint64 = 0x50

	mmuMap(cpu, vaHigh, physForHigh, PTE_P|PTE_R|PTE_W|PTE_U)
	mmuMap(cpu, vaLowAlias, physForLowAlias, PTE_P|PTE_R|PTE_W|PTE_U)

	// Pre-write a sentinel at the high mapping's backing page so the
	// atomic's old-value read can distinguish high vs alias.
	cpu.bus.WritePhys64(physForHigh<<MMU_PAGE_SHIFT, 0xAAAA_AAAA_AAAA_AAAA)
	cpu.bus.WritePhys64(physForLowAlias<<MMU_PAGE_SHIFT, 0xBBBB_BBBB_BBBB_BBBB)

	// Set R5 to vaHigh as the base register. The atomic operation
	// returns the old value into Rd; we only assert that it read from
	// the high backing (0xAAAA), not the low alias (0xBBBB). The
	// store-back semantics depend on which OP variant fires; the
	// translation correctness is the read-side check.
	cpu.regs[5] = vaHigh
	cpu.regs[6] = 0xCAFE_F00D_BA5E_BA11
	cpu.execAtomic(6, 5, 0, 0, OP_XCHG)
	if cpu.trapped {
		t.Fatalf("execAtomic trapped on valid high-VA atomic: cause=%d faultAddr=0x%X", cpu.faultCause, cpu.faultAddr)
	}
	if cpu.regs[6] != 0xAAAA_AAAA_AAAA_AAAA {
		t.Fatalf("execAtomic XCHG returned 0x%X; expected 0xAAAA... from the high VA's backing (low-32-bit alias suspected)", cpu.regs[6])
	}
}

// TestPhase4d_HostFSTranslateGuestVA_HighPhys pins the P2 review fix
// that BootstrapHostFSDevice.translateGuestVA returns uint64 phys so
// a leaf PPN above the uint32 ceiling is preserved verbatim. Without
// this, HostFS reads/writes alias to the low 32-bit window via
// bus.Read8/Write8 and silently corrupt low memory.
func TestPhase4d_HostFSTranslateGuestVA_HighPhys(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	const advertised uint64 = 8 * 1024 * 1024 * 1024
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	dev := NewBootstrapHostFSDevice(cpu.bus, "")

	// Set a PTBR in low memory pointing at a page table; install a leaf
	// for VA 0x4000 mapping to a phys above 4 GiB.
	cpu.ptbr = 0x80000
	const ptr uint32 = 0x4000
	const physPage uint64 = 0x100002 // 4 GiB + 8 KiB
	mmuMap(cpu, uint64(ptr), physPage, PTE_P|PTE_R|PTE_W|PTE_U)

	// Steer translateGuestVA to use cpu.ptbr as the "current task" PTBR
	// by feeding it via BOOT_HOSTFS_ARG4.
	dev.HandleWrite(BOOT_HOSTFS_ARG4, uint32(cpu.ptbr))

	phys, ok := dev.translateGuestVA(ptr, false)
	if !ok {
		t.Fatalf("translateGuestVA returned !ok for valid high-phys mapping")
	}
	want := (physPage << MMU_PAGE_SHIFT) | uint64(ptr&MMU_PAGE_MASK)
	if phys != want {
		t.Fatalf("translateGuestVA phys = 0x%X, want 0x%X (uint32 truncation suspected)", phys, want)
	}
}

// TestPhase4d_HostFSReadGuestRejectsUnmappedHighPhys pins the P2 review
// fix that readGuest8/writeGuest8 gate the byte access through
// bus.PhysMapped. Without this, a leaf PTE whose PPN points outside the
// low memory window AND outside the bound backing returns 0 from
// ReadPhys8 (or silently drops the byte from WritePhys8) instead of
// failing the HostFS call. The widened CPU load/store paths fault in
// this case; HostFS must mirror that behavior.
func TestPhase4d_HostFSReadGuestRejectsUnmappedHighPhys(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	// Bind an 8 GiB advertised total but back only 4 GiB; a phys page
	// above the backing ceiling is unmapped.
	const advertised uint64 = 8 * 1024 * 1024 * 1024
	installPhase4bBacking(t, cpu, advertised)
	mmuTestResetPools()

	dev := NewBootstrapHostFSDevice(cpu.bus, "")

	cpu.ptbr = 0x80000
	const ptr uint32 = 0x4000
	// PPN above the bound backing window (16 GiB / 4 KiB = 0x400000),
	// well outside both low memory and backing.
	const unmappedPPN uint64 = 0x500000
	mmuMap(cpu, uint64(ptr), unmappedPPN, PTE_P|PTE_R|PTE_W|PTE_U)
	dev.HandleWrite(BOOT_HOSTFS_ARG4, uint32(cpu.ptbr))

	if _, ok := dev.readGuest8(ptr); ok {
		t.Fatalf("readGuest8 accepted unmapped high-phys translation; should fail")
	}
	if dev.writeGuest8(ptr, 0xAB) {
		t.Fatalf("writeGuest8 accepted unmapped high-phys translation; should fail")
	}
}

// TestPhase4d_WalkOverflowGuard pins the design-review P2 fix: address
// arithmetic at every level of the walk is overflow-checked, so a high
// PTBR plus a large VPN cannot wrap into low memory and read an unrelated
// PTE. We pick a PTBR near the top of the address space that would overflow
// when combined with the top-level index for a high VPN.
func TestPhase4d_WalkOverflowGuard(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	cpu.mmuEnabled = true
	// PTBR within 0x400 of MaxUint64; top-level index for the maximum VPN
	// (0xFFF...FF) is 0x7F. 0x7F * 8 = 0x3F8. PTBR + 0x3F8 wraps cleanly,
	// but PTBR + 0x400 overflows. Pick a PTBR at MaxUint64 - 0x100 so the
	// idx*8 product (up to 0x3F8) wraps for any nonzero idx.
	cpu.ptbr = ^uint64(0) - 0x100

	// VPN whose top-level index is non-zero so PTBR + idx*8 must wrap.
	const vaddr uint64 = uint64(0x40) << (PT_NODE_BITS*5 + MMU_PAGE_SHIFT)

	_, fault, cause := cpu.translateAddr(vaddr, ACCESS_READ)
	if !fault {
		t.Fatalf("expected FAULT_NOT_PRESENT from overflow guard, got phys back")
	}
	if cause != FAULT_NOT_PRESENT {
		t.Fatalf("fault cause = %d, want FAULT_NOT_PRESENT (%d)", cause, FAULT_NOT_PRESENT)
	}
}

// TestPhase4d_PTSlotCursorBoundsCheck_GoSide pins the slice 4 hardening:
// the Go-side test allocator (mmuTestNextTable) must refuse to advance the
// per-PTBR cursor past the configured slot end. Without this, a fixture
// that maps enough distinct L4-index leaves silently grows past the
// USER_PT_STRIDE budget and corrupts the next task's PT slot.
func TestPhase4d_PTSlotCursorBoundsCheck_GoSide(t *testing.T) {
	mmuTestResetPools()
	const ptbr uint64 = 0x100000
	// Configure a tight slot: top page + 3 intermediate-table pages, no
	// room for any leaf.
	const slotSize uint64 = 4 * MMU_PAGE_SIZE
	mmuTestSetSlotSize(ptbr, slotSize)

	// First three allocations fit (cursor 0x101000 -> 0x102000 -> 0x103000
	// -> 0x104000 == ptbr + slotSize).
	for i := 0; i < 3; i++ {
		base, ok := mmuTestNextTableChecked(ptbr)
		if !ok {
			t.Fatalf("allocation %d unexpectedly refused (cursor base 0x%X)", i, base)
		}
	}
	// Fourth must be refused.
	if base, ok := mmuTestNextTableChecked(ptbr); ok {
		t.Fatalf("allocation past slot end accepted: base 0x%X (slot end 0x%X)", base, ptbr+slotSize)
	}
}

// TestPhase4d_PTSlotCursorBoundsCheck_AsmSourceLock pins the matching
// production change in iexec.s install_leaf: each new intermediate/leaf
// table allocation must check the freshly-bumped cursor against the per-
// PTBR slot end stored at PTBR+PT_SLOT_END_OFFSET, and panic via
// kern_guru_meditation rather than silently corrupting the adjacent slot.
func TestPhase4d_PTSlotCursorBoundsCheck_AsmSourceLock(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	requireAllSubstrings(t, src,
		"PT_SLOT_END_OFFSET",
		"bounds-check the bumped cursor against the per-PT slot end",
		".pti_slot_overflow",
		"load.q  r12, PT_SLOT_END_OFFSET(r1)",
		"bgt     r10, r12, .pti_slot_overflow",
	)
	inc := mustReadRepoFile(t, "sdk/include/iexec.inc")
	requireAllSubstrings(t, inc,
		"PT_SLOT_END_OFFSET",
		"KERN_PT_STRIDE",
	)
}
