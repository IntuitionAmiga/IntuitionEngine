package main

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestBuildManifestNoteUsesIOSMSchema(t *testing.T) {
	spec := libManifestSpec{
		Name:          "template.library",
		Kind:          iosmKindLibrary,
		Version:       23,
		Revision:      4,
		Patch:         5,
		Flags:         iosmModfCompatPort | iosmModfASLRCapable,
		MsgABIVersion: 9,
		BuildDate:     "2026-04-22",
		Copyright:     iosmCopyright,
	}

	note := buildManifestNote(spec)
	if got := binary.LittleEndian.Uint32(note[0:4]); got != uint32(len(iosmNoteName)+1) {
		t.Fatalf("namesz=%d, want %d", got, len(iosmNoteName)+1)
	}
	if got := binary.LittleEndian.Uint32(note[4:8]); got != iosmSize {
		t.Fatalf("descsz=%d, want %d", got, iosmSize)
	}
	if got := binary.LittleEndian.Uint32(note[8:12]); got != iosmNoteType {
		t.Fatalf("note type=%#x, want %#x", got, iosmNoteType)
	}

	nameStart := 12
	descStart := nameStart + len(iosmNoteName) + 1
	desc := note[descStart : descStart+iosmSize]
	if got := binary.LittleEndian.Uint32(desc[0:4]); got != iosmMagic {
		t.Fatalf("magic=%#x, want %#x", got, iosmMagic)
	}
	if got := binary.LittleEndian.Uint32(desc[4:8]); got != iosmSchemaVersion {
		t.Fatalf("schema=%d, want %d", got, iosmSchemaVersion)
	}
	if got := desc[8]; got != iosmKindLibrary {
		t.Fatalf("kind=%d, want %d", got, iosmKindLibrary)
	}
	if got := desc[9]; got != 0 {
		t.Fatalf("reserved0=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(desc[10:12]); got != 23 {
		t.Fatalf("version=%d, want 23", got)
	}
	if got := binary.LittleEndian.Uint16(desc[12:14]); got != 4 {
		t.Fatalf("revision=%d, want 4", got)
	}
	if got := binary.LittleEndian.Uint16(desc[14:16]); got != 5 {
		t.Fatalf("patch=%d, want 5", got)
	}
	if got := string(bytes.TrimRight(desc[16:48], "\x00")); got != "template.library" {
		t.Fatalf("name=%q, want template.library", got)
	}
	if got := binary.LittleEndian.Uint32(desc[48:52]); got != iosmModfCompatPort|iosmModfASLRCapable {
		t.Fatalf("flags=%#x, want %#x", got, iosmModfCompatPort|iosmModfASLRCapable)
	}
	if got := binary.LittleEndian.Uint32(desc[52:56]); got != 9 {
		t.Fatalf("msg_abi=%d, want 9", got)
	}
	if got := string(bytes.TrimRight(desc[56:72], "\x00")); got != "2026-04-22" {
		t.Fatalf("build_date=%q, want 2026-04-22", got)
	}
	if got := string(bytes.TrimRight(desc[72:120], "\x00")); got != iosmCopyright {
		t.Fatalf("copyright=%q, want %q", got, iosmCopyright)
	}
	if !bytes.Equal(desc[120:128], make([]byte, 8)) {
		t.Fatalf("reserved2 not zero: %v", desc[120:128])
	}
}

func TestBuildELFUsesPTNoteIOSMManifestMetadata(t *testing.T) {
	spec := libManifestSpec{
		Name:          "template.library",
		Kind:          iosmKindLibrary,
		Version:       23,
		Revision:      4,
		Flags:         iosmModfCompatPort | iosmModfASLRCapable,
		MsgABIVersion: 9,
		BuildDate:     "2026-04-22",
		Copyright:     iosmCopyright,
	}

	image := buildELF([]byte{0xE0, 0, 0, 0, 0, 0, 0, 0}, []byte{1, 2, 3, 4}, spec, true)
	if got := binary.LittleEndian.Uint64(image[40:48]); got != 0 {
		t.Fatalf("e_shoff=%#x, want stripped section-header-free ELF", got)
	}
	if got := binary.LittleEndian.Uint16(image[60:62]); got != 0 {
		t.Fatalf("e_shnum=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(image[62:64]); got != 0 {
		t.Fatalf("e_shstrndx=%d, want 0", got)
	}
	note := mustBuildELFPTNote(t, image)
	if got := binary.LittleEndian.Uint32(note[8:12]); got != iosmNoteType {
		t.Fatalf("note type=%#x, want %#x", got, iosmNoteType)
	}
}

func mustBuildELFPTNote(t *testing.T, image []byte) []byte {
	t.Helper()
	phoff := binary.LittleEndian.Uint64(image[32:40])
	phentsize := binary.LittleEndian.Uint16(image[54:56])
	phnum := binary.LittleEndian.Uint16(image[56:58])
	for i := uint16(0); i < phnum; i++ {
		off := phoff + uint64(i)*uint64(phentsize)
		if binary.LittleEndian.Uint32(image[off:off+4]) != elfPTNote {
			continue
		}
		fileOff := binary.LittleEndian.Uint64(image[off+8 : off+16])
		filesz := binary.LittleEndian.Uint64(image[off+32 : off+40])
		return image[fileOff : fileOff+filesz]
	}
	t.Fatal("missing PT_NOTE")
	return nil
}

func TestResolveBuildDate(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1774310400") // 2026-03-24 UTC
	if got, err := resolveBuildDate(""); err != nil || got != "2026-03-24" {
		t.Fatalf("resolveBuildDate with SOURCE_DATE_EPOCH = %q, %v; want 2026-03-24 nil", got, err)
	}
	if got, err := resolveBuildDate("2026-04-22"); err != nil || got != "2026-04-22" {
		t.Fatalf("resolveBuildDate explicit = %q, %v; want 2026-04-22 nil", got, err)
	}
	t.Setenv("SOURCE_DATE_EPOCH", "")
	got, err := resolveBuildDate("")
	if err != nil {
		t.Fatalf("resolveBuildDate fallback: %v", err)
	}
	if _, err := time.Parse("2006-01-02", got); err != nil {
		t.Fatalf("fallback build date %q is not YYYY-MM-DD: %v", got, err)
	}
}

func TestManifestSpecForLabelPrefersListing(t *testing.T) {
	listing := []byte(`
                         prog_graphics_library:
00001000  E0 00 00 00 00 00 00 00... nop
                         .libmanifest name="graphics.library", version=77, revision=6, type=1, flags=5, msg_abi=9
00001008  E0 00 00 00 00 00 00 00... nop
`)

	spec, ok, err := manifestSpecForLabel(listing, "prog_graphics_library")
	if err != nil {
		t.Fatalf("manifestSpecForLabel: %v", err)
	}
	if !ok {
		t.Fatal("manifestSpecForLabel did not find manifest")
	}
	if spec.Version != 77 || spec.Revision != 6 || spec.Flags != 5 || spec.MsgABIVersion != 9 {
		t.Fatalf("listing manifest lost precedence: got %+v", spec)
	}
}

func TestManifestSpecForLabel_RequiresSourceManifestForLibraries(t *testing.T) {
	listing := []byte(`
                         prog_graphics_library:
00001000  E0 00 00 00 00 00 00 00... nop
00001008  E0 00 00 00 00 00 00 00... nop
`)

	_, ok, err := manifestSpecForLabel(listing, "prog_graphics_library")
	if err == nil {
		t.Fatal("manifestSpecForLabel error=nil, want missing source manifest failure")
	}
	if ok {
		t.Fatal("manifestSpecForLabel ok=true, want false on missing library manifest")
	}
}
