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

func TestOtoPlayer_Read_ShortBuffer(t *testing.T) {
	for _, size := range []int{1, 2, 3} {
		t.Run("len", func(t *testing.T) {
			op := &OtoPlayer{}
			op.chip.Store(&SoundChip{})
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = 0xA5
			}
			n, err := op.Read(buf)
			if err != nil || n != len(buf) {
				t.Fatalf("Read len=%d returned (%d,%v), want (%d,nil)", size, n, err, len(buf))
			}
			for i, b := range buf {
				if b != 0 {
					t.Fatalf("buf[%d]=0x%02X, want zero-filled silence", i, b)
				}
			}
		})
	}
}

func TestOtoPlayer_Read_NonMod4(t *testing.T) {
	op := &OtoPlayer{}
	op.chip.Store(&SoundChip{})
	buf := []byte{0xA5, 0xA5, 0xA5, 0xA5, 0xA5, 0xA5, 0xA5}
	n, err := op.Read(buf)
	if err != nil || n != len(buf) {
		t.Fatalf("Read returned (%d,%v), want (%d,nil)", n, err, len(buf))
	}
	for i := 4; i < len(buf); i++ {
		if buf[i] != 0 {
			t.Fatalf("trailing buf[%d]=0x%02X, want zero-filled tail", i, buf[i])
		}
	}
}

func TestOtoPlayer_Read_NilChip(t *testing.T) {
	op := &OtoPlayer{}
	buf := []byte{0xA5, 0xA5, 0xA5, 0xA5, 0xA5, 0xA5, 0xA5, 0xA5}
	n, err := op.Read(buf)
	if err != nil || n != len(buf) {
		t.Fatalf("Read returned (%d,%v), want (%d,nil)", n, err, len(buf))
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("buf[%d]=0x%02X, want zero-filled silence", i, b)
		}
	}
}

func TestOtoPlayer_StopThenClose_NoPanic(t *testing.T) {
	op, err := NewOtoPlayer(44100)
	if err != nil {
		t.Skipf("oto unavailable: %v", err)
	}
	op.SetupPlayer(&SoundChip{})
	op.Start()
	op.Stop()
	op.Close()
	op.Close()
}

func TestOtoPlayer_Stop_LeavesPlayerReusable(t *testing.T) {
	op, err := NewOtoPlayer(44100)
	if err != nil {
		t.Skipf("oto unavailable: %v", err)
	}
	op.SetupPlayer(&SoundChip{})
	op.Start()
	op.Stop()
	op.Start()
	if !op.IsStarted() {
		t.Fatal("player was not reusable after Stop")
	}
	op.Close()
}
