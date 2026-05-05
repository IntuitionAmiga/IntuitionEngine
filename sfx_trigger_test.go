package main

import (
	"math"
	"testing"
)

func newSFXTestRig(t *testing.T) (*SoundChip, *MachineBus) {
	t.Helper()
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	bus := NewMachineBus()
	chip.AttachBus(bus)
	bus.MapIO(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterRead, chip.HandleRegisterWrite)
	bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)
	bus.MapIO(IE_SFX_REGION_BASE, IE_SFX_REGION_END, chip.sfx.HandleRead, chip.sfx.HandleWrite)
	bus.MapIOByte(IE_SFX_REGION_BASE, IE_SFX_REGION_END, chip.sfx.HandleWrite8)
	bus.Write32(AUDIO_CTRL, 1)
	return chip, bus
}

func TestSFXTrigger_OneShot_PlaysOnce(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2000)
	bus.memory[ptr] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)

	if got := chip.ReadSample(); got <= 0.9 {
		t.Fatalf("first sample: got %f, want audible positive sample", got)
	}
	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("second sample after one-shot: got %f, want silence", got)
	}
}

func TestSFXTrigger_Loop_Continues(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2100)
	bus.memory[ptr] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, SFX_CTRL_LOOP_EN)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_PTR, ptr)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_LEN, 1)
	bus.Write32(IE_SFX_CH_BASE+SFX_CTRL, SFX_CTRL_TRIGGER|SFX_CTRL_LOOP_EN)

	for i := 0; i < 4; i++ {
		if got := chip.ReadSample(); got <= 0.9 {
			t.Fatalf("loop sample %d: got %f, want audible positive sample", i, got)
		}
	}
}

func TestSFXTrigger_LoopLength_LimitsWrappedPlayback(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2180)
	for i, v := range []byte{10, 20, 30, 40} {
		bus.memory[ptr+uint32(i)] = v
	}
	triggerSFX(bus, 0, ptr, 4, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, SFX_CTRL_LOOP_EN)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_PTR, ptr+1)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_LEN, 2)
	bus.Write32(IE_SFX_CH_BASE+SFX_CTRL, SFX_CTRL_TRIGGER|SFX_CTRL_LOOP_EN)

	var got []float32
	for i := 0; i < 8; i++ {
		got = append(got, chip.ReadSample())
	}
	wantBytes := []byte{10, 20, 30, 40, 20, 30, 20, 30}
	for i, b := range wantBytes {
		want := float32(int8(b)) / 127.0
		if math.Abs(float64(got[i]-want)) > 0.0001 {
			t.Fatalf("sample %d: got %f, want %f", i, got[i], want)
		}
	}
}

func TestSFXTrigger_LoopRange_CanLiveOutsideIntroRange(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	introPtr := uint32(0x21C0)
	loopPtr := uint32(0x21D0)
	for i, v := range []byte{10, 20} {
		bus.memory[introPtr+uint32(i)] = v
	}
	for i, v := range []byte{30, 40, 50} {
		bus.memory[loopPtr+uint32(i)] = v
	}

	triggerSFX(bus, 0, introPtr, 2, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, SFX_CTRL_LOOP_EN)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_PTR, loopPtr)
	bus.Write32(IE_SFX_CH_BASE+SFX_LOOP_LEN, 3)
	bus.Write32(IE_SFX_CH_BASE+SFX_CTRL, SFX_CTRL_TRIGGER|SFX_CTRL_LOOP_EN)

	wantBytes := []byte{10, 20, 30, 40, 50, 30, 40, 50}
	for i, b := range wantBytes {
		got := chip.ReadSample()
		want := float32(int8(b)) / 127.0
		if math.Abs(float64(got-want)) > 0.0001 {
			t.Fatalf("sample %d: got %f, want %f", i, got, want)
		}
	}
}

func TestSFXTrigger_Stop_Silences(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2200)
	for i := 0; i < 8; i++ {
		bus.memory[ptr+uint32(i)] = 127
	}
	triggerSFX(bus, 0, ptr, 8, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got == 0 {
		t.Fatal("expected first sample before stop to be audible")
	}
	bus.Write32(IE_SFX_CH_BASE+SFX_CTRL, SFX_CTRL_STOP)
	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("sample after stop: got %f, want silence", got)
	}
}

func TestSFXTrigger_Volume_Scales(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2300)
	bus.memory[ptr] = 127
	bus.memory[ptr+1] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 0, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("VOL=0 sample: got %f, want silence", got)
	}
	triggerSFX(bus, 0, ptr+1, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got <= 0.9 {
		t.Fatalf("VOL=255 sample: got %f, want near full-scale", got)
	}
}

func TestSFXTrigger_VolumeZero_StillAdvancesPlayback(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2380)
	bus.memory[ptr] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 0, SFX_FORMAT_SIGNED8, 0)
	if status := bus.Read32(IE_SFX_CH_BASE + SFX_CTRL); status&SFX_STATUS_PLAYING == 0 {
		t.Fatalf("status after muted trigger: got 0x%X, want playing bit", status)
	}
	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("muted sample: got %f, want silence", got)
	}
	chip.ReadSample()
	if status := bus.Read32(IE_SFX_CH_BASE + SFX_CTRL); status&SFX_STATUS_PLAYING != 0 {
		t.Fatalf("status after muted playback consumed: got 0x%X, want stopped", status)
	}
}

func TestSFXTrigger_Status_PlayingBit(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2400)
	bus.memory[ptr] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if status := bus.Read32(IE_SFX_CH_BASE + SFX_CTRL); status&SFX_STATUS_PLAYING == 0 {
		t.Fatalf("status after trigger: got 0x%X, want playing bit", status)
	}
	chip.ReadSample()
	chip.ReadSample()
	if status := bus.Read32(IE_SFX_CH_BASE + SFX_CTRL); status&SFX_STATUS_PLAYING != 0 {
		t.Fatalf("status after playback: got 0x%X, want stopped", status)
	}
}

func TestSFXTrigger_FourChannelsIndependent(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2500)
	for i, v := range []byte{16, 32, 64, 127} {
		bus.memory[ptr+uint32(i)] = v
		triggerSFX(bus, i, ptr+uint32(i), 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	}
	got := chip.ReadSample()
	want := (float32(16) + float32(32) + float32(64) + float32(127)) / 127.0
	want = clampF32(want, MIN_SAMPLE, MAX_SAMPLE)
	if math.Abs(float64(got-want)) > 0.0001 {
		t.Fatalf("mixed channels: got %f, want %f", got, want)
	}
}

func TestSFXTrigger_PtrOutOfRange_Errors(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	triggerSFX(bus, 0, uint32(len(bus.memory)+1), 4, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("out-of-range sample: got %f, want silence", got)
	}
	status := bus.Read32(IE_SFX_CH_BASE + SFX_CTRL)
	if status&SFX_STATUS_ERROR == 0 {
		t.Fatalf("status: got 0x%X, want error bit", status)
	}
}

func TestSFXTrigger_Reset_ClearsPlaybackAndStatus(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2580)
	for i := 0; i < 8; i++ {
		bus.memory[ptr+uint32(i)] = 127
	}
	triggerSFX(bus, 0, ptr, 8, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got == 0 {
		t.Fatal("expected audible SFX before reset")
	}
	triggerSFX(bus, 1, uint32(len(bus.memory)+1), 4, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if status := bus.Read32(IE_SFX_CH_BASE + IE_SFX_CH_STRIDE + SFX_CTRL); status&SFX_STATUS_ERROR == 0 {
		t.Fatalf("pre-reset status: got 0x%X, want error bit", status)
	}

	chip.Reset()
	bus.Write32(AUDIO_CTRL, 1)

	if got := chip.ReadSample(); got != 0 {
		t.Fatalf("post-reset sample: got %f, want silence", got)
	}
	for ch := 0; ch < IE_SFX_CHANNELS; ch++ {
		status := bus.Read32(IE_SFX_CH_BASE + uint32(ch)*IE_SFX_CH_STRIDE + SFX_CTRL)
		if status != 0 {
			t.Fatalf("channel %d status after reset: got 0x%X, want 0", ch, status)
		}
	}
}

func TestSFXTrigger_CoexistsWithMOD(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	bus.Write32(FLEX_CH0_BASE+FLEX_OFF_DAC, 64)
	before := bus.Read32(FLEX_CH0_BASE + FLEX_OFF_DAC)

	ptr := uint32(0x2600)
	bus.memory[ptr] = 127
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got == 0 {
		t.Fatal("expected SFX output")
	}
	after := bus.Read32(FLEX_CH0_BASE + FLEX_OFF_DAC)
	if after != before {
		t.Fatalf("FLEX DAC changed: got 0x%X, want 0x%X", after, before)
	}
}

func TestSFXTrigger_Format_Signed8_Unsigned8_Signed16(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2700)
	bus.memory[ptr] = 0x80
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	if got := chip.ReadSample(); got > -0.99 {
		t.Fatalf("signed8: got %f, want negative full scale", got)
	}

	bus.memory[ptr+1] = 255
	triggerSFX(bus, 0, ptr+1, 1, SAMPLE_RATE, 255, SFX_FORMAT_UNSIGNED8, 0)
	if got := chip.ReadSample(); got < 0.98 {
		t.Fatalf("unsigned8: got %f, want positive full scale", got)
	}

	bus.memory[ptr+2] = 0xFF
	bus.memory[ptr+3] = 0x7F
	triggerSFX(bus, 0, ptr+2, 2, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED16, 0)
	if got := chip.ReadSample(); got < 0.99 {
		t.Fatalf("signed16: got %f, want positive full scale", got)
	}
}

func TestSFXTrigger_PanByteIgnored(t *testing.T) {
	chip, bus := newSFXTestRig(t)
	ptr := uint32(0x2800)
	bus.memory[ptr] = 64
	bus.memory[ptr+1] = 64
	triggerSFX(bus, 0, ptr, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	bus.Write32(IE_SFX_CH_BASE+SFX_PAN_RESERVED, 0)
	left := chip.ReadSample()

	triggerSFX(bus, 0, ptr+1, 1, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	bus.Write32(IE_SFX_CH_BASE+SFX_PAN_RESERVED, 0xFFFF)
	right := chip.ReadSample()
	if left != right {
		t.Fatalf("pan-reserved changed mono output: got %f and %f", left, right)
	}
}

func triggerSFX(bus *MachineBus, channel int, ptr, length, freq uint32, vol uint16, format uint8, ctrl uint32) {
	base := IE_SFX_CH_BASE + uint32(channel)*IE_SFX_CH_STRIDE
	bus.Write32(base+SFX_PTR, ptr)
	bus.Write32(base+SFX_LEN, length)
	bus.Write32(base+SFX_FREQ, freq)
	bus.Write16(base+SFX_VOL, vol)
	bus.Write8(base+SFX_FORMAT, format)
	bus.Write32(base+SFX_CTRL, ctrl|SFX_CTRL_TRIGGER)
}
