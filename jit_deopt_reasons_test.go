package main

import (
	"strings"
	"testing"
)

func TestDeoptClassify_ExitFlags(t *testing.T) {
	cases := []struct {
		name  string
		flags DeoptExitFlags
		want  DeoptReason
	}{
		{"none", DeoptExitFlags{}, DeoptNone},
		{"unsupported", DeoptExitFlags{Unsupported: true}, DeoptUnsupported},
		{"helper", DeoptExitFlags{NeedHelper: true}, DeoptHelper},
		{"mmio", DeoptExitFlags{NeedIO: true}, DeoptMMIO},
		{"smc", DeoptExitFlags{NeedInval: true}, DeoptSMC},
		{"interrupt", DeoptExitFlags{Interrupt: true}, DeoptInterrupt},
		{"cache-pressure", DeoptExitFlags{CachePressure: true}, DeoptCachePressure},
		{"debug", DeoptExitFlags{Debug: true}, DeoptDebug},
		{"smc-dominates-mmio", DeoptExitFlags{NeedInval: true, NeedIO: true}, DeoptSMC},
		{"mmio-dominates-helper", DeoptExitFlags{NeedIO: true, NeedHelper: true}, DeoptMMIO},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyDeoptExit(tc.flags); got != tc.want {
				t.Fatalf("ClassifyDeoptExit(%+v) = %s, want %s", tc.flags, got, tc.want)
			}
		})
	}
}

func TestDeoptStats_StringerTotal(t *testing.T) {
	withPerfAcct(t, true, func() {
		var stats DeoptStats
		stats.Add(DeoptMMIO)
		stats.Add(DeoptMMIO)
		stats.Add(DeoptSMC)

		snap := stats.Snapshot()
		if snap.Total != 3 {
			t.Fatalf("total = %d, want 3", snap.Total)
		}
		got := stats.String()
		for _, want := range []string{"total=3", "mmio=2", "smc=1"} {
			if !strings.Contains(got, want) {
				t.Fatalf("String() = %q, missing %q", got, want)
			}
		}
	})
}

func TestRecordBlockDeopt_FirstReasonWins(t *testing.T) {
	withPerfAcct(t, true, func() {
		var stats DeoptStats
		block := &JITBlock{}
		recordBlockDeopt(&stats, block, DeoptMMIO)
		recordBlockDeopt(&stats, block, DeoptSMC)

		if block.dominantDeopt != DeoptMMIO {
			t.Fatalf("dominantDeopt = %s, want %s", block.dominantDeopt, DeoptMMIO)
		}
		if got := stats.Snapshot().Total; got != 2 {
			t.Fatalf("stats total = %d, want 2", got)
		}
	})
}
