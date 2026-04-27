// docs_max_ram_check_test.go - PLAN_MAX_RAM.md slice 7 enforcement.
//
// Asserts that authoritative repo + SDK docs do not describe the current
// machine as a fixed 32 MB bus and do not silently replace that with a
// fixed 2 GiB claim. Also asserts that each authoritative doc mentions
// the autodetected appliance RAM model so newcomers do not pick up the
// historical layout as the current contract.
//
// Tests are layered: stale-claim check (must NOT contain fixed-32-MB
// language without a historical/legacy marker) and current-claim check
// (must contain the autodetect language somewhere).
//
// Historical references inside clearly-labeled historical/legacy/M-series
// blocks are tolerated. The check looks at full docs and the test will
// require us to either update the language or annotate the surviving
// historical text inline.

package main

import (
	"os"
	"strings"
	"testing"
)

// authoritativeDocs lists the docs PLAN_MAX_RAM.md slice 7 commits to
// keeping current. Each entry is repo-relative; the test resolves it
// relative to the package directory (which is the repo root for the
// main package).
var authoritativeDocs = []string{
	"README.md",
	"DEVELOPERS.md",
	"AGENTS.md",
	"CLAUDE.md",
	"sdk/intuitionos/CLAUDE.md",
	"sdk/docs/architecture.md",
	"sdk/docs/IE64_ISA.md",
	"sdk/docs/IE64_ABI.md",
	"sdk/docs/IE64_JIT.md",
	"sdk/docs/IE64_COOKBOOK.md",
	"sdk/docs/IntuitionOS/IExec.md",
	"sdk/docs/IntuitionOS/ELF.md",
	"sdk/docs/TUTORIAL.md",
	"sdk/docs/iemon.md",
	"sdk/docs/x86_JIT.md",
	"sdk/docs/M68K_JIT.md",
}

// staleFixed32MBPhrases describes the bus/RAM as a fixed 32 MB current
// fact. Any authoritative doc that contains one of these must either
// remove it or rewrite it to describe the autodetect model.
var staleFixed32MBPhrases = []string{
	"32MB unified memory",
	"32 MB unified memory",
	"32MB unified memory bus",
	"32 MB unified memory bus",
	"32MB shared system",
	"32 MB shared system",
	"shared 32MB system bus",
	"shared 32 MB system bus",
	"global 32MB RAM",
	"global 32 MB RAM",
	"32-bit flat address space (32MB",
	"32-bit flat address space (32 MB",
	"32MB address space",
	"32 MB address space",
	"unified 32MB address space",
	"unified 32 MB address space",
	"masked to 25 bits / 32MB",
	"masked to 25 bits / 32 MB",
}

// staleFixed2GiBPhrases catches any doc that claims a fixed 2 GiB
// replacement. PLAN_MAX_RAM.md explicitly forbids replacing 32 MB with
// 2 GiB unless describing a deliberately-chosen compatibility profile.
var staleFixed2GiBPhrases = []string{
	"fixed 2 GiB",
	"fixed 2GiB",
	"fixed 2 GB",
	"fixed 2GB",
	"hardcoded 2 GiB",
	"hardcoded 2GiB",
}

// requiredAutodetectMarkers lists phrases that a current-state doc must
// include to satisfy the rule that authoritative docs explain the
// autodetected appliance RAM model. The test passes if at least one
// marker appears in the doc.
var requiredAutodetectMarkers = []string{
	"autodetected guest RAM",
	"autodetect guest RAM",
	"autodetected appliance RAM",
	"total guest RAM",
	"active visible RAM",
	"ProfileMemoryCap",
	"SYSINFO_TOTAL_RAM",
	"SYSINFO_ACTIVE_RAM",
	"CR_RAM_SIZE_BYTES",
}

// historicalAllowList allows a doc to retain historical 32 MB language
// inside paragraphs that mention one of these tokens. The check below
// matches *line-by-line*: a line containing a stale phrase is tolerated
// only if it also contains a historical-token. Block-level historical
// markers (M-series snapshots, "historical", "legacy") wider than a
// single line should be rewritten to inline-tag the affected lines.
var historicalAllowList = []string{
	"historical",
	"Historical",
	"legacy",
	"Legacy",
	"snapshot",
	"Snapshot",
	"prior to",
	"before MAX_RAM",
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func lineContainsAny(line string, needles []string) (string, bool) {
	for _, n := range needles {
		if strings.Contains(line, n) {
			return n, true
		}
	}
	return "", false
}

func TestDocs_NoStaleFixed32MBClaim(t *testing.T) {
	for _, doc := range authoritativeDocs {
		t.Run(doc, func(t *testing.T) {
			text := readDoc(t, doc)
			for i, line := range strings.Split(text, "\n") {
				phrase, hit := lineContainsAny(line, staleFixed32MBPhrases)
				if !hit {
					continue
				}
				if _, allowed := lineContainsAny(line, historicalAllowList); allowed {
					continue
				}
				t.Errorf("%s:%d: stale fixed-32-MB claim %q in current-state line: %q",
					doc, i+1, phrase, strings.TrimSpace(line))
			}
		})
	}
}

func TestDocs_NoStaleFixed2GiBClaim(t *testing.T) {
	for _, doc := range authoritativeDocs {
		t.Run(doc, func(t *testing.T) {
			text := readDoc(t, doc)
			for i, line := range strings.Split(text, "\n") {
				phrase, hit := lineContainsAny(line, staleFixed2GiBPhrases)
				if !hit {
					continue
				}
				if _, allowed := lineContainsAny(line, historicalAllowList); allowed {
					continue
				}
				t.Errorf("%s:%d: stale fixed-2-GiB claim %q: %q",
					doc, i+1, phrase, strings.TrimSpace(line))
			}
		})
	}
}

func TestDocs_MentionAutodetectModel(t *testing.T) {
	for _, doc := range authoritativeDocs {
		t.Run(doc, func(t *testing.T) {
			text := readDoc(t, doc)
			if _, ok := lineContainsAny(text, requiredAutodetectMarkers); !ok {
				t.Errorf("%s: missing autodetected-RAM marker; expected at least one of %v",
					doc, requiredAutodetectMarkers)
			}
		})
	}
}
