package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	iesRotoTextureBase = 0x600000
	iesRotoBackBuffer  = 0x900000
	iesRotoMIDIBase    = 0xA40000
	iesRotoWidth       = 640
	iesRotoHeight      = 480
)

type midiMMIOWrite struct {
	addr  uint32
	value uint32
}

func runBoundedIESRotozoomer(t *testing.T, bus *MachineBus, se *ScriptEngine, frames int, disableAudio bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("sdk", "scripts", "rotozoomer_ies.ies"))
	if err != nil {
		t.Fatalf("read rotozoomer_ies.ies: %v", err)
	}
	prefix := "IE_ROTO_MAX_FRAMES = " + strconv.Itoa(frames) + "\n"
	if disableAudio {
		prefix += "IE_ROTO_DISABLE_AUDIO = true\n"
	}
	if err := se.RunString(prefix+string(data), filepath.Join("sdk", "scripts", "rotozoomer_ies.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count leaked: %d", got)
	}
	_ = bus
}

func TestIESRotozoomerReferencesCommittedAssets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("sdk", "scripts", "rotozoomer_ies.ies"))
	if err != nil {
		t.Fatalf("read rotozoomer_ies.ies: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{"../IntuitionSubtractor", "IntuitionSubtractor"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("script references non-SDK source %q", forbidden)
		}
	}
	if strings.Contains(text, "audio.midi_") {
		t.Fatal("script must use MIDI MMIO, not audio.midi_* helpers")
	}
	for _, path := range []string{
		filepath.Join("sdk", "examples", "assets", "rotozoomtexture_ies.raw"),
		filepath.Join("sdk", "examples", "assets", "music", "yourlove.mid"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("script asset %s missing: %v", path, err)
		}
	}
}

func TestIESRotozoomerVideoSmoke(t *testing.T) {
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.Write32(BLT_FLAGS, 0x10)

	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	runBoundedIESRotozoomer(t, bus, se, 1, true)

	if got := bus.Read32(VIDEO_CTRL); got != 1 {
		t.Fatalf("VIDEO_CTRL = %#x, want 1", got)
	}
	if got := bus.Read32(VIDEO_MODE); got != MODE_640x480 {
		t.Fatalf("VIDEO_MODE = %#x, want MODE_640x480", got)
	}
	if got := bus.Read32(VIDEO_COLOR_MODE); got != 0 {
		t.Fatalf("VIDEO_COLOR_MODE = %#x, want RGBA", got)
	}
	if got := bus.Read32(VIDEO_FB_BASE); got != 0 {
		t.Fatalf("VIDEO_FB_BASE = %#x, want default front buffer", got)
	}
	if got := bus.Read32(BLT_MODE7_TEX_W); got != 255 {
		t.Fatalf("BLT_MODE7_TEX_W = %#x, want 255", got)
	}
	if got := bus.Read32(BLT_MODE7_TEX_H); got != 255 {
		t.Fatalf("BLT_MODE7_TEX_H = %#x, want 255", got)
	}
	if got := bus.Read32(BLT_FLAGS); got != 0 {
		t.Fatalf("BLT_FLAGS = %#x, want reset RGBA copy mode", got)
	}
	if got := bus.Read32(BLT_DST); got != VRAM_START {
		t.Fatalf("final blitter dst = %#x, want VRAM_START", got)
	}
	if got := bus.Read8(iesRotoTextureBase); got == 0 {
		t.Fatal("texture bytes were not staged at 0x600000")
	}
	if !framebufferHasNonZeroPixel(bus, iesRotoBackBuffer, iesRotoWidth, iesRotoHeight) {
		t.Fatal("back buffer stayed black after Mode7 blit")
	}
	if !videoFramebufferHasNonZeroPixel(video, VRAM_START, iesRotoWidth, iesRotoHeight) {
		t.Fatal("visible framebuffer stayed black after bounded rotozoomer frame")
	}
	if frame := video.FinishFrame(); !rgbaFrameHasNonZeroPixel(frame, iesRotoWidth, iesRotoHeight) {
		t.Fatal("presented frame stayed black after bounded rotozoomer frame")
	}
}

func TestIESRotozoomerMIDIMMIO(t *testing.T) {
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

	player := NewMIDIPlayer(nil, SAMPLE_RATE)
	player.AttachBus(bus)
	var writes []midiMMIOWrite
	bus.MapIO(MIDI_PLAY_PTR, MIDI_END, player.HandlePlayRead, func(addr, value uint32) {
		writes = append(writes, midiMMIOWrite{addr: addr, value: value})
		player.HandlePlayWrite(addr, value)
	})

	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	runBoundedIESRotozoomer(t, bus, se, 1, false)

	if got := bus.Read8(iesRotoMIDIBase); got != 'M' {
		t.Fatalf("MIDI data base first byte = %#x, want 'M'", got)
	}
	for _, want := range []uint32{MIDI_PLAY_PTR, MIDI_PLAY_LEN, MIDI_VOLUME} {
		if !sawMIDIWrite(writes, want, nil) {
			t.Fatalf("missing MIDI write to %#x", want)
		}
	}
	if !sawMIDIWrite(writes, MIDI_PLAY_CTRL, func(v uint32) bool { return v&1 != 0 }) {
		t.Fatal("missing MIDI start command")
	}
	if !sawMIDIWrite(writes, MIDI_PLAY_CTRL, func(v uint32) bool { return v&2 != 0 }) {
		t.Fatal("missing MIDI stop command")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for player.HandlePlayRead(MIDI_PLAY_STATUS)&MIDI_STATUS_LOADING != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
}

func sawMIDIWrite(writes []midiMMIOWrite, addr uint32, pred func(uint32) bool) bool {
	for _, write := range writes {
		if write.addr != addr {
			continue
		}
		if pred == nil || pred(write.value) {
			return true
		}
	}
	return false
}

func framebufferHasNonZeroPixel(bus *MachineBus, base uint32, width, height int) bool {
	const stride = 4
	for y := 0; y < height; y += 31 {
		for x := 0; x < width; x += 29 {
			if bus.Read32(base+uint32((y*width+x)*stride)) != 0 {
				return true
			}
		}
	}
	return false
}

func videoFramebufferHasNonZeroPixel(video *VideoChip, base uint32, width, height int) bool {
	const stride = 4
	for y := 0; y < height; y += 31 {
		for x := 0; x < width; x += 29 {
			if video.HandleRead(base+uint32((y*width+x)*stride)) != 0 {
				return true
			}
		}
	}
	return false
}

func rgbaFrameHasNonZeroPixel(frame []byte, width, height int) bool {
	const stride = 4
	for y := 0; y < height; y += 31 {
		for x := 0; x < width; x += 29 {
			off := (y*width + x) * stride
			if off+stride <= len(frame) && (frame[off] != 0 || frame[off+1] != 0 || frame[off+2] != 0 || frame[off+3] != 0) {
				return true
			}
		}
	}
	return false
}
