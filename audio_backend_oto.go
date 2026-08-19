//go:build !headless

// audio_backend_oto.go - OTO v3 audio output implementation

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

import (
	"github.com/ebitengine/oto/v3"
	"sync"
	"sync/atomic"
	"unsafe"
)

func init() {
	compiledFeatures = append(compiledFeatures, "audio:oto")
}

type OtoPlayer struct {
	ctx       *oto.Context
	player    *oto.Player
	chip      atomic.Pointer[SoundChip] // Atomic for lock-free Read()
	sampleBuf []float32                 // Pre-allocated sample buffer
	started   bool
	closed    bool
	mutex     sync.Mutex // Only for setup/control operations
}

// otoContextOptions builds the oto context options for a mono float32 output
// at the given sample rate. The device buffer duration comes from
// otoBufferDuration so that it is pinned by a single test.
func otoContextOptions(sampleRate int) *oto.NewContextOptions {
	return &oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatFloat32LE,
		BufferSize:   otoBufferDuration(),
	}
}

func NewOtoPlayer(sampleRate int) (*OtoPlayer, error) {
	op := otoContextOptions(sampleRate)

	ctx, ready, err := oto.NewContext(op)
	if err != nil {
		return nil, err
	}
	// On native the device is ready almost immediately. On js/wasm the ready
	// channel only closes once the browser's AudioContext leaves "suspended",
	// which requires a user gesture; blocking here would stall machine boot
	// forever on a page loaded without one, so the wasm variant returns
	// immediately and audio starts on the first keypress or click.
	otoAwaitReady(ready)

	return &OtoPlayer{
		ctx:     ctx,
		started: false,
	}, nil
}

func (op *OtoPlayer) SetupPlayer(chip *SoundChip) {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	op.chip.Store(chip)
	op.closed = false
	op.player = op.ctx.NewPlayer(op)
	bufSize := otoPlayerBufferSize(SAMPLE_RATE)
	op.player.SetBufferSize(bufSize)
	// Pre-allocate buffer for typical oto buffer sizes
	if bufSize/4 > 4096 {
		op.sampleBuf = make([]float32, bufSize/4)
	} else {
		op.sampleBuf = make([]float32, 4096)
	}
}

func (op *OtoPlayer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	fullBytes := (len(p) / 4) * 4
	if fullBytes == 0 {
		clear(p)
		return len(p), nil
	}

	// Load chip pointer atomically - no lock needed for the hot path
	chip := op.chip.Load()
	if chip == nil {
		clear(p)
		return len(p), nil
	}

	numSamples := fullBytes / 4

	// Ensure our pre-allocated buffer is large enough
	// This should rarely happen after initial SetupPlayer
	if len(op.sampleBuf) < numSamples {
		op.sampleBuf = make([]float32, numSamples)
	}
	samples := op.sampleBuf[:numSamples]

	chip.ReadSamples(samples)

	copy(p[:fullBytes], (*[1 << 30]byte)(unsafe.Pointer(&samples[0]))[:fullBytes])
	clear(p[fullBytes:])
	return len(p), nil
}

func (op *OtoPlayer) Start() {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if !op.started && op.player != nil && !op.closed {
		op.player.Play()
		op.started = true
	}
}

func (op *OtoPlayer) Stop() {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.started && op.player != nil && !op.closed {
		op.player.Pause()
		op.started = false
	}
}

func (op *OtoPlayer) Close() {
	op.Stop()
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if !op.closed && op.player != nil {
		op.player.Close()
		op.player = nil
	}
	op.closed = true
}

func (op *OtoPlayer) IsStarted() bool {
	op.mutex.Lock()
	defer op.mutex.Unlock()
	return op.started
}
