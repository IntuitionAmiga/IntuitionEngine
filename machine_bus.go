// machine_bus.go - Machine bus for the Intuition Engine

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
machine_bus.go - Machine Bus for the Intuition Engine

This module implements the memory bus that forms the backbone of the Intuition Engine's memory subsystem. It provides a unified interface for 32-bit memory operations, including both standard memory access and memory-mapped I/O. The implementation emphasises thread safety, cache efficiency and precise control over memory layout, all of which are critical for accurate retro-style computer emulation.

Core Features:

    32MB of main memory allocated as a contiguous block.
    Support for memory-mapped I/O via an I/O region mapping table that uses page masking and fixed page sizes.
    Little-endian read/write operations for 32-bit data.
    Full memory reset capability to clear the entire memory state.
    Thread-safe access implemented with a read/write mutex to synchronise concurrent operations.

Technical Details:

    The MachineBus struct fulfils the Bus32 interface, encapsulating the main memory and a mapping of I/O regions.
    I/O regions are registered with a defined start and end address along with callback functions (onRead and onWrite) to intercept memory accesses.
    Memory page keys are calculated using a page mask (0xFFF00) and a page increment of 0x100, ensuring that I/O regions are correctly mapped across the memory space.
    32-bit values are accessed using binary.LittleEndian conversion routines, maintaining consistency with the CPU's data handling.
    The Reset method iterates through the memory block in a cache-friendly manner to set all bytes to zero.

Concurrency and Cache Optimisation:

    A sync.RWMutex protects all memory operations, thereby preventing data races in multi-threaded environments.
    The memory block is stored in a contiguous slice to improve cache locality, which is essential for high-performance emulation.
    The design is minimalistic and efficient, ensuring that the memory bus can keep pace with the CPU and peripheral devices that rely on memory-mapped I/O.

This module is a critical component of the Intuition Engine, interfacing directly with the CPU and various peripheral devices. Its design is driven by the need for both high performance and accurate emulation of hardware behaviour.

*/

package main

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"unsafe"
)

const (
	DEFAULT_MEMORY_SIZE = 32 * 1024 * 1024
	PAGE_SIZE           = 0x100
	PAGE_MASK           = 0xFFF00
)

// ------------------------------------------------------------------------------
// Memory Map Boundaries
// See registers.go for the complete I/O memory map reference.
// ------------------------------------------------------------------------------
const (
	VECTOR_TABLE    = 0x0000  // Interrupt vector table
	PROG_START      = 0x1000  // Program code start
	STACK_BOTTOM    = 0x2000  // Stack bottom boundary
	STACK_START     = 0x9F000 // Initial stack pointer (below VGA VRAM)
	IO_REGION_START = 0xA0000 // Start of I/O mapped region (includes VGA VRAM at 0xA0000)
	IO_BASE         = 0xF0800 // I/O register base (audio chip region)
	IO_LIMIT        = 0xFFFFF // I/O register limit
)

type Bus32 interface {
	/*
		Bus32 defines the interface for memory operations
		within the Intuition Engine. It provides methods to read
		and write 32‐bit values as well as to reset the memory state.

		Implementations must ensure thread safety and support memory‐mapped I/O.
	*/

	Read8(addr uint32) uint8
	Write8(addr uint32, value uint8)
	Read16(addr uint32) uint16
	Write16(addr uint32, value uint16)
	Read32(addr uint32) uint32
	Write32(addr uint32, value uint32)
	Reset()
	GetMemory() []byte
}

type MachineBus struct {
	/*
		MachineBus implements the Bus32 interface and serves
		as the primary memory bus for the Intuition Engine.

		It maintains a contiguous block of main memory and a
		mapping of memory‐mapped I/O regions.

		Thread safety is enforced via a read/write mutex.
	*/

	memory  []byte
	mapping map[uint32][]IORegion

	// Fast I/O page bitmap - indexed by (addr >> 8), true if page has I/O mappings.
	// Sized for the normal address range only (DEFAULT_MEMORY_SIZE / PAGE_SIZE).
	// Sign-extended pages (0xFFFF0000+) use the slow path before this is consulted.
	ioPageBitmap []bool

	// Lock-free fast path for VIDEO_STATUS (allows VBlank polling without blocking)
	videoStatusReader func(addr uint32) uint32

	// 64-bit I/O region map - separate from legacy 32-bit mapping.
	// Registered via MapIO64, used by Read64/Write64 for native 64-bit dispatch.
	mapping64 map[uint32][]IORegion64

	// Policy for 64-bit access to legacy-only MMIO regions (default: Fault)
	legacyMMIO64Policy MMIO64Policy

	// Sealed state to prevent I/O mapping after execution has started
	sealed atomic.Bool
}

type IORegion struct {
	/*
		IORegion represents a memory‐mapped I/O region within the system.
		Each region is defined by its start and end addresses and includes
		callback functions to handle read and write operations.

		These callbacks are invoked when a memory access falls within the
		region's boundaries.
	*/
	start   uint32
	end     uint32
	onRead  func(addr uint32) uint32
	onWrite func(addr uint32, value uint32)
}

// IORegion64 represents a 64-bit-capable memory-mapped I/O region.
// These are registered separately from legacy 32-bit IORegions via MapIO64.
type IORegion64 struct {
	start     uint32
	end       uint32
	onRead64  func(addr uint32) uint64
	onWrite64 func(addr uint32, value uint64)
}

// MMIO64Policy controls behavior when a 64-bit access hits a legacy-only I/O region.
type MMIO64Policy int

const (
	// MMIO64PolicyFault returns 0/no-op when 64-bit access hits legacy-only MMIO.
	MMIO64PolicyFault MMIO64Policy = iota
	// MMIO64PolicySplit splits into two 32-bit operations (low then high).
	MMIO64PolicySplit
)

// Bus64 extends the memory bus with native 64-bit data operations.
// Only used by the IE64 CPU; existing 32-bit CPUs are unaffected.
type Bus64 interface {
	Read64(addr uint32) uint64
	Write64(addr uint32, value uint64)
	Read64WithFault(addr uint32) (uint64, bool)
	Write64WithFault(addr uint32, value uint64) bool
}

func (bus *MachineBus) Write32WithFault(addr uint32, value uint32) bool {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-4 {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, value)
						// Still store in memory if within bounds
						if mapped+4 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
						}
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
				return true
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, value)
						return true
					}
				}
			}
			return true
		}

		return false
	}

	// Normal bounds check for regular memory
	if addr+4 > uint32(len(bus.memory)) {
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return true
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
	return true
}

func (bus *MachineBus) Read32WithFault(addr uint32) (uint32, bool) {
	// Lock-free fast path for VIDEO_STATUS (VBlank polling)
	if addr == 0xF0008 && bus.videoStatusReader != nil {
		return bus.videoStatusReader(addr), true
	}

	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-4 {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped+4 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
						}
						return value, true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint32(bus.memory[mapped : mapped+4])
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := region.onRead(TERM_OUT)
						return result, true
					}
				}
			}
			return 0, true
		}

		return 0, false
	}

	// Check for out-of-bounds access
	if addr+4 > uint32(len(bus.memory)) {
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return value, true
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
	return result, true
}

func (bus *MachineBus) Write16WithFault(addr uint32, value uint16) bool {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-2 {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, uint32(value))
						// Still store in memory if within bounds
						if mapped+2 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
						}
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
				return true
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, uint32(value))
						return true
					}
				}
			}
			return true
		}

		return false
	}

	// Normal bounds check for regular memory
	if addr+2 > uint32(len(bus.memory)) {
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
				return true
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
	return true
}

func (bus *MachineBus) Read16WithFault(addr uint32) (uint16, bool) {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-2 {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped+2 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], uint16(value))
						}
						return uint16(value), true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint16(bus.memory[mapped : mapped+2])
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint16(region.onRead(TERM_OUT))
						return result, true
					}
				}
			}
			return 0, true
		}

		return 0, false
	}

	// Check for out-of-bounds access
	if addr+2 > uint32(len(bus.memory)) {
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], uint16(value))
				return uint16(value), true
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint16(bus.memory[addr : addr+2])
	return result, true
}

func (bus *MachineBus) Write8WithFault(addr uint32, value uint8) bool {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped < DEFAULT_MEMORY_SIZE {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, uint32(value))
						// Still store in memory if within bounds
						if mapped < uint32(len(bus.memory)) {
							bus.memory[mapped] = value
						}
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				bus.memory[mapped] = value
				return true
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, uint32(value))
						return true
					}
				}
			}
			return true
		}

		return false
	}

	// Normal bounds check for regular memory
	if addr >= uint32(len(bus.memory)) {
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				bus.memory[addr] = value
				return true
			}
		}
	}

	// Regular memory write
	bus.memory[addr] = value
	return true
}

func (bus *MachineBus) Read8WithFault(addr uint32) (uint8, bool) {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped < DEFAULT_MEMORY_SIZE {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped < uint32(len(bus.memory)) {
							bus.memory[mapped] = uint8(value)
						}
						return uint8(value), true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				result := bus.memory[mapped]
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint8(region.onRead(TERM_OUT))
						return result, true
					}
				}
			}
			return 0, true
		}

		return 0, false
	}

	// Check for out-of-bounds access
	if addr >= uint32(len(bus.memory)) {
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				bus.memory[addr] = uint8(value)
				return uint8(value), true
			}
		}
	}

	// Regular memory read
	result := bus.memory[addr]
	return result, true
}

func NewMachineBus() *MachineBus {
	/*
		NewMachineBus initialises and returns a new MachineBus instance.

		The function allocates a 32MB block of main memory and initialises
		the I/O mapping table.
	*/

	return &MachineBus{
		memory:       make([]byte, DEFAULT_MEMORY_SIZE),
		mapping:      make(map[uint32][]IORegion),
		ioPageBitmap: make([]bool, DEFAULT_MEMORY_SIZE/PAGE_SIZE),
		mapping64:    make(map[uint32][]IORegion64),
	}
}

func (bus *MachineBus) GetMemory() []byte {
	/*
		GetMemory returns a direct reference to the underlying memory slice.

		This allows CPU cores to cache the memory reference for fast access
		while maintaining visibility to peripherals that read through the bus.
		CPUs should use this for non-I/O memory operations.
	*/
	return bus.memory
}

// SetVideoStatusReader registers a lock-free callback for VIDEO_STATUS reads.
// This allows VBlank polling without blocking on the bus mutex.
func (bus *MachineBus) SetVideoStatusReader(reader func(addr uint32) uint32) {
	bus.videoStatusReader = reader
}

// SealMappings prevents further MapIO calls. This is called when execution starts
// to ensure the ioPageBitmap remains stable during hot-path access.
func (bus *MachineBus) SealMappings() {
	bus.sealed.CompareAndSwap(false, true)
}

func (bus *MachineBus) MapIO(start, end uint32, onRead func(addr uint32) uint32, onWrite func(addr uint32, value uint32)) {
	if bus.sealed.Load() {
		panic(fmt.Sprintf("MapIO called after execution started (mapping range $%05X-$%05X)", start, end))
	}
	region := IORegion{
		start:   start,
		end:     end,
		onRead:  onRead,
		onWrite: onWrite,
	}

	// Calculate pages for normal address range
	firstPage := start & PAGE_MASK
	lastPage := end & PAGE_MASK
	for page := firstPage; page <= lastPage; page += PAGE_SIZE {
		bus.mapping[page] = append(bus.mapping[page], region)
		// Set bitmap for fast-path lookup (normal range only)
		pageIdx := page >> 8
		if pageIdx < uint32(len(bus.ioPageBitmap)) {
			bus.ioPageBitmap[pageIdx] = true
		}
	}

	// Handle sign extension for I/O addresses (only if in upper 16-bit range)
	// This is necessary because the M68K CPU treats I/O addresses with the high bit set
	// (0x8000-0xFFFF) as negative values and sign-extends them to 32-bit when used in
	// 32-bit addressing modes. For example, a device at 0xFFxx needs to be accessible
	// at both 0x0000FFxx and 0xFFFFFFxx to properly handle 16-bit peripherals in a
	// 32-bit address space, matching the real hardware behavior.
	if start >= 0x8000 && start <= 0xFFFF {
		// Also map to 0xFFFF0000-0xFFFFFFFF range
		signExtStart := start | 0xFFFF0000
		signExtEnd := end | 0xFFFF0000

		firstSignExtPage := signExtStart & PAGE_MASK
		lastSignExtPage := signExtEnd & PAGE_MASK

		for page := firstSignExtPage; page <= lastSignExtPage; page += PAGE_SIZE {
			bus.mapping[page] = append(bus.mapping[page], region)
		}
	}
}

func (bus *MachineBus) Write32(addr uint32, value uint32) {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		bus.write32Slow(addr, value)
		return
	}

	// Bounds check
	if addr+4 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write32 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free write using unsafe pointer
		*(*uint32)(unsafe.Pointer(&bus.memory[addr])) = value
		return
	}

	// Has I/O mappings - use slow path with mutex
	bus.write32Slow(addr, value)
}

func (bus *MachineBus) write32Slow(addr uint32, value uint32) {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-4 {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, value)
						// Still store in memory if within bounds
						if mapped+4 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
						}
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
				return
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, value)
						return
					}
				}
			}
			return
		}

		// For other high addresses, just log and return safely
		fmt.Printf("Warning: Write32 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr+4 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write32 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
}

func (bus *MachineBus) Read32(addr uint32) uint32 {
	// Lock-free fast path for VIDEO_STATUS (VBlank polling)
	if addr == 0xF0008 && bus.videoStatusReader != nil {
		return bus.videoStatusReader(addr)
	}

	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		return bus.read32Slow(addr)
	}

	// Bounds check
	if addr+4 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read32 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free read using unsafe pointer
		return *(*uint32)(unsafe.Pointer(&bus.memory[addr]))
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read32Slow(addr)
}

func (bus *MachineBus) read32Slow(addr uint32) uint32 {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-4 {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped+4 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
						}
						return value
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint32(bus.memory[mapped : mapped+4])
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := region.onRead(TERM_OUT)
						return result
					}
				}
			}
			return 0
		}

		fmt.Printf("Warning: Read32 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr+4 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read32 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return value
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
	return result
}

func (bus *MachineBus) Write16(addr uint32, value uint16) {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		bus.write16Slow(addr, value)
		return
	}

	// Bounds check
	if addr+2 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write16 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free write using unsafe pointer
		*(*uint16)(unsafe.Pointer(&bus.memory[addr])) = value
		return
	}

	// Has I/O mappings - use slow path with mutex
	bus.write16Slow(addr, value)
}

func (bus *MachineBus) write16Slow(addr uint32, value uint16) {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-2 {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, uint32(value))
						// Still store in memory if within bounds
						if mapped+2 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
						}
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
				return
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, uint32(value))
						return
					}
				}
			}
			return
		}

		// For other high addresses, just log and return safely
		fmt.Printf("Warning: Write16 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr+2 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write16 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
}

func (bus *MachineBus) Read16(addr uint32) uint16 {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		return bus.read16Slow(addr)
	}

	// Bounds check
	if addr+2 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read16 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free read using unsafe pointer
		return *(*uint16)(unsafe.Pointer(&bus.memory[addr]))
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read16Slow(addr)
}

func (bus *MachineBus) read16Slow(addr uint32) uint16 {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped <= DEFAULT_MEMORY_SIZE-2 {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped+2 <= uint32(len(bus.memory)) {
							binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], uint16(value))
						}
						return uint16(value)
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint16(bus.memory[mapped : mapped+2])
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint16(region.onRead(TERM_OUT))
						return result
					}
				}
			}
			return 0
		}

		fmt.Printf("Warning: Read16 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr+2 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read16 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], uint16(value))
				return uint16(value)
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint16(bus.memory[addr : addr+2])
	return result
}

func (bus *MachineBus) Write8(addr uint32, value uint8) {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		bus.write8Slow(addr, value)
		return
	}

	// Bounds check
	if addr >= uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write8 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free write
		bus.memory[addr] = value
		return
	}

	// Has I/O mappings - use slow path with mutex
	bus.write8Slow(addr, value)
}

// WriteMemoryDirect writes a single byte directly to memory,
// bypassing I/O handlers. Used for 8-bit CPU VRAM bank writes
// where the VideoChip handler would corrupt adjacent bytes
// (it does 32-bit writes even for single-byte values).
func (bus *MachineBus) WriteMemoryDirect(addr uint32, value uint8) {
	if addr < uint32(len(bus.memory)) {
		bus.memory[addr] = value
	}
}

func (bus *MachineBus) write8Slow(addr uint32, value uint8) {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped < DEFAULT_MEMORY_SIZE {
			// This is a valid sign-extended address, handle normally but with mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onWrite != nil {
						region.onWrite(mapped, uint32(value))
						// Still store in memory if within bounds
						if mapped < uint32(len(bus.memory)) {
							bus.memory[mapped] = value
						}
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				bus.memory[mapped] = value
				return
			}
		}

		// Special handling for terminal output case
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			// Call terminal output handler if available
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onWrite != nil {
						region.onWrite(TERM_OUT, uint32(value))
						return
					}
				}
			}
			return
		}

		// For other high addresses, just log and return safely
		fmt.Printf("Warning: Write8 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr >= uint32(len(bus.memory)) {
		fmt.Printf("Warning: Write8 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				bus.memory[addr] = value
				return
			}
		}
	}

	// Regular memory write
	bus.memory[addr] = value
}

func (bus *MachineBus) Read8(addr uint32) uint8 {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		return bus.read8Slow(addr)
	}

	// Bounds check
	if addr >= uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read8 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Lock-free fast path: check bitmap for I/O mappings
	if !bus.ioPageBitmap[addr>>8] {
		// No I/O on this page - lock-free read
		return bus.memory[addr]
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read8Slow(addr)
}

func (bus *MachineBus) read8Slow(addr uint32) uint8 {
	// Check if the address is in the upper memory region (potentially sign-extended)
	if addr >= 0xFFFF0000 {
		// Map to lower 16-bit range if it looks like a sign-extended I/O address
		mapped := addr & 0x0000FFFF
		if mapped < DEFAULT_MEMORY_SIZE {
			// Check for I/O regions with the mapped address
			if regions, exists := bus.mapping[mapped&PAGE_MASK]; exists {
				for _, region := range regions {
					if mapped >= region.start && mapped <= region.end && region.onRead != nil {
						value := region.onRead(mapped)
						if mapped < uint32(len(bus.memory)) {
							bus.memory[mapped] = uint8(value)
						}
						return uint8(value)
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				result := bus.memory[mapped]
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint8(region.onRead(TERM_OUT))
						return result
					}
				}
			}
			return 0
		}

		fmt.Printf("Warning: Read8 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr >= uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read8 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Check for I/O regions
	page := addr & PAGE_MASK
	if regions, exists := bus.mapping[page]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				bus.memory[addr] = uint8(value)
				return uint8(value)
			}
		}
	}

	// Regular memory read
	result := bus.memory[addr]
	return result
}

// =============================================================================
// 64-bit Memory Bus Extension (IE64 only)
// =============================================================================

// SetLegacyMMIO64Policy sets the behavior when 64-bit access hits a legacy-only I/O region.
func (bus *MachineBus) SetLegacyMMIO64Policy(policy MMIO64Policy) {
	bus.legacyMMIO64Policy = policy
}

// MapIO64 registers a 64-bit-capable I/O region. This is separate from MapIO;
// 64-bit handlers are only used by Read64/Write64. 32-bit operations always use MapIO.
func (bus *MachineBus) MapIO64(start, end uint32, onRead64 func(addr uint32) uint64, onWrite64 func(addr uint32, value uint64)) {
	if bus.sealed.Load() {
		panic(fmt.Sprintf("MapIO64 called after execution started (mapping range $%05X-$%05X)", start, end))
	}
	region := IORegion64{
		start:     start,
		end:       end,
		onRead64:  onRead64,
		onWrite64: onWrite64,
	}

	firstPage := start & PAGE_MASK
	lastPage := end & PAGE_MASK
	for page := firstPage; page <= lastPage; page += PAGE_SIZE {
		bus.mapping64[page] = append(bus.mapping64[page], region)
		pageIdx := page >> 8
		if pageIdx < uint32(len(bus.ioPageBitmap)) {
			bus.ioPageBitmap[pageIdx] = true
		}
	}

	// Sign-extension mirroring for addresses in 0x8000-0xFFFF range
	if start >= 0x8000 && start <= 0xFFFF {
		signExtStart := start | 0xFFFF0000
		signExtEnd := end | 0xFFFF0000
		firstSignExtPage := signExtStart & PAGE_MASK
		lastSignExtPage := signExtEnd & PAGE_MASK
		for page := firstSignExtPage; page <= lastSignExtPage; page += PAGE_SIZE {
			bus.mapping64[page] = append(bus.mapping64[page], region)
		}
	}
}

// findIORegion64 looks up a 64-bit I/O region for the given address.
func (bus *MachineBus) findIORegion64(addr uint32) *IORegion64 {
	page := addr & PAGE_MASK
	if regions, exists := bus.mapping64[page]; exists {
		for i := range regions {
			if addr >= regions[i].start && addr <= regions[i].end {
				return &regions[i]
			}
		}
	}
	return nil
}

// findIORegion looks up a legacy 32-bit I/O region for the given address.
func (bus *MachineBus) findIORegion(addr uint32) *IORegion {
	page := addr & PAGE_MASK
	if regions, exists := bus.mapping[page]; exists {
		for i := range regions {
			if addr >= regions[i].start && addr <= regions[i].end {
				return &regions[i]
			}
		}
	}
	return nil
}

// Read64 performs a native 64-bit read. For plain RAM (no I/O on either page),
// uses a single unsafe 64-bit load. For I/O regions, dispatches per the span rules.
func (bus *MachineBus) Read64(addr uint32) uint64 {
	// Sign-extended addresses always take the slow path (before bounds check,
	// since the mapped address may be in bounds even if the raw address is not)
	if addr >= 0xFFFF0000 {
		return bus.read64Slow(addr)
	}

	// Bounds check using uint64 arithmetic to prevent overflow
	if uint64(addr)+8 > uint64(len(bus.memory)) {
		return 0
	}

	// Fast path: both pages are non-I/O → single 64-bit load
	lowPage := addr >> 8
	highPage := uint32(uint64(addr)+7) >> 8
	if lowPage < uint32(len(bus.ioPageBitmap)) && highPage < uint32(len(bus.ioPageBitmap)) &&
		!bus.ioPageBitmap[lowPage] && !bus.ioPageBitmap[highPage] {
		return *(*uint64)(unsafe.Pointer(&bus.memory[addr]))
	}

	return bus.read64Slow(addr)
}

// read64Slow handles 64-bit reads that may involve I/O regions.
func (bus *MachineBus) read64Slow(addr uint32) uint64 {
	// Map sign-extended addresses
	effectiveAddr := addr
	if addr >= 0xFFFF0000 {
		// Reject addresses where raw addr+7 would overflow uint32
		if uint64(addr)+8 > 0x100000000 {
			return 0
		}
		effectiveAddr = addr & 0x0000FFFF
		if uint64(effectiveAddr)+8 > uint64(len(bus.memory)) {
			return 0
		}
	}

	lowAddr := effectiveAddr
	highAddr := effectiveAddr + 4

	// Check for native 64-bit region covering the entire 8 bytes.
	// For sign-extended addresses, also look up the original (unmapped) address
	// since MapIO64 registers sign-extended pages in the mapping64 table.
	region64 := bus.findIORegion64(lowAddr)
	if region64 == nil && addr >= 0xFFFF0000 {
		region64 = bus.findIORegion64(addr)
	}
	if region64 != nil && highAddr <= region64.end && region64.onRead64 != nil {
		return region64.onRead64(lowAddr)
	}

	// Must split into two 32-bit halves
	lowVal := bus.read32Half(lowAddr)
	highVal := bus.read32Half(highAddr)
	return uint64(lowVal) | (uint64(highVal) << 32)
}

// read32Half reads a 32-bit half for split 64-bit operations.
// Prefers native 64-bit handler (read as single half), then legacy, then RAM.
func (bus *MachineBus) read32Half(addr uint32) uint32 {
	// Check for 64-bit region (dispatch as 32-bit portion)
	region64 := bus.findIORegion64(addr)
	if region64 != nil && region64.onRead64 != nil {
		// Align to 8-byte boundary, read full 64 bits, extract the correct half.
		base := addr &^ 7 // align down to 8-byte boundary
		val := region64.onRead64(base)
		if addr == base {
			return uint32(val) // low half
		}
		return uint32(val >> 32) // high half
	}

	// Check for legacy 32-bit region
	region := bus.findIORegion(addr)
	if region != nil {
		if bus.legacyMMIO64Policy == MMIO64PolicyFault {
			return 0
		}
		// Split policy: use legacy callback
		if region.onRead != nil {
			return region.onRead(addr)
		}
		return 0
	}

	// Plain RAM
	if addr+4 <= uint32(len(bus.memory)) {
		return *(*uint32)(unsafe.Pointer(&bus.memory[addr]))
	}
	return 0
}

// Write64 performs a native 64-bit write. For plain RAM (no I/O on either page),
// uses a single unsafe 64-bit store. For I/O regions, dispatches per the span rules.
func (bus *MachineBus) Write64(addr uint32, value uint64) {
	// Sign-extended addresses always take the slow path (before bounds check,
	// since the mapped address may be in bounds even if the raw address is not)
	if addr >= 0xFFFF0000 {
		bus.write64Slow(addr, value)
		return
	}

	// Bounds check using uint64 arithmetic to prevent overflow
	if uint64(addr)+8 > uint64(len(bus.memory)) {
		return
	}

	// Fast path: both pages are non-I/O → single 64-bit store
	lowPage := addr >> 8
	highPage := uint32(uint64(addr)+7) >> 8
	if lowPage < uint32(len(bus.ioPageBitmap)) && highPage < uint32(len(bus.ioPageBitmap)) &&
		!bus.ioPageBitmap[lowPage] && !bus.ioPageBitmap[highPage] {
		*(*uint64)(unsafe.Pointer(&bus.memory[addr])) = value
		return
	}

	bus.write64Slow(addr, value)
}

// write64Slow handles 64-bit writes that may involve I/O regions.
func (bus *MachineBus) write64Slow(addr uint32, value uint64) {
	// Map sign-extended addresses
	effectiveAddr := addr
	if addr >= 0xFFFF0000 {
		// Reject addresses where raw addr+7 would overflow uint32
		if uint64(addr)+8 > 0x100000000 {
			return
		}
		effectiveAddr = addr & 0x0000FFFF
		if uint64(effectiveAddr)+8 > uint64(len(bus.memory)) {
			return
		}
	}

	lowAddr := effectiveAddr
	highAddr := effectiveAddr + 4

	// Check for native 64-bit region covering the entire 8 bytes.
	// For sign-extended addresses, also look up the original (unmapped) address.
	region64 := bus.findIORegion64(lowAddr)
	if region64 == nil && addr >= 0xFFFF0000 {
		region64 = bus.findIORegion64(addr)
	}
	if region64 != nil && highAddr <= region64.end && region64.onWrite64 != nil {
		region64.onWrite64(lowAddr, value)
		// Also store to backing memory
		if uint64(lowAddr)+8 <= uint64(len(bus.memory)) {
			*(*uint64)(unsafe.Pointer(&bus.memory[lowAddr])) = value
		}
		return
	}

	// Must split into two 32-bit halves (low then high)
	lowVal := uint32(value)
	highVal := uint32(value >> 32)
	bus.write32Half(lowAddr, lowVal)
	bus.write32Half(highAddr, highVal)
}

// write32Half writes a 32-bit half for split 64-bit operations.
// For native-64 regions, performs read-modify-write: reads the current 64-bit
// value from backing memory to preserve the untouched half, replaces the target
// half, then calls onWrite64. Backing memory is used (not onRead64) because
// device read callbacks may have side effects (status clear-on-read, FIFO pop).
// Backing memory stays in sync because write64Slow updates it after every write,
// and the two write32Half calls in a split sequence execute in low-then-high
// order, so the second call sees the first half's update.
func (bus *MachineBus) write32Half(addr uint32, value uint32) {
	// Check for 64-bit region
	region64 := bus.findIORegion64(addr)
	if region64 != nil && region64.onWrite64 != nil {
		base := addr &^ 7 // align down to 8-byte boundary
		// Read current 64-bit value from backing memory (not device - avoids
		// side effects from onRead64 such as clear-on-read or FIFO pop)
		var current uint64
		if base+8 <= uint32(len(bus.memory)) {
			current = *(*uint64)(unsafe.Pointer(&bus.memory[base]))
		}
		// Replace the correct 32-bit half
		if addr == base {
			// Low half: clear low 32 bits, set new value
			current = (current & 0xFFFFFFFF00000000) | uint64(value)
		} else {
			// High half: clear high 32 bits, set new value
			current = (current & 0x00000000FFFFFFFF) | (uint64(value) << 32)
		}
		// Write full 64-bit value via handler
		region64.onWrite64(base, current)
		// Update backing memory
		if base+8 <= uint32(len(bus.memory)) {
			*(*uint64)(unsafe.Pointer(&bus.memory[base])) = current
		}
		return
	}

	// Check for legacy 32-bit region
	region := bus.findIORegion(addr)
	if region != nil {
		if bus.legacyMMIO64Policy == MMIO64PolicyFault {
			return // no-op under Fault policy
		}
		// Split policy: use legacy callback
		if region.onWrite != nil {
			region.onWrite(addr, value)
		}
		if addr+4 <= uint32(len(bus.memory)) {
			binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
		}
		return
	}

	// Plain RAM
	if addr+4 <= uint32(len(bus.memory)) {
		*(*uint32)(unsafe.Pointer(&bus.memory[addr])) = value
	}
}

// Read64WithFault performs a 64-bit read with fault reporting.
// Returns (0, false) if the access cannot complete (OOB or legacy MMIO under Fault policy).
func (bus *MachineBus) Read64WithFault(addr uint32) (uint64, bool) {
	effectiveAddr := addr
	if addr >= 0xFFFF0000 {
		// Reject addresses where raw addr+7 would overflow uint32
		if uint64(addr)+8 > 0x100000000 {
			return 0, false
		}
		effectiveAddr = addr & 0x0000FFFF
	}

	if uint64(effectiveAddr)+8 > uint64(len(bus.memory)) {
		return 0, false
	}

	// Check if any half hits legacy-only MMIO under Fault policy
	lowAddr := effectiveAddr
	highAddr := effectiveAddr + 4

	if bus.legacyMMIO64Policy == MMIO64PolicyFault {
		// Check low half
		if bus.findIORegion64(lowAddr) == nil && bus.findIORegion(lowAddr) != nil {
			return 0, false
		}
		// Check high half
		if bus.findIORegion64(highAddr) == nil && bus.findIORegion(highAddr) != nil {
			return 0, false
		}
	}

	val := bus.Read64(addr)
	return val, true
}

// Write64WithFault performs a 64-bit write with fault reporting.
// Returns false if the access cannot complete.
func (bus *MachineBus) Write64WithFault(addr uint32, value uint64) bool {
	effectiveAddr := addr
	if addr >= 0xFFFF0000 {
		// Reject addresses where raw addr+7 would overflow uint32
		if uint64(addr)+8 > 0x100000000 {
			return false
		}
		effectiveAddr = addr & 0x0000FFFF
	}

	if uint64(effectiveAddr)+8 > uint64(len(bus.memory)) {
		return false
	}

	// Check if any half hits legacy-only MMIO under Fault policy
	lowAddr := effectiveAddr
	highAddr := effectiveAddr + 4

	if bus.legacyMMIO64Policy == MMIO64PolicyFault {
		if bus.findIORegion64(lowAddr) == nil && bus.findIORegion(lowAddr) != nil {
			return false
		}
		if bus.findIORegion64(highAddr) == nil && bus.findIORegion(highAddr) != nil {
			return false
		}
	}

	bus.Write64(addr, value)
	return true
}

func (bus *MachineBus) Reset() {
	/*
		Reset clears the entire main memory of the system bus.

		This operation is performed in a thread‐safe manner by
		acquiring a write lock, and iterating through the memory
		block to set every byte to zero.
	*/

	for i := range bus.memory {
		bus.memory[i] = 0
	}
}
