package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestANTICNoDead6502SurfaceOrPrivateFramebuffer(t *testing.T) {
	type forbidden struct {
		file    string
		pattern string
	}
	checks := []forbidden{
		{"video_antic.go", `Handle6502Read`},
		{"video_antic.go", `Handle6502Write`},
		{"video_antic.go", `Handle6502GTIARead`},
		{"video_antic.go", `Handle6502GTIAWrite`},
		{"video_antic.go", `advanceToNextScanline`},
		{"video_antic.go", `debugFrameCount`},
		{"video_antic.go", `debugWriteCount`},
		{"video_antic.go", `\bstatusReads\b`},
		{"video_antic.go", `(?m)^\s*frameBuffer\s+\[\]byte\b`},
		{"antic_constants.go", `C6502_ANTIC_`},
		{"antic_constants.go", `C6502_GTIA_`},
		{"sdk/include/ie65.inc", `ANTIC_DMACTL\s*=\s*\$D400`},
		{"sdk/include/ie80.inc", `\.set\s+ANTIC_BASE,0xD400`},
	}

	for _, check := range checks {
		data, err := os.ReadFile(filepath.FromSlash(check.file))
		if err != nil {
			t.Fatalf("read %s: %v", check.file, err)
		}
		if regexp.MustCompile(check.pattern).Find(data) != nil {
			t.Fatalf("%s still contains forbidden ANTIC dead-surface pattern %q", check.file, check.pattern)
		}
	}
}
