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
	loader := NewMediaLoader(bus, nil, ".", nil, nil, nil, nil)

	if _, ok := loader.sanitizePathLocked("safe.sid"); !ok {
		t.Fatalf("expected safe relative path to be accepted")
	}
	if _, ok := loader.sanitizePathLocked("../escape.sid"); ok {
		t.Fatalf("expected traversal path to be rejected")
	}
	if _, ok := loader.sanitizePathLocked("/abs/path.sid"); ok {
		t.Fatalf("expected absolute path to be rejected")
	}
}

// newMediaLoaderTestEnv creates a MediaLoader with nil players for MMIO tests.
// Uses nil soundChip/players since most tests verify file handling and MMIO state,
// not actual audio playback. Player nil checks in loadAndStart are safe.
func newMediaLoaderTestEnv(t *testing.T, baseDir string) (*MediaLoader, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	loader := NewMediaLoader(bus, nil, baseDir, nil, nil, nil, nil)
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
