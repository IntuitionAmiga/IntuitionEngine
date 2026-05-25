package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func outputContains(lines []OutputLine, text string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, text) {
			return true
		}
	}
	return false
}

func TestAccessService_EventCarriesCpuIdAndKind(t *testing.T) {
	svc := NewDebugAccessService()
	ch := make(chan BreakpointEvent, 1)
	svc.RegisterCPU(2, ch)
	svc.Guard(0x1000, 0x10FF, PermRead|PermExecute, GuardScope{AllCPUs: true})

	svc.OnFetch(2, 0x1004, 1)

	select {
	case ev := <-ch:
		if !ev.IsGuard || ev.CPUID != 2 || ev.Access != AccessExecute || ev.Address != 0x1004 {
			t.Fatalf("event = %+v, want guard cpu=2 execute addr=0x1004", ev)
		}
	default:
		t.Fatal("expected guard event")
	}
}

func TestAccessHitStopsCPUBeforePublishingEvent(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	cpu := NewCPU64(bus)
	id := mon.RegisterCPU("IE64", NewDebugIE64(cpu))
	cpu.running.Store(true)

	mon.access.Guard(0x1200, 0x12FF, PermRead, GuardScope{CPUID: id})
	mon.access.OnRead(id, 0x1234, 1)

	if cpu.IsRunning() {
		t.Fatal("CPU still running after synchronous access guard hit")
	}
	select {
	case ev := <-mon.breakpointChan:
		if !ev.IsGuard || ev.CPUID != id || ev.Address != 0x1234 {
			t.Fatalf("event = %+v, want guard hit at $1234", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected guard event after synchronous stop")
	}
}

func TestAccessHitIn6502TrapLoopDoesNotDeadlock(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	adapter := NewDebug6502(cpu, nil)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("6502", adapter)

	mem := bus.GetMemory()
	mem[0x0600] = 0xA5 // LDA $10
	mem[0x0601] = 0x10
	mem[0x0010] = 0x42
	cpu.PC = 0x0600
	mon.ExecuteCommand("pg add $0010 $0010 r cpu=current")

	adapter.Resume()
	select {
	case ev := <-mon.breakpointChan:
		if !ev.IsGuard || ev.CPUID != 0 || ev.Address != 0x0010 {
			t.Fatalf("event = %+v, want 6502 read guard at $0010", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("6502 access guard in trap loop deadlocked or failed to publish")
	}
	deadline := time.After(500 * time.Millisecond)
	for adapter.trapRunning.Load() {
		select {
		case <-deadline:
			t.Fatal("6502 trap loop did not exit after access guard hit")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestPageGuard_CommandListAndClear(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.access.SetInstrumented(true)

	mon.ExecuteCommand("pg add $1000 $10ff rx cpu=current")
	_, out := mon.ExecuteCommandResult("pg list")
	var saw bool
	for _, line := range out {
		if strings.Contains(line.Text, "$1000-$10FF rx cpu=0") {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("pg list output = %#v, want guard", out)
	}

	mon.ExecuteCommand("pg clear")
	if guards := mon.access.ListGuards(); len(guards) != 0 {
		t.Fatalf("guards after clear = %#v, want empty", guards)
	}
}

func TestAccessEventRing_RecordsLastNAndOverlap(t *testing.T) {
	svc := NewDebugAccessService()
	svc.EnableHistory(2)

	svc.OnRead(1, 0x1000, 1)
	svc.OnWrite(2, 0x2000, 4, 0x11223344, 0x55667788)
	svc.OnFetch(3, 0x3000, 2)

	events := svc.HistoryTail(8)
	if len(events) != 2 {
		t.Fatalf("history len = %d, want 2", len(events))
	}
	if events[0].Kind != AccessWrite || events[1].Kind != AccessExecute {
		t.Fatalf("events = %+v, want write then execute", events)
	}
	ev, ok := svc.LastAccess(AccessWrite, 0x2003)
	if !ok {
		t.Fatal("expected write lookup to match later byte in access width")
	}
	if ev.CPUID != 2 || ev.Address != 0x2000 || ev.Width != 4 || ev.NewValue != 0x55667788 {
		t.Fatalf("write event = %+v, want cpu=2 addr=0x2000 width=4 new=0x55667788", ev)
	}
}

func TestAccessLogAndWhoCommands(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.ExecuteCommand("accesslog on 4")
	mon.bus.Write32(0x4000, 0xBB)
	_ = mon.bus.Read16(0x4000)
	mon.bus.Write64(0x4010, 0x1122334455667788)

	_, out := mon.ExecuteCommandResult("who wrote $4002")
	if !outputContains(out, "cpu=-1") || !outputContains(out, "write $4000..$4003") {
		t.Fatalf("who output = %#v, want matching write event", out)
	}

	_, out = mon.ExecuteCommandResult("who read $4001")
	if !outputContains(out, "cpu=-1") || !outputContains(out, "read $4000..$4001") {
		t.Fatalf("who read output = %#v, want matching bus read event", out)
	}

	_, out = mon.ExecuteCommandResult("who wrote $4017")
	if !outputContains(out, "write $4010..$4017") {
		t.Fatalf("who write64 output = %#v, want matching 64-bit bus write", out)
	}
}

func TestAccessLog_WriteEventsCaptureOldValues(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 16")

	bus.Write8(0x4100, 0x12)
	bus.Write8(0x4100, 0x34)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x4100); !ok || ev.OldValue != 0x12 || ev.NewValue != 0x34 {
		t.Fatalf("write8 event = %+v, ok=%v; want old=$12 new=$34", ev, ok)
	}

	bus.Write16(0x4102, 0x1122)
	bus.Write16(0x4102, 0x3344)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x4103); !ok || ev.OldValue != 0x1122 || ev.NewValue != 0x3344 {
		t.Fatalf("write16 event = %+v, ok=%v; want old=$1122 new=$3344", ev, ok)
	}

	bus.Write32(0x4104, 0x11223344)
	bus.Write32(0x4104, 0x55667788)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x4107); !ok || ev.OldValue != 0x11223344 || ev.NewValue != 0x55667788 {
		t.Fatalf("write32 event = %+v, ok=%v; want old=$11223344 new=$55667788", ev, ok)
	}

	bus.Write64(0x4110, 0x1122334455667788)
	bus.Write64(0x4110, 0x99AABBCCDDEEFF00)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x4117); !ok || ev.OldValue != 0x1122334455667788 || ev.NewValue != 0x99AABBCCDDEEFF00 {
		t.Fatalf("write64 event = %+v, ok=%v; want old=$1122334455667788 new=$99AABBCCDDEEFF00", ev, ok)
	}
}

func TestAccessLog_BackedRAMWriteEventsCaptureOldValues(t *testing.T) {
	bus := NewMachineBus()
	base := uint32(len(bus.GetMemory())) + 0x2000
	bus.SetBacking(NewSparseBacking(uint64(base) + 0x1000))
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 16")

	bus.Write8(base, 0x12)
	bus.Write8(base, 0x34)
	if ev, ok := mon.access.LastAccess(AccessWrite, uint64(base)); !ok || !ev.OldValueKnown || ev.OldValue != 0x12 || ev.NewValue != 0x34 {
		t.Fatalf("backed write8 event = %+v, ok=%v; want old=$12 new=$34", ev, ok)
	}

	bus.Write16(base+2, 0x1122)
	bus.Write16(base+2, 0x3344)
	if ev, ok := mon.access.LastAccess(AccessWrite, uint64(base+3)); !ok || !ev.OldValueKnown || ev.OldValue != 0x1122 || ev.NewValue != 0x3344 {
		t.Fatalf("backed write16 event = %+v, ok=%v; want old=$1122 new=$3344", ev, ok)
	}

	bus.Write32(base+4, 0x11223344)
	bus.Write32(base+4, 0x55667788)
	if ev, ok := mon.access.LastAccess(AccessWrite, uint64(base+7)); !ok || !ev.OldValueKnown || ev.OldValue != 0x11223344 || ev.NewValue != 0x55667788 {
		t.Fatalf("backed write32 event = %+v, ok=%v; want old=$11223344 new=$55667788", ev, ok)
	}
}

func TestAccessWatchpointsClearedWhenCPUUnregistered(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	cpu := NewCPU64(bus)
	id := mon.RegisterCPU("IE64", NewDebugIE64(cpu))

	mon.access.Watch(id, 0x2345, 1, WatchWrite)
	if len(mon.access.watches) != 1 {
		t.Fatalf("watches before unregister = %d, want 1", len(mon.access.watches))
	}

	mon.UnregisterCPU(id)
	if len(mon.access.watches) != 0 {
		t.Fatalf("watches after unregister = %#v, want empty", mon.access.watches)
	}
}

func TestAccessLog_IE64DirectWriteCapturesOldValue(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	cpu := NewCPU64(bus)
	mon.RegisterCPU("IE64", NewDebugIE64(cpu))
	mon.ExecuteCommand("accesslog on 16")

	binary.LittleEndian.PutUint32(cpu.memory[0x2400:], 0x11223344)
	cpu.storeMem(0x2400, 0x55667788, IE64_SIZE_L)

	ev, ok := mon.access.LastAccess(AccessWrite, 0x2400)
	if !ok {
		t.Fatal("missing IE64 direct write event")
	}
	if !ev.OldValueKnown || ev.OldValue != 0x11223344 || ev.NewValue != 0x55667788 {
		t.Fatalf("IE64 write event = %+v, want old=$11223344 new=$55667788", ev)
	}
}

func TestAccessLog_IE64HighPhysWriteCapturesOldValue(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	addr := uint64(len(cpu.memory)) + 0x2000
	bus.SetBacking(NewSparseBacking(addr + 0x1000))
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE64", NewDebugIE64(cpu))
	mon.ExecuteCommand("accesslog on 16")

	bus.backing.Write32(addr, 0x10203040)
	cpu.storeMem(addr, 0x50607080, IE64_SIZE_L)

	ev, ok := mon.access.LastAccess(AccessWrite, addr)
	if !ok {
		t.Fatal("missing IE64 high-phys write event")
	}
	if ev.CPUID != 0 || !ev.OldValueKnown || ev.OldValue != 0x10203040 || ev.NewValue != 0x50607080 {
		t.Fatalf("IE64 high-phys write event = %+v, want cpu=0 old=$10203040 new=$50607080", ev)
	}
	if ev, ok := mon.access.LastAccess(AccessRead, addr); ok {
		t.Fatalf("old-value sampling recorded a read event: %+v", ev)
	}
}

func TestAccessLog_IE64MMIOWriteOldValueUnknown(t *testing.T) {
	bus := NewMachineBus()
	var writes int
	bus.MapIONoShadow(0xF6000, 0xF6003, func(addr uint32) uint32 {
		return 0xDEADBEEF
	}, func(addr uint32, value uint32) {
		writes++
	})
	mon := NewMachineMonitor(bus)
	cpu := NewCPU64(bus)
	mon.RegisterCPU("IE64", NewDebugIE64(cpu))
	mon.ExecuteCommand("accesslog on 16")

	cpu.storeMem(0xF6000, 0xABCDEF01, IE64_SIZE_L)

	if writes != 1 {
		t.Fatalf("MMIO writes = %d, want 1", writes)
	}
	ev, ok := mon.access.LastAccess(AccessWrite, 0xF6000)
	if !ok {
		t.Fatal("missing IE64 MMIO write event")
	}
	if ev.CPUID != 0 || ev.OldValueKnown || ev.OldValue != 0 || ev.NewValue != 0xABCDEF01 {
		t.Fatalf("IE64 MMIO write event = %+v, want cpu=0 unknown old new=$ABCDEF01", ev)
	}
}

func TestAccessLog_IE32DirectWriteCapturesOldValue(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	cpu := NewCPU(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	mon.ExecuteCommand("accesslog on 16")

	binary.LittleEndian.PutUint32(cpu.memory[0x2800:], 0x01020304)
	cpu.Write32(0x2800, 0xA0B0C0D0)

	ev, ok := mon.access.LastAccess(AccessWrite, 0x2800)
	if !ok {
		t.Fatal("missing IE32 direct write event")
	}
	if !ev.OldValueKnown || ev.OldValue != 0x01020304 || ev.NewValue != 0xA0B0C0D0 {
		t.Fatalf("IE32 write event = %+v, want old=$01020304 new=$A0B0C0D0", ev)
	}
}

func TestAccessLog_6502FastWritesCaptureOldValue(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	cpu := NewCPU_6502(bus)
	mon.RegisterCPU("6502", NewDebug6502(cpu, nil))
	mon.ExecuteCommand("accesslog on 16")

	mem := bus.GetMemory()
	mem[0x0042] = 0x7A
	cpu.writeByte(0x0042, 0x99)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x0042); !ok || !ev.OldValueKnown || ev.OldValue != 0x7A || ev.NewValue != 0x99 {
		t.Fatalf("zero-page fast write event = %+v, ok=%v; want old=$7A new=$99", ev, ok)
	}

	mem[0x0120] = 0x33
	cpu.fastAdapter.WriteStack(0x20, 0x44)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x0120); !ok || !ev.OldValueKnown || ev.OldValue != 0x33 || ev.NewValue != 0x44 {
		t.Fatalf("stack fast write event = %+v, ok=%v; want old=$33 new=$44", ev, ok)
	}

	mem[0x2400] = 0x55
	cpu.writeByte(0x2400, 0x66)
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x2400); !ok || !ev.OldValueKnown || ev.OldValue != 0x55 || ev.NewValue != 0x66 {
		t.Fatalf("slow plain-RAM write event = %+v, ok=%v; want old=$55 new=$66", ev, ok)
	}

	translated := uint32(2 * BANK_WINDOW_SIZE)
	mem[translated] = 0x22
	cpu.fastAdapter.bank1 = 2
	cpu.fastAdapter.bank1Enable = true
	cpu.writeByte(BANK1_WINDOW_BASE, 0x77)
	ev, ok := mon.access.LastAccess(AccessWrite, uint64(translated))
	if !ok {
		t.Fatal("missing translated 6502 bus write event")
	}
	if ev.CPUID != 0 || ev.OldValueKnown || ev.OldValue != 0 || ev.NewValue != 0x77 {
		t.Fatalf("translated bus write event = %+v, want cpu=0 old=? new=$77", ev)
	}
}

func TestAccessLog_MMIOWriteOldValueDoesNotInvokeReadCallback(t *testing.T) {
	bus := NewMachineBus()
	var reads int
	var writes int
	bus.MapIONoShadow(0x5000, 0x5003, func(addr uint32) uint32 {
		reads++
		return 0xDEADBEEF
	}, func(addr uint32, value uint32) {
		writes++
	})

	bus.Write32(0x5000, 0x11111111)
	if reads != 0 || writes != 1 {
		t.Fatalf("inactive MMIO write callbacks reads=%d writes=%d, want reads=0 writes=1", reads, writes)
	}

	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")
	bus.Write32(0x5000, 0x22222222)
	if reads != 0 || writes != 2 {
		t.Fatalf("active MMIO write callbacks reads=%d writes=%d, want reads=0 writes=2", reads, writes)
	}
	ev, ok := mon.access.LastAccess(AccessWrite, 0x5000)
	if !ok || ev.OldValueKnown || ev.NewValue != 0x22222222 {
		t.Fatalf("MMIO event = %+v, ok=%v; want unknown old value and new=$22222222", ev, ok)
	}
	if got := formatAccessEvent(ev); !strings.Contains(got, "old=?") || !strings.Contains(got, "new=$22222222") {
		t.Fatalf("formatted event = %q, want unknown old value and new value", got)
	}
}

func TestAccessLog_ShadowedMMIOWriteOldValueFromShadow(t *testing.T) {
	bus := NewMachineBus()
	var reads int
	bus.MapIOShadow(0x5100, 0x5103, func(addr uint32) uint32 {
		reads++
		return 0xDEADBEEF
	}, func(addr uint32, value uint32) {})
	bus.Write32(0x5100, 0x12345678)

	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")
	bus.Write32(0x5100, 0xCAFEBABE)
	if reads != 0 {
		t.Fatalf("shadowed MMIO old-value sampling invoked read callback %d time(s)", reads)
	}
	ev, ok := mon.access.LastAccess(AccessWrite, 0x5103)
	if !ok || !ev.OldValueKnown || ev.OldValue != 0x12345678 || ev.NewValue != 0xCAFEBABE {
		t.Fatalf("shadowed MMIO event = %+v, ok=%v; want known old=$12345678 new=$CAFEBABE", ev, ok)
	}
}

func TestAccessLog_SignExtendedWriteOldValueSamplesContiguousMappedSpan(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()
	mem[0xFFFE] = 0x11
	mem[0xFFFF] = 0x22
	mem[0x10000] = 0x33
	mem[0x10001] = 0x44
	mem[0] = 0xAA
	mem[1] = 0xBB
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")

	bus.Write32(0xFFFFFFFE, 0x55667788)

	ev, ok := mon.access.LastAccess(AccessWrite, 0xFFFFFFFE)
	if !ok || !ev.OldValueKnown || ev.OldValue != 0x44332211 || ev.NewValue != 0x55667788 {
		t.Fatalf("sign-extended write event = %+v, ok=%v; want old=$44332211 new=$55667788", ev, ok)
	}
	if got := binary.LittleEndian.Uint32(mem[0xFFFE:0x10002]); got != 0x55667788 {
		t.Fatalf("mapped memory value=$%08X, want $55667788", got)
	}
	if mem[0] != 0xAA || mem[1] != 0xBB {
		t.Fatalf("low wrapped bytes changed/sampled unexpectedly: mem[0]=$%02X mem[1]=$%02X", mem[0], mem[1])
	}
}

func TestAccessLog_NoEventForNoOpWrappingWrite64(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()
	binary.LittleEndian.PutUint64(mem[0xFFFC:0x10004], 0x1122334455667788)
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")

	bus.Write64(0xFFFFFFFC, 0xAABBCCDDEEFF0011)

	if got := binary.LittleEndian.Uint64(mem[0xFFFC:0x10004]); got != 0x1122334455667788 {
		t.Fatalf("wrapping Write64 mutated memory: got $%016X", got)
	}
	if ev, ok := mon.access.LastAccess(AccessWrite, 0xFFFFFFFC); ok {
		t.Fatalf("unexpected write event for no-op wrapping Write64: %+v", ev)
	}
}

func TestAccessLog_NoEventForLegacyFaultPolicyWrite64NoOp(t *testing.T) {
	bus := NewMachineBus()
	var writes int
	bus.MapIO(0x5200, 0x5207, func(addr uint32) uint32 { return 0 }, func(addr uint32, value uint32) {
		writes++
	})
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")

	bus.Write64(0x5200, 0x1122334455667788)

	if writes != 0 {
		t.Fatalf("legacy fault-policy Write64 invoked %d legacy write(s), want none", writes)
	}
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x5200); ok {
		t.Fatalf("unexpected write event for legacy fault-policy Write64 no-op: %+v", ev)
	}
}

func TestAccessLog_EventForPartialWrite64Mutation(t *testing.T) {
	bus := NewMachineBus()
	var writes int
	bus.MapIO(0x5300, 0x5303, func(addr uint32) uint32 { return 0 }, func(addr uint32, value uint32) {
		writes++
	})
	mem := bus.GetMemory()
	binary.LittleEndian.PutUint32(mem[0x52FC:0x5300], 0xAABBCCDD)
	mon := NewMachineMonitor(bus)
	mon.ExecuteCommand("accesslog on 4")

	bus.Write64(0x52FC, 0x1122334455667788)

	if got := binary.LittleEndian.Uint32(mem[0x52FC:0x5300]); got != 0x55667788 {
		t.Fatalf("low RAM half=$%08X, want $55667788", got)
	}
	if writes != 0 {
		t.Fatalf("legacy fault-policy high half invoked %d write(s), want none", writes)
	}
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x52FC); !ok || ev.Address != 0x52FC || ev.Width != 4 || ev.NewValue != 0x55667788 {
		t.Fatalf("partial Write64 event = %+v, ok=%v; want 4-byte event for real low-half mutation", ev, ok)
	}
	if ev, ok := mon.access.LastAccess(AccessWrite, 0x5300); ok {
		t.Fatalf("unexpected event covering no-op high MMIO half: %+v", ev)
	}
}

func TestAccessLog_PhysBackingReadWrite(t *testing.T) {
	bus := NewMachineBus()
	addr := uint64(len(bus.GetMemory())) + 0x40
	bus.SetBacking(NewSparseBacking(addr + 0x1000))
	mon := NewMachineMonitor(bus)

	mon.ExecuteCommand("accesslog on 4")
	bus.WritePhys32(addr, 0xAABBCCDD)
	_ = bus.ReadPhys16(addr + 2)

	ev, ok := mon.access.LastAccess(AccessWrite, addr+3)
	if !ok || ev.CPUID != -1 || ev.Address != addr || ev.Width != 4 {
		t.Fatalf("phys write event = %+v, ok=%v; want cpu=-1 addr=$%X width=4", ev, ok, addr)
	}

	bus.WritePhys32(addr, 0x11223344)
	ev, ok = mon.access.LastAccess(AccessWrite, addr+3)
	if !ok || ev.OldValue != 0xAABBCCDD || ev.NewValue != 0x11223344 {
		t.Fatalf("phys write event = %+v, ok=%v; want old=$AABBCCDD new=$11223344", ev, ok)
	}

	ev, ok = mon.access.LastAccess(AccessRead, addr+3)
	if !ok || ev.CPUID != -1 || ev.Address != addr+2 || ev.Width != 2 {
		t.Fatalf("phys read event = %+v, ok=%v; want cpu=-1 addr=$%X width=2", ev, ok, addr+2)
	}
}

func TestBreakFirst_FiresOnceOnlyForRegion(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.ExecuteCommand("bfirst write mmio")
	guards := mon.access.ListGuards()
	if len(guards) != 1 || !guards[0].Once || guards[0].Start != 0xF0000 || guards[0].End != 0xFFFFF {
		t.Fatalf("guards = %#v, want one-shot mmio write guard", guards)
	}

	mon.access.OnWrite(0, 0xF0000, 1, 0, 1)
	if guards := mon.access.ListGuards(); len(guards) != 0 {
		t.Fatalf("guards after first write = %#v, want cleared", guards)
	}
}

func TestTraceMMIO_FiltersAccessHistoryByRegion(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.ExecuteCommand("accesslog on 4")
	mon.bus.Write8(0xF0000, 1)
	mon.bus.Write8(0x1000, 2)

	_, out := mon.ExecuteCommandResult("trace mmio mmio 4")
	if !outputContains(out, "write $F0000") {
		t.Fatalf("trace mmio output = %#v, want mmio write", out)
	}
	if outputContains(out, "write $1000") {
		t.Fatalf("trace mmio output = %#v, should not include non-mmio write", out)
	}
}

func TestAccessCommandsUseBusInstrumentation(t *testing.T) {
	mon, _ := newTestMonitor()

	_, out := mon.ExecuteCommandResult("accesslog on 4")
	if !outputContains(out, "Access log enabled") {
		t.Fatalf("accesslog output = %#v, want enabled", out)
	}

	_, out = mon.ExecuteCommandResult("pg add $1000 $10ff r")
	if !outputContains(out, "Guard added") {
		t.Fatalf("pg output = %#v, want guard", out)
	}

	_, out = mon.ExecuteCommandResult("bfirst write mmio")
	if !outputContains(out, "Break-on-first") {
		t.Fatalf("bfirst output = %#v, want armed", out)
	}
}

func TestBusGuard_DoesNotFocusSyntheticCPU(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.Activate()
	mon.ExecuteCommand("pg add $1000 $10ff w")
	mon.ExecuteCommand("accesslog on 4")

	mon.bus.Write8(0x1000, 0x7F)

	if mon.focusedID != 0 {
		t.Fatalf("focusedID = %d, want unchanged real CPU 0", mon.focusedID)
	}
	_, out := mon.ExecuteCommandResult("who wrote $1000")
	if !outputContains(out, "cpu=-1") {
		t.Fatalf("who wrote output = %#v, want passive synthetic bus history", out)
	}
}

func Test6502BusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	mem := bus.GetMemory()
	mem[0x0600] = 0xA9 // LDA #$7F
	mem[0x0601] = 0x7F
	mem[0x0602] = 0x8D // STA $D200 -> POKEY_BASE
	pokeyAddr := uint16(C6502_POKEY_BASE)
	mem[0x0603] = byte(pokeyAddr)
	mem[0x0604] = byte(pokeyAddr >> 8)

	mon := NewMachineMonitor(bus)
	adapter := NewDebug6502(cpu, nil)
	mon.RegisterCPU("6502", adapter)
	mon.ExecuteCommand("pg add $f0d00 $f0d00 w cpu=current")

	adapter.Step()
	adapter.Step()

	select {
	case ev := <-mon.breakpointChan:
		if ev.CPUID != 0 || !ev.IsGuard || ev.Access != AccessWrite || ev.Address != POKEY_BASE {
			t.Fatalf("event = %+v, want cpu=0 POKEY write guard", ev)
		}
	case <-time.After(250 * time.Millisecond):
		_, out := mon.ExecuteCommandResult("who wrote $f0d00")
		t.Fatalf("expected 6502 bus-mediated guard event; who output = %#v", out)
	}
}

func TestIE64BusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.PC = 0
	cpu.regs[1] = 0x5A
	cpu.regs[2] = IO_REGION_START
	copy(cpu.memory[0:], ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	mon.ExecuteCommand("pg add $a0000 $a0000 w cpu=current")

	adapter.Step()

	select {
	case ev := <-mon.breakpointChan:
		if ev.CPUID != 0 || !ev.IsGuard || ev.Access != AccessWrite || ev.Address != IO_REGION_START {
			t.Fatalf("event = %+v, want cpu=0 IE64 MMIO write guard", ev)
		}
	case <-time.After(250 * time.Millisecond):
		_, out := mon.ExecuteCommandResult("who wrote $a0000")
		t.Fatalf("expected IE64 bus-mediated guard event; who output = %#v", out)
	}
}

func TestIE64BusGuard_ResumeAttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.PC = 0
	cpu.CoprocMode = true
	cpu.regs[1] = 0x5A
	cpu.regs[2] = IO_REGION_START
	copy(cpu.memory[0:], ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))
	copy(cpu.memory[8:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	mon.ExecuteCommand("pg add $a0000 $a0000 w cpu=current")

	adapter.Resume()

	select {
	case ev := <-mon.breakpointChan:
		if ev.CPUID != 0 || !ev.IsGuard || ev.Access != AccessWrite || ev.Address != IO_REGION_START {
			t.Fatalf("event = %+v, want cpu=0 IE64 resumed MMIO write guard", ev)
		}
	case <-time.After(250 * time.Millisecond):
		_, out := mon.ExecuteCommandResult("who wrote $a0000")
		t.Fatalf("expected IE64 resumed guard event; who output = %#v", out)
	}
}

func TestIE32BusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	mon.ExecuteCommand("pg add $a0000 $a0003 w cpu=current")

	cpu.Write32(IO_REGION_START, 0x12345678)
	expectGuardEvent(t, mon, 0, AccessWrite, IO_REGION_START)
}

func TestM68KBusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("M68K", NewDebugM68K(cpu, nil))
	mon.ExecuteCommand("pg add $a0000 $a0000 w cpu=current")

	cpu.Write8(IO_REGION_START, 0x12)
	expectGuardEvent(t, mon, 0, AccessWrite, IO_REGION_START)
}

func TestM68KDirectRAMAccessInstrumentation(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("M68K", NewDebugM68K(cpu, nil))

	mon.ExecuteCommand("pg add $0100 $0103 r cpu=current")
	cpu.Write32(0x0100, 0x11223344)
	if got := cpu.Read32(0x0100); got != 0x11223344 {
		t.Fatalf("Read32 = $%08X, want $11223344", got)
	}
	expectGuardEvent(t, mon, 0, AccessRead, 0x0100)

	mon.ExecuteCommand("pg clear")
	mon.ExecuteCommand("pg add $0200 $0203 w cpu=current")
	cpu.Write32(0x0200, 0x55667788)
	expectGuardEvent(t, mon, 0, AccessWrite, 0x0200)
}

func TestM68KTerminalGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("M68K", NewDebugM68K(cpu, nil))

	mon.ExecuteCommand("pg add $f0700 $f0700 r cpu=current")
	_ = cpu.Read8(TERM_OUT)
	expectGuardEvent(t, mon, 0, AccessRead, TERM_OUT)

	mon.ExecuteCommand("pg clear")
	mon.ExecuteCommand("pg add $fffff700 $fffff700 w cpu=current")
	cpu.Write8(TERM_OUT_SIGNEXT, 0x21)
	expectGuardEvent(t, mon, 0, AccessWrite, TERM_OUT_SIGNEXT)
}

func TestZ80BusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("Z80", NewDebugZ80(cpu, nil))
	hostAddr := translateIO8Bit(0xF000)
	mon.ExecuteCommand("pg add $f0000 $f0000 w cpu=current")

	cpu.write(0xF000, 0x34)
	expectGuardEvent(t, mon, 0, AccessWrite, hostAddr)
}

func TestX86BusGuard_AttributedToRealCPU(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("X86", NewDebugX86(cpu, nil))
	hostAddr := adapter.translateIO(0xF000)
	mon.ExecuteCommand("pg add $f0000 $f0003 w cpu=current")

	cpu.write32(0xF000, 0x12345678)
	expectGuardEvent(t, mon, 0, AccessWrite, hostAddr)
}

func TestAccessService_FetchGuards_AllCPUs(t *testing.T) {
	t.Run("6502", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		cpu.PC = 0x0600
		bus.GetMemory()[0x0600] = 0xEA // NOP
		mon := NewMachineMonitor(bus)
		adapter := NewDebug6502(cpu, nil)
		mon.RegisterCPU("6502", adapter)
		mon.ExecuteCommand("pg add $0600 $0600 x cpu=current")
		adapter.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0x0600)
	})

	t.Run("Z80", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewZ80BusAdapter(bus)
		cpu := NewCPU_Z80(adapter)
		bus.GetMemory()[0] = 0x00 // NOP
		mon := NewMachineMonitor(bus)
		dbg := NewDebugZ80(cpu, nil)
		mon.RegisterCPU("Z80", dbg)
		mon.ExecuteCommand("pg add $0000 $0000 x cpu=current")
		dbg.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0)
	})

	t.Run("M68K", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewM68KCPU(bus)
		cpu.PC = 0
		copy(bus.GetMemory()[0:], []byte{0x4E, 0x71}) // NOP
		mon := NewMachineMonitor(bus)
		dbg := NewDebugM68K(cpu, nil)
		mon.RegisterCPU("M68K", dbg)
		mon.ExecuteCommand("pg add $0000 $0001 x cpu=current")
		dbg.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0)
	})

	t.Run("IE32", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU(bus)
		cpu.PC = 0
		copy(cpu.memory[0:], createInstruction(NOP, 0, ADDR_IMMEDIATE, 0))
		mon := NewMachineMonitor(bus)
		adapter := NewDebugIE32(cpu)
		mon.RegisterCPU("IE32", adapter)
		mon.ExecuteCommand("pg add $0000 $0007 x cpu=current")
		adapter.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0)
	})

	t.Run("IE64", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU64(bus)
		cpu.PC = 0
		cpu.CoprocMode = true
		copy(cpu.memory[0:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
		mon := NewMachineMonitor(bus)
		adapter := NewDebugIE64(cpu)
		mon.RegisterCPU("IE64", adapter)
		mon.ExecuteCommand("pg add $0000 $0007 x cpu=current")
		adapter.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0)
	})

	t.Run("X86", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		bus.GetMemory()[0] = 0x90 // NOP
		mon := NewMachineMonitor(bus)
		dbg := NewDebugX86(cpu, nil)
		mon.RegisterCPU("X86", dbg)
		mon.ExecuteCommand("pg add $0000 $0000 x cpu=current")
		dbg.Step()
		expectGuardEvent(t, mon, 0, AccessExecute, 0)
	})
}

func TestAccessService_M68KExecuteInstructionFetchGuard(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.PC = 0
	copy(bus.GetMemory()[0:], []byte{0x4E, 0x71}) // NOP
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("M68K", NewDebugM68K(cpu, nil))
	mon.ExecuteCommand("pg add $0000 $0001 x cpu=current")

	cpu.InstructionHook = func(cpu *M68KCPU) {
		cpu.running.Store(false)
	}
	cpu.running.Store(true)
	cpu.ExecuteInstruction()
	expectGuardEvent(t, mon, 0, AccessExecute, 0)
}

func TestAccessService_FetchDoesNotTripReadGuards_ByteCPUs(t *testing.T) {
	t.Run("6502", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		cpu.PC = 0x0600
		bus.GetMemory()[0x0600] = 0xEA // NOP
		mon := NewMachineMonitor(bus)
		adapter := NewDebug6502(cpu, nil)
		mon.RegisterCPU("6502", adapter)
		mon.ExecuteCommand("accesslog on 8")
		mon.ExecuteCommand("pg add $0600 $0600 r cpu=current")
		adapter.Step()
		assertNoBreakpointEvent(t, mon)
		_, out := mon.ExecuteCommandResult("who read $0600")
		if !outputContains(out, "No read recorded") {
			t.Fatalf("who read output = %#v, want no data read for fetch", out)
		}
	})

	t.Run("6502 I/O page", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewBus6502Adapter(bus)
		bus.GetMemory()[POKEY_BASE] = 0xEA
		mon := NewMachineMonitor(bus)
		mon.ExecuteCommand("accesslog on 8")

		if got := adapter.FetchFast(C6502_POKEY_BASE); got != 0xEA {
			t.Fatalf("FetchFast POKEY page = $%02X, want mapped byte $EA", got)
		}
		if ev, ok := mon.access.LastAccess(AccessRead, C6502_POKEY_BASE); ok {
			t.Fatalf("unexpected source read event for fetch: %+v", ev)
		}
		if ev, ok := mon.access.LastAccess(AccessRead, POKEY_BASE); ok {
			t.Fatalf("unexpected mapped read event for fetch: %+v", ev)
		}
	})

	t.Run("Z80", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewZ80BusAdapter(bus)
		cpu := NewCPU_Z80(adapter)
		bus.GetMemory()[0] = 0x00 // NOP
		mon := NewMachineMonitor(bus)
		dbg := NewDebugZ80(cpu, nil)
		mon.RegisterCPU("Z80", dbg)
		mon.ExecuteCommand("accesslog on 8")
		mon.ExecuteCommand("pg add $0000 $0000 r cpu=current")
		dbg.Step()
		assertNoBreakpointEvent(t, mon)
		_, out := mon.ExecuteCommandResult("who read $0000")
		if !outputContains(out, "No read recorded") {
			t.Fatalf("who read output = %#v, want no data read for fetch", out)
		}
	})

	t.Run("X86", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		bus.GetMemory()[0] = 0x90 // NOP
		mon := NewMachineMonitor(bus)
		dbg := NewDebugX86(cpu, nil)
		mon.RegisterCPU("X86", dbg)
		mon.ExecuteCommand("accesslog on 8")
		mon.ExecuteCommand("pg add $0000 $0000 r cpu=current")
		dbg.Step()
		assertNoBreakpointEvent(t, mon)
		_, out := mon.ExecuteCommandResult("who read $0000")
		if !outputContains(out, "No read recorded") {
			t.Fatalf("who read output = %#v, want no data read for fetch", out)
		}
	})
}

func TestAccessService_M68KFetch32DoesNotRecordRead(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.PC = 0
	copy(bus.GetMemory()[0:], []byte{0x12, 0x34, 0x56, 0x78})
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("M68K", NewDebugM68K(cpu, nil))
	mon.ExecuteCommand("accesslog on 8")
	mon.ExecuteCommand("pg add $0000 $0003 r cpu=current")

	if got := cpu.Fetch32(); got != 0x12345678 {
		t.Fatalf("Fetch32 = $%08X, want $12345678", got)
	}
	assertNoBreakpointEvent(t, mon)
	if ev, ok := mon.access.LastAccess(AccessRead, 0); ok {
		t.Fatalf("unexpected data read event for Fetch32 high word: %+v", ev)
	}
	if ev, ok := mon.access.LastAccess(AccessRead, 2); ok {
		t.Fatalf("unexpected data read event for Fetch32 low word: %+v", ev)
	}
	if _, ok := mon.access.LastAccess(AccessExecute, 0); !ok {
		t.Fatal("Fetch32 did not record execute event for high word")
	}
	if _, ok := mon.access.LastAccess(AccessExecute, 2); !ok {
		t.Fatal("Fetch32 did not record execute event for low word")
	}
}

func TestAccessService_FetchReadPreservesBankTranslation(t *testing.T) {
	t.Run("6502", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewBus6502Adapter(bus)
		mem := bus.GetMemory()
		mem[BANK1_WINDOW_BASE] = 0x00
		mem[2*BANK_WINDOW_SIZE] = 0xEA
		adapter.bank1 = 2
		adapter.bank1Enable = true
		if got := adapter.FetchFast(BANK1_WINDOW_BASE); got != 0xEA {
			t.Fatalf("FetchFast banked 6502 = $%02X, want translated byte $EA", got)
		}
	})

	t.Run("Z80", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewZ80BusAdapter(bus)
		mem := bus.GetMemory()
		mem[Z80_BANK1_WINDOW_BASE] = 0x00
		mem[2*Z80_BANK_WINDOW_SIZE] = 0x76
		adapter.bank1 = 2
		adapter.bank1Enable = true
		if got := adapter.fetchRead(Z80_BANK1_WINDOW_BASE); got != 0x76 {
			t.Fatalf("fetchRead banked Z80 = $%02X, want translated byte $76", got)
		}
	})

	t.Run("X86", func(t *testing.T) {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		mem := bus.GetMemory()
		mem[X86_BANK1_WINDOW_BASE] = 0x00
		mem[2*X86_BANK_WINDOW_SIZE] = 0x90
		adapter.bank1 = 2
		adapter.bank1Enable = true
		if got := adapter.fetchRead(X86_BANK1_WINDOW_BASE); got != 0x90 {
			t.Fatalf("fetchRead banked x86 = $%02X, want translated byte $90", got)
		}
	})
}

func TestAccessService_IE32IE64NormalRAMReadWrite(t *testing.T) {
	t.Run("IE32", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU(bus)
		mon := NewMachineMonitor(bus)
		mon.RegisterCPU("IE32", NewDebugIE32(cpu))

		mon.ExecuteCommand("pg add $4000 $4003 w cpu=current")
		cpu.Write32(0x4000, 0x11223344)
		expectGuardEvent(t, mon, 0, AccessWrite, 0x4000)

		mon.ExecuteCommand("pg clear")
		mon.ExecuteCommand("pg add $4000 $4003 r cpu=current")
		_ = cpu.Read32(0x4000)
		expectGuardEvent(t, mon, 0, AccessRead, 0x4000)
	})

	t.Run("IE64", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU64(bus)
		mon := NewMachineMonitor(bus)
		mon.RegisterCPU("IE64", NewDebugIE64(cpu))

		mon.ExecuteCommand("pg add $4000 $4007 w cpu=current")
		cpu.storeMem(0x4000, 0x1122334455667788, IE64_SIZE_Q)
		expectGuardEvent(t, mon, 0, AccessWrite, 0x4000)

		mon.ExecuteCommand("pg clear")
		mon.ExecuteCommand("pg add $4000 $4007 r cpu=current")
		_ = cpu.loadMem(0x4000, IE64_SIZE_Q)
		expectGuardEvent(t, mon, 0, AccessRead, 0x4000)
	})
}

func expectGuardEvent(t *testing.T, mon *MachineMonitor, cpuID int, kind AccessKind, addr uint32) {
	t.Helper()
	select {
	case ev := <-mon.breakpointChan:
		if ev.CPUID != cpuID || !ev.IsGuard || ev.Access != kind || ev.Address != uint64(addr) {
			t.Fatalf("event = %+v, want cpu=%d %s guard at $%X", ev, cpuID, accessKindString(kind), addr)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected guard event cpu=%d %s at $%X", cpuID, accessKindString(kind), addr)
	}
}

func assertNoBreakpointEvent(t *testing.T, mon *MachineMonitor) {
	t.Helper()
	select {
	case ev := <-mon.breakpointChan:
		t.Fatalf("unexpected breakpoint event: %+v", ev)
	default:
	}
}

func TestAccessService_UnattributedAccessDoesNotHitCurrentCPUGuard(t *testing.T) {
	svc := NewDebugAccessService()
	ch := make(chan BreakpointEvent, 1)
	svc.RegisterCPU(0, ch)
	svc.EnableHistory(4)
	svc.Guard(0x1000, 0x10FF, PermWrite, GuardScope{CPUID: 0})

	svc.OnWrite(-1, 0x1000, 1, 0, 1)

	select {
	case ev := <-ch:
		t.Fatalf("unexpected guard event for synthetic CPU: %+v", ev)
	default:
	}
	events := svc.HistoryTail(1)
	if len(events) != 1 || events[0].CPUID != -1 {
		t.Fatalf("history = %+v, want passive synthetic bus event", events)
	}
}

func TestAccessService_6502DirectReadWriteFetch(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	mem := bus.GetMemory()
	mem[0x0010] = 0x42
	mem[0x0600] = 0xA5 // LDA $10
	mem[0x0601] = 0x10
	mem[0x0602] = 0x85 // STA $11
	mem[0x0603] = 0x11

	mon := NewMachineMonitor(bus)
	adapter := NewDebug6502(cpu, nil)
	mon.RegisterCPU("6502", adapter)
	mon.ExecuteCommand("accesslog on 16")

	adapter.Step()
	adapter.Step()

	_, out := mon.ExecuteCommandResult("who fetched $0600")
	if !outputContains(out, "cpu=0") || !outputContains(out, "execute $600") {
		t.Fatalf("who fetched output = %#v, want 6502 fetch", out)
	}
	_, out = mon.ExecuteCommandResult("who read $0010")
	if !outputContains(out, "cpu=0") || !outputContains(out, "read $10") {
		t.Fatalf("who read output = %#v, want 6502 direct read", out)
	}
	_, out = mon.ExecuteCommandResult("who wrote $0011")
	if !outputContains(out, "cpu=0") || !outputContains(out, "write $11") {
		t.Fatalf("who wrote output = %#v, want 6502 direct write", out)
	}
}

func TestWatchpoint_ReadModeBackedByAccessService(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	mem := bus.GetMemory()
	mem[0x0010] = 0x42
	mem[0x0600] = 0xA5 // LDA $10
	mem[0x0601] = 0x10

	mon := NewMachineMonitor(bus)
	adapter := NewDebug6502(cpu, nil)
	mon.RegisterCPU("6502", adapter)
	_, out := mon.ExecuteCommandResult("bpmbr $0010")
	if !outputContains(out, "R1 watchpoint set at $10") {
		t.Fatalf("bpmbr output = %#v, want read watchpoint enabled", out)
	}

	adapter.Step()
	expectWatchEvent(t, mon, 0, AccessRead, 0x10)
}

func TestWatchpoint_ReadWriteModeBackedByAccessService(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	_, out := mon.ExecuteCommandResult("bpmd $a0000")
	if !outputContains(out, "RW4 watchpoint set at $A0000") {
		t.Fatalf("bpmd output = %#v, want read/write dword watchpoint enabled", out)
	}

	cpu.Write32(IO_REGION_START, 0x12345678)
	expectWatchEvent(t, mon, 0, AccessWrite, IO_REGION_START)
}

func TestWatchpoint_BPMWriteModeBackedByAccessService(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE32", NewDebugIE32(cpu))
	_, out := mon.ExecuteCommandResult("bpmdw $a0000")
	if !outputContains(out, "W4 watchpoint set at $A0000") {
		t.Fatalf("bpmdw output = %#v, want write dword watchpoint enabled", out)
	}

	cpu.Write32(IO_REGION_START, 0)
	expectWatchEvent(t, mon, 0, AccessWrite, IO_REGION_START)
}

func TestAccessDebugActiveResumeUsesTrapLoopInsteadOfJIT(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	adapter := NewDebugIE64(cpu)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("IE64", adapter)
	mon.access.SetInstrumented(true)
	mon.ExecuteCommand("pg add $1000 $10ff r cpu=current")

	adapter.Resume()
	defer adapter.Freeze()
	deadline := time.After(250 * time.Millisecond)
	for !adapter.trapRunning.Load() {
		select {
		case <-deadline:
			t.Fatal("access guard resume did not enter debug trap loop")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWatchpoint_AccessServiceClearedWithCommand(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	mem := bus.GetMemory()
	mem[0x0010] = 0x42
	mem[0x0600] = 0xA5 // LDA $10
	mem[0x0601] = 0x10

	mon := NewMachineMonitor(bus)
	adapter := NewDebug6502(cpu, nil)
	mon.RegisterCPU("6502", adapter)
	mon.ExecuteCommand("bpmbr $0010")
	mon.ExecuteCommand("wc $0010")

	adapter.Step()
	select {
	case ev := <-mon.breakpointChan:
		t.Fatalf("unexpected watchpoint event after clear: %+v", ev)
	default:
	}
}

func TestAccessService_GuardEventDeliveryWaitsWhenChannelFull(t *testing.T) {
	svc := NewDebugAccessService()
	ch := make(chan BreakpointEvent)
	svc.RegisterCPU(0, ch)
	svc.SetInstrumented(true)
	svc.Guard(0x1000, 0x1000, PermRead, GuardScope{CPUID: 0})

	done := make(chan struct{})
	go func() {
		svc.OnRead(0, 0x1000, 1)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("guard send returned before the stop event was received")
	case <-time.After(25 * time.Millisecond):
	}

	select {
	case ev := <-ch:
		if !ev.IsGuard || ev.CPUID != 0 || ev.Address != 0x1000 {
			t.Fatalf("event = %+v, want guard at $1000", ev)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("guard event was not delivered")
	}
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("guard send did not complete after receiver drained event")
	}
}

func expectWatchEvent(t *testing.T, mon *MachineMonitor, cpuID int, kind AccessKind, addr uint32) {
	t.Helper()
	select {
	case ev := <-mon.breakpointChan:
		if ev.CPUID != cpuID || !ev.IsWatch || ev.Access != kind || ev.WatchAddr != uint64(addr) {
			t.Fatalf("event = %+v, want cpu=%d %s watch at $%X", ev, cpuID, accessKindString(kind), addr)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected watchpoint event cpu=%d %s at $%X", cpuID, accessKindString(kind), addr)
	}
}
