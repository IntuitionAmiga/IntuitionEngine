package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type M68KFaultClass string

const (
	M68KFaultClassBusError           M68KFaultClass = "bus_error"
	M68KFaultClassAddressError       M68KFaultClass = "address_error"
	M68KFaultClassIllegalInstruction M68KFaultClass = "illegal_instruction"
	M68KFaultClassLineA              M68KFaultClass = "line_a"
	M68KFaultClassLineF              M68KFaultClass = "line_f"
)

type M68KFaultRecord struct {
	Class          M68KFaultClass
	Vector         uint8
	PC             uint32
	FaultPC        uint32
	Opcode         uint16
	AccessAddr     uint32
	AccessSize     uint8
	Write          bool
	Instruction    bool
	Data           uint32
	MnemonicFamily string
	AddressingMode string
	Message        string
	Count          int
}

func (r M68KFaultRecord) Signature() string {
	return fmt.Sprintf("%s|%08X|%04X|%s|%s",
		r.Class,
		r.FaultPC,
		r.Opcode,
		strings.ToUpper(r.MnemonicFamily),
		r.AddressingMode,
	)
}

type M68KFaultManifest struct {
	mu      sync.Mutex
	index   map[string]int
	records []M68KFaultRecord
}

func NewM68KFaultManifest() *M68KFaultManifest {
	return &M68KFaultManifest{
		index: make(map[string]int),
	}
}

func (m *M68KFaultManifest) Add(record M68KFaultRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sig := record.Signature()
	if idx, ok := m.index[sig]; ok {
		m.records[idx].Count++
		return
	}
	record.Count = 1
	m.index[sig] = len(m.records)
	m.records = append(m.records, record)
}

func (m *M68KFaultManifest) Records() []M68KFaultRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]M68KFaultRecord, len(m.records))
	copy(out, m.records)
	return out
}

func NormalizeM68KFaultRecord(cpu *M68KCPU, record M68KFaultRecord) M68KFaultRecord {
	if cpu == nil {
		return record
	}
	mem := cpu.bus.GetMemory()
	if record.Opcode == 0 && record.FaultPC < DEFAULT_MEMORY_SIZE-1 {
		record.Opcode = cpu.Read16(record.FaultPC)
	}
	if record.FaultPC >= DEFAULT_MEMORY_SIZE {
		return record
	}

	lines := disassembleM68K(func(addr uint64, size int) []byte {
		start := uint32(addr)
		end := start + uint32(size)
		if end > uint32(len(mem)) || end < start {
			return nil
		}
		out := make([]byte, size)
		copy(out, mem[start:end])
		return out
	}, uint64(record.FaultPC), 1)
	if len(lines) == 0 {
		return record
	}

	family, mode := normalizeM68KFaultMnemonic(lines[0].Mnemonic)
	if record.MnemonicFamily == "" {
		record.MnemonicFamily = family
	}
	if record.AddressingMode == "" {
		record.AddressingMode = mode
	}
	return record
}

func normalizeM68KFaultMnemonic(mnemonic string) (family, mode string) {
	mnemonic = strings.TrimSpace(mnemonic)
	if mnemonic == "" {
		return "", ""
	}
	if strings.HasPrefix(mnemonic, "dc.w") {
		switch {
		case strings.Contains(mnemonic, "Line-F"):
			return "FPU", ""
		case strings.Contains(mnemonic, "Line-A"):
			return "LINEA", ""
		default:
			return "DC", ""
		}
	}

	op, rest, found := strings.Cut(mnemonic, " ")
	family = strings.ToUpper(strings.TrimSuffix(op, ".B"))
	family = strings.TrimSuffix(family, ".W")
	family = strings.TrimSuffix(family, ".L")
	if !found {
		return family, ""
	}

	operands := splitM68KOperands(rest)
	if len(operands) == 0 {
		return family, ""
	}
	return family, normalizeM68KOperandMode(operands[0])
}

func splitM68KOperands(s string) []string {
	var out []string
	start := 0
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(s[start:i])
				if part != "" {
					out = append(out, part)
				}
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func normalizeM68KOperandMode(operand string) string {
	operand = strings.TrimSpace(operand)
	switch {
	case operand == "":
		return ""
	case strings.HasPrefix(operand, "#"):
		return "#<data>"
	case len(operand) == 2 && operand[0] == 'D' && operand[1] >= '0' && operand[1] <= '7':
		return "Dn"
	case len(operand) == 2 && operand[0] == 'A' && operand[1] >= '0' && operand[1] <= '7':
		return "An"
	case strings.HasPrefix(operand, "-(A") && strings.HasSuffix(operand, ")"):
		return "-(An)"
	case strings.HasPrefix(operand, "(A") && strings.HasSuffix(operand, ")+"):
		return "(An)+"
	case strings.HasPrefix(operand, "(A") && strings.HasSuffix(operand, ")"):
		return "(An)"
	case strings.Contains(operand, "(PC,"):
		return "(d8,PC,Xn)"
	case strings.HasSuffix(operand, "(PC)"):
		return "(d16,PC)"
	case strings.Contains(operand, "(A") && strings.Contains(operand, ","):
		return "(d8,An,Xn)"
	case strings.Contains(operand, "(A"):
		return "(d16,An)"
	case strings.HasSuffix(operand, ".W"):
		return "(xxx).W"
	case strings.HasPrefix(operand, "$"):
		return "(xxx).L"
	default:
		return operand
	}
}

const (
	arosExecThisTaskOffset  = 0x114
	arosExecTaskReadyOffset = 0x196
	arosExecTaskWaitOffset  = 0x1A4
	arosTaskNameOffset      = 10
)

type AROSReadyState struct {
	Ready          bool
	SysBase        uint32
	ThisTask       uint32
	TaskName       string
	TaskReadyCount int
	TaskWaitCount  int
	IRQReady       bool
}

func ProbeAROSReadyState(cpu *M68KCPU, loader *AROSLoader) AROSReadyState {
	state := AROSReadyState{
		TaskReadyCount: -1,
		TaskWaitCount:  -1,
	}
	if cpu == nil {
		return state
	}

	sysBase := cpu.Read32(4)
	state.SysBase = sysBase
	if !isValidAROSGuestPtr(sysBase) {
		return state
	}

	thisTask := cpu.Read32(sysBase + arosExecThisTaskOffset)
	state.ThisTask = thisTask
	if !isValidAROSGuestPtr(thisTask) {
		return state
	}

	namePtr := cpu.Read32(thisTask + arosTaskNameOffset)
	if isValidAROSGuestPtr(namePtr) {
		state.TaskName = readAROSCStr(cpu, namePtr, 64)
	}

	state.TaskReadyCount = countAROSList(cpu, sysBase+arosExecTaskReadyOffset)
	state.TaskWaitCount = countAROSList(cpu, sysBase+arosExecTaskWaitOffset)
	if loader != nil {
		state.IRQReady = loader.l2Armed || loader.l4Armed || loader.l5Armed
	}

	state.Ready = state.TaskName != "" && state.TaskReadyCount >= 0 && state.TaskWaitCount >= 0
	return state
}

func isValidAROSGuestPtr(addr uint32) bool {
	return addr >= 0x1000 && addr < DEFAULT_MEMORY_SIZE
}

func readAROSCStr(cpu *M68KCPU, addr uint32, maxLen int) string {
	var b strings.Builder
	for i := range maxLen {
		ch := cpu.Read8(addr + uint32(i))
		if ch == 0 {
			break
		}
		if ch >= 32 && ch < 127 {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func countAROSList(cpu *M68KCPU, listAddr uint32) int {
	head := cpu.Read32(listAddr)
	if !isValidAROSGuestPtr(head) {
		return 0
	}

	count := 0
	node := head
	for range 256 {
		if !isValidAROSGuestPtr(node) {
			break
		}
		count++
		next := cpu.Read32(node)
		if next == 0 || next == node {
			break
		}
		node = next
	}
	return count
}

func m68kFaultClassForVector(vector uint8) M68KFaultClass {
	switch vector {
	case M68K_VEC_BUS_ERROR:
		return M68KFaultClassBusError
	case M68K_VEC_ADDRESS_ERROR:
		return M68KFaultClassAddressError
	case M68K_VEC_ILLEGAL_INSTR:
		return M68KFaultClassIllegalInstruction
	case M68K_VEC_LINE_A:
		return M68KFaultClassLineA
	case M68K_VEC_LINE_F:
		return M68KFaultClassLineF
	default:
		return ""
	}
}

func (cpu *M68KCPU) emitStructuredFault(vector uint8, faultPC uint32) {
	if cpu == nil || cpu.FaultHook == nil {
		return
	}

	class := m68kFaultClassForVector(vector)
	if class == "" {
		return
	}

	cpu.FaultHook(M68KFaultRecord{
		Class:       class,
		Vector:      vector,
		PC:          cpu.PC,
		FaultPC:     faultPC,
		Opcode:      cpu.lastExecOpcode,
		AccessAddr:  cpu.lastFaultAddr,
		AccessSize:  cpu.lastFaultSize,
		Write:       cpu.lastFaultWrite,
		Instruction: cpu.lastFaultIsInstruction,
		Data:        cpu.lastFaultData,
	})
}

func MissingAROSBootRegressionCoverage(records []M68KFaultRecord) []string {
	inventory := KnownAROSBootRegressionInventory()
	seen := make(map[string]struct{})
	var missing []string
	for _, record := range records {
		sig := record.Signature()
		if sig == "" {
			continue
		}
		if _, ok := inventory[sig]; ok {
			continue
		}
		if _, ok := seen[sig]; ok {
			continue
		}
		seen[sig] = struct{}{}
		missing = append(missing, sig)
	}
	sort.Strings(missing)
	return missing
}

type AROSBootHarness struct {
	CPU          *M68KCPU
	Loader       *AROSLoader
	Timeout      time.Duration
	PollInterval time.Duration
	Probe        func(*M68KCPU, *AROSLoader) AROSReadyState
}

type AROSBootResult struct {
	Ready    AROSReadyState
	Faults   []M68KFaultRecord
	TimedOut bool
}

type AROSInterpreterBootEnvironment struct {
	Bus      *MachineBus
	CPU      *M68KCPU
	Runner   *M68KRunner
	Loader   *AROSLoader
	Harness  AROSBootHarness
	Video    *VideoChip
	Sound    *SoundChip
	hostRoot string
}

func (h AROSBootHarness) Run(ctx context.Context) AROSBootResult {
	result := AROSBootResult{}
	if h.CPU == nil {
		result.TimedOut = true
		return result
	}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	poll := h.PollInterval
	if poll <= 0 {
		poll = 5 * time.Millisecond
	}
	probe := h.Probe
	if probe == nil {
		probe = ProbeAROSReadyState
	}

	manifest := NewM68KFaultManifest()
	prevHook := h.CPU.FaultHook
	h.CPU.FaultHook = func(record M68KFaultRecord) {
		record = NormalizeM68KFaultRecord(h.CPU, record)
		manifest.Add(record)
		if prevHook != nil {
			prevHook(record)
		}
	}
	defer func() {
		h.CPU.FaultHook = prevHook
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		result.Ready = probe(h.CPU, h.Loader)
		result.Faults = manifest.Records()
		if result.Ready.Ready || len(result.Faults) > 0 {
			return result
		}

		select {
		case <-ctx.Done():
			result.TimedOut = true
			return result
		case <-deadline.C:
			result.TimedOut = true
			result.Ready = probe(h.CPU, h.Loader)
			result.Faults = manifest.Records()
			return result
		case <-ticker.C:
		}
	}
}

func NewAROSInterpreterBootEnvironment(rom []byte, hostRoot string) (*AROSInterpreterBootEnvironment, error) {
	bus := NewMachineBus()

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		return nil, fmt.Errorf("new video chip: %w", err)
	}
	video.AttachBus(bus)
	configureArosVRAM(bus, video)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)

	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		video.Stop()
		return nil, fmt.Errorf("new sound chip: %w", err)
	}
	sound.AttachBusMemory(bus.GetMemory())

	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = false
	runner := NewM68KRunner(cpu)
	runner.cpu.m68kJitEnabled = false

	loader := NewAROSLoader(bus, cpu, video)
	if err := loader.LoadROM(rom); err != nil {
		sound.Stop()
		video.Stop()
		return nil, fmt.Errorf("load AROS ROM: %w", err)
	}

	if hostRoot != "" {
		if dos, err := NewArosDOSDevice(bus, hostRoot); err == nil {
			bus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, dos.HandleRead, dos.HandleWrite)
		}
	}

	dma := NewArosAudioDMA(bus, sound, cpu)
	bus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, dma.HandleRead, dma.HandleWrite)
	sound.SetSampleTicker(dma)

	clip := NewClipboardBridge(bus)
	bus.MapIO(CLIP_REGION_BASE, CLIP_REGION_END, clip.HandleRead, clip.HandleWrite)

	loader.MapIRQDiagnostics()

	video.Start()
	sound.Start()

	env := &AROSInterpreterBootEnvironment{
		Bus:      bus,
		CPU:      cpu,
		Runner:   runner,
		Loader:   loader,
		Video:    video,
		Sound:    sound,
		hostRoot: hostRoot,
	}
	env.Harness = AROSBootHarness{
		CPU:          cpu,
		Loader:       loader,
		Timeout:      15 * time.Second,
		PollInterval: 5 * time.Millisecond,
	}
	return env, nil
}

func (env *AROSInterpreterBootEnvironment) BootAndWait(ctx context.Context) (AROSBootResult, error) {
	if env == nil || env.Runner == nil || env.Loader == nil {
		return AROSBootResult{}, fmt.Errorf("AROS interpreter boot environment is incomplete")
	}
	env.Loader.StartTimer()
	env.Runner.StartExecution()
	result := env.Harness.Run(ctx)
	return result, nil
}

func (env *AROSInterpreterBootEnvironment) Close() {
	if env == nil {
		return
	}
	if env.Runner != nil {
		env.Runner.Stop()
	}
	if env.Loader != nil {
		env.Loader.Stop()
	}
	if env.Sound != nil {
		env.Sound.Stop()
	}
	if env.Video != nil {
		env.Video.Stop()
	}
}
