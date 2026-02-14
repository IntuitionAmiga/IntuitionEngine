package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CoprocWorker represents a running coprocessor worker.
type CoprocWorker struct {
	cpuType  uint32
	stop     func()        // sets running=false on the worker CPU
	done     chan struct{} // closed when Execute() returns
	loadBase uint32
	loadEnd  uint32
	debugCPU DebuggableCPU // retained for monitor access
}

// CoprocCompletion tracks a ticket's completion state.
type CoprocCompletion struct {
	ticket     uint32
	cpuType    uint32 // stored at enqueue time for worker-down checks
	status     uint32
	resultCode uint32
	respLen    uint32
	observed   bool // true after first POLL of terminal state
	created    time.Time
}

// CoprocessorManager handles coprocessor MMIO, worker lifecycle, and ticket routing.
type CoprocessorManager struct {
	bus     *MachineBus
	baseDir string

	mu          sync.Mutex
	workers     [7]*CoprocWorker // indexed by cpuType (1-6)
	nextTicket  uint32
	completions map[uint32]*CoprocCompletion

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
	workerState  uint32
}

// NewCoprocessorManager creates a new coprocessor manager.
func NewCoprocessorManager(bus *MachineBus, baseDir string) *CoprocessorManager {
	mgr := &CoprocessorManager{
		bus:         bus,
		baseDir:     baseDir,
		nextTicket:  1,
		completions: make(map[uint32]*CoprocCompletion),
	}
	// Initialize ring headers in mailbox RAM
	for i := range 5 {
		base := ringBaseAddr(i)
		bus.Write8(base+RING_HEAD_OFFSET, 0)
		bus.Write8(base+RING_TAIL_OFFSET, 0)
		bus.Write8(base+RING_CAPACITY_OFFSET, RING_CAPACITY)
	}
	return mgr
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
	case COPROC_WORKER_STATE:
		return m.computeWorkerState()
	default:
		return 0
	}
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
	}
}

// HandleRead reads an MMIO register. Supports both aligned 32-bit reads
// and byte-level reads at sub-register offsets (for 8-bit CPUs).
func (m *CoprocessorManager) HandleRead(addr uint32) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	offset := addr - COPROC_BASE
	regBase := COPROC_BASE + (offset & ^uint32(3))
	byteOff := offset & 3
	val := m.readReg(regBase)
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
	defer m.mu.Unlock()

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
}

func (m *CoprocessorManager) dispatchCmd() {
	switch m.cmd {
	case COPROC_CMD_START:
		m.cmdStart()
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
	cpuIdx := cpuTypeToIndex(m.cpuType)
	if cpuIdx < 0 {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_INVALID_CPU
		return
	}

	filename := m.readFileName(m.namePtr)
	fullPath, ok := m.sanitizePath(filename)
	if !ok {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_PATH_INVALID
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_NOT_FOUND
		return
	}

	// Stop existing worker for this CPU type if running
	if existing := m.workers[m.cpuType]; existing != nil {
		existing.stop()
		m.mu.Unlock()
		select {
		case <-existing.done:
		case <-time.After(2 * time.Second):
		}
		m.mu.Lock()
		m.workers[m.cpuType] = nil
	}

	worker, err := m.createWorker(m.cpuType, data)
	if err != nil {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_LOAD_FAILED
		return
	}

	m.workers[m.cpuType] = worker
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdStop() {
	cpuIdx := cpuTypeToIndex(m.cpuType)
	if cpuIdx < 0 {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_INVALID_CPU
		return
	}

	worker := m.workers[m.cpuType]
	if worker == nil {
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_NO_WORKER
		return
	}

	worker.stop()
	m.mu.Unlock()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
	}
	m.mu.Lock()
	m.workers[m.cpuType] = nil
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdEnqueue() {
	cpuIdx := cpuTypeToIndex(m.cpuType)
	if cpuIdx < 0 {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_INVALID_CPU
		return
	}

	worker := m.workers[m.cpuType]
	if worker == nil {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_NO_WORKER
		return
	}

	// Prune stale completions
	m.pruneCompletions()

	// Check ring capacity
	ringBase := ringBaseAddr(cpuIdx)
	head := m.bus.Read8(ringBase + RING_HEAD_OFFSET)
	capacity := m.bus.Read8(ringBase + RING_CAPACITY_OFFSET)
	nextHead := (head + 1) % capacity
	tail := m.bus.Read8(ringBase + RING_TAIL_OFFSET)
	if nextHead == tail {
		m.ticket = 0
		m.cmdStatus = COPROC_STATUS_ERROR
		m.cmdError = COPROC_ERR_QUEUE_FULL
		return
	}

	// Allocate ticket
	ticket := m.nextTicket
	m.nextTicket++

	// Write request descriptor at entries[head]
	entryAddr := ringBase + RING_ENTRIES_OFFSET + uint32(head)*REQ_DESC_SIZE
	m.bus.Write32(entryAddr+REQ_TICKET_OFF, ticket)
	m.bus.Write32(entryAddr+REQ_CPU_TYPE_OFF, m.cpuType)
	m.bus.Write32(entryAddr+REQ_OP_OFF, m.op)
	m.bus.Write32(entryAddr+REQ_FLAGS_OFF, 0)
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
		ticket:  ticket,
		cpuType: m.cpuType,
		status:  COPROC_TICKET_PENDING,
		created: time.Now(),
	}

	m.ticket = ticket
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdPoll() {
	ticket := m.ticket
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
		// Not yet terminal — scan ring to discover new state
		status = m.scanTicketStatus(ticket)
		if status == COPROC_TICKET_PENDING || status == COPROC_TICKET_RUNNING {
			// Still non-terminal — check if worker is down
			ct := comp.cpuType
			if ct >= 1 && ct <= 6 && m.workers[ct] == nil {
				status = COPROC_TICKET_WORKER_DOWN
			}
		}
	}

	if status != COPROC_TICKET_PENDING && status != COPROC_TICKET_RUNNING {
		// Terminal state — handle two-read eviction
		comp.status = status
		if comp.observed {
			delete(m.completions, ticket)
		} else {
			comp.observed = true
		}
	}

	m.ticketStatus = status
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

func (m *CoprocessorManager) cmdWait() {
	ticket := m.ticket
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
		return
	}

	// Release lock while waiting
	m.mu.Unlock()

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	var status uint32
	for {
		status = m.scanTicketStatus(ticket)
		if status != COPROC_TICKET_PENDING && status != COPROC_TICKET_RUNNING {
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
	m.ticketStatus = status
	m.cmdStatus = COPROC_STATUS_OK
	m.cmdError = COPROC_ERR_NONE
}

// scanTicketStatus scans all ring response slots to find the status for a ticket.
func (m *CoprocessorManager) scanTicketStatus(ticket uint32) uint32 {
	for i := range 5 {
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

func (m *CoprocessorManager) computeWorkerState() uint32 {
	var state uint32
	for i := uint32(1); i <= 6; i++ {
		if m.workers[i] != nil {
			state |= 1 << i
		}
	}
	return state
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

func (m *CoprocessorManager) createWorker(cpuType uint32, data []byte) (*CoprocWorker, error) {
	switch cpuType {
	case EXEC_TYPE_IE32:
		return createIE32Worker(m.bus, data)
	case EXEC_TYPE_6502:
		return create6502Worker(m.bus, data)
	case EXEC_TYPE_M68K:
		return createM68KWorker(m.bus, data)
	case EXEC_TYPE_Z80:
		return createZ80Worker(m.bus, data)
	case EXEC_TYPE_X86:
		return createX86Worker(m.bus, data)
	default:
		return nil, fmt.Errorf("unsupported CPU type: %d", cpuType)
	}
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
	defer m.mu.Unlock()

	var result []CoprocDebugInfo
	for i := uint32(1); i <= 6; i++ {
		w := m.workers[i]
		if w != nil && w.debugCPU != nil {
			label := "coproc:"
			switch i {
			case EXEC_TYPE_IE32:
				label += "IE32"
			case EXEC_TYPE_6502:
				label += "6502"
			case EXEC_TYPE_M68K:
				label += "M68K"
			case EXEC_TYPE_Z80:
				label += "Z80"
			case EXEC_TYPE_X86:
				label += "X86"
			default:
				label += fmt.Sprintf("type%d", i)
			}
			result = append(result, CoprocDebugInfo{
				CPUType: i,
				Label:   label,
				CPU:     w.debugCPU,
			})
		}
	}
	return result
}

// StopAll stops all running workers. Called during shutdown.
func (m *CoprocessorManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := uint32(1); i <= 6; i++ {
		if w := m.workers[i]; w != nil {
			w.stop()
			select {
			case <-w.done:
			case <-time.After(2 * time.Second):
			}
			m.workers[i] = nil
		}
	}
}
