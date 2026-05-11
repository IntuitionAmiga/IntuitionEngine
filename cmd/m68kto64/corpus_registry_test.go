package main

// corpus describes one registered real-world m68k codebase used as an
// end-to-end smoke test for the preprocessor + lowerer + ie64asm chain.
//
// Adding a new port is a matter of appending an entry here with the env-var
// that points at the source root, the relative root file path, and any -I /
// -D flags the build expects. The test harness in
// integration_realworld_test.go is corpus-agnostic.
type corpus struct {
	name     string
	envVar   string
	rootFile string
	includes []string
	defines  map[string]int64
}

// corpora is the registry. Each entry is gated by its env var (test
// t.Skip's if the var is unset or the root file is absent), so adding
// entries is non-disruptive.
var corpora = []corpus{
	{
		name:     "ab3d2",
		envVar:   "IE_M68KTO64_CORPUS_AB3D2",
		rootFile: "ab3d2_source/ie/hires.s",
		includes: []string{"ab3d2_source/ie", "ab3d2_source"},
	},
}
