package main

import (
	"math"
	"testing"
)

func mapTestPSG(bus *MachineBus, engine *PSGEngine) {
	bus.MapIO(PSG_BASE, PSG_END, engine.HandleRead, engine.HandleWrite)
	bus.MapIOByte(PSG_BASE, PSG_END, engine.HandleWrite8)
	bus.MapIOWideWriteFanout(PSG_BASE, PSG_END)
	bus.MapIO(PSG_PLUS_CTRL, PSG_PLUS_CTRL, engine.HandlePSGPlusRead, engine.HandlePSGPlusWrite)
}

func TestPSGBusWideWritesFanOutToRegisters(t *testing.T) {
	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	bus := NewMachineBus()
	mapTestPSG(bus, engine)

	bus.Write32(PSG_BASE, 0x44332211)
	for reg, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := engine.regs[reg]; got != want {
			t.Fatalf("R%d=0x%02X, want 0x%02X", reg, got, want)
		}
	}

	bus.Write16(PSG_BASE+2, 0x6655)
	if got := engine.regs[2]; got != 0x55 {
		t.Fatalf("R2=0x%02X, want 0x55", got)
	}
	if got := engine.regs[3]; got != 0x66 {
		t.Fatalf("R3=0x%02X, want 0x66", got)
	}

	bus.Write8(PSG_BASE+4, 0x77)
	if got := engine.regs[4]; got != 0x77 {
		t.Fatalf("R4=0x%02X, want 0x77", got)
	}
}

func TestPSGBusDoesNotFanOutPSGPlusControl(t *testing.T) {
	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	bus := NewMachineBus()
	mapTestPSG(bus, engine)

	bus.Write32(PSG_PLUS_CTRL, 0x00000100)
	if engine.PSGPlusEnabled() {
		t.Fatalf("PSG+ enabled from low-byte-zero dword; PSG_PLUS_CTRL was fanned out")
	}
	bus.Write32(PSG_PLUS_CTRL, 0x00000001)
	if !engine.PSGPlusEnabled() {
		t.Fatalf("PSG+ not enabled by dword write to PSG_PLUS_CTRL")
	}
}

func TestPSGCopperWideWriteFansOutToRegisters(t *testing.T) {
	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	video, bus := newCopperTestRig(t)
	mapTestPSG(bus, engine)

	listAddr := uint32(0x600)
	bus.Write32(listAddr, copperSetBaseWord(PSG_BASE))
	bus.Write32(listAddr+4, copperMoveWord(0))
	bus.Write32(listAddr+8, 0x44332211)
	bus.Write32(listAddr+12, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	for reg, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := engine.regs[reg]; got != want {
			t.Fatalf("R%d=0x%02X, want 0x%02X", reg, got, want)
		}
	}
}

func TestPSGPeriodZeroToneUsesPeriodOne(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.SetClockHz(PSG_CLOCK_ATARI_ST)
	engine.WriteRegister(0, 0)
	engine.WriteRegister(1, 0)
	engine.WriteRegister(7, 0xFE)

	want := float32(PSG_CLOCK_ATARI_ST) / 16.0
	if got := chip.channels[0].frequency; got != want {
		t.Fatalf("frequency=%.2f, want %.2f", got, want)
	}
}

func TestPSGResetEnvelopeClearsSampleCounter(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetClockHz(uint32(SAMPLE_RATE * 256))
	engine.WriteRegister(11, 4)
	engine.WriteRegister(13, 0)
	engine.TickSample()
	engine.TickSample()
	if engine.envSampleCounter == 0 {
		t.Fatalf("test setup did not advance envSampleCounter")
	}

	engine.WriteRegister(13, 0)
	if got := engine.envSampleCounter; got != 0 {
		t.Fatalf("envSampleCounter=%v, want 0", got)
	}
}

func TestPSGWriteRegisterMirrorsBusMemory(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	mem := make([]byte, PSG_BASE+PSG_REG_COUNT)
	engine.AttachBusMemory(mem)

	engine.WriteRegister(5, 0xA5)
	if got := mem[PSG_BASE+5]; got != 0xA5 {
		t.Fatalf("bus mirror R5=0x%02X, want 0xA5", got)
	}
}

func TestPSGStopPlaybackSilencesChannels(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(7, 0x00)
	engine.WriteRegister(8, 0x0F)
	engine.WriteRegister(9, 0x0F)
	engine.WriteRegister(10, 0x0F)
	for ch := range 3 {
		if chip.channels[ch].volume == 0 {
			t.Fatalf("test setup channel %d volume is zero", ch)
		}
	}

	engine.StopPlayback()
	for ch := range 4 {
		if got := chip.channels[ch].volume; got != 0 {
			t.Fatalf("channel %d volume=%v, want 0", ch, got)
		}
	}
	for ch := range 3 {
		if got := chip.channels[ch].noiseMix; got != 0 {
			t.Fatalf("channel %d noiseMix=%v, want 0", ch, got)
		}
	}
}

func TestPSGSilenceClearsNoiseOnlyOutput(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(6, 0x01)
	engine.WriteRegister(7, 0x37)
	engine.WriteRegister(8, 0x0F)
	if chip.channels[0].noiseMix == 0 {
		t.Fatalf("test setup noiseMix is zero")
	}

	engine.StopPlayback()
	for range 16 {
		if got := chip.GenerateSample(); got != 0 {
			t.Fatalf("sample after StopPlayback=%f, want 0", got)
		}
	}
}

func TestPSGPlayerBusyClearsOnNaturalEnd(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	player.playBusy = true
	engine.SetEvents([]PSGEvent{{Sample: 0, Reg: 8, Value: 0x0F}}, 1, false, 0)

	engine.TickSample()
	if engine.IsPlaying() {
		t.Fatalf("engine still playing after totalSamples")
	}
	if got := player.HandlePlayRead(PSG_PLAY_CTRL); got&1 != 0 {
		t.Fatalf("PLAY_CTRL busy bit still set: 0x%X", got)
	}
	if player.playBusy {
		t.Fatalf("playBusy was not written back false")
	}
}

func TestPSGRegisters14And15AndMovedPSGPlusControl(t *testing.T) {
	if PSG_REG_COUNT != 16 {
		t.Fatalf("PSG_REG_COUNT=%d, want 16", PSG_REG_COUNT)
	}
	if PSG_END != PSG_BASE+15 {
		t.Fatalf("PSG_END=0x%X, want 0x%X", PSG_END, PSG_BASE+15)
	}
	if PSG_PLUS_CTRL != 0xF0C20 {
		t.Fatalf("PSG_PLUS_CTRL=0x%X, want 0xF0C20", PSG_PLUS_CTRL)
	}

	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(14, 0xBE)
	engine.WriteRegister(15, 0xEF)
	if got := engine.HandleRead(PSG_BASE + 14); got != 0xBE {
		t.Fatalf("R14 read=0x%X, want 0xBE", got)
	}
	if got := engine.HandleRead(PSG_BASE + 15); got != 0xEF {
		t.Fatalf("R15 read=0x%X, want 0xEF", got)
	}

	engine.HandlePSGPlusWrite(0xF0C0E, 1)
	if engine.PSGPlusEnabled() {
		t.Fatalf("old PSG+ address still enables PSG+")
	}
	engine.HandlePSGPlusWrite(PSG_PLUS_CTRL, 1)
	if !engine.PSGPlusEnabled() {
		t.Fatalf("new PSG+ address did not enable PSG+")
	}
}

func TestPSG6502AliasCoversRegisters14And15(t *testing.T) {
	rig := newCPU6502TestRig()
	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	mapTestPSG(rig.bus, engine)
	rig.resetAndLoad(0x0200, []byte{
		0xA9, 0xBE, 0x8D, 0x0E, 0xD4, // LDA #$BE; STA $D40E
		0xA9, 0xEF, 0x8D, 0x0F, 0xD4, // LDA #$EF; STA $D40F
		0xAD, 0x0F, 0xD4, // LDA $D40F
		0xEA,
	})

	runSingleInstruction(t, rig.cpu, 0x0200)
	runSingleInstruction(t, rig.cpu, 0x0202)
	runSingleInstruction(t, rig.cpu, 0x0205)
	runSingleInstruction(t, rig.cpu, 0x0207)
	runSingleInstruction(t, rig.cpu, 0x020A)

	if got := engine.regs[14]; got != 0xBE {
		t.Fatalf("R14=0x%02X, want 0xBE", got)
	}
	if got := rig.cpu.A; got != 0xEF {
		t.Fatalf("LDA $D40F A=0x%02X, want 0xEF", got)
	}
}

func TestPSGEngineWriteR13FFResetsEnvelope(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetClockHz(uint32(SAMPLE_RATE * 256))
	engine.WriteRegister(11, 1)
	engine.WriteRegister(13, 0)
	engine.TickSample()
	engine.TickSample()
	engine.WriteRegister(13, 0xFF)

	if engine.envLevel != 0 || engine.envDirection != 1 || engine.envSampleCounter != 0 {
		t.Fatalf("R13=0xFF reset got level=%d dir=%d counter=%v", engine.envLevel, engine.envDirection, engine.envSampleCounter)
	}
}

func TestPSGResetSilencesChannels(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(7, 0x00)
	engine.WriteRegister(8, 0x0F)
	engine.WriteRegister(9, 0x0F)
	engine.WriteRegister(10, 0x0F)

	engine.Reset()
	for ch := range 4 {
		if got := chip.channels[ch].volume; got != 0 {
			t.Fatalf("channel %d volume=%v, want 0", ch, got)
		}
	}
	for reg, got := range engine.regs {
		if got != 0 {
			t.Fatalf("R%d=0x%02X, want 0", reg, got)
		}
	}
	if engine.channelsInit {
		t.Fatalf("channelsInit still true")
	}
}

func TestPSGLogVolumeCurveAndLegacyEscape(t *testing.T) {
	got := psgVolumeGain(8, false)
	if math.Abs(float64(got-0.077637)) > 0.0005 {
		t.Fatalf("default level 8 gain=%f, want AY log ~0.077637", got)
	}

	engine, _ := newTestPSGEngine(SAMPLE_RATE)
	engine.SetLegacyLinearVolume(true)
	if got := engine.volumeGain(8); got != 8.0/15.0 {
		t.Fatalf("legacy level 8 gain=%f, want %f", got, 8.0/15.0)
	}
}

func TestPSGPlusMixGainScalesToneChannels(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(7, 0x38)
	engine.WriteRegister(8, 0x0F)
	engine.WriteRegister(9, 0x0F)
	engine.WriteRegister(10, 0x0F)
	baseline := [3]float32{chip.channels[0].volume, chip.channels[1].volume, chip.channels[2].volume}

	engine.SetPSGPlusEnabled(true)
	engine.WriteRegister(8, 0x0F)
	engine.WriteRegister(9, 0x0F)
	engine.WriteRegister(10, 0x0F)
	for ch, scale := range psgPlusMixGain {
		want := baseline[ch] * scale
		if want > 1 {
			want = 1
		}
		if got := chip.channels[ch].volume; math.Abs(float64(got-want)) > 1.0/NORMALISE_8BIT {
			t.Fatalf("channel %d PSG+ volume=%f, want %f", ch, got, want)
		}
	}
}
