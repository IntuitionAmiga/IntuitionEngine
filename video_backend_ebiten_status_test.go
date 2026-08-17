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

func TestEbitenCRTOffByDefaultAndF7Decision(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	if eo.crtMode != crtModeOff || eo.crtState != crtFilterUninitialised {
		t.Fatalf("new CRT mode = %v state:%v, want off and uninitialised", eo.crtMode, eo.crtState)
	}
	if decideEbitenF7Action(false) {
		t.Fatal("F7 action fired without a just-pressed F7")
	}
	if !decideEbitenF7Action(true) {
		t.Fatal("F7 action did not fire for a just-pressed F7")
	}

	eo.cycleCRTMode()
	if eo.crtMode != crtModeFlat {
		t.Fatal("first F7 transition did not enable flat CRT state")
	}
	eo.cycleCRTMode()
	if eo.crtMode != crtModeCurved {
		t.Fatal("second F7 transition did not enable curved CRT state")
	}
	eo.cycleCRTMode()
	if eo.crtMode != crtModeOff {
		t.Fatal("third F7 transition did not disable requested CRT state")
	}
}

func TestCRTPresentationModeCyclesFlatCurvedOff(t *testing.T) {
	mode := crtModeFlat
	for _, want := range []crtPresentationMode{crtModeCurved, crtModeOff, crtModeFlat} {
		mode = mode.next()
		if mode != want {
			t.Fatalf("F7 CRT mode = %v, want %v", mode, want)
		}
	}
}

func TestEbitenCRTModeControllerUsesF7Cycle(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	for _, want := range []string{"flat", "curved", "off"} {
		if got := eo.cycleCRTModeRequested(); got != want {
			t.Fatalf("IEScript CRT cycle = %q, want %q", got, want)
		}
	}
	if !eo.setCRTModeRequested("curved") {
		t.Fatal("setCRTModeRequested rejected curved")
	}
	if got := eo.crtModeRequested(); got != "curved" {
		t.Fatalf("selected CRT mode = %q, want curved", got)
	}
	if eo.setCRTModeRequested("barrel") {
		t.Fatal("setCRTModeRequested accepted an invalid CRT mode")
	}
}

func TestCRTPresentationModeNamesAreBrowserStable(t *testing.T) {
	for mode, want := range map[crtPresentationMode]string{
		crtModeFlat:   "flat",
		crtModeCurved: "curved",
		crtModeOff:    "off",
	} {
		if got := mode.String(); got != want {
			t.Fatalf("CRT mode %v name = %q, want %q", mode, got, want)
		}
	}
}

func TestCRTPresentationStateDistinguishesUnavailableFilter(t *testing.T) {
	tests := []struct {
		mode      crtPresentationMode
		effective bool
		want      string
	}{
		{crtModeFlat, true, "flat-active"},
		{crtModeCurved, true, "curved-active"},
		{crtModeFlat, false, "flat-unavailable"},
		{crtModeCurved, false, "curved-unavailable"},
		{crtModeOff, false, "off"},
	}
	for _, test := range tests {
		if got := crtPresentationState(test.mode, test.effective); got != test.want {
			t.Errorf("crtPresentationState(%s, %t) = %q, want %q", test.mode, test.effective, got, test.want)
		}
	}
}

func TestCRTModeKeepsScriptBooleanContract(t *testing.T) {
	if !crtModeFlat.enabled() || !crtModeCurved.enabled() || crtModeOff.enabled() {
		t.Fatal("CRT mode enabled mapping is wrong")
	}
	if got := crtModeFromEnabled(true); got != crtModeFlat {
		t.Fatalf("script CRT true mode = %v, want flat", got)
	}
	if got := crtModeFromEnabled(false); got != crtModeOff {
		t.Fatalf("script CRT false mode = %v, want off", got)
	}

	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	eo.crtMode = crtModeCurved
	eo.setCRTRequested(true)
	if eo.crtMode != crtModeFlat {
		t.Fatalf("script CRT true selected mode %v, want flat", eo.crtMode)
	}
	eo.crtMode = crtModeCurved
	if enabled := eo.toggleCRTRequested(); enabled || eo.crtMode != crtModeOff {
		t.Fatalf("script CRT toggle from curved = enabled:%v mode:%v, want off", enabled, eo.crtMode)
	}
}

func TestEbitenF7StillHasGuestScancodes(t *testing.T) {
	if got := ebitenToSTScancode[ebiten.KeyF7]; got != 0x41 {
		t.Fatalf("ST F7 scancode = 0x%02X, want 0x41", got)
	}
	if got := ebitenToAmigaRawkey[ebiten.KeyF7]; got != 0x56 {
		t.Fatalf("Amiga F7 scancode = 0x%02X, want 0x56", got)
	}
}

func TestStatusBarCacheKeyTracksCRTLegendState(t *testing.T) {
	cpu := []statusToken{{name: "IE64", enabled: true}}
	video := []statusToken{{name: "IEVID", enabled: true}}
	audio := []statusToken{{name: "SID", enabled: false}}
	flat := ebitenStatusLegendTokens(false, false, ScaleAspectFit, crtModeFlat, true)
	curved := ebitenStatusLegendTokens(false, false, ScaleAspectFit, crtModeCurved, true)
	off := ebitenStatusLegendTokens(false, false, ScaleAspectFit, crtModeOff, false)
	flatUnavailable := ebitenStatusLegendTokens(false, false, ScaleAspectFit, crtModeFlat, false)
	if statusBarCacheKey(640, 44, cpu, video, audio, flat) == statusBarCacheKey(640, 44, cpu, video, audio, curved) ||
		statusBarCacheKey(640, 44, cpu, video, audio, curved) == statusBarCacheKey(640, 44, cpu, video, audio, off) ||
		statusBarCacheKey(640, 44, cpu, video, audio, flat) == statusBarCacheKey(640, 44, cpu, video, audio, flatUnavailable) {
		t.Fatal("CRT legend mode did not invalidate the status-bar cache")
	}
}

func TestEbitenStatusLegendShowsUnavailableCRTAsDisabled(t *testing.T) {
	for _, mode := range []crtPresentationMode{crtModeFlat, crtModeCurved} {
		tokens := ebitenStatusLegendTokens(false, false, ScaleAspectFit, mode, false)
		index := statusTokenIndex(tokens, "F7:CRT")
		if index < 0 || tokens[index].enabled || tokens[index].colour != statusTokenColourDefault {
			t.Fatalf("unavailable %s CRT legend token = %+v, want disabled grey", mode, tokens)
		}
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
	normal := ebitenStatusLegendTokens(false, true, ScaleAspectFit, crtModeFlat, true)
	crtIdx := statusTokenIndex(normal, "F7:CRT")
	if crtIdx < 0 || !normal[crtIdx].enabled || normal[crtIdx].colour != statusTokenColourDefault {
		t.Fatalf("enabled CRT legend token missing or grey: %v", normal)
	}
	if statusTokenIndex(normal, "Shift+F11:Fullscreen/Windowed") < 0 {
		t.Fatalf("normal legend missing fullscreen/windowed token: %v", statusTokenNames(normal))
	}
	if statusTokenIndex(normal, "F11:") < 0 {
		t.Fatalf("scale-capable legend missing F11 scale token: %v", statusTokenNames(normal))
	}

	curved := ebitenStatusLegendTokens(true, true, ScaleStretchFill, crtModeCurved, true)
	crtIdx = statusTokenIndex(curved, "F7:CRT")
	if crtIdx < 0 || curved[crtIdx].colour != statusTokenColourBlue {
		t.Fatalf("curved CRT legend token missing or not blue: %v", curved)
	}

	locked := ebitenStatusLegendTokens(true, true, ScaleStretchFill, crtModeOff, false)
	crtIdx = statusTokenIndex(locked, "F7:CRT")
	if crtIdx < 0 || locked[crtIdx].enabled {
		t.Fatalf("disabled CRT legend token missing or green: %v", locked)
	}
	if statusTokenIndex(locked, "Shift+F11:Fullscreen/Windowed") >= 0 {
		t.Fatalf("locked legend should omit fullscreen/windowed token: %v", statusTokenNames(locked))
	}

	noScale := ebitenStatusLegendTokens(false, false, ScaleAspectFit, crtModeFlat, true)
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
	eo.drawRuntimeStatusBar(screen, crtModeFlat, true)
	first := eo.statusBarImage
	firstKey := eo.statusBarKey
	if first == nil {
		t.Fatal("the status bar was not rendered")
	}
	for range 5 {
		eo.drawRuntimeStatusBar(screen, crtModeFlat, true)
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
	eo.drawRuntimeStatusBar(wide, crtModeFlat, true)
	if eo.statusBarImage == first {
		t.Fatal("the status bar survived a resize, so it would be drawn at the wrong width")
	}
}
