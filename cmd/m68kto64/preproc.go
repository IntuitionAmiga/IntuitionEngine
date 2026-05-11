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
	symtab          *Symtab
}

// condFrame tracks one level of conditional nesting during preprocessor walk.
type condFrame struct {
	parentActive bool // combined-active state of the enclosing frame
	own          bool // current branch's predicate result
	taken        bool // some prior branch in this if-chain has been true
	inElse       bool // we've seen `else` for this chain
}

func (f condFrame) active() bool { return f.parentActive && f.own }

// Preprocess runs the vasm/devpac preprocessor over `data` rooted at `rootPath`.
// Phase C: conditional rewriting (Model A by default; Model B under
// opts.StripCond). Symbols captured via equ/=/set are confined to active
// branches; ifd/ifnd/ifeq/ifne/if/elseif* lower to `if N`/`elseif N` using the
// transpile-time symtab.
func Preprocess(data []byte, rootPath string, opts PreprocOpts, stderrW io.Writer) (preprocResult, int) {
	r := preprocResult{}
	s := string(data)
	// Reject lone CR (classic Mac line endings).
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
			fmt.Fprintf(stderrW, "%s: lone CR (classic Mac line ending) not supported; convert to LF or CRLF\n", rootPath)
			r.errors++
			return r, r.errors
		}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	r.trailingNewline = strings.HasSuffix(s, "\n")
	inputLines := strings.Split(s, "\n")

	r.symtab = NewSymtab()
	if !opts.NoDefaultSeeds {
		_ = r.symtab.SetMutable("IS_IE", 1)
	}
	for k, v := range opts.Defines {
		_ = r.symtab.SetMutable(k, v)
	}

	condStack := []condFrame{}
	topActive := func() bool {
		if len(condStack) == 0 {
			return true
		}
		return condStack[len(condStack)-1].active()
	}

	out := make([]string, 0, len(inputLines))
	emit := func(line string) { out = append(out, line) }

	errAt := func(lineNum int, format string, args ...interface{}) {
		fmt.Fprintf(stderrW, "%s:%d: "+format+"\n", append([]interface{}{rootPath, lineNum}, args...)...)
		r.errors++
	}

	for i, raw := range inputLines {
		lineNum := i + 1
		l := LexLine(raw)

		// Conditional directives modify state even in inactive branches; all
		// other directives and instructions are gated.
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "if", "ifd", "ifnd", "ifeq", "ifne", "ifb", "ifnb":
				parentActive := topActive()
				var own bool
				switch l.Mnemonic {
				case "if":
					own = false
					if parentActive {
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "if: %v", err)
						}
						own = v != 0
					}
				case "ifd":
					name := ""
					if len(l.Operands) == 1 {
						name = strings.TrimSpace(l.Operands[0])
					}
					own = r.symtab.Has(name)
				case "ifnd":
					name := ""
					if len(l.Operands) == 1 {
						name = strings.TrimSpace(l.Operands[0])
					}
					own = !r.symtab.Has(name)
				case "ifeq":
					own = false
					if parentActive {
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "ifeq: %v", err)
						}
						own = v == 0
					}
				case "ifne":
					own = false
					if parentActive {
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "ifne: %v", err)
						}
						own = v != 0
					}
				case "ifb":
					// ifb \N is a macro-body construct (Phase E). Outside a
					// macro, treat \N as the literal string; only blank if
					// operand is whitespace/empty.
					arg := ""
					if len(l.Operands) == 1 {
						arg = strings.TrimSpace(l.Operands[0])
					}
					own = arg == ""
				case "ifnb":
					arg := ""
					if len(l.Operands) == 1 {
						arg = strings.TrimSpace(l.Operands[0])
					}
					own = arg != ""
				}
				frame := condFrame{parentActive: parentActive, own: own, taken: own}
				condStack = append(condStack, frame)
				if !opts.StripCond {
					if frame.active() {
						emit("if 1")
					} else {
						emit("if 0")
					}
				}
				continue
			case "elseif", "elseifd", "elseifnd", "elseifeq", "elseifne":
				if len(condStack) == 0 {
					errAt(lineNum, "%s without matching if", l.Mnemonic)
					continue
				}
				top := &condStack[len(condStack)-1]
				if top.inElse {
					errAt(lineNum, "%s after else", l.Mnemonic)
					continue
				}
				var pred bool
				if top.parentActive && !top.taken {
					switch l.Mnemonic {
					case "elseif":
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "elseif: %v", err)
						}
						pred = v != 0
					case "elseifd":
						name := ""
						if len(l.Operands) == 1 {
							name = strings.TrimSpace(l.Operands[0])
						}
						pred = r.symtab.Has(name)
					case "elseifnd":
						name := ""
						if len(l.Operands) == 1 {
							name = strings.TrimSpace(l.Operands[0])
						}
						pred = !r.symtab.Has(name)
					case "elseifeq":
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "elseifeq: %v", err)
						}
						pred = v == 0
					case "elseifne":
						v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
						if err != nil {
							errAt(lineNum, "elseifne: %v", err)
						}
						pred = v != 0
					}
				}
				top.own = pred
				if pred {
					top.taken = true
				}
				if !opts.StripCond {
					if pred {
						emit("elseif 1")
					} else {
						emit("elseif 0")
					}
				}
				continue
			case "else":
				if len(condStack) == 0 {
					errAt(lineNum, "else without matching if")
					continue
				}
				top := &condStack[len(condStack)-1]
				if top.inElse {
					errAt(lineNum, "duplicate else")
					continue
				}
				top.inElse = true
				if top.parentActive && !top.taken {
					top.own = true
					top.taken = true
				} else {
					top.own = false
				}
				if !opts.StripCond {
					emit("else")
				}
				continue
			case "endc", "endif":
				if len(condStack) == 0 {
					errAt(lineNum, "%s without matching if", l.Mnemonic)
					continue
				}
				condStack = condStack[:len(condStack)-1]
				if !opts.StripCond {
					emit("endif")
				}
				continue
			}
		}

		// Beyond this point we're not on a conditional directive. If the
		// current frame is inactive, drop the line under Model B; keep it
		// under Model A (let ie64asm-level wrapper gating handle skipping).
		if !topActive() {
			if opts.StripCond {
				continue
			}
			emit(raw)
			continue
		}

		// Active branch: capture equ/set/= into symtab (vasm: only active
		// branches affect symbol state).
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "equ":
				if l.Label != "" && len(l.Operands) > 0 {
					v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
					if err != nil {
						errAt(lineNum, "equ %s: %v", l.Label, err)
					} else if err := r.symtab.SetEqu(l.Label, v); err != nil {
						errAt(lineNum, "%v", err)
					}
				}
			case "set", "=":
				if l.Label != "" && len(l.Operands) > 0 {
					v, err := EvalExpr(strings.Join(l.Operands, ","), r.symtab)
					if err != nil {
						errAt(lineNum, "%s %s: %v", l.Mnemonic, l.Label, err)
					} else if err := r.symtab.SetMutable(l.Label, v); err != nil {
						errAt(lineNum, "%v", err)
					}
				}
			}
		}
		emit(raw)
	}

	if len(condStack) > 0 {
		errAt(len(inputLines), "unterminated if-chain (%d frame(s) open)", len(condStack))
	}

	r.lines = out
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
	// Hand the populated symtab to the converter so ifd/ifnd in any leftover
	// directives (e.g. inside macro bodies that pass through verbatim
	// pre-Phase-E) see the same defined-ness view.
	c.symtab = pre.symtab
	out, cerrs := c.ConvertLines(pre.lines)
	return out, perrs + cerrs
}
