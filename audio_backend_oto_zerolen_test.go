//go:build !headless

package main

import "testing"

func TestOtoPlayerZeroLengthRead(t *testing.T) {
	op := &OtoPlayer{}
	op.chip.Store(&SoundChip{})
	if n, err := op.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil)=(%d,%v), want (0,nil)", n, err)
	}
	if n, err := op.Read([]byte{}); n != 0 || err != nil {
		t.Fatalf("Read(empty)=(%d,%v), want (0,nil)", n, err)
	}
}
