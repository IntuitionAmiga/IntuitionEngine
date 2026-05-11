package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// PreprocOpts carries CLI-level preprocessor configuration. Phase A scaffolds
// the full surface; per-flag activation is phased (see plan §CLI):
//   - IncludeDirs   activated Phase D
//   - Defines       activated Phase B
//   - NoDefaultSeeds activated Phase B
//   - StripCond     activated Phase C
//   - MaxMacroRecurs activated Phase E
//   - WerrorUnknownMnem activated Phase E
type PreprocOpts struct {
	IncludeDirs       []string
	Defines           map[string]int64
	StripCond         bool
	MaxMacroRecurs    int
	WerrorUnknownMnem bool
	NoDefaultSeeds    bool
}

// DefaultPreprocOpts returns Opts with vasm-compatible defaults.
func DefaultPreprocOpts() PreprocOpts {
	return PreprocOpts{
		MaxMacroRecurs:    1000,
		WerrorUnknownMnem: true,
	}
}

// preprocResult is the output of Preprocess.
type preprocResult struct {
	lines           []string
	trailingNewline bool
	errors          int
}

// Preprocess runs the vasm/devpac preprocessor over `data` rooted at `rootPath`
// (used for relative include resolution). Phase A: stub — performs only the
// line-split contract (CRLF→LF normalize, lone-\r reject, trailing-newline
// state capture). Subsequent phases layer condition/macro/include handling on
// top without disturbing this contract.
//
// Errors are written to stderrW. The returned int is the error count.
func Preprocess(data []byte, rootPath string, opts PreprocOpts, stderrW io.Writer) (preprocResult, int) {
	r := preprocResult{}
	s := string(data)
	// Reject lone CR (classic Mac line endings) per plan §Line-split contract.
	// CRLF is normalized; isolated CR is an error.
	if idx := strings.IndexByte(s, '\r'); idx >= 0 {
		// Scan: any \r not immediately followed by \n is a lone-\r error.
		for i := 0; i < len(s); i++ {
			if s[i] == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
				fmt.Fprintf(stderrW, "%s: lone CR (classic Mac line ending) not supported; convert to LF or CRLF\n", rootPath)
				r.errors++
				return r, r.errors
			}
		}
		_ = idx
	}
	// Normalize CRLF → LF.
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Capture trailing-newline state so ConvertFile can faithfully reproduce
	// the byte-for-byte shape of the legacy ConvertSource(string(data)) path.
	if strings.HasSuffix(s, "\n") {
		r.trailingNewline = true
		// Strip the final \n so Split doesn't yield a phantom trailing "".
		// We re-append it via the trailingNewline flag at emit time.
		// NOTE: legacy strings.Split keeps the trailing "" element, which
		// ConvertLines processes as a final empty line emitting "\n". To
		// stay byte-identical we mirror that exact shape: keep the trailing
		// empty element.
		r.lines = strings.Split(s, "\n")
	} else {
		r.lines = strings.Split(s, "\n")
	}
	return r, r.errors
}

// ConvertFile reads `path`, runs the preprocessor with `opts`, then routes the
// expanded lines through ConvertLines. Returns the emitted IE64 source plus
// the combined preprocessor + converter error count.
func (c *Converter) ConvertFile(path string, opts PreprocOpts, stderrW io.Writer) (string, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderrW, "error reading %s: %v\n", path, err)
		return "", 1
	}
	pre, perrs := Preprocess(data, path, opts, stderrW)
	if perrs > 0 {
		return "", perrs
	}
	out, cerrs := c.ConvertLines(pre.lines)
	return out, perrs + cerrs
}
