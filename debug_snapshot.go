// debug_snapshot.go - Machine state snapshot for save/load and backstep

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
)

const (
	snapshotMagic        = "IEMS"
	snapshotVersion      = 1
	snapshotMaxMemory    = 512 << 20
	snapshotMaxRegisters = 1024
)

// MachineSnapshot captures CPU registers and memory for save/load and backstep.
type MachineSnapshot struct {
	CPUType   string
	Registers []RegisterInfo
	Memory    []byte // full memory (compressed on disk, raw in memory for backstep)
}

// SnapshotPage stores one non-zero memory page in a sparse snapshot.
type SnapshotPage struct {
	Addr uint64
	Data []byte
}

// WholeMachineCPUState captures one registered monitor CPU instance. IDs and
// labels come from MachineMonitor, so multi-CPU profiles restore by identity.
type WholeMachineCPUState struct {
	ID           int
	Label        string
	CPUType      string
	AddressWidth int
	Registers    []RegisterInfo
	MemorySize   uint64
	Pages        []SnapshotPage
}

// WholeMachineBusState captures shared bus memory and optional IE64 backing.
type WholeMachineBusState struct {
	MemorySize   uint64
	Pages        []SnapshotPage
	BackingSize  uint64
	BackingPages []SnapshotPage
}

// DeviceStateBlob is the versioned extension point for serialisable devices.
type DeviceStateBlob struct {
	Name    string
	Version uint32
	Data    []byte
}

// DebugSnapshotDevice is implemented by mutable devices that participate in
// whole-machine reverse snapshots. Host resources such as windows, file
// descriptors, callbacks, and goroutines stay outside the blob; only
// guest-visible deterministic state belongs here.
type DebugSnapshotDevice interface {
	DebugSnapshotName() string
	DebugSnapshot() (version uint32, data []byte, err error)
	DebugRestoreSnapshot(version uint32, data []byte) error
}

// WholeMachineSnapshot captures all registered CPUs plus shared bus memory.
// Device blobs are intentionally versioned so chips can join the contract
// incrementally without changing the snapshot envelope.
type WholeMachineSnapshot struct {
	Version    uint32
	ID         uint64
	BaseID     uint64
	Full       bool
	DeltaBytes uint64
	CPUs       []WholeMachineCPUState
	Bus        WholeMachineBusState
	Devices    []DeviceStateBlob
}

// memSizeFromWidth returns the memory size in bytes for a given address width.
func memSizeFromWidth(width int) int {
	switch {
	case width <= 16:
		return 64 * 1024 // 64KB
	default:
		return 32 * 1024 * 1024 // 32MB
	}
}

func sparsePagesFromBytes(base uint64, data []byte) []SnapshotPage {
	if len(data) == 0 {
		return nil
	}
	const pageSize = MMU_PAGE_SIZE
	pages := make([]SnapshotPage, 0)
	for off := 0; off < len(data); off += pageSize {
		end := off + pageSize
		if end > len(data) {
			end = len(data)
		}
		page := data[off:end]
		nonZero := false
		for _, b := range page {
			if b != 0 {
				nonZero = true
				break
			}
		}
		if !nonZero {
			continue
		}
		cp := append([]byte(nil), page...)
		pages = append(pages, SnapshotPage{Addr: base + uint64(off), Data: cp})
	}
	return pages
}

func writeSparsePages(dst func(addr uint64, data []byte) error, pages []SnapshotPage) error {
	for _, page := range pages {
		if len(page.Data) == 0 {
			continue
		}
		if err := dst(page.Addr, page.Data); err != nil {
			return err
		}
	}
	return nil
}

func cloneSnapshotPages(pages []SnapshotPage) []SnapshotPage {
	if len(pages) == 0 {
		return nil
	}
	out := make([]SnapshotPage, 0, len(pages))
	for _, page := range pages {
		out = append(out, SnapshotPage{Addr: page.Addr, Data: append([]byte(nil), page.Data...)})
	}
	return out
}

func cloneRegisterInfos(regs []RegisterInfo) []RegisterInfo {
	return append([]RegisterInfo(nil), regs...)
}

func cloneDeviceBlobs(blobs []DeviceStateBlob) []DeviceStateBlob {
	if len(blobs) == 0 {
		return nil
	}
	out := make([]DeviceStateBlob, 0, len(blobs))
	for _, blob := range blobs {
		out = append(out, DeviceStateBlob{Name: blob.Name, Version: blob.Version, Data: append([]byte(nil), blob.Data...)})
	}
	return out
}

func cloneWholeMachineSnapshot(snap *WholeMachineSnapshot) *WholeMachineSnapshot {
	if snap == nil {
		return nil
	}
	out := *snap
	out.CPUs = make([]WholeMachineCPUState, 0, len(snap.CPUs))
	for _, cpu := range snap.CPUs {
		cp := cpu
		cp.Registers = cloneRegisterInfos(cpu.Registers)
		cp.Pages = cloneSnapshotPages(cpu.Pages)
		out.CPUs = append(out.CPUs, cp)
	}
	out.Bus.Pages = cloneSnapshotPages(snap.Bus.Pages)
	out.Bus.BackingPages = cloneSnapshotPages(snap.Bus.BackingPages)
	out.Devices = cloneDeviceBlobs(snap.Devices)
	return &out
}

func registerInfosEqual(a, b []RegisterInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func snapshotPagesEqual(a, b []SnapshotPage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Addr != b[i].Addr || !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

func deviceBlobsEqual(a, b []DeviceStateBlob) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Version != b[i].Version || !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

func wholeSnapshotsEquivalent(a, b *WholeMachineSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.CPUs) != len(b.CPUs) ||
		a.Bus.MemorySize != b.Bus.MemorySize ||
		a.Bus.BackingSize != b.Bus.BackingSize ||
		!snapshotPagesEqual(a.Bus.Pages, b.Bus.Pages) ||
		!snapshotPagesEqual(a.Bus.BackingPages, b.Bus.BackingPages) ||
		!deviceBlobsEqual(a.Devices, b.Devices) {
		return false
	}
	for i := range a.CPUs {
		ac, bc := a.CPUs[i], b.CPUs[i]
		if ac.ID != bc.ID ||
			ac.Label != bc.Label ||
			ac.CPUType != bc.CPUType ||
			ac.AddressWidth != bc.AddressWidth ||
			ac.MemorySize != bc.MemorySize ||
			!registerInfosEqual(ac.Registers, bc.Registers) ||
			!snapshotPagesEqual(ac.Pages, bc.Pages) {
			return false
		}
	}
	return true
}

func pageBytes(pages []SnapshotPage) uint64 {
	var total uint64
	for _, page := range pages {
		total += uint64(len(page.Data))
	}
	return total
}

func snapshotDeltaBytes(snap *WholeMachineSnapshot) uint64 {
	if snap == nil {
		return 0
	}
	total := pageBytes(snap.Bus.Pages) + pageBytes(snap.Bus.BackingPages)
	for _, cpu := range snap.CPUs {
		total += pageBytes(cpu.Pages)
	}
	for _, blob := range snap.Devices {
		total += uint64(len(blob.Data))
	}
	return total
}

func snapshotPageMap(pages []SnapshotPage) map[uint64][]byte {
	out := make(map[uint64][]byte, len(pages))
	for _, page := range pages {
		out[page.Addr] = page.Data
	}
	return out
}

func zeroSnapshotPage(addr, memorySize uint64) SnapshotPage {
	size := uint64(MMU_PAGE_SIZE)
	if addr+size > memorySize {
		size = memorySize - addr
	}
	return SnapshotPage{Addr: addr, Data: make([]byte, size)}
}

func snapshotPageAllZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func diffSnapshotPages(cur, prev []SnapshotPage, memorySize uint64) []SnapshotPage {
	curMap := snapshotPageMap(cur)
	prevMap := snapshotPageMap(prev)
	addrs := make([]uint64, 0, len(curMap)+len(prevMap))
	seen := make(map[uint64]bool, len(curMap)+len(prevMap))
	for addr := range curMap {
		seen[addr] = true
		addrs = append(addrs, addr)
	}
	for addr := range prevMap {
		if !seen[addr] {
			addrs = append(addrs, addr)
		}
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	out := make([]SnapshotPage, 0)
	for _, addr := range addrs {
		curPage, curOK := curMap[addr]
		prevPage, prevOK := prevMap[addr]
		switch {
		case curOK && (!prevOK || !bytes.Equal(curPage, prevPage)):
			out = append(out, SnapshotPage{Addr: addr, Data: append([]byte(nil), curPage...)})
		case !curOK && prevOK && addr < memorySize:
			out = append(out, zeroSnapshotPage(addr, memorySize))
		}
	}
	return out
}

func applySnapshotPageDelta(base, delta []SnapshotPage) []SnapshotPage {
	pages := snapshotPageMap(cloneSnapshotPages(base))
	for _, page := range delta {
		if snapshotPageAllZero(page.Data) {
			delete(pages, page.Addr)
			continue
		}
		pages[page.Addr] = append([]byte(nil), page.Data...)
	}
	addrs := make([]uint64, 0, len(pages))
	for addr := range pages {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	out := make([]SnapshotPage, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, SnapshotPage{Addr: addr, Data: pages[addr]})
	}
	return out
}

func captureSparseBackingPages(backing Backing) ([]SnapshotPage, error) {
	if backing == nil {
		return nil, nil
	}
	if sparse, ok := backing.(*SparseBacking); ok {
		sparse.mu.RLock()
		defer sparse.mu.RUnlock()
		sparse.assertOpenLocked()
		pages := make([]SnapshotPage, 0, len(sparse.pages))
		pageIdxs := make([]uint64, 0, len(sparse.pages))
		for idx := range sparse.pages {
			pageIdxs = append(pageIdxs, idx)
		}
		sort.Slice(pageIdxs, func(i, j int) bool { return pageIdxs[i] < pageIdxs[j] })
		for _, idx := range pageIdxs {
			page := sparse.pages[idx]
			if len(page) == 0 {
				continue
			}
			cp := append([]byte(nil), page...)
			pages = append(pages, SnapshotPage{Addr: idx * sparse.pageSize, Data: cp})
		}
		return pages, nil
	}
	if backing.Size() > snapshotMaxMemory {
		return nil, fmt.Errorf("backing size %d exceeds non-sparse snapshot cap %d", backing.Size(), snapshotMaxMemory)
	}
	buf := make([]byte, backing.Size())
	backing.ReadBytes(0, buf)
	return sparsePagesFromBytes(0, buf), nil
}

// TakeWholeMachineSnapshot captures every CPU registered with the monitor plus
// shared bus memory. It uses sparse page storage so large mostly-empty RAM does
// not become a full raw copy.
func TakeWholeMachineSnapshot(m *MachineMonitor) (*WholeMachineSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("nil monitor")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.takeWholeMachineSnapshotLocked()
}

func (m *MachineMonitor) takeWholeMachineSnapshotLocked() (*WholeMachineSnapshot, error) {
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
		state := WholeMachineCPUState{
			ID:           id,
			Label:        entry.Label,
			CPUType:      entry.CPU.CPUName(),
			AddressWidth: entry.CPU.AddressWidth(),
			Registers:    entry.CPU.GetRegisters(),
			MemorySize:   uint64(len(mem)),
			Pages:        sparsePagesFromBytes(0, mem),
		}
		snap.CPUs = append(snap.CPUs, state)
	}
	if m.bus != nil {
		snap.Bus.MemorySize = uint64(len(m.bus.memory))
		snap.Bus.Pages = sparsePagesFromBytes(0, m.bus.memory)
		if m.bus.backing != nil {
			snap.Bus.BackingSize = m.bus.backing.Size()
			pages, err := captureSparseBackingPages(m.bus.backing)
			if err != nil {
				return nil, err
			}
			snap.Bus.BackingPages = pages
		}
	}
	deviceNames := make([]string, 0, len(m.devices))
	for name := range m.devices {
		deviceNames = append(deviceNames, name)
	}
	sort.Strings(deviceNames)
	for _, name := range deviceNames {
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

func (m *MachineMonitor) materializeWholeMachineSnapshotLocked(snap *WholeMachineSnapshot) (*WholeMachineSnapshot, error) {
	if snap == nil {
		return nil, fmt.Errorf("nil whole-machine snapshot")
	}
	if snap.Full || snap.BaseID == 0 {
		out := cloneWholeMachineSnapshot(snap)
		out.Full = true
		out.BaseID = 0
		return out, nil
	}
	base := m.findWholeSnapshotLocked(snap.BaseID)
	if base == nil {
		return nil, fmt.Errorf("base snapshot %d for snapshot %d is not retained", snap.BaseID, snap.ID)
	}
	material, err := m.materializeWholeMachineSnapshotLocked(base)
	if err != nil {
		return nil, err
	}
	material.ID = snap.ID
	material.BaseID = 0
	material.Full = true
	material.DeltaBytes = snapshotDeltaBytes(material)
	material.Bus.Pages = applySnapshotPageDelta(material.Bus.Pages, snap.Bus.Pages)
	material.Bus.MemorySize = snap.Bus.MemorySize
	material.Bus.BackingPages = applySnapshotPageDelta(material.Bus.BackingPages, snap.Bus.BackingPages)
	material.Bus.BackingSize = snap.Bus.BackingSize
	material.Devices = cloneDeviceBlobs(snap.Devices)

	baseByID := make(map[int]WholeMachineCPUState, len(material.CPUs))
	for _, cpu := range material.CPUs {
		baseByID[cpu.ID] = cpu
	}
	material.CPUs = material.CPUs[:0]
	for _, deltaCPU := range snap.CPUs {
		cpu := deltaCPU
		if baseCPU, ok := baseByID[deltaCPU.ID]; ok {
			cpu.Pages = applySnapshotPageDelta(baseCPU.Pages, deltaCPU.Pages)
		} else {
			cpu.Pages = cloneSnapshotPages(deltaCPU.Pages)
		}
		cpu.Registers = cloneRegisterInfos(deltaCPU.Registers)
		material.CPUs = append(material.CPUs, cpu)
	}
	return material, nil
}

func (m *MachineMonitor) findWholeSnapshotLocked(id uint64) *WholeMachineSnapshot {
	for i := len(m.wholeHistory) - 1; i >= 0; i-- {
		if m.wholeHistory[i] != nil && m.wholeHistory[i].ID == id {
			return m.wholeHistory[i]
		}
	}
	return nil
}

func makeWholeMachineDelta(cur, base *WholeMachineSnapshot) *WholeMachineSnapshot {
	if cur == nil || base == nil {
		return cur
	}
	delta := cloneWholeMachineSnapshot(cur)
	delta.Full = false
	delta.BaseID = base.ID
	delta.Bus.Pages = diffSnapshotPages(cur.Bus.Pages, base.Bus.Pages, cur.Bus.MemorySize)
	delta.Bus.BackingPages = diffSnapshotPages(cur.Bus.BackingPages, base.Bus.BackingPages, cur.Bus.BackingSize)
	baseCPU := make(map[int]WholeMachineCPUState, len(base.CPUs))
	for _, cpu := range base.CPUs {
		baseCPU[cpu.ID] = cpu
	}
	for i := range delta.CPUs {
		if prev, ok := baseCPU[delta.CPUs[i].ID]; ok {
			delta.CPUs[i].Pages = diffSnapshotPages(cur.CPUs[i].Pages, prev.Pages, cur.CPUs[i].MemorySize)
		}
	}
	delta.DeltaBytes = snapshotDeltaBytes(delta)
	return delta
}

// RestoreWholeMachineSnapshot restores CPU state by monitor id and shared bus
// memory from a whole-machine snapshot. Missing CPUs are reported rather than
// silently ignored.
func RestoreWholeMachineSnapshot(m *MachineMonitor, snap *WholeMachineSnapshot) error {
	if m == nil {
		return fmt.Errorf("nil monitor")
	}
	if snap == nil {
		return fmt.Errorf("nil whole-machine snapshot")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restoreWholeMachineSnapshotLocked(snap)
}

func (m *MachineMonitor) restoreWholeMachineSnapshotLocked(snap *WholeMachineSnapshot) error {
	if m.bus != nil {
		if snap.Bus.MemorySize > uint64(len(m.bus.memory)) {
			return fmt.Errorf("snapshot bus memory size %d exceeds current bus size %d", snap.Bus.MemorySize, len(m.bus.memory))
		}
		clear(m.bus.memory)
		for _, page := range snap.Bus.Pages {
			end := page.Addr + uint64(len(page.Data))
			if end > uint64(len(m.bus.memory)) {
				return fmt.Errorf("bus snapshot page $%X exceeds current bus memory", page.Addr)
			}
			copy(m.bus.memory[page.Addr:end], page.Data)
		}
		if snap.Bus.BackingSize > 0 {
			if m.bus.backing == nil {
				m.bus.backing = NewSparseBacking(snap.Bus.BackingSize)
			}
			if m.bus.backing.Size() < snap.Bus.BackingSize {
				return fmt.Errorf("snapshot backing size %d exceeds current backing size %d", snap.Bus.BackingSize, m.bus.backing.Size())
			}
			m.bus.backing.Reset()
			for _, page := range snap.Bus.BackingPages {
				m.bus.backing.WriteBytes(page.Addr, page.Data)
			}
		} else if m.bus.backing != nil {
			_ = m.bus.backing.Close()
			m.bus.backing = nil
		}
	}

	for _, cpuSnap := range snap.CPUs {
		entry := m.cpus[cpuSnap.ID]
		if entry == nil || entry.CPU == nil {
			return fmt.Errorf("snapshot CPU id %d (%s) is not registered", cpuSnap.ID, cpuSnap.Label)
		}
		if entry.CPU.CPUName() != cpuSnap.CPUType {
			return fmt.Errorf("snapshot CPU id %d type %s does not match current %s", cpuSnap.ID, cpuSnap.CPUType, entry.CPU.CPUName())
		}
		for _, r := range cpuSnap.Registers {
			entry.CPU.SetRegister(r.Name, r.Value)
		}
		if cpuSnap.MemorySize > 0 {
			if cpuSnap.MemorySize > snapshotMaxMemory {
				return fmt.Errorf("snapshot CPU id %d memory size %d exceeds cap", cpuSnap.ID, cpuSnap.MemorySize)
			}
			entry.CPU.WriteMemory(0, make([]byte, int(cpuSnap.MemorySize)))
			err := writeSparsePages(func(addr uint64, data []byte) error {
				if addr+uint64(len(data)) > cpuSnap.MemorySize {
					return fmt.Errorf("CPU %d snapshot page $%X exceeds captured memory", cpuSnap.ID, addr)
				}
				entry.CPU.WriteMemory(addr, data)
				return nil
			}, cpuSnap.Pages)
			if err != nil {
				return err
			}
		}
	}

	for _, blob := range snap.Devices {
		dev := m.devices[blob.Name]
		if dev == nil {
			return fmt.Errorf("snapshot device %s is not registered", blob.Name)
		}
		if err := dev.DebugRestoreSnapshot(blob.Version, append([]byte(nil), blob.Data...)); err != nil {
			return fmt.Errorf("restore device %s: %w", blob.Name, err)
		}
	}
	return nil
}

// TakeSnapshot captures the current CPU registers and full memory.
func TakeSnapshot(cpu DebuggableCPU) *MachineSnapshot {
	regs := cpu.GetRegisters()
	memSize := memSizeFromWidth(cpu.AddressWidth())
	mem := cpu.ReadMemory(0, memSize)
	if mem == nil {
		mem = make([]byte, 0)
	}
	return &MachineSnapshot{
		CPUType:   cpu.CPUName(),
		Registers: regs,
		Memory:    mem,
	}
}

// RestoreSnapshot restores CPU registers and memory from a CPU-local snapshot.
func RestoreSnapshot(cpu DebuggableCPU, snap *MachineSnapshot) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	if snap.CPUType != "" && snap.CPUType != cpu.CPUName() {
		return fmt.Errorf("snapshot CPU type %s does not match focussed CPU %s", snap.CPUType, cpu.CPUName())
	}
	for _, r := range snap.Registers {
		cpu.SetRegister(r.Name, r.Value)
	}
	if len(snap.Memory) > 0 {
		cpu.WriteMemory(0, snap.Memory)
	}
	return nil
}

// SaveSnapshotToFile writes a snapshot to disk with gzip compression.
func SaveSnapshotToFile(snap *MachineSnapshot, path string) error {
	var buf bytes.Buffer

	// Magic
	buf.WriteString(snapshotMagic)

	// Version
	binary.Write(&buf, binary.LittleEndian, uint32(snapshotVersion))

	// CPU type
	cpuBytes := []byte(snap.CPUType)
	buf.WriteByte(byte(len(cpuBytes)))
	buf.Write(cpuBytes)

	// Register count
	binary.Write(&buf, binary.LittleEndian, uint32(len(snap.Registers)))

	// Registers
	for _, r := range snap.Registers {
		nameBytes := []byte(r.Name)
		buf.WriteByte(byte(len(nameBytes)))
		buf.Write(nameBytes)
		binary.Write(&buf, binary.LittleEndian, r.Value)
		binary.Write(&buf, binary.LittleEndian, uint32(r.BitWidth))
	}

	// Memory: uncompressed length, then gzip-compressed data
	binary.Write(&buf, binary.LittleEndian, uint32(len(snap.Memory)))

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(snap.Memory); err != nil {
		return fmt.Errorf("compressing memory: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("closing gzip: %w", err)
	}
	buf.Write(compressed.Bytes())

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// LoadSnapshotFromFile reads and decompresses a snapshot from disk.
func LoadSnapshotFromFile(path string) (*MachineSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	r := bytes.NewReader(data)

	// Magic
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("reading magic: %w", err)
	}
	if string(magic) != snapshotMagic {
		return nil, fmt.Errorf("invalid snapshot magic: %q", string(magic))
	}

	// Version
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}
	if version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version: %d", version)
	}

	// CPU type
	cpuTypeLen, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading CPU type length: %w", err)
	}
	cpuType := make([]byte, cpuTypeLen)
	if _, err := io.ReadFull(r, cpuType); err != nil {
		return nil, fmt.Errorf("reading CPU type: %w", err)
	}

	// Registers
	var regCount uint32
	if err := binary.Read(r, binary.LittleEndian, &regCount); err != nil {
		return nil, fmt.Errorf("reading register count: %w", err)
	}
	if regCount > snapshotMaxRegisters {
		return nil, fmt.Errorf("snapshot register count %d exceeds maximum %d", regCount, snapshotMaxRegisters)
	}

	regs := make([]RegisterInfo, regCount)
	for i := range regCount {
		nameLen, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading register name length: %w", err)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(r, name); err != nil {
			return nil, fmt.Errorf("reading register name: %w", err)
		}
		var value uint64
		if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
			return nil, fmt.Errorf("reading register value: %w", err)
		}
		var bitWidth uint32
		if err := binary.Read(r, binary.LittleEndian, &bitWidth); err != nil {
			return nil, fmt.Errorf("reading register width: %w", err)
		}
		regs[i] = RegisterInfo{
			Name:     string(name),
			Value:    value,
			BitWidth: int(bitWidth),
		}
	}

	// Memory
	var uncompressedLen uint32
	if err := binary.Read(r, binary.LittleEndian, &uncompressedLen); err != nil {
		return nil, fmt.Errorf("reading memory length: %w", err)
	}
	if uncompressedLen > snapshotMaxMemory {
		return nil, fmt.Errorf("snapshot memory length %d exceeds maximum %d", uncompressedLen, snapshotMaxMemory)
	}

	remaining := data[len(data)-r.Len():]
	gz, err := gzip.NewReader(bytes.NewReader(remaining))
	if err != nil {
		return nil, fmt.Errorf("opening gzip reader: %w", err)
	}
	defer gz.Close()

	mem := make([]byte, uncompressedLen)
	if _, err := io.ReadFull(gz, mem); err != nil {
		return nil, fmt.Errorf("decompressing memory: %w", err)
	}

	return &MachineSnapshot{
		CPUType:   string(cpuType),
		Registers: regs,
		Memory:    mem,
	}, nil
}
