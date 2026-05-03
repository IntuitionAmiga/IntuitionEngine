package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// MediaLoader implements SOUND PLAY "file" MMIO orchestration.
type MediaLoader struct {
	bus       *MachineBus
	soundChip *SoundChip
	baseDir   string

	psgPlayer   *PSGPlayer
	sidPlayer   *SIDPlayer
	tedPlayer   *TEDPlayer
	ahxPlayer   *AHXPlayer
	pokeyPlayer *POKEYPlayer
	modPlayer   *MODPlayer
	wavPlayer   *WAVPlayer

	namePtr uint32
	subsong uint32
	status  uint32
	typ     uint32
	errCode uint32

	reqGen uint64

	mu sync.Mutex
}

func NewMediaLoader(bus *MachineBus, soundChip *SoundChip, baseDir string, psgPlayer *PSGPlayer, sidPlayer *SIDPlayer, tedPlayer *TEDPlayer, ahxPlayer *AHXPlayer, pokeyPlayer *POKEYPlayer, modPlayer *MODPlayer, wavPlayer *WAVPlayer) *MediaLoader {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = baseDir
	}
	return &MediaLoader{
		bus:         bus,
		soundChip:   soundChip,
		baseDir:     absBase,
		psgPlayer:   psgPlayer,
		sidPlayer:   sidPlayer,
		tedPlayer:   tedPlayer,
		ahxPlayer:   ahxPlayer,
		pokeyPlayer: pokeyPlayer,
		modPlayer:   modPlayer,
		wavPlayer:   wavPlayer,
		status:      MEDIA_STATUS_IDLE,
		typ:         MEDIA_TYPE_NONE,
		errCode:     MEDIA_ERR_OK,
	}
}

func (m *MediaLoader) HandleRead(addr uint32) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch addr {
	case MEDIA_NAME_PTR:
		return m.namePtr
	case MEDIA_SUBSONG:
		return m.subsong
	case MEDIA_STATUS:
		m.refreshStatusLocked()
		return m.status
	case MEDIA_TYPE:
		return m.typ
	case MEDIA_ERROR:
		return m.errCode
	default:
		return 0
	}
}

func (m *MediaLoader) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case MEDIA_NAME_PTR:
		m.mu.Lock()
		m.namePtr = val
		m.mu.Unlock()
	case MEDIA_SUBSONG:
		m.mu.Lock()
		m.subsong = val
		m.mu.Unlock()
	case MEDIA_CTRL:
		switch val {
		case MEDIA_OP_PLAY:
			m.startPlay()
		case MEDIA_OP_STOP:
			m.stopAll()
		}
	}
}

func (m *MediaLoader) startPlay() {
	m.mu.Lock()
	namePtr := m.namePtr
	subsong := m.subsong
	fileName := m.readFileNameLocked(namePtr)
	fullPath, ok := m.sanitizePathLocked(fileName)
	typ := detectMediaType(fileName)
	if !ok {
		m.status = MEDIA_STATUS_ERROR
		m.errCode = MEDIA_ERR_PATH_INVALID
		m.typ = MEDIA_TYPE_NONE
		m.mu.Unlock()
		return
	}
	if typ == MEDIA_TYPE_NONE {
		m.status = MEDIA_STATUS_ERROR
		m.errCode = MEDIA_ERR_UNSUPPORTED
		m.typ = MEDIA_TYPE_NONE
		m.mu.Unlock()
		return
	}
	m.reqGen++
	reqGen := m.reqGen
	m.status = MEDIA_STATUS_LOADING
	m.errCode = MEDIA_ERR_OK
	m.typ = typ
	m.mu.Unlock()

	go m.loadAndStart(reqGen, fullPath, typ, subsong)
}

func (m *MediaLoader) loadAndStart(reqGen uint64, fullPath string, typ uint32, subsong uint32) {
	defer func() {
		if r := recover(); r != nil {
			m.mu.Lock()
			defer m.mu.Unlock()
			if reqGen == m.reqGen {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
			}
		}
	}()

	data, err := os.ReadFile(fullPath)
	if err != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if reqGen != m.reqGen {
			return
		}
		m.status = MEDIA_STATUS_ERROR
		if os.IsNotExist(err) {
			m.errCode = MEDIA_ERR_NOT_FOUND
		} else {
			m.errCode = MEDIA_ERR_BAD_FORMAT
		}
		return
	}

	// WAV files can exceed MEDIA_STAGING_SIZE (64KB); load directly via player
	if typ == MEDIA_TYPE_WAV {
		m.mu.Lock()
		if reqGen != m.reqGen {
			m.mu.Unlock()
			return
		}
		if m.wavPlayer != nil {
			m.mu.Unlock()
			loadErr := m.wavPlayer.Load(data)
			m.mu.Lock()
			if reqGen != m.reqGen {
				m.mu.Unlock()
				return
			}
			if loadErr != nil {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
				m.mu.Unlock()
				return
			}
			if m.soundChip != nil && m.wavPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.wavPlayer.engine)
			}
			m.wavPlayer.Play()
			m.status = MEDIA_STATUS_PLAYING
			m.errCode = MEDIA_ERR_OK
		}
		m.mu.Unlock()
		return
	}

	// MOD files can exceed MEDIA_STAGING_SIZE (64KB); load directly via player
	if typ == MEDIA_TYPE_MOD {
		m.mu.Lock()
		if reqGen != m.reqGen {
			m.mu.Unlock()
			return
		}
		if m.modPlayer != nil {
			m.stopPlayersOnly()
			m.mu.Unlock()
			loadErr := m.modPlayer.Load(data)
			m.mu.Lock()
			if reqGen != m.reqGen {
				m.mu.Unlock()
				return
			}
			if loadErr != nil {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
				m.mu.Unlock()
				return
			}
			if m.soundChip != nil && m.modPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.modPlayer.engine)
			}
			m.modPlayer.Play()
			m.status = MEDIA_STATUS_PLAYING
			m.errCode = MEDIA_ERR_OK
		}
		m.mu.Unlock()
		return
	}

	// SID, PSG, and POKEY use direct loading (LoadData*) and don't need
	// the staging buffer. Only copy into staging for types that read from it.
	needsStaging := typ != MEDIA_TYPE_SID && typ != MEDIA_TYPE_PSG && typ != MEDIA_TYPE_POKEY
	if needsStaging {
		if len(data) > MEDIA_STAGING_SIZE {
			m.mu.Lock()
			defer m.mu.Unlock()
			if reqGen != m.reqGen {
				return
			}
			m.status = MEDIA_STATUS_ERROR
			m.errCode = MEDIA_ERR_TOO_LARGE
			return
		}

		mem := m.bus.GetMemory()
		if MEDIA_STAGING_END >= uint32(len(mem)) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if reqGen != m.reqGen {
				return
			}
			m.status = MEDIA_STATUS_ERROR
			m.errCode = MEDIA_ERR_BAD_FORMAT
			return
		}
		copy(mem[MEDIA_STAGING_BASE:MEDIA_STAGING_BASE+uint32(len(data))], data)
	}

	m.mu.Lock()
	if reqGen != m.reqGen {
		m.mu.Unlock()
		return
	}
	// Keep media state lock held while applying side effects so a newer
	// request cannot interleave and leave stale playback running.
	m.stopPlayersOnly()

	// All formats use direct loading — data is already in memory from os.ReadFile.
	// Release lock during potentially slow decoding (Z80 emulation, SID parsing, etc.).
	var loadErr error
	switch typ {
	case MEDIA_TYPE_SID:
		if m.sidPlayer != nil {
			m.mu.Unlock()
			loadErr = m.sidPlayer.LoadDataWithOptions(data, int(subsong), false, false)
			m.mu.Lock()
			if reqGen != m.reqGen {
				m.mu.Unlock()
				return
			}
			if loadErr != nil {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
				m.mu.Unlock()
				return
			}
			if m.soundChip != nil && m.sidPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.sidPlayer.engine)
			}
			m.sidPlayer.Play()
		}
	case MEDIA_TYPE_PSG:
		if m.psgPlayer != nil {
			ext := strings.ToLower(filepath.Ext(fullPath))
			m.mu.Unlock()
			loadErr = m.psgPlayer.LoadDataWithHint(data, ext)
			m.mu.Lock()
			if reqGen != m.reqGen {
				m.mu.Unlock()
				return
			}
			if loadErr != nil {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
				m.mu.Unlock()
				return
			}
			if m.soundChip != nil && m.psgPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.psgPlayer.engine)
			}
			m.psgPlayer.Play()
		}
	case MEDIA_TYPE_TED:
		if m.tedPlayer != nil {
			m.tedPlayer.HandlePlayWrite(TED_PLAY_PTR, MEDIA_STAGING_BASE)
			m.tedPlayer.HandlePlayWrite(TED_PLAY_LEN, uint32(len(data)))
			m.tedPlayer.HandlePlayWrite(TED_PLAY_CTRL, 1)
			if m.soundChip != nil && m.tedPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.tedPlayer.engine)
			}
		}
	case MEDIA_TYPE_AHX:
		if m.ahxPlayer != nil {
			m.ahxPlayer.HandlePlayWrite(AHX_PLAY_PTR, MEDIA_STAGING_BASE)
			m.ahxPlayer.HandlePlayWrite(AHX_PLAY_LEN, uint32(len(data)))
			m.ahxPlayer.HandlePlayWrite(AHX_SUBSONG, subsong)
			m.ahxPlayer.HandlePlayWrite(AHX_PLAY_CTRL, 1)
			if m.soundChip != nil && m.ahxPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.ahxPlayer.engine)
			}
		}
	case MEDIA_TYPE_POKEY:
		if m.pokeyPlayer != nil {
			m.mu.Unlock()
			loadErr = m.pokeyPlayer.LoadDataWithSubsong(data, int(subsong))
			m.mu.Lock()
			if reqGen != m.reqGen {
				m.mu.Unlock()
				return
			}
			if loadErr != nil {
				m.status = MEDIA_STATUS_ERROR
				m.errCode = MEDIA_ERR_BAD_FORMAT
				m.mu.Unlock()
				return
			}
			if m.soundChip != nil && m.pokeyPlayer.engine != nil {
				m.soundChip.SetSampleTicker(m.pokeyPlayer.engine)
			}
			m.pokeyPlayer.Play()
		}
	default:
		if reqGen == m.reqGen {
			m.status = MEDIA_STATUS_ERROR
			m.errCode = MEDIA_ERR_UNSUPPORTED
		}
		m.mu.Unlock()
		return
	}

	defer m.mu.Unlock()
	if reqGen != m.reqGen {
		return
	}
	m.status = MEDIA_STATUS_PLAYING
	m.errCode = MEDIA_ERR_OK
}

func (m *MediaLoader) stopPlayersOnly() {
	if m.sidPlayer != nil {
		m.sidPlayer.Stop()
	}
	if m.psgPlayer != nil {
		m.psgPlayer.Stop()
	}
	if m.tedPlayer != nil {
		m.tedPlayer.Stop()
	}
	if m.ahxPlayer != nil {
		m.ahxPlayer.Stop()
	}
	if m.pokeyPlayer != nil {
		m.pokeyPlayer.Stop()
	}
	if m.modPlayer != nil {
		m.modPlayer.Stop()
	}
	if m.wavPlayer != nil {
		m.wavPlayer.Stop()
	}
	// Force GC to reclaim large event/frame slices released by player Stop()
	// calls. Without this, deferred collection can spike during the next
	// song's audio callback, causing choppy playback or memory pressure.
	runtime.GC()
}

func (m *MediaLoader) stopAll() {
	m.mu.Lock()
	m.reqGen++
	m.typ = MEDIA_TYPE_NONE
	m.status = MEDIA_STATUS_IDLE
	m.errCode = MEDIA_ERR_OK
	m.mu.Unlock()
	m.stopPlayersOnly()
}

func (m *MediaLoader) refreshStatusLocked() {
	if m.status != MEDIA_STATUS_PLAYING {
		return
	}

	var busy bool
	var playerErr bool

	switch m.typ {
	case MEDIA_TYPE_SID:
		if m.sidPlayer != nil {
			status := m.sidPlayer.HandlePlayRead(SID_PLAY_STATUS)
			busy = m.sidPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_PSG:
		if m.psgPlayer != nil {
			status := m.psgPlayer.HandlePlayRead(PSG_PLAY_STATUS)
			busy = (m.psgPlayer.engine != nil && m.psgPlayer.engine.IsPlaying()) || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_TED:
		if m.tedPlayer != nil {
			status := m.tedPlayer.HandlePlayRead(TED_PLAY_STATUS)
			busy = m.tedPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_AHX:
		if m.ahxPlayer != nil {
			status := m.ahxPlayer.HandlePlayRead(AHX_PLAY_STATUS)
			busy = m.ahxPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_POKEY:
		if m.pokeyPlayer != nil {
			status := m.pokeyPlayer.HandlePlayRead(SAP_PLAY_STATUS)
			busy = m.pokeyPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_MOD:
		if m.modPlayer != nil {
			status := m.modPlayer.HandlePlayRead(MOD_PLAY_STATUS)
			busy = m.modPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	case MEDIA_TYPE_WAV:
		if m.wavPlayer != nil {
			status := m.wavPlayer.HandlePlayRead(WAV_PLAY_STATUS)
			busy = m.wavPlayer.IsPlaying() || (status&0x1) != 0
			playerErr = (status & 0x2) != 0
		}
	}

	if playerErr {
		m.status = MEDIA_STATUS_ERROR
		m.errCode = MEDIA_ERR_BAD_FORMAT
		return
	}
	if !busy {
		m.status = MEDIA_STATUS_IDLE
		m.errCode = MEDIA_ERR_OK
	}
}

func detectMediaType(path string) uint32 {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sid":
		return MEDIA_TYPE_SID
	case ".ym", ".ay", ".sndh",
		".vtx", ".vt", ".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc",
		".vgm", ".vgz", ".snd":
		return MEDIA_TYPE_PSG
	case ".ted", ".prg":
		return MEDIA_TYPE_TED
	case ".ahx":
		return MEDIA_TYPE_AHX
	case ".sap":
		return MEDIA_TYPE_POKEY
	case ".mod":
		return MEDIA_TYPE_MOD
	case ".wav":
		return MEDIA_TYPE_WAV
	default:
		return MEDIA_TYPE_NONE
	}
}

func (m *MediaLoader) sanitizePathLocked(path string) (string, bool) {
	if strings.Contains(path, "..") {
		return "", false
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}
	fullPath := filepath.Join(m.baseDir, path)
	rel, err := filepath.Rel(m.baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return fullPath, true
}

func (m *MediaLoader) readFileNameLocked(ptr uint32) string {
	var name []byte
	addr := ptr
	for {
		b := m.bus.Read8(addr)
		if b == 0 {
			break
		}
		name = append(name, b)
		addr++
		if len(name) > 255 {
			break
		}
	}
	return string(name)
}
