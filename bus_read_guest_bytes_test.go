//go:build headless

package main

import (
	"errors"
	"testing"
)

func TestReadGuestBytesLowMemory(t *testing.T) {
	bus := NewMachineBus()
	copy(bus.GetMemory()[0x1000:], []byte{1, 2, 3})
	got := make([]byte, 3)
	if err := ReadGuestBytes(bus, 0x1000, 0, got); err != nil {
		t.Fatalf("ReadGuestBytes failed: %v", err)
	}
	if string(got) != string([]byte{1, 2, 3}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadGuestBytesRejectsSeamCrossing(t *testing.T) {
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(uint64(len(bus.GetMemory())) + 4096))
	got := make([]byte, 8)
	err := ReadGuestBytes(bus, uint32(len(bus.GetMemory())-4), 0, got)
	if !errors.Is(err, ErrSeamCrossing) {
		t.Fatalf("err = %v, want ErrSeamCrossing", err)
	}
}

func TestReadGuestBytesHighRAMOnly(t *testing.T) {
	bus := NewMachineBus()
	addr := uint64(len(bus.GetMemory())) + 64
	backing := NewSparseBacking(addr + 4096)
	backing.WriteBytes(addr, []byte{4, 5, 6})
	bus.SetBacking(backing)
	got := make([]byte, 3)
	if err := ReadGuestBytes(bus, uint32(addr), uint32(addr>>32), got); err != nil {
		t.Fatalf("ReadGuestBytes high RAM failed: %v", err)
	}
	if string(got) != string([]byte{4, 5, 6}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadGuestBytesRejectsOutOfRange(t *testing.T) {
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(uint64(len(bus.GetMemory())) + 4096))
	got := make([]byte, 8)
	err := ReadGuestBytes(bus, uint32(len(bus.GetMemory())+8192), 0, got)
	if !errors.Is(err, ErrAddrOutOfRange) {
		t.Fatalf("err = %v, want ErrAddrOutOfRange", err)
	}
}

func TestReadGuestBytesLowMemoryRespectsActiveVisibleRAM(t *testing.T) {
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(uint64(len(bus.GetMemory())) + 4096))
	bus.SetSizing(MemorySizing{ActiveVisibleRAM: 0x2000})
	copy(bus.GetMemory()[0x2100:], []byte{7, 8, 9})

	err := ReadGuestBytes(bus, 0x2100, 0, make([]byte, 3))
	if !errors.Is(err, ErrAddrOutOfRange) {
		t.Fatalf("err = %v, want ErrAddrOutOfRange", err)
	}
}

func TestReadGuestBytesIE32BusRejectsHIPtr(t *testing.T) {
	bus := &bus32Only{mem: make([]byte, 16)}
	err := ReadGuestBytes(bus, 0, 1, make([]byte, 1))
	if !errors.Is(err, ErrHIPtrUnsupported) {
		t.Fatalf("err = %v, want ErrHIPtrUnsupported", err)
	}
}

type bus32Only struct{ mem []byte }

func (b *bus32Only) Read8(addr uint32) uint8           { return b.mem[addr] }
func (b *bus32Only) Write8(addr uint32, value uint8)   { b.mem[addr] = value }
func (b *bus32Only) Read16(addr uint32) uint16         { return 0 }
func (b *bus32Only) Write16(addr uint32, value uint16) {}
func (b *bus32Only) Read32(addr uint32) uint32         { return 0 }
func (b *bus32Only) Write32(addr uint32, value uint32) {}
func (b *bus32Only) Reset()                            {}
func (b *bus32Only) GetMemory() []byte                 { return b.mem }
