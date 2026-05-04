package main

import "testing"

func TestAROSRebootLifecycleTeardownUnmapsSingletonRegions(t *testing.T) {
	bus, err := NewMachineBusSized(32 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	cpu := NewM68KCPU(bus)
	_, dos, root := newTestArosDOSDevice(t)
	dos.bus = bus
	_ = root
	dma, err := NewArosAudioDMA(bus, chip, cpu)
	if err != nil {
		t.Fatalf("NewArosAudioDMA: %v", err)
	}
	clip := NewClipboardBridge(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	bus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, dos.HandleRead, dos.HandleWrite)
	bus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, dma.HandleRead, dma.HandleWrite)
	bus.MapIO(CLIP_REGION_BASE, CLIP_REGION_END, clip.HandleRead, clip.HandleWrite)
	loader.MapIRQDiagnostics()
	runtimeStatus.setAROSDOS(dos)
	runtimeStatus.setPaulaDMA(dma)
	runtimeStatus.setAROSClipboard(clip)

	if got := mappingCount(bus, AROS_DOS_REGION_BASE, AROS_DOS_REGION_END); got != 1 {
		t.Fatalf("DOS mappings before teardown=%d, want 1", got)
	}
	arosTeardownAll(runtimeStatus.snapshot(), bus, chip)
	if got := mappingCount(bus, AROS_DOS_REGION_BASE, AROS_DOS_REGION_END); got != 0 {
		t.Fatalf("DOS mappings after teardown=%d, want 0", got)
	}

	dos2, err := NewArosDOSDevice(bus, root)
	if err != nil {
		t.Fatalf("NewArosDOSDevice second: %v", err)
	}
	bus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, dos2.HandleRead, dos2.HandleWrite)
	if got := mappingCount(bus, AROS_DOS_REGION_BASE, AROS_DOS_REGION_END); got != 1 {
		t.Fatalf("DOS mappings after remap=%d, want 1", got)
	}
}
