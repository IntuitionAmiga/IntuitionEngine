// ahx_player_test.go - AHX player surface tests.

package main

import (
	"testing"
	"time"
)

type ahxSparseTestBus struct {
	mem   []byte
	data  map[uint32]byte
	reads int
}

func (b *ahxSparseTestBus) Read8(addr uint32) uint8 {
	b.reads++
	return b.data[addr]
}

func (b *ahxSparseTestBus) Write8(addr uint32, value uint8) {
	b.data[addr] = byte(value)
}

func (b *ahxSparseTestBus) Read16(addr uint32) uint16 {
	return uint16(b.Read8(addr)) | uint16(b.Read8(addr+1))<<8
}

func (b *ahxSparseTestBus) Write16(addr uint32, value uint16) {
	b.Write8(addr, uint8(value))
	b.Write8(addr+1, uint8(value>>8))
}

func (b *ahxSparseTestBus) Read32(addr uint32) uint32 {
	return uint32(b.Read16(addr)) | uint32(b.Read16(addr+2))<<16
}

func (b *ahxSparseTestBus) Write32(addr uint32, value uint32) {
	b.Write16(addr, uint16(value))
	b.Write16(addr+2, uint16(value>>16))
}

func (b *ahxSparseTestBus) Reset() {}

func (b *ahxSparseTestBus) GetMemory() []byte {
	return b.mem
}

func TestAHX_PlayerBusSafeRead(t *testing.T) {
	data := buildAHXModule(ahxModuleOptions{})
	const loadAddr = uint32(0x2000)
	bus := &ahxSparseTestBus{
		mem:  make([]byte, int(loadAddr)+len(data)),
		data: make(map[uint32]byte),
	}
	for i, b := range data {
		bus.data[loadAddr+uint32(i)] = b
		bus.mem[int(loadAddr)+i] = b
	}

	player := NewAHXPlayer(newTestSoundChip(), 44100)
	player.AttachBus(bus)
	player.HandlePlayWrite(AHX_PLAY_PTR, loadAddr)
	player.HandlePlayWrite(AHX_PLAY_LEN, uint32(len(data)))
	player.HandlePlayWrite(AHX_PLAY_CTRL, 1)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if player.IsPlaying() {
			return
		}
		if player.HandlePlayRead(AHX_PLAY_STATUS)&0x2 != 0 {
			t.Fatal("AHX player reported error while reading via Bus32.Read8")
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("AHX player did not start")
}

type ahxReentrantReadBus struct {
	mem    []byte
	player *AHXPlayer
	reads  int
}

func (b *ahxReentrantReadBus) Read8(addr uint32) uint8 {
	b.reads++
	_ = b.player.HandlePlayRead(AHX_PLAY_STATUS)
	return 0
}

func (b *ahxReentrantReadBus) Write8(addr uint32, value uint8) {}

func (b *ahxReentrantReadBus) Read16(addr uint32) uint16 { return uint16(b.Read8(addr)) }

func (b *ahxReentrantReadBus) Write16(addr uint32, value uint16) {}

func (b *ahxReentrantReadBus) Read32(addr uint32) uint32 { return uint32(b.Read16(addr)) }

func (b *ahxReentrantReadBus) Write32(addr uint32, value uint32) {}

func (b *ahxReentrantReadBus) Reset() {}

func (b *ahxReentrantReadBus) GetMemory() []byte { return b.mem }

func TestAHX_PlayerMMIOPointerDoesNotDeadlock(t *testing.T) {
	player := NewAHXPlayer(newTestSoundChip(), 44100)
	bus := &ahxReentrantReadBus{mem: make([]byte, int(AHX_PLAY_STATUS)+14), player: player}
	player.AttachBus(bus)
	player.HandlePlayWrite(AHX_PLAY_PTR, AHX_PLAY_STATUS)
	player.HandlePlayWrite(AHX_PLAY_LEN, 14)

	done := make(chan struct{})
	go func() {
		player.HandlePlayWrite(AHX_PLAY_CTRL, 1)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("HandlePlayWrite deadlocked while AHX bus read re-entered HandlePlayRead")
	}
	if bus.reads != 0 {
		t.Fatalf("expected bulk helper to avoid reentrant Read8, got %d reads", bus.reads)
	}
}

func TestAHX_PlayerRejectsOversizedPlayLenBeforeRead(t *testing.T) {
	bus := &ahxSparseTestBus{
		mem:  make([]byte, 16),
		data: make(map[uint32]byte),
	}
	player := NewAHXPlayer(newTestSoundChip(), 44100)
	player.AttachBus(bus)
	player.HandlePlayWrite(AHX_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(AHX_PLAY_LEN, ahxMaxPlayLen+1)
	player.HandlePlayWrite(AHX_PLAY_CTRL, 1)

	if got := player.HandlePlayRead(AHX_PLAY_STATUS); got&0x2 == 0 {
		t.Fatalf("AHX_PLAY_STATUS = 0x%X, want error bit for oversized play length", got)
	}
	if bus.reads != 0 {
		t.Fatalf("Read8 called %d times, want oversized request rejected before bus reads", bus.reads)
	}
	if player.IsPlaying() {
		t.Fatal("player should not start for oversized play length")
	}
}

func TestAHX_PlayerRejectsWrappedPlayRangeBeforeRead(t *testing.T) {
	bus := &ahxSparseTestBus{
		mem:  make([]byte, 16),
		data: make(map[uint32]byte),
	}
	player := NewAHXPlayer(newTestSoundChip(), 44100)
	player.AttachBus(bus)
	player.HandlePlayWrite(AHX_PLAY_PTR, 0xfffffff0)
	player.HandlePlayWrite(AHX_PLAY_LEN, 0x20)
	player.HandlePlayWrite(AHX_PLAY_CTRL, 1)

	if got := player.HandlePlayRead(AHX_PLAY_STATUS); got&0x2 == 0 {
		t.Fatalf("AHX_PLAY_STATUS = 0x%X, want error bit for wrapped play range", got)
	}
	if bus.reads != 0 {
		t.Fatalf("Read8 called %d times, want wrapped request rejected before bus reads", bus.reads)
	}
}

func TestAHX_GetSubsongCount(t *testing.T) {
	player := NewAHXPlayer(newTestSoundChip(), 44100)
	data := buildAHXModule(ahxModuleOptions{
		PositionNr: 2,
		Subsongs:   []int{1, 0},
	})
	if err := player.Load(data); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := player.GetSubsongCount(); got != 3 {
		t.Fatalf("GetSubsongCount = %d, want main song plus 2 subsongs", got)
	}
}
