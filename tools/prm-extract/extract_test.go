package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempMD writes a markdown fixture to a temp file and returns its
// path + the path of the repo root the test uses (the temp dir).
func writeTempMD(t *testing.T, body string) (path, root string) {
	t.Helper()
	dir := t.TempDir()
	root = dir
	chDir := filepath.Join(dir, "sdk", "docs", "refman.publish")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(chDir, "00-fixture.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestExtractFile_BasicSingleREPL(t *testing.T) {
	path, root := writeTempMD(t, "Intro\n\n```basic\nPRINT 2+3\n 5\nReady\n.\n```\n")
	fences, err := extractFile(path, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fences) != 1 {
		t.Fatalf("want 1 fence, got %d", len(fences))
	}
	kind, _ := classifyFence(fences[0])
	if kind != KindBasic {
		t.Fatalf("kind=%q", kind)
	}
	cases, err := segmentBasic(fences[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || len(cases[0].BasicSteps) != 1 {
		t.Fatalf("want 1 case 1 step, got %+v", cases)
	}
	step := cases[0].BasicSteps[0]
	if step.Input != "PRINT 2+3" {
		t.Errorf("input=%q", step.Input)
	}
	if len(step.Expected) != 1 || step.Expected[0] != " 5" {
		t.Errorf("expected=%v", step.Expected)
	}
}

func TestExtractFile_AssignmentAsInput(t *testing.T) {
	path, root := writeTempMD(t, "```basic\nA = 5\nB$ = \"DOG\"\nA(3) = 99\n```\n")
	fences, _ := extractFile(path, root)
	cases, err := segmentBasic(fences[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(cases[0].BasicSteps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(cases[0].BasicSteps))
	}
	for i, want := range []string{"A = 5", "B$ = \"DOG\"", "A(3) = 99"} {
		if got := cases[0].BasicSteps[i].Input; got != want {
			t.Errorf("step %d: input=%q want %q", i, got, want)
		}
	}
}

func TestExtractFile_LISTOutputNotRetypedAsInput(t *testing.T) {
	body := "```basic\n10 PRINT \"HI\"\n20 PRINT \"BYE\"\nLIST\n10 PRINT \"HI\"\n20 PRINT \"BYE\"\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, err := segmentBasic(fences[0])
	if err != nil {
		t.Fatal(err)
	}
	steps := cases[0].BasicSteps
	if len(steps) != 3 {
		t.Fatalf("want 3 steps (two numbered + LIST), got %d", len(steps))
	}
	if steps[2].Input != "LIST" {
		t.Errorf("step[2] input=%q", steps[2].Input)
	}
	if len(steps[2].Expected) != 2 {
		t.Errorf("LIST expected len=%d (want 2)", len(steps[2].Expected))
	}
}

func TestExtractFile_BareNumberDelete(t *testing.T) {
	body := "```basic\n10 PRINT \"HI\"\n20 PRINT \"BYE\"\n10\nLIST\n20 PRINT \"BYE\"\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if got := cases[0].BasicSteps[2].Input; got != "10" {
		t.Errorf("bare-number delete input=%q want %q", got, "10")
	}
	if cases[0].BasicSteps[2].ExpectsOutput {
		t.Errorf("bare-number delete should not expect output")
	}
}

func TestExtractFile_NumericOnlyOutput(t *testing.T) {
	body := "```basic\nPRINT BIN$(10)\n1010\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	step := cases[0].BasicSteps[0]
	if step.Input != "PRINT BIN$(10)" {
		t.Errorf("input=%q", step.Input)
	}
	if len(step.Expected) != 1 || step.Expected[0] != "1010" {
		t.Errorf("numeric-only output not captured: %v", step.Expected)
	}
}

func TestExtractFile_PromptArtifactsStripped(t *testing.T) {
	body := "```basic\nPRINT 1\n 1\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if len(cases[0].BasicSteps[0].Expected) != 1 {
		t.Errorf("expected blocks should be stripped of Ready/.: %v",
			cases[0].BasicSteps[0].Expected)
	}
}

func TestExtractFile_IndependentMultiGroup(t *testing.T) {
	body := "```basic\nPRINT 1\n 1\nReady\n.\n\nPRINT 2\n 2\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if len(cases) != 2 {
		t.Fatalf("want 2 independent cases, got %d", len(cases))
	}
}

func TestExtractFile_CumulativeOptIn(t *testing.T) {
	body := "<!-- @prm-mode: cumulative -->\n```basic\n10 A=5:B=3\n20 PRINT (A>B)\nRUN\n-1\n\n30 PRINT 10 * (A>B)\nRUN\n-10\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if len(cases) != 1 || cases[0].Mode != BasicModeCumulative {
		t.Fatalf("want 1 cumulative case, got %+v", cases)
	}
}

func TestExtractFile_NeedsAnnotation(t *testing.T) {
	body := "```basic\n10 PRINT 1\nRUN\n 1\n\n20 PRINT 2\nRUN\n 2\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if len(cases) != 1 || cases[0].Status != StatusSkip || cases[0].SkipReason != "needs-annotation" {
		t.Fatalf("want needs-annotation skip, got %+v", cases)
	}
}

func TestExtractFile_IemonCPUAutoDetect(t *testing.T) {
	body := "```\n(6502)> r\nA: $00 X: $00\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	kind, cpu := classifyFence(fences[0])
	if kind != KindIemon || cpu != "6502" {
		t.Fatalf("want iemon/6502, got %s/%s", kind, cpu)
	}
}

func TestExtractFile_IemonExplicitCPUWithBarePrompt(t *testing.T) {
	body := "<!-- @prm-cpu: 6502 -->\n```text\n> r\nA: $00\n> m 0\n0000: AA\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	kind, cpu := classifyFence(fences[0])
	if kind != KindIemon || cpu != "6502" {
		t.Fatalf("want iemon/6502, got %s/%s", kind, cpu)
	}
	cases, err := segmentIemon(fences[0], cpu)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases[0].IemonSteps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(cases[0].IemonSteps))
	}
}

func TestExtractFile_TextFenceOptOutOfIemonAutoDetect(t *testing.T) {
	body := "```text\n(ie64)> A 1000\nIE64 assemble at $0000000000001000; empty line exits\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	kind, cpu := classifyFence(fences[0])
	if kind != "" || cpu != "" {
		t.Fatalf("want text opt-out, got %s/%s", kind, cpu)
	}
}

func TestExtractFile_IemonMixedCPURejected(t *testing.T) {
	body := "```\n(6502)> r\n(z80)> r\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	kind, cpu := classifyFence(fences[0])
	cases, _ := segmentIemon(fences[0], cpu)
	if kind != KindIemon || cases[0].Status != StatusSkip || cases[0].SkipReason != "iemon-mixed-cpu" {
		t.Fatalf("want iemon mixed-cpu skip, got %+v", cases)
	}
}

func TestExtractFile_IemonGoClassification(t *testing.T) {
	tests := []struct {
		body     string
		wantKind string
		wantSkip string
		wantBR   bool
	}{
		{"```\n(6502)> g\n```\n", IemonStepGo, "", false},
		{"```\n(6502)> g\nBREAK at $1234\n```\n", IemonStepSkip, "iemon-g-postrun-capture-unsupported", true},
		{"```\n(6502)> g $1234\n```\n", IemonStepSkip, "iemon-g-addr-needs-validated-go-api", true},
		{"```\n(6502)> u $1234\n```\n", IemonStepSkip, "iemon-u-needs-core-api", true},
		{"```\n(6502)> save foo.bin\n```\n", IemonStepSkip, "iemon-host-file-command", true},
		{"```\n(6502)> trace file off\n```\n", IemonStepInspect, "", false},
		{"```\n(6502)> trace file foo.log\n```\n", IemonStepSkip, "iemon-host-file-command", false},
		{"```\n(6502)> s\n```\n", IemonStepStep, "", false},
		{"```\n(6502)> bs\n```\n", IemonStepBackstep, "", false},
		{"```\n(6502)> r\n```\n", IemonStepInspect, "", false},
	}
	for i, tc := range tests {
		path, root := writeTempMD(t, tc.body)
		fences, _ := extractFile(path, root)
		_, cpu := classifyFence(fences[0])
		cases, err := segmentIemon(fences[0], cpu)
		if err != nil {
			t.Fatalf("[%d] %v", i, err)
		}
		if len(cases) != 1 || len(cases[0].IemonSteps) != 1 {
			t.Fatalf("[%d] bad case shape: %+v", i, cases)
		}
		s := cases[0].IemonSteps[0]
		if s.Kind != tc.wantKind {
			t.Errorf("[%d] kind=%q want %q", i, s.Kind, tc.wantKind)
		}
		if s.SkipReason != tc.wantSkip {
			t.Errorf("[%d] skip_reason=%q want %q", i, s.SkipReason, tc.wantSkip)
		}
		if s.BlocksRest != tc.wantBR {
			t.Errorf("[%d] blocks_rest=%v want %v", i, s.BlocksRest, tc.wantBR)
		}
	}
}

func TestExtractFile_IemonSetupAllowlist(t *testing.T) {
	// w (memory write, in allowlist) + empty expected → ignore mode.
	body := "```\n(6502)> w 1000 EA\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	_, cpu := classifyFence(fences[0])
	cases, _ := segmentIemon(fences[0], cpu)
	if cases[0].IemonSteps[0].ExpectedMode != ExpectedIgnore {
		t.Errorf("allowlist-w empty-expected should be ignore, got %q",
			cases[0].IemonSteps[0].ExpectedMode)
	}
	// m (memory dump, NOT in allowlist) + empty expected → strict (intentional FAIL).
	body2 := "```\n(6502)> m 1000\n```\n"
	path2, root2 := writeTempMD(t, body2)
	fences2, _ := extractFile(path2, root2)
	cases2, _ := segmentIemon(fences2[0], "6502")
	if cases2[0].IemonSteps[0].ExpectedMode != ExpectedStrict {
		t.Errorf("non-allowlist empty-expected should be strict, got %q",
			cases2[0].IemonSteps[0].ExpectedMode)
	}
}

func TestExtractFile_DuplicateIDDetected(t *testing.T) {
	dir := t.TempDir()
	chDir := filepath.Join(dir, "sdk", "docs", "refman.publish")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "```basic\nPRINT 2+3\n 5\nReady\n.\n```\n\n```basic\nPRINT 2+3\n 5\nReady\n.\n```\n"
	if err := os.WriteFile(filepath.Join(chDir, "00-dup.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := extractAll(chDir, "00-dup", dir)
	if err == nil {
		t.Fatal("want duplicate-id error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate case IDs") {
		t.Errorf("error doesn't mention duplicate IDs: %v", err)
	}
}

func TestExtractFile_IESLintPasses(t *testing.T) {
	body := "```ies\nsys.print(\"hi\")\nsys.exit(0)\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, err := segmentIES(fences[0])
	if err != nil {
		t.Fatal(err)
	}
	if cases[0].Mode != IESModeLint || cases[0].APILint == nil ||
		cases[0].APILint.Status != "PASS" {
		t.Fatalf("want lint PASS, got %+v", cases)
	}
}

func TestExtractFile_IESLintFlagsStaleAPI(t *testing.T) {
	body := "```ies\nsys.sleep_frames(2)\nvideo.snapshot()\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentIES(fences[0])
	if cases[0].APILint == nil || cases[0].APILint.Status != "FAIL" {
		t.Fatalf("want lint FAIL on stale API, got %+v", cases[0].APILint)
	}
}

func TestExtractFile_IESPrmIdStable(t *testing.T) {
	body := "<!-- @prm-id: my-name -->\n```basic\nPRINT 1\n 1\nReady\n.\n```\n"
	path, root := writeTempMD(t, body)
	fences, _ := extractFile(path, root)
	cases, _ := segmentBasic(fences[0])
	if cases[0].ID != "my-name" {
		t.Errorf("want id 'my-name', got %q", cases[0].ID)
	}
}

func TestExtractFile_StableContentHash(t *testing.T) {
	// Identical body → identical ID, regardless of surrounding prose.
	bodyA := "Some prose here.\n\n```basic\nPRINT 7\n 7\nReady\n.\n```\n"
	bodyB := "Very different prose.\n\n```basic\nPRINT 7\n 7\nReady\n.\n```\n"
	pA, rA := writeTempMD(t, bodyA)
	pB, rB := writeTempMD(t, bodyB)
	fa, _ := extractFile(pA, rA)
	fb, _ := extractFile(pB, rB)
	ca, _ := segmentBasic(fa[0])
	cb, _ := segmentBasic(fb[0])
	if ca[0].ID != cb[0].ID {
		t.Errorf("ID drift: %s != %s", ca[0].ID, cb[0].ID)
	}
}
