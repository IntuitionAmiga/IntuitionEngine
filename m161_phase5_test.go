package main

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type m161PortNameByIndexResult struct {
	taskID uint64
	err    uint64
	name   [32]byte
}

func TestIExec_M161_Phase5_PortNameByIndex_Iteration(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	want := []string{
		"exec.library",
		"console.handler",
		"dos.library",
		"hardware.resource",
		"input.device",
		"graphics.library",
	}
	for i, name := range want {
		got := runM161PortNameByIndexClient(t, client, uint32(i), true)
		if got.err != 0 {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) err=%d, want 0", i, got.err)
		}
		if got.taskID == 0 {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) taskID=0, want owner task id", i)
		}
		if gotName := cStringFromFixed(got.name[:]); gotName != name {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) name=%q, want %q", i, gotName, name)
		}
	}

	got := runM161PortNameByIndexClient(t, client, uint32(len(want)), true)
	if got.err != 0 {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel err=%d, want 0", got.err)
	}
	if got.taskID != 0 {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel taskID=%d, want 0", got.taskID)
	}
	if !bytes.Equal(got.name[:], make([]byte, len(got.name))) {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel name=%q, want zeroed buffer", got.name)
	}
}

func TestIExec_M161_Phase5_PortNameByIndex_BadBufferPointer(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	errBadArg := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["ERR_BADARG"]

	got := runM161PortNameByIndexClient(t, client, 0, false)
	if got.err != uint64(errBadArg) {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX bad buffer err=%d, want ERR_BADARG (%d)", got.err, errBadArg)
	}
}

type m161ListResidentsResult struct {
	findErr  uint64
	allocErr uint64
	waitErr  uint64
	count    uint64
	bytes    uint64
	names    []string
}

type m161ParseManifestResult struct {
	findErr  uint64
	allocErr uint64
	waitErr  uint64
	reply    uint64
	replyD0  uint64
	rc       uint32
	manifest [m16LibManifestDescSize]byte
}

func TestIExec_M161_Phase5_DOSParseManifest_KnownPath(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	got := runM161ParseManifestClient(t, client, "C:DIR", true)
	if got.findErr != 0 || got.allocErr != 0 || got.waitErr != 0 || got.reply != 0 || got.rc != 0 {
		t.Fatalf("PARSE_MANIFEST C:DIR errors find=%d alloc=%d wait=%d reply=%d d0=%d rc=%d", got.findErr, got.allocErr, got.waitErr, got.reply, got.replyD0, got.rc)
	}
	if name := cStringFromFixed(got.manifest[16:48]); name != "Dir" {
		t.Fatalf("PARSE_MANIFEST C:DIR name=%q, want Dir", name)
	}
	if got.manifest[8] != byte(parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["IOSM_KIND_COMMAND"]) {
		t.Fatalf("PARSE_MANIFEST C:DIR kind=%d, want command", got.manifest[8])
	}
}

func TestIExec_M161_Phase5_DOSParseManifest_LibsPath(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	got := runM161ParseManifestClient(t, client, "LIBS:dos.library", true)
	if got.findErr != 0 || got.allocErr != 0 || got.waitErr != 0 || got.reply != 0 || got.rc != 0 {
		t.Fatalf("PARSE_MANIFEST LIBS:dos.library errors find=%d alloc=%d wait=%d reply=%d d0=%d rc=%d", got.findErr, got.allocErr, got.waitErr, got.reply, got.replyD0, got.rc)
	}
	want := mustReadM16ManifestDescBytes(t, "sdk/intuitionos/iexec/boot_dos_library.elf")
	if !bytes.Equal(got.manifest[:], want) {
		t.Fatalf("PARSE_MANIFEST LIBS:dos.library manifest mismatch\n got=% x\nwant=% x", got.manifest[:], want)
	}
}

func TestIExec_M161_Phase5_DOSParseManifest_NotFoundAndNoNUL(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	vals := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))

	got := runM161ParseManifestClient(t, client, "C:NOPE", true)
	if got.rc != uint32(vals["ERR_NOTFOUND"]) {
		t.Fatalf("PARSE_MANIFEST C:NOPE rc=%d, want ERR_NOTFOUND", got.rc)
	}
}

func TestIExec_M161_Phase5_DOSParseManifest_BoundsSectionName(t *testing.T) {
	body := string(mustReadRepoBytes(t, filepath.Join("sdk", "intuitionos", "iexec", "lib", "dos_library.s")))
	for _, want := range []string{
		"load.l  r11, (r27)                   ; sh_name",
		"bge     r11, r30, .dpcife_next_sh",
		"add     r12, r12, #14                 ; len(\".ios.manifest\\0\")",
		"bgt     r12, r30, .dpcife_next_sh",
		"jsr     .dos_pmp_is_iosm_section_name",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dos PARSE_MANIFEST section-name bounds check missing %q", want)
		}
	}
}

func runM161ParseManifestClient(t *testing.T, client *m161Task0ClientRig, path string, nulTerminate bool) m161ParseManifestResult {
	t.Helper()

	const (
		offName     = 0x500
		offPortID   = 0x540
		offFindErr  = 0x548
		offReplyPrt = 0x550
		offBufferVA = 0x558
		offAllocErr = 0x560
		offShareHdl = 0x568
		offReply    = 0x570
		offWaitErr  = 0x578
		offSentinel = 0x580
		offReplyD0  = 0x588
	)

	mem := client.rig.cpu.memory
	stateBase := uint32(client.usp) - MMU_PAGE_SIZE
	clear(mem[stateBase+offName : stateBase+offName+0x180])
	copy(mem[stateBase+offName:], []byte("dos.library\x00"))

	pc := client.t0
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0+MMU_PAGE_SIZE {
			t.Fatalf("PARSE_MANIFEST client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	vals := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))
	opParseManifest := vals["DOS_OP_PARSE_MANIFEST"]

	w(ie64Instr(OP_SUB, 31, IE64_SIZE_L, 1, 31, 0, 16))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, stateBase))
	w(ie64Instr(OP_STORE, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, stateBase+offName))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFindPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offPortID))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offFindErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysCreatePort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offReplyPrt))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 4096))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10001))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offShareHdl))

	for i := 0; i < len(path); i++ {
		w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
		w(ie64Instr(OP_LOAD, 28, IE64_SIZE_Q, 0, 29, 0, offBufferVA))
		w(ie64Instr(OP_MOVE, 27, IE64_SIZE_L, 1, 0, 0, uint32(path[i])))
		w(ie64Instr(OP_STORE, 27, IE64_SIZE_B, 1, 28, 0, uint32(i)))
	}
	if nulTerminate {
		w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
		w(ie64Instr(OP_LOAD, 28, IE64_SIZE_Q, 0, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 0, IE64_SIZE_B, 1, 28, 0, uint32(len(path))))
	}

	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offPortID))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, opParseManifest))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 0, 29, 0, offShareHdl))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	yieldPC := pc
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldPC)-int32(pc))))

	resetM161Task0ClientState(client)
	parkM161TasksExcept(client, "dos.library")
	runRigForDuration(client.rig, 1200*time.Millisecond)

	var res m161ParseManifestResult
	res.findErr = binary.LittleEndian.Uint64(mem[stateBase+offFindErr:])
	res.allocErr = binary.LittleEndian.Uint64(mem[stateBase+offAllocErr:])
	res.reply = binary.LittleEndian.Uint64(mem[stateBase+offReply:])
	res.replyD0 = binary.LittleEndian.Uint64(mem[stateBase+offReplyD0:])
	res.waitErr = binary.LittleEndian.Uint64(mem[stateBase+offWaitErr:])
	bufVA := uint32(binary.LittleEndian.Uint64(mem[stateBase+offBufferVA:]))
	bufPhys, ok := taskVAToPhys(mem, client.pub, uint64(bufVA))
	if !ok {
		t.Fatalf("runM161ParseManifestClient(%q): buffer VA %#x is not mapped", path, bufVA)
	}
	res.rc = binary.LittleEndian.Uint32(mem[bufPhys+uint32(vals["DOS_PMP_RC_OFF"]):])
	copy(res.manifest[:], mem[bufPhys+uint32(vals["DOS_PMP_IOSM_OFF"]):bufPhys+uint32(vals["DOS_PMP_IOSM_OFF"])+m16LibManifestDescSize])
	return res
}

func parkM161TasksExcept(client *m161Task0ClientRig, servicePort string) {
	mem := client.rig.cpu.memory
	_, portBase, ok := findPublicPortIDByName(mem, servicePort)
	serviceSlot := uint32(maxTasks)
	if ok {
		serviceSlot = uint32(mem[portBase+kdPortOwner])
	}
	for slot := uint32(0); slot < maxTasks; slot++ {
		if slot == client.slot || slot == serviceSlot {
			continue
		}
		tcb := kernDataBase + kdTCBBase + slot*tcbStride
		if mem[tcb+tcbStateOff] == taskFree {
			continue
		}
		mem[tcb+tcbStateOff] = taskWaiting
	}
}

func TestIExec_M161_Phase5_ListResidents_AllBaselinePresent(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	got := runM161ListResidentsClient(t, client)
	if got.findErr != 0 || got.allocErr != 0 || got.waitErr != 0 {
		t.Fatalf("LIST_RESIDENTS errors find=%d alloc=%d wait=%d", got.findErr, got.allocErr, got.waitErr)
	}
	want := []string{
		"exec.library",
		"console.handler",
		"dos.library",
		"hardware.resource",
		"input.device",
		"graphics.library",
	}
	if got.count != uint64(len(want)) {
		t.Fatalf("LIST_RESIDENTS count=%d names=%v, want %d", got.count, got.names, len(want))
	}
	if got.bytes != uint64((len(want)+1)*32) {
		t.Fatalf("LIST_RESIDENTS bytes=%d, want %d", got.bytes, (len(want)+1)*32)
	}
	if len(got.names) != len(want) {
		t.Fatalf("LIST_RESIDENTS names=%v, want %v", got.names, want)
	}
	for i := range want {
		if got.names[i] != want[i] {
			t.Fatalf("LIST_RESIDENTS name[%d]=%q, want %q (all=%v)", i, got.names[i], want[i], got.names)
		}
	}
}

func runM161ListResidentsClient(t *testing.T, client *m161Task0ClientRig) m161ListResidentsResult {
	t.Helper()

	const (
		offName     = 0x400
		offPortID   = 0x440
		offFindErr  = 0x448
		offReplyPrt = 0x450
		offBufferVA = 0x458
		offAllocErr = 0x460
		offShareHdl = 0x468
		offCount    = 0x470
		offBytes    = 0x478
		offWaitErr  = 0x480
		offSentinel = 0x488
	)

	mem := client.rig.cpu.memory
	clear(mem[client.data+offName : client.data+offName+0x500])
	copy(mem[client.data+offName:], []byte("exec.library\x00"))

	pc := client.t0
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0+MMU_PAGE_SIZE {
			t.Fatalf("LIST_RESIDENTS client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	msgListResidents := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["MSG_LIST_RESIDENTS"]

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
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 4096))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10001))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offPortID))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, msgListResidents))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 0, 29, 0, offShareHdl))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offCount))
	w(ie64Instr(OP_STORE, 4, IE64_SIZE_Q, 1, 29, 0, offBytes))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offWaitErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offSentinel))
	w(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	resetM161Task0ClientState(client)
	runRigForDuration(client.rig, 300*time.Millisecond)

	if got := binary.LittleEndian.Uint64(mem[client.data+offSentinel:]); got != 0xCAFE {
		output := client.term.DrainOutput()
		t.Fatalf("runM161ListResidentsClient: sentinel=%#x findErr=%d allocErr=%d waitErr=%d output=%q",
			got,
			binary.LittleEndian.Uint64(mem[client.data+offFindErr:]),
			binary.LittleEndian.Uint64(mem[client.data+offAllocErr:]),
			binary.LittleEndian.Uint64(mem[client.data+offWaitErr:]),
			output[:min(len(output), 800)])
	}

	var res m161ListResidentsResult
	res.findErr = binary.LittleEndian.Uint64(mem[client.data+offFindErr:])
	res.allocErr = binary.LittleEndian.Uint64(mem[client.data+offAllocErr:])
	res.waitErr = binary.LittleEndian.Uint64(mem[client.data+offWaitErr:])
	res.count = binary.LittleEndian.Uint64(mem[client.data+offCount:])
	res.bytes = binary.LittleEndian.Uint64(mem[client.data+offBytes:])
	bufVA := uint32(binary.LittleEndian.Uint64(mem[client.data+offBufferVA:]))
	bufPhys, ok := taskVAToPhys(mem, client.pub, uint64(bufVA))
	if !ok {
		t.Fatalf("runM161ListResidentsClient: buffer VA %#x is not mapped", bufVA)
	}
	manifestCount := binary.LittleEndian.Uint32(mem[bufPhys:])
	for i := uint32(0); i < manifestCount && i < 127; i++ {
		off := bufPhys + 32 + i*32
		res.names = append(res.names, cStringFromFixed(mem[off:off+32]))
	}
	return res
}

func runM161PortNameByIndexClient(t *testing.T, client *m161Task0ClientRig, index uint32, goodBuffer bool) m161PortNameByIndexResult {
	t.Helper()

	const (
		offName     = 0x300
		offTaskID   = 0x328
		offErr      = 0x330
		offSentinel = 0x338
	)

	mem := client.rig.cpu.memory
	clear(mem[client.data+offName : client.data+offName+0x40])

	pc := client.t0
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0+MMU_PAGE_SIZE {
			t.Fatalf("PORT_NAME_BY_INDEX client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	sysinfoPortNameByIndex := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["SYSINFO_PORT_NAME_BY_INDEX"]

	w(ie64Instr(OP_SUB, 31, IE64_SIZE_L, 1, 31, 0, 16))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, client.data))
	w(ie64Instr(OP_STORE, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, sysinfoPortNameByIndex))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, index))
	if goodBuffer {
		w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, client.data+offName))
	} else {
		w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0x1ff0000))
	}
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysGetSysInfo))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offTaskID))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offSentinel))
	w(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	resetM161Task0ClientState(client)
	runRigForDuration(client.rig, 300*time.Millisecond)

	if got := binary.LittleEndian.Uint64(mem[client.data+offSentinel:]); got != 0xCAFE {
		output := client.term.DrainOutput()
		t.Fatalf("runM161PortNameByIndexClient(%d): sentinel=%#x err=%d taskID=%d output=%q",
			index,
			got,
			binary.LittleEndian.Uint64(mem[client.data+offErr:]),
			binary.LittleEndian.Uint64(mem[client.data+offTaskID:]),
			output[:min(len(output), 800)])
	}

	var res m161PortNameByIndexResult
	res.taskID = binary.LittleEndian.Uint64(mem[client.data+offTaskID:])
	res.err = binary.LittleEndian.Uint64(mem[client.data+offErr:])
	copy(res.name[:], mem[client.data+offName:client.data+offName+32])
	return res
}

func cStringFromFixed(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
