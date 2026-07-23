// machine_bus_span_test.go - differentials for bulk span transfers.
//
// The contract is that ReadSpan and WriteSpan are indistinguishable from the
// byte loops they replace, so every test here runs both and compares. The
// interesting cases are the ones where the bulk path must refuse: MMIO inside
// the span, watchpoints that have to fire per byte, and the seam between low
// memory and the backing.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"bytes"
	"math/rand"
	"testing"
)

const spanTestBase uint32 = 0x100000

// spanTestBus returns a bus with a sparse backing bound above low memory, so
// both sides of the seam are reachable.
func spanTestBus() (*MachineBus, uint64) {
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(64 << 20))
	return bus, uint64(len(bus.memory))
}

// byteLoopRead is the oracle: what the call site did before the span API.
func byteLoopRead(bus *MachineBus, addr uint64, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = bus.ReadPhys8(addr + uint64(i))
	}
	return out
}

// byteLoopWrite is the write-side oracle.
func byteLoopWrite(bus *MachineBus, addr uint64, src []byte) {
	for i, v := range src {
		bus.WritePhys8(addr+uint64(i), v)
	}
}

// TestBusReadSpan_MatchesByteLoop compares the bulk path against the byte loop
// over low RAM, backing RAM, spans crossing the seam, spans overlapping an MMIO
// region and spans running off the end of mapped memory.
func TestBusReadSpan_MatchesByteLoop(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	rng := rand.New(rand.NewSource(7))
	bus, seam := spanTestBus()
	bus.MapIO(spanTestBase+0x1000, spanTestBase+0x10FF,
		func(addr uint32) uint32 { return uint32(addr & 0xFF) }, func(uint32, uint32) {})

	// Seed both sides of the seam with distinguishable data.
	for i := range 0x4000 {
		bus.memory[int(spanTestBase)+i] = byte(rng.Intn(256))
	}
	backingSeed := make([]byte, 0x4000)
	for i := range backingSeed {
		backingSeed[i] = byte(rng.Intn(256))
	}
	bus.backing.WriteBytes(seam, backingSeed)

	cases := []struct {
		name string
		addr uint64
		n    int
	}{
		{"LowRAM", uint64(spanTestBase), 0x800},
		{"LowRAMUnaligned", uint64(spanTestBase) + 3, 0x7FD},
		{"OverMMIO", uint64(spanTestBase) + 0xF00, 0x400},
		{"EndsInMMIO", uint64(spanTestBase) + 0xFF0, 0x20},
		{"Backing", seam + 0x100, 0x800},
		{"CrossesSeam", seam - 0x40, 0x80},
		{"PastBacking", bus.backing.Size() - 0x20, 0x80},
		{"Single", uint64(spanTestBase) + 0x55, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := byteLoopRead(bus, tc.addr, tc.n)
			got := make([]byte, tc.n)
			bus.ReadSpan(tc.addr, got)
			if !bytes.Equal(got, want) {
				t.Fatalf("ReadSpan differs from the byte loop at 0x%X len %d", tc.addr, tc.n)
			}
		})
	}
}

// TestBusWriteSpan_MatchesByteLoop writes the same payload through both paths
// on two separate buses and compares the resulting memory, so a difference in
// where the bytes land shows up as well as a difference in what was written.
func TestBusWriteSpan_MatchesByteLoop(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	rng := rand.New(rand.NewSource(11))
	payload := make([]byte, 0x900)
	for i := range payload {
		payload[i] = byte(rng.Intn(256))
	}

	build := func() (*MachineBus, uint64, *[]uint32) {
		bus, seam := spanTestBus()
		var mmioWrites []uint32
		bus.MapIO(spanTestBase+0x1000, spanTestBase+0x10FF,
			func(addr uint32) uint32 { return 0 },
			func(addr uint32, value uint32) { mmioWrites = append(mmioWrites, addr) })
		return bus, seam, &mmioWrites
	}

	cases := []struct {
		name string
		addr func(seam uint64) uint64
		n    int
	}{
		{"LowRAM", func(uint64) uint64 { return uint64(spanTestBase) }, 0x800},
		{"OverMMIO", func(uint64) uint64 { return uint64(spanTestBase) + 0xF00 }, 0x400},
		{"Backing", func(seam uint64) uint64 { return seam + 0x100 }, 0x800},
		{"CrossesSeam", func(seam uint64) uint64 { return seam - 0x40 }, 0x80},
		{"PastBacking", func(uint64) uint64 { return 0 }, 0},
	}
	for _, tc := range cases {
		if tc.n == 0 {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			loopBus, loopSeam, loopMMIO := build()
			spanBus, spanSeam, spanMMIO := build()
			src := payload[:tc.n]

			byteLoopWrite(loopBus, tc.addr(loopSeam), src)
			spanBus.WriteSpan(tc.addr(spanSeam), src)

			if !bytes.Equal(loopBus.memory, spanBus.memory) {
				t.Fatal("low memory differs between the byte loop and WriteSpan")
			}
			loopBacking := byteLoopRead(loopBus, loopSeam, 0x1000)
			spanBacking := byteLoopRead(spanBus, spanSeam, 0x1000)
			if !bytes.Equal(loopBacking, spanBacking) {
				t.Fatal("backing memory differs between the byte loop and WriteSpan")
			}
			if len(*loopMMIO) != len(*spanMMIO) {
				t.Fatalf("MMIO write count differs: byte loop %d, WriteSpan %d", len(*loopMMIO), len(*spanMMIO))
			}
			for i := range *loopMMIO {
				if (*loopMMIO)[i] != (*spanMMIO)[i] {
					t.Fatalf("MMIO write %d hit 0x%X via the byte loop and 0x%X via WriteSpan",
						i, (*loopMMIO)[i], (*spanMMIO)[i])
				}
			}
		})
	}
}

// TestBusSpan_DebugHistoryStillRecordsPerByte is the guard that matters most.
// Bus-level accesses are reported with CPU identity -1, which the guard and
// watch scans skip, so the observable the span path can destroy is the access
// history: one entry per byte, in order, with the old and new values. A span
// over memory the debug service is recording must produce exactly the history
// the byte loop produced, which means the bulk path has to refuse it outright.
func TestBusSpan_DebugHistoryStillRecordsPerByte(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	build := func() (*MachineBus, *DebugAccessService) {
		bus := NewMachineBus()
		access := NewDebugAccessService()
		bus.SetDebugAccessService(access)
		access.EnableHistory(1024)
		return bus, access
	}

	payload := make([]byte, 0x40)
	for i := range payload {
		payload[i] = byte(i + 1)
	}

	sameHistory := func(t *testing.T, what string, a, b []AccessEvent) {
		t.Helper()
		if len(a) != len(b) {
			t.Fatalf("%s recorded %d history entries, the byte loop recorded %d", what, len(b), len(a))
		}
		for i := range a {
			if a[i].Address != b[i].Address || a[i].Width != b[i].Width ||
				a[i].Kind != b[i].Kind || a[i].NewValue != b[i].NewValue ||
				a[i].OldValue != b[i].OldValue || a[i].OldValueKnown != b[i].OldValueKnown {
				t.Fatalf("%s history entry %d differs: byte loop %+v, span %+v", what, i, a[i], b[i])
			}
		}
	}

	loopBus, loopAccess := build()
	spanBus, spanAccess := build()
	byteLoopWrite(loopBus, uint64(spanTestBase), payload)
	spanBus.WriteSpan(uint64(spanTestBase), payload)
	sameHistory(t, "WriteSpan", loopAccess.HistoryTail(0), spanAccess.HistoryTail(0))
	if len(loopAccess.HistoryTail(0)) != len(payload) {
		t.Fatalf("the byte loop oracle recorded %d entries for %d bytes; the test is not observing what it claims",
			len(loopAccess.HistoryTail(0)), len(payload))
	}

	loopBus, loopAccess = build()
	spanBus, spanAccess = build()
	_ = byteLoopRead(loopBus, uint64(spanTestBase), len(payload))
	spanBus.ReadSpan(uint64(spanTestBase), make([]byte, len(payload)))
	sameHistory(t, "ReadSpan", loopAccess.HistoryTail(0), spanAccess.HistoryTail(0))
}

// TestBusSpan_DisabledMatchesEnabled pins that the kill switch changes speed
// and nothing else.
func TestBusSpan_DisabledMatchesEnabled(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	payload := make([]byte, 0x400)
	for i := range payload {
		payload[i] = byte(rng.Intn(256))
	}

	run := func(enabled bool) ([]byte, []byte) {
		restore := setBusSpansEnabled(enabled)
		defer restore()
		bus, seam := spanTestBus()
		bus.WriteSpan(uint64(spanTestBase), payload)
		bus.WriteSpan(seam+0x200, payload)
		low := make([]byte, len(payload))
		high := make([]byte, len(payload))
		bus.ReadSpan(uint64(spanTestBase), low)
		bus.ReadSpan(seam+0x200, high)
		return low, high
	}

	offLow, offHigh := run(false)
	onLow, onHigh := run(true)
	if !bytes.Equal(offLow, onLow) || !bytes.Equal(offHigh, onHigh) {
		t.Fatal("IE_BUS_SPANS changed the data, not just the path")
	}
	if !bytes.Equal(onLow, payload) || !bytes.Equal(onHigh, payload) {
		t.Fatal("span round trip did not return the payload")
	}
}

// TestBusSpan_BulkPathActuallyTaken pins that the eligible cases really do take
// the bulk path. Without it the differentials above would still pass with the
// fast path permanently disabled, and the tranche would have measured nothing.
func TestBusSpan_BulkPathActuallyTaken(t *testing.T) {
	bus, seam := spanTestBus()
	bus.MapIO(spanTestBase+0x1000, spanTestBase+0x10FF, func(uint32) uint32 { return 0 }, func(uint32, uint32) {})

	if !bus.spanBulkEligible(uint64(spanTestBase), 0x800) {
		t.Fatal("a plain low-RAM span was refused the bulk path")
	}
	if !bus.spanBulkEligible(seam+0x100, 0x800) {
		t.Fatal("a plain backing span was refused the bulk path")
	}
	if bus.spanBulkEligible(uint64(spanTestBase)+0xF00, 0x400) {
		t.Fatal("a span overlapping MMIO was allowed the bulk path")
	}
	if bus.spanBulkEligible(seam-0x40, 0x80) {
		t.Fatal("a seam-crossing span was allowed the bulk path")
	}
	if bus.spanBulkEligible(bus.backing.Size()-0x20, 0x80) {
		t.Fatal("a span running past the backing was allowed the bulk path")
	}

	access := NewDebugAccessService()
	bus.SetDebugAccessService(access)
	access.Watch(-1, 0x7654321, 1, WatchWrite)
	if bus.spanBulkEligible(uint64(spanTestBase), 0x800) {
		t.Fatal("a span was allowed the bulk path with the debug service active")
	}
}
