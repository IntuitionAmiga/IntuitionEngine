package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type ProgramExecutor struct {
	bus          *MachineBus
	ie64CPU      *CPU64
	videoChip    *VideoChip
	vgaEngine    *VGAEngine
	voodooEngine *VoodooEngine
	baseDir      string
	emuTOSLoader *EmuTOSLoader
	// launchExternal delegates full reset+launch orchestration to main when set.
	launchExternal func(path string) error
	// loadEmuTOS boots EmuTOS without a filename (ROM is resolved by main).
	loadEmuTOS func() error
	// loadAROS boots AROS without a filename (ROM is resolved by main).
	loadAROS func() error
	// loadIExec boots the IExec microkernel (ROM is resolved by main).
	loadIExec func() error

	// GEMDOS drive mapping for EmuTOS mode
	gemdosHostRoot string
	gemdosDriveNum uint16

	namePtr uint32
	status  uint32
	typ     uint32
	errCode uint32
	session uint32

	mu sync.Mutex
}

func NewProgramExecutor(bus *MachineBus, ie64CPU *CPU64, videoChip *VideoChip, vgaEngine *VGAEngine, voodooEngine *VoodooEngine, baseDir string) *ProgramExecutor {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = baseDir
	}
	return &ProgramExecutor{
		bus:          bus,
		ie64CPU:      ie64CPU,
		videoChip:    videoChip,
		vgaEngine:    vgaEngine,
		voodooEngine: voodooEngine,
		baseDir:      absBase,
		status:       EXEC_STATUS_IDLE,
		typ:          EXEC_TYPE_NONE,
		errCode:      EXEC_ERR_OK,
	}
}

// SetCPU updates the IE64 CPU pointer. Called during mode switches so EXEC
// MMIO targets the correct CPU instance.
func (e *ProgramExecutor) SetCPU(cpu *CPU64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ie64CPU = cpu
}

// SetEmuTOSBootLoader configures a callback that boots EmuTOS from the
// embedded ROM, -emutos-image flag, or local .img file. Called when BASIC
// writes EXEC_OP_EMUTOS to EXEC_CTRL.
func (e *ProgramExecutor) SetEmuTOSBootLoader(fn func() error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loadEmuTOS = fn
}

// SetAROSBootLoader configures a callback that boots AROS from the
// embedded ROM, -aros-image flag, or local ROM file. Called when BASIC
// writes EXEC_OP_AROS to EXEC_CTRL.
func (e *ProgramExecutor) SetAROSBootLoader(fn func() error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loadAROS = fn
}

// SetGemdosConfig sets the GEMDOS drive mapping for EmuTOS mode reloads.
func (e *ProgramExecutor) SetGemdosConfig(hostRoot string, driveNum uint16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gemdosHostRoot = hostRoot
	e.gemdosDriveNum = driveNum
}

// SetExternalLauncher configures an optional shared launcher callback.
// When set, ProgramExecutor delegates RUN "file" handoff to this callback so
// monitor/runtime reset behavior is consistent with other launch paths.
func (e *ProgramExecutor) SetExternalLauncher(fn func(path string) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.launchExternal = fn
}

func (e *ProgramExecutor) HandleRead(addr uint32) uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch addr {
	case EXEC_NAME_PTR:
		return e.namePtr
	case EXEC_STATUS:
		return e.status
	case EXEC_TYPE:
		return e.typ
	case EXEC_ERROR:
		return e.errCode
	case EXEC_SESSION:
		return e.session
	default:
		return 0
	}
}

func (e *ProgramExecutor) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case EXEC_NAME_PTR:
		e.mu.Lock()
		e.namePtr = val
		e.mu.Unlock()
	case EXEC_CTRL:
		if val == EXEC_OP_EXECUTE {
			e.startExecute()
		} else if val == EXEC_OP_EMUTOS {
			e.startEmuTOS()
		} else if val == EXEC_OP_AROS {
			e.startAROS()
		} else if val == EXEC_OP_IEXEC {
			e.startIExec()
		}
	}
}

func (e *ProgramExecutor) startExecute() {
	e.mu.Lock()
	namePtr := e.namePtr
	fileName := e.readFileNameLocked(namePtr)
	fullPath, ok := e.sanitizePathLocked(fileName)
	typ := detectExecType(fileName)
	if !ok {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_PATH_INVALID
		e.typ = EXEC_TYPE_NONE
		e.mu.Unlock()
		return
	}
	if typ == EXEC_TYPE_NONE {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_UNSUPPORTED
		e.typ = EXEC_TYPE_NONE
		e.mu.Unlock()
		return
	}
	st, err := os.Stat(fullPath)
	if err != nil {
		e.status = EXEC_STATUS_ERROR
		if os.IsNotExist(err) {
			e.errCode = EXEC_ERR_NOT_FOUND
		} else {
			e.errCode = EXEC_ERR_LOAD_FAILED
		}
		e.typ = typ
		e.mu.Unlock()
		return
	}
	if st.IsDir() {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_LOAD_FAILED
		e.typ = typ
		e.mu.Unlock()
		return
	}
	e.session++
	session := e.session
	e.status = EXEC_STATUS_LOADING
	e.typ = typ
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()

	go e.executeAsync(session, fullPath, typ)
}

func (e *ProgramExecutor) startEmuTOS() {
	e.mu.Lock()
	loader := e.loadEmuTOS
	if loader == nil {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_LOAD_FAILED
		e.typ = EXEC_TYPE_EMUTOS
		e.mu.Unlock()
		return
	}
	e.session++
	session := e.session
	e.status = EXEC_STATUS_LOADING
	e.typ = EXEC_TYPE_EMUTOS
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()

	go func() {
		if err := loader(); err != nil {
			e.failSession(session, EXEC_ERR_LOAD_FAILED)
			return
		}
		e.mu.Lock()
		if session == e.session {
			e.status = EXEC_STATUS_RUNNING
			e.errCode = EXEC_ERR_OK
		}
		e.mu.Unlock()
	}()
}

func (e *ProgramExecutor) startAROS() {
	e.mu.Lock()
	loader := e.loadAROS
	if loader == nil {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_LOAD_FAILED
		e.typ = EXEC_TYPE_AROS
		e.mu.Unlock()
		return
	}
	e.session++
	session := e.session
	e.status = EXEC_STATUS_LOADING
	e.typ = EXEC_TYPE_AROS
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()

	go func() {
		if err := loader(); err != nil {
			e.failSession(session, EXEC_ERR_LOAD_FAILED)
			return
		}
		e.mu.Lock()
		if session == e.session {
			e.status = EXEC_STATUS_RUNNING
			e.errCode = EXEC_ERR_OK
		}
		e.mu.Unlock()
	}()
}

// SetIExecBootLoader configures a callback that boots the IExec microkernel.
// Called when BASIC writes EXEC_OP_IEXEC to EXEC_CTRL.
func (e *ProgramExecutor) SetIExecBootLoader(fn func() error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loadIExec = fn
}

func (e *ProgramExecutor) startIExec() {
	e.mu.Lock()
	loader := e.loadIExec
	if loader == nil {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_LOAD_FAILED
		e.typ = EXEC_TYPE_IEXEC
		e.mu.Unlock()
		return
	}
	e.session++
	session := e.session
	e.status = EXEC_STATUS_LOADING
	e.typ = EXEC_TYPE_IEXEC
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()

	go func() {
		if err := loader(); err != nil {
			e.failSession(session, EXEC_ERR_LOAD_FAILED)
			return
		}
		e.mu.Lock()
		if session == e.session {
			e.status = EXEC_STATUS_RUNNING
			e.errCode = EXEC_ERR_OK
		}
		e.mu.Unlock()
	}()
}

func (e *ProgramExecutor) executeAsync(session uint32, fullPath string, typ uint32) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		e.failSession(session, EXEC_ERR_LOAD_FAILED)
		return
	}

	e.mu.Lock()
	if session != e.session {
		e.mu.Unlock()
		return
	}
	launcher := e.launchExternal
	preLaunchIE64 := e.ie64CPU
	e.mu.Unlock()

	if err := e.launchProgram(fullPath, data, typ, launcher); err != nil {
		e.mu.Lock()
		if session == e.session {
			e.status = EXEC_STATUS_ERROR
			e.errCode = EXEC_ERR_LOAD_FAILED
		}
		e.mu.Unlock()
		return
	}

	e.mu.Lock()
	if session != e.session {
		e.mu.Unlock()
		return
	}
	if preLaunchIE64 != nil {
		preLaunchIE64.running.Store(false)
	}
	e.status = EXEC_STATUS_RUNNING
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()
}

func (e *ProgramExecutor) launchProgram(fullPath string, data []byte, typ uint32, externalLauncher func(path string) error) error {
	if externalLauncher != nil {
		return externalLauncher(fullPath)
	}
	return e.prepareAndLaunch(data, typ)
}

func (e *ProgramExecutor) prepareAndLaunch(data []byte, typ uint32) error {
	e.stopRunningCPUs()
	if e.emuTOSLoader != nil {
		e.emuTOSLoader.Stop()
		e.emuTOSLoader = nil
	}
	runtime.GC()

	switch typ {
	case EXEC_TYPE_IE32:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		cpu := NewCPU(e.bus)
		mem := e.bus.GetMemory()
		for i := PROG_START; i < len(mem) && i < STACK_START; i++ {
			mem[i] = 0
		}
		if PROG_START+len(data) > len(mem) {
			return fmt.Errorf("program too large")
		}
		copy(mem[PROG_START:], data)
		cpu.PC = PROG_START
		runtimeStatus.setCPUs(runtimeCPUIE32, cpu, nil, nil, nil, nil, nil)
		go cpu.Execute()
		return nil

	case EXEC_TYPE_IE64:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		cpu := NewCPU64(e.bus)
		cpu.jitEnabled = jitAvailable
		cpu.LoadProgramBytes(data)
		runtimeStatus.setCPUs(runtimeCPUIE64, nil, cpu, nil, nil, nil, nil)
		go cpu.jitExecute()
		return nil

	case EXEC_TYPE_6502:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPU6502Runner(e.bus, CPU6502Config{
			LoadAddr:     0x0800,
			Entry:        0,
			VoodooEngine: e.voodooEngine,
		})
		e.bus.Reset()
		loadAddr := uint32(0x0800)
		if loadAddr+uint32(len(data)) > DEFAULT_MEMORY_SIZE {
			return fmt.Errorf("program too large")
		}
		for i, b := range data {
			e.bus.Write8(loadAddr+uint32(i), b)
		}
		entry := uint16(0x0800)
		e.bus.Write8(RESET_VECTOR, uint8(entry&0x00FF))
		e.bus.Write8(RESET_VECTOR+1, uint8(entry>>8))
		e.bus.Write8(NMI_VECTOR, uint8(entry&0x00FF))
		e.bus.Write8(NMI_VECTOR+1, uint8(entry>>8))
		e.bus.Write8(IRQ_VECTOR, uint8(entry&0x00FF))
		e.bus.Write8(IRQ_VECTOR+1, uint8(entry>>8))
		runner.cpu.Reset()
		runner.cpu.SetRDYLine(true)
		runtimeStatus.setCPUs(runtimeCPU6502, nil, nil, nil, nil, nil, runner)
		go runner.Execute()
		return nil

	case EXEC_TYPE_M68K:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(true)
		}
		cpu := NewM68KCPU(e.bus)
		cpu.LoadProgramBytes(data)
		runner := NewM68KRunner(cpu)
		runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, runner, nil, nil, nil)
		go runner.Execute()
		return nil

	case EXEC_TYPE_Z80:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPUZ80Runner(e.bus, CPUZ80Config{
			LoadAddr:     0,
			Entry:        0,
			JITEnabled:   z80JitAvailable,
			VGAEngine:    e.vgaEngine,
			VoodooEngine: e.voodooEngine,
		})
		e.bus.Reset()
		limit := uint32(e.bus.BankedVisibleCeiling())
		if uint32(len(data)) > limit {
			return fmt.Errorf("program too large: size=0x%X, banked-ceiling=0x%X", uint32(len(data)), limit)
		}
		for i, b := range data {
			e.bus.Write8(uint32(i), b)
		}
		runner.cpu.Reset()
		runner.cpu.PC = 0
		runtimeStatus.setCPUs(runtimeCPUZ80, nil, nil, nil, runner, nil, nil)
		go runner.Execute()
		return nil

	case EXEC_TYPE_X86:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPUX86Runner(e.bus, &CPUX86Config{
			LoadAddr:     0,
			Entry:        0,
			JITEnabled:   x86JitAvailable,
			VGAEngine:    e.vgaEngine,
			VoodooEngine: e.voodooEngine,
		})
		if err := runner.LoadProgramData(data); err != nil {
			return err
		}
		runtimeStatus.setCPUs(runtimeCPUX86, nil, nil, nil, nil, runner, nil)
		go runner.Execute()
		return nil

	case EXEC_TYPE_EMUTOS:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(true)
		}
		cpu := NewM68KCPU(e.bus)
		loader := NewEmuTOSLoader(e.bus, cpu, e.videoChip)
		if err := loader.LoadROM(data); err != nil {
			return err
		}
		if e.gemdosHostRoot != "" {
			if err := loader.SetupGemdos(e.gemdosHostRoot, e.gemdosDriveNum); err != nil {
				fmt.Printf("Warning: GEMDOS drive U: disabled: %v\n", err)
			}
		}
		loader.StartTimer()
		e.emuTOSLoader = loader

		runner := NewM68KRunner(cpu)
		runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, runner, nil, nil, nil)
		go runner.Execute()
		return nil
	}

	return fmt.Errorf("unsupported execute type")
}

func (e *ProgramExecutor) stopRunningCPUs() {
	snap := runtimeStatus.snapshot()
	if snap.ie32 != nil {
		snap.ie32.Stop()
	}
	if snap.ie64 != nil {
		snap.ie64.Stop()
	}
	if snap.m68k != nil {
		snap.m68k.Stop()
	}
	if snap.z80 != nil {
		snap.z80.Stop()
	}
	if snap.x86 != nil {
		snap.x86.Stop()
	}
	if snap.cpu65 != nil {
		snap.cpu65.Stop()
	}
}

func (e *ProgramExecutor) failSession(session uint32, errCode uint32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if session != e.session {
		return
	}
	e.status = EXEC_STATUS_ERROR
	e.errCode = errCode
}

func detectExecType(path string) uint32 {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".iex", ".ie32":
		return EXEC_TYPE_IE32
	case ".ie64":
		return EXEC_TYPE_IE64
	case ".ie65":
		return EXEC_TYPE_6502
	case ".ie68":
		return EXEC_TYPE_M68K
	case ".ie80":
		return EXEC_TYPE_Z80
	case ".ie86":
		return EXEC_TYPE_X86
	case ".tos", ".img":
		return EXEC_TYPE_EMUTOS
	case ".ies":
		return EXEC_TYPE_SCRIPT
	default:
		return EXEC_TYPE_NONE
	}
}

func (e *ProgramExecutor) sanitizePathLocked(path string) (string, bool) {
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", false
	}
	fullPath := filepath.Join(e.baseDir, path)
	rel, err := filepath.Rel(e.baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return fullPath, true
}

func (e *ProgramExecutor) readFileNameLocked(ptr uint32) string {
	var name []byte
	addr := ptr
	for {
		b := e.bus.Read8(addr)
		if b == 0 {
			break
		}
		name = append(name, b)
		addr++
		if len(name) > 255 {
			break
		}
	}
	return string(name)
}
