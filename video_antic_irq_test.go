package main

import "testing"

type recordingInterruptSink struct {
	pulses []InterruptMask
}

func (s *recordingInterruptSink) Pulse(mask InterruptMask) {
	s.pulses = append(s.pulses, mask)
}

func TestANTICNilInterruptSinkKeepsPollingOnlyPath(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.HandleWrite(ANTIC_NMIEN, ANTIC_NMIEN_VBI)

	antic.tickFrame(100)
	if got := antic.HandleRead(ANTIC_NMIST); got&ANTIC_NMIST_VBI == 0 {
		t.Fatalf("nil-sink tickFrame did not leave VBI pending in NMIST")
	}

	for i := 0; i < 1000; i++ {
		_ = antic.HandleRead(ANTIC_STATUS)
	}
	if got := antic.HandleRead(ANTIC_NMIST); got&ANTIC_NMIST_VBI == 0 {
		t.Fatalf("polling reads cleared VBI pending")
	}
}

func TestANTICInterruptSinkPulsesOncePerVBIFrame(t *testing.T) {
	antic := NewANTICEngine(nil)
	sink := &recordingInterruptSink{}
	antic.SetInterruptSink(sink)
	antic.HandleWrite(ANTIC_NMIEN, ANTIC_NMIEN_VBI)

	antic.tickFrame(100)
	if len(sink.pulses) != 1 || sink.pulses[0] != IntMaskVBI {
		t.Fatalf("pulses after first frame = %v, want [VBI]", sink.pulses)
	}

	for i := 0; i < 1000; i++ {
		_ = antic.HandleRead(ANTIC_STATUS)
	}
	if len(sink.pulses) != 1 {
		t.Fatalf("STATUS reads caused extra pulses: %v", sink.pulses)
	}

	antic.HandleWrite(ANTIC_NMIST, 0)
	if len(sink.pulses) != 1 {
		t.Fatalf("NMIRES caused pulse: %v", sink.pulses)
	}

	antic.tickFrame(200)
	if len(sink.pulses) != 2 || sink.pulses[1] != IntMaskVBI {
		t.Fatalf("pulses after second frame = %v, want two VBI pulses", sink.pulses)
	}
}

func TestANTICInterruptSinkSilentWhenNMIENDisabled(t *testing.T) {
	antic := NewANTICEngine(nil)
	sink := &recordingInterruptSink{}
	antic.SetInterruptSink(sink)

	antic.tickFrame(100)
	if len(sink.pulses) != 0 {
		t.Fatalf("disabled NMIEN pulses = %v, want none", sink.pulses)
	}
}
