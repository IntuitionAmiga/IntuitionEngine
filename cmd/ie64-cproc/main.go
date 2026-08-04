package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type mode int

const (
	modeLink mode = iota
	modePreprocess
	modeAssembly
	modeObject
	modeDependencies
)

type config struct {
	mode                                           mode
	output, sysroot, entry, mapPath                string
	nostdlib, nodefaultlibs, nostartfiles, verbose bool
	cppArgs, libDirs, inputs                       []string
	linkArgs                                       []linkArgument
	depFile                                        string
	depTargets                                     []string
	depSide, depOmit, depPhony                     bool
	werror                                         bool
	optimisation                                   int
}
type linkArgument struct {
	value   string
	library bool
}
type layout struct{ bin, root, include, standardInclude, lib, cproc, qbe, asm, ld string }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ie64-cproc: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	c, done, err := parseArgs(args)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	l, err := findLayout(c.sysroot)
	if err != nil {
		return err
	}
	return build(c, l)
}

func parseArgs(args []string) (config, bool, error) {
	c := config{optimisation: 2}
	take := func(i *int) (string, error) {
		*i++
		if *i >= len(args) {
			return "", errors.New("missing option argument")
		}
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if isTargetSelectionOption(a) {
			return c, false, errors.New("IE_TARGET_* definitions are reserved for ie64-cproc")
		}
		switch {
		case a == "--help":
			fmt.Println("usage: ie64-cproc [options] file...\n" +
				"modes: -E -S -c (default: link)\n" +
				"paths: -I -iquote -isystem -idirafter -include -L -l --sysroot\n" +
				"preprocessor: -D -U -M -MM -MD -MMD -MF -MT -MP -nostdinc\n" +
				"optimisation: -O0 -O1 -O2 -O3 (default: -O2)\n" +
				"runtime: -nostdlib -nodefaultlibs -nostartfiles --entry --map\n" +
				"language: -std=c23 -ffreestanding -fno-builtin -Werror")
			return c, true, nil
		case a == "--version":
			fmt.Println("ie64-cproc 3 (ie64-unknown-none ABI V3)")
			return c, true, nil
		case a == "-dumpmachine":
			fmt.Println("ie64-unknown-none")
			return c, true, nil
		case a == "-E":
			c.mode = modePreprocess
		case a == "-S":
			c.mode = modeAssembly
		case a == "-c":
			c.mode = modeObject
		case a == "-nostdlib":
			c.nostdlib = true
		case a == "-nodefaultlibs":
			c.nodefaultlibs = true
		case a == "-nostartfiles":
			c.nostartfiles = true
		case a == "-v":
			c.verbose = true
		case len(a) == 3 && strings.HasPrefix(a, "-O") && a[2] >= '0' && a[2] <= '3':
			c.optimisation = int(a[2] - '0')
		case a == "-o":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.output = v
		case a == "--sysroot":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.sysroot = v
		case strings.HasPrefix(a, "--sysroot="):
			c.sysroot = strings.TrimPrefix(a, "--sysroot=")
		case a == "--entry":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.entry = v
		case a == "--map":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.mapPath = v
		case a == "-I" || a == "-D" || a == "-U" || a == "-iquote" || a == "-isystem" || a == "-idirafter" || a == "-include":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			if (a == "-D" || a == "-U") && isTargetSelection(v) {
				return c, false, errors.New("IE_TARGET_* definitions are reserved for ie64-cproc")
			}
			c.cppArgs = append(c.cppArgs, a, v)
		case a == "-MF" || a == "-MT":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			if a == "-MF" {
				c.depFile = v
			} else {
				c.depTargets = append(c.depTargets, v)
			}
		case strings.HasPrefix(a, "-I") || strings.HasPrefix(a, "-D") || strings.HasPrefix(a, "-U"):
			c.cppArgs = append(c.cppArgs, a)
		case a == "-M" || a == "-MM":
			c.mode = modeDependencies
			c.depOmit = a == "-MM"
		case a == "-MD" || a == "-MMD":
			c.depSide = true
			c.depOmit = a == "-MMD"
		case a == "-MP":
			c.depPhony = true
		case a == "-nostdinc" || a == "-std=c23":
			c.cppArgs = append(c.cppArgs, a)
		case a == "-L":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.libDirs = append(c.libDirs, v)
		case strings.HasPrefix(a, "-L"):
			c.libDirs = append(c.libDirs, a[2:])
		case a == "-l":
			v, e := take(&i)
			if e != nil {
				return c, false, e
			}
			c.linkArgs = append(c.linkArgs, linkArgument{value: v, library: true})
		case strings.HasPrefix(a, "-l"):
			c.linkArgs = append(c.linkArgs, linkArgument{value: a[2:], library: true})
		case a == "-Werror":
			c.werror = true
		case a == "-ffreestanding" || a == "-fno-builtin" || a == "-Wall" || a == "-Wextra" || a == "-Wpedantic": // Accepted only where semantics are already target-neutral.
		case strings.HasPrefix(a, "-"):
			return c, false, fmt.Errorf("unsupported option %s", a)
		default:
			c.inputs = append(c.inputs, a)
			c.linkArgs = append(c.linkArgs, linkArgument{value: a})
		}
	}
	if len(c.inputs) == 0 {
		return c, false, errors.New("no input files")
	}
	if c.output != "" && c.mode != modeLink && len(c.inputs) != 1 {
		return c, false, errors.New("-o with multiple inputs is invalid outside link mode")
	}
	return c, false, nil
}

func isTargetSelectionOption(arg string) bool {
	return (strings.HasPrefix(arg, "-D") || strings.HasPrefix(arg, "-U")) && len(arg) > 2 && isTargetSelection(arg[2:])
}

func isTargetSelection(value string) bool {
	name := value
	if i := strings.IndexByte(name, '='); i >= 0 {
		name = name[:i]
	}
	return strings.HasPrefix(name, "IE_TARGET_")
}

func findLayout(sysroot string) (layout, error) {
	exe, err := os.Executable()
	if err != nil {
		return layout{}, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return layout{}, err
	}
	bin := filepath.Dir(exe)
	root := filepath.Dir(bin)
	if sysroot != "" {
		root = sysroot
	}
	l := layout{bin: bin, root: root, include: filepath.Join(root, "include"), lib: filepath.Join(root, "lib", "ie64-unknown-none")}
	l.standardInclude = filepath.Join(l.lib, "include")
	if _, err := os.Stat(l.standardInclude); err != nil {
		l.standardInclude = l.include
	}
	l.cproc = filepath.Join(bin, "cproc-qbe")
	l.qbe = filepath.Join(bin, "qbe")
	l.asm = filepath.Join(bin, "ie64asm")
	l.ld = filepath.Join(bin, "ie64ld")
	if filesExist(l.cproc, l.qbe, l.asm, l.ld) {
		return l, nil
	}
	// Development fallback from the IntuitionEngine checkout.
	_, file, _, _ := runtime.Caller(0)
	ie := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	parent := filepath.Dir(ie)
	l.root = filepath.Join(ie, "sdk")
	if sysroot != "" {
		l.root = sysroot
	}
	l.include = filepath.Join(l.root, "include")
	l.lib = filepath.Join(l.root, "lib", "ie64-unknown-none")
	l.standardInclude = filepath.Join(l.lib, "include")
	if _, err := os.Stat(l.standardInclude); err != nil {
		l.standardInclude = l.include
	}
	l.cproc = filepath.Join(parent, "cproc", "cproc-qbe")
	l.qbe = filepath.Join(parent, "qbe", "qbe")
	l.asm = filepath.Join(ie, "sdk", "bin", "ie64asm")
	l.ld = filepath.Join(ie, "sdk", "bin", "ie64ld")
	if !filesExist(l.cproc, l.qbe, l.asm, l.ld) {
		return layout{}, errors.New("cannot locate cproc-qbe, qbe, ie64asm and ie64ld companions")
	}
	return l, nil
}
func filesExist(paths ...string) bool {
	for _, p := range paths {
		if st, e := os.Stat(p); e != nil || st.IsDir() {
			return false
		}
	}
	return true
}

func build(c config, l layout) error {
	if err := validateOutputPaths(c); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "ie64-cproc-v3-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if c.mode == modePreprocess {
		if len(c.inputs) != 1 || filepath.Ext(c.inputs[0]) != ".c" {
			return errors.New("-E requires one C source")
		}
		out := c.output
		return preprocess(c, l, c.inputs[0], out)
	}
	if c.mode == modeDependencies {
		if len(c.inputs) != 1 || filepath.Ext(c.inputs[0]) != ".c" {
			return errors.New("-M/-MM requires one C source")
		}
		return dependencies(c, l, c.inputs[0])
	}
	placedInputs := make([]string, len(c.inputs))
	for i, input := range c.inputs {
		ext := strings.ToLower(filepath.Ext(input))
		switch ext {
		case ".c":
			asmPath := filepath.Join(tmp, fmt.Sprintf("unit-%d.s", i))
			if err = compileC(c, l, input, asmPath, tmp, i); err != nil {
				return err
			}
			if c.mode == modeAssembly {
				if err = copyOutput(asmPath, outputFor(c, input, ".s")); err != nil {
					return err
				}
				continue
			}
			objPath := filepath.Join(tmp, fmt.Sprintf("unit-%d.o", i))
			if err = command(c.verbose, l.asm, "-c", "-Werror", "-o", objPath, asmPath); err != nil {
				return err
			}
			if c.mode == modeObject {
				if err = copyOutput(objPath, outputFor(c, input, ".o")); err != nil {
					return err
				}
				continue
			}
			placedInputs[i] = objPath
		case ".s":
			if c.mode == modeAssembly || c.mode == modePreprocess {
				return fmt.Errorf("mode does not accept assembly input %s", input)
			}
			objPath := filepath.Join(tmp, fmt.Sprintf("asm-%d.o", i))
			if err = command(c.verbose, l.asm, "-c", "-Werror", "-I", l.include, "-o", objPath, input); err != nil {
				return err
			}
			if c.mode == modeObject {
				if err = copyOutput(objPath, outputFor(c, input, ".o")); err != nil {
					return err
				}
				continue
			}
			placedInputs[i] = objPath
		case ".o", ".a":
			if c.mode != modeLink {
				return fmt.Errorf("mode does not accept %s", input)
			}
			placedInputs[i] = input
		default:
			return fmt.Errorf("unsupported input %s", input)
		}
	}
	if c.mode != modeLink {
		return nil
	}
	var linkInputs []string
	if !c.nostdlib && !c.nostartfiles {
		crt := filepath.Join(l.lib, "crt0.o")
		if !filesExist(crt) {
			source := filepath.Join(l.root, "lib", "ie64-cproc", "crt0.s")
			crt = filepath.Join(tmp, "crt0.o")
			if err = command(c.verbose, l.asm, "-c", "-Werror", "-o", crt, source); err != nil {
				return err
			}
		}
		// The flat-image contract requires _start at PROG_START, so crt0 is a
		// start file rather than a default library and must remain first.
		linkInputs = append(linkInputs, crt)
	}
	inputIndex := 0
	for _, argument := range c.linkArgs {
		if argument.library {
			p, err := findLibrary(argument.value, append(c.libDirs, l.lib))
			if err != nil {
				return err
			}
			linkInputs = append(linkInputs, p)
		} else {
			linkInputs = append(linkInputs, placedInputs[inputIndex])
			inputIndex++
		}
	}
	// Keep every user object and library in exactly the order provided. Default
	// libraries are appended afterwards so they cannot alter archive extraction
	// among user arguments.
	if !c.nostdlib && !c.nodefaultlibs {
		for _, name := range []string{"libc.a", "libm.a", "libatomic.a"} {
			p := filepath.Join(l.lib, name)
			if filesExist(p) {
				linkInputs = append(linkInputs, p)
			}
		}
		legacy := filepath.Join(l.root, "lib", "ie64-cproc", "libie64c.s")
		if !filesExist(filepath.Join(l.lib, "libc.a")) && filesExist(legacy) {
			o := filepath.Join(tmp, "libie64c.o")
			if err = command(c.verbose, l.asm, "-c", "-Werror", "-o", o, legacy); err != nil {
				return err
			}
			linkInputs = append(linkInputs, o)
		}
	}
	args := []string{"-o", outputFor(c, "a.ie64", "")}
	if c.entry != "" {
		args = append(args, "--entry", c.entry)
	}
	if c.mapPath != "" {
		args = append(args, "--map", c.mapPath)
	}
	args = append(args, linkInputs...)
	return command(c.verbose, l.ld, args...)
}

func validateOutputPaths(c config) error {
	var outputs []string
	add := func(path string) {
		if path != "" && path != "-" {
			outputs = append(outputs, path)
		}
	}
	for _, input := range c.inputs {
		switch c.mode {
		case modePreprocess:
			add(c.output)
		case modeAssembly:
			if strings.EqualFold(filepath.Ext(input), ".c") {
				add(outputFor(c, input, ".s"))
			}
		case modeObject:
			if ext := strings.ToLower(filepath.Ext(input)); ext == ".c" || ext == ".s" {
				add(outputFor(c, input, ".o"))
			}
		}
		if c.depSide && strings.EqualFold(filepath.Ext(input), ".c") {
			depfile := c.depFile
			if depfile == "" {
				depfile = strings.TrimSuffix(input, filepath.Ext(input)) + ".d"
			}
			add(depfile)
		}
	}
	if c.mode == modeDependencies {
		add(c.depFile)
	}
	if c.mode == modeLink {
		add(outputFor(c, "a.ie64", ""))
		add(c.mapPath)
	}

	for _, output := range outputs {
		for _, input := range c.inputs {
			same, err := sameResolvedPath(output, input)
			if err != nil {
				return err
			}
			if same {
				return fmt.Errorf("output %s would overwrite input %s", output, input)
			}
		}
	}
	for i, output := range outputs {
		for _, other := range outputs[:i] {
			same, err := sameResolvedPath(output, other)
			if err != nil {
				return err
			}
			if same {
				return fmt.Errorf("multiple outputs resolve to %s", output)
			}
		}
	}
	return nil
}

func sameResolvedPath(a, b string) (bool, error) {
	aPath, aInfo, err := resolvedPath(a)
	if err != nil {
		return false, err
	}
	bPath, bInfo, err := resolvedPath(b)
	if err != nil {
		return false, err
	}
	return aPath == bPath || aInfo != nil && bInfo != nil && os.SameFile(aInfo, bInfo), nil
}

func resolvedPath(path string) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, statErr := os.Stat(abs)
	if statErr == nil {
		resolved, err := filepath.EvalSymlinks(abs)
		return resolved, info, err
	}
	if !os.IsNotExist(statErr) {
		return "", nil, statErr
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil, nil
}

func outputFor(c config, input, ext string) string {
	if c.output != "" {
		return c.output
	}
	if ext == "" {
		return input
	}
	return strings.TrimSuffix(input, filepath.Ext(input)) + ext
}
func preprocess(c config, l layout, input, output string) error {
	args, err := cprocArgs(c, l, input)
	if err != nil {
		return err
	}
	args = append([]string{"-E"}, args...)
	args = append(args, input)
	return commandOutput(c.verbose, output, l.cproc, args...)
}
func compileC(c config, l layout, input, assembly, tmp string, index int) error {
	args, err := cprocArgs(c, l, input)
	if err != nil {
		return err
	}
	qbeIL := filepath.Join(tmp, fmt.Sprintf("unit-%d.qbe", index))
	args = append(args, input)
	if err := commandOutput(c.verbose, qbeIL, l.cproc, args...); err != nil {
		return err
	}
	return command(c.verbose, l.qbe, "-t", "ie64", fmt.Sprintf("-O%d", c.optimisation), "-o", assembly, qbeIL)
}

func cprocArgs(c config, l layout, input string) ([]string, error) {
	args := []string{"-t", "ie64", "-D", "IE_TARGET_IE64=1"}
	if c.werror {
		args = append(args, "-Werror")
	}
	nostdinc := false
	for _, arg := range c.cppArgs {
		if arg == "-nostdinc" {
			nostdinc = true
		}
	}
	if !nostdinc {
		args = append(args, "-Y", l.standardInclude)
		args = append(args, "-Y", l.include)
	}
	for i := 0; i < len(c.cppArgs); i++ {
		a := c.cppArgs[i]
		switch a {
		case "-I", "-D", "-U":
			if i+1 >= len(c.cppArgs) {
				return nil, fmt.Errorf("missing value for %s", a)
			}
			i++
			args = append(args, a, c.cppArgs[i])
		case "-iquote", "-isystem", "-idirafter", "-include":
			if i+1 >= len(c.cppArgs) {
				return nil, fmt.Errorf("missing value for %s", a)
			}
			i++
			internal := map[string]string{"-iquote": "-Q", "-isystem": "-Y", "-idirafter": "-A", "-include": "-F"}[a]
			args = append(args, internal, c.cppArgs[i])
		case "-nostdinc", "-std=c23":
		default:
			args = append(args, a)
		}
	}
	if c.depSide {
		depfile := c.depFile
		if depfile == "" {
			depfile = strings.TrimSuffix(input, filepath.Ext(input)) + ".d"
		}
		args = append(args, "-d", depfile)
		args = appendDependencyOptions(args, c, dependencyTarget(c, input))
	}
	return args, nil
}

func dependencies(c config, l layout, input string) error {
	args, err := cprocArgs(c, l, input)
	if err != nil {
		return err
	}
	depfile := c.depFile
	if depfile == "" {
		depfile = "-"
	}
	args = append([]string{"-Z", "-d", depfile}, args...)
	args = appendDependencyOptions(args, c, dependencyTarget(c, input))
	args = append(args, input)
	return command(c.verbose, l.cproc, args...)
}

func appendDependencyOptions(args []string, c config, target string) []string {
	if c.depOmit {
		args = append(args, "-k")
	}
	if c.depPhony {
		args = append(args, "-P")
	}
	if len(c.depTargets) == 0 {
		return append(args, "-T", target)
	}
	for _, t := range c.depTargets {
		args = append(args, "-T", t)
	}
	return args
}

func dependencyTarget(c config, input string) string {
	if c.mode == modeObject && c.output != "" {
		return c.output
	}
	return strings.TrimSuffix(input, filepath.Ext(input)) + ".o"
}
func command(verbose bool, name string, args ...string) error {
	return commandOutput(verbose, "", name, args...)
}
func commandOutput(verbose bool, output, name string, args ...string) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", name, strings.Join(args, " "))
	}
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	if output == "" || output == "-" {
		cmd.Stdout = os.Stdout
	} else {
		f, e := os.Create(output)
		if e != nil {
			return e
		}
		cmd.Stdout = f
		defer f.Close()
	}
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("%s failed: %w", filepath.Base(name), e)
	}
	return nil
}
func copyOutput(src, dst string) error {
	if src == dst {
		return nil
	}
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	if dst == "-" {
		_, e = io.Copy(os.Stdout, in)
		return e
	}
	out, e := os.Create(dst)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}
func findLibrary(name string, dirs []string) (string, error) {
	for _, d := range dirs {
		p := filepath.Join(d, "lib"+name+".a")
		if filesExist(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("library -l%s not found", name)
}
