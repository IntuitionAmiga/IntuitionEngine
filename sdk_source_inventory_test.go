package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type sdkSourceFact struct {
	Surface  string
	Kind     string
	Name     string
	Evidence string
}

func TestSDKIEMonSourceInventoryGoldenMatchesSource(t *testing.T) {
	assertSourceInventoryGolden(t, "sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md", renderSDKIEMonSourceAudit(t), "UPDATE_SDK_IEMON_SOURCE_AUDIT")
}

func TestSDKIEScriptSourceInventoryGoldenMatchesSource(t *testing.T) {
	assertSourceInventoryGolden(t, "sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md", renderSDKIEScriptSourceAudit(t), "UPDATE_SDK_IESCRIPT_SOURCE_AUDIT")
}

func TestSDKArchitectureSourceInventoryGoldenMatchesSource(t *testing.T) {
	assertSourceInventoryGolden(t, "sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md", renderSDKArchitectureSourceAudit(t), "UPDATE_SDK_ARCH_SOURCE_AUDIT")
}

func TestSDKIEMonManualCoverageMatchesSourceInventory(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	for _, fact := range sdkIEMonFactsFromSource(t) {
		if fact.Kind != "command" && fact.Kind != "dispatch alias" && fact.Kind != "command syntax" && fact.Kind != "region divergence row" {
			continue
		}
		if !manualMentionsCodeToken(doc, fact.Name) && !normalizedContains(doc, fact.Name) {
			t.Fatalf("iemon.md missing source-derived %s %q from %s", fact.Kind, fact.Name, fact.Evidence)
		}
	}
	for _, heading := range []string{
		"#### `trace mmio <region> [count]`",
	} {
		if !strings.Contains(doc, heading) {
			t.Fatalf("iemon.md missing full command-reference heading %q", heading)
		}
	}
}

func TestSDKIEScriptManualCoverageMatchesSourceInventory(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, fact := range sdkIEScriptFactsFromSource(t) {
		if fact.Kind != "binding" && fact.Kind != "api claim" && fact.Kind != "api contract" {
			continue
		}
		if !manualMentionsCodeToken(doc, fact.Name) && !normalizedContains(doc, fact.Name) {
			t.Fatalf("iescript.md missing source-derived %s %q from %s", fact.Kind, fact.Name, fact.Evidence)
		}
	}
}

func TestSDKArchitectureManualCoverageMatchesSourceInventory(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/architecture.md")
	for _, fact := range sdkArchitectureFactsFromSource(t) {
		switch fact.Kind {
		case "public architecture category":
			if !strings.Contains(doc, fact.Name) {
				t.Fatalf("architecture.md missing source-derived architecture category %q from %s", fact.Name, fact.Evidence)
			}
		case "memory map row", "memory map subrange", "cpu bridge row", "jit matrix row", "architecture claim":
			if !normalizedContains(doc, fact.Name) && !normalizedContains(doc, "`"+fact.Name+"`") {
				t.Fatalf("architecture.md missing source-derived %s %q from %s", fact.Kind, fact.Name, fact.Evidence)
			}
		}
	}
}

func TestSDKDocAuditLedgerRequiresFiveManualEmpiricalInventories(t *testing.T) {
	ledger := readAuditFile(t, "sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md")
	for _, needle := range []string{
		"SDK_ISA_SOURCE_AUDIT.md",
		"SDK_IEMON_SOURCE_AUDIT.md",
		"SDK_IESCRIPT_SOURCE_AUDIT.md",
		"SDK_ARCH_SOURCE_AUDIT.md",
		"Positive gates compare each manual against its empirical inventory.",
	} {
		if !strings.Contains(ledger, needle) {
			t.Fatalf("SDK doc audit ledger is missing five-manual empirical inventory rule %q", needle)
		}
	}
	for _, path := range []string{
		"sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("empirical inventory %s is missing: %v", path, err)
		}
	}
}

func assertSourceInventoryGolden(t *testing.T, path, expected, updateEnv string) {
	t.Helper()
	if os.Getenv(updateEnv) == "1" {
		if err := os.WriteFile(path, []byte(expected), 0o644); err != nil {
			t.Fatalf("update %s: %v", path, err)
		}
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s is missing or unreadable: %v\n--- want\n%s", path, err, expected)
	}
	if got := string(gotBytes); got != expected {
		t.Fatalf("%s drifted from executable source facts\n--- got\n%s\n--- want\n%s", path, got, expected)
	}
}

func renderSDKIEMonSourceAudit(t *testing.T) string {
	return renderSDKSourceFacts("# SDK IEMon Source Audit", sdkIEMonFactsFromSource(t))
}

func renderSDKIEScriptSourceAudit(t *testing.T) string {
	return renderSDKSourceFacts("# SDK IEScript Source Audit", sdkIEScriptFactsFromSource(t))
}

func renderSDKArchitectureSourceAudit(t *testing.T) string {
	return renderSDKSourceFacts("# SDK Architecture Source Audit", sdkArchitectureFactsFromSource(t))
}

func renderSDKSourceFacts(title string, facts []sdkSourceFact) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("| Surface | Kind | Name | Executable evidence |\n")
	b.WriteString("|---------|------|------|---------------------|\n")
	for _, fact := range facts {
		b.WriteString(fmt.Sprintf("| %s | %s | `%s` | %s |\n", escapeMarkdownTableCell(fact.Surface), escapeMarkdownTableCell(fact.Kind), escapeMarkdownTableCell(fact.Name), escapeMarkdownTableCell(fact.Evidence)))
	}
	return b.String()
}

func sdkIEMonFactsFromSource(t *testing.T) []sdkSourceFact {
	t.Helper()
	source := readAuditFile(t, "debug_commands.go")
	registry := sourceBetween(t, source, "func monitorHelpRegistry() []monitorHelpEntry {", "func monitorHelpByName")
	dispatch := sourceBetween(t, source, "switch cmd.Name {", "default:")
	registryNames := parseQuotedNamesAfterPrefix(registry, `Name:\s*`)
	dispatchNames := parseSwitchCaseNames(dispatch)

	seen := map[string]bool{}
	var facts []sdkSourceFact
	entries := parseMonitorHelpEntries(registry)
	for _, entry := range entries {
		name := entry.Name
		seen[name] = true
		facts = append(facts, sdkSourceFact{
			Surface:  "IEMon",
			Kind:     "command",
			Name:     name,
			Evidence: "`debug_commands.go` `monitorHelpRegistry` entry",
		})
		facts = append(facts, sdkSourceFact{
			Surface:  "IEMon",
			Kind:     "command summary",
			Name:     name + " - " + entry.Summary,
			Evidence: "`debug_commands.go` `monitorHelpRegistry` summary",
		})
		for _, syntax := range entry.Syntax {
			facts = append(facts, sdkSourceFact{
				Surface:  "IEMon",
				Kind:     "command syntax",
				Name:     syntax,
				Evidence: "`debug_commands.go` `monitorHelpRegistry` syntax for `" + name + "`",
			})
		}
		for _, example := range entry.Examples {
			facts = append(facts, sdkSourceFact{
				Surface:  "IEMon",
				Kind:     "command example",
				Name:     example,
				Evidence: "`debug_commands.go` `monitorHelpRegistry` example for `" + name + "`",
			})
		}
	}
	if len(entries) != len(registryNames) {
		t.Fatalf("parsed %d monitor help entries, want %d registry names", len(entries), len(registryNames))
	}
	for _, name := range dispatchNames {
		if seen[name] {
			continue
		}
		facts = append(facts, sdkSourceFact{
			Surface:  "IEMon",
			Kind:     "dispatch alias",
			Name:     name,
			Evidence: "`debug_commands.go` `executeCommand` switch case",
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"Z80 | 0xF000-0xF0FF direct MMIO window and 0xA0-0xAD VGA port range", "`cpu_z80_runner.go` MMIO translation, `vga_constants.go` `Z80_VGA_PORT_*`"},
		{"6502 | Page-1 stack, 0xF000-0xF0FF direct MMIO, VGA at 0xD700-0xD70D, and ULA at 0xD800-0xD817", "`cpu_six5go2.go` stack/MMIO mapping, `vga_constants.go` `C6502_VGA_*`, `ula_constants.go` `C6502_ULA_BASE`"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "IEMon",
			Kind:     "region divergence row",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	sortSDKSourceFacts(facts)
	return facts
}

func sdkIEScriptFactsFromSource(t *testing.T) []sdkSourceFact {
	t.Helper()
	source := readAuditFile(t, "script_engine.go")
	registerModules := sourceBetween(t, source, "func (se *ScriptEngine) registerModules", "func (se *ScriptEngine) luaSysWaitFrames")
	re := regexp.MustCompile(`(?s)([a-zA-Z][a-zA-Z0-9_]*) := L\.SetFuncs\(L\.NewTable\(\), map\[string\]lua\.LGFunction\{(.*?)\}\)\s*L\.SetGlobal\("([^"]+)", ([a-zA-Z][a-zA-Z0-9_]*)\)`)
	keyRe := regexp.MustCompile(`(?m)^\s*"([^"]+)":`)
	var facts []sdkSourceFact
	for _, m := range re.FindAllStringSubmatch(registerModules, -1) {
		if m[1] != m[4] {
			continue
		}
		module := m[3]
		for _, key := range keyRe.FindAllStringSubmatch(m[2], -1) {
			facts = append(facts, sdkSourceFact{
				Surface:  "IEScript",
				Kind:     "binding",
				Name:     module + "." + key[1],
				Evidence: "`script_engine.go` `registerModules` binding",
			})
		}
	}
	registerBit32 := sourceBetween(t, source, "func (se *ScriptEngine) registerBit32", "func (se *ScriptEngine) onFrameComplete")
	for _, key := range keyRe.FindAllStringSubmatch(registerBit32, -1) {
		facts = append(facts, sdkSourceFact{
			Surface:  "IEScript",
			Kind:     "binding",
			Name:     "bit32." + key[1],
			Evidence: "`script_engine.go` `registerBit32` binding",
		})
	}
	keysBlock := sourceBetween(t, source, "keys := L.NewTable()", "L.SetGlobal(\"keys\", keys)")
	keyConstRe := regexp.MustCompile(`\{"([^"]+)",\s*0x[0-9A-Fa-f]+\}`)
	for _, key := range keyConstRe.FindAllStringSubmatch(keysBlock, -1) {
		facts = append(facts, sdkSourceFact{
			Surface:  "IEScript",
			Kind:     "binding",
			Name:     "keys." + key[1],
			Evidence: "`script_engine.go` `keys` table binding",
		})
	}
	facts = append(facts, sdkSourceFact{
		Surface:  "IEScript",
		Kind:     "api claim",
		Name:     "raw memory access requires cpu.freeze()",
		Evidence: "`script_engine.go` `requireFrozenForRange` error path",
	})
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"mem.read8(addr) returns number and truncates addr to uint32", "`script_engine.go` `luaMemRead8` `uint32(L.CheckInt(1))`"},
		{"mem.read16(addr) returns number and truncates addr to uint32", "`script_engine.go` `luaMemRead16` `uint32(L.CheckInt(1))`"},
		{"mem.read32(addr) returns number and truncates addr to uint32", "`script_engine.go` `luaMemRead32` `uint32(L.CheckInt(1))`"},
		{"mem.write8(addr, value) returns nothing and truncates addr to uint32", "`script_engine.go` `luaMemWrite8` `uint32(L.CheckInt(1))`"},
		{"mem.write16(addr, value) returns nothing and truncates addr to uint32", "`script_engine.go` `luaMemWrite16` `uint32(L.CheckInt(1))`"},
		{"mem.write32(addr, value) returns nothing and truncates addr to uint32", "`script_engine.go` `luaMemWrite32` `uint32(L.CheckInt(1))`"},
		{"mem.read_block(addr, len) returns a raw byte string; len must be >= 0", "`script_engine.go` `luaMemReadBlock` length check and `lua.LString` return"},
		{"mem.write_block(addr, bytes) writes a raw byte string and returns nothing", "`script_engine.go` `luaMemWriteBlock` byte loop"},
		{"mem.fill(addr, len, value) fills bytes, returns nothing, and requires len >= 0", "`script_engine.go` `luaMemFill` length check and write loop"},
		{"bit32.lshift(x, disp) masks disp to 0..31 and returns number", "`script_engine.go` `registerBit32` `lshift`"},
		{"bit32.rshift(x, disp) masks disp to 0..31 and returns number", "`script_engine.go` `registerBit32` `rshift`"},
		{"bit32.arshift(x, disp) masks disp to 0..31, sign-extends, and returns number", "`script_engine.go` `registerBit32` `arshift`"},
		{"bit32.lrotate(x, disp) masks disp to 0..31 and returns number", "`script_engine.go` `registerBit32` `lrotate`"},
		{"bit32.rrotate(x, disp) masks disp to 0..31 and returns number", "`script_engine.go` `registerBit32` `rrotate`"},
		{"bit32.btest(...) returns boolean true when the bitwise AND result is non-zero", "`script_engine.go` `registerBit32` `btest`"},
		{"bit32.extract(x, field[, width]) raises an error for field < 0, width <= 0, or field + width > 32", "`script_engine.go` `registerBit32` `extract` range check"},
		{"bit32.replace(x, v, field[, width]) raises an error for field < 0, width <= 0, or field + width > 32", "`script_engine.go` `registerBit32` `replace` range check"},
		{"dbg.history_horizon() returns snapshots, checkpoints, deltas, capacity, delta_bytes, checkpoint_interval, checkpoint_mib, retained_checkpoints, and devices", "`script_engine.go` `luaDbgHistoryHorizon` table fields"},
		{"dbg.history_config([opts]) accepts delta_interval, delta_mib, checkpoints, and snapshots as positive table fields", "`script_engine.go` `luaDbgHistoryConfig` option fields and positive-value check"},
		{"dbg.history_config([opts]) returns delta_interval, delta_mib, checkpoints, and snapshots", "`script_engine.go` `luaDbgHistoryConfig` return table fields"},
		{"media.type() returns sid, psg, ted, ahx, pokey, mod, wav, midi, or none", "`script_engine.go` `mediaTypeToString`, `media_loader.go` MIDI extension detection"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "IEScript",
			Kind:     "api contract",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	if len(facts) == 0 {
		t.Fatal("no IEScript bindings parsed from registerModules")
	}
	sortSDKSourceFacts(facts)
	return facts
}

func sdkArchitectureFactsFromSource(t *testing.T) []sdkSourceFact {
	t.Helper()
	categoryEvidence := map[string][]string{
		"Audio Subsystem": goFilesByPrefix(t, "audio_", "sid_", "ted_audio_", "pokey_", "ahx_", "mod_", "wav_", "midi_", "paula_"),
		"Bus and RAM":     goFilesByPrefix(t, "machine_bus", "memory_sizing", "boot_guest_ram", "profile_bounds"),
		"CPU Subsystem":   goFilesByPrefix(t, "cpu_", "debug_cpu_"),
		"Debug monitor":   goFilesByPrefix(t, "debug_"),
		"File I/O":        goFilesByPrefix(t, "media_", "host_helper_", "gemdos_", "disk_", "file_"),
		"JIT":             goFilesByPrefix(t, "jit_", "amd64_", "arm64_"),
		"Lua Scripting":   goFilesByPrefix(t, "script_"),
		"Snapshot":        goFilesByPrefix(t, "debug_snapshot", "snapshot_"),
		"Video Subsystem": goFilesByPrefix(t, "video_", "vga_", "ula_", "antic_", "gtia_", "ted_video_", "voodoo_", "copper_", "blitter_"),
	}
	var facts []sdkSourceFact
	for category, files := range categoryEvidence {
		if len(files) == 0 {
			t.Fatalf("architecture category %q has no source evidence", category)
		}
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "public architecture category",
			Name:     category,
			Evidence: "`" + strings.Join(files, "`, `") + "`",
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{sdkHexRange(0x00000, IO_REGION_START-0x1001), "`machine_bus.go` `VECTOR_TABLE`/`PROG_START`/`STACK_START`/`IO_REGION_START` low-RAM boundary constants"},
		{sdkHexRange(0x9F000, 0x9FFFF), "`cpu_ie32.go` and `cpu_ie64.go` reset stack seed convention"},
		{sdkHexRange(VGA_VRAM_BASE, VGA_VRAM_END), "`registers.go` `VGA_VRAM_BASE`/`VGA_VRAM_END`, `main.go` `MapIO`"},
		{sdkHexRange(VGA_TEXT_BASE, VGA_TEXT_END), "`registers.go` `VGA_TEXT_BASE`/`VGA_TEXT_END`, `main.go` `MapIO`"},
		{sdkHexRange(VOODOO_TEXMEM_BASE, VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE-1), "`voodoo_constants.go` texture-memory constants, `main.go` `MapIO`"},
		{sdkHexRange(VIDEO_CTRL, VIDEO_REG_END), "`video_chip.go` `VIDEO_CTRL`/`VIDEO_REG_END`, `main.go` `MapIO`"},
		{sdkHexRange(TERMINAL_REGION_BASE, TERMINAL_REGION_END), "`registers.go` `TERMINAL_REGION_BASE`/`TERMINAL_REGION_END`, `main.go` `MapIO`"},
		{sdkHexRange(VRAM_START, VRAM_START+VRAM_SIZE-1), "`video_chip.go` `VRAM_START`/`VRAM_SIZE`, `main.go` `MapIO`"},
		{sdkHexRange(AUDIO_CTRL, AUDIO_REG_END), "`audio_chip.go` `AUDIO_CTRL`/`AUDIO_REG_END`, `main.go` `MapIO`"},
		{sdkHexRange(AHX_BASE, AHX_SUBSONG), "`ahx_constants.go` `AHX_BASE`/`AHX_SUBSONG`, `main.go` `MapIO`"},
		{sdkHexRange(MIDI_PLAY_PTR, MIDI_END), "`midi_constants.go` `MIDI_PLAY_PTR`/`MIDI_END`, `main.go` `MapIO`"},
		{sdkHexRange(MOD_PLAY_PTR, MOD_END), "`mod_constants.go` `MOD_PLAY_PTR`/`MOD_END`, `main.go` `MapIO`"},
		{sdkHexRange(WAV_PLAY_PTR, WAV_END), "`wav_constants.go` `WAV_PLAY_PTR`/`WAV_END`, `main.go` `MapIO`"},
		{sdkHexRange(PSG_BASE, PSG_END), "`psg_constants.go` `PSG_BASE`/`PSG_END`, `main.go` `MapIO`"},
		{sdkHexRange(PSG_PLAY_PTR, PSG_PLAY_STATUS+3), "`psg_constants.go` `PSG_PLAY_PTR`/`PSG_PLAY_STATUS`, `main.go` `MapIO`"},
		{sdkHexRange(PSG_PLUS_CTRL, PSG_PLUS_CTRL), "`psg_constants.go` `PSG_PLUS_CTRL`, `main.go` `MapIO`"},
		{sdkHexRange(SN_BASE, SN_END), "`sn76489_constants.go` `SN_BASE`/`SN_END`, `main.go` `MapIO`"},
		{sdkHexRange(SID2_FLEX_BASE, SID2_FLEX_END), "`audio_chip.go` SID2 FLEX constants, `main.go` `MapIO`"},
		{sdkHexRange(POKEY_BASE, POKEY_END), "`pokey_constants.go` `POKEY_BASE`/`POKEY_END`, `main.go` `MapIO`"},
		{sdkHexRange(SAP_PLAY_PTR, SAP_SUBSONG), "`pokey_constants.go` SAP player constants, `main.go` `MapIO`"},
		{sdkHexRange(SID3_FLEX_BASE, SID3_FLEX_END), "`audio_chip.go` SID3 FLEX constants, `main.go` `MapIO`"},
		{sdkHexRange(SID_BASE, SID_END), "`sid_constants.go` `SID_BASE`/`SID_END`, `main.go` `MapIO`"},
		{sdkHexRange(SID_PLAY_PTR, SID_SUBSONG), "`sid_constants.go` SID player constants, `main.go` `MapIO`"},
		{sdkHexRange(SID2_BASE, SID2_END), "`sid_constants.go` `SID2_BASE`/`SID2_END`, `main.go` `MapIO`"},
		{sdkHexRange(SID3_BASE, SID3_END), "`sid_constants.go` `SID3_BASE`/`SID3_END`, `main.go` `MapIO`"},
		{sdkHexRange(IE_SFX_REGION_BASE, IE_SFX_REGION_END), "`sfx_constants.go` SFX constants, `main.go` `MapIO`"},
		{sdkHexRange(TED_BASE, TED_END), "`ted_constants.go` `TED_BASE`/`TED_END`, `main.go` `MapIO`"},
		{sdkHexRange(TED_PLAY_PTR, TED_PLAY_STATUS+3), "`ted_constants.go` TED player constants, `main.go` `MapIO`"},
		{sdkHexRange(TED_VIDEO_BASE, TED_VIDEO_END), "`ted_video_constants.go` `TED_VIDEO_BASE`/`TED_VIDEO_END`, `main.go` `MapIO`"},
		{sdkHexRange(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1), "`ted_video_constants.go` `TED_V_VRAM_BASE`/`TED_V_VRAM_SIZE`, `main.go` `MapIO`"},
		{sdkHexRange(VGA_BASE, VGA_REG_END), "`vga_constants.go` `VGA_BASE`/`VGA_REG_END`, `main.go` `MapIO`"},
		{sdkHexRange(HOST_MMIO_REGION_BASE, HOST_MMIO_REGION_END), "`registers.go` host-helper constants, `main.go` host helper registration"},
		{sdkHexRange(ULA_BASE, ULA_REG_END), "`ula_constants.go` `ULA_BASE`/`ULA_REG_END`, `main.go` `MapIO`"},
		{sdkHexRange(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END), "`ula_constants.go` ULA VRAM aperture constants, `main.go` `MapIO`"},
		{sdkHexRange(ANTIC_BASE, ANTIC_END), "`antic_constants.go` `ANTIC_BASE`/`ANTIC_END`, `main.go` `MapIO`"},
		{sdkHexRange(GTIA_BASE, GTIA_END), "`antic_constants.go` `GTIA_BASE`/`GTIA_END`, `main.go` `MapIO`"},
		{sdkHexRange(FILE_IO_BASE, FILE_IO_END), "`file_io_constants.go` `FILE_IO_BASE`/`FILE_IO_END`, `main.go` `MapIO`"},
		{sdkHexRange(FILE_DATA_PTR64, FILE_DATA_PTR64_END), "`file_io_constants.go` `FILE_DATA_PTR64`/`FILE_DATA_PTR64_END`, `main.go` `MapIO64`"},
		{sdkHexRange(AROS_DOS_BASE, AROS_DOS_END), "`aros_dos_constants.go` `AROS_DOS_BASE`/`AROS_DOS_END`, `main.go` `MapIO`"},
		{sdkHexRange(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END), "`aros_audio_constants.go` `AROS_AUD_REGION_BASE`/`AROS_AUD_REGION_END`, `main.go` `MapIO`"},
		{sdkHexRange(MEDIA_LOADER_BASE, MEDIA_LOADER_END), "`media_loader_constants.go` `MEDIA_LOADER_BASE`/`MEDIA_LOADER_END`, `main.go` `MapIO`"},
		{sdkHexRange(EXEC_BASE, EXEC_END), "`program_executor_constants.go` `EXEC_BASE`/`EXEC_END`, `main.go` `MapIO`"},
		{sdkHexRange(MEDIA_STAGING_BASE, MEDIA_STAGING_END), "`media_loader_constants.go` `MEDIA_STAGING_BASE`/`MEDIA_STAGING_END`"},
		{sdkHexRange(COPROC_BASE, COPROC_END), "`coprocessor_constants.go` `COPROC_BASE`/`COPROC_END`, `main.go` `MapIO`"},
		{sdkHexRange(CLIP_REGION_BASE, CLIP_REGION_END), "`clipboard_bridge_constants.go` `CLIP_REGION_BASE`/`CLIP_REGION_END`, `main.go` `MapIO`"},
		{sdkHexRange(COPROC_EXT_BASE, COPROC_EXT_END), "`coprocessor_constants.go` `COPROC_EXT_BASE`/`COPROC_EXT_END`, `main.go` `MapIO`"},
		{sdkHexRange(IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END), "`registers.go` IRQ diagnostic constants; `aros_loader.go` `MapIRQDiagnostics`; `main.go` AROS call sites; `machine_lifecycle.go` AROS reset loader call site; `aros_audio_dma.go` `UnmapIO` teardown"},
		{sdkHexRange(BOOT_HOSTFS_BASE, BOOT_HOSTFS_END), "`bootstrap_hostfs_constants.go` `BOOT_HOSTFS_BASE`/`BOOT_HOSTFS_END`, `main.go` `MapIO`"},
		{sdkHexRange(SYSINFO_REGION_BASE, SYSINFO_REGION_END), "`sysinfo_mmio.go` `RegisterSysInfoMMIOFromBus`, `main.go` registration"},
		{sdkHexRange(VOODOO_BASE, VOODOO_END), "`voodoo_constants.go` `VOODOO_BASE`/`VOODOO_END`, `main.go` `MapIO`"},
		{sdkHexRange(VOODOO_FOG_TABLE_BASE, VOODOO_FOG_TABLE_END-1), "`voodoo_constants.go` fog-table constants"},
		{sdkHexRange(WORKER_IE32_BASE, WORKER_IE32_END), "`coprocessor_constants.go` IE32 worker-memory constants"},
		{sdkHexRange(WORKER_M68K_BASE, WORKER_M68K_END), "`coprocessor_constants.go` M68K worker-memory constants"},
		{sdkHexRange(WORKER_6502_BASE, WORKER_6502_END), "`coprocessor_constants.go` 6502 worker-memory constants"},
		{sdkHexRange(WORKER_Z80_BASE, WORKER_Z80_END), "`coprocessor_constants.go` Z80 worker-memory constants"},
		{sdkHexRange(WORKER_X86_BASE, WORKER_X86_END), "`coprocessor_constants.go` x86 worker-memory constants"},
		{sdkHexRange(WORKER_IE64_BASE, WORKER_IE64_END), "`coprocessor_constants.go` IE64 worker-memory constants"},
		{sdkHexRange(MAILBOX_BASE, MAILBOX_END), "`coprocessor_constants.go` `MAILBOX_BASE`/`MAILBOX_END`"},
		{sdkHexRange(0x800000, 0x1DFFFFF), "`main.go` AROS profile fast-memory allocation convention"},
		{sdkHexRange(0x1E00000, 0x5DFFFFF), "`main.go` AROS profile video-memory allocation convention"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "memory map row",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{sdkHexRange(VIDEO_REG_BASE+VIDEO_REG_OFFSET_BLT_MODE7_U0, VIDEO_REG_BASE+VIDEO_REG_OFFSET_BLT_MODE7_TEX_H), "`video_chip.go` Mode7 register offsets"},
		{"0xF0900-0xF093F except 0xF0914 and 0xF0918", "`audio_chip.go` square legacy register constants and sweep dispatch exceptions"},
		{"0xF0940-0xF097F plus 0xF0914", "`audio_chip.go` triangle legacy register constants and `TRI_SWEEP`"},
		{"0xF0980-0xF09BF plus 0xF0918", "`audio_chip.go` sine legacy register constants and `SINE_SWEEP`"},
		{sdkHexRange(NOISE_FREQ, 0xF09FF), "`audio_chip.go` noise legacy register constants"},
		{sdkHexRange(SYNC_SOURCE_CH0, RING_MOD_SOURCE_CH3), "`audio_chip.go` sync/ring-mod source constants"},
		{sdkHexRange(SAW_REG_START, SAW_REG_END), "`audio_chip.go` sawtooth legacy register constants"},
		{sdkHexRange(FILTER_CUTOFF, FILTER_MOD_AMOUNT), "`audio_chip.go` global filter register constants"},
		{sdkHexRange(FLEX_CH_BASE, FLEX_CH_PRIMARY_END), "`audio_chip.go` primary FLEX channel constants"},
		{sdkHexRange(SID2_FLEX_BASE, SID2_FLEX_END), "`audio_chip.go` SID2 FLEX channel constants"},
		{sdkHexRange(SID3_FLEX_BASE, SID3_FLEX_END), "`audio_chip.go` SID3 FLEX channel constants"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "memory map subrange",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"$A0-$AD | VGA | 0xF1000 | Direct register map (MODE, STATUS, CTRL, SEQ, CRTC, GC, DAC, DAC read index, DAC mask, VRAM bank)", "`vga_constants.go` `Z80_VGA_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out`"},
		{"$D620-$D632 | TED Video | 0xF0F20+offset x4 | Stride-4 register mapping including raster compare registers", "`ted_video_constants.go` `C6502_TED_V_*`, `cpu_six5go2.go` `readTEDPage`/`writeTEDPage`"},
		{"x86 does not implement standard PC VGA I/O ports; VGA access is through the shared bus MMIO aperture and the direct $A0000-$AFFFF VRAM memory window.", "`cpu_x86_runner.go` `X86BusAdapter.In`/`Out` omit VGA port cases and `translateVRAM` handles the VRAM window"},
		{"$D700-$D70D | VGA | 0xF1000 | Direct handler call plus DAC read index, DAC mask, and VRAM bank", "`vga_constants.go` `C6502_VGA_*`, `cpu_six5go2.go` `Bus6502Adapter.Read`/`Write`"},
		{"$E4/$E5 | SN76489 | 0xF0C30/0xF0C31 | Data write / last-written read and ready-status read", "`sn76489_constants.go` `Z80_SN_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out`"},
		{"$F2/$F3 | TED | 0xF0F00 / 0xF0F20-0xF0F6B | Register select / data (audio indices $00-$05, video indices $20-$32 x4 stride)", "`ted_constants.go` `TED_REG_COUNT`, `ted_video_constants.go` `TED_V_INDEX_*`/`Z80_TED_V_INDEX_*`, `cpu_x86_runner.go` `X86_TED_V_INDEX_*`"},
		{"$FE | ULA | 0xF2000 | Border colour only (bits 0-2)", "`cpu_x86_runner.go` `X86BusAdapter.In`/`Out` ULA border-port case"},
		{"$FE/$FD/$BE/$FA/$FB/$FC | ULA | 0xF2000-0xF2014 | Border, control, status, VRAM address latch low/high, and paged VRAM data", "`ula_constants.go` `Z80_ULA_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out`"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "cpu bridge row",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"Linux amd64 | IE64, 6502, M68K, Z80, x86", "`jit_dispatch.go`, `jit_6502_dispatch.go`, `jit_m68k_dispatch.go`, `jit_z80_dispatch.go`, `jit_x86_dispatch.go` build tags"},
		{"Linux arm64 | IE64", "`jit_dispatch.go`, `jit_z80_dispatch.go` runtime amd64 guard, non-IE64 stubs"},
		{"Windows amd64 | IE64, 6502, M68K, Z80, x86", "`jit_dispatch.go`, `jit_6502_dispatch.go`, `jit_m68k_dispatch.go`, `jit_z80_dispatch.go`, `jit_x86_dispatch.go` build tags"},
		{"Windows arm64 | IE64", "`jit_dispatch.go` arm64 windows tag plus non-IE64 stubs"},
		{"macOS amd64 | IE64, 6502, M68K, Z80, x86", "`jit_dispatch.go`, amd64 per-core dispatch files"},
		{"macOS arm64 | IE64", "`jit_dispatch.go` arm64 darwin tag plus non-IE64 stubs"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "jit matrix row",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"Bare .ie68 uses the active-visible RAM ceiling; EmuTOS and AROS M68K loader modes use profile bounds.", "`boot_guest_ram.go` `resolveModeCaps`/`resolveActiveVisibleCeiling` cases for `modeM68KBare`, `modeEmuTOS`, and `modeAros`"},
		{"Darwin RAM sizing uses a page-aligned conservative half of hw.memsize as the detected base before applying the per-platform reserve.", "`memory_sizing_usable_darwin.go` `unix.SysctlUint64(\"hw.memsize\")`, `pageAlignDown(total / 2)`, and `memory_sizing.go` `ReserveFor`"},
		{"Each ring has 16 descriptor slots but uses one slot to distinguish full from empty, so it can hold 15 queued requests at once.", "`coprocessor_constants.go` ring constants and coprocessor queue implementation"},
		{"EXEC_CTRL operation values: 1=Execute, 2=EmuTOS, 3=AROS, 4=IntuitionOS IExec, 5=Hard reset", "`program_executor_constants.go` `EXEC_OP_*`, `program_executor.go` `HandleWrite` dispatch, `program_executor_test.go` operation-value pins"},
		{"mem.* helpers are raw 32-bit bus helpers, not an above-4GiB IE64 RAM or CPU-virtual-address API.", "`script_engine.go` mem helpers cast addresses to `uint32`"},
		{"Mutable devices join the snapshot contract through MachineMonitor.RegisterSnapshotDevice.", "`debug_monitor.go` `RegisterSnapshotDevice`, `main.go` registrations"},
		{"Video compositor default scale mode is stretch-fill; F11 toggles non-16:9 sources to aspect-fit.", "`video_compositor.go` `NewVideoCompositor`/`ToggleScaleModeIfNonNative`, `video_compositor_test.go` default-scale regression"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "Architecture",
			Kind:     "architecture claim",
			Name:     row.name,
			Evidence: row.evidence,
		})
	}
	sortSDKSourceFacts(facts)
	return facts
}

func sdkHexRange(start, end uint32) string {
	if start == end {
		return fmt.Sprintf("0x%05X", start)
	}
	return fmt.Sprintf("0x%05X-0x%05X", start, end)
}

type sdkMonitorHelpEntry struct {
	Name     string
	Summary  string
	Syntax   []string
	Examples []string
}

func sourceBetween(t *testing.T, source, start, end string) string {
	t.Helper()
	startIdx := strings.Index(source, start)
	if startIdx < 0 {
		t.Fatalf("source start marker not found: %s", start)
	}
	endIdx := strings.Index(source[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("source end marker not found after %s: %s", start, end)
	}
	return source[startIdx : startIdx+endIdx]
}

func parseQuotedNamesAfterPrefix(source, prefixPattern string) []string {
	re := regexp.MustCompile(prefixPattern + `"([^"]+)"`)
	var names []string
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		names = append(names, m[1])
	}
	return names
}

func parseSwitchCaseNames(source string) []string {
	re := regexp.MustCompile(`(?m)^\s*case\s+([^:]+):`)
	quoted := regexp.MustCompile(`"([^"]+)"`)
	seen := map[string]bool{}
	var names []string
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		for _, q := range quoted.FindAllStringSubmatch(m[1], -1) {
			if !seen[q[1]] {
				seen[q[1]] = true
				names = append(names, q[1])
			}
		}
	}
	return names
}

func parseMonitorHelpEntries(source string) []sdkMonitorHelpEntry {
	re := regexp.MustCompile(`(?m)^\s*\{Name:\s*"([^"]+)",\s*Summary:\s*"([^"]+)",\s*Syntax:\s*\[\]string\{([^}]*)\},\s*Examples:\s*\[\]string\{([^}]*)\}\},`)
	var entries []sdkMonitorHelpEntry
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		entries = append(entries, sdkMonitorHelpEntry{
			Name:     m[1],
			Summary:  m[2],
			Syntax:   parseGoStringList(m[3]),
			Examples: parseGoStringList(m[4]),
		})
	}
	return entries
}

func parseGoStringList(source string) []string {
	re := regexp.MustCompile(`"([^"]*)"`)
	var values []string
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		values = append(values, m[1])
	}
	return values
}

func goFilesByPrefix(t *testing.T, prefixes ...string) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read repository root: %v", err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				files = append(files, filepath.ToSlash(name))
				break
			}
		}
	}
	sort.Strings(files)
	return files
}

func manualMentionsCodeToken(doc, token string) bool {
	for _, needle := range []string{
		"`" + token + "`",
		"`" + token + "(",
		"`" + strings.TrimPrefix(token, "?") + "`",
		" " + token + " ",
		"|" + token + "|",
		"| " + token + " ",
	} {
		if strings.Contains(doc, needle) {
			return true
		}
	}
	return false
}

func normalizedContains(haystack, needle string) bool {
	clean := func(s string) string {
		replacer := strings.NewReplacer("`", "", `\|`, "|", "\n", " ", "\t", " ")
		return strings.ToLower(strings.Join(strings.Fields(replacer.Replace(s)), " "))
	}
	return strings.Contains(clean(haystack), clean(needle))
}

func escapeMarkdownTableCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

func sortSDKSourceFacts(facts []sdkSourceFact) {
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Surface != facts[j].Surface {
			return facts[i].Surface < facts[j].Surface
		}
		if facts[i].Kind != facts[j].Kind {
			return facts[i].Kind < facts[j].Kind
		}
		return facts[i].Name < facts[j].Name
	})
}
