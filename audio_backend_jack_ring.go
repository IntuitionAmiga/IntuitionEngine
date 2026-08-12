package main

import "sync/atomic"

// jackRingPeriods is intentionally small. Two periods are published as
// startup silence and the third is kept free for the renderer. Physical Pi
// latency and xrun measurements, not convenience, decide any future change.
const jackRingPeriods = 3

// jackSampleRing is a preallocated SPSC mono ring. The producer is the normal
// Go renderer; the consumer is the JACK process callback. Its methods perform
// no allocation or locking so the callback can copy directly to JACK buffers.
type jackSampleRing struct {
	samples  []float32
	period   uint64
	read     atomic.Uint64
	writePos atomic.Uint64
	playing  atomic.Bool
	xruns    atomic.Uint64
}

func newJACKSampleRing(period int) *jackSampleRing {
	if period <= 0 {
		panic("JACK period must be positive")
	}
	capacity := period * jackRingPeriods
	ring := &jackSampleRing{
		samples: make([]float32, capacity),
		period:  uint64(period),
	}
	// Two complete zero periods are readable before playback is enabled.
	ring.writePos.Store(uint64(period * (jackRingPeriods - 1)))
	return ring
}

func (r *jackSampleRing) capacity() uint64    { return uint64(len(r.samples)) }
func (r *jackSampleRing) readCursor() uint64  { return r.read.Load() }
func (r *jackSampleRing) writeCursor() uint64 { return r.writePos.Load() }

func (r *jackSampleRing) available() int {
	return int(r.writePos.Load() - r.read.Load())
}

func (r *jackSampleRing) free() int { return int(r.capacity()) - r.available() }

func (r *jackSampleRing) write(samples []float32) bool {
	if len(samples) == 0 || uint64(len(samples)) > r.period {
		return false
	}
	write := r.writePos.Load()
	read := r.read.Load()
	if uint64(len(samples)) > r.capacity()-(write-read) {
		return false
	}
	capacity := r.capacity()
	for i, sample := range samples {
		r.samples[(write+uint64(i))%capacity] = sample
	}
	r.writePos.Store(write + uint64(len(samples)))
	return true
}

// readMono is used by deterministic unit tests and non-JACK consumers. Missing
// samples are silence; consuming startup prefill is allowed only once playback
// has been enabled.
func (r *jackSampleRing) readMono(dst []float32) {
	r.readInto(dst, nil)
}

func (r *jackSampleRing) readStereo(left, right []float32) {
	r.readInto(left, right)
}

func (r *jackSampleRing) readInto(left, right []float32) {
	if len(right) != 0 && len(right) != len(left) {
		panic("JACK output buffers differ in length")
	}
	if !r.playing.Load() {
		for i := range left {
			left[i] = 0
			if len(right) != 0 {
				right[i] = 0
			}
		}
		return
	}
	read := r.read.Load()
	write := r.writePos.Load()
	available := write - read
	consume := uint64(len(left))
	if consume > available {
		consume = available
	}
	capacity := r.capacity()
	for i := range left {
		var sample float32
		if uint64(i) < consume {
			sample = r.samples[(read+uint64(i))%capacity]
		}
		left[i] = sample
		if len(right) != 0 {
			right[i] = sample
		}
	}
	if consume != 0 {
		r.read.Store(read + consume)
	}
	if consume < uint64(len(left)) {
		r.xruns.Add(1)
	}
}

func (r *jackSampleRing) enablePlayback()   { r.playing.Store(true) }
func (r *jackSampleRing) underruns() uint64 { return r.xruns.Load() }
