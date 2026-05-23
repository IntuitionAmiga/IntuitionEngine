// IEScript fence handling.
//
// Default behaviour for an `ies` fence is `mode:"lint"` + `status:"READY"`.
// The extractor regex-scans the snippet for `module.fn(...)` call targets,
// diffs them against a checked-in symbol list at `symbols.txt`, and writes
// the result into the case's APILint field.
//
// Runnable opt-in: a line starting with `-- @prm-runnable` (anywhere in
// the snippet) flips the case to `mode:"runnable"` and records the
// snippet for out-of-process execution. The orchestrator emits a wrapped
// `build/<id>.ies` and launches it in its own child.
//
// The symbol list lives in `symbols.txt` next to this source. Regenerate
// with `go run ./tools/prm-extract/cmd/regen_symbols > tools/prm-extract/symbols.txt`.

package main

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed symbols.txt
var embeddedSymbols string

// callRe matches `module.fn` references in IES snippet text. We only flag
// the registered top-level Lua modules — local Lua variables that happen
// to share a name (`local sys = ...`) would slip through, but the
// snippets in the PRM never shadow these globals.
var callRe = regexp.MustCompile(`\b(sys|cpu|mem|term|audio|video|rec|repl|dbg|sym|regions|coproc|media)\.([a-zA-Z_][a-zA-Z0-9_]*)`)

var runnableRe = regexp.MustCompile(`--\s*@prm-runnable(?:\s+timeout\s*=\s*(\d+)s?)?`)

var expectBlockRe = regexp.MustCompile(`(?ms)^--\s*@prm-expect\s*\n((?:--[^\n]*\n)+)`)

// segmentIES emits one Case per ies fence.
func segmentIES(f RawFence) ([]Case, error) {
	id := f.Directives.ID
	if id == "" {
		id = hashID(f.Body)
	}

	lint := lintSnippet(f.Body)

	c := Case{
		ID:             id,
		Source:         f.Source,
		FenceStartLine: f.StartLine,
		Kind:           KindIES,
		Status:         StatusReady,
		Mode:           IESModeLint,
		APILint:        &lint,
	}

	if rm := runnableRe.FindStringSubmatch(f.Body); rm != nil {
		c.Mode = IESModeRunnable
		timeout := 5
		if rm[1] != "" {
			fmt.Sscanf(rm[1], "%d", &timeout)
		}
		c.IESTimeoutS = timeout
	}
	if em := expectBlockRe.FindStringSubmatch(f.Body); em != nil {
		c.ExpectedStdout = stripCommentPrefix(em[1])
	}
	return []Case{c}, nil
}

func stripCommentPrefix(block string) string {
	var out []string
	for _, ln := range strings.Split(block, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		t = strings.TrimPrefix(t, "--")
		t = strings.TrimPrefix(t, " ")
		out = append(out, t)
	}
	return strings.Join(out, "\n")
}

// lintSnippet returns PASS/FAIL + the set of unknown call targets.
func lintSnippet(body string) APILint {
	known := loadSymbolSet()
	unknownSet := map[string]struct{}{}
	for _, m := range callRe.FindAllStringSubmatch(body, -1) {
		fullName := m[1] + "." + m[2]
		if _, ok := known[fullName]; !ok {
			unknownSet[fullName] = struct{}{}
		}
	}
	if len(unknownSet) == 0 {
		return APILint{Status: "PASS"}
	}
	unknown := make([]string, 0, len(unknownSet))
	for k := range unknownSet {
		unknown = append(unknown, k)
	}
	sort.Strings(unknown)
	return APILint{Status: "FAIL", Unknown: unknown}
}

var cachedSymbols map[string]struct{}

func loadSymbolSet() map[string]struct{} {
	if cachedSymbols != nil {
		return cachedSymbols
	}
	cachedSymbols = make(map[string]struct{})
	for _, ln := range strings.Split(embeddedSymbols, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		cachedSymbols[t] = struct{}{}
	}
	return cachedSymbols
}
