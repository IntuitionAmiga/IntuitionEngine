package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMonitorAssembleModeWritesAndAdvances(t *testing.T) {
	mon, cpu := newTestMonitor()

	_, out := mon.ExecuteCommandResult("A $1000")
	if text := uxOutputText(out); !strings.Contains(text, "IE64 assemble at $0000000000001000") {
		t.Fatalf("entry output = %q", text)
	}
	if got := mon.currentPrompt(); got != "asm $0000000000001000> " {
		t.Fatalf("prompt after entry = %q", got)
	}

	_, out = mon.ExecuteCommandResult("move.l r2,#42")
	want := []byte{0x01, 0x15, 0x00, 0x00, 0x2A, 0x00, 0x00, 0x00}
	if got := cpu.memory[0x1000 : 0x1000+8]; !bytes.Equal(got, want) {
		t.Fatalf("assembled bytes = % X, want % X", got, want)
	}
	text := uxOutputText(out)
	if !strings.Contains(text, "$0000000000001000: 01 15 00 00 2A 00 00 00") ||
		!strings.Contains(text, "move.l r2") {
		t.Fatalf("success output = %q", text)
	}
	if got := mon.currentPrompt(); got != "asm $0000000000001008> " {
		t.Fatalf("prompt after first write = %q", got)
	}

	_, _ = mon.ExecuteCommandResult("nop")
	if got := cpu.memory[0x1008 : 0x1008+8]; !bytes.Equal(got, []byte{0xE0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("second assembled bytes = % X, want nop", got)
	}
	if got := mon.currentPrompt(); got != "asm $0000000000001010> " {
		t.Fatalf("prompt after second write = %q", got)
	}

	_, out = mon.ExecuteCommandResult("")
	if text := uxOutputText(out); !strings.Contains(text, "Exited IE64 assemble mode") {
		t.Fatalf("exit output = %q", text)
	}
	if got := mon.currentPrompt(); got != "> " {
		t.Fatalf("prompt after exit = %q", got)
	}
}

func TestMonitorAssembleModeFlushesAllIE64JITCaches(t *testing.T) {
	bus := NewMachineBus()
	cpuA := NewCPU64(bus)
	cpuA.running.Store(false)
	cpuA.jitCache = NewCodeCache()
	cpuA.jitCache.Put(&JITBlock{startPC: 0x1000, endPC: 0x1008})
	cpuB := NewCPU64(bus)
	cpuB.running.Store(false)
	cpuB.jitCache = NewCodeCache()
	cpuB.jitCache.Put(&JITBlock{startPC: 0x1000, endPC: 0x1008})
	cpuB.jitCtx = newJITContext(cpuB)
	cpuB.jitCtx.RTSCache0PC = 0x1000
	cpuB.jitCtx.RTSCache0Addr = 0x1234

	mon := NewMachineMonitor(bus)
	idA := mon.RegisterCPU("IE64-A", NewDebugIE64(cpuA))
	mon.RegisterCPU("IE64-B", NewDebugIE64(cpuB))
	mon.ExecuteCommand("cpu " + strconv.Itoa(idA))

	mon.ExecuteCommand("A $1000")
	_, out := mon.ExecuteCommandResult("nop")
	if text := uxOutputText(out); !strings.Contains(text, "$0000000000001000: E0 00 00 00 00 00 00 00") {
		t.Fatalf("assemble output = %q", text)
	}
	if cpuA.jitCache.Get(0x1000) != nil {
		t.Fatal("focused IE64 JIT cache still contains patched block")
	}
	if cpuB.jitCache.Get(0x1000) != nil {
		t.Fatal("second IE64 JIT cache still contains patched block")
	}
	if cpuB.jitCtx.RTSCache0PC != 0 || cpuB.jitCtx.RTSCache0Addr != 0 {
		t.Fatalf("second IE64 RTS cache not cleared: pc=%#x addr=%#x", cpuB.jitCtx.RTSCache0PC, cpuB.jitCtx.RTSCache0Addr)
	}
}

func TestMonitorAssembleModeFailuresDoNotAdvance(t *testing.T) {
	mon, cpu := newTestMonitor()

	mon.ExecuteCommand("A $1000")
	_, out := mon.ExecuteCommandResult("badmnemonic r1")
	if text := uxOutputText(out); !strings.Contains(text, "unknown instruction") {
		t.Fatalf("bad mnemonic output = %q", text)
	}
	if got := mon.currentPrompt(); got != "asm $0000000000001000> " {
		t.Fatalf("prompt after failed assembly = %q", got)
	}
	if got := cpu.memory[0x1000 : 0x1000+8]; !bytes.Equal(got, make([]byte, 8)) {
		t.Fatalf("memory changed after failed assembly: % X", got)
	}

	_, out = mon.ExecuteCommandResult("li r1,#$12345678")
	if text := uxOutputText(out); !strings.Contains(text, "pseudo-instruction expands to more than one instruction") {
		t.Fatalf("li output = %q", text)
	}
	if got := mon.currentPrompt(); got != "asm $0000000000001000> " {
		t.Fatalf("prompt after pseudo failure = %q", got)
	}

	_, _ = mon.ExecuteCommandResult("nop")
	if got := cpu.memory[0x1000 : 0x1000+8]; !bytes.Equal(got, []byte{0xE0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("post-failure write bytes = % X, want nop at original address", got)
	}
}

func TestMonitorAssembleModeRejectsUnsupportedInput(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.ExecuteCommand("A $1000")
	for _, tt := range []struct {
		line string
		want string
	}{
		{"label:", "labels are not supported in monitor assemble mode"},
		{"org $2000", "directives are not supported in monitor assemble mode"},
		{"include \"x.i\"", "include is not supported in monitor assemble mode"},
		{"incbin \"x.bin\"", "incbin is not supported in monitor assemble mode"},
		{"; comment", "no instruction assembled"},
	} {
		_, out := mon.ExecuteCommandResult(tt.line)
		if text := uxOutputText(out); !strings.Contains(text, tt.want) {
			t.Fatalf("%q output = %q, want %q", tt.line, text, tt.want)
		}
		if got := mon.currentPrompt(); got != "asm $0000000000001000> " {
			t.Fatalf("%q prompt = %q, want unchanged asm address", tt.line, got)
		}
	}
}

func TestMonitorAssembleCommandAddressParsingAndCPUFocus(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(NewCPU(bus)))

	_, out := mon.ExecuteCommandResult("A $1000")
	if text := uxOutputText(out); !strings.Contains(text, "monitor assembly is IE64-only") {
		t.Fatalf("non-IE64 output = %q", text)
	}

	mon, _ = newTestMonitor()
	_, out = mon.ExecuteCommandResult("A $10000000000000000")
	if text := uxOutputText(out); !strings.Contains(text, "$10000000000000000") || !strings.Contains(text, "overflow") {
		t.Fatalf("overflow output = %q", text)
	}

	_, out = mon.ExecuteCommandResult("A $FFFFFFFFFFFFFFFF")
	if text := uxOutputText(out); !strings.Contains(text, "IE64 assemble at $FFFFFFFFFFFFFFFF") {
		t.Fatalf("max uint64 output = %q", text)
	}
}

func TestMonitorAssembleModeClearsOnCPUSwitch(t *testing.T) {
	bus := NewMachineBus()
	cpu64 := NewCPU64(bus)
	cpu64.running.Store(false)
	cpu32 := NewCPU(bus)
	mon := NewMachineMonitor(bus)
	id64 := mon.RegisterCPU("IE64", NewDebugIE64(cpu64))
	id32 := mon.RegisterCPU("IE32", NewDebugIE32(cpu32))
	_ = id64

	mon.ExecuteCommand("A $1000")
	_, out := mon.ExecuteCommandResult("cpu " + strconv.Itoa(id32))
	if text := uxOutputText(out); !strings.Contains(text, "Focussed on id:") {
		t.Fatalf("cpu switch output = %q", text)
	}
	if got := mon.currentPrompt(); got != "> " {
		t.Fatalf("prompt after cpu switch = %q", got)
	}
	_, out = mon.ExecuteCommandResult("nop")
	if text := uxOutputText(out); !strings.Contains(text, "Unknown command: nop") {
		t.Fatalf("post-switch command output = %q", text)
	}
}

func TestMonitorAssembleModeRejectedFromScriptsAndMacros(t *testing.T) {
	mon, _ := newTestMonitor()
	scriptPath := filepath.Join(t.TempDir(), "patch.imon")
	if err := os.WriteFile(scriptPath, []byte("A $1000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, out := mon.ExecuteCommandResult("script " + scriptPath)
	if text := uxOutputText(out); !strings.Contains(text, "monitor assemble mode is interactive only") {
		t.Fatalf("script output = %q", text)
	}

	mon.ExecuteCommand("macro patch A $1000")
	_, out = mon.ExecuteCommandResult("patch")
	if text := uxOutputText(out); !strings.Contains(text, "monitor assemble mode is interactive only") {
		t.Fatalf("macro output = %q", text)
	}
}

func TestMonitorAssembleModeRejectedFromRCFiles(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "direct assemble",
			body: "A $1000\n",
			want: "rc command not allowed: a",
		},
		{
			name: "alias to assemble",
			body: "alias patch A $1000\n",
			want: "rc command not allowed: alias",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			t.Setenv("IEMON_HOME", home)
			t.Chdir(project)
			rcPath := filepath.Join(project, ".iemonrc")
			if err := os.WriteFile(rcPath, []byte(tt.body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := trustIEMONRC(rcPath); err != nil {
				t.Fatal(err)
			}

			mon, _ := newRCTestMonitor()
			_, out := mon.ExecuteCommandResult("rc load .iemonrc")
			if text := uxOutputText(out); !strings.Contains(text, tt.want) {
				t.Fatalf("rc load output = %q, want %q", text, tt.want)
			}
		})
	}
}

func TestIEScriptSandboxRejectsMonitorAssembleMode(t *testing.T) {
	mon, _ := newTestMonitor()

	if err := validateSandboxedMonitorCommand("A $1000", mon); err == nil ||
		!strings.Contains(err.Error(), "IEScript cannot use monitor assemble mode") {
		t.Fatalf("dbg.command A error = %v", err)
	}
	if err := validateSandboxedMonitorMacroBody("A $1000"); err == nil ||
		!strings.Contains(err.Error(), "IEScript cannot use monitor assemble mode") {
		t.Fatalf("dbg.macro body error = %v", err)
	}

	mon.ExecuteCommand("alias patch A $1000")
	if err := validateSandboxedMonitorCommand("patch", mon); err == nil ||
		!strings.Contains(err.Error(), "IEScript cannot use monitor assemble mode") {
		t.Fatalf("dbg.command alias error = %v", err)
	}

	mon.ExecuteCommand("A $1000")
	if err := validateSandboxedMonitorCommand("nop", mon); err == nil ||
		!strings.Contains(err.Error(), "IEScript cannot use monitor assemble mode") {
		t.Fatalf("dbg.command feed error = %v", err)
	}
	if err := validateSandboxedMonitorCommand("", mon); err == nil ||
		!strings.Contains(err.Error(), "IEScript cannot use monitor assemble mode") {
		t.Fatalf("dbg.command empty-feed error = %v", err)
	}
}
