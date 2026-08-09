package main

import "testing"

func TestIEScriptPresentationScaleControls(t *testing.T) {
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(NewMachineBus(), comp, NewTerminalMMIO())
	if err := se.RunString(`
		if video.get_scale_mode() ~= "stretch" then error("default scale mode should be stretch") end
		video.set_scale_mode("fit")
		if video.get_scale_mode() ~= "fit" then error("set_scale_mode did not select fit") end
		video.set_scale_mode("stretch")
		if video.get_scale_mode() ~= "stretch" then error("set_scale_mode did not restore stretch") end
	`, "presentation-scale-control"); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}
