//go:build headless && basiclong

package main

import (
	"hash/crc32"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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

func TestREPL_ResonanceDemoRunAOTMaintainsFrameCadence(t *testing.T) {
	if testing.Short() {
		t.Skip("BASIC graphics demos enter long render loops")
	}
	testBasicIEVIDDemoMaintainsFrameCadence(t, "resonance.bas", "RUN AOT")
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
	testBasicIEVIDDemoMaintainsFrameCadence(t, demoName, "RUN")
}

func testBasicIEVIDDemoMaintainsFrameCadence(t *testing.T, demoName string, command string) {
	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.cpu.jitEnabled = true
	helperTrace := measureJITHelpers(h.cpu)
	video := mapMeasuredIEVIDForBasicGraphicsTest(t, h.bus)
	mapMediaLoaderForBasicGraphicsTest(t, h.bus, repo)

	if out := h.runCommand(`LOAD "` + filepath.ToSlash(filepath.Join("sdk", "examples", "basic", demoName)) + `"`); strings.Contains(out, "ERROR") {
		t.Fatalf("LOAD %s failed: %q", demoName, out)
	}

	h.sendInput(command + "\n")
	visible := false
	h.pumpUntil(func() bool {
		visible = rgbaFrameHasNonBlack(video.GetFrame())
		return visible
	}, 120*time.Second)
	if !visible {
		out := h.readOutput()
		t.Fatalf("%s %s did not reach a non-black IEVID frame; ctrl=%#x status=%#x output=%q",
			command, demoName, video.HandleRead(VIDEO_CTRL), video.HandleRead(VIDEO_STATUS), out)
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
	fbBase := video.HandleRead(VIDEO_FB_BASE)
	fbPage := fbBase >> 8
	fbMappedIO := fbPage < uint32(len(h.bus.ioPageBitmap)) && h.bus.ioPageBitmap[fbPage]
	t.Logf("%s %s frame hash changes=%d in %s; fb=%#x fbMappedIO=%t mem=%d ioBitmap=%d; JIT={tier1:%d regions:%d candidates:%d rejected:%d direct_ram:%d io_bails:%d io_bail_ops:%s helper_resumes:%d resume_cancels:%d invalidations:%d helpers:%s helper_pages:%s}; blits=%s",
		command, demoName, hashChanges, sampleWindow,
		fbBase, fbMappedIO, len(h.cpu.memory), len(h.bus.ioPageBitmap),
		diff.tier1Blocks, diff.regions, diff.regionCandidates, diff.regionRejected,
		diff.directRAMProofs, diff.ioBails, ioBailOpcodeSummary(diff), diff.helperResumes, diff.helperResumeCancels, diff.invalidations,
		helperExitSummary(diff), helperTrace.summary(), video.summary())
	if hashChanges < 25 {
		t.Fatalf("%s %s produced %d frame hash changes in %s, want at least 25; invalidations=%d",
			command, demoName, hashChanges, sampleWindow, diff.invalidations)
	}
	if diff.invalidations > 100 {
		t.Fatalf("%s %s caused %d JIT invalidations in %s, want <= 100", command, demoName, diff.invalidations, sampleWindow)
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

type measuredVideoChip struct {
	*VideoChip
	mu     sync.Mutex
	counts map[uint32]int
	timeNS map[uint32]int64
}

type measuredJITHelpers struct {
	mu    sync.Mutex
	pages map[string]uint64
}

func measureJITHelpers(cpu *CPU64) *measuredJITHelpers {
	m := &measuredJITHelpers{pages: make(map[string]uint64)}
	cpu.jitHelperTrace = func(op uint32, addr uint64) {
		if op != HELPER_LOAD && op != HELPER_STORE {
			return
		}
		key := helperName(op) + "@" + formatHelperAddr(addr)
		m.mu.Lock()
		m.pages[key]++
		m.mu.Unlock()
	}
	return m
}

func (m *measuredJITHelpers) summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pages) == 0 {
		return "none"
	}
	type row struct {
		key   string
		count uint64
	}
	rows := make([]row, 0, len(m.pages))
	for key, count := range m.pages {
		rows = append(rows, row{key: key, count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].count > rows[j].count
	})
	if len(rows) > 8 {
		rows = rows[:8]
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(r.key)
		b.WriteString("=")
		b.WriteString(strconv.FormatUint(r.count, 10))
	}
	return b.String()
}

func helperName(op uint32) string {
	switch op {
	case HELPER_LOAD:
		return "load"
	case HELPER_STORE:
		return "store"
	default:
		return "h" + strconv.FormatUint(uint64(op), 10)
	}
}

func formatHelperAddr(addr uint64) string {
	return "0x" + strconv.FormatUint(addr, 16)
}

func mapMeasuredIEVIDForBasicGraphicsTest(t *testing.T, bus *MachineBus) *measuredVideoChip {
	t.Helper()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(false)
	m := &measuredVideoChip{
		VideoChip: video,
		counts:    make(map[uint32]int),
		timeNS:    make(map[uint32]int64),
	}
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, func(addr uint32, value uint32) {
		op := uint32(0)
		started := addr == BLT_CTRL && value&bltCtrlStart != 0
		if started {
			op = video.HandleRead(BLT_OP)
		}
		start := time.Now()
		video.HandleWrite(addr, value)
		if started {
			m.mu.Lock()
			m.counts[op]++
			m.timeNS[op] += time.Since(start).Nanoseconds()
			m.mu.Unlock()
		}
	})
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	return m
}

func (m *measuredVideoChip) summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.counts) == 0 {
		return "none"
	}
	order := []uint32{bltOpCopy, bltOpFill, bltOpAlphaCopy, bltOpMode7, bltOpMemcopy}
	var b strings.Builder
	for _, op := range order {
		count := m.counts[op]
		if count == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteString(blitOpName(op))
		b.WriteString("=")
		b.WriteString(formatBlitTiming(count, time.Duration(m.timeNS[op])))
	}
	return b.String()
}

func blitOpName(op uint32) string {
	switch op {
	case bltOpCopy:
		return "copy"
	case bltOpFill:
		return "fill"
	case bltOpAlphaCopy:
		return "alpha"
	case bltOpMode7:
		return "mode7"
	case bltOpMemcopy:
		return "memcopy"
	default:
		return "op" + strconv.FormatUint(uint64(op), 10)
	}
}

func formatBlitTiming(count int, d time.Duration) string {
	if count == 0 {
		return "0"
	}
	return strconv.Itoa(count) + "/" + d.String()
}

func helperExitSummary(stats ie64JITStatsSnapshot) string {
	names := map[int]string{
		int(HELPER_LOAD):    "load",
		int(HELPER_STORE):   "store",
		int(HELPER_FLOAD):   "fload",
		int(HELPER_FSTORE):  "fstore",
		int(HELPER_DLOAD):   "dload",
		int(HELPER_DSTORE):  "dstore",
		int(HELPER_PUSH):    "push",
		int(HELPER_POP):     "pop",
		int(HELPER_JSR):     "jsr",
		int(HELPER_RTS):     "rts",
		int(HELPER_JSR_IND): "jsr_ind",
		int(HELPER_DTRANS):  "dtrans",
	}
	var b strings.Builder
	for i, count := range stats.helperExits {
		if count == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(",")
		}
		name := names[i]
		if name == "" {
			name = "h" + strconv.Itoa(i)
		}
		b.WriteString(name)
		b.WriteString("=")
		b.WriteString(strconv.FormatUint(count, 10))
	}
	if b.Len() == 0 {
		return "none"
	}
	return b.String()
}

func ioBailOpcodeSummary(stats ie64JITStatsSnapshot) string {
	type row struct {
		op    byte
		count uint64
	}
	rows := make([]row, 0, 8)
	for op, count := range stats.ioBailOpcodes {
		if count != 0 {
			rows = append(rows, row{op: byte(op), count: count})
		}
	}
	if len(rows) == 0 {
		return "none"
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].count > rows[j].count
	})
	if len(rows) > 6 {
		rows = rows[:6]
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		name := ie64OpcodeNames[r.op]
		if name == "" {
			name = "op" + strconv.FormatUint(uint64(r.op), 16)
		}
		b.WriteString(name)
		b.WriteString("=")
		b.WriteString(strconv.FormatUint(r.count, 10))
	}
	return b.String()
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
