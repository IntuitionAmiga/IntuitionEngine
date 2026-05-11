package main

import (
	"strconv"
	"strings"
)

// macroDef holds a captured macro body. Body lines are raw (\1..\9 / \@
// markers preserved); substitution happens at expansion time for positional
// args, at emit time for \@ (so rept inside a macro body sees the inner
// counter, not the macro's).
type macroDef struct {
	name string
	body []string
}

// substituteMacroArgs replaces \1..\9 in `line` with args[N-1] (empty if out
// of range). \@ is preserved verbatim (resolved at emit time). \? and \<name>
// are explicit errors per plan §Scope boundaries.
func substituteMacroArgs(line string, args []string) (string, string) {
	var sb strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			c := line[i+1]
			switch {
			case c >= '1' && c <= '9':
				idx := int(c-'0') - 1
				if idx < len(args) {
					sb.WriteString(args[idx])
				}
				i++
				continue
			case c == '@':
				sb.WriteByte('\\')
				sb.WriteByte('@')
				i++
				continue
			case c == '?':
				return "", "\\? (devpac alt-arg) not supported"
			case c == '<':
				return "", "\\<name> named-arg macros not supported"
			}
		}
		sb.WriteByte(line[i])
	}
	return sb.String(), ""
}

// resolveAt rewrites every `\@` in `line` to `_<N>` where N is the current
// atStack top. Returns the line untouched if no \@ markers present.
func (p *preprocCtx) resolveAt(line string) string {
	if !strings.Contains(line, "\\@") {
		return line
	}
	at := -1
	if len(p.atStack) > 0 {
		at = p.atStack[len(p.atStack)-1]
	}
	return strings.ReplaceAll(line, "\\@", "_"+strconv.Itoa(at))
}

// parseMacroInvocation extracts the arg list from a raw line that invokes
// macro `name`. The line is whitespace-prefix-tolerant; comments stripped.
func parseMacroInvocation(raw, name string) []string {
	s := strings.TrimLeft(raw, " \t")
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, strings.ToLower(name)) {
		s = s[len(name):]
		s = strings.TrimPrefix(s, ":")
	}
	s = strings.TrimLeft(s, " \t")
	code, _ := SplitComment(s)
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	return SplitOperands(code)
}

// captureMacro consumes lines starting at `start` (which is the `macro`
// directive). Returns the captured body slice and the index of the matching
// `endm` line (so the caller can resume after it). On unterminated capture
// returns endIdx = len(lines) and an error.
func captureMacro(lines []string, start int) (body []string, endIdx int, ok bool) {
	depth := 1
	for i := start + 1; i < len(lines); i++ {
		l := LexLine(lines[i])
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "macro":
				depth++
			case "endm":
				depth--
				if depth == 0 {
					return body, i, true
				}
			}
		}
		body = append(body, lines[i])
	}
	return body, len(lines), false
}

// captureRept consumes lines starting at `start` (the `rept` directive) until
// matching `endr`. Nested rept blocks bump depth.
func captureRept(lines []string, start int) (body []string, endIdx int, ok bool) {
	depth := 1
	for i := start + 1; i < len(lines); i++ {
		l := LexLine(lines[i])
		if l.Kind == LineDirective {
			switch l.Mnemonic {
			case "rept":
				depth++
			case "endr":
				depth--
				if depth == 0 {
					return body, i, true
				}
			}
		}
		body = append(body, lines[i])
	}
	return body, len(lines), false
}
