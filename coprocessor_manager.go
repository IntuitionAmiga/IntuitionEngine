package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CoprocWorker represents a running coprocessor worker.
type CoprocWorker struct {
	cpuType   uint32
	monitorID int           // -1 when not registered with monitor
	debugCPU  DebuggableCPU // retained for monitor access
	loadBase  uint32
	loadEnd   uint32

	mu         sync.Mutex
	stopCPU    func()        // sets running=false on the worker CPU
	execCPU    func()        // architecture-specific run loop (blocks until done)
	disposeCPU func()        // releases an architecture-specific discarded CPU
	done       chan struct{} // closed when current Execute() returns
	frozen     bool

	// gatePending is set while a worker is installed in its slot but has not yet
	// cleared the START-time version handshake. enqueue rejects a pending worker
	// so a concurrent COPROC_CMD_ENQUEUE (issued while START polls with m.mu
	// released) cannot route work to a not-yet-acknowledged worker.
	// gateAcked is set once the worker clears the handshake; enqueue's
	// defence-in-depth re-check only guards workers that once acked, so
	// synthetic workers injected directly by tests are not falsely rejected.
	gatePending bool
	gateAcked   bool

	// Deprecated: kept for compatibility during transition
	stop func()
}

// CoprocCompletion tracks a ticket's completion state.
type CoprocCompletion struct {
	ticket     uint32
	cpuType    uint32 // stored at enqueue time for worker-down checks
	instance   uint32 // worker instance the ticket was enqueued to
	status     uint32
	resultCode uint32
	respLen    uint32
	observed   bool // true after first POLL of terminal state
	created    time.Time
}

type reapedMonitor struct {
	monitor *MachineMonitor
	id      int
}

// CoprocWorkerSlot describes one coprocessor worker slot for monitor display.
type CoprocWorkerSlot struct {
	CPUType   uint32
	Label     string
	Online    bool
	MonitorID int
	CPU       DebuggableCPU
	Path      string
}

type coprocLifecycleError struct {
	code uint32
	err  error
}

func (e *coprocLifecycleError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *coprocLifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func coprocLifecycleErr(code uint32, format string, args ...any) error {
	return &coprocLifecycleError{code: code, err: fmt.Errorf(format, args...)}
}

// busyBucket tracks busy/idle nanoseconds for a 100ms window.
type busyBucket struct {
	busyNs uint64
	idleNs uint64
}

const busyBucketDuration = 100 * time.Millisecond

// CoprocessorManager handles coprocessor MMIO, worker lifecycle, and ticket routing.
type CoprocessorManager struct {
	bus     *MachineBus
	baseDir string
	monitor *MachineMonitor

	mu sync.Mutex
	// workers is indexed [cpuType 1..6][instance 0..1]. The instance-1 slots
	// of single-instance types stay nil.
	workers              [7][2]*CoprocWorker
	nextTicket           uint32
	completions          map[uint32]*CoprocCompletion
	pendingMonitorUnregs []reapedMonitor

	// MMIO shadow registers
	cmd          uint32
	cpuType      uint32
	cmdStatus    uint32
	cmdError     uint32
	ticket       uint32
	ticketStatus uint32
	op           uint32
	reqPtr       uint32
	reqLen       uint32
	respPtr      uint32
	respCap      uint32
	timeout      uint32
	namePtr      uint32
	instance     uint32
	workerState  uint32

	// Stats tracking
	opsDispatched  uint32
	bytesProcessed uint64

	// Completion interrupt support
	completionIRQEnabled atomic.Bool
	completedTicket      atomic.Uint32
	irqTargetCPU         *M68KCPU
	watcherRunning       atomic.Bool
	watcherDone          chan struct{}
	completionWake       chan struct{}
	watcherScanCount     atomic.Uint64

	// Adaptive threshold calibration
	dispatchOverheadNs atomic.Uint64

	// versionGateEnabled enforces the START-time layout-version handshake: a
	// worker that does not echo COPROC_LAYOUT_VERSION into its ring within the
	// timeout is stopped and START fails with COPROC_ERR_STALE_WORKER. Enabled
	// in production (NewCoprocessorManager); unit tests that start raw,
	// non-conforming spin-loop images disable it via SetVersionGateEnabled.
	versionGateEnabled bool
	versionGateTimeout time.Duration

	// ie32DisableJIT is inherited from the process startup policy and applied
	// to every IE32 worker construction.
	ie32DisableJIT bool

	// Worker start time and image path for uptime tracking, per (cpuType, instance)
	workerStartTime [7][2]time.Time
	workerImagePath [7][2]string

	// Rolling busy% tracking (10x100ms buckets = 1 second window)
	busyBuckets       [10]busyBucket
	busyBucketIdx     int
	busyRotateCounter int
	busyBucketStart   time.Time
	lastTransition    time.Time
	workerBusy        bool
}

// NewCoprocessorManager creates a new coprocessor manager.
func NewCoprocessorManager(bus *MachineBus, baseDir string) *CoprocessorManager {
	mgr := &CoprocessorManager{
		bus:                bus,
		baseDir:            baseDir,
		nextTicket:         1,
		completions:        make(map[uint32]*CoprocCompletion),
		completionWake:     make(chan struct{}, 1),
		versionGateEnabled: true,
		versionGateTimeout: 100 * time.Millisecond,
	}
	if bus != nil {
		bus.RegisterCoprocessorCompletionWake(mgr.handleCompletionStatusWrite)
	}
	mgr.initRings()
	return mgr
}

func (m *CoprocessorManager) SetIE32JITDisabled(disabled bool) {
	m.mu.Lock()
	m.ie32DisableJIT = disabled
	m.mu.Unlock()
}

// initRings clears mailbox descriptors and initializes all ring headers.
// Caller may hold m.mu; bus writes do not re-enter the manager lock.
func (m *CoprocessorManager) initRings() {
	for i := range COPROC_RING_COUNT {
		base := ringBaseAddr(i)
		for off := uint32(RING_ENTRIES_OFFSET); off < RING_RESPONSES_OFFSET+uint32(RING_CAPACITY)*RESP_DESC_SIZE; off++ {
			m.bus.Write8(base+off, 0)
		}
		m.bus.Write8(base+RING_HEAD_OFFSET, 0)
		m.bus.Write8(base+RING_TAIL_OFFSET, 0)
		m.bus.Write8(base+RING_CAPACITY_OFFSET, RING_CAPACITY)
		// Version-gate handshake: publish the layout version, clear the ack. A
		// conforming worker echoes the version into RING_ACK_VERSION_OFFSET at
		// startup; startWorkerLocked refuses to route work otherwise.
		m.bus.Write8(base+RING_LAYOUT_VERSION_OFFSET, COPROC_LAYOUT_VERSION)
		m.bus.Write8(base+RING_ACK_VERSION_OFFSET, 0)
	}
}

// workerSlotLocked returns the worker slot for (cpuType, instance), or nil
// if the combination is unsupported. Caller holds m.mu.
func (m *CoprocessorManager) workerSlotLocked(cpuType, instance uint32) **CoprocWorker {
	if cpuType < 1 || cpuType > 6 || instance >= coprocInstanceLimit(cpuType) {
		return nil
	}
	return &m.workers[cpuType][instance]
}

func (m *CoprocessorManager) workerAtLocked(cpuType, instance uint32) *CoprocWorker {
	slot := m.workerSlotLocked(cpuType, instance)
	if slot == nil {
		return nil
	}
	return *slot
}

// forEachWorkerSlotLocked visits every (cpuType, instance) worker slot.
func (m *CoprocessorManager) forEachWorkerSlotLocked(fn func(cpuType, instance uint32, slot **CoprocWorker)) {
	for cpuType := uint32(1); cpuType <= 6; cpuType++ {
		for inst := uint32(0); inst < coprocInstanceLimit(cpuType); inst++ {
			fn(cpuType, inst, m.workerSlotLocked(cpuType, inst))
		}
	}
}

func (m *CoprocessorManager) setWorkerMetaLocked(cpuType, instance uint32, start time.Time, path string) {
	if cpuType < 1 || cpuType > 6 || instance > 1 {
		return
	}
	m.workerStartTime[cpuType][instance] = start
	m.workerImagePath[cpuType][instance] = path
}

func (m *CoprocessorManager) workerStartTimeAt(cpuType, instance uint32) time.Time {
	if cpuType < 1 || cpuType > 6 || instance > 1 {
		return time.Time{}
	}
	return m.workerStartTime[cpuType][instance]
}

// SetIRQTarget sets the M68K CPU that receives completion interrupts.
func (m *CoprocessorManager) SetIRQTarget(cpu *M68KCPU) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.irqTargetCPU = cpu
}

// StartCompletionWatcher launches the background goroutine that monitors
// ring buffers for completed tickets and fires M68K interrupts.
func (m *CoprocessorManager) StartCompletionWatcher() {
	if m.watcherRunning.Swap(true) {
		return
	}
	m.watcherDone = make(chan struct{})
	go m.completionWatcherLoop()
}

// StopCompletionWatcher stops the background watcher goroutine.
func (m *CoprocessorManager) StopCompletionWatcher() {
	if !m.watcherRunning.Swap(false) {
		return
	}
	m.signalCompletionWake()
	<-m.watcherDone
}

func (m *CoprocessorManager) signalCompletionWake() {
	if m == nil || m.completionWake == nil {
		return
	}
	select {
	case m.completionWake <- struct{}{}:
	default:
	}
}

func (m *CoprocessorManager) handleCompletionStatusWrite(addr, value uint32) {
	if value == COPROC_TICKET_PENDING || value == COPROC_TICKET_RUNNING {
		return
	}
	if !isCoprocResponseStatusAddr(addr) {
		return
	}
	m.signalCompletionWake()
}

func isCoprocResponseStatusAddr(addr uint32) bool {
	for i := range COPROC_RING_COUNT {
		base := ringBaseAddr(i) + RING_RESPONSES_OFFSET
		if addr < base+RESP_STATUS_OFF {
			continue
		}
		off := addr - base
		if off >= uint32(RING_CAPACITY)*RESP_DESC_SIZE {
			continue
		}
		if off%RESP_DESC_SIZE == RESP_STATUS_OFF {
			return true
		}
	}
	return false
}

func (m *CoprocessorManager) completionWatcherLoop() {
	defer close(m.watcherDone)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.completionWake:
		case <-ticker.C:
		}
		if !m.watcherRunning.Load() {
			return
		}
		m.scanForCompletions()
		m.rotateBusyBuckets()
	}
}

func (m *CoprocessorManager) scanForCompletions() {
	m.watcherScanCount.Add(1)
	m.mu.Lock()
	m.reapDeadWorkersLocked()

	anyPending := false
	for ticket, comp := range m.completions {
		if comp.status != COPROC_TICKET_PENDING && comp.status != COPROC_TICKET_RUNNING {
			continue
		}
		status := m.scanTicketStatus(ticket)
		if status == COPROC_TICKET_PENDING || status == COPROC_TICKET_RUNNING {
			anyPending = true
			continue
		}
		comp.status = status
		m.completedTicket.Store(ticket)
		if m.completionIRQEnabled.Load() && m.irqTargetCPU != nil {
			m.irqTargetCPU.AssertInterrupt(6)
		}
	}

	// Transition busy -> idle when all completions are resolved
	if !anyPending && m.workerBusy {
		m.transitionBusyIdleLocked()
	}
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
}

func (m *CoprocessorManager) rotateBusyBuckets() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rotateBusyBucketsAtLocked(time.Now())
}

func (m *CoprocessorManager) rotateBusyBucketsAtLocked(now time.Time) {
	if m.busyBucketStart.IsZero() {
		m.busyBucketStart = now
		return
	}
	if now.Before(m.busyBucketStart) {
		m.busyBucketStart = now
		if !m.lastTransition.IsZero() && m.lastTransition.After(now) {
			m.lastTransition = now
		}
		return
	}
	for {
		boundary := m.busyBucketStart.Add(busyBucketDuration)
		if now.Before(boundary) {
			return
		}
		m.accrueBusyTimeLocked(boundary)
		m.busyBucketIdx = (m.busyBucketIdx + 1) % len(m.busyBuckets)
		m.busyBuckets[m.busyBucketIdx] = busyBucket{}
		m.busyBucketStart = boundary
		m.busyRotateCounter++
	}
}

func (m *CoprocessorManager) accrueBusyTimeLocked(until time.Time) {
	if m.lastTransition.IsZero() || until.Before(m.lastTransition) {
		return
	}
	elapsed := uint64(until.Sub(m.lastTransition).Nanoseconds())
	if m.workerBusy {
		m.busyBuckets[m.busyBucketIdx].busyNs += elapsed
	} else {
		m.busyBuckets[m.busyBucketIdx].idleNs += elapsed
	}
	m.lastTransition = until
}

func (m *CoprocessorManager) reapDeadWorkersLocked() {
	m.forEachWorkerSlotLocked(func(cpuType, instance uint32, slot **CoprocWorker) {
		w := *slot
		if w == nil || w.done == nil {
			return
		}
		select {
		case <-w.done:
			w.mu.Lock()
			frozen := w.frozen
			w.mu.Unlock()
			if frozen {
				return
			}
			*slot = nil
			m.setWorkerMetaLocked(cpuType, instance, time.Time{}, "")
			m.markWorkerDownCompletionsLocked(cpuType, instance)
			if m.monitor != nil && w.monitorID >= 0 {
				m.pendingMonitorUnregs = append(m.pendingMonitorUnregs, reapedMonitor{
					monitor: m.monitor,
					id:      w.monitorID,
				})
			}
		default:
		}
	})
}

func (m *CoprocessorManager) markWorkerDownCompletionsLocked(cpuType, instance uint32) {
	for _, comp := range m.completions {
		if comp.cpuType != cpuType || comp.instance != instance {
			continue
		}
		if comp.status == COPROC_TICKET_PENDING || comp.status == COPROC_TICKET_RUNNING {
			status := m.scanTicketStatus(comp.ticket)
			if status == COPROC_TICKET_PENDING || status == COPROC_TICKET_RUNNING {
				status = COPROC_TICKET_WORKER_DOWN
			}
			comp.status = status
			m.completedTicket.Store(comp.ticket)
			if m.completionIRQEnabled.Load() && m.irqTargetCPU != nil {
				m.irqTargetCPU.AssertInterrupt(6)
			}
		}
	}
}

func (m *CoprocessorManager) drainPendingUnregsLocked() []reapedMonitor {
	drained := m.pendingMonitorUnregs
	m.pendingMonitorUnregs = nil
	return drained
}

func (m *CoprocessorManager) flushReaped(reaped []reapedMonitor) {
	for _, r := range reaped {
		if r.monitor != nil && r.id >= 0 {
			r.monitor.UnregisterCPU(r.id)
		}
	}
}

func (m *CoprocessorManager) transitionBusyIdleLocked() {
	now := time.Now()
	m.accrueBusyTimeLocked(now)
	m.lastTransition = now
	m.workerBusy = false
}

func (m *CoprocessorManager) maybeFlushBusyIdleLocked() {
	if !m.workerBusy {
		return
	}
	for _, comp := range m.completions {
		if comp.status == COPROC_TICKET_PENDING || comp.status == COPROC_TICKET_RUNNING {
			return
		}
	}
	m.transitionBusyIdleLocked()
}

// readReg returns the shadow register value for a given aligned register base address.
func (m *CoprocessorManager) readReg(regBase uint32) uint32 {
	switch regBase {
	case COPROC_CMD:
		return m.cmd
	case COPROC_CPU_TYPE:
		return m.cpuType
	case COPROC_CMD_STATUS:
		return m.cmdStatus
	case COPROC_CMD_ERROR:
		return m.cmdError
	case COPROC_TICKET:
		return m.ticket
	case COPROC_TICKET_STATUS:
		return m.ticketStatus
	case COPROC_OP:
		return m.op
	case COPROC_REQ_PTR:
		return m.reqPtr
	case COPROC_REQ_LEN:
		return m.reqLen
	case COPROC_RESP_PTR:
		return m.respPtr
	case COPROC_RESP_CAP:
		return m.respCap
	case COPROC_TIMEOUT:
		return m.timeout
	case COPROC_NAME_PTR:
		return m.namePtr
	case COPROC_INSTANCE:
		return m.instance
	case COPROC_WORKER_STATE:
		return m.computeWorkerState()
	case COPROC_STATS_OPS:
		return m.opsDispatched
	case COPROC_STATS_BYTES:
		return uint32(m.bytesProcessed)
	case COPROC_IRQ_CTRL:
		if m.completionIRQEnabled.Load() {
			return 1
		}
		return 0
	case COPROC_DISPATCH_OVERHEAD:
		return uint32(m.dispatchOverheadNs.Load())
	case COPROC_COMPLETED_TICKET:
		return m.completedTicket.Load()
	case COPROC_RING_DEPTH:
		typeIdx, cpuType := m.selectedMonitorCPUIndexLocked()
		if typeIdx < 0 {
			return 0
		}
		ringIdx := coprocRingIndex(cpuType, m.instance)
		if ringIdx < 0 {
			return 0
		}
		ringBase := ringBaseAddr(ringIdx)
		head := uint32(m.bus.Read8(ringBase + RING_HEAD_OFFSET))
		tail := uint32(m.bus.Read8(ringBase + RING_TAIL_OFFSET))
		cap := uint32(m.bus.Read8(ringBase + RING_CAPACITY_OFFSET))
		if cap == 0 {
			return 0
		}
		return (head - tail + cap) % cap
	case COPROC_WORKER_UPTIME:
		_, cpuType := m.selectedMonitorCPUIndexLocked()
		if cpuType < 1 || cpuType > 6 {
			return 0
		}
		start := m.workerStartTimeAt(cpuType, m.instance)
		if m.workerAtLocked(cpuType, m.instance) != nil && !start.IsZero() {
			return uint32(time.Since(start).Seconds())
		}
		return 0
	case COPROC_BUSY_PCT:
		return m.computeBusyPct()
	case COPROC_STATS_RESET:
		return 0
	case COPROC_INSTANCE_LIMIT:
		return coprocInstanceLimit(m.cpuType)
	case COPROC_SELECTED_STATE:
		return m.computeSelectedState()
	case COPROC_MAILBOX_VERSION:
		return COPROC_LAYOUT_VERSION
	case COPROC_WORKER_BASE:
		if base, _, _, ok := workerWindow(m.cpuType, m.instance); ok {
			return base
		}
		return 0
	case COPROC_WORKER_END:
		if _, end, _, ok := workerWindow(m.cpuType, m.instance); ok {
			return end
		}
		return 0
	case COPROC_WORKER_RING:
		if idx := coprocRingIndex(m.cpuType, m.instance); idx >= 0 {
			return ringBaseAddr(idx)
		}
		return 0
	case COPROC_INSTANCE_STATE:
		return m.computeInstanceStateMask()
	default:
		return 0
	}
}

func (m *CoprocessorManager) selectedMonitorCPUIndexLocked() (int, uint32) {
	if idx := cpuTypeToIndex(m.cpuType); idx >= 0 {
		return idx, m.cpuType
	}
	for cpuType := uint32(1); cpuType <= 6; cpuType++ {
		if m.workers[cpuType][0] != nil {
			return cpuTypeToIndex(cpuType), cpuType
		}
	}
	return -1, 0
}

// writeReg sets a shadow register value for a given aligned register base address.
func (m *CoprocessorManager) writeReg(regBase, val uint32) {
	switch regBase {
	case COPROC_CMD:
		m.cmd = val
	case COPROC_CPU_TYPE:
		m.cpuType = val
	case COPROC_TICKET:
		m.ticket = val
	case COPROC_OP:
		m.op = val
	case COPROC_REQ_PTR:
		m.reqPtr = val
	case COPROC_REQ_LEN:
		m.reqLen = val
	case COPROC_RESP_PTR:
		m.respPtr = val
	case COPROC_RESP_CAP:
		m.respCap = val
	case COPROC_TIMEOUT:
		m.timeout = val
	case COPROC_NAME_PTR:
		m.namePtr = val
	case COPROC_INSTANCE:
		m.instance = val
	case COPROC_IRQ_CTRL:
		m.completionIRQEnabled.Store(val&1 != 0)
	case COPROC_STATS_RESET:
		if val == 1 {
			now := time.Now()
			m.opsDispatched = 0
			m.bytesProcessed = 0
			m.busyBuckets = [10]busyBucket{}
			m.busyBucketIdx = 0
			m.busyRotateCounter = 0
			m.busyBucketStart = now
			// Reset transition state so busy% starts fresh from this point
			m.lastTransition = now
			// workerBusy keeps its current value — if the worker IS busy,
			// we just reset the epoch so elapsed time accrues from now.
		}
	}
}

// HandleRead reads an MMIO register. Supports both aligned 32-bit reads
// and byte-level reads at sub-register offsets (for 8-bit CPUs).
func (m *CoprocessorManager) HandleRead(addr uint32) uint32 {
	m.mu.Lock()

	offset := addr - COPROC_BASE
	regBase := COPROC_BASE + (offset & ^uint32(3))
	byteOff := offset & 3
	val := m.readReg(regBase)
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
	if byteOff != 0 {
		return (val >> (byteOff * 8)) & 0xFF
	}
	return val
}

// HandleWrite writes an MMIO register. Supports both aligned 32-bit writes
// and byte-level writes at sub-register offsets (for 8-bit CPUs).
// Writing to COPROC_CMD byte 0 triggers command dispatch.
func (m *CoprocessorManager) HandleWrite(addr uint32, val uint32) {
	m.mu.Lock()

	offset := addr - COPROC_BASE
	regBase := COPROC_BASE + (offset & ^uint32(3))
	byteOff := offset & 3

	if byteOff != 0 {
		// Byte-level write: read-modify-write into the aligned register
		existing := m.readReg(regBase)
		shift := byteOff * 8
		val = (existing & ^(uint32(0xFF) << shift)) | ((val & 0xFF) << shift)
	}

	m.writeReg(regBase, val)

	// Only dispatch when byte 0 of COPROC_CMD is written
	if regBase == COPROC_CMD && byteOff == 0 {
		m.dispatchCmd()
	}
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
}

func (m *CoprocessorManager) dispatchCmd() {
	switch m.cmd {
	case COPROC_CMD_START:
		m.cmdStart()
	case COPROC_CMD_START_MEM:
		m.cmdStartMem()
	case COPROC_CMD_STOP:
		m.cmdStop()
	case COPROC_CMD_ENQUEUE:
		m.cmdEnqueue()
	case COPROC_CMD_POLL:
		m.cmdPoll()
	case COPROC_CMD_WAIT:
		m.cmdWait()
	default:
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_NONE
	}
}

func (m *CoprocessorManager) cmdStart() {
	cpuType := m.cpuType
	filename := m.readFileName(m.namePtr)
	err := m.startWorkerLocked(cpuType, m.instance, filename, true)
	m.setCmdResultFromLifecycleErr(err)

	// Auto-calibrate dispatch overhead on first IE64 worker start
	if err == nil && cpuType == EXEC_TYPE_IE64 && m.dispatchOverheadNs.Load() == 0 {
		go m.calibrateDispatchOverhead()
	}
}

// cmdStartMem starts a worker from a service image in guest RAM:
// COPROC_REQ_PTR/COPROC_REQ_LEN describe the blob. Self-contained
// program images use this to start embedded services with no host
// filesystem access.
func (m *CoprocessorManager) cmdStartMem() {
	cpuType := m.cpuType
	blobPtr := m.reqPtr
	blobLen := m.reqLen
	mem := m.bus.GetMemory()
	end := uint64(blobPtr) + uint64(blobLen)
	if blobLen == 0 || end > uint64(len(mem)) {
		m.setCmdResultFromLifecycleErr(coprocLifecycleErr(COPROC_ERR_NOT_FOUND,
			"service blob out of range: ptr=%#x len=%d", blobPtr, blobLen))
		return
	}
	data := make([]byte, blobLen)
	copy(data, mem[blobPtr:end])
	err := m.startWorkerFromDataLocked(cpuType, m.instance, "guest-ram", data, true)
	m.setCmdResultFromLifecycleErr(err)

	if err == nil && cpuType == EXEC_TYPE_IE64 && m.dispatchOverheadNs.Load() == 0 {
		go m.calibrateDispatchOverhead()
	}
}

// calibrateDispatchOverhead measures the round-trip time for a no-op
// coprocessor request (op=0) and stores the result in dispatchOverheadNs.
// Called once on the first IE64 worker start.
func (m *CoprocessorManager) calibrateDispatchOverhead() {
	// Give the worker goroutine a moment to start executing
	time.Sleep(5 * time.Millisecond)

	start := time.Now()

	// Enqueue NOP under lock, then immediately release. The shadow registers
	// are guest-visible command state: a guest staging its own command (or a
	// non-default COPROC_INSTANCE selection) while this async calibration
	// runs must not have it redirected, so save and restore everything the
	// enqueue mutates before dropping the lock. The ticket is polled via
	// scanTicketStatus afterwards (lock-free ring scan).
	m.mu.Lock()
	saved := struct {
		cpuType, instance, op, reqPtr, reqLen, respPtr, respCap uint32
		ticket, cmdStatus, cmdError                             uint32
	}{
		m.cpuType, m.instance, m.op, m.reqPtr, m.reqLen, m.respPtr, m.respCap,
		m.ticket, m.cmdStatus, m.cmdError,
	}
	m.cpuType = EXEC_TYPE_IE64
	m.instance = 0
	m.op = 0 // NOP
	m.reqPtr = 0
	m.reqLen = 0
	m.respPtr = 0
	m.respCap = 0
	m.cmdEnqueue()
	ticket := m.ticket
	ok := m.cmdStatus == COPROC_STATUS_OK
	m.cpuType, m.instance, m.op, m.reqPtr, m.reqLen, m.respPtr, m.respCap =
		saved.cpuType, saved.instance, saved.op, saved.reqPtr, saved.reqLen, saved.respPtr, saved.respCap
	m.ticket, m.cmdStatus, m.cmdError = saved.ticket, saved.cmdStatus, saved.cmdError
	m.mu.Unlock()

	if !ok || ticket == 0 {
		return
	}

	// Poll directly without touching shadow registers — avoids race with
	// concurrent MMIO writes from M68K while we hold/release mu.
	deadline := time.Now().Add(1 * time.Second)
	for {
		status := m.scanTicketStatus(ticket)
		if status != COPROC_TICKET_PENDING && status != COPROC_TICKET_RUNNING {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}

	elapsed := time.Since(start)
	m.dispatchOverheadNs.Store(uint64(elapsed.Nanoseconds()))
}

func (m *CoprocessorManager) cmdStop() {
	m.setCmdResultFromLifecycleErr(m.stopWorkerLocked(m.cpuType, m.instance))
}

func (m *CoprocessorManager) cmdEnqueue() {
	m.reapDeadWorkersLocked()
	cpuIdx := coprocRingIndex(m.cpuType, m.instance)
	if cpuIdx < 0 {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = coprocSelectionError(m.cpuType, m.instance)
		return
	}

	worker := m.workerAtLocked(m.cpuType, m.instance)
	if worker == nil {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_NO_WORKER
		return
	}

	// Defence in depth: if a gated worker's version ack regressed, refuse to
	// enqueue. Only guards workers that cleared the START handshake, so
	// directly-injected synthetic workers are unaffected.
	if m.versionGateEnabled {
		worker.mu.Lock()
		pending := worker.gatePending
		gated := worker.gateAcked
		worker.mu.Unlock()
		if pending {
			// Worker is installed but has not cleared the START handshake yet:
			// never route work to it.
			m.ticket = 0
			m.cmdStatus = COPROC_STATUS_ERROR
			m.cmdError = COPROC_ERR_STALE_WORKER
			return
		}
		if gated {
			ackAddr := ringBaseAddr(cpuIdx) + RING_ACK_VERSION_OFFSET
			if uint32(m.bus.Read8(ackAddr)) != COPROC_LAYOUT_VERSION {
				m.ticket = 0
				m.cmdStatus = COPROC_STATUS_ERROR
				m.cmdError = COPROC_ERR_STALE_WORKER
				return
			}
		}
	}

	// Prune stale completions
	m.pruneCompletions()

	// Check ring capacity
	ringBase := ringBaseAddr(cpuIdx)
	head := m.bus.Read8(ringBase + RING_HEAD_OFFSET)
	capacity := m.bus.Read8(ringBase + RING_CAPACITY_OFFSET)
	if capacity == 0 {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_QUEUE_FULL
		return
	}
	nextHead := (head + 1) % capacity
	tail := m.bus.Read8(ringBase + RING_TAIL_OFFSET)
	if nextHead == tail {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_QUEUE_FULL
		return
	}

	// Allocate ticket
	if m.nextTicket == 0 {
		m.nextTicket = 1
	}
	ticket := m.nextTicket
	m.nextTicket++
	if m.nextTicket == 0 {
		m.nextTicket = 1
	}

	// Write request descriptor at entries[head]
	entryAddr := ringBase + RING_ENTRIES_OFFSET + uint32(head)*REQ_DESC_SIZE
	m.bus.Write32(entryAddr+REQ_TICKET_OFF, ticket)
	m.bus.Write32(entryAddr+REQ_CPU_TYPE_OFF, m.cpuType)
	m.bus.Write32(entryAddr+REQ_OP_OFF, m.op)
	m.bus.Write32(entryAddr+REQ_TIMEOUT_OFF, m.timeout)
	m.bus.Write32(entryAddr+REQ_REQ_PTR_OFF, m.reqPtr)
	m.bus.Write32(entryAddr+REQ_REQ_LEN_OFF, m.reqLen)
	m.bus.Write32(entryAddr+REQ_RESP_PTR_OFF, m.respPtr)
	m.bus.Write32(entryAddr+REQ_RESP_CAP_OFF, m.respCap)

	// Initialize response descriptor as pending
	respAddr := ringBase + RING_RESPONSES_OFFSET + uint32(head)*RESP_DESC_SIZE
	m.bus.Write32(respAddr+RESP_TICKET_OFF, ticket)
	m.bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_PENDING)
	m.bus.Write32(respAddr+RESP_RESULT_CODE_OFF, 0)
	m.bus.Write32(respAddr+RESP_RESP_LEN_OFF, 0)

	// Advance head
	m.bus.Write8(ringBase+RING_HEAD_OFFSET, nextHead)

	// Track completion
	m.completions[ticket] = &CoprocCompletion{
		ticket:   ticket,
		cpuType:  m.cpuType,
		instance: m.instance,
		status:   COPROC_TICKET_PENDING,
		created:  time.Now(),
	}

	// Track stats
	m.opsDispatched++
	m.bytesProcessed += uint64(m.reqLen)

	// Track busy transition for busy% computation
	if !m.workerBusy {
		now := time.Now()
		if m.busyBucketStart.IsZero() {
			m.busyBucketStart = now
		}
		m.lastTransition = now
		m.workerBusy = true
	}

	m.ticket = ticket
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdPoll() {
	m.reapDeadWorkersLocked()
	ticket := m.ticket
	// Ticket 0 is the "already complete" sentinel (fallback path)
	if ticket == 0 {
		m.ticketStatus = COPROC_TICKET_OK
		m.cmdStatus = COPROC_STATUS_OK
		m.cmdError = COPROC_ERR_NONE
		return
	}
	comp, ok := m.completions[ticket]
	if !ok {
		m.ticketStatus = COPROC_TICKET_ERROR
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_STALE_TICKET
		return
	}

	status := comp.status

	// If already in a terminal state (cached from previous poll/wait), use it
	if status == COPROC_TICKET_PENDING || status == COPROC_TICKET_RUNNING {
		// Not yet terminal - scan ring to discover new state
		status = m.scanTicketStatus(ticket)
		if status == COPROC_TICKET_PENDING || status == COPROC_TICKET_RUNNING {
			// Still non-terminal - check if worker is down
			ct := comp.cpuType
			if ct >= 1 && ct <= 6 && m.workerAtLocked(ct, comp.instance) == nil {
				status = COPROC_TICKET_WORKER_DOWN
			}
		}
	}

	if status != COPROC_TICKET_PENDING && status != COPROC_TICKET_RUNNING {
		// Terminal state - handle two-read eviction
		comp.status = status
		if comp.observed {
			delete(m.completions, ticket)
		} else {
			comp.observed = true
		}
	}
	m.maybeFlushBusyIdleLocked()

	m.ticketStatus = status
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdWait() {
	m.reapDeadWorkersLocked()
	ticket := m.ticket
	// Ticket 0 is the "already complete" sentinel (fallback path)
	if ticket == 0 {
		m.ticketStatus = COPROC_TICKET_OK
		m.cmdStatus = COPROC_STATUS_OK
		m.cmdError = COPROC_ERR_NONE
		return
	}
	timeoutMs := m.timeout
	if timeoutMs == 0 {
		timeoutMs = 1000
	}

	comp, ok := m.completions[ticket]
	if !ok {
		m.ticketStatus = COPROC_TICKET_ERROR
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_STALE_TICKET
		return
	}

	// Already terminal? Return immediately.
	if comp.status != COPROC_TICKET_PENDING && comp.status != COPROC_TICKET_RUNNING {
		m.ticketStatus = comp.status
		m.cmdStatus = COPROC_STATUS_OK
		m.cmdError = COPROC_ERR_NONE
		m.maybeFlushBusyIdleLocked()
		return
	}

	// Release lock while waiting
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	var status uint32
	for {
		status = m.scanTicketStatus(ticket)
		if status != COPROC_TICKET_PENDING && status != COPROC_TICKET_RUNNING {
			break
		}
		m.mu.Lock()
		m.reapDeadWorkersLocked()
		down := false
		if comp.cpuType >= 1 && comp.cpuType <= 6 {
			down = m.workerAtLocked(comp.cpuType, comp.instance) == nil
		}
		drained := m.drainPendingUnregsLocked()
		m.mu.Unlock()
		m.flushReaped(drained)
		if down {
			status = COPROC_TICKET_WORKER_DOWN
			break
		}
		if time.Now().After(deadline) {
			status = COPROC_TICKET_TIMEOUT
			break
		}
		time.Sleep(100 * time.Microsecond)
	}

	m.mu.Lock()
	comp.status = status
	m.maybeFlushBusyIdleLocked()
	m.ticketStatus = status
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

// scanTicketStatus scans all ring response slots to find the status for a ticket.
func (m *CoprocessorManager) scanTicketStatus(ticket uint32) uint32 {
	for i := range COPROC_RING_COUNT {
		ringBase := ringBaseAddr(i)
		for slot := range uint32(RING_CAPACITY) {
			respAddr := ringBase + RING_RESPONSES_OFFSET + slot*RESP_DESC_SIZE
			t := m.bus.Read32(respAddr + RESP_TICKET_OFF)
			if t == ticket {
				return m.bus.Read32(respAddr + RESP_STATUS_OFF)
			}
		}
	}
	return COPROC_TICKET_PENDING
}

// IsWorkerRunning returns true if the given EXEC_TYPE_* worker is active.
func (m *CoprocessorManager) IsWorkerRunning(cpuType uint32) bool {
	m.mu.Lock()
	m.reapDeadWorkersLocked()
	running := false
	if cpuType >= 1 && cpuType <= 6 {
		for inst := uint32(0); inst < coprocInstanceLimit(cpuType); inst++ {
			if m.workers[cpuType][inst] != nil {
				running = true
				break
			}
		}
	}
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
	return running
}

// computeWorkerState returns the aggregate per-type running bitmask: bit N
// (1..6) is set when ANY instance of that cpuType is online. Per-instance state
// is reported separately through COPROC_SELECTED_STATE (computeSelectedState).
func (m *CoprocessorManager) computeWorkerState() uint32 {
	m.reapDeadWorkersLocked()
	var state uint32
	for i := uint32(1); i <= 6; i++ {
		for inst := uint32(0); inst < coprocInstanceLimit(i); inst++ {
			if m.workers[i][inst] != nil {
				state |= 1 << i
				break
			}
		}
	}
	return state
}

// computeInstanceStateMask returns a bitmask with bit (cpuType*2 + instance)
// set for every running worker. It reads no command-selection state, so a
// consumer can enumerate all instances with a single atomic read without
// disturbing COPROC_CPU_TYPE / COPROC_INSTANCE.
func (m *CoprocessorManager) computeInstanceStateMask() uint32 {
	m.reapDeadWorkersLocked()
	var mask uint32
	for cpuType := uint32(1); cpuType <= 6; cpuType++ {
		for inst := uint32(0); inst < coprocInstanceLimit(cpuType); inst++ {
			if m.workers[cpuType][inst] != nil {
				mask |= 1 << (cpuType*2 + inst)
			}
		}
	}
	return mask
}

// computeSelectedState returns 1 when the worker selected by the current
// COPROC_CPU_TYPE / COPROC_INSTANCE shadow registers is online, else 0. It
// replaces the retired COPROC_WORKER_STATE bit-7 overload.
func (m *CoprocessorManager) computeSelectedState() uint32 {
	m.reapDeadWorkersLocked()
	if m.workerAtLocked(m.cpuType, m.instance) != nil {
		return 1
	}
	return 0
}

func (m *CoprocessorManager) computeBusyPct() uint32 {
	var totalBusy, totalIdle uint64
	for i := range 10 {
		totalBusy += m.busyBuckets[i].busyNs
		totalIdle += m.busyBuckets[i].idleNs
	}
	total := totalBusy + totalIdle
	if total == 0 {
		return 0
	}
	return uint32(totalBusy * 100 / total)
}

func (m *CoprocessorManager) readFileName(ptr uint32) string {
	var name []byte
	addr := ptr
	for {
		b := m.bus.Read8(addr)
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

func (m *CoprocessorManager) sanitizePath(path string) (string, bool) {
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", false
	}
	fullPath := filepath.Join(m.baseDir, path)
	rel, err := filepath.Rel(m.baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return fullPath, true
}

func (m *CoprocessorManager) setCmdResultFromLifecycleErr(err error) {
	if err == nil {
		m.cmdStatus = COPROC_STATUS_OK
		m.cmdError = COPROC_ERR_NONE
		return
	}
	m.cmdStatus = COPROC_STATUS_ERROR
	if le, ok := err.(*coprocLifecycleError); ok {
		m.cmdError = le.code
	} else {
		m.cmdError = COPROC_ERR_LOAD_FAILED
	}
}

func inferCoprocCPUTypeFromImagePath(path string) uint32 {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ie64":
		return EXEC_TYPE_IE64
	case ".iex", ".ie32":
		return EXEC_TYPE_IE32
	case ".ie68":
		return EXEC_TYPE_M68K
	case ".ie80":
		return EXEC_TYPE_Z80
	case ".ie65":
		return EXEC_TYPE_6502
	case ".ie86":
		return EXEC_TYPE_X86
	default:
		return EXEC_TYPE_NONE
	}
}

func (m *CoprocessorManager) stagedServicePathLocked() string {
	if m.namePtr == 0 {
		return ""
	}
	return strings.TrimSpace(m.readFileName(m.namePtr))
}

// StagedServicePath returns the currently staged COPROC_NAME_PTR service path.
func (m *CoprocessorManager) StagedServicePath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stagedServicePathLocked()
}

// StartWorkerFromStaged starts a coprocessor worker from the currently staged
// COPROC_NAME_PTR service path. It is intended for IEMon, where duplicate
// starts are rejected unless replace is true.
func (m *CoprocessorManager) StartWorkerFromStaged(cpuType uint32, replace bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startWorkerLocked(cpuType, 0, m.stagedServicePathLocked(), replace)
}

// StartWorkerFromImage starts a coprocessor worker from an explicit relative
// image path. When cpuType is EXEC_TYPE_NONE, the type is inferred from the
// typed .ie* extension.
func (m *CoprocessorManager) StartWorkerFromImage(cpuType uint32, path string, replace bool) (uint32, error) {
	inferred := inferCoprocCPUTypeFromImagePath(path)
	if inferred == EXEC_TYPE_NONE {
		return 0, coprocLifecycleErr(COPROC_ERR_PATH_INVALID, "unsupported coprocessor image extension: %s", path)
	}
	if cpuType == EXEC_TYPE_NONE {
		cpuType = inferred
	} else if cpuType != inferred {
		return 0, coprocLifecycleErr(COPROC_ERR_INVALID_CPU, "CPU type %s does not match image extension for %s", coprocCPUTypeToString(cpuType), path)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cpuType, m.startWorkerLocked(cpuType, 0, path, replace)
}

func (m *CoprocessorManager) startWorkerLocked(cpuType, instance uint32, filename string, replace bool) error {
	if code := coprocSelectionError(cpuType, instance); code != COPROC_ERR_NONE {
		return coprocLifecycleErr(code, "invalid coprocessor CPU type %d instance %d", cpuType, instance)
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return coprocLifecycleErr(COPROC_ERR_NOT_FOUND, "no staged coprocessor service image for %s", coprocCPUTypeToString(cpuType))
	}
	if inferred := inferCoprocCPUTypeFromImagePath(filename); inferred != EXEC_TYPE_NONE && inferred != cpuType {
		return coprocLifecycleErr(COPROC_ERR_INVALID_CPU, "CPU type %s does not match image extension for %s", coprocCPUTypeToString(cpuType), filename)
	}
	fullPath, ok := m.sanitizePath(filename)
	if !ok {
		return coprocLifecycleErr(COPROC_ERR_PATH_INVALID, "invalid coprocessor service path: %s", filename)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return coprocLifecycleErr(COPROC_ERR_NOT_FOUND, "coprocessor service %q is not readable: %w", filename, err)
	}
	return m.startWorkerFromDataLocked(cpuType, instance, filename, data, replace)
}

// startWorkerFromDataLocked runs the shared worker lifecycle for a
// service image already in memory - either read from a file or handed
// over directly from guest RAM (COPROC_CMD_START_MEM), which is how a
// self-contained program image starts its embedded services without
// any host filesystem.
func (m *CoprocessorManager) startWorkerFromDataLocked(cpuType, instance uint32, label string, data []byte, replace bool) error {
	filename := label
	slot := m.workerSlotLocked(cpuType, instance)
	if slot == nil {
		return coprocLifecycleErr(coprocSelectionError(cpuType, instance), "invalid coprocessor CPU type %d instance %d", cpuType, instance)
	}

	if existing := *slot; existing != nil {
		if !replace {
			return coprocLifecycleErr(COPROC_ERR_LOAD_FAILED, "%s coprocessor worker is already online", coprocInstanceLabel(cpuType, instance))
		}
		*slot = nil
		m.setWorkerMetaLocked(cpuType, instance, time.Time{}, "")
		m.mu.Unlock()
		m.stopWorkerAndUnregister(cpuType, existing)
		m.mu.Lock()
	}

	// Clear any stale version ack left by a previous worker on this ring before
	// launching, so a non-conforming replacement cannot pass the START gate on a
	// leftover acknowledgement (initRings only clears at init/reset).
	if idx := coprocRingIndex(cpuType, instance); idx >= 0 {
		m.bus.Write8(ringBaseAddr(idx)+RING_ACK_VERSION_OFFSET, 0)
	}

	m.mu.Unlock()
	worker, err := m.createWorker(cpuType, instance, data)
	m.mu.Lock()
	if err != nil {
		return coprocLifecycleErr(COPROC_ERR_LOAD_FAILED, "%w", err)
	}

	if *slot != nil {
		m.mu.Unlock()
		worker.stopCPU()
		stopped := false
		select {
		case <-worker.done:
			stopped = true
		case <-time.After(2 * time.Second):
		}
		if stopped && worker.disposeCPU != nil {
			worker.disposeCPU()
		}
		m.mu.Lock()
		return coprocLifecycleErr(COPROC_ERR_LOAD_FAILED, "%s coprocessor worker was started concurrently", coprocInstanceLabel(cpuType, instance))
	}
	*slot = worker
	// Mark the worker startup-pending under an enabled gate so a concurrent
	// enqueue cannot reach it before awaitWorkerAckLocked clears the handshake.
	if m.versionGateEnabled {
		worker.mu.Lock()
		worker.gatePending = true
		worker.mu.Unlock()
	}
	m.setWorkerMetaLocked(cpuType, instance, time.Now(), filename)
	mon := m.monitor

	m.mu.Unlock()
	newID := -1
	if mon != nil && worker.debugCPU != nil {
		newID = mon.RegisterCPU(coprocInstanceLabel(cpuType, instance), worker.debugCPU)
	}
	m.mu.Lock()
	if *slot == worker {
		worker.monitorID = newID
	} else if mon != nil && newID >= 0 {
		m.mu.Unlock()
		mon.UnregisterCPU(newID)
		m.mu.Lock()
	}
	m.watchWorkerDone(worker)
	if err := m.awaitWorkerAckLocked(cpuType, instance, slot, worker); err != nil {
		return err
	}
	return nil
}

// SetVersionGateEnabled toggles the START-time layout-version handshake. It is
// used by tests that start raw, non-conforming worker images; production leaves
// it enabled.
func (m *CoprocessorManager) SetVersionGateEnabled(enabled bool) {
	m.mu.Lock()
	m.versionGateEnabled = enabled
	m.mu.Unlock()
}

// awaitWorkerAckLocked blocks (releasing m.mu while polling) until the freshly
// started worker echoes COPROC_LAYOUT_VERSION into its ring's
// RING_ACK_VERSION_OFFSET, or the gate timeout expires. On timeout it tears the
// worker down and returns COPROC_ERR_STALE_WORKER, guaranteeing the manager
// never routes a request descriptor to a non-acknowledging worker. Caller holds
// m.mu on entry and exit.
func (m *CoprocessorManager) awaitWorkerAckLocked(cpuType, instance uint32, slot **CoprocWorker, worker *CoprocWorker) error {
	if !m.versionGateEnabled {
		return nil
	}
	idx := coprocRingIndex(cpuType, instance)
	if idx < 0 {
		return nil
	}
	ackAddr := ringBaseAddr(idx) + RING_ACK_VERSION_OFFSET
	timeout := m.versionGateTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	m.mu.Unlock()
	deadline := time.Now().Add(timeout)
	acked := false
	for {
		if uint32(m.bus.Read8(ackAddr)) == COPROC_LAYOUT_VERSION {
			acked = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Microsecond)
	}
	m.mu.Lock()

	if acked {
		// Success requires that we still own the slot. Another lifecycle op may
		// have stopped or replaced this worker while m.mu was released for the
		// ack poll; a leftover or replacement ack must not let START report
		// success for a worker that is no longer installed.
		if *slot == worker {
			worker.mu.Lock()
			worker.gateAcked = true
			worker.gatePending = false
			worker.mu.Unlock()
			return nil
		}
		// Lost ownership: stop our now-orphaned worker and fail the start.
		m.mu.Unlock()
		m.stopWorkerAndUnregister(cpuType, worker)
		m.mu.Lock()
		return coprocLifecycleErr(COPROC_ERR_LOAD_FAILED,
			"%s coprocessor worker was stopped or replaced during startup",
			coprocInstanceLabel(cpuType, instance))
	}
	// Stale worker: tear it down if it is still the installed one.
	if *slot == worker {
		*slot = nil
		m.setWorkerMetaLocked(cpuType, instance, time.Time{}, "")
		m.markWorkerDownCompletionsLocked(cpuType, instance)
		m.mu.Unlock()
		m.stopWorkerAndUnregister(cpuType, worker)
		m.mu.Lock()
	}
	return coprocLifecycleErr(COPROC_ERR_STALE_WORKER,
		"%s coprocessor worker did not acknowledge mailbox layout version %d",
		coprocInstanceLabel(cpuType, instance), COPROC_LAYOUT_VERSION)
}

func (m *CoprocessorManager) watchWorkerDone(worker *CoprocWorker) {
	if m == nil || worker == nil || worker.done == nil {
		return
	}
	go func() {
		<-worker.done
		m.signalCompletionWake()
	}()
}

// StopWorker stops an online coprocessor worker and unregisters it from IEMon.
func (m *CoprocessorManager) StopWorker(cpuType uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopWorkerLocked(cpuType, 0)
}

func (m *CoprocessorManager) stopWorkerLocked(cpuType, instance uint32) error {
	slot := m.workerSlotLocked(cpuType, instance)
	if slot == nil {
		return coprocLifecycleErr(coprocSelectionError(cpuType, instance), "invalid coprocessor CPU type %d instance %d", cpuType, instance)
	}
	worker := *slot
	if worker == nil {
		return coprocLifecycleErr(COPROC_ERR_NO_WORKER, "%s coprocessor worker is not online", coprocInstanceLabel(cpuType, instance))
	}
	*slot = nil
	m.setWorkerMetaLocked(cpuType, instance, time.Time{}, "")
	m.markWorkerDownCompletionsLocked(cpuType, instance)
	m.mu.Unlock()
	m.stopWorkerAndUnregister(cpuType, worker)
	m.mu.Lock()
	return nil
}

func (m *CoprocessorManager) pruneCompletions() {
	now := time.Now()
	// TTL-based pruning
	for k, c := range m.completions {
		if now.Sub(c.created).Seconds() > float64(COPROC_COMPLETION_TTL) {
			delete(m.completions, k)
		}
	}
	// Cap-based pruning
	for len(m.completions) > COPROC_MAX_COMPLETIONS {
		var oldestKey uint32
		var oldestTime time.Time
		first := true
		for k, c := range m.completions {
			if first || c.created.Before(oldestTime) {
				oldestKey = k
				oldestTime = c.created
				first = false
			}
		}
		delete(m.completions, oldestKey)
	}
}

func (m *CoprocessorManager) createWorker(cpuType, instance uint32, data []byte) (*CoprocWorker, error) {
	if instance >= coprocInstanceLimit(cpuType) {
		return nil, fmt.Errorf("CPU type %d does not support worker instance %d", cpuType, instance)
	}
	switch cpuType {
	case EXEC_TYPE_IE32:
		return createIE32WorkerConfigured(m.bus, data, instance, m.ie32DisableJIT)
	case EXEC_TYPE_6502:
		return create6502Worker(m.bus, data, instance)
	case EXEC_TYPE_M68K:
		return createM68KWorker(m.bus, data, instance)
	case EXEC_TYPE_Z80:
		return createZ80Worker(m.bus, data, instance)
	case EXEC_TYPE_X86:
		return createX86Worker(m.bus, data, instance)
	case EXEC_TYPE_IE64:
		return createIE64Worker(m.bus, data, instance)
	default:
		return nil, fmt.Errorf("unsupported CPU type: %d", cpuType)
	}
}

// createWorkerAndRegister creates a worker and registers it with the monitor.
// Caller must NOT hold m.mu.
func (m *CoprocessorManager) createWorkerAndRegister(cpuType uint32, data []byte) (*CoprocWorker, error) {
	worker, err := m.createWorker(cpuType, 0, data)
	if err != nil {
		return nil, err
	}
	if m.monitor != nil && worker.debugCPU != nil {
		label := coprocLabel(cpuType)
		worker.monitorID = m.monitor.RegisterCPU(label, worker.debugCPU)
	}
	return worker, nil
}

// stopWorkerAndUnregister stops a worker and unregisters it from the monitor.
// Caller must NOT hold m.mu.
func (m *CoprocessorManager) stopWorkerAndUnregister(cpuType uint32, worker *CoprocWorker) {
	if worker.debugCPU != nil {
		worker.debugCPU.Freeze()
	}
	worker.stopCPU()
	stopped := false
	select {
	case <-worker.done:
		stopped = true
	case <-time.After(2 * time.Second):
	}
	if stopped && worker.disposeCPU != nil {
		worker.disposeCPU()
	}
	if m.monitor != nil && worker.monitorID >= 0 {
		m.monitor.UnregisterCPU(worker.monitorID)
	}
}

// Pause stops the worker CPU and waits for the goroutine to exit.
// If the stop times out (2s), frozen is NOT set to avoid inconsistent state.
func (w *CoprocWorker) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.frozen {
		return
	}
	w.stopCPU()
	select {
	case <-w.done:
		w.frozen = true
	case <-time.After(2 * time.Second):
		// Timeout: goroutine still alive. Do NOT set frozen.
	}
}

// Unpause launches a new goroutine running execCPU.
func (w *CoprocWorker) Unpause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.frozen {
		return
	}
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		w.execCPU()
	}()
	w.frozen = false
}

// coprocLabel returns the monitor label for a CPU type.
func coprocLabel(cpuType uint32) string {
	switch cpuType {
	case EXEC_TYPE_IE32:
		return "coproc:IE32"
	case EXEC_TYPE_6502:
		return "coproc:6502"
	case EXEC_TYPE_M68K:
		return "coproc:M68K"
	case EXEC_TYPE_Z80:
		return "coproc:Z80"
	case EXEC_TYPE_X86:
		return "coproc:X86"
	case EXEC_TYPE_IE64:
		return "coproc:IE64"
	default:
		return fmt.Sprintf("coproc:type%d", cpuType)
	}
}

// coprocInstanceLabel is coprocLabel with a "#<instance>" suffix for
// second and later worker instances.
func coprocInstanceLabel(cpuType, instance uint32) string {
	if instance == 0 {
		return coprocLabel(cpuType)
	}
	return fmt.Sprintf("%s#%d", coprocLabel(cpuType), instance)
}

// CoprocDebugInfo holds a coprocessor's debug adapter and type label.
type CoprocDebugInfo struct {
	CPUType uint32
	Label   string
	CPU     DebuggableCPU
}

// GetActiveWorkers returns a snapshot of all running coprocessor workers
// with their DebuggableCPU references. Safe for inspection from the monitor.
func (m *CoprocessorManager) GetActiveWorkers() []CoprocDebugInfo {
	m.mu.Lock()
	m.reapDeadWorkersLocked()

	var result []CoprocDebugInfo
	m.forEachWorkerSlotLocked(func(cpuType, instance uint32, slot **CoprocWorker) {
		w := *slot
		if w != nil && w.debugCPU != nil {
			result = append(result, CoprocDebugInfo{
				CPUType: cpuType,
				Label:   coprocInstanceLabel(cpuType, instance),
				CPU:     w.debugCPU,
			})
		}
	})
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
	return result
}

// WorkerInventory returns every supported coprocessor worker slot, online or
// offline. JIT-capable CPU types contribute two slots; the others contribute
// one.
func (m *CoprocessorManager) WorkerInventory() []CoprocWorkerSlot {
	m.mu.Lock()
	m.reapDeadWorkersLocked()

	result := make([]CoprocWorkerSlot, 0, 7)
	for _, cpuType := range []uint32{EXEC_TYPE_IE32, EXEC_TYPE_IE64, EXEC_TYPE_M68K, EXEC_TYPE_Z80, EXEC_TYPE_6502, EXEC_TYPE_X86} {
		for inst := uint32(0); inst < coprocInstanceLimit(cpuType); inst++ {
			slot := CoprocWorkerSlot{
				CPUType:   cpuType,
				Label:     coprocInstanceLabel(cpuType, inst),
				MonitorID: -1,
			}
			slot.Path = m.workerImagePath[cpuType][inst]
			if w := m.workerAtLocked(cpuType, inst); w != nil {
				slot.Online = true
				slot.MonitorID = w.monitorID
				slot.CPU = w.debugCPU
			}
			result = append(result, slot)
		}
	}
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)
	return result
}

// StopAll stops all running workers and the completion watcher. Called during shutdown.
func (m *CoprocessorManager) StopAll() {
	m.StopCompletionWatcher()

	m.mu.Lock()
	var toStop []struct {
		cpuType uint32
		worker  *CoprocWorker
	}
	m.forEachWorkerSlotLocked(func(cpuType, instance uint32, slot **CoprocWorker) {
		if w := *slot; w != nil {
			toStop = append(toStop, struct {
				cpuType uint32
				worker  *CoprocWorker
			}{cpuType, w})
			*slot = nil
			m.setWorkerMetaLocked(cpuType, instance, time.Time{}, "")
		}
	})
	drained := m.drainPendingUnregsLocked()
	m.mu.Unlock()
	m.flushReaped(drained)

	for _, s := range toStop {
		m.stopWorkerAndUnregister(s.cpuType, s.worker)
	}
}
