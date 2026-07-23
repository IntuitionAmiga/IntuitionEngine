package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// buildAB3D2TestZip returns a minimal asset archive whose layout satisfies
// verifyAB3D2BuildAssets: it contains the two directories the verifier checks
// for, each with one file so the directory is materialised on extraction. The
// marker byte lets a caller produce two archives that differ only in content.
func buildAB3D2TestZip(t *testing.T, marker byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{
		"ab3d2_source/_build/ie_unpacked/level",
		"ab3d2_source/_build/ie_media/redux-high/tex",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte{marker}); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAssetStamp_ContentAddressedStable pins the stamp contract: the stamp is a
// pure function of the archive bytes, stable for identical bytes and different
// for any change. This is what lets the stamp alone decide whether extracted
// assets are still current.
func TestAssetStamp_ContentAddressedStable(t *testing.T) {
	a := buildAB3D2TestZip(t, 0x01)
	b := buildAB3D2TestZip(t, 0x02)

	if s1, s2 := ab3d2AssetStampContent(a), ab3d2AssetStampContent(a); s1 != s2 {
		t.Fatalf("stamp not stable for identical bytes: %q vs %q", s1, s2)
	}
	if ab3d2AssetStampContent(a) == ab3d2AssetStampContent(b) {
		t.Fatal("stamp collides for differing archive bytes")
	}
	if got := ab3d2AssetStampContent(a); !bytes.HasPrefix([]byte(got), []byte("IntuitionEngine AB3D2 _build assets\nsha256:")) {
		t.Fatalf("stamp missing content-addressed header: %q", got)
	}
}

// TestAssetStamp_ChangedEmbedForcesReextract proves the stamp gates
// regeneration: extracting the same archive twice leaves a sentinel written
// between the runs untouched, but extracting a changed archive wipes the build
// directory and re-extracts. ensureEmbeddedAB3D2AssetsInDir changes the working
// directory on success, so the original is saved and restored.
func TestAssetStamp_ChangedEmbedForcesReextract(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	dir := t.TempDir()
	zipA := buildAB3D2TestZip(t, 0x01)

	if err := ensureEmbeddedAB3D2AssetsInDir(zipA, dir); err != nil {
		t.Fatalf("first extract: %v", err)
	}
	sentinel := filepath.Join(dir, "ab3d2_source", "_build", "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Same archive: stamp matches, assets verify, so nothing is re-extracted and
	// the sentinel survives.
	if err := ensureEmbeddedAB3D2AssetsInDir(zipA, dir); err != nil {
		t.Fatalf("second extract (same archive): %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("unchanged embed re-extracted assets (sentinel gone): %v", err)
	}

	// Changed archive: stamp differs, the build directory is replaced and the
	// sentinel is gone.
	zipB := buildAB3D2TestZip(t, 0x02)
	if err := ensureEmbeddedAB3D2AssetsInDir(zipB, dir); err != nil {
		t.Fatalf("third extract (changed archive): %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("changed embed did not force re-extraction (sentinel survived), err=%v", err)
	}
	stamp, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ab3d2AssetStampRel)))
	if err != nil {
		t.Fatal(err)
	}
	if string(stamp) != ab3d2AssetStampContent(zipB) {
		t.Fatal("stamp on disk does not match the re-extracted archive")
	}
}
