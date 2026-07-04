package main

import (
	"os"
	"sort"
	"sync/atomic"
)

var mmioStatsOn = os.Getenv("IE_MMIO_STATS") == "1"

type IODeviceStats struct {
	Start  uint32
	End    uint32
	Reads  atomic.Uint64
	Writes atomic.Uint64
}

type MMIOStatsRow struct {
	Start  uint32
	End    uint32
	Name   string
	Reads  uint64
	Writes uint64
}

func MMIOStatsEnabled() bool {
	return mmioStatsOn
}

func mmioStatsForceEnableForTest(on bool) {
	mmioStatsOn = on
}

func (bus *MachineBus) registerMMIOStats(start, end uint32) int32 {
	for i, stats := range bus.mmioStats {
		if stats.Start == start && stats.End == end {
			return int32(i)
		}
	}
	stats := &IODeviceStats{Start: start, End: end}
	bus.mmioStats = append(bus.mmioStats, stats)
	return int32(len(bus.mmioStats) - 1)
}

func (bus *MachineBus) mmioStatsRecordRead(idx int32) {
	if !mmioStatsOn || idx < 0 {
		return
	}
	stats := bus.mmioStatsByIndex(idx)
	if stats != nil {
		stats.Reads.Add(1)
	}
}

func (bus *MachineBus) mmioStatsRecordWrite(idx int32) {
	if !mmioStatsOn || idx < 0 {
		return
	}
	stats := bus.mmioStatsByIndex(idx)
	if stats != nil {
		stats.Writes.Add(1)
	}
}

func (bus *MachineBus) mmioStatsByIndex(idx int32) *IODeviceStats {
	if idx < 0 || int(idx) >= len(bus.mmioStats) {
		return nil
	}
	return bus.mmioStats[idx]
}

func (bus *MachineBus) mmioStatsIndexForAddr(addr uint32) int32 {
	if regions, exists := bus.legacyRegions(addr & PAGE_MASK); exists {
		for i := len(regions) - 1; i >= 0; i-- {
			region := regions[i]
			if addr >= region.start && addr <= region.end {
				return region.statsIdx
			}
		}
	}
	return -1
}

func (bus *MachineBus) MMIOStatsSnapshot() []MMIOStatsRow {
	rows := make([]MMIOStatsRow, 0, len(bus.mmioStats))
	for _, stats := range bus.mmioStats {
		rows = append(rows, MMIOStatsRow{
			Start:  stats.Start,
			End:    stats.End,
			Name:   mmioStatsName(stats.Start, stats.End),
			Reads:  stats.Reads.Load(),
			Writes: stats.Writes.Load(),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Start == rows[j].Start {
			return rows[i].End < rows[j].End
		}
		return rows[i].Start < rows[j].Start
	})
	return rows
}

func mmioStatsName(start, end uint32) string {
	for _, dev := range ioDevices {
		for _, reg := range dev.Registers {
			if reg.Addr >= start && reg.Addr <= end {
				return dev.Name
			}
		}
	}
	return ""
}
