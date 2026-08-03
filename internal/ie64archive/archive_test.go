package ie64archive

import (
	"bytes"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func objectBytes(t *testing.T, name string) []byte {
	t.Helper()
	o := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: name, Bind: ie64obj.STBGlobal, Section: 1}}}
	b, e := o.Marshal()
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestMarshalParseDeterministicIndexedArchive(t *testing.T) {
	members := []Member{{Name: "first.o", Data: objectBytes(t, "first")}, {Name: "a-very-long-member-name.o", Data: objectBytes(t, "second")}}
	a, err := Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("archive output is not deterministic")
	}
	parsed, err := Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Members) != 2 || parsed.Members[1].Name != members[1].Name {
		t.Fatalf("members = %#v", parsed.Members)
	}
	if parsed.Symbols["first"] != 0 || parsed.Symbols["second"] != 1 {
		t.Fatalf("index = %#v", parsed.Symbols)
	}
}

func TestReplaceMembersUsesLastReplacement(t *testing.T) {
	a := []Member{{Name: "x.o", Data: []byte("old")}, {Name: "y.o", Data: []byte("y")}}
	got := Replace(a, []Member{{Name: "x.o", Data: []byte("new")}})
	if len(got) != 2 || string(got[0].Data) != "new" || got[1].Name != "y.o" {
		t.Fatalf("members = %#v", got)
	}
}
