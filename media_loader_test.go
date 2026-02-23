package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		name string
		path string
		want uint32
	}{
		{name: "sid", path: "song.sid", want: MEDIA_TYPE_SID},
		{name: "ym", path: "song.ym", want: MEDIA_TYPE_PSG},
		{name: "ay", path: "song.ay", want: MEDIA_TYPE_PSG},
		{name: "sndh", path: "song.sndh", want: MEDIA_TYPE_PSG},
		{name: "ted", path: "song.ted", want: MEDIA_TYPE_TED},
		{name: "prg alias", path: "song.prg", want: MEDIA_TYPE_TED},
		{name: "ahx", path: "song.ahx", want: MEDIA_TYPE_AHX},
		{name: "unknown", path: "song.bin", want: MEDIA_TYPE_NONE},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectMediaType(tc.path); got != tc.want {
				t.Fatalf("detectMediaType(%q)=%d, want %d", tc.path, got, tc.want)
			}
		})
	}
}

func TestMediaLoaderSanitizePath(t *testing.T) {
	bus := NewMachineBus()
	loader := NewMediaLoader(bus, nil, ".", nil, nil, nil, nil, nil, nil, nil)

	if _, ok := loader.sanitizePathLocked("safe.sid"); !ok {
		t.Fatalf("expected safe relative path to be accepted")
	}
	if _, ok := loader.sanitizePathLocked("../escape.sid"); ok {
		t.Fatalf("expected traversal path to be rejected")
	}
	if p, ok := loader.sanitizePathLocked("/abs/path.sid"); !ok || p != "/abs/path.sid" {
		t.Fatalf("expected absolute path to be accepted, got ok=%v path=%q", ok, p)
	}
	if _, ok := loader.sanitizePathLocked("/abs/../escape.sid"); ok {
		t.Fatalf("expected absolute path with traversal to be rejected")
	}
}

// newMediaLoaderTestEnv creates a MediaLoader with nil players for MMIO tests.
// Uses nil soundChip/players since most tests verify file handling and MMIO state,
// not actual audio playback. Player nil checks in loadAndStart are safe.
func newMediaLoaderTestEnv(t *testing.T, baseDir string) (*MediaLoader, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	loader := NewMediaLoader(bus, nil, baseDir, nil, nil, nil, nil, nil, nil, nil)
	return loader, bus
}

// writeFilenameTobus writes a NUL-terminated filename into bus memory at addr.
func writeFilenameToBus(bus *MachineBus, addr uint32, name string) {
	for i := 0; i < len(name); i++ {
		bus.Write8(addr+uint32(i), name[i])
	}
	bus.Write8(addr+uint32(len(name)), 0)
}

func TestMediaLoader_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "nonexistent.sid")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_SUBSONG, 0)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async goroutine to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s == MEDIA_STATUS_ERROR {
			break
		}
		runtime.Gosched()
	}

	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, MEDIA_STATUS_ERROR)
	}
	if got := loader.HandleRead(MEDIA_ERROR); got != MEDIA_ERR_NOT_FOUND {
		t.Fatalf("error=%d, want %d (NOT_FOUND)", got, MEDIA_ERR_NOT_FOUND)
	}
}

func TestMediaLoader_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	// Create a file with unsupported extension
	os.WriteFile(filepath.Join(dir, "file.bin"), []byte("data"), 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "file.bin")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Unsupported is a synchronous error (detected before goroutine)
	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, MEDIA_STATUS_ERROR)
	}
	if got := loader.HandleRead(MEDIA_ERROR); got != MEDIA_ERR_UNSUPPORTED {
		t.Fatalf("error=%d, want %d (UNSUPPORTED)", got, MEDIA_ERR_UNSUPPORTED)
	}
}

func TestMediaLoader_PathTraversalReject(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "../escape.sid")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, MEDIA_STATUS_ERROR)
	}
	if got := loader.HandleRead(MEDIA_ERROR); got != MEDIA_ERR_PATH_INVALID {
		t.Fatalf("error=%d, want %d (PATH_INVALID)", got, MEDIA_ERR_PATH_INVALID)
	}
}

func TestMediaLoader_TooLargePayload(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	// Create a file larger than MEDIA_STAGING_SIZE (64KB)
	bigData := make([]byte, MEDIA_STAGING_SIZE+1)
	os.WriteFile(filepath.Join(dir, "huge.sid"), bigData, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "huge.sid")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s == MEDIA_STATUS_ERROR {
			break
		}
		runtime.Gosched()
	}

	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, MEDIA_STATUS_ERROR)
	}
	if got := loader.HandleRead(MEDIA_ERROR); got != MEDIA_ERR_TOO_LARGE {
		t.Fatalf("error=%d, want %d (TOO_LARGE)", got, MEDIA_ERR_TOO_LARGE)
	}
}

func TestMediaLoader_StagingDataCopied(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	// Write a small test file (doesn't need to be valid SID, just test staging copy)
	testData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x42}
	os.WriteFile(filepath.Join(dir, "test.sid"), testData, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "test.sid")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async completion
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	// Verify data was copied to staging area
	mem := bus.GetMemory()
	for i, b := range testData {
		if mem[MEDIA_STAGING_BASE+uint32(i)] != b {
			t.Fatalf("staging[%d]=%#x, want %#x", i, mem[MEDIA_STAGING_BASE+uint32(i)], b)
		}
	}
}

func TestMediaLoader_StopResetsState(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	// Create a valid file so we can transition to playing
	os.WriteFile(filepath.Join(dir, "song.sid"), []byte{0x00}, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "song.sid")

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for load to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	// Now stop
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)

	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_IDLE {
		t.Fatalf("status after STOP=%d, want %d (IDLE)", got, MEDIA_STATUS_IDLE)
	}
	if got := loader.HandleRead(MEDIA_ERROR); got != MEDIA_ERR_OK {
		t.Fatalf("error after STOP=%d, want %d (OK)", got, MEDIA_ERR_OK)
	}
	if got := loader.HandleRead(MEDIA_TYPE); got != MEDIA_TYPE_NONE {
		t.Fatalf("type after STOP=%d, want %d (NONE)", got, MEDIA_TYPE_NONE)
	}
}

func TestMediaLoader_RequestGenSupersedes(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	// First request: file not found (async error)
	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "missing.sid")
	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Immediately fire second request (stop) which increments reqGen
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)

	// The stale goroutine from first request should not overwrite STOP state
	time.Sleep(100 * time.Millisecond)

	if got := loader.HandleRead(MEDIA_STATUS); got != MEDIA_STATUS_IDLE {
		t.Fatalf("status after supersede=%d, want %d (IDLE)", got, MEDIA_STATUS_IDLE)
	}
}

// TestMediaLoader_SIDPlayback exercises the full SOUND PLAY path for SID files
// with real players wired up, matching the main.go wiring.
func TestMediaLoader_SIDPlayback(t *testing.T) {
	sidPath := "sdk/examples/assets/music/Edge_of_Disgrace.sid"
	if _, err := os.Stat(sidPath); os.IsNotExist(err) {
		t.Skip("SID test file not available")
	}

	bus := NewMachineBus()
	soundChip := newTestSoundChip()
	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)
	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sidPlayer := NewSIDPlayer(sidEngine)
	sidPlayer.AttachBus(bus)

	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)
	bus.MapIO(SID_BASE, SID_END, sidEngine.HandleRead, sidEngine.HandleWrite)
	bus.MapIO(SID_PLAY_PTR, SID_PLAY_STATUS+3, sidPlayer.HandlePlayRead, sidPlayer.HandlePlayWrite)

	loader := NewMediaLoader(bus, soundChip, ".", psgPlayer, sidPlayer, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	// Mimic what EhBASIC does: write filename to bus, set MEDIA_NAME_PTR, trigger play
	nameAddr := uint32(0x6FF000) // FILE_NAME_BUF
	writeFilenameToBus(bus, nameAddr, sidPath)

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_SUBSONG, 0)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async load to complete
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	status := loader.HandleRead(MEDIA_STATUS)
	errCode := loader.HandleRead(MEDIA_ERROR)
	typ := loader.HandleRead(MEDIA_TYPE)

	if status == MEDIA_STATUS_ERROR {
		t.Fatalf("SOUND PLAY SID failed: status=ERROR, errCode=%d, type=%d", errCode, typ)
	}
	if status != MEDIA_STATUS_PLAYING {
		t.Fatalf("SOUND PLAY SID: status=%d (want PLAYING=%d), errCode=%d, type=%d",
			status, MEDIA_STATUS_PLAYING, errCode, typ)
	}
	if typ != MEDIA_TYPE_SID {
		t.Fatalf("type=%d, want %d (SID)", typ, MEDIA_TYPE_SID)
	}
	if !sidPlayer.IsPlaying() {
		t.Error("SID player not playing after SOUND PLAY")
	}
}

// TestMediaLoader_PSGTrackerPlayback exercises SOUND PLAY for a PT3 tracker file.
func TestMediaLoader_PSGTrackerPlayback(t *testing.T) {
	pt3Path := "testdata/music/test_pt3.pt3"
	if _, err := os.Stat(pt3Path); os.IsNotExist(err) {
		t.Skip("PT3 test file not available")
	}

	bus := NewMachineBus()
	soundChip := newTestSoundChip()
	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)

	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)

	loader := NewMediaLoader(bus, soundChip, ".", psgPlayer, nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	nameAddr := uint32(0x6FF000)
	writeFilenameToBus(bus, nameAddr, pt3Path)

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_SUBSONG, 0)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	status := loader.HandleRead(MEDIA_STATUS)
	errCode := loader.HandleRead(MEDIA_ERROR)

	if status == MEDIA_STATUS_ERROR {
		t.Fatalf("SOUND PLAY PT3 failed: status=ERROR, errCode=%d", errCode)
	}
	if status != MEDIA_STATUS_PLAYING {
		t.Fatalf("SOUND PLAY PT3: status=%d (want PLAYING=%d), errCode=%d",
			status, MEDIA_STATUS_PLAYING, errCode)
	}
	if !psgEngine.IsPlaying() {
		t.Error("PSG engine not playing after SOUND PLAY PT3")
	}
}

// TestMediaLoader_BusPath_SIDPlayback exercises the SOUND PLAY path for SID files
// using bus.Write32/Read32 instead of direct loader.HandleWrite/HandleRead.
// This matches the actual EhBASIC path where writes go through the bus.
func TestMediaLoader_BusPath_SIDPlayback(t *testing.T) {
	sidPath := "sdk/examples/assets/music/Edge_of_Disgrace.sid"
	if _, err := os.Stat(sidPath); os.IsNotExist(err) {
		t.Skip("SID test file not available")
	}

	bus := NewMachineBus()
	soundChip := newTestSoundChip()
	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)
	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sidPlayer := NewSIDPlayer(sidEngine)
	sidPlayer.AttachBus(bus)

	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)
	bus.MapIO(SID_BASE, SID_END, sidEngine.HandleRead, sidEngine.HandleWrite)
	bus.MapIO(SID_PLAY_PTR, SID_PLAY_STATUS+3, sidPlayer.HandlePlayRead, sidPlayer.HandlePlayWrite)

	loader := NewMediaLoader(bus, soundChip, ".", psgPlayer, sidPlayer, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	// Use bus.Write32 — this is the actual path EhBASIC takes
	nameAddr := uint32(0x6FF000)
	writeFilenameToBus(bus, nameAddr, sidPath)

	bus.Write32(MEDIA_NAME_PTR, nameAddr)
	bus.Write32(MEDIA_SUBSONG, 0)
	bus.Write32(MEDIA_CTRL, MEDIA_OP_STOP)
	bus.Write32(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async load to complete, polling via bus.Read32
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := bus.Read32(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	status := bus.Read32(MEDIA_STATUS)
	errCode := bus.Read32(MEDIA_ERROR)
	typ := bus.Read32(MEDIA_TYPE)

	if status == MEDIA_STATUS_ERROR {
		t.Fatalf("BusPath SOUND PLAY SID failed: status=ERROR, errCode=%d, type=%d", errCode, typ)
	}
	if status != MEDIA_STATUS_PLAYING {
		t.Fatalf("BusPath SOUND PLAY SID: status=%d (want PLAYING=%d), errCode=%d, type=%d",
			status, MEDIA_STATUS_PLAYING, errCode, typ)
	}
	if typ != MEDIA_TYPE_SID {
		t.Fatalf("type=%d, want %d (SID)", typ, MEDIA_TYPE_SID)
	}
	if !sidPlayer.IsPlaying() {
		t.Error("SID player not playing after bus-path SOUND PLAY")
	}
}

// TestMediaLoader_AudioPipeline_PSG verifies actual audio output through the full
// SOUND PLAY pipeline: load → set sample ticker → TickSample → GenerateSample → non-zero samples.
func TestMediaLoader_AudioPipeline_PSG(t *testing.T) {
	pt3Path := "testdata/music/test_pt3.pt3"
	if _, err := os.Stat(pt3Path); os.IsNotExist(err) {
		t.Skip("PT3 test file not available")
	}

	bus := NewMachineBus()
	soundChip := newTestSoundChip()
	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)

	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)

	loader := NewMediaLoader(bus, soundChip, ".", psgPlayer, nil, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	nameAddr := uint32(0x6FF000)
	writeFilenameToBus(bus, nameAddr, pt3Path)

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_SUBSONG, 0)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async load to reach PLAYING state
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	status := loader.HandleRead(MEDIA_STATUS)
	if status == MEDIA_STATUS_ERROR {
		t.Fatalf("SOUND PLAY PT3 failed: errCode=%d", loader.HandleRead(MEDIA_ERROR))
	}
	if status != MEDIA_STATUS_PLAYING {
		t.Fatalf("status=%d, want PLAYING=%d", status, MEDIA_STATUS_PLAYING)
	}
	if !psgEngine.IsPlaying() {
		t.Fatal("PSG engine not playing after SOUND PLAY")
	}

	// Generate samples via ReadSample() and verify at least some are non-zero.
	// ReadSample calls TickSample (advancing PSG events) then GenerateSample.
	const numSamples = 44100 // 1 second at 44.1kHz
	nonZero := 0
	for range numSamples {
		s := soundChip.ReadSample()
		if s != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Fatalf("all %d samples were zero — audio pipeline not producing output", numSamples)
	}
	t.Logf("audio pipeline: %d/%d non-zero samples (%.1f%%)", nonZero, numSamples, float64(nonZero)/float64(numSamples)*100)
}

// TestMediaLoader_AudioPipeline_SID verifies the full SID audio pipeline:
// load via MediaLoader MMIO → SID rendering → TickSample → GenerateSample → non-zero audio.
func TestMediaLoader_AudioPipeline_SID(t *testing.T) {
	sidPath := "sdk/examples/assets/music/Edge_of_Disgrace.sid"
	if _, err := os.Stat(sidPath); os.IsNotExist(err) {
		t.Skip("SID test file not available")
	}

	bus := NewMachineBus()
	soundChip := newTestSoundChip()
	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sidPlayer := NewSIDPlayer(sidEngine)
	sidPlayer.AttachBus(bus)

	bus.MapIO(SID_BASE, SID_END, sidEngine.HandleRead, sidEngine.HandleWrite)
	bus.MapIO(SID_PLAY_PTR, SID_PLAY_STATUS+3, sidPlayer.HandlePlayRead, sidPlayer.HandlePlayWrite)

	loader := NewMediaLoader(bus, soundChip, ".", nil, sidPlayer, nil, nil, nil, nil, nil)
	bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	nameAddr := uint32(0x6FF000)
	writeFilenameToBus(bus, nameAddr, sidPath)

	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_SUBSONG, 0)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	// Wait for async load to reach PLAYING state
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s := loader.HandleRead(MEDIA_STATUS)
		if s != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}

	status := loader.HandleRead(MEDIA_STATUS)
	if status == MEDIA_STATUS_ERROR {
		t.Fatalf("SOUND PLAY SID failed: errCode=%d", loader.HandleRead(MEDIA_ERROR))
	}
	if status != MEDIA_STATUS_PLAYING {
		t.Fatalf("status=%d, want PLAYING=%d, errCode=%d", status, MEDIA_STATUS_PLAYING, loader.HandleRead(MEDIA_ERROR))
	}
	if !sidPlayer.IsPlaying() {
		t.Fatal("SID player not playing after SOUND PLAY")
	}

	// Verify SID engine has events and samples
	t.Logf("SID: events=%d, totalSamples=%d, enabled=%v, playing=%v",
		len(sidEngine.events), sidEngine.totalSamples,
		sidEngine.enabled.Load(), sidEngine.IsPlaying())

	// Generate samples via ReadSample() — same path OTO uses in real binary.
	const numSamples = 44100 // 1 second at 44.1kHz
	nonZero := 0
	for range numSamples {
		s := soundChip.ReadSample()
		if s != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Fatalf("all %d samples were zero — SID audio pipeline not producing output", numSamples)
	}
	t.Logf("SID audio pipeline: %d/%d non-zero samples (%.1f%%)", nonZero, numSamples, float64(nonZero)/float64(numSamples)*100)
}

func TestMediaLoader_TypeDetectedOnPlay(t *testing.T) {
	dir := t.TempDir()
	loader, bus := newMediaLoaderTestEnv(t, dir)

	tests := []struct {
		name string
		file string
		want uint32
	}{
		{"sid", "test.sid", MEDIA_TYPE_SID},
		{"ym", "test.ym", MEDIA_TYPE_PSG},
		{"ahx", "test.ahx", MEDIA_TYPE_AHX},
		{"ted", "test.ted", MEDIA_TYPE_TED},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create file
			os.WriteFile(filepath.Join(dir, tc.file), []byte{0x00}, 0644)

			nameAddr := uint32(0x1000)
			writeFilenameToBus(bus, nameAddr, tc.file)

			loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
			loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

			// Wait for load
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				s := loader.HandleRead(MEDIA_STATUS)
				if s != MEDIA_STATUS_LOADING {
					break
				}
				runtime.Gosched()
			}

			if got := loader.HandleRead(MEDIA_TYPE); got != tc.want {
				t.Fatalf("type=%d, want %d", got, tc.want)
			}

			// Clean up for next subtest
			loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_STOP)
		})
	}
}
