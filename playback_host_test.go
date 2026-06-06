package main

import (
	"os"
	"strings"
	"testing"
)

func TestPlaybackHost_StopBookkeepingIsShared(t *testing.T) {
	host := PlayerControlState{PlayBusy: true, PlayErr: true, PlayGen: 4}
	host.StopPlaybackRequest()

	if host.PlayGen != 5 || host.PlayBusy {
		t.Fatalf("stop state gen=%d busy=%v, want gen 5 and not busy", host.PlayGen, host.PlayBusy)
	}
	if !host.PlayErr {
		t.Fatal("stop should not clear the previous error flag")
	}
}

func TestPlaybackHost_GoldenSID6502EventSequence(t *testing.T) {
	file := &SIDFile{
		Header: SIDHeader{
			MagicID:     "PSID",
			Version:     2,
			DataOffset:  0x7C,
			LoadAddress: 0x1000,
			InitAddress: 0x1000,
			PlayAddress: 0x1001,
			Songs:       1,
			StartSong:   1,
		},
		Data: []byte{
			0x60,       // INIT: RTS
			0xA9, 0x21, // PLAY: LDA #$21
			0x8D, 0x00, 0xD4, // STA $D400
			0x60, // RTS
		},
	}
	player, err := newSID6502Player(file, 1, 44100)
	if err != nil {
		t.Fatalf("newSID6502Player: %v", err)
	}
	events, _ := player.RenderFrames(1)
	if len(events) != 1 {
		t.Fatalf("SID events=%d, want 1", len(events))
	}
	if events[0].Reg != 0 || events[0].Value != 0x21 {
		t.Fatalf("SID event=%+v, want reg 0 value 0x21", events[0])
	}
}

func TestPlaybackHost_GoldenSAP6502EventSequence(t *testing.T) {
	_, events, _, _, _, _, _, _, _, err := renderSAPWithLimit(buildTestSAPWithPOKEYWrites(), 44100, 1, 0)
	if err != nil {
		t.Fatalf("renderSAPWithLimit: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("SAP events=%d, want at least 2", len(events))
	}
	if events[0].Reg != 0 || events[0].Value != 0x50 || events[1].Reg != 1 || events[1].Value != 0xAF {
		t.Fatalf("SAP events=%+v, want AUDF1=0x50 then AUDC1=0xAF", events[:2])
	}
}

func TestPlaybackHost_GoldenAYZ80EventSequence(t *testing.T) {
	file := &AYZ80File{
		Header: AYZ80Header{PlayerVersion: 3},
		Songs: []AYZ80Song{{
			Name: "AY",
			Data: AYZ80SongData{
				Points: &AYZ80Points{Stack: 0xF000, Init: 0x0000, Interrupt: 0x4000},
				Blocks: []AYZ80Block{{
					Addr: 0x4000,
					Data: []byte{
						0x01, 0xFD, 0xFF, // LD BC,$FFFD
						0x3E, 0x07, // LD A,$07
						0xED, 0x79, // OUT (C),A
						0x01, 0xFD, 0xBF, // LD BC,$BFFD
						0x3E, 0x55, // LD A,$55
						0xED, 0x79, // OUT (C),A
						0xC9, // RET
					},
				}},
			},
		}},
	}
	player, err := newAYZ80Player(file, 0, 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
	if err != nil {
		t.Fatalf("newAYZ80Player: %v", err)
	}
	events, _ := player.RenderFrames(1)
	if len(events) != 2 || events[1].Reg != 0x07 || events[1].Value != 0x55 {
		t.Fatalf("AY events=%+v, want register 7 value 0x55", events)
	}
}

func TestPlaybackHost_GoldenSN76489VGMEventSequence(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, 44100)
	player := NewPSGPlayer(engine)
	player.SetSNChip(sn)
	data := append(buildVGMHeaderSN(3, SN_CLOCK_NTSC, 0), 0x50, 0x90, 0x70, 0x50, 0x9F, 0x66)

	if err := player.LoadData(data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x90 {
		t.Fatalf("sample 0 SN byte: got 0x%02X, want 0x90", snLastWritten(sn))
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x9F {
		t.Fatalf("sample 1 SN byte: got 0x%02X, want 0x9F", snLastWritten(sn))
	}
}

func TestPlaybackHost_GoldenSNDH68KEventSequence(t *testing.T) {
	data := buildSNDHData([]byte("TITLHost\x00"))
	play := []byte{
		0x13, 0xFC, 0x00, 0x07, 0x00, 0xFF, 0x88, 0x00, // MOVE.B #7,$FF8800
		0x13, 0xFC, 0x00, 0x44, 0x00, 0xFF, 0x88, 0x02, // MOVE.B #$44,$FF8802
		0x4E, 0x75, // RTS
	}
	copy(data[0x300:], play)

	_, events, _, _, _, _, _, _, _, err := renderSNDHWithLimit(data, 44100, 1, 1)
	if err != nil {
		t.Fatalf("renderSNDHWithLimit: %v", err)
	}
	if len(events) != 1 || events[0].Reg != 7 || events[0].Value != 0x44 {
		t.Fatalf("SNDH events=%+v, want register 7 value 0x44", events)
	}
}

func TestPlaybackHost_GoldenTED6502IRQEventSequence(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x01)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)
	bus.AddCycles(TED_CYCLES_PER_LINE + 1)
	if !bus.CheckIRQ() {
		t.Fatal("TED raster compare did not assert IRQ")
	}
	bus.Write(PLUS4_TED_SND_CTRL, 0x18)
	events := bus.CollectEvents()
	if len(events) != 1 || events[0].Reg != TED_REG_SND_CTRL || events[0].Value != 0x18 {
		t.Fatalf("TED events=%+v, want sound control 0x18", events)
	}
}

func TestPlaybackHost_RegisterMappedAudioPlayersUseSharedControl(t *testing.T) {
	if _, err := os.Stat("playback_host.go"); !os.IsNotExist(err) {
		t.Fatalf("playback_host.go must not exist after consolidating on PlayerControlState, stat err=%v", err)
	}

	cases := []struct {
		path string
	}{
		{"sid_player.go"},
		{"psg_player.go"},
		{"ted_player.go"},
		{"pokey_player.go"},
		{"ahx_player.go"},
		{"mod_player.go"},
		{"wav_player.go"},
		{"midi_player.go"},
	}

	for _, tc := range cases {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", tc.path, err)
		}
		source := string(data)
		if !strings.Contains(source, "PlayerControlState") {
			t.Fatalf("%s must use shared playback control PlayerControlState", tc.path)
		}
		if strings.Contains(source, "playbackHostControl") {
			t.Fatalf("%s must not use removed playbackHostControl", tc.path)
		}
	}
}
