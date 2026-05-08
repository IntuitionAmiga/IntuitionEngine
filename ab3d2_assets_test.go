package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipBytesWritesNestedFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("ab3d2_source/_build/ie_media/redux-high/includes/text_file")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("asset")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := extractZipBytes(buf.Bytes(), dir); err != nil {
		t.Fatalf("extractZipBytes: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ab3d2_source", "_build", "ie_media", "redux-high", "includes", "text_file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "asset" {
		t.Fatalf("extracted file = %q, want asset", got)
	}
}

func TestExtractZipBytesRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := extractZipBytes(buf.Bytes(), dir); err == nil {
		t.Fatal("expected path traversal zip entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escape.txt")); err == nil {
		t.Fatal("path traversal entry was written outside destination")
	}
}

func TestEnsureEmbeddedAB3D2AssetsExtractsBuildZip(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})

	if err := ensureEmbeddedAB3D2AssetsInDir(testAB3D2BuildZip(t), dir); err != nil {
		t.Fatalf("ensureEmbeddedAB3D2AssetsInDir: %v", err)
	}

	assertTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "redux")
	assertTestFile(t, dir, "ab3d2_source/_build/ie_unpacked/media/includes/panelraw", "unpacked")
	assertTestFile(t, dir, ab3d2AssetStampRel, "IntuitionEngine AB3D2 _build assets\n")
}

func TestEnsureEmbeddedAB3D2AssetsSkipsStampedBuildDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "existing")
	writeTestFile(t, dir, "ab3d2_source/_build/ie_unpacked/media/includes/panelraw", "existing")
	writeTestFile(t, dir, ab3d2AssetStampRel, "stamp")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})

	if err := ensureEmbeddedAB3D2AssetsInDir([]byte("not a zip"), dir); err != nil {
		t.Fatalf("stamped _build dir should skip extraction: %v", err)
	}

	assertTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "existing")
}

func TestEnsureEmbeddedAB3D2AssetsReplacesUnstampedBuildDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "stale")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})

	if err := ensureEmbeddedAB3D2AssetsInDir(testAB3D2BuildZip(t), dir); err != nil {
		t.Fatalf("ensureEmbeddedAB3D2AssetsInDir: %v", err)
	}

	assertTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "redux")
	assertTestFile(t, dir, "ab3d2_source/_build/ie_unpacked/media/includes/panelraw", "unpacked")
}

func TestEnsureEmbeddedAB3D2AssetsRemovesLegacyStampedBuildDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "_build/.intuitionengine-ab3d2-assets", "legacy")
	writeTestFile(t, dir, "_build/ie_media/redux-high/includes/text_file", "stale")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})

	if err := ensureEmbeddedAB3D2AssetsInDir(testAB3D2BuildZip(t), dir); err != nil {
		t.Fatalf("ensureEmbeddedAB3D2AssetsInDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "_build")); !os.IsNotExist(err) {
		t.Fatalf("legacy _build should be removed, stat err = %v", err)
	}
	assertTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "redux")
}

func TestEnsureEmbeddedAB3D2AssetsRejectsZipWithoutBuildRoot(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("other-root/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("wrong")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := ensureEmbeddedAB3D2AssetsInDir(buf.Bytes(), dir); err == nil {
		t.Fatal("expected zip without ab3d2_source/_build root to fail")
	}
}

func TestAB3D2RuntimeBaseDirReadsBuildAsset(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "ab3d2_source/_build/ie_media/redux-high/includes/text_file", "asset")

	bus := NewMachineBus()
	fio := NewFileIODevice(bus, dir)
	nameAddr := uint32(0x1000)
	dataAddr := uint32(0x2000)
	name := "ab3d2_source/_build/ie_media/redux-high/includes/text_file"
	for i, b := range append([]byte(name), 0) {
		bus.Write8(nameAddr+uint32(i), b)
	}

	fio.HandleWrite(FILE_NAME_PTR, nameAddr)
	fio.HandleWrite(FILE_DATA_PTR, dataAddr)
	fio.HandleWrite(FILE_CTRL, FILE_OP_READ)

	if got := fio.HandleRead(FILE_STATUS); got != 0 {
		t.Fatalf("status: got %d error=%d", got, fio.HandleRead(FILE_ERROR_CODE))
	}
	if got := fio.HandleRead(FILE_RESULT_LEN); got != 5 {
		t.Fatalf("result len: got %d, want 5", got)
	}
	got := make([]byte, 5)
	for i := range got {
		got[i] = bus.Read8(dataAddr + uint32(i))
	}
	if string(got) != "asset" {
		t.Fatalf("read asset = %q, want asset", got)
	}
}

func testAB3D2BuildZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range []struct {
		name string
		data string
	}{
		{"ab3d2_source/_build/ie_media/redux-high/includes/text_file", "redux"},
		{"ab3d2_source/_build/ie_unpacked/media/includes/panelraw", "unpacked"},
	} {
		w, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTestFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t *testing.T, root, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", rel, got, want)
	}
}
