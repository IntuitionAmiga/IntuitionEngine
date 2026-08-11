//go:build !js

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

const (
	boingTestULAData = 0xF2014
	boingTestWait    = 0xF2580
	boingImageSHA256 = "538de509d2850b789d6d4e02fccffed5bc2c317662eedfc6273da6ec790d3d93"
	boingMapSHA256   = "c9b61bfd814b14ae988164ca60845cf61108193d11f4c11f7caa1c74c648a636"
)

func TestULABoingCommittedFixtures(t *testing.T) {
	for path, want := range map[string]string{
		"sdk/examples/prebuilt/ula_boing_ie64.ie64": boingImageSHA256,
		"sdk/examples/prebuilt/ula_boing_ie64.map":  boingMapSHA256,
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read committed ULA Boing fixture %s: %v", path, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != want {
			t.Fatalf("committed ULA Boing fixture %s hash = %s, want %s; rebuild with make ula-boing-ie64", path, got, want)
		}
	}
}

func boingMapSymbol(t *testing.T, contents []byte, name string) uint32 {
	t.Helper()
	re := regexp.MustCompile(`(?m)^0x([0-9a-fA-F]+)\s+` + regexp.QuoteMeta(name) + `$`)
	match := re.FindSubmatch(contents)
	if match == nil {
		t.Fatalf("linker map has no %s symbol", name)
	}
	value, err := strconv.ParseUint(string(match[1]), 16, 32)
	if err != nil {
		t.Fatalf("parse %s address: %v", name, err)
	}
	return uint32(value)
}

func boingRunOneFrame(t *testing.T, jit, forceImpact bool) ([]byte, []byte, []byte, []uint64, []uint64, []byte) {
	t.Helper()
	image, err := os.ReadFile("sdk/examples/prebuilt/ula_boing_ie64.ie64")
	if err != nil {
		t.Fatalf("read ULA Boing image: %v", err)
	}
	linkMap, err := os.ReadFile("sdk/examples/prebuilt/ula_boing_ie64.map")
	if err != nil {
		t.Fatalf("read ULA Boing linker map: %v", err)
	}
	diag := boingMapSymbol(t, linkMap, "boing_diagnostics")
	bus := NewMachineBus()
	bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))
	var ulaMu sync.Mutex
	ulaWrites := make([]byte, ULA_VRAM_SIZE)
	ulaWriteCount := 0
	var completeULAFrame []byte
	var audioMu sync.Mutex
	var audioWrites []uint64
	var musicWrites []uint64
	bus.MapIONoShadow(ULA_BASE, ULA_REG_END, func(uint32) uint32 { return 0 }, func(uint32, uint32) {})
	bus.MapIOByte(ULA_BASE, ULA_REG_END, func(addr uint32, value uint8) {
		if addr == boingTestULAData {
			ulaMu.Lock()
			if ulaWriteCount < len(ulaWrites) {
				ulaWrites[ulaWriteCount] = value
			}
			ulaWriteCount++
			ulaMu.Unlock()
		}
	})
	bus.MapIONoShadow(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, func(uint32) uint32 { return 0 }, func(uint32, uint32) {})
	bus.MapIO64NoShadow(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, func(uint32) uint64 { return 0 }, func(addr uint32, value uint64) {
		ulaMu.Lock()
		offset := int(addr - ULA_VRAM_AP_BASE)
		if offset >= 0 && offset+8 <= len(ulaWrites) {
			binary.LittleEndian.PutUint64(ulaWrites[offset:offset+8], value)
			ulaWriteCount += 8
			if offset+8 == len(ulaWrites) {
				completeULAFrame = append(completeULAFrame[:0], ulaWrites...)
			}
		}
		ulaMu.Unlock()
	})
	bus.MapIONoShadow(AUDIO_CTRL, AUDIO_REG_END, func(uint32) uint32 { return 0 }, func(addr, value uint32) {
		audioMu.Lock()
		audioWrites = append(audioWrites, uint64(addr)<<32|uint64(value))
		audioMu.Unlock()
	})
	bus.MapIONoShadow(MIDI_PLAY_PTR, MIDI_END, func(uint32) uint32 { return 0 }, func(addr, value uint32) {
		audioMu.Lock()
		musicWrites = append(musicWrites, uint64(addr)<<32|uint64(value))
		audioMu.Unlock()
	})
	bus.MapIONoShadow(MEDIA_LOADER_BASE, MEDIA_LOADER_REGION_END, func(uint32) uint32 { return 0 }, func(addr, value uint32) {
		audioMu.Lock()
		musicWrites = append(musicWrites, uint64(addr)<<32|uint64(value))
		audioMu.Unlock()
	})
	bus.MapIONoShadow(boingTestWait, boingTestWait+3, func(uint32) uint32 { return 0 }, func(uint32, uint32) {})

	cpu := NewCPU64(bus)
	if err := cpu.LoadFlatProgramBytes(image); err != nil {
		t.Fatalf("load ULA Boing image: %v", err)
	}
	if forceImpact {
		binary.LittleEndian.PutUint32(cpu.memory[diag+20:diag+24], math.Float32bits(131.9))
		binary.LittleEndian.PutUint32(cpu.memory[diag+28:diag+32], math.Float32bits(3.0))
	}
	cpu.jitEnabled = jit
	done := make(chan struct{})
	go func() {
		if jit {
			cpu.ExecuteJIT()
		} else {
			cpu.Execute()
		}
		close(done)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if binary.LittleEndian.Uint32(cpu.memory[diag+12:diag+16]) != 0 {
			cpu.running.Store(false)
			<-done
			ulaMu.Lock()
			writes := append([]byte(nil), completeULAFrame...)
			ulaMu.Unlock()
			audioMu.Lock()
			soundWrites := append([]uint64(nil), audioWrites...)
			guestMusicWrites := append([]uint64(nil), musicWrites...)
			audioMu.Unlock()
			var musicData []byte
			for _, write := range guestMusicWrites {
				if uint32(write>>32) != MIDI_PLAY_PTR {
					continue
				}
				ptr := uint32(write)
				for _, lengthWrite := range guestMusicWrites {
					if uint32(lengthWrite>>32) != MIDI_PLAY_LEN {
						continue
					}
					length := uint32(lengthWrite)
					if ptr+length <= uint32(len(cpu.memory)) {
						musicData = append([]byte(nil), cpu.memory[ptr:ptr+length]...)
					}
				}
			}
			return writes, writes,
				append([]byte(nil), cpu.memory[diag:diag+144]...), soundWrites, guestMusicWrites, musicData
		}
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	<-done
	t.Fatalf("ULA Boing did not commit a frame (jit=%v PC=%#x started=%d rendered=%d)",
		jit, cpu.PC,
		binary.LittleEndian.Uint32(cpu.memory[diag+4:diag+8]),
		binary.LittleEndian.Uint32(cpu.memory[diag+8:diag+12]))
	return nil, nil, nil, nil, nil, nil
}

func TestULABoingIE64InterpreterJITFrameParity(t *testing.T) {
	interpreterFrame, interpreterWrites, interpreterDiagnostics, _, _, _ := boingRunOneFrame(t, false, false)
	jitFrame, jitWrites, jitDiagnostics, _, _, _ := boingRunOneFrame(t, true, false)
	if !bytes.Equal(interpreterFrame, jitFrame) {
		t.Fatal("interpreter and JIT generated different ULA frame bytes")
	}
	if len(interpreterWrites) != ULA_VRAM_SIZE || len(jitWrites) != ULA_VRAM_SIZE {
		t.Fatalf("ULA upload sizes interpreter=%d JIT=%d, want %d",
			len(interpreterWrites), len(jitWrites), ULA_VRAM_SIZE)
	}
	if !bytes.Equal(interpreterFrame, interpreterWrites) || !bytes.Equal(jitFrame, jitWrites) {
		t.Fatal("captured completed ULA upload changed during collection")
	}
	for name, diagnostics := range map[string][]byte{"interpreter": interpreterDiagnostics, "JIT": jitDiagnostics} {
		if rendered, committed := binary.LittleEndian.Uint32(diagnostics[8:12]), binary.LittleEndian.Uint32(diagnostics[12:16]); rendered != committed {
			t.Fatalf("%s committed frame %d before rendered frame %d", name, committed, rendered)
		}
	}

	redWhite := 0
	boundary := 0
	for _, attr := range jitFrame[ULA_BITMAP_SIZE:] {
		paper, ink := (attr>>3)&7, attr&7
		if paper == 2 && ink == 7 {
			redWhite++
		}
		if paper == 0 && (ink == 2 || ink == 7) {
			boundary++
		}
	}
	if redWhite == 0 || boundary == 0 {
		t.Fatalf("missing interior or controlled boundary cells: red/white=%d boundary=%d", redWhite, boundary)
	}
	if capture := os.Getenv("IE_ULA_BOING_TEST_CAPTURE"); capture != "" {
		ula := NewULAEngine(nil)
		ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE)
		for offset, value := range jitFrame {
			ula.HandleVRAMWrite(uint16(offset), value)
		}
		rgba := ula.RenderFrame()
		width, height := ula.GetDimensions()
		shot := image.NewRGBA(image.Rect(0, 0, width, height))
		copy(shot.Pix, rgba)
		file, err := os.Create(capture)
		if err != nil {
			t.Fatalf("create capture: %v", err)
		}
		if err := png.Encode(file, shot); err != nil {
			file.Close()
			t.Fatalf("encode capture: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close capture: %v", err)
		}
	}
}

func TestULABoingIE64CollisionTriggersOneSoundAndSquash(t *testing.T) {
	_, _, diagnostics, soundWrites, _, _ := boingRunOneFrame(t, true, true)
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(diagnostics[offset : offset+4]) }
	if got := u32(44); got != 1 {
		t.Fatalf("impact count = %d, want 1", got)
	}
	if got := u32(56); got != 1 {
		t.Fatalf("audio trigger count = %d, want 1", got)
	}
	if got := u32(60); got == 0 || got > u32(4) {
		t.Fatalf("last audio frame = %d, started frame = %d", got, u32(4))
	}
	if got := u32(64); got < 1 || got > 2 {
		t.Fatalf("squash remaining after impact = %d, want 1 or 2", got)
	}
	if got := math.Float32frombits(u32(48)); got < 3.0 || got > 4.0 {
		t.Fatalf("recorded impact velocity = %f, want the forced floor collision", got)
	}
	wrote := func(address uint32) bool {
		for _, write := range soundWrites {
			if uint32(write>>32) == address {
				return true
			}
		}
		return false
	}
	for _, address := range []uint32{AUDIO_CTRL, REVERB_MIX, REVERB_DECAY, FLEX_CH0_BASE + FLEX_OFF_PHASE, FLEX_CH0_BASE + FLEX_OFF_CTRL, FLEX_CH1_BASE + FLEX_OFF_PHASE, FLEX_CH1_BASE + FLEX_OFF_CTRL} {
		if !wrote(address) {
			t.Fatalf("guest did not write required SoundChip register %#x", address)
		}
	}
}

func TestULABoingStartsEmbeddedQuietLoopingMIDIBehindEffects(t *testing.T) {
	_, _, _, soundWrites, musicWrites, musicData := boingRunOneFrame(t, true, true)
	wantMusic, err := os.ReadFile("sdk/examples/assets/music/shadowofthebeast.mid")
	if err != nil {
		t.Fatalf("read source MIDI: %v", err)
	}
	if !bytes.Equal(musicData, wantMusic) {
		t.Fatalf("embedded MIDI is %d bytes, want exact %d-byte source asset; writes=%#x", len(musicData), len(wantMusic), musicWrites)
	}
	find := func(writes []uint64, address uint32) (uint32, int, bool) {
		for index, write := range writes {
			if uint32(write>>32) == address {
				return uint32(write), index, true
			}
		}
		return 0, -1, false
	}
	volume, volumeIndex, ok := find(musicWrites, MIDI_VOLUME)
	if !ok || volume != 64 {
		t.Fatalf("MIDI volume = %d, present=%v, want 64", volume, ok)
	}
	play, playIndex, ok := find(musicWrites, MIDI_PLAY_CTRL)
	if !ok || play != 0x05 {
		t.Fatalf("MIDI play control = %#x, present=%v, want play+loop %#x", play, ok, uint32(0x05))
	}
	_, pointerIndex, pointerOK := find(musicWrites, MIDI_PLAY_PTR)
	_, lengthIndex, lengthOK := find(musicWrites, MIDI_PLAY_LEN)
	if !pointerOK || !lengthOK || volumeIndex > playIndex || pointerIndex > playIndex || lengthIndex > playIndex {
		t.Fatal("MIDI pointer, length and volume must precede direct play")
	}
	if _, _, mediaPlay := find(musicWrites, MEDIA_CTRL); mediaPlay {
		t.Fatal("self-contained demo must not invoke the host media loader")
	}
	bodyVolume, _, ok := find(soundWrites, FLEX_CH0_BASE+FLEX_OFF_VOL)
	if !ok || bodyVolume <= volume {
		t.Fatalf("effect body volume = %d, present=%v, want greater than MIDI volume %d", bodyVolume, ok, volume)
	}
}

func TestULABoingBitmapAddressingIsBijective(t *testing.T) {
	seen := make([]bool, ULA_BITMAP_SIZE)
	for y := uint32(0); y < ULA_DISPLAY_HEIGHT; y++ {
		for x := uint32(0); x < ULA_CELLS_X; x++ {
			off := ((y & 0xc0) << 5) | ((y & 7) << 8) | ((y & 0x38) << 2) | x
			if off >= ULA_BITMAP_SIZE || seen[off] {
				t.Fatalf("invalid or duplicate ULA offset %#x for (%d,%d)", off, x, y)
			}
			seen[off] = true
		}
	}
	for off, ok := range seen {
		if !ok {
			t.Fatalf("ULA bitmap byte %d was not addressed", off)
		}
	}
}
