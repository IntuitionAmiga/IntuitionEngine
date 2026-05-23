// Render report.json into a per-chapter markdown report.
//
// Layout:
//   - Top: counts by status (PASS / FAIL / SKIP / ERROR / LINT_PASS / LINT_FAIL).
//   - Per chapter: a table with columns Line | Status | Kind | Command |
//     Expected | Actual. FAIL rows expand below with a fenced diff and the
//     monitor_dump if present.

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func renderReport(in, out string) {
	report, err := readReport(in)
	if err != nil {
		die(err)
	}
	md := renderMarkdown(report)
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		die(err)
	}
	fmt.Printf("prm-extract: rendered %d case(s) → %s\n", len(report.Cases), out)
}

func renderMarkdown(report *Report) string {
	var sb strings.Builder
	counts := map[string]int{}
	for _, c := range report.Cases {
		counts[c.Status]++
	}
	sb.WriteString("# PRM Doc-as-Test Report\n\n")
	sb.WriteString("## Summary\n\n")
	for _, s := range []string{RStatusPass, RStatusFail, RStatusSkip, RStatusError, RStatusLintPass, RStatusLintFail} {
		if n := counts[s]; n > 0 {
			sb.WriteString(fmt.Sprintf("- **%s**: %d\n", s, n))
		}
	}
	sb.WriteString("\n")

	byChapter := map[string][]ReportCase{}
	for _, c := range report.Cases {
		byChapter[c.Source] = append(byChapter[c.Source], c)
	}
	var chapters []string
	for k := range byChapter {
		chapters = append(chapters, k)
	}
	sort.Strings(chapters)
	for _, ch := range chapters {
		sb.WriteString(fmt.Sprintf("## %s\n\n", ch))
		sb.WriteString("| Line | Status | Kind | Command | Expected | Actual |\n")
		sb.WriteString("|---|---|---|---|---|---|\n")
		cases := byChapter[ch]
		sort.SliceStable(cases, func(i, j int) bool {
			return cases[i].FenceStartLine < cases[j].FenceStartLine
		})
		for _, c := range cases {
			if len(c.Steps) == 0 {
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | _(no steps)_ | _-_ | %s |\n",
					c.FenceStartLine, c.Status, c.Kind, escTbl(c.SkipReason+c.Error)))
				continue
			}
			for _, s := range c.Steps {
				cmd := s.Cmd
				if cmd == "" {
					cmd = s.Input
				}
				exp := strings.Join(s.Expected, "↵")
				act := strings.Join(s.Actual, "↵")
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s |\n",
					c.FenceStartLine, s.Status, c.Kind, escTbl(cmd), escTbl(exp), escTbl(act)))
			}
		}
		sb.WriteString("\n")
		for _, c := range cases {
			if c.Status != RStatusFail && c.Status != RStatusError {
				continue
			}
			sb.WriteString(fmt.Sprintf("### FAIL %s:%d (%s)\n\n", c.Source, c.FenceStartLine, c.ID))
			if c.Error != "" {
				sb.WriteString("```\nerror: " + c.Error + "\n```\n\n")
			}
			for _, s := range c.Steps {
				if s.Status != RStatusFail && s.Status != RStatusError {
					continue
				}
				cmd := s.Cmd
				if cmd == "" {
					cmd = s.Input
				}
				sb.WriteString(fmt.Sprintf("- step `%s`\n", cmd))
				sb.WriteString("```diff\n")
				for _, l := range s.Expected {
					sb.WriteString("- " + l + "\n")
				}
				for _, l := range s.Actual {
					sb.WriteString("+ " + l + "\n")
				}
				sb.WriteString("```\n")
			}
			if c.MonitorDump != "" {
				sb.WriteString("monitor_dump:\n```\n" + c.MonitorDump + "\n```\n\n")
			}
		}
	}
	return sb.String()
}

func escTbl(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "↵")
	return s
}
