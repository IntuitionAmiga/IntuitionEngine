// Command m68kto64 transpiles Motorola m68k (vasm/devpac) assembly to IE64
// assembly that can be assembled by sdk/bin/ie64asm.
//
// See sdk/docs/M68KtoIE64.md and .claude/plans/M68KtoIE64plan.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Indirection points for testability — tests stub osExit and osArgs to
// exercise the main entry without terminating the test process.
var (
	osExit = os.Exit
	osArgs = os.Args
)

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
	sizeFlag := fs.String("size", ".l", "Default size suffix (.l or .q)")
	fs.Usage = func() {
		fmt.Fprintf(stderrW, "Usage: m68kto64 [options] input.s\n\nSee sdk/docs/M68KtoIE64.md.\n\nOptions:\n")
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
	data, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintf(stderrW, "error reading %s: %v\n", in, err)
		return 1
	}

	c := NewConverter()
	c.noHeader = *noHeader
	c.noFlagsFuse = *noFlagsFuse
	c.strict = *strict
	c.defaultSize = *sizeFlag
	source, errs := c.ConvertSource(string(data))

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
