// machine_bus_alloc_unix.go - PLAN_MAX_RAM slice 10 reviewer P1 fix.
//
// On Linux and darwin the default bus.memory allocator uses an anonymous
// private mmap. Pages are demand-faulted, so a guest RAM budget of e.g.
// 4 GiB does not eagerly commit 4 GiB of host RSS at boot — it commits
// only the pages actually touched by guest code, ROMs and DMA. This
// matches the appliance use case (most of host RAM advertised, only the
// working set resident) without requiring the caller to plumb a high-
// range Backing through the Bus32 path.
//
// Small allocations (below the mmap threshold) fall through to a Go-heap
// make() so test rigs that allocate dozens of small buses do not pay
// per-mapping syscall overhead.

//go:build linux || darwin

package main

import "golang.org/x/sys/unix"

// busMemMmapThreshold picks the size above which the production
// allocator switches from a Go-heap make() to an anonymous mmap.
// Sized at 64 MiB: well above any banked or EmuTOS profile (so those
// stay on the Go heap), well below any IE64-family appliance allocation
// (so those land on lazy mmap).
const busMemMmapThreshold uint64 = 64 * 1024 * 1024

// busMemBootClamp is the boot-time upper bound on memSize the appliance
// will request from NewMachineBusSized. On mmap-capable platforms it
// equals busMemMaxBytes — large allocations are lazy-paged so there is
// no benefit to a smaller cap.
const busMemBootClamp uint64 = busMemMaxBytes

func defaultBusMemAllocator(size uint64) []byte {
	if size < busMemMmapThreshold {
		return make([]byte, size)
	}
	mem, err := unix.Mmap(-1, 0, int(size),
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_ANON|unix.MAP_PRIVATE)
	if err != nil {
		// PLAN_MAX_RAM slice 10 reviewer P2: do NOT fall back to a Go-
		// heap make() at this size. A multi-hundred-MiB heap slice would
		// reintroduce the eager-commit regression mmap was chosen to
		// avoid. Returning nil triggers an InvalidArg-class error from
		// NewMachineBusSized; the caller (boot path) must fall back to
		// a smaller bus and high-range Backing.
		return nil
	}
	return mem
}

// allocateBusMemory wraps the allocator to also produce a reset closure
// suited to the underlying allocation strategy. Mmap-backed slices use
// madvise(MADV_DONTNEED) so a guest reset releases pages instead of
// touching every byte (which would eagerly commit a multi-GiB region).
// Heap-backed slices fall through to a plain byte-loop zero.
func allocateBusMemory(size uint64, allocator func(size uint64) []byte) ([]byte, func()) {
	mem := allocator(size)
	if size >= busMemMmapThreshold && uint64(len(mem)) == size {
		return mem, func() {
			if len(mem) == 0 {
				return
			}
			if err := unix.Madvise(mem, busMemMadviseDiscardFlag); err != nil {
				// madvise rejection (e.g. mem came from heap fallback):
				// fall through to byte-loop. Cheap on small mappings.
				for i := range mem {
					mem[i] = 0
				}
			}
		}
	}
	return mem, nil
}
