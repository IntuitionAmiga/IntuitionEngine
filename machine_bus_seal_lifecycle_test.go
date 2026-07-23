// machine_bus_seal_lifecycle_test.go - seal policy across reset and reload.
//
// Sealing freezes the MMIO map so the hot path reads a plain field instead of
// an atomic pointer, and so the I/O page bitmap cannot change underneath an
// access. The lifecycle question these tests settle is what happens to that
// seal at the two points a machine changes shape: a guest reset, and a reload
// of a different program.
//
// The policy, pinned below, is:
//
//   - Reset does not touch the seal. A guest reset clears RAM and resets
//     devices, but the devices are still present and still mapped, so the map
//     remains valid and there is no reason to give up the sealed fast path.
//   - Reload unseals first, remaps, and reseals when the next runner starts.
//     ProgramExecutor.prepareAndLaunch and Machine.ResetDevicesBeforeLoad both
//     call UnsealMappings before touching the map. That call is required, not
//     advisory: MapIO panics on a sealed bus.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import "testing"

const sealTestIOBase uint32 = 0x30000

// TestBusReseal_AfterReset pins that a guest reset leaves the seal and the
// sealed map alone.
func TestBusReseal_AfterReset(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(sealTestIOBase, sealTestIOBase+0xFF, func(uint32) uint32 { return 0x5A }, nil)
	bus.SealMappings()

	if !bus.mappingsSealed() {
		t.Fatal("bus did not seal")
	}
	if got := bus.Read32(sealTestIOBase); got != 0x5A {
		t.Fatalf("mapped read before reset = 0x%X, want 0x5A", got)
	}

	bus.Reset()

	if !bus.mappingsSealed() {
		t.Fatal("reset dropped the seal; devices are still mapped, so the sealed fast path should survive")
	}
	if got := bus.Read32(sealTestIOBase); got != 0x5A {
		t.Fatalf("mapped read after reset = 0x%X, want 0x5A: reset lost the MMIO map", got)
	}
	if !bus.IsIOPageMapped(sealTestIOBase >> 8) {
		t.Fatal("I/O page bitmap lost its entry across reset")
	}
}

// TestBusReseal_AfterReload pins the reload sequence: unseal, remap, reseal,
// with the new map live and the old one gone.
func TestBusReseal_AfterReload(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(sealTestIOBase, sealTestIOBase+0xFF, func(uint32) uint32 { return 0x5A }, nil)
	bus.SealMappings()

	// Remapping while sealed is a programming error, not a silent no-op. This
	// is why the reload path must unseal explicitly before it touches the map.
	expectPanic(t, func() {
		bus.MapIO(sealTestIOBase+0x1000, sealTestIOBase+0x10FF, func(uint32) uint32 { return 0xA5 }, nil)
	})

	// The reload sequence proper.
	bus.UnsealMappings()
	if bus.mappingsSealed() {
		t.Fatal("UnsealMappings left the bus sealed")
	}
	bus.UnmapIO(sealTestIOBase, sealTestIOBase+0xFF)
	bus.MapIO(sealTestIOBase+0x1000, sealTestIOBase+0x10FF, func(uint32) uint32 { return 0xA5 }, nil)
	bus.Reset()
	bus.SealMappings()

	if !bus.mappingsSealed() {
		t.Fatal("reload did not reseal")
	}
	if bus.IsIOPageMapped(sealTestIOBase >> 8) {
		t.Fatal("the previous program's MMIO mapping survived the reload")
	}
	if got := bus.Read32(sealTestIOBase + 0x1000); got != 0xA5 {
		t.Fatalf("reloaded mapping read = 0x%X, want 0xA5: reseal published a stale snapshot", got)
	}
}

// TestBusReseal_SealIsIdempotent pins that repeating either half of the
// lifecycle is harmless, because a lifecycle owner cannot always know whether
// a previous runner sealed already.
func TestBusReseal_SealIsIdempotent(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(sealTestIOBase, sealTestIOBase+0xFF, func(uint32) uint32 { return 0x5A }, nil)
	bus.SealMappings()
	bus.SealMappings()
	if got := bus.Read32(sealTestIOBase); got != 0x5A {
		t.Fatalf("mapped read after double seal = 0x%X, want 0x5A", got)
	}
	bus.UnsealMappings()
	bus.UnsealMappings()
	if bus.mappingsSealed() {
		t.Fatal("double unseal left the bus sealed")
	}
	if got := bus.Read32(sealTestIOBase); got != 0x5A {
		t.Fatalf("mapped read after unseal = 0x%X, want 0x5A: unsealing dropped the map", got)
	}
}
