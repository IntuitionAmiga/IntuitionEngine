package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

func TestIExec_M1641_DocsFreezePTNoteStrippedContract(t *testing.T) {
	for _, rel := range []string{
		"sdk/docs/IntuitionOS/M16.4.1-plan.md",
		"sdk/docs/IntuitionOS/ELF.md",
		"sdk/docs/IntuitionOS/Toolchain.md",
		"sdk/docs/IntuitionOS/IExec.md",
		"sdk/docs/include-files.md",
		"sdk/README.md",
		"IntuitionOS_Roadmap.md",
	} {
		body := mustReadRepoFile(t, rel)
		requireAllSubstrings(t, body,
			"PT_NOTE",
			"stripped",
			"section-header-only",
			"dynamic linking",
			"KASLR",
			"trusted read-only system source",
		)
	}
}

func TestIExec_M1641_RuntimeELFValidatorAcceptsStrippedPTNoteNoRelocations(t *testing.T) {
	image := makeM1641ELFFixture(t, nil, false)
	if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err != nil {
		t.Fatalf("valid stripped PT_NOTE ET_DYN rejected: %v", err)
	}
}

func TestIExec_M1641_RuntimeELFValidatorAcceptsEmptyIOSREL(t *testing.T) {
	image := makeM1641ELFFixture(t, nil, true)
	if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err != nil {
		t.Fatalf("valid stripped PT_NOTE ET_DYN with empty IOS-REL rejected: %v", err)
	}
}

func TestIExec_M1641_RuntimeELFValidatorAppliesPTNoteRelative64Relocation(t *testing.T) {
	image := makeM1641ELFFixture(t, []m164RelaSpec{{Offset: 0x2000, Type: m164RelRelative64, Addend: 0x2000}}, false)
	mapped := make([]byte, 0x3000)
	_, entry, err := m164LoadRuntimeELF(image, m164Placement{Base: 0x00650000}, mapped)
	if err != nil {
		t.Fatalf("valid IOS-REL relative relocation rejected: %v", err)
	}
	if entry != 0x00650000 {
		t.Fatalf("entry=0x%X, want chosen base for e_entry=0", entry)
	}
	if got := binary.LittleEndian.Uint64(mapped[0x2000:]); got != 0x00652000 {
		t.Fatalf("relocated pointer=0x%X, want 0x652000", got)
	}
}

func TestIExec_M1641_RuntimeELFValidatorRejectsSectionHeaderOnlyMetadata(t *testing.T) {
	image := makeM164ELFFixture(t, nil)
	if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err == nil {
		t.Fatal("section-header-only M16.4 image accepted after M16.4.1 cutover")
	}
}

func TestIExec_M1641_RuntimeELFValidatorRejectsForbiddenPTNoteInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func([]byte)
	}{
		{"nonzero e_shoff", func(img []byte) { binary.LittleEndian.PutUint64(img[40:48], 0x4000) }},
		{"nonzero e_shnum", func(img []byte) { binary.LittleEndian.PutUint16(img[60:62], 1) }},
		{"nonzero e_shstrndx", func(img []byte) { binary.LittleEndian.PutUint16(img[62:64], 1) }},
		{"bad ehsize", func(img []byte) { binary.LittleEndian.PutUint16(img[52:54], 63) }},
		{"bad phentsize", func(img []byte) { binary.LittleEndian.PutUint16(img[54:56], 55) }},
		{"program headers out of bounds", func(img []byte) { binary.LittleEndian.PutUint64(img[32:40], uint64(len(img)-8)) }},
		{"bad load align", func(img []byte) { binary.LittleEndian.PutUint64(img[64+48:64+56], 8) }},
		{"bad vaddr alignment", func(img []byte) { binary.LittleEndian.PutUint64(img[64+56+16:64+56+24], 0x2001) }},
		{"bad offset congruence", func(img []byte) { binary.LittleEndian.PutUint64(img[64+56+8:64+56+16], 0x2100) }},
		{"filesz exceeds memsz", func(img []byte) { binary.LittleEndian.PutUint64(img[64+56+32:64+56+40], 0x1001) }},
		{"zero memsz", func(img []byte) { binary.LittleEndian.PutUint64(img[64+56+40:64+56+48], 0) }},
		{"load file range out of bounds", func(img []byte) { binary.LittleEndian.PutUint64(img[64+56+8:64+56+16], uint64(len(img)-4)) }},
		{"note file range out of bounds", func(img []byte) { binary.LittleEndian.PutUint64(img[64+112+32:64+112+40], uint64(len(img))) }},
		{"multiple PT_NOTE", func(img []byte) {
			binary.LittleEndian.PutUint16(img[56:58], 4)
			copy(img[64+168:64+224], img[64+112:64+168])
		}},
		{"unsupported PT_INTERP", func(img []byte) { binary.LittleEndian.PutUint32(img[64:68], m14ELFPTInterp) }},
		{"unsupported PT_DYNAMIC", func(img []byte) { binary.LittleEndian.PutUint32(img[64:68], m14ELFPTDynamic) }},
		{"unsupported PT_TLS", func(img []byte) { binary.LittleEndian.PutUint32(img[64:68], m14ELFPTTLS) }},
		{"unknown segment flags", func(img []byte) { binary.LittleEndian.PutUint32(img[64+4:64+8], 0x80) }},
		{"zero permissions", func(img []byte) { binary.LittleEndian.PutUint32(img[64+4:64+8], 0) }},
		{"write-only", func(img []byte) { binary.LittleEndian.PutUint32(img[64+56+4:64+56+8], m14ELFSegFlagW) }},
		{"W|X", func(img []byte) {
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

func TestIExec_M1641_RuntimeELFValidatorRejectsBadNotesAndRelocations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func([]byte)
	}{
		{"unknown note name", func(img []byte) { copy(img[0x3000+12:], []byte("BAD-NTE\x00")) }},
		{"unknown note type", func(img []byte) { binary.LittleEndian.PutUint32(img[0x3000+8:0x3000+12], 0x1234) }},
		{"duplicate IOS-MOD", func(img []byte) {
			n := makeM164IOSMNote()
			copy(img[0x3000+len(n):], n)
			binary.LittleEndian.PutUint64(img[64+112+32:64+112+40], uint64(len(n)*2))
		}},
		{"duplicate IOS-REL", func(img []byte) {
			n := makeM164IOSRELNote(nil)
			off := 0x3000 + len(makeM164IOSMNote()) + len(n)
			copy(img[off:], n)
			binary.LittleEndian.PutUint64(img[64+112+32:64+112+40], uint64(off-0x3000+len(n)))
		}},
		{"bad IOSM size", func(img []byte) { binary.LittleEndian.PutUint32(img[0x3000+4:0x3000+8], 127) }},
		{"bad IOSM schema", func(img []byte) { binary.LittleEndian.PutUint32(img[0x3000+24:0x3000+28], 2) }},
		{"bad IOS-REL magic", func(img []byte) {
			relOff := 0x3000 + len(makeM164IOSMNote())
			binary.LittleEndian.PutUint32(img[relOff+24:relOff+28], 0)
		}},
		{"bad IOS-REL header size", func(img []byte) {
			relOff := 0x3000 + len(makeM164IOSMNote())
			binary.LittleEndian.PutUint16(img[relOff+30:relOff+32], 24)
		}},
		{"relocation count overflow", func(img []byte) {
			relOff := 0x3000 + len(makeM164IOSMNote())
			binary.LittleEndian.PutUint32(img[relOff+36:relOff+40], 0xffffffff)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			image := makeM1641ELFFixture(t, nil, true)
			tc.patch(image)
			if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err == nil {
				t.Fatalf("%s accepted", tc.name)
			}
		})
	}
}

func TestIExec_M1641_RuntimeELFValidatorRejectsBadIOSRELRecords(t *testing.T) {
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

func makeM1641ELFFixture(t *testing.T, relocs []m164RelaSpec, forceRelNote bool) []byte {
	t.Helper()
	const (
		codeOff = 0x1000
		dataOff = 0x2000
		noteOff = 0x3000
	)
	note := append([]byte{}, makeM164IOSMNote()...)
	if len(relocs) > 0 || forceRelNote {
		note = append(note, makeM164IOSRELNote(relocs)...)
	}
	out := make([]byte, 0x4000)
	copy(out[codeOff:], []byte{0xE0, 0, 0, 0, 0, 0, 0, 0})
	copy(out[dataOff:], []byte{0, 0, 0, 0, 0, 0, 0, 0})
	copy(out[noteOff:], note)

	copy(out[0:16], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(out[16:18], m164ELFTypeDyn)
	binary.LittleEndian.PutUint16(out[18:20], m14ELFMachineIE64)
	binary.LittleEndian.PutUint32(out[20:24], 1)
	binary.LittleEndian.PutUint64(out[24:32], 0)
	binary.LittleEndian.PutUint64(out[32:40], 64)
	binary.LittleEndian.PutUint64(out[40:48], 0)
	binary.LittleEndian.PutUint16(out[52:54], 64)
	binary.LittleEndian.PutUint16(out[54:56], 56)
	binary.LittleEndian.PutUint16(out[56:58], 3)
	binary.LittleEndian.PutUint16(out[58:60], 0)
	binary.LittleEndian.PutUint16(out[60:62], 0)
	binary.LittleEndian.PutUint16(out[62:64], 0)

	putPH := func(off int, typ, flags uint32, fileOff, vaddr, filesz, memsz, align uint64) {
		binary.LittleEndian.PutUint32(out[off:off+4], typ)
		binary.LittleEndian.PutUint32(out[off+4:off+8], flags)
		binary.LittleEndian.PutUint64(out[off+8:off+16], fileOff)
		binary.LittleEndian.PutUint64(out[off+16:off+24], vaddr)
		binary.LittleEndian.PutUint64(out[off+24:off+32], vaddr)
		binary.LittleEndian.PutUint64(out[off+32:off+40], filesz)
		binary.LittleEndian.PutUint64(out[off+40:off+48], memsz)
		binary.LittleEndian.PutUint64(out[off+48:off+56], align)
	}
	putPH(64, m14ELFPTLoad, m14ELFSegFlagR|m14ELFSegFlagX, codeOff, 0, 8, 0x1000, m14ELFPageAlign)
	putPH(64+56, m14ELFPTLoad, m14ELFSegFlagR|m14ELFSegFlagW, dataOff, 0x2000, 8, 0x1000, m14ELFPageAlign)
	putPH(64+112, m14ELFPTNote, m14ELFSegFlagR, noteOff, 0, uint64(len(note)), uint64(len(note)), 4)
	return out
}

func makeM164IOSRELNote(relocs []m164RelaSpec) []byte {
	name := []byte("IOS-REL\x00")
	desc := make([]byte, 32+len(relocs)*24)
	binary.LittleEndian.PutUint32(desc[0:4], m164IOSRelMagic)
	binary.LittleEndian.PutUint16(desc[4:6], 1)
	binary.LittleEndian.PutUint16(desc[6:8], 32)
	binary.LittleEndian.PutUint16(desc[8:10], 24)
	binary.LittleEndian.PutUint32(desc[12:16], uint32(len(relocs)))
	for i, rel := range relocs {
		off := 32 + i*24
		binary.LittleEndian.PutUint64(desc[off:off+8], rel.Offset)
		binary.LittleEndian.PutUint64(desc[off+8:off+16], uint64(rel.Symbol)<<32|uint64(rel.Type))
		binary.LittleEndian.PutUint64(desc[off+16:off+24], uint64(rel.Addend))
	}
	nameLen := (len(name) + 3) &^ 3
	descLen := (len(desc) + 3) &^ 3
	out := make([]byte, 12+nameLen+descLen)
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(name)))
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(desc)))
	binary.LittleEndian.PutUint32(out[8:12], m164IOSRelNoteType)
	copy(out[12:], name)
	copy(out[12+nameLen:], desc)
	return out
}

func TestIExec_M1641_NoDynamicLinkingWording(t *testing.T) {
	body := mustReadRepoFile(t, "sdk/docs/IntuitionOS/M16.4.1-plan.md")
	for _, forbidden := range []string{"DT_NEEDED", "PLT/GOT", "imported-symbol lookup", "lazy binding"} {
		if !strings.Contains(body, forbidden) {
			t.Fatalf("M16.4.1 plan missing dynamic-linker rejection wording %q", forbidden)
		}
	}
}

func TestIExec_M1641_AllIExecRuntimeTargetsValidateAsStrippedPTNoteELF(t *testing.T) {
	targets := m163RuntimeELFTargets(t)
	if len(targets) == 0 {
		t.Fatal("Makefile exposed no IEXEC_RUNTIME_ELF_TARGETS")
	}
	for _, target := range targets {
		t.Run(target.elfName, func(t *testing.T) {
			image := mustReadRepoBytes(t, "sdk/intuitionos/iexec/"+target.elfName)
			assertM1641StrippedRuntimeELF(t, image, target.elfName)
		})
	}
}

func TestIExec_M1641_ExportedRuntimeELFsMatchRebuiltSourcesAndValidate(t *testing.T) {
	root := filepath.Join(repoRootDir(t), "sdk", "intuitionos", "system", "SYS", "IOSSYS")
	for _, tc := range m1641RuntimeExports() {
		t.Run(tc.exportRel, func(t *testing.T) {
			source := mustReadRepoBytes(t, tc.sourceRel)
			exported := mustReadRepoBytes(t, filepath.Join(root, filepath.FromSlash(tc.exportRel)))
			if !bytes.Equal(exported, source) {
				t.Fatalf("%s does not match rebuilt source artifact %s", tc.exportRel, tc.sourceRel)
			}
			assertM1641StrippedRuntimeELF(t, exported, tc.exportRel)
		})
	}
}

func TestIExec_M1641_RebuiltRuntimeManifestVersions(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(filepath.Base(want.path), func(t *testing.T) {
			manifest, err := parseM16LibManifestNote(mustReadRepoBytes(t, want.path))
			if err != nil {
				t.Fatalf("parse IOSM: %v", err)
			}
			if manifest.Name != want.name || manifest.Kind != want.kind {
				t.Fatalf("manifest identity name=%q kind=%d, want %q kind=%d", manifest.Name, manifest.Kind, want.name, want.kind)
			}
			if manifest.LibVersion != want.version || manifest.LibRevision != want.revision || manifest.Patch != want.patch {
				t.Fatalf("manifest version=%d.%d.%d, want %d.%d.%d",
					manifest.LibVersion, manifest.LibRevision, manifest.Patch, want.version, want.revision, want.patch)
			}
		})
	}
}

func TestIExec_M1641_VersionBumpSourceAudit(t *testing.T) {
	inc := mustReadRepoFile(t, "sdk/include/iexec.inc")
	iexec := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	doslib := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	version := mustReadRepoFile(t, "sdk/intuitionos/iexec/cmd/version.s")
	help := mustReadRepoFile(t, "sdk/intuitionos/iexec/assets/system/S/Help")

	requireAllSubstrings(t, inc, "IOS_VERSION_PATCH  equ 6")
	requireAllSubstrings(t, version, "IntuitionOS 1.16.6", "exec.library 1.16.6")
	requireAllSubstrings(t, help, "IntuitionOS 1.16.6 help")
	requireAllSubstrings(t, iexec, "move.l  r12, #15")
	requireAllSubstrings(t, doslib,
		`.libmanifest name="dos.library", version=15, revision=0`,
		"move.l  r2, #15",
	)
	requireNoSubstrings(t, version, "IntuitionOS 1.16.4", "exec.library 1.16.4")
	requireNoSubstrings(t, help, "IntuitionOS 1.16.4")
}

func TestIExec_M1641_KASLRReadinessAndSecurityStaticGuards(t *testing.T) {
	iexec := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	doslib := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	inc := mustReadRepoFile(t, "sdk/include/iexec.inc")
	docs := mustReadRepoFile(t, "sdk/docs/IntuitionOS/M16.4.1-plan.md")

	requireAllSubstrings(t, docs,
		"fixed `KERN_PAGE_TABLE`",
		"`KERN_DATA_BASE`",
		"`KERN_STACK_TOP`",
		"supervisor identity mapping",
		"trap/fault paths",
		"scheduler",
		"state access",
		"panic/debug paths",
		"task page-table copying of kernel mappings",
	)
	requireAllSubstrings(t, iexec,
		"MMU_CTRL_ENABLE | MMU_CTRL_SKEF | MMU_CTRL_SKAC",
		"and     r11, r30, #BOOT_ELF_EXEC_FLAG_TRUSTED_INTERNAL",
		"and     r11, r2, #(MAPF_READ | MAPF_WRITE)",
		"beqz    r11, .ms_badarg",
		"bne     r11, r2, .ms_badarg",
	)
	requireAllSubstrings(t, doslib,
		"SYSINFO_ASLR_IMAGE_BASE equ 7",
		"BOOT_ELF_EXEC_FLAG_TRUSTED_INTERNAL",
		".dos_resolved_is_iossys:",
		"beqz    r3, .dos_loadlib_fail",
	)
	requireNoSubstrings(t, inc,
		"SYSINFO_KERNEL",
		"SYSINFO_KASLR",
		"MAPF_EXEC",
	)
	requireNoSubstrings(t, iexec, "MAPF_EXEC", "PT_DYNAMIC")
	requireNoSubstrings(t, doslib, "MAPF_EXEC", "PT_DYNAMIC")
}

func TestIExec_M1641_StrippedIOSRELRelocationPathsAreLive(t *testing.T) {
	iexec := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	doslib := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")

	requireAllSubstrings(t, iexec,
		"beqz    r18, .bear_ptnote_relocs",
		".bear_ptnote_relocs:",
		"kbvlm_reloc_note_name:",
		"dc.b    \"IOS-REL\", 0",
		"move.l  r12, #0x494F5231           ; IOS-REL note type",
		"move.l  r12, #0x52534F49           ; IOSR",
		"bra     .bear_rela_loop",
	)
	requireAllSubstrings(t, doslib,
		"beqz    r18, .dear_ptnote_relocs",
		".dear_ptnote_relocs:",
		".dos_pmp_is_iosrel_note_name:",
		"move.l  r12, #0x494F5231            ; IOS-REL note type",
		"move.l  r12, #0x52534F49            ; IOSR",
		"bra     .dear_rela_loop",
	)
	requireNoSubstrings(t, iexec,
		"load.q  r18, 40(r1)                ; e_shoff\n    beqz    r18, .bear_ok",
	)
	requireNoSubstrings(t, doslib,
		"load.q  r18, 40(r1)                 ; e_shoff\n    beqz    r18, .dear_ok",
	)
}

func TestIExec_M1641_DocsCloseoutCrossLinks(t *testing.T) {
	for _, rel := range []string{
		"sdk/docs/IntuitionOS/ELF.md",
		"sdk/docs/IntuitionOS/Toolchain.md",
		"sdk/docs/IntuitionOS/IExec.md",
		"sdk/docs/IntuitionOS/M16.4-plan.md",
		"sdk/docs/IntuitionOS/M16-plan.md",
		"sdk/docs/include-files.md",
		"sdk/README.md",
		"README.md",
		"IntuitionOS_Roadmap.md",
	} {
		body := mustReadRepoFile(t, rel)
		requireAllSubstrings(t, body,
			"M16.4.1",
			"PT_NOTE",
			"stripped",
			"section-header-free",
			"dynamic linking",
		)
	}
}

func assertM1641StrippedRuntimeELF(t *testing.T, image []byte, label string) {
	t.Helper()
	if len(image) < 64 {
		t.Fatalf("%s too small for ELF header", label)
	}
	if eShOff := binary.LittleEndian.Uint64(image[40:48]); eShOff != 0 {
		t.Fatalf("%s e_shoff=%#x, want 0", label, eShOff)
	}
	if eShNum := binary.LittleEndian.Uint16(image[60:62]); eShNum != 0 {
		t.Fatalf("%s e_shnum=%d, want 0", label, eShNum)
	}
	if eShStrNdx := binary.LittleEndian.Uint16(image[62:64]); eShStrNdx != 0 {
		t.Fatalf("%s e_shstrndx=%d, want 0", label, eShStrNdx)
	}
	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("%s parse ELF: %v", label, err)
	}
	noteCount := 0
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_NOTE {
			noteCount++
		}
	}
	if noteCount != 1 {
		t.Fatalf("%s PT_NOTE count=%d, want 1", label, noteCount)
	}
	if err := validateM164RuntimeELFContract(image, m164Placement{Base: 0x00640000}); err != nil {
		t.Fatalf("%s violates M16.4.1 runtime ELF contract: %v", label, err)
	}
}

func m1641RuntimeExports() []struct {
	exportRel string
	sourceRel string
} {
	return []struct {
		exportRel string
		sourceRel string
	}{
		{"Tools/Shell", "sdk/intuitionos/iexec/boot_shell.elf"},
		{"LIBS/dos.library", "sdk/intuitionos/iexec/boot_dos_library.elf"},
		{"LIBS/graphics.library", "sdk/intuitionos/iexec/boot_graphics_library.elf"},
		{"LIBS/intuition.library", "sdk/intuitionos/iexec/boot_intuition_library.elf"},
		{"DEVS/input.device", "sdk/intuitionos/iexec/boot_input_device.elf"},
		{"RESOURCES/hardware.resource", "sdk/intuitionos/iexec/boot_hardware_resource.elf"},
		{"L/console.handler", "sdk/intuitionos/iexec/boot_console_handler.elf"},
		{"C/Version", "sdk/intuitionos/iexec/cmd_version.elf"},
		{"C/Avail", "sdk/intuitionos/iexec/cmd_avail.elf"},
		{"C/Dir", "sdk/intuitionos/iexec/cmd_dir.elf"},
		{"C/Type", "sdk/intuitionos/iexec/cmd_type.elf"},
		{"C/Echo", "sdk/intuitionos/iexec/cmd_echo.elf"},
		{"C/Resident", "sdk/intuitionos/iexec/cmd_resident.elf"},
		{"C/Assign", "sdk/intuitionos/iexec/cmd_assign.elf"},
		{"C/List", "sdk/intuitionos/iexec/cmd_list.elf"},
		{"C/Which", "sdk/intuitionos/iexec/cmd_which.elf"},
		{"C/Help", "sdk/intuitionos/iexec/cmd_help.elf"},
		{"C/GfxDemo", "sdk/intuitionos/iexec/cmd_gfxdemo.elf"},
		{"C/About", "sdk/intuitionos/iexec/cmd_about.elf"},
		{"C/ElfSeg", "sdk/intuitionos/iexec/elfseg_fixture.elf"},
	}
}
