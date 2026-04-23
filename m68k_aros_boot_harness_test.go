package main

import (
	"context"
	"testing"
	"time"
)

func TestM68KFaultManifest_DeduplicatesBySignature(t *testing.T) {
	manifest := NewM68KFaultManifest()

	sig := M68KFaultRecord{
		Class:          M68KFaultClassIllegalInstruction,
		PC:             0x00624918,
		FaultPC:        0x00624918,
		Opcode:         0x4E7B,
		MnemonicFamily: "MOVEA",
		AddressingMode: "(d16,An)",
	}
	manifest.Add(sig)
	manifest.Add(sig)
	manifest.Add(M68KFaultRecord{
		Class:          M68KFaultClassIllegalInstruction,
		PC:             0x00624918,
		FaultPC:        0x00624918,
		Opcode:         0x4E7B,
		MnemonicFamily: "MOVEA",
		AddressingMode: "(d16,An)",
		Message:        "same signature, different console text",
	})
	manifest.Add(M68KFaultRecord{
		Class:          M68KFaultClassLineF,
		PC:             0x00624918,
		FaultPC:        0x00624918,
		Opcode:         0xF200,
		MnemonicFamily: "FPU",
		AddressingMode: "",
	})

	records := manifest.Records()
	if len(records) != 2 {
		t.Fatalf("manifest dedupe count = %d, want 2", len(records))
	}
	if records[0].Count != 3 {
		t.Fatalf("first manifest count = %d, want 3", records[0].Count)
	}
	if records[0].Signature() != sig.Signature() {
		t.Fatalf("first manifest signature = %q, want %q", records[0].Signature(), sig.Signature())
	}
}

func TestM68KCPU_ProcessException_ReportsStructuredFault(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.PC = M68K_ENTRY_POINT + 2
	cpu.lastExecPC = M68K_ENTRY_POINT
	cpu.lastExecOpcode = 0x4AFC
	cpu.Write32(uint32(M68K_VEC_ILLEGAL_INSTR)*4, 0x00002000)

	var got M68KFaultRecord
	cpu.FaultHook = func(record M68KFaultRecord) {
		got = record
	}

	cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)

	if got.Class != M68KFaultClassIllegalInstruction {
		t.Fatalf("fault class = %q, want %q", got.Class, M68KFaultClassIllegalInstruction)
	}
	if got.FaultPC != M68K_ENTRY_POINT {
		t.Fatalf("fault pc = 0x%08X, want 0x%08X", got.FaultPC, M68K_ENTRY_POINT)
	}
	if got.Opcode != 0x4AFC {
		t.Fatalf("fault opcode = 0x%04X, want 0x4AFC", got.Opcode)
	}
	if got.Vector != M68K_VEC_ILLEGAL_INSTR {
		t.Fatalf("fault vector = %d, want %d", got.Vector, M68K_VEC_ILLEGAL_INSTR)
	}
}

func TestNormalizeM68KFaultRecord_DerivesMoveASignatureFromFaultPC(t *testing.T) {
	cpu := newMoveATestCPU()
	cpu.Write16(0x00624910, 0x226B)
	cpu.Write16(0x00624912, 0x0010)

	record := NormalizeM68KFaultRecord(cpu, M68KFaultRecord{
		Class:   M68KFaultClassIllegalInstruction,
		FaultPC: 0x00624910,
	})

	if record.Opcode != 0x226B {
		t.Fatalf("opcode = 0x%04X, want 0x226B", record.Opcode)
	}
	if record.MnemonicFamily != "MOVEA" {
		t.Fatalf("mnemonic family = %q, want %q", record.MnemonicFamily, "MOVEA")
	}
	if record.AddressingMode != "(d16,An)" {
		t.Fatalf("addressing mode = %q, want %q", record.AddressingMode, "(d16,An)")
	}
}

func TestProbeAROSReadyState_RequiresExecBaseAndTaskContext(t *testing.T) {
	cpu := newMoveATestCPU()

	if probe := ProbeAROSReadyState(cpu, nil); probe.Ready {
		t.Fatal("probe without SysBase unexpectedly reported ready")
	}

	sysBase := uint32(0x00004000)
	thisTask := uint32(0x00005000)
	taskName := uint32(0x00006000)
	readyNode := uint32(0x00007000)
	waitNode := uint32(0x00008000)

	cpu.Write32(4, sysBase)
	cpu.Write32(sysBase+arosExecThisTaskOffset, thisTask)
	cpu.Write32(thisTask+10, taskName)
	for i, b := range []byte("ExecTask\x00") {
		cpu.Write8(taskName+uint32(i), b)
	}

	cpu.Write32(sysBase+arosExecTaskReadyOffset, readyNode)
	cpu.Write32(readyNode, 0)
	cpu.Write32(sysBase+arosExecTaskWaitOffset, waitNode)
	cpu.Write32(waitNode, 0)

	probe := ProbeAROSReadyState(cpu, nil)
	if !probe.Ready {
		t.Fatalf("probe = %+v, want Ready=true", probe)
	}
	if probe.SysBase != sysBase {
		t.Fatalf("probe SysBase = 0x%08X, want 0x%08X", probe.SysBase, sysBase)
	}
	if probe.ThisTask != thisTask {
		t.Fatalf("probe ThisTask = 0x%08X, want 0x%08X", probe.ThisTask, thisTask)
	}
	if probe.TaskName != "ExecTask" {
		t.Fatalf("probe TaskName = %q, want %q", probe.TaskName, "ExecTask")
	}
}

func TestAROSBootHarness_ReturnsReadyState(t *testing.T) {
	cpu := newMoveATestCPU()
	sysBase := uint32(0x00004000)
	thisTask := uint32(0x00005000)
	taskName := uint32(0x00006000)
	cpu.Write32(4, sysBase)
	cpu.Write32(sysBase+arosExecThisTaskOffset, thisTask)
	cpu.Write32(thisTask+10, taskName)
	for i, b := range []byte("ExecTask\x00") {
		cpu.Write8(taskName+uint32(i), b)
	}
	cpu.Write32(sysBase+arosExecTaskReadyOffset, 0x00007000)
	cpu.Write32(0x00007000, 0)
	cpu.Write32(sysBase+arosExecTaskWaitOffset, 0x00008000)
	cpu.Write32(0x00008000, 0)

	h := AROSBootHarness{
		CPU:          cpu,
		Timeout:      50 * time.Millisecond,
		PollInterval: time.Millisecond,
	}

	result := h.Run(context.Background())
	if !result.Ready.Ready {
		t.Fatalf("result ready probe = %+v, want Ready=true", result.Ready)
	}
	if result.TimedOut {
		t.Fatal("ready harness run unexpectedly timed out")
	}
	if len(result.Faults) != 0 {
		t.Fatalf("ready harness faults = %d, want 0", len(result.Faults))
	}
}

func TestAROSBootHarness_StopsOnStructuredFault(t *testing.T) {
	cpu := newMoveATestCPU()
	cpu.Write16(0x00624910, 0x226B)
	cpu.Write16(0x00624912, 0x0010)
	h := AROSBootHarness{
		CPU:          cpu,
		Timeout:      100 * time.Millisecond,
		PollInterval: time.Millisecond,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cpu.lastExecPC = 0x00624910
		cpu.lastExecOpcode = 0x226B
		cpu.emitStructuredFault(M68K_VEC_ILLEGAL_INSTR, 0x00624910)
	}()

	result := h.Run(context.Background())
	if result.TimedOut {
		t.Fatal("fault harness run unexpectedly timed out")
	}
	if len(result.Faults) != 1 {
		t.Fatalf("fault manifest len = %d, want 1", len(result.Faults))
	}
	if result.Faults[0].Class != M68KFaultClassIllegalInstruction {
		t.Fatalf("fault class = %q, want %q", result.Faults[0].Class, M68KFaultClassIllegalInstruction)
	}
	if result.Faults[0].MnemonicFamily != "MOVEA" {
		t.Fatalf("fault mnemonic family = %q, want %q", result.Faults[0].MnemonicFamily, "MOVEA")
	}
	if result.Faults[0].AddressingMode != "(d16,An)" {
		t.Fatalf("fault addressing mode = %q, want %q", result.Faults[0].AddressingMode, "(d16,An)")
	}
}

func TestAROSBootHarness_TimesOutWhenNoSignalArrives(t *testing.T) {
	h := AROSBootHarness{
		CPU:          newMoveATestCPU(),
		Timeout:      20 * time.Millisecond,
		PollInterval: time.Millisecond,
	}

	result := h.Run(context.Background())
	if !result.TimedOut {
		t.Fatal("timeout harness run unexpectedly reported success")
	}
	if result.Ready.Ready {
		t.Fatal("timeout harness run unexpectedly reported ready")
	}
	if len(result.Faults) != 0 {
		t.Fatalf("timeout harness faults = %d, want 0", len(result.Faults))
	}
}

func TestKnownAROSBootRegressionInventory_CoversMoveARegression(t *testing.T) {
	cpu := newMoveATestCPU()
	cpu.Write16(0x00624910, 0x226B)
	cpu.Write16(0x00624912, 0x0010)
	signature := NormalizeM68KFaultRecord(cpu, M68KFaultRecord{
		Class:   M68KFaultClassIllegalInstruction,
		FaultPC: 0x00624910,
	}).Signature()

	got := KnownAROSBootRegressionInventory()[signature]
	if got != "TestM68K_MOVEAL_AddressDisplacementToAddressRegister" {
		t.Fatalf("inventory[%q] = %q, want %q", signature, got, "TestM68K_MOVEAL_AddressDisplacementToAddressRegister")
	}
}

func TestMissingAROSBootRegressionCoverage_ReturnsOnlyUncoveredSignatures(t *testing.T) {
	records := []M68KFaultRecord{
		{
			Class:          M68KFaultClassIllegalInstruction,
			FaultPC:        0x00624910,
			Opcode:         0x226B,
			MnemonicFamily: "MOVEA",
			AddressingMode: "(d16,An)",
		},
		{
			Class:          M68KFaultClassLineF,
			FaultPC:        0x00630000,
			Opcode:         0xF200,
			MnemonicFamily: "FPU",
			AddressingMode: "",
		},
		{
			Class:          M68KFaultClassLineF,
			FaultPC:        0x00630000,
			Opcode:         0xF200,
			MnemonicFamily: "FPU",
			AddressingMode: "",
		},
	}

	missing := MissingAROSBootRegressionCoverage(records)
	if len(missing) != 1 {
		t.Fatalf("missing len = %d, want 1 (%v)", len(missing), missing)
	}
	want := (M68KFaultRecord{
		Class:          M68KFaultClassLineF,
		FaultPC:        0x00630000,
		Opcode:         0xF200,
		MnemonicFamily: "FPU",
		AddressingMode: "",
	}).Signature()
	if missing[0] != want {
		t.Fatalf("missing[0] = %q, want %q", missing[0], want)
	}
}
