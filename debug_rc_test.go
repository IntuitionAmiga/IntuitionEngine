package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRCTestMonitor() (*MachineMonitor, *DebugIE64) {
	bus := NewMachineBus()
	adapter := NewDebugIE64(NewCPU64(bus))
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE64", adapter)
	return mon, adapter
}

func TestRC_RequiresTrustBeforeLoad(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("IEMON_HOME", home)
	t.Chdir(project)
	if err := os.WriteFile(filepath.Join(project, ".iemonrc"), []byte("alias bootbp b $2000\nb $1000\npg list\nhistory config 4 8 2 16\n"), 0600); err != nil {
		t.Fatal(err)
	}

	mon, adapter := newRCTestMonitor()
	_, out := mon.ExecuteCommandResult("rc load .iemonrc")
	if !strings.Contains(uxOutputText(out), "not trusted") {
		t.Fatalf("rc load output = %#v, want trust refusal", out)
	}

	mon.ExecuteCommand("rc trust .iemonrc")
	_, out = mon.ExecuteCommandResult("rc load .iemonrc")
	if !strings.Contains(uxOutputText(out), "Loaded 4 command(s)") {
		t.Fatalf("rc load output = %#v, want loaded count", out)
	}
	if mon.aliases["bootbp"] != "b $2000" {
		t.Fatalf("alias bootbp = %q, want b $2000", mon.aliases["bootbp"])
	}
	bps := adapter.ListBreakpoints()
	if len(bps) != 1 || bps[0] != 0x1000 {
		t.Fatalf("breakpoints = %#v, want $1000", bps)
	}
	if mon.wholeCheckpointInterval != 4 || mon.wholeCheckpointBytes != 8<<20 || mon.maxWholeCheckpoints != 2 || mon.maxWholeHistory != 16 {
		t.Fatalf("history config = interval %d bytes %d checkpoints %d snapshots %d",
			mon.wholeCheckpointInterval, mon.wholeCheckpointBytes, mon.maxWholeCheckpoints, mon.maxWholeHistory)
	}
}

func TestRC_RejectsUnsafeCommands(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("IEMON_HOME", home)
	t.Chdir(project)
	if err := os.WriteFile(filepath.Join(project, ".iemonrc"), []byte("load patch.bin $1000\n"), 0600); err != nil {
		t.Fatal(err)
	}

	mon, _ := newRCTestMonitor()
	mon.ExecuteCommand("rc trust .iemonrc")
	_, out := mon.ExecuteCommandResult("rc load .iemonrc")
	if !strings.Contains(uxOutputText(out), "rc command not allowed: load") {
		t.Fatalf("rc load output = %#v, want unsafe-command refusal", out)
	}
}

func TestRC_HashMismatchInvalidatesTrust(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("IEMON_HOME", home)
	t.Chdir(project)
	rcPath := filepath.Join(project, ".iemonrc")
	if err := os.WriteFile(rcPath, []byte("alias ni s\n"), 0600); err != nil {
		t.Fatal(err)
	}

	mon, _ := newRCTestMonitor()
	mon.ExecuteCommand("rc trust .iemonrc")
	if err := os.WriteFile(rcPath, []byte("alias ni s\nb $2000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, out := mon.ExecuteCommandResult("rc load .iemonrc")
	if !strings.Contains(uxOutputText(out), "not trusted") {
		t.Fatalf("rc load output = %#v, want hash-mismatch refusal", out)
	}
}

func TestRC_TrustedFileAutoLoadsAfterCPURegistration(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("IEMON_HOME", home)
	t.Chdir(project)
	if err := os.WriteFile(filepath.Join(project, ".iemonrc"), []byte("b $1234\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := trustIEMONRC(filepath.Join(project, ".iemonrc")); err != nil {
		t.Fatal(err)
	}

	_, adapter := newRCTestMonitor()
	bps := adapter.ListBreakpoints()
	if len(bps) != 1 || bps[0] != 0x1234 {
		t.Fatalf("auto-loaded breakpoints = %#v, want $1234", bps)
	}
}
