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
	"strings"
	"sync"
	"sync/atomic"
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

type eventQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []BreakpointEvent
	closed bool
}

func newEventQueue() *eventQueue {
	q := &eventQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *eventQueue) push(ev BreakpointEvent) {
	q.mu.Lock()
	if !q.closed {
		q.buf = append(q.buf, ev)
		q.cond.Signal()
	}
	q.mu.Unlock()
}

func (q *eventQueue) pop() (BreakpointEvent, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.buf) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.buf) == 0 {
		return BreakpointEvent{}, false
	}
	ev := q.buf[0]
	copy(q.buf, q.buf[1:])
	q.buf = q.buf[:len(q.buf)-1]
	return ev, true
}

func (q *eventQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

type monitorCPUReader struct {
	cancel chan struct{}
	done   chan struct{}
}

type StopReason int

const (
	StopBreakpoint StopReason = iota
	StopWatchpoint
	StopFault
	StopReset
	StopManualFreeze
	StopUserAbort
)

type StopHook func(cpuID int, reason StopReason, addr uint64)

// MachineMonitor is the core debugger state machine.
type MachineMonitor struct {
	mu    sync.Mutex
	state MonitorState

	cpus      map[int]*CPUEntry
	nextID    int
	focusedID int

	breakpointChan chan BreakpointEvent
	eventQueue     *eventQueue
	cpuReaders     map[int]*monitorCPUReader
	listenerOnce   sync.Once
	listenerActive atomic.Bool
	stopHooks      map[int]StopHook
	nextStopHookID int

	outputLines  []OutputLine
	maxOutput    int
	scrollOffset int

	inputLine          []byte
	cursorPos          int
	history            []string
	historyIdx         int
	aliases            map[string]string
	loadedRC           map[string]string
	lastRepeat         string
	historySearchQuery string
	historySearchIdx   int

	wasRunning map[int]bool
	soundChip  *SoundChip
	devices    map[string]DebugSnapshotDevice

	bus       *MachineBus
	coprocMgr *CoprocessorManager
	prevRegs  map[string]uint64 // for change highlighting
	symbols   *SymbolTable
	sources   *SourceLineTable
	regions   *RegionRegistry
	access    *DebugAccessService
	faults    *DebugFaultService

	// Run-Until temp breakpoints (Feature 2)
	tempBreakpoints map[int]map[uint64]bool
	savedConditions map[int]map[uint64]*BreakpointCondition // original conditions saved during run-until
	runUntilHooks   map[int]map[uint64]int

	// Trace state (Feature 8)
	traceFile      *os.File
	traceWatches   map[uint64]bool
	traceSnapshots map[uint64]byte
	writeHistory   map[uint64][]WriteRecord
	traceRings     map[int]*DebugTraceRing
	timelineEvents []TimelineEvent
	timelineSeq    atomic.Uint64
	maxTimeline    int

	// Backstep (Feature 9) - per-CPU history keyed by CPU ID
	stepHistory             map[int][]*MachineSnapshot
	wholeHistory            []*WholeMachineSnapshot
	nextWholeID             uint64
	wholeDeltaCount         int
	wholeDeltaBytes         uint64
	maxBackstep             int
	maxWholeHistory         int
	wholeCheckpointInterval int
	wholeCheckpointBytes    uint64
	maxWholeCheckpoints     int

	// Hex editor (Feature 12)
	hexEditAddr   uint64
	hexEditCursor int
	hexEditNibble int
	hexEditDirty  map[uint64]byte

	// IE64 monitor assembler mode
	assembleMode bool
	assembleAddr uint64

	// Scripting (Feature 13)
	macros      map[string][]string
	scriptDepth int
}

// NewMachineMonitor creates a new monitor instance.
func NewMachineMonitor(bus *MachineBus) *MachineMonitor {
	monitor := &MachineMonitor{
		state:                   MonitorInactive,
		cpus:                    make(map[int]*CPUEntry),
		breakpointChan:          make(chan BreakpointEvent, 8),
		eventQueue:              newEventQueue(),
		cpuReaders:              make(map[int]*monitorCPUReader),
		stopHooks:               make(map[int]StopHook),
		maxOutput:               500,
		wasRunning:              make(map[int]bool),
		devices:                 make(map[string]DebugSnapshotDevice),
		bus:                     bus,
		prevRegs:                make(map[string]uint64),
		symbols:                 NewSymbolTable(),
		sources:                 NewSourceLineTable(),
		regions:                 NewRegionRegistry(),
		access:                  NewDebugAccessService(),
		faults:                  NewDebugFaultService(),
		tempBreakpoints:         make(map[int]map[uint64]bool),
		savedConditions:         make(map[int]map[uint64]*BreakpointCondition),
		runUntilHooks:           make(map[int]map[uint64]int),
		traceWatches:            make(map[uint64]bool),
		traceSnapshots:          make(map[uint64]byte),
		writeHistory:            make(map[uint64][]WriteRecord),
		traceRings:              make(map[int]*DebugTraceRing),
		maxTimeline:             4096,
		stepHistory:             make(map[int][]*MachineSnapshot),
		maxBackstep:             32,
		maxWholeHistory:         32,
		wholeCheckpointInterval: 32,
		wholeCheckpointBytes:    64 << 20,
		maxWholeCheckpoints:     8,
		hexEditDirty:            make(map[uint64]byte),
		macros:                  make(map[string][]string),
		aliases:                 make(map[string]string),
		loadedRC:                make(map[string]string),
	}
	if bus != nil {
		bus.SetDebugAccessService(monitor.access)
	}
	monitor.access.SetSequenceSource(monitor.nextTimelineSeq)
	monitor.loadPersistentHistory()
	return monitor
}

func (m *MachineMonitor) nextTimelineSeq() uint64 {
	return m.timelineSeq.Add(1)
}

// RegisterSnapshotDevice adds a versioned mutable device to whole-machine
// snapshots. Names are stable snapshot keys, so replacing a device with the
// same name updates future capture/restore operations.
func (m *MachineMonitor) RegisterSnapshotDevice(dev DebugSnapshotDevice) {
	if m == nil || dev == nil {
		return
	}
	name := strings.TrimSpace(dev.DebugSnapshotName())
	if name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[name] = dev
}

// RegisterCPU adds a CPU to the monitor and returns its stable ID.
func (m *MachineMonitor) RegisterCPU(label string, cpu DebuggableCPU) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.cpus[id] = &CPUEntry{ID: id, Label: label, CPU: cpu}
	adapterCh := make(chan BreakpointEvent, 8)
	reader := &monitorCPUReader{cancel: make(chan struct{}), done: make(chan struct{})}
	m.cpuReaders[id] = reader
	cpu.SetBreakpointChannel(adapterCh, id)
	if attachable, ok := cpu.(interface{ SetDebugFaultService(*DebugFaultService) }); ok {
		attachable.SetDebugFaultService(m.faults)
	}
	if m.access != nil {
		m.access.RegisterCPU(id, adapterCh)
		m.access.RegisterPCReader(id, cpu.GetPC)
		if stopper, ok := cpu.(interface{ StopForAccessHit() }); ok {
			m.access.RegisterStopper(id, stopper.StopForAccessHit)
		} else {
			m.access.RegisterStopper(id, cpu.Freeze)
		}
	}
	if m.faults != nil {
		m.faults.RegisterCPU(id, adapterCh)
	}
	go func() {
		defer close(reader.done)
		for {
			select {
			case ev := <-adapterCh:
				if !m.listenerActive.Load() {
					select {
					case m.breakpointChan <- ev:
					default:
					}
				}
				m.eventQueue.push(ev)
			case <-reader.cancel:
				return
			}
		}
	}()
	if len(m.cpus) == 1 {
		m.focusedID = id
	}
	m.autoLoadTrustedIEMONRCs()
	return id
}

// ResetCPUs stops all trap-mode goroutines, clears breakpoints, and
// removes all registered CPUs. Must be called before recreating CPUs
// (e.g. F10 reset / IPC mode switch) to prevent stale trapLoop
// goroutines from continuing to step against reset state.
func (m *MachineMonitor) ResetCPUs() {
	m.mu.Lock()
	ids := make([]int, 0, len(m.cpus))
	for id := range m.cpus {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.UnregisterCPU(id)
	}
	m.fireStopHooks(-1, StopReset, 0)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cpus = make(map[int]*CPUEntry)
	m.cpuReaders = make(map[int]*monitorCPUReader)
	m.nextID = 0
	m.focusedID = 0
	m.wasRunning = make(map[int]bool)
	m.tempBreakpoints = make(map[int]map[uint64]bool)
	m.savedConditions = make(map[int]map[uint64]*BreakpointCondition)
	m.runUntilHooks = make(map[int]map[uint64]int)
	m.stepHistory = make(map[int][]*MachineSnapshot)
	m.wholeHistory = nil
	m.loadedRC = make(map[string]string)
	m.clearAssembleModeLocked()
}

// UnregisterCPU removes a CPU by its stable ID.
func (m *MachineMonitor) UnregisterCPU(id int) {
	m.mu.Lock()
	entry, ok := m.cpus[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.cpus, id)
	reader := m.cpuReaders[id]
	delete(m.cpuReaders, id)
	delete(m.stepHistory, id)
	delete(m.tempBreakpoints, id)
	delete(m.savedConditions, id)
	delete(m.runUntilHooks, id)
	if m.focusedID == id {
		m.focusedID = 0 // fall back to primary
	}
	m.mu.Unlock()

	if entry.CPU.IsRunning() {
		entry.CPU.Freeze()
	}
	entry.CPU.SetBreakpointChannel(nil, id)
	if m.access != nil {
		m.access.RegisterCPU(id, nil)
		m.access.ClearWatchesForCPU(id)
	}
	if reader != nil {
		close(reader.cancel)
		<-reader.done
	}
	entry.CPU.ClearAllBreakpoints()
	entry.CPU.ClearAllWatchpoints()
}

// IsActive returns whether the monitor is currently shown.
func (m *MachineMonitor) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == MonitorActive || m.state == MonitorHexEdit
}

// FocusedCPU returns the currently focussed CPU entry, or nil.
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
	m.clearAssembleModeLocked()
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

// RequestBreakIn raises a host-side break request for every currently running
// CPU. Stopped CPUs are left untouched so a break-in request cannot change
// their paused/stopped state when the monitor later deactivates.
func (m *MachineMonitor) RequestBreakIn() {
	m.mu.Lock()
	entries := make([]*CPUEntry, 0, len(m.cpus))
	for _, entry := range m.cpus {
		if entry.CPU.IsRunning() {
			entries = append(entries, entry)
		}
	}
	m.mu.Unlock()

	for _, entry := range entries {
		entry.CPU.RequestBreakIn()
	}
}

// Deactivate resumes previously-running CPUs and exits the monitor.
func (m *MachineMonitor) Deactivate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == MonitorInactive {
		return
	}
	m.state = MonitorInactive
	m.clearAssembleModeLocked()

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
	m.fireStopHooks(-1, StopManualFreeze, 0)
}

// appendOutput adds a line to the scrollback buffer.
func (m *MachineMonitor) appendOutput(text string, color uint32) {
	m.outputLines = append(m.outputLines, OutputLine{Text: text, Color: color})
	if len(m.outputLines) > m.maxOutput {
		m.outputLines = m.outputLines[len(m.outputLines)-m.maxOutput:]
	}
}

// saveCurrentRegs snapshots the focussed CPU's registers for change detection.
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
	m.listenerOnce.Do(func() {
		m.listenerActive.Store(true)
		go func() {
			for ev := range m.breakpointChan {
				m.eventQueue.push(ev)
			}
		}()
		go func() {
			for {
				ev, ok := m.eventQueue.pop()
				if !ok {
					return
				}
				m.handleBreakpointHit(ev)
			}
		}()
	})
}

func (m *MachineMonitor) RegisterStopHook(hk StopHook) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextStopHookID
	m.nextStopHookID++
	m.stopHooks[id] = hk
	return id
}

func (m *MachineMonitor) UnregisterStopHook(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.stopHooks, id)
}

func (m *MachineMonitor) fireStopHooks(cpuID int, reason StopReason, addr uint64) {
	m.mu.Lock()
	hooks := make([]StopHook, 0, len(m.stopHooks))
	for _, hk := range m.stopHooks {
		hooks = append(hooks, hk)
	}
	m.mu.Unlock()
	for _, hk := range hooks {
		hk(cpuID, reason, addr)
	}
}

// handleBreakpointHit freezes all CPUs, focuses on the one that hit,
// and activates the monitor.
func (m *MachineMonitor) handleBreakpointHit(ev BreakpointEvent) {
	reason := StopBreakpoint
	if ev.IsWatch {
		reason = StopWatchpoint
	} else if ev.IsFault {
		reason = StopFault
	}
	m.fireStopHooks(ev.CPUID, reason, ev.Address)

	m.mu.Lock()
	defer m.mu.Unlock()
	if ev.IsBreakIn {
		if entry := m.cpus[ev.CPUID]; entry != nil {
			entry.CPU.ConsumeBreakIn()
		}
	}

	// Snapshot which CPUs are running BEFORE freezing, so Deactivate
	// only resumes CPUs that were genuinely running. The CPU that hit
	// the breakpoint already stopped its own trapLoop, so IsRunning()
	// returns false for it - we record it explicitly below.
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
				entry.CPU.SetBreakpointCondition(ev.Address, cond)
			}
			if hooks := m.runUntilHooks[ev.CPUID]; hooks != nil {
				if hookID, ok := hooks[ev.Address]; ok {
					delete(m.stopHooks, hookID)
					delete(hooks, ev.Address)
				}
				if len(hooks) == 0 {
					delete(m.runUntilHooks, ev.CPUID)
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
	if ev.IsFault {
		msg = fmt.Sprintf("FAULT %s at $%X on %s (id:%d)", ev.FaultKind, ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
		if ev.FaultInfo != "" {
			msg += " " + ev.FaultInfo
		}
	} else if ev.IsGuard {
		msg = fmt.Sprintf("GUARD %s at $%X on %s (id:%d)", accessKindString(ev.Access), ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	} else if ev.IsWatch {
		old := fmt.Sprintf("$%02X", ev.WatchOldValue)
		if !ev.WatchOldValueKnown {
			old = "?"
		}
		msg = fmt.Sprintf("WATCH $%X: %s -> $%02X at PC=$%X on %s (id:%d)",
			ev.WatchAddr, old, ev.WatchNewValue, ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	} else if ev.IsBreakIn {
		msg = fmt.Sprintf("BREAK-IN at $%X on %s (id:%d)", ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	} else {
		msg = fmt.Sprintf("BREAK at $%X on %s (id:%d)", ev.Address, getLabelForCPU(m.cpus, ev.CPUID), ev.CPUID)
	}
	timelineKind := "break"
	if ev.IsFault {
		timelineKind = "fault"
	} else if ev.IsGuard {
		timelineKind = "guard"
	} else if ev.IsWatch {
		timelineKind = "watch"
	} else if ev.IsBreakIn {
		timelineKind = "break-in"
	}
	snapshotID := m.recordWholeMachineHistory()
	m.recordTimelineEventWithSnapshotLocked(timelineKind, ev.CPUID, ev.Address, msg, snapshotID)

	// If already active, just print the message and switch focus
	if m.state == MonitorActive {
		if _, ok := m.cpus[ev.CPUID]; ok {
			m.focusedID = ev.CPUID
		}
		m.appendOutput(msg, colorRed)
		m.saveCurrentRegs()
		m.showRegisters()
		m.showDisassembly(0, 8)
		return
	}

	// Activate the monitor
	m.state = MonitorActive
	m.wasRunning = wasRunning
	if _, ok := m.cpus[ev.CPUID]; ok {
		m.focusedID = ev.CPUID
	}

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
