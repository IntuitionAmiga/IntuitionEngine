//go:build !headless

package main

import "testing"

func TestRuntimeCPUStatusTokensIESPlacementAndState(t *testing.T) {
	tokens := runtimeCPUStatusTokens(runtimeStatusSnapshot{})
	iesIdx := statusTokenIndex(tokens, "IES")
	if iesIdx < 0 {
		t.Fatal("CPU status tokens missing IES")
	}
	if iesIdx < 2 || tokens[iesIdx-2].name != "6502" || tokens[iesIdx-1].name != "|" {
		t.Fatalf("IES placement = tokens[%d], want after 6502 separator; tokens=%v", iesIdx, statusTokenNames(tokens))
	}
	if tokens[iesIdx].enabled {
		t.Fatal("IES enabled without script engine")
	}

	idleScript := &ScriptEngine{}
	tokens = runtimeCPUStatusTokens(runtimeStatusSnapshot{scriptEngine: idleScript})
	iesIdx = statusTokenIndex(tokens, "IES")
	if tokens[iesIdx].enabled {
		t.Fatal("IES enabled while script engine is idle")
	}

	runningScript := &ScriptEngine{}
	runningScript.running.Store(true)
	tokens = runtimeCPUStatusTokens(runtimeStatusSnapshot{scriptEngine: runningScript})
	iesIdx = statusTokenIndex(tokens, "IES")
	if !tokens[iesIdx].enabled {
		t.Fatal("IES disabled while script engine is running")
	}
}

func TestEbitenF11ActionSwapAndFullscreenLock(t *testing.T) {
	tests := []struct {
		name                 string
		shift                bool
		lockFullscreen       bool
		scaleToggleAvailable bool
		want                 ebitenF11Action
	}{
		{name: "plain F11 toggles scale when available", scaleToggleAvailable: true, want: ebitenF11ActionToggleScale},
		{name: "plain F11 ignored without scale toggle", want: ebitenF11ActionNone},
		{name: "shift F11 toggles fullscreen when unlocked", shift: true, scaleToggleAvailable: true, want: ebitenF11ActionToggleFullscreen},
		{name: "shift F11 ignored when locked", shift: true, lockFullscreen: true, scaleToggleAvailable: true, want: ebitenF11ActionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideEbitenF11Action(true, tt.shift, tt.lockFullscreen, tt.scaleToggleAvailable)
			if got != tt.want {
				t.Fatalf("action = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEbitenDisplayConfigLockFullscreenSticky(t *testing.T) {
	eo := &EbitenOutput{}
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 320, Height: 240, LockFullscreen: true}); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
	}
	got := eo.GetDisplayConfig()
	if !got.LockFullscreen || !got.Fullscreen {
		t.Fatalf("locked config = LockFullscreen %v Fullscreen %v, want both true", got.LockFullscreen, got.Fullscreen)
	}

	if err := eo.SetDisplayConfig(DisplayConfig{Width: 320, Height: 240, Fullscreen: false}); err != nil {
		t.Fatalf("second SetDisplayConfig returned error: %v", err)
	}
	got = eo.GetDisplayConfig()
	if !got.LockFullscreen || !got.Fullscreen {
		t.Fatalf("sticky locked config = LockFullscreen %v Fullscreen %v, want both true", got.LockFullscreen, got.Fullscreen)
	}
}

func TestEbitenStatusLegendFullscreenLockAndScaleTokens(t *testing.T) {
	normal := ebitenStatusLegendTokens(false, true, ScaleAspectFit)
	if statusTokenIndex(normal, "Shift+F11:Fullscreen/Windowed") < 0 {
		t.Fatalf("normal legend missing fullscreen/windowed token: %v", statusTokenNames(normal))
	}
	if statusTokenIndex(normal, "F11:") < 0 {
		t.Fatalf("scale-capable legend missing F11 scale token: %v", statusTokenNames(normal))
	}

	locked := ebitenStatusLegendTokens(true, true, ScaleStretchFill)
	if statusTokenIndex(locked, "Shift+F11:Fullscreen/Windowed") >= 0 {
		t.Fatalf("locked legend should omit fullscreen/windowed token: %v", statusTokenNames(locked))
	}

	noScale := ebitenStatusLegendTokens(false, false, ScaleAspectFit)
	if statusTokenIndex(noScale, "F11:") >= 0 {
		t.Fatalf("non-scale legend should omit F11 scale token: %v", statusTokenNames(noScale))
	}
}

func statusTokenIndex(tokens []statusToken, name string) int {
	for i, token := range tokens {
		if token.name == name {
			return i
		}
	}
	return -1
}

func statusTokenNames(tokens []statusToken) []string {
	names := make([]string, len(tokens))
	for i, token := range tokens {
		names[i] = token.name
	}
	return names
}
