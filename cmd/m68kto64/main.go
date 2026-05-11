// Command m68kto64 transpiles Motorola m68k (vasm/devpac) assembly to IE64
// assembly that can be assembled by sdk/bin/ie64asm.
//
// The pipeline is: os.ReadFile → Preprocess (vasm/devpac preprocessor:
// include / -D / equ / set / = capture, generic if/ifd/ifnd/ifeq/ifne plus
// the elseif* family with first-true latch, macro / endm / mexit, rept /
// endr, \1..\9 positional args, globally-monotonic \@ unique-label suffix)
// → ConvertLines (m68k → IE64 lowering with CMP/Bcc + FCMP/FBcc fuse and
// shadow CCR/FPCC) → emit. See sdk/docs/m68Kto64.md and
// .claude/plans/M68KtoIE64plan.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Indirection points for testability — tests stub osExit and osArgs to
// exercise the main entry without terminating the test process.
var (
	osExit = os.Exit
	osArgs = os.Args
)

// repeatedString is a flag.Value backing repeatable string flags (e.g. -I).
type repeatedString struct {
	vals *[]string
}

func (r repeatedString) String() string {
	if r.vals == nil {
		return ""
	}
	return strings.Join(*r.vals, ",")
}

func (r repeatedString) Set(v string) error {
	*r.vals = append(*r.vals, v)
	return nil
}

// defineFlag is a flag.Value backing repeatable -D NAME[=VALUE] flags. Whitespace
// around `=` is rejected per plan §Expression evaluator. Bare `-D NAME` seeds
// to 1.
type defineFlag struct {
	defs map[string]int64
}

func (d defineFlag) String() string {
	if d.defs == nil {
		return ""
	}
	parts := make([]string, 0, len(d.defs))
	for k, v := range d.defs {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return strings.Join(parts, ",")
}

func (d defineFlag) Set(v string) error {
	if d.defs == nil {
		return fmt.Errorf("defineFlag uninitialized")
	}
	eq := strings.IndexByte(v, '=')
	if eq < 0 {
		name := v
		if name == "" {
			return fmt.Errorf("empty -D name")
		}
		if strings.ContainsAny(name, " \t") {
			return fmt.Errorf("-D %q: whitespace not allowed", v)
		}
		d.defs[name] = 1
		return nil
	}
	name := v[:eq]
	val := v[eq+1:]
	if strings.ContainsAny(name, " \t") || strings.ContainsAny(val, " \t") {
		return fmt.Errorf("-D %q: whitespace around '=' not allowed", v)
	}
	if name == "" {
		return fmt.Errorf("-D %q: empty symbol name", v)
	}
	n, err := parseDefineLiteral(val)
	if err != nil {
		return fmt.Errorf("-D %s: %v", v, err)
	}
	d.defs[name] = n
	return nil
}

// parseDefineLiteral parses the value portion of a -D NAME=VALUE flag. Accepts
// the same literal grammar as the source-level expr engine: decimal, $hex,
// 0x..., %bin, optional leading sign.
func parseDefineLiteral(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("missing digits")
	}
	var n int64
	var err error
	switch {
	case strings.HasPrefix(s, "$"):
		n, err = strconv.ParseInt(s[1:], 16, 64)
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		n, err = strconv.ParseInt(s[2:], 16, 64)
	case strings.HasPrefix(s, "%"):
		n, err = strconv.ParseInt(s[1:], 2, 64)
	default:
		n, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		return 0, err
	}
	if neg {
		n = -n
	}
	return n, nil
}

func main() {
	osExit(run(osArgs[1:], os.Stderr))
}

// run is the testable entry point. Returns the exit code; writes diagnostic
// messages to stderrW.
func run(args []string, stderrW io.Writer) int {
	fs := flag.NewFlagSet("m68kto64", flag.ContinueOnError)
	fs.SetOutput(stderrW)
	outFile := fs.String("o", "", "Output file (default: <input>_ie64.s)")
	noHeader := fs.Bool("no-header", false, "Omit header comment")
	noFlagsFuse := fs.Bool("no-flags-fuse", false, "Disable CMP/Bcc fuse (debug aid)")
	strict := fs.Bool("strict", false, "Error on unfused flag spans / unsupported ops")
	fpIrqWrap := fs.Bool("fp-irq-wrap", false, "Auto-wrap RTE handlers with FP-slot save/restore (Phase 2 opt-in)")
	sizeFlag := fs.String("size", ".l", "Default size suffix (.l or .q)")
	labelSalt := fs.String("label-salt", "", "Namespace __m68kto64_* labels with this salt (prevents collisions in multi-TU concat builds)")
	flagLiveness := fs.Bool("flag-liveness", false, "Phase H: elide shadow N/Z/C/V/X emission when no downstream consumer reads them (opt-in)")

	opts := DefaultPreprocOpts()
	opts.Defines = map[string]int64{}
	fs.Var(repeatedString{&opts.IncludeDirs}, "I", "Add directory to include search path (repeatable)")
	fs.Var(defineFlag{opts.Defines}, "D", "Define symbol; -D NAME or -D NAME=VALUE (repeatable)")
	fs.BoolVar(&opts.StripCond, "strip-cond", false, "Strip if/else/endif wrappers from output (Model B)")
	fs.IntVar(&opts.MaxMacroRecurs, "max-macro-recurs", opts.MaxMacroRecurs, "Max macro expansion depth")
	fs.BoolVar(&opts.WerrorUnknownMnem, "Werror-unknown-mnemonic", opts.WerrorUnknownMnem, "Treat unknown mnemonics as errors")
	fs.BoolVar(&opts.NoDefaultSeeds, "no-default-seeds", false, "Skip IE-convenience symbol seeds (IS_IE=1)")

	fs.Usage = func() {
		fmt.Fprintf(stderrW, "Usage: m68kto64 [options] input.s\n\nSee sdk/docs/m68Kto64.md.\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 1
	}
	in := fs.Arg(0)

	c := NewConverter()
	c.noHeader = *noHeader
	c.noFlagsFuse = *noFlagsFuse
	c.strict = *strict
	c.fpIrqWrap = *fpIrqWrap
	c.defaultSize = *sizeFlag
	c.labelSalt = *labelSalt
	c.flagLiveness = *flagLiveness
	source, errs := c.ConvertFile(in, opts, stderrW)
	if errs > 0 && source == "" {
		// Pure preprocessor failure (e.g. read error or lone-CR rejection);
		// ConvertFile already wrote a diagnostic to stderrW.
		return 1
	}

	out := *outFile
	if out == "" {
		out = strings.TrimSuffix(in, ".s") + "_ie64.s"
	}
	if err := os.WriteFile(out, []byte(source), 0o644); err != nil {
		fmt.Fprintf(stderrW, "error writing %s: %v\n", out, err)
		return 1
	}
	if errs > 0 {
		fmt.Fprintf(stderrW, "%d conversion error(s); search for '; ERROR:' in %s\n", errs, out)
		return 1
	}
	return 0
}
