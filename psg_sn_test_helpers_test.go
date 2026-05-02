package main

import "testing"

func assertSNEvents(t *testing.T, got []SNEvent, want []SNEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("SN event count: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SN event[%d]: got {Sample:%d Byte:0x%02X}, want {Sample:%d Byte:0x%02X}",
				i, got[i].Sample, got[i].Byte, want[i].Sample, want[i].Byte)
		}
	}
}

func assertPSGEvents(t *testing.T, got []PSGEvent, want []PSGEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("PSG event count: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PSG event[%d]: got {Sample:%d Reg:0x%02X Value:0x%02X}, want {Sample:%d Reg:0x%02X Value:0x%02X}",
				i, got[i].Sample, got[i].Reg, got[i].Value, want[i].Sample, want[i].Reg, want[i].Value)
		}
	}
}
