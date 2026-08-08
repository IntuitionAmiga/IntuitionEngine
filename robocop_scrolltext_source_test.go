//go:build headless

package main

import (
	"os"
	"strings"
	"testing"
)

// TestRobocopScrolltextUsesActiveDrawFramebuffer prevents glyph blits from
// targeting the fixed front buffer while the demo is rendering the next frame
// into its alternating draw buffer.
func TestRobocopScrolltextUsesActiveDrawFramebuffer(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		function string
		want     string
		forbid   string
	}{
		{
			name:     "IE32",
			path:     "sdk/examples/asm/robocop_intro.asm",
			function: "draw_scrolltext:",
			want:     "ADD T, @VAR_DRAW_FB",
			forbid:   "ADD T, #VRAM_START",
		},
		{
			name:     "M68K",
			path:     "sdk/examples/asm/robocop_intro_68k.asm",
			function: "draw_scrolltext:",
			want:     "add.l   VAR_DRAW_FB,d0",
			forbid:   "addi.l  #VRAM_START,d0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			body := string(source)
			start := strings.Index(body, tc.function)
			if start < 0 {
				t.Fatalf("%s does not contain %q", tc.path, tc.function)
			}
			body = body[start:]
			if strings.Contains(body, tc.forbid) {
				t.Fatalf("%s scrolltext targets the fixed framebuffer: %q", tc.path, tc.forbid)
			}
			if !strings.Contains(body, tc.want) {
				t.Fatalf("%s scrolltext does not target the active draw framebuffer: want %q", tc.path, tc.want)
			}
		})
	}
}
