package main

// ProgramExecutor MMIO Registers (0xF2320-0xF233F)
const (
	EXEC_BASE = 0xF2320

	EXEC_NAME_PTR = EXEC_BASE + 0x00 // Pointer to null-terminated filename
	EXEC_CTRL     = EXEC_BASE + 0x04 // 1=execute
	EXEC_STATUS   = EXEC_BASE + 0x08 // 0=idle,1=loading,2=running,3=error
	EXEC_TYPE     = EXEC_BASE + 0x0C // Executor type
	EXEC_ERROR    = EXEC_BASE + 0x10 // 0=ok,1=not-found,2=unsupported,3=path-invalid,4=load-failed
	EXEC_SESSION  = EXEC_BASE + 0x14 // Monotonic session id

	EXEC_END = EXEC_BASE + 0x1F
)

const (
	EXEC_OP_EXECUTE = 1
)

const (
	EXEC_STATUS_IDLE = iota
	EXEC_STATUS_LOADING
	EXEC_STATUS_RUNNING
	EXEC_STATUS_ERROR
)

const (
	EXEC_TYPE_NONE = iota
	EXEC_TYPE_IE32
	EXEC_TYPE_IE64
	EXEC_TYPE_6502
	EXEC_TYPE_M68K
	EXEC_TYPE_Z80
	EXEC_TYPE_X86
)

const (
	EXEC_ERR_OK = iota
	EXEC_ERR_NOT_FOUND
	EXEC_ERR_UNSUPPORTED
	EXEC_ERR_PATH_INVALID
	EXEC_ERR_LOAD_FAILED
)
