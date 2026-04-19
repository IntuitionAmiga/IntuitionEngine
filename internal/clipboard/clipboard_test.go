//go:build headless

package clipboard

import "testing"

func TestHeadlessClipboardRoundTrip(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cases := [][]byte{
		[]byte("foo"),
		[]byte(""),
		[]byte("hello\r\nworld"),
		[]byte("snowman: \xe2\x98\x83"),
	}

	for _, tc := range cases {
		if err := WriteText(tc); err != nil {
			t.Fatalf("WriteText(%q) error = %v", tc, err)
		}
		got, err := ReadText()
		if err != nil {
			t.Fatalf("ReadText() error = %v", err)
		}
		if string(got) != string(tc) {
			t.Fatalf("round-trip = %q, want %q", got, tc)
		}
	}
}
