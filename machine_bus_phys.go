// machine_bus_phys.go - IE64 uint64-addressed physical bus path.
//
// PLAN_MAX_RAM.md slice 3: widen IE64 address plumbing without enabling the
// MMU large-address path yet.
//
// Routing rules:
//
//   - Addresses fully contained in the legacy [0, len(bus.memory)) window
//     bridge to the existing 32-bit Read*/Write* accessors. This preserves
//     MMIO dispatch, sign-extension handling, and all other low-memory
//     semantics for IE64 callers that happen to access low addresses.
//   - Addresses fully contained in the bound Backing's [0, Size()) window
//     dispatch directly to the Backing. This is the above-4-GiB IE64 path
//     and never truncates the address through uint32.
//   - Addresses outside both ranges read as zero / write as no-op (no
//     panic). The *WithFault variants return ok=false instead.
//   - Spans that straddle the boundary between legacy memory and Backing
//     are treated as out-of-range to keep semantics unambiguous; IE64
//     access patterns either stay in low memory or in the backing.
//
// MMIO remains in the low 32-bit window. There is intentionally no
// MapIO64Phys; IE64 access to MMIO is a checked bridge through the legacy
// dispatch by virtue of falling into the low-memory routing case.

package main

// Bus64Phys is the uint64-addressed physical-memory bus interface used by
// the IE64 large-address path. Implementations must accept any uint64
// address; out-of-range reads return zero and out-of-range writes are
// silently ignored. The *WithFault variants report unmapped/OOB accesses
// for the IE64 fault path.
type Bus64Phys interface {
	ReadPhys8(addr uint64) byte
	WritePhys8(addr uint64, v byte)
	ReadPhys16(addr uint64) uint16
	WritePhys16(addr uint64, v uint16)
	ReadPhys32(addr uint64) uint32
	WritePhys32(addr uint64, v uint32)
	ReadPhys64(addr uint64) uint64
	WritePhys64(addr uint64, v uint64)

	ReadPhys64WithFault(addr uint64) (uint64, bool)
	WritePhys64WithFault(addr uint64, v uint64) bool
}

// SetBacking binds an addressable Backing to the bus. The backing covers
// the high portion of the guest physical address space that the legacy
// 32 MB memory[] buffer does not. AllocateGuestRAM calls this after a
// successful allocation; tests may call it directly to install a
// SparseBacking.
func (bus *MachineBus) SetBacking(b Backing) {
	bus.backing = b
}

// Backing returns the bound Backing, or nil if none has been set.
func (bus *MachineBus) Backing() Backing {
	return bus.backing
}

// addrInLowMemory reports whether [addr, addr+length) lies entirely within
// the legacy bus.memory[] window. Uses uint64 arithmetic so length+addr
// cannot wrap.
func (bus *MachineBus) addrInLowMemory(addr, length uint64) bool {
	end := addr + length
	if end < addr { // overflow
		return false
	}
	return end <= uint64(len(bus.memory))
}

// addrInBacking reports whether [addr, addr+length) lies entirely within
// the bound Backing's advertised range AND starts at or above the legacy
// memory window. Accesses that straddle the legacy/backing seam are
// treated as unmapped: the legacy slice covers low addresses, the backing
// covers high addresses, and a span that crosses the boundary is not a
// valid IE64 access pattern.
func (bus *MachineBus) addrInBacking(addr, length uint64) bool {
	if bus.backing == nil {
		return false
	}
	if addr < uint64(len(bus.memory)) {
		return false
	}
	end := addr + length
	if end < addr {
		return false
	}
	return end <= bus.backing.Size()
}

// PhysMapped reports whether [addr, addr+length) is fully mapped through
// the legacy low memory window or the bound Backing. PLAN_MAX_RAM.md
// slice 4 callers use this to fault on data accesses that translate to
// physical addresses outside both windows, instead of accepting the
// non-fault Read/Write helpers' silent zero/no-op behavior.
func (bus *MachineBus) PhysMapped(addr, length uint64) bool {
	return bus.addrInLowMemory(addr, length) || bus.addrInBacking(addr, length)
}

func (bus *MachineBus) debugOnPhysRead(addr uint64, width int) {
	if bus == nil || bus.debugAccess == nil || !bus.debugAccess.AnyActive(-1) {
		return
	}
	bus.debugAccess.OnRead(-1, addr, width)
}

func (bus *MachineBus) debugOnPhysWrite(addr uint64, width int, oldVal, newVal uint64) {
	bus.debugOnPhysWriteKnown(addr, width, oldVal, newVal, true)
}

func (bus *MachineBus) debugOnPhysWriteKnown(addr uint64, width int, oldVal, newVal uint64, oldKnown bool) {
	if bus == nil || bus.debugAccess == nil || !bus.debugAccess.AnyActive(-1) {
		return
	}
	bus.debugAccess.OnWriteKnown(-1, addr, width, oldVal, newVal, oldKnown)
}

// ReadPhys8 reads a byte at the given uint64 physical address.
func (bus *MachineBus) ReadPhys8(addr uint64) byte {
	if bus.addrInLowMemory(addr, 1) {
		return bus.Read8(uint32(addr))
	}
	if bus.addrInBacking(addr, 1) {
		value := bus.backing.Read8(addr)
		bus.debugOnPhysRead(addr, 1)
		return value
	}
	return 0
}

// WritePhys8 writes a byte at the given uint64 physical address.
func (bus *MachineBus) WritePhys8(addr uint64, v byte) {
	if bus.addrInLowMemory(addr, 1) {
		bus.Write8(uint32(addr), v)
		return
	}
	if bus.addrInBacking(addr, 1) {
		var old byte
		if bus.debugWriteActive() {
			old = bus.backing.Read8(addr)
		}
		bus.backing.Write8(addr, v)
		bus.debugOnPhysWrite(addr, 1, uint64(old), uint64(v))
	}
}

// ReadPhys16 reads a 16-bit value at the given uint64 physical address.
func (bus *MachineBus) ReadPhys16(addr uint64) uint16 {
	if bus.addrInLowMemory(addr, 2) {
		return bus.Read16(uint32(addr))
	}
	if bus.addrInBacking(addr, 2) {
		lo := uint16(bus.backing.Read8(addr))
		hi := uint16(bus.backing.Read8(addr + 1))
		value := lo | hi<<8
		bus.debugOnPhysRead(addr, 2)
		return value
	}
	return 0
}

// WritePhys16 writes a 16-bit value at the given uint64 physical address.
func (bus *MachineBus) WritePhys16(addr uint64, v uint16) {
	if bus.addrInLowMemory(addr, 2) {
		bus.Write16(uint32(addr), v)
		return
	}
	if bus.addrInBacking(addr, 2) {
		var old uint16
		if bus.debugWriteActive() {
			old = uint16(bus.backing.Read8(addr)) | uint16(bus.backing.Read8(addr+1))<<8
		}
		bus.backing.Write8(addr, byte(v))
		bus.backing.Write8(addr+1, byte(v>>8))
		bus.debugOnPhysWrite(addr, 2, uint64(old), uint64(v))
	}
}

// ReadPhys32 reads a 32-bit value at the given uint64 physical address.
func (bus *MachineBus) ReadPhys32(addr uint64) uint32 {
	if bus.addrInLowMemory(addr, 4) {
		return bus.Read32(uint32(addr))
	}
	if bus.addrInBacking(addr, 4) {
		value := bus.backing.Read32(addr)
		bus.debugOnPhysRead(addr, 4)
		return value
	}
	return 0
}

// WritePhys32 writes a 32-bit value at the given uint64 physical address.
func (bus *MachineBus) WritePhys32(addr uint64, v uint32) {
	if bus.addrInLowMemory(addr, 4) {
		bus.Write32(uint32(addr), v)
		return
	}
	if bus.addrInBacking(addr, 4) {
		var old uint32
		if bus.debugWriteActive() {
			old = bus.backing.Read32(addr)
		}
		bus.backing.Write32(addr, v)
		bus.debugOnPhysWrite(addr, 4, uint64(old), uint64(v))
	}
}

// ReadPhys64 reads a 64-bit value at the given uint64 physical address.
func (bus *MachineBus) ReadPhys64(addr uint64) uint64 {
	if bus.addrInLowMemory(addr, 8) {
		return bus.Read64(uint32(addr))
	}
	if bus.addrInBacking(addr, 8) {
		value := bus.backing.Read64(addr)
		bus.debugOnPhysRead(addr, 8)
		return value
	}
	return 0
}

// WritePhys64 writes a 64-bit value at the given uint64 physical address.
func (bus *MachineBus) WritePhys64(addr uint64, v uint64) {
	if bus.addrInLowMemory(addr, 8) {
		bus.Write64(uint32(addr), v)
		return
	}
	if bus.addrInBacking(addr, 8) {
		var old uint64
		if bus.debugWriteActive() {
			old = bus.backing.Read64(addr)
		}
		bus.backing.Write64(addr, v)
		bus.debugOnPhysWrite(addr, 8, old, v)
	}
}

// ReadPhys64WithFault reads a 64-bit value with fault reporting.
func (bus *MachineBus) ReadPhys64WithFault(addr uint64) (uint64, bool) {
	if bus.addrInLowMemory(addr, 8) {
		return bus.Read64WithFault(uint32(addr))
	}
	if bus.addrInBacking(addr, 8) {
		value := bus.backing.Read64(addr)
		bus.debugOnPhysRead(addr, 8)
		return value, true
	}
	return 0, false
}

// WritePhys64WithFault writes a 64-bit value with fault reporting.
func (bus *MachineBus) WritePhys64WithFault(addr uint64, v uint64) bool {
	if bus.addrInLowMemory(addr, 8) {
		return bus.Write64WithFault(uint32(addr), v)
	}
	if bus.addrInBacking(addr, 8) {
		var old uint64
		if bus.debugWriteActive() {
			old = bus.backing.Read64(addr)
		}
		bus.backing.Write64(addr, v)
		bus.debugOnPhysWrite(addr, 8, old, v)
		return true
	}
	return false
}
