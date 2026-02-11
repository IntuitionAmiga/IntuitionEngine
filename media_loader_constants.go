package main

// MediaLoader MMIO Registers (0xF2300-0xF231F)
const (
	MEDIA_LOADER_BASE = 0xF2300

	MEDIA_NAME_PTR = MEDIA_LOADER_BASE + 0x00 // Pointer to null-terminated filename
	MEDIA_SUBSONG  = MEDIA_LOADER_BASE + 0x04 // Subsong selection
	MEDIA_CTRL     = MEDIA_LOADER_BASE + 0x08 // 1=play, 2=stop
	MEDIA_STATUS   = MEDIA_LOADER_BASE + 0x0C // 0=idle,1=loading,2=playing,3=error
	MEDIA_TYPE     = MEDIA_LOADER_BASE + 0x10 // 1=SID,2=PSG,3=TED,4=AHX
	MEDIA_ERROR    = MEDIA_LOADER_BASE + 0x14 // 0=ok,1=not-found,2=bad-format/io,3=unsupported,4=path-invalid,5=too-large

	MEDIA_LOADER_END = MEDIA_LOADER_BASE + 0x1F
)

// Media loader control operations.
const (
	MEDIA_OP_PLAY = 1
	MEDIA_OP_STOP = 2
)

// Media status values.
const (
	MEDIA_STATUS_IDLE = iota
	MEDIA_STATUS_LOADING
	MEDIA_STATUS_PLAYING
	MEDIA_STATUS_ERROR
)

// Media type values.
const (
	MEDIA_TYPE_NONE = iota
	MEDIA_TYPE_SID
	MEDIA_TYPE_PSG
	MEDIA_TYPE_TED
	MEDIA_TYPE_AHX
)

// Media error values.
const (
	MEDIA_ERR_OK = iota
	MEDIA_ERR_NOT_FOUND
	MEDIA_ERR_BAD_FORMAT
	MEDIA_ERR_UNSUPPORTED
	MEDIA_ERR_PATH_INVALID
	MEDIA_ERR_TOO_LARGE
)

// Staging buffer used for transient media payload copies.
// Placed near FILE_DATA_BUF (0x700000), above EhBASIC vars and stack,
// within DEFAULT_MEMORY_SIZE (32MB). Not MapIO-registered, so bus
// treats it as regular memory regardless of IO_REGION_START threshold.
const (
	MEDIA_STAGING_BASE = 0x800000
	MEDIA_STAGING_SIZE = 0x00010000 // 64KB
	MEDIA_STAGING_END  = MEDIA_STAGING_BASE + MEDIA_STAGING_SIZE - 1
)
