// prm-extract is the PRM doc-as-test harness front-end.
//
// Phases (each invocable independently so the orchestrator can keep
// reading state when one stage exits non-zero):
//
//	go run ./tools/prm-extract \
//	    -glob 01-* \
//	    -out  sdk/scripts/prm-runner/cases.json \
//	    -build sdk/scripts/prm-runner/build
//
//	go run ./tools/prm-extract -run-ies \
//	    -in   sdk/scripts/prm-runner/cases.json \
//	    -build sdk/scripts/prm-runner/build \
//	    -append sdk/scripts/prm-runner/report.json
//
//	go run ./tools/prm-extract -run-iemon \
//	    -in   sdk/scripts/prm-runner/cases.json \
//	    -build sdk/scripts/prm-runner/build \
//	    -append sdk/scripts/prm-runner/report.json
//
//	go run ./tools/prm-extract -render \
//	    -in   sdk/scripts/prm-runner/report.json \
//	    -out  tools/prm-extract/report.md
//
// Repo root is auto-detected from the executable's working directory. Pass
// `-repo-root` to override.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		glob     = flag.String("glob", "*", "chapter glob (without .md)")
		out      = flag.String("out", "", "extract phase output (cases.json path)")
		in       = flag.String("in", "", "run/render input path")
		build    = flag.String("build", "sdk/scripts/prm-runner/build", "wrapper output dir")
		appendTo = flag.String("append", "", "report.json to append into")
		runIES   = flag.Bool("run-ies", false, "run IES runnable cases (phase)")
		runIemon = flag.Bool("run-iemon", false, "run iemon cases (phase)")
		render   = flag.Bool("render", false, "render report.json → markdown")
		repoRoot = flag.String("repo-root", "", "repo root (defaults to cwd)")
		binary   = flag.String("ie-binary", "./bin/IntuitionEngine", "Intuition Engine binary for child launches")
	)
	flag.Parse()

	root := *repoRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			die(err)
		}
		root = cwd
	}

	switch {
	case *runIES:
		mustNonempty(*in, "-in")
		runIESPhase(*in, *build, *appendTo, root, *binary)
	case *runIemon:
		mustNonempty(*in, "-in")
		runIemonPhase(*in, *build, *appendTo, root, *binary)
	case *render:
		mustNonempty(*in, "-in")
		mustNonempty(*out, "-out")
		renderReport(*in, *out)
	default:
		mustNonempty(*out, "-out")
		extractPhase(*glob, *out, *build, root)
	}
}

func extractPhase(glob, out, build, root string) {
	refmanDir := filepath.Join(root, "sdk", "docs", "refman.publish")
	if _, err := os.Stat(refmanDir); err != nil {
		die(fmt.Errorf("refman dir %s: %w", refmanDir, err))
	}
	cases, err := extractAll(refmanDir, glob, root)
	if err != nil {
		die(err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		die(err)
	}
	if err := os.MkdirAll(build, 0o755); err != nil {
		die(err)
	}
	if err := writeIESWrappers(cases.Cases, build); err != nil {
		die(err)
	}
	if err := writeIemonWrappers(cases.Cases, build, root); err != nil {
		die(err)
	}
	data, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		die(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		die(err)
	}
	// Mirror as Lua table next to cases.json so the Lua runner can
	// `require("cases")` without needing a JSON parser in the sandbox.
	luaOut := filepath.Join(filepath.Dir(out), "cases.lua")
	if err := writeCasesLua(cases.Cases, luaOut); err != nil {
		die(err)
	}
	fmt.Printf("prm-extract: %d case(s) → %s\n", len(cases.Cases), out)
}

func mustNonempty(v, name string) {
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required flag: %s\n", name)
		os.Exit(2)
	}
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "prm-extract: %v\n", err)
	os.Exit(1)
}
