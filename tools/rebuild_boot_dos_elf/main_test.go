package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"testing"
)

func TestParseLibManifestSpecFromListing(t *testing.T) {
	listing := []byte(`
                         prog_graphics_library:
00001000  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=11, revision=0, type=1, flags=2, msg_abi=0
00001008  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := parseLibManifestSpecFromListing(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("parseLibManifestSpecFromListing error: %v", err)
	}
	if !ok {
		t.Fatal("parseLibManifestSpecFromListing did not find a manifest")
	}
	if spec.Name != "graphics.library" {
		t.Fatalf("spec.Name=%q, want graphics.library", spec.Name)
	}
	if spec.Version != 11 || spec.Revision != 0 || spec.Type != 1 || spec.Flags != 2 || spec.MsgABIVersion != 0 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestParseLibManifestSpecFromListingSupportsExpressions(t *testing.T) {
	listing := []byte(`
         = 000000000000000B  LIB_VER equ 11
         = 0000000000000002  MODF_COMPAT_PORT equ 2
                         prog_graphics_library:
00001000  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=LIB_VER, revision=10+1, type=(1), flags=MODF_COMPAT_PORT|4, msg_abi=1<<2
00001008  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := parseLibManifestSpecFromListing(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("parseLibManifestSpecFromListing error: %v", err)
	}
	if !ok {
		t.Fatal("parseLibManifestSpecFromListing did not find a manifest")
	}
	if spec.Version != 11 {
		t.Fatalf("Version=%d, want 11", spec.Version)
	}
	if spec.Revision != 11 {
		t.Fatalf("Revision=%d, want 11", spec.Revision)
	}
	if spec.Type != 1 {
		t.Fatalf("Type=%d, want 1", spec.Type)
	}
	if spec.Flags != 6 {
		t.Fatalf("Flags=%d, want 6", spec.Flags)
	}
	if spec.MsgABIVersion != 4 {
		t.Fatalf("MsgABIVersion=%d, want 4", spec.MsgABIVersion)
	}
}

func TestParseLibManifestSpecFromListingSupportsSetAndLabelExpressions(t *testing.T) {
	listing := []byte(`
         = 0000000000000280  SCREEN_W set 640
                         data_start:
00001200  00 00 00 00             dc.l 0
                         data_end:
00001208  E0 00 00 00 00 00 00 00... nop
                         prog_graphics_library:
00002000  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=11, revision=SCREEN_W/128, type=1, flags=data_end-data_start, msg_abi=0
00002008  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := parseLibManifestSpecFromListing(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("parseLibManifestSpecFromListing error: %v", err)
	}
	if !ok {
		t.Fatal("parseLibManifestSpecFromListing did not find a manifest")
	}
	if spec.Revision != 5 {
		t.Fatalf("Revision=%d, want 5", spec.Revision)
	}
	if spec.Flags != 8 {
		t.Fatalf("Flags=%d, want 8", spec.Flags)
	}
}

func TestParseLibManifestSpecFromListingSupportsStackedLabelAliases(t *testing.T) {
	listing := []byte(`
                         alias_start:
                         real_start:
00001200  00 00 00 00             dc.l 0
                         alias_end:
                         real_end:
00001208  E0 00 00 00 00 00 00 00... nop
                         prog_graphics_library:
00002000  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=11, revision=11, type=1, flags=alias_end-alias_start, msg_abi=0
00002008  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := parseLibManifestSpecFromListing(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("parseLibManifestSpecFromListing error: %v", err)
	}
	if !ok {
		t.Fatal("parseLibManifestSpecFromListing did not find a manifest")
	}
	if spec.Flags != 8 {
		t.Fatalf("Flags=%d, want 8", spec.Flags)
	}
}

func TestParseLibManifestSpecFromListingSupportsLocalLabelShorthand(t *testing.T) {
	listing := []byte(`
                         prog_graphics_library:
                         .start:
00002000  00 00 00 00             dc.l 0
                         .end:
00002008  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=11, revision=11, type=1, flags=.end-.start, msg_abi=0
00002010  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := parseLibManifestSpecFromListing(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("parseLibManifestSpecFromListing error: %v", err)
	}
	if !ok {
		t.Fatal("parseLibManifestSpecFromListing did not find a manifest")
	}
	if spec.Flags != 8 {
		t.Fatalf("Flags=%d, want 8", spec.Flags)
	}
}

func TestBuildELFUsesListingManifestMetadata(t *testing.T) {
	spec := libManifestSpec{
		Name:          "template.library",
		Version:       23,
		Revision:      4,
		Type:          m16LibManifestTypeLibrary,
		Flags:         m16ModfCompatPort,
		MsgABIVersion: 9,
		BuildDate:     "2026-04-22",
		Copyright:     iosmCopyright,
	}

	image := buildELF([]byte{0xE0, 0, 0, 0, 0, 0, 0, 0}, []byte{1, 2, 3, 4}, spec, true)
	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("elf.NewFile: %v", err)
	}
	sec := f.Section(".ios.manifest")
	if sec == nil {
		t.Fatal("missing .ios.manifest section")
	}
	if sec.Type != elf.SHT_NOTE {
		t.Fatalf("section type=%v, want SHT_NOTE", sec.Type)
	}
	data, err := sec.Data()
	if err != nil {
		t.Fatalf("sec.Data: %v", err)
	}
	if got := binary.LittleEndian.Uint32(data[8:12]); got != m16LibManifestNoteType {
		t.Fatalf("note type=%#x, want %#x", got, m16LibManifestNoteType)
	}
	desc := data[12+len("IOS-MOD\x00"):]
	if got := binary.LittleEndian.Uint32(desc[0:4]); got != m16LibManifestMagic {
		t.Fatalf("magic=%#x, want %#x", got, m16LibManifestMagic)
	}
	if got := string(bytes.TrimRight(desc[16:48], "\x00")); got != "template.library" {
		t.Fatalf("name=%q, want template.library", got)
	}
	if got := binary.LittleEndian.Uint16(desc[10:12]); got != 23 {
		t.Fatalf("version=%d, want 23", got)
	}
	if got := binary.LittleEndian.Uint16(desc[12:14]); got != 4 {
		t.Fatalf("revision=%d, want 4", got)
	}
	if got := binary.LittleEndian.Uint32(desc[52:56]); got != 9 {
		t.Fatalf("msg_abi=%d, want 9", got)
	}
}
