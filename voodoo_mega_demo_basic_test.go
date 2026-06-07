package main

import (
	"os"
	"path/filepath"
	"regexp"
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
	if strings.Count(upper, "BLOAD") != 1 || !strings.Contains(src, `BLOAD "sdk/examples/assets/music/Reggae_2.sid",SA`) {
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
		"SN=MEMALLOC(4096,4096)",
		"PR=MEMALLOC(12288,4096)",
		"ST=MEMALLOC(4096,4096)",
		"MS=MEMALLOC(4096,4096)",
		"SA=MEMALLOC(8192,4096)",
		"RS=54321",
		"POKE32 VX,&H028001E0",
		"POKE32 FB,&H0770",
		"POKE32 CP,0",
		"POKE32 &HF0E24,4790",
		"POKE32 &HF0E28,5",
		"POKE32 &HF8128,1",
		"SO=SO+SS",
		"WR=6944",
		"FOR SI=0 TO 255",
		"CHC=PEEK(MS+MI)",
	} {
		if !strings.Contains(upper, strings.ToUpper(want)) {
			t.Fatalf("demo source missing %q", want)
		}
	}

	if regexp.MustCompile(`(?m)(^|:)\s*(VOODOO|VERTEX|VSYNC)\b`).MatchString(upper) {
		t.Fatalf("render loop must use direct POKE32 Voodoo MMIO without high-level Voodoo commands or VSYNC")
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
		"10 X=1664525*54321+1013904223",
		"20 X=X-INT(X/4294967296)*4294967296:IF X<0 THEN X=X+4294967296",
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
		if v.HandleRead(VOODOO_ENABLE) != 1 || sidCtrl&1 == 0 || sidCtrl&2 != 0 || !sidPlayer.ForceLoop {
			return false
		}
		frame := v.GetFrame()
		if !voodooFrameHasNonBlack(frame) {
			return false
		}
		capturedFrame = append(capturedFrame[:0], frame...)
		return true
	}, 120*time.Second)
	out := h.readOutput()
	if strings.Contains(out, "?COMPILE ERROR") || strings.Contains(out, "?SYNTAX ERROR") || strings.Contains(out, "?FC ERROR") || strings.Contains(out, "?OUT OF MEMORY") {
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
		t.Fatalf("Voodoo frame remained black after bounded RUN AOT smoke; pc=%#x instr=%#x asm=%#x code=%#x text len=%#x code len=%#x current line=%d error=%d error line=%d output=%q",
			h.cpu.PC,
			h.bus.Read64(uint32(h.cpu.PC)),
			h.bus.Read64(0x042818),
			h.bus.Read64(0x042820),
			h.bus.Read64(0x042828),
			h.bus.Read64(0x042830),
			h.bus.Read32(0x042000+0x200),
			h.bus.Read32(0x042000+0x208),
			h.bus.Read32(0x042000+0x228),
			out)
	}
	if whiteish*100 > pixels*80 {
		t.Fatalf("Voodoo frame is mostly white after bounded RUN AOT smoke: %d/%d pixels", whiteish, pixels)
	}
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
