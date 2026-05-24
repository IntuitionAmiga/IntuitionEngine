// Markdown walker for PRM doc-as-test extraction.
//
// Each chapter is scanned for fenced code blocks. The contiguous run of
// HTML-comment lines immediately above a fence carries optional
// `<!-- @prm-* -->` directives that bind to that fence. A blank line or
// any non-`@prm-*` HTML comment breaks the run.
//
// Fence classification (in priority order):
//   1. explicit info-string class (basic | ies | iemon)
//   2. `<!-- @prm-fence: <class> -->` override
//   3. `<!-- @prm-cpu: <cpu> -->` implies iemon
//   4. auto-detect: first non-blank body line matches `^\((cpu)>` → iemon
//
// Anything else is ignored.

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FenceDirectives is everything parsed out of the contiguous comment block
// directly above the opening triple-backtick line.
type FenceDirectives struct {
	Fence       string
	CPU         string
	Mode        string
	ID          string
	TimeoutS    int
	Strict      bool
	IgnoreSetup bool
	Expect      string
	Runnable    bool
	RunnableS   int
}

// RawFence is one fenced block as extracted from the markdown source.
type RawFence struct {
	Source     string
	StartLine  int
	InfoString string
	Body       string
	Directives FenceDirectives
}

type commentLine struct {
	num  int
	text string
}

var promptRe = regexp.MustCompile(`^\((ie64|ie32|6502|z80|m68k|x86)\)>`)

var directiveRe = regexp.MustCompile(`^<!--\s*@prm-([a-z][a-z0-9-]*)(?::\s*([^>]*?))?\s*-->\s*$`)

var htmlCommentRe = regexp.MustCompile(`^<!--.*-->\s*$`)

// extractFile reads a markdown file and returns every fence with its
// resolved class.
func extractFile(path, repoRoot string) ([]RawFence, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)

	var fences []RawFence
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	var (
		prevCommentBlock []commentLine
		lineNum          int
		inFence          bool
		fenceStart       int
		fenceInfo        string
		fenceBody        strings.Builder
		fenceDirectives  FenceDirectives
	)

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()

		if !inFence {
			trim := strings.TrimRight(raw, " \t")
			if strings.HasPrefix(trim, "```") {
				info := strings.TrimSpace(strings.TrimPrefix(trim, "```"))
				dirs := parseDirectiveBlock(prevCommentBlock)
				inFence = true
				fenceStart = lineNum
				fenceInfo = info
				fenceBody.Reset()
				fenceDirectives = dirs
				prevCommentBlock = nil
				continue
			}
			if htmlCommentRe.MatchString(trim) {
				prevCommentBlock = append(prevCommentBlock, commentLine{num: lineNum, text: trim})
			} else {
				// Either blank line or other content. Both break the block.
				prevCommentBlock = nil
			}
			continue
		}

		if strings.TrimRight(raw, " \t") == "```" {
			fences = append(fences, RawFence{
				Source:     rel,
				StartLine:  fenceStart,
				InfoString: fenceInfo,
				Body:       strings.TrimRight(fenceBody.String(), "\n"),
				Directives: fenceDirectives,
			})
			inFence = false
			fenceBody.Reset()
			continue
		}

		fenceBody.WriteString(raw)
		fenceBody.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return fences, nil
}

func parseDirectiveBlock(lines []commentLine) FenceDirectives {
	var d FenceDirectives
	for _, ln := range lines {
		m := directiveRe.FindStringSubmatch(ln.text)
		if m == nil {
			// Non-directive HTML comment breaks the binding chain — every
			// directive *above* it detaches, only ones *below* still bind.
			d = FenceDirectives{}
			continue
		}
		name, val := m[1], strings.TrimSpace(m[2])
		switch name {
		case "fence":
			d.Fence = val
		case "cpu":
			d.CPU = val
		case "mode":
			d.Mode = val
		case "id":
			d.ID = val
		case "timeout":
			d.TimeoutS = parseTimeoutSeconds(val)
		case "strict":
			d.Strict = true
		case "ignore-setup-output":
			d.IgnoreSetup = true
		case "expect":
			d.Expect = val
		case "runnable":
			d.Runnable = true
			if val != "" {
				d.RunnableS = parseTimeoutSeconds(val)
			}
		}
	}
	return d
}

func parseTimeoutSeconds(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "s")
	if s == "" {
		return 0
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// classifyFence applies the priority order to decide kind, and resolves
// the iemon CPU when applicable.
func classifyFence(f RawFence) (kind, cpu string) {
	switch f.Directives.Fence {
	case KindBasic, KindIES, KindIemon:
		kind = f.Directives.Fence
	}
	if kind == "" {
		switch f.InfoString {
		case KindBasic, KindIES, KindIemon:
			kind = f.InfoString
		}
	}
	if kind == "" && f.Directives.CPU != "" {
		kind = KindIemon
	}
	if kind == "" && f.InfoString != "" {
		// Unknown info strings such as `text` are an explicit opt-out from
		// auto-detection. This lets the manual show explanatory transcripts
		// for interactive-only features that the PRM runner cannot feed.
		return
	}
	if kind == "" {
		for _, ln := range strings.Split(f.Body, "\n") {
			t := strings.TrimSpace(ln)
			if t == "" {
				continue
			}
			if m := promptRe.FindStringSubmatch(t); m != nil {
				kind = KindIemon
				cpu = m[1]
			}
			break
		}
		return
	}
	if kind == KindIemon {
		if f.Directives.CPU != "" {
			cpu = f.Directives.CPU
			return
		}
		for _, ln := range strings.Split(f.Body, "\n") {
			t := strings.TrimSpace(ln)
			if t == "" {
				continue
			}
			if m := promptRe.FindStringSubmatch(t); m != nil {
				cpu = m[1]
			}
			break
		}
	}
	return
}

func hashID(body string) string {
	h := sha256.Sum256([]byte(normalizeForHash(body)))
	return hex.EncodeToString(h[:])[:12]
}

func normalizeForHash(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// extractAll walks every matching markdown file under refmanDir, classifies
// every fence, dispatches to the per-kind segmenter, and emits a CasesFile.
// Duplicate IDs abort with a clear error listing all collision sites.
func extractAll(refmanDir, glob, repoRoot string) (*CasesFile, error) {
	pattern := filepath.Join(refmanDir, glob+".md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}
	sort.Strings(matches)

	all := &CasesFile{}
	idSites := map[string][]string{}

	for _, path := range matches {
		fences, err := extractFile(path, repoRoot)
		if err != nil {
			return nil, err
		}
		for _, f := range fences {
			kind, cpu := classifyFence(f)
			if kind == "" {
				continue
			}
			cases, err := segmentFence(f, kind, cpu)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", f.Source, f.StartLine, err)
			}
			for _, c := range cases {
				site := fmt.Sprintf("%s:%d", c.Source, c.FenceStartLine)
				if c.ID != "" {
					idSites[c.ID] = append(idSites[c.ID], site)
				}
				all.Cases = append(all.Cases, c)
			}
		}
	}

	var collisions []string
	for id, sites := range idSites {
		if len(sites) > 1 {
			collisions = append(collisions, fmt.Sprintf("  id %s: %s", id, strings.Join(sites, ", ")))
		}
	}
	if len(collisions) > 0 {
		sort.Strings(collisions)
		return nil, fmt.Errorf("duplicate case IDs detected (add `<!-- @prm-id: ... -->` to disambiguate):\n%s", strings.Join(collisions, "\n"))
	}
	return all, nil
}

func segmentFence(f RawFence, kind, cpu string) ([]Case, error) {
	switch kind {
	case KindBasic:
		return segmentBasic(f)
	case KindIemon:
		return segmentIemon(f, cpu)
	case KindIES:
		return segmentIES(f)
	}
	return nil, fmt.Errorf("unknown kind %q", kind)
}
