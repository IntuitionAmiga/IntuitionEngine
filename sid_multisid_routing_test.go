package main

import "testing"

func newSIDRoutingTestSoundChip() *SoundChip {
	chip := &SoundChip{}
	for i := range NUM_CHANNELS {
		chip.channels[i] = &Channel{}
	}
	return chip
}

func TestSIDEngineMultiSIDRoutesPerVoiceStateToBaseChannel(t *testing.T) {
	tests := []struct {
		name        string
		baseChannel int
		regBase     uint32
		regEnd      uint32
	}{
		{name: "SID2", baseChannel: 4, regBase: SID2_BASE, regEnd: SID2_END},
		{name: "SID3", baseChannel: 7, regBase: SID3_BASE, regEnd: SID3_END},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chip := newSIDRoutingTestSoundChip()
			engine := NewSIDEngineMulti(chip, 44100, tt.baseChannel, tt.regBase, tt.regEnd)

			engine.WriteRegister(0x04, SID_CTRL_TRIANGLE|SID_CTRL_GATE|SID_CTRL_TEST)
			engine.WriteRegister(0x05, 0xA5)
			engine.WriteRegister(0x06, 0xF7)
			engine.WriteRegister(0x17, SID_FILT_V1|0xF0)
			engine.WriteRegister(0x18, SID_MODE_LP|0x0F)

			target := chip.channels[tt.baseChannel]
			if target.sidWaveMask != SID_CTRL_TRIANGLE {
				t.Fatalf("channel %d sidWaveMask=0x%02X, want 0x%02X", tt.baseChannel, target.sidWaveMask, SID_CTRL_TRIANGLE)
			}
			if !target.sidTestBit {
				t.Fatalf("channel %d sidTest=false, want true", tt.baseChannel)
			}
			if target.sidAttackIndex != 0x0A || target.sidDecayIndex != 0x05 || target.sidReleaseIndex != 0x07 {
				t.Fatalf("channel %d ADSR indices=(%d,%d,%d), want (10,5,7)",
					tt.baseChannel, target.sidAttackIndex, target.sidDecayIndex, target.sidReleaseIndex)
			}
			if target.filterModeMask != 0x01 {
				t.Fatalf("channel %d filterModeMask=0x%02X, want 0x01", tt.baseChannel, target.filterModeMask)
			}

			if chip.channels[0].sidWaveMask != 0 || chip.channels[0].sidTestBit || chip.channels[0].sidAttackIndex != 0 || chip.channels[0].filterModeMask != 0 {
				t.Fatalf("SID1 channel 0 was modified by %s register writes", tt.name)
			}
		})
	}
}

func TestSIDEngineMultiSIDRoutesModulationSourcesToSameBank(t *testing.T) {
	tests := []struct {
		name        string
		baseChannel int
		regBase     uint32
		regEnd      uint32
	}{
		{name: "SID2", baseChannel: 4, regBase: SID2_BASE, regEnd: SID2_END},
		{name: "SID3", baseChannel: 7, regBase: SID3_BASE, regEnd: SID3_END},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chip := newSIDRoutingTestSoundChip()
			engine := NewSIDEngineMulti(chip, 44100, tt.baseChannel, tt.regBase, tt.regEnd)

			engine.WriteRegister(0x04, SID_CTRL_TRIANGLE|SID_CTRL_RINGMOD|SID_CTRL_SYNC|SID_CTRL_GATE)

			target := chip.channels[tt.baseChannel]
			source := chip.channels[tt.baseChannel+2]
			if target.ringModSource != source {
				t.Fatalf("channel %d ringModSource=%p, want channel %d %p", tt.baseChannel, target.ringModSource, tt.baseChannel+2, source)
			}
			if target.syncSource != source {
				t.Fatalf("channel %d syncSource=%p, want channel %d %p", tt.baseChannel, target.syncSource, tt.baseChannel+2, source)
			}
			if chip.channels[0].ringModSource != nil || chip.channels[0].syncSource != nil {
				t.Fatalf("SID1 channel 0 modulation was modified by %s register writes", tt.name)
			}
		})
	}
}
