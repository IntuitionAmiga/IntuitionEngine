package main

// Coprocessor MMIO Registers (0xF2340-0xF237F)
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

	COPROC_END = COPROC_BASE + 0x3F
)

// Coprocessor commands (written to COPROC_CMD)
const (
	COPROC_CMD_START   = 1 // Start worker from file
	COPROC_CMD_STOP    = 2 // Stop worker
	COPROC_CMD_ENQUEUE = 3 // Submit request, returns ticket in COPROC_TICKET
	COPROC_CMD_POLL    = 4 // Check ticket status, returns in COPROC_TICKET_STATUS
	COPROC_CMD_WAIT    = 5 // Block until ticket completes or timeout
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
)

// Mailbox shared RAM (0x820000-0x820FFF, 4KB)
const (
	MAILBOX_BASE = 0x820000
	MAILBOX_SIZE = 0x1000
	MAILBOX_END  = MAILBOX_BASE + MAILBOX_SIZE - 1

	// Ring buffer layout
	RING_CAPACITY = 16    // Max entries per ring
	RING_STRIDE   = 0x300 // 768 bytes per CPU ring

	// Offsets within a ring
	RING_HEAD_OFFSET      = 0x00  // uint8: next write slot (producer)
	RING_TAIL_OFFSET      = 0x01  // uint8: next read slot (consumer)
	RING_CAPACITY_OFFSET  = 0x02  // uint8: ring depth (16)
	RING_ENTRIES_OFFSET   = 0x08  // Request descriptors start
	RING_RESPONSES_OFFSET = 0x208 // Response descriptors start

	// Request descriptor (32 bytes)
	REQ_DESC_SIZE    = 32
	REQ_TICKET_OFF   = 0x00
	REQ_CPU_TYPE_OFF = 0x04
	REQ_OP_OFF       = 0x08
	REQ_FLAGS_OFF    = 0x0C
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
)

// cpuTypeToIndex maps EXEC_TYPE_* constants to ring index (0-4).
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
	default:
		return -1
	}
}

// ringBaseAddr returns the bus address of the ring buffer for the given CPU index.
func ringBaseAddr(cpuIdx int) uint32 {
	return MAILBOX_BASE + uint32(cpuIdx)*RING_STRIDE
}

// Maximum completions tracked and eviction parameters
const (
	COPROC_MAX_COMPLETIONS = 256
	COPROC_COMPLETION_TTL  = 60 // seconds
)
