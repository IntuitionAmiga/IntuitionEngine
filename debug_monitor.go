// debug_monitor.go - Machine Monitor core (freeze/resume, activate/deactivate)

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

// MonitorState represents whether the monitor is active.
type MonitorState int

const (
	MonitorInactive MonitorState = iota
	MonitorActive
	MonitorHexEdit
)

// OutputLine holds styled text for the monitor scrollback buffer.
type OutputLine struct {
	Text  string
	Color uint32 // RGBA packed
}

// WriteRecord records a single write to a watched address during trace.
type WriteRecord struct {
	PC       uint64
	OldValue byte
	NewValue byte
	StepNum  int
}

// CPUEntry associates a stable integer ID with a debuggable CPU.
type CPUEntry struct {
	ID    int
	Label string
	CPU   DebuggableCPU
}

// MachineMonitor is the core debugger state machine.
type MachineMonitor struct {
	mu    sync.Mutex
	state MonitorState

	cpus      map[int]*CPUEntry
	nextID    int
	focusedID int

	breakpointChan chan BreakpointEvent

	outputLines  []OutputLine
	maxOutput    int
	scrollOffset int

	inputLine  []byte
	cursorPos  int
	history    []string
	historyIdx int

	wasRunning map[int]bool
	soundChip  *SoundChip

	bus       *MachineBus
	coprocMgr *CoprocessorManager
	prevRegs  map[string]uint64 // for change highlighting

	// Run-Until temp breakpoints (Feature 2)
	tempBreakpoints map[int]map[uint64]bool
	savedConditions map[int]map[uint64]*BreakpointCondition // original conditions saved during run-until

	// Trace state (Feature 8)
	traceFile      *os.File
	traceWatches   map[uint64]bool
	traceSnapshots map[uint64]byte
	writeHistory   map[uint64][]WriteRecord

	// Backstep (Feature 9) — per-CPU history keyed by CPU ID
	stepHistory map[int][]*MachineSnapshot
	maxBackstep int

	// Hex editor (Feature 12)
	hexEditAddr   uint64
	hexEditCursor int
	hexEditNibble int
	hexEditDirty  map[uint64]byte

	// Scripting (Feature 13)
	macros      map[string][]string
	scriptDepth int
}

// NewMachineMonitor creates a new monitor instance.
func NewMachineMonitor(bus *MachineBus) *MachineMonitor {
	return &MachineMonitor{
		state:           MonitorInactive,
		cpus:            make(map[int]*CPUEntry),
		breakpointChan:  make(chan BreakpointEvent, 1),
		maxOutput:       500,
		wasRunning:      make(map[int]bool),
		bus:             bus,
		prevRegs:        make(map[string]uint64),
		tempBreakpoints: make(map[int]map[uint64]bool),
		savedConditions: make(map[int]map[uint64]*BreakpointCondition),
		traceWatches:    make(map[uint64]bool),
		traceSnapshots:  make(map[uint64]byte),
		writeHistory:    make(map[uint64][]WriteRecord),
		stepHistory:     make(map[int][]*MachineSnapshot),
		maxBackstep:     32,
		hexEditDirty:    make(map[uint64]byte),
		macros:          make(map[string][]string),
	}
}

// RegisterCPU adds a CPU to the monitor and returns its stable ID.
func (m *MachineMonitor) RegisterCPU(label string, cpu DebuggableCPU) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.cpus[id] = &CPUEntry{ID: id, Label: label, CPU: cpu}
	cpu.SetBreakpointChannel(m.breakpointChan, id)
	if len(m.cpus) == 1 {
		m.focusedID = id
	}
	return id
}

// ResetCPUs stops all trap-mode goroutines, clears breakpoints, and
// removes all registered CPUs. Must be called before recreating CPUs
// (e.g. F10 reset / IPC mode switch) to prevent stale trapLoop
// goroutines from continuing to step against reset state.
func (m *MachineMonitor) ResetCPUs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.cpus {
		// Freeze stops any adapter-managed trapLoop goroutine that may
		// be running independently of the native CPU runner.
		if entry.CPU.IsRunning() {
			entry.CPU.Freeze()
		}
		entry.CPU.ClearAllBreakpoints()
		entry.CPU.ClearAllWatchpoints()
	}
	m.cpus = make(map[int]*CPUEntry)
	m.nextID = 0
	m.focusedID = 0
	m.wasRunning = make(map[int]bool)
	m.tempBreakpoints = make(map[int]map[uint64]bool)
	m.savedConditions = make(map[int]map[uint64]*BreakpointCondition)
	m.stepHistory = make(map[int][]*MachineSnapshot)
}

// UnregisterCPU removes a CPU by its stable ID.
func (m *MachineMonitor) UnregisterCPU(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cpus[id]
	if !ok {
		return
	}
	entry.CPU.ClearAllBreakpoints()
	delete(m.cpus, id)
	delete(m.stepHistory, id)
	delete(m.tempBreakpoints, id)
	delete(m.savedConditions, id)
	if m.focusedID == id {
		m.focusedID = 0 // fall back to primary
	}
}

// IsActive returns whether the monitor is currently shown.
func (m *MachineMonitor) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == MonitorActive || m.state == MonitorHexEdit
}

// FocusedCPU returns the currently focused CPU entry, or nil.
func (m *MachineMonitor) FocusedCPU() *CPUEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cpus[m.focusedID]
}

// Activate freezes all CPUs and enters the monitor.
func (m *MachineMonitor) Activate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == MonitorActive {
		return
	}
	m.state = MonitorActive
	m.wasRunning = make(map[int]bool)

	for id, entry := range m.cpus {
		if entry.CPU.IsRunning() {
			m.wasRunning[id] = true
			entry.CPU.Freeze()
		}
	}

	m.scrollOffset = 0
	m.inputLine = nil
	m.cursorPos = 0
	m.historyIdx = len(m.history)

	// Save current register state for change highlighting
	m.saveCurrentRegs()

	// Show activation message and initial register dump
	m.appendOutput("MACHINE MONITOR - Type ? for help", colorCyan)
	m.showRegisters()
	m.showDisassembly(0, 8)
}

// Deactivate resumes previously-running CPUs and exits the monitor.
func (m *MachineMonitor) Deactivate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == MonitorInactive {
		return
	}
	m.state = MonitorInactive

	for id, entry := range m.cpus {
		if m.wasRunning[id] {
			entry.CPU.Resume()
		}
	}
}

// FreezeAll freezes all registered CPUs.
func (m *MachineMonitor) FreezeAll() {
	for _, entry := range m.cpus {
		if entry.CPU.IsRunning() {
			entry.CPU.Freeze()
		}
	}
}

// appendOutput adds a line to the scrollback buffer.
func (m *MachineMonitor) appendOutput(text string, color uint32) {
	m.outputLines = append(m.outputLines, OutputLine{Text: text, Color: color})
	if len(m.outputLines) > m.maxOutput {
		m.outputLines = m.outputLines[len(m.outputLines)-m.maxOutput:]
	}
}

// saveCurrentRegs snapshots the focused CPU's registers for change detection.
func (m *MachineMonitor) saveCurrentRegs() {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}
	m.prevRegs = make(map[string]uint64)
	for _, r := range entry.CPU.GetRegisters() {
		m.prevRegs[r.Name] = r.Value
	}
}

// StartBreakpointListener runs a background goroutine that watches for
// breakpoint events from any CPU and auto-activates the monitor.
func (m *MachineMonitor) StartBreakpointListener() {
	go func() {
		for ev := range m.breakpointChan {
			m.handleBreakpointHit(ev)
		}
	}()
}

// handleBreakpointHit freezes all CPUs, focuses on the one that hit,
// and activates the monitor.
func (m *MachineMonitor) handleBreakpointHit(ev BreakpointEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Snapshot which CPUs are running BEFORE freezing, so Deactivate
	// only resumes CPUs that were genuinely running. The CPU that hit
	// the breakpoint already stopped its own trapLoop, so IsRunning()
	// returns false for it — we record it explicitly below.
	wasRunning := make(map[int]bool)
	for id, entry := range m.cpus {
		if entry.CPU.IsRunning() {
			wasRunning[id] = true
		}
	}
	// The breakpoint-hitting CPU was running (it stopped itself just
	// before publishing the event), so mark it too.
	wasRunning[ev.CPUID] = true

	// Now freeze all still-running CPUs
	for _, entry := range m.cpus {
		if entry.CPU.IsRunning() {
			entry.CPU.Freeze()
		}
	}

	// Handle run-until: clear temp breakpoint or restore saved condition
	if temps, ok := m.tempBreakpoints[ev.CPUID]; ok {
		if temps[ev.Address] {
			if entry := m.cpus[ev.CPUID]; entry != nil {
				entry.CPU.ClearBreakpoint(ev.Address)
			}
			delete(temps, ev.Address)
			if len(temps) == 0 {
				delete(m.tempBreakpoints, ev.CPUID)
			}
		}
	}
	if saved, ok := m.savedConditions[ev.CPUID]; ok {
		if cond, hasSaved := saved[ev.Address]; hasSaved {
			// Restore original condition on the user's breakpoint
			if entry := m.cpus[ev.CPUID]; entry != nil {
				if bp := entry.CPU.GetConditionalBreakpoint(ev.Address); bp != nil {
					bp.Condition = cond
				}
			}
			delete(saved, ev.Address)
			if len(saved) == 0 {
				delete(m.savedConditions, ev.CPUID)
			}
		}
	}

	// Build the display message
	var msg string
	if ev.IsWatch {
		msg = fmt.Sprintf("WATCH $%X: $%02X -> $%02X at PC=$%X on %s (id:%d)",
			ev.WatchAddr, ev.WatchOldValue, ev.WatchNewValue, ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	} else {
		msg = fmt.Sprintf("BREAK at $%X on %s (id:%d)", ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	}

	// If already active, just print the message and switch focus
	if m.state == MonitorActive {
		m.focusedID = ev.CPUID
		m.appendOutput(msg, colorRed)
		m.saveCurrentRegs()
		m.showRegisters()
		m.showDisassembly(0, 8)
		return
	}

	// Activate the monitor
	m.state = MonitorActive
	m.wasRunning = wasRunning
	m.focusedID = ev.CPUID

	m.scrollOffset = 0
	m.inputLine = nil
	m.cursorPos = 0
	m.historyIdx = len(m.history)

	m.appendOutput(msg, colorRed)
	m.saveCurrentRegs()
	m.showRegisters()
	m.showDisassembly(0, 8)
}

func getLabelForCPU(cpus map[int]*CPUEntry, id int) string {
	if e, ok := cpus[id]; ok {
		return e.Label
	}
	return "???"
}

// Color constants (RGBA packed as 0xRRGGBBAA)
const (
	colorWhite   = 0xFFFFFFFF
	colorCyan    = 0x64C8FFFF
	colorYellow  = 0xFFFF55FF
	colorRed     = 0xFF5555FF
	colorGreen   = 0x55FF55FF
	colorDim     = 0x5555FFFF
	colorMagenta = 0xFF55FFFF
)

// yieldLock temporarily releases mu so the Ebiten render loop can acquire it,
// then re-acquires. Used during trace to keep the UI responsive.
func (m *MachineMonitor) yieldLock() {
	m.mu.Unlock()
	runtime.Gosched()
	m.mu.Lock()
}
