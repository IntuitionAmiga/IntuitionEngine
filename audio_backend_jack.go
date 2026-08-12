//go:build linux && cgo && jack

package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/xthexder/go-jack"
)

// JACKAudioOutput leaves rendering on an ordinary Go goroutine. The process
// callback only copies pre-rendered mono samples to JACK's output buffers.
type JACKAudioOutput struct {
	chip   *SoundChip
	client *jack.Client
	left   *jack.Port
	right  *jack.Port
	ring   *jackSampleRing
	server *jackServerHandle

	stopCh  chan struct{}
	doneCh  chan struct{}
	failCh  chan struct{}
	started atomic.Bool
	closed  atomic.Bool
	failed  atomic.Bool
	xruns   atomic.Uint64
}

var requestJACKTermination = func() { exitProfiled(1) }

func init() { compiledFeatures = append(compiledFeatures, "audio:jack") }

func NewJackAudioOutput(sampleRate int, chip *SoundChip) (AudioOutput, error) {
	if chip == nil {
		return nil, fmt.Errorf("JACK audio requires a sound chip")
	}
	if sampleRate != jackSampleRate {
		return nil, fmt.Errorf("JACK requires %d Hz, got %d", jackSampleRate, sampleRate)
	}

	client, status := jack.ClientOpen("IntuitionEngine", jack.NoStartServer)
	var server *jackServerHandle
	if !jackClientOpenSucceeded(client, status) {
		var err error
		server, err = startOwnedJACK(os.Getenv("IE_JACK_ALSA_DEVICE"))
		if err != nil {
			return nil, err
		}
		client, status = jack.ClientOpen("IntuitionEngine", jack.NoStartServer)
		if !jackClientOpenSucceeded(client, status) {
			server.stop()
			return nil, fmt.Errorf("open JACK client after server start (status=%#x)", status)
		}
	}
	if int(client.GetSampleRate()) != jackSampleRate || int(client.GetBufferSize()) != jackPeriodSize {
		_ = client.Close()
		server.stop()
		return nil, fmt.Errorf("JACK server must use %d Hz and %d-frame periods", jackSampleRate, jackPeriodSize)
	}
	output := &JACKAudioOutput{chip: chip, client: client, ring: newJACKSampleRing(jackPeriodSize), server: server, stopCh: make(chan struct{}), doneCh: make(chan struct{}), failCh: make(chan struct{}, 1)}
	output.left = client.PortRegister("out_left", jack.DEFAULT_AUDIO_TYPE, jack.PortIsOutput, 0)
	if output.left == nil {
		output.Close()
		return nil, fmt.Errorf("register JACK left output port")
	}
	output.right = client.PortRegister("out_right", jack.DEFAULT_AUDIO_TYPE, jack.PortIsOutput, 0)
	if output.right == nil {
		output.Close()
		return nil, fmt.Errorf("register JACK right output port")
	}
	if client.SetProcessCallback(output.process) != 0 || client.SetSampleRateCallback(output.sampleRateChanged) != 0 || client.SetBufferSizeCallback(output.bufferSizeChanged) != 0 || client.SetXRunCallback(output.xrun) != 0 {
		output.Close()
		return nil, fmt.Errorf("register JACK callbacks")
	}
	client.OnShutdown(output.shutdown)
	if client.Activate() != 0 {
		output.Close()
		return nil, fmt.Errorf("activate JACK client")
	}
	if err := output.connectPlaybackPorts(); err != nil {
		output.Close()
		return nil, err
	}
	go output.superviseFailure()
	return output, nil
}

// JACK reports advisory status bits such as JackNameNotUnique alongside a
// perfectly usable, renamed client. A nil client is the only open failure.
func jackClientOpenSucceeded(client *jack.Client, _ int) bool { return client != nil }

func (o *JACKAudioOutput) Start() {
	if o.closed.Load() || !o.started.CompareAndSwap(false, true) {
		return
	}
	o.ring.xruns.Store(0)
	o.xruns.Store(0)
	go o.render()
	o.ring.enablePlayback()
}

func (o *JACKAudioOutput) Stop() { o.Close() }

func (o *JACKAudioOutput) Close() {
	if !o.closed.CompareAndSwap(false, true) {
		return
	}
	close(o.stopCh)
	if o.started.Load() {
		<-o.doneCh
	}
	if o.client != nil {
		_ = o.client.Close()
	}
	o.server.stop()
}

func (o *JACKAudioOutput) IsStarted() bool { return o.started.Load() && !o.closed.Load() }

func (o *JACKAudioOutput) render() {
	defer close(o.doneCh)
	block := make([]float32, jackPeriodSize)
	wait := time.NewTimer(time.Second)
	if !wait.Stop() {
		<-wait.C
	}
	defer wait.Stop()
	for {
		if o.ring.free() >= jackPeriodSize {
			o.chip.ReadSamples(block)
			if !o.ring.write(block) {
				continue
			}
			continue
		}
		wait.Reset(time.Second / jackSampleRate * jackPeriodSize / 2)
		select {
		case <-o.stopCh:
			return
		case <-wait.C:
		}
	}
}

func (o *JACKAudioOutput) process(nframes uint32) int {
	left, right := o.left.GetBuffer(nframes), o.right.GetBuffer(nframes)
	if o.failed.Load() || int(nframes) != jackPeriodSize {
		for i := range left {
			left[i], right[i] = 0, 0
		}
		return 0
	}
	read := o.ring.read.Load()
	write := o.ring.writePos.Load()
	available := write - read
	if !o.ring.playing.Load() {
		available = 0
	}
	consume := uint64(nframes)
	if consume > available {
		consume = available
	}
	for i := range left {
		var sample float32
		if uint64(i) < consume {
			sample = o.ring.samples[(read+uint64(i))%o.ring.capacity()]
		}
		left[i] = jack.AudioSample(sample)
		right[i] = jack.AudioSample(sample)
	}
	if consume != 0 {
		o.ring.read.Store(read + consume)
	}
	if consume < uint64(nframes) && o.ring.playing.Load() {
		o.ring.xruns.Add(1)
	}
	return 0
}

func (o *JACKAudioOutput) sampleRateChanged(rate uint32) int {
	if rate != jackSampleRate {
		o.signalFailure()
	}
	return 0
}
func (o *JACKAudioOutput) bufferSizeChanged(size uint32) int {
	if size != jackPeriodSize {
		o.signalFailure()
	}
	return 0
}
func (o *JACKAudioOutput) xrun() int { o.xruns.Add(1); return 0 }
func (o *JACKAudioOutput) shutdown() {
	o.signalFailure()
}

func (o *JACKAudioOutput) signalFailure() {
	if o.failed.CompareAndSwap(false, true) {
		select {
		case o.failCh <- struct{}{}:
		default:
		}
	}
}

func (o *JACKAudioOutput) superviseFailure() {
	select {
	case <-o.failCh:
		o.Close()
		requestJACKTermination()
	case <-o.stopCh:
	}
}

func (o *JACKAudioOutput) connectPlaybackPorts() error {
	ports := o.client.GetPorts("", jack.DEFAULT_AUDIO_TYPE, jack.PortIsInput|jack.PortIsPhysical)
	if len(ports) == 0 {
		return fmt.Errorf("no physical JACK playback ports are available")
	}
	if o.client.Connect(o.left.GetName(), ports[0]) != 0 {
		return fmt.Errorf("connect JACK left output to %q", ports[0])
	}
	if len(ports) > 1 && o.client.Connect(o.right.GetName(), ports[1]) != 0 {
		return fmt.Errorf("connect JACK right output to %q", ports[1])
	}
	return nil
}
