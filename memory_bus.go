// memory_bus.go - Memory bus for the Intuition Engine

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
memory_bus.go - Memory Bus for the Intuition Engine

This module implements the memory bus that forms the backbone of the Intuition Engine's memory subsystem. It provides a unified interface for 32-bit memory operations, including both standard memory access and memory-mapped I/O. The implementation emphasises thread safety, cache efficiency and precise control over memory layout, all of which are critical for accurate retro-style computer emulation.

Core Features:

    32MB of main memory allocated as a contiguous block.
    Support for memory-mapped I/O via an I/O region mapping table that uses page masking and fixed page sizes.
    Little-endian read/write operations for 32-bit data.
    Full memory reset capability to clear the entire memory state.
    Thread-safe access implemented with a read/write mutex to synchronise concurrent operations.

Technical Details:

    The SystemBus struct fulfils the MemoryBus interface, encapsulating the main memory and a mapping of I/O regions.
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
	"sync"
	"unsafe"
)

const (
	DEFAULT_MEMORY_SIZE = 32 * 1024 * 1024
	PAGE_SIZE           = 0x100
	PAGE_MASK           = 0xFFF00
)

type MemoryBus interface {
	/*
		MemoryBus defines the interface for memory operations
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

type SystemBus struct {
	/*
		SystemBus implements the MemoryBus interface and serves
		as the primary memory bus for the Intuition Engine.

		It maintains a contiguous block of main memory and a
		mapping of memory‐mapped I/O regions.

		Thread safety is enforced via a read/write mutex.
	*/

	memory  []byte
	mutex   sync.RWMutex
	mapping map[uint32][]IORegion

	// Lock-free fast path for VIDEO_STATUS (allows VBlank polling without blocking)
	videoStatusReader func(addr uint32) uint32
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

func (bus *SystemBus) Write32WithFault(addr uint32, value uint32) bool {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return true
					}
				}
			}
			bus.mutex.Unlock()
			return true
		}

		bus.mutex.Unlock()
		return false
	}

	// Normal bounds check for regular memory
	if addr+4 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				bus.mutex.Unlock()
				return true
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
	bus.mutex.Unlock()
	return true
}

func (bus *SystemBus) Read32WithFault(addr uint32) (uint32, bool) {
	// Lock-free fast path for VIDEO_STATUS (VBlank polling)
	if addr == 0xF0008 && bus.videoStatusReader != nil {
		return bus.videoStatusReader(addr), true
	}

	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return value, true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint32(bus.memory[mapped : mapped+4])
				bus.mutex.Unlock()
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := region.onRead(TERM_OUT)
						bus.mutex.Unlock()
						return result, true
					}
				}
			}
			bus.mutex.Unlock()
			return 0, true
		}

		bus.mutex.Unlock()
		return 0, false
	}

	// Check for out-of-bounds access
	if addr+4 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				bus.mutex.Unlock()
				return value, true
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
	bus.mutex.Unlock()
	return result, true
}

func (bus *SystemBus) Write16WithFault(addr uint32, value uint16) bool {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return true
					}
				}
			}
			bus.mutex.Unlock()
			return true
		}

		bus.mutex.Unlock()
		return false
	}

	// Normal bounds check for regular memory
	if addr+2 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
				bus.mutex.Unlock()
				return true
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
	bus.mutex.Unlock()
	return true
}

func (bus *SystemBus) Read16WithFault(addr uint32) (uint16, bool) {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return uint16(value), true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint16(bus.memory[mapped : mapped+2])
				bus.mutex.Unlock()
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint16(region.onRead(TERM_OUT))
						bus.mutex.Unlock()
						return result, true
					}
				}
			}
			bus.mutex.Unlock()
			return 0, true
		}

		bus.mutex.Unlock()
		return 0, false
	}

	// Check for out-of-bounds access
	if addr+2 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], uint16(value))
				bus.mutex.Unlock()
				return uint16(value), true
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint16(bus.memory[addr : addr+2])
	bus.mutex.Unlock()
	return result, true
}

func (bus *SystemBus) Write8WithFault(addr uint32, value uint8) bool {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return true
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				bus.memory[mapped] = value
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return true
					}
				}
			}
			bus.mutex.Unlock()
			return true
		}

		bus.mutex.Unlock()
		return false
	}

	// Normal bounds check for regular memory
	if addr >= uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return false
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				bus.memory[addr] = value
				bus.mutex.Unlock()
				return true
			}
		}
	}

	// Regular memory write
	bus.memory[addr] = value
	bus.mutex.Unlock()
	return true
}

func (bus *SystemBus) Read8WithFault(addr uint32) (uint8, bool) {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return uint8(value), true
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				result := bus.memory[mapped]
				bus.mutex.Unlock()
				return result, true
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint8(region.onRead(TERM_OUT))
						bus.mutex.Unlock()
						return result, true
					}
				}
			}
			bus.mutex.Unlock()
			return 0, true
		}

		bus.mutex.Unlock()
		return 0, false
	}

	// Check for out-of-bounds access
	if addr >= uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		return 0, false
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				bus.memory[addr] = uint8(value)
				bus.mutex.Unlock()
				return uint8(value), true
			}
		}
	}

	// Regular memory read
	result := bus.memory[addr]
	bus.mutex.Unlock()
	return result, true
}

func NewSystemBus() *SystemBus {
	/*
		NewSystemBus initialises and returns a new SystemBus instance.

		The function allocates a 32MB block of main memory and initialises
		the I/O mapping table.
	*/

	return &SystemBus{
		memory:  make([]byte, DEFAULT_MEMORY_SIZE),
		mapping: make(map[uint32][]IORegion),
	}
}

func (bus *SystemBus) GetMemory() []byte {
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
func (bus *SystemBus) SetVideoStatusReader(reader func(addr uint32) uint32) {
	bus.videoStatusReader = reader
}

func (bus *SystemBus) MapIO(start, end uint32, onRead func(addr uint32) uint32, onWrite func(addr uint32, value uint32)) {
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

func (bus *SystemBus) Write32(addr uint32, value uint32) {
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

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
		// No I/O on this page - lock-free write using unsafe pointer
		*(*uint32)(unsafe.Pointer(&bus.memory[addr])) = value
		return
	}

	// Has I/O mappings - use slow path with mutex
	bus.write32Slow(addr, value)
}

func (bus *SystemBus) write32Slow(addr uint32, value uint32) {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint32(bus.memory[mapped:mapped+4], value)
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return
					}
				}
			}
			bus.mutex.Unlock()
			return
		}

		// For other high addresses, just log and return safely
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write32 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr+4 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write32 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				bus.mutex.Unlock()
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
	bus.mutex.Unlock()
}

func (bus *SystemBus) Read32(addr uint32) uint32 {
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

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
		// No I/O on this page - lock-free read using unsafe pointer
		return *(*uint32)(unsafe.Pointer(&bus.memory[addr]))
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read32Slow(addr)
}

func (bus *SystemBus) read32Slow(addr uint32) uint32 {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return value
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+4 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint32(bus.memory[mapped : mapped+4])
				bus.mutex.Unlock()
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := region.onRead(TERM_OUT)
						bus.mutex.Unlock()
						return result
					}
				}
			}
			bus.mutex.Unlock()
			return 0
		}

		bus.mutex.Unlock()
		fmt.Printf("Warning: Read32 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr+4 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		fmt.Printf("Warning: Read32 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				bus.mutex.Unlock()
				return value
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
	bus.mutex.Unlock()
	return result
}

func (bus *SystemBus) Write16(addr uint32, value uint16) {
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

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
		// No I/O on this page - lock-free write using unsafe pointer
		*(*uint16)(unsafe.Pointer(&bus.memory[addr])) = value
		return
	}

	// Has I/O mappings - use slow path with mutex
	bus.write16Slow(addr, value)
}

func (bus *SystemBus) write16Slow(addr uint32, value uint16) {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				binary.LittleEndian.PutUint16(bus.memory[mapped:mapped+2], value)
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return
					}
				}
			}
			bus.mutex.Unlock()
			return
		}

		// For other high addresses, just log and return safely
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write16 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr+2 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write16 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
				bus.mutex.Unlock()
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], value)
	bus.mutex.Unlock()
}

func (bus *SystemBus) Read16(addr uint32) uint16 {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		return bus.read16Slow(addr)
	}

	// Bounds check
	if addr+2 > uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read16 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
		// No I/O on this page - lock-free read using unsafe pointer
		return *(*uint16)(unsafe.Pointer(&bus.memory[addr]))
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read16Slow(addr)
}

func (bus *SystemBus) read16Slow(addr uint32) uint16 {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return uint16(value)
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped+2 <= uint32(len(bus.memory)) {
				result := binary.LittleEndian.Uint16(bus.memory[mapped : mapped+2])
				bus.mutex.Unlock()
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint16(region.onRead(TERM_OUT))
						bus.mutex.Unlock()
						return result
					}
				}
			}
			bus.mutex.Unlock()
			return 0
		}

		bus.mutex.Unlock()
		fmt.Printf("Warning: Read16 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr+2 > uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		fmt.Printf("Warning: Read16 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Check for I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint16(bus.memory[addr:addr+2], uint16(value))
				bus.mutex.Unlock()
				return uint16(value)
			}
		}
	}

	// Regular memory read
	result := binary.LittleEndian.Uint16(bus.memory[addr : addr+2])
	bus.mutex.Unlock()
	return result
}

func (bus *SystemBus) Write8(addr uint32, value uint8) {
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

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
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
func (bus *SystemBus) WriteMemoryDirect(addr uint32, value uint8) {
	if addr < uint32(len(bus.memory)) {
		bus.memory[addr] = value
	}
}

func (bus *SystemBus) write8Slow(addr uint32, value uint8) {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return
					}
				}
			}

			// Proceed with writing to the mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				bus.memory[mapped] = value
				bus.mutex.Unlock()
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
						bus.mutex.Unlock()
						return
					}
				}
			}
			bus.mutex.Unlock()
			return
		}

		// For other high addresses, just log and return safely
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write8 to unmapped high address 0x%08X\n", addr)
		return
	}

	// Normal bounds check for regular memory
	if addr >= uint32(len(bus.memory)) {
		bus.mutex.Unlock()
		fmt.Printf("Warning: Write8 to out-of-bounds address 0x%08X\n", addr)
		return
	}

	// Process I/O regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onWrite != nil {
				region.onWrite(addr, uint32(value))
				bus.memory[addr] = value
				bus.mutex.Unlock()
				return
			}
		}
	}

	// Regular memory write
	bus.memory[addr] = value
	bus.mutex.Unlock()
}

func (bus *SystemBus) Read8(addr uint32) uint8 {
	// Skip sign-extended addresses (rare, use slow path)
	if addr >= 0xFFFF0000 {
		return bus.read8Slow(addr)
	}

	// Bounds check
	if addr >= uint32(len(bus.memory)) {
		fmt.Printf("Warning: Read8 from out-of-bounds address 0x%08X\n", addr)
		return 0
	}

	// Lock-free fast path: check if this page has ANY I/O mappings
	page := addr & PAGE_MASK
	if _, hasIO := bus.mapping[page]; !hasIO {
		// No I/O on this page - lock-free read
		return bus.memory[addr]
	}

	// Has I/O mappings - use slow path with mutex
	return bus.read8Slow(addr)
}

func (bus *SystemBus) read8Slow(addr uint32) uint8 {
	bus.mutex.Lock()

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
						bus.mutex.Unlock()
						return uint8(value)
					}
				}
			}

			// Regular memory read with mapped address if in bounds
			if mapped < uint32(len(bus.memory)) {
				result := bus.memory[mapped]
				bus.mutex.Unlock()
				return result
			}
		}

		// Special handling for terminal input
		if addr == TERM_OUT_SIGNEXT || addr == TERM_OUT_16BIT {
			if regions, exists := bus.mapping[TERM_OUT&PAGE_MASK]; exists {
				for _, region := range regions {
					if TERM_OUT >= region.start && TERM_OUT <= region.end && region.onRead != nil {
						result := uint8(region.onRead(TERM_OUT))
						bus.mutex.Unlock()
						return result
					}
				}
			}
			bus.mutex.Unlock()
			return 0
		}

		bus.mutex.Unlock()
		fmt.Printf("Warning: Read8 from unmapped high address 0x%08X\n", addr)
		return 0
	}

	// Check for out-of-bounds access
	if addr >= uint32(len(bus.memory)) {
		bus.mutex.Unlock()
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
				bus.mutex.Unlock()
				return uint8(value)
			}
		}
	}

	// Regular memory read
	result := bus.memory[addr]
	bus.mutex.Unlock()
	return result
}

func (bus *SystemBus) Reset() {
	/*
		Reset clears the entire main memory of the system bus.

		This operation is performed in a thread‐safe manner by
		acquiring a write lock, and iterating through the memory
		block to set every byte to zero.
	*/

	bus.mutex.Lock()
	for i := range bus.memory {
		bus.memory[i] = 0
	}
	bus.mutex.Unlock()
}
