package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestAB3D64PlatformScratchAddressesFitLegacyBus(t *testing.T) {
	files := []string{
		"sdk/ab3d64/src/ie/ie_hires_platform.s",
		"sdk/ab3d64/src/ie/menu/menunb.s",
	}
	re := regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s+equ\s+\$([0-9A-Fa-f]+)`)
	watched := map[string]bool{
		"CHUNKY_BASE":      true,
		"CHUNKY_BACK_BASE": true,
		"PRESENT_BASE":     true,
		"SCALE_BASE":       true,
		"SCALE_BACK_BASE":  true,
		"_mnu_screen":      true,
		"_mnu_morescreen":  true,
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			name := m[1]
			if !watched[name] {
				continue
			}
			v, err := strconv.ParseUint(m[2], 16, 32)
			if err != nil {
				t.Fatalf("parse %s in %s: %v", name, path, err)
			}
			if v >= DEFAULT_MEMORY_SIZE {
				t.Fatalf("%s in %s = %#x, outside 32 MiB bus", name, path, v)
			}
		}
	}
}

func TestAB3D64EntrySeedsStackAboveGeneratedImage(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/ie_hires_platform.s")
	if err != nil {
		t.Fatalf("read platform source: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "IE_STACK_TOP\tequ\t$00FFF000") {
		t.Fatal("AB3D64 must reserve a high guest stack above the generated image")
	}
	entry := strings.Index(text, "_ie64_entry:")
	if entry < 0 {
		t.Fatal("missing _ie64_entry")
	}
	body := text[entry:]
	if next := strings.Index(body, "\n_Sys_Init:"); next >= 0 {
		body = body[:next]
	}
	if !strings.Contains(body, "move.l\t#IE_STACK_TOP,sp") {
		t.Fatal("_ie64_entry must seed the m68k guest stack before jumping into _startup")
	}
}

func TestAB3D64MakefileBuildsInteractiveMenuByDefault(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(b)
	if strings.Contains(text, "-D IE_AUTOSTART") {
		t.Fatal("sdk/ab3d64 Makefile must not define IE_AUTOSTART in the default interactive game build")
	}
	if strings.Contains(text, "-D mnu_nocode") {
		t.Fatal("sdk/ab3d64 Makefile must include the AB3D2 menu code in the default interactive game build")
	}
	if !strings.Contains(text, "-D IS_IE=1") {
		t.Fatal("sdk/ab3d64 Makefile must define IS_IE so IE-specific source guards are active")
	}
	if !strings.Contains(text, "IE64_SOURCE_OUT=build/ab3d64.ie64.s") {
		t.Fatal("sdk/ab3d64 Makefile must persist generated IE64 source for inspection/debugging")
	}
}

func TestM68KTo64KMakeSupportsPersistedIE64Source(t *testing.T) {
	b, err := os.ReadFile("sdk/scripts/ab3d2/kmake.sh")
	if err != nil {
		t.Fatalf("read kmake: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "IE64_SOURCE_OUT") {
		t.Fatal("kmake.sh must support IE64_SOURCE_OUT")
	}
	if !strings.Contains(text, `cp "$CONCAT" "$IE64_SOURCE_OUT"`) {
		t.Fatal("kmake.sh must copy the generated concat.ie64.s before deleting its temp dir")
	}
}

func TestAB3D64IEMenuOpenDoesNotUseStackForSavedMenuPointer(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/menu/menunb.s")
	if err != nil {
		t.Fatalf("read IE menu source: %v", err)
	}
	text := string(b)
	start := strings.Index(text, "mnu_openmenu:")
	if start < 0 {
		t.Fatal("missing mnu_openmenu")
	}
	body := text[start:]
	if next := strings.Index(body, "\nmnu_waitmenu:"); next >= 0 {
		body = body[:next]
	}
	if strings.Contains(body, "move.l\ta0,-(a7)") || strings.Contains(body, "move.l\t(a7)+,a0") {
		t.Fatal("IE mnu_openmenu must not use stack save/restore for the active menu pointer")
	}
	if !strings.Contains(body, "mnu_openmenu_savedptr") {
		t.Fatal("IE mnu_openmenu must preserve the active menu pointer in mnu_openmenu_savedptr")
	}
}

func TestAB3D64SymbolExtractorKeepsReferencedBSSLabels(t *testing.T) {
	dir := t.TempDir()
	listing := filepath.Join(dir, "listing.txt")
	out := filepath.Join(dir, "symbols.lua")
	const listingText = `Successfully assembled

--- Listing ---
                         _Lvl_ListOfGraphRoomsPtr_l:
                         Lvl_ListOfGraphRoomsPtr_l:
                         ds.l 1  ; 4 bytes reserved
                         DrawDisplay:
00001000  04 87 00 00 34 12 00 00... lea r16, first_instruction_target(r0)
0000A230  04 87 00 00 D4 E4 01 00... lea r16, _Lvl_ListOfGraphRoomsPtr_l(r0)
000B610B  04 87 00 00 62 B7 02 00... lea r16, Draw_ZoneClipL_w(r0)
`
	if err := os.WriteFile(listing, []byte(listingText), 0o644); err != nil {
		t.Fatalf("write listing fixture: %v", err)
	}
	cmd := exec.Command("python3", "sdk/ab3d64/tools/extract_ie64_symbols.py", listing, out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extract symbols: %v\n%s", err, output)
	}
	gotBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read extracted symbols: %v", err)
	}
	got := string(gotBytes)
	for _, needle := range []string{
		"_Lvl_ListOfGraphRoomsPtr_l = 0x0001E4D4",
		"DrawDisplay = 0x00001000",
		"first_instruction_target = 0x00001234",
		"Draw_ZoneClipL_w = 0x0002B762",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("extracted symbols missing %q in:\n%s", needle, got)
		}
	}
}

func TestAB3D64AutostartUsesConcreteMediaPaths(t *testing.T) {
	files := []string{
		"sdk/ab3d64/src/ie/hires.s",
		"sdk/ab3d64/src/ie/controlloop.s",
		"sdk/ab3d64/src/data/draw_data.s",
		"sdk/ab3d64/src/data/level_data.s",
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(b)
		if strings.Contains(text, "ab3:includes/") || strings.Contains(text, "ab3:levels/") {
			t.Fatalf("%s still routes autostart assets through Amiga volume prefixes", path)
		}
	}
}

func TestAB3D64MediaLoadsPreferUnpackedAssets(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/ie_file_io_runtime.i")
	if err != nil {
		t.Fatalf("read IE file runtime: %v", err)
	}
	text := string(b)
	if strings.Contains(text, "move.l\td1,a6") {
		t.Fatal("io_ie_load_to_heap must use the normalized a0 filename, not a stale d1 path fast path")
	}
	unpacked := strings.Index(text, "bsr\t\tio_ie_make_unpacked_media_path")
	repoBuild := strings.Index(text, "bsr\t\tio_ie_make_repo_root_build_path")
	if unpacked < 0 || repoBuild < 0 {
		t.Fatal("io_ie_load_to_heap must include unpacked-media and repo-build candidates")
	}
	if unpacked > repoBuild {
		t.Fatal("media loads must try _build/ie_unpacked/media before packed media candidates")
	}
}

func TestAB3D64BootAuditRefreshesUnpackedMediaMirror(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/tools/run_boot_audit.sh")
	if err != nil {
		t.Fatalf("read boot audit script: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "unpack_sb_assets.py") {
		t.Fatal("boot audit must refresh _build/ie_unpacked/media before launching AB3D64")
	}
	if !strings.Contains(text, "--source \"$LAUNCH_CWD/media\"") ||
		!strings.Contains(text, "--out \"$LAUNCH_CWD/_build/ie_unpacked/media\"") {
		t.Fatal("boot audit must unpack launch media into the launch cwd unpacked mirror")
	}
}

func TestAB3D64BootAuditKeepsAccessGuardsOptIn(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/tools/diag_boot_audit.ies")
	if err != nil {
		t.Fatalf("read boot audit script: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, `AB3D64_AUDIT_TRACE`) {
		t.Fatal("boot audit must expose an explicit trace mode for expensive diagnostics")
	}
	if !strings.Contains(text, `if heavy_trace and dbg.guard_add and S[name] then`) {
		t.Fatal("code write guards must remain opt-in so normal boot audit can use IE64 JIT breakpoints")
	}
}

func TestAB3D64BootAuditUsesPresentMarkerInsteadOfFinalBreakpoint(t *testing.T) {
	audit, err := os.ReadFile("sdk/ab3d64/tools/diag_boot_audit.ies")
	if err != nil {
		t.Fatalf("read boot audit script: %v", err)
	}
	platform, err := os.ReadFile("sdk/ab3d64/src/ie/ie_hires_platform.s")
	if err != nil {
		t.Fatalf("read platform source: %v", err)
	}
	if !strings.Contains(string(platform), "IE_PRESENT_COUNT") ||
		!strings.Contains(string(platform), "addq.l\t#1,IE_PRESENT_COUNT") {
		t.Fatal("_Vid_Present must increment the audit-visible present marker")
	}
	text := string(audit)
	if !strings.Contains(text, "wait_for_present_marker") ||
		!strings.Contains(text, "IE_PRESENT_COUNT = 0x00625FFC") {
		t.Fatal("boot audit must poll the present marker instead of relying on a final JIT breakpoint")
	}
	if !strings.Contains(text, "scan_fb_visible_count") ||
		!strings.Contains(text, "r ~= 0 or g ~= 0 or b ~= 0") {
		t.Fatal("boot audit must count visible RGB pixels, not opaque-black alpha bytes")
	}
}

func TestAB3D64FirstDrawDisplayPresentsBeforeFullRenderer(t *testing.T) {
	makefile, err := os.ReadFile("sdk/ab3d64/Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "-D IE_FIRST_FRAME_PRESENT_EARLY=1") {
		t.Fatal("AB3D64 audit build must enable the one-shot first-frame present path")
	}
	b, err := os.ReadFile("sdk/ab3d64/src/ie/hires.s")
	if err != nil {
		t.Fatalf("read hires source: %v", err)
	}
	text := string(b)
	start := strings.Index(text, "DrawDisplay:")
	if start < 0 {
		t.Fatal("missing DrawDisplay")
	}
	body := text[start:]
	if next := strings.Index(body, "\n\t\t\t\tbsr\t\tDraw_Zone_Graph"); next >= 0 {
		body = body[:next]
	}
	for _, needle := range []string{
		"IFD\t\tIE_FIRST_FRAME_PRESENT_EARLY",
		"tst.b\tie_first_present_done",
		"CALLC\tVid_Present",
		"st\t\tie_first_present_done",
		"rts",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("first DrawDisplay path must present before the full renderer; missing %q", needle)
		}
	}
}

func TestAB3D64RuntimeDoesNotOverwriteFlatImageFakeVectors(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/ie_hires_platform.s")
	if err != nil {
		t.Fatalf("read platform source: %v", err)
	}
	text := string(b)
	start := strings.Index(text, "ie_init_fake_lib_vectors:")
	if start < 0 {
		t.Fatal("missing fake vector initializer landmark")
	}
	body := text[start:]
	if next := strings.Index(body, "\n_"); next >= 0 {
		body = body[:next]
	}
	for _, forbidden := range []string{
		"FAKE_LIB_BASE+_LVOCacheControl",
		"FAKE_LIB_BASE+FAKE_LVO_WAITTOF",
		"#$4E75",
		"#$51",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("runtime initializer must not overwrite flat-image fake vectors with %q", forbidden)
		}
	}
}

func TestAB3D64FakeVectorsArePresentInFlatImageSources(t *testing.T) {
	makefile, err := os.ReadFile("sdk/ab3d64/Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "src/ie/ie_fake_vectors.s") {
		t.Fatal("AB3D64 build must include the fake vector image segment")
	}
	vectors, err := os.ReadFile("sdk/ab3d64/src/ie/ie_fake_vectors.s")
	if err != nil {
		t.Fatalf("read fake vector source: %v", err)
	}
	text := string(vectors)
	for _, needle := range []string{
		"org FAKE_LIB_BASE+_LVOWaitTOF",
		"org FAKE_LIB_BASE+_LVOCacheControl",
		"bra\tie_fake_lvo_rts",
		"ie_fake_lvo_rts:\n\trts",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("fake vector source missing %q", needle)
		}
	}
}

func TestAB3D64CacheMacrosAreNoOpsUnderIE(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/macros.i")
	if err != nil {
		t.Fatalf("read macros: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "IFD IS_IE") {
		t.Fatal("cache macros must have an IS_IE branch")
	}
	if !strings.Contains(text, "DataCacheOff\tmacro\n\t\t\t\tendm") ||
		!strings.Contains(text, "DataCacheOn\t\tmacro\n\t\t\t\tendm") {
		t.Fatal("DataCacheOn/Off must expand to no-ops under IS_IE")
	}
}

func TestAB3D64TweenBrightsBoundsInitialAnchorScan(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/objdrawhires.s")
	if err != nil {
		t.Fatalf("read objdraw source: %v", err)
	}
	text := string(b)
	start := strings.Index(text, "draw_TweenBrights:")
	if start < 0 {
		t.Fatal("missing draw_TweenBrights")
	}
	body := text[start:]
	if next := strings.Index(body, "\n*************************************"); next >= 0 {
		body = body[:next]
	}
	for _, needle := range []string{
		"moveq\t#15,d6",
		"dbra\td6,.backinto",
		"rts",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("draw_TweenBrights initial scan must be bounded; missing %q", needle)
		}
	}
}

func TestAB3D64HiresEdgeWalkersClampProjectedY(t *testing.T) {
	b, err := os.ReadFile("sdk/ab3d64/src/ie/hires.s")
	if err != nil {
		t.Fatalf("read hires source: %v", err)
	}
	text := string(b)
	for _, label := range []string{"lineclipped:", "lineclippedGOUR:"} {
		start := strings.Index(text, label)
		if start < 0 {
			t.Fatalf("missing %s", label)
		}
		body := text[start:]
		if next := strings.Index(body, "\n\t\t\t\tmove.l\t#RightSideTable_vw,a3"); next >= 0 {
			body = body[:next]
		}
		for _, needle := range []string{
			"cmp.w\t#0,d1",
			"moveq\t#0,d1",
			"cmp.w\t#SCREEN_HEIGHT-1,d1",
			"move.w\t#SCREEN_HEIGHT-1,d1",
			"cmp.w\t#0,d3",
			"moveq\t#0,d3",
			"cmp.w\t#SCREEN_HEIGHT-1,d3",
			"move.w\t#SCREEN_HEIGHT-1,d3",
		} {
			if !strings.Contains(body, needle) {
				t.Fatalf("%s must clamp projected Y before side-table indexing; missing %q", label, needle)
			}
		}
	}
}
