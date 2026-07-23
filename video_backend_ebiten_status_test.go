//go:build !headless

package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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

// TestStatusBarCacheKey_TracksEverythingDrawn pins the cache key against the
// things the bar renders. A key that missed a change would leave a stale
// overlay on screen, which is worse than the cost the cache saves.
func TestStatusBarCacheKey_TracksEverythingDrawn(t *testing.T) {
	cpu := []statusToken{{name: "IE64", enabled: true}}
	video := []statusToken{{name: "IEVID", enabled: false}}
	audio := []statusToken{{name: "SID", enabled: true}}
	legend := []statusToken{{name: "F11", enabled: false}}

	base := statusBarCacheKey(640, 44, cpu, video, audio, legend)
	if same := statusBarCacheKey(640, 44, cpu, video, audio, legend); same != base {
		t.Fatal("identical content produced a different key, so the bar would redraw every frame")
	}

	cases := []struct {
		name string
		key  string
	}{
		{"width", statusBarCacheKey(800, 44, cpu, video, audio, legend)},
		{"height", statusBarCacheKey(640, 22, cpu, video, audio, legend)},
		{"a token toggling on", statusBarCacheKey(640, 44, cpu,
			[]statusToken{{name: "IEVID", enabled: true}}, audio, legend)},
		{"a token renamed", statusBarCacheKey(640, 44,
			[]statusToken{{name: "IE32", enabled: true}}, video, audio, legend)},
		{"a token added", statusBarCacheKey(640, 44, cpu, video,
			append(append([]statusToken(nil), audio...), statusToken{name: "PSG"}), legend)},
		{"the legend changing", statusBarCacheKey(640, 44, cpu, video, audio,
			[]statusToken{{name: "F11", enabled: true}})},
	}
	for _, tc := range cases {
		if tc.key == base {
			t.Fatalf("%s did not change the key, so the cached bar would go stale", tc.name)
		}
	}
}

// TestStatusBar_CachedImageReusedAcrossFrames pins that the bar is rendered
// once and reused while nothing changes, which is the whole point.
func TestStatusBar_CachedImageReusedAcrossFrames(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = true
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 320, Height: 200, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		t.Fatalf("SetDisplayConfig: %v", err)
	}

	screen := ebiten.NewImage(320, 200)
	eo.drawRuntimeStatusBar(screen)
	first := eo.statusBarImage
	firstKey := eo.statusBarKey
	if first == nil {
		t.Fatal("the status bar was not rendered")
	}
	for range 5 {
		eo.drawRuntimeStatusBar(screen)
	}
	if eo.statusBarImage != first {
		t.Fatal("the status bar image was rebuilt with nothing changed")
	}
	if eo.statusBarKey != firstKey {
		t.Fatalf("the cache key moved with nothing changed: %q then %q", firstKey, eo.statusBarKey)
	}

	// A geometry change must rebuild it.
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 640, Height: 400, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		t.Fatalf("SetDisplayConfig: %v", err)
	}
	wide := ebiten.NewImage(640, 400)
	eo.drawRuntimeStatusBar(wide)
	if eo.statusBarImage == first {
		t.Fatal("the status bar survived a resize, so it would be drawn at the wrong width")
	}
}
