package main

import "testing"

func newTestSN(t *testing.T) (*SoundChip, *SN76489Chip) {
	t.Helper()
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	return sound, NewSN76489Chip(sound)
}

func snAtten(c *SN76489Chip, ch int) uint8    { return c.atten[ch] }
func snDivider(c *SN76489Chip, ch int) uint16 { return c.tone[ch] }
func snLastWritten(c *SN76489Chip) uint8      { return c.lastWritten }
func snWriteCount(c *SN76489Chip) uint64      { return c.writeCount }

func TestSN_BusWrite_ToneLatch(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0x80)
	sn.HandleWrite8(SN_PORT_WRITE, 0x01)

	if got := snDivider(sn, 0); got != 0x10 {
		t.Fatalf("divider: got 0x%03X, want 0x010", got)
	}
}

func TestSN_BusWrite_Attenuation(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0x90)

	if got := snAtten(sn, 0); got != 0 {
		t.Fatalf("atten: got %d, want 0", got)
	}
}

func TestSN_BusWrite_NoiseControl(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0xE7)

	if sn.noiseReg != 7 {
		t.Fatalf("noise reg: got %d, want 7", sn.noiseReg)
	}
}

func TestSN_NoiseDataByteIgnoredAfterNoiseLatch(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0xE4)
	sn.lfsr = 0x1234
	sn.HandleWrite8(SN_PORT_WRITE, 0x03)

	if sn.noiseReg != 4 {
		t.Fatalf("noise reg changed after data byte: got %d, want 4", sn.noiseReg)
	}
	if sn.lfsr != 0x1234 {
		t.Fatalf("LFSR reset after ignored data byte: got 0x%X want 0x1234", sn.lfsr)
	}
}

func TestSN_NoiseControlResetsAudibleLFSR(t *testing.T) {
	sound, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0xE4)
	sound.snVoices[3].noiseSR = 0x1234

	sn.HandleWrite8(SN_PORT_WRITE, 0xE0)

	if sound.snVoices[3].noiseSR != sn.lfsrSeed() {
		t.Fatalf("audible LFSR: got 0x%X want 0x%X", sound.snVoices[3].noiseSR, sn.lfsrSeed())
	}
}

func TestSN_ModeWriteResetsAudibleLFSR(t *testing.T) {
	sound, sn := newTestSN(t)
	sound.snVoices[3].noiseSR = 0x1234

	sn.HandleWrite8(SN_PORT_MODE, SN76489_MODE_LFSR_16)

	if sound.snVoices[3].noiseSR != SN16_NOISE_LFSR_MASK {
		t.Fatalf("audible LFSR after mode write: got 0x%X want 0x%X", sound.snVoices[3].noiseSR, SN16_NOISE_LFSR_MASK)
	}
}

func TestSN_AttenuationEnablesVoiceInstantly(t *testing.T) {
	sound, sn := newTestSN(t)

	sn.HandleWrite8(SN_PORT_WRITE, 0x90)

	voice := &sound.snVoices[0]
	if voice.envelopePhase != ENV_SUSTAIN || voice.envelopeLevel != MAX_LEVEL || voice.sustainLevel != MAX_LEVEL {
		t.Fatalf("SN voice envelope not instant: phase=%d level=%f sustain=%f", voice.envelopePhase, voice.envelopeLevel, voice.sustainLevel)
	}
}

func TestSN_DividerZeroIsMaxPeriod(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0x80)
	sn.HandleWrite8(SN_PORT_WRITE, 0x00)

	if got := sn.effectiveDivider(0); got != 1024 {
		t.Fatalf("effective divider: got %d, want 1024", got)
	}
}

func TestSN_PeriodicNoiseDistinct(t *testing.T) {
	_, white := newTestSN(t)
	_, periodic := newTestSN(t)
	white.HandleWrite8(SN_PORT_WRITE, 0xE4)
	periodic.HandleWrite8(SN_PORT_WRITE, 0xE0)

	for range 8 {
		white.clockNoise()
		periodic.clockNoise()
	}

	if white.lfsr == periodic.lfsr {
		t.Fatalf("white and periodic LFSR states unexpectedly match: 0x%X", white.lfsr)
	}
}

func TestSN_NoiseChannelHasOwnVolume(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0xD2)
	sn.HandleWrite8(SN_PORT_WRITE, 0xF7)

	if snAtten(sn, 2) != 2 || snAtten(sn, 3) != 7 {
		t.Fatalf("attenuators collided: ch2=%d ch3=%d", snAtten(sn, 2), snAtten(sn, 3))
	}
}

func TestSN_ChannelOutput_Independent(t *testing.T) {
	sound, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0x90)
	sn.HandleWrite8(SN_PORT_WRITE, 0xBF)
	sn.HandleWrite8(SN_PORT_WRITE, 0xDF)
	sn.HandleWrite8(SN_PORT_WRITE, 0xFF)

	if sound.snVoices[0].enabled != true || sound.snVoices[1].enabled || sound.snVoices[2].enabled || sound.snVoices[3].enabled {
		t.Fatalf("unexpected SN voice enables: %v %v %v %v", sound.snVoices[0].enabled, sound.snVoices[1].enabled, sound.snVoices[2].enabled, sound.snVoices[3].enabled)
	}
}

func TestSN_PowerOn_AllSilent(t *testing.T) {
	_, sn := newTestSN(t)
	for ch := range 4 {
		if snAtten(sn, ch) != 15 {
			t.Fatalf("atten[%d]: got %d, want 15", ch, snAtten(sn, ch))
		}
	}
}

func TestSN_ReadyFlag(t *testing.T) {
	_, sn := newTestSN(t)
	if got := sn.HandleRead(SN_PORT_READY); got&1 == 0 {
		t.Fatalf("ready: got 0x%X, want bit 0 set", got)
	}
}

func TestSN_ModeRegister_Default(t *testing.T) {
	_, sn := newTestSN(t)
	if got := sn.HandleRead(SN_PORT_MODE); got != SN76489_MODE_LFSR_15 {
		t.Fatalf("mode: got %d, want default 15-bit", got)
	}
}

func TestSN_ModeRegister_SegaSelect(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_MODE, SN76489_MODE_LFSR_16)
	if got := sn.HandleRead(SN_PORT_MODE); got != SN76489_MODE_LFSR_16 {
		t.Fatalf("mode: got %d, want 16-bit", got)
	}
	if sn.lfsr != SN16_NOISE_LFSR_MASK {
		t.Fatalf("lfsr seed: got 0x%X, want 0x%X", sn.lfsr, SN16_NOISE_LFSR_MASK)
	}
}

func TestSN_BusWrite32_OnlyLowByteHonored(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite(SN_PORT_WRITE, 0xDEADBE90)
	if snAtten(sn, 0) != 0 {
		t.Fatalf("atten: got %d, want low-byte latch", snAtten(sn, 0))
	}
}

func TestSN_ToneFreqMatchesDatasheet(t *testing.T) {
	_, sn := newTestSN(t)
	sn.SetClockHz(3579545)
	sn.HandleWrite8(SN_PORT_WRITE, 0x84)
	sn.HandleWrite8(SN_PORT_WRITE, 0x06)

	got := sn.toneFrequency(0)
	want := float64(3579545) / (32 * 100)
	if got < want-0.01 || got > want+0.01 {
		t.Fatalf("freq: got %f, want %f", got, want)
	}
}

func TestSN_VolumeDataByte_Updates(t *testing.T) {
	_, sn := newTestSN(t)
	sn.HandleWrite8(SN_PORT_WRITE, 0x90)
	sn.HandleWrite8(SN_PORT_WRITE, 0x0F)

	if snAtten(sn, 0) != 15 {
		t.Fatalf("atten after data byte: got %d, want 15", snAtten(sn, 0))
	}
}
