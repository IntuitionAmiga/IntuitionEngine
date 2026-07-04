package main

import (
	"sync"
	"testing"
)

func withMMIOStatsForTest(t *testing.T, enabled bool) {
	t.Helper()
	old := mmioStatsOn
	mmioStatsForceEnableForTest(enabled)
	t.Cleanup(func() { mmioStatsForceEnableForTest(old) })
}

func findMMIOStatsRow(rows []MMIOStatsRow, start, end uint32) (MMIOStatsRow, bool) {
	for _, row := range rows {
		if row.Start == start && row.End == end {
			return row, true
		}
	}
	return MMIOStatsRow{}, false
}

func TestMachineBusMMIOStatsCountsMappedReadsAndWrites(t *testing.T) {
	withMMIOStatsForTest(t, true)
	bus := NewMachineBus()

	bus.MapIO(0xF1000, 0xF1003,
		func(addr uint32) uint32 { return 0x12345678 },
		func(addr uint32, value uint32) {},
	)

	if got := bus.Read32(0xF1000); got != 0x12345678 {
		t.Fatalf("Read32 = %#x, want 0x12345678", got)
	}
	bus.Write32(0xF1000, 0xCAFEBABE)

	row, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), 0xF1000, 0xF1003)
	if !ok {
		t.Fatal("missing stats row for mapped device")
	}
	if row.Reads != 1 || row.Writes != 1 {
		t.Fatalf("stats reads=%d writes=%d, want 1/1", row.Reads, row.Writes)
	}
}

func TestMachineBusMMIOStatsCountsFaultingMappedReadsAndWrites(t *testing.T) {
	withMMIOStatsForTest(t, true)
	bus := NewMachineBus()

	bus.MapIO(0xF1000, 0xF1003,
		func(addr uint32) uint32 { return 0x12345678 },
		func(addr uint32, value uint32) {},
	)

	if got, ok := bus.Read32WithFault(0xF1000); !ok || got != 0x12345678 {
		t.Fatalf("Read32WithFault = %#x/%v, want 0x12345678/true", got, ok)
	}
	if ok := bus.Write32WithFault(0xF1000, 0xCAFEBABE); !ok {
		t.Fatal("Write32WithFault failed")
	}

	row, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), 0xF1000, 0xF1003)
	if !ok {
		t.Fatal("missing stats row for mapped device")
	}
	if row.Reads != 1 || row.Writes != 1 {
		t.Fatalf("faulting stats reads=%d writes=%d, want 1/1", row.Reads, row.Writes)
	}
}

func TestMachineBusMMIOStatsGateOffLeavesCountsZero(t *testing.T) {
	withMMIOStatsForTest(t, false)
	bus := NewMachineBus()

	bus.MapIO(0xF1000, 0xF1003,
		func(addr uint32) uint32 { return 1 },
		func(addr uint32, value uint32) {},
	)
	_ = bus.Read32(0xF1000)
	bus.Write32(0xF1000, 2)

	row, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), 0xF1000, 0xF1003)
	if !ok {
		t.Fatal("missing stats row for mapped device")
	}
	if row.Reads != 0 || row.Writes != 0 {
		t.Fatalf("gate-off stats reads=%d writes=%d, want 0/0", row.Reads, row.Writes)
	}
}

func TestMachineBusMMIOStatsCountsVideoStatusFastPath(t *testing.T) {
	withMMIOStatsForTest(t, true)
	bus := NewMachineBus()
	chip := &VideoChip{}
	chip.vblankCond = sync.NewCond(&chip.vblankMu)

	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, chip.HandleRead, chip.HandleWrite)
	bus.SetVideoStatusReader(chip.HandleRead)
	_ = bus.Read32(VIDEO_STATUS)

	row, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), VIDEO_CTRL, VIDEO_REG_END)
	if !ok {
		t.Fatal("missing stats row for video region")
	}
	if row.Reads != 1 {
		t.Fatalf("video fast-path reads=%d, want 1", row.Reads)
	}
}

func TestMachineBusMMIOStatsOverlappingRegionsUseMatchedRegion(t *testing.T) {
	withMMIOStatsForTest(t, true)
	bus := NewMachineBus()

	bus.MapIO(0xF1000, 0xF10FF, nil, func(addr uint32, value uint32) {})
	bus.MapIO(0xF1004, 0xF1007, nil, func(addr uint32, value uint32) {})
	bus.Write32(0xF1004, 1)

	wide, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), 0xF1000, 0xF10FF)
	if !ok {
		t.Fatal("missing wide stats row")
	}
	narrow, ok := findMMIOStatsRow(bus.MMIOStatsSnapshot(), 0xF1004, 0xF1007)
	if !ok {
		t.Fatal("missing narrow stats row")
	}
	if wide.Writes != 0 || narrow.Writes != 1 {
		t.Fatalf("wide writes=%d narrow writes=%d, want 0/1", wide.Writes, narrow.Writes)
	}
}
