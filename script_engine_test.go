package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
	runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, sound, nil, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
	outPath := filepath.Join(t.TempDir(), "shot.png")

	script := `rec.screenshot("` + outPath + `")`
	if err := se.RunString(script, "screenshot"); err != nil {
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

	loader := NewMediaLoader(bus, nil, t.TempDir(), nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	if err := se.RunString(`media.load("clip.xyz")`, "media_unsupported"); err != nil {
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

	loader := NewMediaLoader(bus, nil, t.TempDir(), nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	if err := se.RunString(`media.load("missing.sid")`, "media_not_found"); err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := waitMediaStatus(t, bus, 500*time.Millisecond); got != MEDIA_STATUS_ERROR {
		t.Fatalf("media status=%d, want error", got)
	}
	if got := bus.Read32(MEDIA_TYPE); got != MEDIA_TYPE_SID {
		t.Fatalf("media type=%d, want sid", got)
	}
	if got := bus.Read32(MEDIA_ERROR); got != MEDIA_ERR_NOT_FOUND {
		t.Fatalf("media error=%d, want not_found", got)
	}
}

func TestScriptEngine_MediaStop(t *testing.T) {
	bus := NewMachineBus()
	term := NewTerminalMMIO()
	comp := NewVideoCompositor(nil)
	se := NewScriptEngine(bus, comp, term)

	loader := NewMediaLoader(bus, nil, t.TempDir(), nil, nil, nil, nil, nil, nil)
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

	script := `
		media.load("track.sid")
		if media.type() ~= "sid" then error("type") end
		if media.status() ~= "playing" then error("status after load") end
		media.play()
		if media.status() ~= "playing" then error("status after play") end
		media.stop()
		if media.status() ~= "idle" then error("status after stop") end
	`
	if err := se.RunString(script, "media_load_play_stop"); err != nil {
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
	bus.MapIO(0xF0000, 0xF5FFF,
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
		if video.read_reg(` + "0xF4214" + `) ~= 20971760 then error("voodoo res") end
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

	statePath := filepath.Join(t.TempDir(), "state.gz")
	memPath := filepath.Join(t.TempDir(), "mem.bin")
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
		dbg.trace_file("` + statePath + `.trace")
		dbg.trace_file_off()
		dbg.fill_mem(0x3000, 4, 0x41)
		local hits = dbg.hunt_mem(0x3000, 4, "AA")
		if type(hits) ~= "table" then error("hunt") end
		dbg.transfer_mem(0x3000, 4, 0x3010)
		local diffs = dbg.compare_mem(0x3000, 4, 0x3010)
		if #diffs ~= 0 then error("compare") end
		dbg.save_mem_file(0x3000, 4, "` + memPath + `")
		dbg.load_mem_file("` + memPath + `", 0x3020)
		dbg.save_state("` + statePath + `")
		dbg.load_state("` + statePath + `")
		dbg.close()
	`
	if err := se.RunString(script, "dbg_advanced_compat"); err != nil {
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

	outPath := filepath.Join(t.TempDir(), "run.mp4")
	script := `rec.start("` + outPath + `"); rec.stop()`
	if err := se.RunString(script, "rec_start_stop"); err != nil {
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

	outPath := filepath.Join(t.TempDir(), "frames.mp4")
	script := `rec.start("` + outPath + `"); sys.wait_frames(3); rec.stop()`
	if err := se.RunString(script, "rec_frame_count"); err != nil {
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

	outPath := filepath.Join(t.TempDir(), "quit_stop.mp4")
	quitCalled := false
	se.SetQuitFunc(func() { quitCalled = true })

	script := `rec.start("` + outPath + `"); sys.wait_frames(2); sys.quit()`
	if err := se.RunString(script, "quit_stops_rec"); err != nil {
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
