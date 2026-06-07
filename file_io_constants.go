package main

// File I/O MMIO Registers (0xF2200-0xF221F plus IE64 extension at 0xF22B0)
const (
	FILE_IO_BASE = 0xF2200

	FILE_NAME_PTR   = FILE_IO_BASE + 0x00 // Pointer to null-terminated filename
	FILE_DATA_PTR   = FILE_IO_BASE + 0x04 // Pointer to data buffer
	FILE_DATA_LEN   = FILE_IO_BASE + 0x08 // Data length (for WRITE)
	FILE_CTRL       = FILE_IO_BASE + 0x0C // 1=READ, 2=WRITE, 3=LIST
	FILE_STATUS     = FILE_IO_BASE + 0x10 // 0=OK, 1=ERROR
	FILE_RESULT_LEN = FILE_IO_BASE + 0x14 // Bytes actually read
	FILE_ERROR_CODE = FILE_IO_BASE + 0x18 // 0=OK, 1=NOT_FOUND, 2=PERMISSION, 3=PATH_TRAVERSAL
	// FILE_READ_MAX caps the next READ: if set non-zero before triggering FILE_OP_READ,
	// a file larger than this many bytes is refused (FILE_ERR_RANGE) BEFORE any bytes
	// are copied into guest memory. It is consumed (reset to 0) by each read, so it
	// affects only the read that set it; 0 means unbounded (the default for
	// BLOAD/LOAD/runtime-blob reads). Used by ASSEMBLE to bound its staging buffer.
	FILE_READ_MAX = FILE_IO_BASE + 0x1C
	FILE_IO_END   = FILE_IO_BASE + 0x1F

	// FILE_DATA_PTR64 is an IE64-native extension for data buffers above the
	// 32-bit File I/O staging window. Legacy callers keep using FILE_DATA_PTR.
	FILE_DATA_PTR64     = 0xF22B0
	FILE_DATA_PTR64_END = FILE_DATA_PTR64 + 0x07
)

// FILE_CTRL operations
const (
	FILE_OP_READ  = 1
	FILE_OP_WRITE = 2
	FILE_OP_LIST  = 3
)

// FILE_ERROR_CODE values
const (
	FILE_ERR_OK             = 0
	FILE_ERR_NOT_FOUND      = 1
	FILE_ERR_PERMISSION     = 2
	FILE_ERR_PATH_TRAVERSAL = 3
	// FILE_ERR_RANGE: the staging buffer [FILE_DATA_PTR, +len) overflows the
	// 32-bit address space or exceeds guest RAM. The transfer is refused whole
	// rather than wrapping or partially writing out of bounds.
	FILE_ERR_RANGE = 4
)
