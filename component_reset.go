// component_reset.go - Reset() methods for all hardware components (hard reset support)

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "sync"

// SoundChip.Reset restores audio to constructor defaults. Preserves OTO output.
func (chip *SoundChip) Reset() {
	chip.mu.Lock()
	defer chip.mu.Unlock()

	chip.filterLP = DEFAULT_FILTER_LP
	chip.filterBP = DEFAULT_FILTER_BP
	chip.filterHP = DEFAULT_FILTER_HP
	chip.filterCutoff = 0
	chip.filterResonance = 0
	chip.filterModAmount = 0
	chip.overdriveLevel = 0
	chip.overdriveGain = 0
	chip.reverbMix = 0
	chip.sidMixerDCOffset = 0
	chip.filterType = 0
	chip.sidMixerEnabled = false
	chip.sidMixerSaturate = false

	// Reset channels to constructor defaults
	waveTypes := [NUM_CHANNELS]int{
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
	}
	for i, ch := range chip.channels {
		if ch == nil {
			continue
		}
		ch.waveType = waveTypes[i]
		ch.frequency = 0
		ch.volume = MIN_VOLUME
		ch.phase = MIN_PHASE
		ch.enabled = false
		ch.gate = false
		ch.attackTime = DEFAULT_ATTACK_TIME
		ch.decayTime = DEFAULT_DECAY_TIME
		ch.sustainLevel = DEFAULT_SUSTAIN
		ch.releaseTime = DEFAULT_RELEASE_TIME
		ch.attackRecip = 0
		ch.decayRecip = 0
		ch.releaseRecip = 0
		ch.releaseDecay = 0
		ch.envelopePhase = ENV_ATTACK
		ch.envelopeLevel = 0
		ch.envelopeSample = 0
		ch.dutyCycle = DEFAULT_DUTY_CYCLE
		ch.noiseSR = NOISE_LFSR_SEED
		ch.noiseMode = 0
		ch.noisePhase = 0
		ch.noiseValue = 0
		ch.noiseFilter = 0
		ch.noiseFilterState = 0
		ch.dacMode = false
		ch.dacValue = 0
		ch.pwmEnabled = false
		ch.pwmRate = 0
		ch.pwmDepth = 0
		ch.pwmPhase = 0
		ch.prevRawSample = 0
		ch.phaseWrapped = false
		ch.phaseMSB = false
		ch.sweepEnabled = false
		ch.sweepDirection = false
		ch.sweepPeriod = 0
		ch.sweepCounter = 0
		ch.sweepShift = 0
		ch.syncSource = nil
		ch.ringModSource = nil
		ch.sidEnvelope = false
		ch.sidTestBit = false
		ch.sidFilterMode = false
		ch.sidRateCounter = false
		ch.sidDACEnabled = false
		ch.sidADSRBugsEnabled = false
		ch.sidNoisePhaseLocked = false
		ch.sid6581FilterDistort = false
		ch.sidEnvLevel = 0
		ch.sidADSRDelayCounter = 0
		ch.sidCycleAccum = 0
		ch.sidExpIndex = 0
		ch.sidAttackIndex = 0
		ch.sidDecayIndex = 0
		ch.sidWaveMask = 0
		ch.sidMixLowpassState = 0
		ch.sidOscOutput = 0
		ch.filterLP = 0
		ch.filterBP = 0
		ch.filterHP = 0
		ch.filterCutoff = 0
		ch.filterResonance = 0
		ch.filterCutoffTarget = 0
		ch.filterResonanceTarget = 0
		ch.filterType = 0
		ch.filterModeMask = 0
		ch.psgPlusEnabled = false
		ch.psgPlusGain = 1.0
		ch.psgPlusOversample = 1
		ch.psgPlusDrive = 0
		ch.psgPlusRoomMix = 0
		ch.psgPlusBqZ1 = 0
		ch.psgPlusBqZ2 = 0
		ch.psgPlusTransGain = 0
		ch.psgPlusTransStep = 0
		ch.psgPlusTransCounter = 0
		ch.pokeyPlusEnabled = false
		ch.pokeyPlusGain = 1.0
		ch.pokeyPlusOversample = 1
		ch.pokeyPlusDrive = 0
		ch.pokeyPlusRoomMix = 0
		ch.pokeyPlusBqZ1 = 0
		ch.pokeyPlusBqZ2 = 0
		ch.pokeyPlusTransGain = 0
		ch.pokeyPlusTransStep = 0
		ch.pokeyPlusTransCounter = 0
		ch.sidPlusEnabled = false
		ch.sidPlusGain = 1.0
		ch.sidPlusDrive = 0
		ch.sidPlusRoomMix = 0
		ch.sidPlusBqZ1 = 0
		ch.sidPlusBqZ2 = 0
		ch.sidPlusTransGain = 0
		ch.sidPlusTransStep = 0
		ch.sidPlusTransCounter = 0
		ch.tedPlusEnabled = false
		ch.tedPlusGain = 1.0
		ch.tedPlusDrive = 0
		ch.tedPlusRoomMix = 0
		ch.tedPlusBqZ1 = 0
		ch.tedPlusBqZ2 = 0
		ch.tedPlusTransGain = 0
		ch.tedPlusTransStep = 0
		ch.tedPlusTransCounter = 0
		ch.ahxPlusEnabled = false
		ch.ahxPlusGain = 1.0
		ch.ahxPlusDrive = 0
		ch.ahxPlusRoomMix = 0
		ch.ahxPlusBqZ1 = 0
		ch.ahxPlusBqZ2 = 0
		ch.ahxPlusTransGain = 0
		ch.ahxPlusTransStep = 0
		ch.ahxPlusTransCounter = 0
		ch.ahxPlusPan = 0
		ch.plusBqB0 = 0
		ch.plusBqB1 = 0
		ch.plusBqB2 = 0
		ch.plusBqA1 = 0
		ch.plusBqA2 = 0
	}
	for i := range chip.snVoices {
		chip.initSNVoice(&chip.snVoices[i], i)
	}

	// Reset reverb buffers in-place
	for i := range chip.preDelayBuf {
		chip.preDelayBuf[i] = 0
	}
	chip.preDelayPos = 0
	for i := range chip.combFilters {
		for j := range chip.combFilters[i].buffer {
			chip.combFilters[i].buffer[j] = 0
		}
		chip.combFilters[i].pos = 0
	}
	for i := range chip.allpassBuf {
		for j := range chip.allpassBuf[i] {
			chip.allpassBuf[i][j] = 0
		}
	}
	for i := range chip.allpassPos {
		chip.allpassPos[i] = 0
	}

	// Clear byte accumulation shadow buffer
	chip.flexShadow = [4 * FLEX_CH_STRIDE]byte{}

	// Preserve registered sample tickers across reset; engines re-establish
	// their internal state and should keep advancing afterwards.
	chip.sampleTap.Store(&sampleTapHolder{})
	chip.resetMasterNormalizerLocked()

	chip.enabled.Store(false)
	chip.audioFrozen.Store(false)
}

// PSGEngine.Reset clears all registers and envelope state.
func (e *PSGEngine) Reset() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range e.regs {
		e.regs[i] = 0
	}
	e.envLevel = 15
	e.envDirection = -1
	e.envContinue = false
	e.envAlternate = false
	e.envAttack = false
	e.envHoldRequest = false
	e.envHoldActive = false
	e.envSampleCounter = 0

	e.events = nil
	e.eventIndex = 0
	e.snEvents = nil
	e.snEventIndex = 0
	e.snLoopEventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
	e.playing = false
	e.enabled.Store(false)
	e.channelsInit = false
	e.silenceChannels()
	e.updateEnvPeriodSamples()
}

// VideoChip.Reset restores video chip to cold boot state.
// Preserves: output, layer, bus, busMemory, splashBuffer, bigEndianMode, directVRAM, onResolutionChange.
func (vc *VideoChip) Reset() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.currentMode = DEFAULT_VIDEO_MODE
	vc.frameCounter = 0
	vc.enabled.Store(false)
	vc.hasContent.Store(false)
	vc.inVBlank.Store(false)
	vc.directMode.Store(false)
	vc.fullScreenDirty.Store(false)
	vc.copperEnabled = false
	vc.copperManagedByCompositor = false
	vc.bigEndianMode = false
	vc.directVRAM = nil

	mode := VideoModes[vc.currentMode]
	if len(vc.frontBuffer) != mode.totalSize {
		vc.frontBuffer = make([]byte, mode.totalSize)
		vc.backBuffer = make([]byte, mode.totalSize)
	} else {
		clear(vc.frontBuffer)
		clear(vc.backBuffer)
	}
	clear(vc.prevVRAM)
	vc.initialiseDirtyGrid(mode)

	// Reset copper state
	vc.copperPC = 0
	vc.copperWaiting = false
	vc.copperHalted = false
	vc.copperWaitX = 0
	vc.copperWaitY = 0

	// Reset blitter state
	vc.bltBusy = false

	// Reset CLUT8 palette state
	vc.clutMode.Store(false)
	clear(vc.clutPalette[:])
	clear(vc.clutPaletteHW[:])
	vc.palIndex = 0
	vc.fbBase = 0
	vc.clutWarnOnce = sync.Once{}
}

// VGAEngine.Reset restores VGA to power-on defaults.
func (vga *VGAEngine) Reset() {
	vga.mu.Lock()
	defer vga.mu.Unlock()

	vga.mode = 0
	vga.control = 0
	vga.status = 0

	// Reset DAC
	vga.dacWriteIndex = 0
	vga.dacReadIndex = 0
	vga.dacWritePhase = 0
	vga.dacReadPhase = 0
	vga.dacMask = 0xFF

	// Reset sequencer
	vga.seqIndex = 0
	for i := range vga.seqRegs {
		vga.seqRegs[i] = 0
	}
	vga.seqRegs[VGA_SEQ_MAPMASK_R] = 0x0F
	vga.seqRegs[VGA_SEQ_MEMMODE] = VGA_SEQ_MEMMODE_CHAIN4

	// Reset CRTC
	vga.crtcIndex = 0
	for i := range vga.crtcRegs {
		vga.crtcRegs[i] = 0
	}

	// Reset GC
	vga.gcIndex = 0
	for i := range vga.gcRegs {
		vga.gcRegs[i] = 0
	}
	vga.gcRegs[VGA_GC_BITMASK_R] = 0xFF

	// Reset attribute controller
	vga.attrIndex = 0
	for i := range vga.attrRegs {
		vga.attrRegs[i] = 0
	}
	vga.initDefaultAttrRegs()

	// Reinitialize default palette
	vga.initDefaultPalette()
	vga.dacMask = 0xFF
	vga.dacMaskAtomic.Store(0xFF)
	vga.storeFullPaletteSnapshotLocked()

	// Clear VRAM planes
	for i := range vga.vram {
		for j := range vga.vram[i] {
			vga.vram[i][j] = 0
		}
	}

	// Clear text buffer
	for i := range vga.textBuffer {
		vga.textBuffer[i] = 0
	}

	// Clear frame buffers
	for i := range vga.frameBuffer13h {
		vga.frameBuffer13h[i] = 0
	}
	for i := range vga.frameBuffer12h {
		vga.frameBuffer12h[i] = 0
	}
	for i := range vga.frameBufferX {
		vga.frameBufferX[i] = 0
	}
	for i := range vga.frameBufferTxt {
		vga.frameBufferTxt[i] = 0
	}

	// Reset read latches
	for i := range vga.latch {
		vga.latch[i] = 0
	}

	// Reset atomic state
	vga.enabled.Store(false)
	vga.vsync.Store(false)

	// Reset triple-buffer
	for i := range vga.frameBufs {
		for j := range vga.frameBufs[i] {
			vga.frameBufs[i][j] = 0
		}
	}
	vga.writeIdx = 0
	vga.sharedIdx.Store(1)
	vga.readingIdx = 2
}

// ULAEngine.Reset restores ULA to cold boot state.
func (ula *ULAEngine) Reset() {
	ula.mu.Lock()
	defer ula.mu.Unlock()

	ula.border = 0
	ula.control = 0
	ula.enabled.Store(false)
	ula.vblankActive.Store(false)

	for i := range ula.vram {
		ula.vram[i] = 0
	}
	ula.flashState = false
	ula.flashCounter = 0

	for i := range ula.frameBuffer {
		ula.frameBuffer[i] = 0
	}

	// Reset triple-buffer
	for i := range ula.frameBufs {
		for j := range ula.frameBufs[i] {
			ula.frameBufs[i][j] = 0
		}
	}
	ula.writeIdx = 0
	ula.sharedIdx.Store(1)
	ula.readingIdx = 2
}

// TEDVideoEngine.Reset restores TED video to cold boot state.
func (ted *TEDVideoEngine) Reset() {
	ted.mu.Lock()
	defer ted.mu.Unlock()

	ted.ctrl1 = 0
	ted.ctrl2 = 0
	ted.charBase = 0
	ted.videoBase = 0
	for i := range ted.bgColor {
		ted.bgColor[i] = 0
	}
	ted.border = 0
	ted.cursorPos = 0
	ted.cursorColor = 0
	ted.enabled.Store(false)
	ted.vblankActive.Store(false)
	ted.rasterLine = 0
	ted.cursorVisible = true
	ted.cursorCounter = 0

	for i := range ted.vram {
		ted.vram[i] = 0
	}
	// Reload default character set
	charsetOffset := TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE
	for i := 0; i < 256 && i < len(TEDDefaultCharset); i++ {
		for j := range 8 {
			ted.vram[charsetOffset+i*8+j] = TEDDefaultCharset[i][j]
		}
	}

	for i := range ted.frameBuffer {
		ted.frameBuffer[i] = 0
	}

	// Reset triple-buffer
	for i := range ted.frameBufs {
		for j := range ted.frameBufs[i] {
			ted.frameBufs[i][j] = 0
		}
	}
	ted.writeIdx = 0
	ted.sharedIdx.Store(1)
	ted.readingIdx = 2
}

// ANTICEngine.Reset restores ANTIC/GTIA to cold boot state.
func (a *ANTICEngine) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.dmactl = 0
	a.chactl = 0
	a.dlistl = 0
	a.dlisth = 0
	a.hscrol = 0
	a.vscrol = 0
	a.pmbase = 0
	a.chbase = 0
	a.nmien = 0
	a.nmist = 0
	a.vcount = 0
	a.scanline = 0
	a.penh = 0
	a.penv = 0

	// GTIA defaults
	a.prior = 0
	a.gractl = 0
	a.consol = 0x07
	for i := range a.colpf {
		a.colpf[i] = 0
	}
	a.colbk = 0
	for i := range a.colpm {
		a.colpm[i] = 0
	}
	for i := range a.hposp {
		a.hposp[i] = 0
	}
	for i := range a.hposm {
		a.hposm[i] = 0
	}
	for i := range a.sizep {
		a.sizep[i] = 0
	}
	a.sizem = 0
	for i := range a.grafp {
		a.grafp[i] = 0
	}
	a.grafm = 0

	a.enabled.Store(false)
	a.vblankActive.Store(false)
	a.writeBuffer = 0
	a.frameReady = false

	for i := range a.frameBuffer {
		a.frameBuffer[i] = 0
	}

	// Reset triple-buffer
	for i := range a.frameBufs {
		for j := range a.frameBufs[i] {
			a.frameBufs[i][j] = 0
		}
	}
	a.writeIdx = 0
	a.sharedIdx.Store(1)
	a.readingIdx = 2
}

// VoodooEngine.Reset restores Voodoo to power-on defaults.
func (v *VoodooEngine) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.enabled.Store(false)
	v.width.Store(VOODOO_DEFAULT_WIDTH)
	v.height.Store(VOODOO_DEFAULT_HEIGHT)

	for i := range v.regs {
		v.regs[i] = 0
	}

	v.currentVertex = VoodooVertex{}
	v.vertexIndex = 0
	v.vertices = [3]VoodooVertex{}
	v.vertexColors = [3]VoodooVertex{}
	v.currentColorTarget = 0
	v.gouraudEnabled = false
	v.triangleBatch = v.triangleBatch[:0]
	v.fbzColorPath = 0
	v.textureMode = 0
	v.fogMode = 0
	v.lfbMode = 0
	v.tlod = 0
	v.texBase = [9]uint32{}
	v.stipple = 0
	v.chromaRange = 0
	v.slopes = VoodooSlopes{}
	v.slopesValid = false
	v.pipelineDirty = false

	v.clipLeft = 0
	v.clipRight = VOODOO_DEFAULT_WIDTH
	v.clipTop = 0
	v.clipBottom = VOODOO_DEFAULT_HEIGHT

	// Reset texture memory
	for i := range v.textureMemory {
		v.textureMemory[i] = 0
	}
	v.textureWidth = 0
	v.textureHeight = 0
	v.busy = false
	v.swapPending = false
	v.vretrace.Store(0)

	// Reset triple-buffer
	defW := int(VOODOO_DEFAULT_WIDTH)
	defH := int(VOODOO_DEFAULT_HEIGHT)
	bufSize := defW * defH * BYTES_PER_PIXEL
	for i := range v.frameBufs {
		if len(v.frameBufs[i]) != bufSize {
			v.frameBufs[i] = make([]byte, bufSize)
		} else {
			for j := range v.frameBufs[i] {
				v.frameBufs[i][j] = 0
			}
		}
	}
	v.writeIdx = 0
	v.sharedIdx.Store(1)
	v.readingIdx.Store(2)

	v.initDefaultState()
	if v.backend != nil {
		v.backend.Reset()
	}
}

// TerminalMMIO.Reset clears all buffers and restores defaults.
func (t *TerminalMMIO) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inputHead = 0
	t.inputTail = 0
	t.inputLen = 0
	t.newlines = 0
	t.outputBuf = t.outputBuf[:0]
	t.echoEnabled = true
	t.lineInputMode = true
	t.rawKeyHead = 0
	t.rawKeyTail = 0
	t.rawKeyLen = 0
	t.mouseX.Store(0)
	t.mouseY.Store(0)
	t.mouseButtons.Store(0)
	t.mouseChanged.Store(false)
	t.scanHead = 0
	t.scanTail = 0
	t.scanLen = 0
	t.modifiers.Store(0)
	t.SentinelTriggered.Store(false)
}

// VideoTerminal.Reset re-initializes the terminal display.
func (vt *VideoTerminal) Reset() {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	vt.cursorOn = true
	vt.fgColor = 0xFFFFFFFF
	vt.bgColor = 0xFFAA5500
	vt.escState = 0
	vt.escParam = 0
	vt.inputEscState = 0
	vt.inputEscParam = 0
	vt.inputEscParam2 = 0
	vt.inputActive = false
	vt.inputStartCol = 0
	vt.inputStartRow = 0
	vt.historyIdx = len(vt.history)
	vt.savedInput = ""

	if vt.screen != nil {
		vt.screen.Clear()
	}
	vt.clearScreenLocked()
}

// CoprocessorManager.Reset stops all workers and clears state.
func (m *CoprocessorManager) Reset() {
	m.StopAll()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextTicket = 1
	m.completions = make(map[uint32]*CoprocCompletion)

	m.cmd = 0
	m.cpuType = 0
	m.cmdStatus = 0
	m.cmdError = 0
	m.ticket = 0
	m.ticketStatus = 0
	m.op = 0
	m.reqPtr = 0
	m.reqLen = 0
	m.respPtr = 0
	m.respCap = 0
	m.timeout = 0
	m.namePtr = 0
	m.workerState = 0
	m.opsDispatched = 0
	m.bytesProcessed = 0
	m.completionIRQEnabled.Store(false)
	m.completedTicket.Store(0)
	m.dispatchOverheadNs.Store(0)
}

// PSGPlayer.Reset stops playback and clears metadata.
func (p *PSGPlayer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metadata = PSGMetadata{}
	p.frameRate = 0
	p.clockHz = 0
	p.loopSample = 0
	p.loop = false
	p.playPtrStaged = 0
	p.playLenStaged = 0
	p.playPtr = 0
	p.playLen = 0
	p.playBusy = false
	p.playErr = false
	p.forceLoop = false
	p.playGen = 0
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
}

// SIDPlayer.Reset stops playback and clears metadata.
func (p *SIDPlayer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metadata = SIDMetadata{}
	p.clockHz = 0
	p.loop = false
	p.playPtrStaged = 0
	p.playLenStaged = 0
	p.playPtr = 0
	p.playLen = 0
	p.playBusy = false
	p.playErr = false
	p.forceLoop = false
	p.subsong = 0
	p.playGen = 0
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
}

// TEDPlayer.Reset stops playback and clears metadata.
func (p *TEDPlayer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metadata = TEDPlayerMetadata{}
	p.clockHz = 0
	p.loop = false
	p.playPtrStaged = 0
	p.playLenStaged = 0
	p.playPtr = 0
	p.playLen = 0
	p.playBusy = false
	p.playErr = false
	p.forceLoop = false
	p.playGen = 0
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
}

// POKEYPlayer.Reset stops playback and clears metadata.
func (p *POKEYPlayer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metadata = SAPMetadata{}
	p.playPtrStaged = 0
	p.playLenStaged = 0
	p.playPtr = 0
	p.playLen = 0
	p.playBusy = false
	p.playErr = false
	p.forceLoop = false
	p.playGen = 0
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
}
