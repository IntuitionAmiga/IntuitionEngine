//go:build headless

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeZ80LoopService(t *testing.T, dir, name string) {
	t.Helper()
	// JP $0000 keeps the worker alive until the manager stops it.
	if err := os.WriteFile(filepath.Join(dir, name), []byte{0xC3, 0x00, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
}

func newIEMONCoprocHarness(t *testing.T) (*MachineBus, *CoprocessorManager, *MachineMonitor, string) {
	t.Helper()
	bus, mgr, baseDir := newCoprocStagingHarness(t)
	mon := NewMachineMonitor(bus)
	mon.coprocMgr = mgr
	mgr.monitor = mon
	mon.RegisterCPU("IE64", NewDebugIE64(NewCPU64(bus)))
	t.Cleanup(mgr.StopAll)
	return bus, mgr, mon, baseDir
}

func monitorText(lines []OutputLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func requireMonitorOutputContains(t *testing.T, lines []OutputLine, want string) {
	t.Helper()
	text := monitorText(lines)
	if !strings.Contains(text, want) {
		t.Fatalf("monitor output missing %q:\n%s", want, text)
	}
}

func findMonitorCPUByLabel(mon *MachineMonitor, label string) (int, bool) {
	mon.mu.Lock()
	defer mon.mu.Unlock()
	for id, entry := range mon.cpus {
		if strings.EqualFold(entry.Label, label) {
			return id, true
		}
	}
	return 0, false
}

func TestIEMONCPUListsOfflineCoprocSlots(t *testing.T) {
	_, _, mon, _ := newIEMONCoprocHarness(t)

	_, out := mon.ExecuteCommandResult("cpu")
	requireMonitorOutputContains(t, out, "coproc:Z80")
	requireMonitorOutputContains(t, out, "[OFFLINE]")
}

func TestCoprocWorkerLifecycleAPIStartStopInventory(t *testing.T) {
	_, mgr, baseDir := newCoprocStagingHarness(t)
	t.Cleanup(mgr.StopAll)
	writeZ80LoopService(t, baseDir, "svc.ie80")

	cpuType, err := mgr.StartWorkerFromImage(EXEC_TYPE_NONE, "svc.ie80", false)
	if err != nil {
		t.Fatal(err)
	}
	if cpuType != EXEC_TYPE_Z80 {
		t.Fatalf("inferred cpuType=%d, want Z80", cpuType)
	}
	if _, err := mgr.StartWorkerFromImage(EXEC_TYPE_Z80, "svc.ie80", false); err == nil {
		t.Fatal("duplicate start without replace succeeded")
	}
	if _, err := mgr.StartWorkerFromImage(EXEC_TYPE_Z80, "svc.ie80", true); err != nil {
		t.Fatalf("replace start failed: %v", err)
	}

	var found bool
	for _, slot := range mgr.WorkerInventory() {
		if slot.CPUType == EXEC_TYPE_Z80 {
			found = true
			if !slot.Online {
				t.Fatal("Z80 slot is not online in inventory")
			}
			if slot.Path != "svc.ie80" {
				t.Fatalf("Z80 slot path=%q, want svc.ie80", slot.Path)
			}
		}
	}
	if !found {
		t.Fatal("Z80 slot missing from inventory")
	}

	if err := mgr.StopWorker(EXEC_TYPE_Z80); err != nil {
		t.Fatal(err)
	}
	for _, slot := range mgr.WorkerInventory() {
		if slot.CPUType == EXEC_TYPE_Z80 && slot.Online {
			t.Fatal("Z80 slot remains online after StopWorker")
		}
	}
}

func TestIEMONCPUOnlineStagedZ80RegistersAndFocuses(t *testing.T) {
	bus, mgr, mon, baseDir := newIEMONCoprocHarness(t)
	writeZ80LoopService(t, baseDir, "svc.ie80")
	if err := stageCoprocService(bus, mgr, "svc.ie80"); err != nil {
		t.Fatal(err)
	}

	_, out := mon.ExecuteCommandResult("cpu online z80")
	requireMonitorOutputContains(t, out, "Online z80 as coproc:Z80")
	if !mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker is not running")
	}
	id, ok := findMonitorCPUByLabel(mon, "coproc:Z80")
	if !ok {
		t.Fatal("Z80 worker was not registered with monitor")
	}

	_, out = mon.ExecuteCommandResult("cpu " + strconv.Itoa(id))
	requireMonitorOutputContains(t, out, "Focussed on id:")
}

func TestIEMONCPUOnlineStagedZ80MissingServiceFails(t *testing.T) {
	_, _, mon, _ := newIEMONCoprocHarness(t)

	_, out := mon.ExecuteCommandResult("cpu online z80")
	requireMonitorOutputContains(t, out, "no staged coprocessor service image for z80")
}

func TestIEMONCPUOnlineInfersAndValidatesTypedImage(t *testing.T) {
	_, mgr, mon, baseDir := newIEMONCoprocHarness(t)
	writeZ80LoopService(t, baseDir, "svc.ie80")
	if err := os.WriteFile(filepath.Join(baseDir, "svc.iex"), []byte{0xEE}, 0644); err != nil {
		t.Fatal(err)
	}

	_, out := mon.ExecuteCommandResult("cpu online svc.ie80")
	requireMonitorOutputContains(t, out, "Online z80 as coproc:Z80")
	if !mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker is not running")
	}
	_, _ = mon.ExecuteCommandResult("cpu offline z80")

	_, out = mon.ExecuteCommandResult("cpu online z80 svc.ie80")
	requireMonitorOutputContains(t, out, "Online z80 as coproc:Z80")
	_, _ = mon.ExecuteCommandResult("cpu offline z80")

	_, out = mon.ExecuteCommandResult("cpu online z80 svc.iex")
	requireMonitorOutputContains(t, out, "does not match image extension")
	if mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker started after type/extension mismatch")
	}
}

func TestIEMONCPUDuplicateOnlineRequiresReplaceAndOfflineReturnsSlot(t *testing.T) {
	_, mgr, mon, baseDir := newIEMONCoprocHarness(t)
	writeZ80LoopService(t, baseDir, "svc.ie80")

	_, out := mon.ExecuteCommandResult("cpu online svc.ie80")
	requireMonitorOutputContains(t, out, "Online z80 as coproc:Z80")

	_, out = mon.ExecuteCommandResult("cpu online svc.ie80")
	requireMonitorOutputContains(t, out, "already online")
	if !mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker stopped after refused duplicate start")
	}

	_, out = mon.ExecuteCommandResult("cpu online --replace svc.ie80")
	requireMonitorOutputContains(t, out, "Online z80 as coproc:Z80")
	if !mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker is not running after --replace")
	}

	_, out = mon.ExecuteCommandResult("cpu offline z80")
	requireMonitorOutputContains(t, out, "Offline coproc:Z80")
	if mgr.IsWorkerRunning(EXEC_TYPE_Z80) {
		t.Fatal("Z80 worker still running after offline")
	}
	_, out = mon.ExecuteCommandResult("cpu")
	requireMonitorOutputContains(t, out, "coproc:Z80")
	requireMonitorOutputContains(t, out, "[OFFLINE]")
}

func TestHelpCPUDocumentsOnlineOffline(t *testing.T) {
	mon := NewMachineMonitor(nil)

	_, out := mon.ExecuteCommandResult("help cpu")
	requireMonitorOutputContains(t, out, "cpu online [--replace] <type|path.ie*> [path.ie*]")
	requireMonitorOutputContains(t, out, "cpu offline <id|label|type>")
}
