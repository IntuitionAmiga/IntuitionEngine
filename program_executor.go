package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	if st, err := os.Stat(fullPath); err != nil || st.IsDir() {
		e.status = EXEC_STATUS_ERROR
		e.errCode = EXEC_ERR_NOT_FOUND
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

func (e *ProgramExecutor) executeAsync(session uint32, fullPath string, typ uint32) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		e.failSession(session, EXEC_ERR_LOAD_FAILED)
		return
	}

	if err := e.prepareAndLaunch(data, typ); err != nil {
		e.failSession(session, EXEC_ERR_LOAD_FAILED)
		return
	}

	e.mu.Lock()
	if session != e.session {
		e.mu.Unlock()
		return
	}
	e.status = EXEC_STATUS_RUNNING
	e.errCode = EXEC_ERR_OK
	e.mu.Unlock()

	if e.ie64CPU != nil {
		e.ie64CPU.running.Store(false)
	}
}

func (e *ProgramExecutor) prepareAndLaunch(data []byte, typ uint32) error {
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
		go cpu.Execute()
		return nil

	case EXEC_TYPE_IE64:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		cpu := NewCPU64(e.bus)
		cpu.LoadProgramBytes(data)
		go cpu.Execute()
		return nil

	case EXEC_TYPE_6502:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPU6502Runner(e.bus, CPU6502Config{LoadAddr: 0x0800, Entry: 0})
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
		go runner.Execute()
		return nil

	case EXEC_TYPE_M68K:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(true)
		}
		cpu := NewM68KCPU(e.bus)
		cpu.LoadProgramBytes(data)
		runner := NewM68KRunner(cpu)
		go runner.Execute()
		return nil

	case EXEC_TYPE_Z80:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPUZ80Runner(e.bus, CPUZ80Config{
			LoadAddr:     0,
			Entry:        0,
			VGAEngine:    e.vgaEngine,
			VoodooEngine: e.voodooEngine,
		})
		e.bus.Reset()
		if uint32(len(data)) > DEFAULT_MEMORY_SIZE {
			return fmt.Errorf("program too large")
		}
		for i, b := range data {
			e.bus.Write8(uint32(i), b)
		}
		runner.cpu.Reset()
		runner.cpu.PC = 0
		go runner.Execute()
		return nil

	case EXEC_TYPE_X86:
		if e.videoChip != nil {
			e.videoChip.SetBigEndianMode(false)
		}
		runner := NewCPUX86Runner(e.bus, &CPUX86Config{
			LoadAddr:     0,
			Entry:        0,
			VGAEngine:    e.vgaEngine,
			VoodooEngine: e.voodooEngine,
		})
		if err := runner.LoadProgramData(data); err != nil {
			return err
		}
		go runner.Execute()
		return nil
	}

	return fmt.Errorf("unsupported execute type")
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
