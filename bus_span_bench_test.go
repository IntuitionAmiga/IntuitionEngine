// bus_span_bench_test.go - measurement benchmarks for bulk span transfers.
//
// Tranche 3 item 10 is gated on numbers, not on the observation that per-byte
// loops exist. Each benchmark below pairs the byte loop a production path
// actually runs with the bulk equivalent that a span API would give it, at the
// size that path actually moves, so the decision gate ("the byte loop is worth
// removing where it is more than roughly 5% of the operation") can be applied
// with a measurement rather than a guess.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"testing"
)

// spanBenchSizes are the transfer sizes the production paths move: a directory
// listing, a GEMDOS chunk, a texture or sound buffer, and a whole file or ROM
// image.
var spanBenchSizes = []int{4096, 64 << 10, 1 << 20}

func spanBenchName(size int) string {
	switch {
	case size >= 1<<20:
		return "1MiB"
	case size >= 64<<10:
		return "64KiB"
	default:
		return "4KiB"
	}
}

// benchLowRAMBus returns a bus with enough low RAM for the largest span, and no
// MMIO mapped over the region under test.
func benchLowRAMBus() *MachineBus {
	return NewMachineBus()
}

const spanBenchBase uint32 = 0x200000

// BenchmarkFileIOBulkRead_ByteLoop measures FileIODevice.writeReadResult's
// staging loop (file_io.go:443) against the bulk copy a span API would do.
// The comparison is deliberately loop-versus-copy with no host file read in
// either arm: including the os.ReadFile would hide the very ratio the decision
// gate needs.
func BenchmarkFileIOBulkRead_ByteLoop(b *testing.B) {
	for _, size := range spanBenchSizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i)
		}
		b.Run(spanBenchName(size)+"/ByteLoop", func(b *testing.B) {
			bus := benchLowRAMBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				for i, v := range data {
					bus.Write8(spanBenchBase+uint32(i), v)
				}
			}
		})
		b.Run(spanBenchName(size)+"/Span", func(b *testing.B) {
			bus := benchLowRAMBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				if err := bus.WritePhysRAMOnly(uint64(spanBenchBase), data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTextureUpload_ByteLoop measures reading guest RAM out to a host
// buffer, the shape the ANTIC display fetch and the blitter fallbacks use.
func BenchmarkTextureUpload_ByteLoop(b *testing.B) {
	for _, size := range spanBenchSizes {
		dst := make([]byte, size)
		b.Run(spanBenchName(size)+"/ByteLoop", func(b *testing.B) {
			bus := benchLowRAMBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				for i := range dst {
					dst[i] = bus.Read8(spanBenchBase + uint32(i))
				}
			}
		})
		b.Run(spanBenchName(size)+"/Span", func(b *testing.B) {
			bus := benchLowRAMBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				copy(dst, bus.GetMemory()[spanBenchBase:spanBenchBase+uint32(size)])
			}
		})
	}
}

// benchSparseBus returns a bus whose high addresses route to a sparse backing,
// which is where WritePhysRAMOnly still degrades to per-byte dispatch
// (machine_bus_phys.go:143).
func benchSparseBus() (*MachineBus, uint64) {
	bus := NewMachineBus()
	backing := NewSparseBacking(64 << 20)
	bus.SetBacking(backing)
	// The backing is addressed from zero and only owns addresses at or above
	// the end of bus.memory, so the base must clear the 32 MiB legacy RAM and
	// still leave room for the largest span below the advertised 64 MiB.
	return bus, 40 << 20
}

// BenchmarkSparseBacking_PerWordDispatch measures the backing dispatch itself:
// one interface call and one sparse page lookup per byte, against the backing's
// own span entry points.
func BenchmarkSparseBacking_PerWordDispatch(b *testing.B) {
	for _, size := range spanBenchSizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i)
		}
		b.Run(spanBenchName(size)+"/Write8Loop", func(b *testing.B) {
			bus, base := benchSparseBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				for i, v := range data {
					bus.backing.Write8(base+uint64(i), v)
				}
			}
		})
		b.Run(spanBenchName(size)+"/WriteBytes", func(b *testing.B) {
			bus, base := benchSparseBus()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				bus.backing.WriteBytes(base, data)
			}
		})
		b.Run(spanBenchName(size)+"/Read8Loop", func(b *testing.B) {
			bus, base := benchSparseBus()
			dst := make([]byte, size)
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				for i := range dst {
					dst[i] = bus.backing.Read8(base + uint64(i))
				}
			}
		})
		b.Run(spanBenchName(size)+"/ReadBytes", func(b *testing.B) {
			bus, base := benchSparseBus()
			dst := make([]byte, size)
			b.SetBytes(int64(size))
			b.ResetTimer()
			for range b.N {
				bus.backing.ReadBytes(base, dst)
			}
		})
	}
}

// BenchmarkDMACopy_ByteLoop measures the AROS audio DMA sample fetch
// (aros_audio_dma.go:190), which reads one byte per channel per sample through
// ReadPhys8 when the buffer is above low memory. The span arm models caching
// the channel buffer once per refill instead.
func BenchmarkDMACopy_ByteLoop(b *testing.B) {
	const samples = 4410 // one tenth of a second, four channels' worth per tick
	b.Run("ReadPhys8", func(b *testing.B) {
		bus, base := benchSparseBus()
		b.SetBytes(int64(samples))
		b.ResetTimer()
		for range b.N {
			var acc byte
			for i := range samples {
				acc += bus.ReadPhys8(base + uint64(i))
			}
			_ = acc
		}
	})
	b.Run("Span", func(b *testing.B) {
		bus, base := benchSparseBus()
		buf := make([]byte, samples)
		b.SetBytes(int64(samples))
		b.ResetTimer()
		for range b.N {
			bus.backing.ReadBytes(base, buf)
			var acc byte
			for _, v := range buf {
				acc += v
			}
			_ = acc
		}
	})
}
