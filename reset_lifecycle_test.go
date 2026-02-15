// reset_lifecycle_test.go - Tests for hard reset, CPU lifecycle, compositor lifecycle, and IPC

//go:build headless

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestCPU_IE32_StartExecution_Stop tests IE32 CPU lifecycle via StartExecution/Stop.
func TestCPU_IE32_StartExecution_Stop(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	// Write a HALT instruction at PROG_START so Execute exits immediately
	bus.Write8(PROG_START, 0xFF)

	cpu.StartExecution()
	time.Sleep(50 * time.Millisecond)
	cpu.Stop()

	// Verify Stop() is idempotent
	cpu.Stop()
}

// TestCPU_IE64_StartExecution_Stop tests IE64 CPU lifecycle via StartExecution/Stop.
func TestCPU_IE64_StartExecution_Stop(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write a HALT instruction (0xFF) at PROG_START
	bus.Write8(PROG_START, 0xFF)

	cpu.StartExecution()
	time.Sleep(50 * time.Millisecond)
	cpu.Stop()

	// Verify Stop() is idempotent
	cpu.Stop()
}

// TestCPU_StartExecution_DoubleStart tests that double-start is idempotent.
func TestCPU_StartExecution_DoubleStart(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	bus.Write8(PROG_START, 0xFF)

	cpu.StartExecution()
	cpu.StartExecution() // Should be no-op
	time.Sleep(50 * time.Millisecond)
	cpu.Stop()
}

// TestCPU_StopJoin_NoLeak verifies Stop() doesn't leak goroutines across repeated cycles.
func TestCPU_StopJoin_NoLeak(t *testing.T) {
	bus := NewMachineBus()

	initial := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		cpu := NewCPU(bus)
		// Write infinite loop: JMP PROG_START
		bus.Write8(PROG_START, 0x0A)   // JMP opcode
		bus.Write8(PROG_START+1, 0x00) // addr low
		bus.Write8(PROG_START+2, 0x10) // addr high (0x1000)
		bus.Write8(PROG_START+3, 0x00)
		bus.Write8(PROG_START+4, 0x00)

		cpu.StartExecution()
		time.Sleep(10 * time.Millisecond)
		cpu.Stop()
	}

	// Allow goroutines to settle
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	final := runtime.NumGoroutine()
	// Allow some slack for background goroutines (GC, runtime, etc.)
	if final > initial+5 {
		t.Errorf("goroutine leak: started with %d, ended with %d", initial, final)
	}
}

// TestCPU_StopJoin_Concurrent tests that concurrent Stop() calls don't panic.
func TestCPU_StopJoin_Concurrent(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	// Write infinite loop
	bus.Write8(PROG_START, 0x0A)
	bus.Write8(PROG_START+1, 0x00)
	bus.Write8(PROG_START+2, 0x10)
	bus.Write8(PROG_START+3, 0x00)
	bus.Write8(PROG_START+4, 0x00)

	cpu.StartExecution()
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cpu.Stop()
		}()
	}
	wg.Wait()
}

// TestModeFromExtension validates extension-to-mode mapping.
func TestModeFromExtension(t *testing.T) {
	tests := []struct {
		path string
		mode string
		err  bool
	}{
		{"program.ie32", "ie32", false},
		{"program.iex", "ie32", false},
		{"program.ie64", "ie64", false},
		{"program.ie65", "6502", false},
		{"program.ie68", "m68k", false},
		{"program.ie80", "z80", false},
		{"program.ie86", "x86", false},
		{"program.IE32", "ie32", false}, // case insensitive
		{"program.txt", "", true},
		{"program", "", true},
	}

	for _, tt := range tests {
		mode, err := modeFromExtension(tt.path)
		if tt.err && err == nil {
			t.Errorf("modeFromExtension(%q) expected error", tt.path)
		}
		if !tt.err && err != nil {
			t.Errorf("modeFromExtension(%q) unexpected error: %v", tt.path, err)
		}
		if mode != tt.mode {
			t.Errorf("modeFromExtension(%q) = %q, want %q", tt.path, mode, tt.mode)
		}
	}
}

// TestCreateCPURunner validates CPU runner factory for all modes.
func TestCreateCPURunner(t *testing.T) {
	bus := NewMachineBus()
	vc, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		// In headless mode, video chip creation may fail - skip
		t.Skipf("NewVideoChip failed (expected in headless): %v", err)
	}

	modes := []string{"ie32", "ie64", "m68k", "z80", "x86", "6502"}
	for _, mode := range modes {
		runner, err := createCPURunner(mode, bus, vc, nil, nil)
		if err != nil {
			t.Errorf("createCPURunner(%q) error: %v", mode, err)
			continue
		}
		if runner == nil {
			t.Errorf("createCPURunner(%q) returned nil runner", mode)
		}
	}

	// Invalid mode
	_, err = createCPURunner("invalid", bus, vc, nil, nil)
	if err == nil {
		t.Error("createCPURunner(\"invalid\") expected error")
	}
}

// TestBuildReloadClosure verifies reload closures for all CPU modes.
func TestBuildReloadClosure(t *testing.T) {
	bus := NewMachineBus()
	vc, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Skipf("NewVideoChip failed (expected in headless): %v", err)
	}
	programBytes := []byte{0x01, 0x02, 0x03, 0x04}

	modes := []string{"ie32", "ie64", "m68k", "z80", "x86", "6502"}
	for _, mode := range modes {
		runner, err := createCPURunner(mode, bus, vc, nil, nil)
		if err != nil {
			t.Fatalf("createCPURunner(%q) error: %v", mode, err)
		}
		closure := buildReloadClosure(mode, runner, programBytes, bus)
		if closure == nil {
			t.Errorf("buildReloadClosure(%q) returned nil", mode)
			continue
		}
		// Should not panic
		closure()
	}
}

// testSocketPath returns an isolated socket path for a test.
func testSocketPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "test.sock")
}

// TestIPC_SocketLifecycle tests IPC server start and stop.
func TestIPC_SocketLifecycle(t *testing.T) {
	sockPath := testSocketPath(t)
	server, err := newIPCServerAt(sockPath, func(path string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("newIPCServerAt failed: %v", err)
	}
	server.Start()
	defer server.Stop()

	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Fatal("socket file not created")
	}
}

// TestIPC_SendOpen tests IPC client-server communication.
func TestIPC_SendOpen(t *testing.T) {
	sockPath := testSocketPath(t)
	handled := make(chan string, 1)
	server, err := newIPCServerAt(sockPath, func(path string) error {
		handled <- path
		return nil
	})
	if err != nil {
		t.Fatalf("newIPCServerAt failed: %v", err)
	}
	server.Start()
	defer server.Stop()

	// Create a temp file with a valid extension
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.ie32")
	if err := os.WriteFile(tmpFile, []byte{0xFF}, 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err = sendIPCOpenAt(sockPath, tmpFile)
	if err != nil {
		t.Fatalf("sendIPCOpenAt failed: %v", err)
	}

	select {
	case path := <-handled:
		if path != tmpFile {
			t.Errorf("handler got path %q, want %q", path, tmpFile)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler not called within timeout")
	}
}

// TestIPC_ValidateRejectsRelative tests that IPC rejects relative paths.
func TestIPC_ValidateRejectsRelative(t *testing.T) {
	err := validateIPCPath("relative/path.ie32")
	if err == nil {
		t.Error("expected error for relative path")
	}
}

// TestIPC_ValidateRejectsBadExtension tests that IPC rejects unknown extensions.
func TestIPC_ValidateRejectsBadExtension(t *testing.T) {
	err := validateIPCPath("/tmp/file.txt")
	if err == nil {
		t.Error("expected error for .txt extension")
	}
}

// TestIPC_StaleSocketCleanup tests stale socket detection and cleanup.
func TestIPC_StaleSocketCleanup(t *testing.T) {
	sockPath := testSocketPath(t)

	// Create a stale socket file (not listening)
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("failed to create stale socket: %v", err)
	}
	f.Close()

	// newIPCServerAt should clean up stale socket and bind successfully
	server, err := newIPCServerAt(sockPath, func(path string) error { return nil })
	if err != nil {
		t.Fatalf("newIPCServerAt failed with stale socket: %v", err)
	}
	server.Start()
	server.Stop()
}

// TestIPC_ConcurrentRequests tests multiple concurrent IPC OPEN requests.
func TestIPC_ConcurrentRequests(t *testing.T) {
	sockPath := testSocketPath(t)
	var mu sync.Mutex
	var paths []string

	server, err := newIPCServerAt(sockPath, func(path string) error {
		mu.Lock()
		paths = append(paths, path)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("newIPCServerAt failed: %v", err)
	}
	server.Start()
	defer server.Stop()

	tmpDir := t.TempDir()
	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		tmpFile := filepath.Join(tmpDir, "test.ie32")
		if err := os.WriteFile(tmpFile, []byte{0xFF}, 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		go func(path string) {
			defer wg.Done()
			if err := sendIPCOpenAt(sockPath, path); err != nil {
				t.Errorf("sendIPCOpenAt failed: %v", err)
			}
		}(tmpFile)
	}
	wg.Wait()

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != n {
		t.Errorf("expected %d handled requests, got %d", n, len(paths))
	}
}

// TestComponentReset_SoundChip tests SoundChip.Reset() doesn't panic.
func TestComponentReset_SoundChip(t *testing.T) {
	chip, err := NewSoundChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Skipf("NewSoundChip failed (expected in headless): %v", err)
	}
	chip.Reset()

	if chip.enabled.Load() {
		t.Error("SoundChip should be disabled after reset")
	}
}

// TestComponentReset_PSGEngine tests PSGEngine.Reset() doesn't panic.
func TestComponentReset_PSGEngine(t *testing.T) {
	chip, err := NewSoundChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Skipf("NewSoundChip failed (expected in headless): %v", err)
	}
	engine := NewPSGEngine(chip, 44100)
	engine.Reset()

	if engine.enabled.Load() {
		t.Error("PSGEngine should be disabled after reset")
	}
}

// TestComponentReset_VideoChip tests VideoChip.Reset() doesn't panic.
func TestComponentReset_VideoChip(t *testing.T) {
	vc, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Skipf("NewVideoChip failed (expected in headless): %v", err)
	}
	vc.Reset()

	if vc.enabled.Load() {
		t.Error("VideoChip should be disabled after reset")
	}
	if vc.hasContent.Load() {
		t.Error("VideoChip hasContent should be false after reset")
	}
}

// TestComponentReset_VGAEngine tests VGAEngine.Reset() doesn't panic.
func TestComponentReset_VGAEngine(t *testing.T) {
	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	vga.Reset()

	if vga.enabled.Load() {
		t.Error("VGAEngine should be disabled after reset")
	}
}

// TestComponentReset_ULAEngine tests ULAEngine.Reset() doesn't panic.
func TestComponentReset_ULAEngine(t *testing.T) {
	bus := NewMachineBus()
	ula := NewULAEngine(bus)
	ula.Reset()

	if ula.enabled.Load() {
		t.Error("ULAEngine should be disabled after reset")
	}
}

// TestComponentReset_TEDVideoEngine tests TEDVideoEngine.Reset() doesn't panic.
func TestComponentReset_TEDVideoEngine(t *testing.T) {
	bus := NewMachineBus()
	ted := NewTEDVideoEngine(bus)
	ted.Reset()

	if ted.enabled.Load() {
		t.Error("TEDVideoEngine should be disabled after reset")
	}
}

// TestComponentReset_ANTICEngine tests ANTICEngine.Reset() doesn't panic.
func TestComponentReset_ANTICEngine(t *testing.T) {
	bus := NewMachineBus()
	antic := NewANTICEngine(bus)
	antic.Reset()

	if antic.enabled.Load() {
		t.Error("ANTICEngine should be disabled after reset")
	}
}

// TestComponentReset_VoodooEngine tests VoodooEngine.Reset() doesn't panic.
func TestComponentReset_VoodooEngine(t *testing.T) {
	bus := NewMachineBus()
	voodoo, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	voodoo.Reset()

	if voodoo.enabled.Load() {
		t.Error("VoodooEngine should be disabled after reset")
	}
}

// TestComponentReset_TerminalMMIO tests TerminalMMIO.Reset() doesn't panic.
func TestComponentReset_TerminalMMIO(t *testing.T) {
	term := NewTerminalMMIO()
	term.Reset()

	if term.SentinelTriggered.Load() {
		t.Error("TerminalMMIO sentinel should be false after reset")
	}
}

// TestComponentReset_CoprocessorManager tests CoprocessorManager.Reset() doesn't panic.
func TestComponentReset_CoprocessorManager(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	mgr.Reset()
}

// TestComponentReset_MachineBus tests MachineBus.Reset() clears memory.
func TestComponentReset_MachineBus(t *testing.T) {
	bus := NewMachineBus()
	bus.Write8(0x1000, 0xAA)
	bus.Write8(0x2000, 0xBB)

	bus.Reset()

	if bus.Read8(0x1000) != 0 {
		t.Error("memory at 0x1000 should be 0 after reset")
	}
	if bus.Read8(0x2000) != 0 {
		t.Error("memory at 0x2000 should be 0 after reset")
	}
}
