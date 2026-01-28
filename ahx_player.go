// ahx_player.go - High-level AHX player interface

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// AHXPlayer provides a high-level interface for AHX playback
type AHXPlayer struct {
	engine *AHXEngine

	// I/O register state for memory-mapped playback control
	bus           MemoryBus
	playPtrStaged uint32
	playLenStaged uint32
	playPtr       uint32
	playLen       uint32
	subsong       uint8
	playBusy      bool
	playErr       bool
	forceLoop     bool
}

// NewAHXPlayer creates a new AHX player
func NewAHXPlayer(sound *SoundChip, sampleRate int) *AHXPlayer {
	return &AHXPlayer{
		engine: NewAHXEngine(sound, sampleRate),
	}
}

// Load loads AHX data into the player
func (p *AHXPlayer) Load(data []byte) error {
	return p.engine.LoadData(data)
}

// LoadFile loads an AHX file from disk
func (p *AHXPlayer) LoadFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return p.Load(data)
}

// Play starts playback
func (p *AHXPlayer) Play() {
	p.engine.SetPlaying(true)
}

// PlaySubsong starts playback of a specific subsong
func (p *AHXPlayer) PlaySubsong(nr int) {
	p.engine.replayer.InitSubsong(nr)
	p.engine.SetPlaying(true)
}

// Stop stops playback
func (p *AHXPlayer) Stop() {
	p.engine.SetPlaying(false)
}

// IsPlaying returns true if playing
func (p *AHXPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// SetLoop enables/disables looping
func (p *AHXPlayer) SetLoop(loop bool) {
	p.engine.SetLoop(loop)
}

// TickSample advances playback by one sample (call from audio callback)
func (p *AHXPlayer) TickSample() {
	p.engine.TickSample()
}

// GetSongName returns the name of the loaded song
func (p *AHXPlayer) GetSongName() string {
	return p.engine.GetSongName()
}

// AHXMetadata contains metadata about the loaded AHX file
type AHXMetadata struct {
	Name string
}

// Metadata returns metadata about the loaded AHX file
func (p *AHXPlayer) Metadata() AHXMetadata {
	return AHXMetadata{
		Name: p.engine.GetSongName(),
	}
}

// GetSubsongCount returns the number of subsongs
func (p *AHXPlayer) GetSubsongCount() int {
	if p.engine.replayer.Song != nil {
		return p.engine.replayer.Song.SubsongNr
	}
	return 0
}

// GetInstrumentCount returns the number of instruments
func (p *AHXPlayer) GetInstrumentCount() int {
	if p.engine.replayer.Song != nil {
		return p.engine.replayer.Song.InstrumentNr
	}
	return 0
}

// GetInstrumentName returns the name of an instrument
func (p *AHXPlayer) GetInstrumentName(nr int) string {
	if p.engine.replayer.Song != nil && nr > 0 && nr <= p.engine.replayer.Song.InstrumentNr {
		return p.engine.replayer.Song.Instruments[nr].Name
	}
	return ""
}

// GetPosition returns the current position and note
func (p *AHXPlayer) GetPosition() (posNr, noteNr int) {
	return p.engine.GetPosition()
}

// GetPlayingTime returns the playing time in ticks
func (p *AHXPlayer) GetPlayingTime() int {
	return p.engine.GetPlayingTime()
}

// Reset resets the player state
func (p *AHXPlayer) Reset() {
	p.engine.Reset()
}

// AttachBus attaches the memory bus for reading embedded AHX data
func (p *AHXPlayer) AttachBus(bus MemoryBus) {
	p.bus = bus
}

// HandlePlayWrite handles writes to AHX_PLAY_* registers
func (p *AHXPlayer) HandlePlayWrite(addr uint32, value uint32) {
	switch addr {
	case AHX_PLUS_CTRL:
		p.engine.SetAHXPlusEnabled(value != 0)
	case AHX_PLAY_PTR:
		p.playPtrStaged = value
	case AHX_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case AHX_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case AHX_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case AHX_PLAY_LEN:
		p.playLenStaged = value
	case AHX_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case AHX_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case AHX_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case AHX_SUBSONG:
		p.subsong = uint8(value)
	case AHX_PLAY_CTRL:
		if value&0x2 != 0 {
			p.Stop()
			p.playBusy = false
			p.playErr = false
			return
		}
		if value&0x1 == 0 {
			return
		}
		if p.playBusy {
			return
		}
		p.playPtr = p.playPtrStaged
		p.playLen = p.playLenStaged
		p.forceLoop = (value & 0x4) != 0
		p.playErr = false
		if p.bus == nil {
			p.playErr = true
			return
		}
		if p.playLen == 0 {
			p.playErr = true
			return
		}
		p.playBusy = true
		// Read directly from bus memory
		mem := p.bus.GetMemory()
		if int(p.playPtr)+int(p.playLen) > len(mem) {
			p.playErr = true
			p.playBusy = false
			return
		}
		data := make([]byte, p.playLen)
		copy(data, mem[p.playPtr:p.playPtr+p.playLen])
		if err := p.Load(data); err != nil {
			p.playErr = true
			p.playBusy = false
			return
		}
		// Set subsong if specified
		if p.subsong > 0 {
			p.engine.replayer.InitSubsong(int(p.subsong))
		}
		if p.forceLoop {
			p.engine.SetLoop(true)
		}
		p.Play()
	default:
		return
	}
}

// HandlePlayRead handles reads from AHX_PLAY_* registers
func (p *AHXPlayer) HandlePlayRead(addr uint32) uint32 {
	switch addr {
	case AHX_PLUS_CTRL:
		if p.engine.AHXPlusEnabled() {
			return 1
		}
		return 0
	case AHX_PLAY_PTR:
		return p.playPtrStaged
	case AHX_PLAY_LEN:
		return p.playLenStaged
	case AHX_PLAY_CTRL:
		return p.playCtrlStatus()
	case AHX_PLAY_STATUS:
		return p.playStatus()
	case AHX_SUBSONG:
		return uint32(p.subsong)
	case AHX_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case AHX_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case AHX_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case AHX_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case AHX_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case AHX_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case AHX_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case AHX_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case AHX_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	default:
		return 0
	}
}

func (p *AHXPlayer) playCtrlStatus() uint32 {
	ctrl := uint32(0)
	busy := p.playBusy
	if busy && !p.IsPlaying() {
		p.playBusy = false
		busy = false
	}
	if busy {
		ctrl |= 0x1
	}
	if p.forceLoop {
		ctrl |= 0x4
	}
	return ctrl
}

func (p *AHXPlayer) playStatus() uint32 {
	status := uint32(0)
	busy := p.playBusy
	if busy && !p.IsPlaying() {
		p.playBusy = false
		busy = false
	}
	if busy {
		status |= 0x1
	}
	if p.playErr {
		status |= 0x2
	}
	return status
}

func isAHXExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ahx", ".thx":
		return true
	default:
		return false
	}
}
