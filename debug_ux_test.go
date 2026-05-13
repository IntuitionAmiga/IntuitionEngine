package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uxOutputText(lines []OutputLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHelpRegistry_AllCommandsHaveExamples(t *testing.T) {
	seen := make(map[string]bool)
	for _, entry := range monitorHelpRegistry() {
		if entry.Name == "" {
			t.Fatal("help entry has empty name")
		}
		if entry.Summary == "" {
			t.Fatalf("%s help entry has empty summary", entry.Name)
		}
		if len(entry.Syntax) == 0 {
			t.Fatalf("%s help entry has no syntax", entry.Name)
		}
		if len(entry.Examples) == 0 {
			t.Fatalf("%s help entry has no examples", entry.Name)
		}
		if seen[entry.Name] {
			t.Fatalf("duplicate help entry %s", entry.Name)
		}
		seen[entry.Name] = true
	}
	for _, name := range []string{"pg", "accesslog", "who", "bfirst", "fault", "layout", "alias", "rc", "bug", "bpm", "rg", "rt", "tl", "history"} {
		if _, ok := monitorHelpByName(name); !ok {
			t.Fatalf("missing help for %s", name)
		}
	}
}

func TestHelpCommand_PrintsCommandExamples(t *testing.T) {
	mon := NewMachineMonitor(nil)
	_, out := mon.ExecuteCommandResult("help pg")
	text := uxOutputText(out)
	if !strings.Contains(text, "pg - Add, list, or clear page-access guards") {
		t.Fatalf("help pg output = %q", text)
	}
	if !strings.Contains(text, "Examples:") || !strings.Contains(text, "pg add $4000 $4FFF rw cpu=current") {
		t.Fatalf("help pg examples missing: %q", text)
	}
}

func TestHistory_PersistsAcrossSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IEMON_HOME", dir)

	mon := NewMachineMonitor(nil)
	mon.ExecuteCommand("help r")
	mon.ExecuteCommand("help d")

	data, err := os.ReadFile(filepath.Join(dir, "history"))
	if err != nil {
		t.Fatalf("history not written: %v", err)
	}
	if !strings.Contains(string(data), "help r") || !strings.Contains(string(data), "help d") {
		t.Fatalf("history file = %q", string(data))
	}

	next := NewMachineMonitor(nil)
	if len(next.history) < 2 || next.history[0] != "help r" || next.history[1] != "help d" {
		t.Fatalf("loaded history = %#v", next.history)
	}
}

func TestHistory_ReverseSearchUsesCurrentInputAsQuery(t *testing.T) {
	mon := NewMachineMonitor(nil)
	mon.history = []string{"help r", "pg list", "help d", "pg add $1000 $10ff r"}
	mon.historyIdx = len(mon.history)
	mon.inputLine = []byte("pg")
	mon.cursorPos = len(mon.inputLine)

	if !mon.reverseHistorySearch() || string(mon.inputLine) != "pg add $1000 $10ff r" {
		t.Fatalf("first reverse search input = %q", mon.inputLine)
	}
	if !mon.reverseHistorySearch() || string(mon.inputLine) != "pg list" {
		t.Fatalf("second reverse search input = %q", mon.inputLine)
	}
}

func TestAlias_ExpandsToCommand(t *testing.T) {
	mon := NewMachineMonitor(nil)
	mon.ExecuteCommand("alias hp help pg")
	_, out := mon.ExecuteCommandResult("hp")
	text := uxOutputText(out)
	if !strings.Contains(text, "pg - Add, list, or clear page-access guards") {
		t.Fatalf("alias output = %q", text)
	}
}

func TestLayoutAndBugCommands_AreAvailable(t *testing.T) {
	mon := NewMachineMonitor(nil)
	_, layoutOut := mon.ExecuteCommandResult("layout list")
	if !strings.Contains(uxOutputText(layoutOut), "Layouts: cpu, trace, debug") {
		t.Fatalf("layout list output = %#v", layoutOut)
	}
	_, bugOut := mon.ExecuteCommandResult("bug")
	if !strings.Contains(uxOutputText(bugOut), "IEMon bug report") {
		t.Fatalf("bug output = %#v", bugOut)
	}
}

func TestFmt_RegisterLineShape_AllWidths(t *testing.T) {
	tests := []struct {
		name      string
		reg       RegisterInfo
		addrWidth int
		want      string
	}{
		{"16-bit", RegisterInfo{Name: "A", Value: 0x12, BitWidth: 8}, 16, "A    $0012"},
		{"32-bit", RegisterInfo{Name: "D0", Value: 0x1234, BitWidth: 32}, 32, "D0   $00001234"},
		{"64-bit", RegisterInfo{Name: "R1", Value: 0x1234, BitWidth: 64}, 64, "R1   $0000000000001234"},
	}
	for _, tt := range tests {
		if got := formatMonitorRegisterLine(tt.reg, tt.addrWidth); got != tt.want {
			t.Fatalf("%s register line = %q, want %q", tt.name, got, tt.want)
		}
	}
}
