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

func TestZ80JITDocumentationRejectsStalePartialBackendClaims(t *testing.T) {
	paths := []string{"sdk/docs/Z80_JIT.md", "sdk/docs/architecture.md", "sdk/docs/iescript.md", "sdk/docs/iemon.md", "sdk/docs/wasm.md"}
	stale := []string{"NOP and register-only loads", "Z80 emits NOP, register-only loads", "frozen canonical helpers execute remaining forms", "remaining Z80 forms use frozen canonical helpers", "ARM64 direct coverage", "other forms use canonical helpers", "breaks on HALT"}
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, claim := range stale {
			if strings.Contains(string(contents), claim) {
				t.Errorf("%s retains stale Z80 JIT claim %q", path, claim)
			}
		}
	}
}

func TestSDKIEMonManualCoverageMatchesSourceInventory(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	for _, fact := range sdkIEMonFactsFromSource(t) {
		if fact.Kind != "command" && fact.Kind != "dispatch alias" && fact.Kind != "command syntax" && fact.Kind != "region divergence row" && fact.Kind != "io view" && fact.Kind != "monitor contract" {
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
	for path, needle := range map[string]string{
		"cpu_ie64.go":        "jitEnabled:     jitAvailable || wasmJITSupported",
		"cpu_m68k.go":        "m68kJitEnabled:  m68kJitAvailable",
		"cpu_z80.go":         "jitEnabled: z80JitAvailable",
		"cpu_six5go2.go":     "jitEnabled:    true",
		"cpu_x86.go":         "x86JitEnabled: x86JitAvailable",
		"cpu_6502_runner.go": "JITEnabled: !config.DisableJIT",
		"cpu_z80_runner.go":  "cpu.jitEnabled = z80JitAvailable && !config.DisableJIT",
		"cpu_x86_runner.go":  "cpu.x86JitEnabled = x86JitAvailable && (config == nil || !config.DisableJIT)",
	} {
		if !strings.Contains(readAuditFile(t, path), needle) {
			t.Fatalf("%s JIT default changed; review architecture.md and iescript.md: %s", path, needle)
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
	ioview := readAuditFile(t, "debug_ioview.go")
	for _, needle := range []string{
		`"midilive":`,
		`{"LIVE_DATA", IE_MIDI_LIVE_DATA, 1, "WO"}`,
		`{"LIVE_STATUS", IE_MIDI_LIVE_STATUS, 1, "RO"}`,
		`{"LIVE_CTRL", IE_MIDI_LIVE_CTRL, 1, "WO"}`,
	} {
		if !strings.Contains(ioview, needle) {
			t.Fatalf("debug_ioview.go live-MIDI I/O view changed; review iemon.md: %s", needle)
		}
	}
	for _, row := range []struct {
		name     string
		evidence string
	}{
		{"midilive", "`debug_ioview.go` `ioDevices` key"},
		{"LIVE_DATA ($F0BF4) = $00 [0] WO", "`debug_ioview.go` `LIVE_DATA` descriptor"},
		{"LIVE_STATUS ($F0BF5) = $01 [1] RO", "`debug_ioview.go` `LIVE_STATUS` descriptor"},
		{"LIVE_CTRL ($F0BF6) = $00 [0] WO", "`debug_ioview.go` `LIVE_CTRL` descriptor"},
	} {
		facts = append(facts, sdkSourceFact{
			Surface:  "IEMon",
			Kind:     "io view",
			Name:     row.name,
			Evidence: row.evidence,
		})
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
	monitor := readAuditFile(t, "debug_monitor.go")
	for _, needle := range []string{
		"m.audioWasFrozen = m.soundChip.audioFrozen.Swap(true)",
		"if m.audioCmdInSession",
		"m.soundChip.audioFrozen.Store(m.audioWasFrozen)",
	} {
		if !strings.Contains(monitor, needle) {
			t.Fatalf("debug_monitor.go media-freeze contract changed; review iemon.md: %s", needle)
		}
	}
	facts = append(facts, sdkSourceFact{
		Surface:  "IEMon",
		Kind:     "monitor contract",
		Name:     "Entering the monitor freezes every guest CPU and the audio clock; leaving restores the pre-entry audio state unless fa or ta was issued during the session.",
		Evidence: "`debug_monitor.go` `freezeMediaOnEntry`/`resumeMediaOnExit`, `debug_commands.go` `cmdFreezeAudio`/`cmdThawAudio`",
	})
	facts = append(facts, sdkSourceFact{
		Surface:  "IEMon",
		Kind:     "monitor contract",
		Name:     "Second M68K, x86, and IE64 instances are labelled coproc:M68K#1, coproc:X86#1, and coproc:IE64#1 and receive separate monitor CPU IDs; cpu online starts instance 0 only.",
		Evidence: "`coprocessor_manager.go` `coprocInstanceLabel`/`RegisterCPU` and `WorkerInventory`",
	})
	facts = append(facts, sdkSourceFact{
		Surface:  "IEMon",
		Kind:     "monitor contract",
		Name:     "The first whole-machine reverse-history record automatically arms a bus page-dirty cursor and takes a full checkpoint.",
		Evidence: "`debug_commands.go` `recordWholeMachineHistory`, `debug_reverse_epoch.go` `ensureEpochHistoryLocked`",
	})
	cpuCommand := sourceBetween(t, source, "func (m *MachineMonitor) cmdCPU", "func (m *MachineMonitor) cmdCPUOnline")
	for _, needle := range []string{
		"running := entry.CPU.IsRunning()",
		"entry.CPU.Freeze()",
		"entry.CPU.Resume()",
		"m.showRegisters()",
		"m.showDisassembly(0, 8)",
	} {
		if !strings.Contains(cpuCommand, needle) {
			t.Fatalf("debug_commands.go CPU inspection coherence contract changed; review iemon.md: %s", needle)
		}
	}
	facts = append(facts, sdkSourceFact{
		Surface:  "IEMon",
		Kind:     "monitor contract",
		Name:     "When a thawed CPU is listed or selected for focus, IEMon temporarily freezes it to capture coherent state and then restores its prior running state.",
		Evidence: "`debug_commands.go` `cmdCPU` running-state capture around program-counter, register, and disassembly inspection",
	})
	sortSDKSourceFacts(facts)
	return facts
}

func sdkIEScriptFactsFromSource(t *testing.T) []sdkSourceFact {
	t.Helper()
	source := readAuditFile(t, "script_engine.go")
	statsFunction := sourceBetween(t, source, "func (se *ScriptEngine) luaCPUJITStats", "type m68kJITCompileFailureStat")
	ie64StatsBlock := sourceBetween(t, statsFunction, "case runtimeCPUIE64:", "case runtimeCPUX86:")
	x86StatsBlock := sourceBetween(t, statsFunction, "case runtimeCPUX86:", "L.Push(tbl)")
	for mode, block := range map[string]string{"IE64": ie64StatsBlock, "x86": x86StatsBlock} {
		for _, field := range map[string][]string{
			"IE64": {"backend", "instruction_count", "native_entries", "native_retired", "compiled_blocks", "compiled_regions", "region_candidates", "region_rejections", "fallback_instructions", "helper_exits", "helper_resumes", "helper_resume_cancellations", "io_bails", "invalidations", "cache_hits", "cache_misses", "spills", "fpu_spills", "direct_ram_proofs", "inlined_calls"},
			"x86":  {"backend", "instruction_count", "native_entries", "native_retired", "compiled_blocks", "compiled_regions", "region_candidates", "fallback_instructions", "helper_exits", "io_bails", "invalidations", "invalidated_blocks", "chain_exits", "cache_hits", "cache_misses", "code_cache_resets"},
		}[mode] {
			if !strings.Contains(block, `L.SetField(tbl, "`+field+`"`) {
				t.Fatalf("script_engine.go %s JIT statistics table changed; review iescript.md: %s", mode, field)
			}
		}
	}
	statsSource := readAuditFile(t, "jit_ies_stats.go")
	for _, needle := range []string{
		"Each CPU owns its counters",
		"func (s *cpuJITStats) reset()",
		"NativeEntries:             s.nativeEntries.Load()",
		"FallbackInstructions:      s.fallbackInstructions.Load()",
	} {
		if !strings.Contains(statsSource, needle) {
			t.Fatalf("JIT statistics ownership or reset contract changed; review iescript.md: %s", needle)
		}
	}
	for _, needle := range []string{
		"base := VGA_PALETTE + idx*3",
		"se.bus.Write8(base, uint8(r))",
		"se.bus.Read8(base + 2)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go VGA palette API contract changed; review iescript.md: %s", needle)
		}
	}
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
	registerBit32 := sourceBetween(t, source, "func (se *ScriptEngine) registerBit32", "func (se *ScriptEngine) onFrameTiming")
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
	for _, needle := range []string{
		"case runtimeCPU6502:",
		`L.SetField(tbl, "instruction_count"`,
		`L.SetField(tbl, "tier1_blocks"`,
		`L.SetField(tbl, "native_entries"`,
		`L.SetField(tbl, "bailouts"`,
		`L.SetField(tbl, "invalidations"`,
		`L.SetField(tbl, "chain_exits"`,
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go 6502 JIT statistics contract changed; review iescript.md: %s", needle)
		}
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
		{"sys.perf_report() returns a string subsystem performance report; it is empty when IE_PERF_ACCT is off or no subsystem counters have recorded work", "`script_engine.go` `luaSysPerfReport`, `perf_accounting_subsys.go` `Report`"},
		{"sys.perf_reset() resets subsystem performance counters and returns nothing", "`script_engine.go` `luaSysPerfReset`, `perf_accounting_subsys.go` `Reset`"},
		{"An explicit budget must be an integer greater than or equal to zero and limits the number of frame notifications consumed; zero performs only the immediate evaluation.", "`script_engine.go` `luaSysWaitUntil`, `script_batching_test.go` bounded-wait coverage"},
		{"Each pair performs its own ordered 32-bit bus write, exactly as audio.write_reg does.", "`script_engine.go` `luaAudioWriteRegs`, `script_batching_test.go` ordered-write coverage"},
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
		{"dbg.mmio_stats() returns rows with start, end, name, reads, and writes", "`script_engine.go` `luaDbgMMIOStats`, `mmio_stats.go` `MMIOStatsSnapshot`"},
		{"media.type() returns sid, psg, ted, ahx, pokey, mod, wav, midi, or none", "`script_engine.go` `mediaTypeToString`, `media_loader.go` MIDI extension detection"},
		{"Supported for IE32, m68k, z80, x86, 6502, and ie64", "`script_engine.go` `luaCPUJITEnabled` selected-CPU switch"},
		{"IE32 is available on Linux x64, Linux arm64, and browser js/wasm when its runtime backend is available.", "`script_engine.go` `luaCPUSetJITEnabled`, `jit_ie32_available_linux.go`, `jit_ie32_available_wasm.go`"},
		{"In IE32 mode, returns backend, instruction_count, native_entries, compiled_blocks, compiled_regions, hot_recompilations, retired_instructions, direct_instructions, helper_instructions, helper_exits, helper_resumes, chains, chain_budget_exits, deoptimizations, helper_deopts, source_stamp_deopts, code_cache_resets, invalidations, invalidated_blocks, cache_hits, return_cache_hits, mmio_poll_iterations, mmio_store_helpers, resident_spills_saved, counted_loops, and profitability_fallbacks.", "`script_engine.go` `luaCPUJITStats`, `jit_ie32_policy.go` `JITStats`"},
		{"In m68k mode, returns instruction_count, native_blocks, native_retired, native_chain_instructions, native_no_chain_returns, native_helper_exits, native_exception_exits, native_invalidation_exits, native_mmio_guard_exits, unsupported_one_exits, compile_failure_exits, transcendental_bursts, warmup_instructions, region_promotions, last_native_pc, fallback_instructions, bailouts, last_fallback_pc, last_fallback_opcode, fallback_opcodes, native_pcs, native_invalidation_pcs, native_pc_ring, and compile_failures.", "`script_engine.go` `luaCPUJITStats` m68k table fields"},
		{"In IE64 mode, returns backend, instruction_count, native_entries, native_retired, compiled_blocks, compiled_regions, region_candidates, region_rejections, fallback_instructions, helper_exits, helper_resumes, helper_resume_cancellations, io_bails, invalidations, cache_hits, cache_misses, spills, fpu_spills, direct_ram_proofs, and inlined_calls.", "`script_engine.go` `luaCPUJITStats`, `jit_ies_stats.go`, and IE64 native and wasm dispatchers"},
		{"In x86 mode, returns backend, instruction_count, native_entries, native_retired, compiled_blocks, compiled_regions, region_candidates, fallback_instructions, helper_exits, io_bails, invalidations, invalidated_blocks, chain_exits, cache_hits, cache_misses, and code_cache_resets.", "`script_engine.go` `luaCPUJITStats`, `jit_ies_stats.go`, and x86 amd64, ARM64, and wasm dispatchers"},
		{"In 6502 mode, returns CPU-owned instruction_count, tier1_blocks, native_entries, bailouts, invalidations, and chain_exits. Reset clears all 6502 counters.", "`script_engine.go` `luaCPUJITStats`, `cpu_six5go2.go` `Reset`, and `jit_6502_policy.go` `resetJITStats`"},
		{"For IE64, instruction_count is the processor's total retired-instruction count. For x86, it is the total accounted by the JIT dispatcher and equals native_retired plus fallback_instructions.", "`script_engine.go` `luaCPUJITStats`, `jit_exec_test.go` and `jit_x86_exec_test.go` execution-provenance coverage"},
		{"native_entries counts entries into generated code, native_retired counts instructions retired there, and fallback_instructions counts instructions executed through interpreter fallback while JIT dispatch is active.", "`jit_exec.go`, `jit_exec_wasm.go`, `jit_x86_exec.go`, `jit_x86_exec_arm64.go`, and `jit_x86_exec_wasm.go` counter updates"},
		{"IE64 and x86 counters are owned by one CPU instance and reset when that CPU resets or loads a program; they do not require the console statistics switches.", "`jit_ies_stats.go` ownership/reset, CPU reset/load paths, and `script_engine_test.go` `TestCPUJITStatsResetAndOwnership`"},
		{"compiled_blocks counts installed single-block compilations, compiled_regions counts successful promoted regions, and region_candidates counts promotion attempts. IE64 region_rejections counts candidates that do not install a region.", "IE64 and x86 native/ARM64/wasm compilation and promotion paths"},
		{"cache_hits and cache_misses count JIT block-cache lookup outcomes. invalidations counts code-write invalidation events; x86 invalidated_blocks counts blocks removed and code_cache_resets counts complete cache or allocator resets.", "IE64 and x86 native/ARM64/wasm cache and invalidation paths"},
		{"IE64 helper_resumes counts generated-code continuations after helper completion, helper_resume_cancellations counts rejected continuations, and spills, fpu_spills, direct_ram_proofs, and inlined_calls are cumulative compiler metadata for installed compilation units.", "IE64 helper dispatch/resume and native/wasm block installation paths"},
		{"helper_exits counts semantic helper hand-offs and io_bails counts I/O or guarded-memory hand-offs. x86 chain_exits counts returns from generated block chains to the dispatcher.", "IE64 and x86 native/ARM64/wasm exit-accounting paths"},
		{"Available backends start enabled during normal CPU and runner construction; the primary command-line CPU starts disabled when --nojit is used.", "CPU constructors and runner defaults; `main.go` `nojit` flag and per-primary-CPU disable paths"},
		{"dbg.open() freezes every CPU and the audio clock; final dbg.close() restores the pre-entry audio state unless fa or ta changed it during the session", "`script_engine.go` `luaDbgOpen`/`luaDbgClose`, `debug_monitor.go` media-freeze entry/exit contract"},
		{"coproc.start(cpu_type, filename [, instance]), coproc.stop(cpu_type [, instance]), and coproc.enqueue(cpu_type, op, request [, instance]) default to instance 0; M68K, x86, and IE64 accept instance 1, while IE32, 6502, and Z80 reject it", "`script_engine.go` `coprocLuaInstance`/`luaCoprocStart`/`luaCoprocStop`/`luaCoprocEnqueue`, `coprocessor_constants.go` `coprocInstanceLimit`"},
		{"coproc.workers() returns cpu_type, instance, and is_running for every active instance without changing COPROC_CPU_TYPE or COPROC_INSTANCE", "`script_engine.go` `luaCoprocWorkers` reads `COPROC_INSTANCE_STATE`, `coprocessor_manager.go` `computeInstanceStateMask`"},
		{"rec.start() and rec.start_screen() follow wall-clock time; after an encoder stall they discard missed video-frame debt and matching oldest buffered audio instead of producing an unbounded catch-up burst", "`video_recorder.go` `loop`/`audioPump`/`sampleRing.discard`, `video_recorder_test.go` discard and cursor-protocol coverage"},
		{"rec.start() and rec.start_screen() pump video and audio independently; frozen or unchanged video is held, and audio starvation beyond 500 ms produces silence instead of stalling", "`video_recorder.go` wall-clock `loop`, independent `audioPump`, and `recorderAudioGraceTicks`; `video_recorder_test.go` audio-starvation coverage"},
		{"video.get_crt_mode(), video.set_crt_mode(mode), and video.cycle_crt_mode() expose the full flat, curved, off cycle used by F7. The boolean functions remain compatibility controls: enabling selects flat and their toggle only switches flat and off.", "`script_engine.go` mode bindings, `video_compositor.go` mode controller, `video_backend_ebiten.go` shared F7 transition, and `script_crt_control_test.go`"},
		{"video.vga_set_palette(idx, r, g, b) masks idx to 8 bits and stores the low 6 bits of each colour component; video.vga_get_palette(idx) returns those three 6-bit values.", "`script_engine.go` `luaVGASetPalette`/`luaVGAGetPalette`, `video_vga.go` direct palette access, and `script_engine_test.go` three-byte-entry coverage"},
		{"Return the host presentation scale mode: \"fit\" for aspect-fit or \"stretch\" for stretch-fill.", "`script_engine.go` `luaVideoGetScaleMode` and `script_scale_control_test.go`"},
		{"Raises if the selected output has no compositor.", "`script_engine.go` `luaVideoGetScaleMode` and `script_scale_control_test.go`"},
		{"This is a host presentation action and does not inject a guest key.", "`script_engine.go` `luaVideoSetScaleMode` and `script_scale_control_test.go`"},
		{"Raises for an invalid mode or if the selected output has no compositor.", "`script_engine.go` `luaVideoSetScaleMode` and `script_scale_control_test.go`"},
		{"rec.screenshot_composed(path) captures the GPU-composed image before CRT, cursor, and status-bar processing", "`script_engine.go` `luaRecScreenshotComposed`, `video_backend_ebiten.go` composition capture stage, and `video_crt_script_ebiten_test.go`"},
		{"rec.screenshot_screen(path) captures the final displayed frame after host presentation processing", "`script_engine.go` `luaRecScreenshotScreen`, `video_backend_ebiten.go` final screen capture stage, and `video_crt_script_ebiten_test.go`"},
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
	piManifest := readAuditFile(t, "scripts/rpi-ie-golden.manifest")
	piSession := readAuditFile(t, "scripts/rpi-live/ie-session.sh")
	piGreetd := readAuditFile(t, "scripts/rpi-live/greetd-config.toml")
	for _, check := range []struct {
		path   string
		text   string
		needle string
	}{
		{"scripts/rpi-ie-golden.manifest", piManifest, "os_release=Debian GNU/Linux 13 (trixie)"},
		{"scripts/rpi-live/ie-session.sh", piSession, `cage -s -- /opt/ie/ie-launch.sh`},
		{"scripts/rpi-live/ie-launch.sh", readAuditFile(t, "scripts/rpi-live/ie-launch.sh"), `/opt/ie/IntuitionEngine "$@"`},
		{"scripts/rpi-live/greetd-config.toml", piGreetd, "[initial_session]"},
		{"scripts/rpi-live/greetd-config.toml", piGreetd, `command = "/opt/ie/ie-session.sh"`},
	} {
		if !strings.Contains(check.text, check.needle) {
			t.Fatalf("%s Raspberry Pi appliance contract changed; review architecture.md: %s", check.path, check.needle)
		}
	}
	videoChip := readAuditFile(t, "video_chip.go")
	videoCompositor := readAuditFile(t, "video_compositor.go")
	for _, check := range []struct {
		path   string
		text   string
		needle string
	}{
		{"video_chip.go", videoChip, "chip.copperFrameClockOwners.Load() == 0 && !chip.copperManagedByCompositor"},
		{"video_chip.go", videoChip, "func (chip *VideoChip) acquireCompositorFrameClock()"},
		{"video_chip.go", videoChip, "func (chip *VideoChip) releaseCompositorFrameClock()"},
		{"video_compositor.go", videoCompositor, "acquireCompositorFrameClock(&c.sources[len(c.sources)-1])"},
		{"video_compositor.go", videoCompositor, "releaseCompositorFrameClock(&c.sources[i])"},
	} {
		if !strings.Contains(check.text, check.needle) {
			t.Fatalf("%s Copper frame-clock ownership changed; review architecture.md: %s", check.path, check.needle)
		}
	}
	z80Source := readAuditFile(t, "cpu_z80_runner.go")
	for _, needle := range []string{
		"translated >= VGA_TEXT_WINDOW && translated < VGA_TEXT_WINDOW+VGA_TEXT_SIZE",
		"b.vgaEngine.HandleTextWrite(translated, uint32(value))",
		"VGA text buffer at 0xB8000 (bank 0x2E = 46)",
	} {
		if !strings.Contains(z80Source, needle) {
			t.Fatalf("cpu_z80_runner.go VGA text-bank contract changed; review architecture.md: %s", needle)
		}
	}
	vgaSource := readAuditFile(t, "video_vga.go")
	for _, needle := range []string{
		"displayStartCell := int(v.getStartAddressInternal()) % textCells",
		"if blinkEnabled {",
		"bg &= 0x07",
		"cellAddress == cursorOff",
	} {
		if !strings.Contains(vgaSource, needle) {
			t.Fatalf("video_vga.go text-mode contract changed; review architecture.md: %s", needle)
		}
	}
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
	for path, needles := range map[string][]string{
		"jit_6502_dispatch.go":      {"//go:build (amd64 || arm64) && linux", "jit6502Available = true"},
		"jit_6502_dispatch_stub.go": {"//go:build !((amd64 || arm64) && linux) && !(js && wasm)", "cpu.Execute()"},
		"jit_6502_dispatch_wasm.go": {"//go:build js && wasm", `os.Getenv("P65_WASM_JIT") != "0"`, "cpu.ExecuteJIT6502()"},
	} {
		source := readAuditFile(t, path)
		for _, needle := range needles {
			if !strings.Contains(source, needle) {
				t.Fatalf("%s 6502 JIT platform contract changed; review architecture.md: %s", path, needle)
			}
		}
	}
	goMod := readAuditFile(t, "go.mod")
	if !strings.Contains(goMod, "go 1.27rc2") || strings.Contains(goMod, "\ntoolchain ") {
		t.Fatal("go.mod minimum/unpinned toolchain contract changed; review architecture.md")
	}
	workflow := readAuditFile(t, ".github/workflows/test.yml")
	for _, needle := range []string{"go-version: 1.27.0-rc.2", "GOTOOLCHAIN: local", "make headless-novulkan"} {
		if !strings.Contains(workflow, needle) {
			t.Fatalf("Go compatibility workflow changed; review architecture.md: %s", needle)
		}
	}
	releaseWorkflow := readAuditFile(t, ".github/workflows/release.yml")
	for _, needle := range []string{"go-version: 1.27.0-rc.2", "GOTOOLCHAIN: local"} {
		if !strings.Contains(releaseWorkflow, needle) {
			t.Fatalf("Go release workflow changed; review architecture.md: %s", needle)
		}
	}
	makefile := readAuditFile(t, "Makefile")
	if !strings.Contains(makefile, "GOEXPERIMENT=simd") {
		t.Fatal("Makefile no longer enables the documented SIMD experiment")
	}
	debBuilder := readAuditFile(t, "scripts/build-intuitionengine-deb.sh")
	debInstaller := readAuditFile(t, "scripts/install-intuitionengine-package.sh")
	repositoryBuilder := readAuditFile(t, "scripts/stage-intuitionengine-repository.sh")
	x64ImageBuilder := readAuditFile(t, "build_x64_ie_img.sh")
	rpiImageBuilder := readAuditFile(t, "scripts/build_rpi_live_image.sh")
	for path, contract := range map[string]struct {
		text    string
		needles []string
	}{
		"Makefile": {makefile, []string{
			"deb-intuitionengine-amd64-v3: check-intuitionengine-app-version x86-64-v3",
			"deb-intuitionengine-arm64-pi4: check-intuitionengine-app-version rpi-4-arm64",
			"deb-intuitionengine-arm64-pi5: check-intuitionengine-app-version rpi-5-arm64",
		}},
		"scripts/build-intuitionengine-deb.sh": {debBuilder, []string{
			`sha256sum "$root/opt/ie/IntuitionEngine"`,
			`exec /usr/lib/intuitionengine/package-check`,
			`cp -p /opt/ie/IntuitionEngine /opt/ie/IntuitionEngine.previous`,
		}},
		"scripts/install-intuitionengine-package.sh": {debInstaller, []string{
			`https://intuitionengine.io stable main`,
			`actual_checksum="$(sha256sum "$staged_binary"`,
		}},
		"scripts/stage-intuitionengine-repository.sh": {repositoryBuilder, []string{
			"dpkg-scanpackages",
			"--clearsign",
			"--detach-sign",
		}},
		"build_x64_ie_img.sh": {x64ImageBuilder, []string{
			`cmp -s "$package_root/opt/ie/IntuitionEngine" "$IE_BINARY"`,
		}},
		"scripts/build_rpi_live_image.sh": {rpiImageBuilder, []string{
			`--app-version "$app_version"`,
		}},
	} {
		for _, needle := range contract.needles {
			if !strings.Contains(contract.text, needle) {
				t.Fatalf("%s package-delivery contract changed; review architecture.md: %s", path, needle)
			}
		}
	}
	for _, needle := range []string{
		"RPI_BINARY_TAGS := $(VM_EMBED_TAGS) jack",
		"build-rpi-binary,pi4,v8.0,cortex-a72,build/rpi4-live/IntuitionEngine-rpi4,default.pgo.rpi400",
		"rpi-400-arm64: rpi-4-arm64",
		"build-rpi-binary,pi5,v8.2,cortex-a76,build/rpi5-live/IntuitionEngine-rpi5,default.pgo.rpi5",
		"--source-image build/rpi4-live/intuition-engine-rpi4.img",
		"--payload $(X64_LIVE_DIR)/work/ieshare-payload",
	} {
		if !strings.Contains(makefile, needle) {
			t.Fatalf("Raspberry Pi build-profile contract changed; review architecture.md: %s", needle)
		}
	}
	runtimeAudio := readAuditFile(t, "runtime_audio.go")
	for _, needle := range []string{
		`case "", "oto":`,
		"attempts = []int{AUDIO_BACKEND_OTO, AUDIO_BACKEND_NULL}",
		`case "jack":`,
		"attempts = []int{AUDIO_BACKEND_JACK, AUDIO_BACKEND_OTO, AUDIO_BACKEND_NULL}",
		`case "null":`,
		"attempts = []int{AUDIO_BACKEND_NULL}",
	} {
		if !strings.Contains(runtimeAudio, needle) {
			t.Fatalf("runtime audio-selection contract changed; review architecture.md: %s", needle)
		}
	}
	jackAudio := readAuditFile(t, "audio_backend_jack.go")
	for _, needle := range []string{
		"//go:build linux && cgo && jack",
		"int(client.GetSampleRate()) != jackSampleRate",
		"int(client.GetBufferSize()) != jackPeriodSize",
		"requestJACKTermination()",
	} {
		if !strings.Contains(jackAudio, needle) {
			t.Fatalf("JACK audio contract changed; review architecture.md: %s", needle)
		}
	}
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
		{sdkHexRange(IE_MIDI_LIVE_DATA, IE_MIDI_LIVE_END), "`midi_constants.go` `IE_MIDI_LIVE_*`, `midi_live.go` `MapRegisters`, `main.go` `NewLiveMIDI`"},
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
		{sdkHexRange(IE_SFX_REGION_BASE, IE_SFX_REGION_END), "`sfx_constants.go` SFX legacy constants, `main.go` `MapIO`"},
		{sdkHexRange(IE_SFX_EXT_REGION_BASE, IE_SFX_EXT_REGION_END), "`sfx_constants.go` SFX extended constants, `main.go` `MapIO`"},
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
		{sdkHexRange(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END), "`registers.go` `AROS_HOST_SOCKET_REGION_*`, `aros_host_socket_constants.go` neutral and compatibility register offsets, `host_socket_mapping.go` shared owner"},
		{sdkHexRange(CPU_WAIT_REGION_BASE, CPU_WAIT_REGION_END), "`cpu_wait_mmio.go` `RegisterCPUWaitMMIO`, SDK include `CPU_WAIT_*`, `main.go` registration"},
		{sdkHexRange(COPROC_EXT2_BASE, COPROC_EXT2_END), "`coprocessor_constants.go` `COPROC_EXT2_BASE`/`COPROC_EXT2_END`, `main.go` `MapIO`"},
		{sdkHexRange(GAMEPAD_REGION_BASE, GAMEPAD_REGION_END), "`registers.go` `GAMEPAD_REGION_BASE`/`GAMEPAD_REGION_END`, `input_gamepad.go` `RegisterGamepadMMIO`, `main.go` registration"},
		{sdkHexRange(VOODOO_BASE, VOODOO_END), "`voodoo_constants.go` `VOODOO_BASE`/`VOODOO_END`, `main.go` `MapIO`"},
		{sdkHexRange(VOODOO_FOG_TABLE_BASE, VOODOO_FOG_TABLE_END-1), "`voodoo_constants.go` fog-table constants"},
		{sdkHexRange(WORKER_IE32_BASE, WORKER_IE32_END), "`coprocessor_constants.go` IE32 worker-memory constants"},
		{sdkHexRange(WORKER_M68K_BASE, WORKER_M68K_END), "`coprocessor_constants.go` M68K worker-memory constants"},
		{sdkHexRange(0x420000, 0x49FFFF), "`coprocessor_constants.go` `workerWindow` M68K instance-1 window"},
		{sdkHexRange(0x4A0000, 0x51FFFF), "`coprocessor_constants.go` `workerWindow` x86 instance-1 window"},
		{sdkHexRange(0x520000, 0x59FFFF), "`coprocessor_constants.go` `workerWindow` IE64 instance-1 window"},
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
		{"Linux amd64 | IE32, IE64, 6502, M68K, Z80, x86", "`jit_ie32_available_linux.go`, `jit_dispatch.go`, `jit_6502_dispatch.go`, `jit_m68k_dispatch.go`, `jit_z80_dispatch.go`, and `jit_x86_dispatch.go` build tags"},
		{"Linux arm64 | IE32, IE64, M68K, 6502, Z80, x86", "`jit_ie32_available_linux.go`, `jit_dispatch.go`, `jit_m68k_dispatch_arm64.go`, `jit_6502_dispatch.go`, `jit_z80_dispatch.go`, and `jit_x86_dispatch_arm64.go`"},
		{"Windows amd64 | IE64, M68K, Z80, x86", "`jit_6502_dispatch_stub.go` interpreter fallback plus amd64 dispatch files for the listed cores"},
		{"Windows arm64 | IE64, M68K", "`jit_dispatch.go` and `jit_m68k_dispatch_arm64.go` arm64 Windows tags plus other-core stubs"},
		{"macOS amd64 | IE64, M68K, Z80, x86", "`jit_6502_dispatch_stub.go` interpreter fallback plus amd64 dispatch files for the listed cores"},
		{"macOS arm64 | IE64, M68K", "`jit_dispatch.go`, `jit_m68k_dispatch_arm64.go`, and Darwin arm64 JIT write-protect helpers"},
		{"Browser (js/wasm) | IE32, IE64, M68K, 6502, Z80 and x86 (wasm bytecode backends)", "`jit_ie32_available_wasm.go`, `jit_ie32_lower_wasm.go`, `jit_exec_wasm.go`, `jit_wasm_runtime.go`, `jit_m68k_dispatch_wasm.go`, `jit_6502_dispatch_wasm.go`, `jit_z80_dispatch_wasm.go`, and `jit_x86_dispatch_wasm.go`"},
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
		{"IE_AUDIO_BACKEND selects oto, jack, or null.", "`runtime_audio.go` `newRuntimeSoundChip` backend switch"},
		{"The default and oto paths try Oto and then silent output.", "`runtime_audio.go` `newRuntimeSoundChip` default attempt list"},
		{"The jack path tries JACK, Oto, and silent output in that order.", "`runtime_audio.go` `newRuntimeSoundChip` JACK attempt list"},
		{"The null path selects silent output directly.", "`runtime_audio.go` `newRuntimeSoundChip` null attempt list"},
		{"Linux cgo builds compiled with the jack tag provide the JACK backend.", "`audio_backend_jack.go` build constraint"},
		{"It requires the server to run at 44.1 kHz and 64-frame periods.", "`audio_backend_jack.go` constructor validation and `audio_jack_launcher.go` fixed server arguments"},
		{"After startup, a JACK shutdown, sample-rate change, or period-size change marks the backend failed and terminates through the common process cleanup path.", "`audio_backend_jack.go` callbacks and failure supervisor"},
		{"Clean shutdown, script exit, CPU-profile exit, performance-accounting exit, and terminal-signal shutdown closes the selected audio output.", "`main.go` cleanup registration, `profile_cpu.go` process cleanup hook, `runtime_audio_signal.go`, `perf_report_exit_signal.go`, and `profile_cpu_signal.go`"},
		{"This also covers an output constructed before SoundChip.Start has been called.", "`main.go` cleanup registration and `audio_chip.go` `Stop`"},
		{"Pi 4 and Pi 400 use one ARMv8.0 Cortex-A72 binary and default.pgo.rpi400 when that profile exists; otherwise PGO is disabled.", "`Makefile` Pi 4 `build-rpi-binary` call and Pi 400 compatibility alias"},
		{"Pi 5 uses ARMv8.2 Cortex-A76 settings and default.pgo.rpi5 when that profile exists; otherwise PGO is disabled.", "`Makefile` Pi 5 `build-rpi-binary` call"},
		{"Both Raspberry Pi live binaries include the jack tag.", "`Makefile` `RPI_BINARY_TAGS` and board targets"},
		{"The Raspberry Pi appliance is based on Debian 13 (Trixie), and its automatic session runs through greetd, Cage, and integrated Xwayland.", "`scripts/rpi-ie-golden.manifest`, `scripts/rpi-live/greetd-config.toml`, `scripts/rpi-live/ie-session.sh`, and `scripts/rpi-live/ie-launch.sh`"},
		{"The Pi 4 and Pi 400 image is built once with one IESHARE payload; the Pi 5 image is copied from it and receives only the Pi 5 binary before independent verification and packaging.", "`Makefile` image graph and `scripts/build_rpi_live_image.sh` source-image path"},
		{"The x64 payload check stages one canonical IESHARE tree, and both Raspberry Pi image builds consume that same tree.", "`Makefile` `x64-live-payload-check` and Raspberry Pi image recipes"},
		{"Bare .ie68 uses the active-visible RAM ceiling; EmuTOS and AROS M68K loader modes use profile bounds.", "`boot_guest_ram.go` `resolveModeCaps`/`resolveActiveVisibleCeiling` cases for `modeM68KBare`, `modeEmuTOS`, and `modeAros`"},
		{"Darwin RAM sizing uses a page-aligned conservative half of hw.memsize as the detected base before applying the per-platform reserve.", "`memory_sizing_usable_darwin.go` `unix.SysctlUint64(\"hw.memsize\")`, `pageAlignDown(total / 2)`, and `memory_sizing.go` `ReserveFor`"},
		{"ADOS_CMD_EXAMINE_ALL accelerates ACTION_EXAMINE_ALL through a 20-byte big-endian request descriptor, guest span validation, direct ExAllData packing, eac_LastKey continuation, and ERROR_ACTION_NOT_KNOWN fallback for match strings or hooks.", "`aros_dos_constants.go` `ADOS_CMD_EXAMINE_ALL`/`ADOS_EXALL_REQ_*`, `aros_dos_intercept.go` `cmdExamineAll`"},
		{"AROS HostFS fast paths use strict bulk guest-memory helpers, a 64 KiB sequential read-ahead cache, and cache invalidation on non-sequential reads, seeks outside cache, writes, truncates, close, create, delete, rename, and dirty close paths.", "`bus_helpers.go` `ValidateGuestSpan`/`WriteGuestBytes`, `aros_dos_intercept.go` `arosDOSReadAheadSize`/`readAhead`/cache invalidation"},
		{"The shared host socket block at 0xF2500-0xF257F is mapped in every recognised CPU and operating-system mode except BASIC.", "`host_socket_mapping.go` mode policy and mapping owner, `main.go` initial registration, `machine_lifecycle.go` reload registration"},
		{"The shared host socket block uses 96-byte big-endian request descriptors, strict guest-memory helper copies, 64 KiB send/receive caps, 128-byte sockaddr caps, guest-visible descriptor handles, WaitSelect fd_set translation and Release/ReleaseCopy/Obtain transfer keys.", "`aros_host_socket_constants.go` descriptor and limit constants, `aros_host_socket.go` dispatch/copy validation, and `aros_host_socket_unix.go` handle and transfer-key tables"},
		{"Byte writes are ordered from the most significant byte at offset zero to the least significant byte at offset three.", "`aros_host_socket.go` `HandleRead8`/`HandleWrite8` big-endian lane calculation and `host_socket_mapping_test.go` lane coverage"},
		{"A command assembled from byte writes dispatches only after all four command bytes have been written.", "`aros_host_socket.go` command byte-write mask and `host_socket_mapping_test.go` delayed-dispatch coverage"},
		{"Linux and Darwin provide the Unix host-socket backend.", "`aros_host_socket_unix.go` Linux/Darwin build constraint and Unix backend"},
		{"Windows and other host platforms still expose the mapped register block in non-BASIC modes, but socket commands fail closed with ENOSYS.", "`aros_host_socket_other.go` build constraint and `disabledArosHostSocketBackend` fail-closed methods"},
		{"Changing or reloading the guest mode first unmaps the old socket block, closes every active or released host descriptor exactly once, clears both guest handle tables, and then installs the replacement mapping when the new mode permits it.", "`host_socket_mapping.go` `Configure`, `aros_host_socket_unix.go` `CloseAll`, `aros_host_socket_unix_test.go`, and `host_socket_mapping_test.go`"},
		{"A hard reset restages the configured coprocessor service after coprocessor reset and before CPU restart, so the service name pointer and worker-start path remain available across reset.", "`machine_lifecycle.go` `ResetDevicesBeforeLoad`/`StartAfterReset`, `main.go` `StageConfiguredCoprocService`, and `coprocessor_cli_test.go` hard-reset restage coverage"},
		{"In flat M68K mode the DOS bridge is mapped directly, rooted at the runtime file directory, and connected to the monitor symbol table.", "`main.go` flat-M68K setup paths and `aros_dos_intercept.go` `setupDirectM68KDOS`"},
		{"Resetting or reloading flat M68K mode closes the current DOS bridge handles and locks, removes its MMIO mapping, clears runtime ownership, and installs one fresh bridge for the reloaded program.", "`machine_lifecycle.go` `CaptureCPUResetState`, `aros_audio_dma.go` `arosTeardownAll`, `aros_dos_intercept.go` `Close`/`setupDirectM68KDOS`, and `aros_reboot_lifecycle_test.go`"},
		{"Each ring has 16 descriptor slots but uses one slot to distinguish full from empty, so it can hold 15 queued requests at once.", "`coprocessor_constants.go` ring constants and coprocessor queue implementation"},
		{"M68K, x86, and IE64 support worker instances 0 and 1; IE32, 6502, and Z80 support instance 0 only.", "`coprocessor_constants.go` `coprocInstanceLimit`, `coprocessor_manager.go` instance-scoped worker table"},
		{"Instance-1 windows are M68K 0x420000-0x49FFFF, x86 0x4A0000-0x51FFFF, and IE64 0x520000-0x59FFFF, each 512 KiB; the ring index is cpuTypeIndex*2+instance, COPROC_SELECTED_STATE reports the selected worker, and COPROC_INSTANCE_STATE reports all live instances without changing the selectors.", "`coprocessor_constants.go` `workerWindow`/`coprocRingIndex`, `coprocessor_manager.go` `computeSelectedState`/`computeInstanceStateMask`"},
		{"The coprocessor discovery block at 0xF25A0-0xF25BF reports the selected type's instance limit, selected-instance state, mailbox layout version, worker window, worker ring, and an atomic all-instance liveness mask.", "`coprocessor_constants.go` `COPROC_EXT2_*`, `coprocessor_manager.go` `readReg`, `main.go` `MapIO`"},
		{"The mailbox contains twelve 0x400-byte ring slots from 0x790000 through 0x792FFF; each ring publishes layout version 1 and a worker must acknowledge that version before START succeeds.", "`coprocessor_constants.go` mailbox/ring/version constants, `coprocessor_manager.go` `initRings`/`awaitWorkerAckLocked`"},
		{"When IE_SWAP_HASH=1, each presented VOODOO_SWAP_BUFFER_CMD receives a deterministic sequence number and a 32-bit FNV-1a frame hash captured immediately after readback.", "`video_voodoo.go` swap worker hash capture and `voodoo_constants.go` swap-hash registers"},
		{"EXEC_CTRL operation values: 1=Execute, 2=EmuTOS, 3=AROS, 4=IntuitionOS IExec, 5=Hard reset", "`program_executor_constants.go` `EXEC_OP_*`, `program_executor.go` `HandleWrite` dispatch, `program_executor_test.go` operation-value pins"},
		{"mem.* helpers are raw 32-bit bus helpers, not an above-4GiB IE64 RAM or CPU-virtual-address API.", "`script_engine.go` mem helpers cast addresses to `uint32`"},
		{"Mutable devices join the snapshot contract through MachineMonitor.RegisterSnapshotDevice.", "`debug_monitor.go` `RegisterSnapshotDevice`, `main.go` registrations"},
		{"Video compositor default scale mode is stretch-fill; F11 toggles non-16:9 sources to aspect-fit.", "`video_compositor.go` `NewVideoCompositor`/`ToggleScaleModeIfNonNative`, `video_compositor_test.go` default-scale regression"},
		{"Browser Z80 emits every non-observation opcode-manifest row; port and block I/O use frozen canonical helpers with immutable decoded operands.", "`jit_z80_manifest.go` opcode inventory, `jit_z80_wasm_emit.go` emission, and `z80_frozen_fetch.go` canonical helper payload"},
		{"AY and Z80 tracker playback stops at the exclusive rendered-sample frontier until the producer extends it, preserving absolute event timing.", "`psg_engine.go` `AppendEvents`/`tickSampleLocked` and `psg_player.go` progressive render loop"},
		{"Command-line media auto-detection uses the shared media extension registry; EmuTOS .tos and .img files require an explicit EmuTOS mode.", "`runtime_helpers.go` `cliModeFromExtension`, `media_loader.go` `mediaExtensionDefinitions`, and `main.go` mode dispatch"},
		{"Guest-Advanced is the default CRT profile and flat CRT is the initial presentation mode. F7 cycles flat, curved, and off while continuing to reach the guest.", "`video_backend_ebiten.go` `NewEbitenOutput`/`Update`/`handleKeyboardInput`, `video_crt_filter_ebiten.go` mode ordering, and `video_backend_ebiten_status_test.go`"},
		{"CRT processing is a final host presentation stage. Software-composited frames are filtered after composition; hardware layers retain native source geometry while scaling and filtering, then cursor, status-bar, and host-overlay content joins the final presentation path.", "`video_backend_ebiten.go` hardware/software draw routes and `video_crt_filter_geometry_test.go`/`video_crt_filter_ebiten_test.go`"},
		{"Guest-Advanced uses preparation, luminance, Gaussian glow, bloom, and final screen passes; curved mode warps the image, glow, and bloom together in the final screen pass.", "`video_crt_guest_advanced_ebiten.go` pass graph and final shader, `video_crt_profile_ebiten_test.go`"},
		{"Video frame leases are enabled by default and can be disabled with IE_VIDEO_FRAME_LEASES=0.", "`video_compositor.go` `videoFrameLeasesEnabled`, `video_frame_lease.go` `VideoFrameLeaseRing`, and `video_compositor_test.go` lease kill-switch coverage"},
		{"Frame leases keep compositor handoff buffers stable until release; hardware layers retain leases or stage copies when leases are unavailable.", "`video_frame_lease.go` `Retain`/`Release`, `video_backend_ebiten.go` hardware layer lease retention, and `video_compositor_test.go` handoff stability coverage"},
		{"Sources that implement the compositor copy interface can copy a stable frame directly into the caller-provided lease buffer, avoiding an intermediate source snapshot before compositor handoff.", "`video_interface.go` `CompositorFrameCopySource`, `video_compositor.go` `appendCopiedCompositeLayer`, and `video_chip.go` `CopyFrameForCompositor`"},
		{"FrameGenerationSource lets the compositor skip collect/copy/blend/upload work only after source TickFrame hooks run and only when every enabled source generation is unchanged.", "`video_interface.go` `FrameGenerationSource`, `video_compositor.go` `canSkipUnchangedCompositeLocked`"},
		{"The unchanged-frame composite skip is enabled by default and can be disabled with IE_VIDEO_COMPOSITE_SKIP=0; logical frame timing still advances on skipped ticks.", "`video_compositor.go` `videoCompositeSkipEnabled`/timing callback path, `script_engine.go` `onFrameTiming`, and `video_compositor_skip_test.go`"},
		{"VIDEO_CTRL bit 1 is a presentation hold. It retains the last completed VideoChip frame while guest updates continue; clearing it presents the completed framebuffer to compositor and direct frame readers.", "`video_chip.go` `setPresentationHoldLocked`/`presentationFrameLocked`, `video_chip_regressions_test.go`"},
		{"While a VideoChip is registered with a running compositor, the compositor owns its Copper frame clock for the complete registration lifecycle. The private VideoChip refresh ticker continues its other timing work but does not start a second Copper frame.", "`video_chip.go` `acquireCompositorFrameClock`/`releaseCompositorFrameClock`/`runPrivateRefreshTick`, `video_compositor.go` registration and lifecycle ownership, and compositor Copper ownership tests"},
		{"IE32 JIT backends are available on Linux amd64, Linux arm64, and js/wasm. --nojit also selects interpreter execution for IE32 coprocessor workers created by that launch.", "`jit_ie32_available_linux.go`, `jit_ie32_available_wasm.go`, `main.go` `SetIE32JITDisabled`, `coprocessor_manager.go` `createWorker`"},
		{"The BLT_CTRL start edge samples the register shadow from the shared bus image in little-endian register order; VideoChip big-endian mode affects guest-facing register and pixel access, not the internal bus-shadow hydration order.", "`video_chip.go` `handleBlitterWriteLocked`/`hydrateBlitterStagedFromShadowLocked`/`readBlitterShadowU32Locked`, and `video_blitter_test.go` big-endian shadow hydration coverage"},
		{"VideoChip Mode7 honours the BLT_FLAGS BPP field: RGBA32 samples and writes 4-byte pixels, while CLUT8 samples and writes 1-byte palette indices with BPP-aware default strides.", "`video_chip.go` `blitMode7Locked`, `bppFromFlags`, `defaultStrideBPP`, and `video_blitter_test.go` Mode7 CLUT8 coverage"},
		{"A write that commits VOODOO_TRIANGLE_CMD binds the current Voodoo raster state to that triangle", "`video_voodoo.go` `rasterStateRegister`/`captureRasterStateLocked`/`rasterStateDirty`/`VOODOO_TEX_UPLOAD`, `voodoo_software.go` state-group flush, `voodoo_vulkan.go` software reference fallback, and `video_voodoo_state_batch_test.go` state-stamped batching coverage"},
		{"Voodoo swap jobs run asynchronously; oversized triangle batches render mid-frame without presentation or swap callbacks, and STATUS exposes busy and SWAPBUF while a presented swap is pending.", "`video_voodoo.go` `executeSwapBufferCmd`/`flushBatchLocked`/`swapWorker`/`getStatus`, `video_voodoo_batch_overflow_test.go` overflow coverage"},
		{"Voodoo texture slots retain immutable uploaded textures by slot identifier; VOODOO_TEX_BIND selects a resident texture for subsequently submitted triangles without another guest-memory transfer, and SYSINFO advertises the slot contract.", "`video_voodoo.go` `VOODOO_TEX_SLOT`/`VOODOO_TEX_UPLOAD`/`VOODOO_TEX_BIND`, `voodoo_constants.go` slot registers, and `sysinfo_mmio.go` `SYSINFO_FEATURE_VOODOO_TEX_SLOTS`"},
		{"Pooled Voodoo texture uploads are enabled by default and can be disabled with IE_VOODOO_TEXPOOL=0.", "`video_voodoo.go` pooled/fused upload path and `video_voodoo_texpool_test.go` byte-for-byte differential"},
		{"The native Vulkan backend keeps a persistent host mapping of its readback staging buffer, enabled by default and disabled with IE_VOODOO_PERSIST_MAP=0.", "`voodoo_vulkan.go` staging-map lifecycle and `voodoo_vulkan_persistmap_test.go`"},
		{"The native Vulkan backend folds the frame-final render and the framebuffer readback into one submission, enabled by default and disabled with IE_VOODOO_ONE_SUBMIT=0.", "`voodoo_vulkan.go` `FlushTrianglesForPresent`/`SwapBuffers` and `voodoo_vulkan_onesubmit_test.go` differential"},
		{"The native Vulkan backend defers the present fence wait to the next frame, enabled by default and disabled with IE_VOODOO_ASYNC_PRESENT=0.", "`voodoo_vulkan.go` deferred readback lifecycle, `video_voodoo.go` publication gate, and `voodoo_vulkan_async_test.go`/`voodoo_vulkan_finalframe_test.go`"},
		{"Retained hardware-compositor layer textures are enabled by default and can be disabled with IE_VIDEO_RETAINED_LAYERS=0.", "`video_backend_ebiten.go` retained upload/geometry cache and `video_backend_ebiten_retained_test.go`"},
		{"Browser builds use an IE64 WebAssembly bytecode JIT for supported MMU-off integer, FP32, and FP64 blocks, with interpreter fallback and IE64_WASM_JIT=0 as the runtime disable switch.", "`jit_exec_wasm.go` dispatcher gate, `jit_wasm_runtime.go` `wasmJITEnabled`, and `jit_wasm_ie64_emit.go` `wasmSupportedOpcode`"},
		{"The IE64 wasm JIT executes FP32 FMOD and transcendental operations, and FP64 DMOD and transcendental operations, through helper exits that apply the processor FPU operation and resume the compiled block.", "`jit_wasm_ie64_emit.go` helper-exit opcode cases, `jit_wasm_runtime.go` helper dispatch, and `jit_helper_dispatch.go` `HELPER_FTRANS`/`HELPER_DTRANS`"},
		{"Linux arm64 x86 executes a verified direct subset and resumes remaining forms through the interpreter.", "`jit_x86_dispatch_arm64.go` activation, `jit_x86_emit_arm64.go` direct lowering and guarded exits, and `jit_x86_exec_arm64.go` interpreter resume"},
		{"The x86 wasm JIT requires its executable coverage manifest, WebAssembly SIMD, the Go memory export, and X86_WASM_JIT not equal to 0; otherwise x86 continues in the interpreter.", "`jit_x86_dispatch_wasm.go` coverage gate and `jit_x86_exec_wasm.go` `x86WasmJITEnabled`"},
		{"The x64 live image gives Oto an unreachable PulseAudio server so it selects ALSA, and pipewire-alsa carries that stream into PipeWire.", "`build_x64_ie_img.sh` launch wrapper `PULSE_SERVER`, `pipewire-alsa` package list, and AppArmor audio/PipeWire rules; `x64_live_test.go` launcher contract coverage"},
		{"The Linux x86-64 Host SDK packages ie32asm, ie32to64, ie64asm, ie64dis, ie64-cproc, ie64ld, ie64-ar, ie64-ranlib, QBE, cproc-qbe, the IE64 runtime and libraries, public assembly includes, intuitionengine.h, and user documentation.", "`scripts/dist-host-sdk-linux-amd64.sh` staged tools, libraries, includes, and documentation; `host_sdk_test.go` public-header contract"},
		{"The x64 live image stages that archive and its SHA-256 file under SDK/Toolchains; the former per-platform SDK/Tools tree and standalone IE64 toolchain archive are not staged.", "`build_x64_ie_img.sh` `stage_share_payload`/`verify_staged_share_payload`, `x64_live_test.go` Host SDK payload assertions"},
		{"The live-image binaries are delivered as target-specific Debian packages for x64, Pi 4, and Pi 5", "`Makefile` package targets"},
		{"Each package contains its target executable, a SHA-256 manifest, and a guarded restart hook.", "`scripts/build-intuitionengine-deb.sh` package payload and maintainer scripts"},
		{"The x64 and Pi image builders verify that the package executable is identical to the binary selected for the image before staging it.", "`build_x64_ie_img.sh` package identity check and `scripts/build_rpi_live_image.sh` package-version check"},
		{"A package upgrade preserves the previous executable; checksum, version, or appliance-session validation failures restore it.", "`scripts/build-intuitionengine-deb.sh` package-check and upgrade maintainer scripts"},
		{"The public Debian repository publishes separate amd64 and arm64 indexes and signed InRelease and Release.gpg metadata.", "`scripts/stage-intuitionengine-repository.sh` architecture indexes and signatures"},
		{"Repository staging rejects a different package payload under an existing package version.", "`scripts/stage-intuitionengine-repository.sh` immutable package check and `scripts/test-intuitionengine-repository.sh`"},
		{"Appliance images use an HTTPS stable source and install the repository keyring.", "`scripts/install-intuitionengine-package.sh` source-list and keyring staging"},
		{"The package target file selects the matching architecture-specific package for repair and upgrade operations.", "`cmd/host-helper/main.go` `updateTargetPackage` and package repair path"},
		{"M68020 JIT backends are available on amd64 and arm64 Linux, Windows and macOS, plus js/wasm; the wasm backend requires __goMem and M68K_WASM_JIT=0 disables it.", "`jit_m68k_dispatch.go`, `jit_m68k_dispatch_arm64.go`, and `jit_m68k_dispatch_wasm.go` build and activation gates"},
		{"The M68020 JIT shares an untagged scanner, admission rules, CCR liveness, region formation and tier policy while keeping native and wasm lowering target-specific.", "`jit_m68k_common.go`, `jit_m68k_admission.go`, `jit_m68k_ccr_liveness.go`, `jit_m68k_region_form.go`, `jit_m68k_policy.go`, and per-target dispatch/emitter files"},
		{"Z80 VRAM banks 0x2E and 0x2F map the 16 KiB window at $8000-$BFFF onto the two halves of the VGA text aperture at 0xB8000-0xBFFFF; accesses are routed through the VGA text handler.", "`cpu_z80_runner.go` `translateVRAM`, `readNoDebug`/`Read`/`Write`, and `vga_cpu_access_test.go`"},
		{"In VGA text mode, the CRTC start address selects a character-cell display origin modulo the 16K-cell text aperture, and the cursor address uses the same cell-address space.", "`video_vga.go` `renderTextMode`/`renderScanlineText` and `vga_engine_test.go` paged-display coverage"},
		{"VGA attribute-controller mode bit 3 selects blink semantics: when set, attribute bit 7 blinks the foreground and the background index is three bits; when clear, bit 7 is the high background-colour bit.", "`video_vga.go` text renderers and `vga_engine_test.go` blink/bright-background parity coverage"},
		{"M68020 JIT memory guards use the profile-visible RAM ceiling, and native and wasm stores invalidate compiled code before stale execution.", "`jit_m68k_exec.go` profile-bound context setup and invalidation, `jit_m68k_dispatch_wasm.go` ceiling/stamp checks, and per-target store guards"},
		{"All three backends index compiled code by 4 KiB guest pages and retain one occupancy bit for each compiled guest byte.", "`cpu_m68k.go` `m68kJitCodePageMap`, `jit_m68k_exec_arm64.go`, and `jit_m68k_wasm_emit.go` `emitSMCStoreCheck`"},
		{"A write invalidates code only when its byte range overlaps compiled instruction bytes; writes to data gaps on the same page do not invalidate code.", "`jit_m68k_exec.go` exact overlap checks and per-target store guards"},
		{"Removing a block rebuilds the occupancy of every page covered by that block from the surviving cache entries.", "`jit_m68k_exec.go` and `jit_m68k_exec_arm64.go` metadata rebuild paths"},
		{"Hot M68020 native blocks can form bounded regions using constant-address propagation, constant-only folding, loop-invariant hoisting, observed-path cold exits and safe JSR leaf fusion.", "`jit_m68k_const_addr.go`, `jit_m68k_const_fold.go`, `jit_m68k_loop_analysis.go`, `jit_m68k_observed_region.go`, `jit_m68k_region_form.go`, and `m68kAnalyzeJSRLeafFusion`"},
		{"Browser FileIO and Bootstrap HostFS use in-memory volumes seeded from web assets, with file contents fetched lazily on first read.", "`file_io_select_wasm.go` asset manifest registration, `file_io_mem.go` lazy fetch path, and `hostfs_select_wasm.go`/`bootstrap_hostfs_mem.go` in-memory HostFS"},
		{"The 6502 JIT is available only on Linux amd64, Linux arm64, and js/wasm; Windows and macOS use the interpreter. Native and wasm backends invalidate cached code after physical RAM writes and resume unsupported or observed accesses through the interpreter.", "`jit_6502_dispatch.go`/`jit_6502_dispatch_stub.go`/`jit_6502_dispatch_wasm.go` build and activation gates, `machine_bus.go` generation publication, and `jit_6502_exec.go` invalidation and fallback paths"},
		{"Every available JIT backend starts enabled for directly constructed CPUs, normal runners, Program Executor launches, and JIT-capable coprocessor workers. --nojit selects interpreter execution for the primary CPU started from the command line and for IE32 coprocessor workers created by that launch.", "CPU constructors and runner defaults, `program_executor.go`, coprocessor worker constructors, and `main.go` `nojit` paths"},
		{"The browser exposes an in-tab bridge over the same FileIO memory volume.", "`file_io_select_wasm.go` `registerWasmFileBridge`, `wasm_file_bridge.go` global bridge registration"},
		{"ieImportFile adds a session-local file with a 64 MiB per-file limit, ieExportFile returns a saved file's bytes, and ieDeleteFile removes a file; none of these operations uploads data to a server.", "`wasm_file_bridge.go` `maxImportFileBytes`/`registerWasmFileBridge`, `file_io_mem.go` memory-volume operations"},
		{"ieTypeText injects representable text bytes and ieKey injects the supported editing and navigation key sequences through the normal terminal input path.", "`wasm_input_bridge.go` `registerWasmInput`, `runeToInputByte`, `translateSpecialKey`"},
		{"The resident IE64 BASIC environment implements RUN AOT, COMPILE, TRANSPILE, and ASSEMBLE with a typed linear-IR compiler resident in guest memory.", "`sdk/include/ehbasic_compiler.inc` compiler pipeline, `sdk/include/ehbasic_compiler_ir.inc` typed IR, `sdk/include/ehbasic_compiler_driver.inc` command drivers"},
		{"The replacement compiler does not delegate unsupported statements to the removed legacy transpiler.", "`ehbasic_compiler_removal_test.go` legacy artefact and marker gates, `sdk/include/ehbasic_compiler_driver.inc` replacement driver"},
		{"amd64 Linux, Windows and macOS builds, plus Linux arm64, promote hot static control-flow chains into bounded multi-block regions.", "`jit_ie64_region_policy.go` amd64 build constraint/default policy, `jit_ie64_region_policy_stub.go` Linux arm64 policy"},
		{"Each IE64 compilation unit runs its shared analyses in a fixed order before emission: FPSR condition-code liveness, loop analysis (including fixpoint hoist selection of loop-invariant chains for single-block pure integer loops), constant-only folding, then FP residency planning.", "`jit_emit_amd64.go`/`jit_emit_arm64.go`/`jit_wasm_ie64_emit.go` analysis setup order, `jit_ie64_loop_analysis.go`, `jit_ie64_const_fold.go`"},
		{"Folding treats plain STORE as constant-preserving and LOAD as invalidating only its destination; FP, control-flow and system instructions remain full barriers.", "`jit_ie64_const_fold.go` `ie64ConstFoldBarrier`/`ie64AnalyseConstFold`"},
		{"Native observed regions outline every adjacent-forward conditional cold exit after the emitted hot path.", "`jit_ie64_cold_exit.go` eligibility policy, `jit_emit_amd64.go`/`jit_emit_arm64.go` cold-exit stub emission"},
		{"The read-only gamepad block occupies 0xF25C0-0xF25FF.", "`registers.go` gamepad constants, `input_gamepad.go` `RegisterGamepadMMIO`"},
		{"Non-headless Ebiten builds poll standard-layout controllers before overlay early returns and publish one coherent snapshot per frame.", "`input_gamepad_ebiten.go` `!headless` build constraint and `Poll`, `video_backend_ebiten.go` gamepad poll placement"},
		{"This includes native desktop and js/wasm browser builds; the browser path obtains controller state through the browser Gamepad API.", "`input_gamepad_ebiten.go` `!headless` build constraint and Ebiten standard-gamepad API calls"},
		{"Headless builds publish an empty snapshot.", "`input_gamepad_stub.go` `headless` build constraint and `Poll`"},
		{"GAMEPAD_STATUS reports the connected mask in bits 3:0 and connected-pad count in bits 11:8; four 12-byte pad records contain canonical buttons and packed signed-16 left/right stick axes.", "`registers.go` gamepad layout constants, `input_gamepad.go` `readWord`/`packAxisPair`"},
		{"Byte and halfword gamepad reads use the addressed little-endian lane of the containing 32-bit word.", "`input_gamepad.go` `read`, `input_gamepad_test.go` byte-lane coverage"},
		{"Stores are accepted but ignored.", "`input_gamepad.go` `write`, `input_gamepad_test.go` write coverage"},
		{"Recording follows wall-clock time; after an encoder stall the recorder discards missed video-frame debt and matching oldest buffered audio while preserving the newest output batch.", "`video_recorder.go` `loop`/`audioPump`/`sampleRing.discard`, `video_recorder_test.go` discard and cursor-protocol coverage"},
		{"Video and audio recording pumps are independent; frozen or unchanged video is held, and audio starvation beyond 500 ms produces silence instead of stalling.", "`video_recorder.go` wall-clock `loop`, independent `audioPump`, and `recorderAudioGraceTicks`; `video_recorder_test.go` audio-starvation coverage"},
		{"CPU wait writes park until the next VBlank edge or a latched RTC_MONO_USEC deadline, capped by a 50 ms safety timeout; reads return 0.", "`cpu_wait_mmio.go` `HandleWrite`/`HandleRead` and `cpuWaitSafetyTimeout`"},
		{"IE_PERF_ACCT=1 enables per-CPU JIT/interpreter time, instruction, subsystem, and deopt counters; disabled accounting avoids atomic hot-path updates.", "`perf_accounting.go` `PerfAcct`, `perf_accounting_subsys.go` subsystem counters, and `jit_deopt_reasons.go` `DeoptStats`"},
		{"When IE_PERF_ACCT is enabled, the subsystem report is written to standard error once during clean shutdown or terminal-signal shutdown; IE_PERF_ACCT_OUT also writes it to a file.", "`perf_report_exit.go` `dumpSubsysPerfReport`, `perf_report_exit_signal.go`, `profile_cpu.go` `exitProfiled`, and `profile_cpu_signal.go`"},
		{"IE64 BASIC startup and a full reset that reloads BASIC force one Go collection after image loading and before CPU, compositor, render-loop, and audio startup.", "`boot_gc.go` `bootForcedGC`, `main.go` initial BASIC and full-reset call sites, and `boot_gc_test.go`"},
		{"The Makefile passes PGO profiles explicitly: PGO_PROFILE selects native profiles and WASM_PGO selects wasm profiles; make pgo-regenerate writes default.pgo.new.", "`Makefile` PGO variables and build recipes, `scripts/pgo-regenerate.sh`, and `build_profiles_drift_test.go`"},
		{"The source requires Go 1.27rc2. go.mod declares that minimum, and CI and release workflows pin Go 1.27rc2 with GOTOOLCHAIN=local. The default Make build enables the experimental simd/archsimd API explicitly.", "`go.mod` language directive, `.github/workflows/test.yml`, `.github/workflows/release.yml`, and `Makefile` `GOEXPERIMENT=simd` export"},
		{"Deopt reasons are unsupported, helper, mmio, smc, interrupt, cache_pressure, and debug.", "`jit_deopt_reasons.go` `deoptReasonNames`"},
		{"IE64 helper resume is enabled by default and can be disabled with IE64_JIT_RESUME=0, false, off, or no.", "`jit_helper_resume_common.go` `ie64JITResumeEnabled`, `jit_exec.go` resume loop, and `jit_helper_resume_test.go` environment coverage"},
		{"IE64 helper resume is cancelled by timer delivery, debug breakpoints, pending invalidation, PC changes, MMU mode changes, or PTBR changes.", "`jit_helper_resume_common.go` `canResumeJITHelper`, `jit_exec.go` pending-interrupt cancellation, and `jit_helper_resume_test.go` cancellation coverage"},
		{"IE64 JIT MMU helpers use a four-entry read/write micro-TLB for translated low-window RAM pages.", "`jit_common.go` `jitCtxMicroTLBEntries`, `jit_mmu_microtlb_common.go` key/fill helpers, and `jit_mmu_microtlb_test.go`"},
		{"AMD64 and ARM64 probe it for LOAD and STORE; ARM64 store hits retain physical self-modifying-code invalidation.", "`jit_emit_amd64.go` `emitMMUMicroTLBProbeAMD64`, `jit_emit_arm64.go` `emitMMUMicroTLBProbeARM64`/`emitIE64SMCStoreCheckARM64`, and `jit_store_helper_arm64_test.go`"},
		{"TLBFLUSH clears the IE64 JIT micro-TLB and TLBINVAL invalidates matching micro-TLB VPN entries.", "`mmu_ie64.go` `tlbFlush`/`tlbInvalidate`, `jit_mmu_microtlb_common.go` invalidation helpers"},
		{"IE64 self-modifying-code tracking uses 256-byte guest code pages and physical code-page tracking for MMU-compiled blocks.", "`jit_ie64_smc_range.go` guest/physical page marking and `jit_ie64_smc_range_test.go`"},
		{"x86 self-modifying-code tracking uses 256-byte code pages and range invalidation.", "`jit_x86_smc_range.go` code-page marking and range invalidation, `jit_x86_smc_range_test.go`"},
		{"ReadSamples uses safe block ticking only when every active sample ticker implements ReadSamplesBlockTicker, SFX allows block ticking, and no sample mixers are registered.", "`audio_chip.go` `ReadSamples`/`canUseReadSamplesBlockGraph`, `audio_chip_block_test.go` block-ticker fallback coverage"},
		{"The audio event ring is enabled by default and can be disabled with IE_AUDIO_EVENT_RING=0, which restores the synchronous barrier path.", "`audio_event_ring.go` `audioEventRingRequested`, `audio_chip.go` event-ring construction"},
		{"The IEScript compile cache is opt-in with IE_SCRIPT_COMPILE_CACHE=1.", "`script_engine.go` `NewScriptEngine`, `script_compile_cache.go` cache path"},
		{"It is cached by script name plus exact source text", "`script_compile_cache.go` `compileScript` cache key"},
		{"SIMD acceleration kernels are enabled by default on x64 and Linux ARM64 builds and can be disabled with IE_SIMD=0.", "`simd_gate.go` `simdRequested`/`simdKernelsActive`, host gate files, architecture dispatch files, and `simd_gate_stub.go` scalar fallback"},
		{"MachineBus 16- and 32-bit shared-RAM transfers use striped locks, so an aligned or unaligned transfer cannot be observed as a torn value.", "`machine_bus.go` `lockAtomicRAMSpan`/`atomicRAM16`/`atomicRAM32`, and `machine_bus_test.go` torn-transfer coverage"},
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
