package main

// Bootstrap HostFS MMIO registers (0xF23E0-0xF23FF).
const (
	BOOT_HOSTFS_BASE = 0xF23E0

	BOOT_HOSTFS_CMD  = BOOT_HOSTFS_BASE + 0x00
	BOOT_HOSTFS_ARG1 = BOOT_HOSTFS_BASE + 0x04
	BOOT_HOSTFS_ARG2 = BOOT_HOSTFS_BASE + 0x08
	BOOT_HOSTFS_ARG3 = BOOT_HOSTFS_BASE + 0x0C
	BOOT_HOSTFS_ARG4 = BOOT_HOSTFS_BASE + 0x10
	BOOT_HOSTFS_RES1 = BOOT_HOSTFS_BASE + 0x14
	BOOT_HOSTFS_RES2 = BOOT_HOSTFS_BASE + 0x18
	BOOT_HOSTFS_ERR  = BOOT_HOSTFS_BASE + 0x1C

	BOOT_HOSTFS_END = BOOT_HOSTFS_BASE + 0x1F
)

const (
	BOOT_HOSTFS_DISCOVER = 0
	BOOT_HOSTFS_OPEN     = 1
	BOOT_HOSTFS_READ     = 2
	BOOT_HOSTFS_CLOSE    = 3
	BOOT_HOSTFS_STAT     = 4
	BOOT_HOSTFS_READDIR  = 5
	// M15.3: writable SYS: overlay support. The hostfs rejects writes
	// below any IOSSYS/ prefix so the embedded read-only system tree
	// remains tamper-proof.
	BOOT_HOSTFS_CREATE_WRITE = 6 // open/create a file for writing; arg1 = path ptr, res1 = handle
	BOOT_HOSTFS_WRITE        = 7 // write bytes to open handle; arg1 = handle, arg2 = src ptr, arg3 = byte_count, res1 = bytes_written
)

// BOOT_HOSTFS_OPEN_MODE_* values passed in arg4 to OPEN. Default (0) stays
// read-only to preserve M15.2 boot semantics; CREATE_WRITE uses a separate
// command for clarity.
const (
	BOOT_HOSTFS_MODE_READ  = 0
	BOOT_HOSTFS_MODE_WRITE = 1
)

const (
	BOOT_HOSTFS_KIND_NONE = 0
	BOOT_HOSTFS_KIND_FILE = 1
	BOOT_HOSTFS_KIND_DIR  = 2
)

const (
	BOOT_HOSTFS_STAT_SIZE_OFF = 0
	BOOT_HOSTFS_STAT_KIND_OFF = 8
	BOOT_HOSTFS_STAT_SIZE     = 16

	BOOT_HOSTFS_DIRENT_KIND_OFF = 0
	BOOT_HOSTFS_DIRENT_NAME_OFF = 8
	BOOT_HOSTFS_DIRENT_NAME_MAX = 64
	BOOT_HOSTFS_DIRENT_SIZE     = BOOT_HOSTFS_DIRENT_NAME_OFF + BOOT_HOSTFS_DIRENT_NAME_MAX
)
