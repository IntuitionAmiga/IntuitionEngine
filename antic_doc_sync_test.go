package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"
)

func TestANTICRegisterSurfaceDocSync(t *testing.T) {
	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`MapIO\(GTIA_BASE,\s*GTIA_END,`).Match(mainSrc) {
		t.Fatal("main.go must map GTIA_BASE through GTIA_END")
	}

	for _, file := range []string{
		"sdk/include/ie32.inc",
		"sdk/include/ie64.inc",
		"sdk/include/ie68.inc",
		"sdk/include/ie86.inc",
	} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if !bytes.Contains(data, []byte("GTIA_END")) ||
			!(bytes.Contains(data, []byte("0xF21FB")) || bytes.Contains(data, []byte("$F21FB"))) {
			t.Fatalf("%s does not document GTIA_END as F21FB", file)
		}
		if !bytes.Contains(data, []byte("HITCLR")) {
			t.Fatalf("%s does not document GTIA HITCLR", file)
		}
	}
}
