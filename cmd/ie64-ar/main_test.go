package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func TestRunRCSCreatesAndReplacesIndexedArchive(t *testing.T) {
	dir := t.TempDir()
	member := filepath.Join(dir, "x.o")
	archive := filepath.Join(dir, "libx.a")
	write := func(size int) {
		o := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Data: make([]byte, size)}}, Symbols: []ie64obj.Symbol{{Name: "x", Bind: ie64obj.STBGlobal, Section: 1}}}
		b, e := o.Marshal()
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(member, b, 0o644); e != nil {
			t.Fatal(e)
		}
	}
	write(8)
	if c := run([]string{"rcs", archive, member}); c != 0 {
		t.Fatalf("create=%d", c)
	}
	write(16)
	if c := run([]string{"rcs", archive, member}); c != 0 {
		t.Fatalf("replace=%d", c)
	}
	data, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	a, e := ie64archive.Parse(data)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Members) != 1 || a.Symbols["x"] != 0 {
		t.Fatalf("archive=%#v", a)
	}
}
