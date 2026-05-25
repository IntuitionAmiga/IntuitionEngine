package main

import "sync"

const (
	runtimeCPUNone = iota
	runtimeCPUIE32
	runtimeCPUIE64
	runtimeCPUM68K
	runtimeCPUZ80
	runtimeCPUX86
	runtimeCPU6502
)

type runtimeStatusSnapshot struct {
	selectedCPU int

	ie32  *CPU
	ie64  *CPU64
	m68k  *M68KRunner
	z80   *CPUZ80Runner
	x86   *CPUX86Runner
	cpu65 *CPU6502Runner

	video      *VideoChip
	vga        *VGAEngine
	ula        *ULAEngine
	tedVideo   *TEDVideoEngine
	antic      *ANTICEngine
	voodoo     *VoodooEngine
	sound      *SoundChip
	psgEngine  *PSGEngine
	sidEngine  *SIDEngine
	pokey      *POKEYEngine
	tedEngine  *TEDEngine
	ahxEngine  *AHXEngine
	modEngine  *MODEngine
	wavEngine  *WAVEngine
	midiEngine *MIDIEngine
	paulaDMA   *ArosAudioDMA
	arosDOS    *ArosDOSDevice
	arosClip   *ClipboardBridge

	psgPlayer   *PSGPlayer
	sidPlayer   *SIDPlayer
	pokeyPlayer *POKEYPlayer
	tedPlayer   *TEDPlayer
	midiPlayer  *MIDIPlayer

	coprocManager *CoprocessorManager
	scriptEngine  *ScriptEngine
}

type runtimeStatusStore struct {
	mu sync.RWMutex
	runtimeStatusSnapshot
}

type runtimeStatusIndicator struct {
	name    string
	enabled bool
}

func runtimeAudioStatusIndicators(s runtimeStatusSnapshot) []runtimeStatusIndicator {
	soundOn := s.sound != nil && s.sound.IsEnabled()
	psgOn := s.psgEngine != nil && s.psgEngine.IsPlaying()
	sidOn := s.sidEngine != nil && s.sidEngine.IsPlaying()
	pokeyOn := s.pokey != nil && s.pokey.IsPlaying()
	tedOn := s.tedEngine != nil && s.tedEngine.IsPlaying()
	ahxOn := s.ahxEngine != nil && s.ahxEngine.IsPlaying()
	modOn := s.modEngine != nil && s.modEngine.IsPlaying()
	wavOn := s.wavEngine != nil && s.wavEngine.IsPlaying()
	paulaOn := s.paulaDMA != nil && s.paulaDMA.enabled.Load()
	midiOn := s.midiPlayer != nil && s.midiPlayer.IsPlaying()

	return []runtimeStatusIndicator{
		{name: "IESND", enabled: soundOn},
		{name: "|", enabled: false},
		{name: "PSG", enabled: psgOn},
		{name: "|", enabled: false},
		{name: "TED", enabled: tedOn},
		{name: "|", enabled: false},
		{name: "SID", enabled: sidOn},
		{name: "|", enabled: false},
		{name: "POKEY", enabled: pokeyOn},
		{name: "|", enabled: false},
		{name: "AHX", enabled: ahxOn},
		{name: "|", enabled: false},
		{name: "MOD", enabled: modOn},
		{name: "|", enabled: false},
		{name: "WAV", enabled: wavOn},
		{name: "|", enabled: false},
		{name: "PAULA", enabled: paulaOn},
		{name: "|", enabled: false},
		{name: "MIDI", enabled: midiOn},
	}
}

func (s *runtimeStatusStore) setCPUs(selectedCPU int, ie32 *CPU, ie64 *CPU64, m68k *M68KRunner, z80 *CPUZ80Runner, x86 *CPUX86Runner, cpu65 *CPU6502Runner) {
	s.mu.Lock()
	s.selectedCPU = selectedCPU
	s.ie32 = ie32
	s.ie64 = ie64
	s.m68k = m68k
	s.z80 = z80
	s.x86 = x86
	s.cpu65 = cpu65
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setChips(video *VideoChip, vga *VGAEngine, ula *ULAEngine, tedVideo *TEDVideoEngine, antic *ANTICEngine, voodoo *VoodooEngine, sound *SoundChip, psg *PSGEngine, sid *SIDEngine, pokey *POKEYEngine, ted *TEDEngine, ahx *AHXEngine, mod *MODEngine, wav *WAVEngine) {
	s.mu.Lock()
	s.video = video
	s.vga = vga
	s.ula = ula
	s.tedVideo = tedVideo
	s.antic = antic
	s.voodoo = voodoo
	s.sound = sound
	s.psgEngine = psg
	s.sidEngine = sid
	s.pokey = pokey
	s.tedEngine = ted
	s.ahxEngine = ahx
	s.modEngine = mod
	s.wavEngine = wav
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setPaulaDMA(dma *ArosAudioDMA) {
	s.mu.Lock()
	s.paulaDMA = dma
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setAROSDOS(dos *ArosDOSDevice) {
	s.mu.Lock()
	s.arosDOS = dos
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setAROSClipboard(cb *ClipboardBridge) {
	s.mu.Lock()
	s.arosClip = cb
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setCoprocManager(cm *CoprocessorManager) {
	s.mu.Lock()
	s.coprocManager = cm
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setScriptEngine(se *ScriptEngine) {
	s.mu.Lock()
	s.scriptEngine = se
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setPlayers(psg *PSGPlayer, sid *SIDPlayer, pokey *POKEYPlayer, ted *TEDPlayer) {
	s.mu.Lock()
	s.psgPlayer = psg
	s.sidPlayer = sid
	s.pokeyPlayer = pokey
	s.tedPlayer = ted
	s.mu.Unlock()
}

func (s *runtimeStatusStore) setMIDI(midi *MIDIPlayer) {
	s.mu.Lock()
	s.midiPlayer = midi
	if midi != nil {
		s.midiEngine = midi.engine
	} else {
		s.midiEngine = nil
	}
	s.mu.Unlock()
}

func (s *runtimeStatusStore) snapshot() runtimeStatusSnapshot {
	s.mu.RLock()
	snap := s.runtimeStatusSnapshot
	s.mu.RUnlock()
	return snap
}

var runtimeStatus = &runtimeStatusStore{}
