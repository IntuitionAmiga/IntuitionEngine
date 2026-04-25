package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const m163ModfASLRCapable = 0x00000004

func TestIExec_M163_FlagContractAndDocs(t *testing.T) {
	vals := parseIncConstants(t, "sdk/include/iexec.inc")
	if got := vals["MODF_RESIDENT"]; got != 0x00000001 {
		t.Fatalf("MODF_RESIDENT=%#x, want %#x", got, uint32(0x00000001))
	}
	if got := vals["MODF_COMPAT_PORT"]; got != 0x00000002 {
		t.Fatalf("MODF_COMPAT_PORT=%#x, want %#x", got, uint32(0x00000002))
	}
	if got := vals["MODF_ASLR_CAPABLE"]; got != m163ModfASLRCapable {
		t.Fatalf("MODF_ASLR_CAPABLE=%#x, want %#x", got, uint32(m163ModfASLRCapable))
	}

	for _, rel := range []string{
		"sdk/docs/IntuitionOS/M16.3-plan.md",
		"sdk/docs/IntuitionOS/ELF.md",
		"sdk/docs/IntuitionOS/Toolchain.md",
		"sdk/docs/IntuitionOS/IExec.md",
	} {
		body := mustReadRepoFile(t, rel)
		for _, fragment := range []string{
			"M16.3 makes `MODF_ASLR_CAPABLE` mandatory for all DOS-loaded ELFs",
			"M16.4",
			"relocation and ASLR",
		} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s missing M16.3 doc fragment %q", rel, fragment)
			}
		}
	}
}

func TestIExec_M163_ShippedRuntimeELFsHaveExactASLRFlagMasks(t *testing.T) {
	targets := m163RuntimeELFTargets(t)
	if len(targets) != 20 {
		t.Fatalf("runtime ELF targets=%d, want 20", len(targets))
	}
	for _, target := range targets {
		manifest := m163ParseManifestFromFile(t, "sdk/intuitionos/iexec/"+target.elfName)
		if manifest.Magic != m16LibManifestMagic || manifest.DescVersion != 1 {
			t.Fatalf("%s bad IOSM magic/schema: magic=%#x schema=%d", target.elfName, manifest.Magic, manifest.DescVersion)
		}
		want := uint32(m163ModfASLRCapable)
		switch manifest.Kind {
		case 1, 2, 3, 4:
			want = m16ModfCompatPort | m163ModfASLRCapable
		case 5:
			want = m163ModfASLRCapable
		default:
			t.Fatalf("%s unknown IOSM kind=%d", target.elfName, manifest.Kind)
		}
		if manifest.Flags != want {
			t.Fatalf("%s %s kind=%d flags=%#x, want exact %#x", target.elfName, manifest.Name, manifest.Kind, manifest.Flags, want)
		}
		if manifest.Patch != 0 {
			t.Fatalf("%s patch=%d, want 0", target.elfName, manifest.Patch)
		}
	}
}

func TestIExec_M163_DOSAndProtectedModulePathsValidateASLRManifest(t *testing.T) {
	dos := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	for _, fragment := range []string{
		"jsr     .dos_validate_iosm_aslr_contract",
		"jsr     .dos_validate_loaded_elf_aslr_contract",
		"move.l  r12, #MODF_ASLR_CAPABLE",
		"move.l  r12, #(MODF_COMPAT_PORT | MODF_ASLR_CAPABLE)",
	} {
		if !strings.Contains(dos, fragment) {
			t.Fatalf("dos_library.s missing M16.3 enforcement fragment %q", fragment)
		}
	}

	root := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	for _, fragment := range []string{
		"move.l  r12, #(MODF_COMPAT_PORT | MODF_ASLR_CAPABLE)",
		"store.l r11, KD_MODULE_FLAGS(r21)",
		"MODF_RESIDENT | MODF_COMPAT_PORT | MODF_ASLR_CAPABLE",
	} {
		if !strings.Contains(root, fragment) {
			t.Fatalf("iexec.s missing M16.3 protected-module fragment %q", fragment)
		}
	}
}

func TestIExec_M163_HostFSLoadSegRejectsInvalidASLRManifestAsBadArg(t *testing.T) {
	valid := mustReadRepoBytes(t, "sdk/intuitionos/iexec/elfseg_fixture.elf")
	missingIOSM := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{
			Vaddr:  0x00601000,
			Flags:  m14ELFSegFlagR | m14ELFSegFlagX,
			Data:   []byte{0x11, 0x22, 0x33, 0x44},
			Memsz:  0x1000,
			Offset: 0x1000,
		},
		{
			Vaddr:  0x00602000,
			Flags:  m14ELFSegFlagR | m14ELFSegFlagW,
			Data:   []byte{0x55, 0x66, 0x77, 0x88},
			Memsz:  0x1000,
			Offset: 0x2000,
		},
	})

	for _, tc := range []struct {
		name  string
		image []byte
	}{
		{name: "cleared ASLR flag", image: patchM163ManifestFlags(t, valid, 0)},
		{name: "missing IOSM", image: missingIOSM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig, dataBase := m163RunLoadSegHostFixture(t, tc.image)
			got := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+200:])
			if got != dosErrBadArg {
				t.Fatalf("DOS_LOADSEG reply.type=%d, want DOS_ERR_BADARG (%d)", got, dosErrBadArg)
			}
			if seglist := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+208:]); seglist != 0 {
				t.Fatalf("DOS_LOADSEG returned seglist VA 0x%X for invalid HostFS ELF", seglist)
			}
		})
	}
}

func TestIExec_M163_HostFSDOSRunRejectsInvalidASLRManifestAsBadArg(t *testing.T) {
	valid := mustReadRepoBytes(t, "sdk/intuitionos/iexec/elfseg_fixture.elf")
	missingIOSM := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{
			Vaddr:  0x00601000,
			Flags:  m14ELFSegFlagR | m14ELFSegFlagX,
			Data:   []byte{0x11, 0x22, 0x33, 0x44},
			Memsz:  0x1000,
			Offset: 0x1000,
		},
		{
			Vaddr:  0x00602000,
			Flags:  m14ELFSegFlagR | m14ELFSegFlagW,
			Data:   []byte{0x55, 0x66, 0x77, 0x88},
			Memsz:  0x1000,
			Offset: 0x2000,
		},
	})

	for _, tc := range []struct {
		name  string
		image []byte
	}{
		{name: "cleared ASLR flag", image: patchM163ManifestFlags(t, valid, 0)},
		{name: "missing IOSM", image: missingIOSM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig, term, dataBase := m163RunDosRunHostFixture(t, tc.image, "C/ElfSeg", "")
			got := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+200:])
			if got != dosErrBadArg {
				t.Fatalf("DOS_RUN reply.type=%d, want DOS_ERR_BADARG (%d), task_id=%d output=%q",
					got,
					dosErrBadArg,
					binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+208:]),
					term.DrainOutput(),
				)
			}
			if taskID := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+208:]); taskID != 0 {
				t.Fatalf("DOS_RUN launched task %d for invalid HostFS ELF", taskID)
			}
		})
	}
}

func TestIExec_M163_MetaDOSRunRejectsInvalidASLRManifestAsBadArg(t *testing.T) {
	rig, term, dataBase := m163RunDosRunRAMMetaFixture(t, m163MinimalELFWithoutIOSM(), "M163Bad")
	got := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+160:])
	if got != dosErrBadArg {
		t.Fatalf("meta-backed DOS_RUN reply.type=%d, want DOS_ERR_BADARG (%d), task_id=%d output=%q",
			got,
			dosErrBadArg,
			binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+168:]),
			term.DrainOutput(),
		)
	}
	if taskID := binary.LittleEndian.Uint64(rig.cpu.memory[dataBase+168:]); taskID != 0 {
		t.Fatalf("meta-backed DOS_RUN launched task %d for invalid RAM/meta ELF", taskID)
	}
}

func TestIExec_M163_BootstrapRejectsDosLibraryWithoutASLRManifestFlag(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	dosImage := mustReadRepoBytes(t, "sdk/intuitionos/iexec/boot_dos_library.elf")
	writeHostRootFileBytes(t, hostRoot, "LIBS/dos.library", patchM163ManifestFlags(t, dosImage, m16ModfCompatPort))

	rig, term := assembleAndLoadKernelWithBootstrapHostRoot(t, hostRoot)
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(2 * time.Second)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.Contains(output, "BOOT FAIL") {
		t.Fatalf("expected BOOT FAIL for dos.library without MODF_ASLR_CAPABLE, output=%q", output[:min(len(output), 400)])
	}
	for _, unwanted := range []string{"dos.library M14 [Task ", "Shell M10 [Task ", "1> "} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("dos.library without ASLR flag still reached %q output=%q", unwanted, output[:min(len(output), 400)])
		}
	}
}

func TestIExec_M163_NoShippedSourceUsesSlotDerivedTaskLocalAddresses(t *testing.T) {
	for _, rel := range m163SourceAuditFiles(t) {
		body := m163StripAsmComments(mustReadRepoFile(t, rel))
		for _, forbidden := range []string{
			"CURRENT_TASK * USER_SLOT_STRIDE",
			"USER_DATA_BASE + task_slot",
			"USER_CODE_BASE + task_slot",
			"USER_STACK_BASE + task_slot",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains forbidden task-local addressing pattern %q", rel, forbidden)
			}
		}
		for _, re := range []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bUSER_(DATA|CODE|STACK)_BASE\b.*\bUSER_SLOT_STRIDE\b`),
			regexp.MustCompile(`(?i)\bCURRENT_TASK\b.*\bUSER_SLOT_STRIDE\b`),
		} {
			if loc := re.FindStringIndex(body); loc != nil {
				t.Fatalf("%s contains forbidden slot-derived task-local expression near %q", rel, body[loc[0]:min(len(body), loc[1]+80)])
			}
		}
	}
}

func m163StripAsmComments(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func m163ParseManifestFromFile(t *testing.T, rel string) *m16LibManifest {
	t.Helper()
	image := mustReadRepoBytes(t, rel)
	manifest, err := parseM16LibManifestNote(image)
	if err != nil {
		t.Fatalf("%s: parse IOSM: %v", rel, err)
	}
	return manifest
}

type m163RuntimeTarget struct {
	elfName string
	label   string
}

func m163RuntimeELFTargets(t *testing.T) []m163RuntimeTarget {
	t.Helper()
	makefile := mustReadRepoFile(t, "Makefile")
	re := regexp.MustCompile(`(?m)^\s+([A-Za-z0-9_./-]+\.elf):(prog_[A-Za-z0-9_]+)\s*\\?$`)
	matches := re.FindAllStringSubmatch(makefile, -1)
	out := make([]m163RuntimeTarget, 0, len(matches))
	for _, match := range matches {
		out = append(out, m163RuntimeTarget{elfName: match[1], label: match[2]})
	}
	return out
}

func m163SourceAuditFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, root := range []string{"sdk/intuitionos/iexec", "sdk/include"} {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", root, err)
		}
		for _, entry := range entries {
			path := root + "/" + entry.Name()
			if entry.IsDir() {
				out = append(out, m163SourceAuditFilesUnder(t, path)...)
				continue
			}
			if strings.HasSuffix(path, ".s") || strings.HasSuffix(path, ".inc") {
				out = append(out, path)
			}
		}
	}
	return out
}

func m163SourceAuditFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", root, err)
	}
	var out []string
	for _, entry := range entries {
		path := root + "/" + entry.Name()
		if entry.IsDir() {
			out = append(out, m163SourceAuditFilesUnder(t, path)...)
			continue
		}
		if strings.HasSuffix(path, ".s") || strings.HasSuffix(path, ".inc") {
			out = append(out, path)
		}
	}
	return out
}

func patchM163ManifestFlags(t *testing.T, image []byte, flags uint32) []byte {
	t.Helper()
	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("parse ELF: %v", err)
	}
	sec := f.Section(m16LibManifestSectionName)
	if sec == nil {
		t.Fatalf("missing %s section", m16LibManifestSectionName)
	}
	patched := append([]byte(nil), image...)
	descOff := int(sec.Offset) + 12 + len(m16LibManifestNoteName)
	binary.LittleEndian.PutUint32(patched[descOff+48:], flags)
	return patched
}

func m163RunLoadSegHostFixture(t *testing.T, image []byte) (*ie64TestRig, uint32) {
	t.Helper()
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	writeHostRootFileBytes(t, hostRoot, "C/ElfSeg", image)
	rig, _ := bootRigWithPatchedHostShellELFOnHostRoot(t, hostRoot, func(image []byte) {})
	return runM14LoadSegClientOnRig(t, rig, "C/ElfSeg", 1, false)
}

func m163RunDosRunHostFixture(t *testing.T, image []byte, command string, args string) (*ie64TestRig, *TerminalMMIO, uint32) {
	t.Helper()
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	writeHostRootFileBytes(t, hostRoot, "C/ElfSeg", image)
	rig, term := bootRigWithPatchedHostShellELFOnHostRoot(t, hostRoot, func(image []byte) {})

	const (
		offDosPort  = 128
		offReplyPrt = 136
		offBufferVA = 144
		offShareHdl = 152
		offRunType  = 200
		offTaskID   = 208
	)

	off := findShellClientCodeStart(t, rig.cpu.memory)
	w := func(instr []byte) { copy(rig.cpu.memory[off:], instr); off += 8 }

	w(ie64Instr(OP_SUB, 31, IE64_SIZE_L, 1, 31, 0, 16))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 8))
	w(ie64Instr(OP_STORE, 29, IE64_SIZE_Q, 0, 31, 0, 0))

	findLoop := off
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_ADD, 1, IE64_SIZE_L, 1, 29, 0, 16))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFindPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	beqInstr := off
	w(ie64Instr(OP_BEQ, 0, 0, 0, 2, 0, 0))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	braFind := off
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(findLoop)-int32(braFind))))
	foundDos := off
	copy(rig.cpu.memory[beqInstr:], ie64Instr(OP_BEQ, 0, 0, 0, 2, 0, uint32(int32(foundDos)-int32(beqInstr))))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offDosPort))

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
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, offShareHdl))

	w(ie64Instr(OP_LOAD, 4, IE64_SIZE_Q, 0, 29, 0, offBufferVA))
	for i := 0; i < len(command); i++ {
		w(ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(command[i])))
		w(ie64Instr(OP_STORE, 5, IE64_SIZE_B, 0, 4, 0, uint32(i)))
	}
	w(ie64Instr(OP_STORE, 0, IE64_SIZE_B, 0, 4, 0, uint32(len(command))))
	for i := 0; i < len(args); i++ {
		w(ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(args[i])))
		w(ie64Instr(OP_STORE, 5, IE64_SIZE_B, 0, 4, 0, uint32(len(command)+1+i)))
	}
	w(ie64Instr(OP_STORE, 0, IE64_SIZE_B, 0, 4, 0, uint32(len(command)+1+len(args))))

	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offDosPort))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, dosRun))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_L, 0, 29, 0, offShareHdl))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 29, 0, offReplyPrt))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offRunType))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offTaskID))

	yieldLoop := off
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	braYield := off
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldLoop)-int32(braYield))))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()

	time.Sleep(3 * time.Second)
	rig.cpu.running.Store(false)
	<-done

	dataBase := findShellTaskDataBase(t, rig.cpu.memory)
	return rig, term, dataBase
}

func m163RunDosRunRAMMetaFixture(t *testing.T, image []byte, command string) (*ie64TestRig, *TerminalMMIO, uint32) {
	t.Helper()
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	rig, term := bootRigWithPatchedHostShellELFOnHostRoot(t, hostRoot, func(image []byte) {})

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	fileVA, _, ok := waitForDosFileMeta(rig.cpu.memory, "readme", 8*time.Second)
	rig.cpu.running.Store(false)
	<-done
	if !ok {
		t.Fatalf("readme metadata never published, output=%q", term.DrainOutput())
	}

	m163WriteDosExtentChain(t, rig.cpu.memory, fileVA, image)
	m163AddDosFileAlias(t, rig.cpu.memory, "C/M163Bad", fileVA, uint32(len(image)))
	rig, dataBase := runDOSRunClientOnRig(t, rig, append(append([]byte(command), 0), 0))
	return rig, term, dataBase
}

func m163MinimalELFWithoutIOSM() []byte {
	image := make([]byte, 64)
	copy(image, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(image[16:], 2)
	binary.LittleEndian.PutUint16(image[18:], 0x4945)
	binary.LittleEndian.PutUint32(image[20:], 1)
	binary.LittleEndian.PutUint16(image[52:], 64)
	return image
}

func m163AddDosFileAlias(t *testing.T, mem []byte, alias string, fileVA uint64, fileSize uint32) {
	t.Helper()
	const (
		metaHdrSz   = 16
		metaEntrySz = 48
		metaPerPage = 85
	)
	if len(alias) == 0 || len(alias) > 31 {
		t.Fatalf("m163AddDosFileAlias: alias %q length invalid", alias)
	}
	dosData := uint32(taskLayoutFieldQ(mem, 1, kdTaskDataBase))
	metaHead := binary.LittleEndian.Uint64(mem[dosData+152:])
	if metaHead == 0 {
		t.Fatal("m163AddDosFileAlias: meta chain head is 0")
	}
	for page := metaHead; page != 0; {
		pagePhys, ok := taskVAToPhys(mem, 1, page)
		if !ok {
			t.Fatalf("m163AddDosFileAlias: could not translate meta page VA 0x%X", page)
		}
		next := binary.LittleEndian.Uint64(mem[pagePhys:])
		for i := uint32(0); i < metaPerPage; i++ {
			entry := pagePhys + metaHdrSz + i*metaEntrySz
			if mem[entry] != 0 {
				continue
			}
			clear(mem[entry : entry+32])
			copy(mem[entry:], []byte(alias))
			binary.LittleEndian.PutUint64(mem[entry+32:], fileVA)
			binary.LittleEndian.PutUint32(mem[entry+40:], fileSize)
			return
		}
		page = next
	}
	t.Fatalf("m163AddDosFileAlias: no free metadata slot for %q", alias)
}

func m163WriteDosExtentChain(t *testing.T, mem []byte, firstVA uint64, image []byte) {
	t.Helper()
	const (
		dosExtOffNext   = 0
		dosExtHdrSize   = 16
		dosExtPayload   = 4080
		dosTaskPublicID = 1
	)
	remaining := image
	for extentVA := firstVA; extentVA != 0 && len(remaining) > 0; {
		phys, ok := taskVAToPhys(mem, dosTaskPublicID, extentVA)
		if !ok {
			t.Fatalf("could not translate DOS extent VA 0x%X", extentVA)
		}
		n := min(len(remaining), dosExtPayload)
		copy(mem[phys+dosExtHdrSize:phys+dosExtHdrSize+uint32(n)], remaining[:n])
		remaining = remaining[n:]
		extentVA = binary.LittleEndian.Uint64(mem[phys+dosExtOffNext:])
	}
	if len(remaining) != 0 {
		t.Fatalf("DOS extent chain too short by %d bytes", len(remaining))
	}
}
