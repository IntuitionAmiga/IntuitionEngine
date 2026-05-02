package main

import (
	"testing"
	"time"
)

func TestPSGEngine_StreamsSNBytesAtSampleTime(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetEvents(nil, 5, false, 0)
	engine.SetSNStream([]SNEvent{{Sample: 1, Byte: 0x90}, {Sample: 3, Byte: 0x9F}}, sn, SN_CLOCK_NTSC)

	engine.TickSample()
	if snWriteCount(sn) != 0 {
		t.Fatalf("write count at sample 0: got %d, want 0", snWriteCount(sn))
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x90 {
		t.Fatalf("last byte at sample 1: got 0x%02X, want 0x90", snLastWritten(sn))
	}
	engine.TickSample()
	engine.TickSample()
	if snLastWritten(sn) != 0x9F {
		t.Fatalf("last byte at sample 3: got 0x%02X, want 0x9F", snLastWritten(sn))
	}
}

func TestPSGEngine_StopPlayback_SilencesSN(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetEvents(nil, 10, false, 0)
	engine.SetSNStream([]SNEvent{{Sample: 0, Byte: 0x90}}, sn, SN_CLOCK_NTSC)
	engine.TickSample()

	engine.StopPlayback()

	for ch := range 4 {
		if snAtten(sn, ch) != 15 {
			t.Fatalf("atten[%d]: got %d, want 15", ch, snAtten(sn, ch))
		}
	}
}

func TestPSGPlayer_LoadData_RoutesSNEvents(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	player.SetSNChip(sn)
	data := append(buildVGMHeaderSN(3, SN_CLOCK_NTSC, 0), 0x50, 0x90, 0x70, 0x50, 0x9F, 0x66)

	if err := player.LoadData(data); err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x90 {
		t.Fatalf("last byte after sample 0: got 0x%02X, want 0x90", snLastWritten(sn))
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x9F {
		t.Fatalf("last byte after sample 1: got 0x%02X, want 0x9F", snLastWritten(sn))
	}
}

func TestPSGPlayer_MixedAYAndSN(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	player.SetSNChip(sn)
	data := append(buildVGMHeaderSN(2, SN_CLOCK_NTSC, PSG_CLOCK_MSX),
		0x50, 0x90,
		0xA0, 0x08, 0x0F,
		0x66)

	if err := player.LoadData(data); err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}
	engine.TickSample()

	if snLastWritten(sn) != 0x90 {
		t.Fatalf("SN byte: got 0x%02X, want 0x90", snLastWritten(sn))
	}
	if engine.regs[8] != 0x0F {
		t.Fatalf("AY reg 8: got 0x%02X, want 0x0F", engine.regs[8])
	}
}

func TestPSGPlayer_NonVGMLoad_ClearsSNStream(t *testing.T) {
	_, sn := newTestSN(t)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	player.SetSNChip(sn)
	data := append(buildVGMHeaderSN(4, SN_CLOCK_NTSC, 0), 0x61, 0x02, 0x00, 0x50, 0x90, 0x66)
	if err := player.LoadData(data); err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}
	if err := player.LoadData([]byte{}); err == nil {
		t.Fatal("expected empty load error")
	}
	for range 4 {
		engine.TickSample()
	}
	if snWriteCount(sn) != 0 {
		t.Fatalf("stale SN stream wrote %d bytes after failed load", snWriteCount(sn))
	}
}

func TestPSGPlayer_AsyncMMIO_RoutesSNEvents(t *testing.T) {
	bus, engine, player, sn := newSNPSGMMIORig(t)
	data := append(buildVGMHeaderSN(3, SN_CLOCK_NTSC, 0), 0x50, 0x90, 0x70, 0x50, 0x9F, 0x66)
	writeBlob(bus, 0x2000, data)

	player.HandlePlayWrite(PSG_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(PSG_PLAY_LEN, uint32(len(data)))
	player.HandlePlayWrite(PSG_PLAY_CTRL, 1)
	waitFor(t, func() bool { return engine.IsPlaying() && engine.totalSamples == 3 })

	engine.TickSample()
	if snLastWritten(sn) != 0x90 {
		t.Fatalf("sample 0 SN byte: got 0x%02X, want 0x90", snLastWritten(sn))
	}
	engine.TickSample()
	if snLastWritten(sn) != 0x9F {
		t.Fatalf("sample 1 SN byte: got 0x%02X, want 0x9F", snLastWritten(sn))
	}
}

func TestPSGPlayer_AsyncMMIO_ClearsSNStreamOnAYLoad(t *testing.T) {
	bus, engine, player, sn := newSNPSGMMIORig(t)
	snData := append(buildVGMHeaderSN(3, SN_CLOCK_NTSC, 0), 0x61, 0x02, 0x00, 0x50, 0x90, 0x66)
	ymData := buildYM2Data(2)
	writeBlob(bus, 0x2000, snData)
	writeBlob(bus, 0x3000, ymData)

	player.HandlePlayWrite(PSG_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(PSG_PLAY_LEN, uint32(len(snData)))
	player.HandlePlayWrite(PSG_PLAY_CTRL, 1)
	waitFor(t, func() bool { return engine.IsPlaying() && len(engine.snEvents) == 1 })

	player.HandlePlayWrite(PSG_PLAY_CTRL, 2)
	player.HandlePlayWrite(PSG_PLAY_PTR, 0x3000)
	player.HandlePlayWrite(PSG_PLAY_LEN, uint32(len(ymData)))
	player.HandlePlayWrite(PSG_PLAY_CTRL, 1)
	waitFor(t, func() bool { return engine.IsPlaying() && engine.totalSamples > 0 && len(engine.snEvents) == 0 })

	before := snWriteCount(sn)
	for range 4 {
		engine.TickSample()
	}
	if snWriteCount(sn) != before {
		t.Fatalf("stale async SN stream wrote %d bytes", snWriteCount(sn)-before)
	}
}

func TestPSGPlayer_DirectSNWrite_StillWorksDuringPlayback(t *testing.T) {
	_, engine, player, sn := newSNPSGMMIORig(t)
	data := append(buildVGMHeaderSN(5, SN_CLOCK_NTSC, 0), 0x61, 0x03, 0x00, 0x50, 0x9F, 0x66)
	if err := player.LoadData(data); err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}

	engine.TickSample()
	sn.HandleWrite8(SN_PORT_WRITE, 0x90)
	if snLastWritten(sn) != 0x90 {
		t.Fatalf("direct SN write during playback not accepted: got 0x%02X", snLastWritten(sn))
	}
	engine.TickSample()
	engine.TickSample()
	engine.TickSample()
	if snLastWritten(sn) != 0x9F {
		t.Fatalf("scheduled SN write after direct write: got 0x%02X, want 0x9F", snLastWritten(sn))
	}
}

func newSNPSGMMIORig(t *testing.T) (*MachineBus, *PSGEngine, *PSGPlayer, *SN76489Chip) {
	t.Helper()
	bus := NewMachineBus()
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewPSGEngine(sound, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	sn := NewSN76489Chip(sound)
	player.SetSNChip(sn)
	player.AttachBus(bus)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, player.HandlePlayRead, player.HandlePlayWrite)
	return bus, engine, player, sn
}

func writeBlob(bus *MachineBus, addr uint32, data []byte) {
	for i, b := range data {
		bus.Write8(addr+uint32(i), b)
	}
}

func waitFor(t *testing.T, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
