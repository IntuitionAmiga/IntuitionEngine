package main

import (
	"bytes"
	"testing"
	"time"
)

func TestBreakInHotkeyListener_RequestsBreakIn(t *testing.T) {
	cpu := NewCPU64(NewMachineBus())
	adapter := NewDebugIE64(cpu)
	mon := NewMachineMonitor(nil)
	mon.RegisterCPU("IE64", adapter)

	listener := NewBreakInHotkeyListener(mon, bytes.NewReader([]byte{defaultBreakInHotkey}))
	listener.Start()

	deadline := time.After(500 * time.Millisecond)
	for !adapter.BreakInRequested() {
		select {
		case <-deadline:
			t.Fatal("hotkey did not request break-in")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
