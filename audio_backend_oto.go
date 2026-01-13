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
	"unsafe"
)

type OtoPlayer struct {
	ctx     *oto.Context
	player  *oto.Player
	chip    *SoundChip
	started bool
	mutex   sync.Mutex
}

func NewOtoPlayer(sampleRate int) (*OtoPlayer, error) {
	op := &oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatFloat32LE,
		BufferSize:   4,
	}

	ctx, ready, err := oto.NewContext(op)
	if err != nil {
		return nil, err
	}
	<-ready

	return &OtoPlayer{
		ctx:     ctx,
		started: false,
	}, nil
}

func (op *OtoPlayer) SetupPlayer(chip *SoundChip) {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	op.chip = chip
	op.player = op.ctx.NewPlayer(op)
}

func (op *OtoPlayer) Read(p []byte) (n int, err error) {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.chip == nil {
		for i := range p {
			p[i] = 0
		}
		return len(p), nil
	}

	if !op.started {
		op.Start() // Ensure started before first read
	}

	numSamples := len(p) / 4
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		samples[i] = op.chip.ReadSample()
	}

	copy(p, (*[1 << 30]byte)(unsafe.Pointer(&samples[0]))[:len(p)])
	return len(p), nil
}

func (op *OtoPlayer) Start() {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if !op.started && op.player != nil {
		op.player.Play()
		op.started = true
	}
}

func (op *OtoPlayer) Stop() {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.started && op.player != nil {
		op.player.Close()
		op.started = false
	}
}

func (op *OtoPlayer) Close() {
	op.Stop()
	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.player != nil {
		op.player.Close()
		op.player = nil
	}
}

func (op *OtoPlayer) IsStarted() bool {
	op.mutex.Lock()
	defer op.mutex.Unlock()
	return op.started
}
