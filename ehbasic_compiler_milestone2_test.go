package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Milestone 2 of IE64_BASIC_COMPILER_REPLACEMENT_PLAN.md: optimise measured
// hot paths until the shipped demo performance gates pass.
//
// Gates:
//   - voodoo_mega_demo_basic.bas sustains at least 60 uncapped completed
//     frames per second under RUN AOT (one warm-up, three measured samples,
//     median comparison on the same machine and configuration).
//   - No other shipped example regresses by more than 5 per cent.
//
// Wall-clock gates are opt-in so the functional suite stays portable:
// IE_BASIC_RUN_PERF_TESTS=1 enables them.

func milestone2PerfTestsEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("IE_BASIC_RUN_PERF_TESTS") != "1" {
		t.Skip("wall-clock Milestone 2 performance gates are opt-in via IE_BASIC_RUN_PERF_TESTS=1")
	}
}

// startMegaDemoRunAOT boots the REPL with Voodoo and SID mapped, loads the
// shipped mega demo and starts RUN AOT. It returns once frames are advancing.
func startMegaDemoRunAOT(t *testing.T) (*ehbasicTestHarness, *VoodooEngine) {
	t.Helper()
	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	v := mapVoodooForMegaDemoBasicTest(t, h.bus)
	soundChip := newTestSoundChip()
	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sidPlayer := NewSIDPlayer(sidEngine)
	sidPlayer.AttachBus(h.bus)
	h.bus.MapIO(SID_PLAY_PTR, SID_PLAY_STATUS+3, sidPlayer.HandlePlayRead, sidPlayer.HandlePlayWrite)

	if out := h.runCommand(`LOAD "sdk/examples/basic/voodoo_mega_demo_basic.bas"`); strings.Contains(out, "ERROR") {
		t.Fatalf("LOAD failed: %q", out)
	}
	h.sendInput("RUN AOT\n")
	h.pumpUntil(func() bool {
		return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= 3
	}, 120*time.Second)
	out := h.readOutput()
	if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("RUN AOT failed: %q", out)
	}
	if readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] < 3 {
		t.Fatalf("mega demo did not start rendering; pc=%#x\n%s", h.cpu.PC, readAOTStateDebug(h))
	}
	return h, v
}

func measureMegaDemoMedianFPS(t *testing.T, h *ehbasicTestHarness, measuredFrames uint64, samples int) float64 {
	t.Helper()
	durations := make([]time.Duration, samples)
	for i := range durations {
		start := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
		target := start + measuredFrames
		began := time.Now()
		h.pumpUntil(func() bool {
			return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= target
		}, 60*time.Second)
		durations[i] = time.Since(began)
		if got := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]; got < target {
			t.Fatalf("sample %d advanced from frame %d to %d, want at least %d", i+1, start, got, target)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	median := durations[len(durations)/2]
	fps := float64(measuredFrames) / median.Seconds()
	t.Logf("mega demo RUN AOT median throughput %.2f fps; samples=%v", fps, durations)
	return fps
}

// TestIE64BasicCompilerMilestone2MegaDemoPerformanceGate is the primary
// Milestone 2 acceptance gate: the shipped mega demo must sustain at least 60
// uncapped completed frames per second when compiled.
func TestIE64BasicCompilerMilestone2MegaDemoPerformanceGate(t *testing.T) {
	milestone2PerfTestsEnabled(t)
	h, _ := startMegaDemoRunAOT(t)
	// Warm-up: one measured window discarded before the three samples.
	warmStart := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
	h.pumpUntil(func() bool {
		return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= warmStart+30
	}, 60*time.Second)
	fps := measureMegaDemoMedianFPS(t, h, 30, 3)
	if fps < 60 {
		t.Fatalf("Milestone 2 gate: mega demo RUN AOT median %.2f fps is below the required 60 fps", fps)
	}
}

// compileM2Programme compiles BASIC source lines through the full standalone
// pipeline (tokenise, parse, optimise, lower, emit) and returns the generated
// assembly together with the host-assembled flat image.
func compileM2Programme(t *testing.T, lines ...string) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	basPath := filepath.Join(dir, "m2.bas")
	if err := os.WriteFile(basPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := runCompilerUnitCycles(t, compilerCorpusBody(t, basPath), 200_000_000)
	if status, line, offset := h.bus.Read64(0x1E0000), h.bus.Read64(0x1E0008), h.bus.Read64(0x1E0010); status != 0 {
		t.Fatalf("compiler diagnostic = status %d, line %d, offset %d", status, line, offset)
	}
	if status, length := h.bus.Read64(0x1E0020), h.bus.Read64(0x1E0028); status != 0 || length == 0 {
		t.Fatalf("emitter result = status %d, length %d", status, length)
	}
	length := h.bus.Read64(0x1E0028)
	source := string(append([]byte(nil), h.cpu.memory[0xA00000:0xA00000+length]...))
	asmPath := filepath.Join(dir, "m2.asm")
	if err := os.WriteFile(asmPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), asmPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("host assembly failed: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "m2.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	return source, image
}

// m2StatementRegion returns the generated statement code, excluding the
// runtime helper bodies emitted after the programme end label. Helper bodies
// legitimately keep tagged fallback paths for dynamically typed callers.
func m2StatementRegion(t *testing.T, source string) string {
	t.Helper()
	idx := strings.Index(source, "compiler_program_end:")
	if idx < 0 {
		t.Fatal("generated source lacks the programme end label")
	}
	return source[:idx]
}

func runM2Image(t *testing.T, image []byte, cycles int) *ehbasicTestHarness {
	t.Helper()
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(cycles)
	return run
}

// Milestone 2 optimisation: whole-programme scalar type proof. Every write to A
// and B stores a proven I64 value, so the additions and bitwise operations must
// lower to direct integer instructions instead of the tagged runtime helper.
// Profile evidence: the mega demo PC profile is dominated by
// compiler_runtime_numeric calls whose operands are integer-only scalars.
func TestIE64BasicCompilerMilestone2ProvenScalarsUseDirectIntegerOps(t *testing.T) {
	source, image := compileM2Programme(t,
		"10 A=5",
		"20 B=A+3",
		"30 C=B AND 7",
		"40 D=C*2+(B >> 1)",
		"50 POKE32 327680,D",
		"60 END",
	)
	if statements := m2StatementRegion(t, source); strings.Contains(statements, "jsr compiler_runtime_numeric") {
		t.Fatalf("proven integer scalar programme still calls the tagged numeric helper:\n%s", statements)
	}
	for _, want := range []string{"add.q r8, r1, r2", "and.q r8, r1, r2"} {
		if !strings.Contains(source, want) {
			t.Fatalf("generated source lacks direct integer lowering %q:\n%s", want, source)
		}
	}
	run := runM2Image(t, image, 200_000)
	// A=5, B=8, C=0, D=(0*2)+(8>>1)=4
	if got := run.bus.Read32(327680); got != 4 {
		t.Fatalf("direct integer programme result = %d, want 4", got)
	}
}

// The proof must be conservative: a single dynamically typed write (here a
// division, which produces FP64) forces every read of that scalar back onto
// the tagged runtime path.
func TestIE64BasicCompilerMilestone2DynamicScalarStaysTagged(t *testing.T) {
	source, image := compileM2Programme(t,
		"10 A=5",
		"20 A=A/2",
		"30 B=A+A",
		"40 POKE32 327680,B",
		"50 END",
	)
	if !strings.Contains(source, "jsr compiler_runtime_numeric") {
		t.Fatalf("dynamically typed scalar lost its tagged runtime path:\n%s", source)
	}
	run := runM2Image(t, image, 200_000)
	// A=2.5 (FP64), B=5.0; POKE32 truncates to 5.
	if got := run.bus.Read32(327680); got != 5 {
		t.Fatalf("dynamic scalar programme result = %d, want 5", got)
	}
}

// Milestone 2 optimisation: proven-I64 FOR/NEXT loops must lower to direct
// integer induction (add, compare, branch) with no per-iteration calls into
// compiler_runtime_numeric. Profile evidence: the mega demo render loop spends
// a large share of samples in the FOR/NEXT helper call sequences.
func TestIE64BasicCompilerMilestone2IntegerForLoopAvoidsRuntimeHelper(t *testing.T) {
	source, image := compileM2Programme(t,
		"10 FOR I=0 TO 255",
		"20 POKE32 327680,I",
		"30 NEXT I",
		"40 POKE32 327684,12345",
		"50 END",
	)
	if statements := m2StatementRegion(t, source); strings.Contains(statements, "jsr compiler_runtime_numeric") {
		t.Fatalf("proven integer FOR loop still calls the tagged numeric helper:\n%s", statements)
	}
	run := runM2Image(t, image, 2_000_000)
	if got := run.bus.Read32(327680); got != 255 {
		t.Fatalf("FOR loop final induction value = %d, want 255", got)
	}
	if got := run.bus.Read32(327684); got != 12345 {
		t.Fatalf("code after the loop did not execute; sentinel = %d, want 12345", got)
	}
}

// A FOR loop with a fractional step is not proven I64 and must keep the tagged
// runtime evaluation to preserve BASIC numeric semantics.
func TestIE64BasicCompilerMilestone2FractionalForLoopStaysTagged(t *testing.T) {
	source, image := compileM2Programme(t,
		"10 T=0",
		"20 FOR I=0 TO 2 STEP 0.5",
		"30 T=T+1",
		"40 NEXT I",
		"50 POKE32 327680,T",
		"60 END",
	)
	if !strings.Contains(source, "jsr compiler_runtime_numeric") {
		t.Fatalf("fractional-step FOR loop lost its tagged runtime path:\n%s", source)
	}
	run := runM2Image(t, image, 2_000_000)
	if got := run.bus.Read32(327680); got != 5 {
		t.Fatalf("fractional FOR loop iterations = %d, want 5", got)
	}
}

// Milestone 2 optimisation: an I64 comparison feeding IF must fuse into one
// conditional branch instead of materialising a boolean and calling the truthy
// helper. Profile evidence: compiler_runtime_truthy call sequences follow
// every compare in the mega demo profile.
func TestIE64BasicCompilerMilestone2IntegerCompareBranchFusion(t *testing.T) {
	source, image := compileM2Programme(t,
		"10 A=5",
		"20 IF A>3 THEN POKE32 327680,1",
		"30 IF A<3 THEN POKE32 327680,2",
		"40 POKE32 327684,777",
		"50 END",
	)
	if strings.Contains(source, "jsr compiler_runtime_truthy") {
		t.Fatalf("integer IF still calls the truthy helper:\n%s", source)
	}
	if strings.Contains(source, "CTRUE") {
		t.Fatalf("integer IF still materialises a boolean through CTRUE:\n%s", source)
	}
	run := runM2Image(t, image, 200_000)
	if got := run.bus.Read32(327680); got != 1 {
		t.Fatalf("fused compare branch result = %d, want 1", got)
	}
	if got := run.bus.Read32(327684); got != 777 {
		t.Fatalf("post-IF sentinel = %d, want 777", got)
	}
}

// TestIE64BasicCompilerMilestone2MegaDemoProfile is a profiling aid, not a
// gate. It samples the guest PC while the compiled mega demo renders and logs
// the hottest code regions with their disassembly, so each Milestone 2
// optimisation can cite measured evidence. Enable with IE_BASIC_RUN_PERF_TESTS=1.
func TestIE64BasicCompilerMilestone2MegaDemoProfile(t *testing.T) {
	milestone2PerfTestsEnabled(t)
	h, _ := startMegaDemoRunAOT(t)

	const bucketShift = 6 // 64-byte buckets
	histogram := make(map[uint64]int)
	stop := make(chan struct{})
	sampler := make(chan struct{})
	go func() {
		defer close(sampler)
		tick := time.NewTicker(100 * time.Microsecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				histogram[h.cpu.PC>>bucketShift]++
			}
		}
	}()
	start := readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"]
	h.pumpUntil(func() bool {
		return readAOTNativeVarsForVoodoo(t, h, "FrameCounter")["FrameCounter"] >= start+240
	}, 120*time.Second)
	close(stop)
	<-sampler

	type bucket struct {
		base  uint64
		count int
	}
	var buckets []bucket
	total := 0
	for base, count := range histogram {
		buckets = append(buckets, bucket{base << bucketShift, count})
		total += count
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].count > buckets[j].count })
	var report strings.Builder
	fmt.Fprintf(&report, "total samples: %d\n", total)
	for i, b := range buckets {
		if i >= 24 {
			break
		}
		fmt.Fprintf(&report, "== %#x: %d samples (%.1f%%)\n", b.base, b.count, 100*float64(b.count)/float64(total))
		disasm := disassembleIE64(func(addr uint64, size int) []byte {
			buf := make([]byte, size)
			for j := range buf {
				buf[j] = h.bus.Read8(uint32(addr) + uint32(j))
			}
			return buf
		}, b.base, 8)
		fmt.Fprintf(&report, "%s\n", fmt.Sprint(disasm))
	}
	t.Logf("mega demo RUN AOT PC profile:\n%s", report.String())

	profilePath := filepath.Join(os.TempDir(), "ie64_basic_milestone2_profile.txt")
	if err := os.WriteFile(profilePath, []byte(report.String()), 0o644); err == nil {
		t.Logf("profile written to %s", profilePath)
	}
}
