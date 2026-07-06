//go:build headless && basiclong

package main

import (
	"hash/crc32"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestREPL_ResonanceBasicRunAOTProducesIEVIDPixels(t *testing.T) {
	testBasicIEVIDDemoProducesPixels(t, "resonance.bas", 120*time.Second)
}

func TestREPL_WobbleZoomBasicRunAOTProducesIEVIDPixels(t *testing.T) {
	testBasicIEVIDDemoProducesPixels(t, "wobble_zoom.bas", 90*time.Second)
}

func TestREPL_IEVIDDemoRunJITMaintainsFrameCadence(t *testing.T) {
	if testing.Short() {
		t.Skip("BASIC graphics demos enter long render loops")
	}
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	for _, demoName := range []string{"resonance.bas", "wobble_zoom.bas"} {
		t.Run(demoName, func(t *testing.T) {
			testBasicIEVIDDemoRunJITMaintainsFrameCadence(t, demoName)
		})
	}
}

func testBasicIEVIDDemoProducesPixels(t *testing.T, demoName string, timeout time.Duration) {
	t.Helper()
	if testing.Short() {
		t.Skip("RUN AOT enters the demo render loop")
	}

	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	video := mapIEVIDForBasicGraphicsTest(t, h.bus)
	mapMediaLoaderForBasicGraphicsTest(t, h.bus, repo)

	if out := h.runCommand(`LOAD "` + filepath.ToSlash(filepath.Join("sdk", "examples", "basic", demoName)) + `"`); strings.Contains(out, "ERROR") {
		t.Fatalf("LOAD %s failed: %q", demoName, out)
	}

	h.sendInput("RUN AOT\n")
	var capturedFrame []byte
	h.pumpUntil(func() bool {
		frame := video.GetFrame()
		if !rgbaFrameHasNonBlack(frame) {
			return false
		}
		capturedFrame = append(capturedFrame[:0], frame...)
		return true
	}, timeout)

	out := h.readOutput()
	if strings.Contains(out, "?COMPILE ERROR") || strings.Contains(out, "?SYNTAX ERROR") ||
		strings.Contains(out, "?FC ERROR") || strings.Contains(out, "?OUT OF MEMORY") ||
		strings.Contains(out, "out of compiler memory") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("RUN AOT %s failed: %q\n%s\n%s", demoName, out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	if got := video.HandleRead(VIDEO_CTRL); got&1 == 0 {
		t.Fatalf("%s left IEVID disabled; ctrl=%#x status=%#x output=%q", demoName, got, video.HandleRead(VIDEO_STATUS), out)
	}
	if got := video.HandleRead(VIDEO_MODE); got != MODE_640x480 {
		t.Fatalf("%s video mode=%#x, want MODE_640x480", demoName, got)
	}
	if got := video.HandleRead(VIDEO_COLOR_MODE); got != 0 {
		t.Fatalf("%s colour mode=%#x, want RGBA32", demoName, got)
	}
	if fb := video.HandleRead(VIDEO_FB_BASE); fb == 0 {
		t.Fatalf("%s VIDEO_FB_BASE is zero; status=%#x output=%q", demoName, video.HandleRead(VIDEO_STATUS), out)
	}
	if !rgbaFrameHasNonBlack(capturedFrame) {
		frame := video.GetFrame()
		if !rgbaFrameHasNonBlack(frame) {
			t.Fatalf("%s IEVID frame stayed black; fb=%#x status=%#x bltStatus=%#x blt={op:%#x src:%#x dst:%#x width:%#x height:%#x ctrl:%#x} state={line:%d error:%d errorLine:%d} output=%q",
				demoName,
				video.HandleRead(VIDEO_FB_BASE), video.HandleRead(VIDEO_STATUS), video.HandleRead(BLT_STATUS),
				video.HandleRead(BLT_OP), video.HandleRead(BLT_SRC), video.HandleRead(BLT_DST),
				video.HandleRead(BLT_WIDTH), video.HandleRead(BLT_HEIGHT), video.HandleRead(BLT_CTRL),
				h.bus.Read32(0x042000+0x200), h.bus.Read32(0x042000+0x208), h.bus.Read32(0x042000+0x228), out)
		}
	}
}

func testBasicIEVIDDemoRunJITMaintainsFrameCadence(t *testing.T, demoName string) {
	t.Helper()
	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.cpu.jitEnabled = true
	video := mapIEVIDForBasicGraphicsTest(t, h.bus)
	mapMediaLoaderForBasicGraphicsTest(t, h.bus, repo)

	if out := h.runCommand(`LOAD "` + filepath.ToSlash(filepath.Join("sdk", "examples", "basic", demoName)) + `"`); strings.Contains(out, "ERROR") {
		t.Fatalf("LOAD %s failed: %q", demoName, out)
	}

	h.sendInput("RUN\n")
	visible := false
	h.pumpUntil(func() bool {
		visible = rgbaFrameHasNonBlack(video.GetFrame())
		return visible
	}, 120*time.Second)
	if !visible {
		out := h.readOutput()
		t.Fatalf("RUN %s did not reach a non-black IEVID frame; ctrl=%#x status=%#x output=%q",
			demoName, video.HandleRead(VIDEO_CTRL), video.HandleRead(VIDEO_STATUS), out)
	}

	base := ie64JITStatsLoad()
	const sampleWindow = 5 * time.Second
	const sampleInterval = 25 * time.Millisecond
	start := time.Now()
	nextSample := start
	var lastHash uint32
	var haveHash bool
	hashChanges := 0
	h.pumpUntil(func() bool {
		now := time.Now()
		if now.Before(nextSample) {
			return now.Sub(start) >= sampleWindow
		}
		nextSample = now.Add(sampleInterval)
		frame := video.GetFrame()
		if rgbaFrameHasNonBlack(frame) {
			hash := crc32.ChecksumIEEE(frame)
			if haveHash && hash != lastHash {
				hashChanges++
			}
			lastHash = hash
			haveHash = true
		}
		return now.Sub(start) >= sampleWindow
	}, sampleWindow+2*time.Second)
	diff := ie64JITStatsLoad().Sub(base)
	if hashChanges < 25 {
		t.Fatalf("RUN %s produced %d frame hash changes in %s, want at least 25; invalidations=%d",
			demoName, hashChanges, sampleWindow, diff.invalidations)
	}
	if diff.invalidations > 100 {
		t.Fatalf("RUN %s caused %d JIT invalidations in %s, want <= 100", demoName, diff.invalidations, sampleWindow)
	}
}

func mapIEVIDForBasicGraphicsTest(t *testing.T, bus *MachineBus) *VideoChip {
	t.Helper()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(false)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	return video
}

func mapMediaLoaderForBasicGraphicsTest(t *testing.T, bus *MachineBus, root string) {
	t.Helper()
	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(bus)
	bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(bus, sound, root, nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
}

func rgbaFrameHasNonBlack(frame []byte) bool {
	for i := 0; i+3 < len(frame); i += BYTES_PER_PIXEL {
		if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
			return true
		}
	}
	return false
}
