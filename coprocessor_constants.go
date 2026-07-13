package main

// Coprocessor MMIO Registers (0xF2340-0xF238F)
const (
	COPROC_BASE = 0xF2340

	COPROC_CMD           = COPROC_BASE + 0x00 // Command register (triggers action on write)
	COPROC_CPU_TYPE      = COPROC_BASE + 0x04 // Target CPU type (EXEC_TYPE_*)
	COPROC_CMD_STATUS    = COPROC_BASE + 0x08 // Status of last CMD operation (ok/error)
	COPROC_CMD_ERROR     = COPROC_BASE + 0x0C // Error code for last CMD
	COPROC_TICKET        = COPROC_BASE + 0x10 // Ticket ID (written by ENQUEUE, read by POLL/WAIT)
	COPROC_TICKET_STATUS = COPROC_BASE + 0x14 // Per-ticket status (set by POLL/WAIT)
	COPROC_OP            = COPROC_BASE + 0x18 // Operation code for request
	COPROC_REQ_PTR       = COPROC_BASE + 0x1C // Request data pointer
	COPROC_REQ_LEN       = COPROC_BASE + 0x20 // Request data length
	COPROC_RESP_PTR      = COPROC_BASE + 0x24 // Response buffer pointer
	COPROC_RESP_CAP      = COPROC_BASE + 0x28 // Response buffer capacity
	COPROC_TIMEOUT       = COPROC_BASE + 0x2C // Timeout in ms (for WAIT)
	COPROC_NAME_PTR      = COPROC_BASE + 0x30 // Pointer to service filename string
	COPROC_WORKER_STATE  = COPROC_BASE + 0x34 // Bitmask of running workers (read-only)

	// Extended registers (IE64 coprocessor support)
	COPROC_STATS_OPS         = COPROC_BASE + 0x38 // Total ops dispatched (read-only)
	COPROC_STATS_BYTES       = COPROC_BASE + 0x3C // Total bytes processed (read-only)
	COPROC_IRQ_CTRL          = COPROC_BASE + 0x40 // IRQ enable/disable (write bit 0), status (read)
	COPROC_DISPATCH_OVERHEAD = COPROC_BASE + 0x44 // Calibrated overhead in nanoseconds (read-only)
	COPROC_COMPLETED_TICKET  = COPROC_BASE + 0x48 // Last completed ticket ID (read-only)
	COPROC_INSTANCE          = COPROC_BASE + 0x4C // Worker instance selector (0 = default; pairs with COPROC_CPU_TYPE)

	COPROC_END = COPROC_BASE + 0x4F

	// Extended monitor registers (after clipboard bridge gap at 0xF2390-0xF23AF)
	COPROC_EXT_BASE      = 0xF23B0
	COPROC_RING_DEPTH    = 0xF23B0 // Selected CPU ring occupancy: write COPROC_CPU_TYPE first (R)
	COPROC_WORKER_UPTIME = 0xF23B4 // Seconds since selected CPU worker started: write COPROC_CPU_TYPE first (R)
	COPROC_STATS_RESET   = 0xF23B8 // Write 1 to zero global stats + busy buckets (W)
	COPROC_BUSY_PCT      = 0xF23BC // Aggregate worker busy % over last 1s, 0-100 (R)
	COPROC_EXT_END       = 0xF23BF

	// Third register block: capability, version, and selected-instance
	// discovery. Sits in the free gap between CPU Wait (..0xF259F) and the SFX
	// extended aliases (0xF2600..). The nearer 0xF23C0 gap is taken by the AROS
	// IRQ_DIAG_REGION. Reachable only by 32-bit-addressing CPUs and the
	// main-CPU BASIC runtime, not by the 6502/Z80 $F200 gateway. Workers learn
	// the layout version from the ring header, not this block.
	COPROC_EXT2_BASE       = 0xF25A0
	COPROC_INSTANCE_LIMIT  = 0xF25A0 // Instance count for current COPROC_CPU_TYPE (R)
	COPROC_SELECTED_STATE  = 0xF25A4 // State of selected (COPROC_CPU_TYPE, COPROC_INSTANCE) (R)
	COPROC_MAILBOX_VERSION = 0xF25A8 // COPROC_LAYOUT_VERSION (R)
	COPROC_WORKER_BASE     = 0xF25AC // Selected worker window base address (R)
	COPROC_WORKER_END      = 0xF25B0 // Selected worker window end address (R)
	COPROC_WORKER_RING     = 0xF25B4 // Selected worker ring base address (R)
	// Per-instance liveness bitmask, bit (cpuType*2 + instance) set when that
	// worker is running. A single atomic read that needs no COPROC_CPU_TYPE /
	// COPROC_INSTANCE selection, so enumerating workers cannot redirect a
	// concurrent raw coprocessor command.
	COPROC_INSTANCE_STATE = 0xF25B8 // Per-(cpuType,instance) running bitmask (R)
	COPROC_EXT2_END       = 0xF25BF
)

// Coprocessor commands (written to COPROC_CMD)
const (
	COPROC_CMD_START     = 1 // Start worker from file
	COPROC_CMD_STOP      = 2 // Stop worker
	COPROC_CMD_ENQUEUE   = 3 // Submit request, returns ticket in COPROC_TICKET
	COPROC_CMD_POLL      = 4 // Check ticket status, returns in COPROC_TICKET_STATUS
	COPROC_CMD_WAIT      = 5 // Block until ticket completes or timeout
	COPROC_CMD_START_MEM = 6 // Start worker from a guest-RAM blob (REQ_PTR/REQ_LEN)
)

// Coprocessor command status (read from COPROC_CMD_STATUS)
const (
	COPROC_STATUS_OK    = 0
	COPROC_STATUS_ERROR = 1
)

// Coprocessor ticket status (read from COPROC_TICKET_STATUS)
const (
	COPROC_TICKET_PENDING     = 0
	COPROC_TICKET_RUNNING     = 1
	COPROC_TICKET_OK          = 2
	COPROC_TICKET_ERROR       = 3
	COPROC_TICKET_TIMEOUT     = 4
	COPROC_TICKET_WORKER_DOWN = 5
)

// Coprocessor error codes (read from COPROC_CMD_ERROR)
const (
	COPROC_ERR_NONE         = 0
	COPROC_ERR_INVALID_CPU  = 1
	COPROC_ERR_NOT_FOUND    = 2
	COPROC_ERR_PATH_INVALID = 3
	COPROC_ERR_LOAD_FAILED  = 4
	COPROC_ERR_QUEUE_FULL   = 5
	COPROC_ERR_NO_WORKER    = 6
	COPROC_ERR_STALE_TICKET = 7
	// COPROC_ERR_INVALID_INSTANCE is distinct from COPROC_ERR_INVALID_CPU: the
	// CPU type is valid but the requested instance is beyond its limit.
	COPROC_ERR_INVALID_INSTANCE = 8
	// COPROC_ERR_STALE_WORKER: the started worker image did not acknowledge the
	// current mailbox layout version within the START handshake timeout.
	COPROC_ERR_STALE_WORKER = 9
)

// COPROC_LAYOUT_VERSION is the current mailbox/ring layout revision. A worker
// echoes this from its ring header into RING_ACK_VERSION_OFFSET at startup;
// startWorkerLocked refuses to route work to a worker that does not.
const COPROC_LAYOUT_VERSION = 1

// 16-bit CPU gateway window for coprocessor MMIO.
// Z80 and 6502 cannot address 0xF2340 directly (16-bit address space).
// Their adapters intercept 0xF200-0xF23F and redirect to COPROC_BASE on the bus.
// This range is within the I/O region (0xF000-0xFFFF) and maps to bus address
// 0xF0200 via translateIO8Bit, which is an unused gap - safe to reserve.
const (
	COPROC_GATEWAY_BASE = 0xF200 // Z80/6502 address for COPROC_CMD
	COPROC_GATEWAY_END  = 0xF24F // Z80/6502 address for COPROC_END
)

// Mailbox shared RAM (0x790000-0x792FFF, 12 KiB for 12 ring slots).
// Placed in the ROM region gap (after BSS/data at 0x780000-0x78FFFF,
// before ROM region end at 0x7FFFFF) to avoid overlap with AROS
// fast memory (0x800000-0x1DFFFFF).
const (
	MAILBOX_BASE = 0x790000

	// Ring buffer layout.
	RING_CAPACITY = 16 // Max entries per ring
	// RING_STRIDE is a power of two >= a ring's true content size
	// (RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE = 0x308). At
	// 0x400 no ring's slot-15 response overflows into the next ring's header,
	// which retires the old ring-6 placement hack, and the shift addressing is
	// cheap. See COPROC_LAYOUT_VERSION.
	RING_STRIDE = 0x400

	// Twelve ring slots: six CPU-type indices (cpuTypeToIndex) times two
	// instances. Ring index = cpuTypeToIndex(cpuType)*2 + instance, pure
	// arithmetic with no special case. The instance-1 slots of the three
	// single-instance types (IE32 ring 1, 6502 ring 3, Z80 ring 7) are
	// reserved but never allocated, keeping the addressing trivial.
	COPROC_RING_COUNT = 12
	MAILBOX_SIZE      = COPROC_RING_COUNT * RING_STRIDE // 0x3000, ends 0x793000
	MAILBOX_END       = MAILBOX_BASE + MAILBOX_SIZE - 1

	// Offsets within a ring
	RING_HEAD_OFFSET      = 0x00 // uint8: next write slot (producer)
	RING_TAIL_OFFSET      = 0x01 // uint8: next read slot (consumer)
	RING_CAPACITY_OFFSET  = 0x02 // uint8: ring depth (16)
	// Version-gate handshake fields, inside the reserved 0x03..0x07 header
	// bytes ahead of RING_ENTRIES_OFFSET. initRings writes the layout version
	// into RING_LAYOUT_VERSION_OFFSET; a conforming worker echoes it into
	// RING_ACK_VERSION_OFFSET at startup.
	RING_LAYOUT_VERSION_OFFSET = 0x03 // uint8: layout version (host writes)
	RING_ACK_VERSION_OFFSET    = 0x04 // uint8: worker's echoed version
	RING_ENTRIES_OFFSET        = 0x08  // Request descriptors start
	RING_RESPONSES_OFFSET      = 0x208 // Response descriptors start

	// Request descriptor (32 bytes)
	REQ_DESC_SIZE    = 32
	REQ_TICKET_OFF   = 0x00
	REQ_CPU_TYPE_OFF = 0x04
	REQ_OP_OFF       = 0x08
	REQ_TIMEOUT_OFF  = 0x0C
	// Deprecated: request descriptor offset 0x0C stores timeout metadata, not flags.
	REQ_FLAGS_OFF    = REQ_TIMEOUT_OFF
	REQ_REQ_PTR_OFF  = 0x10
	REQ_REQ_LEN_OFF  = 0x14
	REQ_RESP_PTR_OFF = 0x18
	REQ_RESP_CAP_OFF = 0x1C

	// Response descriptor (16 bytes)
	RESP_DESC_SIZE       = 16
	RESP_TICKET_OFF      = 0x00
	RESP_STATUS_OFF      = 0x04
	RESP_RESULT_CODE_OFF = 0x08
	RESP_RESP_LEN_OFF    = 0x0C
)

// Worker memory regions
const (
	WORKER_IE32_BASE = 0x200000
	WORKER_IE32_END  = 0x27FFFF
	WORKER_IE32_SIZE = WORKER_IE32_END - WORKER_IE32_BASE + 1

	WORKER_M68K_BASE = 0x280000
	WORKER_M68K_END  = 0x2FFFFF
	WORKER_M68K_SIZE = WORKER_M68K_END - WORKER_M68K_BASE + 1

	WORKER_6502_BASE = 0x300000
	WORKER_6502_END  = 0x30FFFF
	WORKER_6502_SIZE = WORKER_6502_END - WORKER_6502_BASE + 1

	WORKER_Z80_BASE = 0x310000
	WORKER_Z80_END  = 0x31FFFF
	WORKER_Z80_SIZE = WORKER_Z80_END - WORKER_Z80_BASE + 1

	WORKER_X86_BASE = 0x320000
	WORKER_X86_END  = 0x39FFFF
	WORKER_X86_SIZE = WORKER_X86_END - WORKER_X86_BASE + 1

	WORKER_IE64_BASE = 0x3A0000
	WORKER_IE64_END  = 0x41FFFF
	WORKER_IE64_SIZE = WORKER_IE64_END - WORKER_IE64_BASE + 1
)

// cpuTypeToIndex maps EXEC_TYPE_* constants to ring index (0-5).
// Returns -1 for invalid/unsupported types.
func cpuTypeToIndex(cpuType uint32) int {
	switch cpuType {
	case EXEC_TYPE_IE32:
		return 0
	case EXEC_TYPE_6502:
		return 1
	case EXEC_TYPE_M68K:
		return 2
	case EXEC_TYPE_Z80:
		return 3
	case EXEC_TYPE_X86:
		return 4
	case EXEC_TYPE_IE64:
		return 5
	default:
		return -1
	}
}

// ringBaseAddr returns the bus address of the ring buffer for the given ring
// index. Uniform rule: MAILBOX_BASE + ringIdx*RING_STRIDE, no special cases.
func ringBaseAddr(ringIdx int) uint32 {
	return MAILBOX_BASE + uint32(ringIdx)*RING_STRIDE
}

// coprocInstanceLimit returns how many worker instances a CPU type supports.
// The JIT-capable coprocessors (M68K, x86, IE64) run two instances; IE32,
// 6502, and Z80 run one. Invalid types return 0. This layout is final.
func coprocInstanceLimit(cpuType uint32) uint32 {
	switch cpuType {
	case EXEC_TYPE_M68K, EXEC_TYPE_X86, EXEC_TYPE_IE64:
		return 2
	case EXEC_TYPE_IE32, EXEC_TYPE_6502, EXEC_TYPE_Z80:
		return 1
	default:
		return 0
	}
}

// coprocRingIndex maps (cpuType, instance) to a mailbox ring index, or -1 when
// the combination is unsupported. Rule: cpuTypeToIndex(cpuType)*2 + instance.
func coprocRingIndex(cpuType, instance uint32) int {
	if instance >= coprocInstanceLimit(cpuType) {
		return -1
	}
	idx := cpuTypeToIndex(cpuType)
	if idx < 0 {
		return -1
	}
	return idx*2 + int(instance)
}

// coprocSelectionError classifies a (cpuType, instance) selection: NONE when
// valid, INVALID_CPU when the type is unknown, INVALID_INSTANCE when the type
// is valid but the instance is beyond its limit.
func coprocSelectionError(cpuType, instance uint32) uint32 {
	if cpuTypeToIndex(cpuType) < 0 {
		return COPROC_ERR_INVALID_CPU
	}
	if instance >= coprocInstanceLimit(cpuType) {
		return COPROC_ERR_INVALID_INSTANCE
	}
	return COPROC_ERR_NONE
}

// workerWindow is the single source of truth for a worker's dedicated RAM
// window. It returns the window base, inclusive end, and size for a given
// (cpuType, instance), or ok=false when the combination is unsupported.
//
// Instance-0 windows keep their historical addresses. The three JIT-capable
// types (M68K, x86, IE64) get a full 512 KiB second window packed from
// 0x420000 (the reclaimed retired WORKER_M68K2 slot) up to 0x59FFFF, all below
// BASIC_EXPORT_BASE (0x600000) and inside the worker-safe aperture
// 0x200000..0x5FFFFF.
func workerWindow(cpuType, instance uint32) (base, end, size uint32, ok bool) {
	if instance >= coprocInstanceLimit(cpuType) {
		return 0, 0, 0, false
	}
	switch cpuType {
	case EXEC_TYPE_IE32:
		base, end = 0x200000, 0x27FFFF
	case EXEC_TYPE_M68K:
		if instance == 0 {
			base, end = 0x280000, 0x2FFFFF
		} else {
			base, end = 0x420000, 0x49FFFF
		}
	case EXEC_TYPE_6502:
		base, end = 0x300000, 0x30FFFF
	case EXEC_TYPE_Z80:
		base, end = 0x310000, 0x31FFFF
	case EXEC_TYPE_X86:
		if instance == 0 {
			base, end = 0x320000, 0x39FFFF
		} else {
			base, end = 0x4A0000, 0x51FFFF
		}
	case EXEC_TYPE_IE64:
		if instance == 0 {
			base, end = 0x3A0000, 0x41FFFF
		} else {
			base, end = 0x520000, 0x59FFFF
		}
	default:
		return 0, 0, 0, false
	}
	return base, end, end - base + 1, true
}

// isWorkerCodeWindow reports whether addr lies inside either M68K worker RAM
// window. Those windows hold big-endian M68K code, data, and stack, so the
// M68K byte-swap must stay active over them even though instance 1 sits inside
// the coprocessor shared-data aperture (see isCoprocSharedAddr).
func isWorkerCodeWindow(addr uint32) bool {
	for inst := uint32(0); inst < coprocInstanceLimit(EXEC_TYPE_M68K); inst++ {
		if base, end, _, ok := workerWindow(EXEC_TYPE_M68K, inst); ok && addr >= base && addr <= end {
			return true
		}
	}
	return false
}

// Maximum completions tracked and eviction parameters
const (
	COPROC_MAX_COMPLETIONS = 256
	COPROC_COMPLETION_TTL  = 60 // seconds
)
