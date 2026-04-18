package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapHostFSSpecialFileRegistrationClonesBytes(t *testing.T) {
	dev := NewBootstrapHostFSDevice(nil, "")
	orig := []byte{0x7f, 'E', 'L', 'F'}

	dev.SetSpecialFile("IOSSYS/Tools/Shell", orig)
	orig[0] = 0

	got, ok := dev.specialFile("IOSSYS/Tools/Shell")
	if !ok {
		t.Fatal("special file not registered")
	}
	if len(got) != 4 || got[0] != 0x7f || got[1] != 'E' || got[2] != 'L' || got[3] != 'F' {
		t.Fatalf("special file bytes mutated with source slice: %v", got)
	}
}

func TestBootstrapHostFSResolveRelativePathRejectsSymlinkedMiddleComponent(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Real"), 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Real", "payload.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := os.Symlink("Real", filepath.Join(root, "Link")); err != nil {
		t.Fatalf("symlink link->real: %v", err)
	}

	dev := NewBootstrapHostFSDevice(nil, root)
	if !dev.available {
		t.Fatal("device not available")
	}

	if _, errCode := dev.resolveRelativePath("Link/payload.txt"); errCode != 5 {
		t.Fatalf("resolveRelativePath through middle symlink err=%d, want 5", errCode)
	}
}

func TestBootstrapHostFSResolveForCreateRejectsSymlinkedMiddleComponent(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Target"), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink("Target", filepath.Join(root, "Link")); err != nil {
		t.Fatalf("symlink link->target: %v", err)
	}

	dev := NewBootstrapHostFSDevice(nil, root)
	if !dev.available {
		t.Fatal("device not available")
	}

	if _, errCode := dev.resolveForCreate("Link/newfile.txt"); errCode != 5 {
		t.Fatalf("resolveForCreate through middle symlink err=%d, want 5", errCode)
	}
}

func TestBootstrapHostFSResolveRelativePathCanonicalizesCaseVariants(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "MiXeD")
	if err := os.Mkdir(wantDir, 0o755); err != nil {
		t.Fatalf("mkdir mixed: %v", err)
	}
	wantPath := filepath.Join(wantDir, "Name.Txt")
	if err := os.WriteFile(wantPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dev := NewBootstrapHostFSDevice(nil, root)
	if !dev.available {
		t.Fatal("device not available")
	}

	gotA, errCode := dev.resolveRelativePath("mixed/name.txt")
	if errCode != 0 {
		t.Fatalf("resolve lowercase err=%d", errCode)
	}
	gotB, errCode := dev.resolveRelativePath("MIXED/NAME.TXT")
	if errCode != 0 {
		t.Fatalf("resolve uppercase err=%d", errCode)
	}
	if gotA != wantPath {
		t.Fatalf("lowercase path=%q, want canonical %q", gotA, wantPath)
	}
	if gotB != wantPath {
		t.Fatalf("uppercase path=%q, want canonical %q", gotB, wantPath)
	}
	if gotA != gotB {
		t.Fatalf("case variants resolved differently: %q vs %q", gotA, gotB)
	}
}

func TestBootstrapHostFSResolveForCreateCanonicalizesExistingLeafCase(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "MiXeD")
	if err := os.Mkdir(wantDir, 0o755); err != nil {
		t.Fatalf("mkdir mixed: %v", err)
	}
	wantPath := filepath.Join(wantDir, "Name.Txt")
	if err := os.WriteFile(wantPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dev := NewBootstrapHostFSDevice(nil, root)
	if !dev.available {
		t.Fatal("device not available")
	}

	got, errCode := dev.resolveForCreate("mixed/name.txt")
	if errCode != 0 {
		t.Fatalf("resolveForCreate err=%d", errCode)
	}
	if got != wantPath {
		t.Fatalf("resolveForCreate path=%q, want canonical %q", got, wantPath)
	}
}
