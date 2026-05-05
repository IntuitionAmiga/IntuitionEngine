//go:build headless

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCoprocStagingHarness(t *testing.T) (*MachineBus, *CoprocessorManager, string) {
	t.Helper()
	bus := NewMachineBus()
	baseDir := t.TempDir()
	mgr := NewCoprocessorManager(bus, baseDir)
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)
	return bus, mgr, baseDir
}

func readCStringForTest(bus *MachineBus, addr uint32) string {
	var out []byte
	for {
		b := bus.Read8(addr)
		if b == 0 {
			return string(out)
		}
		out = append(out, b)
		addr++
	}
}

func TestCoprocStaging_NameAndPtr(t *testing.T) {
	bus, mgr, baseDir := newCoprocStagingHarness(t)
	if err := os.WriteFile(filepath.Join(baseDir, "svc.iex"), []byte{1, 2, 3}, 0644); err != nil {
		t.Fatal(err)
	}

	if err := stageCoprocService(bus, mgr, "svc.iex"); err != nil {
		t.Fatal(err)
	}

	ptr := bus.Read32(COPROC_NAME_PTR)
	if ptr != coprocServiceNameStagingAddr {
		t.Fatalf("COPROC_NAME_PTR = 0x%08X, want 0x%08X", ptr, coprocServiceNameStagingAddr)
	}
	if got := readCStringForTest(bus, ptr); got != "svc.iex" {
		t.Fatalf("staged filename = %q, want %q", got, "svc.iex")
	}
}

func TestCoprocStaging_RejectsMissing(t *testing.T) {
	bus, mgr, _ := newCoprocStagingHarness(t)
	if err := stageCoprocService(bus, mgr, "missing.iex"); err == nil {
		t.Fatal("stageCoprocService accepted missing service")
	}
}

func TestCoprocStaging_RejectsAbsolutePath(t *testing.T) {
	bus, mgr, baseDir := newCoprocStagingHarness(t)
	path := filepath.Join(baseDir, "svc.iex")
	if err := os.WriteFile(path, []byte{1}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := stageCoprocService(bus, mgr, path); err == nil {
		t.Fatal("stageCoprocService accepted absolute service path")
	}
}

func TestCoprocStaging_AliasFlag(t *testing.T) {
	for _, flagName := range []string{"coproc-svc", "coproc"} {
		t.Run(flagName, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			var path string
			registerCoprocServiceFlags(fs, &path)
			if err := fs.Parse([]string{"-" + flagName, "svc.iex"}); err != nil {
				t.Fatal(err)
			}
			if path != "svc.iex" {
				t.Fatalf("%s parsed path = %q", flagName, path)
			}
		})
	}
}

func TestCoprocStaging_StartCmdLoadsWorker(t *testing.T) {
	bus, mgr, baseDir := newCoprocStagingHarness(t)
	data := assembleService(t, []string{
		"nasm", "-f", "bin", "-I", "sdk/include/", "-o", "OUTPUT",
	}, "sdk/examples/asm/coproc_service_x86.asm")
	if err := os.WriteFile(filepath.Join(baseDir, "svc.iex"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := stageCoprocService(bus, mgr, "svc.iex"); err != nil {
		t.Fatal(err)
	}

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_X86)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if got := bus.Read32(COPROC_CMD_STATUS); got != COPROC_STATUS_OK {
		t.Fatalf("START status=%d err=%d", got, bus.Read32(COPROC_CMD_ERROR))
	}
	if !mgr.IsWorkerRunning(EXEC_TYPE_X86) {
		t.Fatal("x86 coprocessor worker is not running after START")
	}
}

func TestCoprocStaging_RestageAfterBusReset(t *testing.T) {
	bus, mgr, baseDir := newCoprocStagingHarness(t)
	if err := os.WriteFile(filepath.Join(baseDir, "svc.iex"), []byte{1, 2, 3}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := stageCoprocService(bus, mgr, "svc.iex"); err != nil {
		t.Fatal(err)
	}

	bus.Reset()
	ptr := bus.Read32(COPROC_NAME_PTR)
	if ptr != coprocServiceNameStagingAddr {
		t.Fatalf("COPROC_NAME_PTR after reset = 0x%08X, want preserved MMIO value 0x%08X", ptr, coprocServiceNameStagingAddr)
	}
	if got := readCStringForTest(bus, ptr); got != "" {
		t.Fatalf("bus reset unexpectedly preserved staged filename %q", got)
	}

	if err := stageCoprocService(bus, mgr, "svc.iex"); err != nil {
		t.Fatal(err)
	}
	ptr = bus.Read32(COPROC_NAME_PTR)
	if ptr != coprocServiceNameStagingAddr {
		t.Fatalf("COPROC_NAME_PTR after restage = 0x%08X, want 0x%08X", ptr, coprocServiceNameStagingAddr)
	}
	if got := readCStringForTest(bus, ptr); got != "svc.iex" {
		t.Fatalf("staged filename after reset = %q, want %q", got, "svc.iex")
	}
}

func TestCoprocRunComments_ServiceFlagBeforeProgram(t *testing.T) {
	for _, rel := range []string{
		"sdk/examples/asm/coproc_caller_65.asm",
		"sdk/examples/asm/coproc_caller_68k.asm",
		"sdk/examples/asm/coproc_caller_ie32.asm",
		"sdk/examples/asm/coproc_caller_x86.asm",
		"sdk/examples/asm/coproc_caller_z80.asm",
	} {
		t.Run(filepath.Base(rel), func(t *testing.T) {
			src := readDemoSource(t, rel)
			for _, line := range strings.Split(src, "\n") {
				if !strings.Contains(line, "./bin/IntuitionEngine") || !strings.Contains(line, "-coproc-svc") {
					continue
				}
				flagIdx := strings.Index(line, "-coproc-svc")
				programIdx := strings.LastIndex(line, "coproc_caller_")
				if programIdx < 0 {
					t.Fatalf("run comment lacks caller binary: %s", line)
				}
				if flagIdx > programIdx {
					t.Fatalf("-coproc-svc appears after caller binary: %s", line)
				}
			}
		})
	}
}

func TestCoprocStaging_MainStagesAfterResettingLoaders(t *testing.T) {
	src := readDemoSource(t, "main.go")
	for _, tc := range []struct {
		name      string
		load      string
		startExec string
	}{
		{
			name:      "z80",
			load:      "z80CPU.LoadProgram(filename)",
			startExec: "z80CPU.StartExecution()",
		},
		{
			name:      "6502",
			load:      "cpu6502.LoadProgram(filename)",
			startExec: "cpu6502.StartExecution()",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadIdx := strings.Index(src, tc.load)
			startIdx := strings.Index(src, tc.startExec)
			if loadIdx < 0 {
				t.Fatalf("missing load call %q", tc.load)
			}
			if startIdx < 0 {
				t.Fatalf("missing start call %q", tc.startExec)
			}
			if startIdx < loadIdx {
				t.Fatal("start call appears before load call")
			}
			stageIdx := strings.Index(src[loadIdx:startIdx], "stageConfiguredCoprocService()")
			if stageIdx < 0 {
				t.Fatal("missing stageConfiguredCoprocService call")
			}
		})
	}
}

func TestCoprocStaging_FullResetRestagesAfterCoprocReset(t *testing.T) {
	src := readDemoSource(t, "main.go")
	resetIdx := strings.Index(src, "coprocMgr.Reset()")
	if resetIdx < 0 {
		t.Fatal("coprocMgr.Reset call not found")
	}
	startIdx := strings.Index(src[resetIdx:], "cpuRunner.StartExecution()")
	if startIdx < 0 {
		t.Fatal("full-reset CPU start call not found after coproc reset")
	}
	region := src[resetIdx : resetIdx+startIdx]
	stageIdx := strings.Index(region, "stageConfiguredCoprocService()")
	if stageIdx < 0 {
		t.Fatal("full-reset path does not restage configured coprocessor service before CPU restart")
	}
	loadIdx := strings.LastIndex(region[:stageIdx], "reloadProgram()")
	if loadIdx < 0 && !strings.Contains(region[:stageIdx], "loader.LoadROM(bytes)") {
		t.Fatal("restage must occur after the selected program or ROM loader")
	}
}
