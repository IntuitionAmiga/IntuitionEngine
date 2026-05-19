package main

import (
	"sort"
	"strings"
	"sync"
)

type RegionKind string

const (
	RegionRAM   RegionKind = "RAM"
	RegionROM   RegionKind = "ROM"
	RegionMMIO  RegionKind = "MMIO"
	RegionVRAM  RegionKind = "VRAM"
	RegionStack RegionKind = "STACK"
	RegionJIT   RegionKind = "JIT"
)

type MemoryRegion struct {
	Start uint64
	End   uint64
	Name  string
	Kind  RegionKind
	CPUs  []string
}

type RegionRegistry struct {
	mu      sync.RWMutex
	regions []MemoryRegion
}

func NewRegionRegistry() *RegionRegistry {
	rr := &RegionRegistry{}
	rr.addDefaults()
	return rr
}

func normalizeRegionCPU(cpu string) string {
	return strings.ToUpper(strings.TrimSpace(cpu))
}

func (rr *RegionRegistry) Add(start, end uint64, name string, kind RegionKind, cpus ...string) {
	if rr == nil || end < start {
		return
	}
	region := MemoryRegion{Start: start, End: end, Name: name, Kind: kind}
	for _, cpu := range cpus {
		region.CPUs = append(region.CPUs, normalizeRegionCPU(cpu))
	}
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.regions = append(rr.regions, region)
	sort.Slice(rr.regions, func(i, j int) bool {
		if rr.regions[i].Start == rr.regions[j].Start {
			return rr.regions[i].End < rr.regions[j].End
		}
		return rr.regions[i].Start < rr.regions[j].Start
	})
}

func (rr *RegionRegistry) Lookup(cpu string, addr uint64) *MemoryRegion {
	if rr == nil {
		return nil
	}
	cpu = normalizeRegionCPU(cpu)
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	var best *MemoryRegion
	var bestSpan uint64
	for i := range rr.regions {
		region := rr.regions[i]
		if addr >= region.Start && addr <= region.End && region.matchesCPU(cpu) {
			cp := region
			span := cp.End - cp.Start
			if best == nil || span < bestSpan {
				best = &cp
				bestSpan = span
			}
		}
	}
	return best
}

func (rr *RegionRegistry) List(cpu string) []MemoryRegion {
	if rr == nil {
		return nil
	}
	cpu = normalizeRegionCPU(cpu)
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	out := make([]MemoryRegion, 0, len(rr.regions))
	for _, region := range rr.regions {
		if region.matchesCPU(cpu) {
			out = append(out, region)
		}
	}
	return out
}

func (rr *RegionRegistry) LookupName(cpu, name string) *MemoryRegion {
	if rr == nil {
		return nil
	}
	cpu = normalizeRegionCPU(cpu)
	name = strings.ToLower(strings.TrimSpace(name))
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	for i := range rr.regions {
		region := rr.regions[i]
		if strings.EqualFold(region.Name, name) && region.matchesCPU(cpu) {
			cp := region
			return &cp
		}
	}
	return nil
}

func (r MemoryRegion) matchesCPU(cpu string) bool {
	if len(r.CPUs) == 0 {
		return true
	}
	for _, c := range r.CPUs {
		if c == cpu || c == "ALL" {
			return true
		}
	}
	return false
}

func (rr *RegionRegistry) addDefaults() {
	common := []string{"IE64", "IE32", "M68K", "X86"}
	rr.Add(0x00000, 0x9EFFF, "main-ram", RegionRAM, common...)
	rr.Add(0x9F000, 0x9FFFF, "stack", RegionStack, common...)
	rr.Add(0xA0000, 0xAFFFF, "vga-vram", RegionVRAM, common...)
	rr.Add(0xB8000, 0xBFFFF, "vga-text", RegionVRAM, common...)
	rr.Add(0xD0000, 0xDFFFF, "voodoo-texture", RegionVRAM, common...)
	rr.Add(0xF0000, 0xFFFFF, "mmio", RegionMMIO, common...)
	rr.Add(HOST_MMIO_REGION_BASE, HOST_MMIO_REGION_END, "host-helper", RegionMMIO, common...)
	rr.Add(0x100000, 0x5FFFFF, "video-ram", RegionVRAM, common...)

	rr.Add(0x0000, 0xCFFF, "main-ram", RegionRAM, "6502", "Z80")
	rr.Add(0x0100, 0x01FF, "stack", RegionStack, "6502")
	rr.Add(0xD700, 0xD70A, "vga-ports", RegionMMIO, "6502")
	rr.Add(0xD800, 0xD817, "ula-ports", RegionMMIO, "6502")
	rr.Add(0xF000, 0xF0FF, "direct-mmio", RegionMMIO, "6502", "Z80")
	rr.Add(0x00A0, 0x00AA, "z80-vga-ports", RegionMMIO, "Z80")
}
