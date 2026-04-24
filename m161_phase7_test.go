package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestIExec_M161_Phase7_BootBannerUsesIOSVersion(t *testing.T) {
	body := string(mustReadRepoBytes(t, filepath.Join("sdk", "intuitionos", "iexec", "boot", "strings.s")))
	if !bytes.Contains([]byte(body), []byte(`"exec.library 1.16.1 boot"`)) {
		t.Fatalf("boot banner must use IOS version 1.16.1, got:\n%s", body)
	}
	if bytes.Contains([]byte(body), []byte(`"exec.library M11 boot"`)) {
		t.Fatalf("boot banner still contains stale M11 label")
	}
}

func TestIExec_M161_Phase7_HelpHeaderUsesIOSVersion(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("sdk", "intuitionos", "iexec", "cmd", "help.s"),
		filepath.Join("sdk", "intuitionos", "iexec", "assets", "system", "S", "Help"),
	} {
		body := string(mustReadRepoBytes(t, rel))
		if !bytes.Contains([]byte(body), []byte("IntuitionOS 1.16.1 help")) {
			t.Fatalf("%s help header must use IOS version 1.16.1, got:\n%s", rel, body)
		}
		if bytes.Contains([]byte(body), []byte("M15 help surface")) {
			t.Fatalf("%s help header still contains stale M15 label", rel)
		}
	}
}

func TestIExec_M161_Phase7_VERSIONUsesIOSVersion(t *testing.T) {
	body := string(mustReadRepoBytes(t, filepath.Join("sdk", "intuitionos", "iexec", "cmd", "version.s")))
	if !bytes.Contains([]byte(body), []byte("IntuitionOS 1.16.1")) {
		t.Fatalf("VERSION command must use IOS version 1.16.1, got:\n%s", body)
	}
	if bytes.Contains([]byte(body), []byte("IntuitionOS 0.18")) {
		t.Fatalf("VERSION command still contains stale 0.18 label")
	}
}
