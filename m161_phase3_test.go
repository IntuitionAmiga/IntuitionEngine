package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type m161Task0ClientRig struct {
	rig  *ie64TestRig
	term *TerminalMMIO

	t0    uint32
	t0p   uint32
	data  uint32
	datap uint32
	pt    uint32
	usp   uint64
	slot  uint32
	pub   uint64
}

func TestIExec_M161_Phase3_GetIOSM_PersistentModules(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	for _, tc := range []struct {
		name    string
		port    string
		elfPath string
	}{
		{name: "console", port: "console.handler", elfPath: "sdk/intuitionos/iexec/boot_console_handler.elf"},
		{name: "dos", port: "dos.library", elfPath: "sdk/intuitionos/iexec/boot_dos_library.elf"},
		{name: "hardware", port: "hardware.resource", elfPath: "sdk/intuitionos/iexec/boot_hardware_resource.elf"},
		{name: "input", port: "input.device", elfPath: "sdk/intuitionos/iexec/boot_input_device.elf"},
		{name: "graphics", port: "graphics.library", elfPath: "sdk/intuitionos/iexec/boot_graphics_library.elf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runM161GetIOSMClient(t, client, tc.port, true)
			if got.findErr != 0 {
				t.Fatalf("FindPort(%q) err=%d, want 0", tc.port, got.findErr)
			}
			if got.allocErr != 0 {
				t.Fatalf("AllocMem err=%d, want 0", got.allocErr)
			}
			if got.replyErr != 0 {
				t.Fatalf("reply err=%d, want 0", got.replyErr)
			}
			if got.waitErr != 0 {
				t.Fatalf("WaitPort err=%d, want 0", got.waitErr)
			}

			want := mustReadM16ManifestDescBytes(t, tc.elfPath)
			if !bytes.Equal(got.manifest[:], want) {
				t.Fatalf("manifest bytes mismatch for %s\n got=% x\nwant=% x", tc.port, got.manifest[:], want)
			}
		})
	}
}

func TestIExec_M161_Phase3_GetIOSM_ZeroSharedHandle(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	errBadArg := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["ERR_BADARG"]

	for _, tc := range []struct {
		name string
		port string
	}{
		{name: "console", port: "console.handler"},
		{name: "dos", port: "dos.library"},
		{name: "hardware", port: "hardware.resource"},
		{name: "input", port: "input.device"},
		{name: "graphics", port: "graphics.library"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runM161GetIOSMClient(t, client, tc.port, false)
			if got.findErr != 0 {
				t.Fatalf("FindPort(%q) err=%d, want 0", tc.port, got.findErr)
			}
			if got.replyErr != uint64(errBadArg) {
				t.Fatalf("reply err=%d, want ERR_BADARG (%d)", got.replyErr, errBadArg)
			}
			if got.waitErr != 0 {
				t.Fatalf("WaitPort err=%d, want 0", got.waitErr)
			}
		})
	}
}

type m161GetIOSMResult struct {
	findErr  uint64
	allocErr uint64
	waitErr  uint64
	replyErr uint64
	manifest [m16LibManifestDescSize]byte
}

func bootM161Phase3Task0ClientRig(t *testing.T) *m161Task0ClientRig {
	t.Helper()

	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	writeHostRootFileBytes(t, hostRoot, "S/Startup-Sequence", []byte(
		"RESOURCES/hardware.resource\r\n"+
			"DEVS/input.device\r\n"+
			"LIBS:graphics.library\r\n"+
			"ECHO M161-PHASE3\r\n",
	))

	rig, term, shellCode, shellData := bootAndResetToShellTaskWithBootstrapHostRoot(t, hostRoot)
	for _, want := range []string{
		"console.handler",
		"dos.library",
		"hardware.resource",
		"input.device",
		"graphics.library",
	} {
		if _, _, ok := findPublicPortIDByName(rig.cpu.memory, want); !ok {
			output := term.DrainOutput()
			t.Fatalf("bootM161Phase3Task0ClientRig: missing public port %q output=%q", want, output[:min(len(output), 1200)])
		}
	}

	layout, ok := tryFindShellTaskLayout(rig.cpu.memory)
	if !ok || layout.codeVA != shellCode || layout.dataVA != shellData {
		t.Fatalf("bootM161Phase3Task0ClientRig: could not locate live shell layout code=%#x data=%#x", shellCode, shellData)
	}
	usp0 := m161InstallClientScratch(t, layout)
	pt0 := layout.pt
	if shellCode == 0 || shellData == 0 || pt0 == 0 || usp0 == 0 {
		t.Fatalf("bootM161Phase3Task0ClientRig: missing shell layout code=%#x data=%#x pt=%#x usp=%#x", shellCode, shellData, pt0, usp0)
	}

	return &m161Task0ClientRig{
		rig:   rig,
		term:  term,
		t0:    shellCode,
		t0p:   layout.codePhys,
		data:  shellData,
		datap: layout.dataPhys,
		pt:    pt0,
		usp:   usp0,
		slot:  layout.slot,
		pub:   uint64(layout.pubid),
	}
}

func m161InstallClientScratch(t *testing.T, layout shellTaskLayout) uint64 {
	t.Helper()
	if layout.codeVA == 0 || layout.codePhys == 0 || layout.dataVA == 0 || layout.dataPhys == 0 || layout.pt == 0 || layout.usp == 0 {
		t.Fatalf("m161InstallClientScratch: incomplete shell layout: %+v", layout)
	}
	return layout.usp
}

func runM161GetIOSMClient(t *testing.T, client *m161Task0ClientRig, portName string, useShare bool) m161GetIOSMResult {
	t.Helper()

	const (
		offName     = 0x200
		offPortID   = 0x240
		offFindErr  = 0x248
		offReplyPrt = 0x250
		offBufferVA = 0x258
		offAllocErr = 0x260
		offShareHdl = 0x268
		offReplyErr = 0x270
		offWaitErr  = 0x278
		offSentinel = 0x280
	)

	mem := client.rig.cpu.memory
	clear(mem[client.datap+offName : client.datap+offName+0x180])
	copy(mem[client.datap+offName:], append([]byte(portName), 0))

	pc := client.t0p
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0p+MMU_PAGE_SIZE {
			t.Fatalf("GET_IOSM client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	msgGetIOSM := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["MSG_GET_IOSM"]

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

	if useShare {
		w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, m16LibManifestDescSize))
		w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10001))
		w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
		w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
		w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	} else {
		w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offBufferVA))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offAllocErr))
		w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offShareHdl))
	}

	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offPortID))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, msgGetIOSM))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 0, 29, 0, offShareHdl))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offReplyErr))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offWaitErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offSentinel))
	w(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	resetM161Task0ClientState(client)
	runRigForDuration(t, client.rig, 300*time.Millisecond)

	if got := binary.LittleEndian.Uint64(mem[client.datap+offSentinel:]); got != 0xCAFE {
		output := client.term.DrainOutput()
		t.Fatalf("runM161GetIOSMClient(%q): sentinel=%#x, want 0xCAFE findErr=%d allocErr=%d replyErr=%d waitErr=%d bufferVA=%#x share=%#x output=%q",
			portName,
			got,
			binary.LittleEndian.Uint64(mem[client.datap+offFindErr:]),
			binary.LittleEndian.Uint64(mem[client.datap+offAllocErr:]),
			binary.LittleEndian.Uint64(mem[client.datap+offReplyErr:]),
			binary.LittleEndian.Uint64(mem[client.datap+offWaitErr:]),
			binary.LittleEndian.Uint64(mem[client.datap+offBufferVA:]),
			binary.LittleEndian.Uint64(mem[client.datap+offShareHdl:]),
			output[:min(len(output), 800)])
	}

	var res m161GetIOSMResult
	res.findErr = binary.LittleEndian.Uint64(mem[client.datap+offFindErr:])
	res.allocErr = binary.LittleEndian.Uint64(mem[client.datap+offAllocErr:])
	res.replyErr = binary.LittleEndian.Uint64(mem[client.datap+offReplyErr:])
	res.waitErr = binary.LittleEndian.Uint64(mem[client.datap+offWaitErr:])
	if useShare {
		bufVA := uint32(binary.LittleEndian.Uint64(mem[client.datap+offBufferVA:]))
		bufPhys, ok := taskVAToPhys(mem, client.pub, uint64(bufVA))
		if !ok {
			t.Fatalf("runM161GetIOSMClient(%q): buffer VA %#x is not mapped", portName, bufVA)
		}
		copy(res.manifest[:], mem[bufPhys:bufPhys+m16LibManifestDescSize])
	}
	return res
}

func resetM161Task0ClientState(client *m161Task0ClientRig) {
	mem := client.rig.cpu.memory
	tcb := kernDataBase + kdTCBBase + client.slot*tcbStride
	binary.LittleEndian.PutUint64(mem[kernDataBase+kdCurrentTask:], uint64(client.slot))
	binary.LittleEndian.PutUint64(mem[tcb+tcbPCOff:], uint64(client.t0))
	binary.LittleEndian.PutUint64(mem[tcb+tcbUSPOff:], client.usp)
	binary.LittleEndian.PutUint32(mem[tcb+tcbSigWaitOff:], 0)
	binary.LittleEndian.PutUint32(mem[tcb+tcbSigRecvOff:], 0)
	binary.LittleEndian.PutUint64(mem[kernDataBase+kdPTBRBase+client.slot*8:], uint64(client.pt))
	binary.LittleEndian.PutUint64(mem[kernDataBase+kdTaskLayoutBase+client.slot*kdTaskLayoutStr+kdTaskLayoutPT:], uint64(client.pt))
	client.rig.cpu.memory[tcb+tcbStateOff] = taskRunning
	client.rig.cpu.memory[tcb+tcbGPRSavedOff] = 0
	client.rig.cpu.PC = uint64(client.t0)
	client.rig.cpu.regs[31] = client.usp
	client.rig.cpu.userSP = client.usp
	client.rig.cpu.kernelSP = kernStackTop
	client.rig.cpu.ptbr = uint64(client.pt)
	client.rig.cpu.supervisorMode = false
	client.rig.cpu.previousMode = false
	client.rig.cpu.trapped = false
	client.rig.cpu.faultCause = 0
	client.rig.cpu.faultAddr = 0
	client.rig.cpu.faultPC = 0
	client.rig.cpu.timerEnabled.Store(false)
	client.rig.cpu.interruptEnabled.Store(false)
	client.rig.cpu.timerCount.Store(0)
}

func mustReadM16ManifestDescBytes(t *testing.T, rel string) []byte {
	t.Helper()

	image := mustReadRepoBytes(t, rel)
	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	sec := f.Section(m16LibManifestSectionName)
	if sec == nil {
		t.Fatalf("%s missing %s", rel, m16LibManifestSectionName)
	}
	data, err := sec.Data()
	if err != nil {
		t.Fatalf("read %s manifest: %v", rel, err)
	}
	if len(data) < 12 {
		t.Fatalf("%s manifest note too small", rel)
	}
	namesz := binary.LittleEndian.Uint32(data[0:4])
	descsz := binary.LittleEndian.Uint32(data[4:8])
	nameEnd := 12 + int((namesz+3)&^3)
	descEnd := nameEnd + int(descsz)
	if descsz != m16LibManifestDescSize || descEnd > len(data) {
		t.Fatalf("%s manifest descsz=%d descEnd=%d len=%d", rel, descsz, descEnd, len(data))
	}
	return append([]byte(nil), data[nameEnd:descEnd]...)
}

func TestIExec_M161_Phase3_GetIOSM_ManifestFixturesUseIOSMWireFormat(t *testing.T) {
	for _, rel := range []string{
		"sdk/intuitionos/iexec/boot_console_handler.elf",
		"sdk/intuitionos/iexec/boot_dos_library.elf",
		"sdk/intuitionos/iexec/boot_shell.elf",
		"sdk/intuitionos/iexec/boot_hardware_resource.elf",
		"sdk/intuitionos/iexec/boot_input_device.elf",
		"sdk/intuitionos/iexec/boot_graphics_library.elf",
	} {
		t.Run(filepath.Base(rel), func(t *testing.T) {
			desc := mustReadM16ManifestDescBytes(t, rel)
			if got := binary.LittleEndian.Uint32(desc[0:4]); got != m16LibManifestMagic {
				t.Fatalf("%s magic=%#x, want %#x", rel, got, m16LibManifestMagic)
			}
			if got := binary.LittleEndian.Uint32(desc[4:8]); got != 1 {
				t.Fatalf("%s schema=%d, want 1", rel, got)
			}
			if len(desc) != m16LibManifestDescSize {
				t.Fatalf("%s desc len=%d, want %d", rel, len(desc), m16LibManifestDescSize)
			}
		})
	}
}

func TestIExec_M161_Phase3_HelperSanity(t *testing.T) {
	if got := fmt.Sprintf("%d", m16LibManifestDescSize); got != "128" {
		t.Fatalf("unexpected manifest desc size helper=%s", got)
	}
}

func TestIExec_M161_Phase3_GetIOSM_DOSUnmapsSharedBufferBeforeReply(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	body := sliceBetween(t, src, ".dos_do_get_iosm:", ".dos_do_dir:")
	if !strings.Contains(body, "syscall #SYS_FREE_MEM") {
		t.Fatalf("DOS GET_IOSM success path must unmap the shared buffer before replying")
	}
	if !strings.Contains(body, ".dos_get_iosm_badarg_free:") ||
		!strings.Contains(body, "lsl     r2, r2, #12") {
		t.Fatalf("DOS GET_IOSM oversized-share reject must free share_pages-sized mapping")
	}
}

func TestIExec_M161_Phase3_GetIOSM_DOSUsesGenericBadArg(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	body := sliceBetween(t, src, ".dos_do_get_iosm:", ".dos_do_dir:")
	reply := sliceBetween(t, src, ".dos_get_iosm_reply_badarg:", ".dos_do_dir:")
	if !strings.Contains(body, "beqz    r21, .dos_get_iosm_reply_badarg") {
		t.Fatalf("DOS GET_IOSM missing-share path must use generic GET_IOSM badarg reply")
	}
	if !strings.Contains(body, "bra     .dos_get_iosm_reply_badarg") {
		t.Fatalf("DOS GET_IOSM oversized-share path must use generic GET_IOSM badarg reply")
	}
	if !strings.Contains(reply, "move.l  r2, #ERR_BADARG") {
		t.Fatalf("DOS GET_IOSM badarg reply must return ERR_BADARG")
	}
	if strings.Contains(reply, "DOS_ERR_BADARG") {
		t.Fatalf("DOS GET_IOSM badarg reply must not return DOS_ERR_BADARG")
	}
}

func TestIExec_M161_Phase3_GetIOSM_IntuitionUsesGenericBadArg(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/intuition_library.s")
	body := sliceBetween(t, src, ".intui_do_get_iosm:", ".intui_poll_input:")
	reply := sliceBetween(t, src, ".intui_get_iosm_reply_badarg:", ".intui_get_iosm_maperr:")
	if !strings.Contains(body, "beqz    r14, .intui_get_iosm_reply_badarg") {
		t.Fatalf("intuition GET_IOSM missing-share path must use generic GET_IOSM badarg reply")
	}
	if !strings.Contains(body, "bra     .intui_get_iosm_reply_badarg") {
		t.Fatalf("intuition GET_IOSM oversized-share path must use generic GET_IOSM badarg reply")
	}
	if !strings.Contains(reply, "move.l  r2, #ERR_BADARG") {
		t.Fatalf("intuition GET_IOSM badarg reply must return ERR_BADARG")
	}
	if strings.Contains(reply, "INTUI_ERR_BADARG") {
		t.Fatalf("intuition GET_IOSM badarg reply must not return INTUI_ERR_BADARG")
	}
}

func TestIExec_M161_Phase3_GetIOSM_OversizedRejectFreesMappedSize(t *testing.T) {
	for _, tc := range []struct {
		path  string
		label string
	}{
		{path: "sdk/intuitionos/iexec/handler/console_handler.s", label: ".con_get_iosm_badarg_free:"},
		{path: "sdk/intuitionos/iexec/dev/input_device.s", label: ".idev_get_iosm_badarg_free:"},
		{path: "sdk/intuitionos/iexec/resource/hardware_resource.s", label: ".hwres_get_iosm_badarg_free:"},
		{path: "sdk/intuitionos/iexec/lib/graphics_library.s", label: ".gfx_get_iosm_badarg_free:"},
		{path: "sdk/intuitionos/iexec/lib/intuition_library.s", label: ".intui_get_iosm_badarg_free:"},
	} {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			src := mustReadRepoFile(t, tc.path)
			body := sliceBetween(t, src, tc.label, "syscall #SYS_FREE_MEM")
			if !strings.Contains(body, "move.q  r2, r3") || !strings.Contains(body, "lsl     r2, r2, #12") {
				t.Fatalf("%s oversized GET_IOSM cleanup must free share_pages<<12, body=%q", tc.path, body)
			}
		})
	}
}

func sliceBetween(t *testing.T, src string, start string, end string) string {
	t.Helper()
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("missing start marker %q", start)
	}
	j := strings.Index(src[i:], end)
	if j < 0 {
		t.Fatalf("missing end marker %q after %q", end, start)
	}
	return src[i : i+j+len(end)]
}
