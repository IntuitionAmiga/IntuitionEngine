package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWriteAROSScreenModePrefsCreatesIE1080pPreference(t *testing.T) {
	out := filepath.Join(t.TempDir(), "screenmode.prefs")
	cmd := exec.Command("./scripts/write-aros-screenmode-prefs.sh", out, "1920", "1080", "8")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write-aros-screenmode-prefs failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated prefs: %v", err)
	}

	if got, want := len(data), 62; got != want {
		t.Fatalf("prefs length = %d, want %d", got, want)
	}
	if !bytes.Equal(data[0:4], []byte("FORM")) || !bytes.Equal(data[8:12], []byte("PREF")) {
		t.Fatalf("prefs missing FORM/PREF header: %q %q", data[0:4], data[8:12])
	}
	if got, want := binary.BigEndian.Uint32(data[4:8]), uint32(len(data)-8); got != want {
		t.Fatalf("FORM size = %d, want %d", got, want)
	}
	if !bytes.Equal(data[12:16], []byte("PRHD")) {
		t.Fatalf("missing PRHD chunk")
	}
	if got := binary.BigEndian.Uint32(data[16:20]); got != 6 {
		t.Fatalf("PRHD size = %d, want 6", got)
	}
	if !bytes.Equal(data[26:30], []byte("SCRM")) {
		t.Fatalf("missing SCRM chunk")
	}
	if got := binary.BigEndian.Uint32(data[30:34]); got != 28 {
		t.Fatalf("SCRM size = %d, want 28", got)
	}

	scrm := data[34:62]
	if got := binary.BigEndian.Uint32(scrm[16:20]); got != 0xffffffff {
		t.Fatalf("display id = %#x, want INVALID_ID", got)
	}
	if got := binary.BigEndian.Uint16(scrm[20:22]); got != 1920 {
		t.Fatalf("width = %d, want 1920", got)
	}
	if got := binary.BigEndian.Uint16(scrm[22:24]); got != 1080 {
		t.Fatalf("height = %d, want 1080", got)
	}
	if got := binary.BigEndian.Uint16(scrm[24:26]); got != 8 {
		t.Fatalf("depth = %d, want 8", got)
	}
	if got := binary.BigEndian.Uint16(scrm[26:28]); got != 0 {
		t.Fatalf("control = %d, want 0", got)
	}
}

func TestWriteAROSPalettePrefsCreatesDefaultPens(t *testing.T) {
	out := filepath.Join(t.TempDir(), "palette.prefs")
	cmd := exec.Command("./scripts/write-aros-palette-prefs.sh", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write-aros-palette-prefs failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated prefs: %v", err)
	}

	if got, want := len(data), 434; got != want {
		t.Fatalf("prefs length = %d, want %d", got, want)
	}
	if !bytes.Equal(data[0:4], []byte("FORM")) || !bytes.Equal(data[8:12], []byte("PREF")) {
		t.Fatalf("prefs missing FORM/PREF header: %q %q", data[0:4], data[8:12])
	}
	if got, want := binary.BigEndian.Uint32(data[4:8]), uint32(len(data)-8); got != want {
		t.Fatalf("FORM size = %d, want %d", got, want)
	}
	if !bytes.Equal(data[26:30], []byte("PALT")) {
		t.Fatalf("missing PALT chunk")
	}
	if got := binary.BigEndian.Uint32(data[30:34]); got != 400 {
		t.Fatalf("PALT size = %d, want 400", got)
	}

	palt := data[34:]
	if got := binary.BigEndian.Uint16(palt[16:18]); got != 1 {
		t.Fatalf("DETAILPEN mapping = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint16(palt[18:20]); got != 0 {
		t.Fatalf("BLOCKPEN mapping = %d, want 0", got)
	}
	if got := binary.BigEndian.Uint16(palt[20:22]); got != 1 {
		t.Fatalf("TEXTPEN mapping = %d, want 1", got)
	}

	colors := palt[16+64+64:]
	if got := binary.BigEndian.Uint16(colors[0:2]); got != 0 {
		t.Fatalf("first color index = %d, want 0", got)
	}
	if got := binary.BigEndian.Uint16(colors[2:4]); got != 0xaaaa {
		t.Fatalf("color 0 red = %#x, want 0xaaaa", got)
	}
	if got := binary.BigEndian.Uint16(colors[8:10]); got != 1 {
		t.Fatalf("second color index = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint16(colors[64:66]); got != 0xffff {
		t.Fatalf("color sentinel = %#x, want 0xffff", got)
	}
}
