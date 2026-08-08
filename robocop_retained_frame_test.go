//go:build headless

package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRobocopPortsUseRetainedFrameComposition(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		want      []string
		forbidden []string
	}{
		{
			name: "IE32",
			path: "sdk/examples/asm/robocop_intro.asm",
			want: []string{
				"LDA #0\n    STA @VIDEO_FB_BASE",
				"JSR wait_frame",
				"LDA #3\n    STA @VIDEO_CTRL",
				"JSR wait_blit\n    ; Publish the completed retained frame as one presentation step.\n    LDA #1\n    STA @VIDEO_CTRL",
				"LDA T\n    STA @BLT_DST",
				"LDA #FRONT_FB\n    STA @VAR_DRAW_FB",
				"STC @VAR_PREV_X_ADDR",
			},
			forbidden: []string{"Rebuild the complete back frame", ".use_back_next:"},
		},
		{
			name: "M68K",
			path: "sdk/examples/asm/robocop_intro_68k.asm",
			want: []string{
				"move.l  #FRONT_FB,d0\n    move.l  d0,VIDEO_FB_BASE",
				"jsr     wait_frame",
				"moveq   #3,d0\n    move.l  d0,VIDEO_CTRL",
				"jsr     wait_blit\n    ; Publish the completed retained frame as one presentation step.\n    moveq   #1,d0\n    move.l  d0,VIDEO_CTRL",
				"move.l  d6,BLT_DST",
				"move.l  #FRONT_FB,d0\n    move.l  d0,VAR_DRAW_FB",
				"move.l  d2,VAR_PREV_X_ADDR",
			},
			forbidden: []string{"Rebuild the complete back frame", ".use_back_next:"},
		},
		{
			name: "6502",
			path: "sdk/examples/asm/robocop_intro_65.asm",
			want: []string{
				"lda #0\n    sta VIDEO_FB_BASE+2",
				"jsr wait_frame",
				"lda #3\n    sta VIDEO_CTRL",
				"jsr wait_blit\n\n    ; Publish the completed retained frame.\n    lda #1\n    sta VIDEO_CTRL",
				"jsr clear_prev_sprite",
				"lda #FRONT_FB_BANK\n    sta draw_fb_bank",
				"sta prev_x",
			},
			forbidden: []string{"Rebuild the complete back frame", "@use_back_next:"},
		},
		{
			name: "Z80",
			path: "sdk/examples/asm/robocop_intro_z80.asm",
			want: []string{
				"xor a\n    ld (VIDEO_FB_BASE+2),a",
				"call wait_frame",
				"ld a,3\n    ld (VIDEO_CTRL),a",
				"call wait_blit\n\n    ; Publish the completed retained frame.\n    ld a,1\n    ld (VIDEO_CTRL),a",
				"call clear_prev_sprite",
				"ld a,FRONT_FB_BANK\n    ld (draw_fb_bank),a",
				"ld (prev_x),hl",
			},
			forbidden: []string{"Rebuild the complete back frame", ".use_back_next:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("%s is missing retained-frame operation %q", tc.path, want)
				}
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still contains full-frame composition state %q", tc.path, forbidden)
				}
			}
		})
	}
}

func TestRobocopPublishedImagesMatchPrebuilt(t *testing.T) {
	tests := []struct {
		prebuilt  string
		published string
	}{
		{"sdk/examples/prebuilt/robocop_intro.iex", "intuitionengine.com/assets/Demos/ie32/robocop_intro.iex"},
		{"sdk/examples/prebuilt/robocop_intro_68k.ie68", "intuitionengine.com/assets/Demos/m68k/robocop_intro_68k.ie68"},
		{"sdk/examples/prebuilt/robocop_intro_65.ie65", "intuitionengine.com/assets/Demos/m6502/robocop_intro_65.ie65"},
		{"sdk/examples/prebuilt/robocop_intro_z80.ie80", "intuitionengine.com/assets/Demos/z80/robocop_intro_z80.ie80"},
	}

	for _, tc := range tests {
		t.Run(filepath.Base(tc.prebuilt), func(t *testing.T) {
			prebuilt, err := os.ReadFile(tc.prebuilt)
			if err != nil {
				t.Fatal(err)
			}
			published, err := os.ReadFile(tc.published)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(prebuilt) != sha256.Sum256(published) {
				t.Fatalf("published image %s is stale relative to %s", tc.published, tc.prebuilt)
			}
		})
	}
}
