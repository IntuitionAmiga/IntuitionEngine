package ie64obj

import (
	"strings"
	"testing"
)

func TestParseRejectsStaleObject(t *testing.T) {
	b, err := (&Object{Sections: []Section{{Name: ".text", Type: SHTProgBits}}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b[48] = 2
	if _, err := Parse(b); err == nil || !strings.Contains(err.Error(), "stale IE64 compiler object") {
		t.Fatalf("Parse stale object error = %v, want explicit rebuild diagnostic", err)
	}
}
