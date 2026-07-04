package main

import "testing"

func newSFXBenchmarkRig(b *testing.B) (*SoundChip, *MachineBus) {
	b.Helper()
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		b.Fatalf("NewSoundChip: %v", err)
	}
	bus := NewMachineBus()
	chip.AttachBus(bus)
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterRead, chip.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)
	bus.MapIO(IE_SFX_REGION_BASE, IE_SFX_REGION_END, chip.sfx.HandleRead, chip.sfx.HandleWrite)
	bus.MapIOByte(IE_SFX_REGION_BASE, IE_SFX_REGION_END, chip.sfx.HandleWrite8)
	bus.MapIO(IE_SFX_EXT_REGION_BASE, IE_SFX_EXT_REGION_END, chip.sfx.HandleRead, chip.sfx.HandleWrite)
	bus.MapIOByte(IE_SFX_EXT_REGION_BASE, IE_SFX_EXT_REGION_END, chip.sfx.HandleWrite8)
	bus.Write32(AUDIO_CTRL, 1)
	return chip, bus
}

func benchmarkSFXTriggerTickSample(b *testing.B, liveChannels int) {
	chip, bus := newSFXBenchmarkRig(b)
	ptr := uint32(0x3000)
	for ch := 0; ch < liveChannels; ch++ {
		bus.memory[ptr+uint32(ch)] = 64
		triggerSFX(bus, ch, ptr+uint32(ch), 1, SAMPLE_RATE, 128, SFX_FORMAT_SIGNED8, SFX_CTRL_LOOP_EN)
		base := ieSFXChannelBase(ch)
		bus.Write32(base+SFX_LOOP_PTR, ptr+uint32(ch))
		bus.Write32(base+SFX_LOOP_LEN, 1)
		bus.Write32(base+SFX_CTRL, SFX_CTRL_TRIGGER|SFX_CTRL_LOOP_EN)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chip.sfx.TickSample()
	}
}

func BenchmarkSFXTrigger_TickSampleIdle32Channels(b *testing.B) {
	benchmarkSFXTriggerTickSample(b, 0)
}

func BenchmarkSFXTrigger_TickSampleFourLive(b *testing.B) {
	benchmarkSFXTriggerTickSample(b, 4)
}

func BenchmarkSFXTrigger_TickSampleTwentyEightLive(b *testing.B) {
	benchmarkSFXTriggerTickSample(b, 28)
}
