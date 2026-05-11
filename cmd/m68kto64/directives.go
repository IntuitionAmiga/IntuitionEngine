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

// labelBindingDirective reports whether a directive folds the preceding
// label into its own line — i.e. emitting `LABEL:` ahead of it would create
// a duplicate symbol definition.
func labelBindingDirective(mnem string) bool {
	switch mnem {
	case "equ", "set", "=", "rs":
		return true
	}
	return false
}

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
	case "section":
		// vasm `section` is layout metadata; ie64asm assembles into a single
		// flat output. Drop with a diagnostic line so the source-to-output
		// trace is still legible.
		if len(l.Operands) > 0 {
			e.Lf("; m68kto64: dropped section %s", strings.Join(l.Operands, ","))
		}
		return true
	case "ifd":
		// ie64asm has no `defined()` predicate. The preprocessor-time symbol
		// table (seeded with IS_IE=1 by default; extended via -D and source
		// equ/set/= captures) decides defined-ness. Unknown symbols → if 0.
		name := ""
		if len(l.Operands) == 1 {
			name = strings.TrimSpace(l.Operands[0])
		}
		if c.symtab != nil && c.symtab.Has(name) {
			e.L("if 1")
		} else {
			e.L("if 0")
		}
		return true
	case "ifnd":
		name := ""
		if len(l.Operands) == 1 {
			name = strings.TrimSpace(l.Operands[0])
		}
		if c.symtab != nil && c.symtab.Has(name) {
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
	case "equ", "set", "=":
		// ie64asm parses `LABEL equ EXPR` / `LABEL set EXPR` as a single
		// line with the symbol in field 0. Emit the bound form, never the
		// `LABEL:\n equ EXPR` two-line shape — the latter silently mis-
		// defines the symbol as a code-offset label.
		ie := mnem
		if ie == "=" {
			ie = "set"
		}
		if l.Label == "" {
			e.Lf("; m68kto64: dropped orphan %s %s", mnem, strings.Join(l.Operands, ","))
			return true
		}
		e.Lf("%s %s %s", l.Label, ie, strings.Join(l.Operands, ","))
		return true
	case "ds":
		// `ds.<sz> N` reserves N elements zero-filled — pass through.
		// `ds.<sz> N,V` is vasm-only with an init value; ie64asm has no
		// equivalent, so unroll to `dc.<sz> V,V,...` (or back to plain
		// `ds.<sz> N` when V==0). One operand → passthrough.
		if len(l.Operands) < 2 {
			return false
		}
		// Two-operand form falls through to dcb handler.
		l.Mnemonic = "dcb"
		return c.emitDirective(e, l)
	case "dcb", "blk":
		// vasm `dcb.<sz> N,V` (or alias `blk`) — emit N copies of V as a
		// `dc.<sz>` list. ie64asm has no dcb/blk equivalent; the expanded
		// form keeps the same on-disk layout. The N expression must be a
		// preprocessor-known constant so we can unroll at transpile time.
		if len(l.Operands) < 1 {
			e.Lf("; m68kto64: empty %s%s", mnem, l.Size)
			return true
		}
		nExpr := strings.TrimSpace(l.Operands[0])
		vExpr := "0"
		if len(l.Operands) >= 2 {
			vExpr = strings.TrimSpace(l.Operands[1])
		}
		n, err := EvalExpr(nExpr, c.symtab)
		if err != nil || n < 0 {
			e.Lf("; m68kto64: %s%s count %q not constant: %v", mnem, l.Size, nExpr, err)
			return true
		}
		size := l.Size
		if size == "" {
			size = ".w"
		}
		// Unroll. For large N use ds.<sz> fill-zero shortcut when V==0.
		if vExpr == "0" || vExpr == "$0" || vExpr == "#0" {
			e.Lf("ds%s %d", size, n)
			return true
		}
		// Otherwise emit a comma-joined dc.<sz> list. Chunk per 64 entries
		// to keep output lines a sane length.
		const chunk = 64
		for off := int64(0); off < n; off += chunk {
			end := off + chunk
			if end > n {
				end = n
			}
			vals := make([]string, end-off)
			for i := range vals {
				vals[i] = vExpr
			}
			e.Lf("dc%s %s", size, strings.Join(vals, ","))
		}
		return true
	case "output", "opt":
		// vasm meta-directives. `output` names the object-file output;
		// `opt` toggles assembler options. Both are no-ops for the
		// transpile-then-ie64asm pipeline.
		e.Lf("; m68kto64: dropped %s %s", mnem, strings.Join(l.Operands, ","))
		return true
	case "rsreset":
		// vasm `rsreset` — reset the implicit structure-offset counter
		// (__rs) to zero. Emit as a mutable `set` so subsequent `rs.x`
		// rewrites can read it.
		e.L("__m68kto64_rs set 0")
		return true
	case "rs":
		// vasm `LABEL rs.<sz> N` — assign LABEL = current __rs, then
		// advance __rs by N * sizeof(sz). With a missing label this is
		// just an advance.
		size := SizeBytes(l.Size)
		if size == 0 {
			size = 2 // vasm default for rs is .w
		}
		count := "1"
		if len(l.Operands) >= 1 && strings.TrimSpace(l.Operands[0]) != "" {
			count = strings.TrimSpace(l.Operands[0])
		}
		if l.Label != "" {
			e.Lf("%s equ __m68kto64_rs", l.Label)
		}
		e.Lf("__m68kto64_rs set __m68kto64_rs + (%s) * %d", count, size)
		return true
	}
	return false
}
