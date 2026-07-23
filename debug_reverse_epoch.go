// debug_reverse_epoch.go - epoch-driven capture for whole-machine reverse history.
//
// The legacy reverse-history capture scans all of guest RAM on every step to
// build a sparse page set, then diffs that against the previous snapshot to
// form a delta. On a machine with gigabytes of RAM that full scan dominates the
// step, which is exactly the cost this item removes.
//
// With IE_MON_EPOCH_HISTORY=1 the monitor keeps a page-dirty cursor over the
// bus (see machine_bus_page_dirty.go). Between snapshots the bus records which
// pages the guest wrote; a delta then copies only those pages instead of
// scanning and diffing the whole address space. Full checkpoints still take the
// complete scan, because a checkpoint must stand alone, and draining the cursor
// at each checkpoint rebaselines the delta stream to that point.
//
// The reconstruction is identical to the legacy delta's. The cursor reports a
// page as dirty whenever it was written, even if the write restored its old
// contents, so the epoch delta is a superset of the strict byte diff: it may
// carry a few unchanged pages, but applying it over the same base yields the
// same bytes. A page written back to all zeroes is emitted as a zero page,
// which reconstruction reads as a delete, matching the legacy diff.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"fmt"
	"os"
	"sort"
)

// monitorEpochHistoryRequested reports whether epoch-driven reverse history is
// switched on. It is opt-in until the parity gate is met.
func monitorEpochHistoryRequested() bool {
	return os.Getenv("IE_MON_EPOCH_HISTORY") == "1"
}

// enableEpochHistoryLocked binds a page-dirty cursor to the bus so deltas can be
// built from written pages rather than a full scan. It is idempotent.
func (m *MachineMonitor) enableEpochHistoryLocked() {
	if m == nil || m.bus == nil {
		return
	}
	if !m.bus.PageDirtyTrackingActive() {
		m.bus.EnablePageDirtyTracking(m.bus.backingVisibleSize())
	}
	if m.busEpochCursor == nil || !m.busEpochCursor.Active() {
		m.busEpochCursor = m.bus.NewPageDirtyCursor()
	}
}

// wholeCheckpointDue reports whether the next capture must be a standalone full
// checkpoint rather than a delta. It is the same interval-and-bytes rule the
// legacy path applies, lifted out so the epoch path can defer to the legacy
// full scan when a checkpoint is owed.
func (m *MachineMonitor) wholeCheckpointDue() bool {
	interval := m.wholeCheckpointInterval
	if interval <= 0 {
		interval = 32
	}
	bytesLimit := m.wholeCheckpointBytes
	if bytesLimit == 0 {
		bytesLimit = 64 << 20
	}
	return !(m.wholeDeltaCount < interval && m.wholeDeltaBytes < bytesLimit)
}

// takeWholeMachineNonBusLocked captures every registered CPU and device, leaving
// the bus fields empty for the caller to fill from the page-dirty cursor. It is
// the CPU/device half of takeWholeMachineSnapshotLocked, which stays the source
// of truth for the field layout.
func (m *MachineMonitor) takeWholeMachineNonBusLocked() (*WholeMachineSnapshot, error) {
	snap := &WholeMachineSnapshot{Version: snapshotVersion, Full: true}
	ids := make([]int, 0, len(m.cpus))
	for id := range m.cpus {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		entry := m.cpus[id]
		if entry == nil || entry.CPU == nil {
			continue
		}
		memSize := memSizeFromWidth(entry.CPU.AddressWidth())
		if memSize > snapshotMaxMemory {
			return nil, fmt.Errorf("CPU %d memory size %d exceeds snapshot cap %d", id, memSize, snapshotMaxMemory)
		}
		mem := entry.CPU.ReadMemory(0, memSize)
		snap.CPUs = append(snap.CPUs, WholeMachineCPUState{
			ID:           id,
			Label:        entry.Label,
			CPUType:      entry.CPU.CPUName(),
			AddressWidth: entry.CPU.AddressWidth(),
			Registers:    entry.CPU.GetRegisters(),
			MemorySize:   uint64(len(mem)),
			Pages:        sparsePagesFromBytes(0, mem),
		})
	}
	names := make([]string, 0, len(m.devices))
	for name := range m.devices {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dev := m.devices[name]
		if dev == nil {
			continue
		}
		version, data, err := dev.DebugSnapshot()
		if err != nil {
			return nil, fmt.Errorf("snapshot device %s: %w", name, err)
		}
		snap.Devices = append(snap.Devices, DeviceStateBlob{
			Name:    name,
			Version: version,
			Data:    append([]byte(nil), data...),
		})
	}
	return snap, nil
}

// busDeltaPagesFromCursorLocked copies the pages the cursor reports dirty out of
// bus memory and backing, splitting on the visible-RAM boundary. A page that is
// all zeroes is emitted as a zero page so reconstruction deletes it, exactly as
// the legacy byte diff would.
func (m *MachineMonitor) busDeltaPagesFromCursorLocked(pages []uint64) (busPages, backingPages []SnapshotPage) {
	visible := uint64(len(m.bus.memory))
	var backingSize uint64
	if m.bus.backing != nil {
		backingSize = m.bus.backing.Size()
	}
	buf := make([]byte, PageDirtySize)
	for _, page := range pages {
		addr := PageDirtyAddress(page)
		if addr < visible {
			size := PageDirtySize
			if addr+size > visible {
				size = visible - addr
			}
			data := m.bus.memory[addr : addr+size]
			if snapshotPageAllZero(data) {
				busPages = append(busPages, zeroSnapshotPage(addr, visible))
				continue
			}
			busPages = append(busPages, SnapshotPage{Addr: addr, Data: append([]byte(nil), data...)})
			continue
		}
		if m.bus.backing == nil || addr >= backingSize {
			continue
		}
		size := PageDirtySize
		if addr+size > backingSize {
			size = backingSize - addr
		}
		slice := buf[:size]
		m.bus.backing.ReadBytes(addr, slice)
		if snapshotPageAllZero(slice) {
			backingPages = append(backingPages, zeroSnapshotPage(addr, backingSize))
			continue
		}
		backingPages = append(backingPages, SnapshotPage{Addr: addr, Data: append([]byte(nil), slice...)})
	}
	return busPages, backingPages
}

// cpuDeltaPagesLocked diffs each current CPU's pages against the materialised
// base, the same per-CPU diff the legacy delta performs. CPU-local memory is not
// on the bus, so the cursor cannot cover it and it is diffed the old way; it is
// small next to guest RAM.
func cpuDeltaPagesLocked(cur, base *WholeMachineSnapshot) []WholeMachineCPUState {
	baseByID := make(map[int]WholeMachineCPUState, len(base.CPUs))
	for _, cpu := range base.CPUs {
		baseByID[cpu.ID] = cpu
	}
	out := make([]WholeMachineCPUState, 0, len(cur.CPUs))
	for _, cpu := range cur.CPUs {
		delta := cpu
		delta.Registers = cloneRegisterInfos(cpu.Registers)
		if prev, ok := baseByID[cpu.ID]; ok {
			delta.Pages = diffSnapshotPages(cpu.Pages, prev.Pages, cpu.MemorySize)
		} else {
			delta.Pages = cloneSnapshotPages(cpu.Pages)
		}
		out = append(out, delta)
	}
	return out
}

// recordWholeMachineHistoryEpochLocked records one delta built from the
// page-dirty cursor. It returns ok=false when the capture must be a full
// checkpoint instead, so the caller falls back to the legacy full scan, which
// then rebaselines the cursor.
func (m *MachineMonitor) recordWholeMachineHistoryEpochLocked() (uint64, bool) {
	if len(m.wholeHistory) == 0 || m.wholeCheckpointDue() || m.epochForceFull {
		// A forced full capture falls through to the legacy full scan, which
		// then rebaselines the cursor and clears the flag.
		return 0, false
	}
	base, err := m.materializeWholeMachineSnapshotLocked(m.wholeHistory[len(m.wholeHistory)-1])
	if err != nil {
		return 0, false
	}
	cur, err := m.takeWholeMachineNonBusLocked()
	if err != nil {
		return 0, false
	}
	cur.Bus.MemorySize = uint64(len(m.bus.memory))
	if m.bus.backing != nil {
		cur.Bus.BackingSize = m.bus.backing.Size()
	}

	dirty := m.busEpochCursor.CollectPages()
	if len(dirty) == 0 && nonBusStateEqual(cur, base) {
		return base.ID, true
	}

	busPages, backingPages := m.busDeltaPagesFromCursorLocked(dirty)
	delta := &WholeMachineSnapshot{
		Version: snapshotVersion,
		Full:    false,
		BaseID:  base.ID,
		CPUs:    cpuDeltaPagesLocked(cur, base),
		Devices: cloneDeviceBlobs(cur.Devices),
	}
	delta.Bus.MemorySize = cur.Bus.MemorySize
	delta.Bus.BackingSize = cur.Bus.BackingSize
	delta.Bus.Pages = busPages
	delta.Bus.BackingPages = backingPages

	m.nextWholeID++
	delta.ID = m.nextWholeID
	delta.DeltaBytes = snapshotDeltaBytes(delta)
	m.wholeDeltaCount++
	m.wholeDeltaBytes += delta.DeltaBytes
	m.wholeHistory = append(m.wholeHistory, delta)
	m.pruneWholeHistoryLocked()
	return delta.ID, true
}

// nonBusStateEqual reports whether two snapshots agree on every CPU and device,
// ignoring the bus. It backs the epoch dedup: when the cursor reports no dirty
// pages and this holds, the machine state has not moved.
func nonBusStateEqual(a, b *WholeMachineSnapshot) bool {
	if len(a.CPUs) != len(b.CPUs) || !deviceBlobsEqual(a.Devices, b.Devices) {
		return false
	}
	for i := range a.CPUs {
		ac, bc := a.CPUs[i], b.CPUs[i]
		if ac.ID != bc.ID || ac.Label != bc.Label || ac.CPUType != bc.CPUType ||
			ac.AddressWidth != bc.AddressWidth || ac.MemorySize != bc.MemorySize ||
			!registerInfosEqual(ac.Registers, bc.Registers) ||
			!snapshotPagesEqual(ac.Pages, bc.Pages) {
			return false
		}
	}
	return true
}
