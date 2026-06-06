package main

import (
	"fmt"
	"os"
	"sync"
)

type MIDIPlayer struct {
	PlayerControlState
	engine *MIDIEngine

	paused bool
	volume uint8
	file   *MIDIFile
	mu     sync.Mutex
}

func NewMIDIPlayer(sound *SoundChip, sampleRate int) *MIDIPlayer {
	p := &MIDIPlayer{engine: NewMIDIEngine(sound, sampleRate), volume: 255}
	p.engine.SetVolume(255)
	return p
}

func (p *MIDIPlayer) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return p.LoadData(data)
}

func (p *MIDIPlayer) LoadData(data []byte) error {
	file, err := ParseMIDIData(data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.file = file
	p.mu.Unlock()
	p.engine.LoadMIDI(file)
	return nil
}

func (p *MIDIPlayer) Play() {
	p.engine.SetPlaying(true)
	p.mu.Lock()
	p.PlayBusy = false
	p.mu.Unlock()
}

func (p *MIDIPlayer) Stop() {
	p.mu.Lock()
	p.PlayGen++
	p.PlayBusy = false
	p.paused = false
	p.mu.Unlock()
	p.engine.SetPlaying(false)
	p.engine.SetPaused(false)
}

func (p *MIDIPlayer) IsPlaying() bool { return p.engine.IsPlaying() }

func (p *MIDIPlayer) DurationSeconds() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return 0
	}
	return float64(p.file.DurationSamples) / SAMPLE_RATE
}

func (p *MIDIPlayer) DurationText() string {
	dur := p.DurationSeconds()
	if dur <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%02d", int(dur)/60, int(dur)%60)
}

func (p *MIDIPlayer) AttachBus(bus Bus32) { p.Bus = bus }

func (p *MIDIPlayer) SetLoop(loop bool) { p.engine.SetLoop(loop) }

func (p *MIDIPlayer) Pause() {
	p.mu.Lock()
	p.paused = true
	p.mu.Unlock()
	p.engine.SetPaused(true)
}

func (p *MIDIPlayer) Resume() {
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
	p.engine.SetPaused(false)
}

func (p *MIDIPlayer) SetVolume(volume uint8) {
	p.mu.Lock()
	p.volume = volume
	p.mu.Unlock()
	p.engine.SetVolume(volume)
}

func (p *MIDIPlayer) Metadata() MusicMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return MusicMetadata{}
	}
	return p.file.Metadata
}

func (p *MIDIPlayer) MIDIFile() *MIDIFile {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.file
}

func (p *MIDIPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var startData []byte
	var startGen uint64
	var stop bool
	p.mu.Lock()
	switch addr {
	case MIDI_PLAY_PTR:
		p.HandlePtrWrite(0, value)
	case MIDI_PLAY_PTR + 1:
		p.HandlePtrWrite(1, value)
	case MIDI_PLAY_PTR + 2:
		p.HandlePtrWordWrite(2, value)
	case MIDI_PLAY_PTR + 3:
		p.HandlePtrWrite(3, value)
	case MIDI_PLAY_LEN:
		p.HandleLenWrite(0, value)
	case MIDI_PLAY_LEN + 1:
		p.HandleLenWrite(1, value)
	case MIDI_PLAY_LEN + 2:
		p.HandleLenWordWrite(2, value)
	case MIDI_PLAY_LEN + 3:
		p.HandleLenWrite(3, value)
	case MIDI_VOLUME:
		p.volume = uint8(value)
		p.engine.SetVolume(p.volume)
	case MIDI_PLAY_CTRL:
		if value&0x2 != 0 {
			p.PlayGen++
			p.PlayBusy = false
			p.ClearError()
			p.paused = false
			stop = true
			break
		}
		if value&0x8 != 0 {
			p.paused = true
			p.engine.SetPaused(true)
		} else {
			p.paused = false
			p.engine.SetPaused(false)
		}
		if value&0x10 != 0 {
			p.ForceLoop = value&0x4 != 0
			p.engine.SetLoop(p.ForceLoop)
			break
		}
		if value&0x1 == 0 {
			break
		}
		p.PlayGen++
		startGen = p.PlayGen
		if errText := p.PreparePlay(value&0x4 != 0); errText != "" {
			break
		}
		data, err := p.ReadDataFromBus()
		if err != nil {
			p.SetError()
			break
		}
		p.PlayBusy = true
		p.ClearError()
		startData = data
	}
	p.mu.Unlock()
	if stop {
		p.engine.SetPlaying(false)
	}
	if startData != nil {
		go p.startAsync(startGen, startData)
	}
}

func (p *MIDIPlayer) startAsync(gen uint64, data []byte) {
	file, err := ParseMIDIData(data)
	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.PlayGen {
		return
	}
	if err != nil {
		p.SetError()
		return
	}
	p.file = file
	p.PlayBusy = false
	p.engine.LoadMIDI(file)
	p.engine.SetLoop(p.ForceLoop)
	p.engine.SetVolume(p.volume)
	p.engine.SetPaused(p.paused)
	p.engine.SetPlaying(true)
}

func (p *MIDIPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch addr {
	case MIDI_PLAY_PTR, MIDI_PLAY_PTR + 1, MIDI_PLAY_PTR + 2, MIDI_PLAY_PTR + 3:
		return p.ReadPtrByte(addr - MIDI_PLAY_PTR)
	case MIDI_PLAY_LEN, MIDI_PLAY_LEN + 1, MIDI_PLAY_LEN + 2, MIDI_PLAY_LEN + 3:
		return p.ReadLenByte(addr - MIDI_PLAY_LEN)
	case MIDI_PLAY_CTRL, MIDI_PLAY_CTRL + 1, MIDI_PLAY_CTRL + 2, MIDI_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatusLocked(), addr-MIDI_PLAY_CTRL)
	case MIDI_PLAY_STATUS, MIDI_PLAY_STATUS + 1, MIDI_PLAY_STATUS + 2, MIDI_PLAY_STATUS + 3:
		return readUint32Byte(p.playStatusLocked(), addr-MIDI_PLAY_STATUS)
	case MIDI_POSITION, MIDI_POSITION + 1, MIDI_POSITION + 2, MIDI_POSITION + 3:
		return readUint32Byte(uint32(p.engine.PositionSamples()), addr-MIDI_POSITION)
	case MIDI_VOLUME:
		return uint32(p.volume)
	case MIDI_TEMPO_BPM:
		return uint32(p.engine.CurrentTempoBPM())
	default:
		return 0
	}
}

func (p *MIDIPlayer) playCtrlStatusLocked() uint32 {
	var ctrl uint32
	if p.PlayBusy || p.engine.IsPlaying() {
		ctrl |= 1
	}
	if p.ForceLoop {
		ctrl |= 4
	}
	if p.paused {
		ctrl |= 8
	}
	return ctrl
}

func (p *MIDIPlayer) playStatusLocked() uint32 {
	var status uint32
	if p.PlayBusy || p.engine.IsPlaying() {
		status |= MIDI_STATUS_BUSY
	}
	if p.PlayErr {
		status |= MIDI_STATUS_ERROR
	}
	if p.paused {
		status |= MIDI_STATUS_PAUSED
	}
	if p.PlayBusy {
		status |= MIDI_STATUS_LOADING
	}
	return status
}
