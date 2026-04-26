package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

const (
	m164TestTask0CodeTarget  = 0x620000
	m164TestTask0DataTarget  = 0x621000
	m164TestTask0StackTarget = 0x623000
	m164ASLRStateOffset      = 66848
	m164TaskImgBitmapOffset  = 65688
	m164TaskImgBitmapSize    = 64
	m164LegacyTask0Seed      = 7
)

func m164InstallLegacyTask0Seed(mem []byte) {
	binary.LittleEndian.PutUint64(mem[kernDataBase+m164ASLRStateOffset:], m164LegacyTask0Seed)
}

func m164TaskPhysOrFatal(t *testing.T, mem []byte, taskID uint64, va uint32) uint32 {
	t.Helper()
	phys, ok := taskVAToPhys(mem, taskID, uint64(va))
	if !ok && taskID == 0 && va == m164TestTask0DataTarget {
		return userStackBase
	}
	if !ok {
		t.Fatalf("could not translate task %d VA 0x%X", taskID, va)
	}
	return phys
}

func m164WaitTaskPhysOrFatal(t *testing.T, mem []byte, taskID uint64, va uint32, timeout time.Duration) uint32 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if phys, ok := taskVAToPhys(mem, taskID, uint64(va)); ok {
			return phys
		}
		time.Sleep(time.Millisecond)
	}
	return m164TaskPhysOrFatal(t, mem, taskID, va)
}

func m164TaskStartupTargetQ(t *testing.T, mem []byte, taskID uint64, off uint32) uint32 {
	t.Helper()
	startupBase := taskLayoutFieldQ(mem, taskID, kdTaskStartupBase)
	if startupBase == 0 {
		t.Fatalf("task %d startup base is zero", taskID)
	}
	return uint32(binary.LittleEndian.Uint64(mem[uint32(startupBase)+off:]))
}

func m164TaskReader(t *testing.T, mem []byte, taskID uint64, baseVA uint32) func(uint32) uint64 {
	t.Helper()
	basePhys, ok := taskVAToPhys(mem, taskID, uint64(baseVA))
	if !ok {
		basePhys = m164TaskPhysOrFatal(t, mem, taskID, baseVA)
	}
	return func(off uint32) uint64 {
		return binary.LittleEndian.Uint64(mem[basePhys+off:])
	}
}

func m164NextASLRBaseFromState(state uint64) (uint64, uint32) {
	next := state*1664525 + 1013904223
	slot := uint32((next >> 16) & 0xFF)
	return next, uint32(userCodeBase) + slot*MMU_PAGE_SIZE
}

func m164ImageBitmapSet(mem []byte, base uint32, pages uint32) {
	first := (base - uint32(userCodeBase)) / MMU_PAGE_SIZE
	for i := uint32(0); i < pages; i++ {
		bit := first + i
		mem[kernDataBase+m164TaskImgBitmapOffset+bit/8] |= byte(1 << (bit & 7))
	}
}

func m164BootAndResetToDosTask(t *testing.T) (*ie64TestRig, shellTaskLayout) {
	t.Helper()
	rig, _ := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(2 * time.Second)
	rig.cpu.running.Store(false)
	<-done

	for slot := uint32(0); slot < maxTasks; slot++ {
		state := rig.cpu.memory[kernDataBase+kdTCBBase+slot*tcbStride+tcbStateOff]
		if state == taskFree {
			continue
		}
		pubid := binary.LittleEndian.Uint32(rig.cpu.memory[kernDataBase+kdTaskPubIDBase+slot*kdTaskPubIDStr:])
		dataBase := uint32(taskLayoutFieldQ(rig.cpu.memory, uint64(pubid), kdTaskDataBase))
		if dataBase == 0 {
			continue
		}
		dataPhys, ok := taskVAToPhys(rig.cpu.memory, uint64(pubid), uint64(dataBase))
		if !ok || dataPhys+43 >= uint32(len(rig.cpu.memory)) {
			continue
		}
		if !strings.HasPrefix(string(rig.cpu.memory[dataPhys+32:dataPhys+43]), "dos.library") {
			continue
		}
		codeBase := taskLayoutFieldQ(rig.cpu.memory, uint64(pubid), kdTaskCodeBase)
		runtimeDataBase := uint64(dataBase)
		if startupBase := taskLayoutFieldQ(rig.cpu.memory, uint64(pubid), kdTaskStartupBase); startupBase != 0 {
			if got := binary.LittleEndian.Uint64(rig.cpu.memory[uint32(startupBase)+taskStartupCodeBase:]); got != 0 {
				codeBase = got
			}
			if got := binary.LittleEndian.Uint64(rig.cpu.memory[uint32(startupBase)+taskStartupDataBase:]); got != 0 {
				runtimeDataBase = got
			}
		}
		codePhys, ok := taskVAToPhys(rig.cpu.memory, uint64(pubid), codeBase)
		if !ok {
			continue
		}
		dataPhys, ok = taskVAToPhys(rig.cpu.memory, uint64(pubid), runtimeDataBase)
		if !ok {
			continue
		}
		layout := shellTaskLayout{
			slot:     slot,
			pubid:    pubid,
			codeVA:   uint32(codeBase),
			codePhys: codePhys,
			dataVA:   uint32(runtimeDataBase),
			dataPhys: dataPhys,
			pt:       uint32(taskLayoutFieldQ(rig.cpu.memory, uint64(pubid), kdTaskLayoutPT)),
			usp:      binary.LittleEndian.Uint64(rig.cpu.memory[kernDataBase+kdTCBBase+slot*tcbStride+tcbUSPOff:]),
		}
		if layout.pt == 0 || layout.usp == 0 {
			t.Fatalf("dos.library layout incomplete: pubid=%d code=0x%X data=0x%X pt=0x%X usp=0x%X", layout.pubid, layout.codeVA, layout.dataVA, layout.pt, layout.usp)
		}
		tcb := kernDataBase + kdTCBBase + slot*tcbStride
		binary.LittleEndian.PutUint64(rig.cpu.memory[kernDataBase+kdCurrentTask:], uint64(slot))
		binary.LittleEndian.PutUint64(rig.cpu.memory[tcb+tcbPCOff:], uint64(layout.codeVA))
		binary.LittleEndian.PutUint32(rig.cpu.memory[tcb+tcbSigWaitOff:], 0)
		rig.cpu.memory[tcb+tcbStateOff] = taskRunning
		rig.cpu.memory[tcb+tcbGPRSavedOff] = 0
		rig.cpu.PC = uint64(layout.codeVA)
		rig.cpu.regs[31] = layout.usp
		rig.cpu.userSP = layout.usp
		rig.cpu.kernelSP = kernStackTop
		rig.cpu.ptbr = layout.pt
		rig.cpu.supervisorMode = false
		rig.cpu.previousMode = false
		rig.cpu.trapped = false
		rig.cpu.faultCause = 0
		rig.cpu.faultAddr = 0
		rig.cpu.faultPC = 0
		rig.cpu.timerEnabled.Store(false)
		rig.cpu.interruptEnabled.Store(false)
		rig.cpu.timerCount.Store(0)
		return rig, layout
	}
	t.Fatal("could not locate live dos.library task")
	return nil, shellTaskLayout{}
}

func m164RunPrivateASLRSelectorFromDosTask(t *testing.T, rig *ie64TestRig, layout shellTaskLayout, pages uint32) (uint64, uint64) {
	t.Helper()
	mem := rig.cpu.memory
	outVA := layout.dataVA + 0x300
	copy(mem[layout.codePhys:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 7))
	copy(mem[layout.codePhys+8:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, pages))
	copy(mem[layout.codePhys+16:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysGetSysInfo))
	copy(mem[layout.codePhys+24:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, outVA))
	copy(mem[layout.codePhys+32:], ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0))
	copy(mem[layout.codePhys+40:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 3, 0, 8))
	copy(mem[layout.codePhys+48:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(1500 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	return binary.LittleEndian.Uint64(mem[layout.dataPhys+0x300:]), binary.LittleEndian.Uint64(mem[layout.dataPhys+0x308:])
}

func TestIExec_M164_DocsFreezeRuntimeETDYNContract(t *testing.T) {
	for _, rel := range []string{
		"sdk/docs/IntuitionOS/M16.4-plan.md",
		"sdk/docs/IntuitionOS/ELF.md",
		"sdk/docs/IntuitionOS/Toolchain.md",
		"sdk/docs/IntuitionOS/IExec.md",
	} {
		body := mustReadRepoFile(t, rel)
		for _, fragment := range []string{
			"self-contained `ET_DYN`",
			"dynamic linking",
			"PT_NOTE",
			"W^X",
			"KASLR",
		} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s missing M16.4 contract fragment %q", rel, fragment)
			}
		}
	}
}

func TestIExec_M164_RuntimeELFValidatorAcceptsZeroRelocationETDYN(t *testing.T) {
	image := makeM1641ELFFixture(t, nil, false)
	if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err != nil {
		t.Fatalf("valid zero-relocation ET_DYN rejected: %v", err)
	}
}

func TestIExec_M164_RuntimeELFValidatorAppliesRelative64Relocation(t *testing.T) {
	image := makeM1641ELFFixture(t, []m164RelaSpec{{Offset: 0x2000, Type: m164RelRelative64, Addend: 0x2000}}, false)
	mapped := make([]byte, 0x3000)
	_, entry, err := m164LoadRuntimeELF(image, m164Placement{Base: 0x00650000}, mapped)
	if err != nil {
		t.Fatalf("valid relative relocation rejected: %v", err)
	}
	if entry != 0x00650000 {
		t.Fatalf("entry=0x%X, want chosen base for e_entry=0", entry)
	}
	if got := binary.LittleEndian.Uint64(mapped[0x2000:]); got != 0x00652000 {
		t.Fatalf("relocated pointer=0x%X, want 0x652000", got)
	}
}

func TestIExec_M164_RuntimeELFValidatorAppliesBSSRelative64Relocation(t *testing.T) {
	image := makeM1641ELFFixture(t, []m164RelaSpec{{Offset: 0x2008, Type: m164RelRelative64, Addend: 0x2000}}, false)
	mapped := make([]byte, 0x3000)
	_, _, err := m164LoadRuntimeELF(image, m164Placement{Base: 0x00650000}, mapped)
	if err != nil {
		t.Fatalf("valid BSS relative relocation rejected: %v", err)
	}
	if got := binary.LittleEndian.Uint64(mapped[0x2008:]); got != 0x00652000 {
		t.Fatalf("BSS relocated pointer=0x%X, want 0x652000", got)
	}
}

func TestIExec_M164_RuntimeELFValidatorRejectsForbiddenInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func([]byte)
	}{
		{"ET_EXEC", func(img []byte) { binary.LittleEndian.PutUint16(img[16:18], m14ELFTypeExec) }},
		{"nonzero lowest PT_LOAD", func(img []byte) { binary.LittleEndian.PutUint64(img[64+16:64+24], 0x600000) }},
		{"entry outside text", func(img []byte) { binary.LittleEndian.PutUint64(img[24:32], 0x2000) }},
		{"nonzero section headers", func(img []byte) { binary.LittleEndian.PutUint16(img[60:62], 1) }},
		{"dynamic header", func(img []byte) { binary.LittleEndian.PutUint32(img[64:68], m14ELFPTDynamic) }},
		{"WX segment", func(img []byte) {
			binary.LittleEndian.PutUint32(img[64+56+4:64+56+8], m14ELFSegFlagR|m14ELFSegFlagW|m14ELFSegFlagX)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			image := makeM1641ELFFixture(t, nil, false)
			tc.patch(image)
			if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err == nil {
				t.Fatalf("%s accepted", tc.name)
			}
		})
	}
}

func TestIExec_M164_RuntimeELFValidatorRejectsBadRelocations(t *testing.T) {
	for _, tc := range []struct {
		name string
		rel  m164RelaSpec
	}{
		{"unaligned target", m164RelaSpec{Offset: 0x2001, Type: m164RelRelative64, Addend: 0x1000}},
		{"external symbol", m164RelaSpec{Offset: 0x2000, Symbol: 1, Type: m164RelRelative64, Addend: 0x1000}},
		{"unknown type", m164RelaSpec{Offset: 0x2000, Type: 99, Addend: 0x1000}},
		{"target in text", m164RelaSpec{Offset: 0x1000, Type: m164RelRelative64, Addend: 0x1000}},
		{"negative addend", m164RelaSpec{Offset: 0x2000, Type: m164RelRelative64, Addend: -1}},
		{"gap addend", m164RelaSpec{Offset: 0x2000, Type: m164RelRelative64, Addend: 0x1800}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			image := makeM1641ELFFixture(t, []m164RelaSpec{tc.rel}, false)
			if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err == nil {
				t.Fatalf("%s accepted", tc.name)
			}
		})
	}
}

func TestIExec_M164_AllShippedRuntimeELFsAreETDYNZeroRelativeAndSelfContained(t *testing.T) {
	for _, target := range m163RuntimeELFTargets(t) {
		t.Run(target.elfName, func(t *testing.T) {
			image := mustReadRepoBytes(t, "sdk/intuitionos/iexec/"+target.elfName)
			if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err != nil {
				t.Fatalf("%s violates M16.4 runtime ELF contract: %v", target.elfName, err)
			}
		})
	}
}

func TestIExec_M164_ASLRPlacementUsesKernelPRNGContract(t *testing.T) {
	inc := mustReadRepoFile(t, "sdk/include/iexec.inc")
	iexec := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	doslib := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")

	requireAllSubstrings(t, inc,
		"KD_NONCE_COUNTER  equ 24          ; uint64: monotonic shared-memory nonce counter",
		"KD_ASLR_STATE            equ 66848 ; u64 private M16.4 ASLR PRNG seed/state",
	)
	requireAllSubstrings(t, iexec,
		"SYSINFO_ASLR_IMAGE_BASE equ 7",
		"load.q  r11, KD_ASLR_STATE(r12)",
		"kern_aslr_next_base:",
		"load.q  r1, KD_ASLR_STATE(r5)",
		"bnez    r11, .init_aslr_seed_ready",
		"move.l  r2, #1664525",
		"move.l  r11, #SYSINFO_ASLR_IMAGE_BASE",
		"load.l  r12, KD_DOSLIB_PUBID(r11)",
		"bne     r1, r12, .info_aslr_denied",
		"move.q  r2, #ERR_PERM",
		"move.q  r1, r13",
		"jsr     kern_aslr_choose_image_base",
	)
	requireAllSubstrings(t, iexec,
		"kern_aslr_choose_image_base:",
		"move.l  r6, #16",
		"jsr     kern_aslr_next_base",
		".kaslrci_collision:",
		"move.q  r2, #ERR_NOMEM",
		".blei_zero_temp_data:",
		"store.q r5, M14_LDSEG_OFF_SRCSZ(r30)",
		"jsr     boot_elf_apply_relocations",
		"boot_elf_apply_relocations:",
	)
	requireAllSubstrings(t, doslib,
		"SYSINFO_ASLR_IMAGE_BASE equ 7",
		"move.l  r1, #SYSINFO_ASLR_IMAGE_BASE",
		"syscall #SYS_GET_SYS_INFO",
	)
	requireNoSubstrings(t, inc,
		"SYSINFO_ASLR_IMAGE_BASE",
	)
	requireNoSubstrings(t, iexec,
		"load.q  r5, KD_TASKID_NEXT(r4)\n    and     r5, r5, #7",
		"move.l  r1, #1\n    jsr     kern_aslr_choose_image_base",
	)
	requireNoSubstrings(t, doslib,
		"load.q  r3, 888(r29)\n    add     r3, r3, #1",
	)
}

func TestIExec_M164_RoadmapCurrentStateMatchesRuntimeELFASLR(t *testing.T) {
	roadmap := mustReadRepoFile(t, "IntuitionOS_Roadmap.md")
	requireAllSubstrings(t, roadmap,
		"## Current State: M16.4.1 PT_NOTE Runtime ELF / Userland ASLR",
		"strict runtime ELF loading: stripped, section-header-free self-contained IE64 `ET_DYN`",
		"userland ASLR placement",
		"automatic cleanup of task-owned module handles/resources",
	)
	requireNoSubstrings(t, roadmap,
		"## Current State: M13 Complete",
		"does not add demand-load, PIE, relocation, ASLR",
	)
}

func TestIExec_M164_ASLRImageBaseSelectorDeniedToUserTasks(t *testing.T) {
	rig, _, _, _ := bootAndResetToTask0Only(t)
	beforeSeed := binary.LittleEndian.Uint64(rig.cpu.memory[kernDataBase+m164ASLRStateOffset:])
	dataVA := uint32(m164TestTask0DataTarget)
	codePhys := m164TaskPhysOrFatal(t, rig.cpu.memory, 0, m164TestTask0CodeTarget)

	copy(rig.cpu.memory[codePhys:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 7))
	copy(rig.cpu.memory[codePhys+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysGetSysInfo))
	copy(rig.cpu.memory[codePhys+16:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, dataVA))
	copy(rig.cpu.memory[codePhys+24:], ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0))
	copy(rig.cpu.memory[codePhys+32:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 3, 0, 8))
	copy(rig.cpu.memory[codePhys+40:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(1500 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	dataPhys := m164TaskPhysOrFatal(t, rig.cpu.memory, 0, dataVA)
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[dataPhys:]); got != 0 {
		t.Fatalf("ASLR selector result=%#x, want 0 for denied user task", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[dataPhys+8:]); got != errPerm {
		t.Fatalf("ASLR selector err=%d, want ERR_PERM (%d)", got, errPerm)
	}
	if afterSeed := binary.LittleEndian.Uint64(rig.cpu.memory[kernDataBase+m164ASLRStateOffset:]); afterSeed != beforeSeed {
		t.Fatalf("ASLR selector advanced PRNG for denied user task: before=%#x after=%#x", beforeSeed, afterSeed)
	}
}

func TestIExec_M164_ASLRPlacementCollisionRetryChoosesAnotherBase(t *testing.T) {
	rig, layout := m164BootAndResetToDosTask(t)
	mem := rig.cpu.memory
	const pages = uint32(2)
	bitmap := mem[kernDataBase+m164TaskImgBitmapOffset : kernDataBase+m164TaskImgBitmapOffset+m164TaskImgBitmapSize]
	clear(bitmap)
	binary.LittleEndian.PutUint64(mem[kernDataBase+m164ASLRStateOffset:], 1)

	state := uint64(1)
	state, first := m164NextASLRBaseFromState(state)
	m164ImageBitmapSet(mem, first, pages)
	var want uint32
	for attempt := 1; attempt < 16; attempt++ {
		state, want = m164NextASLRBaseFromState(state)
		firstPage := (want - uint32(userCodeBase)) / MMU_PAGE_SIZE
		free := true
		for i := uint32(0); i < pages; i++ {
			bit := firstPage + i
			if bitmap[bit/8]&(1<<(bit&7)) != 0 {
				free = false
			}
		}
		if free {
			break
		}
	}
	got, err := m164RunPrivateASLRSelectorFromDosTask(t, rig, layout, pages)
	if err != 0 {
		t.Fatalf("ASLR collision retry err=%d, want ERR_OK", err)
	}
	if got != uint64(want) {
		t.Fatalf("ASLR collision retry base=0x%X, want next free candidate 0x%X after occupied first=0x%X", got, want, first)
	}
}

func TestIExec_M164_ASLRPlacementExhaustionReturnsNOMEM(t *testing.T) {
	rig, layout := m164BootAndResetToDosTask(t)
	mem := rig.cpu.memory
	for i := uint32(0); i < m164TaskImgBitmapSize; i++ {
		mem[kernDataBase+m164TaskImgBitmapOffset+i] = 0xFF
	}
	binary.LittleEndian.PutUint64(mem[kernDataBase+m164ASLRStateOffset:], 1)

	got, err := m164RunPrivateASLRSelectorFromDosTask(t, rig, layout, 1)
	if got != 0 {
		t.Fatalf("ASLR exhausted base=0x%X, want 0", got)
	}
	if err != 1 {
		t.Fatalf("ASLR exhausted err=%d, want ERR_NOMEM (1)", err)
	}
}

func TestIExec_M164_BootRuntimeTasksUseChosenImageBases(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(300 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.Contains(output, "IntuitionOS 1.16.5") {
		t.Fatalf("boot did not reach VERSION output: %q", output[:min(len(output), 200)])
	}
	code0 := m164StartupCodeBase(t, rig.cpu.memory, 0)
	code1 := m164StartupCodeBase(t, rig.cpu.memory, 1)
	code2 := m164StartupCodeBase(t, rig.cpu.memory, 2)
	for taskID, code := range map[uint64]uint64{0: code0, 1: code1, 2: code2} {
		if code == uint64(userCodeBase) {
			t.Fatalf("task %d code base still uses fixed USER_CODE_BASE 0x%X", taskID, code)
		}
		if code < uint64(userCodeBase) || code >= uint64(userPTBase) || code%0x1000 != 0 {
			t.Fatalf("task %d code base 0x%X outside user image window", taskID, code)
		}
	}
	if code0 == code1 || code1 == code2 || code0 == code2 {
		t.Fatalf("boot task code bases not distinct: task0=0x%X task1=0x%X task2=0x%X", code0, code1, code2)
	}
}

func TestIExec_M164_DOSLoadSegUsesChosenImageBase(t *testing.T) {
	image := mustReadRepoBytes(t, "sdk/intuitionos/iexec/elfseg_fixture.elf")
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosOK {
		t.Fatalf("DOS_LOADSEG reply=%d, want DOS_OK", got)
	}
	seglistVA := binary.LittleEndian.Uint64(mem[dataBase+208:])
	if seglistVA == 0 {
		t.Fatal("DOS_LOADSEG returned zero seglist")
	}
	seglistPhys, ok := taskVAToPhys(mem, 1, seglistVA)
	if !ok {
		t.Fatalf("could not translate seglist VA 0x%X", seglistVA)
	}
	entry := binary.LittleEndian.Uint64(mem[seglistPhys+dosSegEntryVAOff:])
	target0 := binary.LittleEndian.Uint64(mem[seglistPhys+dosSegEntryBase+dosSegTargetOff:])
	if entry == uint64(userCodeBase) || target0 == uint64(userCodeBase) {
		t.Fatalf("LoadSeg still used fixed base: entry=0x%X target0=0x%X", entry, target0)
	}
	if entry < uint64(userCodeBase) || entry >= uint64(userPTBase) || target0 < uint64(userCodeBase) || target0 >= uint64(userPTBase) {
		t.Fatalf("LoadSeg target outside user image window: entry=0x%X target0=0x%X", entry, target0)
	}
}

func TestIExec_M164_DOSLoadSegHonorsNonzeroRelativeEntry(t *testing.T) {
	image := makeM164ELFFixture(t, nil)
	binary.LittleEndian.PutUint64(image[24:32], 4)
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosOK {
		t.Fatalf("DOS_LOADSEG reply=%d, want DOS_OK", got)
	}
	seglistVA := binary.LittleEndian.Uint64(mem[dataBase+208:])
	seglistPhys, ok := taskVAToPhys(mem, 1, seglistVA)
	if !ok {
		t.Fatalf("could not translate seglist VA 0x%X", seglistVA)
	}
	entry := binary.LittleEndian.Uint64(mem[seglistPhys+dosSegEntryVAOff:])
	target0 := binary.LittleEndian.Uint64(mem[seglistPhys+dosSegEntryBase+dosSegTargetOff:])
	if want := target0 + 4; entry != want {
		t.Fatalf("LoadSeg entry=0x%X, want first PT_LOAD base + e_entry 0x%X", entry, want)
	}
}

func TestIExec_M164_DOSLoadSegAppliesRelative64Relocations(t *testing.T) {
	image := makeM164ELFFixture(t, []m164RelaSpec{{Offset: 0x2000, Type: m164RelRelative64, Addend: 0x2000}})
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosOK {
		t.Fatalf("DOS_LOADSEG reply=%d, want DOS_OK", got)
	}
	seglistVA := binary.LittleEndian.Uint64(mem[dataBase+208:])
	seglistPhys, ok := taskVAToPhys(mem, 1, seglistVA)
	if !ok {
		t.Fatalf("could not translate seglist VA 0x%X", seglistVA)
	}
	if got := binary.LittleEndian.Uint32(mem[seglistPhys+dosSegCountOff:]); got != 2 {
		t.Fatalf("seg_count=%d, want 2", got)
	}
	dataEntry := seglistPhys + dosSegEntryBase + dosSegEntryStr
	dataVA := binary.LittleEndian.Uint64(mem[dataEntry+dosSegMemVAOff:])
	dataPhys, ok := taskVAToPhys(mem, 1, dataVA)
	if !ok {
		t.Fatalf("could not translate data segment VA 0x%X", dataVA)
	}
	want := binary.LittleEndian.Uint64(mem[dataEntry+dosSegTargetOff:])
	if got := binary.LittleEndian.Uint64(mem[dataPhys:]); got != want {
		t.Fatalf("relocated data pointer=0x%X, want data target 0x%X", got, want)
	}
}

func TestIExec_M164_DOSLoadSegAppliesBSSRelative64Relocations(t *testing.T) {
	image := makeM164ELFFixture(t, []m164RelaSpec{{Offset: 0x2008, Type: m164RelRelative64, Addend: 0x2000}})
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosOK {
		t.Fatalf("DOS_LOADSEG reply=%d, want DOS_OK", got)
	}
	seglistVA := binary.LittleEndian.Uint64(mem[dataBase+208:])
	seglistPhys, ok := taskVAToPhys(mem, 1, seglistVA)
	if !ok {
		t.Fatalf("could not translate seglist VA 0x%X", seglistVA)
	}
	dataEntry := seglistPhys + dosSegEntryBase + dosSegEntryStr
	dataVA := binary.LittleEndian.Uint64(mem[dataEntry+dosSegMemVAOff:])
	dataPhys, ok := taskVAToPhys(mem, 1, dataVA)
	if !ok {
		t.Fatalf("could not translate data segment VA 0x%X", dataVA)
	}
	want := binary.LittleEndian.Uint64(mem[dataEntry+dosSegTargetOff:])
	if got := binary.LittleEndian.Uint64(mem[dataPhys+8:]); got != want {
		t.Fatalf("BSS relocated data pointer=0x%X, want data target 0x%X", got, want)
	}
}

func TestIExec_M164_DOSLoadSegRejectsBadRelative64Relocations(t *testing.T) {
	image := makeM164ELFFixture(t, []m164RelaSpec{{Offset: 0x1000, Type: m164RelRelative64, Addend: 0x2000}})
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosErrBadArg {
		t.Fatalf("DOS_LOADSEG reply=%d, want DOS_ERR_BADARG (%d)", got, dosErrBadArg)
	}
	if seglist := binary.LittleEndian.Uint64(mem[dataBase+208:]); seglist != 0 {
		t.Fatalf("DOS_LOADSEG returned seglist VA 0x%X for invalid relocation", seglist)
	}
}

func TestIExec_M164_DOSLoadSegRejectsZeroMemszAsBadArg(t *testing.T) {
	image := makeM164ELFFixture(t, nil)
	binary.LittleEndian.PutUint64(image[64+40:64+48], 0)
	binary.LittleEndian.PutUint64(image[64+56+40:64+56+48], 0)
	rig, dataBase := m163RunLoadSegHostFixture(t, image)
	mem := rig.cpu.memory
	if got := binary.LittleEndian.Uint64(mem[dataBase+200:]); got != dosErrBadArg {
		t.Fatalf("DOS_LOADSEG zero memsz reply=%d, want DOS_ERR_BADARG (%d)", got, dosErrBadArg)
	}
	if seglist := binary.LittleEndian.Uint64(mem[dataBase+208:]); seglist != 0 {
		t.Fatalf("DOS_LOADSEG zero memsz returned seglist VA 0x%X", seglist)
	}
}

type m164RelaSpec struct {
	Offset uint64
	Symbol uint32
	Type   uint32
	Addend int64
}

func makeM164ELFFixture(t *testing.T, relocs []m164RelaSpec) []byte {
	t.Helper()
	const (
		codeOff = 0x1000
		dataOff = 0x2000
		noteOff = 0x3000
		relaOff = 0x3100
		strOff  = 0x3400
		shOff   = 0x3500
	)
	shstr := []byte("\x00.ios.manifest\x00.rela.dyn\x00.shstrtab\x00")
	note := makeM164IOSMNote()
	out := make([]byte, shOff+4*64)
	copy(out[codeOff:], []byte{0xE0, 0, 0, 0, 0, 0, 0, 0})
	copy(out[dataOff:], []byte{0, 0, 0, 0, 0, 0, 0, 0})
	copy(out[noteOff:], note)
	for i, rel := range relocs {
		off := relaOff + i*24
		binary.LittleEndian.PutUint64(out[off:off+8], rel.Offset)
		binary.LittleEndian.PutUint64(out[off+8:off+16], uint64(rel.Symbol)<<32|uint64(rel.Type))
		binary.LittleEndian.PutUint64(out[off+16:off+24], uint64(rel.Addend))
	}
	copy(out[strOff:], shstr)

	copy(out[0:16], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(out[16:18], m164ELFTypeDyn)
	binary.LittleEndian.PutUint16(out[18:20], m14ELFMachineIE64)
	binary.LittleEndian.PutUint32(out[20:24], 1)
	binary.LittleEndian.PutUint64(out[24:32], 0)
	binary.LittleEndian.PutUint64(out[32:40], 64)
	binary.LittleEndian.PutUint64(out[40:48], shOff)
	binary.LittleEndian.PutUint16(out[52:54], 64)
	binary.LittleEndian.PutUint16(out[54:56], 56)
	binary.LittleEndian.PutUint16(out[56:58], 2)
	binary.LittleEndian.PutUint16(out[58:60], 64)
	binary.LittleEndian.PutUint16(out[60:62], 4)
	binary.LittleEndian.PutUint16(out[62:64], 3)

	putPH := func(off int, flags uint32, fileOff, vaddr, filesz, memsz uint64) {
		binary.LittleEndian.PutUint32(out[off:off+4], m14ELFPTLoad)
		binary.LittleEndian.PutUint32(out[off+4:off+8], flags)
		binary.LittleEndian.PutUint64(out[off+8:off+16], fileOff)
		binary.LittleEndian.PutUint64(out[off+16:off+24], vaddr)
		binary.LittleEndian.PutUint64(out[off+24:off+32], vaddr)
		binary.LittleEndian.PutUint64(out[off+32:off+40], filesz)
		binary.LittleEndian.PutUint64(out[off+40:off+48], memsz)
		binary.LittleEndian.PutUint64(out[off+48:off+56], m14ELFPageAlign)
	}
	putPH(64, m14ELFSegFlagR|m14ELFSegFlagX, codeOff, 0, 8, 0x1000)
	putPH(64+56, m14ELFSegFlagR|m14ELFSegFlagW, dataOff, 0x2000, 8, 0x1000)

	putSH := func(idx int, name, typ uint32, off, size, align, entsize uint64) {
		base := shOff + idx*64
		binary.LittleEndian.PutUint32(out[base:base+4], name)
		binary.LittleEndian.PutUint32(out[base+4:base+8], typ)
		binary.LittleEndian.PutUint64(out[base+24:base+32], off)
		binary.LittleEndian.PutUint64(out[base+32:base+40], size)
		binary.LittleEndian.PutUint64(out[base+48:base+56], align)
		binary.LittleEndian.PutUint64(out[base+56:base+64], entsize)
	}
	putSH(1, 1, 7, noteOff, uint64(len(note)), 4, 0)
	putSH(2, 15, uint32(m164SHTRela), relaOff, uint64(len(relocs)*24), 8, 24)
	putSH(3, 25, 3, strOff, uint64(len(shstr)), 1, 0)
	return out
}

func makeM164IOSMNote() []byte {
	name := []byte("IOS-MOD\x00")
	desc := make([]byte, 128)
	binary.LittleEndian.PutUint32(desc[0:4], 0x4D534F49)
	binary.LittleEndian.PutUint32(desc[4:8], 1)
	desc[8] = 5
	copy(desc[16:48], []byte("M164Fixture"))
	binary.LittleEndian.PutUint32(desc[48:52], m163ModfASLRCapable)
	nameLen := (len(name) + 3) &^ 3
	out := make([]byte, 12+nameLen+len(desc))
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(name)))
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(desc)))
	binary.LittleEndian.PutUint32(out[8:12], 0x494F5331)
	copy(out[12:], name)
	copy(out[12+nameLen:], desc)
	return out
}

func m164StartupCodeBase(t *testing.T, mem []byte, taskID uint64) uint64 {
	t.Helper()
	startupBase := taskLayoutFieldQ(mem, taskID, kdTaskStartupBase)
	if startupBase == 0 {
		t.Fatalf("task %d startup base is zero", taskID)
	}
	return binary.LittleEndian.Uint64(mem[uint32(startupBase)+taskStartupCodeBase:])
}
