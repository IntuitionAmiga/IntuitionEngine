package main

import "testing"

func TestIntegerCCLiveness_ProducerFollowedByConsumer(t *testing.T) {
	lines := lexLines(t, []string{
		"\tadd.w d1,d0", // producer (line 0) — flags live (next op reads)
		"\tbeq exit",    // consumer
		"\tnop",
	})
	live := computeIntegerCCLiveness(lines)
	if !live[0] {
		t.Errorf("add followed by beq: liveAt[0] should be true; got %v", live)
	}
}

func TestIntegerCCLiveness_ProducerFollowedByProducer(t *testing.T) {
	lines := lexLines(t, []string{
		"\tadd.w d1,d0", // producer (line 0) — flags dead (next op overwrites)
		"\tadd.w d3,d2", // producer (line 1) — flags dead (terminator below)
		"\trts",
	})
	live := computeIntegerCCLiveness(lines)
	if live[0] {
		t.Errorf("add followed by add followed by rts: liveAt[0] should be false; got %v", live)
	}
	if live[1] {
		t.Errorf("trailing add before rts: liveAt[1] should be false; got %v", live)
	}
}

func TestIntegerCCLiveness_LabelForcesLive(t *testing.T) {
	lines := lexLines(t, []string{
		"\tadd.w d1,d0", // producer (line 0)
		"label:",         // label — forces live before
		"\tadd.w d3,d2", // producer (line 2)
		"\trts",
	})
	live := computeIntegerCCLiveness(lines)
	if !live[0] {
		t.Errorf("producer followed by label: liveAt[0] should be true (branch in from elsewhere); got %v", live)
	}
}

func TestIntegerCCLiveness_AddxReadsX(t *testing.T) {
	lines := lexLines(t, []string{
		"\tadd.w d1,d0",  // producer (writes X)
		"\taddx.w d3,d2", // consumer (reads X)
		"\trts",
	})
	live := computeIntegerCCLiveness(lines)
	if !live[0] {
		t.Errorf("add followed by addx: liveAt[0] should be true (addx reads X); got %v", live)
	}
}

func TestIntegerCCLiveness_DbraIsNotConsumer(t *testing.T) {
	// DBRA tests counter, NOT flags. So producer immediately before
	// DBRA still has dead flags (assuming no later consumer).
	lines := lexLines(t, []string{
		"\tadd.w d1,d0",
		"\tdbra d7,loop",
		"\trts",
	})
	live := computeIntegerCCLiveness(lines)
	if live[0] {
		t.Errorf("add followed by dbra+rts: liveAt[0] should be false (dbra doesn't read flags); got %v", live)
	}
}

func TestIntegerCCLiveness_DbccIsConsumer(t *testing.T) {
	// DBcc (conditional, not DBRA/DBF) reads condition flags.
	lines := lexLines(t, []string{
		"\tadd.w d1,d0",
		"\tdbeq d7,loop",
		"\trts",
	})
	live := computeIntegerCCLiveness(lines)
	if !live[0] {
		t.Errorf("add followed by dbeq: liveAt[0] should be true (dbcc reads flags); got %v", live)
	}
}

// lexLines is a small test helper: lex each input line.
func lexLines(t *testing.T, src []string) []Line {
	t.Helper()
	out := make([]Line, len(src))
	for i, s := range src {
		out[i] = LexLine(s)
	}
	return out
}
