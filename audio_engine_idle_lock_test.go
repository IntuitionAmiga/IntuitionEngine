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
