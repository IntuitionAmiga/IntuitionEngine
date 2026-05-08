package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ab3d2AssetDirName  = "ab3d2_source/_build"
	ab3d2AssetStampRel = "ab3d2_source/_build/.intuitionengine-ab3d2-assets"
)

func ensureEmbeddedAB3D2Assets() (string, error) {
	if len(embeddedAB3D2AssetZip) == 0 {
		return "", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	exeDir := filepath.Dir(exe)
	if err := ensureEmbeddedAB3D2AssetsInDir(embeddedAB3D2AssetZip, exeDir); err != nil {
		return "", err
	}
	return exeDir, nil
}

func ensureEmbeddedAB3D2AssetsInDir(assetZip []byte, exeDir string) error {
	targetDir := filepath.Join(exeDir, ab3d2AssetDirName)
	stampPath := filepath.Join(exeDir, filepath.FromSlash(ab3d2AssetStampRel))
	if st, err := os.Stat(stampPath); err == nil && !st.IsDir() {
		if err := verifyAB3D2BuildAssets(exeDir); err == nil {
			return os.Chdir(exeDir)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check AB3D2 asset stamp: %w", err)
	}
	if err := removeLegacyStampedAB3D2BuildDir(exeDir); err != nil {
		return err
	}
	if err := removeAB3D2BuildDir(exeDir); err != nil {
		return err
	}
	if err := extractZipBytes(assetZip, exeDir); err != nil {
		return fmt.Errorf("extract embedded AB3D2 assets: %w", err)
	}
	if st, err := os.Stat(targetDir); err != nil || !st.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return fmt.Errorf("verify embedded AB3D2 asset directory: %w", err)
	}
	if err := verifyAB3D2BuildAssets(exeDir); err != nil {
		return err
	}
	if err := os.WriteFile(stampPath, []byte("IntuitionEngine AB3D2 _build assets\n"), 0o644); err != nil {
		return fmt.Errorf("write AB3D2 asset stamp: %w", err)
	}
	return os.Chdir(exeDir)
}

func verifyAB3D2BuildAssets(exeDir string) error {
	for _, rel := range []string{
		"ab3d2_source/_build/ie_unpacked",
		"ab3d2_source/_build/ie_media/redux-high",
	} {
		path := filepath.Join(exeDir, filepath.FromSlash(rel))
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			if err == nil {
				err = fmt.Errorf("not a directory")
			}
			return fmt.Errorf("verify embedded AB3D2 asset path %s: %w", rel, err)
		}
	}
	return nil
}

func removeAB3D2BuildDir(exeDir string) error {
	cleanExeDir, err := filepath.Abs(exeDir)
	if err != nil {
		return fmt.Errorf("resolve AB3D2 asset directory: %w", err)
	}
	targetDir := filepath.Join(cleanExeDir, filepath.FromSlash(ab3d2AssetDirName))
	if filepath.Base(targetDir) != "_build" || filepath.Base(filepath.Dir(targetDir)) != "ab3d2_source" {
		return fmt.Errorf("refusing to remove unexpected AB3D2 asset path: %s", targetDir)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("replace stale AB3D2 _build directory: %w", err)
	}
	return nil
}

func removeLegacyStampedAB3D2BuildDir(exeDir string) error {
	cleanExeDir, err := filepath.Abs(exeDir)
	if err != nil {
		return fmt.Errorf("resolve AB3D2 legacy asset directory: %w", err)
	}
	legacyDir := filepath.Join(cleanExeDir, "_build")
	stampPath := filepath.Join(legacyDir, ".intuitionengine-ab3d2-assets")
	if st, err := os.Stat(stampPath); err == nil && !st.IsDir() {
		if err := os.RemoveAll(legacyDir); err != nil {
			return fmt.Errorf("remove legacy AB3D2 _build directory: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check legacy AB3D2 asset stamp: %w", err)
	}
	return nil
}

func extractZipBytes(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	cleanDest, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	cleanDest = filepath.Clean(cleanDest)
	for _, f := range reader.File {
		if err := extractZipEntry(f, cleanDest); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destDir string) error {
	name := filepath.Clean(filepath.FromSlash(f.Name))
	if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
		return fmt.Errorf("unsafe zip entry path: %s", f.Name)
	}
	target := filepath.Join(destDir, name)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("zip entry escapes destination: %s", f.Name)
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	mode := f.FileInfo().Mode()
	if mode == 0 {
		mode = 0o644
	}
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}
