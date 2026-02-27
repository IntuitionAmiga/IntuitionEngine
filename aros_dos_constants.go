package main

// AROS DOS MMIO Registers (0xF2220-0xF225F)
//
// This MMIO region is used by the AROS m68k-ie packet handler (iehandler)
// to translate AmigaDOS packet operations into host filesystem operations.
// The M68K handler writes arguments to ARG registers, then writes a command
// code to DOS_CMD which triggers the Go side to execute the operation
// synchronously. Results are available in RESULT1/RESULT2 immediately after.
const (
	AROS_DOS_BASE = 0xF2220

	AROS_DOS_CMD     = AROS_DOS_BASE + 0x00 // Command code (write triggers action)
	AROS_DOS_ARG1    = AROS_DOS_BASE + 0x04 // Argument 1 (pointer/value)
	AROS_DOS_ARG2    = AROS_DOS_BASE + 0x08 // Argument 2
	AROS_DOS_ARG3    = AROS_DOS_BASE + 0x0C // Argument 3
	AROS_DOS_ARG4    = AROS_DOS_BASE + 0x10 // Argument 4
	AROS_DOS_RESULT1 = AROS_DOS_BASE + 0x14 // Primary result (dp_Res1)
	AROS_DOS_RESULT2 = AROS_DOS_BASE + 0x18 // Secondary result / IoErr (dp_Res2)
	AROS_DOS_STATUS  = AROS_DOS_BASE + 0x1C // Status: 0=ready

	AROS_DOS_END = AROS_DOS_BASE + 0x3F
)

// AROS DOS command codes (written to AROS_DOS_CMD)
const (
	ADOS_CMD_LOCK         = 1  // ARG1=name_ptr, ARG2=parent_key, ARG3=mode → RESULT1=lock_key
	ADOS_CMD_UNLOCK       = 2  // ARG1=lock_key
	ADOS_CMD_EXAMINE      = 3  // ARG1=lock_key, ARG2=fib_ptr → fills FIB
	ADOS_CMD_EXNEXT       = 4  // ARG1=lock_key, ARG2=fib_ptr → fills next FIB entry
	ADOS_CMD_FINDINPUT    = 5  // ARG1=name_ptr, ARG2=parent_key → RESULT1=handle_key
	ADOS_CMD_FINDOUTPUT   = 6  // ARG1=name_ptr, ARG2=parent_key → RESULT1=handle_key
	ADOS_CMD_FINDUPDATE   = 7  // ARG1=name_ptr, ARG2=parent_key → RESULT1=handle_key
	ADOS_CMD_READ         = 8  // ARG1=handle_key, ARG2=buf_ptr, ARG3=length → RESULT1=bytes_read
	ADOS_CMD_WRITE        = 9  // ARG1=handle_key, ARG2=buf_ptr, ARG3=length → RESULT1=bytes_written
	ADOS_CMD_SEEK         = 10 // ARG1=handle_key, ARG2=offset, ARG3=mode → RESULT1=old_position
	ADOS_CMD_CLOSE        = 11 // ARG1=handle_key
	ADOS_CMD_PARENT       = 12 // ARG1=lock_key → RESULT1=parent_lock_key
	ADOS_CMD_DELETE       = 13 // ARG1=parent_key, ARG2=name_ptr
	ADOS_CMD_CREATEDIR    = 14 // ARG1=parent_key, ARG2=name_ptr → RESULT1=lock_key
	ADOS_CMD_RENAME       = 15 // ARG1=src_parent, ARG2=src_name, ARG3=dst_parent, ARG4=dst_name
	ADOS_CMD_DISKINFO     = 16 // ARG1=info_ptr → fills InfoData
	ADOS_CMD_DUPLOCK      = 17 // ARG1=lock_key → RESULT1=new_lock_key
	ADOS_CMD_SAMELOCK     = 18 // ARG1=key1, ARG2=key2 → RESULT1=same/different
	ADOS_CMD_IS_FS        = 19 // → RESULT1=DOSTRUE
	ADOS_CMD_SET_FILESIZE = 20 // ARG1=handle_key, ARG2=size, ARG3=mode → RESULT1=new_size
	ADOS_CMD_SET_PROTECT  = 21 // ARG1=parent_key, ARG2=name_ptr, ARG3=protect_bits
	ADOS_CMD_EXAMINE_FH   = 22 // ARG1=handle_key, ARG2=fib_ptr → fills FIB
)

// AmigaDOS error codes (stored in RESULT2 on failure)
const (
	ADOS_ERR_NONE                   = 0
	ADOS_ERROR_NO_FREE_STORE        = 103
	ADOS_ERROR_OBJECT_IN_USE        = 202
	ADOS_ERROR_OBJECT_EXISTS        = 203
	ADOS_ERROR_DIR_NOT_FOUND        = 204
	ADOS_ERROR_OBJECT_NOT_FOUND     = 205
	ADOS_ERROR_BAD_STREAM_NAME      = 206
	ADOS_ERROR_OBJECT_TOO_LARGE     = 207
	ADOS_ERROR_ACTION_NOT_KNOWN     = 209
	ADOS_ERROR_INVALID_LOCK         = 211
	ADOS_ERROR_OBJECT_WRONG_TYPE    = 212
	ADOS_ERROR_DISK_NOT_VALIDATED   = 213
	ADOS_ERROR_DISK_WRITE_PROTECTED = 214
	ADOS_ERROR_DELETE_PROTECTED     = 222
	ADOS_ERROR_WRITE_PROTECTED      = 223
	ADOS_ERROR_READ_PROTECTED       = 224
	ADOS_ERROR_NO_MORE_ENTRIES      = 232
	ADOS_ERROR_SEEK_ERROR           = 219
)

// AmigaDOS constants
const (
	ADOS_DOSTRUE  = 0xFFFFFFFF // -1 in signed 32-bit
	ADOS_DOSFALSE = 0

	ADOS_SHARED_LOCK    = 0xFFFFFFFE // -2
	ADOS_EXCLUSIVE_LOCK = 0xFFFFFFFF // -1

	ADOS_OFFSET_BEGINNING = 0xFFFFFFFF // -1
	ADOS_OFFSET_CURRENT   = 0
	ADOS_OFFSET_END       = 1

	ADOS_ST_FILE    = 0xFFFFFFFD // -3
	ADOS_ST_USERDIR = 2

	ADOS_LOCK_SAME        = 0
	ADOS_LOCK_SAME_VOLUME = 1
	ADOS_LOCK_DIFFERENT   = 0xFFFFFFFF // -1

	ADOS_ID_DOS_DISK  = 0x444F5300 // "DOS\0"
	ADOS_ID_VALIDATED = 0          // ID_VALIDATED (disk state)
)

// FileInfoBlock field offsets (260 bytes total, big-endian in guest memory)
const (
	ADOS_FIB_DISK_KEY       = 0
	ADOS_FIB_DIR_ENTRY_TYPE = 4
	ADOS_FIB_FILE_NAME      = 8 // 108 bytes (BSTR: length byte + 107 chars)
	ADOS_FIB_PROTECTION     = 116
	ADOS_FIB_ENTRY_TYPE     = 120
	ADOS_FIB_SIZE           = 124
	ADOS_FIB_NUM_BLOCKS     = 128
	ADOS_FIB_DATE           = 132 // DateStamp: 3 LONGs (days, mins, ticks) = 12 bytes
	ADOS_FIB_COMMENT        = 144 // 80 bytes (BSTR)
	ADOS_FIB_OWNER_UID      = 224
	ADOS_FIB_OWNER_GID      = 226
	ADOS_FIB_RESERVED       = 228
	ADOS_FIB_TOTAL_SIZE     = 260
)

// InfoData field offsets (36 bytes total, big-endian in guest memory)
const (
	ADOS_ID_NUM_SOFT_ERRORS = 0
	ADOS_ID_UNIT_NUMBER     = 4
	ADOS_ID_DISK_STATE      = 8
	ADOS_ID_NUM_BLOCKS      = 12
	ADOS_ID_NUM_BLOCKS_USED = 16
	ADOS_ID_BYTES_PER_BLOCK = 20
	ADOS_ID_DISK_TYPE       = 24
	ADOS_ID_VOLUME_NODE     = 28
	ADOS_ID_IN_USE          = 32
	ADOS_INFO_DATA_SIZE     = 36
)
