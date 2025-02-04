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
License: GPLv3 or later
*/

package main

import (
	"encoding/binary"
	"sync"
)

type MemoryBus interface {
	Read32(addr uint32) uint32
	Write32(addr uint32, value uint32)
	Reset()
}

type SystemBus struct {
	memory  []byte
	mutex   sync.RWMutex
	mapping map[uint32][]IORegion
}

type IORegion struct {
	start   uint32
	end     uint32
	onRead  func(addr uint32) uint32
	onWrite func(addr uint32, value uint32)
}

func NewSystemBus() *SystemBus {
	return &SystemBus{
		memory:  make([]byte, 16*1024*1024),
		mapping: make(map[uint32][]IORegion),
	}
}

func (bus *SystemBus) MapIO(start, end uint32, onRead func(addr uint32) uint32, onWrite func(addr uint32, value uint32)) {
	page := start & 0xFFF00
	region := IORegion{
		start:   start,
		end:     end,
		onRead:  onRead,
		onWrite: onWrite,
	}
	bus.mapping[page] = append(bus.mapping[page], region)
}

func (bus *SystemBus) Write32(addr uint32, value uint32) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	// Check for IO regions
	if regions, exists := bus.mapping[addr&0xFFF00]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end {
				region.onWrite(addr, value)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return
			}
		}
	}

	// Regular memory write
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
}

func (bus *SystemBus) Read32(addr uint32) uint32 {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	if regions, exists := bus.mapping[addr&0xFFF00]; exists {
		for _, region := range regions {
			if addr >= region.start && addr <= region.end && region.onRead != nil {
				value := region.onRead(addr)
				binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
				return value
			}
		}
	}

	return binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
}

func (bus *SystemBus) Reset() {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	for i := range bus.memory {
		bus.memory[i] = 0
	}
}
