package main

import (
	"strings"
	"testing"
)

func TestRegions_Lookup_PerCPU_DivergentMappings(t *testing.T) {
	regions := NewRegionRegistry()

	if got := regions.Lookup("6502", 0xF000); got == nil || got.Name != "direct-mmio" {
		t.Fatalf("6502 0xF000 region = %#v, want direct-mmio", got)
	}
	if got := regions.Lookup("IE64", 0xF000); got == nil || got.Kind != RegionRAM {
		t.Fatalf("IE64 0xF000 region = %#v, want RAM", got)
	}
	if got := regions.Lookup("Z80", 0x00A0); got == nil || got.Name != "z80-vga-ports" {
		t.Fatalf("Z80 0x00A0 region = %#v, want z80-vga-ports", got)
	}
	if got := regions.Lookup("M68K", 0xF0000); got == nil || got.Kind != RegionMMIO {
		t.Fatalf("M68K 0xF0000 region = %#v, want MMIO", got)
	}
}

func TestMapCommand_ListsRegions(t *testing.T) {
	mon, _ := newTestMonitor()
	_, out := mon.ExecuteCommandResult("map")
	var sawMMIO bool
	for _, line := range out {
		if strings.Contains(line.Text, "mmio") {
			sawMMIO = true
			break
		}
	}
	if !sawMMIO {
		t.Fatalf("map output did not include mmio: %#v", out)
	}
}

func TestAddrCommand_UsesSymbolsAndRegions(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.ExecuteCommand("sym add io $F0000 label")
	_, out := mon.ExecuteCommandResult("addr io")
	if len(out) == 0 || !strings.Contains(out[len(out)-1].Text, "MMIO") {
		t.Fatalf("addr io output = %#v, want MMIO region", out)
	}
}
