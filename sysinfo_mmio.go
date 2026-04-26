// sysinfo_mmio.go - Low-MMIO RAM-size discovery block.
//
// Exposes total guest RAM and active CPU/profile visible RAM as two
// little-endian 64-bit values built from four 32-bit MMIO registers in the
// SYSINFO_REGION block. The values are stable for the lifetime of the
// emulator process; writes are silently ignored.
//
// PLAN_MAX_RAM.md slice 2.

package main

// RegisterSysInfoMMIO registers read-only handlers for the SYSINFO RAM-size
// register pairs. total and active are reported in bytes; high words round-
// trip values above 4 GiB without truncation.
//
// Writes are accepted (so guest stores do not fault under strict MMIO
// policy) but ignored.
func RegisterSysInfoMMIO(bus *MachineBus, total, active uint64) {
	totalLo := uint32(total & 0xFFFFFFFF)
	totalHi := uint32(total >> 32)
	activeLo := uint32(active & 0xFFFFFFFF)
	activeHi := uint32(active >> 32)

	read := func(addr uint32) uint32 {
		switch addr {
		case SYSINFO_TOTAL_RAM_LO:
			return totalLo
		case SYSINFO_TOTAL_RAM_HI:
			return totalHi
		case SYSINFO_ACTIVE_RAM_LO:
			return activeLo
		case SYSINFO_ACTIVE_RAM_HI:
			return activeHi
		default:
			return 0
		}
	}
	write := func(addr uint32, value uint32) {
		// Read-only: ignore.
	}

	bus.MapIO(SYSINFO_REGION_BASE, SYSINFO_REGION_END, read, write)
}

// RegisterSysInfoMMIOFromBus registers the SYSINFO RAM-size handlers using
// the bus's published guest RAM sizing as the single source of truth. The
// values are snapshotted at registration time -- callers must call this
// after the sizing has settled (post-AllocateGuestRAM retry).
func RegisterSysInfoMMIOFromBus(bus *MachineBus) {
	RegisterSysInfoMMIO(bus, bus.TotalGuestRAM(), bus.ActiveVisibleRAM())
}
