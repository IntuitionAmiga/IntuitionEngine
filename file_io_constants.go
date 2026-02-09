package main

// File I/O MMIO Registers (0xF2200-0xF221F)
const (
	FILE_IO_BASE = 0xF2200

	FILE_NAME_PTR   = FILE_IO_BASE + 0x00 // Pointer to null-terminated filename
	FILE_DATA_PTR   = FILE_IO_BASE + 0x04 // Pointer to data buffer
	FILE_DATA_LEN   = FILE_IO_BASE + 0x08 // Data length (for WRITE)
	FILE_CTRL       = FILE_IO_BASE + 0x0C // Bit 0=READ, Bit 1=WRITE
	FILE_STATUS     = FILE_IO_BASE + 0x10 // 0=OK, 1=ERROR
	FILE_RESULT_LEN = FILE_IO_BASE + 0x14 // Bytes actually read
	FILE_ERROR_CODE = FILE_IO_BASE + 0x18 // 0=OK, 1=NOT_FOUND, 2=PERMISSION, 3=PATH_TRAVERSAL

	FILE_IO_END = FILE_IO_BASE + 0x1F
)

// FILE_CTRL operations
const (
	FILE_OP_READ  = 1
	FILE_OP_WRITE = 2
)

// FILE_ERROR_CODE values
const (
	FILE_ERR_OK             = 0
	FILE_ERR_NOT_FOUND      = 1
	FILE_ERR_PERMISSION     = 2
	FILE_ERR_PATH_TRAVERSAL = 3
)
