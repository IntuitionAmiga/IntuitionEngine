package main

import "testing"

type testSnapshotDevice struct {
	name    string
	version uint32
	data    []byte
}

func (d *testSnapshotDevice) DebugSnapshotName() string {
	return d.name
}

func (d *testSnapshotDevice) DebugSnapshot() (uint32, []byte, error) {
	return d.version, append([]byte(nil), d.data...), nil
}

func (d *testSnapshotDevice) DebugRestoreSnapshot(version uint32, data []byte) error {
	d.version = version
	d.data = append([]byte(nil), data...)
	return nil
}

func TestWholeMachineSnapshot_RoundTripRegisteredCPUsAndBus(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(DEFAULT_MEMORY_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	ie64 := NewCPU64(bus)
	ie64.PC = 0x1000
	ie64.regs[1] = 0x1111
	ie64.memory[0x80] = 0x64

	ie32 := NewCPU(bus)
	ie32.PC = 0x2000
	ie32.A = 0x2222
	ie32.memory[0x90] = 0x32
	bus.memory[0x40] = 0xB5

	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("main", NewDebugIE64(ie64))
	mon.RegisterCPU("coproc", NewDebugIE32(ie32))

	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.CPUs) != 2 {
		t.Fatalf("captured CPUs = %d, want 2", len(snap.CPUs))
	}
	if len(snap.Bus.Pages) != 1 || snap.Bus.Pages[0].Addr != 0 {
		t.Fatalf("bus sparse pages = %+v, want one page at zero", snap.Bus.Pages)
	}

	ie64.PC = 0x3000
	ie64.regs[1] = 0
	ie64.memory[0x80] = 0
	ie32.PC = 0x4000
	ie32.A = 0
	ie32.memory[0x90] = 0
	bus.memory[0x40] = 0

	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if ie64.PC != 0x1000 || ie64.regs[1] != 0x1111 || ie64.memory[0x80] != 0x64 {
		t.Fatalf("IE64 restore failed: pc=$%X r1=$%X mem=$%X", ie64.PC, ie64.regs[1], ie64.memory[0x80])
	}
	if ie32.PC != 0x2000 || ie32.A != 0x2222 || ie32.memory[0x90] != 0x32 {
		t.Fatalf("IE32 restore failed: pc=$%X a=$%X mem=$%X", ie32.PC, ie32.A, ie32.memory[0x90])
	}
	if bus.memory[0x40] != 0xB5 {
		t.Fatalf("bus memory restored to $%X, want $B5", bus.memory[0x40])
	}
}

func TestWholeMachineSnapshot_RoundTripRegisteredDevices(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	dev := &testSnapshotDevice{name: "test-device", version: 7, data: []byte{1, 2, 3}}
	mon.RegisterSnapshotDevice(dev)

	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Devices) != 1 || snap.Devices[0].Name != "test-device" || snap.Devices[0].Version != 7 {
		t.Fatalf("device blobs = %+v", snap.Devices)
	}

	dev.version = 1
	dev.data = []byte{9}
	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if dev.version != 7 || len(dev.data) != 3 || dev.data[2] != 3 {
		t.Fatalf("device restore version=%d data=%v", dev.version, dev.data)
	}
}

func TestWholeMachineSnapshot_RoundTripGuestVisibleRuntimeDevices(t *testing.T) {
	bus, err := NewMachineBusSized(64 * 1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}
	bus.SetBacking(NewSparseBacking(uint64(AROS_PROFILE_TOP)))
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    uint64(AROS_PROFILE_TOP),
		ActiveVisibleRAM: uint64(AROS_PROFILE_TOP),
	})
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	term := NewTerminalMMIO()
	term.EnqueueByte('A')
	term.HandleWrite(TERM_ECHO, 1)
	clip := NewClipboardBridge(bus)
	clip.HandleWrite(CLIP_DATA_PTR, 0x100)
	clip.HandleWrite(CLIP_DATA_LEN, 12)
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	dma, err := NewArosAudioDMA(bus, sound, NewM68KCPU(bus))
	if err != nil {
		t.Fatal(err)
	}
	writeArosDMAChannel(dma, 0, 0x120, 2, 128, 64)
	armArosDMAChannel(dma, 0)
	mon.RegisterSnapshotDevice(term)
	mon.RegisterSnapshotDevice(clip)
	mon.RegisterSnapshotDevice(dma)

	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	term.HandleRead(TERM_IN)
	term.HandleWrite(TERM_ECHO, 0)
	clip.HandleWrite(CLIP_DATA_PTR, 0x200)
	dma.Reset()

	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if got := term.HandleRead(TERM_ECHO); got != 1 {
		t.Fatalf("terminal echo=%d, want 1", got)
	}
	if got := term.HandleRead(TERM_IN); got != 'A' {
		t.Fatalf("terminal input=%q, want A", byte(got))
	}
	if got := clip.HandleRead(CLIP_DATA_PTR); got != 0x100 {
		t.Fatalf("clipboard data ptr=$%X, want $100", got)
	}
	if got := dma.HandleRead(AROS_AUD_DMACON); got&1 == 0 {
		t.Fatalf("DMA dmacon=$%X, want channel 0 restored active", got)
	}
}

func TestWholeMachineSnapshot_MissingDeviceFailsRestore(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	mon.RegisterSnapshotDevice(&testSnapshotDevice{name: "missing-later", version: 1, data: []byte{1}})
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}

	next := NewMachineMonitor(bus)
	next.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	if err := RestoreWholeMachineSnapshot(next, snap); err == nil {
		t.Fatal("restore succeeded without captured device registered")
	}
}

func TestWholeMachineSnapshot_CapturesSparseBackingPages(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	const backingSize = uint64(128 * 1024 * 1024)
	backing := NewSparseBacking(backingSize)
	bus.SetBacking(backing)
	backing.Write8(uint64(MMU_PAGE_SIZE*5)+7, 0xA7)

	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Bus.BackingSize != backing.Size() || len(snap.Bus.BackingPages) != 1 {
		t.Fatalf("backing snapshot = size %d pages %+v", snap.Bus.BackingSize, snap.Bus.BackingPages)
	}

	backing.Write8(uint64(MMU_PAGE_SIZE*5)+7, 0)
	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if got := backing.Read8(uint64(MMU_PAGE_SIZE*5) + 7); got != 0xA7 {
		t.Fatalf("backing restored $%X, want $A7", got)
	}
}

func TestWholeMachineSnapshot_RestoreInvalidatesBackingPagesAfterWrite(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	const backingSize = uint64(128 * 1024 * 1024)
	backing := NewSparseBacking(backingSize)
	bus.SetBacking(backing)
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    backing.Size(),
		ActiveVisibleRAM: backing.Size(),
	})
	target := uint64(64*1024*1024 + 7)
	backing.Write8(target, 0xA7)

	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Bus.BackingPages) != 1 || snap.Bus.BackingPages[0].Addr > target ||
		target >= snap.Bus.BackingPages[0].Addr+uint64(len(snap.Bus.BackingPages[0].Data)) {
		t.Fatalf("snapshot backing pages %+v do not cover target $%X", snap.Bus.BackingPages, target)
	}

	backing.Write8(target, 0)
	var sawBackingInvalidation bool
	var observed byte
	type invalidationCall struct {
		addr uint64
		size uint64
	}
	var calls []invalidationCall
	bus.RegisterM68KJITInvalidator(func(addr, size uint64) {
		calls = append(calls, invalidationCall{addr: addr, size: size})
		if addr <= target && target < addr+size {
			sawBackingInvalidation = true
			observed = backing.Read8(target)
		}
	})

	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if !sawBackingInvalidation {
		t.Fatalf("restore did not invalidate restored backing page; invalidations=%+v", calls)
	}
	if observed != 0xA7 {
		t.Fatalf("invalidator observed backing byte $%X, want restored byte $A7", observed)
	}
}

func TestWholeMachineSnapshot_RestoreInvalidatesBackingPagesRemovedByReset(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	const backingSize = uint64(128 * 1024 * 1024)
	backing := NewSparseBacking(backingSize)
	bus.SetBacking(backing)
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    backing.Size(),
		ActiveVisibleRAM: backing.Size(),
	})

	kept := uint64(64*1024*1024 + 7)
	removed := uint64(80*1024*1024 + 11)
	backing.Write8(kept, 0xA7)

	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}

	backing.Write8(removed, 0x5A)
	var sawRemovedInvalidation bool
	var observed byte
	type invalidationCall struct {
		addr uint64
		size uint64
	}
	var calls []invalidationCall
	bus.RegisterM68KJITInvalidator(func(addr, size uint64) {
		calls = append(calls, invalidationCall{addr: addr, size: size})
		if addr <= removed && removed < addr+size {
			sawRemovedInvalidation = true
			observed = backing.Read8(removed)
		}
	})

	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if !sawRemovedInvalidation {
		t.Fatalf("restore did not invalidate backing page removed by reset; invalidations=%+v", calls)
	}
	if observed != 0 {
		t.Fatalf("invalidator observed removed backing byte $%X, want reset byte $00", observed)
	}
}

func TestWholeMachineSnapshot_RestoreWithoutBackingClearsCurrentBacking(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Bus.BackingSize != 0 {
		t.Fatalf("snapshot backing size = %d, want none", snap.Bus.BackingSize)
	}

	const backingSize = uint64(128 * 1024 * 1024)
	backing := NewSparseBacking(backingSize)
	removed := uint64(64*1024*1024 + 3)
	backing.Write8(removed, 0x5A)
	bus.SetBacking(backing)
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    backing.Size(),
		ActiveVisibleRAM: backing.Size(),
	})
	var sawRemovedInvalidation bool
	type invalidationCall struct {
		addr uint64
		size uint64
	}
	var calls []invalidationCall
	bus.RegisterM68KJITInvalidator(func(addr, size uint64) {
		if len(calls) < 16 {
			calls = append(calls, invalidationCall{addr: addr, size: size})
		}
		if addr <= removed && removed < addr+size {
			sawRemovedInvalidation = true
		}
	})
	if err := RestoreWholeMachineSnapshot(mon, snap); err != nil {
		t.Fatal(err)
	}
	if bus.Backing() != nil {
		t.Fatal("restore of no-backing snapshot left current backing installed")
	}
	if !sawRemovedInvalidation {
		t.Fatalf("restore of no-backing snapshot did not invalidate removed backing range; invalidations=%+v", calls)
	}
}

func TestWholeMachineSnapshotHistory_MaterialisesDeltaChain(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE * 2))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	mon.wholeCheckpointInterval = 32
	mon.maxWholeHistory = 8

	bus.memory[0x10] = 0x11
	cpu.regs[1] = 1
	mon.recordWholeMachineHistory()
	bus.memory[0x10] = 0x22
	cpu.regs[1] = 2
	mon.recordWholeMachineHistory()

	if len(mon.wholeHistory) != 2 {
		t.Fatalf("history len = %d, want 2", len(mon.wholeHistory))
	}
	if !mon.wholeHistory[0].Full {
		t.Fatal("first history entry must be a full checkpoint")
	}
	if mon.wholeHistory[1].Full || mon.wholeHistory[1].BaseID != mon.wholeHistory[0].ID {
		t.Fatalf("second history entry = full %v base %d, want delta from %d", mon.wholeHistory[1].Full, mon.wholeHistory[1].BaseID, mon.wholeHistory[0].ID)
	}

	bus.memory[0x10] = 0
	cpu.regs[1] = 0
	material, err := mon.materializeWholeMachineSnapshotLocked(mon.wholeHistory[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := mon.restoreWholeMachineSnapshotLocked(material); err != nil {
		t.Fatal(err)
	}
	if bus.memory[0x10] != 0x22 || cpu.regs[1] != 2 {
		t.Fatalf("materialised restore bus=$%X r1=%d", bus.memory[0x10], cpu.regs[1])
	}
}

func TestWholeMachineSnapshotHistory_PrunesByCheckpointRetention(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(NewCPU64(bus)))
	mon.wholeCheckpointInterval = 1
	mon.maxWholeCheckpoints = 2
	mon.maxWholeHistory = 16

	for i := 0; i < 6; i++ {
		bus.memory[0] = byte(i + 1)
		mon.recordWholeMachineHistory()
	}
	checkpoints, _, _ := mon.wholeHistoryStatsLocked()
	if checkpoints > 2 {
		t.Fatalf("retained checkpoints = %d, want <= 2", checkpoints)
	}
	if len(mon.wholeHistory) == 0 || !mon.wholeHistory[0].Full {
		t.Fatalf("history must start at a retained checkpoint: %+v", mon.wholeHistory)
	}
}
