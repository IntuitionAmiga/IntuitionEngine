// jit_bench_harness_test.go - unified JIT benchmark harness (Phase 1 of the
// six-CPU JIT unification plan).
//
// This file defines canonical workload identifiers shared by every backend's
// benchmark suite, the mapping from canonical workload to existing per-backend
// Benchmark function, and helpers for normalizing throughput across ISAs.
//
// The harness is consumed by:
//
//   - the per-backend benchmark files (ie64_benchmark_test.go,
//     jit_6502_benchmark_test.go, z80_jit_benchmark_test.go,
//     m68k_jit_benchmark_test.go, x86_jit_benchmark_test.go) which emit
//     normalized side metrics via ReportMIPSHostNormalized;
//   - bench_uniformity_gate_test.go (Phase 9) which loads the latest
//     benchstat output and asserts the ±15% per-workload-median gate.
//
// The harness itself does not run guest code. It is data + helpers.
//
// Build tag note: the file lives in the default package; no extra tags so it
// compiles under every test build (headless, novulkan, full).
//
// Usage from a per-backend bench:
//
//   func BenchmarkX86JIT_ALU_JIT(b *testing.B) {
//     // ... existing benchmark loop ...
//     ReportMIPSHostNormalized(b, totalInstrs)
//   }

package main

import (
	"testing"
)

// CanonicalWorkload enumerates the cross-backend workload categories defined
// in the JIT unification plan. New canonical workloads added in later phases
// must extend this enum and the mapping table below.
type CanonicalWorkload int

const (
	WorkloadALUTight CanonicalWorkload = iota
	WorkloadMemStream
	WorkloadCallChurn
	WorkloadBranchDense
	WorkloadMixed
)

// String returns the canonical workload name as used in Phase-9 reports.
func (w CanonicalWorkload) String() string {
	switch w {
	case WorkloadALUTight:
		return "ALUTight"
	case WorkloadMemStream:
		return "MemStream"
	case WorkloadCallChurn:
		return "CallChurn"
	case WorkloadBranchDense:
		return "BranchDense"
	case WorkloadMixed:
		return "Mixed"
	}
	return "unknown"
}

// allCanonicalWorkloads is iteration order for harness self-checks.
var allCanonicalWorkloads = []CanonicalWorkload{
	WorkloadALUTight,
	WorkloadMemStream,
	WorkloadCallChurn,
	WorkloadBranchDense,
	WorkloadMixed,
}

// BenchBackend identifies a CPU backend in the harness.
type BenchBackend int

const (
	BackendIE64 BenchBackend = iota
	Backend6502
	BackendZ80
	BackendM68K
	BackendX86
)

// String returns the canonical backend tag as used in Phase-9 reports.
func (b BenchBackend) String() string {
	switch b {
	case BackendIE64:
		return "IE64"
	case Backend6502:
		return "6502"
	case BackendZ80:
		return "Z80"
	case BackendM68K:
		return "M68K"
	case BackendX86:
		return "x86"
	}
	return "unknown"
}

// allBenchBackends is iteration order for harness self-checks.
var allBenchBackends = []BenchBackend{
	BackendIE64,
	Backend6502,
	BackendZ80,
	BackendM68K,
	BackendX86,
}

// BenchMapping records, for one (backend, workload) cell, which existing
// per-backend Benchmark function provides the equivalent measurement.
//
//   - JITBenchName / InterpBenchName are the existing Benchmark function names
//     in this package (without the Benchmark prefix). Empty means no
//     equivalent exists; that cell is excluded from the cross-backend median
//     gate (Metric 1) per the Phase-1 mapping table.
//   - Approximate is true when the cell is the closest existing bench but is
//     not a clean canonical equivalent (e.g. M68K Mixed → LazyCCR_CMP_Bcc).
//     Reported in the addendum but excluded from the gate.
//   - InterpGated is true while the JIT path is routed through the
//     interpreter (currently x86 only — see jit_x86_dispatch.go). Cells with
//     InterpGated=true are excluded from the Metric 1 median until
//     Phase 8 flips the dispatch gate.
type BenchMapping struct {
	Backend         BenchBackend
	Workload        CanonicalWorkload
	JITBenchName    string
	InterpBenchName string
	Approximate     bool
	InterpGated     bool
}

// canonicalBenchMappings is the Phase-1 deliverable: the mapping table from
// canonical workloads to existing per-backend benches. It is used by
// Phase-9's uniformity gate to know which cells apply to the ±15% median test
// and which are excluded.
//
// Sources:
//
//   - IE64:  ie64_benchmark_test.go         (BenchmarkIE64_*_{Interpreter,JIT})
//   - 6502:  jit_6502_benchmark_test.go     (Benchmark6502_*_{Interpreter,JIT})
//   - Z80:   z80_jit_benchmark_test.go      (BenchmarkZ80_*_{Interpreter,JIT})
//   - M68K:  m68k_jit_benchmark_test.go     (BenchmarkM68K_*_{Interpreter,JIT})
//   - x86:   x86_jit_benchmark_test.go      (BenchmarkX86JIT_*_{Interpreter,JIT})
//
// Empty JITBenchName / InterpBenchName means no peer exists yet under that
// backend; Phase 1b is allowed to add fresh adapters there, in which case the
// cell flips out of "no equivalent" and into the gate.
var canonicalBenchMappings = []BenchMapping{
	// ALUTight — every backend has an ALU bench.
	{BackendIE64, WorkloadALUTight, "IE64_ALU_JIT", "IE64_ALU_Interpreter", false, false},
	{Backend6502, WorkloadALUTight, "6502_ALU_JIT", "6502_ALU_Interpreter", false, false},
	{BackendZ80, WorkloadALUTight, "Z80_ALU_JIT", "Z80_ALU_Interpreter", false, false},
	{BackendM68K, WorkloadALUTight, "M68K_ALU_JIT", "M68K_ALU_Interpreter", false, false},
	{BackendX86, WorkloadALUTight, "X86JIT_ALU_JIT", "X86JIT_ALU_Interpreter", false, false},

	// MemStream — IE64/6502/Z80/x86 all call it Memory; M68K calls it MemCopy.
	{BackendIE64, WorkloadMemStream, "IE64_Memory_JIT", "IE64_Memory_Interpreter", false, false},
	{Backend6502, WorkloadMemStream, "6502_Memory_JIT", "6502_Memory_Interpreter", false, false},
	{BackendZ80, WorkloadMemStream, "Z80_Memory_JIT", "Z80_Memory_Interpreter", false, false},
	{BackendM68K, WorkloadMemStream, "M68K_MemCopy_JIT", "M68K_MemCopy_Interpreter", false, false},
	{BackendX86, WorkloadMemStream, "X86JIT_Memory_JIT", "X86JIT_Memory_Interpreter", false, false},

	// CallChurn — every backend has a Call bench.
	{BackendIE64, WorkloadCallChurn, "IE64_Call_JIT", "IE64_Call_Interpreter", false, false},
	{Backend6502, WorkloadCallChurn, "6502_Call_JIT", "6502_Call_Interpreter", false, false},
	{BackendZ80, WorkloadCallChurn, "Z80_Call_JIT", "Z80_Call_Interpreter", false, false},
	{BackendM68K, WorkloadCallChurn, "M68K_Call_JIT", "M68K_Call_Interpreter", false, false},
	{BackendX86, WorkloadCallChurn, "X86JIT_Call_JIT", "X86JIT_Call_Interpreter", false, false},

	// BranchDense — only 6502 (Branch) and M68K (Chain_BRA) have peers today.
	// IE64/Z80/x86 baselined fresh in Phase 1b — gate does not apply until
	// adapters land.
	{BackendIE64, WorkloadBranchDense, "", "", false, false},
	{Backend6502, WorkloadBranchDense, "6502_Branch_JIT", "6502_Branch_Interpreter", false, false},
	{BackendZ80, WorkloadBranchDense, "", "", false, false},
	{BackendM68K, WorkloadBranchDense, "M68K_Chain_BRA_JIT", "M68K_Chain_BRA_Interpreter", false, false},
	{BackendX86, WorkloadBranchDense, "", "", false, false},

	// Mixed — IE64/6502/Z80/x86 have a clean Mixed bench; M68K has only
	// LazyCCR_CMP_Bcc which is its closest mixed-flavored peer (approximate).
	{BackendIE64, WorkloadMixed, "IE64_Mixed_JIT", "IE64_Mixed_Interpreter", false, false},
	{Backend6502, WorkloadMixed, "6502_Mixed_JIT", "6502_Mixed_Interpreter", false, false},
	{BackendZ80, WorkloadMixed, "Z80_Mixed_JIT", "Z80_Mixed_Interpreter", false, false},
	{BackendM68K, WorkloadMixed, "M68K_LazyCCR_CMP_Bcc_JIT", "M68K_LazyCCR_CMP_Bcc_Interpreter", true, false},
	{BackendX86, WorkloadMixed, "X86JIT_Mixed_JIT", "X86JIT_Mixed_Interpreter", false, false},
}

// MappingFor returns the BenchMapping for a (backend, workload) cell.
// Returns ok=false if the harness does not yet record that cell.
func MappingFor(backend BenchBackend, workload CanonicalWorkload) (BenchMapping, bool) {
	for _, m := range canonicalBenchMappings {
		if m.Backend == backend && m.Workload == workload {
			return m, true
		}
	}
	return BenchMapping{}, false
}

// CellInGate reports whether a (backend, workload) cell participates in the
// Phase-9 ±15%-of-per-workload-median uniformity gate.
//
// A cell is in the gate iff:
//
//   - it has a real existing peer (JITBenchName != "");
//   - it is not flagged Approximate;
//   - it is not flagged InterpGated. (Once Phase 8 flips the x86 dispatch
//     gate, the InterpGated bool will be cleared and x86 cells will re-enter
//     the median.)
func CellInGate(backend BenchBackend, workload CanonicalWorkload) bool {
	m, ok := MappingFor(backend, workload)
	if !ok {
		return false
	}
	if m.JITBenchName == "" {
		return false
	}
	if m.Approximate {
		return false
	}
	if m.InterpGated {
		return false
	}
	return true
}

// ReportMIPSHostNormalized emits a side metric "MIPS_host" on a benchmark
// record. The metric is guest-instructions-retired per host-nanosecond × 1000,
// which normalizes throughput across ISAs whose canonical workloads execute
// different guest-instruction counts.
//
// Callers pass instrsPerOp = guest instructions retired in ONE outer benchmark
// iteration (matching the existing instructions/op convention used by every
// per-backend bench file in the tree). The benchmark must have run b.N to
// completion already (call this after the timed loop).
//
// MIPS_host = instrsPerOp / ns/op * 1000
//
//   - ns/op = b.Elapsed().Nanoseconds() / b.N
//
// Phase-1 spec: "geometric mean of three runs after a warm-up run".
// Multi-run aggregation is the responsibility of the caller (or the
// run_6502_bench_report.sh-style harness wrapper); this helper only emits the
// single-run metric in a form benchstat can consume.
func ReportMIPSHostNormalized(b *testing.B, instrsPerOp int) {
	b.Helper()
	if b.N <= 0 || instrsPerOp <= 0 {
		return
	}
	elapsed := b.Elapsed()
	if elapsed <= 0 {
		return
	}
	nsPerOp := float64(elapsed.Nanoseconds()) / float64(b.N)
	if nsPerOp <= 0 {
		return
	}
	mips := float64(instrsPerOp) / nsPerOp * 1000.0
	b.ReportMetric(mips, "MIPS_host")
}

// TestBenchHarness_MappingTableConsistent asserts the mapping table covers
// exactly the (5 backends × 5 canonical workloads) grid with no duplicates
// and no stray entries. This is the unit-of-truth contract for Phase 9 — if a
// later phase adds a new canonical workload, this test fails until the table
// is extended.
func TestBenchHarness_MappingTableConsistent(t *testing.T) {
	expected := len(allBenchBackends) * len(allCanonicalWorkloads)
	if got := len(canonicalBenchMappings); got != expected {
		t.Fatalf("mapping table size: got %d entries, want %d (backends=%d × workloads=%d)",
			got, expected, len(allBenchBackends), len(allCanonicalWorkloads))
	}
	seen := make(map[[2]int]bool, expected)
	for _, m := range canonicalBenchMappings {
		key := [2]int{int(m.Backend), int(m.Workload)}
		if seen[key] {
			t.Errorf("duplicate mapping for backend=%v workload=%v", m.Backend, m.Workload)
		}
		seen[key] = true
		// JIT and interpreter names must either both be present or both be empty.
		if (m.JITBenchName == "") != (m.InterpBenchName == "") {
			t.Errorf("backend=%v workload=%v: JITBenchName and InterpBenchName must both be set or both empty (got %q / %q)",
				m.Backend, m.Workload, m.JITBenchName, m.InterpBenchName)
		}
	}
	for _, b := range allBenchBackends {
		for _, w := range allCanonicalWorkloads {
			if !seen[[2]int{int(b), int(w)}] {
				t.Errorf("missing mapping for backend=%v workload=%v", b, w)
			}
		}
	}
}

// TestBenchHarness_GateMembershipMatchesPlan locks the gate-membership
// decisions in a test so a future inadvertent edit fails loudly.
//
// Phase 8 flipped the x86 dispatch gate; the InterpGated flag for every
// x86 mapping was cleared in lockstep, so x86 cells with a JITBenchName
// now participate in the gate. Any cell with JITBenchName=="" (the gate
// adapter is still TBD) remains excluded by CellInGate's contract.
func TestBenchHarness_GateMembershipMatchesPlan(t *testing.T) {
	// Per Phase 8: x86 cells with a JITBenchName are now in the gate.
	// Cells whose adapter is still "" remain excluded (BranchDense for x86).
	for _, w := range allCanonicalWorkloads {
		m, _ := MappingFor(BackendX86, w)
		want := m.JITBenchName != ""
		got := CellInGate(BackendX86, w)
		if got != want {
			t.Errorf("x86 %v: CellInGate=%v, want %v (JITBenchName=%q)",
				w, got, want, m.JITBenchName)
		}
	}
	// Per Phase 1: M68K Mixed (LazyCCR_CMP_Bcc) is approximate and excluded.
	if CellInGate(BackendM68K, WorkloadMixed) {
		t.Error("M68K Mixed should be Approximate=true and excluded from gate")
	}
	// Per Phase 1: BranchDense gate covers only 6502 + M68K until 1b adapters land.
	wantInGate := map[BenchBackend]bool{
		BackendIE64: false,
		Backend6502: true,
		BackendZ80:  false,
		BackendM68K: true,
		BackendX86:  false,
	}
	for backend, want := range wantInGate {
		if got := CellInGate(backend, WorkloadBranchDense); got != want {
			t.Errorf("BranchDense %v: CellInGate=%v want %v", backend, got, want)
		}
	}
}
