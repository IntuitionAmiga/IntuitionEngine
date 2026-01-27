// audio_sync_test.go - Unit tests for hard sync oscillator mode

package main

import (
	"testing"
)

const testSampleRate = float32(SAMPLE_RATE)

// TestHardSync_PhaseResetOnSourceWrap verifies that slave oscillators reset their
// phase when the master oscillator wraps (except for noise, which doesn't use phase).
func TestHardSync_PhaseResetOnSourceWrap(t *testing.T) {
	tests := []struct {
		name        string
		slaveWave   int
		expectReset bool
	}{
		{"square_resets", WAVE_SQUARE, true},
		{"triangle_resets", WAVE_TRIANGLE, true},
		{"sine_resets", WAVE_SINE, true},
		{"sawtooth_resets", WAVE_SAWTOOTH, true},
		{"noise_does_not_reset", WAVE_NOISE, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			master := &Channel{waveType: WAVE_SQUARE, phaseWrapped: true}
			slave := &Channel{
				waveType:   tc.slaveWave,
				phase:      3.14,
				syncSource: master,
				frequency:  440,
				noiseSR:    NOISE_LFSR_SEED,
			}
			slave.generateWaveSample(testSampleRate)
			if tc.expectReset && slave.phase > 0.1 {
				t.Errorf("expected phase reset, got %f", slave.phase)
			}
			if !tc.expectReset && slave.phase < 0.1 {
				t.Errorf("expected no reset for noise, got %f", slave.phase)
			}
		})
	}
}

// TestHardSync_PhaseWrappedFlag verifies that the phaseWrapped flag is correctly
// set when phase crosses 2*pi and cleared otherwise.
func TestHardSync_PhaseWrappedFlag(t *testing.T) {
	tests := []struct {
		name         string
		initialPhase float32
		frequency    float32
		expectWrap   bool
	}{
		{"under_2pi", 5.0, 440, false},
		{"crosses_2pi", 6.25, 440, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := &Channel{
				waveType:  WAVE_SINE,
				phase:     tc.initialPhase,
				frequency: tc.frequency,
			}
			ch.generateWaveSample(testSampleRate)
			if ch.phaseWrapped != tc.expectWrap {
				t.Errorf("phaseWrapped = %v, want %v", ch.phaseWrapped, tc.expectWrap)
			}
		})
	}
}

// TestHardSync_SelfSyncPrevented verifies that a channel cannot sync to itself
// via either the FLEX register path or the SYNC_SOURCE_CHx register path.
func TestHardSync_SelfSyncPrevented(t *testing.T) {
	chip := newTestSoundChip()

	// Via FLEX register - should prevent self-sync
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_OFF_SYNC, 0x80) // ch0 sync to ch0
	if chip.channels[0].syncSource != nil {
		t.Error("FLEX path should prevent self-sync")
	}

	// Via SYNC_SOURCE_CH0 - should also prevent self-sync
	chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0) // ch0 sync to ch0
	if chip.channels[0].syncSource != nil {
		t.Error("SYNC_SOURCE path should prevent self-sync")
	}

	// Verify that non-self sync still works via SYNC_SOURCE
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0) // ch1 sync to ch0
	if chip.channels[1].syncSource != chip.channels[0] {
		t.Error("SYNC_SOURCE should allow syncing to different channel")
	}
}

// TestHardSync_SyncChain verifies that sync chains work correctly:
// when A wraps, B should reset; when B wraps, C should reset.
func TestHardSync_SyncChain(t *testing.T) {
	chA := &Channel{waveType: WAVE_SQUARE, frequency: 110, dutyCycle: 0.5}
	chB := &Channel{waveType: WAVE_TRIANGLE, frequency: 220, syncSource: chA}
	chC := &Channel{waveType: WAVE_SINE, frequency: 440, syncSource: chB}

	// Advance chA until it wraps
	for i := 0; i < 1000 && !chA.phaseWrapped; i++ {
		chA.generateWaveSample(testSampleRate)
	}
	if !chA.phaseWrapped {
		t.Fatal("chA should have wrapped after 1000 samples")
	}

	chB.phase = 3.0 // Set non-zero phase
	chB.generateWaveSample(testSampleRate)

	if chB.phase > 0.2 {
		t.Errorf("chB should reset when chA wraps, phase = %f", chB.phase)
	}

	// Now advance chB until it wraps
	chA.phaseWrapped = false // Clear master flag
	for i := 0; i < 1000 && !chB.phaseWrapped; i++ {
		chB.generateWaveSample(testSampleRate)
	}
	if !chB.phaseWrapped {
		t.Fatal("chB should have wrapped after 1000 samples")
	}

	chC.phase = 3.0 // Set non-zero phase
	chC.generateWaveSample(testSampleRate)

	if chC.phase > 0.2 {
		t.Errorf("chC should reset when chB wraps, phase = %f", chC.phase)
	}
}

// TestHardSync_DisableSync verifies that sync can be disabled by clearing bit 7.
func TestHardSync_DisableSync(t *testing.T) {
	chip := newTestSoundChip()

	// Enable sync via FLEX register
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_CH_STRIDE+FLEX_OFF_SYNC, 0x80) // ch1 sync to ch0
	if chip.channels[1].syncSource == nil {
		t.Fatal("sync should be enabled")
	}

	// Disable sync (bit 7 = 0)
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_CH_STRIDE+FLEX_OFF_SYNC, 0x00)
	if chip.channels[1].syncSource != nil {
		t.Error("sync should be disabled when bit 7 is 0")
	}
}

// TestHardSync_AllChannels verifies that sync works for all channel combinations.
func TestHardSync_AllChannels(t *testing.T) {
	chip := newTestSoundChip()

	// Test each channel syncing to a different channel
	for slave := 0; slave < NUM_CHANNELS; slave++ {
		for master := 0; master < NUM_CHANNELS; master++ {
			if slave == master {
				continue // Skip self-sync
			}
			// Clear previous sync
			chip.channels[slave].syncSource = nil

			// Set up sync via FLEX
			addr := FLEX_CH_BASE + uint32(slave)*FLEX_CH_STRIDE + FLEX_OFF_SYNC
			chip.HandleRegisterWrite(addr, 0x80|uint32(master))

			if chip.channels[slave].syncSource != chip.channels[master] {
				t.Errorf("ch%d should sync to ch%d", slave, master)
			}
		}
	}
}

// TestHardSync_RegisterPaths verifies both register paths work identically.
func TestHardSync_RegisterPaths(t *testing.T) {
	chip := newTestSoundChip()

	// Test FLEX path: ch1 syncs to ch0
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_CH_STRIDE+FLEX_OFF_SYNC, 0x80)
	if chip.channels[1].syncSource != chip.channels[0] {
		t.Error("FLEX path failed to set sync source")
	}

	// Clear and test SYNC_SOURCE path
	chip.channels[1].syncSource = nil
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
	if chip.channels[1].syncSource != chip.channels[0] {
		t.Error("SYNC_SOURCE path failed to set sync source")
	}
}
