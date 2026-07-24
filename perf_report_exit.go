// perf_report_exit.go - dump the subsystem perf report at process exit.
//
// The PerfSubsysAcct counters (VoodooSwapWait, VoodooReadback, VoodooFlush, ...)
// were previously reachable only through IEScript sys.perf_report(). A guest
// image with no measurement script therefore produced no output even with
// IE_PERF_ACCT=1. This prints the report to stderr on exit when accounting is
// on, so a plain `IE_PERF_ACCT=1 ./IntuitionEngine game.ie68` yields the numbers.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"fmt"
	"os"
	"sync"
)

var perfReportOnce sync.Once

// dumpSubsysPerfReport prints the subsystem perf report to stderr once, when
// IE_PERF_ACCT is on and any counter recorded activity.
func dumpSubsysPerfReport() {
	if !perfAcctOn {
		return
	}
	perfReportOnce.Do(func() {
		report := perfSubsysAcct.Report()
		body := "[IE_PERF_ACCT] subsystem report:\n" + report
		if report == "" {
			body = "[IE_PERF_ACCT] subsystem report: no instrumented activity recorded\n"
		}
		fmt.Fprint(os.Stderr, body)
		// Also write to a file when IE_PERF_ACCT_OUT is set, so the report
		// survives launchers that do not forward the child process stderr
		// (e.g. switcherooctl).
		if path := os.Getenv("IE_PERF_ACCT_OUT"); path != "" {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "[IE_PERF_ACCT] failed to write %s: %v\n", path, err)
			}
		}
	})
}
