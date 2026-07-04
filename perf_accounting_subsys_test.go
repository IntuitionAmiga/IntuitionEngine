package main

import (
	"sync/atomic"
	"testing"
)

type perfAcctVideoSource struct {
	frame   []byte
	enabled atomic.Bool
}

func newPerfAcctVideoSource() *perfAcctVideoSource {
	src := &perfAcctVideoSource{frame: []byte{0x11, 0x22, 0x33, 0xff}}
	src.enabled.Store(true)
	return src
}

func (s *perfAcctVideoSource) GetFrame() []byte          { return s.frame }
func (s *perfAcctVideoSource) IsEnabled() bool           { return s.enabled.Load() }
func (s *perfAcctVideoSource) GetLayer() int             { return 0 }
func (s *perfAcctVideoSource) GetDimensions() (int, int) { return 1, 1 }
func (s *perfAcctVideoSource) SignalVSync()              {}

func TestSubsysAcct_VideoFramePath(t *testing.T) {
	withPerfAcct(t, true, func() {
		perfSubsysAcct.Reset()
		comp := NewVideoCompositor(nil)
		comp.RegisterSource(newPerfAcctVideoSource())

		comp.composite()

		snap := perfSubsysAcct.VideoFramePath.Snapshot()
		if snap.Ops != 1 {
			t.Fatalf("video ops = %d, want 1", snap.Ops)
		}
		if snap.Ns <= 0 {
			t.Fatalf("video ns = %d, want > 0", snap.Ns)
		}
	})
}

func TestSubsysAcct_AudioPull(t *testing.T) {
	withPerfAcct(t, true, func() {
		perfSubsysAcct.Reset()
		chip, err := NewSoundChip(AUDIO_BACKEND_NULL)
		if err != nil {
			t.Fatalf("NewSoundChip: %v", err)
		}

		dst := make([]float32, 16)
		if got := chip.ReadSamples(dst); got != len(dst) {
			t.Fatalf("ReadSamples = %d, want %d", got, len(dst))
		}

		snap := perfSubsysAcct.AudioPull.Snapshot()
		if snap.Ops != uint64(len(dst)) {
			t.Fatalf("audio ops = %d, want %d", snap.Ops, len(dst))
		}
		if snap.Ns <= 0 {
			t.Fatalf("audio ns = %d, want > 0", snap.Ns)
		}
	})
}

func TestSubsysAcct_BusSlowPathCount(t *testing.T) {
	withPerfAcct(t, true, func() {
		perfSubsysAcct.Reset()
		bus := NewMachineBus()
		bus.MapIO(0xF1000, 0xF1003,
			func(addr uint32) uint32 { return 0xAABBCCDD },
			func(addr uint32, value uint32) {},
		)

		bus.Write32(0xF1000, 0x11223344)
		if got := bus.Read32(0xF1000); got != 0xAABBCCDD {
			t.Fatalf("Read32 = 0x%08X, want 0xAABBCCDD", got)
		}

		writeSnap := perfSubsysAcct.BusWrite32Slow.Snapshot()
		readSnap := perfSubsysAcct.BusRead32Slow.Snapshot()
		if writeSnap.Ops != 1 {
			t.Fatalf("write slow ops = %d, want 1", writeSnap.Ops)
		}
		if readSnap.Ops != 1 {
			t.Fatalf("read slow ops = %d, want 1", readSnap.Ops)
		}
		if writeSnap.Ns <= 0 || readSnap.Ns <= 0 {
			t.Fatalf("slow path ns write=%d read=%d, want both > 0", writeSnap.Ns, readSnap.Ns)
		}
	})
}
