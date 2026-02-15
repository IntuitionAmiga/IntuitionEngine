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

	video     *VideoChip
	vga       *VGAEngine
	ula       *ULAEngine
	tedVideo  *TEDVideoEngine
	antic     *ANTICEngine
	voodoo    *VoodooEngine
	sound     *SoundChip
	psgEngine *PSGEngine
	sidEngine *SIDEngine
	pokey     *POKEYEngine
	tedEngine *TEDEngine
	ahxEngine *AHXEngine

	psgPlayer   *PSGPlayer
	sidPlayer   *SIDPlayer
	pokeyPlayer *POKEYPlayer
	tedPlayer   *TEDPlayer
}

type runtimeStatusStore struct {
	mu sync.RWMutex
	runtimeStatusSnapshot
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

func (s *runtimeStatusStore) setChips(video *VideoChip, vga *VGAEngine, ula *ULAEngine, tedVideo *TEDVideoEngine, antic *ANTICEngine, voodoo *VoodooEngine, sound *SoundChip, psg *PSGEngine, sid *SIDEngine, pokey *POKEYEngine, ted *TEDEngine, ahx *AHXEngine) {
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

func (s *runtimeStatusStore) snapshot() runtimeStatusSnapshot {
	s.mu.RLock()
	snap := s.runtimeStatusSnapshot
	s.mu.RUnlock()
	return snap
}

var runtimeStatus = &runtimeStatusStore{}
