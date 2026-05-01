// jit_bench_adapters_test.go - per-backend bench adapter registry
// (Phase 1b of the six-CPU JIT unification plan).
//
// Ties every JITBenchName / InterpBenchName string in canonicalBenchMappings
// to the actual Benchmark function symbol in this package. If a benchmark is
// renamed or deleted while the mapping still references the old name:
//
//   - the symbol reference below fails to compile (rename / delete), OR
//   - TestBenchAdapters_PresenceMatchesMapping fails (mapping has a string
//     with no registered (name, func) pair).
//
// This is the guard that the adapter registry actually tracks code, not
// just self-validates against the same mapping.
//
// Build tag matches the most restrictive bench file (m68k_jit_benchmark_test.go
// + x86_jit_benchmark_test.go are `amd64 && linux`). Z80/6502/IE64 benches
// build on a wider set, but the registry must reference all five so the tag
// here is the intersection.

//go:build amd64 && linux

package main

import (
	"testing"
)

// benchSymbolPair binds a benchmark function name string to the actual
// Benchmark function value. The function-value side ensures the symbol
// exists at compile time; the string side ensures it matches the mapping
// table.
type benchSymbolPair struct {
	name string
	fn   func(*testing.B)
}

// benchSymbolPairs is the full set of Benchmark functions referenced by
// canonicalBenchMappings. New entries land here when a new bench function
// is added to a per-backend file.
var benchSymbolPairs = []benchSymbolPair{
	// IE64
	{"IE64_ALU_Interpreter", BenchmarkIE64_ALU_Interpreter},
	{"IE64_ALU_JIT", BenchmarkIE64_ALU_JIT},
	{"IE64_Memory_Interpreter", BenchmarkIE64_Memory_Interpreter},
	{"IE64_Memory_JIT", BenchmarkIE64_Memory_JIT},
	{"IE64_Call_Interpreter", BenchmarkIE64_Call_Interpreter},
	{"IE64_Call_JIT", BenchmarkIE64_Call_JIT},
	{"IE64_Mixed_Interpreter", BenchmarkIE64_Mixed_Interpreter},
	{"IE64_Mixed_JIT", BenchmarkIE64_Mixed_JIT},
	// 6502
	{"6502_ALU_Interpreter", Benchmark6502_ALU_Interpreter},
	{"6502_ALU_JIT", Benchmark6502_ALU_JIT},
	{"6502_Memory_Interpreter", Benchmark6502_Memory_Interpreter},
	{"6502_Memory_JIT", Benchmark6502_Memory_JIT},
	{"6502_Call_Interpreter", Benchmark6502_Call_Interpreter},
	{"6502_Call_JIT", Benchmark6502_Call_JIT},
	{"6502_Branch_Interpreter", Benchmark6502_Branch_Interpreter},
	{"6502_Branch_JIT", Benchmark6502_Branch_JIT},
	{"6502_Mixed_Interpreter", Benchmark6502_Mixed_Interpreter},
	{"6502_Mixed_JIT", Benchmark6502_Mixed_JIT},
	// Z80
	{"Z80_ALU_Interpreter", BenchmarkZ80_ALU_Interpreter},
	{"Z80_ALU_JIT", BenchmarkZ80_ALU_JIT},
	{"Z80_Memory_Interpreter", BenchmarkZ80_Memory_Interpreter},
	{"Z80_Memory_JIT", BenchmarkZ80_Memory_JIT},
	{"Z80_Call_Interpreter", BenchmarkZ80_Call_Interpreter},
	{"Z80_Call_JIT", BenchmarkZ80_Call_JIT},
	{"Z80_Mixed_Interpreter", BenchmarkZ80_Mixed_Interpreter},
	{"Z80_Mixed_JIT", BenchmarkZ80_Mixed_JIT},
	// M68K
	{"M68K_ALU_Interpreter", BenchmarkM68K_ALU_Interpreter},
	{"M68K_ALU_JIT", BenchmarkM68K_ALU_JIT},
	{"M68K_MemCopy_Interpreter", BenchmarkM68K_MemCopy_Interpreter},
	{"M68K_MemCopy_JIT", BenchmarkM68K_MemCopy_JIT},
	{"M68K_Call_Interpreter", BenchmarkM68K_Call_Interpreter},
	{"M68K_Call_JIT", BenchmarkM68K_Call_JIT},
	{"M68K_Chain_BRA_Interpreter", BenchmarkM68K_Chain_BRA_Interpreter},
	{"M68K_Chain_BRA_JIT", BenchmarkM68K_Chain_BRA_JIT},
	{"M68K_LazyCCR_CMP_Bcc_Interpreter", BenchmarkM68K_LazyCCR_CMP_Bcc_Interpreter},
	{"M68K_LazyCCR_CMP_Bcc_JIT", BenchmarkM68K_LazyCCR_CMP_Bcc_JIT},
	// x86
	{"X86JIT_ALU_Interpreter", BenchmarkX86JIT_ALU_Interpreter},
	{"X86JIT_ALU_JIT", BenchmarkX86JIT_ALU_JIT},
	{"X86JIT_Memory_Interpreter", BenchmarkX86JIT_Memory_Interpreter},
	{"X86JIT_Memory_JIT", BenchmarkX86JIT_Memory_JIT},
	{"X86JIT_Call_Interpreter", BenchmarkX86JIT_Call_Interpreter},
	{"X86JIT_Call_JIT", BenchmarkX86JIT_Call_JIT},
	{"X86JIT_Mixed_Interpreter", BenchmarkX86JIT_Mixed_Interpreter},
	{"X86JIT_Mixed_JIT", BenchmarkX86JIT_Mixed_JIT},
}

// builderPresence maps a benchmark name string to whether a real Benchmark
// function symbol is bound to it. Populated from benchSymbolPairs.
var builderPresence = func() map[string]bool {
	m := make(map[string]bool, len(benchSymbolPairs))
	for _, p := range benchSymbolPairs {
		if p.fn == nil {
			continue
		}
		m[p.name] = true
	}
	return m
}()

// TestBenchAdapters_PresenceMatchesMapping asserts every (backend, workload)
// cell that the mapping table claims is "in the gate" has a real
// Benchmark function bound in benchSymbolPairs. If a future edit renames or
// deletes a Benchmark function, the symbol reference above fails to compile;
// if it edits the mapping string without updating benchSymbolPairs, this
// test fails.
func TestBenchAdapters_PresenceMatchesMapping(t *testing.T) {
	for _, m := range canonicalBenchMappings {
		if !CellInGate(m.Backend, m.Workload) {
			continue
		}
		if !builderPresence[m.JITBenchName] {
			t.Errorf("backend=%v workload=%v: JIT bench %q has no Benchmark function bound in benchSymbolPairs",
				m.Backend, m.Workload, m.JITBenchName)
		}
		if !builderPresence[m.InterpBenchName] {
			t.Errorf("backend=%v workload=%v: interpreter bench %q has no Benchmark function bound in benchSymbolPairs",
				m.Backend, m.Workload, m.InterpBenchName)
		}
	}
	// Inverse: every binding in benchSymbolPairs must correspond to either
	// an entry in the mapping table or be deliberately retained (e.g. the
	// x86 String bench has no canonical workload yet). The forward check
	// above is the load-bearing one; this loop just guards typos.
	mappingNames := make(map[string]bool)
	for _, m := range canonicalBenchMappings {
		if m.JITBenchName != "" {
			mappingNames[m.JITBenchName] = true
		}
		if m.InterpBenchName != "" {
			mappingNames[m.InterpBenchName] = true
		}
	}
	for _, p := range benchSymbolPairs {
		if !mappingNames[p.name] {
			t.Logf("benchSymbolPairs has %q with no mapping entry — keep if intentional (e.g. x86 String)",
				p.name)
		}
	}
}

// TestBenchAdapters_GapsAreExplicit lists the (backend, workload) cells that
// are explicitly empty in the Phase-1 mapping (i.e. JITBenchName == ""). The
// test passes as long as those cells stay accounted for in the addendum's
// gap list. It exists so that a future maintainer cannot quietly add a
// fresh-but-untested cell without expanding the test expectation.
func TestBenchAdapters_GapsAreExplicit(t *testing.T) {
	wantGaps := map[BenchBackend]map[CanonicalWorkload]bool{
		BackendIE64: {WorkloadBranchDense: true},
		BackendZ80:  {WorkloadBranchDense: true},
		BackendX86:  {WorkloadBranchDense: true},
	}
	for _, m := range canonicalBenchMappings {
		if m.JITBenchName != "" {
			continue
		}
		if !wantGaps[m.Backend][m.Workload] {
			t.Errorf("unexpected gap at backend=%v workload=%v — extend wantGaps if intentional",
				m.Backend, m.Workload)
		}
	}
	for backend, workloads := range wantGaps {
		for w := range workloads {
			m, ok := MappingFor(backend, w)
			if !ok {
				t.Errorf("declared gap for backend=%v workload=%v but no mapping entry", backend, w)
				continue
			}
			if m.JITBenchName != "" {
				t.Errorf("declared gap for backend=%v workload=%v but mapping has JITBenchName=%q",
					backend, w, m.JITBenchName)
			}
		}
	}
}
