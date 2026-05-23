// EhBASIC fence segmentation.
//
// The walk is a tiny state machine over the fence body, splitting on blank
// lines into "groups". Each group is either an independent case (the default
// for direct-mode fences and multi-group fences that look like a sequence
// of unrelated REPL pairs) or one slice of a cumulative case (the whole
// fence is replayed in order).
//
// Line classification per group:
//   - `^\d+\s`  → numbered source line (input, no expected output)
//   - `^\d+$`   → bare-number delete idiom (input) UNLESS the previous input
//                 is expecting output, in which case it is output (`1010`
//                 from `PRINT BIN$(10)` — 02-basic-vocabulary.md:101).
//   - `^[A-Z][A-Z0-9_]*[$%]?(\([^)]*\))?\s*=`  → assignment (input).
//   - bare keyword from KEYWORDS list  → input. Output-producing keywords
//     (RUN | LIST | PRINT | ?) flip the state to "awaiting output".
//   - LIST output  → numbered lines from a `LIST` are expected output, not
//     program input — handled by a sub-state.
//   - anything else  → expected output for the most recent input.
//
// Prompt artifacts: a `Ready` line or a single-dot line is stripped from
// expected output at extract time (the runner's capture loop also strips
// the Ready delimiter).

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	numberedLineRe = regexp.MustCompile(`^\d+\s`)
	bareNumberRe   = regexp.MustCompile(`^\d+\s*$`)
	assignmentRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*[\$%]?\([^)]*\)?\s*=|^[A-Z][A-Z0-9_]*[\$%]?\s*=`)
	listLineRe     = regexp.MustCompile(`^\d+\s+`)
)

// keywords is the subset of EhBASIC tokens that, on their own, look like
// direct-mode input lines. Output-producing forms flip the state to
// awaiting-output.
var keywords = map[string]struct{}{
	"RUN": {}, "LIST": {}, "NEW": {}, "CONT": {}, "CLEAR": {},
	"LOAD": {}, "SAVE": {}, "PRINT": {}, "?": {}, "INPUT": {},
	"FOR": {}, "IF": {}, "LET": {}, "DIM": {}, "DATA": {},
	"READ": {}, "GOTO": {}, "GOSUB": {}, "POKE": {}, "DEF": {},
	"DO": {}, "WHILE": {}, "UNTIL": {}, "LOOP": {}, "ON": {},
	"REM": {}, "STOP": {}, "END": {}, "RETURN": {}, "NEXT": {},
	"DIR": {}, "DEL": {}, "RENAME": {}, "SCREEN": {}, "PALETTE": {},
	"PLOT": {}, "SOUND": {}, "PSET": {}, "COLOR": {}, "PAINT": {},
	"DRAW": {}, "CIRCLE": {}, "LINE": {}, "GET": {}, "PUT": {},
	"WAIT": {}, "WEND": {},
}

// outputKeywords flip awaiting_output to true after the line is sent.
var outputKeywords = map[string]struct{}{
	"RUN": {}, "LIST": {}, "PRINT": {}, "?": {}, "DIR": {},
}

func segmentBasic(f RawFence) ([]Case, error) {
	groups := splitGroups(f.Body)

	mode := f.Directives.Mode
	if mode == "" {
		mode = inferBasicMode(groups)
	}

	if mode == BasicModeNeedsAnnotation {
		// Whole fence becomes a single SKIP case so the report shows the
		// outstanding annotation work.
		id := f.Directives.ID
		if id == "" {
			id = hashID(f.Body)
		}
		return []Case{{
			ID:             id,
			Source:         f.Source,
			FenceStartLine: f.StartLine,
			Kind:           KindBasic,
			Status:         StatusSkip,
			SkipReason:     "needs-annotation",
			Mode:           mode,
		}}, nil
	}

	if mode == BasicModeCumulative {
		// Whole fence is one case; build steps from concatenated groups.
		var allLines []string
		for i, g := range groups {
			if i > 0 {
				allLines = append(allLines, "")
			}
			allLines = append(allLines, g...)
		}
		steps, err := segmentGroup(allLines)
		if err != nil {
			return nil, err
		}
		id := f.Directives.ID
		if id == "" {
			id = hashID(f.Body)
		}
		return []Case{{
			ID:             id,
			Source:         f.Source,
			FenceStartLine: f.StartLine,
			Kind:           KindBasic,
			Status:         StatusReady,
			Mode:           BasicModeCumulative,
			BasicSteps:     steps,
		}}, nil
	}

	// Independent: each group is its own case.
	var cases []Case
	for i, g := range groups {
		steps, err := segmentGroup(g)
		if err != nil {
			return nil, fmt.Errorf("group %d: %w", i, err)
		}
		body := strings.Join(g, "\n")
		id := f.Directives.ID
		if id != "" {
			if len(groups) > 1 {
				id = fmt.Sprintf("%s:%d", id, i)
			}
		} else {
			id = hashID(body)
		}
		cases = append(cases, Case{
			ID:             id,
			Source:         f.Source,
			FenceStartLine: f.StartLine,
			Kind:           KindBasic,
			Status:         StatusReady,
			Mode:           BasicModeIndependent,
			BasicSteps:     steps,
		})
	}
	return cases, nil
}

// splitGroups breaks the fence body on blank lines, dropping pure-blank
// groups. Each returned slice is the lines of one group.
func splitGroups(body string) [][]string {
	var groups [][]string
	var cur []string
	for _, ln := range strings.Split(body, "\n") {
		if strings.TrimSpace(ln) == "" {
			if len(cur) > 0 {
				groups = append(groups, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, ln)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

// inferBasicMode picks a default mode for a fence with no explicit
// `@prm-mode` directive. Multi-group fences that contain numbered source
// lines need an author decision and yield `needs-annotation`.
func inferBasicMode(groups [][]string) string {
	if len(groups) <= 1 {
		return BasicModeIndependent
	}
	hasNumbered := false
	for _, g := range groups {
		for _, ln := range g {
			if numberedLineRe.MatchString(ln) {
				hasNumbered = true
				break
			}
		}
		if hasNumbered {
			break
		}
	}
	if !hasNumbered {
		return BasicModeIndependent
	}
	return BasicModeNeedsAnnotation
}

// segmentGroup converts one group of lines into ordered BasicSteps.
func segmentGroup(lines []string) ([]BasicStep, error) {
	var steps []BasicStep
	var awaiting bool
	var listExpected bool

	flush := func(input string, expects bool) {
		steps = append(steps, BasicStep{
			Input:         input,
			ExpectsOutput: expects,
		})
		awaiting = expects
	}
	appendExpected := func(line string) {
		if len(steps) == 0 {
			return
		}
		steps[len(steps)-1].Expected = append(steps[len(steps)-1].Expected, line)
	}

	for _, raw := range lines {
		// Drop prompt artifacts from the wire form before classification.
		if isPromptArtifact(raw) {
			continue
		}
		ln := raw

		if listExpected {
			// While inside a LIST output, numbered lines are output, not
			// new program lines. A non-numbered line ends the sub-state.
			if listLineRe.MatchString(strings.TrimLeft(ln, " ")) {
				appendExpected(ln)
				continue
			}
			listExpected = false
			awaiting = false
			// Fall through and re-classify.
		}

		if numberedLineRe.MatchString(ln) {
			flush(ln, false)
			continue
		}
		if bareNumberRe.MatchString(ln) {
			if awaiting {
				appendExpected(ln)
				continue
			}
			flush(strings.TrimSpace(ln), false)
			continue
		}

		// Keyword / direct command dispatch.
		first := firstToken(ln)
		key := strings.ToUpper(first)
		if _, ok := keywords[key]; ok {
			expects := false
			if _, isOutput := outputKeywords[key]; isOutput {
				expects = true
			}
			if key == "LIST" {
				listExpected = true
			}
			flush(ln, expects)
			continue
		}
		if assignmentRe.MatchString(ln) {
			flush(ln, false)
			continue
		}

		// Default: expected output for the most recent input.
		if len(steps) == 0 {
			// Output before any input would be a malformed fence; treat as
			// a synthetic empty input so the runner does not panic, and
			// flag via Notes elsewhere.
			steps = append(steps, BasicStep{ExpectsOutput: true})
		}
		appendExpected(ln)
	}

	// Trim a trailing single dot off the last expected block.
	for i := range steps {
		exp := steps[i].Expected
		for len(exp) > 0 && (strings.TrimSpace(exp[len(exp)-1]) == "" ||
			strings.TrimSpace(exp[len(exp)-1]) == ".") {
			exp = exp[:len(exp)-1]
		}
		steps[i].Expected = exp
	}
	return steps, nil
}

func isPromptArtifact(line string) bool {
	t := strings.TrimSpace(line)
	return t == "Ready" || t == "."
}

func firstToken(s string) string {
	t := strings.TrimLeft(s, " \t")
	end := len(t)
	for i, r := range t {
		if r == ' ' || r == '\t' || r == ':' {
			end = i
			break
		}
	}
	return t[:end]
}
