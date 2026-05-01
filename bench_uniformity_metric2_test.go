// bench_uniformity_metric2_test.go - Metric 2 fixture parser + per-backend
// self-comparison gate.
//
// Metric 2 evaluates each backend on its canonical real workload (rotozoomer
// for M68K, AROS boot, Spectrum demo for Z80, diag program for 6502, an
// IE64 demo). For each backend the three Phase-1 conditions must hold:
//
//   1. No regression vs Phase 0 baseline (within ±5% of phase0).
//   2. Reaches real-time (current ≥ real_time_target).
//   3. ≥10% improvement on JIT-bound time vs Phase 0 (jit_bound_current
//      must be ≤ 0.90 × jit_bound_phase0). Waivable when io_bound_waiver
//      is true and a justification line is recorded.
//
// Format (one record per real workload, blocks separated by `---`):
//
//   backend=<6502|z80|m68k|ie64|x86>
//   workload=<short-name>
//   metric=<frames_per_sec|wallclock_ms_to_milestone>
//   phase0=<float>
//   current=<float>
//   real_time_target=<float>     ; Hz or ms (units match metric)
//   jit_bound_phase0=<float>     ; ns spent in JIT during Phase 0 sample
//   jit_bound_current=<float>    ; ns spent in JIT during current sample
//   io_bound_waiver=<true|false>
//   waiver_reason=<text>         ; required when io_bound_waiver=true
//   ---
//
// Lines starting with `#` or blank lines are ignored. Unknown keys cause a
// parse error so silent typos cannot pass the gate.
//
// No build tag: parser/evaluator are pure Go and consumed by the
// untagged Metric 2 test in bench_uniformity_gate_test.go. Tagging this
// file amd64-only would break the arm64 test binary at compile time
// (the consumer's identifiers go undefined under the same tag).

package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

type Metric2Record struct {
	Backend         string
	Workload        string
	Metric          string
	Phase0          float64
	Current         float64
	RealTimeTarget  float64
	JITBoundPhase0  float64
	JITBoundCurrent float64
	IOBoundWaiver   bool
	WaiverReason    string
}

const (
	metric2RegressionTolerance     = 0.05 // ±5% vs phase0
	metric2JITImprovementMinReduce = 0.10 // jit_bound_current ≤ 0.90 × phase0
)

var metric2RequiredKeys = []string{
	"backend", "workload", "metric", "phase0", "current",
	"real_time_target", "jit_bound_phase0", "jit_bound_current",
	"io_bound_waiver",
}

var metric2KnownBackends = map[string]bool{
	"6502": true, "z80": true, "m68k": true, "ie64": true, "x86": true,
}

func parseMetric2(r io.Reader) ([]Metric2Record, error) {
	var out []Metric2Record
	sc := bufio.NewScanner(r)
	cur := map[string]string{}
	lineNo := 0
	flush := func() error {
		if len(cur) == 0 {
			return nil
		}
		for _, k := range metric2RequiredKeys {
			if _, ok := cur[k]; !ok {
				return fmt.Errorf("metric2 record near line %d missing required key %q", lineNo, k)
			}
		}
		rec := Metric2Record{
			Backend:      cur["backend"],
			Workload:     cur["workload"],
			Metric:       cur["metric"],
			WaiverReason: cur["waiver_reason"],
		}
		if !metric2KnownBackends[rec.Backend] {
			return fmt.Errorf("metric2 record near line %d: unknown backend %q", lineNo, rec.Backend)
		}
		floats := []struct {
			key string
			dst *float64
		}{
			{"phase0", &rec.Phase0},
			{"current", &rec.Current},
			{"real_time_target", &rec.RealTimeTarget},
			{"jit_bound_phase0", &rec.JITBoundPhase0},
			{"jit_bound_current", &rec.JITBoundCurrent},
		}
		for _, f := range floats {
			v, err := strconv.ParseFloat(cur[f.key], 64)
			if err != nil {
				return fmt.Errorf("metric2 record near line %d: %s=%q: %v", lineNo, f.key, cur[f.key], err)
			}
			if v <= 0 {
				return fmt.Errorf("metric2 record near line %d: %s=%g must be positive "+
					"(zero or negative measurements would make the gate vacuous)",
					lineNo, f.key, v)
			}
			*f.dst = v
		}
		switch cur["io_bound_waiver"] {
		case "true":
			rec.IOBoundWaiver = true
		case "false":
			rec.IOBoundWaiver = false
		default:
			return fmt.Errorf("metric2 record near line %d: io_bound_waiver=%q must be true|false",
				lineNo, cur["io_bound_waiver"])
		}
		if rec.IOBoundWaiver && strings.TrimSpace(rec.WaiverReason) == "" {
			return fmt.Errorf("metric2 record near line %d: io_bound_waiver=true requires waiver_reason", lineNo)
		}
		out = append(out, rec)
		cur = map[string]string{}
		return nil
	}
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "---" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("metric2 line %d: not key=value: %q", lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		known := key == "waiver_reason"
		for _, k := range metric2RequiredKeys {
			if k == key {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("metric2 line %d: unknown key %q", lineNo, key)
		}
		if _, dup := cur[key]; dup {
			return nil, fmt.Errorf("metric2 line %d: duplicate key %q in record", lineNo, key)
		}
		cur[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

// evalMetric2 returns one failure string per violated condition. Empty
// slice = pass.
func evalMetric2(rec Metric2Record) []string {
	var fails []string
	// (1) regression vs phase0 within ±5%
	if rec.Phase0 > 0 {
		delta := (rec.Current - rec.Phase0) / rec.Phase0
		if delta < -metric2RegressionTolerance {
			fails = append(fails, fmt.Sprintf(
				"backend=%s workload=%s: current=%g regressed %.2f%% vs phase0=%g (>%.0f%% allowed)",
				rec.Backend, rec.Workload, rec.Current, delta*100, rec.Phase0,
				metric2RegressionTolerance*100))
		}
	}
	// (2) reaches real-time. Metric direction depends on metric type.
	switch rec.Metric {
	case "frames_per_sec":
		// higher-is-better: current must be ≥ real_time_target
		if rec.Current < rec.RealTimeTarget {
			fails = append(fails, fmt.Sprintf(
				"backend=%s workload=%s: current=%g fps below real_time_target=%g fps",
				rec.Backend, rec.Workload, rec.Current, rec.RealTimeTarget))
		}
	case "wallclock_ms_to_milestone":
		// lower-is-better: current must be ≤ real_time_target
		if rec.Current > rec.RealTimeTarget {
			fails = append(fails, fmt.Sprintf(
				"backend=%s workload=%s: current=%g ms exceeds real_time_target=%g ms",
				rec.Backend, rec.Workload, rec.Current, rec.RealTimeTarget))
		}
	default:
		fails = append(fails, fmt.Sprintf(
			"backend=%s workload=%s: unknown metric=%q (want frames_per_sec|wallclock_ms_to_milestone)",
			rec.Backend, rec.Workload, rec.Metric))
	}
	// (3) ≥10% improvement on JIT-bound time
	if rec.IOBoundWaiver {
		return fails
	}
	if rec.JITBoundPhase0 > 0 {
		maxAllowed := rec.JITBoundPhase0 * (1.0 - metric2JITImprovementMinReduce)
		if rec.JITBoundCurrent > maxAllowed {
			reduce := (rec.JITBoundPhase0 - rec.JITBoundCurrent) / rec.JITBoundPhase0
			fails = append(fails, fmt.Sprintf(
				"backend=%s workload=%s: jit_bound_current=%g ns vs phase0=%g ns (only %.2f%% reduction; need ≥%.0f%%)",
				rec.Backend, rec.Workload, rec.JITBoundCurrent, rec.JITBoundPhase0,
				reduce*100, metric2JITImprovementMinReduce*100))
		}
	}
	return fails
}

func TestParseMetric2_ValidBlock(t *testing.T) {
	in := `# comment line
backend=ie64
workload=ie64-demo
metric=frames_per_sec
phase0=58.0
current=60.0
real_time_target=60.0
jit_bound_phase0=12000000
jit_bound_current=10000000
io_bound_waiver=false
---
backend=m68k
workload=rotozoomer
metric=wallclock_ms_to_milestone
phase0=120.0
current=110.0
real_time_target=125.0
jit_bound_phase0=8000000
jit_bound_current=6500000
io_bound_waiver=false
`
	recs, err := parseMetric2(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseMetric2: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].Backend != "ie64" || recs[0].Current != 60.0 {
		t.Errorf("rec0 mismatch: %+v", recs[0])
	}
	if recs[1].Backend != "m68k" || recs[1].Metric != "wallclock_ms_to_milestone" {
		t.Errorf("rec1 mismatch: %+v", recs[1])
	}
}

func TestParseMetric2_RejectsUnknownKey(t *testing.T) {
	in := `backend=ie64
workload=demo
totally_made_up_key=42
metric=frames_per_sec
phase0=60
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
io_bound_waiver=false
`
	_, err := parseMetric2(strings.NewReader(in))
	if err == nil {
		t.Fatal("parseMetric2: expected error on unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("error %q does not mention unknown key", err)
	}
}

func TestParseMetric2_RejectsMissingRequiredKey(t *testing.T) {
	in := `backend=ie64
workload=demo
metric=frames_per_sec
phase0=60
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
`
	_, err := parseMetric2(strings.NewReader(in))
	if err == nil {
		t.Fatal("parseMetric2: expected error on missing io_bound_waiver, got nil")
	}
	if !strings.Contains(err.Error(), "io_bound_waiver") {
		t.Errorf("error %q does not mention missing key", err)
	}
}

func TestParseMetric2_RejectsUnknownBackend(t *testing.T) {
	in := `backend=68040
workload=demo
metric=frames_per_sec
phase0=60
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
io_bound_waiver=false
`
	_, err := parseMetric2(strings.NewReader(in))
	if err == nil {
		t.Fatal("parseMetric2: expected error on unknown backend, got nil")
	}
}

func TestParseMetric2_WaiverRequiresReason(t *testing.T) {
	in := `backend=ie64
workload=demo
metric=frames_per_sec
phase0=60
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
io_bound_waiver=true
`
	_, err := parseMetric2(strings.NewReader(in))
	if err == nil {
		t.Fatal("parseMetric2: expected error on waiver without reason, got nil")
	}
	if !strings.Contains(err.Error(), "waiver_reason") {
		t.Errorf("error %q does not mention waiver_reason", err)
	}
}

func TestEvalMetric2_AllPass(t *testing.T) {
	rec := Metric2Record{
		Backend:         "ie64",
		Workload:        "demo",
		Metric:          "frames_per_sec",
		Phase0:          58.0,
		Current:         60.0,
		RealTimeTarget:  60.0,
		JITBoundPhase0:  10000000,
		JITBoundCurrent: 8500000, // 15% reduction
	}
	if fails := evalMetric2(rec); len(fails) != 0 {
		t.Errorf("expected 0 fails, got %v", fails)
	}
}

func TestEvalMetric2_RegressionFails(t *testing.T) {
	rec := Metric2Record{
		Backend: "ie64", Workload: "demo", Metric: "frames_per_sec",
		Phase0: 60.0, Current: 50.0, RealTimeTarget: 60.0,
		JITBoundPhase0: 10, JITBoundCurrent: 8,
	}
	fails := evalMetric2(rec)
	hit := false
	for _, f := range fails {
		if strings.Contains(f, "regressed") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("expected regression failure, got %v", fails)
	}
}

func TestEvalMetric2_RealTimeFloorFails_FPS(t *testing.T) {
	rec := Metric2Record{
		Backend: "ie64", Workload: "demo", Metric: "frames_per_sec",
		Phase0: 50.0, Current: 50.0, RealTimeTarget: 60.0,
		JITBoundPhase0: 10, JITBoundCurrent: 8,
	}
	fails := evalMetric2(rec)
	hit := false
	for _, f := range fails {
		if strings.Contains(f, "below real_time_target") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("expected real-time floor failure, got %v", fails)
	}
}

func TestEvalMetric2_RealTimeFloorFails_MS(t *testing.T) {
	rec := Metric2Record{
		Backend: "m68k", Workload: "rotozoomer", Metric: "wallclock_ms_to_milestone",
		Phase0: 100.0, Current: 100.0, RealTimeTarget: 80.0,
		JITBoundPhase0: 10, JITBoundCurrent: 8,
	}
	fails := evalMetric2(rec)
	hit := false
	for _, f := range fails {
		if strings.Contains(f, "exceeds real_time_target") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("expected wallclock-ms floor failure, got %v", fails)
	}
}

func TestEvalMetric2_JITImprovementShortfallFails(t *testing.T) {
	rec := Metric2Record{
		Backend: "ie64", Workload: "demo", Metric: "frames_per_sec",
		Phase0: 60.0, Current: 60.0, RealTimeTarget: 60.0,
		JITBoundPhase0: 10000000, JITBoundCurrent: 9500000, // only 5% reduction
	}
	fails := evalMetric2(rec)
	hit := false
	for _, f := range fails {
		if strings.Contains(f, "jit_bound_current") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("expected JIT-bound improvement shortfall, got %v", fails)
	}
}

func TestParseMetric2_RejectsNonPositiveMeasurement(t *testing.T) {
	cases := []struct {
		key  string
		body string
	}{
		{"phase0=0", `backend=ie64
workload=demo
metric=frames_per_sec
phase0=0
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
io_bound_waiver=false
`},
		{"phase0=negative", `backend=ie64
workload=demo
metric=frames_per_sec
phase0=-1
current=60
real_time_target=60
jit_bound_phase0=1
jit_bound_current=1
io_bound_waiver=false
`},
		{"jit_bound_phase0=0", `backend=ie64
workload=demo
metric=frames_per_sec
phase0=60
current=60
real_time_target=60
jit_bound_phase0=0
jit_bound_current=1
io_bound_waiver=false
`},
	}
	for _, c := range cases {
		_, err := parseMetric2(strings.NewReader(c.body))
		if err == nil {
			t.Errorf("case %q: expected positive-measurement rejection, got nil", c.key)
			continue
		}
		if !strings.Contains(err.Error(), "must be positive") {
			t.Errorf("case %q: error %q missing 'must be positive'", c.key, err)
		}
	}
}

func TestEvalMetric2_WaiverSuppressesJITGate(t *testing.T) {
	rec := Metric2Record{
		Backend: "ie64", Workload: "io-bound-demo", Metric: "frames_per_sec",
		Phase0: 60.0, Current: 60.0, RealTimeTarget: 60.0,
		JITBoundPhase0: 10000000, JITBoundCurrent: 9900000, // 1% reduction
		IOBoundWaiver: true, WaiverReason: "workload is host-IO-bound, JIT time is noise",
	}
	fails := evalMetric2(rec)
	for _, f := range fails {
		if strings.Contains(f, "jit_bound_current") {
			t.Errorf("waiver did not suppress JIT-bound check: %v", fails)
		}
	}
}
