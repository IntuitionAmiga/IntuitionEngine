package main

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

type fakeMachineScript struct {
	runPath string
	runErr  error
}

func (s *fakeMachineScript) Cancel()                   {}
func (s *fakeMachineScript) IsLoadingProgram() bool    { return false }
func (s *fakeMachineScript) RunFile(path string) error { s.runPath = path; return s.runErr }

type fakeMachineMedia struct {
	path    string
	subsong uint32
	err     error
}

func (m *fakeMachineMedia) PlayHostPath(path string, subsong uint32) error {
	m.path = path
	m.subsong = subsong
	return m.err
}

type orderedCalls struct {
	calls []string
}

func (o *orderedCalls) add(name string) {
	o.calls = append(o.calls, name)
}

type fakeMachineCompositor struct{ order *orderedCalls }

func (c fakeMachineCompositor) Start() error { c.order.add("compositor.start"); return nil }
func (c fakeMachineCompositor) Stop()        { c.order.add("compositor.stop") }

type fakeMachineVideoStarter struct{ order *orderedCalls }

func (v fakeMachineVideoStarter) Start() error { v.order.add("video.start"); return nil }

type fakeMachineSoundStarter struct{ order *orderedCalls }

func (s fakeMachineSoundStarter) Start() { s.order.add("sound.start") }

type fakeMachineRenderLoop struct {
	name  string
	order *orderedCalls
}

func (r fakeMachineRenderLoop) StartRenderLoop() { r.order.add(r.name + ".start") }
func (r fakeMachineRenderLoop) StopRenderLoop()  { r.order.add(r.name + ".stop") }

type nilSensitiveRenderLoop struct{}

func (r *nilSensitiveRenderLoop) StartRenderLoop() {
	if r == nil {
		panic("StartRenderLoop called on nil render loop")
	}
}

func (r *nilSensitiveRenderLoop) StopRenderLoop() {
	if r == nil {
		panic("StopRenderLoop called on nil render loop")
	}
}

type fakeMachineCPU struct{ order *orderedCalls }

func (c fakeMachineCPU) LoadProgram(filename string) error { return nil }
func (c fakeMachineCPU) Reset()                            { c.order.add("cpu.reset") }
func (c fakeMachineCPU) Execute()                          { c.order.add("cpu.execute") }
func (c fakeMachineCPU) Stop()                             { c.order.add("cpu.stop") }
func (c fakeMachineCPU) StartExecution()                   { c.order.add("cpu.start") }

type fakeMachineMonitor struct {
	order  *orderedCalls
	active bool
}

func (m fakeMachineMonitor) Deactivate()                                     { m.order.add("monitor.deactivate") }
func (m fakeMachineMonitor) IsActive() bool                                  { return m.active }
func (m fakeMachineMonitor) RegisterCPU(label string, cpu DebuggableCPU) int { return 0 }
func (m fakeMachineMonitor) RegisterSnapshotDevice(dev DebugSnapshotDevice)  {}
func (m fakeMachineMonitor) ResetCPUs()                                      { m.order.add("monitor.resetCPUs") }

type fakeMachineMediaPlayers struct{ order *orderedCalls }

func (m fakeMachineMediaPlayers) stopPlayersOnly() { m.order.add("media.stopPlayers") }

type fakeMachineROMLoader struct {
	name  string
	order *orderedCalls
}

func (l fakeMachineROMLoader) Stop() { l.order.add(l.name + ".stop") }

type nilSensitiveROMLoader struct{}

func (l *nilSensitiveROMLoader) Stop() {
	if l == nil {
		panic("Stop called on nil ROM loader")
	}
}

type fakeMachineResetDevice struct {
	name  string
	order *orderedCalls
}

func (d fakeMachineResetDevice) Reset() { d.order.add(d.name + ".reset") }

func testLifecycleM68KProfileTargets(t *testing.T) (MachineProfileLoadTargets, *M68KRunner, func()) {
	t.Helper()

	bus, err := NewMachineBusSized(128 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cpu := NewM68KCPU(bus)
	runner := NewM68KRunner(cpu)
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		_ = video.Stop()
		t.Fatalf("NewSoundChip: %v", err)
	}
	monitor := NewMachineMonitor(bus)
	status := &runtimeStatusStore{}
	term := NewTerminalMMIO()

	targets := MachineProfileLoadTargets{
		Bus:           bus,
		Runner:        runner,
		VideoChip:     video,
		PSGEngine:     NewPSGEngine(sound, SAMPLE_RATE),
		Monitor:       monitor,
		RuntimeStatus: status,
		SoundChip:     sound,
		TermMMIO:      term,
	}
	cleanup := func() {
		_ = video.Stop()
	}
	return targets, runner, cleanup
}

func testLifecycleROM(pc uint32) []byte {
	rom := make([]byte, 8)
	binary.BigEndian.PutUint32(rom[4:8], pc)
	return rom
}

func TestMachineLoad_ScriptPathRunsWithoutMachineReset(t *testing.T) {
	machine := NewMachine(MachineDeps{})
	script := &fakeMachineScript{}
	machine.SetScriptEngine(script)
	machine.SetProgramReset(func(path string) error {
		t.Fatalf("reset called for script path %q", path)
		return nil
	})

	if err := machine.LaunchProgramOrScript("boot.ies"); err != nil {
		t.Fatalf("LaunchProgramOrScript returned error: %v", err)
	}
	if script.runPath != "boot.ies" {
		t.Fatalf("script path = %q, want boot.ies", script.runPath)
	}
}

func TestMachineLoad_MIDIPathDispatchesWithoutMachineReset(t *testing.T) {
	machine := NewMachine(MachineDeps{})
	media := &fakeMachineMedia{}
	machine.SetMediaLoader(media)
	machine.SetProgramReset(func(path string) error {
		t.Fatalf("reset called for MIDI path %q", path)
		return nil
	})

	if err := machine.LaunchProgramOrScript("song.mid"); err != nil {
		t.Fatalf("LaunchProgramOrScript returned error: %v", err)
	}
	if media.path != "song.mid" || media.subsong != 0 {
		t.Fatalf("media dispatch = (%q, %d), want (song.mid, 0)", media.path, media.subsong)
	}
}

func TestMachineLoad_NonScriptMediaDelegatesReset(t *testing.T) {
	machine := NewMachine(MachineDeps{})
	var resetPath string
	machine.SetProgramReset(func(path string) error {
		resetPath = path
		return nil
	})

	if err := machine.LaunchProgramOrScript("demo.ie64"); err != nil {
		t.Fatalf("LaunchProgramOrScript returned error: %v", err)
	}
	if resetPath != "demo.ie64" {
		t.Fatalf("reset path = %q, want demo.ie64", resetPath)
	}
}

func TestMachineLoad_ResetDelegateErrorPropagates(t *testing.T) {
	want := errors.New("reset failed")
	machine := NewMachine(MachineDeps{})
	machine.SetProgramReset(func(path string) error { return want })

	if err := machine.HardReset(); !errors.Is(err, want) {
		t.Fatalf("HardReset error = %v, want %v", err, want)
	}
}

func TestMachineLoad_ROMBootCommandsDelegateToSentinelReset(t *testing.T) {
	machine := NewMachine(MachineDeps{})
	var paths []string
	machine.SetProgramReset(func(path string) error {
		paths = append(paths, path)
		return nil
	})

	if err := machine.BootEmuTOS(); err != nil {
		t.Fatalf("BootEmuTOS returned error: %v", err)
	}
	if err := machine.BootAROS(); err != nil {
		t.Fatalf("BootAROS returned error: %v", err)
	}
	if err := machine.BootIExec(); err != nil {
		t.Fatalf("BootIExec returned error: %v", err)
	}

	want := []string{emutosSentinel, arosSentinel, intuitionOSSentinel}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("boot reset paths = %v, want %v", paths, want)
	}
}

func TestMachineLoad_FailedIE64FlatLoadLeavesRunningMachineUnchanged(t *testing.T) {
	machine := NewMachine(MachineDeps{
		ReadFile: func(name string) ([]byte, error) {
			if name != "too-big.ie64" {
				t.Fatalf("ReadFile path = %q, want too-big.ie64", name)
			}
			return []byte{1, 2, 3, 4, 5}, nil
		},
		ModeFromExtension: func(path string) (string, error) {
			return "ie64", nil
		},
	})
	machine.SetProgramReset(func(path string) error {
		t.Fatalf("reset called for rejected load %q", path)
		return nil
	})

	_, err := machine.ResolveLoad(MachineLoadOptions{
		Path:          "too-big.ie64",
		GuestRAMBytes: 4,
	})
	if err == nil {
		t.Fatalf("ResolveLoad returned nil error for oversized IE64 image")
	}
}

func TestMachineLoad_HardResetResolvesBasicBeforeCommit(t *testing.T) {
	machine := NewMachine(MachineDeps{
		LoadBasicBootImage: func() ([]byte, string, error) {
			return []byte{0xaa, 0xbb}, "basic.ie64", nil
		},
	})

	resolved, err := machine.ResolveLoad(MachineLoadOptions{})
	if err != nil {
		t.Fatalf("ResolveLoad returned error: %v", err)
	}
	if resolved.Mode != "ie64" || !resolved.ForceBasicBoot || resolved.Path != "basic.ie64" {
		t.Fatalf("resolved = %+v, want forced BASIC IE64 boot", resolved)
	}
}

func TestMachineLoad_IntuitionOSSentinelResolvesKernelBootMode(t *testing.T) {
	machine := NewMachine(MachineDeps{
		LoadIntuitionOSImage: func() ([]byte, string, error) {
			return []byte{0x10, 0x20, 0x30, 0x40}, "iexec.ie64", nil
		},
	})

	resolved, err := machine.ResolveLoad(MachineLoadOptions{
		Path:          intuitionOSSentinel,
		GuestRAMBytes: PROG_START + 4,
	})
	if err != nil {
		t.Fatalf("ResolveLoad(IntuitionOS) returned error: %v", err)
	}
	if resolved.Mode != "intuitionos" || resolved.Path != "iexec.ie64" {
		t.Fatalf("resolved = %+v, want IntuitionOS kernel boot mode", resolved)
	}
}

func TestMachineLoad_IntuitionOSSentinelRejectsKernelPastGuestRAM(t *testing.T) {
	machine := NewMachine(MachineDeps{
		LoadIntuitionOSImage: func() ([]byte, string, error) {
			return []byte{0x10, 0x20, 0x30, 0x40}, "iexec.ie64", nil
		},
	})

	_, err := machine.ResolveLoad(MachineLoadOptions{
		Path:          intuitionOSSentinel,
		GuestRAMBytes: PROG_START + 3,
	})
	if err == nil {
		t.Fatalf("ResolveLoad(IntuitionOS) accepted kernel past guest RAM")
	}
}

func TestMachineLoad_6502DefaultLoadAddressSignal(t *testing.T) {
	machine := NewMachine(MachineDeps{
		ReadFile:          func(name string) ([]byte, error) { return []byte{0xea}, nil },
		ModeFromExtension: func(path string) (string, error) { return "6502", nil },
	})

	resolved, err := machine.ResolveLoad(MachineLoadOptions{
		Path:                        "demo.ie65",
		ApplyDefault6502LoadAddress: true,
	})
	if err != nil {
		t.Fatalf("ResolveLoad returned error: %v", err)
	}
	if !resolved.UseDefault6502LoadAddress {
		t.Fatalf("UseDefault6502LoadAddress = false, want true")
	}
}

func TestMachineLoad_EmuTOSBootWiresLoaderAndSymbols(t *testing.T) {
	targets, runner, cleanup := testLifecycleM68KProfileTargets(t)
	defer cleanup()
	targets.GemdosHostRoot = t.TempDir()
	targets.GemdosDriveNum = 21

	var sidecarCPU, sidecarPath string
	machine := NewMachine(MachineDeps{
		LoadELFSymbolSidecar: func(symbols *SymbolTable, cpu, imagePath string) (string, error) {
			if symbols != targets.Monitor.symbols {
				t.Fatalf("LoadELFSymbolSidecar symbols = %p, want monitor symbols %p", symbols, targets.Monitor.symbols)
			}
			sidecarCPU, sidecarPath = cpu, imagePath
			return "emutos.sym", nil
		},
	})

	emutos, aros, err := machine.LoadROMProfile("emutos", testLifecycleROM(0x00e00004), "emutos.elf", targets)
	if err != nil {
		t.Fatalf("LoadROMProfile(emutos) returned error: %v", err)
	}
	defer emutos.Stop()

	if emutos == nil || aros != nil {
		t.Fatalf("LoadROMProfile(emutos) returned emutos=%v aros=%v, want EmuTOS loader only", emutos, aros)
	}
	if emutos.symbols != targets.Monitor.symbols {
		t.Fatalf("EmuTOS symbol table = %p, want monitor symbols %p", emutos.symbols, targets.Monitor.symbols)
	}
	if sidecarCPU != "M68K" || sidecarPath != "emutos.elf" {
		t.Fatalf("sidecar request = (%q, %q), want (M68K, emutos.elf)", sidecarCPU, sidecarPath)
	}
	if runner.cpu.xbiosHandler == nil {
		t.Fatalf("EmuTOS boot did not wire XBIOS interceptor")
	}
	if emutos.gemdos == nil {
		t.Fatalf("EmuTOS boot did not wire GEMDOS host drive")
	}
	if emutos.cancel == nil {
		t.Fatalf("EmuTOS boot did not start loader timers")
	}
}

func TestMachineLoad_AROSBootWiresDOSAudioClipboardAndDiagnostics(t *testing.T) {
	targets, _, cleanup := testLifecycleM68KProfileTargets(t)
	defer cleanup()
	hostRoot := t.TempDir()

	var sidecarCPU, sidecarPath string
	machine := NewMachine(MachineDeps{
		EnsureAROSHostRoot: func() (string, error) {
			return hostRoot, nil
		},
		LoadELFSymbolSidecar: func(symbols *SymbolTable, cpu, imagePath string) (string, error) {
			if symbols != targets.Monitor.symbols {
				t.Fatalf("LoadELFSymbolSidecar symbols = %p, want monitor symbols %p", symbols, targets.Monitor.symbols)
			}
			sidecarCPU, sidecarPath = cpu, imagePath
			return "aros.sym", nil
		},
	})

	emutos, aros, err := machine.LoadROMProfile("aros", testLifecycleROM(0x00600004), "aros.elf", targets)
	if err != nil {
		t.Fatalf("LoadROMProfile(aros) returned error: %v", err)
	}
	defer aros.Stop()

	if emutos != nil || aros == nil {
		t.Fatalf("LoadROMProfile(aros) returned emutos=%v aros=%v, want AROS loader only", emutos, aros)
	}
	if sidecarCPU != "M68K" || sidecarPath != "aros.elf" {
		t.Fatalf("sidecar request = (%q, %q), want (M68K, aros.elf)", sidecarCPU, sidecarPath)
	}
	snap := targets.RuntimeStatus.snapshot()
	if snap.arosDOS == nil || snap.arosDOS.hostRoot != hostRoot {
		t.Fatalf("runtime AROS DOS = %+v, want host root %q", snap.arosDOS, hostRoot)
	}
	if snap.paulaDMA == nil {
		t.Fatalf("runtime Paula DMA was not wired")
	}
	if snap.arosClip == nil {
		t.Fatalf("runtime AROS clipboard bridge was not wired")
	}
	if !targets.SoundChip.HasSampleTicker("default") {
		t.Fatalf("AROS audio DMA was not registered as the sound sample ticker")
	}
	if _, ok := targets.Monitor.devices["aros-audio-dma"]; !ok {
		t.Fatalf("monitor missing AROS audio DMA snapshot device")
	}
	if _, ok := targets.Monitor.devices["clipboard-bridge"]; !ok {
		t.Fatalf("monitor missing clipboard bridge snapshot device")
	}
	if got := mappingCount(targets.Bus, AROS_DOS_REGION_BASE, AROS_DOS_REGION_END); got != 1 {
		t.Fatalf("AROS DOS mapping count = %d, want 1", got)
	}
	if got := mappingCount(targets.Bus, AROS_AUD_REGION_BASE, AROS_AUD_REGION_END); got != 1 {
		t.Fatalf("AROS audio mapping count = %d, want 1", got)
	}
	if got := mappingCount(targets.Bus, CLIP_REGION_BASE, CLIP_REGION_END); got != 1 {
		t.Fatalf("AROS clipboard mapping count = %d, want 1", got)
	}
	if got := mappingCount(targets.Bus, IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END); got != 1 {
		t.Fatalf("AROS IRQ diagnostics mapping count = %d, want 1", got)
	}
	if !targets.TermMMIO.amigaScancodeMode.Load() {
		t.Fatalf("AROS boot did not enable Amiga scancode mode")
	}
	if !targets.TermMMIO.mouseNativeLocked.Load() ||
		targets.TermMMIO.mouseNativeW.Load() != DefaultPresentationWidth ||
		targets.TermMMIO.mouseNativeH.Load() != DefaultPresentationHeight {
		t.Fatalf("AROS boot did not lock terminal mouse resolution to presentation size")
	}
	if aros.cancel == nil {
		t.Fatalf("AROS boot did not start loader timers")
	}
}

func TestMachineReset_OrderStopsProducersBeforeBusAndDeviceReset(t *testing.T) {
	order := &orderedCalls{}
	script := &fakeMachineScript{}
	machine := NewMachine(MachineDeps{})
	machine.QuiesceBeforeReset(MachineQuiesceTargets{
		Script:     script,
		Compositor: fakeMachineCompositor{order: order},
		RenderLoops: []MachineRenderLoop{
			fakeMachineRenderLoop{name: "vga", order: order},
			fakeMachineRenderLoop{name: "ula", order: order},
		},
		Monitor: fakeMachineMonitor{order: order, active: true},
		Media:   fakeMachineMediaPlayers{order: order},
		CPU:     fakeMachineCPU{order: order},
		EmuTOS:  fakeMachineROMLoader{name: "emutos", order: order},
		AROS:    fakeMachineROMLoader{name: "aros", order: order},
	})
	order.add("bus.reset")
	order.add("device.reset")

	wantPrefix := []string{
		"compositor.stop",
		"vga.stop",
		"ula.stop",
		"monitor.deactivate",
		"media.stopPlayers",
		"cpu.stop",
		"emutos.stop",
		"aros.stop",
		"compositor.stop",
		"vga.stop",
		"ula.stop",
	}
	if len(order.calls) < len(wantPrefix) || !reflect.DeepEqual(order.calls[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("quiesce order prefix = %v, want %v", order.calls, wantPrefix)
	}
	if order.calls[len(wantPrefix)] != "bus.reset" || order.calls[len(wantPrefix)+1] != "device.reset" {
		t.Fatalf("reset order suffix = %v, want bus/device after quiesce", order.calls[len(wantPrefix):])
	}
}

func TestMachineReset_DeviceResetSequenceBeforeProgramLoad(t *testing.T) {
	order := &orderedCalls{}
	machine := NewMachine(MachineDeps{})

	err := machine.ResetDevicesBeforeLoad("ie64", MachineDeviceResetTargets{
		AudioDevices: []MachineResetDevice{
			fakeMachineResetDevice{name: "psg", order: order},
			fakeMachineResetDevice{name: "sid", order: order},
		},
		Memory: fakeMachineResetDevice{name: "bus", order: order},
		ApplyRuntimeVisibleRAM: func(mode string) {
			if mode != "ie64" {
				t.Fatalf("mode = %q, want ie64", mode)
			}
			order.add("ram.visible")
		},
		PrepareVideoBeforeReset: func() error {
			order.add("video.prepare")
			return nil
		},
		VideoChip: fakeMachineResetDevice{name: "videoChip", order: order},
		ApplyVideoConfigAfterReset: func() error {
			order.add("video.config")
			return nil
		},
		VideoDevices: []MachineResetDevice{
			fakeMachineResetDevice{name: "vga", order: order},
			fakeMachineResetDevice{name: "ula", order: order},
		},
		ForceBasicBoot: true,
		ConfigureBasicTerminal: func() {
			order.add("terminal.basic")
		},
		ResetActiveTerminal: func() {
			order.add("terminal.reset")
		},
		Coprocessor: fakeMachineResetDevice{name: "coproc", order: order},
	})
	if err != nil {
		t.Fatalf("ResetDevicesBeforeLoad returned error: %v", err)
	}

	want := []string{
		"psg.reset",
		"sid.reset",
		"bus.reset",
		"ram.visible",
		"video.prepare",
		"videoChip.reset",
		"video.config",
		"vga.reset",
		"ula.reset",
		"terminal.basic",
		"terminal.reset",
		"coproc.reset",
	}
	if !reflect.DeepEqual(order.calls, want) {
		t.Fatalf("device reset order = %v, want %v", order.calls, want)
	}
}

func TestMachineReset_UnsealsBusBeforeROMProfileRemapping(t *testing.T) {
	bus := NewMachineBus()
	bus.SealMappings()

	machine := NewMachine(MachineDeps{})
	err := machine.ResetDevicesBeforeLoad("aros", MachineDeviceResetTargets{
		Memory: bus,
		ApplyRuntimeVisibleRAM: func(mode string) {
			if mode != "aros" {
				t.Fatalf("mode = %q, want aros", mode)
			}
			bus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, func(addr uint32) uint32 { return 0 }, func(addr uint32, value uint32) {})
		},
	})
	if err != nil {
		t.Fatalf("ResetDevicesBeforeLoad returned error: %v", err)
	}
}

func TestMachineReset_PreservesSelectedJITForSameCPUFamily(t *testing.T) {
	bus := NewMachineBus()
	oldCPU := NewCPU64(bus)
	oldCPU.jitEnabled = false
	status := &runtimeStatusStore{}
	status.setCPUs(runtimeCPUIE64, nil, oldCPU, nil, nil, nil, nil)

	machine := NewMachine(MachineDeps{})
	state := machine.CaptureCPUResetState("ie64", status, bus, nil)
	newCPU := NewCPU64(bus)
	newCPU.jitEnabled = true
	machine.ApplyCPUResetState("ie64", newCPU, state)

	if newCPU.jitEnabled {
		t.Fatalf("new IE64 jitEnabled = true, want preserved false")
	}
}

func TestMachineReset_ReregistersMonitorCPUAfterRunnerRecreate(t *testing.T) {
	bus := NewMachineBus()
	monitor := NewMachineMonitor(bus)
	oldCPU := NewCPU64(bus)
	monitor.RegisterCPU("IE64", NewDebugIE64(oldCPU))

	machine := NewMachine(MachineDeps{})
	newCPU := NewCPU64(bus)
	machine.ReregisterMonitorCPU("ie64", newCPU, monitor)

	focused := monitor.FocusedCPU()
	if focused == nil {
		t.Fatalf("monitor has no focused CPU after re-register")
	}
	if focused.Label != "IE64" {
		t.Fatalf("focused label = %q, want IE64", focused.Label)
	}
	if len(monitor.cpus) != 1 {
		t.Fatalf("registered CPU count = %d, want 1", len(monitor.cpus))
	}
}

func TestMachineReset_StartsPeripheralsBeforeCPU(t *testing.T) {
	order := &orderedCalls{}
	machine := NewMachine(MachineDeps{})
	machine.StartAfterReset(MachineStartTargets{
		VideoChip:  fakeMachineVideoStarter{order: order},
		SoundChip:  fakeMachineSoundStarter{order: order},
		Compositor: fakeMachineCompositor{order: order},
		RenderLoops: []MachineRenderLoop{
			fakeMachineRenderLoop{name: "vga", order: order},
			fakeMachineRenderLoop{name: "ula", order: order},
		},
		CPU: fakeMachineCPU{order: order},
	})

	want := []string{
		"video.start",
		"sound.start",
		"compositor.start",
		"vga.start",
		"ula.start",
		"cpu.start",
	}
	if !reflect.DeepEqual(order.calls, want) {
		t.Fatalf("start order = %v, want %v", order.calls, want)
	}
}

func TestMachineReset_IgnoresTypedNilRenderLoops(t *testing.T) {
	var typedNil *nilSensitiveRenderLoop
	machine := NewMachine(MachineDeps{})

	machine.QuiesceBeforeReset(MachineQuiesceTargets{
		RenderLoops: []MachineRenderLoop{typedNil},
	})
	machine.StartAfterReset(MachineStartTargets{
		RenderLoops: []MachineRenderLoop{typedNil},
	})
}

func TestMachineReset_IgnoresTypedNilROMLoaders(t *testing.T) {
	var emutos *nilSensitiveROMLoader
	var aros *nilSensitiveROMLoader
	machine := NewMachine(MachineDeps{})

	machine.QuiesceBeforeReset(MachineQuiesceTargets{
		EmuTOS: emutos,
		AROS:   aros,
	})
}
