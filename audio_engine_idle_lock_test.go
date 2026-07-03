package main

import (
	"testing"
	"time"
)

func requireTickSampleWithoutLocking(t *testing.T, lock func(), unlock func(), tick func()) {
	t.Helper()

	lock()
	done := make(chan struct{})
	go func() {
		tick()
		close(done)
	}()

	select {
	case <-done:
		unlock()
	case <-time.After(250 * time.Millisecond):
		unlock()
		<-done
		t.Fatal("idle TickSample blocked on engine mutex")
	}
}

func TestSIDTickSample_IdleNoLock(t *testing.T) {
	engine := NewSIDEngine(nil, SAMPLE_RATE)
	engine.enabled.Store(true)
	requireTickSampleWithoutLocking(t, engine.mutex.Lock, engine.mutex.Unlock, engine.TickSample)
}

func TestTEDTickSample_IdleNoLock(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	engine.enabled.Store(true)
	requireTickSampleWithoutLocking(t, engine.mutex.Lock, engine.mutex.Unlock, engine.TickSample)
}

func TestMIDITickSample_IdleNoLock(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	requireTickSampleWithoutLocking(t, engine.mu.Lock, engine.mu.Unlock, engine.TickSample)
}

func TestPSGTickSample_IdleNoLock(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	requireTickSampleWithoutLocking(t, engine.mutex.Lock, engine.mutex.Unlock, engine.TickSample)
}

func TestPOKEYTickSample_IdleNoLock(t *testing.T) {
	engine := NewPOKEYEngine(nil, SAMPLE_RATE)
	requireTickSampleWithoutLocking(t, engine.mutex.Lock, engine.mutex.Unlock, engine.TickSample)
}

func TestAHXTickSample_IdleNoLock(t *testing.T) {
	engine := NewAHXEngine(nil, SAMPLE_RATE)
	requireTickSampleWithoutLocking(t, engine.mutex.Lock, engine.mutex.Unlock, engine.TickSample)
}

func TestMODTickSample_IdleNoLock(t *testing.T) {
	engine := NewMODEngine(nil, SAMPLE_RATE)
	requireTickSampleWithoutLocking(t, engine.mu.Lock, engine.mu.Unlock, engine.TickSample)
}

func TestArosAudioDMATickSample_IdleNoLock(t *testing.T) {
	bus, err := NewMachineBusSized(64 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetBacking(NewSparseBacking(uint64(AROS_PROFILE_TOP)))
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    uint64(AROS_PROFILE_TOP),
		ActiveVisibleRAM: uint64(AROS_PROFILE_TOP),
	})
	dma, err := NewArosAudioDMA(bus, nil, nil)
	if err != nil {
		t.Fatalf("NewArosAudioDMA: %v", err)
	}
	requireTickSampleWithoutLocking(t, dma.mu.Lock, dma.mu.Unlock, dma.TickSample)
}

func TestSFXTriggerTickSample_IdleNoLock(t *testing.T) {
	trigger := NewSFXTrigger()
	requireTickSampleWithoutLocking(t, trigger.channels[0].mu.Lock, trigger.channels[0].mu.Unlock, trigger.TickSample)
}

func TestSIDDebugRestoreSnapshotRestoresPlayingGate(t *testing.T) {
	engine := NewSIDEngine(nil, SAMPLE_RATE)
	engine.enabled.Store(true)
	engine.SetPlaying(true)

	version, data, err := engine.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	engine.SetPlaying(false)
	if engine.playingActive.Load() {
		t.Fatal("SetPlaying(false) left playingActive set")
	}
	if err := engine.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	if !engine.playingActive.Load() {
		t.Fatal("DebugRestoreSnapshot did not restore playingActive")
	}
}

func TestTEDDebugRestoreSnapshotRestoresPlayingGate(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	engine.enabled.Store(true)
	engine.SetPlaying(true)

	version, data, err := engine.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	engine.SetPlaying(false)
	if engine.playingActive.Load() {
		t.Fatal("SetPlaying(false) left playingActive set")
	}
	if err := engine.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	if !engine.playingActive.Load() {
		t.Fatal("DebugRestoreSnapshot did not restore playingActive")
	}
}
