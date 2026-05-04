package main

// GEMDOS function numbers (TRAP #1)
const (
	GEMDOS_DSETDRV  = 0x0E // Set default drive
	GEMDOS_FSETDTA  = 0x1A // Set DTA address
	GEMDOS_DGETDRV  = 0x19 // Get default drive
	GEMDOS_DFREE    = 0x36 // Get free disk space
	GEMDOS_DCREATE  = 0x39 // Create directory
	GEMDOS_DDELETE  = 0x3A // Delete directory
	GEMDOS_DSETPATH = 0x3B // Set current directory
	GEMDOS_FCREATE  = 0x3C // Create file
	GEMDOS_FOPEN    = 0x3D // Open file
	GEMDOS_FCLOSE   = 0x3E // Close file
	GEMDOS_FREAD    = 0x3F // Read from file
	GEMDOS_FWRITE   = 0x40 // Write to file
	GEMDOS_FDELETE  = 0x41 // Delete file
	GEMDOS_FSEEK    = 0x42 // Seek in file
	GEMDOS_FATTRIB  = 0x43 // Get/set file attributes
	GEMDOS_DGETPATH = 0x47 // Get current directory
	GEMDOS_MSHRINK  = 0x4A // Shrink memory block
	GEMDOS_PEXEC    = 0x4B // Execute program (Pexec)
	GEMDOS_PTERM    = 0x4C // Terminate process with exit code
	GEMDOS_FSFIRST  = 0x4E // Find first file
	GEMDOS_FSNEXT   = 0x4F // Find next file
	GEMDOS_FRENAME  = 0x56 // Rename file
	GEMDOS_FDATIME  = 0x57 // Get/set file date/time
)

// GEMDOS error codes (negative values)
const (
	GEMDOS_E_OK   = 0   // No error
	GEMDOS_EINVFN = -32 // Invalid function number
	GEMDOS_EFILNF = -33 // File not found
	GEMDOS_EPTHNF = -34 // Path not found
	GEMDOS_ENHNDL = -35 // No more handles
	GEMDOS_EACCDN = -36 // Access denied
	GEMDOS_EIHNDL = -37 // Invalid handle
	GEMDOS_ENSMEM = -39 // Insufficient memory
	GEMDOS_ERANGE = -64 // Range error
	GEMDOS_EINTRN = -65 // Internal error
	GEMDOS_EPLFMT = -66 // Invalid program load format
	GEMDOS_EIMBA  = -40 // Invalid memory block address
	GEMDOS_ENSAME = -48 // Not same drive (rename across drives)
	GEMDOS_ENMFIL = -49 // No more files (Fsnext exhausted)
)

// GEMDOS file open modes
const (
	GEMDOS_OPEN_READ  = 0
	GEMDOS_OPEN_WRITE = 1
	GEMDOS_OPEN_RW    = 2
)

// GEMDOS seek modes
const (
	GEMDOS_SEEK_SET = 0 // From beginning
	GEMDOS_SEEK_CUR = 1 // From current position
	GEMDOS_SEEK_END = 2 // From end
)

// GEMDOS file attributes
const (
	GEMDOS_ATTR_READONLY  = 0x01
	GEMDOS_ATTR_HIDDEN    = 0x02
	GEMDOS_ATTR_SYSTEM    = 0x04
	GEMDOS_ATTR_VOLUME    = 0x08
	GEMDOS_ATTR_DIRECTORY = 0x10
	GEMDOS_ATTR_ARCHIVE   = 0x20
)

// DTA (Disk Transfer Address) offsets
const (
	GEMDOS_DTA_RESERVED = 0  // 21 bytes reserved (search state)
	GEMDOS_DTA_ATTR     = 21 // 1 byte: file attributes
	GEMDOS_DTA_TIME     = 22 // 2 bytes: GEMDOS time
	GEMDOS_DTA_DATE     = 24 // 2 bytes: GEMDOS date
	GEMDOS_DTA_SIZE     = 26 // 4 bytes: file size
	GEMDOS_DTA_NAME     = 30 // 14 bytes: filename (null-terminated)
	GEMDOS_DTA_TOTAL    = 44 // Total DTA size
)

// Handle allocation
const (
	GEMDOS_HANDLE_MIN = 1000  // Start of our handle range (well above EmuTOS)
	GEMDOS_HANDLE_MAX = 32767 // int16 max
	GEMDOS_IO_CHUNK   = 64 << 10
)

// EmuTOS system variable for drive bitmap
const (
	GEMDOS_DRVBITS_ADDR = 0x4C2 // _drvbits: uint32 bitmap of available drives
)

// Pexec modes
const (
	GEMDOS_PEXEC_LOAD_GO = 0 // Load & Go
)

// TOS PRG header
const (
	TOS_PRG_MAGIC      = 0x601A // Magic number for TOS .PRG files
	TOS_PRG_HEADER_LEN = 28     // Size of TOS program header
	TOS_BASEPAGE_SIZE  = 256    // Size of basepage structure
)

// TPA allocation base — above EmuTOS-managed memory and VRAM ($100000-$4FFFFF)
const (
	PEXEC_TPA_BASE = 0x800000 // 8MB mark, well above EmuTOS and VRAM
)
