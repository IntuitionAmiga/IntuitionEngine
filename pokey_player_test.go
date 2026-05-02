package main

import "testing"

func TestPOKEY_PlayerWrite_PTR_Plus2_Byte(t *testing.T) {
	player := NewPOKEYPlayer(NewPOKEYEngine(nil, 44100))
	player.HandlePlayWrite(SAP_PLAY_PTR, 0xAABBCCDD)
	player.HandlePlayWrite8(SAP_PLAY_PTR+2, 0x11)

	if got, want := player.HandlePlayRead(SAP_PLAY_PTR), uint32(0xAA11CCDD); got != want {
		t.Fatalf("PTR byte +2 write got 0x%08X, want 0x%08X", got, want)
	}
}

func TestPOKEY_PlayerBusWrite8AndWrite16Staging(t *testing.T) {
	bus := NewMachineBus()
	player := NewPOKEYPlayer(NewPOKEYEngine(nil, 44100))
	bus.MapIO(SAP_PLAY_PTR, SAP_SUBSONG, player.HandlePlayRead, player.HandlePlayWrite)
	bus.MapIOByte(SAP_PLAY_PTR, SAP_SUBSONG, player.HandlePlayWrite8)

	bus.Write32(SAP_PLAY_PTR, 0xAABBCCDD)
	bus.Write8(SAP_PLAY_PTR+2, 0x11)
	if got, want := player.HandlePlayRead(SAP_PLAY_PTR), uint32(0xAA11CCDD); got != want {
		t.Fatalf("bus Write8 PTR +2 got 0x%08X, want 0x%08X", got, want)
	}

	bus.Write32(SAP_PLAY_LEN, 0x00114455)
	bus.Write16(SAP_PLAY_LEN+2, 0xCCDD)
	if got, want := player.HandlePlayRead(SAP_PLAY_LEN), uint32(0xCCDD4455); got != want {
		t.Fatalf("bus Write16 LEN +2 got 0x%08X, want 0x%08X", got, want)
	}
}

func TestPOKEY_PlayerWrite_PTR_LEN_Plus2_WordPreservesHighByte(t *testing.T) {
	player := NewPOKEYPlayer(NewPOKEYEngine(nil, 44100))
	player.HandlePlayWrite(SAP_PLAY_PTR, 0x0011CCDD)
	player.HandlePlayWrite(SAP_PLAY_LEN, 0x00224455)

	player.HandlePlayWrite(SAP_PLAY_PTR+2, 0xAABB)
	player.HandlePlayWrite(SAP_PLAY_LEN+2, 0xCCDD)

	if got, want := player.HandlePlayRead(SAP_PLAY_PTR), uint32(0xAABBCCDD); got != want {
		t.Fatalf("PTR +2 word write got 0x%08X, want 0x%08X", got, want)
	}
	if got, want := player.HandlePlayRead(SAP_PLAY_LEN), uint32(0xCCDD4455); got != want {
		t.Fatalf("LEN +2 word write got 0x%08X, want 0x%08X", got, want)
	}
}

func TestPOKEY_SAP_Subsong_Roundtrip(t *testing.T) {
	player := NewPOKEYPlayer(NewPOKEYEngine(nil, 44100))
	player.HandlePlayWrite(SAP_SUBSONG, 2)

	if got := player.HandlePlayRead(SAP_SUBSONG); got != 2 {
		t.Fatalf("SAP_SUBSONG read got %d, want 2", got)
	}
}

func TestPOKEY_StereoSAP_Routing(t *testing.T) {
	chip := newTestSoundChip()
	left := NewPOKEYEngine(chip, 44100)
	right := NewPOKEYEngineMulti(chip, 44100, 4)
	left.setRight(right)
	left.SetEvents([]SAPPOKEYEvent{
		{Sample: 0, Reg: 1, Value: AUDC_VOLUME_MASK, Chip: 1},
	}, 2, false, 0)
	left.SetPlaying(true)

	left.TickSample()

	if got := chip.channels[4].volume; got == 0 {
		t.Fatal("stereo POKEY event for Chip=1 did not route to right bank")
	}
	if got := chip.channels[0].volume; got != 0 {
		t.Fatalf("stereo POKEY Chip=1 event leaked to left channel 0 volume %.4f", got)
	}
}

func TestPOKEY_StereoSAP_RestoresSharedBank(t *testing.T) {
	chip := newTestSoundChip()
	player := NewPOKEYPlayer(NewPOKEYEngine(chip, 44100))
	player.configureStereo(true)
	chip.channels[4].waveType = WAVE_NOISE
	chip.channels[4].frequency = 1234
	chip.channels[4].volume = 1
	chip.channels[4].enabled = true

	player.Stop()

	if got := chip.channels[4].waveType; got != WAVE_SQUARE {
		t.Fatalf("channel 4 waveType = %d, want WAVE_SQUARE", got)
	}
	if got := chip.channels[4].frequency; got != 0 {
		t.Fatalf("channel 4 frequency = %.4f, want 0", got)
	}
	if got := chip.channels[4].volume; got != 0 {
		t.Fatalf("channel 4 volume = %.4f, want 0", got)
	}
	if chip.channels[4].enabled {
		t.Fatal("channel 4 still enabled after stereo SAP release")
	}
}

func TestPOKEY_StereoSAP_RightClockTracksLeftClock(t *testing.T) {
	player := NewPOKEYPlayer(NewPOKEYEngine(newTestSoundChip(), 44100))
	player.configureStereo(true)

	player.engine.SetClockHz(POKEY_CLOCK_PAL)
	player.syncStereoClock()

	if player.right == nil {
		t.Fatal("right POKEY was not configured")
	}
	if got := player.right.clockHz; got != POKEY_CLOCK_PAL {
		t.Fatalf("right POKEY clock = %d, want %d", got, POKEY_CLOCK_PAL)
	}
}
