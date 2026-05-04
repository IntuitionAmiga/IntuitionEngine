package main

import (
	"sync"
	"testing"
)

func newTestArosAudioDMA(t *testing.T) (*MachineBus, *SoundChip, *M68KCPU, *ArosAudioDMA) {
	t.Helper()
	bus, err := NewMachineBusSized(32 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	cpu := NewM68KCPU(bus)
	dma, err := NewArosAudioDMA(bus, chip, cpu)
	if err != nil {
		t.Fatalf("NewArosAudioDMA: %v", err)
	}
	return bus, chip, cpu, dma
}

func writeArosDMAChannel(dma *ArosAudioDMA, ch int, ptr, length, period, volume uint32) {
	base := AROS_AUD_REGION_BASE + uint32(ch)*AROS_AUD_CH_STRIDE
	dma.HandleWrite(base+AROS_AUD_OFF_PTR, ptr)
	dma.HandleWrite(base+AROS_AUD_OFF_LEN, length)
	dma.HandleWrite(base+AROS_AUD_OFF_PER, period)
	dma.HandleWrite(base+AROS_AUD_OFF_VOL, volume)
}

func armArosDMAChannel(dma *ArosAudioDMA, ch int) {
	dma.HandleWrite(AROS_AUD_DMACON, 0x8000|uint32(1<<ch))
}

func TestPaulaShim_DMACONReArmAfterCompletion(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 2, 1, 64)
	armArosDMAChannel(dma, 0)

	dma.TickSample()
	if dma.channels[0].active {
		t.Fatalf("channel still active after one-sample exhaust")
	}

	writeArosDMAChannel(dma, 0, 0x200, 8, 64, 64)
	armArosDMAChannel(dma, 0)
	if !dma.channels[0].active {
		t.Fatalf("channel did not re-arm after completion")
	}
	before := dma.channels[0].pos
	dma.TickSample()
	if dma.channels[0].pos == before {
		t.Fatalf("channel did not advance after re-arm")
	}
}

func TestPaulaShim_ExhaustClearsDMACONBit(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 2, 1, 64)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	armArosDMAChannel(dma, 0)

	dma.TickSample()
	if got := dma.HandleRead(AROS_AUD_DMACON); got&0x1 != 0 {
		t.Fatalf("DMACON bit still set after exhaust: 0x%X", got)
	}
	if cpu.pendingInterrupt.Load()&(1<<arosAudioIRQLevel) == 0 {
		t.Fatalf("IRQ level %d not asserted after exhaust", arosAudioIRQLevel)
	}
}

func TestPaulaShim_ExhaustWithoutINTENADoesNotAssertIRQ(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 2, 1, 64)
	armArosDMAChannel(dma, 0)

	dma.TickSample()
	if dma.status&0x1 == 0 {
		t.Fatalf("status bit not set after exhaust")
	}
	if got := cpu.pendingInterrupt.Load(); got&(1<<arosAudioIRQLevel) != 0 {
		t.Fatalf("IRQ asserted with INTENA disabled: pending=0x%X", got)
	}
}

func TestPaulaShim_ResetClearsState(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 4, 128, 64)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	armArosDMAChannel(dma, 0)
	dma.status = 0xF

	dma.Reset()
	if dma.dmacon != 0 || dma.intena != 0 || dma.status != 0 || dma.enabled.Load() {
		t.Fatalf("reset left state: dmacon=%X intena=%X status=%X enabled=%v", dma.dmacon, dma.intena, dma.status, dma.enabled.Load())
	}
	for i := range dma.channels {
		if dma.channels[i].active {
			t.Fatalf("channel %d still active after reset", i)
		}
	}
}

func TestPaulaShim_ReloadDoesNotStackMMIO(t *testing.T) {
	bus, chip, cpu, oldDMA := newTestArosAudioDMA(t)
	bus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, oldDMA.HandleRead, oldDMA.HandleWrite)
	chip.SetSampleTicker(oldDMA)
	runtimeStatus.setPaulaDMA(oldDMA)

	arosAudioTeardown(oldDMA, bus, chip)
	newDMA, err := NewArosAudioDMA(bus, chip, cpu)
	if err != nil {
		t.Fatalf("NewArosAudioDMA: %v", err)
	}
	bus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, newDMA.HandleRead, newDMA.HandleWrite)
	chip.SetSampleTicker(newDMA)
	bus.Write32(AROS_AUD_REGION_BASE+AROS_AUD_OFF_VOL, 33)

	if oldDMA.channels[0].vol != 0 {
		t.Fatalf("old DMA received write after teardown: vol=%d", oldDMA.channels[0].vol)
	}
	if newDMA.channels[0].vol != 33 {
		t.Fatalf("new DMA did not receive write: vol=%d", newDMA.channels[0].vol)
	}

	writeArosDMAChannel(oldDMA, 0, 0x100, 8, 128, 64)
	armArosDMAChannel(oldDMA, 0)
	chip.ReadSample()
	if oldDMA.channels[0].pos != 0 {
		t.Fatalf("old DMA still ticked after teardown: pos=%d", oldDMA.channels[0].pos)
	}
}

func TestPaulaShim_TeardownUnregistersTicker(t *testing.T) {
	bus, chip, _, dma := newTestArosAudioDMA(t)
	chip.SetSampleTicker(dma)
	arosAudioTeardown(dma, bus, chip)
	if chip.HasSampleTicker("default") {
		t.Fatalf("default ticker still registered after teardown")
	}
}

func TestPaulaShim_TeardownNilInputsNoPanic(t *testing.T) {
	bus, chip, _, dma := newTestArosAudioDMA(t)
	arosAudioTeardown(nil, bus, chip)
	arosAudioTeardown(dma, nil, chip)
	arosAudioTeardown(dma, bus, nil)
}

func TestPaulaShim_TeardownLeavesForeignTickerAlone(t *testing.T) {
	bus, chip, _, dma := newTestArosAudioDMA(t)
	foreign := &countTicker{}
	chip.SetSampleTicker(foreign)
	arosAudioTeardown(dma, bus, chip)
	chip.ReadSample()
	if foreign.count != 1 {
		t.Fatalf("foreign ticker removed or not fired: count=%d", foreign.count)
	}
}

func TestPaulaShim_LatchedAtArmIgnoresMidBufferWrites(t *testing.T) {
	bus, chip, _, dma := newTestArosAudioDMA(t)
	bus.memory[0x100] = 0x40
	bus.memory[0x200] = 0x7F
	writeArosDMAChannel(dma, 0, 0x100, 4, 128, 64)
	armArosDMAChannel(dma, 0)

	writeArosDMAChannel(dma, 0, 0x200, 4, 1, 64)
	dma.TickSample()

	chip.mu.Lock()
	got := chip.channels[0].dacValue
	chip.mu.Unlock()
	want := float32(int8(0x40)) / 128.0
	if got != want {
		t.Fatalf("DAC read from live ptr after arm: got %f want %f", got, want)
	}
}

func TestPaulaShim_DACHandoffEnablesFlexChannel(t *testing.T) {
	bus, chip, _, dma := newTestArosAudioDMA(t)
	bus.memory[0x100] = 0x40
	writeArosDMAChannel(dma, 0, 0x100, 4, 128, 64)
	armArosDMAChannel(dma, 0)
	dma.TickSample()

	chip.mu.Lock()
	ch := chip.channels[0]
	enabled := ch.enabled
	gate := ch.gate
	dacMode := ch.dacMode
	dacValue := ch.dacValue
	envelopePhase := ch.envelopePhase
	chip.mu.Unlock()

	if !enabled || !gate || !dacMode {
		t.Fatalf("flex channel not enabled for DMA: enabled=%v gate=%v dacMode=%v", enabled, gate, dacMode)
	}
	if dacValue == 0 {
		t.Fatalf("flex channel DAC value was not updated")
	}
	if envelopePhase != ENV_ATTACK {
		t.Fatalf("envelope phase = %d, want ENV_ATTACK", envelopePhase)
	}
}

func TestPaulaShim_BadPointerMutesAndDeactivates(t *testing.T) {
	_, chip, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, dma.profileTop+4, 4, 128, 64)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	armArosDMAChannel(dma, 0)
	dma.TickSample()

	if dma.channels[0].active {
		t.Fatalf("channel still active after bad pointer")
	}
	if dma.status&0x1 == 0 {
		t.Fatalf("status bit not set after bad pointer")
	}
	if cpu.pendingInterrupt.Load()&(1<<arosAudioIRQLevel) == 0 {
		t.Fatalf("IRQ level %d not asserted after bad pointer", arosAudioIRQLevel)
	}
	chip.mu.Lock()
	got := chip.channels[0].dacValue
	chip.mu.Unlock()
	if got != 0 {
		t.Fatalf("DAC not muted after bad pointer: %f", got)
	}
}

func TestPaulaShim_BadPointerWithoutINTENADoesNotAssertIRQ(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, dma.profileTop+4, 4, 128, 64)
	armArosDMAChannel(dma, 0)

	dma.TickSample()
	if dma.status&0x1 == 0 {
		t.Fatalf("status bit not set after bad pointer")
	}
	if got := cpu.pendingInterrupt.Load(); got&(1<<arosAudioIRQLevel) != 0 {
		t.Fatalf("IRQ asserted with INTENA disabled: pending=0x%X", got)
	}
}

func TestPaulaShim_ArmRejectsZeroPeriod(t *testing.T) {
	_, cpuChip, cpu, dma := newTestArosAudioDMA(t)
	_ = cpuChip
	writeArosDMAChannel(dma, 0, 0x100, 4, 0, 64)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	armArosDMAChannel(dma, 0)
	if dma.channels[0].active || dma.dmacon&0x1 != 0 || dma.status&0x1 == 0 {
		t.Fatalf("zero period arm state: active=%v dmacon=%X status=%X", dma.channels[0].active, dma.dmacon, dma.status)
	}
	if cpu.pendingInterrupt.Load()&(1<<arosAudioIRQLevel) == 0 {
		t.Fatalf("zero period did not assert IRQ")
	}
}

func TestPaulaShim_ArmRejectWithoutINTENADoesNotAssertIRQ(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 4, 0, 64)
	armArosDMAChannel(dma, 0)
	if dma.status&0x1 == 0 {
		t.Fatalf("status bit not set after rejected arm")
	}
	if got := cpu.pendingInterrupt.Load(); got&(1<<arosAudioIRQLevel) != 0 {
		t.Fatalf("IRQ asserted with INTENA disabled: pending=0x%X", got)
	}
}

func TestPaulaShim_ArmRejectsZeroLength(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 0, 128, 64)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	armArosDMAChannel(dma, 0)
	if dma.channels[0].active || dma.dmacon&0x1 != 0 || dma.status&0x1 == 0 {
		t.Fatalf("zero length arm state: active=%v dmacon=%X status=%X", dma.channels[0].active, dma.dmacon, dma.status)
	}
	if cpu.pendingInterrupt.Load()&(1<<arosAudioIRQLevel) == 0 {
		t.Fatalf("zero length did not assert IRQ")
	}
}

func TestPaulaShim_ConcurrentWriteAndTick(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 256, 128, 64)
	armArosDMAChannel(dma, 0)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			writeArosDMAChannel(dma, 0, 0x100, 256, uint32(64+i%128), uint32(i%96))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			dma.TickSample()
		}
	}()
	wg.Wait()
}

func TestPaulaShim_OddPtrMaskedToEven(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	dma.HandleWrite(AROS_AUD_REGION_BASE+AROS_AUD_OFF_PTR, 0x101)
	if got := dma.HandleRead(AROS_AUD_REGION_BASE + AROS_AUD_OFF_PTR); got != 0x100 {
		t.Fatalf("ptr got 0x%X, want 0x100", got)
	}
}

func TestPaulaShim_OddLenPreservedAsWordCount(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	dma.HandleWrite(AROS_AUD_REGION_BASE+AROS_AUD_OFF_LEN, 5)
	if got := dma.HandleRead(AROS_AUD_REGION_BASE + AROS_AUD_OFF_LEN); got != 5 {
		t.Fatalf("len got %d, want 5", got)
	}
}

func TestPaulaShim_PeriodZeroIgnored(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	dma.HandleWrite(AROS_AUD_REGION_BASE+AROS_AUD_OFF_PER, 128)
	dma.HandleWrite(AROS_AUD_REGION_BASE+AROS_AUD_OFF_PER, 0)
	if got := dma.HandleRead(AROS_AUD_REGION_BASE + AROS_AUD_OFF_PER); got != 128 {
		t.Fatalf("period got %d, want 128", got)
	}
}

func TestPaulaShim_VolumeClampedAt64(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	dma.HandleWrite(AROS_AUD_REGION_BASE+AROS_AUD_OFF_VOL, 127)
	if got := dma.HandleRead(AROS_AUD_REGION_BASE + AROS_AUD_OFF_VOL); got != 64 {
		t.Fatalf("volume got %d, want 64", got)
	}
}

func TestArosAudioDMA_DoubleBufferReArmContinuesWithoutDMACONRewrite(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 2, 128, 64)
	armArosDMAChannel(dma, 0)
	writeArosDMAChannel(dma, 0, 0x200, 4, 128, 64)

	dma.mu.Lock()
	dma.channels[0].pos = dma.channels[0].llen * 2
	dma.mu.Unlock()
	dma.TickSample()

	if got := dma.HandleRead(AROS_AUD_DMACON); got&1 == 0 {
		t.Fatalf("DMACON channel bit cleared across double-buffer re-arm: 0x%X", got)
	}
	if !dma.channels[0].active || dma.channels[0].lptr != 0x200 || dma.channels[0].llen != 4 {
		t.Fatalf("channel did not latch next buffer: active=%v lptr=0x%X llen=%d",
			dma.channels[0].active, dma.channels[0].lptr, dma.channels[0].llen)
	}
}

func TestArosAudioDMA_PhasePreservedAcrossReArm(t *testing.T) {
	_, _, _, dma := newTestArosAudioDMA(t)
	writeArosDMAChannel(dma, 0, 0x100, 2, 256, 64)
	armArosDMAChannel(dma, 0)
	writeArosDMAChannel(dma, 0, 0x200, 2, 256, 64)

	dma.mu.Lock()
	dma.channels[0].phase = 0.5
	dma.channels[0].pos = dma.channels[0].llen * 2
	dma.mu.Unlock()
	dma.TickSample()

	if dma.channels[0].phase == 0 {
		t.Fatalf("phase reset across double-buffer re-arm")
	}
}

func TestArosAudioDMA_ProfileBoundErrorSurfaces(t *testing.T) {
	bus, err := NewMachineBusSized(1 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	if _, err := NewArosAudioDMA(bus, nil, nil); err == nil {
		t.Fatalf("NewArosAudioDMA succeeded with undersized AROS profile")
	}
}

func TestArosAudioDMA_IRQOnEveryWrappedBuffer(t *testing.T) {
	_, _, cpu, dma := newTestArosAudioDMA(t)
	dma.HandleWrite(AROS_AUD_INTENA, 0x8001)
	writeArosDMAChannel(dma, 0, 0x100, 2, 128, 64)
	armArosDMAChannel(dma, 0)

	for i := 0; i < 2; i++ {
		writeArosDMAChannel(dma, 0, 0x200+uint32(i)*0x100, 2, 128, 64)
		cpu.pendingInterrupt.Store(0)
		dma.mu.Lock()
		dma.channels[0].pos = dma.channels[0].llen * 2
		dma.mu.Unlock()
		dma.TickSample()
		if dma.status&1 == 0 {
			t.Fatalf("wrap %d did not set status", i)
		}
		if cpu.pendingInterrupt.Load()&(1<<arosAudioIRQLevel) == 0 {
			t.Fatalf("wrap %d did not assert L%d IRQ", i, arosAudioIRQLevel)
		}
		dma.HandleWrite(AROS_AUD_STATUS, 1)
	}
}
