// audio_event_ring_test.go - ordering and equivalence gates for the audio event ring.
//
// The synchronous mutex path is the oracle. The ring is only allowed to change
// when a guest write reaches chip state, never what the result is, so every
// test here compares against the same schedule run without the ring.
//
// (c) 2024 - 2026 Zayn Otley. GPLv3 or later.

package main

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newRingTestChip builds a chip with the event ring active.
func newRingTestChip() *SoundChip {
	chip := newTestSoundChip()
	chip.eventRing = newAudioEventRing()
	return chip
}

// ringWriteSchedule is a deterministic sequence of register writes issued at
// explicit sample boundaries. Free-running goroutines cannot establish bit
// identity against a path whose lock acquisition order is nondeterministic, so
// the schedule fixes the order and the test drives the boundaries.
type ringWriteSchedule struct {
	segment int
	writes  []struct {
		afterSegment int
		addr         uint32
		value        uint32
	}
}

func defaultRingSchedule() ringWriteSchedule {
	s := ringWriteSchedule{segment: 96}
	add := func(afterSegment int, addr, value uint32) {
		s.writes = append(s.writes, struct {
			afterSegment int
			addr         uint32
			value        uint32
		}{afterSegment, addr, value})
	}
	add(0, FLEX_CH0_BASE+FLEX_OFF_FREQ, 220*256)
	add(0, FLEX_CH1_BASE+FLEX_OFF_FREQ, 330*256)
	add(1, FLEX_CH1_BASE+FLEX_OFF_VOL, 160)
	add(1, FLEX_CH1_BASE+FLEX_OFF_SUS, 255)
	add(1, FLEX_CH1_BASE+FLEX_OFF_CTRL, 3)
	add(2, FILTER_TYPE, 1)
	add(2, FILTER_CUTOFF, 190)
	add(3, FLEX_CH0_BASE+FLEX_OFF_VOL, 96)
	add(4, REVERB_MIX, 100)
	add(5, FLEX_CH1_BASE+FLEX_OFF_CTRL, 1)
	return s
}

// runRingSchedule renders segments, issuing each scheduled write at its
// boundary through the supplied write function.
func runRingSchedule(chip *SoundChip, sched ringWriteSchedule, segments int, write func(addr, value uint32)) []float32 {
	out := make([]float32, 0, segments*sched.segment)
	buf := make([]float32, sched.segment)
	for seg := range segments {
		for _, w := range sched.writes {
			if w.afterSegment == seg {
				write(w.addr, w.value)
			}
		}
		chip.ReadSamples(buf)
		out = append(out, buf...)
	}
	return out
}

// TestAudioEventRing_BitIdenticalToMutexPath is the headline gate: the same
// deterministic schedule, once through the mutex path and once through the
// ring, must produce byte-equal audio and byte-equal final register state.
func TestAudioEventRing_BitIdenticalToMutexPath(t *testing.T) {
	sched := defaultRingSchedule()
	const segments = 12

	mutexChip := newTestSoundChip()
	configureBlockReadChip(mutexChip)
	want := runRingSchedule(mutexChip, sched, segments, mutexChip.HandleRegisterWrite)

	ringChip := newRingTestChip()
	configureBlockReadChip(ringChip)
	got := runRingSchedule(ringChip, sched, segments, ringChip.HandleRegisterWriteFromBus)

	assertFloat32BitsEqual(t, got, want)
	if ringChip.eventRing.published.Load() == 0 {
		t.Fatal("no write reached the ring, so the comparison proved nothing")
	}
	assertChannelStateEqual(t, ringChip, mutexChip)
}

// assertChannelStateEqual compares the register-visible channel state of two
// chips, which catches a queued write that was applied to the wrong place even
// when the audio happens to match.
func assertChannelStateEqual(t *testing.T, got, want *SoundChip) {
	t.Helper()
	got.mu.Lock()
	defer got.mu.Unlock()
	want.mu.Lock()
	defer want.mu.Unlock()
	for i := range NUM_CHANNELS {
		g, w := got.channels[i], want.channels[i]
		if g == nil || w == nil {
			continue
		}
		if g.frequency != w.frequency || g.volume != w.volume || g.enabled != w.enabled || g.gate != w.gate {
			t.Fatalf("channel %d state differs: ring freq=%g vol=%g enabled=%v gate=%v, mutex freq=%g vol=%g enabled=%v gate=%v",
				i, g.frequency, g.volume, g.enabled, g.gate, w.frequency, w.volume, w.enabled, w.gate)
		}
	}
	if got.filterType != want.filterType || got.filterCutoffTarget != want.filterCutoffTarget {
		t.Fatalf("post-FX config differs: ring type=%d cutoff=%g, mutex type=%d cutoff=%g",
			got.filterType, got.filterCutoffTarget, want.filterType, want.filterCutoffTarget)
	}
}

// TestAudioEventRing_CrossPathWritePreservesOrder interleaves an older queued
// ring event, a normal non-overflow barrier write and a render boundary. The
// barrier write must land after the queued one, whatever order the calls are
// made in.
func TestAudioEventRing_CrossPathWritePreservesOrder(t *testing.T) {
	mutexChip := newTestSoundChip()
	configureBlockReadChip(mutexChip)
	ringChip := newRingTestChip()
	configureBlockReadChip(ringChip)

	buf := make([]float32, 64)
	apply := func(chip *SoundChip, queued func(addr, value uint32)) []float32 {
		out := make([]float32, 0, 3*len(buf))
		// An older write goes down the queued path, then a barrier write to
		// the same register follows it. The barrier one must win.
		queued(FLEX_CH0_BASE+FLEX_OFF_VOL, 40)
		chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 200)
		chip.ReadSamples(buf)
		out = append(out, buf...)
		queued(FLEX_CH0_BASE+FLEX_OFF_FREQ, 500*256)
		chip.ReadSamples(buf)
		out = append(out, buf...)
		chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 700*256)
		chip.ReadSamples(buf)
		return append(out, buf...)
	}

	want := apply(mutexChip, mutexChip.HandleRegisterWrite)
	got := apply(ringChip, ringChip.HandleRegisterWriteFromBus)
	assertFloat32BitsEqual(t, got, want)
	assertChannelStateEqual(t, ringChip, mutexChip)
}

// TestAudioEventRing_OverflowPreservesOrder fills the ring so the next guest
// write has to fall back to the barrier path, and checks the result is the same
// as if the ring had never been involved.
func TestAudioEventRing_OverflowPreservesOrder(t *testing.T) {
	mutexChip := newTestSoundChip()
	configureBlockReadChip(mutexChip)
	ringChip := newRingTestChip()
	configureBlockReadChip(ringChip)

	// More writes than the ring can hold, issued with no render in between, so
	// the tail never advances and the ring genuinely overflows.
	writes := make([]struct{ addr, value uint32 }, 0, audioEventRingCapacity+64)
	for i := range audioEventRingCapacity + 64 {
		writes = append(writes, struct{ addr, value uint32 }{
			FLEX_CH0_BASE + FLEX_OFF_VOL, uint32(i % 256),
		})
	}
	for _, w := range writes {
		mutexChip.HandleRegisterWrite(w.addr, w.value)
		ringChip.HandleRegisterWriteFromBus(w.addr, w.value)
	}
	if ringChip.eventRing.overflows.Load() == 0 {
		t.Fatal("the ring never overflowed, so the fallback path was not exercised")
	}

	want := make([]float32, 256)
	got := make([]float32, 256)
	mutexChip.ReadSamples(want)
	ringChip.ReadSamples(got)
	assertFloat32BitsEqual(t, got, want)
	assertChannelStateEqual(t, ringChip, mutexChip)
}

// TestAudioEventRing_ProducerPausedAcrossGateClose drives the race the
// admission handshake exists for: a producer has claimed a sequence but has
// not yet published its payload when a barrier writer closes the gate. The
// writer must wait for that producer, drain its event, and only then apply its
// own mutation, so a write carrying an older sequence can never land after it.
func TestAudioEventRing_ProducerPausedAcrossGateClose(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)

	admitted := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	audioEventTestHook = func(stage int) {
		if stage != audioEventStageReserved {
			return
		}
		once.Do(func() {
			close(admitted)
			<-release
		})
	}
	defer func() { audioEventTestHook = nil }()

	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, 33)
	}()
	<-admitted

	barrierEntered := make(chan struct{})
	barrierDone := make(chan struct{})
	go func() {
		defer close(barrierDone)
		close(barrierEntered)
		chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 222)
	}()
	<-barrierEntered

	// The barrier writer is now either waiting on the gate or waiting for the
	// in-flight producer. Releasing the producer must let both finish, with the
	// barrier write applied last.
	select {
	case <-barrierDone:
		t.Fatal("the barrier writer completed while a producer was still in flight")
	default:
	}
	close(release)
	<-producerDone
	<-barrierDone

	if chip.eventRing.published.Load() != 1 {
		t.Fatalf("the paused producer published %d events, want 1", chip.eventRing.published.Load())
	}
	if chip.eventRing.applied.Load() != 1 {
		t.Fatalf("the barrier writer drained %d events, want the 1 the producer had claimed", chip.eventRing.applied.Load())
	}

	chip.drainAudioEventsBeforeRender()
	chip.mu.Lock()
	got := chip.channels[0].volume
	chip.mu.Unlock()
	want := float32(222) / 255.0
	if math.Abs(float64(got-want)) > 1e-6 {
		t.Fatalf("channel 0 volume = %g, want %g: the barrier write must be applied after the queued one", got, want)
	}
}

// TestAudioEventRing_ConcurrentBarrierWritersSerialised runs two barrier
// writers at once while a producer keeps publishing, and checks the barrier
// mutex actually serialises them: no two barriers overlap, and the final state
// matches the last barrier write.
func TestAudioEventRing_ConcurrentBarrierWritersSerialised(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)

	var (
		mu       sync.Mutex
		inside   int
		maxSeen  int
		sequence []uint32
	)
	observe := func(value uint32) {
		mu.Lock()
		inside++
		if inside > maxSeen {
			maxSeen = inside
		}
		sequence = append(sequence, value)
		mu.Unlock()
		mu.Lock()
		inside--
		mu.Unlock()
	}

	stop := make(chan struct{})
	var producerWrites atomic.Uint64
	var producers sync.WaitGroup
	producers.Add(1)
	go func() {
		defer producers.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			chip.HandleRegisterWriteFromBus(FLEX_CH1_BASE+FLEX_OFF_VOL, uint32(i%256))
			producerWrites.Add(1)
		}
	}()

	var writers sync.WaitGroup
	for w := range 2 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for i := range 100 {
				value := uint32(w*1000 + i)
				chip.audioEventBarrier(func() { observe(value) })
				// Let admission reopen for long enough that producers really
				// publish, so the barriers are contended against queued events
				// and not only against each other.
				time.Sleep(10 * time.Microsecond)
			}
		}()
	}
	writers.Wait()
	close(stop)
	producers.Wait()

	if maxSeen != 1 {
		t.Fatalf("%d barrier writers were inside the protocol at once; barriers must be strictly serialised", maxSeen)
	}
	if len(sequence) != 200 {
		t.Fatalf("%d barrier bodies ran, want 200", len(sequence))
	}
	if producerWrites.Load() == 0 {
		t.Fatal("the producer never ran, so the barriers were not contended")
	}
	if chip.eventRing.published.Load() == 0 {
		t.Fatal("the producer never published, so no ring event was ever in flight across a barrier")
	}
}

// TestAudioEventRing_ConfigSnapshotImmutable checks that a queued write cannot
// mutate the post-FX configuration a flush is already using. The capture is
// taken under chip.mu and the drain also needs chip.mu, so a write published
// mid-flush must land after that flush, not inside it.
func TestAudioEventRing_ConfigSnapshotImmutable(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)
	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	chip.HandleRegisterWrite(FILTER_CUTOFF, 120)

	// Queue a configuration change, then render. The samples produced by the
	// flush that drains it must all have used the pre-change configuration,
	// which is what comparing against the mutex path at the same boundary shows.
	mutexChip := newTestSoundChip()
	configureBlockReadChip(mutexChip)
	mutexChip.HandleRegisterWrite(FILTER_TYPE, 1)
	mutexChip.HandleRegisterWrite(FILTER_CUTOFF, 120)

	buf := make([]float32, 128)
	want := make([]float32, 0, 3*len(buf))
	got := make([]float32, 0, 3*len(buf))
	for seg := range 3 {
		if seg == 1 {
			chip.HandleRegisterWriteFromBus(FILTER_CUTOFF, 240)
			mutexChip.HandleRegisterWrite(FILTER_CUTOFF, 240)
		}
		chip.ReadSamples(buf)
		got = append(got, buf...)
		mutexChip.ReadSamples(buf)
		want = append(want, buf...)
	}
	assertFloat32BitsEqual(t, got, want)
}

// TestAudioEventRing_DrainOnResetAndDisable pins that sealing the ring applies
// everything committed and then keeps admission shut, so a reset or a shutdown
// cannot drop or reorder a queued write.
func TestAudioEventRing_DrainOnResetAndDisable(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)

	chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, 77)
	published := chip.eventRing.published.Load()
	if published == 0 {
		t.Fatal("the write did not reach the ring")
	}

	chip.sealAudioEventRing()
	if applied := chip.eventRing.applied.Load(); applied != published {
		t.Fatalf("sealing applied %d of %d queued writes; none may be dropped", applied, published)
	}
	if !chip.eventRing.gateClosed.Load() {
		t.Fatal("a sealed ring must keep admission shut")
	}

	// A guest write against a sealed ring falls back to the barrier path rather
	// than being lost.
	chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, 190)
	if chip.eventRing.published.Load() != published {
		t.Fatal("a sealed ring accepted a new publication")
	}
	chip.mu.Lock()
	got := chip.channels[0].volume
	chip.mu.Unlock()
	if want := float32(190) / 255.0; math.Abs(float64(got-want)) > 1e-6 {
		t.Fatalf("channel 0 volume = %g, want %g: the fallback write was lost", got, want)
	}

	chip.unsealAudioEventRing()
	chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, 33)
	if chip.eventRing.published.Load() == published {
		t.Fatal("unsealing did not let publications resume")
	}
}

// TestAudioEventRing_EnabledByDefault pins the default-on ring contract.
func TestAudioEventRing_EnabledByDefault(t *testing.T) {
	if !audioEventRingRequested() {
		t.Skip("IE_AUDIO_EVENT_RING=0 in the environment")
	}
	chip := newTestSoundChip()
	if !chip.audioEventRingActive() {
		t.Fatal("a chip built with the default must have the event ring active")
	}
}

func TestAudioEventRing_DisabledBySwitch(t *testing.T) {
	t.Setenv("IE_AUDIO_EVENT_RING", "0")
	chip := newTestSoundChip()
	if chip.audioEventRingActive() {
		t.Fatal("IE_AUDIO_EVENT_RING=0 must build a chip with no event ring")
	}
}

// TestAudioEventRing_ConcurrentWritesDuringRender hammers the case the ring is
// built for, with and without it: register writes arriving continuously while
// the renderer pulls samples. It exists to be run under the race detector, and
// it is what exposed two pre-existing faults in the pending-block handover,
// where the block state was published and retired outside state.mu and the tap
// loop read the destination slice after another pull had cleared it.
//
// One renderer only: ReadSamples is single-consumer by construction, sharing
// one mixer-capture scratch and one pending-block state across calls, so two
// concurrent pulls would corrupt each other whatever the ring does.
func TestAudioEventRing_ConcurrentWritesDuringRender(t *testing.T) {
	for _, useRing := range []bool{false, true} {
		name := "mutex path"
		if useRing {
			name = "event ring"
		}
		t.Run(name, func(t *testing.T) {
			chip := newTestSoundChip()
			configureBlockReadChip(chip)
			if useRing {
				chip.eventRing = newAudioEventRing()
			}

			stop := make(chan struct{})
			var background, bounded sync.WaitGroup

			background.Add(1)
			go func() {
				defer background.Done()
				buf := make([]float32, 256)
				for {
					select {
					case <-stop:
						return
					default:
					}
					chip.ReadSamples(buf)
				}
			}()

			for w := range 3 {
				bounded.Add(1)
				go func() {
					defer bounded.Done()
					for i := range 4000 {
						chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, uint32((i+w)%256))
						if i%16 == 0 {
							chip.HandleRegisterRead(AUDIO_CTRL)
						}
					}
				}()
			}

			bounded.Wait()
			close(stop)
			background.Wait()
		})
	}
}

// TestAudioEventRing_QueuedUnfreezeIsApplied pins the ordering that made the
// freeze bit a trap: AUDIO_CTRL is an ordinary guest register, so the write
// that clears the freeze arrives through the ring. If a render pull tested the
// freeze bit before draining, it would return early without ever applying the
// unfreeze and the chip would stay frozen for good.
func TestAudioEventRing_QueuedUnfreezeIsApplied(t *testing.T) {
	for _, name := range []string{"ReadSamples", "ReadSample"} {
		t.Run(name, func(t *testing.T) {
			chip := newRingTestChip()
			configureBlockReadChip(chip)

			// Freeze, then queue the unfreeze, with no synchronous read or
			// setter in between to drain it for us.
			chip.HandleRegisterWrite(AUDIO_CTRL, 0x03)
			if !chip.audioFrozen.Load() {
				t.Fatal("the chip did not freeze")
			}
			chip.HandleRegisterWriteFromBus(AUDIO_CTRL, 0x01)
			if chip.eventRing.published.Load() == 0 {
				t.Fatal("the unfreeze write did not reach the ring")
			}

			// Only render pulls from here on.
			if name == "ReadSamples" {
				out := make([]float32, 64)
				chip.ReadSamples(out)
			} else {
				chip.ReadSample()
			}

			if chip.audioFrozen.Load() {
				t.Fatal("the chip is still frozen: a render pull never applied the queued unfreeze write")
			}
		})
	}
}

// TestAudioEventRing_UnsealSerialisedWithBarriers pins that reopening a sealed
// ring participates in the barrier protocol. Reopening the gate from outside
// barrierMu would let a producer publish into the window between an in-flight
// barrier's admission close and its drain, so that barrier would apply the
// newer event before its own older write.
func TestAudioEventRing_UnsealSerialisedWithBarriers(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)
	chip.sealAudioEventRing()

	// Open a barrier and hold it.
	chip.beginAudioEventBarrier()

	unsealed := make(chan struct{})
	go func() {
		chip.unsealAudioEventRing()
		close(unsealed)
	}()

	select {
	case <-unsealed:
		t.Fatal("unsealing completed while a barrier was open; it must be serialised against barrier writers")
	case <-time.After(50 * time.Millisecond):
	}

	chip.endAudioEventBarrier()
	select {
	case <-unsealed:
	case <-time.After(2 * time.Second):
		t.Fatal("unsealing never completed after the barrier closed")
	}

	// The ring is usable again.
	chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, 44)
	if chip.eventRing.published.Load() == 0 {
		t.Fatal("publications did not resume after unsealing")
	}
}

// TestAudioEventRing_SnapshotSeesQueuedWrites pins that a debug snapshot
// captures state the guest has already written but the renderer has not yet
// drained. Without the barrier the snapshot records the pre-write state, the
// event is drained afterwards, and a reverse-debug restore replaces the chip
// state and loses the write entirely.
func TestAudioEventRing_SnapshotSeesQueuedWrites(t *testing.T) {
	chip := newRingTestChip()
	configureBlockReadChip(chip)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)

	// Queue a change with no render in between, so it is still in the ring.
	chip.HandleRegisterWriteFromBus(FILTER_TYPE, 3)
	if chip.eventRing.applied.Load() != 0 {
		t.Fatal("the write was drained before the snapshot, so the test proves nothing")
	}

	version, data, err := chip.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	if chip.eventRing.applied.Load() == 0 {
		t.Fatal("the snapshot did not drain the ring, so it captured stale state")
	}

	// Move the chip away from the snapshotted value, then restore.
	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	if err := chip.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	chip.mu.Lock()
	got := chip.filterType
	chip.mu.Unlock()
	if got != 3 {
		t.Fatalf("filter type after restore = %d, want 3: the snapshot must include the queued guest write", got)
	}
}
