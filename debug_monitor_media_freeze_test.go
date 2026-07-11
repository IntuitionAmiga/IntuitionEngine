package main

// debug_monitor_media_freeze_test.go - whole-machine freeze includes media.
//
// The SoundChip is the machine's only audio clock: the output backend pulls
// every sample through SoundChip.ReadSamples, and all players and audio
// engines advance from that tick graph. Freezing it therefore holds song
// positions and silences output in one step. These tests pin the monitor's
// contract: entering the monitor freezes the audio clock alongside the
// CPUs, leaving restores whatever state the machine had before entry, and
// an explicit fa/ta issued inside the session wins over the automatic
// restore.

import "testing"

func newMediaFreezeRig(t *testing.T) (*MachineMonitor, *SoundChip) {
	t.Helper()
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	mon.soundChip = chip
	return mon, chip
}

func TestMonitorActivateFreezesMachineAudio(t *testing.T) {
	mon, chip := newMediaFreezeRig(t)

	mon.Activate()
	if !chip.audioFrozen.Load() {
		t.Fatal("monitor activation did not freeze the audio clock")
	}
	mon.Deactivate()
	if chip.audioFrozen.Load() {
		t.Fatal("monitor deactivation did not resume the audio clock")
	}
}

func TestMonitorRestoresPreEntryAudioFreeze(t *testing.T) {
	mon, chip := newMediaFreezeRig(t)

	// A guest (or a previous session) froze audio through the control
	// register before the monitor was entered; leaving the monitor must
	// not thaw it behind their back.
	chip.audioFrozen.Store(true)
	mon.Activate()
	if !chip.audioFrozen.Load() {
		t.Fatal("audio clock should stay frozen across activation")
	}
	mon.Deactivate()
	if !chip.audioFrozen.Load() {
		t.Fatal("deactivation thawed an audio clock that was frozen before entry")
	}
}

func TestMonitorExitHonoursExplicitAudioCommand(t *testing.T) {
	mon, chip := newMediaFreezeRig(t)

	// Pre-entry state frozen; inside the session the user explicitly thaws
	// with ta. Their command outlives the session: the automatic restore
	// must not re-freeze on exit.
	chip.audioFrozen.Store(true)
	mon.Activate()
	mon.executeCommand("ta")
	if chip.audioFrozen.Load() {
		t.Fatal("ta did not thaw the audio clock")
	}
	mon.Deactivate()
	if chip.audioFrozen.Load() {
		t.Fatal("exit re-froze audio after an explicit ta in the session")
	}

	// The mirror case: pre-entry running, explicit fa inside the session,
	// exit must leave it frozen.
	mon2, chip2 := newMediaFreezeRig(t)
	mon2.Activate()
	mon2.executeCommand("fa")
	mon2.Deactivate()
	if !chip2.audioFrozen.Load() {
		t.Fatal("exit thawed audio after an explicit fa in the session")
	}
}

func TestMonitorBreakEventFreezesMachineAudio(t *testing.T) {
	mon, chip := newMediaFreezeRig(t)

	// A breakpoint-style entry goes through handleBreakpointHit rather
	// than Activate; it must freeze the audio clock all the same.
	mon.handleBreakpointHit(BreakpointEvent{CPUID: 0, Address: 0x1000})
	if !chip.audioFrozen.Load() {
		t.Fatal("break-event monitor entry did not freeze the audio clock")
	}
	mon.Deactivate()
	if chip.audioFrozen.Load() {
		t.Fatal("deactivation after break entry did not resume the audio clock")
	}
}

func TestMonitorOverlayExitResumesMachineAudio(t *testing.T) {
	mon, chip := newMediaFreezeRig(t)

	mon.Activate()
	if !chip.audioFrozen.Load() {
		t.Fatal("monitor activation did not freeze the audio clock")
	}
	// The overlay's Escape and command-exit paths share this helper.
	mon.exitFromOverlay()
	if chip.audioFrozen.Load() {
		t.Fatal("overlay exit did not resume the audio clock")
	}
	if mon.state != MonitorInactive {
		t.Fatal("overlay exit did not deactivate the monitor")
	}
}
