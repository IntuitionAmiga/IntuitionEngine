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
	waveTypes := [NUM_CHANNELS]int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i, ch := range chip.channels {
		if ch == nil {
			continue
		}
		ch.waveType = waveTypes[i]
		ch.frequency = 0
		ch.volume = MIN_VOLUME
		ch.phase = MIN_PHASE
		ch.enabled = false
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
		ch.dutyCycle = DEFAULT_DUTY_CYCLE
		ch.noiseSR = NOISE_LFSR_SEED
		ch.psgPlusGain = 1.0
		ch.psgPlusOversample = 1
		ch.pokeyPlusGain = 1.0
		ch.pokeyPlusOversample = 1
		ch.syncSource = nil
		ch.ringModSource = nil
		ch.sweepEnabled = false
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
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
	e.playing = false
	e.enabled.Store(false)
	e.channelsInit = false
	e.updateEnvPeriodSamples()
}

// VideoChip.Reset restores video chip to cold boot state.
// Preserves: output, layer, bus, busMemory, splashBuffer, bigEndianMode, onResolutionChange.
func (vc *VideoChip) Reset() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.currentMode = MODE_640x480
	vc.frameCounter = 0
	vc.enabled.Store(false)
	vc.hasContent.Store(false)
	vc.inVBlank.Store(false)
	vc.copperEnabled = false

	for i := range vc.frontBuffer {
		vc.frontBuffer[i] = 0
	}
	for i := range vc.backBuffer {
		vc.backBuffer[i] = 0
	}
	for i := range vc.prevVRAM {
		vc.prevVRAM[i] = 0
	}

	// Reset copper state
	vc.copperPC = 0
	vc.copperWaiting = false
	vc.copperHalted = false
	vc.copperWaitX = 0
	vc.copperWaitY = 0

	// Reset blitter state
	vc.bltBusy = false
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
	vga.attrFlip = false
	for i := range vga.attrRegs {
		vga.attrRegs[i] = 0
	}

	// Reinitialize default palette
	vga.initDefaultPalette()
	vga.paletteDirty = true

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
