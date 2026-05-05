//go:build headless

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func readDemoSource(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(assemblerExamplesRepoRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func regionBetween(t *testing.T, src, start, end string) string {
	t.Helper()
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("start marker %q not found", start)
	}
	j := strings.Index(src[i+len(start):], end)
	if j < 0 {
		t.Fatalf("end marker %q not found after %q", end, start)
	}
	return src[i : i+len(start)+j]
}

func TestIEWarpAMix_UsesSignedNegativeSaturation(t *testing.T) {
	src := readDemoSource(t, "sdk/examples/asm/iewarp_service.asm")
	region := regionBetween(t, src, "amix_clamp_lo:", "amix_store:")
	if !strings.Contains(region, "move.l r7, #0xFFFF8000") {
		t.Fatal("AMix negative clamp does not load -32768")
	}
	if !strings.Contains(region, "bge r22, r7, amix_store") {
		t.Fatal("AMix negative clamp does not use signed >= branch against accumulator")
	}
	if strings.Contains(region, "unsigned") {
		t.Fatal("AMix negative clamp still documents -32768 as unsigned")
	}
}

func TestIEWarpADPCM_NoQuadraticApproximation(t *testing.T) {
	src := readDemoSource(t, "sdk/examples/asm/iewarp_service.asm")
	if strings.Contains(src, "quadratic") || strings.Contains(src, "index^2") {
		t.Fatal("IEWarp ADPCM still references quadratic step approximation")
	}
}

func TestArosRotozoomers_StartMusicWhenAssetEmbedded(t *testing.T) {
	for _, rel := range []string{
		"sdk/examples/asm/rotozoomer_aros_api.asm",
		"sdk/examples/asm/rotozoomer_aros_hw.asm",
		"sdk/examples/c/rotozoomer_aros_api.c",
		"sdk/examples/c/rotozoomer_aros_hw.c",
	} {
		t.Run(filepath.Base(rel), func(t *testing.T) {
			src := readDemoSource(t, rel)
			if !strings.Contains(src, "start_music") {
				t.Fatal("missing start_music hook")
			}
			if !strings.Contains(src, "stop_music") {
				t.Fatal("missing stop_music hook")
			}
			if !strings.Contains(src, "chopper.ahx") && !strings.Contains(src, "MEDIA_CTRL") {
				t.Fatal("music init neither embeds chopper.ahx nor uses media loader")
			}
		})
	}
}

func TestDemoPerfHoists_SourceShape(t *testing.T) {
	fire := readDemoSource(t, "sdk/examples/asm/vga_mode13h_fire.asm")
	fireLoop := regionBetween(t, fire, ".col_loop:", ".done:")
	if strings.Count(fireLoop, "MUL A, #WIDTH") != 0 {
		t.Fatal("vga_mode13h_fire still multiplies row address inside col_loop")
	}
	if !strings.Contains(fire, "clamp_nonnegative:") {
		t.Fatal("vga_mode13h_fire missing shared clamp helper")
	}
	rightNeighbor := regionBetween(t, fire, "; --- Add right-below neighbour", ".no_right:")
	if strings.Contains(rightNeighbor, "LDC #WIDTH") {
		t.Fatal("vga_mode13h_fire clobbers source address register C during right-edge check")
	}
	if !strings.Contains(rightNeighbor, "LDU #WIDTH") || !strings.Contains(rightNeighbor, "LDA C\n    ADD A, #1") {
		t.Fatal("vga_mode13h_fire no longer preserves C before sampling source+1")
	}

	roto65 := readDemoSource(t, "sdk/examples/asm/rotozoomer_65.asm")
	if strings.Contains(roto65, "We re-read the reciprocal because mul16_signed consumed mul_b") {
		t.Fatal("rotozoomer_65 still reloads reciprocal for var_sa")
	}

	mandel := readDemoSource(t, "sdk/examples/asm/mandelbrot_ie64.asm")
	if !strings.Contains(mandel, "mulu.q  r10, r9, #0x010101") {
		t.Fatal("mandelbrot_ie64 does not pack RGB with 0x010101 multiply")
	}
	if strings.Contains(mandel, "lsl.q   r10, r10, #8") {
		t.Fatal("mandelbrot_ie64 still uses shift/add RGB packing")
	}
	if !strings.Contains(mandel, "asr.q   r10, r10, #15") {
		t.Fatal("mandelbrot_ie64 did not fuse 2*zx*zy shift to asr 15")
	}
}

func TestVoodooMegaDemo_SineTableIndexingMatchesLayout(t *testing.T) {
	src := readDemoSource(t, "sdk/examples/asm/voodoo_mega_demo.asm")

	getSin := regionBetween(t, src, "get_sin:", "; ----------------------------------------------------------------------------\n; random")
	if !strings.Contains(getSin, "MUL A, #4") {
		t.Fatal("get_sin must scale angles for 4-byte sin_table entries")
	}

	buildTable := regionBetween(t, src, "build_sin_table:", "; ============================================================================\n; init_stars")
	if !strings.Contains(buildTable, "LDX #quarter_sin\n    ADD X, A\n    LDA [X]\n    AND A, #0xFF") {
		t.Fatal("build_sin_table must mask byte-packed quarter_sin loads")
	}
	if !strings.Contains(buildTable, "STA [X]\n    ADD X, #4") {
		t.Fatal("build_sin_table must keep sin_table as 4-byte entries")
	}
}

func TestVoodooMegaDemo_ScrollOffsetWrapsAtMessageWidth(t *testing.T) {
	src := readDemoSource(t, "sdk/examples/asm/voodoo_mega_demo.asm")
	msgStart := strings.Index(src, "scroll_message:")
	if msgStart < 0 {
		t.Fatal("scroll_message label not found")
	}
	asciiStart := strings.Index(src[msgStart:], ".ascii \"")
	if asciiStart < 0 {
		t.Fatal("scroll_message .ascii not found")
	}
	msg := src[msgStart+asciiStart+len(".ascii \""):]
	msgEnd := strings.Index(msg, "\"")
	if msgEnd < 0 {
		t.Fatal("scroll_message string is unterminated")
	}
	msgLen := msgEnd
	wrap := msgLen * 32

	if !strings.Contains(src, ".equ SCROLL_WRAP    "+strconv.Itoa(wrap)) {
		t.Fatalf("scroll offset wrap must cover exactly %d message pixels", wrap)
	}
	scrollLoop := regionBetween(t, src, "scroll_char_loop:", "; Fetch ASCII character from message")
	if !strings.Contains(scrollLoop, "SUB A, #"+strconv.Itoa(msgLen)) || !strings.Contains(scrollLoop, "ADD A, #"+strconv.Itoa(msgLen)) {
		t.Fatalf("scroll modulo must use real message length %d", msgLen)
	}

	advance := regionBetween(t, src, "; --- Advance animation ---", "JMP main_loop")
	if strings.Contains(advance, "AND A, #31") {
		t.Fatal("scroll offset is masked to one character and cannot advance the message")
	}
	if !strings.Contains(advance, "SUB B, #SCROLL_WRAP") || !strings.Contains(advance, "scroll_advance_store:") {
		t.Fatal("scroll offset advance must wrap at SCROLL_WRAP")
	}
}
