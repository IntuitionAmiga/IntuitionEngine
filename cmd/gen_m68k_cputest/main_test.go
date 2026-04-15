package main

import (
	"strings"
	"testing"
)

func TestBuildCatalogSorted(t *testing.T) {
	shards := buildCatalog()
	for i := 1; i < len(shards); i++ {
		if shards[i-1].Name > shards[i].Name {
			t.Fatalf("shards not sorted: %q before %q", shards[i-1].Name, shards[i].Name)
		}
	}
}

func TestBuildCatalogShardAndCaseCount(t *testing.T) {
	shards := buildCatalog()
	totalCases := 0
	for _, sh := range shards {
		totalCases += len(sh.Cases)
		// Verify all cases have the right shard
		for _, tc := range sh.Cases {
			if tc.Shard != sh.Name {
				t.Errorf("case %q has shard %q, expected %q", tc.ID, tc.Shard, sh.Name)
			}
			if tc.ID == "" {
				t.Errorf("case in shard %q has empty ID", sh.Name)
			}
		}
	}
	t.Logf("Total shards: %d, total cases: %d", len(shards), totalCases)
	if len(shards) < 20 {
		t.Errorf("expected at least 20 shards, got %d", len(shards))
	}
	if totalCases < 400 {
		t.Errorf("expected at least 400 cases, got %d", totalCases)
	}
}

func TestRenderManifestContainsShardLabels(t *testing.T) {
	shards := buildCatalog()
	totalCases := 0
	for _, sh := range shards {
		totalCases += len(sh.Cases)
	}
	out := renderManifest(shards, totalCases)
	for _, label := range []string{"run_core_alu_shard", "run_core_020_shard", "run_fpu_data_shard"} {
		if !strings.Contains(out, label) {
			t.Fatalf("manifest missing %q", label)
		}
	}
	// Verify expected total is emitted
	if !strings.Contains(out, "ct_expected_total") {
		t.Fatal("manifest should reference ct_expected_total")
	}
	if !strings.Contains(out, "ct_set_expected_total") {
		t.Fatal("manifest should define ct_set_expected_total")
	}
}

func TestRenderShardContainsCases(t *testing.T) {
	shards := buildCatalog()
	out := renderShard(shards[0])
	if !strings.Contains(out, "case_") {
		t.Fatal("shard output should contain case labels")
	}
	if !strings.Contains(out, "ct_fail_") || !strings.Contains(out, "ct_log_pass") {
		t.Fatal("shard output should call failure/pass helpers")
	}
}

func TestNoDuplicateIDs(t *testing.T) {
	shards := buildCatalog()
	seen := make(map[string]bool)
	for _, sh := range shards {
		for _, tc := range sh.Cases {
			if seen[tc.ID] {
				t.Errorf("duplicate case ID: %q", tc.ID)
			}
			seen[tc.ID] = true
		}
	}
}
