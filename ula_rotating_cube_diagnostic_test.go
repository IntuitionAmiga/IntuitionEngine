//go:build headless

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestULARotatingCubeDiagnosticLoadsPrebuiltImageAndTracksGuestFrame(t *testing.T) {
	diagnostic, err := os.ReadFile("sdk/scripts/diag_ula_rotating_cube_65.ies")
	if err != nil {
		t.Fatalf("read ULA cube diagnostic: %v", err)
	}
	text := string(diagnostic)

	for _, want := range []string{
		`cpu.load("sdk/examples/prebuilt/ula_rotating_cube_65.ie65")`,
		"local CURRENT_FRAME = 0x000D",
		"local current_frame = mem.read8(CURRENT_FRAME)",
		"observed_frames = observed_frames + 1",
		"local visible_phase = (current_frame + 31) % 32",
		`"[ula cube] frame=%d visible=%d hash=%s", current_frame, visible_phase, tostring(hash)`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("ULA cube diagnostic missing %q", want)
		}
	}
	for _, stale := range []string{
		"FRAME_COMMITTED",
		"COMMITTED_FRAME",
		"committed, visible_phase",
		"Copper neither running, waiting, nor halted",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("ULA cube diagnostic still contains stale state %q", stale)
		}
	}

	assembly, err := os.ReadFile("sdk/examples/asm/ula_rotating_cube_65.asm")
	if err != nil {
		t.Fatalf("read ULA cube source: %v", err)
	}
	if !strings.Contains(string(assembly), "curr_frame:     .res 1") ||
		!strings.Contains(string(assembly), "inc curr_frame") {
		t.Fatal("ULA cube diagnostic must track the guest's incremented curr_frame state")
	}
}

func TestULARotatingCubeDiagnosticPrebuiltPathResolvesFromScriptDirectory(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	var loaded string
	se.SetProgramLoader(func(path string) error {
		loaded = path
		return nil
	})
	scriptPath, err := filepath.Abs(filepath.Join("sdk", "scripts", "diag_ula_rotating_cube_65.ies"))
	if err != nil {
		t.Fatal(err)
	}
	if err := se.RunString(
		`cpu.load("sdk/examples/prebuilt/ula_rotating_cube_65.ie65")`,
		scriptPath,
	); err != nil {
		t.Fatalf("start diagnostic load: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("diagnostic image path is not loadable: %v", err)
	}
	want, err := filepath.Abs(filepath.Join("sdk", "examples", "prebuilt", "ula_rotating_cube_65.ie65"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded != want {
		t.Fatalf("loaded path = %q, want %q", loaded, want)
	}
}
