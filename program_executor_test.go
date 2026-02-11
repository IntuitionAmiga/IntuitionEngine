package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDetectExecType(t *testing.T) {
	tests := []struct {
		name string
		path string
		want uint32
	}{
		{name: "iex", path: "prog.iex", want: EXEC_TYPE_IE32},
		{name: "ie32", path: "prog.ie32", want: EXEC_TYPE_IE32},
		{name: "ie64", path: "prog.ie64", want: EXEC_TYPE_IE64},
		{name: "ie65", path: "prog.ie65", want: EXEC_TYPE_6502},
		{name: "ie68", path: "prog.ie68", want: EXEC_TYPE_M68K},
		{name: "ie80", path: "prog.ie80", want: EXEC_TYPE_Z80},
		{name: "ie86", path: "prog.ie86", want: EXEC_TYPE_X86},
		{name: "unknown", path: "prog.bin", want: EXEC_TYPE_NONE},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectExecType(tc.path); got != tc.want {
				t.Fatalf("detectExecType(%q)=%d, want %d", tc.path, got, tc.want)
			}
		})
	}
}

func TestProgramExecutor_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "missing.ie64")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	// File-not-found is synchronous (os.Stat in startExecute)
	if got := exec.HandleRead(EXEC_STATUS); got != EXEC_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, EXEC_STATUS_ERROR)
	}
	if got := exec.HandleRead(EXEC_ERROR); got != EXEC_ERR_NOT_FOUND {
		t.Fatalf("error=%d, want %d (NOT_FOUND)", got, EXEC_ERR_NOT_FOUND)
	}
}

func TestProgramExecutor_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)

	// Create a file with unsupported extension
	os.WriteFile(filepath.Join(dir, "file.bin"), []byte("data"), 0644)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "file.bin")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	if got := exec.HandleRead(EXEC_STATUS); got != EXEC_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, EXEC_STATUS_ERROR)
	}
	if got := exec.HandleRead(EXEC_ERROR); got != EXEC_ERR_UNSUPPORTED {
		t.Fatalf("error=%d, want %d (UNSUPPORTED)", got, EXEC_ERR_UNSUPPORTED)
	}
}

func TestProgramExecutor_PathInvalid(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "../escape.ie64")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	if got := exec.HandleRead(EXEC_STATUS); got != EXEC_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, EXEC_STATUS_ERROR)
	}
	if got := exec.HandleRead(EXEC_ERROR); got != EXEC_ERR_PATH_INVALID {
		t.Fatalf("error=%d, want %d (PATH_INVALID)", got, EXEC_ERR_PATH_INVALID)
	}
}

func TestProgramExecutor_SessionIncrement(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	s0 := exec.HandleRead(EXEC_SESSION)
	if s0 != 0 {
		t.Fatalf("initial session=%d, want 0", s0)
	}

	// Create a valid file so session increments
	// Write a minimal IE64 program: a single RTS (0x00000035 = rts opcode for IE64)
	os.WriteFile(filepath.Join(dir, "test.ie64"), []byte{0x35, 0x00, 0x00, 0x00}, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "test.ie64")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	// Session should have incremented
	s1 := exec.HandleRead(EXEC_SESSION)
	if s1 != 1 {
		t.Fatalf("session after first exec=%d, want 1", s1)
	}

	// Wait for async completion
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := exec.HandleRead(EXEC_STATUS)
		if s != EXEC_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}
}

func TestProgramExecutor_FailureKeepsIE64Running(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)
	ie64CPU.running.Store(true)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "missing.ie64")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	// Error is synchronous (file not found via os.Stat)
	if got := exec.HandleRead(EXEC_STATUS); got != EXEC_STATUS_ERROR {
		t.Fatalf("status=%d, want %d (ERROR)", got, EXEC_STATUS_ERROR)
	}

	// IE64 CPU must still be running
	if !ie64CPU.running.Load() {
		t.Fatalf("IE64 CPU running should remain true on failure")
	}
}

func TestProgramExecutor_SuccessStopsIE64(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)
	ie64CPU.running.Store(true)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	// Write a minimal program
	os.WriteFile(filepath.Join(dir, "test.ie64"), []byte{0x35, 0x00, 0x00, 0x00}, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "test.ie64")

	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	// Wait for async to reach running state
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := exec.HandleRead(EXEC_STATUS)
		if s == EXEC_STATUS_RUNNING {
			break
		}
		if s == EXEC_STATUS_ERROR {
			t.Fatalf("unexpected error state")
		}
		runtime.Gosched()
	}

	if got := exec.HandleRead(EXEC_STATUS); got != EXEC_STATUS_RUNNING {
		t.Fatalf("status=%d, want %d (RUNNING)", got, EXEC_STATUS_RUNNING)
	}

	// IE64 CPU should be stopped after successful handoff
	if ie64CPU.running.Load() {
		t.Fatalf("IE64 CPU running should be false after successful exec handoff")
	}
}

func TestProgramExecutor_SessionPreventsStaleOverwrite(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	ie64CPU := NewCPU64(bus)

	exec := NewProgramExecutor(bus, ie64CPU, nil, nil, nil, dir)

	// Create two valid files so both requests pass the synchronous os.Stat check
	os.WriteFile(filepath.Join(dir, "first.ie64"), []byte{0x35, 0x00, 0x00, 0x00}, 0644)
	os.WriteFile(filepath.Join(dir, "second.ie64"), []byte{0x35, 0x00, 0x00, 0x00}, 0644)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, "first.ie64")
	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	s1 := exec.HandleRead(EXEC_SESSION)

	// Second request immediately supersedes first
	writeFilenameToBus(bus, nameAddr, "second.ie64")
	exec.HandleWrite(EXEC_NAME_PTR, nameAddr)
	exec.HandleWrite(EXEC_CTRL, EXEC_OP_EXECUTE)

	s2 := exec.HandleRead(EXEC_SESSION)
	if s2 <= s1 {
		t.Fatalf("second session %d should be > first session %d", s2, s1)
	}

	// Wait for both async goroutines to finish
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := exec.HandleRead(EXEC_STATUS)
		if s == EXEC_STATUS_RUNNING {
			break
		}
		runtime.Gosched()
	}

	// Session should still reflect the second request
	if got := exec.HandleRead(EXEC_SESSION); got != s2 {
		t.Fatalf("session=%d, want %d (second request)", got, s2)
	}
}
