package main

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestP65JITGenerationServiceCoversBusWriteWidths(t *testing.T) {
	bus := NewMachineBus()
	var mu sync.Mutex
	pages := make(map[uint64]int)
	unregister := bus.RegisterP65JITInvalidator(func(addr, size uint64) {
		mu.Lock()
		defer mu.Unlock()
		for page := addr >> 8; page <= (addr+size-1)>>8; page++ {
			pages[page]++
		}
	})
	defer unregister()

	bus.Write8(0x0600, 1)
	bus.Write16(0x06FF, 2) // crosses $06/$07
	bus.Write32(0x07FE, 3) // crosses $07/$08
	bus.Write64(0x08FC, 4) // crosses $08/$09
	if !bus.Write8WithFault(0x0A00, 5) {
		t.Fatal("Write8WithFault rejected ordinary RAM")
	}
	if !bus.Write16WithFault(0x0AFF, 6) { // crosses $0A/$0B
		t.Fatal("Write16WithFault rejected ordinary RAM")
	}
	if !bus.Write32WithFault(0x0BFE, 7) { // crosses $0B/$0C
		t.Fatal("Write32WithFault rejected ordinary RAM")
	}
	if !bus.Write64WithFault(0x0CFC, 8) { // crosses $0C/$0D
		t.Fatal("Write64WithFault rejected ordinary RAM")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, page := range []uint64{0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D} {
		if pages[page] == 0 {
			t.Errorf("page $%02X received no generation publication", page)
		}
	}
}

func TestP65JITGenerationServiceCoversFastInterpreterPlainRAMWrites(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	adapter := cpu.memory.(*Bus6502Adapter)

	var mu sync.Mutex
	pages := make(map[uint64]int)
	unregister := bus.RegisterP65JITInvalidator(func(addr, size uint64) {
		mu.Lock()
		defer mu.Unlock()
		for page := addr >> 8; page <= (addr+size-1)>>8; page++ {
			pages[page]++
		}
	})
	defer unregister()

	adapter.WriteFast(0x0600, 0xEA)
	adapter.WriteZP(0x80, 0x42)
	adapter.WriteStack(0xFE, 0x99)

	mu.Lock()
	defer mu.Unlock()
	for _, page := range []uint64{0x00, 0x01, 0x06} {
		if pages[page] == 0 {
			t.Errorf("fast interpreter write did not publish code generation for page $%02X", page)
		}
	}
}

func TestP65JITGenerationServiceCoversExecuteFastDirectStores(t *testing.T) {
	bus := NewMachineBus()
	// LDA #$EA; STA $0010; STA $0700; PHA; JAM.
	for offset, value := range []byte{0xA9, 0xEA, 0x85, 0x10, 0x8D, 0x00, 0x07, 0x48, 0x02} {
		bus.Write8(0x0600+uint32(offset), value)
	}
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)

	var mu sync.Mutex
	pages := make(map[uint64]int)
	unregister := bus.RegisterP65JITInvalidator(func(addr, size uint64) {
		mu.Lock()
		defer mu.Unlock()
		for page := addr >> 8; page <= (addr+size-1)>>8; page++ {
			pages[page]++
		}
	})
	defer unregister()
	cpu.ExecuteFast()

	mu.Lock()
	defer mu.Unlock()
	for _, page := range []uint64{0x00, 0x01, 0x07} {
		if pages[page] == 0 {
			t.Errorf("ExecuteFast direct store did not publish code generation for page $%02X", page)
		}
	}
}

func TestP65JITGenerationServiceAuditsFastInterpreterDirectWrites(t *testing.T) {
	source, err := os.ReadFile("cpu_six5go2_fast.go")
	if err != nil {
		t.Fatalf("read fast interpreter: %v", err)
	}
	const allowed = "cpu.fastAdapter.memDirect[addr] = val"
	for lineNo, line := range strings.Split(string(source), "\n") {
		if !strings.Contains(line, "memDirect[") || !strings.Contains(line, "=") || strings.Contains(line, "==") {
			continue
		}
		if strings.TrimSpace(line) != allowed {
			t.Fatalf("raw direct-memory write at cpu_six5go2_fast.go:%d: %s; route it through fastWriteDirect", lineNo+1, strings.TrimSpace(line))
		}
	}
}

func TestP65JITGenerationServiceUnregisters(t *testing.T) {
	bus := NewMachineBus()
	var calls int
	unregister := bus.RegisterP65JITInvalidator(func(uint64, uint64) { calls++ })
	bus.Write8(0x0600, 1)
	unregister()
	unregister() // idempotent
	bus.Write8(0x0600, 2)
	if calls != 1 {
		t.Fatalf("callback calls=%d, want 1", calls)
	}
}

func TestP65JITGenerationServiceCoversFileIOSparseBackingWrite(t *testing.T) {
	bus := NewMachineBus()
	base := uint64(len(bus.GetMemory()))
	bus.SetBacking(NewSparseBacking(base + 0x2000))
	var gotAddr, gotSize uint64
	unregister := bus.RegisterP65JITInvalidator(func(addr, size uint64) {
		gotAddr, gotSize = addr, size
	})
	defer unregister()

	file := &FileIODevice{bus: bus}
	addr := base + 0x321
	if !file.writeGuest8(addr, 0xA5) {
		t.Fatal("file I/O sparse-backing write failed")
	}
	if gotAddr != addr || gotSize != 1 {
		t.Fatalf("generation publication=(%#x,%d), want=(%#x,1)", gotAddr, gotSize, addr)
	}
}

func TestP65JITGenerationServiceCoversBusReset(t *testing.T) {
	bus := NewMachineBus()
	lowSize := uint64(len(bus.GetMemory()))
	bus.SetBacking(NewSparseBacking(lowSize + 0x1000))
	type publication struct{ addr, size uint64 }
	var writes []publication
	unregister := bus.RegisterP65JITInvalidator(func(addr, size uint64) {
		writes = append(writes, publication{addr, size})
	})
	defer unregister()

	bus.Reset()
	if len(writes) != 2 {
		t.Fatalf("reset publications=%v, want low RAM and sparse backing", writes)
	}
	if writes[0] != (publication{0, lowSize}) || writes[1] != (publication{lowSize, 0x1000}) {
		t.Fatalf("reset publications=%v, want [{0 %#x} {%#x 0x1000}]", writes, lowSize, lowSize)
	}
}
