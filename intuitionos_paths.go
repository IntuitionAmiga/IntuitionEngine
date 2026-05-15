package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	intuitionOSDevRootRel   = "sdk/intuitionos/system/SYS"
	intuitionOSDevImageRel  = "sdk/intuitionos/iexec/iexec.ie64"
	intuitionOSLiveRootRel  = "Systems/IntuitionOS"
	intuitionOSLiveImageRel = "Systems/IntuitionOS/Boot/iexec.ie64"
)

type intuitionOSPathOptions struct {
	ExplicitRoot  string
	ExplicitImage string
	Executable    string
	WorkDir       string
	RequireKernel bool
}

type intuitionOSPaths struct {
	Root  string
	Image string
}

func resolveIntuitionOSPaths(opts intuitionOSPathOptions) (intuitionOSPaths, error) {
	wd := opts.WorkDir
	if wd == "" {
		if got, err := os.Getwd(); err == nil {
			wd = got
		}
	}
	exe := opts.Executable
	if exe == "" {
		exe, _ = os.Executable()
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	root := chooseIntuitionOSRoot(opts.ExplicitRoot, wd, exe)
	image := chooseIntuitionOSImage(opts.ExplicitImage, wd, exe)
	if opts.RequireKernel && !regularFileExists(image) {
		if opts.ExplicitImage != "" {
			return intuitionOSPaths{}, fmt.Errorf("IntuitionOS kernel not found at %s (run 'make intuitionos' to build)", image)
		}
		return intuitionOSPaths{}, fmt.Errorf("IntuitionOS kernel not found at %s (run 'make intuitionos' to build)", image)
	}
	return intuitionOSPaths{Root: root, Image: image}, nil
}

func chooseIntuitionOSRoot(explicitRoot, wd, exe string) string {
	if explicitRoot != "" {
		return absOrSelf(explicitRoot)
	}
	if envRoot := os.Getenv("INTUITIONOS_HOST_ROOT"); envRoot != "" {
		return absOrSelf(envRoot)
	}
	for _, candidate := range intuitionOSRootCandidates(wd, exe) {
		if dirExists(candidate) {
			return candidate
		}
	}
	candidates := intuitionOSRootCandidates(wd, exe)
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return absOrSelf(intuitionOSDevRootRel)
}

func chooseIntuitionOSImage(explicitImage, wd, exe string) string {
	if explicitImage != "" {
		return absOrSelf(explicitImage)
	}
	for _, candidate := range intuitionOSImageCandidates(wd, exe) {
		if regularFileExists(candidate) {
			return candidate
		}
	}
	candidates := intuitionOSImageCandidates(wd, exe)
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return absOrSelf(intuitionOSDevImageRel)
}

func intuitionOSRootCandidates(wd, exe string) []string {
	var out []string
	if wd != "" {
		out = append(out,
			absOrSelf(filepath.Join(wd, intuitionOSLiveRootRel)),
			absOrSelf(filepath.Join(wd, intuitionOSDevRootRel)),
		)
	}
	if exe != "" {
		exeDir := filepath.Dir(exe)
		out = append(out,
			absOrSelf(filepath.Join(exeDir, "..", intuitionOSLiveRootRel)),
			absOrSelf(filepath.Join(exeDir, intuitionOSLiveRootRel)),
			absOrSelf(filepath.Join(exeDir, "..", intuitionOSDevRootRel)),
			absOrSelf(filepath.Join(exeDir, intuitionOSDevRootRel)),
		)
	}
	out = append(out, absOrSelf(intuitionOSDevRootRel))
	return out
}

func intuitionOSImageCandidates(wd, exe string) []string {
	var out []string
	if wd != "" {
		out = append(out,
			absOrSelf(filepath.Join(wd, intuitionOSLiveImageRel)),
			absOrSelf(filepath.Join(wd, intuitionOSDevImageRel)),
		)
	}
	if exe != "" {
		exeDir := filepath.Dir(exe)
		out = append(out,
			absOrSelf(filepath.Join(exeDir, "..", intuitionOSLiveImageRel)),
			absOrSelf(filepath.Join(exeDir, intuitionOSLiveImageRel)),
			absOrSelf(filepath.Join(exeDir, "iexec.ie64")),
			absOrSelf(filepath.Join(exeDir, "..", intuitionOSDevImageRel)),
			absOrSelf(filepath.Join(exeDir, intuitionOSDevImageRel)),
		)
	}
	out = append(out, absOrSelf(intuitionOSDevImageRel))
	return out
}

func absOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
