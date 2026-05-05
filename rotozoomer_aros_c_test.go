//go:build headless

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotozoomerArosC_TextureLoadChecksOpenAndShortRead(t *testing.T) {
	root := assemblerExamplesRepoRoot(t)
	for _, rel := range []string{
		"sdk/examples/c/rotozoomer_aros_api.c",
		"sdk/examples/c/rotozoomer_aros_hw.c",
	} {
		t.Run(filepath.Base(rel), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			src := string(data)
			open := strings.Index(src, `Open("PROGDIR:rotozoomtexture.raw"`)
			if open < 0 {
				t.Fatal("texture Open call not found")
			}
			window := src[open:]
			if len(window) > 900 {
				window = window[:900]
			}
			if !strings.Contains(window, "if (!fh)") {
				t.Fatal("texture Open failure is not checked immediately")
			}
			if !strings.Contains(window, "bytes_read != TEX_SIZE") {
				t.Fatal("texture Read short count is not checked")
			}
		})
	}
}

func TestRotozoomerArosC_APIWaitTOFOpensGraphicsLibrary(t *testing.T) {
	src := readDemoSource(t, "sdk/examples/c/rotozoomer_aros_api.c")
	if !strings.Contains(src, "#include <graphics/gfxbase.h>") {
		t.Fatal("API C demo must include graphics/gfxbase.h for GfxBase")
	}
	if !strings.Contains(src, "struct GfxBase *GfxBase = NULL") {
		t.Fatal("API C demo must define GfxBase for WaitTOF")
	}
	mainIdx := strings.Index(src, "int main(void)")
	if mainIdx < 0 {
		t.Fatal("main function not found")
	}
	mainSrc := src[mainIdx:]
	openIdx := strings.Index(mainSrc, `OpenLibrary("graphics.library"`)
	waitIdx := strings.Index(mainSrc, "wait_vsync();")
	closeIdx := strings.Index(src, "CloseLibrary((struct Library *)GfxBase)")
	if openIdx < 0 {
		t.Fatal("API C demo does not open graphics.library")
	}
	if waitIdx < 0 {
		t.Fatal("API C demo does not call wait_vsync from main loop")
	}
	if openIdx > waitIdx {
		t.Fatal("graphics.library is opened after the main loop wait_vsync call")
	}
	if closeIdx < 0 {
		t.Fatal("API C demo does not close graphics.library")
	}
}
