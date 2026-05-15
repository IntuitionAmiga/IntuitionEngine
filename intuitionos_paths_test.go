package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntuitionOSResolverDevDefaults(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, intuitionOSDevRootRel)
	image := filepath.Join(base, intuitionOSDevImageRel)
	mustMkdirAll(t, root)
	mustWriteTestFile(t, image, "kernel")

	got, err := resolveIntuitionOSPaths(intuitionOSPathOptions{WorkDir: base, RequireKernel: true})
	if err != nil {
		t.Fatalf("resolveIntuitionOSPaths: %v", err)
	}
	if got.Root != root {
		t.Fatalf("root=%q, want %q", got.Root, root)
	}
	if got.Image != image {
		t.Fatalf("image=%q, want %q", got.Image, image)
	}
}

func TestIntuitionOSResolverLiveDefaults(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, intuitionOSLiveRootRel)
	image := filepath.Join(base, intuitionOSLiveImageRel)
	mustMkdirAll(t, root)
	mustWriteTestFile(t, image, "kernel")

	got, err := resolveIntuitionOSPaths(intuitionOSPathOptions{WorkDir: base, RequireKernel: true})
	if err != nil {
		t.Fatalf("resolveIntuitionOSPaths: %v", err)
	}
	if got.Root != root {
		t.Fatalf("root=%q, want %q", got.Root, root)
	}
	if got.Image != image {
		t.Fatalf("image=%q, want %q", got.Image, image)
	}
}

func TestIntuitionOSResolverEnvOverridesOnlyRoot(t *testing.T) {
	base := t.TempDir()
	envRoot := filepath.Join(base, "custom-sys")
	image := filepath.Join(base, intuitionOSDevImageRel)
	mustMkdirAll(t, envRoot)
	mustWriteTestFile(t, image, "kernel")
	t.Setenv("INTUITIONOS_HOST_ROOT", envRoot)

	got, err := resolveIntuitionOSPaths(intuitionOSPathOptions{WorkDir: base, RequireKernel: true})
	if err != nil {
		t.Fatalf("resolveIntuitionOSPaths: %v", err)
	}
	if got.Root != envRoot {
		t.Fatalf("root=%q, want %q", got.Root, envRoot)
	}
	if got.Image != image {
		t.Fatalf("image=%q, want %q", got.Image, image)
	}
}

func TestIntuitionOSResolverExplicitFlagsOverrideDefaults(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "flag-root")
	image := filepath.Join(base, "flag-kernel.ie64")
	mustMkdirAll(t, root)
	mustWriteTestFile(t, image, "kernel")

	got, err := resolveIntuitionOSPaths(intuitionOSPathOptions{
		WorkDir:       base,
		ExplicitRoot:  root,
		ExplicitImage: image,
		RequireKernel: true,
	})
	if err != nil {
		t.Fatalf("resolveIntuitionOSPaths: %v", err)
	}
	if got.Root != root {
		t.Fatalf("root=%q, want %q", got.Root, root)
	}
	if got.Image != image {
		t.Fatalf("image=%q, want %q", got.Image, image)
	}
}

func TestIntuitionOSResolverMissingKernelExplainsMakeTarget(t *testing.T) {
	base := t.TempDir()
	mustMkdirAll(t, filepath.Join(base, intuitionOSDevRootRel))

	_, err := resolveIntuitionOSPaths(intuitionOSPathOptions{
		WorkDir:       base,
		ExplicitImage: filepath.Join(base, "missing.ie64"),
		RequireKernel: true,
	})
	if err == nil {
		t.Fatalf("resolveIntuitionOSPaths succeeded with missing kernel")
	}
	if !strings.Contains(err.Error(), "run 'make intuitionos'") {
		t.Fatalf("error %q does not mention make intuitionos", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
