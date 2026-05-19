package main

import (
	"context"
	"testing"
	"time"
)

func TestHostHelperMMIORegistersDirect(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK, ExitCode: 0x12345678})
	helper := NewHostHelperWithRunner(true, false, runner)
	helper.SetUpdateConfirmer(newScriptedHostUpdateConfirmer(true))

	if got := helper.HandleRead(HostMMIOBase + HostMMIOStatus); got != HostStatusIdle {
		t.Fatalf("status before trigger = %d, want IDLE", got)
	}

	helper.HandleWrite(HostMMIOBase+HostMMIOCommand, uint32(HostCommandUpdate))
	helper.HandleWrite(HostMMIOBase+HostMMIOTrigger, 1)

	select {
	case got := <-runner.calls:
		if got != HostCommandUpdate {
			t.Fatalf("runner command = %d, want %d", got, HostCommandUpdate)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked")
	}

	if got := helper.HandleRead(HostMMIOBase + HostMMIOStatus); got != HostStatusRunning {
		t.Fatalf("status while runner blocked = %d, want RUNNING", got)
	}

	runner.release()
	waitForHostStatus(t, helper, HostStatusOK)

	if got := helper.HandleRead(HostMMIOBase + HostMMIOStatus); got != HostStatusOK {
		t.Fatalf("status after completion = %d, want OK", got)
	}
	if got := helper.HandleRead(HostMMIOBase + HostMMIOExit); got != 0x12345678 {
		t.Fatalf("exit code = %#x, want 0x12345678", got)
	}
	if got := helper.HandleRead(HostMMIOBase + 0x20); got != 0 {
		t.Fatalf("unknown register read = %#x, want 0", got)
	}
}

func TestHostHelperMMIORegionIsDocumented(t *testing.T) {
	if got := GetIORegion(HostMMIOBase); got != "HostHelper" {
		t.Fatalf("GetIORegion(HostMMIOBase) = %q, want HostHelper", got)
	}
	if got := GetIORegion(HostMMIOEnd); got != "HostHelper" {
		t.Fatalf("GetIORegion(HostMMIOEnd) = %q, want HostHelper", got)
	}

	regions := NewRegionRegistry()
	region := regions.Lookup("IE64", HostMMIOBase)
	if region == nil || region.Name != "host-helper" || region.Kind != RegionMMIO {
		t.Fatalf("host helper region = %#v, want host-helper MMIO", region)
	}
}

func TestHostHelperMMIORegistersAcceptOffsets(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusErr, ExitCode: 7})
	helper := NewHostHelperWithRunner(true, false, runner)

	helper.HandleWrite(HostMMIOCommand, uint32(HostCommandPoweroff))
	helper.HandleWrite(HostMMIOTrigger, 1)

	select {
	case got := <-runner.calls:
		if got != HostCommandPoweroff {
			t.Fatalf("runner command = %d, want %d", got, HostCommandPoweroff)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked")
	}

	runner.release()
	waitForHostStatus(t, helper, HostStatusErr)

	if got := helper.HandleRead(HostMMIOStatus); got != HostStatusErr {
		t.Fatalf("offset status read = %d, want ERR", got)
	}
	if got := helper.HandleRead(HostMMIOExit); got != 7 {
		t.Fatalf("offset exit read = %d, want 7", got)
	}
}

func TestHostHelperMMIOIgnoresZeroTrigger(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	helper := NewHostHelperWithRunner(true, false, runner)

	helper.HandleWrite(HostMMIOBase+HostMMIOCommand, uint32(HostCommandNet))
	helper.HandleWrite(HostMMIOBase+HostMMIOTrigger, 0)

	if got := helper.HandleRead(HostMMIOBase + HostMMIOStatus); got != HostStatusIdle {
		t.Fatalf("status after zero trigger = %d, want IDLE", got)
	}
	select {
	case got := <-runner.calls:
		t.Fatalf("zero trigger invoked runner with command %d", got)
	case <-time.After(10 * time.Millisecond):
	}
}

func TestHostHelperMMIOBusMapping(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK, ExitCode: 0xA1B2C3D4})
	helper := NewHostHelperWithRunner(true, false, runner)
	bus := NewMachineBus()
	RegisterHostHelperMMIO(bus, helper)

	if got := bus.Read32(HostMMIOBase + HostMMIOStatus); got != HostStatusIdle {
		t.Fatalf("bus status before trigger = %d, want IDLE", got)
	}

	bus.Write32(HostMMIOBase+HostMMIOCommand, uint32(HostCommandReboot))
	bus.Write32(HostMMIOBase+HostMMIOTrigger, 1)

	select {
	case got := <-runner.calls:
		if got != HostCommandReboot {
			t.Fatalf("runner command = %d, want %d", got, HostCommandReboot)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked")
	}

	if got := bus.Read32(HostMMIOBase + HostMMIOStatus); got != HostStatusRunning {
		t.Fatalf("bus status while runner blocked = %d, want RUNNING", got)
	}

	runner.release()
	waitForHostStatus(t, helper, HostStatusOK)

	if got := bus.Read32(HostMMIOBase + HostMMIOExit); got != 0xA1B2C3D4 {
		t.Fatalf("bus exit code = %#x, want 0xA1B2C3D4", got)
	}
}

func TestHostHelperMMIOBusRead8ExitBytes(t *testing.T) {
	runner := immediateHostCommandRunner{result: HostCommandResult{Status: HostStatusOK, ExitCode: 0xA1B2C3D4}}
	helper := NewHostHelperWithRunner(true, false, runner)
	bus := NewMachineBus()
	RegisterHostHelperMMIO(bus, helper)

	bus.Write32(HostMMIOBase+HostMMIOCommand, uint32(HostCommandNet))
	bus.Write32(HostMMIOBase+HostMMIOTrigger, 1)
	waitForHostStatus(t, helper, HostStatusOK)

	got := []uint8{
		bus.Read8(HostMMIOBase + HostMMIOExit + 0),
		bus.Read8(HostMMIOBase + HostMMIOExit + 1),
		bus.Read8(HostMMIOBase + HostMMIOExit + 2),
		bus.Read8(HostMMIOBase + HostMMIOExit + 3),
	}
	want := []uint8{0xD4, 0xC3, 0xB2, 0xA1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("exit byte %d = %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestHostHelperMMIOBusRead16ExitWords(t *testing.T) {
	runner := immediateHostCommandRunner{result: HostCommandResult{Status: HostStatusOK, ExitCode: 0xA1B2C3D4}}
	helper := NewHostHelperWithRunner(true, false, runner)
	bus := NewMachineBus()
	RegisterHostHelperMMIO(bus, helper)

	bus.Write32(HostMMIOBase+HostMMIOCommand, uint32(HostCommandNet))
	bus.Write32(HostMMIOBase+HostMMIOTrigger, 1)
	waitForHostStatus(t, helper, HostStatusOK)

	if got := bus.Read16(HostMMIOBase + HostMMIOExit); got != 0xC3D4 {
		t.Fatalf("low exit word = %#x, want 0xC3D4", got)
	}
	if got := bus.Read16(HostMMIOBase + HostMMIOExit + 2); got != 0xA1B2 {
		t.Fatalf("high exit word = %#x, want 0xA1B2", got)
	}
}

func TestRegisterHostHelperMMIOProductionDefaultIsGuestVisibleDisabled(t *testing.T) {
	helper := NewHostHelperWithRunner(false, false, nil)
	bus := NewMachineBus()
	RegisterHostHelperMMIO(bus, helper)

	bus.Write32(HostMMIOBase+HostMMIOCommand, uint32(HostCommandUpdate))
	if got := bus.Read32(HostMMIOBase + HostMMIOCommand); got != uint32(HostCommandUpdate) {
		t.Fatalf("registered command read = %d, want %d", got, HostCommandUpdate)
	}

	bus.Write32(HostMMIOBase+HostMMIOTrigger, 1)
	if got := bus.Read32(HostMMIOBase + HostMMIOStatus); got != HostStatusDisabled {
		t.Fatalf("registered disabled status = %d, want DISABLED", got)
	}
}

type immediateHostCommandRunner struct {
	result HostCommandResult
}

func (r immediateHostCommandRunner) RunHostCommand(ctx context.Context, cmd HostCommand) HostCommandResult {
	return r.result
}
