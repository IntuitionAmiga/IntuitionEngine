//go:build headless

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var imaStepTable = []int{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17,
	19, 21, 23, 25, 28, 31, 34, 37, 41, 45,
	50, 55, 60, 66, 73, 80, 88, 97, 107, 118,
	130, 143, 157, 173, 190, 209, 230, 253, 279, 307,
	337, 371, 408, 449, 494, 544, 598, 658, 724, 796,
	876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066,
	2272, 2499, 2749, 3024, 3327, 3660, 4026, 4428, 4871, 5358,
	5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899,
	15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794, 32767,
}

var imaIndexAdjust = []int{-1, -1, -1, -1, 2, 4, 6, 8, -1, -1, -1, -1, 2, 4, 6, 8}

func readIEWarpSource(t *testing.T) string {
	t.Helper()
	root := assemblerExamplesRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "sdk", "examples", "asm", "iewarp_service.asm"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func parseIEWarpTable(t *testing.T, src, label, directive string) []int {
	t.Helper()
	idx := strings.Index(src, label+":")
	if idx < 0 {
		t.Fatalf("label %s not found", label)
	}
	var out []int
	for _, line := range strings.Split(src[idx:], "\n")[1:] {
		line = strings.Split(line, ";")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ":") {
			break
		}
		if !strings.HasPrefix(strings.ToLower(line), directive) {
			continue
		}
		for _, field := range strings.Split(strings.TrimSpace(line[len(directive):]), ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			field = strings.TrimPrefix(field, "#")
			v, err := strconv.ParseInt(field, 0, 32)
			if err != nil {
				t.Fatalf("parse %s value %q: %v", label, field, err)
			}
			out = append(out, int(v))
		}
	}
	return out
}

func TestIEWarpADPCM_StepTableMatchesIMA(t *testing.T) {
	got := parseIEWarpTable(t, readIEWarpSource(t), "ima_step_table", "dc.w")
	if len(got) != len(imaStepTable) {
		t.Fatalf("ima_step_table length=%d, want %d", len(got), len(imaStepTable))
	}
	for i := range imaStepTable {
		if got[i] != imaStepTable[i] {
			t.Fatalf("ima_step_table[%d]=%d, want %d", i, got[i], imaStepTable[i])
		}
	}
}

func TestIEWarpADPCM_IndexAdjustMatchesIMA(t *testing.T) {
	got := parseIEWarpTable(t, readIEWarpSource(t), "ima_index_adjust", "dc.l")
	if len(got) != len(imaIndexAdjust) {
		t.Fatalf("ima_index_adjust length=%d, want %d", len(got), len(imaIndexAdjust))
	}
	for i := range imaIndexAdjust {
		if got[i] != imaIndexAdjust[i] {
			t.Fatalf("ima_index_adjust[%d]=%d, want %d", i, got[i], imaIndexAdjust[i])
		}
	}
}

func TestIEWarpAlpha_NoDeadStore(t *testing.T) {
	src := readIEWarpSource(t)
	start := strings.Index(src, "op_blit_convert:")
	end := strings.Index(src[start:], "op_blit_alpha:")
	if start < 0 || end < 0 {
		t.Fatal("op_blit_convert region not found")
	}
	if strings.Contains(src[start:start+end], "store.b r0, 3(r25)") {
		t.Fatal("op_blit_convert still contains dead alpha-channel clear")
	}
}
