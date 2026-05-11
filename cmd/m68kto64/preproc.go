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
	// symtab holds equ/=/set captures plus -D seeds and IE-convenience seeds.
	// Phase B: populated but has no consumer yet; Phase C consumes it for
	// conditional-gate predicates.
	symtab *Symtab
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

	r.trailingNewline = strings.HasSuffix(s, "\n")
	r.lines = strings.Split(s, "\n")

	// Phase B: seed symtab from -D plus IE-convenience defaults; scan for
	// equ/set/= lines and capture values. No emission change yet — symtab
	// is consumed in Phase C.
	r.symtab = NewSymtab()
	if !opts.NoDefaultSeeds {
		_ = r.symtab.SetMutable("IS_IE", 1)
	}
	for k, v := range opts.Defines {
		// -D bindings are mutable per plan §Symbol seeding.
		_ = r.symtab.SetMutable(k, v)
	}
	for i, raw := range r.lines {
		l := LexLine(raw)
		if l.Kind != LineDirective {
			continue
		}
		switch l.Mnemonic {
		case "equ":
			if l.Label == "" || len(l.Operands) == 0 {
				continue
			}
			v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
			if err != nil {
				fmt.Fprintf(stderrW, "%s:%d: equ %s: %v\n", rootPath, i+1, l.Label, err)
				r.errors++
				continue
			}
			if err := r.symtab.SetEqu(l.Label, v); err != nil {
				fmt.Fprintf(stderrW, "%s:%d: %v\n", rootPath, i+1, err)
				r.errors++
			}
		case "set", "=":
			if l.Label == "" || len(l.Operands) == 0 {
				continue
			}
			v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
			if err != nil {
				fmt.Fprintf(stderrW, "%s:%d: %s %s: %v\n", rootPath, i+1, l.Mnemonic, l.Label, err)
				r.errors++
				continue
			}
			if err := r.symtab.SetMutable(l.Label, v); err != nil {
				fmt.Fprintf(stderrW, "%s:%d: %v\n", rootPath, i+1, err)
				r.errors++
			}
		}
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
