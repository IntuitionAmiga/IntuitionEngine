// Shared data types for the PRM doc-as-test harness extractor.
//
// Two type families:
//   - cases.json: produced by the extractor, consumed by the Lua runner and
//     the orchestrator child phases.
//   - report.json: produced by the runner + orchestrator phases, consumed by
//     the renderer.
//
// Kept in one file so the JSON contracts are easy to audit.

package main

const (
	KindBasic = "basic"
	KindIES   = "ies"
	KindIemon = "iemon"

	StatusReady = "READY"
	StatusSkip  = "SKIP"

	BasicModeIndependent     = "independent"
	BasicModeCumulative      = "cumulative"
	BasicModeNeedsAnnotation = "needs-annotation"

	IESModeLint     = "lint"
	IESModeRunnable = "runnable"

	IemonStepGo       = "go"
	IemonStepStep     = "step"
	IemonStepBackstep = "backstep"
	IemonStepInspect  = "inspect"
	IemonStepSkip     = "skip"

	ExpectedStrict = "strict"
	ExpectedIgnore = "ignore"
)

// Case is one extracted example, identified by content hash or @prm-id.
type Case struct {
	ID             string      `json:"id"`
	Source         string      `json:"source"`
	FenceStartLine int         `json:"fence_start_line"`
	Kind           string      `json:"kind"`
	Status         string      `json:"status"`
	SkipReason     string      `json:"skip_reason,omitempty"`
	Mode           string      `json:"mode,omitempty"`
	CPU            string      `json:"cpu,omitempty"`
	BasicSteps     []BasicStep `json:"basic_steps,omitempty"`
	IemonSteps     []IemonStep `json:"iemon_steps,omitempty"`
	IESPath        string      `json:"ies_path,omitempty"`
	IESTimeoutS    int         `json:"ies_timeout_s,omitempty"`
	ExpectedStdout string      `json:"expected_stdout,omitempty"`
	APILint        *APILint    `json:"api_lint,omitempty"`
	Notes          string      `json:"notes,omitempty"`
}

// BasicStep is one input line (numbered source, direct command, etc.) plus
// the expected output captured up to the next Ready prompt.
type BasicStep struct {
	Input    string   `json:"input"`
	Expected []string `json:"expected"`
	Loose    bool     `json:"loose,omitempty"`
	// Numbered source lines never produce output; the runner consumes the
	// echo only. Direct-mode lines do.
	ExpectsOutput bool `json:"expects_output,omitempty"`
}

// IemonStep is one monitor command (or skipped placeholder).
type IemonStep struct {
	Cmd          string `json:"cmd"`
	Kind         string `json:"kind"`
	Expected     string `json:"expected"`
	ExpectedMode string `json:"expected_mode"`
	TimeoutMS    int    `json:"timeout_ms,omitempty"`
	SkipReason   string `json:"skip_reason,omitempty"`
	BlocksRest   bool   `json:"blocks_rest,omitempty"`
}

type APILint struct {
	Status  string   `json:"status"`
	Unknown []string `json:"unknown,omitempty"`
}

// CasesFile is the on-disk shape of cases.json.
type CasesFile struct {
	Cases []Case `json:"cases"`
}

// ----- Report types -----

const (
	RStatusPass     = "PASS"
	RStatusFail     = "FAIL"
	RStatusSkip     = "SKIP"
	RStatusError    = "ERROR"
	RStatusLintPass = "LINT_PASS"
	RStatusLintFail = "LINT_FAIL"
	RStatusBlocked  = "BLOCKED"
)

type ReportCase struct {
	ID             string       `json:"id"`
	Source         string       `json:"source"`
	FenceStartLine int          `json:"fence_start_line"`
	Kind           string       `json:"kind"`
	CPU            string       `json:"cpu,omitempty"`
	Status         string       `json:"status"`
	SkipReason     string       `json:"skip_reason,omitempty"`
	Steps          []ReportStep `json:"steps,omitempty"`
	MonitorDump    string       `json:"monitor_dump,omitempty"`
	Error          string       `json:"error,omitempty"`
	APILint        *APILint     `json:"api_lint,omitempty"`
}

type ReportStep struct {
	Input      string   `json:"input,omitempty"`
	Cmd        string   `json:"cmd,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Expected   []string `json:"expected"`
	Actual     []string `json:"actual"`
	Status     string   `json:"status"`
	SkipReason string   `json:"skip_reason,omitempty"`
}

type Report struct {
	Cases []ReportCase `json:"cases"`
}
