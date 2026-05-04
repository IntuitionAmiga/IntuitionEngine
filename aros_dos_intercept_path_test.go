package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newTestArosDOSDevice(t *testing.T) (*MachineBus, *ArosDOSDevice, string) {
	t.Helper()
	root := t.TempDir()
	bus, err := NewMachineBusSized(32 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	d, err := NewArosDOSDevice(bus, root)
	if err != nil {
		t.Fatalf("NewArosDOSDevice: %v", err)
	}
	return bus, d, root
}

func TestArosDOSPath_HelpersContainmentAndSymlinkPolicy(t *testing.T) {
	_, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "foo"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile foo: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("Mkdir dir: %v", err)
	}
	if err := os.Symlink("foo", filepath.Join(root, "linkIn")); err != nil {
		t.Fatalf("Symlink linkIn: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "linkOut")); err != nil {
		t.Fatalf("Symlink linkOut: %v", err)
	}
	sibling := filepath.Join(filepath.Dir(root), filepath.Base(root)+"2")
	if err := os.MkdirAll(filepath.Join(sibling, "etc"), 0o755); err != nil {
		t.Fatalf("Mkdir sibling: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "etc", "file"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile sibling: %v", err)
	}

	for _, tc := range []struct {
		name string
		fn   func(string, string) (string, resolveReason)
	}{
		{"openRead", d.resolveOpenReadPath},
		{"source", d.resolveSourcePath},
		{"update", d.resolveUpdatePath},
		{"create", d.resolveCreatePath},
	} {
		t.Run(tc.name+" rejects escapes", func(t *testing.T) {
			for _, leaf := range []string{"..", "../etc", "../" + filepath.Base(sibling) + "/etc/file", "/etc/passwd"} {
				if _, reason := tc.fn(root, leaf); reason != resolveNotFound {
					t.Fatalf("%s(%q) reason=%v, want resolveNotFound", tc.name, leaf, reason)
				}
			}
		})
	}

	if got, reason := d.resolveOpenReadPath(root, "linkIn"); reason != resolveOK || got != filepath.Join(root, "foo") {
		t.Fatalf("open-read in-root symlink got (%q,%v), want resolved foo/OK", got, reason)
	}
	if _, reason := d.resolveOpenReadPath(root, "linkOut"); reason != resolveNotFound {
		t.Fatalf("open-read external symlink reason=%v, want resolveNotFound", reason)
	}
	if got, reason := d.resolveSourcePath(root, "linkOut"); reason != resolveOK || got != filepath.Join(root, "linkOut") {
		t.Fatalf("source external symlink got (%q,%v), want link itself/OK", got, reason)
	}
	if _, reason := d.resolveUpdatePath(root, "linkIn"); reason != resolveWrongType {
		t.Fatalf("update symlink reason=%v, want resolveWrongType", reason)
	}
	if _, reason := d.resolveCreatePath(root, "linkIn"); reason != resolveWrongType {
		t.Fatalf("create symlink reason=%v, want resolveWrongType", reason)
	}
	if _, reason := d.resolveCreatePath(root, "foo"); reason != resolveExists {
		t.Fatalf("create existing regular reason=%v, want resolveExists", reason)
	}
	if _, reason := d.resolveCreatePath(root, "missing"); reason != resolveOK {
		t.Fatalf("create missing reason=%v, want resolveOK", reason)
	}
}

func TestArosDOSPath_HelpersSpecialNodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX special-node semantics")
	}
	_, d, root := newTestArosDOSDevice(t)
	fifo := filepath.Join(root, "fifo")
	if err := makeTestFIFO(fifo); err != nil {
		t.Skipf("FIFO not available: %v", err)
	}
	if _, reason := d.resolveOpenReadPath(root, "fifo"); reason != resolveOK {
		t.Fatalf("open-read FIFO reason=%v, want resolveOK", reason)
	}
	if _, reason := d.resolveSourcePath(root, "fifo"); reason != resolveWrongType {
		t.Fatalf("source FIFO reason=%v, want resolveWrongType", reason)
	}
	if _, reason := d.resolveUpdatePath(root, "fifo"); reason != resolveWrongType {
		t.Fatalf("update FIFO reason=%v, want resolveWrongType", reason)
	}
	if _, reason := d.resolveCreatePath(root, "fifo"); reason != resolveWrongType {
		t.Fatalf("create FIFO reason=%v, want resolveWrongType", reason)
	}
}

func TestArosDOSPath_OpenFlagsByCommandClass(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "in"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile in: %v", err)
	}
	writeArosDOSString(t, bus, 0x1000, "in")
	writeArosDOSString(t, bus, 0x1100, "out")

	var calls []int
	oldOpen := arosOpenFile
	arosOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		calls = append(calls, flag)
		return oldOpen(name, flag, perm)
	}
	t.Cleanup(func() { arosOpenFile = oldOpen })

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDINPUT, 0x1000, 0); res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDINPUT res2=%d", res2)
	}
	if len(calls) != 1 || calls[0]&arosOpenNoFollow != 0 {
		t.Fatalf("FINDINPUT flags=%v, want no O_NOFOLLOW", calls)
	}

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDUPDATE, 0x1000, 0); res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDUPDATE res2=%d", res2)
	}
	if arosOpenNoFollow != 0 && calls[len(calls)-1]&arosOpenNoFollow == 0 {
		t.Fatalf("FINDUPDATE flags=0x%X, want O_NOFOLLOW", calls[len(calls)-1])
	}

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDOUTPUT, 0x1100, 0); res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDOUTPUT res2=%d", res2)
	}
	if arosOpenNoFollow != 0 && calls[len(calls)-1]&arosOpenNoFollow == 0 {
		t.Fatalf("FINDOUTPUT flags=0x%X, want O_NOFOLLOW", calls[len(calls)-1])
	}
}
