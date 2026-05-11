package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PreprocOpts carries CLI-level preprocessor configuration.
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
	parentActive bool
	own          bool
	taken        bool
	inElse       bool
}

func (f condFrame) active() bool { return f.parentActive && f.own }

// preprocCtx threads state across recursive include / macro processing.
type preprocCtx struct {
	opts        PreprocOpts
	symtab      *Symtab
	stderrW     io.Writer
	condStack   []condFrame
	fileStack   []string
	out         []string
	errors      int
	macros      map[string]*macroDef
	uniqCounter int   // global monotonic, never reset
	expandDepth int   // current macro-expansion depth (vs MaxMacroRecurs)
	atStack     []int // \@ resolution stack (innermost wins)
}

func (p *preprocCtx) topActive() bool {
	if len(p.condStack) == 0 {
		return true
	}
	return p.condStack[len(p.condStack)-1].active()
}

func (p *preprocCtx) emit(s string) { p.out = append(p.out, s) }

func (p *preprocCtx) errAt(source string, lineNum int, format string, args ...interface{}) {
	fmt.Fprintf(p.stderrW, "%s:%d: "+format+"\n", append([]interface{}{source, lineNum}, args...)...)
	p.errors++
}

func (p *preprocCtx) resolveInclude(name, includerPath string) (string, error) {
	name = strings.Trim(name, "\"'")
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			abs, _ := filepath.Abs(name)
			return abs, nil
		}
		return "", fmt.Errorf("include %q not found", name)
	}
	if includerPath != "" {
		cand := filepath.Join(filepath.Dir(includerPath), name)
		if _, err := os.Stat(cand); err == nil {
			abs, _ := filepath.Abs(cand)
			return abs, nil
		}
	}
	for _, dir := range p.opts.IncludeDirs {
		cand := filepath.Join(dir, name)
		if _, err := os.Stat(cand); err == nil {
			abs, _ := filepath.Abs(cand)
			return abs, nil
		}
	}
	return "", fmt.Errorf("include %q not found (searched %d -I path(s))", name, len(p.opts.IncludeDirs))
}

func (p *preprocCtx) processFile(path string) {
	for _, on := range p.fileStack {
		if on == path {
			fmt.Fprintf(p.stderrW, "include cycle: %s already on stack (%v)\n", path, p.fileStack)
			p.errors++
			return
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(p.stderrW, "include %s: %v\n", path, err)
		p.errors++
		return
	}
	s := string(data)
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
			fmt.Fprintf(p.stderrW, "%s: lone CR (classic Mac line ending) not supported\n", path)
			p.errors++
			return
		}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	p.fileStack = append(p.fileStack, path)
	p.processLines(lines, path)
	p.fileStack = p.fileStack[:len(p.fileStack)-1]
}

// detectMacroInvocation returns (canonical name, args, true) if `l` (lexed from
// `raw`) is a call to a registered macro.
func (p *preprocCtx) detectMacroInvocation(l Line, raw string) (string, []string, bool) {
	if l.Mnemonic != "" {
		if _, ok := p.macros[strings.ToLower(l.Mnemonic)]; ok {
			return l.Mnemonic, l.Operands, true
		}
	}
	if l.Label != "" {
		if _, ok := p.macros[strings.ToLower(l.Label)]; ok {
			return l.Label, parseMacroInvocation(raw, l.Label), true
		}
	}
	return "", nil, false
}

func (p *preprocCtx) expandMacro(name string, args []string, source string, lineNum int) {
	p.expandDepth++
	defer func() { p.expandDepth-- }()
	if p.expandDepth > p.opts.MaxMacroRecurs {
		p.errAt(source, lineNum, "macro expansion depth exceeded MAXMACRECURS=%d (likely recursive macro: %s)", p.opts.MaxMacroRecurs, name)
		return
	}
	m := p.macros[strings.ToLower(name)]
	p.uniqCounter++
	atVal := p.uniqCounter
	p.atStack = append(p.atStack, atVal)
	defer func() { p.atStack = p.atStack[:len(p.atStack)-1] }()

	expanded := make([]string, 0, len(m.body))
	for _, line := range m.body {
		sub, errMsg := substituteMacroArgs(line, args)
		if errMsg != "" {
			p.errAt(source, lineNum, "macro %s: %s", name, errMsg)
			return
		}
		l := LexLine(sub)
		if l.Kind == LineDirective && l.Mnemonic == "mexit" {
			break
		}
		expanded = append(expanded, sub)
	}
	p.processLines(expanded, source+":<macro "+name+">")
}

func (p *preprocCtx) processLines(lines []string, source string) {
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		// Resolve \@ markers against the current atStack top. Safe to call
		// unconditionally — no-op when no markers are present.
		raw = p.resolveAt(raw)
		lineNum := i + 1
		l := LexLine(raw)

		// Conditional directives unconditionally modify state.
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "if", "ifd", "ifnd", "ifeq", "ifne", "ifb", "ifnb":
				p.handleIf(l, source, lineNum)
				continue
			case "elseif", "elseifd", "elseifnd", "elseifeq", "elseifne":
				p.handleElseif(l, source, lineNum)
				continue
			case "else":
				p.handleElse(source, lineNum)
				continue
			case "endc", "endif":
				p.handleEndif(source, lineNum)
				continue
			}
		}

		// Inactive branches: macro/rept blocks must still be range-consumed so
		// the matching endm/endr lines do not leak out as stray directives.
		if !p.topActive() {
			if l.Kind == LineDirective {
				switch l.Mnemonic {
				case "macro":
					_, endIdx, ok := captureMacro(lines, i)
					if !ok {
						p.errAt(source, lineNum, "unterminated macro in inactive branch")
						return
					}
					i = endIdx
					continue
				case "rept":
					_, endIdx, ok := captureRept(lines, i)
					if !ok {
						p.errAt(source, lineNum, "unterminated rept in inactive branch")
						return
					}
					i = endIdx
					continue
				case "endm", "endr", "mexit":
					continue
				}
			}
			if p.opts.StripCond {
				continue
			}
			p.emit(raw)
			continue
		}

		// Active branch.
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "macro":
				name := strings.ToLower(l.Label)
				if name == "" {
					p.errAt(source, lineNum, "macro requires a label name")
					continue
				}
				body, endIdx, ok := captureMacro(lines, i)
				if !ok {
					p.errAt(source, lineNum, "unterminated macro %s", name)
					return
				}
				p.macros[name] = &macroDef{name: name, body: body}
				i = endIdx
				continue
			case "endm":
				p.errAt(source, lineNum, "stray endm")
				continue
			case "rept":
				count, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
				if err != nil {
					p.errAt(source, lineNum, "rept: %v", err)
					continue
				}
				body, endIdx, ok := captureRept(lines, i)
				if !ok {
					p.errAt(source, lineNum, "unterminated rept")
					return
				}
				for j := int64(0); j < count; j++ {
					p.uniqCounter++
					p.atStack = append(p.atStack, p.uniqCounter)
					p.processLines(body, source+":<rept>")
					p.atStack = p.atStack[:len(p.atStack)-1]
				}
				i = endIdx
				continue
			case "endr":
				p.errAt(source, lineNum, "stray endr")
				continue
			case "mexit":
				// Outside a macro expansion this is a no-op; expandMacro
				// detects MEXIT in its own loop before lines reach here.
				continue
			case "equ":
				if l.Label != "" && len(l.Operands) > 0 {
					v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
					if err != nil {
						p.errAt(source, lineNum, "equ %s: %v", l.Label, err)
					} else if err := p.symtab.SetEqu(l.Label, v); err != nil {
						p.errAt(source, lineNum, "%v", err)
					}
				}
				p.emit(raw)
				continue
			case "set", "=":
				if l.Label != "" && len(l.Operands) > 0 {
					v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
					if err != nil {
						p.errAt(source, lineNum, "%s %s: %v", l.Mnemonic, l.Label, err)
					} else if err := p.symtab.SetMutable(l.Label, v); err != nil {
						p.errAt(source, lineNum, "%v", err)
					}
				}
				p.emit(raw)
				continue
			case "include":
				if len(l.Operands) != 1 {
					p.errAt(source, lineNum, "include: expected single path operand, got %v", l.Operands)
					continue
				}
				path, err := p.resolveInclude(l.Operands[0], source)
				if err != nil {
					p.errAt(source, lineNum, "%v", err)
					continue
				}
				p.processFile(path)
				continue
			case "incbin":
				p.emit(raw)
				continue
			}
		}

		// Macro invocation? Active-branch only.
		if name, args, isMacro := p.detectMacroInvocation(l, raw); isMacro {
			p.expandMacro(name, args, source, lineNum)
			continue
		}

		p.emit(raw)
	}
}

func (p *preprocCtx) handleIf(l Line, source string, lineNum int) {
	parentActive := p.topActive()
	var own bool
	switch l.Mnemonic {
	case "if":
		if parentActive {
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "if: %v", err)
			}
			own = v != 0
		}
	case "ifd":
		name := ""
		if len(l.Operands) == 1 {
			name = strings.TrimSpace(l.Operands[0])
		}
		own = p.symtab.Has(name)
	case "ifnd":
		name := ""
		if len(l.Operands) == 1 {
			name = strings.TrimSpace(l.Operands[0])
		}
		own = !p.symtab.Has(name)
	case "ifeq":
		if parentActive {
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "ifeq: %v", err)
			}
			own = v == 0
		}
	case "ifne":
		if parentActive {
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "ifne: %v", err)
			}
			own = v != 0
		}
	case "ifb":
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
	p.condStack = append(p.condStack, frame)
	if !p.opts.StripCond {
		if frame.active() {
			p.emit("if 1")
		} else {
			p.emit("if 0")
		}
	}
}

func (p *preprocCtx) handleElseif(l Line, source string, lineNum int) {
	if len(p.condStack) == 0 {
		p.errAt(source, lineNum, "%s without matching if", l.Mnemonic)
		return
	}
	top := &p.condStack[len(p.condStack)-1]
	if top.inElse {
		p.errAt(source, lineNum, "%s after else", l.Mnemonic)
		return
	}
	var pred bool
	if top.parentActive && !top.taken {
		switch l.Mnemonic {
		case "elseif":
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "elseif: %v", err)
			}
			pred = v != 0
		case "elseifd":
			name := ""
			if len(l.Operands) == 1 {
				name = strings.TrimSpace(l.Operands[0])
			}
			pred = p.symtab.Has(name)
		case "elseifnd":
			name := ""
			if len(l.Operands) == 1 {
				name = strings.TrimSpace(l.Operands[0])
			}
			pred = !p.symtab.Has(name)
		case "elseifeq":
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "elseifeq: %v", err)
			}
			pred = v == 0
		case "elseifne":
			v, err := EvalExpr(strings.Join(l.Operands, ","), p.symtab)
			if err != nil {
				p.errAt(source, lineNum, "elseifne: %v", err)
			}
			pred = v != 0
		}
	}
	top.own = pred
	if pred {
		top.taken = true
	}
	if !p.opts.StripCond {
		if pred {
			p.emit("elseif 1")
		} else {
			p.emit("elseif 0")
		}
	}
}

func (p *preprocCtx) handleElse(source string, lineNum int) {
	if len(p.condStack) == 0 {
		p.errAt(source, lineNum, "else without matching if")
		return
	}
	top := &p.condStack[len(p.condStack)-1]
	if top.inElse {
		p.errAt(source, lineNum, "duplicate else")
		return
	}
	top.inElse = true
	if top.parentActive && !top.taken {
		top.own = true
		top.taken = true
	} else {
		top.own = false
	}
	if !p.opts.StripCond {
		p.emit("else")
	}
}

func (p *preprocCtx) handleEndif(source string, lineNum int) {
	if len(p.condStack) == 0 {
		p.errAt(source, lineNum, "endif without matching if")
		return
	}
	p.condStack = p.condStack[:len(p.condStack)-1]
	if !p.opts.StripCond {
		p.emit("endif")
	}
}

// Preprocess runs the vasm/devpac preprocessor over `data` rooted at `rootPath`.
func Preprocess(data []byte, rootPath string, opts PreprocOpts, stderrW io.Writer) (preprocResult, int) {
	r := preprocResult{}
	s := string(data)
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

	ctx := &preprocCtx{
		opts:    opts,
		symtab:  r.symtab,
		stderrW: stderrW,
		macros:  map[string]*macroDef{},
	}
	absRoot, _ := filepath.Abs(rootPath)
	ctx.fileStack = append(ctx.fileStack, absRoot)
	ctx.processLines(inputLines, absRoot)
	ctx.fileStack = ctx.fileStack[:len(ctx.fileStack)-1]

	if len(ctx.condStack) > 0 {
		fmt.Fprintf(stderrW, "%s: unterminated if-chain (%d frame(s) open)\n", rootPath, len(ctx.condStack))
		ctx.errors++
	}

	r.lines = ctx.out
	r.errors += ctx.errors
	return r, r.errors
}

// ConvertFile reads `path`, runs the preprocessor with `opts`, then routes the
// expanded lines through ConvertLines.
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
	c.symtab = pre.symtab
	c.werrorUnknownMnem = opts.WerrorUnknownMnem
	out, cerrs := c.ConvertLines(pre.lines)
	return out, perrs + cerrs
}
