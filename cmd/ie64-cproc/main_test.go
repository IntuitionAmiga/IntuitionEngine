package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandOutputDashWritesStdout(t *testing.T) {
	dir := t.TempDir()
	producer := filepath.Join(dir, "producer")
	if err := os.WriteFile(producer, []byte("#!/bin/sh\nprintf 'preprocessed output\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	err = commandOutput(false, "-", producer)
	os.Stdout = oldStdout
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if string(got) != "preprocessed output\n" {
		t.Fatalf("stdout = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); !os.IsNotExist(err) {
		t.Fatalf("literal output file was created: %v", err)
	}
}

func TestCopyOutputDashWritesStdout(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("object bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	err = copyOutput(source, "-")
	os.Stdout = oldStdout
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if string(got) != "object bytes" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestParsePublicModesAndTarget(t *testing.T) {
	for _, args := range [][]string{
		{"-E", "input.c"},
		{"-S", "input.c"},
		{"-c", "a.c", "b.c"},
		{"-MMD", "-MP", "-MF", "input.d", "input.c"},
		{"-nostdlib", "--entry", "start", "input.o"},
	} {
		if _, done, err := parseArgs(args); err != nil || done {
			t.Fatalf("parseArgs(%q) = done %v, err %v", args, done, err)
		}
	}
}

func TestParseRejectsUserTargetSelection(t *testing.T) {
	for _, args := range [][]string{
		{"-DIE_TARGET_IE64=1", "input.c"},
		{"-D", "IE_TARGET_M68K=1", "input.c"},
		{"-UIE_TARGET_Z80", "input.c"},
		{"-U", "IE_TARGET_6502", "input.c"},
		{"-DIE_TARGET_X86", "input.c"},
	} {
		if _, _, err := parseArgs(args); err == nil {
			t.Fatalf("parseArgs(%q) accepted user target selection", args)
		}
	}
}

func TestCprocArgsInjectsOnlyDriverTarget(t *testing.T) {
	c, _, err := parseArgs([]string{"input.c"})
	if err != nil {
		t.Fatal(err)
	}
	args, err := cprocArgs(c, layout{include: "/sdk/include", standardInclude: "/sdk/lib/ie64-unknown-none/include"}, "input.c")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacent(args, "-D", "IE_TARGET_IE64=1") {
		t.Fatalf("missing driver target definition in %q", args)
	}
	if !containsAdjacent(args, "-Y", "/sdk/lib/ie64-unknown-none/include") || !containsAdjacent(args, "-Y", "/sdk/include") {
		t.Fatalf("missing standard or public include path in %q", args)
	}
}

func containsAdjacent(args []string, first, second string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

func TestParseOptimisationLevels(t *testing.T) {
	defaultConfig, _, err := parseArgs([]string{"input.c"})
	if err != nil || defaultConfig.optimisation != 2 {
		t.Fatalf("default optimisation = %d, err %v", defaultConfig.optimisation, err)
	}
	for level := 0; level <= 3; level++ {
		c, done, err := parseArgs([]string{fmt.Sprintf("-O%d", level), "input.c"})
		if err != nil || done {
			t.Fatalf("parseArgs(-O%d) = done %v, err %v", level, done, err)
		}
		if c.optimisation != level {
			t.Fatalf("parseArgs(-O%d) optimisation = %d", level, c.optimisation)
		}
	}
	c, _, err := parseArgs([]string{"-O0", "-O3", "input.c"})
	if err != nil || c.optimisation != 3 {
		t.Fatalf("last optimisation option did not win: config %+v, err %v", c, err)
	}
}

func TestParseRejectsMeaningChangingUnknownOption(t *testing.T) {
	for _, option := range []string{"-fPIC", "-Wunknown-warning", "-O", "-O4", "-Os", "-Ofast"} {
		if _, _, err := parseArgs([]string{option, "input.c"}); err == nil {
			t.Fatalf("unsupported %s was accepted", option)
		}
	}
}

func TestCompileCForwardsOptimisationLevelToQBE(t *testing.T) {
	dir := t.TempDir()
	cproc := filepath.Join(dir, "cproc-qbe")
	qbe := filepath.Join(dir, "qbe")
	logPath := filepath.Join(dir, "qbe.args")
	if err := os.WriteFile(cproc, []byte("#!/bin/sh\nprintf 'export function w $f() { @start ret 0 }\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	qbeScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\n"
	if err := os.WriteFile(qbe, []byte(qbeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.c")
	if err := os.WriteFile(input, []byte("int f(void) { return 0; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for level := 0; level <= 3; level++ {
		c := config{optimisation: level}
		err := compileC(c, layout{cproc: cproc, qbe: qbe}, input,
			filepath.Join(dir, fmt.Sprintf("output-%d.s", level)), dir, level)
		if err != nil {
			t.Fatalf("compileC at -O%d: %v", level, err)
		}
		args, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(args), fmt.Sprintf("-O%d\n", level)) {
			t.Fatalf("QBE args at level %d = %q", level, args)
		}
	}
}

func TestParseRejectsOneOutputForSeveralCompileInputs(t *testing.T) {
	if _, _, err := parseArgs([]string{"-c", "-o", "all.o", "a.c", "b.c"}); err == nil {
		t.Fatal("one -o was accepted for several -c inputs")
	}
}

func TestValidateOutputPathsRejectsInputCollisions(t *testing.T) {
	dir := t.TempDir()
	cSource := filepath.Join(dir, "source.c")
	asmSource := filepath.Join(dir, "source.s")
	for _, path := range []string{cSource, asmSource} {
		if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []config{
		{mode: modeAssembly, output: cSource, inputs: []string{cSource}},
		{mode: modeObject, output: asmSource, inputs: []string{asmSource}},
		{mode: modeLink, output: cSource, inputs: []string{cSource}},
		{mode: modeLink, mapPath: cSource, inputs: []string{cSource}},
		{mode: modeAssembly, inputs: []string{cSource, asmSource}},
	} {
		if err := validateOutputPaths(c); err == nil || !strings.Contains(err.Error(), "would overwrite input") {
			t.Fatalf("validateOutputPaths(%+v) = %v", c, err)
		}
	}
}

func TestBuildRejectsCollisionBeforeOverwritingSource(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.c")
	want := []byte("int preserved;\n")
	if err := os.WriteFile(input, want, 0o600); err != nil {
		t.Fatal(err)
	}
	err := build(config{mode: modeAssembly, output: input, inputs: []string{input}}, layout{})
	if err == nil || !strings.Contains(err.Error(), "would overwrite input") {
		t.Fatalf("build returned %v", err)
	}
	got, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("source changed to %q", got)
	}
}

func TestValidateOutputPathsResolvesAliases(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.c")
	if err := os.WriteFile(input, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliases := []string{filepath.Join(dir, "source-link.c"), filepath.Join(dir, "source-hardlink.c")}
	if err := os.Symlink(input, aliases[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(input, aliases[1]); err != nil {
		t.Fatal(err)
	}
	for _, output := range aliases {
		if err := validateOutputPaths(config{mode: modeAssembly, output: output, inputs: []string{input}}); err == nil {
			t.Fatalf("alias output %s was accepted", output)
		}
	}
}

func TestBuildPlacesUserLibrariesBeforeDefaultLibraries(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	if err := os.Mkdir(libDir, 0o700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.o")
	userLib := filepath.Join(libDir, "libfoo.a")
	defaultLib := filepath.Join(libDir, "libc.a")
	for _, path := range []string{input, userLib, defaultLib} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(dir, "link.args")
	linker := filepath.Join(dir, "ie64ld")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\n"
	if err := os.WriteFile(linker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	c := config{
		mode:         modeLink,
		output:       filepath.Join(dir, "program.ie64"),
		nostartfiles: true,
		libDirs:      []string{libDir},
		inputs:       []string{input},
		linkArgs: []linkArgument{
			{value: input},
			{value: "foo", library: true},
		},
	}
	if err := build(c, layout{root: dir, lib: libDir, ld: linker}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	userIndex := strings.Index(got, userLib)
	defaultIndex := strings.Index(got, defaultLib)
	if userIndex < 0 || defaultIndex < 0 || userIndex > defaultIndex {
		t.Fatalf("user library does not precede compiler default libraries:\n%s", got)
	}
}

func TestBuildPreservesInterleavedLibraryPositions(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	if err := os.Mkdir(libDir, 0o700); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "first.o")
	second := filepath.Join(dir, "second.o")
	foo := filepath.Join(libDir, "libfoo.a")
	bar := filepath.Join(libDir, "libbar.a")
	for _, path := range []string{first, second, foo, bar} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(dir, "link.args")
	linker := filepath.Join(dir, "ie64ld")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\n"
	if err := os.WriteFile(linker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	c, _, err := parseArgs([]string{"-nostdlib", "-L", libDir, first, "-lfoo", second, "-lbar", "-o", filepath.Join(dir, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if err := build(c, layout{lib: libDir, ld: linker}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(args))
	want := []string{first, foo, second, bar}
	position := 0
	for _, arg := range got {
		if position < len(want) && arg == want[position] {
			position++
		}
	}
	if position != len(want) {
		t.Fatalf("link order = %q, want subsequence %q", got, want)
	}
}
