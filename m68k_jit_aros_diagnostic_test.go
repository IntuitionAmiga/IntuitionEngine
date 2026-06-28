//go:build headless && m68k_test && amd64

package main

import (
	"context"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// isolatedAROSDriveRoot returns a private, copy-on-write clone of the AROS drive
// tree for a single boot pass. The interp-vs-JIT comparison tests run the two
// passes sequentially against the host filesystem; without isolation the first
// pass mutates the shared tree (e.g. AROS deletes/recreates the Fonts/__TEST__
// writability-probe file) and the second pass observes different on-disk state,
// producing a spurious "divergence" that has nothing to do with CPU/JIT parity.
//
// btrfs/xfs reflink makes a full-tree clone near-instant with negligible disk
// use. Reflink requires the destination be on the same filesystem as the
// source, so the clone lives under build/ (same repo volume) rather than
// t.TempDir() (which is on /tmp and would be a cross-device copy). cp falls back
// to a full copy if reflink is unavailable; that is correct, just slower.
func isolatedAROSDriveRoot(t *testing.T, label string) string {
	t.Helper()
	src := requireAROSDriveRoot(t)
	base := filepath.Join("build", ".cowdrive")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create CoW drive base dir: %v", err)
	}
	holder, err := os.MkdirTemp(base, label+"-")
	if err != nil {
		t.Fatalf("create CoW drive holder dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(holder) })
	dst := filepath.Join(holder, "AROS")
	out, err := exec.Command("cp", "--reflink=auto", "-a", src, dst).CombinedOutput()
	if err != nil {
		t.Fatalf("clone AROS drive tree for isolation: %v\n%s", err, out)
	}
	if !isAROSDrivePath(dst) {
		t.Fatalf("cloned AROS drive tree is not a valid drive root: %s", dst)
	}
	return dst
}

type opcodeCount struct {
	opcode uint16
	count  uint64
}

type nativePCCount struct {
	pc      uint32
	count   uint64
	retired uint64
}

func TestM68KJIT_AROSBootFallbackDiagnostics(t *testing.T) {
	runM68KJITAROSBootDiagnostic(t, true, 8*time.Second, true)
}

func TestM68KJIT_AROSBootLongDiagnostics(t *testing.T) {
	runM68KJITAROSBootDiagnostic(t, true, 30*time.Second, true)
}

func TestM68KJIT_AROSRealIRQNoCeilingBootDiagnostic(t *testing.T) {
	if os.Getenv("IE_M68K_JIT_AROS_REAL_IRQ_NO_CEILING") != "1" {
		t.Skip("set IE_M68K_JIT_AROS_REAL_IRQ_NO_CEILING=1 to run real-IRQ no-ceiling AROS diagnostic")
	}
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, requireAROSDriveRoot(t), AROSBootEnvironmentOptions{NoNativeCeiling: true})
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	env.CPU.m68kJitPersist = true
	env.CPU.m68kJitRecordNativePCs.Store(true)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)
	env.CPU.m68kJitRecordFallbackSnapshots = true
	var (
		faultMu sync.Mutex
		faults  []M68KFaultRecord
	)
	prevFaultHook := env.CPU.FaultHook
	env.CPU.FaultHook = func(record M68KFaultRecord) {
		record = NormalizeM68KFaultRecord(env.CPU, record)
		faultMu.Lock()
		faults = append(faults, record)
		faultMu.Unlock()
		if prevFaultHook != nil {
			prevFaultHook(record)
		}
	}
	defer func() {
		env.CPU.FaultHook = prevFaultHook
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	result, err := env.BootAndWait(ctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	t.Logf("AROS real-IRQ no-ceiling boot elapsed=%s ready=%+v timedOut=%v faults=%+v pc=%08X sr=%04X instr=%d native_blocks=%d fallback=%d helper=%d mmio=%d unsupported=%d compile_failure=%d",
		elapsed, result.Ready, result.TimedOut, result.Faults,
		env.CPU.PC, env.CPU.SR, env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(),
		env.CPU.m68kJitFallbackInstructions.Load(),
		env.CPU.m68kJitNativeHelperExits.Load(),
		env.CPU.m68kJitMMIOGuardExits.Load(),
		env.CPU.m68kJitUnsupportedOneExits.Load(),
		env.CPU.m68kJitCompileFailureExits.Load())
	if os.Getenv("IE_M68K_JIT_AROS_VERBOSE") == "1" {
		logM68KJITTopNativePCs(t, "AROS real-IRQ no-ceiling", env.CPU)
		logM68KJITRecentNativePCs(t, "AROS real-IRQ no-ceiling", env.CPU)
	}
	if result.TimedOut || len(result.Faults) != 0 || !result.Ready.Ready {
		t.Fatalf("AROS real-IRQ no-ceiling boot failed: ready=%+v timedOut=%v faults=%+v", result.Ready, result.TimedOut, result.Faults)
	}

	soak := m68kJITAROSDiagnosticSoak(30 * time.Second)
	deadline := time.Now().Add(soak)
	for time.Now().Before(deadline) {
		faultMu.Lock()
		gotFaults := append([]M68KFaultRecord(nil), faults...)
		faultMu.Unlock()
		if len(gotFaults) != 0 {
			logM68KJITFaultWindows(t, "AROS real-IRQ no-ceiling fault", env.CPU, gotFaults)
			logM68KJITRecentNativePCs(t, "AROS real-IRQ no-ceiling fault", env.CPU)
			t.Fatalf("AROS real-IRQ no-ceiling hit post-ready faults: %+v", gotFaults)
		}
		if !env.CPU.Running() {
			t.Fatalf("AROS real-IRQ no-ceiling halted during post-ready soak: pc=%08X sr=%04X instr=%d",
				env.CPU.PC, env.CPU.SR, env.CPU.InstructionCount)
		}
		time.Sleep(25 * time.Millisecond)
	}
	faultMu.Lock()
	gotFaults := append([]M68KFaultRecord(nil), faults...)
	faultMu.Unlock()
	if len(gotFaults) != 0 {
		logM68KJITFaultWindows(t, "AROS real-IRQ no-ceiling fault", env.CPU, gotFaults)
		logM68KJITRecentNativePCs(t, "AROS real-IRQ no-ceiling fault", env.CPU)
		t.Fatalf("AROS real-IRQ no-ceiling hit post-ready faults: %+v", gotFaults)
	}
	readyAfterSoak := ProbeAROSReadyState(env.CPU, env.Loader)
	t.Logf("AROS real-IRQ no-ceiling post-ready soak=%s ready=%+v pc=%08X sr=%04X instr=%d native_blocks=%d fallback=%d helper=%d mmio=%d unsupported=%d compile_failure=%d",
		soak, readyAfterSoak, env.CPU.PC, env.CPU.SR, env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(),
		env.CPU.m68kJitFallbackInstructions.Load(),
		env.CPU.m68kJitNativeHelperExits.Load(),
		env.CPU.m68kJitMMIOGuardExits.Load(),
		env.CPU.m68kJitUnsupportedOneExits.Load(),
		env.CPU.m68kJitCompileFailureExits.Load())
}

func m68kJITAROSDiagnosticSoak(defaultSoak time.Duration) time.Duration {
	raw := os.Getenv("IE_M68K_JIT_AROS_SOAK_SECONDS")
	if raw == "" {
		return defaultSoak
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultSoak
	}
	return time.Duration(seconds) * time.Second
}

func TestM68KJIT_AROSDOSStatsCompareInterpreter(t *testing.T) {
	t.Run("interpreter", func(t *testing.T) {
		runM68KJITAROSBootDiagnostic(t, false, 8*time.Second, false)
	})
	t.Run("jit", func(t *testing.T) {
		runM68KJITAROSBootDiagnostic(t, true, 8*time.Second, false)
	})
}

func TestM68KJIT_AROSDOSCommandTraceMatchesInterpreter(t *testing.T) {
	target := m68kJITAROSDiagnosticTraceTarget(2048)
	interpWait := m68kJITAROSDiagnosticTraceWait("IE_M68K_JIT_AROS_INTERP_TRACE_SECONDS", 30*time.Second)
	jitWait := m68kJITAROSDiagnosticTraceWait("IE_M68K_JIT_AROS_JIT_TRACE_SECONDS", 45*time.Second)
	interp := collectAROSDOSCommandTraceUntil(t, false, target, interpWait)
	jit := collectAROSDOSCommandTraceUntil(t, true, target, jitWait)

	limit := len(interp)
	if len(jit) < limit {
		limit = len(jit)
	}
	for i := 0; i < limit; i++ {
		if os.Getenv("IE_M68K_JIT_AROS_COMPARE_INSTR_DELTAS") == "1" && i > 0 {
			interpDelta := interp[i].Instructions - interp[i-1].Instructions
			jitDelta := jit[i].Instructions - jit[i-1].Instructions
			if interpDelta != jitDelta {
				t.Fatalf("DOS command instruction delta diverged at %d/%d: interp_delta=%d jit_delta=%d interpreter=%s %s prev=%s jit=%s %s prev=%s",
					i, limit, interpDelta, jitDelta,
					formatArosDOSCommandEvent(interp[i]), formatArosDOSCommandEventRegs(interp[i]), formatArosDOSCommandEvent(interp[i-1]),
					formatArosDOSCommandEvent(jit[i]), formatArosDOSCommandEventRegs(jit[i]), formatArosDOSCommandEvent(jit[i-1]))
			}
		}
		if os.Getenv("IE_M68K_JIT_AROS_COMPARE_REGS") == "1" && !equalArosDOSCommandEventRegs(interp[i], jit[i]) {
			t.Fatalf("DOS command register state diverged at %d/%d: interpreter=%s %s jit=%s %s",
				i, limit,
				formatArosDOSCommandEvent(interp[i]), formatArosDOSCommandEventRegs(interp[i]),
				formatArosDOSCommandEvent(jit[i]), formatArosDOSCommandEventRegs(jit[i]))
		}
		if equalArosDOSCommandEventForTrace(interp[i], jit[i]) {
			continue
		}
		windowStart := i - 4
		if windowStart < 0 {
			windowStart = 0
		}
		t.Fatalf("DOS command trace diverged at %d/%d: interpreter=%s %s jit=%s %s interp_window=%s jit_window=%s interp_named=%s jit_named=%s interp_lock4d=%s jit_lock4d=%s",
			i, limit,
			formatArosDOSCommandEvent(interp[i]), formatArosDOSCommandEventRegs(interp[i]),
			formatArosDOSCommandEvent(jit[i]), formatArosDOSCommandEventRegs(jit[i]),
			formatArosDOSCommandWindow(interp, windowStart, 9),
			formatArosDOSCommandWindow(jit, windowStart, 9),
			formatArosDOSCommandNameHistory(interp, "__TEST__", i, 12),
			formatArosDOSCommandNameHistory(jit, "__TEST__", i, 12),
			formatArosDOSCommandLockHistory(interp, 0x4D, i, 12),
			formatArosDOSCommandLockHistory(jit, 0x4D, i, 12))
	}
	if len(interp) != len(jit) {
		t.Fatalf("DOS command trace length diverged after %d common events: interpreter=%d last=%s next=%s jit=%d last=%s",
			limit, len(interp), formatArosDOSCommandEvent(lastArosDOSCommandEvent(interp)),
			formatArosDOSCommandWindow(interp, limit, 8), len(jit), formatArosDOSCommandEvent(lastArosDOSCommandEvent(jit)))
	}
}

func m68kJITAROSDiagnosticTraceWait(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return def
	}
	return time.Duration(seconds) * time.Second
}

func equalArosDOSCommandEventForTrace(a, b ArosDOSCommandEvent) bool {
	return a.Cmd == b.Cmd &&
		a.Arg1 == b.Arg1 &&
		a.Arg2 == b.Arg2 &&
		a.Arg3 == b.Arg3 &&
		a.Arg4 == b.Arg4 &&
		a.Res1 == b.Res1 &&
		a.Res2 == b.Res2 &&
		a.Name == b.Name &&
		a.Task == b.Task &&
		a.TaskName == b.TaskName
}

func equalArosDOSCommandEventRegs(a, b ArosDOSCommandEvent) bool {
	return a.D == b.D && a.A == b.A
}

func TestM68KJIT_AROSStateAtDOSCommandTraceBoundary(t *testing.T) {
	boundary := 505
	if raw := os.Getenv("IE_M68K_JIT_AROS_DOS_BOUNDARY"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			boundary = parsed
		}
	}
	collectAROSStateAtDOSCommandBoundary(t, false, boundary)
	collectAROSStateAtDOSCommandBoundary(t, true, boundary)
}

func TestM68KJIT_AROSTaskStateMatchesInterpreterAtDOSCommandTraceBoundary(t *testing.T) {
	boundary := 2048
	if raw := os.Getenv("IE_M68K_JIT_AROS_DOS_BOUNDARY"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			boundary = parsed
		}
	}
	interp := collectAROSTaskStateAtDOSCommandBoundary(t, false, boundary)
	jit := collectAROSTaskStateAtDOSCommandBoundary(t, true, boundary)
	if interp.equalForScheduling(jit) {
		t.Logf("AROS task state matched at DOS boundary %d: interpreter=%s jit=%s",
			boundary, interp.shortString(), jit.shortString())
		return
	}
	t.Fatalf("AROS task state diverged at DOS boundary %d:\ninterpreter: %s\njit:         %s",
		boundary, interp.longString(), jit.longString())
}

type arosTaskStateSummary struct {
	Label                string
	TraceLen             int
	PC                   uint32
	SR                   uint16
	SP                   uint32
	LastPC               uint32
	LastOpcode           uint16
	Instructions         uint64
	NativeBlocks         uint64
	FallbackInstructions uint64
	Ready                AROSReadyState
	ReadyList            []string
	WaitList             []string
	RecentCommands       []ArosDOSCommandEvent
}

func (s arosTaskStateSummary) equalForScheduling(other arosTaskStateSummary) bool {
	return s.TraceLen == other.TraceLen &&
		s.Ready.ThisTask == other.Ready.ThisTask &&
		s.Ready.TaskName == other.Ready.TaskName &&
		strings.Join(s.ReadyList, "\x00") == strings.Join(other.ReadyList, "\x00") &&
		strings.Join(s.WaitList, "\x00") == strings.Join(other.WaitList, "\x00")
}

func (s arosTaskStateSummary) shortString() string {
	return fmt.Sprintf("trace=%d task=%q this=%08X pc=%08X sr=%04X instr=%d ready=%v wait=%v",
		s.TraceLen, s.Ready.TaskName, s.Ready.ThisTask, s.PC, s.SR, s.Instructions, s.ReadyList, s.WaitList)
}

func (s arosTaskStateSummary) longString() string {
	return fmt.Sprintf("%s native=%d fallback=%d last=%08X/%04X recent=%s",
		s.shortString(), s.NativeBlocks, s.FallbackInstructions, s.LastPC, s.LastOpcode,
		formatArosDOSCommandWindow(s.RecentCommands, 0, len(s.RecentCommands)))
}

func collectAROSTaskStateAtDOSCommandBoundary(t *testing.T, useJIT bool, boundary int) arosTaskStateSummary {
	t.Helper()
	env := newAROSDiagnosticBootEnvironment(t, useJIT)
	defer env.Close()
	startAROSDiagnosticCompositor(t, env)

	env.CPU.m68kJitPersist = useJIT
	env.CPU.m68kJitRecordNativePCs.Store(useJIT)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)
	env.CPU.m68kJitRecordFallbackSnapshots = useJIT
	reached := make(chan struct{})
	var reachedOnce sync.Once
	env.DOS.CommandRecordedHook = func(count int, _ ArosDOSCommandEvent) {
		if count >= boundary {
			reachedOnce.Do(func() {
				env.CPU.SetRunning(false)
				close(reached)
			})
		}
	}
	defer func() {
		env.DOS.CommandRecordedHook = nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || len(result.Faults) != 0 || !result.Ready.Ready {
		t.Fatalf("%s initial boot failed: ready=%+v timedOut=%v faults=%+v",
			labelForAROSDiagnostic(useJIT), result.Ready, result.TimedOut, result.Faults)
	}

	deadline := time.After(25 * time.Second)
	select {
	case <-reached:
		env.Runner.Stop()
		if useJIT {
			logM68KJITTopNativePCs(t, labelForAROSDiagnostic(useJIT)+" boundary", env.CPU)
			logM68KJITRecentNativePCs(t, labelForAROSDiagnostic(useJIT)+" boundary", env.CPU)
		}
		return snapshotAROSTaskStateSummary(labelForAROSDiagnostic(useJIT), env, len(env.DOS.FullCommandsSnapshot()))
	case <-deadline:
		got := len(env.DOS.FullCommandsSnapshot())
		env.Runner.Stop()
		t.Fatalf("%s did not reach DOS command boundary %d, got %d",
			labelForAROSDiagnostic(useJIT), boundary, got)
	}
	return arosTaskStateSummary{}
}

func snapshotAROSTaskStateSummary(label string, env *AROSBootEnvironment, traceLen int) arosTaskStateSummary {
	trace := env.DOS.FullCommandsSnapshot()
	start := len(trace) - 8
	if start < 0 {
		start = 0
	}
	recent := append([]ArosDOSCommandEvent(nil), trace[start:]...)
	return arosTaskStateSummary{
		Label:                label,
		TraceLen:             traceLen,
		PC:                   env.CPU.PC,
		SR:                   env.CPU.SR,
		SP:                   env.CPU.AddrRegs[7],
		LastPC:               env.CPU.lastExecPC,
		LastOpcode:           env.CPU.lastExecOpcode,
		Instructions:         env.CPU.InstructionCount,
		NativeBlocks:         env.CPU.m68kJitNativeBlocksExecuted.Load(),
		FallbackInstructions: env.CPU.m68kJitFallbackInstructions.Load(),
		Ready:                ProbeAROSReadyState(env.CPU, env.Loader),
		ReadyList:            snapshotAROSTaskList(env.CPU, arosExecTaskReadyOffset),
		WaitList:             snapshotAROSTaskList(env.CPU, arosExecTaskWaitOffset),
		RecentCommands:       recent,
	}
}

func snapshotAROSTaskList(cpu *M68KCPU, listOffset uint32) []string {
	if cpu == nil {
		return nil
	}
	sysBase := cpu.Read32(4)
	if !isValidAROSGuestPtr(sysBase) {
		return []string{fmt.Sprintf("invalid-sysbase:%08X", sysBase)}
	}
	listAddr := sysBase + listOffset
	node := cpu.Read32(listAddr)
	var out []string
	for i := 0; i < 64; i++ {
		if !isValidAROSGuestPtr(node) {
			out = append(out, fmt.Sprintf("invalid:%08X", node))
			return out
		}
		namePtr := cpu.Read32(node + arosTaskNameOffset)
		name := ""
		if isValidAROSGuestPtr(namePtr) {
			name = readAROSCStr(cpu, namePtr, 64)
		}
		out = append(out, fmt.Sprintf("%08X:%s", node, name))
		next := cpu.Read32(node)
		if next == 0 || next == node {
			return out
		}
		node = next
	}
	return append(out, "truncated")
}

func newAROSDiagnosticBootEnvironment(t *testing.T, useJIT bool) *AROSBootEnvironment {
	t.Helper()
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	opts := AROSBootEnvironmentOptions{DeterministicIRQs: true}
	if !useJIT {
		opts.InterpreterOnly = true
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, requireAROSDriveRoot(t), opts)
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	return env
}

func startAROSDiagnosticCompositor(t *testing.T, env *AROSBootEnvironment) {
	t.Helper()
	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	t.Cleanup(func() {
		compositor.Stop()
	})
}

func collectAROSStateAtDOSCommandBoundary(t *testing.T, useJIT bool, boundary int) {
	t.Helper()
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	driveRoot := isolatedAROSDriveRoot(t, labelForAROSDiagnostic(useJIT))
	var env *AROSBootEnvironment
	if useJIT {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{DeterministicIRQs: true})
	} else {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{InterpreterOnly: true, DeterministicIRQs: true})
	}
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()

	env.CPU.m68kJitPersist = useJIT
	env.CPU.m68kJitRecordNativePCs.Store(useJIT)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)
	env.CPU.m68kJitRecordFallbackSnapshots = useJIT
	env.CPU.DebugTraceEnabled = true

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || len(result.Faults) != 0 || !result.Ready.Ready {
		t.Fatalf("%s initial boot failed: ready=%+v timedOut=%v faults=%+v",
			labelForAROSDiagnostic(useJIT), result.Ready, result.TimedOut, result.Faults)
	}

	deadline := time.After(20 * time.Second)
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := len(env.DOS.FullCommandsSnapshot()); got >= boundary {
			env.Runner.Stop()
			logAROSBoundaryState(t, labelForAROSDiagnostic(useJIT), env, got)
			return
		}
		select {
		case <-deadline:
			got := len(env.DOS.FullCommandsSnapshot())
			env.Runner.Stop()
			t.Fatalf("%s did not reach DOS command boundary %d, got %d",
				labelForAROSDiagnostic(useJIT), boundary, got)
		case <-ticker.C:
		}
	}
}

func logAROSBoundaryState(t *testing.T, label string, env *AROSBootEnvironment, traceLen int) {
	t.Helper()
	ready := ProbeAROSReadyState(env.CPU, env.Loader)
	t.Logf("%s boundary trace_len=%d ready=%+v pc=%08X sr=%04X sp=%08X stopped=%v last_pc=%08X last_opcode=%04X instructions=%d native_blocks=%d fallback_instructions=%d",
		label, traceLen, ready, env.CPU.PC, env.CPU.SR, env.CPU.AddrRegs[7], env.CPU.stopped.Load(),
		env.CPU.lastExecPC, env.CPU.lastExecOpcode, env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load())
	t.Logf("%s boundary D0-D7=%08X %08X %08X %08X %08X %08X %08X %08X",
		label,
		env.CPU.DataRegs[0], env.CPU.DataRegs[1], env.CPU.DataRegs[2], env.CPU.DataRegs[3],
		env.CPU.DataRegs[4], env.CPU.DataRegs[5], env.CPU.DataRegs[6], env.CPU.DataRegs[7])
	t.Logf("%s boundary A0-A7=%08X %08X %08X %08X %08X %08X %08X %08X",
		label,
		env.CPU.AddrRegs[0], env.CPU.AddrRegs[1], env.CPU.AddrRegs[2], env.CPU.AddrRegs[3],
		env.CPU.AddrRegs[4], env.CPU.AddrRegs[5], env.CPU.AddrRegs[6], env.CPU.AddrRegs[7])
	logAROSTaskLists(t, label+" boundary", env.CPU)
	logArosRecentDOSCommands(t, label+" boundary", env.DOS)
	env.CPU.DumpDebugTrace(64)
}

func m68kJITAROSDiagnosticTraceTarget(defaultTarget int) int {
	raw := os.Getenv("IE_M68K_JIT_AROS_DOS_TARGET")
	if raw == "" {
		return defaultTarget
	}
	target, err := strconv.Atoi(raw)
	if err != nil || target <= 0 {
		return defaultTarget
	}
	return target
}

func collectAROSDOSCommandTrace(t *testing.T, useJIT bool, soak time.Duration) []ArosDOSCommandEvent {
	return collectAROSDOSCommandTraceUntil(t, useJIT, 0, soak)
}

func collectAROSDOSCommandTraceUntil(t *testing.T, useJIT bool, targetEvents int, wait time.Duration) []ArosDOSCommandEvent {
	t.Helper()
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	driveRoot := isolatedAROSDriveRoot(t, labelForAROSDiagnostic(useJIT))
	var env *AROSBootEnvironment
	if useJIT {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{DeterministicIRQs: true})
	} else {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{InterpreterOnly: true, DeterministicIRQs: true})
	}
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()

	env.CPU.m68kJitPersist = useJIT
	env.CPU.m68kJitRecordNativePCs.Store(false)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)
	env.CPU.m68kJitRecordFallbackSnapshots = useJIT

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || len(result.Faults) != 0 || !result.Ready.Ready {
		t.Fatalf("%s initial boot failed: ready=%+v timedOut=%v faults=%+v",
			labelForAROSDiagnostic(useJIT), result.Ready, result.TimedOut, result.Faults)
	}

	if targetEvents > 0 {
		deadline := time.After(wait)
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for len(env.DOS.FullCommandsSnapshot()) < targetEvents {
			select {
			case <-deadline:
				trace := env.DOS.FullCommandsSnapshot()
				ready := ProbeAROSReadyState(env.CPU, env.Loader)
				t.Logf("%s DOS trace timed out waiting for %d events: got=%d ready=%+v instructions=%d native_blocks=%d fallback_instructions=%d pc=%08X sr=%04X",
					labelForAROSDiagnostic(useJIT), targetEvents, len(trace), ready, env.CPU.InstructionCount,
					env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load(),
					env.CPU.PC, env.CPU.SR)
				return trace
			case <-ticker.C:
			}
		}
	} else {
		time.Sleep(wait)
	}
	ready := ProbeAROSReadyState(env.CPU, env.Loader)
	trace := env.DOS.FullCommandsSnapshot()
	if targetEvents > 0 && len(trace) > targetEvents {
		trace = trace[:targetEvents]
	}
	t.Logf("%s DOS trace events=%d ready=%+v instructions=%d native_blocks=%d fallback_instructions=%d",
		labelForAROSDiagnostic(useJIT), len(trace), ready, env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load())
	return trace
}

func lastArosDOSCommandEvent(events []ArosDOSCommandEvent) ArosDOSCommandEvent {
	if len(events) == 0 {
		return ArosDOSCommandEvent{}
	}
	return events[len(events)-1]
}

func formatArosDOSCommandEvent(event ArosDOSCommandEvent) string {
	return fmt.Sprintf("cmd=%d res=(0x%X,%d) args=(0x%X,0x%X,0x%X,0x%X) name=%q task=%08X/%q pc=%08X sr=%04X instr=%d",
		event.Cmd, event.Res1, event.Res2, event.Arg1, event.Arg2, event.Arg3, event.Arg4, event.Name,
		event.Task, event.TaskName, event.PC, event.SR, event.Instructions)
}

func formatArosDOSCommandEventRegs(event ArosDOSCommandEvent) string {
	return fmt.Sprintf("D=%08X/%08X/%08X/%08X/%08X/%08X/%08X/%08X A=%08X/%08X/%08X/%08X/%08X/%08X/%08X/%08X",
		event.D[0], event.D[1], event.D[2], event.D[3], event.D[4], event.D[5], event.D[6], event.D[7],
		event.A[0], event.A[1], event.A[2], event.A[3], event.A[4], event.A[5], event.A[6], event.A[7])
}

func formatArosDOSCommandWindow(events []ArosDOSCommandEvent, start, count int) string {
	if start >= len(events) {
		return "[]"
	}
	end := start + count
	if end > len(events) {
		end = len(events)
	}
	out := "["
	for i := start; i < end; i++ {
		if i != start {
			out += "; "
		}
		out += fmt.Sprintf("%d:%s", i, formatArosDOSCommandEvent(events[i]))
	}
	return out + "]"
}

func formatArosDOSCommandNameHistory(events []ArosDOSCommandEvent, name string, before, count int) string {
	if before > len(events) {
		before = len(events)
	}
	if before < 0 {
		before = 0
	}
	matches := make([]string, 0, count)
	for i := before - 1; i >= 0 && len(matches) < count; i-- {
		if events[i].Name != name {
			continue
		}
		matches = append(matches, fmt.Sprintf("%d:%s", i, formatArosDOSCommandEvent(events[i])))
	}
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return "[" + strings.Join(matches, "; ") + "]"
}

func formatArosDOSCommandLockHistory(events []ArosDOSCommandEvent, key uint32, before, count int) string {
	if before > len(events) {
		before = len(events)
	}
	if before < 0 {
		before = 0
	}
	matches := make([]string, 0, count)
	for i := before - 1; i >= 0 && len(matches) < count; i-- {
		if events[i].Res1 != key && events[i].Arg1 != key && events[i].Arg2 != key {
			continue
		}
		matches = append(matches, fmt.Sprintf("%d:%s", i, formatArosDOSCommandEvent(events[i])))
	}
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return "[" + strings.Join(matches, "; ") + "]"
}

func runM68KJITAROSBootDiagnostic(t *testing.T, useJIT bool, soak time.Duration, recordNativePCs bool) {
	t.Helper()
	soak = m68kJITAROSDiagnosticSoak(soak)

	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	driveRoot := isolatedAROSDriveRoot(t, labelForAROSDiagnostic(useJIT))
	var env *AROSBootEnvironment
	if useJIT {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{DeterministicIRQs: true})
	} else {
		env, err = NewAROSBootEnvironmentWithOptions(rom, driveRoot, AROSBootEnvironmentOptions{InterpreterOnly: true, DeterministicIRQs: true})
	}
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()

	env.CPU.m68kJitPersist = useJIT
	env.CPU.m68kJitRecordNativePCs.Store(useJIT && recordNativePCs)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)
	env.CPU.m68kJitRecordFallbackSnapshots = useJIT

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}

	for elapsed := time.Duration(0); elapsed < soak; {
		step := 5 * time.Second
		if remaining := soak - elapsed; remaining < step {
			step = remaining
		}
		time.Sleep(step)
		elapsed += step
		if elapsed < soak {
			ready := ProbeAROSReadyState(env.CPU, env.Loader)
			stats, _ := env.DOS.StatsSnapshot()
			t.Logf("%s progress elapsed=%s ready=%+v instructions=%d dos_command_kinds=%d native_blocks=%d region_promotions=%d static_jmp_chases=%d warmup_instructions=%d fallback_instructions=%d bailouts=%d",
				labelForAROSDiagnostic(useJIT), elapsed, ready, env.CPU.InstructionCount, len(stats),
				env.CPU.m68kJitNativeBlocksExecuted.Load(),
				env.CPU.m68kJitRegionPromotions.Load(),
				env.CPU.m68kJitStaticJMPChases.Load(),
				env.CPU.m68kJitWarmupInstructions.Load(),
				env.CPU.m68kJitFallbackInstructions.Load(),
				env.CPU.m68kJitBailoutCount.Load())
		}
	}
	readyAfterSoak := ProbeAROSReadyState(env.CPU, env.Loader)
	rawFrame := collectFrameStats(env.Video.GetFrame())
	compFrame := collectFrameStats(compositor.GetCurrentFrame())
	label := "AROS interpreter"
	if useJIT {
		label = "AROS JIT"
	}

	t.Logf("%s boot: ready=%+v timedOut=%v faults=%+v ready_after_soak=%+v raw_frame=%+v compositor_frame=%+v",
		label, result.Ready, result.TimedOut, result.Faults, readyAfterSoak, rawFrame, compFrame)
	t.Logf("%s stats: instructions=%d native_blocks=%d region_promotions=%d static_jmp_chases=%d native_ret_sum=%d native_chain_sum=%d native_no_chain_returns=%d native_helper_exits=%d native_exception_exits=%d native_inval_exits=%d mmio_guard_exits=%d unsupported_one_exits=%d compile_failure_exits=%d warmup_instructions=%d fallback_instructions=%d bailouts=%d last_pc=%08X last_opcode=%04X",
		label,
		env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(),
		env.CPU.m68kJitRegionPromotions.Load(),
		env.CPU.m68kJitStaticJMPChases.Load(),
		env.CPU.m68kJitNativeRetCountSum.Load(),
		env.CPU.m68kJitNativeChainCountSum.Load(),
		env.CPU.m68kJitNativeNoChainReturns.Load(),
		env.CPU.m68kJitNativeHelperExits.Load(),
		env.CPU.m68kJitNativeExceptionExits.Load(),
		env.CPU.m68kJitNativeInvalExits.Load(),
		env.CPU.m68kJitMMIOGuardExits.Load(),
		env.CPU.m68kJitUnsupportedOneExits.Load(),
		env.CPU.m68kJitCompileFailureExits.Load(),
		env.CPU.m68kJitWarmupInstructions.Load(),
		env.CPU.m68kJitFallbackInstructions.Load(),
		env.CPU.m68kJitBailoutCount.Load(),
		env.CPU.m68kJitLastFallbackPC.Load(),
		env.CPU.m68kJitLastFallbackOpcode.Load())
	// Phase tracking (M68K_JIT_FALLBACK_REMOVAL_PLAN.md): unsupported_one ratio.
	if total := env.CPU.InstructionCount; total > 0 {
		unsupportedOne := env.CPU.m68kJitUnsupportedOneExits.Load()
		t.Logf("%s ratios: unsupported_one=%d/%d (%.2f%%) compile_failure=%d mmio_guard=%d helper=%d native_exception=%d",
			label, unsupportedOne, total, float64(unsupportedOne)/float64(total)*100,
			env.CPU.m68kJitCompileFailureExits.Load(),
			env.CPU.m68kJitMMIOGuardExits.Load(),
			env.CPU.m68kJitNativeHelperExits.Load(),
			env.CPU.m68kJitNativeExceptionExits.Load())
	}
	intena := false
	if env.CPU.AmigaINTENA != nil {
		intena = env.CPU.AmigaINTENA.Load()
	}
	base := env.CPU.VBR
	vec2 := env.CPU.Read32(base + uint32(M68K_VEC_LEVEL2)*4)
	vec3 := env.CPU.Read32(base + uint32(M68K_VEC_LEVEL3)*4)
	vec4 := env.CPU.Read32(base + uint32(M68K_VEC_LEVEL4)*4)
	vec5 := env.CPU.Read32(base + uint32(M68K_VEC_LEVEL5)*4)
	t.Logf("%s cpu_state pc=%08X sr=%04X stopped=%v running=%v pending_interrupt=%08X pending_exception=%d amiga_intena=%v stop_spins=%d stop_watchdog=%d irq4_delivered=%d irq4_blocked=%d irq5_delivered=%d irq5_blocked=%d vectors=(l2:%08X l3:%08X l4:%08X l5:%08X) armed=(l2:%v l3:%v l4:%v l5:%v) A7=%08X D0=%08X D1=%08X A0=%08X A1=%08X A6=%08X",
		label,
		env.CPU.PC,
		env.CPU.SR,
		env.CPU.stopped.Load(),
		env.CPU.running.Load(),
		env.CPU.pendingInterrupt.Load(),
		env.CPU.pendingException.Load(),
		intena,
		env.CPU.stopSpinCount.Load(),
		env.CPU.stopWatchdogHits.Load(),
		env.CPU.irqL4Delivered.Load(),
		env.CPU.irqL4Blocked.Load(),
		env.CPU.irqL5Delivered.Load(),
		env.CPU.irqL5Blocked.Load(),
		vec2,
		vec3,
		vec4,
		vec5,
		env.Loader.l2Armed,
		env.Loader.l3Armed,
		env.Loader.l4Armed,
		env.Loader.l5Armed,
		env.CPU.AddrRegs[7],
		env.CPU.DataRegs[0],
		env.CPU.DataRegs[1],
		env.CPU.AddrRegs[0],
		env.CPU.AddrRegs[1],
		env.CPU.AddrRegs[6])
	logAROSTaskLists(t, label, env.CPU)
	logM68KMemoryWords(t, label, env.CPU, 0x008010E0, 48)
	logM68KMemoryWords(t, label, env.CPU, 0x0091D000, 96)
	logM68KMemoryWords(t, label, env.CPU, 0x0091D600, 96)
	logM68KMemoryWords(t, label, env.CPU, 0x0091D640, 96)
	logM68KMemoryWords(t, label, env.CPU, 0x0091D680, 96)

	var counts []opcodeCount
	for opcode := range env.CPU.m68kJitFallbackOpcodeCounts {
		count := env.CPU.m68kJitFallbackOpcodeCounts[opcode].Load()
		if count != 0 {
			counts = append(counts, opcodeCount{opcode: uint16(opcode), count: count})
		}
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].opcode < counts[j].opcode
		}
		return counts[i].count > counts[j].count
	})
	if len(counts) > 24 {
		counts = counts[:24]
	}
	for _, c := range counts {
		t.Logf("%s fallback opcode %04X pc=%08X count=%d",
			label, c.opcode, env.CPU.m68kJitFallbackOpcodePCs[c.opcode].Load(), c.count)
	}
	logM68KJITTopNativePCs(t, label, env.CPU)
	nativeRangeStart, nativeRangeEnd := m68kJITAROSNativeLogRange(0x0091C000, 0x0091E000)
	logM68KJITNativePCsInRange(t, label, env.CPU, nativeRangeStart, nativeRangeEnd)
	logM68KJITCompileFailures(t, label, env.CPU)
	logM68KJITRecentNativePCs(t, label, env.CPU)
	logM68KJITTopInvalidationPCs(t, label, env.CPU)
	logM68KJITFallbackSnapshots(t, label, env.CPU, counts)
	logM68KJITRejectedBlocks(t, label, env.CPU, counts)
	logArosDOSStats(t, label, env.DOS)
	logArosRecentDOSCommands(t, label, env.DOS)
	if output := env.Terminal.DrainOutput(); output != "" {
		t.Logf("%s terminal_output=%q", label, output)
	}

	if useJIT && env.CPU.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("AROS JIT executed no native blocks")
	}
	if result.TimedOut {
		t.Fatalf("%s boot timed out: ready=%+v faults=%+v", label, result.Ready, result.Faults)
	}
	if len(result.Faults) != 0 {
		t.Fatalf("%s boot hit structured faults: %+v", label, result.Faults)
	}
	if !result.Ready.Ready {
		t.Fatalf("%s boot did not reach ready state: %+v", label, result.Ready)
	}
}

func logAROSTaskLists(t *testing.T, label string, cpu *M68KCPU) {
	t.Helper()
	if cpu == nil {
		return
	}
	sysBase := cpu.Read32(4)
	if !isValidAROSGuestPtr(sysBase) {
		t.Logf("%s task_lists unavailable sysBase=%08X", label, sysBase)
		return
	}
	logAROSTaskList(t, label, cpu, "ready", sysBase+arosExecTaskReadyOffset)
	logAROSTaskList(t, label, cpu, "wait", sysBase+arosExecTaskWaitOffset)
}

func logAROSTaskList(t *testing.T, label string, cpu *M68KCPU, listName string, listAddr uint32) {
	t.Helper()
	head := cpu.Read32(listAddr)
	tail := cpu.Read32(listAddr + 4)
	tailPred := cpu.Read32(listAddr + 8)
	t.Logf("%s task_list %s list=%08X head=%08X tail=%08X tailpred=%08X",
		label, listName, listAddr, head, tail, tailPred)
	node := head
	for i := 0; i < 64; i++ {
		if !isValidAROSGuestPtr(node) {
			t.Logf("%s task_list %s[%02d] stop invalid node=%08X", label, listName, i, node)
			return
		}
		next := cpu.Read32(node)
		prev := cpu.Read32(node + 4)
		namePtr := cpu.Read32(node + arosTaskNameOffset)
		name := ""
		if isValidAROSGuestPtr(namePtr) {
			name = readAROSCStr(cpu, namePtr, 64)
		}
		t.Logf("%s task_list %s[%02d] node=%08X next=%08X prev=%08X nameptr=%08X name=%q",
			label, listName, i, node, next, prev, namePtr, name)
		if next == 0 || next == node {
			return
		}
		node = next
	}
	t.Logf("%s task_list %s truncated after 64 entries", label, listName)
}

func logM68KJITCompileFailures(t *testing.T, label string, cpu *M68KCPU) {
	t.Helper()
	if cpu == nil {
		return
	}
	cpu.m68kJitCompileFailMu.Lock()
	failures := make([]nativePCCount, 0, len(cpu.m68kJitCompileFailCounts))
	errs := make(map[uint32]string, len(cpu.m68kJitCompileFailErrors))
	for pc, count := range cpu.m68kJitCompileFailCounts {
		failures = append(failures, nativePCCount{pc: pc, count: count})
	}
	for pc, err := range cpu.m68kJitCompileFailErrors {
		errs[pc] = err
	}
	cpu.m68kJitCompileFailMu.Unlock()
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].count == failures[j].count {
			return failures[i].pc < failures[j].pc
		}
		return failures[i].count > failures[j].count
	})
	if len(failures) > 16 {
		failures = failures[:16]
	}
	for _, f := range failures {
		instrs := m68kScanBlock(cpu.memory, f.pc)
		opcode := uint16(0)
		if len(instrs) != 0 {
			opcode = instrs[0].opcode
		}
		t.Logf("%s compile_fail pc=%08X opcode=%04X count=%d instrs=%d err=%q",
			label, f.pc, opcode, f.count, len(instrs), errs[f.pc])
	}
}

func logM68KJITTopNativePCs(t *testing.T, label string, cpu *M68KCPU) {
	t.Helper()
	if cpu == nil {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	counts := make([]nativePCCount, 0, len(cpu.m68kJitNativePCCounts))
	retCounts := make(map[uint32]uint64, len(cpu.m68kJitNativePCRetCounts))
	for pc, count := range cpu.m68kJitNativePCCounts {
		counts = append(counts, nativePCCount{pc: pc, count: count, retired: cpu.m68kJitNativePCRetCounts[pc]})
	}
	for pc, count := range cpu.m68kJitNativePCRetCounts {
		retCounts[pc] = count
	}
	cpu.m68kJitNativePCMu.Unlock()
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].pc < counts[j].pc
		}
		return counts[i].count > counts[j].count
	})
	if len(counts) > 16 {
		counts = counts[:16]
	}
	readMem := func(addr uint64, size int) []byte {
		out := make([]byte, size)
		for i := range size {
			if addr+uint64(i) < uint64(len(cpu.memory)) {
				out[i] = cpu.memory[addr+uint64(i)]
			}
		}
		return out
	}
	for _, c := range counts {
		logM68KJITNativePCDetail(t, label, cpu, readMem, c.pc, c.count, retCounts[c.pc])
	}

	byRetired := append([]nativePCCount(nil), counts...)
	sort.Slice(byRetired, func(i, j int) bool {
		if byRetired[i].retired == byRetired[j].retired {
			return byRetired[i].pc < byRetired[j].pc
		}
		return byRetired[i].retired > byRetired[j].retired
	})
	if len(byRetired) > 16 {
		byRetired = byRetired[:16]
	}
	for _, c := range byRetired {
		logM68KJITNativePCDetail(t, label+" retired_rank", cpu, readMem, c.pc, c.count, c.retired)
	}

	byExcess := append([]nativePCCount(nil), counts...)
	sort.Slice(byExcess, func(i, j int) bool {
		leftInstrs := uint64(len(m68kScanBlock(cpu.memory, byExcess[i].pc)))
		rightInstrs := uint64(len(m68kScanBlock(cpu.memory, byExcess[j].pc)))
		left := int64(byExcess[i].retired) - int64(byExcess[i].count*leftInstrs)
		right := int64(byExcess[j].retired) - int64(byExcess[j].count*rightInstrs)
		if left == right {
			return byExcess[i].pc < byExcess[j].pc
		}
		return left > right
	})
	if len(byExcess) > 16 {
		byExcess = byExcess[:16]
	}
	for _, c := range byExcess {
		logM68KJITNativePCDetail(t, label+" excess_rank", cpu, readMem, c.pc, c.count, c.retired)
	}
}

func logM68KJITNativePCDetail(t *testing.T, label string, cpu *M68KCPU, readMem func(uint64, int) []byte, pc uint32, count, retired uint64) {
	t.Helper()
	instrs := m68kScanBlock(cpu.memory, pc)
	avgRetired := float64(0)
	if count != 0 {
		avgRetired = float64(retired) / float64(count)
	}
	staticRetired := count * uint64(len(instrs))
	excess := int64(retired) - int64(staticRetired)
	t.Logf("%s native_pc pc=%08X count=%d retired=%d avg_retired=%.2f static=%d excess=%d instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
		label, pc, count, retired, avgRetired, staticRetired, excess, len(instrs),
		m68kNeedsFallback(instrs),
		m68kNeedsConservativeFallback(cpu.memory, pc, instrs),
		m68kBlockProductionNativeSafe(instrs),
		m68kBlockMayUseGenericIOFallback(instrs))
	disasmLimit := len(instrs)
	if disasmLimit < 4 {
		disasmLimit = 4
	}
	for _, line := range disassembleM68K(readMem, uint64(pc), disasmLimit) {
		t.Logf("%s native_pc pc=%08X disasm %08X: %-16s %s",
			label, pc, line.Address, line.HexBytes, line.Mnemonic)
	}
}

func logM68KJITNativePCsInRange(t *testing.T, label string, cpu *M68KCPU, start, end uint32) {
	t.Helper()
	if cpu == nil || start >= end {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	counts := make([]nativePCCount, 0, len(cpu.m68kJitNativePCCounts))
	for pc, count := range cpu.m68kJitNativePCCounts {
		if pc >= start && pc < end {
			counts = append(counts, nativePCCount{pc: pc, count: count})
		}
	}
	cpu.m68kJitNativePCMu.Unlock()
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].pc < counts[j].pc
		}
		return counts[i].count > counts[j].count
	})
	if len(counts) > 32 {
		counts = counts[:32]
	}
	readMem := func(addr uint64, size int) []byte {
		out := make([]byte, size)
		for i := range size {
			if addr+uint64(i) < uint64(len(cpu.memory)) {
				out[i] = cpu.memory[addr+uint64(i)]
			}
		}
		return out
	}
	for _, c := range counts {
		instrs := m68kScanBlock(cpu.memory, c.pc)
		t.Logf("%s native_range pc=%08X count=%d instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			label, c.pc, c.count, len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(cpu.memory, c.pc, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
		disasmLimit := len(instrs)
		if disasmLimit < 4 {
			disasmLimit = 4
		}
		for _, line := range disassembleM68K(readMem, uint64(c.pc), disasmLimit) {
			t.Logf("%s native_range pc=%08X disasm %08X: %-16s %s",
				label, c.pc, line.Address, line.HexBytes, line.Mnemonic)
		}
	}
}

func logM68KJITFaultWindows(t *testing.T, label string, cpu *M68KCPU, faults []M68KFaultRecord) {
	t.Helper()
	if cpu == nil || len(faults) == 0 {
		return
	}
	readMem := func(addr uint64, size int) []byte {
		out := make([]byte, size)
		for i := range size {
			if addr+uint64(i) < uint64(len(cpu.memory)) {
				out[i] = cpu.memory[addr+uint64(i)]
			}
		}
		return out
	}
	for _, fault := range faults {
		pc := fault.FaultPC
		if pc == 0 {
			pc = fault.PC
		}
		start := uint32(0)
		if pc >= 32 {
			start = pc - 32
		}
		t.Logf("%s window fault_pc=%08X exception_pc=%08X opcode=%04X", label, fault.FaultPC, fault.PC, fault.Opcode)
		for _, line := range disassembleM68K(readMem, uint64(start), 24) {
			t.Logf("%s window %08X: %-16s %s", label, line.Address, line.HexBytes, line.Mnemonic)
		}
	}
}

func m68kJITAROSNativeLogRange(defaultStart, defaultEnd uint32) (uint32, uint32) {
	raw := os.Getenv("IE_M68K_JIT_AROS_NATIVE_LOG_RANGE")
	if raw == "" {
		return defaultStart, defaultEnd
	}
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return defaultStart, defaultEnd
	}
	start, startErr := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
	end, endErr := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
	if startErr != nil || endErr != nil || start >= end {
		return defaultStart, defaultEnd
	}
	return uint32(start), uint32(end)
}

func logM68KJITRecentNativePCs(t *testing.T, label string, cpu *M68KCPU) {
	t.Helper()
	if cpu == nil {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	idx := cpu.m68kJitNativePCRingIdx
	ring := cpu.m68kJitNativePCRing
	cpu.m68kJitNativePCMu.Unlock()
	if idx == 0 {
		return
	}
	start := uint32(0)
	if idx > uint32(len(ring)) {
		start = idx - uint32(len(ring))
	}
	for i := start; i < idx; i++ {
		pc := ring[i%uint32(len(ring))]
		if pc == 0 {
			continue
		}
		t.Logf("%s recent_native[%02d]=%08X", label, i-start, pc)
		instrs := m68kScanBlock(cpu.memory, pc)
		limit := len(instrs)
		if limit > 4 {
			limit = 4
		}
		readMem := func(addr uint64, size int) []byte {
			out := make([]byte, size)
			for j := range size {
				if addr+uint64(j) < uint64(len(cpu.memory)) {
					out[j] = cpu.memory[addr+uint64(j)]
				}
			}
			return out
		}
		for _, line := range disassembleM68K(readMem, uint64(pc), limit) {
			t.Logf("%s recent_native[%02d] disasm %08X: %-16s %s",
				label, i-start, line.Address, line.HexBytes, line.Mnemonic)
		}
	}
}

func logM68KJITTopInvalidationPCs(t *testing.T, label string, cpu *M68KCPU) {
	t.Helper()
	if cpu == nil {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	counts := make([]nativePCCount, 0, len(cpu.m68kJitNativeInvalPCCounts))
	for pc, count := range cpu.m68kJitNativeInvalPCCounts {
		counts = append(counts, nativePCCount{pc: pc, count: count})
	}
	cpu.m68kJitNativePCMu.Unlock()
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].pc < counts[j].pc
		}
		return counts[i].count > counts[j].count
	})
	if len(counts) > 16 {
		counts = counts[:16]
	}
	readMem := func(addr uint64, size int) []byte {
		out := make([]byte, size)
		for i := range size {
			if addr+uint64(i) < uint64(len(cpu.memory)) {
				out[i] = cpu.memory[addr+uint64(i)]
			}
		}
		return out
	}
	for _, c := range counts {
		instrs := m68kScanBlock(cpu.memory, c.pc)
		t.Logf("%s inval_pc pc=%08X count=%d instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			label, c.pc, c.count, len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(cpu.memory, c.pc, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
		disasmLimit := len(instrs)
		if disasmLimit < 4 {
			disasmLimit = 4
		}
		for _, line := range disassembleM68K(readMem, uint64(c.pc), disasmLimit) {
			t.Logf("%s inval_pc pc=%08X disasm %08X: %-16s %s",
				label, c.pc, line.Address, line.HexBytes, line.Mnemonic)
		}
	}
}

func logM68KJITFallbackSnapshots(t *testing.T, label string, cpu *M68KCPU, counts []opcodeCount) {
	t.Helper()
	if cpu == nil {
		return
	}
	limit := len(counts)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		pc := uint32(cpu.m68kJitFallbackOpcodePCs[counts[i].opcode].Load())
		snap, ok := m68kJITFallbackSnapshotForPC(cpu, pc)
		if !ok {
			continue
		}
		ea, reason := m68kDescribeJITFallbackSnapshot(cpu, snap)
		stackPage, stackMin, stackMax, stackMarked := m68kJITDiagnosticPageBounds(cpu, snap.addr[7])
		ioPage, ioMarked, ioInBounds := m68kJITDiagnosticIOPage(cpu, snap.addr[7])
		t.Logf("%s fallback_snapshot opcode=%04X pc=%08X count=%d sr=%04X ea=%s reason=%s stack_page=%X stack_marked=%v stack_code_range=%03X-%03X stack_io_page=%X stack_io_in_bounds=%v stack_io_marked=%v A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X",
			label, snap.opcode, snap.pc, counts[i].count, snap.sr, ea, reason, stackPage, stackMarked, stackMin, stackMax, ioPage, ioInBounds, ioMarked,
			snap.addr[0], snap.addr[1], snap.addr[2], snap.addr[3], snap.addr[4], snap.addr[5], snap.addr[6], snap.addr[7],
			snap.data[0], snap.data[1], snap.data[2], snap.data[3])
	}
}

func m68kJITDiagnosticPageBounds(cpu *M68KCPU, addr uint32) (uint32, uint16, uint16, bool) {
	if cpu == nil {
		return 0, 0, 0, false
	}
	page := addr >> 12
	if int(page) >= len(cpu.m68kJitCodePageMin) || int(page) >= len(cpu.m68kJitCodePageMax) {
		return page, 0, 0, false
	}
	min := cpu.m68kJitCodePageMin[page]
	max := cpu.m68kJitCodePageMax[page]
	return page, min, max, min != 0xFFFF && max != 0
}

func m68kJITDiagnosticIOPage(cpu *M68KCPU, addr uint32) (uint32, bool, bool) {
	if cpu == nil {
		return 0, false, false
	}
	bus, ok := cpu.bus.(*MachineBus)
	if !ok {
		return addr >> 8, false, false
	}
	page := addr >> 8
	if int(page) >= len(bus.ioPageBitmap) {
		return page, false, false
	}
	return page, bus.ioPageBitmap[page], true
}

func m68kJITFallbackSnapshotForPC(cpu *M68KCPU, pc uint32) (m68kJITFallbackSnapshot, bool) {
	cpu.m68kJitFallbackSnapshotMu.Lock()
	defer cpu.m68kJitFallbackSnapshotMu.Unlock()
	snap, ok := cpu.m68kJitFallbackFirstSnapshots[pc]
	return snap, ok
}

func m68kDescribeJITFallbackSnapshot(cpu *M68KCPU, snap m68kJITFallbackSnapshot) (string, string) {
	opcode := snap.opcode
	switch {
	case opcode&0xFFF0 == 0x4E40:
		return "-", "TRAP executes in interpreter"
	case opcode>>12 == 0xF:
		return "-", "Line-F/FPU executes in interpreter"
	case opcode == 0x4E73:
		return "-", "RTE runtime guard"
	case opcode == 0x4E75:
		return "-", "RTS runtime guard/cache miss"
	case opcode&0xFFC0 == 0x40C0 && snap.sr&M68K_SR_S == 0:
		return "-", "MOVE SR from user mode privilege fallback"
	}

	if opcode>>12 == 0x5 && opcode&0x00C0 != 0x00C0 {
		size := int((opcode >> 6) & 3)
		if size == 3 {
			return "-", "ADDQ/SUBQ invalid size"
		}
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		ea, ok := m68kFallbackSnapshotEA(cpu, snap, mode, reg, size)
		if !ok {
			return "-", fmt.Sprintf("ADDQ/SUBQ unclassified EA mode=%d reg=%d", mode, reg)
		}
		accessBytes := m68kAccessSizeBytes(size)
		return fmt.Sprintf("%08X+%d", ea, accessBytes), m68kClassifyJITMemoryFallback(cpu, ea, accessBytes)
	}

	if opcode>>12 >= 0x1 && opcode>>12 <= 0x3 && m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
		size := M68K_SIZE_LONG
		switch opcode >> 12 {
		case 0x1:
			size = M68K_SIZE_BYTE
		case 0x3:
			size = M68K_SIZE_WORD
		}
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		srcEA, srcOK := m68kFallbackSnapshotEAAt(cpu, snap, srcMode, srcReg, size, snap.pc+2)
		dstExtPC := snap.pc + 2
		if srcOK {
			dstExtPC += uint32(m68kEAExtBytes(srcMode, srcReg, size, cpu.memory, snap.pc+2))
		}
		dstEA, dstOK := m68kFallbackSnapshotEAAt(cpu, snap, dstMode, dstReg, size, dstExtPC)
		if !srcOK || !dstOK {
			return "-", fmt.Sprintf("MOVE mem-to-mem unclassified srcMode=%d srcReg=%d dstMode=%d dstReg=%d", srcMode, srcReg, dstMode, dstReg)
		}
		accessBytes := m68kAccessSizeBytes(size)
		return fmt.Sprintf("src:%08X+%d dst:%08X+%d", srcEA, accessBytes, dstEA, accessBytes),
			fmt.Sprintf("src=%s dst=%s",
				m68kClassifyJITMemoryFallback(cpu, srcEA, accessBytes),
				m68kClassifyJITMemoryFallback(cpu, dstEA, accessBytes))
	}

	if opcode&0xFB80 == 0x4880 && m68kIsNativeSupportedMOVEM(opcode) {
		mask, ok := m68kSnapshotRead16(cpu, snap.pc+2)
		if !ok {
			return "-", "MOVEM missing register-mask extension"
		}
		regCount := bits.OnesCount16(mask)
		if regCount == 0 {
			return "-", "MOVEM empty register mask"
		}
		size := M68K_SIZE_WORD
		if (opcode>>6)&1 == 1 {
			size = M68K_SIZE_LONG
		}
		step := m68kAccessSizeBytes(size)
		totalBytes := uint32(regCount) * step
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		extPC := snap.pc + 4
		ea, ok := m68kFallbackSnapshotEAAt(cpu, snap, mode, reg, size, extPC)
		if !ok {
			return "-", fmt.Sprintf("MOVEM unclassified EA mode=%d reg=%d", mode, reg)
		}
		if mode == 4 && (opcode>>10)&1 == 0 {
			ea = snap.addr[reg] - totalBytes
		}
		return fmt.Sprintf("%08X+%d", ea, totalBytes), m68kClassifyJITMemoryFallback(cpu, ea, totalBytes)
	}

	return "-", "unclassified runtime fallback"
}

func m68kFallbackSnapshotEA(cpu *M68KCPU, snap m68kJITFallbackSnapshot, mode, reg uint16, size int) (uint32, bool) {
	return m68kFallbackSnapshotEAAt(cpu, snap, mode, reg, size, snap.pc+2)
}

func m68kSnapshotRead16(cpu *M68KCPU, addr uint32) (uint16, bool) {
	if uint64(addr)+2 > uint64(len(cpu.memory)) {
		return 0, false
	}
	return uint16(cpu.memory[addr])<<8 | uint16(cpu.memory[addr+1]), true
}

func m68kFallbackSnapshotEAAt(cpu *M68KCPU, snap m68kJITFallbackSnapshot, mode, reg uint16, size int, extPC uint32) (uint32, bool) {
	read16 := func(addr uint32) (uint16, bool) {
		if uint64(addr)+2 > uint64(len(cpu.memory)) {
			return 0, false
		}
		return uint16(cpu.memory[addr])<<8 | uint16(cpu.memory[addr+1]), true
	}
	read32 := func(addr uint32) (uint32, bool) {
		hi, ok := read16(addr)
		if !ok {
			return 0, false
		}
		lo, ok := read16(addr + 2)
		if !ok {
			return 0, false
		}
		return uint32(hi)<<16 | uint32(lo), true
	}

	switch mode {
	case 2, 3:
		return snap.addr[reg], true
	case 4:
		return snap.addr[reg] - m68kStepSize(size, reg), true
	case 5:
		disp, ok := read16(extPC)
		if !ok {
			return 0, false
		}
		return snap.addr[reg] + uint32(int32(int16(disp))), true
	case 6:
		ext, ok := read16(extPC)
		if !ok || ext&0x0100 != 0 {
			return 0, false
		}
		idxReg := (ext >> 12) & 7
		idx := snap.data[idxReg]
		if ext&0x8000 != 0 {
			idx = snap.addr[idxReg]
		}
		if ext&0x0800 == 0 {
			idx = uint32(int32(int16(idx)))
		}
		idx *= 1 << ((ext >> 9) & 3)
		return snap.addr[reg] + idx + uint32(int32(int8(ext&0xFF))), true
	case 7:
		switch reg {
		case 0:
			abs, ok := read16(extPC)
			if !ok {
				return 0, false
			}
			return uint32(int32(int16(abs))), true
		case 1:
			return read32(extPC)
		}
	}
	return 0, false
}

func m68kClassifyJITMemoryFallback(cpu *M68KCPU, addr, size uint32) string {
	end := uint64(addr) + uint64(size)
	if end < uint64(addr) {
		return "address wrap"
	}
	if end > uint64(len(cpu.memory)) {
		return fmt.Sprintf("range outside direct RAM len=%08X", len(cpu.memory))
	}
	if bus, ok := cpu.bus.(*MachineBus); ok {
		firstPage := addr >> 8
		lastPage := uint32((end - 1) >> 8)
		for page := firstPage; page <= lastPage; page++ {
			if page >= uint32(len(bus.ioPageBitmap)) {
				return fmt.Sprintf("I/O bitmap OOB page=%X len=%X", page, len(bus.ioPageBitmap))
			}
			if bus.ioPageBitmap[page] {
				return fmt.Sprintf("I/O page=%X", page)
			}
		}
	}
	if len(cpu.m68kJitCodeBitmap) != 0 {
		firstCodePage := addr >> 12
		lastCodePage := uint32((end - 1) >> 12)
		for page := firstCodePage; page <= lastCodePage; page++ {
			if page < uint32(len(cpu.m68kJitCodeBitmap)) && cpu.m68kJitCodeBitmap[page] != 0 {
				return fmt.Sprintf("SMC/code page=%X", page)
			}
		}
	}
	return "direct RAM; inspect emitter-specific guard"
}

func logM68KJITRejectedBlocks(t *testing.T, label string, cpu *M68KCPU, counts []opcodeCount) {
	t.Helper()
	if cpu == nil {
		return
	}
	limit := len(counts)
	if limit > 8 {
		limit = 8
	}
	readMem := func(addr uint64, size int) []byte {
		out := make([]byte, size)
		for i := range size {
			if addr+uint64(i) < uint64(len(cpu.memory)) {
				out[i] = cpu.memory[addr+uint64(i)]
			}
		}
		return out
	}
	for i := 0; i < limit; i++ {
		pc := uint32(cpu.m68kJitFallbackOpcodePCs[counts[i].opcode].Load())
		instrs := m68kScanBlock(cpu.memory, pc)
		arosPostincFillLoop := false
		if _, ok := m68kIsAROSLongPostincFillLoopBlock(instrs, pc, cpu.memory); ok {
			arosPostincFillLoop = true
		}
		arosIndexedLookupPrefix := false
		if _, ok := m68kIsAROSIndexedLookupPrefix(instrs, pc, cpu.memory); ok {
			arosIndexedLookupPrefix = true
		}
		_, _, _, _, _, memCopyLoop := m68kIsMemCopyLoopBlock(instrs, pc, cpu.memory)
		_, moveA7PostincRTSBlock := m68kIsMoveA7PostincRTSBlock(instrs)
		t.Logf("%s rejected_block opcode=%04X pc=%08X count=%d instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v arosPostincFill=%v arosCMPIJSR=%v arosStackLoadJSR=%v arosStandaloneJSR=%v standaloneRTS=%v movemPostincRTS=%v arosMOVEMPrologueJSR=%v stackLoadAbsJSR=%v arosStackCallWrapper=%v arosAddStoreJSR=%v arosIndexedLookup=%v subqBCCMoveRTS=%v subqLSRBNEStoreBRA=%v subqSubCmpBLSAddStore=%v bneMoveMOVEMRTS=%v moveA7PostincRTS=%v memCopyLoop=%v",
			label, counts[i].opcode, pc, counts[i].count, len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(cpu.memory, pc, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs),
			arosPostincFillLoop,
			m68kIsAROSCMPIJSRBlock(instrs, pc, cpu.memory),
			m68kIsAROSStackLoadJSRBlock(instrs, pc, cpu.memory),
			m68kIsAROSStandaloneJSRBlock(instrs, pc, cpu.memory),
			m68kIsStandaloneRTSBlock(instrs),
			m68kIsMOVEMPostincRTSBlock(instrs),
			m68kIsAROSMOVEMPrologueJSRBlock(instrs, pc, cpu.memory),
			m68kIsStackLoadAbsJSRBlock(instrs, pc, cpu.memory),
			m68kIsAROSStackCallWrapperBlock(instrs, pc, cpu.memory),
			m68kIsAROSAddStoreJSRBlock(instrs, pc, cpu.memory),
			arosIndexedLookupPrefix,
			m68kIsSubqBCCMoveRTSBlock(instrs, pc),
			m68kIsSubqLSRBNEStoreBRABlock(instrs, pc),
			m68kIsSubqSubCmpBLSAddStoreBlock(instrs, pc, cpu.memory),
			m68kIsBNEMoveMOVEMRTSBlock(instrs, pc),
			moveA7PostincRTSBlock,
			memCopyLoop)
		rawWordLimit := uint32(12)
		if len(instrs) > 0 {
			last := instrs[len(instrs)-1]
			if words := (last.pcOffset + uint32(last.length) + 1) / 2; words > rawWordLimit {
				rawWordLimit = words
			}
		}
		if rawWordLimit > 64 {
			rawWordLimit = 64
		}
		rawWords := ""
		for word := uint32(0); word < rawWordLimit && pc+word*2+1 < uint32(len(cpu.memory)); word++ {
			raw := uint16(cpu.memory[pc+word*2])<<8 | uint16(cpu.memory[pc+word*2+1])
			if word != 0 {
				rawWords += " "
			}
			rawWords += fmt.Sprintf("%04X", raw)
		}
		t.Logf("%s rejected_block pc=%08X raw_words %s", label, pc, rawWords)
		disasmLimit := len(instrs)
		if disasmLimit < 12 {
			disasmLimit = 12
		}
		lines := disassembleM68K(readMem, uint64(pc), disasmLimit)
		for _, line := range lines {
			t.Logf("%s rejected_block pc=%08X disasm %08X: %-16s %s",
				label, pc, line.Address, line.HexBytes, line.Mnemonic)
		}
	}
}

func logM68KMemoryWords(t *testing.T, label string, cpu *M68KCPU, addr uint32, words int) {
	t.Helper()
	if cpu == nil || words <= 0 {
		return
	}
	const wordsPerLine = 8
	for off := 0; off < words; off += wordsPerLine {
		lineWords := wordsPerLine
		if remaining := words - off; remaining < lineWords {
			lineWords = remaining
		}
		rawWords := ""
		for i := 0; i < lineWords; i++ {
			wordAddr := addr + uint32((off+i)*2)
			if wordAddr+1 >= uint32(len(cpu.memory)) {
				break
			}
			if rawWords != "" {
				rawWords += " "
			}
			rawWords += fmt.Sprintf("%04X", uint16(cpu.memory[wordAddr])<<8|uint16(cpu.memory[wordAddr+1]))
		}
		t.Logf("%s memory_words addr=%08X %s", label, addr+uint32(off*2), rawWords)
	}
}

func labelForAROSDiagnostic(useJIT bool) string {
	if useJIT {
		return "AROS JIT"
	}
	return "AROS interpreter"
}

func logArosDOSStats(t *testing.T, label string, dos *ArosDOSDevice) {
	t.Helper()
	if dos == nil {
		t.Logf("%s DOS stats: device not mapped", label)
		return
	}
	stats, errs := dos.StatsSnapshot()
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Failures == stats[j].Failures {
			if stats[i].Count == stats[j].Count {
				return stats[i].Cmd < stats[j].Cmd
			}
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Failures > stats[j].Failures
	})
	for _, stat := range stats {
		t.Logf("%s DOS cmd=%d count=%d failures=%d last_res=(0x%X,%d) last_args=(0x%X,0x%X,0x%X,0x%X) last_name=%q",
			label, stat.Cmd, stat.Count, stat.Failures, stat.LastRes1, stat.LastRes2,
			stat.LastArg1, stat.LastArg2, stat.LastArg3, stat.LastArg4, stat.LastName)
	}
	for _, event := range errs {
		t.Logf("%s DOS recent_error cmd=%d res=(0x%X,%d) args=(0x%X,0x%X,0x%X,0x%X) name=%q",
			label, event.Cmd, event.Res1, event.Res2, event.Arg1, event.Arg2, event.Arg3, event.Arg4, event.Name)
	}
}

func logArosRecentDOSCommands(t *testing.T, label string, dos *ArosDOSDevice) {
	t.Helper()
	if dos == nil {
		return
	}
	events := dos.RecentCommandsSnapshot()
	if len(events) > 32 {
		events = events[len(events)-32:]
	}
	for i, event := range events {
		t.Logf("%s DOS recent_cmd[%02d] cmd=%d res=(0x%X,%d) args=(0x%X,0x%X,0x%X,0x%X) name=%q",
			label, i, event.Cmd, event.Res1, event.Res2,
			event.Arg1, event.Arg2, event.Arg3, event.Arg4, event.Name)
	}
}

// TestM68KJIT_AROSCrash0099465EDiag captures the interpreter's (correct)
// register state at the first execution of the zero-fill loop at PC 0x0099465E
// and disassembles the loop plus its setup, so the JIT-only crash there
// (MOVE.L D1,(A2)+ with A2 walking off to 0x05DFFFFE because A0 is garbage) can
// be localized. Interp is the oracle and does not crash here; the JIT does.
// Gate: IE_DIAG_0099465E=1.
func TestM68KJIT_AROSCrash0099465EDiag(t *testing.T) {
	if os.Getenv("IE_DIAG_0099465E") != "1" {
		t.Skip("set IE_DIAG_0099465E=1 to run the 0x0099465E crash localization diagnostic")
	}
	const loopPC = uint32(0x0099465E)
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	useJIT := os.Getenv("IE_DIAG_0099465E_JIT") == "1"
	opts := AROSBootEnvironmentOptions{DeterministicIRQs: true, NoNativeCeiling: true}
	if !useJIT {
		opts.InterpreterOnly = true
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, isolatedAROSDriveRoot(t, labelForAROSDiagnostic(useJIT)), opts)
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()
	env.CPU.m68kJitPersist = useJIT
	if useJIT && os.Getenv("IE_M68K_JIT_DIAG_A0WATCH") == "1" {
		env.CPU.m68kJitRecordNativePCs.Store(true)
	}

	readMem := func(addr uint64, size int) []byte {
		if addr+uint64(size) > uint64(len(env.CPU.memory)) {
			return nil
		}
		return env.CPU.memory[addr : addr+uint64(size)]
	}
	dumpRegion := func(label string, start uint64, count int) {
		for _, line := range disassembleM68K(readMem, start, count) {
			t.Logf("  %s %08X: %-18s %s", label, line.Address, line.HexBytes, line.Mnemonic)
		}
	}

	var captured bool
	if !useJIT {
		// Interpreter path: per-instruction hook fires; grab the first loop entry.
		env.CPU.InstructionHook = func(cpu *M68KCPU) {
			// Capture the invocation in the crash window (JIT faults at instr
			// ~198.8M / DOS event ~10525), not an earlier benign memset call.
			if captured || cpu.PC != loopPC || cpu.InstructionCount < 194000000 {
				return
			}
			captured = true
			t.Logf("INTERP loop-entry @%08X instr=%d", loopPC, cpu.InstructionCount)
			t.Logf("  D0-7=%08X %08X %08X %08X %08X %08X %08X %08X",
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7])
			t.Logf("  A0-7=%08X %08X %08X %08X %08X %08X %08X %08X",
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7])
			dumpRegion("rtn  ", 0x0099460C, 72)
		}
	} else {
		env.CPU.FaultHook = func(rec M68KFaultRecord) {
			if captured {
				return
			}
			captured = true
			t.Logf("JIT fault @%08X addr=%08X", rec.PC, rec.AccessAddr)
			dumpRegion("rtn  ", 0x0099460C, 72)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := env.BootAndWait(ctx); err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	deadline := time.Now().Add(120 * time.Second)
	for !captured && time.Now().Before(deadline) {
		if len(env.DOS.FullCommandsSnapshot()) > 10560 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !captured {
		t.Logf("did not capture loop entry/fault at %08X (events=%d instr=%d pc=%08X)",
			loopPC, len(env.DOS.FullCommandsSnapshot()), env.CPU.InstructionCount, env.CPU.PC)
	}
}

// TestM68KJIT_AROSCrash0099465EWatchSlot watches the stack slot the crashing AROS
// memset reads its length from (28(A7) ≈ 0x00999D40 from the crash registers) and
// records every write to it with the writer PC. IE_M68K_JIT_WATCH_WRITE_ADDR makes
// native stores that overlap the slot bail to the interpreter, whose Write path
// fires DebugWatchFn — so this catches the upstream writer that puts the garbage
// length into the slot, even when that store is JIT-native.
// Gate: IE_DIAG_0099465E_WATCH=1 (optionally IE_DIAG_WATCH_SLOT=0x........ to override).
func TestM68KJIT_AROSCrash0099465EWatchSlot(t *testing.T) {
	if os.Getenv("IE_DIAG_0099465E_WATCH") != "1" {
		t.Skip("set IE_DIAG_0099465E_WATCH=1 to run the 0x0099465E stack-slot write watch")
	}
	// Run with the caller forced to the interpreter so its argument-push stores
	// flow through DebugWatchFn:
	//   IE_M68K_JIT_EXCLUDE_LO=0x00994E00 IE_M68K_JIT_EXCLUDE_HI=0x00994F00
	// (these are read at package init, so set them on the command line). The
	// crash still reproduces with the caller interpreted.
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, isolatedAROSDriveRoot(t, "watchslot"),
		AROSBootEnvironmentOptions{DeterministicIRQs: true, NoNativeCeiling: true})
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()
	env.CPU.m68kJitPersist = true

	type memWrite struct {
		addr  uint32
		value uint32
		pc    uint32
		instr uint64
	}
	// Watch a wide window covering the crashing task's stack so the actual slot
	// (28(A7) at the fault) is captured regardless of run-to-run stack address.
	const (
		watchLo = uint32(0x00999000)
		watchHi = uint32(0x0099A000)
	)
	var (
		mu      sync.Mutex
		writes  []memWrite
		faulted bool
	)
	env.CPU.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr < watchLo || addr >= watchHi {
			return
		}
		mu.Lock()
		writes = append(writes, memWrite{addr: addr, value: value, pc: pc, instr: env.CPU.InstructionCount})
		if len(writes) > 8192 {
			writes = writes[len(writes)-8192:]
		}
		mu.Unlock()
	}
	prevFault := env.CPU.FaultHook
	env.CPU.FaultHook = func(rec M68KFaultRecord) {
		mu.Lock()
		if !faulted {
			faulted = true
			a7 := env.CPU.AddrRegs[7]
			slot := a7 + 28
			t.Logf("FAULT @%08X access=%08X A7=%08X → length slot 28(A7)=%08X (value now=%08X)",
				rec.PC, rec.AccessAddr, a7, slot, env.CPU.Read32(slot))
			t.Logf("writes within +/-12 of the slot, most recent last:")
			n := 0
			for i := len(writes) - 1; i >= 0 && n < 24; i-- {
				w := writes[i]
				if w.addr+4 > slot-12 && w.addr < slot+12 {
					t.Logf("  [%08X] = %08X by PC=%08X @instr=%d", w.addr, w.value, w.pc, w.instr)
					n++
				}
			}
			if n == 0 {
				t.Logf("  (no recorded writes near the slot — last 16 stack writes overall:)")
				start := 0
				if len(writes) > 16 {
					start = len(writes) - 16
				}
				for i := start; i < len(writes); i++ {
					w := writes[i]
					t.Logf("  [%08X] = %08X by PC=%08X @instr=%d", w.addr, w.value, w.pc, w.instr)
				}
			}
		}
		mu.Unlock()
		if prevFault != nil {
			prevFault(rec)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := env.BootAndWait(ctx); err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := faulted
		mu.Unlock()
		if done || len(env.DOS.FullCommandsSnapshot()) > 10600 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if !faulted {
		t.Logf("no fault reproduced; stack writes recorded=%d", len(writes))
	}
}
