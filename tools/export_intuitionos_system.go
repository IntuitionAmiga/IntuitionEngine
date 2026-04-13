package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type exportSpec struct {
	src string
	dst string
}

var systemExports = []exportSpec{
	{src: "boot_shell.elf", dst: "Tools/Shell"},
	{src: "boot_dos_library.elf", dst: "LIBS/dos.library"},
	{src: "boot_graphics_library.elf", dst: "LIBS/graphics.library"},
	{src: "boot_intuition_library.elf", dst: "LIBS/intuition.library"},
	{src: "boot_input_device.elf", dst: "DEVS/input.device"},
	{src: "boot_hardware_resource.elf", dst: "RESOURCES/hardware.resource"},
	{src: "seed_version.elf", dst: "C/Version"},
	{src: "seed_avail.elf", dst: "C/Avail"},
	{src: "seed_dir.elf", dst: "C/Dir"},
	{src: "seed_type.elf", dst: "C/Type"},
	{src: "seed_echo.elf", dst: "C/Echo"},
	{src: "seed_assign.elf", dst: "C/Assign"},
	{src: "seed_list.elf", dst: "C/List"},
	{src: "seed_which.elf", dst: "C/Which"},
	{src: "seed_help.elf", dst: "C/Help"},
	{src: "seed_gfxdemo.elf", dst: "C/GfxDemo"},
	{src: "seed_about.elf", dst: "C/About"},
	{src: "seed_elfseg.elf", dst: "C/ElfSeg"},
	{src: "sdk/intuitionos/iexec/assets/system/S/Startup-Sequence", dst: "S/Startup-Sequence"},
	{src: "sdk/intuitionos/iexec/assets/system/S/Help", dst: "S/Help"},
	{src: "sdk/intuitionos/iexec/assets/system/L/Loader-Info", dst: "L/Loader-Info"},
}

func main() {
	repoRoot := flag.String("repo-root", ".", "repository root")
	iexecDir := flag.String("iexec-dir", "sdk/intuitionos/iexec", "IExec build artifact directory")
	outRoot := flag.String("out-root", "sdk/intuitionos/system/SYS/IOSSYS", "output system tree root")
	flag.Parse()

	absRepoRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve repo root: %v\n", err)
		os.Exit(1)
	}
	absIExecDir := filepath.Join(absRepoRoot, *iexecDir)
	absOutRoot := filepath.Join(absRepoRoot, *outRoot)

	if err := os.RemoveAll(filepath.Dir(filepath.Dir(absOutRoot))); err != nil {
		fmt.Fprintf(os.Stderr, "remove existing system tree: %v\n", err)
		os.Exit(1)
	}
	for _, spec := range systemExports {
		src := filepath.Join(absIExecDir, spec.src)
		if filepath.IsAbs(spec.src) {
			src = spec.src
		}
		if filepath.Clean(spec.src) == spec.src && len(spec.src) >= 4 && spec.src[:4] == "sdk/" {
			src = filepath.Join(absRepoRoot, spec.src)
		}
		dst := filepath.Join(absOutRoot, spec.dst)
		if err := copyFile(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "export %s -> %s: %v\n", src, dst, err)
			os.Exit(1)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
