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
		"video.get_crt_mode()",
		"video.set_crt_mode('flat')",
		"video.cycle_crt_mode()",
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

func TestIEScriptCRTModeControlsMatchF7Cycle(t *testing.T) {
	out := &mockCRTControlOutput{mockVideoOutput: newMockVideoOutput(), mode: "flat"}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(out), NewTerminalMMIO())
	if err := se.RunString(`
		if video.get_crt_mode() ~= "flat" then error("CRT should start flat") end
		if video.cycle_crt_mode() ~= "curved" then error("first CRT cycle should be curved") end
		if video.get_crt_mode() ~= "curved" then error("curved CRT mode was not selected") end
		if not video.is_crt_enabled() then error("curved CRT should be enabled") end
		if video.cycle_crt_mode() ~= "off" then error("second CRT cycle should be off") end
		if video.is_crt_enabled() then error("off CRT should be disabled") end
		video.set_crt_mode("flat")
		if video.get_crt_mode() ~= "flat" then error("set_crt_mode did not select flat") end
	`, "crt-mode-control"); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if got := out.mode; got != "flat" {
		t.Fatalf("final CRT mode = %q, want flat", got)
	}
}

func TestIEScriptCRTModeControlsRejectInvalidMode(t *testing.T) {
	out := &mockCRTControlOutput{mockVideoOutput: newMockVideoOutput(), mode: "flat"}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(out), NewTerminalMMIO())
	if err := se.RunString(`video.set_crt_mode("barrel")`, "invalid-crt-mode"); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil || !strings.Contains(err.Error(), "invalid CRT mode") {
		t.Fatalf("LastError = %v, want invalid-mode Lua error", err)
	}
}

type mockCRTControlOutput struct {
	*mockVideoOutput
	mode string
}

func (m *mockCRTControlOutput) crtIsRequested() bool { return m.mode != "off" }

func (m *mockCRTControlOutput) setCRTRequested(enabled bool) {
	if enabled {
		m.mode = "flat"
		return
	}
	m.mode = "off"
}

func (m *mockCRTControlOutput) toggleCRTRequested() bool {
	m.setCRTRequested(!m.crtIsRequested())
	return m.crtIsRequested()
}

func (m *mockCRTControlOutput) crtModeRequested() string { return m.mode }

func (m *mockCRTControlOutput) setCRTModeRequested(mode string) bool {
	switch mode {
	case "flat", "curved", "off":
		m.mode = mode
		return true
	default:
		return false
	}
}

func (m *mockCRTControlOutput) cycleCRTModeRequested() string {
	switch m.mode {
	case "flat":
		m.mode = "curved"
	case "curved":
		m.mode = "off"
	default:
		m.mode = "flat"
	}
	return m.mode
}
