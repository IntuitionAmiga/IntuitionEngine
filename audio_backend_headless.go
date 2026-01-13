//go:build headless

package main

type OtoPlayer struct {
	started bool
	chip    *SoundChip
}

func NewOtoPlayer(sampleRate int) (*OtoPlayer, error) {
	return &OtoPlayer{}, nil
}

func (op *OtoPlayer) SetupPlayer(chip *SoundChip) {
	op.chip = chip
}

func (op *OtoPlayer) Read(p []byte) (n int, err error) {
	return len(p), nil
}

func (op *OtoPlayer) Start() {
	op.started = true
}

func (op *OtoPlayer) Stop() {
	op.started = false
}

func (op *OtoPlayer) Close() {
	op.started = false
}

func (op *OtoPlayer) IsStarted() bool {
	return op.started
}

type ALSAPlayer struct {
	started bool
}

func NewALSAPlayer() (*ALSAPlayer, error) {
	return &ALSAPlayer{}, nil
}

func (ap *ALSAPlayer) SetupPlayer() {}

func (ap *ALSAPlayer) IsStarted() bool {
	return ap.started
}

func (ap *ALSAPlayer) Write(samples []float32) error {
	return nil
}

func (ap *ALSAPlayer) Start() {
	ap.started = true
}

func (ap *ALSAPlayer) Stop() {
	ap.started = false
}

func (ap *ALSAPlayer) Close() {
	ap.started = false
}
