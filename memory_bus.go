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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
memory_bus.go - Memory Bus for the Intuition Engine

This module implements the memory bus that forms the backbone of the Intuition Engine's memory subsystem. It provides a unified interface for 32-bit memory operations, including both standard memory access and memory-mapped I/O. The implementation emphasises thread safety, cache efficiency and precise control over memory layout, all of which are critical for accurate retro-style computer emulation.

Core Features:

    16MB of main memory allocated as a contiguous block.
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
	"sync"
)

const (
	DEFAULT_MEMORY_SIZE = 16 * 1024 * 1024
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

	Read32(addr uint32) uint32
	Write32(addr uint32, value uint32)
	Reset()
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

func NewSystemBus() *SystemBus {
	/*
		NewSystemBus initialises and returns a new SystemBus instance.

		The function allocates a 16MB block of main memory and initialises
		the I/O mapping table.
	*/

	return &SystemBus{
		memory:  make([]byte, DEFAULT_MEMORY_SIZE),
		mapping: make(map[uint32][]IORegion),
	}
}

func (bus *SystemBus) MapIO(start, end uint32, onRead func(addr uint32) uint32, onWrite func(addr uint32, value uint32)) {
	/*
		MapIO registers a new memory‐mapped I/O region with the system bus.
		The region is specified by its start and end addresses and associated
		read/write callback functions.

		The function calculates the first and last page keys that the region
		spans using a page size of 0x100 and a mask of 0xFFF00, and appends
		the I/O region to the mapping for each page within the range.
	*/

	region := IORegion{
		start:   start,
		end:     end,
		onRead:  onRead,
		onWrite: onWrite,
	}
	// Calculate the first and last page keys that the region spans.
	//Our page size is PAGE_SIZE (masking with PAGE_MASK zeroes the lower 8 bits).
	firstPage := start & PAGE_MASK
	lastPage := end & PAGE_MASK
	for page := firstPage; page <= lastPage; page += PAGE_SIZE {
		bus.mapping[page] = append(bus.mapping[page], region)
	}
}

func (bus *SystemBus) Write32(addr uint32, value uint32) {
	/*
		Write32 performs a thread‐safe 32‐bit write to main memory. Before writing, it checks whether the target address falls within any registered I/O region. If so, the corresponding onWrite callback is invoked and the value is stored in memory. Otherwise, the value is written directly to main memory.

		Parameters:

			addr: The target memory address.

			value: The 32‐bit value to write.
	*/

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	// Check for IO regions
	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+WORD_SIZE], value)
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+WORD_SIZE], value)
}

func (bus *SystemBus) Read32(addr uint32) uint32 {
	/*
		Read32 performs a thread‐safe 32‐bit read from main memory. If the specified address is within a registered I/O region and a valid onRead callback is provided, the callback is invoked to obtain the value, which is then written to memory. If no such region exists, the value is read directly from main memory.

		Parameters:

		    addr: The source memory address.

		Returns:

		    The 32‐bit value read from memory.
	*/

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	if regions, exists := bus.mapping[addr&PAGE_MASK]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+WORD_SIZE], value)
				return value
			}
		}
	}

	return binary.LittleEndian.Uint32(bus.memory[addr : addr+WORD_SIZE])
}

func (bus *SystemBus) Reset() {
	/*
		Reset clears the entire main memory of the system bus.

		This operation is performed in a thread‐safe manner by
		acquiring a write lock, and iterating through the memory
		block to set every byte to zero.
	*/

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	for i := range bus.memory {
		bus.memory[i] = 0
	}
}
