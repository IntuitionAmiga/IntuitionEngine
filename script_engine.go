package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ScriptEngine executes Lua automation scripts against the running emulator.
type ScriptEngine struct {
	bus           *MachineBus
	compositor    *VideoCompositor
	terminal      *TerminalMMIO
	monitor       *MachineMonitor
	recorder      *VideoRecorder
	luaOverlay    *LuaOverlay
	videoTerminal *VideoTerminal

	loadProgram    func(string) error
	hardReset      func() error
	quitFunc       func()
	exitFunc       func(int)
	setEmutosDrive func(string, uint16)
	emutosSentinel string
	arosSentinel   string

	frameChan chan struct{}

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	lastError error

	running        atomic.Bool
	loadingProgram atomic.Bool
	freezeCount    atomic.Int32
	frameCount     atomic.Uint64
	lastYieldNS    atomic.Int64

	scratchMu   sync.Mutex
	scratchNext uint32

	coprocMu      sync.Mutex
	coprocTickets map[uint32]coprocTicketBuf
}

type coprocTicketBuf struct {
	respPtr uint32
	respCap uint32
}

func NewScriptEngine(bus *MachineBus, compositor *VideoCompositor, terminal *TerminalMMIO) *ScriptEngine {
	se := &ScriptEngine{
		bus:           bus,
		compositor:    compositor,
		terminal:      terminal,
		frameChan:     make(chan struct{}, 1),
		recorder:      NewVideoRecorder(compositor),
		coprocTickets: make(map[uint32]coprocTicketBuf),
	}
	if compositor != nil {
		compositor.SetFrameCallback(se.onFrameComplete)
	}
	return se
}

func (se *ScriptEngine) SetProgramLoader(fn func(string) error) {
	se.mu.Lock()
	se.loadProgram = fn
	se.mu.Unlock()
}

func (se *ScriptEngine) SetEmutosSentinel(s string) {
	se.mu.Lock()
	se.emutosSentinel = s
	se.mu.Unlock()
}

func (se *ScriptEngine) SetArosSentinel(s string) {
	se.mu.Lock()
	se.arosSentinel = s
	se.mu.Unlock()
}

func (se *ScriptEngine) SetHardReset(fn func() error) {
	se.mu.Lock()
	se.hardReset = fn
	se.mu.Unlock()
}

func (se *ScriptEngine) SetQuitFunc(fn func()) {
	se.mu.Lock()
	se.quitFunc = fn
	se.mu.Unlock()
}

func (se *ScriptEngine) SetExitFunc(fn func(int)) {
	se.mu.Lock()
	se.exitFunc = fn
	se.mu.Unlock()
}

func (se *ScriptEngine) IsLoadingProgram() bool {
	return se.loadingProgram.Load()
}

func (se *ScriptEngine) SetLuaOverlay(o *LuaOverlay) {
	se.mu.Lock()
	se.luaOverlay = o
	se.mu.Unlock()
}

func (se *ScriptEngine) SetMonitor(mon *MachineMonitor) {
	se.mu.Lock()
	se.monitor = mon
	se.mu.Unlock()
}

func (se *ScriptEngine) SetVideoTerminal(vt *VideoTerminal) {
	se.mu.Lock()
	se.videoTerminal = vt
	se.mu.Unlock()
}

func (se *ScriptEngine) SetEmutosDriveFunc(fn func(string, uint16)) {
	se.mu.Lock()
	se.setEmutosDrive = fn
	se.mu.Unlock()
}

func (se *ScriptEngine) sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (se *ScriptEngine) IsRunning() bool {
	return se.running.Load()
}

// Done returns a channel that is closed when the current script finishes.
// Returns nil if no script is running.
func (se *ScriptEngine) Done() <-chan struct{} {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.done
}

func (se *ScriptEngine) FreezeCount() int32 {
	return se.freezeCount.Load()
}

func (se *ScriptEngine) LastError() error {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.lastError
}

func (se *ScriptEngine) RunFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	return se.runScript(string(data), absPath)
}

func (se *ScriptEngine) RunString(script string, name string) error {
	return se.runScript(script, name)
}

func (se *ScriptEngine) runScript(script string, scriptName string) error {
	if err := se.validateScript(script, scriptName); err != nil {
		return err
	}

	se.Cancel()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	se.mu.Lock()
	se.cancel = cancel
	se.done = done
	se.lastError = nil
	se.running.Store(true)
	se.mu.Unlock()

	go se.run(ctx, done, script, scriptName)
	return nil
}

func (se *ScriptEngine) Cancel() {
	se.mu.Lock()
	cancel := se.cancel
	done := se.done
	se.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if se.recorder != nil {
		_ = se.recorder.Stop()
	}
}

func (se *ScriptEngine) validateScript(script string, name string) error {
	L := lua.NewState()
	defer L.Close()
	if _, err := L.LoadString(script); err != nil {
		if name != "" {
			return fmt.Errorf("script parse failed (%s): %w", name, err)
		}
		return fmt.Errorf("script parse failed: %w", err)
	}
	return nil
}

func (se *ScriptEngine) run(ctx context.Context, done chan struct{}, script string, scriptName string) {
	defer func() {
		se.running.Store(false)
		// Release mouse override so the backend resumes hardware mouse updates.
		if se.terminal != nil {
			se.terminal.mouseOverride.Store(false)
		}
		se.mu.Lock()
		if se.done == done {
			se.done = nil
			se.cancel = nil
		}
		se.mu.Unlock()
		close(done)
	}()

	L := lua.NewState()
	defer L.Close()
	se.lastYieldNS.Store(time.Now().UnixNano())
	se.registerModules(L, ctx)
	se.configurePackagePath(L, scriptName)
	se.registerBit32(L)

	if err := L.DoString(script); err != nil {
		se.mu.Lock()
		se.lastError = err
		se.mu.Unlock()
	}
}

func (se *ScriptEngine) configurePackagePath(L *lua.LState, scriptName string) {
	if scriptName == "" {
		return
	}
	dir := filepath.Dir(scriptName)
	if dir == "" || dir == "." {
		return
	}
	packageTbl, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		return
	}
	currentPath := packageTbl.RawGetString("path").String()
	localPath := filepath.Join(dir, "?.lua")
	initPath := filepath.Join(dir, "?", "init.lua")
	packageTbl.RawSetString("path", lua.LString(currentPath+";"+localPath+";"+initPath))
}

func (se *ScriptEngine) registerBit32(L *lua.LState) {
	toU32 := func(v lua.LValue) uint32 {
		if n, ok := v.(lua.LNumber); ok {
			return uint32(int64(n))
		}
		return 0
	}
	bit32 := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"band": func(L *lua.LState) int {
			if L.GetTop() == 0 {
				L.Push(lua.LNumber(^uint32(0)))
				return 1
			}
			out := toU32(L.Get(1))
			for i := 2; i <= L.GetTop(); i++ {
				out &= toU32(L.Get(i))
			}
			L.Push(lua.LNumber(out))
			return 1
		},
		"bor": func(L *lua.LState) int {
			var out uint32
			for i := 1; i <= L.GetTop(); i++ {
				out |= toU32(L.Get(i))
			}
			L.Push(lua.LNumber(out))
			return 1
		},
		"bxor": func(L *lua.LState) int {
			var out uint32
			for i := 1; i <= L.GetTop(); i++ {
				out ^= toU32(L.Get(i))
			}
			L.Push(lua.LNumber(out))
			return 1
		},
		"bnot": func(L *lua.LState) int {
			L.Push(lua.LNumber(^toU32(L.CheckAny(1))))
			return 1
		},
		"lshift": func(L *lua.LState) int {
			x := toU32(L.CheckAny(1))
			disp := uint(L.CheckInt(2) & 31)
			L.Push(lua.LNumber(x << disp))
			return 1
		},
		"rshift": func(L *lua.LState) int {
			x := toU32(L.CheckAny(1))
			disp := uint(L.CheckInt(2) & 31)
			L.Push(lua.LNumber(x >> disp))
			return 1
		},
		"arshift": func(L *lua.LState) int {
			x := int32(toU32(L.CheckAny(1)))
			disp := uint(L.CheckInt(2) & 31)
			L.Push(lua.LNumber(uint32(x >> disp)))
			return 1
		},
		"lrotate": func(L *lua.LState) int {
			x := toU32(L.CheckAny(1))
			disp := uint(L.CheckInt(2) & 31)
			L.Push(lua.LNumber((x << disp) | (x >> ((32 - disp) & 31))))
			return 1
		},
		"rrotate": func(L *lua.LState) int {
			x := toU32(L.CheckAny(1))
			disp := uint(L.CheckInt(2) & 31)
			L.Push(lua.LNumber((x >> disp) | (x << ((32 - disp) & 31))))
			return 1
		},
	})
	L.SetGlobal("bit32", bit32)
}

func (se *ScriptEngine) onFrameComplete() {
	se.frameCount.Add(1)

	// EmuTOS on M68K relies on VBL (level 4) for periodic screen service.
	// Deliver one VBL interrupt per composed frame when execution is in ROM space.
	snap := runtimeStatus.snapshot()
	if snap.selectedCPU == runtimeCPUM68K && snap.m68k != nil {
		cpu := snap.m68k.CPU()
		if cpu != nil && cpu.Running() {
			pc := cpu.PC
			if pc >= 0x00E00000 && pc < 0x00E80000 {
				cpu.AssertInterrupt(4)
			}
		}
	}

	if se.recorder != nil {
		se.recorder.OnFrame()
	}
	select {
	case se.frameChan <- struct{}{}:
	default:
	}
}

func (se *ScriptEngine) registerModules(L *lua.LState, ctx context.Context) {
	sys := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"wait_frames":  se.luaSysWaitFrames(ctx),
		"wait_ms":      se.luaSysWaitMS(ctx),
		"print":        se.luaSysPrint(),
		"log":          se.luaSysLog(),
		"time_ms":      se.luaSysTimeMS(),
		"frame_count":  se.luaSysFrameCount(),
		"frame_time":   se.luaSysFrameTime(),
		"fps":          se.luaSysFPS(),
		"quit":         se.luaSysQuit(),
		"exit":         se.luaSysExit(),
		"emutos_drive": se.luaSysEmutosDrive(),
	})
	L.SetGlobal("sys", sys)

	cpu := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"load":       se.luaCPULoad(),
		"reset":      se.luaCPUReset(),
		"freeze":     se.luaCPUFreeze(),
		"resume":     se.luaCPUResume(),
		"start":      se.luaCPUStart(),
		"stop":       se.luaCPUStop(),
		"is_running": se.luaCPUIsRunning(),
		"mode":       se.luaCPUMode(),
	})
	L.SetGlobal("cpu", cpu)

	mem := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"read8":       se.luaMemRead8(),
		"read16":      se.luaMemRead16(),
		"read32":      se.luaMemRead32(),
		"write8":      se.luaMemWrite8(),
		"write16":     se.luaMemWrite16(),
		"write32":     se.luaMemWrite32(),
		"read_block":  se.luaMemReadBlock(),
		"write_block": se.luaMemWriteBlock(),
		"fill":        se.luaMemFill(),
	})
	L.SetGlobal("mem", mem)

	term := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"type":               se.luaTermType(),
		"type_line":          se.luaTermTypeLine(),
		"read":               se.luaTermRead(),
		"clear":              se.luaTermClear(),
		"echo":               se.luaTermEcho(),
		"wait_output":        se.luaTermWaitOutput(ctx),
		"mouse_move":         se.luaTermMouseMove(),
		"mouse_click":        se.luaTermMouseClick(ctx),
		"mouse_double_click": se.luaTermMouseDoubleClick(ctx),
		"mouse_release":      se.luaTermMouseRelease(),
		"scancode":           se.luaTermScancode(),
		"key_press":          se.luaTermKeyPress(ctx),
	})
	L.SetGlobal("term", term)

	audio := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"start":            se.luaAudioStart(),
		"stop":             se.luaAudioStop(),
		"reset":            se.luaAudioReset(),
		"freeze":           se.luaAudioFreeze(),
		"resume":           se.luaAudioResume(),
		"write_reg":        se.luaAudioWriteReg(),
		"psg_load":         se.luaAudioPSGLoad(),
		"psg_play":         se.luaAudioPSGPlay(),
		"psg_stop":         se.luaAudioPSGStop(),
		"psg_is_playing":   se.luaAudioPSGIsPlaying(),
		"psg_metadata":     se.luaAudioPSGMetadata(),
		"sid_load":         se.luaAudioSIDLoad(),
		"sid_play":         se.luaAudioSIDPlay(),
		"sid_stop":         se.luaAudioSIDStop(),
		"sid_is_playing":   se.luaAudioSIDIsPlaying(),
		"sid_metadata":     se.luaAudioSIDMetadata(),
		"ted_load":         se.luaAudioTEDLoad(),
		"ted_play":         se.luaAudioTEDPlay(),
		"ted_stop":         se.luaAudioTEDStop(),
		"ted_is_playing":   se.luaAudioTEDIsPlaying(),
		"pokey_load":       se.luaAudioPOKEYLoad(),
		"pokey_play":       se.luaAudioPOKEYPlay(),
		"pokey_stop":       se.luaAudioPOKEYStop(),
		"pokey_is_playing": se.luaAudioPOKEYIsPlaying(),
		"ahx_load":         se.luaAudioAHXLoad(),
		"ahx_play":         se.luaAudioAHXPlay(),
		"ahx_stop":         se.luaAudioAHXStop(),
		"ahx_is_playing":   se.luaAudioAHXIsPlaying(),
	})
	L.SetGlobal("audio", audio)

	video := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"write_reg":             se.luaVideoWriteReg(),
		"read_reg":              se.luaVideoReadReg(),
		"get_dimensions":        se.luaVideoGetDimensions(),
		"is_enabled":            se.luaVideoIsEnabled(),
		"vga_get_dimensions":    se.luaVGADimensions(),
		"vga_enable":            se.luaVGAEnable(),
		"vga_set_mode":          se.luaVGASetMode(),
		"vga_set_palette":       se.luaVGASetPalette(),
		"vga_get_palette":       se.luaVGAGetPalette(),
		"ula_enable":            se.luaULAEnable(),
		"ula_is_enabled":        se.luaULAIsEnabled(),
		"ula_border":            se.luaULABorder(),
		"ula_get_dimensions":    se.luaULADimensions(),
		"antic_enable":          se.luaANTICEnable(),
		"antic_is_enabled":      se.luaANTICIsEnabled(),
		"antic_dlist":           se.luaANTICDList(),
		"antic_dma":             se.luaANTICDMA(),
		"antic_scroll":          se.luaANTICScroll(),
		"antic_charset":         se.luaANTICCharset(),
		"antic_pmbase":          se.luaANTICPMBase(),
		"antic_get_dimensions":  se.luaANTICDimensions(),
		"gtia_color":            se.luaGTIAColor(),
		"gtia_player_pos":       se.luaGTIAPlayerPos(),
		"gtia_player_size":      se.luaGTIAPlayerSize(),
		"gtia_player_gfx":       se.luaGTIAPlayerGfx(),
		"gtia_priority":         se.luaGTIAPriority(),
		"ted_enable":            se.luaTEDEnable(),
		"ted_is_enabled":        se.luaTEDIsEnabled(),
		"ted_mode":              se.luaTEDMode(),
		"ted_colors":            se.luaTEDColors(),
		"ted_charset":           se.luaTEDCharset(),
		"ted_video_base":        se.luaTEDVideoBase(),
		"ted_cursor":            se.luaTEDCursor(),
		"ted_get_dimensions":    se.luaTEDDimensions(),
		"voodoo_enable":         se.luaVoodooEnable(),
		"voodoo_is_enabled":     se.luaVoodooIsEnabled(),
		"voodoo_resolution":     se.luaVoodooResolution(),
		"voodoo_vertex":         se.luaVoodooVertex(),
		"voodoo_color":          se.luaVoodooColor(),
		"voodoo_depth":          se.luaVoodooDepth(),
		"voodoo_texcoord":       se.luaVoodooTexcoord(),
		"voodoo_draw":           se.luaVoodooDraw(),
		"voodoo_swap":           se.luaVoodooSwap(),
		"voodoo_clear":          se.luaVoodooClear(),
		"voodoo_fog":            se.luaVoodooFog(),
		"voodoo_alpha":          se.luaVoodooAlpha(),
		"voodoo_zbuffer":        se.luaVoodooZBuffer(),
		"voodoo_clip":           se.luaVoodooClip(),
		"voodoo_texture":        se.luaVoodooTexture(),
		"voodoo_chromakey":      se.luaVoodooChromaKey(),
		"voodoo_dither":         se.luaVoodooDither(),
		"voodoo_get_dimensions": se.luaVoodooDimensions(),
		"copper_enable":         se.luaCopperEnable(),
		"copper_set_program":    se.luaCopperSetProgram(),
		"copper_is_running":     se.luaCopperIsRunning(),
		"blit_copy":             se.luaBlitCopy(),
		"blit_fill":             se.luaBlitFill(),
		"blit_line":             se.luaBlitLine(),
		"blit_wait":             se.luaBlitWait(ctx),
		"get_pixel":             se.luaVideoGetPixel(),
		"get_region":            se.luaVideoGetRegion(),
		"frame_hash":            se.luaVideoFrameHash(),
		"wait_pixel":            se.luaVideoWaitPixel(ctx),
		"wait_stable":           se.luaVideoWaitStable(ctx),
		"wait_condition":        se.luaVideoWaitCondition(ctx),
	})
	L.SetGlobal("video", video)

	rec := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"screenshot":   se.luaRecScreenshot(),
		"start":        se.luaRecStart(),
		"start_screen": se.luaRecStartScreen(),
		"stop":         se.luaRecStop(),
		"is_recording": se.luaRecIsRecording(),
		"frame_count":  se.luaRecFrameCount(),
	})
	L.SetGlobal("rec", rec)

	repl := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"show":        se.luaReplShow(),
		"hide":        se.luaReplHide(),
		"is_open":     se.luaReplIsOpen(),
		"print":       se.luaReplPrint(),
		"clear":       se.luaReplClear(),
		"scroll_up":   se.luaReplScrollUp(),
		"scroll_down": se.luaReplScrollDown(),
		"line_count":  se.luaReplLineCount(),
	})
	L.SetGlobal("repl", repl)

	dbg := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"open":                se.luaDbgOpen(),
		"close":               se.luaDbgClose(),
		"is_open":             se.luaDbgIsOpen(),
		"freeze":              se.luaDbgFreeze(),
		"resume":              se.luaDbgResume(),
		"step":                se.luaDbgStep(),
		"continue":            se.luaDbgContinue(),
		"run_until":           se.luaDbgRunUntil(),
		"set_bp":              se.luaDbgSetBP(),
		"set_conditional_bp":  se.luaDbgSetConditionalBP(),
		"clear_bp":            se.luaDbgClearBP(),
		"clear_all_bp":        se.luaDbgClearAllBP(),
		"list_bp":             se.luaDbgListBP(),
		"set_wp":              se.luaDbgSetWP(),
		"clear_wp":            se.luaDbgClearWP(),
		"clear_all_wp":        se.luaDbgClearAllWP(),
		"list_wp":             se.luaDbgListWP(),
		"get_reg":             se.luaDbgGetReg(),
		"set_reg":             se.luaDbgSetReg(),
		"get_regs":            se.luaDbgGetRegs(),
		"get_pc":              se.luaDbgGetPC(),
		"set_pc":              se.luaDbgSetPC(),
		"read_mem":            se.luaDbgReadMem(),
		"write_mem":           se.luaDbgWriteMem(),
		"fill_mem":            se.luaDbgFillMem(),
		"hunt_mem":            se.luaDbgHuntMem(),
		"compare_mem":         se.luaDbgCompareMem(),
		"transfer_mem":        se.luaDbgTransferMem(),
		"backtrace":           se.luaDbgBacktrace(),
		"disasm":              se.luaDbgDisasm(),
		"trace":               se.luaDbgTrace(),
		"backstep":            se.luaDbgBackstep(),
		"trace_file":          se.luaDbgTraceFile(),
		"trace_file_off":      se.luaDbgTraceFileOff(),
		"trace_watch_add":     se.luaDbgTraceWatchAdd(),
		"trace_watch_del":     se.luaDbgTraceWatchDel(),
		"trace_watch_list":    se.luaDbgTraceWatchList(),
		"trace_history":       se.luaDbgTraceHistory(),
		"trace_history_clear": se.luaDbgTraceHistoryClear(),
		"save_state":          se.luaDbgSaveState(),
		"load_state":          se.luaDbgLoadState(),
		"save_mem_file":       se.luaDbgSaveMemFile(),
		"load_mem_file":       se.luaDbgLoadMemFile(),
		"cpu_list":            se.luaDbgCPUList(),
		"cpu_focus":           se.luaDbgCPUFocus(),
		"freeze_cpu":          se.luaDbgFreezeCPU(),
		"thaw_cpu":            se.luaDbgThawCPU(),
		"freeze_all":          se.luaDbgFreezeAll(),
		"thaw_all":            se.luaDbgThawAll(),
		"freeze_audio":        se.luaDbgFreezeAudio(),
		"thaw_audio":          se.luaDbgThawAudio(),
		"io_devices":          se.luaDbgIODevices(),
		"io":                  se.luaDbgIO(),
		"run_script":          se.luaDbgRunScript(),
		"macro":               se.luaDbgMacro(),
		"command":             se.luaDbgCommand(),
	})
	L.SetGlobal("dbg", dbg)

	coproc := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"start":    se.luaCoprocStart(),
		"stop":     se.luaCoprocStop(),
		"enqueue":  se.luaCoprocEnqueue(),
		"poll":     se.luaCoprocPoll(),
		"wait":     se.luaCoprocWait(),
		"workers":  se.luaCoprocWorkers(),
		"response": se.luaCoprocResponse(),
	})
	L.SetGlobal("coproc", coproc)

	media := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"load":         se.luaMediaLoad(),
		"load_subsong": se.luaMediaLoadSubsong(),
		"play":         se.luaMediaPlay(),
		"stop":         se.luaMediaStop(),
		"status":       se.luaMediaStatus(),
		"type":         se.luaMediaType(),
		"error":        se.luaMediaError(),
	})
	L.SetGlobal("media", media)

	keys := L.NewTable()
	for _, kv := range []struct {
		name string
		code int
	}{
		{"ESCAPE", 0x01}, {"BACKSPACE", 0x0E}, {"TAB", 0x0F},
		{"ENTER", 0x1C}, {"SPACE", 0x39},
		{"LSHIFT", 0x2A}, {"RSHIFT", 0x36}, {"LCTRL", 0x1D},
		{"CAPSLOCK", 0x3A},
		{"F1", 0x3B}, {"F2", 0x3C}, {"F3", 0x3D}, {"F4", 0x3E},
		{"F5", 0x3F}, {"F6", 0x40}, {"F7", 0x41}, {"F8", 0x42},
		{"F9", 0x43}, {"F10", 0x44},
		{"UP", 0x48}, {"DOWN", 0x50}, {"LEFT", 0x4B}, {"RIGHT", 0x4D},
		{"A", 0x1E}, {"B", 0x30}, {"C", 0x2E}, {"D", 0x20},
		{"E", 0x12}, {"F", 0x21}, {"G", 0x22}, {"H", 0x23},
		{"I", 0x17}, {"J", 0x24}, {"K", 0x25}, {"L", 0x26},
		{"M", 0x32}, {"N", 0x31}, {"O", 0x18}, {"P", 0x19},
		{"Q", 0x10}, {"R", 0x13}, {"S", 0x1F}, {"T", 0x14},
		{"U", 0x16}, {"V", 0x2F}, {"W", 0x11}, {"X", 0x2D},
		{"Y", 0x15}, {"Z", 0x2C},
		{"DIGIT_1", 0x02}, {"DIGIT_2", 0x03}, {"DIGIT_3", 0x04}, {"DIGIT_4", 0x05},
		{"DIGIT_5", 0x06}, {"DIGIT_6", 0x07}, {"DIGIT_7", 0x08}, {"DIGIT_8", 0x09},
		{"DIGIT_9", 0x0A}, {"DIGIT_0", 0x0B},
		{"MINUS", 0x0C}, {"EQUAL", 0x0D},
	} {
		L.SetField(keys, kv.name, lua.LNumber(kv.code))
	}
	L.SetGlobal("keys", keys)
}

func (se *ScriptEngine) luaSysWaitFrames(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.CheckInt(1)
		if n < 0 {
			L.ArgError(1, "must be >= 0")
			return 0
		}
		for range n {
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-se.frameChan:
			}
		}
		se.lastYieldNS.Store(time.Now().UnixNano())
		return 0
	}
}

func (se *ScriptEngine) luaSysWaitMS(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		ms := L.CheckInt(1)
		if ms < 0 {
			L.ArgError(1, "must be >= 0")
			return 0
		}
		timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			L.RaiseError("script cancelled")
		case <-timer.C:
		}
		se.lastYieldNS.Store(time.Now().UnixNano())
		return 0
	}
}

func (se *ScriptEngine) luaSysPrint() lua.LGFunction {
	return func(L *lua.LState) int {
		fmt.Println(se.luaArgsToString(L))
		return 0
	}
}

func (se *ScriptEngine) luaSysLog() lua.LGFunction {
	return func(L *lua.LState) int {
		// REPL overlay logging is not wired yet; mirror to stdout for now.
		fmt.Println(se.luaArgsToString(L))
		return 0
	}
}

func (se *ScriptEngine) luaSysTimeMS() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LNumber(time.Now().UnixMilli()))
		return 1
	}
}

func (se *ScriptEngine) luaSysFrameCount() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LNumber(se.frameCount.Load()))
		return 1
	}
}

func (se *ScriptEngine) luaSysFrameTime() lua.LGFunction {
	return func(L *lua.LState) int {
		last := se.lastYieldNS.Load()
		if last <= 0 {
			L.Push(lua.LNumber(0))
			return 1
		}
		elapsed := max(time.Since(time.Unix(0, last)).Milliseconds(), 0)
		L.Push(lua.LNumber(elapsed))
		return 1
	}
}

func (se *ScriptEngine) luaSysFPS() lua.LGFunction {
	return func(L *lua.LState) int {
		fps := 0
		if se.compositor != nil {
			fps = se.compositor.GetRefreshRate()
		}
		L.Push(lua.LNumber(fps))
		return 1
	}
}

func (se *ScriptEngine) luaArgsToString(L *lua.LState) string {
	top := L.GetTop()
	if top == 0 {
		return ""
	}
	parts := make([]string, 0, top)
	for i := 1; i <= top; i++ {
		parts = append(parts, L.Get(i).String())
	}
	return strings.Join(parts, " ")
}

func (se *ScriptEngine) luaSysQuit() lua.LGFunction {
	return func(L *lua.LState) int {
		if se.recorder != nil && se.recorder.IsRecording() {
			if err := se.recorder.Stop(); err != nil {
				L.RaiseError("%v", err)
				return 0
			}
		}
		se.mu.Lock()
		quit := se.quitFunc
		se.mu.Unlock()
		if quit != nil {
			quit()
		}
		return 0
	}
}

func (se *ScriptEngine) luaSysExit() lua.LGFunction {
	return func(L *lua.LState) int {
		code := L.OptInt(1, 0)
		if se.recorder != nil && se.recorder.IsRecording() {
			_ = se.recorder.Stop()
		}
		se.mu.Lock()
		exit := se.exitFunc
		se.mu.Unlock()
		if exit != nil {
			exit(code)
		}
		return 0
	}
}

func (se *ScriptEngine) luaSysEmutosDrive() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		driveNum := uint16(L.OptInt(2, 20)) // default U: = drive 20
		se.mu.Lock()
		fn := se.setEmutosDrive
		se.mu.Unlock()
		if fn != nil {
			fn(path, driveNum)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCPULoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		se.mu.Lock()
		loader := se.loadProgram
		emutosSent := se.emutosSentinel
		arosSent := se.arosSentinel
		se.mu.Unlock()
		if loader == nil {
			L.RaiseError("program loader not configured")
			return 0
		}
		if path == "EMUTOS" && emutosSent != "" {
			path = emutosSent
		} else if path == "AROS" && arosSent != "" {
			path = arosSent
		}
		se.loadingProgram.Store(true)
		err := loader(path)
		se.loadingProgram.Store(false)
		if err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCPUReset() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		reset := se.hardReset
		se.mu.Unlock()
		if reset == nil {
			L.RaiseError("hard reset not configured")
			return 0
		}
		if err := reset(); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCPUFreeze() lua.LGFunction {
	return func(L *lua.LState) int {
		se.freezeCount.Add(1)
		return 0
	}
}

func (se *ScriptEngine) luaCPUResume() lua.LGFunction {
	return func(L *lua.LState) int {
		if se.freezeCount.Load() <= 0 {
			return 0
		}
		se.freezeCount.Add(-1)
		return 0
	}
}

func (se *ScriptEngine) luaCPUStart() lua.LGFunction {
	return func(L *lua.LState) int {
		snap := runtimeStatus.snapshot()
		switch snap.selectedCPU {
		case runtimeCPUIE32:
			if snap.ie32 != nil {
				snap.ie32.StartExecution()
			}
		case runtimeCPUIE64:
			if snap.ie64 != nil {
				snap.ie64.StartExecution()
			}
		case runtimeCPUM68K:
			if snap.m68k != nil {
				snap.m68k.StartExecution()
			}
		case runtimeCPUZ80:
			if snap.z80 != nil {
				snap.z80.StartExecution()
			}
		case runtimeCPUX86:
			if snap.x86 != nil {
				snap.x86.StartExecution()
			}
		case runtimeCPU6502:
			if snap.cpu65 != nil {
				snap.cpu65.StartExecution()
			}
		}
		return 0
	}
}

func (se *ScriptEngine) luaCPUStop() lua.LGFunction {
	return func(L *lua.LState) int {
		snap := runtimeStatus.snapshot()
		switch snap.selectedCPU {
		case runtimeCPUIE32:
			if snap.ie32 != nil {
				snap.ie32.Stop()
			}
		case runtimeCPUIE64:
			if snap.ie64 != nil {
				snap.ie64.Stop()
			}
		case runtimeCPUM68K:
			if snap.m68k != nil {
				snap.m68k.Stop()
			}
		case runtimeCPUZ80:
			if snap.z80 != nil {
				snap.z80.Stop()
			}
		case runtimeCPUX86:
			if snap.x86 != nil {
				snap.x86.Stop()
			}
		case runtimeCPU6502:
			if snap.cpu65 != nil {
				snap.cpu65.Stop()
			}
		}
		return 0
	}
}

func (se *ScriptEngine) luaCPUIsRunning() lua.LGFunction {
	return func(L *lua.LState) int {
		snap := runtimeStatus.snapshot()
		running := false
		switch snap.selectedCPU {
		case runtimeCPUIE32:
			running = snap.ie32 != nil && snap.ie32.IsRunning()
		case runtimeCPUIE64:
			running = snap.ie64 != nil && snap.ie64.IsRunning()
		case runtimeCPUM68K:
			running = snap.m68k != nil && snap.m68k.IsRunning()
		case runtimeCPUZ80:
			running = snap.z80 != nil && snap.z80.IsRunning()
		case runtimeCPUX86:
			running = snap.x86 != nil && snap.x86.IsRunning()
		case runtimeCPU6502:
			running = snap.cpu65 != nil && snap.cpu65.IsRunning()
		}
		L.Push(lua.LBool(running))
		return 1
	}
}

func (se *ScriptEngine) luaCPUMode() lua.LGFunction {
	return func(L *lua.LState) int {
		snap := runtimeStatus.snapshot()
		mode := "none"
		switch snap.selectedCPU {
		case runtimeCPUIE32:
			mode = "ie32"
		case runtimeCPUIE64:
			mode = "ie64"
		case runtimeCPUM68K:
			mode = "m68k"
		case runtimeCPUZ80:
			mode = "z80"
		case runtimeCPUX86:
			mode = "x86"
		case runtimeCPU6502:
			mode = "6502"
		}
		L.Push(lua.LString(mode))
		return 1
	}
}

func (se *ScriptEngine) requireFrozenForAddress(L *lua.LState, addr uint32) bool {
	if se.bus.IsIOAddress(addr) {
		return true
	}
	if se.freezeCount.Load() > 0 {
		return true
	}
	L.RaiseError("raw memory access requires cpu.freeze()")
	return false
}

func (se *ScriptEngine) luaMemRead8() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		L.Push(lua.LNumber(se.bus.Read8(addr)))
		return 1
	}
}

func (se *ScriptEngine) luaMemRead16() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		L.Push(lua.LNumber(se.bus.Read16(addr)))
		return 1
	}
}

func (se *ScriptEngine) luaMemRead32() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		L.Push(lua.LNumber(se.bus.Read32(addr)))
		return 1
	}
}

func (se *ScriptEngine) luaMemWrite8() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		val := uint8(L.CheckInt(2))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		se.bus.Write8(addr, val)
		return 0
	}
}

func (se *ScriptEngine) luaMemWrite16() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		val := uint16(L.CheckInt(2))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		se.bus.Write16(addr, val)
		return 0
	}
}

func (se *ScriptEngine) luaMemWrite32() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		val := uint32(L.CheckInt64(2))
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		se.bus.Write32(addr, val)
		return 0
	}
}

func (se *ScriptEngine) luaMemReadBlock() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		n := L.CheckInt(2)
		if n < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		if n == 0 {
			L.Push(lua.LString(""))
			return 1
		}
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		out := make([]byte, n)
		for i := range n {
			out[i] = se.bus.Read8(addr + uint32(i))
		}
		L.Push(lua.LString(string(out)))
		return 1
	}
}

func (se *ScriptEngine) luaMemWriteBlock() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		data := []byte(L.CheckString(2))
		if len(data) == 0 {
			return 0
		}
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		for i, b := range data {
			se.bus.Write8(addr+uint32(i), b)
		}
		return 0
	}
}

func (se *ScriptEngine) luaMemFill() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		n := L.CheckInt(2)
		val := uint8(L.CheckInt(3))
		if n < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		if n == 0 {
			return 0
		}
		if !se.requireFrozenForAddress(L, addr) {
			return 0
		}
		for i := range n {
			se.bus.Write8(addr+uint32(i), val)
		}
		return 0
	}
}

func (se *ScriptEngine) luaTermType() lua.LGFunction {
	return func(L *lua.LState) int {
		s := L.CheckString(1)
		if se.videoTerminal != nil {
			for i := 0; i < len(s); i++ {
				se.videoTerminal.HandleKeyInput(s[i])
			}
		} else {
			for i := 0; i < len(s); i++ {
				se.terminal.EnqueueByte(s[i])
			}
		}
		return 0
	}
}

func (se *ScriptEngine) luaTermTypeLine() lua.LGFunction {
	return func(L *lua.LState) int {
		s := L.CheckString(1)
		if se.videoTerminal != nil {
			for i := 0; i < len(s); i++ {
				se.videoTerminal.HandleKeyInput(s[i])
			}
			se.videoTerminal.HandleKeyInput('\n')
		} else {
			for i := 0; i < len(s); i++ {
				se.terminal.EnqueueByte(s[i])
			}
			se.terminal.EnqueueByte('\n')
		}
		return 0
	}
}

func (se *ScriptEngine) luaTermRead() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LString(se.terminal.DrainOutput()))
		return 1
	}
}

func (se *ScriptEngine) luaTermClear() lua.LGFunction {
	return func(L *lua.LState) int {
		_ = se.terminal.DrainOutput()
		return 0
	}
}

func (se *ScriptEngine) luaTermEcho() lua.LGFunction {
	return func(L *lua.LState) int {
		on := L.CheckBool(1)
		if on {
			se.terminal.HandleWrite(TERM_ECHO, 1)
		} else {
			se.terminal.HandleWrite(TERM_ECHO, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaTermWaitOutput(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		pattern := L.CheckString(1)
		timeoutMS := L.CheckInt(2)
		if timeoutMS < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}

		var builder strings.Builder
		builder.WriteString(se.terminal.DrainOutput())
		if strings.Contains(builder.String(), pattern) {
			L.Push(lua.LBool(true))
			return 1
		}

		deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-ticker.C:
				builder.WriteString(se.terminal.DrainOutput())
				if strings.Contains(builder.String(), pattern) {
					L.Push(lua.LBool(true))
					return 1
				}
				if time.Now().After(deadline) {
					L.Push(lua.LBool(false))
					return 1
				}
			}
		}
	}
}

func (se *ScriptEngine) clampMouse(x, y int) (int32, int32) {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if se.compositor != nil {
		w, h := se.compositor.GetNativeSourceDimensions()
		if w > 0 && x >= w {
			x = w - 1
		}
		if h > 0 && y >= h {
			y = h - 1
		}
	}
	return int32(x), int32(y)
}

func validateMouseButton(L *lua.LState, argN int, val int) uint32 {
	if val < 1 || val > 3 {
		L.ArgError(argN, "button must be 1 (left), 2 (right), or 3 (both)")
		return 0
	}
	return uint32(val)
}

func (se *ScriptEngine) luaTermMouseMove() lua.LGFunction {
	return func(L *lua.LState) int {
		cx, cy := se.clampMouse(L.CheckInt(1), L.CheckInt(2))
		se.terminal.mouseOverride.Store(true)
		se.terminal.mouseX.Store(cx)
		se.terminal.mouseY.Store(cy)
		se.terminal.mouseChanged.Store(true)
		return 0
	}
}

func (se *ScriptEngine) luaTermMouseClick(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		cx, cy := se.clampMouse(L.CheckInt(1), L.CheckInt(2))
		btn := validateMouseButton(L, 3, L.OptInt(3, 1))
		se.terminal.mouseOverride.Store(true)
		se.terminal.mouseX.Store(cx)
		se.terminal.mouseY.Store(cy)
		se.terminal.mouseChanged.Store(true)
		se.sleepCtx(ctx, 50*time.Millisecond)
		se.terminal.mouseButtons.Store(btn)
		se.terminal.mouseChanged.Store(true)
		se.sleepCtx(ctx, 60*time.Millisecond)
		se.terminal.mouseButtons.Store(0)
		se.terminal.mouseChanged.Store(true)
		se.sleepCtx(ctx, 50*time.Millisecond)
		return 0
	}
}

func (se *ScriptEngine) luaTermMouseDoubleClick(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		cx, cy := se.clampMouse(L.CheckInt(1), L.CheckInt(2))
		btn := validateMouseButton(L, 3, L.OptInt(3, 1))
		se.terminal.mouseOverride.Store(true)
		// Move to position and let VBL register it
		se.terminal.mouseX.Store(cx)
		se.terminal.mouseY.Store(cy)
		se.terminal.mouseChanged.Store(true)
		se.sleepCtx(ctx, 50*time.Millisecond)
		// Two quick clicks — short hold and gap for fast double-click
		for range 2 {
			se.terminal.mouseButtons.Store(btn)
			se.terminal.mouseChanged.Store(true)
			se.sleepCtx(ctx, 60*time.Millisecond)
			se.terminal.mouseButtons.Store(0)
			se.terminal.mouseChanged.Store(true)
			se.sleepCtx(ctx, 80*time.Millisecond)
		}
		return 0
	}
}

func (se *ScriptEngine) luaTermMouseRelease() lua.LGFunction {
	return func(L *lua.LState) int {
		se.terminal.mouseOverride.Store(false)
		return 0
	}
}

func (se *ScriptEngine) luaTermScancode() lua.LGFunction {
	return func(L *lua.LState) int {
		code := L.CheckInt(1)
		if code < 0 || code > 255 {
			L.ArgError(1, "scancode must be 0..255")
			return 0
		}
		se.terminal.EnqueueScancode(uint8(code))
		return 0
	}
}

func (se *ScriptEngine) luaTermKeyPress(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		code := L.CheckInt(1)
		if code < 0 || code > 127 {
			L.ArgError(1, "scancode must be 0..127 (make code)")
			return 0
		}
		hold := L.OptInt(2, 50)
		if hold < 0 {
			L.ArgError(2, "hold_ms must be >= 0")
			return 0
		}
		se.terminal.EnqueueScancode(uint8(code))
		se.sleepCtx(ctx, time.Duration(hold)*time.Millisecond)
		se.terminal.EnqueueScancode(uint8(code) | 0x80)
		return 0
	}
}

func (se *ScriptEngine) luaAudioStart() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.Start()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.Stop()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioReset() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.Reset()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioFreeze() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.audioFrozen.Store(true)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioResume() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.audioFrozen.Store(false)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioWriteReg() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt64(1))
		val := uint32(L.CheckInt64(2))
		se.bus.Write32(addr, val)
		return 0
	}
}

func (se *ScriptEngine) luaAudioPSGLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		p := runtimeStatus.snapshot().psgPlayer
		if p == nil {
			L.RaiseError("psg player unavailable")
			return 0
		}
		if err := p.Load(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioPSGPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().psgPlayer; p != nil {
			p.Play()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioPSGStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().psgPlayer; p != nil {
			p.Stop()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioPSGIsPlaying() lua.LGFunction {
	return func(L *lua.LState) int {
		playing := false
		if p := runtimeStatus.snapshot().psgEngine; p != nil {
			playing = p.IsPlaying()
		}
		L.Push(lua.LBool(playing))
		return 1
	}
}

func (se *ScriptEngine) luaAudioPSGMetadata() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		if p := runtimeStatus.snapshot().psgPlayer; p != nil {
			meta := p.Metadata()
			t.RawSetString("title", lua.LString(meta.Title))
			t.RawSetString("author", lua.LString(meta.Author))
			t.RawSetString("system", lua.LString(meta.System))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaAudioSIDLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		subsong := L.OptInt(2, 0)
		p := runtimeStatus.snapshot().sidPlayer
		if p == nil {
			L.RaiseError("sid player unavailable")
			return 0
		}
		if err := p.LoadWithOptions(path, subsong, false, false); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioSIDPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().sidPlayer; p != nil {
			p.Play()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioSIDStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().sidPlayer; p != nil {
			p.Stop()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioSIDIsPlaying() lua.LGFunction {
	return func(L *lua.LState) int {
		playing := false
		if p := runtimeStatus.snapshot().sidPlayer; p != nil {
			playing = p.IsPlaying()
		}
		L.Push(lua.LBool(playing))
		return 1
	}
}

func (se *ScriptEngine) luaAudioSIDMetadata() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		if p := runtimeStatus.snapshot().sidPlayer; p != nil {
			meta := p.Metadata()
			t.RawSetString("title", lua.LString(meta.Title))
			t.RawSetString("author", lua.LString(meta.Author))
			t.RawSetString("released", lua.LString(meta.Released))
			t.RawSetString("duration", lua.LString(p.DurationText()))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaAudioTEDLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		p := runtimeStatus.snapshot().tedPlayer
		if p == nil {
			L.RaiseError("ted player unavailable")
			return 0
		}
		if err := p.Load(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioTEDPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().tedPlayer; p != nil {
			p.Play()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioTEDStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().tedPlayer; p != nil {
			p.Stop()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioTEDIsPlaying() lua.LGFunction {
	return func(L *lua.LState) int {
		playing := false
		if p := runtimeStatus.snapshot().tedPlayer; p != nil {
			playing = p.IsPlaying()
		}
		L.Push(lua.LBool(playing))
		return 1
	}
}

func (se *ScriptEngine) luaAudioPOKEYPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().pokeyPlayer; p != nil {
			p.Play()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioPOKEYStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if p := runtimeStatus.snapshot().pokeyPlayer; p != nil {
			p.Stop()
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioPOKEYIsPlaying() lua.LGFunction {
	return func(L *lua.LState) int {
		playing := false
		if p := runtimeStatus.snapshot().pokeyPlayer; p != nil {
			playing = p.IsPlaying()
		}
		L.Push(lua.LBool(playing))
		return 1
	}
}

func (se *ScriptEngine) luaAudioPOKEYLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		p := runtimeStatus.snapshot().pokeyPlayer
		if p == nil {
			L.RaiseError("pokey player unavailable")
			return 0
		}
		if err := p.Load(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioAHXLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		a := runtimeStatus.snapshot().ahxEngine
		if a == nil {
			L.RaiseError("ahx engine unavailable")
			return 0
		}
		data, err := os.ReadFile(path)
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		if err := a.LoadData(data); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioAHXPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		if a := runtimeStatus.snapshot().ahxEngine; a != nil {
			a.SetPlaying(true)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioAHXStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if a := runtimeStatus.snapshot().ahxEngine; a != nil {
			a.SetPlaying(false)
		}
		return 0
	}
}

func (se *ScriptEngine) luaAudioAHXIsPlaying() lua.LGFunction {
	return func(L *lua.LState) int {
		playing := false
		if a := runtimeStatus.snapshot().ahxEngine; a != nil {
			playing = a.IsPlaying()
		}
		L.Push(lua.LBool(playing))
		return 1
	}
}

func (se *ScriptEngine) luaVideoWriteReg() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt64(1))
		val := uint32(L.CheckInt64(2))
		se.bus.Write32(addr, val)
		return 0
	}
}

func (se *ScriptEngine) luaVideoReadReg() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt64(1))
		L.Push(lua.LNumber(se.bus.Read32(addr)))
		return 1
	}
}

func (se *ScriptEngine) luaVideoGetDimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		if se.compositor == nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 2
		}
		w, h := se.compositor.GetDimensions()
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaVideoIsEnabled() lua.LGFunction {
	return func(L *lua.LState) int {
		enabled := false
		if v := runtimeStatus.snapshot().video; v != nil {
			enabled = v.IsEnabled()
		}
		L.Push(lua.LBool(enabled))
		return 1
	}
}

func (se *ScriptEngine) luaVGADimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		w, h := 0, 0
		if v := runtimeStatus.snapshot().vga; v != nil {
			w, h = v.GetDimensions()
		}
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaVGAEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		on := L.CheckBool(1)
		if on {
			se.bus.Write32(VGA_CTRL, VGA_CTRL_ENABLE)
		} else {
			se.bus.Write32(VGA_CTRL, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaVGASetMode() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(VGA_MODE, uint32(L.CheckInt(1)))
		return 0
	}
}

func (se *ScriptEngine) luaVGASetPalette() lua.LGFunction {
	return func(L *lua.LState) int {
		idx := uint32(L.CheckInt(1) & 0xFF)
		r := uint32(L.CheckInt(2) & 0xFF)
		g := uint32(L.CheckInt(3) & 0xFF)
		b := uint32(L.CheckInt(4) & 0xFF)
		base := VGA_PALETTE + idx*4
		se.bus.Write32(base, r|(g<<8)|(b<<16))
		return 0
	}
}

func (se *ScriptEngine) luaVGAGetPalette() lua.LGFunction {
	return func(L *lua.LState) int {
		idx := uint32(L.CheckInt(1) & 0xFF)
		val := se.bus.Read32(VGA_PALETTE + idx*4)
		L.Push(lua.LNumber(val & 0xFF))
		L.Push(lua.LNumber((val >> 8) & 0xFF))
		L.Push(lua.LNumber((val >> 16) & 0xFF))
		return 3
	}
}

func (se *ScriptEngine) luaULAEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		if L.CheckBool(1) {
			se.bus.Write32(ULA_CTRL, ULA_CTRL_ENABLE)
		} else {
			se.bus.Write32(ULA_CTRL, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaULAIsEnabled() lua.LGFunction {
	return func(L *lua.LState) int {
		v := runtimeStatus.snapshot().ula
		L.Push(lua.LBool(v != nil && v.IsEnabled()))
		return 1
	}
}

func (se *ScriptEngine) luaULABorder() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(ULA_BORDER, uint32(L.CheckInt(1)&0x07))
		return 0
	}
}

func (se *ScriptEngine) luaULADimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		w, h := 0, 0
		if u := runtimeStatus.snapshot().ula; u != nil {
			w, h = u.GetDimensions()
		}
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaANTICEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		if L.CheckBool(1) {
			se.bus.Write32(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)
		} else {
			se.bus.Write32(ANTIC_ENABLE, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaANTICIsEnabled() lua.LGFunction {
	return func(L *lua.LState) int {
		a := runtimeStatus.snapshot().antic
		L.Push(lua.LBool(a != nil && a.IsEnabled()))
		return 1
	}
}

func (se *ScriptEngine) luaANTICDList() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint32(L.CheckInt(1))
		se.bus.Write32(ANTIC_DLISTL, addr&0xFF)
		se.bus.Write32(ANTIC_DLISTH, (addr>>8)&0xFF)
		return 0
	}
}

func (se *ScriptEngine) luaANTICDMA() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(ANTIC_DMACTL, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaANTICScroll() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(ANTIC_HSCROL, uint32(L.CheckInt(1)&0x0F))
		se.bus.Write32(ANTIC_VSCROL, uint32(L.CheckInt(2)&0x0F))
		return 0
	}
}

func (se *ScriptEngine) luaANTICCharset() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(ANTIC_CHBASE, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaANTICPMBase() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(ANTIC_PMBASE, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaANTICDimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		w, h := 0, 0
		if a := runtimeStatus.snapshot().antic; a != nil {
			w, h = a.GetDimensions()
		}
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaGTIAColor() lua.LGFunction {
	return func(L *lua.LState) int {
		reg := L.CheckInt(1)
		val := uint32(L.CheckInt(2) & 0xFF)
		addrs := []uint32{GTIA_COLPF0, GTIA_COLPF1, GTIA_COLPF2, GTIA_COLPF3, GTIA_COLBK, GTIA_COLPM0, GTIA_COLPM1, GTIA_COLPM2, GTIA_COLPM3}
		if reg < 0 || reg >= len(addrs) {
			L.ArgError(1, "invalid GTIA colour register")
			return 0
		}
		se.bus.Write32(addrs[reg], val)
		return 0
	}
}

func (se *ScriptEngine) luaGTIAPlayerPos() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.CheckInt(1)
		x := uint32(L.CheckInt(2) & 0xFF)
		addrs := []uint32{GTIA_HPOSP0, GTIA_HPOSP1, GTIA_HPOSP2, GTIA_HPOSP3}
		if n < 0 || n >= len(addrs) {
			L.ArgError(1, "player index must be 0..3")
			return 0
		}
		se.bus.Write32(addrs[n], x)
		return 0
	}
}

func (se *ScriptEngine) luaGTIAPlayerSize() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.CheckInt(1)
		size := uint32(L.CheckInt(2) & 0x03)
		addrs := []uint32{GTIA_SIZEP0, GTIA_SIZEP1, GTIA_SIZEP2, GTIA_SIZEP3}
		if n < 0 || n >= len(addrs) {
			L.ArgError(1, "player index must be 0..3")
			return 0
		}
		se.bus.Write32(addrs[n], size)
		return 0
	}
}

func (se *ScriptEngine) luaGTIAPlayerGfx() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.CheckInt(1)
		data := uint32(L.CheckInt(2) & 0xFF)
		addrs := []uint32{GTIA_GRAFP0, GTIA_GRAFP1, GTIA_GRAFP2, GTIA_GRAFP3}
		if n < 0 || n >= len(addrs) {
			L.ArgError(1, "player index must be 0..3")
			return 0
		}
		se.bus.Write32(addrs[n], data)
		return 0
	}
}

func (se *ScriptEngine) luaGTIAPriority() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(GTIA_PRIOR, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaTEDEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		if L.CheckBool(1) {
			se.bus.Write32(TED_V_ENABLE, TED_V_ENABLE_VIDEO)
		} else {
			se.bus.Write32(TED_V_ENABLE, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaTEDIsEnabled() lua.LGFunction {
	return func(L *lua.LState) int {
		t := runtimeStatus.snapshot().tedVideo
		L.Push(lua.LBool(t != nil && t.IsEnabled()))
		return 1
	}
}

func (se *ScriptEngine) luaTEDMode() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(TED_V_CTRL1, uint32(L.CheckInt(1)&0xFF))
		se.bus.Write32(TED_V_CTRL2, uint32(L.CheckInt(2)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaTEDColors() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(TED_V_BG_COLOR0, uint32(L.CheckInt(1)&0x7F))
		se.bus.Write32(TED_V_BG_COLOR1, uint32(L.CheckInt(2)&0x7F))
		se.bus.Write32(TED_V_BG_COLOR2, uint32(L.CheckInt(3)&0x7F))
		se.bus.Write32(TED_V_BG_COLOR3, uint32(L.CheckInt(4)&0x7F))
		se.bus.Write32(TED_V_BORDER, uint32(L.CheckInt(5)&0x7F))
		return 0
	}
}

func (se *ScriptEngine) luaTEDCharset() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(TED_V_CHAR_BASE, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaTEDVideoBase() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(TED_V_VIDEO_BASE, uint32(L.CheckInt(1)&0xFF))
		return 0
	}
}

func (se *ScriptEngine) luaTEDCursor() lua.LGFunction {
	return func(L *lua.LState) int {
		pos := uint32(L.CheckInt(1) & 0xFFFF)
		clr := uint32(L.CheckInt(2) & 0x7F)
		se.bus.Write32(TED_V_CURSOR_HI, (pos>>8)&0xFF)
		se.bus.Write32(TED_V_CURSOR_LO, pos&0xFF)
		se.bus.Write32(TED_V_CURSOR_CLR, clr)
		return 0
	}
}

func (se *ScriptEngine) luaTEDDimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		w, h := 0, 0
		if t := runtimeStatus.snapshot().tedVideo; t != nil {
			w, h = t.GetDimensions()
		}
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaVoodooEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		if L.CheckBool(1) {
			se.bus.Write32(VOODOO_ENABLE, 1)
		} else {
			se.bus.Write32(VOODOO_ENABLE, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaVoodooIsEnabled() lua.LGFunction {
	return func(L *lua.LState) int {
		v := runtimeStatus.snapshot().voodoo
		L.Push(lua.LBool(v != nil && v.IsEnabled()))
		return 1
	}
}

func (se *ScriptEngine) luaVoodooResolution() lua.LGFunction {
	return func(L *lua.LState) int {
		w := uint32(L.CheckInt(1) & 0xFFFF)
		h := uint32(L.CheckInt(2) & 0xFFFF)
		se.bus.Write32(VOODOO_VIDEO_DIM, (w<<16)|h)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooVertex() lua.LGFunction {
	return func(L *lua.LState) int {
		ax := uint32(L.CheckInt(1) << VOODOO_FIXED_12_4_SHIFT)
		ay := uint32(L.CheckInt(2) << VOODOO_FIXED_12_4_SHIFT)
		bx := uint32(L.CheckInt(3) << VOODOO_FIXED_12_4_SHIFT)
		by := uint32(L.CheckInt(4) << VOODOO_FIXED_12_4_SHIFT)
		cx := uint32(L.CheckInt(5) << VOODOO_FIXED_12_4_SHIFT)
		cy := uint32(L.CheckInt(6) << VOODOO_FIXED_12_4_SHIFT)
		se.bus.Write32(VOODOO_VERTEX_AX, ax)
		se.bus.Write32(VOODOO_VERTEX_AY, ay)
		se.bus.Write32(VOODOO_VERTEX_BX, bx)
		se.bus.Write32(VOODOO_VERTEX_BY, by)
		se.bus.Write32(VOODOO_VERTEX_CX, cx)
		se.bus.Write32(VOODOO_VERTEX_CY, cy)
		return 0
	}
}

func voodooFixed12_12FromByte(v int) uint32 {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint32((v * (1 << VOODOO_FIXED_12_12_SHIFT)) / 255)
}

func (se *ScriptEngine) luaVoodooColor() lua.LGFunction {
	return func(L *lua.LState) int {
		idx := uint32(L.CheckInt(1) & 0x03)
		r := voodooFixed12_12FromByte(L.CheckInt(2))
		g := voodooFixed12_12FromByte(L.CheckInt(3))
		b := voodooFixed12_12FromByte(L.CheckInt(4))
		a := voodooFixed12_12FromByte(L.CheckInt(5))
		se.bus.Write32(VOODOO_COLOR_SELECT, idx)
		se.bus.Write32(VOODOO_START_R, r)
		se.bus.Write32(VOODOO_START_G, g)
		se.bus.Write32(VOODOO_START_B, b)
		se.bus.Write32(VOODOO_START_A, a)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooDepth() lua.LGFunction {
	return func(L *lua.LState) int {
		z := uint32(L.CheckInt(1) << VOODOO_FIXED_20_12_SHIFT)
		se.bus.Write32(VOODOO_START_Z, z)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooTexcoord() lua.LGFunction {
	return func(L *lua.LState) int {
		s := uint32(int(L.CheckNumber(1) * (1 << VOODOO_FIXED_14_18_SHIFT)))
		t := uint32(int(L.CheckNumber(2) * (1 << VOODOO_FIXED_14_18_SHIFT)))
		w := uint32(int(L.CheckNumber(3) * (1 << VOODOO_FIXED_2_30_SHIFT)))
		se.bus.Write32(VOODOO_START_S, s)
		se.bus.Write32(VOODOO_START_T, t)
		se.bus.Write32(VOODOO_START_W, w)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooDraw() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(VOODOO_TRIANGLE_CMD, 1)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooSwap() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(VOODOO_SWAP_BUFFER_CMD, 1)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooClear() lua.LGFunction {
	return func(L *lua.LState) int {
		r := uint32(L.CheckInt(1) & 0xFF)
		g := uint32(L.CheckInt(2) & 0xFF)
		b := uint32(L.CheckInt(3) & 0xFF)
		se.bus.Write32(VOODOO_COLOR0, r|(g<<8)|(b<<16)|0xFF000000)
		se.bus.Write32(VOODOO_FAST_FILL_CMD, 1)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooFog() lua.LGFunction {
	return func(L *lua.LState) int {
		on := L.CheckBool(1)
		r := uint32(L.CheckInt(2) & 0xFF)
		g := uint32(L.CheckInt(3) & 0xFF)
		b := uint32(L.CheckInt(4) & 0xFF)
		mode := uint32(0)
		if on {
			mode = VOODOO_FOG_ENABLE
		}
		se.bus.Write32(VOODOO_FOG_MODE, mode)
		se.bus.Write32(VOODOO_FOG_COLOR, r|(g<<8)|(b<<16))
		return 0
	}
}

func (se *ScriptEngine) luaVoodooAlpha() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(VOODOO_ALPHA_MODE, uint32(L.CheckInt(1)))
		return 0
	}
}

func (se *ScriptEngine) luaVoodooZBuffer() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(VOODOO_FBZ_MODE, uint32(L.CheckInt(1)))
		return 0
	}
}

func (se *ScriptEngine) luaVoodooClip() lua.LGFunction {
	return func(L *lua.LState) int {
		left := uint32(L.CheckInt(1) & 0xFFFF)
		right := uint32(L.CheckInt(2) & 0xFFFF)
		top := uint32(L.CheckInt(3) & 0xFFFF)
		bottom := uint32(L.CheckInt(4) & 0xFFFF)
		se.bus.Write32(VOODOO_CLIP_LEFT_RIGHT, (left<<16)|right)
		se.bus.Write32(VOODOO_CLIP_LOW_Y_HIGH, (top<<16)|bottom)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooTexture() lua.LGFunction {
	return func(L *lua.LState) int {
		w := L.CheckInt(1)
		h := L.CheckInt(2)
		data := []byte(L.CheckString(3))
		if w <= 0 || h <= 0 {
			L.ArgError(1, "width/height must be > 0")
			return 0
		}
		for i, b := range data {
			se.bus.Write8(VOODOO_TEXMEM_BASE+uint32(i), b)
		}
		se.bus.Write32(VOODOO_TEX_WIDTH, uint32(w))
		se.bus.Write32(VOODOO_TEX_HEIGHT, uint32(h))
		se.bus.Write32(VOODOO_TEX_UPLOAD, 1)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooChromaKey() lua.LGFunction {
	return func(L *lua.LState) int {
		on := L.CheckBool(1)
		r := uint32(L.CheckInt(2) & 0xFF)
		g := uint32(L.CheckInt(3) & 0xFF)
		b := uint32(L.CheckInt(4) & 0xFF)
		se.bus.Write32(VOODOO_CHROMA_KEY, r|(g<<8)|(b<<16))
		fbz := se.bus.Read32(VOODOO_FBZ_MODE)
		if on {
			fbz |= VOODOO_FBZ_CHROMAKEY
		} else {
			fbz &^= VOODOO_FBZ_CHROMAKEY
		}
		se.bus.Write32(VOODOO_FBZ_MODE, fbz)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooDither() lua.LGFunction {
	return func(L *lua.LState) int {
		on := L.CheckBool(1)
		fbz := se.bus.Read32(VOODOO_FBZ_MODE)
		if on {
			fbz |= VOODOO_FBZ_DITHER
		} else {
			fbz &^= VOODOO_FBZ_DITHER
		}
		se.bus.Write32(VOODOO_FBZ_MODE, fbz)
		return 0
	}
}

func (se *ScriptEngine) luaVoodooDimensions() lua.LGFunction {
	return func(L *lua.LState) int {
		w, h := 0, 0
		if v := runtimeStatus.snapshot().voodoo; v != nil {
			w, h = v.GetDimensions()
		}
		L.Push(lua.LNumber(w))
		L.Push(lua.LNumber(h))
		return 2
	}
}

func (se *ScriptEngine) luaCopperEnable() lua.LGFunction {
	return func(L *lua.LState) int {
		if L.CheckBool(1) {
			se.bus.Write32(COPPER_CTRL, copperCtrlEnable)
		} else {
			se.bus.Write32(COPPER_CTRL, 0)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCopperSetProgram() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(COPPER_PTR, uint32(L.CheckInt(1)))
		return 0
	}
}

func (se *ScriptEngine) luaCopperIsRunning() lua.LGFunction {
	return func(L *lua.LState) int {
		status := se.bus.Read32(COPPER_STATUS)
		L.Push(lua.LBool((status & copperStatusRunning) != 0))
		return 1
	}
}

func (se *ScriptEngine) luaBlitCopy() lua.LGFunction {
	return func(L *lua.LState) int {
		src := uint32(L.CheckInt(1))
		dst := uint32(L.CheckInt(2))
		w := uint32(L.CheckInt(3))
		h := uint32(L.CheckInt(4))
		srcStride := uint32(L.CheckInt(5))
		dstStride := uint32(L.CheckInt(6))
		se.bus.Write32(BLT_OP, bltOpCopy)
		se.bus.Write32(BLT_SRC, src)
		se.bus.Write32(BLT_DST, dst)
		se.bus.Write32(BLT_WIDTH, w)
		se.bus.Write32(BLT_HEIGHT, h)
		se.bus.Write32(BLT_SRC_STRIDE, srcStride)
		se.bus.Write32(BLT_DST_STRIDE, dstStride)
		se.bus.Write32(BLT_CTRL, bltCtrlStart)
		return 0
	}
}

func (se *ScriptEngine) luaBlitFill() lua.LGFunction {
	return func(L *lua.LState) int {
		dst := uint32(L.CheckInt(1))
		w := uint32(L.CheckInt(2))
		h := uint32(L.CheckInt(3))
		color := uint32(L.CheckInt64(4))
		dstStride := uint32(L.CheckInt(5))
		se.bus.Write32(BLT_OP, bltOpFill)
		se.bus.Write32(BLT_DST, dst)
		se.bus.Write32(BLT_WIDTH, w)
		se.bus.Write32(BLT_HEIGHT, h)
		se.bus.Write32(BLT_COLOR, color)
		se.bus.Write32(BLT_DST_STRIDE, dstStride)
		se.bus.Write32(BLT_CTRL, bltCtrlStart)
		return 0
	}
}

func (se *ScriptEngine) luaBlitLine() lua.LGFunction {
	return func(L *lua.LState) int {
		x0 := uint32(L.CheckInt(1) & 0xFFFF)
		y0 := uint32(L.CheckInt(2) & 0xFFFF)
		x1 := uint32(L.CheckInt(3) & 0xFFFF)
		y1 := uint32(L.CheckInt(4) & 0xFFFF)
		color := uint32(L.CheckInt64(5))
		se.bus.Write32(BLT_OP, bltOpLine)
		se.bus.Write32(BLT_SRC, (y0<<16)|x0)
		se.bus.Write32(BLT_DST, (y1<<16)|x1)
		se.bus.Write32(BLT_COLOR, color)
		se.bus.Write32(BLT_CTRL, bltCtrlStart)
		return 0
	}
}

func (se *ScriptEngine) luaBlitWait(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		for {
			if (se.bus.Read32(BLT_CTRL) & bltCtrlBusy) == 0 {
				return 0
			}
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-time.After(1 * time.Millisecond):
			}
		}
	}
}

func (se *ScriptEngine) luaVideoGetPixel() lua.LGFunction {
	return func(L *lua.LState) int {
		x := L.CheckInt(1)
		y := L.CheckInt(2)
		frame, w, h := se.compositorFrame()
		if frame == nil || x < 0 || y < 0 || x >= w || y >= h {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 4
		}
		i := (y*w + x) * 4
		L.Push(lua.LNumber(frame[i]))
		L.Push(lua.LNumber(frame[i+1]))
		L.Push(lua.LNumber(frame[i+2]))
		L.Push(lua.LNumber(frame[i+3]))
		return 4
	}
}

func (se *ScriptEngine) luaVideoGetRegion() lua.LGFunction {
	return func(L *lua.LState) int {
		x := L.CheckInt(1)
		y := L.CheckInt(2)
		wr := L.CheckInt(3)
		hr := L.CheckInt(4)
		if wr <= 0 || hr <= 0 {
			L.Push(lua.LString(""))
			return 1
		}

		frame, fw, fh := se.compositorFrame()
		if frame == nil || fw <= 0 || fh <= 0 {
			L.Push(lua.LString(""))
			return 1
		}

		if x < 0 {
			wr += x
			x = 0
		}
		if y < 0 {
			hr += y
			y = 0
		}
		if x >= fw || y >= fh || wr <= 0 || hr <= 0 {
			L.Push(lua.LString(""))
			return 1
		}
		if x+wr > fw {
			wr = fw - x
		}
		if y+hr > fh {
			hr = fh - y
		}

		out := make([]byte, 0, wr*hr*4)
		for row := 0; row < hr; row++ {
			start := ((y+row)*fw + x) * 4
			out = append(out, frame[start:start+wr*4]...)
		}
		L.Push(lua.LString(string(out)))
		return 1
	}
}

func (se *ScriptEngine) luaVideoFrameHash() lua.LGFunction {
	return func(L *lua.LState) int {
		frame, _, _ := se.compositorFrame()
		if len(frame) == 0 {
			L.Push(lua.LNumber(0))
			return 1
		}
		h := fnv.New32a()
		_, _ = h.Write(frame)
		L.Push(lua.LNumber(h.Sum32()))
		return 1
	}
}

func (se *ScriptEngine) luaVideoWaitPixel(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		x := L.CheckInt(1)
		y := L.CheckInt(2)
		r := clamp8(L.CheckInt(3))
		g := clamp8(L.CheckInt(4))
		b := clamp8(L.CheckInt(5))
		timeoutMS := L.CheckInt(6)
		if timeoutMS < 0 {
			L.ArgError(6, "must be >= 0")
			return 0
		}
		deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
		for {
			frame, w, h := se.compositorFrame()
			if frame != nil && x >= 0 && y >= 0 && x < w && y < h {
				i := (y*w + x) * 4
				if withinTol(int(frame[i]), r, 2) &&
					withinTol(int(frame[i+1]), g, 2) &&
					withinTol(int(frame[i+2]), b, 2) {
					L.Push(lua.LBool(true))
					return 1
				}
			}
			if time.Now().After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-se.frameChan:
			}
			se.lastYieldNS.Store(time.Now().UnixNano())
		}
	}
}

func (se *ScriptEngine) luaVideoWaitStable(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		nFrames := L.CheckInt(1)
		timeoutMS := L.CheckInt(2)
		if nFrames <= 0 {
			L.ArgError(1, "must be > 0")
			return 0
		}
		if timeoutMS < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
		var last uint32
		stable := 0
		for {
			frame, _, _ := se.compositorFrame()
			if len(frame) > 0 {
				h := frameHash(frame)
				if stable == 0 || h != last {
					last = h
					stable = 1
				} else {
					stable++
					if stable >= nFrames {
						L.Push(lua.LBool(true))
						return 1
					}
				}
			}
			if time.Now().After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-se.frameChan:
			}
			se.lastYieldNS.Store(time.Now().UnixNano())
		}
	}
}

func (se *ScriptEngine) luaVideoWaitCondition(ctx context.Context) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		timeoutMS := L.CheckInt(2)
		if timeoutMS < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
		for {
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
				L.RaiseError("wait_condition callback failed: %v", err)
				return 0
			}
			ret := L.Get(-1)
			L.Pop(1)
			if lv, ok := ret.(lua.LBool); ok && bool(lv) {
				L.Push(lua.LBool(true))
				return 1
			}
			if time.Now().After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}
			select {
			case <-ctx.Done():
				L.RaiseError("script cancelled")
				return 0
			case <-se.frameChan:
			}
			se.lastYieldNS.Store(time.Now().UnixNano())
		}
	}
}

func (se *ScriptEngine) compositorFrame() ([]byte, int, int) {
	if se.compositor == nil {
		return nil, 0, 0
	}
	frame := se.compositor.GetCurrentFrame()
	w, h := se.compositor.GetDimensions()
	if len(frame) < w*h*4 {
		return nil, w, h
	}
	return frame, w, h
}

func frameHash(frame []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(frame)
	return h.Sum32()
}

func clamp8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func withinTol(v, target, tol int) bool {
	if v < target-tol {
		return false
	}
	return v <= target+tol
}

func (se *ScriptEngine) luaRecScreenshot() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		if err := se.TakeScreenshot(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaRecStart() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		if se.recorder == nil {
			L.RaiseError("recorder unavailable")
			return 0
		}
		se.recorder.SetSoundChip(runtimeStatus.snapshot().sound)
		if err := se.recorder.Start(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaRecStop() lua.LGFunction {
	return func(L *lua.LState) int {
		if se.recorder == nil {
			L.RaiseError("recorder unavailable")
			return 0
		}
		if err := se.recorder.Stop(); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaRecIsRecording() lua.LGFunction {
	return func(L *lua.LState) int {
		recording := false
		if se.recorder != nil {
			recording = se.recorder.IsRecording()
		}
		L.Push(lua.LBool(recording))
		return 1
	}
}

func (se *ScriptEngine) luaRecFrameCount() lua.LGFunction {
	return func(L *lua.LState) int {
		var n uint64
		if se.recorder != nil {
			n = se.recorder.FrameCount()
		}
		L.Push(lua.LNumber(n))
		return 1
	}
}

func (se *ScriptEngine) luaRecStartScreen() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		if se.recorder == nil {
			L.RaiseError("recorder unavailable")
			return 0
		}
		se.recorder.SetSoundChip(runtimeStatus.snapshot().sound)
		if err := se.recorder.StartScreen(path); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplShow() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.Show()
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplHide() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.Hide()
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplIsOpen() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		v := false
		if overlay != nil {
			v = overlay.IsActive()
		}
		L.Push(lua.LBool(v))
		return 1
	}
}

func (se *ScriptEngine) luaReplPrint() lua.LGFunction {
	return func(L *lua.LState) int {
		text := L.CheckString(1)
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.AppendLine(text)
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplClear() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.Clear()
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplScrollUp() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.OptInt(1, 1)
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.ScrollUp(n)
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplScrollDown() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.OptInt(1, 1)
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		if overlay != nil {
			overlay.ScrollDown(n)
		}
		return 0
	}
}

func (se *ScriptEngine) luaReplLineCount() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		overlay := se.luaOverlay
		se.mu.Unlock()
		n := 0
		if overlay != nil {
			n = overlay.LineCount()
		}
		L.Push(lua.LNumber(n))
		return 1
	}
}

func (se *ScriptEngine) TakeScreenshot(path string) error {
	if se.compositor == nil {
		return fmt.Errorf("compositor unavailable")
	}
	frame := se.compositor.GetCurrentFrame()
	w, h := se.compositor.GetDimensions()
	if len(frame) == 0 || w <= 0 || h <= 0 {
		return fmt.Errorf("no frame available")
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		row := y * w * 4
		for x := range w {
			i := row + x*4
			img.SetRGBA(x, y, color.RGBA{
				R: frame[i],
				G: frame[i+1],
				B: frame[i+2],
				A: frame[i+3],
			})
		}
	}

	// Composite software cursor if terminal MMIO is available (AROS/EmuTOS mode).
	if se.terminal != nil && se.terminal.mouseOverride.Load() {
		mx := int(se.terminal.mouseX.Load())
		my := int(se.terminal.mouseY.Load())
		drawSoftwareCursor(img, mx, my)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// drawSoftwareCursor renders a classic Amiga-style arrow cursor onto an image.
func drawSoftwareCursor(img *image.RGBA, mx, my int) {
	cursor := [16][16]byte{
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 1, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 0, 0, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	bounds := img.Bounds()
	for cy := range 16 {
		for cx := range 16 {
			px, py := mx+cx, my+cy
			if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
				continue
			}
			switch cursor[cy][cx] {
			case 1:
				img.SetRGBA(px, py, color.RGBA{0, 0, 0, 255})
			case 2:
				img.SetRGBA(px, py, color.RGBA{255, 255, 255, 255})
			}
		}
	}
}

func (se *ScriptEngine) luaDbgOpen() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		if !mon.IsActive() {
			mon.Activate()
		}
		se.freezeCount.Add(1)
		return 0
	}
}

func (se *ScriptEngine) luaDbgClose() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		if mon.IsActive() {
			mon.Deactivate()
		}
		if se.freezeCount.Load() > 0 {
			se.freezeCount.Add(-1)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgIsOpen() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.Push(lua.LBool(false))
			return 1
		}
		L.Push(lua.LBool(mon.IsActive()))
		return 1
	}
}

func (se *ScriptEngine) luaDbgFreeze() lua.LGFunction {
	return se.luaDbgOpen()
}

func (se *ScriptEngine) luaDbgResume() lua.LGFunction {
	return se.luaDbgClose()
}

func (se *ScriptEngine) luaDbgStep() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.OptInt(1, 1)
		if n <= 0 {
			L.ArgError(1, "must be > 0")
			return 0
		}
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		for range n {
			mon.ExecuteCommand("s")
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgContinue() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("g")
		return 0
	}
}

func (se *ScriptEngine) getMonitorAndCPU() (*MachineMonitor, DebuggableCPU, error) {
	se.mu.Lock()
	mon := se.monitor
	se.mu.Unlock()
	if mon == nil {
		return nil, nil, fmt.Errorf("monitor unavailable")
	}
	entry := mon.FocusedCPU()
	if entry == nil || entry.CPU == nil {
		return mon, nil, fmt.Errorf("no focused cpu")
	}
	return mon, entry.CPU, nil
}

func (se *ScriptEngine) luaDbgRunUntil() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("u $%X", addr))
		return 0
	}
}

func (se *ScriptEngine) luaDbgSetBP() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.SetBreakpoint(addr)
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgSetConditionalBP() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		cond := L.CheckString(2)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("b $%X %s", addr, cond))
		return 0
	}
}

func (se *ScriptEngine) luaDbgClearBP() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.ClearBreakpoint(addr)
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgClearAllBP() lua.LGFunction {
	return func(L *lua.LState) int {
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.ClearAllBreakpoints()
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgListBP() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.Push(t)
			return 1
		}
		i := 1
		for _, bp := range cpu.ListConditionalBreakpoints() {
			e := L.NewTable()
			e.RawSetString("addr", lua.LNumber(bp.Address))
			cond := ""
			if bp.Condition != nil {
				cond = formatBreakpointCondition(bp.Condition)
			}
			e.RawSetString("condition", lua.LString(cond))
			e.RawSetString("hit_count", lua.LNumber(bp.HitCount))
			t.RawSetInt(i, e)
			i++
		}
		L.Push(t)
		return 1
	}
}

func formatBreakpointCondition(cond *BreakpointCondition) string {
	if cond == nil {
		return ""
	}
	var lhs string
	switch cond.Source {
	case CondSourceRegister:
		lhs = cond.RegName
	case CondSourceMemory:
		lhs = fmt.Sprintf("[$%X]", cond.MemAddr)
	case CondSourceHitCount:
		lhs = "hitcount"
	default:
		lhs = "unknown"
	}
	op := "=="
	switch cond.Op {
	case CondOpNotEqual:
		op = "!="
	case CondOpLess:
		op = "<"
	case CondOpGreater:
		op = ">"
	case CondOpLessEqual:
		op = "<="
	case CondOpGreaterEqual:
		op = ">="
	}
	return fmt.Sprintf("%s%s$%X", lhs, op, cond.Value)
}

func (se *ScriptEngine) luaDbgSetWP() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.SetWatchpoint(addr)
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgClearWP() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.ClearWatchpoint(addr)
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgClearAllWP() lua.LGFunction {
	return func(L *lua.LState) int {
		if _, cpu, err := se.getMonitorAndCPU(); err == nil {
			cpu.ClearAllWatchpoints()
		} else {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgListWP() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.Push(t)
			return 1
		}
		for i, addr := range cpu.ListWatchpoints() {
			t.RawSetInt(i+1, lua.LNumber(addr))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgGetReg() lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		val, ok := cpu.GetRegister(name)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(val))
		return 1
	}
}

func (se *ScriptEngine) luaDbgSetReg() lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		val := uint64(L.CheckInt64(2))
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		if !cpu.SetRegister(name, val) {
			L.RaiseError("unknown register: %s", name)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgGetRegs() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.Push(t)
			return 1
		}
		for _, r := range cpu.GetRegisters() {
			t.RawSetString(r.Name, lua.LNumber(r.Value))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgGetPC() lua.LGFunction {
	return func(L *lua.LState) int {
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		L.Push(lua.LNumber(cpu.GetPC()))
		return 1
	}
}

func (se *ScriptEngine) luaDbgSetPC() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		cpu.SetPC(addr)
		return 0
	}
}

func (se *ScriptEngine) luaDbgReadMem() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		n := L.CheckInt(2)
		if n < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		L.Push(lua.LString(string(cpu.ReadMemory(addr, n))))
		return 1
	}
}

func (se *ScriptEngine) luaDbgWriteMem() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		data := []byte(L.CheckString(2))
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		cpu.WriteMemory(addr, data)
		return 0
	}
}

func (se *ScriptEngine) luaDbgFillMem() lua.LGFunction {
	return func(L *lua.LState) int {
		start := uint32(L.CheckInt64(1))
		n := L.CheckInt(2)
		val := uint8(L.CheckInt(3))
		if n < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		for i := range n {
			se.bus.Write8(start+uint32(i), val)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgHuntMem() lua.LGFunction {
	return func(L *lua.LState) int {
		start := uint32(L.CheckInt64(1))
		n := L.CheckInt(2)
		pat := []byte(L.CheckString(3))
		out := L.NewTable()
		if n <= 0 || len(pat) == 0 || len(pat) > n {
			L.Push(out)
			return 1
		}
		limit := n - len(pat)
		idx := 1
		for i := 0; i <= limit; i++ {
			match := true
			for j := range len(pat) {
				if se.bus.Read8(start+uint32(i+j)) != pat[j] {
					match = false
					break
				}
			}
			if match {
				out.RawSetInt(idx, lua.LNumber(start+uint32(i)))
				idx++
			}
		}
		L.Push(out)
		return 1
	}
}

func (se *ScriptEngine) luaDbgCompareMem() lua.LGFunction {
	return func(L *lua.LState) int {
		start := uint32(L.CheckInt64(1))
		n := L.CheckInt(2)
		dest := uint32(L.CheckInt64(3))
		out := L.NewTable()
		if n <= 0 {
			L.Push(out)
			return 1
		}
		idx := 1
		for i := range n {
			v1 := se.bus.Read8(start + uint32(i))
			v2 := se.bus.Read8(dest + uint32(i))
			if v1 == v2 {
				continue
			}
			e := L.NewTable()
			e.RawSetString("offset", lua.LNumber(i))
			e.RawSetString("val1", lua.LNumber(v1))
			e.RawSetString("val2", lua.LNumber(v2))
			out.RawSetInt(idx, e)
			idx++
		}
		L.Push(out)
		return 1
	}
}

func (se *ScriptEngine) luaDbgTransferMem() lua.LGFunction {
	return func(L *lua.LState) int {
		start := uint32(L.CheckInt64(1))
		n := L.CheckInt(2)
		dest := uint32(L.CheckInt64(3))
		if n < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		buf := make([]byte, n)
		for i := range n {
			buf[i] = se.bus.Read8(start + uint32(i))
		}
		for i, b := range buf {
			se.bus.Write8(dest+uint32(i), b)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgBacktrace() lua.LGFunction {
	return func(L *lua.LState) int {
		depth := max(L.OptInt(1, 8), 1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		lines := se.monitorCommandOutput(mon, fmt.Sprintf("bt %d", depth))
		t := L.NewTable()
		for i, line := range lines {
			t.RawSetInt(i+1, lua.LString(line))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgDisasm() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		count := L.CheckInt(2)
		if count < 0 {
			L.ArgError(2, "must be >= 0")
			return 0
		}
		out := L.NewTable()
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.Push(out)
			return 1
		}
		lines := cpu.Disassemble(addr, count)
		for i, d := range lines {
			e := L.NewTable()
			e.RawSetString("addr", lua.LNumber(d.Address))
			e.RawSetString("hex", lua.LString(d.HexBytes))
			e.RawSetString("mnemonic", lua.LString(d.Mnemonic))
			out.RawSetInt(i+1, e)
		}
		L.Push(out)
		return 1
	}
}

func (se *ScriptEngine) luaDbgTrace() lua.LGFunction {
	return func(L *lua.LState) int {
		n := L.CheckInt(1)
		if n < 0 {
			L.ArgError(1, "must be >= 0")
			return 0
		}
		out := L.NewTable()
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.Push(out)
			return 1
		}
		for i := range n {
			pc := cpu.GetPC()
			dis := cpu.Disassemble(pc, 1)
			_ = cpu.Step()
			e := L.NewTable()
			if len(dis) > 0 {
				e.RawSetString("addr", lua.LNumber(dis[0].Address))
				e.RawSetString("mnemonic", lua.LString(dis[0].Mnemonic))
			} else {
				e.RawSetString("addr", lua.LNumber(pc))
				e.RawSetString("mnemonic", lua.LString(""))
			}
			e.RawSetString("reg_changes", L.NewTable())
			out.RawSetInt(i+1, e)
		}
		L.Push(out)
		return 1
	}
}

func (se *ScriptEngine) luaDbgBackstep() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("bs")
		return 0
	}
}

func (se *ScriptEngine) luaDbgTraceFile() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("trace file " + path)
		return 0
	}
}

func (se *ScriptEngine) luaDbgTraceFileOff() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("trace file off")
		return 0
	}
}

func (se *ScriptEngine) luaDbgTraceWatchAdd() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("trace watch add $%X", addr))
		return 0
	}
}

func (se *ScriptEngine) luaDbgTraceWatchDel() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := uint64(L.CheckInt64(1))
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("trace watch del $%X", addr))
		return 0
	}
}

func (se *ScriptEngine) luaDbgTraceWatchList() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		t := L.NewTable()
		mon.mu.Lock()
		addrs := make([]uint64, 0, len(mon.traceWatches))
		for addr := range mon.traceWatches {
			addrs = append(addrs, addr)
		}
		mon.mu.Unlock()
		for i, addr := range addrs {
			t.RawSetInt(i+1, lua.LNumber(addr))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgTraceHistory() lua.LGFunction {
	return func(L *lua.LState) int {
		addrStr := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		if addrStr == "*" {
			L.Push(L.NewTable())
			return 1
		}
		addr, ok := ParseAddress(addrStr)
		if !ok {
			L.RaiseError("invalid address: %s", addrStr)
			return 0
		}
		mon.mu.Lock()
		hist := append([]WriteRecord(nil), mon.writeHistory[addr]...)
		mon.mu.Unlock()
		t := L.NewTable()
		for i, wr := range hist {
			e := L.NewTable()
			e.RawSetString("pc", lua.LNumber(wr.PC))
			e.RawSetString("old_val", lua.LNumber(wr.OldValue))
			e.RawSetString("new_val", lua.LNumber(wr.NewValue))
			t.RawSetInt(i+1, e)
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgTraceHistoryClear() lua.LGFunction {
	return func(L *lua.LState) int {
		addr := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("trace history clear " + addr)
		return 0
	}
}

func (se *ScriptEngine) monitorCommandOutput(mon *MachineMonitor, cmd string) []string {
	mon.mu.Lock()
	before := len(mon.outputLines)
	mon.mu.Unlock()
	mon.ExecuteCommand(cmd)
	mon.mu.Lock()
	defer mon.mu.Unlock()
	if before < 0 || before > len(mon.outputLines) {
		before = 0
	}
	out := make([]string, 0, len(mon.outputLines)-before)
	for _, line := range mon.outputLines[before:] {
		out = append(out, line.Text)
	}
	return out
}

func (se *ScriptEngine) luaDbgSaveState() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("ss " + path)
		return 0
	}
}

func (se *ScriptEngine) luaDbgLoadState() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("sl " + path)
		return 0
	}
}

func (se *ScriptEngine) luaDbgSaveMemFile() lua.LGFunction {
	return func(L *lua.LState) int {
		start := uint64(L.CheckInt64(1))
		length := uint64(L.CheckInt64(2))
		path := L.CheckString(3)
		if length == 0 {
			return 0
		}
		end := start + length - 1
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("save $%X $%X %s", start, end, path))
		return 0
	}
}

func (se *ScriptEngine) luaDbgLoadMemFile() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		addr := uint64(L.CheckInt64(2))
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(fmt.Sprintf("load %s $%X", path, addr))
		return 0
	}
}

func (se *ScriptEngine) luaDbgCPUList() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.Push(t)
			return 1
		}
		mon.mu.Lock()
		defer mon.mu.Unlock()
		i := 1
		for _, entry := range mon.cpus {
			e := L.NewTable()
			e.RawSetString("id", lua.LNumber(entry.ID))
			e.RawSetString("label", lua.LString(entry.Label))
			e.RawSetString("cpu_name", lua.LString(entry.CPU.CPUName()))
			e.RawSetString("is_running", lua.LBool(entry.CPU.IsRunning()))
			t.RawSetInt(i, e)
			i++
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgCPUFocus() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		arg := L.Get(1)
		switch v := arg.(type) {
		case lua.LNumber:
			mon.ExecuteCommand(fmt.Sprintf("cpu %d", int(v)))
		default:
			mon.ExecuteCommand("cpu " + L.CheckString(1))
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgFreezeCPU() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("freeze " + L.CheckString(1))
		return 0
	}
}

func (se *ScriptEngine) luaDbgThawCPU() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("thaw " + L.CheckString(1))
		return 0
	}
}

func (se *ScriptEngine) luaDbgFreezeAll() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("freeze *")
		return 0
	}
}

func (se *ScriptEngine) luaDbgThawAll() lua.LGFunction {
	return func(L *lua.LState) int {
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("thaw *")
		return 0
	}
}

func (se *ScriptEngine) luaDbgFreezeAudio() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.audioFrozen.Store(true)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgThawAudio() lua.LGFunction {
	return func(L *lua.LState) int {
		if s := runtimeStatus.snapshot().sound; s != nil {
			s.audioFrozen.Store(false)
		}
		return 0
	}
}

func (se *ScriptEngine) luaDbgIODevices() lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		for i, name := range listIODevices() {
			t.RawSetInt(i+1, lua.LString(name))
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgIO() lua.LGFunction {
	return func(L *lua.LState) int {
		device := L.CheckString(1)
		t := L.NewTable()
		desc, ok := ioDevices[device]
		if !ok {
			L.Push(t)
			return 1
		}
		_, cpu, err := se.getMonitorAndCPU()
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		for i, reg := range desc.Registers {
			data := cpu.ReadMemory(uint64(reg.Addr), reg.Width)
			val := uint32(0)
			for j := 0; j < len(data) && j < 4; j++ {
				val |= uint32(data[j]) << (8 * j)
			}
			e := L.NewTable()
			e.RawSetString("name", lua.LString(reg.Name))
			e.RawSetString("addr", lua.LNumber(reg.Addr))
			e.RawSetString("value", lua.LNumber(val))
			e.RawSetString("access", lua.LString(reg.Access))
			t.RawSetInt(i+1, e)
		}
		L.Push(t)
		return 1
	}
}

func (se *ScriptEngine) luaDbgRunScript() lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("script " + path)
		return 0
	}
}

func (se *ScriptEngine) luaDbgMacro() lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		cmds := L.CheckString(2)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand("macro " + name + " " + cmds)
		return 0
	}
}

func (se *ScriptEngine) luaDbgCommand() lua.LGFunction {
	return func(L *lua.LState) int {
		cmd := L.CheckString(1)
		se.mu.Lock()
		mon := se.monitor
		se.mu.Unlock()
		if mon == nil {
			L.RaiseError("monitor unavailable")
			return 0
		}
		mon.ExecuteCommand(cmd)
		return 0
	}
}

func (se *ScriptEngine) scriptScratchRange() (uint32, uint32) {
	mem := se.bus.GetMemory()
	if len(mem) == 0 {
		return 0, 0
	}
	const preferredBase = uint32(0x810000)
	const preferredSize = uint32(0x10000)
	memLen := uint32(len(mem))
	if memLen > preferredBase+preferredSize {
		return preferredBase, preferredSize
	}
	if memLen <= preferredSize+1 {
		return 0, memLen - 1
	}
	return memLen - preferredSize, preferredSize
}

func (se *ScriptEngine) writeScratchBytes(data []byte, addNull bool) (uint32, error) {
	base, size := se.scriptScratchRange()
	if size == 0 {
		return 0, fmt.Errorf("scratch memory unavailable")
	}
	needed := uint32(len(data))
	if addNull {
		needed++
	}
	if needed == 0 {
		needed = 1
	}
	if needed >= size {
		return 0, fmt.Errorf("scratch buffer too small")
	}

	se.scratchMu.Lock()
	defer se.scratchMu.Unlock()

	if se.scratchNext == 0 {
		se.scratchNext = base
	}
	if se.scratchNext < base || se.scratchNext+needed > base+size {
		se.scratchNext = base
	}
	addr := se.scratchNext
	se.scratchNext += needed

	for i, b := range data {
		se.bus.Write8(addr+uint32(i), b)
	}
	if addNull {
		se.bus.Write8(addr+uint32(len(data)), 0)
	}
	return addr, nil
}

func coprocCPUTypeFromString(v string) uint32 {
	switch strings.ToLower(v) {
	case "ie32":
		return EXEC_TYPE_IE32
	case "6502":
		return EXEC_TYPE_6502
	case "m68k":
		return EXEC_TYPE_M68K
	case "z80":
		return EXEC_TYPE_Z80
	case "x86":
		return EXEC_TYPE_X86
	default:
		return EXEC_TYPE_NONE
	}
}

func coprocCPUTypeToString(cpuType uint32) string {
	switch cpuType {
	case EXEC_TYPE_IE32:
		return "ie32"
	case EXEC_TYPE_IE64:
		return "ie64"
	case EXEC_TYPE_6502:
		return "6502"
	case EXEC_TYPE_M68K:
		return "m68k"
	case EXEC_TYPE_Z80:
		return "z80"
	case EXEC_TYPE_X86:
		return "x86"
	default:
		return "unknown"
	}
}

func coprocStatusToString(status uint32) string {
	switch status {
	case COPROC_TICKET_PENDING:
		return "pending"
	case COPROC_TICKET_RUNNING:
		return "running"
	case COPROC_TICKET_OK:
		return "ok"
	case COPROC_TICKET_ERROR:
		return "error"
	case COPROC_TICKET_TIMEOUT:
		return "timeout"
	case COPROC_TICKET_WORKER_DOWN:
		return "error"
	default:
		return "error"
	}
}

func mediaStatusToString(status uint32) string {
	switch status {
	case MEDIA_STATUS_IDLE:
		return "idle"
	case MEDIA_STATUS_LOADING:
		return "loading"
	case MEDIA_STATUS_PLAYING:
		return "playing"
	case MEDIA_STATUS_ERROR:
		return "error"
	default:
		return "error"
	}
}

func mediaTypeToString(typ uint32) string {
	switch typ {
	case MEDIA_TYPE_SID:
		return "sid"
	case MEDIA_TYPE_PSG:
		return "psg"
	case MEDIA_TYPE_TED:
		return "ted"
	case MEDIA_TYPE_AHX:
		return "ahx"
	case MEDIA_TYPE_POKEY:
		return "pokey"
	default:
		return "none"
	}
}

func (se *ScriptEngine) coprocRunCommand(cmd uint32) (uint32, uint32) {
	se.bus.Write32(COPROC_CMD, cmd)
	return se.bus.Read32(COPROC_CMD_STATUS), se.bus.Read32(COPROC_CMD_ERROR)
}

func (se *ScriptEngine) coprocFindResponse(ticket uint32) (uint32, uint32, uint32, bool) {
	for i := range 5 {
		ringBase := ringBaseAddr(i)
		for slot := range uint32(RING_CAPACITY) {
			reqAddr := ringBase + RING_ENTRIES_OFFSET + slot*REQ_DESC_SIZE
			respAddr := ringBase + RING_RESPONSES_OFFSET + slot*RESP_DESC_SIZE
			if se.bus.Read32(reqAddr+REQ_TICKET_OFF) != ticket || se.bus.Read32(respAddr+RESP_TICKET_OFF) != ticket {
				continue
			}
			respPtr := se.bus.Read32(reqAddr + REQ_RESP_PTR_OFF)
			respCap := se.bus.Read32(reqAddr + REQ_RESP_CAP_OFF)
			respLen := se.bus.Read32(respAddr + RESP_RESP_LEN_OFF)
			status := se.bus.Read32(respAddr + RESP_STATUS_OFF)
			return respPtr, min(respCap, respLen), status, true
		}
	}
	return 0, 0, COPROC_TICKET_PENDING, false
}

func (se *ScriptEngine) readRawBytes(addr uint32, n uint32) string {
	if n == 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range n {
		out[i] = se.bus.Read8(addr + i)
	}
	return string(out)
}

func (se *ScriptEngine) luaCoprocStart() lua.LGFunction {
	return func(L *lua.LState) int {
		cpuType := coprocCPUTypeFromString(L.CheckString(1))
		if cpuType == EXEC_TYPE_NONE || cpuType == EXEC_TYPE_IE64 {
			L.RaiseError("unsupported coprocessor cpu_type")
			return 0
		}
		namePtr, err := se.writeScratchBytes([]byte(L.CheckString(2)), true)
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		se.bus.Write32(COPROC_CPU_TYPE, cpuType)
		se.bus.Write32(COPROC_NAME_PTR, namePtr)
		status, code := se.coprocRunCommand(COPROC_CMD_START)
		if status != COPROC_STATUS_OK {
			L.RaiseError("coproc.start failed (%d)", code)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCoprocStop() lua.LGFunction {
	return func(L *lua.LState) int {
		cpuType := coprocCPUTypeFromString(L.CheckString(1))
		if cpuType == EXEC_TYPE_NONE || cpuType == EXEC_TYPE_IE64 {
			L.RaiseError("unsupported coprocessor cpu_type")
			return 0
		}
		se.bus.Write32(COPROC_CPU_TYPE, cpuType)
		status, code := se.coprocRunCommand(COPROC_CMD_STOP)
		if status != COPROC_STATUS_OK {
			L.RaiseError("coproc.stop failed (%d)", code)
		}
		return 0
	}
}

func (se *ScriptEngine) luaCoprocEnqueue() lua.LGFunction {
	return func(L *lua.LState) int {
		cpuType := coprocCPUTypeFromString(L.CheckString(1))
		if cpuType == EXEC_TYPE_NONE || cpuType == EXEC_TYPE_IE64 {
			L.RaiseError("unsupported coprocessor cpu_type")
			return 0
		}
		op := uint32(L.CheckInt(2))
		req := []byte(L.CheckString(3))

		reqPtr, err := se.writeScratchBytes(req, false)
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}

		respCap := max(uint32(len(req)*2), 1024)
		respPtr, err := se.writeScratchBytes(make([]byte, respCap), false)
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}

		se.bus.Write32(COPROC_CPU_TYPE, cpuType)
		se.bus.Write32(COPROC_OP, op)
		se.bus.Write32(COPROC_REQ_PTR, reqPtr)
		se.bus.Write32(COPROC_REQ_LEN, uint32(len(req)))
		se.bus.Write32(COPROC_RESP_PTR, respPtr)
		se.bus.Write32(COPROC_RESP_CAP, respCap)

		status, code := se.coprocRunCommand(COPROC_CMD_ENQUEUE)
		if status != COPROC_STATUS_OK {
			L.RaiseError("coproc.enqueue failed (%d)", code)
			return 0
		}

		ticket := se.bus.Read32(COPROC_TICKET)
		se.coprocMu.Lock()
		se.coprocTickets[ticket] = coprocTicketBuf{respPtr: respPtr, respCap: respCap}
		se.coprocMu.Unlock()

		L.Push(lua.LNumber(ticket))
		return 1
	}
}

func (se *ScriptEngine) luaCoprocPoll() lua.LGFunction {
	return func(L *lua.LState) int {
		ticket := uint32(L.CheckInt(1))
		se.bus.Write32(COPROC_TICKET, ticket)
		status, code := se.coprocRunCommand(COPROC_CMD_POLL)
		if status != COPROC_STATUS_OK {
			L.RaiseError("coproc.poll failed (%d)", code)
			return 0
		}
		L.Push(lua.LString(coprocStatusToString(se.bus.Read32(COPROC_TICKET_STATUS))))
		return 1
	}
}

func (se *ScriptEngine) luaCoprocWait() lua.LGFunction {
	return func(L *lua.LState) int {
		ticket := uint32(L.CheckInt(1))
		timeoutMS := uint32(L.CheckInt(2))
		se.bus.Write32(COPROC_TICKET, ticket)
		se.bus.Write32(COPROC_TIMEOUT, timeoutMS)
		status, code := se.coprocRunCommand(COPROC_CMD_WAIT)
		if status != COPROC_STATUS_OK {
			L.RaiseError("coproc.wait failed (%d)", code)
			return 0
		}
		ticketStatus := se.bus.Read32(COPROC_TICKET_STATUS)
		L.Push(lua.LString(coprocStatusToString(ticketStatus)))
		if respPtr, respLen, _, ok := se.coprocFindResponse(ticket); ok && ticketStatus == COPROC_TICKET_OK {
			L.Push(lua.LString(se.readRawBytes(respPtr, respLen)))
		} else {
			L.Push(lua.LString(""))
		}
		return 2
	}
}

func (se *ScriptEngine) luaCoprocWorkers() lua.LGFunction {
	return func(L *lua.LState) int {
		mask := se.bus.Read32(COPROC_WORKER_STATE)
		tbl := L.NewTable()
		idx := 1
		for cpuType := uint32(1); cpuType <= uint32(EXEC_TYPE_X86); cpuType++ {
			if mask&(1<<cpuType) == 0 {
				continue
			}
			entry := L.NewTable()
			entry.RawSetString("cpu_type", lua.LString(coprocCPUTypeToString(cpuType)))
			entry.RawSetString("is_running", lua.LBool(true))
			tbl.RawSetInt(idx, entry)
			idx++
		}
		L.Push(tbl)
		return 1
	}
}

func (se *ScriptEngine) luaCoprocResponse() lua.LGFunction {
	return func(L *lua.LState) int {
		ticket := uint32(L.CheckInt(1))
		respPtr, respLen, status, ok := se.coprocFindResponse(ticket)
		if !ok {
			se.coprocMu.Lock()
			buf, known := se.coprocTickets[ticket]
			se.coprocMu.Unlock()
			if known && buf.respCap > 0 {
				L.Push(lua.LString(se.readRawBytes(buf.respPtr, buf.respCap)))
				return 1
			}
			L.Push(lua.LString(""))
			return 1
		}
		if status != COPROC_TICKET_OK {
			L.Push(lua.LString(""))
			return 1
		}
		L.Push(lua.LString(se.readRawBytes(respPtr, respLen)))
		return 1
	}
}

func (se *ScriptEngine) mediaPlayWith(filename string, subsong uint32) error {
	namePtr, err := se.writeScratchBytes([]byte(filename), true)
	if err != nil {
		return err
	}
	se.bus.Write32(MEDIA_NAME_PTR, namePtr)
	se.bus.Write32(MEDIA_SUBSONG, subsong)
	se.bus.Write32(MEDIA_CTRL, MEDIA_OP_PLAY)
	return nil
}

func (se *ScriptEngine) luaMediaLoad() lua.LGFunction {
	return func(L *lua.LState) int {
		if err := se.mediaPlayWith(L.CheckString(1), 0); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaMediaLoadSubsong() lua.LGFunction {
	return func(L *lua.LState) int {
		filename := L.CheckString(1)
		subsong := uint32(L.CheckInt(2))
		if err := se.mediaPlayWith(filename, subsong); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}
}

func (se *ScriptEngine) luaMediaPlay() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(MEDIA_CTRL, MEDIA_OP_PLAY)
		return 0
	}
}

func (se *ScriptEngine) luaMediaStop() lua.LGFunction {
	return func(L *lua.LState) int {
		se.bus.Write32(MEDIA_CTRL, MEDIA_OP_STOP)
		return 0
	}
}

func (se *ScriptEngine) luaMediaStatus() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LString(mediaStatusToString(se.bus.Read32(MEDIA_STATUS))))
		return 1
	}
}

func (se *ScriptEngine) luaMediaType() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LString(mediaTypeToString(se.bus.Read32(MEDIA_TYPE))))
		return 1
	}
}

func (se *ScriptEngine) luaMediaError() lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(lua.LNumber(se.bus.Read32(MEDIA_ERROR)))
		return 1
	}
}
