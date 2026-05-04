package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func waitScriptStopped(t *testing.T, se *ScriptEngine) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !se.IsRunning() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("script did not stop before timeout")
}

func driveFramesUntilStopped(t *testing.T, se *ScriptEngine) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !se.IsRunning() {
			return
		}
		se.onFrameComplete()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("script did not stop before timeout")
}

func TestScriptEngine_WaitFramesAndMemoryAccess(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	err := se.RunString(`
		sys.wait_frames(2)
		cpu.freeze()
		mem.write8(0x2000, 0x2A)
		cpu.resume()
	`, "test")
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	driveFramesUntilStopped(t, se)

	if got := bus.Read8(0x2000); got != 0x2A {
		t.Fatalf("memory value=%#x, want %#x", got, 0x2A)
	}
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count=%d, want 0", got)
	}
	if err := se.LastError(); err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}
}

func TestScriptEngine_ShowreelDiagnosisScriptsParse(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	paths := []string{
		filepath.Join("sdk", "scripts", "showreel_diag.lua"),
		filepath.Join("sdk", "scripts", "diag_rotozoomer_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_robocop_intro_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_ted_121_colors_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_rotating_cube_copper_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_voodoo_triangle_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_voodoo_cube_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_voodoo_3dfx_logo_68k.ies"),
		filepath.Join("sdk", "scripts", "diag_emutos_rotozoomer_gem.ies"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) failed: %v", path, err)
		}
		if err := se.validateScript(string(data), path); err != nil {
			t.Fatalf("validateScript(%s) failed: %v", path, err)
		}
	}
}

func TestScriptEngine_MemoryRequiresFreezeForRawRAM(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	if err := se.RunString(`mem.write8(0x2000, 1)`, "test"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("expected script error for raw memory write without freeze")
	}
}

func TestScriptEngine_MemoryIOAllowedWithoutFreeze(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	var wrote uint32
	bus.MapIO(0xF2400, 0xF2400,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) { wrote = value & 0xFF },
	)

	if err := se.RunString(`mem.write8(0xF2400, 0x7F)`, "test"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("unexpected script error: %v", err)
	}
	if wrote != 0x7F {
		t.Fatalf("io write value=%#x, want %#x", wrote, 0x7F)
	}
}

func TestScriptEngine_AudioWriteReg(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip failed: %v", err)
	}
	t.Cleanup(sound.Stop)

	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, sound.HandleRegisterRead, sound.HandleRegisterWrite)
	runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, sound, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	})

	bus.Write32(FLEX_CH0_BASE+FLEX_OFF_VOL, 60)
	if sanity := bus.Read32(FLEX_CH0_BASE + FLEX_OFF_VOL); sanity == 0 {
		t.Fatalf("audio MMIO sanity write failed")
	}

	if err := se.RunString(`audio.write_reg(985732, 77)`, "audio_reg"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}

	got := bus.Read32(FLEX_CH0_BASE + FLEX_OFF_VOL)
	if got != 77 {
		t.Fatalf("FLEX_CH0_VOL=%#x, want %#x", got, uint32(77))
	}
}

func TestScriptEngine_AudioLoadFunctionsRequirePlayers(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "psg_load", script: `audio.psg_load("missing.pt3")`},
		{name: "sid_load", script: `audio.sid_load("missing.sid", 0)`},
		{name: "ted_load", script: `audio.ted_load("missing.prg")`},
		{name: "pokey_load", script: `audio.pokey_load("missing.sap")`},
		{name: "ahx_load", script: `audio.ahx_load("missing.ahx")`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			term := NewTerminalMMIO()
			comp := NewVideoCompositor(nil)
			se := NewScriptEngine(bus, comp, term)

			if err := se.RunString(tc.script, tc.name); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatalf("expected script error")
			}
		})
	}
}

func TestScriptEngine_AudioMetadataTables(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	script := `
		local p = audio.psg_metadata()
		if type(p) ~= "table" then error("psg_metadata") end
		local s = audio.sid_metadata()
		if type(s) ~= "table" then error("sid_metadata") end
	`
	if err := se.RunString(script, "audio_metadata_tables"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_CPUResetDoesNotCancelScript(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	resetCalls := 0
	se.SetHardReset(func() error {
		resetCalls++
		return nil
	})

	script := `
		cpu.reset()
		sys.print("after reset")
	`
	if err := se.RunString(script, "cpu_reset_continues"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if resetCalls != 1 {
		t.Fatalf("resetCalls=%d, want 1", resetCalls)
	}
}

func TestScriptEngine_AudioMasterNormalizerControls(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip failed: %v", err)
	}
	t.Cleanup(sound.Stop)

	runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, sound, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	})

	script := `
		audio.set_master_gain_db(-4.5)
		audio.configure_master_auto_level(-18.0, -10.0, 12.0, 250.0, 2500.0)
		audio.set_master_auto_level_enabled(true)
		audio.configure_master_compressor(-10.0, 3.0, 5.0, 120.0, 4.0, 0.0, 1.0)
		audio.set_master_compressor_enabled(true)
		audio.reset_master_dynamics()
		audio.use_showreel_normalizer_preset()
		local gain = audio.get_master_gain_db()
		if math.abs(gain + 4.5) > 0.001 then error("gain") end
	`
	if err := se.RunString(script, "audio_master_normalizer"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}

	if !sound.MasterCompressorEnabled() {
		t.Fatal("master compressor should be enabled")
	}
	if !sound.MasterAutoLevelEnabled() {
		t.Fatal("master auto level should be enabled")
	}
	if sound.masterCompThresholdDB != showreelCompThresholdDB {
		t.Fatalf("threshold=%f, want showreel preset %f", sound.masterCompThresholdDB, showreelCompThresholdDB)
	}
}

type scriptTestSource struct {
	frame   []byte
	w, h    int
	enabled bool
}

func (s *scriptTestSource) GetFrame() []byte          { return s.frame }
func (s *scriptTestSource) IsEnabled() bool           { return s.enabled }
func (s *scriptTestSource) GetLayer() int             { return 0 }
func (s *scriptTestSource) GetDimensions() (int, int) { return s.w, s.h }
func (s *scriptTestSource) SignalVSync()              {}

func TestScriptEngine_RecScreenshot(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)

	src := &scriptTestSource{
		w:       2,
		h:       2,
		enabled: true,
		frame: []byte{
			255, 0, 0, 255,
			0, 255, 0, 255,
			0, 0, 255, 255,
			255, 255, 0, 255,
		},
	}
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start failed: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	comp.Stop()

	se := NewScriptEngine(bus, comp, term)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "shot.png")

	script := `rec.screenshot("shot.png")`
	if err := se.RunString(script, filepath.Join(dir, "screenshot.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("screenshot missing: %v", err)
	}
}

func TestScriptEngine_DbgOpenClose(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	if err := se.RunString(`dbg.open(); dbg.close()`, "dbg_open_close"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if mon.IsActive() {
		t.Fatalf("monitor should be inactive after dbg.close()")
	}
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count=%d, want 0", got)
	}
}

func TestScriptEngine_DbgCommand(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	if err := se.RunString(`dbg.command("?")`, "dbg_command"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if len(mon.outputLines) == 0 {
		t.Fatalf("expected monitor output after dbg.command")
	}
}

func TestScriptEngine_DbgIsOpenStepContinue(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	script := `
		dbg.open()
		if not dbg.is_open() then error("not open") end
		dbg.step(1)
		dbg.continue()
		dbg.close()
	`
	if err := se.RunString(script, "dbg_is_open_step_continue"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if mon.IsActive() {
		t.Fatalf("monitor should be inactive after dbg.close()")
	}
}

func TestScriptEngine_CoprocWorkers(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	if err := se.RunString(`local w=coproc.workers(); if #w ~= 0 then error("expected no workers") end`, "coproc_workers"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_CoprocStartMissingFile(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	if err := se.RunString(`coproc.start("ie32", "missing.iex")`, "coproc_missing"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatalf("expected script error for missing coprocessor binary")
	}
}

func waitMediaStatus(t *testing.T, bus *MachineBus, timeout time.Duration) uint32 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := bus.Read32(MEDIA_STATUS)
		if st != MEDIA_STATUS_LOADING {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	return bus.Read32(MEDIA_STATUS)
}

func TestScriptEngine_MediaLoadUnsupported(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.xyz"), []byte("dummy"), 0644); err != nil {
		t.Fatalf("write media fixture: %v", err)
	}

	loader := NewMediaLoader(bus, nil, dir, nil, nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	if err := se.RunString(`media.load("clip.xyz")`, filepath.Join(dir, "media_unsupported.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read32(MEDIA_STATUS); got != MEDIA_STATUS_ERROR {
		t.Fatalf("media status=%d, want error", got)
	}
	if got := bus.Read32(MEDIA_TYPE); got != MEDIA_TYPE_NONE {
		t.Fatalf("media type=%d, want none", got)
	}
	if got := bus.Read32(MEDIA_ERROR); got != MEDIA_ERR_UNSUPPORTED {
		t.Fatalf("media error=%d, want unsupported", got)
	}
}

func TestScriptEngine_MediaLoadNotFound(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)
	dir := t.TempDir()

	loader := NewMediaLoader(bus, nil, t.TempDir(), nil, nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	if err := se.RunString(`media.load("missing.sid")`, filepath.Join(dir, "media_not_found.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatalf("expected script path validation error")
	}
	if got := bus.Read32(MEDIA_CTRL); got != 0 {
		t.Fatalf("media ctrl=%d, want no media loader command", got)
	}
}

func TestScriptEngine_MediaStop(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	loader := NewMediaLoader(bus, nil, t.TempDir(), nil, nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
	bus.Write32(MEDIA_NAME_PTR, 0x1000)
	bus.Write32(MEDIA_STATUS, MEDIA_STATUS_PLAYING)
	bus.Write32(MEDIA_TYPE, MEDIA_TYPE_SID)

	if err := se.RunString(`media.stop()`, "media_stop"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read32(MEDIA_STATUS); got != MEDIA_STATUS_IDLE {
		t.Fatalf("media status=%d, want idle", got)
	}
}

func TestScriptEngine_SysPrintAndLog(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = orig
		_ = r.Close()
		_ = w.Close()
	})

	if err := se.RunString(`sys.print("hello", 123); sys.log("world")`, "sys_print_log"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	_ = w.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "hello 123") {
		t.Fatalf("stdout missing print output: %q", got)
	}
	if !strings.Contains(got, "world") {
		t.Fatalf("stdout missing log output: %q", got)
	}
}

func TestScriptEngine_SysCaptureOutput(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "captured.log")
	script := `
		sys.capture_output("captured.log")
		sys.print("captured", 123)
		sys.capture_output_off()
	`
	if err := se.RunString(script, filepath.Join(dir, "sys_capture_output.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(data), "captured 123") {
		t.Fatalf("capture file missing output: %q", string(data))
	}
}

func TestScript_SysCaptureOutputRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		path   string
		target string
	}{
		{"absolute", filepath.Join(t.TempDir(), "outside.log"), ""},
		{"traversal", "../outside.log", filepath.Join(filepath.Dir(dir), "outside.log")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
			if err := se.RunString(`sys.capture_output("`+tc.path+`")`, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected capture path validation error")
			}
			if tc.target != "" {
				if _, err := os.Stat(tc.target); !os.IsNotExist(err) {
					t.Fatalf("unexpected escaped capture target exists or stat failed: %v", err)
				}
			}
		})
	}
}

func TestScript_RecOutputRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		script string
		target string
	}{
		{"screenshot_absolute", `rec.screenshot("` + filepath.Join(t.TempDir(), "shot.png") + `")`, ""},
		{"screenshot_traversal", `rec.screenshot("../shot.png")`, filepath.Join(filepath.Dir(dir), "shot.png")},
		{"start_absolute", `rec.start("` + filepath.Join(t.TempDir(), "out.mp4") + `")`, ""},
		{"start_screen_traversal", `rec.start_screen("../screen.mp4")`, filepath.Join(filepath.Dir(dir), "screen.mp4")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected recorder path validation error")
			}
			if tc.target != "" {
				if _, err := os.Stat(tc.target); !os.IsNotExist(err) {
					t.Fatalf("unexpected escaped recorder target exists or stat failed: %v", err)
				}
			}
		})
	}
}

func TestScriptEngine_SysTimeFrameCountAndFPS(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	script := `
		if sys.time_ms() <= 0 then error("time_ms") end
		if sys.fps() <= 0 then error("fps") end
		sys.wait_frames(2)
		if sys.frame_count() < 2 then error("frame_count") end
		if sys.frame_time() < 0 then error("frame_time") end
	`
	if err := se.RunString(script, "sys_metrics"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_CPUJITControls_M68K(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	runner := NewM68KRunner(NewM68KCPU(bus))
	runner.cpu.SetRunning(false)
	runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, runner, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil)
	})

	wantJIT := m68kJitAvailable
	script := `
		local before = cpu.jit_enabled()
		local initial_mode = cpu.execution_mode()
		cpu.set_jit_enabled(false)
		if cpu.jit_enabled() then error("expected jit disabled") end
		if cpu.execution_mode() ~= "interpreter" then error("expected interpreter mode") end
	`
	if wantJIT {
		script = script + `
			cpu.set_jit_enabled(true)
			if not cpu.jit_enabled() then error("expected jit enabled") end
			if cpu.execution_mode() ~= "jit" then error("expected jit mode") end
		`
	}

	if err := se.RunString(script, "cpu_jit_controls_m68k"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if runner.cpu.m68kJitEnabled != wantJIT {
		t.Fatalf("final m68k jit enabled=%v, want %v", runner.cpu.m68kJitEnabled, wantJIT)
	}
}

func TestScriptEngine_CPUSetJITEnabledWhileRunningFails(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	runner := NewM68KRunner(NewM68KCPU(bus))
	runner.cpu.SetRunning(true)
	runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, runner, nil, nil, nil)
	t.Cleanup(func() {
		runner.cpu.SetRunning(false)
		runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil)
	})

	if err := se.RunString(`cpu.set_jit_enabled(false)`, "cpu_set_jit_enabled_running"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatalf("expected script error when toggling JIT on a running CPU")
	}
}

func TestScriptEngine_Bit32AndRequirePath(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "helper.lua")
	mainPath := filepath.Join(dir, "main.ies")
	if err := os.WriteFile(helperPath, []byte(`
local M = {}
function M.mask(v)
  return bit32.band(v, 0x0F)
end
return M
`), 0644); err != nil {
		t.Fatalf("write helper.lua failed: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(`
local helper = require("helper")
cpu.freeze()
mem.write8(0x2200, helper.mask(0xF3))
cpu.resume()
`), 0644); err != nil {
		t.Fatalf("write main.ies failed: %v", err)
	}

	if err := se.RunFile(mainPath); err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read8(0x2200); got != 0x03 {
		t.Fatalf("memory value=%#x, want %#x", got, uint32(0x03))
	}
}

func TestScriptEngine_DbgOpenThenCpuFreeze(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	script := `
		dbg.open()
		cpu.freeze()
		dbg.close()
		mem.write8(0x2400, 0x5A)
		cpu.resume()
	`
	if err := se.RunString(script, "dbg_open_then_cpu_freeze"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read8(0x2400); got != 0x5A {
		t.Fatalf("memory value=%#x, want %#x", got, uint32(0x5A))
	}
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count=%d, want 0", got)
	}
}

func TestScriptEngine_CancelDuringWait(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	if err := se.RunString(`sys.wait_frames(100000)`, "cancel_during_wait"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	se.Cancel()
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
}

func TestScriptEngine_CancelAndRerun(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	if err := se.RunString(`sys.wait_frames(100000)`, "first"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := se.RunString(`
		cpu.freeze()
		mem.write8(0x2500, 0x33)
		cpu.resume()
	`, "second"); err != nil {
		t.Fatalf("second RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("second script error: %v", err)
	}
	if got := bus.Read8(0x2500); got != 0x33 {
		t.Fatalf("memory value=%#x, want %#x", got, uint32(0x33))
	}
}

func TestScriptEngine_VideoWaitPixelTimeout(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	src := &scriptTestSource{
		w:       1,
		h:       1,
		enabled: true,
		frame:   []byte{255, 0, 0, 255},
	}
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start failed: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	comp.Stop()

	se := NewScriptEngine(bus, comp, term)
	script := `
		local ok = video.wait_pixel(0, 0, 0, 255, 0, 25)
		if ok then error("expected timeout") end
	`
	if err := se.RunString(script, "video_wait_pixel_timeout"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_CoprocEnqueuePollWait(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	var pollCount int
	regs := make(map[uint32]uint32)
	bus.MapIO(COPROC_BASE, COPROC_END,
		func(addr uint32) uint32 {
			return regs[addr]
		},
		func(addr uint32, value uint32) {
			regs[addr] = value
			if addr != COPROC_CMD {
				return
			}
			switch value {
			case COPROC_CMD_ENQUEUE:
				regs[COPROC_CMD_STATUS] = COPROC_STATUS_OK
				regs[COPROC_CMD_ERROR] = COPROC_ERR_NONE
				regs[COPROC_TICKET] = 1
				regs[COPROC_WORKER_STATE] = 1 << EXEC_TYPE_IE32

				reqPtr := regs[COPROC_REQ_PTR]
				respPtr := regs[COPROC_RESP_PTR]
				respCap := regs[COPROC_RESP_CAP]
				payload := []byte("pong")
				for i := range uint32(len(payload)) {
					bus.Write8(respPtr+i, payload[i])
				}
				reqAddr := ringBaseAddr(0) + RING_ENTRIES_OFFSET
				respAddr := ringBaseAddr(0) + RING_RESPONSES_OFFSET
				bus.Write32(reqAddr+REQ_TICKET_OFF, 1)
				bus.Write32(reqAddr+REQ_RESP_PTR_OFF, respPtr)
				bus.Write32(reqAddr+REQ_RESP_CAP_OFF, respCap)
				bus.Write32(reqAddr+REQ_REQ_PTR_OFF, reqPtr)
				bus.Write32(respAddr+RESP_TICKET_OFF, 1)
				bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_RUNNING)
				bus.Write32(respAddr+RESP_RESP_LEN_OFF, uint32(len(payload)))
			case COPROC_CMD_POLL:
				regs[COPROC_CMD_STATUS] = COPROC_STATUS_OK
				regs[COPROC_CMD_ERROR] = COPROC_ERR_NONE
				pollCount++
				status := uint32(COPROC_TICKET_RUNNING)
				if pollCount > 1 {
					status = COPROC_TICKET_OK
				}
				regs[COPROC_TICKET_STATUS] = status
				respAddr := ringBaseAddr(0) + RING_RESPONSES_OFFSET
				bus.Write32(respAddr+RESP_STATUS_OFF, status)
			case COPROC_CMD_WAIT:
				regs[COPROC_CMD_STATUS] = COPROC_STATUS_OK
				regs[COPROC_CMD_ERROR] = COPROC_ERR_NONE
				regs[COPROC_TICKET_STATUS] = COPROC_TICKET_OK
				respAddr := ringBaseAddr(0) + RING_RESPONSES_OFFSET
				bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)
			}
		},
	)

	script := `
		local t = coproc.enqueue("ie32", 7, "ping")
		local p = coproc.poll(t)
		if p ~= "running" then error("poll status "..p) end
		local st, resp = coproc.wait(t, 50)
		if st ~= "ok" then error("wait status "..st) end
		if resp ~= "pong" then error("wait response "..resp) end
		local r2 = coproc.response(t)
		if r2 ~= "pong" then error("response "..r2) end
	`
	if err := se.RunString(script, "coproc_enqueue_poll_wait"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_CoprocWorkerDownStatusString(t *testing.T) {
	if got := coprocStatusToString(COPROC_TICKET_WORKER_DOWN); got != "worker_down" {
		t.Fatalf("worker-down status string = %q, want worker_down", got)
	}
}

func TestScriptEngine_MediaLoadPlayStop(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	regs := make(map[uint32]uint32)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END,
		func(addr uint32) uint32 { return regs[addr] },
		func(addr uint32, value uint32) {
			regs[addr] = value
			if addr != MEDIA_CTRL {
				return
			}
			switch value {
			case MEDIA_OP_PLAY:
				regs[MEDIA_STATUS] = MEDIA_STATUS_PLAYING
				regs[MEDIA_TYPE] = MEDIA_TYPE_SID
				regs[MEDIA_ERROR] = MEDIA_ERR_OK
			case MEDIA_OP_STOP:
				regs[MEDIA_STATUS] = MEDIA_STATUS_IDLE
				regs[MEDIA_TYPE] = MEDIA_TYPE_NONE
				regs[MEDIA_ERROR] = MEDIA_ERR_OK
			}
		},
	)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "track.sid"), []byte("dummy"), 0644); err != nil {
		t.Fatalf("write media fixture: %v", err)
	}

	script := `
		media.load("track.sid")
		if media.type() ~= "sid" then error("type") end
		if media.status() ~= "playing" then error("status after load") end
		media.play()
		if media.status() ~= "playing" then error("status after play") end
		media.stop()
		if media.status() ~= "idle" then error("status after stop") end
	`
	if err := se.RunString(script, filepath.Join(dir, "media_load_play_stop.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_VideoFrameInspection(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	src := &scriptTestSource{
		w:       2,
		h:       2,
		enabled: true,
		frame: []byte{
			255, 0, 0, 255,
			0, 255, 0, 255,
			0, 0, 255, 255,
			255, 255, 0, 255,
		},
	}
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start failed: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	comp.Stop()

	se := NewScriptEngine(bus, comp, term)
	script := `
		local r,g,b,a = video.get_pixel(0,0)
		if r ~= 255 or g ~= 0 or b ~= 0 or a ~= 255 then error("pixel") end
		local region = video.get_region(0,0,2,1)
		if #region ~= 8 then error("region") end
		local h1 = video.frame_hash()
		local h2 = video.frame_hash()
		if h1 ~= h2 then error("hash instability") end
	`
	if err := se.RunString(script, "video_inspect"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_VideoWaitHelpers(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	src := &scriptTestSource{
		w:       1,
		h:       1,
		enabled: true,
		frame:   []byte{255, 0, 0, 255},
	}
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start failed: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	comp.Stop()

	se := NewScriptEngine(bus, comp, term)
	script := `
		local ok1 = video.wait_pixel(0, 0, 255, 0, 0, 500)
		if not ok1 then error("wait_pixel") end
		local ok2 = video.wait_stable(2, 500)
		if not ok2 then error("wait_stable") end
		local ok3 = video.wait_condition(function()
			local h = video.frame_hash()
			return h ~= 0
		end, 500)
		if not ok3 then error("wait_condition") end
	`
	if err := se.RunString(script, "video_wait_helpers"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_VideoDeviceWrappers(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	regs := make(map[uint32]uint32)
	bus.MapIO(0xF0000, 0xF8FFF,
		func(addr uint32) uint32 { return regs[addr] },
		func(addr uint32, value uint32) { regs[addr] = value },
	)

	script := `
		video.vga_set_mode(0x13)
		if video.read_reg(` + "0xF1000" + `) ~= 0x13 then error("vga mode") end
		video.ula_enable(true)
		if video.read_reg(` + "0xF2004" + `) ~= 1 then error("ula enable") end
		video.antic_scroll(3,4)
		if video.read_reg(` + "0xF2110" + `) ~= 3 then error("antic hscroll") end
		if video.read_reg(` + "0xF2114" + `) ~= 4 then error("antic vscroll") end
		video.ted_mode(0x10, 0x20)
		if video.read_reg(` + "0xF0F20" + `) ~= 0x10 then error("ted mode1") end
		if video.read_reg(` + "0xF0F24" + `) ~= 0x20 then error("ted mode2") end
		video.voodoo_resolution(320, 240)
		if video.read_reg(` + "0xF8214" + `) ~= 20971760 then error("voodoo res") end
		video.copper_set_program(0x123456)
		if video.read_reg(` + "0xF0010" + `) ~= 0x123456 then error("copper ptr") end
		video.blit_line(1,2,3,4,0xAABBCCDD)
		if video.read_reg(` + "0xF0020" + `) ~= 2 then error("blit op") end
	`
	if err := se.RunString(script, "video_devices"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_DbgExtendedWrappers(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	script := `
		dbg.open()
		dbg.set_reg("PC", 0x2000)
		if dbg.get_pc() ~= 0x2000 then error("pc") end
		dbg.write_mem(0x2100, "ABC")
		if dbg.read_mem(0x2100, 3) ~= "ABC" then error("mem") end
		dbg.set_bp(0x2000)
		local bps = dbg.list_bp()
		if #bps < 1 then error("bps") end
		dbg.set_wp(0x2200)
		local wps = dbg.list_wp()
		if #wps < 1 then error("wps") end
		local regs = dbg.get_regs()
		if type(regs) ~= "table" then error("regs") end
		local cpus = dbg.cpu_list()
		if #cpus < 1 then error("cpus") end
		local devs = dbg.io_devices()
		if #devs < 1 then error("devs") end
		local io = dbg.io("video")
		if type(io) ~= "table" then error("io") end
		dbg.freeze_audio()
		dbg.thaw_audio()
		dbg.clear_all_bp()
		dbg.clear_all_wp()
		dbg.close()
	`
	if err := se.RunString(script, "dbg_extended"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestScriptEngine_DbgAdvancedCompatibility(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	se.SetMonitor(mon)

	dir := t.TempDir()
	script := `
		dbg.open()
		local d = dbg.disasm(0, 1)
		if type(d) ~= "table" then error("disasm") end
		local tr = dbg.trace(1)
		if type(tr) ~= "table" then error("trace") end
		dbg.backstep()
		dbg.trace_watch_add(0x3000)
		dbg.trace_watch_list()
		dbg.trace_history("$3000")
		dbg.trace_history_clear("$3000")
		dbg.trace_file("state.trace")
		dbg.trace_file_off()
		dbg.fill_mem(0x3000, 4, 0x41)
		local hits = dbg.hunt_mem(0x3000, 4, "AA")
		if type(hits) ~= "table" then error("hunt") end
		dbg.transfer_mem(0x3000, 4, 0x3010)
		local diffs = dbg.compare_mem(0x3000, 4, 0x3010)
		if #diffs ~= 0 then error("compare") end
		dbg.save_mem_file(0x3000, 4, "mem.bin")
		dbg.load_mem_file("mem.bin", 0x3020)
		dbg.save_state("state.gz")
		dbg.load_state("state.gz")
		dbg.close()
	`
	if err := se.RunString(script, filepath.Join(dir, "dbg_advanced_compat.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestVideoRecorder_FFmpegDetection(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	out := filepath.Join(t.TempDir(), "out.mp4")
	err := rec.Start(out)
	if _, lookErr := exec.LookPath("ffmpeg"); lookErr != nil {
		if err == nil {
			t.Fatalf("expected ffmpeg detection error")
		}
		return
	}
	if err != nil {
		t.Skipf("ffmpeg present but recorder could not start in this environment: %v", err)
	}
	if !rec.IsRecording() {
		t.Fatalf("expected recorder running state")
	}
	if stopErr := rec.Stop(); stopErr != nil {
		t.Fatalf("unexpected recorder stop error: %v", stopErr)
	}
}

func TestScriptEngine_RecStartStop(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	probe := NewVideoRecorder(comp)
	if err := probe.Start(filepath.Join(t.TempDir(), "probe.mp4")); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}
	_ = probe.Stop()

	dir := t.TempDir()
	script := `rec.start("run.mp4"); rec.stop()`
	if err := se.RunString(script, filepath.Join(dir, "rec_start_stop.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if se.recorder.IsRecording() {
		t.Fatalf("recorder should be stopped")
	}
}

func TestScriptEngine_RecFrameCount(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	probe := NewVideoRecorder(comp)
	if err := probe.Start(filepath.Join(t.TempDir(), "probe.mp4")); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}
	_ = probe.Stop()

	dir := t.TempDir()
	script := `rec.start("frames.mp4"); sys.wait_frames(3); rec.stop()`
	if err := se.RunString(script, filepath.Join(dir, "rec_frame_count.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Skipf("recorder stop reported environment-specific ffmpeg error: %v", err)
	}
	if got := se.recorder.FrameCount(); got < 2 {
		t.Fatalf("frame count=%d, want >=2", got)
	}
}

func TestVideoRecorder_ResolutionLock(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	outPath := filepath.Join(t.TempDir(), "lock.mp4")

	if err := rec.Start(outPath); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}
	if !comp.lockedResolution {
		t.Fatalf("expected locked resolution while recording")
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("recorder stop failed: %v", err)
	}
	if comp.lockedResolution {
		t.Fatalf("expected unlocked resolution after stop")
	}
}

func TestVideoRecorder_AudioSync(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	rec.sound = &SoundChip{}
	rec.ring = newSampleRing(recorderAudioRate)
	rec.width = 320
	rec.height = 200
	rec.fps = 50

	videoOut, err := os.CreateTemp(t.TempDir(), "video-*.raw")
	if err != nil {
		t.Fatalf("CreateTemp video failed: %v", err)
	}
	audioOut, err := os.CreateTemp(t.TempDir(), "audio-*.raw")
	if err != nil {
		t.Fatalf("CreateTemp audio failed: %v", err)
	}
	t.Cleanup(func() {
		_ = videoOut.Close()
		_ = audioOut.Close()
	})
	rec.videoIn = videoOut
	rec.audioW = audioOut

	for range 100 {
		rec.ring.push(0.5)
	}
	rec.writeFrame()
	if got := rec.FrameCount(); got != 0 {
		t.Fatalf("frame count with insufficient audio=%d, want 0", got)
	}

	for range 1000 {
		rec.ring.push(0.5)
	}
	rec.writeFrame()
	if got := rec.FrameCount(); got != 1 {
		t.Fatalf("frame count after sufficient audio=%d, want 1", got)
	}
}

func TestRecorder_AVSync_NoDrift(t *testing.T) {
	rec := NewVideoRecorder(nil)
	rec.sound = &SoundChip{}
	rec.ring = newSampleRing(recorderAudioRate * 20)
	rec.width = 1
	rec.height = 1
	rec.fps = 60

	videoOut, err := os.CreateTemp(t.TempDir(), "video-*.raw")
	if err != nil {
		t.Fatalf("CreateTemp video failed: %v", err)
	}
	audioOut, err := os.CreateTemp(t.TempDir(), "audio-*.raw")
	if err != nil {
		t.Fatalf("CreateTemp audio failed: %v", err)
	}
	t.Cleanup(func() {
		_ = videoOut.Close()
		_ = audioOut.Close()
	})
	rec.videoIn = videoOut
	rec.audioW = audioOut

	const frames = 1000
	expectedSamples := recorderAudioRate * frames / rec.fps
	for range expectedSamples + rec.fps {
		rec.ring.push(0.25)
	}
	pixels := make([]byte, 4)
	for range frames {
		rec.writeFrameData(pixels)
	}

	info, err := audioOut.Stat()
	if err != nil {
		t.Fatalf("audio Stat failed: %v", err)
	}
	gotSamples := int(info.Size() / 2)
	if gotSamples < expectedSamples-1 || gotSamples > expectedSamples+1 {
		t.Fatalf("audio samples after %d frames = %d, want %d +/- 1", frames, gotSamples, expectedSamples)
	}
}

func TestVideoRecorder_StartStop(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	outPath := filepath.Join(t.TempDir(), "start_stop.mp4")

	if err := rec.Start(outPath); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}
	for range 5 {
		rec.OnFrame()
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("recorder stop failed: %v", err)
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("recording file missing: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("recording file is empty")
	}
}

func TestVideoRecorder_FFmpegCrash(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	outPath := filepath.Join(t.TempDir(), "crash.mp4")

	if err := rec.Start(outPath); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}
	rec.mu.Lock()
	cmd := rec.cmd
	rec.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		t.Skip("recorder process handle unavailable")
	}
	_ = cmd.Process.Kill()
	time.Sleep(50 * time.Millisecond)
	if rec.IsRecording() {
		t.Fatalf("recorder should report stopped after ffmpeg crash")
	}
	if err := rec.Stop(); err == nil {
		t.Fatalf("expected stop to surface ffmpeg error after crash")
	}
}

func TestVideoRecorder_ModeSwitchDuringRecording(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	comp := NewVideoCompositor(nil)
	comp.SetDimensions(320, 200)
	src := &scriptTestSource{
		w:       320,
		h:       200,
		enabled: true,
		frame:   make([]byte, 320*200*4),
	}
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start failed: %v", err)
	}
	defer comp.Stop()
	time.Sleep(40 * time.Millisecond)

	rec := NewVideoRecorder(comp)
	outPath := filepath.Join(t.TempDir(), "mode_switch.mp4")
	if err := rec.Start(outPath); err != nil {
		t.Skipf("recorder start unavailable in this environment: %v", err)
	}

	// Simulate source resolution change while recording.
	src.w, src.h = 640, 480
	src.frame = make([]byte, 640*480*4)
	comp.NotifyResolutionChange(640, 480)
	time.Sleep(50 * time.Millisecond)

	// While locked, compositor keeps start dimensions.
	w, h := comp.GetDimensions()
	if w != 320 || h != 200 {
		t.Fatalf("dimensions while recording = %dx%d, want 320x200", w, h)
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("recorder stop failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	// After stop/unlock, compositor applies pending mode change.
	w, h = comp.GetDimensions()
	if w != 640 || h != 480 {
		t.Fatalf("dimensions after stop = %dx%d, want 640x480", w, h)
	}
}

func TestScriptEngine_QuitStopsRecording(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	dir := t.TempDir()
	quitCalled := false
	se.SetQuitFunc(func() { quitCalled = true })

	script := `rec.start("quit_stop.mp4"); sys.wait_frames(2); sys.quit()`
	if err := se.RunString(script, filepath.Join(dir, "quit_stops_rec.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	driveFramesUntilStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Skipf("environment-specific recorder error during quit path: %v", err)
	}
	if se.recorder.IsRecording() {
		t.Fatalf("recorder should be stopped by sys.quit")
	}
	if !quitCalled {
		t.Fatalf("quit callback should be invoked")
	}
}

func TestScript_SandboxHostAccessBlocked(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	dir := t.TempDir()
	execCanary := filepath.Join(dir, "exec_canary")
	ioCanary := filepath.Join(dir, "io_canary")
	modulePath := filepath.Join(dir, "valid_local_module.lua")
	if err := os.WriteFile(modulePath, []byte(`return { value = 42 }`), 0644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "pkg"), 0755); err != nil {
		t.Fatalf("mkdir package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "init.lua"), []byte(`return { value = 99 }`), 0644); err != nil {
		t.Fatalf("write package init: %v", err)
	}
	script := `
		local ok_exec = pcall(function() os.execute("touch ` + execCanary + `") end)
		local ok_io = pcall(function() local f = io.open("` + ioCanary + `", "w"); f:write("x"); f:close() end)
		local ok_dofile = pcall(function() dofile("x.lua") end)
		local ok_loadfile = pcall(function() loadfile("x.lua") end)
		local ok_debug = debug ~= nil
		local ok_loadlib = package.loadlib ~= nil
		local ok_bad_require = pcall(function() require("../../etc/passwd_helper") end)
		local m = require("valid_local_module")
		local p = require("pkg")
		if ok_exec or ok_io or ok_dofile or ok_loadfile or ok_debug or ok_loadlib or ok_bad_require then error("sandbox escape") end
		if m.value ~= 42 then error("local require failed") end
		if p.value ~= 99 then error("local init require failed") end
		if string.format("%02d", math.floor(7.9)) ~= "07" then error("base libs missing") end
		local t = {}; table.insert(t, bit32.band(7, 3))
		if t[1] ~= 3 then error("bit32 missing") end
		if type(os.time()) ~= "number" or type(os.date("*t")) ~= "table" or type(os.clock()) ~= "number" then error("safe os missing") end
		os.getenv("PATH")
	`
	if err := se.RunString(script, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if _, err := os.Stat(execCanary); !os.IsNotExist(err) {
		t.Fatalf("os.execute canary exists or stat failed: %v", err)
	}
	if _, err := os.Stat(ioCanary); !os.IsNotExist(err) {
		t.Fatalf("io.open canary exists or stat failed: %v", err)
	}
}

func TestScript_Cancel_TightLoop(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`while true do end`, "tight_loop"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	done := make(chan struct{})
	go func() {
		se.Cancel()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Cancel did not stop tight loop")
	}
}

func TestScript_Panic_DoesNotLeakRunning(t *testing.T) {
	se := NewScriptEngine(nil, NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`mem.read8(0)`, "panic"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if se.IsRunning() {
		t.Fatal("script still running after panic")
	}
	if err := se.LastError(); err == nil {
		t.Fatalf("LastError is nil, want recovered callback error")
	}
}

func TestScript_FreezeCleanupOnError(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`cpu.freeze(); error("x")`, "freeze_error"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count=%d, want 0", got)
	}
}

func TestScript_AudioFreezeCleanupOnError(t *testing.T) {
	bus := NewMachineBus()
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip failed: %v", err)
	}
	t.Cleanup(sound.Stop)
	runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, sound, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	})
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`audio.freeze(); error("x")`, "audio_freeze_error"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if sound.audioFrozen.Load() {
		t.Fatal("audio freeze leaked after script error")
	}
}

func TestScript_DbgLifecycleCleanupAndRefcount(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	se.SetMonitor(mon)
	if err := se.RunString(`dbg.open(); dbg.open(); dbg.close(); if not dbg.is_open() then error("closed early") end; error("x")`, "dbg_error"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if mon.IsActive() {
		t.Fatal("monitor active after script error")
	}
	if got := se.FreezeCount(); got != 0 {
		t.Fatalf("freeze count=%d, want 0", got)
	}
}

func TestScript_DbgContinueAndRunUntilDeactivateMonitor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"continue", `dbg.open(); dbg.continue()`},
		{"run_until", `dbg.open(); dbg.run_until(0x1000)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			mon := NewMachineMonitor(bus)
			mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			se.SetMonitor(mon)
			if err := se.RunString(tc.script, tc.name); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err != nil {
				t.Fatalf("script error: %v", err)
			}
			if mon.IsActive() {
				t.Fatal("monitor should be inactive after resume command")
			}
			if got := se.FreezeCount(); got != 0 {
				t.Fatalf("freeze count=%d, want 0", got)
			}
		})
	}
}

func TestMem_BlockRangesRequireFullFreezeOrFullMMIO(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"read_block", `mem.read_block(0xF2400, 5)`},
		{"write_block", `mem.write_block(0xF2400, "abcde")`},
		{"fill", `mem.fill(0xF2400, 5, 1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			bus.MapIO(0xF24FF, 0xF24FF, func(addr uint32) uint32 { return 0 }, func(addr uint32, value uint32) {})
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			script := strings.ReplaceAll(tc.script, "0xF2400", "0xF24FF")
			if err := se.RunString(script, tc.name); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected mixed MMIO/RAM range error")
			}
		})
	}
}

func TestScript_PathValidationForCpuLoad(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "prog.bin")
	if err := os.WriteFile(prog, []byte{1, 2, 3}, 0644); err != nil {
		t.Fatalf("write program: %v", err)
	}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	var loaded string
	se.SetProgramLoader(func(path string) error {
		loaded = path
		return nil
	})
	if err := se.RunString(`cpu.load("prog.bin")`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if loaded != prog {
		t.Fatalf("loaded path=%q, want %q", loaded, prog)
	}

	loaded = ""
	if err := se.RunString(`cpu.load("../prog.bin")`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("expected traversal error")
	}
	if loaded != "" {
		t.Fatalf("loader called for rejected path: %q", loaded)
	}
}

func TestScript_PathValidationForDbgWrappers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"save_state", `dbg.save_state("../state.gz")`},
		{"load_state", `dbg.load_state("../state.gz")`},
		{"save_mem_file", `dbg.save_mem_file(0x3000, 4, "../mem.bin")`},
		{"load_mem_file", `dbg.load_mem_file("../mem.bin", 0x3000)`},
		{"run_script", `dbg.run_script("../other.ies")`},
		{"trace_file", `dbg.trace_file("../trace.log")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bus := NewMachineBus()
			mon := NewMachineMonitor(bus)
			mon.RegisterCPU("ie32", NewDebugIE32(NewCPU(bus)))
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			se.SetMonitor(mon)
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected path validation error")
			}
		})
	}
}

func TestScript_DbgCommandRejectsHostFileCommands(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"save_mem", `dbg.command("save $0 $0 /tmp/out")`},
		{"load_mem", `dbg.command("load /tmp/in $1000")`},
		{"save_state", `dbg.command("ss /tmp/state.gz")`},
		{"load_state", `dbg.command("sl /tmp/state.gz")`},
		{"trace_file", `dbg.command("trace file /tmp/trace.log")`},
		{"script", `dbg.command("script /tmp/run")`},
		{"macro", `dbg.command("macro x save $0 $0 /tmp/out")`},
		{"command_output", `dbg.command_output("save $0 $0 /tmp/out")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bus := NewMachineBus()
			mon := NewMachineMonitor(bus)
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			se.SetMonitor(mon)
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected raw monitor file-command rejection")
			}
		})
	}
}

func TestScript_DbgMacroRejectsHostFileBodyAndRawInvocation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"unsafe_body", `dbg.macro("x", "save $0 $0 /tmp/out")`},
		{"raw_invoke", `dbg.macro("x", "?"); dbg.command("x")`},
		{"raw_output_invoke", `dbg.macro("x", "?"); dbg.command_output("x")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bus := NewMachineBus()
			mon := NewMachineMonitor(bus)
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			se.SetMonitor(mon)
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected macro sandbox rejection")
			}
		})
	}
}

func TestScript_DbgRunScriptRejectsHostFileCommands(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unsafe.mon"), []byte("?\nsave $0 $0 /tmp/out\n"), 0644); err != nil {
		t.Fatalf("write monitor script: %v", err)
	}
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	se.SetMonitor(mon)
	if err := se.RunString(`dbg.run_script("unsafe.mon")`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("expected monitor script sandbox rejection")
	}
}

func TestScript_DbgRunScriptRejectsExistingMacroInvocation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invoke.mon"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write monitor script: %v", err)
	}
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("macro x save $0 $0 /tmp/out")
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	se.SetMonitor(mon)
	if err := se.RunString(`dbg.run_script("invoke.mon")`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("expected monitor script macro invocation rejection")
	}
}

func TestScript_WriteValidationRejectsExistingSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.log")
	if err := os.WriteFile(outside, []byte("original"), 0644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "capture.log")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`sys.capture_output("capture.log")`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("expected symlink escape validation error")
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside target: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("outside target was modified: %q", data)
	}
}

func TestScript_AudioLoadRejectsPathEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"psg_absolute", `audio.psg_load("` + filepath.Join(t.TempDir(), "song.pt3") + `")`},
		{"sid_traversal", `audio.sid_load("../song.sid", 0)`},
		{"ted_traversal", `audio.ted_load("../song.prg")`},
		{"pokey_traversal", `audio.pokey_load("../song.sap")`},
		{"ahx_absolute", `audio.ahx_load("` + filepath.Join(t.TempDir(), "song.ahx") + `")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected audio path validation error")
			}
		})
	}
}

func TestScript_MediaLoadRejectsPathEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"load_absolute", `media.load("` + filepath.Join(t.TempDir(), "outside.wav") + `")`},
		{"load_traversal", `media.load("../outside.mod")`},
		{"load_subsong_absolute", `media.load_subsong("` + filepath.Join(t.TempDir(), "outside.sid") + `", 1)`},
		{"load_subsong_traversal", `media.load_subsong("../outside.ahx", 1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			ctrlWrites := 0
			bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END,
				func(addr uint32) uint32 { return 0 },
				func(addr uint32, value uint32) {
					if addr == MEDIA_CTRL {
						ctrlWrites++
					}
				},
			)
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			if err := se.RunString(tc.script, filepath.Join(dir, "main.ies")); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err == nil {
				t.Fatal("expected media path validation error")
			}
			if ctrlWrites != 0 {
				t.Fatalf("media control wrote %d times for rejected path", ctrlWrites)
			}
		})
	}
}

func TestDbgSaveState_PropagatesMonitorError(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	se.SetMonitor(mon)
	if err := se.RunString(`
		local ok, err = pcall(function() dbg.save_state("state.gz") end)
		if ok then error("expected monitor error") end
		if not string.find(tostring(err), "No CPU focused", 1, true) then error("wrong error: " .. tostring(err)) end
	`, filepath.Join(dir, "main.ies")); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestMonitor_ExecuteCommandResult_BadSyntax(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	_, out := mon.ExecuteCommandResult("definitely_not_a_command")
	if len(out) == 0 {
		t.Fatal("expected command output")
	}
	foundRed := false
	for _, line := range out {
		if line.Color == colorRed {
			foundRed = true
		}
	}
	if !foundRed {
		t.Fatalf("expected red output, got %#v", out)
	}
}

func TestCoproc_Tickets_NoLeak(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	ticket := uint32(0x1234)
	ringBase := ringBaseAddr(0)
	reqAddr := ringBase + RING_ENTRIES_OFFSET
	respAddr := ringBase + RING_RESPONSES_OFFSET
	bus.Write32(reqAddr+REQ_TICKET_OFF, ticket)
	bus.Write32(respAddr+RESP_TICKET_OFF, ticket)
	bus.Write32(reqAddr+REQ_RESP_PTR_OFF, 0x2000)
	bus.Write32(reqAddr+REQ_RESP_CAP_OFF, 16)
	bus.Write32(respAddr+RESP_RESP_LEN_OFF, 4)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)
	se.coprocMu.Lock()
	se.coprocTickets[ticket] = coprocTicketBuf{respPtr: 0x2000, respCap: 16}
	se.coprocMu.Unlock()
	if _, _, _, ok := se.coprocFindResponse(ticket); !ok {
		t.Fatal("expected response")
	}
	se.coprocMu.Lock()
	remaining := len(se.coprocTickets)
	se.coprocMu.Unlock()
	if remaining != 0 {
		t.Fatalf("ticket map size=%d, want 0", remaining)
	}

	se.coprocMu.Lock()
	se.coprocTickets[99] = coprocTicketBuf{}
	se.coprocMu.Unlock()
	if err := se.RunString(`sys.print("done")`, "coproc_cleanup"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	se.coprocMu.Lock()
	remaining = len(se.coprocTickets)
	se.coprocMu.Unlock()
	if remaining != 0 {
		t.Fatalf("ticket map size after script=%d, want 0", remaining)
	}
}

func TestScript_DbgCommandOutputCapturesLines(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	se.SetMonitor(mon)
	if err := se.RunString(`
		local out = dbg.command_output("definitely_not_a_command")
		if #out == 0 then error("no output") end
		if out[1].color ~= `+strconv.Itoa(int(colorRed))+` then error("not red") end
	`, "command_output"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestBit32_ExtractReplaceBtest(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`
		if bit32.extract(0xF0, 4, 4) ~= 0xF then error("extract") end
		if bit32.extract(0x10, 4) ~= 1 then error("extract default") end
		if bit32.replace(0xFFFF0000, 0x12, 8, 8) ~= 0xFFFF1200 then error("replace") end
		if not bit32.btest(0x10, 0x18) then error("btest true") end
		if bit32.btest(0x10, 0x08) then error("btest false") end
	`, "bit32"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}

func TestMediaType_MODAndWAV(t *testing.T) {
	if got := mediaTypeToString(MEDIA_TYPE_MOD); got != "mod" {
		t.Fatalf("MOD type=%q", got)
	}
	if got := mediaTypeToString(MEDIA_TYPE_WAV); got != "wav" {
		t.Fatalf("WAV type=%q", got)
	}
}

func TestScript_MediaType_MODAndWAV(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typeCode uint32
		want     string
	}{
		{"mod", MEDIA_TYPE_MOD, "mod"},
		{"wav", MEDIA_TYPE_WAV, "wav"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			regs := map[uint32]uint32{MEDIA_TYPE: tc.typeCode}
			bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END,
				func(addr uint32) uint32 { return regs[addr] },
				func(addr uint32, value uint32) { regs[addr] = value },
			)
			se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
			if err := se.RunString(`if media.type() ~= "`+tc.want+`" then error(media.type()) end`, tc.name); err != nil {
				t.Fatalf("RunString failed: %v", err)
			}
			waitScriptStopped(t, se)
			if err := se.LastError(); err != nil {
				t.Fatalf("script error: %v", err)
			}
		})
	}
}
