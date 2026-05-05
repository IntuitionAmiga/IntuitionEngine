package main

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	execMsgListResidentInventory = 0x128
	rsivMagic                    = 0x56495352
	rsivVersion                  = 1
	rsivHeaderSize               = 32
	rsivRecordSize               = 64
	rsivFlagTruncated            = 1
	rsivStatusResident           = 1 << 0
	rsivStatusProtected          = 1 << 1
	m1642ErrBadHandle            = 2
)

type m1642InventoryRecord struct {
	name        string
	kind        byte
	class       byte
	state       byte
	statusFlags byte
	version     uint16
	revision    uint16
	patch       uint16
	reserved0   uint16
	moduleFlags uint32
	openCount   uint32
	generation  uint32
	reserved1   uint64
}

func TestIExec_M1642_ABIConstantsAndVersionBump(t *testing.T) {
	vals := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))
	want := map[string]uint32{
		"IOS_VERSION_PATCH":                7,
		"EXEC_MSG_LIST_RESIDENT_INVENTORY": execMsgListResidentInventory,
		"RSIV_MAGIC":                       rsivMagic,
		"RSIV_VERSION":                     rsivVersion,
		"RSIV_HEADER_SIZE":                 rsivHeaderSize,
		"RSIV_RECORD_SIZE":                 rsivRecordSize,
		"RSIV_FLAG_TRUNCATED":              rsivFlagTruncated,
	}
	for name, value := range want {
		if got := vals[name]; got != value {
			t.Fatalf("%s=%#x, want %#x", name, got, value)
		}
	}
	if _, ok := vals["SYS_LIST_RESIDENT_INVENTORY"]; ok {
		t.Fatal("resident inventory must be exec.library IPC, not a new syscall")
	}
}

func TestIExec_M1642_SetResidentSpecificErrors(t *testing.T) {
	body := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/iexec.s"))
	for _, want := range []string{
		"beqz    r1, .m16sr_notfound",
		"bne     r23, r11, .m16sr_unsupported",
		"beq     r11, r12, .m16sr_unsupported",
		".m16sr_notfound:",
		"move.q  r2, #ERR_NOTFOUND",
		".m16sr_unsupported:",
		"move.q  r2, #ERR_UNSUPPORTED",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SYS_SET_RESIDENT specific-error implementation missing %q", want)
		}
	}
}

func TestIExec_M1642_ListResidentInventoryViaExecLibraryIPC(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	got := runM1642ListResidentInventoryClient(t, client, m1642ShareOnePage)
	if got.findErr != 0 || got.allocErr != 0 || got.putErr != 0 || got.replyErr != 0 || got.waitErr != 0 {
		t.Fatalf("inventory IPC errors find=%d alloc=%d put=%d reply=%d wait=%d records=%v", got.findErr, got.allocErr, got.putErr, got.replyErr, got.waitErr, got.records)
	}
	if got.replyType != uint64(execMsgListResidentInventory|execReplyFlag) {
		t.Fatalf("reply type=%#x, want %#x", got.replyType, execMsgListResidentInventory|execReplyFlag)
	}
	if got.magic != rsivMagic || got.version != rsivVersion || got.headerSize != rsivHeaderSize || got.recordSize != rsivRecordSize {
		t.Fatalf("bad RSIV header magic=%#x version=%d header=%d record=%d", got.magic, got.version, got.headerSize, got.recordSize)
	}
	if got.reserved0 != 0 || got.reserved1 != 0 || got.replyShare != 0 {
		t.Fatalf("reserved/reply share not zero: header reserved0=%d reserved1=%d replyShare=%d", got.reserved0, got.reserved1, got.replyShare)
	}
	if got.bytesUsed != uint32(rsivHeaderSize+len(got.records)*rsivRecordSize) {
		t.Fatalf("bytes_used=%d records=%d", got.bytesUsed, len(got.records))
	}
	rec, ok := got.byName("dos.library")
	if !ok {
		t.Fatalf("dos.library missing from resident inventory: %#v", got.records)
	}
	if rec.class != modClassLibrary || rec.kind != byte(valsIOSMKindLibrary(t)) {
		t.Fatalf("dos.library class=%d kind=%d, want library", rec.class, rec.kind)
	}
	if rec.statusFlags&rsivStatusResident == 0 || rec.statusFlags&rsivStatusProtected == 0 {
		t.Fatalf("dos.library status_flags=%#x, want resident+protected", rec.statusFlags)
	}
	if rec.moduleFlags&modfResident == 0 {
		t.Fatalf("dos.library flags=%#x, want MODF_RESIDENT", rec.moduleFlags)
	}
	if rec.reserved0 != 0 || rec.reserved1 != 0 {
		t.Fatalf("dos.library reserved fields not zero: reserved0=%d reserved1=%d", rec.reserved0, rec.reserved1)
	}
}

func TestIExec_M1642_ListResidentInventoryRejectsZeroShare(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	got := runM1642ListResidentInventoryClient(t, client, m1642ShareZero)
	if got.replyErr != uint64(errBadArg) {
		t.Fatalf("zero-share inventory err=%d, want ERR_BADARG", got.replyErr)
	}
	if got.replyType != uint64(execMsgListResidentInventory|execReplyFlag) {
		t.Fatalf("zero-share reply type=%#x, want flagged inventory reply", got.replyType)
	}
}

func TestIExec_M1642_ListResidentInventoryRejectsWrongShareSize(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	got := runM1642ListResidentInventoryClient(t, client, m1642ShareTwoPages)
	if got.replyType != uint64(execMsgListResidentInventory|execReplyFlag) {
		t.Fatalf("wrong-size reply type=%#x, want flagged inventory reply", got.replyType)
	}
	if got.replyCnt != 0 || got.replyErr != uint64(errBadArg) {
		t.Fatalf("wrong-size reply count=%d err=%d, want count=0 ERR_BADARG", got.replyCnt, got.replyErr)
	}
	if got.replyShare != 0 {
		t.Fatalf("wrong-size reply share=%d, want 0", got.replyShare)
	}
}

func TestIExec_M1642_ListResidentInventoryRejectsInvalidShareHandle(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	got := runM1642ListResidentInventoryClient(t, client, m1642ShareInvalid)
	if got.replyType != uint64(execMsgListResidentInventory|execReplyFlag) {
		t.Fatalf("invalid-handle reply type=%#x, want flagged inventory reply", got.replyType)
	}
	if got.replyCnt != 0 || got.replyErr != uint64(m1642ErrBadHandle) {
		t.Fatalf("invalid-handle reply count=%d err=%d, want count=0 ERR_BADHANDLE", got.replyCnt, got.replyErr)
	}
	if got.replyShare != 0 {
		t.Fatalf("invalid-handle reply share=%d, want 0", got.replyShare)
	}
}

func TestIExec_M1642_ResidentCommandSurfaceWiredToInventoryIPC(t *testing.T) {
	src := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd/resident.s"))
	for _, want := range []string{
		"#EXEC_MSG_LIST_RESIDENT_INVENTORY",
		"#SYS_SET_RESIDENT",
		"#SYS_PUT_MSG",
		"console.handler",
		"Resident: added",
		"Resident: removed",
		"Resident: not found",
		"Resident: unsupported target",
		"Resident usage: Resident [<name> ADD|REMOVE]",
		"dc.w    1\n    dc.w    2\n    dc.w    0",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("Resident command source missing %q", want)
		}
	}
	if strings.Contains(src, "#SYS_DEBUG_PUTCHAR") {
		t.Fatal("Resident command must route output through console.handler, not SYS_DEBUG_PUTCHAR")
	}
	elfBytes := mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd_resident.elf")
	for _, want := range [][]byte{
		[]byte("Resident: added"),
		[]byte("Resident: unsupported target"),
		[]byte("Resident usage: Resident [<name> ADD|REMOVE]"),
	} {
		if !strings.Contains(string(elfBytes), string(want)) {
			t.Fatalf("cmd_resident.elf missing %q; run make intuitionos after command changes", string(want))
		}
	}
}

func TestIExec_M1642_ResidentCommandListPreservesIPCStateAcrossSyscalls(t *testing.T) {
	src := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd/resident.s"))
	for _, want := range []string{
		"store.q r1, 56(r29)                ; exec.library port",
		"load.q  r1, 56(r29)",
		"store.l r26, 64(r29)               ; resident inventory record count",
		"store.l r0, 72(r29)                ; resident inventory loop index",
		"load.l  r26, 64(r29)",
		"load.l  r27, 72(r29)",
		"store.l r27, 72(r29)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("Resident list path does not preserve volatile IPC state; missing %q", want)
		}
	}
}

func TestIExec_M1642_ResidentCommandMutationDirectionSurvivesSyscall(t *testing.T) {
	src := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd/resident.s"))
	for _, want := range []string{
		"store.l r2, 80(r29)                ; resident mutation direction",
		"load.l  r24, 80(r29)",
		"ds.b    8                         ; +80 resident mutation direction",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("Resident mutation path keeps direction in volatile registers; missing %q", want)
		}
	}
}

func TestIExec_M1642_ShellResidentCommandListsInventoryAndShowsUsage(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	rig, term := assembleAndLoadKernelWithBootstrapHostRoot(t, hostRoot)
	rig.cpu.running.Store(true)
	bootDone := make(chan struct{})
	go func() { rig.cpu.Execute(); close(bootDone) }()
	time.Sleep(5 * time.Second)
	rig.cpu.running.Store(false)
	<-bootDone
	term.DrainOutput()

	if _, ok := m16FindModuleRowByName(rig.cpu.memory, "dos.library"); !ok {
		t.Fatal("dos.library row missing before shell Resident test")
	}
	for _, ch := range "ECHO BEFORE\nRESIDENT intuition.library ADD\nRESIDENT about ADD\nECHO AFTER\nRESIDENT\nRESIDENT all\n" {
		term.EnqueueByte(byte(ch))
	}
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(12 * time.Second)
	rig.cpu.running.Store(false)
	waitDoneWithGuard(t, done)
	output := term.DrainOutput()
	if !strings.Contains(output, "BEFORE") || !strings.Contains(output, "AFTER") {
		t.Fatalf("shell injected ECHO commands did not print output=%q", output[:min(len(output), 500)])
	}
	if !strings.Contains(output, "Resident: added") {
		t.Fatalf("Resident ADD did not print status output=%q", output[:min(len(output), 500)])
	}
	if strings.Count(output, "Resident: added") < 2 {
		t.Fatalf("Resident protected row and command ADD did not both print added output=%q", output[:min(len(output), 500)])
	}
	if !strings.Contains(output, "dos.library") || !strings.Contains(output, "intuition.library") {
		t.Fatalf("Resident did not list resident inventory output=%q", output[:min(len(output), 500)])
	}
	if strings.Contains(output, "Resident usage: Resident [<name> ADD|REMOVE]") {
		t.Fatalf("Resident ALL printed usage instead of listing inventory output=%q", output[:min(len(output), 500)])
	}
	idx, ok := m16FindModuleRowByName(rig.cpu.memory, "intuition.library")
	if !ok {
		t.Fatalf("intuition.library row missing after Resident ADD output=%q", output[:min(len(output), 500)])
	}
	intuiRow := m16ModuleRowBase(idx)
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[intuiRow+kdModuleFlags:]); got&modfResident == 0 {
		t.Fatalf("Resident ADD did not mutate intuition.library flags=%#x output=%q", got, output[:min(len(output), 500)])
	}
}

func TestIExec_M1642_ResidentCommandDOSRunMutatesEligibleLibrary(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	writeHostRootFileBytes(t, hostRoot, "S/Startup-Sequence", []byte("Version\n"))
	rig, term := assembleAndLoadKernelWithBootstrapHostRoot(t, hostRoot)
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(5 * time.Second)
	rig.cpu.running.Store(false)
	waitDoneWithGuard(t, done)
	term.DrainOutput()

	idx, ok := m16FindModuleRowByName(rig.cpu.memory, "dos.library")
	if !ok {
		t.Fatal("dos.library row missing before Resident ADD")
	}
	row := m16ModuleRowBase(idx)
	flags := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleFlags:])
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleFlags:], flags&^modfResident)

	input := append(append([]byte("Resident"), 0), append([]byte("dos.library ADD"), 0)...)
	var dataBase uint32
	rig, dataBase = runDOSRunClientOnRig(t, rig, input)
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+160:]); got != dosOK {
		t.Fatalf("DOS_RUN Resident reply=%d, want DOS_OK input=%q output=%q", got, input, term.DrainOutput())
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleFlags:]); got&modfResident == 0 {
		t.Fatalf("Resident ADD did not set MODF_RESIDENT; flags=%#x", got)
	}
}

func valsIOSMKindLibrary(t *testing.T) uint32 {
	t.Helper()
	return parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["IOSM_KIND_LIBRARY"]
}

type m1642InventoryResult struct {
	findErr    uint64
	allocErr   uint64
	putErr     uint64
	waitErr    uint64
	replyType  uint64
	replyCnt   uint64
	recordCnt  uint32
	replyErr   uint64
	replyShare uint64
	magic      uint32
	version    uint16
	headerSize uint16
	recordSize uint16
	reserved0  uint16
	flags      uint32
	bytesUsed  uint32
	reserved1  uint64
	records    []m1642InventoryRecord
}

func (r m1642InventoryResult) byName(name string) (m1642InventoryRecord, bool) {
	for _, rec := range r.records {
		if rec.name == name {
			return rec, true
		}
	}
	return m1642InventoryRecord{}, false
}

type m1642ShareMode int

const (
	m1642ShareZero m1642ShareMode = iota
	m1642ShareOnePage
	m1642ShareTwoPages
	m1642ShareInvalid
)

func runM1642ListResidentInventoryClient(t *testing.T, client *m161Task0ClientRig, shareMode m1642ShareMode) m1642InventoryResult {
	t.Helper()

	const (
		offName     = 0x500
		offPortID   = 0x540
		offFindErr  = 0x548
		offReplyPrt = 0x550
		offBufferVA = 0x558
		offAllocErr = 0x560
		offShareHdl = 0x568
		offPutErr   = 0x570
		offReplyTyp = 0x578
		offReplyCnt = 0x580
		offReplyErr = 0x588
		offReplySh  = 0x590
		offWaitErr  = 0x598
		offSentinel = 0x5A0
	)

	mem := client.rig.cpu.memory
	clear(mem[client.datap+offName : client.datap+offName+0x600])
	copy(mem[client.datap+offName:], []byte("exec.library\x00"))

	pc := client.t0p
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0p+MMU_PAGE_SIZE {
			t.Fatalf("inventory client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0p)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	w(ie64Instr(OP_SUB, 31, IE64_SIZE_L, 1, 31, 0, 16))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, client.data))
	w(ie64Instr(OP_STORE, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, client.data+offName))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFindPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offPortID))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offFindErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysCreatePort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offReplyPrt))
	switch shareMode {
	case m1642ShareOnePage, m1642ShareTwoPages:
		allocSize := uint32(4096)
		if shareMode == m1642ShareTwoPages {
			allocSize = 8192
		}
		w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, allocSize))
		w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, uint32(memfPublic|memfClear)))
		w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
		w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
		w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	case m1642ShareInvalid:
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
		w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFF))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	default:
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	}
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offPortID))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, execMsgListResidentInventory))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 0, 29, 0, offShareHdl))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offPutErr))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offReplyTyp))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offReplyCnt))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offWaitErr))
	w(ie64Instr(OP_STORE, 4, IE64_SIZE_Q, 1, 29, 0, offReplyErr))
	w(ie64Instr(OP_STORE, 6, IE64_SIZE_Q, 1, 29, 0, offReplySh))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offSentinel))
	w(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	resetM161Task0ClientState(client)
	runRigForDuration(t, client.rig, 300*time.Millisecond)
	if got := binary.LittleEndian.Uint64(mem[client.datap+offSentinel:]); got != 0xCAFE {
		output := client.term.DrainOutput()
		t.Fatalf("inventory client did not finish: sentinel=%#x output=%q", got, output[:min(len(output), 800)])
	}

	res := m1642InventoryResult{
		findErr:    binary.LittleEndian.Uint64(mem[client.datap+offFindErr:]),
		allocErr:   binary.LittleEndian.Uint64(mem[client.datap+offAllocErr:]),
		putErr:     binary.LittleEndian.Uint64(mem[client.datap+offPutErr:]),
		waitErr:    binary.LittleEndian.Uint64(mem[client.datap+offWaitErr:]),
		replyType:  binary.LittleEndian.Uint64(mem[client.datap+offReplyTyp:]),
		replyCnt:   binary.LittleEndian.Uint64(mem[client.datap+offReplyCnt:]),
		recordCnt:  uint32(binary.LittleEndian.Uint64(mem[client.datap+offReplyCnt:])),
		replyErr:   binary.LittleEndian.Uint64(mem[client.datap+offReplyErr:]),
		replyShare: binary.LittleEndian.Uint64(mem[client.datap+offReplySh:]),
	}
	if shareMode != m1642ShareOnePage {
		return res
	}
	bufVA := uint32(binary.LittleEndian.Uint64(mem[client.datap+offBufferVA:]))
	bufPhys, ok := taskVAToPhys(mem, client.pub, uint64(bufVA))
	if !ok {
		t.Fatalf("inventory buffer VA %#x is not mapped", bufVA)
	}
	res.magic = binary.LittleEndian.Uint32(mem[bufPhys:])
	res.version = binary.LittleEndian.Uint16(mem[bufPhys+4:])
	res.headerSize = binary.LittleEndian.Uint16(mem[bufPhys+6:])
	res.recordSize = binary.LittleEndian.Uint16(mem[bufPhys+8:])
	res.reserved0 = binary.LittleEndian.Uint16(mem[bufPhys+10:])
	res.recordCnt = binary.LittleEndian.Uint32(mem[bufPhys+12:])
	res.bytesUsed = binary.LittleEndian.Uint32(mem[bufPhys+16:])
	res.flags = binary.LittleEndian.Uint32(mem[bufPhys+20:])
	res.reserved1 = binary.LittleEndian.Uint64(mem[bufPhys+24:])
	for i := uint32(0); i < res.recordCnt && i < 64; i++ {
		off := bufPhys + rsivHeaderSize + i*rsivRecordSize
		res.records = append(res.records, m1642InventoryRecord{
			name:        cStringFromFixed(mem[off : off+32]),
			kind:        mem[off+32],
			class:       mem[off+33],
			state:       mem[off+34],
			statusFlags: mem[off+35],
			version:     binary.LittleEndian.Uint16(mem[off+36:]),
			revision:    binary.LittleEndian.Uint16(mem[off+38:]),
			patch:       binary.LittleEndian.Uint16(mem[off+40:]),
			reserved0:   binary.LittleEndian.Uint16(mem[off+42:]),
			moduleFlags: binary.LittleEndian.Uint32(mem[off+44:]),
			openCount:   binary.LittleEndian.Uint32(mem[off+48:]),
			generation:  binary.LittleEndian.Uint32(mem[off+52:]),
			reserved1:   binary.LittleEndian.Uint64(mem[off+56:]),
		})
	}
	return res
}
