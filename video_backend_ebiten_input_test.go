//go:build !headless

package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestClipboardPaste_Normalize(t *testing.T) {
	in := []byte("a\r\nb\rc\n")
	got := normalizePasteText(in)
	want := "a\nb\nc\n"
	if string(got) != want {
		t.Fatalf("expected %q, got %q", want, string(got))
	}
}

func TestClipboardPaste_Cap(t *testing.T) {
	in := make([]byte, 5000)
	got := capPasteText(in, 4096)
	if len(got) != 4096 {
		t.Fatalf("expected capped length 4096, got %d", len(got))
	}
}

func TestKeyTranslation_Enter(t *testing.T) {
	seq, ok := translateSpecialKey(ebiten.KeyEnter)
	if !ok {
		t.Fatal("expected enter translation")
	}
	if string(seq) != "\n" {
		t.Fatalf("expected newline for enter, got %v", seq)
	}
}

func TestKeyTranslation_ArrowLeft(t *testing.T) {
	seq, ok := translateSpecialKey(ebiten.KeyArrowLeft)
	if !ok {
		t.Fatal("expected arrow-left translation")
	}
	if len(seq) != 3 || seq[0] != 0x1B || seq[1] != '[' || seq[2] != 'D' {
		t.Fatalf("expected ESC[D, got %v", seq)
	}
}

func TestKeyTranslation_Printable(t *testing.T) {
	b, ok := runeToInputByte('a')
	if !ok {
		t.Fatal("expected printable translation")
	}
	if b != 0x61 {
		t.Fatalf("expected 0x61, got 0x%02X", b)
	}
}
