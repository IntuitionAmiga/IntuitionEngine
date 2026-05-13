package main

import (
	"encoding/json"
	"fmt"
)

const (
	sn76489SnapshotVersion  = 1
	psgSnapshotVersion      = 1
	sidSnapshotVersion      = 1
	pokeySnapshotVersion    = 1
	tedAudioSnapshotVersion = 1
	vgaSnapshotVersion      = 1
	ulaSnapshotVersion      = 1
	tedVideoSnapshotVersion = 1
	anticSnapshotVersion    = 1
	voodooSnapshotVersion   = 1
)

type sn76489DebugSnapshot struct {
	ClockHz     uint32
	Mode        uint8
	LatchCh     uint8
	LatchVolume bool
	Tone        [3]uint16
	Atten       [4]uint8
	NoiseReg    uint8
	LFSR        uint32
	LastWritten uint8
	WriteCount  uint64
}

func (c *SN76489Chip) DebugSnapshotName() string { return "sn76489" }

func (c *SN76489Chip) DebugSnapshot() (uint32, []byte, error) {
	if c == nil {
		return sn76489SnapshotVersion, nil, fmt.Errorf("nil SN76489 chip")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return marshalSnapshot(sn76489SnapshotVersion, sn76489DebugSnapshot{
		ClockHz: c.clockHz, Mode: c.mode, LatchCh: c.latchCh, LatchVolume: c.latchVolume,
		Tone: c.tone, Atten: c.atten, NoiseReg: c.noiseReg, LFSR: c.lfsr,
		LastWritten: c.lastWritten, WriteCount: c.writeCount,
	})
}

func (c *SN76489Chip) DebugRestoreSnapshot(version uint32, data []byte) error {
	if c == nil {
		return fmt.Errorf("nil SN76489 chip")
	}
	if version != sn76489SnapshotVersion {
		return fmt.Errorf("unsupported SN76489 snapshot version %d", version)
	}
	var snap sn76489DebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clockHz, c.mode, c.latchCh, c.latchVolume = snap.ClockHz, snap.Mode, snap.LatchCh, snap.LatchVolume
	c.tone, c.atten, c.noiseReg, c.lfsr = snap.Tone, snap.Atten, snap.NoiseReg, snap.LFSR
	c.lastWritten, c.writeCount = snap.LastWritten, snap.WriteCount
	c.syncAllVoicesLocked()
	return nil
}

type psgDebugSnapshot struct {
	SampleRate, EnvLevel, EnvDirection            int
	ClockHz                                       uint32
	Regs                                          [PSG_REG_COUNT]uint8
	EnvPeriodSamples, EnvSampleCounter            float64
	EnvContinue, EnvAlternate, EnvAttack          bool
	EnvHoldRequest, EnvHoldActive                 bool
	EventIndex                                    int
	CurrentSample, TotalSamples, LoopSample       uint64
	LoopEventIndex                                int
	Loop, Playing, Enabled                        bool
	PSGPlusEnabled, UseLegacyLinear, ChannelsInit bool
	Events                                        []PSGEvent
	SNEventIndex, SNLoopEventIndex                int
	SNEvents                                      []SNEvent
}

func (e *PSGEngine) DebugSnapshotName() string { return "psg-engine" }

func (e *PSGEngine) DebugSnapshot() (uint32, []byte, error) {
	if e == nil {
		return psgSnapshotVersion, nil, fmt.Errorf("nil PSG engine")
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return marshalSnapshot(psgSnapshotVersion, psgDebugSnapshot{
		SampleRate: e.sampleRate, ClockHz: e.clockHz, Regs: e.regs,
		EnvPeriodSamples: e.envPeriodSamples, EnvSampleCounter: e.envSampleCounter,
		EnvLevel: e.envLevel, EnvDirection: e.envDirection, EnvContinue: e.envContinue,
		EnvAlternate: e.envAlternate, EnvAttack: e.envAttack, EnvHoldRequest: e.envHoldRequest,
		EnvHoldActive: e.envHoldActive, EventIndex: e.eventIndex, CurrentSample: e.currentSample,
		TotalSamples: e.totalSamples, Loop: e.loop, LoopSample: e.loopSample, LoopEventIndex: e.loopEventIndex,
		Playing: e.playing, Enabled: e.enabled.Load(), PSGPlusEnabled: e.psgPlusEnabled,
		UseLegacyLinear: e.useLegacyLinear, ChannelsInit: e.channelsInit,
		Events:       append([]PSGEvent(nil), e.events...),
		SNEventIndex: e.snEventIndex, SNLoopEventIndex: e.snLoopEventIndex, SNEvents: append([]SNEvent(nil), e.snEvents...),
	})
}

func (e *PSGEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if e == nil {
		return fmt.Errorf("nil PSG engine")
	}
	if version != psgSnapshotVersion {
		return fmt.Errorf("unsupported PSG snapshot version %d", version)
	}
	var snap psgDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.sampleRate, e.clockHz, e.regs = snap.SampleRate, snap.ClockHz, snap.Regs
	e.envPeriodSamples, e.envSampleCounter = snap.EnvPeriodSamples, snap.EnvSampleCounter
	e.envLevel, e.envDirection = snap.EnvLevel, snap.EnvDirection
	e.envContinue, e.envAlternate, e.envAttack = snap.EnvContinue, snap.EnvAlternate, snap.EnvAttack
	e.envHoldRequest, e.envHoldActive = snap.EnvHoldRequest, snap.EnvHoldActive
	e.eventIndex, e.currentSample, e.totalSamples = snap.EventIndex, snap.CurrentSample, snap.TotalSamples
	e.loop, e.loopSample, e.loopEventIndex = snap.Loop, snap.LoopSample, snap.LoopEventIndex
	e.playing, e.psgPlusEnabled, e.useLegacyLinear, e.channelsInit = snap.Playing, snap.PSGPlusEnabled, snap.UseLegacyLinear, snap.ChannelsInit
	e.events = append(e.events[:0], snap.Events...)
	e.snEventIndex, e.snLoopEventIndex = snap.SNEventIndex, snap.SNLoopEventIndex
	e.snEvents = append(e.snEvents[:0], snap.SNEvents...)
	e.enabled.Store(snap.Enabled)
	e.syncToChip()
	return nil
}

type sidDebugSnapshot struct {
	Name           string
	SampleRate     int
	ClockHz        uint32
	Regs           [SID_REG_COUNT]uint8
	EventIndex     int
	CurrentSample  uint64
	TotalSamples   uint64
	Loop           bool
	LoopSample     uint64
	LoopEventIndex int
	Playing        bool
	DebugEnabled   bool
	DebugUntil     uint64
	DebugNextTick  uint64
	LastCtrl       [3]uint8
	LastAD         [3]uint8
	LastSR         [3]uint8
	VoiceGate      [3]bool
	VoiceWave      [3]bool
	Enabled        bool
	SIDPlusEnabled bool
	ChannelsInit   bool
	Model          int
	ForceLoop      bool
	BaseChannel    int
	RegBase        uint32
	RegEnd         uint32
	Events         []SIDEvent
}

func (e *SIDEngine) DebugSnapshotName() string {
	if e == nil {
		return "sid-engine"
	}
	switch e.regBase {
	case SID2_BASE:
		return "sid2-engine"
	case SID3_BASE:
		return "sid3-engine"
	default:
		return "sid-engine"
	}
}

func (e *SIDEngine) DebugSnapshot() (uint32, []byte, error) {
	if e == nil {
		return sidSnapshotVersion, nil, fmt.Errorf("nil SID engine")
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return marshalSnapshot(sidSnapshotVersion, sidDebugSnapshot{
		Name: e.DebugSnapshotName(), SampleRate: e.sampleRate, ClockHz: e.clockHz, Regs: e.regs,
		EventIndex: e.eventIndex, CurrentSample: e.currentSample, TotalSamples: e.totalSamples,
		Loop: e.loop, LoopSample: e.loopSample, LoopEventIndex: e.loopEventIndex, Playing: e.playing,
		DebugEnabled: e.debugEnabled, DebugUntil: e.debugUntil, DebugNextTick: e.debugNextTick,
		LastCtrl: e.lastCtrl, LastAD: e.lastAD, LastSR: e.lastSR, VoiceGate: e.voiceGate,
		VoiceWave: e.voiceWave, Enabled: e.enabled.Load(), SIDPlusEnabled: e.sidPlusEnabled,
		ChannelsInit: e.channelsInit, Model: e.model, ForceLoop: e.forceLoop,
		BaseChannel: e.baseChannel, RegBase: e.regBase, RegEnd: e.regEnd,
		Events: append([]SIDEvent(nil), e.events...),
	})
}

func (e *SIDEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if e == nil {
		return fmt.Errorf("nil SID engine")
	}
	if version != sidSnapshotVersion {
		return fmt.Errorf("unsupported SID snapshot version %d", version)
	}
	var snap sidDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	e.mutex.Lock()
	e.sampleRate, e.clockHz, e.regs = snap.SampleRate, snap.ClockHz, snap.Regs
	e.eventIndex, e.currentSample, e.totalSamples = snap.EventIndex, snap.CurrentSample, snap.TotalSamples
	e.loop, e.loopSample, e.loopEventIndex, e.playing = snap.Loop, snap.LoopSample, snap.LoopEventIndex, snap.Playing
	e.debugEnabled, e.debugUntil, e.debugNextTick = snap.DebugEnabled, snap.DebugUntil, snap.DebugNextTick
	e.lastCtrl, e.lastAD, e.lastSR = snap.LastCtrl, snap.LastAD, snap.LastSR
	e.voiceGate, e.voiceWave = snap.VoiceGate, snap.VoiceWave
	e.sidPlusEnabled, e.channelsInit, e.model, e.forceLoop = snap.SIDPlusEnabled, snap.ChannelsInit, snap.Model, snap.ForceLoop
	e.baseChannel, e.regBase, e.regEnd = snap.BaseChannel, snap.RegBase, snap.RegEnd
	e.events = append(e.events[:0], snap.Events...)
	e.enabled.Store(snap.Enabled)
	e.mutex.Unlock()
	e.syncToChip()
	return nil
}

type pokeyDebugSnapshot struct {
	SampleRate, BaseChannel                 int
	ClockHz, RandomSR                       uint32
	Regs                                    [POKEY_REG_COUNT]uint8
	POKEYPlusEnabled, ChannelsInit          bool
	Clock179MHz, Clock64KHz, Clock15KHz     float64
	EventIndex                              int
	CurrentSample, TotalSamples, LoopSample uint64
	Playing, Loop, ForceLoop                bool
	Events                                  []SAPPOKEYEvent
}

func (e *POKEYEngine) DebugSnapshotName() string {
	if e != nil && e.baseChannel != 0 {
		return fmt.Sprintf("pokey-engine-%d", e.baseChannel)
	}
	return "pokey-engine"
}

func (e *POKEYEngine) DebugSnapshot() (uint32, []byte, error) {
	if e == nil {
		return pokeySnapshotVersion, nil, fmt.Errorf("nil POKEY engine")
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return marshalSnapshot(pokeySnapshotVersion, pokeyDebugSnapshot{
		SampleRate: e.sampleRate, ClockHz: e.clockHz, BaseChannel: e.baseChannel,
		Regs: e.regs, POKEYPlusEnabled: e.pokeyPlusEnabled, ChannelsInit: e.channelsInit,
		RandomSR: e.randomSR, Clock179MHz: e.clock179MHz, Clock64KHz: e.clock64KHz, Clock15KHz: e.clock15KHz,
		EventIndex: e.eventIndex, CurrentSample: e.currentSample, TotalSamples: e.totalSamples,
		Playing: e.playing.Load(), Loop: e.loop, LoopSample: e.loopSample, ForceLoop: e.forceLoop,
		Events: append([]SAPPOKEYEvent(nil), e.events...),
	})
}

func (e *POKEYEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if e == nil {
		return fmt.Errorf("nil POKEY engine")
	}
	if version != pokeySnapshotVersion {
		return fmt.Errorf("unsupported POKEY snapshot version %d", version)
	}
	var snap pokeyDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	e.mutex.Lock()
	e.sampleRate, e.clockHz, e.baseChannel = snap.SampleRate, snap.ClockHz, snap.BaseChannel
	e.regs, e.pokeyPlusEnabled, e.channelsInit = snap.Regs, snap.POKEYPlusEnabled, snap.ChannelsInit
	e.randomSR, e.clock179MHz, e.clock64KHz, e.clock15KHz = snap.RandomSR, snap.Clock179MHz, snap.Clock64KHz, snap.Clock15KHz
	e.eventIndex, e.currentSample, e.totalSamples = snap.EventIndex, snap.CurrentSample, snap.TotalSamples
	e.loop, e.loopSample, e.forceLoop = snap.Loop, snap.LoopSample, snap.ForceLoop
	e.events = append(e.events[:0], snap.Events...)
	e.playing.Store(snap.Playing)
	state := e.snapshotSyncStateLocked()
	e.mutex.Unlock()
	applyPOKEYSyncState(state)
	return nil
}

type tedAudioDebugSnapshot struct {
	SampleRate, EventIndex, LoopEventIndex  int
	ClockHz                                 uint32
	Regs                                    [TED_REG_COUNT]uint8
	CurrentSample, TotalSamples, LoopSample uint64
	Loop, Playing, Enabled, TEDPlusEnabled  bool
	ChannelsInit                            bool
	SoundClock                              float64
	Events                                  []TEDEvent
}

func (e *TEDEngine) DebugSnapshotName() string { return "ted-audio-engine" }

func (e *TEDEngine) DebugSnapshot() (uint32, []byte, error) {
	if e == nil {
		return tedAudioSnapshotVersion, nil, fmt.Errorf("nil TED audio engine")
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return marshalSnapshot(tedAudioSnapshotVersion, tedAudioDebugSnapshot{
		SampleRate: e.sampleRate, ClockHz: e.clockHz, Regs: e.regs, EventIndex: e.eventIndex,
		CurrentSample: e.currentSample, TotalSamples: e.totalSamples, Loop: e.loop,
		LoopSample: e.loopSample, LoopEventIndex: e.loopEventIndex, Playing: e.playing,
		Enabled: e.enabled.Load(), TEDPlusEnabled: e.tedPlusEnabled, ChannelsInit: e.channelsInit,
		SoundClock: e.soundClock, Events: append([]TEDEvent(nil), e.events...),
	})
}

func (e *TEDEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if e == nil {
		return fmt.Errorf("nil TED audio engine")
	}
	if version != tedAudioSnapshotVersion {
		return fmt.Errorf("unsupported TED audio snapshot version %d", version)
	}
	var snap tedAudioDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.sampleRate, e.clockHz, e.regs = snap.SampleRate, snap.ClockHz, snap.Regs
	e.eventIndex, e.currentSample, e.totalSamples = snap.EventIndex, snap.CurrentSample, snap.TotalSamples
	e.loop, e.loopSample, e.loopEventIndex, e.playing = snap.Loop, snap.LoopSample, snap.LoopEventIndex, snap.Playing
	e.tedPlusEnabled, e.channelsInit, e.soundClock = snap.TEDPlusEnabled, snap.ChannelsInit, snap.SoundClock
	e.events = append(e.events[:0], snap.Events...)
	e.enabled.Store(snap.Enabled)
	e.syncToChip()
	return nil
}

func marshalSnapshot(version uint32, v any) (uint32, []byte, error) {
	data, err := json.Marshal(v)
	return version, data, err
}

type vgaDebugSnapshot struct {
	Layer                                        int
	Mode, Control, Status                        uint8
	DACWriteIndex, DACReadIndex                  uint8
	DACWritePhase, DACReadPhase, DACMask         uint8
	Palette                                      [VGA_PALETTE_SIZE * 3]uint8
	SeqIndex                                     uint8
	SeqRegs                                      [VGA_SEQ_REG_COUNT]uint8
	CRTCIndex                                    uint8
	CRTCRegs                                     [VGA_CRTC_REG_COUNT]uint8
	GCIndex                                      uint8
	GCRegs                                       [VGA_GC_REG_COUNT]uint8
	AttrIndex                                    uint8
	AttrRegs                                     [VGA_ATTR_REG_COUNT]uint8
	VRAM                                         [VGA_PLANE_COUNT][VGA_PLANE_SIZE]uint8
	TextBuffer                                   [VGA_TEXT_SIZE]uint8
	Latch                                        [4]uint8
	VSync                                        bool
	FrameStart                                   int64
	FrameCount                                   uint64
	Enabled                                      bool
	ScanlineFrame, FrameBuffer13h                []uint8
	FrameBuffer12h, FrameBufferX, FrameBufferTxt []uint8
	FrameBufs                                    [3][]byte
	WriteIdx, ReadingIdx                         int
	SharedIdx                                    int32
}

func (v *VGAEngine) DebugSnapshotName() string { return "vga-engine" }

func (v *VGAEngine) DebugSnapshot() (uint32, []byte, error) {
	if v == nil {
		return vgaSnapshotVersion, nil, fmt.Errorf("nil VGA engine")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return marshalSnapshot(vgaSnapshotVersion, vgaDebugSnapshot{
		Layer: v.layer, Mode: v.mode, Control: v.control, Status: v.status,
		DACWriteIndex: v.dacWriteIndex, DACReadIndex: v.dacReadIndex,
		DACWritePhase: v.dacWritePhase, DACReadPhase: v.dacReadPhase, DACMask: v.dacMask,
		Palette: v.palette, SeqIndex: v.seqIndex, SeqRegs: v.seqRegs,
		CRTCIndex: v.crtcIndex, CRTCRegs: v.crtcRegs, GCIndex: v.gcIndex, GCRegs: v.gcRegs,
		AttrIndex: v.attrIndex, AttrRegs: v.attrRegs, VRAM: v.vram, TextBuffer: v.textBuffer,
		Latch: v.latch, VSync: v.vsync.Load(), FrameStart: v.frameStart.Load(), FrameCount: v.frameCount.Load(),
		Enabled: v.enabled.Load(), ScanlineFrame: append([]uint8(nil), v.scanlineFrame...),
		FrameBuffer13h: append([]uint8(nil), v.frameBuffer13h...), FrameBuffer12h: append([]uint8(nil), v.frameBuffer12h...),
		FrameBufferX: append([]uint8(nil), v.frameBufferX...), FrameBufferTxt: append([]uint8(nil), v.frameBufferTxt...),
		FrameBufs: cloneByteSlices3(v.frameBufs), WriteIdx: v.writeIdx, SharedIdx: v.sharedIdx.Load(), ReadingIdx: v.readingIdx,
	})
}

func (v *VGAEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if v == nil {
		return fmt.Errorf("nil VGA engine")
	}
	if version != vgaSnapshotVersion {
		return fmt.Errorf("unsupported VGA snapshot version %d", version)
	}
	var snap vgaDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.layer, v.mode, v.control, v.status = snap.Layer, snap.Mode, snap.Control, snap.Status
	v.dacWriteIndex, v.dacReadIndex = snap.DACWriteIndex, snap.DACReadIndex
	v.dacWritePhase, v.dacReadPhase, v.dacMask = snap.DACWritePhase, snap.DACReadPhase, snap.DACMask
	v.dacMaskAtomic.Store(uint32(snap.DACMask))
	v.palette, v.seqIndex, v.seqRegs = snap.Palette, snap.SeqIndex, snap.SeqRegs
	v.crtcIndex, v.crtcRegs, v.gcIndex, v.gcRegs = snap.CRTCIndex, snap.CRTCRegs, snap.GCIndex, snap.GCRegs
	v.attrIndex, v.attrRegs, v.vram, v.textBuffer, v.latch = snap.AttrIndex, snap.AttrRegs, snap.VRAM, snap.TextBuffer, snap.Latch
	v.vsync.Store(snap.VSync)
	v.frameStart.Store(snap.FrameStart)
	v.frameCount.Store(snap.FrameCount)
	v.enabled.Store(snap.Enabled)
	v.scanlineFrame = append(v.scanlineFrame[:0], snap.ScanlineFrame...)
	v.frameBuffer13h = append(v.frameBuffer13h[:0], snap.FrameBuffer13h...)
	v.frameBuffer12h = append(v.frameBuffer12h[:0], snap.FrameBuffer12h...)
	v.frameBufferX = append(v.frameBufferX[:0], snap.FrameBufferX...)
	v.frameBufferTxt = append(v.frameBufferTxt[:0], snap.FrameBufferTxt...)
	restoreByteSlices3(&v.frameBufs, snap.FrameBufs)
	v.writeIdx, v.readingIdx = snap.WriteIdx, snap.ReadingIdx
	v.sharedIdx.Store(snap.SharedIdx)
	v.storeFullPaletteSnapshotLocked()
	return nil
}

type ulaDebugSnapshot struct {
	Border, Control                    uint8
	Enabled, VBlankActive, IRQAsserted bool
	VRAM                               [ULA_VRAM_SIZE]uint8
	AddrLatch                          uint16
	FlashState                         bool
	FlashCounter                       int32
	FrameBuffer                        []byte
	FrameBufs                          [3][]byte
	WriteIdx, ReadingIdx               int
	SharedIdx                          int32
	CompositorSnap                     ulaScanlineDebugSnapshot
	CompositorWriteIdx                 int
}

type ulaScanlineDebugSnapshot struct {
	VRAM       [ULA_VRAM_SIZE]uint8
	Border     uint8
	FlashState bool
	Target     []byte
}

func (u *ULAEngine) DebugSnapshotName() string { return "ula-engine" }

func (u *ULAEngine) DebugSnapshot() (uint32, []byte, error) {
	if u == nil {
		return ulaSnapshotVersion, nil, fmt.Errorf("nil ULA engine")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return marshalSnapshot(ulaSnapshotVersion, ulaDebugSnapshot{
		Border: u.border, Control: u.control, Enabled: u.enabled.Load(), VBlankActive: u.vblankActive.Load(),
		IRQAsserted: u.irqAsserted.Load(), VRAM: u.vram, AddrLatch: u.addrLatch,
		FlashState: u.flashState.Load(), FlashCounter: u.flashCounter.Load(),
		FrameBuffer: append([]byte(nil), u.frameBuffer...), FrameBufs: cloneByteSlices3(u.frameBufs),
		WriteIdx: u.writeIdx, SharedIdx: u.sharedIdx.Load(), ReadingIdx: u.readingIdx,
		CompositorSnap: snapshotULAScanline(u.compositorSnap), CompositorWriteIdx: u.compositorWriteIdx,
	})
}

func (u *ULAEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if u == nil {
		return fmt.Errorf("nil ULA engine")
	}
	if version != ulaSnapshotVersion {
		return fmt.Errorf("unsupported ULA snapshot version %d", version)
	}
	var snap ulaDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.border, u.control, u.vram, u.addrLatch = snap.Border, snap.Control, snap.VRAM, snap.AddrLatch
	u.enabled.Store(snap.Enabled)
	u.vblankActive.Store(snap.VBlankActive)
	u.irqAsserted.Store(snap.IRQAsserted)
	u.flashState.Store(snap.FlashState)
	u.flashCounter.Store(snap.FlashCounter)
	u.frameBuffer = append(u.frameBuffer[:0], snap.FrameBuffer...)
	restoreByteSlices3(&u.frameBufs, snap.FrameBufs)
	u.writeIdx, u.readingIdx = snap.WriteIdx, snap.ReadingIdx
	u.sharedIdx.Store(snap.SharedIdx)
	u.compositorSnap = restoreULAScanline(snap.CompositorSnap)
	u.compositorWriteIdx = snap.CompositorWriteIdx
	return nil
}

func cloneByteSlices3(in [3][]byte) [3][]byte {
	var out [3][]byte
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

func restoreByteSlices3(dst *[3][]byte, src [3][]byte) {
	for i := range src {
		(*dst)[i] = append((*dst)[i][:0], src[i]...)
	}
}

func snapshotULAScanline(in ulaScanlineSnapshot) ulaScanlineDebugSnapshot {
	return ulaScanlineDebugSnapshot{
		VRAM:       in.vram,
		Border:     in.border,
		FlashState: in.flashState,
		Target:     append([]byte(nil), in.target...),
	}
}

func restoreULAScanline(in ulaScanlineDebugSnapshot) ulaScanlineSnapshot {
	return ulaScanlineSnapshot{
		vram:       in.VRAM,
		border:     in.Border,
		flashState: in.FlashState,
		target:     append([]byte(nil), in.Target...),
	}
}

type tedVideoDebugSnapshot struct {
	Ctrl1, Ctrl2, CharBase, VideoBase uint8
	BGColor                           [4]uint8
	Border, CursorColor               uint8
	CursorPos                         uint16
	Enabled, VBlankActive             bool
	RasterLine, RasterCompare         uint16
	RasterComparePending              bool
	BaseFallbackCount                 uint64
	CursorVisible                     bool
	CursorCounter                     int
	VRAM, SnapVRAM                    [TED_V_VRAM_SIZE]uint8
	FrameBuffer                       []byte
	FrameBufs                         [3][]byte
	WriteIdx, ReadingIdx              int
	SharedIdx                         int32
}

func (t *TEDVideoEngine) DebugSnapshotName() string { return "ted-video-engine" }

func (t *TEDVideoEngine) DebugSnapshot() (uint32, []byte, error) {
	if t == nil {
		return tedVideoSnapshotVersion, nil, fmt.Errorf("nil TED video engine")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return marshalSnapshot(tedVideoSnapshotVersion, tedVideoDebugSnapshot{
		Ctrl1: t.ctrl1, Ctrl2: t.ctrl2, CharBase: t.charBase, VideoBase: t.videoBase,
		BGColor: t.bgColor, Border: t.border, CursorPos: t.cursorPos, CursorColor: t.cursorColor,
		Enabled: t.enabled.Load(), VBlankActive: t.vblankActive.Load(), RasterLine: t.rasterLine,
		RasterCompare: t.rasterCompare, RasterComparePending: t.rasterComparePending,
		BaseFallbackCount: t.baseFallbackCount, CursorVisible: t.cursorVisible, CursorCounter: t.cursorCounter,
		VRAM: t.vram, SnapVRAM: t.snapVram, FrameBuffer: append([]byte(nil), t.frameBuffer...),
		FrameBufs: cloneByteSlices3(t.frameBufs), WriteIdx: t.writeIdx, SharedIdx: t.sharedIdx.Load(), ReadingIdx: t.readingIdx,
	})
}

func (t *TEDVideoEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if t == nil {
		return fmt.Errorf("nil TED video engine")
	}
	if version != tedVideoSnapshotVersion {
		return fmt.Errorf("unsupported TED video snapshot version %d", version)
	}
	var snap tedVideoDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ctrl1, t.ctrl2, t.charBase, t.videoBase = snap.Ctrl1, snap.Ctrl2, snap.CharBase, snap.VideoBase
	t.bgColor, t.border, t.cursorPos, t.cursorColor = snap.BGColor, snap.Border, snap.CursorPos, snap.CursorColor
	t.enabled.Store(snap.Enabled)
	t.vblankActive.Store(snap.VBlankActive)
	t.rasterLine, t.rasterCompare = snap.RasterLine, snap.RasterCompare
	t.rasterComparePending, t.baseFallbackCount = snap.RasterComparePending, snap.BaseFallbackCount
	t.cursorVisible, t.cursorCounter = snap.CursorVisible, snap.CursorCounter
	t.vram, t.snapVram = snap.VRAM, snap.SnapVRAM
	t.frameBuffer = append(t.frameBuffer[:0], snap.FrameBuffer...)
	restoreByteSlices3(&t.frameBufs, snap.FrameBufs)
	t.writeIdx, t.readingIdx = snap.WriteIdx, snap.ReadingIdx
	t.sharedIdx.Store(snap.SharedIdx)
	return nil
}

type anticDebugSnapshot struct {
	DMACTL, CHACTL, DLISTL, DLISTH, HSCROL, VSCROL, PMBASE, CHBASE uint8
	NMIEN, NMIST, VCOUNT, PENH, PENV                               uint8
	Enabled, PALMode, VBlankActive                                 bool
	LastFrameStart                                                 int64
	FrameID                                                        uint64
	Scanline                                                       uint16
	COLPF                                                          [4]uint8
	COLBK                                                          uint8
	COLPM                                                          [4]uint8
	PRIOR, GRACTL, CONSOL                                          uint8
	HPOSP, HPOSM, SIZEP                                            [4]uint8
	SIZEM                                                          uint8
	GRAFP                                                          [4]uint8
	GRAFM                                                          uint8
	PlayerGfx                                                      [2][4][ANTIC_DISPLAY_HEIGHT]uint8
	PlayerPos                                                      [2][4][ANTIC_DISPLAY_HEIGHT]uint8
	MissileGfx                                                     [2][4][ANTIC_DISPLAY_HEIGHT]uint8
	MissilePos                                                     [2][4][ANTIC_DISPLAY_HEIGHT]uint8
	MissilePF, PlayerPF, MissilePL, PlayerPL                       [4]uint8
	ScanlineColors                                                 [2][ANTIC_SCANLINES_NTSC]uint8
	WriteBuffer                                                    int
	FrameReady                                                     bool
	FrameBufs                                                      [3][]byte
	WriteIdx, ReadingIdx                                           int
	SharedIdx                                                      int32
	ScanlineCursor, ScanlineWriteIdx                               int
	ScanlinePass                                                   anticScanlinePassSnapshot
}

type anticScanlinePassSnapshot struct {
	Target     []byte
	PFMask     []uint8
	PMG        pmgDebugSnapshot
	PC         uint16
	ScreenAddr uint16
	DisplayY   int
	Entries    int
	Entry      DisplayListEntry
	EntryValid bool
	EntryLine  int
	Stopped    bool
}

type pmgDebugSnapshot struct {
	GRACTL     uint8
	PRIOR      uint8
	SIZEP      [4]uint8
	SIZEM      uint8
	COLPM      [4]uint8
	COLPF      [4]uint8
	PlayerGfx  [4][ANTIC_DISPLAY_HEIGHT]uint8
	PlayerPos  [4][ANTIC_DISPLAY_HEIGHT]uint8
	MissileGfx [4][ANTIC_DISPLAY_HEIGHT]uint8
	MissilePos [4][ANTIC_DISPLAY_HEIGHT]uint8
	PFMask     []uint8
}

func (a *ANTICEngine) DebugSnapshotName() string { return "antic-engine" }

func (a *ANTICEngine) DebugSnapshot() (uint32, []byte, error) {
	if a == nil {
		return anticSnapshotVersion, nil, fmt.Errorf("nil ANTIC engine")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return marshalSnapshot(anticSnapshotVersion, anticDebugSnapshot{
		DMACTL: a.dmactl, CHACTL: a.chactl, DLISTL: a.dlistl, DLISTH: a.dlisth,
		HSCROL: a.hscrol, VSCROL: a.vscrol, PMBASE: a.pmbase, CHBASE: a.chbase,
		NMIEN: a.nmien, NMIST: a.nmist, VCOUNT: a.vcount, Scanline: a.scanline,
		PENH: a.penh, PENV: a.penv, Enabled: a.enabled.Load(), PALMode: a.palMode.Load(),
		VBlankActive: a.vblankActive.Load(), LastFrameStart: a.lastFrameStart, FrameID: a.frameID,
		COLPF: a.colpf, COLBK: a.colbk, COLPM: a.colpm, PRIOR: a.prior, GRACTL: a.gractl, CONSOL: a.consol,
		HPOSP: a.hposp, HPOSM: a.hposm, SIZEP: a.sizep, SIZEM: a.sizem, GRAFP: a.grafp, GRAFM: a.grafm,
		PlayerGfx: a.playerGfx, PlayerPos: a.playerPos, MissileGfx: a.missileGfx, MissilePos: a.missilePos,
		MissilePF: a.missilePF, PlayerPF: a.playerPF, MissilePL: a.missilePL, PlayerPL: a.playerPL,
		ScanlineColors: a.scanlineColors, WriteBuffer: a.writeBuffer, FrameReady: a.frameReady,
		FrameBufs: cloneByteSlices3(a.frameBufs), WriteIdx: a.writeIdx, SharedIdx: a.sharedIdx.Load(), ReadingIdx: a.readingIdx,
		ScanlineCursor: a.scanlineCursor, ScanlineWriteIdx: a.scanlineWriteIdx, ScanlinePass: snapshotANTICPass(a.scanlinePass),
	})
}

func (a *ANTICEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if a == nil {
		return fmt.Errorf("nil ANTIC engine")
	}
	if version != anticSnapshotVersion {
		return fmt.Errorf("unsupported ANTIC snapshot version %d", version)
	}
	var snap anticDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dmactl, a.chactl, a.dlistl, a.dlisth = snap.DMACTL, snap.CHACTL, snap.DLISTL, snap.DLISTH
	a.hscrol, a.vscrol, a.pmbase, a.chbase = snap.HSCROL, snap.VSCROL, snap.PMBASE, snap.CHBASE
	a.nmien, a.nmist, a.vcount, a.scanline = snap.NMIEN, snap.NMIST, snap.VCOUNT, snap.Scanline
	a.penh, a.penv, a.lastFrameStart, a.frameID = snap.PENH, snap.PENV, snap.LastFrameStart, snap.FrameID
	a.enabled.Store(snap.Enabled)
	a.palMode.Store(snap.PALMode)
	a.vblankActive.Store(snap.VBlankActive)
	a.colpf, a.colbk, a.colpm = snap.COLPF, snap.COLBK, snap.COLPM
	a.prior, a.gractl, a.consol = snap.PRIOR, snap.GRACTL, snap.CONSOL
	a.hposp, a.hposm, a.sizep, a.sizem = snap.HPOSP, snap.HPOSM, snap.SIZEP, snap.SIZEM
	a.grafp, a.grafm = snap.GRAFP, snap.GRAFM
	a.playerGfx, a.playerPos, a.missileGfx, a.missilePos = snap.PlayerGfx, snap.PlayerPos, snap.MissileGfx, snap.MissilePos
	a.missilePF, a.playerPF, a.missilePL, a.playerPL = snap.MissilePF, snap.PlayerPF, snap.MissilePL, snap.PlayerPL
	a.scanlineColors, a.writeBuffer, a.frameReady = snap.ScanlineColors, snap.WriteBuffer, snap.FrameReady
	restoreByteSlices3(&a.frameBufs, snap.FrameBufs)
	a.writeIdx, a.readingIdx = snap.WriteIdx, snap.ReadingIdx
	a.sharedIdx.Store(snap.SharedIdx)
	a.scanlineCursor, a.scanlineWriteIdx = snap.ScanlineCursor, snap.ScanlineWriteIdx
	a.scanlinePass = restoreANTICPass(snap.ScanlinePass)
	return nil
}

func snapshotANTICPass(in anticScanlinePass) anticScanlinePassSnapshot {
	return anticScanlinePassSnapshot{
		Target: append([]byte(nil), in.target...), PFMask: append([]uint8(nil), in.pfMask...),
		PMG: snapshotPMG(in.pmg), PC: in.pc, ScreenAddr: in.screenAddr, DisplayY: in.displayY,
		Entries: in.entries, Entry: in.entry, EntryValid: in.entryValid, EntryLine: in.entryLine, Stopped: in.stopped,
	}
}

func restoreANTICPass(in anticScanlinePassSnapshot) anticScanlinePass {
	return anticScanlinePass{
		target: append([]byte(nil), in.Target...), pfMask: append([]uint8(nil), in.PFMask...),
		pmg: restorePMG(in.PMG), pc: in.PC, screenAddr: in.ScreenAddr, displayY: in.DisplayY,
		entries: in.Entries, entry: in.Entry, entryValid: in.EntryValid, entryLine: in.EntryLine, stopped: in.Stopped,
	}
}

func snapshotPMG(in pmgSnapshot) pmgDebugSnapshot {
	return pmgDebugSnapshot{
		GRACTL: in.gractl, PRIOR: in.prior, SIZEP: in.sizep, SIZEM: in.sizem,
		COLPM: in.colpm, COLPF: in.colpf, PlayerGfx: in.playerGfx, PlayerPos: in.playerPos,
		MissileGfx: in.missileGfx, MissilePos: in.missilePos, PFMask: append([]uint8(nil), in.pfMask...),
	}
}

func restorePMG(in pmgDebugSnapshot) pmgSnapshot {
	return pmgSnapshot{
		gractl: in.GRACTL, prior: in.PRIOR, sizep: in.SIZEP, sizem: in.SIZEM,
		colpm: in.COLPM, colpf: in.COLPF, playerGfx: in.PlayerGfx, playerPos: in.PlayerPos,
		missileGfx: in.MissileGfx, missilePos: in.MissilePos, pfMask: append([]uint8(nil), in.PFMask...),
	}
}

type voodooDebugSnapshot struct {
	Width, Height                            int32
	Layer                                    int
	Enabled                                  bool
	Regs                                     []uint32
	CurrentVertex                            VoodooVertex
	VertexIndex                              int
	Vertices, VertexColors                   [3]VoodooVertex
	CurrentColorTarget                       int
	GouraudEnabled                           bool
	TriangleBatch                            []VoodooTriangle
	FBZMode, AlphaMode, FBZColorPath         uint32
	TextureMode, FogMode, LFBMode            uint32
	TLOD                                     uint32
	TexBase                                  [9]uint32
	Stipple, ChromaRange                     uint32
	Slopes                                   VoodooSlopes
	SlopesValid, PipelineDirty               bool
	ClipLeft, ClipRight, ClipTop, ClipBottom int
	Color0, Color1, FogColor, ZAColor        uint32
	ChromaKey                                uint32
	Busy, SwapPending                        bool
	VRetrace                                 int64
	FrameBufs                                [3][]byte
	SharedIdx, ReadingIdx                    int32
	WriteIdx                                 int
	TextureMemory                            []byte
	TextureWidth, TextureHeight              int
}

func (v *VoodooEngine) DebugSnapshotName() string { return "voodoo-engine" }

func (v *VoodooEngine) DebugSnapshot() (uint32, []byte, error) {
	if v == nil {
		return voodooSnapshotVersion, nil, fmt.Errorf("nil Voodoo engine")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return marshalSnapshot(voodooSnapshotVersion, voodooDebugSnapshot{
		Width: v.width.Load(), Height: v.height.Load(), Layer: v.layer, Enabled: v.enabled.Load(),
		Regs: append([]uint32(nil), v.regs...), CurrentVertex: v.currentVertex, VertexIndex: v.vertexIndex,
		Vertices: v.vertices, VertexColors: v.vertexColors, CurrentColorTarget: v.currentColorTarget,
		GouraudEnabled: v.gouraudEnabled, TriangleBatch: append([]VoodooTriangle(nil), v.triangleBatch...),
		FBZMode: v.fbzMode, AlphaMode: v.alphaMode, FBZColorPath: v.fbzColorPath,
		TextureMode: v.textureMode, FogMode: v.fogMode, LFBMode: v.lfbMode, TLOD: v.tlod,
		TexBase: v.texBase, Stipple: v.stipple, ChromaRange: v.chromaRange,
		Slopes: v.slopes, SlopesValid: v.slopesValid, PipelineDirty: v.pipelineDirty,
		ClipLeft: v.clipLeft, ClipRight: v.clipRight, ClipTop: v.clipTop, ClipBottom: v.clipBottom,
		Color0: v.color0, Color1: v.color1, FogColor: v.fogColor, ZAColor: v.zaColor, ChromaKey: v.chromaKey,
		Busy: v.busy, SwapPending: v.swapPending, VRetrace: v.vretrace.Load(),
		FrameBufs: cloneByteSlices3(v.frameBufs), SharedIdx: v.sharedIdx.Load(), ReadingIdx: v.readingIdx.Load(), WriteIdx: v.writeIdx,
		TextureMemory: append([]byte(nil), v.textureMemory...), TextureWidth: v.textureWidth, TextureHeight: v.textureHeight,
	})
}

func (v *VoodooEngine) DebugRestoreSnapshot(version uint32, data []byte) error {
	if v == nil {
		return fmt.Errorf("nil Voodoo engine")
	}
	if version != voodooSnapshotVersion {
		return fmt.Errorf("unsupported Voodoo snapshot version %d", version)
	}
	var snap voodooDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.width.Store(snap.Width)
	v.height.Store(snap.Height)
	v.layer = snap.Layer
	v.enabled.Store(snap.Enabled)
	v.regs = append(v.regs[:0], snap.Regs...)
	v.currentVertex, v.vertexIndex, v.vertices = snap.CurrentVertex, snap.VertexIndex, snap.Vertices
	v.vertexColors, v.currentColorTarget, v.gouraudEnabled = snap.VertexColors, snap.CurrentColorTarget, snap.GouraudEnabled
	v.triangleBatch = append(v.triangleBatch[:0], snap.TriangleBatch...)
	v.fbzMode, v.alphaMode, v.fbzColorPath = snap.FBZMode, snap.AlphaMode, snap.FBZColorPath
	v.textureMode, v.fogMode, v.lfbMode, v.tlod = snap.TextureMode, snap.FogMode, snap.LFBMode, snap.TLOD
	v.texBase, v.stipple, v.chromaRange = snap.TexBase, snap.Stipple, snap.ChromaRange
	v.slopes, v.slopesValid, v.pipelineDirty = snap.Slopes, snap.SlopesValid, snap.PipelineDirty
	v.clipLeft, v.clipRight, v.clipTop, v.clipBottom = snap.ClipLeft, snap.ClipRight, snap.ClipTop, snap.ClipBottom
	v.color0, v.color1, v.fogColor, v.zaColor, v.chromaKey = snap.Color0, snap.Color1, snap.FogColor, snap.ZAColor, snap.ChromaKey
	v.busy, v.swapPending = snap.Busy, snap.SwapPending
	v.vretrace.Store(snap.VRetrace)
	restoreByteSlices3(&v.frameBufs, snap.FrameBufs)
	v.sharedIdx.Store(snap.SharedIdx)
	v.readingIdx.Store(snap.ReadingIdx)
	v.writeIdx = snap.WriteIdx
	v.textureMemory = append(v.textureMemory[:0], snap.TextureMemory...)
	v.textureWidth, v.textureHeight = snap.TextureWidth, snap.TextureHeight
	return nil
}
