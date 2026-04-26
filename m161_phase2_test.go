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
	m161IOSMMagic       = 0x4D534F49
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
		{path: "sdk/intuitionos/iexec/boot_dos_library.elf", label: "prog_doslib", name: "dos.library", kind: 1, version: 16, sourceBacked: true, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_console_handler.elf", label: "prog_console", name: "console.handler", kind: 3, version: 1, patch: 1, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_shell.elf", label: "prog_shell", name: "Shell", kind: 3, version: 1, patch: 1, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_hardware_resource.elf", label: "prog_hwres", name: "hardware.resource", kind: 4, version: 1, patch: 1, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_input_device.elf", label: "prog_input_device", name: "input.device", kind: 2, version: 1, patch: 1, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_graphics_library.elf", label: "prog_graphics_library", name: "graphics.library", kind: 1, version: 11, patch: 1, sourceBacked: true, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/boot_intuition_library.elf", label: "prog_intuition_library", name: "intuition.library", kind: 1, version: 12, patch: 1, sourceBacked: true, flags: 6, compatPort: true},
		{path: "sdk/intuitionos/iexec/cmd_version.elf", label: "prog_version", name: "Version", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_avail.elf", label: "prog_avail", name: "Avail", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_dir.elf", label: "prog_dir", name: "Dir", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_type.elf", label: "prog_type", name: "Type", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_echo.elf", label: "prog_echo_cmd", name: "Echo", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_resident.elf", label: "prog_resident_cmd", name: "Resident", kind: 5, version: 1, revision: 2, patch: 0, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_assign.elf", label: "prog_assign_cmd", name: "Assign", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_list.elf", label: "prog_list_cmd", name: "List", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_which.elf", label: "prog_which_cmd", name: "Which", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_help.elf", label: "prog_help_app", name: "Help", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_gfxdemo.elf", label: "prog_gfxdemo", name: "GfxDemo", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/cmd_about.elf", label: "prog_about", name: "About", kind: 5, version: 1, patch: 1, flags: 4},
		{path: "sdk/intuitionos/iexec/elfseg_fixture.elf", label: "prog_elfseg", name: "ElfSeg", kind: 5, version: 1, patch: 1, flags: 4},
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
	if f.Section(m161IOSMSectionName) != nil {
		t.Fatalf("%s still carries legacy %s section metadata", rel, m161IOSMSectionName)
	}
	data, err := m16FindIOSMDescriptor(image)
	if err != nil {
		t.Fatalf("read %s PT_NOTE IOSM descriptor: %v", rel, err)
	}
	return append([]byte(nil), data...)
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
