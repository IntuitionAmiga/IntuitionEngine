package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSDKDocPDFRenderManifestMatchesCurrentInputs(t *testing.T) {
	const manifestPath = "sdk/docs/verify/SDK_DOC_PDF_RENDER_MANIFEST.sha256"
	file, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	entries := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("malformed render-manifest row %q", scanner.Text())
		}
		entries[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	required := []string{
		"sdk/docs/IE64_ISA.md", "sdk/docs/IE64_ISA.pdf",
		"sdk/docs/IE32_ISA.md", "sdk/docs/IE32_ISA.pdf",
		"sdk/docs/iemon.md", "sdk/docs/iemon.pdf",
		"sdk/docs/iescript.md", "sdk/docs/iescript.pdf",
		"sdk/docs/architecture.md", "sdk/docs/architecture.pdf",
		"sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md",
		"sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md",
		"sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md",
		"sdk/include/intuitionengine.h",
		"build_x64_ie_img.sh",
		"scripts/dist-host-sdk-linux-amd64.sh",
		"scripts/sdk-companion-pdf.sh", "scripts/refman-pdf.sh",
	}
	rootGo, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	required = append(required, rootGo...)

	for _, path := range required {
		want, ok := entries[path]
		if !ok {
			t.Errorf("render manifest omits required input %s", path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("render manifest hash for %s = %s, want %s", path, want, got)
		}
	}
}
