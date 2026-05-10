package main

import (
	"strings"
)

// Phase 4: directive translation.
//
// vasm/devpac directives that share syntax with ie64asm pass through
// verbatim. The few exceptions are translated here:
//
//   - `xdef` / `xref` / `public` / `global` / `extern` -> dropped (single-file
//     namespace; warn if referenced symbol is otherwise undefined)
//   - `ifd IS_IE`  -> `if 1`  (the transpiler always targets IE)
//   - `ifnd IS_IE` -> `if 0`
//   - `endc`       -> `endif` (vasm/devpac alias)
//   - `even`       -> `align 2`  (devpac convention)
//   - `cnop 0,N`   -> `align N`
//
// macro / endm / rept / endr pass through unchanged (ie64asm uses the same
// `\1..\9` syntax). `IS_IE`-conditioned blocks remain bracketed so that
// nested non-IE branches stay disabled.

// emitDirective is the Phase-4 entry point invoked by convertLexed. It
// returns true if the directive was handled here; otherwise the caller falls
// back to verbatim pass-through.
func (c *Converter) emitDirective(e *Emit, l Line) bool {
	mnem := l.Mnemonic
	switch mnem {
	case "xdef", "xref", "public", "global", "extern":
		// Drop linkage directives (single-file namespace).
		if len(l.Operands) > 0 {
			e.Lf("; m68kto64: dropped %s %s (single-file namespace)",
				mnem, strings.Join(l.Operands, ","))
		}
		return true
	case "ifd":
		// ie64asm has no `defined()` predicate. We assume IS_IE is defined
		// (transpiler always targets IE) and any other symbol is not. The
		// user can override by editing the transpiled output or supplying
		// `-define` flags to ie64asm directly.
		if len(l.Operands) == 1 && strings.EqualFold(strings.TrimSpace(l.Operands[0]), "IS_IE") {
			e.L("if 1")
		} else {
			e.L("if 0")
		}
		return true
	case "ifnd":
		if len(l.Operands) == 1 && strings.EqualFold(strings.TrimSpace(l.Operands[0]), "IS_IE") {
			e.L("if 0")
		} else {
			e.L("if 1")
		}
		return true
	case "ifeq":
		// vasm/devpac: assemble if EXPR == 0. ie64asm `if` accepts an
		// expression with `==` so we lower directly.
		expr := strings.TrimSpace(strings.Join(l.Operands, ","))
		e.Lf("if (%s) == 0", expr)
		return true
	case "ifne":
		expr := strings.TrimSpace(strings.Join(l.Operands, ","))
		e.Lf("if (%s) != 0", expr)
		return true
	case "endc":
		e.L("endif")
		return true
	case "even":
		e.L("align 2")
		return true
	case "cnop":
		// cnop offset,n  -- align to multiple of n with offset; ie64asm has
		// `align N` only. Approximate by ignoring offset.
		if len(l.Operands) >= 2 {
			e.Lf("align %s", strings.TrimSpace(l.Operands[1]))
			return true
		}
	}
	return false
}
