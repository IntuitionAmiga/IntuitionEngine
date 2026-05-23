package main

import (
	"os/exec"
	"testing"
)

// TestRunIemonCase_NonZeroExitDowngradesStatus covers the iemon analog
// of the IES P2 fix. The wrapper can print sentinels and pass every
// expected_mode:"ignore" setup step, then exit non-zero (because
// pcall caught a monitor error after the last meaningful step, or
// because the child crashed). Such a case must report FAIL/ERROR, not
// PASS.
func TestRunIemonCase_NonZeroExitDowngradesStatus(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh required")
	}
	c := Case{
		ID:             "fixture-iemon-fail",
		Source:         "fixture.md",
		FenceStartLine: 1,
		Kind:           KindIemon,
		CPU:            "6502",
		Status:         StatusReady,
		IemonSteps:     nil,
	}
	binPath := t.TempDir() + "/fake_ie"
	wrapper := `#!/bin/sh
echo "=== PRM_BEGIN ==="
echo "=== PRM_END ==="
exit 11
`
	if err := writeExecutable(binPath, wrapper); err != nil {
		t.Fatal(err)
	}
	rc := runIemonCase(c, t.TempDir(), t.TempDir(), binPath)
	if rc.Status == RStatusPass {
		t.Fatalf("want FAIL/ERROR when child exits 11, got PASS")
	}
	if rc.Error == "" {
		t.Errorf("want non-empty Error message describing the exit code")
	}
}

// TestRunIemonCase_ZeroExitWithCleanStepsPasses confirms the inverse.
func TestRunIemonCase_ZeroExitWithCleanStepsPasses(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh required")
	}
	c := Case{
		ID:             "fixture-iemon-clean",
		Source:         "fixture.md",
		FenceStartLine: 1,
		Kind:           KindIemon,
		CPU:            "6502",
		Status:         StatusReady,
		IemonSteps:     nil,
	}
	binPath := t.TempDir() + "/fake_ie"
	wrapper := `#!/bin/sh
echo "=== PRM_BEGIN ==="
echo "=== PRM_END ==="
exit 0
`
	if err := writeExecutable(binPath, wrapper); err != nil {
		t.Fatal(err)
	}
	rc := runIemonCase(c, t.TempDir(), t.TempDir(), binPath)
	if rc.Status != RStatusPass {
		t.Fatalf("want PASS, got %q (error=%q)", rc.Status, rc.Error)
	}
}
