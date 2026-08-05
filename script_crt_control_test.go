package main

import (
	"strings"
	"testing"
)

func TestIEScriptCRTControlsRejectNilCompositor(t *testing.T) {
	for _, call := range []string{
		"video.is_crt_enabled()",
		"video.set_crt_enabled(false)",
		"video.toggle_crt()",
	} {
		t.Run(call, func(t *testing.T) {
			se := NewScriptEngine(NewMachineBus(), nil, NewTerminalMMIO())
			if err := se.RunString(call, "nil-compositor-crt-control"); err != nil {
				t.Fatalf("RunString: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil || !strings.Contains(err.Error(), "host CRT control is unavailable") {
				t.Fatalf("LastError = %v, want unavailable-control Lua error", err)
			}
		})
	}
}
