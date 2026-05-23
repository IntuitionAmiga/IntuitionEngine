// IE Monitor (iemon) transcript segmentation.
//
// The fence body is walked top-down. A line that matches `^\((cpu)>\s`
// (bracketed form) or `^>\s` (bare form, requires @prm-cpu) is a command;
// every line until the next command is its expected output. Each command
// becomes one IemonStep, classified into go|step|backstep|inspect|skip.
//
// CPU detection:
//   - explicit `@prm-cpu: <cpu>`
//   - else the bracketed CPU on the first command line
//   - else SKIP iemon-cpu-unspecified
//
// Bracketed CPUs must agree fence-wide; mixed CPUs are rejected as
// inconsistent (returned as a synthetic SKIP rather than fatal so the
// other chapters keep running).

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	bareCmdRe       = regexp.MustCompile(`^>\s`)
	registerWriteRe = regexp.MustCompile(`^r\s+[A-Za-z_][A-Za-z0-9_]*\s+\S+`)
	bpmCmdRe        = regexp.MustCompile(`^bpm[a-z]+$`)
)

// setupAllowlist is the set of monitor commands that print a noisy
// confirmation line (`Breakpoint set at $X`, `Wrote N byte(s) at $X`,
// etc.) which docs typically omit. When the doc shows them with empty
// expected output, the step runs in `ignore` mode and passes as long as
// the monitor does not return a red error row.
var setupAllowlist = map[string]struct{}{
	"w":  {},
	"b":  {},
	"bc": {},
	"ww": {},
	"wr": {}, "wrw": {},
	"wc": {},
}

const defaultIemonTimeoutMS = 5000

// segmentIemon converts an iemon fence into one Case (one fence = one
// case, since CPU is fence-locked).
func segmentIemon(f RawFence, cpu string) ([]Case, error) {
	id := f.Directives.ID
	if id == "" {
		id = hashID(f.Body)
	}
	timeoutMS := defaultIemonTimeoutMS
	if f.Directives.TimeoutS > 0 {
		timeoutMS = f.Directives.TimeoutS * 1000
	}
	base := Case{
		ID:             id,
		Source:         f.Source,
		FenceStartLine: f.StartLine,
		Kind:           KindIemon,
		CPU:            cpu,
	}

	// Walk lines, accumulate commands + expected blocks.
	type pair struct {
		cmd      string
		expected []string
	}
	var pairs []pair
	var (
		curCmd      string
		curExpected []string
		hasCmd      bool
	)
	flush := func() {
		if hasCmd {
			pairs = append(pairs, pair{cmd: curCmd, expected: curExpected})
		}
		curCmd = ""
		curExpected = nil
		hasCmd = false
	}

	allowBare := f.Directives.CPU != ""

	for _, raw := range strings.Split(f.Body, "\n") {
		line := raw
		// Bracketed form: `(cpu)> cmd`
		if m := promptRe.FindStringSubmatch(line); m != nil {
			lineCPU := m[1]
			if cpu != "" && lineCPU != cpu {
				return []Case{{
					ID:             id,
					Source:         f.Source,
					FenceStartLine: f.StartLine,
					Kind:           KindIemon,
					Status:         StatusSkip,
					SkipReason:     "iemon-mixed-cpu",
				}}, nil
			}
			cpu = lineCPU
			base.CPU = cpu
			rest := line[len(m[0]):]
			rest = strings.TrimLeft(rest, " \t")
			flush()
			curCmd = rest
			hasCmd = true
			continue
		}
		// Bare form: `> cmd` only if @prm-cpu is set.
		if allowBare && bareCmdRe.MatchString(line) {
			flush()
			curCmd = strings.TrimLeft(line[1:], " \t")
			hasCmd = true
			continue
		}
		if hasCmd {
			curExpected = append(curExpected, line)
		}
	}
	flush()

	if cpu == "" || len(pairs) == 0 {
		return []Case{{
			ID:             id,
			Source:         f.Source,
			FenceStartLine: f.StartLine,
			Kind:           KindIemon,
			Status:         StatusSkip,
			SkipReason:     "iemon-cpu-unspecified",
		}}, nil
	}

	// Normalize expected blocks: strip trailing blank lines so empty
	// expected is unambiguous.
	for i := range pairs {
		for len(pairs[i].expected) > 0 && strings.TrimSpace(pairs[i].expected[len(pairs[i].expected)-1]) == "" {
			pairs[i].expected = pairs[i].expected[:len(pairs[i].expected)-1]
		}
	}

	// Classify each step.
	var steps []IemonStep
	for _, p := range pairs {
		steps = append(steps, classifyIemonCommand(p.cmd, p.expected, f.Directives, timeoutMS))
	}

	base.Status = StatusReady
	base.IemonSteps = steps
	return []Case{base}, nil
}

func classifyIemonCommand(cmd string, expected []string, dirs FenceDirectives, timeoutMS int) IemonStep {
	cmd = strings.TrimSpace(cmd)
	expectedJoined := strings.Join(expected, "\n")
	hasExpected := len(expected) > 0
	name, args := splitCmd(cmd)

	step := IemonStep{
		Cmd:       cmd,
		Expected:  expectedJoined,
		TimeoutMS: timeoutMS,
	}

	// Skip-class dispatch first.
	switch name {
	case "save", "load", "ss", "sl", "script", "macro":
		step.Kind = IemonStepSkip
		step.SkipReason = "iemon-host-file-command"
		step.BlocksRest = true
		return step
	case "trace":
		if len(args) >= 1 && strings.EqualFold(args[0], "file") {
			if len(args) >= 2 && strings.EqualFold(args[1], "off") {
				// `trace file off` is allowed by the sandbox validator.
				return finalizeMode(IemonStep{Cmd: cmd, Kind: IemonStepInspect,
					Expected: expectedJoined, TimeoutMS: timeoutMS},
					expected, dirs, false)
			}
			step.Kind = IemonStepSkip
			step.SkipReason = "iemon-host-file-command"
			step.BlocksRest = false
			return step
		}
		// other trace subcommands → inspect (no state mutation we cannot diff).
		return finalizeMode(IemonStep{Cmd: cmd, Kind: IemonStepInspect,
			Expected: expectedJoined, TimeoutMS: timeoutMS},
			expected, dirs, false)
	case "g":
		if len(args) >= 1 {
			step.Kind = IemonStepSkip
			step.SkipReason = "iemon-g-addr-needs-validated-go-api"
			step.BlocksRest = true
			return step
		}
		if hasExpected {
			step.Kind = IemonStepSkip
			step.SkipReason = "iemon-g-postrun-capture-unsupported"
			step.BlocksRest = true
			return step
		}
		step.Kind = IemonStepGo
		step.ExpectedMode = ExpectedStrict
		return step
	case "u":
		step.Kind = IemonStepSkip
		step.SkipReason = "iemon-u-needs-core-api"
		step.BlocksRest = true
		return step
	case "s":
		return finalizeMode(IemonStep{Cmd: cmd, Kind: IemonStepStep,
			Expected: expectedJoined, TimeoutMS: timeoutMS},
			expected, dirs, isSetupCommand(name, args))
	case "bs":
		return finalizeMode(IemonStep{Cmd: cmd, Kind: IemonStepBackstep,
			Expected: expectedJoined, TimeoutMS: timeoutMS},
			expected, dirs, isSetupCommand(name, args))
	}

	// Inspect-class catch-all.
	return finalizeMode(IemonStep{Cmd: cmd, Kind: IemonStepInspect,
		Expected: expectedJoined, TimeoutMS: timeoutMS},
		expected, dirs, isSetupCommand(name, args))
}

// finalizeMode picks expected_mode (strict vs ignore) for non-skip steps.
func finalizeMode(step IemonStep, expected []string, dirs FenceDirectives, isSetup bool) IemonStep {
	if dirs.Strict {
		step.ExpectedMode = ExpectedStrict
		return step
	}
	if len(expected) > 0 {
		step.ExpectedMode = ExpectedStrict
		return step
	}
	if dirs.IgnoreSetup || isSetup {
		step.ExpectedMode = ExpectedIgnore
		return step
	}
	step.ExpectedMode = ExpectedStrict
	return step
}

// isSetupCommand identifies commands on the empty-expected allowlist.
func isSetupCommand(name string, args []string) bool {
	if _, ok := setupAllowlist[name]; ok {
		return true
	}
	if bpmCmdRe.MatchString(name) {
		return true
	}
	if name == "r" && len(args) >= 2 {
		return true
	}
	return false
}

// splitCmd splits a monitor command into name + args, handling extra
// whitespace.
func splitCmd(cmd string) (string, []string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", nil
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// formatCmdName is a tiny helper for error messages; the underscore avoids
// a "declared and not used" if every call site is removed.
var _ = fmt.Sprintf
