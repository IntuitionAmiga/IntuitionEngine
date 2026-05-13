package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDwarf_AddressToLine_ManualSeed(t *testing.T) {
	lines := NewSourceLineTable()
	lines.Add("IE64", 0x1000, "main.ie64.s", 12)

	src, ok := lines.Resolve("IE64", 0x1004)
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if src.File != "main.ie64.s" || src.Line != 12 {
		t.Fatalf("source = %+v, want main.ie64.s:12", src)
	}
}

func TestDwarf_LoadELF_ReadsRealLineTable(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcPath, []byte("package main\n\nfunc main() { helper() }\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tiny")
	cmd := exec.Command("go", "build", "-gcflags=all=-N -l", "-o", path, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build DWARF fixture: %v\n%s", err, out)
	}
	lines := NewSourceLineTable()
	if err := lines.LoadELF("X86", path); err != nil {
		t.Fatalf("LoadELF(%q): %v", path, err)
	}
	got := lines.byCPU[normalizeSymbolCPU("X86")]
	if len(got) == 0 {
		t.Fatalf("LoadELF(%q) produced no source lines", path)
	}
	resolved, ok := lines.Resolve("X86", got[len(got)/2].Addr)
	if !ok || resolved.File == "" || resolved.Line <= 0 {
		t.Fatalf("Resolve after LoadELF = %+v, %v", resolved, ok)
	}
}

func TestDwarf_LoadELF_GracefullyIgnoresNonDWARFELF(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "nodwarf-*.elf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x7f, 'E', 'L', 'F'}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	lines := NewSourceLineTable()
	if err := lines.LoadELF("IE64", f.Name()); err == nil {
		t.Fatalf("LoadELF accepted malformed ELF %q", f.Name())
	}
}

func TestDisasmSlashS_InterleavesSource(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.PC = PROG_START
	cpu.memory[PROG_START] = OP_NOP64
	mon.sources.Add("IE64", PROG_START, "demo.ie64.s", 7)

	_, out := mon.ExecuteCommandResult("d /s pc 1")
	var sawSource bool
	for _, line := range out {
		if strings.Contains(line.Text, "demo.ie64.s:7") {
			sawSource = true
			break
		}
	}
	if !sawSource {
		t.Fatalf("d /s output = %#v, want source line", out)
	}
}

func TestDisasmSlashS_DegradesGracefully(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.PC = PROG_START
	cpu.memory[PROG_START] = OP_NOP64

	_, out := mon.ExecuteCommandResult("d /s pc 1")
	var sawNoSource bool
	for _, line := range out {
		if strings.Contains(line.Text, "no source info") {
			sawNoSource = true
			break
		}
	}
	if !sawNoSource {
		t.Fatalf("d /s output = %#v, want graceful no-source message", out)
	}
}

func TestListCommand_SourceOrGraceful(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.PC = PROG_START
	_, out := mon.ExecuteCommandResult("list")
	if len(out) == 0 || !strings.Contains(out[len(out)-1].Text, "no source info") {
		t.Fatalf("list output = %#v, want no-source message", out)
	}

	mon.sources.Add("IE64", PROG_START, "demo.go", 3)
	_, out = mon.ExecuteCommandResult("list")
	if len(out) == 0 || !strings.Contains(out[len(out)-1].Text, "demo.go:3") {
		t.Fatalf("list output = %#v, want source location", out)
	}
}

func TestListCommand_UsesIEMONSrcPathForContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte("package main\nfunc main() {\n\tprintln(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IEMON_SRC_PATH", dir)

	mon, cpu := newTestMonitor()
	cpu.PC = PROG_START
	mon.sources.Add("IE64", PROG_START, "demo.go", 3)

	_, out := mon.ExecuteCommandResult("list")
	text := uxOutputText(out)
	if !strings.Contains(text, "demo.go:3") || !strings.Contains(text, "> 3") || !strings.Contains(text, "println") {
		t.Fatalf("list output = %#v, want source context", out)
	}
}
