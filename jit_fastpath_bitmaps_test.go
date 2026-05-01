//go:build amd64 && linux

package main

import (
	"bytes"
	"testing"
)

func TestFastPathBitmapShapesMatchRuntimePolarity(t *testing.T) {
	tests := []struct {
		kind FastPathBitmapKind
		want FastPathBitmapShape
	}{
		{FPBitmapDenseRAM, FastPathBitmapShape{PageShift: 8, BitMeansFastPath: false}},
		{FPBitmapMMIO, FastPathBitmapShape{PageShift: 8, BitMeansFastPath: true}},
		{FPBitmapCodePageDirty, FastPathBitmapShape{PageShift: 8, BitMeansFastPath: false}},
		{FPBitmapZeroPageStyle, FastPathBitmapShape{PageShift: 8, BitMeansFastPath: false}},
	}
	for _, tt := range tests {
		got, ok := LookupFastPathBitmapShape(tt.kind)
		if !ok {
			t.Fatalf("LookupFastPathBitmapShape(%v) missing", tt.kind)
		}
		if got != tt.want {
			t.Fatalf("LookupFastPathBitmapShape(%v) = %+v, want %+v", tt.kind, got, tt.want)
		}
	}
}

func TestEmitAMD64FastPathBitmapProbe_DenseRAMBranchesOnClearByte(t *testing.T) {
	cb := &CodeBuffer{}
	off, ok := emitAMD64FastPathBitmapProbe(cb, FPBitmapDenseRAM, amd64R9, amd64RAX, amd64RCX, amd64RDX, true)
	if !ok {
		t.Fatal("emitAMD64FastPathBitmapProbe returned !ok")
	}
	if off != len(cb.Bytes())-4 {
		t.Fatalf("rel32 offset = %d, want %d", off, len(cb.Bytes())-4)
	}
	want := []byte{
		0x89, 0xC1, // MOV ECX, EAX
		0xC1, 0xE9, 0x08, // SHR ECX, 8
		0x41, 0x0F, 0xB6, 0x14, 0x09, // MOVZX EDX, BYTE [R9+RCX]
		0x85, 0xD2, // TEST EDX, EDX
		0x0F, 0x84, 0, 0, 0, 0, // JZ fast
	}
	if !bytes.Equal(cb.Bytes(), want) {
		t.Fatalf("emitted bytes:\n got % X\nwant % X", cb.Bytes(), want)
	}
}

func TestEmitAMD64FastPathBitmapProbe_DirtyCodeBranchesOnSetByteForSlow(t *testing.T) {
	cb := &CodeBuffer{}
	_, ok := emitAMD64FastPathBitmapProbe(cb, FPBitmapCodePageDirty, amd64RDX, amd64RAX, amd64RCX, amd64RCX, false)
	if !ok {
		t.Fatal("emitAMD64FastPathBitmapProbe returned !ok")
	}
	got := cb.Bytes()
	if len(got) < 6 {
		t.Fatalf("emitted probe too short: % X", got)
	}
	jcc := got[len(got)-6 : len(got)-4]
	if !bytes.Equal(jcc, []byte{0x0F, 0x85}) {
		t.Fatalf("tail Jcc = % X, want 0F 85 (JNZ slow)", jcc)
	}
}
