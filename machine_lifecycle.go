package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

type MachineScriptEngine interface {
	Cancel()
	IsLoadingProgram() bool
	RunFile(path string) error
}

type MachineMediaLoader interface {
	PlayHostPath(path string, subsong uint32) error
}

type MachineDebugMonitor interface {
	Deactivate()
	IsActive() bool
	RegisterCPU(label string, cpu DebuggableCPU) int
	RegisterSnapshotDevice(dev DebugSnapshotDevice)
	ResetCPUs()
}

type MachineRuntimeStatus interface {
	snapshot() runtimeStatusSnapshot
	setAROSClipboard(cb *ClipboardBridge)
	setAROSDOS(dos *ArosDOSDevice)
	setCPUs(selectedCPU int, ie32 *CPU, ie64 *CPU64, m68k *M68KRunner, z80 *CPUZ80Runner, x86 *CPUX86Runner, cpu65 *CPU6502Runner)
	setPaulaDMA(dma *ArosAudioDMA)
}

type MachineProgramExecutor interface {
	SetAROSBootLoader(fn func() error)
	SetCPU(cpu *CPU64)
	SetEmuTOSBootLoader(fn func() error)
	SetExternalLauncher(fn func(path string) error)
	SetHardReset(fn func() error)
	SetIExecBootLoader(fn func() error)
}

type MachineRenderLoop interface {
	StartRenderLoop()
	StopRenderLoop()
}

type MachineCompositor interface {
	Stop()
}

type MachineMediaPlayers interface {
	stopPlayersOnly()
}

type MachineROMLoader interface {
	Stop()
}

type MachineResetDevice interface {
	Reset()
}

var (
	_ MachineScriptEngine    = (*ScriptEngine)(nil)
	_ MachineMediaLoader     = (*MediaLoader)(nil)
	_ MachineMediaPlayers    = (*MediaLoader)(nil)
	_ MachineDebugMonitor    = (*MachineMonitor)(nil)
	_ MachineRuntimeStatus   = (*runtimeStatusStore)(nil)
	_ MachineProgramExecutor = (*ProgramExecutor)(nil)
)

type MachineDeps struct {
	ReadFile                     func(name string) ([]byte, error)
	ModeFromExtension            func(path string) (string, error)
	LoadBasicBootImage           func() ([]byte, string, error)
	LoadIntuitionOSImage         func() ([]byte, string, error)
	LoadEmuTOSImage              func() ([]byte, string, error)
	LoadAROSImage                func() ([]byte, string, error)
	NewEmuTOSLoader              func(bus *MachineBus, cpu *M68KCPU, videoChip *VideoChip) *EmuTOSLoader
	NewAROSLoader                func(bus *MachineBus, cpu *M68KCPU, videoChip *VideoChip) *AROSLoader
	NewArosDOSDevice             func(bus *MachineBus, hostRoot string) (*ArosDOSDevice, error)
	NewArosAudioDMA              func(bus *MachineBus, soundChip *SoundChip, cpu *M68KCPU) (*ArosAudioDMA, error)
	NewClipboardBridge           func(bus *MachineBus) *ClipboardBridge
	LoadELFSymbolSidecar         func(symbols *SymbolTable, cpu, imagePath string) (string, error)
	EnsureAROSHostRoot           func() (string, error)
	StageConfiguredCoprocService func()
	Quit                         func()
	Exit                         func(code int)
}

func (d MachineDeps) withDefaults() MachineDeps {
	if d.ReadFile == nil {
		d.ReadFile = os.ReadFile
	}
	if d.ModeFromExtension == nil {
		d.ModeFromExtension = modeFromExtension
	}
	if d.LoadBasicBootImage == nil {
		d.LoadBasicBootImage = func() ([]byte, string, error) {
			return nil, "", fmt.Errorf("BASIC boot image resolver unavailable")
		}
	}
	if d.LoadIntuitionOSImage == nil {
		d.LoadIntuitionOSImage = func() ([]byte, string, error) {
			return nil, "", fmt.Errorf("IntuitionOS image resolver unavailable")
		}
	}
	if d.LoadEmuTOSImage == nil {
		d.LoadEmuTOSImage = func() ([]byte, string, error) {
			return nil, "", fmt.Errorf("EmuTOS image resolver unavailable")
		}
	}
	if d.LoadAROSImage == nil {
		d.LoadAROSImage = func() ([]byte, string, error) {
			return nil, "", fmt.Errorf("AROS image resolver unavailable")
		}
	}
	if d.NewEmuTOSLoader == nil {
		d.NewEmuTOSLoader = NewEmuTOSLoader
	}
	if d.NewAROSLoader == nil {
		d.NewAROSLoader = NewAROSLoader
	}
	if d.NewArosDOSDevice == nil {
		d.NewArosDOSDevice = NewArosDOSDevice
	}
	if d.NewArosAudioDMA == nil {
		d.NewArosAudioDMA = NewArosAudioDMA
	}
	if d.NewClipboardBridge == nil {
		d.NewClipboardBridge = NewClipboardBridge
	}
	if d.LoadELFSymbolSidecar == nil {
		d.LoadELFSymbolSidecar = loadELFSymbolSidecar
	}
	if d.EnsureAROSHostRoot == nil {
		d.EnsureAROSHostRoot = func() (string, error) {
			return "", fmt.Errorf("AROS host root resolver unavailable")
		}
	}
	if d.Quit == nil {
		d.Quit = func() { os.Exit(0) }
	}
	if d.Exit == nil {
		d.Exit = os.Exit
	}
	return d
}

type Machine struct {
	deps MachineDeps

	script MachineScriptEngine
	media  MachineMediaLoader

	resetProgram func(path string) error
}

func NewMachine(deps MachineDeps) *Machine {
	return &Machine{deps: deps.withDefaults()}
}

type MachineLoadOptions struct {
	Path                        string
	AB3D2DefaultBoot            bool
	InitialEmuTOSMode           bool
	GuestRAMBytes               int
	ApplyDefault6502LoadAddress bool
}

type MachineLoadResolution struct {
	Bytes                     []byte
	Path                      string
	Mode                      string
	ForceBasicBoot            bool
	DispatchedWithoutReset    bool
	UseDefault6502LoadAddress bool
}

type MachineQuiesceTargets struct {
	Script      MachineScriptEngine
	Compositor  MachineCompositor
	RenderLoops []MachineRenderLoop
	Monitor     MachineDebugMonitor
	Media       MachineMediaPlayers
	CPU         EmulatorCPU
	EmuTOS      MachineROMLoader
	AROS        MachineROMLoader
}

type MachineVideoStarter interface {
	Start() error
}

type MachineSoundStarter interface {
	Start()
}

type MachineStartTargets struct {
	VideoChip   MachineVideoStarter
	SoundChip   MachineSoundStarter
	Compositor  MachineCompositorStarter
	RenderLoops []MachineRenderLoop
	CPU         EmulatorCPU
}

type MachineCompositorStarter interface {
	Start() error
}

type MachineProfileLoadTargets struct {
	Bus            *MachineBus
	Runner         EmulatorCPU
	VideoChip      *VideoChip
	PSGEngine      *PSGEngine
	Monitor        *MachineMonitor
	RuntimeStatus  *runtimeStatusStore
	SoundChip      *SoundChip
	TermMMIO       *TerminalMMIO
	GemdosHostRoot string
	GemdosDriveNum uint16
}

type MachineDeviceResetTargets struct {
	AudioDevices               []MachineResetDevice
	Memory                     MachineResetDevice
	ApplyRuntimeVisibleRAM     func(mode string)
	PrepareVideoBeforeReset    func() error
	VideoChip                  MachineResetDevice
	ApplyVideoConfigAfterReset func() error
	VideoDevices               []MachineResetDevice
	Terminal                   *TerminalMMIO
	ForceBasicBoot             bool
	ConfigureBasicTerminal     func()
	ResetActiveTerminal        func()
	Coprocessor                MachineResetDevice
}

type MachineCPUResetState struct {
	PreserveM68KJIT bool
	HaveM68KJIT     bool
	PreserveZ80JIT  bool
	HaveZ80JIT      bool
	Preserve6502JIT bool
	Have6502JIT     bool
	PreserveIE64JIT bool
	HaveIE64JIT     bool
}

func (m *Machine) SetScriptEngine(script MachineScriptEngine) {
	m.script = script
}

func (m *Machine) SetMediaLoader(media MachineMediaLoader) {
	m.media = media
}

func (m *Machine) SetProgramReset(fn func(path string) error) {
	m.resetProgram = fn
}

func (m *Machine) ResolveLoad(opts MachineLoadOptions) (MachineLoadResolution, error) {
	var resolved MachineLoadResolution
	path := opts.Path

	ie64FlatResolved := false
	if path == intuitionOSSentinel {
		bytes, imagePath, err := m.deps.LoadIntuitionOSImage()
		if err != nil {
			return MachineLoadResolution{}, err
		}
		if !flatProgramFitsRAM(opts.GuestRAMBytes, len(bytes)) {
			return MachineLoadResolution{}, fmt.Errorf("IntuitionOS kernel too large: %d bytes exceeds guest RAM", len(bytes))
		}
		resolved.Bytes = bytes
		resolved.Path = imagePath
		resolved.Mode = "intuitionos"
		return resolved, nil
	} else if path != "" && path != emutosSentinel && path != arosSentinel {
		if mode, err := m.deps.ModeFromExtension(path); err == nil && mode == "ie64" {
			bytes, readErr := m.deps.ReadFile(path)
			if readErr != nil {
				return MachineLoadResolution{}, readErr
			}
			resolved.Bytes = bytes
			resolved.Path = path
			resolved.Mode = "ie64"
			ie64FlatResolved = true
		}
	}

	if ie64FlatResolved {
		if !flatProgramFitsRAM(opts.GuestRAMBytes, len(resolved.Bytes)) {
			return MachineLoadResolution{}, fmt.Errorf("IE64 program too large: %d bytes exceeds guest RAM", len(resolved.Bytes))
		}
		return resolved, nil
	}

	if path == "" {
		if opts.AB3D2DefaultBoot {
			resolved.Bytes = append([]byte(nil), embeddedAB3D2Image...)
			resolved.Mode = "m68k"
			return resolved, nil
		}
		if opts.InitialEmuTOSMode {
			bytes, imagePath, err := m.deps.LoadEmuTOSImage()
			if err != nil {
				return MachineLoadResolution{}, err
			}
			resolved.Bytes = bytes
			resolved.Path = imagePath
			resolved.Mode = "emutos"
			return resolved, nil
		}
		bytes, imagePath, err := m.deps.LoadBasicBootImage()
		if err != nil {
			return MachineLoadResolution{}, err
		}
		resolved.Bytes = bytes
		resolved.Path = imagePath
		resolved.Mode = "ie64"
		resolved.ForceBasicBoot = true
		return resolved, nil
	}

	if path == emutosSentinel {
		bytes, imagePath, err := m.deps.LoadEmuTOSImage()
		if err != nil {
			return MachineLoadResolution{}, err
		}
		resolved.Bytes = bytes
		resolved.Path = imagePath
		resolved.Mode = "emutos"
		return resolved, nil
	}
	if path == arosSentinel {
		bytes, imagePath, err := m.deps.LoadAROSImage()
		if err != nil {
			return MachineLoadResolution{}, err
		}
		resolved.Bytes = bytes
		resolved.Path = imagePath
		resolved.Mode = "aros"
		return resolved, nil
	}

	bytes, err := m.deps.ReadFile(path)
	if err != nil {
		return MachineLoadResolution{}, err
	}
	mode, err := m.deps.ModeFromExtension(path)
	if err != nil {
		return MachineLoadResolution{}, err
	}
	if mode == "script" {
		if m.script == nil {
			return MachineLoadResolution{}, fmt.Errorf("script engine unavailable")
		}
		if err := m.script.RunFile(path); err != nil {
			return MachineLoadResolution{}, err
		}
		return MachineLoadResolution{DispatchedWithoutReset: true}, nil
	}
	if mode == "midi" {
		if m.media == nil {
			return MachineLoadResolution{}, fmt.Errorf("media loader unavailable")
		}
		if err := m.media.PlayHostPath(path, 0); err != nil {
			return MachineLoadResolution{}, err
		}
		return MachineLoadResolution{DispatchedWithoutReset: true}, nil
	}
	resolved.Bytes = bytes
	resolved.Path = path
	resolved.Mode = mode
	resolved.UseDefault6502LoadAddress = mode == "6502" && opts.ApplyDefault6502LoadAddress
	return resolved, nil
}

func (m *Machine) QuiesceBeforeReset(t MachineQuiesceTargets) {
	if !isNilLifecycleInterface(t.Script) && !t.Script.IsLoadingProgram() {
		t.Script.Cancel()
	}
	if !isNilLifecycleInterface(t.Compositor) {
		t.Compositor.Stop()
	}
	stopRenderLoops(t.RenderLoops)
	if !isNilLifecycleInterface(t.Monitor) && t.Monitor.IsActive() {
		t.Monitor.Deactivate()
	}
	if !isNilLifecycleInterface(t.Media) {
		t.Media.stopPlayersOnly()
	}
	if !isNilLifecycleInterface(t.CPU) {
		t.CPU.Stop()
	}
	if !isNilLifecycleInterface(t.EmuTOS) {
		t.EmuTOS.Stop()
	}
	if !isNilLifecycleInterface(t.AROS) {
		t.AROS.Stop()
	}
	if !isNilLifecycleInterface(t.Compositor) {
		t.Compositor.Stop()
	}
	stopRenderLoops(t.RenderLoops)
}

func (m *Machine) StartAfterReset(t MachineStartTargets) {
	if !isNilLifecycleInterface(t.VideoChip) {
		_ = t.VideoChip.Start()
	}
	if !isNilLifecycleInterface(t.SoundChip) {
		t.SoundChip.Start()
	}
	if !isNilLifecycleInterface(t.Compositor) {
		_ = t.Compositor.Start()
	}
	for _, loop := range t.RenderLoops {
		if !isNilLifecycleInterface(loop) {
			loop.StartRenderLoop()
		}
	}
	if !isNilLifecycleInterface(t.CPU) {
		t.CPU.StartExecution()
	}
}

func (m *Machine) ResetDevicesBeforeLoad(mode string, t MachineDeviceResetTargets) error {
	resetDevices(t.AudioDevices)
	if t.Memory != nil {
		if unsealer, ok := t.Memory.(interface{ UnsealMappings() }); ok {
			unsealer.UnsealMappings()
		}
		t.Memory.Reset()
	}
	if t.ApplyRuntimeVisibleRAM != nil {
		t.ApplyRuntimeVisibleRAM(mode)
	}
	if t.PrepareVideoBeforeReset != nil {
		if err := t.PrepareVideoBeforeReset(); err != nil {
			return err
		}
	}
	if t.VideoChip != nil {
		t.VideoChip.Reset()
	}
	if t.ApplyVideoConfigAfterReset != nil {
		if err := t.ApplyVideoConfigAfterReset(); err != nil {
			return err
		}
	}
	resetDevices(t.VideoDevices)
	if t.Terminal != nil {
		t.Terminal.Reset()
		t.Terminal.amigaScancodeMode.Store(false)
		t.Terminal.UnlockMouseNativeResolution()
		t.Terminal.SetMouseNativeResolution(DefaultScreenWidth, DefaultScreenHeight)
	}
	if t.ForceBasicBoot && t.ConfigureBasicTerminal != nil {
		t.ConfigureBasicTerminal()
	}
	if t.ResetActiveTerminal != nil {
		t.ResetActiveTerminal()
	}
	if t.Coprocessor != nil {
		t.Coprocessor.Reset()
	}
	return nil
}

func resetDevices(devices []MachineResetDevice) {
	for _, dev := range devices {
		if !isNilLifecycleInterface(dev) {
			dev.Reset()
		}
	}
}

func stopRenderLoops(loops []MachineRenderLoop) {
	for _, loop := range loops {
		if !isNilLifecycleInterface(loop) {
			loop.StopRenderLoop()
		}
	}
}

func isNilLifecycleInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func (m *Machine) CaptureCPUResetState(currentMode string, status MachineRuntimeStatus, bus *MachineBus, sound *SoundChip) MachineCPUResetState {
	snap := status.snapshot()
	if currentMode == "aros" {
		arosTeardownAll(snap, bus, sound)
	}
	var state MachineCPUResetState
	switch snap.selectedCPU {
	case runtimeCPUM68K:
		if snap.m68k != nil && snap.m68k.cpu != nil {
			state.PreserveM68KJIT = snap.m68k.cpu.m68kJitEnabled
			state.HaveM68KJIT = true
		}
	case runtimeCPUZ80:
		if snap.z80 != nil && snap.z80.cpu != nil {
			state.PreserveZ80JIT = snap.z80.cpu.jitEnabled
			state.HaveZ80JIT = true
		}
	case runtimeCPU6502:
		if snap.cpu65 != nil && snap.cpu65.cpu != nil {
			state.Preserve6502JIT = snap.cpu65.JITEnabled
			state.Have6502JIT = true
		}
	case runtimeCPUIE64:
		if snap.ie64 != nil {
			state.PreserveIE64JIT = snap.ie64.jitEnabled
			state.HaveIE64JIT = true
		}
	}
	return state
}

func (m *Machine) ApplyCPUResetState(mode string, runner EmulatorCPU, state MachineCPUResetState) {
	switch mode {
	case "ie64", "intuitionos":
		if state.HaveIE64JIT {
			runner.(*CPU64).jitEnabled = state.PreserveIE64JIT
		}
	case "m68k", "emutos", "aros":
		if state.HaveM68KJIT {
			runner.(*M68KRunner).cpu.m68kJitEnabled = state.PreserveM68KJIT
		}
	case "z80":
		if state.HaveZ80JIT {
			runner.(*CPUZ80Runner).cpu.jitEnabled = state.PreserveZ80JIT
		}
	case "6502":
		if state.Have6502JIT {
			r := runner.(*CPU6502Runner)
			r.JITEnabled = state.Preserve6502JIT
			r.cpu.jitEnabled = state.Preserve6502JIT
		}
	}
}

func (m *Machine) UpdateRuntimeCPU(mode string, runner EmulatorCPU, status MachineRuntimeStatus, exec MachineProgramExecutor) {
	switch mode {
	case "ie32":
		status.setCPUs(runtimeCPUIE32, runner.(*CPU), nil, nil, nil, nil, nil)
		exec.SetCPU(nil)
	case "ie64", "intuitionos":
		cpu64 := runner.(*CPU64)
		status.setCPUs(runtimeCPUIE64, nil, cpu64, nil, nil, nil, nil)
		exec.SetCPU(cpu64)
	case "m68k":
		status.setCPUs(runtimeCPUM68K, nil, nil, runner.(*M68KRunner), nil, nil, nil)
		exec.SetCPU(nil)
	case "emutos", "aros":
		status.setCPUs(runtimeCPUM68K, nil, nil, runner.(*M68KRunner), nil, nil, nil)
		exec.SetCPU(nil)
	case "z80":
		status.setCPUs(runtimeCPUZ80, nil, nil, nil, runner.(*CPUZ80Runner), nil, nil)
		exec.SetCPU(nil)
	case "x86":
		status.setCPUs(runtimeCPUX86, nil, nil, nil, nil, runner.(*CPUX86Runner), nil)
		exec.SetCPU(nil)
	case "6502":
		status.setCPUs(runtimeCPU6502, nil, nil, nil, nil, nil, runner.(*CPU6502Runner))
		exec.SetCPU(nil)
	}
}

func (m *Machine) ReregisterMonitorCPU(mode string, runner EmulatorCPU, monitor MachineDebugMonitor) {
	monitor.ResetCPUs()
	switch mode {
	case "ie32":
		monitor.RegisterCPU("IE32", NewDebugIE32(runner.(*CPU)))
	case "ie64", "intuitionos":
		monitor.RegisterCPU("IE64", NewDebugIE64(runner.(*CPU64)))
	case "m68k", "emutos", "aros":
		r := runner.(*M68KRunner)
		monitor.RegisterCPU("M68K", NewDebugM68K(r.cpu, r))
	case "z80":
		r := runner.(*CPUZ80Runner)
		monitor.RegisterCPU("Z80", NewDebugZ80(r.cpu, r))
	case "x86":
		r := runner.(*CPUX86Runner)
		monitor.RegisterCPU("X86", NewDebugX86(r.cpu, r))
	case "6502":
		r := runner.(*CPU6502Runner)
		monitor.RegisterCPU("6502", NewDebug6502(r.cpu, r))
	}
}

func (m *Machine) LoadROMProfile(mode string, bytes []byte, path string, t MachineProfileLoadTargets) (*EmuTOSLoader, *AROSLoader, error) {
	switch mode {
	case "emutos":
		r := t.Runner.(*M68KRunner)
		if disabler, ok := t.VideoChip.GetOutput().(SoftwareCursorDisabler); ok {
			disabler.DisableSoftwareCursor()
		}
		loader := m.deps.NewEmuTOSLoader(t.Bus, r.cpu, t.VideoChip)
		r.cpu.xbiosHandler = NewXBIOSInterceptor(r.cpu, t.Bus, t.VideoChip, t.PSGEngine)
		if err := loader.LoadROM(bytes); err != nil {
			return nil, nil, fmt.Errorf("failed to load EmuTOS ROM: %w", err)
		}
		if sidecar, err := m.deps.LoadELFSymbolSidecar(t.Monitor.symbols, "M68K", path); err != nil {
			fmt.Printf("Warning: EmuTOS ELF symbols disabled: %v\n", err)
		} else if sidecar != "" {
			fmt.Printf("IEMon: loaded EmuTOS symbols from %s\r\n", sidecar)
		}
		loader.SetSymbolTable(t.Monitor.symbols)
		if t.GemdosHostRoot != "" {
			if err := loader.SetupGemdos(t.GemdosHostRoot, t.GemdosDriveNum); err != nil {
				fmt.Printf("Warning: GEMDOS drive U: disabled: %v\n", err)
			}
		}
		loader.StartTimer()
		runtime.GC()
		return loader, nil, nil
	case "aros":
		r := t.Runner.(*M68KRunner)
		loader := m.deps.NewAROSLoader(t.Bus, r.cpu, t.VideoChip)
		if err := loader.LoadROM(bytes); err != nil {
			return nil, nil, fmt.Errorf("failed to load AROS ROM: %w", err)
		}
		if sidecar, err := m.deps.LoadELFSymbolSidecar(t.Monitor.symbols, "M68K", path); err != nil {
			fmt.Printf("Warning: AROS ELF symbols disabled: %v\n", err)
		} else if sidecar != "" {
			fmt.Printf("IEMon: loaded AROS symbols from %s\r\n", sidecar)
		}
		hostRoot, err := m.deps.EnsureAROSHostRoot()
		if err != nil {
			return nil, nil, err
		}
		arosDOS, dosErr := m.deps.NewArosDOSDevice(t.Bus, hostRoot)
		if dosErr != nil {
			return nil, nil, fmt.Errorf("AROS DOS device init failed: %w", dosErr)
		}
		arosDOS.SetSymbolTable(t.Monitor.symbols)
		t.Bus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, arosDOS.HandleRead, arosDOS.HandleWrite)
		t.RuntimeStatus.setAROSDOS(arosDOS)
		fmt.Printf("AROS DOS: IE: → %s\r\n", hostRoot)

		arosSockets := NewArosHostSocketDevice(t.Bus, NewUnixArosHostSocketBackend(), true)
		t.Bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, arosSockets.HandleRead, arosSockets.HandleWrite)

		arosDMA, dmaErr := m.deps.NewArosAudioDMA(t.Bus, t.SoundChip, r.cpu)
		if dmaErr != nil {
			return nil, nil, fmt.Errorf("create AROS audio DMA: %w", dmaErr)
		}
		t.Bus.UnmapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END)
		t.Bus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, arosDMA.HandleRead, arosDMA.HandleWrite)
		t.SoundChip.SetSampleTicker(arosDMA)
		t.RuntimeStatus.setPaulaDMA(arosDMA)
		t.Monitor.RegisterSnapshotDevice(arosDMA)

		clipBridge := m.deps.NewClipboardBridge(t.Bus)
		t.Bus.MapIO(CLIP_REGION_BASE, CLIP_REGION_END, clipBridge.HandleRead, clipBridge.HandleWrite)
		t.RuntimeStatus.setAROSClipboard(clipBridge)
		t.Monitor.RegisterSnapshotDevice(clipBridge)

		loader.MapIRQDiagnostics()
		t.TermMMIO.amigaScancodeMode.Store(true)
		t.TermMMIO.LockMouseNativeResolution(DefaultPresentationWidth, DefaultPresentationHeight)
		loader.StartTimer()
		runtime.GC()
		return nil, loader, nil
	default:
		return nil, nil, fmt.Errorf("unsupported ROM profile mode: %s", mode)
	}
}

func (m *Machine) LoadProgram(path string) error {
	if m.resetProgram == nil {
		return fmt.Errorf("machine reset loader unavailable")
	}
	return m.resetProgram(path)
}

func (m *Machine) HardReset() error {
	return m.LoadProgram("")
}

func (m *Machine) BootEmuTOS() error {
	return m.LoadProgram(emutosSentinel)
}

func (m *Machine) BootAROS() error {
	return m.LoadProgram(arosSentinel)
}

func (m *Machine) BootIExec() error {
	return m.LoadProgram(intuitionOSSentinel)
}

func (m *Machine) LaunchProgramOrScript(path string) error {
	if strings.EqualFold(filepath.Ext(path), ".ies") {
		if m.script == nil {
			return fmt.Errorf("script engine unavailable")
		}
		return m.script.RunFile(path)
	}
	if detectMediaType(path) != MEDIA_TYPE_NONE {
		if m.media == nil {
			return fmt.Errorf("media loader unavailable")
		}
		return m.media.PlayHostPath(path, 0)
	}
	return m.LoadProgram(path)
}
