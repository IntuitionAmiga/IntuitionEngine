package main

import (
	"os/exec"
	"testing"
)

// TestRunIESCase_NonZeroExitFailsEvenIfStdoutMatches exercises the P2
// review finding: a snippet that prints its expected stdout but signals
// failure via a non-zero exit must be reported as FAIL, not PASS.
//
// We swap the real `bin/IntuitionEngine` for a /bin/sh wrapper that
// mimics the sentinels + a custom exit code, then check the result.
func TestRunIESCase_NonZeroExitFailsEvenIfStdoutMatches(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh required")
	}
	c := Case{
		ID:             "fixture-nonzero",
		Source:         "fixture.md",
		FenceStartLine: 1,
		Kind:           KindIES,
		ExpectedStdout: "HELLO",
		IESTimeoutS:    5,
	}
	binPath := t.TempDir() + "/fake_ie"
	wrapper := `#!/bin/sh
echo "=== PRM_BEGIN ==="
echo HELLO
echo "=== PRM_END ==="
exit 7
`
	if err := writeExecutable(binPath, wrapper); err != nil {
		t.Fatal(err)
	}
	rc := runIESCase(c, t.TempDir(), t.TempDir(), binPath)
	if rc.Status != RStatusFail {
		t.Fatalf("want FAIL when child exits 7, got %q (error=%q)", rc.Status, rc.Error)
	}
	if rc.Error == "" {
		t.Errorf("want non-empty Error message describing the exit code")
	}
}

// TestRunIESCase_ZeroExitPassesWhenStdoutMatches confirms the inverse:
// matching stdout + clean exit still passes.
func TestRunIESCase_ZeroExitPassesWhenStdoutMatches(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh required")
	}
	c := Case{
		ID:             "fixture-clean",
		Source:         "fixture.md",
		FenceStartLine: 1,
		Kind:           KindIES,
		ExpectedStdout: "HELLO",
		IESTimeoutS:    5,
	}
	binPath := t.TempDir() + "/fake_ie"
	wrapper := `#!/bin/sh
echo "=== PRM_BEGIN ==="
echo HELLO
echo "=== PRM_END ==="
exit 0
`
	if err := writeExecutable(binPath, wrapper); err != nil {
		t.Fatal(err)
	}
	rc := runIESCase(c, t.TempDir(), t.TempDir(), binPath)
	if rc.Status != RStatusPass {
		t.Fatalf("want PASS, got %q (error=%q)", rc.Status, rc.Error)
	}
}

func writeExecutable(path, body string) error {
	if err := writeFile(path, body); err != nil {
		return err
	}
	return makeExecutable(path)
}
