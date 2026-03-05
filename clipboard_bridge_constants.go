package main

// Clipboard Bridge MMIO Registers (0xF2380-0xF239F)
//
// This MMIO region bridges the host OS clipboard with AROS applications.
// The guest writes a data pointer and length, then writes a command to CTRL
// to trigger a read (host→guest) or write (guest→host) operation.
const (
	CLIP_REGION_BASE = 0xF2380
	CLIP_REGION_END  = 0xF239F

	CLIP_DATA_PTR   = CLIP_REGION_BASE + 0x00 // Guest RAM pointer for clipboard data
	CLIP_DATA_LEN   = CLIP_REGION_BASE + 0x04 // Data length in bytes
	CLIP_CTRL       = CLIP_REGION_BASE + 0x08 // Command: 1=read from host, 2=write to host
	CLIP_STATUS     = CLIP_REGION_BASE + 0x0C // Status: 0=ready, 1=busy, 2=empty, 3=error
	CLIP_RESULT_LEN = CLIP_REGION_BASE + 0x10 // Bytes actually read/written
	CLIP_FORMAT     = CLIP_REGION_BASE + 0x14 // Format: 0=text, 1=IFF
)

// Clipboard command codes (written to CLIP_CTRL)
const (
	CLIP_CMD_READ  = 1 // Read from host clipboard into guest RAM
	CLIP_CMD_WRITE = 2 // Write from guest RAM to host clipboard
)

// Clipboard status codes (read from CLIP_STATUS)
const (
	CLIP_STATUS_READY = 0 // Ready for next command
	CLIP_STATUS_BUSY  = 1 // Operation in progress
	CLIP_STATUS_EMPTY = 2 // Host clipboard is empty
	CLIP_STATUS_ERROR = 3 // Operation failed
)
