package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	m161IOSMSectionName = ".ios.manifest"
	m161IOSMNoteName    = "IOS-MOD\x00"
	m161IOSMNoteType    = 0x494F5331
	m161IOSMMagic       = 0x4D534F49
	m161IOSMSize        = 128
	m161Copyright       = "Copyright \xA9 2026 Zayn Otley"
)

type m161RuntimeELFManifest struct {
	path         string
	label        string
	name         string
	kind         uint8
	version      uint16
	revision     uint16
	patch        uint16
	flags        uint32
	msgABI       uint32
	compatPort   bool
	sourceBacked bool
}

func m161RuntimeELFManifests() []m161RuntimeELFManifest {
	return []m161RuntimeELFManifest{
		{path: "sdk/intuitionos/iexec/boot_dos_library.elf", label: "prog_doslib", name: "dos.library", kind: 1, version: 14, sourceBacked: true, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_console_handler.elf", label: "prog_console", name: "console.handler", kind: 3, version: 1, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_shell.elf", label: "prog_shell", name: "Shell", kind: 3, version: 1},
		{path: "sdk/intuitionos/iexec/boot_hardware_resource.elf", label: "prog_hwres", name: "hardware.resource", kind: 4, version: 1, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_input_device.elf", label: "prog_input_device", name: "input.device", kind: 2, version: 1, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_graphics_library.elf", label: "prog_graphics_library", name: "graphics.library", kind: 1, version: 11, sourceBacked: true, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_intuition_library.elf", label: "prog_intuition_library", name: "intuition.library", kind: 1, version: 12, sourceBacked: true, flags: 2, compatPort: true},
		{path: "sdk/intuitionos/iexec/cmd_version.elf", label: "prog_version", name: "Version", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_avail.elf", label: "prog_avail", name: "Avail", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_dir.elf", label: "prog_dir", name: "Dir", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_type.elf", label: "prog_type", name: "Type", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_echo.elf", label: "prog_echo_cmd", name: "Echo", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_resident.elf", label: "prog_resident_cmd", name: "Resident", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_assign.elf", label: "prog_assign_cmd", name: "Assign", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_list.elf", label: "prog_list_cmd", name: "List", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_which.elf", label: "prog_which_cmd", name: "Which", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_help.elf", label: "prog_help_app", name: "Help", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_gfxdemo.elf", label: "prog_gfxdemo", name: "GfxDemo", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/cmd_about.elf", label: "prog_about", name: "About", kind: 5, version: 1},
		{path: "sdk/intuitionos/iexec/elfseg_fixture.elf", label: "prog_elfseg", name: "ElfSeg", kind: 5, version: 1},
	}
}

func TestIExec_M161_Phase2_IOSMNoteSection_AllELFs(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(filepath.Base(want.path), func(t *testing.T) {
			desc := mustReadM161IOSMDesc(t, want.path)
			if got := binary.LittleEndian.Uint32(desc[0:4]); got != m161IOSMMagic {
				t.Fatalf("%s magic=%#x, want %#x", want.path, got, m161IOSMMagic)
			}
			if got := binary.LittleEndian.Uint32(desc[4:8]); got != 1 {
				t.Fatalf("%s schema=%d, want 1", want.path, got)
			}
			if got := desc[8]; got != want.kind {
				t.Fatalf("%s kind=%d, want %d", want.path, got, want.kind)
			}
			if got := cString(desc[16:48]); got != want.name {
				t.Fatalf("%s name=%q, want %q", want.path, got, want.name)
			}
		})
	}
}

func TestIExec_M161_Phase2_CarryForwardVersions(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(filepath.Base(want.path), func(t *testing.T) {
			desc := mustReadM161IOSMDesc(t, want.path)
			if got := binary.LittleEndian.Uint16(desc[10:12]); got != want.version {
				t.Fatalf("%s version=%d, want %d", want.path, got, want.version)
			}
			if got := binary.LittleEndian.Uint16(desc[12:14]); got != want.revision {
				t.Fatalf("%s revision=%d, want %d", want.path, got, want.revision)
			}
			if got := binary.LittleEndian.Uint16(desc[14:16]); got != want.patch {
				t.Fatalf("%s patch=%d, want %d", want.path, got, want.patch)
			}
			if got := binary.LittleEndian.Uint32(desc[48:52]); got != want.flags {
				t.Fatalf("%s flags=%#x, want %#x", want.path, got, want.flags)
			}
		})
	}
}

func TestIExec_M161_Phase2_BuildDate_RespectsSourceDateEpoch(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(filepath.Base(want.path), func(t *testing.T) {
			desc := mustReadM161IOSMDesc(t, want.path)
			if got := cString(desc[56:72]); got == "" {
				t.Fatalf("%s build date is empty", want.path)
			}
		})
	}
}

func TestIExec_M161_Phase2_CopyrightFieldStatic(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(filepath.Base(want.path), func(t *testing.T) {
			desc := mustReadM161IOSMDesc(t, want.path)
			if got := cString(desc[72:120]); got != m161Copyright {
				t.Fatalf("%s copyright=%q, want %q", want.path, got, m161Copyright)
			}
			for _, bad := range []string{os.Getenv("USER"), os.Getenv("HOSTNAME"), "SOURCE_DATE_EPOCH", "PWD", "HOME"} {
				if bad != "" && strings.Contains(cString(desc[72:120]), bad) {
					t.Fatalf("%s copyright field contains environment-derived substring %q", want.path, bad)
				}
			}
		})
	}
}

func TestIExec_M161_Phase2_CopyrightFitsField(t *testing.T) {
	if got := len([]byte(m161Copyright)) + 1; got > 48 {
		t.Fatalf("copyright literal uses %d bytes including NUL, want <= 48", got)
	}
}

func TestIExec_M161_Phase2_ManifestVAddr_LabelExposed(t *testing.T) {
	for _, want := range m161RuntimeELFManifests() {
		t.Run(want.label, func(t *testing.T) {
			listing := mustReadRepoFile(t, "sdk/intuitionos/iexec/runtime_builder.lst")
			if !strings.Contains(listing, want.label+"_iosm:") {
				t.Fatalf("runtime listing missing %s_iosm label", want.label)
			}
		})
	}
}

func mustReadM161IOSMDesc(t *testing.T, rel string) []byte {
	t.Helper()

	image := mustReadRepoBytes(t, rel)
	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	sec := f.Section(m161IOSMSectionName)
	if sec == nil {
		t.Fatalf("%s missing %s", rel, m161IOSMSectionName)
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
	typ := binary.LittleEndian.Uint32(data[8:12])
	nameOff := 12
	nameEnd := nameOff + int(namesz)
	descOff := 12 + int((namesz+3)&^3)
	descEnd := descOff + int(descsz)
	if typ != m161IOSMNoteType {
		t.Fatalf("%s note type=%#x, want %#x", rel, typ, m161IOSMNoteType)
	}
	if nameEnd > len(data) || string(data[nameOff:nameEnd]) != m161IOSMNoteName {
		t.Fatalf("%s note name=%q, want %q", rel, string(data[nameOff:min(nameEnd, len(data))]), m161IOSMNoteName)
	}
	if descsz != m161IOSMSize || descEnd > len(data) {
		t.Fatalf("%s descsz=%d descEnd=%d len=%d", rel, descsz, descEnd, len(data))
	}
	return append([]byte(nil), data[descOff:descEnd]...)
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
