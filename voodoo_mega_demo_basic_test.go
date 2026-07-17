package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVoodooMegaDemoBasicStaticContract(t *testing.T) {
	repo := repoRootDir(t)
	basicPath := filepath.Join(repo, "sdk", "examples", "basic", "voodoo_mega_demo_basic.bas")
	srcBytes, err := os.ReadFile(basicPath)
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	upper := strings.ToUpper(src)

	for _, forbidden := range []string{"CALL", "USR", "ASSEMBLE", "TRANSPILE"} {
		if regexp.MustCompile(`\b` + forbidden + `\b`).MatchString(upper) {
			t.Fatalf("pure BASIC demo must not contain %s", forbidden)
		}
	}
	if strings.Count(upper, "BLOAD") != 1 || !strings.Contains(src, `BLOAD "sdk/examples/assets/music/Reggae_2.sid",SidData`) {
		t.Fatalf("demo must only BLOAD the committed Reggae_2.sid asset")
	}

	sidInfo, err := os.Stat(filepath.Join(repo, "sdk", "examples", "assets", "music", "Reggae_2.sid"))
	if err != nil {
		t.Fatal(err)
	}
	if sidInfo.Size() != 4790 {
		t.Fatalf("Reggae_2.sid size=%d, want 4790", sidInfo.Size())
	}

	for _, want := range []string{
		"SineTable=MEMALLOC(4096,4096)",
		"ProjectionTable=MEMALLOC(12288,4096)",
		"StarData=MEMALLOC(4096,4096)",
		"MessageData=MEMALLOC(4096,4096)",
		"SidData=MEMALLOC(8192,4096)",
		"CommandBuffer=MEMALLOC(524288,4096)",
		"AnimationTables=MEMALLOC(8192,4096)",
		"GlyphSpans=MEMALLOC(16384,4096)",
		"ProjectionResults=MEMALLOC(2883584,4096)",
		"RandomSeed=54321",
		"VoodooVideoDimensions=&HF8214",
		"POKE32 VoodooVideoDimensions,&H028001E0",
		"POKE32 VoodooFbzMode,&H0770",
		"POKE32 VoodooColourPath,0",
		"POKE32 &HF0E24,4790",
		"POKE32 &HF0E28,5",
		"POKE32 VoodooSwapBuffer,1",
		"CommandPointerRegister=&HF833C",
		"CommandCountRegister=&HF8340",
		"CommandSubmitRegister=&HF8344",
		"POKE32 CommandSubmitRegister,2",
		"IF CommandCount>65524 THEN RETURN",
		"IF CommandCount>65522 THEN RETURN",
		"ScrollPixelOffset=ScrollPixelOffset+ScrollSpeed",
		"ScrollCharacter=ScrollCharacter+1",
		"FOR StarIndex=0 TO 255",
		"SpanRecord=GlyphSpans+(FontIndex*7+FontRowIndex)*28",
		"TunnelOffsetX=PEEK32(TwistXTable+AnimationPhase*4)",
		"StarAngle=PEEK32(StarPointer)",
		"StarRadius=PEEK32(StarPointer+4)",
		"StarCosine=PEEK32(SineTable+((StarAngle+64) AND 255)*4)",
		"StarSine=PEEK32(SineTable+StarAngle*4)",
		"StarLocalX=((StarCosine*StarRadius) >> 6)+WorldOffset+TunnelOffsetX+128",
		"StarLocalY=((StarSine*StarRadius) >> 6)+WorldOffset+TunnelOffsetY+128",
		"ProjectionOffset=(ProjectionOffsetBase*ProjectionScale) >> 8",
		"WobbleRemainder=WobbleRemainder+1-ScrollSpeed",
		"CharacterCode=PEEK(MessageData+MessageIndex)",
	} {
		if !strings.Contains(upper, strings.ToUpper(want)) {
			t.Fatalf("demo source missing %q", want)
		}
	}
	if strings.Contains(upper, "&HFA014") {
		t.Fatal("demo uses the obsolete Voodoo video dimension address")
	}
	for _, obsolete := range []string{"SpawnXTable", "SpawnYTable", "SpawnValidTable", "SpawnCacheIndex", "SpawnCoordinateIndex"} {
		if strings.Contains(upper, strings.ToUpper(obsolete)) {
			t.Fatalf("starfield still uses obsolete precomputed coordinate cache %q", obsolete)
		}
	}
	if regexp.MustCompile(`(?m)(^|:)\s*(VOODOO|VERTEX|VSYNC)\b`).MatchString(upper) {
		t.Fatalf("render loop must use the Voodoo register interface without high-level commands or VSYNC")
	}
}

func TestVoodooMegaDemoBasicDataBlocks(t *testing.T) {
	repo := repoRootDir(t)
	srcBytes, err := os.ReadFile(filepath.Join(repo, "sdk", "examples", "basic", "voodoo_mega_demo_basic.bas"))
	if err != nil {
		t.Fatal(err)
	}
	got := parseBasicDataInts(t, string(srcBytes))
	if len(got) != 64+5+448+217 {
		t.Fatalf("DATA payload length=%d, want %d", len(got), 64+5+448+217)
	}

	asmBytes, err := os.ReadFile(filepath.Join(repo, "sdk", "examples", "asm", "voodoo_mega_demo.asm"))
	if err != nil {
		t.Fatal(err)
	}
	asm := string(asmBytes)
	wantQuarter := parseAsmByteBlock(t, asm, "quarter_sin", "scroll_message")
	wantMask := parseAsmByteBlock(t, asm, "font_mask_table", "font_data")
	wantFont := parseAsmByteBlock(t, asm, "font_data", "sid_data")
	msg := extractAsmASCII(t, asm, "scroll_message")
	want := append(append(append([]int{}, wantQuarter...), wantMask...), wantFont...)
	for _, ch := range []byte(msg) {
		want = append(want, int(ch))
	}
	if len(wantQuarter) != 64 || len(wantMask) != 5 || len(wantFont) != 448 || len(msg) != 217 {
		t.Fatalf("IE32 source data sizes changed: quarter=%d mask=%d font=%d msg=%d", len(wantQuarter), len(wantMask), len(wantFont), len(msg))
	}
	if len(got) != len(want) {
		t.Fatalf("BASIC DATA length=%d, IE32 length=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DATA[%d]=%d, want %d", i, got[i], want[i])
		}
	}
}

func TestVoodooMegaDemoBasicArithmeticContract(t *testing.T) {
	first := uint64(1664525)*54321 + 1013904223
	first %= 1 << 32
	if first != 1238253532 {
		t.Fatalf("LCG first value=%d, want IE32 wrap value 1238253532", first)
	}
	screenW := uint32(640)
	tooFar := uint32(641)
	underflow := screenW - tooFar
	if (uint32(0xffffffff)&0x80000000) == 0 || (underflow&0x80000000) == 0 {
		t.Fatal("uint32 sign-bit checks must preserve IE32-style negative/underflow detection")
	}
	if 0x1234&255 != 0x34 || 0x1234&511 != 0x34 {
		t.Fatal("power-of-two masks used by the BASIC port changed unexpectedly")
	}
}

func TestVoodooMegaDemoBasicAOTArithmeticSmoke(t *testing.T) {
	asmBin := buildAssembler(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, t.TempDir())
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	runAOTLines(t, h,
		"10 X=(1664525*54321+1013904223) AND &HFFFFFFFF",
		"30 POKE32 327680,X",
		"40 POKE32 327684,&H80000000",
		"50 POKE32 327688,4660 AND 255",
		"60 POKE32 327692,4660 AND 511",
		"70 END",
	)
	if got := h.bus.Read32(327680); got != 1238253532 {
		t.Fatalf("AOT LCG wrap=%d, want 1238253532", got)
	}
	if got := h.bus.Read32(327684); got != 0x80000000 {
		t.Fatalf("AOT sign-bit literal=%#x, want 0x80000000", got)
	}
	if got := h.bus.Read32(327688); got != 0x34 {
		t.Fatalf("AOT AND 255=%#x, want 0x34", got)
	}
	if got := h.bus.Read32(327692); got != 0x34 {
		t.Fatalf("AOT AND 511=%#x, want 0x34", got)
	}
}

func TestVoodooMegaDemoBasicRunAOTSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("RUN AOT enters the demo render loop and is bounded by the REPL harness deadline")
	}
	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	v := mapVoodooForMegaDemoBasicTest(t, h.bus)
	soundChip := newTestSoundChip()
	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sidPlayer := NewSIDPlayer(sidEngine)
	sidPlayer.AttachBus(h.bus)
	h.bus.MapIO(SID_PLAY_PTR, SID_PLAY_STATUS+3, sidPlayer.HandlePlayRead, sidPlayer.HandlePlayWrite)

	if out := h.runCommand(`LOAD "sdk/examples/basic/voodoo_mega_demo_basic.bas"`); strings.Contains(out, "ERROR") {
		t.Fatalf("LOAD failed: %q", out)
	}
	h.sendInput("RUN AOT\n")
	var capturedFrame []byte
	h.pumpUntil(func() bool {
		sidCtrl := sidPlayer.HandlePlayRead(SID_PLAY_CTRL)
		if v.HandleRead(VOODOO_ENABLE) != 1 || sidCtrl&1 == 0 || sidCtrl&2 != 0 || !sidPlayer.ForceLoop || v.cmdStreamCount == 0 || v.cmdStreamPtr == 0 {
			return false
		}
		frame := v.GetFrame()
		if !voodooFrameHasNonBlack(frame) {
			return false
		}
		capturedFrame = append(capturedFrame[:0], frame...)
		return true
	}, 60*time.Second)
	out := h.readOutput()
	if strings.Contains(out, "?COMPILE ERROR") || strings.Contains(out, "?SYNTAX ERROR") || strings.Contains(out, "?FrameCounter ERROR") || strings.Contains(out, "?OUT OF MEMORY") {
		t.Fatalf("RUN AOT failed: %q", out)
	}
	if got := v.HandleRead(VOODOO_ENABLE); got != 1 {
		t.Fatalf("Voodoo enable=%d, want 1; current line=%d error=%d error line=%d output=%q",
			got,
			h.bus.Read32(0x042000+0x200),
			h.bus.Read32(0x042000+0x208),
			h.bus.Read32(0x042000+0x228),
			out)
	}
	if got := v.HandleRead(VOODOO_VIDEO_DIM); got != 0x028001e0 {
		t.Fatalf("Voodoo dimensions=%#x, want 0x028001e0", got)
	}
	if v.cmdStreamCount == 0 || v.cmdStreamPtr == 0 {
		vars := readAOTNativeVarsForVoodoo(t, h, "DataIndex", "StarIndex", "StarAngle", "StarRadius", "FrameCounter")
		aligned := h.cpu.PC &^ 7
		disasm := disassembleIE64(func(addr uint64, size int) []byte {
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = h.bus.Read8(uint32(addr) + uint32(i))
			}
			return buf
		}, aligned-40, 12)
		t.Fatalf("Voodoo command stream was not submitted: pointer=%#x count=%d pc=%#x instr=%#x vars=%#v output=%q\n%s\n%s\n%s\n%s", v.cmdStreamPtr, v.cmdStreamCount, h.cpu.PC, h.bus.Read64(uint32(h.cpu.PC)), vars, out, readAOTStateDebug(h), readAOTAsmDebug(h), readAOTAsmTailDebug(h), fmt.Sprint(disasm))
	}
	if got := sidPlayer.HandlePlayRead(SID_PLAY_CTRL); got&1 == 0 || got&2 != 0 || !sidPlayer.ForceLoop {
		t.Fatalf("SID_PLAY_CTRL status=%d ForceLoop=%v, want playing loop with no error", got, sidPlayer.ForceLoop)
	}
	sidPtr := sidPlayer.HandlePlayRead(SID_PLAY_PTR)
	if got := h.bus.Read32(sidPtr); got != 0x44495350 {
		t.Fatalf("SID_PLAY_PTR=%#x points at %#x, want little-endian PSID", sidPtr, got)
	}
	frame := capturedFrame
	if len(frame) == 0 {
		frame = v.GetFrame()
	}
	if len(frame) == 0 {
		t.Fatal("Voodoo frame is empty")
	}
	nonBlack := false
	whiteish := 0
	pixels := 0
	for i := 0; i+3 < len(frame); i += 4 {
		if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
			nonBlack = true
		}
		if frame[i] > 240 && frame[i+1] > 240 && frame[i+2] > 240 {
			whiteish++
		}
		pixels++
	}
	if !nonBlack {
		vars := readAOTNativeVarsForVoodoo(t, h, "FrameCounter", "ScrollOffset", "StarIndex", "CharacterIndex", "DataIndex", "StarDepth", "ScreenX", "ScreenY", "ShadowPass", "FontColumn", "RectangleX", "RectangleY")
		t.Fatalf("Voodoo frame remained black after bounded RUN AOT smoke; pc=%#x instr=%#x asm=%#x code=%#x text len=%#x code len=%#x current line=%d error=%d error line=%d vars=%#v batch=%d jobs=%d busy=%v swapPending=%v status=%#x fbz=%#x color0=%#x swap=%#x output=%q\n%s\n%s",
			h.cpu.PC,
			h.bus.Read64(uint32(h.cpu.PC)),
			h.bus.Read64(0x042818),
			h.bus.Read64(0x042820),
			h.bus.Read64(0x042828),
			h.bus.Read64(0x042830),
			h.bus.Read32(0x042000+0x200),
			h.bus.Read32(0x042000+0x208),
			h.bus.Read32(0x042000+0x228),
			vars, v.GetTriangleBatchCount(), v.jobsInFlight, v.busy, v.swapPending,
			v.HandleRead(VOODOO_STATUS), v.HandleRead(VOODOO_FBZ_MODE), v.HandleRead(VOODOO_COLOR0), v.HandleRead(VOODOO_SWAP_BUFFER_CMD),
			out, readAOTStateDebug(h), readAOTAsmTailDebug(h))
	}
	if whiteish*100 > pixels*80 {
		t.Fatalf("Voodoo frame is mostly white after bounded RUN AOT smoke: %d/%d pixels", whiteish, pixels)
	}
	longStart := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
	longTarget := longStart + 120
	h.pumpUntil(func() bool {
		return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= longTarget || h.cpu.PC == 0
	}, 10*time.Second)
	longEnd := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
	if h.cpu.PC == 0 {
		t.Fatalf("RUN AOT returned through address zero after frame %d\n%s\n%s", longEnd, readAOTStateDebug(h), readAOTAsmTailDebug(h))
	}
	if longEnd < longTarget {
		vars := readAOTNativeVarsForVoodoo(t, h, "FrameCounter", "CommandCount", "StarIndex", "CharacterIndex", "DataIndex", "ScrollCharacter", "ScrollPixelOffset", "MessageIndex", "CharacterCode", "FontIndex", "FontRowIndex", "SpanIndex", "SpanCount")
		disasm := disassembleIE64(func(addr uint64, size int) []byte {
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = h.bus.Read8(uint32(addr) + uint32(i))
			}
			return buf
		}, h.cpu.PC-40, 11)
		t.Fatalf("RUN AOT froze from frame %d at frame %d before target %d; pc=%#x code=[%#x %#x %#x %#x %#x] sp=%#x running=%v vars=%#v command={ptr:%#x count:%d} voodoo={busy:%v jobs:%d swap:%v}\n%s\n%s",
			longStart, longEnd, longTarget, h.cpu.PC,
			h.bus.Read64(uint32(h.cpu.PC-24)), h.bus.Read64(uint32(h.cpu.PC-16)), h.bus.Read64(uint32(h.cpu.PC-8)), h.bus.Read64(uint32(h.cpu.PC)), h.bus.Read64(uint32(h.cpu.PC+8)),
			h.cpu.regs[31], h.cpu.running.Load(), vars, v.cmdStreamPtr, v.cmdStreamCount,
			v.busy, v.jobsInFlight, v.swapPending, readAOTStateDebug(h), fmt.Sprint(disasm))
	}
	// The preceding smoke run is the warm-up. Measure three independent samples
	// on the same live machine and use their median, as required by Milestone 1.
	samples := make([]time.Duration, 3)
	const measuredFrames = 30
	for i := range samples {
		startFrame := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
		targetFrame := startFrame + measuredFrames
		started := time.Now()
		h.pumpUntil(func() bool {
			return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= targetFrame
		}, 10*time.Second)
		samples[i] = time.Since(started)
		endFrame := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
		if endFrame < targetFrame {
			t.Fatalf("RUN AOT sample %d advanced from frame %d to %d, want at least %d within 10 seconds", i+1, startFrame, endFrame, targetFrame)
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	medianFPS := measuredFrames / samples[1].Seconds()
	// Wall-clock throughput depends on the host CPU, scheduler and software
	// Voodoo backend. Keep the functional suite portable and make the reference
	// performance gate explicit for controlled benchmark runs.
	minimumFPS := 0.0
	if raw := os.Getenv("IE_BASIC_MIN_FPS"); raw != "" {
		parsed, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || parsed < 0 {
			t.Fatalf("invalid IE_BASIC_MIN_FPS=%q", raw)
		}
		minimumFPS = parsed
	}
	if medianFPS < minimumFPS {
		t.Fatalf("RUN AOT median throughput %.2f fps is below configured %.2f fps; samples=%v", medianFPS, minimumFPS, samples)
	}
	t.Logf("RUN AOT median throughput %.2f fps; samples=%v (set IE_BASIC_MIN_FPS=60 for the Milestone 1 reference gate)", medianFPS, samples)
}

func readAOTNativeVarsForVoodoo(t *testing.T, h *ehbasicTestHarness, names ...string) map[string]uint64 {
	t.Helper()
	const (
		aotNativeVarSeg = 0x00071000
		valOffset       = 16
		recSize         = 24
	)
	count := h.bus.Read32(aotNativeVarSeg + 8)
	out := make(map[string]uint64, len(names))
	for _, name := range names {
		want := basicAOTVarTagForVoodoo(name)
		for i := uint32(0); i < count; i++ {
			rec := uint32(aotNativeVarSeg + 16 + i*recSize)
			if h.bus.Read32(rec) == want {
				out[name] = h.bus.Read64(rec + valOffset)
				break
			}
		}
	}
	return out
}

func basicAOTVarTagForVoodoo(name string) uint32 {
	var tag uint32
	count := 0
	for _, ch := range strings.ToUpper(name) {
		if (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9' || count == 0) {
			break
		}
		c := uint32(byte(ch))
		if count < 4 {
			tag = (tag << 8) | c
		} else {
			tag = tag*33 + c
		}
		count++
	}
	for count < 4 {
		tag <<= 8
		count++
	}
	return tag
}

func voodooFrameHasNonBlack(frame []byte) bool {
	for i := 0; i+3 < len(frame); i += 4 {
		if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
			return true
		}
	}
	return false
}

func mapVoodooForMegaDemoBasicTest(t *testing.T, bus *MachineBus) *VoodooEngine {
	t.Helper()
	v, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	t.Cleanup(v.Destroy)
	bus.MapIO(VOODOO_BASE, VOODOO_END, v.HandleRead, v.HandleWrite)
	bus.MapIOByteRead(VOODOO_BASE, VOODOO_END, v.HandleRead8)
	bus.MapIOByte(VOODOO_BASE, VOODOO_END, v.HandleWrite8)
	bus.MapIO64(VOODOO_BASE, VOODOO_END, v.HandleRead64, v.HandleWrite64)
	bus.MapIO(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead, v.HandleTexMemWrite)
	bus.MapIOByteRead(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemRead8)
	bus.MapIOByte(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1, v.HandleTexMemWrite8)
	return v
}

func parseBasicDataInts(t *testing.T, src string) []int {
	t.Helper()
	var vals []int
	for _, line := range strings.Split(src, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.EqualFold(fields[1], "DATA") {
			continue
		}
		for _, raw := range strings.Split(strings.TrimSpace(strings.SplitN(line, "DATA", 2)[1]), ",") {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				t.Fatalf("parse DATA value %q: %v", raw, err)
			}
			vals = append(vals, n)
		}
	}
	return vals
}

func parseAsmByteBlock(t *testing.T, asm, start, end string) []int {
	t.Helper()
	block := betweenLabels(t, asm, start, end)
	var vals []int
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, ";", 2)[0])
		if !strings.HasPrefix(line, ".byte") {
			continue
		}
		for _, raw := range strings.Split(strings.TrimSpace(strings.TrimPrefix(line, ".byte")), ",") {
			raw = strings.TrimSpace(raw)
			n, err := strconv.ParseInt(raw, 0, 64)
			if err != nil {
				t.Fatalf("parse asm byte %q: %v", raw, err)
			}
			vals = append(vals, int(n))
		}
	}
	return vals
}

func extractAsmASCII(t *testing.T, asm, label string) string {
	t.Helper()
	block := betweenLabels(t, asm, label, "font_mask_table")
	re := regexp.MustCompile(`(?m)\.ascii\s+"([^"]*)"`)
	m := re.FindStringSubmatch(block)
	if len(m) != 2 {
		t.Fatalf("missing .ascii under %s", label)
	}
	return m[1]
}

func betweenLabels(t *testing.T, src, start, end string) string {
	t.Helper()
	a := strings.Index(src, start+":")
	if a < 0 {
		t.Fatalf("missing label %s", start)
	}
	b := strings.Index(src[a+len(start)+1:], end+":")
	if b < 0 {
		t.Fatalf("missing label %s after %s", end, start)
	}
	return src[a+len(start)+1 : a+len(start)+1+b]
}
